// services/parsers/dicom/dicom_parser_service.go
// DICOMParserService adapts the dicom/ package's own metadata extraction to
// this codebase's MessageParser interface (services/parser_factory.go), the
// same role services/parsers/edix12/edi_x12_parser_service.go plays for EDI.
// Needs no schema directory — go-dicom's own data dictionary is compiled
// into the library, not loaded from external files.
package dicomparser

import (
	"bytes"
	"fmt"
	"time"

	dicompkg "ezhealthkonnect/dicom"
	"ezhealthkonnect/models"

	"github.com/amrshadid/go-dicom/filebase"
	"github.com/amrshadid/go-dicom/filereader"
)

// DICOMParserService re-decodes raw DICOM Part 10 bytes into a canonical
// ParserResult. Used for messages that need re-parsing mid-pipeline (the
// same role edi.parse/astm.parse play for their own formats) — the live
// receive path (services/connectors/dicom_storage_inbound.go) already has
// the *dataset.Dataset in hand and does not go through this adapter.
type DICOMParserService struct{}

// NewDICOMParserService constructs the parser service. No schema directory
// to self-initialize from, unlike CDA/EDI/NCPDP/ASTM.
func NewDICOMParserService() *DICOMParserService {
	return &DICOMParserService{}
}

// GetSupportedFormat satisfies the services.MessageParser interface used by ParserFactory.
func (s *DICOMParserService) GetSupportedFormat() models.MessageFormat {
	return models.FormatDICOM
}

// Parse re-decodes raw DICOM Part 10 bytes (smuggled through the Go string
// convention every format in this codebase uses for InboundMessage.Content
// — Go strings are raw byte sequences with no UTF-8 enforcement, so this is
// lossless) and extracts metadata via dicom.ExtractMetadata — the SAME
// function the live receive path uses, so metadata extraction logic exists
// exactly once. PixelData is never read or inlined.
func (s *DICOMParserService) Parse(raw string) *models.ParserResult {
	start := time.Now()

	result := &models.ParserResult{
		Format:         models.FormatDICOM,
		EnhancedFields: make(map[string]*models.EnhancedField),
		FieldOrder:     []string{},
	}

	// filereader.ReadDICOMFile is deliberately permissive: content with no
	// preamble is parsed as a raw, implicit-VR dataset rather than
	// rejected outright — confirmed directly (garbage, non-DICOM content
	// was accepted with a logged warning rather than a returned error).
	// The DICM magic check must happen here, not be left to the library to
	// catch, or arbitrary non-DICOM content silently "succeeds" as an
	// empty/garbage result.
	if len(raw) <= 132 || raw[128:132] != "DICM" {
		result.Success = false
		result.Error = "dicom: content does not have the DICOM preamble + \"DICM\" magic at byte offset 128"
		return result
	}

	df, err := filereader.ReadDICOMFile(filebase.NewFileReader(bytes.NewReader([]byte(raw))))
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("dicom: %v", err)
		return result
	}

	ds := df.GetDataset()
	meta := dicompkg.ExtractMetadata(ds)

	result.Success = true
	result.ParsingTime = time.Since(start)
	result.Metadata = models.ParserMetadata{
		MessageType:      meta.SOPClassUID,
		MessageControlID: meta.SOPInstanceUID,
		ParsedAt:         time.Now(),
	}

	// "raw"/"_format" mirror the same sentinel-key convention every other
	// parser service in this codebase uses — a later mid-pipeline
	// dicom.parse step (default sourceField "raw") can find the original
	// content, and "_format" identifies the document without a separate
	// lookup. PixelData is deliberately absent from ParsedJSON — a
	// multi-MB binary blob has no place inlined in a pipeline's canonical
	// JSON; the full image remains intact in the raw Part 10 bytes.
	result.ParsedJSON = map[string]interface{}{
		"_format":               "dicom",
		"raw":                   raw,
		"patientId":             meta.PatientID,
		"patientName":           meta.PatientName,
		"studyInstanceUID":      meta.StudyInstanceUID,
		"seriesInstanceUID":     meta.SeriesInstanceUID,
		"sopInstanceUID":        meta.SOPInstanceUID,
		"sopClassUID":           meta.SOPClassUID,
		"modality":              meta.Modality,
		"accessionNumber":       meta.AccessionNumber,
		"manufacturer":          meta.Manufacturer,
		"manufacturerModelName": meta.ManufacturerModelName,
		"institutionName":       meta.InstitutionName,
		"stationName":           meta.StationName,
	}

	for key, value := range result.ParsedJSON {
		if key == "_format" || key == "raw" {
			continue
		}
		strVal, _ := value.(string)
		result.EnhancedFields[key] = &models.EnhancedField{
			Path:     key,
			Value:    value,
			HasValue: strVal != "",
		}
		result.FieldOrder = append(result.FieldOrder, key)
	}

	return result
}

// ValidateStructure checks whether the raw content looks like a DICOM Part
// 10 file at all (a cheap shape check, not full conformance validation).
func (s *DICOMParserService) ValidateStructure(rawContent string) (*models.ValidationResult, error) {
	result := &models.ValidationResult{Warnings: []string{}, Errors: []string{}}

	if len(rawContent) <= 132 || rawContent[128:132] != "DICM" {
		result.Errors = append(result.Errors, "content does not have the DICOM preamble + \"DICM\" magic at byte offset 128")
		return result, nil
	}

	if _, err := filereader.ReadDICOMFile(filebase.NewFileReader(bytes.NewReader([]byte(rawContent)))); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to parse: %v", err))
		return result, nil
	}

	result.IsValid = true
	return result, nil
}
