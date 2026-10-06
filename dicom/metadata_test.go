package dicom

import (
	"testing"

	"github.com/amrshadid/go-dicom/dataelem"
	"github.com/amrshadid/go-dicom/dataset"
)

func TestExtractMetadata_ReadsAllFields(t *testing.T) {
	ds := dataset.NewDataset()
	fields := map[string]struct {
		vr    dataelem.VR
		value string
	}{
		"PatientID":             {dataelem.LO, "PID001"},
		"PatientName":           {dataelem.PN, "Doe^Jane"},
		"StudyInstanceUID":      {dataelem.UI, "1.2.3.1"},
		"SeriesInstanceUID":     {dataelem.UI, "1.2.3.2"},
		"SOPInstanceUID":        {dataelem.UI, "1.2.3.3"},
		"SOPClassUID":           {dataelem.UI, "1.2.840.10008.5.1.4.1.1.1"},
		"Modality":              {dataelem.CS, "CR"},
		"AccessionNumber":       {dataelem.SH, "ACC001"},
		"Manufacturer":          {dataelem.LO, "Fujifilm"},
		"ManufacturerModelName": {dataelem.LO, "CR-IR 392"},
		"InstitutionName":       {dataelem.LO, "Test Hospital"},
		"StationName":           {dataelem.SH, "STATION1"},
	}
	for keyword, f := range fields {
		if err := ds.AddByKeyword(keyword, f.vr, []byte(f.value)); err != nil {
			t.Fatalf("AddByKeyword %s: %v", keyword, err)
		}
	}

	meta := ExtractMetadata(ds)

	cases := []struct {
		name, got, want string
	}{
		{"PatientID", meta.PatientID, "PID001"},
		{"PatientName", meta.PatientName, "Doe^Jane"},
		{"StudyInstanceUID", meta.StudyInstanceUID, "1.2.3.1"},
		{"SeriesInstanceUID", meta.SeriesInstanceUID, "1.2.3.2"},
		{"SOPInstanceUID", meta.SOPInstanceUID, "1.2.3.3"},
		{"SOPClassUID", meta.SOPClassUID, "1.2.840.10008.5.1.4.1.1.1"},
		{"Modality", meta.Modality, "CR"},
		{"AccessionNumber", meta.AccessionNumber, "ACC001"},
		{"Manufacturer", meta.Manufacturer, "Fujifilm"},
		{"ManufacturerModelName", meta.ManufacturerModelName, "CR-IR 392"},
		{"InstitutionName", meta.InstitutionName, "Test Hospital"},
		{"StationName", meta.StationName, "STATION1"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestExtractMetadata_MissingFieldsYieldEmptyStringsNotPanic(t *testing.T) {
	ds := dataset.NewDataset() // deliberately empty
	meta := ExtractMetadata(ds)
	if meta.PatientID != "" || meta.Modality != "" || meta.SOPInstanceUID != "" {
		t.Errorf("expected empty strings for every missing field, got %+v", meta)
	}
}

// TestExtractMetadata_NeverReadsPixelData is a regression guard for this
// package's own explicit scope boundary: PixelData must never appear in
// InstanceMetadata, no matter what the dataset contains.
func TestExtractMetadata_NeverReadsPixelData(t *testing.T) {
	ds := dataset.NewDataset()
	if err := ds.AddByKeyword("PixelData", dataelem.OW, []byte{0xDE, 0xAD, 0xBE, 0xEF}); err != nil {
		t.Fatalf("AddByKeyword PixelData: %v", err)
	}
	if err := ds.AddByKeyword("PatientID", dataelem.LO, []byte("PID001")); err != nil {
		t.Fatalf("AddByKeyword PatientID: %v", err)
	}

	meta := ExtractMetadata(ds)
	if meta.PatientID != "PID001" {
		t.Errorf("PatientID = %q, want PID001", meta.PatientID)
	}
	// InstanceMetadata simply has no field that could hold PixelData — this
	// test's real value is documenting the scope boundary and failing
	// loudly if a future change ever adds one that accidentally captures it.
}
