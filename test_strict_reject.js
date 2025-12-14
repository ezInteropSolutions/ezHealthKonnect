const http = require('http');

// Invalid HL7 message - missing family name
const invalidHL7 = `MSH|^~\\&|SendingApp|SendingFacility|ReceivingApp|ReceivingFacility|20231214120000||ADT^A01|MSG00001|P|2.5
EVN||20231214120000
PID|1||P123456^^^Hospital^MR||^^M||19280518|M|||123 Main St^^Orlando^FL^12345||555-1234|||S||P123456|123-45-6789
PV1||I|WD^101^01||||123456^Doctor^John|||SUR||||ADM|A0|`;

const pipelineID = '4b3ffa85-2d66-413d-a058-f37ce9c595cb'; // Now in strict_reject mode

console.log('=== Testing STRICT REJECT Mode ===\n');
console.log('Message Issues:');
console.log('  ❌ Missing family name (PID.5.1)');
console.log('  ❌ Control ID too short (MSG00001 = 8 chars, needs 10-12)');
console.log('\nExpected Behavior:');
console.log('  ❌ NACK will be sent');
console.log('  🛑 Pipeline will STOP immediately');
console.log('  ❌ No further processing');
console.log('  📊 Status: "rejected"');
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
            console.log('=== STRICT REJECT MODE TEST RESULTS ===\n');

            if (!result.success) {
                console.log('❌ Pipeline REJECTED (This is correct behavior for strict_reject mode)');
            } else {
                console.log('✅ Pipeline succeeded (unexpected for invalid message)');
            }

            console.log(`\nError Message: ${result.error || 'None'}`);

            console.log('\nExecution Results:');
            if (result.execution_results) {
                result.execution_results.forEach((step, i) => {
                    const statusIcon = step.status === 'success' ? '✅' : step.status === 'failed' ? '❌' : '⏭️';
                    console.log(`  ${statusIcon} Step ${i + 1}: ${step.step} - ${step.status.toUpperCase()}`);
                    if (step.error) {
                        console.log(`      Error: ${step.error}`);
                    }
                });
            }

            console.log('\nMessage Data Status:');
            if (result.parsed_message) {
                const validationStatus = result.parsed_message._validation_status;
                const validationErrors = result.parsed_message._validation_errors;

                console.log(`  Status: ${validationStatus || 'unknown'}`);

                if (validationErrors && validationErrors.length > 0) {
                    console.log(`\n  Validation Errors (${validationErrors.length}):`);
                    validationErrors.forEach((error, i) => {
                        console.log(`    ${i + 1}. ${error.message}`);
                        console.log(`       Field: ${error.field}`);
                    });
                }
            }

            console.log('\n📋 Mode Comparison:');
            console.log('┌────────────────────┬─────────────────────┬────────────────────┐');
            console.log('│ Mode               │ Strict Reject       │ Accept & Flag      │');
            console.log('├────────────────────┼─────────────────────┼────────────────────┤');
            console.log(`│ Pipeline Success   │ ${result.success ? '✅ Yes' : '❌ No '}              │ ✅ Yes              │`);
            console.log('│ Response           │ ❌ NACK             │ ✅ ACK              │');
            console.log('│ Processing         │ 🛑 Stop             │ ▶️  Continue         │');
            console.log('│ Status             │ ❌ rejected         │ ⚠️  warning         │');
            console.log('│ Use Case           │ Critical data       │ Data quality       │');
            console.log('└────────────────────┴─────────────────────┴────────────────────┘');

            console.log('\n💡 Configuration Change:');
            console.log('   To switch to lenient mode (ACK with warnings):');
            console.log(`   node change_validation_mode.js ${pipelineID} accept_and_flag`);
            console.log('');
            console.log('   To disable validation entirely:');
            console.log(`   node change_validation_mode.js ${pipelineID} no_validation`);

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
