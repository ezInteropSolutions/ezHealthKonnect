// services/hl7assembly/assembly.go
//
// Stateless HL7→FHIR structural assembly functions.
// Lives in its own sub-package so it can be imported by both:
//   • services (transform service — direct API/wizard path)
//   • services/executors/transform (pipeline step path)
// without creating an import cycle.
package hl7assembly

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"ezhealthkonnect/hl7"
)

// ──────────────────────────────────────────────────────────────────────────────
// Private helpers  (pure functions — no external state)
// ──────────────────────────────────────────────────────────────────────────────
//
// Data-type builder functions (NormalizeCodingSystem, BuildCodeableConceptFromCE,
// BuildIdentifierFromCX, BuildHumanNameFromXPN, etc.) are defined in datatypes.go
// in this same package.

// obxGroup is one logical observation — either a single OBX or multiple
// TX/FT continuation segments sharing the same OBX.3 identifier.
type obxGroup struct {
	segments      []hl7.EnhancedSegment
	firstOBXIndex int // index into the original obxList (for NTE mapping)
}

// groupOBXSegments partitions a flat OBX list into logical groups.
// When collapseText is true, consecutive TX/FT segments with the same OBX.3
// are merged into one group (the HL7 "continuation segment" pattern).
// Every other OBX becomes its own single-segment group.
func groupOBXSegments(obxList []hl7.EnhancedSegment, collapseText bool) []obxGroup {
	var groups []obxGroup
	i := 0
	for i < len(obxList) {
		seg := obxList[i]
		vtype := strings.ToUpper(strings.TrimSpace(SegFieldValue(seg, "OBX.2")))
		code3 := SegFieldValue(seg, "OBX.3")

		if collapseText && (vtype == "TX" || vtype == "FT") && code3 != "" {
			// Look ahead: collect consecutive OBX with same OBX.3 and TX/FT type
			grp := obxGroup{firstOBXIndex: i, segments: []hl7.EnhancedSegment{seg}}
			j := i + 1
			for j < len(obxList) {
				next := obxList[j]
				nextType := strings.ToUpper(strings.TrimSpace(SegFieldValue(next, "OBX.2")))
				nextCode := SegFieldValue(next, "OBX.3")
				if (nextType == "TX" || nextType == "FT") && nextCode == code3 {
					grp.segments = append(grp.segments, next)
					j++
				} else {
					break
				}
			}
			groups = append(groups, grp)
			i = j
		} else {
			groups = append(groups, obxGroup{firstOBXIndex: i, segments: []hl7.EnhancedSegment{seg}})
			i++
		}
	}
	return groups
}

// buildMergedTextObservation combines multiple TX/FT continuation OBX segments
// (same OBX.3) into a single FHIR Observation.
// The first segment provides all metadata (status, date, subject, code).
// All OBX.5 values are joined with "\n" as valueString, preserving report layout.
func buildMergedTextObservation(
	segs []hl7.EnhancedSegment,
	patientRef string,
	rules AssemblyRules,
) (map[string]interface{}, string, bool) {
	// Build from the first segment — gets all metadata + code
	obs, obsID, tier3Used := BuildObservationFromOBX(segs[0], patientRef, rules)

	// Collect text lines from ALL segments (segment 0's value is already set
	// on obs["valueString"] — overwrite with the full merged text)
	var lines []string
	for _, seg := range segs {
		lines = append(lines, SegFieldValue(seg, "OBX.5"))
	}

	// Trim leading and trailing empty lines; preserve internal blank lines (report formatting)
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	fullText := strings.Join(lines, "\n")

	if fullText != "" {
		obs["valueString"] = fullText
	} else {
		delete(obs, "valueString")
	}

	// Regenerate narrative with the merged text
	if ruleOn(rules.ObsNarrative) {
		obs["text"] = map[string]interface{}{
			"status": "generated",
			"div":    buildObservationNarrative(obs),
		}
	}

	return obs, obsID, tier3Used
}

// obxValueTypeLabel returns a human-readable label for an OBX.2 value type code.
// Used as the fallback CodeableConcept.text when OBX.3 (observation identifier) is absent.
func obxValueTypeLabel(obx2 string) string {
	switch obx2 {
	case "TX", "FT":
		return "Text observation"
	case "NM":
		return "Numeric observation"
	case "SN":
		return "Structured numeric observation"
	case "CE", "CWE", "CNE":
		return "Coded observation"
	case "DT":
		return "Date observation"
	case "TM", "TS":
		return "Time observation"
	case "ST":
		return "String observation"
	case "ED":
		return "Encapsulated data observation"
	case "RP":
		return "Reference pointer observation"
	default:
		return "Observation"
	}
}

// BuildAttachmentFromED, BuildAttachmentFromRP → see datatypes.go

// buildQuantityFromSN converts an HL7 SN (Structured Numeric) field to a FHIR Quantity.
// SN wire format (HL7 v2 Table 0298 / 2.A.63):
//
//	Comparator ^ Num1 ^ Separator ^ Num2
//
// Comparator: "<", ">", "<=", ">=", "" (equal)
// Separator: "-" (range), "+" (plus/minus), ":" (ratio) — when Num2 present, use valueRange
func buildQuantityFromSN(sn, unitRaw string, includeUnit bool) map[string]interface{} {
	parts := strings.Split(sn, "^")
	qty := map[string]interface{}{}

	comparator := ""
	num1Str := sn
	sep := ""
	num2Str := ""

	if len(parts) >= 1 {
		comparator = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 {
		num1Str = strings.TrimSpace(parts[1])
	}
	if len(parts) >= 3 {
		sep = strings.TrimSpace(parts[2])
	}
	if len(parts) >= 4 {
		num2Str = strings.TrimSpace(parts[3])
	}

	if fv, err := strconv.ParseFloat(num1Str, 64); err == nil {
		qty["value"] = fv
	} else {
		qty["value"] = num1Str
	}
	if comparator == "<" || comparator == ">" || comparator == "<=" || comparator == ">=" {
		qty["comparator"] = comparator
	}
	if includeUnit && unitRaw != "" {
		ucum := normalizeUCUMUnit(unitRaw)
		ucumSys, _ := NormalizeCodingSystem("UCUM")
		qty["unit"] = ucum
		qty["system"] = ucumSys
		qty["code"] = ucum
	}
	// Note: when sep=="-" and num2Str is present this is really a range.
	// Returning a single Quantity is lossy but keeps the return type uniform.
	// Callers that need the full range can inspect OBX.7 (reference range) instead.
	_ = sep
	_ = num2Str
	return qty
}

// normalizeTMToFHIRTime converts HL7 TM (time) to FHIR time format HH:MM:SS.
// HL7 TM format: HHMMSS[.SSSS][+/-ZZZZ]
func normalizeTMToFHIRTime(tm string) string {
	// Strip timezone suffix
	t := strings.TrimSpace(tm)
	for _, sep := range []string{"+", "-"} {
		if idx := strings.LastIndex(t, sep); idx > 4 {
			t = t[:idx]
		}
	}
	switch {
	case len(t) >= 6:
		return fmt.Sprintf("%s:%s:%s", t[0:2], t[2:4], t[4:6])
	case len(t) >= 4:
		return fmt.Sprintf("%s:%s:00", t[0:2], t[2:4])
	case len(t) >= 2:
		return fmt.Sprintf("%s:00:00", t[0:2])
	}
	return tm
}

// normalizeUCUMUnit maps common HL7 unit strings to valid UCUM codes.
// HL7 senders often use plain English descriptions that are not valid UCUM;
// this table covers the most frequent offenders found in lab ORU messages.
func normalizeUCUMUnit(unit string) string {
	trimmed := strings.TrimSpace(unit)
	upper := strings.ToUpper(trimmed)
	ucumMap := map[string]string{
		// Time
		"SECOND(S)": "s", "SECONDS": "s", "SEC": "s", "SECS": "s",
		"MINUTE(S)": "min", "MINUTES": "min", "MIN": "min",
		"HOUR(S)": "h", "HOURS": "h", "HR": "h", "HRS": "h",
		// Mass
		"GRAM(S)": "g", "GRAMS": "g",
		"MILLIGRAM(S)": "mg", "MILLIGRAMS": "mg",
		"MICROGRAM(S)": "ug", "MICROGRAMS": "ug",
		// Volume
		"MILLILITER(S)": "mL", "MILLILITERS": "mL", "ML": "mL",
		"LITER(S)": "L", "LITERS": "L",
		"DECILITER(S)": "dL", "DECILITERS": "dL", "DL": "dL",
		// Concentration
		"MG/DL": "mg/dL", "G/DL": "g/dL", "G/L": "g/L",
		"MEQ/L": "meq/L", "MMOL/L": "mmol/L", "UMOL/L": "umol/L",
		// Count / cell counts
		"CELLS/MCL": "cells/uL", "CELLS/UL": "cells/uL",
		"THOUSAND/MCL": "10*3/uL", "10^3/MCL": "10*3/uL",
		"MILLION/MCL": "10*6/uL", "10^6/MCL": "10*6/uL",
		// Other common
		"PERCENT": "%", "PERCENT(%)": "%",
		"INTERNATIONAL UNITS/L": "IU/L", "IU/L": "IU/L",
		"NANOGRAM(S)/ML": "ng/mL", "NG/ML": "ng/mL",
		"PICOGRAM(S)": "pg", "PICOGRAMS": "pg",
		"FEMTOLITER(S)": "fL", "FEMTOLITERS": "fL", "FL": "fL",
	}
	if mapped, ok := ucumMap[upper]; ok {
		return mapped
	}
	return trimmed // pass through as-is; UCUM validator may still accept it
}

// FacilityNamespaceURI, ToISO → see datatypes.go

// ──────────────────────────────────────────────────────────────────────────────
// Segment helpers
// ──────────────────────────────────────────────────────────────────────────────

// ExtractSegmentGroup returns all instances of segmentName from parsedHL7Data.
// Handles both the typed Go path and the JSON-unmarshalled generic path.
func ExtractSegmentGroup(parsedHL7Data map[string]interface{}, segmentName string) []hl7.EnhancedSegment {
	rawGroups, exists := parsedHL7Data["segmentGroups"]
	if !exists {
		return nil
	}
	if typed, ok := rawGroups.(map[string][]hl7.EnhancedSegment); ok {
		return typed[segmentName]
	}
	b, err := json.Marshal(rawGroups)
	if err != nil {
		log.Printf("⚠️ hl7assembly.ExtractSegmentGroup: marshal failed: %v", err)
		return nil
	}
	var converted map[string][]hl7.EnhancedSegment
	if err = json.Unmarshal(b, &converted); err != nil {
		log.Printf("⚠️ hl7assembly.ExtractSegmentGroup: unmarshal failed: %v", err)
		return nil
	}
	return converted[segmentName]
}

// SegFieldValue returns the value of a named field (e.g. "OBX.3") from a segment.
func SegFieldValue(seg hl7.EnhancedSegment, key string) string {
	for _, f := range seg.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

// ──────────────────────────────────────────────────────────────────────────────
// Data-type converters (pure)
// ──────────────────────────────────────────────────────────────────────────────

// BuildCodeableConceptFromCE, CEToCodeableConcept → see datatypes.go
// OBRStatusToDRStatus, ORCStatusToDRStatus, OBXStatusToObsStatus → see datatypes.go

// AbnormalFlagToInterpretation maps OBX.8 to a FHIR interpretation CodeableConcept
func AbnormalFlagToInterpretation(flag string) map[string]interface{} {
	f := strings.ToUpper(strings.TrimSpace(flag))
	code, display := "U", "Unknown"
	switch f {
	case "H", "HH":
		code, display = "H", "High"
	case "L", "LL":
		code, display = "L", "Low"
	case "A":
		code, display = "A", "Abnormal"
	case "N":
		code, display = "N", "Normal"
	case "AA":
		code, display = "AA", "Critical abnormal"
	case "CR":
		code, display = "CR", "Critical low"
	case "IE":
		code, display = "IE", "Insufficient evidence"
	case "IND":
		code, display = "IND", "Indeterminate"
	}
	return map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation",
				"code":    code,
				"display": display,
			},
		},
		"text": display,
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Narrative generators (XHTML — dom-6 best-practice compliance)
// ──────────────────────────────────────────────────────────────────────────────

// buildObservationNarrative produces a minimal FHIR-compliant XHTML narrative
// for an Observation resource built from an OBX segment.
// The narrative is generated entirely from the resource map itself so it stays
// consistent with whatever fields were actually populated.
func buildObservationNarrative(obs map[string]interface{}) string {
	var b strings.Builder
	b.WriteString(`<div xmlns="http://www.w3.org/1999/xhtml"><table class="grid"><tbody>`)

	// Code / description
	codeLabel := ""
	if cc, ok := obs["code"].(map[string]interface{}); ok {
		if txt, ok := cc["text"].(string); ok && txt != "" {
			codeLabel = txt
		} else if codings, ok := cc["coding"].([]interface{}); ok && len(codings) > 0 {
			if first, ok := codings[0].(map[string]interface{}); ok {
				if c, ok := first["code"].(string); ok {
					codeLabel = c
					if d, ok := first["display"].(string); ok && d != "" {
						codeLabel = d + " (" + c + ")"
					}
				}
			}
		}
	}
	if codeLabel != "" {
		b.WriteString("<tr><td><b>Code</b></td><td>")
		b.WriteString(escapeXML(codeLabel))
		b.WriteString("</td></tr>")
	}

	// Value
	switch {
	case obs["valueQuantity"] != nil:
		if vq, ok := obs["valueQuantity"].(map[string]interface{}); ok {
			val := fmt.Sprintf("%v", vq["value"])
			if unit, ok := vq["unit"].(string); ok && unit != "" {
				val += " " + unit
			}
			b.WriteString("<tr><td><b>Value</b></td><td>")
			b.WriteString(escapeXML(val))
			b.WriteString("</td></tr>")
		}
	case obs["valueString"] != nil:
		b.WriteString("<tr><td><b>Value</b></td><td>")
		b.WriteString(escapeXML(fmt.Sprintf("%v", obs["valueString"])))
		b.WriteString("</td></tr>")
	case obs["valueCodeableConcept"] != nil:
		if vcc, ok := obs["valueCodeableConcept"].(map[string]interface{}); ok {
			label := ""
			if txt, ok := vcc["text"].(string); ok {
				label = txt
			}
			if label != "" {
				b.WriteString("<tr><td><b>Value</b></td><td>")
				b.WriteString(escapeXML(label))
				b.WriteString("</td></tr>")
			}
		}
	}

	// Reference range
	if rrs, ok := obs["referenceRange"].([]interface{}); ok && len(rrs) > 0 {
		if rr, ok := rrs[0].(map[string]interface{}); ok {
			if txt, ok := rr["text"].(string); ok && txt != "" {
				b.WriteString("<tr><td><b>Reference Range</b></td><td>")
				b.WriteString(escapeXML(txt))
				b.WriteString("</td></tr>")
			}
		}
	}

	// Interpretation
	if interps, ok := obs["interpretation"].([]interface{}); ok && len(interps) > 0 {
		if interp, ok := interps[0].(map[string]interface{}); ok {
			if txt, ok := interp["text"].(string); ok && txt != "" && txt != "Unknown" {
				b.WriteString("<tr><td><b>Flag</b></td><td>")
				b.WriteString(escapeXML(txt))
				b.WriteString("</td></tr>")
			}
		}
	}

	// Status and date
	if status, ok := obs["status"].(string); ok && status != "" {
		b.WriteString("<tr><td><b>Status</b></td><td>")
		b.WriteString(escapeXML(status))
		b.WriteString("</td></tr>")
	}
	if eff, ok := obs["effectiveDateTime"].(string); ok && eff != "" {
		b.WriteString("<tr><td><b>Date</b></td><td>")
		b.WriteString(escapeXML(eff))
		b.WriteString("</td></tr>")
	}

	b.WriteString("</tbody></table></div>")
	return b.String()
}

// buildDRNarrative produces a FHIR-compliant XHTML narrative for a DiagnosticReport.
func buildDRNarrative(dr map[string]interface{}, resultCount int) string {
	var b strings.Builder
	b.WriteString(`<div xmlns="http://www.w3.org/1999/xhtml"><table class="grid"><tbody>`)

	// Code
	if cc, ok := dr["code"].(map[string]interface{}); ok {
		label := ""
		if txt, ok := cc["text"].(string); ok && txt != "" {
			label = txt
		} else if codings, ok := cc["coding"].([]interface{}); ok && len(codings) > 0 {
			if first, ok := codings[0].(map[string]interface{}); ok {
				if d, ok := first["display"].(string); ok && d != "" {
					label = d
				} else if c, ok := first["code"].(string); ok {
					label = c
				}
			}
		}
		if label != "" {
			b.WriteString("<tr><td><b>Report</b></td><td>")
			b.WriteString(escapeXML(label))
			b.WriteString("</td></tr>")
		}
	}

	if status, ok := dr["status"].(string); ok && status != "" {
		b.WriteString("<tr><td><b>Status</b></td><td>")
		b.WriteString(escapeXML(status))
		b.WriteString("</td></tr>")
	}
	if eff, ok := dr["effectiveDateTime"].(string); ok && eff != "" {
		b.WriteString("<tr><td><b>Date</b></td><td>")
		b.WriteString(escapeXML(eff))
		b.WriteString("</td></tr>")
	}
	if resultCount > 0 {
		b.WriteString(fmt.Sprintf("<tr><td><b>Results</b></td><td>%d observation(s)</td></tr>", resultCount))
	}

	b.WriteString("</tbody></table></div>")
	return b.String()
}

// escapeXML escapes the five predefined XML entities.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// ──────────────────────────────────────────────────────────────────────────────
// Observation builder
// ──────────────────────────────────────────────────────────────────────────────

// BuildObservationFromOBX constructs a FHIR Observation from a single OBX segment.
//
// Returns the resource map, its id string, and a bool that is true when the
// Tier-3 facility-namespace fallback was applied to at least one coding in this
// observation (used by the caller to emit a consolidated advisory warning).
func BuildObservationFromOBX(seg hl7.EnhancedSegment, patientRef string, rules AssemblyRules) (map[string]interface{}, string, bool) {
	setNum := SegFieldValue(seg, "OBX.1")
	if setNum == "" {
		setNum = fmt.Sprintf("%d", seg.SegmentIndex+1)
	}
	obsID := "obs-" + setNum

	valueType := strings.ToUpper(strings.TrimSpace(SegFieldValue(seg, "OBX.2")))
	codeRaw := SegFieldValue(seg, "OBX.3")
	valueRaw := SegFieldValue(seg, "OBX.5")
	unitRaw := SegFieldValue(seg, "OBX.6")
	refRange := SegFieldValue(seg, "OBX.7")
	abnFlag := SegFieldValue(seg, "OBX.8")
	statusRaw := SegFieldValue(seg, "OBX.11")
	obsDate := SegFieldValue(seg, "OBX.14")
	performerRaw := SegFieldValue(seg, "OBX.16")

	facilityNS := rules.FacilityNamespace
	tier3Used := false

	obs := map[string]interface{}{
		"resourceType": "Observation",
		"id":           obsID,
	}

	if ruleOn(rules.ObsStatus) {
		obs["status"] = OBXStatusToObsStatus(statusRaw)
	}

	if ruleOn(rules.ObsCategory) {
		obs["category"] = []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  "http://terminology.hl7.org/CodeSystem/observation-category",
						"code":    "laboratory",
						"display": "Laboratory",
					},
				},
			},
		}
	}

	if ruleOn(rules.ObsCode) {
		if codeRaw != "" {
			cc, used := BuildCodeableConceptFromCE(codeRaw, facilityNS)
			obs["code"] = cc
			if used {
				tier3Used = true
			}
		} else {
			// OBX.3 is empty — Observation.code is required (1..1) in FHIR.
			// Use a text-only CodeableConcept (valid FHIR — coding[] is 0..*).
			// Derive display text from OBX.2 value type for human readability.
			valueType := strings.ToUpper(strings.TrimSpace(SegFieldValue(seg, "OBX.2")))
			obs["code"] = map[string]interface{}{
				"text": obxValueTypeLabel(valueType),
			}
		}
	}
	if ruleOn(rules.ObsSubject) && patientRef != "" {
		obs["subject"] = map[string]interface{}{"reference": patientRef}
	}
	if ruleOn(rules.ObsEffective) && obsDate != "" {
		obs["effectiveDateTime"] = ToISO(obsDate)
	}

	// value[x] — dispatched by OBX.2 type per HL7 v2.x Table 0125.
	// Each OBX.2 code maps to exactly one FHIR Observation.value[x] choice type.
	if ruleOn(rules.ObsValueDispatch) && valueRaw != "" {
		switch valueType {
		// ── Numeric ────────────────────────────────────────────────────────────
		case "NM": // Numeric (decimal)
			qty := map[string]interface{}{}
			if fv, err := strconv.ParseFloat(strings.TrimSpace(valueRaw), 64); err == nil {
				qty["value"] = fv
			} else {
				qty["value"] = valueRaw
			}
			if ruleOn(rules.ObsValueUnit) && unitRaw != "" {
				ucumUnit := normalizeUCUMUnit(unitRaw)
				qty["unit"] = ucumUnit
				ucumSys, _ := NormalizeCodingSystem("UCUM")
				qty["system"] = ucumSys
				qty["code"] = ucumUnit
			}
			obs["valueQuantity"] = qty

		case "SN": // Structured Numeric — comparator&value1[&separator&value2]
			// SN format (HL7 v2 2.A.63): comparator^num1^separator^num2
			obs["valueQuantity"] = buildQuantityFromSN(valueRaw, unitRaw, ruleOn(rules.ObsValueUnit))

		// ── Coded ──────────────────────────────────────────────────────────────
		case "CE", "CWE", "CNE": // Coded Element / Coded with Exceptions / Coded No Exceptions
			cc, used := BuildCodeableConceptFromCE(valueRaw, facilityNS)
			obs["valueCodeableConcept"] = cc
			if used {
				tier3Used = true
			}

		case "IS", "ID": // Coded value (table-driven) — no external system URI
			obs["valueCodeableConcept"] = map[string]interface{}{
				"text": valueRaw,
			}

		// ── String / text ──────────────────────────────────────────────────────
		case "ST": // String
			obs["valueString"] = valueRaw

		case "TX": // Text data (multi-line)
			obs["valueString"] = valueRaw

		case "FT": // Formatted text
			obs["valueString"] = valueRaw

		// ── Date / Time ────────────────────────────────────────────────────────
		case "DT": // Date (YYYYMMDD)
			obs["valueDateTime"] = ToISO(valueRaw)

		case "TM": // Time (HHMMSS[.SSSS][+/-ZZZZ])
			obs["valueTime"] = normalizeTMToFHIRTime(valueRaw)

		case "TS": // Timestamp (date + time)
			obs["valueDateTime"] = ToISO(valueRaw)

		case "DTM": // Date/Time (v2.7+, replaces TS)
			obs["valueDateTime"] = ToISO(valueRaw)

		// ── Boolean ────────────────────────────────────────────────────────────
		case "BIT": // Bit
			obs["valueBoolean"] = strings.TrimSpace(valueRaw) == "1"

		// ── Attachment / Binary ────────────────────────────────────────────────
		case "ED": // Encapsulated Data — SourceApp^TypeOfData^DataSubtype^Encoding^Data
			obs["valueAttachment"] = BuildAttachmentFromED(valueRaw)

		case "RP": // Reference Pointer — Pointer^ApplicationID^TypeOfData^DataSubtype (HL7 v2 2.A.65)
			// RP is a URL/URI pointer to external data; the actual bytes live at the URL.
			// FHIR Observation has no valueUri choice type in R4 — map to valueAttachment.url
			// so the content-type is preserved and the pointer is unambiguously a URL, not
			// free-form text. This mirrors the ED→valueAttachment pattern but uses url not data.
			obs["valueAttachment"] = BuildAttachmentFromRP(valueRaw)

		// ── Quantity with comparator ───────────────────────────────────────────
		case "MO": // Money — value^currency
			moParts := strings.SplitN(valueRaw, "^", 2)
			qty := map[string]interface{}{}
			if fv, err := strconv.ParseFloat(strings.TrimSpace(moParts[0]), 64); err == nil {
				qty["value"] = fv
			}
			if len(moParts) >= 2 && moParts[1] != "" {
				qty["unit"] = strings.TrimSpace(moParts[1])
				qty["system"] = "urn:iso:std:iso:4217"
				qty["code"] = strings.TrimSpace(moParts[1])
			}
			obs["valueQuantity"] = qty

		// ── Ratio / Range ──────────────────────────────────────────────────────
		case "NA": // Numeric Array — "^"-separated values; use valueString as fallback
			obs["valueString"] = valueRaw

		// ── Default ────────────────────────────────────────────────────────────
		default:
			// Unknown OBX.2 type — preserve value as string (lossy but valid)
			obs["valueString"] = valueRaw
		}
	}

	// referenceRange — parse "low-high" into structured quantities
	if ruleOn(rules.ObsReferenceRange) && refRange != "" {
		rr := map[string]interface{}{"text": refRange}
		ucumUnit := ""
		if unitRaw != "" {
			ucumUnit = normalizeUCUMUnit(unitRaw)
		}
		parts := strings.SplitN(refRange, "-", 2)
		if len(parts) == 2 {
			if lo, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64); err == nil {
				e := map[string]interface{}{"value": lo}
				if ucumUnit != "" {
					e["unit"] = ucumUnit
				}
				rr["low"] = e
			}
			if hi, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
				e := map[string]interface{}{"value": hi}
				if ucumUnit != "" {
					e["unit"] = ucumUnit
				}
				rr["high"] = e
			}
		}
		obs["referenceRange"] = []interface{}{rr}
	}

	if ruleOn(rules.ObsInterpretation) && abnFlag != "" && strings.ToUpper(abnFlag) != "N" {
		obs["interpretation"] = []interface{}{AbnormalFlagToInterpretation(abnFlag)}
	}

	if performerRaw != "" {
		parts := strings.Split(performerRaw, "^")
		display := performerRaw
		if len(parts) >= 3 && parts[2] != "" {
			display = parts[2] + " " + parts[1]
		} else if len(parts) >= 2 && parts[1] != "" {
			display = parts[1]
		}
		obs["performer"] = []interface{}{map[string]interface{}{"display": display}}
	}

	// Narrative (dom-6 best-practice): generate after all fields are populated.
	if ruleOn(rules.ObsNarrative) {
		obs["text"] = map[string]interface{}{
			"status": "generated",
			"div":    buildObservationNarrative(obs),
		}
	}

	return obs, obsID, tier3Used
}

// ──────────────────────────────────────────────────────────────────────────────
// Assembly rules (per-interface user config)
// ──────────────────────────────────────────────────────────────────────────────

// AssemblyRules controls which individual OOB transforms run during assembly.
// All fields default to true when the key is absent (zero value = false means
// the user explicitly disabled it).
type AssemblyRules struct {
	// Observation rules (per OBX)
	ObsValueDispatch  *bool // OBX.2+OBX.5 → value[x] type dispatch
	ObsValueUnit      *bool // OBX.6       → valueQuantity.unit / system
	ObsCode           *bool // OBX.3       → code CodeableConcept
	ObsReferenceRange *bool // OBX.7       → referenceRange[] structured
	ObsInterpretation *bool // OBX.8       → interpretation[]
	ObsStatus         *bool // OBX.11      → status
	ObsCategory       *bool // (fixed)     → category[laboratory]
	ObsSubject        *bool // Patient.id  → subject.reference
	ObsEffective      *bool // OBX.14      → effectiveDateTime
	ObsNarrative      *bool // generate XHTML narrative (dom-6 best practice)

	// DiagnosticReport rules (once per message)
	DRResultLinks *bool // Observation refs → result[]
	DRSubject     *bool // Patient.id        → subject.reference
	DRCode        *bool // OBR.4             → code
	DRStatus      *bool // OBR.25            → status
	DREffective   *bool // OBR.7             → effectiveDateTime
	DRCategory    *bool // (fixed)           → category[LAB]
	DRNarrative   *bool // regenerate DR narrative after result[] links are wired

	// CollapseTextOBX controls whether consecutive OBX segments that share the
	// same OBX.3 identifier AND are all TX/FT (text/formatted text) value type
	// are merged into a single Observation with a concatenated valueString.
	// This is the standard HL7 "continuation segment" pattern used by radiology
	// and pathology systems to split long reports across multiple OBX lines.
	// Default: ON (nil).  Set to false to keep every OBX as its own Observation.
	CollapseTextOBX *bool

	// CollapseTextOBXServices lists OBR.24 diagnostic service codes for which
	// the text-OBX merge is applied.  When empty (default), merging applies to
	// every ORU message regardless of service.  When non-empty, only messages
	// whose OBR.24 matches one of these codes are merged; all others keep each
	// OBX as a separate Observation.
	// Common OBR.24 values: AP (Anatomic Pathology), RAD (Radiology),
	// SP (Surgical Pathology), NMR (Nuclear Magnetic Resonance), NM (Nuclear Med),
	// OTH (Other), LAB (Laboratory), MB (Microbiology), CH (Chemistry).
	CollapseTextOBXServices []string

	// Coding system rules
	// FacilityNamespace is the Tier-3 fallback system URI for local-only codes
	// (HL7 system = L / LOCAL / 99ZZZ) that have no standard alternate coding.
	// When empty, AssembleORUObservations auto-derives it from MSH.4 (Sending Facility).
	// Set to "-" to disable Tier-3 entirely (codes will have no system).
	FacilityNamespace string
}

// ruleOn returns true when the pointer is nil (absent = default on) or points to true.
func ruleOn(p *bool) bool { return p == nil || *p }

// AssemblyRulesFromConfig builds an AssemblyRules from the map stored in step.config.assemblyRules.
func AssemblyRulesFromConfig(raw map[string]interface{}) AssemblyRules {
	get := func(key string) *bool {
		v, ok := raw[key]
		if !ok {
			return nil // absent → default on
		}
		b, ok := v.(bool)
		if !ok {
			return nil
		}
		return &b
	}
	r := AssemblyRules{
		ObsValueDispatch:  get("obs_value_dispatch"),
		ObsValueUnit:      get("obs_value_unit"),
		ObsCode:           get("obs_code"),
		ObsReferenceRange: get("obs_reference_range"),
		ObsInterpretation: get("obs_interpretation"),
		ObsStatus:         get("obs_status"),
		ObsCategory:       get("obs_category"),
		ObsSubject:        get("obs_subject"),
		ObsEffective:      get("obs_effective"),
		ObsNarrative:      get("obs_narrative"),
		DRResultLinks:     get("dr_result_links"),
		DRSubject:         get("dr_subject"),
		DRCode:            get("dr_code"),
		DRStatus:          get("dr_status"),
		DREffective:       get("dr_effective"),
		DRCategory:        get("dr_category"),
		DRNarrative:       get("dr_narrative"),
		CollapseTextOBX:   get("collapse_text_obx"),
	}
	// CollapseTextOBXServices: []string or []interface{}
	if svcs, ok := raw["collapse_text_obx_services"]; ok {
		switch v := svcs.(type) {
		case []string:
			r.CollapseTextOBXServices = v
		case []interface{}:
			for _, s := range v {
				if str, ok2 := s.(string); ok2 && str != "" {
					r.CollapseTextOBXServices = append(r.CollapseTextOBXServices, str)
				}
			}
		}
	}
	if ns, ok := raw["facility_namespace"].(string); ok {
		r.FacilityNamespace = ns
	}
	return r
}

// ──────────────────────────────────────────────────────────────────────────────
// Main assembly entry point
// ──────────────────────────────────────────────────────────────────────────────

// AssembleORUObservations performs ORU^R01 structural assembly:
//  1. Derives facility namespace from MSH.4 for Tier-3 coding fallback
//  2. Reads all OBX segments from parsedHL7Data.segmentGroups
//  3. Removes placeholder Observations the mapping engine produced
//  4. Builds one FHIR Observation per OBX (with narrative)
//  5. Links them to DiagnosticReport.result[]
//  6. Populates DR.code (OBR.4), DR.category, DR.effectiveDateTime (OBR.7)
//  7. Strips empty DR identifiers
//  8. Regenerates DR narrative to reflect the wired result[] links
//
// Returns the updated resource list and advisory warnings.
func AssembleORUObservations(
	parsedHL7Data map[string]interface{},
	resources []map[string]interface{},
	opts ...AssemblyRules,
) ([]map[string]interface{}, []string) {
	var rules AssemblyRules
	if len(opts) > 0 {
		rules = opts[0]
	}

	var warnings []string

	// ── Derive facility namespace for Tier-3 coding fallback ──────────────────
	// MSH.4 (Sending Facility) is the canonical identifier for the code space.
	// We construct it here once so every OBX can apply it consistently.
	if rules.FacilityNamespace == "" {
		mshList := ExtractSegmentGroup(parsedHL7Data, "MSH")
		if len(mshList) > 0 {
			sendingFacility := SegFieldValue(mshList[0], "MSH.4")
			if sendingFacility != "" {
				rules.FacilityNamespace = FacilityNamespaceURI(sendingFacility)
			}
		}
		if rules.FacilityNamespace == "" {
			rules.FacilityNamespace = "urn:facility:unknown"
		}
	}

	obrList := ExtractSegmentGroup(parsedHL7Data, "OBR")
	obrDateTime := ""
	if len(obrList) > 0 {
		obrDateTime = SegFieldValue(obrList[0], "OBR.7")
	}

	// Separate: keep DR and Patient, discard placeholder Observations.
	// This must happen before the OBX early-return so DR mandatory fields
	// (status, subject) can be set even when no observations exist (e.g. ORM orders).
	var dr map[string]interface{}
	var patientRef string
	var kept []map[string]interface{}
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		switch rt {
		case "DiagnosticReport":
			dr = r
			kept = append(kept, r)
		case "Patient":
			if id, ok := r["id"].(string); ok && id != "" {
				patientRef = "Patient/" + id
			}
			kept = append(kept, r)
		case "Observation":
			// discard — rebuilt from OBX loop below
		default:
			kept = append(kept, r)
		}
	}
	resources = kept

	// ── Enrich DiagnosticReport with mandatory FHIR R4 fields ────────────────
	// Run unconditionally — DiagnosticReport.status (1..1) and subject must be
	// set regardless of whether OBX observations are present in the message.
	if dr != nil {
		// subject — link every DiagnosticReport to its Patient
		if ruleOn(rules.DRSubject) && patientRef != "" {
			dr["subject"] = map[string]interface{}{"reference": patientRef}
		}
		// status — derivation chain: OBR.25 → ORC.1 → "unknown"
		if ruleOn(rules.DRStatus) {
			if _, hasStatus := dr["status"]; !hasStatus {
				rawStatus := ""
				if len(obrList) > 0 {
					rawStatus = SegFieldValue(obrList[0], "OBR.25")
				}
				if rawStatus == "" {
					// Fallback: derive from ORC.1 (order control code, Table 0119)
					orcList := ExtractSegmentGroup(parsedHL7Data, "ORC")
					if len(orcList) > 0 {
						rawStatus = SegFieldValue(orcList[0], "ORC.1")
						dr["status"] = ORCStatusToDRStatus(rawStatus)
					} else {
						dr["status"] = "unknown"
					}
				} else {
					dr["status"] = OBRStatusToDRStatus(rawStatus)
				}
			}
		}
		// code — always re-process OBR.4 through BuildCodeableConceptFromCE so the Tier-3
		// facility namespace is applied when the sender omitted the coding system.
		// The mapping engine may have set dr["code"] already but without a system;
		// BuildCodeableConceptFromCE fills the gap rather than leaving an invalid system-less Coding.
		// A warning is emitted when the fallback was needed so data quality is visible.
		if ruleOn(rules.DRCode) && len(obrList) > 0 {
			if obrCode := SegFieldValue(obrList[0], "OBR.4"); obrCode != "" {
				cc, tier3Used := BuildCodeableConceptFromCE(obrCode, rules.FacilityNamespace)
				dr["code"] = cc
				if tier3Used {
					warnings = append(warnings,
						"DiagnosticReport.code: OBR.4 has no standard coding system — "+
							"facility namespace applied as fallback ("+rules.FacilityNamespace+"). "+
							"Ask the sending system to include the coding system in OBR.4.3.")
				}
			}
		}
	}

	obxList := ExtractSegmentGroup(parsedHL7Data, "OBX")
	if len(obxList) == 0 {
		warnings = append(warnings, "hl7.assemble_observations: no OBX segments found in segmentGroups")
		return resources, warnings
	}
	log.Printf("🔬 hl7.assemble_observations: %d OBX segments", len(obxList))

	// Determine effective collapse setting.
	// CollapseTextOBXServices narrows the merge to specific OBR.24 service codes.
	// When the list is non-empty, check OBR.24 of the first OBR; if it doesn't
	// match, merging is suppressed even when CollapseTextOBX is ON.
	collapseOBX := ruleOn(rules.CollapseTextOBX)
	if collapseOBX && len(rules.CollapseTextOBXServices) > 0 {
		obrService := ""
		if len(obrList) > 0 {
			obrService = strings.ToUpper(strings.TrimSpace(SegFieldValue(obrList[0], "OBR.24")))
		}
		matched := false
		for _, svc := range rules.CollapseTextOBXServices {
			if strings.ToUpper(strings.TrimSpace(svc)) == obrService {
				matched = true
				break
			}
		}
		collapseOBX = matched
	}

	// Group OBX segments: consecutive TX/FT segments with the same OBX.3 are merged
	// into a single logical "text block" when collapseOBX is true.
	obxGroups := groupOBXSegments(obxList, collapseOBX)

	// Build OBX-index → []NTE text mapping using segmentOrder for positional ordering.
	// NTE immediately after OBX[i] (before next OBX or end) → Observation[i].note
	// NTE after OBR but before first OBX → DiagnosticReport.note (OBR-level)
	obxNotes, drNotes := extractNTEByPosition(parsedHL7Data)

	// Attach OBR-level NTE notes to DiagnosticReport.note.
	// Multiple NTE lines are one logical comment split by field-length limits → join into one Annotation.
	if dr != nil && len(drNotes) > 0 {
		if combined := joinNTELines(drNotes); combined != "" {
			dr["note"] = []interface{}{map[string]interface{}{"text": combined}}
		}
	}

	// One Observation per OBX group; track whether any Tier-3 fallback was used
	var resultRefs []interface{}
	tier3ObsCount := 0
	for _, grp := range obxGroups {
		var obs map[string]interface{}
		var obsID string
		var tier3Used bool

		if len(grp.segments) == 1 {
			// Single OBX — normal path
			obs, obsID, tier3Used = BuildObservationFromOBX(grp.segments[0], patientRef, rules)
		} else {
			// Multiple continuation TX/FT segments with same OBX.3 → merged Observation
			obs, obsID, tier3Used = buildMergedTextObservation(grp.segments, patientRef, rules)
		}

		// Attach NTE notes from the first OBX in the group (positional index = grp.firstOBXIndex).
		// Multiple NTE lines are one logical comment split by field-length limits → join into one Annotation.
		if noteTexts, ok := obxNotes[grp.firstOBXIndex]; ok && len(noteTexts) > 0 {
			if combined := joinNTELines(noteTexts); combined != "" {
				obs["note"] = []interface{}{map[string]interface{}{"text": combined}}
			}
		}

		resources = append(resources, obs)
		resultRefs = append(resultRefs, map[string]interface{}{"reference": "Observation/" + obsID})
		if tier3Used {
			tier3ObsCount++
		}
	}

	// Advisory warning when Tier-3 facility namespace was applied
	if tier3ObsCount > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"hl7.assemble_observations: %d observation(s) contain local-only codes (no standard system). "+
				"Applied facility namespace '%s' as a Tier-3 fallback so the FHIR bundle is structurally valid. "+
				"Add a 'fhir.standardize_codes' pipeline step to map these codes to standard terminologies (LOINC, SNOMED, etc.).",
			tier3ObsCount, rules.FacilityNamespace,
		))
	}

	// FHIR R4 compliance: Observation.value[x] does not include Attachment (that was added in R5).
	// When an OBX.2=ED observation carries valueAttachment, lift the content into a
	// DocumentReference resource and link the Observation via derivedFrom.
	// Per HL7 v2-to-FHIR mapping guidance (https://build.fhir.org/ig/HL7/v2-to-fhir/):
	//   OBX-5 (ED) → DocumentReference.content[0].attachment
	//   OBX-3      → DocumentReference.type (CodeableConcept)
	// The Observation retains its code and status but has no value[x].
	var docRefs []map[string]interface{}
	docRefSeq := 0
	for _, res := range resources {
		rt, _ := res["resourceType"].(string)
		if rt != "Observation" {
			continue
		}
		att, hasAtt := res["valueAttachment"].(map[string]interface{})
		if !hasAtt || len(att) == 0 {
			continue
		}
		docRefSeq++
		docID := fmt.Sprintf("docref-%d", docRefSeq)

		// Derive a human-readable label for the narrative from the attachment metadata.
		docTitle := "Encapsulated Document"
		if t, ok := att["title"].(string); ok && t != "" {
			docTitle = t
		}
		ct, _ := att["contentType"].(string)
		if ct == "" {
			ct = "unknown type"
		}

		// Build minimal valid R4 DocumentReference
		docRef := map[string]interface{}{
			"resourceType": "DocumentReference",
			"id":           docID,
			"status":       "current",
			"content": []interface{}{
				map[string]interface{}{"attachment": att},
			},
			// dom-6: narrative required for robust resource management
			"text": map[string]interface{}{
				"status": "generated",
				"div": fmt.Sprintf(
					`<div xmlns="http://www.w3.org/1999/xhtml"><p><b>%s</b> (%s)</p></div>`,
					escapeXML(docTitle), escapeXML(ct),
				),
			},
		}
		// Propagate subject from the Observation
		if subj, ok := res["subject"]; ok {
			docRef["subject"] = subj
		}
		// Copy observation code → DocumentReference.type (identifies what the document is)
		if code, ok := res["code"]; ok {
			docRef["type"] = code
		}
		// Copy effectiveDateTime → DocumentReference.date
		if eff, ok := res["effectiveDateTime"].(string); ok && eff != "" {
			docRef["date"] = eff
		}

		docRefs = append(docRefs, docRef)

		// Remove valueAttachment; add derivedFrom pointing to the new DocumentReference.
		// Observation.value[x] is 0..1 so omitting it is valid.
		delete(res, "valueAttachment")
		res["derivedFrom"] = []interface{}{
			map[string]interface{}{"reference": "DocumentReference/" + docID},
		}

		log.Printf("📎 hl7.assemble_observations: lifted ED attachment from Observation/%s → DocumentReference/%s",
			res["id"], docID)
	}
	// Append DocumentReference resources to the bundle after all Observations.
	for _, dr2 := range docRefs {
		resources = append(resources, dr2)
	}

	// Update DiagnosticReport — post-OBX pass
	if dr != nil {
		if ruleOn(rules.DRResultLinks) {
			dr["result"] = resultRefs
		}
		// subject is already set by the pre-OBX pass; overwrite only if it
		// was not set there (patientRef may have changed if fullUrl is known).
		if ruleOn(rules.DRSubject) && patientRef != "" {
			dr["subject"] = map[string]interface{}{"reference": patientRef}
		}
		// status: read OBR.25 directly from the segment (not from dr["status"]
		// which was already set to a FHIR value by the pre-OBX pass).
		if ruleOn(rules.DRStatus) && len(obrList) > 0 {
			if obr25 := SegFieldValue(obrList[0], "OBR.25"); obr25 != "" {
				// OBR.25 has an actual result status — it takes priority.
				dr["status"] = OBRStatusToDRStatus(obr25)
			}
			// If OBR.25 is empty, the pre-OBX pass already set a baseline status;
			// leave it alone.
		}
		if ruleOn(rules.DRCode) && len(obrList) > 0 {
			if obrCode := SegFieldValue(obrList[0], "OBR.4"); obrCode != "" {
				// Always set from OBR.4 so Tier-3 facility namespace is applied.
				cc, _ := BuildCodeableConceptFromCE(obrCode, rules.FacilityNamespace)
				dr["code"] = cc
			}
		}
		if ruleOn(rules.DREffective) {
			if obrDateTime != "" {
				dr["effectiveDateTime"] = ToISO(obrDateTime)
			} else if raw, ok := dr["effectiveDateTime"].(string); ok {
				dr["effectiveDateTime"] = ToISO(raw)
			}
		}
		if raw, ok := dr["issued"].(string); ok {
			dr["issued"] = ToISO(raw)
		}
		if ruleOn(rules.DRCategory) {
			if _, hasCat := dr["category"]; !hasCat {
				dr["category"] = []interface{}{
					map[string]interface{}{
						"coding": []interface{}{
							map[string]interface{}{
								"system":  "http://terminology.hl7.org/CodeSystem/v2-0074",
								"code":    "LAB",
								"display": "Laboratory",
							},
						},
					},
				}
			}
		}
		// Strip empty identifiers
		if ids, ok := dr["identifier"].([]interface{}); ok {
			var filtered []interface{}
			for _, id := range ids {
				if m, ok2 := id.(map[string]interface{}); ok2 && len(m) > 0 {
					filtered = append(filtered, id)
				}
			}
			if len(filtered) == 0 {
				delete(dr, "identifier")
			} else {
				dr["identifier"] = filtered
			}
		}
		// Regenerate DR narrative now that result[] links are wired.
		// The narrative produced by the mapping engine ran before result[] was
		// populated, so it would say "0 results". Refresh it here.
		if ruleOn(rules.DRNarrative) {
			dr["text"] = map[string]interface{}{
				"status": "generated",
				"div":    buildDRNarrative(dr, len(resultRefs)),
			}
		}
	}

	log.Printf("✅ hl7.assemble_observations: %d Observations, %d DR result refs", len(obxList), len(resultRefs))
	return resources, warnings
}

// extractNTEByPosition uses segmentOrder to map NTE segments to their
// preceding context (OBX or OBR) by absolute position in the message.
//
// Returns:
//   obxNotes  map[obxIndex][]string  — note texts for Observation[i].note
//   drNotes   []string               — note texts for DiagnosticReport.note (NTE after OBR, before first OBX)
//
// NTE.3 (comment text) may contain multiple lines separated by "~" (field repeat).
// Empty NTE.3 values (blank comment lines) are preserved as empty strings so callers
// can decide whether to include them.
func extractNTEByPosition(parsedHL7Data map[string]interface{}) (obxNotes map[int][]string, drNotes []string) {
	obxNotes = map[int][]string{}

	// segmentOrder may be []string (typed Go path, test/wizard) or
	// []interface{} (JSON-unmarshal path, production pipeline).
	var segOrder []string
	switch v := parsedHL7Data["segmentOrder"].(type) {
	case []string:
		segOrder = v
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				segOrder = append(segOrder, s)
			}
		}
	}
	if len(segOrder) == 0 {
		return
	}

	// Use ExtractSegmentGroup which already handles both typed and JSON paths.
	nteSegs := ExtractSegmentGroup(parsedHL7Data, "NTE")
	nteConsumed := 0

	obxIdx := -1      // index of last seen OBX (-1 = none yet)
	afterOBR := false // have we passed at least one OBR?
	afterOBX := false // have we passed at least one OBX?

	for _, segType := range segOrder {
		switch segType {
		case "OBR":
			afterOBR = true
		case "OBX":
			obxIdx++
			afterOBX = true
		case "NTE":
			if nteConsumed >= len(nteSegs) {
				break
			}
			text := nteTextFromEnhanced(nteSegs[nteConsumed])
			nteConsumed++

			if afterOBX {
				obxNotes[obxIdx] = append(obxNotes[obxIdx], text)
			} else if afterOBR {
				drNotes = append(drNotes, text)
			}
			// NTE before OBR (patient-level) → ignored for now
		}
	}
	return
}

// joinNTELines joins multiple NTE.3 comment lines into a single Annotation text.
// Blank lines (empty NTE.3) are preserved as paragraph breaks (\n\n).
// Consecutive non-empty lines are joined with a single \n.
func joinNTELines(lines []string) string {
	var parts []string
	for _, l := range lines {
		parts = append(parts, l) // keep blanks; they become \n\n below
	}
	// Trim trailing blanks
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return ""
	}
	// Build output: consecutive non-empty lines joined by \n, blank lines become \n\n
	result := ""
	for i, p := range parts {
		if i == 0 {
			result = p
			continue
		}
		if p == "" {
			result += "\n" // adds an extra \n on top of the upcoming \n from next line
		} else {
			result += "\n" + p
		}
	}
	return result
}

// nteTextFromEnhanced extracts NTE.3 (comment text) from a typed hl7.EnhancedSegment.
func nteTextFromEnhanced(seg hl7.EnhancedSegment) string {
	for _, f := range seg.Fields {
		if f.Key == "NTE.3" {
			return f.Value
		}
	}
	return ""
}

// nteText extracts NTE.3 (comment text) from a generic map representation
// (used when the segment arrived as map[string]interface{} via JSON unmarshalling).
func nteText(seg map[string]interface{}) string {
	fields, _ := seg["fields"].([]interface{})
	for _, f := range fields {
		fm, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		if key, _ := fm["key"].(string); key == "NTE.3" {
			val, _ := fm["value"].(string)
			return val
		}
	}
	// Fallback: check map-style fields
	if fieldsMap, ok := seg["fields"].(map[string]interface{}); ok {
		if v, ok2 := fieldsMap["NTE.3"].(string); ok2 {
			return v
		}
	}
	return ""
}

// toSegList normalises a segmentGroups value to []map[string]interface{}.
// The parser stores single segments as a map and multiple as a []interface{}.
func toSegList(raw interface{}) []map[string]interface{} {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]interface{}:
		return []map[string]interface{}{v}
	}
	return nil
}

// buildFacilityCodeSystem scans all resources for codings that use facilityNS as
// their system and emits a FHIR CodeSystem resource that enumerates every code.
// This makes the Bundle self-contained so validators can resolve the system URI
// without needing an external registry (stops "CodeSystem not found" warnings).
func buildFacilityCodeSystem(resources []map[string]interface{}, facilityNS string) map[string]interface{} {
	seen := map[string]string{} // code → display

	// Walk every coding in every resource
	var collectCodings func(v interface{})
	collectCodings = func(v interface{}) {
		switch val := v.(type) {
		case map[string]interface{}:
			// If this map looks like a Coding with our system, record it
			if sys, _ := val["system"].(string); sys == facilityNS {
				if code, _ := val["code"].(string); code != "" {
					if _, exists := seen[code]; !exists {
						seen[code], _ = val["display"].(string)
					}
				}
			}
			for _, child := range val {
				collectCodings(child)
			}
		case []interface{}:
			for _, item := range val {
				collectCodings(item)
			}
		}
	}
	for _, r := range resources {
		collectCodings(r)
	}

	// Build concept list sorted for determinism
	concepts := make([]interface{}, 0, len(seen))
	for code, display := range seen {
		concept := map[string]interface{}{"code": code}
		if display != "" {
			concept["display"] = display
		}
		concepts = append(concepts, concept)
	}

	// Derive a human-readable name from the namespace URI
	name := strings.TrimPrefix(facilityNS, "urn:facility:")
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.Title(name) + " Local Codes" //nolint:staticcheck — Title is fine for ASCII facility names

	return map[string]interface{}{
		"resourceType": "CodeSystem",
		"url":          facilityNS,
		"status":       "active",
		"content":      "complete",
		"name":         strings.ReplaceAll(name, " ", ""),
		"title":        name,
		"description":  "Locally-defined codes used by this facility. Applied as Tier-3 fallback by ezHealthKonnect when no standard terminology mapping is available.",
		"concept":      concepts,
	}
}

// RebuildFHIRBundle wraps a resource list back into a FHIR Bundle collection.
func RebuildFHIRBundle(resources []map[string]interface{}) map[string]interface{} {
	entries := make([]interface{}, 0, len(resources))
	for _, r := range resources {
		entries = append(entries, map[string]interface{}{"resource": r})
	}
	return map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "collection",
		"entry":        entries,
	}
}
