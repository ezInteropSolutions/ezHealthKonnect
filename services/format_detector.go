// services/format_detector.go
// OOB automatic message format detection

package services

import (
	"encoding/json"
	"log"
	"strings"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/parsers"
)

// FormatDetector automatically detects message format (OOB principle)
type FormatDetector struct {
	// No dependencies - pure detection logic
}

// NewFormatDetector creates a new format detector (OOB pattern)
func NewFormatDetector() *FormatDetector {
	return &FormatDetector{}
}

// ParseSampleMessage auto-detects a raw message's format and parses it into
// the same JSON envelope shape production pipelines consume — no DB access,
// no persistence, safe to call against a hand-written or synthetic sample.
// Extracted from controllers/transformation_test_controller.go's private
// parseTestMessage so dry-run tooling outside the controller layer (e.g. the
// Agent Mode script-verification tool) can reuse it instead of duplicating it.
func ParseSampleMessage(message string) map[string]interface{} {
	fd := NewFormatDetector()
	detection := fd.DetectFormat(message)
	detectedFormat := string(detection.DetectedFormat)

	registry := parsers.NewParserRegistry()
	parseResult := registry.Get(detectedFormat).Parse(message)

	result := parseResult.ParsedJSON
	if result == nil {
		result = map[string]interface{}{"raw": message}
	}

	// _format drives enrichMessageEnvelope (semantic index, sensitivity map).
	result["_format"] = detectedFormat

	// Format-agnostic enhanced fields for new consumers (schema-annotated view).
	if len(parseResult.EnhancedFields) > 0 {
		result["enhancedFields"] = parseResult.EnhancedFields
		result["fieldOrder"] = parseResult.FieldOrder
	}
	if parseResult.TypeName != "" {
		result["typeName"] = parseResult.TypeName
		result["typeDescription"] = parseResult.TypeDescription
	}

	log.Printf("✅ [ParseSampleMessage] Parsed %s message: type=%s fields=%d schemaLoaded=%v",
		detectedFormat, parseResult.Metadata.MessageType,
		len(parseResult.EnhancedFields), parseResult.TypeName != "")
	return result
}

// DetectFormat automatically detects message format (OOB principle)
func (fd *FormatDetector) DetectFormat(rawContent string) *models.FormatDetectionResult {
	// Check in order of specificity

	// 1. Check for HL7 v2 (most specific signature)
	if fd.isHL7v2(rawContent) {
		return &models.FormatDetectionResult{
			DetectedFormat: models.FormatHL7v2,
			Confidence:     0.95,
			Indicators:     []string{"MSH segment found", "Pipe delimiters"},
		}
	}

	// 2. Check for CCD/CCDA before generic HL7v3 — CDA is HL7v3 XML and would
	// otherwise match the HL7v3 check first, causing the CDA parser to be skipped.
	if fd.isCCDA(rawContent) {
		return &models.FormatDetectionResult{
			DetectedFormat: models.FormatCCDA,
			Confidence:     0.95,
			Indicators:     []string{"ClinicalDocument root element", "CDA namespace"},
		}
	}

	// 3. Check for HL7 v3 (non-CDA)
	if fd.isHL7v3(rawContent) {
		return &models.FormatDetectionResult{
			DetectedFormat: models.FormatHL7v3,
			Confidence:     0.90,
			Indicators:     []string{"HL7 v3 XML namespace"},
		}
	}

	// 4. Check for FHIR
	if fd.isFHIR(rawContent) {
		return &models.FormatDetectionResult{
			DetectedFormat: models.FormatFHIR,
			Confidence:     0.95,
			Indicators:     []string{"FHIR resourceType found"},
		}
	}

	// 5. Check for JSON
	if fd.isJSON(rawContent) {
		return &models.FormatDetectionResult{
			DetectedFormat: models.FormatJSON,
			Confidence:     0.80,
			Indicators:     []string{"Valid JSON structure"},
		}
	}

	// 6. Check for NCPDP SCRIPT (pharmacy e-prescribing XML) before the
	// generic XML catch-all — a <Message TransactionDomain="SCRIPT" ...>
	// root is valid XML too and would otherwise be swallowed by step 7,
	// same reasoning as CCDA being checked ahead of the generic HL7v3 check
	// above. No collision risk with CCDA/HL7v3 (NCPDP SCRIPT carries neither
	// "urn:hl7-org:v3" nor "<ClinicalDocument") or with FHIR/JSON (it's XML,
	// not JSON).
	if fd.isNCPDPScript(rawContent) {
		return &models.FormatDetectionResult{
			DetectedFormat: models.FormatNCPDPScript,
			Confidence:     0.95,
			Indicators:     []string{"Message root element", "TransactionDomain=SCRIPT"},
		}
	}

	// 7. Check for XML (generic)
	if fd.isXML(rawContent) {
		return &models.FormatDetectionResult{
			DetectedFormat: models.FormatXML,
			Confidence:     0.75,
			Indicators:     []string{"XML structure"},
		}
	}

	// 8. Check for EDI
	if fd.isEDI(rawContent) {
		return &models.FormatDetectionResult{
			DetectedFormat: models.FormatEDI,
			Confidence:     0.85,
			Indicators:     []string{"EDI segments", "ISA header"},
		}
	}

	// 9. Check for NCPDP Telecommunication D.0 (pharmacy claims) before the
	// generic CSV catch-all — D.0's own control-character-delimited content
	// would never coincidentally match any earlier check (no leading
	// <?xml/</MSH|/ISA, not valid JSON), but is checked here defensively so
	// it never falls through to a CSV false-positive either.
	if fd.isNCPDPTelecom(rawContent) {
		return &models.FormatDetectionResult{
			DetectedFormat: models.FormatNCPDPTelecom,
			Confidence:     0.9,
			Indicators:     []string{"RS (0x1E) segment separator found", "Version/Release field = D0"},
		}
	}

	// 10. Check for CSV
	if fd.isCSV(rawContent) {
		return &models.FormatDetectionResult{
			DetectedFormat: models.FormatCSV,
			Confidence:     0.70,
			Indicators:     []string{"CSV structure"},
		}
	}

	// Default: Unknown
	return &models.FormatDetectionResult{
		DetectedFormat: models.FormatUnknown,
		Confidence:     0.0,
		Indicators:     []string{"No recognizable format detected"},
	}
}

// Detection helper methods

func (fd *FormatDetector) isHL7v2(content string) bool {
	// HL7 v2 always starts with MSH
	return strings.HasPrefix(content, "MSH|") ||
		strings.Contains(content, "\rMSH|") ||
		strings.Contains(content, "\nMSH|")
}

func (fd *FormatDetector) isHL7v3(content string) bool {
	return strings.Contains(content, "urn:hl7-org:v3") ||
		strings.Contains(content, "<ClinicalDocument") ||
		strings.Contains(content, "xmlns=\"urn:hl7-org:v3\"")
}

func (fd *FormatDetector) isFHIR(content string) bool {
	// Any JSON with a "resourceType" key is FHIR — no resource-type whitelist needed.
	// The whitelist approach missed legitimate resources (Practitioner, MessageHeader, etc.)
	return strings.Contains(content, "\"resourceType\"") && fd.isJSON(content)
}

func (fd *FormatDetector) isCCDA(content string) bool {
	return strings.Contains(content, "urn:hl7-org:v3") &&
		strings.Contains(content, "<ClinicalDocument")
}

func (fd *FormatDetector) isNCPDPScript(content string) bool {
	return strings.Contains(content, "<Message") &&
		strings.Contains(content, "TransactionDomain=\"SCRIPT\"")
}

func (fd *FormatDetector) isJSON(content string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(content), &js) == nil
}

func (fd *FormatDetector) isXML(content string) bool {
	trimmed := strings.TrimSpace(content)
	return strings.HasPrefix(trimmed, "<?xml") || strings.HasPrefix(trimmed, "<")
}

func (fd *FormatDetector) isEDI(content string) bool {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "ISA") && strings.Contains(content, "GS") && strings.Contains(content, "ST") {
		return true
	}
	// Bare transaction set (no ISA/GS envelope) — the shape
	// processing/batch_splitter.go's splitEDITransactions produces for each
	// part of a multi-transaction-set interchange. edi.parse doesn't depend
	// on this (it reads its configured source field directly and handles
	// both shapes itself); this only matters for content-sniffing call
	// sites outside the pipeline.
	return strings.HasPrefix(trimmed, "ST") && strings.Contains(content, "SE")
}

// isNCPDPTelecom detects a raw NCPDP Telecommunication D.0 transmission: the
// fixed-width header's own Version/Release field (positions 7-8, 0-indexed
// 6-7) is a recognized D.0 version code, AND the body contains at least one
// 0x1E (RS) segment separator — both conditions together rule out any
// coincidental match against plain text that happens to start with "D0" at
// that position.
func (fd *FormatDetector) isNCPDPTelecom(content string) bool {
	if len(content) < 8 {
		return false
	}
	if content[6:8] != "D0" {
		return false
	}
	return strings.ContainsRune(content, '\x1E')
}

func (fd *FormatDetector) isCSV(content string) bool {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return false
	}
	// Check if first two lines have consistent comma counts
	commaCount1 := strings.Count(lines[0], ",")
	commaCount2 := strings.Count(lines[1], ",")
	return commaCount1 > 0 && commaCount1 == commaCount2
}
