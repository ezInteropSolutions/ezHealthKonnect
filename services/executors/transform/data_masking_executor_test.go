package transform

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"strings"
	"testing"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func makeMaskStep(cfg map[string]interface{}) *models.TransformationStep {
	return &models.TransformationStep{
		StepName: "Test Masking",
		StepType: "data_masking",
		Enabled:  true,
		Config:   cfg,
	}
}

func runMask(t *testing.T, cfg map[string]interface{}, input map[string]interface{}) map[string]interface{} {
	t.Helper()
	exec := NewDataMaskingExecutor()
	out, err := exec.Execute(context.Background(), makeMaskStep(cfg), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	return out
}

func maskStepOutput(t *testing.T, out map[string]interface{}) map[string]interface{} {
	t.Helper()
	so, ok := out["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("_stepOutput not found; keys=%v", dmKeys(out))
	}
	return so
}

func dmKeys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// ─── TC-1: strategy=substitute, substituteType=custom ─────────────────────────

func TestDataMasking_Substitute_Custom(t *testing.T) {
	input := map[string]interface{}{
		"patientName": "John Smith",
	}
	out := runMask(t, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":          "patientName",
				"strategy":       "substitute",
				"substituteType": "custom",
				"substituteValue": "Test Patient",
			},
		},
	}, input)

	if out["patientName"] != "Test Patient" {
		t.Errorf("expected 'Test Patient', got %v", out["patientName"])
	}
	so := maskStepOutput(t, out)
	if count, _ := so["masked_count"].(int); count != 1 {
		t.Errorf("expected masked_count=1, got %v", so["masked_count"])
	}
}

// TC-2: substituteType=custom with no substituteValue → falls back to "[TEST DATA]"

func TestDataMasking_Substitute_CustomNoValue(t *testing.T) {
	input := map[string]interface{}{"name": "Real Name"}
	out := runMask(t, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":          "name",
				"strategy":       "substitute",
				"substituteType": "custom",
				// no substituteValue
			},
		},
	}, input)
	if out["name"] != "[TEST DATA]" {
		t.Errorf("expected '[TEST DATA]', got %v", out["name"])
	}
}

// TC-3: substituteType=name — produces "First Last" format, deterministic for same input

func TestDataMasking_Substitute_Name_Format(t *testing.T) {
	input := map[string]interface{}{"patientName": "Jane Doe"}
	out := runMask(t, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":          "patientName",
				"strategy":       "substitute",
				"substituteType": "name",
			},
		},
	}, input)
	got, _ := out["patientName"].(string)
	if got == "" || got == "Jane Doe" {
		t.Errorf("expected a different name, got %q", got)
	}
	// Should contain a space (First Last)
	if !strings.Contains(got, " ") {
		t.Errorf("expected 'First Last' format, got %q", got)
	}
}

// TC-4: substituteType=name is deterministic — same input → same substitute

func TestDataMasking_Substitute_Name_Deterministic(t *testing.T) {
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":          "patientName",
				"strategy":       "substitute",
				"substituteType": "name",
			},
		},
	}
	exec := NewDataMaskingExecutor()
	run := func() string {
		out, _ := exec.Execute(context.Background(), makeMaskStep(cfg),
			map[string]interface{}{"patientName": "Alice Walker"})
		return out["patientName"].(string)
	}
	first := run()
	second := run()
	if first != second {
		t.Errorf("substitute name is not deterministic: %q != %q", first, second)
	}
}

// TC-5: substituteType=ssn — 9XX-XX-XXXX format (ITIN range, never a real SSN)

func TestDataMasking_Substitute_SSN_Format(t *testing.T) {
	input := map[string]interface{}{"ssn": "123-45-6789"}
	out := runMask(t, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":          "ssn",
				"strategy":       "substitute",
				"substituteType": "ssn",
			},
		},
	}, input)
	got, _ := out["ssn"].(string)
	if !strings.HasPrefix(got, "9") {
		t.Errorf("expected SSN to start with 9 (ITIN range), got %q", got)
	}
	// Format: 9XX-XX-XXXX
	parts := strings.Split(got, "-")
	if len(parts) != 3 {
		t.Errorf("expected 3 hyphen-separated parts, got %d in %q", len(parts), got)
	}
}

// TC-6: substituteType=phone — 555 exchange

func TestDataMasking_Substitute_Phone_Format(t *testing.T) {
	input := map[string]interface{}{"phone": "212-555-1212"}
	out := runMask(t, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":          "phone",
				"strategy":       "substitute",
				"substituteType": "phone",
			},
		},
	}, input)
	got, _ := out["phone"].(string)
	if !strings.Contains(got, "555") {
		t.Errorf("expected 555 exchange in phone, got %q", got)
	}
}

// TC-7: substituteType=email — example-test.com domain

func TestDataMasking_Substitute_Email_Format(t *testing.T) {
	input := map[string]interface{}{"email": "john.smith@realco.com"}
	out := runMask(t, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":          "email",
				"strategy":       "substitute",
				"substituteType": "email",
			},
		},
	}, input)
	got, _ := out["email"].(string)
	if !strings.HasSuffix(got, "@example-test.com") {
		t.Errorf("expected @example-test.com domain, got %q", got)
	}
	if got == "john.smith@realco.com" {
		t.Errorf("email was not masked")
	}
}

// TC-8: substituteType=date — preserves year, changes month/day

func TestDataMasking_Substitute_Date_PreservesYear(t *testing.T) {
	input := map[string]interface{}{"dob": "1990-06-15"}
	out := runMask(t, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":          "dob",
				"strategy":       "substitute",
				"substituteType": "date",
			},
		},
	}, input)
	got, _ := out["dob"].(string)
	if !strings.HasPrefix(got, "1990-") {
		t.Errorf("expected year 1990 preserved, got %q", got)
	}
	if got == "1990-06-15" {
		t.Errorf("expected month/day to change, got same value %q", got)
	}
}

// TC-9: substituteType=address — number + fictional street

func TestDataMasking_Substitute_Address_Format(t *testing.T) {
	input := map[string]interface{}{"address": "123 Main St"}
	out := runMask(t, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":          "address",
				"strategy":       "substitute",
				"substituteType": "address",
			},
		},
	}, input)
	got, _ := out["address"].(string)
	if got == "123 Main St" {
		t.Errorf("address was not replaced")
	}
	// Should contain a space (house number + street)
	if !strings.Contains(got, " ") {
		t.Errorf("expected 'NNNNN Street' format, got %q", got)
	}
}

// TC-10: unknown substituteType → "[TEST DATA]"

func TestDataMasking_Substitute_Unknown_Type(t *testing.T) {
	input := map[string]interface{}{"field": "some value"}
	out := runMask(t, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":          "field",
				"strategy":       "substitute",
				"substituteType": "unknown_type_xyz",
			},
		},
	}, input)
	if out["field"] != "[TEST DATA]" {
		t.Errorf("expected '[TEST DATA]' for unknown type, got %v", out["field"])
	}
}

// TC-11: existing strategies still work alongside substitute

func TestDataMasking_MultipleStrategies(t *testing.T) {
	input := map[string]interface{}{
		"ssn":    "123-45-6789",
		"name":   "Jane Doe",
		"secret": "my-secret",
	}
	out := runMask(t, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"field": "ssn", "strategy": "hash"},
			map[string]interface{}{
				"field": "name", "strategy": "substitute",
				"substituteType": "name",
			},
			map[string]interface{}{"field": "secret", "strategy": "redact"},
		},
	}, input)

	// ssn should be 64-char hex (SHA-256)
	if h, _ := out["ssn"].(string); len(h) != 64 {
		t.Errorf("expected 64-char hash for ssn, got %q (len=%d)", h, len(h))
	}
	// name should be different from input
	if out["name"] == "Jane Doe" {
		t.Error("name should be substituted")
	}
	// secret should be redacted
	if out["secret"] != "[REDACTED]" {
		t.Errorf("expected [REDACTED], got %v", out["secret"])
	}

	so := maskStepOutput(t, out)
	if count, _ := so["masked_count"].(int); count != 3 {
		t.Errorf("expected masked_count=3, got %v", so["masked_count"])
	}
}

// TC-12: maskAllPHI + maskAllPHIFormat="hl7v2" — rules applied to HL7-shaped JSON

func TestDataMasking_MaskAllPHI_HL7Format(t *testing.T) {
	// Build a simplified HL7-shaped enhancedSegments map via JSON round-trip
	// (the executor accepts generic map[string]interface{} for HL7 data)
	rawInput := `{
		"enhancedSegments": {
			"PID": {
				"key": "PID",
				"fields": [
					{"key": "PID.5", "value": "Smith^John", "position": 5, "subfields": []},
					{"key": "PID.7", "value": "19900615", "position": 7, "subfields": []},
					{"key": "PID.19", "value": "123456789", "position": 19, "subfields": []}
				]
			}
		}
	}`
	var input map[string]interface{}
	json.Unmarshal([]byte(rawInput), &input)

	out := runMask(t, map[string]interface{}{
		"maskAllPHI":       true,
		"maskAllPHIFormat": "hl7v2",
	}, input)

	so := maskStepOutput(t, out)
	maskedCount, _ := so["masked_count"].(int)
	// At least PID.5, PID.7, PID.19 should be masked (PHI fields present in input)
	if maskedCount < 3 {
		t.Errorf("expected at least 3 PHI fields masked, got %d", maskedCount)
	}
}

// TC-13: no rules configured → no-op, input unchanged (except _stepOutput added)

func TestDataMasking_NoRules_Noop(t *testing.T) {
	input := map[string]interface{}{"patientName": "John Smith"}
	out := runMask(t, map[string]interface{}{}, input)
	if out["patientName"] != "John Smith" {
		t.Errorf("expected unchanged value, got %v", out["patientName"])
	}
}
