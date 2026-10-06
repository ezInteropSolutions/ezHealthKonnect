package cdacoverage

import (
	"context"
	"testing"

	"ezhealthkonnect/services/executors"
)

func testPatientEnvelope() map[string]interface{} {
	return map[string]interface{}{
		"resourceType":   "Patient",
		"id":             "123",
		"meta":           map[string]interface{}{"versionId": "1"},
		"name":           []interface{}{map[string]interface{}{"family": "Doe"}},
		"birthDate":      "1980-01-01",
		"deceasedString": nil, // present as a key, but nil value — must be excluded
		"raw":            "{...}",
		"_format":        "fhir",
		"parsedAt":       "2026-09-26T00:00:00Z",
		"schemaLoaded":   true,
		"enhancedFields": map[string]interface{}{},
		"_coverageTracker": "placeholder", // exercises the underscore-prefix exclusion rule; real value is a *executors.CDACoverageTracker, but exclusion happens by key name alone
	}
}

func TestFHIRAdapter_BuildInventory_ExcludesAdministrativeAndBookkeepingFields(t *testing.T) {
	adapter := &fhirAdapter{}
	items, err := adapter.BuildInventory(context.Background(), "", testPatientEnvelope(), false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byPath := map[string]InventoryItem{}
	for _, item := range items {
		byPath[item.SectionKey] = item
	}

	for _, excluded := range []string{"id", "meta", "raw", "_format", "parsedAt", "schemaLoaded", "enhancedFields", "resourceType", "deceasedString", "_coverageTracker"} {
		if _, ok := byPath[excluded]; ok {
			t.Errorf("expected %q to be excluded (administrative, envelope bookkeeping, or nil-valued), but it was in the inventory", excluded)
		}
	}
	for _, included := range []string{"name", "birthDate"} {
		if _, ok := byPath[included]; !ok {
			t.Errorf("expected %q to be a real inventory item, items=%v", included, byPath)
		}
	}
}

func TestFHIRAdapter_BuildInventory_NoResourceType_ReturnsNil(t *testing.T) {
	adapter := &fhirAdapter{}
	items, err := adapter.BuildInventory(context.Background(), "", map[string]interface{}{"foo": "bar"}, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil inventory for a non-FHIR envelope, got %v", items)
	}
}

// TestFHIRAdapter_TrackingKey_MatchesRealLiveTrackingHook is the mandatory
// exact-key proof this whole feature's design doc calls for (see the plan's
// own "lesson that must not be relearned" section): it drives a REAL field
// read through the REAL resolution path a fhir.build/field_mapping/etc.
// sourcePath would use (executors.GetFieldValue) and confirms the tracker key
// that produces is byte-identical to what this adapter's own
// InventoryItem.TrackingKey() computes for the same field — not merely a
// substring/shape check.
//
// This test's FIRST version used a "Patient.name"-prefixed path (the
// intuitively-obvious guess) and genuinely failed here: DetectPathType's
// IsFHIRPath check does match "Patient.name", routing it to
// resolveFHIRFieldValue → resolveJSONPathValue(fhirData, "Patient.name") —
// but fhirData IS this same flat envelope (no "resource"/"fhirBundle"
// wrapper present), which has no NESTED "Patient" key to walk into, so the
// read genuinely returned nil. A real sourcePath reading this resource's own
// field is the BARE key ("name") — which has no HL7/FHIR/CDA path shape at
// all, so it falls through to the generic PathTypeJSON branch instead. This
// is exactly the "byte-identical path convention" risk this test exists to
// catch, caught before shipping rather than in production.
func TestFHIRAdapter_TrackingKey_MatchesRealLiveTrackingHook(t *testing.T) {
	tracker := executors.NewCDACoverageTracker()
	envelope := testPatientEnvelope()
	envelope["_coverageTracker"] = tracker

	// Exactly what a real fhir.build/field_mapping sourcePath reading this
	// inbound resource's own "name" field actually looks like.
	got := executors.GetFieldValue(envelope, "name")
	if got == nil {
		t.Fatalf("expected a real value back from GetFieldValue, got nil")
	}

	adapter := &fhirAdapter{}
	items, err := adapter.BuildInventory(context.Background(), "", envelope, false, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var nameItem *InventoryItem
	for i := range items {
		if items[i].SectionKey == "name" {
			nameItem = &items[i]
		}
	}
	if nameItem == nil {
		t.Fatalf("expected an inventory item for \"name\", items=%v", items)
	}
	if nameItem.TrackingKey() != "name" {
		t.Fatalf("expected TrackingKey() to be the bare path \"name\" (no \"#N\" suffix), got %q", nameItem.TrackingKey())
	}
	if !tracker.Touched(nameItem.TrackingKey()) {
		t.Errorf("expected the real GetFieldValue read to have recorded exactly the same key the inventory item computes, tracker snapshot=%v", tracker.Snapshot())
	}
}

func TestInventoryItem_TrackingKey_NegativeEntryIndex_OmitsSuffix(t *testing.T) {
	item := InventoryItem{SectionKey: "Patient.name", EntryIndex: -1}
	if got := item.TrackingKey(); got != "Patient.name" {
		t.Errorf("expected bare SectionKey with no \"#N\" suffix, got %q", got)
	}
}

// TestFHIRAdapter_ElementLevel_DecomposesNestedField_ExactKeyMatchesRealTrackingHook
// is the element-level equivalent of TestFHIRAdapter_TrackingKey_
// MatchesRealLiveTrackingHook above: it drives a REAL nested field read
// through executors.GetFieldValue (the same resolution path a fhir.build
// sourcePath actually uses) and confirms the produced tracker key is
// byte-identical to what fhirLeafPaths/TrackingKey() compute for the same
// leaf — never a substring/shape check, per this whole feature's own
// standing "exact key" discipline.
func TestFHIRAdapter_ElementLevel_DecomposesNestedField_ExactKeyMatchesRealTrackingHook(t *testing.T) {
	tracker := executors.NewCDACoverageTracker()
	envelope := testPatientEnvelope()
	envelope["_coverageTracker"] = tracker
	envelope["name"] = []interface{}{
		map[string]interface{}{
			"family": "Doe",
			"given":  []interface{}{"Jane", "Q"},
		},
	}

	// Exactly what a real fhir.build/field_mapping sourcePath drilling into
	// one specific given name looks like.
	got := executors.GetFieldValue(envelope, "name[0].given[0]")
	if got != "Jane" {
		t.Fatalf("expected \"Jane\" back from GetFieldValue, got %v", got)
	}

	adapter := &fhirAdapter{}
	items, err := adapter.BuildInventory(context.Background(), "", envelope, true, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byKey := map[string]InventoryItem{}
	for _, item := range items {
		byKey[item.SectionKey] = item
	}

	// The coarse "name" item must NOT also be present -- it was fully
	// decomposed into leaves, and emitting both would double-count one real
	// data point as two separate report entries.
	if _, ok := byKey["name"]; ok {
		t.Errorf("expected the coarse \"name\" item to be replaced by its own leaves, but it was still present: %v", byKey)
	}

	wantLeaves := []string{"name[0].family", "name[0].given[0]", "name[0].given[1]"}
	for _, leaf := range wantLeaves {
		item, ok := byKey[leaf]
		if !ok {
			t.Fatalf("expected a leaf item for %q, items=%v", leaf, byKey)
		}
		if item.TrackingKey() != leaf {
			t.Errorf("expected TrackingKey() for %q to be the bare leaf path (no \"#N\" suffix), got %q", leaf, item.TrackingKey())
		}
	}

	// Only the ACTUALLY-read leaf ("name[0].given[0]") should show as
	// touched -- its untouched siblings ("name[0].family", "name[0].given[1]")
	// must genuinely show up as real, honest gaps, not be swept in by an
	// over-broad rollup.
	touchedLeaf := byKey["name[0].given[0]"]
	if !tracker.Touched(touchedLeaf.TrackingKey()) {
		t.Errorf("expected the real GetFieldValue read to have recorded exactly this leaf's own key, tracker snapshot=%v", tracker.Snapshot())
	}
	for _, untouchedLeaf := range []string{"name[0].family", "name[0].given[1]"} {
		item := byKey[untouchedLeaf]
		if tracker.Touched(item.TrackingKey()) {
			t.Errorf("expected %q to be a genuine, untouched gap, but it was marked covered", untouchedLeaf)
		}
	}
}

// TestFHIRAdapter_ElementLevel_WholeContainerRead_CoversEveryLeaf proves
// reconcileFHIRAncestorCoverage's own reason for existing: a real sourcePath
// reading a container FIELD wholesale (e.g. copying "name" through
// unprocessed, never drilling into a specific given/family) must mark every
// one of that field's own leaves covered too -- report.go's own isCovered
// ancestor rollup never fires here (it only activates for CDA/HL7-style
// "/"-delimited keys), so without this adapter-level reconciliation every
// leaf under a wholesale-read container would show as a false-positive gap.
func TestFHIRAdapter_ElementLevel_WholeContainerRead_CoversEveryLeaf(t *testing.T) {
	tracker := executors.NewCDACoverageTracker()
	envelope := testPatientEnvelope()
	envelope["_coverageTracker"] = tracker
	envelope["name"] = []interface{}{
		map[string]interface{}{"family": "Doe", "given": []interface{}{"Jane"}},
	}

	// A real sourcePath that reads the WHOLE "name" array as one opaque
	// value, e.g. to pass it straight through to an outbound builder.
	got := executors.GetFieldValue(envelope, "name")
	if got == nil {
		t.Fatalf("expected a real value back from GetFieldValue, got nil")
	}

	adapter := &fhirAdapter{}
	items, err := adapter.BuildInventory(context.Background(), "", envelope, true, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, item := range items {
		if item.SectionKey == "name[0].family" || item.SectionKey == "name[0].given[0]" {
			if !tracker.Touched(item.TrackingKey()) {
				t.Errorf("expected %q to be covered by the wholesale \"name\" read, but it was not", item.SectionKey)
			}
		}
	}
}

// TestFHIRAdapter_ElementLevel_ScalarField_FallsBackToCoarseItem proves a
// field with nothing to decompose (already a scalar, e.g. "birthDate") is
// still reported exactly as the entry-level tier always has been -- element
// level only ever ADDS decomposition for fields that genuinely have nested
// structure, it never removes coverage for a field that doesn't.
func TestFHIRAdapter_ElementLevel_ScalarField_FallsBackToCoarseItem(t *testing.T) {
	adapter := &fhirAdapter{}
	items, err := adapter.BuildInventory(context.Background(), "", testPatientEnvelope(), true, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, item := range items {
		if item.SectionKey == "birthDate" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the scalar \"birthDate\" field to still be reported as its own coarse item, items=%v", items)
	}
}

func TestFHIRAncestorCandidates_StripsBracketAndDotSegmentsInOrder(t *testing.T) {
	got := fhirAncestorCandidates("name[0].given[0]")
	want := []string{"name[0].given[0]", "name[0].given", "name[0]", "name"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: expected %q, got %q (full: %v)", i, want[i], got[i], got)
		}
	}
}

func TestInventoryItem_TrackingKey_NonNegativeEntryIndex_Unchanged(t *testing.T) {
	// Regression guard: CDA/HL7 both always use a real, non-negative
	// EntryIndex — this must be completely unaffected by the new sentinel.
	item := InventoryItem{SectionKey: "medications", EntryIndex: 2}
	if got := item.TrackingKey(); got != "medications#2" {
		t.Errorf("expected unchanged CDAEntryKey-style output, got %q", got)
	}
}
