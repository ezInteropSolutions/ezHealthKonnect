// services/executors/transform/cda_build_executor_test.go
package transform

import (
	"context"
	"strings"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// newTestCDABuildExecutor constructs a CDABuildExecutor with a real schema
// loader without going through NewCDABuildExecutor()'s hardcoded
// "./cda/schemas" path — that path is relative to the running process's
// CWD (correct at server runtime, from the repo root), not this test
// package's directory (`go test` runs with CWD = the package dir).
func newTestCDABuildExecutor(t *testing.T) *CDABuildExecutor {
	t.Helper()
	loader, err := cdaSchema.NewCDASchemaLoader("../../../cda/schemas")
	if err != nil {
		t.Fatalf("failed to load CDA schema: %v", err)
	}
	return &CDABuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("cda.build", models.ExecutorMetadata{
			Name: "CDA/CCD Document Builder", Category: "CDA Transform",
		}),
		schemaLoader: loader,
	}
}

func minimalCanonicalDoc() map[string]interface{} {
	return map[string]interface{}{
		"header": map[string]interface{}{
			"patient": map[string]interface{}{"firstName": "Jane", "lastName": "Doe"},
			"author":  map[string]interface{}{"given": "John", "family": "Smith"},
		},
		"sections": map[string]interface{}{},
	}
}

// TestCDABuildExecutor_CustodianConfigReachesXML verifies the step config's
// Custodian struct (services/executors/transform's own CdaCustodianConfig)
// actually reaches cda/builder.BuildDocument's output — the wiring this
// step's Execute() is responsible for, not BuildDocument's own XML-writing
// correctness (already covered by cda/builder/document_builder_test.go).
func TestCDABuildExecutor_CustodianConfigReachesXML(t *testing.T) {
	exec := newTestCDABuildExecutor(t)
	step := &models.TransformationStep{
		StepName: "Test Build CDA", StepType: "cda.build", Enabled: true,
		Config: map[string]interface{}{
			"sourceField":  "parsedCDA",
			"outputField":  "cdaXML",
			"documentType": "CCD",
			"custodian": map[string]interface{}{
				"idRoot": "2.16.840.1.113883.19.5", "idExtension": "CUST-42",
				"orgName": "Config-Wired Health System", "phone": "5551112222",
			},
		},
	}

	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"parsedCDA": minimalCanonicalDoc()})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	xml, ok := output["cdaXML"].(string)
	if !ok || xml == "" {
		t.Fatalf("expected a non-empty cdaXML output field, got %T", output["cdaXML"])
	}
	if !strings.Contains(xml, `root="2.16.840.1.113883.19.5"`) || !strings.Contains(xml, `extension="CUST-42"`) {
		t.Errorf("expected configured custodian id in output XML, got: %s", xml)
	}
	if !strings.Contains(xml, "<name>Config-Wired Health System</name>") {
		t.Errorf("expected configured custodian org name in output XML, got: %s", xml)
	}
	if !strings.Contains(xml, "tel:5551112222") {
		t.Errorf("expected configured custodian phone in output XML, got: %s", xml)
	}
}

// TestCDABuildExecutor_NoCustodianConfig_UsesLegacyOrgName verifies a
// pipeline saved before the Custodian tab existed (only the legacy flat
// "orgName" key) still produces the pre-existing custodian shape.
func TestCDABuildExecutor_NoCustodianConfig_UsesLegacyOrgName(t *testing.T) {
	exec := newTestCDABuildExecutor(t)
	step := &models.TransformationStep{
		StepName: "Test Build CDA", StepType: "cda.build", Enabled: true,
		Config: map[string]interface{}{
			"sourceField": "parsedCDA", "outputField": "cdaXML", "documentType": "CCD",
			"orgName": "Legacy Org",
		},
	}

	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"parsedCDA": minimalCanonicalDoc()})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	xml := output["cdaXML"].(string)
	if !strings.Contains(xml, "<name>Legacy Org</name>") {
		t.Errorf("expected custodian name to fall back to legacy orgName, got: %s", xml)
	}
}
