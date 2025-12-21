# API Endpoint Tester - User Guide

**Feature**: Test API Endpoint Before Configuring Response Mapping
**Date**: 2025-12-20
**Status**: ✅ Implementation Complete

---

## Overview

The API Endpoint Tester allows first-time users (and experienced users) to **test their API configuration and see the actual response** before configuring field mappings. This eliminates guesswork and enables visual field selection.

### **Key Benefits**

✅ **Eliminate Guesswork**: See actual API response structure before configuration
✅ **Visual Field Picker**: Click fields to add them to mapping instead of writing JSONPath manually
✅ **Auto-Suggest Field Names**: Intelligent target field naming ($.patient.id → patientId)
✅ **Immediate Feedback**: Test auth, endpoints, and field mappings instantly
✅ **Error Detection**: Catch configuration errors before saving
✅ **Learning Tool**: Explore API structure interactively

---

## How It Works

### **User Flow**

```
1. Configure API Endpoint (URL, method, auth, field mappings)
   ↓
2. Enter Sample Message Data (for testing field mappings)
   ↓
3. Click "🧪 Test API Endpoint"
   ↓
4. See Full Request/Response
   ↓
5. Click Fields to Add to Response Mapping
   ↓
6. Configuration Auto-Generated
```

---

## Step-by-Step Guide

### **Step 1: Configure API Endpoint**

In the API Enrichment step configuration:

```
Endpoint: https://api.epic.com/empi/patient/{patientId}
Method: GET
Auth Type: Bearer
Bearer Token: your-token-here
Field Mappings:
  - patientId → PID.3
```

### **Step 2: Provide Sample Data**

Enter sample HL7 data to resolve field mappings:

```json
{
  "PID.3": "12345",
  "PID.5": "John^Doe",
  "MSH.9": "ADT^A01"
}
```

This data resolves `{patientId}` in the URL to `12345`.

### **Step 3: Test API Endpoint**

Click the **"🧪 Test API Endpoint"** button.

The system will:
1. Resolve field mappings from sample data
2. Build the actual API request
3. Execute the API call
4. Return full request/response details

### **Step 4: View Results**

#### **Success Response**

```
┌──────────────────────────────────────────────┐
│ ✅ API Call Successful                       │
│ Status: 200 OK (333ms)                       │
├──────────────────────────────────────────────┤
│ Tabs:                                        │
│ [Parsed Response] [Raw Response] [Request]   │
└──────────────────────────────────────────────┘

Parsed Response:
{
  "patient": {
    "id": "12345",
    "firstName": "John",
    "lastName": "Doe",
    "dateOfBirth": "1990-12-25",
    "insurance": [
      {
        "type": "primary",
        "memberId": "ABC123"
      },
      {
        "type": "secondary",
        "memberId": "XYZ789"
      }
    ]
  }
}
```

#### **Failure Response**

```
┌──────────────────────────────────────────────┐
│ ❌ API Call Failed                           │
│ Error: 401 Unauthorized                      │
│                                              │
│ ▼ Request Details                            │
└──────────────────────────────────────────────┘
```

Shows authentication errors, network errors, or API errors immediately.

### **Step 5: Visual Field Picker**

After successful API call, a field picker appears:

```
📋 Response Fields (Click to Add to Mapping)

┌─────────────────────────────────────────────────────┐
│ $.patient.id              [string]    + Add         │
│ Sample: "12345"                                     │
├─────────────────────────────────────────────────────┤
│ $.patient.firstName       [string]    + Add         │
│ Sample: "John"                                      │
├─────────────────────────────────────────────────────┤
│ $.patient.lastName        [string]    + Add         │
│ Sample: "Doe"                                       │
├─────────────────────────────────────────────────────┤
│ $.patient.dateOfBirth     [string]    + Add         │
│ Sample: "1990-12-25"                                │
├─────────────────────────────────────────────────────┤
│ $.patient.insurance       [array]     + Add         │
│ Sample: [2 items]                                   │
├─────────────────────────────────────────────────────┤
│ $.patient.insurance[0].type       [string] + Add    │
│ Sample: "primary"                                   │
├─────────────────────────────────────────────────────┤
│ $.patient.insurance[0].memberId   [string] + Add    │
│ Sample: "ABC123"                                    │
└─────────────────────────────────────────────────────┘
```

### **Step 6: Add Fields to Mapping**

Click **"+ Add to Mapping"** on desired fields:

- Click `$.patient.id` → Auto-creates rule with targetField: `patientId`
- Click `$.patient.firstName` → Auto-creates rule with targetField: `patientFirstName`
- Click `$.patient.insurance[0].memberId` → Auto-creates rule with targetField: `patientInsuranceMemberId`

The system **automatically generates** the response mapping configuration:

```json
{
  "responseMapping": {
    "mode": "custom",
    "extractors": [
      {
        "sourcePath": "$.patient.id",
        "targetField": "patientId",
        "transformType": "none"
      },
      {
        "sourcePath": "$.patient.firstName",
        "targetField": "patientFirstName",
        "transformType": "none"
      },
      {
        "sourcePath": "$.patient.insurance[0].memberId",
        "targetField": "patientInsuranceMemberId",
        "transformType": "none"
      }
    ]
  }
}
```

---

## API Endpoint

### **Backend Endpoint**

```http
POST /api/fhir/pipeline/test-api-endpoint
Content-Type: application/json

{
  "stepConfig": {
    "endpoint": "https://api.epic.com/patient/{id}",
    "method": "GET",
    "authType": "bearer",
    "bearerToken": "...",
    "fieldMappings": {
      "id": "PID.3"
    }
  },
  "testData": {
    "PID.3": "12345"
  }
}
```

### **Response Format (Success)**

```json
{
  "success": true,
  "message": "API call successful - inspect response to configure field mapping",
  "request": {
    "method": "GET",
    "url": "https://api.epic.com/patient/12345",
    "headers": {
      "Authorization": "***MASKED***",
      "Content-Type": "application/json"
    },
    "sent_at": "2025-12-20T10:30:00.123Z",
    "timeout_ms": 5000
  },
  "response": {
    "status_code": 200,
    "status_text": "OK",
    "duration_ms": 333,
    "headers": {
      "Content-Type": "application/json"
    },
    "body_raw": "{\"patient\":{\"id\":\"12345\",...}}",
    "body_parsed": {
      "patient": {
        "id": "12345",
        "firstName": "John",
        "lastName": "Doe"
      }
    },
    "field_structure": [
      {
        "path": "$.patient.id",
        "key": "id",
        "type": "string",
        "sample": "12345"
      },
      {
        "path": "$.patient.firstName",
        "key": "firstName",
        "type": "string",
        "sample": "John"
      }
    ]
  },
  "help": "Click on fields below to add them to your response mapping configuration"
}
```

### **Response Format (Failure)**

```json
{
  "success": false,
  "error": "401 Unauthorized",
  "message": "API call failed - check endpoint configuration and authentication",
  "request": {
    "method": "GET",
    "url": "https://api.epic.com/patient/12345",
    "headers": {...}
  },
  "response": {
    "error": "401 Unauthorized",
    "duration_ms": 150
  }
}
```

---

## Field Structure Analysis

The backend automatically analyzes the API response and generates a `field_structure` array:

### **Example Response**

```json
{
  "patient": {
    "id": "12345",
    "name": {
      "first": "John",
      "last": "Doe"
    },
    "contacts": [
      {"type": "phone", "value": "555-1234"},
      {"type": "email", "value": "john@example.com"}
    ]
  }
}
```

### **Generated Field Structure**

```json
[
  {"path": "$.patient.id", "key": "id", "type": "string", "sample": "12345"},
  {"path": "$.patient.name", "key": "name", "type": "object", "sample": "{...}"},
  {"path": "$.patient.name.first", "key": "first", "type": "string", "sample": "John"},
  {"path": "$.patient.name.last", "key": "last", "type": "string", "sample": "Doe"},
  {"path": "$.patient.contacts", "key": "contacts", "type": "array", "sample": "[2 items]"},
  {"path": "$.patient.contacts[0].type", "key": "type", "type": "string", "sample": "phone"},
  {"path": "$.patient.contacts[0].value", "key": "value", "type": "string", "sample": "555-1234"}
]
```

This enables the UI to show a **flat, clickable list** of all available fields.

---

## Auto-Generated Target Field Names

The system intelligently suggests target field names:

| JSONPath | Suggested Target Field |
|----------|------------------------|
| `$.id` | `id` |
| `$.patient.id` | `patientId` |
| `$.patient.name.first` | `patientNameFirst` |
| `$.data.insurance[0].memberId` | `dataInsuranceMemberId` |
| `$.results[0].value` | `resultsValue` |

**Algorithm**:
1. Extract segments from JSONPath
2. Remove `$`, `.`, and array indices `[0]`
3. Apply camelCase: first segment lowercase, rest capitalized

---

## Frontend Integration

### **Include Component**

Add to your HTML page:

```html
<script src="/js/pipeline/components/APIEndpointTester.js"></script>
<link rel="stylesheet" href="/css/api-endpoint-tester.css">
```

### **Initialize Component**

```javascript
// Create tester instance
const tester = new APIEndpointTester('api-endpoint-tester-container');

// Set callback for when user adds a field
tester.setOnAddMappingRule((ruleData) => {
    console.log('User added field:', ruleData);
    // {
    //   sourcePath: "$.patient.id",
    //   targetField: "patientId",
    //   transformType: "none",
    //   fieldType: "string"
    // }

    // Add to your response mapping config
    addMappingRuleToUI(ruleData);
});

// Render with current step configuration
tester.render(currentStepConfig);
```

### **Event Listener Alternative**

```javascript
document.getElementById('api-endpoint-tester-container')
    .addEventListener('add-mapping-rule', (e) => {
        const ruleData = e.detail;
        addMappingRuleToUI(ruleData);
    });
```

---

## Use Cases

### **Use Case 1: First-Time Epic EMPI Integration**

**Scenario**: User has never seen Epic's EMPI API response before.

**Flow**:
1. User enters Epic endpoint and auth token
2. Clicks "Test API Endpoint"
3. Sees Epic's response structure for the first time
4. Clicks desired fields to build mapping
5. **Result**: Configuration ready in 2 minutes vs 30+ minutes of trial-and-error

---

### **Use Case 2: Debugging Failed API Call**

**Scenario**: API enrichment step keeps failing in production.

**Flow**:
1. User opens step configuration
2. Clicks "Test API Endpoint"
3. Sees error: `401 Unauthorized`
4. Realizes bearer token expired
5. Updates token, tests again
6. **Result**: Issue identified and fixed in 1 minute

---

### **Use Case 3: Complex Nested Response**

**Scenario**: API returns deeply nested JSON with 50+ fields.

**Flow**:
1. User tests API endpoint
2. Field picker shows flat list of all 50+ fields
3. User clicks only the 5 fields they need
4. **Result**: Extract exactly what's needed, ignore the rest (performance win)

---

## Implementation Files

### **Backend**
- [controllers/transformation_test_controller.go](controllers/transformation_test_controller.go) - TestAPIEndpoint method (lines 430-528)
- [services/executors/enrichment/api_enrichment_executor.go](services/executors/enrichment/api_enrichment_executor.go) - Field structure analysis (lines 620-710)
- [main.go](main.go) - Route registration (line 344)

### **Frontend**
- [public/js/pipeline/components/APIEndpointTester.js](public/js/pipeline/components/APIEndpointTester.js) - Main component (370 lines)
- [public/css/api-endpoint-tester.css](public/css/api-endpoint-tester.css) - Styling (420 lines)

### **Documentation**
- [API_ENDPOINT_TESTER_GUIDE.md](API_ENDPOINT_TESTER_GUIDE.md) - This file (user guide)

---

## Testing

### **Manual Test**

```bash
# 1. Start backend
docker-compose up app

# 2. Open browser
http://localhost:3000/pipeline-builder.html

# 3. Create API enrichment step
# 4. Configure endpoint
# 5. Click "Test API Endpoint"
# 6. Verify response appears
# 7. Click field to add to mapping
# 8. Verify mapping rule created
```

### **API Test with cURL**

```bash
curl -X POST http://localhost:8080/api/fhir/pipeline/test-api-endpoint \
  -H "Content-Type: application/json" \
  -d '{
    "stepConfig": {
      "endpoint": "https://jsonplaceholder.typicode.com/users/1",
      "method": "GET"
    },
    "testData": {}
  }'
```

**Expected Response**:
```json
{
  "success": true,
  "response": {
    "status_code": 200,
    "body_parsed": {
      "id": 1,
      "name": "Leanne Graham",
      ...
    },
    "field_structure": [
      {"path": "$.id", "type": "number", "sample": 1},
      {"path": "$.name", "type": "string", "sample": "Leanne Graham"},
      ...
    ]
  }
}
```

---

## Future Enhancements

### **Phase 2** (Future)
- [ ] **Save Test Data**: Save sample data with step configuration
- [ ] **Test History**: Show previous test results
- [ ] **Diff Viewer**: Compare current vs previous response structure
- [ ] **Smart Suggestions**: AI-powered field mapping suggestions
- [ ] **Template Detection**: Auto-detect if response matches known template (Epic, Cerner, etc.)
- [ ] **Response Mocking**: Mock API responses for offline development
- [ ] **Performance Metrics**: Show API response time trends

---

## Conclusion

The API Endpoint Tester transforms the response mapping configuration experience from **"trial and error"** to **"test and click"**. First-time users can now:

1. ✅ See actual API responses before configuration
2. ✅ Visually pick fields instead of writing JSONPath manually
3. ✅ Catch errors immediately (auth, endpoint, field mapping)
4. ✅ Learn API structure through exploration
5. ✅ Complete configuration in minutes instead of hours

**Status**: ✅ **Ready for Testing**

---

**Last Updated**: 2025-12-20
**Total Lines**: ~790 lines (backend + frontend + CSS)
**Feature Complexity**: Medium
**User Impact**: **HIGH** - Eliminates major pain point for first-time users
