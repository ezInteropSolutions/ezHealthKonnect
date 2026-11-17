package transform

import (
	"context"
	"strings"
	"testing"

	"ezhealthkonnect/models"
)

func TestHL7Build_TriggerOverrides(t *testing.T) {
	initHL7Schema(t)
	cfg := func(over map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"messageType": "ADT", "version": "2.5.1",
			"triggerEventPath":         "type",
			"triggerEventMap":          map[string]interface{}{"record.updated": "A31", "identity.created": "A28"},
			"triggerEventMapOverrides": over,
			"messageStructureMap":      map[string]interface{}{"A28": "ADT_A05", "A31": "ADT_A05", "A04": "ADT_A01", "A08": "ADT_A01"},
			"segments":                 []interface{}{map[string]interface{}{"segment": "EVN", "fields": []interface{}{map[string]interface{}{"fieldKey": "EVN.1", "literalValue": "x"}}}},
		}
	}
	msh9 := func(c map[string]interface{}, typ string) string {
		return strings.Split(hl7Lines(t, runHL7Build(t, c, map[string]interface{}{"type": typ}), "hl7Message")[0], "|")[8]
	}
	// no override: the template's own table
	if got := msh9(cfg(nil), "record.updated"); got != "ADT^A31^ADT_A05" {
		t.Fatalf("default: %q", got)
	}
	// an override wins, case-insensitively, with its own structure; others keep the default
	over := map[string]interface{}{"record.updated": " a08 "}
	if got := msh9(cfg(over), "record.updated"); got != "ADT^A08^ADT_A01" {
		t.Fatalf("override: %q", got)
	}
	if got := msh9(cfg(over), "identity.created"); got != "ADT^A28^ADT_A05" {
		t.Fatalf("untouched event: %q", got)
	}
	// a trigger the template cannot build is refused, naming what it can
	step := NewHL7BuildExecutor()
	_, err := step.Execute(context.Background(), &models.TransformationStep{StepType: "hl7.build", Enabled: true,
		Config: cfg(map[string]interface{}{"record.updated": "A24"})}, map[string]interface{}{"type": "record.updated"})
	if err == nil || !strings.Contains(err.Error(), "A24") || !strings.Contains(err.Error(), "A04, A08, A28, A31") {
		t.Fatalf("unbuildable override: %v", err)
	}
}
