// services/cda_fhir/mapper.go
// CDATOFHIRMapper orchestrates all individual resource mappers and produces a
// FHIR R4 Bundle (type: "collection") from a parsed CDA document.
//
// Input: the ParsedJSON map produced by CDAParserService.Parse().
// Output: a FHIR Bundle map ready for JSON serialisation.

package cdafhir

import (
	"fmt"
	"time"
)

// CDATOFHIRMapper converts a fully-parsed CDA document (USCDI-keyed ParsedJSON)
// to a FHIR R4 Bundle. Construct once and reuse — all fields are stateless.
type CDATOFHIRMapper struct {
	patient      *PatientMapper
	allergy      *AllergyMapper
	medication   *MedicationMapper
	condition    *ConditionMapper
	observation  *ObservationMapper
	immunization *ImmunizationMapper
}

// NewCDATOFHIRMapper returns a ready-to-use mapper.
func NewCDATOFHIRMapper() *CDATOFHIRMapper {
	return &CDATOFHIRMapper{
		patient:      &PatientMapper{},
		allergy:      &AllergyMapper{},
		medication:   &MedicationMapper{},
		condition:    &ConditionMapper{},
		observation:  &ObservationMapper{},
		immunization: &ImmunizationMapper{},
	}
}

// Map converts a parsed CDA document to a FHIR R4 Bundle.
// parsedCDA must be the map returned by CDAParserService.Parse().ParsedJSON.
func (m *CDATOFHIRMapper) Map(parsedCDA map[string]interface{}) (map[string]interface{}, error) {
	if parsedCDA == nil {
		return nil, fmt.Errorf("cda_fhir: parsedCDA is nil")
	}

	// ── Patient ──────────────────────────────────────────────────
	header, _ := parsedCDA["header"].(map[string]interface{})
	var patientData map[string]interface{}
	if header != nil {
		patientData, _ = header["patient"].(map[string]interface{})
	}

	patientResource := m.patient.Map(patientData)
	patientRef := "Patient/patient-1"
	if patientResource == nil {
		patientResource = map[string]interface{}{"resourceType": "Patient", "id": "patient-1"}
	}
	patientResource["id"] = "patient-1"

	var entries []interface{}
	entries = append(entries, bundleEntry(patientRef, patientResource))

	// ── Sections ─────────────────────────────────────────────────
	sections, _ := parsedCDA["sections"].(map[string]interface{})
	if sections == nil {
		sections = map[string]interface{}{}
	}

	// Allergies
	if allergySection, ok := sections["allergiesAndIntolerances"].(map[string]interface{}); ok {
		for _, r := range m.allergy.Map(sectionEntries(allergySection), patientRef) {
			entries = append(entries, bundleEntry("AllergyIntolerance/"+r["id"].(string), r))
		}
	}

	// Medications
	if medSection, ok := sections["medications"].(map[string]interface{}); ok {
		for _, r := range m.medication.Map(sectionEntries(medSection), patientRef) {
			entries = append(entries, bundleEntry("MedicationStatement/"+r["id"].(string), r))
		}
	}

	// Problems / Conditions
	if probSection, ok := sections["problems"].(map[string]interface{}); ok {
		for _, r := range m.condition.Map(sectionEntries(probSection), patientRef) {
			entries = append(entries, bundleEntry("Condition/"+r["id"].(string), r))
		}
	}

	// Vital Signs
	if vitalsSection, ok := sections["vitalSigns"].(map[string]interface{}); ok {
		for _, r := range m.observation.MapVitals(sectionEntries(vitalsSection), patientRef) {
			entries = append(entries, bundleEntry("Observation/"+r["id"].(string), r))
		}
	}

	// Lab Results
	if resultsSection, ok := sections["results"].(map[string]interface{}); ok {
		for _, r := range m.observation.MapResults(sectionEntries(resultsSection), patientRef) {
			entries = append(entries, bundleEntry("Observation/"+r["id"].(string), r))
		}
	}

	// Immunizations
	if immunSection, ok := sections["immunizations"].(map[string]interface{}); ok {
		for _, r := range m.immunization.Map(sectionEntries(immunSection), patientRef) {
			entries = append(entries, bundleEntry("Immunization/"+r["id"].(string), r))
		}
	}

	// ── Bundle ───────────────────────────────────────────────────
	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "collection",
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"entry":        entries,
		"meta": map[string]interface{}{
			"profile": []interface{}{
				"http://hl7.org/fhir/us/core/StructureDefinition/us-core-documentreference",
			},
			"source": "CDA/CCD",
		},
	}

	// Propagate source profile metadata when present
	if profile, ok := parsedCDA["cdaProfile"].(string); ok {
		bundle["_cdaProfile"] = profile
	}

	return bundle, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func bundleEntry(fullURL string, resource map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"fullUrl":  "urn:uuid:" + fullURL,
		"resource": resource,
	}
}

// sectionEntries extracts the "entries" slice from a section map, returning
// nil when absent or of unexpected type.
func sectionEntries(section map[string]interface{}) []map[string]interface{} {
	raw, ok := section["entries"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []map[string]interface{}:
		return v
	case []interface{}:
		result := make([]map[string]interface{}, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result
	}
	return nil
}
