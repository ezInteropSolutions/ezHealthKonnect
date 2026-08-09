// services/cda_fhir/declarative_document_mapper.go
//
// Phase 4 Slice B of the CDA→FHIR Declarative Mapping Engine (see
// architecture/CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md's Phase 4 section).
//
// DeclarativeMapDocument is the declarative analogue of document_mapper.go's
// MapDocument(): same orchestration shell (parallel per-section processing
// with panic isolation, per-entry Mapping Log tracing, cross-section
// resource-ID renumbering, the assembly layer, US Core profile injection,
// narrative generation, bundle assembly, ProcessingResult, async audit log),
// with ONLY the per-section dispatch itself replaced — a Go mapper function
// call becomes a DeclarativeEngine.BuildResourcesForRules call against the
// OOB *MappingRules() rule sets. Every block of orchestration logic below
// that ISN'T the dispatch itself is deliberately copied verbatim from
// MapDocument(), not reinvented, so the two stay comparable for Phase 4
// Slice C's shadow-mode diffing.
package cdafhir

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/services/cda_fhir/assembly"
	"ezhealthkonnect/services/cda_fhir/assembly/rules"
	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"
	"ezhealthkonnect/services/executors"
)

// declarativeSectionRuleGroups groups every OOB *MappingRules() function's
// output by its own SectionKey — the declarative analogue of
// document_mapper.go's typedSectionDispatchers, built generically (every
// rule already declares its own SectionKey) instead of hand-maintained.
// Header rules (HeaderPath set, SectionKey=="" — Author/Custodian) are
// excluded; they're built separately, the same way document_mapper.go calls
// MapAuthor/MapCustodian directly before this dispatch loop even starts.
// Built once at package init (mirrors typedSectionDispatchers' own eager-init
// style) since every *MappingRules() function is a pure, input-independent
// constructor — there is no per-document reason to rebuild this map.
var declarativeSectionRuleGroupsCache = buildDeclarativeSectionRuleGroups()

func buildDeclarativeSectionRuleGroups() map[string][]MappingRule {
	var all []MappingRule
	all = append(all, AllergyMappingRules()...)
	all = append(all, MedicationMappingRules()...)
	all = append(all, ProblemsMappingRules()...)
	all = append(all, HealthConcernsMappingRules()...)
	all = append(all, VitalSignsMappingRules()...)
	all = append(all, ResultsMappingRules()...)
	all = append(all, SocialHistoryMappingRules()...)
	all = append(all, FunctionalStatusMappingRules()...)
	all = append(all, MentalStatusMappingRules()...)
	all = append(all, ClinicalNoteMappingRules()...)
	all = append(all, ImmunizationMappingRules()...)
	all = append(all, EncounterMappingRules()...)
	all = append(all, ProcedureMappingRules()...)
	all = append(all, GoalMappingRules()...)
	all = append(all, PlanOfCareMappingRules()...)
	all = append(all, CareTeamMappingRules()...)
	all = append(all, CoverageMappingRules()...)
	all = append(all, FamilyMemberHistoryMappingRules()...)
	all = append(all, DeviceMappingRules()...)

	groups := make(map[string][]MappingRule)
	for _, rule := range all {
		if rule.SectionKey == "" {
			continue
		}
		groups[rule.SectionKey] = append(groups[rule.SectionKey], rule)
	}
	return groups
}

// DeclarativeMapDocument converts a typed *CDADocument to a FHIR R4 Bundle
// using the OOB declarative rules instead of document_mapper.go's hardcoded
// per-section Go mappers. The returned *MapOutput is identical in shape to
// MapDocument()'s, by design — Phase 4 Slice C's shadow-mode diffing depends
// on the two being directly comparable for the same input.
//
// Deliberately NOT included here (kept strictly at parity with MapDocument(),
// which itself doesn't build either): LegalAuthenticator (genuinely new
// functionality with no MapDocument() equivalent — wiring it in here would
// make every legalAuthenticator-bearing document a guaranteed shadow-mode
// "mismatch" for a reason that isn't a bug) and Composition/bundleType=
// "document" assembly (only Map(), the dormant legacy path, calls
// m.compositionMapper today — MapDocument() never does either). Both are
// reasonable candidates to add once Slice D makes this the only engine and
// parity-with-Go is no longer the bar.
func (m *GenericCDAFHIRMapper) DeclarativeMapDocument(
	ctx context.Context,
	doc *cdadocument.CDADocument,
	config CDAToFHIRConfig,
) (*MapOutput, error) {
	if doc == nil {
		return nil, fmt.Errorf("cda_fhir: DeclarativeMapDocument: nil CDADocument")
	}

	start := time.Now()

	if config.CCDAVersion == "" {
		config.CCDAVersion = "2.1"
	}
	if config.FHIRVersion == "" {
		config.FHIRVersion = "R4"
	}
	if config.BundleType == "" {
		config.BundleType = "collection"
	}
	if config.OnSectionFailure == "" {
		config.OnSectionFailure = "continue"
	}
	if config.DocType == "" || config.DocType == "auto" {
		// "auto" is the step builder's own default sentinel
		// (CdaToFhirStepBuilder._applyDefaultConfig: documentType="auto")
		// meaning "let the system detect it" — never a real document type to
		// key anything on. The wizard's own saveDelta() already normalizes
		// "auto"→"CCD" before computing the key interface_cda_mappings rows
		// are saved under; normalizing it here too keeps
		// loadCDAMappingOverrides' lookup key consistent with that, not just
		// the empty-string case.
		config.DocType = "CCD"
	}

	pr := ProcessingResult{
		DocumentType: config.DocType,
		CCDAVersion:  config.CCDAVersion,
	}

	docID := doc.Header.DocumentId.Extension
	if docID == "" {
		docID = doc.Header.DocumentId.Root
	}
	logBuilder := mappinglog.NewLogBuilder(docID)

	// ── Build documentMap once ───────────────────────────────────────────────
	// The one cost MapDocument() never pays (it reads the typed struct
	// directly) — named, not hidden, per the Phase 2 design note's own
	// "each rule call independently re-resolves..." performance caution.
	// Benchmark this explicitly (declarative_document_mapper_bench_test.go)
	// against the Phase 0 baseline table before this engine replaces
	// MapDocument() in production.
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("cda_fhir: DeclarativeMapDocument: marshalling document: %w", err)
	}
	var documentMap map[string]interface{}
	if err := json.Unmarshal(encoded, &documentMap); err != nil {
		return nil, fmt.Errorf("cda_fhir: DeclarativeMapDocument: unmarshalling document: %w", err)
	}

	engine := NewDeclarativeEngine()
	engine.CoverageTracker = config.CoverageTracker

	var (
		allResources []map[string]interface{}
		wg           sync.WaitGroup

		sectionErrors []SectionError
		failedSects   []string
		successSects  []string
	)

	// ── Header resources (patient, author, custodian) ────────────────────────
	patientRef := ""
	if patientResource, _, _ := engine.BuildHeaderResourceWithCoveragePrefix(documentMap, PatientMappingRules()[0], "header.patient"); patientResource != nil {
		patientResource["id"] = "patient-1"
		patientRef = "Patient/patient-1"
		if config.ProfileMode != "base" {
			m.profileBuilder.InjectProfile(patientResource, "")
			// Race/ethnicity/religion extensions — see PatientMappingRules'
			// own doc comment on why these aren't plain Fields rows.
			// patientResource isn't in allResources yet, so this can't go
			// through InjectProfiles' resource-slice loop; called directly
			// against the one patient header sub-object instead.
			if headerMap, ok := documentMap["header"].(map[string]interface{}); ok {
				if patientHeaderMap, ok := headerMap["patient"].(map[string]interface{}); ok {
					m.profileBuilder.InjectPatientExtensions(patientResource, patientHeaderMap)
				}
			}
		}
		allResources = append(allResources, patientResource)
	}
	// PatientRef is engine state (see DeclarativeEngine.PatientRef's doc
	// comment) — set once, here, before any section dispatch starts (and
	// before the parallel goroutines below launch) so every PatientRefPath
	// row across every section gets the real reference. Never written again
	// after this point, so concurrent reads from the section goroutines
	// below are race-free.
	engine.PatientRef = patientRef

	if authorResource, authorExtra, _ := engine.BuildHeaderResourceWithCoveragePrefix(documentMap, AuthorMappingRules()[0], "header.author"); authorResource != nil {
		// Go's MapAuthor numbers the id by the matched author's ORIGINAL
		// index in doc.Header.Authors (author-1, author-2, ...);
		// firstAuthorWithPersonIndexed's own index return value is used for
		// CDA Coverage Audit's coverage key (see that function), not for
		// this resource id. Hardcoding "author-1" is a narrow, deliberate
		// simplification — at most one Author resource is ever built per
		// document, so global uniqueness (the only property this id needs
		// to have, per the BP-panel-synthesis/CareTeam-Practitioner
		// precedent elsewhere in this engine) holds either way.
		authorResource["id"] = "author-1"
		if config.ProfileMode != "base" {
			m.profileBuilder.InjectProfile(authorResource, "")
		}
		allResources = append(allResources, authorResource)
		// authorExtra carries AuthorMappingRules' assignedEntityRoleRow
		// emission (PractitionerRole + a second, richer Practitioner +
		// Organization) -- previously discarded here via "_", silently
		// dropping the author's RepresentedOrganization the same way
		// patient_mapper.go's MapAuthor always did. DeduplicationRule
		// merges the richer Practitioner into authorResource's own
		// (sparser) one downstream; profile injection happens generically
		// below for every member of allResources, so these don't need
		// InjectProfile called on them here too.
		allResources = append(allResources, authorExtra...)
	}
	if custodianResource, _, _ := engine.BuildHeaderResourceWithCoveragePrefix(documentMap, CustodianMappingRules()[0], "header.custodian"); custodianResource != nil {
		custodianResource["id"] = "custodian-1"
		allResources = append(allResources, custodianResource)
	}
	encompassingLocationBuilt := false
	if locationResource, locationExtra, _ := engine.BuildHeaderResourceWithCoveragePrefix(documentMap, EncompassingEncounterLocationMappingRules()[0], "header.encompassingEncounter"); locationResource != nil {
		// componentOf/encompassingEncounter describes at most ONE facility for
		// the whole document -- same singular-per-document reasoning as
		// author-1/custodian-1, hardcoded here rather than going through the
		// id-renumbering loop below (which "Location" is excluded from for
		// exactly this reason).
		locationResource["id"] = "location-1"
		if config.ProfileMode != "base" {
			m.profileBuilder.InjectProfile(locationResource, "")
		}
		allResources = append(allResources, locationResource)
		allResources = append(allResources, locationExtra...)
		encompassingLocationBuilt = true
	}
	// Built but deliberately NOT appended to allResources yet -- whether
	// this becomes its own standalone Encounter or gets merged into a
	// matching in-section one (by shared CDA <id>, per the IG) can only be
	// decided once section dispatch has produced that in-section Encounter,
	// below. See EncompassingEncounterMappingRules' own doc comment for why.
	//
	// Shares the SAME "header.encompassingEncounter" coverage prefix as the
	// Location call above -- both calls' typed roots physically overlap
	// (Location resolves to a sub-node of Encounter), so they must share one
	// tracking unit or the same raw XML fields would be inventoried once but
	// recorded under two disjoint, non-matching prefixes. Each computes its
	// own basePath from its own HeaderPath via the same "CDAHeader"-rooted
	// translation (see BuildHeaderResourceWithCoveragePrefix), so nothing
	// double-tracks despite the shared prefix.
	encompassingEncounterCandidate, _, _ := engine.BuildHeaderResourceWithCoveragePrefix(documentMap, EncompassingEncounterMappingRules()[0], "header.encompassingEncounter")

	// ── Section resources (parallel, section-failure isolated) ───────────────
	var sectionKeys []string
	for sectionKey, section := range doc.SectionsByKey {
		if section == nil || len(section.Entries) == 0 {
			continue
		}
		if !m.isSectionEnabled(sectionKey, config.EnabledSections) {
			continue
		}
		if _, ok := declarativeSectionRuleGroupsCache[sectionKey]; !ok {
			continue // no declarative rule group for this section — skip silently
		}
		sectionKeys = append(sectionKeys, sectionKey)
	}
	sort.Strings(sectionKeys)

	// Per-interface field-level overrides (interface_cda_mappings), loaded
	// ONCE per document — the same row covers every section for this
	// interface+docType, so this is a single DB round-trip, not one per
	// section or per entry. config.InterfaceID=="" (the common case: no
	// interface context, or pure-OOB validation calls) skips the lookup
	// entirely via loadCDAMappingOverrides' own early return.
	overridesBySection := make(map[string][]CDAMappingOverride)
	if config.InterfaceID != "" {
		for _, ov := range m.loadCDAMappingOverrides(ctx, config.InterfaceID, config.DocType) {
			overridesBySection[ov.SectionKey] = append(overridesBySection[ov.SectionKey], ov)
		}
	}

	// Plan-of-Care "entryType=encounter" entries' target resource
	// (Appointment | Encounter) — resolved ONCE per document, same
	// precedence/DB-round-trip-sharing rationale as overridesBySection
	// above. PlanOfCareMappingRules()'s own Go literal still hardcodes
	// "Appointment" (unchanged, so it stays the literal every
	// migration/drift-guard test compares against) — the RUNTIME default
	// here is "Encounter" instead: pipeline step config
	// (config.PlanOfCareEncounterTarget, set by cda_to_fhir_executor.go)
	// wins if non-empty; else the interface's own
	// processing_rules.cda.planOfCareEncounterTarget default; else
	// "Encounter", the richer, better-evidenced mapping (see
	// encounterFields()'s own doc comment) — so even a document with no
	// interface/step context at all gets the improved default, not the
	// thinner Appointment rule.
	planOfCareEncounterTarget := config.PlanOfCareEncounterTarget
	if planOfCareEncounterTarget == "" && config.InterfaceID != "" {
		planOfCareEncounterTarget = m.loadPlanOfCareEncounterTargetDefault(ctx, config.InterfaceID)
	}
	if planOfCareEncounterTarget == "" {
		planOfCareEncounterTarget = "Encounter"
	}

	sectionResults := make([][]map[string]interface{}, len(sectionKeys))
	sectionErrs := make([][]SectionError, len(sectionKeys))
	sectionLogs := make([]mappinglog.SectionLog, len(sectionKeys))

	// When enabled (interface debug_logging/log_level=debug — see
	// services.ResolveInterfaceDebugLevel), every resource is tagged with its
	// originating CDA section + entry index so dedup/synthesis assembly rules
	// can record per-resource provenance (see assembly.ExtractLineage). Tags are
	// stripped before bundle emission (below) regardless of this flag.
	deepLineage := config.Assembly.DeepLineage

	mappingStart := time.Now()
	for idx, sectionKey := range sectionKeys {
		section := doc.SectionsByKey[sectionKey]
		sectionRules := declarativeSectionRuleGroupsCache[sectionKey]
		ovs := overridesBySection[sectionKey]
		swapToEncounter := planOfCareEncounterTarget == "Encounter" && isPlanOfCareSectionKey(sectionKey)
		if len(ovs) > 0 || swapToEncounter {
			// Clone before patching — declarativeSectionRuleGroupsCache is a
			// package-level singleton read concurrently by every section
			// goroutine across every in-flight DeclarativeMapDocument call;
			// mutating it in place would corrupt every OTHER interface's
			// "pure OOB" output too. Swap BEFORE applying field overrides:
			// ApplyFieldOverrides patches by TargetPath key and is a no-op
			// when ovs is empty, so running it after the swap is always
			// safe, never stale.
			sectionRules = CloneMappingRules(sectionRules)
			if swapToEncounter {
				sectionRules = applyPlanOfCareEncounterTarget(sectionRules)
			}
			if len(ovs) > 0 {
				ApplyFieldOverrides(sectionRules, ovs)
			}
		}

		// JSON-shaped entries array for this section, aligned by index with
		// section.Entries (both come from the same json.Marshal(doc) call
		// above, in the same order) — sliced per-entry below instead of
		// re-marshalled per-entry, so the one marshal cost paid above is
		// the only one this whole document pays, not one per entry.
		var sectionEntriesJSON []interface{}
		if sectionsByKeyNode, ok := documentMap["sectionsByKey"].(map[string]interface{}); ok {
			if secNode, ok := sectionsByKeyNode[sectionKey].(map[string]interface{}); ok {
				sectionEntriesJSON, _ = secNode["entries"].([]interface{})
			}
		}

		wg.Add(1)
		go func(i int, sk string, sec *cdadocument.CDASection, rules []MappingRule, entriesJSON []interface{}) {
			defer wg.Done()

			sb := mappinglog.NewSectionBuilder(sk, sec.Title, len(sec.Entries))

			var (
				resources []map[string]interface{}
				errs      []SectionError
			)

			func() {
				defer func() {
					if r := recover(); r != nil {
						errs = append(errs, SectionError{
							SectionKey: sk,
							Error:      fmt.Sprintf("panic: %v", r),
							Severity:   "error",
						})
						sb.AddError(fmt.Sprintf("panic: %v", r))
					}
				}()
				// Per-entry, not per-section, exactly like document_mapper.go's
				// own MapDocument loop — see that function's comment on WHY
				// (Mapping Log entry-level drill-down needs to know which
				// entry produced which resource(s); BuildResourcesForRules'
				// claimed-tracking is per-call, so calling it once per
				// single-entry slice is safe and mirrors Go's own per-entry
				// mapper calls exactly, including organizer-flattening
				// behavior for an entry that's itself an organizer).
				for entryIdx, entry := range sec.Entries {
					if entryIdx >= len(entriesJSON) {
						continue // documentMap/typed tree disagree in length -- shouldn't happen, skip defensively
					}
					perEntryDoc := map[string]interface{}{
						"sectionsByKey": map[string]interface{}{
							sk: map[string]interface{}{"entries": []interface{}{entriesJSON[entryIdx]}},
						},
					}

					// CDA Coverage Audit: this loop is the one place with both
					// sk (section key) and the REAL, section-relative entryIdx
					// together — BuildResourcesForRules below only ever sees
					// perEntryDoc's single-entry wrapper, so any index it
					// works with internally is relative to that wrapper (always
					// 0), never the real position in sec.Entries. Recorded
					// unconditionally, not only on success: "was this entry
					// considered by mapping logic" is the question this
					// feature answers, independent of whether the fields
					// inside happened to resolve to anything. See
					// CoverageTracker's doc comment (services/executors/
					// cda_coverage_tracker.go) and CDAEntryKey's.
					coveragePrefix := executors.CDAEntryKey(sk, entryIdx)
					engine.CoverageTracker.Record(coveragePrefix)

					// Element-granularity recording (Phase 1) reuses this SAME
					// key, not a second independently-computed one — see
					// BuildResourcesForRulesWithCoveragePrefix's own doc
					// comment (declarative_engine.go) for why plain
					// BuildResourcesForRules can't safely derive this itself.
					entryResources, entryErrs := engine.BuildResourcesForRulesWithCoveragePrefix(perEntryDoc, rules, coveragePrefix)
					if deepLineage {
						for _, res := range entryResources {
							res["_cdaSection"] = sk
							res["_cdaEntryIndex"] = entryIdx
						}
					}

					// "_warnings" is buildEmittedSubResource's marker for an
					// additionalValues drop on a NESTED EmitAsResource
					// resource (e.g. an Assessment Scale Supporting
					// Observation COMP child) -- that function has no
					// SectionKey/EntryIndex of its own to attach a
					// SectionError directly (see its own doc comment), but
					// this loop does, unconditionally (unlike
					// "_cdaSection"/"_cdaEntryIndex" above, gated behind
					// deepLineage). Read back into a real SectionError here
					// and strip immediately so it never reaches the entry
					// trace below or the shipped bundle.
					for _, res := range entryResources {
						msgs, ok := res["_warnings"].([]string)
						if !ok {
							continue
						}
						delete(res, "_warnings")
						for _, msg := range msgs {
							entryErrs = append(entryErrs, SectionError{
								SectionKey: sk,
								EntryIndex: entryIdx,
								FieldKey:   "value",
								Error:      msg,
								Severity:   "warning",
							})
						}
					}

					resources = append(resources, entryResources...)

					// entryErrs' own EntryIndex is relative to the synthetic
					// single-entry call above (always 0) -- overwrite with
					// the real index in this section's full entry list
					// before it's surfaced in ProcessingResult/the Mapping Log.
					// The "_warnings"-derived entries above already carry the
					// correct entryIdx, but this loop runs unconditionally
					// over every entryErrs element regardless of origin, so
					// it's a no-op for them, not a double-correction bug.
					for i := range entryErrs {
						entryErrs[i].EntryIndex = entryIdx
					}
					errs = append(errs, entryErrs...)

					// Mirror warning-severity entries into the Mapping Log
					// (object-storage-persisted for every real message, not
					// just Test Pipeline runs -- see SectionLog.Warnings'
					// doc comment) so a live production message's "more than
					// one <value>" notice is visible on the message detail
					// screen's Mapping Log tab, not only in a manual test.
					// Deliberately generic over every warning-severity
					// SectionError, not just additionalValues -- any
					// optional field's transform warning already produced
					// one of these before this feature existed, it just had
					// nowhere to surface.
					// entryWarnings mirrors the same warning-severity entries into
					// this entry's own EntryTrace (see that field's doc comment)
					// so the Mapping Log UI can show a warning directly on the
					// entry row it belongs to, not only as a section-level count.
					var entryWarnings []string
					for _, se := range entryErrs {
						if se.Severity != "warning" {
							continue
						}
						sb.AddWarning(fmt.Sprintf("entry %d, %s: %s", se.EntryIndex, se.FieldKey, se.Error))
						entryWarnings = append(entryWarnings, se.FieldKey+": "+se.Error)
					}

					trace := mappinglog.EntryTrace{
						EntryIndex:  entryIdx,
						EntryType:   entry.EntryType,
						Code:        entry.Code.Code,
						CodeSystem:  entry.Code.CodeSystem,
						DisplayName: entry.Code.DisplayName,
						Resources:   entryResources,
						Warnings:    entryWarnings,
					}
					if trace.DisplayName == "" {
						trace.DisplayName = displayNameFromResources(entryResources)
					}
					if len(entry.Id) > 0 && entry.Id[0].Extension != "" {
						trace.EntryID = entry.Id[0].Root + ":" + entry.Id[0].Extension
					}
					sb.AddEntry(trace)
				}
			}()

			sectionResults[i] = resources
			sectionErrs[i] = errs
			sectionLogs[i] = sb.Build(len(resources))
		}(idx, sectionKey, section, sectionRules, sectionEntriesJSON)
	}
	wg.Wait()
	mappingMs := time.Since(mappingStart).Milliseconds()

	// Serial merge of section results.
	//
	// failedSects/OnSectionFailure react to error-severity entries only --
	// NOT warning-severity ones (e.g. additionalValuesWarningMessages' "more
	// than one <value>" notice). A warning means the engine fully handled
	// the entry and made an explicit, intentional choice (keep the first
	// value); it is not a conformance failure, just something worth a
	// human's attention. Before this section-level distinction existed,
	// EVERY SectionError of either severity marked the whole section
	// "failed" -- harmless while warnings were rare (only an optional
	// field's transform genuinely erroring), but additionalValues turned
	// "more than one <value>" into a common, fully-expected occurrence
	// across real corpora (found while cross-checking three more real CCDs:
	// a section with nothing wrong beyond one informational warning was
	// landing in FailedSections, and would have had its resources silently
	// dropped under any interface configured with OnSectionFailure:
	// "fail-fast" -- exactly the outcome additionalValues warnings exist to
	// avoid).
	for idx, sectionKey := range sectionKeys {
		errs := sectionErrs[idx]
		resources := sectionResults[idx]
		hasError := false
		for _, se := range errs {
			if se.Severity != "warning" {
				hasError = true
				break
			}
		}
		if hasError {
			failedSects = append(failedSects, sectionKey)
			sectionErrors = append(sectionErrors, errs...)
			if config.OnSectionFailure != "fail-fast" {
				allResources = append(allResources, resources...)
			}
		} else {
			successSects = append(successSects, sectionKey)
			sectionErrors = append(sectionErrors, errs...)
			allResources = append(allResources, resources...)
		}
	}

	// componentOf/encompassingEncounter consolidation (HL7 C-CDA on FHIR IG:
	// "when the same encounter is referenced multiple times ... it should be
	// converted to a single FHIR resource") — match encompassingEncounterCandidate
	// against every in-section Encounter by shared CDA <id> (EncounterMappingRules'
	// own id[*] row, EmbedCDAIdentity:true), copy the candidate's
	// hospitalization across (the one field with no in-section source at
	// all), merge identifier, and fill period only if the in-section side
	// is somehow missing one. If no in-section Encounter shares the
	// candidate's id, it survives as its own standalone Encounter resource —
	// the IG's other explicit instruction ("this should also be converted
	// to a FHIR Encounter resource"). See EncompassingEncounterMappingRules'
	// doc comment for why this is an explicit field-level merge rather than
	// a DeduplicationRule registration (that rule's whole-resource
	// winner-take-all merge would discard hospitalization whenever the
	// richer in-section Encounter — which is every real case — wins).
	// Only proceed when the candidate carries SOMETHING beyond what the
	// Location-donor step above already covers generically (every Encounter
	// lacking its own location gets Location/location-1 regardless) --
	// status alone (always "unknown", since base CDA's EncompassingEncounter
	// class has no statusCode at all) doesn't count. Without this gate, a
	// componentOf/encompassingEncounter that carries ONLY a
	// location/healthCareFacility (real, common case: id and
	// effectiveTime/dischargeDispositionCode are frequently omitted even
	// when location is populated) would spawn a near-duplicate, no-new-
	// information standalone Encounter next to the real one whenever no
	// matching <id> happens to exist to consolidate against -- worse for
	// data quality than just leaving location-donation as the only effect,
	// which is exactly what this engine already did correctly before this
	// rule existed.
	candidateHasIdentifyingData := len(assembly.ExtractIdentityKeys(encompassingEncounterCandidate)) > 0
	if !candidateHasIdentifyingData && encompassingEncounterCandidate != nil {
		_, hasPeriod := encompassingEncounterCandidate["period"]
		_, hasHosp := encompassingEncounterCandidate["hospitalization"]
		candidateHasIdentifyingData = hasPeriod || hasHosp
	}
	if encompassingEncounterCandidate != nil && candidateHasIdentifyingData {
		candidateKeys := assembly.ExtractIdentityKeys(encompassingEncounterCandidate)
		var matched map[string]interface{}
		if len(candidateKeys) > 0 {
			for _, r := range allResources {
				if strField(r, "resourceType") != "Encounter" {
					continue
				}
				if identityKeysOverlap(candidateKeys, assembly.ExtractIdentityKeys(r)) {
					matched = r
					break
				}
			}
		}
		if matched != nil {
			if hosp, ok := encompassingEncounterCandidate["hospitalization"]; ok {
				if _, already := matched["hospitalization"]; !already {
					matched["hospitalization"] = hosp
				}
			}
			if _, already := matched["period"]; !already {
				if period, ok := encompassingEncounterCandidate["period"]; ok {
					matched["period"] = period
				}
			}
			if candidateIdent, ok := encompassingEncounterCandidate["identifier"].([]interface{}); ok {
				matchedIdent, _ := matched["identifier"].([]interface{})
				for _, ci := range candidateIdent {
					if !identifierPresent(matchedIdent, ci) {
						matchedIdent = append(matchedIdent, ci)
					}
				}
				matched["identifier"] = matchedIdent
			}
		} else {
			allResources = append(allResources, encompassingEncounterCandidate)
		}
	}

	// componentOf/encompassingEncounter describes ONE overarching visit for
	// the whole document, not a specific section entry — applying it
	// document-wide to every Encounter resource that doesn't already carry
	// its own (more specific) location is the correct interpretation, not a
	// per-rule reference EncounterMappingRules itself could resolve (it has
	// no visibility into a header resource built outside its own section
	// dispatch). Skips any Encounter whose own
	// "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC]"
	// row (EncounterMappingRules, see that row's doc comment) already
	// resolved a location — that section-entry-level data is more specific
	// and takes priority.
	if encompassingLocationBuilt {
		for _, r := range allResources {
			if strField(r, "resourceType") != "Encounter" {
				continue
			}
			if _, hasLocation := r["location"]; hasLocation {
				continue
			}
			r["location"] = []interface{}{
				map[string]interface{}{
					"location": map[string]interface{}{"reference": "Location/location-1"},
				},
			}
		}
	}

	// ── Resource ID uniqueness ────────────────────────────────────────────────
	// Resources emitted via MappingRow.EmitAsResource (marked "_emitted") have
	// already had their final, document-wide-unique id assigned at emission
	// time (DeclarativeEngine.nextEmittedID) and a reference to that exact id
	// already written into their parent resource — renumbering them here
	// would silently invalidate that reference, since this loop never walks
	// other resources to fix up references it changes. Seeding idCounters
	// past every already-used emitted id of each type (instead of just
	// skipping the type entirely, as Patient/Practitioner/Organization do)
	// keeps the two id spaces from colliding without requiring every
	// resource of an emission-using type to go through emission.
	idCounters := make(map[string]int)
	for _, r := range allResources {
		if emitted, _ := r["_emitted"].(bool); !emitted {
			continue
		}
		rt := strField(r, "resourceType")
		var n int
		if _, err := fmt.Sscanf(strField(r, "id"), resourceIDPrefix(rt)+"-%d", &n); err == nil && n > idCounters[rt] {
			idCounters[rt] = n
		}
	}
	for _, r := range allResources {
		rt := strField(r, "resourceType")
		switch rt {
		case "", "Patient", "Practitioner", "Organization", "Location":
			continue
		}
		if emitted, _ := r["_emitted"].(bool); emitted {
			continue
		}
		idCounters[rt]++
		r["id"] = fmt.Sprintf("%s-%d", resourceIDPrefix(rt), idCounters[rt])
	}

	// Condition.encounter back-reference -- EncounterMappingRules' nested-
	// diagnosis row (declarative_oob_rules.go) already writes
	// Encounter.diagnosis[].condition as a Reference(Condition); this is the
	// reverse link, set here (not as a conditionFields() row) because
	// conditionFields is shared with the standalone Problems/Health Concerns
	// sections, which have no Encounter to link to — only encounter-diagnosis
	// Conditions, identified by following the Encounter's own diagnosis[]
	// array forward, get this. Must run AFTER the renumbering loop above:
	// Encounter ids aren't final until then (Condition's own emitted id
	// already was, at build time, which is why diagnosis[].condition.reference
	// could be resolved earlier without this ordering constraint).
	conditionsByRef := make(map[string]map[string]interface{}, len(allResources))
	for _, r := range allResources {
		if strField(r, "resourceType") == "Condition" {
			conditionsByRef["Condition/"+strField(r, "id")] = r
		}
	}
	for _, r := range allResources {
		if strField(r, "resourceType") != "Encounter" {
			continue
		}
		diagnoses, _ := r["diagnosis"].([]interface{})
		if len(diagnoses) == 0 {
			continue
		}
		encounterRef := map[string]interface{}{"reference": "Encounter/" + strField(r, "id")}
		for _, d := range diagnoses {
			dMap, _ := d.(map[string]interface{})
			condRef, _ := dMap["condition"].(map[string]interface{})
			ref, _ := condRef["reference"].(string)
			if cond, ok := conditionsByRef[ref]; ok {
				if _, hasEncounter := cond["encounter"]; !hasEncounter {
					cond["encounter"] = encounterRef
				}
			}
		}
	}

	for i := range sectionLogs {
		for j := range sectionLogs[i].Entries {
			sectionLogs[i].Entries[j].ResolveRefs()
		}
	}
	for _, sl := range sectionLogs {
		logBuilder.AddSection(sl)
	}

	// ── Section-level narrative DocumentReferences ───────────────────────────
	// A second pass, SEQUENTIAL (only a handful of sections, each O(1)), over
	// EVERY section in the document that carries clinical content exclusively
	// in its narrative <text> block and has no structured <entry> elements.
	// These sections are silently skipped by the entry-dispatch loop above
	// (len(entries)==0 guard); this pass captures the content they DO carry
	// as FHIR DocumentReference resources instead of losing it entirely.
	// US Core v9.0.0 + Argonaut consensus: DocumentReference is the right FHIR
	// resource for "narrative broader than a specific clinical resource type".
	//
	// DefaultNarrativeSectionDefs (declarative_oob_rules.go) is consulted only
	// as a LOINC/display FALLBACK, not a gatekeeper — a section not present in
	// that registry (an unrecognized key, or a normally-structured section
	// like "medications" that happens to carry no entries in this document)
	// still gets a DocumentReference, built from the section's own LoincCode
	// and Title. buildNarrativeDocRef applies a final generic-Note fallback
	// (34109-9) if neither the section nor the registry has a LOINC code.
	for sectionKey, section := range doc.SectionsByKey {
		if section == nil || len(section.Entries) > 0 || strings.TrimSpace(section.NarrativeText) == "" {
			continue
		}
		if !m.isSectionEnabled(sectionKey, config.EnabledSections) {
			continue
		}
		def := DefaultNarrativeSectionDefs[sectionKey] // zero value for unregistered keys — buildNarrativeDocRef falls back further
		docRef := m.buildNarrativeDocRef(section, def, patientRef)
		if docRef != nil {
			allResources = append(allResources, docRef)
			successSects = append(successSects, sectionKey+"/narrative")
		}
	}

	// ── Assembly layer ────────────────────────────────────────────────────────
	assemblyStart := time.Now()
	var dedupRedirects map[string]string

	if !config.Assembly.Disabled && len(allResources) > 0 {
		originalCount := len(allResources)

		engineAssembly := assembly.NewDefaultRuleEngine()
		engineAssembly.Register(rules.NewDeduplicationRule())
		engineAssembly.Register(rules.NewBPPanelSynthesisRule())

		assemblyCtx := &assembly.AssemblyContext{
			Resources:      append([]map[string]interface{}(nil), allResources...),
			DedupRedirects: make(map[string]string),
			Removed:        make(map[string]bool),
			Log:            logBuilder,
			Config:         config.Assembly,
		}

		if err := engineAssembly.Run(assemblyCtx); err != nil {
			log.Printf("  ⚠️  [cda.to_fhir/declarative-assembly] %v", err)
		}

		dedupRedirects = assemblyCtx.DedupRedirects

		if len(assemblyCtx.Removed) > 0 {
			filtered := make([]map[string]interface{}, 0, len(allResources))
			for _, r := range allResources {
				key := strField(r, "resourceType") + "/" + strField(r, "id")
				if !assemblyCtx.Removed[key] {
					filtered = append(filtered, r)
				}
			}
			allResources = filtered
		}

		for i := originalCount; i < len(assemblyCtx.Resources); i++ {
			allResources = append(allResources, assemblyCtx.Resources[i])
		}
	}
	assemblyMs := time.Since(assemblyStart).Milliseconds()

	// ── Strip internal fields ────────────────────────────────────────────────
	// "_emitOnly" is a CollectAll+Fields vehicle row's own sub-object output
	// (see CareTeamMappingRules' component-performer-enrichment row for the
	// first use) -- it exists only to reach EmitAsResource processing, never
	// to carry a real FHIR value, so it's stripped the same as "_cdaIds"/
	// "_emitted".
	for _, r := range allResources {
		delete(r, "_cdaIds")
		delete(r, "_emitted")
		delete(r, "_emitOnly")
		delete(r, "_cdaSection")
		delete(r, "_cdaEntryIndex")
	}

	// ── US Core profiles + narratives ────────────────────────────────────────
	serialStart := time.Now()
	for i, r := range allResources {
		if config.ProfileMode != "base" {
			m.profileBuilder.InjectProfile(r, "")
		}
		narrative := m.narrativeGen.Generate(r)
		if narrative != "" {
			r["text"] = map[string]interface{}{
				"status": "generated",
				"div":    narrative,
			}
		}
		allResources[i] = r
	}
	serialMs := time.Since(serialStart).Milliseconds()

	// ── Assemble FHIR Bundle ─────────────────────────────────────────────────
	var entries []interface{}
	if len(dedupRedirects) > 0 {
		entries = r4.AssembleEntriesWithRedirects(allResources, dedupRedirects)
	} else {
		entries = r4.AssembleEntries(allResources)
	}

	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         config.BundleType,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"entry":        entries,
	}
	if config.ProfileMode != "base" {
		bundle["meta"] = map[string]interface{}{
			"source": "CDA/" + config.DocType,
		}
	}

	// ── ProcessingResult ─────────────────────────────────────────────────────
	pr.SuccessfulSections = successSects
	pr.FailedSections = failedSects
	pr.SectionErrors = sectionErrors
	pr.ResourcesProduced = len(allResources)
	pr.SectionsProcessed = len(successSects) + len(failedSects)
	pr.PartialSuccess = len(failedSects) > 0 && len(successSects) > 0

	totalMs := time.Since(start).Milliseconds()
	logBuilder.SetTimings(totalMs, mappingMs, assemblyMs, serialMs)
	mappingLogResult := logBuilder.Build(len(allResources))

	log.Printf("  ✅ [cda.to_fhir/declarative] %s → %d resources (%d ok, %d failed) mapping=%dms assembly=%dms serial=%dms total=%dms",
		config.DocType, len(allResources), len(successSects), len(failedSects),
		mappingMs, assemblyMs, serialMs, totalMs)

	if m.db != nil {
		go m.writeTransformAuditLog(ctx, config, pr, totalMs)
	}

	return &MapOutput{FHIRBundle: bundle, ProcessingResult: pr, MappingLog: mappingLogResult}, nil
}

// DeclarativeDispatchedSectionKeys returns the section keys that have a
// declarative rule group registered above. The declarative analogue of the
// now-deleted document_mapper.go's DispatchedSectionKeys() — exposed so
// cda_fhir_dispatch_keys_test.go can keep guarding against the exact bug
// class found in production (a section resolves correctly via templateId/
// LOINC, but the dispatch table has a typo'd or missing key, so it's
// silently dropped) for the declarative dispatch table too.
// DeclarativeSectionRules returns the OOB MappingRule slice for one
// sectionKey — a read-only accessor onto declarativeSectionRuleGroupsCache
// for controllers/cda_schema_controller.go's field-listing endpoint. The
// returned slice MUST NOT be mutated by callers — it aliases the package
// singleton; use CloneMappingRules first if mutation is needed.
func DeclarativeSectionRules(sectionKey string) []MappingRule {
	return declarativeSectionRuleGroupsCache[sectionKey]
}

func DeclarativeDispatchedSectionKeys() []string {
	keys := make([]string, 0, len(declarativeSectionRuleGroupsCache))
	for k := range declarativeSectionRuleGroupsCache {
		keys = append(keys, k)
	}
	return keys
}

// displayNameFromResources extracts a human-readable label from the first
// produced resource's "code" CodeableConcept (text, then first coding's
// display, then first coding's code). Used as a Mapping Log fallback when the
// source CDA entry itself carries no useful display name. Moved here from
// the now-deleted document_mapper.go — this is its only remaining caller.
func displayNameFromResources(resources []map[string]interface{}) string {
	for _, r := range resources {
		code, ok := r["code"].(map[string]interface{})
		if !ok {
			continue
		}
		if text := strField(code, "text"); text != "" {
			return text
		}
		codings, ok := code["coding"].([]interface{})
		if !ok || len(codings) == 0 {
			continue
		}
		c, ok := codings[0].(map[string]interface{})
		if !ok {
			continue
		}
		if display := strField(c, "display"); display != "" {
			return display
		}
		if codeVal := strField(c, "code"); codeVal != "" {
			return codeVal
		}
	}
	return ""
}

// identityKeysOverlap reports whether a and b share at least one
// assembly.IdentityKey (root+extension) — the same equality DeduplicationRule's
// registry uses internally, exposed here as a plain function since this
// match is an explicit field-level merge, not a dedup registration (see
// EncompassingEncounterMappingRules' doc comment for why).
func identityKeysOverlap(a, b []assembly.IdentityKey) bool {
	for _, ka := range a {
		for _, kb := range b {
			if ka == kb {
				return true
			}
		}
	}
	return false
}

// identifierPresent reports whether existing already contains an Identifier
// matching candidate's system+value -- a real, not just theoretical, case:
// encompassingEncounter and a matching in-section Encounter Activity sharing
// the same CDA <id> (the whole basis of the match this guards) produce
// IIToIdentifier output for the SAME root+extension on both sides, so a
// naive append would duplicate it verbatim in Encounter.identifier.
func identifierPresent(existing []interface{}, candidate interface{}) bool {
	cm, ok := candidate.(map[string]interface{})
	if !ok {
		return false
	}
	cSystem, _ := cm["system"].(string)
	cValue, _ := cm["value"].(string)
	for _, e := range existing {
		em, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		eSystem, _ := em["system"].(string)
		eValue, _ := em["value"].(string)
		if eSystem == cSystem && eValue == cValue {
			return true
		}
	}
	return false
}
