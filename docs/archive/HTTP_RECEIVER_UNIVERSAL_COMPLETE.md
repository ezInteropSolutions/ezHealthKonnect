# Universal HTTP Receiver Configuration - Complete ✅

## What Was Built

You're absolutely right - we had HTTP listener infrastructure and needed to make it **universal** for any HTTP/REST endpoint, not just FHIR-specific.

### The Key Insight

There are now **TWO types** of HTTP receivers:

1. **FHIR Receiver** (`sourceType='fhir'` + `connectivity='http'`)
   - FHIR-specific features (resource types, REST operations, FHIR validation)
   - For receiving FHIR resources from EHR systems

2. **HTTP Receiver** (`sourceType='hl7v2|json|xml|etc.'` + `connectivity='http'`)  ✨ **NEW**
   - Universal HTTP listener for ANY payload
   - For HL7 messages, JSON, XML, webhooks, custom formats
   - Configurable context paths and authentication

## Configuration Comparison

### HTTP Receiver (Universal) - NEW

```
🌐 HTTP/REST Receiver Configuration
ℹ️ Universal HTTP Receiver
Accept any HTTP payload: HL7 messages, JSON, XML, custom formats, webhooks, etc.

🎧 Listener Configuration
├─ Listen Port: [8082]  (Where ezHealthKonnect listens)
├─ Listen Host: [0.0.0.0]
└─ Preview: http://your-server:8082/api/receive

📍 Context Path: [/api/receive]
   Examples: /api/receive, /hl7/inbound, /webhook, /integration/epic

🔐 Allowed HTTP Methods:
├─ ☑ POST (Most Common)
├─ ☐ PUT
├─ ☐ GET
└─ ☐ PATCH

📦 Expected Content Type:
├─ Any (Accept All) ← Default
├─ application/json
├─ application/xml
├─ text/plain (HL7 v2)
└─ application/hl7-v2

🔐 HTTP Authentication:
├─ No Authentication (Development)
├─ API Key (Header)
├─ Basic Authentication
├─ Bearer Token
├─ OAuth 2.0
└─ Mutual TLS (Certificate)

📨 Response Configuration:
├─ Return correlation ID in headers (X-Correlation-ID)
└─ Return message ID in response

⚙️ Advanced Settings:
├─ Max Payload Size (MB): [10]
└─ Timeout (seconds): [30]
```

### FHIR Receiver (Specialized)

```
🏥 FHIR Receiver Configuration

🎧 Listener Configuration
├─ Listen Port: [8082]
└─ Base URL Path: [/fhir/r4]

🔹 FHIR Version: [R4]

✅ Supported REST Operations:
├─ CREATE (POST)
├─ READ (GET)
├─ UPDATE (PUT)
├─ PATCH
├─ DELETE
├─ SEARCH
└─ BATCH/TRANSACTION

🔐 HTTP Authentication: (same as universal)

🎯 Resource Filtering: (Patient, Observation, etc.)
✅ Validation Settings
📊 Post-Reception Actions
```

## Use Cases

### Use Case 1: HL7 v2 Messages over HTTP

**Scenario**: Epic sends HL7 ADT messages via HTTP POST

**Configuration**:
```
Source Type: HL7 v2.x
Source Connectivity: HTTP/REST
Listen Port: 8082
Context Path: /hl7/inbound
Allowed Methods: POST
Content Type: text/plain (HL7 v2)
Authentication: API Key
```

**External System Sends To**:
```
POST http://your-ezhealthkonnect:8082/hl7/inbound
Content-Type: text/plain
X-API-Key: abc123...

MSH|^~\&|EPIC|HOSPITAL|...
PID|||123456...
```

### Use Case 2: JSON Webhooks

**Scenario**: External system sends JSON events via webhook

**Configuration**:
```
Source Type: JSON
Source Connectivity: HTTP/REST
Listen Port: 8083
Context Path: /webhook/events
Allowed Methods: POST
Content Type: application/json
Authentication: Bearer Token
```

**External System Sends To**:
```
POST http://your-ezhealthkonnect:8083/webhook/events
Content-Type: application/json
Authorization: Bearer eyJhbGc...

{
  "event": "patient.admitted",
  "patient_id": "12345",
  "timestamp": "2025-10-28T12:00:00Z"
}
```

### Use Case 3: Custom XML Integration

**Scenario**: Legacy system sends custom XML format

**Configuration**:
```
Source Type: XML
Source Connectivity: HTTP/REST
Listen Port: 8084
Context Path: /integration/legacy
Allowed Methods: POST, PUT
Content Type: application/xml
Authentication: Basic Auth
```

**External System Sends To**:
```
POST http://your-ezhealthkonnect:8084/integration/legacy
Content-Type: application/xml
Authorization: Basic dXNlcjpwYXNz

<PatientUpdate>
  <Id>12345</Id>
  <Name>John Doe</Name>
  ...
</PatientUpdate>
```

### Use Case 4: FHIR Receiver (Specialized)

**Scenario**: Cerner sends FHIR resources

**Configuration**:
```
Source Type: FHIR
Source Connectivity: HTTP/REST
Listen Port: 8082
Base Path: /fhir/r4
FHIR Version: R4
Operations: CREATE, READ, SEARCH
Authentication: OAuth 2.0
```

**External System Sends To**:
```
POST http://your-ezhealthkonnect:8082/fhir/r4/Patient
Content-Type: application/fhir+json
Authorization: Bearer ...

{
  "resourceType": "Patient",
  "name": [{"given": ["John"], "family": "Doe"}],
  ...
}
```

## How to Configure

### Step 1: Choose Source Type

In the wizard, when you select:
- **HL7 v2.x** → Universal HTTP Receiver (with HL7-specific hints)
- **JSON** → Universal HTTP Receiver (accepts JSON)
- **XML** → Universal HTTP Receiver (accepts XML)
- **FHIR** → FHIR Receiver (FHIR-specific features)

### Step 2: Choose Connectivity

- **TCP/MLLP** → TCP listener (existing)
- **HTTP/REST** → Shows appropriate receiver config based on source type

### Step 3: Configure Listener

**For Universal HTTP Receiver**:
```
Listen Port: 8082 (or any available port)
Context Path: /api/receive (or your custom path)
Allowed Methods: POST (most common)
Content Type: Any (or specific type)
Authentication: Choose based on security requirements
```

**For FHIR Receiver**:
```
Listen Port: 8082
Base Path: /fhir/r4
FHIR Version: R4
Operations: CREATE, READ, UPDATE, SEARCH
Authentication: OAuth 2.0 (recommended for FHIR)
Resources: Patient, Observation, Encounter
```

## Architecture

### Routing Logic

```javascript
// InterfaceConfigComponents.js line 91-98
case 'http':
    const isFhirReceiver = (sourceType === 'fhir');
    if (isFhirReceiver) {
        return this.getFhirReceiverConfig(config, idPrefix);
    } else {
        // Universal HTTP Receiver (for HL7, JSON, XML, custom payloads)
        return this.getHttpReceiverConfig(config, idPrefix);
    }
```

### Data Storage

Both configurations store to `source_config` JSONB column:

**Universal HTTP Receiver**:
```json
{
  "port": 8082,
  "host": "0.0.0.0",
  "contextPath": "/api/receive",
  "allowedMethods": ["POST"],
  "contentType": "any",
  "authType": "api_key",
  "apiKeyHeader": "X-API-Key",
  "apiKeyValue": "...",
  "returnCorrelationId": true,
  "returnMessageId": true,
  "maxPayloadSize": 10,
  "timeout": 30
}
```

**FHIR Receiver**:
```json
{
  "port": 8082,
  "host": "0.0.0.0",
  "basePath": "/fhir/r4",
  "fhirVersion": "R4",
  "operations": ["CREATE", "READ", "UPDATE", "SEARCH"],
  "contentType": "application/fhir+json",
  "authType": "oauth2",
  "oauthIssuer": "https://auth.hospital.com",
  "acceptedResources": ["Patient", "Observation"],
  "validateStructure": true
}
```

## Backend Implementation Status

### ✅ What's Ready

1. **UI Configuration** - Complete
   - Universal HTTP Receiver form
   - FHIR Receiver form
   - Port validation and hints
   - Authentication configuration

2. **Reference Implementation** - Available
   - `http_input_connector.go.reference` exists
   - Shows how to create HTTP listener
   - HTTP server, routing, request handling

### ⏳ What Needs to Be Built

1. **Restore HTTP Input Connector** (Week 1)
   - Copy reference file to active location
   - Modernize for universal use (not just FHIR)
   - Add support for different content types
   - Implement authentication middleware

2. **Dynamic Server Manager** (Week 2)
   - Start/stop HTTP servers per interface
   - Port management and conflict detection
   - Integration with interface lifecycle

3. **Request Handling** (Week 3)
   - Route requests to correct interface handler
   - Authentication validation
   - Payload parsing (HL7, JSON, XML)
   - Store in database + MongoDB

## Testing Instructions

### Test 1: Universal HTTP Receiver (HL7 v2 via HTTP)

1. **Open Wizard**: http://localhost:3000/interfaces.html → "Create New Interface"
2. **Step 1 - Source Configuration**:
   - Source Type: **HL7 v2.x**
   - Source Connectivity: **HTTP/REST**
3. **Verify Form Shows**:
   - ✅ "🌐 HTTP/REST Receiver Configuration" header
   - ✅ "ℹ️ Universal HTTP Receiver" banner
   - ✅ Listen Port field (default: 8082)
   - ✅ Context Path field (default: /api/receive)
   - ✅ Allowed HTTP Methods checkboxes
   - ✅ Content Type dropdown (with HL7 options)
   - ✅ HTTP Authentication section
   - ✅ Response Configuration
   - ✅ Advanced Settings
4. **Configure**:
   - Listen Port: 8082
   - Context Path: /hl7/inbound
   - Methods: POST
   - Content Type: text/plain (HL7 v2)
   - Auth: API Key

### Test 2: FHIR Receiver (Specialized)

1. **Open Wizard**: Create new interface
2. **Step 1**:
   - Source Type: **FHIR**
   - Source Connectivity: **HTTP/REST**
3. **Verify Form Shows**:
   - ✅ "🏥 FHIR Receiver Configuration" header (different from universal)
   - ✅ FHIR Version dropdown
   - ✅ REST Operations checkboxes
   - ✅ Resource Filtering
   - ✅ Validation Settings
   - ✅ Post-Reception Actions

### Test 3: Dynamic Form Switching

1. **Start with**: Source Type = HL7 v2, Connectivity = HTTP
   - Should show **Universal HTTP Receiver** (context path, methods)
2. **Change to**: Source Type = FHIR
   - Should instantly update to **FHIR Receiver** (FHIR version, operations, resources)
3. **Change to**: Source Type = JSON
   - Should show **Universal HTTP Receiver** again (with JSON hints)

## Documentation Reference

- **Architecture**: [DYNAMIC_PORT_LISTENERS_DESIGN.md](DYNAMIC_PORT_LISTENERS_DESIGN.md)
- **HTTP Input Connector**: [processing/http_input_connector.go.reference](processing/http_input_connector.go.reference)
- **Shared Components**: [public/js/components/InterfaceConfigComponents.js:475-628](public/js/components/InterfaceConfigComponents.js#L475-L628)

## Next Steps

**Immediate** (This Release):
- ✅ Universal HTTP Receiver UI complete
- ✅ FHIR Receiver UI complete
- ✅ Configuration forms working
- ✅ Port validation and hints

**Phase 1** (Next 1-2 weeks):
- ⏳ Restore HTTP input connector from reference
- ⏳ Make it universal (not just FHIR)
- ⏳ Add content-type routing
- ⏳ Implement authentication middleware

**Phase 2** (3-4 weeks):
- ⏳ Dynamic server manager
- ⏳ Start/stop listeners on interface activation
- ⏳ Port conflict detection
- ⏳ Integration with processing engine

**Phase 3** (5-6 weeks):
- ⏳ Request handling per interface
- ⏳ Payload parsing (HL7, JSON, XML)
- ⏳ Storage and transformation trigger
- ⏳ End-to-end testing

## Summary

✅ **Universal HTTP Receiver Configuration - COMPLETE**

You can now configure ezHealthKonnect to listen on custom ports with custom context paths for:
- HL7 v2 messages over HTTP
- JSON webhooks
- XML integrations
- Custom formats
- FHIR resources (specialized config)

The UI is complete with:
- Clear port/host configuration
- Context path customization
- HTTP method selection
- Content type filtering
- Full authentication support
- Response configuration
- Advanced settings

**Next step**: Implement the backend HTTP input connector to actually start listening on these configured ports!

---

**Status**: ✅ **UI COMPLETE - Backend Implementation Ready to Start**
**Priority**: 🔴 **High** - Core integration feature
**Impact**: 🎯 **High** - Enables universal HTTP-based integration patterns
