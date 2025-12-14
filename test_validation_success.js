const http = require('http');

// Valid HL7 message that should pass ALL validations
const validHL7 = `MSH|^~\\&|SendingApp|SendingFacility|ReceivingApp|ReceivingFacility|20231214120000||ADT^A01|MSG000123456|P|2.5
EVN||20231214120000
PID|1||P123456789^^^Hospital^MR||Mouse^Mickey^M||19280518|M|||123 Main St^^Orlando^FL^12345||555-1234|||S||P123456789|123-45-6789
PV1||I|WD^101^01||||123456^Doctor^John|||SUR||||ADM|A0|`;

const pipelineID = '4b3ffa85-2d66-413d-a058-f37ce9c595cb';

console.log('=== Test: Valid HL7 Message (Should Pass All Validations) ===\n');
console.log('Validation Rules Being Tested:');
console.log('1. MSH.9 (Message Type) - Required');
console.log('2. PID.3 (Patient ID) - Required');
console.log('3. PID.5.1 (Family Name) - Required');
console.log('4. PID.7 (Date of Birth) - Format check (YYYYMMDD)');
console.log('5. PID.3.1 (Patient ID) - Pattern check (^P[0-9]+)');
console.log('6. MSH.10 (Control ID) - Length check (10-12 characters)');
console.log('\nMessage Control ID: MSG000123456 (12 characters) ✓');
console.log('Patient ID: P123456789 (starts with P) ✓');
console.log('Family Name: Mouse ✓');
console.log('Date of Birth: 19280518 (YYYYMMDD format) ✓');
console.log('\nSending test request...\n');

const data = JSON.stringify({
    pipeline_id: pipelineID,
    test_message: validHL7
});

const options = {
    hostname: 'localhost',
    port: 8080,
    path: '/api/fhir/pipeline/test',
    method: 'POST',
    headers: {
        'Content-Type': 'application/json',
        'Content-Length': data.length
    }
};

const req = http.request(options, (res) => {
    let responseData = '';

    res.on('data', (chunk) => {
        responseData += chunk;
    });

    res.on('end', () => {
        try {
            const result = JSON.parse(responseData);
            console.log('=== VALIDATION TEST RESULTS ===\n');
            console.log('Success:', result.success ? '✅ PASSED' : '❌ FAILED');
            console.log('Error:', result.error || 'None');

            if (result.execution_results) {
                console.log('\nExecution Steps:');
                result.execution_results.forEach((step, i) => {
                    const statusIcon = step.status === 'success' ? '✅' : '❌';
                    console.log(`  ${statusIcon} Step ${i + 1}: ${step.step} - ${step.status.toUpperCase()}`);
                    if (step.error) {
                        console.log(`      Error: ${step.error}`);
                    }
                });
            }

            if (result.success) {
                console.log('\n🎉 SUCCESS! All validations passed!');
                console.log('✓ Required fields are present');
                console.log('✓ Format validations passed');
                console.log('✓ Pattern validations passed');
                console.log('✓ Length validations passed');
            } else {
                console.log('\n❌ FAILED! Validation errors detected');
            }
        } catch (e) {
            console.error('Failed to parse response:', e.message);
            console.error('Response:', responseData);
        }
    });
});

req.on('error', (error) => {
    console.error('Request failed:', error.message);
});

req.write(data);
req.end();
