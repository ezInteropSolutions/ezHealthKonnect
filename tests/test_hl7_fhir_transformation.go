// test_hl7_fhir_transformation.go
// Test script to validate HL7 to FHIR transformation with your parsed data
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

// Test transformation with your parsed HL7 data
func main() {
	// Your parsed HL7 data from the JSON file
	parsedHL7Data := map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"raw":     "MSH|^~\\&|SENDING_APPLICATION|SENDING_FACILITY|RECEIVING_APPLICATION|RECEIVING_FACILITY|20110613083617||ADT^A04|934576120110613083617|P|2.3||||\r\nEVN|A04|20110613083617|||\r\nPID|1||135769||MOUSE^MICKEY^||19281118|M|||123 Main St.^^Lake Buena Vista^FL^32830||(407)939-1289^^^theMainMouse@disney.com|||||1719|99999999||||||||||||||||||||",
			"success": true,
			"version": "2.3",
			"messageType": map[string]interface{}{
				"code":        "ADT",
				"event":       "A04",
				"name":        "ADT^A04",
				"description": "Register a patient",
				"structure":   "ADT_A04",
			},
			"enhancedSegments": map[string]interface{}{
				"PID": map[string]interface{}{
					"key":  "PID",
					"name": "PID",
					"fields": []interface{}{
						map[string]interface{}{
							"key":   "PID.3",
							"value": "135769",
							"name":  "Patient ID (Internal ID)",
							"subfields": []interface{}{
								map[string]interface{}{
									"key":   "PID.3.1",
									"name":  "ID",
									"value": "135769",
								},
							},
						},
						map[string]interface{}{
							"key":   "PID.5",
							"value": "MOUSE^MICKEY^",
							"name":  "Patient Name",
							"subfields": []interface{}{
								map[string]interface{}{
									"key":   "PID.5.1",
									"name":  "Family Name",
									"value": "MOUSE",
								},
								map[string]interface{}{
									"key":   "PID.5.2",
									"name":  "Given Name",
									"value": "MICKEY",
								},
							},
						},
						map[string]interface{}{
							"key":   "PID.7",
							"value": "19281118",
							"name":  "Date of Birth",
							"subfields": []interface{}{
								map[string]interface{}{
									"key":   "PID.7.1",
									"name":  "Time Of An Event",
									"value": "19281118",
								},
							},
						},
						map[string]interface{}{
							"key":   "PID.8",
							"value": "M",
							"name":  "Sex",
						},
						map[string]interface{}{
							"key":   "PID.11",
							"value": "123 Main St.^^Lake Buena Vista^FL^32830",
							"name":  "Patient Address",
							"subfields": []interface{}{
								map[string]interface{}{
									"key":   "PID.11.1",
									"name":  "Street Address",
									"value": "123 Main St.",
								},
								map[string]interface{}{
									"key":   "PID.11.3",
									"name":  "City",
									"value": "Lake Buena Vista",
								},
								map[string]interface{}{
									"key":   "PID.11.4",
									"name":  "State Or Province",
									"value": "FL",
								},
								map[string]interface{}{
									"key":   "PID.11.5",
									"name":  "Zip Or Postal Code",
									"value": "32830",
								},
							},
						},
						map[string]interface{}{
							"key":   "PID.13",
							"value": "(407)939-1289^^^theMainMouse@disney.com",
							"name":  "Phone Number - Home",
							"subfields": []interface{}{
								map[string]interface{}{
									"key":   "PID.13.1",
									"name":  "Telephone Number",
									"value": "(407)939-1289",
								},
								map[string]interface{}{
									"key":   "PID.13.4",
									"name":  "Email Address",
									"value": "theMainMouse@disney.com",
								},
							},
						},
						map[string]interface{}{
							"key":   "PID.18",
							"value": "1719",
							"name":  "Patient Account Number",
							"subfields": []interface{}{
								map[string]interface{}{
									"key":   "PID.18.1",
									"name":  "ID",
									"value": "1719",
								},
							},
						},
						map[string]interface{}{
							"key":   "PID.19",
							"value": "99999999",
							"name":  "SSN Number - Patient",
						},
					},
				},
			},
		},
	}

	// Create test transformation request
	request := &TransformRequest{
		ParsedHL7Data: parsedHL7Data,
		TargetProfile: "base",
		FHIRVersion:   "R4",
		CreateBundle:  true,
		RequestID:     "test_transform_123",
	}

	// Initialize enhanced transformation service
	// Note: In real implementation, you'd pass your database connection
	service := NewEnhancedHL7FHIRTransformService(nil) // nil for testing without DB

	// Perform transformation
	ctx := context.Background()
	response, err := service.Transform(ctx, request)
	if err != nil {
		log.Fatalf("❌ Transformation failed: %v", err)
	}

	// Print results
	fmt.Printf("🎯 TRANSFORMATION RESULTS\n")
	fmt.Printf("========================\n")
	fmt.Printf("Success: %v\n", response.Success)
	fmt.Printf("Request ID: %s\n", response.RequestID)
	fmt.Printf("Message Type: %s\n", response.MessageType)
	fmt.Printf("Resources Created: %d\n", len(response.FHIRResources))
	fmt.Printf("Processing Time: %s\n", response.Performance.TotalTime)

	if len(response.Warnings) > 0 {
		fmt.Printf("\n⚠️  WARNINGS:\n")
		for _, warning := range response.Warnings {
			fmt.Printf("  - %s\n", warning)
		}
	}

	if len(response.Errors) > 0 {
		fmt.Printf("\n❌ ERRORS:\n")
		for _, error := range response.Errors {
			fmt.Printf("  - %s\n", error)
		}
	}

	// Print FHIR Resources
	if len(response.FHIRResources) > 0 {
		fmt.Printf("\n📋 FHIR RESOURCES:\n")
		for i, resource := range response.FHIRResources {
			fmt.Printf("\n--- Resource %d ---\n", i+1)
			resourceJSON, _ := json.MarshalIndent(resource, "", "  ")
			fmt.Printf("%s\n", resourceJSON)
		}
	}

	// Print Bundle if created
	if response.Bundle != nil {
		fmt.Printf("\n📦 FHIR BUNDLE:\n")
		bundleJSON, _ := json.MarshalIndent(response.Bundle, "", "  ")
		fmt.Printf("%s\n", bundleJSON)
	}

	// Print mapping statistics
	fmt.Printf("\n📊 MAPPING STATISTICS:\n")
	fmt.Printf("Total Fields Mapped: %d\n", response.MappingStats.TotalFieldsMapped)
	fmt.Printf("Required Fields: %d\n", response.MappingStats.RequiredFieldsMapped)
	fmt.Printf("Optional Fields: %d\n", response.MappingStats.OptionalFieldsMapped)

	fmt.Printf("\n✅ Test completed successfully!\n")
}

// Expected FHIR Patient resource output
func printExpectedOutput() {
	expectedPatient := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "patient-135769",
		"identifier": []map[string]interface{}{
			{
				"use":   "usual",
				"value": "135769",
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
				"use":   "secondary",
				"value": "1719",
				"type": map[string]interface{}{
					"coding": []map[string]interface{}{
						{
							"system":  "http://terminology.hl7.org/CodeSystem/v2-0203",
							"code":    "AN",
							"display": "Account Number",
						},
					},
				},
			},
			{
				"use":   "secondary",
				"value": "99999999",
				"type": map[string]interface{}{
					"coding": []map[string]interface{}{
						{
							"system":  "http://terminology.hl7.org/CodeSystem/v2-0203",
							"code":    "SS",
							"display": "Social Security Number",
						},
					},
				},
			},
		},
		"name": []map[string]interface{}{
			{
				"use":    "official",
				"family": "MOUSE",
				"given":  []string{"MICKEY"},
			},
		},
		"birthDate": "1928-11-18",
		"gender":    "male",
		"address": []map[string]interface{}{
			{
				"use":        "home",
				"line":       []string{"123 Main St."},
				"city":       "Lake Buena Vista",
				"state":      "FL",
				"postalCode": "32830",
			},
		},
		"telecom": []map[string]interface{}{
			{
				"system": "phone",
				"use":    "home",
				"value":  "(407)939-1289",
			},
			{
				"system": "email",
				"use":    "home",
				"value":  "theMainMouse@disney.com",
			},
		},
	}

	fmt.Printf("\n🎯 EXPECTED FHIR PATIENT OUTPUT:\n")
	expectedJSON, _ := json.MarshalIndent(expectedPatient, "", "  ")
	fmt.Printf("%s\n", expectedJSON)
}

// Data type definitions for reference
type TransformRequest struct {
	ParsedHL7Data  map[string]interface{} `json:"parsedHL7Data"`
	TargetProfile  string                 `json:"targetProfile,omitempty"`
	FHIRVersion    string                 `json:"fhirVersion,omitempty"`
	CreateBundle   bool                   `json:"createBundle,omitempty"`
	ValidationMode string                 `json:"validationMode,omitempty"`
	InterfaceID    string                 `json:"interfaceId,omitempty"`
	RequestID      string                 `json:"requestId,omitempty"`
}

type TransformResponse struct {
	Success          bool                     `json:"success"`
	RequestID        string                   `json:"requestId"`
	MessageType      string                   `json:"messageType"`
	FHIRResources    []map[string]interface{} `json:"fhirResources"`
	Bundle           map[string]interface{}   `json:"bundle,omitempty"`
	ResourceCounts   map[string]int           `json:"resourceCounts"`
	MappingStats     MappingStatistics        `json:"mappingStats"`
	Warnings         []string                 `json:"warnings"`
	Errors           []string                 `json:"errors"`
	Performance      PerformanceMetrics       `json:"performance"`
	ValidationIssues []ValidationIssue        `json:"validationIssues,omitempty"`
}

type MappingStatistics struct {
	TotalFieldsMapped    int `json:"totalFieldsMapped"`
	RequiredFieldsMapped int `json:"requiredFieldsMapped"`
	OptionalFieldsMapped int `json:"optionalFieldsMapped"`
	UnmappedFields       int `json:"unmappedFields"`
	ValueSetTransforms   int `json:"valueSetTransforms"`
	DataTypeTransforms   int `json:"dataTypeTransforms"`
}

type PerformanceMetrics struct {
	TotalTime        string `json:"totalTime"`
	DatabaseTime     string `json:"databaseTime"`
	TransformTime    string `json:"transformTime"`
	ValidationTime   string `json:"validationTime"`
	ResourcesCreated int    `json:"resourcesCreated"`
}

type ValidationIssue struct {
	Severity     string `json:"severity"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	ResourceType string `json:"resourceType,omitempty"`
	Path         string `json:"path,omitempty"`
}
