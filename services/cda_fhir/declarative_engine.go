// services/cda_fhir/declarative_engine.go
//
// DeclarativeEngine — Phase 2's interpreter loop, modeled on
// createFHIRResourceFromSection (generic_mapper.go:775) but resolving
// against Phase 1's wildcard/predicate paths over the typed "document.*"
// tree instead of a pre-flattened entry map. See the Phase 2 design note in
// architecture/CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md for the schema this
// interprets (declarative_schema.go) and why each dependency below was
// chosen over the more obvious-looking alternative already in this package.
package cdafhir

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/services/executors"
)

// DeclarativeEngine builds FHIR resources from a typed CDA document tree
// (already JSON-round-tripped to map[string]interface{}, the same shape
// Phase 1's resolver targets) by interpreting MappingRules instead of
// dispatching to a hardcoded per-section Go mapper function.
type DeclarativeEngine struct {
	transformReg *DeclarativeTransformRegistry

	// PatientRef is the FHIR reference string (e.g. "Patient/patient-1") the
	// document-level orchestrator (Phase 4 Slice B) computes once per
	// document and assigns here before calling BuildResources/
	// BuildResourcesForRules/BuildHeaderResource. Deliberately engine state,
	// not a Build* parameter: ~80 existing call sites across
	// declarative_engine_test.go/declarative_oob_rules_test.go/
	// declarative_oob_rules_corpus_test.go never exercise patient linkage, so
	// threading a new parameter through every signature would force touching
	// all of them for no behavioral reason. Zero value "" means every Build*
	// call behaves exactly as it did before this field existed -- see
	// MappingRule.PatientRefPath's doc comment for how this is applied.
	PatientRef string

	// emittedIDMu/emittedIDCounters give every EmitAsResource-built resource
	// a document-wide-unique id (e.g. "condition-1", "condition-2") at
	// emission time, independent of the section-level per-entry idx the
	// caller happens to be looping over. Needed because multiple matched
	// entries (e.g. two Encounters, each with their own nested-diagnosis
	// CollectAll loop) each restart their own loop's idx at 0 — without a
	// shared counter, two unrelated emitted Conditions could both compute
	// "condition-1" and collide. Mutex-protected because
	// BuildResourcesForRules' per-section dispatch in
	// declarative_document_mapper.go runs one goroutine per section, all
	// sharing this one engine instance.
	emittedIDMu       sync.Mutex
	emittedIDCounters map[string]int

	// CoverageTracker records which section entries this engine actually
	// built a resource attempt from, for the optional, per-interface CDA
	// Coverage Audit feature. Same write-once-before-goroutine-fan-out
	// pattern as PatientRef above, for the same reason: set once by
	// DeclarativeMapDocument before the per-section goroutines launch, never
	// written again, so no additional locking is needed on this field itself
	// (the tracker's own internal state is separately mutex-protected — see
	// services/executors/cda_coverage_tracker.go). Nil (the common case,
	// audit disabled) makes every CDACoverageTracker method a no-op.
	CoverageTracker *executors.CDACoverageTracker
}

// nextEmittedID returns a document-wide-unique, 1-based sequence number for
// resourceType, scoped to this one engine instance (one per document — see
// DeclarativeMapDocument). See emittedIDCounters' doc comment for why this
// can't just be the caller's own loop index.
func (e *DeclarativeEngine) nextEmittedID(resourceType string) int {
	e.emittedIDMu.Lock()
	defer e.emittedIDMu.Unlock()
	if e.emittedIDCounters == nil {
		e.emittedIDCounters = make(map[string]int)
	}
	e.emittedIDCounters[resourceType]++
	return e.emittedIDCounters[resourceType]
}

// NewDeclarativeEngine constructs an engine with its own transform registry.
func NewDeclarativeEngine() *DeclarativeEngine {
	return &DeclarativeEngine{transformReg: NewDeclarativeTransformRegistry()}
}

// BuildResources runs one MappingRule against documentMap (ParsedJSON["document"],
// or any map sharing that shape) and returns one FHIR resource per matched
// entry, plus any field-level errors/warnings — the same SectionError shape
// MapOutput/ProcessingResult already use elsewhere in this package.
//
// The returned slice may contain MIXED resource types: any row with
// EmitAsResource set (e.g. a recorder/asserter Practitioner/PractitionerRole/
// Organization) has its sub-resource appended into this same slice alongside
// the section's primary resource (AllergyIntolerance, Condition, etc.), via
// the `extra` return of buildOneResource above. This is intentional —
// declarative_document_mapper.go appends this exact mixed-type slice straight
// into the FHIR Bundle, where mixed resource types are normal. Callers that
// need only the primary resource type must filter by resourceType first; see
// TestDeclarativeEngine_CareTeam_BuildsCareTeamAndPractitioner
// (declarative_oob_rules_test.go) for the established pattern.
//
// Use BuildResourcesForRules instead when more than one rule targets the
// same SectionKey (e.g. Medications' moodCode-driven MedicationRequest vs
// MedicationStatement split) — calling BuildResources separately per rule
// would let a later rule's EntryMatch re-claim entries an earlier rule
// already matched. BuildResourcesForRules has the identical mixed-resource-
// type caveat at its own concatenation point.
func (e *DeclarativeEngine) BuildResources(
	documentMap map[string]interface{},
	rule MappingRule,
) ([]map[string]interface{}, []SectionError) {
	entryPath := buildEntryMatchPath(rule.SectionKey, rule.EntryMatch)
	entryNodes := executors.ResolveCDAPaths(documentMap, entryPath, false)
	flattened := wrapAsFlattenedEntries(entryNodes)
	if rule.FlattenOrganizers {
		flattened = flattenOrganizerEntries(entryNodes)
	}

	var resources []map[string]interface{}
	var errs []SectionError

	for entryIdx, fe := range flattened {
		entryMap, ok := fe.node.(map[string]interface{})
		if !ok {
			continue
		}
		// coveragePrefix is always "" here (see this function's own
		// zero-tracking callers), so the computed basePath is threaded for
		// consistency only -- buildOneResource's own coveragePrefix==""
		// short-circuit means it never actually matters.
		basePath := sectionEntryBasePath("", fe.basePrefix, entryMap)
		resource, extra, rowErrs := e.buildOneResource(entryMap, rule, entryIdx, "", basePath, "CDAEntry")
		errs = append(errs, rowErrs...)
		resources = append(resources, extra...)
		if resource != nil {
			resources = append(resources, resource)
		}
	}

	return resources, errs
}

// wrapAsFlattenedEntries lifts a plain node slice into []flattenedEntry with
// no organizer prefix -- the non-FlattenOrganizers common case, sharing one
// loop shape with the flattened case at each call site.
func wrapAsFlattenedEntries(nodes []interface{}) []flattenedEntry {
	out := make([]flattenedEntry, len(nodes))
	for i, n := range nodes {
		out[i] = flattenedEntry{node: n}
	}
	return out
}

// BuildResourcesForRules runs multiple MappingRules against the same
// documentMap, implementing the first-match-wins group dispatch described in
// MappingRule's doc comment: rules are evaluated in slice order, and an
// entry already claimed by an earlier rule for the same SectionKey is not
// reconsidered by a later one. This is what makes Medications'
// EntryMatch="moodCode=INT" → MedicationRequest rule, followed by an
// EntryMatch="" → MedicationStatement rule, behave like
// medication_mapper.go's `if moodCode == "INT" {...} else {...}` instead of
// producing both resources for the same INT entry.
//
// FlattenOrganizers is honored here too (not just in BuildResources) as of
// CarePlan/Goal: plan_of_care_mapper.go's flattenPlanOfCareEntries expands
// organizer components before its own moodCode/EntryType dispatch switch,
// the same "organizer wraps several real activities" shape Vital
// Signs/Results/Social History already needed FlattenOrganizers for. Every
// rule sharing one SectionKey MUST set FlattenOrganizers consistently
// (all-or-none) — claimed[] tracks entries by *index into this rule's own
// allEntries slice*, and flattening changes that slice's length/order, so
// mixing flattened and unflattened rules for the same SectionKey would
// silently misalign claims across rules.
//
// See BuildResources' doc comment above: the returned slice may likewise
// contain EmitAsResource sub-resources mixed in with each rule's primary
// resource type.
func (e *DeclarativeEngine) BuildResourcesForRules(
	documentMap map[string]interface{},
	rules []MappingRule,
) ([]map[string]interface{}, []SectionError) {
	return e.BuildResourcesForRulesWithCoveragePrefix(documentMap, rules, "")
}

// BuildResourcesForRulesWithCoveragePrefix is BuildResourcesForRules, plus a
// caller-supplied CDA Coverage Audit element-tracking prefix (see
// recordElementCoverage, declarative_engine.go) threaded down to every row
// this call resolves.
//
// Exists as a separate function, not a new BuildResourcesForRules parameter,
// specifically to avoid touching the ~80 existing BuildResources/
// BuildResourcesForRules call sites across declarative_engine_test.go/
// declarative_oob_rules_test.go/declarative_oob_rules_corpus_test.go (none
// of which set CoverageTracker or care about this) — same rationale
// DeclarativeEngine.PatientRef's own doc comment already documents for a
// different field.
//
// coveragePrefix must be the caller's own real, section-relative
// CDAEntryKey (e.g. "vitalSigns#3") for the SPECIFIC entry documentMap
// resolves against — this function has no way to compute that itself
// (documentMap may be declarative_document_mapper.go's per-entry synthetic
// single-entry wrapper, in which case entries resolved here would all
// appear at index 0 regardless of which real entry is being processed; see
// buildOneResource's own doc comment for the identical, already-fixed
// problem at entry granularity). The one real caller,
// declarative_document_mapper.go's outer per-entry loop, already computes
// this exact key for its own entry-level Record() call and passes the same
// value here — not two independently-computed prefixes that could drift.
func (e *DeclarativeEngine) BuildResourcesForRulesWithCoveragePrefix(
	documentMap map[string]interface{},
	rules []MappingRule,
	coveragePrefix string,
) ([]map[string]interface{}, []SectionError) {
	claimed := make(map[string]map[int]bool)

	var allResources []map[string]interface{}
	var allErrs []SectionError

	for _, rule := range rules {
		allEntries := executors.ResolveCDAPaths(documentMap, "sectionsByKey."+rule.SectionKey+".entries[*]", false)
		flattened := wrapAsFlattenedEntries(allEntries)
		if rule.FlattenOrganizers {
			flattened = flattenOrganizerEntries(allEntries)
		}
		if claimed[rule.SectionKey] == nil {
			claimed[rule.SectionKey] = make(map[int]bool)
		}

		for idx, fe := range flattened {
			if claimed[rule.SectionKey][idx] || !entryMatchesPredicate(fe.node, rule.EntryMatch) {
				continue
			}
			claimed[rule.SectionKey][idx] = true

			entryMap, ok := fe.node.(map[string]interface{})
			if !ok {
				continue
			}
			basePath := sectionEntryBasePath(coveragePrefix, fe.basePrefix, entryMap)
			resource, extra, rowErrs := e.buildOneResource(entryMap, rule, idx, coveragePrefix, basePath, "CDAEntry")
			allErrs = append(allErrs, rowErrs...)
			allResources = append(allResources, extra...)
			if resource != nil {
				allResources = append(allResources, resource)
			}
		}
	}

	return allResources, allErrs
}

// BuildHeaderResource runs one MappingRule against documentMap's document
// header (see MappingRule.HeaderPath's doc comment) instead of a section's
// entries, producing at most one resource — header constructs are
// inherently singular (one document has one author being mapped, one
// custodian), unlike section entries which can repeat. Coverage-tracking-
// free convenience wrapper over BuildHeaderResourceWithCoveragePrefix; see
// that function's own doc comment for the real behavior.
func (e *DeclarativeEngine) BuildHeaderResource(
	documentMap map[string]interface{},
	rule MappingRule,
) (map[string]interface{}, []map[string]interface{}, []SectionError) {
	return e.BuildHeaderResourceWithCoveragePrefix(documentMap, rule, "")
}

// BuildHeaderResourceWithCoveragePrefix is BuildHeaderResource, plus CDA
// Coverage Audit element-level recording -- the header-field counterpart to
// BuildResourcesForRulesWithCoveragePrefix. coveragePrefixBase is a
// "header.<construct>" literal (e.g. "header.patient") the caller supplies;
// "" means "not wired up here" (every OTHER caller, including every
// existing test) and behaves EXACTLY as before this function existed.
//
// Two header-specific wrinkles neither section entries nor
// BuildResourcesForRulesWithCoveragePrefix have:
//
//  1. rule.HeaderPath=="authors" doesn't resolve through the translator at
//     all -- WHICH raw <author> gets used is a Go predicate
//     (firstAuthorWithPersonIndexed: "first author with a usable
//     assignedPerson"), not a path segment. Its own real, selected index
//     becomes both this construct's CDAEntryKey index (so an unselected
//     author's raw content still shows as its OWN distinct, genuinely-
//     missed unit -- services/cda_coverage/inventory.go's
//     buildHeaderInventory walks every raw author, not just the selected
//     one) and the literal "author[N]" basePath.
//  2. Every other HeaderPath resolves via the SAME translator entries as
//     the real value lookup (still done the untranslated way just below),
//     but rooted at the new "CDAHeader" struct-kind -- e.g. resolving
//     "encompassingEncounter.location.healthCareFacility" (the Location
//     rule) yields a basePath several levels deeper than resolving bare
//     "encompassingEncounter" (the Encounter rule), even though both
//     share the SAME coveragePrefixBase ("header.encompassingEncounter")
//     -- see this function's 5 real call sites in
//     declarative_document_mapper.go for why that sharing is required
//     (their typed roots physically overlap).
func (e *DeclarativeEngine) BuildHeaderResourceWithCoveragePrefix(
	documentMap map[string]interface{},
	rule MappingRule,
	coveragePrefixBase string,
) (map[string]interface{}, []map[string]interface{}, []SectionError) {
	headerMap, ok := documentMap["header"].(map[string]interface{})
	if !ok || headerMap == nil {
		return nil, nil, nil
	}

	var entryMap map[string]interface{}
	index := 0
	var basePath, baseKind string

	if rule.HeaderPath == "authors" {
		var found bool
		entryMap, index, found = firstAuthorWithPersonIndexed(headerMap)
		if !found {
			return nil, nil, nil
		}
		if coveragePrefixBase != "" {
			// header_parser.go:158 -- root.SelectElements("author"), a
			// direct, repeatable child of the document root itself; always
			// index 0 relative to ITS OWN position (no further wrapper).
			basePath = fmt.Sprintf("author[%d]", index)
			baseKind = "CDAAuthor"
		}
	} else {
		nodes := executors.ResolveCDAPaths(headerMap, rule.HeaderPath, false)
		if len(nodes) == 0 {
			return nil, nil, nil
		}
		entryMap, _ = nodes[0].(map[string]interface{})
		if entryMap == nil {
			return nil, nil, nil
		}
		if coveragePrefixBase != "" {
			if node := executors.ResolveCDAPathTranslated(headerMap, rule.HeaderPath, "CDAHeader"); node.Translatable {
				basePath, baseKind = node.XMLPath, node.Kind
			}
		}
	}

	coveragePrefix := ""
	if coveragePrefixBase != "" {
		coveragePrefix = executors.CDAEntryKey(coveragePrefixBase, index)
		e.CoverageTracker.Record(coveragePrefix)
	}

	return e.buildOneResource(entryMap, rule, 0, coveragePrefix, basePath, baseKind)
}

// firstAuthorWithPersonIndexed returns the first element of
// headerMap["authors"] whose assignedAuthor.assignedPerson carries at least
// one name, AND its own real index among every raw author present — mirrors
// patient_mapper.go's MapAuthor loop ("return on first author with a
// person") exactly. An existence check ("has a person", not "person equals
// X"), which is why this is a small Go helper instead of an EntryMatch
// bracket-predicate: that grammar only supports equality. The index is
// needed by BuildHeaderResourceWithCoveragePrefix so an author that exists
// in the raw document but was never selected still gets its own distinct,
// genuinely-tracked coverage unit rather than silently sharing index 0 with
// whichever author WAS selected.
func firstAuthorWithPersonIndexed(headerMap map[string]interface{}) (entryMap map[string]interface{}, index int, ok bool) {
	authors, _ := headerMap["authors"].([]interface{})
	for i, a := range authors {
		am, isMap := a.(map[string]interface{})
		if !isMap {
			continue
		}
		assignedAuthor, _ := am["assignedAuthor"].(map[string]interface{})
		if assignedAuthor == nil {
			continue
		}
		person, _ := assignedAuthor["assignedPerson"].(map[string]interface{})
		if person == nil {
			continue
		}
		if names, _ := person["names"].([]interface{}); len(names) > 0 {
			return am, i, true
		}
	}
	return nil, 0, false
}

// sectionEntryBasePath computes buildOneResource's basePath for a SECTION
// entry (as opposed to a header construct, which BuildHeaderResourceWith
// CoveragePrefix computes its own way) -- raw XML's <entry> directly wraps
// exactly one of the 7 act tags (observation|act|organizer|
// substanceAdministration|procedure|encounter|supply), which the typed
// parser flattens away into entryMap's own top-level fields (code,
// statusCode, effectiveTime, value, ...) -- see executors.
// CDAEntryActTagPrefix's own doc comment. Every recorded key for THIS
// entry's own fields must start from that wrapper segment, or it will never
// match inventory.go's walkEntryElements (which walks the unflattened raw
// XML and sees the wrapper as a real, present level, e.g.
// "encounter[0].code[0]" not bare "code[0]"). Gated on coveragePrefix != ""
// so a zero-tracking caller pays no extra cost.
//
// basePrefix (from flattenOrganizerEntries, e.g. "organizer[0].component[3]")
// is prepended when this entry is itself a flattened organizer component --
// without it, every component sibling under the same organizer would
// collapse onto the identical "observation[0].code[0]"-style key (a real
// bug found live: Vital Signs read as 0/45 clinical elements found despite
// genuinely mapping several vitals correctly, because every one of them
// recorded under the SAME collapsed key that never matched any single one
// of the raw mirror's per-component inventory paths).
func sectionEntryBasePath(coveragePrefix, basePrefix string, entryMap map[string]interface{}) string {
	if coveragePrefix == "" {
		return ""
	}
	prefix, ok := executors.CDAEntryActTagPrefix(entryMap)
	if !ok {
		return ""
	}
	return executors.JoinCDAXMLPath(basePrefix, prefix)
}

// buildOneResource applies every MappingRow in rule.Fields to one already-
// selected entry, shared by both BuildResources and BuildResourcesForRules
// so the per-row error/severity bookkeeping isn't duplicated between them.
//
// The second return value carries any resources emitted as a side effect of
// an EmitAsResource row (e.g. CareTeam's per-participant Practitioners) —
// these are independent top-level resources, not nested inside the main one,
// and are returned regardless of whether the main resource itself ends up
// kept or discarded: careteam_mapper.go's MapCareTeam appends practitioners
// unconditionally, before even checking whether buildCareTeamResource
// produced a CareTeam at all.
func (e *DeclarativeEngine) buildOneResource(
	entryMap map[string]interface{},
	rule MappingRule,
	entryIdx int,
	coveragePrefix string,
	basePath string,
	baseKind string,
) (map[string]interface{}, []map[string]interface{}, []SectionError) {
	// CDA Coverage Audit, ENTRY granularity is NOT recorded here. entryIdx is
	// relative to whatever documentMap this call's caller resolved entries
	// from — for declarative_document_mapper.go's live per-entry loop (the
	// only production caller; every other caller is a direct-engine unit
	// test that never sets CoverageTracker), that documentMap is a synthetic
	// single-entry wrapper (see that loop's own comment on why), so entryIdx
	// here is ALWAYS 0 regardless of which real entry is being processed —
	// recording here would mark only "sectionKey#0" as touched no matter how
	// many entries a section actually has. That loop already carries the one
	// real, section-relative entryIdx (its own range variable) and records
	// with it directly, unconditionally, for every entry it feeds into
	// BuildResourcesForRules — see that loop's own CoverageTracker.Record
	// call for the actual recording point.
	//
	// ELEMENT granularity has the identical problem and the identical fix:
	// coveragePrefix is NOT derived from entryIdx here (that was tried and
	// reverted — see git history/session notes if this comment is ever
	// questioned) for exactly the same reason. It is instead threaded in as
	// a parameter, sourced from the same real, section-relative key
	// declarative_document_mapper.go's outer loop already computes for
	// entry-level recording — see BuildResourcesForRulesWithCoveragePrefix,
	// the entry point that actually carries it down to here. Empty string
	// (every OTHER caller: BuildResources, BuildResourcesForRules, and thus
	// every existing test) means "not wired up," and recordElementCoverage
	// silently no-ops — zero behavioral change for any caller that doesn't
	// opt in.
	resource := map[string]interface{}{"resourceType": rule.FHIRResource}
	var errs []SectionError
	var extra []map[string]interface{}

	// CDA Coverage Audit, element granularity: basePath/baseKind are already
	// FINAL by the time they reach this function -- computed by the caller
	// (sectionEntryBasePath for section entries; BuildHeaderResourceWith
	// CoveragePrefix for header constructs), since the correct derivation
	// differs by caller (CDAEntryActTagPrefix's entryType-based lookup only
	// makes sense for CDAEntry-shaped nodes; a header construct has no
	// entryType at all). Passed straight through to every row -- see
	// sectionEntryBasePath's own doc comment for why section entries need
	// this, and BuildHeaderResourceWithCoveragePrefix's for the header case.
	for _, row := range rule.Fields {
		if err := e.applyRow(resource, entryMap, row, &extra, coveragePrefix, basePath, baseKind); err != nil {
			severity := "warning"
			if row.Required || row.Conformance == "SHALL" {
				severity = "error"
			}
			errs = append(errs, SectionError{
				SectionKey: rule.SectionKey,
				EntryIndex: entryIdx,
				FieldKey:   row.TargetPath,
				Transform:  row.Transform,
				Error:      err.Error(),
				Severity:   severity,
			})
		}
	}

	// additionalValues surfaces (never silently drops) any CDA <value>
	// sibling beyond the first -- see additionalValuesWarningMessages' own
	// doc comment. Attributable directly to a SectionError here (SectionKey/
	// EntryIndex are both in scope) since this is the rule's own top-level
	// entry; buildEmittedSubResource's analogous check (a nested
	// EmitAsResource resource, e.g. an Assessment Scale Supporting
	// Observation COMP child) has no such context available at that depth
	// and stamps a temporary "_warnings" marker instead -- see that
	// function's own doc comment for why, and declarative_document_mapper.go
	// for where that marker is read back into a SectionError (with the
	// section/entry context ITS caller has) and stripped before the bundle
	// ships.
	for _, msg := range additionalValuesWarningMessages(entryMap) {
		errs = append(errs, SectionError{
			SectionKey: rule.SectionKey,
			EntryIndex: entryIdx,
			FieldKey:   "value",
			Error:      msg,
			Severity:   "warning",
		})
	}

	// SkipIfCodeNullFlavor discards only THIS entry's own main resource --
	// checked AFTER Fields already ran (not before, as a short-circuit)
	// specifically so any EmitAsResource child built by one of the Fields
	// above (already collected into extra) survives even when the entry
	// itself has no real code to build a conformant Observation from. Real
	// gap found auditing Functional Status (99397 sample): its "Alcohol Use"
	// and PHQ-2 Assessment Scale Observation (.4.69) shells carry
	// code.nullFlavor="UNK" (non-conformant -- the IG's own
	// AssessmentScaleObservation StructureDefinition requires code+value
	// both [1..1]), but their COMP-nested Assessment Scale Supporting
	// Observation children (.4.86, which DOES mandate code [1..1] LOINC) are
	// fully conformant, real clinical data (e.g. AUDIT-C/PHQ-2 question/
	// answer pairs) -- the early-return this check used to do (before any
	// Field ran) discarded those children too, silently losing real data
	// instead of just the code-less, FHIR-non-conformant shell. See
	// FunctionalStatusMappingRules' own doc comment for the fix this enabled.
	if rule.SkipIfCodeNullFlavor {
		if code, ok := entryMap["code"].(map[string]interface{}); ok {
			nullFlavor, _ := code["nullFlavor"].(string)
			realCode, _ := code["code"].(string)
			// A null-flavor code with no structured coding can still carry a
			// real, code-specific label via <code><originalText> -- a valid,
			// non-empty Observation.code (CodeableConcept.text alone is
			// FHIR-conformant). Real gap found auditing Results (Marshfield
			// sample): a full cytology/pathology report (real diagnosis text,
			// signing pathologist, date) has exactly this shape
			// (code.nullFlavor="UNK", code.originalText="Thin Prep, PAP and
			// HPV if ASCUS") and was being discarded entirely just because it
			// has no LOINC code -- losing genuinely substantive clinical
			// content, not a conformance-incomplete shell like the Functional
			// Status .4.69 case this guard was written for (confirmed via the
			// 99397 sample: those shells are a BARE <code nullFlavor="UNK"/>
			// with no originalText at all, so they keep discarding correctly
			// here). A section's own genuinely-empty placeholder shell (e.g.
			// Dignity Health's "No data available" Results entry) also has no
			// originalText on its code element, so it isn't affected either.
			originalText, _ := code["originalText"].(string)
			if nullFlavor != "" && realCode == "" && originalText == "" {
				return nil, extra, errs
			}
		}
	}

	// Only include non-empty resources (at least one field was set beyond
	// resourceType) — same convention as createFHIRResourceFromSection.
	// SkipEmptyCheck bypasses this for the one rule (Author) whose Go
	// counterpart never had an equivalent check — see that field's doc
	// comment in declarative_schema.go.
	if countRealKeys(resource) <= 1 && !rule.SkipEmptyCheck {
		return nil, extra, errs
	}
	if !resourceHasAllPaths(resource, rule.RequiredPaths) {
		return nil, extra, errs
	}

	if e.PatientRef != "" {
		for _, path := range rule.PatientRefPath {
			if path != "" {
				resource[path] = map[string]interface{}{"reference": e.PatientRef}
			}
		}
	}

	return resource, extra, errs
}

// describeCDAValueForWarning renders a JSON-shaped CDAValue node (the "value"
// or one "additionalValues[i]" entry) as a short human-readable string for
// the additionalValues warning above -- code+display when coded, else the
// raw text/boolean/integer/real, else "(no value)" for a bare nullFlavor.
// Deliberately tolerant of any shape (every field is a best-effort type
// assertion) since this only feeds a warning message, never a written FHIR
// field -- a malformed node here must never itself produce an error.
func describeCDAValueForWarning(v interface{}) string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return "(no value)"
	}
	if code, ok := m["code"].(map[string]interface{}); ok {
		codeStr, _ := code["code"].(string)
		display, _ := code["displayName"].(string)
		switch {
		case codeStr != "" && display != "":
			return codeStr + " " + display
		case codeStr != "":
			return codeStr
		}
	}
	if text, ok := m["text"].(string); ok && text != "" {
		return text
	}
	if b, ok := m["boolean"]; ok {
		return fmt.Sprintf("%v", b)
	}
	if i, ok := m["integer"]; ok {
		return fmt.Sprintf("%v", i)
	}
	if r, ok := m["real"]; ok {
		return fmt.Sprintf("%v", r)
	}
	return "(no value)"
}

// additionalValuesWarningMessages returns one human-readable message per CDA
// <value> sibling beyond the first present on entryMap (cda/document's
// entry_parser.go keeps the first in "value" -- the one
// observationValueXDispatchRows-style rows actually map -- and the rest in
// "additionalValues", since C-CDA allows [1..*] <value> on some templates
// (e.g. Assessment Scale Supporting Observation) but FHIR R4's
// Observation.value[x] can only ever hold one). Returns nil when there's
// nothing extra -- the overwhelmingly common case, so callers can append
// the result unconditionally with no extra branching.
//
// Used by both buildOneResource (the rule's own top-level entry, where
// SectionKey/EntryIndex are already in scope to attach directly to a
// SectionError) and buildEmittedSubResource (a nested EmitAsResource
// resource, where they are not -- see that function's own doc comment for
// how it surfaces this instead). Real gap found auditing Functional Status
// (99397 sample): a PHQ-2 total-score entry's second <value> (a SNOMED
// interpretation, alongside the INT score already mapped) was silently
// discarded with no trace anywhere. Deliberately produces warnings, not
// errors -- per user direction, the engine's mapping behavior is unchanged
// (first value always wins); this only makes sure a human can see that a
// second value existed and was not mapped.
func additionalValuesWarningMessages(entryMap map[string]interface{}) []string {
	extras, ok := entryMap["additionalValues"].([]interface{})
	if !ok || len(extras) == 0 {
		return nil
	}
	kept := describeCDAValueForWarning(entryMap["value"])
	messages := make([]string, 0, len(extras))
	for _, ev := range extras {
		extraMap, _ := ev.(map[string]interface{})
		messages = append(messages, "entry has more than one <value>; only the first was mapped (kept: "+kept+") -- not mapped: "+describeCDAValueForWarning(extraMap))
	}
	return messages
}

// entryMatchesPredicate evaluates an EntryMatch predicate clause against an
// already-resolved entry node, reusing Phase 1's bracket-predicate resolver
// instead of duplicating its (unexported) matching logic: entryNode is
// wrapped in a single-key synthetic map so the same "[key=value,...]"
// bracket grammar that ordinarily narrows a *path* can narrow this one
// already-in-hand node instead.
func entryMatchesPredicate(entryNode interface{}, entryMatch string) bool {
	if entryMatch == "" {
		return true
	}
	wrapper := map[string]interface{}{"_entry": entryNode}
	return len(executors.ResolveCDAPaths(wrapper, "_entry["+entryMatch+"]", false)) > 0
}

// buildEntryMatchPath turns a MappingRule's EntryMatch predicate clause into
// the Phase 1 path that selects matching entries: Phase 1's resolver applied
// at the section level is entryMatch, with no new parsing logic required.
func buildEntryMatchPath(sectionKey, entryMatch string) string {
	base := "sectionsByKey." + sectionKey + ".entries"
	if entryMatch == "" {
		return base + "[*]"
	}
	return base + "[" + entryMatch + "]"
}

// flattenOrganizerEntries expands any "organizer" node with non-empty
// "components" into those components, in place of the organizer itself —
// mirrors observation_mapper.go's flattenObservationEntries exactly. A
// non-organizer node, or an organizer with no components, passes through
// unchanged. See MappingRule.FlattenOrganizers' doc comment.
// flattenedEntry is one entry flattenOrganizerEntries produced, plus the
// raw-XML basePath PREFIX it needs for CDA Coverage Audit element-level
// recording. A flattened component's own act-tag prefix alone (e.g.
// "observation[0]", from executors.CDAEntryActTagPrefix) would collide
// across every sibling component under the same organizer -- raw XML has
// each at its own "organizer[0].component[N].observation[0]" position, not
// a bare "observation[0]" repeated identically for every one. BasePrefix is
// "" for a pass-through (non-organizer) entry, where the normal
// CDAEntryActTagPrefix(entryMap) computation alone is already correct.
type flattenedEntry struct {
	node       interface{}
	basePrefix string
}

// flattenOrganizerEntries replaces an "organizer"-type entry with its own
// nested component entries (one per <component>), for rules that opted in
// via MappingRule.FlattenOrganizers (e.g. a Vital Signs Organizer wrapping
// several individual vital-sign Observations, or a Results Organizer
// wrapping several lab Result Observations). The organizer wrapper is
// always index [0] relative to its own section entry (raw XML's <entry>
// wraps exactly one act tag -- same reasoning as CDAEntryActTagPrefix's own
// doc comment), so each flattened component's basePrefix is
// "organizer[0].component[<its own real index>]".
func flattenOrganizerEntries(nodes []interface{}) []flattenedEntry {
	var flat []flattenedEntry
	for _, node := range nodes {
		nodeMap, ok := node.(map[string]interface{})
		if ok && nodeMap["entryType"] == "organizer" {
			if components, ok := nodeMap["components"].([]interface{}); ok && len(components) > 0 {
				for idx, c := range components {
					flat = append(flat, flattenedEntry{
						node:       c,
						basePrefix: "organizer[0].component[" + strconv.Itoa(idx) + "]",
					})
				}
				continue
			}
		}
		flat = append(flat, flattenedEntry{node: node})
	}
	return flat
}

// countRealKeys counts resource's top-level keys excluding "_cdaIds" — the
// internal lineage/debug marker EmbedCDAIdentity rows attach (declarative_
// engine.go's CollectAll branch, ~line 457). Both "did anything real get
// set" survival gates (buildOneResource and buildEmittedSubResource) need
// this instead of a bare len(): a real gap found auditing Vital Signs
// against the 99397 sample -- every author there has
// <id root="2.16.840.1.113883.4.6" nullFlavor="UNK"/> (no @extension, the
// bare NPI system OID with no actual NPI value), which
// transforms.IIToIdentifier correctly recognizes as not a real identifier
// (see that function's own doc comment) and never writes to "identifier" --
// but EmbedCDAIdentity's own check only looks at root/extension being
// non-empty, not nullFlavor, so it still recorded a useless "_cdaIds" entry.
// That alone was enough to push len(sub) from 1 to 2, defeating the gate and
// letting an otherwise completely content-free Practitioner (no name, no
// identifier, no telecom, no address — just resourceType + the phantom
// debug key) survive into the bundle as a real, emitted resource.
// "_emitted" (buildEmittedSubResource's own marker) doesn't need the same
// exclusion: it's added AFTER that function's gate check, never before.
func countRealKeys(resource map[string]interface{}) int {
	n := len(resource)
	if _, ok := resource["_cdaIds"]; ok {
		n--
	}
	return n
}

// resourceHasAnyKey reports whether resource already has any of keys set —
// see MappingRow.SkipIfResourceHasAnyOf's doc comment.
func resourceHasAnyKey(resource map[string]interface{}, keys []string) bool {
	for _, k := range keys {
		if _, ok := resource[k]; ok {
			return true
		}
	}
	return false
}

// resourceHasAllPaths reports whether resource has ALL of paths set as
// top-level keys — see MappingRule.RequiredPaths' doc comment.
func resourceHasAllPaths(resource map[string]interface{}, paths []string) bool {
	for _, p := range paths {
		if _, ok := resource[p]; !ok {
			return false
		}
	}
	return true
}

// isNilValue reports whether v is nil OR a typed nil boxed in an interface
// (a nil map/slice/pointer) -- the classic Go gotcha where `var m
// map[string]interface{}; var i interface{} = m; i == nil` is FALSE, because
// the interface carries a concrete type even though its value is nil.
// Found while porting Vital Signs: transforms.CDACodeToCodeableConcept
// (and several siblings) explicitly `return nil` as a typed `map[string]interface{}`
// — every transform that does this, returned through
// DeclarativeTransformFn's `interface{}` return type, used to slip past a
// plain `== nil` check and get written into the resource as a literal JSON
// `null` instead of being skipped (caught via real PracticeFusion/Kareo
// corpus data: a fully nullFlavor=NI Result organizer produced
// `"valueCodeableConcept": null` in output instead of omitting the key).
// Pre-existing in every section ported so far, not new to this rule — fixed
// at the one shared chokepoint instead of patched per-row.
func isNilValue(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// applyRow resolves and writes one MappingRow's value(s) into resource.
// A Scope that matches nothing is not an error — it's exactly how "which row
// applies" falls out for free (see declarative_schema.go's doc comment).
//
// extraOut, when non-nil, accumulates any resources emitted by a nested
// EmitAsResource row reached while applying this row — via the
// row.CollectAll+Fields branch below (one EmitAsResource per matched node),
// or via row.EmitAsResource itself on a singular (non-CollectAll) row, which
// recurses into buildEmittedSubResource the same way and lets that emitted
// resource's OWN Fields contain further EmitAsResource rows (e.g.
// PractitionerRole.practitioner/.organization, each its own emitted
// resource) — extraOut is threaded all the way down so those land as
// independent top-level resources too, not nested values. Most callers pass
// nil: only buildOneResource (the top of the recursion) needs to actually
// collect these.
func (e *DeclarativeEngine) applyRow(resource map[string]interface{}, entryMap map[string]interface{}, row MappingRow, extraOut *[]map[string]interface{}, coveragePrefix, basePath, baseKind string) error {
	if row.Disabled {
		return nil
	}
	if resourceHasAnyKey(resource, row.SkipIfResourceHasAnyOf) {
		return nil
	}
	scopedNodes := e.resolveScope(entryMap, row.Scope, row.ScopeFallbacks)
	if len(scopedNodes) == 0 {
		return nil
	}

	// CDA Coverage Audit, element granularity. translatedScope is ONLY
	// computed when tracking is actually active (nil CoverageTracker or ""
	// coveragePrefix are both "not wired up here" signals — see
	// recordElementCoverage's own doc comment) — every other row in a
	// document that hasn't opted in pays zero extra resolution cost. Also
	// needed below by the CollectAll+Fields/EmitAsResource branches, which
	// each recurse into further applyRow calls that need THEIR OWN basePath/
	// baseKind continuing from wherever the specific match that owns them
	// resolved to — computed once here, by index, and threaded down, rather
	// than re-resolved once per branch.
	var translatedScope []executors.CDAResolvedNode
	if e.CoverageTracker != nil && coveragePrefix != "" {
		translatedScope = executors.ResolveCDAPathsTranslated(entryMap, row.Scope, baseKind)
		e.recordElementCoverage(entryMap, coveragePrefix, basePath, baseKind, translatedScope, row.Scope, row.SourcePath)
	}

	if row.CollectAll {
		if row.EmitAsResource != "" {
			return e.applyCollectAllEmitAsResource(resource, scopedNodes, row, extraOut, coveragePrefix, basePath, translatedScope)
		}
		if len(row.Fields) > 0 {
			return e.applyCollectAllWithFields(resource, scopedNodes, row, extraOut, coveragePrefix, basePath, translatedScope)
		}

		idx := 0
		var cdaIds []interface{}
		for _, scoped := range scopedNodes {
			if row.EmbedCDAIdentity {
				if m, ok := scoped.(map[string]interface{}); ok {
					root, _ := m["root"].(string)
					ext, _ := m["extension"].(string)
					if root != "" || ext != "" {
						cdaIds = append(cdaIds, map[string]interface{}{"root": root, "extension": ext})
					}
				}
			}
			resolved := e.resolveWithCondition(scoped, row)
			transformed, err := e.transformReg.Apply(resolved.transform, resolved.value, resolved.valueMap)
			if err != nil {
				return err
			}
			if isNilValue(transformed) {
				continue
			}
			if err := e.checkValueSet(row.ValueSetURL, transformed); err != nil {
				return err
			}
			setFHIRPath(resource, indexedPath(resolved.targetPath, idx), transformed)
			idx++
		}
		if len(cdaIds) > 0 {
			resource["_cdaIds"] = cdaIds
		}
		return nil
	}

	if row.EmitAsResource != "" {
		scopedMap, ok := scopedNodes[0].(map[string]interface{})
		if !ok {
			return nil
		}
		childBasePath, childBaseKind := childCoverageContext(basePath, translatedScope, 0)
		sub, refVal := e.buildEmittedSubResource(scopedMap, row, extraOut, coveragePrefix, childBasePath, childBaseKind)
		if sub == nil {
			return nil
		}
		if extraOut != nil {
			*extraOut = append(*extraOut, sub)
		}
		setFHIRPath(resource, row.TargetPath, refVal)
		return nil
	}

	resolved := e.resolveWithCondition(scopedNodes[0], row)
	if resolved.value == nil {
		if row.Required || row.Conformance == "SHALL" {
			return fmt.Errorf("empty value for %s-conformance field", row.Conformance)
		}
		return nil
	}
	transformed, err := e.transformReg.Apply(resolved.transform, resolved.value, resolved.valueMap)
	if err != nil {
		return err
	}
	if isNilValue(transformed) {
		return nil
	}
	if err := e.checkValueSet(row.ValueSetURL, transformed); err != nil {
		return err
	}
	setFHIRPath(resource, resolved.targetPath, transformed)
	return nil
}

// recordElementCoverage records the XML-mirror element(s) row's Scope, and
// (when set at this row's own level) Scope+SourcePath, resolve to — CDA
// Coverage Audit's element-level tracking, see cda_element_translation.go.
// A side channel to the row's real value resolution (resolveScope/
// resolveRowSourceValue just above), deliberately kept separate so
// element-level recording can never alter what value actually gets mapped,
// only what gets recorded as "read."
//
// coveragePrefix is this row's entry's own CDAEntryKey (e.g. "vitalSigns#3")
// — empty means this call site isn't wired up for element-level recording
// (e.g. BuildHeaderResource, permanently out of scope — see that function's
// own call site) and is a safe, silent no-op. basePath/baseKind place this
// row correctly within nested recursion (e.g. inside a matched
// entryRelationship's own act) — both are "" at the top level, where
// entryMap already IS the entry root. translatedScope is row.Scope's own
// ALREADY-COMPUTED translation (applyRow computes it once and passes it
// here AND to whichever recursive branch needs it, rather than resolving
// the same path twice).
func (e *DeclarativeEngine) recordElementCoverage(entryMap map[string]interface{}, coveragePrefix, basePath, baseKind string, translatedScope []executors.CDAResolvedNode, scope, sourcePath string) {
	if coveragePrefix == "" {
		return
	}
	record := func(node executors.CDAResolvedNode) {
		if node.Translatable && node.XMLPath != "" {
			e.CoverageTracker.Record(coveragePrefix + "/" + executors.JoinCDAXMLPath(basePath, node.XMLPath))
		}
	}
	for _, node := range translatedScope {
		record(node)
	}
	if sourcePath == "" {
		return
	}
	combined := sourcePath
	if scope != "" {
		combined = scope + "." + sourcePath
	}
	for _, node := range executors.ResolveCDAPathsTranslated(entryMap, combined, baseKind) {
		record(node)
	}
}

// childCoverageContext derives the basePath/baseKind a recursive applyRow
// call should use for the idx-th match in translatedScope — that match's
// own translated XMLPath (joined onto the parent's basePath) and Kind. All
// three of: translatedScope being nil (tracking inactive for this call —
// see applyRow's own comment), idx out of range, or that specific match not
// being Translatable, safely return ("", "") — a recursive call with an
// empty coveragePrefix downstream (unrelated to this) or empty basePath/
// baseKind simply records nothing deeper, never guesses.
func childCoverageContext(basePath string, translatedScope []executors.CDAResolvedNode, idx int) (childBasePath, childBaseKind string) {
	if idx < 0 || idx >= len(translatedScope) {
		return "", ""
	}
	node := translatedScope[idx]
	if !node.Translatable {
		return "", ""
	}
	return executors.JoinCDAXMLPath(basePath, node.XMLPath), node.Kind
}

// applyCollectAllWithFields implements MappingRow.Fields' "one sub-object
// per Scope match" semantics — see that field's doc comment for why this
// exists (two independent CollectAll rows can't be kept index-aligned;
// building one sub-object per match in one pass makes misalignment
// structurally impossible). Each child row is applied via the SAME applyRow
// every other row in this engine goes through — no special-cased recursion,
// just a fresh subObj as the "resource" and the matched node as the
// "entryMap" for that one recursive call.
func (e *DeclarativeEngine) applyCollectAllWithFields(resource map[string]interface{}, scopedNodes []interface{}, row MappingRow, extraOut *[]map[string]interface{}, coveragePrefix, basePath string, translatedScope []executors.CDAResolvedNode) error {
	// Seed idx from row.TargetPath's CURRENT length rather than always 0:
	// EncounterMappingRules has two independent CollectAll+Fields rows
	// (Scope="participants[*]", then Scope="performers[*]") both writing
	// into TargetPath="participant" — without this, the second row's own
	// idx restarting at 0 would call setFHIRPath("participant[0]", ...) and
	// silently overwrite the first row's already-written participant[0]
	// (ensureArray preserves existing elements, but arr[idx]=value still
	// replaces whichever one is already sitting at that index). Confirmed
	// via real corpus data: the LOC participant a prior row had written
	// disappeared, replaced by the new performer-derived PRF participant,
	// before this fix.
	idx := 0
	if existing, ok := resource[row.TargetPath].([]interface{}); ok {
		idx = len(existing)
	}
	for i, scoped := range scopedNodes {
		scopedMap, ok := scoped.(map[string]interface{})
		if !ok {
			continue
		}
		// This specific match's own basePath/baseKind for CDA Coverage Audit
		// — scopedMap is reached via translatedScope[i], the SAME index
		// (both scopedNodes and translatedScope walk the identical
		// wildcard/predicate fan-out over the identical Scope path, so they
		// stay index-aligned — see applyRow's own translatedScope comment).
		childBasePath, childBaseKind := childCoverageContext(basePath, translatedScope, i)
		subObj := map[string]interface{}{}
		for _, childRow := range row.Fields {
			if childRow.EmitAsResource != "" {
				// See MappingRow.EmitAsResource's doc comment: this child row
				// builds an INDEPENDENT resource (e.g. CareTeam's per-
				// participant Practitioner), not a value inside subObj —
				// subObj only gets a Reference to it.
				if sub, refVal := e.buildEmittedSubResource(scopedMap, childRow, extraOut, coveragePrefix, childBasePath, childBaseKind); sub != nil {
					if extraOut != nil {
						*extraOut = append(*extraOut, sub)
					}
					setFHIRPath(subObj, childRow.TargetPath, refVal)
				}
				continue
			}
			// Errors are intentionally swallowed here, not propagated: every
			// field ported under Fields so far is genuinely optional per the
			// IG ([0..1]/[0..*] cardinality) — a missing child field is
			// normal, not a conformance violation worth surfacing at the
			// resource level. Revisit if a future child row's Required/SHALL
			// genuinely needs to be visible.
			_ = e.applyRow(subObj, scopedMap, childRow, extraOut, coveragePrefix, childBasePath, childBaseKind)
		}
		if len(subObj) == 0 {
			continue
		}
		setFHIRPath(resource, indexedPath(row.TargetPath, idx), subObj)
		idx++
	}
	return nil
}

// applyCollectAllEmitAsResource implements MappingRow.CollectAll combined
// DIRECTLY with EmitAsResource (row.Fields describing the EMITTED
// resource's own fields, not a wrapping subObj's fields): each Scope match
// becomes its own independent resource via buildEmittedSubResource, pushed
// to extraOut, with a bare {"reference": ...} appended to
// resource[row.TargetPath] per match. Deliberately a separate code path
// from applyCollectAllWithFields (CollectAll+Fields with NO EmitAsResource
// on the outer row) rather than a variant of it -- that function wraps
// each match's result in a multi-field subObj, which doesn't compose with a
// bare Reference without nesting it under a spurious extra key.
//
// First real use: SocialHistoryMappingRules' SDOH Assessment Scale ->
// Assessment Scale Supporting Observation fan-out (C-CDA's
// entryRelationship typeCode=COMP, templateId
// 2.16.840.1.113883.10.20.22.4.86) -- a variable number (1 to 6+ in real
// 99397 sample data) of sibling question/answer observations, each
// becoming its own Observation resource referenced from the parent's
// hasMember[].
func (e *DeclarativeEngine) applyCollectAllEmitAsResource(resource map[string]interface{}, scopedNodes []interface{}, row MappingRow, extraOut *[]map[string]interface{}, coveragePrefix, basePath string, translatedScope []executors.CDAResolvedNode) error {
	idx := 0
	if existing, ok := resource[row.TargetPath].([]interface{}); ok {
		idx = len(existing)
	}
	for i, scoped := range scopedNodes {
		scopedMap, ok := scoped.(map[string]interface{})
		if !ok {
			continue
		}
		childBasePath, childBaseKind := childCoverageContext(basePath, translatedScope, i)
		sub, refVal := e.buildEmittedSubResource(scopedMap, row, extraOut, coveragePrefix, childBasePath, childBaseKind)
		if sub == nil {
			continue
		}
		if extraOut != nil {
			*extraOut = append(*extraOut, sub)
		}
		setFHIRPath(resource, indexedPath(row.TargetPath, idx), refVal)
		idx++
	}
	return nil
}

// buildEmittedSubResource builds a brand new, independent resource from
// row.Fields against scopedMap — see MappingRow.EmitAsResource's doc
// comment. Returns (nil, nil) when nothing usable was built, the same
// "len(resource) <= 1" convention buildOneResource uses for top-level
// resources. The id comes from nextEmittedID, not the caller's own loop
// index — see emittedIDCounters' doc comment for why a per-call idx isn't
// document-wide-unique on its own.
//
// extraOut is NOT forwarded directly into childRow's own applyRow calls —
// they're given a local buffer instead (see localExtra below), so a
// childRow that itself sets EmitAsResource — e.g. a PractitionerRole row
// whose Fields contain a singular "practitioner" row and a singular
// "organization" row, each EmitAsResource on its own — has ITS sub resource
// land in that local buffer first. Only once THIS resource (sub) survives
// its own len()/EmitAsResourceRequiredPaths gate does localExtra get merged
// into the real extraOut; if sub is discarded, every resource nested inside
// it is discarded right along with it, atomically, instead of leaking into
// the bundle as orphaned clutter with nothing left referencing them. This
// is what makes EmitAsResource nest (PractitionerRole -> Practitioner +
// Organization, each landing as its own independent top-level resource —
// but only when PractitionerRole itself is worth keeping).
//
// "_emitted": true is a marker the document-level orchestrator's resource-id
// renumbering pass (declarative_document_mapper.go) checks: that pass
// otherwise unconditionally overwrites every non-Patient/Practitioner/
// Organization resource's "id" with a fresh sequential number, which would
// silently invalidate the reference this function already wrote into the
// parent sub-object at emission time. Practitioner dodges this by being on
// that pass's exclusion list already; Condition (this function's first
// non-excluded-type caller, via EncounterMappingRules' nested-diagnosis row)
// needs the explicit marker instead. Stripped before the bundle ships,
// alongside "_cdaIds".
func (e *DeclarativeEngine) buildEmittedSubResource(scopedMap map[string]interface{}, row MappingRow, extraOut *[]map[string]interface{}, coveragePrefix, basePath, baseKind string) (map[string]interface{}, map[string]interface{}) {
	sub := map[string]interface{}{"resourceType": row.EmitAsResource}
	var localExtra []map[string]interface{}
	for _, childRow := range row.Fields {
		// basePath/baseKind already describe scopedMap's own position
		// relative to the true entry root -- computed by the caller (see
		// childCoverageContext), passed straight through since childRow's
		// Scope/SourcePath are resolved relative to scopedMap itself.
		_ = e.applyRow(sub, scopedMap, childRow, &localExtra, coveragePrefix, basePath, baseKind)
	}
	if countRealKeys(sub) <= 1 {
		return nil, nil
	}
	if !resourceHasAllPaths(sub, row.EmitAsResourceRequiredPaths) {
		return nil, nil
	}
	if e.PatientRef != "" {
		for _, path := range row.EmitAsResourcePatientRefPath {
			if path != "" {
				sub[path] = map[string]interface{}{"reference": e.PatientRef}
			}
		}
	}
	id := fmt.Sprintf("%s-%d", strings.ToLower(row.EmitAsResource), e.nextEmittedID(row.EmitAsResource))
	sub["id"] = id
	sub["_emitted"] = true
	// "_warnings" is a temporary marker, the EmitAsResource analogue of
	// buildOneResource's direct-to-errs additionalValues check above: this
	// function has no SectionError channel of its own (applyRow/
	// applyCollectAllEmitAsResource/applyCollectAllWithFields, every caller
	// in the chain back to buildOneResource, only ever thread resources, not
	// errors, through EmitAsResource -- a real, pre-existing limitation this
	// doesn't attempt to fix wholesale, see applyCollectAllWithFields'
	// "errors are intentionally swallowed" doc comment for the established
	// precedent). declarative_document_mapper.go's per-entry loop (which DOES
	// have the SectionKey/EntryIndex this resource's own entry belongs to)
	// reads this back into a real SectionError and strips it before the
	// resource ships, the same way it already strips "_emitted"/"_cdaIds".
	if msgs := additionalValuesWarningMessages(scopedMap); len(msgs) > 0 {
		sub["_warnings"] = msgs
	}
	if extraOut != nil {
		*extraOut = append(*extraOut, localExtra...)
	}
	return sub, map[string]interface{}{"reference": row.EmitAsResource + "/" + id}
}

// resolveScope resolves row.Scope relative to entryMap, falling back to each
// of scopeFallbacks in order (the Scope-level "field A, else field B, else
// the entry itself" primitive — see MappingRow.ScopeFallbacks) when Scope
// itself resolves to zero nodes. An empty candidate string means the entry
// itself is the scoped node — true for both Scope=="" and any ""
// encountered while walking scopeFallbacks.
func (e *DeclarativeEngine) resolveScope(entryMap map[string]interface{}, scope string, scopeFallbacks []string) []interface{} {
	if nodes := e.resolveScopeCandidate(entryMap, scope); len(nodes) > 0 {
		return nodes
	}
	for _, candidate := range scopeFallbacks {
		if nodes := e.resolveScopeCandidate(entryMap, candidate); len(nodes) > 0 {
			return nodes
		}
	}
	return nil
}

func (e *DeclarativeEngine) resolveScopeCandidate(entryMap map[string]interface{}, scope string) []interface{} {
	if scope == "" {
		return []interface{}{entryMap}
	}
	return executors.ResolveCDAPaths(entryMap, scope, false)
}

// rowResolution carries a row's value plus the (possibly Condition-overridden)
// transform/valueMap/targetPath to use for this one write.
type rowResolution struct {
	value      interface{}
	transform  string
	valueMap   map[string]string
	targetPath string
}

// resolveWithCondition resolves row's source value, then evaluates
// row.Condition (if set) against the same scoped root — Condition is value
// branching *within* an already-matched row, a different question from
// whether the row applies at all (Scope/EntryMatch).
func (e *DeclarativeEngine) resolveWithCondition(scoped interface{}, row MappingRow) rowResolution {
	res := rowResolution{
		value:      e.resolveRowSourceValue(scoped, row),
		transform:  row.Transform,
		valueMap:   row.ValueMap,
		targetPath: row.TargetPath,
	}

	if row.Condition == nil || !conditionMatches(scoped, row.Condition) {
		return res
	}

	if row.Condition.ThenLiteralValue != nil {
		res.value = row.Condition.ThenLiteralValue
	}
	if row.Condition.ThenTransform != "" {
		res.transform = row.Condition.ThenTransform
	}
	if row.Condition.ThenValueMap != nil {
		res.valueMap = row.Condition.ThenValueMap
	}
	if row.Condition.ThenTargetPath != "" {
		res.targetPath = row.Condition.ThenTargetPath
	}
	return res
}

// resolveRowSourceValue resolves row's raw value before any transform.
// LiteralValue takes priority over SourcePath; an empty SourcePath with no
// LiteralValue means "the scoped node itself is the value" (used by
// CollectAll rows whose Scope already resolves directly to scalar leaves).
//
// When SourcePath+FallbackPaths are ALL exhausted (every candidate path
// resolves empty) and LiteralValue is also set, LiteralValue is used as a
// final, literal fallback rather than nil — the "path, else path, else a
// non-empty default" 3-tier chain MedicationRequest.requester needs
// (requesterReference's literal "Ordering Provider" placeholder; us-core-21
// requires this field is never empty). This only changes behavior for rows
// that set both FallbackPaths AND LiteralValue together — a combination no
// existing row used before, so it's purely additive.
func (e *DeclarativeEngine) resolveRowSourceValue(scoped interface{}, row MappingRow) interface{} {
	if row.SourcePath == "" {
		if row.LiteralValue != nil {
			return row.LiteralValue
		}
		return scoped
	}
	if len(row.FallbackPaths) > 0 {
		paths := append([]string{row.SourcePath}, row.FallbackPaths...)
		if v := executors.ResolveCDAFallback(scoped, false, paths...); v != nil {
			return v
		}
		return row.LiteralValue
	}
	return executors.ResolveCDAPath(scoped, row.SourcePath, false)
}

// conditionMatches evaluates cond.WhenPath plus cond.WhenPaths (OR
// semantics — see RowCondition.WhenPaths) against scoped, returning whether
// any of them stringify-equal cond.Equals.
func conditionMatches(scoped interface{}, cond *RowCondition) bool {
	paths := cond.WhenPaths
	if cond.WhenPath != "" {
		paths = append([]string{cond.WhenPath}, paths...)
	}
	for _, p := range paths {
		v := executors.ResolveCDAPath(scoped, p, false)
		if stringifyForCondition(v) == cond.Equals {
			return true
		}
	}
	return false
}

// stringifyForCondition normalises a resolved value for RowCondition's
// equality check — the only comparison this schema supports, deliberately,
// per the sprint plan's Performance checklist ("not a general expression evaluator").
func stringifyForCondition(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// indexedPath rewrites a CollectAll row's TargetPath into one array-element
// path per match: "reasonCode" → "reasonCode[0]", "reasonCode[1]", ... .
// setFHIRPath/ensureArray (fhir_path_writer.go) already grow the array as
// needed — no new array-write logic required for the "collect all" primitive.
func indexedPath(targetPath string, idx int) string {
	if strings.HasSuffix(targetPath, "[]") {
		return fmt.Sprintf("%s[%d]", strings.TrimSuffix(targetPath, "[]"), idx)
	}
	return fmt.Sprintf("%s[%d]", targetPath, idx)
}

// checkValueSet validates the final transformed value against
// fhir/r4.TerminologyRegistry's compiled ValueSet membership table — see the
// Phase 2 design note for why this, not cda_terminology.TerminologyService,
// is "the TerminologyRegistry" the original task text meant.
func (e *DeclarativeEngine) checkValueSet(valueSetURL string, value interface{}) error {
	if valueSetURL == "" {
		return nil
	}
	code, ok := extractCodeForValidation(value)
	if !ok || code == "" {
		return nil
	}
	inSet, known := r4.GetTerminologyRegistry().Contains(valueSetURL, code)
	if known && !inSet {
		return fmt.Errorf("terminology validation: %q is not a member of %s", code, valueSetURL)
	}
	return nil
}

// extractCodeForValidation pulls a bare code string out of either a plain
// string value or a CodeableConcept-shaped map's first coding — the two
// shapes DeclarativeTransformRegistry transforms actually produce.
func extractCodeForValidation(value interface{}) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case map[string]interface{}:
		codings, ok := v["coding"].([]interface{})
		if !ok || len(codings) == 0 {
			return "", false
		}
		c, ok := codings[0].(map[string]interface{})
		if !ok {
			return "", false
		}
		code, ok := c["code"].(string)
		return code, ok
	default:
		return "", false
	}
}
