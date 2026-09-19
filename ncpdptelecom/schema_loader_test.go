package ncpdptelecom_test

import (
	"testing"

	"ezhealthkonnect/ncpdptelecom"
)

const testSchemaDir = "schemas/telecom_d0"

func loadRealSpec(t *testing.T) *ncpdptelecom.TelecomSpecDef {
	t.Helper()
	loader, err := ncpdptelecom.NewTelecomSchemaLoader(testSchemaDir)
	if err != nil {
		t.Fatalf("failed to load D.0 schema: %v", err)
	}
	return loader.Spec()
}

func TestSchemaLoader_LoadsRealSchema(t *testing.T) {
	spec := loadRealSpec(t)

	if len(spec.HeaderFields) == 0 {
		t.Fatal("expected header fields to be loaded")
	}
	totalWidth := 0
	for _, f := range spec.HeaderFields {
		totalWidth += f.Width
	}
	if totalWidth != 56 {
		t.Errorf("expected header total width 56 bytes, got %d", totalWidth)
	}

	for _, key := range []string{"Patient", "PharmacyProvider", "Prescriber", "Insurance", "Claim", "Pricing", "ResponseMessage", "ResponseStatus", "ResponseClaim", "ResponsePricing"} {
		if spec.Segments[key] == nil {
			t.Errorf("expected segment %q to be loaded", key)
		}
	}

	req := spec.Transactions[ncpdptelecom.TransactionKey("B1", "request")]
	if req == nil {
		t.Fatal("expected B1 request transaction to be loaded")
	}
	if req.Direction != "request" {
		t.Errorf("B1 request Direction = %q, want \"request\"", req.Direction)
	}

	resp := spec.Transactions[ncpdptelecom.TransactionKey("B1", "response")]
	if resp == nil {
		t.Fatal("expected B1 response transaction to be loaded")
	}
	if resp.Direction != "response" {
		t.Errorf("B1 response Direction = %q, want \"response\"", resp.Direction)
	}
}

func TestSchemaLoader_SegmentByIdentifier(t *testing.T) {
	spec := loadRealSpec(t)

	patient := spec.SegmentByIdentifier("01")
	if patient == nil || patient.Key != "Patient" {
		t.Errorf("SegmentByIdentifier(\"01\") = %v, want Patient", patient)
	}

	responseStatus := spec.SegmentByIdentifier("21")
	if responseStatus == nil || responseStatus.Key != "ResponseStatus" {
		t.Errorf("SegmentByIdentifier(\"21\") = %v, want ResponseStatus", responseStatus)
	}

	if spec.SegmentByIdentifier("99") != nil {
		t.Error("SegmentByIdentifier(\"99\") should be nil for an unregistered identifier")
	}
}

func TestSchemaLoader_FailsFastOnMissingManifest(t *testing.T) {
	_, err := ncpdptelecom.NewTelecomSchemaLoader("schemas/does_not_exist")
	if err == nil {
		t.Fatal("expected an error for a missing schema directory, got nil")
	}
}
