package hl7

import "testing"

func sf(key, value string) SubfieldInfo {
	return SubfieldInfo{Key: key, Value: value, HasValue: value != ""}
}

func TestBuildFieldPresenceWithGranularity_Disabled_MatchesBuildFieldPresence(t *testing.T) {
	msg := &EnhancedParsedMessage{
		SegmentGroups: map[string][]EnhancedSegment{
			"PID": {{Key: "PID", SegmentIndex: 0, Fields: []FieldInfo{
				{Key: "PID.5", Value: "DOE^JOHN", HasValue: true, Subfields: []SubfieldInfo{sf("PID.5.1", "DOE"), sf("PID.5.2", "JOHN")}},
			}}},
		},
	}
	withGranularity := BuildFieldPresenceWithGranularity(msg, false)
	plain := BuildFieldPresence(msg)
	if len(withGranularity) != 1 || len(plain) != 1 {
		t.Fatalf("expected exactly 1 field-level item from both, got %d and %d", len(withGranularity), len(plain))
	}
	if withGranularity[0].FieldPath != "PID.5" || plain[0].FieldPath != "PID.5" {
		t.Errorf("expected the coarse \"PID.5\" item from both when elementLevel=false, got %q and %q", withGranularity[0].FieldPath, plain[0].FieldPath)
	}
}

func TestBuildFieldPresenceWithGranularity_Enabled_DecomposesPopulatedSubfields(t *testing.T) {
	msg := &EnhancedParsedMessage{
		SegmentGroups: map[string][]EnhancedSegment{
			"PID": {{Key: "PID", SegmentIndex: 0, Fields: []FieldInfo{
				{Key: "PID.5", Value: "DOE^JOHN", HasValue: true, Subfields: []SubfieldInfo{sf("PID.5.1", "DOE"), sf("PID.5.2", "JOHN")}},
			}}},
		},
	}
	items := BuildFieldPresenceWithGranularity(msg, true)
	if len(items) != 2 {
		t.Fatalf("expected 2 component-level items (the coarse field item must NOT also appear), got %d: %+v", len(items), items)
	}
	byPath := map[string]FieldPresence{}
	for _, it := range items {
		byPath[it.FieldPath] = it
	}
	if v, ok := byPath["PID.5.1"]; !ok || v.Value != "DOE" {
		t.Errorf("expected PID.5.1=DOE, got %+v (ok=%v)", v, ok)
	}
	if v, ok := byPath["PID.5.2"]; !ok || v.Value != "JOHN" {
		t.Errorf("expected PID.5.2=JOHN, got %+v (ok=%v)", v, ok)
	}
	if _, ok := byPath["PID.5"]; ok {
		t.Errorf("expected the coarse \"PID.5\" item to be replaced by its own components, but it was still present")
	}
}

func TestBuildFieldPresenceWithGranularity_Enabled_NoPopulatedSubfields_FallsBackToField(t *testing.T) {
	msg := &EnhancedParsedMessage{
		SegmentGroups: map[string][]EnhancedSegment{
			"PID": {{Key: "PID", SegmentIndex: 0, Fields: []FieldInfo{
				// A field with a Subfields slice present, but none carrying
				// their own value (e.g. a simple non-composite field whose
				// schema still declares component positions).
				{Key: "PID.7", Value: "19800101", HasValue: true, Subfields: []SubfieldInfo{{Key: "PID.7.1", HasValue: false}}},
				// A field with no Subfields slice at all.
				{Key: "PID.8", Value: "M", HasValue: true},
			}}},
		},
	}
	items := BuildFieldPresenceWithGranularity(msg, true)
	if len(items) != 2 {
		t.Fatalf("expected 2 field-level fallback items, got %d: %+v", len(items), items)
	}
	paths := map[string]bool{}
	for _, it := range items {
		paths[it.FieldPath] = true
	}
	if !paths["PID.7"] || !paths["PID.8"] {
		t.Errorf("expected coarse field-level items for PID.7 and PID.8, got %+v", items)
	}
}

func TestBuildFieldPresenceWithGranularity_SegmentIndexPreservedOnComponents(t *testing.T) {
	// Repeating segment instances must each keep their own SegmentIndex on
	// their decomposed component items too, not just on field-level ones.
	msg := &EnhancedParsedMessage{
		SegmentGroups: map[string][]EnhancedSegment{
			"NK1": {
				{Key: "NK1", SegmentIndex: 0, Fields: []FieldInfo{
					{Key: "NK1.2", Value: "SMITH^JANE", HasValue: true, Subfields: []SubfieldInfo{sf("NK1.2.1", "SMITH")}},
				}},
				{Key: "NK1", SegmentIndex: 1, Fields: []FieldInfo{
					{Key: "NK1.2", Value: "JONES^BOB", HasValue: true, Subfields: []SubfieldInfo{sf("NK1.2.1", "JONES")}},
				}},
			},
		},
	}
	items := BuildFieldPresenceWithGranularity(msg, true)
	if len(items) != 2 {
		t.Fatalf("expected 2 component items (one per NK1 instance), got %d: %+v", len(items), items)
	}
	bySegmentIndex := map[int]string{}
	for _, it := range items {
		bySegmentIndex[it.SegmentIndex] = it.Value
	}
	if bySegmentIndex[0] != "SMITH" || bySegmentIndex[1] != "JONES" {
		t.Errorf("expected SegmentIndex 0 -> SMITH, 1 -> JONES, got %+v", bySegmentIndex)
	}
}
