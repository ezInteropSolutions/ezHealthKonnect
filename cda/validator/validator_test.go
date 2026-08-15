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

// TestNISTFullCCD_ShallScoreReflectsRealSections verifies the NIST reference
// CCD (full_ccd_nist.xml) scores 6/7 on the CORRECT CCD SHALL list per the
// C-CDA 2.1 IG, Table 30 (2018 errata, CONF:1198-30659..30690): Allergies,
// Medications, Problem, Results, Plan of Treatment, Social History, Vital
// Signs. (Immunizations — which this fixture DOES have — is actually MAY
// per that table, not SHALL; this was previously miscounted.) The fixture
// itself doesn't include a Plan of Treatment section, so it's short by
// exactly one real SHALL section under the corrected list — this test
// documents that gap rather than asserting a false 1.0.
func TestNISTFullCCD_ShallScoreReflectsRealSections(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	// planOfTreatment moved SHALL→SHOULD (corrected against Vol2 Table 30 +
	// Companion Guide Table 18 — it was never actually required). CCD's real
	// 6 SHALL sections (allergiesAndIntolerances, medications, problems,
	// results, socialHistory, vitalSigns) are all present in this fixture,
	// so ShallScore should now be a clean 1.0 with nothing missing.
	if report.ShallScore != 1.0 {
		t.Errorf("expected ShallScore=1.0 (all 6 real SHALL sections present), got %.4f", report.ShallScore)
	}

	var missing []string
	for _, sr := range report.SectionReports {
		if sr.Conformance == "SHALL" && sr.Status == "missing" {
			missing = append(missing, sr.SectionKey)
		}
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing SHALL sections, got %v", missing)
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

// TestCCDShallSectionCount verifies the validator generates exactly 6 SHALL entries for a CCD.
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
	// CCD SHALL (Vol2 Table 30 + Companion Guide Table 18, corrected —
	// planOfTreatment moved SHALL→SHOULD, it was never actually required):
	// allergiesAndIntolerances, medications, problems, results,
	// socialHistory, vitalSigns = 6
	if shallRules != 6 {
		t.Errorf("expected 6 CCD SHALL section rules, got %d", shallRules)
	}
}

// TestStrippedCCD_MissingAllergy_OneShallViolation removes the
// allergiesAndIntolerances section from the NIST reference CCD and verifies
// exactly one SHALL violation is reported. planOfTreatment moved SHALL→SHOULD
// (corrected — it was never actually required), so it no longer contributes
// a permanent baseline violation the way it used to.
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
	if len(missing) != 1 || missing[0] != "allergiesAndIntolerances" {
		t.Errorf("expected exactly 1 missing SHALL section (allergiesAndIntolerances), got %d: %v", len(missing), missing)
	}
}

// TestStrippedCCD_ShallScoreIs5of6 verifies the score numerics after
// stripping one section from a base that was a clean 6/6 (see
// TestNISTFullCCD_ShallScoreReflectsRealSections).
func TestStrippedCCD_ShallScoreIs5of6(t *testing.T) {
	raw := loadXML(t, "full_ccd_nist.xml")
	raw = stripComponentByTemplateID(raw, "2.16.840.1.113883.10.20.22.2.6.1")

	doc := parseCDA(t, raw)
	v := newValidator(t)

	report := v.Validate(doc)

	const expected = 5.0 / 6.0
	const eps = 0.001
	if report.ShallScore < expected-eps || report.ShallScore > expected+eps {
		t.Errorf("expected ShallScore≈%.4f (5/6) after stripping allergy, got %.4f", expected, report.ShallScore)
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
	raw := `<ClinicalDocument><templateId root="2.16.840.1.113883.10.20.22.1.8"/></ClinicalDocument>`
	if got := v.DetectDocumentType(raw); got != "Discharge Summary" {
		t.Errorf("expected Discharge Summary, got %q", got)
	}
}

func TestDetectDocumentType_CarePlan(t *testing.T) {
	v := newValidator(t)
	raw := `<ClinicalDocument><templateId root="2.16.840.1.113883.10.20.22.1.15"/></ClinicalDocument>`
	if got := v.DetectDocumentType(raw); got != "Care Plan" {
		t.Errorf("expected Care Plan, got %q", got)
	}
}

func TestDetectDocumentType_TransferSummary(t *testing.T) {
	v := newValidator(t)
	raw := `<ClinicalDocument><templateId root="2.16.840.1.113883.10.20.22.1.13"/></ClinicalDocument>`
	if got := v.DetectDocumentType(raw); got != "Transfer Summary" {
		t.Errorf("expected Transfer Summary, got %q", got)
	}
}

func TestDetectDocumentType_DiagnosticImagingReport(t *testing.T) {
	v := newValidator(t)
	raw := `<ClinicalDocument><templateId root="2.16.840.1.113883.10.20.22.1.5"/></ClinicalDocument>`
	if got := v.DetectDocumentType(raw); got != "Diagnostic Imaging Report" {
		t.Errorf("expected Diagnostic Imaging Report, got %q", got)
	}
}

func TestDetectDocumentType_OperativeNote(t *testing.T) {
	v := newValidator(t)
	raw := `<ClinicalDocument><templateId root="2.16.840.1.113883.10.20.22.1.7"/></ClinicalDocument>`
	if got := v.DetectDocumentType(raw); got != "Operative Note" {
		t.Errorf("expected Operative Note, got %q", got)
	}
}

func TestDetectDocumentType_ProcedureNote(t *testing.T) {
	v := newValidator(t)
	raw := `<ClinicalDocument><templateId root="2.16.840.1.113883.10.20.22.1.6"/></ClinicalDocument>`
	if got := v.DetectDocumentType(raw); got != "Procedure Note" {
		t.Errorf("expected Procedure Note, got %q", got)
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
