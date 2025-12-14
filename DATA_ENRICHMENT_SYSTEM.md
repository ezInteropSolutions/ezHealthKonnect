# Data Enrichment System - Complete Architecture & Implementation Guide

## 📋 Table of Contents
1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Enrichment Strategies](#enrichment-strategies)
4. [Implementation Status](#implementation-status)
5. [Configuration Guide](#configuration-guide)
6. [API Reference](#api-reference)
7. [Examples](#examples)
8. [Best Practices](#best-practices)

---

## Overview

The Data Enrichment System provides **pluggable, strategy-based enrichment** capabilities for HL7 messages and other healthcare data formats. It follows the **Strategy Pattern** to allow different enrichment approaches to be configured and executed independently.

### 🎯 Key Features
- **4 Built-in Enrichment Strategies**: API, Database, Cache, Script
- **Strategy Pattern**: Pluggable executors for extensibility
- **Field Mapping**: Dynamic field extraction from HL7 messages
- **Error Handling**: Configurable failure modes with default values
- **Retry Logic**: Automatic retry with exponential backoff (API)
- **Timeout Management**: Per-strategy timeout configuration
- **Target Path Mapping**: Store enriched data at any location in message

---

## Architecture

### 🏗️ Component Hierarchy

```
Data Enrichment System
│
├── Models (models/enrichment_models.go)
│   ├── EnrichmentStrategy (enum)
│   ├── APIEnrichmentConfig
│   ├── DatabaseEnrichmentConfigV2
│   ├── CacheEnrichmentConfig
│   ├── ScriptEnrichmentConfig
│   └── UnifiedEnrichmentConfig
│
├── Executors (services/executors/enrichment/)
│   ├── MetadataEnrichmentExecutor (✅ Complete)
│   ├── APIEnrichmentExecutor (✅ Complete)
│   ├── DatabaseEnrichmentExecutor (🔄 In Progress)
│   ├── CacheEnrichmentExecutor (📋 Planned)
│   └── ScriptEnrichmentExecutor (📋 Planned)
│
└── Registry (services/executor_registry.go)
    └── Auto-registration of all enrichment executors
```

### 🔄 Data Flow

```
HL7 Message Received
    ↓
JSON Parsing (Layer 1)
    ↓
Pre-Processing Steps (Layer 2)
    ↓
┌─────────────────────────────────────┐
│  ENRICHMENT EXECUTORS (Parallel)    │
│                                     │
│  1. Metadata Enrichment             │
│     - Add timestamps                │
│     - Add correlation IDs           │
│     - Add interface metadata        │
│                                     │
│  2. API Enrichment                  │
│     - Query EMPI for demographics   │
│     - Query EHR for medical history │
│     - Query LIMS for lab results    │
│                                     │
│  3. Database Enrichment             │
│     - Lookup patient from local DB  │
│     - Get provider information      │
│     - Retrieve facility details     │
│                                     │
│  4. Cache Enrichment                │
│     - Check Redis for cached data   │
│     - Retrieve frequently-used data │
│     - Write-back cache updates      │
│                                     │
│  5. Script Enrichment               │
│     - Custom JavaScript logic       │
│     - Business rule calculations    │
│     - Data transformations          │
└─────────────────────────────────────┘
    ↓
Enriched Message Data
    ↓
Core Mapping (HL7 → FHIR)
    ↓
Post-Processing & Delivery
```

---

## Enrichment Strategies

### 1. 🌐 API Enrichment (`pre.enrichment.api`)

**Purpose**: Query external REST APIs to enrich message data

**Use Cases**:
- Query EMPI (Enterprise Master Patient Index) for patient demographics
- Retrieve medical history from EHR systems
- Get lab results from LIMS (Laboratory Information Management System)
- Verify insurance eligibility via payer APIs

**Configuration**:
```json
{
  "strategy": "api",
  "apiConfig": {
    "endpoint": "https://api.empi.hospital.org/patients/{patientId}",
    "method": "GET",
    "authType": "bearer",
    "bearerToken": "your-api-token",
    "headers": {
      "Accept": "application/json"
    },
    "fieldMappings": {
      "patientId": "PID.3"
    },
    "targetPath": "enriched.empi",
    "timeoutMs": 5000,
    "retryCount": 2,
    "retryDelayMs": 1000,
    "failOnError": false,
    "defaultValue": {
      "status": "not_found"
    }
  }
}
```

**Field Mappings**:
- Extract values from HL7 message using simple field keys (PID.3, MSH.9, etc.)
- Use placeholders in endpoint URL: `{patientId}` → replaced with PID.3 value
- Map response data to target path in enriched message

**Authentication Support**:
- **None**: No authentication
- **Basic**: Username + Password (Base64 encoded)
- **Bearer**: JWT or OAuth tokens
- **API Key**: Custom API key header

**Error Handling**:
- **failOnError: true** → Pipeline stops if API call fails
- **failOnError: false** → Continue with defaultValue or empty response
- **Retry Logic**: Automatic retries with configurable delay

---

### 2. 🗄️ Database Enrichment (`pre.enrichment.database`)

**Purpose**: Query local or remote databases to enrich message data

**Use Cases**:
- Lookup patient from local patient registry
- Retrieve provider/practitioner details
- Get facility/location information
- Query reference data (codes, value sets)

**Configuration**:
```json
{
  "strategy": "database",
  "databaseConfig": {
    "connectionString": "postgresql://user:pass@localhost:5432/hospital",
    "databaseType": "postgresql",
    "query": "SELECT * FROM patients WHERE patient_id = $1",
    "queryParams": {
      "1": "PID.3"
    },
    "targetPath": "enriched.database.patient",
    "resultMapping": {
      "first_name": "firstName",
      "last_name": "lastName",
      "date_of_birth": "dob"
    },
    "timeoutMs": 3000,
    "cacheResults": true,
    "cacheTTL": 300,
    "failOnError": false
  }
}
```

**Database Support**:
- **PostgreSQL** (`postgresql`)
- **MySQL** (`mysql`)
- **SQL Server** (`sqlserver`)
- **Oracle** (`oracle`)

**Query Parameters**:
- Parameterized queries prevent SQL injection
- Map parameters to HL7 message fields
- Support for numbered ($1, $2) and named (:param) placeholders

**Result Mapping**:
- Map database column names to output field names
- Store results at configurable target path
- Support for nested result structures

**Caching**:
- Optional result caching to reduce database load
- Configurable TTL (time-to-live) in seconds
- Cache key generated from query + parameters

---

### 3. ⚡ Cache Enrichment (`pre.enrichment.cache`)

**Purpose**: Query cache systems (Redis, Memcached) for fast data retrieval

**Use Cases**:
- Retrieve frequently-accessed patient data
- Get cached reference data (codes, translations)
- Implement session/context tracking
- Store temporary processing state

**Configuration**:
```json
{
  "strategy": "cache",
  "cacheConfig": {
    "connectionString": "redis://localhost:6379",
    "cacheType": "redis",
    "keyTemplate": "patient:{patientId}:demographics",
    "keyMappings": {
      "patientId": "PID.3"
    },
    "targetPath": "enriched.cache.patient",
    "timeoutMs": 1000,
    "failOnError": false,
    "writeBack": true,
    "ttlSeconds": 3600
  }
}
```

**Cache Types**:
- **Redis**: In-memory key-value store
- **Memcached**: Distributed memory caching system

**Key Templates**:
- Dynamic key generation from message fields
- Template placeholders: `patient:{patientId}` → `patient:P123456`
- Support for multi-level keys: `facility:{facilityId}:location:{locationId}`

**Write-Back**:
- Optional write-back to cache after enrichment
- Configurable TTL for cached entries
- Automatic cache invalidation

---

### 4. 📜 Script Enrichment (`pre.enrichment.script`)

**Purpose**: Execute custom JavaScript for business logic and calculations

**Use Cases**:
- Calculate derived fields (age from DOB, BMI from height/weight)
- Apply business rules (VIP patient detection, priority scoring)
- Custom data transformations
- Complex validations requiring multiple fields

**Configuration**:
```json
{
  "strategy": "script",
  "scriptConfig": {
    "script": "function enrich(input) { var pid = input.enhancedSegments.PID; var dob = pid.fields[7].value; var age = calculateAge(dob); return { age: age, ageGroup: age < 18 ? 'pediatric' : 'adult' }; }",
    "context": {
      "hospitalId": "H12345",
      "environment": "production"
    },
    "targetPath": "enriched.calculated",
    "timeoutMs": 5000,
    "failOnError": false
  }
}
```

**JavaScript Runtime**:
- Uses `goja` library for Go-based JavaScript execution
- Access to message data via `input` parameter
- Access to context variables
- Safe sandbox execution with timeout

**Example Scripts**:

**Calculate Age from Date of Birth**:
```javascript
function enrich(input) {
    var dob = input.enhancedSegments.PID.fields[7].value; // PID.7
    var birthDate = new Date(
        dob.substring(0, 4),   // Year
        dob.substring(4, 6) - 1, // Month (0-indexed)
        dob.substring(6, 8)     // Day
    );
    var today = new Date();
    var age = today.getFullYear() - birthDate.getFullYear();

    return {
        age: age,
        ageGroup: age < 18 ? 'pediatric' : 'adult'
    };
}
```

**VIP Patient Detection**:
```javascript
function enrich(input) {
    var patientClass = input.enhancedSegments.PV1.fields[2].value; // PV1.2
    var lastName = input.enhancedSegments.PID.fields[5].subfields[0].value; // PID.5.1

    var isVIP = patientClass === 'V' || lastName.includes('VIP');

    return {
        isVIP: isVIP,
        priority: isVIP ? 'high' : 'normal'
    };
}
```

---

## Implementation Status

### ✅ Completed
1. **Models** (`models/enrichment_models.go`)
   - EnrichmentStrategy enum
   - APIEnrichmentConfig
   - DatabaseEnrichmentConfigV2
   - CacheEnrichmentConfig
   - ScriptEnrichmentConfig
   - UnifiedEnrichmentConfig

2. **Metadata Enrichment** (`services/executors/enrichment/metadata_enrichment_executor.go`)
   - Adds timestamps, correlation IDs, interface IDs
   - Custom metadata fields
   - Message ID extraction from MSH.10

3. **API Enrichment** (`services/executors/enrichment/api_enrichment_executor.go`)
   - HTTP client with timeout and retry
   - Authentication (Basic, Bearer, API Key)
   - Field mapping and URL placeholder replacement
   - Response mapping to target path
   - Error handling with default values

4. **Base Executor Utilities** (`services/executors/base_executor.go`)
   - GetNestedValue for field extraction
   - SetNestedValue for storing enriched data
   - HL7 field key resolution (PID.3, MSH.9, etc.)

### 🔄 In Progress
1. **Database Enrichment** (`services/executors/enrichment/database_enrichment_executor.go`)
   - PostgreSQL, MySQL, SQL Server, Oracle support
   - Parameterized queries
   - Result mapping
   - Connection pooling
   - Query caching

### 📋 Planned
1. **Cache Enrichment** (`services/executors/enrichment/cache_enrichment_executor.go`)
   - Redis client integration
   - Memcached support
   - Key template generation
   - Write-back support

2. **Script Enrichment** (`services/executors/enrichment/script_enrichment_executor.go`)
   - Goja JavaScript runtime
   - Sandbox execution
   - Context variable injection
   - Timeout handling

3. **Executor Registry Integration**
   - Auto-register all enrichment executors
   - Replace placeholder in `executor_registry.go`

4. **UI Configuration**
   - Pipeline Builder enrichment step configuration
   - Strategy selection dropdown
   - Dynamic form fields based on selected strategy

5. **Testing**
   - Unit tests for each enrichment executor
   - Integration tests with sample HL7 messages
   - Performance benchmarks

---

## Configuration Guide

### Step 1: Add Enrichment Step to Pipeline

In the Pipeline Builder, add an enrichment step to the pre-processing layer:

```
Pipeline Builder → Pre-Processing → Add Enrichment Step
```

### Step 2: Select Strategy

Choose the enrichment strategy:
- API Enrichment
- Database Enrichment
- Cache Enrichment
- Script Enrichment

### Step 3: Configure Strategy-Specific Settings

#### API Enrichment Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| endpoint | string | Yes | - | API endpoint URL with placeholders |
| method | string | No | GET | HTTP method (GET, POST, PUT, etc.) |
| authType | enum | No | none | Authentication type |
| fieldMappings | object | No | {} | Map placeholders to HL7 fields |
| targetPath | string | No | enriched.api | Where to store response |
| timeoutMs | integer | No | 5000 | Request timeout in milliseconds |
| retryCount | integer | No | 0 | Number of retry attempts |
| failOnError | boolean | No | false | Stop pipeline on failure |

#### Database Enrichment Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| connectionString | string | Yes* | - | Database connection string |
| connectionName | string | Yes* | - | Reference to stored connection |
| databaseType | enum | Yes | - | postgresql, mysql, sqlserver, oracle |
| query | string | Yes | - | SQL query with parameters |
| queryParams | object | No | {} | Map parameters to HL7 fields |
| targetPath | string | No | enriched.database | Where to store results |
| timeoutMs | integer | No | 3000 | Query timeout in milliseconds |
| cacheResults | boolean | No | false | Cache query results |
| cacheTTL | integer | No | 0 | Cache TTL in seconds |
| failOnError | boolean | No | false | Stop pipeline on failure |

*Either connectionString or connectionName is required

#### Cache Enrichment Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| connectionString | string | Yes* | - | Cache connection string |
| connectionName | string | Yes* | - | Reference to stored connection |
| cacheType | enum | Yes | - | redis, memcached |
| keyTemplate | string | Yes | - | Cache key template |
| keyMappings | object | No | {} | Map placeholders to HL7 fields |
| targetPath | string | No | enriched.cache | Where to store cached data |
| timeoutMs | integer | No | 1000 | Cache operation timeout |
| writeBack | boolean | No | false | Write enriched data back to cache |
| ttlSeconds | integer | No | 0 | TTL for cache entries |
| failOnError | boolean | No | false | Stop pipeline on failure |

*Either connectionString or connectionName is required

#### Script Enrichment Configuration

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| script | string | Yes | - | JavaScript code to execute |
| context | object | No | {} | Variables to pass to script |
| targetPath | string | No | enriched.script | Where to store script output |
| timeoutMs | integer | No | 5000 | Script execution timeout |
| failOnError | boolean | No | false | Stop pipeline on failure |

---

## API Reference

### EnrichmentStrategy Enum

```go
type EnrichmentStrategy string

const (
    EnrichmentStrategyAPI      EnrichmentStrategy = "api"
    EnrichmentStrategyDatabase EnrichmentStrategy = "database"
    EnrichmentStrategyCache    EnrichmentStrategy = "cache"
    EnrichmentStrategyScript   EnrichmentStrategy = "script"
)
```

### UnifiedEnrichmentConfig

```go
type UnifiedEnrichmentConfig struct {
    Strategy       EnrichmentStrategy
    APIConfig      *APIEnrichmentConfig
    DatabaseConfig *DatabaseEnrichmentConfigV2
    CacheConfig    *CacheEnrichmentConfig
    ScriptConfig   *ScriptEnrichmentConfig
    Enabled        bool   // Default: true
    StopOnError    bool   // Default: false
    LogLevel       string // debug, info, warn, error
}
```

---

## Examples

### Example 1: EMPI Patient Lookup

**Scenario**: Query EMPI for patient demographics using patient ID from HL7 message

```json
{
  "step_name": "EMPI Patient Lookup",
  "step_type": "pre.enrichment.api",
  "sequence": 20,
  "config": {
    "strategy": "api",
    "apiConfig": {
      "endpoint": "https://empi.hospital.org/api/v1/patients/{patientId}",
      "method": "GET",
      "authType": "bearer",
      "bearerToken": "${EMPI_API_TOKEN}",
      "headers": {
        "Accept": "application/json",
        "Content-Type": "application/json"
      },
      "fieldMappings": {
        "patientId": "PID.3"
      },
      "targetPath": "enriched.empi.demographics",
      "timeoutMs": 5000,
      "retryCount": 2,
      "retryDelayMs": 1000,
      "failOnError": false,
      "defaultValue": {
        "status": "not_found",
        "source": "empi"
      }
    }
  }
}
```

**Input Message**:
```
MSH|^~\&|SendingApp|SendingFacility|ReceivingApp|ReceivingFacility|20231214120000||ADT^A01|MSG00001|P|2.5
PID|1||P123456^^^Hospital^MR||Doe^John^M||19800515|M|||123 Main St^^City^ST^12345
```

**Enriched Output**:
```json
{
  "enhancedSegments": { ... },
  "enriched": {
    "empi": {
      "demographics": {
        "patientId": "P123456",
        "masterPatientId": "MRN987654",
        "firstName": "John",
        "lastName": "Doe",
        "dateOfBirth": "1980-05-15",
        "gender": "M",
        "addresses": [
          {
            "line1": "123 Main St",
            "city": "City",
            "state": "ST",
            "zip": "12345"
          }
        ]
      }
    }
  }
}
```

### Example 2: Local Database Provider Lookup

**Scenario**: Query local database for provider information

```json
{
  "step_name": "Provider Lookup",
  "step_type": "pre.enrichment.database",
  "sequence": 30,
  "config": {
    "strategy": "database",
    "databaseConfig": {
      "connectionName": "hospital_db",
      "databaseType": "postgresql",
      "query": "SELECT provider_id, first_name, last_name, npi, specialty FROM providers WHERE provider_id = $1",
      "queryParams": {
        "1": "PV1.7"
      },
      "targetPath": "enriched.provider",
      "resultMapping": {
        "provider_id": "id",
        "first_name": "firstName",
        "last_name": "lastName",
        "npi": "nationalProviderId",
        "specialty": "specialty"
      },
      "timeoutMs": 3000,
      "cacheResults": true,
      "cacheTTL": 3600,
      "failOnError": false
    }
  }
}
```

### Example 3: Redis Cache Lookup

**Scenario**: Check Redis for cached patient preferences

```json
{
  "step_name": "Patient Preferences Cache",
  "step_type": "pre.enrichment.cache",
  "sequence": 25,
  "config": {
    "strategy": "cache",
    "cacheConfig": {
      "connectionString": "redis://localhost:6379",
      "cacheType": "redis",
      "keyTemplate": "patient:{patientId}:preferences",
      "keyMappings": {
        "patientId": "PID.3"
      },
      "targetPath": "enriched.preferences",
      "timeoutMs": 1000,
      "failOnError": false,
      "writeBack": true,
      "ttlSeconds": 86400
    }
  }
}
```

### Example 4: Calculate Age from Date of Birth

**Scenario**: Use JavaScript to calculate patient age

```json
{
  "step_name": "Calculate Patient Age",
  "step_type": "pre.enrichment.script",
  "sequence": 15,
  "config": {
    "strategy": "script",
    "scriptConfig": {
      "script": "function enrich(input) { var dob = input.enhancedSegments.PID.fields[7].value; var year = parseInt(dob.substring(0, 4)); var month = parseInt(dob.substring(4, 6)) - 1; var day = parseInt(dob.substring(6, 8)); var birthDate = new Date(year, month, day); var today = new Date(); var age = today.getFullYear() - birthDate.getFullYear(); var m = today.getMonth() - birthDate.getMonth(); if (m < 0 || (m === 0 && today.getDate() < birthDate.getDate())) { age--; } return { age: age, ageGroup: age < 18 ? 'pediatric' : (age >= 65 ? 'geriatric' : 'adult'), ageInMonths: age * 12 + m }; }",
      "targetPath": "enriched.demographics.age",
      "timeoutMs": 2000,
      "failOnError": false
    }
  }
}
```

---

## Best Practices

### 1. Strategy Selection

| Use Case | Recommended Strategy |
|----------|---------------------|
| External system lookup | API Enrichment |
| Local data queries | Database Enrichment |
| High-frequency lookups | Cache Enrichment |
| Business logic/calculations | Script Enrichment |
| Add timestamps/IDs | Metadata Enrichment |

### 2. Error Handling

✅ **Do**:
- Set `failOnError: false` for non-critical enrichment
- Provide `defaultValue` for API/Cache failures
- Use caching for expensive operations
- Set appropriate timeouts (API: 5s, DB: 3s, Cache: 1s)

❌ **Don't**:
- Set `failOnError: true` for optional enrichment
- Use long timeouts that block pipeline
- Skip error handling configuration
- Hardcode sensitive credentials in config

### 3. Performance Optimization

- **Use caching**: Enable `cacheResults` for database queries
- **Parallel execution**: Multiple enrichment steps run in parallel
- **Timeout tuning**: Adjust timeouts based on SLA
- **Retry logic**: Configure retries for transient failures
- **Connection pooling**: Reuse database connections

### 4. Security

- **Credentials**: Store API keys/passwords in environment variables
- **Encryption**: Use TLS for external API calls
- **SQL Injection**: Always use parameterized queries
- **Sandbox**: Script execution runs in isolated environment
- **Audit Trail**: All enrichment operations are logged

### 5. Monitoring

- **Log Level**: Use `debug` for development, `info` for production
- **Metrics**: Track enrichment success/failure rates
- **Latency**: Monitor enrichment execution times
- **Cache Hit Rate**: Track cache effectiveness
- **Error Patterns**: Identify and fix common failures

---

## Next Steps

1. **Complete Implementation**
   - ✅ API Enrichment Executor
   - 🔄 Database Enrichment Executor (in progress)
   - 📋 Cache Enrichment Executor
   - 📋 Script Enrichment Executor

2. **Registry Integration**
   - Register all enrichment executors in `executor_registry.go`
   - Replace placeholder enrichment executor

3. **UI Configuration**
   - Add enrichment step type to Pipeline Builder
   - Create strategy selection dropdown
   - Dynamic form fields for each strategy

4. **Testing**
   - Unit tests for each executor
   - Integration tests with sample HL7 messages
   - Performance benchmarks

5. **Documentation**
   - Update SYSTEM_DOCUMENTATION.md
   - Add enrichment examples to cookbook
   - Create video tutorials

---

## Related Documentation

- [VALIDATION_SYSTEM_SUMMARY.md](VALIDATION_SYSTEM_SUMMARY.md) - Validation executors
- [SYSTEM_DOCUMENTATION.md](SYSTEM_DOCUMENTATION.md) - Complete system reference
- [TRANSFORMATION_PIPELINE_DESIGN.md](TRANSFORMATION_PIPELINE_DESIGN.md) - Pipeline architecture
- [CONNECTIVITY_CATALOG.md](CONNECTIVITY_CATALOG.md) - Connector reference

---

**Last Updated**: December 14, 2025
**Status**: In Development (60% Complete)
**Next Milestone**: Database & Cache Enrichment Executors
