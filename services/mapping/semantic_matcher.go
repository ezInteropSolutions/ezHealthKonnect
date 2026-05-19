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

// fhirPathBlocklist contains substrings of FHIR element paths that the
// heuristic passes (type_match and name_similarity) must never suggest as
// mapping targets for HL7v2 sources.
//
// Entries belong here when the HL7 V2-to-FHIR Implementation Guide
// (https://build.fhir.org/ig/HL7/v2-to-fhir/) defines no v2 field as a source
// for the element, or when the element requires information that v2 cannot
// supply (e.g. statusHistory requires a closed period with both start and end,
// something a single v2 event cannot provide).
//
// Anchors already take priority over heuristics (Pass 1 short-circuits), so
// only unanchored fields need this guard.  Add here whenever a new gap is
// identified from the IG ConceptMaps; remove only if the IG is updated to
// define a v2 source.
var fhirPathBlocklist = []string{
	"statusHistory",   // Encounter.statusHistory — no v2 source; requires period with start+end
	"episodeOfCare",   // Encounter.episodeOfCare — EpisodeOfCare is a FHIR-server concept; no v2 source
	"Patient.link",    // FHIR-server-managed record linkage; not populated by v2 transforms
	"implicitRules",   // Resource.implicitRules — FHIR infrastructure URL; no v2 source per IG
	"Coverage.language",   // Resource.language on Coverage — BCP-47 code; no IN1/IN2 v2 source per IG ConceptMaps
	"Coverage.dependent",  // Member sequence number assigned by insurer; no IN1/IN2 v2 source per IG ConceptMaps
}

// isBlocklisted returns true when fhirPath contains any blocklisted substring.
func isBlocklisted(fhirPath string) bool {
	for _, b := range fhirPathBlocklist {
		if strings.Contains(fhirPath, b) {
			return true
		}
	}
	return false
}

// knownAnchorsR4 is the gold-standard segment.field → FHIR R4 path table.
// Keyed as "SEGMENT.FIELD" (uppercase segment, dotted field number).
// These bypass heuristics entirely and are emitted at their stated confidence.
// Use anchorsForVersion() to get the right table for a given FHIR version.
var knownAnchorsR4 = map[string][]Candidate{
	// ── MSH ───────────────────────────────────────────────────────────────
	// MSH.1 "Field Separator" and MSH.2 "Encoding Characters" are skipped
	// by classifyField (FieldCategoryProtocolDelimiter) — no anchors needed.
	"MSH.3":  {{FHIRPath: "MessageHeader.source.name", FHIRType: "string", Confidence: 0.99, Source: "anchor"}},
	"MSH.4":  {{FHIRPath: "MessageHeader.source.endpoint", FHIRType: "url", Confidence: 0.90, Source: "anchor"}},
	"MSH.5":  {{FHIRPath: "MessageHeader.destination[0].name", FHIRType: "string", Confidence: 0.97, Source: "anchor"}},
	"MSH.6":  {{FHIRPath: "MessageHeader.destination[0].endpoint", FHIRType: "url", Confidence: 0.88, Source: "anchor"}},
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
	"PID.16": {{FHIRPath: "Patient.maritalStatus", FHIRType: "CodeableConcept", Confidence: 0.95, Source: "anchor"}},
	"PID.18": {{FHIRPath: "Patient.identifier[1]", FHIRType: "Identifier", Confidence: 0.88, Source: "anchor"}}, // CX composite → full Identifier
	"PID.19": {{FHIRPath: "Patient.identifier[2].value", FHIRType: "string", Confidence: 0.85, Source: "anchor"}},  // ST (plain string) → scalar .value is correct
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
	// EVN.1 (Event Type Code) is redundant with MSH.9 — the IG defines no
	// separate FHIR target for it.  Empty slice short-circuits Pass 1 so the
	// heuristic never runs and cannot match e.g. Encounter.statusHistory.
	"EVN.1": {},
	"EVN.2": {{FHIRPath: "Encounter.period.start", FHIRType: "dateTime", Confidence: 0.88, Source: "anchor"}},
	"EVN.4": {{FHIRPath: "Encounter.type[0].coding[0].code", FHIRType: "code", Confidence: 0.82, Source: "anchor"}},
	"EVN.5": {{FHIRPath: "Encounter.participant[0].individual.display", FHIRType: "string", Confidence: 0.80, Source: "anchor"}},

	// ── OBR ───────────────────────────────────────────────────────────────
	// OBR.2/3 are EI (Entity Identifier) composites → full Identifier, not scalar .value
	"OBR.2":  {{FHIRPath: "DiagnosticReport.identifier[0]", FHIRType: "Identifier", Confidence: 0.90, Source: "anchor"}},
	"OBR.3":  {{FHIRPath: "DiagnosticReport.identifier[1]", FHIRType: "Identifier", Confidence: 0.93, Source: "anchor"}},
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
	// ORC.2/3 are EI composites → full Identifier
	"ORC.1":  {{FHIRPath: "ServiceRequest.status", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"ORC.2":  {{FHIRPath: "ServiceRequest.identifier[0]", FHIRType: "Identifier", Confidence: 0.92, Source: "anchor"}},
	"ORC.3":  {{FHIRPath: "ServiceRequest.identifier[1]", FHIRType: "Identifier", Confidence: 0.90, Source: "anchor"}},
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
	// NK1.3 is CE composite → full CodeableConcept, not scalar .code leaf
	"NK1.2": {{FHIRPath: "RelatedPerson.name[0]", FHIRType: "HumanName", Confidence: 0.95, Source: "anchor"}},
	"NK1.3": {{FHIRPath: "RelatedPerson.relationship[0]", FHIRType: "CodeableConcept", Confidence: 0.92, Source: "anchor"}},
	"NK1.4": {{FHIRPath: "RelatedPerson.address[0]", FHIRType: "Address", Confidence: 0.88, Source: "anchor"}},
	"NK1.5": {{FHIRPath: "RelatedPerson.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.88, Source: "anchor"}},
	"NK1.7": {{FHIRPath: "RelatedPerson.period.start", FHIRType: "dateTime", Confidence: 0.82, Source: "anchor"}},
	"NK1.8": {{FHIRPath: "RelatedPerson.period.end", FHIRType: "dateTime", Confidence: 0.82, Source: "anchor"}},

	// ── IN1 ───────────────────────────────────────────────────────────────
	// IN1.1 is SI (sequence integer, set-id) — not a business identifier; value scalar is appropriate
	// IN1.3 is CWE (insurance company code) → full Identifier composite for payor
	"IN1.1":  {{FHIRPath: "Coverage.identifier[0].value", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"IN1.2":  {{FHIRPath: "Coverage.type", FHIRType: "CodeableConcept", Confidence: 0.90, Source: "anchor"}},
	"IN1.3":  {{FHIRPath: "Coverage.payor[0].identifier[0]", FHIRType: "Identifier", Confidence: 0.92, Source: "anchor"}},
	"IN1.4":  {{FHIRPath: "Coverage.payor[0].display", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"IN1.8":  {{FHIRPath: "Coverage.class[0].value", FHIRType: "string", Confidence: 0.92, Source: "anchor"}}, // IG: Group Number (ST) → Coverage.class.value (type=group)
	"IN1.12": {{FHIRPath: "Coverage.period.start", FHIRType: "dateTime", Confidence: 0.90, Source: "anchor"}},
	"IN1.13": {{FHIRPath: "Coverage.period.end", FHIRType: "dateTime", Confidence: 0.90, Source: "anchor"}},
	"IN1.17": {{FHIRPath: "Coverage.relationship", FHIRType: "CodeableConcept", Confidence: 0.92, Source: "anchor"}}, // IG ConceptMap: Insured's Relation to Patient (IS/CE) → Coverage.relationship
	"IN1.32": {}, // v2.3 Billing Status (IS, Table 0022) — no fm-status concept map; heuristic cannot resolve; normalizer defaults to "active"
	"IN1.36": {{FHIRPath: "Coverage.subscriberId", FHIRType: "string", Confidence: 0.88, Source: "anchor"}}, // R4: string field; R5 changed to subscriberIdentifier (Identifier)
	"IN1.43": {}, // v2.3 Insured's Sex / v2.5 Insured's Birth Place — no Coverage mapping per IG; block heuristic

	// ── STF (Practitioner master) ─────────────────────────────────────────
	// STF.1 is CX composite → full Identifier
	"STF.1":   {{FHIRPath: "Practitioner.identifier[0]", FHIRType: "Identifier", Confidence: 0.97, Source: "anchor"}},
	"STF.3":   {{FHIRPath: "Practitioner.name[0]", FHIRType: "HumanName", Confidence: 0.99, Source: "anchor"}},
	"STF.3.1": {{FHIRPath: "Practitioner.name[0].family", FHIRType: "string", Confidence: 0.97, Source: "anchor"}},
	"STF.3.2": {{FHIRPath: "Practitioner.name[0].given[0]", FHIRType: "string", Confidence: 0.95, Source: "anchor"}},
	"STF.3.3": {{FHIRPath: "Practitioner.name[0].given[1]", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"STF.3.5": {{FHIRPath: "Practitioner.name[0].prefix[0]", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"STF.4":   {{FHIRPath: "Practitioner.qualification[0].code.text", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"STF.5":   {{FHIRPath: "Practitioner.gender", FHIRType: "code", Confidence: 0.95, Source: "anchor"}},
	"STF.6":   {{FHIRPath: "Practitioner.birthDate", FHIRType: "date", Confidence: 0.93, Source: "anchor"}},
	"STF.7":   {{FHIRPath: "Practitioner.active", FHIRType: "boolean", Confidence: 0.95, Source: "anchor"}},
	"STF.10":  {{FHIRPath: "Practitioner.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.90, Source: "anchor"}},
	"STF.11":  {{FHIRPath: "Practitioner.address[0]", FHIRType: "Address", Confidence: 0.90, Source: "anchor"}},

	// ── PRA (Practitioner detail) ─────────────────────────────────────────
	// PRA.1 is CE used as primary key (links to STF.1 CX); map to composite Identifier
	// PRA.2 is CE (Practitioner Group) → PractitionerRole.specialty per v2-to-FHIR IG.
	//   NOT an identifier — CE type → ce_to_codeableconcept → CodeableConcept.
	// PRA.5 is PLN (Practitioner License Number) composite → full Identifier
	"PRA.1": {{FHIRPath: "PractitionerRole.identifier[0]", FHIRType: "Identifier", Confidence: 0.85, Source: "anchor"}},
	"PRA.2": {{FHIRPath: "PractitionerRole.specialty[0]", FHIRType: "CodeableConcept", Confidence: 0.88, Source: "anchor"}},
	"PRA.3": {{FHIRPath: "PractitionerRole.specialty[0].coding[0].code", FHIRType: "code", Confidence: 0.92, Source: "anchor"}},
	"PRA.5": {{FHIRPath: "Practitioner.qualification[0].identifier[0]", FHIRType: "Identifier", Confidence: 0.88, Source: "anchor"}},

	// ── LOC ───────────────────────────────────────────────────────────────
	// LOC.1 is PL composite (person location) → full Identifier for primary key
	"LOC.1": {{FHIRPath: "Location.identifier[0]", FHIRType: "Identifier", Confidence: 0.97, Source: "anchor"}},
	"LOC.2": {{FHIRPath: "Location.name", FHIRType: "string", Confidence: 0.97, Source: "anchor"}},
	"LOC.3": {{FHIRPath: "Location.type[0].coding[0].code", FHIRType: "code", Confidence: 0.88, Source: "anchor"}},
	"LOC.4": {{FHIRPath: "Location.address", FHIRType: "Address", Confidence: 0.88, Source: "anchor"}},
	"LOC.5": {{FHIRPath: "Location.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.85, Source: "anchor"}},

	// ── CDM ───────────────────────────────────────────────────────────────
	// CDM.1 is CWE (charge code) used as primary key → Identifier composite
	"CDM.1": {{FHIRPath: "ChargeItemDefinition.identifier[0]", FHIRType: "Identifier", Confidence: 0.97, Source: "anchor"}},
	"CDM.3": {{FHIRPath: "ChargeItemDefinition.title", FHIRType: "string", Confidence: 0.95, Source: "anchor"}},
	"CDM.4": {{FHIRPath: "ChargeItemDefinition.description", FHIRType: "markdown", Confidence: 0.90, Source: "anchor"}},

	// ── OM1 ───────────────────────────────────────────────────────────────
	// IG: OM1.2 (CWE) → ObservationDefinition.code (CodeableConcept), not identifier
	"OM1.2":  {{FHIRPath: "ObservationDefinition.code", FHIRType: "CodeableConcept", Confidence: 0.95, Source: "anchor"}},
	"OM1.7":  {{FHIRPath: "ObservationDefinition.code", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"OM1.11": {{FHIRPath: "ObservationDefinition.preferredReportName", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},

	// ── SCH ───────────────────────────────────────────────────────────────
	// SCH.1  EI  → full Identifier (placer appointment ID)
	// SCH.6  CWE → serviceType[0] (event/service reason, not clinical reason)
	// SCH.7  CE  → reasonCode[0] CodeableConcept (clinical reason, not appointmentType)
	// SCH.8  CWE → appointmentType CodeableConcept
	// SCH.9  NM  → minutesDuration positiveInt
	// SCH.10 CE  → skipped by classifyField (isUnitModifierField: "Appointment Duration Units")
	// SCH.11 TQ  → parent anchor; triggers component decomposition below
	// SCH.11.4 TS → start instant  (TQ component 4 = start date/time)
	// SCH.11.5 TS → end   instant  (TQ component 5 = end   date/time)
	// SCH.12 NM  → "Priority – Requested Start Date/Time Range" — NOT a timestamp,
	//              NOT Appointment.end. No anchor; heuristic produces no confident match.
	// SCH.25 CWE → status code (filler status; runtime applies siuFillerStatus lookup)
	"SCH.1":    {{FHIRPath: "Appointment.identifier[0]", FHIRType: "Identifier", Confidence: 0.95, Source: "anchor"}},
	"SCH.6":    {{FHIRPath: "Appointment.serviceType[0]", FHIRType: "CodeableConcept", Confidence: 0.85, Source: "anchor"}},
	"SCH.7":    {{FHIRPath: "Appointment.reasonCode[0]", FHIRType: "CodeableConcept", Confidence: 0.90, Source: "anchor"}},
	"SCH.8":    {{FHIRPath: "Appointment.appointmentType", FHIRType: "CodeableConcept", Confidence: 0.88, Source: "anchor"}},
	"SCH.9":    {{FHIRPath: "Appointment.minutesDuration", FHIRType: "positiveInt", Confidence: 0.95, Source: "anchor"}},
	"SCH.11":   {{FHIRPath: "Appointment.start", FHIRType: "instant", Confidence: 0.95, Source: "anchor"}},
	"SCH.11.4": {{FHIRPath: "Appointment.start", FHIRType: "instant", Confidence: 0.97, Source: "anchor"}},
	"SCH.11.5": {{FHIRPath: "Appointment.end", FHIRType: "instant", Confidence: 0.97, Source: "anchor"}},
	"SCH.25":   {{FHIRPath: "Appointment.status", FHIRType: "code", Confidence: 0.97, Source: "anchor"}},

	// ── AIS ───────────────────────────────────────────────────────────────
	// AIS.3 is the Universal Service Identifier (CWE) — the scheduled service,
	// NOT a participant actor. Mapping to serviceType[0] avoids collision with
	// AIP.3 (XCN practitioner) which correctly owns participant[0].actor.display.
	"AIS.3":  {{FHIRPath: "Appointment.serviceType[0]", FHIRType: "CodeableConcept", Confidence: 0.90, Source: "anchor"}},
	"AIS.4":  {{FHIRPath: "Appointment.participant[0].period.start", FHIRType: "dateTime", Confidence: 0.88, Source: "anchor"}},
	"AIS.10": {{FHIRPath: "Appointment.participant[0].status", FHIRType: "code", Confidence: 0.85, Source: "anchor"}},

	// ── AIG — General Resource (equipment / supplies) ──────────────────────
	// AIG.3 CE/CWE → actor display (general resource identifier)
	// AIG.4 ST     → participant type text
	"AIG.3": {{FHIRPath: "Appointment.participant[0].actor.display", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"AIG.4": {{FHIRPath: "Appointment.participant[0].type[0].text", FHIRType: "string", Confidence: 0.80, Source: "anchor"}},

	// ── AIL — Location Resource ────────────────────────────────────────────
	// AIL.3 PL → location actor display (room / bed / facility)
	// AIL.4 CE → location type text
	"AIL.3": {{FHIRPath: "Appointment.participant[0].actor.display", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"AIL.4": {{FHIRPath: "Appointment.participant[0].type[0].text", FHIRType: "string", Confidence: 0.82, Source: "anchor"}},

	// ── AIP — Personnel Resource (practitioners) ───────────────────────────
	// AIP.3 XCN → practitioner actor display (provider ID + name)
	// AIP.4 CE  → practitioner role / type text
	"AIP.3": {{FHIRPath: "Appointment.participant[0].actor.display", FHIRType: "string", Confidence: 0.92, Source: "anchor"}},
	"AIP.4": {{FHIRPath: "Appointment.participant[0].type[0].text", FHIRType: "string", Confidence: 0.83, Source: "anchor"}},

	// ── TQ1 — Timing/Quantity (HL7 v2.5+ replacement for the TQ composite) ────
	// Primary use in SIU messages: per-service timing for AIS/AIG/AIL/AIP blocks.
	// TQ1.5  NM  → minutesDuration (relative time; units default minutes)
	// TQ1.6  CQ  → minutesDuration (explicit service duration composite)
	// TQ1.7  TS  → start instant
	// TQ1.8  TS  → end   instant (takes precedence over SCH.11.5 when present)
	// TQ1.9  CWE → priority  (FHIR positiveInt — 0=highest; HL7 maps STAT→0, ROUTINE→2)
	// TQ1.11 TX  → patientInstruction free-text
	"TQ1.5":  {{FHIRPath: "Appointment.minutesDuration", FHIRType: "positiveInt", Confidence: 0.88, Source: "anchor"}},
	"TQ1.6":  {{FHIRPath: "Appointment.minutesDuration", FHIRType: "positiveInt", Confidence: 0.86, Source: "anchor"}},
	"TQ1.7":  {{FHIRPath: "Appointment.start", FHIRType: "instant", Confidence: 0.92, Source: "anchor"}},
	"TQ1.8":  {{FHIRPath: "Appointment.end", FHIRType: "instant", Confidence: 0.92, Source: "anchor"}},
	"TQ1.9":  {{FHIRPath: "Appointment.priority", FHIRType: "positiveInt", Confidence: 0.80, Source: "anchor"}},
	"TQ1.11": {{FHIRPath: "Appointment.patientInstruction", FHIRType: "string", Confidence: 0.75, Source: "anchor"}},

	// ── TXA ───────────────────────────────────────────────────────────────
	// TXA.9 is XCN (person reference); IG maps to DocumentReference.author, not identifier
	"TXA.2":  {{FHIRPath: "DocumentReference.type", FHIRType: "CodeableConcept", Confidence: 0.95, Source: "anchor"}},
	"TXA.4":  {{FHIRPath: "DocumentReference.date", FHIRType: "instant", Confidence: 0.93, Source: "anchor"}},
	"TXA.5":  {{FHIRPath: "DocumentReference.author[0].display", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"TXA.9":  {{FHIRPath: "DocumentReference.author[0].display", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"TXA.17": {{FHIRPath: "DocumentReference.status", FHIRType: "code", Confidence: 0.92, Source: "anchor"}},

	// ── RXA ───────────────────────────────────────────────────────────────
	"RXA.3":  {{FHIRPath: "Immunization.occurrenceDateTime", FHIRType: "dateTime", Confidence: 0.97, Source: "anchor"}},
	"RXA.5":  {{FHIRPath: "Immunization.vaccineCode", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"RXA.6":  {{FHIRPath: "Immunization.doseQuantity.value", FHIRType: "decimal", Confidence: 0.90, Source: "anchor"}},
	"RXA.20": {{FHIRPath: "Immunization.status", FHIRType: "code", Confidence: 0.93, Source: "anchor"}},

	// ── GT1 ───────────────────────────────────────────────────────────────
	// GT1.2 is CX composite (guarantor number) → full Identifier
	"GT1.2": {{FHIRPath: "RelatedPerson.identifier[0]", FHIRType: "Identifier", Confidence: 0.90, Source: "anchor"}},
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

// knownAnchorsR5 is the FHIR R5 version of the anchor table.
// Entries identical to R4 are copied verbatim. Changed entries are annotated
// with the reason. To add a new R5 change: update this table and leave R4
// untouched — the two maps are fully independent.
//
// Confirmed R5 structural changes applied here:
//   PV1.2   Encounter.class: Coding (R4) → CodeableConcept[] (R5)
//   PV1.7   Encounter.participant.individual → .actor
//   PV1.10  Encounter.hospitalization → Encounter.admission
//   PV1.17  Encounter.participant.individual → .actor
//   EVN.5   Encounter.participant.individual → .actor
//   IN1.36  Coverage.subscriberId (string) removed → Coverage.subscriberIdentifier (Identifier)
//   TXA.4   DocumentReference.date type: instant → dateTime
//   OM1.11  ObservationDefinition.preferredReportName removed → .name
var knownAnchorsR5 = map[string][]Candidate{
	// ── MSH ── (unchanged from R4) ────────────────────────────────────────
	"MSH.3":  {{FHIRPath: "MessageHeader.source.name", FHIRType: "string", Confidence: 0.99, Source: "anchor"}},
	"MSH.4":  {{FHIRPath: "MessageHeader.source.endpoint", FHIRType: "url", Confidence: 0.90, Source: "anchor"}},
	"MSH.5":  {{FHIRPath: "MessageHeader.destination[0].name", FHIRType: "string", Confidence: 0.97, Source: "anchor"}},
	"MSH.7":  {{FHIRPath: "MessageHeader.meta.lastUpdated", FHIRType: "instant", Confidence: 0.95, Source: "anchor"}},
	"MSH.9":  {{FHIRPath: "MessageHeader.eventCoding.code", FHIRType: "code", Confidence: 0.99, Source: "anchor"}},
	"MSH.10": {{FHIRPath: "MessageHeader.id", FHIRType: "id", Confidence: 0.99, Source: "anchor"}},
	"MSH.11": {{FHIRPath: "MessageHeader.meta.security[0].code", FHIRType: "code", Confidence: 0.85, Source: "anchor"}},

	// ── PID ── (unchanged from R4 except PID.18 composite fix) ──────────
	"PID.3":  {{FHIRPath: "Patient.identifier", FHIRType: "Identifier", Confidence: 0.99, Source: "anchor"}},
	"PID.5":  {{FHIRPath: "Patient.name[0]", FHIRType: "HumanName", Confidence: 0.99, Source: "anchor"}},
	"PID.7":  {{FHIRPath: "Patient.birthDate", FHIRType: "date", Confidence: 0.99, Source: "anchor"}},
	"PID.8":  {{FHIRPath: "Patient.gender", FHIRType: "code", Confidence: 0.97, Source: "anchor"}},
	"PID.10": {{FHIRPath: "Patient.extension[race]", FHIRType: "Extension", Confidence: 0.85, Source: "anchor"}},
	"PID.11": {{FHIRPath: "Patient.address[0]", FHIRType: "Address", Confidence: 0.97, Source: "anchor"}},
	"PID.13": {{FHIRPath: "Patient.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.95, Source: "anchor"}},
	"PID.14": {{FHIRPath: "Patient.telecom[1]", FHIRType: "ContactPoint", Confidence: 0.90, Source: "anchor"}},
	"PID.15": {{FHIRPath: "Patient.communication[0].language", FHIRType: "CodeableConcept", Confidence: 0.90, Source: "anchor"}},
	"PID.16": {{FHIRPath: "Patient.maritalStatus", FHIRType: "CodeableConcept", Confidence: 0.95, Source: "anchor"}},
	"PID.18": {{FHIRPath: "Patient.identifier[1]", FHIRType: "Identifier", Confidence: 0.88, Source: "anchor"}}, // CX composite → full Identifier
	"PID.19": {{FHIRPath: "Patient.identifier[2].value", FHIRType: "string", Confidence: 0.85, Source: "anchor"}},  // ST (plain string) → scalar .value is correct
	"PID.22": {{FHIRPath: "Patient.extension[ethnicity]", FHIRType: "Extension", Confidence: 0.85, Source: "anchor"}},
	"PID.29": {{FHIRPath: "Patient.deceasedDateTime", FHIRType: "dateTime", Confidence: 0.92, Source: "anchor"}},
	"PID.30": {{FHIRPath: "Patient.deceasedBoolean", FHIRType: "boolean", Confidence: 0.90, Source: "anchor"}},

	// ── PV1 ── R5: class→CodeableConcept[], hospitalization→admission, individual→actor
	"PV1.2":  {{FHIRPath: "Encounter.class[0].coding[0].code", FHIRType: "code", Confidence: 0.95, Source: "anchor"}},   // R4: Encounter.class.code (Coding); R5: CodeableConcept[]
	"PV1.3":  {{FHIRPath: "Encounter.location[0].location.display", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"PV1.7":  {{FHIRPath: "Encounter.participant[0].actor.display", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},  // R4: .individual; R5: .actor
	"PV1.10": {{FHIRPath: "Encounter.admission.admitSource", FHIRType: "CodeableConcept", Confidence: 0.88, Source: "anchor"}}, // R4: .hospitalization; R5: .admission
	"PV1.17": {{FHIRPath: "Encounter.participant[1].actor.display", FHIRType: "string", Confidence: 0.85, Source: "anchor"}},  // R4: .individual; R5: .actor
	"PV1.18": {{FHIRPath: "Encounter.type[0].coding[0].code", FHIRType: "code", Confidence: 0.85, Source: "anchor"}},
	"PV1.44": {{FHIRPath: "Encounter.period.start", FHIRType: "dateTime", Confidence: 0.97, Source: "anchor"}},
	"PV1.45": {{FHIRPath: "Encounter.period.end", FHIRType: "dateTime", Confidence: 0.97, Source: "anchor"}},

	// ── EVN ── R5: individual→actor
	// EVN.1 — same rationale as R4: no IG FHIR target, empty anchor prevents heuristic.
	"EVN.1": {},
	"EVN.2": {{FHIRPath: "Encounter.period.start", FHIRType: "dateTime", Confidence: 0.88, Source: "anchor"}},
	"EVN.4": {{FHIRPath: "Encounter.type[0].coding[0].code", FHIRType: "code", Confidence: 0.82, Source: "anchor"}},
	"EVN.5": {{FHIRPath: "Encounter.participant[0].actor.display", FHIRType: "string", Confidence: 0.80, Source: "anchor"}}, // R4: .individual; R5: .actor

	// ── OBR ── EI composite → full Identifier (same fix as R4)
	"OBR.2":  {{FHIRPath: "DiagnosticReport.identifier[0]", FHIRType: "Identifier", Confidence: 0.90, Source: "anchor"}},
	"OBR.3":  {{FHIRPath: "DiagnosticReport.identifier[1]", FHIRType: "Identifier", Confidence: 0.93, Source: "anchor"}},
	"OBR.4":  {{FHIRPath: "DiagnosticReport.code", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"OBR.7":  {{FHIRPath: "DiagnosticReport.effectiveDateTime", FHIRType: "dateTime", Confidence: 0.93, Source: "anchor"}},
	"OBR.16": {{FHIRPath: "DiagnosticReport.resultsInterpreter[0].display", FHIRType: "string", Confidence: 0.85, Source: "anchor"}},
	"OBR.22": {{FHIRPath: "DiagnosticReport.issued", FHIRType: "instant", Confidence: 0.90, Source: "anchor"}},
	"OBR.25": {{FHIRPath: "DiagnosticReport.status", FHIRType: "code", Confidence: 0.92, Source: "anchor"}},

	// ── OBX ── (unchanged from R4) ────────────────────────────────────────
	"OBX.3":  {{FHIRPath: "Observation.code", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"OBX.5":  {{FHIRPath: "Observation.value[x]", FHIRType: "varies", Confidence: 0.90, Source: "anchor"}},
	"OBX.6":  {{FHIRPath: "Observation.valueQuantity.unit", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"OBX.7":  {{FHIRPath: "Observation.referenceRange[0].text", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"OBX.8":  {{FHIRPath: "Observation.interpretation[0].coding[0].code", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"OBX.11": {{FHIRPath: "Observation.status", FHIRType: "code", Confidence: 0.95, Source: "anchor"}},
	"OBX.14": {{FHIRPath: "Observation.effectiveDateTime", FHIRType: "dateTime", Confidence: 0.93, Source: "anchor"}},

	// ── ORC ── EI composite → full Identifier (same fix as R4)
	"ORC.1":  {{FHIRPath: "ServiceRequest.status", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"ORC.2":  {{FHIRPath: "ServiceRequest.identifier[0]", FHIRType: "Identifier", Confidence: 0.92, Source: "anchor"}},
	"ORC.3":  {{FHIRPath: "ServiceRequest.identifier[1]", FHIRType: "Identifier", Confidence: 0.90, Source: "anchor"}},
	"ORC.9":  {{FHIRPath: "ServiceRequest.authoredOn", FHIRType: "dateTime", Confidence: 0.93, Source: "anchor"}},
	"ORC.12": {{FHIRPath: "ServiceRequest.requester.display", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},

	// ── AL1 ── (unchanged from R4) ────────────────────────────────────────
	"AL1.2": {{FHIRPath: "AllergyIntolerance.category[0]", FHIRType: "code", Confidence: 0.92, Source: "anchor"}},
	"AL1.3": {{FHIRPath: "AllergyIntolerance.code", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"AL1.4": {{FHIRPath: "AllergyIntolerance.reaction[0].severity", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"AL1.5": {{FHIRPath: "AllergyIntolerance.reaction[0].description", FHIRType: "string", Confidence: 0.85, Source: "anchor"}},
	"AL1.6": {{FHIRPath: "AllergyIntolerance.onsetDateTime", FHIRType: "dateTime", Confidence: 0.88, Source: "anchor"}},

	// ── DG1 ── (unchanged from R4) ────────────────────────────────────────
	"DG1.3": {{FHIRPath: "Condition.code", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"DG1.5": {{FHIRPath: "Condition.onsetDateTime", FHIRType: "dateTime", Confidence: 0.90, Source: "anchor"}},
	"DG1.6": {{FHIRPath: "Condition.category[0].coding[0].code", FHIRType: "code", Confidence: 0.88, Source: "anchor"}},

	// ── NK1 ── NK1.3 CE composite → full CodeableConcept (same fix as R4)
	"NK1.2": {{FHIRPath: "RelatedPerson.name[0]", FHIRType: "HumanName", Confidence: 0.95, Source: "anchor"}},
	"NK1.3": {{FHIRPath: "RelatedPerson.relationship[0]", FHIRType: "CodeableConcept", Confidence: 0.92, Source: "anchor"}},
	"NK1.4": {{FHIRPath: "RelatedPerson.address[0]", FHIRType: "Address", Confidence: 0.88, Source: "anchor"}},
	"NK1.5": {{FHIRPath: "RelatedPerson.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.88, Source: "anchor"}},
	"NK1.7": {{FHIRPath: "RelatedPerson.period.start", FHIRType: "dateTime", Confidence: 0.82, Source: "anchor"}},
	"NK1.8": {{FHIRPath: "RelatedPerson.period.end", FHIRType: "dateTime", Confidence: 0.82, Source: "anchor"}},

	// ── IN1 ── R5: subscriberId→subscriberIdentifier (Identifier); IN1.2 CE→CodeableConcept; IN1.3 CWE→Identifier
	"IN1.1":  {{FHIRPath: "Coverage.identifier[0].value", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},   // SI (sequence int) → scalar ok
	"IN1.2":  {{FHIRPath: "Coverage.type", FHIRType: "CodeableConcept", Confidence: 0.90, Source: "anchor"}},
	"IN1.3":  {{FHIRPath: "Coverage.payor[0].identifier[0]", FHIRType: "Identifier", Confidence: 0.92, Source: "anchor"}},
	"IN1.4":  {{FHIRPath: "Coverage.payor[0].display", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"IN1.8":  {{FHIRPath: "Coverage.class[0].value", FHIRType: "string", Confidence: 0.92, Source: "anchor"}}, // IG: Group Number (ST) → Coverage.class.value (type=group)
	"IN1.12": {{FHIRPath: "Coverage.period.start", FHIRType: "dateTime", Confidence: 0.90, Source: "anchor"}},
	"IN1.13": {{FHIRPath: "Coverage.period.end", FHIRType: "dateTime", Confidence: 0.90, Source: "anchor"}},
	"IN1.17": {{FHIRPath: "Coverage.relationship", FHIRType: "CodeableConcept", Confidence: 0.92, Source: "anchor"}}, // IG ConceptMap: Insured's Relation to Patient → Coverage.relationship (same as R4)
	"IN1.32": {}, // v2.3 Billing Status — no fm-status concept map; normalizer handles defaulting (same as R4)
	"IN1.36": {{FHIRPath: "Coverage.subscriberIdentifier", FHIRType: "Identifier", Confidence: 0.88, Source: "anchor"}}, // R5: CX composite → full Identifier
	"IN1.43": {}, // v2.3 Insured's Sex / v2.5 Insured's Birth Place — no Coverage mapping per IG (same as R4)

	// ── STF ── CX composite → full Identifier (same fix as R4)
	"STF.1":   {{FHIRPath: "Practitioner.identifier[0]", FHIRType: "Identifier", Confidence: 0.97, Source: "anchor"}},
	"STF.3":   {{FHIRPath: "Practitioner.name[0]", FHIRType: "HumanName", Confidence: 0.99, Source: "anchor"}},
	"STF.3.1": {{FHIRPath: "Practitioner.name[0].family", FHIRType: "string", Confidence: 0.97, Source: "anchor"}},
	"STF.3.2": {{FHIRPath: "Practitioner.name[0].given[0]", FHIRType: "string", Confidence: 0.95, Source: "anchor"}},
	"STF.3.3": {{FHIRPath: "Practitioner.name[0].given[1]", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"STF.3.5": {{FHIRPath: "Practitioner.name[0].prefix[0]", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"STF.4":   {{FHIRPath: "Practitioner.qualification[0].code.text", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"STF.5":   {{FHIRPath: "Practitioner.gender", FHIRType: "code", Confidence: 0.95, Source: "anchor"}},
	"STF.6":   {{FHIRPath: "Practitioner.birthDate", FHIRType: "date", Confidence: 0.93, Source: "anchor"}},
	"STF.7":   {{FHIRPath: "Practitioner.active", FHIRType: "boolean", Confidence: 0.95, Source: "anchor"}},
	"STF.10":  {{FHIRPath: "Practitioner.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.90, Source: "anchor"}},
	"STF.11":  {{FHIRPath: "Practitioner.address[0]", FHIRType: "Address", Confidence: 0.90, Source: "anchor"}},

	// ── PRA ── composite fixes same as R4
	"PRA.1": {{FHIRPath: "PractitionerRole.identifier[0]", FHIRType: "Identifier", Confidence: 0.85, Source: "anchor"}},
	"PRA.2": {{FHIRPath: "PractitionerRole.specialty[0]", FHIRType: "CodeableConcept", Confidence: 0.88, Source: "anchor"}},
	"PRA.3": {{FHIRPath: "PractitionerRole.specialty[0].coding[0].code", FHIRType: "code", Confidence: 0.92, Source: "anchor"}},
	"PRA.5": {{FHIRPath: "Practitioner.qualification[0].identifier[0]", FHIRType: "Identifier", Confidence: 0.88, Source: "anchor"}},

	// ── LOC ── PL composite → full Identifier (same fix as R4)
	"LOC.1": {{FHIRPath: "Location.identifier[0]", FHIRType: "Identifier", Confidence: 0.97, Source: "anchor"}},
	"LOC.2": {{FHIRPath: "Location.name", FHIRType: "string", Confidence: 0.97, Source: "anchor"}},
	"LOC.3": {{FHIRPath: "Location.type[0].coding[0].code", FHIRType: "code", Confidence: 0.88, Source: "anchor"}},
	"LOC.4": {{FHIRPath: "Location.address", FHIRType: "Address", Confidence: 0.88, Source: "anchor"}},
	"LOC.5": {{FHIRPath: "Location.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.85, Source: "anchor"}},

	// ── CDM ── CWE composite → full Identifier (same fix as R4)
	"CDM.1": {{FHIRPath: "ChargeItemDefinition.identifier[0]", FHIRType: "Identifier", Confidence: 0.97, Source: "anchor"}},
	"CDM.3": {{FHIRPath: "ChargeItemDefinition.title", FHIRType: "string", Confidence: 0.95, Source: "anchor"}},
	"CDM.4": {{FHIRPath: "ChargeItemDefinition.description", FHIRType: "markdown", Confidence: 0.90, Source: "anchor"}},

	// ── OM1 ── OM1.2 CWE → ObservationDefinition.code CodeableConcept (per IG); R5: .name replaces preferredReportName
	"OM1.2":  {{FHIRPath: "ObservationDefinition.code", FHIRType: "CodeableConcept", Confidence: 0.95, Source: "anchor"}},
	"OM1.7":  {{FHIRPath: "ObservationDefinition.code", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"OM1.11": {{FHIRPath: "ObservationDefinition.name", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},

	// ── SCH ── same fixes as R4; R5 Appointment structure is identical for these fields
	"SCH.1":    {{FHIRPath: "Appointment.identifier[0]", FHIRType: "Identifier", Confidence: 0.95, Source: "anchor"}},
	"SCH.6":    {{FHIRPath: "Appointment.serviceType[0]", FHIRType: "CodeableConcept", Confidence: 0.85, Source: "anchor"}},
	"SCH.7":    {{FHIRPath: "Appointment.reasonCode[0]", FHIRType: "CodeableConcept", Confidence: 0.90, Source: "anchor"}},
	"SCH.8":    {{FHIRPath: "Appointment.appointmentType", FHIRType: "CodeableConcept", Confidence: 0.88, Source: "anchor"}},
	"SCH.9":    {{FHIRPath: "Appointment.minutesDuration", FHIRType: "positiveInt", Confidence: 0.95, Source: "anchor"}},
	"SCH.11":   {{FHIRPath: "Appointment.start", FHIRType: "instant", Confidence: 0.95, Source: "anchor"}},
	"SCH.11.4": {{FHIRPath: "Appointment.start", FHIRType: "instant", Confidence: 0.97, Source: "anchor"}},
	"SCH.11.5": {{FHIRPath: "Appointment.end", FHIRType: "instant", Confidence: 0.97, Source: "anchor"}},
	"SCH.12":   {{FHIRPath: "Appointment.end", FHIRType: "instant", Confidence: 0.93, Source: "anchor"}},
	"SCH.25":   {{FHIRPath: "Appointment.status", FHIRType: "code", Confidence: 0.97, Source: "anchor"}},

	// ── AIS ── (same fix as R4: AIS.3 → serviceType, not actor) ─────────────
	"AIS.3":  {{FHIRPath: "Appointment.serviceType[0]", FHIRType: "CodeableConcept", Confidence: 0.90, Source: "anchor"}},
	"AIS.4":  {{FHIRPath: "Appointment.participant[0].period.start", FHIRType: "dateTime", Confidence: 0.88, Source: "anchor"}},
	"AIS.10": {{FHIRPath: "Appointment.participant[0].status", FHIRType: "code", Confidence: 0.85, Source: "anchor"}},

	// ── AIG / AIL / AIP ── (same as R4) ──────────────────────────────────
	"AIG.3": {{FHIRPath: "Appointment.participant[0].actor.display", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"AIG.4": {{FHIRPath: "Appointment.participant[0].type[0].text", FHIRType: "string", Confidence: 0.80, Source: "anchor"}},
	"AIL.3": {{FHIRPath: "Appointment.participant[0].actor.display", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"AIL.4": {{FHIRPath: "Appointment.participant[0].type[0].text", FHIRType: "string", Confidence: 0.82, Source: "anchor"}},
	"AIP.3": {{FHIRPath: "Appointment.participant[0].actor.display", FHIRType: "string", Confidence: 0.92, Source: "anchor"}},
	"AIP.4": {{FHIRPath: "Appointment.participant[0].type[0].text", FHIRType: "string", Confidence: 0.83, Source: "anchor"}},

	// ── TQ1 ── (same as R4; R5 Appointment timing fields are identical)
	"TQ1.5":  {{FHIRPath: "Appointment.minutesDuration", FHIRType: "positiveInt", Confidence: 0.88, Source: "anchor"}},
	"TQ1.6":  {{FHIRPath: "Appointment.minutesDuration", FHIRType: "positiveInt", Confidence: 0.86, Source: "anchor"}},
	"TQ1.7":  {{FHIRPath: "Appointment.start", FHIRType: "instant", Confidence: 0.92, Source: "anchor"}},
	"TQ1.8":  {{FHIRPath: "Appointment.end", FHIRType: "instant", Confidence: 0.92, Source: "anchor"}},
	"TQ1.9":  {{FHIRPath: "Appointment.priority", FHIRType: "positiveInt", Confidence: 0.80, Source: "anchor"}},
	"TQ1.11": {{FHIRPath: "Appointment.patientInstruction", FHIRType: "string", Confidence: 0.75, Source: "anchor"}},

	// ── TXA ── R5: date type changed instant→dateTime; TXA.2 CE→CodeableConcept; TXA.9 XCN→author
	"TXA.2":  {{FHIRPath: "DocumentReference.type", FHIRType: "CodeableConcept", Confidence: 0.95, Source: "anchor"}},
	"TXA.4":  {{FHIRPath: "DocumentReference.date", FHIRType: "dateTime", Confidence: 0.93, Source: "anchor"}},
	"TXA.5":  {{FHIRPath: "DocumentReference.author[0].display", FHIRType: "string", Confidence: 0.90, Source: "anchor"}},
	"TXA.9":  {{FHIRPath: "DocumentReference.author[0].display", FHIRType: "string", Confidence: 0.88, Source: "anchor"}},
	"TXA.17": {{FHIRPath: "DocumentReference.status", FHIRType: "code", Confidence: 0.92, Source: "anchor"}},

	// ── RXA ── (unchanged from R4) ────────────────────────────────────────
	"RXA.3":  {{FHIRPath: "Immunization.occurrenceDateTime", FHIRType: "dateTime", Confidence: 0.97, Source: "anchor"}},
	"RXA.5":  {{FHIRPath: "Immunization.vaccineCode", FHIRType: "CodeableConcept", Confidence: 0.97, Source: "anchor"}},
	"RXA.6":  {{FHIRPath: "Immunization.doseQuantity.value", FHIRType: "decimal", Confidence: 0.90, Source: "anchor"}},
	"RXA.20": {{FHIRPath: "Immunization.status", FHIRType: "code", Confidence: 0.93, Source: "anchor"}},

	// ── GT1 ── CX composite → full Identifier (same fix as R4)
	"GT1.2": {{FHIRPath: "RelatedPerson.identifier[0]", FHIRType: "Identifier", Confidence: 0.90, Source: "anchor"}},
	"GT1.3": {{FHIRPath: "RelatedPerson.name[0]", FHIRType: "HumanName", Confidence: 0.95, Source: "anchor"}},
	"GT1.5": {{FHIRPath: "RelatedPerson.address[0]", FHIRType: "Address", Confidence: 0.90, Source: "anchor"}},
	"GT1.6": {{FHIRPath: "RelatedPerson.telecom[0]", FHIRType: "ContactPoint", Confidence: 0.88, Source: "anchor"}},
	"GT1.8": {{FHIRPath: "RelatedPerson.birthDate", FHIRType: "date", Confidence: 0.90, Source: "anchor"}},
	"GT1.9": {{FHIRPath: "RelatedPerson.gender", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},

	// ── ROL ── (unchanged from R4) ────────────────────────────────────────
	"ROL.3": {{FHIRPath: "PractitionerRole.code[0].coding[0].code", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"ROL.4": {{FHIRPath: "PractitionerRole.practitioner.display", FHIRType: "string", Confidence: 0.92, Source: "anchor"}},
	"ROL.5": {{FHIRPath: "PractitionerRole.period.start", FHIRType: "dateTime", Confidence: 0.85, Source: "anchor"}},

	// ── PR1 ── (unchanged from R4) ────────────────────────────────────────
	"PR1.3": {{FHIRPath: "Procedure.code", FHIRType: "CodeableConcept", Confidence: 0.95, Source: "anchor"}},
	"PR1.5": {{FHIRPath: "Procedure.performedDateTime", FHIRType: "dateTime", Confidence: 0.93, Source: "anchor"}},
	"PR1.6": {{FHIRPath: "Procedure.performer[0].actor.display", FHIRType: "string", Confidence: 0.85, Source: "anchor"}},

	// ── FT1 ── (unchanged from R4) ────────────────────────────────────────
	"FT1.4": {{FHIRPath: "ChargeItem.occurrenceDateTime", FHIRType: "dateTime", Confidence: 0.90, Source: "anchor"}},
	"FT1.6": {{FHIRPath: "ChargeItem.code", FHIRType: "CodeableConcept", Confidence: 0.93, Source: "anchor"}},
	"FT1.7": {{FHIRPath: "ChargeItem.quantity.value", FHIRType: "decimal", Confidence: 0.88, Source: "anchor"}},

	// ── MFI / MFE ── (unchanged from R4) ─────────────────────────────────
	"MFI.1": {{FHIRPath: "MessageHeader.meta.tag[0].code", FHIRType: "code", Confidence: 0.88, Source: "anchor"}},
	"MFI.3": {{FHIRPath: "MessageHeader.meta.tag[1].code", FHIRType: "code", Confidence: 0.82, Source: "anchor"}},
	"MFE.1": {{FHIRPath: "meta.tag[0].code", FHIRType: "code", Confidence: 0.90, Source: "anchor"}},
	"MFE.2": {{FHIRPath: "meta.versionId", FHIRType: "id", Confidence: 0.85, Source: "anchor"}},
}

// anchorsForVersion returns the anchor table for the given FHIR version.
func anchorsForVersion(v FHIRVersion) map[string][]Candidate {
	if v == FHIRVersionR5 {
		return knownAnchorsR5
	}
	return knownAnchorsR4
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
	fhirVer FHIRVersion,
) []Candidate {
	key := strings.ToUpper(segmentName) + "." + fieldNumber(fieldKey)

	// Pass 1 — known anchors, version-specific table selected upfront
	if anchors, ok := anchorsForVersion(fhirVer)[key]; ok {
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
			if isBlocklisted(path) {
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
			if isBlocklisted(path) {
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
