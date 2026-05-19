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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"ezhealthkonnect/fhir"
	"ezhealthkonnect/hl7" // For composite field parsing
	"ezhealthkonnect/services/hl7assembly"
	"ezhealthkonnect/services/hl7type"
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
	RuleType       string // "reference"|"focus"|"result"|"subject"|"performer"|"encounter"|"author"
	SourceResource string
	TargetResource string
	ReferencePath  string
	ConditionExpr  string
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
			// status: map HL7 ADT trigger event codes to FHIR encounter-status values.
			// Primary: map whatever value the template wrote (e.g. MSH.9.2 mapped to status).
			if st, ok := r["status"].(string); ok {
				if mapped := mapEncounterStatus(st); mapped != "" {
					r["status"] = mapped
				}
			}
			// Fallback: template had no mapping for status — derive from the message type
			// event code (e.g. "ADT^A01" → "A01" → "in-progress").  This is the IG-correct
			// path for ADT/ORM/ORU messages where the status is implicit in the event.
			if _, hasStatus := r["status"]; !hasStatus {
				if idx := strings.Index(messageType, "^"); idx >= 0 {
					if derived := mapEncounterStatus(messageType[idx+1:]); derived != "" {
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
			// meta.security[].system: MSH.11 processing ID (P/T/D) maps to security[].code
			// but HL7 v2-0103 values need the v2-0103 system URI to be valid.
			if meta, ok := r["meta"].(map[string]interface{}); ok {
				if secs, ok2 := meta["security"].([]interface{}); ok2 {
					for _, secRaw := range secs {
						if sec, ok3 := secRaw.(map[string]interface{}); ok3 {
							if sys, _ := sec["system"].(string); sys == "" {
								if code, _ := sec["code"].(string); code != "" {
									sec["system"] = "http://terminology.hl7.org/CodeSystem/v2-0103"
								}
							}
						}
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
					// "ORU^R01" composite format — extract trigger event after "^"
					parts := strings.SplitN(code, "^", 2)
					ec["code"] = parts[1]
					delete(ec, "display")
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
			// focus: MessageHeader must reference the primary clinical resource
			// in the bundle so the validator can confirm every entry is reachable.
			// We defer this to after all resources are collected — see below.
		}
	}

	// Regenerate narratives after all normalizations so the human-readable text reflects
	// fully-normalized values (e.g. "male" not "M", "1960-01-01" not "19600101").
	// We only update resources that already have a text.div — newly assembled resources
	// (built by assembly.go) have their own purpose-built narrative and don't need this.
	for _, r := range allResources {
		rt, _ := r["resourceType"].(string)
		switch rt {
		case "Patient", "DiagnosticReport", "Observation", "Encounter", "MessageHeader",
			"Practitioner", "PractitionerRole", "RelatedPerson", "Coverage":
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

	// ── Stage: Focus augmentation ─────────────────────────────────────────────
	// Ensures MessageHeader.focus references every resource whose type appears in
	// the manifest's FocusTypes list.  Runs after segment processors so that
	// processor-added resources (e.g. the prior Patient from MRGProcessor) are
	// included.  Duplicate-safe: existing references are not re-added.
	// Uses "ResourceType/id" format — rewriteReferences() converts to urn:uuid:.
	augmentMessageHeaderFocus(allResources, manifest.GetFocusTypes(messageType))

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

// ──────────────────────────────────────────────────────────────────────────────
// Assembly rule engine
// ──────────────────────────────────────────────────────────────────────────────

// loadAssemblyRulesForType returns the assembly rules for one message type,
// using an in-memory cache keyed by messageType.  The cache is populated on
// first access and can be invalidated by calling InvalidateAssemblyRuleCache.
func (s *HL7FHIRTransformServiceV3) loadAssemblyRulesForType(ctx context.Context, messageType string) []AssemblyRule {
	s.assemblyRulesMu.RLock()
	if s.assemblyRules != nil {
		rules := s.assemblyRules[messageType]
		s.assemblyRulesMu.RUnlock()
		return rules
	}
	s.assemblyRulesMu.RUnlock()

	// Cache miss — load ALL rules from DB once.
	s.assemblyRulesMu.Lock()
	defer s.assemblyRulesMu.Unlock()
	if s.assemblyRules != nil { // double-check after acquiring write lock
		return s.assemblyRules[messageType]
	}

	all := make(map[string][]AssemblyRule)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, message_type, rule_type, source_resource, target_resource,
		       reference_path, COALESCE(condition_expr,''), sequence
		FROM   assembly_rules
		WHERE  is_active = true
		ORDER  BY message_type, sequence`)
	if err != nil {
		log.Printf("⚠️  assembly_rules query failed: %v — cross-resource wiring disabled", err)
		s.assemblyRules = all // empty cache prevents repeated DB errors
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var r AssemblyRule
		if err2 := rows.Scan(&r.ID, &r.MessageType, &r.RuleType,
			&r.SourceResource, &r.TargetResource,
			&r.ReferencePath, &r.ConditionExpr, &r.Sequence); err2 != nil {
			log.Printf("⚠️  assembly_rules scan error: %v", err2)
			continue
		}
		all[r.MessageType] = append(all[r.MessageType], r)
	}
	s.assemblyRules = all
	log.Printf("📋 assembly_rules cache loaded: %d message types", len(all))
	return all[messageType]
}

// InvalidateAssemblyRuleCache clears the in-memory cache so the next
// transformation reloads from the database.  Call after migrating new rules.
func (s *HL7FHIRTransformServiceV3) InvalidateAssemblyRuleCache() {
	s.assemblyRulesMu.Lock()
	s.assemblyRules = nil
	s.assemblyRulesMu.Unlock()
	log.Printf("🔄 assembly_rules cache invalidated")
}

// applyAssemblyRules executes every rule for messageType against the resource
// slice and returns the updated slice.
func (s *HL7FHIRTransformServiceV3) applyAssemblyRules(
	ctx context.Context,
	messageType string,
	resources []map[string]interface{},
) []map[string]interface{} {
	rules := s.loadAssemblyRulesForType(ctx, messageType)
	if len(rules) == 0 {
		return resources
	}
	for _, rule := range rules {
		resources = s.applyOneAssemblyRule(rule, resources)
	}
	log.Printf("✅ assembly_rules applied: %d rules for %s", len(rules), messageType)
	return resources
}

// applyOneAssemblyRule applies a single assembly rule to the resource slice.
func (s *HL7FHIRTransformServiceV3) applyOneAssemblyRule(
	rule AssemblyRule,
	resources []map[string]interface{},
) []map[string]interface{} {
	switch rule.RuleType {

	case "reference", "subject", "encounter":
		// Set source.referencePath = { reference: "TargetType/id" } when absent.
		// FHIR paths with cardinality 0..* (e.g. Encounter.appointment, DiagnosticReport.basedOn)
		// must be JSON arrays; wrap the reference object accordingly.
		targetID := assemblyFindID(resources, rule.TargetResource)
		if targetID == "" {
			return resources
		}
		ref := map[string]interface{}{"reference": rule.TargetResource + "/" + targetID}
		arrayPaths := map[string]bool{
			"appointment": true,
			"basedOn":     true,
			"partOf":      true,
			"reasonReference": true,
		}
		useArray := arrayPaths[rule.ReferencePath]
		for _, r := range resources {
			if rt, _ := r["resourceType"].(string); rt != rule.SourceResource {
				continue
			}
			existing := r[rule.ReferencePath]
			if existing != nil {
				// "subject" rules must win over a heuristic display-only value
				// (e.g. {"display": "Colfer, Eoin"} with no "reference" key).
				// Other rule types skip when any value is already present.
				if rule.RuleType != "subject" {
					continue
				}
				if em, ok := existing.(map[string]interface{}); ok {
					if _, hasRef := em["reference"]; hasRef {
						continue // already correctly wired — leave it
					}
					// display-only: fall through and overwrite
				} else {
					continue // non-map value (raw string etc.) — leave it
				}
			}
			if useArray {
				r[rule.ReferencePath] = []interface{}{ref}
			} else {
				r[rule.ReferencePath] = ref
			}
		}

	case "focus":
		// Append { reference: "TargetType/id" } to source.focus[] when not present.
		targetID := assemblyFindID(resources, rule.TargetResource)
		if targetID == "" {
			return resources
		}
		ref := map[string]interface{}{"reference": rule.TargetResource + "/" + targetID}
		for _, r := range resources {
			if rt, _ := r["resourceType"].(string); rt != rule.SourceResource {
				continue
			}
			focus, _ := r[rule.ReferencePath].([]interface{})
			if !assemblyRefPresent(focus, targetID) {
				r[rule.ReferencePath] = append(focus, ref)
			}
		}

	case "result":
		// Collect ALL target resources and set source.referencePath = [refs…].
		// Overwrites any existing value so the field-mapper's placeholder is replaced.
		var refs []interface{}
		for _, r := range resources {
			if rt, _ := r["resourceType"].(string); rt != rule.TargetResource {
				continue
			}
			if id, _ := r["id"].(string); id != "" {
				refs = append(refs, map[string]interface{}{
					"reference": rule.TargetResource + "/" + id,
				})
			}
		}
		if len(refs) == 0 {
			return resources
		}
		for _, r := range resources {
			if rt, _ := r["resourceType"].(string); rt == rule.SourceResource {
				r[rule.ReferencePath] = refs
			}
		}

	case "performer", "author":
		// Set source.referencePath = [{ reference: "TargetType/id" }] when absent.
		targetID := assemblyFindID(resources, rule.TargetResource)
		if targetID == "" {
			return resources
		}
		ref := map[string]interface{}{"reference": rule.TargetResource + "/" + targetID}
		for _, r := range resources {
			if rt, _ := r["resourceType"].(string); rt != rule.SourceResource {
				continue
			}
			if _, exists := r[rule.ReferencePath]; !exists {
				r[rule.ReferencePath] = []interface{}{ref}
			}
		}
	}

	return resources
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

// assemblyFindID returns the id of the first resource with the given type.
func assemblyFindID(resources []map[string]interface{}, resourceType string) string {
	for _, r := range resources {
		if rt, _ := r["resourceType"].(string); rt == resourceType {
			if id, _ := r["id"].(string); id != "" {
				return id
			}
		}
	}
	return ""
}

// assemblyRefPresent reports whether a focus/result slice already contains a
// reference to the given id (prevents duplicates when the template also wires focus).
func assemblyRefPresent(refs []interface{}, targetID string) bool {
	for _, f := range refs {
		if fm, ok := f.(map[string]interface{}); ok {
			if ref, _ := fm["reference"].(string); strings.Contains(ref, targetID) {
				return true
			}
		}
	}
	return false
}

// mfeActionFromSegments returns the MFE.1 action code (MAD/MUP/MDL/MDC) from
// enhancedSegments, upper-cased and trimmed. Returns "" when no MFE segment is present.
// Used to distinguish a genuine MDL/MDC deactivation from a false produced by
// transformToBoolean's default-false behaviour on an unrecognised STF.7 value.
func (s *HL7FHIRTransformServiceV3) mfeActionFromSegments(enhancedSegments map[string]interface{}) string {
	mfeSeg, ok := enhancedSegments["MFE"].(map[string]interface{})
	if !ok {
		return ""
	}
	fields, ok := mfeSeg["fields"].([]interface{})
	if !ok {
		return ""
	}
	for _, f := range fields {
		if fm, ok2 := f.(map[string]interface{}); ok2 {
			if key, _ := fm["key"].(string); key == "MFE.1" {
				if val, _ := fm["value"].(string); val != "" {
					return strings.ToUpper(strings.TrimSpace(val))
				}
			}
		}
	}
	return ""
}

// extractSegmentGroup retrieves all instances of a segment type from segmentGroups,
// handling both the typed map[string][]hl7.EnhancedSegment path and the JSON-unmarshaled path.
func (s *HL7FHIRTransformServiceV3) extractSegmentGroup(parsedHL7Data map[string]interface{}, segmentName string) []hl7.EnhancedSegment {
	rawGroups, exists := parsedHL7Data["segmentGroups"]
	if !exists {
		return nil
	}

	// Typed path — comes directly from the Go parser (test controller, processing engine)
	if typed, ok := rawGroups.(map[string][]hl7.EnhancedSegment); ok {
		return typed[segmentName]
	}

	// JSON-unmarshaled path — marshal back and decode into typed struct
	jsonBytes, err := json.Marshal(rawGroups)
	if err != nil {
		log.Printf("⚠️ extractSegmentGroup: marshal failed: %v", err)
		return nil
	}
	var converted map[string][]hl7.EnhancedSegment
	if err = json.Unmarshal(jsonBytes, &converted); err != nil {
		log.Printf("⚠️ extractSegmentGroup: unmarshal failed: %v", err)
		return nil
	}
	return converted[segmentName]
}

// segFieldValue returns the value of a named field (e.g. "OBX.3") from an EnhancedSegment.
func segFieldValue(seg hl7.EnhancedSegment, key string) string {
	for _, f := range seg.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
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

// tsToISODateTime converts an HL7 TS value to ISO 8601.
// Returns datetime (YYYY-MM-DDTHH:MM:SS+00:00) when time components are present,
// date-only (YYYY-MM-DD) otherwise.
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

// =====================================

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

		// Extract HL7 value using atomic extraction
		hl7Value, found := s.extractHL7ValueAtomic(enhancedSegments, mapping)
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

// =====================================
// NARRATIVE GENERATION
// =====================================

// narrativeRowsBuilder appends resource-specific rows to a narrative table.
// Method expressions are used so each builder is a plain function value stored
// in the registry — adding a new resource type is a single registry entry + method.
type narrativeRowsBuilder func(s *HL7FHIRTransformServiceV3, html *strings.Builder, resource map[string]interface{})

// narrativeBuilderRegistry maps FHIR resourceType → its row-builder.
// Register new resource types here; the engine picks them up automatically.
var narrativeBuilderRegistry = map[string]narrativeRowsBuilder{
	"Patient":          (*HL7FHIRTransformServiceV3).addPatientRows,
	"Encounter":        (*HL7FHIRTransformServiceV3).addEncounterRows,
	"MessageHeader":    (*HL7FHIRTransformServiceV3).addMessageHeaderRows,
	"DiagnosticReport": (*HL7FHIRTransformServiceV3).addDiagnosticReportRows,
	"Observation":      (*HL7FHIRTransformServiceV3).addObservationRows,
	"RelatedPerson":    (*HL7FHIRTransformServiceV3).addRelatedPersonRows,
	"Coverage":         (*HL7FHIRTransformServiceV3).addCoverageRows,
}

// generateNarrative creates a FHIR-spec compliant XHTML narrative (text.div).
// The div wrapper and xmlns are required by the FHIR narrative spec.
func (s *HL7FHIRTransformServiceV3) generateNarrative(resourceType string, resource map[string]interface{}) string {
	html := s.generateResourceTable(resourceType, resource)
	return fmt.Sprintf("<div xmlns=\"http://www.w3.org/1999/xhtml\">%s</div>", html)
}

// generateResourceTable renders an XHTML table of key resource fields.
// Looks up a registered builder for the resourceType; falls back to generic scalar rendering.
func (s *HL7FHIRTransformServiceV3) generateResourceTable(resourceType string, resource map[string]interface{}) string {
	var html strings.Builder

	html.WriteString("<table class=\"grid\" style=\"border-collapse:collapse;width:100%;\">")
	html.WriteString(fmt.Sprintf("<thead><tr style=\"background:#f0f0f0;\"><th colspan=\"2\" style=\"padding:8px;text-align:left;\">%s</th></tr></thead>", resourceType))
	html.WriteString("<tbody>")

	s.addTableRow(&html, "ID", fmt.Sprintf("%v", resource["id"]))

	if builder, ok := narrativeBuilderRegistry[resourceType]; ok {
		builder(s, &html, resource)
	} else {
		s.addGenericRows(&html, resource)
	}

	html.WriteString("</tbody></table>")
	return html.String()
}

// extractFirstGiven returns the first given name from a name map's "given" field.
// Handles both []interface{} (after JSON round-trip) and []string (direct engine output).
func extractFirstGiven(name map[string]interface{}) string {
	switch gv := name["given"].(type) {
	case []interface{}:
		if len(gv) > 0 {
			s, _ := gv[0].(string)
			return s
		}
	case []string:
		if len(gv) > 0 {
			return gv[0]
		}
	}
	return ""
}

// addGenericRows renders all scalar fields alphabetically for resource types
// that have no dedicated builder registered in narrativeBuilderRegistry.
func (s *HL7FHIRTransformServiceV3) addGenericRows(html *strings.Builder, resource map[string]interface{}) {
	keys := make([]string, 0, len(resource))
	for key := range resource {
		if key != "resourceType" && key != "id" && key != "text" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		switch v := resource[key].(type) {
		case string:
			s.addTableRow(html, key, v)
		case bool:
			s.addTableRow(html, key, fmt.Sprintf("%v", v))
		case float64:
			s.addTableRow(html, key, fmt.Sprintf("%g", v))
		}
	}
}

func (s *HL7FHIRTransformServiceV3) addTableRow(buf *strings.Builder, label, value string) {
	if value == "" || value == "<nil>" {
		return
	}
	// Escape HTML entities in value
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "\"", "&quot;")

	buf.WriteString(fmt.Sprintf("<tr><td style=\"padding:4px 8px;border:1px solid #ddd;font-weight:bold;\">%s</td><td style=\"padding:4px 8px;border:1px solid #ddd;\">%s</td></tr>",
		label, value))
}

func (s *HL7FHIRTransformServiceV3) addPatientRows(html *strings.Builder, resource map[string]interface{}) {
	// Name
	if names, ok := resource["name"].([]interface{}); ok && len(names) > 0 {
		if name, ok := names[0].(map[string]interface{}); ok {
			family, _ := name["family"].(string)
			given := extractFirstGiven(name)
			if family != "" || given != "" {
				s.addTableRow(html, "Name", fmt.Sprintf("%s, %s", family, given))
			}
		}
	}

	// Gender
	if gender, ok := resource["gender"].(string); ok {
		s.addTableRow(html, "Gender", gender)
	}

	// Birth Date
	if birthDate, ok := resource["birthDate"].(string); ok {
		s.addTableRow(html, "Birth Date", birthDate)
	}

	// Address
	if addresses, ok := resource["address"].([]interface{}); ok && len(addresses) > 0 {
		if addr, ok := addresses[0].(map[string]interface{}); ok {
			var parts []string
			if city, ok := addr["city"].(string); ok && city != "" {
				parts = append(parts, city)
			}
			if state, ok := addr["state"].(string); ok && state != "" {
				parts = append(parts, state)
			}
			if len(parts) > 0 {
				s.addTableRow(html, "Address", strings.Join(parts, ", "))
			}
		}
	}
}

func (s *HL7FHIRTransformServiceV3) addEncounterRows(html *strings.Builder, resource map[string]interface{}) {
	// Class
	if class, ok := resource["class"].(map[string]interface{}); ok {
		if code, ok := class["code"].(string); ok {
			classDesc := code
			// Map common codes to descriptions
			switch code {
			case "I": classDesc = "Inpatient"
			case "O": classDesc = "Outpatient"
			case "E": classDesc = "Emergency"
			}
			s.addTableRow(html, "Class", classDesc)
		}
	}

	// Status
	if status, ok := resource["status"].(string); ok {
		s.addTableRow(html, "Status", status)
	}

	// Period
	if period, ok := resource["period"].(map[string]interface{}); ok {
		if start, ok := period["start"].(string); ok {
			s.addTableRow(html, "Start", start)
		}
		if end, ok := period["end"].(string); ok {
			s.addTableRow(html, "End", end)
		}
	}

	// Location
	if locations, ok := resource["location"].([]interface{}); ok && len(locations) > 0 {
		if loc, ok := locations[0].(map[string]interface{}); ok {
			if location, ok := loc["location"].(map[string]interface{}); ok {
				if display, ok := location["display"].(string); ok {
					s.addTableRow(html, "Location", display)
				}
			}
		}
	}
}

func (s *HL7FHIRTransformServiceV3) addMessageHeaderRows(html *strings.Builder, resource map[string]interface{}) {
	// Event
	if eventCoding, ok := resource["eventCoding"].(map[string]interface{}); ok {
		code, _ := eventCoding["code"].(string)
		display, _ := eventCoding["display"].(string)
		if code != "" {
			eventStr := code
			if display != "" {
				eventStr = fmt.Sprintf("%s (%s)", code, display)
			}
			s.addTableRow(html, "Event", eventStr)
		}
	}

	// Source
	if source, ok := resource["source"].(map[string]interface{}); ok {
		if name, ok := source["name"].(string); ok {
			s.addTableRow(html, "Source", name)
		}
	}
}

func (s *HL7FHIRTransformServiceV3) addDiagnosticReportRows(html *strings.Builder, resource map[string]interface{}) {
	// Report name from code.text or code.coding[0].display
	reportName := ""
	if code, ok := resource["code"].(map[string]interface{}); ok {
		if t, ok2 := code["text"].(string); ok2 && t != "" {
			reportName = t
		} else if codings, ok2 := code["coding"].([]interface{}); ok2 && len(codings) > 0 {
			if c, ok3 := codings[0].(map[string]interface{}); ok3 {
				reportName, _ = c["display"].(string)
			}
		}
	}
	if reportName != "" {
		s.addTableRow(html, "Report", reportName)
	}

	// Status
	if status, ok := resource["status"].(string); ok {
		s.addTableRow(html, "Status", status)
	}

	// Effective date
	if eff, ok := resource["effectiveDateTime"].(string); ok {
		s.addTableRow(html, "Date", eff)
	}

	// Results count
	if results, ok := resource["result"].([]interface{}); ok {
		s.addTableRow(html, "Results", fmt.Sprintf("%d observation(s)", len(results)))
	}
}

func (s *HL7FHIRTransformServiceV3) addObservationRows(html *strings.Builder, resource map[string]interface{}) {
	// Code
	codeName := ""
	if code, ok := resource["code"].(map[string]interface{}); ok {
		if t, ok2 := code["text"].(string); ok2 && t != "" {
			codeName = t
		} else if codings, ok2 := code["coding"].([]interface{}); ok2 && len(codings) > 0 {
			if c, ok3 := codings[0].(map[string]interface{}); ok3 {
				codeName, _ = c["display"].(string)
			}
		}
	}
	if codeName != "" {
		s.addTableRow(html, "Code", codeName)
	}

	// Value — handle all value[x] types
	for _, vk := range []string{"valueString", "valueQuantity", "valueCodeableConcept",
		"valueDateTime", "valueBoolean", "valueInteger", "valueTime", "valueSampledData"} {
		if val, ok := resource[vk]; ok {
			switch v := val.(type) {
			case string:
				s.addTableRow(html, "Value", v)
			case bool:
				s.addTableRow(html, "Value", fmt.Sprintf("%v", v))
			case float64:
				s.addTableRow(html, "Value", fmt.Sprintf("%g", v))
			case map[string]interface{}:
				// Quantity: "value unit" or CodeableConcept: text or coding[0].display
				if num, ok2 := v["value"]; ok2 {
					unit, _ := v["unit"].(string)
					comp, _ := v["comparator"].(string)
					numStr := fmt.Sprintf("%g", num)
					if comp != "" {
						s.addTableRow(html, "Value", fmt.Sprintf("%s %s %s", comp, numStr, unit))
					} else {
						s.addTableRow(html, "Value", fmt.Sprintf("%s %s", numStr, unit))
					}
				} else if text, ok2 := v["text"].(string); ok2 && text != "" {
					s.addTableRow(html, "Value", text)
				} else if codings, ok2 := v["coding"].([]interface{}); ok2 && len(codings) > 0 {
					if c, ok3 := codings[0].(map[string]interface{}); ok3 {
						if disp, _ := c["display"].(string); disp != "" {
							s.addTableRow(html, "Value", disp)
						}
					}
				}
			}
			break // only one value[x] can be present
		}
	}
	if _, hasAtt := resource["valueAttachment"]; hasAtt {
		s.addTableRow(html, "Value", "(attachment)")
	}
	if derived, ok := resource["derivedFrom"].([]interface{}); ok && len(derived) > 0 {
		s.addTableRow(html, "Value", "(referenced document)")
	}

	// Status
	if status, ok := resource["status"].(string); ok {
		s.addTableRow(html, "Status", status)
	}

	// Date
	if eff, ok := resource["effectiveDateTime"].(string); ok {
		s.addTableRow(html, "Date", eff)
	}
}

func (s *HL7FHIRTransformServiceV3) addRelatedPersonRows(html *strings.Builder, resource map[string]interface{}) {
	// Name
	if names, ok := resource["name"].([]interface{}); ok && len(names) > 0 {
		if name, ok := names[0].(map[string]interface{}); ok {
			family, _ := name["family"].(string)
			given := extractFirstGiven(name)
			if family != "" || given != "" {
				s.addTableRow(html, "Name", fmt.Sprintf("%s, %s", family, given))
			}
		}
	}

	// Gender
	if gender, ok := resource["gender"].(string); ok {
		s.addTableRow(html, "Gender", gender)
	}

	// Birth Date
	if birthDate, ok := resource["birthDate"].(string); ok {
		s.addTableRow(html, "Birth Date", birthDate)
	}
}

func (s *HL7FHIRTransformServiceV3) addCoverageRows(html *strings.Builder, resource map[string]interface{}) {
	// Status
	if status, ok := resource["status"].(string); ok {
		s.addTableRow(html, "Status", status)
	}

	// Subscriber ID
	if sid, ok := resource["subscriberId"].(string); ok {
		s.addTableRow(html, "Subscriber ID", sid)
	}

	// Class — show type code + value
	if classes, ok := resource["class"].([]interface{}); ok && len(classes) > 0 {
		if cls, ok := classes[0].(map[string]interface{}); ok {
			value, _ := cls["value"].(string)
			typeCode := ""
			if t, ok2 := cls["type"].(map[string]interface{}); ok2 {
				if codings, ok3 := t["coding"].([]interface{}); ok3 && len(codings) > 0 {
					if c, ok4 := codings[0].(map[string]interface{}); ok4 {
						typeCode, _ = c["code"].(string)
					}
				}
			}
			if value != "" {
				label := "Class"
				if typeCode != "" {
					label = fmt.Sprintf("Class (%s)", typeCode)
				}
				s.addTableRow(html, label, value)
			}
		}
	}

	// Relationship
	if rel, ok := resource["relationship"].(map[string]interface{}); ok {
		if codings, ok2 := rel["coding"].([]interface{}); ok2 && len(codings) > 0 {
			if c, ok3 := codings[0].(map[string]interface{}); ok3 {
				code, _ := c["code"].(string)
				if code != "" {
					s.addTableRow(html, "Relationship", strings.ToUpper(code))
				}
			}
		}
	}
}

// =====================================
// ATOMIC HL7 VALUE EXTRACTION
// =====================================

func (s *HL7FHIRTransformServiceV3) extractHL7ValueAtomic(
	enhancedSegments map[string]interface{},
	mapping FieldMapping,
) (string, bool) {
	log.Printf("🔍 extractHL7ValueAtomic called: segment=%s, field=%s, component=%s",
		mapping.SegmentName, mapping.HL7Field, mapping.HL7Component)

	// Get the segment
	segment, segmentExists := enhancedSegments[mapping.SegmentName].(map[string]interface{})
	if !segmentExists {
		log.Printf("🔍 Segment %s not found in HL7 data", mapping.SegmentName)
		return "", false
	}

	// DEBUG: Log segment keys to see what's available
	segmentKeys := make([]string, 0, len(segment))
	for k := range segment {
		segmentKeys = append(segmentKeys, k)
	}
	log.Printf("🔍 Segment %s has keys: %v", mapping.SegmentName, segmentKeys)

	// Get the fields array (handle both []interface{} and primitive.A from MongoDB)
	var fields []interface{}
	fieldsRaw, fieldsExists := segment["fields"]
	if !fieldsExists {
		log.Printf("🔍 No fields found in segment %s (key missing)", mapping.SegmentName)
		return "", false
	}

	// Try direct []interface{} first
	if fieldsSlice, ok := fieldsRaw.([]interface{}); ok {
		fields = fieldsSlice
	} else {
		// Try primitive.A from MongoDB (import "go.mongodb.org/mongo-driver/bson/primitive")
		// Convert any slice type to []interface{}
		fieldsValue := reflect.ValueOf(fieldsRaw)
		if fieldsValue.Kind() == reflect.Slice {
			fields = make([]interface{}, fieldsValue.Len())
			for i := 0; i < fieldsValue.Len(); i++ {
				fields[i] = fieldsValue.Index(i).Interface()
			}
			log.Printf("✅ Converted fields from %T to []interface{} (%d items)", fieldsRaw, len(fields))
		} else {
			log.Printf("🔍 fields key exists but wrong type: %T (not a slice)", fieldsRaw)
			return "", false
		}
	}

	// Build the expected field key (with component if specified)
	var expectedFieldKey string
	if mapping.HL7Component != "" {
		expectedFieldKey = fmt.Sprintf("%s.%s.%s", mapping.SegmentName, mapping.HL7Field, mapping.HL7Component)
	} else {
		expectedFieldKey = fmt.Sprintf("%s.%s", mapping.SegmentName, mapping.HL7Field)
	}

	log.Printf("🔍 Looking for field key: '%s' (component='%s')", expectedFieldKey, mapping.HL7Component)

	// Find the target field
	for _, field := range fields {
		fieldMap, ok := field.(map[string]interface{})
		if !ok {
			continue
		}

		key, keyOk := fieldMap["key"].(string)
		if !keyOk {
			continue
		}

		// Handle component mapping (e.g., MSH.9.2)
		if mapping.HL7Component != "" {
			// Look for the component directly in subfields
			if key == fmt.Sprintf("%s.%s", mapping.SegmentName, mapping.HL7Field) {
				return s.extractComponentFromHL7Field(fieldMap, mapping)
			}
		} else {
			// Simple field match
			if key == expectedFieldKey {
				if value, valueOk := fieldMap["value"].(string); valueOk {
					return value, true
				}
			}
		}
	}

	log.Printf("🔍 Field %s not found in segment %s", expectedFieldKey, mapping.SegmentName)
	return "", false
}

// extractHL7FieldDirect looks up a single field value from enhancedSegments without
// a FieldMapping struct.  fieldKey must be the full qualified key, e.g. "OBX.2".
// Used to inject sibling control fields (OBX.2, OBX.6) into transformation rules.
func (s *HL7FHIRTransformServiceV3) extractHL7FieldDirect(
	enhancedSegments map[string]interface{},
	segName, fieldKey string,
) string {
	seg, ok := enhancedSegments[segName].(map[string]interface{})
	if !ok {
		return ""
	}
	fieldsRaw, ok := seg["fields"]
	if !ok {
		return ""
	}
	var fields []interface{}
	if fs, ok := fieldsRaw.([]interface{}); ok {
		fields = fs
	} else {
		v := reflect.ValueOf(fieldsRaw)
		if v.Kind() == reflect.Slice {
			fields = make([]interface{}, v.Len())
			for i := 0; i < v.Len(); i++ {
				fields[i] = v.Index(i).Interface()
			}
		}
	}
	for _, f := range fields {
		fm, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		if k, ok := fm["key"].(string); ok && k == fieldKey {
			if val, ok := fm["value"].(string); ok {
				return val
			}
		}
	}
	return ""
}

func (s *HL7FHIRTransformServiceV3) extractComponentFromHL7Field(
	fieldMap map[string]interface{},
	mapping FieldMapping,
) (string, bool) {

	// Look for the exact component in subfields
	if subfields, subfieldOk := fieldMap["subfields"].([]interface{}); subfieldOk {
		expectedComponentKey := fmt.Sprintf("%s.%s.%s", mapping.SegmentName, mapping.HL7Field, mapping.HL7Component)

		log.Printf("🔍 Looking for component key: %s", expectedComponentKey)

		for _, subfield := range subfields {
			subfieldMap, ok := subfield.(map[string]interface{})
			if !ok {
				continue
			}

			subfieldKey, keyOk := subfieldMap["key"].(string)
			if !keyOk {
				continue
			}

			log.Printf("🔍 Found subfield key: %s", subfieldKey)

			if subfieldKey == expectedComponentKey {
				if value, valueOk := subfieldMap["value"].(string); valueOk {
					log.Printf("✅ Extracted component %s: %s", expectedComponentKey, value)
					return value, true
				}
			}
		}
	}

	// Fallback: Manual parsing using the HL7 parser
	if value, valueOk := fieldMap["value"].(string); valueOk && value != "" {
		log.Printf("🔄 Fallback: Manual parsing component %s from value: %s", mapping.HL7Component, value)
		return s.manualParseComponentValue(value, mapping)
	}

	log.Printf("⚠️ Component %s not found in field", mapping.HL7Component)
	return "", false
}

func (s *HL7FHIRTransformServiceV3) manualParseComponentValue(
	fieldValue string,
	mapping FieldMapping,
) (string, bool) {

	// Use the HL7 parser to handle composite fields properly
	enhancedFieldData := hl7.ParseHL7Field(fieldValue)
	if enhancedFieldData != nil && len(enhancedFieldData.Repetitions) > 0 {
		rep := enhancedFieldData.Repetitions[0]

		componentPos, err := strconv.Atoi(mapping.HL7Component)
		if err != nil {
			return "", false
		}

		if componentPos > 0 && componentPos <= len(rep.Components) {
			componentValue := rep.Components[componentPos-1].RawValue
			return strings.TrimSpace(componentValue), true
		}
	}

	// Simple fallback: split by ^ and extract component
	components := strings.Split(fieldValue, "^")
	componentPos, err := strconv.Atoi(mapping.HL7Component)
	if err != nil || componentPos < 1 || componentPos > len(components) {
		return "", false
	}

	return strings.TrimSpace(components[componentPos-1]), true
}

// =====================================
// ATOMIC VALUE TRANSFORMATION
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
	case "msh9_trigger_event_to_coding":
		// ✅ EXPLICIT CASE for trigger event transformation
		result := s.transformTriggerEventToCoding(hl7Value)
		log.Printf("✅ Trigger event transform: '%s' → %+v", hl7Value, result)
		return result, nil
	case "xtn_to_contactpoint", "phone_to_contactpoint":
		return s.transformPhoneToContactPointEnhanced(hl7Value, mapping.TransformationRules), nil
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
		return hl7Value, nil
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
		return hl7Value, nil
	}
}

// =====================================
// SCHEMA-DRIVEN ATOMIC FIELD SETTING
// =====================================

func (s *HL7FHIRTransformServiceV3) setAtomicFieldInResource(
	resource map[string]interface{},
	fieldPath string,
	value interface{},
	schema *fhir.FHIRSchema,
) error {

	// Strip the "ResourceType." prefix that template fhirPaths carry
	// (e.g. "Practitioner.name[0].family" → "name[0].family").
	// setNestedFieldFromSchema handles this for simple 2-part paths, but
	// setFieldWithArrayIndices does not — causing values to land under a
	// spurious "Practitioner" key instead of directly on the resource.
	if schema != nil && schema.ResourceType != "" {
		if pfx := schema.ResourceType + "."; strings.HasPrefix(fieldPath, pfx) {
			fieldPath = strings.TrimPrefix(fieldPath, pfx)
		}
	}

	// Handle paths with array indices (e.g., "name[0].given[1]")
	if strings.Contains(fieldPath, "[") {
		return s.setFieldWithArrayIndices(resource, fieldPath, value, schema)
	}

	// Handle nested paths using schema structure
	if strings.Contains(fieldPath, ".") {
		return s.setNestedFieldFromSchema(resource, fieldPath, value, schema)
	}

	// ✅ FIX: Allow known FHIR choice type fields without strict validation
	// FHIR choice types like event[x] are defined as eventCoding, eventUri, etc.
	knownChoiceFields := map[string]bool{
		"eventCoding":            true,
		"eventUri":               true,
		"eventCanonical":         true,
		"valueString":            true,
		"valueCode":              true,
		"valueCoding":            true,
		"valueBoolean":           true,
		"valueInteger":           true,
		"valueDecimal":           true,
		"valueDateTime":          true,
		"valueDate":              true,
		"valueTime":              true,
		"effectiveDateTime":      true,
		"effectivePeriod":        true,
		"subjectReference":       true,
		"subjectCodeableConcept": true,
	}

	if knownChoiceFields[fieldPath] {
		log.Printf("✅ Allowing known choice type field: %s", fieldPath)
		resource[fieldPath] = value
		return nil
	}

	// Handle simple fields - STRICT SCHEMA VALIDATION for non-choice types
	normalizedPath := s.normalizeFieldPath(fieldPath, schema.ResourceType)
	element, exists := schema.Elements[normalizedPath]

	if !exists {
		element, exists = schema.Elements[fieldPath]
	}

	// ✅ ENHANCED: Try to find choice type elements if direct lookup fails
	if !exists {
		choiceElement, choiceExists := s.findChoiceTypeElement(fieldPath, schema)
		if choiceExists {
			element = choiceElement
			exists = true
			log.Printf("✅ Found choice type element for %s", fieldPath)
		}
	}

	if !exists {
		// ✅ STRICT: Reject fields not in schema
		log.Printf("❌ REJECTED: Field %s not found in %s schema - invalid property", fieldPath, schema.ResourceType)
		return fmt.Errorf("field %s not found in FHIR schema for %s", fieldPath, schema.ResourceType)
	}

	// Handle cardinality based on schema
	if strings.Contains(element.Cardinality, "*") {
		return s.setArrayField(resource, fieldPath, value)
	} else {
		resource[fieldPath] = value
		return nil
	}
}

// Helper method to find choice type elements (e.g., event[x] for eventCoding)
func (s *HL7FHIRTransformServiceV3) findChoiceTypeElement(
	fieldPath string,
	schema *fhir.FHIRSchema,
) (*fhir.FHIRElement, bool) {

	// Common FHIR choice type patterns
	choicePatterns := map[string]string{
		"eventCoding":            "event[x]",
		"eventUri":               "event[x]",
		"eventCanonical":         "event[x]",
		"valueString":            "value[x]",
		"valueCode":              "value[x]",
		"valueCoding":            "value[x]",
		"valueBoolean":           "value[x]",
		"valueInteger":           "value[x]",
		"valueDecimal":           "value[x]",
		"valueDateTime":          "value[x]",
		"valueDate":              "value[x]",
		"valueTime":              "value[x]",
		"effectiveDateTime":      "effective[x]",
		"effectivePeriod":        "effective[x]",
		"subjectReference":       "subject[x]",
		"subjectCodeableConcept": "subject[x]",
	}

	// Check if this field is a known choice type
	if choicePattern, isChoice := choicePatterns[fieldPath]; isChoice {
		// Try to find the choice pattern in schema
		choicePath := fmt.Sprintf("%s.%s", schema.ResourceType, choicePattern)
		if element, exists := schema.Elements[choicePath]; exists {
			log.Printf("✅ Found choice type: %s → %s", fieldPath, choicePath)
			return element, true
		}

		// Try without resource prefix
		if element, exists := schema.Elements[choicePattern]; exists {
			log.Printf("✅ Found choice type: %s → %s", fieldPath, choicePattern)
			return element, true
		}
	}

	// Fallback: try to infer choice pattern from field name
	if strings.HasSuffix(fieldPath, "Coding") || strings.HasSuffix(fieldPath, "Uri") ||
		strings.HasSuffix(fieldPath, "Canonical") || strings.HasSuffix(fieldPath, "String") ||
		strings.HasSuffix(fieldPath, "Boolean") || strings.HasSuffix(fieldPath, "Integer") ||
		strings.HasSuffix(fieldPath, "DateTime") || strings.HasSuffix(fieldPath, "Date") ||
		strings.HasSuffix(fieldPath, "Reference") || strings.HasSuffix(fieldPath, "CodeableConcept") {

		// Extract base name (e.g., "eventCoding" -> "event")
		baseName := s.extractChoiceBaseName(fieldPath)
		if baseName != "" {
			choicePattern := baseName + "[x]"
			choicePath := fmt.Sprintf("%s.%s", schema.ResourceType, choicePattern)

			if element, exists := schema.Elements[choicePath]; exists {
				log.Printf("✅ Inferred choice type: %s → %s", fieldPath, choicePath)
				return element, true
			}
		}
	}

	return nil, false
}

// Extract base name from choice type field (e.g., "eventCoding" -> "event")
func (s *HL7FHIRTransformServiceV3) extractChoiceBaseName(fieldPath string) string {
	commonSuffixes := []string{
		"Coding", "Uri", "Canonical", "String", "Code", "Boolean",
		"Integer", "Decimal", "DateTime", "Date", "Time", "Period",
		"Reference", "Quantity", "Range", "Ratio", "Attachment",
		"CodeableConcept", "Identifier",
	}

	for _, suffix := range commonSuffixes {
		if strings.HasSuffix(fieldPath, suffix) {
			return strings.TrimSuffix(fieldPath, suffix)
		}
	}

	return ""
}

// Schema-driven nested field setting (e.g., "source.name", "destination.endpoint")
func (s *HL7FHIRTransformServiceV3) setNestedFieldFromSchema(
	resource map[string]interface{},
	fieldPath string,
	value interface{},
	schema *fhir.FHIRSchema,
) error {

	parts := strings.Split(fieldPath, ".")

	// Handle deep nesting (3+ levels) by recursively creating objects
	if len(parts) > 2 {
		return s.setDeeplyNestedField(resource, parts, value, schema)
	}

	if len(parts) < 2 {
		// Single field, set directly
		resource[fieldPath] = value
		return nil
	}

	parentField := parts[0]
	childField := parts[1]

	// ✅ FIX: If parentField is the resource type itself, treat this as a direct field
	if parentField == schema.ResourceType {
		// This is a direct field like "Patient.birthDate" or "PractitionerRole.code"
		log.Printf("🔧 Direct resource field: %s.%s -> setting %s directly", parentField, childField, childField)

		// Validate the child field exists in schema
		childPath := fmt.Sprintf("%s.%s", schema.ResourceType, childField)
		elem, exists := schema.Elements[childPath]
		if !exists {
			log.Printf("❌ REJECTED: Field %s not found in %s schema", childPath, schema.ResourceType)
			return fmt.Errorf("field %s not found in FHIR schema for %s", childField, schema.ResourceType)
		}

		// Enforce cardinality: 0..* / 1..* fields must be serialised as JSON arrays.
		// Without this check the field mapper sets them as plain objects when the
		// FHIR path carries no "[0]" index notation (e.g. PractitionerRole.code).
		if strings.Contains(elem.Cardinality, "*") {
			return s.setArrayField(resource, childField, value)
		}
		resource[childField] = value
		return nil
	}

	// Check if parent field exists in schema - STRICT VALIDATION for nested objects
	parentPath := s.normalizeFieldPath(parentField, schema.ResourceType)
	parentElement, parentExists := schema.Elements[parentPath]

	if !parentExists {
		// ✅ Try to find choice type element (e.g., "eventCoding" -> "event[x]")
		choiceElement, choiceExists := s.findChoiceTypeElement(parentField, schema)
		if choiceExists {
			parentElement = choiceElement
			parentExists = true
			log.Printf("✅ Found choice type parent for %s", parentField)
		}
	}

	if !parentExists {
		// ✅ STRICT: Reject nested fields where parent doesn't exist in schema
		log.Printf("❌ REJECTED: Parent field %s not found in %s schema", parentPath, schema.ResourceType)
		return fmt.Errorf("parent field %s not found in FHIR schema for %s", parentField, schema.ResourceType)
	}

	// Check if child field exists in schema too
	// Note: For complex types (arrays of objects), child validation may be relaxed
	childPath := fmt.Sprintf("%s.%s", parentPath, childField)
	_, childExists := schema.Elements[childPath]

	// ✅ FIX: If parent is a complex data type (Coding, HumanName, etc.), allow standard properties
	// Complex types like Coding have properties (code, display, system) that aren't in the resource schema
	if !childExists && s.isComplexDataType(parentElement.DataType) {
		// Allow standard properties for complex types
		if s.isStandardComplexTypeProperty(parentElement.DataType, childField) {
			log.Printf("✅ Allowing standard property %s for complex type %s", childField, parentElement.DataType)
			childExists = true
		}
	}

	// If parent is an array of complex types, don't strict validate children
	// (e.g., name is HumanName[], so name.family may not be directly in schema)
	if !childExists && !strings.Contains(parentElement.Cardinality, "*") {
		// Only reject for non-array parents
		log.Printf("❌ REJECTED: Child field %s not found in %s schema", childPath, schema.ResourceType)
		return fmt.Errorf("child field %s not found in FHIR schema for %s", childPath, schema.ResourceType)
	}

	if !childExists {
		log.Printf("ℹ️  Child field %s not in schema, but parent is array - allowing (complex type)", childPath)
	}

	// Create parent object if it doesn't exist
	if _, exists := resource[parentField]; !exists {
		// Check if parent is array type
		if strings.Contains(parentElement.Cardinality, "*") {
			resource[parentField] = []map[string]interface{}{}
		} else {
			resource[parentField] = map[string]interface{}{}
		}
	}

	// Set child value based on parent type
	if strings.Contains(parentElement.Cardinality, "*") {
		// Parent is array - handle array of objects
		return s.setNestedArrayField(resource, parentField, childField, value, schema)
	} else {
		// Parent is single object
		parentObj, ok := resource[parentField].(map[string]interface{})
		if !ok {
			return fmt.Errorf("parent field %s is not an object", parentField)
		}
		parentObj[childField] = value
		log.Printf("✅ Set nested field: %s.%s = %v", parentField, childField, value)
		return nil
	}
}

// Handle paths with array indices like "name[0].given[1]"
func (s *HL7FHIRTransformServiceV3) setFieldWithArrayIndices(
	resource map[string]interface{},
	fieldPath string,
	value interface{},
	schema *fhir.FHIRSchema,
) error {
	// Parse path with array indices: "name[0].given[1]" -> segments with indices
	// Use regex to split by dots while preserving array indices
	parts := strings.Split(fieldPath, ".")

	current := resource
	for i := 0; i < len(parts); i++ {
		part := parts[i]

		// Check if this part has an array index like "name[0]" or "given[1]"
		if strings.Contains(part, "[") {
			// Extract field name and index
			openBracket := strings.Index(part, "[")
			closeBracket := strings.Index(part, "]")
			if openBracket == -1 || closeBracket == -1 {
				return fmt.Errorf("invalid array syntax in path: %s", part)
			}

			fieldName := part[:openBracket]
			indexStr := part[openBracket+1 : closeBracket]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return fmt.Errorf("invalid array index: %s", indexStr)
			}

			// Ensure array exists
			if _, exists := current[fieldName]; !exists {
				current[fieldName] = []interface{}{}
			}

			// Get array
			arr, ok := current[fieldName].([]interface{})
			if !ok {
				return fmt.Errorf("field %s is not an array", fieldName)
			}

			// Extend array if needed
			for len(arr) <= index {
				arr = append(arr, map[string]interface{}{})
			}
			current[fieldName] = arr

			// If this is the last part, set the value
			if i == len(parts)-1 {
				arr[index] = value
				current[fieldName] = arr
				log.Printf("✅ Set array field: %s = %v", fieldPath, value)
				return nil
			}

			// Navigate into the array element
			elem, ok := arr[index].(map[string]interface{})
			if !ok {
				// Create a new map at this index
				elem = map[string]interface{}{}
				arr[index] = elem
			}
			current = elem

		} else {
			// Simple field name without array index
			if i == len(parts)-1 {
				// Last part - set the value
				current[part] = value
				log.Printf("✅ Set field: %s = %v", fieldPath, value)
				return nil
			}

			// Navigate deeper
			if _, exists := current[part]; !exists {
				current[part] = map[string]interface{}{}
			}

			next, ok := current[part].(map[string]interface{})
			if !ok {
				return fmt.Errorf("field %s is not an object", part)
			}
			current = next
		}
	}

	return nil
}

// Handle deeply nested fields (3+ levels) like "identifier.type.coding.code"
func (s *HL7FHIRTransformServiceV3) setDeeplyNestedField(
	resource map[string]interface{},
	parts []string,
	value interface{},
	schema *fhir.FHIRSchema,
) error {
	if len(parts) == 0 {
		return fmt.Errorf("empty field path")
	}

	// Navigate/create the nested structure
	current := resource
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]

		// Check if current level exists
		if _, exists := current[part]; !exists {
			// Create new object at this level
			current[part] = map[string]interface{}{}
		}

		// Try to cast to map for next level
		nextMap, ok := current[part].(map[string]interface{})
		if !ok {
			// Current level is not an object, can't nest further
			log.Printf("⚠️  Cannot nest into %s - not an object", strings.Join(parts[:i+1], "."))
			// Fall back to setting as flat string
			fullPath := strings.Join(parts, ".")
			resource[fullPath] = value
			return nil
		}

		current = nextMap
	}

	// Set the final value
	finalField := parts[len(parts)-1]
	current[finalField] = value
	log.Printf("✅ Set deeply nested field: %s = %v", strings.Join(parts, "."), value)
	return nil
}

// Handle nested array fields (e.g., destination.name where destination is array)
func (s *HL7FHIRTransformServiceV3) setNestedArrayField(
	resource map[string]interface{},
	parentField string,
	childField string,
	value interface{},
	schema *fhir.FHIRSchema,
) error {

	parentArray, ok := resource[parentField].([]map[string]interface{})
	if !ok {
		return fmt.Errorf("parent field %s is not an array of objects", parentField)
	}

	// For now, add to first object in array or create new object
	if len(parentArray) == 0 {
		// Create first object in array
		newObj := map[string]interface{}{
			childField: value,
		}
		resource[parentField] = []map[string]interface{}{newObj}
	} else {
		// Add to existing first object
		parentArray[0][childField] = value
	}

	log.Printf("✅ Set nested array field: %s[0].%s = %v", parentField, childField, value)
	return nil
}

func (s *HL7FHIRTransformServiceV3) setArrayField(
	resource map[string]interface{},
	fieldName string,
	value interface{},
) error {

	// Initialize array if it doesn't exist
	if _, exists := resource[fieldName]; !exists {
		resource[fieldName] = []interface{}{}
	}

	// Get existing array
	existingArray, ok := resource[fieldName].([]interface{})
	if !ok {
		return fmt.Errorf("field %s is not an array", fieldName)
	}

	// Handle different value types appropriately
	switch v := value.(type) {
	case []interface{}:
		// If transformation returned an array, add each element individually
		for _, item := range v {
			if item != nil {
				existingArray = append(existingArray, item)
			}
		}
	case []map[string]interface{}:
		// Handle array of maps (common for contact points, identifiers)
		for _, item := range v {
			if item != nil {
				existingArray = append(existingArray, item)
			}
		}
	case map[string]interface{}:
		// Single object, add directly
		if v != nil {
			existingArray = append(existingArray, v)
		}
	case nil:
		// Skip nil values
		return nil
	default:
		// Other single values
		existingArray = append(existingArray, v)
	}

	resource[fieldName] = existingArray
	return nil
}

// =====================================
// FIELD PATH HELPERS
// =====================================

func (s *HL7FHIRTransformServiceV3) normalizeFieldPath(fieldPath, resourceType string) string {
	// Fix double resource name issue: "Patient.Patient.identifier" -> "Patient.identifier"
	doubleResourcePrefix := resourceType + "." + resourceType + "."
	if strings.HasPrefix(fieldPath, doubleResourcePrefix) {
		// Remove the double resource prefix and re-add single prefix
		withoutDouble := strings.TrimPrefix(fieldPath, doubleResourcePrefix)
		return fmt.Sprintf("%s.%s", resourceType, withoutDouble)
	}

	if strings.HasPrefix(fieldPath, resourceType+".") {
		return fieldPath // Already normalized
	}
	return fmt.Sprintf("%s.%s", resourceType, fieldPath)
}

func (s *HL7FHIRTransformServiceV3) getResourceFieldName(fullPath string) string {
	// Remove resource type prefix: "Patient.telecom" -> "telecom"
	if parts := strings.SplitN(fullPath, ".", 2); len(parts) == 2 {
		return parts[1]
	}
	return fullPath
}

// =====================================
// SCHEMA VALIDATION
// =====================================

func (s *HL7FHIRTransformServiceV3) validateResourceAgainstSchema(
	resource map[string]interface{},
	schema *fhir.FHIRSchema,
	warnings *[]string,
) []string {

	var validationErrors []string

	// Check required fields from schema
	for _, requiredField := range schema.Required {
		// Check if this is a nested field (e.g., "MessageHeader.destination.endpoint")
		parts := strings.Split(requiredField, ".")
		if len(parts) > 2 {
			// This is a nested required field - check if parent exists first
			// e.g., "MessageHeader.destination.endpoint" -> check if "destination" exists
			parentField := parts[1]

			// If parent doesn't exist, skip validation for this nested required field
			if _, parentExists := resource[parentField]; !parentExists {
				continue // Parent is optional and not present, so child requirement doesn't apply
			}

			// Parent exists, now check if the nested field exists
			// For now, we'll skip deep validation of nested structures
			// This prevents false positives for conditional requirements
			continue
		}

		// For top-level required fields, validate normally
		normalizedField := s.getResourceFieldName(requiredField) // Remove "MessageHeader." prefix

		// ✅ Handle choice types like event[x] - check if ANY variant exists
		if strings.HasSuffix(normalizedField, "[x]") {
			// This is a choice type - check for any variant
			baseField := strings.TrimSuffix(normalizedField, "[x]")
			hasAnyVariant := false

			// Check all possible variants of the choice type
			for fieldName := range resource {
				if strings.HasPrefix(fieldName, baseField) {
					hasAnyVariant = true
					log.Printf("✅ Found choice type variant: %s for required field %s", fieldName, requiredField)
					break
				}
			}

			if !hasAnyVariant {
				validationError := fmt.Sprintf("FHIR Validation: Required field %s missing from resource", requiredField)
				validationErrors = append(validationErrors, validationError)
				*warnings = append(*warnings, fmt.Sprintf("Required field %s missing from FHIR resource", requiredField))
			}
		} else {
			// Regular field - check directly
			if _, hasValue := resource[normalizedField]; !hasValue {
				validationError := fmt.Sprintf("FHIR Validation: Required field %s missing from resource", requiredField)
				validationErrors = append(validationErrors, validationError)
				*warnings = append(*warnings, fmt.Sprintf("Required field %s missing from FHIR resource", requiredField))
			}
		}
	}

	// Check for properties that don't exist in schema
	for fieldName := range resource {
		if fieldName == "resourceType" || fieldName == "id" || fieldName == "text" ||
			fieldName == "meta" || fieldName == "extension" || fieldName == "modifierExtension" ||
			fieldName == "contained" || fieldName == "implicitRules" || fieldName == "language" {
			continue // Always-valid FHIR base properties
		}
		// Skip resourceType-named sub-objects (internal mapping engine artefact)
		if fieldName == schema.ResourceType {
			continue
		}

		// Check if field exists in schema
		fieldPath := fmt.Sprintf("%s.%s", schema.ResourceType, fieldName)
		_, exists := schema.Elements[fieldPath]

		// ✅ If not found directly, check if it's a choice type variant (e.g., eventCoding for event[x])
		if !exists {
			choiceElement, choiceExists := s.findChoiceTypeElement(fieldName, schema)
			if choiceExists {
				exists = true
				log.Printf("✅ Validated choice type property: %s (maps to %s)", fieldName, choiceElement.Path)
			}
		}

		if !exists {
			validationError := fmt.Sprintf("FHIR Validation: Property %s does not exist in %s schema", fieldName, schema.ResourceType)
			validationErrors = append(validationErrors, validationError)
		}
	}

	return validationErrors
}

// ✅ SCHEMA-DRIVEN: Simple required field validation - no defaults, no database config
func (s *HL7FHIRTransformServiceV3) ensureRequiredFieldsFromSchema(
	resource map[string]interface{},
	schema *fhir.FHIRSchema,
	warnings *[]string,
	errors *[]string,
) {

	for _, requiredFieldPath := range schema.Required {
		if s.isRequiredFieldSatisfied(resource, requiredFieldPath) {
			continue // Field requirement is satisfied
		}

		// Determine if this is a parent or child field
		if strings.Contains(requiredFieldPath, ".") {
			// Child field - only required if parent exists
			s.handleRequiredChildField(resource, requiredFieldPath, warnings)
		} else if strings.Contains(requiredFieldPath, "[x]") {
			// Choice field - check if any choice is present
			s.handleRequiredChoiceField(resource, requiredFieldPath, errors)
		} else {
			// Parent field - absolutely required
			s.handleRequiredParentField(resource, requiredFieldPath, errors)
		}
	}
}

// Check if a required field is satisfied
func (s *HL7FHIRTransformServiceV3) isRequiredFieldSatisfied(resource map[string]interface{}, requiredFieldPath string) bool {
	// Handle choice types like event[x]
	if strings.Contains(requiredFieldPath, "[x]") {
		if strings.Contains(requiredFieldPath, "event[x]") {
			// Check if ANY event field exists
			eventFields := []string{"eventCoding", "eventUri", "eventCanonical"}
			for _, eventField := range eventFields {
				if _, exists := resource[eventField]; exists {
					return true
				}
			}
		}
		return false
	}

	// Handle nested fields like "source.endpoint"
	if strings.Contains(requiredFieldPath, ".") {
		return s.nestedFieldExists(resource, requiredFieldPath)
	}

	// Handle simple fields
	fieldName := s.getResourceFieldName(requiredFieldPath)
	_, exists := resource[fieldName]
	return exists
}

// Handle required parent fields - these MUST exist
func (s *HL7FHIRTransformServiceV3) handleRequiredParentField(
	resource map[string]interface{},
	fieldPath string,
	errors *[]string,
) {
	errorMsg := fmt.Sprintf("CRITICAL: Required parent field %s missing - no HL7 mapping found", fieldPath)
	*errors = append(*errors, errorMsg)
	log.Printf("❌ %s", errorMsg)
}

// Handle required choice fields like event[x] - one choice MUST exist
func (s *HL7FHIRTransformServiceV3) handleRequiredChoiceField(
	resource map[string]interface{},
	fieldPath string,
	errors *[]string,
) {
	if strings.Contains(fieldPath, "event[x]") {
		errorMsg := fmt.Sprintf("CRITICAL: Required field %s missing - need HL7 mapping like MSH.9.2 → eventCoding", fieldPath)
		*errors = append(*errors, errorMsg)
		log.Printf("❌ %s", errorMsg)
	}
}

// Handle required child fields - only required if parent exists
func (s *HL7FHIRTransformServiceV3) handleRequiredChildField(
	resource map[string]interface{},
	fieldPath string,
	warnings *[]string,
) {
	parts := strings.Split(s.getResourceFieldName(fieldPath), ".")
	parentField := parts[0]

	// Check if parent exists
	if _, parentExists := resource[parentField]; !parentExists {
		log.Printf("✅ Child field %s not required - parent %s doesn't exist", fieldPath, parentField)
		return
	}

	// Parent exists, so child is required
	warningMsg := fmt.Sprintf("Required child field %s missing - need HL7 mapping", fieldPath)
	*warnings = append(*warnings, warningMsg)
	log.Printf("⚠️ %s", warningMsg)
}

// Check if nested field exists (reuse existing method)
func (s *HL7FHIRTransformServiceV3) nestedFieldExists(resource map[string]interface{}, fieldPath string) bool {
	parts := strings.Split(s.getResourceFieldName(fieldPath), ".")

	current := resource
	for _, part := range parts {
		if obj, ok := current[part].(map[string]interface{}); ok {
			current = obj
		} else if i, ok := current[part]; ok && i != nil {
			// Handle arrays - check if first element has the nested structure
			if arr, ok := i.([]interface{}); ok && len(arr) > 0 {
				if obj, ok := arr[0].(map[string]interface{}); ok {
					current = obj
				} else {
					return false
				}
			} else {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

// =====================================
// SCHEMA LOADING HELPERS
// =====================================

func (s *HL7FHIRTransformServiceV3) loadFHIRSchema(resourceType, profile, version string) (*fhir.FHIRSchema, error) {
	if profile == "" {
		profile = "base"
	}
	if version == "" {
		version = "R4"
	}

	schema, err := s.fhirLoader.LoadFHIRSchema(resourceType, profile, version)
	if err != nil {
		// Try fallback to base profile
		if profile != "base" {
			return s.fhirLoader.LoadFHIRSchema(resourceType, "base", version)
		}
		return nil, err
	}

	return schema, nil
}

func (s *HL7FHIRTransformServiceV3) extractResourceTypes(mappings []FieldMapping) []string {
	resourceTypeSet := make(map[string]bool)
	var resourceTypes []string

	for _, mapping := range mappings {
		if !resourceTypeSet[mapping.FHIRResourceType] {
			resourceTypes = append(resourceTypes, mapping.FHIRResourceType)
			resourceTypeSet[mapping.FHIRResourceType] = true
		}
	}

	return resourceTypes
}

func (s *HL7FHIRTransformServiceV3) filterMappingsForResource(mappings []FieldMapping, resourceType string) []FieldMapping {
	var filtered []FieldMapping
	for _, mapping := range mappings {
		if mapping.FHIRResourceType == resourceType {
			filtered = append(filtered, mapping)
		}
	}
	return filtered
}

// =====================================
// FIELD MAPPING & HL7 EXTRACTION (Reused from V1)
// =====================================

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

// extractFieldMappingsFromWizardArray converts wizard-saved array format to FieldMapping array
// Wizard format: [{ "sourcePath": "PID.3", "targetPath": "Patient.identifier[0].value", "transformType": "string_direct", "isRequired": true }, ...]
func (s *HL7FHIRTransformServiceV3) extractFieldMappingsFromWizardArray(mappingArray []map[string]interface{}) []FieldMapping {
	var mappings []FieldMapping

	for idx, item := range mappingArray {
		sourcePath, _ := item["sourcePath"].(string)
		targetPath, _ := item["targetPath"].(string)
		transformType, _ := item["transformType"].(string)
		isRequired, _ := item["isRequired"].(bool)

		if sourcePath == "" || targetPath == "" {
			log.Printf("⚠️ V3 Service: Skipping item %d - sourcePath='%s', targetPath='%s'", idx, sourcePath, targetPath)
			continue
		}

		log.Printf("🔍 V3 Service: Processing item %d - source='%s', target='%s', transform='%s'", idx, sourcePath, targetPath, transformType)

		// Parse sourcePath (e.g., "PID.3.1" -> segment="PID", field="3", component="1")
		sourceParts := strings.Split(sourcePath, ".")
		if len(sourceParts) < 2 {
			continue
		}

		segmentName := sourceParts[0]
		fieldNum := sourceParts[1]
		component := ""
		if len(sourceParts) > 2 {
			component = sourceParts[2]
		}

		// Parse targetPath (e.g., "Patient.identifier[0].value" -> resource="Patient", path="identifier[0].value")
		// KEEP array indices in the path - we'll handle them during field setting
		targetParts := strings.Split(targetPath, ".")
		if len(targetParts) < 2 {
			continue
		}

		resourceType := targetParts[0]
		elementPath := strings.Join(targetParts[1:], ".")

		// Create mapping
		mapping := FieldMapping{
			SegmentName:       segmentName,
			HL7Field:          fieldNum,
			HL7Component:      component,
			FHIRResourceType:  resourceType,
			FHIRElementPath:   elementPath,
			DataTypeTransform: transformType,
			IsRequired:        isRequired,
		}

		mappings = append(mappings, mapping)
	}

	return mappings
}

// extractFieldMappingsFromWizardConfig converts wizard-saved mapping format to FieldMapping array
func (s *HL7FHIRTransformServiceV3) extractFieldMappingsFromWizardConfig(mappingData map[string]interface{}) []FieldMapping {
	var mappings []FieldMapping

	// The wizard saves mappings in format: { "PID.3": { "fhirPath": "Patient.identifier", ... }, ... }
	for hl7Path, mappingValue := range mappingData {
		mappingObj, ok := mappingValue.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract segment name and field from HL7 path (e.g., "PID.3" -> segment="PID", field="3")
		parts := strings.Split(hl7Path, ".")
		if len(parts) < 2 {
			continue
		}

		segmentName := parts[0]
		fieldNum := parts[1]

		// Extract FHIR path (e.g., "Patient.identifier")
		fhirPath, _ := mappingObj["fhirPath"].(string)
		if fhirPath == "" {
			continue
		}

		// Extract resource type from FHIR path
		fhirParts := strings.Split(fhirPath, ".")
		if len(fhirParts) < 2 {
			continue
		}

		resourceType := fhirParts[0]
		elementPath := strings.Join(fhirParts[1:], ".")

		// Create mapping
		mapping := FieldMapping{
			SegmentName:       segmentName,
			HL7Field:          fieldNum,
			HL7Component:      "", // Can be extended from mappingObj if needed
			FHIRResourceType:  resourceType,
			FHIRElementPath:   elementPath,
			DataTypeTransform: getDataTypeTransform(mappingObj),
			IsRequired:        getIsRequired(mappingObj),
		}

		mappings = append(mappings, mapping)
	}

	return mappings
}

func getDataTypeTransform(mappingObj map[string]interface{}) string {
	if transform, ok := mappingObj["transform"].(string); ok {
		return transform
	}
	return ""
}

func getIsRequired(mappingObj map[string]interface{}) bool {
	if required, ok := mappingObj["required"].(bool); ok {
		return required
	}
	return false
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

			// condition: only emit the mapping when the HL7 condition field is non-empty
			if cond, _ := mapping["condition"].(string); cond != "" {
				transformRules["condition"] = cond
			}

			fieldMapping := FieldMapping{
				SegmentName:         segmentName,
				HL7Field:            hl7Field,
				HL7Component:        hl7Component,
				FHIRResourceType:    resourceType,
				FHIRElementPath:     fhirPath,
				DataTypeTransform:   transformFunc,
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
func extractContextLinks(templateData map[string]interface{}) *ContextLinks {
	contextRaw, hasContext := templateData["context"]
	if !hasContext {
		return nil // v1.0 template — existing hardcoded wiring applies
	}
	contextMap, ok := contextRaw.(map[string]interface{})
	if !ok || len(contextMap) == 0 {
		return nil
	}

	cl := &ContextLinks{
		RoleToSegment: make(map[string]string),
		ResourceLinks: make(map[string]map[string]string),
	}

	for role, segRaw := range contextMap {
		if seg, ok := segRaw.(string); ok {
			cl.RoleToSegment[role] = seg
		}
	}

	if resourcesRaw, ok := templateData["resources"]; ok {
		if resources, ok := resourcesRaw.(map[string]interface{}); ok {
			for resourceType, resourceDataRaw := range resources {
				resourceData, ok := resourceDataRaw.(map[string]interface{})
				if !ok {
					continue
				}
				linksRaw, ok := resourceData["contextLinks"]
				if !ok {
					continue
				}
				linksMap, ok := linksRaw.(map[string]interface{})
				if !ok {
					continue
				}
				links := make(map[string]string)
				for field, roleRaw := range linksMap {
					if role, ok := roleRaw.(string); ok {
						links[field] = role
					}
				}
				if len(links) > 0 {
					cl.ResourceLinks[resourceType] = links
				}
			}
		}
	}

	if len(cl.RoleToSegment) == 0 {
		return nil
	}
	return cl
}

// applyContextLinks wires FHIR cross-resource references declared in the
// template's contextLinks block.  It finds the ID of each context role's
// resource in the already-built slice, then sets the declared FHIR fields to
// "ResourceType/id" references.  Fields already populated by the mapping engine
// are not overwritten.
//
// This is a no-op when cl is nil (v1.0 templates or templates with no links).
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
func (s *HL7FHIRTransformServiceV3) parseHL7Path(hl7Path string) (segment, field, component string) {
	parts := strings.Split(hl7Path, ".")
	if len(parts) < 2 {
		return hl7Path, "", ""
	}

	segment = parts[0]
	field = parts[1]

	if len(parts) >= 3 {
		component = parts[2]
	}

	return segment, field, component
}

// Helper function to get keys from map
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// =====================================
// UTILITY METHODS (Reused)
// =====================================

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
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// isComplexDataType checks if the data type is a FHIR complex type (not a primitive)
func (s *HL7FHIRTransformServiceV3) isComplexDataType(dataType string) bool {
	complexTypes := map[string]bool{
		"Coding":           true,
		"CodeableConcept":  true,
		"HumanName":        true,
		"Address":          true,
		"ContactPoint":     true,
		"Identifier":       true,
		"Reference":        true,
		"Period":           true,
		"Quantity":         true,
		"Range":            true,
		"Ratio":            true,
		"SampledData":      true,
		"Attachment":       true,
		"Annotation":       true,
		"Signature":        true,
		"Timing":           true,
		"Age":              true,
		"Distance":         true,
		"Duration":         true,
		"Count":            true,
		"Money":            true,
		"ContactDetail":    true,
		"Contributor":      true,
		"DataRequirement":  true,
		"Expression":       true,
		"ParameterDefinition": true,
		"RelatedArtifact":  true,
		"TriggerDefinition": true,
		"UsageContext":     true,
		"Dosage":           true,
		"Meta":             true,
	}
	return complexTypes[dataType]
}

// isStandardComplexTypeProperty checks if a field is a standard property of a FHIR complex type
func (s *HL7FHIRTransformServiceV3) isStandardComplexTypeProperty(dataType, property string) bool {
	// Define standard properties for each complex type
	standardProperties := map[string]map[string]bool{
		"Coding": {
			"system":       true,
			"version":      true,
			"code":         true,
			"display":      true,
			"userSelected": true,
		},
		"CodeableConcept": {
			"coding": true,
			"text":   true,
		},
		"HumanName": {
			"use":    true,
			"text":   true,
			"family": true,
			"given":  true,
			"prefix": true,
			"suffix": true,
			"period": true,
		},
		"Address": {
			"use":        true,
			"type":       true,
			"text":       true,
			"line":       true,
			"city":       true,
			"district":   true,
			"state":      true,
			"postalCode": true,
			"country":    true,
			"period":     true,
		},
		"ContactPoint": {
			"system": true,
			"value":  true,
			"use":    true,
			"rank":   true,
			"period": true,
		},
		"Identifier": {
			"use":      true,
			"type":     true,
			"system":   true,
			"value":    true,
			"period":   true,
			"assigner": true,
		},
		"Reference": {
			"reference": true,
			"type":      true,
			"identifier": true,
			"display":   true,
		},
		"Period": {
			"start": true,
			"end":   true,
		},
		"Quantity": {
			"value":      true,
			"comparator": true,
			"unit":       true,
			"system":     true,
			"code":       true,
		},
	}

	if props, exists := standardProperties[dataType]; exists {
		return props[property]
	}
	return false
}

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
		// Build HL7 source path from segment and field components
		sourcePath := fieldMapping.SegmentName + "." + fieldMapping.HL7Field
		if fieldMapping.HL7Component != "" {
			sourcePath += "." + fieldMapping.HL7Component
		}
		if fieldMapping.HL7SubComponent != "" {
			sourcePath += "." + fieldMapping.HL7SubComponent
		}

		atomicMappings[i] = AtomicMapping{
			ID:               fmt.Sprintf("%d", fieldMapping.ID),
			SourcePath:       sourcePath,
			TargetPath:       fieldMapping.FHIRElementPath,
			ResourceType:     fieldMapping.FHIRResourceType,
			FHIRResourceType: fieldMapping.FHIRResourceType,
			TransformType:    fieldMapping.DataTypeTransform,
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

	// Assign a urn:uuid: fullUrl to every entry and build a lookup map so we can
	// rewrite all internal ResourceType/id references to their urn:uuid: equivalents.
	// FHIR spec (3.3.2.1): when fullUrl is urn:uuid:, relative references like
	// "Patient/id" are resolved against a base of "urn:uuid:" — they never match.
	// Every internal reference MUST use the same urn:uuid: that was assigned here.
	shortToUUID := make(map[string]string, len(sorted)) // "Patient/patient-123" → "urn:uuid:..."
	entries := make([]map[string]interface{}, len(sorted))
	for i, resource := range sorted {
		fullURL := "urn:uuid:" + uuid.New().String()
		rt, _ := resource["resourceType"].(string)
		id, _ := resource["id"].(string)
		if rt != "" && id != "" {
			shortToUUID[rt+"/"+id] = fullURL
		}
		entries[i] = map[string]interface{}{
			"fullUrl":  fullURL,
			"resource": resource,
		}
	}

	// Rewrite all internal references in every resource to use urn:uuid: form.
	for _, entry := range entries {
		if res, ok := entry["resource"].(map[string]interface{}); ok {
			rewriteReferences(res, shortToUUID)
		}
	}

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

// rewriteReferences walks any FHIR resource map and replaces "reference" values
// of the form "ResourceType/id" with their urn:uuid: equivalents from the lookup.
// This is called after all entries have been assigned their fullUrls so that
// subject, result[], performer[], etc. all point to the correct urn:uuid:.
func rewriteReferences(node interface{}, lookup map[string]string) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		if ref, ok := v["reference"].(string); ok {
			if uuid, found := lookup[ref]; found {
				v["reference"] = uuid
			}
		}
		for k, child := range v {
			v[k] = rewriteReferences(child, lookup)
		}
	case []interface{}:
		for i, item := range v {
			v[i] = rewriteReferences(item, lookup)
		}
	}
	return node
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
		if m.HL7DataType == "" && hl7Loader != nil && m.SegmentName != "" && m.HL7Field != "" {
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
// DATA TYPE TRANSFORMERS (Complete Implementation)
// =====================================

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

// transformCXToIdentifier converts one CX repetition to a FHIR Identifier map.
// Delegates to the canonical hl7assembly.BuildIdentifierFromCX which correctly maps:
//   CX.1 → value, CX.4 (Assigning Authority HD) → system, CX.5 (Table 0203 type) → type
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
	"http://loinc.org":                           true,
	"http://snomed.info/sct":                     true,
	"http://www.ama-assn.org/go/cpt":             true,
	"http://www.nlm.nih.gov/research/umls/rxnorm": true,
	"http://hl7.org/fhir/sid/icd-10-cm":          true,
	"http://hl7.org/fhir/sid/icd-9-cm":           true,
	"http://hl7.org/fhir/sid/ndc":                true,
	"http://hl7.org/fhir/sid/cvx":                true,
	"http://hl7.org/fhir/sid/mvx":                true,
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
		quantity := map[string]interface{}{"value": value}
		if rules != nil {
			if unit, ok := rules["unit"].(string); ok && unit != "" {
				quantity["unit"] = unit
				quantity["system"] = "http://unitsofmeasure.org"
				quantity["code"] = unit
			}
		}
		return map[string]interface{}{"valueQuantity": quantity}
	case "CE", "CWE", "CNE": // Coded → valueCodeableConcept
		return map[string]interface{}{"valueCodeableConcept": s.transformCEToCodeableConcept(value, rules)}
	case "ST", "TX", "FT": // String/Text → valueString
		return map[string]interface{}{"valueString": value}
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
		// No OBX.2 known — return as string
		return map[string]interface{}{"valueString": value}
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
// DEFAULT MAPPINGS FOR TESTING
// =====================================

// getDefaultMappingsForTesting provides hardcoded mappings when database is not available
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

// =====================================
// SPEC-DRIVEN NORMALIZER HELPERS
// =====================================

// hl7v2IdentifierTypeSystem and table0203 are package-level aliases pointing at the
// canonical definitions in hl7assembly/datatypes.go.  Use hl7assembly.HL7V2IdentifierTypeSystem
// and hl7assembly.Table0203 directly in new code.
const hl7v2IdentifierTypeSystem = hl7assembly.HL7V2IdentifierTypeSystem

var table0203 = hl7assembly.Table0203

// table0202 maps HL7 Table 0202 (telecommunication use codes) → FHIR ContactPoint.use
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
}

// table0201 maps HL7 Table 0201 (telecommunication equipment type) → FHIR ContactPoint.system
var table0201 = map[string]string{
	"PH":  "phone",
	"FX":  "fax",
	"MD":  "other",
	"CP":  "phone",
	"BP":  "pager",
	"X.400": "email",
	"Internet": "email",
	"internet": "email",
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
		codings, _ := typeObj["coding"].([]interface{})
		if len(codings) == 0 {
			continue // no code to enrich
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
		// Normalize identifier.system: bare OID → urn:oid:
		if sys, ok2 := idMap["system"].(string); ok2 && sys != "" {
			idMap["system"] = normalizeIdentifierSystem(sys)
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

// normalizeIdentifierSystem is a package-level shim delegating to the canonical
// hl7assembly.NormalizeIdentifierSystem.  All new code should call that function directly.
func normalizeIdentifierSystem(sys string) string {
	return hl7assembly.NormalizeIdentifierSystem(sys)
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

	// PID.13: home phone, PID.14: work phone, PID.40: email / alternate phone
	type pidTelecom struct {
		pidKey      string
		defaultUse  string
		defaultSys  string
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

// =====================================
// HELPER FUNCTIONS
// =====================================

// ─────────────────────────────────────────────────────────────────────────────
// PROFILE-DRIVEN COMPOSITE ENGINE
//
// Reads the v2.0 extended template_config from hl7_fhir_templates and
// executes composite rules (condition, forEach, firstOf) to build FHIR
// arrays (participants, identifiers, etc.) without any hardcoded assembly.
// ─────────────────────────────────────────────────────────────────────────────

// AssemblyProcedureFunc is a named, parameterised assembly procedure that can be
// referenced from a v2.0 profile composite using "procedure": "<name>".
// It receives the full parsed HL7 data, the current resource slice, and any
// JSON params from the profile.  Returns the updated resource slice + warnings.
type AssemblyProcedureFunc func(
	parsedHL7 map[string]interface{},
	resources []map[string]interface{},
	params map[string]interface{},
) ([]map[string]interface{}, []string)

// assemblyProcedureRegistry holds all named assembly procedures registered at
// init() time from services/hl7assembly/procedure_registry.go.
var assemblyProcedureRegistry = map[string]AssemblyProcedureFunc{}

// RegisterAssemblyProcedure makes a named procedure available to the profile engine.
// Called from init() in procedure_registry.go so registration is automatic.
func RegisterAssemblyProcedure(name string, fn AssemblyProcedureFunc) {
	assemblyProcedureRegistry[name] = fn
}

// ProfileComposite holds one rule parsed from a resource's "composites" array.
type ProfileComposite struct {
	FHIRResource    string                 // e.g. "Appointment"
	FHIRPath        string                 // e.g. "participant"
	CompositeType   string                 // "condition" | "forEach" | "firstOf" | "procedure"
	ConditionField  string                 // for condition composites: HL7 field that must be non-empty
	ForEachSeg      string                 // for forEach composites: segment name to iterate (e.g. "AIP")
	FirstOfFields   []string               // for firstOf composites: ordered list of HL7 field paths
	ValueTemplate   map[string]interface{} // value object with {{FIELD}} placeholders
	TemplateArray   []interface{}          // for firstOf: array template
	ProcedureName   string                 // for procedure composites: registered name
	ProcedureParams map[string]interface{} // for procedure composites: JSON params passed to the func
}

// getProfileComposites loads composite rules + valueSets for a message type from
// the v2.0 profile stored in hl7_fhir_templates.
// Returns ([]ProfileComposite, valueSets map, error implicitly logged).
func (s *HL7FHIRTransformServiceV3) getProfileComposites(
	ctx context.Context,
	messageType string,
) ([]ProfileComposite, map[string]map[string]interface{}) {

	query := `
		SELECT template_config
		FROM   hl7_fhir_templates
		WHERE  message_type = $1
		  AND  is_default   = true
		  AND  profile_version = '2.0'
		LIMIT 1`

	var configJSON string
	if err := s.db.QueryRowContext(ctx, query, messageType).Scan(&configJSON); err != nil {
		if err != sql.ErrNoRows {
			log.Printf("⚠️ ProfileComposites: DB error for %s: %v", messageType, err)
		}
		return nil, nil
	}

	var profile map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &profile); err != nil {
		log.Printf("⚠️ ProfileComposites: invalid JSON for %s: %v", messageType, err)
		return nil, nil
	}

	// ── Extract valueSets ──────────────────────────────────────────────────
	valueSets := map[string]map[string]interface{}{}
	if vsRaw, ok := profile["valueSets"]; ok {
		if vsMap, ok := vsRaw.(map[string]interface{}); ok {
			for k, v := range vsMap {
				if entries, ok := v.(map[string]interface{}); ok {
					valueSets[k] = entries
				}
			}
		}
	}

	// ── Extract composites from each resource definition ──────────────────
	var composites []ProfileComposite

	mappingsRaw, _ := profile["mappings"].(map[string]interface{})
	for resourceKey, resDef := range mappingsRaw {
		resMap, ok := resDef.(map[string]interface{})
		if !ok {
			continue
		}
		resourceType := resourceKey // e.g. "Appointment"
		if rt, _ := resMap["resourceType"].(string); rt != "" {
			resourceType = rt
		}

		compRaw, ok := resMap["composites"].([]interface{})
		if !ok {
			continue
		}

		for _, ci := range compRaw {
			c, ok := ci.(map[string]interface{})
			if !ok {
				continue
			}

			pc := ProfileComposite{
				FHIRResource:  resourceType,
				FHIRPath:      getString(c, "fhir"),
				ValueTemplate: getMap(c, "value"),
			}

			switch {
			case c["condition"] != nil:
				pc.CompositeType = "condition"
				pc.ConditionField = getString(c, "condition")
			case c["forEach"] != nil:
				pc.CompositeType = "forEach"
				pc.ForEachSeg = getString(c, "forEach")
			case c["firstOf"] != nil:
				pc.CompositeType = "firstOf"
				if arr, ok := c["firstOf"].([]interface{}); ok {
					for _, f := range arr {
						if s, ok := f.(string); ok {
							pc.FirstOfFields = append(pc.FirstOfFields, s)
						}
					}
				}
				if tmpl, ok := c["template"].([]interface{}); ok {
					pc.TemplateArray = tmpl
				}
			case getString(c, "procedure") != "":
				pc.CompositeType = "procedure"
				pc.ProcedureName = getString(c, "procedure")
				if p, ok := c["params"].(map[string]interface{}); ok {
					pc.ProcedureParams = p
				}
			}

			composites = append(composites, pc)
		}
	}

	log.Printf("✅ ProfileComposites: loaded %d composites for %s", len(composites), messageType)
	return composites, valueSets
}

// applyProfileComposites executes composite rules against the parsed HL7 data and
// appends results to the already-field-mapped FHIR resources slice.
// Each element in the slice is a FHIR resource map containing at minimum a "resourceType" key.
func (s *HL7FHIRTransformServiceV3) applyProfileComposites(
	resources []map[string]interface{},
	parsedHL7 map[string]interface{},
	composites []ProfileComposite,
	valueSets map[string]map[string]interface{},
) []map[string]interface{} {

	for _, pc := range composites {
		// Find or create the target FHIR resource in the slice
		res, resources2 := s.getOrCreateProfileResource(resources, pc.FHIRResource)
		resources = resources2
		if res == nil {
			continue
		}

		switch pc.CompositeType {
		case "condition":
			// Emit exactly one entry when the condition field is non-empty
			condVal := s.resolveHL7FieldFromParsed(parsedHL7, pc.ConditionField)
			if condVal == "" {
				continue
			}
			entry := resolveTemplateMap(pc.ValueTemplate, nil, parsedHL7, condVal)
			if !isEmptyResolved(entry) {
				appendToFHIRArray(res, pc.FHIRPath, entry)
			}

		case "forEach":
			// Emit one entry per occurrence of the named segment
			segs := hl7assembly.ExtractSegmentGroup(parsedHL7, pc.ForEachSeg)
			for _, seg := range segs {
				segMap := segmentToMap(seg)
				entry := resolveTemplateMap(pc.ValueTemplate, segMap, parsedHL7, "")
				if !isEmptyResolved(entry) {
					appendToFHIRArray(res, pc.FHIRPath, entry)
				}
			}

		case "firstOf":
			// Use the first non-empty HL7 field value
			var chosen string
			for _, field := range pc.FirstOfFields {
				v := s.resolveHL7FieldFromParsed(parsedHL7, field)
				if v != "" {
					chosen = v
					break
				}
			}
			if chosen == "" {
				continue
			}
			if len(pc.TemplateArray) > 0 {
				// Expand array template with the chosen value
				for _, ti := range pc.TemplateArray {
					if tmpl, ok := ti.(map[string]interface{}); ok {
						entry := resolveTemplateMap(tmpl, nil, parsedHL7, chosen)
						if !isEmptyResolved(entry) {
							appendToFHIRArray(res, pc.FHIRPath, entry)
						}
					}
				}
			} else {
				appendToFHIRArray(res, pc.FHIRPath, chosen)
			}

		case "procedure":
			// Dispatch to a named, registered Go assembly procedure.
			// The procedure receives the full resource slice and may replace/add/remove
			// resources (not just append to a single FHIR array).
			fn, ok := assemblyProcedureRegistry[pc.ProcedureName]
			if !ok {
				log.Printf("⚠️ ProfileComposites: unknown procedure %q — register it in procedure_registry.go", pc.ProcedureName)
				continue
			}
			var procWarnings []string
			resources, procWarnings = fn(parsedHL7, resources, pc.ProcedureParams)
			if len(procWarnings) > 0 {
				log.Printf("⚠️ Procedure %s warnings: %v", pc.ProcedureName, procWarnings)
			}
			// After a procedure call the resource slice may have changed entirely;
			// skip the res pointer update and re-enter the loop with the new slice.
			continue
		}
	}

	return resources
}

// getOrCreateProfileResource finds a FHIR resource in the slice by resourceType,
// or creates a new skeleton resource and appends it to the slice.
// Returns the resource pointer and the (possibly extended) slice.
func (s *HL7FHIRTransformServiceV3) getOrCreateProfileResource(
	resources []map[string]interface{},
	resourceType string,
) (map[string]interface{}, []map[string]interface{}) {
	for _, r := range resources {
		if rt, _ := r["resourceType"].(string); strings.EqualFold(rt, resourceType) {
			return r, resources
		}
	}
	// Not found — create a skeleton and append
	r := map[string]interface{}{"resourceType": resourceType}
	resources = append(resources, r)
	return r, resources
}

// resolveHL7FieldFromParsed resolves an HL7 path like "PID.3" or "SCH.25"
// against the parsed HL7 data map (supporting both segmentGroups and enhancedSegments).
func (s *HL7FHIRTransformServiceV3) resolveHL7FieldFromParsed(
	parsedHL7 map[string]interface{},
	fieldPath string,
) string {
	if fieldPath == "" {
		return ""
	}
	fieldPath = strings.TrimSpace(fieldPath)

	// Split into at most 3 parts: segment, field, component
	parts := strings.SplitN(fieldPath, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	segName := parts[0]
	fieldNum := parts[1]
	componentNum := ""
	if len(parts) == 3 {
		componentNum = parts[2]
	}

	segs := hl7assembly.ExtractSegmentGroup(parsedHL7, segName)
	if len(segs) == 0 {
		return ""
	}
	seg := segs[0]

	wantFieldKey := segName + "." + fieldNum
	for _, f := range seg.Fields {
		if f.Key == wantFieldKey {
			if componentNum == "" {
				return strings.TrimSpace(f.Value)
			}
			// 3-part path: find sub-component by key first
			wantSubKey := wantFieldKey + "." + componentNum
			for _, sf := range f.Subfields {
				if sf.Key == wantSubKey {
					return strings.TrimSpace(sf.Value)
				}
			}
			// Fallback: split raw value by "^" and extract by position
			compIdx := 0
			fmt.Sscanf(componentNum, "%d", &compIdx)
			if compIdx > 0 {
				comps := strings.Split(f.Value, "^")
				if compIdx <= len(comps) {
					return strings.TrimSpace(comps[compIdx-1])
				}
			}
			return ""
		}
	}
	// Field index fallback (1-based)
	fieldIdx := 0
	fmt.Sscanf(fieldNum, "%d", &fieldIdx)
	if fieldIdx > 0 && fieldIdx <= len(seg.Fields) {
		f := seg.Fields[fieldIdx-1]
		if componentNum == "" {
			return strings.TrimSpace(f.Value)
		}
		compIdx := 0
		fmt.Sscanf(componentNum, "%d", &compIdx)
		if compIdx > 0 {
			comps := strings.Split(f.Value, "^")
			if compIdx <= len(comps) {
				return strings.TrimSpace(comps[compIdx-1])
			}
		}
	}
	return ""
}

// resolveTemplateMap deep-copies a template map replacing {{FIELD}} placeholders.
// segFields: segment fields keyed by "SEG.N" (used for forEach composites).
// parsedHL7: full parsed data (for cross-segment placeholders like {{PID.3}}).
// condVal:   value passed for condition composites (replaces {{_value}}).
func resolveTemplateMap(
	tmpl map[string]interface{},
	segFields map[string]string,
	parsedHL7 map[string]interface{},
	condVal string,
) map[string]interface{} {
	result := make(map[string]interface{}, len(tmpl))
	for k, v := range tmpl {
		result[k] = resolveTemplateValue(v, segFields, parsedHL7, condVal)
	}
	return result
}

func resolveTemplateValue(
	v interface{},
	segFields map[string]string,
	parsedHL7 map[string]interface{},
	condVal string,
) interface{} {
	switch tv := v.(type) {
	case string:
		return resolvePlaceholders(tv, segFields, parsedHL7, condVal)
	case map[string]interface{}:
		return resolveTemplateMap(tv, segFields, parsedHL7, condVal)
	case []interface{}:
		result := make([]interface{}, len(tv))
		for i, item := range tv {
			result[i] = resolveTemplateValue(item, segFields, parsedHL7, condVal)
		}
		return result
	default:
		return v
	}
}

var rePlaceholder = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// resolvePlaceholders replaces {{FIELD}} tokens in a string template.
// Lookup order: condVal (for {{_value}}), segFields (e.g. {{AIP.3}}), parsedHL7.
func resolvePlaceholders(
	s string,
	segFields map[string]string,
	parsedHL7 map[string]interface{},
	condVal string,
) string {
	return rePlaceholder.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[2 : len(match)-2] // strip {{ }}
		if inner == "_value" {
			return condVal
		}
		// Check segment field map first (forEach context)
		if segFields != nil {
			if val, ok := segFields[inner]; ok {
				return val
			}
		}
		// Fall back to full parsed HL7 data (cross-segment).
		// Supports 2-part (SEG.F) and 3-part (SEG.F.C) paths.
		if parsedHL7 != nil {
			parts := strings.SplitN(inner, ".", 3)
			if len(parts) >= 2 {
				segs := hl7assembly.ExtractSegmentGroup(parsedHL7, parts[0])
				if len(segs) > 0 {
					fieldKey := parts[0] + "." + parts[1]
					for _, f := range segs[0].Fields {
						if f.Key == fieldKey {
							if len(parts) == 2 {
								return strings.TrimSpace(f.Value)
							}
							// 3-part: resolve component
							subKey := fieldKey + "." + parts[2]
							for _, sf := range f.Subfields {
								if sf.Key == subKey {
									return strings.TrimSpace(sf.Value)
								}
							}
							// Fall back to splitting raw value by ^
							compIdx := 0
							fmt.Sscanf(parts[2], "%d", &compIdx)
							if compIdx > 0 {
								comps := strings.Split(f.Value, "^")
								if compIdx <= len(comps) {
									return strings.TrimSpace(comps[compIdx-1])
								}
							}
							return ""
						}
					}
				}
			}
		}
		return "" // placeholder not resolved → empty string
	})
}

// segmentToMap converts an hl7.EnhancedSegment into a flat map[segKey]value
// suitable for placeholder resolution (e.g. "AIP.3" → "12345").
func segmentToMap(seg interface{}) map[string]string {
	// Accept either typed or interface{}
	b, err := json.Marshal(seg)
	if err != nil {
		return nil
	}
	var enhSeg struct {
		Key    string `json:"key"`
		Fields []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(b, &enhSeg); err != nil {
		return nil
	}
	m := make(map[string]string, len(enhSeg.Fields))
	for _, f := range enhSeg.Fields {
		m[f.Key] = f.Value
	}
	return m
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

// augmentMessageHeaderFocus ensures MessageHeader.focus references every resource
// whose type is listed in focusTypes.  It is called after segment processors run
// so that processor-added resources (e.g. the prior Patient from MRGProcessor)
// are included.  Existing references are not duplicated.
// References use "ResourceType/id" format; rewriteReferences() converts them to
// urn:uuid: during bundle assembly.
func augmentMessageHeaderFocus(resources []map[string]interface{}, focusTypes []string) {
	if len(focusTypes) == 0 {
		return
	}

	// Build a fast lookup for which types belong in focus.
	wantedTypes := make(map[string]bool, len(focusTypes))
	for _, ft := range focusTypes {
		wantedTypes[ft] = true
	}

	// Find the MessageHeader.
	var mh map[string]interface{}
	for _, r := range resources {
		if rt, _ := r["resourceType"].(string); rt == "MessageHeader" {
			mh = r
			break
		}
	}
	if mh == nil {
		return
	}

	// Build a set of references already present so we don't duplicate.
	existing := make(map[string]bool)
	var focus []interface{}
	if f, ok := mh["focus"].([]interface{}); ok {
		focus = f
		for _, entry := range f {
			if fm, ok := entry.(map[string]interface{}); ok {
				if ref, _ := fm["reference"].(string); ref != "" {
					existing[ref] = true
				}
			}
		}
	}

	// Append a focus reference for every resource of a wanted type not yet present.
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		if !wantedTypes[rt] {
			continue
		}
		id, _ := r["id"].(string)
		if id == "" {
			continue
		}
		ref := rt + "/" + id
		if !existing[ref] {
			focus = append(focus, map[string]interface{}{"reference": ref})
			existing[ref] = true
		}
	}

	mh["focus"] = focus
}

// filterResolvedRequiredFieldErrors removes "FHIR Validation: Required field X.Y missing"
// errors from allErrors when field Y is now present in the corresponding resource.
// This eliminates false positives that arise because validateResourceAgainstSchema runs
// before profile composites and post-processing inject participant, status, etc.
func filterResolvedRequiredFieldErrors(allErrors []string, resources []map[string]interface{}) []string {
	// Build a quick lookup: resourceType → set of top-level keys present
	presentFields := make(map[string]map[string]bool, len(resources))
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		if rt == "" {
			continue
		}
		if presentFields[rt] == nil {
			presentFields[rt] = make(map[string]bool)
		}
		for k := range r {
			presentFields[rt][k] = true
		}
	}

	var kept []string
	for _, e := range allErrors {
		const prefix = "FHIR Validation: Required field "
		const suffix = " missing from resource"
		if strings.HasPrefix(e, prefix) && strings.HasSuffix(e, suffix) {
			// Extract "ResourceType.fieldName"
			inner := e[len(prefix) : len(e)-len(suffix)]
			if dotIdx := strings.Index(inner, "."); dotIdx != -1 {
				rt := inner[:dotIdx]
				field := inner[dotIdx+1:]
				if fields, ok := presentFields[rt]; ok && fields[field] {
					continue // field is now present — drop the error
				}
			}
		}
		kept = append(kept, e)
	}
	return kept
}

// filterResolvedRequiredFieldWarnings is the warnings counterpart of filterResolvedRequiredFieldErrors.
func filterResolvedRequiredFieldWarnings(allWarnings []string, resources []map[string]interface{}) []string {
	presentFields := make(map[string]map[string]bool, len(resources))
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		if rt == "" {
			continue
		}
		if presentFields[rt] == nil {
			presentFields[rt] = make(map[string]bool)
		}
		for k := range r {
			presentFields[rt][k] = true
		}
	}

	var kept []string
	for _, w := range allWarnings {
		const prefix = "Required field "
		const suffix = " missing from FHIR resource"
		if strings.HasPrefix(w, prefix) && strings.HasSuffix(w, suffix) {
			inner := w[len(prefix) : len(w)-len(suffix)]
			if dotIdx := strings.Index(inner, "."); dotIdx != -1 {
				rt := inner[:dotIdx]
				field := inner[dotIdx+1:]
				if fields, ok := presentFields[rt]; ok && fields[field] {
					continue
				}
			}
		}
		kept = append(kept, w)
	}
	return kept
}

// getString is a nil-safe helper to extract a string from a map.
func getString(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

// getMap is a nil-safe helper to extract a map from a map.
func getMap(m map[string]interface{}, key string) map[string]interface{} {
	v, _ := m[key].(map[string]interface{})
	return v
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

// ── Delta / Override Model ────────────────────────────────────────────────────
// An interface can store a *sparse* set of overrides on top of its OOB template
// rather than a full copy of every mapping.  This keeps the DB lean and makes it
// trivial to pick up OOB improvements: just rerun the merge at query time.
//
// Delta JSON stored in interface_message_mappings.mapping_overrides:
//
//	{
//	  "version": 1,
//	  "base_template_id": "<uuid>",
//	  "overrides": [
//	    {"action":"replace","hl7Path":"PID.5.1","fhirPath":"Patient.name[0].family","transform":"XPN_family","isRequired":true,"confidence":0.95},
//	    {"action":"add",    "hl7Path":"PID.29",  "fhirPath":"Patient.deceasedDateTime","transform":"TS_datetime"},
//	    {"action":"remove", "hl7Path":"PID.10"}
//	  ]
//	}

// MappingOverride is a single delta entry stored in interface_message_mappings.mapping_overrides.
type MappingOverride struct {
	Action     string  `json:"action"`               // "replace" | "add" | "remove"
	HL7Path    string  `json:"hl7Path"`              // "PID.5.1" — segment.field[.component]
	FHIRPath   string  `json:"fhirPath,omitempty"`   // full FHIR target path incl. resource type
	Transform  string  `json:"transform,omitempty"`  // transform key
	IsRequired bool    `json:"isRequired,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
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
			if _, exists := indexed[ov.HL7Path]; !exists {
				parts := strings.Split(ov.HL7Path, ".")
				if len(parts) < 2 {
					continue
				}
				seg, field := parts[0], parts[1]
				comp := ""
				if len(parts) > 2 {
					comp = parts[2]
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
					IsRequired:        ov.IsRequired,
					Confidence:        ov.Confidence,
				}
				indexed[ov.HL7Path] = len(result)
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
				Action:     "add",
				HL7Path:    key,
				FHIRPath:   am.FHIRResourceType + "." + am.TargetPath,
				Transform:  am.TransformType,
				IsRequired: am.IsRequired,
				Confidence: am.Confidence,
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
