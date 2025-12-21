const http = require('http');

// Sample HL7 message for testing
const sampleHL7 = `MSH|^~\\&|SendingApp|SendingFacility|ReceivingApp|ReceivingFacility|20231214120000||ADT^A01|MSG00001|P|2.5
EVN||20231214120000
PID|1||P123456^^^Hospital^MR||Mouse^Mickey^M||19280518|M|||123 Main St^^Orlando^FL^12345||555-1234|||S||P123456|123-45-6789
PV1||I|WD^101^01||||123456^Doctor^John|||SUR||||ADM|A0|`;

// Pipeline with validation rules
const pipelineID = '4b3ffa85-2d66-413d-a058-f37ce9c595cb'; // Has comprehensive validation rules

// Test 1: Valid message (should pass all validations)
console.log('=== Test 1: Valid HL7 Message ===');
testPipeline(pipelineID, sampleHL7, 'Test 1');

// Test 2: Invalid message - missing required field (PID.5.1)
const invalidHL7MissingName = sampleHL7.replace('Mouse^Mickey^M', '^^M');
console.log('\n=== Test 2: Invalid Message - Missing Family Name ===');
setTimeout(() => testPipeline(pipelineID, invalidHL7MissingName, 'Test 2'), 2000);

// Test 3: Invalid message - wrong date format
const invalidHL7WrongDate = sampleHL7.replace('19280518', '1928-05-18');
console.log('\n=== Test 3: Invalid Message - Wrong Date Format ===');
setTimeout(() => testPipeline(pipelineID, invalidHL7WrongDate, 'Test 3'), 4000);

// Test 4: Invalid message - patient ID doesn't match pattern
const invalidHL7WrongPattern = sampleHL7.replace('P123456', 'X123456');
console.log('\n=== Test 4: Invalid Message - Patient ID Pattern Mismatch ===');
setTimeout(() => testPipeline(pipelineID, invalidHL7WrongPattern, 'Test 4'), 6000);

function testPipeline(pipelineID, message, testName) {
    const data = JSON.stringify({
        pipeline_id: pipelineID,
        test_message: message
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
                console.log(`\n${testName} Results:`);
                console.log('Success:', result.success);
                console.log('Error:', result.error || 'None');

                if (result.execution_results) {
                    console.log('\nExecution Results:');
                    result.execution_results.forEach((step, i) => {
                        console.log(`  Step ${i + 1}: ${step.step} - ${step.status}`);
                        if (step.error) {
                            console.log(`    Error: ${step.error}`);
                        }
                    });
                }
            } catch (e) {
                console.error('Failed to parse response:', e.message);
                console.error('Response:', responseData);
            }
        });
    });

    req.on('error', (error) => {
        console.error(`${testName} Request failed:`, error.message);
    });

    req.write(data);
    req.end();
}
