// services/fhir_narrative/patient_narrative.go
// Generates XHTML narrative for a FHIR R4 Patient resource.
// USCDI v3 class: Patient Demographics (Name, DOB, Race, Ethnicity, Sex,
// Preferred Language, Address, Phone, Email).

package fhirnarrative

import "strings"

// GeneratePatientNarrative produces FHIR-compliant XHTML narrative for a
// Patient resource. USCDI v3 labels are used for all field headings.
func GeneratePatientNarrative(r map[string]interface{}) string {
	rows := tableRow("Patient Name", patientDisplayName(r))
	rows += tableRow("Date of Birth", fhirStr(r, "birthDate"))
	rows += tableRow("Sex", patientGenderLabel(fhirStr(r, "gender")))

	// Race and ethnicity (US Core extensions)
	race, eth := patientRaceEthnicity(r)
	rows += tableRow("Race", race)
	rows += tableRow("Ethnicity", eth)

	// Preferred language (first communication entry)
	rows += tableRow("Preferred Language", patientPreferredLanguage(r))

	// Address (first home address)
	rows += tableRow("Address", patientAddressDisplay(r))

	// Telecom — phone and email
	phone, email := patientTelecom(r)
	rows += tableRow("Phone", phone)
	rows += tableRow("Email", email)

	return wrapDiv(heading("Patient Demographics") + buildTable(rows))
}

// patientDisplayName builds "Given Family" from the first name entry.
func patientDisplayName(r map[string]interface{}) string {
	names := fhirArr(r, "name")
	if len(names) == 0 {
		return ""
	}
	nameMap, ok := names[0].(map[string]interface{})
	if !ok {
		return ""
	}
	family := fhirStr(nameMap, "family")
	var givenParts []string
	if givens, ok := nameMap["given"].([]interface{}); ok {
		for _, g := range givens {
			if gs, ok := g.(string); ok && gs != "" {
				givenParts = append(givenParts, gs)
			}
		}
	}
	parts := append(givenParts, family)
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, " ")
}

// patientGenderLabel converts a FHIR gender code to a display label.
func patientGenderLabel(code string) string {
	switch code {
	case "male":
		return "Male"
	case "female":
		return "Female"
	case "other":
		return "Other"
	case "unknown":
		return "Unknown"
	default:
		return code
	}
}

// patientRaceEthnicity extracts race and ethnicity text from US Core extensions.
func patientRaceEthnicity(r map[string]interface{}) (race, ethnicity string) {
	exts, _ := r["extension"].([]interface{})
	for _, rawExt := range exts {
		ext, ok := rawExt.(map[string]interface{})
		if !ok {
			continue
		}
		url, _ := ext["url"].(string)
		subExts, _ := ext["extension"].([]interface{})

		var textVal string
		for _, rawSub := range subExts {
			sub, ok := rawSub.(map[string]interface{})
			if !ok {
				continue
			}
			if subURL, _ := sub["url"].(string); subURL == "text" {
				textVal, _ = sub["valueString"].(string)
			}
		}

		switch {
		case strings.Contains(url, "us-core-race"):
			race = textVal
		case strings.Contains(url, "us-core-ethnicity"):
			ethnicity = textVal
		}
	}
	return
}

// patientPreferredLanguage returns the preferred language text from the first
// communication entry where preferred=true, or the first entry if none is marked.
func patientPreferredLanguage(r map[string]interface{}) string {
	comms := fhirArr(r, "communication")
	first := ""
	for _, rawComm := range comms {
		comm, ok := rawComm.(map[string]interface{})
		if !ok {
			continue
		}
		lang := ccText(comm["language"])
		if lang == "" {
			continue
		}
		if first == "" {
			first = lang
		}
		preferred, _ := comm["preferred"].(bool)
		if preferred {
			return lang
		}
	}
	return first
}

// patientAddressDisplay formats the first address as a single line.
func patientAddressDisplay(r map[string]interface{}) string {
	addrs := fhirArr(r, "address")
	if len(addrs) == 0 {
		return ""
	}
	addr, ok := addrs[0].(map[string]interface{})
	if !ok {
		return ""
	}

	var parts []string

	if lines, ok := addr["line"].([]interface{}); ok {
		for _, l := range lines {
			if ls, ok := l.(string); ok && ls != "" {
				parts = append(parts, ls)
			}
		}
	}
	if city := fhirStr(addr, "city"); city != "" {
		parts = append(parts, city)
	}
	if state := fhirStr(addr, "state"); state != "" {
		parts = append(parts, state)
	}
	if zip := fhirStr(addr, "postalCode"); zip != "" {
		parts = append(parts, zip)
	}
	return strings.Join(parts, ", ")
}

// patientTelecom extracts the first phone and first email from the telecom array.
func patientTelecom(r map[string]interface{}) (phone, email string) {
	telecoms := fhirArr(r, "telecom")
	for _, raw := range telecoms {
		t, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		system := fhirStr(t, "system")
		value := fhirStr(t, "value")
		switch system {
		case "phone":
			if phone == "" {
				phone = value
			}
		case "email":
			if email == "" {
				email = value
			}
		}
	}
	return
}
