// services/mapping/type_registry.go
// HL7 v2 data type → FHIR R4 data type + transform key registry.
//
// This is the stable lookup table used by the schema-driven map generator
// at design time (wizard, AI suggest, OOB template build). It is NOT used
// in the runtime hot path — the transform key emitted here is stored in the
// position map (template JSON) and executed directly at runtime.
//
// Adding a new FHIR version: add a FHIRVersion constant and, if the target
// data type names changed, extend TypeEntry with a per-version override map.
// The registry itself stays the same; callers pass the version they need.
package mapping

import "strings"

// FHIRVersion identifies which FHIR specification version a generated
// mapping targets.
type FHIRVersion string

const (
	FHIRVersionR4 FHIRVersion = "R4"
	FHIRVersionR5 FHIRVersion = "R5"
)

// TypeEntry describes how an HL7 v2 data type maps to a FHIR data type and
// which transform function the runtime executor should call.
type TypeEntry struct {
	// FHIRType is the FHIR element data type name (e.g. "HumanName", "string").
	FHIRType string
	// TransformKey is the identifier used by the runtime value-transform
	// dispatcher (fhir/value_transformers.go).
	TransformKey string
	// ComponentSeparator indicates that the HL7 field value is composite and
	// individual components should be mapped to child FHIR elements rather
	// than the parent element directly.
	ComponentSeparator bool
	// Notes captures clinical or implementation notes useful in generated
	// mapping descriptions and AI suggest reasoning.
	Notes string
}

// typeRegistry is the authoritative HL7DataType → TypeEntry table.
// Keys are uppercase HL7 v2 primitive and composite data type codes.
var typeRegistry = map[string]TypeEntry{
	// ── Primitive string types ─────────────────────────────────────────────
	"ST": {FHIRType: "string", TransformKey: "string_direct",
		Notes: "String Data — free text, maps directly to FHIR string"},
	"FT": {FHIRType: "string", TransformKey: "string_direct",
		Notes: "Formatted Text — HTML/plain text, maps to string (HTML stripped at runtime if needed)"},
	"TX": {FHIRType: "string", TransformKey: "string_direct",
		Notes: "Text Data — multi-line text"},
	"GTS": {FHIRType: "string", TransformKey: "string_direct",
		Notes: "General Timing Specification — complex timing expression"},

	// ── Coded / identifier primitives ─────────────────────────────────────
	"ID": {FHIRType: "code", TransformKey: "string_direct",
		Notes: "Coded values defined by HL7 tables — maps to FHIR code"},
	"IS": {FHIRType: "code", TransformKey: "string_direct",
		Notes: "Coded value for user-defined tables — maps to FHIR code"},
	"NM": {FHIRType: "decimal", TransformKey: "string_direct",
		Notes: "Numeric — decimal precision preserved"},
	"SI": {FHIRType: "integer", TransformKey: "string_direct",
		Notes: "Sequence ID — integer position"},

	// ── Date / time primitives ─────────────────────────────────────────────
	"DT": {FHIRType: "date", TransformKey: "hl7_timestamp_to_fhir_date",
		Notes: "Date (YYYYMMDD) → FHIR date (YYYY-MM-DD)"},
	"TM": {FHIRType: "time", TransformKey: "string_direct",
		Notes: "Time (HHMMSS.SSSS[+/-ZZZZ]) → FHIR time"},
	"TS": {FHIRType: "dateTime", TransformKey: "hl7_timestamp_to_fhir_datetime",
		Notes: "Timestamp (YYYYMMDDHHMMSS[+/-ZZZZ]) → FHIR dateTime (ISO-8601)"},
	"DTM": {FHIRType: "dateTime", TransformKey: "hl7_timestamp_to_fhir_datetime",
		Notes: "Date/Time (HL7 v2.6+) — same format as TS"},

	// ── Composite: coded concepts ──────────────────────────────────────────
	"CE": {FHIRType: "CodeableConcept", TransformKey: "ce_to_codeableconcept",
		ComponentSeparator: true,
		Notes: "Coded Element (code^display^codeSystem) → CodeableConcept with one Coding"},
	"CWE": {FHIRType: "CodeableConcept", TransformKey: "ce_to_codeableconcept",
		ComponentSeparator: true,
		Notes: "Coded With Exceptions (v2.6+ replacement for CE) → CodeableConcept"},
	"CNE": {FHIRType: "CodeableConcept", TransformKey: "ce_to_codeableconcept",
		ComponentSeparator: true,
		Notes: "Coded No Exceptions — required binding, same shape as CWE"},
	"CF": {FHIRType: "CodeableConcept", TransformKey: "ce_to_codeableconcept",
		ComponentSeparator: true,
		Notes: "Coded Element With Formatted Values — maps to CodeableConcept"},
	"CSU": {FHIRType: "CodeableConcept", TransformKey: "ce_to_codeableconcept",
		ComponentSeparator: true,
		Notes: "Channel Sensitivity and Units — maps to CodeableConcept"},

	// ── Composite: identifiers ─────────────────────────────────────────────
	"CX": {FHIRType: "Identifier", TransformKey: "cx_to_identifier",
		ComponentSeparator: true,
		Notes: "Extended Composite ID With Check Digit → Identifier (value + system + type)"},
	"EI": {FHIRType: "Identifier", TransformKey: "cx_to_identifier",
		ComponentSeparator: true,
		Notes: "Entity Identifier (value^namespace^universal_id^type) → Identifier. CX-compatible structure: EI.1→value, EI.2→namespace (system), EI.3/EI.4→HD universal ID."},
	"HD": {FHIRType: "string", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Hierarchic Designator (namespace^universal_id^type) — usually mapped to identifier system or display"},
	"PLN": {FHIRType: "Identifier", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Practitioner License or Other ID Number → Identifier"},

	// ── Composite: person names ────────────────────────────────────────────
	"XPN": {FHIRType: "HumanName", TransformKey: "xpn_to_humanname",
		ComponentSeparator: true,
		Notes: "Extended Person Name (family^given^middle^suffix^prefix) → HumanName"},
	"FN": {FHIRType: "string", TransformKey: "name_component",
		ComponentSeparator: true,
		Notes: "Family Name composite (surname^own_surname_prefix...) → HumanName.family string"},

	// ── Composite: addresses ──────────────────────────────────────────────
	"XAD": {FHIRType: "Address", TransformKey: "xad_to_address",
		ComponentSeparator: true,
		Notes: "Extended Address (street^other^city^state^zip^country^type) → Address"},
	"SAD": {FHIRType: "string", TransformKey: "address_component",
		ComponentSeparator: true,
		Notes: "Street Address sub-type of XAD — maps to Address.line[0]"},

	// ── Composite: telecommunications ─────────────────────────────────────
	"XTN": {FHIRType: "ContactPoint", TransformKey: "xtn_to_contactpoint",
		ComponentSeparator: true,
		Notes: "Extended Telecommunication Number (use^tel_equip^email^country^area^local) → ContactPoint"},

	// ── Composite: person references ──────────────────────────────────────
	"XCN": {FHIRType: "Reference", TransformKey: "xcn_to_reference",
		ComponentSeparator: true,
		Notes: "Extended Composite ID Number And Name For Persons → Reference (display) or Identifier"},
	"XON": {FHIRType: "Reference", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Extended Composite Name And Identification Number For Organizations → Organization Reference"},

	// ── Composite: location ───────────────────────────────────────────────
	"PL": {FHIRType: "string", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Person Location (point_of_care^room^bed^facility) — maps to Location display or Encounter.location"},

	// ── Composite: quantities ─────────────────────────────────────────────
	"CQ": {FHIRType: "Quantity", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Composite Quantity With Units (quantity^units) → Quantity"},
	"SN": {FHIRType: "Quantity", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Structured Numeric (comparator^num1^separator^num2) → Quantity or Range"},
	"NR": {FHIRType: "Range", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Numeric Range (low^high) → Range"},
	"VR": {FHIRType: "string", TransformKey: "string_direct",
		Notes: "Value Range — maps to string representation"},

	// ── Composite: timing ────────────────────────────────────────────────
	"TQ": {FHIRType: "Timing", TransformKey: "tq_to_timing",
		ComponentSeparator: true,
		Notes: "Timing/Quantity (deprecated in v2.7) — maps to Timing or period start/end"},
	"RPT": {FHIRType: "Timing", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Repeat Pattern (v2.7+) → Timing.repeat"},

	// ── Composite: money ─────────────────────────────────────────────────
	"MO": {FHIRType: "Money", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Money (quantity^denomination) → Money"},
	"MOP": {FHIRType: "Money", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Money Or Percentage → Money"},

	// ── Composite: reference pointers ────────────────────────────────────
	"RP": {FHIRType: "Attachment", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Reference Pointer (ptr^application^type^subtype) → Attachment.url"},
	"ED": {FHIRType: "Attachment", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Encapsulated Data (source^type^encoding^data) → Attachment"},

	// ── Composite: specialty / practitioner ──────────────────────────────
	"SPD": {FHIRType: "string", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Specialty Description (specialty^governing_board^eligible^certified) → Qualification display"},
	"NDL": {FHIRType: "Reference", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Name With Date And Location → PractitionerRole Reference"},
	"DLN": {FHIRType: "Identifier", TransformKey: "string_direct",
		ComponentSeparator: true,
		Notes: "Driver's License Number → Identifier"},

	// ── Composite: address + name combos ─────────────────────────────────
	"XPN_ADDR": {FHIRType: "HumanName", TransformKey: "xpn_to_humanname",
		Notes: "Alias for XPN in some segment definitions"},

	// ── Misc primitives ───────────────────────────────────────────────────
	"varies": {FHIRType: "string", TransformKey: "string_direct",
		Notes: "Varies — OBX.5 type resolved from OBX.2 at runtime"},
	"any": {FHIRType: "string", TransformKey: "string_direct",
		Notes: "Any — catch-all for untyped fields"},
}

// Lookup returns the TypeEntry for an HL7 data type code.
// Returns a safe string/string_direct default when the type is unknown so
// callers always get a usable result.
func Lookup(hl7DataType string) TypeEntry {
	key := strings.ToUpper(strings.TrimSpace(hl7DataType))
	if entry, ok := typeRegistry[key]; ok {
		return entry
	}
	return TypeEntry{
		FHIRType:     "string",
		TransformKey: "string_direct",
		Notes:        "Unknown HL7 data type — defaulting to string",
	}
}

// LookupForVersion returns the TypeEntry for an HL7 data type targeting a
// specific FHIR version. Currently R4 and R5 share the same type mappings;
// this hook exists so R5-specific overrides can be added without changing
// callers.
func LookupForVersion(hl7DataType string, _ FHIRVersion) TypeEntry {
	return Lookup(hl7DataType)
}

// IsComposite reports whether the HL7 data type is composite (i.e. contains
// sub-components separated by '^'). Composite fields may need their components
// mapped individually to child FHIR elements.
func IsComposite(hl7DataType string) bool {
	return Lookup(hl7DataType).ComponentSeparator
}
