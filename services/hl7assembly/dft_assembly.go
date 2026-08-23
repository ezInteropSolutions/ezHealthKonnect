// services/hl7assembly/dft_assembly.go
//
// Structural assembly for DFT (Detailed Financial Transaction) messages.
//
// DFT^P03 messages post charge-level financial transactions.  The relevant
// segment groups are:
//
//	FT1* — Financial transaction (one per charge line item)
//	PR1* — Procedure associated with the charge (paired with FT1 by set-ID)
//	DG1* — Diagnosis codes linked to the charges
//
// Correct FHIR R4 mapping:
//
//	FT1  → ChargeItem   (one per FT1 segment)
//	PR1  → Procedure    (one per PR1 segment; linked to same-set-ID ChargeItem)
//	DG1  → Condition    (one per DG1 segment; referenced by ChargeItem.reason)
//
// ChargeItem is the primary resource.  Procedure and Condition are supporting
// resources that the field-mapping engine may have already partially built via
// generic DG1/PR1 templates; AssembleDFTCharges removes those and replaces them
// with properly structured resources.
//
// Mandatory FHIR R4 ChargeItem fields satisfied here:
//   status  (1..1)  derived from FT1.6 (transaction type)
//   code    (1..1)  from FT1.7/FT1.8
//   subject (1..1)  Patient reference
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

// FT1TransactionTypeToChargeStatus maps FT1.6 (HL7 Table 0017) to FHIR R4
// ChargeItem.status values.
func FT1TransactionTypeToChargeStatus(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "CG": // Charge
		return "billable"
	case "CD", "CR": // Credit / Credit (returned merchandise)
		return "entered-in-error"
	case "PT": // Payment
		return "billed"
	case "AJ", "CO": // Adjustment / Co-payment
		return "billable"
	default:
		return "unknown"
	}
}

// DG1DiagnosisTypeToVerification maps DG1.6 (HL7 Table 0052) to the FHIR R4
// Condition.verificationStatus code.
func DG1DiagnosisTypeToVerification(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "F": // Final
		return "confirmed"
	case "W": // Working / preliminary
		return "provisional"
	case "A": // Admitting
		return "provisional"
	default:
		return "unconfirmed"
	}
}

// pr1CodingSystemToURI maps PR1.2 / PR1.3 explicit coding method strings to FHIR
// system URIs.  These are not HL7 Table 0396 abbreviations but free-text names
// that sending systems place in PR1.2 (e.g. "CPT", "ICD10PCS").
func pr1CodingSystemToURI(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "CPT", "CPT4", "C4":
		return "http://www.ama-assn.org/go/cpt"
	case "ICD10PCS", "ICD-10-PCS":
		return "http://www.cms.gov/Medicare/Coding/ICD10"
	case "ICD9", "ICD-9", "ICD9CM", "I9":
		return "http://hl7.org/fhir/sid/icd-9-cm"
	case "HCPCS":
		return "http://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets"
	case "SCT", "SNOMED":
		return "http://snomed.info/sct"
	default:
		return ""
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Main DFT assembly entry point
// ──────────────────────────────────────────────────────────────────────────────

// AssembleDFTCharges performs DFT^P03 structural assembly:
//
//  1. Skips silently when no FT1 segment is present (not a DFT message).
//  2. Derives the facility namespace from MSH.4 (Tier-3 coding fallback).
//  3. Builds one ChargeItem per FT1 segment.
//  4. Builds one Procedure per PR1 segment; links it to the same-set-ID
//     ChargeItem via ChargeItem.service.
//  5. Builds one Condition per DG1 segment.
//  6. Removes Procedure and Condition resources previously produced by the
//     field-mapping engine (they are incomplete without set-ID correlation).
//  7. Wires MessageHeader.focus to the first ChargeItem.
//
// Returns the updated resource list and advisory warnings.
func AssembleDFTCharges(
	parsedHL7Data map[string]interface{},
	resources []map[string]interface{},
	opts ...AssemblyRules,
) ([]map[string]interface{}, []string) {
	var rules AssemblyRules
	if len(opts) > 0 {
		rules = opts[0]
	}

	var warnings []string

	// ── Guard: only proceed when FT1 segments are present ────────────────────
	ft1List := ExtractSegmentGroup(parsedHL7Data, "FT1")
	if len(ft1List) == 0 {
		return resources, warnings
	}

	// ── Derive facility namespace (Tier-3 coding fallback) ────────────────────
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

	// ── Index PR1 segments by set-ID for fast lookup ──────────────────────────
	pr1List := ExtractSegmentGroup(parsedHL7Data, "PR1")
	pr1BySetID := make(map[string]hl7.EnhancedSegment, len(pr1List))
	for _, pr1 := range pr1List {
		if setID := SegFieldValue(pr1, "PR1.1"); setID != "" {
			pr1BySetID[setID] = pr1
		}
	}

	// ── Build Conditions from DG1 ─────────────────────────────────────────────
	dg1List := ExtractSegmentGroup(parsedHL7Data, "DG1")
	var conditions []map[string]interface{}
	// condRefBySetID maps DG1.1 set-ID → "Condition/<id>" for ChargeItem.reason
	condRefBySetID := make(map[string]string, len(dg1List))

	for _, dg1 := range dg1List {
		cond, condID := buildConditionFromDG1(dg1, patientRef, rules.FacilityNamespace)
		if cond != nil {
			conditions = append(conditions, cond)
			if setID := SegFieldValue(dg1, "DG1.1"); setID != "" {
				condRefBySetID[setID] = "Condition/" + condID
			}
		}
	}

	// ── Build ChargeItems from FT1 + Procedures from paired PR1 ──────────────
	var chargeItems []map[string]interface{}
	var procedures []map[string]interface{}

	for _, ft1 := range ft1List {
		setID := SegFieldValue(ft1, "FT1.1")

		// Build ChargeItem
		ci := buildChargeItemFromFT1(ft1, patientRef, rules.FacilityNamespace, condRefBySetID)
		chargeItems = append(chargeItems, ci)

		// Build paired Procedure (same set-ID)
		if pr1, ok := pr1BySetID[setID]; ok {
			proc := buildProcedureFromPR1(pr1, patientRef, rules.FacilityNamespace)
			if proc != nil {
				// Link ChargeItem → Procedure.
				// ChargeItem.service is 0..* in FHIR R4 — must be an array.
				if procID, ok2 := proc["id"].(string); ok2 {
					ci["service"] = []interface{}{
						map[string]interface{}{"reference": "Procedure/" + procID},
					}
				}
				procedures = append(procedures, proc)
			}
		}
	}

	// ── Build standalone Procedures for any PR1 not paired with an FT1 ────────
	pairedSetIDs := make(map[string]bool, len(ft1List))
	for _, ft1 := range ft1List {
		if sid := SegFieldValue(ft1, "FT1.1"); sid != "" {
			pairedSetIDs[sid] = true
		}
	}
	for _, pr1 := range pr1List {
		if sid := SegFieldValue(pr1, "PR1.1"); !pairedSetIDs[sid] {
			proc := buildProcedureFromPR1(pr1, patientRef, rules.FacilityNamespace)
			if proc != nil {
				procedures = append(procedures, proc)
			}
		}
	}

	// ── Wire Procedure.reasonReference → all Conditions ─────────────────────
	// All DG1 diagnoses apply to all charges in a DFT.  Adding reasonReference
	// to each Procedure makes every Condition reachable by graph traversal from
	// MessageHeader.focus → ChargeItem.service → Procedure.reasonReference →
	// Condition, satisfying the FHIR message-bundle reachability requirement.
	var conditionRefs []interface{}
	for _, cond := range conditions {
		if id, ok := cond["id"].(string); ok {
			conditionRefs = append(conditionRefs, map[string]interface{}{
				"reference": "Condition/" + id,
			})
		}
	}
	if len(conditionRefs) > 0 {
		for _, proc := range procedures {
			proc["reasonReference"] = conditionRefs
		}
	}

	// ── Rebuild resource list ─────────────────────────────────────────────────
	// Drop:   Procedure and Condition resources from the field-mapping engine
	//         (incomplete — no set-ID correlation or reason[]/service links).
	// Update: MessageHeader.focus → ALL ChargeItems (not just the first) so
	//         every ChargeItem is reachable by graph traversal.
	// Add:    ChargeItems, Procedures, Conditions
	var result []map[string]interface{}

	// Pre-build focus refs from all assembled ChargeItems.
	var focusRefs []interface{}
	for _, ci := range chargeItems {
		if id, ok := ci["id"].(string); ok {
			focusRefs = append(focusRefs, map[string]interface{}{"reference": "ChargeItem/" + id})
		}
	}

	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		switch rt {
		case "Procedure", "Condition":
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

	result = append(result, chargeItems...)
	result = append(result, procedures...)
	result = append(result, conditions...)

	log.Printf("📋 hl7.assemble_dft: built %d ChargeItem(s), %d Procedure(s), %d Condition(s)",
		len(chargeItems), len(procedures), len(conditions))

	return result, warnings
}

// ──────────────────────────────────────────────────────────────────────────────
// Per-segment builders
// ──────────────────────────────────────────────────────────────────────────────

// buildChargeItemFromFT1 maps one FT1 segment to a FHIR R4 ChargeItem.
//
// FT1 fields used:
//
//	FT1.1  set-ID          → id suffix
//	FT1.2  transaction ID  → id (preferred)
//	FT1.4  transaction date → occurrenceDateTime
//	FT1.6  transaction type → status
//	FT1.7  transaction code → code.coding (CE: code^display^system)
//	FT1.8  description      → code.text
//	FT1.10 quantity         → quantity.value
//	FT1.11 unit price       → priceOverride (extension — informational)
//	FT1.19 diagnosis code(s)→ reason[] CodeableConcept (repeating, ~ separated)
//	FT1.20 performed by     → performer[0].actor (display only)
func buildChargeItemFromFT1(
	ft1 hl7.EnhancedSegment,
	patientRef string,
	facilityNS string,
	condRefBySetID map[string]string,
) map[string]interface{} {
	ci := map[string]interface{}{
		"resourceType": "ChargeItem",
		"id":           ft1ChargeItemID(ft1),
	}

	// status (1..1)
	ci["status"] = FT1TransactionTypeToChargeStatus(SegFieldValue(ft1, "FT1.6"))

	// code (1..1) — FT1.7 is CE: code^display^system
	ft17 := SegFieldValue(ft1, "FT1.7")
	ft18 := SegFieldValue(ft1, "FT1.8")
	if ft17 != "" {
		cc, _ := BuildCodeableConceptFromCE(ft17, facilityNS)
		// Ensure text is the human-readable description
		if ft18 != "" {
			cc["text"] = ft18
			// Also populate display on the coding if it's missing
			if codings, ok := cc["coding"].([]interface{}); ok && len(codings) > 0 {
				if c, ok2 := codings[0].(map[string]interface{}); ok2 {
					if c["display"] == nil || c["display"] == "" {
						c["display"] = ft18
					}
				}
			}
		}
		ci["code"] = cc
	} else if ft18 != "" {
		// No code, only description — still need a code element
		ci["code"] = map[string]interface{}{"text": ft18}
	} else {
		ci["code"] = map[string]interface{}{"text": "Financial transaction"}
	}

	// subject (1..1)
	if patientRef != "" {
		ci["subject"] = map[string]interface{}{"reference": patientRef}
	}

	// occurrenceDateTime from FT1.4
	if dt := SegFieldValue(ft1, "FT1.4"); dt != "" {
		ci["occurrenceDateTime"] = ToISO(dt)
	}

	// quantity from FT1.10
	if qty := SegFieldValue(ft1, "FT1.10"); qty != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(qty), 64); err == nil {
			ci["quantity"] = map[string]interface{}{"value": f}
		}
	}

	// performer from FT1.20 (XCN — display only, same pattern as xcnToRef)
	if perf := SegFieldValue(ft1, "FT1.20"); perf != "" {
		actor := xcnToRef(perf)
		if len(actor) > 0 {
			ci["performer"] = []interface{}{
				map[string]interface{}{"actor": actor},
			}
		}
	}

	// reason[] from FT1.19 (repeating CE codes, ~ separator)
	if diagRaw := SegFieldValue(ft1, "FT1.19"); diagRaw != "" {
		var reasons []interface{}
		for _, rep := range strings.Split(diagRaw, "~") {
			rep = strings.TrimSpace(rep)
			if rep == "" {
				continue
			}
			cc, _ := BuildCodeableConceptFromCE(rep, facilityNS)
			reasons = append(reasons, cc)
		}
		if len(reasons) > 0 {
			ci["reason"] = reasons
		}
	}

	// narrative
	ci["text"] = map[string]interface{}{
		"status": "generated",
		"div":    sharedNarrativeGenerator.Generate(ci, nil, nil),
	}

	return ci
}

// buildProcedureFromPR1 maps one PR1 segment to a FHIR R4 Procedure.
//
// PR1 fields used:
//
//	PR1.1  set-ID             → id suffix
//	PR1.2  coding method      → code.coding[0].system (PR1-specific system)
//	PR1.3  procedure code     → code.coding[0].code
//	PR1.4  procedure desc     → code.text
//	PR1.5  procedure date     → performedDateTime
//	PR1.12 procedure practitioner → performer[0].actor (display only)
//	PR1.13 diagnosis codes    → reasonCode[] (repeating CE, ~ separated)
func buildProcedureFromPR1(
	pr1 hl7.EnhancedSegment,
	patientRef string,
	facilityNS string,
) map[string]interface{} {
	code := SegFieldValue(pr1, "PR1.3")
	desc := SegFieldValue(pr1, "PR1.4")
	if code == "" && desc == "" {
		return nil
	}

	setID := SegFieldValue(pr1, "PR1.1")
	if setID == "" {
		setID = "1"
	}

	proc := map[string]interface{}{
		"resourceType": "Procedure",
		"id":           "proc-" + setID,
		"status":       "completed", // DFT posts completed charges
	}

	// subject (1..1)
	if patientRef != "" {
		proc["subject"] = map[string]interface{}{"reference": patientRef}
	}

	// code (1..1) — PR1.2 carries the coding method explicitly
	codingMethod := SegFieldValue(pr1, "PR1.2")
	codingSystem := pr1CodingSystemToURI(codingMethod)
	if codingSystem == "" {
		// Try as HL7 Table 0396 abbreviation via NormalizeCodingSystem
		codingSystem, _ = NormalizeCodingSystem(codingMethod)
	}
	if codingSystem == "" {
		codingSystem = facilityNS
	}

	coding := map[string]interface{}{"code": code}
	if codingSystem != "" {
		coding["system"] = codingSystem
	}
	// Only embed display in the coding when the system is a local/facility
	// namespace.  For official external terminologies (CPT, ICD, SNOMED, HCPCS)
	// the local abbreviated description from PR1.4 will not match the official
	// display text stored in the terminology server, causing a validation error.
	// The description is still preserved in code.text (see below).
	if desc != "" && isLocalCodingSystem(codingSystem) {
		coding["display"] = desc
	}

	codeCC := map[string]interface{}{
		"coding": []interface{}{coding},
	}
	if desc != "" {
		codeCC["text"] = desc // always preserve local description as text
	}
	proc["code"] = codeCC

	// performedDateTime from PR1.5
	if dt := SegFieldValue(pr1, "PR1.5"); dt != "" {
		proc["performedDateTime"] = ToISO(dt)
	}

	// performer from PR1.12 (XCN — display only)
	if xcn := SegFieldValue(pr1, "PR1.12"); xcn != "" {
		actor := xcnToRef(xcn)
		if len(actor) > 0 {
			proc["performer"] = []interface{}{
				map[string]interface{}{"actor": actor},
			}
		}
	}

	// reasonCode[] from PR1.13 (repeating CE, ~ separator)
	if diagRaw := SegFieldValue(pr1, "PR1.13"); diagRaw != "" {
		var reasons []interface{}
		for _, rep := range strings.Split(diagRaw, "~") {
			rep = strings.TrimSpace(rep)
			if rep == "" {
				continue
			}
			cc, _ := BuildCodeableConceptFromCE(rep, facilityNS)
			reasons = append(reasons, cc)
		}
		if len(reasons) > 0 {
			proc["reasonCode"] = reasons
		}
	}

	// narrative
	proc["text"] = map[string]interface{}{
		"status": "generated",
		"div":    sharedNarrativeGenerator.Generate(proc, nil, nil),
	}

	return proc
}

// buildConditionFromDG1 maps one DG1 segment to a FHIR R4 Condition.
// Returns the resource map and its FHIR id string.
//
// DG1 fields used:
//
//	DG1.1  set-ID             → id suffix
//	DG1.3  diagnosis code     → code (CE: code^display^system)
//	DG1.4  diagnosis desc     → code.text (when non-empty)
//	DG1.5  diagnosis date     → recordedDate
//	DG1.6  diagnosis type     → verificationStatus
//	DG1.17 diagnosing clinician → asserter (display only)
func buildConditionFromDG1(
	dg1 hl7.EnhancedSegment,
	patientRef string,
	facilityNS string,
) (map[string]interface{}, string) {
	diagCE := SegFieldValue(dg1, "DG1.3")
	diagDesc := SegFieldValue(dg1, "DG1.4")
	if diagCE == "" && diagDesc == "" {
		return nil, ""
	}

	setID := SegFieldValue(dg1, "DG1.1")
	if setID == "" {
		setID = "1"
	}
	condID := "cond-" + setID

	cond := map[string]interface{}{
		"resourceType": "Condition",
		"id":           condID,
	}

	// clinicalStatus (1..1 in R4) — default to active for posted charges
	cond["clinicalStatus"] = map[string]interface{}{
		"coding": []interface{}{map[string]interface{}{
			"system": "http://terminology.hl7.org/CodeSystem/condition-clinical",
			"code":   "active",
		}},
	}

	// verificationStatus from DG1.6
	verCode := DG1DiagnosisTypeToVerification(SegFieldValue(dg1, "DG1.6"))
	cond["verificationStatus"] = map[string]interface{}{
		"coding": []interface{}{map[string]interface{}{
			"system": "http://terminology.hl7.org/CodeSystem/condition-ver-status",
			"code":   verCode,
		}},
	}

	// subject (1..1)
	if patientRef != "" {
		cond["subject"] = map[string]interface{}{"reference": patientRef}
	}

	// code — DG1.3 is CE; DG1.4 provides a plain-text override
	if diagCE != "" {
		cc, _ := BuildCodeableConceptFromCE(diagCE, facilityNS)
		if diagDesc != "" {
			cc["text"] = diagDesc
		}
		cond["code"] = cc
	} else {
		cond["code"] = map[string]interface{}{"text": diagDesc}
	}

	// recordedDate from DG1.5
	if dt := SegFieldValue(dg1, "DG1.5"); dt != "" {
		cond["recordedDate"] = ToISO(dt)
	}

	// asserter from DG1.17 (XCN — display only)
	if xcn := SegFieldValue(dg1, "DG1.17"); xcn != "" {
		actor := xcnToRef(xcn)
		if len(actor) > 0 {
			cond["asserter"] = actor
		}
	}

	// narrative
	cond["text"] = map[string]interface{}{
		"status": "generated",
		"div":    sharedNarrativeGenerator.Generate(cond, nil, nil),
	}

	return cond, condID
}

// ──────────────────────────────────────────────────────────────────────────────
// ID helpers
// ──────────────────────────────────────────────────────────────────────────────

// isLocalCodingSystem returns true when the system URI is a local/facility
// namespace rather than an official external terminology.  Display text from
// local source data can be embedded in the coding for local systems; for
// official terminologies (CPT, ICD-9/10, SNOMED, HCPCS) only the code is
// reliable — the display must match the official term definition exactly.
func isLocalCodingSystem(system string) bool {
	officialPrefixes := []string{
		"http://www.ama-assn.org/go/cpt",
		"http://www.cms.gov/Medicare/Coding/",
		"http://hl7.org/fhir/sid/icd-",
		"http://snomed.info/sct",
		"http://loinc.org",
		"http://terminology.hl7.org/CodeSystem/",
		"http://hl7.org/fhir/",
	}
	for _, prefix := range officialPrefixes {
		if strings.HasPrefix(system, prefix) {
			return false
		}
	}
	return true // urn:facility:*, urn:oid:*, custom URNs → local
}

// ft1ChargeItemID returns a deterministic FHIR id for a ChargeItem.
// Prefers FT1.2 (transaction ID), then FT1.1 (set-ID).
func ft1ChargeItemID(ft1 hl7.EnhancedSegment) string {
	if txID := SegFieldValue(ft1, "FT1.2"); txID != "" {
		return sanitizeID("charge-" + txID + "-" + SegFieldValue(ft1, "FT1.1"))
	}
	if setID := SegFieldValue(ft1, "FT1.1"); setID != "" {
		return "charge-" + setID
	}
	return "charge-1"
}

// ──────────────────────────────────────────────────────────────────────────────
// Narrative generation — see sharedNarrativeGenerator in assembly.go. ChargeItem,
// Procedure, and Condition narratives used to have their own hand-written
// builders here; all three now delegate to services/fhir_narrative (Procedure
// and Condition get that package's dedicated builders; ChargeItem renders
// generically).
// ──────────────────────────────────────────────────────────────────────────────
