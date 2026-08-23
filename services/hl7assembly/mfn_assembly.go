// services/hl7assembly/mfn_assembly.go
//
// Structural assembly for MFN (Master File Notification) messages.
//
// MFN messages distribute master file updates to downstream systems.  The
// file type is declared in MFI.1 (master file identifier); the record action
// in MFE.1 (add / update / delete / deactivate).  Payload segments vary by
// file type:
//
//	MFI.1 = EMP  → ZEM (employer/org) → Organization
//	MFI.1 = STF  → STF + PRA          → Practitioner
//	MFI.1 = PRA  → PRA + STF          → Practitioner
//	MFI.1 = LOC  → LOC + LDP          → Location
//	MFI.1 = CDM  → CDM                → ChargeItemDefinition (stub)
//	Other / M13  → Z-segments         → Organization (fallback)
//
// MFE.1 action codes (HL7 Table 0180):
//
//	MAD → add    → resource.active = true
//	MUP → update → resource.active = true
//	MDL → delete → resource.active = false
//	MDC → deactivate → resource.active = false
package hl7assembly

import (
	"fmt"
	"log"
	"strings"

	"ezhealthkonnect/hl7"
)

// ──────────────────────────────────────────────────────────────────────────────
// Action-code mapper
// ──────────────────────────────────────────────────────────────────────────────

// mfe1ToActive maps MFE.1 (HL7 Table 0180) to a boolean active flag.
func mfe1ToActive(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "MDL", "MDC": // Delete / Deactivate
		return false
	default: // MAD (Add), MUP (Update), or unknown → treat as active
		return true
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Main assembly entry point
// ──────────────────────────────────────────────────────────────────────────────

// AssembleMFNResources performs MFN structural assembly:
//
//  1. Reads MFI.1 to determine the master file type.
//  2. Reads MFE.1 for the record action (add/update/delete/deactivate).
//  3. Dispatches to the appropriate per-type builder.
//  4. Updates MessageHeader.focus to the assembled resource.
//
// Returns the updated resource list and advisory warnings.
func AssembleMFNResources(
	parsedHL7Data map[string]interface{},
	resources []map[string]interface{},
	opts ...AssemblyRules,
) ([]map[string]interface{}, []string) {
	var rules AssemblyRules
	if len(opts) > 0 {
		rules = opts[0]
	}

	var warnings []string

	// ── MFI: master file identification ──────────────────────────────────────
	mfiList := ExtractSegmentGroup(parsedHL7Data, "MFI")
	if len(mfiList) == 0 {
		warnings = append(warnings, "MFN message has no MFI segment — cannot determine master file type")
		return resources, warnings
	}
	mfi := mfiList[0]
	masterFileID := strings.ToUpper(strings.TrimSpace(SegFieldValue(mfi, "MFI.1")))

	// ── MFE: master file entry (record action + primary key) ─────────────────
	mfeList := ExtractSegmentGroup(parsedHL7Data, "MFE")
	recordAction := "MUP" // default: update
	primaryKey := ""
	if len(mfeList) > 0 {
		recordAction = SegFieldValue(mfeList[0], "MFE.1")
		primaryKey = strings.TrimSpace(SegFieldValue(mfeList[0], "MFE.4"))
	}
	active := mfe1ToActive(recordAction)

	// ── Facility namespace ────────────────────────────────────────────────────
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

	// ── Dispatch by master file type ─────────────────────────────────────────
	var assembled map[string]interface{}
	var resourceType string

	switch masterFileID {
	case "STF", "PRA":
		assembled, resourceType = buildPractitionerFromSTF(parsedHL7Data, primaryKey, active, rules)

	case "LOC":
		assembled, resourceType = buildLocationFromLOC(parsedHL7Data, primaryKey, active, rules)

	case "CDM":
		assembled, resourceType = buildChargeItemDefinitionFromCDM(parsedHL7Data, primaryKey, active, rules)
		if assembled == nil {
			warnings = append(warnings, "CDM master file: ChargeItemDefinition assembly not fully implemented; skipping")
			return resources, warnings
		}

	default:
		// EMP, M13 generic, or any Z-segment-based master file → Organization
		assembled, resourceType = buildOrganizationFromMFN(parsedHL7Data, primaryKey, active, masterFileID, rules)
	}

	if assembled == nil {
		warnings = append(warnings,
			fmt.Sprintf("MFN assembly: no resource built for master file type '%s' (key=%s)", masterFileID, primaryKey))
		return resources, warnings
	}

	// ── Update MessageHeader.focus ────────────────────────────────────────────
	resourceID, _ := assembled["id"].(string)
	var result []map[string]interface{}
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		// Drop any prior resource of the same type (field-mapper may have
		// produced a partial version).
		if rt == resourceType {
			continue
		}
		// For STF/PRA assembly: wire any field-mapper PractitionerRole back to the
		// assembled Practitioner so it is reachable from MessageHeader via backward
		// reference traversal.  PractitionerRole.practitioner → Practitioner/id.
		if resourceType == "Practitioner" && rt == "PractitionerRole" && resourceID != "" {
			r["practitioner"] = map[string]interface{}{
				"reference": "Practitioner/" + resourceID,
			}
		}
		if rt == "MessageHeader" && resourceID != "" {
			r["focus"] = []interface{}{
				map[string]interface{}{"reference": resourceType + "/" + resourceID},
			}
		}
		result = append(result, r)
	}
	result = append(result, assembled)

	log.Printf("📋 hl7.assemble_mfn: built %s/%s (fileType=%s, action=%s)",
		resourceType, resourceID, masterFileID, recordAction)

	return result, warnings
}

// ──────────────────────────────────────────────────────────────────────────────
// Organization builder  (EMP / generic Z-segment master files)
// ──────────────────────────────────────────────────────────────────────────────

// buildOrganizationFromMFN builds a FHIR R4 Organization from an MFN message.
// It first tries to find a ZEM (employer Z-segment); if absent it looks for
// EB (insurance/employer benefit) or uses whatever MFE data is available.
//
// ZEM fields (site-specific employer segment):
//
//	ZEM.1  employer ID  → Organization.identifier
//	ZEM.2  name         → Organization.name
//	ZEM.3  address (XAD)→ Organization.address[]
//	ZEM.4  plan type    → Organization.type[].text
//	ZEM.5  phone        → Organization.telecom[]
func buildOrganizationFromMFN(
	parsedHL7Data map[string]interface{},
	primaryKey string,
	active bool,
	masterFileID string,
	rules AssemblyRules,
) (map[string]interface{}, string) {
	org := map[string]interface{}{
		"resourceType": "Organization",
		"active":       active,
	}

	// id — prefer MFE.4 primary key; sanitise for FHIR
	orgID := "org-" + sanitizeID(primaryKey)
	if primaryKey == "" {
		orgID = "org-1"
	}
	org["id"] = orgID

	// ── Organization.type (from master file identifier) ──────────────────────
	typeCode := "bus"  // default FHIR v3 OrgType = 'other business'
	typeDisplay := "Non-Healthcare Business or Corporation"
	switch masterFileID {
	case "EMP":
		typeCode = "bus" // "emp" is not in FHIR organization-type ValueSet; "bus" = Non-Healthcare Business
		typeDisplay = "Non-Healthcare Business or Corporation"
	case "INS":
		typeCode = "ins"
		typeDisplay = "Insurance Company"
	case "PAY":
		typeCode = "pay"
		typeDisplay = "Payer"
	}
	org["type"] = []interface{}{
		map[string]interface{}{
			"coding": []interface{}{map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/organization-type",
				"code":    typeCode,
				"display": typeDisplay,
			}},
			"text": typeDisplay,
		},
	}

	// ── Try ZEM segment first ─────────────────────────────────────────────────
	zemList := ExtractSegmentGroup(parsedHL7Data, "ZEM")
	if len(zemList) > 0 {
		zem := zemList[0]
		populateOrgFromZEM(org, zem, rules)
	} else {
		// No ZEM — try other Z-segments or EB segments as fallback
		ebList := ExtractSegmentGroup(parsedHL7Data, "EB")
		if len(ebList) > 0 {
			// EB.3 = insurance plan name (site-specific position)
			if name := SegFieldValue(ebList[0], "EB.3"); name != "" {
				org["name"] = name
			}
		}
	}

	// Ensure name is always set
	if _, hasName := org["name"]; !hasName {
		if primaryKey != "" {
			org["name"] = "Organization " + primaryKey
		} else {
			org["name"] = "Unknown Organization"
		}
	}

	// narrative
	org["text"] = map[string]interface{}{
		"status": "generated",
		"div":    sharedNarrativeGenerator.Generate(org, nil, nil),
	}

	return org, "Organization"
}

// populateOrgFromZEM fills Organization fields from a ZEM Z-segment.
// ZEM is a site-specific employer segment — field positions are:
//
//	ZEM.1  ID (CWE)  — employer ID
//	ZEM.2  name      — employer name
//	ZEM.3  address (XAD)
//	ZEM.4  plan/coverage type
//	ZEM.5  phone
func populateOrgFromZEM(org map[string]interface{}, zem hl7.EnhancedSegment, rules AssemblyRules) {
	// ZEM.1 — employer identifier
	if id := SegFieldValue(zem, "ZEM.1"); id != "" {
		// ZEM.1 may be "code^description" — use the first component as value
		idVal := strings.SplitN(id, "^", 2)[0]
		if strings.TrimSpace(idVal) != "" {
			org["identifier"] = []interface{}{
				map[string]interface{}{
					"system": rules.FacilityNamespace + ":employer-id",
					"value":  strings.TrimSpace(idVal),
				},
			}
		}
	}

	// ZEM.2 — employer / organization name
	if name := SegFieldValue(zem, "ZEM.2"); name != "" {
		org["name"] = strings.TrimSpace(name)
	}

	// ZEM.3 — address (XAD format: street^other^city^state^zip^country)
	if addrRaw := SegFieldValue(zem, "ZEM.3"); addrRaw != "" {
		addr := BuildAddressFromXAD(addrRaw, nil)
		if len(addr) > 0 {
			org["address"] = []interface{}{addr}
		}
	}

	// ZEM.5 — phone number; Organization.telecom.use must not be "home" (org-3 constraint)
	if phone := SegFieldValue(zem, "ZEM.5"); phone != "" {
		cp := BuildContactPointFromXTN(phone, nil)
		if len(cp) > 0 {
			if use, ok := cp["use"].(string); ok && use == "home" {
				cp["use"] = "work"
			} else if _, hasUse := cp["use"]; !hasUse {
				cp["use"] = "work"
			}
			org["telecom"] = []interface{}{cp}
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Practitioner builder  (STF / PRA master files)
// ──────────────────────────────────────────────────────────────────────────────

// buildPractitionerFromSTF builds a FHIR R4 Practitioner from STF/PRA segments.
//
// STF fields used:
//
//	STF.1  primary key value  → identifier
//	STF.2  staff ID codes     → identifier[]
//	STF.3  staff name (XPN)   → name[]
//	STF.4  staff type         → qualification[].code (informational)
//	STF.5  administrative sex → gender
//	STF.6  date of birth      → birthDate
//	STF.7  active/inactive    → active (overrides MFE action)
//	STF.10 phone – business   → telecom[]
//	STF.11 office/division    → address
//	STF.12 institution        → qualification[].issuer
func buildPractitionerFromSTF(
	parsedHL7Data map[string]interface{},
	primaryKey string,
	active bool,
	rules AssemblyRules,
) (map[string]interface{}, string) {
	stfList := ExtractSegmentGroup(parsedHL7Data, "STF")
	if len(stfList) == 0 {
		return nil, ""
	}
	stf := stfList[0]

	practID := "pract-" + sanitizeID(primaryKey)
	if primaryKey == "" {
		if pk := SegFieldValue(stf, "STF.1"); pk != "" {
			practID = "pract-" + sanitizeID(pk)
		} else {
			practID = "pract-1"
		}
	}

	pract := map[string]interface{}{
		"resourceType": "Practitioner",
		"id":           practID,
		"active":       active,
	}

	// STF.7 explicit active flag overrides MFE action when present
	if stf7 := strings.ToUpper(strings.TrimSpace(SegFieldValue(stf, "STF.7"))); stf7 != "" {
		pract["active"] = stf7 == "A" // A=Active, I=Inactive
	}

	// identifier — STF.1 primary key
	if pk := SegFieldValue(stf, "STF.1"); pk != "" {
		pract["identifier"] = []interface{}{
			map[string]interface{}{
				"system": rules.FacilityNamespace + ":staff-id",
				"value":  pk,
			},
		}
	}

	// name — STF.3 (XPN)
	if nameRaw := SegFieldValue(stf, "STF.3"); nameRaw != "" {
		hn := BuildHumanNameFromXPN(nameRaw, nil)
		if len(hn) > 0 {
			pract["name"] = []interface{}{hn}
		}
	}

	// gender — STF.5 (HL7 Table 0001)
	if sex := strings.ToUpper(strings.TrimSpace(SegFieldValue(stf, "STF.5"))); sex != "" {
		pract["gender"] = hl7GenderToFHIR(sex)
	}

	// birthDate — STF.6 (TS)
	if dob := SegFieldValue(stf, "STF.6"); dob != "" {
		pract["birthDate"] = ToISO(dob)
	}

	// telecom — STF.10 (XTN business phone)
	if phone := SegFieldValue(stf, "STF.10"); phone != "" {
		cp := BuildContactPointFromXTN(phone, nil)
		if len(cp) > 0 {
			pract["telecom"] = []interface{}{cp}
		}
	}

	// qualification — STF.4 staff type + PRA segment if present
	var quals []interface{}
	if staffType := SegFieldValue(stf, "STF.4"); staffType != "" {
		quals = append(quals, map[string]interface{}{
			"code": map[string]interface{}{"text": staffType},
		})
	}
	praList := ExtractSegmentGroup(parsedHL7Data, "PRA")
	if len(praList) > 0 {
		pra := praList[0]
		// PRA.1 = primary key / specialty
		if spec := SegFieldValue(pra, "PRA.3"); spec != "" {
			quals = append(quals, map[string]interface{}{
				"code": map[string]interface{}{"text": spec},
			})
		}
	}
	if len(quals) > 0 {
		pract["qualification"] = quals
	}

	// narrative
	pract["text"] = map[string]interface{}{
		"status": "generated",
		"div":    sharedNarrativeGenerator.Generate(pract, nil, nil),
	}

	return pract, "Practitioner"
}

// ──────────────────────────────────────────────────────────────────────────────
// Location builder  (LOC master files)
// ──────────────────────────────────────────────────────────────────────────────

// buildLocationFromLOC builds a FHIR R4 Location from LOC/LDP segments.
//
// LOC fields used:
//
//	LOC.1  location ID         → identifier
//	LOC.2  description         → name
//	LOC.3  location type (CWE) → type[]
//	LOC.4  organization (XON)  → managingOrganization (display only)
//	LOC.5  address (XAD)       → address
//	LOC.6  phone (XTN)         → telecom[]
//	LOC.7  license number      → identifier[]
func buildLocationFromLOC(
	parsedHL7Data map[string]interface{},
	primaryKey string,
	active bool,
	rules AssemblyRules,
) (map[string]interface{}, string) {
	locList := ExtractSegmentGroup(parsedHL7Data, "LOC")
	if len(locList) == 0 {
		return nil, ""
	}
	loc := locList[0]

	locID := "loc-" + sanitizeID(primaryKey)
	if primaryKey == "" {
		if pk := SegFieldValue(loc, "LOC.1"); pk != "" {
			locID = "loc-" + sanitizeID(pk)
		} else {
			locID = "loc-1"
		}
	}

	location := map[string]interface{}{
		"resourceType": "Location",
		"id":           locID,
	}

	// status — Location.status is "active" | "suspended" | "inactive"
	if active {
		location["status"] = "active"
	} else {
		location["status"] = "inactive"
	}

	// identifier — LOC.1
	if locKey := SegFieldValue(loc, "LOC.1"); locKey != "" {
		location["identifier"] = []interface{}{
			map[string]interface{}{
				"system": rules.FacilityNamespace + ":location-id",
				"value":  locKey,
			},
		}
	}

	// name — LOC.2
	if desc := SegFieldValue(loc, "LOC.2"); desc != "" {
		location["name"] = desc
	}

	// type — LOC.3 (CWE)
	if typeRaw := SegFieldValue(loc, "LOC.3"); typeRaw != "" {
		cc, _ := BuildCodeableConceptFromCE(typeRaw, rules.FacilityNamespace)
		location["type"] = []interface{}{cc}
	}

	// address — LOC.5 (XAD)
	if addrRaw := SegFieldValue(loc, "LOC.5"); addrRaw != "" {
		addr := BuildAddressFromXAD(addrRaw, nil)
		if len(addr) > 0 {
			location["address"] = addr
		}
	}

	// telecom — LOC.6 (XTN)
	if phone := SegFieldValue(loc, "LOC.6"); phone != "" {
		cp := BuildContactPointFromXTN(phone, nil)
		if len(cp) > 0 {
			location["telecom"] = []interface{}{cp}
		}
	}

	// managingOrganization — LOC.4 (XON — display only)
	if orgRaw := SegFieldValue(loc, "LOC.4"); orgRaw != "" {
		display := strings.SplitN(orgRaw, "^", 2)[0]
		if display != "" {
			location["managingOrganization"] = map[string]interface{}{"display": display}
		}
	}

	// narrative
	location["text"] = map[string]interface{}{
		"status": "generated",
		"div":    sharedNarrativeGenerator.Generate(location, nil, nil),
	}

	return location, "Location"
}

// ──────────────────────────────────────────────────────────────────────────────
// ChargeItemDefinition stub  (CDM master files)
// ──────────────────────────────────────────────────────────────────────────────

// buildChargeItemDefinitionFromCDM builds a minimal FHIR R4 ChargeItemDefinition
// from CDM (Charge Description Master) segments.
//
// CDM fields used:
//
//	CDM.1  charge code (CWE)   → code
//	CDM.2  charge description  → title
//	CDM.3  description (long)  → description
//	CDM.4  charge type         → applicability
//	CDM.7  active/inactive     → status
func buildChargeItemDefinitionFromCDM(
	parsedHL7Data map[string]interface{},
	primaryKey string,
	active bool,
	rules AssemblyRules,
) (map[string]interface{}, string) {
	cdmList := ExtractSegmentGroup(parsedHL7Data, "CDM")
	if len(cdmList) == 0 {
		return nil, ""
	}
	cdm := cdmList[0]

	cdmID := "cid-" + sanitizeID(primaryKey)
	if primaryKey == "" {
		if pk := SegFieldValue(cdm, "CDM.1"); pk != "" {
			cdmID = "cid-" + sanitizeID(strings.SplitN(pk, "^", 2)[0])
		} else {
			cdmID = "cid-1"
		}
	}

	status := "active"
	if !active {
		status = "retired"
	}
	// CDM.7 explicit status overrides
	if cdm7 := strings.ToUpper(strings.TrimSpace(SegFieldValue(cdm, "CDM.7"))); cdm7 == "I" {
		status = "retired"
	}

	cid := map[string]interface{}{
		"resourceType": "ChargeItemDefinition",
		"id":           cdmID,
		"status":       status,
		"url":          rules.FacilityNamespace + "/ChargeItemDefinition/" + cdmID,
	}

	// title — CDM.2
	if title := SegFieldValue(cdm, "CDM.2"); title != "" {
		cid["title"] = title
	}

	// description — CDM.3
	if desc := SegFieldValue(cdm, "CDM.3"); desc != "" {
		cid["description"] = desc
	}

	// code — CDM.1 (CWE)
	if codeRaw := SegFieldValue(cdm, "CDM.1"); codeRaw != "" {
		cc, _ := BuildCodeableConceptFromCE(codeRaw, rules.FacilityNamespace)
		cid["code"] = cc
	}

	// narrative
	cid["text"] = map[string]interface{}{
		"status": "generated",
		"div":    sharedNarrativeGenerator.Generate(cid, nil, nil),
	}

	return cid, "ChargeItemDefinition"
}

// ──────────────────────────────────────────────────────────────────────────────
// Shared helpers
// ──────────────────────────────────────────────────────────────────────────────

// hl7GenderToFHIR maps HL7 Table 0001 administrative sex codes to FHIR gender.
func hl7GenderToFHIR(code string) string {
	switch strings.ToUpper(code) {
	case "M":
		return "male"
	case "F":
		return "female"
	case "O":
		return "other"
	case "U", "N", "":
		return "unknown"
	default:
		return "unknown"
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Narrative generation — see sharedNarrativeGenerator in assembly.go.
// Organization, Practitioner, and Location narratives used to have their own
// hand-written builders here; all three now delegate to services/fhir_narrative
// (which has dedicated builders for all three); ChargeItemDefinition renders
// generically.
// ──────────────────────────────────────────────────────────────────────────────
