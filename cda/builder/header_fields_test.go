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
