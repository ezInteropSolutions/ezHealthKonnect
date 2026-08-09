package validator

import (
	"testing"

	"ezhealthkonnect/hl7"
)

// testHL7SchemaDir mirrors hl7/builder/field_catalog_test.go's own constant —
// same real compiled fixtures, same directory depth (hl7/validator/ sits at
// the same level as hl7/builder/).
const testHL7SchemaDir = "../../schemas/hl7"

// parseFixture initializes the real schema loader and parses raw through the
// same hl7.ParseWithRealSchema production callers use, returning both the
// parsed message and its resolved schema (via hl7.ResolveSchemaForMessage)
// so category functions can be exercised against real, schema-derived data
// rather than hand-built structs.
func parseFixture(t *testing.T, raw string) (*hl7.EnhancedParsedMessage, *hl7.RealHL7Schema) {
	t.Helper()
	hl7.InitRealSchemaLoader(testHL7SchemaDir)
	msg := hl7.ParseWithRealSchema(raw)
	if msg == nil || !msg.Success {
		t.Fatalf("ParseWithRealSchema failed: %+v", msg)
	}
	schema, err := hl7.ResolveSchemaForMessage(raw)
	if err != nil {
		t.Fatalf("ResolveSchemaForMessage failed: %v", err)
	}
	return msg, schema
}

// validADTA01 is deliberately minimal (only EVN/PID/PV1's leading fields
// populated) rather than densely filled — a first, densely-filled draft of
// this fixture (copied from hl7-reader.html's illustrative sample, which was
// only ever exercised for display/parsing, never conformance) turned out to
// have several PV1 fields carrying placeholder/duplicated values that are
// NOT real codes in their bound HL7 tables (e.g. PV1.15 Ambulatory Status,
// PV1.36 Discharge Disposition) — the new validator correctly flagged them,
// which is exactly its job, but made this fixture useless as a "genuinely
// valid message" golden path. Every value below is confirmed against the
// real compiled v2.5.1 schema/table data.
const validADTA01 = "MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240101120000||ADT^A01|MSG001|P|2.5.1\r" +
	"EVN|A01|20240101120000\r" +
	"PID|1||12345^^^MRN||DOE^JOHN^A||19800115|M\r" +
	"PV1|1|I"

func TestValidateMessage_LevelBasic_IsNoOp(t *testing.T) {
	msg, schema := parseFixture(t, validADTA01)
	result := ValidateMessage(schema, msg, ValidationOptions{Level: LevelBasic})
	if !result.Valid {
		t.Errorf("expected Valid=true at LevelBasic (no-op), got errors: %+v", result.Errors)
	}
	if len(result.All()) != 0 {
		t.Errorf("expected zero validation entries at LevelBasic, got %d: %+v", len(result.All()), result.All())
	}
}

func TestValidateMessage_GoldenPath_ValidADTA01_NoFalsePositives(t *testing.T) {
	msg, schema := parseFixture(t, validADTA01)
	result := ValidateMessage(schema, msg, ValidationOptions{Level: LevelStandard})
	if !result.Valid || len(result.Errors) != 0 {
		t.Errorf("expected zero errors for a fully valid ADT^A01, got: %+v", result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected zero warnings for a fully valid ADT^A01, got: %+v", result.Warnings)
	}
}

func TestValidateMessage_NilSchemaOrMessage_ReturnsValidNoop(t *testing.T) {
	msg, schema := parseFixture(t, validADTA01)
	if r := ValidateMessage(nil, msg, ValidationOptions{Level: LevelStandard}); !r.Valid || len(r.All()) != 0 {
		t.Errorf("expected no-op result for nil schema, got %+v", r)
	}
	if r := ValidateMessage(schema, nil, ValidationOptions{Level: LevelStandard}); !r.Valid || len(r.All()) != 0 {
		t.Errorf("expected no-op result for nil message, got %+v", r)
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]ValidationLevel{
		"":         LevelBasic,
		"basic":    LevelBasic,
		"Standard": LevelStandard,
		"STRICT":   LevelStrict,
		"garbage":  LevelBasic,
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestValidateMessage_StandardVsStrict_SeverityEscalates uses a table-binding
// violation (an invalid PID.8 code) to confirm Standard reports it as a
// WARNING and Strict escalates the same finding to ERROR.
func TestValidateMessage_StandardVsStrict_SeverityEscalates(t *testing.T) {
	raw := "MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240101120000||ADT^A01|MSG001|P|2.5.1\r" +
		"EVN|A01|20240101120000\r" +
		"PID|1||12345^^^MRN||DOE^JOHN^A||19800115|Z\r" +
		"PV1|1|I"
	msg, schema := parseFixture(t, raw)

	standard := ValidateMessage(schema, msg, ValidationOptions{Level: LevelStandard})
	if len(standard.Errors) != 0 {
		t.Errorf("standard: expected zero ERRORs from a table-binding issue, got: %+v", standard.Errors)
	}
	if len(standard.Warnings) == 0 {
		t.Fatal("standard: expected the invalid PID.8 code to produce a WARNING")
	}

	strict := ValidateMessage(schema, msg, ValidationOptions{Level: LevelStrict})
	if len(strict.Warnings) != 0 {
		t.Errorf("strict: expected the table-binding issue to escalate out of Warnings, got: %+v", strict.Warnings)
	}
	if len(strict.Errors) == 0 {
		t.Fatal("strict: expected the invalid PID.8 code to escalate to an ERROR")
	}
}
