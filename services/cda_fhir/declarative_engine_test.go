// services/cda_fhir/declarative_engine_test.go
//
// Phase 2 synthetic test suite — proves the worked examples named in the
// Phase 2 design note (architecture/CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md):
// allergy negation→verificationStatus (Scope + Condition together), the
// Medication Free-Text-Sig-vs-Instruction(V2) discriminator (per-row Scope
// deciding "which row applies" with no separate flag), RSON collect-all
// (the repeating primitive), and Medications' moodCode-driven
// MedicationRequest/Statement dispatch (BuildResourcesForRules' first-
// match-wins exclusivity). Two real fixtures already committed for Phase 0/1
// are reused here rather than re-invented (negation_and_frequency.xml,
// medication_sig_instruction.xml); the RSON and moodCode-dispatch examples
// have no real fixture yet (Phase 3's job, with real OOB content) so they're
// hand-built maps, exercising the engine mechanics in isolation.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	cdafhir "ezhealthkonnect/services/cda_fhir"

	"github.com/beevik/etree"
)

// loadDocumentMapFixture parses a CDA testdata fixture through the real
// production CDAParser, then JSON-round-trips the typed *CDADocument exactly
// as cda_parser_service.go does for ParsedJSON["document"] — the shape
// DeclarativeEngine actually consumes in production.
func loadDocumentMapFixture(t testing.TB, relPath string) map[string]interface{} {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	data, err := os.ReadFile(filepath.Join(repoRoot, "cda", "document", "testdata", relPath))
	if err != nil {
		t.Fatalf("reading testdata %s: %v", relPath, err)
	}
	raw := string(data)

	doc := etree.NewDocument()
	if err := doc.ReadFromString(raw); err != nil {
		t.Fatalf("parsing XML in %s: %v", relPath, err)
	}

	loader, err := cdaSchema.NewCDASchemaLoader(filepath.Join(repoRoot, "cda", "schemas"))
	if err != nil {
		t.Fatalf("loading CDA schema: %v", err)
	}
	typedDoc := cdadocument.NewCDAParser(loader).ParseDocument(doc.Root(), raw)

	encoded, err := json.Marshal(typedDoc)
	if err != nil {
		t.Fatalf("marshalling typed document for %s: %v", relPath, err)
	}
	var documentMap map[string]interface{}
	if err := json.Unmarshal(encoded, &documentMap); err != nil {
		t.Fatalf("unmarshalling typed document for %s: %v", relPath, err)
	}
	return documentMap
}

// ===============================================================
// Worked example 1: allergy negation → verificationStatus
// (Scope navigates to the nested SUBJ observation; Condition flips the
// literal "confirmed" default to "refuted" when negationInd=true)
// ===============================================================

func negationVerificationStatusRule() cdafhir.MappingRule {
	return cdafhir.MappingRule{
		SectionKey:   "allergiesAndIntolerances",
		FHIRResource: "AllergyIntolerance",
		Fields: []cdafhir.MappingRow{
			{
				Scope:        "entryRelationships[typeCode=SUBJ].entry",
				LiteralValue: "confirmed",
				Transform:    "allergy_verification_status_to_fhir",
				TargetPath:   "verificationStatus",
				Condition: &cdafhir.RowCondition{
					WhenPath:         "negationInd",
					Equals:           "true",
					ThenLiteralValue: "refuted",
				},
			},
		},
	}
}

func TestDeclarativeEngine_AllergyNegation_FlipsVerificationStatus(t *testing.T) {
	documentMap := loadDocumentMapFixture(t, "negation_and_frequency.xml")
	engine := cdafhir.NewDeclarativeEngine()

	resources, errs := engine.BuildResources(documentMap, negationVerificationStatusRule())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}

	vs, ok := resources[0]["verificationStatus"].(map[string]interface{})
	if !ok {
		t.Fatalf("verificationStatus missing or wrong shape: %v", resources[0]["verificationStatus"])
	}
	coding := vs["coding"].([]interface{})[0].(map[string]interface{})
	if coding["code"] != "refuted" {
		t.Errorf("verificationStatus.coding[0].code = %v, want \"refuted\" — this fixture's "+
			"nested SUBJ observation has negationInd=true", coding["code"])
	}
	if coding["display"] != "Refuted" {
		t.Errorf("verificationStatus.coding[0].display = %v, want \"Refuted\"", coding["display"])
	}
}

func TestDeclarativeEngine_AllergyNoNegation_StaysConfirmed(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode": "SUBJ",
								"entry":    map[string]interface{}{"negationInd": false},
							},
						},
					},
				},
			},
		},
	}
	engine := cdafhir.NewDeclarativeEngine()

	resources, errs := engine.BuildResources(documentMap, negationVerificationStatusRule())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}
	vs := resources[0]["verificationStatus"].(map[string]interface{})
	coding := vs["coding"].([]interface{})[0].(map[string]interface{})
	if coding["code"] != "confirmed" {
		t.Errorf("code = %v, want \"confirmed\" when negationInd is false (Condition must not fire)", coding["code"])
	}
}

// ===============================================================
// Worked example 2: Medication Free Text Sig vs Instruction(V2)
// (byte-identical {typeCode, templateId, text} shape; two separate rows,
// each gated by its own Scope — "which row applies" needs no extra flag)
// ===============================================================

const (
	freeTextSigTemplateID   = "2.16.840.1.113883.10.20.22.4.147"
	instructionV2TemplateID = "2.16.840.1.113883.10.20.22.4.20"
)

func sigInstructionRule() cdafhir.MappingRule {
	return cdafhir.MappingRule{
		SectionKey:   "medications",
		FHIRResource: "MedicationRequest",
		EntryMatch:   "moodCode=INT",
		Fields: []cdafhir.MappingRow{
			{
				Scope:      "entryRelationships[typeCode=COMP].entry[templateId=" + freeTextSigTemplateID + "]",
				SourcePath: "text",
				TargetPath: "dosage[0].text",
			},
			{
				Scope:      "entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=" + instructionV2TemplateID + "]",
				SourcePath: "text",
				TargetPath: "dosage[0].patientInstruction",
			},
		},
	}
}

func TestDeclarativeEngine_FreeTextSigVsInstructionV2(t *testing.T) {
	documentMap := loadDocumentMapFixture(t, "medication_sig_instruction.xml")
	engine := cdafhir.NewDeclarativeEngine()

	resources, errs := engine.BuildResources(documentMap, sigInstructionRule())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}

	dosages, ok := resources[0]["dosage"].([]interface{})
	if !ok || len(dosages) != 1 {
		t.Fatalf("dosage = %v, want a 1-element array", resources[0]["dosage"])
	}
	dosage := dosages[0].(map[string]interface{})
	if dosage["text"] != "Take one tablet by mouth every morning" {
		t.Errorf("dosage.text = %v, want the Free Text Sig narrative", dosage["text"])
	}
	if dosage["patientInstruction"] != "Take with food" {
		t.Errorf("dosage.patientInstruction = %v, want the Instruction(V2) narrative", dosage["patientInstruction"])
	}
}

func TestDeclarativeEngine_FreeTextSigVsInstructionV2_TemplateIDIsLoadBearing(t *testing.T) {
	documentMap := loadDocumentMapFixture(t, "medication_sig_instruction.xml")
	engine := cdafhir.NewDeclarativeEngine()

	// Deliberately wrong templateId: the COMP relationship's nested entry
	// carries .147 (Free Text Sig), not .20 (Instruction(V2)) — must not match.
	rule := cdafhir.MappingRule{
		SectionKey:   "medications",
		FHIRResource: "MedicationRequest",
		EntryMatch:   "moodCode=INT",
		Fields: []cdafhir.MappingRow{
			{
				Scope:      "entryRelationships[typeCode=COMP].entry[templateId=" + instructionV2TemplateID + "]",
				SourcePath: "text",
				TargetPath: "dosage[0].text",
			},
		},
	}

	resources, errs := engine.BuildResources(documentMap, rule)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 0 {
		t.Fatalf("got %d resources, want 0 — the COMP relationship's nested entry "+
			"does not carry the Instruction(V2) templateId", len(resources))
	}
}

// ===============================================================
// Worked example 3: RSON → reasonCode[] (CollectAll, not first-match)
// ===============================================================

func TestDeclarativeEngine_RSONCollectAll_ReasonCodeArray(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"medications": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"moodCode": "INT",
						"entryRelationships": []interface{}{
							map[string]interface{}{
								"typeCode": "RSON",
								"entry": map[string]interface{}{
									"code": map[string]interface{}{
										"code": "38341003", "displayName": "Hypertension",
										"codeSystem": "2.16.840.1.113883.6.96",
									},
								},
							},
							map[string]interface{}{
								"typeCode": "RSON",
								"entry": map[string]interface{}{
									"code": map[string]interface{}{
										"code": "44054006", "displayName": "Diabetes mellitus",
										"codeSystem": "2.16.840.1.113883.6.96",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.MappingRule{
		SectionKey:   "medications",
		FHIRResource: "MedicationRequest",
		EntryMatch:   "moodCode=INT",
		Fields: []cdafhir.MappingRow{
			{
				Scope:      "entryRelationships[typeCode=RSON].entry",
				SourcePath: "code",
				Transform:  "cda_code_to_codeable_concept",
				TargetPath: "reasonCode",
				CollectAll: true,
			},
		},
	}

	resources, errs := engine.BuildResources(documentMap, rule)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}

	reasonCodes, ok := resources[0]["reasonCode"].([]interface{})
	if !ok || len(reasonCodes) != 2 {
		t.Fatalf("reasonCode = %v, want a 2-element array (collect-all, not first-match)", resources[0]["reasonCode"])
	}
	firstCoding := reasonCodes[0].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
	if firstCoding["code"] != "38341003" || firstCoding["system"] != "http://snomed.info/sct" {
		t.Errorf("reasonCode[0] coding = %v", firstCoding)
	}
	secondCoding := reasonCodes[1].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
	if secondCoding["code"] != "44054006" {
		t.Errorf("reasonCode[1] coding = %v, want code 44054006", secondCoding)
	}
}

// ===============================================================
// CollectAll + Fields: index alignment across multiple sub-fields
// (Phase 3 addition — see MappingRow.Fields' doc comment for the bug this
// replaces: two independent CollectAll rows packing their own indices can
// drift out of alignment when one row's transform skips an item the other
// doesn't. This test proves the fix directly with the adversarial case:
// the MIDDLE of 3 matches is missing the optional sub-field.)
// ===============================================================

func TestDeclarativeEngine_CollectAllWithFields_StaysIndexAlignedWhenMiddleItemMissingOptionalField(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"entryRelationships": []interface{}{
							// Reaction 0: manifestation + severity.
							map[string]interface{}{
								"typeCode": "MFST",
								"entry": map[string]interface{}{
									"code": map[string]interface{}{"code": "R0"},
									"entryRelationships": []interface{}{
										map[string]interface{}{
											"typeCode": "SUBJ",
											"entry":    map[string]interface{}{"value": map[string]interface{}{"code": map[string]interface{}{"code": "SEV0"}}},
										},
									},
								},
							},
							// Reaction 1: manifestation ONLY -- no SUBJ relationship at all
							// (a real [0..1] absence, e.g. severity not documented).
							map[string]interface{}{
								"typeCode": "MFST",
								"entry": map[string]interface{}{
									"code": map[string]interface{}{"code": "R1"},
								},
							},
							// Reaction 2: manifestation + severity again.
							map[string]interface{}{
								"typeCode": "MFST",
								"entry": map[string]interface{}{
									"code": map[string]interface{}{"code": "R2"},
									"entryRelationships": []interface{}{
										map[string]interface{}{
											"typeCode": "SUBJ",
											"entry":    map[string]interface{}{"value": map[string]interface{}{"code": map[string]interface{}{"code": "SEV2"}}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.MappingRule{
		SectionKey:   "allergiesAndIntolerances",
		FHIRResource: "AllergyIntolerance",
		Fields: []cdafhir.MappingRow{
			{
				Scope:      "entryRelationships[typeCode=MFST]",
				CollectAll: true,
				TargetPath: "reaction",
				Fields: []cdafhir.MappingRow{
					{SourcePath: "entry.code.code", TargetPath: "manifestationCode"},
					{
						Scope:      "entry.entryRelationships[typeCode=SUBJ].entry",
						SourcePath: "value.code.code",
						TargetPath: "severityCode",
					},
				},
			},
		},
	}

	resources, errs := engine.BuildResources(documentMap, rule)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}

	reactions, ok := resources[0]["reaction"].([]interface{})
	if !ok || len(reactions) != 3 {
		t.Fatalf("reaction = %v, want a 3-element array (one per MFST match, including the one missing severity)", resources[0]["reaction"])
	}

	r0 := reactions[0].(map[string]interface{})
	if r0["manifestationCode"] != "R0" || r0["severityCode"] != "SEV0" {
		t.Errorf("reaction[0] = %v, want manifestationCode=R0 severityCode=SEV0", r0)
	}

	r1 := reactions[1].(map[string]interface{})
	if r1["manifestationCode"] != "R1" {
		t.Errorf("reaction[1].manifestationCode = %v, want R1", r1["manifestationCode"])
	}
	if _, has := r1["severityCode"]; has {
		t.Errorf("reaction[1] should have no severityCode key at all, got %v", r1["severityCode"])
	}

	r2 := reactions[2].(map[string]interface{})
	if r2["manifestationCode"] != "R2" || r2["severityCode"] != "SEV2" {
		t.Errorf("reaction[2] = %v, want manifestationCode=R2 severityCode=SEV2 -- if this drifted to "+
			"SEV0 or is missing, the old two-independent-CollectAll-rows bug is back", r2)
	}
}

// ===============================================================
// FlattenOrganizers: one resource per organizer Component, none for the
// organizer itself -- see MappingRule.FlattenOrganizers' doc comment.
// Phase 3 addition for Vital Signs/Results/Social History.
// ===============================================================

func TestDeclarativeEngine_FlattenOrganizers_ExpandsComponentsNotOrganizerItself(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"vitalSigns": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"entryType": "organizer",
						"code":      map[string]interface{}{"code": "46680005"},
						"components": []interface{}{
							map[string]interface{}{"entryType": "observation", "code": map[string]interface{}{"code": "8302-2"}},
							map[string]interface{}{"entryType": "observation", "code": map[string]interface{}{"code": "3141-9"}},
						},
					},
					// A non-organizer entry in the SAME section passes through unchanged.
					map[string]interface{}{"entryType": "observation", "code": map[string]interface{}{"code": "39156-5"}},
					// An organizer with zero components also passes through unchanged
					// (Go's len(Components)>0 gate).
					map[string]interface{}{"entryType": "organizer", "code": map[string]interface{}{"code": "EMPTY-ORGANIZER"}},
				},
			},
		},
	}

	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.MappingRule{
		SectionKey:        "vitalSigns",
		FHIRResource:      "Observation",
		FlattenOrganizers: true,
		Fields: []cdafhir.MappingRow{
			{SourcePath: "code.code", TargetPath: "code.coding[0].code"},
		},
	}

	resources, errs := engine.BuildResources(documentMap, rule)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 4 {
		t.Fatalf("got %d resources, want 4 (2 organizer components + 1 plain entry + 1 empty organizer itself), resources=%+v", len(resources), resources)
	}

	var codes []string
	for _, r := range resources {
		coding := firstCoding(t, r["code"])
		codes = append(codes, coding["code"].(string))
	}
	want := []string{"8302-2", "3141-9", "39156-5", "EMPTY-ORGANIZER"}
	for i, w := range want {
		if codes[i] != w {
			t.Errorf("resources[%d] code = %q, want %q (full: %v)", i, codes[i], w, codes)
		}
	}
}

// TestDeclarativeEngine_BuildResourcesForRules_FlattenOrganizers_KeepsClaimedIndicesAligned
// proves FlattenOrganizers' extension to BuildResourcesForRules (added
// porting CarePlan/Goal -- plan_of_care_mapper.go's flattenPlanOfCareEntries
// runs before its own moodCode/EntryType dispatch switch) doesn't break the
// claimed-by-index dispatch tracking: two rules sharing one SectionKey, both
// FlattenOrganizers=true, must see the SAME flattened entries at the SAME
// indices, or the second rule could silently re-process (or miss) an entry
// the first rule already claimed.
func TestDeclarativeEngine_BuildResourcesForRules_FlattenOrganizers_KeepsClaimedIndicesAligned(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"carePlan": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"entryType": "organizer",
						"components": []interface{}{
							map[string]interface{}{"entryType": "encounter", "moodCode": "INT", "code": map[string]interface{}{"code": "FIRST"}},
							map[string]interface{}{"entryType": "supply", "moodCode": "INT", "code": map[string]interface{}{"code": "SECOND"}},
						},
					},
				},
			},
		},
	}

	engine := cdafhir.NewDeclarativeEngine()
	rules := []cdafhir.MappingRule{
		{
			SectionKey:        "carePlan",
			FHIRResource:      "Appointment",
			EntryMatch:        "entryType=encounter",
			FlattenOrganizers: true,
			Fields:            []cdafhir.MappingRow{{SourcePath: "code.code", TargetPath: "serviceType[0].coding[0].code"}},
		},
		{
			SectionKey:        "carePlan",
			FHIRResource:      "SupplyRequest",
			EntryMatch:        "entryType=supply",
			FlattenOrganizers: true,
			Fields:            []cdafhir.MappingRow{{SourcePath: "code.code", TargetPath: "itemCodeableConcept.coding[0].code"}},
		},
	}

	resources, errs := engine.BuildResourcesForRules(documentMap, rules)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2 (the organizer's 2 flattened components, each claimed by its own rule), resources=%+v", len(resources), resources)
	}
	if resources[0]["resourceType"] != "Appointment" {
		t.Errorf("resources[0].resourceType = %v, want Appointment (the FIRST flattened component)", resources[0]["resourceType"])
	}
	if resources[1]["resourceType"] != "SupplyRequest" {
		t.Errorf("resources[1].resourceType = %v, want SupplyRequest (the SECOND flattened component)", resources[1]["resourceType"])
	}
}

// ===============================================================
// SkipIfResourceHasAnyOf: a row only fires when the resource doesn't
// already carry one of the listed keys -- the us-core-2 dataAbsentReason
// gate. Phase 3 addition for Vital Signs/Results/Social History.
// ===============================================================

func TestDeclarativeEngine_SkipIfResourceHasAnyOf_GatesDataAbsentReasonRow(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"vitalSigns": map[string]interface{}{
				"entries": []interface{}{
					// Has a real value -- dataAbsentReason should NOT be written.
					map[string]interface{}{"entryType": "observation", "value": map[string]interface{}{"type": "PQ", "quantity": map[string]interface{}{"value": "71.0", "unit": "[in_us]"}}},
					// No value at all -- dataAbsentReason SHOULD be written.
					map[string]interface{}{"entryType": "observation"},
				},
			},
		},
	}

	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.MappingRule{
		SectionKey:   "vitalSigns",
		FHIRResource: "Observation",
		Fields: []cdafhir.MappingRow{
			{Scope: "value[type=PQ]", SourcePath: "quantity", Transform: "cda_quantity_to_fhir", TargetPath: "valueQuantity"},
			{
				LiteralValue:           "unknown",
				Transform:              "observation_data_absent_reason_to_fhir",
				TargetPath:             "dataAbsentReason",
				SkipIfResourceHasAnyOf: []string{"valueQuantity"},
			},
		},
	}

	resources, errs := engine.BuildResources(documentMap, rule)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2", len(resources))
	}

	if _, has := resources[0]["dataAbsentReason"]; has {
		t.Errorf("resources[0] has a real valueQuantity, should NOT have dataAbsentReason, got %v", resources[0]["dataAbsentReason"])
	}
	if _, has := resources[0]["valueQuantity"]; !has {
		t.Error("resources[0] should have valueQuantity set")
	}

	if _, has := resources[1]["valueQuantity"]; has {
		t.Error("resources[1] has no value at all, should NOT have valueQuantity")
	}
	dar, ok := resources[1]["dataAbsentReason"].(map[string]interface{})
	if !ok {
		t.Fatalf("resources[1] should have dataAbsentReason set, got %v", resources[1]["dataAbsentReason"])
	}
	coding := firstCoding(t, dar)
	if coding["code"] != "unknown" {
		t.Errorf("dataAbsentReason coding = %v, want code=unknown", coding)
	}
}

// ===============================================================
// Typed-nil regression: a transform returning a nil map[string]interface{}
// (transforms.CDACodeToCodeableConcept does this for a fully nullFlavor
// code, e.g. PracticeFusion's fully-null Results organizer) must be treated
// as "skip this write", not written as a literal JSON null. Found via real
// corpus data while porting Vital Signs/Results/Social History -- a plain
// `transformed == nil` check does NOT catch this (the classic Go typed-nil-
// in-interface gotcha), and the bug was pre-existing across every section
// ported so far, not new to this one. Fixed once in isNilValue, used by
// every applyRow call site.
// ===============================================================

func TestDeclarativeEngine_TypedNilTransformResult_SkipsWriteNotLiteralNull(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"results": map[string]interface{}{
				"entries": []interface{}{
					// A fully nullFlavor code -- CDACodeToCodeableConcept
					// returns a nil map[string]interface{} for this shape.
					map[string]interface{}{"code": map[string]interface{}{"nullFlavor": "NI"}, "statusCode": "completed"},
				},
			},
		},
	}

	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.MappingRule{
		SectionKey:   "results",
		FHIRResource: "Observation",
		Fields: []cdafhir.MappingRow{
			{SourcePath: "code", Transform: "cda_code_to_codeable_concept", TargetPath: "code"},
			{SourcePath: "statusCode", Transform: "observation_status_to_fhir", TargetPath: "status"},
		},
	}

	resources, errs := engine.BuildResources(documentMap, rule)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}
	if v, has := resources[0]["code"]; has {
		t.Errorf("resources[0][\"code\"] = %#v (present, has=%v) -- a nullFlavor code should be OMITTED entirely, not written as null", v, has)
	}
	if resources[0]["status"] != "final" {
		t.Errorf("resources[0][\"status\"] = %v, want \"final\" -- unrelated rows must still apply normally", resources[0]["status"])
	}
}

// ===============================================================
// Worked example 4: Medications moodCode dispatch
// (BuildResourcesForRules' first-match-wins exclusivity)
// ===============================================================

func TestDeclarativeEngine_BuildResourcesForRules_MoodCodeDispatch(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"medications": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{"moodCode": "INT", "statusCode": "active"},
					map[string]interface{}{"moodCode": "EVN", "statusCode": "completed"},
				},
			},
		},
	}

	engine := cdafhir.NewDeclarativeEngine()
	rules := []cdafhir.MappingRule{
		{
			SectionKey:   "medications",
			FHIRResource: "MedicationRequest",
			EntryMatch:   "moodCode=INT",
			Fields:       []cdafhir.MappingRow{{SourcePath: "statusCode", TargetPath: "status"}},
		},
		{
			SectionKey:   "medications",
			FHIRResource: "MedicationStatement",
			EntryMatch:   "", // "else": claims whatever the INT rule didn't already claim
			Fields:       []cdafhir.MappingRow{{SourcePath: "statusCode", TargetPath: "status"}},
		},
	}

	resources, errs := engine.BuildResourcesForRules(documentMap, rules)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want exactly 2 (1 Request + 1 Statement, no double-claim)", len(resources))
	}

	var requestCount, statementCount int
	for _, r := range resources {
		switch r["resourceType"] {
		case "MedicationRequest":
			requestCount++
			if r["status"] != "active" {
				t.Errorf("MedicationRequest.status = %v, want \"active\"", r["status"])
			}
		case "MedicationStatement":
			statementCount++
			if r["status"] != "completed" {
				t.Errorf("MedicationStatement.status = %v, want \"completed\"", r["status"])
			}
		default:
			t.Errorf("unexpected resourceType %v", r["resourceType"])
		}
	}
	if requestCount != 1 || statementCount != 1 {
		t.Errorf("got %d MedicationRequest, %d MedicationStatement, want exactly 1 each "+
			"(the INT entry must not be claimed by both rules)", requestCount, statementCount)
	}
}

// ===============================================================
// Engine mechanics — Required/SHALL, unmatched Scope, terminology
// ===============================================================

func TestDeclarativeEngine_RequiredFieldEmpty_ReturnsErrorSeverity(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{}, // no "code" field at all
				},
			},
		},
	}
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.MappingRule{
		SectionKey:   "allergiesAndIntolerances",
		FHIRResource: "AllergyIntolerance",
		Fields: []cdafhir.MappingRow{
			{SourcePath: "code", TargetPath: "code", Required: true, Conformance: "SHALL"},
		},
	}

	resources, errs := engine.BuildResources(documentMap, rule)
	if len(resources) != 0 {
		t.Fatalf("got %d resources, want 0 (the only field was the missing required one)", len(resources))
	}
	if len(errs) != 1 || errs[0].Severity != "error" {
		t.Fatalf("errs = %+v, want exactly 1 error-severity entry", errs)
	}
}

func TestDeclarativeEngine_ScopeMatchesNothing_RowSilentlySkipped(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{"statusCode": "active"}, // no entryRelationships at all
				},
			},
		},
	}
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.MappingRule{
		SectionKey:   "allergiesAndIntolerances",
		FHIRResource: "AllergyIntolerance",
		Fields: []cdafhir.MappingRow{
			{Scope: "entryRelationships[typeCode=SUBJ].entry", SourcePath: "negationInd", TargetPath: "_unused"},
			{SourcePath: "statusCode", TargetPath: "status"},
		},
	}

	resources, errs := engine.BuildResources(documentMap, rule)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 || resources[0]["status"] != "active" {
		t.Fatalf("got %v, want exactly 1 resource with status=active "+
			"(the unmatched-Scope row contributes nothing, not an error)", resources)
	}
	if _, has := resources[0]["_unused"]; has {
		t.Errorf("the unmatched-Scope row should not have written anything")
	}
}

func TestDeclarativeEngine_ValueSetValidation_RejectsUnknownCode(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{"category": "not-a-real-category"},
				},
			},
		},
	}
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.MappingRule{
		SectionKey:   "allergiesAndIntolerances",
		FHIRResource: "AllergyIntolerance",
		Fields: []cdafhir.MappingRow{
			{
				SourcePath:  "category",
				TargetPath:  "category[0]",
				ValueSetURL: "http://hl7.org/fhir/ValueSet/allergy-intolerance-category",
				Required:    true,
			},
		},
	}

	_, errs := engine.BuildResources(documentMap, rule)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1 (terminology validation failure): %+v", len(errs), errs)
	}
	if errs[0].Severity != "error" {
		t.Errorf("severity = %v, want \"error\" (Required=true)", errs[0].Severity)
	}
}

func TestDeclarativeEngine_ValueSetValidation_AcceptsKnownCode(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{"category": "medication"},
				},
			},
		},
	}
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.MappingRule{
		SectionKey:   "allergiesAndIntolerances",
		FHIRResource: "AllergyIntolerance",
		Fields: []cdafhir.MappingRow{
			{
				SourcePath:  "category",
				TargetPath:  "category[0]",
				ValueSetURL: "http://hl7.org/fhir/ValueSet/allergy-intolerance-category",
			},
		},
	}

	resources, errs := engine.BuildResources(documentMap, rule)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 || resources[0]["category"].([]interface{})[0] != "medication" {
		t.Fatalf("got %v", resources[0])
	}
}

func TestDeclarativeEngine_FallbackPaths_UsesSecondWhenFirstEmpty(t *testing.T) {
	documentMap := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{"fallbackCode": "419199007"}, // "code" absent
				},
			},
		},
	}
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.MappingRule{
		SectionKey:   "allergiesAndIntolerances",
		FHIRResource: "AllergyIntolerance",
		Fields: []cdafhir.MappingRow{
			{
				SourcePath:    "code",
				FallbackPaths: []string{"fallbackCode"},
				TargetPath:    "_code",
			},
		},
	}

	resources, errs := engine.BuildResources(documentMap, rule)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 || resources[0]["_code"] != "419199007" {
		t.Fatalf("got %v, want the fallback value when the primary path is absent", resources)
	}
}

// ===============================================================
// DeclarativeTransformRegistry — unit coverage
// ===============================================================

func TestDeclarativeTransformRegistry_UnknownTransform_ReturnsError(t *testing.T) {
	reg := cdafhir.NewDeclarativeTransformRegistry()
	if _, err := reg.Apply("does_not_exist", "x", nil); err == nil {
		t.Fatal("expected an error for an unknown transform name")
	}
}

func TestDeclarativeTransformRegistry_EmptyName_Passthrough(t *testing.T) {
	reg := cdafhir.NewDeclarativeTransformRegistry()
	got, err := reg.Apply("", "raw-value", nil)
	if err != nil || got != "raw-value" {
		t.Fatalf("got (%v, %v), want (\"raw-value\", nil)", got, err)
	}
}

func TestDeclarativeTransformRegistry_CodeToCodeableConcept(t *testing.T) {
	reg := cdafhir.NewDeclarativeTransformRegistry()
	value := map[string]interface{}{
		"code": "38341003", "displayName": "Hypertension", "codeSystem": "2.16.840.1.113883.6.96",
	}
	got, err := reg.Apply("cda_code_to_codeable_concept", value, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cc, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("got %v, want a CodeableConcept map", got)
	}
	coding := cc["coding"].([]interface{})[0].(map[string]interface{})
	if coding["code"] != "38341003" || coding["system"] != "http://snomed.info/sct" {
		t.Errorf("coding = %v", coding)
	}
}

func TestDeclarativeTransformRegistry_StringDirect_ValueMap(t *testing.T) {
	reg := cdafhir.NewDeclarativeTransformRegistry()
	got, err := reg.Apply("string_direct", "M", map[string]string{"M": "male"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "male" {
		t.Errorf("got %v, want \"male\"", got)
	}
}

func TestDeclarativeTransformRegistry_HasTransform(t *testing.T) {
	reg := cdafhir.NewDeclarativeTransformRegistry()
	if !reg.HasTransform("cda_code_to_codeable_concept") {
		t.Error("expected cda_code_to_codeable_concept to be registered")
	}
	if reg.HasTransform("totally_made_up") {
		t.Error("did not expect totally_made_up to be registered")
	}
}
