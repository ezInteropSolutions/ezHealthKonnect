// services/cda_fhir/document_mapper.go
// MapDocument — typed-struct-driven CDA→FHIR conversion (Sprint D).
//
// Unlike Map() which reads from a map[string]interface{} produced by the legacy
// CDA header parser, MapDocument() operates entirely on the *CDADocument typed
// struct built by cda/document.CDAParser. No XPath strings; no etree calls.
//
// Section dispatch table maps each C-CDA section key (from ccda_2_1.json) to
// its dedicated typed mapper function in services/cda_fhir/mappers.

package cdafhir

import (
	"context"
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
	"ezhealthkonnect/services/cda_fhir/mappers"
)

// sectionMapFn is the common signature for typed section mapper functions.
// patientRef is "Patient/patient-1" (or "" when patient could not be extracted).
type sectionMapFn func(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{}

// typedSectionDispatchers maps each C-CDA section key to its typed mapper.
// Keys match the sectionKey values resolved by CDASchemaLoader from ccda_2_1.json.
var typedSectionDispatchers = map[string]sectionMapFn{
	// Allergy & Intolerance
	"allergiesAndIntolerances": mappers.MapAllergies,

	// Problems / Health Concerns. Same mapper, different Condition.category
	// per US Core's us-core-condition-problems-health-concerns profile —
	// see conditionCategoryCC in condition_mapper.go for the spec citation.
	"problems": func(e []cdadocument.CDAEntry, p string) []map[string]interface{} {
		return mappers.MapConditions(e, p, "problem-list-item")
	},
	"healthConcerns": func(e []cdadocument.CDAEntry, p string) []map[string]interface{} {
		return mappers.MapConditions(e, p, "health-concern")
	},

	// Medications
	"medications": mappers.MapMedications,

	// Vital Signs
	"vitalSigns": func(e []cdadocument.CDAEntry, p string) []map[string]interface{} {
		return mappers.MapObservations(e, p, "vital-signs")
	},

	// Laboratory Results
	"results":    func(e []cdadocument.CDAEntry, p string) []map[string]interface{} { return mappers.MapObservations(e, p, "laboratory") },
	"labResults": func(e []cdadocument.CDAEntry, p string) []map[string]interface{} { return mappers.MapObservations(e, p, "laboratory") },

	// Social / Functional / Mental History
	"socialHistory": func(e []cdadocument.CDAEntry, p string) []map[string]interface{} {
		return mappers.MapObservations(e, p, "social-history")
	},
	"functionalStatus": func(e []cdadocument.CDAEntry, p string) []map[string]interface{} {
		return mappers.MapObservations(e, p, "functional-status")
	},
	"mentalStatus": func(e []cdadocument.CDAEntry, p string) []map[string]interface{} {
		return mappers.MapObservations(e, p, "cognitive-status")
	},

	// Encounters
	"encounters": mappers.MapEncounters,

	// Procedures
	"procedures": mappers.MapProcedures,

	// Immunizations
	"immunizations": mappers.MapImmunizations,

	// Care Team. "careTeam" is the schema/templateId-resolved key (see
	// cda/schemas/ccda_2_1.json); "careTeams" is what title-fallback produces
	// for documents titled "Care Teams" (plural) that carry no templateId match
	// — kept as an alias so both resolve correctly. See careteam_mapper.go.
	"careTeam":  mappers.MapCareTeam,
	"careTeams": mappers.MapCareTeam,

	// Goals
	"goals": mappers.MapGoals,

	// Care Plan / Plan of Care / Plan of Treatment. Per the C-CDA on FHIR IG,
	// each entry maps to whichever resource its own template+moodCode calls
	// for (ServiceRequest/MedicationRequest/Appointment/SupplyRequest/Goal) —
	// see plan_of_care_mapper.go. "planOfTreatment" is the schema/templateId-
	// resolved key actually produced for a "Plan of Treatment"-titled section;
	// "carePlan"/"planOfCare"/"assessmentAndPlan" are kept as aliases for the
	// other title/key variants this same section appears under.
	"carePlan":          mappers.MapPlanOfCare,
	"planOfCare":        mappers.MapPlanOfCare,
	"assessmentAndPlan": mappers.MapPlanOfCare,
	"planOfTreatment":   mappers.MapPlanOfCare,

	// Family History
	"familyHistory": mappers.MapFamilyHistory,

	// Payers / Coverage. "payersInsurance" is the schema/templateId-resolved
	// key (cda/schemas/ccda_2_1.json); "payors" kept as a defensive alias.
	"payersInsurance": mappers.MapCoverage,
	"payors":          mappers.MapCoverage,

	// Medical Equipment
	"medicalEquipment": mappers.MapDeviceUseStatements,
}

// DispatchedSectionKeys returns the section keys that have a typed FHIR mapper
// registered above. Section resolution (cda/document/section_parser.go) always
// prefers templateId or LOINC code over title text — those are a separate,
// independently-maintained lookup from this dispatch table, so a key typo here
// silently drops an entire section even when it resolved correctly. Exposed so
// tests can verify every dispatch key actually corresponds to a real schema
// section key or a documented title-fallback alias (see
// cda_fhir_dispatch_keys_test.go).
func DispatchedSectionKeys() []string {
	keys := make([]string, 0, len(typedSectionDispatchers))
	for k := range typedSectionDispatchers {
		keys = append(keys, k)
	}
	return keys
}

// MapDocument converts a typed *CDADocument to a FHIR R4 Bundle.
// This is the Sprint D primary path — every field is read from typed Go structs;
// no XPath expressions, no etree, no map[string]interface{} extraction.
//
// The returned *MapOutput is identical in shape to Map()'s output, so all
// downstream pipeline steps (validators, narrative generators, delivery connectors)
// work unchanged with either path.
func (m *GenericCDAFHIRMapper) MapDocument(
	ctx context.Context,
	doc *cdadocument.CDADocument,
	config CDAToFHIRConfig,
) (*MapOutput, error) {
	if doc == nil {
		return nil, fmt.Errorf("cda_fhir: MapDocument: nil CDADocument")
	}

	start := time.Now()

	// Apply config defaults (same defaults as Map()).
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
	if config.DocType == "" {
		config.DocType = "CCD"
	}

	pr := ProcessingResult{
		DocumentType: config.DocType,
		CCDAVersion:  config.CCDAVersion,
	}

	// Document identifier for MappingLog — prefer extension (the actual value),
	// fall back to root OID, then empty string.
	docID := doc.Header.DocumentId.Extension
	if docID == "" {
		docID = doc.Header.DocumentId.Root
	}
	logBuilder := mappinglog.NewLogBuilder(docID)

	var (
		allResources []map[string]interface{}
		wg           sync.WaitGroup

		sectionErrors []SectionError
		failedSects   []string
		successSects  []string
	)

	// ── Header resources (patient, author, custodian) ────────────────────────
	patientRef := ""
	if patientResource := mappers.MapPatient(doc.Header.Patient); patientResource != nil {
		patientRef = "Patient/patient-1"
		if config.ProfileMode != "base" {
			m.profileBuilder.InjectProfile(patientResource, "")
		}
		allResources = append(allResources, patientResource)
	}
	if authorResource := mappers.MapAuthor(doc.Header.Authors); authorResource != nil {
		if config.ProfileMode != "base" {
			m.profileBuilder.InjectProfile(authorResource, "")
		}
		allResources = append(allResources, authorResource)
	}
	if custodianResource := mappers.MapCustodian(doc.Header.Custodian); custodianResource != nil {
		allResources = append(allResources, custodianResource)
	}

	// ── Section resources (parallel, section-failure isolated) ───────────────
	// Eligible section keys are collected and sorted first, then each goroutine
	// is given its own pre-allocated result slot. This makes bundle output
	// deterministic across runs (Go map iteration order is randomized) and avoids
	// a mutex around shared aggregation state, since no two goroutines ever write
	// to the same slice index.
	var sectionKeys []string
	for sectionKey, section := range doc.SectionsByKey {
		if section == nil || len(section.Entries) == 0 {
			continue
		}
		if !m.isSectionEnabled(sectionKey, config.EnabledSections) {
			continue
		}
		if _, ok := typedSectionDispatchers[sectionKey]; !ok {
			continue // no typed mapper for this section — skip silently
		}
		sectionKeys = append(sectionKeys, sectionKey)
	}
	sort.Strings(sectionKeys)

	sectionResults := make([][]map[string]interface{}, len(sectionKeys))
	sectionErrs := make([][]SectionError, len(sectionKeys))
	sectionLogs := make([]mappinglog.SectionLog, len(sectionKeys))

	mappingStart := time.Now()
	for idx, sectionKey := range sectionKeys {
		section := doc.SectionsByKey[sectionKey]
		fn := typedSectionDispatchers[sectionKey]

		wg.Add(1)
		go func(i int, sk string, sec *cdadocument.CDASection, fn sectionMapFn) {
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
				// Call the mapper once per entry (instead of once for the whole
				// section) so we can record which entry produced which resource(s)
				// for the Mapping Log's entry-level drill-down. Safe because no
				// typed mapper does cross-entry lookups within a single call — each
				// entry's CDAEntry (including its own nested Components) is processed
				// independently; the only cross-resource merging (BP panel synthesis,
				// dedup) happens later in the assembly layer, over the flattened
				// allResources list, not here.
				for entryIdx, entry := range sec.Entries {
					entryResources := fn([]cdadocument.CDAEntry{entry}, patientRef)
					resources = append(resources, entryResources...)

					trace := mappinglog.EntryTrace{
						EntryIndex:  entryIdx,
						EntryType:   entry.EntryType,
						Code:        entry.Code.Code,
						CodeSystem:  entry.Code.CodeSystem,
						DisplayName: entry.Code.DisplayName,
						Resources:   entryResources,
					}
					if trace.DisplayName == "" {
						// Many CDA sections wrap the clinically meaningful content in an
						// outer act (e.g. an Allergy Concern Act coded "CONC", no display
						// name) with the real substance/observation nested in an
						// entryRelationship. The mapper already unwraps that into the
						// resource's own "code" field, so fall back to reading it from
						// there instead of showing an uninformative wrapper code.
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
		}(idx, sectionKey, section, fn)
	}
	wg.Wait()
	mappingMs := time.Since(mappingStart).Milliseconds()

	// Serial merge of section results (build allResources before renumbering IDs
	// and resolving entry traces below — both depend on final resource state).
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
	// Multiple CDA sections can dispatch to the same typed mapper (e.g. "problems"
	// and "healthConcerns" both → MapConditions), and each mapper numbers its own
	// output starting at 1. Without this pass, resources from different sections
	// can collide on the same id (two things both named "condition-1").
	// Patient/Practitioner/Organization are stable header resources — left untouched.
	// NOTE: _cdaIds is NOT stripped here; the assembly layer reads it below.
	idCounters := make(map[string]int)
	for _, r := range allResources {
		rt := strField(r, "resourceType")
		switch rt {
		case "", "Patient", "Practitioner", "Organization":
			continue
		}
		idCounters[rt]++
		r["id"] = fmt.Sprintf("%s-%d", resourceIDPrefix(rt), idCounters[rt])
	}

	// Resolve each entry trace's resource refs now that IDs are final (the
	// resource maps were mutated in place above), then merge section logs.
	for i := range sectionLogs {
		for j := range sectionLogs[i].Entries {
			sectionLogs[i].Entries[j].ResolveRefs()
		}
	}
	for _, sl := range sectionLogs {
		logBuilder.AddSection(sl)
	}

	// ── Assembly layer ────────────────────────────────────────────────────────
	// Runs serially after all section goroutines have completed and IDs are stable.
	// DeduplicationRule populates dedupRedirects; BPPanelSynthesisRule appends new
	// resources to assemblyCtx.Resources beyond the original slice boundary.
	assemblyStart := time.Now()
	var dedupRedirects map[string]string

	if !config.Assembly.Disabled && len(allResources) > 0 {
		originalCount := len(allResources)

		engine := assembly.NewDefaultRuleEngine()
		engine.Register(rules.NewDeduplicationRule())
		engine.Register(rules.NewBPPanelSynthesisRule())

		assemblyCtx := &assembly.AssemblyContext{
			// Pass a copy of the slice header so rules can append without modifying
			// allResources. The backing array is shared for read access; we reconcile
			// by reading assemblyCtx.Resources after Run().
			Resources:      append([]map[string]interface{}(nil), allResources...),
			DedupRedirects: make(map[string]string),
			Removed:        make(map[string]bool),
			Log:            logBuilder,
			Config:         config.Assembly,
		}

		if err := engine.Run(assemblyCtx); err != nil {
			log.Printf("  ⚠️  [cda.to_fhir/assembly] %v", err)
		}

		dedupRedirects = assemblyCtx.DedupRedirects

		// Filter duplicates out of allResources.
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

		// Collect synthesized resources appended by rules beyond the original slice.
		for i := originalCount; i < len(assemblyCtx.Resources); i++ {
			allResources = append(allResources, assemblyCtx.Resources[i])
		}
	}
	assemblyMs := time.Since(assemblyStart).Milliseconds()

	// ── Strip _cdaIds internal field ─────────────────────────────────────────
	// Must happen after assembly (which reads _cdaIds) and before FHIR emission.
	for _, r := range allResources {
		delete(r, "_cdaIds")
	}

	// ── US Core profiles + narratives ────────────────────────────────────────
	serialStart := time.Now()
	for i, r := range allResources {
		rt := strField(r, "resourceType")
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
		_ = rt // reserved for per-resource post-processing
	}
	serialMs := time.Since(serialStart).Milliseconds()

	// ── Assemble FHIR Bundle ─────────────────────────────────────────────────
	// urn:uuid: fullUrls + internal reference rewriting — shared with the
	// HL7→FHIR v3 pipeline; see fhir/r4/bundle_assembler.go. When dedup redirects
	// exist, AssembleEntriesWithRedirects collapses dedup rewriting and reference
	// rewriting into one O(n×depth) pass.
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

	log.Printf("  ✅ [cda.to_fhir/typed] %s → %d resources (%d ok, %d failed) mapping=%dms assembly=%dms serial=%dms total=%dms",
		config.DocType, len(allResources), len(successSects), len(failedSects),
		mappingMs, assemblyMs, serialMs, totalMs)

	if m.db != nil {
		go m.writeTransformAuditLog(ctx, config, pr, totalMs)
	}

	return &MapOutput{FHIRBundle: bundle, ProcessingResult: pr, MappingLog: mappingLogResult}, nil
}

// displayNameFromResources extracts a human-readable label from the first
// produced resource's "code" CodeableConcept (text, then first coding's
// display, then first coding's code). Used as a Mapping Log fallback when the
// source CDA entry itself carries no useful display name.
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
