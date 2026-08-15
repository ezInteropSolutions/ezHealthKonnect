package builder

import (
	"strings"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/cda/validator"
)

func loadTestSchema(t *testing.T) *cdaSchema.CDASchemaLoader {
	t.Helper()
	loader, err := cdaSchema.NewCDASchemaLoader("../schemas")
	if err != nil {
		t.Fatalf("failed to load CDA schema: %v", err)
	}
	return loader
}

// fullCanonicalDoc covers all 7 CCD SHALL sections plus all 6 CCD SHOULD
// sections, one entry each, using the exact field.Key/+Display/+System/+Unit
// convention generic_section_processor.go's extractValueByXPath produces.
func fullCanonicalDoc() map[string]interface{} {
	return map[string]interface{}{
		"header": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName":         "Jane",
				"lastName":          "Doe",
				"dateOfBirth":       "19800101",
				"sex":               "F",
				"sexDisplay":        "Female",
				"address":           map[string]interface{}{"street": "123 Main St", "city": "Springfield", "state": "IL", "postalCode": "62704"},
				"phone":             "5551234567",
				"preferredLanguage": "en",
				"ids":               []interface{}{map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "PAT-001"}},
			},
			"author": map[string]interface{}{"given": "John", "family": "Smith", "npi": "1234567890"},
			"informants": []interface{}{
				map[string]interface{}{"given": "Referring", "family": "Physician", "npi": "9876543210"},
			},
			"documentationOf": map[string]interface{}{
				"effectiveTimeLow": "20240101", "effectiveTimeHigh": "20240102",
				"performers": []interface{}{
					map[string]interface{}{"given": "John", "family": "Smith", "npi": "1234567890"},
				},
			},
			"encompassingEncounter": map[string]interface{}{
				"id": "ENC-001", "effectiveTimeLow": "20240101080000", "effectiveTimeHigh": "20240101120000",
				"dischargeDispositionCode": "01", "facilityName": "Main Campus", "facilityOrgName": "Test Health System",
			},
		},
		"sections": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "7980", "medicationAllergyCodeDisplay": "Penicillin",
					"medicationAllergyCodeSystem": "2.16.840.1.113883.6.88",
					"reaction":                    "247472004", "reactionDisplay": "Hives",
					"severity": "moderate", "status": "Active", "onsetDate": "20100101",
					"criticality": "high", "criticalityDisplay": "High risk",
				},
			}},
			"medications": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"drugCode": "197361", "drugCodeDisplay": "Lisinopril 10mg",
					"doseQuantity": "10", "doseQuantityUnit": "mg",
					"routeCode": "C38288", "routeCodeDisplay": "Oral",
					"status": "active", "startDate": "20200101",
				},
			}},
			"problems": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"conditionCode": "44054006", "conditionCodeDisplay": "Diabetes mellitus type 2",
					"conditionCodeSystem": "2.16.840.1.113883.6.96",
					"status":              "55561003", "onsetDate": "20190601",
					"ageAtOnset": "45", "ageAtOnsetUnit": "a",
				},
			}},
			"vitalSigns": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"vitalCode": "8480-6", "vitalCodeDisplay": "Systolic BP",
					"value": "120", "valueUnit": "mm[Hg]", "effectiveTime": "20240101120000",
				},
			}},
			"results": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"testCode": "2345-7", "testCodeDisplay": "Glucose",
					"resultValue": "95", "resultValueUnit": "mg/dL", "resultStatus": "completed",
					"effectiveTime": "20240101120000",
				},
			}},
			"immunizations": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"vaccineCode": "140", "vaccineCodeDisplay": "Influenza",
					"vaccineCodeSystem":  "2.16.840.1.113883.12.292",
					"administrationDate": "20231001", "status": "completed",
				},
			}},
			"socialHistory": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"observationCode": "72166-2", "observationCodeDisplay": "Smoking status",
					"smokingStatus": "8517006", "smokingStatusDisplay": "Former smoker",
					"effectiveTime": "20240101",
				},
			}},
			"encounters": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"encounterCode": "99213", "encounterCodeDisplay": "Office visit",
					"effectiveTime": "20240101", "performerGiven": "John", "performerFamily": "Smith",
				},
			}},
			"procedures": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"procedureCode": "80146002", "procedureCodeDisplay": "Appendectomy",
					"procedureCodeSystem": "2.16.840.1.113883.6.96",
					"effectiveTime":       "20230501", "status": "completed",
				},
			}},
			"functionalStatus": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"observationCode": "54522-8", "observationCodeDisplay": "Functional status",
					"value": "LA9938-9", "valueDisplay": "Independent", "effectiveTime": "20240101",
				},
			}},
			"familyHistory": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"relationship": "66839005", "relationshipDisplay": "Mother",
					"conditionCode": "38341003", "conditionCodeDisplay": "Hypertension",
					"conditionCodeSystem": "2.16.840.1.113883.6.96",
				},
			}},
			"assessmentAndPlan": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"activityCode": "384821006", "activityCodeDisplay": "Follow up",
					"effectiveTime": "20240101", "status": "active",
				},
			}},
			"payersInsurance": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"coverageType": "PPO", "coverageTypeDisplay": "Preferred Provider Organization",
					"payerName": "Acme Health Plan", "memberId": "MBR-001",
				},
			}},
			"planOfTreatment": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"activityCode": "182836005", "activityCodeDisplay": "Review of medication",
					"effectiveTime": "20240201", "status": "INT",
				},
			}},
			"advanceDirectives": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"directiveCode": "304251008", "directiveCodeDisplay": "Advance directive",
					"value": "425392003", "valueDisplay": "DNR",
				},
			}},
			"medicalEquipment": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"equipmentCode": "58938008", "equipmentCodeDisplay": "Wheelchair",
					"productName": "Standard Wheelchair",
				},
			}},
			"mentalStatus": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"observationCode": "454741000124104", "observationCodeDisplay": "Orientation",
					"value": "LA9933-0", "valueDisplay": "Oriented x3", "effectiveTime": "20240101",
				},
			}},
			"nutrition": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"observationCode": "75305-3", "observationCodeDisplay": "Nutritional status",
					"status": "248325000", "statusDisplay": "Well nourished", "effectiveTime": "20240101",
				},
			}},
		},
	}
}

func TestBuildDocument_ProducesWellFormedXML(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, "<ClinicalDocument") {
		t.Fatal("expected ClinicalDocument root element in output")
	}
	if !strings.Contains(xml, "Jane") || !strings.Contains(xml, "Doe") {
		t.Error("expected patient name in output")
	}
	if !strings.Contains(xml, "Test Health System") {
		t.Error("expected configured org name in custodian/author output")
	}
	if !strings.Contains(xml, "<informant>") || !strings.Contains(xml, "Referring") {
		t.Error("expected an <informant> element with the configured referring physician")
	}
	if !strings.Contains(xml, "<documentationOf>") || !strings.Contains(xml, `classCode="PCPR"`) {
		t.Error("expected a <documentationOf><serviceEvent classCode=\"PCPR\"> element")
	}
	if !strings.Contains(xml, "<componentOf>") || !strings.Contains(xml, "ENC-001") {
		t.Error("expected a <componentOf><encompassingEncounter> element with the configured encounter id")
	}
}

// TestBuildDocument_TemplateIdsIncludeExtension verifies that both the
// document-level and every section/entry-level templateId includes the
// correct @extension per the C-CDA 2.1 IG (2018 errata) — e.g.
// root="...22.1.2" extension="2015-08-01" for the CCD document itself,
// root="...22.4.16" extension="2014-06-09" for Medication Activity (a V2
// template) — not just @root, which was the pre-existing gap this test
// guards against regressing.
func TestBuildDocument_TemplateIdsIncludeExtension(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	cases := []struct {
		name, root, extension string
	}{
		{"US Realm Header", "2.16.840.1.113883.10.20.22.1.1", "2015-08-01"},
		{"CCD document", "2.16.840.1.113883.10.20.22.1.2", "2015-08-01"},
		{"Allergies section", "2.16.840.1.113883.10.20.22.2.6.1", "2015-08-01"},
		{"Allergy Concern Act", "2.16.840.1.113883.10.20.22.4.30", "2015-08-01"},
		{"Allergy-Intolerance Observation", "2.16.840.1.113883.10.20.22.4.7", "2014-06-09"},
		{"Medications section", "2.16.840.1.113883.10.20.22.2.1.1", "2014-06-09"},
		{"Medication Activity", "2.16.840.1.113883.10.20.22.4.16", "2014-06-09"},
		{"Medication Information (manufacturedProduct)", "2.16.840.1.113883.10.20.22.4.23", "2014-06-09"},
		{"Problem Observation", "2.16.840.1.113883.10.20.22.4.4", "2015-08-01"},
		// Plan of Treatment: real regression test for a pre-existing schema
		// bug — entryTemplateId used to point at Planned Encounter (.4.40)
		// instead of the Planned Observation (.4.44) the section's own
		// fields actually target (confirmed against the C-CDA 2.1 IG's own
		// Plan of Treatment Section chapter, Table 166).
		{"Planned Observation", "2.16.840.1.113883.10.20.22.4.44", "2014-06-09"},
		// Advance Directives: another real regression test for a pre-existing
		// schema bug — entryTemplateId used .4.61, which actually belongs to
		// Payers' Policy Activity (confirmed while reading Coverage Activity's
		// entry template), not Advance Directive Observation (.4.48, confirmed
		// directly against the Advance Directives Section chapter's own
		// "Contains:" reference, Table 65).
		{"Advance Directive Observation", "2.16.840.1.113883.10.20.22.4.48", "2015-08-01"},
		{"Payers section", "2.16.840.1.113883.10.20.22.2.18", "2015-08-01"},
		{"Coverage Activity", "2.16.840.1.113883.10.20.22.4.60", "2015-08-01"},
		{"Policy Activity", "2.16.840.1.113883.10.20.22.4.61", "2015-08-01"},
	}
	for _, c := range cases {
		want := `root="` + c.root + `" extension="` + c.extension + `"`
		if !strings.Contains(xml, want) {
			t.Errorf("%s: expected templateId %s in output, not found", c.name, want)
		}
	}

	// Structural templateIds for Allergy's Reaction/Severity/Status/
	// Criticality Observations and Problem's Age Observation have no
	// extension of their own (confirmed against their entry-template
	// chapters — "urn:oid:X" identifier form, not "urn:hl7ii:X:date") —
	// verify root alone lands correctly.
	noExtCases := []struct{ name, root string }{
		{"Reaction Observation", "2.16.840.1.113883.10.20.22.4.9"},
		{"Severity Observation", "2.16.840.1.113883.10.20.22.4.8"},
		{"Allergy Status Observation", "2.16.840.1.113883.10.20.22.4.28"},
		{"Criticality Observation", "2.16.840.1.113883.10.20.22.4.145"},
		{"Age Observation", "2.16.840.1.113883.10.20.22.4.31"},
		{"Problem Status", "2.16.840.1.113883.10.20.22.4.6"},
	}
	for _, c := range noExtCases {
		if !strings.Contains(xml, `root="`+c.root+`"`) {
			t.Errorf("%s: expected templateId root=%q in output, not found", c.name, c.root)
		}
	}

	// Plan of Treatment's Planned Observation is SHALL statusCode="active"
	// (CONF:1098-32032) — distinct from the generic "observation" tag
	// default of "completed" every other plain-observation section uses.
	if !strings.Contains(xml, `<code code="182836005"`) {
		t.Fatal("expected the Plan of Treatment entry's activityCode in output")
	}
	planIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.44"`)
	if planIdx == -1 {
		t.Fatal("expected Planned Observation templateId in output")
	}
	// statusCode is written BEFORE templateId in build order (tag boilerplate
	// runs first, then injectTemplateID) — so the entry's window has to start
	// at the enclosing <observation> tag, not mid-way through the templateId
	// attribute planIdx itself points at.
	entryStart := strings.LastIndex(xml[:planIdx], "<observation")
	if entryStart == -1 {
		t.Fatal("expected an enclosing <observation> before the Planned Observation templateId")
	}
	tail := xml[entryStart:]
	if end := strings.Index(tail, "</observation>"); end != -1 {
		tail = tail[:end]
	}
	if !strings.Contains(tail, `<statusCode code="active"`) {
		t.Errorf("expected Planned Observation's statusCode to be overridden to \"active\", got: %s", tail)
	}
}

// TestBuildDocument_PayersFixedCodeAndStatus locks in Coverage Activity's two
// EntryFixedCode/EntryStatusCodeOverride mechanisms (schema_types.go): its
// own <code> is never populated by any field (coverageType maps to the
// NESTED Policy Activity, one level deeper), and its statusCode default of
// "active" (the generic "act" tag boilerplate) is wrong for Coverage
// Activity specifically — SHALL be "completed" (CONF:1198-8875).
func TestBuildDocument_PayersFixedCodeAndStatus(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	covIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.60"`)
	if covIdx == -1 {
		t.Fatal("expected Coverage Activity templateId in output")
	}
	entryStart := strings.LastIndex(xml[:covIdx], "<act")
	if entryStart == -1 {
		t.Fatal("expected an enclosing <act> before the Coverage Activity templateId")
	}
	tail := xml[entryStart:]
	if end := strings.Index(tail, "</act>"); end != -1 {
		// The FIRST </act> closes the nested Policy Activity, not Coverage
		// Activity itself — find the outer close by counting past it.
		if next := strings.Index(tail[end+len("</act>"):], "</act>"); next != -1 {
			tail = tail[:end+len("</act>")+next]
		}
	}
	if !strings.Contains(tail, `code code="48768-6"`) {
		t.Errorf("expected Coverage Activity's fixed code=\"48768-6\", got: %s", tail)
	}
	if !strings.Contains(tail, `<statusCode code="completed"`) {
		t.Errorf("expected Coverage Activity's statusCode overridden to \"completed\", got: %s", tail)
	}
}

// TestBuildDocument_ProblemObservationFixedCode locks in ObsFixedCode
// (schema_types.go): C-CDA 2.1's Problem Observation (templateId .4.4) SHALL
// have exactly one <code> selected from the Problem Type value set (SNOMED
// CT 55607006 "Problem") on the OBSERVATION itself — distinct from the real
// diagnosis code a field write already puts on <value> one level deeper.
// Found via /api/cda/validate flagging "Entry.Code" missing on a real
// pipeline-built CCD even though the diagnosis <value> was fully populated.
func TestBuildDocument_ProblemObservationFixedCode(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	obsIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.4"`)
	if obsIdx == -1 {
		t.Fatal("expected Problem Observation templateId in output")
	}
	entryStart := strings.LastIndex(xml[:obsIdx], "<observation")
	if entryStart == -1 {
		t.Fatal("expected an enclosing <observation> before the Problem Observation templateId")
	}
	// The Problem Observation nests its own child <observation> (Problem
	// Status, templateId .4.6), so find the CLOSE tag matching THIS
	// observation's own open tag by tracking nesting depth, rather than
	// naively grabbing the first "</observation>" (which would be the
	// nested child's, truncating before this observation's own <code>).
	rest := xml[entryStart:]
	depth := 0
	tail := rest
	for pos := 0; pos < len(rest); {
		openIdx := strings.Index(rest[pos:], "<observation")
		closeIdx := strings.Index(rest[pos:], "</observation>")
		if closeIdx == -1 {
			break
		}
		if openIdx != -1 && openIdx < closeIdx {
			depth++
			pos += openIdx + len("<observation")
			continue
		}
		depth--
		pos += closeIdx + len("</observation>")
		if depth == 0 {
			tail = rest[:pos]
			break
		}
	}

	if !strings.Contains(tail, `code="55607006"`) || !strings.Contains(tail, `codeSystem="2.16.840.1.113883.6.96"`) {
		t.Errorf("expected Problem Observation's fixed code=\"55607006\" (SNOMED Problem), got: %s", tail)
	}
	// The real diagnosis code (from fullCanonicalDoc's conditionCode) must
	// still be present on <value> — the fixed code must not replace it.
	if !strings.Contains(tail, `<value xsi:type="CD" code="44054006"`) {
		t.Errorf("expected the real diagnosis code on <value> to still be present, got: %s", tail)
	}
}

func TestBuildDocument_OmitsOptionalHeaderElementsWhenAbsent(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	header := doc["header"].(map[string]interface{})
	delete(header, "informants")
	delete(header, "documentationOf")
	delete(header, "encompassingEncounter")

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if strings.Contains(xml, "<informant>") {
		t.Error("expected no <informant> element when header carries no informants data")
	}
	if strings.Contains(xml, "<componentOf>") {
		t.Error("expected no <componentOf> element when header carries no encompassingEncounter data")
	}
	// documentationOf is a genuine CCD SHALL (CONF:1198-8452) — unlike
	// informant/componentOf, it must still be present with a nullFlavor
	// default, not omitted, when the source has no real data.
	if !strings.Contains(xml, `<documentationOf>`) || !strings.Contains(xml, `classCode="PCPR"`) {
		t.Error("expected a default <documentationOf><serviceEvent classCode=\"PCPR\"> even with no source data (CCD SHALL)")
	}
	if !strings.Contains(xml, `nullFlavor="UNK"`) {
		t.Error("expected nullFlavor=\"UNK\" on the synthesized serviceEvent effectiveTime low/high")
	}
}

// carePlanCanonicalDoc supplies one entry each for Care Plan's 2 SHALL
// sections (healthConcerns, goals — both already registered/reused by CCD),
// using the exact field.Key/+Display convention fullCanonicalDoc() follows.
func carePlanCanonicalDoc() map[string]interface{} {
	return map[string]interface{}{
		"header": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName": "Jane", "lastName": "Doe", "dateOfBirth": "19800101",
				"sex": "F", "sexDisplay": "Female",
				"ids": []interface{}{map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "PAT-001"}},
			},
			"author": map[string]interface{}{"given": "John", "family": "Smith", "npi": "1234567890"},
		},
		"sections": map[string]interface{}{
			"healthConcerns": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"concernCode": "44054006", "concernCodeDisplay": "Diabetes mellitus type 2",
					"status": "active", "effectiveTime": "20240101",
				},
			}},
			"goals": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"goalCode": "182840001", "goalCodeDisplay": "Drug therapy",
					"value": "162673000", "valueDisplay": "General examination of patient",
					"targetDate": "20240601",
					"achievementStatus": "390855002", "achievementStatusDisplay": "In progress",
				},
			}},
		},
	}
}

// TestBuildDocument_CarePlan_ShallSectionsAlwaysEmitted mirrors
// TestBuildDocument_EmptyShallSection_StillEmitted for Care Plan's own SHALL
// sections — an empty SHALL section must still be emitted (narrative only),
// never silently dropped.
func TestBuildDocument_CarePlan_ShallSectionsAlwaysEmitted(t *testing.T) {
	loader := loadTestSchema(t)
	doc := carePlanCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["goals"] = map[string]interface{}{"entries": []interface{}{}}

	xml, err := BuildDocument(loader, doc, "Care Plan", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, "75310-3") { // Health Concerns section LOINC code
		t.Error("expected Health Concerns SHALL section to be emitted with an entry")
	}
	if !strings.Contains(xml, "61146-5") { // Goals section LOINC code
		t.Error("expected empty SHALL section (Goals) to still be emitted with its templateId/code")
	}
}

// transferSummaryCanonicalDoc supplies one entry each for Transfer
// Summary's 6 SHALL sections — all already registered/reused by other
// document types (allergiesAndIntolerances, medications, problems, results,
// vitalSigns from fullCanonicalDoc()'s own convention; reasonForReferral
// per its own field catalog: referralCode/referralText/effectiveTime).
func transferSummaryCanonicalDoc() map[string]interface{} {
	return map[string]interface{}{
		"header": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName": "Jane", "lastName": "Doe", "dateOfBirth": "19800101",
				"sex": "F", "sexDisplay": "Female",
				"ids": []interface{}{map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "PAT-001"}},
			},
			"author": map[string]interface{}{"given": "John", "family": "Smith", "npi": "1234567890"},
		},
		"sections": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "7980", "medicationAllergyCodeDisplay": "Penicillin",
					"medicationAllergyCodeSystem": "2.16.840.1.113883.6.88",
					"reaction":                    "247472004", "reactionDisplay": "Hives",
					"severity": "moderate", "status": "Active", "onsetDate": "20100101",
				},
			}},
			"medications": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"drugCode": "197361", "drugCodeDisplay": "Lisinopril 10mg",
					"doseQuantity": "10", "doseQuantityUnit": "mg",
					"routeCode": "C38288", "routeCodeDisplay": "Oral",
					"status": "active", "startDate": "20200101",
				},
			}},
			"problems": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"conditionCode": "44054006", "conditionCodeDisplay": "Diabetes mellitus type 2",
					"conditionCodeSystem": "2.16.840.1.113883.6.96",
					"status":              "55561003", "onsetDate": "20190601",
				},
			}},
			"results": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"testCode": "2345-7", "testCodeDisplay": "Glucose",
					"resultValue": "95", "resultValueUnit": "mg/dL", "resultStatus": "completed",
					"effectiveTime": "20240101120000",
				},
			}},
			"vitalSigns": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"vitalCode": "8480-6", "vitalCodeDisplay": "Systolic BP",
					"value": "120", "valueUnit": "mm[Hg]", "effectiveTime": "20240101120000",
				},
			}},
			"reasonForReferral": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"referralCode": "3457005", "referralCodeDisplay": "Patient referral",
					"referralText": "Transfer to skilled nursing facility for continued rehabilitation.",
					"effectiveTime": "20240101",
				},
			}},
		},
	}
}

// TestBuildDocument_TransferSummary_ShallSectionsAlwaysEmitted mirrors the
// Care Plan version above for Transfer Summary's 6 SHALL sections.
func TestBuildDocument_TransferSummary_ShallSectionsAlwaysEmitted(t *testing.T) {
	loader := loadTestSchema(t)
	doc := transferSummaryCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["results"] = map[string]interface{}{"entries": []interface{}{}}

	xml, err := BuildDocument(loader, doc, "Transfer Summary", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, "30954-2") { // Results section LOINC code
		t.Error("expected empty SHALL section (Results) to still be emitted with its templateId/code")
	}
	if !strings.Contains(xml, "42349-1") { // Reason for Referral section LOINC code
		t.Error("expected Reason for Referral SHALL section to be emitted with an entry")
	}
}

// TestBuildDocument_TransferSummary_RoundTripsThroughParserAndValidator is
// the same pattern as the CCD/Care Plan round-trip tests above, scaled to
// Transfer Summary's 6-section SHALL list.
func TestBuildDocument_TransferSummary_RoundTripsThroughParserAndValidator(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, transferSummaryCanonicalDoc(), "Transfer Summary", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.1.13"`) || !strings.Contains(xml, `extension="2015-08-01"`) {
		t.Errorf("expected the document-level templateId to be Transfer Summary's own (.1.13, 2015-08-01)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	doc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}

	v := validator.NewCDAConformanceValidator(loader)
	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			t.Errorf("SHALL section %q is missing after round-trip (xml below)\n--- XML ---\n%s", sr.SectionKey, xml)
		}
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (all 6 SHALL sections present)", report.ShallScore)
	}
}

// diagnosticImagingReportCanonicalDoc supplies only header data — Findings
// Section (DIR), Diagnostic Imaging Report's one SHALL section, is
// narrative-only (fields: [], no entryElementPath, same shape as
// hospitalCourse) so it needs no canonical section entries at all; it must
// still be auto-emitted with its templateId/boilerplate narrative per SHALL
// conformance, the same "empty SHALL section still emitted" guarantee
// TestBuildDocument_EmptyShallSection_StillEmitted locks in for structured
// sections.
func diagnosticImagingReportCanonicalDoc() map[string]interface{} {
	return map[string]interface{}{
		"header": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName": "Jane", "lastName": "Doe", "dateOfBirth": "19800101",
				"sex": "F", "sexDisplay": "Female",
				"ids": []interface{}{map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "PAT-001"}},
			},
			"author": map[string]interface{}{"given": "John", "family": "Smith", "npi": "1234567890"},
		},
		"sections": map[string]interface{}{},
	}
}

// TestBuildDocument_DiagnosticImagingReport_FindingsSectionAlwaysEmitted
// confirms the one SHALL section (Findings Section (DIR)) is emitted even
// though DIR is the only document type in this schema whose SHALL section
// has no canonical entry data path at all — the narrative-only shape must
// still satisfy SHALL conformance on its own.
func TestBuildDocument_DiagnosticImagingReport_FindingsSectionAlwaysEmitted(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, diagnosticImagingReportCanonicalDoc(), "Diagnostic Imaging Report", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, "2.16.840.1.113883.10.20.6.1.2") { // Findings Section (DIR) templateId
		t.Error("expected Findings Section (DIR) to be emitted with its templateId even with no canonical entry data")
	}
}

// TestBuildDocument_DiagnosticImagingReport_RoundTripsThroughParserAndValidator
// mirrors the Care Plan/Transfer Summary round-trip tests above.
func TestBuildDocument_DiagnosticImagingReport_RoundTripsThroughParserAndValidator(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, diagnosticImagingReportCanonicalDoc(), "Diagnostic Imaging Report", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.1.5"`) || !strings.Contains(xml, `extension="2015-08-01"`) {
		t.Errorf("expected the document-level templateId to be Diagnostic Imaging Report's own (.1.5, 2015-08-01)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	doc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}

	v := validator.NewCDAConformanceValidator(loader)
	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			t.Errorf("SHALL section %q is missing after round-trip (xml below)\n--- XML ---\n%s", sr.SectionKey, xml)
		}
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (the one SHALL section present)", report.ShallScore)
	}
}

// operativeNoteCanonicalDoc and procedureNoteCanonicalDoc supply only header
// data — every SHALL section registered for these two document types is
// narrative-only (fields: [], no entryElementPath) for this first pass, per
// the plan's own explicit v1 scoping (structured entries are real, named
// follow-on work, not silently dropped — see the plan file's "Coverage-
// Audit-visible-but-unmapped" trade-off). Every one of these sections'
// spec-verified constraints table shows entries as 0..* MAY or 0..1 SHOULD,
// never 1..1 SHALL, so a narrative-only implementation is spec-conformant
// on its own, same as Diagnostic Imaging Report's Findings Section (DIR)
// above.
func operativeNoteCanonicalDoc() map[string]interface{} {
	return map[string]interface{}{
		"header": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName": "Jane", "lastName": "Doe", "dateOfBirth": "19800101",
				"sex": "F", "sexDisplay": "Female",
				"ids": []interface{}{map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "PAT-001"}},
			},
			"author": map[string]interface{}{"given": "John", "family": "Smith", "npi": "1234567890"},
		},
		"sections": map[string]interface{}{},
	}
}

func procedureNoteCanonicalDoc() map[string]interface{} {
	return operativeNoteCanonicalDoc()
}

// TestBuildDocument_OperativeNote_ShallSectionsAlwaysEmitted confirms all 8
// SHALL sections (anesthesia, complications, preoperativeDiagnosis,
// procedureEstimatedBloodLoss, procedureFindings, procedureSpecimensTaken,
// procedureDescription, postoperativeDiagnosis) are emitted even with no
// canonical section data.
func TestBuildDocument_OperativeNote_ShallSectionsAlwaysEmitted(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, operativeNoteCanonicalDoc(), "Operative Note", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	shallTemplateIDs := map[string]string{
		"anesthesia":                  "2.16.840.1.113883.10.20.22.2.25",
		"complications":               "2.16.840.1.113883.10.20.22.2.37",
		"preoperativeDiagnosis":       "2.16.840.1.113883.10.20.22.2.34",
		"procedureEstimatedBloodLoss": "2.16.840.1.113883.10.20.18.2.9",
		"procedureFindings":           "2.16.840.1.113883.10.20.22.2.28",
		"procedureSpecimensTaken":     "2.16.840.1.113883.10.20.22.2.31",
		"procedureDescription":        "2.16.840.1.113883.10.20.22.2.27",
		"postoperativeDiagnosis":      "2.16.840.1.113883.10.20.22.2.35",
	}
	for key, templateID := range shallTemplateIDs {
		if !strings.Contains(xml, templateID) {
			t.Errorf("expected SHALL section %q (templateId %s) to be emitted with no canonical entry data", key, templateID)
		}
	}
}

// TestBuildDocument_OperativeNote_RoundTripsThroughParserAndValidator
// mirrors the round-trip tests above for Operative Note's larger 8-section
// SHALL list.
func TestBuildDocument_OperativeNote_RoundTripsThroughParserAndValidator(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, operativeNoteCanonicalDoc(), "Operative Note", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.1.7"`) || !strings.Contains(xml, `extension="2015-08-01"`) {
		t.Errorf("expected the document-level templateId to be Operative Note's own (.1.7, 2015-08-01)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	doc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}

	v := validator.NewCDAConformanceValidator(loader)
	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			t.Errorf("SHALL section %q is missing after round-trip (xml below)\n--- XML ---\n%s", sr.SectionKey, xml)
		}
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (all 8 SHALL sections present)", report.ShallScore)
	}
}

// TestBuildDocument_ProcedureNote_ShallSectionsAlwaysEmitted confirms all 4
// SHALL sections (complications, procedureDescription, procedureIndications,
// postprocedureDiagnosis) are emitted even with no canonical section data.
func TestBuildDocument_ProcedureNote_ShallSectionsAlwaysEmitted(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, procedureNoteCanonicalDoc(), "Procedure Note", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	shallTemplateIDs := map[string]string{
		"complications":          "2.16.840.1.113883.10.20.22.2.37",
		"procedureDescription":   "2.16.840.1.113883.10.20.22.2.27",
		"procedureIndications":   "2.16.840.1.113883.10.20.22.2.29",
		"postprocedureDiagnosis": "2.16.840.1.113883.10.20.22.2.36",
	}
	for key, templateID := range shallTemplateIDs {
		if !strings.Contains(xml, templateID) {
			t.Errorf("expected SHALL section %q (templateId %s) to be emitted with no canonical entry data", key, templateID)
		}
	}
}

// TestBuildDocument_ProcedureNote_RoundTripsThroughParserAndValidator
// mirrors the round-trip tests above for Procedure Note.
func TestBuildDocument_ProcedureNote_RoundTripsThroughParserAndValidator(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, procedureNoteCanonicalDoc(), "Procedure Note", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.1.6"`) || !strings.Contains(xml, `extension="2015-08-01"`) {
		t.Errorf("expected the document-level templateId to be Procedure Note's own (.1.6, 2015-08-01)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	doc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}

	v := validator.NewCDAConformanceValidator(loader)
	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			t.Errorf("SHALL section %q is missing after round-trip (xml below)\n--- XML ---\n%s", sr.SectionKey, xml)
		}
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (all 4 SHALL sections present)", report.ShallScore)
	}
}

// TestBuildDocument_ProcedureNote_NewOptionalSections covers the 5 sections
// verified and added in the "boost to 100%" follow-on pass: reasonForVisit,
// chiefComplaintAndReasonForVisit, medicalGeneralHistory (all narrative-only
// per their own spec constraints tables — no entry row at all), plus
// medicationsAdministered, which DOES have a spec-defined entry (0..* MAY
// substanceAdministration reusing Medication Activity (V2), same shape as
// anesthesia) and physicalExamination, which reuses the H&P section verbatim
// (byte-for-byte same templateId/extension/LOINC — confirmed against Vol 2
// §2.47, not a new section).
func TestBuildDocument_ProcedureNote_NewOptionalSections(t *testing.T) {
	loader := loadTestSchema(t)
	doc := procedureNoteCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["reasonForVisit"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}
	sections["chiefComplaintAndReasonForVisit"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}
	sections["medicalGeneralHistory"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}
	sections["physicalExamination"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}
	sections["medicationsAdministered"] = map[string]interface{}{"entries": []interface{}{
		map[string]interface{}{
			"drugCode": "387458008", "drugCodeDisplay": "Isotonic saline", "drugCodeSystem": "2.16.840.1.113883.6.96",
			"routeCode": "C38276", "routeCodeDisplay": "Intravenous", "status": "completed",
		},
	}}

	xml, err := BuildDocument(loader, doc, "Procedure Note", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	narrativeTemplateIDs := map[string]string{
		"reasonForVisit":                  "2.16.840.1.113883.10.20.22.2.12",
		"chiefComplaintAndReasonForVisit": "2.16.840.1.113883.10.20.22.2.13",
		"medicalGeneralHistory":           "2.16.840.1.113883.10.20.22.2.39",
		"physicalExamination":             "2.16.840.1.113883.10.20.2.10",
	}
	for key, templateID := range narrativeTemplateIDs {
		if !strings.Contains(xml, templateID) {
			t.Errorf("expected section %q (templateId %s) to be emitted", key, templateID)
		}
	}
	if !strings.Contains(xml, "29549-3") { // Medications Administered Section (V2) LOINC
		t.Error("expected Medications Administered Section's LOINC code to be emitted")
	}
	if !strings.Contains(xml, `<substanceAdministration classCode="SBADM" moodCode="EVN">`) {
		t.Errorf("expected Medications Administered's entry to be a substanceAdministration (Medication Activity V2)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	parsedDoc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}
	sec, ok := parsedDoc.SectionsByKey["medicationsAdministered"]
	if !ok || len(sec.Entries) != 1 {
		t.Fatalf("expected medicationsAdministered to survive round-trip with 1 entry, got %+v", sec)
	}
}

// TestBuildDocument_CarePlan_NewOptionalSections covers Health Status
// Evaluations and Outcomes Section (structured — Outcome Observation
// .4.144, a free-form LOINC code per its own SHALL constraint, unlike most
// reused-observation sections which have a fixed code) and Interventions
// Section (V3) (narrative-only v1 — its own entries are all 0..* SHOULD,
// never required, and Intervention Act (V2) wraps 11 possible nested
// activity types per its own contexts table, too polymorphic for a v1 pass).
func TestBuildDocument_CarePlan_NewOptionalSections(t *testing.T) {
	loader := loadTestSchema(t)
	doc := carePlanCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["healthStatusEvaluationsOutcomes"] = map[string]interface{}{"entries": []interface{}{
		map[string]interface{}{
			"outcomeCode": "59408-5", "outcomeCodeDisplay": "Oxygen saturation in Arterial blood",
			"value": "95", "valueDisplay": "95%",
		},
	}}
	sections["interventions"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}

	xml, err := BuildDocument(loader, doc, "Care Plan", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, "11383-7") { // Health Status Evaluations and Outcomes Section LOINC
		t.Error("expected Health Status Evaluations and Outcomes Section to be emitted")
	}
	if !strings.Contains(xml, "59408-5") {
		t.Error("expected Outcome Observation's free-form code to be emitted (not a fixed discriminator)")
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.21.2.3" extension="2015-08-01"`) {
		t.Errorf("expected Interventions Section (V3)'s templateId with its extension\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	parsedDoc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}
	sec, ok := parsedDoc.SectionsByKey["healthStatusEvaluationsOutcomes"]
	if !ok || len(sec.Entries) != 1 {
		t.Fatalf("expected healthStatusEvaluationsOutcomes to survive round-trip with 1 entry, got %+v", sec)
	}
	if sec.Entries[0].Value == nil || sec.Entries[0].Value.Code == nil || sec.Entries[0].Value.Code.Code != "95" {
		t.Errorf("expected Outcome Observation's value code to survive round-trip, got %+v", sec.Entries[0].Value)
	}
}

// TestBuildDocument_TransferSummary_CourseOfCareAndGeneralStatus_NarrativeOnly
// covers the 2 sections added in the "boost to 100%" follow-on pass — both
// confirmed narrative-only against Vol 2 (Course of Care §2.11, General
// Status §2.21: SHALL lists show only templateId/code/title/text, no entry
// row at all).
func TestBuildDocument_TransferSummary_CourseOfCareAndGeneralStatus_NarrativeOnly(t *testing.T) {
	loader := loadTestSchema(t)
	doc := transferSummaryCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["courseOfCare"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}
	sections["generalStatus"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}

	xml, err := BuildDocument(loader, doc, "Transfer Summary", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.2.64"`) {
		t.Error("expected Course of Care Section's templateId")
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.2.5"`) {
		t.Error("expected General Status Section's templateId (legacy non-C-CDA OID root)")
	}
}

// TestBuildDocument_HistoryAndPhysical_CorrectedShallSections proves the fix
// for a pre-existing bug: documentTypeSections["History and Physical"] used
// 3 optional sections as SHALL and was missing 8 of the 10 truly required
// sections per Vol 2 §1.1.12 Table 39 / Companion Guide §3.3.6 Table 21
// (both agree). All 10 corrected SHALL sections are already-registered keys
// reused from CCD/Discharge Summary/etc. — this is a pure documentTypeSections
// data fix, not a new section.
func TestBuildDocument_HistoryAndPhysical_CorrectedShallSections(t *testing.T) {
	loader := loadTestSchema(t)
	doc := transferSummaryCanonicalDoc() // reuse: has allergiesAndIntolerances/medications/problems/results/vitalSigns entries
	sections := doc["sections"].(map[string]interface{})
	sections["pastMedicalHistory"] = map[string]interface{}{"entries": []interface{}{
		map[string]interface{}{"conditionCode": "44054006", "conditionCodeDisplay": "Diabetes mellitus type 2"},
	}}
	sections["socialHistory"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}
	sections["familyHistory"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}
	sections["reviewOfSystems"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}
	sections["generalStatus"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}
	sections["physicalExamination"] = map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}}

	xml, err := BuildDocument(loader, doc, "History and Physical", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	parser := cdadocument.NewCDAParser(loader)
	parsedDoc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}
	v := validator.NewCDAConformanceValidator(loader)
	report := v.Validate(parsedDoc)
	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			t.Errorf("SHALL section %q is missing after round-trip (xml below)\n--- XML ---\n%s", sr.SectionKey, xml)
		}
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (all 10 corrected SHALL sections present)", report.ShallScore)
	}
	// chiefComplaint/historyOfPresentIllness/assessment must NOT be reported
	// as missing SHALL sections anymore — they moved to MAY.
	for _, sr := range report.SectionReports {
		if (sr.SectionKey == "chiefComplaint" || sr.SectionKey == "historyOfPresentIllness" || sr.SectionKey == "assessment") && sr.Conformance == "SHALL" {
			t.Errorf("section %q should no longer be SHALL for History and Physical", sr.SectionKey)
		}
	}
}

// dischargeSummaryCanonicalDoc supplies one entry each for Discharge
// Summary's corrected 4 SHALL sections (allergiesAndIntolerances,
// hospitalCourse, dischargeDiagnosis, planOfTreatment — corrected against
// Vol2 Table 36 / Companion Guide Table 20: the previous list had
// dischargeMedications/problems wrongly SHALL and planOfTreatment missing
// entirely).
func dischargeSummaryCanonicalDoc() map[string]interface{} {
	return map[string]interface{}{
		"header": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName": "Jane", "lastName": "Doe", "dateOfBirth": "19800101",
				"sex": "F", "sexDisplay": "Female",
				"ids": []interface{}{map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "PAT-001"}},
			},
			"author": map[string]interface{}{"given": "John", "family": "Smith", "npi": "1234567890"},
		},
		"sections": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "7980", "medicationAllergyCodeDisplay": "Penicillin",
					"medicationAllergyCodeSystem": "2.16.840.1.113883.6.88",
					"reaction":                    "247472004", "reactionDisplay": "Hives",
					"severity": "moderate", "status": "Active", "onsetDate": "20100101",
				},
			}},
			"hospitalCourse": map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}},
			"dischargeDiagnosis": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"conditionCode": "44054006", "conditionCodeDisplay": "Diabetes mellitus type 2"},
			}},
			"planOfTreatment": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"activityCode": "182836005", "activityCodeDisplay": "Review of medication",
					"effectiveTime": "20240201", "status": "INT",
				},
			}},
		},
	}
}

func TestBuildDocument_DischargeSummary_ShallSectionsAlwaysEmitted(t *testing.T) {
	loader := loadTestSchema(t)
	doc := dischargeSummaryCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["hospitalCourse"] = map[string]interface{}{"entries": []interface{}{}}

	xml, err := BuildDocument(loader, doc, "Discharge Summary", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, "8648-8") { // Hospital Course Section LOINC
		t.Error("expected empty SHALL section (Hospital Course) to still be emitted with its templateId/code")
	}
	if !strings.Contains(xml, "44054006") { // dischargeDiagnosis's real condition code, non-empty SHALL section
		t.Error("expected the non-empty SHALL section (dischargeDiagnosis) entry data to be emitted")
	}
}

func TestBuildDocument_DischargeSummary_RoundTripsThroughParserAndValidator(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, dischargeSummaryCanonicalDoc(), "Discharge Summary", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.1.8"`) || !strings.Contains(xml, `extension="2015-08-01"`) {
		t.Errorf("expected the document-level templateId to be Discharge Summary's own (.1.8, 2015-08-01)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	doc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}

	v := validator.NewCDAConformanceValidator(loader)
	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			t.Errorf("SHALL section %q is missing after round-trip (xml below)\n--- XML ---\n%s", sr.SectionKey, xml)
		}
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (all 4 corrected SHALL sections present)", report.ShallScore)
	}
}

// referralNoteCanonicalDoc supplies Referral Note's 4 SHALL sections plus
// assessmentAndPlan to satisfy the document type's choice constraint
// ("SHALL contain Assessment and Plan Section, OR both Assessment Section
// and Plan of Treatment Section").
func referralNoteCanonicalDoc() map[string]interface{} {
	return map[string]interface{}{
		"header": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName": "Jane", "lastName": "Doe", "dateOfBirth": "19800101",
				"sex": "F", "sexDisplay": "Female",
				"ids": []interface{}{map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "PAT-001"}},
			},
			"author": map[string]interface{}{"given": "John", "family": "Smith", "npi": "1234567890"},
		},
		"sections": map[string]interface{}{
			"reasonForReferral": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"referralCode": "3457005", "referralCodeDisplay": "Patient referral",
					"referralText": "Transfer to skilled nursing facility for continued rehabilitation.",
					"effectiveTime": "20240101",
				},
			}},
			"problems": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"conditionCode": "44054006", "conditionCodeDisplay": "Diabetes mellitus type 2",
					"conditionCodeSystem": "2.16.840.1.113883.6.96",
					"status":              "55561003", "onsetDate": "20190601",
				},
			}},
			"medications": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"drugCode": "197361", "drugCodeDisplay": "Lisinopril 10mg",
					"doseQuantity": "10", "doseQuantityUnit": "mg",
					"routeCode": "C38288", "routeCodeDisplay": "Oral",
					"status": "active", "startDate": "20200101",
				},
			}},
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "7980", "medicationAllergyCodeDisplay": "Penicillin",
					"medicationAllergyCodeSystem": "2.16.840.1.113883.6.88",
					"reaction":                    "247472004", "reactionDisplay": "Hives",
					"severity": "moderate", "status": "Active", "onsetDate": "20100101",
				},
			}},
			"assessmentAndPlan": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"activityCode": "384821006", "activityCodeDisplay": "Follow up",
					"effectiveTime": "20240101", "status": "active",
				},
			}},
		},
	}
}

func TestBuildDocument_ReferralNote_RoundTripsThroughParserAndValidator(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, referralNoteCanonicalDoc(), "Referral Note", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.1.14"`) || !strings.Contains(xml, `extension="2015-08-01"`) {
		t.Errorf("expected the document-level templateId to be Referral Note's own (.1.14, 2015-08-01)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	doc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}

	v := validator.NewCDAConformanceValidator(loader)
	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			t.Errorf("SHALL section %q is missing after round-trip (xml below)\n--- XML ---\n%s", sr.SectionKey, xml)
		}
	}
	if len(report.ChoiceConstraintReports) != 1 || !report.ChoiceConstraintReports[0].Satisfied {
		t.Errorf("expected the assessmentAndPlan/assessment+planOfTreatment choice constraint to be satisfied, got %+v", report.ChoiceConstraintReports)
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (4 SHALL sections + satisfied choice constraint)", report.ShallScore)
	}
}

// consultationNoteCanonicalDoc supplies Consultation Note's 3 SHALL sections
// (historyOfPresentIllness is narrative-only) plus assessmentAndPlan to
// satisfy the same choice constraint as Referral Note.
func consultationNoteCanonicalDoc() map[string]interface{} {
	return map[string]interface{}{
		"header": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName": "Jane", "lastName": "Doe", "dateOfBirth": "19800101",
				"sex": "F", "sexDisplay": "Female",
				"ids": []interface{}{map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "PAT-001"}},
			},
			"author": map[string]interface{}{"given": "John", "family": "Smith", "npi": "1234567890"},
		},
		"sections": map[string]interface{}{
			"historyOfPresentIllness": map[string]interface{}{"entries": []interface{}{map[string]interface{}{}}},
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "7980", "medicationAllergyCodeDisplay": "Penicillin",
					"medicationAllergyCodeSystem": "2.16.840.1.113883.6.88",
					"reaction":                    "247472004", "reactionDisplay": "Hives",
					"severity": "moderate", "status": "Active", "onsetDate": "20100101",
				},
			}},
			"problems": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"conditionCode": "44054006", "conditionCodeDisplay": "Diabetes mellitus type 2",
					"conditionCodeSystem": "2.16.840.1.113883.6.96",
					"status":              "55561003", "onsetDate": "20190601",
				},
			}},
			"assessmentAndPlan": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"activityCode": "384821006", "activityCodeDisplay": "Follow up",
					"effectiveTime": "20240101", "status": "active",
				},
			}},
		},
	}
}

func TestBuildDocument_ConsultationNote_RoundTripsThroughParserAndValidator(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, consultationNoteCanonicalDoc(), "Consultation Note", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.1.4"`) || !strings.Contains(xml, `extension="2015-08-01"`) {
		t.Errorf("expected the document-level templateId to be Consultation Note's own (.1.4, 2015-08-01)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	doc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}

	v := validator.NewCDAConformanceValidator(loader)
	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			t.Errorf("SHALL section %q is missing after round-trip (xml below)\n--- XML ---\n%s", sr.SectionKey, xml)
		}
	}
	if len(report.ChoiceConstraintReports) != 1 || !report.ChoiceConstraintReports[0].Satisfied {
		t.Errorf("expected the assessmentAndPlan/assessment+planOfTreatment choice constraint to be satisfied, got %+v", report.ChoiceConstraintReports)
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (3 SHALL sections + satisfied choice constraint)", report.ShallScore)
	}
}

// progressNoteCanonicalDoc supplies ONLY assessmentAndPlan — Progress Note's
// SHALL list is genuinely empty per spec (Companion Guide Table 24's
// Required Sections column is blank), the choice constraint is the sole
// SHALL-level requirement.
func progressNoteCanonicalDoc() map[string]interface{} {
	return map[string]interface{}{
		"header": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName": "Jane", "lastName": "Doe", "dateOfBirth": "19800101",
				"sex": "F", "sexDisplay": "Female",
				"ids": []interface{}{map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "PAT-001"}},
			},
			"author": map[string]interface{}{"given": "John", "family": "Smith", "npi": "1234567890"},
		},
		"sections": map[string]interface{}{
			"assessmentAndPlan": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"activityCode": "384821006", "activityCodeDisplay": "Follow up",
					"effectiveTime": "20240101", "status": "active",
				},
			}},
		},
	}
}

func TestBuildDocument_ProgressNote_RoundTripsThroughParserAndValidator(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, progressNoteCanonicalDoc(), "Progress Note", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.1.9"`) || !strings.Contains(xml, `extension="2015-08-01"`) {
		t.Errorf("expected the document-level templateId to be Progress Note's own (.1.9, 2015-08-01)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	doc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}

	v := validator.NewCDAConformanceValidator(loader)
	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			t.Errorf("SHALL section %q is missing after round-trip (xml below)\n--- XML ---\n%s", sr.SectionKey, xml)
		}
	}
	if len(report.ChoiceConstraintReports) != 1 || !report.ChoiceConstraintReports[0].Satisfied {
		t.Errorf("expected the assessmentAndPlan/assessment+planOfTreatment choice constraint to be satisfied, got %+v", report.ChoiceConstraintReports)
	}
	// Progress Note's SHALL list is empty per spec — ShallScore must be
	// achievable (1.0) purely via the satisfied choice constraint, with
	// zero individually-SHALL sections contributing to the score.
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (empty SHALL list + satisfied choice constraint)", report.ShallScore)
	}
	shallSectionCount := 0
	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" {
			shallSectionCount++
		}
	}
	if shallSectionCount != 0 {
		t.Errorf("expected 0 individually-SHALL sections for Progress Note, got %d", shallSectionCount)
	}
}

// TestBuildDocument_OperativeNote_StructuredEntries_RoundTrip covers the 6
// sections that DO have a spec-defined structured entry (Anesthesia,
// Complications, Preoperative Diagnosis via act+SUBJ+Problem Observation,
// Procedure Findings, Procedure Indications, Planned Procedure) — every
// templateId/fixed-code value here was verified directly against the C-CDA
// R2.1 Volume 2 spec's own entry-level template chapters (3.44 Indication
// (V2), 3.72 Postprocedure Diagnosis (V3), 3.75 Preoperative Diagnosis
// (V3)), not guessed. Confirms real canonical entry data produces the
// correct nested XML shape and survives a full build -> re-parse round
// trip into typed CDAEntry structs.
func TestBuildDocument_OperativeNote_StructuredEntries_RoundTrip(t *testing.T) {
	loader := loadTestSchema(t)
	doc := operativeNoteCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["anesthesia"] = map[string]interface{}{"entries": []interface{}{
		map[string]interface{}{
			"drugCode": "372733002", "drugCodeDisplay": "Sevoflurane", "drugCodeSystem": "2.16.840.1.113883.6.96",
			"routeCode": "C38216", "routeCodeDisplay": "Inhalation", "status": "completed",
		},
	}}
	sections["complications"] = map[string]interface{}{"entries": []interface{}{
		map[string]interface{}{
			"complicationCode": "110466006", "complicationCodeDisplay": "Postoperative hemorrhage",
			"complicationCodeSystem": "2.16.840.1.113883.6.96",
		},
	}}
	sections["preoperativeDiagnosis"] = map[string]interface{}{"entries": []interface{}{
		map[string]interface{}{
			"conditionCode": "74400008", "conditionCodeDisplay": "Appendicitis",
			"conditionCodeSystem": "2.16.840.1.113883.6.96",
		},
	}}
	sections["procedureFindings"] = map[string]interface{}{"entries": []interface{}{
		map[string]interface{}{
			"findingCode": "247472004", "findingCodeDisplay": "Inflamed appendix",
			"findingCodeSystem": "2.16.840.1.113883.6.96",
		},
	}}
	sections["procedureIndications"] = map[string]interface{}{"entries": []interface{}{
		map[string]interface{}{
			"indicationCode": "74400008", "indicationCodeDisplay": "Appendicitis",
			"indicationCodeSystem": "2.16.840.1.113883.6.96",
		},
	}}
	sections["plannedProcedure"] = map[string]interface{}{"entries": []interface{}{
		map[string]interface{}{
			"procedureCode": "80146002", "procedureCodeDisplay": "Appendectomy",
			"procedureCodeSystem": "2.16.840.1.113883.6.96",
		},
	}}

	xml, err := BuildDocument(loader, doc, "Operative Note", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	// Spot-check the shapes the spec's own worked examples showed.
	if !strings.Contains(xml, `<substanceAdministration classCode="SBADM" moodCode="EVN">`) {
		t.Errorf("expected Anesthesia's entry to be a substanceAdministration (Medication Activity V2)\n--- XML ---\n%s", xml)
	}
	if !strings.Contains(xml, "2.16.840.1.113883.10.20.22.4.65") {
		t.Errorf("expected Preoperative Diagnosis's own act templateId (.4.65) in output\n--- XML ---\n%s", xml)
	}
	if !strings.Contains(xml, `code="ASSERTION" codeSystem="2.16.840.1.113883.5.4"`) {
		t.Errorf("expected Procedure Indications' fixed ASSERTION discriminator code\n--- XML ---\n%s", xml)
	}
	if !strings.Contains(xml, `moodCode="RQO"`) {
		t.Errorf("expected Planned Procedure's entry moodCode override to RQO (per spec Figure 101)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	parsedDoc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}

	wantEntries := map[string]int{
		"anesthesia":            1,
		"complications":         1,
		"preoperativeDiagnosis": 1,
		"procedureFindings":     1,
		"procedureIndications":  1,
		"plannedProcedure":      1,
	}
	for key, want := range wantEntries {
		sec, ok := parsedDoc.SectionsByKey[key]
		if !ok {
			t.Errorf("section %q missing from re-parsed document", key)
			continue
		}
		if got := len(sec.Entries); got != want {
			t.Errorf("section %q: got %d entries after round-trip, want %d", key, got, want)
		}
	}
	// Preoperative Diagnosis's own Problem Observation must survive the
	// act -> entryRelationship[SUBJ] -> observation nesting round-trip.
	if preop, ok := parsedDoc.SectionsByKey["preoperativeDiagnosis"]; ok && len(preop.Entries) == 1 {
		entry := preop.Entries[0]
		if entry.EntryType != "act" {
			t.Errorf("Preoperative Diagnosis entry type = %q, want \"act\"", entry.EntryType)
		}
		if len(entry.EntryRelationships) != 1 || entry.EntryRelationships[0].Entry.Value == nil || entry.EntryRelationships[0].Entry.Value.Code == nil || entry.EntryRelationships[0].Entry.Value.Code.Code != "74400008" {
			t.Errorf("expected Preoperative Diagnosis's nested Problem Observation value code 74400008 to survive round-trip, got %+v", entry.EntryRelationships)
		}
	}
}

func TestBuildDocument_RoundTripsThroughParserAndValidator(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	parser := cdadocument.NewCDAParser(loader)
	doc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}

	v := validator.NewCDAConformanceValidator(loader)
	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			t.Errorf("SHALL section %q is missing after round-trip (xml below)\n--- XML ---\n%s", sr.SectionKey, xml)
		}
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (all SHALL sections present)", report.ShallScore)
	}

	// fullCanonicalDoc() supplies at least one entry for every one of CCD's
	// 17 real sections (6 SHALL + 2 SHOULD + 9 MAY, corrected against Vol2
	// Table 30 + Companion Guide Table 18 — planOfTreatment moved SHALL→SHOULD
	// and payersInsurance moved SHOULD→MAY from an earlier, uncorrected pass)
	// — confirm every one of them actually made it into the built document
	// as "present" or "present_empty", not just the SHALL subset checked
	// above.
	allCCDSections := []string{
		"allergiesAndIntolerances", "medications", "problems", "results", "planOfTreatment", "socialHistory", "vitalSigns",
		"procedures", "payersInsurance",
		"advanceDirectives", "encounters", "familyHistory", "functionalStatus", "immunizations", "medicalEquipment", "mentalStatus", "nutrition",
	}
	statusBySection := make(map[string]string, len(report.SectionReports))
	for _, sr := range report.SectionReports {
		statusBySection[sr.SectionKey] = sr.Status
	}
	for _, key := range allCCDSections {
		if statusBySection[key] == "missing" || statusBySection[key] == "" {
			t.Errorf("section %q: got status %q, want present/present_empty — cda.build should support every CCD section", key, statusBySection[key])
		}
	}

	if len(doc.Header.Informants) != 1 || doc.Header.Informants[0].AssignedEntity.AssignedPerson == nil {
		t.Error("expected the built <informant> to re-parse back into doc.Header.Informants")
	}
	if doc.Header.DocumentOf == nil || len(doc.Header.DocumentOf.Performers) != 1 {
		t.Error("expected the built <documentationOf> to re-parse back into doc.Header.DocumentOf")
	}
	if doc.Header.EncompassingEncounter == nil || doc.Header.EncompassingEncounter.Id.Extension != "ENC-001" {
		t.Error("expected the built <componentOf><encompassingEncounter> to re-parse back with id ENC-001")
	}
}

// TestBuildDocument_CarePlan_RoundTripsThroughParserAndValidator is the
// same pattern as the CCD round-trip test above, scaled to Care Plan's
// smaller 2-section SHALL list (healthConcerns, goals) — confirms the new
// document type resolves correctly end-to-end: build -> re-parse ->
// validate, with both SHALL sections reporting present, not missing.
func TestBuildDocument_CarePlan_RoundTripsThroughParserAndValidator(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, carePlanCanonicalDoc(), "Care Plan", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.1.15"`) || !strings.Contains(xml, `extension="2015-08-01"`) {
		t.Errorf("expected the document-level templateId to be Care Plan's own (.1.15, 2015-08-01)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	doc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}

	v := validator.NewCDAConformanceValidator(loader)
	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			t.Errorf("SHALL section %q is missing after round-trip (xml below)\n--- XML ---\n%s", sr.SectionKey, xml)
		}
	}
	if report.ShallScore != 1.0 {
		t.Errorf("ShallScore = %v, want 1.0 (both SHALL sections present)", report.ShallScore)
	}
}

func TestBuildDocument_EmptyShallSection_StillEmitted(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	// Remove all entries from a SHALL section — it must still be emitted
	// (with narrative only), per CCD conformance, not silently dropped.
	sections := doc["sections"].(map[string]interface{})
	sections["results"] = map[string]interface{}{"entries": []interface{}{}}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, "30954-2") { // Results section LOINC code
		t.Error("expected empty SHALL section (Results) to still be emitted with its templateId/code")
	}
}

func TestBuildDocument_EmptyShouldSection_Omitted(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["procedures"] = map[string]interface{}{"entries": []interface{}{}}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if strings.Contains(xml, "47519-4") { // Procedures section LOINC code
		t.Error("expected empty SHOULD section (Procedures) to be omitted entirely")
	}
}

func TestBuildDocument_UnknownDocumentType_Errors(t *testing.T) {
	loader := loadTestSchema(t)
	_, err := BuildDocument(loader, fullCanonicalDoc(), "Not A Real Document Type", BuildOptions{})
	if err == nil {
		t.Fatal("expected an error for an unknown document type")
	}
}

// unstructuredDocCanonicalDoc supplies header data plus an
// "unstructuredContent" block — the canonical shape BuildDocument checks
// for to decide between emitting <structuredBody> (every other document
// type) and <nonXMLBody> (Unstructured Document only, per CDA's own
// ClinicalDocument.component CHOICE).
func unstructuredDocCanonicalDoc() map[string]interface{} {
	return map[string]interface{}{
		"header": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName": "Jane", "lastName": "Doe", "dateOfBirth": "19800101",
				"sex": "F", "sexDisplay": "Female",
				"ids": []interface{}{map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "PAT-001"}},
			},
			"author": map[string]interface{}{"given": "John", "family": "Smith", "npi": "1234567890"},
		},
		"unstructuredContent": map[string]interface{}{
			"mediaType":     "application/pdf",
			"data":          "JVBERi0xLjQK",
			"documentCode":  "34108-1",
			"documentTitle": "Outpatient Note",
		},
	}
}

// TestBuildDocument_UnstructuredDocument_EmitsNonXMLBody confirms
// BuildDocument emits <component><nonXMLBody> instead of <structuredBody>
// for Unstructured Document, using the canonical data's own documentCode/
// documentTitle to override the document-level <code> (this is the one
// document type with no fixed LOINC per the IG — its code is selected
// per-document from whatever content is actually embedded).
func TestBuildDocument_UnstructuredDocument_EmitsNonXMLBody(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, unstructuredDocCanonicalDoc(), "Unstructured Document", BuildOptions{OrgName: "Test Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if strings.Contains(xml, "<structuredBody>") {
		t.Errorf("expected NO <structuredBody> for Unstructured Document\n--- XML ---\n%s", xml)
	}
	if !strings.Contains(xml, "<nonXMLBody>") {
		t.Errorf("expected <nonXMLBody> to be emitted\n--- XML ---\n%s", xml)
	}
	if !strings.Contains(xml, `mediaType="application/pdf"`) || !strings.Contains(xml, `representation="B64"`) || !strings.Contains(xml, "JVBERi0xLjQK") {
		t.Errorf("expected the nonXMLBody's text to carry the configured mediaType/representation/inline data\n--- XML ---\n%s", xml)
	}
	if !strings.Contains(xml, `code="34108-1"`) {
		t.Errorf("expected the document-level code to be overridden by the canonical documentCode\n--- XML ---\n%s", xml)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.1.10"`) {
		t.Errorf("expected the document-level templateId to be Unstructured Document's own (.1.10)\n--- XML ---\n%s", xml)
	}

	parser := cdadocument.NewCDAParser(loader)
	doc, err := parser.ParseFromRawXML(xml)
	if err != nil {
		t.Fatalf("re-parsing built XML failed: %v\n--- XML ---\n%s", err, xml)
	}
	if doc.UnstructuredBody == nil {
		t.Fatal("expected the re-parsed document to have UnstructuredBody populated")
	}
	if doc.UnstructuredBody.Data != "JVBERi0xLjQK" {
		t.Errorf("UnstructuredBody.Data = %q, want the round-tripped base64 content", doc.UnstructuredBody.Data)
	}
	if len(doc.Sections) != 0 {
		t.Errorf("expected zero sections for Unstructured Document, got %d", len(doc.Sections))
	}
}

// TestBuildDocument_UnstructuredDocument_ExternalReference covers the
// referenceUrl shape (an external file reference instead of inline base64
// content) — CDA's ED datatype allows either, never both.
func TestBuildDocument_UnstructuredDocument_ExternalReference(t *testing.T) {
	loader := loadTestSchema(t)
	doc := unstructuredDocCanonicalDoc()
	doc["unstructuredContent"] = map[string]interface{}{
		"mediaType":    "application/pdf",
		"referenceUrl": "https://example.org/docs/doc-1.pdf",
	}
	xml, err := BuildDocument(loader, doc, "Unstructured Document", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `<reference value="https://example.org/docs/doc-1.pdf"/>`) {
		t.Errorf("expected a <reference> element with the configured URL\n--- XML ---\n%s", xml)
	}
	if strings.Contains(xml, `representation="B64"`) {
		t.Error("expected no representation=\"B64\" attribute for a reference-shaped body")
	}
}

// TestBuildDocument_CustodianBackwardCompatible_OrgNameOnly locks in that
// CustodianOptions' zero value (every pipeline saved before this feature)
// still produces the exact pre-existing hardcoded shape: a bare
// npiOID-rooted id with no extension, and name from resolveOrgName(opts).
func TestBuildDocument_CustodianBackwardCompatible_OrgNameOnly(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{OrgName: "Legacy Health System"})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	idx := strings.Index(xml, "<custodian>")
	if idx == -1 {
		t.Fatal("expected a <custodian> element")
	}
	end := strings.Index(xml[idx:], "</custodian>")
	if end == -1 {
		t.Fatal("expected a closing </custodian> element")
	}
	custodianXML := xml[idx : idx+end]

	if !strings.Contains(custodianXML, `root="`+npiOID+`"`) {
		t.Errorf("expected custodian id root=%q, got: %s", npiOID, custodianXML)
	}
	if strings.Contains(custodianXML, "extension=") {
		t.Errorf("expected no id extension when Custodian is unset, got: %s", custodianXML)
	}
	if !strings.Contains(custodianXML, "<name>Legacy Health System</name>") {
		t.Errorf("expected custodian name to fall back to OrgName, got: %s", custodianXML)
	}
	if strings.Contains(custodianXML, "<addr>") || strings.Contains(custodianXML, "<telecom") {
		t.Errorf("expected no address/telecom when Custodian is unset, got: %s", custodianXML)
	}
}

// TestBuildDocument_ProblemNegationInd verifies the "no known problems"
// pattern (negationInd="true" on the Problem Observation, CONF:1198-10139)
// builds correctly, spec-verified against the C-CDA 2.1 IG (2018 errata)
// directly (pdftotext-extracted, not recall).
func TestBuildDocument_ProblemNegationInd(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"problems": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"conditionCode": "64572001", "conditionCodeDisplay": "No known problems",
					"conditionCodeSystem": "2.16.840.1.113883.6.96",
					"negationInd":         "true",
					"onsetDate":           "20200101",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `negationInd="true"`) {
		t.Errorf("expected negationInd=\"true\" in output, got: %s", xml)
	}
}

// TestBuildDocument_ProblemProgosisAndPriorityPreference verifies Problem
// Observation's two additional entryRelationship targets this session added
// — Prognosis Observation (templateId .4.113, CONF:1198-29951/29952, fixed
// code 75328-5 LOINC "Prognosis") and Priority Preference (templateId
// .4.143, CONF:1198-31063/31064, fixed code 225773000 SNOMED "Preference")
// — build with their spec-fixed codes alongside the pre-existing Age
// Observation/Problem Status entryRelationships, without collision (all
// four disambiguated by nested templateId, the same mechanism Allergy's
// reaction/severity/status/criticality already prove works).
func TestBuildDocument_ProblemProgosisAndPriorityPreference(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"problems": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"conditionCode": "44054006", "conditionCodeDisplay": "Diabetes mellitus type 2",
					"conditionCodeSystem": "2.16.840.1.113883.6.96",
					"onsetDate":           "20190601",
					"status":              "55561003",
					"ageAtOnset":          "45", "ageAtOnsetUnit": "a",
					"prognosisCode": "67334001", "prognosisCodeDisplay": "Guarded prognosis",
					"prognosisCodeSystem": "2.16.840.1.113883.6.96",
					"prognosisDate":       "20240301",
					"priorityCode":        "394849002", "priorityCodeDisplay": "High priority",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	prognosisIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.113"`)
	if prognosisIdx == -1 {
		t.Fatal("expected Prognosis Observation templateId in output")
	}
	prognosisTail := xml[prognosisIdx:]
	if end := strings.Index(prognosisTail, "</observation>"); end != -1 {
		prognosisTail = prognosisTail[:end]
	}
	if !strings.Contains(prognosisTail, `code="75328-5"`) {
		t.Errorf("expected Prognosis Observation's fixed LOINC code 75328-5, got: %s", prognosisTail)
	}
	if !strings.Contains(prognosisTail, `<value xsi:type="CD" code="67334001"`) {
		t.Errorf("expected the real prognosis value code on <value>, got: %s", prognosisTail)
	}

	priorityIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.143"`)
	if priorityIdx == -1 {
		t.Fatal("expected Priority Preference templateId in output")
	}
	priorityTail := xml[priorityIdx:]
	if end := strings.Index(priorityTail, "</observation>"); end != -1 {
		priorityTail = priorityTail[:end]
	}
	if !strings.Contains(priorityTail, `code="225773000"`) {
		t.Errorf("expected Priority Preference's fixed SNOMED code 225773000, got: %s", priorityTail)
	}
	if !strings.Contains(priorityTail, `<value xsi:type="CD" code="394849002"`) {
		t.Errorf("expected the real priority value code on <value>, got: %s", priorityTail)
	}

	// Age Observation (.4.31) and Problem Status (.4.6) — already-existing
	// entryRelationships — must still both be present alongside the two new
	// ones, confirming no collision across four same-parent entryRelationship
	// siblings disambiguated only by nested templateId.
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.31"`) {
		t.Error("expected pre-existing Age Observation templateId to still be present")
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.6"`) {
		t.Error("expected pre-existing Problem Status templateId to still be present")
	}

	// Age Observation and Problem Status both gained a fixedCode this pass
	// (previously missing entirely — a real schema-validator-confirmed
	// SHALL gap) — and Age's own value is xsi:type="PQ" (a physical
	// quantity, unlike every other coded value in this document).
	ageIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.31"`)
	ageTail := xml[ageIdx:]
	if end := strings.Index(ageTail, "</observation>"); end != -1 {
		ageTail = ageTail[:end]
	}
	if !strings.Contains(ageTail, `code="445518008"`) || !strings.Contains(ageTail, `codeSystem="2.16.840.1.113883.6.96"`) {
		t.Errorf("expected Age Observation's fixed code 445518008 (SNOMED Age At Onset), got: %s", ageTail)
	}
	if !strings.Contains(ageTail, `<value xsi:type="PQ" value="45" unit="a"`) {
		t.Errorf("expected Age Observation's value as xsi:type=PQ with value/unit, got: %s", ageTail)
	}

	statusIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.6"`)
	statusTail := xml[statusIdx:]
	if end := strings.Index(statusTail, "</observation>"); end != -1 {
		statusTail = statusTail[:end]
	}
	if !strings.Contains(statusTail, `code="33999-4"`) || !strings.Contains(statusTail, `codeSystem="2.16.840.1.113883.6.1"`) {
		t.Errorf("expected Problem Status's fixed code 33999-4 (LOINC Status), got: %s", statusTail)
	}
}

// TestBuildDocument_VitalSignsMultiComponent is the real (non-synthetic)
// proof of the RepeatingGroup loop engine: builds the actual loaded
// "vitalSigns" section (cda/schemas/ccda_2_1.json) with a canonical record
// shaped exactly the way map_to_canonical's GroupBy mode produces it
// ({"effectiveTime": ..., "components": [...]}), and verifies the real
// Vital Signs Organizer (V3, CONF:1198-7285 "SHALL contain at least one
// [1..*] component") / Vital Sign Observation (V2) spec shape: one
// <organizer> with its own fixed code (46680005 SNOMED, CONF:1198-32740/
// 32741/32742) and SHALL effectiveTime, containing 3 distinct <component>
// siblings — each with its own templateId (.4.27/2014-06-09), LOINC code,
// value+unit, and a MAY interpretationCode on the one record that supplies
// it.
func TestBuildDocument_VitalSignsMultiComponent(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"vitalSigns": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"effectiveTime": "20240115120000",
					"components": []interface{}{
						map[string]interface{}{
							"vitalCode": "8480-6", "vitalCodeDisplay": "Systolic blood pressure",
							"value": "120", "valueUnit": "mm[Hg]",
							"componentEffectiveTime": "20240115120000",
						},
						map[string]interface{}{
							"vitalCode": "8462-4", "vitalCodeDisplay": "Diastolic blood pressure",
							"value": "80", "valueUnit": "mm[Hg]",
							"componentEffectiveTime":    "20240115120000",
							"interpretationCode":        "N",
							"interpretationCodeDisplay": "Normal",
						},
						map[string]interface{}{
							"vitalCode": "8310-5", "vitalCodeDisplay": "Body temperature",
							"value": "37.0", "valueUnit": "Cel",
						},
					},
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	// Organizer's own fixed code + templateId (unaffected by the loop —
	// still written by the section-level EntryFixedCode/EntryTemplateID
	// mechanism exactly like every other section).
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.26" extension="2015-08-01"`) {
		t.Fatal("expected Vital Signs Organizer templateId in output")
	}
	if !strings.Contains(xml, `code="46680005"`) || !strings.Contains(xml, `codeSystem="2.16.840.1.113883.6.96"`) {
		t.Errorf("expected the Organizer's fixed SNOMED code 46680005, got: %s", xml)
	}

	componentCount := strings.Count(xml, `root="2.16.840.1.113883.10.20.22.4.27" extension="2014-06-09"`)
	if componentCount != 3 {
		t.Fatalf("expected 3 distinct Vital Sign Observation templateIds (one per component), got %d:\n%s", componentCount, xml)
	}

	wantCodes := []string{`code="8480-6"`, `code="8462-4"`, `code="8310-5"`}
	for _, want := range wantCodes {
		if !strings.Contains(xml, want) {
			t.Errorf("expected %s in output, not found", want)
		}
	}
	if !strings.Contains(xml, `value="120" unit="mm[Hg]"`) {
		t.Errorf("expected systolic value+unit in output, got: %s", xml)
	}
	if !strings.Contains(xml, `<interpretationCode code="N"`) {
		t.Errorf("expected the one component with an interpretationCode to carry it, got: %s", xml)
	}

	// Each <observation> must carry the standard OBS/EVN/completed
	// boilerplate — the same guard entry_archetypes_test.go's synthetic test
	// already covers, re-verified here against the real schema-loaded
	// section rather than a hand-built CDASectionDef.
	obsCount := strings.Count(xml, `classCode="OBS" moodCode="EVN"`)
	if obsCount != 3 {
		t.Errorf("expected 3 <observation classCode=\"OBS\" moodCode=\"EVN\"> elements, got %d", obsCount)
	}
}

// TestBuildDocument_VitalSigns_ComponentMissingRequiredValue_Skipped verifies
// RequiredPaths (["vitalCode","value"]) skips an incomplete component
// without leaving a gap in the resulting sibling numbering — the real-schema
// counterpart to entry_archetypes_test.go's synthetic
// TestWriteRepeatingGroups_RequiredPathsSkipsIncompleteItems.
func TestBuildDocument_VitalSigns_ComponentMissingRequiredValue_Skipped(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"vitalSigns": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"effectiveTime": "20240115120000",
					"components": []interface{}{
						map[string]interface{}{"vitalCode": "8480-6", "value": "120", "valueUnit": "mm[Hg]"},
						map[string]interface{}{"vitalCode": "8462-4"}, // missing "value" — must be skipped
					},
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if strings.Contains(xml, `code="8462-4"`) {
		t.Errorf("expected the incomplete component (no value) to be skipped, got: %s", xml)
	}
	componentCount := strings.Count(xml, `root="2.16.840.1.113883.10.20.22.4.27" extension="2014-06-09"`)
	if componentCount != 1 {
		t.Errorf("expected exactly 1 surviving component, got %d", componentCount)
	}
}

// TestBuildDocument_MedicationIndications_CrossTableJoinShape is the real
// (non-synthetic) proof of Block 3 Option C (cross-table join) reaching a
// real CDA archetype: builds the actual loaded "medications" section with a
// canonical record shaped exactly the way map_to_canonical's RelatedRows
// join produces it ({"drugCode": ..., "indications": [...]}) — verifying
// Medication Activity's 0..* Indication entryRelationship (CONF:1098-7536,
// typeCode="RSON" CONF:1098-7537) builds as 2 distinct <entryRelationship
// typeCode="RSON"> siblings, each with a nested Indication (V2) observation
// (templateId .4.19/2014-06-09, fixed code="ASSERTION"
// codeSystem="2.16.840.1.113883.5.4" per the IG's own Figure 164 example,
// and the real per-record diagnosis on <value>) — and that this coexists
// correctly with medications' PRE-EXISTING consumable/manufacturedProduct
// StructuralTemplateIDs anchor (the actual regression the WrapperAttr fix
// protects against, now exercised through the real schema+build pipeline
// instead of just the synthetic entry_archetypes_test.go case).
func TestBuildDocument_MedicationIndications_CrossTableJoinShape(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"medications": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"drugCode": "197361", "drugCodeDisplay": "Lisinopril 10mg",
					"status": "active",
					"indications": []interface{}{
						map[string]interface{}{"indicationCode": "44054006", "indicationCodeDisplay": "Diabetes mellitus type 2"},
						map[string]interface{}{"indicationCode": "38341003", "indicationCodeDisplay": "Hypertension"},
					},
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	// The pre-existing consumable/manufacturedProduct anchor must still work
	// (unaffected by the new RepeatingGroup on the same section).
	if !strings.Contains(xml, `code="197361"`) {
		t.Errorf("expected the drug code in output, got: %s", xml)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.23" extension="2014-06-09"`) {
		t.Error("expected Medication Information templateId (pre-existing StructuralTemplateIDs anchor) still present")
	}

	relCount := strings.Count(xml, `<entryRelationship typeCode="RSON">`)
	if relCount != 2 {
		t.Fatalf("expected 2 <entryRelationship typeCode=\"RSON\"> siblings (one per indication), got %d:\n%s", relCount, xml)
	}
	indicationTemplateCount := strings.Count(xml, `root="2.16.840.1.113883.10.20.22.4.19" extension="2014-06-09"`)
	if indicationTemplateCount != 2 {
		t.Fatalf("expected 2 Indication (V2) templateIds, got %d", indicationTemplateCount)
	}
	if !strings.Contains(xml, `<code code="ASSERTION" codeSystem="2.16.840.1.113883.5.4"`) {
		t.Errorf("expected the Indication observation's fixed ASSERTION code (per IG Figure 164), got: %s", xml)
	}
	for _, want := range []string{`<value xsi:type="CD" code="44054006"`, `<value xsi:type="CD" code="38341003"`} {
		if !strings.Contains(xml, want) {
			t.Errorf("expected %s in output, not found", want)
		}
	}
}

// TestBuildDocument_ResultsMultiComponent is the real (non-synthetic) proof
// of Results section fixes found auditing Result Organizer (V3, §3.91) /
// Result Observation (V3, §3.90) against the IG: (1) the organizer's own
// classCode is "BATTERY" (confirmed from Figure 213's worked example, NOT
// the tagBoilerplate "organizer" default of "CLUSTER" — a real bug the new
// EntryClassCodeOverride mechanism fixes, verified here so it can't silently
// regress); (2) Result Organizer is SHALL 1..* component (CONF:1198-7124),
// same loop shape as Vital Signs; (3) the organizer's own SHALL code/
// statusCode (CONF:1198-7128/7123) had no mappable field before this fix.
func TestBuildDocument_ResultsMultiComponent(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"results": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"panelCode": "57021-8", "panelCodeDisplay": "CBC W Auto Differential panel",
					"panelStatus": "completed",
					"components": []interface{}{
						map[string]interface{}{
							"testCode": "718-7", "testCodeDisplay": "Hemoglobin",
							"resultValue": "14.2", "resultValueUnit": "g/dL",
							"resultStatus": "completed", "componentEffectiveTime": "20240115120000",
						},
						map[string]interface{}{
							"testCode": "4544-3", "testCodeDisplay": "Hematocrit",
							"resultValue": "42", "resultValueUnit": "%",
							"resultStatus": "completed", "componentEffectiveTime": "20240115120000",
							"interpretationCode": "L", "interpretationCodeDisplay": "Low",
						},
					},
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	if !strings.Contains(xml, `<organizer classCode="BATTERY" moodCode="EVN">`) {
		t.Fatalf("expected Result Organizer's classCode=\"BATTERY\" (not the generic CLUSTER default), got: %s", xml)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.1" extension="2015-08-01"`) {
		t.Error("expected Result Organizer templateId in output")
	}
	if !strings.Contains(xml, `code="57021-8"`) {
		t.Errorf("expected the organizer's own panel code, got: %s", xml)
	}

	componentCount := strings.Count(xml, `root="2.16.840.1.113883.10.20.22.4.2" extension="2015-08-01"`)
	if componentCount != 2 {
		t.Fatalf("expected 2 distinct Result Observation templateIds (one per component), got %d:\n%s", componentCount, xml)
	}
	for _, want := range []string{`code="718-7"`, `code="4544-3"`, `value="14.2" unit="g/dL"`, `<interpretationCode code="L"`} {
		if !strings.Contains(xml, want) {
			t.Errorf("expected %s in output, not found", want)
		}
	}
}

// TestBuildDocument_SocialHistory_TobaccoUseCorrectTemplate is a real bug
// regression guard: the schema used to model smoking status by reusing the
// generic Social History Observation (V3, templateId .4.38) with an
// arbitrary "ASSERTION" discriminator code — but the IG's own Social History
// Observation chapter explicitly says "it is recommended to use ... the
// Tobacco Use template instead" for smoking (§3.99), and Tobacco Use (V2)
// has its OWN templateId (.4.85) and its OWN SHALL fixed code
// ("11367-0" LOINC "History of tobacco use", CONF:1098-19174/19175) — a
// completely different template, not a variant of the generic one. Before
// this fix, every built smoking-status entry carried either the WRONG
// templateId (.4.38) or, via the old "ASSERTION" predicate, no recognizable
// templateId injection at all (structuralTemplateIds is matched by
// discriminator code, and "ASSERTION" isn't the real one).
func TestBuildDocument_SocialHistory_TobaccoUseCorrectTemplate(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"socialHistory": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"observationCode": "72166-2", "observationCodeDisplay": "Tobacco smoking status",
					"smokingStatus": "8517006", "smokingStatusDisplay": "Former smoker",
					"smokingStatusEffectiveTime": "20150601",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.85" extension="2014-06-09"`) {
		t.Fatalf("expected Tobacco Use (V2) templateId in output, got: %s", xml)
	}
	if !strings.Contains(xml, `code="11367-0"`) {
		t.Errorf("expected Tobacco Use's fixed discriminator code 11367-0, got: %s", xml)
	}
	if !strings.Contains(xml, `<value xsi:type="CD" code="8517006"`) {
		t.Errorf("expected the real smoking status value code, got: %s", xml)
	}
}

// TestBuildDocument_MayLevelSections_FixedDiscriminatorCodes is a real bug
// regression guard covering three sections (functionalStatus, mentalStatus,
// nutrition) that all shared the SAME latent defect: their own root
// observation's <code> was modeled as a user-mappable "observationCode"
// field, but the IG fixes it to an exact constant in every one of the three
// cases (only visible in each template's own numbered constraint list, not
// the garbled overview table) — Functional Status Observation (V2)
// CONF:1098-31522/31523 ("54522-8" LOINC), Mental Status Observation (V3)
// CONF:1198-32788/32789 ("373930000" SNOMED), Nutritional Status
// Observation CONF:1098-29897/29898 ("75305-3" LOINC). Before this fix, a
// no-code config supplying any other value for these would have silently
// produced a non-conformant document with the wrong classifying code.
func TestBuildDocument_MayLevelSections_FixedDiscriminatorCodes(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"functionalStatus": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"value": "LA9938-9", "valueDisplay": "Independent", "effectiveTime": "20240101"},
			}},
			"mentalStatus": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"value": "LA9933-0", "valueDisplay": "Oriented x3", "effectiveTime": "20240101"},
			}},
			"nutrition": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"nutritionalStatusValue": "248325000", "effectiveTime": "20240101"},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `code="54522-8"`) {
		t.Errorf("expected Functional Status Observation's fixed code 54522-8, got: %s", xml)
	}
	if !strings.Contains(xml, `code="373930000" codeSystem="2.16.840.1.113883.6.96"`) {
		t.Errorf("expected Mental Status Observation's fixed SNOMED code 373930000, got: %s", xml)
	}
	if !strings.Contains(xml, `code="75305-3"`) {
		t.Errorf("expected Nutritional Status Observation's fixed code 75305-3, got: %s", xml)
	}
}

// TestBuildDocument_DischargeSummarySections_ActWrapperShapeAndFixedCodes is
// the real (non-synthetic) proof of the Discharge Summary structural bugs
// found this pass: dischargeDiagnosis/admissionDiagnosis/dischargeMedications/
// admissionMedications were all missing entryElementPath entirely (so their
// <act> wrapper never got templateId/boilerplate/fixed-code injected at
// all — the flat field writes were only ever auto-vivifying a bare,
// unidentified <act>), and dischargeMedications/admissionMedications were
// using plain Medication Activity's own templateId instead of their real,
// distinct wrapper templates (Discharge Medication V3 .4.35/2016-03-01,
// Admission Medication V2 .4.36/2014-06-09) confirmed only from each
// template's own chapter (§3.19, §3.1) — the section-level constraint
// tables just say "Contains: Medication Activity (V2) (required)" without
// making the DISTINCT wrapper-act identity obvious from a skim.
func TestBuildDocument_DischargeSummarySections_ActWrapperShapeAndFixedCodes(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"dischargeDiagnosis": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"conditionCode": "44054006", "conditionCodeDisplay": "Diabetes mellitus type 2"},
			}},
			"admissionDiagnosis": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"conditionCode": "38341003", "conditionCodeDisplay": "Hypertension"},
			}},
			"dischargeMedications": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"drugCode": "197361", "status": "active"},
			}},
			"admissionMedications": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"drugCode": "861007", "status": "active"},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "Discharge Summary", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	// Hospital Discharge Diagnosis (V3): act wrapper templateId + fixed code,
	// nested Problem Observation (V3) with the real condition code.
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.33" extension="2015-08-01"`) {
		t.Error("expected Hospital Discharge Diagnosis (V3) act wrapper templateId in output")
	}
	if !strings.Contains(xml, `<code code="11535-2"`) {
		t.Error("expected the act wrapper's fixed code 11535-2 (Hospital discharge diagnosis)")
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.4" extension="2015-08-01"`) {
		t.Error("expected the nested Problem Observation (V3) templateId in output")
	}
	if !strings.Contains(xml, `<value xsi:type="CD" code="44054006"`) {
		t.Error("expected the real discharge diagnosis condition code on the nested observation's value")
	}

	// Hospital Admission Diagnosis (V3): same shape, different fixed code.
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.34" extension="2015-08-01"`) {
		t.Error("expected Hospital Admission Diagnosis (V3) act wrapper templateId in output")
	}
	if !strings.Contains(xml, `<code code="46241-6"`) {
		t.Error("expected the act wrapper's fixed code 46241-6 (Admission diagnosis)")
	}
	if !strings.Contains(xml, `<value xsi:type="CD" code="38341003"`) {
		t.Error("expected the real admission diagnosis condition code on the nested observation's value")
	}

	// Discharge Medication (V3): act wrapper, fixed code, statusCode override
	// to "completed" (NOT the generic act default "active"), nested
	// Medication Activity (V2).
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.35" extension="2016-03-01"`) {
		t.Error("expected Discharge Medication (V3) act wrapper templateId in output")
	}
	if !strings.Contains(xml, `<code code="10183-2"`) {
		t.Error("expected the act wrapper's fixed code 10183-2 (Discharge medication)")
	}
	if !strings.Contains(xml, `code="197361"`) {
		t.Error("expected the real discharge medication drug code on the nested substanceAdministration")
	}

	// Admission Medication (V2): act wrapper, fixed code, generic "active"
	// default (no override needed/found for this one).
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.36" extension="2014-06-09"`) {
		t.Error("expected Admission Medication (V2) act wrapper templateId in output")
	}
	if !strings.Contains(xml, `<code code="42346-7"`) {
		t.Error("expected the act wrapper's fixed code 42346-7 (Medications on Admission)")
	}
	if !strings.Contains(xml, `code="861007"`) {
		t.Error("expected the real admission medication drug code on the nested substanceAdministration")
	}

	// The Discharge Medication act wrapper's own statusCode must be
	// "completed" — the EntryStatusCodeOverride this section needed,
	// confirmed from its own chapter (CONF:1198-32779/32780), distinct from
	// the generic "active" default the OTHER three act-wrapped sections
	// above correctly keep untouched.
	dmIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.35"`)
	if dmIdx == -1 {
		t.Fatal("expected to find the Discharge Medication templateId to check its enclosing act")
	}
	actStart := strings.LastIndex(xml[:dmIdx], "<act")
	if actStart == -1 {
		t.Fatal("expected an enclosing <act> before the Discharge Medication templateId")
	}
	actTail := xml[actStart:]
	if end := strings.Index(actTail, "</act>"); end != -1 {
		actTail = actTail[:end]
	}
	if !strings.Contains(actTail, `<statusCode code="completed"`) {
		t.Errorf("expected the Discharge Medication act wrapper's statusCode to be overridden to \"completed\", got: %s", actTail)
	}
}

// TestBuildDocument_HospitalCourseAndDischargePhysical_NarrativeOnly proves
// two more real bugs found this pass: both sections were previously
// modeled with a WRONG templateId (hospitalCourse was literally reusing
// Medications Administered Section's own templateId; dischargePhysical had
// fabricated entry-level fields reusing Result Observation's templateId)
// when both are actually pure-narrative sections per their own IG chapters
// (§2.27, §2.29) — no discrete entries at all, just templateId/code/title/
// text. Confirms both still build without error as narrative-only, SHALL
// sections with the CORRECTED (legacy IHE) template roots.
func TestBuildDocument_HospitalCourseAndDischargePhysical_NarrativeOnly(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			// dischargePhysical is MAY-conformance for Discharge Summary — it's
			// correctly omitted entirely when it has zero entries (same rule
			// proven by TestBuildDocument_EmptyShouldSection_Omitted), so a
			// single empty entry is supplied here purely to force its inclusion
			// and let this test inspect its (narrative-only) templateId.
			"dischargePhysical": map[string]interface{}{
				"entries": []interface{}{map[string]interface{}{}},
			},
		},
	}
	xml, err := BuildDocument(loader, doc, "Discharge Summary", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="1.3.6.1.4.1.19376.1.5.3.1.3.5"`) {
		t.Error("expected Hospital Course Section's real (IHE legacy) templateId, not Medications Administered Section's")
	}
	if !strings.Contains(xml, `root="1.3.6.1.4.1.19376.1.5.3.1.3.26"`) {
		t.Error("expected Hospital Discharge Physical Section's real (IHE legacy) templateId")
	}
	if strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.2" extension="2015-08-01"`) {
		t.Error("expected dischargePhysical to no longer emit Result Observation's templateId (the old copy-paste bug)")
	}
}

// TestBuildDocument_ReasonForReferral_CorrectActShape proves the
// reasonForReferral bug found this pass: its entryTemplateId
// (2.16.840.1.113883.10.20.22.4.22) didn't correspond to ANY real C-CDA 2.1
// template at all (confirmed via a full-text search of the IG), and its
// section templateId was actually Hospital Consultations Section's. The
// real entry archetype is Patient Referral Act (classCode="PCPR", no
// tagBoilerplate entry of its own, hence EntryClassCodeOverride).
func TestBuildDocument_ReasonForReferral_CorrectActShape(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"reasonForReferral": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"referralCode": "3457005", "referralText": "Cardiology consult"},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "Referral Note", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="1.3.6.1.4.1.19376.1.5.3.1.3.1" extension="2014-06-09"`) {
		t.Error("expected Reason for Referral Section's real (IHE legacy) templateId")
	}
	if !strings.Contains(xml, `<act classCode="PCPR"`) {
		t.Error("expected the Patient Referral Act's classCode override to PCPR")
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.140"`) {
		t.Error("expected the Patient Referral Act's real templateId")
	}
	if !strings.Contains(xml, `code="3457005"`) {
		t.Error("expected the real referral code in output")
	}
}

// TestBuildDocument_PastMedicalHistory_DirectObservationShape proves the
// pastMedicalHistory bug found this pass: it was modeled with an act+SUBJ
// wrapper shape (matching dischargeDiagnosis), but the real Past Medical
// History (V3) section's own constraint table shows entries as DIRECT
// Problem Observation (V3) entries (0..* MAY entry, 1..1 SHALL observation)
// — no wrapping act at all. Its section templateId was also confirmed wrong
// (matched what History of Present Illness now correctly uses instead).
func TestBuildDocument_PastMedicalHistory_DirectObservationShape(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"pastMedicalHistory": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"conditionCode": "195967001", "conditionCodeDisplay": "Asthma", "onsetDate": "20100101"},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "History and Physical", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.2.20" extension="2015-08-01"`) {
		t.Error("expected Past Medical History (V3) section's real templateId")
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.4" extension="2015-08-01"`) {
		t.Error("expected the direct Problem Observation (V3) entry templateId")
	}
	if !strings.Contains(xml, `<value xsi:type="CD" code="195967001"`) {
		t.Error("expected the real past condition code directly on the observation's value (no act wrapper)")
	}
	if strings.Contains(xml, `<act`) {
		t.Error("expected NO <act> wrapper for pastMedicalHistory — entries are direct observations per spec")
	}
}

// TestBuildDocument_AllergyNegationInd verifies the "no known allergies"
// pattern (negationInd="true" on the Allergy-Intolerance Observation,
// CONF:1098-31526) builds correctly.
func TestBuildDocument_AllergyNegationInd(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "160244002", "medicationAllergyCodeDisplay": "No known allergies",
					"negationInd": "true",
					"status":      "55561003",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `negationInd="true"`) {
		t.Errorf("expected negationInd=\"true\" in output, got: %s", xml)
	}
}

// TestBuildDocument_AllergyFixedCodeAndTypeAndResolutionDate verifies three
// real gaps found auditing Allergy - Intolerance Observation (V2) against
// the IG (§3.103.1): the observation's own SHALL <code code="ASSERTION"
// codeSystem="2.16.840.1.113883.5.4"> (CONF:1098-15947/15948/32153) was
// never written at all before this fix; its SHALL <value> (Allergy/
// Intolerance Type, e.g. "Allergy to substance", CONF:1098-7390) had no
// mappable field; and effectiveTime/high (the "resolution date",
// CONF:1098-31539) had no field either. All three are additive schema-only
// changes — zero engine code changed.
func TestBuildDocument_AllergyFixedCodeAndTypeAndResolutionDate(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "7980", "medicationAllergyCodeDisplay": "Penicillin",
					"allergyTypeCode": "419199007", "allergyTypeCodeDisplay": "Allergy to substance",
					"onsetDate": "20100101", "resolutionDate": "20150601",
					"status": "55561003",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `<code code="ASSERTION" codeSystem="2.16.840.1.113883.5.4"`) {
		t.Errorf("expected the Allergy-Intolerance Observation's fixed ASSERTION code, got: %s", xml)
	}
	if !strings.Contains(xml, `<value xsi:type="CD" code="419199007" displayName="Allergy to substance"`) {
		t.Errorf("expected the observation's own Allergy/Intolerance Type value, got: %s", xml)
	}
	// Substance reference chain (participant/participantRole/playingEntity)
	// needs 3 fixed attributes a real schema validator confirmed missing —
	// @typeCode="CSM" (Consumable), @classCode="MANU" (Manufactured Product),
	// @classCode="MMAT" (Manufactured Material).
	if !strings.Contains(xml, `<participant typeCode="CSM">`) {
		t.Errorf("expected participant/@typeCode=CSM, got: %s", xml)
	}
	if !strings.Contains(xml, `<participantRole classCode="MANU">`) {
		t.Errorf("expected participantRole/@classCode=MANU, got: %s", xml)
	}
	if !strings.Contains(xml, `<playingEntity classCode="MMAT">`) {
		t.Errorf("expected playingEntity/@classCode=MMAT, got: %s", xml)
	}
	if !strings.Contains(xml, `<low value="20100101"`) {
		t.Errorf("expected effectiveTime/low (onset), got: %s", xml)
	}
	if !strings.Contains(xml, `<high value="20150601"`) {
		t.Errorf("expected effectiveTime/high (resolution date), got: %s", xml)
	}
}

// TestBuildDocument_ImmunizationNegationInd verifies "immunization not
// given" (negationInd="true" on the Immunization Activity substance
// administration, CONF:1198-8985 — SHALL, unlike Problem/Allergy's MAY).
func TestBuildDocument_ImmunizationNegationInd(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"immunizations": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"vaccineCode": "88", "vaccineCodeDisplay": "Influenza virus vaccine",
					"administrationDate": "20231001",
					"negationInd":        "true",
					"status":             "completed",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `negationInd="true"`) {
		t.Errorf("expected negationInd=\"true\" in output, got: %s", xml)
	}
}

// TestBuildDocument_CustodianStructuredFields verifies the new structured
// CustodianOptions (id/extension, own org name, address, phone) all reach
// the built XML, and that a structured OrgName takes precedence over the
// legacy top-level BuildOptions.OrgName fallback.
func TestBuildDocument_CustodianStructuredFields(t *testing.T) {
	loader := loadTestSchema(t)
	opts := BuildOptions{
		OrgName: "Fallback Org", // must be overridden by Custodian.OrgName below
		Custodian: CustodianOptions{
			IdRoot: "2.16.840.1.113883.19.5", IdExtension: "CUST-001",
			OrgName: "Structured Health System",
			Street:  "1 Care Way", City: "Springfield", State: "IL", PostalCode: "62704", Country: "US",
			Phone: "5559876543",
		},
	}
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", opts)
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	idx := strings.Index(xml, "<custodian>")
	if idx == -1 {
		t.Fatal("expected a <custodian> element")
	}
	end := strings.Index(xml[idx:], "</custodian>")
	custodianXML := xml[idx : idx+end]

	if !strings.Contains(custodianXML, `root="2.16.840.1.113883.19.5"`) || !strings.Contains(custodianXML, `extension="CUST-001"`) {
		t.Errorf("expected structured custodian id root/extension, got: %s", custodianXML)
	}
	if !strings.Contains(custodianXML, "<name>Structured Health System</name>") {
		t.Errorf("expected Custodian.OrgName to take precedence over BuildOptions.OrgName, got: %s", custodianXML)
	}
	if !strings.Contains(custodianXML, "1 Care Way") || !strings.Contains(custodianXML, "Springfield") {
		t.Errorf("expected custodian address in output, got: %s", custodianXML)
	}
	if !strings.Contains(custodianXML, `tel:5559876543`) {
		t.Errorf("expected custodian phone in output, got: %s", custodianXML)
	}
}

// TestBuildDocument_CustodianTelecomBeforeAddr verifies representedCustodian
// Organization's child order is name, telecom, addr — the OPPOSITE of
// author/patient (both want addr before telecom) — a real schema validator
// found this reversed: "Invalid content was found starting with element
// 'telecom'. No child element is expected at this point," since addr was
// being written first.
func TestBuildDocument_TimezoneOffset_AppliedToDocumentAndAuthorTime(t *testing.T) {
	loader := loadTestSchema(t)
	opts := BuildOptions{TimezoneOffset: "-0500"}
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", opts)
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	recordTargetIdx := strings.Index(xml, "<recordTarget>")
	if recordTargetIdx == -1 {
		t.Fatalf("expected <recordTarget> in output")
	}
	headerXML := xml[:recordTargetIdx]
	docEffIdx := strings.Index(headerXML, "<effectiveTime value=\"")
	if docEffIdx == -1 {
		t.Fatalf("expected document-level effectiveTime before recordTarget, got: %s", headerXML)
	}
	docEffValue := headerXML[docEffIdx+len("<effectiveTime value=\"") : docEffIdx+len("<effectiveTime value=\"")+19]
	if !strings.HasSuffix(docEffValue, "-0500") {
		t.Errorf("expected document effectiveTime to carry -0500 offset, got %q", docEffValue)
	}

	authorIdx := strings.Index(xml, "<author>")
	if authorIdx == -1 {
		t.Fatalf("expected <author> in output")
	}
	authorEnd := strings.Index(xml[authorIdx:], "<assignedAuthor>")
	authorXML := xml[authorIdx : authorIdx+authorEnd]
	timeIdx := strings.Index(authorXML, "<time value=\"")
	if timeIdx == -1 {
		t.Fatalf("expected <time> under document author, got: %s", authorXML)
	}
	timeValue := authorXML[timeIdx+len("<time value=\"") : timeIdx+len("<time value=\"")+19]
	if !strings.HasSuffix(timeValue, "-0500") {
		t.Errorf("expected author time to carry -0500 offset, got %q", timeValue)
	}
}

func TestBuildDocument_TimezoneOffset_DefaultsToUTC(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	recordTargetIdx := strings.Index(xml, "<recordTarget>")
	headerXML := xml[:recordTargetIdx]
	docEffIdx := strings.Index(headerXML, "<effectiveTime value=\"")
	docEffValue := headerXML[docEffIdx+len("<effectiveTime value=\"") : docEffIdx+len("<effectiveTime value=\"")+19]
	if !strings.HasSuffix(docEffValue, "+0000") {
		t.Errorf("expected default document effectiveTime to carry +0000 (UTC) offset, got %q", docEffValue)
	}
}

// authorParticipationXMLAt extracts the <author>...</author> block whose
// start index is idx (already located by the caller) — a small shared
// helper for the Author Participation tests below, since several of them
// need to inspect more than one <author> instance in the same document.
func authorParticipationXMLAt(xml string, idx int) string {
	end := strings.Index(xml[idx:], "</author>")
	return xml[idx : idx+end+len("</author>")]
}

func TestBuildDocument_AuthorParticipation_ActAndNestedObservation(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"problems": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"conditionCode": "38341003", "conditionCodeDisplay": "Hypertension",
					"onsetDate": "20150304", "concernEffectiveTime": "20150304",
					"status":       "55561003",
					"authorGiven":  "Sarah",
					"authorFamily": "Kim",
				},
			}},
		},
	}
	fullXML, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	// Scope past the document header — it has its own unrelated <author>
	// element (the document's own author, from writeAuthorHeader), which
	// would otherwise be double-counted alongside the entry-level ones this
	// test actually cares about.
	bodyIdx := strings.Index(fullXML, "<structuredBody>")
	if bodyIdx == -1 {
		t.Fatalf("expected <structuredBody> in output")
	}
	xml := fullXML[bodyIdx:]

	authorCount := strings.Count(xml, "<author>")
	if authorCount != 2 {
		t.Fatalf("expected exactly 2 <author> elements (Concern Act + Problem Observation), got %d in: %s", authorCount, xml)
	}

	for _, idx := range []int{strings.Index(xml, "<author>"), strings.LastIndex(xml, "<author>")} {
		authorXML := authorParticipationXMLAt(xml, idx)
		if !strings.Contains(authorXML, `root="2.16.840.1.113883.10.20.22.4.119"`) {
			t.Errorf("expected Author Participation templateId, got: %s", authorXML)
		}
		if !strings.Contains(authorXML, `<time nullFlavor="UNK"/>`) {
			t.Errorf("expected placeholder <time nullFlavor=\"UNK\"/> (CDA Author requires time, canonical data never supplies one), got: %s", authorXML)
		}
		if strings.Contains(authorXML, "<id") {
			t.Errorf("author has no <id> in its CDA content model — must not be added, got: %s", authorXML)
		}
		if !strings.Contains(authorXML, "<given>Sarah</given>") || !strings.Contains(authorXML, "<family>Kim</family>") {
			t.Errorf("expected author name Sarah Kim, got: %s", authorXML)
		}
		// templateId must precede time, which must precede assignedAuthor.
		tidIdx := strings.Index(authorXML, "<templateId")
		timeIdx := strings.Index(authorXML, "<time ")
		assignedIdx := strings.Index(authorXML, "<assignedAuthor>")
		if !(tidIdx < timeIdx && timeIdx < assignedIdx) {
			t.Errorf("expected templateId, time, assignedAuthor in that order, got: %s", authorXML)
		}
	}
}

// TestBuildDocument_MedicationRouteCode_IncludesCodeSystem is the
// regression test for a real, previously-unmapped gap found while
// investigating the SITE validator's persistent routeCode DYNAMIC-valueset
// warnings: routeCode never had an xpathSystem at all, so @codeSystem was
// never written regardless of input data — confirmed against the real IG
// PDF's own worked example (Figure 168), which explicitly sets
// codeSystem="2.16.840.1.113883.3.26.1.1" (NCI Thesaurus) alongside the
// code. Doesn't guarantee the validator's own DYNAMIC valueset check
// passes (its own locally-cached snapshot may or may not include this
// code), but this closes a genuine, verifiable completeness gap regardless.
func TestBuildDocument_MedicationRouteCode_IncludesCodeSystem(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"medications": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"drugCode": "197361", "drugCodeDisplay": "Lisinopril 10 MG Oral Tablet",
					"status": "active", "startDate": "20230601",
					"routeCode": "C38288", "routeCodeDisplay": "Oral", "routeCodeSystem": "2.16.840.1.113883.3.26.1.1",
				},
			}},
		},
	}
	fullXML, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(fullXML, `<routeCode code="C38288" displayName="Oral" codeSystem="2.16.840.1.113883.3.26.1.1" codeSystemName="NCI Thesaurus"/>`) {
		t.Errorf("expected routeCode with codeSystem + auto-derived codeSystemName, got: %s", fullXML)
	}
}

func TestBuildDocument_AuthorParticipation_SubstanceAdministration(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"medications": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"drugCode": "197361", "drugCodeDisplay": "Lisinopril 10 MG Oral Tablet",
					"status": "active", "startDate": "20230601",
					"authorGiven": "Sarah", "authorFamily": "Kim", "authorNPI": "1730123456",
				},
			}},
		},
	}
	fullXML, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	bodyIdx := strings.Index(fullXML, "<structuredBody>")
	if bodyIdx == -1 {
		t.Fatalf("expected <structuredBody> in output")
	}
	xml := fullXML[bodyIdx:]
	idx := strings.Index(xml, "<author>")
	if idx == -1 {
		t.Fatalf("expected an <author> element on Medication Activity, got: %s", xml)
	}
	authorXML := authorParticipationXMLAt(xml, idx)
	if !strings.Contains(authorXML, `root="2.16.840.1.113883.10.20.22.4.119"`) {
		t.Errorf("expected Author Participation templateId, got: %s", authorXML)
	}
	if !strings.Contains(authorXML, `<id root="2.16.840.1.113883.4.6" extension="1730123456"/>`) {
		t.Errorf("expected author NPI id to carry BOTH root (NPI OID) and extension — a bare id/@extension with no root is schema-incomplete, got: %s", authorXML)
	}
	// assignedAuthor's own sequence requires id BEFORE assignedPerson —
	// authorNPI is written by a plain field-driven WriteAtXPath AFTER
	// authorGiven/authorFamily already created assignedPerson, so creation
	// order alone would put id last without ensureAuthorParticipationShape's
	// explicit reorder.
	idIdx := strings.Index(authorXML, "<id ")
	personIdx := strings.Index(authorXML, "<assignedPerson>")
	if idIdx == -1 || personIdx == -1 || idIdx > personIdx {
		t.Errorf("expected <id> before <assignedPerson> in assignedAuthor, got: %s", authorXML)
	}
}

// TestBuildDocument_ObservationAuthor_PrecedesEntryRelationship_RegardlessOfFieldOrder
// is the regression test for a real bug found via a live Test Pipeline run
// (2026-07): problems.fields[] lists status (creates an entryRelationship
// sibling for Problem Status) BEFORE the nested observation's own author
// fields — construction order alone (both tags unlisted in
// structuralElementOrder at the time) left <entryRelationship> before
// <author> in the final output, which every CDA base type's fixed sequence
// forbids (author always precedes entryRelationship/participant/
// precondition/referenceRange/reference). Fixed by adding "author" to
// structuralElementOrder so it sorts correctly regardless of which field
// happened to run first.
func TestBuildDocument_ObservationAuthor_PrecedesEntryRelationship_RegardlessOfFieldOrder(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"problems": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"conditionCode": "38341003", "conditionCodeDisplay": "Hypertension",
					"onsetDate": "20150304", "concernEffectiveTime": "20150304",
					"status":       "55561003", // creates the Problem Status entryRelationship
					"authorGiven":  "Sarah",    // observation-level author — listed AFTER status in the schema
					"authorFamily": "Kim",
				},
			}},
		},
	}
	fullXML, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	bodyIdx := strings.Index(fullXML, "<structuredBody>")
	xml := fullXML[bodyIdx:]

	// The SECOND <author> in the document is the Problem Observation's own
	// (the first is the Concern Act's) — confirm it precedes the
	// entryRelationship for Problem Status within the SAME observation.
	authorIdx := strings.LastIndex(xml, "<author>")
	entryRelIdx := strings.Index(xml, `<entryRelationship typeCode="REFR">`)
	if authorIdx == -1 || entryRelIdx == -1 {
		t.Fatalf("expected both <author> and <entryRelationship typeCode=\"REFR\"> in output, got: %s", xml)
	}
	if authorIdx > entryRelIdx {
		t.Errorf("expected <author> BEFORE <entryRelationship> (CDA base type sequence), got author at %d, entryRelationship at %d: %s", authorIdx, entryRelIdx, xml)
	}
}

// TestBuildDocument_ProblemStatusText_PrecedesStatusCodeAndValue is the
// regression test for a real bug found via a live Test Pipeline run
// (2026-07): Problem Status's own statusText field is listed in the schema
// AFTER the fields that create value/interpretationCode, so construction
// order alone put <text> after <value> — invalid per CDA's fixed sequence
// (code, text?, statusCode, effectiveTime?, value*, ...). Fixed by adding
// "text" to structuralElementOrder, positioned right after "code".
func TestBuildDocument_ProblemStatusText_PrecedesStatusCodeAndValue(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"problems": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"conditionCode": "38341003", "conditionCodeDisplay": "Hypertension",
					"onsetDate": "20150304", "concernEffectiveTime": "20150304",
					"status": "55561003", "statusText": "Confirmed diagnosis.",
				},
			}},
		},
	}
	fullXML, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	idx := strings.Index(fullXML, "33999-4") // Problem Status's own fixed code
	if idx == -1 {
		t.Fatalf("expected Problem Status observation in output")
	}
	end := strings.Index(fullXML[idx:], "</observation>")
	statusObsXML := fullXML[idx : idx+end]
	textIdx := strings.Index(statusObsXML, "<text>")
	statusCodeIdx := strings.Index(statusObsXML, "<statusCode")
	valueIdx := strings.Index(statusObsXML, "<value")
	if textIdx == -1 || statusCodeIdx == -1 || valueIdx == -1 {
		t.Fatalf("expected text, statusCode, and value all present, got: %s", statusObsXML)
	}
	if !(textIdx < statusCodeIdx && statusCodeIdx < valueIdx) {
		t.Errorf("expected order text < statusCode < value, got: %s", statusObsXML)
	}
}

// TestBuildDocument_AllergyStatusValue_UsesCEDataTypeWithCodeSystem is the
// regression test for a real spec bug (not an engine bug): Allergy Status
// Observation's own <value> requires xsi:type="CE" per the IG's own worked
// example (Figure 158) — Problem Status uses "CD" instead (Figure 216, a
// genuinely different archetype), confirmed directly from the real R1.1 IG
// PDF text, not recall — the schema previously used "CD" for both.
func TestBuildDocument_AllergyStatusValue_UsesCEDataTypeWithCodeSystem(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "7980", "medicationAllergyCodeDisplay": "Penicillin",
					"status": "55561003", "statusSystem": "2.16.840.1.113883.6.96",
					"onsetDate": "20100604",
				},
			}},
		},
	}
	fullXML, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(fullXML, `xsi:type="CE" code="55561003" codeSystem="2.16.840.1.113883.6.96"`) {
		t.Errorf("expected Allergy Status value with xsi:type=CE and codeSystem, got: %s", fullXML)
	}
}

// TestBuildDocument_AdvanceDirectiveCustodian_TelecomAndCodePrecedePlayingEntity
// is the regression test for a real bug found via a live Test Pipeline run
// (2026-07): participantRole is never a StructuralTemplateAnchor target, so
// it's never explicitly reordered — its children's final order is purely
// whichever order the schema's fields[] array happens to write them in.
// custodianName (playingEntity/name) was listed before custodianPhone
// (participantRole/telecom) and custodianRelationshipCode
// (participantRole/code), so telecom/code landed AFTER playingEntity —
// invalid per ParticipantRole's fixed sequence (id*, code?, addr*, telecom*,
// playingEntity). Fixed by reordering the schema fields, not the engine.
func TestBuildDocument_AdvanceDirectiveCustodian_TelecomAndCodePrecedePlayingEntity(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"advanceDirectives": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"directiveCode": "304251008", "directiveCodeDisplay": "Resuscitation",
					"value": "425392003", "valueDisplay": "Do not resuscitate",
					"effectiveTimeLow": "20240101", "effectiveTimeHigh": "20991231",
					"custodianName": "Robert Chen", "custodianPhone": "2175551234",
					"custodianRelationshipCode": "63450004", "custodianRelationshipCodeDisplay": "Family member",
				},
			}},
		},
	}
	fullXML, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	idx := strings.Index(fullXML, `typeCode="CST"`)
	if idx == -1 {
		t.Fatalf("expected Custodian participant in output")
	}
	end := strings.Index(fullXML[idx:], "</participant>")
	participantXML := fullXML[idx : idx+end]
	codeIdx := strings.Index(participantXML, "<code")
	telecomIdx := strings.Index(participantXML, "<telecom")
	playingEntityIdx := strings.Index(participantXML, "<playingEntity>")
	if codeIdx == -1 || telecomIdx == -1 || playingEntityIdx == -1 {
		t.Fatalf("expected code, telecom, and playingEntity all present, got: %s", participantXML)
	}
	if !(codeIdx < telecomIdx && telecomIdx < playingEntityIdx) {
		t.Errorf("expected order code < telecom < playingEntity, got: %s", participantXML)
	}
}

// TestBuildDocument_AdvanceDirectiveVerifier_NoStrayID is the regression
// test for a real bug found via a live Test Pipeline run (2026-07):
// Participant2 (the type backing a bare <participant> like Verifier) has NO
// <id> in its content model, unlike act/observation/organizer — the
// StructuralTemplateIDs loop's ensureGeneratedID call was adding one anyway.
func TestBuildDocument_AdvanceDirectiveVerifier_NoStrayID(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"advanceDirectives": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"directiveCode": "304251008", "directiveCodeDisplay": "Resuscitation",
					"value": "425392003", "valueDisplay": "Do not resuscitate",
					"effectiveTimeLow": "20240101", "effectiveTimeHigh": "20991231",
					"verifierGiven": "Michael", "verifierFamily": "Chen",
				},
			}},
		},
	}
	fullXML, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	idx := strings.Index(fullXML, `typeCode="VRF"`)
	if idx == -1 {
		t.Fatalf("expected Verifier participant in output")
	}
	end := strings.Index(fullXML[idx:], "</participant>")
	verifierXML := fullXML[idx : idx+end]
	if !strings.Contains(verifierXML, `root="2.16.840.1.113883.10.20.1.58"`) {
		t.Errorf("expected Verifier's own required templateId, got: %s", verifierXML)
	}
	if strings.Contains(verifierXML, "<id") {
		t.Errorf("Participant2 has no <id> in its content model — must not be added, got: %s", verifierXML)
	}
}

func TestBuildDocument_AuthorParticipation_OrganizerAndRepeatingGroupItem(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"vitalSigns": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"effectiveTime": "20260101090000",
					"authorGiven":   "Sarah", "authorFamily": "Kim",
					"components": []interface{}{
						map[string]interface{}{
							"vitalCode": "8480-6", "vitalCodeDisplay": "Systolic Blood Pressure",
							"value": "128", "valueUnit": "mm[Hg]", "componentEffectiveTime": "20260101090000",
							"authorGiven": "Alex", "authorFamily": "Rivera",
						},
					},
				},
			}},
		},
	}
	fullXML, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	bodyIdx := strings.Index(fullXML, "<structuredBody>")
	if bodyIdx == -1 {
		t.Fatalf("expected <structuredBody> in output")
	}
	xml := fullXML[bodyIdx:]

	authorCount := strings.Count(xml, "<author>")
	if authorCount != 2 {
		t.Fatalf("expected 2 <author> elements (organizer + its one component), got %d in: %s", authorCount, xml)
	}

	orgAuthorXML := authorParticipationXMLAt(xml, strings.Index(xml, "<author>"))
	if !strings.Contains(orgAuthorXML, "<given>Sarah</given>") {
		t.Errorf("expected organizer-level author Sarah, got: %s", orgAuthorXML)
	}

	compAuthorXML := authorParticipationXMLAt(xml, strings.LastIndex(xml, "<author>"))
	if !strings.Contains(compAuthorXML, "<given>Alex</given>") || !strings.Contains(compAuthorXML, "<family>Rivera</family>") {
		t.Errorf("expected component-level author Alex Rivera (independent from the organizer's own author), got: %s", compAuthorXML)
	}
	if !strings.Contains(compAuthorXML, `root="2.16.840.1.113883.10.20.22.4.119"`) {
		t.Errorf("expected Author Participation templateId on the component's own author, got: %s", compAuthorXML)
	}
}

func TestBuildDocument_CustodianTelecomBeforeAddr(t *testing.T) {
	loader := loadTestSchema(t)
	opts := BuildOptions{
		Custodian: CustodianOptions{
			OrgName: "Structured Health System",
			Street:  "1 Care Way", City: "Springfield", State: "IL", PostalCode: "62704", Country: "US",
			Phone: "5559876543",
		},
	}
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", opts)
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	idx := strings.Index(xml, "<representedCustodianOrganization>")
	end := strings.Index(xml[idx:], "</representedCustodianOrganization>")
	orgXML := xml[idx : idx+end]
	telIdx := strings.Index(orgXML, "<telecom")
	addrIdx := strings.Index(orgXML, "<addr ")
	if telIdx == -1 || addrIdx == -1 {
		t.Fatalf("expected both telecom and addr present, got: %s", orgXML)
	}
	if telIdx > addrIdx {
		t.Errorf("expected telecom BEFORE addr (CustodianOrganization's own sequence), got: %s", orgXML)
	}
}

// TestBuildDocument_LegalAuthenticator_AbsentByDefault verifies that with a
// zero-value LegalAuthenticatorOptions (every pipeline predating this
// feature), no <legalAuthenticator> element is written at all — unlike
// Custodian (SHALL exactly one, always written), this element is genuinely
// optional (SHOULD 0..1, CONF:1198-5579) and an incomplete one would be a
// schema violation worse than omitting it.
func TestBuildDocument_LegalAuthenticator_AbsentByDefault(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if strings.Contains(xml, "<legalAuthenticator>") {
		t.Errorf("expected no <legalAuthenticator> when LegalAuthenticator is unset, got it present in: %s", xml)
	}
}

// TestBuildDocument_LegalAuthenticator_FullConfig verifies the full
// <legalAuthenticator><time/><signatureCode code="S"/><assignedEntity>
// structure and child order (id, code, addr, telecom, assignedPerson — the
// SAME order as assignedAuthor, unlike representedCustodianOrganization's
// reversed telecom-before-addr) once a name is configured, per Figure 10's
// worked example and CONF:1198-5580/5583/5585/5586/5589/5595/5597.
func TestBuildDocument_LegalAuthenticator_FullConfig(t *testing.T) {
	loader := loadTestSchema(t)
	opts := BuildOptions{
		LegalAuthenticator: LegalAuthenticatorOptions{
			Given: "Patricia", Family: "Primary", NPI: "5555555555",
			SpecialtyCode: "207QA0505X", SpecialtyCodeSystem: "2.16.840.1.113883.6.101", SpecialtyCodeDisplay: "Adult Medicine",
			Street: "1004 Healthcare Drive", City: "Portland", State: "OR", PostalCode: "99123", Country: "US",
			Phone: "5555551004",
		},
	}
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", opts)
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	idx := strings.Index(xml, "<legalAuthenticator>")
	if idx == -1 {
		t.Fatal("expected a <legalAuthenticator> element")
	}
	end := strings.Index(xml[idx:], "</legalAuthenticator>")
	laXML := xml[idx : idx+end]

	if !strings.Contains(laXML, "<time value=") {
		t.Errorf("expected <time> in legalAuthenticator, got: %s", laXML)
	}
	if !strings.Contains(laXML, `<signatureCode code="S"/>`) {
		t.Errorf(`expected <signatureCode code="S"/>, got: %s`, laXML)
	}
	if !strings.Contains(laXML, `root="2.16.840.1.113883.4.6"`) || !strings.Contains(laXML, `extension="5555555555"`) {
		t.Errorf("expected NPI-rooted id, got: %s", laXML)
	}
	if !strings.Contains(laXML, `code="207QA0505X"`) || !strings.Contains(laXML, `codeSystem="2.16.840.1.113883.6.101"`) {
		t.Errorf("expected specialty code+codeSystem, got: %s", laXML)
	}
	if !strings.Contains(laXML, "1004 Healthcare Drive") || !strings.Contains(laXML, "Portland") {
		t.Errorf("expected address, got: %s", laXML)
	}
	if !strings.Contains(laXML, "tel:5555551004") {
		t.Errorf("expected telecom, got: %s", laXML)
	}
	if !strings.Contains(laXML, "<given>Patricia</given>") || !strings.Contains(laXML, "<family>Primary</family>") {
		t.Errorf("expected assignedPerson name, got: %s", laXML)
	}

	idIdx := strings.Index(laXML, "<id ")
	codeIdx := strings.Index(laXML, "<code ")
	addrIdx := strings.Index(laXML, "<addr ")
	telIdx := strings.Index(laXML, "<telecom")
	personIdx := strings.Index(laXML, "<assignedPerson>")
	if !(idIdx < codeIdx && codeIdx < addrIdx && addrIdx < telIdx && telIdx < personIdx) {
		t.Errorf("expected assignedEntity child order id, code, addr, telecom, assignedPerson, got: %s", laXML)
	}

	// Schema position: right after custodian, before documentationOf.
	custodianEnd := strings.Index(xml, "</custodian>")
	if custodianEnd == -1 || custodianEnd > idx {
		t.Errorf("expected legalAuthenticator to come after custodian, got: %s", xml)
	}
}

// TestBuildDocument_AuthorAddressAndTelecom verifies assignedAuthor's addr
// and telecom — confirmed completely unsupported (not just unmapped) via a
// real schema validator run (2026-07): US Realm Header SHALL contain at
// least one addr (CONF:5452) and telecom (CONF:5428) on assignedAuthor, and
// neither had any write path before this fix. Also verifies the correct
// schema child order: id, addr, telecom, assignedPerson,
// representedOrganization — addr/telecom are written AFTER assignedPerson
// by construction, so this also exercises the reorder pass.
func TestBuildDocument_AuthorAddressAndTelecom(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{
			"author": map[string]interface{}{
				"given": "John", "family": "Smith", "npi": "1234567890",
				"address": map[string]interface{}{"street": "500 Clinic Rd", "city": "Springfield", "state": "IL", "postalCode": "62704"},
				"phone":   "2175559999",
			},
		},
		"sections": map[string]interface{}{},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	idx := strings.Index(xml, "<assignedAuthor>")
	if idx == -1 {
		t.Fatal("expected an <assignedAuthor> element")
	}
	end := strings.Index(xml[idx:], "</assignedAuthor>")
	authorXML := xml[idx : idx+end]

	if !strings.Contains(authorXML, "500 Clinic Rd") || !strings.Contains(authorXML, "Springfield") {
		t.Errorf("expected author address in output, got: %s", authorXML)
	}
	if !strings.Contains(authorXML, "tel:2175559999") {
		t.Errorf("expected author phone in output, got: %s", authorXML)
	}

	idIdx := strings.Index(authorXML, "<id")
	addrIdx := strings.Index(authorXML, "<addr ")
	telIdx := strings.Index(authorXML, "<telecom")
	personIdx := strings.Index(authorXML, "<assignedPerson>")
	orgIdx := strings.Index(authorXML, "<representedOrganization>")
	if idIdx == -1 || addrIdx == -1 || telIdx == -1 || personIdx == -1 || orgIdx == -1 {
		t.Fatalf("expected id, addr, telecom, assignedPerson, and representedOrganization all present, got: %s", authorXML)
	}
	if !(idIdx < addrIdx && addrIdx < telIdx && telIdx < personIdx && personIdx < orgIdx) {
		t.Errorf("expected order id < addr < telecom < assignedPerson < representedOrganization, got indices %d,%d,%d,%d,%d: %s",
			idIdx, addrIdx, telIdx, personIdx, orgIdx, authorXML)
	}
}

// TestBuildDocument_RealmCodeAndTypeIdPresent verifies two elements a real
// schema validator found completely missing from the ClinicalDocument root
// (2026-07): realmCode and typeId, both required before templateId per
// CDA's own schema sequence — their absence made a real validator flag
// nearly the entire document as invalid, since every element after the
// first templateId no longer matched the expected sequence.
func TestBuildDocument_RealmCodeAndTypeIdPresent(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `<realmCode code="US"/>`) {
		t.Errorf("expected <realmCode code=\"US\"/>, got: %s", xml)
	}
	if !strings.Contains(xml, `<typeId root="2.16.840.1.113883.1.3" extension="POCD_HD000040"/>`) {
		t.Errorf("expected <typeId root=\"2.16.840.1.113883.1.3\" extension=\"POCD_HD000040\"/>, got: %s", xml)
	}
	realmIdx := strings.Index(xml, "<realmCode")
	typeIdIdx := strings.Index(xml, "<typeId")
	templateIdx := strings.Index(xml, "<templateId")
	if realmIdx == -1 || typeIdIdx == -1 || templateIdx == -1 {
		t.Fatal("expected realmCode, typeId, and templateId all present")
	}
	if !(realmIdx < typeIdIdx && typeIdIdx < templateIdx) {
		t.Errorf("expected order realmCode < typeId < templateId, got indices %d, %d, %d", realmIdx, typeIdIdx, templateIdx)
	}
}

// TestBuildDocument_PatientElementOrder verifies <patient>'s children are
// ordered per CDA's schema sequence (name, administrativeGenderCode,
// birthTime, ..., languageCommunication) — a real schema validator found
// <birthTime> written before <name>, which is invalid (name must come
// first). The two are written via two independent field-mapping tables
// (patientScalarFields vs patientCodedFields), so this is verified with a
// final reorder pass rather than call-order choreography.
func TestBuildDocument_PatientElementOrder(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	patientIdx := strings.Index(xml, "<patient>")
	if patientIdx == -1 {
		t.Fatal("expected a <patient> element")
	}
	patientEnd := strings.Index(xml[patientIdx:], "</patient>")
	patientXML := xml[patientIdx : patientIdx+patientEnd]

	nameIdx := strings.Index(patientXML, "<name>")
	genderIdx := strings.Index(patientXML, "<administrativeGenderCode")
	birthIdx := strings.Index(patientXML, "<birthTime")
	langIdx := strings.Index(patientXML, "<languageCommunication")
	if nameIdx == -1 || genderIdx == -1 || birthIdx == -1 || langIdx == -1 {
		t.Fatalf("expected name, administrativeGenderCode, birthTime, and languageCommunication all present, got: %s", patientXML)
	}
	if !(nameIdx < genderIdx && genderIdx < birthIdx && birthIdx < langIdx) {
		t.Errorf("expected order name < administrativeGenderCode < birthTime < languageCommunication, got indices %d, %d, %d, %d", nameIdx, genderIdx, birthIdx, langIdx)
	}
}

// TestBuildDocument_ManufacturedProductTemplateIdFirst verifies
// manufacturedProduct's <templateId> comes before <manufacturedMaterial> —
// a real schema validator found the reverse (templateId appended AFTER
// manufacturedMaterial, since its StructuralTemplateIDs anchor only
// resolves via TryFindAtXPath once the plain drugCode field write already
// created manufacturedMaterial as manufacturedProduct's first child).
func TestBuildDocument_ManufacturedProductTemplateIdFirst(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	// manufacturedProduct now carries classCode="MANU" (CONF:7408/1098-7408 —
	// a separate real gap found by a later validator run) via a predicate on
	// drugCode's own xpath, so the tag no longer serializes bare.
	prodIdx := strings.Index(xml, `<manufacturedProduct classCode="MANU">`)
	if prodIdx == -1 {
		t.Fatal("expected a <manufacturedProduct classCode=\"MANU\"> element")
	}
	prodEnd := strings.Index(xml[prodIdx:], "</manufacturedProduct>")
	prodXML := xml[prodIdx : prodIdx+prodEnd]

	tidIdx := strings.Index(prodXML, "<templateId")
	matIdx := strings.Index(prodXML, "<manufacturedMaterial")
	if tidIdx == -1 || matIdx == -1 {
		t.Fatalf("expected templateId and manufacturedMaterial both present, got: %s", prodXML)
	}
	if tidIdx > matIdx {
		t.Errorf("expected manufacturedProduct's templateId before manufacturedMaterial, got: %s", prodXML)
	}
}

// TestBuildDocument_EveryEntryGetsGeneratedID verifies every act/observation/
// organizer/substanceAdministration instance gets a system-generated <id> —
// a real schema validator found this missing across the board (patientRole
// aside, which correctly comes from real mapped data, not auto-generation).
// Covers all 4 injection points: rootEl, obsEl, a StructuralTemplateIDs
// anchor (Reaction Observation), and a RepeatingGroup item (Vital Sign
// Observation).
func TestBuildDocument_EveryEntryGetsGeneratedID(t *testing.T) {
	loader := loadTestSchema(t)
	xml, err := BuildDocument(loader, fullCanonicalDoc(), "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	// Allergy act (rootEl) + nested Allergy Observation (obsEl) + Reaction
	// Observation (StructuralTemplateIDs anchor).
	allergyIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.30"`)
	if allergyIdx == -1 {
		t.Fatal("expected Allergy Concern Act templateId in output")
	}
	allergyEnd := strings.Index(xml[allergyIdx:], "</act>")
	allergyXML := xml[allergyIdx : allergyIdx+allergyEnd]
	if strings.Count(allergyXML, "<id root=") < 3 {
		t.Errorf("expected at least 3 generated <id> elements (act, observation, reaction observation), got %d in: %s",
			strings.Count(allergyXML, "<id root="), allergyXML)
	}

	// Medication substanceAdministration (rootEl).
	medIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.16"`)
	if medIdx == -1 {
		t.Fatal("expected Medication Activity templateId in output")
	}
	medTail := xml[medIdx : medIdx+300]
	if !strings.Contains(medTail, "<id root=") {
		t.Errorf("expected a generated <id> on the Medication Activity substanceAdministration, got: %s", medTail)
	}

	// RepeatingGroup item id-generation (Vital Sign Observation, Result
	// Observation, ...) is covered by TestBuildEntry_EveryAnchorGetsGeneratedID
	// instead — fullCanonicalDoc's vitalSigns fixture uses the pre-RepeatingGroup
	// flat shape (no "components" array), so it produces zero vitalSigns
	// entries under the current schema and can't exercise that path here.

	// patientRole must NOT get an auto-generated id — its id comes from real
	// mapped data (fullCanonicalDoc's "ids" array), never fabricated.
	patientRoleIdx := strings.Index(xml, "<patientRole>")
	patientRoleEnd := strings.Index(xml[patientRoleIdx:], "<patient>")
	patientRoleXML := xml[patientRoleIdx : patientRoleIdx+patientRoleEnd]
	if !strings.Contains(patientRoleXML, `root="2.16.840.1.113883.4.1" extension="PAT-001"`) {
		t.Errorf("expected patientRole's real mapped id (not a generated one), got: %s", patientRoleXML)
	}
}

// TestBuildDocument_ConcernActOwnCodeAndEffectiveTime verifies the Allergy/
// Problem Concern Act (the OUTER <act> wrapping the nested assertion
// observation) gets its own SHALL <code code="CONC"> (CONF:1198-7477/9027)
// and its own SHALL <effectiveTime><low> (CONF:7498/9030, and CONF:7504 when
// statusCode is "active") — a real schema validator found BOTH completely
// absent: this engine only ever wrote the NESTED observation's own code/
// effectiveTime, never the wrapping Concern Act's.
func TestBuildDocument_ConcernActOwnCodeAndEffectiveTime(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "7980", "medicationAllergyCodeDisplay": "Penicillin",
					"allergyTypeCode": "419199007", "allergyTypeCodeDisplay": "Allergy to substance",
					"onsetDate": "20100604", "concernEffectiveTime": "20100604",
					"status": "active",
				},
			}},
			"problems": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"conditionCode": "E11.9", "conditionCodeDisplay": "Type 2 diabetes",
					"onsetDate": "20200601", "concernEffectiveTime": "20200601",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	allergyActIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.30"`)
	if allergyActIdx == -1 {
		t.Fatal("expected Allergy Concern Act templateId in output")
	}
	allergyActEnd := strings.Index(xml[allergyActIdx:], "<entryRelationship")
	allergyActXML := xml[allergyActIdx : allergyActIdx+allergyActEnd]
	if !strings.Contains(allergyActXML, `<code code="CONC" codeSystem="2.16.840.1.113883.5.6" displayName="Concern"/>`) {
		t.Errorf("expected Allergy Concern Act's own fixed CONC code, got: %s", allergyActXML)
	}
	if !strings.Contains(allergyActXML, `<effectiveTime xsi:type="IVL_TS">`) || !strings.Contains(allergyActXML, `<low value="20100604"/>`) {
		t.Errorf("expected Allergy Concern Act's own effectiveTime/low, got: %s", allergyActXML)
	}

	problemActIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.3"`)
	if problemActIdx == -1 {
		t.Fatal("expected Problem Concern Act templateId in output")
	}
	problemActEnd := strings.Index(xml[problemActIdx:], "<entryRelationship")
	problemActXML := xml[problemActIdx : problemActIdx+problemActEnd]
	if !strings.Contains(problemActXML, `<code code="CONC" codeSystem="2.16.840.1.113883.5.6" displayName="Concern"/>`) {
		t.Errorf("expected Problem Concern Act's own fixed CONC code, got: %s", problemActXML)
	}
	if !strings.Contains(problemActXML, `<low value="20200601"/>`) {
		t.Errorf("expected Problem Concern Act's own effectiveTime/low, got: %s", problemActXML)
	}
}

// TestBuildDocument_ObsAndSubAdminChildOrdering verifies the structural
// ordering fix that extended structuralElementOrder with effectiveTime/
// value/routeCode/doseQuantity/consumable — a real schema validator found
// this engine's fields-loop iteration order (JSON array order) leaking
// straight into XML output order, producing invalid sequences: <consumable>
// before <doseQuantity>/<routeCode> on Medication Activity, and
// <participant>/<entryRelationship> before <effectiveTime>/<value> on the
// Allergy assertion observation.
func TestBuildDocument_ObsAndSubAdminChildOrdering(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "7980", "medicationAllergyCodeDisplay": "Penicillin",
					"allergyTypeCode": "419199007", "onsetDate": "20100604",
				},
			}},
			"medications": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"drugCode": "197361", "drugCodeDisplay": "Lisinopril",
					"doseQuantity": "10", "routeCode": "C38288", "startDate": "20230601",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	obsIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.7"`)
	obsEnd := strings.Index(xml[obsIdx:], "</observation>")
	obsXML := xml[obsIdx : obsIdx+obsEnd]
	effIdx := strings.Index(obsXML, "<effectiveTime")
	valIdx := strings.Index(obsXML, "<value")
	partIdx := strings.Index(obsXML, "<participant")
	if effIdx == -1 || valIdx == -1 || partIdx == -1 {
		t.Fatalf("expected effectiveTime, value, and participant all present, got: %s", obsXML)
	}
	if !(effIdx < valIdx && valIdx < partIdx) {
		t.Errorf("expected order effectiveTime < value < participant, got effIdx=%d valIdx=%d partIdx=%d in: %s", effIdx, valIdx, partIdx, obsXML)
	}

	subAdminIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.16"`)
	subAdminEnd := strings.Index(xml[subAdminIdx:], "</substanceAdministration>")
	subAdminXML := xml[subAdminIdx : subAdminIdx+subAdminEnd]
	effIdx2 := strings.Index(subAdminXML, "<effectiveTime")
	routeIdx := strings.Index(subAdminXML, "<routeCode")
	doseIdx := strings.Index(subAdminXML, "<doseQuantity")
	consIdx := strings.Index(subAdminXML, "<consumable")
	if effIdx2 == -1 || routeIdx == -1 || doseIdx == -1 || consIdx == -1 {
		t.Fatalf("expected effectiveTime, routeCode, doseQuantity, and consumable all present, got: %s", subAdminXML)
	}
	if !(effIdx2 < routeIdx && routeIdx < doseIdx && doseIdx < consIdx) {
		t.Errorf("expected order effectiveTime < routeCode < doseQuantity < consumable, got %d %d %d %d in: %s",
			effIdx2, routeIdx, doseIdx, consIdx, subAdminXML)
	}
}

// TestBuildDocument_CodeTranslations verifies the new EntryCodeTranslation/
// ObsCodeTranslation mechanism writes a <translation> child under whatever
// <code> ends up on the root/nested-observation element — Advance Directive
// Observation (CONF:1198-32842, fixed regardless of directiveCode's real
// value), Mental Status Observation (CONF:1198-32790, under EntryFixedCode),
// and Problem Observation (CONF:1098-32950, under ObsFixedCode) all SHALL
// have one; a real schema validator found all three missing.
func TestBuildDocument_CodeTranslations(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"problems": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"conditionCode": "E11.9", "conditionCodeDisplay": "Diabetes", "onsetDate": "20200601"},
			}},
			"advanceDirectives": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"directiveCode": "304251008", "value": "425392003",
					"effectiveTimeLow": "20240101", "effectiveTimeHigh": "20991231",
				},
			}},
			"mentalStatus": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"value": "248234008", "effectiveTime": "20260101"},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	if !strings.Contains(xml, `<translation code="75325-1" codeSystem="2.16.840.1.113883.6.1" displayName="Problem"/>`) {
		t.Errorf("expected Problem Observation's translation to LOINC 75325-1 (\"Problem HL7.CCDAR2\", the one unambiguous, unshifted row in the IG's own Table 325), got: %s", xml)
	}
	if !strings.Contains(xml, `<translation code="75320-2" codeSystem="2.16.840.1.113883.6.1" displayName="Advance directive"/>`) {
		t.Errorf("expected Advance Directive Observation's translation to LOINC 75320-2, got: %s", xml)
	}
	if !strings.Contains(xml, `<translation code="75275-8" codeSystem="2.16.840.1.113883.6.1" displayName="Cognitive Function"/>`) {
		t.Errorf("expected Mental Status Observation's translation to LOINC 75275-8, got: %s", xml)
	}
}

// TestBuildDocument_ParticipantTypeCodes verifies the two participant
// typeCode gaps a real schema validator found: Advance Directive
// Observation's custodian participant SHALL @typeCode="CST" (CONF:1198-
// 8662), and Non-Medicinal Supply Activity's device participant SHALL
// @typeCode="PRD" (CONF:1098-8754) — both were being created as bare
// <participant> with no typeCode at all.
func TestBuildDocument_ParticipantTypeCodes(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"advanceDirectives": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"directiveCode": "304251008", "value": "425392003",
					"effectiveTimeLow": "20240101", "effectiveTimeHigh": "20991231",
					"custodianName": "Robert Chen",
				},
			}},
			"medicalEquipment": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{"equipmentCode": "360008007", "equipmentCodeDisplay": "Wheelchair", "status": "completed"},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `<participant typeCode="CST">`) {
		t.Errorf("expected Advance Directive's custodian participant typeCode=CST, got: %s", xml)
	}
	if !strings.Contains(xml, `<participant typeCode="PRD">`) {
		t.Errorf("expected Non-Medicinal Supply Activity's device participant typeCode=PRD, got: %s", xml)
	}
}

// TestBuildDocument_AssignedEntityAlwaysGetsID verifies every <assignedEntity>
// created by an entry-level performer field (e.g. procedures.performerGiven/
// Family) gets at least one <id> even when no NPI was mapped — CDA's base
// AssignedEntity type is unconditionally SHALL [1..*] id (e.g. CONF:1098-
// 7722 for Procedure Activity Performer), confirmed missing via a real
// schema validator run since this engine only ever wrote <id> when a
// performerNPI field happened to be mapped too.
func TestBuildDocument_AssignedEntityAlwaysGetsID(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"procedures": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"procedureCode": "73761001", "status": "completed",
					"performerGiven": "Sarah", "performerFamily": "Kim",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	aeIdx := strings.Index(xml, "<assignedEntity>")
	if aeIdx == -1 {
		t.Fatal("expected an <assignedEntity> element")
	}
	aeEnd := strings.Index(xml[aeIdx:], "</assignedEntity>")
	aeXML := xml[aeIdx : aeIdx+aeEnd]
	if !strings.Contains(aeXML, `<id nullFlavor="UNK"/>`) {
		t.Errorf("expected a fallback <id nullFlavor=\"UNK\"/> when no NPI was mapped, got: %s", aeXML)
	}
	if strings.Index(aeXML, "<id") > strings.Index(aeXML, "<assignedPerson") {
		t.Errorf("expected <id> before <assignedPerson>, got: %s", aeXML)
	}
}

// TestBuildDocument_NutritionAssessmentOwnCodeAndEffectiveTime verifies the
// nested Nutrition Assessment observation (distinct from the root Nutrition
// Status observation) gets its own fixed code="75303-8" (CONF:1098-30329)
// and its own effectiveTime (CONF:1098-31666) — both were completely absent
// since the section only ever declared entryFixedCode (for the root), never
// observationFixedCode, and had no field targeting the nested observation's
// own effectiveTime.
func TestBuildDocument_NutritionAssessmentOwnCodeAndEffectiveTime(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"nutrition": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"nutritionalStatusValue": "166935003", "effectiveTime": "20260101",
					"status": "active", "nutritionAssessmentEffectiveTime": "20260101",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	nestedIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.138"`)
	if nestedIdx == -1 {
		t.Fatal("expected the nested Nutrition Assessment observation's templateId")
	}
	nestedEnd := strings.Index(xml[nestedIdx:], "</observation>")
	nestedXML := xml[nestedIdx : nestedIdx+nestedEnd]
	if !strings.Contains(nestedXML, `<code code="75303-8" codeSystem="2.16.840.1.113883.6.1" displayName="Nutrition assessment"/>`) {
		t.Errorf("expected Nutrition Assessment's own fixed code, got: %s", nestedXML)
	}
	if !strings.Contains(nestedXML, `<effectiveTime value="20260101"/>`) {
		t.Errorf("expected Nutrition Assessment's own effectiveTime, got: %s", nestedXML)
	}
}

// TestBuildDocument_R1CompatTemplateIDs verifies the R1.1 backward-
// compatibility companion (CONF:1198-32936/32934 — "SHALL include both the
// C-CDA R2.1 templateId and the C-CDA R1.1 templateId root without an
// extension") is present at every level this engine emits a dated
// templateId: the document root, a section, an entry root, and a nested
// StructuralTemplateIDs anchor observation. Verified against the real 2012
// R1.1 IG this session — every root checked here is confirmed unchanged
// between R1.1 and R2.1 (only the dated @extension was added later).
func TestBuildDocument_R1CompatTemplateIDs(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "7980", "reaction": "247472004", "onsetDate": "20100604",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	// Document root: US Realm Header (.1.1) and CCD (.1.2).
	if !strings.Contains(xml, `<templateId root="2.16.840.1.113883.10.20.22.1.1" extension="2015-08-01"/>`) ||
		!strings.Contains(xml, `<templateId root="2.16.840.1.113883.10.20.22.1.1"/>`) {
		t.Error("expected both the dated and bare-root companion templateId for the US Realm Header (.1.1)")
	}
	if !strings.Contains(xml, `<templateId root="2.16.840.1.113883.10.20.22.1.2"/>`) {
		t.Error("expected the bare-root R1.1 companion for the CCD document templateId (.1.2)")
	}

	// Section: Allergies and Adverse Reactions (.2.6.1).
	if !strings.Contains(xml, `<templateId root="2.16.840.1.113883.10.20.22.2.6.1"/>`) {
		t.Error("expected the bare-root R1.1 companion for the Allergies section templateId")
	}

	// Entry root: Allergy Concern Act (.4.30).
	if !strings.Contains(xml, `<templateId root="2.16.840.1.113883.10.20.22.4.30"/>`) {
		t.Error("expected the bare-root R1.1 companion for the Allergy Concern Act entry templateId")
	}

	// Nested StructuralTemplateIDs anchor: Reaction Observation (.4.9).
	if !strings.Contains(xml, `<templateId root="2.16.840.1.113883.10.20.22.4.9"/>`) {
		t.Error("expected the bare-root R1.1 companion for the Reaction Observation anchor templateId")
	}
}

// TestBuildDocument_Nutrition_NoR1CompatTemplateID verifies a genuinely new,
// R2.1-only template (no R1.1 predecessor — confirmed via a zero-hit search
// of the entire 2012 R1.1 IG for Nutrition's OIDs) does NOT get a spurious
// bare-root companion, since this schema's own data correctly carries no
// extension for it in the first place.
func TestBuildDocument_Nutrition_NoR1CompatTemplateID(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"nutrition": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"nutritionalStatusValue": "166935003", "effectiveTime": "20260101",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if strings.Count(xml, `root="2.16.840.1.113883.10.20.22.4.124"`) != 1 {
		t.Errorf("expected exactly one templateId for Nutrition's entry root (no compat companion), got %d", strings.Count(xml, `root="2.16.840.1.113883.10.20.22.4.124"`))
	}
}

// TestBuildDocument_SocialHistory_NoEmptyPlaceholderObservation is the
// full-document regression test for the Social History fix: a real schema
// validator found two sibling <observation> elements under one <entry> —
// the section's generic, unconditionally-created rootEl (empty, since no
// "observationCode" field is mapped) sitting next to the real Tobacco Use
// observation a predicated field created separately. <entry> only permits
// one clinical statement child, so this asserts exactly one <observation>
// survives, carrying the real Tobacco Use data.
func TestBuildDocument_SocialHistory_NoEmptyPlaceholderObservation(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"socialHistory": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"smokingStatus": "8517006", "smokingStatusDisplay": "Former smoker",
					"smokingStatusEffectiveTime": "20200315",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	entryIdx := strings.Index(xml, `<entry typeCode="DRIV">`)
	entryEnd := strings.Index(xml[entryIdx:], "</entry>")
	entryXML := xml[entryIdx : entryIdx+entryEnd]
	if got := strings.Count(entryXML, "<observation "); got != 1 {
		t.Errorf("expected exactly one <observation> under the Social History entry, got %d: %s", got, entryXML)
	}
	if !strings.Contains(entryXML, `code="11367-0"`) {
		t.Errorf("expected the surviving observation to be the real Tobacco Use one, got: %s", entryXML)
	}
}

// TestBuildDocument_IntervalEffectiveTimeGetsIVLTSXsiType verifies every
// <effectiveTime> with low/high children gets xsi:type="IVL_TS" — CDA's
// base SXCM_TS type (effectiveTime's default declared type almost
// everywhere) only supports a bare @value scalar; carrying low/high
// children legally requires the IVL_TS override. A real schema validator
// found this missing across the board: the anonymous "effectiveTime must
// have no children, because the type's content type is empty" errors, and
// (once missing) validator confusion cascading onto later siblings like
// Allergy/Tobacco Use's own <value>. Also verifies a SCALAR effectiveTime
// (bare @value, e.g. a vital sign component's own observation date) is
// left untouched — that shape needs no override.
func TestBuildDocument_IntervalEffectiveTimeGetsIVLTSXsiType(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"medications": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"drugCode": "197361", "drugCodeDisplay": "Lisinopril",
					"startDate": "20230601", "stopDate": "20240601",
				},
			}},
			"advanceDirectives": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"directiveCode": "304251008", "value": "425392003",
					"effectiveTimeLow": "20240101", "effectiveTimeHigh": "20991231",
				},
			}},
			"vitalSigns": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"components": []interface{}{
						map[string]interface{}{"vitalCode": "8480-6", "value": "120", "valueUnit": "mm[Hg]", "componentEffectiveTime": "20260101"},
					},
					"effectiveTime": "20260101",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	// documentationOf/serviceEvent's own effectiveTime (low/high nullFlavor).
	if !strings.Contains(xml, `<effectiveTime xsi:type="IVL_TS">`) {
		t.Fatalf("expected at least one interval effectiveTime with xsi:type=IVL_TS, got: %s", xml)
	}
	got := strings.Count(xml, `<effectiveTime xsi:type="IVL_TS">`)
	// serviceEvent + medication (low+high) + advanceDirectives (low+high) = 3.
	if got != 3 {
		t.Errorf("expected 3 interval effectiveTime elements with xsi:type=IVL_TS (serviceEvent, medication, advanceDirectives), got %d: %s", got, xml)
	}

	// Scalar effectiveTime (vital sign component, plain @value) must NOT get an xsi:type.
	if strings.Contains(xml, `<effectiveTime xsi:type="IVL_TS" value=`) {
		t.Error("expected scalar effectiveTime (bare @value) to be left untouched, not given xsi:type=IVL_TS")
	}
	if !strings.Contains(xml, `<effectiveTime value="20260101"/>`) {
		t.Errorf("expected the scalar vital sign effectiveTime to remain a bare @value element, got: %s", xml)
	}
}

// TestBuildDocument_TobaccoUseCodeSystem verifies Tobacco Use's own <code>
// gets BOTH @code="11367-0" AND @codeSystem="2.16.840.1.113883.6.1" on the
// SAME element (CONF:16560/1098-19174) — a real schema validator found
// codeSystem missing, root-caused to applyPredicateConstraint handling two
// comma-AND conditions targeting the same nested child ("code/@code" and
// "code/@codeSystem") as two independent find-or-create passes, which
// produced two SEPARATE sibling <code> elements instead of one with both
// attributes.
func TestBuildDocument_TobaccoUseCodeSystem(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"socialHistory": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"smokingStatus": "8517006", "smokingStatusDisplay": "Former smoker",
					"smokingStatusEffectiveTime": "20200315",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `<code code="11367-0" codeSystem="2.16.840.1.113883.6.1"/>`) {
		t.Errorf("expected ONE <code> element carrying both @code and @codeSystem, got: %s", xml)
	}
	obsIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.85"`)
	obsEnd := strings.Index(xml[obsIdx:], "</observation>")
	obsXML := xml[obsIdx : obsIdx+obsEnd]
	if got := strings.Count(obsXML, "<code "); got != 1 {
		t.Errorf("expected exactly one <code> element on the Tobacco Use observation (not two siblings), got %d: %s", got, obsXML)
	}
}

// TestBuildDocument_AdvanceDirectiveCustodianAGNTClassCode verifies the
// custodian participant's participantRole gets @classCode="AGNT"
// (CONF:8670/1198-8670) — was being created as a bare <participantRole>
// with no classCode at all.
func TestBuildDocument_AdvanceDirectiveCustodianAGNTClassCode(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"advanceDirectives": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"directiveCode": "304251008", "value": "425392003",
					"effectiveTimeLow": "20240101", "effectiveTimeHigh": "20991231",
					"custodianName": "Robert Chen",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `<participantRole classCode="AGNT">`) {
		t.Errorf("expected the custodian participantRole's classCode=AGNT, got: %s", xml)
	}
}

// TestBuildDocument_ImmunizationManufacturedProductTemplateID verifies
// Immunization's manufacturedProduct gets the Immunization Medication
// Information (V2) templateId (.4.54/2014-06-09, CONF:1198-15546) — was
// completely missing (unlike Medications' own manufacturedProduct, which
// already had its own StructuralTemplateIDs entry).
func TestBuildDocument_ImmunizationManufacturedProductTemplateID(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"immunizations": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"vaccineCode": "88", "vaccineCodeDisplay": "Influenza",
					"administrationDate": "20251015", "negationInd": "false", "status": "completed",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	prodIdx := strings.Index(xml, `<manufacturedProduct classCode="MANU">`)
	if prodIdx == -1 {
		t.Fatal("expected a <manufacturedProduct classCode=\"MANU\"> element")
	}
	prodEnd := strings.Index(xml[prodIdx:], "</manufacturedProduct>")
	prodXML := xml[prodIdx : prodIdx+prodEnd]
	if !strings.Contains(prodXML, `root="2.16.840.1.113883.10.20.22.4.54" extension="2014-06-09"`) {
		t.Errorf("expected Immunization Medication Information's templateId, got: %s", prodXML)
	}
}

// TestBuildDocument_AssignedEntityAddrTelecomFallback verifies the
// assignedEntity fallback extends to addr/telecom (not just id) — CONF:1098-
// 7731/7732 for Procedure Activity Performer, confirmed missing when only
// name fields (no address/phone) are mapped for a performer.
func TestBuildDocument_AssignedEntityAddrTelecomFallback(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"procedures": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"procedureCode": "73761001", "status": "completed",
					"performerGiven": "Sarah", "performerFamily": "Kim",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	aeIdx := strings.Index(xml, "<assignedEntity>")
	aeEnd := strings.Index(xml[aeIdx:], "</assignedEntity>")
	aeXML := xml[aeIdx : aeIdx+aeEnd]
	if !strings.Contains(aeXML, `<addr nullFlavor="UNK"/>`) {
		t.Errorf("expected a fallback <addr nullFlavor=\"UNK\"/>, got: %s", aeXML)
	}
	if !strings.Contains(aeXML, `<telecom nullFlavor="UNK"/>`) {
		t.Errorf("expected a fallback <telecom nullFlavor=\"UNK\"/>, got: %s", aeXML)
	}
	wantOrder := []string{"<id", "<addr", "<telecom", "<assignedPerson"}
	lastIdx := -1
	for _, tag := range wantOrder {
		idx := strings.Index(aeXML, tag)
		if idx == -1 {
			t.Fatalf("expected %s present, got: %s", tag, aeXML)
		}
		if idx < lastIdx {
			t.Errorf("expected order id,addr,telecom,assignedPerson, got: %s", aeXML)
		}
		lastIdx = idx
	}
}

// TestBuildDocument_CodeSystemNameAutoDerivedForCodedFields verifies fields
// with a mapped codeSystem (reaction, severity, smokingStatus — all newly
// given xpathSystem this round) get codeSystemName auto-derived on the same
// <value> element, matching how real IG worked examples pair the two
// (Figure 219: codeSystem="2.16.840.1.113883.6.96" codeSystemName="SNOMED
// CT").
func TestBuildDocument_CodeSystemNameAutoDerivedForCodedFields(t *testing.T) {
	loader := loadTestSchema(t)
	doc := map[string]interface{}{
		"header": map[string]interface{}{},
		"sections": map[string]interface{}{
			"allergiesAndIntolerances": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"medicationAllergyCode": "7980", "medicationAllergyCodeDisplay": "Penicillin",
					"reaction": "247472004", "reactionDisplay": "Hives", "reactionSystem": "2.16.840.1.113883.6.96",
					"severity": "255604002", "severityDisplay": "Mild", "severitySystem": "2.16.840.1.113883.6.96",
					"allergyTypeCode": "419199007", "allergyTypeCodeDisplay": "Allergy to substance",
					"allergyTypeCodeSystem": "2.16.840.1.113883.6.96",
					"onsetDate":             "20100604",
				},
			}},
			"socialHistory": map[string]interface{}{"entries": []interface{}{
				map[string]interface{}{
					"smokingStatus": "65568007", "smokingStatusDisplay": "Cigarette smoker",
					"smokingStatusSystem":        "2.16.840.1.113883.6.96",
					"smokingStatusEffectiveTime": "20100604", "smokingStatusEffectiveTimeHigh": "20200315",
				},
			}},
		},
	}
	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	wantCount := strings.Count(xml, `codeSystem="2.16.840.1.113883.6.96"`)
	gotNamed := strings.Count(xml, `codeSystem="2.16.840.1.113883.6.96" codeSystemName="SNOMED CT"`)
	if wantCount == 0 {
		t.Fatal("expected at least one SNOMED CT codeSystem in output")
	}
	if wantCount != gotNamed {
		t.Errorf("expected every codeSystem=\"2.16.840.1.113883.6.96\" to be immediately followed by codeSystemName=\"SNOMED CT\" (%d occurrences), got %d paired, full xml: %s", wantCount, gotNamed, xml)
	}
}

// TestBuildDocument_MedicalEquipment_AlternateProcedureArchetype verifies
// Medical Equipment Section2's optional second entry shape (CONF:1098-
// 31885/31886, Procedure Activity Procedure V2 — the SAME archetype the
// Procedures section builds) — a section producing entries of TWO different
// structural shapes (the primary <supply> plus this alternate <procedure>)
// inside ONE physical <section> element via AlternateEntryArchetype.
func TestBuildDocument_MedicalEquipment_AlternateProcedureArchetype(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["medicalEquipment"] = map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"equipmentCode": "360008007", "equipmentCodeDisplay": "Wheelchair",
				"status": "completed",
			},
		},
		"procedureEntries": []interface{}{
			map[string]interface{}{
				"equipmentProcedureCode": "87717006", "equipmentProcedureCodeDisplay": "Insertion of cardiac pacemaker",
				"equipmentProcedureCodeSystem":          "2.16.840.1.113883.6.96",
				"equipmentProcedureStatus":              "completed",
				"equipmentProcedureDate":                "20240301",
				"equipmentProcedurePerformerNPI":        "1730123456",
				"equipmentProcedurePerformerStreet":     "100 Innovation Drive",
				"equipmentProcedurePerformerCity":       "Springfield",
				"equipmentProcedurePerformerState":      "IL",
				"equipmentProcedurePerformerPostalCode": "62701",
				"equipmentProcedurePerformerPhone":      "tel:2175550100",
				"equipmentProcedurePerformerGiven":      "Sarah",
				"equipmentProcedurePerformerFamily":     "Kim",
			},
		},
	}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	if !strings.Contains(xml, `<supply`) {
		t.Errorf("expected the primary <supply> entry to still be present, got:\n%s", xml)
	}
	if !strings.Contains(xml, `<procedure`) {
		t.Fatalf("expected an alternate <procedure> entry, got:\n%s", xml)
	}
	if !strings.Contains(xml, `code="87717006"`) {
		t.Errorf("expected the alternate procedure's own code, got:\n%s", xml)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.14" extension="2014-06-09"`) {
		t.Errorf("expected the alternate procedure to carry Procedure Activity Procedure (V2)'s templateId, got:\n%s", xml)
	}
	// Both shapes must land inside the SAME <section>, not two sections.
	if strings.Count(xml, `code code="46264-8"`) != 1 {
		t.Errorf("expected exactly one Medical Equipment section (LOINC 46264-8), got:\n%s", xml)
	}

	// CONF:1098-7722/7731/7732 (Procedure Activity Performer): assignedEntity
	// SHALL contain id, addr, AND telecom — a real defect found via the
	// external SITE validator (cvc-complex-type.2.4.a: assignedPerson
	// appeared where id/addr/telecom were expected) when this alternate's
	// performer only ever wrote assignedPerson. AssignedEntity's own base
	// type sequence is id*, code?, addr*, telecom*, assignedPerson?,
	// representedOrganization? — verify both presence AND order.
	//
	// Search from AFTER the narrative table, not the whole document —
	// buildSectionNarrative (which runs before the entries) now includes a
	// "Performer Phone" column since equipmentProcedurePerformerPhone has a
	// value, so the bare string "tel:2175550100" ALSO appears earlier as
	// plain narrative text (<td>tel:2175550100</td>), which a naive
	// whole-document strings.Index would match instead of the real
	// structured <telecom> element.
	entryIdx := strings.Index(xml, `<procedure classCode="PROC"`)
	if entryIdx == -1 {
		t.Fatalf("expected the alternate <procedure> entry root, got:\n%s", xml)
	}
	entryXML := xml[entryIdx:]
	idIdx := strings.Index(entryXML, `id root="2.16.840.1.113883.4.6" extension="1730123456"`)
	addrIdx := strings.Index(entryXML, `<streetAddressLine>100 Innovation Drive</streetAddressLine>`)
	telecomIdx := strings.Index(entryXML, `<telecom value="tel:2175550100"`)
	givenIdx := strings.Index(entryXML, `<given>Sarah</given>`)
	if idIdx == -1 || addrIdx == -1 || telecomIdx == -1 || givenIdx == -1 {
		t.Fatalf("expected performer id/addr/telecom/given all present in the entry itself, got:\n%s", entryXML)
	}
	if !(idIdx < addrIdx && addrIdx < telecomIdx && telecomIdx < givenIdx) {
		t.Errorf("expected assignedEntity child order id < addr < telecom < assignedPerson, got indices id=%d addr=%d telecom=%d given=%d", idIdx, addrIdx, telecomIdx, givenIdx)
	}
}

// TestBuildDocument_MedicalEquipment_OnlyAlternateEntries_SectionStillBuilds
// guards the section-skip logic BuildDocument itself does before calling
// buildSectionElement: a MAY section with ZERO primary entries but real
// alternate-archetype entries must still be emitted, not silently dropped —
// the exact edge case a naive `len(entries) == 0 && !isShall` check (the
// pre-existing logic, unaware of alternates) would get wrong.
func TestBuildDocument_MedicalEquipment_OnlyAlternateEntries_SectionStillBuilds(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["medicalEquipment"] = map[string]interface{}{
		"procedureEntries": []interface{}{
			map[string]interface{}{
				"equipmentProcedureCode": "87717006", "equipmentProcedureStatus": "completed",
			},
		},
	}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}
	if !strings.Contains(xml, `code code="46264-8"`) {
		t.Errorf("expected Medical Equipment section to still be built when only alternate entries are present, got:\n%s", xml)
	}
	if !strings.Contains(xml, `<procedure`) {
		t.Errorf("expected the alternate procedure entry itself to be present, got:\n%s", xml)
	}
}

// TestBuildDocument_Immunization_SeriesEntryRelationship verifies the
// Immunization Activity (V3) series-tracking entryRelationship (CONF:1198-
// 31510/31511/31512/31514) — a nested Substance Administered Act (templateId
// .4.118, fixed code="416118004"/SNOMED "Administration", fixed
// statusCode="completed" via StructuralTemplateAnchor.FixedStatusCode
// overriding the "act" tag's own "active" default) alongside a
// user-suppliable sequenceNumber marking this dose's position in a series.
func TestBuildDocument_Immunization_SeriesEntryRelationship(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["immunizations"] = map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"vaccineCode": "43", "vaccineCodeDisplay": "Hepatitis B vaccine, adult dosage",
				"administrationDate": "20240115", "status": "completed", "negationInd": "false",
				"seriesSequenceNumber": "2", "seriesAdministrationDate": "20240115",
			},
		},
	}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	if !strings.Contains(xml, `typeCode="COMP"`) {
		t.Fatalf("expected the series entryRelationship (typeCode=COMP), got:\n%s", xml)
	}
	if !strings.Contains(xml, `inversionInd="true"`) {
		t.Errorf("expected inversionInd=\"true\" on the series entryRelationship, got:\n%s", xml)
	}
	if !strings.Contains(xml, `<sequenceNumber value="2"`) {
		t.Errorf("expected sequenceNumber value=2, got:\n%s", xml)
	}
	// CONF:1198-31513/31514 list sequenceNumber (item c) BEFORE the nested
	// Substance Administered Act (item d) — confirmed against the real IG's
	// own XPath overview table too. entryRelationship isn't a
	// StructuralTemplateAnchor (no automatic reorder pass runs on it), so
	// field DECLARATION order alone determines this — a real external
	// validator catch (cvc-complex-type.2.4.d: "Invalid content...
	// 'sequenceNumber'. No child element is expected at this point") when
	// seriesAdministrationDate was declared before seriesSequenceNumber.
	seqIdx := strings.Index(xml, "<sequenceNumber")
	actIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.118"`)
	if seqIdx == -1 || actIdx == -1 || seqIdx > actIdx {
		t.Errorf("expected <sequenceNumber> to precede the nested Substance Administered Act, got seqIdx=%d actIdx=%d", seqIdx, actIdx)
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.10.20.22.4.118"`) {
		t.Errorf("expected the nested Substance Administered Act's templateId, got:\n%s", xml)
	}
	if !strings.Contains(xml, `code="416118004"`) || !strings.Contains(xml, `codeSystem="2.16.840.1.113883.6.96"`) {
		t.Errorf("expected the fixed Administration code (SNOMED 416118004), got:\n%s", xml)
	}
	// Verify the FixedStatusCode override actually took effect ON THIS
	// specific nested act (not just that "completed" appears somewhere in
	// the document) — window the search to the ~200 chars right after this
	// act's own templateId, where its own statusCode must land per
	// structuralElementOrder (templateId, id, code, statusCode, ...).
	tidIdx := strings.Index(xml, `root="2.16.840.1.113883.10.20.22.4.118"`)
	if tidIdx == -1 {
		t.Fatalf("expected to find the Substance Administered Act's templateId, got:\n%s", xml)
	}
	windowEnd := tidIdx + 400
	if windowEnd > len(xml) {
		windowEnd = len(xml)
	}
	window := xml[tidIdx:windowEnd]
	if !strings.Contains(window, `<statusCode code="completed"`) {
		t.Errorf("expected the Substance Administered Act's own statusCode to be \"completed\" (FixedStatusCode override, not the \"act\" tag's default \"active\"), got window:\n%s", window)
	}
}

// TestBuildDocument_NarrativeReference_AllergyCodeLinksToOwnRow verifies
// Round 23's narrative-ID + reference-linking mechanism (buildSectionNarrative
// assigns each entry's own <tr ID="sectionKey-N">, buildSectionElement
// injects "_narrativeRef" = "#sectionKey-N" into that same entry before
// buildEntry runs, a "_narrativeRef"-keyed schema field writes it into
// originalText/reference/@value) — CONF:16326 for Allergy specifically, the
// same root cause behind 9 of Round 23's 16 real code gaps.
func TestBuildDocument_NarrativeReference_AllergyCodeLinksToOwnRow(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	if !strings.Contains(xml, `<tr ID="allergiesAndIntolerances-1"`) {
		t.Fatalf("expected the allergy narrative row to carry ID=\"allergiesAndIntolerances-1\", got:\n%s", xml)
	}
	if !strings.Contains(xml, `<reference value="#allergiesAndIntolerances-1"`) {
		t.Errorf("expected the allergen code's originalText/reference to link back to its own narrative row, got:\n%s", xml)
	}
}

// TestBuildDocument_NarrativeReference_MedicalEquipmentAlternateGetsOwnRow
// verifies narrative row IDs are assigned across BOTH a section's primary
// entries AND its AlternateArchetype entries from ONE shared, sequential
// counter (not two independently-numbered pools that could collide) — the
// pacemaker (alternate/procedure shape) gets row 2, right after the
// wheelchair (primary/supply shape)'s row 1.
func TestBuildDocument_NarrativeReference_MedicalEquipmentAlternateGetsOwnRow(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["medicalEquipment"] = map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"equipmentCode": "360008007", "equipmentCodeDisplay": "Wheelchair", "status": "completed"},
		},
		"procedureEntries": []interface{}{
			map[string]interface{}{"equipmentProcedureCode": "87717006", "equipmentProcedureCodeDisplay": "Insertion of cardiac pacemaker", "equipmentProcedureStatus": "completed"},
		},
	}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	if !strings.Contains(xml, `<tr ID="medicalEquipment-1"`) {
		t.Errorf("expected the wheelchair (primary) row to be medicalEquipment-1, got:\n%s", xml)
	}
	if !strings.Contains(xml, `<tr ID="medicalEquipment-2"`) {
		t.Errorf("expected the pacemaker (alternate) row to be medicalEquipment-2, continuing the same sequential counter, got:\n%s", xml)
	}
	if !strings.Contains(xml, `<reference value="#medicalEquipment-2"`) {
		t.Errorf("expected the pacemaker procedure's own originalText/reference to link to its own row, got:\n%s", xml)
	}
}

// TestBuildDocument_PatientLanguageCommunication_ProficiencyAndPreferenceOrder
// verifies languageCommunication's base type sequence (languageCode,
// modeCode?, proficiencyLevelCode?, preferenceInd?) — all three new/existing
// fields live in patientScalarFields specifically so they share ONE
// construction pass (see that var's own doc comment for why splitting
// proficiencyLevelCode into the separate, later-running patientCodedFields
// pass would have broken this exact order).
func TestBuildDocument_PatientLanguageCommunication_ProficiencyAndPreferenceOrder(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	header := doc["header"].(map[string]interface{})
	patient := header["patient"].(map[string]interface{})
	patient["preferredLanguage"] = "en"
	patient["languageProficiency"] = "G"
	patient["languageProficiencySystem"] = "2.16.840.1.113883.5.61"
	patient["languagePreferenceInd"] = "true"

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	langIdx := strings.Index(xml, `<languageCode code="en"`)
	profIdx := strings.Index(xml, `<proficiencyLevelCode code="G" codeSystem="2.16.840.1.113883.5.61"`)
	prefIdx := strings.Index(xml, `<preferenceInd value="true"`)
	if langIdx == -1 || profIdx == -1 || prefIdx == -1 {
		t.Fatalf("expected languageCode, proficiencyLevelCode, and preferenceInd all present, got:\n%s", xml)
	}
	if !(langIdx < profIdx && profIdx < prefIdx) {
		t.Errorf("expected languageCommunication child order languageCode < proficiencyLevelCode < preferenceInd, got indices lang=%d prof=%d pref=%d", langIdx, profIdx, prefIdx)
	}
}

// TestBuildDocument_DocumentationOfPerformer_SpecialtyCode_OrderedBeforeName
// verifies writePersonReference's new specialty code (CONF:1198-14842 — a
// real gap, no code mechanism existed at all before Round 23, unlike every
// per-section author field) lands on assignedEntity BEFORE assignedPerson,
// per AssignedEntity's own real sequence (id, code, addr, telecom,
// assignedPerson, representedOrganization).
func TestBuildDocument_DocumentationOfPerformer_SpecialtyCode_OrderedBeforeName(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	header := doc["header"].(map[string]interface{})
	docOf := header["documentationOf"].(map[string]interface{})
	docOf["performers"] = []interface{}{
		map[string]interface{}{
			"given": "Sarah", "family": "Kim", "npi": "1730123456",
			"specialtyCode": "207R00000X", "specialtyCodeSystem": "2.16.840.1.113883.6.101", "specialtyCodeDisplay": "Internal Medicine",
		},
	}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	idIdx := strings.Index(xml, `<id root="2.16.840.1.113883.4.6" extension="1730123456"/>`)
	codeIdx := strings.Index(xml, `<code code="207R00000X" codeSystem="2.16.840.1.113883.6.101"`)
	nameIdx := strings.Index(xml, `<given>Sarah</given>`)
	if idIdx == -1 || codeIdx == -1 || nameIdx == -1 {
		t.Fatalf("expected documentationOf performer's id/code/name all present, got:\n%s", xml)
	}
	if !(idIdx < codeIdx && codeIdx < nameIdx) {
		t.Errorf("expected assignedEntity child order id < code < assignedPerson, got indices id=%d code=%d name=%d", idIdx, codeIdx, nameIdx)
	}
}

// TestBuildDocument_MedicalEquipmentAlternate_TargetSiteOrgAndAuthor verifies
// the three fields the Procedure2 alternate archetype was missing relative
// to the main Procedures section (targetSiteCode, representedOrganization,
// Author Participation) — the last of which needed AlternateEntryArchetype
// to grow its own StructuralTemplateIDs support (Round 23), reusing
// applyStructuralTemplateAnchors rather than duplicating buildEntry's logic.
func TestBuildDocument_MedicalEquipmentAlternate_TargetSiteOrgAndAuthor(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["medicalEquipment"] = map[string]interface{}{
		"procedureEntries": []interface{}{
			map[string]interface{}{
				"equipmentProcedureCode": "87717006", "equipmentProcedureStatus": "completed",
				"equipmentProcedureTargetSiteCode": "43799004", "equipmentProcedureTargetSiteCodeDisplay": "Chest wall structure",
				"equipmentProcedurePerformerOrgName": "ezHealthKonnect Demo",
				"equipmentProcedureAuthorGiven":      "Sarah",
				"equipmentProcedureAuthorFamily":     "Kim",
				"equipmentProcedureAuthorNPI":        "1730123456",
			},
		},
	}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	if !strings.Contains(xml, `<targetSiteCode code="43799004" displayName="Chest wall structure"`) {
		t.Errorf("expected the alternate procedure's targetSiteCode, got:\n%s", xml)
	}
	if !strings.Contains(xml, `<representedOrganization>`) || !strings.Contains(xml, `<name>ezHealthKonnect Demo</name>`) {
		t.Errorf("expected the alternate procedure's representedOrganization, got:\n%s", xml)
	}
	// Search AFTER the pacemaker's own code — the document also has a
	// header-level <author> (John Smith, from fullCanonicalDoc's fixture)
	// that a plain first-match search would find instead.
	pacemakerIdx := strings.Index(xml, `code="87717006"`)
	if pacemakerIdx == -1 {
		t.Fatalf("expected the pacemaker procedure's own code, got:\n%s", xml)
	}
	authorIdx := strings.Index(xml[pacemakerIdx:], `<author>`)
	if authorIdx == -1 {
		t.Fatalf("expected an <author> element on the alternate procedure, got:\n%s", xml)
	}
	authorIdx += pacemakerIdx
	authorWindow := xml[authorIdx:min(authorIdx+500, len(xml))]
	if !strings.Contains(authorWindow, `root="2.16.840.1.113883.10.20.22.4.119"`) {
		t.Errorf("expected the alternate procedure's author to carry Author Participation's templateId, got:\n%s", authorWindow)
	}
	if !strings.Contains(authorWindow, `<given>Sarah</given>`) {
		t.Errorf("expected the alternate procedure's author name, got:\n%s", authorWindow)
	}
}

// TestBuildDocument_AdvanceDirective_ReferencedDocument_URLMode verifies the
// externalDocument/text/reference/@value pointer mode (CONF:1198-8692/8697/
// 8698) still works — the ED datatype's "point at something external" shape,
// unchanged from when it was first added.
func TestBuildDocument_AdvanceDirective_ReferencedDocument_URLMode(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["advanceDirectives"] = map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"directiveCode": "304251008", "value": "425392003",
				"effectiveTimeLow": "20240101", "effectiveTimeHigh": "20991231",
				"referencedDocumentIdRoot": "1.2.3.4.5", "referencedDocumentIdExtension": "DOC-001",
				"referencedDocumentURL": "https://example.org/directives/DOC-001.pdf",
			},
		},
	}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	if !strings.Contains(xml, `<reference typeCode="REFR">`) {
		t.Fatalf("expected the externalDocument reference wrapper, got:\n%s", xml)
	}
	if !strings.Contains(xml, `<id root="1.2.3.4.5" extension="DOC-001"/>`) {
		t.Errorf("expected the externalDocument's own id, got:\n%s", xml)
	}
	if !strings.Contains(xml, `<reference value="https://example.org/directives/DOC-001.pdf"/>`) {
		t.Errorf("expected the URL pointer reference, got:\n%s", xml)
	}
}

// TestBuildDocument_AdvanceDirective_ReferencedDocument_Base64Mode verifies
// the ED datatype's OTHER representation — inline embedded content, an
// alternative to the URL pointer above. representation="B64" and mediaType
// are attributes on the SAME <text> element the base64 payload is the text
// CONTENT of (same "bare xpath = SetText" convention as
// procedureCodeOriginalText/statusText) — id must still come before text
// per externalDocument's own content model (id*, text?).
func TestBuildDocument_AdvanceDirective_ReferencedDocument_Base64Mode(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["advanceDirectives"] = map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"directiveCode": "304251008", "value": "425392003",
				"effectiveTimeLow": "20240101", "effectiveTimeHigh": "20991231",
				"referencedDocumentIdRoot": "1.2.3.4.5", "referencedDocumentIdExtension": "DOC-002",
				"referencedDocumentContent":        "JVBERi0xLjQKJeLjz9M=",
				"referencedDocumentRepresentation": "B64",
				"referencedDocumentMediaType":      "application/pdf",
			},
		},
	}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	if !strings.Contains(xml, `<text representation="B64" mediaType="application/pdf">JVBERi0xLjQKJeLjz9M=</text>`) {
		t.Fatalf("expected the base64-embedded text element with its representation/mediaType attributes and content, got:\n%s", xml)
	}
	idIdx := strings.Index(xml, `<id root="1.2.3.4.5" extension="DOC-002"/>`)
	textIdx := strings.Index(xml, `<text representation="B64"`)
	if idIdx == -1 || textIdx == -1 {
		t.Fatalf("expected both id and text present, got:\n%s", xml)
	}
	if !(idIdx < textIdx) {
		t.Errorf("expected externalDocument child order id < text, got id=%d text=%d", idIdx, textIdx)
	}
}

// TestBuildDocument_ResultObservation_ReferenceRangeInclusiveFlag verifies
// the HL7 V3 IVXB_PQ @inclusive attribute on Result Observation's
// referenceRange/observationRange/value low/high — a base-datatype-level
// optional flag (not a C-CDA IG CONF requirement, so no validator ever
// flags its absence) indicating whether the boundary VALUE ITSELF counts as
// part of the normal range. Only written when a source system actually
// distinguishes inclusive/exclusive boundaries — omitted, the HL7 default
// (true) applies implicitly, same "don't fabricate what wasn't asked for"
// judgment call as every other optional attribute in this schema.
func TestBuildDocument_ResultObservation_ReferenceRangeInclusiveFlag(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["results"] = map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"testCode": "2345-7", "testCodeDisplay": "Glucose",
				"resultValue": "95", "resultValueUnit": "mg/dL", "resultStatus": "completed",
				"effectiveTime":     "20240101120000",
				"referenceRangeLow": "70", "referenceRangeLowInclusive": "true",
				"referenceRangeHigh": "99", "referenceRangeHighInclusive": "false",
			},
		},
	}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	if !strings.Contains(xml, `<low value="70" inclusive="true"`) {
		t.Errorf("expected referenceRange low with value=70 and inclusive=true coexisting, got:\n%s", xml)
	}
	if !strings.Contains(xml, `<high value="99" inclusive="false"`) {
		t.Errorf("expected referenceRange high with value=99 and inclusive=false coexisting, got:\n%s", xml)
	}
}

// TestBuildDocument_ResultObservation_ReferenceRangeInclusiveFlag_OmittedWhenUnmapped
// guards the default-omission case: when a record doesn't map the inclusive
// flag at all, low/high must stay exactly as before (no fabricated
// inclusive="true" the schema loader didn't actually decide) — a real
// regression risk any time a new optional field is added alongside existing
// ones on the same element.
func TestBuildDocument_ResultObservation_ReferenceRangeInclusiveFlag_OmittedWhenUnmapped(t *testing.T) {
	loader := loadTestSchema(t)
	doc := fullCanonicalDoc()
	sections := doc["sections"].(map[string]interface{})
	sections["results"] = map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"testCode": "2345-7", "testCodeDisplay": "Glucose",
				"resultValue": "95", "resultValueUnit": "mg/dL", "resultStatus": "completed",
				"effectiveTime":     "20240101120000",
				"referenceRangeLow": "70", "referenceRangeHigh": "99",
			},
		},
	}

	xml, err := BuildDocument(loader, doc, "CCD", BuildOptions{})
	if err != nil {
		t.Fatalf("BuildDocument failed: %v", err)
	}

	if strings.Contains(xml, `inclusive=`) {
		t.Errorf("expected no @inclusive attribute written when not mapped, got:\n%s", xml)
	}
	if !strings.Contains(xml, `<low value="70"/>`) || !strings.Contains(xml, `<high value="99"/>`) {
		t.Errorf("expected referenceRange low/high still present without inclusive, got:\n%s", xml)
	}
}
