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

// DocumentTypeMetadata is the document-level templateId/LOINC code/title
// needed to construct a valid ClinicalDocument root for a given document
// type (e.g. "CCD") — used by cda/builder, the construction-direction
// counterpart to DocumentTemplates (which only maps OID -> display name).
type DocumentTypeMetadata struct {
	TemplateID    string `json:"templateId"`
	TemplateIDExt string `json:"templateIdExtension,omitempty"`
	LOINCCode     string `json:"loincCode"`
	Title         string `json:"title"`
}

// DocumentTypeSectionInfo lists which sections a document type SHALL, SHOULD, or MAY include.
type DocumentTypeSectionInfo struct {
	SHALL  []string `json:"SHALL"`
	SHOULD []string `json:"SHOULD"`
	MAY    []string `json:"MAY,omitempty"`
}

// CDAProfileDef is the top-level schema document (ccda_2_1.json).
type CDAProfileDef struct {
	Profile              string                              `json:"profile"`
	Version              string                              `json:"version"`
	HL7Version           string                              `json:"hl7Version"`
	DocumentTemplates     map[string]string                  `json:"documentTemplates"`     // OID → display name
	DocumentTypeMetadata  map[string]DocumentTypeMetadata     `json:"documentTypeMetadata"`  // doc type → templateId/LOINC/title, for document construction
	DocumentTypeSections  map[string]DocumentTypeSectionInfo  `json:"documentTypeSections"`  // doc type → section conformance lists
	Sections              []*CDASectionDef                    `json:"sections"`

	// Built at load time — not in JSON.
	sectionByKey        map[string]*CDASectionDef
	sectionByTemplateID map[string]*CDASectionDef
	sectionByLOINC      map[string]*CDASectionDef
}

// CDASectionDef defines a C-CDA section (allergies, medications, etc.).
type CDASectionDef struct {
	Key                string         `json:"key"`                        // e.g. "allergiesAndIntolerances"
	USCDIClass         string         `json:"uscdiClass"`                 // USCDI class label
	DisplayName        string         `json:"displayName"`                // Human-readable name
	LOINCCode          string         `json:"loincCode"`                  // Section LOINC code
	TemplateID         string         `json:"templateId"`                 // C-CDA 2.1 SHALL template OID
	TemplateIDExt      string         `json:"templateIdExtension,omitempty"` // @extension for TemplateID, e.g. "2015-08-01"
	TemplateIDOptional string         `json:"templateIdOptional"`         // C-CDA 2.1 MAY template OID
	Conformance        string         `json:"conformance"`                // SHALL / SHOULD / MAY
	EntryTemplateID    string         `json:"entryTemplateId,omitempty"`
	EntryTemplateIDExt string         `json:"entryTemplateIdExtension,omitempty"` // @extension for EntryTemplateID
	ObsTemplateID      string         `json:"observationTemplateId,omitempty"`
	ObsTemplateIDExt   string         `json:"observationTemplateIdExtension,omitempty"` // @extension for ObsTemplateID
	IsHeader           bool           `json:"isHeader,omitempty"`         // true for header pseudo-sections
	Fields             []*CDAFieldDef `json:"fields"`

	// EntryElementPath is the path (relative to "entry/") to this section's
	// root entry element (act/substanceAdministration/organizer/observation/
	// encounter/procedure/supply) — where EntryTemplateID gets injected as a
	// <templateId> child during CDA construction (cda/builder). Empty for
	// narrative-only sections (no discrete entries, e.g. "hospitalCourse").
	EntryElementPath string `json:"entryElementPath,omitempty"`

	// ObservationElementPath is the path (relative to "entry/") to this
	// section's nested sub-element — where ObsTemplateID gets injected.
	// Despite the name, this isn't always an <observation>: e.g.
	// payersInsurance's nested element is itself an <act> (COMP-typed
	// entryRelationship). Empty when a section has no second anchor
	// (substanceAdministration/encounter/procedure/supply archetypes, and
	// plain-observation sections with no further nesting).
	ObservationElementPath string `json:"observationElementPath,omitempty"`

	// StructuralTemplateIDs are third-level-and-deeper nested observations
	// (Reaction, Severity, Age, Problem Status, etc.) that need their own
	// <templateId> but aren't reachable via the two-anchor Entry/Observation
	// mechanism above — typically because two DIFFERENT nested observations
	// share the exact same entryRelationship predicate (e.g. Allergy's
	// Severity and Status observations both nest under
	// entryRelationship[@typeCode='SUBJ',@inversionInd='true'], distinguished
	// only by their OWN nested templateId — see StructuralTemplateAnchor.Path
	// for how that disambiguation is expressed). Applied AFTER every field is
	// written, and only when Path already resolves to a node a real field
	// write created (see cda/builder.TryFindAtXPath) — never force-creates an
	// empty container just to hold a templateId with no data.
	StructuralTemplateIDs []StructuralTemplateAnchor `json:"structuralTemplateIds,omitempty"`

	// EntryStatusCodeOverride replaces the generic per-tag statusCode default
	// (see cda/builder's tagBoilerplate table, e.g. "completed" for a plain
	// observation) on this section's ROOT entry element specifically. Needed
	// because a handful of entry archetypes assert a business-fixed
	// statusCode that differs from the tag's usual default — e.g. Plan of
	// Treatment's Planned Observation (moodCode SHALL be a "planned" value
	// like INT, not EVN) is itself SHALL statusCode="active"
	// (CONF:1098-32032), never "completed" — a discrete observation like
	// Allergy/Problem's own assertion observation is.
	EntryStatusCodeOverride string `json:"entryStatusCodeOverride,omitempty"`

	// EntryFixedCode/EntryFixedCodeSystem/EntryFixedCodeDisplay write a
	// static <code> child onto this section's ROOT entry element when no
	// field already targets "entry/<rootTag>/code" itself — needed for
	// archetypes whose OWN code is a business-fixed constant unrelated to
	// any per-record data, e.g. Coverage Activity's code is always
	// "48768-6" (Payment sources, matching the section's own LOINC code)
	// even though the record's real, per-entry code (Policy Activity's own
	// coverageType) lives one level deeper. EntryFixedCodeSystem defaults
	// to LOINC (2.16.840.1.113883.6.1) when EntryFixedCode is set but this
	// is empty, since that's true for every current use.
	EntryFixedCode        string `json:"entryFixedCode,omitempty"`
	EntryFixedCodeSystem  string `json:"entryFixedCodeSystem,omitempty"`
	EntryFixedCodeDisplay string `json:"entryFixedCodeDisplay,omitempty"`

	// ObsFixedCode/ObsFixedCodeSystem/ObsFixedCodeDisplay mirror
	// EntryFixedCode/EntryFixedCodeSystem/EntryFixedCodeDisplay exactly, but
	// write the static <code> child onto this section's NESTED OBSERVATION
	// element (ObservationElementPath) instead of the root entry element —
	// needed for archetypes whose observation carries its own business-fixed
	// classifying code, distinct from the per-record value one level deeper.
	// Motivating case: C-CDA 2.1's Problem Observation (templateId .4.4)
	// SHALL have exactly one <code> selected from the Problem Type value set
	// (ValueSet 2.16.840.1.113883.3.88.12.3221.7.2, e.g. SNOMED CT 55607006
	// "Problem") — a fixed classifier, separate from the actual diagnosis
	// code (e.g. ICD-10 "E11.9") that a real field write already puts on
	// <value>. Only written when no field already targets
	// "<observationElementPath>/code" itself, same "don't clobber real data"
	// guard EntryFixedCode uses. No default codeSystem is assumed here
	// (unlike EntryFixedCode's LOINC default) since there's no single system
	// true for every current/future use of this nested-observation slot.
	ObsFixedCode        string `json:"observationFixedCode,omitempty"`
	ObsFixedCodeSystem  string `json:"observationFixedCodeSystem,omitempty"`
	ObsFixedCodeDisplay string `json:"observationFixedCodeDisplay,omitempty"`

	// Built at load time.
	fieldByKey map[string]*CDAFieldDef
}

// StructuralTemplateAnchor names one nested element (by path, relative to
// "entry/") that needs a <templateId> injected once any of the section's
// fields has already caused it to exist. Path may itself use the
// "[childTag/@attr='value']" predicate form to both locate AND (on the field
// write that first creates it) construct the disambiguating child — e.g.
// "act/entryRelationship[@typeCode='SUBJ']/observation/entryRelationship[@typeCode='SUBJ',@inversionInd='true']/observation[templateId/@root='2.16.840.1.113883.10.20.22.4.8']"
// for Allergy's Severity Observation, where the predicate's own templateId/@root
// is what BuildEntry, in effect, uses twice: once (implicitly, via a field's
// own SourcePath ending inside that node) to create the node with the
// correct root already set, and once here to also add the @extension no
// predicate can express.
type StructuralTemplateAnchor struct {
	Path          string `json:"path"`
	TemplateID    string `json:"templateId"`
	TemplateIDExt string `json:"templateIdExtension,omitempty"`
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
