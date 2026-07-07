// services/executors/format/fhir_bundle_adapter.go
//
// Reuses fhirGenderToCDA/fhirSystemToOID from cda_serializer.go (same package).
//
// BundleToCanonicalDoc buckets a FHIR R4 Bundle's resources into the same
// canonical, USCDI-keyed JSON shape services/parsers/cda/cda_parser_service.go
// produces from parsed CDA XML ({"header": {...}, "sections": {key:
// {"entries": [...]}}}) — so cda/builder.BuildDocument (which only knows
// that one canonical shape) can serialize a document built from FHIR input
// exactly the same way it would for CDA-sourced or (Phase 3) CSV/DB-sourced
// input.
//
// Scope: the 7 CCD SHALL sections plus encounters/procedures. Observation is
// disambiguated into vitalSigns/results/socialHistory via resource.category,
// matching the same three-way split OOB CDA→FHIR mapping already uses in
// the opposite direction. familyHistory/assessmentAndPlan/payersInsurance
// have no FHIR resource mapping here yet — feeding those sections currently
// requires canonical JSON directly (e.g. from cda.parse), not this adapter.
package format

import "strconv"

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
		case "AllergyIntolerance":
			addEntries("allergiesAndIntolerances", allergyToCanonical(r))
		case "MedicationStatement", "MedicationRequest":
			addEntries("medications", medicationToCanonical(r))
		case "Condition":
			addEntries("problems", conditionToCanonical(r))
		case "Immunization":
			addEntries("immunizations", immunizationToCanonical(r))
		case "Encounter":
			addEntries("encounters", encounterToCanonical(r))
		case "Procedure":
			addEntries("procedures", procedureToCanonical(r))
		case "Observation":
			key := observationSectionKey(r)
			if key != "" {
				addEntries(key, observationToCanonical(r))
			}
		}
	}

	return map[string]interface{}{
		"header":   map[string]interface{}{"patient": patientToCanonical(patient), "author": practitionerToCanonical(practitioner)},
		"sections": sections,
	}
}

// extractResources and firstCoding are already defined in cda_serializer.go
// (same package) and reused here as-is.

// ─── Header ──────────────────────────────────────────────────────────────────

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

// ─── Section resources ───────────────────────────────────────────────────────

func allergyToCanonical(r map[string]interface{}) map[string]interface{} {
	e := map[string]interface{}{}
	setCode(e, "medicationAllergyCode", firstCoding(r, "code"))
	if status := firstCoding(r, "clinicalStatus"); status != nil {
		if code, _ := status["code"].(string); code != "" {
			e["status"] = allergyClinicalStatusDisplay(code)
		}
	}
	if reactions, ok := r["reaction"].([]interface{}); ok && len(reactions) > 0 {
		if reaction, ok := reactions[0].(map[string]interface{}); ok {
			if manifestations, ok := reaction["manifestation"].([]interface{}); ok && len(manifestations) > 0 {
				if mfst, ok := manifestations[0].(map[string]interface{}); ok {
					setCode(e, "reaction", firstCodingFromCC(mfst))
				}
			}
			if severity, ok := reaction["severity"].(string); ok {
				e["severity"] = severity
			}
		}
	}
	if onset, ok := r["onsetDateTime"].(string); ok {
		e["onsetDate"] = onset
	}
	return e
}

func medicationToCanonical(r map[string]interface{}) map[string]interface{} {
	e := map[string]interface{}{}
	setCode(e, "drugCode", firstCoding(r, "medicationCodeableConcept"))
	if status, ok := r["status"].(string); ok {
		e["status"] = status
	}
	return e
}

func conditionToCanonical(r map[string]interface{}) map[string]interface{} {
	e := map[string]interface{}{}
	setCode(e, "conditionCode", firstCoding(r, "code"))
	if status := firstCoding(r, "clinicalStatus"); status != nil {
		if code, _ := status["code"].(string); code != "" {
			e["status"] = code
		}
	}
	if onset, ok := r["onsetDateTime"].(string); ok {
		e["onsetDate"] = onset
	}
	if abatement, ok := r["abatementDateTime"].(string); ok {
		e["resolutionDate"] = abatement
	}
	return e
}

func immunizationToCanonical(r map[string]interface{}) map[string]interface{} {
	e := map[string]interface{}{}
	setCode(e, "vaccineCode", firstCoding(r, "vaccineCode"))
	if status, ok := r["status"].(string); ok {
		e["status"] = status
	}
	if date, ok := r["occurrenceDateTime"].(string); ok {
		e["administrationDate"] = date
	}
	return e
}

func encounterToCanonical(r map[string]interface{}) map[string]interface{} {
	e := map[string]interface{}{}
	if types, ok := r["type"].([]interface{}); ok && len(types) > 0 {
		if cc, ok := types[0].(map[string]interface{}); ok {
			setCode(e, "encounterCode", firstCodingFromCC(cc))
		}
	}
	if period, ok := r["period"].(map[string]interface{}); ok {
		if start, ok := period["start"].(string); ok {
			e["effectiveTime"] = start
		}
	}
	return e
}

func procedureToCanonical(r map[string]interface{}) map[string]interface{} {
	e := map[string]interface{}{}
	setCode(e, "procedureCode", firstCoding(r, "code"))
	if status, ok := r["status"].(string); ok {
		e["status"] = status
	}
	if dt, ok := r["performedDateTime"].(string); ok {
		e["effectiveTime"] = dt
	}
	return e
}

// observationSectionKey disambiguates Observation into vitalSigns/results/
// socialHistory via resource.category, mirroring the same three-way split
// the OOB CDA->FHIR mapping already uses in the opposite direction.
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

func observationToCanonical(r map[string]interface{}) map[string]interface{} {
	e := map[string]interface{}{}
	setCode(e, "vitalCode", firstCoding(r, "code"))
	// results section uses "testCode" instead of "vitalCode" — write both
	// keys from the same source coding so whichever section this entry is
	// filed under finds the field it expects.
	setCode(e, "testCode", firstCoding(r, "code"))
	setCode(e, "observationCode", firstCoding(r, "code"))

	if q, ok := r["valueQuantity"].(map[string]interface{}); ok {
		if v, ok := q["value"].(float64); ok {
			e["value"] = trimFloat(v)
			e["resultValue"] = trimFloat(v)
		}
		if unit, ok := q["unit"].(string); ok {
			e["valueUnit"] = unit
			e["resultValueUnit"] = unit
		}
	}
	if cc, ok := r["valueCodeableConcept"].(map[string]interface{}); ok {
		setCode(e, "smokingStatus", firstCodingFromCC(cc))
	}
	if status, ok := r["status"].(string); ok {
		e["resultStatus"] = status
	}
	if dt, ok := r["effectiveDateTime"].(string); ok {
		e["effectiveTime"] = dt
	}
	return e
}

// ─── coding helpers ──────────────────────────────────────────────────────────
// firstCoding(resource, field) is already defined in cda_serializer.go (same
// package) and reused here as-is; firstCodingFromCC below is this file's own
// addition — a variant that takes an already-resolved CodeableConcept map
// directly, for the few call sites (reaction manifestation, encounter type,
// Observation.valueCodeableConcept) that don't start from a resource+field pair.

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

func allergyClinicalStatusDisplay(code string) string {
	switch code {
	case "active":
		return "Active"
	case "resolved":
		return "Resolved"
	case "inactive":
		return "Inactive"
	default:
		return code
	}
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
