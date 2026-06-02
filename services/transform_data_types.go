package services

import (
	"fmt"
	"log"
	"strings"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/services/hl7assembly"
	"ezhealthkonnect/services/hl7type"
)

// =====================================
// DATA TYPE TRANSFORMERS (Complete Implementation)
// =====================================

func (s *HL7FHIRTransformServiceV3) transformValueAtomic(
	hl7Value string,
	mapping FieldMapping,
) (interface{}, error) {

	if hl7Value == "" {
		return nil, nil
	}

	log.Printf("🔧 Transforming '%s' using '%s'", hl7Value, mapping.DataTypeTransform)

	switch mapping.DataTypeTransform {
	case "static_value":
		// Value was injected by createResourceFromAtomicMappings from StaticValue;
		// pass it through unchanged so it is written directly to the FHIR path.
		return hl7Value, nil
	case "msh9_trigger_event_to_coding":
		// ✅ EXPLICIT CASE for trigger event transformation
		result := s.transformTriggerEventToCoding(hl7Value)
		log.Printf("✅ Trigger event transform: '%s' → %+v", hl7Value, result)
		return result, nil
	case "xtn_to_contactpoint", "phone_to_contactpoint":
		cp := s.transformPhoneToContactPointEnhanced(hl7Value, mapping.TransformationRules)
		// When the FHIR target is a scalar text leaf (Annotation.text, note.text, etc.)
		// the receiver expects a plain string — not a ContactPoint object.
		// This mirrors the cx_to_identifier pattern for ".value" paths.
		if strings.HasSuffix(mapping.FHIRElementPath, ".text") ||
			strings.HasSuffix(mapping.FHIRElementPath, ".description") {
			return contactPointToAnnotationText(cp), nil
		}
		return cp, nil
	case "gender_mapping", "administrative_sex", "gender":
		return s.transformGender(hl7Value), nil
	case "cx_to_identifier":
		// If the value contains ~ repetitions, return an array of identifiers.
		if strings.Contains(hl7Value, "~") {
			return s.transformCXToIdentifiers(hl7Value, mapping.TransformationRules), nil
		}
		idMap := s.transformCXToIdentifier(hl7Value, mapping.TransformationRules)
		// When the FHIR path ends in a scalar sub-field (e.g. identifier[1].value),
		// the caller expects a plain string — not a nested Identifier object.
		// Extract the value string to avoid "must be simple value, not an Object" errors.
		if strings.HasSuffix(mapping.FHIRElementPath, ".value") {
			if v, ok := idMap["value"].(string); ok && v != "" {
				return v, nil
			}
			if v, ok := idMap["text"].(string); ok && v != "" {
				return v, nil
			}
		}
		return idMap, nil
	case "xpn_to_humanname":
		return s.transformXPNToHumanName(hl7Value, mapping.TransformationRules), nil
	case "xad_to_address":
		return s.transformXADToAddress(hl7Value, mapping.TransformationRules), nil
	case "email_to_contactpoint":
		return s.transformEmailToContactPoint(hl7Value, mapping.TransformationRules), nil
	case "ts_to_date":
		return s.transformTSToDate(hl7Value), nil
	case "ts_to_datetime":
		return tsToISODateTime(hl7Value), nil
	case "ce_to_codeableconcept":
		// When the FHIR target is a scalar leaf (path ends in .code or .system),
		// return just the code/system string instead of the full CodeableConcept
		// object — placing an object at a scalar leaf causes "must be simple value" errors.
		fhirPath := mapping.FHIRElementPath
		if strings.HasSuffix(fhirPath, ".code") || strings.HasSuffix(fhirPath, ".system") {
			parts := strings.SplitN(hl7Value, "^", 4)
			if strings.HasSuffix(fhirPath, ".system") && len(parts) >= 3 && parts[2] != "" {
				return strings.TrimSpace(parts[2]), nil
			}
			code := strings.TrimSpace(parts[0])
			if code == "" {
				return nil, nil
			}
			return code, nil
		}
		return s.transformCEToCodeableConcept(hl7Value, mapping.TransformationRules), nil
	// ce_code_only: extract the code (first ^-component) as a plain string.
	// Used when the FHIR target expects a simple string (e.g. meta.tag[x].code,
	// identifier.value) and the HL7 source is a CE/CWE field.
	case "ce_code_only":
		parts := strings.SplitN(hl7Value, "^", 2)
		code := strings.TrimSpace(parts[0])
		if code == "" {
			return nil, nil
		}
		return code, nil
	// hl7_timestamp_to_fhir_date / _datetime — aliases for the canonical ts_to_date /
	// ts_to_datetime keys.  type_registry.go emits these names; both must resolve.
	case "hl7_timestamp_to_fhir_date":
		return s.transformTSToDate(hl7Value), nil
	case "hl7_timestamp_to_fhir_datetime", "hl7datetime", "datetime":
		return tsToISODateTime(hl7Value), nil
	case "hl7_timestamp_to_fhir_date_only", "hl7date":
		return s.transformTSToDate(hl7Value), nil
	// hl7_active_flag: HL7 table 0183 (A/I) → FHIR boolean.
	// Also accepts common truthy variants used by some senders.
	case "hl7_active_flag":
		switch strings.ToUpper(strings.TrimSpace(hl7Value)) {
		case "A", "Y", "1", "TRUE", "ACTIVE":
			return true, nil
		default:
			return false, nil
		}
	// mfi_response_level: HL7 table 0179 → FHIR MessageHeader.response.code.
	// Valid FHIR ResponseType values: ok | transient-error | fatal-error.
	case "mfi_response_level":
		switch strings.ToUpper(strings.TrimSpace(hl7Value)) {
		case "AL", "SU":
			return "ok", nil
		case "ER":
			return "transient-error", nil
		case "NE":
			// NE = No Acknowledgement expected — response block should be omitted, not set.
			return nil, nil
		default:
			return "ok", nil
		}
	case "ssn_to_identifier":
		return s.transformSSNToIdentifier(hl7Value, mapping.TransformationRules), nil
	case "account_to_identifier":
		return s.transformAccountToIdentifier(hl7Value, mapping.TransformationRules), nil
	case "msh9_to_coding":
		return s.transformMSH9ToCoding(hl7Value), nil
	case "control_id_to_reference":
		return s.transformControlIdToReference(hl7Value), nil
	case "obr_status_to_dr_status":
		return s.transformOBRStatusToDRStatus(hl7Value), nil
	case "obx_status_to_obs_status":
		return s.transformOBXStatusToObsStatus(hl7Value), nil
	case "abnormal_flag_to_interpretation":
		return s.transformAbnormalFlagToInterpretation(hl7Value), nil
	case "obx_value_by_type":
		return s.transformOBXValueByType(hl7Value, mapping.TransformationRules), nil
	case "siu_appointment_status":
		return s.transformSIUAppointmentStatus(hl7Value), nil
	case "xcn_to_reference":
		// XCN composite: id^family^given^middle^suffix^prefix^degree^^^...
		// Build a plain display string from name components; never return raw "id^family^given"
		// which violates the FHIR Reference.display spec.
		display := formatXCNDisplayName(hl7Value)
		if display == "" {
			return nil, nil
		}
		return display, nil
	case "":
		// Auto-translate using HL7 and FHIR data types when available
		if mapping.HL7DataType != "" || mapping.FHIRDataType != "" {
			result := hl7type.AutoTranslate(hl7Value, mapping.HL7DataType, mapping.FHIRDataType, mapping.TransformationRules)
			if len(result.Warnings) > 0 {
				log.Printf("⚠️ AutoTranslate warnings for %s.%s: %v", mapping.SegmentName, mapping.HL7Field, result.Warnings)
			}
			if result.Value != nil {
				log.Printf("🔍 AutoTranslate %s.%s (hl7=%s,fhir=%s): %q → %v",
					mapping.SegmentName, mapping.HL7Field, mapping.HL7DataType, mapping.FHIRDataType, hl7Value, result.Value)
				return result.Value, nil
			}
		}
		log.Printf("🔍 No transformation specified, using raw value")
		return stripHL7Caret(hl7Value, mapping.FHIRDataType), nil
	default:
		// Handle lookup:<valueSetName> transforms inlined from profile v2.0
		if strings.HasPrefix(mapping.DataTypeTransform, "lookup:") {
			vsName := strings.TrimPrefix(mapping.DataTypeTransform, "lookup:")
			if vsRaw, ok := mapping.TransformationRules["valueSet"]; ok {
				if vsMap, ok := vsRaw.(map[string]interface{}); ok {
					key := strings.ToUpper(strings.TrimSpace(hl7Value))
					if mapped, found := vsMap[key]; found {
						return mapped, nil
					}
					// Fallback: try original case
					if mapped, found := vsMap[hl7Value]; found {
						return mapped, nil
					}
					log.Printf("⚠️ lookup:%s — key %q not found in value set, using raw value", vsName, hl7Value)
				}
			} else {
				log.Printf("⚠️ lookup:%s — no valueSet found in TransformationRules", vsName)
			}
			return hl7Value, nil
		}
		// hl7_table_XXXX_* transforms: extract table number and look up in
		// hl7_fhir_value_mappings (V115 IG seed data).  DB is the authoritative
		// source; inline valueMap from the template is the fallback.
		if strings.HasPrefix(mapping.DataTypeTransform, "hl7_table_") {
			parts := strings.SplitN(mapping.DataTypeTransform, "_", 4)
			if len(parts) >= 3 && s.valueMapper != nil {
				tableNum := parts[2]
				if _, fhirCode, _, found := s.valueMapper.MapValue(tableNum, hl7Value); found {
					return fhirCode, nil
				}
			}
		}

		// Inline valueMap from template JSON — fallback when DB has no entry.
		if vmRaw, ok := mapping.TransformationRules["valueMap"]; ok {
			if vmMap, ok := vmRaw.(map[string]interface{}); ok {
				key := strings.ToUpper(strings.TrimSpace(hl7Value))
				if mapped, found := vmMap[key]; found {
					return mapped, nil
				}
				if mapped, found := vmMap[hl7Value]; found {
					return mapped, nil
				}
			}
		}

		log.Printf("⚠️ Unknown transformation '%s', using raw value", mapping.DataTypeTransform)
		return stripHL7Caret(hl7Value, mapping.FHIRDataType), nil
	}
}

// stripHL7Caret removes HL7 component separators ('^') from a string before
// it is written to a FHIR primitive field.  When a composite HL7 value arrives
// without a component-level mapping we keep only the first component (the
// primary value) so the caret never appears in FHIR output.
//
// Only applied to FHIR primitive types — complex types (CodeableConcept,
// HumanName, Address, …) are handled by their own transforms and must not
// be stripped here.
func stripHL7Caret(value, fhirDataType string) string {
	if !strings.Contains(value, "^") {
		return value
	}
	switch strings.ToLower(strings.TrimSpace(fhirDataType)) {
	case "string", "id", "code", "uri", "url", "canonical", "markdown",
		"positiveint", "unsignedint", "integer", "decimal",
		"boolean", "date", "datetime", "instant", "time", "":
		return strings.TrimSpace(strings.SplitN(value, "^", 2)[0])
	}
	return value
}

// tsToISODateTime converts a raw HL7 timestamp string to ISO 8601 / FHIR dateTime format.
func tsToISODateTime(ts string) string {
	ts = strings.TrimSpace(ts)
	// Strip timezone offset if present (e.g. "+0500")
	if plus := strings.IndexByte(ts, '+'); plus > 8 {
		ts = ts[:plus]
	} else if minus := strings.LastIndexByte(ts, '-'); minus > 8 {
		ts = ts[:minus]
	}
	if len(ts) < 8 {
		return ts
	}
	year, month, day := ts[0:4], ts[4:6], ts[6:8]
	if len(ts) >= 12 {
		hour, min := ts[8:10], ts[10:12]
		sec := "00"
		if len(ts) >= 14 {
			sec = ts[12:14]
		}
		return fmt.Sprintf("%s-%s-%sT%s:%s:%s+00:00", year, month, day, hour, min, sec)
	}
	return fmt.Sprintf("%s-%s-%s", year, month, day)
}

func (s *HL7FHIRTransformServiceV3) transformGender(gender string) string {
	upperGender := strings.ToUpper(strings.TrimSpace(gender))

	// ✅ DATABASE-DRIVEN: Look up gender mapping from PostgreSQL
	if s.db != nil {
		query := `SELECT fhir_code FROM value_set_mappings
				 WHERE mapping_name = 'administrative_sex' AND hl7_value = $1 LIMIT 1`

		var fhirCode string
		if err := s.db.QueryRow(query, upperGender).Scan(&fhirCode); err == nil {
			log.Printf("✅ Gender mapping from DB: %s → %s", gender, fhirCode)
			return fhirCode
		}
	}

	// Hardcoded fallback — covers HL7 Table 0001 (Administrative Sex) codes.
	// These are deterministic FHIR AdministrativeGender values; no DB row needed.
	log.Printf("⚠️ No gender mapping found for '%s' in database, applying HL7 Table 0001 fallback", gender)
	switch upperGender {
	case "M":
		return "male"
	case "F":
		return "female"
	case "O":
		return "other"
	case "U", "UN", "UNK", "UNKNOWN":
		return "unknown"
	case "A", "N": // Ambiguous / Not applicable — closest FHIR code
		return "other"
	default:
		return "unknown"
	}
}

func (s *HL7FHIRTransformServiceV3) transformPhoneToContactPointEnhanced(phone string, rules map[string]interface{}) interface{} {
	if phone == "" {
		return nil
	}

	// Handle composite XTN fields like "(407)939-1289^^^theMainMouse@disney.com"
	if strings.Contains(phone, "^") {
		return s.processXTNField(phone, rules)
	}

	return s.createSimpleContactPoint(phone, rules)
}

func (s *HL7FHIRTransformServiceV3) processXTNField(phone string, rules map[string]interface{}) interface{} {
	var contactPoints []map[string]interface{}

	// Use HL7 parser for proper composite field handling
	enhancedFieldData := hl7.ParseHL7Field(phone)
	if enhancedFieldData == nil || len(enhancedFieldData.Repetitions) == 0 {
		// Fallback: simple split parsing
		components := strings.Split(phone, "^")
		for i, component := range components {
			component = strings.TrimSpace(component)
			if component == "" {
				continue
			}

			contactPoint := s.createContactPointFromComponent(component, i+1, rules)
			if contactPoint != nil {
				contactPoints = append(contactPoints, contactPoint)
			}
		}
	} else {
		// Enhanced parsing
		for _, repetition := range enhancedFieldData.Repetitions {
			for compIndex, component := range repetition.Components {
				value := strings.TrimSpace(component.RawValue)
				if value == "" {
					continue
				}

				contactPoint := s.createContactPointFromComponent(value, compIndex+1, rules)
				if contactPoint != nil {
					contactPoints = append(contactPoints, contactPoint)
				}
			}
		}
	}

	if len(contactPoints) == 1 {
		return contactPoints[0]
	}
	return contactPoints
}

func (s *HL7FHIRTransformServiceV3) createContactPointFromComponent(value string, componentNum int, rules map[string]interface{}) map[string]interface{} {
	var contactPoint map[string]interface{}

	if strings.Contains(value, "@") {
		// Email
		contactPoint = map[string]interface{}{
			"system": "email",
			"use":    "home", // Default, can be overridden by rules
			"value":  value,
		}
	} else {
		// Phone
		contactPoint = map[string]interface{}{
			"system": "phone",
			"use":    "home", // Default, can be overridden by rules or database mapping
			"value":  value,
		}

		// ✅ DATABASE-DRIVEN: Look up phone use mapping based on component position
		if s.db != nil {
			phoneUse := s.lookupPhoneUseFromDatabase(componentNum)
			if phoneUse != "" {
				contactPoint["use"] = phoneUse
			}
		}
	}

	// Apply transformation rules if provided (these come from database)
	if rules != nil {
		if system, ok := rules["system"].(string); ok {
			contactPoint["system"] = system
		}
		if use, ok := rules["use"].(string); ok {
			contactPoint["use"] = use
		}
	}

	return contactPoint
}

// ✅ DATABASE-DRIVEN: Look up phone use based on HL7 XTN component position
func (s *HL7FHIRTransformServiceV3) lookupPhoneUseFromDatabase(componentNum int) string {
	query := `
		SELECT fhir_code
		FROM value_set_mappings
		WHERE mapping_name = 'xtn_component_use' AND hl7_value = $1
		LIMIT 1
	`

	var use string
	err := s.db.QueryRow(query, fmt.Sprintf("%d", componentNum)).Scan(&use)
	if err != nil {
		log.Printf("🔍 No XTN component use mapping found for component %d", componentNum)
		return ""
	}

	log.Printf("✅ XTN component mapping: component %d → use '%s'", componentNum, use)
	return use
}

func (s *HL7FHIRTransformServiceV3) createSimpleContactPoint(phone string, rules map[string]interface{}) map[string]interface{} {
	contactPoint := map[string]interface{}{
		"system": "phone",
		"use":    "home",
		"value":  phone,
	}

	if rules != nil {
		if system, ok := rules["system"].(string); ok {
			contactPoint["system"] = system
		}
		if use, ok := rules["use"].(string); ok {
			contactPoint["use"] = use
		}
	}

	return contactPoint
}

// contactPointToAnnotationText serialises a ContactPoint (or slice thereof) to a
// human-readable string suitable for scalar FHIR text fields (Annotation.text).
// Follows the value-extraction-over-object rule: callers that need the full
// ContactPoint structure should use the xtn_to_contactpoint transform against a
// ContactPoint-typed FHIR path, not against a scalar leaf.
func contactPointToAnnotationText(cp interface{}) string {
	extractValue := func(m map[string]interface{}) string {
		if v, ok := m["value"].(string); ok && v != "" {
			return v
		}
		return ""
	}

	switch v := cp.(type) {
	case map[string]interface{}:
		return extractValue(v)
	case []map[string]interface{}:
		var parts []string
		for _, m := range v {
			if s := extractValue(m); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "; ")
	case []interface{}:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if s := extractValue(m); s != "" {
					parts = append(parts, s)
				}
			}
		}
		return strings.Join(parts, "; ")
	}
	return fmt.Sprintf("%v", cp)
}

// transformCXToIdentifier converts one CX repetition to a FHIR Identifier map.
// Delegates to the canonical hl7assembly.BuildIdentifierFromCX which correctly maps:
//
//	CX.1 → value, CX.4 (Assigning Authority HD) → system, CX.5 (Table 0203 type) → type
func (s *HL7FHIRTransformServiceV3) transformCXToIdentifier(cx string, rules map[string]interface{}) map[string]interface{} {
	return hl7assembly.BuildIdentifierFromCX(cx, rules)
}

// transformCXToIdentifiers handles a PID.3 value that may contain multiple CX
// repetitions separated by '~', returning a slice of FHIR Identifier maps.
func (s *HL7FHIRTransformServiceV3) transformCXToIdentifiers(cx string, rules map[string]interface{}) []interface{} {
	return hl7assembly.BuildIdentifiersFromCX(cx, rules)
}

func (s *HL7FHIRTransformServiceV3) transformXPNToHumanName(xpn string, rules map[string]interface{}) map[string]interface{} {
	return hl7assembly.BuildHumanNameFromXPN(xpn, rules)
}

func (s *HL7FHIRTransformServiceV3) transformXADToAddress(xad string, rules map[string]interface{}) map[string]interface{} {
	return hl7assembly.BuildAddressFromXAD(xad, rules)
}

func (s *HL7FHIRTransformServiceV3) transformEmailToContactPoint(email string, rules map[string]interface{}) map[string]interface{} {
	return hl7assembly.BuildContactPointFromXTN(email, rules)
}

// transformTSToDate converts an HL7 TS value to a FHIR date (YYYY-MM-DD).
// Used for fields typed as FHIR date (e.g. Patient.birthDate).
func (s *HL7FHIRTransformServiceV3) transformTSToDate(ts string) string {
	result, warnings := hl7type.ParseTS(ts, "date")
	if len(warnings) > 0 {
		log.Printf("⚠️ transformTSToDate warnings for '%s': %v", ts, warnings)
	}
	if result != "" {
		return result
	}
	// Fallback: basic YYYYMMDD → YYYY-MM-DD
	if len(ts) >= 8 {
		return fmt.Sprintf("%s-%s-%s", ts[0:4], ts[4:6], ts[6:8])
	}
	return ts
}

// transformTSToDateTime converts an HL7 TS value to a FHIR dateTime (ISO 8601).
// Used for fields typed as FHIR dateTime or instant (e.g. Observation.effectiveDateTime,
// DiagnosticReport.issued, MessageHeader.meta.lastUpdated).
func (s *HL7FHIRTransformServiceV3) transformTSToDateTime(ts string) string {
	result, warnings := hl7type.ParseTS(ts, "dateTime")
	if len(warnings) > 0 {
		log.Printf("⚠️ transformTSToDateTime warnings for '%s': %v", ts, warnings)
	}
	if result != "" {
		return result
	}
	// Fallback: YYYYMMDDHHMMSS → YYYY-MM-DDTHH:MM:SS+00:00
	ts = strings.TrimSpace(ts)
	if len(ts) >= 14 {
		return fmt.Sprintf("%s-%s-%sT%s:%s:%s+00:00", ts[0:4], ts[4:6], ts[6:8], ts[8:10], ts[10:12], ts[12:14])
	}
	if len(ts) >= 12 {
		return fmt.Sprintf("%s-%s-%sT%s:%s:00+00:00", ts[0:4], ts[4:6], ts[6:8], ts[8:10], ts[10:12])
	}
	if len(ts) >= 8 {
		return fmt.Sprintf("%s-%s-%sT00:00:00+00:00", ts[0:4], ts[4:6], ts[6:8])
	}
	return ts
}

// normalizeAddressUse maps HL7 v2 table 0190 address type codes to FHIR AddressUse.
// mapEncounterStatus converts HL7 ADT trigger event codes to FHIR encounter-status values
// per the HL7 V2-to-FHIR Implementation Guide (https://build.fhir.org/ig/HL7/v2-to-fhir/).
func mapEncounterStatus(hl7Status string) string {
	s := strings.ToUpper(strings.TrimSpace(hl7Status))
	switch s {
	// Admit / register / outpatient visit — encounter is active.
	case "A01", // Admit/visit notification
		"A02",  // Transfer
		"A04",  // Register outpatient
		"A06",  // Change outpatient → inpatient
		"A07",  // Change inpatient → outpatient
		"A08",  // Update patient information
		"A09",  // Patient departing (tracking)
		"A10",  // Patient arriving (tracking)
		"A12",  // Cancel transfer (patient still admitted)
		"A13",  // Cancel discharge (patient still admitted)
		"A15",  // Pending transfer
		"A16",  // Pending discharge
		"A17",  // Swap patients
		"A22",  // Patient returns from leave of absence
		"A25",  // Cancel pending discharge
		"A26",  // Cancel pending transfer
		"A40",  // Merge patient information
		"A41",  // Merge account information
		"A45",  // Move visit information
		"A47":  // Change patient identifier
		return "in-progress"
	// Discharge / end of visit.
	case "A03":
		return "finished"
	// Pre-admit / pending admit — encounter not yet started.
	case "A05", // Pre-admit
		"A14": // Pending admit notification
		return "planned"
	// Patient on leave of absence — encounter begun but patient temporarily away.
	case "A21":
		return "onleave"
	// Cancel events that undo the encounter entirely.
	case "A11", // Cancel admit
		"A27",  // Cancel pending admit
		"A38":  // Cancel pre-admit
		return "cancelled"
	// Person/account administration events — not encounter-specific.
	case "A28", // Add person information
		"A31": // Update person information
		return "unknown"
	default:
		validStatuses := map[string]bool{
			"planned": true, "arrived": true, "triaged": true, "in-progress": true,
			"onleave": true, "finished": true, "cancelled": true,
			"entered-in-error": true, "unknown": true,
		}
		lower := strings.ToLower(hl7Status)
		if validStatuses[lower] {
			return lower
		}
		return "unknown"
	}
}

// normalizePatientClassToCoding maps a PV1.2 patient class code string to a FHIR Coding object.
func normalizePatientClassToCoding(code string) map[string]interface{} {
	c := strings.ToUpper(strings.TrimSpace(code))
	display := ""
	fhirCode := c
	switch c {
	case "I", "IMP":
		fhirCode = "IMP"
		display = "inpatient encounter"
	case "O", "AMB":
		fhirCode = "AMB"
		display = "ambulatory"
	case "E", "EMER":
		fhirCode = "EMER"
		display = "emergency"
	case "P", "PRENC":
		fhirCode = "PRENC"
		display = "pre-admission"
	case "N", "NEWB":
		fhirCode = "NEWB"
		display = "newborn"
	case "R", "SS":
		fhirCode = "SS"
		display = "short stay"
	case "B":
		fhirCode = "IMP"
		display = "inpatient encounter"
	case "C", "VHH":
		fhirCode = "VHH"
		display = "home health"
	default:
		fhirCode = "IMP"
		display = "inpatient encounter"
	}
	coding := map[string]interface{}{
		"system": "http://terminology.hl7.org/CodeSystem/v3-ActCode",
		"code":   fhirCode,
	}
	if display != "" {
		coding["display"] = display
	}
	return coding
}

// flattenCodingCodeObjects walks resource[field].coding[].code and replaces any
// CodeableConcept object found there with the extracted code string.
func flattenCodingCodeObjects(r map[string]interface{}, field string) {
	if fieldVal, ok := r[field].(map[string]interface{}); ok {
		if codings, ok2 := fieldVal["coding"].([]interface{}); ok2 {
			for _, codRaw := range codings {
				if cod, ok3 := codRaw.(map[string]interface{}); ok3 {
					if codeMap, ok4 := cod["code"].(map[string]interface{}); ok4 {
						if extracted := extractCodeFromCodeableConcept(codeMap); extracted != "" {
							cod["code"] = extracted
						} else {
							delete(cod, "code")
						}
					}
				}
			}
		}
	}
}

// extractCodeFromCodeableConcept pulls a code string out of a nested CodeableConcept map.
// Used when ce_to_codeableconcept was applied to a field whose FHIR path ends at a scalar .code leaf.
func extractCodeFromCodeableConcept(m map[string]interface{}) string {
	if codings, ok := m["coding"].([]interface{}); ok && len(codings) > 0 {
		if cod, ok2 := codings[0].(map[string]interface{}); ok2 {
			if code, ok3 := cod["code"].(string); ok3 && code != "" {
				return code
			}
		}
	}
	if txt, ok := m["text"].(string); ok && txt != "" {
		return txt
	}
	return ""
}

// reparsePractitionerIdentifiers fixes identifiers on Practitioner and PractitionerRole
// resources that still hold raw CX composite strings (e.g. "999^12345^1^PROVIDER^1") in
// their .value field. It re-parses each such value via BuildIdentifierFromCX so the
// resulting identifier carries proper system, type, and a scalar value.
func reparsePractitionerIdentifiers(r map[string]interface{}) {
	ids, ok := r["identifier"].([]interface{})
	if !ok {
		return
	}
	for _, idRaw := range ids {
		id, ok2 := idRaw.(map[string]interface{})
		if !ok2 {
			continue
		}
		valStr, ok3 := id["value"].(string)
		if !ok3 || !strings.Contains(valStr, "^") {
			continue
		}
		// Looks like an unparsed CX composite — re-parse it
		parsed := hl7assembly.BuildIdentifierFromCX(valStr, nil)
		if v, ok4 := parsed["value"].(string); ok4 && v != "" {
			id["value"] = v
		}
		if sys, ok4 := parsed["system"].(string); ok4 && sys != "" {
			if _, already := id["system"]; !already {
				id["system"] = sys
			}
		}
		if typ, ok4 := parsed["type"]; ok4 {
			if _, already := id["type"]; !already {
				id["type"] = typ
			}
		}
	}
}

func normalizeAddressUse(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "H", "HOME":
		return "home"
	case "O", "B", "WORK", "WP", "OFFICE":
		return "work"
	case "C", "TMP", "TEMP", "TEMPORARY":
		return "temp"
	case "OLD", "BA", "BDL", "F", "P":
		return "old"
	default:
		return strings.ToLower(strings.TrimSpace(raw)) // pass through lowercased
	}
}

// normalizeNameUse maps HL7 v2 table 0200 name type codes to FHIR NameUse.
func normalizeNameUse(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "L", "LEGAL":
		return "official"
	case "D", "U", "USUAL", "PREFERRED":
		return "usual"
	case "A", "ALIAS":
		return "usual"
	case "N", "NICKNAME", "NICK":
		return "nickname"
	case "M", "MAIDEN", "NAME AT BIRTH":
		return "maiden"
	case "TEMP", "TEMPORARY", "T":
		return "temp"
	case "S", "P", "ANON", "ANONYMOUS":
		return "anonymous"
	case "OLD", "PREVIOUS", "PRIOR":
		return "old"
	default:
		return "usual" // safest default for unrecognised codes
	}
}

// normalizeContactPointUse maps HL7 v2 table 0201 telecommunication use codes to FHIR ContactPointUse.
func normalizeContactPointUse(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "H", "HOME", "PRN", "ORN":
		return "home"
	case "O", "WPN", "WORK", "W", "B":
		return "work"
	case "T", "TMP", "TEMP":
		return "temp"
	case "OLD", "BAD":
		return "old"
	case "M", "MC", "MOBILE", "CELL", "CP":
		return "mobile"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

// normalizeContactPointSystem maps HL7 v2 table 0202 telecommunication equipment type to FHIR ContactPointSystem.
func normalizeContactPointSystem(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "PH", "PHONE", "TELEPHONE":
		return "phone"
	case "FX", "FAX":
		return "fax"
	case "EMAIL", "NET", "INTERNET", "X.400":
		return "email"
	case "BP", "BEEPER", "PAGER":
		return "pager"
	case "URL", "HTTP", "HTTPS":
		return "url"
	case "TDD", "TTY":
		return "other"
	default:
		lower := strings.ToLower(strings.TrimSpace(raw))
		// If already a valid FHIR ContactPointSystem value, keep it
		valid := map[string]bool{"phone": true, "fax": true, "email": true, "pager": true, "url": true, "sms": true, "other": true}
		if valid[lower] {
			return lower
		}
		return "phone" // most common default for unrecognised values
	}
}

// isAbsoluteIdentifierURI returns true when s looks like an absolute URI
// (http://, https://, urn:, or an OID urn:oid:…).
// Free-text values like "HOSPITAL" or "MRN" are NOT valid FHIR system URIs.
func isAbsoluteIdentifierURI(s string) bool {
	sl := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(sl, "http://") ||
		strings.HasPrefix(sl, "https://") ||
		strings.HasPrefix(sl, "urn:")
}

// hdToURN converts an HL7 HD (Hierarchical Designator) value to a urn:oid: URI
// when the HD carries an ISO OID (HD.3 = "ISO").
// Input formats:
//   - "MIE&1.2.840.114398.1.100&ISO"  → "urn:oid:1.2.840.114398.1.100"
//   - "1.2.840.114398.1.100"           → "urn:oid:1.2.840.114398.1.100" (bare OID)
//
// Returns "" for non-OID values (free-text namespace IDs, etc.).
func hdToURN(hd string) string {
	parts := strings.Split(hd, "&")
	// Three-part HD: Name&OID&Type
	if len(parts) == 3 && strings.ToUpper(strings.TrimSpace(parts[2])) == "ISO" {
		oid := strings.TrimSpace(parts[1])
		if isOID(oid) {
			return "urn:oid:" + oid
		}
	}
	// Single-part bare OID (e.g. from a direct PID.3.4 mapping with no & separators)
	trimmed := strings.TrimSpace(hd)
	if isOID(trimmed) {
		return "urn:oid:" + trimmed
	}
	return ""
}

// isOID returns true for dot-separated numeric OID strings like "1.2.840.114398.1.100".
func isOID(s string) bool {
	if s == "" {
		return false
	}
	for _, seg := range strings.Split(s, ".") {
		if seg == "" {
			return false
		}
		for _, c := range seg {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return strings.Contains(s, ".")
}

// normalizeHL7System converts HL7 v2 coding system abbreviations to canonical FHIR URIs.
// The table covers the abbreviations defined in HL7 v2 Table 0396 (Coding System).
// Any unknown value is returned unchanged so local/proprietary systems pass through.
func normalizeHL7System(system string) string {
	switch strings.ToUpper(strings.TrimSpace(system)) {
	// LOINC
	case "LN", "LOINC":
		return "http://loinc.org"
	// SNOMED CT
	case "SCT", "SNM", "SNOMED", "SNOMEDCT", "SNOMED-CT":
		return "http://snomed.info/sct"
	// UCUM (units of measure) — used in coded units
	case "UCUM":
		return "http://unitsofmeasure.org"
	// RxNorm
	case "RXNORM", "RXN":
		return "http://www.nlm.nih.gov/research/umls/rxnorm"
	// ICD-10-CM
	case "I10", "ICD10", "ICD-10", "ICD-10-CM":
		return "http://hl7.org/fhir/sid/icd-10-cm"
	// ICD-9-CM
	case "I9", "ICD9", "ICD-9", "ICD-9-CM":
		return "http://hl7.org/fhir/sid/icd-9-cm"
	// ICD-10 procedure codes
	case "I10P", "ICD-10-PCS":
		return "http://www.cms.gov/Medicare/Coding/ICD10"
	// CPT
	case "C4", "CPT", "CPT4", "CPT-4":
		return "http://www.ama-assn.org/go/cpt"
	// HCPCS
	case "HCPCS":
		return "http://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets"
	// NDC (National Drug Code)
	case "NDC":
		return "http://hl7.org/fhir/sid/ndc"
	// NCI Thesaurus
	case "NCI":
		return "http://ncithesaurus.nci.nih.gov"
	// HL7 v2 tables (commonly used for gender, admin sex, etc.)
	case "HL70001":
		return "http://terminology.hl7.org/CodeSystem/v2-0001"
	case "HL70002":
		return "http://terminology.hl7.org/CodeSystem/v2-0002"
	case "HL70003":
		return "http://terminology.hl7.org/CodeSystem/v2-0003"
	case "HL70061":
		return "http://terminology.hl7.org/CodeSystem/v2-0061"
	case "HL70078":
		return "http://terminology.hl7.org/CodeSystem/v2-0078"
	case "HL70080":
		return "http://terminology.hl7.org/CodeSystem/v2-0080"
	case "HL70085":
		return "http://terminology.hl7.org/CodeSystem/v2-0085"
	case "HL70123":
		return "http://terminology.hl7.org/CodeSystem/v2-0123"
	case "HL70125":
		return "http://terminology.hl7.org/CodeSystem/v2-0125"
	case "HL70203":
		return "http://terminology.hl7.org/CodeSystem/v2-0203"
	// CVX (vaccine codes)
	case "CVX":
		return "http://hl7.org/fhir/sid/cvx"
	// MVX (manufacturer codes)
	case "MVX":
		return "http://hl7.org/fhir/sid/mvx"
	// NPI
	case "NPI":
		return "http://hl7.org/fhir/sid/us-npi"
	// Local / proprietary — not a valid FHIR system URI; callers must drop system.
	case "LOCAL", "L", "99ZZZ":
		return ""
	default:
		// Bare numeric strings (e.g. "1", "99") are HL7 table numbers, not absolute URIs.
		// Treat as local and omit so the Coding is not rejected by FHIR validators.
		allDigits := true
		for _, c := range strings.TrimSpace(system) {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits && len(strings.TrimSpace(system)) > 0 {
			return ""
		}
		return system
	}
}

// wellKnownHL7Systems mirrors the set in hl7assembly. For external code systems
// (LOINC, SNOMED, etc.) we suppress the source display name because HL7 senders
// routinely use unofficial abbreviations that fail FHIR "Wrong Display Name" checks.
var wellKnownHL7Systems = map[string]bool{
	"http://loinc.org":                            true,
	"http://snomed.info/sct":                      true,
	"http://www.ama-assn.org/go/cpt":              true,
	"http://www.nlm.nih.gov/research/umls/rxnorm": true,
	"http://hl7.org/fhir/sid/icd-10-cm":           true,
	"http://hl7.org/fhir/sid/icd-9-cm":            true,
	"http://hl7.org/fhir/sid/ndc":                 true,
	"http://hl7.org/fhir/sid/cvx":                 true,
	"http://hl7.org/fhir/sid/mvx":                 true,
}

func (s *HL7FHIRTransformServiceV3) transformCEToCodeableConcept(ce string, rules map[string]interface{}) map[string]interface{} {
	components := strings.Split(ce, "^")
	codeableConcept := map[string]interface{}{
		"coding": []map[string]interface{}{},
	}

	codings := []map[string]interface{}{}

	// Primary coding: components[0]=code, [1]=display, [2]=system
	if len(components) > 0 && components[0] != "" {
		coding := map[string]interface{}{
			"code": components[0],
		}
		// Resolve system first so we can decide whether to carry display.
		var resolvedSystem string
		if len(components) > 2 && components[2] != "" {
			resolvedSystem = normalizeHL7System(components[2])
			if resolvedSystem != "" {
				coding["system"] = resolvedSystem
			}
			// resolvedSystem=="" means local-only (L, LOCAL, 99ZZZ) — omit system.
		}
		// rules["system"] override (explicit mapping config takes precedence)
		if rules != nil {
			if system, ok := rules["system"].(string); ok && system != "" {
				coding["system"] = system
				resolvedSystem = system
			}
		}
		// hl7Table fallback: when CE.3 is absent and the template annotates the field with an
		// unambiguous HL7 table number, look up the FHIR system (and canonical code/display) from
		// the value mapper.  This covers cases like PID.16 (Table 0002 marital status) where the
		// source message carries only a bare code with no coding system component.
		if resolvedSystem == "" && rules != nil {
			if tableNum, ok := rules["hl7Table"].(string); ok && tableNum != "" && s.valueMapper != nil {
				if fhirSystem, fhirCode, fhirDisplay, found := s.valueMapper.MapValue(tableNum, components[0]); found {
					coding["system"] = fhirSystem
					resolvedSystem = fhirSystem
					// Prefer the canonical code from the DB mapping over the raw HL7 value
					if fhirCode != "" {
						coding["code"] = fhirCode
					}
					// Use canonical display text when there is no CE.1 display component
					if fhirDisplay != "" {
						if len(components) <= 1 || components[1] == "" {
							coding["display"] = fhirDisplay
						}
						if _, hasText := codeableConcept["text"]; !hasText {
							codeableConcept["text"] = fhirDisplay
						}
					}
				}
			}
		}
		// Carry display only when it will NOT cause "Wrong Display Name" failures.
		// For well-known external systems (LOINC, SNOMED, …) the source display is an
		// unofficial abbreviation; omit it and let terminology servers provide the canonical text.
		if len(components) > 1 && components[1] != "" {
			srcDisplay := components[1]
			if !wellKnownHL7Systems[resolvedSystem] {
				coding["display"] = srcDisplay
			}
			// Always propagate to cc.text (human-readable, not validated by FHIR)
			codeableConcept["text"] = srcDisplay
		}
		codings = append(codings, coding)
	}

	// CWE alternate coding: components[3]=altCode, [4]=altDisplay, [5]=altSystem
	if len(components) > 3 && components[3] != "" {
		altCoding := map[string]interface{}{
			"code": components[3],
		}
		var altSystem string
		if len(components) > 5 && components[5] != "" {
			altSystem = normalizeHL7System(components[5])
			if altSystem != "" {
				altCoding["system"] = altSystem
			}
		}
		if len(components) > 4 && components[4] != "" && !wellKnownHL7Systems[altSystem] {
			altCoding["display"] = components[4]
		}
		codings = append(codings, altCoding)
	}

	codeableConcept["coding"] = codings
	return codeableConcept
}

// transformOBRStatusToDRStatus maps OBR.25 result status to FHIR DiagnosticReport.status
func (s *HL7FHIRTransformServiceV3) transformOBRStatusToDRStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "F": // Final
		return "final"
	case "P": // Preliminary
		return "preliminary"
	case "C": // Correction
		return "corrected"
	case "X": // Cannot obtain
		return "cancelled"
	case "I": // In process
		return "partial"
	case "R": // Results entered, not verified
		return "registered"
	case "S": // Status change only
		return "amended"
	default:
		return "unknown"
	}
}

// transformOBXStatusToObsStatus maps OBX.11 observation result status to FHIR Observation.status
func (s *HL7FHIRTransformServiceV3) transformOBXStatusToObsStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "F": // Final results
		return "final"
	case "P": // Preliminary results
		return "preliminary"
	case "C": // Record coming over is a correction — previous results changed
		return "amended"
	case "X": // Results cannot be obtained for this observation
		return "cancelled"
	case "I": // Specimen in lab — results pending
		return "registered"
	case "R": // Results entered, not verified
		return "preliminary"
	case "W": // Post original as wrong (prior record wrong, replace)
		return "entered-in-error"
	default:
		return "unknown"
	}
}

// transformAbnormalFlagToInterpretation maps OBX.8 abnormality flag to FHIR CodeableConcept
func (s *HL7FHIRTransformServiceV3) transformAbnormalFlagToInterpretation(flag string) map[string]interface{} {
	f := strings.ToUpper(strings.TrimSpace(flag))
	code, display := "", ""
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
	default:
		code, display = "U", "Unknown"
	}
	return map[string]interface{}{
		"coding": []map[string]interface{}{
			{
				"system":  "http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation",
				"code":    code,
				"display": display,
			},
		},
		"text": display,
	}
}

// transformSIUAppointmentStatus maps HL7 v2 SCH.25 scheduling status codes to FHIR R4
// AppointmentStatus value set (http://hl7.org/fhir/ValueSet/appointmentstatus).
// looksLikeSoftwareVersion returns true when s resembles a software/protocol
// version string (e.g. "2.5", "1.0.0", "v2.3.1"). Used to distinguish a
// real version from a facility name that ended up in source.version.
func looksLikeSoftwareVersion(s string) bool {
	hasDigit, hasDot := false, false
	for _, c := range s {
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
		if c == '.' {
			hasDot = true
		}
	}
	return hasDigit && hasDot
}

// formatXCNDisplayName extracts a human-readable "Family, Given" display string
// from a raw XCN composite value (id^family^given^middle^suffix^prefix^degree…).
// Returns "" when no name components are present so callers can skip the field.
func formatXCNDisplayName(raw string) string {
	parts := strings.SplitN(raw, "^", 8)
	get := func(i int) string {
		if i < len(parts) {
			return strings.TrimSpace(parts[i])
		}
		return ""
	}
	family := get(1)
	given := get(2)
	switch {
	case family != "" && given != "":
		return family + ", " + given
	case family != "":
		return family
	case given != "":
		return given
	default:
		// Only an ID present (get(0)) — not useful as a display name
		return ""
	}
}

func (s *HL7FHIRTransformServiceV3) transformSIUAppointmentStatus(hl7Value string) string {
	switch strings.ToUpper(strings.TrimSpace(hl7Value)) {
	case "PENDING", "WAITLIST", "WAIT":
		return "waitlist"
	case "BOOKED", "SCHEDULED", "":
		return "booked"
	case "ARRIVED":
		return "arrived"
	case "FULFILLED", "COMPLETE", "COMPLETED":
		return "fulfilled"
	case "CANCELLED", "CANCELED":
		return "cancelled"
	case "NOSHOW", "NO-SHOW", "NS":
		return "noshow"
	case "CHECKED-IN", "CHECKED_IN":
		return "checked-in"
	default:
		return "booked"
	}
}

// transformOBXValueByType dispatches OBX.5 value based on OBX.2 value type from rules
func (s *HL7FHIRTransformServiceV3) transformOBXValueByType(value string, rules map[string]interface{}) interface{} {
	valueType := ""
	if rules != nil {
		if vt, ok := rules["value_type"].(string); ok {
			valueType = strings.ToUpper(vt)
		}
	}
	switch valueType {
	case "NM": // Numeric → valueQuantity
		// Some senders embed unit in OBX.5 as "value^unit" even when OBX.2=NM
		// (non-conformant but common). Strip anything after '^' so only the
		// numeric part is written to valueQuantity.value.
		numStr := value
		if idx := strings.IndexByte(value, '^'); idx >= 0 {
			numStr = strings.TrimSpace(value[:idx])
		}
		quantity := map[string]interface{}{"value": numStr}
		if rules != nil {
			if unit, ok := rules["unit"].(string); ok && unit != "" {
				quantity["unit"] = unit
				quantity["system"] = "http://unitsofmeasure.org"
				quantity["code"] = unit
			}
		}
		return map[string]interface{}{"valueQuantity": quantity}
	case "CQ": // Composite Quantity: "value^unit" (HL7 Table 0580)
		// e.g. "180^cm", "72^kg" → valueQuantity
		parts := strings.SplitN(value, "^", 2)
		qty := map[string]interface{}{"value": strings.TrimSpace(parts[0])}
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			unit := strings.TrimSpace(parts[1])
			qty["unit"] = unit
			qty["system"] = "http://unitsofmeasure.org"
			qty["code"] = unit
		}
		if rules != nil {
			if unit, ok := rules["unit"].(string); ok && unit != "" {
				qty["unit"] = unit
				qty["code"] = unit
			}
		}
		return map[string]interface{}{"valueQuantity": qty}
	case "CE", "CWE", "CNE": // Coded → valueCodeableConcept
		return map[string]interface{}{"valueCodeableConcept": s.transformCEToCodeableConcept(value, rules)}
	case "ST", "TX", "FT": // String/Text → valueString
		// Strip any HL7 component separator that leaked into a string field.
		// A value like "180^cm" in an ST-typed OBX means the sender encoded a
		// composite where only the first component is meaningful as plain text.
		strVal := value
		if idx := strings.IndexByte(value, '^'); idx >= 0 {
			strVal = strings.TrimSpace(value[:idx])
		}
		return map[string]interface{}{"valueString": strVal}
	case "TS", "DT", "TM": // Timestamp → valueDateTime
		return map[string]interface{}{"valueDateTime": s.transformTSToDate(value)}
	case "SN": // Structured numeric: comparator^number^separator^number2 (HL7 v2)
		// e.g. "^3.5", ">^10.5", "<^5.0" → valueQuantity with optional comparator
		parts := strings.SplitN(value, "^", 4)
		numStr := ""
		comparator := ""
		if len(parts) >= 2 {
			comparator = strings.TrimSpace(parts[0])
			numStr = strings.TrimSpace(parts[1])
		} else if len(parts) == 1 {
			numStr = strings.TrimSpace(parts[0])
		}
		if numStr != "" {
			qty := map[string]interface{}{"value": numStr}
			if comparator != "" {
				qty["comparator"] = comparator
			}
			if rules != nil {
				if unit, ok := rules["unit"].(string); ok && unit != "" {
					qty["unit"] = unit
					qty["system"] = "http://unitsofmeasure.org"
					qty["code"] = unit
				}
			}
			return map[string]interface{}{"valueQuantity": qty}
		}
		return map[string]interface{}{"valueString": value}
	default:
		// OBX.2 absent or unrecognised — return as string, but sanitize any
		// HL7 component separator so it never appears in FHIR output.
		strVal := value
		if idx := strings.IndexByte(value, '^'); idx >= 0 {
			strVal = strings.TrimSpace(value[:idx])
		}
		return map[string]interface{}{"valueString": strVal}
	}
}

func (s *HL7FHIRTransformServiceV3) transformSSNToIdentifier(ssn string, rules map[string]interface{}) map[string]interface{} {
	identifier := map[string]interface{}{
		"use":   "secondary",
		"value": ssn,
		"type": map[string]interface{}{
			"coding": []map[string]interface{}{
				{
					"code":    "SS",
					"display": "Social Security Number",
					"system":  "http://terminology.hl7.org/CodeSystem/v2-0203",
				},
			},
		},
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			identifier["use"] = use
		}
	}

	return identifier
}

func (s *HL7FHIRTransformServiceV3) transformAccountToIdentifier(account string, rules map[string]interface{}) map[string]interface{} {
	identifier := map[string]interface{}{
		"use":   "secondary",
		"value": account,
		"type": map[string]interface{}{
			"coding": []map[string]interface{}{
				{
					"code":    "AN",
					"display": "Account Number",
					"system":  "http://terminology.hl7.org/CodeSystem/v2-0203",
				},
			},
		},
	}

	if rules != nil {
		if use, ok := rules["use"].(string); ok {
			identifier["use"] = use
		}
	}

	return identifier
}

func (s *HL7FHIRTransformServiceV3) transformMSH9ToCoding(msh9 string) map[string]interface{} {
	// v2-0003 CodeSystem contains trigger events (A01, R01, …), NOT message type
	// names (ADT, ORU).  Always use MSH.9.2 (trigger event) as the code.
	components := strings.Split(msh9, "^")
	eventCoding := map[string]interface{}{
		"system": "http://terminology.hl7.org/CodeSystem/v2-0003",
	}

	triggerEvent := ""
	if len(components) > 1 && components[1] != "" {
		triggerEvent = components[1] // MSH.9.2 — e.g. "R01", "A01"
	} else if len(components) > 0 && components[0] != "" {
		triggerEvent = components[0] // fallback: only field provided
	}

	if triggerEvent != "" {
		eventCoding["code"] = triggerEvent
	}
	return eventCoding
}

// NEW: Transform just trigger event (MSH.9.2) to simple Coding for MessageHeader.eventCoding
func (s *HL7FHIRTransformServiceV3) transformTriggerEventToCoding(triggerEvent string) map[string]interface{} {
	// triggerEvent should be just "A04", "A01", etc. from MSH.9.2
	triggerEvent = strings.TrimSpace(triggerEvent)

	coding := map[string]interface{}{
		"system": "http://terminology.hl7.org/CodeSystem/v2-0003",
		"code":   triggerEvent,
	}

	// ✅ NO HARDCODING: Look up display from database value_set_mappings
	if s.db != nil {
		display := s.lookupDisplayFromDatabase("v2-0003", triggerEvent)
		if display != "" {
			coding["display"] = display
		}
	}

	log.Printf("✅ Created Coding from trigger event: %s → %+v", triggerEvent, coding)
	return coding
}

// ✅ DATABASE-DRIVEN: Look up display names from PostgreSQL value_set_mappings
func (s *HL7FHIRTransformServiceV3) lookupDisplayFromDatabase(codeSystem, code string) string {
	query := `
		SELECT fhir_display
		FROM value_set_mappings
		WHERE fhir_system LIKE $1 AND (hl7_value = $2 OR fhir_code = $2)
		ORDER BY mapping_type ASC
		LIMIT 1
	`

	var display string
	err := s.db.QueryRow(query, "%"+codeSystem+"%", code).Scan(&display)
	if err != nil {
		log.Printf("🔍 No display mapping found for %s#%s in database", codeSystem, code)
		return ""
	}

	log.Printf("✅ Found display mapping: %s#%s → %s", codeSystem, code, display)
	return display
}

// Transform MSH.10 Control ID to proper Reference for MessageHeader.focus
func (s *HL7FHIRTransformServiceV3) transformControlIdToReference(controlId string) map[string]interface{} {
	// MessageHeader.focus expects Reference objects, not primitive identifiers
	// Since we don't have actual Patient IDs at this point, we'll create a logical reference
	controlId = strings.TrimSpace(controlId)

	// Create a proper Reference object (not just identifier string)
	reference := map[string]interface{}{
		"identifier": map[string]interface{}{ // ✅ Identifier object, not primitive
			"system": "http://terminology.hl7.org/CodeSystem/v2-0203",
			"value":  controlId,
		},
		"display": fmt.Sprintf("HL7 Message Control ID: %s", controlId),
	}

	log.Printf("✅ Created Reference from control ID: %s → %+v", controlId, reference)
	return reference
}

// =====================================
// SPEC-DRIVEN NORMALIZER HELPERS
// =====================================

// hl7v2IdentifierTypeSystem and table0203 are package-level aliases pointing at the
// canonical definitions in hl7assembly/datatypes.go.  Use hl7assembly.HL7V2IdentifierTypeSystem
// and hl7assembly.Table0203 directly in new code.
const hl7v2IdentifierTypeSystem = hl7assembly.HL7V2IdentifierTypeSystem

var table0203 = hl7assembly.Table0203

// table0202 maps HL7 Table 0202 (telecommunication use codes) → FHIR ContactPoint.use.
// Also covers non-standard plain-English codes (HOME, WORK, CELL, MOBILE) sent by some systems.
var table0202 = map[string]string{
	"PRN": "home",
	"WPN": "work",
	"ORN": "temp",
	"BPN": "temp",
	"VHN": "home",
	"ASN": "temp",
	"EMR": "temp",
	"NET": "home",
	"PRS": "mobile",
	// Non-standard plain-English codes
	"HOME":   "home",
	"WORK":   "work",
	"CELL":   "mobile",
	"MOBILE": "mobile",
}

// table0201 maps HL7 Table 0201 (telecommunication equipment type) → FHIR ContactPoint.system
var table0201 = map[string]string{
	"PH":       "phone",
	"FX":       "fax",
	"MD":       "other",
	"CP":       "phone",
	"BP":       "pager",
	"X.400":    "email",
	"Internet": "email",
	"internet": "email",
}

// normalizeIdentifierSystem is a package-level shim delegating to the canonical
// hl7assembly.NormalizeIdentifierSystem.  All new code should call that function directly.
func normalizeIdentifierSystem(sys string) string {
	return hl7assembly.NormalizeIdentifierSystem(sys)
}

// isValidBCP47Language returns true when lang looks like a valid IETF BCP-47
// language tag (e.g. "en", "en-US", "zh-Hant-TW").  HL7 coding-system names
// such as "ICD10", "ADM", or "SNOMED" are rejected so callers can strip them.
//
// Key invariant: BCP-47 primary language subtags (ISO 639-1/2/3) are ALWAYS
// lowercase ("en", "fra", "zho").  HL7 table values like "ADM", "ICD10", "MED"
// are uppercase or mixed — none are valid BCP-47 primary subtags.
func isValidBCP47Language(lang string) bool {
	if len(lang) < 2 {
		return false
	}
	// Extract the primary subtag (before first hyphen).
	primary := lang
	if idx := strings.Index(lang, "-"); idx >= 0 {
		primary = lang[:idx]
	}
	// Primary subtag: 2–8 ASCII letters only (no digits, underscores, etc.)
	if len(primary) < 2 || len(primary) > 8 {
		return false
	}
	for i := 0; i < len(primary); i++ {
		c := primary[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	// BCP-47 primary language subtags are always lowercase.
	// Anything with uppercase letters in the primary tag is an HL7 table value.
	if primary != strings.ToLower(primary) {
		return false
	}
	return true
}
