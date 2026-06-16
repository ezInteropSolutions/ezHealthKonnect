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

	// Problems / Health Concerns
	"problems":       mappers.MapConditions,
	"healthConcerns": mappers.MapConditions,

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

	// Apply config defaults (same defaults as Map())
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

	for idx, sectionKey := range sectionKeys {
		section := doc.SectionsByKey[sectionKey]
		fn := typedSectionDispatchers[sectionKey]

		wg.Add(1)
		go func(i int, sk string, sec *cdadocument.CDASection, fn sectionMapFn) {
			defer wg.Done()

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
					}
				}()
				resources = fn(sec.Entries, patientRef)
			}()

			sectionResults[i] = resources
			sectionErrs[i] = errs
		}(idx, sectionKey, section, fn)
	}
	wg.Wait()

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
	// output starting at 1 — without this pass, resources from different sections
	// can collide on the same id (two unrelated resources both named
	// "condition-1"). Patient/Practitioner/Organization are single, stable header
	// resources referenced elsewhere by their fixed literal id — left untouched.
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

	// ── US Core profiles + narratives ────────────────────────────────────────
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
		_ = rt // kept for future per-resource processing
	}

	// ── Assemble FHIR Bundle ─────────────────────────────────────────────────
	// urn:uuid: fullUrls + internal reference rewriting — shared with the
	// HL7→FHIR v3 pipeline; see fhir/r4/bundle_assembler.go. Resolves both the
	// "fullUrl must be absolute" and "fullUrl must be unique" bundle constraints,
	// and ensures every "Patient/patient-1"-style reference resolves correctly.
	entries := r4.AssembleEntries(allResources)

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

	durationMs := time.Since(start).Milliseconds()
	log.Printf("  ✅ [cda.to_fhir/typed] %s → %d resources (%d sections ok, %d failed) in %dms",
		config.DocType, len(allResources), len(successSects), len(failedSects), durationMs)

	if m.db != nil {
		go m.writeTransformAuditLog(ctx, config, pr, durationMs)
	}

	return &MapOutput{FHIRBundle: bundle, ProcessingResult: pr}, nil
}
