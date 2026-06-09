// services/executors/format/cda_serializer.go
// CDASerializer converts a FHIR R4 Bundle to a C-CDA 2.1 XML document.
//
// MVP scope: Patient header + 4 clinical sections (Allergies, Medications,
// Problems, Immunizations). XHTML narrative is auto-generated from structured
// FHIR data. Output is well-formed C-CDA 2.1 XML with correct templateId OIDs.
// Full Schematron conformance is Sprint D scope.

package format

import (
	"fmt"
	"strings"
	"time"
)

// CDASerializer converts a FHIR R4 Bundle map to a C-CDA 2.1 XML string.
type CDASerializer struct{}

// NewCDASerializer returns a ready-to-use serializer.
func NewCDASerializer() *CDASerializer { return &CDASerializer{} }

// Serialize converts a FHIR Bundle to C-CDA 2.1 XML.
// profile is currently ignored in MVP (always outputs C-CDA 2.1).
func (s *CDASerializer) Serialize(fhirBundle map[string]interface{}, profile string) (string, error) {
	if fhirBundle == nil {
		return "", fmt.Errorf("cda_serializer: fhirBundle is nil")
	}

	resources := extractResources(fhirBundle)

	var patient map[string]interface{}
	var allergies, medications, conditions, immunizations []map[string]interface{}

	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		switch rt {
		case "Patient":
			if patient == nil {
				patient = r
			}
		case "AllergyIntolerance":
			allergies = append(allergies, r)
		case "MedicationStatement":
			medications = append(medications, r)
		case "Condition":
			conditions = append(conditions, r)
		case "Immunization":
			immunizations = append(immunizations, r)
		}
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString("\n")
	b.WriteString(`<ClinicalDocument xmlns="urn:hl7-org:v3"`)
	b.WriteString(` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`)
	b.WriteString(` xsi:schemaLocation="urn:hl7-org:v3 CDA.xsd">`)
	b.WriteString("\n")

	// Document-level templateIds (C-CDA 2.1 Continuity of Care Document)
	b.WriteString(`  <templateId root="2.16.840.1.113883.10.20.22.1.1" extension="2015-08-01"/>`)
	b.WriteString("\n")
	b.WriteString(`  <templateId root="2.16.840.1.113883.10.20.22.1.2" extension="2015-08-01"/>`)
	b.WriteString("\n")

	// Document metadata
	docID := fmt.Sprintf("ehk-%d", time.Now().UnixNano())
	b.WriteString(fmt.Sprintf(`  <id root="%s"/>`, docID))
	b.WriteString("\n")
	b.WriteString(`  <code code="34133-9" codeSystem="2.16.840.1.113883.6.1"`)
	b.WriteString(` displayName="Summarization of Episode Note" codeSystemName="LOINC"/>`)
	b.WriteString("\n")
	b.WriteString(`  <title>Continuity of Care Document</title>`)
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`  <effectiveTime value="%s"/>`, time.Now().UTC().Format("20060102150405")))
	b.WriteString("\n")
	b.WriteString(`  <confidentialityCode code="N" codeSystem="2.16.840.1.113883.5.25"/>`)
	b.WriteString("\n")
	b.WriteString(`  <languageCode code="en-US"/>`)
	b.WriteString("\n")

	// Patient (recordTarget)
	b.WriteString(s.serializePatient(patient))

	// Author (generated)
	b.WriteString(s.serializeAuthor())

	// Custodian (generated)
	b.WriteString(s.serializeCustodian())

	// Structured Body
	b.WriteString("  <component>\n    <structuredBody>\n")
	b.WriteString(s.serializeAllergiesSection(allergies))
	b.WriteString(s.serializeMedicationsSection(medications))
	b.WriteString(s.serializeProblemsSection(conditions))
	b.WriteString(s.serializeImmunizationsSection(immunizations))
	b.WriteString("    </structuredBody>\n  </component>\n")

	b.WriteString("</ClinicalDocument>\n")

	return b.String(), nil
}

// ─── Patient ─────────────────────────────────────────────────────────────────

func (s *CDASerializer) serializePatient(patient map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("  <recordTarget>\n    <patientRole>\n")

	if patient != nil {
		family, given := "", ""
		if names, ok := patient["name"].([]interface{}); ok && len(names) > 0 {
			if nm, ok := names[0].(map[string]interface{}); ok {
				family, _ = nm["family"].(string)
				if givenList, ok := nm["given"].([]interface{}); ok && len(givenList) > 0 {
					given, _ = givenList[0].(string)
				}
			}
		}
		dob, _ := patient["birthDate"].(string)
		gender, _ := patient["gender"].(string)

		b.WriteString("      <patient>\n")
		b.WriteString(fmt.Sprintf("        <name><given>%s</given><family>%s</family></name>\n",
			xmlEscape(given), xmlEscape(family)))
		if dob != "" {
			b.WriteString(fmt.Sprintf("        <birthTime value=\"%s\"/>\n", strings.ReplaceAll(dob, "-", "")))
		}
		b.WriteString(fmt.Sprintf("        <administrativeGenderCode code=\"%s\" codeSystem=\"2.16.840.1.113883.5.1\"/>\n",
			fhirGenderToCDA(gender)))
		b.WriteString("      </patient>\n")
	}

	b.WriteString("    </patientRole>\n  </recordTarget>\n")
	return b.String()
}

// ─── Author / Custodian ──────────────────────────────────────────────────────

func (s *CDASerializer) serializeAuthor() string {
	ts := time.Now().UTC().Format("20060102150405")
	return fmt.Sprintf(`  <author>
    <time value="%s"/>
    <assignedAuthor>
      <id root="2.16.840.1.113883.4.6"/>
      <representedOrganization><name>ezHealthKonnect</name></representedOrganization>
    </assignedAuthor>
  </author>
`, ts)
}

func (s *CDASerializer) serializeCustodian() string {
	return `  <custodian>
    <assignedCustodian>
      <representedCustodianOrganization>
        <id root="2.16.840.1.113883.4.6"/>
        <name>ezHealthKonnect</name>
      </representedCustodianOrganization>
    </assignedCustodian>
  </custodian>
`
}

// ─── Allergies section ───────────────────────────────────────────────────────

func (s *CDASerializer) serializeAllergiesSection(allergies []map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("      <component>\n        <section>\n")
	b.WriteString(`          <templateId root="2.16.840.1.113883.10.20.22.2.6.1" extension="2015-08-01"/>`)
	b.WriteString("\n")
	b.WriteString(`          <code code="48765-2" codeSystem="2.16.840.1.113883.6.1" displayName="Allergies and Adverse Reactions"/>`)
	b.WriteString("\n          <title>Allergies and Intolerances</title>\n")

	var rows strings.Builder
	for _, a := range allergies {
		code := firstCodingCode(a, "code")
		display := firstCodingDisplay(a, "code")
		status := firstCodingCode(a, "clinicalStatus")
		rows.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td></tr>\n",
			xmlEscape(display), xmlEscape(code), xmlEscape(status)))
	}
	b.WriteString(fmt.Sprintf("          <text><table><thead><tr><th>Substance</th><th>Code</th><th>Status</th></tr></thead><tbody>%s</tbody></table></text>\n",
		rows.String()))

	for _, a := range allergies {
		id, _ := a["id"].(string)
		code := firstCodingCode(a, "code")
		display := firstCodingDisplay(a, "code")
		system := fhirSystemToOID(firstCodingSystem(a, "code"))
		b.WriteString(fmt.Sprintf(`          <entry typeCode="DRIV">
            <act classCode="ACT" moodCode="EVN">
              <templateId root="2.16.840.1.113883.10.20.22.4.30" extension="2015-08-01"/>
              <id root="%s"/>
              <code code="CONC" codeSystem="2.16.840.1.113883.5.6"/>
              <statusCode code="active"/>
              <entryRelationship typeCode="SUBJ">
                <observation classCode="OBS" moodCode="EVN">
                  <templateId root="2.16.840.1.113883.10.20.22.4.7" extension="2014-06-09"/>
                  <id root="%s-obs"/>
                  <code code="ASSERTION" codeSystem="2.16.840.1.113883.5.4"/>
                  <statusCode code="completed"/>
                  <participant typeCode="CSM">
                    <participantRole classCode="MANU">
                      <playingEntity classCode="MMAT">
                        <code code="%s" codeSystem="%s" displayName="%s"/>
                      </playingEntity>
                    </participantRole>
                  </participant>
                </observation>
              </entryRelationship>
            </act>
          </entry>
`, xmlEscape(id), xmlEscape(id), xmlEscape(code), system, xmlEscape(display)))
	}

	b.WriteString("        </section>\n      </component>\n")
	return b.String()
}

// ─── Medications section ─────────────────────────────────────────────────────

func (s *CDASerializer) serializeMedicationsSection(meds []map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("      <component>\n        <section>\n")
	b.WriteString(`          <templateId root="2.16.840.1.113883.10.20.22.2.1.1" extension="2014-06-09"/>`)
	b.WriteString("\n")
	b.WriteString(`          <code code="10160-0" codeSystem="2.16.840.1.113883.6.1" displayName="History of Medication Use"/>`)
	b.WriteString("\n          <title>Medications</title>\n")

	var rows strings.Builder
	for _, m := range meds {
		display := firstCodingDisplay(m, "medicationCodeableConcept")
		status, _ := m["status"].(string)
		rows.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>\n",
			xmlEscape(display), xmlEscape(status)))
	}
	b.WriteString(fmt.Sprintf("          <text><table><thead><tr><th>Medication</th><th>Status</th></tr></thead><tbody>%s</tbody></table></text>\n",
		rows.String()))

	for _, m := range meds {
		id, _ := m["id"].(string)
		code := firstCodingCode(m, "medicationCodeableConcept")
		display := firstCodingDisplay(m, "medicationCodeableConcept")
		system := fhirSystemToOID(firstCodingSystem(m, "medicationCodeableConcept"))
		b.WriteString(fmt.Sprintf(`          <entry typeCode="DRIV">
            <substanceAdministration classCode="SBADM" moodCode="EVN">
              <templateId root="2.16.840.1.113883.10.20.22.4.16" extension="2014-06-09"/>
              <id root="%s"/>
              <statusCode code="completed"/>
              <consumable>
                <manufacturedProduct classCode="MANU">
                  <templateId root="2.16.840.1.113883.10.20.22.4.23" extension="2014-06-09"/>
                  <manufacturedMaterial>
                    <code code="%s" codeSystem="%s" displayName="%s"/>
                  </manufacturedMaterial>
                </manufacturedProduct>
              </consumable>
            </substanceAdministration>
          </entry>
`, xmlEscape(id), xmlEscape(code), system, xmlEscape(display)))
	}

	b.WriteString("        </section>\n      </component>\n")
	return b.String()
}

// ─── Problems section ────────────────────────────────────────────────────────

func (s *CDASerializer) serializeProblemsSection(conditions []map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("      <component>\n        <section>\n")
	b.WriteString(`          <templateId root="2.16.840.1.113883.10.20.22.2.5.1" extension="2015-08-01"/>`)
	b.WriteString("\n")
	b.WriteString(`          <code code="11450-4" codeSystem="2.16.840.1.113883.6.1" displayName="Problem List"/>`)
	b.WriteString("\n          <title>Problems</title>\n")

	var rows strings.Builder
	for _, c := range conditions {
		display := firstCodingDisplay(c, "code")
		status := firstCodingCode(c, "clinicalStatus")
		rows.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>\n",
			xmlEscape(display), xmlEscape(status)))
	}
	b.WriteString(fmt.Sprintf("          <text><table><thead><tr><th>Problem</th><th>Status</th></tr></thead><tbody>%s</tbody></table></text>\n",
		rows.String()))

	for _, c := range conditions {
		id, _ := c["id"].(string)
		code := firstCodingCode(c, "code")
		display := firstCodingDisplay(c, "code")
		system := fhirSystemToOID(firstCodingSystem(c, "code"))
		b.WriteString(fmt.Sprintf(`          <entry typeCode="DRIV">
            <act classCode="ACT" moodCode="EVN">
              <templateId root="2.16.840.1.113883.10.20.22.4.3" extension="2015-08-01"/>
              <id root="%s"/>
              <code code="CONC" codeSystem="2.16.840.1.113883.5.6"/>
              <statusCode code="active"/>
              <entryRelationship typeCode="SUBJ">
                <observation classCode="OBS" moodCode="EVN">
                  <templateId root="2.16.840.1.113883.10.20.22.4.4" extension="2015-08-01"/>
                  <id root="%s-obs"/>
                  <code code="55607006" codeSystem="2.16.840.1.113883.6.96" displayName="Problem"/>
                  <statusCode code="completed"/>
                  <value xsi:type="CD" code="%s" codeSystem="%s" displayName="%s"/>
                </observation>
              </entryRelationship>
            </act>
          </entry>
`, xmlEscape(id), xmlEscape(id), xmlEscape(code), system, xmlEscape(display)))
	}

	b.WriteString("        </section>\n      </component>\n")
	return b.String()
}

// ─── Immunizations section ───────────────────────────────────────────────────

func (s *CDASerializer) serializeImmunizationsSection(immunizations []map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("      <component>\n        <section>\n")
	b.WriteString(`          <templateId root="2.16.840.1.113883.10.20.22.2.2.1" extension="2015-08-01"/>`)
	b.WriteString("\n")
	b.WriteString(`          <code code="11369-6" codeSystem="2.16.840.1.113883.6.1" displayName="Immunizations"/>`)
	b.WriteString("\n          <title>Immunizations</title>\n")

	var rows strings.Builder
	for _, imm := range immunizations {
		display := firstCodingDisplay(imm, "vaccineCode")
		date, _ := imm["occurrenceDateTime"].(string)
		rows.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>\n",
			xmlEscape(display), xmlEscape(date)))
	}
	b.WriteString(fmt.Sprintf("          <text><table><thead><tr><th>Vaccine</th><th>Date</th></tr></thead><tbody>%s</tbody></table></text>\n",
		rows.String()))

	for _, imm := range immunizations {
		id, _ := imm["id"].(string)
		code := firstCodingCode(imm, "vaccineCode")
		display := firstCodingDisplay(imm, "vaccineCode")
		system := fhirSystemToOID(firstCodingSystem(imm, "vaccineCode"))
		date, _ := imm["occurrenceDateTime"].(string)
		b.WriteString(fmt.Sprintf(`          <entry typeCode="DRIV">
            <substanceAdministration classCode="SBADM" moodCode="EVN">
              <templateId root="2.16.840.1.113883.10.20.22.4.52" extension="2015-08-01"/>
              <id root="%s"/>
              <statusCode code="completed"/>
              <effectiveTime value="%s"/>
              <consumable>
                <manufacturedProduct classCode="MANU">
                  <templateId root="2.16.840.1.113883.10.20.22.4.54" extension="2014-06-09"/>
                  <manufacturedMaterial>
                    <code code="%s" codeSystem="%s" displayName="%s"/>
                  </manufacturedMaterial>
                </manufacturedProduct>
              </consumable>
            </substanceAdministration>
          </entry>
`, xmlEscape(id), strings.ReplaceAll(date, "-", ""), xmlEscape(code), system, xmlEscape(display)))
	}

	b.WriteString("        </section>\n      </component>\n")
	return b.String()
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func extractResources(bundle map[string]interface{}) []map[string]interface{} {
	entries, ok := bundle["entry"].([]interface{})
	if !ok {
		return nil
	}
	var resources []map[string]interface{}
	for _, e := range entries {
		entry, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if r, ok := entry["resource"].(map[string]interface{}); ok {
			resources = append(resources, r)
		}
	}
	return resources
}

func firstCoding(resource map[string]interface{}, field string) map[string]interface{} {
	cc, ok := resource[field].(map[string]interface{})
	if !ok {
		return nil
	}
	codings, ok := cc["coding"].([]interface{})
	if !ok || len(codings) == 0 {
		return nil
	}
	c, _ := codings[0].(map[string]interface{})
	return c
}

func firstCodingCode(resource map[string]interface{}, field string) string {
	if c := firstCoding(resource, field); c != nil {
		v, _ := c["code"].(string)
		return v
	}
	return ""
}

func firstCodingDisplay(resource map[string]interface{}, field string) string {
	if c := firstCoding(resource, field); c != nil {
		v, _ := c["display"].(string)
		return v
	}
	if cc, ok := resource[field].(map[string]interface{}); ok {
		if t, ok := cc["text"].(string); ok {
			return t
		}
	}
	return ""
}

func firstCodingSystem(resource map[string]interface{}, field string) string {
	if c := firstCoding(resource, field); c != nil {
		v, _ := c["system"].(string)
		return v
	}
	return ""
}

// fhirSystemToOID converts a FHIR system URI to its HL7 OID equivalent.
func fhirSystemToOID(uri string) string {
	switch uri {
	case "http://www.nlm.nih.gov/research/umls/rxnorm":
		return "2.16.840.1.113883.6.88"
	case "http://snomed.info/sct":
		return "2.16.840.1.113883.6.96"
	case "http://loinc.org":
		return "2.16.840.1.113883.6.1"
	case "http://hl7.org/fhir/sid/icd-10-cm":
		return "2.16.840.1.113883.6.90"
	case "http://hl7.org/fhir/sid/cvx":
		return "2.16.840.1.113883.12.292"
	case "http://hl7.org/fhir/sid/ndc":
		return "2.16.840.1.113883.6.69"
	default:
		if uri != "" {
			return uri
		}
		return "2.16.840.1.113883.6.96" // default SNOMED
	}
}

// fhirGenderToCDA converts FHIR gender to CDA administrative gender code.
func fhirGenderToCDA(fhir string) string {
	switch fhir {
	case "male":
		return "M"
	case "female":
		return "F"
	default:
		return "UN"
	}
}

// xmlEscape escapes special XML characters in text content.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
