package services

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/services/hl7assembly"
)

// postProcessORU rebuilds Observations from raw OBX segments and patches the
// DiagnosticReport produced by the field-mapping engine.  Called only for
// ORU^R01 (and related) message families.
func (s *HL7FHIRTransformServiceV3) postProcessORU(
	parsedHL7Data map[string]interface{},
	resources []map[string]interface{},
	warnings, errors []string,
) ([]map[string]interface{}, []string, []string) {

	obxList := s.extractSegmentGroup(parsedHL7Data, "OBX")
	if len(obxList) == 0 {
		warnings = append(warnings, "ORU post-processing: no OBX segments found in segmentGroups")
		return resources, warnings, errors
	}
	log.Printf("🔬 ORU post-processing: found %d OBX segments", len(obxList))

	// Discard validation errors from placeholder resources built by the mapping engine.
	// Observations are fully replaced; DiagnosticReport is patched. Both get the required
	// fields added below, so their pre-patch validation errors are misleading noise.
	var filteredErrors []string
	for _, e := range errors {
		if !strings.Contains(e, "Observation.") && !strings.Contains(e, "DiagnosticReport.") {
			filteredErrors = append(filteredErrors, e)
		}
	}
	errors = filteredErrors

	// Extract OBR.7 (observation date/time) for DR effectiveDateTime
	obrList := s.extractSegmentGroup(parsedHL7Data, "OBR")
	obrDateTime := ""
	if len(obrList) > 0 {
		obrDateTime = segFieldValue(obrList[0], "OBR.7")
	}

	// Locate existing DiagnosticReport and Patient; strip placeholder Observations
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
			if id, ok := r["id"].(string); ok {
				patientRef = "Patient/" + id
			}
			kept = append(kept, r)
		case "Observation":
			// discard — we rebuild from OBX loop
		default:
			kept = append(kept, r)
		}
	}
	resources = kept

	// Build one Observation per OBX
	var resultRefs []interface{}
	for _, obxSeg := range obxList {
		obs, obsID := s.buildObservationFromOBX(obxSeg, patientRef)
		resources = append(resources, obs)
		resultRefs = append(resultRefs, map[string]interface{}{"reference": "Observation/" + obsID})
	}

	// Update DiagnosticReport: add result[], fix status, fix dates, add category
	if dr != nil {
		dr["result"] = resultRefs
		if patientRef != "" {
			dr["subject"] = map[string]interface{}{"reference": patientRef}
		}
		// Fix status: raw "F" → "final"
		// If the template left status empty/nil, fall back to OBR.25 directly.
		rawStatus, _ := dr["status"].(string)
		if rawStatus == "" && len(obrList) > 0 {
			rawStatus = segFieldValue(obrList[0], "OBR.25")
		}
		if rawStatus != "" {
			dr["status"] = s.transformOBRStatusToDRStatus(rawStatus)
		} else {
			dr["status"] = "unknown"
		}
		// Add code from OBR.4 if missing (required FHIR R4 field)
		if _, hasCode := dr["code"]; !hasCode && len(obrList) > 0 {
			obrCode := segFieldValue(obrList[0], "OBR.4")
			if obrCode != "" {
				dr["code"] = s.transformCEToCodeableConcept(obrCode, nil)
			}
		}
		// Fix issued: raw TS → ISO datetime
		if rawIssued, ok := dr["issued"].(string); ok {
			dr["issued"] = tsToISODateTime(rawIssued)
		}
		// Fix effectiveDateTime — prefer OBR.7, else mapping engine value
		if obrDateTime != "" {
			dr["effectiveDateTime"] = tsToISODateTime(obrDateTime)
		} else if rawEff, ok := dr["effectiveDateTime"].(string); ok {
			dr["effectiveDateTime"] = tsToISODateTime(rawEff)
		}
		// Add standard laboratory category if not already set
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
		// Filter empty identifiers (e.g. when OBR.2 is blank)
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
	}

	// Fix Patient: normalize gender and date
	for _, r := range resources {
		if rt, _ := r["resourceType"].(string); rt == "Patient" {
			if bd, ok := r["birthDate"].(string); ok {
				r["birthDate"] = s.transformTSToDate(bd)
			}
			if g, ok := r["gender"].(string); ok {
				r["gender"] = s.transformGender(g)
			}
		}
	}

	log.Printf("✅ ORU post-processing complete: %d Observations, DR result[] = %d refs",
		len(obxList), len(resultRefs))
	return resources, warnings, errors
}

// sanitizeFHIRResources applies post-assembly structural corrections that cannot
// be expressed as template rules:
//   - strips empty identifier/coding/annotation objects (FHIR constraint "Object must have some content")
//   - suppresses raw HL7 strings in Reference-typed fields (partOf, subject, etc.)
//   - removes Encounter.diagnosis entries missing a required condition reference
//   - injects Appointment.participant.status = "needs-action" when absent (required field)
//   - removes Patient.contact entries with no name/telecom/address/org (constraint pat-1)
//   - sanitizes supportingInformation: raw strings → Reference{display:…}
func (s *HL7FHIRTransformServiceV3) sanitizeFHIRResources(resources []map[string]interface{}) []map[string]interface{} {
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		switch rt {

		case "Patient":
			// Remove contact entries that violate the pat-1 constraint (must have name,
			// telecom, address, or organization with real content). Also strips contacts
			// whose only content is a single-character name.family — this happens when
			// PID.8 (Administrative Sex, e.g. "M") is mis-mapped to contact.name by the
			// heuristic matcher because it's an XPN-shaped field adjacent to PID.5.
			// Contact may be []interface{} or []map[string]interface{} depending on
			// which setter path created it — normalize before filtering.
			var contactMaps []map[string]interface{}
			switch cv := r["contact"].(type) {
			case []interface{}:
				for _, c := range cv {
					if cm, ok := c.(map[string]interface{}); ok {
						contactMaps = append(contactMaps, cm)
					}
				}
			case []map[string]interface{}:
				contactMaps = cv
			}
			if contactMaps != nil {
				var cleaned []interface{}
				for _, cm := range contactMaps {
					// Check each of the pat-1 permitted content fields.
					// "relationship" alone is NOT sufficient — pat-1 requires name, telecom,
					// address, or organization to be present (relationship is contextual only).
					hasContent := false
					for _, key := range []string{"telecom", "address", "organization"} {
						if v, exists := cm[key]; exists && v != nil {
							hasContent = true
							break
						}
					}
					// Name requires at least one non-trivial component (family ≥ 2 chars or given).
					if !hasContent {
						if nameVal, nameExists := cm["name"]; nameExists && nameVal != nil {
							if nm, isMap := nameVal.(map[string]interface{}); isMap {
								if fam, ok := nm["family"].(string); ok && len([]rune(fam)) >= 2 {
									hasContent = true
								}
								if given, ok := nm["given"].([]interface{}); ok && len(given) > 0 {
									hasContent = true
								}
							}
						}
					}
					if hasContent {
						cleaned = append(cleaned, cm)
					}
				}
				if len(cleaned) == 0 {
					delete(r, "contact")
				} else {
					r["contact"] = cleaned
				}
			}

			// Strip empty identifier objects; delete key entirely when all entries were empty.
			if filtered := filterNonEmptySlice(r["identifier"]); filtered == nil {
				delete(r, "identifier")
			} else {
				r["identifier"] = filtered
			}

		case "Encounter":
			// Strip empty identifier objects; delete key entirely when all entries were empty.
			if filtered := filterNonEmptySlice(r["identifier"]); filtered == nil {
				delete(r, "identifier")
			} else {
				r["identifier"] = filtered
			}

			// Remove Encounter.diagnosis entries that are missing the required condition ref.
			// Handle both []interface{} (setArrayField) and []map[string]interface{} (setNestedArrayField).
			var diagMaps []map[string]interface{}
			switch dv := r["diagnosis"].(type) {
			case []interface{}:
				for _, d := range dv {
					if dm, ok := d.(map[string]interface{}); ok {
						diagMaps = append(diagMaps, dm)
					}
				}
			case []map[string]interface{}:
				diagMaps = dv
			}
			if diagMaps != nil {
				var kept []interface{}
				for _, dm := range diagMaps {
					if _, hasCondition := dm["condition"]; hasCondition {
						kept = append(kept, dm)
					}
				}
				if len(kept) == 0 {
					delete(r, "diagnosis")
				} else {
					r["diagnosis"] = kept
				}
			}

			// Suppress partOf when it was set to a raw HL7 string (must be Reference{}).
			if pof, exists := r["partOf"]; exists {
				if _, isStr := pof.(string); isStr {
					delete(r, "partOf")
				}
			}

			// Default Encounter.status to "unknown" when absent (required 1..1 in FHIR R4).
			if _, hasStatus := r["status"]; !hasStatus {
				r["status"] = "unknown"
			}

			// hospitalization sub-object: origin/destination must be Reference objects.
			// Raw HL7 XCN strings land here when the type coercion layer was bypassed by
			// an older template. Wrap them or remove them.
			if hosp, ok := r["hospitalization"].(map[string]interface{}); ok {
				for _, refField := range []string{"origin", "destination"} {
					if v, exists := hosp[refField]; exists {
						if s, isStr := v.(string); isStr {
							if s != "" {
								hosp[refField] = map[string]interface{}{"display": s}
							} else {
								delete(hosp, refField)
							}
						}
					}
				}
			}

			// Guard Encounter.appointment: must be array of References.
			// Assembly rules wire it as []interface{}, but normalize raw strings.
			if appt, exists := r["appointment"]; exists {
				switch av := appt.(type) {
				case map[string]interface{}:
					// Already a Reference object — wrap in array.
					r["appointment"] = []interface{}{av}
				case string:
					if av != "" {
						r["appointment"] = []interface{}{map[string]interface{}{"display": av}}
					} else {
						delete(r, "appointment")
					}
				}
			}

			// Encounter.subject must be a Reference to the Patient resource
			// (set by the assembly code as {"reference": "urn:uuid:..."}).
			// If a heuristic mapping has placed a person display name here
			// ({"display": "Colfer, Eoin"} with no "reference" key), remove it —
			// the assembly code will set the correct patient reference afterwards.
			if subj, ok := r["subject"].(map[string]interface{}); ok {
				if _, hasRef := subj["reference"]; !hasRef {
					delete(r, "subject")
				}
			}

		case "Appointment":
			// Inject participant.status = "needs-action" when missing (FHIR required, 1..1).
			// Participant can be []interface{} (from setArrayField) or []map[string]interface{}
			// (from setNestedArrayField) — normalize to []map[string]interface{} first.
			var participantMaps []map[string]interface{}
			switch pv := r["participant"].(type) {
			case []interface{}:
				for _, p := range pv {
					if pm, ok := p.(map[string]interface{}); ok {
						participantMaps = append(participantMaps, pm)
					}
				}
			case []map[string]interface{}:
				participantMaps = pv
			}
			if len(participantMaps) > 0 {
				for _, pm := range participantMaps {
					if _, hasStatus := pm["status"]; !hasStatus {
						pm["status"] = "needs-action"
					}
					// Normalize participant.type: some template paths write a naked
					// CodeableConcept map (not wrapped in an array). Convert to array
					// so the cleanup loop below handles it uniformly.
					if tm, ok := pm["type"].(map[string]interface{}); ok {
						pm["type"] = []interface{}{tm}
					}

					// Clean participant.type: fix nested text maps, drop empty entries,
					// and drop entries that carry a person name or identifier artifact
					// instead of a coded role (e.g. "ATND").
					if types, ok := pm["type"].([]interface{}); ok {
						var actorDisplay string
						if actor, ok2 := pm["actor"].(map[string]interface{}); ok2 {
							actorDisplay, _ = actor["display"].(string)
						}
						var cleanedTypes []interface{}
						for _, t := range types {
							tm, ok := t.(map[string]interface{})
							if !ok {
								continue
							}
							// Unwrap nested text map (CodeableConcept landed in text field).
							if textVal, exists := tm["text"]; exists {
								if textMap, isMap := textVal.(map[string]interface{}); isMap {
									if inner, ok := textMap["text"].(string); ok {
										tm["text"] = inner
									} else {
										delete(tm, "text")
									}
								}
							}
							// Drop entries that are identifier artifacts or person names
							// rather than coded role values (e.g. "ATND").
							// transformCEToCodeableConcept builds coding as []map[string]interface{},
							// not []interface{}, so we must handle both slice types.
							var codingMaps []map[string]interface{}
							switch cv := tm["coding"].(type) {
							case []interface{}:
								for _, ci := range cv {
									if cm, ok2 := ci.(map[string]interface{}); ok2 {
										codingMaps = append(codingMaps, cm)
									}
								}
							case []map[string]interface{}:
								codingMaps = cv
							}
							if len(codingMaps) > 0 {
								// A coding whose every code is purely numeric is a
								// mis-mapped appointment/resource identifier — remove.
								allNumeric := true
								for _, cm := range codingMaps {
									code, _ := cm["code"].(string)
									if code == "" {
										allNumeric = false
										break
									}
									for _, c := range code {
										if c < '0' || c > '9' {
											allNumeric = false
											break
										}
									}
									if !allNumeric {
										break
									}
								}
								if allNumeric {
									continue // drop — numeric identifier artifact
								}
							} else {
								// Text-only entries: drop when they look like person names.
								if txt, ok := tm["text"].(string); ok && txt != "" {
									if txt == actorDisplay ||
										strings.Contains(txt, ", ") ||
										strings.Contains(txt, "^") {
										continue // drop — person name, not a role code
									}
								}
							}
							if len(tm) > 0 {
								cleanedTypes = append(cleanedTypes, tm)
							}
						}
						if len(cleanedTypes) == 0 {
							delete(pm, "type")
						} else {
							pm["type"] = cleanedTypes
						}
					}

					// Guard actor.display: must be a clean person/resource name string.
					// CE/CWE transforms land a CodeableConcept map at actor["display"];
					// XCN strings contain "^" separators. Both need normalisation.
					// Some older templates map AI*.3 to .actor (not .actor.display),
					// placing the CodeableConcept directly at pm["actor"] — handle that too.
					if actor, ok := pm["actor"].(map[string]interface{}); ok {
						var disp string
						switch dv := actor["display"].(type) {
						case string:
							disp = dv
						case map[string]interface{}:
							// CodeableConcept at actor.display slot — extract text or coding.display
							if txt, _ := dv["text"].(string); txt != "" {
								disp = txt
							} else if codings, _ := dv["coding"].([]interface{}); len(codings) > 0 {
								if cm, _ := codings[0].(map[string]interface{}); cm != nil {
									disp, _ = cm["display"].(string)
								}
							}
							if disp != "" {
								actor["display"] = disp
							}
						case nil:
							// actor IS a CodeableConcept (mapped to .actor not .actor.display).
							// Extract the name from the actor map's own text/coding fields.
							if txt, _ := actor["text"].(string); txt != "" {
								disp = txt
							}
							if disp == "" {
								switch cv := actor["coding"].(type) {
								case []interface{}:
									if len(cv) > 0 {
										if cm, _ := cv[0].(map[string]interface{}); cm != nil {
											if d, _ := cm["display"].(string); d != "" {
												disp = d
											} else {
												disp, _ = cm["code"].(string)
											}
										}
									}
								case []map[string]interface{}:
									if len(cv) > 0 {
										if d, _ := cv[0]["display"].(string); d != "" {
											disp = d
										} else {
											disp, _ = cv[0]["code"].(string)
										}
									}
								}
							}
							if disp != "" {
								pm["actor"] = map[string]interface{}{"display": disp}
								actor = pm["actor"].(map[string]interface{})
							}
						}
						if disp == "" {
							// Preserve reference-type actors (actor.reference is a valid FHIR
							// actor without a display — e.g. Patient/42 from condition composite).
							if ref, _ := actor["reference"].(string); ref != "" {
								// keep actor as-is
							} else {
								delete(pm, "actor")
							}
						} else if strings.Contains(disp, "^") {
							// XCN format: id^family^given^middle^... (0-indexed).
							// Mirror the logic in formatXCNDisplayName exactly.
							parts := strings.SplitN(disp, "^", 8)
							get := func(i int) string {
								if i < len(parts) {
									return strings.TrimSpace(parts[i])
								}
								return ""
							}
							family, given := get(1), get(2)
							switch {
							case family != "" && given != "":
								actor["display"] = family + ", " + given
							case family != "":
								actor["display"] = family
							case given != "":
								actor["display"] = given
							case get(0) != "":
								// ID-only XCN — use the ID rather than deleting the actor
								actor["display"] = get(0)
							default:
								delete(pm, "actor")
							}
						}
					} else if pm["actor"] != nil {
						delete(pm, "actor")
					}
				}
				// app-1: "Either the type or actor on the participant SHALL be specified."
				// Remove participant entries with neither — a status-only entry is invalid.
				var normalized []interface{}
				for _, pm := range participantMaps {
					_, hasType := pm["type"]
					_, hasActor := pm["actor"]
					if hasType || hasActor {
						normalized = append(normalized, pm)
					}
				}
				if len(normalized) == 0 {
					// All entries lacked both type and actor. Synthesize a minimal valid
					// participant from the first entry: inject a generic "PART" type code
					// so app-1 is satisfied without fabricating a practitioner identity.
					first := participantMaps[0]
					first["type"] = []interface{}{
						map[string]interface{}{
							"coding": []map[string]interface{}{
								{"system": "http://terminology.hl7.org/CodeSystem/v3-ParticipationType", "code": "PART"},
							},
						},
					}
					normalized = []interface{}{first}
				}
				r["participant"] = normalized
			}

			// Default Appointment.status to "booked" when absent (required 1..1).
			// SCH.25 (Filler Status Code) may be absent in many real-world messages.
			if _, hasStatus := r["status"]; !hasStatus {
				r["status"] = "booked"
			}

			// app-4 constraint: cancelationReason is only valid when status is
			// "cancelled" or "noshow". Remove it for any other status.
			if status, _ := r["status"].(string); status != "cancelled" && status != "noshow" {
				delete(r, "cancelationReason")
			}

			// Guard minutesDuration: must be a positive integer. Remove if it's a
			// unit-code string like "m" or "min" that crept in from a collision.
			if md, exists := r["minutesDuration"]; exists {
				switch v := md.(type) {
				case int:
					if v <= 0 {
						delete(r, "minutesDuration")
					}
				case float64:
					if v <= 0 {
						delete(r, "minutesDuration")
					} else {
						r["minutesDuration"] = int(v)
					}
				case string:
					if n, err := strconv.Atoi(v); err == nil && n > 0 {
						r["minutesDuration"] = n
					} else {
						delete(r, "minutesDuration")
					}
				}
			}

			// Guard priority: must be an unsignedInt. A unit-code string like "min" gets
			// removed — the field is optional so absence is safer than an invalid value.
			if pv, exists := r["priority"]; exists {
				switch v := pv.(type) {
				case int:
					if v < 0 {
						delete(r, "priority")
					}
				case float64:
					if v < 0 {
						delete(r, "priority")
					} else {
						r["priority"] = int(v)
					}
				case string:
					if n, err := strconv.Atoi(v); err == nil && n >= 0 {
						r["priority"] = n
					} else {
						delete(r, "priority")
					}
				}
			}

			// Guard Appointment.start / Appointment.end: must be ISO dateTime strings.
			// Raw HL7 name tokens or offset numbers land here from template collisions;
			// remove them so the validator does not flag invalid dateTime format.
			isISODateTime := func(s string) bool {
				if len(s) < 4 {
					return false
				}
				// Accept YYYYMMDD..., YYYY-MM-DD, YYYY-MM-DDThh:mm:ss...
				for _, c := range s[:4] {
					if c < '0' || c > '9' {
						return false
					}
				}
				return true
			}
			for _, dtField := range []string{"start", "end"} {
				if v, exists := r[dtField]; exists {
					if s, ok := v.(string); ok {
						if !isISODateTime(s) {
							delete(r, dtField)
						}
					}
				}
			}

			// Sanitize reasonReference: raw HL7 name strings must be wrapped as
			// Reference{display:…} objects; remove empty entries.
			if rrs, ok := r["reasonReference"].([]interface{}); ok {
				var fixedRR []interface{}
				for _, item := range rrs {
					switch v := item.(type) {
					case string:
						if v != "" {
							fixedRR = append(fixedRR, map[string]interface{}{"display": v})
						}
					case map[string]interface{}:
						if len(v) > 0 {
							fixedRR = append(fixedRR, v)
						}
					}
				}
				if len(fixedRR) == 0 {
					delete(r, "reasonReference")
				} else {
					r["reasonReference"] = fixedRR
				}
			}

			// Convert supportingInformation string items to Reference{display:…}.
			if si, ok := r["supportingInformation"].([]interface{}); ok {
				var fixed []interface{}
				for _, item := range si {
					switch v := item.(type) {
					case string:
						if v != "" {
							fixed = append(fixed, map[string]interface{}{"display": v})
						}
					case map[string]interface{}:
						fixed = append(fixed, v)
					}
				}
				if len(fixed) == 0 {
					delete(r, "supportingInformation")
				} else {
					r["supportingInformation"] = fixed
				}
			}

			// Guard reasonCode: coding.code must be a machine-readable token, not
			// free text. When every code in an entry contains spaces, the value is
			// a plain description — move it to text and clear coding.
			if rcList, ok := r["reasonCode"].([]interface{}); ok {
				for _, rc := range rcList {
					rcMap, ok := rc.(map[string]interface{})
					if !ok {
						continue
					}
					var codings []interface{}
					switch cv := rcMap["coding"].(type) {
					case []interface{}:
						codings = cv
					case []map[string]interface{}:
						for _, m := range cv {
							codings = append(codings, m)
						}
					}
					if len(codings) == 0 {
						continue
					}
					// Check whether all codes look like free text (contain spaces)
					allFreeText := true
					for _, c := range codings {
						cm, ok := c.(map[string]interface{})
						if !ok {
							allFreeText = false
							break
						}
						code, _ := cm["code"].(string)
						if !strings.Contains(code, " ") {
							allFreeText = false
							break
						}
					}
					if allFreeText {
						// Promote the first code string to text, drop coding
						first, _ := codings[0].(map[string]interface{})
						if first != nil {
							if existing, _ := rcMap["text"].(string); existing == "" {
								rcMap["text"], _ = first["code"].(string)
							}
						}
						delete(rcMap, "coding")
					}
				}
			}
		}
	}
	return resources
}

// filterNonEmptySlice returns the slice with empty map entries removed.
// Used to strip {} identifier/coding objects that fail "Object must have some content".
func filterNonEmptySlice(raw interface{}) interface{} {
	slice, ok := raw.([]interface{})
	if !ok {
		return raw
	}
	var out []interface{}
	for _, item := range slice {
		switch v := item.(type) {
		case map[string]interface{}:
			if len(v) > 0 {
				out = append(out, v)
			}
		default:
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildObservationFromOBX constructs a FHIR Observation resource from a single OBX segment.
func (s *HL7FHIRTransformServiceV3) buildObservationFromOBX(seg hl7.EnhancedSegment, patientRef string) (map[string]interface{}, string) {
	setNum := segFieldValue(seg, "OBX.1")
	if setNum == "" {
		setNum = fmt.Sprintf("%d", seg.SegmentIndex+1)
	}
	obsID := fmt.Sprintf("obs-%s", setNum)

	valueType := strings.ToUpper(strings.TrimSpace(segFieldValue(seg, "OBX.2")))
	codeRaw := segFieldValue(seg, "OBX.3")
	valueRaw := segFieldValue(seg, "OBX.5")
	unitRaw := segFieldValue(seg, "OBX.6")
	refRange := segFieldValue(seg, "OBX.7")
	abnFlag := segFieldValue(seg, "OBX.8")
	statusRaw := segFieldValue(seg, "OBX.11")
	obsDate := segFieldValue(seg, "OBX.14")
	performerRaw := segFieldValue(seg, "OBX.16")

	obs := map[string]interface{}{
		"resourceType": "Observation",
		"id":           obsID,
		"status":       s.transformOBXStatusToObsStatus(statusRaw),
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  "http://terminology.hl7.org/CodeSystem/observation-category",
						"code":    "laboratory",
						"display": "Laboratory",
					},
				},
			},
		},
	}

	// code — OBX.3 via ce_to_codeableconcept (supports CWE alternate codes)
	if codeRaw != "" {
		obs["code"] = s.transformCEToCodeableConcept(codeRaw, nil)
	}

	// subject
	if patientRef != "" {
		obs["subject"] = map[string]interface{}{"reference": patientRef}
	}

	// effectiveDateTime
	if obsDate != "" {
		obs["effectiveDateTime"] = tsToISODateTime(obsDate)
	}

	// value[x] — dispatch by OBX.2 type
	if valueRaw != "" {
		switch valueType {
		case "NM":
			qty := map[string]interface{}{}
			// FHIR requires valueQuantity.value as a number, not a string
			if fv, err := strconv.ParseFloat(strings.TrimSpace(valueRaw), 64); err == nil {
				qty["value"] = fv
			} else {
				qty["value"] = valueRaw // fallback to string if not parseable
			}
			if unitRaw != "" {
				qty["unit"] = unitRaw
				qty["system"] = normalizeHL7System("UCUM")
				qty["code"] = unitRaw
			}
			obs["valueQuantity"] = qty
		case "CE", "CWE", "CNE":
			obs["valueCodeableConcept"] = s.transformCEToCodeableConcept(valueRaw, nil)
		case "ST", "TX", "FT":
			obs["valueString"] = valueRaw
		case "TS", "DT":
			obs["valueDateTime"] = tsToISODateTime(valueRaw)
		default:
			obs["valueString"] = valueRaw
		}
	}

	// referenceRange — parse "low-high" pattern into structured low/high quantities
	if refRange != "" {
		rr := map[string]interface{}{"text": refRange}
		parts := strings.SplitN(refRange, "-", 2)
		if len(parts) == 2 {
			if lo, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64); err == nil {
				loEntry := map[string]interface{}{"value": lo}
				if unitRaw != "" {
					loEntry["unit"] = unitRaw
				}
				rr["low"] = loEntry
			}
			if hi, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
				hiEntry := map[string]interface{}{"value": hi}
				if unitRaw != "" {
					hiEntry["unit"] = unitRaw
				}
				rr["high"] = hiEntry
			}
		}
		obs["referenceRange"] = []interface{}{rr}
	}

	// interpretation (abnormal flag)
	if abnFlag != "" && strings.ToUpper(abnFlag) != "N" {
		obs["interpretation"] = []interface{}{
			s.transformAbnormalFlagToInterpretation(abnFlag),
		}
	}

	// performer — XCN: ID^LastName^FirstName → use component 2+3 as display, fallback to raw
	if performerRaw != "" {
		parts := strings.Split(performerRaw, "^")
		display := performerRaw
		if len(parts) >= 3 && parts[2] != "" {
			display = parts[2] + " " + parts[1]
		} else if len(parts) >= 2 && parts[1] != "" {
			display = parts[1]
		}
		obs["performer"] = []interface{}{
			map[string]interface{}{"display": display},
		}
	}

	return obs, obsID
}

// cleanFHIRResource recursively removes keys with nil values or empty
// arrays/objects from any FHIR node.  Applied universally to every resource
// before output so that no message type emits explicit JSON nulls (invalid for
// most FHIR element types) or empty arrays ("the property should not be present
// if it has no values" — FHIR R4 §1.3.0.1).
//
// Rules:
//   - nil value          → key deleted
//   - empty []interface{} → key deleted
//   - map entry becomes empty after recursive cleaning → key deleted
//   - slice entry becomes nil after recursive cleaning  → entry dropped
func cleanFHIRResource(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, mv := range val {
			if cleaned := cleanFHIRResource(mv); cleaned == nil {
				delete(val, k)
			} else {
				val[k] = cleaned
			}
		}
		if len(val) == 0 {
			return nil
		}
		return val
	case []interface{}:
		if len(val) == 0 {
			return nil
		}
		var out []interface{}
		for _, item := range val {
			if cleaned := cleanFHIRResource(item); cleaned != nil {
				out = append(out, cleaned)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case nil:
		return nil
	default:
		return v
	}
}

// appendToFHIRArray appends value to the named FHIR array field on a resource.
// Creates the array if it does not exist yet.
func appendToFHIRArray(resource map[string]interface{}, path string, value interface{}) {
	existing, _ := resource[path].([]interface{})
	resource[path] = append(existing, value)
}

// isEmptyResolved returns true when a resolved template map is effectively empty
// (all leaf values are empty strings after placeholder substitution).
func isEmptyResolved(m map[string]interface{}) bool {
	for _, v := range m {
		switch tv := v.(type) {
		case string:
			if tv != "" {
				return false
			}
		case map[string]interface{}:
			if !isEmptyResolved(tv) {
				return false
			}
		default:
			if tv != nil {
				return false
			}
		}
	}
	return true
}

// enrichPatientIdentifiers enriches Patient.identifier entries with HL7 Table 0203 type codings.
// It also adds type-coded entries for well-known PID fields that do not carry a type subfield
// in the raw wire format (PID.19=SSN, PID.18=Account Number).
func (s *HL7FHIRTransformServiceV3) enrichPatientIdentifiers(
	r map[string]interface{},
	parsedHL7Data map[string]interface{},
) {
	// --- Step 1: normalize existing identifier entries from the mapping engine ---
	ids, _ := r["identifier"].([]interface{})
	for _, idRaw := range ids {
		idMap, ok := idRaw.(map[string]interface{})
		if !ok {
			continue
		}
		// Get or create identifier.type
		typeObj, _ := idMap["type"].(map[string]interface{})
		if typeObj == nil {
			typeObj = map[string]interface{}{}
		}
		// Normalize identifier.system first — applies regardless of whether type codings exist.
		// If the normalizer can't produce a valid URI (returns ""), drop the key entirely —
		// an absent system is valid FHIR; an empty-string system is not.
		if sys, ok2 := idMap["system"].(string); ok2 && sys != "" {
			if normalized := normalizeIdentifierSystem(sys); normalized == "" {
				delete(idMap, "system")
			} else {
				idMap["system"] = normalized
			}
		}
		codings, _ := typeObj["coding"].([]interface{})
		if len(codings) == 0 {
			continue // no type coding to enrich
		}
		changed := false
		for _, codRaw := range codings {
			cod, ok2 := codRaw.(map[string]interface{})
			if !ok2 {
				continue
			}
			code, _ := cod["code"].(string)
			if code == "" {
				continue
			}
			code = strings.ToUpper(strings.TrimSpace(code))
			if sys, _ := cod["system"].(string); sys == "" {
				cod["system"] = hl7v2IdentifierTypeSystem
				changed = true
			}
			if disp, _ := cod["display"].(string); disp == "" {
				if label, found := table0203[code]; found {
					cod["display"] = label
					changed = true
				}
			}
		}
		if changed {
			typeObj["coding"] = codings
			idMap["type"] = typeObj
		}
	}

	// --- Step 2: annotate identifiers from well-known PID fields using the CX wire format ---
	// Type codes come from CX.5 in the HL7 message itself (Table 0203), not from hardcoded
	// field-to-type mappings.  BuildIdentifierFromCX extracts CX.1 (value), CX.4 (system),
	// and CX.5 (type) — if the sender included CX.5, we get the type for free; if not, no
	// type is injected.  PID.19 (SSN) is handled separately because its system URI is
	// canonical and well-known regardless of whether CX.5 is present.
	pidList := s.extractSegmentGroup(parsedHL7Data, "PID")
	if len(pidList) == 0 {
		return
	}
	pid := pidList[0]

	// Build a set of existing values so we can annotate in-place without duplicating.
	existingValues := map[string]bool{}
	for _, idRaw := range ids {
		if m, ok := idRaw.(map[string]interface{}); ok {
			if v, ok2 := m["value"].(string); ok2 && v != "" {
				existingValues[v] = true
			}
		}
	}

	// CX-bearing PID fields: parse each as a CX composite so CX.5 drives the type.
	for _, pidKey := range []string{"PID.3", "PID.18", "PID.2", "PID.4"} {
		raw := segFieldValue(pid, pidKey)
		if raw == "" {
			continue
		}
		parsed := hl7assembly.BuildIdentifierFromCX(raw, nil)
		val, _ := parsed["value"].(string)
		if val == "" {
			continue
		}
		typeObj, hasType := parsed["type"]
		sysVal, _ := parsed["system"].(string)

		if existingValues[val] {
			// Annotate the existing entry in-place.
			for _, idRaw := range ids {
				idMap, ok2 := idRaw.(map[string]interface{})
				if !ok2 || idMap["value"] != val {
					continue
				}
				if hasType {
					if _, already := idMap["type"]; !already {
						idMap["type"] = typeObj
					}
				}
				if sysVal != "" {
					if _, already := idMap["system"]; !already {
						idMap["system"] = sysVal
					}
				}
				break
			}
		} else {
			// Not yet present — add as new entry.
			ids = append(ids, parsed)
			existingValues[val] = true
		}
	}

	// PID.19 — SSN: system URI is canonical regardless of CX.5 presence.
	if ssnVal := segFieldValue(pid, "PID.19"); ssnVal != "" {
		const ssnSystem = "http://hl7.org/fhir/sid/us-ssn"
		if existingValues[ssnVal] {
			for _, idRaw := range ids {
				if m, ok := idRaw.(map[string]interface{}); ok && m["value"] == ssnVal {
					if _, hasSys := m["system"]; !hasSys {
						m["system"] = ssnSystem
					}
					break
				}
			}
		} else {
			ids = append(ids, map[string]interface{}{"value": ssnVal, "system": ssnSystem})
			existingValues[ssnVal] = true
		}
	}

	r["identifier"] = ids
}

// enrichEncounterParticipants injects Encounter.participant.type role discriminators
// per the HL7 V2-to-FHIR IG.  The PV1 field → v3-ParticipationType mapping is defined
// once in hl7assembly.PV1ParticipantRoles and reused here — no hardcoding in this file.
//
// For each PV1 doctor field that contains a name:
//   - if a participant with matching display already exists → annotate with type in-place
//   - otherwise → add a new participant entry with type + individual.display
func (s *HL7FHIRTransformServiceV3) enrichEncounterParticipants(
	r map[string]interface{},
	parsedHL7Data map[string]interface{},
) {
	pv1List := s.extractSegmentGroup(parsedHL7Data, "PV1")
	if len(pv1List) == 0 {
		return
	}
	pv1 := pv1List[0]

	existing, _ := r["participant"].([]interface{})

	// Build display → participant map for in-place annotation.
	displayToIdx := map[string]int{}
	for i, p := range existing {
		if pm, ok := p.(map[string]interface{}); ok {
			if ind, ok2 := pm["individual"].(map[string]interface{}); ok2 {
				if disp, ok3 := ind["display"].(string); ok3 && disp != "" {
					displayToIdx[disp] = i
				}
			}
		}
	}

	for _, entry := range hl7assembly.PV1ParticipantRoles {
		raw := segFieldValue(pv1, entry.Field)
		if raw == "" {
			continue
		}
		display := formatXCNDisplayName(raw)
		if display == "" {
			continue
		}
		typeCC := []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  hl7assembly.V3ParticipationTypeSystem,
						"code":    entry.Role.Code,
						"display": entry.Role.Display,
					},
				},
			},
		}
		if idx, found := displayToIdx[display]; found {
			pm := existing[idx].(map[string]interface{})
			if _, hasType := pm["type"]; !hasType {
				pm["type"] = typeCC
			}
		} else {
			newEntry := map[string]interface{}{
				"type":       typeCC,
				"individual": map[string]interface{}{"display": display},
			}
			existing = append(existing, newEntry)
			displayToIdx[display] = len(existing) - 1
		}
	}

	if len(existing) > 0 {
		r["participant"] = existing
	}
}

// enrichPatientTelecom builds / normalizes Patient.telecom from PID.13 (home phone),
// PID.14 (work phone), and PID.40 (email) per HL7 v2 XTN format.
// XTN wire format: TelephoneNumber^TelecomUseCode^TelecomEquipType^EmailAddress^...
func (s *HL7FHIRTransformServiceV3) enrichPatientTelecom(
	r map[string]interface{},
	parsedHL7Data map[string]interface{},
) {
	pidList := s.extractSegmentGroup(parsedHL7Data, "PID")
	if len(pidList) == 0 {
		return
	}
	pid := pidList[0]

	// Get existing telecom to normalize and avoid duplicates
	existingTelecom, _ := r["telecom"].([]interface{})
	existingValues := map[string]bool{}
	for _, tc := range existingTelecom {
		if m, ok := tc.(map[string]interface{}); ok {
			if v, ok2 := m["value"].(string); ok2 && v != "" {
				existingValues[v] = true
			}
		}
	}

	// Normalize any entries added by the mapping engine
	for _, tcRaw := range existingTelecom {
		tc, ok := tcRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if sys, _ := tc["system"].(string); sys != "" {
			if mapped, found := table0201[strings.ToUpper(sys)]; found {
				tc["system"] = mapped
			}
		}
		if use, _ := tc["use"].(string); use != "" {
			if mapped, found := table0202[strings.ToUpper(use)]; found {
				tc["use"] = mapped
			}
		}
	}

	// Drop use-code-only shells (no value) left by the mapping engine from fields
	// like PID.13.2 that carry a use code but no phone number.
	var filtered []interface{}
	for _, tcRaw := range existingTelecom {
		if m, ok := tcRaw.(map[string]interface{}); ok {
			if _, hasVal := m["value"]; hasVal {
				filtered = append(filtered, tcRaw)
			}
		}
	}
	existingTelecom = filtered

	// PID.13: home phone, PID.14: work phone, PID.40: email / alternate phone
	type pidTelecom struct {
		pidKey     string
		defaultUse string
		defaultSys string
	}
	fields := []pidTelecom{
		{"PID.13", "home", "phone"},
		{"PID.14", "work", "phone"},
	}

	for _, f := range fields {
		raw := segFieldValue(pid, f.pidKey)
		if raw == "" {
			continue
		}
		// XTN: TelNum^UseCode^EquipType^Email^CountryCode^AreaCode^LocalNumber^...
		parts := strings.Split(raw, "^")
		telNum := strings.TrimSpace(parts[0])
		useCode := ""
		equipType := ""
		emailAddr := ""
		if len(parts) > 1 {
			useCode = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			equipType = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 {
			emailAddr = strings.TrimSpace(parts[3])
		}

		// Determine FHIR system
		fhirSystem := f.defaultSys
		if equipType != "" {
			if mapped, found := table0201[strings.ToUpper(equipType)]; found {
				fhirSystem = mapped
			}
		}

		// Determine FHIR use
		fhirUse := f.defaultUse
		if useCode != "" {
			if mapped, found := table0202[strings.ToUpper(useCode)]; found {
				fhirUse = mapped
			}
		}

		// Email address takes precedence over phone number in this subfield
		value := telNum
		if fhirSystem == "email" && emailAddr != "" {
			value = emailAddr
		} else if emailAddr != "" && telNum == "" {
			value = emailAddr
			fhirSystem = "email"
		}

		if value == "" || existingValues[value] {
			continue
		}

		entry := map[string]interface{}{
			"system": fhirSystem,
			"use":    fhirUse,
			"value":  value,
		}
		existingTelecom = append(existingTelecom, entry)
		existingValues[value] = true
	}

	if len(existingTelecom) > 0 {
		r["telecom"] = existingTelecom
	}
}

// enrichDiagnosticReportIdentifiers adds HL7 Table 0203 type codings to DiagnosticReport.identifier.
// Per HL7 v2 spec: OBR.2 = Placer Order Number, OBR.3 = Filler Order Number.
// The first mapped identifier gets type PLAC; the second gets FILL.
func (s *HL7FHIRTransformServiceV3) enrichDiagnosticReportIdentifiers(
	r map[string]interface{},
) {
	ids, ok := r["identifier"].([]interface{})
	if !ok || len(ids) == 0 {
		return
	}

	typeCodes := []struct {
		code    string
		display string
	}{
		{"PLAC", "Placer Identifier"},
		{"FILL", "Filler Identifier"},
	}

	for i, idRaw := range ids {
		if i >= len(typeCodes) {
			break
		}
		idMap, ok2 := idRaw.(map[string]interface{})
		if !ok2 {
			continue
		}
		// Only add type if not already set
		if _, hasType := idMap["type"]; hasType {
			continue
		}
		idMap["type"] = map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system":  hl7v2IdentifierTypeSystem,
					"code":    typeCodes[i].code,
					"display": typeCodes[i].display,
				},
			},
			"text": typeCodes[i].display,
		}
	}
}

// enrichResultsInterpreter reconstructs DiagnosticReport.resultsInterpreter display text
// from the OBR.32 (Principal Result Interpreter) XCN composite field.
// XCN: IDNum^FamilyName^GivenName^MiddleName^Suffix^Prefix^Degree^SourceTable^AssigningAuthority^...
// Falls back to OBR.16 (Ordering Provider) if OBR.32 is absent.
func (s *HL7FHIRTransformServiceV3) enrichResultsInterpreter(
	r map[string]interface{},
	parsedHL7Data map[string]interface{},
) {
	obrList := s.extractSegmentGroup(parsedHL7Data, "OBR")
	if len(obrList) == 0 {
		return
	}
	obr := obrList[0]

	// OBR.32 first; fall back to OBR.16
	xcnRaw := segFieldValue(obr, "OBR.32")
	if xcnRaw == "" {
		xcnRaw = segFieldValue(obr, "OBR.16")
	}
	if xcnRaw == "" {
		return
	}

	// Parse XCN composite (^ separated)
	parts := strings.Split(xcnRaw, "^")
	idNum := ""
	familyName := ""
	givenName := ""
	prefix := ""
	suffix := ""

	if len(parts) > 0 {
		idNum = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		familyName = strings.TrimSpace(parts[1])
	}
	if len(parts) > 2 {
		givenName = strings.TrimSpace(parts[2])
	}
	if len(parts) > 5 {
		prefix = strings.TrimSpace(parts[5])
	}
	if len(parts) > 4 {
		suffix = strings.TrimSpace(parts[4])
	}

	// Build display name in "Given Family" order (matches FHIR HumanName convention)
	var nameParts []string
	if prefix != "" {
		nameParts = append(nameParts, prefix)
	}
	if givenName != "" {
		nameParts = append(nameParts, givenName)
	}
	if familyName != "" {
		nameParts = append(nameParts, familyName)
	}
	if suffix != "" {
		nameParts = append(nameParts, suffix)
	}
	// Fallback: if no name components, use the ID
	if len(nameParts) == 0 && idNum != "" {
		nameParts = append(nameParts, idNum)
	}
	if len(nameParts) == 0 {
		return
	}
	displayName := strings.Join(nameParts, " ")

	// Update resultsInterpreter — it may be a []interface{} of References
	ri, _ := r["resultsInterpreter"].([]interface{})
	if len(ri) == 0 {
		// Create a new one
		ref := map[string]interface{}{"display": displayName}
		if idNum != "" {
			ref["identifier"] = map[string]interface{}{
				"value": idNum,
				"type": map[string]interface{}{
					"coding": []interface{}{
						map[string]interface{}{
							"system":  hl7v2IdentifierTypeSystem,
							"code":    "PRN",
							"display": "Provider number",
						},
					},
				},
			}
		}
		r["resultsInterpreter"] = []interface{}{ref}
		return
	}

	// Patch existing first entry
	if first, ok := ri[0].(map[string]interface{}); ok {
		existing, _ := first["display"].(string)
		// Only overwrite if existing value is a bare ID (numeric or short) and we have a proper name
		if existing == "" || (len(existing) < 12 && familyName != "") {
			first["display"] = displayName
		}
		if idNum != "" {
			if _, hasID := first["identifier"]; !hasID {
				first["identifier"] = map[string]interface{}{
					"value": idNum,
					"type": map[string]interface{}{
						"coding": []interface{}{
							map[string]interface{}{
								"system":  hl7v2IdentifierTypeSystem,
								"code":    "PRN",
								"display": "Provider number",
							},
						},
					},
				}
			}
		}
	}
}
