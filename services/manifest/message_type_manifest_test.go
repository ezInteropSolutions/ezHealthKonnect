// services/manifest/message_type_manifest_test.go
//
// Regression coverage for FilterResources' switch from a hand-maintained
// per-trigger-event allow-list to suppress-only filtering (see FilterResources'
// doc comment). The motivating bug: "A08"'s AllowedResources omitted
// AllergyIntolerance even though AL1 is a normal segment on real ADT^A08
// messages, so a correctly-built AllergyIntolerance resource was silently
// stripped. These tests prove: (1) FilterResources no longer excludes a
// resource type just because AllowedResources doesn't list it, (2) explicit
// interface-level "suppress" overrides still work, (3) AllowedResources /
// Lookup() themselves are untouched, since transformation_scorer.go still
// reads them directly as the quality-scoring baseline — filtering and scoring
// are now decoupled, not both broken or both silently changed together.
package manifest

import "testing"

func TestFilterResources_NoLongerGatesOnAllowedResources(t *testing.T) {
	resources := []map[string]interface{}{
		{"resourceType": "Patient"},
		{"resourceType": "AllergyIntolerance"}, // NOT in A08's AllowedResources
	}

	filtered, removed := FilterResources(resources, "ADT^A08", nil)

	if len(removed) != 0 {
		t.Errorf("expected nothing removed with no resourcePolicy, got removed=%v", removed)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected both resources to pass through, got %d: %+v", len(filtered), filtered)
	}

	var sawAllergy bool
	for _, r := range filtered {
		if r["resourceType"] == "AllergyIntolerance" {
			sawAllergy = true
		}
	}
	if !sawAllergy {
		t.Error("AllergyIntolerance was filtered out even though it's no longer gated by AllowedResources")
	}
}

func TestFilterResources_SuppressOverrideStillRemoves(t *testing.T) {
	resources := []map[string]interface{}{
		{"resourceType": "Patient"},
		{"resourceType": "Coverage"},
	}

	filtered, removed := FilterResources(resources, "ADT^A08", map[string]string{"Coverage": "suppress"})

	if len(filtered) != 1 || filtered[0]["resourceType"] != "Patient" {
		t.Fatalf("expected only Patient to survive a Coverage suppress override, got %+v", filtered)
	}
	if len(removed) != 1 || removed[0] != "Coverage" {
		t.Fatalf("expected removed=[Coverage], got %v", removed)
	}
}

func TestFilterResources_AllowOverrideIsHarmlessNoOp(t *testing.T) {
	resources := []map[string]interface{}{{"resourceType": "AllergyIntolerance"}}

	filtered, removed := FilterResources(resources, "ADT^A08", map[string]string{"AllergyIntolerance": "allow"})

	if len(removed) != 0 || len(filtered) != 1 {
		t.Fatalf("an 'allow' override should be a no-op now that nothing is excluded by default, got filtered=%+v removed=%v", filtered, removed)
	}
}

func TestFilterResources_UnknownMessageTypeStillPassesThrough(t *testing.T) {
	resources := []map[string]interface{}{{"resourceType": "Whatever"}}

	filtered, removed := FilterResources(resources, "ZZZ^Z99", nil)

	if len(removed) != 0 || len(filtered) != 1 {
		t.Fatalf("a message type with no manifest entry should pass everything through, got filtered=%+v removed=%v", filtered, removed)
	}
}

// TestLookup_AllowedResourcesUnchangedForScoring proves AllowedResources still
// reflects the original curated "expected resources" list — transformation_scorer.go
// reads this directly (never through FilterResources) as its quality baseline,
// so this data must NOT change just because filtering no longer uses it.
func TestLookup_AllowedResourcesUnchangedForScoring(t *testing.T) {
	m := Lookup("ADT^A08")
	if m == nil {
		t.Fatal("expected a manifest entry for A08")
	}
	for _, rt := range m.AllowedResources {
		if rt == "AllergyIntolerance" {
			t.Error("A08's AllowedResources should still NOT list AllergyIntolerance — this field is the scoring baseline, unrelated to the FilterResources fix, and must stay as originally curated")
		}
	}
}
