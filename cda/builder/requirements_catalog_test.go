package builder

import (
	"testing"

	"ezhealthkonnect/uscdi"
)

// loadTestVocabulary mirrors loadTestSchema's own "../schemas"-relative
// loading convention (document_builder_test.go), for the one real USCDI
// dataset (cda/schemas/uscdi_v3.json) this file's tests bridge into.
func loadTestVocabulary(t *testing.T) *uscdi.USCDIVocabulary {
	t.Helper()
	vocab, err := uscdi.NewUSCDIVocabulary("../schemas/uscdi_v3.json")
	if err != nil {
		t.Fatalf("failed to load USCDI vocabulary: %v", err)
	}
	return vocab
}

func TestGetDocumentTypeRequirements_CCD(t *testing.T) {
	loader := loadTestSchema(t)
	vocab := loadTestVocabulary(t)
	reqs := GetDocumentTypeRequirements(loader, vocab, "CCD")
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
	// CCD has 6 SHALL sections per Vol2 Table 30 + Companion Guide Table 18
	// (corrected from an earlier, uncorrected 7 — planOfTreatment was wrongly
	// SHALL, actually SHOULD) — see document_builder_test.go's
	// TestBuildDocument_RoundTripsThroughParserAndValidator for the same
	// count derived independently via the round-trip validator.
	if shallCount != 6 {
		t.Errorf("SHALL section count = %d, want 6", shallCount)
	}

	for _, group := range []string{"patient", "author", "custodian"} {
		fields, ok := reqs.HeaderGroups[group]
		if !ok || len(fields) == 0 {
			t.Errorf("expected non-empty header requirements for group %q", group)
		}
	}
}

// TestGetDocumentTypeRequirements_USCDIClasses_BridgedOntoSections is the
// Phase C proof: SectionRequirement.USCDIClasses resolves from the real,
// ONC-verified uscdi_v3.json dataset — same additive, honestly-absent-when-
// unknown principle Coverage Audit's own CategoryStat.USCDIClasses already
// established (services/cda_coverage). "problems" is a clean single-class
// section (Problems); "results" is the known multi-class case (Diagnostic
// Imaging + Laboratory) — the regression guard for a naive "one class per
// section" bridge silently dropping one.
func TestGetDocumentTypeRequirements_USCDIClasses_BridgedOntoSections(t *testing.T) {
	loader := loadTestSchema(t)
	vocab := loadTestVocabulary(t)
	reqs := GetDocumentTypeRequirements(loader, vocab, "CCD")
	if reqs == nil {
		t.Fatal("expected non-nil requirements for CCD")
	}

	byKey := make(map[string]SectionRequirement, len(reqs.Sections))
	for _, s := range reqs.Sections {
		byKey[s.Key] = s
	}

	problems, ok := byKey["problems"]
	if !ok {
		t.Fatal("expected a \"problems\" section in CCD's requirements")
	}
	if !containsString(problems.USCDIClasses, "Problems") {
		t.Errorf("expected problems.USCDIClasses to include \"Problems\", got %v", problems.USCDIClasses)
	}

	// "results" is the known multi-class case: Laboratory (its original
	// mapping) plus Clinical Tests (added later — the Results Section's own
	// scope statement is explicit it isn't lab-only, "laboratories, imaging
	// and other procedures"; Diagnostic Imaging itself maps to the separate
	// "findingsDIR" section instead, not "results").
	results, ok := byKey["results"]
	if !ok {
		t.Fatal("expected a \"results\" section in CCD's requirements")
	}
	for _, want := range []string{"Clinical Tests", "Laboratory"} {
		if !containsString(results.USCDIClasses, want) {
			t.Errorf("expected results.USCDIClasses to include %q, got %v", want, results.USCDIClasses)
		}
	}
}

// TestGetDocumentTypeRequirements_NilVocabulary_LeavesUSCDIClassesNil is the
// graceful-degradation guard — a nil vocabulary (e.g. uscdi_v3.json failed to
// load) must not panic or fabricate classes, matching how every other
// USCDIClasses bridge in this codebase degrades.
func TestGetDocumentTypeRequirements_NilVocabulary_LeavesUSCDIClassesNil(t *testing.T) {
	loader := loadTestSchema(t)
	reqs := GetDocumentTypeRequirements(loader, nil, "CCD")
	if reqs == nil {
		t.Fatal("expected non-nil requirements even with a nil vocabulary")
	}
	for _, s := range reqs.Sections {
		if s.USCDIClasses != nil {
			t.Errorf("expected nil USCDIClasses with a nil vocabulary, section %q got %v", s.Key, s.USCDIClasses)
		}
	}
}

func TestGetDocumentTypeRequirements_UnknownType_ReturnsNil(t *testing.T) {
	loader := loadTestSchema(t)
	vocab := loadTestVocabulary(t)
	if reqs := GetDocumentTypeRequirements(loader, vocab, "Not A Real Document Type"); reqs != nil {
		t.Errorf("expected nil for unknown document type, got: %+v", reqs)
	}
}

func TestGetDocumentTypeRequirements_DifferentDocTypesHaveDifferentSHALLSets(t *testing.T) {
	loader := loadTestSchema(t)
	vocab := loadTestVocabulary(t)
	ccd := GetDocumentTypeRequirements(loader, vocab, "CCD")
	discharge := GetDocumentTypeRequirements(loader, vocab, "Discharge Summary")
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
	vocab := loadTestVocabulary(t)
	fields := HeaderRequirementsCatalog(loader, vocab, "patient")
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

// TestHeaderRequirementsCatalog_USCDIClasses_BridgedOntoPatientFields is the
// Phase C header-field proof, incl. the dotted-canonicalKey wrinkle
// classesForHeaderField exists for: uscdi_v3.json's own header.patient entry
// for address uses the coarser bare "address" cdaField, but
// header_requirements.go's own translation table splits it into 4 UI-facing
// rows (address.street/.city/.state/.postalCode) — all 4 must still resolve.
func TestHeaderRequirementsCatalog_USCDIClasses_BridgedOntoPatientFields(t *testing.T) {
	loader := loadTestSchema(t)
	vocab := loadTestVocabulary(t)
	fields := HeaderRequirementsCatalog(loader, vocab, "patient")
	byKey := make(map[string]HeaderFieldRequirement, len(fields))
	for _, f := range fields {
		byKey[f.Key] = f
	}

	f, ok := byKey["firstName"]
	if !ok {
		t.Fatal("expected a \"firstName\" patient requirement")
	}
	if !containsString(f.USCDIClasses, "Patient Demographics/Information") {
		t.Errorf("expected firstName.USCDIClasses to include \"Patient Demographics/Information\", got %v", f.USCDIClasses)
	}

	street, ok := byKey["address.street"]
	if !ok {
		t.Fatal("expected an \"address.street\" patient requirement")
	}
	if !containsString(street.USCDIClasses, "Patient Demographics/Information") {
		t.Errorf("expected address.street.USCDIClasses to resolve via the coarser bare \"address\" cdaField, got %v", street.USCDIClasses)
	}
}

func TestHeaderRequirementsCatalog_AuthorExcludesAlwaysWrittenFields(t *testing.T) {
	loader := loadTestSchema(t)
	vocab := loadTestVocabulary(t)
	fields := HeaderRequirementsCatalog(loader, vocab, "author")
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
	vocab := loadTestVocabulary(t)
	fields := HeaderRequirementsCatalog(loader, vocab, "custodian")
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
	vocab := loadTestVocabulary(t)
	if fields := HeaderRequirementsCatalog(loader, vocab, "not-a-group"); fields != nil {
		t.Errorf("expected nil for unknown group, got: %+v", fields)
	}
}

// TestHeaderRequirementsCatalog_NilVocabulary_LeavesUSCDIClassesNil is the
// graceful-degradation guard, header-field counterpart to
// TestGetDocumentTypeRequirements_NilVocabulary_LeavesUSCDIClassesNil.
func TestHeaderRequirementsCatalog_NilVocabulary_LeavesUSCDIClassesNil(t *testing.T) {
	loader := loadTestSchema(t)
	fields := HeaderRequirementsCatalog(loader, nil, "patient")
	if len(fields) == 0 {
		t.Fatal("expected non-empty patient header requirements even with a nil vocabulary")
	}
	for _, f := range fields {
		if f.USCDIClasses != nil {
			t.Errorf("expected nil USCDIClasses with a nil vocabulary, key %q got %v", f.Key, f.USCDIClasses)
		}
	}
}
