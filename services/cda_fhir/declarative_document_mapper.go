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
	"sync"
	"time"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/services/cda_fhir/assembly"
	"ezhealthkonnect/services/cda_fhir/assembly/rules"
	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"
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

	var (
		allResources []map[string]interface{}
		wg           sync.WaitGroup

		sectionErrors []SectionError
		failedSects   []string
		successSects  []string
	)

	// ── Header resources (patient, author, custodian) ────────────────────────
	patientRef := ""
	if patientResource, _, _ := engine.BuildHeaderResource(documentMap, PatientMappingRules()[0]); patientResource != nil {
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

	if authorResource, _, _ := engine.BuildHeaderResource(documentMap, AuthorMappingRules()[0]); authorResource != nil {
		// Go's MapAuthor numbers the id by the matched author's ORIGINAL
		// index in doc.Header.Authors (author-1, author-2, ...);
		// firstAuthorWithPerson only returns the matched map, not its
		// index. Hardcoding "author-1" is a narrow, deliberate
		// simplification — at most one Author resource is ever built per
		// document, so global uniqueness (the only property this id needs
		// to have, per the BP-panel-synthesis/CareTeam-Practitioner
		// precedent elsewhere in this engine) holds either way.
		authorResource["id"] = "author-1"
		if config.ProfileMode != "base" {
			m.profileBuilder.InjectProfile(authorResource, "")
		}
		allResources = append(allResources, authorResource)
	}
	if custodianResource, _, _ := engine.BuildHeaderResource(documentMap, CustodianMappingRules()[0]); custodianResource != nil {
		custodianResource["id"] = "custodian-1"
		allResources = append(allResources, custodianResource)
	}

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

	sectionResults := make([][]map[string]interface{}, len(sectionKeys))
	sectionErrs := make([][]SectionError, len(sectionKeys))
	sectionLogs := make([]mappinglog.SectionLog, len(sectionKeys))

	mappingStart := time.Now()
	for idx, sectionKey := range sectionKeys {
		section := doc.SectionsByKey[sectionKey]
		sectionRules := declarativeSectionRuleGroupsCache[sectionKey]
		if ovs := overridesBySection[sectionKey]; len(ovs) > 0 {
			// Clone before patching — declarativeSectionRuleGroupsCache is a
			// package-level singleton read concurrently by every section
			// goroutine across every in-flight DeclarativeMapDocument call;
			// mutating it in place would corrupt every OTHER interface's
			// "pure OOB" output too.
			sectionRules = CloneMappingRules(sectionRules)
			ApplyFieldOverrides(sectionRules, ovs)
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
					entryResources, entryErrs := engine.BuildResourcesForRules(perEntryDoc, rules)
					resources = append(resources, entryResources...)

					// entryErrs' own EntryIndex is relative to the synthetic
					// single-entry call above (always 0) -- overwrite with
					// the real index in this section's full entry list
					// before it's surfaced in ProcessingResult/the Mapping Log.
					for i := range entryErrs {
						entryErrs[i].EntryIndex = entryIdx
					}
					errs = append(errs, entryErrs...)

					trace := mappinglog.EntryTrace{
						EntryIndex:  entryIdx,
						EntryType:   entry.EntryType,
						Code:        entry.Code.Code,
						CodeSystem:  entry.Code.CodeSystem,
						DisplayName: entry.Code.DisplayName,
						Resources:   entryResources,
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
	for idx, sectionKey := range sectionKeys {
		errs := sectionErrs[idx]
		resources := sectionResults[idx]
		if len(errs) > 0 {
			failedSects = append(failedSects, sectionKey)
			sectionErrors = append(sectionErrors, errs...)
			if config.OnSectionFailure != "fail-fast" {
				allResources = append(allResources, resources...)
			}
		} else {
			successSects = append(successSects, sectionKey)
			allResources = append(allResources, resources...)
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
		case "", "Patient", "Practitioner", "Organization":
			continue
		}
		if emitted, _ := r["_emitted"].(bool); emitted {
			continue
		}
		idCounters[rt]++
		r["id"] = fmt.Sprintf("%s-%d", resourceIDPrefix(rt), idCounters[rt])
	}

	for i := range sectionLogs {
		for j := range sectionLogs[i].Entries {
			sectionLogs[i].Entries[j].ResolveRefs()
		}
	}
	for _, sl := range sectionLogs {
		logBuilder.AddSection(sl)
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
