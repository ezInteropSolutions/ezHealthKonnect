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
