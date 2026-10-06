package dicomparser

import (
	"testing"

	dicompkg "ezhealthkonnect/dicom"
	"ezhealthkonnect/models"

	"github.com/amrshadid/go-dicom/dataelem"
	"github.com/amrshadid/go-dicom/dataset"
)

// buildTestPart10 reuses dicom.SerializeToPart10 directly rather than
// hand-building fixture bytes a second time — the same real Part 10 file
// construction path dicom_storage_inbound.go's own live receive path uses.
func buildTestPart10(t *testing.T) string {
	t.Helper()
	ds := dataset.NewDataset()
	must := func(err error) {
		if err != nil {
			t.Fatalf("buildTestPart10: %v", err)
		}
	}
	must(ds.AddByKeyword("SOPClassUID", dataelem.UI, []byte("1.2.840.10008.5.1.4.1.1.1")))
	must(ds.AddByKeyword("SOPInstanceUID", dataelem.UI, []byte("1.2.3.4.5.77")))
	must(ds.AddByKeyword("PatientID", dataelem.LO, []byte("PID077")))
	must(ds.AddByKeyword("Modality", dataelem.CS, []byte("CR")))

	out, err := dicompkg.SerializeToPart10("1.2.840.10008.5.1.4.1.1.1", "1.2.3.4.5.77", ds)
	if err != nil {
		t.Fatalf("SerializeToPart10: %v", err)
	}
	return string(out)
}

func TestDICOMParserService_Parse_RealFixture(t *testing.T) {
	svc := NewDICOMParserService()
	raw := buildTestPart10(t)

	result := svc.Parse(raw)
	if !result.Success {
		t.Fatalf("Parse failed: %s", result.Error)
	}
	if result.Format != models.FormatDICOM {
		t.Errorf("Format = %q, want %q", result.Format, models.FormatDICOM)
	}
	if result.ParsedJSON["sopInstanceUID"] != "1.2.3.4.5.77" {
		t.Errorf("sopInstanceUID = %v, want 1.2.3.4.5.77", result.ParsedJSON["sopInstanceUID"])
	}
	if result.ParsedJSON["patientId"] != "PID077" {
		t.Errorf("patientId = %v, want PID077", result.ParsedJSON["patientId"])
	}
	if result.ParsedJSON["raw"] != raw {
		t.Error("ParsedJSON[\"raw\"] does not match the original content — a later dicom.parse step's default sourceField would silently fail to resolve on a genuinely live message")
	}
	if _, present := result.ParsedJSON["pixelData"]; present {
		t.Error("ParsedJSON must never carry pixelData")
	}
}

func TestDICOMParserService_Parse_GarbageContent_ErrorsNotPanic(t *testing.T) {
	svc := NewDICOMParserService()
	result := svc.Parse("this is not a DICOM file at all")
	if result.Success {
		t.Error("expected Parse to fail on non-DICOM content")
	}
}

func TestDICOMParserService_ValidateStructure_RealFixture(t *testing.T) {
	svc := NewDICOMParserService()
	raw := buildTestPart10(t)

	result, err := svc.ValidateStructure(raw)
	if err != nil {
		t.Fatalf("ValidateStructure: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected IsValid=true, got errors: %v", result.Errors)
	}
}

func TestDICOMParserService_ValidateStructure_MissingMagic(t *testing.T) {
	svc := NewDICOMParserService()
	result, err := svc.ValidateStructure("not a dicom file")
	if err != nil {
		t.Fatalf("ValidateStructure: %v", err)
	}
	if result.IsValid {
		t.Error("expected IsValid=false for content with no DICM magic")
	}
}

func TestDICOMParserService_GetSupportedFormat(t *testing.T) {
	svc := NewDICOMParserService()
	if got := svc.GetSupportedFormat(); got != models.FormatDICOM {
		t.Errorf("GetSupportedFormat() = %q, want %q", got, models.FormatDICOM)
	}
}
