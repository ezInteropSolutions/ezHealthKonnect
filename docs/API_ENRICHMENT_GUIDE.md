# API Enrichment Executor - Complete Guide

## Overview

The **API Enrichment Executor** enriches healthcare messages by querying external REST APIs during pipeline processing. This enables real-time data augmentation from:

- **EMPI (Enterprise Master Patient Index)** - Complete patient demographics
- **EHR Systems** (Epic, Cerner, Allscripts) - Clinical data, allergies, medications
- **LIMS (Laboratory Information Management Systems)** - Reference ranges, interpretations
- **Payer APIs** - Insurance eligibility, coverage verification
- **NPI Registry** - Provider credentials, specialties
- **Drug Databases** (RxNorm, First Databank) - Drug interactions, alternatives

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Configuration Reference](#configuration-reference)
3. [Real-World Examples](#real-world-examples)
4. [Authentication Methods](#authentication-methods)
5. [Error Handling](#error-handling)
6. [Performance Optimization](#performance-optimization)
7. [Troubleshooting](#troubleshooting)

---

## Quick Start

### Basic GET Request

```json
{
  "endpoint": "https://api.hospital.org/patients/{patientId}",
  "method": "GET",
  "fieldMappings": {
    "patientId": "enhancedSegments.PID.fields[2].value"
  },
  "targetPath": "enriched.patient"
}
```

**What it does:**
1. Extracts patient ID from HL7 message (PID.3 field)
2. Calls `GET https://api.hospital.org/patients/MRN-12345`
3. Stores API response at `inputData.enriched.patient`

### Example Pipeline Flow

```
HL7 Message → Parse → [API Enrichment] → HL7→FHIR Mapping → Deliver
                            ↓
                  Fetch from EMPI API
                  Add demographics
```

---

## Configuration Reference

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `endpoint` | string | API URL (supports `{fieldName}` placeholders) |

### Optional Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `method` | string | `"GET"` | HTTP method (GET, POST, PUT, PATCH, DELETE) |
| `targetPath` | string | `"enriched.api"` | Where to store API response in message |
| `timeoutMs` | integer | `5000` | Request timeout in milliseconds |
| `retryCount` | integer | `0` | Number of retry attempts on failure |
| `retryDelayMs` | integer | `1000` | Delay between retries (ms) |
| `failOnError` | boolean | `false` | Stop pipeline if API fails |
| `defaultValue` | any | `null` | Fallback value if API fails |

### Authentication

| Field | Type | Description |
|-------|------|-------------|
| `authType` | string | Authentication type: `"none"`, `"basic"`, `"bearer"`, `"apikey"` |
| `username` | string | Username for Basic auth |
| `password` | string | Password for Basic auth |
| `bearerToken` | string | Token for Bearer auth |
| `apiKey` | string | API key (sent as `X-API-Key` header) |

### Request Configuration

| Field | Type | Description |
|-------|------|-------------|
| `headers` | object | Custom HTTP headers |
| `queryParams` | object | URL query parameters |
| `requestBody` | object | Request body for POST/PUT/PATCH |
| `fieldMappings` | object | Map placeholders to message fields |

---

## Real-World Examples

### 1. EMPI Patient Lookup (Epic Interconnect)

**Scenario:** Enrich minimal ED registration with complete demographics from Epic

```json
{
  "endpoint": "https://epic-fhir.hospital.org/api/FHIR/R4/Patient/{patientId}",
  "method": "GET",
  "authType": "bearer",
  "bearerToken": "{{env.EPIC_API_TOKEN}}",
  "headers": {
    "Accept": "application/fhir+json",
    "Epic-Client-ID": "integration-engine"
  },
  "fieldMappings": {
    "patientId": "enhancedSegments.PID.fields[2].value"
  },
  "targetPath": "enriched.demographics",
  "timeoutMs": 3000,
  "retryCount": 2,
  "failOnError": false,
  "defaultValue": {
    "status": "not_found",
    "source": "empi"
  }
}
```

**API Response (Epic FHIR Patient):**
```json
{
  "resourceType": "Patient",
  "id": "12345",
  "name": [{
    "use": "official",
    "text": "John Robert Doe",
    "family": "Doe",
    "given": ["John", "Robert"]
  }],
  "telecom": [
    {"system": "phone", "value": "555-1234", "use": "mobile"},
    {"system": "email", "value": "john.doe@email.com"}
  ],
  "address": [{
    "line": ["123 Main Street", "Apt 4B"],
    "city": "Springfield",
    "state": "IL",
    "postalCode": "62701"
  }],
  "extension": [
    {
      "url": "http://hospital.org/insurance-id",
      "valueString": "INS-999-8888"
    }
  ]
}
```

**Stored at:** `inputData.enriched.demographics`

---

### 2. NPI Provider Lookup (NPPES API)

**Scenario:** Verify provider credentials and specialty

```json
{
  "endpoint": "https://npiregistry.cms.hhs.gov/api/?number={npi}&version=2.1",
  "method": "GET",
  "fieldMappings": {
    "npi": "enhancedSegments.PV1.fields[6].value"
  },
  "targetPath": "enriched.provider",
  "timeoutMs": 5000,
  "retryCount": 1
}
```

**Use Case:** Automatically validate provider NPI and add full credentials to FHIR Practitioner resource

---

### 3. Insurance Eligibility Check (Payer API)

**Scenario:** Real-time eligibility verification (270/271 transaction)

```json
{
  "endpoint": "https://payer-api.insurance.com/eligibility/check",
  "method": "POST",
  "authType": "basic",
  "username": "hospital-client-001",
  "password": "{{env.PAYER_API_SECRET}}",
  "headers": {
    "Content-Type": "application/json"
  },
  "requestBody": {
    "subscriberId": "{insuranceId}",
    "providerNPI": "{providerNPI}",
    "serviceType": "30",
    "dateOfService": "{serviceDate}"
  },
  "fieldMappings": {
    "insuranceId": "enhancedSegments.IN1.fields[1].value",
    "providerNPI": "enhancedSegments.PV1.fields[6].value",
    "serviceDate": "metadata.receivedAt"
  },
  "targetPath": "enriched.eligibility",
  "timeoutMs": 10000,
  "failOnError": false
}
```

**API Response:**
```json
{
  "eligible": true,
  "coverage": {
    "plan": "Blue Cross PPO",
    "copay": 25.00,
    "deductible": 500.00,
    "deductibleMet": 150.00
  },
  "requires_preauth": false
}
```

---

### 4. Lab Reference Range Lookup (LIMS)

**Scenario:** Add normal ranges and clinical interpretation to lab results

```json
{
  "endpoint": "https://lims.hospital.org/reference-ranges/{testCode}",
  "method": "GET",
  "authType": "apikey",
  "apiKey": "{{env.LIMS_API_KEY}}",
  "headers": {
    "Accept": "application/json"
  },
  "fieldMappings": {
    "testCode": "enhancedSegments.OBX.fields[2].components[0].value"
  },
  "targetPath": "enriched.labReference",
  "timeoutMs": 2000
}
```

**API Response:**
```json
{
  "testCode": "GLU",
  "testName": "Glucose, Serum",
  "normalRange": {
    "low": 70,
    "high": 100,
    "unit": "mg/dL"
  },
  "criticalLow": 50,
  "criticalHigh": 400,
  "interpretation": {
    "180": "HIGH - Possible hyperglycemia"
  }
}
```

---

### 5. Drug Interaction Check (RxNorm/First Databank)

**Scenario:** Check for drug-drug interactions before processing pharmacy order

```json
{
  "endpoint": "https://api.firstdatabank.com/interactions/check",
  "method": "POST",
  "authType": "bearer",
  "bearerToken": "{{env.FDB_TOKEN}}",
  "headers": {
    "Content-Type": "application/json"
  },
  "requestBody": {
    "drugCode": "{drugCode}",
    "patientMedications": "{currentMeds}"
  },
  "fieldMappings": {
    "drugCode": "enhancedSegments.RXE.fields[1].value",
    "currentMeds": "enriched.patient.activeMedications"
  },
  "targetPath": "enriched.interactions",
  "timeoutMs": 3000,
  "failOnError": true
}
```

---

## Authentication Methods

### 1. No Authentication

```json
{
  "endpoint": "https://public-api.example.com/data",
  "authType": "none"
}
```

### 2. Basic Authentication

```json
{
  "endpoint": "https://api.example.com/secure",
  "authType": "basic",
  "username": "integration-user",
  "password": "{{env.API_PASSWORD}}"
}
```

**Sends:** `Authorization: Basic aW50ZWdyYXRpb24tdXNlcjpwYXNzd29yZA==`

### 3. Bearer Token (OAuth 2.0)

```json
{
  "endpoint": "https://api.example.com/fhir/Patient/123",
  "authType": "bearer",
  "bearerToken": "{{env.OAUTH_TOKEN}}"
}
```

**Sends:** `Authorization: Bearer eyJhbGciOiJSUzI1NiIs...`

### 4. API Key

```json
{
  "endpoint": "https://api.example.com/data",
  "authType": "apikey",
  "apiKey": "{{env.API_KEY}}"
}
```

**Sends:** `X-API-Key: your-api-key-here`

### Security Best Practices

1. **Never hardcode credentials** - Use environment variables: `{{env.VARIABLE_NAME}}`
2. **Use HTTPS only** - Never send credentials over unencrypted connections
3. **Rotate tokens regularly** - Implement token refresh mechanisms
4. **Least privilege** - Use service accounts with minimum required permissions

---

## Error Handling

### Strategy 1: Continue on Failure (Default)

**Best for:** Optional enrichment, non-critical data

```json
{
  "endpoint": "https://api.example.com/optional",
  "failOnError": false
}
```

**Behavior:**
- API failure → Pipeline continues
- Enriched field not set
- Error logged for debugging

### Strategy 2: Use Default Value

**Best for:** Providing fallback data

```json
{
  "endpoint": "https://api.example.com/patient/{id}",
  "failOnError": false,
  "defaultValue": {
    "status": "unknown",
    "source": "unavailable"
  }
}
```

**Behavior:**
- API failure → `defaultValue` used
- Pipeline continues
- Downstream steps can check `status` field

### Strategy 3: Fail Pipeline

**Best for:** Critical enrichment (insurance, eligibility)

```json
{
  "endpoint": "https://payer.com/eligibility",
  "failOnError": true
}
```

**Behavior:**
- API failure → Pipeline stops
- Error returned to sender
- Message not processed further

### Retry Configuration

```json
{
  "endpoint": "https://flaky-api.example.com/data",
  "retryCount": 3,
  "retryDelayMs": 2000,
  "timeoutMs": 5000
}
```

**Behavior:**
1. Initial attempt (5s timeout)
2. Wait 2 seconds
3. Retry attempt 1 (5s timeout)
4. Wait 2 seconds
5. Retry attempt 2 (5s timeout)
6. Wait 2 seconds
7. Retry attempt 3 (5s timeout)
8. If all fail → Use `failOnError` strategy

---

## Performance Optimization

### 1. Set Appropriate Timeouts

```json
{
  "endpoint": "https://fast-cache.example.com/data",
  "timeoutMs": 1000  // 1 second for cached responses
}
```

```json
{
  "endpoint": "https://slow-legacy-system.example.com/data",
  "timeoutMs": 30000  // 30 seconds for legacy systems
}
```

### 2. Use Parallel Enrichment

**Multiple API calls in same layer execute in parallel:**

```
Pre-Processing Layer:
  ├─ API Enrichment (EMPI) ────┐
  ├─ API Enrichment (LIMS) ────┼─→ Execute in parallel
  └─ API Enrichment (Payer) ───┘
       ↓
  Total time = max(EMPI, LIMS, Payer) ≈ 2s
  Instead of sequential = EMPI + LIMS + Payer ≈ 6s
```

### 3. Cache External API Responses

**Use cache enrichment executor for frequently accessed data:**

```json
{
  "type": "pre.enrichment.cache",
  "config": {
    "cacheKey": "provider-{npi}",
    "ttlSeconds": 3600,
    "fallback": {
      "type": "pre.enrichment.api",
      "config": {
        "endpoint": "https://npi-registry.com/api/{npi}"
      }
    }
  }
}
```

**Result:**
- First request: API call (300ms) + cache store
- Subsequent requests: Cache hit (5ms)
- 60x faster for cached data!

### 4. Minimize Payload Size

**Request only needed fields:**

```json
{
  "endpoint": "https://api.example.com/patients/{id}?fields=name,dob,address"
}
```

---

## Troubleshooting

### Issue: "API request failed: timeout"

**Causes:**
- Slow API response time
- Network latency
- Timeout too short

**Solutions:**
1. Increase `timeoutMs`: `"timeoutMs": 10000`
2. Add retries: `"retryCount": 2`
3. Check API performance with tools like Postman
4. Contact API provider about SLA

### Issue: "Authentication failed (401)"

**Causes:**
- Invalid credentials
- Expired token
- Missing authentication header

**Solutions:**
1. Verify credentials in environment variables
2. Check token expiration: `jwt.io` for JWT tokens
3. Test authentication with cURL:
   ```bash
   curl -H "Authorization: Bearer YOUR_TOKEN" https://api.example.com/test
   ```
4. Check API documentation for required headers

### Issue: "API returned 404 Not Found"

**Causes:**
- Incorrect endpoint URL
- Field mapping extracted wrong ID
- Resource doesn't exist in API

**Solutions:**
1. Verify endpoint URL in API docs
2. Check field mapping path:
   ```json
   "fieldMappings": {
     "patientId": "enhancedSegments.PID.fields[2].value"
   }
   ```
3. Add logging to see extracted value
4. Use `defaultValue` to handle missing resources

### Issue: "Field mapping returns empty value"

**Causes:**
- Incorrect JSONPath
- Field doesn't exist in message
- Array index out of bounds

**Solutions:**
1. Test field path in pipeline test UI
2. Check HL7 message structure
3. Use validation step first to ensure required fields exist
4. Add fallback in field mapping:
   ```json
   "fieldMappings": {
     "patientId": "enhancedSegments.PID.fields[2].value || enhancedSegments.PID.fields[1].value"
   }
   ```

### Debug Logging

**Enable debug logs in Go backend:**

```bash
docker-compose logs -f app | grep "API Enrichment"
```

**Look for:**
- `🌐 [API Enrichment] Calling API: GET https://...`
- `🔐 Added Bearer token authentication`
- `✅ API call successful (status: 200)`
- `⚠️ API call failed, using default value`

---

## Complete Example: Multi-Step Enrichment Pipeline

### Scenario: ED Patient Registration

**Input:** Minimal HL7 ADT^A04 from triage system

**Pipeline:**

```yaml
Pre-Processing Layer:
  1. Field Validation
     - Ensure PID.3 (MRN) exists
     - Ensure PV1.2 (patient class) = "E" (emergency)

  2. API Enrichment (EMPI) - Parallel
     - Get complete demographics
     - Add insurance info
     - Add emergency contacts

  3. API Enrichment (Clinical) - Parallel
     - Get active allergies
     - Get current medications
     - Get advance directives

  4. API Enrichment (Eligibility) - Parallel
     - Check insurance coverage
     - Verify copay amount
     - Check pre-auth requirements

Core Processing Layer:
  5. HL7→FHIR Mapping
     - Use enriched data for complete FHIR Bundle
     - Patient + Coverage + AllergyIntolerance + MedicationStatement

Post-Processing Layer:
  6. FHIR Validation
     - Ensure bundle is valid

  7. Deliver to Epic
```

**Result:** Complete patient record with clinical context sent to EMR in <3 seconds

---

## API Enrichment vs Database Enrichment

| Feature | API Enrichment | Database Enrichment |
|---------|----------------|---------------------|
| **Data Source** | External REST APIs | PostgreSQL/MySQL/SQL Server |
| **Latency** | 50-500ms (network) | 5-50ms (local) |
| **Use Cases** | Real-time external data, cloud services | Local lookups, cached data |
| **Caching** | External (Redis recommended) | Built-in query cache |
| **Authentication** | OAuth, API keys, Basic | Database credentials |
| **Best For** | EMPI, payer APIs, NPI lookup | Reference data, codes, mappings |

**Recommendation:** Use **database enrichment** for frequently accessed reference data (ICD codes, drug codes), use **API enrichment** for dynamic external data (patient demographics, eligibility).

---

## Next Steps

1. ✅ **Test with mock API** - Use httptest in Go tests
2. ✅ **Add to pipeline** - Drag "API Enrichment" from toolbox
3. 📝 **Configure authentication** - Add environment variables
4. 🧪 **Test in UI** - Use pipeline test feature
5. 📊 **Monitor performance** - Check execution times in logs
6. 🚀 **Deploy to production** - Ensure API credentials are secure

---

## Related Documentation

- [Database Enrichment Guide](DATABASE_ENRICHMENT_GUIDE.md)
- [Cache Enrichment Guide](CACHE_ENRICHMENT_GUIDE.md)
- [Script Enrichment Guide](SCRIPT_ENRICHMENT_GUIDE.md)
- [Pipeline Builder User Guide](PIPELINE_BUILDER_GUIDE.md)
- [Error Handling Strategies](ERROR_HANDLING_GUIDE.md)

---

**Version:** 1.0.0
**Last Updated:** December 2025
**Maintainer:** ezHealthKonnect Integration Team
