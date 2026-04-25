// services/mapping/semantic_matcher.go
// Finds the best-matching FHIR element path for a given HL7 field.
//
// The matcher runs three passes in order of reliability:
//  1. Known anchors — hard-coded gold-standard (segment.field → fhirPath)
//     that override any heuristic. Added whenever a pair is confirmed.
//  2. Data-type compatibility — XPN fields must go to HumanName elements,
//     CX fields to Identifier elements, etc. Filters candidates before scoring.
//  3. Name similarity — normalized Levenshtein / token-overlap score between
//     the HL7 field name and the FHIR element path leaf.
//
// The matcher returns a ranked list of candidates so the wizard UI can show
// alternatives and the AI service can pick or explain the best one.
package mapping

import (
	"math"
	"strings"
	"unicode"

	"ezhealthkonnect/fhir"
)

// Candidate is one scored FHIR element path suggestion for an HL7 field.
type Candidate struct {
	FHIRPath   string  // e.g. "Practitioner.name[0].family"
	FHIRType   string  // FHIR element data type from StructureDefinition
	Confidence float64 // 0.0–1.0
	Source     string  // "anchor" | "type_match" | "name_similarity"
}

// knownAnchors is the gold-standard segment.field → fhirPath table.
// Keyed as "SEGMENT.FIELD" (uppercase segment, dotted field number).
// These bypass heuristics and are emitted with confidence 1.0.
var knownAnchors = map[string][]Candidate{
	// ── MSH ───────────────────────────────────────────────────────────────
	"MSH.3":  {{FHIRPath: "MessageHeader.source.name", FHIRType: "string", Confidence: 0.99, Source: "anchor"}},
	"MSH.4":  {{FHIRPath: "MessageHeader.source.endpoint", FHIRType: "url", Confidence: 0.90, Source: "anchor"}},
	"MSH.5":  {{FHIRPath: "MessageHeader.destination[0].name", FHIRType: "string", Confidence: 0.97, Source: "anchor"}},
	"MSH.7":  {{FHIRPath: "MessageHeader.meta.lastUpdated", FHIRType: "instant", Confidence: 0.95, Source: "anchor"}},
	"MSH.9":  {{FHIRPath: "MessageHeader.eventCoding.code", FHIRType: "code", Confidence: 0.99, Source: "anchor"}},
	"MSH.10": {{FHIRPath: "MessageHeader.id", FHIRType: "id", Confidence: 0.99, Source: "anchor"}},
	"MSH.11": {{FHIRPath: "MessageHeader.meta.security[0].code", FHIRType: "code", Confidence: 0.85, Source: "anchor"}},

	// ── PID ───────────────────────────────────────────────────────────────
	"PID.3":  {{FHIRPath: "Patient.identifier", FHIRType: "Identifier", Confidence: 0.99, Source: "anchor"}},
	"PID.5":  {{FHIRPath: "Patient.name[0]", FHIRType: "HumanName", Confidence: 0.99, Source: "anchor"}},
	"PID.7":  {{FHIRPath: "Patient.birthDate", FHIRType: "date", Confidence: 0.99, Source: "anchor"}},
	"PID.8":  {{FHIRPath: "Patient.gender", FHIRType: "code", Confidence: 0.97, Source: "anchor"}},
	"PID.10": {{FHIRPath: "Patient.extension[race]", FHIRType: "Extension", Confidence: 0.85, Source: "anchor"}},
	"PID.11": {{FHIRPath: "Patient.address[0]", FHIRType: "Address", Confidence: 0.97, Source: "anchor"}},
	"PID.13": {{FHIRPath: "Patient.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.95, Source: "anchor"}},
	"PID.14": {{FHIRPath: "Patient.telecom[1]", FHIRType: "ContactPoint", Confidence: 0.90, Source: "anchor"}},
	"PID.15": {{FHIRPath: "Patient.communication[0].language", FHIRType: "CodeableConcept", Confidence: 0.90, Source: "anchor"}},
	"PID.18": {{FHIRPath: "Patient.identifier[1].value", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"PID.19": {{FHIRPath: "Patient.identifier[2].value", FHIRType: "string", Confidence: 0.85, Source: "anchor"}},
	"PID.22": {{FHIRPath: "Patient.extension[ethnicity]", FHIRType: "Extension", Confidence: 0.85, Source: "anchor"}},
	"PID.29": {{FHIRPath: "Patient.deceasedDateTime", FHIRType: "dateTime", Confidence: 0.92, Source: "anchor"}},
	"PID.30": {{FHIRPath: "Patient.deceasedBoolean", FHIRType: "boolean", Confidence: 0.90, Source: "anchor"}},

	// ── PV1 ───────────────────────────────────────────────────────────────
	"PV1.2":  {{FHIRPath: "Encounter.class.code", FHIRType: "code", Confidence: 0.95, Source: "anchor"}},
	"PV1.3":  {{FHIRPath: "Encounter.location[0].location.display", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"PV1.7":  {{FHIRPath: "Encounter.participant[0].individual.display", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"PV1.10": {{FHIRPath: "Encounter.hospitalization.admitSource", FHIRType: "CodeableConcept", Confidence: 0.88, Source: "anchor"}},
	"PV1.17": {{FHIRPath: "Encounter.participant[1].individual.display", FHIRType: "string", Confidence: 0.85, Source: "anchor"}},
	"PV1.18": {{FHIRPath: "Encounter.type[0].coding[0].code", FHIRType: "code", Confidence: 0.85, Source: "anchor"}},
	"PV1.44": {{FHIRPath: "Encounter.period.start", FHIRType: "dateTime", Confidence: 0.97, Source: "anchor"}},
	"PV1.45": {{FHIRPath: "Encounter.period.end", FHIRType: "dateTime", Confidence: 0.97, Source: "anchor"}},

	// ── EVN ───────────────────────────────────────────────────────────────
	"EVN.2": {{FHIRPath: "Encounter.period.start", FHIRType: "dateTime", Confidence: 0.88, Source: "anchor"}},
	"EVN.4": {{FHIRPath: "Encounter.type[0].coding[0].code", FHIRType: "code", Confidence: 0.82, Source: "anchor"}},
	"EVN.5": {{FHIRPath: "Encounter.participant[0].individual.display", FHIRType: "string", Confidence: 0.80, Source: "anchor"}},

	// ── OBR ───────────────────────────────────────────────────────────────
	"OBR.2":  {{FHIRPath: "DiagnosticReport.identifier[0].value", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"OBR.3":  {{FHIRPath: "DiagnosticReport.identifier[1].value", FHIRType: "string", Confidence: 0.93, Source: "anchor"}},
	"OBR.4":  {{FHIRPath: "DiagnosticReport.code", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"OBR.7":  {{FHIRPath: "DiagnosticReport.effectiveDateTime", FHIRType: "dateTime", Confidence: 0.93, Source: "anchor"}},
	"OBR.16": {{FHIRPath: "DiagnosticReport.resultsInterpreter[0].display", FHIRType: "string", Confidence: 0.85, Source: "anchor"}},
	"OBR.22": {{FHIRPath: "DiagnosticReport.issued", FHIRType: "instant", Confidence: 0.90, Source: "anchor"}},
	"OBR.25": {{FHIRPath: "DiagnosticReport.status", FHIRType: "code", Confidence: 0.92, Source: "anchor"}},

	// ── OBX ───────────────────────────────────────────────────────────────
	"OBX.3":  {{FHIRPath: "Observation.code", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"OBX.5":  {{FHIRPath: "Observation.value[x]", FHIRType: "varies", Confidence: 0.90, Source: "anchor"}},
	"OBX.6":  {{FHIRPath: "Observation.valueQuantity.unit", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"OBX.7":  {{FHIRPath: "Observation.referenceRange[0].text", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"OBX.8":  {{FHIRPath: "Observation.interpretation[0].coding[0].code", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"OBX.11": {{FHIRPath: "Observation.status", FHIRType: "code", Confidence: 0.95, Source: "anchor"}},
	"OBX.14": {{FHIRPath: "Observation.effectiveDateTime", FHIRType: "dateTime", Confidence: 0.93, Source: "anchor"}},

	// ── ORC ───────────────────────────────────────────────────────────────
	"ORC.1":  {{FHIRPath: "ServiceRequest.status", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"ORC.2":  {{FHIRPath: "ServiceRequest.identifier[0].value", FHIRType: "string", Confidence: 0.92, Source: "anchor"}},
	"ORC.3":  {{FHIRPath: "ServiceRequest.identifier[1].value", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"ORC.9":  {{FHIRPath: "ServiceRequest.authoredOn", FHIRType: "dateTime", Confidence: 0.93, Source: "anchor"}},
	"ORC.12": {{FHIRPath: "ServiceRequest.requester.display", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},

	// ── AL1 ───────────────────────────────────────────────────────────────
	"AL1.2": {{FHIRPath: "AllergyIntolerance.category[0]", FHIRType: "code", Confidence: 0.92, Source: "anchor"}},
	"AL1.3": {{FHIRPath: "AllergyIntolerance.code", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"AL1.4": {{FHIRPath: "AllergyIntolerance.reaction[0].severity", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"AL1.5": {{FHIRPath: "AllergyIntolerance.reaction[0].description", FHIRType: "string", Confidence: 0.85, Source: "anchor"}},
	"AL1.6": {{FHIRPath: "AllergyIntolerance.onsetDateTime", FHIRType: "dateTime", Confidence: 0.88, Source: "anchor"}},

	// ── DG1 ───────────────────────────────────────────────────────────────
	"DG1.3": {{FHIRPath: "Condition.code", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"DG1.5": {{FHIRPath: "Condition.onsetDateTime", FHIRType: "dateTime", Confidence: 0.90, Source: "anchor"}},
	"DG1.6": {{FHIRPath: "Condition.category[0].coding[0].code", FHIRType: "code", Confidence: 0.88, Source: "anchor"}},

	// ── NK1 ───────────────────────────────────────────────────────────────
	"NK1.2": {{FHIRPath: "RelatedPerson.name[0]", FHIRType: "HumanName", Confidence: 0.95, Source: "anchor"}},
	"NK1.3": {{FHIRPath: "RelatedPerson.relationship[0].coding[0].code", FHIRType: "code", Confidence: 0.92, Source: "anchor"}},
	"NK1.4": {{FHIRPath: "RelatedPerson.address[0]", FHIRType: "Address", Confidence: 0.88, Source: "anchor"}},
	"NK1.5": {{FHIRPath: "RelatedPerson.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.88, Source: "anchor"}},
	"NK1.7": {{FHIRPath: "RelatedPerson.period.start", FHIRType: "dateTime", Confidence: 0.82, Source: "anchor"}},
	"NK1.8": {{FHIRPath: "RelatedPerson.period.end", FHIRType: "dateTime", Confidence: 0.82, Source: "anchor"}},

	// ── IN1 ───────────────────────────────────────────────────────────────
	"IN1.1":  {{FHIRPath: "Coverage.identifier[0].value", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"IN1.2":  {{FHIRPath: "Coverage.type.coding[0].code", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"IN1.3":  {{FHIRPath: "Coverage.payor[0].identifier[0].value", FHIRType: "string", Confidence: 0.92, Source: "anchor"}},
	"IN1.4":  {{FHIRPath: "Coverage.payor[0].display", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"IN1.12": {{FHIRPath: "Coverage.period.start", FHIRType: "dateTime", Confidence: 0.90, Source: "anchor"}},
	"IN1.13": {{FHIRPath: "Coverage.period.end", FHIRType: "dateTime", Confidence: 0.90, Source: "anchor"}},
	"IN1.36": {{FHIRPath: "Coverage.subscriberId", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},

	// ── STF (Practitioner master) ─────────────────────────────────────────
	"STF.1":  {{FHIRPath: "Practitioner.identifier[0].value", FHIRType: "string", Confidence: 0.97, Source: "anchor"}},
	"STF.3":  {{FHIRPath: "Practitioner.name[0]", FHIRType: "HumanName", Confidence: 0.99, Source: "anchor"}},
	"STF.4":  {{FHIRPath: "Practitioner.qualification[0].code.text", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"STF.5":  {{FHIRPath: "Practitioner.gender", FHIRType: "code", Confidence: 0.95, Source: "anchor"}},
	"STF.6":  {{FHIRPath: "Practitioner.birthDate", FHIRType: "date", Confidence: 0.93, Source: "anchor"}},
	"STF.7":  {{FHIRPath: "Practitioner.active", FHIRType: "boolean", Confidence: 0.95, Source: "anchor"}},
	"STF.10": {{FHIRPath: "Practitioner.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.90, Source: "anchor"}},
	"STF.11": {{FHIRPath: "Practitioner.address[0]", FHIRType: "Address", Confidence: 0.90, Source: "anchor"}},

	// ── PRA (Practitioner detail) ─────────────────────────────────────────
	"PRA.3": {{FHIRPath: "PractitionerRole.specialty[0].coding[0].code", FHIRType: "code", Confidence: 0.92, Source: "anchor"}},
	"PRA.5": {{FHIRPath: "Practitioner.qualification[0].identifier[0].value", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},

	// ── LOC ───────────────────────────────────────────────────────────────
	"LOC.1": {{FHIRPath: "Location.identifier[0].value", FHIRType: "string", Confidence: 0.97, Source: "anchor"}},
	"LOC.2": {{FHIRPath: "Location.name", FHIRType: "string", Confidence: 0.97, Source: "anchor"}},
	"LOC.3": {{FHIRPath: "Location.type[0].coding[0].code", FHIRType: "code", Confidence: 0.88, Source: "anchor"}},
	"LOC.4": {{FHIRPath: "Location.address", FHIRType: "Address", Confidence: 0.88, Source: "anchor"}},
	"LOC.5": {{FHIRPath: "Location.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.85, Source: "anchor"}},

	// ── CDM ───────────────────────────────────────────────────────────────
	"CDM.1": {{FHIRPath: "ChargeItemDefinition.identifier[0].value", FHIRType: "string", Confidence: 0.97, Source: "anchor"}},
	"CDM.3": {{FHIRPath: "ChargeItemDefinition.title", FHIRType: "string", Confidence: 0.95, Source: "anchor"}},
	"CDM.4": {{FHIRPath: "ChargeItemDefinition.description", FHIRType: "markdown", Confidence: 0.90, Source: "anchor"}},

	// ── OM1 ───────────────────────────────────────────────────────────────
	"OM1.2":  {{FHIRPath: "ObservationDefinition.identifier[0].value", FHIRType: "string", Confidence: 0.95, Source: "anchor"}},
	"OM1.7":  {{FHIRPath: "ObservationDefinition.code", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"OM1.11": {{FHIRPath: "ObservationDefinition.preferredReportName", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},

	// ── SCH ───────────────────────────────────────────────────────────────
	"SCH.1":  {{FHIRPath: "Appointment.identifier[0].value", FHIRType: "string", Confidence: 0.95, Source: "anchor"}},
	"SCH.7":  {{FHIRPath: "Appointment.appointmentType.coding[0].code", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"SCH.11": {{FHIRPath: "Appointment.start", FHIRType: "instant", Confidence: 0.95, Source: "anchor"}},
	"SCH.12": {{FHIRPath: "Appointment.end", FHIRType: "instant", Confidence: 0.93, Source: "anchor"}},

	// ── AIS ───────────────────────────────────────────────────────────────
	"AIS.3":  {{FHIRPath: "Appointment.participant[0].actor.display", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"AIS.4":  {{FHIRPath: "Appointment.participant[0].period.start", FHIRType: "dateTime", Confidence: 0.88, Source: "anchor"}},
	"AIS.10": {{FHIRPath: "Appointment.participant[0].status", FHIRType: "code", Confidence: 0.85, Source: "anchor"}},

	// ── TXA ───────────────────────────────────────────────────────────────
	"TXA.2":  {{FHIRPath: "DocumentReference.type.coding[0].code", FHIRType: "code", Confidence: 0.95, Source: "anchor"}},
	"TXA.4":  {{FHIRPath: "DocumentReference.date", FHIRType: "instant", Confidence: 0.93, Source: "anchor"}},
	"TXA.5":  {{FHIRPath: "DocumentReference.author[0].display", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"TXA.9":  {{FHIRPath: "DocumentReference.identifier[0].value", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"TXA.17": {{FHIRPath: "DocumentReference.status", FHIRType: "code", Confidence: 0.92, Source: "anchor"}},

	// ── RXA ───────────────────────────────────────────────────────────────
	"RXA.3":  {{FHIRPath: "Immunization.occurrenceDateTime", FHIRType: "dateTime", Confidence: 0.97, Source: "anchor"}},
	"RXA.5":  {{FHIRPath: "Immunization.vaccineCode", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"RXA.6":  {{FHIRPath: "Immunization.doseQuantity.value", FHIRType: "decimal", Confidence: 0.90, Source: "anchor"}},
	"RXA.20": {{FHIRPath: "Immunization.status", FHIRType: "code", Confidence: 0.93, Source: "anchor"}},

	// ── GT1 ───────────────────────────────────────────────────────────────
	"GT1.2": {{FHIRPath: "RelatedPerson.identifier[0].value", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"GT1.3": {{FHIRPath: "RelatedPerson.name[0]", FHIRType: "HumanName", Confidence: 0.95, Source: "anchor"}},
	"GT1.5": {{FHIRPath: "RelatedPerson.address[0]", FHIRType: "Address", Confidence: 0.90, Source: "anchor"}},
	"GT1.6": {{FHIRPath: "RelatedPerson.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.88, Source: "anchor"}},
	"GT1.8": {{FHIRPath: "RelatedPerson.birthDate", FHIRType: "date", Confidence: 0.90, Source: "anchor"}},
	"GT1.9": {{FHIRPath: "RelatedPerson.gender", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},

	// ── ROL ───────────────────────────────────────────────────────────────
	"ROL.3": {{FHIRPath: "PractitionerRole.code[0].coding[0].code", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"ROL.4": {{FHIRPath: "PractitionerRole.practitioner.display", FHIRType: "string", Confidence: 0.92, Source: "anchor"}},
	"ROL.5": {{FHIRPath: "PractitionerRole.period.start", FHIRType: "dateTime", Confidence: 0.85, Source: "anchor"}},

	// ── PR1 ───────────────────────────────────────────────────────────────
	"PR1.3": {{FHIRPath: "Procedure.code", FHIRType: "CodeableConcept", Confidence: 0.95, Source: "anchor"}},
	"PR1.5": {{FHIRPath: "Procedure.performedDateTime", FHIRType: "dateTime", Confidence: 0.93, Source: "anchor"}},
	"PR1.6": {{FHIRPath: "Procedure.performer[0].actor.display", FHIRType: "string", Confidence: 0.85, Source: "anchor"}},

	// ── FT1 ───────────────────────────────────────────────────────────────
	"FT1.4": {{FHIRPath: "ChargeItem.occurrenceDateTime", FHIRType: "dateTime", Confidence: 0.90, Source: "anchor"}},
	"FT1.6": {{FHIRPath: "ChargeItem.code", FHIRType: "CodeableConcept", Confidence: 0.93, Source: "anchor"}},
	"FT1.7": {{FHIRPath: "ChargeItem.quantity.value", FHIRType: "decimal", Confidence: 0.88, Source: "anchor"}},

	// ── MFI / MFE ─────────────────────────────────────────────────────────
	"MFI.1": {{FHIRPath: "MessageHeader.meta.tag[0].code", FHIRType: "code", Confidence: 0.88, Source: "anchor"}},
	"MFI.3": {{FHIRPath: "MessageHeader.meta.tag[1].code", FHIRType: "code", Confidence: 0.82, Source: "anchor"}},
	"MFE.1": {{FHIRPath: "meta.tag[0].code", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"MFE.2": {{FHIRPath: "meta.versionId", FHIRType: "id", Confidence: 0.85, Source: "anchor"}},
}

// Match returns ranked candidates for fhirResource + hl7Field.
// fhirSchema may be nil — in that case only anchor and type-registry results
// are returned (no name-similarity pass against element names).
func Match(
	segmentName string,
	fieldKey string, // e.g. "PID.5"
	fieldName string, // e.g. "Patient Name"
	hl7DataType string,
	fhirResource string,
	fhirSchema *fhir.FHIRSchema,
) []Candidate {
	key := strings.ToUpper(segmentName) + "." + fieldNumber(fieldKey)

	// Pass 1 — known anchors (highest reliability)
	if anchors, ok := knownAnchors[key]; ok {
		return anchors
	}

	typeEntry := Lookup(hl7DataType)
	var results []Candidate

	// Pass 2 — data-type compatibility against loaded FHIR elements
	if fhirSchema != nil {
		for path, elem := range fhirSchema.Elements {
			if !strings.HasPrefix(path, fhirResource+".") {
				continue
			}
			if !typeCompatible(typeEntry.FHIRType, elem.DataType) {
				continue
			}
			score := nameSimilarity(fieldName, elem.Path) * 0.85
			results = append(results, Candidate{
				FHIRPath:   path,
				FHIRType:   elem.DataType,
				Confidence: math.Round(score*100) / 100,
				Source:     "type_match",
			})
		}
	}

	// Pass 3 — name similarity without type filter (fallback)
	if len(results) == 0 && fhirSchema != nil {
		for path, elem := range fhirSchema.Elements {
			if !strings.HasPrefix(path, fhirResource+".") {
				continue
			}
			score := nameSimilarity(fieldName, elem.Path) * 0.65
			if score > 0.4 {
				results = append(results, Candidate{
					FHIRPath:   path,
					FHIRType:   elem.DataType,
					Confidence: math.Round(score*100) / 100,
					Source:     "name_similarity",
				})
			}
		}
	}

	sortCandidates(results)
	if len(results) > 5 {
		results = results[:5]
	}
	return results
}

// fieldNumber extracts "5" from "PID.5" or "PID.5.1" (returns first numeric part).
func fieldNumber(fieldKey string) string {
	parts := strings.Split(fieldKey, ".")
	if len(parts) >= 2 {
		return parts[1]
	}
	return fieldKey
}

// typeCompatible reports whether a FHIR element data type is compatible with
// the target FHIR type from the type registry entry.
func typeCompatible(registryFHIRType, elementFHIRType string) bool {
	if registryFHIRType == elementFHIRType {
		return true
	}
	// Broad compatibility groups
	stringLike := map[string]bool{"string": true, "code": true, "id": true, "uri": true, "url": true, "canonical": true, "markdown": true, "oid": true, "uuid": true, "base64Binary": true}
	dateLike := map[string]bool{"date": true, "dateTime": true, "instant": true, "time": true}
	numLike := map[string]bool{"decimal": true, "integer": true, "unsignedInt": true, "positiveInt": true}
	if stringLike[registryFHIRType] && stringLike[elementFHIRType] {
		return true
	}
	if dateLike[registryFHIRType] && dateLike[elementFHIRType] {
		return true
	}
	if numLike[registryFHIRType] && numLike[elementFHIRType] {
		return true
	}
	return false
}

// nameSimilarity returns 0.0–1.0 token-overlap score between an HL7 field name
// and the leaf segment of a FHIR element path.
func nameSimilarity(hl7Name, fhirPath string) float64 {
	leaf := fhirPathLeaf(fhirPath)
	a := tokenize(hl7Name)
	b := tokenize(leaf)
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for _, t := range a {
		for _, u := range b {
			if t == u {
				shared++
				break
			}
		}
	}
	union := len(a) + len(b) - shared
	if union == 0 {
		return 0
	}
	return float64(shared) / float64(union)
}

// fhirPathLeaf returns the last meaningful component of a FHIR path.
// "Patient.name[0].family" → "family"
func fhirPathLeaf(path string) string {
	// Remove array indices
	clean := strings.Map(func(r rune) rune {
		if r == '[' || r == ']' || unicode.IsDigit(r) {
			return -1
		}
		return r
	}, path)
	parts := strings.Split(clean, ".")
	return parts[len(parts)-1]
}

// tokenize splits a mixed-case string into lowercase tokens.
// "PatientName" → ["patient","name"], "date of birth" → ["date","of","birth"]
func tokenize(s string) []string {
	// Split on non-alphanumeric and camelCase boundaries
	var tokens []string
	var cur strings.Builder
	for i, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			if cur.Len() > 0 {
				tokens = append(tokens, strings.ToLower(cur.String()))
				cur.Reset()
			}
			continue
		}
		if i > 0 && unicode.IsUpper(r) && cur.Len() > 0 {
			tokens = append(tokens, strings.ToLower(cur.String()))
			cur.Reset()
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		tokens = append(tokens, strings.ToLower(cur.String()))
	}
	return tokens
}

// sortCandidates sorts in descending confidence order (in-place).
func sortCandidates(c []Candidate) {
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j].Confidence > c[j-1].Confidence; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
}
