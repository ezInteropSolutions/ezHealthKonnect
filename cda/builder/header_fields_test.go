package builder

import (
	"testing"

	"github.com/beevik/etree"
)

func TestWriteHeaderFields_WritesPresentKeysOnly(t *testing.T) {
	root := etree.NewElement("patientRole")
	data := map[string]interface{}{"dateOfBirth": "19800101", "firstName": "Jane", "lastName": "Doe"}

	writeHeaderFields(root, data, patientScalarFields)

	if got := root.FindElement("patient/birthTime").SelectAttrValue("value", ""); got != "19800101" {
		t.Errorf("birthTime/@value = %q, want 19800101", got)
	}
	if got := root.FindElement("patient/name/family").Text(); got != "Doe" {
		t.Errorf("name/family = %q, want Doe", got)
	}
	// middleName absent from data — no second <given> should be created.
	given := root.FindElement("patient/name").SelectElements("given")
	if len(given) != 1 || given[0].Text() != "Jane" {
		t.Errorf("expected exactly 1 <given>=Jane (no middleName in data), got %d: %v", len(given), given)
	}
}

func TestWriteHeaderFields_TwoGivenNames_CreateSeparateSiblings(t *testing.T) {
	root := etree.NewElement("patientRole")
	data := map[string]interface{}{"firstName": "Jane", "middleName": "Q"}

	writeHeaderFields(root, data, patientScalarFields)

	given := root.FindElement("patient/name").SelectElements("given")
	if len(given) != 2 {
		t.Fatalf("expected 2 <given> siblings, got %d", len(given))
	}
	if given[0].Text() != "Jane" || given[1].Text() != "Q" {
		t.Errorf("got given=[%q, %q], want [Jane, Q]", given[0].Text(), given[1].Text())
	}
}

func TestWriteHeaderFields_DeceasedIndAndNameSuffix_SdtcNamespacedTagRoundTrips(t *testing.T) {
	root := etree.NewElement("patientRole")
	data := map[string]interface{}{
		"firstName":    "Jane",
		"lastName":     "Doe",
		"nameSuffix":   "Jr.",
		"deceasedInd":  "true",
		"deceasedTime": "20240315",
	}

	writeHeaderFields(root, data, patientScalarFields)

	if got := root.FindElement("patient/name/suffix").Text(); got != "Jr." {
		t.Errorf("name/suffix = %q, want Jr.", got)
	}

	// etree.CreateElement("sdtc:deceasedInd") DOES auto-split the namespace
	// prefix into Element.Space="sdtc", leaving Element.Tag="deceasedInd" —
	// confirmed empirically (this test originally asserted the opposite and
	// failed, catching a real bug: reorderChildrenByTag ranks by .Tag only,
	// so its order list must use the bare local name, not "sdtc:deceasedInd",
	// to actually match — see document_builder.go's writePatientHeader).
	deceasedInd := root.FindElement("patient/sdtc:deceasedInd")
	if deceasedInd == nil {
		t.Fatal("expected patient/sdtc:deceasedInd to be findable via its namespace-qualified path")
	}
	if got := deceasedInd.SelectAttrValue("value", ""); got != "true" {
		t.Errorf("sdtc:deceasedInd/@value = %q, want true", got)
	}
	if deceasedInd.Space != "sdtc" || deceasedInd.Tag != "deceasedInd" {
		t.Errorf("expected Space=%q Tag=%q, got Space=%q Tag=%q",
			"sdtc", "deceasedInd", deceasedInd.Space, deceasedInd.Tag)
	}

	deceasedTime := root.FindElement("patient/sdtc:deceasedTime")
	if deceasedTime == nil || deceasedTime.SelectAttrValue("value", "") != "20240315" {
		t.Errorf("expected patient/sdtc:deceasedTime/@value = 20240315, got %+v", deceasedTime)
	}
}

func TestWriteRepeatingGroup_WritesUseAttrFromItemDataWhenPresent(t *testing.T) {
	root := etree.NewElement("patientRole")
	data := map[string]interface{}{
		"addresses": []interface{}{
			map[string]interface{}{"use": "OLD", "street": "1 Old St", "city": "Springfield"},
			map[string]interface{}{"street": "2 No-Use-Given Ave"},
		},
	}

	writeRepeatingGroup(root, data, "addresses", "addr", patientAddressFields)

	addrs := root.SelectElements("addr")
	if len(addrs) != 2 {
		t.Fatalf("expected 2 <addr> elements, got %d", len(addrs))
	}
	if got := addrs[0].SelectAttrValue("use", ""); got != "OLD" {
		t.Errorf("addr[0]/@use = %q, want OLD (caller-supplied, not a hardcoded/invented default)", got)
	}
	if got := addrs[0].FindElement("streetAddressLine").Text(); got != "1 Old St" {
		t.Errorf("addr[0]/streetAddressLine = %q, want '1 Old St'", got)
	}
	if addrs[1].SelectAttr("use") != nil {
		t.Error("expected no @use on addr[1] when the item supplies none")
	}
}

func TestWritePatientHeader_AdditionalNamesAndAddresses_DontDisturbPrimary(t *testing.T) {
	root := etree.NewElement("ClinicalDocument")
	header := map[string]interface{}{
		"patient": map[string]interface{}{
			"firstName": "Jane",
			"lastName":  "Doe",
			"address":   map[string]interface{}{"street": "123 Main St", "city": "Springfield"},
			"names": []interface{}{
				map[string]interface{}{"use": "L", "given": "Janet", "family": "Smith"},
			},
			"addresses": []interface{}{
				map[string]interface{}{"street": "1 Old St", "city": "Old Town"},
			},
		},
	}

	writePatientHeader(root, header)

	patientRole := root.FindElement("recordTarget/patientRole")
	names := patientRole.FindElement("patient").SelectElements("name")
	if len(names) != 2 {
		t.Fatalf("expected primary name + 1 additional name = 2 <name> elements, got %d", len(names))
	}
	if got := names[0].FindElement("given").Text(); got != "Jane" {
		t.Errorf("primary name/given = %q, want Jane (must stay untouched)", got)
	}
	if got := names[1].SelectAttrValue("use", ""); got != "L" {
		t.Errorf("names[1]/@use = %q, want L", got)
	}
	if got := names[1].FindElement("given").Text(); got != "Janet" {
		t.Errorf("names[1]/given = %q, want Janet", got)
	}

	addrs := patientRole.SelectElements("addr")
	if len(addrs) != 2 {
		t.Fatalf("expected primary addr + 1 additional addr = 2 <addr> elements, got %d", len(addrs))
	}
	if got := addrs[0].SelectAttrValue("use", ""); got != "HP" {
		t.Errorf("primary addr/@use = %q, want HP (must stay untouched)", got)
	}
	if got := addrs[1].FindElement("streetAddressLine").Text(); got != "1 Old St" {
		t.Errorf("addrs[1]/streetAddressLine = %q, want '1 Old St'", got)
	}
}

func TestWriteRelatedPersonsHeader_WritesOneParticipantPerPerson(t *testing.T) {
	root := etree.NewElement("ClinicalDocument")
	header := map[string]interface{}{
		"relatedPersons": []interface{}{
			map[string]interface{}{"relationshipCode": "SPS", "relationshipDisplay": "Spouse", "given": "Jane", "family": "Doe"},
			map[string]interface{}{"relationshipCode": "CHILD", "given": "Sam"},
		},
	}

	writeRelatedPersonsHeader(root, header)

	participants := root.SelectElements("participant")
	if len(participants) != 2 {
		t.Fatalf("expected 2 <participant> elements, got %d", len(participants))
	}

	p0 := participants[0]
	if got := p0.SelectAttrValue("typeCode", ""); got != "IND" {
		t.Errorf("participant[0]/@typeCode = %q, want IND", got)
	}
	ae := p0.SelectElement("associatedEntity")
	if ae == nil {
		t.Fatal("expected associatedEntity")
	}
	if got := ae.SelectAttrValue("classCode", ""); got != "PRS" {
		t.Errorf("associatedEntity/@classCode = %q, want PRS", got)
	}
	code := ae.SelectElement("code")
	if code == nil || code.SelectAttrValue("code", "") != "SPS" || code.SelectAttrValue("codeSystem", "") != relatedPersonRoleCodeSystem {
		t.Errorf("expected code=SPS codeSystem=%s, got %+v", relatedPersonRoleCodeSystem, code)
	}
	if code.SelectAttrValue("displayName", "") != "Spouse" {
		t.Errorf("expected code/@displayName=Spouse, got %q", code.SelectAttrValue("displayName", ""))
	}
	if got := ae.FindElement("associatedPerson/name/given").Text(); got != "Jane" {
		t.Errorf("associatedPerson/name/given = %q, want Jane", got)
	}
	if got := ae.FindElement("associatedPerson/name/family").Text(); got != "Doe" {
		t.Errorf("associatedPerson/name/family = %q, want Doe", got)
	}

	// Second person has no relationshipDisplay/family — confirm no displayName
	// or <family> element is fabricated for absent data.
	p1 := participants[1]
	code1 := p1.FindElement("associatedEntity/code")
	if code1.SelectAttrValue("displayName", "") != "" {
		t.Errorf("expected no displayName when relationshipDisplay absent, got %q", code1.SelectAttrValue("displayName", ""))
	}
	if p1.FindElement("associatedEntity/associatedPerson/name/family") != nil {
		t.Error("expected no <family> element when family data is absent")
	}
}

func TestWriteCodedFields_OnlyWritesWhenCodePresent(t *testing.T) {
	root := etree.NewElement("patientRole")
	// sex present, race/ethnicity absent — only administrativeGenderCode should appear.
	data := map[string]interface{}{"sex": "F", "sexDisplay": "Female"}

	writeCodedFields(root, data, patientCodedFields)

	gender := root.FindElement("patient/administrativeGenderCode")
	if gender == nil {
		t.Fatal("expected administrativeGenderCode element")
	}
	if got := gender.SelectAttrValue("code", ""); got != "F" {
		t.Errorf("code = %q, want F", got)
	}
	if got := gender.SelectAttrValue("codeSystem", ""); got != "2.16.840.1.113883.5.1" {
		t.Errorf("codeSystem = %q, want the fixed administrative-sex OID", got)
	}
	if got := gender.SelectAttrValue("displayName", ""); got != "Female" {
		t.Errorf("displayName = %q, want Female", got)
	}
	if root.FindElement("patient/raceCode") != nil {
		t.Error("expected no raceCode element when race data is absent")
	}
}

func TestWriteRepeatingGroup_OneElementPerItem(t *testing.T) {
	root := etree.NewElement("patientRole")
	data := map[string]interface{}{
		"ids": []interface{}{
			map[string]interface{}{"root": "2.16.840.1.113883.4.1", "extension": "PAT-001"},
			map[string]interface{}{"root": "2.16.840.1.113883.19.5", "extension": "MRN-002"},
		},
	}

	writeRepeatingGroup(root, data, "ids", "id", idItemFields)

	ids := root.SelectElements("id")
	if len(ids) != 2 {
		t.Fatalf("expected 2 <id> elements, got %d", len(ids))
	}
	if ids[0].SelectAttrValue("extension", "") != "PAT-001" || ids[1].SelectAttrValue("extension", "") != "MRN-002" {
		t.Errorf("ids not written in order: %v", ids)
	}
}

func TestWritePersonReference_NPIOnlyWhenPresent(t *testing.T) {
	root := etree.NewElement("documentationOf")

	// No npi in data — original behavior: no <id> at all for informant/performer
	// (unlike author, which always carries one — see writeNPI vs writePersonReference).
	withoutNPI := writePersonReference(root, "informant", map[string]interface{}{"given": "Jane"}, true)
	if withoutNPI.FindElement("assignedEntity/id") != nil {
		t.Error("expected no <id> when npi is absent")
	}

	withNPI := writePersonReference(root, "informant", map[string]interface{}{"npi": "1234567890"}, true)
	id := withNPI.FindElement("assignedEntity/id")
	if id == nil {
		t.Fatal("expected <id> when npi is present")
	}
	if id.SelectAttrValue("root", "") != npiOID {
		t.Errorf("id/@root = %q, want the NPI OID", id.SelectAttrValue("root", ""))
	}
	if id.SelectAttrValue("extension", "") != "1234567890" {
		t.Errorf("id/@extension = %q, want 1234567890", id.SelectAttrValue("extension", ""))
	}
}

func TestWritePersonReference_OrgOnlyWhenIncludeOrgTrue(t *testing.T) {
	root := etree.NewElement("documentationOf")
	data := map[string]interface{}{"given": "John", "orgName": "Acme Clinic"}

	withOrg := writePersonReference(root, "informant", data, true)
	if withOrg.FindElement("assignedEntity/representedOrganization/name") == nil {
		t.Error("expected representedOrganization when includeOrg=true and orgName present")
	}

	withoutOrg := writePersonReference(root, "performer", data, false)
	if withoutOrg.FindElement("assignedEntity/representedOrganization") != nil {
		t.Error("expected no representedOrganization when includeOrg=false (matches performer's original shape)")
	}
}
