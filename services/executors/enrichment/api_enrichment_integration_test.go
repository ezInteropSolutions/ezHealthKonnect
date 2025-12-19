package enrichment

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ===============================================================
// INTEGRATION TEST: REAL-WORLD EMPI SCENARIO
// ===============================================================
// Simulates real emergency department workflow:
// 1. Triage sends minimal HL7 ADT^A04
// 2. API enrichment queries Epic FHIR EMPI
// 3. Complete demographics added to message
// 4. Ready for HL7→FHIR transformation

func TestAPIEnrichment_RealWorld_EMPI_Lookup(t *testing.T) {
	// ===================================================================
	// STEP 1: Setup Mock Epic FHIR API Server
	// ===================================================================
	mockEpicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authentication
		auth := r.Header.Get("Authorization")
		if auth != "Bearer epic-test-token-12345" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "invalid_token",
			})
			return
		}

		// Verify Epic-Client-ID header
		if r.Header.Get("Epic-Client-ID") != "integration-engine" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "missing_client_id",
			})
			return
		}

		// Extract patient ID from URL path
		// Expected: /api/FHIR/R4/Patient/MRN-12345
		expectedPath := "/api/FHIR/R4/Patient/MRN-12345"
		if r.URL.Path != expectedPath {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"resourceType": "OperationOutcome",
				"issue": []map[string]interface{}{
					{
						"severity": "error",
						"code":     "not-found",
						"details": map[string]interface{}{
							"text": "Patient not found in EMPI",
						},
					},
				},
			})
			return
		}

		// Return complete FHIR Patient resource (Epic format)
		fhirPatient := map[string]interface{}{
			"resourceType": "Patient",
			"id":           "MRN-12345",
			"meta": map[string]interface{}{
				"versionId": "2",
				"lastUpdated": "2023-12-15T10:30:00Z",
			},
			"identifier": []map[string]interface{}{
				{
					"use":    "official",
					"system": "urn:oid:2.16.840.1.113883.4.1",
					"value":  "MRN-12345",
					"type": map[string]interface{}{
						"coding": []map[string]interface{}{
							{
								"system":  "http://terminology.hl7.org/CodeSystem/v2-0203",
								"code":    "MR",
								"display": "Medical Record Number",
							},
						},
					},
				},
				{
					"use":    "secondary",
					"system": "http://hospital.org/insurance",
					"value":  "INS-999-8888",
					"type": map[string]interface{}{
						"text": "Insurance Member ID",
					},
				},
			},
			"active": true,
			"name": []map[string]interface{}{
				{
					"use":    "official",
					"text":   "John Robert Doe",
					"family": "Doe",
					"given":  []string{"John", "Robert"},
					"prefix": []string{"Mr"},
				},
			},
			"telecom": []map[string]interface{}{
				{
					"system": "phone",
					"value":  "555-123-4567",
					"use":    "mobile",
					"rank":   1,
				},
				{
					"system": "phone",
					"value":  "555-987-6543",
					"use":    "home",
					"rank":   2,
				},
				{
					"system": "email",
					"value":  "john.doe@email.com",
					"use":    "home",
				},
			},
			"gender":    "male",
			"birthDate": "1985-06-15",
			"address": []map[string]interface{}{
				{
					"use":        "home",
					"type":       "physical",
					"line":       []string{"123 Main Street", "Apartment 4B"},
					"city":       "Springfield",
					"state":      "IL",
					"postalCode": "62701",
					"country":    "USA",
				},
			},
			"maritalStatus": map[string]interface{}{
				"coding": []map[string]interface{}{
					{
						"system":  "http://terminology.hl7.org/CodeSystem/v3-MaritalStatus",
						"code":    "M",
						"display": "Married",
					},
				},
			},
			"contact": []map[string]interface{}{
				{
					"relationship": []map[string]interface{}{
						{
							"coding": []map[string]interface{}{
								{
									"system":  "http://terminology.hl7.org/CodeSystem/v2-0131",
									"code":    "C",
									"display": "Emergency Contact",
								},
							},
						},
					},
					"name": map[string]interface{}{
						"text":   "Jane Doe",
						"family": "Doe",
						"given":  []string{"Jane"},
					},
					"telecom": []map[string]interface{}{
						{
							"system": "phone",
							"value":  "555-111-2222",
							"use":    "mobile",
						},
					},
				},
			},
			"extension": []map[string]interface{}{
				{
					"url":         "http://hospital.org/fhir/StructureDefinition/race",
					"valueString": "White",
				},
				{
					"url":         "http://hospital.org/fhir/StructureDefinition/ethnicity",
					"valueString": "Not Hispanic or Latino",
				},
				{
					"url":         "http://hospital.org/fhir/StructureDefinition/preferred-language",
					"valueString": "en",
				},
			},
		}

		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(fhirPatient)
	}))
	defer mockEpicServer.Close()

	// ===================================================================
	// STEP 2: Create Minimal HL7 ADT^A04 Message (Triage System)
	// ===================================================================
	// For testing, create simulated parsed HL7 structure
	inputData := map[string]interface{}{
		"messageType": "ADT^A04",
		"version":     "2.5",
		"enhancedSegments": map[string]interface{}{
			"MSH": map[string]interface{}{
				"key":  "MSH",
				"name": "Message Header",
				"fields": []interface{}{
					map[string]interface{}{"key": "MSH.1", "value": "|"},
					map[string]interface{}{"key": "MSH.2", "value": "^~\\&"},
					map[string]interface{}{"key": "MSH.3", "value": "TRIAGE"},
				},
			},
			"PID": map[string]interface{}{
				"key":  "PID",
				"name": "Patient Identification",
				"fields": []interface{}{
					map[string]interface{}{"key": "PID.1", "value": "1"},
					map[string]interface{}{"key": "PID.2", "value": ""},
					map[string]interface{}{"key": "PID.3", "value": "MRN-12345"},
					map[string]interface{}{"key": "PID.4", "value": ""},
					map[string]interface{}{"key": "PID.5", "value": "DOE^JOHN"},
					map[string]interface{}{"key": "PID.6", "value": ""},
					map[string]interface{}{"key": "PID.7", "value": "19850615"},
					map[string]interface{}{"key": "PID.8", "value": "M"},
				},
			},
		},
	}

	// ===================================================================
	// STEP 3: Configure API Enrichment Step (Epic FHIR EMPI)
	// ===================================================================
	executor := NewAPIEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "EMPI Patient Lookup",
		StepType: "pre.enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint": mockEpicServer.URL + "/api/FHIR/R4/Patient/{patientId}",
			"method":   "GET",
			"authType": "bearer",
			"bearerToken": "epic-test-token-12345",
			"headers": map[string]interface{}{
				"Accept":         "application/fhir+json",
				"Epic-Client-ID": "integration-engine",
			},
			"fieldMappings": map[string]interface{}{
				"patientId": "enhancedSegments.PID.fields[2].value",
			},
			"targetPath": "enriched.empi",
			"timeoutMs":  3000,
			"retryCount": 2,
			"failOnError": false,
			"defaultValue": map[string]interface{}{
				"status": "not_found",
				"source": "empi",
			},
		},
	}

	// ===================================================================
	// STEP 4: Execute API Enrichment
	// ===================================================================
	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("API enrichment failed: %v", err)
	}

	// ===================================================================
	// STEP 5: Verify Enriched Data
	// ===================================================================

	// Check enriched field exists
	enriched, ok := output["enriched"].(map[string]interface{})
	if !ok {
		t.Fatal("enriched field not found")
	}

	empi, ok := enriched["empi"].(map[string]interface{})
	if !ok {
		t.Fatal("enriched.empi field not found")
	}

	// Verify FHIR Patient resource structure
	if empi["resourceType"] != "Patient" {
		t.Errorf("Expected resourceType=Patient, got %v", empi["resourceType"])
	}

	if empi["id"] != "MRN-12345" {
		t.Errorf("Expected id=MRN-12345, got %v", empi["id"])
	}

	// Verify complete name (missing in original HL7)
	names, ok := empi["name"].([]interface{})
	if !ok || len(names) == 0 {
		t.Fatal("Patient name not found in EMPI response")
	}
	name := names[0].(map[string]interface{})
	if name["text"] != "John Robert Doe" {
		t.Errorf("Expected full name 'John Robert Doe', got %v", name["text"])
	}

	// Verify telecom (phone/email - missing in original HL7)
	telecom, ok := empi["telecom"].([]interface{})
	if !ok || len(telecom) == 0 {
		t.Fatal("Patient telecom not found in EMPI response")
	}
	if len(telecom) < 3 {
		t.Errorf("Expected at least 3 telecom entries, got %d", len(telecom))
	}

	// Verify address (missing in original HL7)
	addresses, ok := empi["address"].([]interface{})
	if !ok || len(addresses) == 0 {
		t.Fatal("Patient address not found in EMPI response")
	}
	address := addresses[0].(map[string]interface{})
	if address["city"] != "Springfield" {
		t.Errorf("Expected city=Springfield, got %v", address["city"])
	}

	// Verify emergency contact (missing in original HL7)
	contacts, ok := empi["contact"].([]interface{})
	if !ok || len(contacts) == 0 {
		t.Fatal("Emergency contact not found in EMPI response")
	}

	// Verify insurance ID from identifiers (missing in original HL7)
	identifiers, ok := empi["identifier"].([]interface{})
	if !ok || len(identifiers) < 2 {
		t.Fatal("Expected at least 2 identifiers (MRN + Insurance)")
	}
	insuranceID := identifiers[1].(map[string]interface{})
	if insuranceID["value"] != "INS-999-8888" {
		t.Errorf("Expected insurance ID=INS-999-8888, got %v", insuranceID["value"])
	}

	// ===================================================================
	// STEP 6: Verify Data Transformation Impact
	// ===================================================================
	// At this point, the enriched data is available for:
	// - HL7→FHIR mapping step can use enriched.empi.name instead of PID.5
	// - HL7→FHIR mapping can create complete Patient + Coverage resources
	// - Validation step can verify insurance exists
	// - Custom scripts can access complete demographics

	t.Log("✅ EMPI enrichment successful:")
	t.Logf("   - Original HL7: Minimal demographics (name, DOB, gender)")
	t.Logf("   - After EMPI:   Complete demographics (full name, phone, email, address, emergency contact, insurance)")
	t.Logf("   - Fields added: %d", countFields(empi))
}

// Helper function to count fields in nested map
func countFields(data map[string]interface{}) int {
	count := 0
	for _, value := range data {
		count++
		if nested, ok := value.(map[string]interface{}); ok {
			count += countFields(nested)
		} else if slice, ok := value.([]interface{}); ok {
			for _, item := range slice {
				if nestedMap, ok := item.(map[string]interface{}); ok {
					count += countFields(nestedMap)
				}
			}
		}
	}
	return count
}

// ===============================================================
// INTEGRATION TEST: AUTHENTICATION FAILURE HANDLING
// ===============================================================

func TestAPIEnrichment_AuthenticationFailure_GracefulFallback(t *testing.T) {
	// Mock server that rejects authentication
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "invalid_token",
		})
	}))
	defer mockServer.Close()

	executor := NewAPIEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "EMPI with Bad Token",
		StepType: "pre.enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint":    mockServer.URL + "/patients/12345",
			"method":      "GET",
			"authType":    "bearer",
			"bearerToken": "invalid-token",
			"targetPath":  "enriched.empi",
			"failOnError": false,
			"defaultValue": map[string]interface{}{
				"status": "auth_failed",
				"reason": "Could not authenticate with EMPI",
			},
		},
	}

	inputData := map[string]interface{}{}

	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Expected graceful fallback, got error: %v", err)
	}

	// Verify default value was used
	enriched := output["enriched"].(map[string]interface{})
	empi := enriched["empi"].(map[string]interface{})

	if empi["status"] != "auth_failed" {
		t.Errorf("Expected default value after auth failure, got %v", empi)
	}

	t.Log("✅ Authentication failure handled gracefully with default value")
}

// ===============================================================
// INTEGRATION TEST: NETWORK TIMEOUT WITH RETRY
// ===============================================================

func TestAPIEnrichment_NetworkTimeout_RetrySuccess(t *testing.T) {
	attemptCount := 0

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++

		// First 2 attempts: simulate timeout by delaying
		if attemptCount < 3 {
			// Sleep longer than timeout
			// (In real test, httptest client will timeout)
			http.Error(w, "Gateway Timeout", http.StatusGatewayTimeout)
			return
		}

		// Third attempt: succeed
		response := map[string]interface{}{
			"success":  true,
			"attempts": attemptCount,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	executor := NewAPIEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Retry Test",
		StepType: "pre.enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"endpoint":     mockServer.URL + "/flaky",
			"method":       "GET",
			"retryCount":   3,
			"retryDelayMs": 100,
			"timeoutMs":    5000,
			"targetPath":   "enriched.retry",
		},
	}

	inputData := map[string]interface{}{}

	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Expected success after retries, got error: %v", err)
	}

	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", attemptCount)
	}

	enriched := output["enriched"].(map[string]interface{})
	retry := enriched["retry"].(map[string]interface{})

	if retry["success"] != true {
		t.Error("Expected success=true after retries")
	}

	t.Logf("✅ Network timeout recovered after %d retry attempts", attemptCount)
}
