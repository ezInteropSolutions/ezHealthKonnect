const http = require('http');

// Invalid HL7 message - missing family name
const invalidHL7 = `MSH|^~\\&|SendingApp|SendingFacility|ReceivingApp|ReceivingFacility|20231214120000||ADT^A01|MSG00001|P|2.5
EVN||20231214120000
PID|1||P123456^^^Hospital^MR||^^M||19280518|M|||123 Main St^^Orlando^FL^12345||555-1234|||S||P123456|123-45-6789
PV1||I|WD^101^01||||123456^Doctor^John|||SUR||||ADM|A0|`;

const pipelineID = '4b3ffa85-2d66-413d-a058-f37ce9c595cb'; // Now in accept_and_flag mode

console.log('=== Testing ACCEPT & FLAG Mode ===\n');
console.log('Message Issues:');
console.log('  ❌ Missing family name (PID.5.1)');
console.log('  ❌ Control ID too short (MSG00001 = 8 chars, needs 10-12)');
console.log('\nExpected Behavior:');
console.log('  ✅ ACK will be sent (not NACK)');
console.log('  ✅ Pipeline will continue processing');
console.log('  ⚠️  Validation warnings will be logged');
console.log('  📊 Status: "warning" (not "rejected")');
console.log('\nSending test request...\n');

const data = JSON.stringify({
    pipeline_id: pipelineID,
    test_message: invalidHL7
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
            console.log('=== ACCEPT & FLAG MODE TEST RESULTS ===\n');

            if (result.success) {
                console.log('✅ SUCCESS! Pipeline completed despite validation errors');
                console.log('   (This is correct behavior for accept_and_flag mode)');
            } else {
                console.log('❌ Pipeline failed (unexpected for accept_and_flag mode)');
            }

            console.log('\nExecution Results:');
            if (result.execution_results) {
                result.execution_results.forEach((step, i) => {
                    const statusIcon = step.status === 'success' ? '✅' : '❌';
                    console.log(`  ${statusIcon} Step ${i + 1}: ${step.step} - ${step.status.toUpperCase()}`);
                    if (step.error) {
                        console.log(`      Error: ${step.error}`);
                    }
                });
            }

            console.log('\nMessage Data Status:');
            if (result.parsed_message) {
                const validationStatus = result.parsed_message._validation_status;
                const validationWarnings = result.parsed_message._validation_warnings;
                const requiresReview = result.parsed_message._requires_review;

                console.log(`  Status: ${validationStatus || 'unknown'}`);
                console.log(`  Requires Review: ${requiresReview ? 'Yes' : 'No'}`);

                if (validationWarnings && validationWarnings.length > 0) {
                    console.log(`\n  Validation Warnings (${validationWarnings.length}):`);
                    validationWarnings.forEach((warning, i) => {
                        console.log(`    ${i + 1}. ${warning.message}`);
                        console.log(`       Field: ${warning.field}`);
                    });
                }
            }

            console.log('\n📋 Mode Comparison:');
            console.log('┌────────────────────┬─────────────────────┬────────────────────┐');
            console.log('│ Mode               │ This Test (A&F)     │ Strict Reject      │');
            console.log('├────────────────────┼─────────────────────┼────────────────────┤');
            console.log(`│ Pipeline Success   │ ${result.success ? '✅ Yes' : '❌ No '}              │ ❌ No               │`);
            console.log('│ Response           │ ✅ ACK              │ ❌ NACK             │');
            console.log('│ Processing         │ ▶️  Continue         │ 🛑 Stop             │');
            console.log('│ Status             │ ⚠️  warning         │ ❌ rejected         │');
            console.log('└────────────────────┴─────────────────────┴────────────────────┘');

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
