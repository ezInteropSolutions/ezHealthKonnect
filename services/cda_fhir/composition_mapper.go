// services/cda_fhir/composition_mapper.go
// CompositionMapper builds the FHIR R4 Composition resource that must be the
// first entry in a document-type Bundle (FHIR R4 §3.1.3).
//
// It is only invoked when CDAToFHIRConfig.BundleType == "document". For
// "collection" and "transaction" bundles it is a no-op.
//
// Usage:
//   cm := NewCompositionMapper()
//   comp := cm.BuildComposition(headerMap, allResources, config)
//   // prepend comp to bundle entries

package cdafhir

import (
	"fmt"
	"time"
)

// documentTypeLoincCode maps C-CDA document type names to their canonical
// LOINC codes as specified in C-CDA 2.1 Companion Guide.
var documentTypeLoincCode = map[string]loincEntry{
	"CCD":              {code: "34133-9", display: "Summarization of episode note"},
	"DischargeSummary": {code: "18842-5", display: "Discharge summary"},
	"ReferralNote":     {code: "57133-1", display: "Referral note"},
	"HistoryPhysical":  {code: "34117-2", display: "History and physical note"},
	"ConsultationNote": {code: "11488-4", display: "Consult note"},
	"ProgressNote":     {code: "11506-3", display: "Progress note"},
	"TransferSummary":  {code: "18761-7", display: "Transfer summary note"},
	"ProcedureNote":    {code: "28570-0", display: "Procedure note"},
}

// sectionTypeLoincCode provides the well-known LOINC codes for Composition
// section classification, keyed by FHIR resource type.
var sectionTypeLoincCode = map[string]loincEntry{
	"AllergyIntolerance":      {code: "48765-2", display: "Allergies and adverse reactions Document"},
	"Condition":               {code: "11450-4", display: "Problem list - Reported"},
	"MedicationStatement":     {code: "10160-0", display: "History of Medication use Narrative"},
	"Immunization":            {code: "11369-6", display: "History of Immunization Narrative"},
	"Observation":             {code: "30954-2", display: "Relevant diagnostic tests/laboratory data Narrative"},
	"Procedure":               {code: "47519-4", display: "History of Procedures Document"},
	"Encounter":               {code: "46240-8", display: "History of Hospitalizations+Outpatient visits Narrative"},
	"DiagnosticReport":        {code: "30954-2", display: "Relevant diagnostic tests/laboratory data Narrative"},
	"DocumentReference":       {code: "34109-9", display: "Note"},
	"CarePlan":                {code: "18776-5", display: "Plan of care note"},
	"Goal":                    {code: "61146-7", display: "Goals"},
	"Device":                  {code: "46264-8", display: "History of medical device use"},
	"FamilyMemberHistory":     {code: "10157-6", display: "History of family member diseases Narrative"},
	"VitalSigns":              {code: "8716-3", display: "Vital signs"},
}

type loincEntry struct {
	code    string
	display string
}

// CompositionMapper builds a FHIR Composition resource from parsed CDA header
// data and the flat list of FHIR resources produced by GenericCDAFHIRMapper.
type CompositionMapper struct{}

// NewCompositionMapper constructs a CompositionMapper.
func NewCompositionMapper() *CompositionMapper {
	return &CompositionMapper{}
}

// BuildComposition returns a FHIR R4 Composition resource suitable for
// prepending to a document Bundle. The Composition references all produced
// clinical resources grouped by FHIR resource type as sections.
//
// headerMap is parsedCDA["header"]; allResources is the flat slice of all FHIR
// resources produced (Patient/Practitioner/Organization included — they are
// referenced as subject/author/custodian, not as section entries).
func (cm *CompositionMapper) BuildComposition(
	headerMap map[string]interface{},
	allResources []map[string]interface{},
	config CDAToFHIRConfig,
) map[string]interface{} {
	comp := map[string]interface{}{
		"resourceType": "Composition",
		"id":           "composition-1",
		"status":       "final",
	}

	// type — document-level LOINC code
	loinc := documentTypeLoincCode[config.DocType]
	if loinc.code == "" {
		loinc = documentTypeLoincCode["CCD"]
	}
	comp["type"] = buildCodeableConcept("http://loinc.org", loinc.code, loinc.display)

	// date — prefer CDA effectiveTime, fall back to now
	comp["date"] = cm.extractEffectiveDate(headerMap)

	// title
	comp["title"] = cm.extractTitle(headerMap, config.DocType)

	// subject — always Patient/patient-1 (all C-CDA docs are patient-centric)
	comp["subject"] = map[string]interface{}{"reference": "Patient/patient-1"}

	// author — Practitioner if produced, otherwise omit
	if cm.hasResource(allResources, "Practitioner") {
		comp["author"] = []interface{}{
			map[string]interface{}{"reference": "Practitioner/author-1"},
		}
	}

	// custodian — Organization if produced
	if cm.hasResource(allResources, "Organization") {
		comp["custodian"] = map[string]interface{}{"reference": "Organization/custodian-1"}
	}

	// confidentiality — from CDA header if present, otherwise "N" (Normal)
	comp["confidentiality"] = cm.extractConfidentiality(headerMap)

	// US Core profile injection
	if config.ProfileMode != "base" {
		comp["meta"] = map[string]interface{}{
			"profile": []interface{}{
				"http://hl7.org/fhir/us/ccda/StructureDefinition/US-Realm-Header",
			},
		}
	}

	// Build sections: one section per unique clinical resource type
	sections := cm.buildSections(allResources)
	if len(sections) > 0 {
		comp["section"] = sections
	}

	return comp
}

// -------------------------------------------------------
// private helpers
// -------------------------------------------------------

func (cm *CompositionMapper) extractEffectiveDate(header map[string]interface{}) string {
	if header == nil {
		return time.Now().UTC().Format(time.RFC3339)
	}
	if ts, ok := header["effectiveTime"].(string); ok && ts != "" {
		return FormatDate(ts)
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func (cm *CompositionMapper) extractTitle(header map[string]interface{}, docType string) string {
	if header != nil {
		if t, ok := header["title"].(string); ok && t != "" {
			return t
		}
	}
	// Fallback: human-readable doc type name
	titles := map[string]string{
		"CCD":              "Continuity of Care Document",
		"DischargeSummary": "Discharge Summary",
		"ReferralNote":     "Referral Note",
		"HistoryPhysical":  "History and Physical",
		"ConsultationNote": "Consultation Note",
		"ProgressNote":     "Progress Note",
		"TransferSummary":  "Transfer Summary",
		"ProcedureNote":    "Procedure Note",
	}
	if t, ok := titles[docType]; ok {
		return t
	}
	return "Clinical Document"
}

func (cm *CompositionMapper) extractConfidentiality(header map[string]interface{}) string {
	if header == nil {
		return "N"
	}
	if c, ok := header["confidentialityCode"].(string); ok && c != "" {
		switch c {
		case "N", "R", "V":
			return c
		}
	}
	return "N"
}

// hasResource returns true if allResources contains at least one resource of
// the given resourceType.
func (cm *CompositionMapper) hasResource(allResources []map[string]interface{}, resourceType string) bool {
	for _, r := range allResources {
		if strField(r, "resourceType") == resourceType {
			return true
		}
	}
	return false
}

// buildSections groups clinical resources by type and creates Composition.section
// entries. Header resources (Patient, Practitioner, Organization) are excluded
// because they appear as subject/author/custodian on the Composition itself.
func (cm *CompositionMapper) buildSections(allResources []map[string]interface{}) []interface{} {
	headerTypes := map[string]bool{
		"Patient":      true,
		"Practitioner": true,
		"Organization": true,
	}

	// Collect refs per resource type (stable insertion-order map via ordered keys)
	type sectionData struct {
		refs []string
	}
	sections := map[string]*sectionData{}
	var sectionOrder []string

	for _, r := range allResources {
		rt := strField(r, "resourceType")
		if headerTypes[rt] {
			continue
		}
		id := strField(r, "id")
		ref := fmt.Sprintf("%s/%s", rt, id)

		if _, exists := sections[rt]; !exists {
			sections[rt] = &sectionData{}
			sectionOrder = append(sectionOrder, rt)
		}
		sections[rt].refs = append(sections[rt].refs, ref)
	}

	result := make([]interface{}, 0, len(sectionOrder))
	for _, rt := range sectionOrder {
		sd := sections[rt]
		entries := make([]interface{}, 0, len(sd.refs))
		for _, ref := range sd.refs {
			entries = append(entries, map[string]interface{}{"reference": ref})
		}

		sec := map[string]interface{}{}

		loinc := sectionTypeLoincCode[rt]
		if loinc.code != "" {
			sec["title"] = loinc.display
			sec["code"] = buildCodeableConcept("http://loinc.org", loinc.code, loinc.display)
		} else {
			sec["title"] = rt
		}

		sec["entry"] = entries
		result = append(result, sec)
	}

	return result
}

// buildCodeableConcept is a local helper to avoid a cross-file dependency for
// this simple pattern. GenericMapper uses the same shape inline.
func buildCodeableConcept(system, code, display string) map[string]interface{} {
	coding := map[string]interface{}{
		"system": system,
		"code":   code,
	}
	if display != "" {
		coding["display"] = display
	}
	return map[string]interface{}{
		"coding": []interface{}{coding},
	}
}
