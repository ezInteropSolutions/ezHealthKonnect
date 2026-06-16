// services/hl7_fhir_transform_service_v3.go
// TRUE SCHEMA-DRIVEN HL7 to FHIR transformation service
//
// 🎯 PRINCIPLE: ATOMIC MAPPING-DRIVEN APPROACH
// - Each database mapping = one atomic transformation
// - Handle composite HL7 fields automatically
// - Use schema for validation, mappings for content
// - Global logic works for any resource type
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/fhir"
	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/hl7" // For composite field parsing
	"ezhealthkonnect/services/hl7assembly"
	"ezhealthkonnect/services/manifest"
	"ezhealthkonnect/services/mappers"
	segment_processors "ezhealthkonnect/services/segment_processors"
)

// =====================================
// SCHEMA-DRIVEN TRANSFORM SERVICE V3
// =====================================
// Note: Uses FieldMapping struct from hl7_fhir_transform_service.go

// AssemblyRule is a single cross-resource wiring rule loaded from the
// assembly_rules table.  The engine applies these after field-mapping so that
// no Go file needs to change when a new message type is added.
type AssemblyRule struct {
	ID             string
	MessageType    string
	RuleType       string                 // "reference"|"focus"|"result"|"subject"|"performer"|"encounter"|"author"|"logical_ref"
	SourceResource string
	TargetResource string
	ReferencePath  string
	ConditionExpr  string
	Config         map[string]interface{} // rule-specific parameters (from config JSONB column); nil for standard rules
	Sequence       int
}

type HL7FHIRTransformServiceV3 struct {
	db            *sql.DB
	fhirLoader    *fhir.FHIRSchemaLoader
	schemaReady   bool
	zsegmentSvc   *ZSegmentService // loads MFNRuntimeConfig per interface

	// Assembly rules cache — loaded once from DB, invalidated on OOB rebuild.
	assemblyRules   map[string][]AssemblyRule // messageType → sorted rules
	assemblyRulesMu sync.RWMutex

	// Valueset mapper — DB-driven HL7 table code → FHIR code lookup.
	// Queries hl7_fhir_value_mappings (populated by V115 IG seed data).
	valueMapper *mappers.ValueMapper
}

func NewHL7FHIRTransformServiceV3(database *sql.DB) *HL7FHIRTransformServiceV3 {
	service := &HL7FHIRTransformServiceV3{
		db:           database,
		fhirLoader:   nil, // Will be loaded on-demand during Transform()
		zsegmentSvc:  NewZSegmentService(database),
		valueMapper:  mappers.NewValueMapper(database),
	}

	// Note: FHIR schema loader availability is checked at Transform() time
	// This allows the service to be created before schemas are loaded (OOB pattern)

	return service
}

// =====================================
// MAIN TRANSFORMATION METHOD
// =====================================

func (s *HL7FHIRTransformServiceV3) Transform(
	ctx context.Context,
	request *TransformRequest,
) (*TransformResponse, error) {
	// On-demand FHIR schema loader initialization (OOB pattern)
	if s.fhirLoader == nil {
		s.fhirLoader = fhir.GetFHIRSchemaLoader()
	}

	// Verify schemas are available
	if s.fhirLoader == nil {
		return nil, fmt.Errorf("FHIR schema loader not initialized")
	}

	// Check if schemas are loaded
	available, err := s.fhirLoader.ListAvailableSchemas()
	if err != nil || len(available) == 0 {
		return nil, fmt.Errorf("FHIR schemas not loaded - cannot perform schema-driven transformation")
	}

	startTime := time.Now()
	response := s.initializeResponse(request)

	log.Printf("🔄 HL7→FHIR transformation v3: %s [ATOMIC MAPPING-DRIVEN]", response.RequestID)

	// Use message type from request if provided (OOB: injected from pipeline config)
	messageType := request.MessageType
	if messageType == "" {
		// Fallback: Extract message type from parsed HL7 data
		var err error
		messageType, err = s.extractMessageType(request.ParsedHL7Data)
		if err != nil {
			response.Errors = append(response.Errors, fmt.Sprintf("Failed to extract message type: %v", err))
			return response, nil
		}
	}
	response.MessageType = messageType
	log.Printf("✅ Using message type: %s", messageType)

	// Get field mappings (pass request so embedded_mappings and interface_id are available).
	// contextLinks is non-nil for v1.1+ OOB templates that declare a "context" block;
	// nil for v1.0 templates and custom/embedded mappings (existing wiring handles those).
	fieldMappings, contextLinks, err := s.getFieldMappings(ctx, messageType, request.TargetProfile, request)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to get field mappings: %v", err))
		return response, nil
	}

	log.Printf("📋 Loaded %d atomic field mappings for message type %s", len(fieldMappings), messageType)

	// Enrich mappings with HL7 and FHIR data types from schemas for auto-translation
	fieldMappings = s.enrichMappingsWithDataTypes(fieldMappings, messageType)

	// DEBUG: Log sample mappings to see what we have
	for i, mapping := range fieldMappings {
		if i >= 3 {
			break
		} // Show first 3
		log.Printf("🔍 Mapping %d: %s.%s → %s.%s (transform: %s)",
			i+1, mapping.SegmentName, mapping.HL7Field, mapping.FHIRResourceType, mapping.FHIRElementPath, mapping.DataTypeTransform)
	}

	// Determine which FHIR resources to create based on mappings
	resourceTypes := s.extractResourceTypes(fieldMappings)
	log.Printf("🎯 Will create resources: %v", resourceTypes)

	// Extract HL7 segments once for all resources
	enhancedSegments := s.extractEnhancedSegments(request.ParsedHL7Data)
	if enhancedSegments == nil {
		response.Errors = append(response.Errors, "No enhanced segments found in parsed HL7 data")
		return response, nil
	}

	// DEBUG: Show available HL7 segments
	segmentNames := make([]string, 0, len(enhancedSegments))
	for segName := range enhancedSegments {
		segmentNames = append(segmentNames, segName)
	}
	log.Printf("🔍 Available HL7 segments: %v", segmentNames)

	// Transform each resource type using atomic mappings
	transformStartTime := time.Now()
	var allResources []map[string]interface{}
	var allWarnings []string
	var allErrors []string
	stats := MappingStatistics{}

	for _, resourceType := range resourceTypes {
		log.Printf("🔧 Creating %s using atomic mappings", resourceType)

		// Load real FHIR schema for validation
		schema, err := s.loadFHIRSchema(resourceType, request.TargetProfile, request.FHIRVersion)
		if err != nil {
			allErrors = append(allErrors, fmt.Sprintf("Failed to load schema for %s: %v", resourceType, err))
			continue
		}

		// Get mappings for this resource type
		resourceMappings := s.filterMappingsForResource(fieldMappings, resourceType)
		log.Printf("📊 Processing %d mappings for %s", len(resourceMappings), resourceType)

		// Create resource using atomic mappings
		resource, warnings, errors, mappedCount := s.createResourceFromAtomicMappings(
			resourceType,
			schema,
			enhancedSegments,
			resourceMappings,
		)

		if resource != nil {
			allResources = append(allResources, resource)
		}

		allWarnings = append(allWarnings, warnings...)
		allErrors = append(allErrors, errors...)

		// Track successfully mapped fields
		stats.TotalFieldsMapped += mappedCount
	}

	// ── Profile-driven composite + procedure assembly ───────────────────────
	// All structural assembly (ORU observations, MDM documents, DFT charges,
	// VXU immunizations, MFN master file resources) is now dispatched through
	// the profile engine via named procedures declared in the v2.0 template
	// stored in hl7_fhir_templates. Each message type's profile JSON contains:
	//
	//   "composites": [{ "procedure": "<name>", "params": {...} }]
	//
	// The procedure registry (services/hl7assembly/procedure_registry.go)
	// bridges each named procedure to the typed Go assembly function.
	//
	// Migrations required before this path is active:
	//   V97  — SIU^S12 / S13-S26
	//   V98  — ORU^R01 (assembleObservationsFromOBX)
	//   V99  — MDM^T01-T11 (buildDocumentReferenceFromTXA)
	//   V100 — VXU^V04 (assembleVXUImmunizations)
	//   V101 — DFT^P03 (assembleDFTCharges)
	//   V102 — MFN^M02-M16 (assembleMFNResources)
	profileComposites, profileValueSets := s.getProfileComposites(ctx, messageType)
	if len(profileComposites) > 0 {
		allResources = s.applyProfileComposites(allResources, request.ParsedHL7Data, profileComposites, profileValueSets)
		log.Printf("✅ Profile composites applied: %d rules for %s", len(profileComposites), messageType)
	}

	// Wire cross-resource references declared in the template's contextLinks block
	// (v1.1+ templates).  For v1.0 templates contextLinks is nil and this is a no-op;
	// the existing hardcoded wiring in post-processing normalizers still handles those.
	allResources = s.applyContextLinks(allResources, contextLinks)

	// ── Stage: IG manifest filter ──────────────────────────────────────────────
	// Remove resource types not permitted by the IG for this message type.
	// Interface-level resource_policy overrides are loaded from
	// interface_message_mappings.custom_mapping_config and applied here.
	resourcePolicy := s.loadResourcePolicy(ctx, request.InterfaceID, messageType)
	allResources, removedTypes := manifest.FilterResources(allResources, messageType, resourcePolicy)
	if len(removedTypes) > 0 {
		log.Printf("ℹ️  Manifest filter removed %v for %s (not in IG output set)", removedTypes, messageType)
	}

	// Post-mapping normalizers — run for all message types to fix common field gaps
	// that mapping transforms may not handle (no DB rows, partial templates, etc.).
	// IMPORTANT: narrative (text.div) is regenerated at the END of this block so it
	// always reflects the fully-normalized field values, not pre-normalization raw HL7.
	for _, r := range allResources {
		// Strip null-valued fields and empty arrays first — both are invalid in FHIR
		// output and result in validator errors.  cleanFHIRResource is recursive so
		// nested objects and slice entries are cleaned in one pass.
		cleanFHIRResource(r)

		rt, _ := r["resourceType"].(string)
		switch rt {
		case "Practitioner":
			// birthDate: HL7 TS (YYYYMMDD) → FHIR date (YYYY-MM-DD)
			if bd, ok := r["birthDate"].(string); ok && len(bd) >= 8 && !strings.Contains(bd, "-") {
				r["birthDate"] = s.transformTSToDate(bd)
			}
			// active: HL7 table 0183 (A/I) stored as string → FHIR boolean
			if active, ok := r["active"].(string); ok {
				switch strings.ToUpper(strings.TrimSpace(active)) {
				case "A", "Y", "1", "TRUE", "ACTIVE":
					r["active"] = true
				case "I", "N", "0", "FALSE", "INACTIVE":
					r["active"] = false
				default:
					// Unknown code — default to active (practitioner was looked up)
					r["active"] = true
				}
			}
			// active: transformToBoolean defaults any unrecognised value to bool false
			// (e.g. "M" from a gender code mispositioned in STF.7). Only keep false
			// when MFE.1 explicitly requests deletion or deactivation.
			if activeBool, ok := r["active"].(bool); ok && !activeBool {
				mfeAction := s.mfeActionFromSegments(enhancedSegments)
				if mfeAction != "MDL" && mfeAction != "MDC" {
					r["active"] = true
				}
			}
			// identifier: re-parse any value that is still a raw CX composite string
			// (e.g. "999" stored correctly, or "999^DEA" where CX.2 is a type code).
			// Also flatten any value that landed as an object from cx_to_identifier.
			reparsePractitionerIdentifiers(r)

		case "PractitionerRole":
			// language: the OOB schema generator maps PRA.1 (a CE composite key
			// like "999^12345^1^PROVIDER^1") to the DomainResource.language field
			// because it is a code field and PRA.1 has no explicit anchor. Strip it
			// when the value is clearly an HL7 composite (contains "^") — it is not
			// a BCP-47 language tag and causes the FHIR validator to cascade-fail
			// the entire resource (NarrativeStatus, displayLanguage errors).
			if lang, ok := r["language"].(string); ok && strings.Contains(lang, "^") {
				delete(r, "language")
			}
			// identifier: flatten any identifier whose .value is a CodeableConcept object
			// (CE type mapped via ce_to_codeableconcept instead of ce_code_only)
			if ids, ok := r["identifier"].([]interface{}); ok {
				for _, idRaw := range ids {
					if id, ok2 := idRaw.(map[string]interface{}); ok2 {
						if valMap, ok3 := id["value"].(map[string]interface{}); ok3 {
							// Extract text or first coding.code as the flat identifier value
							if txt, ok4 := valMap["text"].(string); ok4 && txt != "" {
								id["value"] = txt
							} else if codings, ok4 := valMap["coding"].([]interface{}); ok4 && len(codings) > 0 {
								if cod, ok5 := codings[0].(map[string]interface{}); ok5 {
									if code, ok6 := cod["code"].(string); ok6 && code != "" {
										id["value"] = code
									}
								}
							}
						}
					}
				}
			}
			// Re-parse raw CX composite strings that survived as-is (e.g. "999^12345^1^PROVIDER^1")
			reparsePractitionerIdentifiers(r)
		case "DocumentReference":
			// status (1..1): TXAProcessor sets this via TXAAvailabilityStatus(TXA.19).
			// If the template mapper ran without TXAProcessor (e.g. non-MDM messages
			// that somehow produce a DocumentReference), coerce any non-FHIR code to
			// the safe default "current".
			validDRStatus := map[string]bool{"current": true, "superseded": true, "entered-in-error": true}
			if st, ok := r["status"].(string); ok {
				if !validDRStatus[strings.ToLower(st)] {
					r["status"] = "current"
				} else {
					r["status"] = strings.ToLower(st)
				}
			} else {
				r["status"] = "current"
			}
			// language: strip if it contains HL7 composite delimiters (not a BCP-47 tag).
			if lang, ok := r["language"].(string); ok && (strings.Contains(lang, "^") || strings.Contains(lang, "|")) {
				delete(r, "language")
			}
			// content (1..1): required by FHIR R4.  TXAProcessor always provides it.
			// If TXAProcessor did NOT run (e.g. a non-MDM DocumentReference) inject a
			// minimal placeholder so the validator does not hard-fail on missing content.
			if _, hasContent := r["content"]; !hasContent {
				r["content"] = []interface{}{
					map[string]interface{}{
						"attachment": map[string]interface{}{
							"contentType": "text/plain",
							"title":       "(no document content)",
						},
					},
				}
			}

		case "ChargeItem":
			// status (1..1, required) — DFT messages carry financial transaction data;
			// without an explicit status from the HL7 sender, "billable" is the correct
			// default for a posted charge (DFT^P03 = Post Detail Financial Transaction).
			validCIStatus := map[string]bool{
				"planned": true, "billable": true, "not-billable": true,
				"aborted": true, "billed": true, "entered-in-error": true, "unknown": true,
			}
			if st, ok := r["status"].(string); !ok || !validCIStatus[strings.ToLower(st)] {
				r["status"] = "billable"
			}

			// occurrenceDateTime: FT1.4 is an HL7 TS (YYYYMMDDHHmmss) — convert to ISO 8601.
			if odt, ok := r["occurrenceDateTime"].(string); ok && odt != "" && !strings.Contains(odt, "T") && !strings.Contains(odt, "-") {
				r["occurrenceDateTime"] = hl7assembly.ToISO(odt)
			}

			// factorOverride (decimal): FT1.10 (service quantity) lands here as a string;
			// convert to float64 so the FHIR validator accepts the primitive type.
			if fo, ok := r["factorOverride"].(string); ok {
				if f, err := strconv.ParseFloat(strings.TrimSpace(fo), 64); err == nil {
					r["factorOverride"] = f
				} else {
					delete(r, "factorOverride")
				}
			}

			// quantity.value (decimal): must be a number; delete if it cannot be parsed
			// (e.g. a CPT code like "71020" that was mis-anchored to quantity.value).
			if qRaw, ok := r["quantity"].(map[string]interface{}); ok {
				if qvStr, ok2 := qRaw["value"].(string); ok2 {
					if f, err := strconv.ParseFloat(strings.TrimSpace(qvStr), 64); err == nil {
						qRaw["value"] = f
					} else {
						delete(r, "quantity")
					}
				}
			}

			// definitionCanonical: must be absolute URIs or fragment refs — delete any
			// elements that look like raw numeric/price values (e.g. "0.000000").
			if dc, ok := r["definitionCanonical"].([]interface{}); ok {
				validDC := dc[:0]
				for _, v := range dc {
					if s, ok2 := v.(string); ok2 && (strings.HasPrefix(s, "http") || strings.HasPrefix(s, "#") || strings.HasPrefix(s, "urn:")) {
						validDC = append(validDC, v)
					}
				}
				if len(validDC) == 0 {
					delete(r, "definitionCanonical")
				} else {
					r["definitionCanonical"] = validDC
				}
			}

			// implicitRules: a FHIR base field that must be an absolute URI — delete
			// any value that is not a valid URI (e.g. a raw transaction ID "1133").
			if ir, ok := r["implicitRules"].(string); ok {
				if !strings.HasPrefix(ir, "http") && !strings.HasPrefix(ir, "urn:") {
					delete(r, "implicitRules")
				}
			}

		case "ServiceRequest":
			// language: OOB templates sometimes map ORC.6 (Response Flag) or ORC.13
			// (Enterer's Location / Order Status Modifier) to Resource.language because
			// those are IS/PL code fields and the generator picks the closest FHIR code
			// field. Neither carries a BCP-47 language tag; any HL7 composite value ("|"
			// or "^") would poison the FHIR validator's displayLanguage check and cascade
			// false failures across every value-set lookup on the resource.
			if lang, ok := r["language"].(string); ok && (strings.Contains(lang, "^") || strings.Contains(lang, "|")) {
				delete(r, "language")
			}
			// intent is required (1..1) in FHIR R4. ORC.1 (Order Control Code, Table 0119)
			// maps to intent via valueMap, but some senders omit ORC.1 entirely. Default to
			// "order" per the HL7-to-FHIR IG, which treats a missing ORC.1 as an implicit
			// new order.
			if intentVal, hasIntent := r["intent"]; !hasIntent || intentVal == "" {
				r["intent"] = "order"
			}

		case "Patient":
			// birthDate: HL7 TS (YYYYMMDD) → FHIR date (YYYY-MM-DD)
			if bd, ok := r["birthDate"].(string); ok && len(bd) >= 8 && !strings.Contains(bd, "-") {
				r["birthDate"] = s.transformTSToDate(bd)
			}
			// gender: HL7 table 0001 → FHIR AdministrativeGender
			if g, ok := r["gender"].(string); ok {
				switch strings.ToUpper(strings.TrimSpace(g)) {
				case "M":
					r["gender"] = "male"
				case "F":
					r["gender"] = "female"
				case "O":
					r["gender"] = "other"
				case "U", "UN", "UNKNOWN":
					r["gender"] = "unknown"
				}
			}
			// identifier — spec-driven enrichment:
			// 1. Normalize identifier.system (HD/OID → urn:oid:, non-URIs dropped).
			// 2. Add v2-0203 system + display to identifier.type.coding when code is present.
			// 3. Enrich identifiers from raw PID segments for fields that lack a type subfield
			//    in HL7 (e.g. PID.19 = SSN, PID.18 = Account Number).
			s.enrichPatientIdentifiers(r, request.ParsedHL7Data)
			// Flatten any identifier.value that is an object (cx_to_identifier stored a full
			// Identifier map at a scalar leaf — same pattern as PractitionerRole above).
			if ids, ok := r["identifier"].([]interface{}); ok {
				for _, idRaw := range ids {
					if id, ok2 := idRaw.(map[string]interface{}); ok2 {
						if valMap, ok3 := id["value"].(map[string]interface{}); ok3 {
							if txt, ok4 := valMap["text"].(string); ok4 && txt != "" {
								id["value"] = txt
							} else if v, ok4 := valMap["value"].(string); ok4 && v != "" {
								id["value"] = v
							} else if codings, ok4 := valMap["coding"].([]interface{}); ok4 && len(codings) > 0 {
								if cod, ok5 := codings[0].(map[string]interface{}); ok5 {
									if code, ok6 := cod["code"].(string); ok6 && code != "" {
										id["value"] = code
									}
								}
							}
						}
					}
				}
			}
			// Strip empty identifier objects (empty PID fields produce {})
			if ids, ok := r["identifier"].([]interface{}); ok {
				var validIDs []interface{}
				for _, idRaw := range ids {
					if idMap, ok2 := idRaw.(map[string]interface{}); ok2 && len(idMap) > 0 {
						validIDs = append(validIDs, idRaw)
					}
				}
				if len(validIDs) == 0 {
					delete(r, "identifier")
				} else {
					r["identifier"] = validIDs
				}
			}
			// telecom — spec-driven enrichment from PID.13 (home), PID.14 (work), PID.40 (email).
			// Mapping engine may have partially populated telecom; this fills any gaps and
			// normalizes use/system codes per HL7 table 0201/0202.
			s.enrichPatientTelecom(r, request.ParsedHL7Data)
			// Strip empty extension objects (empty PID fields can create {})
			if exts, ok := r["extension"].([]interface{}); ok {
				var validExts []interface{}
				for _, extRaw := range exts {
					if extMap, ok2 := extRaw.(map[string]interface{}); ok2 && len(extMap) > 0 {
						if _, hasURL := extMap["url"]; hasURL {
							validExts = append(validExts, extRaw)
						}
					}
				}
				if len(validExts) == 0 {
					delete(r, "extension")
				} else {
					r["extension"] = validExts
				}
			}
			// deceased: HL7 Y/N indicator → FHIR deceasedBoolean
			for _, decKey := range []string{"deceased", "deceasedBoolean"} {
				if dec, ok := r[decKey].(string); ok {
					delete(r, decKey)
					switch strings.ToUpper(strings.TrimSpace(dec)) {
					case "Y", "YES", "TRUE", "1":
						r["deceasedBoolean"] = true
					}
				}
			}
			// maritalStatus: add system when coding has code but no system
			if ms, ok := r["maritalStatus"].(map[string]interface{}); ok {
				if codings, ok2 := ms["coding"].([]interface{}); ok2 {
					for _, cRaw := range codings {
						if cMap, ok3 := cRaw.(map[string]interface{}); ok3 {
							if sys, _ := cMap["system"].(string); sys == "" {
								cMap["system"] = "http://terminology.hl7.org/CodeSystem/v3-MaritalStatus"
							}
						}
					}
				}
			}
			// address[].use: HL7 table 0190 → FHIR AddressUse
			if addrs, ok := r["address"].([]interface{}); ok {
				for _, aRaw := range addrs {
					if a, ok2 := aRaw.(map[string]interface{}); ok2 {
						if use, ok3 := a["use"].(string); ok3 {
							a["use"] = normalizeAddressUse(use)
						}
					}
				}
			}
			// name[].use: HL7 table 0200 → FHIR NameUse
			if names, ok := r["name"].([]interface{}); ok {
				for _, nRaw := range names {
					if n, ok2 := nRaw.(map[string]interface{}); ok2 {
						if use, ok3 := n["use"].(string); ok3 {
							n["use"] = normalizeNameUse(use)
						}
					}
				}
			}
			// communication[].language: add BCP-47 system when missing
			if comms, ok := r["communication"].([]interface{}); ok {
				for _, cRaw := range comms {
					if c, ok2 := cRaw.(map[string]interface{}); ok2 {
						if lang, ok3 := c["language"].(map[string]interface{}); ok3 {
							if codings, ok4 := lang["coding"].([]interface{}); ok4 {
								for _, codRaw := range codings {
									if cod, ok5 := codRaw.(map[string]interface{}); ok5 {
										if sys, _ := cod["system"].(string); sys == "" {
											if code, _ := cod["code"].(string); code != "" {
												cod["system"] = "urn:ietf:bcp:47"
												cod["code"] = strings.ToLower(strings.TrimSpace(code))
											}
										}
									}
								}
							}
						}
					}
				}
			}

		case "Encounter":
			// language: strip if not a valid BCP-47 tag — HL7 field values like "ADM"
			// (PV1.10 hospital service) are heuristically mapped here by the template and
			// cause cascading validation failures on all value-set lookups in the resource.
			if lang, ok := r["language"].(string); ok && !isValidBCP47Language(lang) {
				delete(r, "language")
			}
			// diagnosis: remove entries where condition reference is absent or null —
			// the template creates the slot but cannot wire the reference without a
			// cross-resource pass (done below after the normalizer loop).
			if diags, ok := r["diagnosis"].([]interface{}); ok {
				var kept []interface{}
				for _, dRaw := range diags {
					if d, ok2 := dRaw.(map[string]interface{}); ok2 {
						if cond := d["condition"]; cond != nil {
							kept = append(kept, dRaw)
						}
					}
				}
				if len(kept) == 0 {
					delete(r, "diagnosis")
				} else {
					r["diagnosis"] = kept
				}
			}
			// status: map HL7 ADT trigger event codes to FHIR encounter-status values.
			// Primary: map whatever value the template wrote (e.g. MSH.9.2 mapped to status).
			if st, ok := r["status"].(string); ok {
				if mapped := mapEncounterStatus(st); mapped != "" {
					r["status"] = mapped
				}
			}
			// Fallback: template had no mapping for status — derive from the ADT trigger
			// event code embedded in the message type (e.g. "ADT^A01^ADT_A01" → "A01").
			// Strip the optional structure-name component after the second ^ so that
			// mapEncounterStatus receives only the two-letter event code.
			if _, hasStatus := r["status"]; !hasStatus {
				if idx := strings.Index(messageType, "^"); idx >= 0 {
					triggerPart := messageType[idx+1:]
					if idx2 := strings.Index(triggerPart, "^"); idx2 >= 0 {
						triggerPart = triggerPart[:idx2]
					}
					if derived := mapEncounterStatus(triggerPart); derived != "" {
						r["status"] = derived
					}
				}
			}
			if _, hasStatus := r["status"]; !hasStatus {
				r["status"] = "unknown"
			}
			// participant.type: inject v3-ParticipationType role discriminators from PV1.
			// Type codes come from hl7assembly.PV1ParticipantRoles — no hardcoding here.
			s.enrichEncounterParticipants(r, request.ParsedHL7Data)
			// class: FHIR R4 required field (Coding with system).
			// OOB template anchor writes PV1.2 → Encounter.class.code (scalar path),
			// producing {"code": "O"} — a Coding without a system.  The template path
			// and the raw-string path both need to reach normalizePatientClassToCoding
			// so the ActCode system URI and display are always present.
			if cls, ok := r["class"]; ok {
				switch clsVal := cls.(type) {
				case string:
					if clsVal != "" {
						r["class"] = normalizePatientClassToCoding(clsVal)
					}
				case map[string]interface{}:
					// Template wrote {"code": "O"} via the .code scalar path — add system.
					if code, hasCode := clsVal["code"].(string); hasCode && code != "" {
						if _, hasSystem := clsVal["system"]; !hasSystem {
							r["class"] = normalizePatientClassToCoding(code)
						}
					}
				}
			} else {
				r["class"] = map[string]interface{}{
					"system":  "http://terminology.hl7.org/CodeSystem/v3-ActCode",
					"code":    "IMP",
					"display": "inpatient encounter",
				}
			}

		case "Coverage":
			// status: required FHIR R4 field — default to "active" when not mapped.
			if _, hasStatus := r["status"]; !hasStatus {
				r["status"] = "active"
			}
			// Validate status against the fm-status value set (active|cancelled|draft|entered-in-error).
			// HL7 billing codes (e.g. "P", "B", "N") that the heuristic may have placed here are
			// not valid fm-status values — the IG has no v2 concept map for IN1 billing status to
			// fm-status, so we default to "active".
			if st, _ := r["status"].(string); st != "" {
				switch st {
				case "active", "cancelled", "draft", "entered-in-error":
					// valid fm-status — keep as-is
				default:
					r["status"] = "active"
				}
			}
			// type.coding[].code: ce_to_codeableconcept may have placed a CodeableConcept
			// object at the scalar .code leaf — extract to string.
			flattenCodingCodeObjects(r, "type")
			// class[].type (1..1) and class[].value (1..1): both required in Coverage.class.
			// JSON round-trip normalises typed Go slices ([]map[string]interface{}) to
			// []interface{} so that the type assertion below always succeeds regardless of
			// how the template engine constructed the slice.
			if rawClass, exists := r["class"]; exists {
				clsBytes, _ := json.Marshal(rawClass)
				var classes []interface{}
				if err := json.Unmarshal(clsBytes, &classes); err == nil {
					for _, clsRaw := range classes {
						if cls, ok2 := clsRaw.(map[string]interface{}); ok2 {
							// Ensure value (1..1): promote name→value if value absent.
							if _, hasValue := cls["value"]; !hasValue {
								if name, _ := cls["name"].(string); name != "" {
									cls["value"] = name
								}
							}
							// Ensure type (1..1): default to "group" per IG IN1.8 → class[group].
							if _, hasType := cls["type"]; !hasType {
								cls["type"] = map[string]interface{}{
									"coding": []interface{}{
										map[string]interface{}{
											"system":  "http://terminology.hl7.org/CodeSystem/coverage-class",
											"code":    "group",
											"display": "Group",
										},
									},
								}
							}
						}
					}
					r["class"] = classes
				}
			}
			// payor: must be []Reference — normalize each element.
			if payors, ok := r["payor"].([]interface{}); ok {
				var validPayors []interface{}
				for _, p := range payors {
					switch pv := p.(type) {
					case map[string]interface{}:
						// identifier must be an object, not an array
						if idArr, ok2 := pv["identifier"].([]interface{}); ok2 && len(idArr) > 0 {
							if idObj, ok3 := idArr[0].(map[string]interface{}); ok3 {
								// Flatten nested identifier.value object
								if valMap, ok4 := idObj["value"].(map[string]interface{}); ok4 {
									if val, ok5 := valMap["value"].(string); ok5 && val != "" {
										idObj["value"] = val
									}
								}
								pv["identifier"] = idObj
							}
						}
						validPayors = append(validPayors, pv)
					case string:
						// Raw string (e.g. group ID "GRP002") — wrap as Reference.display.
						if pv != "" {
							validPayors = append(validPayors, map[string]interface{}{"display": pv})
						}
					}
				}
				if len(validPayors) > 0 {
					r["payor"] = validPayors
				} else {
					delete(r, "payor")
				}
			}
			// relationship.coding[].system: add when absent (SELF etc. need subscriber-relationship system).
			// JSON round-trip normalises the map type so the assertion always succeeds.
			if rawRel, exists := r["relationship"]; exists {
				relBytes, _ := json.Marshal(rawRel)
				var rel map[string]interface{}
				if err := json.Unmarshal(relBytes, &rel); err == nil {
					if codings, ok2 := rel["coding"].([]interface{}); ok2 {
						for _, cRaw := range codings {
							if c, ok3 := cRaw.(map[string]interface{}); ok3 {
								if sys, _ := c["system"].(string); sys == "" {
									if code, _ := c["code"].(string); code != "" {
										c["system"] = "http://terminology.hl7.org/CodeSystem/subscriber-relationship"
										c["code"] = strings.ToLower(strings.TrimSpace(code))
									}
								}
							}
						}
						rel["coding"] = codings
					}
					r["relationship"] = rel
				}
			}

		case "Condition":
			// language: strip if not a valid BCP-47 tag — DG1.2 (coding method, e.g. "ICD10")
			// is sometimes heuristically mapped here by the template and causes cascading
			// validation failures because it is not a language code.
			if lang, ok := r["language"].(string); ok && !isValidBCP47Language(lang) {
				delete(r, "language")
			}
			// category.coding[].system: HL7 diagnosis type → condition-category system.
			if cats, ok := r["category"].([]interface{}); ok {
				for _, catRaw := range cats {
					if cat, ok2 := catRaw.(map[string]interface{}); ok2 {
						if codings, ok3 := cat["coding"].([]interface{}); ok3 {
							for _, cRaw := range codings {
								if c, ok4 := cRaw.(map[string]interface{}); ok4 {
									if sys, _ := c["system"].(string); sys == "" {
										if code, _ := c["code"].(string); code != "" {
											c["system"] = "http://terminology.hl7.org/CodeSystem/condition-category"
											// Map HL7 DG1.6 diagnosis type codes to FHIR condition-category
											switch strings.ToUpper(strings.TrimSpace(code)) {
											case "A", "AD", "ADMITTING":
												c["code"] = "encounter-diagnosis"
												c["display"] = "Encounter Diagnosis"
											case "F", "FINAL", "W", "WORKING":
												c["code"] = "encounter-diagnosis"
												c["display"] = "Encounter Diagnosis"
											case "P", "PROBLEM", "CHRONIC":
												c["code"] = "problem-list-item"
												c["display"] = "Problem List Item"
											default:
												c["code"] = "encounter-diagnosis"
												c["display"] = "Encounter Diagnosis"
											}
										}
									}
								}
							}
						}
					}
				}
			}

		case "RelatedPerson":
			// gender: HL7 Table 0001 (M/F/O/U) → FHIR AdministrativeGender.
			// GT1.9 and NK1.15 are anchored to RelatedPerson.gender — the raw HL7 code
			// must be normalized the same way as Patient.gender (PID.8).
			if g, ok := r["gender"].(string); ok {
				switch strings.ToUpper(strings.TrimSpace(g)) {
				case "M":
					r["gender"] = "male"
				case "F":
					r["gender"] = "female"
				case "O":
					r["gender"] = "other"
				case "U", "UN", "UNKNOWN":
					r["gender"] = "unknown"
				}
			}
			// relationship.coding[].code: ce_to_codeableconcept may have placed a
			// CodeableConcept object at the scalar .code leaf — extract to string.
			if rels, ok := r["relationship"].([]interface{}); ok {
				for _, relRaw := range rels {
					if rel, ok2 := relRaw.(map[string]interface{}); ok2 {
						if codings, ok3 := rel["coding"].([]interface{}); ok3 {
							for _, cRaw := range codings {
								if c, ok4 := cRaw.(map[string]interface{}); ok4 {
									if codeMap, ok5 := c["code"].(map[string]interface{}); ok5 {
										extracted := extractCodeFromCodeableConcept(codeMap)
										if extracted != "" {
											c["code"] = extracted
										} else {
											delete(c, "code")
										}
									}
									// Add system when absent (v3-RoleCode for relatedperson relationships)
									if sys, _ := c["system"].(string); sys == "" {
										if code, _ := c["code"].(string); code != "" {
											c["system"] = "http://terminology.hl7.org/CodeSystem/v3-RoleCode"
										}
									}
								}
							}
						}
					}
				}
			}

		case "DiagnosticReport":
			// identifier type enrichment — OBR.2 = Placer, OBR.3 = Filler per HL7 v2 spec.
			// The mapping engine populates values only; types are structurally known.
			s.enrichDiagnosticReportIdentifiers(r)
			// resultsInterpreter — OBR.32 XCN composite: ID^Last^First^...
			// The mapping engine may set display from subfield 1 only; reconstruct full name.
			s.enrichResultsInterpreter(r, request.ParsedHL7Data)
			// Strip empty identifiers
			if ids, ok := r["identifier"].([]interface{}); ok {
				var filtered []interface{}
				for _, id := range ids {
					if m, ok2 := id.(map[string]interface{}); ok2 && len(m) > 0 {
						filtered = append(filtered, id)
					}
				}
				if len(filtered) == 0 {
					delete(r, "identifier")
				} else {
					r["identifier"] = filtered
				}
			}
			// status: map raw HL7 Table 0123 codes (F/P/C/X/I/R/S) → FHIR
			// DiagnosticReportStatus.  OBRProcessor (AssembleORUObservations) also
			// performs this mapping; this clause is a defensive fallback for any
			// path that does not run through the segment processor stage.
			if st, ok := r["status"].(string); ok && st != "" {
				validFHIR := map[string]bool{
					"registered": true, "partial": true, "preliminary": true,
					"final": true, "amended": true, "corrected": true,
					"appended": true, "cancelled": true, "entered-in-error": true,
					"unknown": true,
				}
				if !validFHIR[strings.ToLower(st)] {
					r["status"] = hl7assembly.OBRStatusToDRStatus(st)
				}
			}
			// issued: format HL7 TS (YYYYMMDDHHMMSS) → FHIR instant.
			// ToISO is idempotent so this is safe even if assembly already ran.
			if issued, ok := r["issued"].(string); ok && issued != "" {
				r["issued"] = hl7assembly.ToISO(issued)
			}

		case "Observation":
			// status: map raw HL7 Table 0085 codes (F/P/C/X/I/W) → FHIR
			// ObservationStatus.  OBRProcessor (AssembleORUObservations) rebuilds
			// every Observation with a correctly-mapped status; this clause is a
			// defensive fallback for Observations produced by other paths.
			if st, ok := r["status"].(string); ok && st != "" {
				validFHIR := map[string]bool{
					"registered": true, "preliminary": true, "final": true,
					"amended": true, "corrected": true, "cancelled": true,
					"entered-in-error": true, "unknown": true,
				}
				if !validFHIR[strings.ToLower(st)] {
					r["status"] = hl7assembly.OBXStatusToObsStatus(st)
				}
			}

		case "Appointment":
			// ── start: required for any status other than "proposed" or "cancelled" ──
			// Constraint app-3: if start is absent the validator rejects booked/pending/
			// fulfilled etc. The OOB template maps SCH.11.4 → start, but many senders
			// put the appointment date only in AIP.6 (personnel start date/time) with
			// SCH.11 carrying only the duration (e.g. "30^MINUTES^ANS"). In that case
			// SCH.11.4 is empty and start is absent from the mapped resource. Derive
			// start from AIP.6 as fallback per the HL7 v2-to-FHIR IG ConceptMap.
			if _, hasStart := r["start"]; !hasStart {
				if aipSeg, ok2 := enhancedSegments["AIP"].(map[string]interface{}); ok2 {
					if fields, ok3 := aipSeg["fields"].([]interface{}); ok3 {
						for _, f := range fields {
							if fm, ok4 := f.(map[string]interface{}); ok4 {
								if k, _ := fm["key"].(string); k == "AIP.6" {
									if v, _ := fm["value"].(string); v != "" {
										if converted := tsToISODateTime(v); converted != "" {
											r["start"] = converted
										}
									}
									break
								}
							}
						}
					}
				}
			}
			// Convert any raw HL7 timestamp in start/end to FHIR instant format.
			for _, tf := range []string{"start", "end"} {
				if v, ok2 := r[tf].(string); ok2 && v != "" && !strings.Contains(v, "T") {
					if converted := tsToISODateTime(v); converted != "" {
						r[tf] = converted
					}
				}
			}
			// ── end: app-2 requires start and end both present or both absent ──────
			// When start is set but end is absent, compute end = start + duration.
			// Duration source priority: minutesDuration in resource (from SCH.9 / TQ1)
			// → AIP.9 (Duration NM per HL7 AIP definition) → AIP.7 (Start Date/Time
			// Offset NM, which some senders overload with a TQ-style duration composite
			// e.g. "30^MINUTES^ANS" when the explicit duration fields are absent).
			if startStr, hasStart := r["start"].(string); hasStart && startStr != "" {
				if _, hasEnd := r["end"]; !hasEnd {
					var durationMins int
					switch md := r["minutesDuration"].(type) {
					case int:
						durationMins = md
					case float64:
						durationMins = int(md)
					}
					if durationMins == 0 {
						// AIP.9 = Duration (authoritative), AIP.7 = Offset (fallback for
						// non-standard messages that pack duration into the offset field).
						if aipSeg, ok2 := enhancedSegments["AIP"].(map[string]interface{}); ok2 {
							if fields, ok3 := aipSeg["fields"].([]interface{}); ok3 {
								for _, candidate := range []string{"AIP.9", "AIP.7"} {
									if durationMins > 0 {
										break
									}
									for _, f := range fields {
										if fm, ok4 := f.(map[string]interface{}); ok4 {
											if k, _ := fm["key"].(string); k == candidate {
												v, _ := fm["value"].(string)
												if idx := strings.IndexByte(v, '^'); idx >= 0 {
													v = v[:idx]
												}
												if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
													durationMins = n
												}
												break
											}
										}
									}
								}
							}
						}
					}
					if durationMins > 0 {
						if t, err := time.Parse(time.RFC3339, startStr); err == nil {
							r["end"] = t.Add(time.Duration(durationMins) * time.Minute).Format(time.RFC3339)
						}
					}
				}
			}
			// ── appointmentType: add system only for standard HL7 Table 0277 codes ──
			// SCH.8 (CWE) uses Table 0277. Only the five standard codes are in the
			// published v2-0277 CodeSystem; local extensions (e.g. "CONSULT") must not
			// reference that system or the FHIR validator will reject the code as unknown.
			// Note: transformCEToCodeableConcept produces []map[string]interface{},
			// not []interface{} — both slice types must be handled.
			v2_0277Codes := map[string]bool{
				"ROUTINE": true, "WALKIN": true, "CHECKUP": true,
				"FOLLOWUP": true, "EMERGENCY": true,
			}
			if at, ok2 := r["appointmentType"].(map[string]interface{}); ok2 {
				var aptCodings []map[string]interface{}
				switch cv := at["coding"].(type) {
				case []interface{}:
					for _, ci := range cv {
						if cm, ok3 := ci.(map[string]interface{}); ok3 {
							aptCodings = append(aptCodings, cm)
						}
					}
				case []map[string]interface{}:
					aptCodings = cv
				}
				for _, c := range aptCodings {
					if sys, _ := c["system"].(string); sys == "" {
						if code, _ := c["code"].(string); v2_0277Codes[strings.ToUpper(code)] {
							c["system"] = "http://terminology.hl7.org/CodeSystem/v2-0277"
						}
					}
				}
			}

		case "MessageHeader":
			// definition: must be a canonical URL — drop raw HL7 composite values.
			if def, ok := r["definition"].(string); ok {
				if !strings.HasPrefix(def, "http://") && !strings.HasPrefix(def, "https://") && !strings.HasPrefix(def, "urn:") {
					delete(r, "definition")
				}
			}
			// response: drop when code is nil/empty (mfi_response_level returns nil for NE).
			// Also drop response.identifier when it is the HL7 encoding chars "^~\&".
			if resp, ok := r["response"].(map[string]interface{}); ok {
				code, _ := resp["code"].(string)
				if code == "" {
					delete(r, "response")
				} else {
					if id, ok2 := resp["identifier"].(string); ok2 {
						if strings.HasPrefix(id, "^~") || id == "" {
							delete(resp, "identifier")
						}
					}
				}
			}
			// meta.extension: Extension.url is required. Strip any extension that has no url
			// (e.g. HL7 version stored as {valueString: "2.5"} without a url identifier).
			if meta, ok := r["meta"].(map[string]interface{}); ok {
				if exts, ok2 := meta["extension"].([]interface{}); ok2 {
					var validExts []interface{}
					for _, extRaw := range exts {
						if ext, ok3 := extRaw.(map[string]interface{}); ok3 {
							if url, _ := ext["url"].(string); url != "" {
								validExts = append(validExts, extRaw)
							}
						}
					}
					if len(validExts) == 0 {
						delete(meta, "extension")
					} else {
						meta["extension"] = validExts
					}
				}
			}
			// meta.security: strip any entries that came from MSH.11 (Processing ID).
			// MSH.11 values (P/T/D) use system v2-0103 which is not in the FHIR
			// SecurityLabels value set — the HL7 v2-to-FHIR IG does not map MSH.11
			// to meta.security. Remove them so the validator does not emit a warning.
			// Match on either the explicit v2-0103 system or on system-less entries
			// whose code is a known Processing ID value (P=Production, T=Training, D=Debugging).
			v2ProcessingIDs := map[string]bool{"P": true, "T": true, "D": true}
			if meta, ok := r["meta"].(map[string]interface{}); ok {
				if secs, ok2 := meta["security"].([]interface{}); ok2 {
					var validSecs []interface{}
					for _, secRaw := range secs {
						if sec, ok3 := secRaw.(map[string]interface{}); ok3 {
							sys, _ := sec["system"].(string)
							code, _ := sec["code"].(string)
							if sys == "http://terminology.hl7.org/CodeSystem/v2-0103" {
								continue
							}
							if sys == "" && v2ProcessingIDs[strings.ToUpper(code)] {
								continue
							}
							validSecs = append(validSecs, secRaw)
						}
					}
					if len(validSecs) == 0 {
						delete(meta, "security")
					} else {
						meta["security"] = validSecs
					}
				}
			}
			// meta.tag: each tag.code must be a string — flatten CodeableConcept objects.
			if meta, ok := r["meta"].(map[string]interface{}); ok {
				if tags, ok2 := meta["tag"].([]interface{}); ok2 {
					for _, tagRaw := range tags {
						if tag, ok3 := tagRaw.(map[string]interface{}); ok3 {
							if codeVal, ok4 := tag["code"].(map[string]interface{}); ok4 {
								// Extract the code string from the nested object
								codeStr := ""
								if codings, ok5 := codeVal["coding"].([]interface{}); ok5 && len(codings) > 0 {
									if cod, ok6 := codings[0].(map[string]interface{}); ok6 {
										codeStr, _ = cod["code"].(string)
									}
								}
								if txt, ok5 := codeVal["text"].(string); ok5 && txt != "" {
									codeStr = txt
								}
								if codeStr != "" {
									tag["code"] = codeStr
								} else {
									delete(tag, "code")
								}
							}
						}
					}
				}
			}
			// ── Promote r["event"] → r["eventCoding"] ─────────────────────────────
			// Some v2.0 templates use FHIR path "event.code" / "event.display" which
			// the field mapper stores as r["event"]["code"]. FHIR R4 MessageHeader
			// requires the choice type eventCoding (or eventUri) — "event" alone is
			// not a valid top-level field and produces the "event[x] min=1, found=0"
			// validation error.  Promote when eventCoding is absent.
			if _, hasEC := r["eventCoding"]; !hasEC {
				if evMap, ok := r["event"].(map[string]interface{}); ok {
					if _, hasCode := evMap["code"]; hasCode {
						r["eventCoding"] = evMap
						delete(r, "event")
					}
				}
			}

			// eventCoding corrections:
			// 1. Ensure system is always set (some transform paths omit it).
			// 2. The V9 OOB template maps MSH.9.1 → code and MSH.9.2 → display which
			//    is wrong: v2-0003 CodeSystem contains trigger events (R01, A01), not
			//    message types (ORU, ADT). Detect this pattern and correct it.
			if ec, ok := r["eventCoding"].(map[string]interface{}); ok {
				code, _ := ec["code"].(string)
				display, _ := ec["display"].(string)

				// If code is a message type prefix and display is a trigger event, swap.
				msgTypePrefixes := map[string]bool{
					"ADT": true, "ORU": true, "ORM": true, "OML": true, "MDM": true,
					"BAR": true, "DFT": true, "MFN": true, "QRY": true, "SIU": true,
					"RDS": true, "ACK": true, "RAS": true,
					// Vaccination / immunization
					"VXU": true, "VXQ": true, "VXX": true, "VXR": true,
					// Scheduling / referral
					"SRM": true, "SRR": true, "REF": true,
					// Pharmacy
					"RXO": true, "RXR": true, "RXA": true,
				}
				if msgTypePrefixes[strings.ToUpper(code)] && display != "" {
					// display holds the actual trigger event (A01, R01, etc.)
					ec["code"] = display
					delete(ec, "display") // omit display — unknown string fails validator
				} else if strings.Contains(code, "^") {
					// MSH.9 composite: "MSG^T02" or "MSG^T02^MSG_T02" (3-component).
					// Trigger event is always the second component (index 1).
					// Use Split (not SplitN 2) so the third component is discarded.
					parts := strings.Split(code, "^")
					if len(parts) >= 2 {
						ec["code"] = parts[1]
					}
					delete(ec, "display")
				} else if msgTypePrefixes[strings.ToUpper(code)] && display == "" {
					// Message type in code with no display — re-derive trigger event from MSH.9.
					if mshSeg, ok2 := enhancedSegments["MSH"].(map[string]interface{}); ok2 {
						if fields, ok3 := mshSeg["fields"].([]interface{}); ok3 {
							for _, f := range fields {
								if fm, ok4 := f.(map[string]interface{}); ok4 {
									if k, _ := fm["key"].(string); k == "MSH.9" {
										if msh9, _ := fm["value"].(string); msh9 != "" {
											parts := strings.SplitN(msh9, "^", 3)
											if len(parts) >= 2 && parts[1] != "" {
												ec["code"] = parts[1]
											}
										}
										break
									}
								}
							}
						}
					}
				}

				if sys, _ := ec["system"].(string); sys == "" {
					ec["system"] = "http://terminology.hl7.org/CodeSystem/v2-0003"
				}
			}

			// ── Final fallback: synthesize eventCoding from MSH.9 ─────────────────
			// If neither the template mapping nor the event promotion produced an
			// eventCoding, build one directly from MSH.9 in the parsed HL7 data.
			// This guards against templates that have no MSH.9 → event mapping at all
			// (e.g. minimal custom templates) and ensures MessageHeader is always valid.
			if _, hasEC := r["eventCoding"]; !hasEC {
				if mshSeg, ok := enhancedSegments["MSH"].(map[string]interface{}); ok {
					msh9 := ""
					if fields, ok2 := mshSeg["fields"].([]interface{}); ok2 {
						for _, f := range fields {
							if fm, ok3 := f.(map[string]interface{}); ok3 {
								if k, _ := fm["key"].(string); k == "MSH.9" {
									msh9, _ = fm["value"].(string)
									break
								}
							}
						}
					}
					if msh9 != "" {
						parts := strings.SplitN(msh9, "^", 3)
						triggerEvent := ""
						if len(parts) > 1 && parts[1] != "" {
							triggerEvent = parts[1]
						} else {
							triggerEvent = parts[0]
						}
						r["eventCoding"] = map[string]interface{}{
							"system": "http://terminology.hl7.org/CodeSystem/v2-0003",
							"code":   triggerEvent,
						}
					}
				}
			}

			// Strip null-valued fields and empty arrays before any further MessageHeader
			// work — template mappings produce null when a source segment field is absent.
			cleanFHIRResource(r)

			// destination.endpoint is required (cardinality 1..1) by the FHIR R4 spec.
			// The OOB mapping sets destination.name from MSH.5 but not endpoint.
			// Inject a synthetic endpoint URI from available destination context.
			if destRaw, ok := r["destination"]; ok && destRaw != nil {
				// destination is a []interface{} of MessageHeader.destination objects
				var dests []interface{}
				switch dv := destRaw.(type) {
				case []interface{}:
					dests = dv
				case map[string]interface{}:
					dests = []interface{}{dv}
				}
				for _, di := range dests {
					if dm, ok2 := di.(map[string]interface{}); ok2 {
						if ep, _ := dm["endpoint"].(string); ep == "" {
							// Build endpoint URI from destination name/receiver name if available
							name, _ := dm["name"].(string)
							if name != "" {
								dm["endpoint"] = "urn:hl7:destination:" + strings.ToLower(
									strings.ReplaceAll(strings.TrimSpace(name), " ", "-"))
							} else {
								dm["endpoint"] = "urn:hl7:destination:unknown"
							}
						}
					}
				}
				if len(dests) > 0 {
					r["destination"] = dests
				} else {
					delete(r, "destination")
				}
			}
			// ── source.endpoint fallback ──────────────────────────────────────────
			// source.endpoint is required (1..1) by FHIR R4. The template maps
			// MSH.3→source.name and MSH.4→source.endpoint, but both may be stripped
			// by stripNullFields when those MSH fields are empty. Synthesize from
			// MSH context when absent or incomplete.
			var mshSrcName, mshSrcEndpoint string
			if mshSeg, ok2 := enhancedSegments["MSH"].(map[string]interface{}); ok2 {
				if fields, ok3 := mshSeg["fields"].([]interface{}); ok3 {
					for _, f := range fields {
						if fm, ok4 := f.(map[string]interface{}); ok4 {
							k, _ := fm["key"].(string)
							v, _ := fm["value"].(string)
							switch k {
							case "MSH.3":
								mshSrcName = v
							case "MSH.4":
								mshSrcEndpoint = v
							}
						}
					}
				}
			}
			srcEndpointURI := func(raw string) string {
				if raw == "" {
					return "urn:hl7:source:unknown"
				}
				return "urn:hl7:source:" + strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), " ", "-"))
			}
			if src, ok2 := r["source"].(map[string]interface{}); ok2 {
				if ep, _ := src["endpoint"].(string); ep == "" {
					if mshSrcEndpoint != "" {
						src["endpoint"] = srcEndpointURI(mshSrcEndpoint)
					} else {
						src["endpoint"] = srcEndpointURI(mshSrcName)
					}
				}
			} else {
				ep := srcEndpointURI(mshSrcEndpoint)
				if mshSrcEndpoint == "" {
					ep = srcEndpointURI(mshSrcName)
				}
				src := map[string]interface{}{"endpoint": ep}
				if mshSrcName != "" {
					src["name"] = mshSrcName
				}
				r["source"] = src
			}
			// implicitRules: MSH.1 "Field Separator" (always "|") is sometimes
			// mapped here by misconfigured templates. "|" is never a valid
			// canonical URI — remove unconditionally.
			delete(r, "implicitRules")

			// source.version: MSH.6 "Receiving Facility" is sometimes mapped
			// here by the heuristic. A facility name is not a software version.
			// Detect non-version strings and relocate to destination[0].endpoint
			// (the correct FHIR path for MSH.6), replacing the synthesized URN
			// that was injected when no real endpoint value was present.
			if src, ok2 := r["source"].(map[string]interface{}); ok2 {
				if ver, exists := src["version"]; exists {
					if verStr, ok3 := ver.(string); ok3 && verStr != "" && !looksLikeSoftwareVersion(verStr) {
						// Move to destination[0].endpoint
						if dests, ok4 := r["destination"].([]interface{}); ok4 && len(dests) > 0 {
							if dest0, ok5 := dests[0].(map[string]interface{}); ok5 {
								dest0["endpoint"] = verStr
							}
						}
						delete(src, "version")
					}
				}
			}

			// source.software: MSH.2 (encoding characters "^~\&") is sometimes
			// mapped to source.software by misconfigured templates. Strip it —
			// it is not a software name and fails FHIR string pattern validation.
			if src, ok2 := r["source"].(map[string]interface{}); ok2 {
				if sw, ok3 := src["software"].(string); ok3 {
					if strings.HasPrefix(sw, "^~") || strings.HasPrefix(sw, "^~\\&") {
						delete(src, "software")
					}
				}
			}
			// meta.lastUpdated: raw HL7 timestamps (e.g. "20260523120000") are not
			// valid FHIR instant values. Convert to ISO 8601 with timezone when the
			// value does not already contain a "T" separator (instant requires one).
			if meta, ok2 := r["meta"].(map[string]interface{}); ok2 {
				if lu, ok3 := meta["lastUpdated"].(string); ok3 && lu != "" && !strings.Contains(lu, "T") {
					if converted := tsToISODateTime(lu); converted != "" {
						meta["lastUpdated"] = converted
					}
				}
			}
			// focus: MessageHeader must reference the primary clinical resource
			// in the bundle so the validator can confirm every entry is reachable.
			// We defer this to after all resources are collected — see below.
		}
	}

	// Cross-resource wiring: wire Condition references into Encounter.diagnosis.
	// The OOB template creates the Encounter.diagnosis slot but cannot fill in the
	// condition reference because both resources are assembled independently.  We do
	// this after the normalizer loop so null-entry removal has already run.
	allResources = s.wireConditionsToEncounterDiagnosis(allResources)

	// Regenerate narratives after all normalizations so the human-readable text reflects
	// fully-normalized values (e.g. "male" not "M", "1960-01-01" not "19600101").
	// We only update resources that already have a text.div — newly assembled resources
	// (built by assembly.go) have their own purpose-built narrative and don't need this.
	for _, r := range allResources {
		rt, _ := r["resourceType"].(string)
		switch rt {
		case "Patient", "DiagnosticReport", "Observation", "Encounter", "MessageHeader",
			"Practitioner", "PractitionerRole", "RelatedPerson", "Coverage", "Condition":
			if _, hasText := r["text"]; hasText {
				narrative := s.generateNarrative(rt, r)
				r["text"] = map[string]interface{}{
					"status": "generated",
					"div":    strings.TrimSpace(narrative),
				}
			}
		}
	}

	// Inject MessageHeader.focus — must reference all primary context resources in the bundle
	// so that every included resource is reachable by forward traversal from MessageHeader.
	//
	// Semantic path (v1.1+ templates): derive focus from the template's "context" block via
	// contextLinks.RoleToSegment (e.g. "encounter"→"PV1") resolved through wellKnownSegmentToType
	// to FHIR resource types.  This is format-agnostic — no per-message-type code needed.
	//
	// Legacy path (v1.0 templates, nil contextLinks): prefer DiagnosticReport then Patient.
	// Last resort: first non-MessageHeader resource in the bundle.
	for _, r := range allResources {
		rt, _ := r["resourceType"].(string)
		if rt != "MessageHeader" {
			continue
		}
		if _, hasFocus := r["focus"]; hasFocus {
			continue // already set by mapping
		}
		var focusRefs []interface{}

		if contextLinks != nil && len(contextLinks.RoleToSegment) > 0 {
			// Semantic path: resolve each context role to its FHIR resource type, then
			// collect every bundle resource whose type is context-primary.
			primaryTypes := make(map[string]bool, len(contextLinks.RoleToSegment))
			for _, segment := range contextLinks.RoleToSegment {
				if fhirType, ok := wellKnownSegmentToType[segment]; ok {
					primaryTypes[fhirType] = true
				}
			}
			for _, candidate := range allResources {
				crt, _ := candidate["resourceType"].(string)
				if primaryTypes[crt] {
					if id, ok := candidate["id"].(string); ok && id != "" {
						focusRefs = append(focusRefs, map[string]interface{}{
							"reference": crt + "/" + id,
						})
					}
				}
			}
		}

		if len(focusRefs) == 0 {
			// Legacy fallback: prefer DiagnosticReport (ORU) then Patient (ADT/all others)
			for _, prio := range []string{"DiagnosticReport", "Patient"} {
				for _, candidate := range allResources {
					crt, _ := candidate["resourceType"].(string)
					if crt == prio {
						if id, ok := candidate["id"].(string); ok && id != "" {
							focusRefs = append(focusRefs, map[string]interface{}{
								"reference": crt + "/" + id,
							})
						}
						break
					}
				}
				if len(focusRefs) > 0 {
					break
				}
			}
		}

		if len(focusRefs) == 0 {
			// Last resort: first non-MessageHeader resource
			for _, candidate := range allResources {
				crt, _ := candidate["resourceType"].(string)
				if crt != "MessageHeader" {
					if id, ok := candidate["id"].(string); ok && id != "" {
						focusRefs = append(focusRefs, map[string]interface{}{
							"reference": crt + "/" + id,
						})
						break
					}
				}
			}
		}

		if len(focusRefs) > 0 {
			r["focus"] = focusRefs
		}
	}

	// Apply DB-driven cross-resource assembly rules (assembly_rules table, V114).
	// Handles reference wiring, focus expansion, result collection, etc. for all
	// message types without any per-message-type Go code changes.
	allResources = s.applyAssemblyRules(ctx, messageType, allResources)

	// Structural sanitization: remove empty sub-objects, coerce reference fields,
	// and inject required FHIR cardinality defaults that can't be expressed in templates.
	allResources = s.sanitizeFHIRResources(allResources)

	// ── Stage: Segment processors ─────────────────────────────────────────────
	// Runs AFTER all post-normalizers and sanitizeFHIRResources so that resources
	// added here (e.g. prior Patient from MRGProcessor) are not touched by the
	// PID/PV1 normalizer loops — those loops would otherwise copy surviving-patient
	// demographics onto the prior Patient record.
	//
	// Processors receive a ResourceCoordinator rather than a raw slice.  They use
	// Add / Get / Update / FindFirst and write "ResourceType/id" relative references;
	// the bundle assembler's rewriteReferences() resolves them to urn:uuid: after
	// UUID assignment — no processor ever needs to know a UUID in advance.
	//
	// ParsedHL7Data may carry enhancedSegments as map[string]hl7.EnhancedSegment
	// (typed Go map from hl7_parser_service.go) rather than map[string]interface{}.
	// We pass a shallow copy with enhancedSegments replaced by the already-converted
	// map[string]interface{} from extractEnhancedSegments() above.
	normalizedParsedData := make(map[string]interface{}, len(request.ParsedHL7Data))
	for k, v := range request.ParsedHL7Data {
		normalizedParsedData[k] = v
	}
	normalizedParsedData["enhancedSegments"] = enhancedSegments

	coord := segment_processors.NewCoordinator(allResources)
	procNames := manifest.SegmentProcessorNames(messageType)
	if len(procNames) > 0 {
		procCtx := &segment_processors.SegmentProcessorContext{
			Coordinator:   coord,
			ParsedHL7Data: normalizedParsedData,
			MessageType:   messageType,
			InterfaceID:   request.InterfaceID,
		}
		if warns := segment_processors.RunAll(procNames, procCtx); len(warns) > 0 {
			allWarnings = append(allWarnings, warns...)
		}
	}
	allResources = coord.All()

	// ── Stage: Optional segment blocks ───────────────────────────────────────
	// User-enabled additional segment→resource mappings.
	// Primary path:  request.AssemblyRules.Opt* fields set by the pipeline step
	//                Assembly tab toggles (stored in step.config.assemblyRules).
	// Fallback path: interface_message_mappings.custom_mapping_config["optional_segments"]
	//                for interfaces using the wizard / direct-API path.
	{
		optEnabled := map[string]bool{}
		if hl7assembly.OptRuleOn(request.AssemblyRules.OptPV1Encounter) {
			optEnabled["PV1_Encounter"] = true
		}
		if hl7assembly.OptRuleOn(request.AssemblyRules.OptSPMSpecimen) {
			optEnabled["SPM_Specimen"] = true
		}
		if hl7assembly.OptRuleOn(request.AssemblyRules.OptORCPractitioner) {
			optEnabled["ORC_OrderingPractitioner"] = true
		}
		// Fallback: DB config when no pipeline step config provided opt blocks.
		if len(optEnabled) == 0 {
			if dbCfg := s.loadOptionalSegmentConfig(ctx, request.InterfaceID, messageType); len(dbCfg) > 0 {
				optEnabled = dbCfg
			}
		}
		if len(optEnabled) > 0 {
			allResources = applyOptionalSegmentBlocks(allResources, request.ParsedHL7Data, messageType, optEnabled)
		}
	}

	// ── Stage: Focus augmentation ─────────────────────────────────────────────
	// Ensures MessageHeader.focus references every resource whose type appears in
	// the manifest's FocusTypes list.  Runs after segment processors so that
	// processor-added resources (e.g. the prior Patient from MRGProcessor) are
	// included.  Duplicate-safe: existing references are not re-added.
	// Uses "ResourceType/id" format — rewriteReferences() converts to urn:uuid:.
	augmentMessageHeaderFocus(allResources, manifest.GetFocusTypes(messageType))

	// ── Final cross-resource wiring ──────────────────────────────────────────
	// Must run AFTER all resource-generating stages (template mapping, segment
	// processors, optional blocks) so every resource type that ends up in the
	// bundle is present.  rewriteReferences() in the bundle assembler later
	// converts our "ResourceType/id" relative refs to urn:uuid: form.

	// Universal subject/patient/beneficiary wiring: every FHIR R4 resource
	// type that requires a Patient reference gets it here.  Idempotent — only
	// sets the field when it is absent or a display-only (no "reference" key).
	allResources = s.wireSubjectReferences(allResources)

	// Synthesize cardinality 1..1 required fields that have no direct HL7
	// source (DiagnosticReport.status, ChargeItemDefinition.url/status).
	allResources = s.synthesizeMissingRequiredFields(allResources)

	// Remove false-positive "Required field X missing" errors/warnings for fields
	// that composites or post-processing have now populated in the final resources.
	allErrors = filterResolvedRequiredFieldErrors(allErrors, allResources)
	allWarnings = filterResolvedRequiredFieldWarnings(allWarnings, allResources)

	// Populate response
	s.populateTransformResponse(
		response,
		allResources,
		stats,
		allWarnings,
		allErrors,
		transformStartTime,
		startTime,
		request.CreateBundle,
		fieldMappings,
	)

	log.Printf("✅ Atomic mapping transformation completed: %s (%d resources, %s)",
		response.RequestID, len(allResources), response.Performance.TotalTime)

	return response, nil
}

// =====================================
// ATOMIC MAPPING-DRIVEN RESOURCE CREATION
// =====================================

// NOTE: ORU^R01 structural assembly (OBX→Observations, DR.result[] linking) was
// previously here as postProcessORU. It is now in services/hl7assembly.AssembleORUObservations
// and runs either:
//   • Inline in Transform() when called directly (wizard / API), via SkipAssembly=false
//   • As a separate pipeline step "hl7.assemble_observations" at seq 110 in a full pipeline

// postProcessORU_PLACEHOLDER — intentionally removed; kept as a marker for git history.
// See services/hl7assembly/assembly.go → AssembleORUObservations
func (s *HL7FHIRTransformServiceV3) createResourceFromAtomicMappings(
	resourceType string,
	schema *fhir.FHIRSchema,
	enhancedSegments map[string]interface{},
	mappings []FieldMapping,
) (map[string]interface{}, []string, []string, int) {

	var warnings []string
	var errors []string

	log.Printf("📋 Creating %s using %d atomic mappings", resourceType, len(mappings))

	// Initialize base resource structure - MINIMAL, SCHEMA-COMPLIANT ONLY
	resource := map[string]interface{}{
		"resourceType": resourceType,
		"id":           fmt.Sprintf("%s-%d", strings.ToLower(resourceType), time.Now().UnixNano()),
	}

	mappedFieldCount := 0

	// Process each atomic mapping
	for i, mapping := range mappings {
		log.Printf("🔄 Processing mapping %d/%d: %s.%s.%s → %s.%s",
			i+1, len(mappings), mapping.SegmentName, mapping.HL7Field, mapping.HL7Component, resourceType, mapping.FHIRElementPath)

		// OBX.2 is a value-type control field — its value is injected into
		// OBX.5's TransformationRules at runtime; it has no direct FHIR target.
		// Skip any OBX.2 row that may exist in legacy OOB templates.
		if mapping.SegmentName == "OBX" && mapping.HL7Field == "2" {
			continue
		}

		// Evaluate mapping condition (e.g. "SCH.11.4 empty") before extracting value.
		// Conditions prevent fallback mappings from overwriting already-set fields.
		if cond, hasCond := mapping.TransformationRules["condition"].(string); hasCond && cond != "" {
			condParts := strings.Fields(cond)
			if len(condParts) == 2 {
				condSeg, condF, condC := s.parseHL7Path(condParts[0])
				condMapping := FieldMapping{SegmentName: condSeg, HL7Field: condF, HL7Component: condC}
				condVal, condFound := s.extractHL7ValueAtomic(enhancedSegments, condMapping)
				var isCondMet bool
				switch condParts[1] {
				case "empty":
					isCondMet = !condFound || condVal == ""
				case "present":
					isCondMet = condFound && condVal != ""
				}
				if !isCondMet {
					log.Printf("⏭️  Skipping mapping %s.%s → %s (condition '%s' not met)",
						mapping.SegmentName, mapping.HL7Field, mapping.FHIRElementPath, cond)
					continue
				}
			}
		}

		// Static value: skip HL7 extraction and use the literal directly.
		var hl7Value string
		if mapping.DataTypeTransform == "static_value" {
			if mapping.StaticValue == "" {
				log.Printf("⚠️  static_value mapping → %s.%s has no StaticValue set — skipping",
					resourceType, mapping.FHIRElementPath)
				continue
			}
			hl7Value = mapping.StaticValue
			log.Printf("📌 static_value: %s.%s = %q", resourceType, mapping.FHIRElementPath, hl7Value)
		} else {
			// Extract HL7 value using atomic extraction
			var found bool
			hl7Value, found = s.extractHL7ValueAtomic(enhancedSegments, mapping)
			if !found {
				if mapping.IsRequired {
					warnings = append(warnings, fmt.Sprintf("Required mapping %s.%s → %s.%s has no HL7 data",
						mapping.SegmentName, mapping.HL7Field, resourceType, mapping.FHIRElementPath))
				}
				log.Printf("⚠️  No HL7 data found for %s.%s.%s", mapping.SegmentName, mapping.HL7Field, mapping.HL7Component)
				continue
			}
			if hl7Value == "" {
				log.Printf("⚠️  Empty HL7 value for %s.%s", mapping.SegmentName, mapping.HL7Field)
				continue
			}
			log.Printf("✅ Found HL7 value: %s.%s = '%s'", mapping.SegmentName, mapping.HL7Field, hl7Value)
		}

		// OBX.5 value[x]: inject OBX.2 (data-type indicator) and OBX.6 (units)
		// into a fresh copy of TransformationRules so transformOBXValueByType can
		// dispatch to valueQuantity / valueString / valueCodeableConcept.
		if mapping.DataTypeTransform == "obx_value_by_type" {
			rules := make(map[string]interface{}, len(mapping.TransformationRules)+2)
			for k, v := range mapping.TransformationRules {
				rules[k] = v
			}
			if obxType := s.extractHL7FieldDirect(enhancedSegments, mapping.SegmentName, mapping.SegmentName+".2"); obxType != "" {
				rules["value_type"] = obxType
			}
			if obxUnit := s.extractHL7FieldDirect(enhancedSegments, mapping.SegmentName, mapping.SegmentName+".6"); obxUnit != "" {
				rules["unit"] = obxUnit
			}
			mapping.TransformationRules = rules
		}

		// Transform value using atomic transformation
		transformedValue, err := s.transformValueAtomic(hl7Value, mapping)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to transform %s.%s: %v",
				mapping.SegmentName, mapping.HL7Field, err))
			continue
		}

		// value[x] choice type: transformOBXValueByType returns {"valueXxx": ...}.
		// Merge the map directly into the resource instead of using the path setter,
		// which cannot resolve the polymorphic "[x]" path segment.
		if strings.HasSuffix(mapping.FHIRElementPath, "value[x]") {
			if valueMap, ok := transformedValue.(map[string]interface{}); ok {
				for k, v := range valueMap {
					resource[k] = v
				}
				mappedFieldCount++
				log.Printf("✅ Successfully mapped %s.%s → %s (value[x] merge)",
					mapping.SegmentName, mapping.HL7Field, mapping.FHIRElementPath)
				continue
			}
		}

		// Type coercion: the FHIR schema declares the target data type; use it to
		// convert plain HL7 strings into the correct JSON structure automatically.
		// This is the "straight map" principle — no template-level transform key
		// is needed when the type system can infer the correct wrapping.
		if strVal, ok := transformedValue.(string); ok && strVal != "" {
			switch strings.ToLower(mapping.FHIRDataType) {
			case "positiveint", "unsignedint", "integer":
				if n, err2 := strconv.Atoi(strVal); err2 == nil {
					transformedValue = n
				}
			case "codeableconcept":
				// HL7 code string → { "coding": [{ "code": "..." }] }
				// The template already does this for complex types via cx_to_identifier etc.,
				// but simple IS/CE/CWE code fields need this automatic wrapping.
				transformedValue = map[string]interface{}{
					"coding": []interface{}{
						map[string]interface{}{"code": strVal},
					},
				}
			case "reference":
				// HL7 name/id string → { "display": "..." }
				// Only wrap when the FHIR path targets the Reference object itself
				// (e.g. basedOn[0], reasonReference[0], slot[0]).
				// When the path already ends in ".display" the target is a plain string
				// leaf inside the Reference — wrapping would produce a doubly-nested
				// object (actor.display = {"display": "..."}) which discards the value.
				if !strings.HasSuffix(strings.ToLower(mapping.FHIRElementPath), ".display") {
					transformedValue = map[string]interface{}{"display": strVal}
				}
				// else: leave strVal as a plain string — path points to the .display leaf
			}
		}

		// Set in resource using atomic field setting
		err = s.setAtomicFieldInResource(resource, mapping.FHIRElementPath, transformedValue, schema)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to set %s: %v", mapping.FHIRElementPath, err))
			continue
		}

		mappedFieldCount++
		log.Printf("✅ Successfully mapped %s.%s → %s.%s",
			mapping.SegmentName, mapping.HL7Field, resourceType, mapping.FHIRElementPath)
	}

	// Only return resource if we successfully mapped some fields
	if mappedFieldCount == 0 {
		log.Printf("⚠️ Skipping %s - no HL7 data was successfully mapped", resourceType)
		return nil, warnings, errors, 0
	}

	// ✅ Generate human-readable narrative based on mapped data
	textPath := fmt.Sprintf("%s.text", resourceType)
	if _, exists := schema.Elements[textPath]; exists {
		narrative := s.generateNarrative(resourceType, resource)
		// Remove any leading/trailing whitespace
		narrative = strings.TrimSpace(narrative)
		log.Printf("🔍 Generated narrative length: %d chars, first 100: %s", len(narrative), narrative[:min(100, len(narrative))])
		resource["text"] = map[string]interface{}{
			"status": "generated",
			"div":    narrative,
		}
	}

	// ✅ SCHEMA-DRIVEN: Ensure all required fields exist before validation
	s.ensureRequiredFieldsFromSchema(resource, schema, &warnings, &errors)

	// Validate against schema - STRICT validation before returning
	validationErrors := s.validateResourceAgainstSchema(resource, schema, &warnings)
	if len(validationErrors) > 0 {
		// Add validation errors to errors list
		errors = append(errors, validationErrors...)
		log.Printf("❌ %s failed schema validation with %d errors", resourceType, len(validationErrors))
	}

	log.Printf("✅ Created %s with %d mapped fields", resourceType, mappedFieldCount)
	return resource, warnings, errors, mappedFieldCount
}
func (s *HL7FHIRTransformServiceV3) getFieldMappings(ctx context.Context, messageType, profile string, request ...*TransformRequest) ([]FieldMapping, *ContextLinks, error) {
	// Determine up-front whether this interface uses the live OOB template.
	// OOB interfaces must always fall through to STEP 2 so that template updates
	// are reflected immediately — stale snapshots (embedded_mappings,
	// interfaces.transformation_mapping) must be bypassed for them.
	isPureOOB := false
	if len(request) > 0 && request[0] != nil && request[0].InterfaceID != "" && s.db != nil {
		isPureOOB = s.isInterfacePureOOB(ctx, request[0].InterfaceID, messageType)
		if isPureOOB {
			log.Printf("ℹ️ V3 Service: Interface %s/%s is pure-OOB — skipping stale snapshots", request[0].InterfaceID, messageType)
		}
	}

	// STEP 0: Use embedded_mappings from pipeline step config when present.
	// Skipped for OOB interfaces — live OOB template must always win.
	if !isPureOOB && len(request) > 0 && request[0] != nil && len(request[0].EmbeddedMappings) > 0 {
		mappings := s.extractFieldMappingsFromWizardArray(request[0].EmbeddedMappings)
		if len(mappings) > 0 {
			log.Printf("✅ V3 Service: Using %d embedded_mappings from pipeline step config for %s", len(mappings), messageType)
			return mappings, nil, nil
		}
	}

	if s.db == nil {
		log.Printf("⚠️ V3 Service: Database not available, using default mappings for %s", messageType)
		return s.getDefaultMappingsForTesting(messageType), nil, nil
	}

	log.Printf("🔍 V3 Service: Loading mappings for message type: %s", messageType)

	// STEP 1a: interface_message_mappings — wizard-saved custom overrides keyed by
	// (interface_id, message_type).  Checked first when the request carries an
	// interfaceID so that a user's wizard customisations always beat the OOB template.
	// Only applies when uses_standard_template = false (custom config is present).
	if len(request) > 0 && request[0] != nil && request[0].InterfaceID != "" {
		mappings, err := s.loadFromInterfaceMessageMappings(ctx, request[0].InterfaceID, messageType)
		if err == nil && len(mappings) > 0 {
			log.Printf("✅ V3 Service: Loaded %d custom mappings from interface_message_mappings for %s/%s",
				len(mappings), request[0].InterfaceID, messageType)
			return mappings, nil, nil
		}
	}

	// STEP 1a+: Delta model — OOB template + sparse interface-level overrides.
	// Applies when uses_standard_template = true AND mapping_overrides IS NOT NULL.
	// Returns the OOB base merged with the stored delta (replace / add / remove ops).
	if len(request) > 0 && request[0] != nil && request[0].InterfaceID != "" {
		mappings, err := s.loadDeltaMappings(ctx, request[0].InterfaceID, messageType)
		if err == nil && len(mappings) > 0 {
			log.Printf("✅ V3 Service: Loaded %d merged delta mappings for %s/%s",
				len(mappings), request[0].InterfaceID, messageType)
			return mappings, nil, nil
		}
	}

	// STEP 1b: Interface-specific mappings from legacy interfaces.transformation_mapping column.
	// Skipped for OOB interfaces — live OOB template must always win over stale legacy snapshots.
	if !isPureOOB {
		mappings, err := s.loadInterfaceSpecificMappings(ctx, messageType)
		if err == nil && len(mappings) > 0 {
			log.Printf("✅ V3 Service: Loaded %d mappings from interface-specific configuration for %s", len(mappings), messageType)
			return mappings, nil, nil // custom mappings carry no contextLinks
		}
	}

	// STEP 2: OOB templates — exact message_type match.
	// Must be tried BEFORE the interface-id fallback so that a known type (e.g.
	// MFN^M02) gets its correct OOB template rather than the wrong interface
	// mapping stored for a structurally different sibling type (e.g. MFN^M13).
	log.Printf("ℹ️ V3 Service: No interface-specific mapping for %s, trying OOB templates...", messageType)
	hl7Version := "2.5"
	if len(request) > 0 && request[0] != nil && request[0].ParsedHL7Data != nil {
		if v, ok := request[0].ParsedHL7Data["version"].(string); ok && v != "" {
			hl7Version = v
		}
	}
	fhirVersion := "R4"
	if len(request) > 0 && request[0] != nil && request[0].FHIRVersion != "" {
		fhirVersion = request[0].FHIRVersion
	}
	mappings, cl, err := s.loadFromV9OOBTemplates(ctx, messageType, hl7Version, fhirVersion)
	if err == nil && len(mappings) > 0 {
		log.Printf("✅ V3 Service: Loaded %d mappings from V9 OOB templates for %s", len(mappings), messageType)
		return mappings, cl, nil
	}

	// STEP 3: Interface-id fallback — only for minor trigger-event variants where
	// no OOB template exists and the incoming type shares the same message-code
	// family as the configured interface (e.g. ORU^R03 on an ORU^R01 interface).
	// Explicitly excluded: structurally heterogeneous families like MFN (each event
	// has different payload segments) and ACK.
	if len(request) > 0 && request[0] != nil && request[0].InterfaceID != "" {
		configuredType := s.getInterfaceMessageType(ctx, request[0].InterfaceID)
		if sameMessageFamily(messageType, configuredType) &&
			!isStructurallyHeterogeneousFamily(messageType) {
			// Prefer the OOB template for the configured type (e.g. ORU^R01) over the
			// stale wizard snapshot in interfaces.transformation_mapping.  This ensures
			// minor event variants (R03, R32, …) get the same well-tested field mappings
			// as the primary event type rather than a potentially stale saved snapshot.
			if oobM, oobCL, oobErr := s.loadFromV9OOBTemplates(ctx, configuredType, hl7Version, fhirVersion); oobErr == nil && len(oobM) > 0 {
				log.Printf("✅ V3 Service: Loaded %d OOB mappings via family fallback %s → %s",
					len(oobM), messageType, configuredType)
				return oobM, oobCL, nil
			}
			// No OOB template for the family type either — fall back to wizard snapshot.
			mappings, err = s.loadInterfaceSpecificMappingsByID(ctx, request[0].InterfaceID)
			if err == nil && len(mappings) > 0 {
				log.Printf("✅ V3 Service: Loaded %d mappings by interface_id %s (minor variant fallback: %s → %s)",
					len(mappings), request[0].InterfaceID, messageType, configuredType)
				return mappings, nil, nil
			}
		} else {
			log.Printf("ℹ️ V3 Service: Skipping interface-id fallback — %s and configured %s are in different families or heterogeneous family",
				messageType, configuredType)
		}
	}

	log.Printf("⚠️ V3 Service: V9 OOB templates not found, trying V5 legacy schema...")

	// STEP 2: Fallback to V5 legacy schema (existing field_element_mappings)
	query := `
		SELECT segment_name, hl7_field,
		       COALESCE(hl7_component, '') as hl7_component,
		       fhir_resource_type, fhir_element_path,
		       COALESCE(data_type_transform, '') as data_type_transform,
		       is_required,
		       COALESCE(transformation_rules, '{}') as transformation_rules
		FROM field_element_mappings fem
		WHERE EXISTS (
			SELECT 1 FROM message_fhir_templates mft
			WHERE mft.message_type = $1
			AND mft.fhir_resources::jsonb ? fem.fhir_resource_type
		)
		ORDER BY fem.fhir_resource_type, fem.segment_name, fem.hl7_field
	`

	rows, err := s.db.QueryContext(ctx, query, messageType)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query field mappings: %w", err)
	}
	defer rows.Close()

	var legacyMappings []FieldMapping
	for rows.Next() {
		var mapping FieldMapping
		var component sql.NullString
		var dataTypeTransform sql.NullString
		var rulesJSON sql.NullString

		err := rows.Scan(
			&mapping.SegmentName,
			&mapping.HL7Field,
			&component,
			&mapping.FHIRResourceType,
			&mapping.FHIRElementPath,
			&dataTypeTransform,
			&mapping.IsRequired,
			&rulesJSON,
		)
		if err != nil {
			continue
		}

		if component.Valid {
			mapping.HL7Component = component.String
		}
		if dataTypeTransform.Valid {
			mapping.DataTypeTransform = dataTypeTransform.String
		}

		if rulesJSON.Valid && rulesJSON.String != "" && rulesJSON.String != "{}" {
			var rules map[string]interface{}
			if err := json.Unmarshal([]byte(rulesJSON.String), &rules); err == nil {
				mapping.TransformationRules = rules
			}
		}

		legacyMappings = append(legacyMappings, mapping)
	}

	if len(legacyMappings) > 0 {
		log.Printf("📊 V3 Service: Found %d legacy field mappings for message type %s", len(legacyMappings), messageType)
	} else {
		log.Printf("⚠️ V3 Service: No mappings found in either V9 or V5 schemas for message type %s", messageType)
	}

	return legacyMappings, nil, nil
}

// loadFromInterfaceMessageMappings reads the custom_mapping_config stored by the wizard
// into the interface_message_mappings table for a specific (interface_id, message_type)
// pair.  Only returns mappings when uses_standard_template = false and a non-empty
// custom_mapping_config is present — if the row says uses_standard_template = true the
// caller should fall through to the OOB template path (Step 2).
// isInterfacePureOOB returns true when the interface+messageType record in
// interface_message_mappings has uses_standard_template = true, or when no
// record exists yet (interface has never been wizard-customised).
// OOB interfaces must bypass stale embedded_mappings / transformation_mapping
// snapshots so that live OOB template changes take effect immediately.
func (s *HL7FHIRTransformServiceV3) isInterfacePureOOB(ctx context.Context, interfaceID, messageType string) bool {
	var usesStandard bool
	err := s.db.QueryRowContext(ctx, `
		SELECT uses_standard_template
		FROM interface_message_mappings
		WHERE interface_id = $1 AND message_type = $2
		LIMIT 1
	`, interfaceID, messageType).Scan(&usesStandard)
	if err == sql.ErrNoRows {
		return true // no wizard customisation → treat as OOB
	}
	if err != nil {
		return false // be conservative on error
	}
	return usesStandard
}

// loadResourcePolicy reads the "resource_policy" key from
// interface_message_mappings.custom_mapping_config for the given interface and
// message type.  Returns nil (no overrides) when the interface has no custom
// config, no resource_policy key, or when the DB is unavailable.
//
// The policy map is keyed by FHIR resourceType with values "allow" or "suppress".
func (s *HL7FHIRTransformServiceV3) loadResourcePolicy(ctx context.Context, interfaceID, messageType string) map[string]string {
	if interfaceID == "" || s.db == nil {
		return nil
	}
	query := `
		SELECT custom_mapping_config
		FROM interface_message_mappings
		WHERE interface_id = $1 AND message_type = $2
		  AND custom_mapping_config IS NOT NULL
		LIMIT 1
	`
	var configBytes []byte
	if err := s.db.QueryRowContext(ctx, query, interfaceID, messageType).Scan(&configBytes); err != nil {
		return nil
	}
	var config struct {
		ResourcePolicy map[string]string `json:"resource_policy"`
	}
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil
	}
	return config.ResourcePolicy
}

// loadOptionalSegmentConfig reads the "optional_segments" key from
// interface_message_mappings.custom_mapping_config.
// Returns nil when the interface has no custom config or no optional_segments key.
func (s *HL7FHIRTransformServiceV3) loadOptionalSegmentConfig(ctx context.Context, interfaceID, messageType string) map[string]bool {
	if interfaceID == "" || s.db == nil {
		return nil
	}
	query := `
		SELECT custom_mapping_config
		FROM interface_message_mappings
		WHERE interface_id = $1 AND message_type = $2
		  AND custom_mapping_config IS NOT NULL
		LIMIT 1
	`
	var configBytes []byte
	if err := s.db.QueryRowContext(ctx, query, interfaceID, messageType).Scan(&configBytes); err != nil {
		return nil
	}
	return manifest.LoadOptionalSegmentConfig(configBytes)
}

// applyOptionalSegmentBlocks runs each enabled optional segment block and
// appends any produced resources to allResources.
// It also wires DiagnosticReport.encounter when PV1_Encounter is enabled and
// a DiagnosticReport already exists in the resource list.
func applyOptionalSegmentBlocks(
	allResources []map[string]interface{},
	parsedHL7Data map[string]interface{},
	messageType string,
	enabled map[string]bool,
) []map[string]interface{} {

	if len(enabled) == 0 {
		return allResources
	}

	// Find the Patient ref once — used by all assemblers that need a subject.
	patientRef := ""
	for _, r := range allResources {
		if rt, _ := r["resourceType"].(string); rt == "Patient" {
			if id, _ := r["id"].(string); id != "" {
				patientRef = "Patient/" + id
				break
			}
		}
	}

	if manifest.IsBlockEnabled(enabled, "PV1_Encounter") {
		// Only assemble if no Encounter already exists (idempotent).
		hasEncounter := false
		for _, r := range allResources {
			if rt, _ := r["resourceType"].(string); rt == "Encounter" {
				hasEncounter = true
				break
			}
		}
		if !hasEncounter {
			enc, encID := hl7assembly.AssemblePV1Encounter(parsedHL7Data, patientRef)
			if enc != nil {
				allResources = append(allResources, enc)
				log.Printf("ℹ️  Optional PV1_Encounter block: Encounter/%s assembled for %s", encID, messageType)

				// Wire DiagnosticReport.encounter if present.
				for _, r := range allResources {
					if rt, _ := r["resourceType"].(string); rt == "DiagnosticReport" {
						r["encounter"] = map[string]interface{}{
							"reference": "Encounter/" + encID,
						}
						break
					}
				}
			}
		}
	}

	// SPM_Specimen and ORC_OrderingPractitioner assemblers will be added here
	// when their hl7assembly functions are implemented.

	return allResources
}

func (s *HL7FHIRTransformServiceV3) loadFromInterfaceMessageMappings(ctx context.Context, interfaceID, messageType string) ([]FieldMapping, error) {
	query := `
		SELECT custom_mapping_config
		FROM interface_message_mappings
		WHERE interface_id = $1
		  AND message_type = $2
		  AND uses_standard_template = false
		  AND custom_mapping_config IS NOT NULL
		LIMIT 1
	`
	var configBytes []byte
	err := s.db.QueryRowContext(ctx, query, interfaceID, messageType).Scan(&configBytes)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no custom mapping in interface_message_mappings for %s/%s", interfaceID, messageType)
		}
		return nil, fmt.Errorf("interface_message_mappings query failed: %w", err)
	}

	// The wizard stores {messageType, atomicMappings: [...], ...}
	var config map[string]interface{}
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil, fmt.Errorf("failed to parse custom_mapping_config: %w", err)
	}

	// Extract atomicMappings array
	atomicRaw, ok := config["atomicMappings"]
	if !ok {
		return nil, fmt.Errorf("no atomicMappings in custom_mapping_config")
	}
	atomicSlice, ok := atomicRaw.([]interface{})
	if !ok || len(atomicSlice) == 0 {
		return nil, fmt.Errorf("empty atomicMappings in custom_mapping_config")
	}

	// Convert to []map[string]interface{} for extractFieldMappingsFromWizardArray
	var mappingArray []map[string]interface{}
	for _, item := range atomicSlice {
		if m, ok := item.(map[string]interface{}); ok {
			mappingArray = append(mappingArray, m)
		}
	}

	return s.extractFieldMappingsFromWizardArray(mappingArray), nil
}

// loadInterfaceSpecificMappingsByID loads mappings from a specific interface by its UUID.
// Used as a fallback when exact message_type lookup fails (e.g. ORU^R03 arriving on an ORU^R01 interface).
func (s *HL7FHIRTransformServiceV3) loadInterfaceSpecificMappingsByID(ctx context.Context, interfaceID string) ([]FieldMapping, error) {
	query := `
		SELECT transformation_mapping
		FROM interfaces
		WHERE id = $1
		  AND transformation_mapping IS NOT NULL
		  AND transformation_mapping != '{}'
	`
	var mappingJSONBytes []byte
	err := s.db.QueryRowContext(ctx, query, interfaceID).Scan(&mappingJSONBytes)
	if err != nil {
		return nil, fmt.Errorf("no mappings for interface %s: %w", interfaceID, err)
	}
	// double-decode: JSONB may be a JSON-encoded string
	var jsonString string
	var mappingArray []map[string]interface{}
	if err := json.Unmarshal(mappingJSONBytes, &jsonString); err == nil {
		if err2 := json.Unmarshal([]byte(jsonString), &mappingArray); err2 != nil {
			return nil, fmt.Errorf("parse error for interface %s: %w", interfaceID, err2)
		}
	} else {
		if err2 := json.Unmarshal(mappingJSONBytes, &mappingArray); err2 != nil {
			return nil, fmt.Errorf("parse error for interface %s: %w", interfaceID, err2)
		}
	}
	return s.extractFieldMappingsFromWizardArray(mappingArray), nil
}

// getInterfaceMessageType returns the message_type configured for an interface.
// Returns empty string if the interface is not found or has no message type.
func (s *HL7FHIRTransformServiceV3) getInterfaceMessageType(ctx context.Context, interfaceID string) string {
	var msgType string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(message_type, '') FROM interfaces WHERE id = $1`, interfaceID,
	).Scan(&msgType)
	if err != nil {
		return ""
	}
	return msgType
}

// sameMessageFamily reports whether two message types share the same HL7 message
// code (the part before "^").  For example "ORU^R01" and "ORU^R03" are in the
// same family; "MFN^M02" and "ADT^A01" are not.
// An empty configuredType (interface has no stored message type) returns false.
func sameMessageFamily(incoming, configured string) bool {
	if incoming == "" || configured == "" {
		return false
	}
	inFamily := strings.ToUpper(strings.SplitN(incoming, "^", 2)[0])
	cfFamily := strings.ToUpper(strings.SplitN(configured, "^", 2)[0])
	return inFamily == cfFamily
}

// isStructurallyHeterogeneousFamily reports whether a message family has
// structurally different payload segments across its trigger events.  Such
// families must NOT use the interface-id fallback because a mapping for one
// event type would silently produce an incorrect (or empty) bundle for another.
//
// MFN: each M0x event targets a different master file with different segments
//
//	(M02=STF staff, M04=CDM charge, M05=LOC location, M13=Z-segments, etc.)
//
// ACK: acknowledgement — no payload segments, not a data message.
func isStructurallyHeterogeneousFamily(messageType string) bool {
	family := strings.ToUpper(strings.SplitN(messageType, "^", 2)[0])
	switch family {
	case "MFN", "ACK", "QBP", "RSP":
		return true
	}
	return false
}

// loadInterfaceSpecificMappings loads mappings from the transformation_mapping field in interfaces table
func (s *HL7FHIRTransformServiceV3) loadInterfaceSpecificMappings(ctx context.Context, messageType string) ([]FieldMapping, error) {
	log.Printf("🎯 V3 Service: Loading interface-specific mappings for message type: %s", messageType)

	// Query for interfaces that have this message type configured
	query := `
		SELECT id, transformation_mapping
		FROM interfaces
		WHERE message_type = $1
		  AND transformation_mapping IS NOT NULL
		  AND transformation_mapping != '{}'
		  AND is_active = true
		ORDER BY updated_at DESC
		LIMIT 1
	`

	var interfaceID string
	var mappingJSONBytes []byte

	log.Printf("🔍 V3 Service: Executing query for message type: %s", messageType)
	err := s.db.QueryRowContext(ctx, query, messageType).Scan(&interfaceID, &mappingJSONBytes)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("ℹ️ V3 Service: No interface-specific mappings found for message type: %s", messageType)
			return nil, fmt.Errorf("no interface-specific mappings found")
		}
		log.Printf("❌ V3 Service: Query/Scan error: %v", err)
		return nil, fmt.Errorf("failed to query interface mappings: %w", err)
	}

	log.Printf("📋 V3 Service: Found interface-specific mappings in interface: %s", interfaceID)
	log.Printf("🔍 V3 Service: Scanned %d bytes from database", len(mappingJSONBytes))

	// Parse the transformation_mapping JSON - wizard saves it as an array
	log.Printf("🔍 V3 Service: Raw mapping JSON length: %d bytes", len(mappingJSONBytes))
	log.Printf("🔍 V3 Service: First 100 chars: %s", string(mappingJSONBytes[:min(100, len(mappingJSONBytes))]))

	// JSONB from PostgreSQL comes as a JSON-encoded string, need to double-decode
	var jsonString string
	if err := json.Unmarshal(mappingJSONBytes, &jsonString); err != nil {
		// If it's not a string, try direct array parse (for backward compatibility)
		var mappingArray []map[string]interface{}
		if err := json.Unmarshal(mappingJSONBytes, &mappingArray); err != nil {
			log.Printf("❌ V3 Service: Failed to parse as string or array: %v", err)
			return nil, fmt.Errorf("failed to parse transformation_mapping JSON: %w", err)
		}
		log.Printf("✅ V3 Service: Parsed array directly with %d items", len(mappingArray))
		mappings := s.extractFieldMappingsFromWizardArray(mappingArray)
		return mappings, nil
	}

	// Now parse the string content as array
	var mappingArray []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonString), &mappingArray); err != nil {
		log.Printf("❌ V3 Service: Failed to parse string content as array: %v", err)
		return nil, fmt.Errorf("failed to parse transformation_mapping JSON content: %w", err)
	}

	log.Printf("✅ V3 Service: Parsed array with %d items", len(mappingArray))
	if len(mappingArray) > 0 {
		log.Printf("🔍 V3 Service: First item keys: %v", getMapKeys(mappingArray[0]))
	}

	// Extract field mappings from the wizard array format
	mappings := s.extractFieldMappingsFromWizardArray(mappingArray)

	if len(mappings) > 0 {
		log.Printf("✅ V3 Service: Extracted %d field mappings from interface configuration", len(mappings))
	} else {
		log.Printf("⚠️ V3 Service: extractFieldMappingsFromWizardArray returned 0 mappings from %d input items", len(mappingArray))
	}

	return mappings, nil
}

// loadFromV9OOBTemplates loads field mappings (and optional context links) from
// the V9 OOB template stored in hl7_fhir_templates for the given message type.
// The second return value is non-nil only for v1.1+ templates that declare a
// "context" block; callers receive nil for v1.0 templates and fall back to the
// existing hardcoded context-wiring path.
func (s *HL7FHIRTransformServiceV3) loadFromV9OOBTemplates(ctx context.Context, messageType, hl7Version, fhirVersion string) ([]FieldMapping, *ContextLinks, error) {
	log.Printf("🎯 V3 Service: Loading V9 OOB template for message type: %s (HL7 v%s → FHIR %s)", messageType, hl7Version, fhirVersion)

	// Prefer exact (hl7_version, fhir_version) match.
	// Fall back first on fhir_version (same HL7 version, any FHIR), then fully generic.
	query := `
		SELECT template_config, template_description, template_name
		FROM hl7_fhir_templates
		WHERE message_type = $1 AND is_default = true
		ORDER BY
		    CASE WHEN hl7_version = $2  THEN 0 ELSE 1 END +
		    CASE WHEN fhir_version = $3 THEN 0 ELSE 2 END,
		    created_at DESC
		LIMIT 1
	`

	var templateConfig string
	var templateDescription string
	var templateName string

	err := s.db.QueryRowContext(ctx, query, messageType, hl7Version, fhirVersion).Scan(&templateConfig, &templateDescription, &templateName)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("ℹ️ V3 Service: No OOB template found for message type: %s", messageType)
			return nil, nil, fmt.Errorf("no OOB template found for message type: %s", messageType)
		}
		return nil, nil, fmt.Errorf("failed to query OOB template: %w", err)
	}

	log.Printf("📋 V3 Service: Found OOB template: %s (%s)", templateName, templateDescription)
	log.Printf("🔍 V3 Service: Template config size: %d bytes", len(templateConfig))

	// Parse the template config JSON
	var templateData map[string]interface{}
	if err := json.Unmarshal([]byte(templateConfig), &templateData); err != nil {
		log.Printf("❌ V3 Service: Failed to parse template JSON: %v", err)
		log.Printf("🔍 V3 Service: Template config sample (first 200 chars): %s", templateConfig[:min(200, len(templateConfig))])
		return nil, nil, fmt.Errorf("failed to parse V9 template config: %w", err)
	}

	log.Printf("✅ V3 Service: Template JSON parsed successfully")
	log.Printf("🔍 V3 Service: Template top-level keys: %v", getKeys(templateData))

	// Extract context links (v1.1+ templates only; nil for v1.0)
	cl := extractContextLinks(templateData)
	if cl != nil {
		log.Printf("🔗 V3 Service: Template v1.1 — extracted context links for %d roles, %d resources",
			len(cl.RoleToSegment), len(cl.ResourceLinks))
	}

	// Convert V9 OOB template format to V3 FieldMapping format
	mappings, err := s.convertV9TemplateToFieldMappings(templateData)
	if err != nil {
		log.Printf("❌ V3 Service: Failed to convert template: %v", err)
		return nil, nil, fmt.Errorf("failed to convert V9 template to field mappings: %w", err)
	}

	log.Printf("✅ V3 Service: Converted V9 template to %d field mappings", len(mappings))
	return mappings, cl, nil
}

// convertV9TemplateToFieldMappings converts our V9 OOB template format to V3 FieldMapping format.
// Supports both v1.0 (plain mappings) and v2.0 (extended with composites + valueSets).
func (s *HL7FHIRTransformServiceV3) convertV9TemplateToFieldMappings(templateData map[string]interface{}) ([]FieldMapping, error) {
	var mappings []FieldMapping

	// ── Extract valueSets from profile v2.0 (used for lookup: transforms) ────
	// Shape: templateData["valueSets"] = { "siuFillerStatus": { "BOOKED": "booked", ... }, ... }
	valueSets := map[string]map[string]interface{}{}
	if vsRaw, ok := templateData["valueSets"]; ok {
		if vsMap, ok := vsRaw.(map[string]interface{}); ok {
			for vsName, vsEntriesRaw := range vsMap {
				if vsEntries, ok := vsEntriesRaw.(map[string]interface{}); ok {
					valueSets[vsName] = vsEntries
				}
			}
		}
		log.Printf("🔧 V3 Service: Loaded %d valueSets from profile", len(valueSets))
	}

	// ── Extract resources from template — try "resources" then "mappings" ────
	var resourcesInterface interface{}
	var ok bool
	resourcesInterface, ok = templateData["resources"]
	if !ok {
		resourcesInterface, ok = templateData["mappings"]
		if !ok {
			return nil, fmt.Errorf("no resources or mappings found in V9 template")
		}
		log.Printf("🔧 V3 Service: Using legacy 'mappings' key format")
	} else {
		log.Printf("🔧 V3 Service: Using new 'resources' key format")
	}

	resources, ok := resourcesInterface.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid resources format in V9 template")
	}

	// Process each resource type (Patient, Appointment, etc.)
	for resourceType, resourceDataInterface := range resources {
		resourceData, ok := resourceDataInterface.(map[string]interface{})
		if !ok {
			log.Printf("⚠️ V3 Service: Skipping invalid resource data for %s", resourceType)
			continue
		}

		// Extract mappings array from resource
		mappingsInterface, ok := resourceData["mappings"]
		if !ok {
			log.Printf("ℹ️ V3 Service: No mappings found for resource %s", resourceType)
			continue
		}

		resourceMappings, ok := mappingsInterface.([]interface{})
		if !ok {
			log.Printf("⚠️ V3 Service: Invalid mappings format for resource %s", resourceType)
			continue
		}

		// Convert each atomic field mapping
		for _, mappingInterface := range resourceMappings {
			mapping, ok := mappingInterface.(map[string]interface{})
			if !ok {
				continue
			}

			hl7Path, ok := mapping["hl7Path"].(string)
			if !ok {
				continue
			}
			fhirPath, ok := mapping["fhirPath"].(string)
			if !ok {
				continue
			}

			// SI (Sequence ID) is a positional counter within a message segment group
			// (e.g. OBR.1 = "1", OBX.1 = "1"…"50"). It must never overwrite a FHIR
			// resource id — FHIR ids are system-generated stable identifiers.
			if hl7DataType, _ := mapping["hl7DataType"].(string); strings.EqualFold(hl7DataType, "SI") {
				bare := fhirPath
				if dot := strings.LastIndex(bare, "."); dot >= 0 {
					bare = bare[dot+1:]
				}
				if strings.EqualFold(bare, "id") {
					log.Printf("⏭️  Skipping SI field %s → %s (sequence ID is not a resource identifier)", hl7Path, fhirPath)
					continue
				}
			}

			segmentName, hl7Field, hl7Component := s.parseHL7Path(hl7Path)

			transformFunc, _ := mapping["transform"].(string)
			required, _ := mapping["required"].(bool)
			confidence, _ := mapping["confidence"].(float64)

			transformRules := map[string]interface{}{
				"confidence":     confidence,
				"sourceTemplate": "V9_OOB",
				"transform":      transformFunc,
			}

			// Capture inline valueMap from the template mapping (e.g. {"M":"male","F":"female"}).
			// Used as a fallback in transformValueAtomic when the DB has no entry for the code.
			if vm, ok := mapping["valueMap"]; ok {
				transformRules["valueMap"] = vm
			}

			// Inline valueSet for lookup: transforms so the switch default can resolve it
			if strings.HasPrefix(transformFunc, "lookup:") {
				vsName := strings.TrimPrefix(transformFunc, "lookup:")
				if vs, found := valueSets[vsName]; found {
					transformRules["valueSet"] = vs
				}
				// fallbackTransform / fallbackField — used when SCH.25 is absent
				if ft, _ := mapping["fallbackTransform"].(string); ft != "" {
					transformRules["fallbackTransform"] = ft
				}
				if ff, _ := mapping["fallbackField"].(string); ff != "" {
					transformRules["fallbackField"] = ff
				}
			}

			// hl7Table: unambiguous HL7-to-FHIR table annotation (e.g. "0002" for marital status).
			// Carried into transformCEToCodeableConcept so it can inject the FHIR system URI
			// for codes that carry no CE.3 component.
			if tableNum, ok := mapping["hl7Table"].(string); ok && tableNum != "" {
				transformRules["hl7Table"] = tableNum
			}

			// condition: only emit the mapping when the HL7 condition field is non-empty
			if cond, _ := mapping["condition"].(string); cond != "" {
				transformRules["condition"] = cond
			}

			// staticValue: literal constant injected directly into the FHIR field.
			// SegmentName/HL7Field are intentionally empty for these mappings.
			staticValue, _ := mapping["staticValue"].(string)

			fieldMapping := FieldMapping{
				SegmentName:         segmentName,
				HL7Field:            hl7Field,
				HL7Component:        hl7Component,
				FHIRResourceType:    resourceType,
				FHIRElementPath:     fhirPath,
				DataTypeTransform:   transformFunc,
				StaticValue:         staticValue,
				IsRequired:          required,
				Confidence:          confidence,
				TransformationRules: transformRules,
			}

			mappings = append(mappings, fieldMapping)
		}
	}

	log.Printf("🔄 V3 Service: Converted %d V9 mappings to FieldMapping format", len(mappings))
	return mappings, nil
}

// =====================================
// CONTEXT LINKS — template v1.1 support
// =====================================

// ContextLinks holds the v1.1 template context declarations.
//
// Template "context" block:
//
//	{ "patient": "PID", "encounter": "PV1", "order": "ORC" }
//
// Template per-resource "contextLinks":
//
//	"Encounter": { "contextLinks": { "subject": "patient" } }
//	"AllergyIntolerance": { "contextLinks": { "patient": "patient", "encounter": "encounter" } }
//
// The engine resolves each role to the FHIR resource that was built from the
// named segment, then writes a Reference into the declared FHIR field.
// Templates that omit the "context" block (v1.0) return nil and the existing
// hardcoded context-wiring path continues to handle those messages.
type ContextLinks struct {
	// RoleToSegment maps a context role name to the HL7 segment that is the
	// authoritative source for that role (e.g. "patient" → "PID").
	RoleToSegment map[string]string
	// ResourceLinks maps each FHIR resource type to a (FHIR field → role name)
	// map (e.g. "Encounter" → {"subject": "patient"}).
	ResourceLinks map[string]map[string]string
}

// wellKnownSegmentToType maps common HL7 segment names to the FHIR resource
// type that the mapping engine builds from that segment.  Used by applyContextLinks
// to resolve a role's segment name to the correct resource type when looking up
// the already-built resource ID.
var wellKnownSegmentToType = map[string]string{
	"PID": "Patient",
	"PV1": "Encounter",
	"ORC": "ServiceRequest",
	"OBR": "DiagnosticReport",
	"AL1": "AllergyIntolerance",
	"DG1": "Condition",
	"IN1": "Coverage",
	"NK1": "RelatedPerson",
	"OBX": "Observation",
	"TXA": "DocumentReference",
	"RXA": "Immunization",
	"STF": "Practitioner",
	"SCH": "Appointment",
	"AIS": "AppointmentResponse",
	"FT1": "ChargeItem",
	"PR1": "Procedure",
	"ROL": "PractitionerRole",
	"GT1": "RelatedPerson",
}

// extractContextLinks parses the "context" and per-resource "contextLinks" blocks
// from a parsed template JSON object.  Returns nil when the template is v1.0
// (no "context" block present) — callers fall back to existing hardcoded wiring.
func (s *HL7FHIRTransformServiceV3) applyContextLinks(resources []map[string]interface{}, cl *ContextLinks) []map[string]interface{} {
	if cl == nil || len(cl.ResourceLinks) == 0 {
		return resources
	}

	// Resolve each role to "ResourceType/id" using wellKnownSegmentToType
	roleToRef := make(map[string]string)
	for role, segment := range cl.RoleToSegment {
		resourceType, ok := wellKnownSegmentToType[segment]
		if !ok {
			continue
		}
		for _, r := range resources {
			rt, _ := r["resourceType"].(string)
			if rt != resourceType {
				continue
			}
			if id, ok := r["id"].(string); ok && id != "" {
				roleToRef[role] = resourceType + "/" + id
				break
			}
		}
	}

	if len(roleToRef) == 0 {
		return resources
	}

	// Wire declared fields on each resource
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		links, ok := cl.ResourceLinks[rt]
		if !ok {
			continue
		}
		for fhirField, role := range links {
			ref, ok := roleToRef[role]
			if !ok {
				continue
			}
			// Only set when the field is absent or empty — never overwrite engine output
			switch existing := r[fhirField].(type) {
			case nil:
				r[fhirField] = map[string]interface{}{"reference": ref}
				log.Printf("🔗 ContextLinks: %s.%s → %s (role: %s)", rt, fhirField, ref, role)
			case string:
				if existing == "" {
					r[fhirField] = map[string]interface{}{"reference": ref}
					log.Printf("🔗 ContextLinks: %s.%s → %s (role: %s)", rt, fhirField, ref, role)
				}
			case map[string]interface{}:
				if len(existing) == 0 {
					r[fhirField] = map[string]interface{}{"reference": ref}
					log.Printf("🔗 ContextLinks: %s.%s → %s (role: %s)", rt, fhirField, ref, role)
				}
			}
		}
	}

	return resources
}

// parseHL7Path parses HL7 path like "PID.3.1" into segment, field, and component
func (s *HL7FHIRTransformServiceV3) initializeResponse(request *TransformRequest) *TransformResponse {
	requestID := request.RequestID
	if requestID == "" {
		requestID = fmt.Sprintf("transform_v3_%d", time.Now().UnixNano())
	}

	return &TransformResponse{
		Success:        false,
		RequestID:      requestID,
		FHIRResources:  []map[string]interface{}{},
		ResourceCounts: make(map[string]int),
		Warnings:       []string{},
		Errors:         []string{},
		MappingStats:   MappingStatistics{},
		Performance:    PerformanceMetrics{},
	}
}

func (s *HL7FHIRTransformServiceV3) extractMessageType(parsedHL7 map[string]interface{}) (string, error) {
	// Log the structure for debugging
	log.Printf("🔍 V3 Service: Parsing message type from HL7 data structure")
	log.Printf("🔍 V3 Service: Available top-level keys: %v", getMapKeys(parsedHL7))

	// Debug: Check messageType field structure
	if messageType, exists := parsedHL7["messageType"]; exists {
		log.Printf("🔍 V3 Service: messageType field exists, type: %T, value: %v", messageType, messageType)
	}

	// Try direct access first (parsedHL7Data is already the .data portion)
	if messageType, exists := parsedHL7["messageType"].(map[string]interface{}); exists {
		if name, nameOk := messageType["name"].(string); nameOk {
			log.Printf("✅ V3 Service: Found message type (direct): %s", name)
			return name, nil
		}
		log.Printf("🔍 V3 Service: messageType exists but no name field. Keys: %v", getMapKeys(messageType))
	}

	// Try direct string access (in case it's just a string)
	if messageType, exists := parsedHL7["messageType"].(string); exists {
		log.Printf("✅ V3 Service: Found message type (string): %s", messageType)
		return messageType, nil
	}

	// Try with "data" wrapper (in case full structure is passed)
	if data, ok := parsedHL7["data"].(map[string]interface{}); ok {
		if messageType, exists := data["messageType"].(map[string]interface{}); exists {
			if name, nameOk := messageType["name"].(string); nameOk {
				log.Printf("✅ V3 Service: Found message type (nested): %s", name)
				return name, nil
			}
		}
	}

	// Try as struct with Name field (when read from MongoDB as Go struct)
	if messageTypeVal, exists := parsedHL7["messageType"]; exists {
		// Use reflection to extract Name field from struct
		if msgTypeStruct, ok := messageTypeVal.(map[string]interface{}); ok {
			if name, nameOk := msgTypeStruct["Name"].(string); nameOk {
				log.Printf("✅ V3 Service: Found message type from struct Name field: %s", name)
				return name, nil
			}
		}
		// Try reflection for actual struct types
		val := reflect.ValueOf(messageTypeVal)
		if val.Kind() == reflect.Struct {
			nameField := val.FieldByName("Name")
			if nameField.IsValid() && nameField.Kind() == reflect.String {
				name := nameField.String()
				log.Printf("✅ V3 Service: Found message type via reflection: %s", name)
				return name, nil
			}
		}
	}

	// Try basicSegments MSH.9 as fallback
	if basicSegments, ok := parsedHL7["basicSegments"].(map[string]interface{}); ok {
		if msh, mshOk := basicSegments["MSH"].(map[string]interface{}); mshOk {
			if fields, fieldsOk := msh["fields"].(map[string]interface{}); fieldsOk {
				if msh9, msh9Ok := fields["MSH.9"].(string); msh9Ok {
					log.Printf("✅ V3 Service: Found message type from MSH.9: %s", msh9)
					return msh9, nil
				}
			}
		}
	}

	log.Printf("❌ V3 Service: Message type extraction failed. Available keys: %v", getMapKeys(parsedHL7))
	return "", fmt.Errorf("message type not found in parsed HL7 data")
}

// Helper function to get map keys for debugging
func (s *HL7FHIRTransformServiceV3) extractEnhancedSegments(parsedHL7 map[string]interface{}) map[string]interface{} {
	// DEBUG: Check what type enhancedSegments actually is
	if rawSegments, exists := parsedHL7["enhancedSegments"]; exists {
		log.Printf("🔍 V3 Service: enhancedSegments exists, type: %T", rawSegments)

		// Try direct access first (parsedHL7Data is already the .data portion)
		if enhancedSegments, ok := rawSegments.(map[string]interface{}); ok {
			log.Printf("✅ V3 Service: Found enhanced segments (direct): %d segments", len(enhancedSegments))
			return enhancedSegments
		}

		// If it's a different map type (like map[string]hl7.EnhancedSegment), convert via JSON
		log.Printf("⚠️ V3 Service: enhancedSegments is not map[string]interface{}, attempting JSON conversion")
		jsonBytes, err := json.Marshal(rawSegments)
		if err != nil {
			log.Printf("❌ V3 Service: Failed to marshal enhancedSegments: %v", err)
		} else {
			var converted map[string]interface{}
			err = json.Unmarshal(jsonBytes, &converted)
			if err != nil {
				log.Printf("❌ V3 Service: Failed to unmarshal enhancedSegments: %v", err)
			} else {
				log.Printf("✅ V3 Service: Successfully converted enhancedSegments via JSON: %d segments", len(converted))
				return converted
			}
		}
	} else {
		log.Printf("❌ V3 Service: enhancedSegments key does not exist")
	}

	// Try with "data" wrapper (in case full structure is passed)
	if data, ok := parsedHL7["data"].(map[string]interface{}); ok {
		if enhancedSegments, ok := data["enhancedSegments"].(map[string]interface{}); ok {
			log.Printf("✅ V3 Service: Found enhanced segments (nested): %d segments", len(enhancedSegments))
			return enhancedSegments
		}
	}

	log.Printf("❌ V3 Service: Enhanced segments not found. Available keys: %v", getMapKeys(parsedHL7))
	return nil
}

// convertToAtomicMappings converts FieldMapping structs to AtomicMapping format for frontend
func (s *HL7FHIRTransformServiceV3) convertToAtomicMappings(fieldMappings []FieldMapping) []AtomicMapping {
	atomicMappings := make([]AtomicMapping, len(fieldMappings))

	for i, fieldMapping := range fieldMappings {
		// Static value mappings have no HL7 source — SourcePath stays empty.
		var sourcePath string
		if fieldMapping.DataTypeTransform != "static_value" {
			sourcePath = fieldMapping.SegmentName + "." + fieldMapping.HL7Field
			if fieldMapping.HL7Component != "" {
				sourcePath += "." + fieldMapping.HL7Component
			}
			if fieldMapping.HL7SubComponent != "" {
				sourcePath += "." + fieldMapping.HL7SubComponent
			}
		}

		atomicMappings[i] = AtomicMapping{
			ID:               fmt.Sprintf("%d", fieldMapping.ID),
			SourcePath:       sourcePath,
			TargetPath:       fieldMapping.FHIRElementPath,
			ResourceType:     fieldMapping.FHIRResourceType,
			FHIRResourceType: fieldMapping.FHIRResourceType,
			TransformType:    fieldMapping.DataTypeTransform,
			DefaultValue:     fieldMapping.StaticValue, // carried to UI for static_value display
			IsRequired:       fieldMapping.IsRequired,
			Confidence:       fieldMapping.Confidence,
		}
	}

	log.Printf("✅ Converted %d FieldMappings to AtomicMappings for frontend", len(atomicMappings))
	return atomicMappings
}

// GetFieldMappingsPublic is a public wrapper so external packages (e.g. the delta
// controller) can call getFieldMappings without duplicating the resolution logic.
func (s *HL7FHIRTransformServiceV3) GetFieldMappingsPublic(ctx context.Context, messageType, profile string, req *TransformRequest) ([]FieldMapping, *ContextLinks, error) {
	return s.getFieldMappings(ctx, messageType, profile, req)
}

// ConvertToAtomicMappingsPublic is a public wrapper for the frontend-facing conversion.
func (s *HL7FHIRTransformServiceV3) ConvertToAtomicMappingsPublic(fieldMappings []FieldMapping) []AtomicMapping {
	return s.convertToAtomicMappings(fieldMappings)
}

func (s *HL7FHIRTransformServiceV3) populateTransformResponse(
	response *TransformResponse,
	resources []map[string]interface{},
	stats MappingStatistics,
	warnings, errors []string,
	transformStartTime, startTime time.Time,
	createBundle bool,
	fieldMappings []FieldMapping,
) {
	// Strip engine artifact: mapping engine sometimes adds a nested sub-object
	// whose key equals the resource type (e.g. resource["Observation"] = {...}).
	// Remove it so validators and consumers see a clean resource.
	for _, resource := range resources {
		if rt, ok := resource["resourceType"].(string); ok && rt != "" {
			delete(resource, rt)
		}
	}

	response.FHIRResources = resources
	response.MappingStats = stats
	response.Warnings = append(response.Warnings, warnings...)
	response.Errors = append(response.Errors, errors...)
	response.Performance.TransformTime = time.Since(transformStartTime).String()

	// Convert field mappings to atomic mappings for frontend
	response.AtomicMappings = s.convertToAtomicMappings(fieldMappings)

	for _, resource := range resources {
		if resourceType, ok := resource["resourceType"].(string); ok {
			response.ResourceCounts[resourceType]++
		}
	}

	log.Printf("🔍 [DEBUG V3] createBundle=%v, resources count=%d", createBundle, len(resources))
	if createBundle && len(resources) > 0 {
		bundle := s.createBundle(resources, response.RequestID, response.MessageType)
		response.Bundle = bundle
		bundleJSON, _ := json.Marshal(bundle)
		log.Printf("🔍 [DEBUG V3] Bundle created! Size: %d bytes, resourceType: %v", len(bundleJSON), bundle["resourceType"])
	} else {
		log.Printf("⚠️  [DEBUG V3] Bundle NOT created - createBundle=%v, resources=%d", createBundle, len(resources))
	}

	// Success if we produced resources, even when schema validation emits warnings.
	// Schema validation errors are advisory — they don't mean the message was unprocessable.
	response.Success = len(resources) > 0 || len(response.Errors) == 0
	response.Performance.TotalTime = time.Since(startTime).String()
	response.Performance.ResourcesCreated = len(resources)
}

// bundleIDSanitizer strips characters that are invalid in a FHIR id ([A-Za-z0-9\-\.]{1,64}).
var bundleIDSanitizer = regexp.MustCompile(`[^A-Za-z0-9\-\.]`)

func (s *HL7FHIRTransformServiceV3) createBundle(resources []map[string]interface{}, requestID, messageType string) map[string]interface{} {
	// FHIR message bundles MUST have MessageHeader as the first entry (bdl-12).
	// Sort: MessageHeader first, then all other resource types in their original order.
	sorted := make([]map[string]interface{}, 0, len(resources))
	var rest []map[string]interface{}
	for _, r := range resources {
		if rt, _ := r["resourceType"].(string); rt == "MessageHeader" {
			sorted = append(sorted, r)
		} else {
			rest = append(rest, r)
		}
	}
	sorted = append(sorted, rest...)

	// Assign a urn:uuid: fullUrl to every entry and rewrite all internal
	// ResourceType/id references to their urn:uuid: equivalents.
	// FHIR spec (3.3.2.1): when fullUrl is urn:uuid:, relative references like
	// "Patient/id" are resolved against a base of "urn:uuid:" — they never match.
	// Every internal reference MUST use the same urn:uuid: that was assigned here.
	// Shared with the CDA→FHIR pipeline — see fhir/r4/bundle_assembler.go.
	entries := r4.AssembleEntries(sorted)

	// FHIR id must match [A-Za-z0-9\-\.]{1,64}.  The requestID may contain underscores
	// (e.g. "transform_v3_1775471815062417349") — strip them and truncate.
	cleanID := bundleIDSanitizer.ReplaceAllString("bundle-"+requestID, "")
	if len(cleanID) > 64 {
		cleanID = cleanID[:64]
	}

	return map[string]interface{}{
		"resourceType": "Bundle",
		"id":           cleanID,
		"type":         "message",
		"timestamp":    time.Now().Format(time.RFC3339),
		"entry":        entries,
	}
}

// =====================================
// DATA TYPE ENRICHMENT
// =====================================

// enrichMappingsWithDataTypes populates HL7DataType and FHIRDataType on each mapping
// by querying the HL7 real schema loader and the FHIR schema loader.
// This is a best-effort operation: any lookup failure is silently skipped so that
// the rest of the transformation continues normally.
func (s *HL7FHIRTransformServiceV3) enrichMappingsWithDataTypes(mappings []FieldMapping, messageType string) []FieldMapping {
	hl7Loader := hl7.GetRealSchemaLoader()

	enriched := make([]FieldMapping, len(mappings))
	copy(enriched, mappings)

	for i := range enriched {
		m := &enriched[i]

		// ── HL7 data type lookup ──────────────────────────────────────────────────
		// Skip sub-component paths (HL7Component != "") — extractHL7ValueAtomic
		// already returns the component as a plain string.  Applying the parent
		// field's composite type (e.g. XPN for PID.5, CX for PID.3, XAD for PID.11)
		// to that component string would cause AutoTranslate to produce structured
		// objects at scalar FHIR paths (e.g. name[0].family = {family:…, use:…}).
		if m.HL7DataType == "" && m.HL7Component == "" && hl7Loader != nil && m.SegmentName != "" && m.HL7Field != "" {
			dt := hl7Loader.GetFieldDataType(messageType, m.SegmentName, m.HL7Field)
			if dt != "" {
				m.HL7DataType = dt
			}
		}

		// ── FHIR data type lookup ─────────────────────────────────────────────────
		if m.FHIRDataType == "" && s.fhirLoader != nil && m.FHIRResourceType != "" && m.FHIRElementPath != "" {
			schema, err := s.fhirLoader.LoadFHIRSchema(m.FHIRResourceType, "base", "R4")
			if err == nil && schema != nil {
				// Build the element path key as stored in schema.Elements, e.g. "Patient.birthDate"
				elementKey := m.FHIRResourceType + "." + m.FHIRElementPath
				// Strip array indices for schema lookup: name[0].given → name.given
				cleanKey := stripArrayIndices(elementKey)
				if elem, ok := schema.Elements[cleanKey]; ok && elem != nil {
					m.FHIRDataType = elem.DataType
				}
				// Try without resource prefix if not found
				if m.FHIRDataType == "" {
					cleanPath := stripArrayIndices(m.FHIRElementPath)
					if elem, ok := schema.Elements[cleanPath]; ok && elem != nil {
						m.FHIRDataType = elem.DataType
					}
				}
			}
		}
	}

	return enriched
}

// stripArrayIndices removes array index notation from FHIR paths for schema lookup.
// e.g. "name[0].given[1]" → "name.given"
func stripArrayIndices(path string) string {
	var b strings.Builder
	inBracket := false
	for _, r := range path {
		switch r {
		case '[':
			inBracket = true
		case ']':
			inBracket = false
		default:
			if !inBracket {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// =====================================
// DEFAULT MAPPINGS FOR TESTING
// =====================================

func (s *HL7FHIRTransformServiceV3) getDefaultMappingsForTesting(messageType string) []FieldMapping {
	log.Printf("🧪 V3 Service: Providing default test mappings for %s", messageType)

	// Return a basic set of common ADT^A01 mappings for testing
	defaultMappings := []FieldMapping{
		// Patient mappings
		{ID: 1, SegmentName: "PID", HL7Field: "3", HL7Component: "1", FHIRResourceType: "Patient", FHIRElementPath: "identifier[0].value", DataTypeTransform: "cx_to_identifier", IsRequired: true},
		{ID: 2, SegmentName: "PID", HL7Field: "5", HL7Component: "1", FHIRResourceType: "Patient", FHIRElementPath: "name[0].family", DataTypeTransform: "xpn_to_humanname", IsRequired: true},
		{ID: 3, SegmentName: "PID", HL7Field: "5", HL7Component: "2", FHIRResourceType: "Patient", FHIRElementPath: "name[0].given[0]", DataTypeTransform: "xpn_to_humanname", IsRequired: true},
		{ID: 4, SegmentName: "PID", HL7Field: "5", HL7Component: "3", FHIRResourceType: "Patient", FHIRElementPath: "name[0].given[1]", DataTypeTransform: "xpn_to_humanname", IsRequired: false},
		{ID: 5, SegmentName: "PID", HL7Field: "7", HL7Component: "", FHIRResourceType: "Patient", FHIRElementPath: "birthDate", DataTypeTransform: "ts_to_date", IsRequired: false},
		{ID: 6, SegmentName: "PID", HL7Field: "8", HL7Component: "", FHIRResourceType: "Patient", FHIRElementPath: "gender", DataTypeTransform: "gender_mapping", IsRequired: false},
		{ID: 7, SegmentName: "PID", HL7Field: "11", HL7Component: "", FHIRResourceType: "Patient", FHIRElementPath: "address[0]", DataTypeTransform: "xad_to_address", IsRequired: false},
		{ID: 8, SegmentName: "PID", HL7Field: "13", HL7Component: "", FHIRResourceType: "Patient", FHIRElementPath: "telecom[0]", DataTypeTransform: "xtn_to_contactpoint", IsRequired: false},

		// MessageHeader mappings
		{ID: 9, SegmentName: "MSH", HL7Field: "9", HL7Component: "", FHIRResourceType: "MessageHeader", FHIRElementPath: "eventCoding", DataTypeTransform: "msh9_trigger_event_to_coding", IsRequired: true},
		{ID: 10, SegmentName: "MSH", HL7Field: "3", HL7Component: "", FHIRResourceType: "MessageHeader", FHIRElementPath: "source.name", DataTypeTransform: "", IsRequired: false},
		{ID: 11, SegmentName: "MSH", HL7Field: "4", HL7Component: "", FHIRResourceType: "MessageHeader", FHIRElementPath: "source.software", DataTypeTransform: "", IsRequired: false},
		{ID: 12, SegmentName: "MSH", HL7Field: "10", HL7Component: "", FHIRResourceType: "MessageHeader", FHIRElementPath: "response.identifier", DataTypeTransform: "control_id_to_reference", IsRequired: false},

		// Encounter mappings
		{ID: 13, SegmentName: "PV1", HL7Field: "2", HL7Component: "", FHIRResourceType: "Encounter", FHIRElementPath: "class.code", DataTypeTransform: "", IsRequired: false},
		{ID: 14, SegmentName: "PV1", HL7Field: "3", HL7Component: "1", FHIRResourceType: "Encounter", FHIRElementPath: "location[0].location.identifier.value", DataTypeTransform: "", IsRequired: false},
		{ID: 15, SegmentName: "PV1", HL7Field: "3", HL7Component: "2", FHIRResourceType: "Encounter", FHIRElementPath: "location[0].location.display", DataTypeTransform: "", IsRequired: false},
		{ID: 16, SegmentName: "PV1", HL7Field: "3", HL7Component: "3", FHIRResourceType: "Encounter", FHIRElementPath: "location[0].physicalType.coding[0].display", DataTypeTransform: "", IsRequired: false},

		// Event mappings
		{ID: 17, SegmentName: "EVN", HL7Field: "1", HL7Component: "", FHIRResourceType: "MessageHeader", FHIRElementPath: "eventCoding.code", DataTypeTransform: "", IsRequired: false},
		{ID: 18, SegmentName: "EVN", HL7Field: "1", HL7Component: "", FHIRResourceType: "MessageHeader", FHIRElementPath: "eventCoding.display", DataTypeTransform: "", IsRequired: false},
		{ID: 19, SegmentName: "EVN", HL7Field: "2", HL7Component: "", FHIRResourceType: "MessageHeader", FHIRElementPath: "meta.lastUpdated", DataTypeTransform: "ts_to_datetime", IsRequired: false},
	}

	log.Printf("✅ V3 Service: Returning %d default test mappings for %s", len(defaultMappings), messageType)
	return defaultMappings
}

// MappingOverride is a single delta entry stored in interface_message_mappings.mapping_overrides.
type MappingOverride struct {
	Action      string  `json:"action"`               // "replace" | "add" | "remove"
	HL7Path     string  `json:"hl7Path"`              // "PID.5.1" — segment.field[.component]; empty for static_value
	FHIRPath    string  `json:"fhirPath,omitempty"`   // full FHIR target path incl. resource type
	Transform   string  `json:"transform,omitempty"`  // transform key
	StaticValue string  `json:"staticValue,omitempty"` // literal constant for transform == "static_value"
	IsRequired  bool    `json:"isRequired,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
}

// MappingDelta is the envelope stored in mapping_overrides.
type MappingDelta struct {
	Version        int               `json:"version"`
	BaseTemplateID string            `json:"base_template_id"`
	BasedOnVersion string            `json:"based_on_version"` // profile_version of the OOB template at save time
	Overrides      []MappingOverride `json:"overrides"`
}

// hl7PathKey returns the canonical lookup key for a FieldMapping: "SEG.field[.component]".
func hl7PathKey(fm FieldMapping) string {
	if fm.HL7Component != "" {
		return fm.SegmentName + "." + fm.HL7Field + "." + fm.HL7Component
	}
	return fm.SegmentName + "." + fm.HL7Field
}

// mergeMappings applies delta overrides on top of the OOB base []FieldMapping and
// returns the merged slice.  Base is never mutated.
func mergeMappings(base []FieldMapping, delta *MappingDelta) []FieldMapping {
	// Index base by HL7 path for O(1) lookup.
	indexed := make(map[string]int, len(base)) // path → index in result
	result := make([]FieldMapping, len(base))
	copy(result, base)
	for i, fm := range result {
		indexed[hl7PathKey(fm)] = i
	}

	for _, ov := range delta.Overrides {
		switch ov.Action {
		case "replace":
			if idx, ok := indexed[ov.HL7Path]; ok {
				fm := result[idx]
				if ov.FHIRPath != "" {
					parts := strings.SplitN(ov.FHIRPath, ".", 2)
					if len(parts) == 2 {
						fm.FHIRResourceType = parts[0]
						fm.FHIRElementPath = parts[1]
					}
				}
				if ov.Transform != "" {
					fm.DataTypeTransform = ov.Transform
				}
				if ov.Confidence > 0 {
					fm.Confidence = ov.Confidence
				}
				fm.IsRequired = ov.IsRequired
				result[idx] = fm
			}

		case "add":
			// Use FHIRPath as the dedup key for static_value mappings (HL7Path is "").
			addKey := ov.HL7Path
			if ov.Transform == "static_value" && addKey == "" {
				addKey = "static:" + ov.FHIRPath
			}
			if _, exists := indexed[addKey]; !exists {
				var seg, field, comp string
				if ov.Transform != "static_value" {
					parts := strings.Split(ov.HL7Path, ".")
					if len(parts) < 2 {
						continue
					}
					seg, field = parts[0], parts[1]
					if len(parts) > 2 {
						comp = parts[2]
					}
				}
				fhirResource, fhirElem := "", ov.FHIRPath
				if fp := strings.SplitN(ov.FHIRPath, ".", 2); len(fp) == 2 {
					fhirResource, fhirElem = fp[0], fp[1]
				}
				newFM := FieldMapping{
					SegmentName:       seg,
					HL7Field:          field,
					HL7Component:      comp,
					FHIRResourceType:  fhirResource,
					FHIRElementPath:   fhirElem,
					DataTypeTransform: ov.Transform,
					StaticValue:       ov.StaticValue,
					IsRequired:        ov.IsRequired,
					Confidence:        ov.Confidence,
				}
				indexed[addKey] = len(result)
				result = append(result, newFM)
			}

		case "remove":
			if idx, ok := indexed[ov.HL7Path]; ok {
				// Nil-out the entry; compact below.
				result[idx] = FieldMapping{} // sentinel: SegmentName == ""
				delete(indexed, ov.HL7Path)
			}
		}
	}

	// Compact: remove sentinel entries.
	compact := result[:0]
	for _, fm := range result {
		if fm.SegmentName != "" {
			compact = append(compact, fm)
		}
	}
	return compact
}

// loadOOBTemplateByID loads the OOB template identified by its UUID primary key and
// returns the parsed []FieldMapping plus the template's message_type.
func (s *HL7FHIRTransformServiceV3) loadOOBTemplateByID(ctx context.Context, templateID string) ([]FieldMapping, string, error) {
	query := `
		SELECT template_config, message_type
		FROM hl7_fhir_templates
		WHERE id = $1
		LIMIT 1
	`
	var templateConfig, messageType string
	if err := s.db.QueryRowContext(ctx, query, templateID).Scan(&templateConfig, &messageType); err != nil {
		return nil, "", fmt.Errorf("loadOOBTemplateByID(%s): %w", templateID, err)
	}
	var templateData map[string]interface{}
	if err := json.Unmarshal([]byte(templateConfig), &templateData); err != nil {
		return nil, "", fmt.Errorf("loadOOBTemplateByID parse(%s): %w", templateID, err)
	}
	mappings, err := s.convertV9TemplateToFieldMappings(templateData)
	return mappings, messageType, err
}

// ComputeDelta compares an incoming wizard []AtomicMapping against the OOB template
// mappings and returns the sparse MappingDelta (only changed/added/removed entries).
// Returns nil delta (no overrides) when the wizard mappings are identical to OOB.
func (s *HL7FHIRTransformServiceV3) ComputeDelta(
	ctx context.Context,
	messageType string,
	incoming []AtomicMapping,
) (*MappingDelta, string, error) {
	// Load OOB template to get base, template ID, and current version.
	query := `
		SELECT id, template_config, COALESCE(profile_version, '1.0')
		FROM hl7_fhir_templates
		WHERE message_type = $1 AND is_default = true
		ORDER BY created_at DESC
		LIMIT 1
	`
	var templateID, templateConfig, profileVersion string
	if err := s.db.QueryRowContext(ctx, query, messageType).Scan(&templateID, &templateConfig, &profileVersion); err != nil {
		return nil, "", fmt.Errorf("ComputeDelta: no OOB template for %s: %w", messageType, err)
	}
	var templateData map[string]interface{}
	if err := json.Unmarshal([]byte(templateConfig), &templateData); err != nil {
		return nil, "", fmt.Errorf("ComputeDelta: parse template: %w", err)
	}
	baseMappings, err := s.convertV9TemplateToFieldMappings(templateData)
	if err != nil {
		return nil, "", fmt.Errorf("ComputeDelta: convert template: %w", err)
	}

	// Index OOB by hl7PathKey for fast lookup.
	type oobEntry struct {
		fm    FieldMapping
		seen  bool
	}
	oobIndex := make(map[string]*oobEntry, len(baseMappings))
	for _, fm := range baseMappings {
		key := hl7PathKey(fm)
		oobIndex[key] = &oobEntry{fm: fm}
	}

	var overrides []MappingOverride

	for _, am := range incoming {
		key := am.SourcePath // sourcePath is the HL7 path key
		entry, inOOB := oobIndex[key]

		if !inOOB {
			// New mapping not in OOB → add
			overrides = append(overrides, MappingOverride{
				Action:      "add",
				HL7Path:     key,
				FHIRPath:    am.FHIRResourceType + "." + am.TargetPath,
				Transform:   am.TransformType,
				StaticValue: am.DefaultValue, // populated only for static_value mappings
				IsRequired:  am.IsRequired,
				Confidence:  am.Confidence,
			})
			continue
		}
		entry.seen = true

		// Check if anything meaningful changed.
		oobFHIRPath := entry.fm.FHIRResourceType + "." + entry.fm.FHIRElementPath
		incomingFHIRPath := am.FHIRResourceType + "." + am.TargetPath
		changed := incomingFHIRPath != oobFHIRPath ||
			am.TransformType != entry.fm.DataTypeTransform ||
			am.IsRequired != entry.fm.IsRequired

		if changed {
			overrides = append(overrides, MappingOverride{
				Action:     "replace",
				HL7Path:    key,
				FHIRPath:   incomingFHIRPath,
				Transform:  am.TransformType,
				IsRequired: am.IsRequired,
				Confidence: am.Confidence,
			})
		}
	}

	// OOB entries the user removed (not present in incoming).
	incomingPaths := make(map[string]struct{}, len(incoming))
	for _, am := range incoming {
		incomingPaths[am.SourcePath] = struct{}{}
	}
	for key, entry := range oobIndex {
		_ = entry
		if _, present := incomingPaths[key]; !present {
			overrides = append(overrides, MappingOverride{
				Action:  "remove",
				HL7Path: key,
			})
		}
	}

	if len(overrides) == 0 {
		return nil, templateID, nil // pure OOB — no delta needed
	}
	return &MappingDelta{
		Version:        1,
		BaseTemplateID: templateID,
		BasedOnVersion: profileVersion,
		Overrides:      overrides,
	}, templateID, nil
}

// loadDeltaMappings loads the interface's stored OOB template + applies mapping_overrides.
// Returns an error (causing fallthrough) when no delta row exists.
func (s *HL7FHIRTransformServiceV3) loadDeltaMappings(ctx context.Context, interfaceID, messageType string) ([]FieldMapping, error) {
	query := `
		SELECT standard_template_id, mapping_overrides
		FROM interface_message_mappings
		WHERE interface_id = $1
		  AND message_type = $2
		  AND uses_standard_template = true
		  AND mapping_overrides IS NOT NULL
		LIMIT 1
	`
	var templateID string
	var overridesBytes []byte
	err := s.db.QueryRowContext(ctx, query, interfaceID, messageType).Scan(&templateID, &overridesBytes)
	if err != nil {
		return nil, fmt.Errorf("no delta row for %s/%s", interfaceID, messageType)
	}

	baseMappings, _, err := s.loadOOBTemplateByID(ctx, templateID)
	if err != nil {
		return nil, fmt.Errorf("loadDeltaMappings: load OOB base: %w", err)
	}

	var delta MappingDelta
	if err := json.Unmarshal(overridesBytes, &delta); err != nil {
		return nil, fmt.Errorf("loadDeltaMappings: parse overrides: %w", err)
	}

	merged := mergeMappings(baseMappings, &delta)
	log.Printf("🔀 Delta merge: OOB %d + %d overrides → %d merged for %s/%s",
		len(baseMappings), len(delta.Overrides), len(merged), interfaceID, messageType)
	return merged, nil
}
