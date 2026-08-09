package hl7

import "testing"

// testSchemaDir points at the same compiled schema fixtures every other
// hl7-adjacent test package already reads from (see e.g.
// services/executors/transform/hl7_build_executor_test.go's
// testHL7SchemaDir) — real ORU_R01.gz/ADT_A01.gz files, not synthetic
// fixtures, since the whole point of GroupTree is to reflect what these real
// schemas actually encode (confirmed by decompressing ORU_R01.gz by hand
// during planning: OBX's own "repeat" is "1", but it sits inside an
// "OBSERVATION" group whose "repeat" is "*").
const testSchemaDir = "../schemas/hl7"

func newTestLoader() *RealSchemaLoader {
	return &RealSchemaLoader{schemaDir: testSchemaDir, cache: make(map[string]*RealHL7Schema)}
}

// findGroupNode recursively searches nodes for a node named name, returning
// its own (non-cumulative) SchemaNode — used to assert what buildGroupTree
// captured before any cumulative Required/CanRepeat logic (that lives in
// hl7/builder.SchemaTree, tested separately) is applied.
func findGroupNode(nodes []*SchemaNode, name string) *SchemaNode {
	for _, n := range nodes {
		if n.Name == name {
			return n
		}
		if found := findGroupNode(n.Children, name); found != nil {
			return found
		}
	}
	return nil
}

func TestGroupTree_ORUR01_PreservesNestingAndOwnRepeat(t *testing.T) {
	loader := newTestLoader()
	schema, err := loader.LoadRealSchema("2.5.1", "ORU", "R01")
	if err != nil {
		t.Fatalf("LoadRealSchema failed: %v", err)
	}
	if len(schema.GroupTree) == 0 {
		t.Fatal("expected non-empty GroupTree for ORU_R01")
	}

	obsGroup := findGroupNode(schema.GroupTree, "OBSERVATION")
	if obsGroup == nil {
		t.Fatal("expected to find the OBSERVATION group node in GroupTree")
	}
	if obsGroup.Kind != "group" {
		t.Errorf("OBSERVATION Kind = %q, want %q", obsGroup.Kind, "group")
	}
	if obsGroup.Repeat != "*" {
		t.Errorf("OBSERVATION group Repeat = %q, want \"*\" (groups repeat, individual OBX doesn't need to)", obsGroup.Repeat)
	}

	obx := findGroupNode(obsGroup.Children, "OBX")
	if obx == nil {
		t.Fatal("expected to find OBX nested under the OBSERVATION group")
	}
	if obx.Kind != "segment" {
		t.Errorf("OBX Kind = %q, want %q", obx.Kind, "segment")
	}
	if obx.Repeat != "1" {
		t.Errorf("OBX's own Repeat = %q, want \"1\" — its real repeatability comes from the enclosing OBSERVATION group, not itself", obx.Repeat)
	}
	if obx.Usage != "R" {
		t.Errorf("OBX Usage = %q, want \"R\"", obx.Usage)
	}

	patientGroup := findGroupNode(schema.GroupTree, "PATIENT")
	if patientGroup == nil {
		t.Fatal("expected to find the PATIENT group node in GroupTree")
	}
	if patientGroup.Usage != "O" {
		t.Errorf("PATIENT group Usage = %q, want \"O\" (optional) — this is why PID isn't truly required overall despite its own usage=R", patientGroup.Usage)
	}
}

func TestGroupTree_DoesNotAffectFlatSegmentsMap(t *testing.T) {
	loader := newTestLoader()
	schema, err := loader.LoadRealSchema("2.5.1", "ORU", "R01")
	if err != nil {
		t.Fatalf("LoadRealSchema failed: %v", err)
	}
	// Regression guard: GroupTree is additive. Inbound parsing's flat
	// Segments/SegmentOrder must be unaffected by its addition.
	if _, ok := schema.Segments["OBX"]; !ok {
		t.Error("expected flat Segments map to still contain OBX")
	}
	if _, ok := schema.Segments["PID"]; !ok {
		t.Error("expected flat Segments map to still contain PID")
	}
	found := false
	for _, name := range schema.SegmentOrder {
		if name == "OBR" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected SegmentOrder to still contain OBR")
	}
}
