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
	"strings"

	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/services/executors"
)

// DeclarativeEngine builds FHIR resources from a typed CDA document tree
// (already JSON-round-tripped to map[string]interface{}, the same shape
// Phase 1's resolver targets) by interpreting MappingRules instead of
// dispatching to a hardcoded per-section Go mapper function.
type DeclarativeEngine struct {
	transformReg *DeclarativeTransformRegistry
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
// Use BuildResourcesForRules instead when more than one rule targets the
// same SectionKey (e.g. Medications' moodCode-driven MedicationRequest vs
// MedicationStatement split) — calling BuildResources separately per rule
// would let a later rule's EntryMatch re-claim entries an earlier rule
// already matched.
func (e *DeclarativeEngine) BuildResources(
	documentMap map[string]interface{},
	rule MappingRule,
) ([]map[string]interface{}, []SectionError) {
	entryPath := buildEntryMatchPath(rule.SectionKey, rule.EntryMatch)
	entryNodes := executors.ResolveCDAPaths(documentMap, entryPath, false)
	if rule.FlattenOrganizers {
		entryNodes = flattenOrganizerEntries(entryNodes)
	}

	var resources []map[string]interface{}
	var errs []SectionError

	for entryIdx, node := range entryNodes {
		entryMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}
		resource, extra, rowErrs := e.buildOneResource(entryMap, rule, entryIdx)
		errs = append(errs, rowErrs...)
		resources = append(resources, extra...)
		if resource != nil {
			resources = append(resources, resource)
		}
	}

	return resources, errs
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
func (e *DeclarativeEngine) BuildResourcesForRules(
	documentMap map[string]interface{},
	rules []MappingRule,
) ([]map[string]interface{}, []SectionError) {
	claimed := make(map[string]map[int]bool)

	var allResources []map[string]interface{}
	var allErrs []SectionError

	for _, rule := range rules {
		allEntries := executors.ResolveCDAPaths(documentMap, "sectionsByKey."+rule.SectionKey+".entries[*]", false)
		if rule.FlattenOrganizers {
			allEntries = flattenOrganizerEntries(allEntries)
		}
		if claimed[rule.SectionKey] == nil {
			claimed[rule.SectionKey] = make(map[int]bool)
		}

		for idx, entryNode := range allEntries {
			if claimed[rule.SectionKey][idx] || !entryMatchesPredicate(entryNode, rule.EntryMatch) {
				continue
			}
			claimed[rule.SectionKey][idx] = true

			entryMap, ok := entryNode.(map[string]interface{})
			if !ok {
				continue
			}
			resource, extra, rowErrs := e.buildOneResource(entryMap, rule, idx)
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
// custodian), unlike section entries which can repeat.
func (e *DeclarativeEngine) BuildHeaderResource(
	documentMap map[string]interface{},
	rule MappingRule,
) (map[string]interface{}, []map[string]interface{}, []SectionError) {
	headerMap, ok := documentMap["header"].(map[string]interface{})
	if !ok || headerMap == nil {
		return nil, nil, nil
	}

	var entryMap map[string]interface{}
	switch rule.HeaderPath {
	case "authors":
		entryMap = firstAuthorWithPerson(headerMap)
	default:
		nodes := executors.ResolveCDAPaths(headerMap, rule.HeaderPath, false)
		if len(nodes) == 0 {
			return nil, nil, nil
		}
		entryMap, _ = nodes[0].(map[string]interface{})
	}
	if entryMap == nil {
		return nil, nil, nil
	}

	return e.buildOneResource(entryMap, rule, 0)
}

// firstAuthorWithPerson returns the first element of headerMap["authors"]
// whose assignedAuthor.assignedPerson carries at least one name — mirrors
// patient_mapper.go's MapAuthor loop ("return on first author with a
// person") exactly. An existence check ("has a person", not "person equals
// X"), which is why this is a small Go helper instead of an EntryMatch
// bracket-predicate: that grammar only supports equality.
func firstAuthorWithPerson(headerMap map[string]interface{}) map[string]interface{} {
	authors, _ := headerMap["authors"].([]interface{})
	for _, a := range authors {
		am, ok := a.(map[string]interface{})
		if !ok {
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
			return am
		}
	}
	return nil
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
) (map[string]interface{}, []map[string]interface{}, []SectionError) {
	resource := map[string]interface{}{"resourceType": rule.FHIRResource}
	var errs []SectionError
	var extra []map[string]interface{}

	for _, row := range rule.Fields {
		if err := e.applyRow(resource, entryMap, row, &extra); err != nil {
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

	// Only include non-empty resources (at least one field was set beyond
	// resourceType) — same convention as createFHIRResourceFromSection.
	if len(resource) <= 1 {
		return nil, extra, errs
	}
	if !resourceHasAllPaths(resource, rule.RequiredPaths) {
		return nil, extra, errs
	}
	return resource, extra, errs
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
func flattenOrganizerEntries(nodes []interface{}) []interface{} {
	var flat []interface{}
	for _, node := range nodes {
		nodeMap, ok := node.(map[string]interface{})
		if ok && nodeMap["entryType"] == "organizer" {
			if components, ok := nodeMap["components"].([]interface{}); ok && len(components) > 0 {
				flat = append(flat, components...)
				continue
			}
		}
		flat = append(flat, node)
	}
	return flat
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
// EmitAsResource row reached while applying this row (only possible via the
// row.CollectAll+Fields branch below — a non-Fields row never recurses).
// Most callers pass nil: only buildOneResource (the top of the recursion)
// needs to actually collect these.
func (e *DeclarativeEngine) applyRow(resource map[string]interface{}, entryMap map[string]interface{}, row MappingRow, extraOut *[]map[string]interface{}) error {
	if resourceHasAnyKey(resource, row.SkipIfResourceHasAnyOf) {
		return nil
	}
	scopedNodes := e.resolveScope(entryMap, row.Scope, row.ScopeFallbacks)
	if len(scopedNodes) == 0 {
		return nil
	}

	if row.CollectAll {
		if len(row.Fields) > 0 {
			return e.applyCollectAllWithFields(resource, scopedNodes, row, extraOut)
		}

		idx := 0
		for _, scoped := range scopedNodes {
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

// applyCollectAllWithFields implements MappingRow.Fields' "one sub-object
// per Scope match" semantics — see that field's doc comment for why this
// exists (two independent CollectAll rows can't be kept index-aligned;
// building one sub-object per match in one pass makes misalignment
// structurally impossible). Each child row is applied via the SAME applyRow
// every other row in this engine goes through — no special-cased recursion,
// just a fresh subObj as the "resource" and the matched node as the
// "entryMap" for that one recursive call.
func (e *DeclarativeEngine) applyCollectAllWithFields(resource map[string]interface{}, scopedNodes []interface{}, row MappingRow, extraOut *[]map[string]interface{}) error {
	idx := 0
	for _, scoped := range scopedNodes {
		scopedMap, ok := scoped.(map[string]interface{})
		if !ok {
			continue
		}
		subObj := map[string]interface{}{}
		for _, childRow := range row.Fields {
			if childRow.EmitAsResource != "" {
				// See MappingRow.EmitAsResource's doc comment: this child row
				// builds an INDEPENDENT resource (e.g. CareTeam's per-
				// participant Practitioner), not a value inside subObj —
				// subObj only gets a Reference to it.
				if sub, refVal := e.buildEmittedSubResource(scopedMap, childRow, idx); sub != nil {
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
			_ = e.applyRow(subObj, scopedMap, childRow, extraOut)
		}
		if len(subObj) == 0 {
			continue
		}
		setFHIRPath(resource, indexedPath(row.TargetPath, idx), subObj)
		idx++
	}
	return nil
}

// buildEmittedSubResource builds a brand new, independent resource from
// row.Fields against scopedMap — see MappingRow.EmitAsResource's doc
// comment. Returns (nil, nil) when nothing usable was built, the same
// "len(resource) <= 1" convention buildOneResource uses for top-level
// resources. idx (the position of scopedMap within the enclosing
// CollectAll loop) makes the synthesized id unique across sibling matches
// within one entry — e.g. "practitioner-1", "practitioner-2" for a
// two-member care team. Resource ids are not final/global at this layer
// (no section ported so far treats them as such — see the BP-panel
// synthesis precedent); only uniqueness within this one build matters.
func (e *DeclarativeEngine) buildEmittedSubResource(scopedMap map[string]interface{}, row MappingRow, idx int) (map[string]interface{}, map[string]interface{}) {
	sub := map[string]interface{}{"resourceType": row.EmitAsResource}
	for _, childRow := range row.Fields {
		_ = e.applyRow(sub, scopedMap, childRow, nil)
	}
	if len(sub) <= 1 {
		return nil, nil
	}
	id := fmt.Sprintf("%s-%d", strings.ToLower(row.EmitAsResource), idx+1)
	sub["id"] = id
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
