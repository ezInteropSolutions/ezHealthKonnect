// services/format_detector_astm_test.go
// Focused coverage for isASTM/DetectFormat's ASTM branch — this detector had
// zero direct unit tests of any kind before this file (confirmed by a
// repo-wide search while writing the device-connectivity test plan). Scoped
// narrowly to the ASTM-related cases this feature added, not a backfill of
// every other format's own detection logic.
package services

import (
	"testing"

	"ezhealthkonnect/models"
)

func TestDetectFormat_RealASTMStream_DetectedAsASTM(t *testing.T) {
	fd := NewFormatDetector()
	content := "H|\\^&|MSGCTRL001||ANALYZER-1^1.0^SN12345|||||LIS||P|E1394-97|20231004120000\r" +
		"P|1||MRN12345||Doe^Jane||19800101|F\r" +
		"L|1|N\r"

	result := fd.DetectFormat(content)
	if result.DetectedFormat != models.FormatASTM {
		t.Errorf("DetectedFormat = %v, want %v", result.DetectedFormat, models.FormatASTM)
	}
	if result.Confidence <= 0 {
		t.Error("expected a positive confidence score for a real ASTM stream")
	}
}

func TestDetectFormat_ASTM_NoHRecord_NotDetectedAsASTM(t *testing.T) {
	fd := NewFormatDetector()
	// Starts with "H" but has no real H-record delimiter declaration — must
	// not be misdetected just because of the leading letter.
	content := "Hello, this is not ASTM content at all, just English text."

	result := fd.DetectFormat(content)
	if result.DetectedFormat == models.FormatASTM {
		t.Error("plain text starting with 'H' should not be detected as ASTM")
	}
}

func TestDetectFormat_CSV_NotMisdetectedAsASTM(t *testing.T) {
	fd := NewFormatDetector()
	content := "name,age,city\nJane,45,Springfield\nBob,32,Shelbyville\n"

	result := fd.DetectFormat(content)
	if result.DetectedFormat == models.FormatASTM {
		t.Error("a CSV file should never be detected as ASTM")
	}
	if result.DetectedFormat != models.FormatCSV {
		t.Errorf("expected CSV to still be detected as CSV, got %v", result.DetectedFormat)
	}
}

func TestDetectFormat_HL7_NotMisdetectedAsASTM(t *testing.T) {
	fd := NewFormatDetector()
	content := "MSH|^~\\&|APP|FAC|APP2|FAC2|20231004120000||ADT^A01|MSG001|P|2.5\r" +
		"PID|1||MRN123||Doe^Jane||19800101|F\r"

	result := fd.DetectFormat(content)
	if result.DetectedFormat != models.FormatHL7v2 {
		t.Errorf("DetectedFormat = %v, want %v (ASTM check must not pre-empt HL7)", result.DetectedFormat, models.FormatHL7v2)
	}
}

func TestIsASTM_EmptyContent_NotDetected(t *testing.T) {
	fd := NewFormatDetector()
	if fd.isASTM("") {
		t.Error("empty content should never be detected as ASTM")
	}
}
