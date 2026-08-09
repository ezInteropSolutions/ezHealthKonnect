package builder

import (
	"testing"

	"ezhealthkonnect/hl7"
)

// testHL7SchemaDir mirrors services/executors/transform/hl7_build_executor_test.go's
// own testHL7SchemaDir constant — same real compiled fixtures, just one
// fewer directory level up (this file lives in hl7/builder/, that one in
// services/executors/transform/).
const testHL7SchemaDir = "../../schemas/hl7"

func loadTestSchema(t *testing.T, messageType, triggerEvent string) *hl7.RealHL7Schema {
	t.Helper()
	hl7.InitRealSchemaLoader(testHL7SchemaDir)
	loader := hl7.GetRealSchemaLoader()
	if loader == nil {
		t.Fatal("expected non-nil HL7 schema loader after InitRealSchemaLoader")
	}
	schema, err := loader.LoadRealSchema("2.5.1", messageType, triggerEvent)
	if err != nil {
		t.Fatalf("LoadRealSchema(%s, %s) failed: %v", messageType, triggerEvent, err)
	}
	return schema
}

func findTreeNode(nodes []*SchemaTreeInfo, name string) *SchemaTreeInfo {
	for _, n := range nodes {
		if n.Name == name {
			return n
		}
		if found := findTreeNode(n.Children, name); found != nil {
			return found
		}
	}
	return nil
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// ADT_A01's schema is flat other than two optional/repeating groups
// (PROCEDURE wrapping PR1, INSURANCE wrapping IN1/IN2/IN3) — a clean case for
// verifying CanRepeat inheritance matches the exact "NK1/IN1 can repeat"
// behavior already expected of this message type.
func TestSchemaTree_ADTA01_CanRepeatInheritance(t *testing.T) {
	schema := loadTestSchema(t, "ADT", "A01")
	tree := SchemaTree(schema)

	cases := []struct {
		segment       string
		wantCanRepeat bool
		reason        string
	}{
		{"NK1", true, "NK1's own repeat is \"*\""},
		{"PID", false, "PID's own repeat is \"1\" and it isn't inside a repeating group"},
		{"IN1", true, "IN1's own repeat is \"1\" but it's inside the repeating INSURANCE group"},
		{"PR1", true, "PR1's own repeat is \"1\" but it's inside the repeating PROCEDURE group"},
	}
	for _, c := range cases {
		node := findTreeNode(tree, c.segment)
		if node == nil {
			t.Fatalf("expected to find %s in the schema tree", c.segment)
		}
		if node.CanRepeat != c.wantCanRepeat {
			t.Errorf("%s.CanRepeat = %v, want %v (%s)", c.segment, node.CanRepeat, c.wantCanRepeat, c.reason)
		}
	}
}

func TestSchemaTree_ADTA01_RequiredCumulative(t *testing.T) {
	schema := loadTestSchema(t, "ADT", "A01")
	tree := SchemaTree(schema)

	cases := []struct {
		segment      string
		wantRequired bool
	}{
		{"EVN", true},
		{"PID", true},
		{"PV1", true},
		{"PD1", false},
		{"PV2", false},
		{"IN1", false}, // own usage=R, but INSURANCE group is optional
	}
	for _, c := range cases {
		node := findTreeNode(tree, c.segment)
		if node == nil {
			t.Fatalf("expected to find %s in the schema tree", c.segment)
		}
		if node.Required != c.wantRequired {
			t.Errorf("%s.Required = %v, want %v", c.segment, node.Required, c.wantRequired)
		}
	}
}

// ORU_R01 exercises the opposite case: a segment with usage=R nested inside
// an OPTIONAL ancestor group is not truly required (PID sits inside the
// optional "PATIENT" group; OBX sits inside the optional "OBSERVATION"
// group) — confirmed against the real compiled schema during planning.
func TestSchemaTree_ORUR01_OptionalAncestorMakesRequiredFalse(t *testing.T) {
	schema := loadTestSchema(t, "ORU", "R01")
	tree := SchemaTree(schema)

	for _, segment := range []string{"PID", "OBX"} {
		node := findTreeNode(tree, segment)
		if node == nil {
			t.Fatalf("expected to find %s in the schema tree", segment)
		}
		if node.Required {
			t.Errorf("%s.Required = true, want false (its ancestor group is optional)", segment)
		}
	}

	obx := findTreeNode(tree, "OBX")
	if !obx.CanRepeat {
		t.Error("OBX.CanRepeat = false, want true (inherited from its immediate parent, the repeating OBSERVATION group)")
	}

	obr := findTreeNode(tree, "OBR")
	if obr == nil || !obr.Required {
		t.Error("expected OBR.Required = true (the one segment whose entire ancestor chain is required)")
	}
}

// Regression test: ORU_R01's outer "PATIENT RESULT" group repeats (batched
// multi-patient results), but PID's own immediate parent, "PATIENT", does
// NOT (repeat="1" — exactly one patient per PATIENT RESULT occurrence). A
// grandparent-level repeat must not leak down to make PID look repeatable —
// per spec, PID can only ever be single. Caught via live manual testing
// after the initial CanRepeat implementation used a fully cumulative OR
// across the whole ancestor chain instead of just the immediate parent.
func TestSchemaTree_ORUR01_GrandparentGroupRepeatDoesNotLeakToPID(t *testing.T) {
	schema := loadTestSchema(t, "ORU", "R01")
	tree := SchemaTree(schema)

	patientResult := findTreeNode(tree, "PATIENT RESULT")
	if patientResult == nil || !patientResult.CanRepeat {
		t.Fatal("expected PATIENT RESULT.CanRepeat = true (precondition: the outer group must actually repeat for this test to be meaningful)")
	}
	patientGroup := findTreeNode(tree, "PATIENT")
	if patientGroup == nil {
		t.Fatal("expected to find the PATIENT group node in the schema tree")
	}
	if patientGroup.Repeat != "1" {
		t.Fatalf("PATIENT group's own repeat = %q, want \"1\" (precondition for this regression case)", patientGroup.Repeat)
	}

	pid := findTreeNode(tree, "PID")
	if pid == nil {
		t.Fatal("expected to find PID in the schema tree")
	}
	if pid.CanRepeat {
		t.Error("PID.CanRepeat = true, want false — PID's immediate parent (PATIENT) doesn't repeat; only the grandparent (PATIENT RESULT) does, which must not leak down")
	}
}

func TestRequiredSpine_ADTA01_ExcludesMSHAndOptionalSegments(t *testing.T) {
	schema := loadTestSchema(t, "ADT", "A01")
	spine := RequiredSpine(schema)
	want := []string{"EVN", "PID", "PV1"}
	if len(spine) != len(want) {
		t.Fatalf("RequiredSpine = %v, want %v", spine, want)
	}
	for i, name := range want {
		if spine[i] != name {
			t.Errorf("RequiredSpine[%d] = %q, want %q (spine: %v)", i, spine[i], name, spine)
		}
	}
}

func TestRequiredSpine_ORUR01_OnlyOBR(t *testing.T) {
	schema := loadTestSchema(t, "ORU", "R01")
	spine := RequiredSpine(schema)
	if len(spine) != 1 || spine[0] != "OBR" {
		t.Errorf("RequiredSpine = %v, want [OBR] (every other segment sits under an optional group in this schema)", spine)
	}
}

func TestNextAllowedSegments_ADTA01_FreshStart_StopsAtFirstRequiredSlot(t *testing.T) {
	schema := loadTestSchema(t, "ADT", "A01")
	allowed, all := NextAllowedSegments(schema, nil)

	// EVN is the first required checkpoint after (optional) SFT — the
	// guardrail must not offer PID yet until EVN is actually added, since
	// EVN precedes PID in the real message structure.
	want := []string{"SFT", "EVN"}
	if len(allowed) != len(want) {
		t.Fatalf("allowed = %v, want %v", allowed, want)
	}
	for i, name := range want {
		if allowed[i] != name {
			t.Errorf("allowed[%d] = %q, want %q (allowed: %v)", i, allowed[i], name, allowed)
		}
	}
	if contains(allowed, "PID") {
		t.Error("allowed should not yet include PID — EVN must be added first")
	}
	if !contains(all, "PID") || !contains(all, "IN1") {
		t.Error("all (the override list) should contain every schema segment regardless of what's been added")
	}
	if contains(all, "MSH") {
		t.Error("all should exclude MSH — it's auto-populated, never a user-added segment")
	}
}

func TestNextAllowedSegments_ADTA01_AfterRequiredSpine_OpensUpRemainingSegments(t *testing.T) {
	schema := loadTestSchema(t, "ADT", "A01")
	// NK1 (sequence 7) actually precedes PV1 (sequence 8) in the real
	// ADT_A01 structure — confirmed against the schema fixture directly —
	// so it must NOT reappear once PV1 has been matched; PV2/ROL/OBX/IN1/PR1
	// all sit after PV1 and remain reachable.
	allowed, _ := NextAllowedSegments(schema, []string{"EVN", "PID", "PV1"})

	for _, name := range []string{"PV2", "ROL", "OBX", "IN1", "PR1"} {
		if !contains(allowed, name) {
			t.Errorf("allowed = %v, expected it to contain %q (PV1 was the last required checkpoint)", allowed, name)
		}
	}
	for _, name := range []string{"EVN", "PID", "NK1"} {
		if contains(allowed, name) {
			t.Errorf("allowed = %v, did not expect %q — the guardrail is forward-only and NK1 precedes PV1 in this schema", allowed, name)
		}
	}
}

func TestNextAllowedSegments_RepeatInPlace_OffersAnotherOfLastRepeatableSegment(t *testing.T) {
	schema := loadTestSchema(t, "ADT", "A01")
	// PD1 (optional, between PID and NK1) is deliberately skipped to exercise
	// "match in order, tolerate gaps" — NK1 ends up the last matched slot.
	allowed, _ := NextAllowedSegments(schema, []string{"EVN", "PID", "NK1"})
	if !contains(allowed, "NK1") {
		t.Errorf("allowed = %v, expected NK1 to remain offered (it can repeat)", allowed)
	}
	if !contains(allowed, "PV1") {
		t.Errorf("allowed = %v, expected PV1 to be reachable next", allowed)
	}
	for _, name := range []string{"EVN", "PID"} {
		if contains(allowed, name) {
			t.Errorf("allowed = %v, did not expect %q — already passed", allowed, name)
		}
	}
}

func TestAvailableVersions_ListsKnownVersionsSortedNumerically(t *testing.T) {
	versions := AvailableVersions(testHL7SchemaDir)
	for _, v := range []string{"2.3", "2.5.1", "2.7.1"} {
		if !contains(versions, v) {
			t.Errorf("AvailableVersions() = %v, expected it to contain %q", versions, v)
		}
	}
	for _, v := range versions {
		if len(v) > 0 && (v[0] == 'v' || v[0] == 'V') {
			t.Errorf("AvailableVersions() returned %q with a leftover v-prefix", v)
		}
	}
}

func TestSegmentFieldCatalog_SurfacesRequiredAndCanRepeat(t *testing.T) {
	schema := loadTestSchema(t, "ADT", "A01")
	fields := SegmentFieldCatalog(schema, "PID")

	var pid1, pid3 *CanonicalFieldInfo
	for i := range fields {
		switch fields[i].Key {
		case "PID.1":
			pid1 = &fields[i]
		case "PID.3":
			pid3 = &fields[i]
		}
	}
	if pid1 == nil || pid3 == nil {
		t.Fatalf("expected PID.1 and PID.3 in catalog, got %+v", fields)
	}
	if pid1.Required || pid1.CanRepeat {
		t.Errorf("PID.1 = %+v, want Required=false, CanRepeat=false", pid1)
	}
	if !pid3.Required || !pid3.CanRepeat {
		t.Errorf("PID.3 = %+v, want Required=true, CanRepeat=true", pid3)
	}
}
