// services/hl7assembly/mdm_assembly.go
//
// Structural assembly for MDM (Medical Document Management) messages.
//
// MDM^T02/T04 messages carry a document in two segment groups:
//
//	TXA  — Transcription document header (metadata)
//	OBX* — Document content (value type TX / FT for plain text, ED for inline
//	        binary, RP for external reference)
//
// The correct FHIR R4 mapping is:
//
//	TXA  → DocumentReference  (metadata + mandatory status)
//	OBX  → DocumentReference.content[0].attachment (document body)
//
// Producing standalone Observation resources from MDM OBX segments is incorrect;
// AssembleMDMDocument removes any such resources and replaces them with a proper
// DocumentReference.
//
// Mandatory FHIR R4 DocumentReference fields satisfied here:
//   status  (1..1)  from TXA.19 (availability status)  — default "current"
//   content (1..1)  — at least one attachment with contentType
package hl7assembly

import (
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	"ezhealthkonnect/hl7"
)

// ──────────────────────────────────────────────────────────────────────────────
// TXA status mappers
// ──────────────────────────────────────────────────────────────────────────────

// TXADocStatus maps TXA.17 (document completion status, HL7 Table 0271) to the
// FHIR R4 DocumentReference.docStatus value set.
func TXADocStatus(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "AU": // Authenticated
		return "final"
	case "LA": // Legally authenticated
		return "final"
	case "OC": // Obsoleted
		return "entered-in-error"
	default: // DI, DO, IP, IN, PA or empty → not-yet-final
		return "preliminary"
	}
}

// TXAAvailabilityStatus maps TXA.19 (document availability status, HL7 Table 0273)
// to the FHIR R4 DocumentReference.status value set.
func TXAAvailabilityStatus(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "UN": // Unavailable
		return "superseded"
	case "OB": // Obsoleted
		return "entered-in-error"
	default: // AV (available) or empty → in service
		return "current"
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Main MDM assembly entry point
// ──────────────────────────────────────────────────────────────────────────────

// AssembleMDMDocument performs MDM^T02/T04 structural assembly:
//
//  1. Skips silently when no TXA segment is present (not an MDM message).
//  2. Derives the facility namespace from MSH.4 (Tier-3 coding fallback).
//  3. Builds a DocumentReference from TXA fields.
//  4. Collects all OBX TX/FT text values, concatenates them, and stores the
//     result as DocumentReference.content[0].attachment.data (base64).
//  5. Removes Observation resources that the mapping engine incorrectly produced
//     from MDM OBX segments.
//  6. Wires MessageHeader.focus to the new DocumentReference.
//
// Returns the updated resource list and advisory warnings.
func AssembleMDMDocument(
	parsedHL7Data map[string]interface{},
	resources []map[string]interface{},
	opts ...AssemblyRules,
) ([]map[string]interface{}, []string) {
	var rules AssemblyRules
	if len(opts) > 0 {
		rules = opts[0]
	}

	var warnings []string

	// ── Guard: only proceed when a TXA segment is present ────────────────────
	txaList := ExtractSegmentGroup(parsedHL7Data, "TXA")
	if len(txaList) == 0 {
		return resources, warnings
	}
	txa := txaList[0]

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

	// ── Collect OBX document content ─────────────────────────────────────────
	// In MDM, OBX carries the document body.  TX / FT / ST (text) values are
	// concatenated in set-ID order.  The first ED or RP value is used when no
	// text content is present.
	obxList := ExtractSegmentGroup(parsedHL7Data, "OBX")
	var textLines []string
	var edAtt map[string]interface{}
	var rpAtt map[string]interface{}
	obxMIME := "text/plain"

	for _, obx := range obxList {
		vtype := strings.ToUpper(strings.TrimSpace(SegFieldValue(obx, "OBX.2")))
		value := SegFieldValue(obx, "OBX.5")
		switch vtype {
		case "TX", "FT", "ST":
			if value != "" {
				textLines = append(textLines, value)
			}
		case "HTML":
			if value != "" {
				textLines = append(textLines, value)
				obxMIME = "text/html"
			}
		case "ED":
			if edAtt == nil && value != "" {
				edAtt = BuildAttachmentFromED(value, ruleOn(rules.ValidateBase64))
			}
		case "RP":
			if rpAtt == nil && value != "" {
				rpAtt = BuildAttachmentFromRP(value)
			}
		}
	}

	// ── Build DocumentReference.id ────────────────────────────────────────────
	docRefID := txaBuildDocRefID(txa)

	// ── Build DocumentReference ───────────────────────────────────────────────
	docRef := map[string]interface{}{
		"resourceType": "DocumentReference",
		"id":           docRefID,
	}

	// status (1..1) from TXA.19
	docRef["status"] = TXAAvailabilityStatus(SegFieldValue(txa, "TXA.19"))

	// docStatus from TXA.17 (optional but meaningful)
	if cs := SegFieldValue(txa, "TXA.17"); cs != "" {
		docRef["docStatus"] = TXADocStatus(cs)
	}

	// type from TXA.2 (document type code) — CodeableConcept
	if docType := SegFieldValue(txa, "TXA.2"); docType != "" {
		cc, tier3Used := BuildCodeableConceptFromCE(docType, rules.FacilityNamespace)
		docRef["type"] = cc
		if tier3Used {
			warnings = append(warnings,
				"DocumentReference.type: TXA.2 has no standard coding system — facility namespace applied ("+rules.FacilityNamespace+")")
		}
	}

	// date from TXA.4 (activity date); fallback TXA.6 (origination date)
	if actDate := SegFieldValue(txa, "TXA.4"); actDate != "" {
		docRef["date"] = ToISO(actDate)
	} else if origDate := SegFieldValue(txa, "TXA.6"); origDate != "" {
		docRef["date"] = ToISO(origDate)
	}

	// subject from Patient
	if patientRef != "" {
		docRef["subject"] = map[string]interface{}{"reference": patientRef}
	}

	// context.encounter — wire Encounter so it is reachable from MessageHeader
	// through the DocumentReference link chain (validator requires all bundle
	// entries to be reachable from MessageHeader forward/backward).
	for _, r := range resources {
		if r["resourceType"] == "Encounter" {
			if encID, ok := r["id"].(string); ok && encID != "" {
				docRef["context"] = map[string]interface{}{
					"encounter": []interface{}{
						map[string]interface{}{"reference": "Encounter/" + encID},
					},
				}
			}
			break
		}
	}

	// author from TXA.9 (originator)
	if orig := SegFieldValue(txa, "TXA.9"); orig != "" {
		docRef["author"] = []interface{}{xcnToRef(orig)}
	}

	// authenticator from TXA.22 (authentication person)
	if auth := SegFieldValue(txa, "TXA.22"); auth != "" {
		docRef["authenticator"] = xcnToRef(auth)
	}

	// masterIdentifier from TXA.12 (unique document number).
	// urn:ietf:rfc:3986 would require the value to be a full URI; for local
	// document numbers use the facility namespace as the system instead.
	if uid := SegFieldValue(txa, "TXA.12"); uid != "" {
		mid := map[string]interface{}{"value": uid}
		if rules.FacilityNamespace != "" && rules.FacilityNamespace != "urn:facility:unknown" {
			mid["system"] = rules.FacilityNamespace
		}
		docRef["masterIdentifier"] = mid
	}

	// ── Build content[0].attachment ───────────────────────────────────────────
	var att map[string]interface{}
	switch {
	case len(textLines) > 0:
		docText := strings.Join(textLines, "\n")
		att = map[string]interface{}{
			"contentType": obxMIME,
			"data":        base64.StdEncoding.EncodeToString([]byte(docText)),
		}
		// Derive a human-readable title
		if pres := SegFieldValue(txa, "TXA.3"); pres != "" {
			att["title"] = strings.ToUpper(pres) + " document"
		} else if docType := SegFieldValue(txa, "TXA.2"); docType != "" {
			att["title"] = strings.Split(docType, "^")[0] + " document"
		}

	case edAtt != nil:
		att = edAtt

	case rpAtt != nil:
		att = rpAtt

	default:
		// Mandatory content placeholder so content[0].attachment is never absent
		att = map[string]interface{}{
			"contentType": "text/plain",
			"title":       "(no document content in message)",
		}
		warnings = append(warnings,
			"DocumentReference: no OBX document content found; placeholder attachment created")
	}

	docRef["content"] = []interface{}{
		map[string]interface{}{"attachment": att},
	}

	// ── Narrative ─────────────────────────────────────────────────────────────
	docRef["text"] = map[string]interface{}{
		"status": "generated",
		"div":    buildDocRefNarrative(docRef),
	}

	// ── Rebuild resource list ─────────────────────────────────────────────────
	// Drop:   Observation resources (incorrect for MDM — OBX is document content)
	// Drop:   DocumentReference resources produced by the template mapper — their
	//         field values are wrong (raw TXA composites placed into FHIR paths
	//         without the correct status/content transforms). The assembled docRef
	//         built above is the authoritative replacement.
	// Update: MessageHeader.focus → DocumentReference (using ResourceType/id form
	//         so the bundle builder's rewriteReferences() can resolve it to the
	//         correct urn:uuid: fullUrl that gets assigned at bundle assembly time)
	// Add:    DocumentReference
	var result []map[string]interface{}

	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		switch rt {
		case "Observation":
			// Drop — MDM OBX carries document content, not clinical observations
		case "DocumentReference":
			// Drop — template mapper produced incorrect raw-field values; the
			// assembled docRef below is the correct replacement
		case "MessageHeader":
			r["focus"] = []interface{}{
				map[string]interface{}{"reference": "DocumentReference/" + docRefID},
			}
			result = append(result, r)
		default:
			result = append(result, r)
		}
	}
	result = append(result, docRef)

	log.Printf("📄 hl7.assemble_mdm: built DocumentReference/%s (status=%s, docStatus=%v, %d text line(s))",
		docRefID, docRef["status"], docRef["docStatus"], len(textLines))

	return result, warnings
}

// ──────────────────────────────────────────────────────────────────────────────
// Private helpers
// ──────────────────────────────────────────────────────────────────────────────

// txaBuildDocRefID returns a deterministic FHIR resource ID for the DocumentReference.
// Prefers TXA.12 (unique document number), then TXA.16 (file name), then TXA.1 (set-ID).
func txaBuildDocRefID(txa hl7.EnhancedSegment) string {
	if uid := SegFieldValue(txa, "TXA.12"); uid != "" {
		return sanitizeID("docref-" + uid)
	}
	if fn := SegFieldValue(txa, "TXA.16"); fn != "" {
		return sanitizeID("docref-" + fn)
	}
	if setID := SegFieldValue(txa, "TXA.1"); setID != "" {
		return "docref-" + setID
	}
	return "docref-1"
}

// sanitizeID strips characters that are not allowed in FHIR resource IDs
// (only [A-Za-z0-9\-\.] are permitted per R4 §2.2).
func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// xcnToRef converts an HL7 XCN (Extended Composite ID and Name for Persons)
// field to a display-only FHIR Reference.
//
// XCN format: IDNumber ^ FamilyName ^ GivenName ^ MiddleName ^ Suffix ^
//
//	Prefix ^ DegreeCode ^ SourceTable ^ AssigningAuthority ^ ...
//
// The Practitioner resource is not included in the bundle, so only a human-readable
// display string is set — no reference field.  This avoids broken reference warnings
// from FHIR validators while still conveying the author/authenticator identity.
func xcnToRef(xcn string) map[string]interface{} {
	parts := strings.Split(xcn, "^")

	id := ""
	if len(parts) > 0 {
		id = strings.TrimSpace(parts[0])
	}
	family := ""
	if len(parts) > 1 {
		family = strings.TrimSpace(parts[1])
	}
	given := ""
	if len(parts) > 2 {
		given = strings.TrimSpace(parts[2])
	}
	prefix := ""
	if len(parts) > 5 {
		prefix = strings.TrimSpace(parts[5])
	}

	// Build a human-readable display string: "Dr Given Family"
	var nameParts []string
	if prefix != "" {
		nameParts = append(nameParts, prefix)
	}
	if given != "" {
		nameParts = append(nameParts, given)
	}
	if family != "" {
		nameParts = append(nameParts, family)
	}

	display := strings.TrimSpace(strings.Join(nameParts, " "))
	if display == "" && id != "" {
		display = id // fall back to the raw ID as a label
	}

	ref := map[string]interface{}{}
	if display != "" {
		ref["display"] = display
	}
	return ref
}

// buildDocRefNarrative generates a minimal FHIR-compliant XHTML narrative for a
// DocumentReference resource.
func buildDocRefNarrative(dr map[string]interface{}) string {
	var b strings.Builder
	b.WriteString(`<div xmlns="http://www.w3.org/1999/xhtml">`)
	b.WriteString(`<table class="grid" style="border-collapse:collapse;width:100%;">`)
	b.WriteString(`<thead><tr style="background:#f0f0f0;">`)
	b.WriteString(`<th colspan="2" style="padding:8px;text-align:left;">DocumentReference</th>`)
	b.WriteString(`</tr></thead><tbody>`)

	row := func(label, value string) {
		if value == "" {
			return
		}
		b.WriteString(`<tr><td style="padding:4px 8px;border:1px solid #ddd;font-weight:bold;">`)
		b.WriteString(escapeXML(label))
		b.WriteString(`</td><td style="padding:4px 8px;border:1px solid #ddd;">`)
		b.WriteString(escapeXML(value))
		b.WriteString(`</td></tr>`)
	}

	if id, ok := dr["id"].(string); ok {
		row("ID", id)
	}
	if status, ok := dr["status"].(string); ok {
		row("Status", status)
	}
	if ds, ok := dr["docStatus"].(string); ok {
		row("Doc Status", ds)
	}
	if t, ok := dr["type"].(map[string]interface{}); ok {
		if txt, ok2 := t["text"].(string); ok2 {
			row("Type", txt)
		} else if codings, ok2 := t["coding"].([]interface{}); ok2 && len(codings) > 0 {
			if c, ok3 := codings[0].(map[string]interface{}); ok3 {
				row("Type", fmt.Sprintf("%v", c["code"]))
			}
		}
	}
	if d, ok := dr["date"].(string); ok {
		row("Date", d)
	}
	if subj, ok := dr["subject"].(map[string]interface{}); ok {
		if ref, ok2 := subj["reference"].(string); ok2 {
			row("Subject", ref)
		}
	}

	b.WriteString(`</tbody></table></div>`)
	return b.String()
}
