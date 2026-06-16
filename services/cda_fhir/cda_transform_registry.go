// services/cda_fhir/cda_transform_registry.go
// CDATransformRegistry — all named CDA→FHIR transform functions and type-pair inference.
//
// Mirrors the HL7 transform registry: each transform is registered by name, looked up
// at runtime, and can be inferred from (cdaDataType, fhirDataType) when the template
// mapping omits an explicit transform.
//
// Startup validation: call ValidateOOBTemplates() after loading the DB template to catch
// authoring errors before any messages arrive.

package cdafhir

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// =========================================================
// Transform function type
// =========================================================

// CDATransformFn processes one CDA field value and returns a FHIR-typed value.
//
//	entry    — the full CDA entry map produced by GenericSectionProcessor.
//	           Supplementary sub-values follow conventions:
//	             fieldKey+"Display" → display name  (from xpathDisplay)
//	             fieldKey+"System"  → OID/URI       (from xpathSystem)
//	             fieldKey+"Unit"    → PQ unit        (from xpathUnit)
//	             fieldKey+"Family"  → PN family name (from xpathFamily)
//	fieldKey — the cdaField key within entry.
//	vm       — valueMap from the template mapping; nil when absent.
//	Returns nil, nil for empty/absent values (field is skipped, not an error).
type CDATransformFn func(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error)

// =========================================================
// Type-pair defaults
// =========================================================

// TypePairDefault maps a (CDA data type, FHIR data type) pair to its default transform.
type TypePairDefault struct {
	CDADataType      string
	FHIRDataType     string
	DefaultTransform string
}

// typePairDefaults is the type-pair inference table from the plan (Part 2B).
var typePairDefaults = []TypePairDefault{
	// Composite CDA → Complex FHIR (Level 1)
	{"CD", "CodeableConcept", "cda_code_to_codeable_concept"},
	{"CE", "CodeableConcept", "cda_code_to_codeable_concept"},
	{"CD", "code", "cda_cd_to_code"},
	{"CE", "code", "cda_cd_to_code"},
	{"PQ", "Quantity", "cda_quantity_to_fhir"},
	{"IVL_TS", "Period", "cda_ivl_ts_to_period"},
	{"IVL_PQ", "Range", "cda_range_to_fhir"},
	{"PN", "HumanName", "cda_name_to_fhir"},
	{"AD", "Address", "cda_address_to_fhir"},
	{"TEL", "ContactPoint", "cda_telecom_to_fhir"},
	{"ED", "Attachment", "cda_ed_to_attachment"},
	{"RTO", "Ratio", "cda_ratio_to_fhir"},
	// Primitive CDA → Primitive FHIR (Level 0)
	{"TS", "dateTime", "cda_timestamp_to_fhir_datetime"},
	{"TS", "date", "cda_timestamp_to_fhir_date"},
	{"TS", "instant", "cda_timestamp_to_fhir_instant"},
	{"ST", "string", "string_direct"},
	{"CS", "code", "string_direct"},
	{"BL", "boolean", "boolean_direct"},
	{"UID", "uri", "oid_to_fhir_system_uri"},
	{"INT", "integer", "string_direct"},
	{"REAL", "decimal", "string_direct"},
}

// =========================================================
// Registry
// =========================================================

// CDATransformRegistry holds all named transform functions and the type-pair index.
// One instance covers all CDA→FHIR transforms; safe for concurrent use.
type CDATransformRegistry struct {
	transforms  map[string]CDATransformFn
	typePairIdx map[string]string // "CDAType|FHIRType" → transformName
}

// NewCDATransformRegistry creates and populates the registry with all built-in transforms.
func NewCDATransformRegistry() *CDATransformRegistry {
	r := &CDATransformRegistry{
		transforms:  make(map[string]CDATransformFn),
		typePairIdx: make(map[string]string),
	}
	r.registerBuiltins()
	r.buildTypePairIndex()
	return r
}

// ApplyTransform runs the named transform function on the given entry and fieldKey.
// Returns nil, nil when the field is absent/empty (caller should skip the FHIR path).
func (r *CDATransformRegistry) ApplyTransform(
	name string,
	entry map[string]interface{},
	fieldKey string,
	vm map[string]string,
) (interface{}, error) {
	fn, ok := r.transforms[name]
	if !ok {
		return nil, fmt.Errorf("cda_transform: unknown transform %q", name)
	}
	return fn(entry, fieldKey, vm)
}

// InferTransform looks up the default transform for a (cdaDataType, fhirDataType) pair.
// Returns an error when no default exists — the template must supply an explicit transform.
func (r *CDATransformRegistry) InferTransform(cdaDataType, fhirDataType string) (string, error) {
	key := cdaDataType + "|" + fhirDataType
	if t, ok := r.typePairIdx[key]; ok {
		return t, nil
	}
	return "", fmt.Errorf("cda_transform: no default for %s→%s; specify transform explicitly", cdaDataType, fhirDataType)
}

// HasTransform returns whether a transform name is registered.
func (r *CDATransformRegistry) HasTransform(name string) bool {
	_, ok := r.transforms[name]
	return ok
}

func (r *CDATransformRegistry) register(name string, fn CDATransformFn) {
	r.transforms[name] = fn
}

func (r *CDATransformRegistry) buildTypePairIndex() {
	for _, tp := range typePairDefaults {
		r.typePairIdx[tp.CDADataType+"|"+tp.FHIRDataType] = tp.DefaultTransform
	}
}

// =========================================================
// Built-in transform registration
// =========================================================

func (r *CDATransformRegistry) registerBuiltins() {
	// Primitives
	r.register("string_direct", transformStringDirect)
	r.register("boolean_direct", transformBooleanDirect)

	// Timestamps
	r.register("cda_timestamp_to_fhir_date", transformTimestampToDate)
	r.register("cda_timestamp_to_fhir_datetime", transformTimestampToDatetime)
	r.register("cda_timestamp_to_fhir_instant", transformTimestampToInstant)
	r.register("cda_timestamp_to_fhir_period", transformTimestampToPeriod)
	r.register("cda_ivl_ts_to_period", transformTimestampToPeriod) // alias: IVL_TS → Period

	// Code systems
	r.register("oid_to_fhir_system_uri", transformOIDToSystemURI)

	// Demographics
	r.register("cda_gender_to_fhir", transformGenderToFHIR)
	r.register("cda_name_to_fhir", transformNameToFHIR)
	r.register("cda_address_to_fhir", transformAddressToFHIR)
	r.register("cda_telecom_to_fhir", transformTelecomToFHIR)

	// Coded types
	r.register("cda_code_to_codeable_concept", transformCodeToCodeableConcept)
	r.register("cda_cd_to_code", transformCDToCode)

	// Clinical status transforms (use valueMap; fall back to built-in defaults)
	r.register("cda_allergy_severity_to_fhir", transformWithValueMapFallback(allergyDefaultSeverity))
	r.register("cda_allergy_status_to_fhir", transformWithValueMapFallback(allergyDefaultStatus))
	r.register("cda_problem_status_to_fhir", transformWithValueMapFallback(problemDefaultStatus))
	r.register("cda_result_status_to_fhir", transformWithValueMapFallback(resultDefaultStatus))
	r.register("cda_immunization_status_to_fhir", transformWithValueMapFallback(immunizationDefaultStatus))
	r.register("cda_encounter_class_to_fhir", transformEncounterClassToFHIR)
	r.register("cda_medication_status_to_fhir", transformWithValueMapFallback(medicationDefaultStatus))
	r.register("cda_procedure_status_to_fhir", transformWithValueMapFallback(procedureDefaultStatus))

	// Quantities and ranges
	r.register("cda_quantity_to_fhir", transformQuantityToFHIR)
	r.register("cda_range_to_fhir", transformRangeToFHIR)
	r.register("cda_reference_range_to_fhir", transformReferenceRangeToFHIR)

	// Less common composite types
	r.register("cda_ed_to_attachment", transformEDToAttachment)
	r.register("cda_ratio_to_fhir", transformRatioToFHIR)

	// Deprecated: CDA section HTML narrative is discarded in favour of generated narrative
	r.register("cda_narrative_to_text_div", transformNarrativeToTextDiv)
}

// =========================================================
// Default value maps for status transforms
// =========================================================

var allergyDefaultSeverity = map[string]string{
	"371924009": "mild",
	"6736007":   "moderate",
	"24484000":  "severe",
	// SNOMED display name variants
	"Mild":     "mild",
	"Moderate": "moderate",
	"Severe":   "severe",
}

var allergyDefaultStatus = map[string]string{
	"55561003":  "active",
	"73425007":  "inactive",
	"413322009": "resolved",
}

var problemDefaultStatus = map[string]string{
	"55561003":  "active",
	"73425007":  "inactive",
	"413322009": "resolved",
	"active":    "active",
	"inactive":  "inactive",
	"resolved":  "resolved",
}

var resultDefaultStatus = map[string]string{
	"completed": "final",
	"active":    "preliminary",
	"aborted":   "cancelled",
	"cancelled": "cancelled",
	"held":      "registered",
}

var immunizationDefaultStatus = map[string]string{
	"completed": "completed",
	"active":    "in-progress",
	"aborted":   "not-done",
	"cancelled": "not-done",
}

var medicationDefaultStatus = map[string]string{
	"active":    "active",
	"completed": "completed",
	"aborted":   "stopped",
	"cancelled": "stopped",
	"suspended": "on-hold",
}

var procedureDefaultStatus = map[string]string{
	"completed": "completed",
	"active":    "in-progress",
	"aborted":   "stopped",
	"cancelled": "not-done",
}

// =========================================================
// Transform implementations
// =========================================================

func transformStringDirect(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	val := strEntry(entry, fieldKey)
	if val == "" {
		return nil, nil
	}
	if vm != nil {
		if mapped, ok := vm[val]; ok {
			return mapped, nil
		}
	}
	return val, nil
}

func transformBooleanDirect(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	val := strings.ToLower(strEntry(entry, fieldKey))
	if val == "" {
		return nil, nil
	}
	return val == "true" || val == "1" || val == "yes", nil
}

func transformTimestampToDate(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	ts := strEntry(entry, fieldKey)
	if ts == "" {
		return nil, nil
	}
	date := cdaTimestampToDate(ts)
	if date == "" {
		return nil, nil
	}
	return date, nil
}

func transformTimestampToDatetime(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	ts := strEntry(entry, fieldKey)
	if ts == "" {
		return nil, nil
	}
	dt := FormatDate(ts) // FormatDate in code_mapper.go handles datetime
	if dt == "" {
		return nil, nil
	}
	return dt, nil
}

func transformTimestampToInstant(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	ts := strEntry(entry, fieldKey)
	if ts == "" {
		return nil, nil
	}
	// instant requires a timezone — append Z if missing
	dt := FormatDate(ts)
	if dt == "" {
		return nil, nil
	}
	if !strings.Contains(dt, "T") {
		// date-only → cannot form a valid instant; add midnight UTC
		dt += "T00:00:00Z"
	} else if !strings.ContainsAny(dt[len(dt)-6:], "+-Z") {
		dt += "Z"
	}
	return dt, nil
}

// transformTimestampToPeriod builds a FHIR Period from the primary field (start)
// and the Display variant of the field (end), following the XPath convention where
// xpathDisplay carries the IVL_TS high/@value.
func transformTimestampToPeriod(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	start := FormatDate(strEntry(entry, fieldKey))
	end := FormatDate(strEntry(entry, fieldKey+"Display"))
	if start == "" && end == "" {
		return nil, nil
	}
	period := map[string]interface{}{}
	if start != "" {
		period["start"] = start
	}
	if end != "" {
		period["end"] = end
	}
	return period, nil
}

func transformOIDToSystemURI(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	oid := strEntry(entry, fieldKey)
	if oid == "" {
		return nil, nil
	}
	return NormalizeSystem(oid), nil
}

func transformGenderToFHIR(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	code := strEntry(entry, fieldKey)
	if code == "" {
		return nil, nil
	}
	if vm != nil {
		if mapped, ok := vm[code]; ok {
			return mapped, nil
		}
	}
	switch strings.ToUpper(code) {
	case "M", "MALE":
		return "male", nil
	case "F", "FEMALE":
		return "female", nil
	case "UN", "UNK", "UNKNOWN":
		return "unknown", nil
	case "O", "OTHER":
		return "other", nil
	default:
		return "unknown", nil
	}
}

// transformWithValueMapFallback returns a CDATransformFn that first checks the
// template-supplied valueMap, then falls back to the built-in defaults map.
func transformWithValueMapFallback(defaults map[string]string) CDATransformFn {
	return func(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
		code := strEntry(entry, fieldKey)
		if code == "" {
			return nil, nil
		}
		if vm != nil {
			if mapped, ok := vm[code]; ok {
				return mapped, nil
			}
		}
		if mapped, ok := defaults[code]; ok {
			return mapped, nil
		}
		return code, nil // passthrough when no mapping found
	}
}

func transformEncounterClassToFHIR(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	code := strEntry(entry, fieldKey)
	display := strEntry(entry, fieldKey+"Display")
	if code == "" {
		return nil, nil
	}
	if vm != nil {
		if mapped, ok := vm[code]; ok {
			return NewCoding(mapped, mapped, "http://terminology.hl7.org/CodeSystem/v3-ActCode"), nil
		}
	}
	// Default CDA act class codes
	fhirCode := code
	switch strings.ToUpper(code) {
	case "AMB", "AMBULATORY":
		fhirCode = "AMB"
	case "IMP", "INPATIENT":
		fhirCode = "IMP"
	case "EMER", "EMERGENCY":
		fhirCode = "EMER"
	case "OBSENC", "OBSERVATION":
		fhirCode = "OBSENC"
	case "VR", "VIRTUAL":
		fhirCode = "VR"
	case "HH", "HOMEHEALTHCARE":
		fhirCode = "HH"
	}
	return NewCoding(fhirCode, display, "http://terminology.hl7.org/CodeSystem/v3-ActCode"), nil
}

func transformNameToFHIR(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	given := strEntry(entry, fieldKey)
	family := strEntry(entry, fieldKey+"Family")
	display := strEntry(entry, fieldKey+"Display")

	if given == "" && family == "" && display == "" {
		return nil, nil
	}

	name := map[string]interface{}{}
	if family != "" {
		name["family"] = family
	}
	if given != "" {
		name["given"] = []interface{}{given}
	}
	if display != "" {
		name["text"] = display
	}
	return name, nil
}

func transformAddressToFHIR(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	// Convention: primary xpath → street, Display → city, System → state, Unit → postalCode, Family → country
	street := strEntry(entry, fieldKey)
	city := strEntry(entry, fieldKey+"Display")
	state := strEntry(entry, fieldKey+"System")
	postal := strEntry(entry, fieldKey+"Unit")
	country := strEntry(entry, fieldKey+"Family")

	if street == "" && city == "" && state == "" && postal == "" {
		return nil, nil
	}

	addr := map[string]interface{}{"use": "work"}
	if street != "" {
		addr["line"] = []interface{}{street}
	}
	if city != "" {
		addr["city"] = city
	}
	if state != "" {
		addr["state"] = state
	}
	if postal != "" {
		addr["postalCode"] = postal
	}
	if country != "" {
		addr["country"] = country
	}
	return addr, nil
}

func transformTelecomToFHIR(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	raw := strEntry(entry, fieldKey)
	if raw == "" {
		return nil, nil
	}
	// CDA tel: URI format: "tel:+15551234567" or "mailto:user@example.com"
	contact := map[string]interface{}{}
	switch {
	case strings.HasPrefix(raw, "tel:"):
		contact["system"] = "phone"
		contact["value"] = strings.TrimPrefix(raw, "tel:")
	case strings.HasPrefix(raw, "mailto:"):
		contact["system"] = "email"
		contact["value"] = strings.TrimPrefix(raw, "mailto:")
	case strings.HasPrefix(raw, "fax:"):
		contact["system"] = "fax"
		contact["value"] = strings.TrimPrefix(raw, "fax:")
	default:
		contact["value"] = raw
	}
	if use := strEntry(entry, fieldKey+"System"); use != "" {
		switch strings.ToUpper(use) {
		case "HP":
			contact["use"] = "home"
		case "WP":
			contact["use"] = "work"
		case "MC":
			contact["use"] = "mobile"
		}
	}
	return contact, nil
}

func transformCodeToCodeableConcept(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	code := strEntry(entry, fieldKey)
	display := strEntry(entry, fieldKey+"Display")
	system := strEntry(entry, fieldKey+"System")

	if code == "" && display == "" {
		return nil, nil
	}

	coding := map[string]interface{}{}
	if code != "" {
		coding["code"] = code
	}
	if display != "" {
		coding["display"] = display
	}
	if system != "" {
		coding["system"] = NormalizeSystem(system)
	}

	cc := map[string]interface{}{
		"coding": []interface{}{coding},
	}
	if display != "" {
		cc["text"] = display
	}
	return cc, nil
}

func transformCDToCode(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	code := strEntry(entry, fieldKey)
	if code == "" {
		return nil, nil
	}
	if vm != nil {
		if mapped, ok := vm[code]; ok {
			return mapped, nil
		}
	}
	return code, nil
}

func transformQuantityToFHIR(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	valStr := strEntry(entry, fieldKey)
	unit := strEntry(entry, fieldKey+"Unit")
	if valStr == "" {
		return nil, nil
	}
	qty := map[string]interface{}{}
	if f, err := strconv.ParseFloat(valStr, 64); err == nil {
		qty["value"] = f
	} else {
		qty["value"] = valStr
	}
	if unit != "" {
		qty["unit"] = unit
		qty["system"] = "http://unitsofmeasure.org"
		qty["code"] = unit
	}
	return qty, nil
}

func transformRangeToFHIR(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	lowStr := strEntry(entry, fieldKey)
	highStr := strEntry(entry, fieldKey+"Display")
	lowUnit := strEntry(entry, fieldKey+"Unit")

	if lowStr == "" && highStr == "" {
		return nil, nil
	}

	rng := map[string]interface{}{}
	if lowStr != "" {
		low := map[string]interface{}{}
		if f, err := strconv.ParseFloat(lowStr, 64); err == nil {
			low["value"] = f
		} else {
			low["value"] = lowStr
		}
		if lowUnit != "" {
			low["unit"] = lowUnit
		}
		rng["low"] = low
	}
	if highStr != "" {
		high := map[string]interface{}{}
		if f, err := strconv.ParseFloat(highStr, 64); err == nil {
			high["value"] = f
		} else {
			high["value"] = highStr
		}
		if lowUnit != "" {
			high["unit"] = lowUnit
		}
		rng["high"] = high
	}
	return rng, nil
}

func transformReferenceRangeToFHIR(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	rng, err := transformRangeToFHIR(entry, fieldKey, vm)
	if err != nil || rng == nil {
		return rng, err
	}
	// Wrap in referenceRange structure expected by Observation
	return map[string]interface{}{"low": rng.(map[string]interface{})["low"], "high": rng.(map[string]interface{})["high"]}, nil
}

func transformEDToAttachment(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	content := strEntry(entry, fieldKey)
	if content == "" {
		return nil, nil
	}
	att := map[string]interface{}{"data": content}
	if mediaType := strEntry(entry, fieldKey+"System"); mediaType != "" {
		att["contentType"] = mediaType
	}
	return att, nil
}

func transformRatioToFHIR(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	num := strEntry(entry, fieldKey)
	den := strEntry(entry, fieldKey+"Display")
	if num == "" && den == "" {
		return nil, nil
	}
	ratio := map[string]interface{}{}
	if num != "" {
		if f, err := strconv.ParseFloat(num, 64); err == nil {
			ratio["numerator"] = map[string]interface{}{"value": f}
		}
	}
	if den != "" {
		if f, err := strconv.ParseFloat(den, 64); err == nil {
			ratio["denominator"] = map[string]interface{}{"value": f}
		}
	}
	return ratio, nil
}

// transformNarrativeToTextDiv is deprecated: CDA section narrative HTML is discarded.
// FHIR narrative is generated from mapped FHIR resources by FHIRNarrativeGenerator.
func transformNarrativeToTextDiv(entry map[string]interface{}, fieldKey string, vm map[string]string) (interface{}, error) {
	return nil, nil // intentional no-op
}

// =========================================================
// Startup validation
// =========================================================

// ValidateOOBMappings checks every mapping in the flat list: if transform is empty,
// it tries to infer one from the type pair. Returns a list of warnings for any
// mapping that cannot be resolved, so authoring mistakes are caught at startup.
func (r *CDATransformRegistry) ValidateOOBMappings(mappings []CDAFieldMapping) []string {
	var warnings []string
	for _, m := range mappings {
		name := m.Transform
		if name == "" {
			var err error
			name, err = r.InferTransform(m.CDADataType, m.FHIRDataType)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf(
					"[%s.%s] no transform specified and cannot infer from (%s→%s): %v",
					m.SectionKey, m.CDAField, m.CDADataType, m.FHIRDataType, err))
				continue
			}
		}
		if !r.HasTransform(name) {
			warnings = append(warnings, fmt.Sprintf(
				"[%s.%s] unknown transform %q", m.SectionKey, m.CDAField, name))
		}
	}
	return warnings
}

// =========================================================
// Helpers (also used by transform functions)
// =========================================================

// strEntry returns the string value at key in the entry map, or "".
func strEntry(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// cdaTimestampToDate converts a CDA TS value to a FHIR date string (YYYY-MM-DD).
// Input may be any length CDA timestamp; only date portion is returned.
func cdaTimestampToDate(ts string) string {
	s := strings.TrimSpace(ts)
	if len(s) < 8 {
		return ""
	}
	// Strip timezone
	for _, sep := range []string{"+", "-"} {
		if idx := strings.LastIndex(s, sep); idx >= 8 {
			s = s[:idx]
			break
		}
	}
	if len(s) >= 8 {
		return fmt.Sprintf("%s-%s-%s", s[0:4], s[4:6], s[6:8])
	}
	return ""
}

// cdaTimestampFallbackNow returns a current UTC RFC3339 timestamp.
// Used when a required timestamp field is absent.
func cdaTimestampFallbackNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
