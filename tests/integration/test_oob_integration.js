#!/usr/bin/env node
// test_oob_integration.js
// Test the OOB template integration with wizard step 4

const fetch = require('node-fetch');

// Sample parsed HL7 data (similar to what wizard sends)
const testRequest = {
    parsedHL7Data: {
        data: {
            messageType: {
                name: "ADT^A01"
            },
            segments: {
                MSH: [{
                    field_1: "^~\\&",
                    field_2: "SENDING_SYSTEM",
                    field_3: "RECEIVING_SYSTEM",
                    field_8: "ADT^A01^ADT_A01",
                    field_9: "20241225123000",
                    field_10: "P",
                    field_11: "2.5.1"
                }],
                PID: [{
                    field_3: {
                        component_1: "123456789",  // Patient ID
                        component_4: "MR"          // ID type
                    },
                    field_5: {
                        component_1: "DOE",        // Last name
                        component_2: "JOHN",       // First name
                        component_3: "MIDDLE"      // Middle name
                    },
                    field_7: "19850315",           // Birth date
                    field_8: "M",                  // Gender
                    field_11: {
                        component_1: "123 MAIN ST",
                        component_3: "ANYTOWN",
                        component_4: "NY",
                        component_5: "12345"
                    },
                    field_13: {
                        component_1: "555-123-4567"
                    }
                }],
                PV1: [{
                    field_2: "I",                  // Patient class (Inpatient)
                    field_3: {
                        component_1: "ER",
                        component_2: "EMERGENCY"
                    },
                    field_7: {
                        component_1: "123456",     // Attending doctor
                        component_2: "SMITH",
                        component_3: "JOHN"
                    }
                }]
            }
        }
    },
    targetProfile: "Patient",
    fhirVersion: "R4",
    createBundle: false,
    requestId: "test_oob_integration_" + Date.now()
};

async function testOOBIntegration() {
    console.log('🧪 Testing OOB Template Integration...');
    console.log('🎯 Request ID:', testRequest.requestId);
    console.log('📋 Message Type:', testRequest.parsedHL7Data.data.messageType.name);

    try {
        console.log('📡 Sending request to /api/fhir/test-transform-v3...');

        const response = await fetch('http://localhost:8080/api/fhir/test-transform-v3', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(testRequest)
        });

        console.log('📨 Response status:', response.status);
        console.log('📨 Response headers:', Object.fromEntries(response.headers.entries()));

        const responseText = await response.text();
        console.log('📨 Response body length:', responseText.length, 'characters');

        let responseData;
        try {
            responseData = JSON.parse(responseText);
            console.log('✅ Response parsed as JSON');
        } catch (parseError) {
            console.log('❌ Response is not valid JSON');
            console.log('🔍 Raw response (first 500 chars):', responseText.substring(0, 500));
            return;
        }

        if (response.ok) {
            console.log('🎉 SUCCESS: Transform completed!');
            console.log('📊 Transform success:', responseData.success);
            console.log('📊 FHIR resources generated:', responseData.fhirResources?.length || 0);
            console.log('📊 Resource counts:', responseData.resourceCounts);
            console.log('📊 Mapping stats:', responseData.mappingStats);

            if (responseData.fhirResources && responseData.fhirResources.length > 0) {
                console.log('\n🎯 Generated FHIR Resources:');
                responseData.fhirResources.forEach((resource, index) => {
                    console.log(`  ${index + 1}. ${resource.resourceType} (ID: ${resource.id})`);
                    if (resource.resourceType === 'Patient') {
                        console.log(`     - Name: ${resource.name?.[0]?.family}, ${resource.name?.[0]?.given?.[0]}`);
                        console.log(`     - Gender: ${resource.gender}`);
                        console.log(`     - ID: ${resource.identifier?.[0]?.value}`);
                    }
                });
            }

            if (responseData.warnings && responseData.warnings.length > 0) {
                console.log('\n⚠️ Warnings:');
                responseData.warnings.forEach(warning => console.log('  -', warning));
            }
        } else {
            console.log('❌ FAILED: Transform failed');
            console.log('❌ Error:', responseData.error || responseText);
            console.log('📊 Errors:', responseData.errors);
        }

    } catch (error) {
        console.error('💥 Request failed:', error.message);

        if (error.code === 'ECONNREFUSED') {
            console.log('🔍 Is the Go backend running on port 8080?');
            console.log('🔍 Try: docker-compose up or go run main.go');
        }
    }
}

// Run the test
if (require.main === module) {
    testOOBIntegration().catch(error => {
        console.error('Fatal error:', error);
        process.exit(1);
    });
}

module.exports = { testOOBIntegration };