// services/connectors/fhir_utils.go
// Shared FHIR R4 utilities used by all FHIR connector implementations.
//
// Design: single-source-of-truth for FHIR protocol logic so that
// http_fhir_inbound.go, http_fhir_outbound.go, and any future FHIR
// connector all share identical behaviour without code duplication.
//
// Provides:
//   FHIRResourceTypes      — canonical R4 resource type list (UI + validation)
//   ValidateFHIRType       — O(1) check against the canonical list
//   ParseFHIRResource      — extract resourceType / id / bundleType from JSON
//   BuildFHIRURL           — FHIR REST URL + HTTP verb per §3.1 RESTful API rules
//   ExtractOpOutcomeDiag   — pull diagnostics text from an OperationOutcome body
//   ToHTTPAuthConfig       — map FHIR authType → services/http.AuthConfig
//   BuildCapabilityStatement — FHIR R4 CapabilityStatement for this server instance
//   TruncateFHIR           — safe string truncation for log messages

package connectors

import (
	"encoding/json"
	"strings"
	"time"

	httpservice "ezhealthkonnect/services/http"
)

// ─────────────────────────────────────────────────────────────────────────────
// Canonical FHIR R4 resource type list
// ─────────────────────────────────────────────────────────────────────────────

// FHIRResourceTypes is the canonical list of FHIR R4 resource types.
// Used by the UI dropdown in ConnectorConfigBuilder and by ValidateFHIRType.
// Extensions and custom profiles are always allowed beyond this list.
var FHIRResourceTypes = []string{
	// Administrative
	"Patient", "Practitioner", "PractitionerRole", "Organization", "Location",
	"HealthcareService", "RelatedPerson", "Person", "Group",
	// Clinical
	"Encounter", "EpisodeOfCare", "Condition", "Observation", "DiagnosticReport",
	"Procedure", "FamilyMemberHistory", "ClinicalImpression", "DetectedIssue",
	// Medications
	"Medication", "MedicationRequest", "MedicationAdministration",
	"MedicationDispense", "MedicationStatement",
	// Allergies & Immunizations
	"AllergyIntolerance", "Immunization", "ImmunizationRecommendation",
	// Care planning
	"CarePlan", "CareTeam", "Goal", "ServiceRequest",
	// Documents & communications
	"DocumentReference", "Composition", "Communication", "Flag", "Task",
	// Scheduling
	"Appointment", "AppointmentResponse", "Schedule", "Slot",
	// Diagnostics & imaging
	"ImagingStudy", "Media", "BodyStructure", "Specimen",
	// Financial
	"Claim", "ClaimResponse", "ExplanationOfBenefit", "Coverage",
	"CoverageEligibilityRequest", "CoverageEligibilityResponse",
	// Infrastructure
	"Bundle", "List", "MessageHeader", "Parameters",
}

// fhirResourceTypeSet enables O(1) ValidateFHIRType lookups.
var fhirResourceTypeSet map[string]struct{}

func init() {
	fhirResourceTypeSet = make(map[string]struct{}, len(FHIRResourceTypes))
	for _, rt := range FHIRResourceTypes {
		fhirResourceTypeSet[rt] = struct{}{}
	}
}

// ValidateFHIRType returns true when rt is a known FHIR R4 resource type.
// Unknown types are NOT rejected — this is an informational check only.
func ValidateFHIRType(rt string) bool {
	_, ok := fhirResourceTypeSet[rt]
	return ok
}

// ─────────────────────────────────────────────────────────────────────────────
// Resource identity
// ─────────────────────────────────────────────────────────────────────────────

// FHIRResourceInfo holds the parsed identity of a FHIR resource.
type FHIRResourceInfo struct {
	ResourceType string // e.g. "Patient", "Bundle"
	ID           string // resource .id — may be empty
	BundleType   string // Bundle.type — e.g. "transaction", "batch" (Bundle only)
}

// ParseFHIRResource parses a JSON body and returns basic FHIR resource identity.
// Returns nil when the body is not valid JSON or has no resourceType field.
func ParseFHIRResource(body string) *FHIRResourceInfo {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return nil
	}
	rt, _ := m["resourceType"].(string)
	if rt == "" {
		return nil
	}
	id, _ := m["id"].(string)
	bt := ""
	if rt == "Bundle" {
		bt, _ = m["type"].(string)
	}
	return &FHIRResourceInfo{ResourceType: rt, ID: id, BundleType: bt}
}

// ─────────────────────────────────────────────────────────────────────────────
// URL routing  (FHIR RESTful API §3.1)
// ─────────────────────────────────────────────────────────────────────────────

// BuildFHIRURL computes the target URL and HTTP method for a FHIR REST delivery.
//
// Implements FHIR RESTful API §3.1 (CRUD) and Operations §3.3.
//
// Parameters:
//   baseURL      — FHIR server base, e.g. "https://fhir.hospital.com/r4"
//   resourceType — FHIR resource type ("Patient", "Bundle", …) or "" for auto-detect
//   resourceID   — Resource logical id for update/instance-op, or ""
//   bundleType   — Bundle.type: "transaction" | "batch" | "collection" | "document"
//   operation    — Optional FHIR operation name with or without "$" prefix,
//                  e.g. "$validate", "everything", "$process-message"
//   queryParams  — Raw query string appended to the URL (no leading "?"),
//                  e.g. "_format=json&_summary=count&family=Smith"
//   configMethod — Explicit HTTP verb override; "" means auto-select per FHIR spec.
//
// Composition rules (in priority order):
//   no resourceType, no operation   → POST baseURL           (body RT resolved at runtime)
//   no resourceType + operation     → POST baseURL/$op       (server-level operation)
//   Bundle / transaction            → POST baseURL/$transaction
//   Bundle / batch                  → POST baseURL/$batch
//   Bundle / other                  → POST baseURL
//   resourceType only               → POST baseURL/RT
//   resourceType + operation        → POST baseURL/RT/$op    (type-level operation)
//   resourceType + id               → PUT  baseURL/RT/id     (update)
//   resourceType + id + operation   → POST baseURL/RT/id/$op (instance-level operation)
//   + queryParams                   → append ?queryParams to any URL above
func BuildFHIRURL(baseURL, resourceType, resourceID, bundleType, operation, queryParams, configMethod string) (url, method string) {
	baseURL = strings.TrimRight(baseURL, "/")
	method = "POST"

	// Normalise operation — ensure "$" prefix, strip surrounding whitespace
	op := strings.TrimSpace(operation)
	if op != "" && !strings.HasPrefix(op, "$") {
		op = "$" + op
	}

	switch {
	case resourceType == "" && op == "":
		// No config override — resourceType resolved from message body at runtime
		url = baseURL

	case resourceType == "" && op != "":
		// Server-level operation: POST baseURL/$op
		url = baseURL + "/" + op

	case resourceType == "Bundle":
		// Bundle routing; explicit operations (rare) are handled after the switch
		switch bundleType {
		case "transaction":
			url = baseURL + "/$transaction"
		case "batch":
			url = baseURL + "/$batch"
		default:
			url = baseURL
		}

	default:
		// Single resource
		url = baseURL + "/" + resourceType
		if resourceID != "" {
			url = url + "/" + resourceID
			if op == "" {
				method = "PUT" // FHIR §3.1.0.2: update requires PUT
			}
		}
		if op != "" {
			// Type-level or instance-level operation
			url = url + "/" + op
			// Operations default to POST; caller may override via configMethod
		}
	}

	// Append query string (search params, _format, _summary, etc.)
	qs := strings.TrimSpace(queryParams)
	if qs != "" {
		if strings.HasPrefix(qs, "?") {
			url = url + qs
		} else {
			url = url + "?" + qs
		}
	}

	// configMethod always wins — supports GET (read-only ops), PATCH (partial update), etc.
	if configMethod != "" {
		method = strings.ToUpper(configMethod)
	}

	return url, method
}

// ─────────────────────────────────────────────────────────────────────────────
// Error handling
// ─────────────────────────────────────────────────────────────────────────────

// ExtractOpOutcomeDiag extracts the first diagnostics string from a FHIR
// OperationOutcome JSON body.  Returns "" when the body is not an OperationOutcome.
func ExtractOpOutcomeDiag(body string) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return ""
	}
	if rt, _ := m["resourceType"].(string); rt != "OperationOutcome" {
		return ""
	}
	issues, _ := m["issue"].([]interface{})
	for _, raw := range issues {
		issue, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if diag, ok := issue["diagnostics"].(string); ok && diag != "" {
			return diag
		}
		// Fallback to details.text
		if details, ok := issue["details"].(map[string]interface{}); ok {
			if text, ok := details["text"].(string); ok && text != "" {
				return text
			}
		}
	}
	return ""
}

// TruncateFHIR safely shortens a string for logging, appending "… (truncated)".
func TruncateFHIR(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "… (truncated)"
}

// ─────────────────────────────────────────────────────────────────────────────
// Auth bridging — FHIR schema → services/http.AuthConfig
// ─────────────────────────────────────────────────────────────────────────────

// ToHTTPAuthConfig converts FHIR connector auth fields into an
// *httpservice.AuthConfig ready for services/http.HTTPClientService.
//
// FHIR connector authType values: none | basic | bearer | smart | api_key
// The "smart" type maps to bearer — SMART on FHIR uses bearer tokens;
// full SMART token acquisition is handled externally.
func ToHTTPAuthConfig(authType, username, password, bearerToken, apiKey, apiKeyHeader string) *httpservice.AuthConfig {
	switch authType {
	case "basic":
		return &httpservice.AuthConfig{
			Type:     httpservice.AuthTypeBasic,
			Username: username,
			Password: password,
		}
	case "bearer", "smart":
		return &httpservice.AuthConfig{
			Type:        httpservice.AuthTypeBearer,
			BearerToken: bearerToken,
		}
	case "api_key":
		hdr := apiKeyHeader
		if hdr == "" {
			hdr = "X-API-Key"
		}
		return &httpservice.AuthConfig{
			Type:         httpservice.AuthTypeAPIKey,
			APIKey:       apiKey,
			APIKeyHeader: hdr,
		}
	default: // "none" or anything unrecognised
		return &httpservice.AuthConfig{Type: httpservice.AuthTypeNone}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// CapabilityStatement
// ─────────────────────────────────────────────────────────────────────────────

// BuildCapabilityStatement returns a minimal FHIR R4 CapabilityStatement
// for the ezHealthKonnect inbound FHIR receiver instance.
// Returned value is JSON-serialisable.
func BuildCapabilityStatement(basePath, fhirVersion string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "CapabilityStatement",
		"status":       "active",
		"date":         time.Now().UTC().Format(time.RFC3339),
		"kind":         "instance",
		"fhirVersion":  "4.0.1",
		"format":       []string{"application/fhir+json"},
		"software": map[string]interface{}{
			"name":    "ezHealthKonnect",
			"version": "2.0.0",
		},
		"implementation": map[string]interface{}{
			"description": "ezHealthKonnect FHIR " + fhirVersion + " Receiver",
			"url":         basePath,
		},
		"rest": []map[string]interface{}{
			{
				"mode": "server",
				"security": map[string]interface{}{
					"service": []map[string]interface{}{
						{
							"coding": []map[string]interface{}{
								{"system": "http://terminology.hl7.org/CodeSystem/restful-security-service", "code": "OAuth"},
								{"system": "http://terminology.hl7.org/CodeSystem/restful-security-service", "code": "Basic"},
								{"system": "http://terminology.hl7.org/CodeSystem/restful-security-service", "code": "Certificates"},
							},
						},
					},
				},
				"interaction": []map[string]interface{}{
					{"code": "transaction"},
					{"code": "batch"},
				},
				"resource": buildCapabilityResources(),
			},
		},
	}
}

// buildCapabilityResources returns the rest[0].resource array for the
// CapabilityStatement — one entry per known FHIR R4 resource type.
func buildCapabilityResources() []map[string]interface{} {
	resources := make([]map[string]interface{}, 0, len(FHIRResourceTypes))
	for _, rt := range FHIRResourceTypes {
		resources = append(resources, map[string]interface{}{
			"type": rt,
			"interaction": []map[string]interface{}{
				{"code": "create"},
				{"code": "update"},
				{"code": "read"},
			},
		})
	}
	return resources
}
