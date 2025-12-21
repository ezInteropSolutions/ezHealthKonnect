# Step Output Improvements - User Feedback

**Date**: 2025-12-20
**Context**: User testing revealed several issues with step output structure

## Issues Identified

### 1. ✅ Validation Detailed Output Toggle
**Status**: **WORKING CORRECTLY** (verified)

**Issue Reported**: "For validation we added support for to show detailed output but I think it is not working meaning the output format does not change."

**Testing Results**:
```json
// detailedOutput: true
{
  "field_results": [
    {"field": "MSH.9", "validation_type": "required", "valid": true},
    {"field": "PID.3", "validation_type": "required", "valid": true}
  ],
  "validation_status": "passed"
}

// detailedOutput: false
{
  "fields_validated": 2,
  "validation_status": "passed"
}
```

**Resolution**: Feature is working as designed. Toggle successfully changes output format.

---

###

 2. ❌ Redundancy: execution_results vs step_outputs
**Status**: **NEEDS FIX**

**Issue**: Both arrays contain the same data

**Current Structure**:
```json
{
  "execution_results": [
    {
      "step_name": "Validate Required HL7 Fields",
      "output": {"fields_validated": 2, "validation_status": "passed"}
    }
  ],
  "step_outputs": {
    "Validate Required HL7 Fields": {
      "fields_validated": 2,
      "validation_status": "passed"
    }
  }
}
```

**Problem**: `step_outputs` is literally just a map version of `execution_results` - complete duplication.

**Proposed Solution**: **Keep BOTH but with different purposes**

```json
{
  // Execution metadata (order, timing, success/fail)
  "execution_results": [
    {
      "step_name": "Validate Required HL7 Fields",
      "step_type": "pre.validation",
      "sequence": 10,
      "success": true,
      "duration_ms": 45
      // NO output data here
    }
  ],

  // Step outputs only (data produced by steps)
  "step_outputs": {
    "Validate Required HL7 Fields": {
      "fields_validated": 2,
      "validation_status": "passed"
    }
  }
}
```

**Benefit**: Clear separation of concerns
- `execution_results` = **execution metadata** (order, timing, errors)
- `step_outputs` = **data produced** (validation results, API responses, etc.)

---

### 3. ❌ Metadata Enrichment Redundancy
**Status**: **NEEDS FIX**

**Issue**: `field_names` array duplicates the keys of `metadata` object

**Current Output**:
```json
{
  "metadata": {
    "receivedAt": "2025-12-20T04:12:03Z",
    "processedAt": "2025-12-20T04:12:03Z",
    "correlationId": "550e8400-e29b-41d4-a716-446655440000"
  },
  "fields_added": 3,
  "field_names": ["receivedAt", "processedAt", "correlationId"],  // ❌ Redundant
  "message": "Added 3 metadata fields"
}
```

**Proposed Solution**: Remove `field_names` - it can be derived from `Object.keys(metadata)`

```json
{
  "metadata": {
    "receivedAt": "2025-12-20T04:12:03Z",
    "processedAt": "2025-12-20T04:12:03Z",
    "correlationId": "550e8400-e29b-41d4-a716-446655440000"
  },
  "fields_added": 3,
  "message": "Added 3 metadata fields"
}
```

---

### 4. ❌ API Enrichment: No Request Visibility
**Status**: **NEEDS IMPLEMENTATION**

**Issue**: "Api enrichment, i see the configured fields but how we see request & response"

**Current Output**:
```json
{
  "api_endpoint": "https://jsonplaceholder.typicode.com/users/1",
  "http_method": "GET",
  "enriched_path": "empiData",
  "api_response": {...},
  "message": "API enrichment completed"
}
```

**Missing**:
- Request headers
- Request body (for POST/PUT)
- Response status code
- Response headers
- Response time

**Proposed Solution**: Add comprehensive request/response details

```json
{
  "request": {
    "method": "GET",
    "url": "https://jsonplaceholder.typicode.com/users/1",
    "headers": {
      "Content-Type": "application/json",
      "Authorization": "Bearer ***"  // Masked
    },
    "body": null,  // For POST/PUT
    "sent_at": "2025-12-20T04:12:03.123Z"
  },
  "response": {
    "status_code": 200,
    "status_text": "OK",
    "headers": {
      "Content-Type": "application/json",
      "Content-Length": "512"
    },
    "body": {...},  // Actual response data
    "received_at": "2025-12-20T04:12:03.456Z",
    "duration_ms": 333
  },
  "enriched_path": "empiData",
  "message": "API enrichment completed"
}
```

**Configuration**: Add option to control detail level

```json
{
  "step_type": "pre.enrichment.api",
  "config": {
    "endpoint": "https://api.example.com/patient",
    "method": "GET",
    "includeRequestDetails": true,   // NEW
    "includeResponseHeaders": true,  // NEW
    "maskSensitiveHeaders": true     // NEW (Authorization, API-Key, etc.)
  }
}
```

---

### 5. ❌ API Response Processing
**Status**: **NEEDS DESIGN**

**Issue**: "also response can be varied how do build support to process response and parse the required information in the message"

**Problem**: API responses are complex and nested. Users need to extract specific data.

**Example Scenario**:
```json
// API Response
{
  "status": "success",
  "data": {
    "patient": {
      "id": "12345",
      "demographics": {
        "firstName": "John",
        "lastName": "Doe",
        "ssn": "123-45-6789"
      },
      "insurance": [
        {"type": "primary", "provider": "Aetna", "memberId": "ABC123"},
        {"type": "secondary", "provider": "UHC", "memberId": "XYZ789"}
      ]
    }
  }
}

// User wants to extract:
// - Patient ID → message.patientId
// - Full Name → message.patientName
// - Primary Insurance → message.primaryInsurance
```

**Proposed Solution**: **Response Mapping Configuration**

```json
{
  "step_type": "pre.enrichment.api",
  "config": {
    "endpoint": "https://api.example.com/patient",
    "method": "GET",
    "targetPath": "empiData",

    // NEW: Response mapping rules
    "responseMapping": {
      "enabled": true,
      "mappings": [
        {
          "source": "data.patient.id",
          "target": "patientId",
          "type": "string"
        },
        {
          "source": "data.patient.demographics",
          "target": "patientName",
          "type": "computed",
          "expression": "`${demographics.firstName} ${demographics.lastName}`"
        },
        {
          "source": "data.patient.insurance",
          "target": "primaryInsurance",
          "type": "filter",
          "filter": "item => item.type === 'primary'"
        }
      ]
    }
  }
}
```

**Alternative: JSONPath Extractors**

```json
{
  "responseMapping": {
    "extractors": [
      {
        "jsonPath": "$.data.patient.id",
        "targetField": "patientId"
      },
      {
        "jsonPath": "$.data.patient.insurance[?(@.type=='primary')].memberId",
        "targetField": "primaryInsuranceId"
      }
    ]
  }
}
```

**Alternative: JavaScript Transform**

```json
{
  "responseMapping": {
    "type": "javascript",
    "script": `
      function transform(apiResponse) {
        return {
          patientId: apiResponse.data.patient.id,
          patientName: \`\${apiResponse.data.patient.demographics.firstName} \${apiResponse.data.patient.demographics.lastName}\`,
          primaryInsurance: apiResponse.data.patient.insurance.find(i => i.type === 'primary')
        };
      }
    `
  }
}
```

---

## Implementation Plan

### Phase 1: Clean Up Redundancy (Quick Wins)
1. **execution_results**: Remove `output` field, keep only execution metadata
2. **Metadata enrichment**: Remove `field_names` array
3. **Update controller**: [transformation_test_controller.go](controllers/transformation_test_controller.go)

**Estimated Time**: 30 minutes

### Phase 2: Enhance API Enrichment Output
1. Add `request` object with full details
2. Add `response` object with status, headers, timing
3. Add configuration options for detail level
4. Implement header masking for sensitive data

**Files to Modify**:
- [api_enrichment_executor.go](services/executors/enrichment/api_enrichment_executor.go)
- [transformation_test_controller.go](controllers/transformation_test_controller.go)
- [PropertiesPanel.js](public/js/pipeline/managers/PropertiesPanel.js) - UI checkboxes

**Estimated Time**: 2 hours

### Phase 3: Response Mapping System (Complex)
**Design Decision Required**: Choose approach
- Option A: JSONPath extractors (simple, declarative)
- Option B: JavaScript transform (flexible, powerful)
- Option C: Mapping rules (structured, UI-friendly)

**Recommendation**: Start with **JSONPath extractors** (Option A)
- Simpler to implement
- Easier to configure in UI
- Covers 80% of use cases
- Can add JavaScript option later for advanced cases

**Files to Create**:
- `services/extractors/jsonpath_extractor.go` (NEW)
- `models/response_mapping_models.go` (NEW)

**Files to Modify**:
- `services/executors/enrichment/api_enrichment_executor.go`
- `public/js/pipeline/components/ResponseMappingBuilder.js` (NEW UI component)

**Estimated Time**: 6-8 hours

---

## Proposed Output Structure (Final)

```json
{
  // Execution metadata ONLY (no data duplication)
  "execution_results": [
    {
      "step_id": "abc123",
      "step_name": "Validate Required HL7 Fields",
      "step_type": "pre.validation",
      "sequence": 10,
      "success": true,
      "duration_ms": 45,
      "executed_at": "2025-12-20T04:12:03Z"
    },
    {
      "step_id": "def456",
      "step_name": "Enrich Patient from EMPI",
      "step_type": "pre.enrichment.api",
      "sequence": 20,
      "success": true,
      "duration_ms": 333,
      "executed_at": "2025-12-20T04:12:03Z"
    }
  ],

  // Step outputs ONLY (data produced)
  "step_outputs": {
    "Validate Required HL7 Fields": {
      "field_results": [...],
      "validation_status": "passed"
    },
    "Enrich Patient from EMPI": {
      "request": {
        "method": "GET",
        "url": "https://api.example.com/patient/12345",
        "headers": {"Authorization": "Bearer ***"},
        "sent_at": "2025-12-20T04:12:03.123Z"
      },
      "response": {
        "status_code": 200,
        "body": {...},
        "duration_ms": 333
      },
      "extracted_data": {
        "patientId": "12345",
        "patientName": "John Doe",
        "primaryInsurance": {...}
      }
    },
    "Add Metadata": {
      "metadata": {
        "receivedAt": "2025-12-20T04:12:03Z",
        "correlationId": "..."
      },
      "fields_added": 3
    }
  },

  // Transformed message (final output)
  "parsed_message": {
    "enhancedSegments": {...},
    "empiData": {...},
    "metadata": {...}
  },

  "steps_count": 3,
  "success": true
}
```

---

## Questions for User

1. **execution_results separation**: Do you agree with removing `output` from execution_results and keeping only execution metadata?

2. **API request/response details**: Which details are most important to you?
   - ☐ Request headers
   - ☐ Request body
   - ☐ Response headers
   - ☐ Response status code
   - ☐ Response timing
   - ☐ All of the above

3. **Response mapping**: Which approach do you prefer?
   - ☐ Option A: JSONPath extractors (simple)
   - ☐ Option B: JavaScript transform (flexible)
   - ☐ Option C: Both (start with JSONPath, add JavaScript later)

4. **Sensitive data masking**: Should we automatically mask sensitive headers like `Authorization`, `API-Key`, etc.?
   - ☐ Yes, always mask
   - ☐ No, show everything
   - ☐ Make it configurable per step

5. **Priority**: Which fix is most urgent?
   1. Remove redundancy (quick)
   2. Add API request/response details (medium)
   3. Add response mapping system (complex)

---

**Ready to proceed with fixes based on your feedback!**
