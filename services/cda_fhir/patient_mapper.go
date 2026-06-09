// services/cda_fhir/patient_mapper.go
// Maps CDA header.patient (from CDAHeaderParser) → FHIR R4 Patient resource.

package cdafhir

import "fmt"

// PatientMapper converts CDA header demographics to a FHIR Patient resource.
type PatientMapper struct{}

// Map converts the header.patient map extracted by CDAHeaderParser into a
// FHIR R4 Patient resource. Returns nil when the input map is empty.
func (m *PatientMapper) Map(patient map[string]interface{}) map[string]interface{} {
	if len(patient) == 0 {
		return nil
	}

	p := map[string]interface{}{
		"resourceType": "Patient",
	}

	// Name
	family := strField(patient, "lastName")
	given := strField(patient, "firstName")
	middle := strField(patient, "middleName")
	suffix := strField(patient, "suffix")

	if family != "" || given != "" {
		nameEntry := map[string]interface{}{"use": "official"}
		if family != "" {
			nameEntry["family"] = family
		}
		var givenList []interface{}
		if given != "" {
			givenList = append(givenList, given)
		}
		if middle != "" {
			givenList = append(givenList, middle)
		}
		if len(givenList) > 0 {
			nameEntry["given"] = givenList
		}
		if suffix != "" {
			nameEntry["suffix"] = []interface{}{suffix}
		}
		p["name"] = []interface{}{nameEntry}
	}

	// Birth date
	if dob := strField(patient, "dateOfBirth"); dob != "" {
		p["birthDate"] = FormatDate(dob)
	}

	// Gender
	if sex := strField(patient, "sex"); sex != "" {
		p["gender"] = mapGender(sex)
	}

	// Telecom
	var telecom []interface{}
	if phone := strField(patient, "phone"); phone != "" {
		telecom = append(telecom, map[string]interface{}{
			"system": "phone", "value": phone, "use": "home",
		})
	}
	if email := strField(patient, "email"); email != "" {
		telecom = append(telecom, map[string]interface{}{
			"system": "email", "value": email,
		})
	}
	if len(telecom) > 0 {
		p["telecom"] = telecom
	}

	// Address
	if addrRaw, ok := patient["address"].(map[string]interface{}); ok && len(addrRaw) > 0 {
		addr := map[string]interface{}{"use": "home"}
		if street := strField(addrRaw, "street"); street != "" {
			addr["line"] = []interface{}{street}
		}
		if city := strField(addrRaw, "city"); city != "" {
			addr["city"] = city
		}
		if state := strField(addrRaw, "state"); state != "" {
			addr["state"] = state
		}
		if zip := strField(addrRaw, "postalCode"); zip != "" {
			addr["postalCode"] = zip
		}
		if country := strField(addrRaw, "country"); country != "" {
			addr["country"] = country
		}
		p["address"] = []interface{}{addr}
	}

	// Race extension (US Core)
	if race := strField(patient, "race"); race != "" {
		raceDisplay := strField(patient, "raceDisplay")
		p["extension"] = buildExtensions(patient, race, raceDisplay)
	}

	// Preferred language
	if lang := strField(patient, "preferredLanguage"); lang != "" {
		p["communication"] = []interface{}{
			map[string]interface{}{
				"language":  NewCodeableConcept(lang, lang, "urn:ietf:bcp:47"),
				"preferred": true,
			},
		}
	}

	return p
}

// mapGender converts CDA administrative gender codes to FHIR gender values.
func mapGender(code string) string {
	switch code {
	case "M":
		return "male"
	case "F":
		return "female"
	case "UN":
		return "unknown"
	default:
		return "unknown"
	}
}

// buildExtensions creates US Core race/ethnicity extensions.
func buildExtensions(patient map[string]interface{}, race, raceDisplay string) []interface{} {
	var exts []interface{}

	raceExt := map[string]interface{}{
		"url": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-race",
		"extension": []interface{}{
			map[string]interface{}{
				"url":           "ombCategory",
				"valueCoding":   NewCoding(race, raceDisplay, "urn:oid:2.16.840.1.113883.6.238"),
			},
			map[string]interface{}{
				"url":         "text",
				"valueString": fmt.Sprintf("%s", raceDisplay),
			},
		},
	}
	exts = append(exts, raceExt)

	if eth := strField(patient, "ethnicity"); eth != "" {
		ethDisplay := strField(patient, "ethnicityDisplay")
		ethExt := map[string]interface{}{
			"url": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-ethnicity",
			"extension": []interface{}{
				map[string]interface{}{
					"url":         "ombCategory",
					"valueCoding": NewCoding(eth, ethDisplay, "urn:oid:2.16.840.1.113883.6.238"),
				},
				map[string]interface{}{
					"url":         "text",
					"valueString": ethDisplay,
				},
			},
		}
		exts = append(exts, ethExt)
	}

	return exts
}
