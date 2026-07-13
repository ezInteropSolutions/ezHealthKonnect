// Package cdadocument defines the typed Go struct hierarchy for a complete CDA document.
// It is the CDA equivalent of the HL7 EnhancedSegments model — every field is represented
// as a Go type; nothing is silently dropped. Produced by CDAParser.ParseDocument().
package cdadocument

// CDADocument is the root typed representation of a parsed CDA/CCD document.
type CDADocument struct {
	Header        CDAHeader              `json:"header"`
	Sections      []CDASection           `json:"sections,omitempty"`      // ordered as they appear in the document
	SectionsByKey map[string]*CDASection `json:"sectionsByKey,omitempty"` // keyed by section key from ccda_2_1.json
	Raw           string                 `json:"raw,omitempty"`           // original XML preserved verbatim
}

// CDAHeader contains all clinical document header elements.
type CDAHeader struct {
	DocumentId            CDAII                  `json:"documentId"`
	DocumentCode          CDACode                `json:"documentCode"`
	Title                 string                 `json:"title,omitempty"`
	SetId                 CDAII                  `json:"setId"`
	VersionNumber         string                 `json:"versionNumber,omitempty"`
	EffectiveTime         CDATime                `json:"effectiveTime"`
	ConfidentialityCode   CDACode                `json:"confidentialityCode"`
	Patient               CDAPatient             `json:"patient"`
	Authors               []CDAAuthor            `json:"authors,omitempty"`
	Custodian             CDACustodian           `json:"custodian"`
	LegalAuthenticator    *CDALegalAuthenticator `json:"legalAuthenticator,omitempty"`
	DocumentOf            *CDAServiceEvent       `json:"documentOf,omitempty"`
	EncompassingEncounter *CDAEncounter          `json:"encompassingEncounter,omitempty"`
	RelatedDocuments      []CDARelatedDocument   `json:"relatedDocuments,omitempty"`
	// Informants are document-level <informant><assignedEntity> elements —
	// distinct from an entry's own Informants (see CDASection) and from
	// Authors: a source of information for the WHOLE document (e.g. a
	// referring provider) who did not author it. Reuses CDAInformant, whose
	// <assignedEntity> shape is identical at header and entry level.
	Informants []CDAInformant `json:"informants,omitempty"`
}

// CDAPatient holds all patient demographics extracted from recordTarget/patientRole.
type CDAPatient struct {
	Ids           []CDAII       `json:"ids,omitempty"`       // ALL <id> elements — SSN, MRN, Medicare, etc.
	Names         []CDAName     `json:"names,omitempty"`     // ALL <name> elements — legal, maiden, alias
	Addresses     []CDAAddress  `json:"addresses,omitempty"` // ALL <addr> elements — home, work, etc.
	Telecoms      []CDATelecom  `json:"telecoms,omitempty"`  // ALL <telecom> elements — phone, fax, email
	BirthDate     CDATime       `json:"birthDate"`
	Gender        CDACode       `json:"gender"`
	// DeceasedInd is sdtc:deceasedInd's value attribute. A pointer (not a
	// bare bool) because CDA can explicitly assert value="false" — that's
	// real data (FHIR Patient.deceasedBoolean=false), distinct from the
	// element being absent entirely (no deceased field on the FHIR side at
	// all). nil means absent.
	DeceasedInd   *bool         `json:"deceasedInd,omitempty"`
	MaritalStatus CDACode       `json:"maritalStatus"`
	Race          CDACode       `json:"race"`
	Ethnicity     CDACode       `json:"ethnicity"`
	// Religion is religiousAffiliationCode — codeSystem
	// 2.16.840.1.113883.5.1076 (HL7 ReligiousAffiliation), mapped to the
	// base FHIR patient-religion extension (no US Core equivalent exists).
	Religion      CDACode       `json:"religion"`
	Languages     []CDALanguage `json:"languages,omitempty"`
}

// CDASection represents one clinical section of a CDA document body.
type CDASection struct {
	TemplateIds   []string   `json:"templateIds,omitempty"`
	Key           string     `json:"key,omitempty"` // resolved from templateId via ccda_2_1.json, then LOINC, then title
	LoincCode     string     `json:"loincCode,omitempty"`
	Title         string     `json:"title,omitempty"`
	NarrativeText string     `json:"narrativeText,omitempty"` // serialised <text> element XML
	Entries       []CDAEntry `json:"entries,omitempty"`
	NullFlavor    string     `json:"nullFlavor,omitempty"` // "NI", "NA", etc. if section present but empty
}

// CDAEntry represents one clinical act extracted from a CDA section <entry> element.
type CDAEntry struct {
	Id            []CDAII      `json:"id,omitempty"`
	Code          CDACode      `json:"code"`
	StatusCode    string       `json:"statusCode,omitempty"`
	EffectiveTime CDATimeRange `json:"effectiveTime"` // first <effectiveTime> only — kept for backward compatibility
	Value         *CDAValue    `json:"value,omitempty"`
	// AdditionalValues holds sibling <value> elements beyond the primary one
	// (Value) that could NOT be merged into it. C-CDA allows [1..*] <value>
	// on some templates (e.g. Assessment Scale Supporting Observation), but
	// FHIR R4's Observation.value[x] can only ever hold one shape at a time.
	// entry_parser.go's multi-<value> handling (see its own comment) already:
	//   1. drops blank/nullFlavor-only siblings entirely (pure noise), and
	//   2. folds any sibling that is ALSO coded into Value.Code.Translations
	//      (same "alternate coding, same concept" relationship a genuine
	//      <translation> child already has — CDACodeToCodeableConcept
	//      renders both the same way).
	// Only a sibling with a genuinely different value[x] shape than Value
	// (e.g. Value is an INT score, the sibling is a CD interpretation) ends
	// up here, where the declarative engine surfaces it as a warning instead
	// of silently losing it. Real gap found auditing the 99397 sample's
	// Functional Status section: a PHQ-2 total-score entry carries an INT raw
	// score AND a CD/SNOMED interpretation as two sibling <value> elements.
	AdditionalValues []CDAValue `json:"additionalValues,omitempty"`
	// InterpretationCode is the Result Observation's <interpretationCode>
	// (CONF:1198-7147, confirmed via WebFetch against build.fhir.org/ig/HL7/
	// CDA-ccda-2.1-sd 2026-06-22): a direct child of the observation element,
	// a sibling of <code>/<statusCode>/<value> — NOT nested under an
	// entryRelationship. 0..* per the IG; only the first is captured here,
	// matching every other single-value CDACode field on this struct.
	InterpretationCode CDACode                `json:"interpretationCode,omitempty"`
	// ReferenceRangeText is the Result Observation's <referenceRange>
	// <observationRange><text> — a direct child of the observation, sibling
	// of <interpretationCode>. Real gap found auditing Results (Ascension
	// Wisconsin sample): every one of its 46 lab Observations (CBC, Chem
	// panel, differential) carries a real free-text range (e.g. "4.0 - 10.0
	// Thou/uL"), parsed nowhere before this — there was a registered FHIR
	// transform (cda_reference_range_to_fhir) but it expects a STRUCTURED
	// <observationRange><value><low/high> shape, which 0/4 corpus files
	// actually use; only the free-text <observationRange><text> shape is
	// evidenced, so only that is captured. Only the first <referenceRange>
	// is kept (C-CDA allows 0..*, but no corpus entry has more than one).
	ReferenceRangeText string `json:"referenceRangeText,omitempty"`
	Participants       []CDAParticipant       `json:"participants,omitempty"`
	Components         []CDAEntry             `json:"components,omitempty"` // nested organizer/component entries
	EntryRelationships []CDAEntryRelationship `json:"entryRelationships,omitempty"`
	EntryType          string                 `json:"entryType,omitempty"` // "observation" | "act" | "procedure" | "substanceAdministration" | "encounter" | "organizer" | "supply"
	ClassCode          string                 `json:"classCode,omitempty"`
	MoodCode           string                 `json:"moodCode,omitempty"`
	NullFlavor         string                 `json:"nullFlavor,omitempty"`

	// NegationInd marks this act as negated, e.g. @negationInd="true" on an
	// Immunization Activity ("not given") or an Allergy/Problem Observation
	// ("no known X"). SHALL-conformance on some C-CDA entry templates,
	// absent/false on most others — always safe to read regardless of template.
	NegationInd bool `json:"negationInd"`

	// TemplateIds holds every <templateId root="..."/> on this entry itself
	// (distinct from CDASection.TemplateIds, which is section-level).
	TemplateIds []string `json:"templateIds,omitempty"`

	// RepeatNumber is the substanceAdministration <repeatNumber value="..."/>
	// attribute — refill/allowed-administration count in INT mood, occurrence
	// number in EVN mood.
	RepeatNumber string `json:"repeatNumber,omitempty"`

	// EffectiveTimes holds every <effectiveTime> child in document order.
	// substanceAdministration entries commonly carry two: an IVL_TS for the
	// administration duration (also mirrored into EffectiveTime above) and a
	// PIVL_TS/EIVL_TS for the dosing frequency.
	EffectiveTimes []CDAEffectiveTimeEntry `json:"effectiveTimes,omitempty"`

	// Text holds this entry's own top-level <text> element (a sibling of
	// <code>, distinct from Code.OriginalText which lives inside
	// <code><originalText>). Used by templates that carry their content this
	// way instead of via a coded value — e.g. Medication Free Text Sig
	// (templateId 2.16.840.1.113883.10.20.22.4.147) and Instruction (V2)
	// (templateId 2.16.840.1.113883.10.20.22.4.20), both IG-required to use
	// <text><reference value="#id"/></text>. Resolved against the section's
	// narrative index by resolveEntryRefs, same as Code.OriginalText.
	Text string `json:"text,omitempty"`

	// Authors/Performers recorded directly on this entry (distinct from the
	// document-level header.Authors) — e.g. who ordered/recorded a medication.
	// Used to populate fields like MedicationRequest.requester.
	Authors    []CDAAuthor    `json:"authors,omitempty"`
	Performers []CDAPerformer `json:"performers,omitempty"`

	// Informants recorded directly on this entry: <informant><assignedEntity>,
	// the act's own source of information, distinct from Authors (who
	// recorded the act) and Performers (who carried it out). Real example:
	// a Note Activity (.4.202)'s <informant><assignedEntity>
	// <representedOrganization> names the facility the note was written at
	// — used for DocumentReference.custodian. Only the assignedEntity choice
	// is parsed (not the base CDA schema's relatedEntity alternative); no
	// real entry seen yet uses it.
	Informants []CDAInformant `json:"informants,omitempty"`

	// substanceAdministration-specific (shared by Medication Activity AND
	// Immunization Activity — both are <substanceAdministration>-based).
	Consumable   *CDAConsumable `json:"consumable,omitempty"`
	RouteCode    *CDACode       `json:"routeCode,omitempty"`
	DoseQuantity *CDAQuantity   `json:"doseQuantity,omitempty"`
	RateQuantity *CDAQuantity   `json:"rateQuantity,omitempty"`

	// ApproachSiteCode is <approachSiteCode> — the anatomical site of
	// administration (e.g. "Right Deltoid" for an injection). Real gap found
	// auditing multiple real CCDs: present with genuine data (not just
	// nullFlavor placeholders) on both medication and immunization entries.
	ApproachSiteCode *CDACode `json:"approachSiteCode,omitempty"`

	// AdministrationUnitCode is <administrationUnitCode> — the dosage
	// form/unit (e.g. "CAPSULE, DELAYED RELEASE"), distinct from
	// DoseQuantity's numeric amount+unit.
	AdministrationUnitCode *CDACode `json:"administrationUnitCode,omitempty"`

	// MaxDoseQuantity is <maxDoseQuantity> (RTO) — the maximum allowed dose
	// per time period. Denominator is very commonly nullFlavor="NI" in real
	// documents; the numerator (max single/daily dose amount) is usually
	// what's actually populated.
	MaxDoseQuantity *CDARatio `json:"maxDoseQuantity,omitempty"`

	// Precondition is <precondition><criterion><value> — the clinical
	// criterion that must be met before administering (PRN conditions).
	// Real-world evidence across several audited documents: usually
	// nullFlavor="NI" (criterion not actually specified), but parsed in case
	// a document populates it meaningfully.
	Precondition *CDACode `json:"precondition,omitempty"`

	// supply-specific: <supply><quantity>, <supply><product><manufacturedProduct>.
	// Distinct from substanceAdministration's Consumable/DoseQuantity above —
	// a supply act (e.g. medication dispense/refill, entryRelationship
	// typeCode="REFR") restates the product and records the dispensed quantity
	// rather than a dose.
	Quantity *CDAQuantity            `json:"quantity,omitempty"`
	Product  *CDAManufacturedProduct `json:"product,omitempty"`

	// TargetSiteCode is procedure-specific: <targetSiteCode> is a direct child
	// of the base CDA R2 <procedure> act class (POCD_MT000040.Procedure),
	// a sibling of <code>/<statusCode>/<effectiveTime> — NOT a nested
	// entryRelationship. Only populated when EntryType == "procedure"; the
	// "Procedure Activity Observation"/"Procedure Activity Act" C-CDA template
	// variants use <observation>/<act> elements, which have no targetSiteCode
	// child in the base schema, so this is always nil for those.
	TargetSiteCode *CDACode `json:"targetSiteCode,omitempty"`

	// RelatedSubjectCode is the Family History Organizer's
	// <subject><relatedSubject><code>: the family member's relationship to the
	// patient (e.g. "mother", "father" — RoleCode system
	// 2.16.840.1.113883.5.111). A sibling of Components (the actual family
	// history observations), not itself a component — only populated when
	// EntryType == "organizer" and a <subject> child is present.
	RelatedSubjectCode *CDACode `json:"relatedSubjectCode,omitempty"`
}

// CDAEffectiveTimeEntry is one <effectiveTime> element preserved in document
// order, tagged with its xsi:type so callers can distinguish the duration
// (IVL_TS) entry from a frequency (PIVL_TS/EIVL_TS) entry without guessing.
type CDAEffectiveTimeEntry struct {
	XSIType string       `json:"xsiType,omitempty"` // "IVL_TS" | "PIVL_TS" | "EIVL_TS" | "TS" | "" (absent)
	Range   CDATimeRange `json:"range"`
	// Period holds the <period value="12" unit="h"/> child of a PIVL_TS
	// element (the dosing-frequency interval) — not applicable to IVL_TS.
	Period *CDAQuantity `json:"period,omitempty"`
}

// CDAEntryRelationship wraps a clinical act nested inside another entry.
type CDAEntryRelationship struct {
	TypeCode     string   `json:"typeCode,omitempty"`
	InversionInd bool     `json:"inversionInd"`
	Entry        CDAEntry `json:"entry"`
}

// ========================
// CDA Primitive Types
// ========================

// CDAII is an HL7 II (Instance Identifier).
type CDAII struct {
	Root                   string `json:"root,omitempty"`
	Extension              string `json:"extension,omitempty"`
	AssigningAuthorityName string `json:"assigningAuthorityName,omitempty"`
	DisplayName            string `json:"displayName,omitempty"`
	NullFlavor             string `json:"nullFlavor,omitempty"`
}

// CDACode is an HL7 CD/CE/CS coded concept.
type CDACode struct {
	Code           string    `json:"code,omitempty"`
	DisplayName    string    `json:"displayName,omitempty"`
	CodeSystem     string    `json:"codeSystem,omitempty"`     // OID
	CodeSystemName string    `json:"codeSystemName,omitempty"` // "SNOMED CT", "LOINC", "RxNorm", etc.
	OriginalText   string    `json:"originalText,omitempty"`
	Translations   []CDACode `json:"translations,omitempty"` // alternate codings for the same concept
	NullFlavor     string    `json:"nullFlavor,omitempty"`
}

// CDATime is an HL7 TS (timestamp).
type CDATime struct {
	Value      string `json:"value,omitempty"`    // raw HL7 timestamp: "20230101120000"
	Operator   string `json:"operator,omitempty"` // "=", "<", ">", etc.
	NullFlavor string `json:"nullFlavor,omitempty"`
}

// CDATimeRange is an HL7 IVL<TS> (interval of timestamps).
type CDATimeRange struct {
	Low        CDATime `json:"low"`
	High       CDATime `json:"high"`
	Value      CDATime `json:"value"` // single-point time (when low/high not present)
	NullFlavor string  `json:"nullFlavor,omitempty"`
}

// CDAQuantity is an HL7 PQ (Physical Quantity).
type CDAQuantity struct {
	Value      string `json:"value,omitempty"`
	Unit       string `json:"unit,omitempty"`
	NullFlavor string `json:"nullFlavor,omitempty"`
}

// CDARatio is an HL7 RTO (ratio of two quantities) — substanceAdministration's
// <maxDoseQuantity> ("N units per M time period", e.g. "4 tablet per 1 d").
// Denominator is very commonly nullFlavor="NI" in real documents (the
// numerator alone — the max single/daily dose — is usually what's populated).
type CDARatio struct {
	Numerator   CDAQuantity `json:"numerator"`
	Denominator CDAQuantity `json:"denominator"`
}

// CDAName is an HL7 PN (Person Name).
type CDAName struct {
	Use       string       `json:"use,omitempty"`
	Prefix    string       `json:"prefix,omitempty"`
	Given     []string     `json:"given,omitempty"` // multiple given names / initials
	Family    string       `json:"family,omitempty"`
	Suffix    string       `json:"suffix,omitempty"`
	ValidTime CDATimeRange `json:"validTime"`
}

// CDAAddress is an HL7 AD (Postal Address).
type CDAAddress struct {
	Use         string       `json:"use,omitempty"`
	StreetLines []string     `json:"streetLines,omitempty"` // multiple streetAddressLine elements
	City        string       `json:"city,omitempty"`
	State       string       `json:"state,omitempty"`
	PostalCode  string       `json:"postalCode,omitempty"`
	Country     string       `json:"country,omitempty"`
	County      string       `json:"county,omitempty"`
	ValidTime   CDATimeRange `json:"validTime"`
	NullFlavor  string       `json:"nullFlavor,omitempty"`
}

// CDATelecom is an HL7 TEL (Telecommunication Address).
type CDATelecom struct {
	Use   string `json:"use,omitempty"`
	Value string `json:"value,omitempty"` // raw value: "tel:555-1234", "mailto:..."
	Type  string `json:"type,omitempty"`  // derived: "phone", "fax", "email", "url"
}

// CDAValue is a polymorphic CDA observation value.
// The Type field identifies which concrete HL7 data type applies.
type CDAValue struct {
	Type       string        `json:"type,omitempty"` // "PQ" | "CD" | "CE" | "CS" | "ST" | "BL" | "INT" | "REAL" | "TS" | "IVL_TS" | "ED"
	Code       *CDACode      `json:"code,omitempty"`
	Quantity   *CDAQuantity  `json:"quantity,omitempty"`
	Text       string        `json:"text,omitempty"`
	Boolean    *bool         `json:"boolean,omitempty"`
	Integer    *int          `json:"integer,omitempty"`
	Real       *float64      `json:"real,omitempty"`
	TimeRange  *CDATimeRange `json:"timeRange,omitempty"`
	NullFlavor string        `json:"nullFlavor,omitempty"`
}

// CDALanguage is a patient language communication preference.
type CDALanguage struct {
	Code             string  `json:"code,omitempty"`
	PreferenceInd    bool    `json:"preferenceInd"`
	ModeCode         CDACode `json:"modeCode"`
	ProficiencyLevel CDACode `json:"proficiencyLevel"`
}

// CDAAuthor represents a document or section author.
type CDAAuthor struct {
	Time           CDATime           `json:"time"`
	AssignedAuthor CDAAssignedAuthor `json:"assignedAuthor"`
}

// CDAAssignedAuthor contains details about the assigned author entity.
type CDAAssignedAuthor struct {
	Ids                     []CDAII          `json:"ids,omitempty"`
	Addresses               []CDAAddress     `json:"addresses,omitempty"`
	Telecoms                []CDATelecom     `json:"telecoms,omitempty"`
	AssignedPerson          *CDAPerson       `json:"assignedPerson,omitempty"`
	AssignedAuthoringDevice *CDADevice       `json:"assignedAuthoringDevice,omitempty"`
	RepresentedOrganization *CDAOrganization `json:"representedOrganization,omitempty"`
}

// CDAPerson is a named person (author, performer, etc.).
type CDAPerson struct {
	Names []CDAName `json:"names,omitempty"`
}

// CDADevice represents an authoring device.
type CDADevice struct {
	SoftwareName          string `json:"softwareName,omitempty"`
	ManufacturerModelName string `json:"manufacturerModelName,omitempty"`
}

// CDAOrganization is a healthcare organization.
type CDAOrganization struct {
	Ids       []CDAII      `json:"ids,omitempty"`
	Names     []string     `json:"names,omitempty"`
	Addresses []CDAAddress `json:"addresses,omitempty"`
	Telecoms  []CDATelecom `json:"telecoms,omitempty"`
}

// CDACustodian holds document custodian information.
type CDACustodian struct {
	AssignedCustodian CDAAssignedCustodian `json:"assignedCustodian"`
}

// CDAAssignedCustodian holds the custodian organization details.
type CDAAssignedCustodian struct {
	RepresentedCustodianOrganization CDAOrganization `json:"representedCustodianOrganization"`
}

// CDALegalAuthenticator holds the document's legal authentication
// (signature) participant, per base CDA's LegalAuthenticator class:
// time (1..1) and signatureCode (1..1) are required whenever the element
// itself is present; assignedEntity (1..1) reuses the same shape as every
// other CDA performer/author participant in this package.
type CDALegalAuthenticator struct {
	Time           CDATime           `json:"time"`
	SignatureCode  string            `json:"signatureCode,omitempty"`
	AssignedEntity CDAAssignedEntity `json:"assignedEntity"`
}

// CDAServiceEvent is the clinical service documented (from documentationOf).
type CDAServiceEvent struct {
	ClassCode     string         `json:"classCode,omitempty"`
	EffectiveTime CDATimeRange   `json:"effectiveTime"`
	Performers    []CDAPerformer `json:"performers,omitempty"`
}

// CDAPerformer is a clinical performer in a service event or entry.
type CDAPerformer struct {
	TypeCode       string            `json:"typeCode,omitempty"`
	FunctionCode   CDACode           `json:"functionCode"`
	Time           CDATimeRange      `json:"time"`
	AssignedEntity CDAAssignedEntity `json:"assignedEntity"`
}

// CDAInformant is an entry's <informant><assignedEntity> — the source of
// information for the act, distinct from CDAPerformer/CDAAuthor.
type CDAInformant struct {
	AssignedEntity CDAAssignedEntity `json:"assignedEntity"`
}

// CDAAssignedEntity is an entity assigned to perform a clinical role.
type CDAAssignedEntity struct {
	Ids                     []CDAII          `json:"ids,omitempty"`
	Code                    CDACode          `json:"code"`
	Addresses               []CDAAddress     `json:"addresses,omitempty"`
	Telecoms                []CDATelecom     `json:"telecoms,omitempty"`
	AssignedPerson          *CDAPerson       `json:"assignedPerson,omitempty"`
	RepresentedOrganization *CDAOrganization `json:"representedOrganization,omitempty"`
}

// CDAEncounter is the encompassing encounter from componentOf.
type CDAEncounter struct {
	Id                       CDAII        `json:"id"`
	EffectiveTime            CDATimeRange `json:"effectiveTime"`
	DischargeDispositionCode CDACode      `json:"dischargeDispositionCode"`
	Location                 *CDALocation `json:"location,omitempty"`
}

// CDALocation wraps a health care facility.
type CDALocation struct {
	HealthCareFacility CDAHealthCareFacility `json:"healthCareFacility"`
}

// CDAHealthCareFacility is the physical care location.
type CDAHealthCareFacility struct {
	Code                        CDACode          `json:"code"`
	Location                    *CDAPlace        `json:"location,omitempty"`
	ServiceProviderOrganization *CDAOrganization `json:"serviceProviderOrganization,omitempty"`
}

// CDAPlace is a physical place with name and address.
type CDAPlace struct {
	Names     []CDAName    `json:"names,omitempty"`
	Addresses []CDAAddress `json:"addresses,omitempty"`
}

// CDARelatedDocument links to a parent document (replacement, addendum, etc.).
type CDARelatedDocument struct {
	TypeCode       string            `json:"typeCode,omitempty"`
	ParentDocument CDAParentDocument `json:"parentDocument"`
}

// CDAParentDocument holds the related parent document reference.
type CDAParentDocument struct {
	Ids           []CDAII `json:"ids,omitempty"`
	SetId         CDAII   `json:"setId"`
	VersionNumber string  `json:"versionNumber,omitempty"`
}

// CDAParticipant is a clinical participant in an entry (e.g. the allergy substance).
type CDAParticipant struct {
	TypeCode        string             `json:"typeCode,omitempty"`
	FunctionCode    CDACode            `json:"functionCode"`
	Time            CDATimeRange       `json:"time"`
	ParticipantRole CDAParticipantRole `json:"participantRole"`
}

// CDAParticipantRole is the role played by a participant.
type CDAParticipantRole struct {
	ClassCode     string            `json:"classCode,omitempty"`
	Ids           []CDAII           `json:"ids,omitempty"`
	Code          CDACode           `json:"code"`
	Addresses     []CDAAddress      `json:"addresses,omitempty"`
	Telecoms      []CDATelecom      `json:"telecoms,omitempty"`
	PlayingEntity *CDAPlayingEntity `json:"playingEntity,omitempty"`
	ScopingEntity *CDAEntity        `json:"scopingEntity,omitempty"`
}

// CDAPlayingEntity is the entity playing the participant role (e.g. a drug or device).
type CDAPlayingEntity struct {
	ClassCode string    `json:"classCode,omitempty"`
	Code      CDACode   `json:"code"`
	Names     []CDAName `json:"names,omitempty"`
	Desc      string    `json:"desc,omitempty"`
}

// CDAEntity is a scoping entity (e.g. an organisation that issued an identifier).
type CDAEntity struct {
	Ids  []CDAII `json:"ids,omitempty"`
	Code CDACode `json:"code"`
	Desc string  `json:"desc,omitempty"`
}

// CDAConsumable holds the manufactured product for a medication administration.
type CDAConsumable struct {
	ManufacturedProduct CDAManufacturedProduct `json:"manufacturedProduct"`
}

// CDAManufacturedProduct is the manufactured medication.
type CDAManufacturedProduct struct {
	Ids                      []CDAII          `json:"ids,omitempty"`
	ManufacturedMaterial     *CDAMaterial     `json:"manufacturedMaterial,omitempty"`
	ManufacturerOrganization *CDAOrganization `json:"manufacturerOrganization,omitempty"`
}

// CDAMaterial is the drug or material substance.
type CDAMaterial struct {
	Code          CDACode `json:"code"`
	Name          string  `json:"name,omitempty"`
	LotNumberText string  `json:"lotNumberText,omitempty"`
}
