// Package cdadocument defines the typed Go struct hierarchy for a complete CDA document.
// It is the CDA equivalent of the HL7 EnhancedSegments model — every field is represented
// as a Go type; nothing is silently dropped. Produced by CDAParser.ParseDocument().
package cdadocument

// CDADocument is the root typed representation of a parsed CDA/CCD document.
type CDADocument struct {
	Header        CDAHeader
	Sections      []CDASection              // ordered as they appear in the document
	SectionsByKey map[string]*CDASection    // keyed by section key from ccda_2_1.json
	Raw           string                    // original XML preserved verbatim
}

// CDAHeader contains all clinical document header elements.
type CDAHeader struct {
	DocumentId            CDAII
	DocumentCode          CDACode
	Title                 string
	SetId                 CDAII
	VersionNumber         string
	EffectiveTime         CDATime
	ConfidentialityCode   CDACode
	Patient               CDAPatient
	Authors               []CDAAuthor
	Custodian             CDACustodian
	DocumentOf            *CDAServiceEvent
	EncompassingEncounter *CDAEncounter
	RelatedDocuments      []CDARelatedDocument
}

// CDAPatient holds all patient demographics extracted from recordTarget/patientRole.
type CDAPatient struct {
	Ids           []CDAII      // ALL <id> elements — SSN, MRN, Medicare, etc.
	Names         []CDAName    // ALL <name> elements — legal, maiden, alias
	Addresses     []CDAAddress // ALL <addr> elements — home, work, etc.
	Telecoms      []CDATelecom // ALL <telecom> elements — phone, fax, email
	BirthDate     CDATime
	Gender        CDACode
	MaritalStatus CDACode
	Race          CDACode
	Ethnicity     CDACode
	Languages     []CDALanguage
}

// CDASection represents one clinical section of a CDA document body.
type CDASection struct {
	TemplateIds   []string
	Key           string // resolved from templateId via ccda_2_1.json, then LOINC, then title
	LoincCode     string
	Title         string
	NarrativeText string // serialised <text> element XML
	Entries       []CDAEntry
	NullFlavor    string // "NI", "NA", etc. if section present but empty
}

// CDAEntry represents one clinical act extracted from a CDA section <entry> element.
type CDAEntry struct {
	Id                 []CDAII
	Code               CDACode
	StatusCode         string
	EffectiveTime      CDATimeRange
	Value              *CDAValue
	Participants       []CDAParticipant
	Components         []CDAEntry // nested organizer/component entries
	EntryRelationships []CDAEntryRelationship
	EntryType          string // "observation" | "act" | "procedure" | "substanceAdministration" | "encounter" | "organizer" | "supply"
	ClassCode          string
	MoodCode           string
	NullFlavor         string

	// Authors/Performers recorded directly on this entry (distinct from the
	// document-level header.Authors) — e.g. who ordered/recorded a medication.
	// Used to populate fields like MedicationRequest.requester.
	Authors    []CDAAuthor
	Performers []CDAPerformer

	// substanceAdministration-specific
	Consumable   *CDAConsumable
	RouteCode    *CDACode
	DoseQuantity *CDAQuantity
	RateQuantity *CDAQuantity
}

// CDAEntryRelationship wraps a clinical act nested inside another entry.
type CDAEntryRelationship struct {
	TypeCode     string
	InversionInd bool
	Entry        CDAEntry
}

// ========================
// CDA Primitive Types
// ========================

// CDAII is an HL7 II (Instance Identifier).
type CDAII struct {
	Root                   string
	Extension              string
	AssigningAuthorityName string
	DisplayName            string
}

// CDACode is an HL7 CD/CE/CS coded concept.
type CDACode struct {
	Code           string
	DisplayName    string
	CodeSystem     string    // OID
	CodeSystemName string    // "SNOMED CT", "LOINC", "RxNorm", etc.
	OriginalText   string
	Translations   []CDACode // alternate codings for the same concept
	NullFlavor     string
}

// CDATime is an HL7 TS (timestamp).
type CDATime struct {
	Value      string // raw HL7 timestamp: "20230101120000"
	Operator   string // "=", "<", ">", etc.
	NullFlavor string
}

// CDATimeRange is an HL7 IVL<TS> (interval of timestamps).
type CDATimeRange struct {
	Low        CDATime
	High       CDATime
	Value      CDATime // single-point time (when low/high not present)
	NullFlavor string
}

// CDAQuantity is an HL7 PQ (Physical Quantity).
type CDAQuantity struct {
	Value string
	Unit  string
}

// CDAName is an HL7 PN (Person Name).
type CDAName struct {
	Use       string   // "L" (legal), "A" (alias), "P" (pseudonym)
	Prefix    string
	Given     []string // multiple given names / initials
	Family    string
	Suffix    string
	ValidTime CDATimeRange
}

// CDAAddress is an HL7 AD (Postal Address).
type CDAAddress struct {
	Use         string   // "HP" (home permanent), "WP" (work), "TMP" (temp)
	StreetLines []string // multiple streetAddressLine elements
	City        string
	State       string
	PostalCode  string
	Country     string
	County      string
	ValidTime   CDATimeRange
	NullFlavor  string
}

// CDATelecom is an HL7 TEL (Telecommunication Address).
type CDATelecom struct {
	Use   string // "HP", "WP", "MC" (mobile)
	Value string // raw value: "tel:555-1234", "mailto:..."
	Type  string // derived: "phone", "fax", "email", "url"
}

// CDAValue is a polymorphic CDA observation value.
// The Type field identifies which concrete HL7 data type applies.
type CDAValue struct {
	Type       string // "PQ" | "CD" | "CE" | "CS" | "ST" | "BL" | "INT" | "REAL" | "TS" | "IVL_TS" | "ED"
	Code       *CDACode
	Quantity   *CDAQuantity
	Text       string
	Boolean    *bool
	Integer    *int
	Real       *float64
	TimeRange  *CDATimeRange
	NullFlavor string
}

// CDALanguage is a patient language communication preference.
type CDALanguage struct {
	Code             string
	PreferenceInd    bool
	ModeCode         CDACode
	ProficiencyLevel CDACode
}

// CDAAuthor represents a document or section author.
type CDAAuthor struct {
	Time           CDATime
	AssignedAuthor CDAAssignedAuthor
}

// CDAAssignedAuthor contains details about the assigned author entity.
type CDAAssignedAuthor struct {
	Ids                     []CDAII
	Addresses               []CDAAddress
	Telecoms                []CDATelecom
	AssignedPerson          *CDAPerson
	AssignedAuthoringDevice *CDADevice
	RepresentedOrganization *CDAOrganization
}

// CDAPerson is a named person (author, performer, etc.).
type CDAPerson struct {
	Names []CDAName
}

// CDADevice represents an authoring device.
type CDADevice struct {
	SoftwareName          string
	ManufacturerModelName string
}

// CDAOrganization is a healthcare organization.
type CDAOrganization struct {
	Ids       []CDAII
	Names     []string
	Addresses []CDAAddress
	Telecoms  []CDATelecom
}

// CDACustodian holds document custodian information.
type CDACustodian struct {
	AssignedCustodian CDAAssignedCustodian
}

// CDAAssignedCustodian holds the custodian organization details.
type CDAAssignedCustodian struct {
	RepresentedCustodianOrganization CDAOrganization
}

// CDAServiceEvent is the clinical service documented (from documentationOf).
type CDAServiceEvent struct {
	ClassCode     string
	EffectiveTime CDATimeRange
	Performers    []CDAPerformer
}

// CDAPerformer is a clinical performer in a service event or entry.
type CDAPerformer struct {
	TypeCode       string
	FunctionCode   CDACode
	Time           CDATimeRange
	AssignedEntity CDAAssignedEntity
}

// CDAAssignedEntity is an entity assigned to perform a clinical role.
type CDAAssignedEntity struct {
	Ids                     []CDAII
	Code                    CDACode
	Addresses               []CDAAddress
	Telecoms                []CDATelecom
	AssignedPerson          *CDAPerson
	RepresentedOrganization *CDAOrganization
}

// CDAEncounter is the encompassing encounter from componentOf.
type CDAEncounter struct {
	Id                       CDAII
	EffectiveTime            CDATimeRange
	DischargeDispositionCode CDACode
	Location                 *CDALocation
}

// CDALocation wraps a health care facility.
type CDALocation struct {
	HealthCareFacility CDAHealthCareFacility
}

// CDAHealthCareFacility is the physical care location.
type CDAHealthCareFacility struct {
	Code                        CDACode
	Location                    *CDAPlace
	ServiceProviderOrganization *CDAOrganization
}

// CDAPlace is a physical place with name and address.
type CDAPlace struct {
	Names     []CDAName
	Addresses []CDAAddress
}

// CDARelatedDocument links to a parent document (replacement, addendum, etc.).
type CDARelatedDocument struct {
	TypeCode       string
	ParentDocument CDAParentDocument
}

// CDAParentDocument holds the related parent document reference.
type CDAParentDocument struct {
	Ids           []CDAII
	SetId         CDAII
	VersionNumber string
}

// CDAParticipant is a clinical participant in an entry (e.g. the allergy substance).
type CDAParticipant struct {
	TypeCode        string
	FunctionCode    CDACode
	Time            CDATimeRange
	ParticipantRole CDAParticipantRole
}

// CDAParticipantRole is the role played by a participant.
type CDAParticipantRole struct {
	ClassCode     string
	Ids           []CDAII
	Code          CDACode
	Addresses     []CDAAddress
	Telecoms      []CDATelecom
	PlayingEntity *CDAPlayingEntity
	ScopingEntity *CDAEntity
}

// CDAPlayingEntity is the entity playing the participant role (e.g. a drug or device).
type CDAPlayingEntity struct {
	ClassCode string
	Code      CDACode
	Names     []CDAName
	Desc      string
}

// CDAEntity is a scoping entity (e.g. an organisation that issued an identifier).
type CDAEntity struct {
	Ids  []CDAII
	Code CDACode
	Desc string
}

// CDAConsumable holds the manufactured product for a medication administration.
type CDAConsumable struct {
	ManufacturedProduct CDAManufacturedProduct
}

// CDAManufacturedProduct is the manufactured medication.
type CDAManufacturedProduct struct {
	Ids                      []CDAII
	ManufacturedMaterial     *CDAMaterial
	ManufacturerOrganization *CDAOrganization
}

// CDAMaterial is the drug or material substance.
type CDAMaterial struct {
	Code          CDACode
	Name          string
	LotNumberText string
}
