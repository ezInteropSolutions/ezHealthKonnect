// services/cda_fhir/narrative_section_mapper.go
//
// Handles CDA sections that carry clinical content exclusively in their
// narrative <text> block, with no structured <entry> elements. These sections
// are mapped to FHIR DocumentReference resources via a dedicated section-level
// pass in DeclarativeMapDocument (see the "section-level narrative" block there),
// NOT the entry-dispatch loop, because the source data is section-level
// (CDASection.LoincCode / .Title / .NarrativeText) rather than per-entry.
//
// This mirrors the pattern already used for header resources (Patient, Author,
// Custodian) — dedicated Go helpers, not MappingRule + Fields — because the
// FHIR output shape is fixed (always DocumentReference) and there is no
// per-entry polymorphism to express declaratively.
package cdafhir

import (
	"encoding/base64"

	cdadocument "ezhealthkonnect/cda/document"
)

// genericNoteLoincCode and genericNoteDisplayName are the last-resort fallback
// used when a narrative-only section has neither its own <code>/@code nor a
// DefaultNarrativeSectionDefs entry to fall back to (an unrecognized section
// key in a document that also omits the LOINC on the <code> element itself —
// rare, but the section's narrative content must not be silently dropped just
// because it can't be classified more specifically). Same code C-CDA's own
// Note Activity template uses for a generic clinical note — see
// noteActivityTemplateID's doc comment in declarative_oob_rules.go.
const (
	genericNoteLoincCode   = "34109-9"
	genericNoteDisplayName = "Note"
)

// buildNarrativeDocRef builds a FHIR R4 DocumentReference from a CDA section's
// own narrative text block. Called once per matched narrative-only section (no
// entries, non-empty NarrativeText) from DeclarativeMapDocument, for EVERY
// section in the document — not just ones with a DefaultNarrativeSectionDefs
// entry; def is the zero value for unregistered section keys.
//
// FHIR shape (US Core v9.0.0 / Argonaut clinical notes guidance):
//
//	DocumentReference {
//	  status: "current"
//	  type.coding[0]: { system: http://loinc.org, code: <LOINC>, display: <title> }
//	  category[0]:    clinical-note (US Core category)
//	  subject:        { reference: <patientRef> }
//	  description:    <section.Title>
//	  content[0].attachment: { contentType: "text/html", data: <base64(NarrativeText)> }
//	}
func (m *GenericCDAFHIRMapper) buildNarrativeDocRef(
	section *cdadocument.CDASection,
	def NarrativeSectionDef,
	patientRef string,
) map[string]interface{} {
	// Section's own LoincCode/Title win over the registry fallback -- the CDA
	// parser populates both from the <code>/@code and <title> elements, which
	// are present in every real-world corpus file examined.
	loinc := section.LoincCode
	if loinc == "" {
		loinc = def.LoincCode
	}
	display := section.Title
	if display == "" {
		display = def.DisplayName
	}
	// Final fallback: neither the section nor the registry has a LOINC code
	// (unrecognized section key + source document omitted <code>/@code).
	if loinc == "" {
		loinc = genericNoteLoincCode
	}
	if display == "" {
		display = genericNoteDisplayName
	}

	docRef := map[string]interface{}{
		"resourceType": "DocumentReference",
		"status":       "current",
		"type": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system":  "http://loinc.org",
					"code":    loinc,
					"display": display,
				},
			},
			"text": display,
		},
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  "http://hl7.org/fhir/us/core/CodeSystem/us-core-documentreference-category",
						"code":    "clinical-note",
						"display": "Clinical Note",
					},
				},
			},
		},
		"content": []interface{}{
			map[string]interface{}{
				"attachment": map[string]interface{}{
					"contentType": "text/html",
					"data":        base64.StdEncoding.EncodeToString([]byte(section.NarrativeText)),
				},
			},
		},
	}

	if patientRef != "" {
		docRef["subject"] = map[string]interface{}{"reference": patientRef}
	}
	if section.Title != "" {
		docRef["description"] = section.Title
	}

	return docRef
}
