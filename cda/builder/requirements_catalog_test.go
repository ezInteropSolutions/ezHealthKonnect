package builder

import "testing"

func TestGetDocumentTypeRequirements_CCD(t *testing.T) {
	loader := loadTestSchema(t)
	reqs := GetDocumentTypeRequirements(loader, "CCD")
	if reqs == nil {
		t.Fatal("expected non-nil requirements for CCD")
	}
	if reqs.DocumentType != "CCD" {
		t.Errorf("DocumentType = %q, want \"CCD\"", reqs.DocumentType)
	}
	if reqs.Metadata == nil {
		t.Error("expected document metadata for CCD")
	}

	shallCount := 0
	for _, s := range reqs.Sections {
		if s.Conformance == "SHALL" {
			shallCount++
		}
	}
	// CCD has 7 SHALL sections per the C-CDA 2.1 IG (2018 errata) — see
	// document_builder_test.go's TestBuildDocument_RoundTripsThroughParserAndValidator
	// for the same count derived independently via the round-trip validator.
	if shallCount != 7 {
		t.Errorf("SHALL section count = %d, want 7", shallCount)
	}

	for _, group := range []string{"patient", "author", "custodian"} {
		fields, ok := reqs.HeaderGroups[group]
		if !ok || len(fields) == 0 {
			t.Errorf("expected non-empty header requirements for group %q", group)
		}
	}
}

func TestGetDocumentTypeRequirements_UnknownType_ReturnsNil(t *testing.T) {
	loader := loadTestSchema(t)
	if reqs := GetDocumentTypeRequirements(loader, "Not A Real Document Type"); reqs != nil {
		t.Errorf("expected nil for unknown document type, got: %+v", reqs)
	}
}

func TestGetDocumentTypeRequirements_DifferentDocTypesHaveDifferentSHALLSets(t *testing.T) {
	loader := loadTestSchema(t)
	ccd := GetDocumentTypeRequirements(loader, "CCD")
	discharge := GetDocumentTypeRequirements(loader, "Discharge Summary")
	if ccd == nil || discharge == nil {
		t.Fatal("expected requirements for both CCD and Discharge Summary")
	}

	shallKeys := func(r *DocumentTypeRequirements) map[string]bool {
		out := make(map[string]bool)
		for _, s := range r.Sections {
			if s.Conformance == "SHALL" {
				out[s.Key] = true
			}
		}
		return out
	}
	ccdShall, dischargeShall := shallKeys(ccd), shallKeys(discharge)
	if len(ccdShall) == 0 || len(dischargeShall) == 0 {
		t.Fatal("expected non-empty SHALL sets for both document types")
	}
	identical := len(ccdShall) == len(dischargeShall)
	if identical {
		for k := range ccdShall {
			if !dischargeShall[k] {
				identical = false
				break
			}
		}
	}
	if identical {
		t.Error("expected CCD and Discharge Summary to have different SHALL section sets — they diverge in the real IG")
	}
}

func TestHeaderRequirementsCatalog_PatientSHALLFields(t *testing.T) {
	loader := loadTestSchema(t)
	fields := HeaderRequirementsCatalog(loader, "patient")
	if len(fields) == 0 {
		t.Fatal("expected non-empty patient header requirements")
	}

	byKey := make(map[string]HeaderFieldRequirement, len(fields))
	for _, f := range fields {
		byKey[f.Key] = f
	}
	for _, wantSHALL := range []string{"ids", "firstName", "lastName", "dateOfBirth", "sex"} {
		f, ok := byKey[wantSHALL]
		if !ok {
			t.Errorf("expected patient requirement for canonical key %q", wantSHALL)
			continue
		}
		if f.Conformance != "SHALL" {
			t.Errorf("patient key %q: Conformance = %q, want SHALL", wantSHALL, f.Conformance)
		}
	}
	for _, wantSHOULD := range []string{"race", "ethnicity", "address.street", "phone"} {
		f, ok := byKey[wantSHOULD]
		if !ok {
			t.Errorf("expected patient requirement for canonical key %q", wantSHOULD)
			continue
		}
		if f.Conformance != "SHOULD" {
			t.Errorf("patient key %q: Conformance = %q, want SHOULD", wantSHOULD, f.Conformance)
		}
	}
}

func TestHeaderRequirementsCatalog_AuthorExcludesAlwaysWrittenFields(t *testing.T) {
	loader := loadTestSchema(t)
	fields := HeaderRequirementsCatalog(loader, "author")
	for _, f := range fields {
		if f.Key == "orgName" || f.Key == "time" {
			t.Errorf("expected author requirements to exclude always-written fields (orgName/time), found key %q", f.Key)
		}
	}
	byKey := make(map[string]HeaderFieldRequirement, len(fields))
	for _, f := range fields {
		byKey[f.Key] = f
	}
	if f, ok := byKey["given"]; !ok || f.Conformance != "SHALL" {
		t.Errorf("expected author 'given' to be SHALL, got %+v (present=%v)", f, ok)
	}
	if f, ok := byKey["npi"]; !ok || f.Conformance != "SHOULD" {
		t.Errorf("expected author 'npi' to be SHOULD, got %+v (present=%v)", f, ok)
	}
}

func TestHeaderRequirementsCatalog_CustodianHasNoAddressOrPhoneRequirement(t *testing.T) {
	loader := loadTestSchema(t)
	fields := HeaderRequirementsCatalog(loader, "custodian")
	byKey := make(map[string]HeaderFieldRequirement, len(fields))
	for _, f := range fields {
		byKey[f.Key] = f
	}
	if f, ok := byKey["name"]; !ok || f.Conformance != "SHALL" {
		t.Errorf("expected custodian 'name' to be SHALL, got %+v (present=%v)", f, ok)
	}
	// header.custodian in ccda_2_1.json defines no address/telecom fields at
	// all — cda.build's Custodian tab accepts them as optional data entry,
	// but the requirements catalog must not invent a requirement for them.
	if _, ok := byKey["address.street"]; ok {
		t.Error("expected no custodian address requirement — not asserted in the schema")
	}
	if _, ok := byKey["phone"]; ok {
		t.Error("expected no custodian phone requirement — not asserted in the schema")
	}
}

func TestHeaderRequirementsCatalog_UnknownGroup_ReturnsNil(t *testing.T) {
	loader := loadTestSchema(t)
	if fields := HeaderRequirementsCatalog(loader, "not-a-group"); fields != nil {
		t.Errorf("expected nil for unknown group, got: %+v", fields)
	}
}
