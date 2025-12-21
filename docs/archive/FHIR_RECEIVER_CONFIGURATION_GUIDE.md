# FHIR Receiver Configuration Guide

## Overview
A **FHIR Receiver** accepts incoming FHIR resources via HTTP/REST from external systems (EHRs, clinical applications, medical devices). This guide explains each configuration option in the wizard.

---

## 📋 Configuration Fields

### 1. FHIR Version
**Purpose**: Specifies which FHIR specification version to accept

**Options**:
- **FHIR R4** (Recommended)
  - Release 4 (2019)
  - Current industry standard
  - Best interoperability
  - Most modern EHR systems use this

- **FHIR STU3**
  - Standard for Trial Use 3 (2017)
  - Still widely deployed
  - Used by older EHR systems
  - Meaningful Use Stage 3 requirement

- **FHIR DSTU2**
  - Draft Standard for Trial Use 2 (2015)
  - Legacy systems only
  - Consider upgrading if possible

**When to Use Which**:
- Choose **R4** for new integrations
- Choose **STU3** if integrating with systems certified for Meaningful Use Stage 3
- Choose **DSTU2** only for legacy systems that can't upgrade

---

### 2. Base URL Path
**Purpose**: The base path where your FHIR receiver will listen for incoming requests

**Examples**:
- `/fhir` → Standard FHIR base path
- `/fhir/receiver/{interfaceId}` → Per-interface isolation
- `/api/fhir` → Custom namespace

**Full Endpoint Structure**:
```
http://localhost:3000/fhir/receiver/{interfaceId}
POST /fhir/receiver/abc-123 → Receives any FHIR resource

OR standard FHIR REST:
POST /fhir/Patient → Creates Patient resource
POST /fhir/Observation → Creates Observation resource
```

**Best Practice**: Use `/fhir` for FHIR spec compliance

---

### 3. Supported REST Operations
**Purpose**: Control which FHIR REST API operations this interface accepts

#### CREATE (POST)
- **What**: Create new resources
- **Endpoint**: `POST /fhir/{resourceType}`
- **When to Enable**: Always (most common operation)
- **Example**: `POST /fhir/Patient` with Patient JSON in body

#### READ (GET)
- **What**: Retrieve existing resources by ID
- **Endpoint**: `GET /fhir/{resourceType}/{id}`
- **When to Enable**: If sender systems need to query stored data
- **Example**: `GET /fhir/Patient/12345`

#### UPDATE (PUT)
- **What**: Replace entire resource
- **Endpoint**: `PUT /fhir/{resourceType}/{id}`
- **When to Enable**: If resources need to be updated
- **Example**: `PUT /fhir/Patient/12345` with updated Patient JSON

#### PATCH
- **What**: Partial update (JSON Patch)
- **Endpoint**: `PATCH /fhir/{resourceType}/{id}`
- **When to Enable**: For efficient partial updates
- **Example**: `PATCH /fhir/Patient/12345` with JSON Patch document

#### DELETE
- **What**: Remove resources
- **Endpoint**: `DELETE /fhir/{resourceType}/{id}`
- **When to Enable**: Rarely (audit concerns)
- **Example**: `DELETE /fhir/Patient/12345`

#### SEARCH
- **What**: Query resources with parameters
- **Endpoint**: `GET /fhir/{resourceType}?param=value`
- **When to Enable**: If sender systems need to search
- **Example**: `GET /fhir/Patient?family=Smith&given=John`

#### BATCH/TRANSACTION
- **What**: Multiple operations in one request
- **Endpoint**: `POST /fhir` with Bundle
- **When to Enable**: For bulk operations
- **Example**: Bundle with 100 Observations

**Recommendation**: Start with CREATE only, add others as needed

---

### 4. Authentication
**Purpose**: Secure your FHIR endpoint from unauthorized access

#### No Authentication (Development)
- **When to Use**: Local development, testing only
- **Security**: ⚠️ NONE - Anyone can access
- **Best For**: Proof of concept, debugging

#### API Key / Bearer Token
- **When to Use**: Simple token-based auth
- **How It Works**: Client sends `Authorization: Bearer {token}`
- **Best For**: Service-to-service communication
- **Example**:
  ```
  POST /fhir/Patient
  Authorization: Bearer abc123xyz789
  Content-Type: application/fhir+json
  ```

#### Basic Authentication
- **When to Use**: Username/password auth
- **How It Works**: Client sends `Authorization: Basic {base64(user:pass)}`
- **Best For**: Internal systems, simple setups
- **Security**: Use HTTPS only (password transmitted)

#### OAuth 2.0 (SMART on FHIR)
- **When to Use**: Patient-facing apps, SMART on FHIR compliance
- **How It Works**: OAuth 2.0 authorization flows
- **Best For**: User-authorized access, EHR integrations
- **Scopes**: `patient/*.read`, `user/*.write`, etc.
- **Standards**: SMART App Launch, SMART Backend Services

#### Mutual TLS (Certificate)
- **When to Use**: Highest security requirements
- **How It Works**: Both client and server authenticate with certificates
- **Best For**: Healthcare enterprise, high-value data
- **Requires**: PKI infrastructure, certificate management

**Recommendation**:
- Development: No Auth
- Production (internal): API Key or Basic Auth + HTTPS
- Production (external): OAuth 2.0 or mTLS

---

### 5. Resource Type Filtering
**Purpose**: Restrict which FHIR resource types this interface accepts

#### Enable Resource Filter
- **Checked**: Only accept specific resource types
- **Unchecked**: Accept all FHIR resources (default)

#### Accepted Resources (21 Common Types)
**Administrative**:
- Patient, Practitioner, Organization, Location

**Clinical**:
- Encounter, Observation, Condition, Procedure
- MedicationRequest, MedicationAdministration, Immunization
- AllergyIntolerance, DiagnosticReport, DocumentReference

**Workflow**:
- Appointment, ServiceRequest, CarePlan, Goal

**Financial**:
- Coverage, Claim, ExplanationOfBenefit

#### Reject Unknown Resources
- **Checked**: Return 400 error for non-listed resources
- **Unchecked**: Accept but log warning

**Use Cases**:
- **Accept All**: General-purpose FHIR receiver
- **Filter Specific**: Patient data only (Patient, Observation, Condition)
- **Reject Unknown**: Strict validation, compliance requirements

---

### 6. Validation Settings
**Purpose**: Ensure received FHIR resources meet quality standards

#### Validate Resource Structure
- **What**: Check required fields, data types, cardinality
- **Example**: Patient.name is required, Patient.birthDate must be valid date
- **When to Enable**: Always (recommended)
- **Performance**: Low impact

#### Validate Against Profiles
- **What**: Validate conformance to FHIR profiles (US Core, IPS)
- **Example**: US Core Patient requires race/ethnicity extensions
- **When to Enable**: Regulatory requirements, specific use cases
- **Performance**: Medium impact
- **Profiles**: US Core, International Patient Summary, custom profiles

#### Validate Terminology
- **What**: Validate codes against CodeSystems and ValueSets
- **Example**: Observation.code must be from LOINC
- **When to Enable**: Data quality critical, reporting requirements
- **Performance**: High impact (requires terminology server)

#### Reject Invalid Resources
- **Checked**: Return 400 error, don't store invalid resources
- **Unchecked**: Accept with warnings, store anyway
- **Best Practice**:
  - **Reject** for production, high-quality data requirements
  - **Accept** for development, pilot projects, flexible intake

---

### 7. Rate Limiting
**Purpose**: Prevent abuse, ensure fair usage, protect system resources

#### Enable Rate Limiting
- **When to Enable**: Production environments, public endpoints
- **When to Disable**: Trusted internal systems, development

#### Requests per Minute
- **Typical Values**: 60-600 per minute
- **Conservative**: 60 (1 per second)
- **Moderate**: 300 (5 per second)
- **High**: 600 (10 per second)

#### Burst Allowance
- **What**: Allow short bursts above the rate limit
- **Example**: Limit 60/min, burst 10 = allow 10 requests immediately, then throttle
- **Typical Values**: 10-100

**Response Headers**:
```
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 45
X-RateLimit-Reset: 1640000000
```

**Use Cases**:
- Public API: Enable with moderate limits
- Trusted partners: Higher limits or disable
- Batch imports: High burst allowance

---

### 8. Content Types
**Purpose**: Specify which content formats to accept

#### application/fhir+json (FHIR Standard)
- **Always Enable**: Yes
- **Standard**: FHIR R4/STU3/DSTU2 official format
- **Example**: `Content-Type: application/fhir+json`

#### application/json (Fallback)
- **Enable**: For compatibility
- **Why**: Some clients send generic JSON
- **Example**: `Content-Type: application/json`

#### application/fhir+xml (XML Support)
- **Enable**: If XML clients exist
- **Rare**: Most modern systems use JSON
- **Example**: `Content-Type: application/fhir+xml`

**Recommendation**: Enable JSON and FHIR+JSON, disable XML unless needed

---

### 9. Post-Reception Actions (Integration Engine Features)
**Purpose**: Define what happens AFTER receiving FHIR resources

#### Store in Database (Always Enabled)
- **What**: Save resources to PostgreSQL + MongoDB
- **Cannot Disable**: Core functionality
- **Storage**: Metadata in PostgreSQL, full content in MongoDB

#### Apply Transformation Pipeline
- **What**: Run custom transformations, enrichment, mappings
- **Use Cases**:
  - Convert FHIR R4 → STU3
  - Add facility-specific extensions
  - Enrich with external data (Epic, Cerner APIs)
  - Anonymize PHI
- **Configuration**: Set up in Transformation Pipeline (separate step)

#### Forward to Destination
- **What**: Route resources to another FHIR server
- **Use Cases**:
  - Mirror to backup system
  - Send to analytics platform
  - Forward to HIE (Health Information Exchange)
  - Route to specialist system

**Forwarding Options**:
- **Destination URL**: Target FHIR server
- **Async Forwarding**: Don't wait for response (faster, fire-and-forget)
- **Forward Only Valid**: Skip invalid resources

#### Trigger Workflow
- **What**: Conditional routing, business rules, notifications
- **Use Cases**:
  - Alert on critical lab results (Observation with high glucose)
  - Route COVID-19 cases to public health
  - Notify care team on new admission (Encounter)
  - Trigger prior authorization workflow

#### Generate FHIR AuditEvent
- **What**: Create FHIR AuditEvent resources for compliance
- **Use Cases**:
  - HIPAA compliance (track all data access)
  - Security auditing
  - Regulatory reporting
- **Contents**: Who, what, when, where, why

**Typical Configurations**:
- **Simple Receiver**: Store only
- **Mirror**: Store + Forward
- **Integration Hub**: Store + Transform + Workflow
- **Compliance**: Store + Audit + Workflow

---

## 🎯 Common Configuration Patterns

### 1. Development FHIR Receiver
```
FHIR Version: R4
Operations: CREATE only
Authentication: No Authentication
Resource Filter: Disabled (accept all)
Validation: Structure only
Rate Limiting: Disabled
Post-Actions: Store only
```

### 2. Production Patient Data Receiver
```
FHIR Version: R4
Operations: CREATE, UPDATE, READ
Authentication: OAuth 2.0 (SMART on FHIR)
Resource Filter: Patient, Observation, Condition, Encounter
Validation: Structure + Profiles (US Core)
Rate Limiting: 300/min, burst 50
Post-Actions: Store + Audit + Transform
```

### 3. HIE Integration Hub
```
FHIR Version: R4 + STU3 (multi-version)
Operations: All
Authentication: mTLS (certificate)
Resource Filter: Disabled (accept all)
Validation: Structure + Terminology
Rate Limiting: 1000/min, burst 200
Post-Actions: Store + Transform + Forward + Workflow + Audit
```

### 4. Mobile App Backend
```
FHIR Version: R4
Operations: CREATE, READ, SEARCH
Authentication: OAuth 2.0 (patient scopes)
Resource Filter: Patient, Observation, CarePlan, Goal
Validation: Structure only (performance)
Rate Limiting: 60/min, burst 10
Post-Actions: Store + Workflow (notifications)
```

---

## 🔒 Security Best Practices

1. **Always use HTTPS** in production (TLS 1.2+)
2. **Enable authentication** - never deploy "No Auth" to production
3. **Rate limiting** - prevent denial of service
4. **Input validation** - protect against malformed data
5. **Audit logging** - track all access for compliance
6. **Scope-based access** - OAuth scopes limit what clients can do
7. **Network isolation** - firewall rules, VPC, private networks
8. **Certificate pinning** - for mTLS deployments
9. **Token rotation** - expire API keys regularly
10. **Anomaly detection** - monitor for unusual patterns

---

## 📊 Monitoring & Operations

### Key Metrics to Track
- **Request volume**: Requests per minute/hour/day
- **Error rate**: 4xx and 5xx responses
- **Latency**: P50, P95, P99 response times
- **Resource types**: Distribution of received resources
- **Validation failures**: Resources rejected due to invalid data
- **Rate limit hits**: Clients being throttled

### Alerts to Configure
- Error rate > 5%
- Latency P95 > 1000ms
- Rate limit hit rate > 10%
- Validation failure rate > 20%
- Disk space < 20%

---

## 🆘 Troubleshooting

### Issue: Resources Not Appearing
**Check**:
1. Authentication configured correctly?
2. Resource type in filter list?
3. Validation rejecting resources?
4. Check logs for errors

### Issue: 401 Unauthorized
**Check**:
1. Auth type matches client configuration
2. Token/credentials valid
3. OAuth scopes sufficient
4. Certificate valid (mTLS)

### Issue: 400 Bad Request
**Check**:
1. Valid FHIR JSON/XML
2. Required fields present
3. Resource type supported
4. FHIR version matches

### Issue: 429 Too Many Requests
**Check**:
1. Rate limit too low?
2. Burst allowance insufficient?
3. Client sending too fast?
4. Adjust limits or use backoff

---

## 📚 Additional Resources

- [FHIR R4 Specification](https://hl7.org/fhir/R4/)
- [SMART on FHIR](https://docs.smarthealthit.org/)
- [US Core Implementation Guide](http://hl7.org/fhir/us/core/)
- [FHIR Terminology Service](https://www.hl7.org/fhir/terminology-service.html)
- [HL7 FHIR Community](https://chat.fhir.org/)
