// services/hl7assembly/vxu_assembly.go
//
// Structural assembly for VXU (Vaccination Update) messages.
//
// VXU^V04 is the HL7 v2 unsolicited vaccination record update message.  The
// vaccination-level segment groups within a VXU are:
//
//	[ORC]    — Common Order (carry-forward, not mapped here)
//	 RXA     — Pharmacy/Treatment Administration (one per vaccination event)
//	[RXR]    — Pharmacy/Treatment Route / Site  (paired with RXA by sub-ID)
//	{OBX}    — Observations: VIS dates, dose number, series status, funding, …
//
// FHIR R4 mapping
//
//	RXA  → Immunization (primary resource; one per RXA segment)
//	RXR  → Immunization.route  +  Immunization.site
//	OBX  → Immunization.protocolApplied  (dose number / series)
//	        Immunization.education        (VIS document dates)
//	        standalone Observation        (all other OBX content)
//
// Mandatory FHIR R4 Immunization fields satisfied here:
//
//	status      (1..1)  from RXA.20 completion status
//	vaccineCode (1..1)  from RXA.5  (CVX / NDC / CPT administered code)
//	patient     (1..1)  Patient reference
//	occurrence  (1..1)  from RXA.3  date/time of administration
package hl7assembly

import (
	"log"
	"strconv"
	"strings"

	"ezhealthkonnect/hl7"
)

// ──────────────────────────────────────────────────────────────────────────────
// Status / coding-system mappers
// ──────────────────────────────────────────────────────────────────────────────

// RXA20ToImmunizationStatus maps RXA.20 (HL7 Table 0322) to FHIR R4
// Immunization.status values.
func RXA20ToImmunizationStatus(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "CP": // Complete
		return "completed"
	case "RE": // Refused
		return "not-done"
	case "NA": // Not Administered
		return "not-done"
	case "PA": // Partially Administered
		return "completed"
	default:
		return "completed"
	}
}

// rxaVaccineSystemToURI maps vaccine coding system abbreviations found in
// RXA.5 component 3 to canonical FHIR system URIs.
func rxaVaccineSystemToURI(system string) string {
	switch strings.ToUpper(strings.TrimSpace(system)) {
	case "CVX", "HL70292", "LIM_CVX", "CVX_IIS":
		return "http://hl7.org/fhir/sid/cvx"
	case "NDC":
		return "http://hl7.org/fhir/sid/ndc"
	case "CPT", "C4":
		return "http://www.ama-assn.org/go/cpt"
	case "SCT", "SNM", "SNOMED":
		return "http://snomed.info/sct"
	default:
		return ""
	}
}

// routeCEToFHIR maps HL7 Table 0162 route codes from RXR.1 to a FHIR
// CodeableConcept using the v2 table as the coding system.
// The SNOMED display strings are included for human readability only; the
// validator will accept the v2-0162 system without display validation.
var routeCodeMap = map[string]struct{ code, display string }{
	"IM":   {"IM", "Injection, intramuscular"},
	"SC":   {"SC", "Injection, subcutaneous"},
	"ID":   {"ID", "Injection, intradermal"},
	"IN":   {"IN", "Inhalation"},
	"INHL": {"INHL", "Inhalation"},
	"PO":   {"PO", "Swallow, oral"},
	"NS":   {"NS", "Injection, intranasal"},
	"SL":   {"SL", "Sublingual"},
	"TD":   {"TD", "Transdermal"},
}

// siteCodeMap maps HL7 Table 0163 administration site codes from RXR.2 to
// display strings for the FHIR site CodeableConcept.
var siteCodeMap = map[string]string{
	"LA":  "Left arm",
	"RA":  "Right arm",
	"LT":  "Left thigh",
	"RT":  "Right thigh",
	"LD":  "Left deltoid",
	"RD":  "Right deltoid",
	"LVL": "Left vastus lateralis",
	"RVL": "Right vastus lateralis",
	"LG":  "Left gluteus medius",
	"RG":  "Right gluteus medius",
}

// ──────────────────────────────────────────────────────────────────────────────
// Vaccination order group
// ──────────────────────────────────────────────────────────────────────────────

// vaccinationGroup represents one RXA-centred order group within a VXU message.
type vaccinationGroup struct {
	rxa   hl7.EnhancedSegment
	rxr   *hl7.EnhancedSegment  // nil when no paired RXR
	obxes []hl7.EnhancedSegment // OBX segments that belong to this vaccination
}

// groupVXUVaccinations collects RXA, RXR, and OBX segments from parsedHL7Data
// and organises them into per-vaccination groups.
//
// Pairing strategy:
//   - RXA[i] is paired with RXR[i] (same sequential position).
//   - OBX segments are matched to an RXA by OBX.4 (sub-ID) == RXA.1 (sub-ID).
//     When sub-IDs are absent or ambiguous, all OBX are assigned to all groups.
func groupVXUVaccinations(parsedHL7Data map[string]interface{}) []vaccinationGroup {
	rxaList := ExtractSegmentGroup(parsedHL7Data, "RXA")
	if len(rxaList) == 0 {
		return nil
	}
	rxrList := ExtractSegmentGroup(parsedHL7Data, "RXR")
	obxList := ExtractSegmentGroup(parsedHL7Data, "OBX")

	groups := make([]vaccinationGroup, len(rxaList))
	for i, rxa := range rxaList {
		groups[i].rxa = rxa
		if i < len(rxrList) {
			rxr := rxrList[i]
			groups[i].rxr = &rxr
		}
	}

	// Assign OBX to groups by sub-ID matching (OBX.4 → RXA.1).
	if len(rxaList) == 1 {
		// Single vaccination — all OBX belong to it.
		groups[0].obxes = obxList
	} else {
		// Build a set-ID → group-index map from RXA.1.
		rxaSubIDIdx := make(map[string]int, len(rxaList))
		for i, rxa := range rxaList {
			if sid := SegFieldValue(rxa, "RXA.1"); sid != "" {
				rxaSubIDIdx[sid] = i
			}
		}
		for _, obx := range obxList {
			subID := SegFieldValue(obx, "OBX.4")
			if idx, ok := rxaSubIDIdx[subID]; ok {
				groups[idx].obxes = append(groups[idx].obxes, obx)
			} else {
				// No sub-ID match — add to all groups (common in simple VXU messages).
				for j := range groups {
					groups[j].obxes = append(groups[j].obxes, obx)
				}
			}
		}
	}

	return groups
}

// ──────────────────────────────────────────────────────────────────────────────
// Main assembly entry point
// ──────────────────────────────────────────────────────────────────────────────

// AssembleVXUImmunizations performs VXU^V04 structural assembly:
//
//  1. Skips silently when no RXA segment is present.
//  2. Derives the facility namespace from MSH.4 (Tier-3 coding fallback).
//  3. Builds one Immunization per RXA segment, pairing each with its RXR
//     (route/site) and OBX (protocol, education, observations).
//  4. Removes any Immunization or Observation resources previously produced
//     by the field-mapping engine (incomplete without RXR/OBX correlation).
//  5. Updates MessageHeader.focus to list all assembled Immunization IDs.
//
// Returns the updated resource list and advisory warnings.
func AssembleVXUImmunizations(
	parsedHL7Data map[string]interface{},
	resources []map[string]interface{},
	opts ...AssemblyRules,
) ([]map[string]interface{}, []string) {
	var rules AssemblyRules
	if len(opts) > 0 {
		rules = opts[0]
	}

	var warnings []string

	// ── Guard: only run when RXA segments are present ────────────────────────
	groups := groupVXUVaccinations(parsedHL7Data)
	if len(groups) == 0 {
		return resources, warnings
	}

	// ── Derive facility namespace ─────────────────────────────────────────────
	if rules.FacilityNamespace == "" {
		mshList := ExtractSegmentGroup(parsedHL7Data, "MSH")
		if len(mshList) > 0 {
			if sf := SegFieldValue(mshList[0], "MSH.4"); sf != "" {
				rules.FacilityNamespace = FacilityNamespaceURI(sf)
			}
		}
		if rules.FacilityNamespace == "" {
			rules.FacilityNamespace = "urn:facility:unknown"
		}
	}

	// ── Find Patient reference from existing resources ────────────────────────
	patientRef := ""
	for _, r := range resources {
		if r["resourceType"] == "Patient" {
			if id, ok := r["id"].(string); ok && id != "" {
				patientRef = "Patient/" + id
			}
			break
		}
	}

	// ── Build Immunizations + supporting Observations ─────────────────────────
	var immunizations []map[string]interface{}
	var observations []map[string]interface{}

	for _, grp := range groups {
		imm, obs := buildImmunizationFromGroup(grp, patientRef, rules.FacilityNamespace, rules)
		if imm != nil {
			immunizations = append(immunizations, imm)
		}
		observations = append(observations, obs...)
	}

	if len(immunizations) == 0 {
		return resources, warnings
	}

	// ── Build focus refs for MessageHeader ───────────────────────────────────
	var focusRefs []interface{}
	for _, imm := range immunizations {
		if id, ok := imm["id"].(string); ok {
			focusRefs = append(focusRefs, map[string]interface{}{"reference": "Immunization/" + id})
		}
	}

	// ── Rebuild resource list ─────────────────────────────────────────────────
	// Drop Immunization and Observation resources from the field-mapping engine
	// (incomplete — no RXR correlation, no VIS/protocol OBX handling).
	var result []map[string]interface{}

	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		switch rt {
		case "Immunization", "Observation":
			// Drop — replaced by assembly-built versions below
		case "MessageHeader":
			if len(focusRefs) > 0 {
				r["focus"] = focusRefs
			}
			result = append(result, r)
		default:
			result = append(result, r)
		}
	}

	result = append(result, immunizations...)
	result = append(result, observations...)

	log.Printf("💉 hl7.assemble_vxu: built %d Immunization(s), %d Observation(s)",
		len(immunizations), len(observations))

	return result, warnings
}

// ──────────────────────────────────────────────────────────────────────────────
// Per-group builders
// ──────────────────────────────────────────────────────────────────────────────

// buildImmunizationFromGroup maps one RXA order group to a FHIR R4 Immunization
// and its supporting Observations.
//
// RXA fields used:
//
//	RXA.1  sub-ID               → id suffix
//	RXA.3  date/time admin start → occurrenceDateTime
//	RXA.5  administered code (CE)→ vaccineCode (CVX/NDC)
//	RXA.6  administered amount   → doseQuantity.value
//	RXA.7  administered units    → doseQuantity.unit
//	RXA.9  administration notes  → note[].text
//	RXA.10 administering provider→ performer[0].actor (display only)
//	RXA.11 administered-at loc.  → location (display only)
//	RXA.15 substance lot number  → lotNumber
//	RXA.16 substance expiry date → expirationDate
//	RXA.17 substance manufacturer→ manufacturer (display only)
//	RXA.18 refusal reason (CE)   → statusReason
//	RXA.20 completion status     → status
//	RXA.21 action code           → "D" → entered-in-error
//
// RXR fields used:
//
//	RXR.1  route (CE)            → route CodeableConcept
//	RXR.2  site  (CWE)           → site  CodeableConcept
//
// OBX fields interpreted:
//
//	30973-2  Dose number in series    → protocolApplied.doseNumberPositiveInt
//	59782-3  Doses in primary series  → protocolApplied.seriesDosesPositiveInt
//	59783-1  Series status            → protocolApplied.series
//	69764-9  VIS document type        → education[].documentType
//	29768-9  Date VIS published       → education[].publicationDate
//	29769-7  Date VIS presented       → education[].presentationDate
//	64994-7  Funding program          → programEligibility[].programStatus
//	Others                            → standalone Observation (returned separately)
func buildImmunizationFromGroup(
	grp vaccinationGroup,
	patientRef string,
	facilityNS string,
	assemblyRules AssemblyRules,
) (map[string]interface{}, []map[string]interface{}) {
	rxa := grp.rxa

	// ── id ───────────────────────────────────────────────────────────────────
	subID := SegFieldValue(rxa, "RXA.1")
	if subID == "" {
		subID = "1"
	}
	immID := "imm-" + subID

	imm := map[string]interface{}{
		"resourceType": "Immunization",
		"id":           immID,
	}

	// ── status (1..1) ────────────────────────────────────────────────────────
	status := RXA20ToImmunizationStatus(SegFieldValue(rxa, "RXA.20"))
	// Action code "D" overrides status to entered-in-error
	if strings.ToUpper(strings.TrimSpace(SegFieldValue(rxa, "RXA.21"))) == "D" {
		status = "entered-in-error"
	}
	imm["status"] = status

	// ── statusReason (when not-done / entered-in-error) ──────────────────────
	if status == "not-done" {
		if refusal := SegFieldValue(rxa, "RXA.18"); refusal != "" {
			cc, _ := BuildCodeableConceptFromCE(refusal, facilityNS)
			imm["statusReason"] = cc
		}
	}

	// ── vaccineCode (1..1) ───────────────────────────────────────────────────
	rxa5 := SegFieldValue(rxa, "RXA.5")
	if rxa5 != "" {
		imm["vaccineCode"] = buildVaccineCode(rxa5, facilityNS)
	} else {
		imm["vaccineCode"] = map[string]interface{}{"text": "Unknown vaccine"}
	}

	// ── patient (1..1) ───────────────────────────────────────────────────────
	if patientRef != "" {
		imm["patient"] = map[string]interface{}{"reference": patientRef}
	}

	// ── occurrenceDateTime (1..1) ────────────────────────────────────────────
	if dt := SegFieldValue(rxa, "RXA.3"); dt != "" {
		imm["occurrenceDateTime"] = ToISO(dt)
	} else {
		// FHIR requires occurrence — default to current convention marker
		imm["occurrenceDateTime"] = "unknown"
	}

	// ── doseQuantity ─────────────────────────────────────────────────────────
	if amt := SegFieldValue(rxa, "RXA.6"); amt != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(amt), 64); err == nil && f > 0 {
			dq := map[string]interface{}{"value": f}
			if unit := SegFieldValue(rxa, "RXA.7"); unit != "" {
				parts := strings.Split(unit, "^")
				dq["unit"] = parts[0]
				if len(parts) >= 3 && parts[2] != "" {
					dq["system"] = "http://unitsofmeasure.org"
					dq["code"] = parts[0]
				}
			}
			imm["doseQuantity"] = dq
		}
	}

	// ── performer (administering provider, RXA.10 XCN) ───────────────────────
	if xcn := SegFieldValue(rxa, "RXA.10"); xcn != "" {
		actor := xcnToRef(xcn)
		if len(actor) > 0 {
			imm["performer"] = []interface{}{
				map[string]interface{}{
					"function": map[string]interface{}{
						"coding": []interface{}{map[string]interface{}{
							"system":  "http://terminology.hl7.org/CodeSystem/v2-0443",
							"code":    "AP",
							"display": "Administering Provider",
						}},
					},
					"actor": actor,
				},
			}
		}
	}

	// ── location (RXA.11 — display only) ─────────────────────────────────────
	if loc := SegFieldValue(rxa, "RXA.11"); loc != "" {
		// RXA.11 is LA2 — a complex location data type; we surface the free-text
		// component (sub-field 9 or raw value) as a display-only reference.
		display := extractLA2Display(loc)
		if display != "" {
			imm["location"] = map[string]interface{}{"display": display}
		}
	}

	// ── lotNumber ────────────────────────────────────────────────────────────
	if lot := SegFieldValue(rxa, "RXA.15"); lot != "" {
		imm["lotNumber"] = lot
	}

	// ── expirationDate ───────────────────────────────────────────────────────
	if exp := SegFieldValue(rxa, "RXA.16"); exp != "" {
		imm["expirationDate"] = ToISO(exp)
	}

	// ── manufacturer (RXA.17 — display-only reference) ───────────────────────
	if mfr := SegFieldValue(rxa, "RXA.17"); mfr != "" {
		display := extractCEDisplay(mfr)
		if display != "" {
			imm["manufacturer"] = map[string]interface{}{"display": display}
		}
	}

	// ── note (RXA.9 — administration notes, repeating CE) ────────────────────
	if notes := SegFieldValue(rxa, "RXA.9"); notes != "" {
		var noteItems []interface{}
		for _, rep := range strings.Split(notes, "~") {
			rep = strings.TrimSpace(rep)
			if rep == "" {
				continue
			}
			noteText := extractCEDisplay(rep)
			if noteText == "" {
				noteText = rep
			}
			noteItems = append(noteItems, map[string]interface{}{"text": noteText})
		}
		if len(noteItems) > 0 {
			imm["note"] = noteItems
		}
	}

	// ── route (RXR.1) ────────────────────────────────────────────────────────
	if grp.rxr != nil {
		if routeRaw := SegFieldValue(*grp.rxr, "RXR.1"); routeRaw != "" {
			imm["route"] = buildRouteCodeableConcept(routeRaw, facilityNS)
		}
		if siteRaw := SegFieldValue(*grp.rxr, "RXR.2"); siteRaw != "" {
			imm["site"] = buildSiteCodeableConcept(siteRaw, facilityNS)
		}
	}

	// ── OBX processing ───────────────────────────────────────────────────────
	// Some OBX codes contribute to Immunization structure (protocolApplied,
	// education, programEligibility); all others become standalone Observations.
	var protocol map[string]interface{} // single protocolApplied entry
	var education []interface{}
	var programEligibility []interface{}
	var supportingInfoRefs []interface{}
	var supportingObs []map[string]interface{}

	// Track which OBX LOINC codes feed into Immunization structure.
	immunizationOBXCodes := map[string]bool{
		"30973-2": true, // dose number
		"59782-3": true, // series doses
		"59783-1": true, // series status
		"69764-9": true, // VIS document type
		"29768-9": true, // VIS published date
		"29769-7": true, // VIS presented date
		"64994-7": true, // funding program eligibility
	}

	for _, obx := range grp.obxes {
		loincCode := extractLOINCFromOBX3(SegFieldValue(obx, "OBX.3"))
		obsValue := SegFieldValue(obx, "OBX.5")

		switch loincCode {
		case "30973-2": // Dose number within series
			if protocol == nil {
				protocol = map[string]interface{}{}
			}
			if n, err := strconv.Atoi(strings.TrimSpace(obsValue)); err == nil {
				protocol["doseNumberPositiveInt"] = n
			} else {
				protocol["doseNumberString"] = obsValue
			}

		case "59782-3": // Number of doses in primary series
			if protocol == nil {
				protocol = map[string]interface{}{}
			}
			if n, err := strconv.Atoi(strings.TrimSpace(obsValue)); err == nil {
				protocol["seriesDosesPositiveInt"] = n
			} else {
				protocol["seriesDosesString"] = obsValue
			}

		case "59783-1": // Status in immunization series
			if protocol == nil {
				protocol = map[string]interface{}{}
			}
			protocol["series"] = extractCEDisplay(obsValue)

		case "69764-9": // VIS document type
			// Starts or reuses the current education entry.
			edu := currentOrNewEducation(&education)
			cc, _ := BuildCodeableConceptFromCE(obsValue, facilityNS)
			edu["documentType"] = cc

		case "29768-9": // Date VIS information statement published
			edu := currentOrNewEducation(&education)
			edu["publicationDate"] = ToISO(strings.TrimSpace(obsValue))

		case "29769-7": // Date VIS information statement presented
			edu := currentOrNewEducation(&education)
			edu["presentationDate"] = ToISO(strings.TrimSpace(obsValue))

		case "64994-7": // Vaccine funding program eligibility
			if obsValue != "" {
				cc, _ := BuildCodeableConceptFromCE(obsValue, facilityNS)
				programEligibility = append(programEligibility, map[string]interface{}{
					"programStatus": cc,
				})
			}

		default:
			if immunizationOBXCodes[loincCode] {
				// Already handled above — skip.
				continue
			}
			// Build a standalone Observation.
			obs, obsID, ok := BuildObservationFromOBX(obx, patientRef, assemblyRules)
			if ok {
				// Link back to Immunization via partOf extension.
				obs["partOf"] = []interface{}{
					map[string]interface{}{"reference": "Immunization/" + immID},
				}
				supportingObs = append(supportingObs, obs)
				supportingInfoRefs = append(supportingInfoRefs, map[string]interface{}{
					"reference": "Observation/" + obsID,
				})
			}
		}
	}

	// Attach protocol if we have any protocol data.
	if protocol != nil {
		// FHIR R4 requires targetDisease or series on protocolApplied.
		// Use vaccine code display as a fallback series name.
		if _, hasSeriesDoses := protocol["doseNumberPositiveInt"]; hasSeriesDoses {
			if _, hasSeries := protocol["series"]; !hasSeries {
				if vc, ok := imm["vaccineCode"].(map[string]interface{}); ok {
					if txt, ok2 := vc["text"].(string); ok2 && txt != "" {
						protocol["series"] = txt + " series"
					}
				}
			}
		}
		imm["protocolApplied"] = []interface{}{protocol}
	}

	if len(education) > 0 {
		imm["education"] = education
	}
	if len(programEligibility) > 0 {
		imm["programEligibility"] = programEligibility
	}
	if len(supportingInfoRefs) > 0 {
		imm["supportingInformation"] = supportingInfoRefs
	}

	// ── Narrative ─────────────────────────────────────────────────────────────
	imm["text"] = map[string]interface{}{
		"status": "generated",
		"div":    sharedNarrativeGenerator.Generate(imm, nil, nil),
	}

	return imm, supportingObs
}

// ──────────────────────────────────────────────────────────────────────────────
// Coding helpers
// ──────────────────────────────────────────────────────────────────────────────

// buildVaccineCode builds a FHIR CodeableConcept from RXA.5.
// RXA.5 is a CE: code^display^codingSystem
// The coding system component maps to the vaccine code system (CVX, NDC, etc.).
func buildVaccineCode(rxa5 string, facilityNS string) map[string]interface{} {
	parts := strings.Split(rxa5, "^")
	code := ""
	display := ""
	systemAbbr := ""
	if len(parts) >= 1 {
		code = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 {
		display = strings.TrimSpace(parts[1])
	}
	if len(parts) >= 3 {
		systemAbbr = strings.TrimSpace(parts[2])
	}

	systemURI := rxaVaccineSystemToURI(systemAbbr)
	if systemURI == "" {
		systemURI = facilityNS
	}

	coding := map[string]interface{}{"code": code, "system": systemURI}
	// Only embed display for local/facility systems — for CVX/NDC the local
	// abbreviated name will differ from the official terminology display.
	if !isLocalCodingSystem(systemURI) {
		// Well-known terminology: omit display (FHIR validator enforces canonical name)
	} else if display != "" {
		coding["display"] = display
	}

	cc := map[string]interface{}{
		"coding": []interface{}{coding},
	}
	if display != "" {
		cc["text"] = display
	}
	return cc
}

// buildRouteCodeableConcept maps an RXR.1 CE value to a FHIR CodeableConcept
// using HL7 Table 0162 as the coding system.
func buildRouteCodeableConcept(routeCE string, facilityNS string) map[string]interface{} {
	parts := strings.Split(routeCE, "^")
	code := strings.TrimSpace(parts[0])
	display := ""
	if len(parts) >= 2 {
		display = strings.TrimSpace(parts[1])
	}

	// Use mapped display if we have it.
	if m, ok := routeCodeMap[strings.ToUpper(code)]; ok {
		if display == "" {
			display = m.display
		}
	}

	const routeSystem = "http://terminology.hl7.org/CodeSystem/v2-0162"
	coding := map[string]interface{}{"system": routeSystem, "code": code}
	if display != "" {
		coding["display"] = display
	}

	cc := map[string]interface{}{"coding": []interface{}{coding}}
	if display != "" {
		cc["text"] = display
	}
	return cc
}

// buildSiteCodeableConcept maps an RXR.2 CWE value to a FHIR CodeableConcept
// using HL7 Table 0163 as the coding system.
func buildSiteCodeableConcept(siteCWE string, facilityNS string) map[string]interface{} {
	parts := strings.Split(siteCWE, "^")
	code := strings.TrimSpace(parts[0])
	display := ""
	if len(parts) >= 2 {
		display = strings.TrimSpace(parts[1])
	}

	if display == "" {
		if d, ok := siteCodeMap[strings.ToUpper(code)]; ok {
			display = d
		}
	}

	const siteSystem = "http://terminology.hl7.org/CodeSystem/v2-0163"
	coding := map[string]interface{}{"system": siteSystem, "code": code}
	if display != "" {
		coding["display"] = display
	}

	cc := map[string]interface{}{"coding": []interface{}{coding}}
	if display != "" {
		cc["text"] = display
	}
	return cc
}

// ──────────────────────────────────────────────────────────────────────────────
// OBX helpers
// ──────────────────────────────────────────────────────────────────────────────

// extractLOINCFromOBX3 returns the LOINC code from OBX.3, which can be a plain
// code or a CE/CWE: code^display^system.  Returns "" for non-LOINC systems.
func extractLOINCFromOBX3(obx3 string) string {
	parts := strings.Split(obx3, "^")
	if len(parts) == 0 {
		return ""
	}
	code := strings.TrimSpace(parts[0])
	// If a system is specified and it's not LOINC, return "".
	if len(parts) >= 3 {
		sys := strings.ToUpper(strings.TrimSpace(parts[2]))
		if sys != "" && sys != "LN" && sys != "LOINC" {
			return ""
		}
	}
	return code
}

// currentOrNewEducation returns the last education entry from the slice if it
// exists, or appends a new one and returns it.  Education entries are built
// incrementally as OBX codes are processed.
func currentOrNewEducation(education *[]interface{}) map[string]interface{} {
	if len(*education) > 0 {
		if last, ok := (*education)[len(*education)-1].(map[string]interface{}); ok {
			return last
		}
	}
	entry := map[string]interface{}{}
	*education = append(*education, entry)
	return entry
}

// extractCEDisplay returns the display component (^2) from a CE/CWE string.
// Falls back to the code component (^1) when the display is blank.
func extractCEDisplay(ce string) string {
	parts := strings.Split(ce, "^")
	if len(parts) >= 2 && strings.TrimSpace(parts[1]) != "" {
		return strings.TrimSpace(parts[1])
	}
	if len(parts) >= 1 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

// extractLA2Display extracts a human-readable location string from an LA2
// (Location with address) composite.  LA2 is:
//   1^2^3^4^5^6^7^8^address-1^address-2^city^state^zip^country
// We use component 9 (address-1) or fall back to component 1 (point-of-care).
func extractLA2Display(la2 string) string {
	parts := strings.Split(la2, "^")
	// Component 9 = address line 1 (0-indexed: parts[8])
	if len(parts) >= 9 && strings.TrimSpace(parts[8]) != "" {
		return strings.TrimSpace(parts[8])
	}
	// Component 1 = point-of-care / location ID
	if len(parts) >= 1 && strings.TrimSpace(parts[0]) != "" {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

// Immunization narrative generation — see sharedNarrativeGenerator in
// assembly.go. Used to have its own hand-written builder here; now delegates
// to services/fhir_narrative's dedicated Immunization builder.
