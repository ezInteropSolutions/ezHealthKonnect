package builder

import "testing"

// TestSectionFieldCatalog_ConformanceThreading verifies Conformance is
// sourced from CDAFieldDef.Conformance on both the base field and its
// Display/System/Unit/Family companions (companions inherit the parent's
// level — see SectionFieldCatalog's own doc comment).
func TestSectionFieldCatalog_ConformanceThreading(t *testing.T) {
	loader := loadTestSchema(t)
	fields := SectionFieldCatalog(loader, "problems")
	if fields == nil {
		t.Fatal("expected non-nil field catalog for \"problems\"")
	}

	byKey := make(map[string]CanonicalFieldInfo, len(fields))
	for _, f := range fields {
		byKey[f.Key] = f
	}

	base, ok := byKey["conditionCode"]
	if !ok {
		t.Fatal("expected a \"conditionCode\" field in the problems catalog")
	}
	if base.Conformance == "" {
		t.Error("expected a non-empty Conformance on conditionCode")
	}
	if companion, ok := byKey["conditionCodeDisplay"]; ok && companion.Conformance != base.Conformance {
		t.Errorf("expected conditionCodeDisplay to inherit conditionCode's Conformance (%q), got %q", base.Conformance, companion.Conformance)
	}
}

// TestSectionFieldCatalog_NegationIndAndEntryRelationshipFields verifies
// this session's new attribute/entryRelationship fields reach the guided
// no-code catalog (cda.map_to_canonical's field picker) with the right
// spec-sourced conformance — problems.negationInd is MAY (CONF:1198-10139),
// immunizations.negationInd is SHALL (CONF:1198-8985, genuinely mandatory,
// unlike Problem/Allergy's MAY), and Problems' two new entryRelationship
// targets (Prognosis, Priority Preference) are addressable no-code fields.
func TestSectionFieldCatalog_NegationIndAndEntryRelationshipFields(t *testing.T) {
	loader := loadTestSchema(t)

	problemFields := SectionFieldCatalog(loader, "problems")
	byKey := make(map[string]CanonicalFieldInfo, len(problemFields))
	for _, f := range problemFields {
		byKey[f.Key] = f
	}
	if f, ok := byKey["negationInd"]; !ok || f.Conformance != "MAY" {
		t.Errorf("expected problems.negationInd MAY, got %+v (present=%v)", f, ok)
	}
	if _, ok := byKey["prognosisCode"]; !ok {
		t.Error("expected problems catalog to include prognosisCode")
	}
	if _, ok := byKey["priorityCode"]; !ok {
		t.Error("expected problems catalog to include priorityCode")
	}

	immFields := SectionFieldCatalog(loader, "immunizations")
	immByKey := make(map[string]CanonicalFieldInfo, len(immFields))
	for _, f := range immFields {
		immByKey[f.Key] = f
	}
	if f, ok := immByKey["negationInd"]; !ok || f.Conformance != "SHALL" {
		t.Errorf("expected immunizations.negationInd SHALL, got %+v (present=%v)", f, ok)
	}

	allergyFields := SectionFieldCatalog(loader, "allergiesAndIntolerances")
	allergyByKey := make(map[string]CanonicalFieldInfo, len(allergyFields))
	for _, f := range allergyFields {
		allergyByKey[f.Key] = f
	}
	if f, ok := allergyByKey["negationInd"]; !ok || f.Conformance != "MAY" {
		t.Errorf("expected allergiesAndIntolerances.negationInd MAY, got %+v (present=%v)", f, ok)
	}
}

// TestSectionRepeatingGroupCatalog_VitalSigns verifies the new per-item
// catalog endpoint surfaces Vital Signs' "components" RepeatingGroup — its
// groupedItemsKey plus per-component field vocabulary (vitalCode/value/
// componentEffectiveTime/interpretationCode/methodCode/targetSiteCode) — a
// DIFFERENT vocabulary than SectionFieldCatalog's own entry-level result for
// the same section (which now only carries the organizer-level
// effectiveTime), so the no-code UI can present the right picker for each of
// map_to_canonical's two field tables (per-item Fields vs. once-per-group
// EntryFields).
func TestSectionRepeatingGroupCatalog_VitalSigns(t *testing.T) {
	loader := loadTestSchema(t)

	group := SectionRepeatingGroupCatalog(loader, "vitalSigns")
	if group == nil {
		t.Fatal("expected a non-nil RepeatingGroup catalog for vitalSigns")
	}
	if group.Key != "components" {
		t.Errorf("group.Key = %q, want \"components\"", group.Key)
	}
	byKey := make(map[string]CanonicalFieldInfo, len(group.Fields))
	for _, f := range group.Fields {
		byKey[f.Key] = f
	}
	for _, want := range []string{"vitalCode", "vitalCodeDisplay", "value", "valueUnit", "componentEffectiveTime", "interpretationCode", "methodCode", "targetSiteCode"} {
		if _, ok := byKey[want]; !ok {
			t.Errorf("expected RepeatingGroup catalog to include %q, got keys: %+v", want, byKey)
		}
	}
	if f, ok := byKey["vitalCode"]; !ok || f.Conformance != "SHALL" {
		t.Errorf("expected vitalCode SHALL, got %+v (present=%v)", f, ok)
	}
	if f, ok := byKey["interpretationCode"]; !ok || f.Conformance != "MAY" {
		t.Errorf("expected interpretationCode MAY, got %+v (present=%v)", f, ok)
	}

	// The section-level (entry-level) catalog for the SAME section must be a
	// DIFFERENT, smaller vocabulary — the organizer's own shared field, not
	// the per-component fields the RepeatingGroup catalog just returned.
	entryFields := SectionFieldCatalog(loader, "vitalSigns")
	entryByKey := make(map[string]CanonicalFieldInfo, len(entryFields))
	for _, f := range entryFields {
		entryByKey[f.Key] = f
	}
	if _, ok := entryByKey["effectiveTime"]; !ok {
		t.Error("expected vitalSigns' entry-level catalog to include the organizer's own effectiveTime")
	}
}

// TestSectionRepeatingGroupCatalog_SectionWithNoGroup_ReturnsNil verifies a
// section that never declares a RepeatingGroup (the vast majority) gets a
// clean nil, not an empty-but-non-nil slice or a spurious error.
func TestSectionRepeatingGroupCatalog_SectionWithNoGroup_ReturnsNil(t *testing.T) {
	loader := loadTestSchema(t)
	if group := SectionRepeatingGroupCatalog(loader, "problems"); group != nil {
		t.Errorf("expected nil RepeatingGroup catalog for \"problems\", got: %+v", group)
	}
}

// TestSectionRepeatingGroupCatalog_UnknownSection_ReturnsNil mirrors
// SectionFieldCatalog's own unknown-section handling.
func TestSectionRepeatingGroupCatalog_UnknownSection_ReturnsNil(t *testing.T) {
	loader := loadTestSchema(t)
	if group := SectionRepeatingGroupCatalog(loader, "not-a-real-section"); group != nil {
		t.Errorf("expected nil for unknown section key, got: %+v", group)
	}
}

func TestSectionFieldCatalog_UnknownSection_ReturnsNil(t *testing.T) {
	loader := loadTestSchema(t)
	if fields := SectionFieldCatalog(loader, "not-a-real-section"); fields != nil {
		t.Errorf("expected nil for unknown section key, got: %+v", fields)
	}
}

func TestSectionCatalog_ExcludesHeaderSections(t *testing.T) {
	loader := loadTestSchema(t)
	sections := SectionCatalog(loader)
	for _, s := range sections {
		if s.Key == "header.patient" || s.Key == "header.author" || s.Key == "header.custodian" {
			t.Errorf("expected SectionCatalog to exclude header pseudo-section %q", s.Key)
		}
	}
}

func TestHeaderFieldCatalog_KnownGroups(t *testing.T) {
	for _, group := range []string{"patient", "author"} {
		if fields := HeaderFieldCatalog(group); len(fields) == 0 {
			t.Errorf("expected non-empty HeaderFieldCatalog for group %q", group)
		}
	}
	// Per decision #2 (custodian lives as structured cda.build config, not a
	// cda.map_to_canonical mapping target), "custodian" must NOT be a
	// recognized HeaderFieldCatalog group.
	if fields := HeaderFieldCatalog("custodian"); fields != nil {
		t.Errorf("expected HeaderFieldCatalog(\"custodian\") to remain nil, got: %+v", fields)
	}
}
