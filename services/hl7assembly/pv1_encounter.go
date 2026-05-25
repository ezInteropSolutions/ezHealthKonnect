// services/hl7assembly/pv1_encounter.go
//
// Optional PV1 → FHIR Encounter assembler.
//
// Called by the transform service when the interface has enabled the
// "PV1_Encounter" optional segment block.  It is NOT called on the default
// ORU^R01 path — Encounter is opt-in for message types that don't produce it
// in the standard OOB template.
//
// Mapped fields:
//
//	PV1.2  → class (HL7 Table 0004 code → FHIR v3 ActCode system)
//	PV1.3  → location[0].location (facility/room/bed)
//	PV1.7  → participant[0] (attending physician, type ATT)
//	PV1.19 → identifier[0] (visit number)
//	PV1.44 → period.start (admit date/time)
//	PV1.45 → period.end   (discharge date/time, omitted when empty)
//	PV1.18 → type[0]      (patient type, e.g. emergency/elective)
//
// Returns nil when no PV1 segment is present in parsedHL7Data.
package hl7assembly

import (
	"strings"

	"github.com/google/uuid"
)

// pv1ClassToFHIR maps HL7 Table 0004 patient class codes to FHIR v3
// ActCode values used in Encounter.class.
var pv1ClassToFHIR = map[string]struct{ code, display string }{
	"I": {code: "IMP", display: "inpatient encounter"},
	"O": {code: "AMB", display: "ambulatory"},
	"E": {code: "EMER", display: "emergency"},
	"P": {code: "PRENC", display: "pre-admission"},
	"R": {code: "AMB", display: "ambulatory"},    // recurring patient
	"B": {code: "AMB", display: "ambulatory"},    // obstetrics
	"C": {code: "AMB", display: "ambulatory"},    // commercial account
	"N": {code: "NEWB", display: "newborn"},
	"U": {code: "AMB", display: "ambulatory"},    // unknown
}

const v3ActCodeSystem = "http://terminology.hl7.org/CodeSystem/v3-ActCode"

// AssemblePV1Encounter builds a minimal FHIR Encounter from PV1 segment data.
// patientRef is the logical "Patient/<id>" reference for Encounter.subject.
// Returns (resource, id) or (nil, "") when PV1 is absent.
func AssemblePV1Encounter(
	parsedHL7Data map[string]interface{},
	patientRef string,
) (map[string]interface{}, string) {

	pv1List := ExtractSegmentGroup(parsedHL7Data, "PV1")
	if len(pv1List) == 0 {
		return nil, ""
	}
	pv1 := pv1List[0]

	enc := map[string]interface{}{
		"resourceType": "Encounter",
		"id":           uuid.NewString(),
	}

	// ── class ──────────────────────────────────────────────────────────────────
	classCode := strings.TrimSpace(SegFieldValue(pv1, "PV1.2"))
	if classCode != "" {
		if fhir, ok := pv1ClassToFHIR[strings.ToUpper(classCode)]; ok {
			enc["class"] = map[string]interface{}{
				"system":  v3ActCodeSystem,
				"code":    fhir.code,
				"display": fhir.display,
			}
		} else {
			enc["class"] = map[string]interface{}{
				"code": classCode,
			}
		}
	} else {
		// FHIR Encounter.class is required (1..1)
		enc["class"] = map[string]interface{}{
			"system":  v3ActCodeSystem,
			"code":    "AMB",
			"display": "ambulatory",
		}
	}

	// ── status — default to "finished" (most ORU messages are post-visit) ──────
	enc["status"] = "finished"

	// ── subject ────────────────────────────────────────────────────────────────
	if patientRef != "" {
		enc["subject"] = map[string]interface{}{"reference": patientRef}
	}

	// ── identifier (visit number from PV1.19) ──────────────────────────────────
	visitNum := strings.TrimSpace(SegFieldValue(pv1, "PV1.19"))
	if visitNum != "" {
		enc["identifier"] = []interface{}{
			map[string]interface{}{
				"type": map[string]interface{}{
					"coding": []interface{}{
						map[string]interface{}{
							"system":  HL7V2IdentifierTypeSystem,
							"code":    "VN",
							"display": "Visit number",
						},
					},
				},
				"value": visitNum,
			},
		}
	}

	// ── period ─────────────────────────────────────────────────────────────────
	admitRaw := strings.TrimSpace(SegFieldValue(pv1, "PV1.44"))
	dischRaw := strings.TrimSpace(SegFieldValue(pv1, "PV1.45"))
	if admitRaw != "" || dischRaw != "" {
		period := map[string]interface{}{}
		if admitRaw != "" {
			period["start"] = ToISO(admitRaw)
		}
		if dischRaw != "" {
			period["end"] = ToISO(dischRaw)
		}
		enc["period"] = period
	}

	// ── location (PV1.3 — point-of-care^room^bed^facility) ────────────────────
	loc := strings.TrimSpace(SegFieldValue(pv1, "PV1.3"))
	if loc != "" {
		// PL (Point of Location) composite — first component is point-of-care
		parts := strings.SplitN(loc, "^", 4)
		display := parts[0]
		if len(parts) >= 4 && parts[3] != "" {
			display = parts[3] + " / " + parts[0]
		}
		enc["location"] = []interface{}{
			map[string]interface{}{
				"location": map[string]interface{}{
					"display": display,
				},
			},
		}
	}

	// ── participant — attending physician (PV1.7) ──────────────────────────────
	// PV1.7 is XCN: ID^family^given^...
	attending := strings.TrimSpace(SegFieldValue(pv1, "PV1.7"))
	if attending != "" {
		parts := strings.SplitN(attending, "^", 5)
		practID := ""
		if len(parts) >= 1 {
			practID = parts[0]
		}
		if practID != "" {
			enc["participant"] = []interface{}{
				map[string]interface{}{
					"type": []interface{}{
						map[string]interface{}{
							"coding": []interface{}{
								map[string]interface{}{
									"system":  "http://terminology.hl7.org/CodeSystem/v3-ParticipationType",
									"code":    "ATND",
									"display": "attender",
								},
							},
						},
					},
					"individual": map[string]interface{}{
						"reference": "Practitioner/" + practID,
					},
				},
			}
		}
	}

	// ── type (PV1.18 — patient type) ───────────────────────────────────────────
	patType := strings.TrimSpace(SegFieldValue(pv1, "PV1.18"))
	if patType != "" {
		enc["type"] = []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"code":    patType,
						"display": patType,
					},
				},
			},
		}
	}

	encID, _ := enc["id"].(string)
	return enc, encID
}
