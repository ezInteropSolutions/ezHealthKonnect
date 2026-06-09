// uscdi/vocabulary_types.go
// Type definitions for the USCDI vocabulary layer.
// USCDI v3 anchors all clinical field labels, search terms, and section names
// across HL7, CDA, and FHIR formats — users never see LOINC codes or OIDs.

package uscdi

// USCDIVersion identifies the USCDI standard release.
type USCDIVersion string

const (
	USCDIv3 USCDIVersion = "v3"
	USCDIv4 USCDIVersion = "v4"
	USCDIv6 USCDIVersion = "v6"
)

// USCDIElement is one row from the USCDI v3 data elements spreadsheet.
// The JSON tags match the field names in cda/schemas/uscdi_v3.json.
type USCDIElement struct {
	// Clinical identity (USCDI vocabulary)
	Class   string `json:"class"`
	Element string `json:"element"`

	// Terminology binding
	ValueSet string `json:"valueSet"`

	// HL7v2 mappings
	HL7Field  string `json:"hl7Field"`
	HL7Inject bool   `json:"hl7Inject"`

	// CDA/CCD mappings
	CCDInject  bool   `json:"ccdInject"`
	CDASection string `json:"cdaSection"`
	CDAField   string `json:"cdaField"`

	// FHIR R4 mappings
	FHIRResource string `json:"fhirResource"`
	FHIRPath     string `json:"fhirPath"`
}

// SearchResult is the payload returned by USCDIVocabulary.Search.
// Short path is always populated; FHIRPath and CDAXPath are surfaced only
// when the caller requests the expanded view.
type SearchResult struct {
	// Always included
	ShortPath  string `json:"shortPath"`
	Label      string `json:"label"`
	USCDIClass string `json:"uscdiClass"`
	ValueSet   string `json:"valueSet,omitempty"`
	Format     string `json:"format"`

	// Included on expand
	FHIRPath string `json:"fhirPath,omitempty"`
	CDAXPath string `json:"cdaXPath,omitempty"`
	HL7Field string `json:"hl7Field,omitempty"`

	// Conformance from CDA schema (SHALL/SHOULD/MAY) — populated by CDAPathResolver
	Conformance string `json:"conformance,omitempty"`

	// Ranking signal — not serialised to the client
	Score float64 `json:"-"`

	// Source element for downstream enrichment
	Source *USCDIElement `json:"-"`
}

// ConformanceLevel represents C-CDA entry conformance.
type ConformanceLevel string

const (
	ConformanceSHALL  ConformanceLevel = "SHALL"
	ConformanceSHOULD ConformanceLevel = "SHOULD"
	ConformanceMAY    ConformanceLevel = "MAY"
)
