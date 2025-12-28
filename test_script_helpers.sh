#!/bin/bash
curl -X POST http://localhost:3000/api/fhir/pipeline/validate-script \
  -H "Content-Type: application/json" \
  -d '{
    "script": "var lastName = getHL7Field(\"PID.5.1\"); return { lastName: lastName };",
    "pipelineId": 1
  }'
