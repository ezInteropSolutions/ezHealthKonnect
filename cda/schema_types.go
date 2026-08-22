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
	SHALL             []string                  `json:"SHALL"`
	SHOULD            []string                  `json:"SHOULD"`
	MAY               []string                  `json:"MAY,omitempty"`
	ChoiceConstraints []SectionChoiceConstraint `json:"choiceConstraints,omitempty"`
}

// SectionChoiceConstraint represents a spec requirement of the form "SHALL
// contain <Branch A>, OR <Branch B>" (e.g. Consultation Note/Referral
// Note/Progress Note's "SHALL contain Assessment and Plan Section, OR both
// Assessment Section and Plan of Treatment Section") — a shape the flat
// SHALL/SHOULD/MAY lists above cannot express on their own. Satisfied if
// ANY branch's section keys are ALL present in the document. Each branch is
// a list of section keys that must ALL be present together for that branch
// to count.
//
// Scope boundary: this only affects conformance SCORING (cda/validator).
// The corresponding "SHALL NOT contain both branches" mutual-exclusion
// constraint some of these sections also carry is not enforced here or at
// build time — cda/builder trusts its canonical input same as everywhere
// else; a caller populating more than one branch gets all of it emitted.
type SectionChoiceConstraint struct {
	Description string     `json:"description"`
	Branches    [][]string `json:"branches"`
}

// CDAProfileDef is the top-level schema document (ccda_2_1.json).
type CDAProfileDef struct {
	Profile              string                             `json:"profile"`
	Version              string                             `json:"version"`
	HL7Version           string                             `json:"hl7Version"`
	DocumentTemplates    map[string]string                  `json:"documentTemplates"`    // OID → display name
	DocumentTypeMetadata map[string]DocumentTypeMetadata    `json:"documentTypeMetadata"` // doc type → templateId/LOINC/title, for document construction
	DocumentTypeSections map[string]DocumentTypeSectionInfo `json:"documentTypeSections"` // doc type → section conformance lists
	Sections             []*CDASectionDef                   `json:"sections"`

	// Built at load time — not in JSON.
	sectionByKey        map[string]*CDASectionDef
	sectionByTemplateID map[string]*CDASectionDef
	sectionByLOINC      map[string]*CDASectionDef
}

// CDASectionDef defines a C-CDA section (allergies, medications, etc.).
type CDASectionDef struct {
	Key                string         `json:"key"`                           // e.g. "allergiesAndIntolerances"
	USCDIClass         string         `json:"uscdiClass"`                    // USCDI class label
	DisplayName        string         `json:"displayName"`                   // Human-readable name
	LOINCCode          string         `json:"loincCode"`                     // Section LOINC code
	TemplateID         string         `json:"templateId"`                    // C-CDA 2.1 SHALL template OID
	TemplateIDExt      string         `json:"templateIdExtension,omitempty"` // @extension for TemplateID, e.g. "2015-08-01"
	TemplateIDOptional string         `json:"templateIdOptional"`            // C-CDA 2.1 MAY template OID
	Conformance        string         `json:"conformance"`                   // SHALL / SHOULD / MAY
	EntryTemplateID    string         `json:"entryTemplateId,omitempty"`
	EntryTemplateIDExt string         `json:"entryTemplateIdExtension,omitempty"` // @extension for EntryTemplateID
	ObsTemplateID      string         `json:"observationTemplateId,omitempty"`
	ObsTemplateIDExt   string         `json:"observationTemplateIdExtension,omitempty"` // @extension for ObsTemplateID
	IsHeader           bool           `json:"isHeader,omitempty"`                       // true for header pseudo-sections
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

	// EntryClassCodeOverride replaces the generic per-tag classCode default
	// (see cda/builder's tagBoilerplate table) on this section's ROOT entry
	// element specifically — the classCode counterpart to
	// EntryStatusCodeOverride, needed for the exact same reason: tagBoilerplate
	// is keyed purely by XML TAG NAME ("organizer"), but different C-CDA
	// organizer archetypes assert different fixed classCode values. E.g.
	// Vital Signs Organizer (V3) is SHALL classCode="CLUSTER" (the
	// tagBoilerplate default, confirmed against its own spec chapter), but
	// Result Organizer (V3) is classCode="BATTERY" (confirmed from that
	// template's own Figure 213 worked example — the constraint table itself
	// only says "SHALL contain exactly one classCode" without stating the
	// value, same pattern as Indication's fixed <code> being only visible in
	// its own worked example, not the constraint table).
	EntryClassCodeOverride string `json:"entryClassCodeOverride,omitempty"`

	// EntryMoodCodeOverride replaces the generic per-tag moodCode default
	// (tagBoilerplate's "EVN" for every archetype) on this section's ROOT
	// entry element — the moodCode counterpart to EntryStatusCodeOverride/
	// EntryClassCodeOverride, for the same reason: some archetypes are
	// SHALL a non-EVN moodCode. E.g. Instruction (V2) is SHALL
	// moodCode="INT" (CONF:1098-7392, "The template's moodCode can only be
	// INT") — confirmed directly against its own spec chapter, not assumed
	// from the generic "act" default.
	EntryMoodCodeOverride string `json:"entryMoodCodeOverride,omitempty"`

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

	// EntryCodeTranslationCode/System/Display write a static <translation>
	// child under whatever <code> element ends up on this section's ROOT
	// entry element — regardless of whether that <code> came from
	// EntryFixedCode or from a real per-record field (e.g. Advance
	// Directive's own directiveCode). Needed for archetypes whose C-CDA
	// constraint is "code SHALL contain exactly one translation" with a
	// business-fixed translation value — e.g. Advance Directive Observation
	// (V3)'s code SHALL have a translation code="75320-2" (LOINC "Advance
	// directive", CONF:1198-32842/32843/32844) regardless of what SNOMED/
	// other code the user's own directiveCode data supplies. Applied once,
	// after EntryFixedCode's own block, only when rootEl already has a
	// <code> child with no existing <translation> — confirmed via a real
	// schema validator run (2026-07) this is missing across every section
	// that needs it.
	EntryCodeTranslationCode    string `json:"entryCodeTranslationCode,omitempty"`
	EntryCodeTranslationSystem  string `json:"entryCodeTranslationSystem,omitempty"`
	EntryCodeTranslationDisplay string `json:"entryCodeTranslationDisplay,omitempty"`

	// ObsCodeTranslationCode/System/Display mirror EntryCodeTranslationCode/
	// System/Display exactly, but target the NESTED OBSERVATION element's
	// own <code> (ObservationElementPath) instead of the root entry
	// element's — e.g. Problem Observation (V3)'s own code (ObsFixedCode
	// "55607006" SNOMED "Problem") SHALL have a translation code="75326-9"
	// (LOINC "Problem") when its code is selected from the Problem Type
	// (SNOMED CT) value set, which the fixed 55607006 discriminator always
	// is (CONF:1098-32950).
	ObsCodeTranslationCode    string `json:"obsCodeTranslationCode,omitempty"`
	ObsCodeTranslationSystem  string `json:"obsCodeTranslationSystem,omitempty"`
	ObsCodeTranslationDisplay string `json:"obsCodeTranslationDisplay,omitempty"`

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

	// RepeatingGroups declares the section's "loop" points — a canonical
	// array field that repeats one XML element pattern once per item, as
	// siblings (e.g. Vital Signs Organizer's 1..* <component> children, each
	// sharing one effectiveTime). See RepeatingGroup's own doc comment for
	// why this covers both the organizer/component shape and the
	// entryRelationship-repeat shape with one mechanism. Absent/empty for
	// every section that doesn't need it — zero effect on existing sections.
	RepeatingGroups []RepeatingGroup `json:"repeatingGroups,omitempty"`

	// AlternateArchetypes lets one section (one <component><section>
	// element) contain entries of more than one structural shape — the
	// motivating case is C-CDA's Medical Equipment Section2 (CONF:1098-
	// 31885/31886), which permits, alongside its primary Non-Medicinal
	// Supply Activity entries (this section's own EntryElementPath/Fields),
	// an ADDITIONAL, independent set of Procedure Activity Procedure (V2)
	// entries — the SAME archetype the Procedures section already builds,
	// reused here since the IG genuinely allows one template inside two
	// different sections. Each alternate reads its own canonical sub-key
	// (sections.<key>.<EntriesKey>), kept separate from the section's own
	// top-level "entries" so cda.map_to_canonical can populate the two
	// shapes independently with zero field-name collision risk. Absent/
	// empty for every section that doesn't need a second shape.
	AlternateArchetypes []AlternateEntryArchetype `json:"alternateArchetypes,omitempty"`

	// Built at load time.
	fieldByKey map[string]*CDAFieldDef
}

// AlternateEntryArchetype is one additional entry shape for a section that
// needs more than the single EntryElementPath/Fields pair CDASectionDef
// itself provides (see AlternateArchetypes' own doc comment for the
// motivating case). Deliberately a small subset of CDASectionDef's own
// knobs — only what's actually been needed so far (no
// ObservationElementPath/RepeatingGroups — nothing built so far needs a
// nested-observation anchor or a repeating loop on an alternate shape);
// extend this struct, not buildEntry itself, if a future alternate needs
// more. StructuralTemplateIDs was added once Medical Equipment's Procedure2
// alternate needed its own Author Participation anchor (same mechanism
// CDASectionDef.StructuralTemplateIDs already provides for the primary
// shape).
type AlternateEntryArchetype struct {
	// EntriesKey is the canonical sub-key (sections.<sectionKey>.<EntriesKey>)
	// this alternate's own rows live under — set by a SEPARATE
	// cda.map_to_canonical sectionMappingRow sharing the same SectionKey but
	// its own EntriesKey (see map_to_canonical_executor.go).
	EntriesKey         string         `json:"entriesKey"`
	EntryElementPath   string         `json:"entryElementPath"`
	EntryTemplateID    string         `json:"entryTemplateId,omitempty"`
	EntryTemplateIDExt string         `json:"entryTemplateIdExtension,omitempty"`
	Fields             []*CDAFieldDef `json:"fields"`

	// StructuralTemplateIDs mirrors CDASectionDef's own field of the same
	// name exactly — third-level-and-deeper nested elements (e.g. an
	// Author Participation anchor under this alternate's own root) that
	// need their own <templateId>/FixedCode/FixedStatusCode but aren't
	// reachable via EntryElementPath alone.
	StructuralTemplateIDs []StructuralTemplateAnchor `json:"structuralTemplateIds,omitempty"`

	// ComponentGroups declares this alternate's own heterogeneous-component
	// wrappers (see ComponentGroup's own doc comment) — e.g. Medical
	// Equipment's Procedure Activity Procedure alternate carrying a UDI
	// Organizer. Added here (not on CDASectionDef) because the first real
	// need for this mechanism is on an alternate shape; add it to
	// CDASectionDef too, symmetrically, if a primary shape ever needs one.
	ComponentGroups []ComponentGroup `json:"componentGroups,omitempty"`
}

// RepeatingGroup is the schema-driven, data-only counterpart to hand-writing
// a loop, for SECTION ENTRY content (clinical facts needing full boilerplate/
// templateId/nested-observation support). Not to be confused with
// cda/builder/header_fields.go's writeRepeatingGroup (singular) — a
// deliberately much simpler, HEADER-only mechanism (patient ids[],
// informants[], performers[]: one flat tag per item, no boilerplate or
// templateId, since header elements don't carry classCode/moodCode the way
// entries do). Different problem shapes, kept as two mechanisms rather than
// forcing one to cover both.
//
// Mirrors this section's own EntryElementPath -> ObservationElementPath
// -> Fields convention, just nested one level deeper and repeated once per
// item in a canonical array:
//
//	record[Key] == []interface{}{item1, item2, ...}
//	  -> a brand-new WrapperTag element created per item (always CREATE,
//	       never find-or-reuse — every item is a genuinely new sibling, so
//	       there's no XPath-predicate matching risk to reason about; see
//	       writeRepeatingGroups' own doc comment for why this is direct
//	       etree creation, not a WriteAtXPath positional predicate), with
//	       WrapperAttr/WrapperAttrValue set if configured
//	       -> each gets ObservationElementPath resolved relative to it
//	            (boilerplate + TemplateID injected exactly like the
//	            section-level ObservationElementPath already does)
//	       -> Fields written relative to that resolved element, via the
//	            SAME writeFieldValue every flat section already uses
//
// Two real C-CDA shapes both reduce to this: Vital Signs' "1..* <component>"
// (WrapperTag="component", no WrapperAttr needed — nothing else creates
// <component> under <organizer>) and Medication's "0..* <entryRelationship
// typeCode='RSON'>" (WrapperTag="entryRelationship", WrapperAttr="typeCode",
// WrapperAttrValue="RSON" — needed because OTHER unrelated entryRelationship
// children, e.g. REFR/CAUS/SUBJ, can share the same parent) — the wrapper
// tag/attribute differs, the repeat mechanism doesn't.
type RepeatingGroup struct {
	Key        string `json:"key"`
	WrapperTag string `json:"wrapperTag"`

	// WrapperAttr/WrapperAttrValue set ONE constant attribute directly on
	// each newly-created WrapperTag element (e.g. WrapperAttr="typeCode",
	// WrapperAttrValue="RSON" for Medication Activity's repeatable
	// Indication entryRelationship, CONF:1098-7536/7537). Required whenever
	// WrapperTag is a bare tag name that repeats under a parent ALSO used by
	// other, unrelated same-tag children (e.g. medications' substanceAdministration
	// root can carry entryRelationship siblings for Reaction/Supply/Dispense
	// too, each with a different typeCode) — see writeRepeatingGroups' own
	// doc comment for why plain positional creation is unsafe in that case.
	// Empty for a WrapperTag that's already unambiguous on its own (e.g.
	// Vital Signs' "component" — nothing else creates <component> under
	// <organizer>).
	WrapperAttr      string `json:"wrapperAttr,omitempty"`
	WrapperAttrValue string `json:"wrapperAttrValue,omitempty"`

	ObservationElementPath string         `json:"observationElementPath,omitempty"`
	TemplateID             string         `json:"templateId,omitempty"`
	TemplateIDExt          string         `json:"templateIdExtension,omitempty"`
	Fields                 []*CDAFieldDef `json:"fields"`

	// FixedCode/FixedCodeSystem/FixedCodeDisplay mirror ObsFixedCode exactly
	// (same "don't clobber real data" guard — only written when no field
	// already put a <code> under ObservationElementPath's resolved element),
	// scoped to THIS repeating item instead of the section's one nested
	// observation. Needed when the item's own classifying code is a
	// business-fixed constant per the IG rather than per-record data — e.g.
	// Indication (V2)'s own <code> is a generic "this is an assertion of
	// fact" discriminator, SHALL code="ASSERTION"/codeSystem=2.16.840.1.113883.5.4
	// (confirmed directly from the IG's own Figure 164 example — the
	// constraint table only says "SHALL contain exactly one code", the fixed
	// value itself is only visible in the worked example), never per-record
	// data the way the item's <value> (the real referenced diagnosis) is.
	FixedCode        string `json:"fixedCode,omitempty"`
	FixedCodeSystem  string `json:"fixedCodeSystem,omitempty"`
	FixedCodeDisplay string `json:"fixedCodeDisplay,omitempty"`

	// AnchorRoot selects which of the section's two existing anchors
	// WrapperTag attaches under: "" or "root" -> the section's
	// EntryElementPath-resolved root (e.g. Vital Signs Organizer's
	// <organizer> — the common case), "observation" -> the section's
	// ObservationElementPath-resolved nested element (needed for sections
	// where the repeating entryRelationship nests one level deeper, e.g.
	// under Problem/Allergy's own SUBJ-observation rather than the wrapping
	// act). Mirrors the section-level EntryElementPath/ObservationElementPath
	// two-anchor convention rather than inventing a third addressing scheme.
	AnchorRoot string `json:"anchorRoot,omitempty"`

	// RequiredPaths: skip this array item entirely (don't create its
	// wrapper element at all) unless the item's own record has a non-empty
	// value at every one of these canonical keys — the "don't build an
	// empty/junk instance" gate. Keys are relative to the item, same
	// vocabulary as Fields[].Key.
	RequiredPaths []string `json:"requiredPaths,omitempty"`

	// AuthorTemplateID/AuthorTemplateIDExt, if set, stamp a <templateId>
	// onto THIS item's own "author" child element once Fields has caused it
	// to exist (e.g. an authorGiven/authorFamily field targeting
	// "author/assignedAuthor/assignedPerson/name/given") — the
	// RepeatingGroup-scoped counterpart to the section-level
	// StructuralTemplateAnchor mechanism. Needed because Author
	// Participation (templateId 2.16.840.1.113883.10.20.22.4.119) can repeat
	// once per RepeatingGroup item (e.g. each Result/Vital Sign Observation
	// has its own independent optional author) rather than once per
	// section, so a single section-level StructuralTemplateAnchor can't
	// reach every item's own <author>.
	AuthorTemplateID    string `json:"authorTemplateId,omitempty"`
	AuthorTemplateIDExt string `json:"authorTemplateIdExt,omitempty"`
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

	// FixedCode/FixedCodeSystem/FixedCodeDisplay write a static <code> child
	// onto this anchor's element when no field already targets
	// "<path>/code" itself — the anchor-scoped counterpart to
	// EntryFixedCode/ObsFixedCode (which only ever cover the section's ONE
	// root/nested-observation element). Needed for entryRelationship
	// sub-templates whose own classifying code is a business-fixed constant
	// per the IG rather than per-record data — e.g. Problem Observation's
	// Prognosis entryRelationship (templateId .4.113) SHALL have exactly one
	// code="75328-5" (LOINC "Prognosis", CONF:1098-29039/29468/31349), and
	// its Priority Preference entryRelationship (templateId .4.143) SHALL
	// have exactly one code="225773000" (SNOMED CT "Preference",
	// CONF:1098-30954/30955/30956) — same "don't clobber real data" guard
	// EntryFixedCode uses (only written when no field write already put a
	// <code> there).
	FixedCode        string `json:"fixedCode,omitempty"`
	FixedCodeSystem  string `json:"fixedCodeSystem,omitempty"`
	FixedCodeDisplay string `json:"fixedCodeDisplay,omitempty"`

	// FixedStatusCode overrides applyTagBoilerplate's own per-TAG statusCode
	// default (e.g. "act" -> "active") on THIS anchor's element specifically —
	// the anchor-scoped counterpart to CDASectionDef.EntryStatusCodeOverride.
	// Needed because a handful of nested sub-templates assert a business-fixed
	// statusCode that differs from their tag's usual default — e.g.
	// Immunization's Substance Administered Act (templateId .4.118) is SHALL
	// statusCode="completed" (CONF:1098-31505) even though a bare <act> tag
	// otherwise defaults to "active".
	FixedStatusCode string `json:"fixedStatusCode,omitempty"`
}

// ComponentGroup models a shared wrapper element (e.g. C-CDA's UDI Organizer)
// containing a FIXED, enumerated set of independently-optional, differently-
// shaped sub-observations — the heterogeneous counterpart to RepeatingGroup.
// RepeatingGroup assumes N items that all share ONE TemplateID/FixedCode/
// Fields shape (e.g. Medication's 0..* Indications); ComponentGroup instead
// has a known, finite Components list, each with its OWN TemplateID/
// FixedCode/Fields/DataType, where any subset (as few as zero, though the
// whole group is only emitted if at least one Component has data) may be
// present per real document.
//
// This shape can't be built with WriteAtXPath's own predicate-matching
// (xpath_writer.go's findOrCreateChild/candidatesForSegment): disambiguating
// repeated same-tag siblings there requires the AMBIGUOUS segment itself to
// carry a "[predicate]" (the discriminator-lookahead only fires when the
// CURRENT segment has one). A UDI Organizer's own <component> wrapper has no
// attributes to predicate on — it's bare and repeated, distinguished only by
// what's nested inside it — so a bare "component" xpath segment would always
// resolve to the FIRST existing <component>, wrongly collapsing every
// sub-observation into one. writeComponentGroups (cda/builder/
// entry_archetypes.go) instead always DIRECT-CREATES a new WrapperTag/
// ComponentTag sibling per present Component, the same "never risk a
// predicate match, just create" approach writeRepeatingGroups already uses
// for its own items.
type ComponentGroup struct {
	Key string `json:"key"` // doc/debug label only, e.g. "udiOrganizer"

	// WrapperTag/WrapperAttr/WrapperAttrValue mirror RepeatingGroup's own
	// fields of the same name — the element (e.g. "entryRelationship",
	// typeCode="COMP") that hosts this group's own OrganizerTag.
	WrapperTag       string `json:"wrapperTag"`
	WrapperAttr      string `json:"wrapperAttr,omitempty"`
	WrapperAttrValue string `json:"wrapperAttrValue,omitempty"`

	// OrganizerTag/OrganizerTemplateID/OrganizerTemplateIDExt/
	// OrganizerFixedCode* describe the ONE shared wrapper element (e.g.
	// "organizer", UDI Organizer's own templateId + fixed LOINC 74711-3)
	// that every Component nests inside. OrganizerFixedStatusCode is rarely
	// needed — tagBoilerplate's own per-tag default (e.g. "organizer" ->
	// "completed") already covers UDI Organizer's own SHALL — but kept for
	// symmetry with StructuralTemplateAnchor.FixedStatusCode, for a future
	// ComponentGroup on a tag whose boilerplate default doesn't match.
	OrganizerTag               string `json:"organizerTag"`
	OrganizerTemplateID        string `json:"organizerTemplateId"`
	OrganizerTemplateIDExt     string `json:"organizerTemplateIdExtension,omitempty"`
	OrganizerFixedCode         string `json:"organizerFixedCode,omitempty"`
	OrganizerFixedCodeSystem   string `json:"organizerFixedCodeSystem,omitempty"`
	OrganizerFixedCodeDisplay  string `json:"organizerFixedCodeDisplay,omitempty"`
	OrganizerFixedStatusCode   string `json:"organizerFixedStatusCode,omitempty"`

	// OrganizerIDField/OrganizerIDSystemField are canonical field KEYS read
	// from the SAME top-level record map buildAlternateEntry already has
	// (NOT a nested per-component map) — the organizer's own <id>/@extension
	// and @root. Pointing these at a field key ANOTHER part of the section
	// already uses (e.g. Medical Equipment's own Tier-1 "deviceIdentifier"/
	// "deviceIdentifierSystem" on Product Instance) satisfies a spec
	// requirement like "the Organizer id SHALL match Product Instance's own
	// id" for free — one caller-supplied value flows into both places, no
	// new correlation logic.
	OrganizerIDField       string `json:"organizerIdField"`
	OrganizerIDSystemField string `json:"organizerIdSystemField,omitempty"`

	Components []ComponentDef `json:"components"`
}

// ComponentDef is ONE possible sub-observation within a ComponentGroup,
// gated by its own Key: emitted only when record[Key] has a non-empty value
// (checked the same way RepeatingGroup.RequiredPaths already checks
// presence, via stringValue). Fields' XPaths are bare, relative to this
// component's own resolved ObsElementPath element — the same convention
// RepeatingGroup.Fields already uses (resolved at BUILD time by
// writeFieldValue, never at schema-load time by resolveXPath).
type ComponentDef struct {
	Key            string `json:"key"`
	ComponentTag   string `json:"componentTag"`   // e.g. "component"
	ObsElementPath string `json:"obsElementPath"` // e.g. "observation", relative to ComponentTag

	TemplateID    string `json:"templateId"`
	TemplateIDExt string `json:"templateIdExtension,omitempty"`

	FixedCode        string `json:"fixedCode"`
	FixedCodeSystem  string `json:"fixedCodeSystem"`
	FixedCodeDisplay string `json:"fixedCodeDisplay,omitempty"`

	Fields []*CDAFieldDef `json:"fields"`
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

	// SkipIfXPathPresent: don't write this field if something else has
	// already written a value at this OTHER xpath (relative to the same
	// root writeFieldValue was called with) — the "don't clobber/duplicate"
	// condition mechanism. E.g. a derived/default field that should only
	// fill in when a more specific field didn't already supply a value.
	// Checked via TryFindAtXPath, which already exists for exactly this
	// kind of presence check (see StructuralTemplateIDs' own usage).
	SkipIfXPathPresent string `json:"skipIfXPathPresent,omitempty"`
}

// C32MappingFile is the root of cda/schemas/c32_mapping.json.
type C32MappingFile struct {
	Version          string                `json:"version"`
	Source           string                `json:"source"`
	Description      string                `json:"description"`
	ProfileDetection C32ProfileDetection   `json:"profileDetection"`
	Mappings         []*C32TemplateMapping `json:"mappings"`
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
