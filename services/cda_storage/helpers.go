package cdastorage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/cda/validator"
)

func marshalJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// documentMeta is the intermediate struct used to build a cda_documents row.
// It is populated from a typed *cdadocument.CDADocument when available,
// with fallback to zero values when the typed doc is nil or type assertion fails.
type documentMeta struct {
	DocumentID          string
	DocumentType        string
	PatientID           string
	PatientName         string
	DocumentDate        *time.Time
	AuthorName          string
	CustodianName       string
}

// extractDocumentMeta derives patient and document identity fields from SaveInput.
// TypedDocument is type-asserted to *cdadocument.CDADocument; if nil or wrong type
// the returned struct contains only the InputID/Type from the SaveInput.
func extractDocumentMeta(input *SaveInput) documentMeta {
	meta := documentMeta{DocumentType: "C-CDA 2.1"}

	doc, ok := input.TypedDocument.(*cdadocument.CDADocument)
	if !ok || doc == nil {
		return meta
	}

	// Document identity
	h := doc.Header
	if h.DocumentId.Extension != "" {
		meta.DocumentID = h.DocumentId.Extension
	} else if h.DocumentId.Root != "" {
		meta.DocumentID = h.DocumentId.Root
	}
	if h.Title != "" {
		meta.DocumentType = h.Title
	}

	// Document date
	if h.EffectiveTime.Value != "" {
		if t, err := parseHL7DateTime(h.EffectiveTime.Value); err == nil {
			meta.DocumentDate = &t
		}
	}

	// Patient
	p := h.Patient
	if len(p.Ids) > 0 {
		meta.PatientID = p.Ids[0].Extension
		if meta.PatientID == "" {
			meta.PatientID = p.Ids[0].Root
		}
	}
	if len(p.Names) > 0 {
		meta.PatientName = buildDisplayName(p.Names[0])
	}

	// Author
	if len(h.Authors) > 0 {
		aa := h.Authors[0].AssignedAuthor
		if aa.AssignedPerson != nil && len(aa.AssignedPerson.Names) > 0 {
			meta.AuthorName = buildDisplayName(aa.AssignedPerson.Names[0])
		}
	}

	// Custodian
	org := h.Custodian.AssignedCustodian.RepresentedCustodianOrganization
	if len(org.Names) > 0 {
		meta.CustodianName = org.Names[0]
	}

	return meta
}

// extractComplianceScore safely reads OverallScore from a *validator.ComplianceReport.
// Returns (0, nil) when input is nil or the wrong type.
func extractComplianceScore(raw interface{}) (float64, []byte) {
	if raw == nil {
		return 0, nil
	}
	report, ok := raw.(*validator.ComplianceReport)
	if !ok || report == nil {
		return 0, nil
	}
	data, _ := marshalJSON(report)
	return report.OverallScore, data
}

// buildDisplayName produces "Given Family" from a CDAName.
func buildDisplayName(n cdadocument.CDAName) string {
	parts := make([]string, 0, len(n.Given)+1)
	for _, g := range n.Given {
		if g = strings.TrimSpace(g); g != "" {
			parts = append(parts, g)
		}
	}
	if f := strings.TrimSpace(n.Family); f != "" {
		parts = append(parts, f)
	}
	return strings.Join(parts, " ")
}

// parseHL7DateTime parses common HL7 timestamp formats.
func parseHL7DateTime(s string) (time.Time, error) {
	for _, layout := range []string{"20060102150405-0700", "20060102150405", "200601021504", "20060102"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cda_storage: cannot parse HL7 datetime %q", s)
}

// nullStr returns nil when s is empty, else returns s.
// Used to map empty strings to SQL NULL for optional VARCHAR columns.
func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullBytes returns nil when b is empty, else returns b.
func nullBytes(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}
