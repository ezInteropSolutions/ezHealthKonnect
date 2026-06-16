package validator_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/cda/validator"
	cdaparser "ezhealthkonnect/services/parsers/cda"
)

// ─── Test Infrastructure ────────────────────────────────────────────────────

// projectRoot navigates from the test file (cda/validator/) three levels up
// to the module root.
func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// file = .../cda/validator/validator_test.go
	//  Dir → .../cda/validator
	//  Dir → .../cda
	//  Dir → project root
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	return root
}

func schemaDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(projectRoot(t), "cda", "schemas")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("schema directory not found at %s: %v", dir, err)
	}
	return dir
}

func testdataDir(t *testing.T) string {
	t.Helper()
	// Sprint B testdata lives in cda/document/testdata
	dir := filepath.Join(projectRoot(t), "cda", "document", "testdata")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("testdata directory not found at %s: %v", dir, err)
	}
	return dir
}

func loadXML(t *testing.T, filename string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir(t), filename))
	if err != nil {
		t.Fatalf("cannot read %s: %v", filename, err)
	}
	return string(data)
}

// newValidator constructs a CDAConformanceValidator backed by the production schema.
func newValidator(t *testing.T) *validator.CDAConformanceValidator {
	t.Helper()
	loader, err := cdaSchema.NewCDASchemaLoader(schemaDir(t))
	if err != nil {
		t.Fatalf("schema loader: %v", err)
	}
	return validator.NewCDAConformanceValidator(loader)
}

// parseCDA uses the full CDAParserService (Sprint B) to parse raw XML and return
// the typed *CDADocument. This is the correct path — it guarantees the same
// etree-rooted parsing the production pipeline uses.
func parseCDA(t *testing.T, rawXML string) *cdadocument.CDADocument {
	t.Helper()
	svc, err := cdaparser.NewFromSchemaDir(schemaDir(t))
	if err != nil {
		t.Fatalf("parser service: %v", err)
	}
	result := svc.Parse(rawXML)
	if !result.Success {
		t.Fatalf("CDA parse failed: %s", result.Error)
	}
	doc, ok := result.TypedDocument.(*cdadocument.CDADocument)
	if !ok || doc == nil {
		t.Fatal("TypedDocument is not *CDADocument — ensure Sprint B parser is active")
	}
	return doc
}

// stripComponentByTemplateID removes the <component> block that contains the given
// templateId OID from the raw CDA XML. Used to simulate a missing section for tests.
func stripComponentByTemplateID(rawXML, templateOID string) string {
	needle := `templateId root="` + templateOID + `"`
	idx := strings.Index(rawXML, needle)
	if idx < 0 {
		// OID not found — nothing to strip.
		return rawXML
	}

	// Walk back to the nearest opening <component> tag.
	const compOpen = "<component>"
	before := rawXML[:idx]
	compStart := strings.LastIndex(before, compOpen)
	if compStart < 0 {
		return rawXML
	}

	// Walk forward to find the matching closing </component>, tracking nesting depth.
	depth := 1
	pos := compStart + len(compOpen)
	for pos < len(rawXML) && depth > 0 {
		nextOpen := strings.Index(rawXML[pos:], "<component>")
		nextClose := strings.Index(rawXML[pos:], "</component>")
		if nextClose < 0 {
			break
		}
		if nextOpen >= 0 && nextOpen < nextClose {
			depth++
			pos += nextOpen + len(compOpen)
		} else {
			depth--
			pos += nextClose + len("</component>")
		}
	}

	return rawXML[:compStart] + rawXML[pos:]
}

// ─── Section-Level Tests ─────────────────────────────────────────────────────

// TestNISTFullCCD_ShallScoreIsOne verifies that the NIST reference CCD (full_ccd_nist.xml)
// produces ShallScore = 1.0. The XML contains all 7 CCD SHALL sections:
// allergiesAndIntolerances, medications, problems, results, vitalSigns, immunizations, socialHistory.
func TestNISTFullCCD_ShallScoreIsOne(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	if report.ShallScore != 1.0 {
		t.Errorf("expected ShallScore=1.0 for NIST full CCD, got %.4f", report.ShallScore)
		for _, sr := range report.SectionReports {
			if sr.Conformance == "SHALL" && sr.Status == "missing" {
				t.Logf("  missing SHALL section: %s", sr.SectionKey)
			}
		}
	}
}

// TestNISTFullCCD_DocumentTypeIsCCD verifies auto-detection of the document type.
func TestNISTFullCCD_DocumentTypeIsCCD(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	if report.DocumentType != "CCD" {
		t.Errorf("expected DocumentType=CCD, got %q", report.DocumentType)
	}
}

// TestCCDShallSectionCount verifies the validator generates exactly 7 SHALL entries for a CCD.
func TestCCDShallSectionCount(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	var shallRules int
	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" {
			shallRules++
		}
	}
	// CCD SHALL: allergiesAndIntolerances, medications, problems,
	//            results, vitalSigns, immunizations, socialHistory = 7
	if shallRules != 7 {
		t.Errorf("expected 7 CCD SHALL section rules, got %d", shallRules)
	}
}

// TestStrippedCCD_MissingAllergy_OneShallViolation removes the allergiesAndIntolerances
// section from the NIST reference CCD and verifies exactly one SHALL violation is reported.
func TestStrippedCCD_MissingAllergy_OneShallViolation(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	raw = stripComponentByTemplateID(raw, "2.16.840.1.113883.10.20.22.2.6.1")

	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	if report.ShallScore >= 1.0 {
		t.Errorf("expected ShallScore < 1.0 after stripping allergy section, got %.4f", report.ShallScore)
	}

	var missing []string
	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			missing = append(missing, sr.SectionKey)
		}
	}
	if len(missing) != 1 {
		t.Errorf("expected exactly 1 missing SHALL section, got %d: %v", len(missing), missing)
	}
	if len(missing) == 1 && missing[0] != "allergiesAndIntolerances" {
		t.Errorf("expected missing section=allergiesAndIntolerances, got %q", missing[0])
	}
}

// TestStrippedCCD_ShallScoreIs6of7 verifies the score numerics after stripping one section.
func TestStrippedCCD_ShallScoreIs6of7(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	raw = stripComponentByTemplateID(raw, "2.16.840.1.113883.10.20.22.2.6.1")

	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	const expected = 6.0 / 7.0
	const eps = 0.001
	if report.ShallScore < expected-eps || report.ShallScore > expected+eps {
		t.Errorf("expected ShallScore≈%.4f (6/7) after stripping allergy, got %.4f", expected, report.ShallScore)
	}
}

// ─── Score Formula Tests ─────────────────────────────────────────────────────

// TestOverallScoreIsWeightedAverage verifies OverallScore = SHALL*0.7 + SHOULD*0.3.
func TestOverallScoreIsWeightedAverage(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	expected := report.ShallScore*0.7 + report.ShouldScore*0.3
	if absf(report.OverallScore-expected) > 0.001 {
		t.Errorf("OverallScore=%.4f, expected %.4f (SHALL*0.7+SHOULD*0.3)", report.OverallScore, expected)
	}
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// ─── Document Type Detection Tests ───────────────────────────────────────────

func TestDetectDocumentType_CCD(t *testing.T) {
	v := newValidator(t)
	raw := loadXML(t, "full_ccd_nist.xml")
	if got := v.DetectDocumentType(raw); got != "CCD" {
		t.Errorf("expected CCD, got %q", got)
	}
}

func TestDetectDocumentType_FallbackToCCD(t *testing.T) {
	v := newValidator(t)
	raw := `<ClinicalDocument xmlns="urn:hl7-org:v3"><id root="1.2.3"/></ClinicalDocument>`
	if got := v.DetectDocumentType(raw); got != "CCD" {
		t.Errorf("expected CCD fallback, got %q", got)
	}
}

func TestDetectDocumentType_DischargeSummary(t *testing.T) {
	v := newValidator(t)
	raw := `<ClinicalDocument><templateId root="2.16.840.1.113883.10.20.22.1.7"/></ClinicalDocument>`
	if got := v.DetectDocumentType(raw); got != "Discharge Summary" {
		t.Errorf("expected Discharge Summary, got %q", got)
	}
}

// ─── Field Report Tests ───────────────────────────────────────────────────────

// TestPresentSections_HaveFieldReports verifies that sections present in the document
// produce at least one field report.
func TestPresentSections_HaveFieldReports(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Status == "present" && len(sr.FieldReports) == 0 {
			t.Errorf("present section %q has no field reports", sr.SectionKey)
		}
	}
}

// TestMissingSections_NoFieldReports verifies that missing sections produce zero field reports.
func TestMissingSections_NoFieldReports(t *testing.T) {
	raw := loadXML(t, "minimal_ccd.xml")
	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	for _, sr := range report.SectionReports {
		if sr.Status == "missing" && len(sr.FieldReports) != 0 {
			t.Errorf("missing section %q should have 0 field reports, got %d", sr.SectionKey, len(sr.FieldReports))
		}
	}
}

// ─── Report Completeness Tests ────────────────────────────────────────────────

// TestReportHasAllCCDRules verifies the report contains at least one entry per
// CCD SHALL/SHOULD/MAY section rule (minimum 7 SHALL + SHOULD entries).
func TestReportHasAllCCDRules(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	// CCD has 7 SHALL + 6 SHOULD + 5 MAY = 18 total, but we only require at least 7.
	if len(report.SectionReports) < 7 {
		t.Errorf("expected at least 7 section reports for CCD, got %d", len(report.SectionReports))
	}
}

// TestGeneratedAt_IsRecent verifies the report timestamp is populated.
func TestGeneratedAt_IsRecent(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	if report.GeneratedAt.IsZero() {
		t.Error("GeneratedAt is zero — should be set to current time")
	}
}

// TestDocumentId_IsPopulated verifies the document ID is extracted from the header.
func TestDocumentId_IsPopulated(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	if report.DocumentId == "" {
		t.Error("DocumentId is empty — should be extracted from ClinicalDocument/id")
	}
}
