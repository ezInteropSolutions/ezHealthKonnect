// uscdi/vocabulary_test.go
//
// Coverage for the USCDIVocabulary index/lookup logic itself, plus a data-
// integrity check over cda/schemas/uscdi_v3.json's hand-curated cdaSection/
// cdaField values — the package had zero test coverage before the CDA
// Coverage Audit / Build Requirements USCDI-bridging work. A typo'd
// cdaSection or cdaField in the JSON would otherwise silently return zero
// matches forever (GetByCDASection just returns nil for an unknown key) —
// this test exists to fail loudly instead.
package uscdi_test

import (
	"path/filepath"
	"runtime"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	"ezhealthkonnect/uscdi"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// file = .../uscdi/vocabulary_test.go → Dir → .../uscdi → Dir → project root
	return filepath.Dir(filepath.Dir(file))
}

func loadVocabulary(t *testing.T) *uscdi.USCDIVocabulary {
	t.Helper()
	path := filepath.Join(projectRoot(t), "cda", "schemas", "uscdi_v3.json")
	vocab, err := uscdi.NewUSCDIVocabulary(path)
	if err != nil {
		t.Fatalf("failed to load USCDI vocabulary: %v", err)
	}
	return vocab
}

func loadVocabSchema(t *testing.T) *cdaSchema.CDASchemaLoader {
	t.Helper()
	loader, err := cdaSchema.NewCDASchemaLoader(filepath.Join(projectRoot(t), "cda", "schemas"))
	if err != nil {
		t.Fatalf("failed to load CDA schema: %v", err)
	}
	return loader
}

func TestGetByCDASection_KnownSections(t *testing.T) {
	vocab := loadVocabulary(t)

	if els := vocab.GetByCDASection("allergiesAndIntolerances"); len(els) != 3 {
		t.Errorf("GetByCDASection(allergiesAndIntolerances) = %d elements, want 3", len(els))
	}
	if els := vocab.GetByCDASection("medications"); len(els) == 0 {
		t.Error("GetByCDASection(medications) returned nothing — Medications was a known gap before this dataset expansion")
	}
	if els := vocab.GetByCDASection("not-a-real-section"); len(els) != 0 {
		t.Errorf("GetByCDASection(not-a-real-section) = %d elements, want 0", len(els))
	}
}

// TestGetByCDASection_MultiClassSections guards the finding that a single CDA
// section can legitimately carry elements from more than one USCDI class — a
// naive "one class per section" assumption in bridging code would silently
// drop classes. socialHistory is the richest real example: SDOH Assessment
// (Care Plan), Smoking/Pregnancy Status (Health Status Assessments), and
// Sexual Orientation/Gender Identity (Patient Demographics/Information) all
// live in this one CDA section.
func TestGetByCDASection_MultiClassSections(t *testing.T) {
	vocab := loadVocabulary(t)

	els := vocab.GetByCDASection("socialHistory")
	if len(els) == 0 {
		t.Fatal("expected socialHistory to have USCDI elements")
	}
	classes := make(map[string]bool)
	for _, el := range els {
		classes[el.Class] = true
	}
	for _, want := range []string{"Care Plan", "Health Status Assessments", "Patient Demographics/Information"} {
		if !classes[want] {
			t.Errorf("expected socialHistory to include class %q, got classes: %v", want, classes)
		}
	}
}

// TestGetByCDASection_HeaderGroupsResolve guards the header section-key
// granularity fix — uscdi_v3.json's header entries used to carry a flat
// "header" cdaSection that never matched Coverage Audit's own granular
// header.patient/header.author InventoryItem keys (inventory.go's
// buildHeaderInventory). GetByCDASection("header") returning elements while
// GetByCDASection("header.patient") returns nothing would be exactly that
// regression.
func TestGetByCDASection_HeaderGroupsResolve(t *testing.T) {
	vocab := loadVocabulary(t)

	if els := vocab.GetByCDASection("header.patient"); len(els) == 0 {
		t.Error("expected header.patient to resolve to Patient Demographics/Information elements")
	}
	if els := vocab.GetByCDASection("header.author"); len(els) == 0 {
		t.Error("expected header.author to resolve to Provenance elements")
	}
	if els := vocab.GetByCDASection("header"); len(els) != 0 {
		t.Errorf("expected bare \"header\" to resolve to nothing post-fix (all entries moved to header.<group>), got %d", len(els))
	}
}

// headerCanonicalKeys mirrors cda/builder's BUILD-direction canonical field
// vocabulary (header_fields.go's patientScalarFields/patientCodedFields/
// authorScalarFields, canonical_field_catalog.go's HeaderFieldCatalog) — the
// header.patient/header.author cdaField values in uscdi_v3.json are
// deliberately written against THIS vocabulary, not the PARSE-direction
// header.patient.json/header.author.json schema field keys (a documented,
// intentional split — see header_requirements.go's own file header comment).
var headerCanonicalKeys = map[string]map[string]bool{
	"header.patient": setOf(
		"firstName", "middleName", "lastName", "nameSuffix", "dateOfBirth", "preferredLanguage",
		"languageProficiency", "languageProficiencySystem", "languagePreferenceInd",
		"address", "phone", "phoneType", "email", "sex", "sexDisplay", "race", "raceDisplay",
		"ethnicity", "ethnicityDisplay", "maritalStatus", "maritalStatusDisplay", "ids",
		"deceasedInd", "deceasedTime", "names", "addresses",
	),
	"header.author": setOf(
		"given", "family", "npi", "specialtyCode", "specialtyCodeSystem",
		"specialtyCodeDisplay", "address", "phone",
	),
	"header.relatedPerson": setOf(
		"relationshipCode", "relationshipDisplay", "given", "family",
	),
	// No header.encompassingEncounter.json PARSE-direction schema file exists
	// (A2.4's own correction — the real PARSE→FHIR capability already lived
	// in services/cda_fhir's EncompassingEncounterLocationMappingRules, so a
	// new pseudo-section wasn't needed) — this set is
	// canonical_field_catalog.go's real HeaderFieldCatalog("encompassingEncounter")
	// keys (healthCareFacilityFields + headerBespokeFields["encompassingEncounter"]),
	// same BUILD-direction-vocabulary convention as every other header.* entry
	// in this map.
	"header.encompassingEncounter": setOf(
		"facilityTypeCode", "facilityName", "facilityOrgName",
		"id", "effectiveTimeLow", "effectiveTimeHigh", "dischargeDispositionCode",
	),
}

func setOf(keys ...string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out
}

// sectionFieldKeys collects every real field key for a body section — its
// own top-level Fields, plus RepeatingGroups[].Fields and
// AlternateArchetypes[].Fields, since several uscdi_v3.json entries
// (Medications' Indication/Indication Date) only exist inside a repeating
// group, not the section's flat field list.
func sectionFieldKeys(sec *cdaSchema.CDASectionDef) map[string]bool {
	keys := make(map[string]bool)
	for _, f := range sec.Fields {
		keys[f.Key] = true
	}
	for _, rg := range sec.RepeatingGroups {
		for _, f := range rg.Fields {
			keys[f.Key] = true
		}
	}
	for _, alt := range sec.AlternateArchetypes {
		for _, f := range alt.Fields {
			keys[f.Key] = true
		}
	}
	return keys
}

// TestVocabularyDataIntegrity_CDASectionAndFieldKeysResolve is the regression
// guard for the hand-curated dataset itself: every non-empty cdaSection must
// be a real, loadable CDA section (or a recognized header.<group> pseudo-
// section), and every non-empty cdaField must be a real field key within
// that section (body sections: schema Fields/RepeatingGroups/
// AlternateArchetypes; header sections: the BUILD-direction canonical
// vocabulary above). Catches exactly the class of bug found during Phase A
// authoring — e.g. a cdaSection of "insurance" (no such section; the real
// key is "payersInsurance") or a cdaField that only ever existed in a
// different, superficially-similar section.
func TestVocabularyDataIntegrity_CDASectionAndFieldKeysResolve(t *testing.T) {
	vocab := loadVocabulary(t)
	loader := loadVocabSchema(t)

	for _, class := range vocab.AllClasses() {
		for _, el := range vocab.GetByClass(class) {
			if el.CDASection == "" {
				if el.CDAField != "" {
					t.Errorf("%s/%s: cdaField %q set but cdaSection empty", el.Class, el.Element, el.CDAField)
				}
				continue
			}

			if headerKeys, isHeader := headerCanonicalKeys[el.CDASection]; isHeader {
				if el.CDAField != "" && !headerKeys[el.CDAField] {
					t.Errorf("%s/%s: cdaField %q not a recognized canonical key for %q", el.Class, el.Element, el.CDAField, el.CDASection)
				}
				continue
			}

			sec := loader.GetSection(el.CDASection)
			if sec == nil {
				t.Errorf("%s/%s: cdaSection %q does not resolve via CDASchemaLoader.GetSection", el.Class, el.Element, el.CDASection)
				continue
			}
			if el.CDAField != "" && !sectionFieldKeys(sec)[el.CDAField] {
				t.Errorf("%s/%s: cdaField %q not found in section %q's fields", el.Class, el.Element, el.CDAField, el.CDASection)
			}
		}
	}
}
