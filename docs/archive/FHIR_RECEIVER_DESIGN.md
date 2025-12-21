# FHIR Receiver Interface - Design Document

## Vision
Create a FHIR receiver interface that completes the HL7 → FHIR → FHIR Receiver integration flow, enabling ezHealthKonnect to act as a full integration engine.

---

## Current Status
- ✅ HL7 message ingestion via TCP/MLLP
- ✅ HL7 → FHIR transformation pipeline
- ✅ FHIR bundle output stored in MongoDB
- ❌ **MISSING:** FHIR receiver to accept transformed FHIR data

---

## Requirements

### Functional Requirements
1. **Accept all FHIR RESTful operations** (Read, Create, Update, Delete, Search, Batch)
2. **Support HTTP and HTTPS** protocols
3. **FHIR R4 compatibility** (design for easy version extension)
4. **Support all resource types** (Patient, Observation, Encounter, etc.)
5. **Multiple authentication modes** (None, Basic, Bearer, OAuth, mTLS)
6. **OOB (Out-of-Box) principle:** Works immediately with minimal configuration
7. **MVC architecture:** Clean separation of concerns

### Non-Functional Requirements
- Performance: Handle 100+ requests/second
- Scalability: Support multiple concurrent interfaces
- Security: HIPAA-compliant audit logging
- Extensibility: Easy to add FHIR R5/R6 support

---

## MVP Scope (Current Phase)

### Goal: Complete HL7 → FHIR → FHIR Receiver Flow

**Simplified MVP:**
- FHIR receiver accepts **POST requests only** (Create operation)
- **HTTP only** (HTTPS in phase 2)
- **No authentication** (add incrementally)
- **All R4 resource types** accepted
- **Lenient validation** (log warnings, don't reject)
- **Store in MongoDB** + trigger processing pipeline

### Use Case Flow
```
1. HL7 message arrives → TCP listener
2. Parse HL7 → Enhanced JSON
3. Transform → FHIR R4 Bundle
4. Store FHIR Bundle → MongoDB (transformed_messages_*)
5. Forward FHIR Bundle → FHIR Receiver (HTTP POST)
6. FHIR Receiver validates → Stores → Returns OperationOutcome
```

---

## Wizard Design (FHIR Receiver)

### Step 1: Interface Configuration (Enhanced)
**Existing Fields:**
- Interface Name (required)
- Source Type: **FHIR**
- Target Type: (Database, Custom, HL7, etc.)
- Connectivity: **HTTP**

**New FHIR-Specific Fields:**
```javascript
{
  "fhir_version": "R4",          // Dropdown: R4 (default), R5, DSTU2
  "protocol": "http",             // Toggle: http/https
  "port": 8090,                   // Number input
  "base_path": "/fhir/r4"        // Text input (default)
}
```

**Generated Endpoint:**
```
http://localhost:8090/fhir/r4
```

### Step 2: FHIR Endpoint Configuration (MVP Simplified)
**Operation Support:**
- [x] **Create (POST)** - Enabled by default
- [ ] Read, Update, Delete, Search - Future phases

**Resource Types:**
- [x] **All R4 resource types** - Enabled by default
- Specific resource filtering - Future phase

**Authentication:**
- [x] **None** - MVP default
- Future: Basic, Bearer, OAuth, mTLS

**Storage:**
```json
{
  "supported_operations": ["create"],
  "resource_types": {"allow_all": true},
  "auth_config": {"mode": "none"}
}
```

### Step 3: Processing Configuration (MVP Simplified)
**Validation Mode:**
- **Lenient** (default) - Accept with warnings
- Future: Strict, None

**Processing Actions:**
- [x] Store in MongoDB
- [x] Trigger processing pipeline
- [x] Generate audit logs
- [x] Send OperationOutcome response

### Step 4: Target Mapping
- For FHIR receiver, this might be minimal (pass-through)
- Or map FHIR → internal format if target is database/HL7

### Step 5: Review & Activate
- Display configuration summary
- **Test Endpoint** button (send sample FHIR resource)
- **Activate** button (start HTTP listener)

---

## Technical Architecture

### Database Schema

#### interfaces table - source_config (JSON column)
```json
{
  "type": "fhir",
  "fhir_version": "R4",
  "protocol": "http",
  "port": 8090,
  "base_path": "/fhir/r4",
  "supported_operations": ["create"],
  "resource_types": {
    "allow_all": true
  },
  "auth_config": {
    "mode": "none"
  },
  "validation_mode": "lenient",
  "processing_options": {
    "store_mongodb": true,
    "trigger_pipeline": true,
    "audit_logging": true,
    "send_response": true
  }
}
```

#### MongoDB Collections
```
received_fhir_resources_intf_{interface-id}
```
**Document Structure:**
```json
{
  "_id": "ObjectId",
  "message_id": "fhir_http_1234567890",
  "interface_id": "uuid",
  "resource_type": "Patient",
  "resource_id": "patient-123",
  "operation": "create",
  "fhir_version": "R4",
  "received_at": "2025-10-18T10:00:00Z",
  "source_ip": "10.0.0.5",
  "http_method": "POST",
  "http_path": "/fhir/r4/Patient",
  "resource_content": { /* Full FHIR resource */ },
  "validation_status": "passed",
  "validation_warnings": [],
  "processed": true,
  "processed_at": "2025-10-18T10:00:01Z"
}
```

### MVC Components

#### Model Layer
**File:** `models/fhir_interface_models.go`
```go
type FHIRInterfaceConfig struct {
    Type                string
    FHIRVersion         string
    Protocol            string
    Port                int
    BasePath            string
    SupportedOperations []string
    ResourceTypes       ResourceTypeConfig
    AuthConfig          AuthConfig
    ValidationMode      string
    ProcessingOptions   ProcessingOptions
}

type ResourceTypeConfig struct {
    AllowAll bool
    Types    []string
}

type AuthConfig struct {
    Mode string // none, basic, bearer, oauth, mtls
}

type ProcessingOptions struct {
    StoreMongoDB      bool
    TriggerPipeline   bool
    AuditLogging      bool
    SendResponse      bool
}
```

#### Controller Layer
**File:** `controllers/fhir_receiver_controller.go`
```go
// POST /fhir/r4/:resourceType
func (frc *FHIRReceiverController) CreateResource(c *gin.Context)

// Handles:
// 1. Parse incoming FHIR resource
// 2. Validate against FHIR R4 schema
// 3. Store in MongoDB
// 4. Trigger processing pipeline
// 5. Return OperationOutcome
```

#### Service Layer
**File:** `services/fhir_receiver_service.go`
```go
type FHIRReceiverService struct {
    db           *sql.DB
    mongoClient  *mongo.Client
    validator    *FHIRValidationService
    pipeline     *TransformationPipelineService
}

func (frs *FHIRReceiverService) ReceiveFHIRResource(
    interfaceID string,
    resourceType string,
    resource map[string]interface{},
) (*OperationOutcome, error)
```

### HTTP Listener Integration

**File:** `processing/http_input_connector.go`
```go
type HTTPInputConnector struct {
    host         string
    port         int
    basePath     string
    interfaceID  string
    fhirVersion  string
    server       *http.Server
}

// Starts HTTP server on configured port
// Routes FHIR requests to FHIRReceiverController
```

### ProcessingEngine Integration

**File:** `processing/engine.go`
```go
// When activating FHIR receiver interface:
func (pe *ProcessingEngine) ActivateInterface(interfaceID string) error {
    // ... existing code ...

    // Check if source type is FHIR + HTTP
    if sourceType == "fhir" && connectivity == "http" {
        // Create HTTP connector instead of TCP
        connector = NewHTTPInputConnector(sourceConfig)
    }

    // Start HTTP listener
    go connector.Start(ctx, messageChan)
}
```

---

## Request/Response Flow

### Incoming FHIR Resource
**HTTP Request:**
```http
POST /fhir/r4/Patient HTTP/1.1
Host: localhost:8090
Content-Type: application/fhir+json

{
  "resourceType": "Patient",
  "id": "patient-123",
  "name": [{
    "family": "Doe",
    "given": ["John"]
  }],
  "gender": "male",
  "birthDate": "1980-01-01"
}
```

**Processing Steps:**
1. HTTP listener receives request
2. Extract resource type, validate JSON
3. FHIR validation (schema check)
4. Store in MongoDB: `received_fhir_resources_intf_{id}`
5. Generate message_id: `fhir_http_1760782345678901234`
6. Trigger processing pipeline (if configured)
7. Generate OperationOutcome

**HTTP Response (Success):**
```http
HTTP/1.1 201 Created
Location: /fhir/r4/Patient/patient-123
Content-Type: application/fhir+json

{
  "resourceType": "OperationOutcome",
  "issue": [{
    "severity": "information",
    "code": "informational",
    "diagnostics": "Resource created successfully"
  }]
}
```

**HTTP Response (Validation Warning):**
```http
HTTP/1.1 201 Created
Location: /fhir/r4/Patient/patient-123
Content-Type: application/fhir+json

{
  "resourceType": "OperationOutcome",
  "issue": [{
    "severity": "warning",
    "code": "business-rule",
    "diagnostics": "Missing recommended field: telecom"
  }]
}
```

---

## Implementation Phases

### Phase 1: MVP (This Sprint) ✅ Current Focus
**Goal:** Complete HL7 → FHIR → FHIR Receiver flow

**Tasks:**
1. ✅ Fix wizard to skip HL7 steps for FHIR source (DONE)
2. Create HTTP input connector for FHIR
3. Implement FHIRReceiverController (POST operation only)
4. Basic FHIR validation service
5. Store received FHIR in MongoDB
6. Update wizard Step 2 UI for FHIR configuration
7. Test end-to-end: HL7 → Transform → Send to FHIR Receiver

**Acceptance Criteria:**
- FHIR receiver accepts POST requests
- Stores FHIR resources in MongoDB
- Returns valid OperationOutcome
- Can receive transformed FHIR from HL7 pipeline

### Phase 2: Production Ready
- Add authentication modes
- Implement Read, Update, Delete, Search operations
- HTTPS support with SSL/TLS
- Strict FHIR validation
- Resource-specific filtering
- Performance optimization

### Phase 3: Advanced Features
- FHIR Subscriptions
- Batch/Transaction bundles
- FHIR Search API (complex queries)
- GraphQL API
- Version conversion (R4 ↔ R5)

---

## Configuration Examples

### Example 1: FHIR Receiver (No Auth)
```json
{
  "name": "Patient Data Receiver",
  "source_type": "fhir",
  "connectivity": "http",
  "source_config": {
    "type": "fhir",
    "fhir_version": "R4",
    "protocol": "http",
    "port": 8090,
    "base_path": "/fhir/r4",
    "supported_operations": ["create"],
    "resource_types": {"allow_all": true},
    "auth_config": {"mode": "none"},
    "validation_mode": "lenient"
  },
  "target_type": "database",
  "status": "active"
}
```

### Example 2: FHIR Receiver → HL7 Converter
```json
{
  "name": "FHIR to HL7 Bridge",
  "source_type": "fhir",
  "connectivity": "http",
  "source_config": { /* FHIR config */ },
  "target_type": "hl7",
  "target_config": {
    "protocol": "tcp",
    "host": "10.0.0.10",
    "port": 2575,
    "message_type": "ADT^A01"
  }
}
```

---

## Testing Strategy

### Unit Tests
- FHIR validation logic
- Resource parsing
- OperationOutcome generation

### Integration Tests
- HTTP endpoint → MongoDB storage
- End-to-end: HL7 → FHIR → FHIR Receiver
- Validation error handling

### Load Tests
- 100+ concurrent POST requests
- Large FHIR bundles (1MB+)
- Multiple active interfaces

---

## Security Considerations

### Phase 1 (MVP)
- Input validation (prevent injection)
- Rate limiting per interface
- Audit logging (who, what, when)

### Future Phases
- Authentication (Basic, OAuth, mTLS)
- Authorization (RBAC, ABAC)
- Encryption at rest (MongoDB)
- Encryption in transit (HTTPS)
- HIPAA compliance audit trail

---

## Monitoring & Observability

### Metrics to Track
- Requests per second (per interface)
- Validation success/failure rate
- Average response time
- Storage size (MongoDB)
- Error rate by resource type

### Logging
- All incoming requests (audit log)
- Validation warnings/errors
- Processing pipeline triggers
- Storage operations

---

## Future Enhancements

### FHIR Data Store (Paid Service)
When user wants persistent FHIR server capabilities:
- Full FHIR Search API
- Resource versioning (_history)
- Subscription support
- GraphQL API
- Bulk data export ($export)

**Integration:**
- Add toggle in wizard: "Enable FHIR Data Store"
- Requires paid subscription
- Provides additional APIs beyond receive-only

### Multi-Version Support
Easy extension pattern:
```go
// Add FHIR R5 support
case "R5":
    validator = NewFHIRR5Validator()
    schema = LoadFHIRR5Schema()
```

---

## Success Metrics

### MVP Success
- ✅ FHIR receiver accepts POST requests
- ✅ HL7 → FHIR → FHIR Receiver flow works
- ✅ Resources stored in MongoDB
- ✅ Valid OperationOutcome responses
- ✅ Basic validation working

### Production Ready
- 99.9% uptime
- < 100ms average response time
- Support 1000+ resources/minute
- Zero data loss

---

## Next Steps (Priority Order)

1. **Create HTTP Input Connector** (processing/http_input_connector.go)
2. **Create FHIR Receiver Controller** (controllers/fhir_receiver_controller.go)
3. **Create FHIR Receiver Service** (services/fhir_receiver_service.go)
4. **Update ProcessingEngine** (handle HTTP connectors)
5. **Update Wizard UI** (Step 2 for FHIR config)
6. **Test End-to-End** (HL7 → FHIR → Receiver)

---

**Document Version:** 1.0
**Last Updated:** 2025-10-18
**Status:** Design Approved - Ready for Implementation
