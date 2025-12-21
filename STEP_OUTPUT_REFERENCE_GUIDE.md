# Step Output Reference Guide

## Overview

The transformation pipeline automatically captures outputs from each step execution, making them available for reference in subsequent steps and delivery configurations. This guide explains how to access and use step outputs in your pipeline configurations.

## Table of Contents

1. [Output Structure](#output-structure)
2. [Referencing Step Outputs](#referencing-step-outputs)
3. [Use Cases](#use-cases)
4. [Step-Specific Output Examples](#step-specific-output-examples)
5. [Best Practices](#best-practices)

---

## Output Structure

Every pipeline execution returns data in **two formats** for maximum flexibility:

### 1. Array Format (Execution Order)
```json
{
  "execution_results": [
    {
      "step_name": "Validate Required HL7 Fields",
      "step_type": "pre.validation",
      "success": true,
      "output": {
        "fields_validated": 2,
        "validation_status": "passed"
      }
    },
    {
      "step_name": "Enrich Patient from EMPI",
      "step_type": "pre.enrichment.api",
      "success": true,
      "output": {
        "api_response": {...},
        "enriched_path": "empiData"
      }
    }
  ]
}
```

### 2. Map Format (Easy Reference)
```json
{
  "step_outputs": {
    "Validate Required HL7 Fields": {
      "fields_validated": 2,
      "validation_status": "passed"
    },
    "Enrich Patient from EMPI": {
      "api_response": {...},
      "enriched_path": "empiData"
    }
  }
}
```

### 3. Transformed Message
```json
{
  "parsed_message": {
    "enhancedSegments": {...},
    "empiData": {...},  // Added by API enrichment step
    "metadata": {...}   // Added by metadata enrichment step
  }
}
```

---

## Referencing Step Outputs

### Syntax Options

#### 1. Step Name Reference (Recommended)
```javascript
// Access step output by step name
step_outputs["Enrich Patient from EMPI"]["api_response"]["name"]
// Returns: "Leanne Graham"
```

#### 2. Step Alias Reference (Coming Soon - V38)
```javascript
// Access step output by alias (shorter, cleaner)
step_outputs["empi"]["api_response"]["name"]
// Same result, cleaner syntax
```

#### 3. Array Index Reference
```javascript
// Access by execution order (less reliable if pipeline changes)
execution_results[1]["output"]["api_response"]["name"]
// Returns: "Leanne Graham"
```

#### 4. Transformed Message Reference
```javascript
// Access enriched data directly from the message
parsed_message["empiData"]["name"]
// Returns: "Leanne Graham"
```

---

## Use Cases

### 1. Conditional Delivery Based on Validation

**Scenario**: Only send messages to downstream system if validation passed.

**Delivery Connector Configuration**:
```json
{
  "connector_type": "http_outbound",
  "enabled": true,
  "condition": {
    "type": "javascript",
    "expression": "step_outputs['Validate Required HL7 Fields']['validation_status'] === 'passed'"
  },
  "endpoint": "https://downstream-system.com/api/messages"
}
```

### 2. Include Validation Results in Delivery

**Scenario**: Send validation metadata along with the transformed message.

**HTTP Outbound Configuration**:
```json
{
  "connector_type": "http_outbound",
  "endpoint": "https://audit-system.com/api/validated-messages",
  "body_template": {
    "message": "{{parsed_message}}",
    "validation": {
      "status": "{{step_outputs['Validate Required HL7 Fields']['validation_status']}}",
      "fields_validated": "{{step_outputs['Validate Required HL7 Fields']['fields_validated']}}"
    },
    "timestamp": "{{metadata.processedAt}}"
  }
}
```

### 3. Enrich Delivery Payload with API Data

**Scenario**: Include EMPI lookup results in the outbound message.

**Message Template**:
```json
{
  "fhirBundle": "{{parsed_message.fhirBundle}}",
  "empiEnrichment": {
    "patientName": "{{step_outputs['Enrich Patient from EMPI']['api_response']['name']}}",
    "email": "{{step_outputs['Enrich Patient from EMPI']['api_response']['email']}}",
    "phone": "{{step_outputs['Enrich Patient from EMPI']['api_response']['phone']}}"
  }
}
```

### 4. Custom Error Handling

**Scenario**: Route failed validations to error queue with detailed field results.

**Error Handler Configuration**:
```json
{
  "on_validation_failure": {
    "connector_type": "rabbitmq_outbound",
    "queue": "validation-errors",
    "message": {
      "original_message": "{{raw_message}}",
      "validation_errors": "{{step_outputs['Validate Required HL7 Fields']['field_results']}}",
      "timestamp": "{{metadata.receivedAt}}"
    }
  }
}
```

### 5. Multi-Step Data Correlation

**Scenario**: Use validation step output in a subsequent custom JavaScript step.

**Custom JavaScript Step**:
```javascript
function transform(input, step_outputs) {
    // Access previous step outputs
    const validationStatus = step_outputs["Validate Required HL7 Fields"]["validation_status"];
    const empiData = step_outputs["Enrich Patient from EMPI"]["api_response"];

    // Add correlation metadata
    input.metadata = input.metadata || {};
    input.metadata.dataQuality = validationStatus === "passed" ? "high" : "low";
    input.metadata.empiVerified = empiData ? true : false;

    return input;
}
```

---

## Step-Specific Output Examples

### Validation Steps (`pre.validation`, `pre.validation.field`)

#### Summary Output (`detailedOutput: false`)
```json
{
  "fields_validated": 2,
  "validation_status": "passed"  // or "warning", "failed"
}
```

#### Detailed Output (`detailedOutput: true`)
```json
{
  "field_results": [
    {
      "field": "MSH.9",
      "validation_type": "required",
      "error_message": "Message Type is required",
      "valid": true
    },
    {
      "field": "PID.3",
      "validation_type": "required",
      "error_message": "Patient ID is required",
      "valid": true
    }
  ],
  "validation_status": "passed"
}
```

**Reference Examples**:
```javascript
// Check if validation passed
step_outputs["Field Validation"]["validation_status"] === "passed"

// Count failed fields (detailed mode only)
step_outputs["Field Validation"]["field_results"].filter(f => !f.valid).length

// Get specific field validation result
step_outputs["Field Validation"]["field_results"].find(f => f.field === "MSH.9")["valid"]
```

### API Enrichment Steps (`pre.enrichment.api`)

```json
{
  "api_endpoint": "https://jsonplaceholder.typicode.com/users/1",
  "http_method": "GET",
  "enriched_path": "empiData",
  "api_response": {
    "id": 1,
    "name": "Leanne Graham",
    "email": "Sincere@april.biz",
    "phone": "1-770-736-8031 x56442",
    "address": {
      "city": "Gwenborough",
      "zipcode": "92998-3874"
    }
  },
  "message": "API enrichment completed"
}
```

**Reference Examples**:
```javascript
// Access enriched data from step output
step_outputs["Enrich Patient from EMPI"]["api_response"]["name"]

// Access enriched data from transformed message (after enrichment)
parsed_message["empiData"]["name"]

// Check if enrichment was successful
step_outputs["Enrich Patient from EMPI"]["api_response"] !== null
```

### Metadata Enrichment Steps (`pre.enrichment.metadata`)

```json
{
  "metadata": {
    "receivedAt": "2025-12-20T04:12:03Z",
    "processedAt": "2025-12-20T04:12:03Z",
    "correlationId": "550e8400-e29b-41d4-a716-446655440000",
    "interfaceId": "629ac1e8-0c50-447a-b93f-ebfc15830a7d"
  },
  "fields_added": 4,
  "field_names": ["receivedAt", "processedAt", "correlationId", "interfaceId"],
  "message": "Added 4 metadata fields"
}
```

**Reference Examples**:
```javascript
// Get correlation ID for logging
step_outputs["Add Metadata"]["metadata"]["correlationId"]

// Check processing time
new Date(step_outputs["Add Metadata"]["metadata"]["processedAt"]) -
new Date(step_outputs["Add Metadata"]["metadata"]["receivedAt"])

// Count metadata fields added
step_outputs["Add Metadata"]["fields_added"]
```

### Core Mapping Steps (`core.mapping`)

```json
{
  "fhirBundle": {
    "resourceType": "Bundle",
    "type": "message",
    "id": "bundle-transform_v3_1766203923115297923",
    "timestamp": "2025-12-20T04:12:03Z",
    "entry": [
      {
        "fullUrl": "urn:uuid:patient-1766203923141403262",
        "resource": {
          "resourceType": "Patient",
          "id": "patient-1766203923141403262",
          "name": [...],
          "gender": "M",
          "birthDate": "19800101"
        }
      }
    ]
  }
}
```

**Reference Examples**:
```javascript
// Access FHIR bundle
step_outputs["HL7 to FHIR Conversion"]["fhirBundle"]

// Count resources in bundle
step_outputs["HL7 to FHIR Conversion"]["fhirBundle"]["entry"].length

// Find specific resource type
step_outputs["HL7 to FHIR Conversion"]["fhirBundle"]["entry"]
  .find(e => e.resource.resourceType === "Patient")
```

---

## Best Practices

### 1. Use Descriptive Step Names
✅ **Good**: `"Enrich Patient from EMPI"`
❌ **Bad**: `"Step 2"` or `"API Call"`

**Reason**: Step outputs are indexed by step name, so descriptive names make references self-documenting.

### 2. Enable Detailed Output Only When Needed
```json
{
  "step_type": "pre.validation",
  "config": {
    "detailedOutput": false  // Default: summary only
  }
}
```

**Reason**: Detailed output includes more data (useful for debugging) but increases storage and processing overhead.

### 3. Reference by Step Name, Not Array Index
✅ **Good**: `step_outputs["Validate Required HL7 Fields"]`
❌ **Bad**: `execution_results[0]["output"]`

**Reason**: Array indices change when you reorder or add steps. Step names are stable.

### 4. Use Step Aliases for Cleaner Code (Coming in V38)
```json
{
  "step_name": "Enrich Patient Demographics from EMPI System",
  "step_alias": "empi",  // Short, memorable alias
  "config": {...}
}
```

**Reference**:
```javascript
// Clean and concise
step_outputs["empi"]["api_response"]

// vs verbose
step_outputs["Enrich Patient Demographics from EMPI System"]["api_response"]
```

### 5. Handle Missing Data Gracefully
```javascript
// Use optional chaining and nullish coalescing
const patientName = step_outputs["Enrich Patient from EMPI"]?.["api_response"]?.["name"] ?? "Unknown";

// Or explicit null checks
if (step_outputs["Enrich Patient from EMPI"] &&
    step_outputs["Enrich Patient from EMPI"]["api_response"]) {
    const name = step_outputs["Enrich Patient from EMPI"]["api_response"]["name"];
}
```

### 6. Document Step Dependencies
When creating custom steps that depend on previous step outputs, document the dependencies:

```json
{
  "step_name": "Generate Patient Summary",
  "step_type": "custom.javascript",
  "dependencies": [
    "Enrich Patient from EMPI",
    "Validate Required HL7 Fields"
  ],
  "config": {
    "script": "function transform(input, step_outputs) { ... }"
  }
}
```

### 7. Use Validation Status for Routing
```javascript
// Route based on validation outcome
const validationPassed = step_outputs["Validate Required HL7 Fields"]["validation_status"] === "passed";

if (validationPassed) {
    // Send to production system
    deliverTo("production-endpoint");
} else {
    // Send to error queue for review
    deliverTo("error-queue");
}
```

---

## Advanced Patterns

### Pattern 1: Aggregate Multiple Step Outputs

**Scenario**: Create a comprehensive audit log combining outputs from multiple steps.

```javascript
function createAuditLog(step_outputs) {
    return {
        validation: step_outputs["Validate Required HL7 Fields"],
        enrichment: {
            empi: step_outputs["Enrich Patient from EMPI"]["api_response"],
            metadata: step_outputs["Add Metadata"]["metadata"]
        },
        transformation: {
            bundleId: step_outputs["HL7 to FHIR Conversion"]["fhirBundle"]["id"],
            resourceCount: step_outputs["HL7 to FHIR Conversion"]["fhirBundle"]["entry"].length
        },
        timestamp: new Date().toISOString()
    };
}
```

### Pattern 2: Conditional Step Execution

**Scenario**: Execute expensive API enrichment only if validation passed.

```javascript
// In step configuration
{
  "step_name": "Enrich from External System",
  "enabled": true,
  "condition": {
    "type": "javascript",
    "expression": "step_outputs['Validate Required HL7 Fields']['validation_status'] === 'passed'"
  }
}
```

### Pattern 3: Dynamic Endpoint Selection

**Scenario**: Route to different endpoints based on validation results.

```javascript
function selectEndpoint(step_outputs) {
    const validationStatus = step_outputs["Validate Required HL7 Fields"]["validation_status"];

    switch(validationStatus) {
        case "passed":
            return "https://production.system.com/api/messages";
        case "warning":
            return "https://staging.system.com/api/review";
        case "failed":
            return "https://error-queue.system.com/api/errors";
        default:
            return "https://fallback.system.com/api/messages";
    }
}
```

### Pattern 4: Build Composite Resources

**Scenario**: Combine FHIR resources with enriched data.

```javascript
function enrichFHIRBundle(parsed_message, step_outputs) {
    const bundle = step_outputs["HL7 to FHIR Conversion"]["fhirBundle"];
    const empiData = step_outputs["Enrich Patient from EMPI"]["api_response"];

    // Find Patient resource
    const patientEntry = bundle.entry.find(e => e.resource.resourceType === "Patient");

    if (patientEntry && empiData) {
        // Add EMPI data as extension
        patientEntry.resource.extension = patientEntry.resource.extension || [];
        patientEntry.resource.extension.push({
            url: "http://example.org/fhir/StructureDefinition/empi-data",
            valueString: JSON.stringify(empiData)
        });
    }

    return bundle;
}
```

---

## Troubleshooting

### Issue 1: Step Output Not Found

**Error**: `Cannot read property 'api_response' of undefined`

**Solution**: Check step name spelling and ensure the step has executed:
```javascript
// Debug: List all available step outputs
console.log(Object.keys(step_outputs));

// Safe access with fallback
const response = step_outputs["Enrich Patient from EMPI"]?.["api_response"] ?? null;
```

### Issue 2: Empty Output Data

**Problem**: Step output exists but contains no data.

**Check**:
1. Verify step executed successfully: `execution_results[i]["success"] === true`
2. Check step-specific configuration (e.g., `detailedOutput` for validation)
3. Review step execution logs for errors

### Issue 3: Step Output vs Transformed Message Confusion

**Key Difference**:
- **Step Output**: Metadata about the step execution (validation status, API response, etc.)
- **Transformed Message**: The actual message data being transformed (parsed HL7, FHIR bundle, etc.)

```javascript
// Step output: Execution metadata
step_outputs["Enrich Patient from EMPI"]["api_response"]

// Transformed message: Enriched data added to message
parsed_message["empiData"]

// Both contain the same enrichment data, but organized differently
```

---

## Future Enhancements (Roadmap)

### V38: Step Aliases
- **Feature**: Assign short aliases to steps for cleaner references
- **Syntax**: `step_outputs["empi"]` instead of `step_outputs["Enrich Patient from EMPI"]`
- **Status**: Database schema ready (V36), UI pending

### V39: Step Output Namespaces
- **Feature**: Automatically namespaced outputs to prevent collisions
- **Example**: `empi_b4c9f1` (alias + short ID)
- **Benefit**: Support multiple instances of same step type

### V40: Step Output Caching
- **Feature**: Cache expensive step outputs for reuse across retries
- **Use Case**: Avoid re-calling external APIs on pipeline retry

### V41: Step Output Schema Validation
- **Feature**: Define expected output schema for steps
- **Benefit**: Catch missing/malformed outputs at configuration time

---

## Summary

**Key Takeaways**:
1. Use `step_outputs` map for easy reference by step name
2. Use `execution_results` array when execution order matters
3. Enable `detailedOutput` on validation steps for field-level results
4. Reference step outputs in delivery configurations, custom scripts, and conditional logic
5. Use descriptive step names for self-documenting pipelines
6. Handle missing data gracefully with optional chaining

**Next Steps**:
- Review [TRANSFORMATION_PIPELINE_DESIGN.md](TRANSFORMATION_PIPELINE_DESIGN.md) for pipeline architecture
- Check [SYSTEM_DOCUMENTATION.md](SYSTEM_DOCUMENTATION.md) for API endpoints
- See [CONNECTIVITY_CATALOG.md](CONNECTIVITY_CATALOG.md) for delivery connector options

---

**Document Version**: 1.0
**Last Updated**: 2025-12-20
**Related Migrations**: V36 (Step Alias), V34 (Step Output Tracking)
