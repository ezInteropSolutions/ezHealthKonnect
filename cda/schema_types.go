// cda/schema_types.go
// Type definitions for the C-CDA 2.1 schema layer.
// These types map directly to cda/schemas/ccda_2_1.json.

package cda

// CDAProfile identifies which CDA profile a document conforms to.
type CDAProfile string

const (
	ProfileCCDA21 CDAProfile = "C-CDA 2.1"
	ProfileC32    CDAProfile = "C32"
	ProfileHITSP  CDAProfile = "HITSP"
	ProfileBase   CDAProfile = "CDA R2"
)

// CDAProfileDef is the top-level schema document (ccda_2_1.json).
type CDAProfileDef struct {
	Profile           string                    `json:"profile"`
	Version           string                    `json:"version"`
	HL7Version        string                    `json:"hl7Version"`
	DocumentTemplates map[string]string         `json:"documentTemplates"` // OID → display name
	Sections          []*CDASectionDef          `json:"sections"`

	// Built at load time — not in JSON.
	sectionByKey        map[string]*CDASectionDef
	sectionByTemplateID map[string]*CDASectionDef
	sectionByLOINC      map[string]*CDASectionDef
}

// CDASectionDef defines a C-CDA section (allergies, medications, etc.).
type CDASectionDef struct {
	Key                string        `json:"key"`               // e.g. "allergiesAndIntolerances"
	USCDIClass         string        `json:"uscdiClass"`        // USCDI class label
	DisplayName        string        `json:"displayName"`       // Human-readable name
	LOINCCode          string        `json:"loincCode"`         // Section LOINC code
	TemplateID         string        `json:"templateId"`        // C-CDA 2.1 SHALL template OID
	TemplateIDOptional string        `json:"templateIdOptional"` // C-CDA 2.1 MAY template OID
	Conformance        string        `json:"conformance"`       // SHALL / SHOULD / MAY
	EntryTemplateID    string        `json:"entryTemplateId,omitempty"`
	ObsTemplateID      string        `json:"observationTemplateId,omitempty"`
	Fields             []*CDAFieldDef `json:"fields"`

	// Built at load time.
	fieldByKey map[string]*CDAFieldDef
}

// CDAFieldDef defines one extractable field within a CDA section.
// XPath strings are absolute relative to the <section> element.
type CDAFieldDef struct {
	Key          string `json:"key"`          // e.g. "medicationAllergyCode"
	USCDIElement string `json:"uscdiElement"` // USCDI element label for display
	Conformance  string `json:"conformance"`  // SHALL / SHOULD / MAY
	DataType     string `json:"dataType"`     // HL7 data type (CD, PQ, TS, etc.)
	ValueSet     string `json:"valueSet,omitempty"`

	// Primary XPath for the coded value (relative to <section>).
	XPath string `json:"xpath"`

	// Supplementary XPaths for associated attributes.
	XPathDisplay string `json:"xpathDisplay,omitempty"`
	XPathSystem  string `json:"xpathSystem,omitempty"`
	XPathUnit    string `json:"xpathUnit,omitempty"`
	XPathFamily  string `json:"xpathFamily,omitempty"`
}

// C32MappingFile is the root of cda/schemas/c32_mapping.json.
type C32MappingFile struct {
	Version          string                 `json:"version"`
	Source           string                 `json:"source"`
	Description      string                 `json:"description"`
	ProfileDetection C32ProfileDetection    `json:"profileDetection"`
	Mappings         []*C32TemplateMapping  `json:"mappings"`
}

// C32ProfileDetection holds OIDs used to detect whether a document is C32 or HITSP.
type C32ProfileDetection struct {
	C32TemplateIDs   []string `json:"c32TemplateIds"`
	HITSPTemplateIDs []string `json:"hitspTemplateIds"`
}

// C32TemplateMapping is one row in the C32→C-CDA 2.1 template OID mapping table.
type C32TemplateMapping struct {
	Table          string   `json:"table"`
	Column         string   `json:"column"`
	NewTemplateID  string   `json:"newTemplateId"`
	OldTemplateIDs []string `json:"oldTemplateIds"`
}
