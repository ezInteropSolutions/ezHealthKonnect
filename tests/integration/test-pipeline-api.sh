#!/bin/bash

# Test pipeline with a sample HL7 message
curl -X POST "http://localhost:3000/api/fhir/pipeline/test" \
  -H "Content-Type: application/json" \
  -d '{
  "pipeline_id": "2fe97e7e-62c6-48c1-a85f-5c325e832e79",
  "test_message": "MSH|^~\\&|SendingApp|SendingFac|ReceivingApp|ReceivingFac|20250101120000||ADT^A01|MSG001|P|2.5\nPID|||12345^^^MRN||Doe^John^A||19800101|M|||123 Main St^^Boston^MA^02101||555-1234|||S||ACC123|||123-45-6789\nPV1||I|ICU^101^1|||||||MED||||||||ACC123|||||||||||||||||||||||||||20250101120000"
}' | python -m json.tool > TestPipelineoutput.json

echo "Test completed. Output saved to TestPipelineoutput.json"
cat TestPipelineoutput.json
