// services/executors/format/fhir_bundle_adapter.go
//
// BundleToCanonicalDoc buckets a FHIR R4 Bundle's resources into the same
// canonical, USCDI-keyed JSON shape services/parsers/cda/cda_parser_service.go
// produces from parsed CDA XML ({"header": {...}, "sections": {key:
// {"entries": [...]}}}) — so cda/builder.BuildDocument (which only knows
// that one canonical shape) can serialize a document built from FHIR input
// exactly the same way it would for CDA-sourced or (Phase 3) CSV/DB-sourced
// input.
//
// Field extraction is declarative: resourceFieldMappings is a data table of
// {canonical key -> FHIR path} per resource type, consumed by ONE generic
// resourceToCanonical loop — adding a new resource type (or field) is a
// table edit, not a new Go function. Reuses
// services/executors/field_utils.go's GetFieldValue for path resolution
// (already format-agnostic and already supports the array-index dot-path
// grammar needed here, e.g. "reaction[0].manifestation[0]") instead of
// hand-rolling nested type assertions per field.
//
// Scope: the 7 CCD SHALL sections plus encounters/procedures. Observation is
// disambiguated into vitalSigns/results/socialHistory via resource.category,
// matching the same three-way split OOB CDA→FHIR mapping already uses in
// the opposite direction — each variant is its own table entry
// ("Observation.vitalSigns" etc.), so each only carries the fields that
// section's schema actually defines (no more writing vitalCode/testCode/
// observationCode simultaneously "just in case"). familyHistory/
// assessmentAndPlan/payersInsurance have no FHIR resource mapping here yet —
// feeding those sections currently requires canonical JSON directly (e.g.
// from cda.parse), not this adapter.
package format

import (
	"strconv"

	"ezhealthkonnect/services/executors"
)

// fieldKind selects how resourceToCanonical interprets one mapping row.
type fieldKind int

const (
	// kindScalar copies a string value from SourcePath as-is.
	kindScalar fieldKind = iota
	// kindCode resolves SourcePath as a FHIR CodeableConcept and writes
	// CanonicalKey/+Display/+System via the existing setCode helper.
	kindCode
	// kindCodeValue resolves SourcePath as a CodeableConcept but writes only
	// its first coding's own code — optionally translated through ValueMap
	// (e.g. allergy clinicalStatus "active" -> "Active"); an empty/missing
	// ValueMap entry falls back to the raw code unchanged.
	kindCodeValue
	// kindQuantity resolves SourcePath as a FHIR Quantity and writes
	// CanonicalKey (value) and CanonicalKey+"Unit".
	kindQuantity
)

// fhirFieldMapping is one declarative "read this FHIR path, write this
// canonical key" row — the data-driven replacement for what used to be one
// hand-written extraction function per resource type.
type fhirFieldMapping struct {
	CanonicalKey string
	SourcePath   string // GetFieldValue-compatible path, relative to the resource itself
	Kind         fieldKind
	ValueMap     map[string]string // kindCodeValue only; nil means "pass the raw code through"
}

// allergyStatusDisplay is data, not control flow — the only vocabulary
// translation table this adapter needs today.
var allergyStatusDisplay = map[string]string{
	"active":   "Active",
	"resolved": "Resolved",
	"inactive": "Inactive",
}

// resourceFieldMappings is the whole "which FHIR fields feed which canonical
// keys" contract, per resource type (or, for Observation, per resolved
// section variant — see observationSectionKey). Extending coverage to a new
// FHIR resource type or field means adding a row here, not a new function.
var resourceFieldMappings = map[string][]fhirFieldMapping{
	"AllergyIntolerance": {
		{"medicationAllergyCode", "code", kindCode, nil},
		{"status", "clinicalStatus", kindCodeValue, allergyStatusDisplay},
		{"reaction", "reaction[0].manifestation[0]", kindCode, nil},
		{"severity", "reaction[0].severity", kindScalar, nil},
		{"onsetDate", "onsetDateTime", kindScalar, nil},
	},
	"MedicationStatement": {
		{"drugCode", "medicationCodeableConcept", kindCode, nil},
		{"status", "status", kindScalar, nil},
	},
	"MedicationRequest": {
		{"drugCode", "medicationCodeableConcept", kindCode, nil},
		{"status", "status", kindScalar, nil},
	},
	"Condition": {
		{"conditionCode", "code", kindCode, nil},
		{"status", "clinicalStatus", kindCodeValue, nil},
		{"onsetDate", "onsetDateTime", kindScalar, nil},
		{"resolutionDate", "abatementDateTime", kindScalar, nil},
	},
	"Immunization": {
		{"vaccineCode", "vaccineCode", kindCode, nil},
		{"status", "status", kindScalar, nil},
		{"administrationDate", "occurrenceDateTime", kindScalar, nil},
	},
	"Encounter": {
		{"encounterCode", "type[0]", kindCode, nil},
		{"effectiveTime", "period.start", kindScalar, nil},
	},
	"Procedure": {
		{"procedureCode", "code", kindCode, nil},
		{"status", "status", kindScalar, nil},
		{"effectiveTime", "performedDateTime", kindScalar, nil},
	},
	"Observation.vitalSigns": {
		{"vitalCode", "code", kindCode, nil},
		{"value", "valueQuantity", kindQuantity, nil},
		{"effectiveTime", "effectiveDateTime", kindScalar, nil},
	},
	"Observation.results": {
		{"testCode", "code", kindCode, nil},
		{"resultValue", "valueQuantity", kindQuantity, nil},
		{"resultStatus", "status", kindScalar, nil},
		{"effectiveTime", "effectiveDateTime", kindScalar, nil},
	},
	"Observation.socialHistory": {
		{"observationCode", "code", kindCode, nil},
		{"smokingStatus", "valueCodeableConcept", kindCode, nil},
		{"effectiveTime", "effectiveDateTime", kindScalar, nil},
	},
}

// BundleToCanonicalDoc converts a FHIR Bundle (as a generic map, entry[]
// containing resource maps) into cda/builder.BuildDocument's canonical input
// shape.
func BundleToCanonicalDoc(bundle map[string]interface{}) map[string]interface{} {
	resources := extractResources(bundle)

	sections := map[string]interface{}{}
	addEntries := func(key string, entry map[string]interface{}) {
		sec, ok := sections[key].(map[string]interface{})
		if !ok {
			sec = map[string]interface{}{"entries": []interface{}{}}
			sections[key] = sec
		}
		entries, _ := sec["entries"].([]interface{})
		sec["entries"] = append(entries, entry)
	}

	var patient map[string]interface{}
	var practitioner map[string]interface{}

	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		switch rt {
		case "Patient":
			if patient == nil {
				patient = r
			}
		case "Practitioner":
			if practitioner == nil {
				practitioner = r
			}
		case "Observation":
			if key := observationSectionKey(r); key != "" {
				if mappings, ok := resourceFieldMappings["Observation."+key]; ok {
					addEntries(key, resourceToCanonical(r, mappings))
				}
			}
		default:
			sectionKey, ok := resourceSectionKeys[rt]
			if !ok {
				continue
			}
			mappings, ok := resourceFieldMappings[rt]
			if !ok {
				continue
			}
			addEntries(sectionKey, resourceToCanonical(r, mappings))
		}
	}

	return map[string]interface{}{
		"header":   map[string]interface{}{"patient": patientToCanonical(patient), "author": practitionerToCanonical(practitioner)},
		"sections": sections,
	}
}

// resourceSectionKeys maps a FHIR resourceType to the canonical section key
// its entries belong under. Observation is handled separately (see
// BundleToCanonicalDoc) since one resource type resolves to three different
// section keys depending on resource.category.
var resourceSectionKeys = map[string]string{
	"AllergyIntolerance":  "allergiesAndIntolerances",
	"MedicationStatement": "medications",
	"MedicationRequest":   "medications",
	"Condition":           "problems",
	"Immunization":        "immunizations",
	"Encounter":           "encounters",
	"Procedure":           "procedures",
}

// resourceToCanonical is the single generic extractor every resource type
// (and every Observation variant) shares — behavior is entirely determined
// by the mappings table passed in, not by which resource type called it.
func resourceToCanonical(resource map[string]interface{}, mappings []fhirFieldMapping) map[string]interface{} {
	e := map[string]interface{}{}
	for _, m := range mappings {
		switch m.Kind {
		case kindCode:
			if cc, ok := executors.GetFieldValue(resource, m.SourcePath).(map[string]interface{}); ok {
				setCode(e, m.CanonicalKey, firstCodingFromCC(cc))
			}
		case kindCodeValue:
			if cc, ok := executors.GetFieldValue(resource, m.SourcePath).(map[string]interface{}); ok {
				if coding := firstCodingFromCC(cc); coding != nil {
					if code, _ := coding["code"].(string); code != "" {
						if display, found := m.ValueMap[code]; found {
							e[m.CanonicalKey] = display
						} else {
							e[m.CanonicalKey] = code
						}
					}
				}
			}
		case kindQuantity:
			if q, ok := executors.GetFieldValue(resource, m.SourcePath).(map[string]interface{}); ok {
				if v, ok := q["value"].(float64); ok {
					e[m.CanonicalKey] = trimFloat(v)
				}
				if unit, ok := q["unit"].(string); ok {
					e[m.CanonicalKey+"Unit"] = unit
				}
			}
		default: // kindScalar
			if v, ok := executors.GetFieldValue(resource, m.SourcePath).(string); ok && v != "" {
				e[m.CanonicalKey] = v
			}
		}
	}
	return e
}

// extractResources is already defined in cda_serializer.go (same package)
// and reused here as-is.

// ─── Header ──────────────────────────────────────────────────────────────────
// Left as dedicated functions (not folded into resourceFieldMappings): these
// feed header.patient/header.author, a differently-shaped canonical target
// (flat legacy keys, not a repeating section entry), and involve real
// structural logic (first-name-vs-middle-name splitting, "first home phone"
// selection) beyond a flat field-to-path copy. See cda/builder/header_fields.go
// for the analogous declarative treatment applied to the CDA-side header
// writers this feeds into.

func patientToCanonical(patient map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	if patient == nil {
		return m
	}
	if names, ok := patient["name"].([]interface{}); ok && len(names) > 0 {
		if nm, ok := names[0].(map[string]interface{}); ok {
			if given, ok := nm["given"].([]interface{}); ok && len(given) > 0 {
				if s, ok := given[0].(string); ok {
					m["firstName"] = s
				}
				if len(given) > 1 {
					if s, ok := given[1].(string); ok {
						m["middleName"] = s
					}
				}
			}
			if family, ok := nm["family"].(string); ok {
				m["lastName"] = family
			}
		}
	}
	if dob, ok := patient["birthDate"].(string); ok {
		m["dateOfBirth"] = dob
	}
	if gender, ok := patient["gender"].(string); ok {
		m["sex"] = fhirGenderToCDA(gender)
		m["sexDisplay"] = gender
	}
	if addrs, ok := patient["address"].([]interface{}); ok && len(addrs) > 0 {
		if a, ok := addrs[0].(map[string]interface{}); ok {
			addr := map[string]interface{}{}
			if lines, ok := a["line"].([]interface{}); ok && len(lines) > 0 {
				if s, ok := lines[0].(string); ok {
					addr["street"] = s
				}
			}
			if v, ok := a["city"].(string); ok {
				addr["city"] = v
			}
			if v, ok := a["state"].(string); ok {
				addr["state"] = v
			}
			if v, ok := a["postalCode"].(string); ok {
				addr["postalCode"] = v
			}
			if v, ok := a["country"].(string); ok {
				addr["country"] = v
			}
			m["address"] = addr
		}
	}
	if telecoms, ok := patient["telecom"].([]interface{}); ok {
		for _, t := range telecoms {
			tm, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			if system, _ := tm["system"].(string); system == "phone" {
				if v, ok := tm["value"].(string); ok {
					m["phone"] = v
					break
				}
			}
		}
	}
	return m
}

func practitionerToCanonical(practitioner map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	if practitioner == nil {
		return m
	}
	if names, ok := practitioner["name"].([]interface{}); ok && len(names) > 0 {
		if nm, ok := names[0].(map[string]interface{}); ok {
			if given, ok := nm["given"].([]interface{}); ok && len(given) > 0 {
				if s, ok := given[0].(string); ok {
					m["given"] = s
				}
			}
			if family, ok := nm["family"].(string); ok {
				m["family"] = family
			}
		}
	}
	if ids, ok := practitioner["identifier"].([]interface{}); ok {
		for _, idRaw := range ids {
			idMap, ok := idRaw.(map[string]interface{})
			if !ok {
				continue
			}
			if system, _ := idMap["system"].(string); system == "http://hl7.org/fhir/sid/us-npi" {
				if v, ok := idMap["value"].(string); ok {
					m["npi"] = v
					break
				}
			}
		}
	}
	return m
}

// observationSectionKey disambiguates Observation into vitalSigns/results/
// socialHistory via resource.category, mirroring the same three-way split
// the OOB CDA->FHIR mapping already uses in the opposite direction. Kept as
// its own resolver (not table-driven): one resource type genuinely resolving
// to three different section keys based on a nested value isn't expressible
// as a flat {canonicalKey, path} row.
func observationSectionKey(r map[string]interface{}) string {
	cats, ok := r["category"].([]interface{})
	if !ok {
		return ""
	}
	for _, c := range cats {
		cc, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		codings, ok := cc["coding"].([]interface{})
		if !ok {
			continue
		}
		for _, codingRaw := range codings {
			coding, ok := codingRaw.(map[string]interface{})
			if !ok {
				continue
			}
			switch code, _ := coding["code"].(string); code {
			case "vital-signs":
				return "vitalSigns"
			case "laboratory":
				return "results"
			case "social-history":
				return "socialHistory"
			}
		}
	}
	return ""
}

// ─── coding helpers ──────────────────────────────────────────────────────────
// firstCoding(resource, field) is already defined in cda_serializer.go (same
// package) and reused here as-is; firstCodingFromCC below is this file's own
// addition — a variant that takes an already-resolved CodeableConcept map
// directly, for the code-kind mapping rows above (which resolve a raw path
// via GetFieldValue first, then need to drill into ITS "coding" array).

func firstCodingFromCC(cc map[string]interface{}) map[string]interface{} {
	codings, ok := cc["coding"].([]interface{})
	if !ok || len(codings) == 0 {
		return nil
	}
	c, _ := codings[0].(map[string]interface{})
	return c
}

// setCode writes fieldKey/fieldKey+"Display"/fieldKey+"System" from a FHIR
// coding map, matching generic_section_processor.go's canonical key
// convention exactly.
func setCode(e map[string]interface{}, fieldKey string, coding map[string]interface{}) {
	if coding == nil {
		return
	}
	if v, ok := coding["code"].(string); ok && v != "" {
		e[fieldKey] = v
	}
	if v, ok := coding["display"].(string); ok && v != "" {
		e[fieldKey+"Display"] = v
	}
	if v, ok := coding["system"].(string); ok && v != "" {
		e[fieldKey+"System"] = fhirSystemToOID(v)
	}
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
