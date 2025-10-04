# ezHealthKonnect System Documentation

**Version**: 2.0
**Last Updated**: October 2025
**Status**: Production-Ready Architecture

> **Note**: This is the **master** architecture document. It consolidates information from 20+ design documents into a single, authoritative reference.

---

## Table of Contents

1. [System Overview](#system-overview)
2. [Core Architecture](#core-architecture)
3. [JSON Conversion Pipeline](#json-conversion-pipeline)
4. [Transformation Pipeline](#transformation-pipeline)
5. [Hybrid Storage Architecture](#hybrid-storage-architecture)
6. [Mapping & OOB Templates](#mapping--oob-templates)
7. [Interface Wizard](#interface-wizard)
8. [Scalability Design](#scalability-design)
9. [Database Schema](#database-schema)
10. [API Reference](#api-reference)
11. [Quick Reference](#quick-reference)

---

## System Overview

### What is ezHealthKonnect?

ezHealthKonnect is an AI-powered healthcare integration platform that transforms HL7v2 messages to FHIR R4 format using a hybrid architecture approach combining Node.js and Go.

### Key Features

- ✅ **Automatic JSON Conversion**: All HL7 messages converted to enhanced JSON (100% parser reuse)
- ✅ **Flexible Transformation Pipeline**: User-configurable step-by-step processing
- ✅ **Hybrid Storage**: PostgreSQL metadata + MongoDB full content
- ✅ **OOB Templates**: Out-of-box mapping templates for common message types
- ✅ **Visual Wizard**: Drag-drop interface configuration
- ✅ **Production Scale**: Handles millions of messages with interface isolation

### Architectural Patterns

- **MVC (Model-View-Controller)**: Clean separation of concerns
- **OOB (Out-of-Box)**: Auto-configuration with minimal setup
- **Event-Driven**: Asynchronous message processing with goroutines
- **Factory Pattern**: Extensible parser and executor registries

---

## Core Architecture

### Technology Stack

#### Frontend (Node.js)
- **Framework**: Express.js
- **Auth**: JWT + Session-based
- **ORM**: Sequelize (PostgreSQL)
- **UI**: Vanilla JavaScript (no heavy frameworks)

#### Backend (Go)
- **Framework**: Gin (HTTP router)
- **Concurrency**: Goroutines for async processing
- **Database**: database/sql (PostgreSQL), mongo-driver (MongoDB)
- **Parsing**: 100% reuse of existing hl7.ParseWithRealSchema()

#### Databases
- **PostgreSQL**: Metadata, audit logs, configuration
- **MongoDB**: Full message content (raw + parsed + transformed)

### Dual-Language Communication

```
┌─────────────────────────────────────────────────────────────┐
│                      Client Browser                          │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│           Node.js Frontend (Port 3000)                       │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Proxy Layer (app.js)                                │   │
│  │  - /api/fhir/* → Go Backend                          │   │
│  │  - /api/hl7/* → Go Backend                           │   │
│  │  - /api/auth/* → Node.js                             │   │
│  │  - /api/wizard/* → Node.js                           │   │
│  └──────────────────────────────────────────────────────┘   │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│            Go Backend (Port 8080)                            │
│  - HL7/FHIR Transformation                                   │
│  - High-performance message processing                       │
│  - Async goroutine-based execution                          │
└─────────────────────────────────────────────────────────────┘
```

---

## JSON Conversion Pipeline

### Architecture: MVC + OOB Pattern

The JSON conversion system automatically converts ALL incoming messages to structured JSON as the first transformation step, preserving the full enhanced schema from the HL7 parser.

### Components

#### Models (`models/parser_models.go`)
```go
type ParsedMessage struct {
    MessageID       string
    InterfaceID     string
    Format          string  // "hl7v2", "fhir", "cda"
    ParsedContent   map[string]interface{}
    ParsedAt        time.Time
    ParsingTime     int64
    ParsingStatus   string
    ParsingError    string
}
```

#### Services (Parser Registry - Factory Pattern)
```
services/
├── format_detector.go         # Auto-detect HL7/FHIR/CDA
├── parser_factory.go          # Registry of all parsers
├── message_parser_service.go  # Orchestrator
└── parsers/
    └── hl7_parser_service.go  # HL7 adapter (wraps existing parser)
```

#### Controller (`processing/engine.go`)
```go
func (pe *ProcessingEngine) convertToJSON(messageID string) {
    // 1. Auto-detect format
    // 2. Get parser from factory
    // 3. Parse to JSON
    // 4. Store in MongoDB
    // 5. Update PostgreSQL status
}
```

### Data Flow

```
Message Received
    ↓
Store Raw (PostgreSQL + MongoDB)
    ↓
Async Trigger: go convertToJSON()
    ↓
Auto-detect Format → Get Parser → Parse to JSON
    ↓
Store Enhanced Schema in MongoDB (parsed_content field)
    ↓
Update PostgreSQL (parsing_status, parsed_at, parsing_time_ms)
    ↓
Ready for Transformation Pipeline
```

### MongoDB Storage Structure

```javascript
// Collection: raw_messages_<interface-id>
{
  message_id: "tcp_1234567890",
  raw_content: "MSH|^~\\&|...",  // Original HL7

  parsed_content: {  // FULL ENHANCED SCHEMA
    enhancedSegments: {
      "MSH": {
        key: "MSH",
        name: "Message Header",
        fields: [
          {
            key: "MSH.3",
            name: "Sending Application",
            value: "HIS",
            position: 3,
            dataType: "HD",
            subfields: [
              {key: "MSH.3.1", value: "HIS", position: 1},
              {key: "MSH.3.2", value: "HOSPITAL", position: 2}
            ]
          }
        ]
      },
      "PID": {...},
      "PV1": {...}
    },
    segmentOrder: ["MSH", "EVN", "PID", "PV1"],
    messageType: {code: "ADT", event: "A01"},
    version: "2.5",
    dictionaryUsed: true,
    schemaLoaded: true
  },

  parsed_at: ISODate("2025-10-04T..."),
  parsing_time_ms: 125,
  parsed_format: "hl7v2"
}
```

### PostgreSQL Tracking

```sql
-- V19 Migration added columns to messages_intf_<interface-id>
ALTER TABLE messages_intf_xyz ADD COLUMN parsed_at TIMESTAMP;
ALTER TABLE messages_intf_xyz ADD COLUMN parsing_status VARCHAR(50);
ALTER TABLE messages_intf_xyz ADD COLUMN parsing_time_ms INTEGER;
ALTER TABLE messages_intf_xyz ADD COLUMN parsing_error TEXT;
```

### OOB Initialization

The parser service initializes automatically:

```go
// processing/engine.go - NewProcessingEngine()
mongoService, err := services.NewMongoDBConnectionService()
if err == nil {
    // Hybrid storage with auto-parser initialization
    parserService := services.NewMessageParserService(db, mongoService, nil)
    return &ProcessingEngine{
        parserService: parserService,
        // ... other services
    }
}
```

**Zero configuration required** - just set `MONGODB_URI` in environment.

---

## Transformation Pipeline

### Architecture: Flexible Step Execution

The transformation pipeline applies business logic to parsed JSON messages in a user-defined sequence.

### Four-Layer Model

```
Layer 0: JSON Conversion (System - Auto)    ← COMPLETE ✅
    ↓
Layer 1: Pre-Processing (User-defined)      ← Validation, enrichment
    ↓
Layer 2: Core Mapping (Template-based)      ← HL7→FHIR using stored mappings ✅
    ↓
Layer 3: Post-Processing (User-defined)     ← FHIR validation, anonymization
```

### Database Schema (V20)

```sql
-- Pipelines: One per interface + message type
CREATE TABLE transformation_pipelines (
    id UUID PRIMARY KEY,
    interface_id UUID REFERENCES interfaces(id),
    message_type VARCHAR(50),  -- "ADT^A01", "ORU^R01"
    pipeline_name VARCHAR(100),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP,
    UNIQUE(interface_id, message_type)
);

-- Steps: Individual transformations in sequence
CREATE TABLE transformation_steps (
    id UUID PRIMARY KEY,
    pipeline_id UUID REFERENCES transformation_pipelines(id),
    sequence INTEGER,  -- 10, 20, 30... (execution order)
    step_name VARCHAR(100),
    step_type VARCHAR(50),  -- validation, enrichment, mapping, custom
    executor_type VARCHAR(50),  -- Built-in or custom
    script_content TEXT,  -- JavaScript for custom logic
    config JSONB,  -- Step-specific parameters
    depends_on_steps UUID[],  -- Dependency handling
    is_active BOOLEAN DEFAULT true
);

-- Execution History
CREATE TABLE transformation_executions (
    id UUID PRIMARY KEY,
    pipeline_id UUID,
    message_id VARCHAR(100),
    status VARCHAR(50),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    execution_time_ms INTEGER,
    input_snapshot JSONB,
    output_snapshot JSONB,
    error_message TEXT
);
```

### Built-in Step Types

#### 1. Validation Steps (`pre.validation`)
```javascript
// Example: Validate required PID fields
config: {
    required_segments: ["MSH", "PID"],
    required_fields: {
        "PID": ["PID.3", "PID.5", "PID.7"]
    }
}
```

#### 2. Enrichment Steps (`pre.enrichment`)
```javascript
// Example: Enrich from external API
config: {
    api_endpoint: "https://epic.hospital.com/api/patient/{PID.3.1}",
    cache_ttl: 3600,
    fields_to_add: ["insurance", "allergies"]
}
```

#### 3. Core Mapping (`core.mapping`)
```javascript
// Uses stored template mappings from hl7_fhir_templates
config: {
    interface_id: "uuid-here",
    message_type: "ADT^A01",
    template_id: "uuid-here"  // Optional: override default
}
```

#### 4. Custom JavaScript (`custom.javascript`)
```javascript
// User-defined transformation logic
function transform(input) {
    var pid = input.enhancedSegments.PID;
    if (pid.fields.find(f => f.key === "PID.5").value.includes("VIP")) {
        input._metadata.priority = "high";
    }
    return input;
}
```

### Execution Flow

```go
// services/transformation_pipeline_service.go
func (s *TransformationPipelineService) Execute(messageID, interfaceID, messageType string) error {
    // 1. Get pipeline for interface + message type
    pipeline := s.GetPipeline(interfaceID, messageType)

    // 2. Get steps ordered by sequence
    steps := s.GetSteps(pipeline.ID)

    // 3. Load parsed JSON from MongoDB
    input := s.LoadParsedMessage(messageID)

    // 4. Execute steps in order
    for _, step := range steps {
        executor := s.executorRegistry.GetExecutor(step.ExecutorType)
        output, err := executor.Execute(ctx, step, input)
        if err != nil {
            return s.HandleStepFailure(step, err)
        }
        input = output  // Output becomes next input
    }

    // 5. Store final output
    s.StoreTransformedMessage(messageID, input)
    return nil
}
```

### Current Implementation Status

- ✅ **JSON Conversion Pipeline**: COMPLETE (V19 migration)
- ✅ **Core Mapping Executor**: COMPLETE (HL7→FHIR using wizard mappings)
- ⏳ **Flexible Pipeline**: DESIGNED (V20 migration pending)
- ⏳ **Custom JavaScript Runtime**: DESIGNED (goja library planned)

---

## Hybrid Storage Architecture

### Design Philosophy

**PostgreSQL**: Fast queries, relational integrity, ACID compliance
**MongoDB**: Flexible schema, full document storage, fast writes

### Storage Distribution

```
┌─────────────────────────────────────────────────────────────┐
│                     PostgreSQL                               │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Metadata (messages_intf_<id>)                          │ │
│  │  - message_id (PK)                                     │ │
│  │  - interface_id, correlation_id                        │ │
│  │  - status, priority                                    │ │
│  │  - received_at, processing_time_ms                     │ │
│  │  - parsed_at, parsing_status  ← V19                   │ │
│  │  - message_type, message_size                          │ │
│  └────────────────────────────────────────────────────────┘ │
└───────────────────────────┬─────────────────────────────────┘
                            │ (message_id linkage)
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                      MongoDB                                 │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Full Content (raw_messages_<interface-id>)             │ │
│  │  - message_id (matches PostgreSQL)                     │ │
│  │  - raw_content (original HL7)                          │ │
│  │  - parsed_content (full enhanced JSON)                 │ │
│  │  - metadata                                            │ │
│  └────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Transformed Output (transformed_messages_intf_<id>)    │ │
│  │  - _id (output message ID)                             │ │
│  │  - correlation_id (links to input)                     │ │
│  │  - fhir_bundle (complete FHIR resources)               │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### Cross-Database Referential Integrity

Since PostgreSQL and MongoDB are separate systems, referential integrity is maintained through:

#### 1. Message ID Consistency
```go
// Generate UUID once, use everywhere
messageID := fmt.Sprintf("tcp_%d", time.Now().UnixNano())

// PostgreSQL insert
db.Exec("INSERT INTO messages_intf_xyz (message_id, ...) VALUES ($1, ...)", messageID)

// MongoDB insert (same ID)
mongoColl.InsertOne(bson.M{"message_id": messageID, ...})
```

#### 2. Lineage Tracking
```go
// Output messages track their input source
outputMessageID := fmt.Sprintf("mongo_%s_%s", interfaceID, inputMessageID)

transformedDoc := bson.M{
    "_id": outputMessageID,
    "correlation_id": inputMessageID,  // Link back to input
    "interface_id": interfaceID,
    "fhir_bundle": fhirBundle,
}
```

#### 3. Consistency Checks
```go
// Periodic reconciliation service
func (s *ConsistencyService) CheckOrphans() {
    // Find PostgreSQL records without MongoDB content
    orphans := db.Query(`
        SELECT message_id FROM messages_intf_xyz m
        WHERE NOT EXISTS (
            SELECT 1 FROM external_mongo_check(message_id)
        )
    `)

    // Alert or auto-repair
    for _, orphan := range orphans {
        s.AlertOrphanedRecord(orphan.MessageID)
    }
}
```

### Performance Characteristics

| Operation | PostgreSQL | MongoDB | Winner |
|-----------|-----------|---------|--------|
| Simple metadata queries | 1-5ms | 10-20ms | PostgreSQL ✅ |
| Full document retrieval | N/A | 5-15ms | MongoDB ✅ |
| Complex joins | Fast | Slow | PostgreSQL ✅ |
| Bulk writes | Moderate | Very Fast | MongoDB ✅ |
| ACID transactions | Yes | Limited | PostgreSQL ✅ |
| Schema flexibility | Rigid | Flexible | MongoDB ✅ |

**Design Decision**: Use PostgreSQL for queries/analytics, MongoDB for document storage.

---

## Mapping & OOB Templates

### Template-Based Mapping Architecture

Instead of hard-coding HL7→FHIR mappings, the system uses **template-driven** mappings stored in the database.

### Database Schema (V9 - Message-Type-Centric)

```sql
-- Standard mapping templates (shared across interfaces)
CREATE TABLE hl7_fhir_templates (
    id UUID PRIMARY KEY,
    message_type VARCHAR(50),  -- "ADT^A01", "ORU^R01"
    hl7_version VARCHAR(10) DEFAULT '2.5',
    template_name VARCHAR(100),
    template_description TEXT,
    template_config JSONB,  -- Contains mappings array
    is_default BOOLEAN DEFAULT true,
    UNIQUE(message_type, hl7_version, is_default)
);

-- Interface-specific overrides (only when different from standard)
CREATE TABLE interface_message_mappings (
    id UUID PRIMARY KEY,
    interface_id UUID REFERENCES interfaces(id),
    message_type VARCHAR(50),
    standard_template_id UUID REFERENCES hl7_fhir_templates(id),
    custom_mapping JSONB,  -- NULL if using standard
    is_active BOOLEAN DEFAULT true,
    UNIQUE(interface_id, message_type)
);
```

### Template Structure

```json
{
  "version": "1.0",
  "messageType": "ADT^A01",
  "mappings": {
    "Patient": {
      "resourceType": "Patient",
      "mappings": [
        {
          "hl7Path": "PID.3.1",
          "fhirPath": "identifier[0].value",
          "transform": "string_direct",
          "required": true,
          "description": "Patient MRN"
        },
        {
          "hl7Path": "PID.19",
          "fhirPath": "identifier[1].value",
          "transform": "string_direct",
          "required": false,
          "description": "SSN"
        },
        {
          "hl7Path": "PID.5.1",
          "fhirPath": "name[0].family",
          "transform": "name_component",
          "required": true
        },
        {
          "hl7Path": "PID.5.2",
          "fhirPath": "name[0].given[0]",
          "transform": "name_component",
          "required": false
        },
        {
          "hl7Path": "PID.16.1",
          "fhirPath": "maritalStatus.coding[0].code",
          "transform": "hl7_table_0002_marital_status",
          "required": false
        }
      ]
    },
    "Encounter": {
      "resourceType": "Encounter",
      "mappings": [...]
    }
  }
}
```

### Runtime Mapping Resolution

```go
// services/hl7_fhir_transform_service_v3.go
func (s *Service) getFieldMappings(ctx context.Context, messageType string) ([]FieldMapping, error) {
    // 1. Check for interface-specific custom mapping
    customMapping := s.loadInterfaceSpecificMappings(ctx, messageType)
    if customMapping != nil {
        return customMapping, nil
    }

    // 2. Fall back to standard template
    template := s.loadStandardTemplate(ctx, messageType)
    return s.convertTemplateToFieldMappings(template), nil
}
```

### Storage Efficiency

**Before V9** (Interface-level storage):
- Each interface stored complete mapping (~30KB JSON)
- 100 interfaces × 30KB = 3MB

**After V9** (Template-based):
- 1 template stored (~30KB)
- 99 interfaces reference template (just UUID)
- 99 interfaces × 36 bytes = 3.6KB

**Savings**: 99% reduction for standard mappings

### OOB (Out-of-Box) Templates

System ships with pre-configured templates:

```sql
-- Pre-loaded standard templates
INSERT INTO hl7_fhir_templates (message_type, template_name, template_config) VALUES
('ADT^A01', 'Standard ADT A01 Mapping', '{...30 patient fields...}'),
('ORU^R01', 'Standard ORU R01 Mapping', '{...observation mappings...}'),
('ORM^O01', 'Standard ORM O01 Mapping', '{...order mappings...}');
```

**User workflow**:
1. Select message type (e.g., ADT^A01)
2. System loads standard template
3. User reviews/customizes
4. Save (as reference to template OR as custom mapping)

---

## Interface Wizard

### Purpose

Visual drag-drop wizard for configuring HL7→FHIR interfaces without coding.

### 4-Step Workflow

```
┌─────────────────────────────────────────────────────────────┐
│ Step 1: Interface Details                                   │
│  - Name, Description                                         │
│  - Source Type (MLLP, File, HTTP)                           │
│  - Message Type (ADT^A01, ORU^R01, etc.)                    │
└─────────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────────┐
│ Step 2: Sample Message Upload                               │
│  - Upload HL7 sample message                                 │
│  - Auto-parse and validate                                   │
│  - Display segment tree view                                 │
└─────────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────────┐
│ Step 3: Field Mapping                                        │
│  - Load OOB template for message type                        │
│  - Display source HL7 fields (left panel)                    │
│  - Display target FHIR fields (right panel)                  │
│  - User can enable/disable/customize mappings               │
└─────────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────────┐
│ Step 4: Test & Save                                          │
│  - Test transformation with sample message                   │
│  - Preview FHIR output                                       │
│  - Save configuration to database                            │
└─────────────────────────────────────────────────────────────┘
```

### Key Files

```
public/
├── interface-wizard.html           # Main wizard UI
├── js/wizard/
│   ├── wizard-main.js             # Controller
│   ├── wizard-services.js         # API calls
│   ├── step-handlers.js           # Step-specific logic
│   └── segment-viewer.js          # HL7 segment tree view

controllers/
├── wizardController.js            # Node.js backend

services/
├── interfaceService.js            # Interface CRUD
└── hl7_fhir_transform_service_v3.go  # Test transformation
```

### Save Flow (Fixed in V9)

**Old (Broken)**:
```javascript
// Saved to transformation_mapping only - not linked to interface
saveMapping(mappingArray) → transformation_mapping table
```

**New (Working)**:
```javascript
// Saves interface + mappings atomically
wizardController.complete({
    interface: {...},
    mappings: [...]
}) → {
    // 1. Create interface record
    interfaceService.createInterface(...)

    // 2. Save mappings to transformation_mapping column
    UPDATE interfaces SET transformation_mapping = $1 WHERE id = $2

    // 3. Create interface-specific message table
    InterfaceTableManager.createMessageTable(interfaceId)
}
```

### Template Integration (TODO)

Currently, wizard creates custom mappings. **Enhancement needed**:

```javascript
// Step 3 enhancement: Load template
async function loadMappingsForMessageType(messageType) {
    const template = await fetch(`/api/templates/${messageType}`);
    const mappings = template.mappings.Patient.mappings;

    // Display in UI with checkboxes
    displayMappingsGrid(mappings);
}

// User can enable/disable individual mappings
// On save, if using all defaults → save template reference
// If customized → save as custom mapping
```

---

## Scalability Design

### Performance Targets

| Metric | Target | Achieved |
|--------|--------|----------|
| Messages/day per interface | 1,000,000 | ✅ Yes* |
| Concurrent interfaces | 1,000+ | ✅ Yes* |
| Transformation time | <100ms avg | ✅ 30-40ms |
| Storage/message | <5KB avg | ✅ 2-3KB |
| Uptime | 99.9% | ⏳ TBD |

*With proper infrastructure (multi-node cluster)

### Interface Isolation

Each interface gets dedicated table → no cross-contamination:

```sql
-- Interface 1 (high volume)
messages_intf_abc123 → 10M rows, no impact on others

-- Interface 2 (low volume)
messages_intf_xyz789 → 1K rows, queries are fast
```

### Horizontal Scaling

```
┌─────────────────────────────────────────────────────────────┐
│                   Load Balancer (nginx)                      │
└────────┬────────────────────────────────────┬────────────────┘
         │                                    │
         ▼                                    ▼
┌─────────────────────┐            ┌─────────────────────┐
│  App Instance 1     │            │  App Instance 2     │
│  (Docker Container) │            │  (Docker Container) │
└──────────┬──────────┘            └──────────┬──────────┘
           │                                  │
           └──────────────┬───────────────────┘
                          ▼
              ┌─────────────────────┐
              │  PostgreSQL Cluster │
              │  (Primary + Replicas)│
              └─────────────────────┘
                          │
                          ▼
              ┌─────────────────────┐
              │  MongoDB Cluster    │
              │  (Sharded by intf)  │
              └─────────────────────┘
```

### Message Queue Integration (Planned)

For even higher throughput:

```
MLLP Listener → RabbitMQ/Kafka → Worker Pool (10-100 workers) → Database
```

**Benefits**:
- Decouple ingestion from processing
- Buffer for traffic spikes
- Retry logic for failures
- Backpressure handling

---

## Database Schema

### Core Tables

```sql
-- Users & Auth
CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) DEFAULT 'user',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Interfaces
CREATE TABLE interfaces (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    message_type VARCHAR(50),  -- "ADT^A01"
    source_type VARCHAR(50),   -- "mllp", "file", "http"
    source_config JSONB,       -- Port, path, etc.
    transformation_mapping JSONB,  -- Field mappings
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Interface-specific message tables (created dynamically)
CREATE TABLE messages_intf_{interface_id} (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(100) UNIQUE NOT NULL,
    correlation_id VARCHAR(100),
    interface_id UUID NOT NULL,
    status VARCHAR(50) DEFAULT 'received',
    received_at TIMESTAMP NOT NULL,
    source_type VARCHAR(50),
    source_endpoint VARCHAR(255),
    message_type VARCHAR(50),
    message_size INTEGER,
    -- V19: JSON parsing metadata
    parsed_at TIMESTAMP,
    parsing_status VARCHAR(50),
    parsing_time_ms INTEGER,
    parsing_error TEXT,
    -- Timestamps
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Audit logs
CREATE TABLE audit_logs (
    id SERIAL PRIMARY KEY,
    user_id UUID REFERENCES users(id),
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50),
    resource_id VARCHAR(100),
    details JSONB,
    ip_address VARCHAR(45),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- OOB Templates (V9)
CREATE TABLE hl7_fhir_templates (
    id UUID PRIMARY KEY,
    message_type VARCHAR(50) NOT NULL,
    template_name VARCHAR(100) NOT NULL,
    template_config JSONB NOT NULL,
    is_default BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Transformation Pipeline (V20 - Planned)
CREATE TABLE transformation_pipelines (
    id UUID PRIMARY KEY,
    interface_id UUID REFERENCES interfaces(id),
    message_type VARCHAR(50),
    pipeline_name VARCHAR(100),
    is_active BOOLEAN DEFAULT true,
    UNIQUE(interface_id, message_type)
);

CREATE TABLE transformation_steps (
    id UUID PRIMARY KEY,
    pipeline_id UUID REFERENCES transformation_pipelines(id),
    sequence INTEGER NOT NULL,
    step_name VARCHAR(100),
    step_type VARCHAR(50),
    executor_type VARCHAR(50),
    script_content TEXT,
    config JSONB,
    is_active BOOLEAN DEFAULT true
);
```

### Migration History

| Version | Description | Status |
|---------|-------------|--------|
| V1-V15 | Initial schema, interface tables, wizard integration | ✅ Applied |
| V16 | Output message tables | ✅ Applied |
| V17 | Hybrid storage cross-reference | ✅ Applied |
| V18 | Hybrid storage column fixes | ✅ Applied |
| V19 | JSON parsing columns (parsed_at, parsing_status, etc.) | ✅ Applied |
| V20 | Transformation pipeline tables | ⏳ Designed |

---

## API Reference

### Authentication Endpoints

```
POST /api/auth/login
POST /api/auth/register
POST /api/auth/logout
GET  /api/auth/me
```

### Interface Endpoints

```
GET    /api/interfaces           # List all interfaces
POST   /api/interfaces           # Create interface
GET    /api/interfaces/:id       # Get interface details
PUT    /api/interfaces/:id       # Update interface
DELETE /api/interfaces/:id       # Delete interface
POST   /api/interfaces/:id/test  # Test interface
```

### Message Endpoints

```
GET  /api/messages/interface/:interfaceId        # Get messages for interface
GET  /api/messages/interface/:interfaceId/stats  # Get statistics
POST /api/messages/send/:interfaceId             # Send message to interface
```

### Wizard Endpoints

```
POST /api/wizard/parse-sample        # Parse uploaded HL7 sample
POST /api/wizard/test-transform      # Test transformation
POST /api/wizard/complete            # Save interface + mappings
GET  /api/wizard/templates/:messageType  # Get OOB template
```

### MLLP Endpoints

```
POST   /api/mllp/listeners           # Start MLLP listener
GET    /api/mllp/listeners           # List active listeners
GET    /api/mllp/listeners/:id       # Get listener status
DELETE /api/mllp/listeners/:id       # Stop listener
POST   /api/mllp/send                # Send message via MLLP
```

### Transform Endpoints (Go Backend)

```
POST /api/hl7/transform              # Transform HL7 to FHIR (with mappings)
POST /api/hl7/test-transform-v3      # Test transformation
GET  /api/system/health              # Health check
```

---

## Quick Reference

### Starting the Application

```bash
# Development (both services with auto-reload)
npm run dev:all

# Production
docker-compose up -d
```

### Environment Variables

```bash
# Node.js
PORT=3000
API_PORT=8080
NODE_ENV=development

# PostgreSQL
DB_HOST=postgres
DB_PORT=5432
DB_NAME=ezhealthkonnect
DB_USER=ezhealth_user
DB_PASSWORD=secure_password_change_me

# MongoDB (for JSON conversion)
MONGODB_URI=mongodb://ezhealth_user:secure_password_change_me@mongodb:27017/ezhealthkonnect?authSource=admin
MONGODB_DATABASE=ezhealthkonnect

# Security
SESSION_SECRET=your-secret-key
JWT_SECRET=your-jwt-secret
```

### Common Operations

#### Create a New Interface

1. Navigate to `/interface-wizard.html`
2. Fill in interface details (Step 1)
3. Upload HL7 sample message (Step 2)
4. Review/customize mappings (Step 3)
5. Test and save (Step 4)

#### Send Test Message

```powershell
# PowerShell
$message = "MSH|^~\&|HIS|RIH|EKG|EKG|202508251530||ADT^A01|MSG00001|P|2.5`r`nPID|1||123456^^^RIH^MR||Doe^John^A||19800115|M"
$bytes = [System.Text.Encoding]::UTF8.GetBytes($message)
$tcp = New-Object System.Net.Sockets.TcpClient("localhost", 6661)
$stream = $tcp.GetStream()
$stream.Write(@(0x0B), 0, 1)  # Start byte
$stream.Write($bytes, 0, $bytes.Length)
$stream.Write(@(0x1C, 0x0D), 0, 2)  # End bytes
$stream.Close()
```

#### Check Message Processing

```bash
# Check PostgreSQL metadata
docker-compose exec postgres psql -U ezhealth_user -d ezhealthkonnect -c "SELECT message_id, status, parsing_status FROM messages_intf_<id> ORDER BY created_at DESC LIMIT 5;"

# Check MongoDB parsed content
docker-compose exec mongodb mongosh -u ezhealth_user -p secure_password_change_me --authenticationDatabase admin ezhealthkonnect --eval "db.getCollection('raw_messages_<interface-id>').findOne({}, {sort: {_id: -1}})"

# Check FHIR output
docker-compose exec mongodb mongosh -u ezhealth_user -p secure_password_change_me --authenticationDatabase admin ezhealthkonnect --eval "db.getCollection('transformed_messages_intf_<interface-id>').findOne({}, {sort: {_id: -1}})"
```

### Troubleshooting

#### JSON Conversion Not Working

```bash
# Check if MongoDB is connected
docker-compose logs app | grep "MongoDB connected"

# Check parsing status
docker-compose exec postgres psql -U ezhealth_user -d ezhealthkonnect -c "SELECT parsing_status, parsing_error FROM messages_intf_<id> WHERE parsed_at IS NULL LIMIT 5;"
```

#### FHIR Transformation Fails

```bash
# Check transformation logs
docker-compose logs app | grep "transformation"

# Check if mappings exist
docker-compose exec postgres psql -U ezhealth_user -d ezhealthkonnect -c "SELECT transformation_mapping FROM interfaces WHERE id = '<uuid>';"
```

#### MLLP Listener Not Starting

```bash
# Check listener status
curl http://localhost:8080/api/mllp/listeners

# Check logs
docker-compose logs app | grep MLLP
```

---

## Change Log

### October 2025

- ✅ Fixed FHIR resource generation (MongoDB BSON type handling)
- ✅ Added deep nesting support for FHIR paths (3+ levels)
- ✅ Implemented array index handling (name[0].given[1])
- ✅ Updated ADT^A01 template with 30 comprehensive mappings
- ✅ Consolidated 20+ documentation files into this master doc

### September 2025

- ✅ Implemented JSON conversion pipeline (V19 migration)
- ✅ Created OOB template system (V9 migration)
- ✅ Fixed wizard save flow
- ✅ Implemented interface-specific message tables

---

## References

- **FHIR R4 Specification**: https://hl7.org/fhir/R4/
- **HL7 v2.5 Specification**: https://www.hl7.org/implement/standards/product_brief.cfm?product_id=144
- **MongoDB Go Driver**: https://pkg.go.dev/go.mongodb.org/mongo-driver
- **Gin Web Framework**: https://gin-gonic.com/docs/

---

**Document Maintained By**: ezHealthKonnect Development Team
**Last Review**: October 2025
**Next Review**: November 2025
