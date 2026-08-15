package validator

import (
	"strings"
	"time"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
)

// CDAConformanceValidator validates a typed CDADocument against C-CDA 2.1 conformance
// rules loaded from ccda_2_1.json.
//
// It is read-only and goroutine-safe once constructed. Construct once at application
// startup and reuse across requests.
//
// Usage:
//
//	v := validator.NewCDAConformanceValidator(loader)
//	report := v.Validate(doc)
type CDAConformanceValidator struct {
	schemaLoader *cdaSchema.CDASchemaLoader
}

// NewCDAConformanceValidator constructs a validator backed by the given CDASchemaLoader.
// loader must not be nil.
func NewCDAConformanceValidator(loader *cdaSchema.CDASchemaLoader) *CDAConformanceValidator {
	return &CDAConformanceValidator{schemaLoader: loader}
}

// Validate runs conformance validation on the typed CDA document.
//
// Document type is auto-detected from doc.Raw using known templateId OIDs.
// Section conformance rules are loaded from documentTypeSections in ccda_2_1.json.
// Field-level checks use the typed CDASection.Entries from Sprint B's parser.
func (v *CDAConformanceValidator) Validate(doc *cdadocument.CDADocument) *ComplianceReport {
	docType := v.DetectDocumentType(doc.Raw)
	rules := buildSectionRules(v.schemaLoader, docType)

	report := &ComplianceReport{
		DocumentId:     resolveDocumentId(doc),
		DocumentType:   docType,
		SectionReports: make([]SectionReport, 0, len(rules)),
		GeneratedAt:    time.Now(),
	}

	var shallTotal, shallPresent, shouldTotal, shouldPresent int

	for _, rule := range rules {
		sr := v.validateSection(rule, doc)
		report.SectionReports = append(report.SectionReports, sr)

		switch rule.Conformance {
		case "SHALL":
			shallTotal++
			if sr.Status != "missing" {
				shallPresent++
			}
		case "SHOULD":
			shouldTotal++
			if sr.Status != "missing" {
				shouldPresent++
			}
		}
	}

	if dtInfo := v.schemaLoader.GetDocumentTypeSections(docType); dtInfo != nil {
		for _, cc := range dtInfo.ChoiceConstraints {
			ccr := v.validateChoiceConstraint(cc, doc)
			report.ChoiceConstraintReports = append(report.ChoiceConstraintReports, ccr)
			shallTotal++
			if ccr.Satisfied {
				shallPresent++
			}
		}
	}

	if shallTotal > 0 {
		report.ShallScore = float64(shallPresent) / float64(shallTotal)
	} else {
		report.ShallScore = 1.0
	}
	if shouldTotal > 0 {
		report.ShouldScore = float64(shouldPresent) / float64(shouldTotal)
	} else {
		report.ShouldScore = 1.0
	}
	// Weighted overall: SHALL carries 70% weight, SHOULD 30%.
	report.OverallScore = report.ShallScore*0.7 + report.ShouldScore*0.3

	return report
}

// validateSection checks a single section rule against the parsed document and
// runs field-level checks when the section is present.
func (v *CDAConformanceValidator) validateSection(
	rule sectionRule, doc *cdadocument.CDADocument,
) SectionReport {
	sr := SectionReport{
		SectionKey:  rule.SectionKey,
		Conformance: rule.Conformance,
		Title:       rule.SchemaDef.DisplayName,
		TemplateId:  rule.SchemaDef.TemplateID,
	}

	section, present := doc.SectionsByKey[rule.SectionKey]
	if !present || section == nil {
		sr.Status = "missing"
		return sr
	}

	sr.EntryCount = len(section.Entries)

	// A section present but having only a nullFlavor annotation and no entries
	// is valid XML but carries no clinical data — report as present_empty.
	if sr.EntryCount == 0 && section.NullFlavor != "" {
		sr.Status = "present_empty"
	} else {
		sr.Status = "present"
	}

	// Field-level checks are only meaningful for present sections.
	checker := resolveFieldChecker(rule.SectionKey)
	sr.FieldReports = checker(section, rule.SchemaDef)

	return sr
}

// validateChoiceConstraint checks a "SHALL contain branch A, OR branch B"
// requirement (cdaSchema.SectionChoiceConstraint) against the parsed
// document. Satisfied if every section key in ANY ONE branch is present
// (status != "missing") — mirrors validateSection's own presence check via
// doc.SectionsByKey, just applied across a whole branch instead of one key.
func (v *CDAConformanceValidator) validateChoiceConstraint(
	cc cdaSchema.SectionChoiceConstraint, doc *cdadocument.CDADocument,
) ChoiceConstraintReport {
	ccr := ChoiceConstraintReport{Description: cc.Description}

	for _, branch := range cc.Branches {
		branchSatisfied := len(branch) > 0
		for _, key := range branch {
			section, present := doc.SectionsByKey[key]
			if !present || section == nil {
				branchSatisfied = false
				break
			}
		}
		if branchSatisfied {
			ccr.Satisfied = true
			ccr.SatisfiedBranch = branch
			break
		}
	}

	return ccr
}

// DetectDocumentType scans the raw CDA XML for known document template OIDs and
// returns the matching human-readable document type name.
// Falls back to "CCD" when no known OID is found.
func (v *CDAConformanceValidator) DetectDocumentType(rawXML string) string {
	// Ordered by specificity — most distinctive OID first.
	candidates := []struct {
		oid     string
		docType string
	}{
		{"2.16.840.1.113883.10.20.22.1.2", "CCD"},
		{"2.16.840.1.113883.10.20.22.1.8", "Discharge Summary"},
		{"2.16.840.1.113883.10.20.22.1.3", "History and Physical"},
		{"2.16.840.1.113883.10.20.22.1.4", "Consultation Note"},
		{"2.16.840.1.113883.10.20.22.1.6", "Procedure Note"},
		{"2.16.840.1.113883.10.20.22.1.7", "Operative Note"},
		{"2.16.840.1.113883.10.20.22.1.9", "Progress Note"},
		{"2.16.840.1.113883.10.20.22.1.14", "Referral Note"},
		{"2.16.840.1.113883.10.20.22.1.15", "Care Plan"},
		{"2.16.840.1.113883.10.20.22.1.13", "Transfer Summary"},
		{"2.16.840.1.113883.10.20.22.1.5", "Diagnostic Imaging Report"},
		{"2.16.840.1.113883.10.20.22.1.10", "Unstructured Document"},
	}
	for _, c := range candidates {
		if strings.Contains(rawXML, c.oid) {
			return c.docType
		}
	}
	return "CCD"
}

// resolveDocumentId extracts a human-readable document identifier from the parsed header.
// Prefers Extension (e.g. "NIST-001"), falls back to Root OID.
func resolveDocumentId(doc *cdadocument.CDADocument) string {
	if doc.Header.DocumentId.Extension != "" {
		return doc.Header.DocumentId.Extension
	}
	return doc.Header.DocumentId.Root
}
