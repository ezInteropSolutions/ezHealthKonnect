package transform

import (
	"context"
	"ezhealthkonnect/models"
	"testing"
)

// makeRDStep builds a TransformationStep with the given config map.
func makeRDStep(cfg map[string]interface{}) *models.TransformationStep {
	return &models.TransformationStep{
		StepType: "remove_duplicates",
		StepName: "Test Dedup",
		Config:   cfg,
	}
}

// makeRDInput wraps records in the expected pipeline input structure.
func makeRDInput(field string, records []interface{}) map[string]interface{} {
	return map[string]interface{}{
		field: records,
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// Helper: run executor and assert no error
// ────────────────────────────────────────────────────────────────────────────────

func runDedup(t *testing.T, cfg map[string]interface{}, input map[string]interface{}) map[string]interface{} {
	t.Helper()
	exec := NewRemoveDuplicatesExecutor()
	out, err := exec.Execute(context.Background(), makeRDStep(cfg), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	return out
}

func getRecords(t *testing.T, out map[string]interface{}, field string) []interface{} {
	t.Helper()
	v, ok := out[field]
	if !ok {
		t.Fatalf("output field %q not found; keys=%v", field, keys(out))
	}
	records, ok := v.([]interface{})
	if !ok {
		t.Fatalf("field %q is not []interface{}, got %T", field, v)
	}
	return records
}

func keys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-1: strategy=first (default) — keeps first, drops subsequent duplicates
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_StrategyFirst(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"id": "A", "name": "Alice"},
		map[string]interface{}{"id": "B", "name": "Bob"},
		map[string]interface{}{"id": "A", "name": "Alice-Dup"},
	}
	out := runDedup(t, map[string]interface{}{
		"sourceField": "items",
		"keyFields":   []string{"id"},
		"strategy":    "first",
	}, makeRDInput("items", records))

	got := getRecords(t, out, "items")
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	first := got[0].(map[string]interface{})
	if first["name"] != "Alice" {
		t.Errorf("expected 'Alice' (first occurrence), got %q", first["name"])
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-2: strategy=last — overwrites with most recent duplicate
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_StrategyLast(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"id": "A", "name": "Alice"},
		map[string]interface{}{"id": "A", "name": "Alice-Updated"},
	}
	out := runDedup(t, map[string]interface{}{
		"sourceField": "items",
		"keyFields":   []string{"id"},
		"strategy":    "last",
	}, makeRDInput("items", records))

	got := getRecords(t, out, "items")
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	if got[0].(map[string]interface{})["name"] != "Alice-Updated" {
		t.Errorf("expected 'Alice-Updated', got %v", got[0])
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-3: strategy=merge — keeps first, fills missing fields from later duplicates
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_StrategyMerge(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"id": "A", "name": "Alice"},
		map[string]interface{}{"id": "A", "name": "SHOULD-NOT-OVERRIDE", "dob": "1990-01-01"},
	}
	out := runDedup(t, map[string]interface{}{
		"sourceField": "items",
		"keyFields":   []string{"id"},
		"strategy":    "merge",
	}, makeRDInput("items", records))

	got := getRecords(t, out, "items")
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	rec := got[0].(map[string]interface{})
	if rec["name"] != "Alice" {
		t.Errorf("merge: 'name' should keep first value 'Alice', got %v", rec["name"])
	}
	if rec["dob"] != "1990-01-01" {
		t.Errorf("merge: 'dob' should be filled from second record, got %v", rec["dob"])
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-4: full-record hash (empty keyFields) — hashes entire record
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_FullRecordHash(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"id": "A", "name": "Alice"},
		map[string]interface{}{"id": "A", "name": "Alice"}, // exact duplicate
		map[string]interface{}{"id": "A", "name": "Different"},
	}
	out := runDedup(t, map[string]interface{}{
		"sourceField": "items",
		"keyFields":   []string{},
	}, makeRDInput("items", records))

	got := getRecords(t, out, "items")
	if len(got) != 2 {
		t.Fatalf("expected 2 records (1 exact dup removed), got %d", len(got))
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-5: caseSensitive=false — "SMITH" == "smith"
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_CaseInsensitive(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"name": "Smith"},
		map[string]interface{}{"name": "SMITH"},
		map[string]interface{}{"name": "Jones"},
	}
	out := runDedup(t, map[string]interface{}{
		"sourceField":   "items",
		"keyFields":     []string{"name"},
		"caseSensitive": false,
	}, makeRDInput("items", records))

	got := getRecords(t, out, "items")
	if len(got) != 2 {
		t.Fatalf("case-insensitive: expected 2 records, got %d", len(got))
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-6: caseSensitive=true (default) — "Smith" != "SMITH"
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_CaseSensitive(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"name": "Smith"},
		map[string]interface{}{"name": "SMITH"},
	}
	out := runDedup(t, map[string]interface{}{
		"sourceField":   "items",
		"keyFields":     []string{"name"},
		"caseSensitive": true,
	}, makeRDInput("items", records))

	got := getRecords(t, out, "items")
	if len(got) != 2 {
		t.Fatalf("case-sensitive: expected 2 records (different keys), got %d", len(got))
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-7: nullKeyBehavior=keep — records with missing key are always kept
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_NullKey_Keep(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"id": "A"},
		map[string]interface{}{"name": "no-id-1"}, // key field missing
		map[string]interface{}{"name": "no-id-2"}, // key field missing
	}
	out := runDedup(t, map[string]interface{}{
		"sourceField":     "items",
		"keyFields":       []string{"id"},
		"nullKeyBehavior": "keep",
	}, makeRDInput("items", records))

	got := getRecords(t, out, "items")
	if len(got) != 3 {
		t.Fatalf("nullKey=keep: expected 3 records, got %d", len(got))
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-8: nullKeyBehavior=remove — records with missing key are dropped
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_NullKey_Remove(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"id": "A"},
		map[string]interface{}{"name": "no-id"}, // key field missing — should be dropped
	}
	out := runDedup(t, map[string]interface{}{
		"sourceField":     "items",
		"keyFields":       []string{"id"},
		"nullKeyBehavior": "remove",
	}, makeRDInput("items", records))

	got := getRecords(t, out, "items")
	if len(got) != 1 {
		t.Fatalf("nullKey=remove: expected 1 record, got %d", len(got))
	}
	if got[0].(map[string]interface{})["id"] != "A" {
		t.Errorf("expected record with id=A to survive, got %v", got[0])
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-9: nullKeyBehavior=group — null-key records share one bucket; strategy applies
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_NullKey_Group_First(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"name": "no-id-1"}, // key missing → grouped
		map[string]interface{}{"name": "no-id-2"}, // key missing → grouped; strategy=first drops this
	}
	out := runDedup(t, map[string]interface{}{
		"sourceField":     "items",
		"keyFields":       []string{"id"},
		"nullKeyBehavior": "group",
		"strategy":        "first",
	}, makeRDInput("items", records))

	got := getRecords(t, out, "items")
	if len(got) != 1 {
		t.Fatalf("nullKey=group+first: expected 1 record, got %d", len(got))
	}
	if got[0].(map[string]interface{})["name"] != "no-id-1" {
		t.Errorf("expected 'no-id-1' (first), got %v", got[0])
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-10: outputField — writes result to a different field, sourceField unchanged
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_OutputField(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"id": "A"},
		map[string]interface{}{"id": "A"},
	}
	input := makeRDInput("source_arr", records)
	out := runDedup(t, map[string]interface{}{
		"sourceField": "source_arr",
		"keyFields":   []string{"id"},
		"outputField": "deduped_arr",
	}, input)

	deduped := getRecords(t, out, "deduped_arr")
	if len(deduped) != 1 {
		t.Fatalf("outputField: expected 1 record in deduped_arr, got %d", len(deduped))
	}
	// source_arr should also be present (in-place not modified; shallow copy)
	if _, ok := out["source_arr"]; !ok {
		t.Error("source_arr should still be present in output")
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-11: missing sourceField config → error
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_MissingSourceField_Error(t *testing.T) {
	exec := NewRemoveDuplicatesExecutor()
	_, err := exec.Execute(context.Background(), makeRDStep(map[string]interface{}{}), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing sourceField, got nil")
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-12: sourceField not found in input → no-op (returns input unchanged)
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_SourceFieldNotFound_NoOp(t *testing.T) {
	exec := NewRemoveDuplicatesExecutor()
	input := map[string]interface{}{"other_field": "value"}
	out, err := exec.Execute(context.Background(), makeRDStep(map[string]interface{}{
		"sourceField": "nonexistent",
	}), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["other_field"] != "value" {
		t.Error("no-op should return input unchanged")
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-13: Validate — invalid strategy rejected
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_Validate_InvalidStrategy(t *testing.T) {
	exec := NewRemoveDuplicatesExecutor()
	err := exec.Validate(&models.TransformationStep{
		Config: map[string]interface{}{
			"sourceField": "items",
			"strategy":    "bogus",
		},
	})
	if err == nil {
		t.Fatal("expected validation error for invalid strategy")
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-14: Validate — invalid nullKeyBehavior rejected
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_Validate_InvalidNullKeyBehavior(t *testing.T) {
	exec := NewRemoveDuplicatesExecutor()
	err := exec.Validate(&models.TransformationStep{
		Config: map[string]interface{}{
			"sourceField":     "items",
			"nullKeyBehavior": "banana",
		},
	})
	if err == nil {
		t.Fatal("expected validation error for invalid nullKeyBehavior")
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-15: empty array — no-op, returns empty slice
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_EmptyArray(t *testing.T) {
	out := runDedup(t, map[string]interface{}{
		"sourceField": "items",
		"keyFields":   []string{"id"},
	}, makeRDInput("items", []interface{}{}))

	got := getRecords(t, out, "items")
	if len(got) != 0 {
		t.Fatalf("empty input: expected 0 records, got %d", len(got))
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// TC-16: step output variables are set (dedup_count, removed_count, etc.)
// ────────────────────────────────────────────────────────────────────────────────

func TestRemoveDuplicates_StepOutputVariables(t *testing.T) {
	records := []interface{}{
		map[string]interface{}{"id": "A"},
		map[string]interface{}{"id": "A"}, // duplicate
		map[string]interface{}{"id": "B"},
	}
	out := runDedup(t, map[string]interface{}{
		"sourceField": "items",
		"keyFields":   []string{"id"},
	}, makeRDInput("items", records))

	stepOut, ok := out["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("_stepOutput not found in output")
	}
	if stepOut["original_count"] != 3 {
		t.Errorf("original_count: want 3, got %v", stepOut["original_count"])
	}
	if stepOut["dedup_count"] != 2 {
		t.Errorf("dedup_count: want 2, got %v", stepOut["dedup_count"])
	}
	if stepOut["removed_count"] != 1 {
		t.Errorf("removed_count: want 1, got %v", stepOut["removed_count"])
	}
}
