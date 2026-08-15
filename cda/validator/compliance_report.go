// Package validator provides C-CDA 2.1 conformance validation for typed CDADocument structs.
// It reads conformance rules from ccda_2_1.json via the CDASchemaLoader and produces
// a ComplianceReport per document with per-section and per-field conformance status.
//
// Typical usage:
//
//	v := validator.NewCDAConformanceValidator(loader)
//	report := v.Validate(doc)          // doc is *cdadocument.CDADocument from Sprint B
package validator

import "time"

// ComplianceReport is the root output of a CDA conformance validation run.
// Scores are normalised to [0.0, 1.0]: 1.0 means fully conformant at that level.
type ComplianceReport struct {
	DocumentId              string                   `json:"documentId"`
	DocumentType            string                   `json:"documentType"`
	OverallScore            float64                  `json:"overallScore"` // SHALL*0.7 + SHOULD*0.3
	ShallScore              float64                  `json:"shallScore"`   // present SHALL / total SHALL (each ChoiceConstraint counts as one SHALL unit)
	ShouldScore             float64                  `json:"shouldScore"`  // present SHOULD / total SHOULD
	SectionReports          []SectionReport          `json:"sectionReports"`
	ChoiceConstraintReports []ChoiceConstraintReport `json:"choiceConstraintReports,omitempty"`
	GeneratedAt             time.Time                `json:"generatedAt"`
}

// ChoiceConstraintReport captures conformance status for one
// SectionChoiceConstraint (e.g. "Assessment and Plan Section OR both
// Assessment Section and Plan of Treatment Section") in the document.
type ChoiceConstraintReport struct {
	Description     string   `json:"description"`
	Satisfied       bool     `json:"satisfied"`
	SatisfiedBranch []string `json:"satisfiedBranch,omitempty"` // which branch's section keys satisfied it, if any
}

// SectionReport captures conformance status for one CDA section in the document.
type SectionReport struct {
	TemplateId   string        `json:"templateId"`
	SectionKey   string        `json:"sectionKey"`
	Title        string        `json:"title"`
	Conformance  string        `json:"conformance"`  // "SHALL" | "SHOULD" | "MAY"
	Status       string        `json:"status"`       // "present" | "missing" | "present_empty"
	EntryCount   int           `json:"entryCount"`
	FieldReports []FieldReport `json:"fieldReports"`
}

// FieldReport captures conformance status for one extractable field within a section.
type FieldReport struct {
	FieldPath    string `json:"fieldPath"`    // Go struct path e.g. "Entry.Code" or "Entry.Consumable.ManufacturedMaterial.Code"
	Conformance  string `json:"conformance"`  // "SHALL" | "SHOULD" | "MAY"
	Status       string `json:"status"`       // "populated" | "missing" | "null_flavor"
	ValueSummary string `json:"valueSummary"` // e.g. "3 identifiers" | "SNOMED 73211009" | "RxNorm 1049502"
}
