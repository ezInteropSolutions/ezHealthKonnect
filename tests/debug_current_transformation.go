// debug_current_transformation.go
// Debug script to identify issues with current HL7 to FHIR transformation
package main

import (
	"encoding/json"
	"fmt"
)

func main() {
	fmt.Printf("🔍 DEBUGGING HL7 TO FHIR TRANSFORMATION\n")
	fmt.Printf("=====================================\n\n")

	// Your actual parsed HL7 data
	parsedHL7Data := map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"messageType": map[string]interface{}{
				"code":        "ADT",
				"event":       "A04",
				"name":        "ADT^A04",
				"description": "Register a patient",
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
						},
						map[string]interface{}{
							"key":   "PID.7",
							"value": "19281118",
							"name":  "Date of Birth",
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

	// Step 1: Debug data structure
	fmt.Printf("1️⃣  PARSED HL7 DATA STRUCTURE ANALYSIS\n")
	fmt.Printf("--------------------------------------\n")
	debugDataStructure(parsedHL7Data)

	// Step 2: Debug message type extraction
	fmt.Printf("\n2️⃣  MESSAGE TYPE EXTRACTION\n")
	fmt.Printf("---------------------------\n")
	messageType := debugMessageTypeExtraction(parsedHL7Data)

	// Step 3: Debug field extraction
	fmt.Printf("\n3️⃣  FIELD EXTRACTION TESTING\n")
	fmt.Printf("----------------------------\n")
	debugFieldExtraction(parsedHL7Data)

	// Step 4: Debug transformation logic
	fmt.Printf("\n4️⃣  TRANSFORMATION TESTING\n")
	fmt.Printf("--------------------------\n")
	debugTransformations()

	// Step 5: Show expected vs actual database queries
	fmt.Printf("\n5️⃣  DATABASE MAPPING ANALYSIS\n")
	fmt.Printf("-----------------------------\n")
	debugDatabaseMappings(messageType)

	// Step 6: Show what the output should be
	fmt.Printf("\n6️⃣  EXPECTED FHIR OUTPUT\n")
	fmt.Printf("------------------------\n")
	showExpectedOutput()

	// Step 7: Provide action items
	fmt.Printf("\n7️⃣  ACTION ITEMS TO FIX ISSUES\n")
	fmt.Printf("------------------------------\n")
	showActionItems()
}

func debugDataStructure(parsedHL7 map[string]interface{}) {
	// Check if basic structure exists
	if data, ok := parsedHL7["data"].(map[string]interface{}); ok {
		fmt.Printf("✅ 'data' object found\n")

		if msgType, ok := data["messageType"].(map[string]interface{}); ok {
			fmt.Printf("✅ 'messageType' object found\n")
			if name, ok := msgType["name"].(string); ok {
				fmt.Printf("✅ Message type: %s\n", name)
			} else {
				fmt.Printf("❌ 'messageType.name' not found or not string\n")
			}
		} else {
			fmt.Printf("❌ 'messageType' object not found\n")
		}

		if segments, ok := data["enhancedSegments"].(map[string]interface{}); ok {
			fmt.Printf("✅ 'enhancedSegments' object found\n")
			for segmentName := range segments {
				fmt.Printf("   - Found segment: %s\n", segmentName)
			}
		} else {
			fmt.Printf("❌ 'enhancedSegments' object not found\n")
		}
	} else {
		fmt.Printf("❌ 'data' object not found or not map\n")
	}
}

func debugMessageTypeExtraction(parsedHL7 map[string]interface{}) string {
	if data, ok := parsedHL7["data"].(map[string]interface{}); ok {
		if messageType, exists := data["messageType"].(map[string]interface{}); exists {
			if name, nameOk := messageType["name"].(string); nameOk {
				fmt.Printf("✅ Successfully extracted message type: %s\n", name)
				return name
			}
		}
	}
	fmt.Printf("❌ Failed to extract message type\n")
	return ""
}

func debugFieldExtraction(parsedHL7 map[string]interface{}) {
	if data, ok := parsedHL7["data"].(map[string]interface{}); ok {
		if enhancedSegments, ok := data["enhancedSegments"].(map[string]interface{}); ok {
			if pidSegment, pidOk := enhancedSegments["PID"].(map[string]interface{}); pidOk {
				if fields, fieldsOk := pidSegment["fields"].([]interface{}); fieldsOk {
					fmt.Printf("✅ PID segment has %d fields\n", len(fields))

					// Test extraction of key fields
					testFieldExtractions := []struct {
						field     string
						component string
						expected  string
					}{
						{"3", "", "135769"},        // Patient ID
						{"5", "", "MOUSE^MICKEY^"}, // Patient Name
						{"7", "", "19281118"},      // Birth Date
						{"8", "", "M"},             // Gender
						{"11", "", "123 Main St.^^Lake Buena Vista^FL^32830"}, // Address
						{"13", "1", "(407)939-1289"},                          // Phone
						{"13", "4", "theMainMouse@disney.com"},                // Email
						{"18", "", "1719"},                                    // Account Number
						{"19", "", "99999999"},                                // SSN
					}

					for _, test := range testFieldExtractions {
						value, found := extractHL7ValueDebug(enhancedSegments, "PID", test.field, test.component)
						if found && value == test.expected {
							fmt.Printf("✅ PID.%s", test.field)
							if test.component != "" {
								fmt.Printf(".%s", test.component)
							}
							fmt.Printf(" = '%s'\n", value)
						} else if found {
							fmt.Printf("⚠️  PID.%s", test.field)
							if test.component != "" {
								fmt.Printf(".%s", test.component)
							}
							fmt.Printf(" = '%s' (expected '%s')\n", value, test.expected)
						} else {
							fmt.Printf("❌ PID.%s", test.field)
							if test.component != "" {
								fmt.Printf(".%s", test.component)
							}
							fmt.Printf(" not found (expected '%s')\n", test.expected)
						}
					}
				}
			}
		}
	}
}

func extractHL7ValueDebug(segments map[string]interface{}, segmentName, fieldName, componentName string) (string, bool) {
	segment, segmentExists := segments[segmentName].(map[string]interface{})
	if !segmentExists {
		return "", false
	}

	fields, fieldsExists := segment["fields"].([]interface{})
	if !fieldsExists {
		return "", false
	}

	expectedKey := fmt.Sprintf("%s.%s", segmentName, fieldName)

	for _, field := range fields {
		fieldMap, ok := field.(map[string]interface{})
		if !ok {
			continue
		}

		key, keyOk := fieldMap["key"].(string)
		if !keyOk || key != expectedKey {
			continue
		}

		// If no component specified, return main value
		if componentName == "" {
			if value, valueOk := fieldMap["value"].(string); valueOk {
				return value, true
			}
		} else {
			// Look for component in subfields
			if subfields, subfieldOk := fieldMap["subfields"].([]interface{}); subfieldOk {
				for _, subfield := range subfields {
					subfieldMap, sfOk := subfield.(map[string]interface{})
					if !sfOk {
						continue
					}

					subfieldKey, sfKeyOk := subfieldMap["key"].(string)
					expectedSubKey := fmt.Sprintf("%s.%s.%s", segmentName, fieldName, componentName)

					if sfKeyOk && subfieldKey == expectedSubKey {
						if value, valueOk := subfieldMap["value"].(string); valueOk {
							return value, true
						}
					}
				}
			}
		}
	}

	return "", false
}

func debugTransformations() {
	testCases := []struct {
		name      string
		input     string
		transform string
		expected  string
	}{
		{"CX to Identifier", "135769^^^MR", "cx_to_identifier", "Patient ID with type"},
		{"XPN to HumanName", "MOUSE^MICKEY^", "xpn_to_humanname", "Family: MOUSE, Given: MICKEY"},
		{"TS to Date", "19281118", "ts_to_date", "1928-11-18"},
		{"Gender Mapping", "M", "gender_mapping", "male"},
		{"XAD to Address", "123 Main St.^^Lake Buena Vista^FL^32830", "xad_to_address", "Structured address"},
	}

	for _, test := range testCases {
		fmt.Printf("🧪 %s:\n", test.name)
		fmt.Printf("   Input: %s\n", test.input)
		fmt.Printf("   Transform: %s\n", test.transform)
		fmt.Printf("   Expected: %s\n", test.expected)

		// Here you would call your actual transformation function
		// result := transformValue(test.input, test.transform)
		// fmt.Printf("   Actual: %s\n", result)

		fmt.Printf("   Status: ⚠️  Test manually with your transformer\n\n")
	}
}

func debugDatabaseMappings(messageType string) {
	fmt.Printf("Required database tables and data:\n\n")

	fmt.Printf("1. message_fhir_templates:\n")
	fmt.Printf("   SELECT * FROM message_fhir_templates WHERE message_type = '%s';\n", messageType)
	fmt.Printf("   Expected: 1 row with fhir_resources = [\"Patient\"]\n\n")

	fmt.Printf("2. field_element_mappings:\n")
	fmt.Printf("   SELECT segment_name, hl7_field, fhir_element_path, data_type_transform\n")
	fmt.Printf("   FROM field_element_mappings\n")
	fmt.Printf("   WHERE fhir_resource_type = 'Patient';\n")
	fmt.Printf("   Expected: ~8-10 rows for Patient fields\n\n")

	fmt.Printf("3. value_set_mappings:\n")
	fmt.Printf("   SELECT mapping_name, hl7_value, fhir_code\n")
	fmt.Printf("   FROM value_set_mappings\n")
	fmt.Printf("   WHERE mapping_name = 'administrative_gender';\n")
	fmt.Printf("   Expected: 4 rows (M->male, F->female, O->other, U->unknown)\n\n")

	fmt.Printf("❌ If any of these queries return empty results, you need to seed your database!\n")
}

func showExpectedOutput() {
	expected := map[string]interface{}{
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

	expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
	fmt.Printf("%s\n", expectedJSON)
}

func showActionItems() {
	actionItems := []string{
		"1. 🗄️  SEED DATABASE: Execute the SQL seed data provided in the artifacts",
		"2. 🔧 REPLACE SERVICE: Use the EnhancedHL7FHIRTransformService",
		"3. 🧪 TEST FIELD EXTRACTION: Verify all PID fields can be extracted correctly",
		"4. 🔄 TEST TRANSFORMATIONS: Validate each data type transformer works",
		"5. 📊 ENABLE LOGGING: Add debug logs to see transformation progress",
		"6. ✅ VALIDATE OUTPUT: Compare actual vs expected FHIR resource",
	}

	for _, item := range actionItems {
		fmt.Printf("%s\n", item)
	}

	fmt.Printf("\n🚀 Quick Fix: Use the enhanced service with built-in fallbacks\n")
	fmt.Printf("   It will work even without database configuration!\n\n")

	fmt.Printf("💡 Next Steps:\n")
	fmt.Printf("   - Run the enhanced transformation service\n")
	fmt.Printf("   - Check if you get the expected Patient resource\n")
	fmt.Printf("   - If issues persist, enable verbose logging to see exact failure points\n")
}
