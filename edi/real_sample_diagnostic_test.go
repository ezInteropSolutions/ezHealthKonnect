// edi/real_sample_diagnostic_test.go
// Parses real, unedited X12 835 files against the real schema — sourced from
// keironstoddart/edi-835-parser's own test fixtures (genuine payer output:
// Blue Cross NC, eMedNY/NY Medicaid, United Healthcare), not synthetic data
// this project wrote. This is the direct answer to "has this actually been
// tested against real-world data" — these three files exercise real-world
// variety a hand-written test never would: a bare-ST file with no envelope
// at all, a full-envelope file, and a file that uses '>' as its component
// separator instead of the default ':' (proving delimiter auto-detection,
// not just the common case).
package edi_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"ezhealthkonnect/edi"
)

func loadRealSample(t *testing.T, filename string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	path := filepath.Join(filepath.Dir(thisFile), "testdata", "real_samples", filename)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	return string(content)
}

func realSpec(t *testing.T) *edi.X12SpecDef {
	t.Helper()
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	return loader.Spec()
}

func TestRealSample_BlueCrossNC_BareSTNoEnvelope(t *testing.T) {
	result, err := edi.ParseTransactionSet(realSpec(t), loadRealSample(t, "blue_cross_nc_sample.txt"))
	if err != nil {
		t.Fatalf("parsing real Blue Cross NC sample: %v", err)
	}
	if result.EnvelopePresent {
		t.Error("this fixture has no ISA envelope — EnvelopePresent should be false")
	}

	loop2000 := result.Loops["2000"].([]map[string]interface{})
	if len(loop2000) != 1 {
		t.Fatalf("expected 1 instance of loop 2000, got %d", len(loop2000))
	}
	claims := loop2000[0]["loops"].(map[string]interface{})["2100"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}
	clp := claims[0]["CLP"].(map[string]interface{})
	if clp["patientControlNumber"] != "200200964A52" {
		t.Errorf("patientControlNumber = %v, want 200200964A52", clp["patientControlNumber"])
	}
	if clp["claimPaymentAmount"] != "1922.86" {
		t.Errorf("claimPaymentAmount = %v, want 1922.86", clp["claimPaymentAmount"])
	}

	svcLines := claims[0]["loops"].(map[string]interface{})["2110"].([]map[string]interface{})
	if len(svcLines) != 3 {
		t.Fatalf("expected 3 service lines, got %d", len(svcLines))
	}
	svc0 := svcLines[0]["SVC"].(map[string]interface{})
	procCode := svc0["procedureCode"].(map[string]interface{})
	if procCode["qualifier"] != "HC" || procCode["code"] != "59430" {
		t.Errorf("first service line procedureCode = %#v", procCode)
	}
}

func TestRealSample_EMedNY_FullEnvelope_MultiClaim(t *testing.T) {
	result, err := edi.ParseTransactionSet(realSpec(t), loadRealSample(t, "emedny_sample.txt"))
	if err != nil {
		t.Fatalf("parsing real eMedNY sample: %v", err)
	}
	if !result.EnvelopePresent {
		t.Error("this fixture has a full ISA envelope — EnvelopePresent should be true")
	}
	if result.Interchange["senderId"] != "EMEDNYBAT" {
		t.Errorf("senderId = %v, want EMEDNYBAT", result.Interchange["senderId"])
	}

	loop2000 := result.Loops["2000"].([]map[string]interface{})
	claims := loop2000[0]["loops"].(map[string]interface{})["2100"].([]map[string]interface{})
	if len(claims) != 3 {
		t.Fatalf("expected 3 claims, got %d", len(claims))
	}

	totalServiceLines := 0
	for _, claim := range claims {
		svcLines := claim["loops"].(map[string]interface{})["2110"].([]map[string]interface{})
		totalServiceLines += len(svcLines)
	}
	if totalServiceLines != 10 {
		t.Errorf("total service lines across all claims = %d, want 10", totalServiceLines)
	}

	// Second claim has claim-level CAS adjustments (CO 29) on two service lines.
	clp2 := claims[1]["CLP"].(map[string]interface{})
	if clp2["claimPaymentAmount"] != "0" {
		t.Errorf("second claim claimPaymentAmount = %v, want 0 (fully adjusted)", clp2["claimPaymentAmount"])
	}
}

func TestRealSample_UnitedHealthcare_NonStandardComponentSeparator(t *testing.T) {
	result, err := edi.ParseTransactionSet(realSpec(t), loadRealSample(t, "united_healthcare_legacy_sample.txt"))
	if err != nil {
		t.Fatalf("parsing real United Healthcare sample: %v", err)
	}
	if !result.EnvelopePresent {
		t.Error("this fixture has a full ISA envelope — EnvelopePresent should be true")
	}

	loop2000 := result.Loops["2000"].([]map[string]interface{})
	claims := loop2000[0]["loops"].(map[string]interface{})["2100"].([]map[string]interface{})
	if len(claims) != 2 {
		t.Fatalf("expected 2 claims, got %d", len(claims))
	}

	// This file's own ISA16 declares '>' as the component separator (not the
	// default ':') — SVC01 is written as "HC>B4152", proving delimiter
	// auto-detection actually reads the real file's own declaration rather
	// than assuming the common case.
	svcLines := claims[0]["loops"].(map[string]interface{})["2110"].([]map[string]interface{})
	if len(svcLines) != 2 {
		t.Fatalf("expected 2 service lines on first claim, got %d", len(svcLines))
	}
	procCode := svcLines[0]["SVC"].(map[string]interface{})["procedureCode"].(map[string]interface{})
	if procCode["qualifier"] != "HC" || procCode["code"] != "B4152" {
		t.Errorf("non-standard-separator composite split incorrectly: %#v", procCode)
	}
}
