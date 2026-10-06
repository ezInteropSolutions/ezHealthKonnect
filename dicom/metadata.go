// dicom/metadata.go
// Metadata extraction shared by two independent callers: the live
// C-STORE receive path (dicom/receiver.go) and the later dicom.parse
// pipeline step (services/parsers/dicom), which re-decodes a stored Part 10
// file. One function, not duplicated logic in each caller.
package dicom

import "github.com/amrshadid/go-dicom/dataset"

// InstanceMetadata holds the clinically-relevant identifying fields of a
// received DICOM instance. Deliberately excludes PixelData (7FE0,0010) and
// any other bulk binary element — this is middleware, not a PACS; pixel
// data travels as opaque bytes inside the Part 10 file itself, never
// inlined into a canonical JSON structure.
type InstanceMetadata struct {
	PatientID             string `json:"patientId"`
	PatientName           string `json:"patientName"`
	StudyInstanceUID      string `json:"studyInstanceUID"`
	SeriesInstanceUID     string `json:"seriesInstanceUID"`
	SOPInstanceUID        string `json:"sopInstanceUID"`
	SOPClassUID           string `json:"sopClassUID"`
	Modality              string `json:"modality"`
	AccessionNumber       string `json:"accessionNumber"`
	Manufacturer          string `json:"manufacturer"`
	ManufacturerModelName string `json:"manufacturerModelName"`
	InstitutionName       string `json:"institutionName"`
	StationName           string `json:"stationName"`
}

// ExtractMetadata reads the identifying fields above from a dataset via
// Dataset.GetStringByKeyword — clean keyword-based access, no manual tag
// arithmetic. A missing element simply yields an empty string (the
// library's own documented behavior for GetStringByKeyword), never an
// error — this is best-effort metadata for routing/logging/HIS
// notification, not a validated clinical record.
func ExtractMetadata(ds *dataset.Dataset) *InstanceMetadata {
	return &InstanceMetadata{
		PatientID:             ds.GetStringByKeyword("PatientID"),
		PatientName:           ds.GetStringByKeyword("PatientName"),
		StudyInstanceUID:      ds.GetStringByKeyword("StudyInstanceUID"),
		SeriesInstanceUID:     ds.GetStringByKeyword("SeriesInstanceUID"),
		SOPInstanceUID:        ds.GetStringByKeyword("SOPInstanceUID"),
		SOPClassUID:           ds.GetStringByKeyword("SOPClassUID"),
		Modality:              ds.GetStringByKeyword("Modality"),
		AccessionNumber:       ds.GetStringByKeyword("AccessionNumber"),
		Manufacturer:          ds.GetStringByKeyword("Manufacturer"),
		ManufacturerModelName: ds.GetStringByKeyword("ManufacturerModelName"),
		InstitutionName:       ds.GetStringByKeyword("InstitutionName"),
		StationName:           ds.GetStringByKeyword("StationName"),
	}
}
