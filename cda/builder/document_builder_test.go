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
					"reaction": "247472004", "reactionDisplay": "Hives",
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
					"status": "55561003", "onsetDate": "20190601",
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
					"vaccineCodeSystem": "2.16.840.1.113883.12.292",
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
					"effectiveTime": "20230501", "status": "completed",
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
	if !strings.Contains(tail, `<value code="44054006"`) {
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
	// 17 real sections (7 SHALL + 2 SHOULD + 8 MAY, per the C-CDA 2.1 IG's
	// Table 30, 2018 errata) — confirm every one of them actually made it
	// into the built document as "present" or "present_empty", not just the
	// SHALL subset checked above.
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
