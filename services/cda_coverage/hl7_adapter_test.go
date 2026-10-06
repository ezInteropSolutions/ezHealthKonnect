package cdacoverage

import (
	"context"
	"encoding/json"
	"testing"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/services"
	"ezhealthkonnect/services/executors"
)

// buildTestEnvelope marshals a synthetic *hl7.EnhancedParsedMessage the same
// way real parsed content is persisted, then unmarshals it back into a plain
// map[string]interface{} — exactly the shape decodeEnhancedParsedMessage
// receives from real object storage, so this test exercises the real
// round-trip rather than a hand-typed map that might not match the real JSON
// tag names.
func buildTestEnvelope(t *testing.T, msg *hl7.EnhancedParsedMessage) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return envelope
}

func field(key, value string) hl7.FieldInfo {
	return hl7.FieldInfo{Key: key, Value: value, HasValue: value != ""}
}

// subfield builds one populated SubfieldInfo — key is the full
// "SEGMENT.FIELD.COMPONENT" shape (e.g. "PID.5.1"), matching exactly what
// resolveHL7FieldValue and FieldMapping.HL7Component/getDefaultMappingsForTesting
// already use.
func subfield(key, value string) hl7.SubfieldInfo {
	return hl7.SubfieldInfo{Key: key, Value: value, HasValue: value != ""}
}

// fieldWithSubfields is a composite HL7 field (e.g. PID.5, an XPN name) that
// decomposes into real, populated components.
func fieldWithSubfields(key, value string, subfields ...hl7.SubfieldInfo) hl7.FieldInfo {
	f := field(key, value)
	f.Subfields = subfields
	return f
}

// testMessage builds a synthetic ADT^A01 message with MSH (must be excluded
// entirely regardless of any mapping), PID (has both mapped and unmapped
// populated fields — services.getDefaultMappingsForTesting's own known,
// deterministic ADT^A01 set is used since the adapter is constructed with a
// nil-db transform service, which falls through to that fallback, avoiding
// any real Postgres dependency for this test), and two PV1... actually a
// single PV1 with one mapped field and one unmapped populated field.
func testMessage() *hl7.EnhancedParsedMessage {
	return &hl7.EnhancedParsedMessage{
		MessageType: hl7.MessageTypeInfo{Code: "ADT", Event: "A01", Name: "ADT^A01"},
		SegmentGroups: map[string][]hl7.EnhancedSegment{
			"MSH": {{
				Key: "MSH", SegmentIndex: 0,
				Fields: []hl7.FieldInfo{
					field("MSH.9", "ADT^A01"), // mapped in the default set, but MSH is excluded entirely
					field("MSH.10", "MSG00001"),
				},
			}},
			"PID": {{
				Key: "PID", SegmentIndex: 0,
				Fields: []hl7.FieldInfo{
					field("PID.3", "12345"),      // mapped
					field("PID.5", "DOE^JOHN"),   // mapped
					field("PID.7", "19800101"),   // mapped
					field("PID.19", "999-99-999"), // NOT in the default mapping set — a genuine gap
				},
			}},
			"PV1": {{
				Key: "PV1", SegmentIndex: 0,
				Fields: []hl7.FieldInfo{
					field("PV1.2", "I"),      // mapped
					field("PV1.99", "extra"), // not a real HL7 field, but exercises "unmapped populated field" either way
				},
			}},
		},
	}
}

func TestHL7Adapter_BuildInventory_ExcludesMSHEntirely(t *testing.T) {
	adapter := &hl7Adapter{transformSvc: services.NewHL7FHIRTransformServiceV3(nil)}
	envelope := buildTestEnvelope(t, testMessage())
	tracker := executors.NewCDACoverageTracker()

	items, err := adapter.BuildInventory(context.Background(), "", envelope, false, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, item := range items {
		if item.Category == "MSH" {
			t.Errorf("expected MSH fields to be excluded entirely from the inventory, found %+v", item)
		}
	}
}

func TestHL7Adapter_BuildInventory_MappedFieldsRecordedAsTouched(t *testing.T) {
	adapter := &hl7Adapter{transformSvc: services.NewHL7FHIRTransformServiceV3(nil)}
	envelope := buildTestEnvelope(t, testMessage())
	tracker := executors.NewCDACoverageTracker()

	items, err := adapter.BuildInventory(context.Background(), "", envelope, false, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byPath := map[string]InventoryItem{}
	for _, item := range items {
		byPath[item.SectionKey] = item
	}

	for _, want := range []string{"PID.3", "PID.5", "PID.7", "PV1.2"} {
		item, ok := byPath[want]
		if !ok {
			t.Fatalf("expected inventory item for %q, got items=%v", want, byPath)
		}
		if !tracker.Touched(item.TrackingKey()) {
			t.Errorf("expected %q (a field with a real, default-set mapping) to be recorded as touched, key=%q", want, item.TrackingKey())
		}
	}
}

func TestHL7Adapter_BuildInventory_UnmappedPopulatedFieldsAreGaps(t *testing.T) {
	adapter := &hl7Adapter{transformSvc: services.NewHL7FHIRTransformServiceV3(nil)}
	envelope := buildTestEnvelope(t, testMessage())
	tracker := executors.NewCDACoverageTracker()

	items, err := adapter.BuildInventory(context.Background(), "", envelope, false, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byPath := map[string]InventoryItem{}
	for _, item := range items {
		byPath[item.SectionKey] = item
	}

	for _, want := range []string{"PID.19", "PV1.99"} {
		item, ok := byPath[want]
		if !ok {
			t.Fatalf("expected inventory item for %q (it IS populated in the message), got items=%v", want, byPath)
		}
		if tracker.Touched(item.TrackingKey()) {
			t.Errorf("expected %q (populated but not in the default mapping set) to be a real gap, but it was recorded as touched", want)
		}
	}
}

// TestHL7Adapter_BuildInventory_LiveTrackedField_CoversEvenWithoutCatalogEntry
// is the direct counterpart to the gap test above, and the cross-package
// exact-key proof for the 2026-10 live-tracking addition: PID.19 has no
// mapping in the default catalog (same fixture, same genuine-gap field), but
// this time the tracker ALREADY has its bare key recorded BEFORE
// BuildInventory runs -- simulating exactly what extractHL7ValueAtomic
// (services/transform_hl7_extractor.go) or resolveHL7FieldValue
// (services/executors/field_utils.go) would have done during a real message's
// actual processing. Confirms the adapter's new tracker.Touched(p.FieldPath)
// OR-check picks this up as real coverage -- and, critically, that the key
// format BOTH sides use ("SEGMENT.FIELD", no "#N" suffix) genuinely matches;
// a drift here would have this test fail exactly the way the project's own
// history warns about (CLAUDE.md's "lesson that must not be relearned").
func TestHL7Adapter_BuildInventory_LiveTrackedField_CoversEvenWithoutCatalogEntry(t *testing.T) {
	adapter := &hl7Adapter{transformSvc: services.NewHL7FHIRTransformServiceV3(nil)}
	envelope := buildTestEnvelope(t, testMessage())
	tracker := executors.NewCDACoverageTracker()
	// Simulate a real, live field read of PID.19 having already happened
	// during this message's actual HL7->FHIR processing.
	tracker.Record("PID.19")

	items, err := adapter.BuildInventory(context.Background(), "", envelope, false, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byPath := map[string]InventoryItem{}
	for _, item := range items {
		byPath[item.SectionKey] = item
	}

	item, ok := byPath["PID.19"]
	if !ok {
		t.Fatalf("expected inventory item for \"PID.19\", items=%v", byPath)
	}
	if !tracker.Touched(item.TrackingKey()) {
		t.Errorf("expected \"PID.19\" to be covered via live tracking despite having no catalog entry, but it was reported as a gap")
	}

	// PV1.99 (also unmapped, also populated) was NEVER live-tracked -- must
	// remain a genuine gap, proving the OR-check doesn't blanket-cover
	// everything once ANY field is live-tracked.
	pv199, ok := byPath["PV1.99"]
	if !ok {
		t.Fatalf("expected inventory item for \"PV1.99\", items=%v", byPath)
	}
	if tracker.Touched(pv199.TrackingKey()) {
		t.Errorf("expected \"PV1.99\" to remain a genuine gap (never live-tracked, not in the catalog), but it was covered")
	}
}

func TestHL7Adapter_BuildInventory_NoMessageType_ReturnsNil(t *testing.T) {
	adapter := &hl7Adapter{transformSvc: services.NewHL7FHIRTransformServiceV3(nil)}
	envelope := buildTestEnvelope(t, &hl7.EnhancedParsedMessage{
		SegmentGroups: map[string][]hl7.EnhancedSegment{"PID": {{Key: "PID", Fields: []hl7.FieldInfo{field("PID.3", "1")}}}},
	})
	items, err := adapter.BuildInventory(context.Background(), "", envelope, false, executors.NewCDACoverageTracker())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil inventory when message type is empty, got %v", items)
	}
}

// elementLevelTestMessage builds an ADT^A01 message with real, composite
// fields whose subfields genuinely decompose — reusing
// getDefaultMappingsForTesting's OWN known field/component set (PID.5 has
// component-level mappings for .1/.2/.3; PID.11 has only a FIELD-level
// mapping with no component; PID.19 has no mapping at all) so this test
// exercises the REAL default mapping catalog, not an invented one.
func elementLevelTestMessage() *hl7.EnhancedParsedMessage {
	return &hl7.EnhancedParsedMessage{
		MessageType: hl7.MessageTypeInfo{Code: "ADT", Event: "A01", Name: "ADT^A01"},
		SegmentGroups: map[string][]hl7.EnhancedSegment{
			"PID": {{
				Key: "PID", SegmentIndex: 0,
				Fields: []hl7.FieldInfo{
					// PID.5 (XPN name): component-level mappings exist for
					// .1 (family) and .2 (given[0]) in the default set, but
					// NOT for a 3rd, made-up component -- genuine per-
					// component coverage to prove.
					fieldWithSubfields("PID.5", "DOE^JOHN",
						subfield("PID.5.1", "DOE"),
						subfield("PID.5.2", "JOHN"),
					),
					// PID.11 (XAD address): only a FIELD-level mapping
					// exists (HL7Component == "") -- proves the whole-field
					// mapping covers every one of its own components
					// (ancestor rollup).
					fieldWithSubfields("PID.11", "123 MAIN ST^^SPRINGFIELD",
						subfield("PID.11.1", "123 MAIN ST"),
						subfield("PID.11.3", "SPRINGFIELD"),
					),
					// PID.19 (SSN): not in the default mapping set at all --
					// every component here must be a genuine, independent gap.
					fieldWithSubfields("PID.19", "999-99-9999^EXTRA",
						subfield("PID.19.1", "999-99-9999"),
						subfield("PID.19.2", "EXTRA"),
					),
					// PID.7: has a Subfields slice, but NONE of them carry
					// their own value -- must fall back to the coarse
					// field-level item, not decompose into empty components.
					fieldWithSubfields("PID.7", "19800101",
						hl7.SubfieldInfo{Key: "PID.7.1", HasValue: false},
					),
				},
			}},
		},
	}
}

func TestHL7Adapter_ElementLevel_ComponentMapping_ExactKeyTouched(t *testing.T) {
	adapter := &hl7Adapter{transformSvc: services.NewHL7FHIRTransformServiceV3(nil)}
	envelope := buildTestEnvelope(t, elementLevelTestMessage())
	tracker := executors.NewCDACoverageTracker()

	items, err := adapter.BuildInventory(context.Background(), "", envelope, true, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byPath := map[string]InventoryItem{}
	for _, item := range items {
		byPath[item.SectionKey] = item
	}

	// The coarse "PID.5" item must NOT be present -- it decomposed into its
	// own components, and emitting both would double-count one real value.
	if _, ok := byPath["PID.5"]; ok {
		t.Errorf("expected the coarse \"PID.5\" item to be replaced by its own components, but it was still present: %v", byPath)
	}

	for _, want := range []string{"PID.5.1", "PID.5.2"} {
		item, ok := byPath[want]
		if !ok {
			t.Fatalf("expected a component-level item for %q, items=%v", want, byPath)
		}
		// Unlike FHIR/EDI/NCPDP's EntryIndex:-1 sentinel, HL7 items always
		// carry a real, non-negative SegmentIndex (0 here, a non-repeating
		// PID) -- TrackingKey() therefore always appends the CDA-style
		// "#N" suffix, same as every pre-existing field-level HL7 item.
		wantKey := want + "#0"
		if item.TrackingKey() != wantKey {
			t.Errorf("expected TrackingKey() for %q to be %q, got %q", want, wantKey, item.TrackingKey())
		}
		if !tracker.Touched(item.TrackingKey()) {
			t.Errorf("expected %q (a real component-level mapping in the default set) to be touched, tracker snapshot=%v", want, tracker.Snapshot())
		}
	}
}

func TestHL7Adapter_ElementLevel_WholeFieldMapping_CoversEveryComponent(t *testing.T) {
	adapter := &hl7Adapter{transformSvc: services.NewHL7FHIRTransformServiceV3(nil)}
	envelope := buildTestEnvelope(t, elementLevelTestMessage())
	tracker := executors.NewCDACoverageTracker()

	items, err := adapter.BuildInventory(context.Background(), "", envelope, true, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byPath := map[string]InventoryItem{}
	for _, item := range items {
		byPath[item.SectionKey] = item
	}

	// PID.11 is mapped at the WHOLE-FIELD level only (no component specified
	// in the default set) -- every one of its own real components must still
	// show as covered, since reading the whole composite field necessarily
	// reads everything inside it.
	for _, want := range []string{"PID.11.1", "PID.11.3"} {
		item, ok := byPath[want]
		if !ok {
			t.Fatalf("expected a component-level item for %q, items=%v", want, byPath)
		}
		if !tracker.Touched(item.TrackingKey()) {
			t.Errorf("expected %q to be covered by PID.11's own whole-field mapping, but it was not", want)
		}
	}
}

func TestHL7Adapter_ElementLevel_UnmappedField_EveryComponentIsAGenuineGap(t *testing.T) {
	adapter := &hl7Adapter{transformSvc: services.NewHL7FHIRTransformServiceV3(nil)}
	envelope := buildTestEnvelope(t, elementLevelTestMessage())
	tracker := executors.NewCDACoverageTracker()

	items, err := adapter.BuildInventory(context.Background(), "", envelope, true, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byPath := map[string]InventoryItem{}
	for _, item := range items {
		byPath[item.SectionKey] = item
	}

	// PID.19 has no mapping at all (field-level or component-level) in the
	// default set -- neither of its real components should be swept into
	// coverage by any rollup.
	for _, want := range []string{"PID.19.1", "PID.19.2"} {
		item, ok := byPath[want]
		if !ok {
			t.Fatalf("expected a component-level item for %q, items=%v", want, byPath)
		}
		if tracker.Touched(item.TrackingKey()) {
			t.Errorf("expected %q to be a genuine, unmapped gap, but it was recorded as touched", want)
		}
	}
}

func TestHL7Adapter_ElementLevel_SubfieldsWithNoValue_FallsBackToCoarseFieldItem(t *testing.T) {
	adapter := &hl7Adapter{transformSvc: services.NewHL7FHIRTransformServiceV3(nil)}
	envelope := buildTestEnvelope(t, elementLevelTestMessage())
	tracker := executors.NewCDACoverageTracker()

	items, err := adapter.BuildInventory(context.Background(), "", envelope, true, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, item := range items {
		if item.SectionKey == "PID.7" {
			found = true
			if !tracker.Touched(item.TrackingKey()) {
				t.Errorf("expected PID.7 (a real, field-level default-set mapping) to be touched")
			}
		}
	}
	if !found {
		t.Errorf("expected PID.7 to still be reported as its own coarse item (its Subfields carry no real value), items=%v", items)
	}
}

func TestHL7Adapter_ElementLevel_Disabled_PreservesFieldLevelOnlyBehavior(t *testing.T) {
	// Regression guard: elementLevel=false must reproduce today's exact
	// field-level-only inventory, completely unaffected by this enhancement
	// — even for a message whose fields DO have real, populated subfields.
	adapter := &hl7Adapter{transformSvc: services.NewHL7FHIRTransformServiceV3(nil)}
	envelope := buildTestEnvelope(t, elementLevelTestMessage())
	tracker := executors.NewCDACoverageTracker()

	items, err := adapter.BuildInventory(context.Background(), "", envelope, false, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, item := range items {
		if item.SectionKey == "PID.5.1" || item.SectionKey == "PID.11.1" {
			t.Errorf("expected no component-level items when elementLevel=false, found %q", item.SectionKey)
		}
	}
	found := false
	for _, item := range items {
		if item.SectionKey == "PID.5" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the coarse \"PID.5\" item when elementLevel=false, items=%v", items)
	}
}

func TestHL7ParentFieldKey(t *testing.T) {
	cases := []struct {
		path       string
		wantParent string
		wantOK     bool
	}{
		{"PID.5.1", "PID.5", true},
		{"PID.5", "", false},
		{"PID", "", false},
		{"PID.5.1.2", "", false}, // deeper than this adapter's model supports
	}
	for _, c := range cases {
		gotParent, gotOK := hl7ParentFieldKey(c.path)
		if gotParent != c.wantParent || gotOK != c.wantOK {
			t.Errorf("hl7ParentFieldKey(%q) = (%q, %v), want (%q, %v)", c.path, gotParent, gotOK, c.wantParent, c.wantOK)
		}
	}
}

func TestBuildFieldPresence_OnlyPopulatedFieldsIncluded(t *testing.T) {
	msg := &hl7.EnhancedParsedMessage{
		SegmentGroups: map[string][]hl7.EnhancedSegment{
			"OBX": {
				{Key: "OBX", SegmentIndex: 0, Fields: []hl7.FieldInfo{field("OBX.5", "14.2"), field("OBX.6", "")}},
				{Key: "OBX", SegmentIndex: 1, Fields: []hl7.FieldInfo{field("OBX.5", "7.1"), field("OBX.6", "")}},
			},
		},
	}
	presence := hl7.BuildFieldPresence(msg)
	if len(presence) != 2 {
		t.Fatalf("expected 2 populated fields (OBX.6 has no value in either instance), got %d: %+v", len(presence), presence)
	}
	if presence[0].SegmentIndex == presence[1].SegmentIndex {
		t.Errorf("expected the two OBX instances to have distinct SegmentIndex values, both were %d", presence[0].SegmentIndex)
	}
}
