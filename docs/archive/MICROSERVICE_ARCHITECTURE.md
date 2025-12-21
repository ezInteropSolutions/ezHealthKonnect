# ezHealthKonnect Microservice Architecture

## Overview

ezHealthKonnect follows a **connector-agnostic microservice architecture** where message processing is completely decoupled from connectivity. The same transformation and delivery pipeline works with **ANY** of the 32 supported connectors.

---

## 🏗️ Architecture Pattern

```
┌─────────────────────────────────────────────────────────────────────┐
│                     CONNECTOR LAYER (Input)                          │
│  TCP/MLLP │ HTTP │ File │ Database │ SFTP │ RabbitMQ │ Kafka │ ... │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             ▼
                  ┌──────────────────────┐
                  │  models.InboundMessage│  ← UNIFIED MODEL
                  └──────────┬────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    PROCESSING ENGINE CORE                            │
│                                                                       │
│  1. Store PostgreSQL  →  2. Store MongoDB  →  3. Parse JSON          │
│  4. Execute Pipeline  →  5. Transform      →  6. Generate Delivery   │
└────────────────────────────┬───────────────────────────────────────┘
                             │
                             ▼
                  ┌──────────────────────┐
                  │ models.OutboundMessage│  ← UNIFIED MODEL
                  └──────────┬────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    CONNECTOR LAYER (Output)                          │
│  TCP/MLLP │ HTTP │ File │ Database │ SFTP │ RabbitMQ │ Kafka │ ... │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 📊 Message Flow (Connector-Agnostic)

### Example 1: HL7→FHIR via TCP/MLLP

```
1. TCP Connector receives HL7 via MLLP
   └─> Creates models.InboundMessage

2. Processing Engine
   ├─> Store in PostgreSQL (messages_intf_*)
   ├─> Store in MongoDB (raw_messages_intf_*)
   ├─> Parse to JSON (enhancedSegments)
   ├─> Execute transformation pipeline
   └─> Generate FHIR Bundle + DeliveryPayload

3. HTTP Connector delivers FHIR
   └─> POST to FHIR endpoint
```

### Example 2: HL7→FHIR via File Listener

```
1. File Connector reads HL7 from directory
   └─> Creates models.InboundMessage

2. Processing Engine (SAME AS ABOVE)
   ├─> Store in PostgreSQL
   ├─> Store in MongoDB
   ├─> Parse to JSON
   ├─> Execute transformation pipeline
   └─> Generate FHIR Bundle + DeliveryPayload

3. HTTP Connector delivers FHIR (SAME AS ABOVE)
   └─> POST to FHIR endpoint
```

### Example 3: HL7→FHIR via Database Query

```
1. PostgreSQL Connector polls database table
   └─> Creates models.InboundMessage

2. Processing Engine (SAME AS ABOVE)
   ├─> Store in interface table
   ├─> Store in MongoDB
   ├─> Parse to JSON
   ├─> Execute transformation pipeline
   └─> Generate FHIR Bundle + DeliveryPayload

3. File Connector writes FHIR to file
   └─> Write to output directory
```

---

## 🔑 Key Principle: **Unified Message Model**

All connectors convert their specific format to `models.InboundMessage`:

```go
type InboundMessage struct {
    MessageID       string            // Unique identifier
    CorrelationID   string            // For tracking
    Content         []byte            // Raw message content
    ContentType     string            // MIME type
    SourceType      string            // tcp_mllp, file_listener, postgresql_inbound, etc.
    SourceEndpoint  string            // Source identifier
    SourceIP        string            // Client IP (if applicable)
    MessageType     string            // Detected message type
    ReceivedAt      time.Time         // Timestamp
    Headers         map[string]string // Connector-specific metadata
}
```

**Result:** The processing engine doesn't care about connectivity!

---

## 🎯 Transformation Pipeline (Connector-Independent)

### Current Implementation (V3 - Atomic Mapping)

Located in: [processing/engine_message_processor.go:267-520](processing/engine_message_processor.go#L267-L520)

```go
func (pe *ProcessingEngine) executeTransformationPipeline(
    ctx context.Context,
    interfaceID string,
    messageID string,
    messageType string,
    parsedResult *services.ParseResult,
) {
    // 1. Load transformation configuration (from database)
    mappingConfig := pe.loadMappingConfig(interfaceID, messageType)

    // 2. Execute HL7→FHIR transformation (format-specific)
    fhirBundle := pe.transformHL7ToFHIR(parsedResult, mappingConfig)

    // 3. Generate delivery payload (connector-agnostic)
    deliveryPayload := pe.createDeliveryPayload(fhirBundle, interfaceID, messageID)

    // 4. Store transformed output in MongoDB
    pe.storeTransformedOutput(deliveryPayload)

    // 5. Deliver via outbound connector (ANY connector type)
    go pe.deliverMessage(ctx, interfaceID, messageID, deliveryPayload)
}
```

**Key Points:**
- ✅ Doesn't know or care about input connectivity
- ✅ Doesn't know or care about output connectivity
- ✅ Configuration-driven (OOB pattern)
- ✅ Format transformation is separate from delivery

---

## 🔌 Connector Factory Pattern (OOB)

All 32 connectors are registered in the factory:

```go
// services/connectors/connector_factory.go
func (f *DefaultConnectorFactory) registerBuiltInConnectors() {
    // Network
    f.RegisterInbound("tcp_mllp_inbound", NewTCPMLLPInboundConnector)
    f.RegisterInbound("http_rest_inbound", NewHTTPFHIRInboundConnector)
    f.RegisterOutbound("http_outbound", NewHTTPOutboundConnector)

    // File System
    f.RegisterInbound("file_listener", NewFileListenerConnector)
    f.RegisterOutbound("file_writer", NewFileWriterConnector)

    // Database (10 connectors)
    f.RegisterInbound("postgresql_inbound", NewPostgreSQLInboundConnector)
    f.RegisterOutbound("postgresql_outbound", NewPostgreSQLOutboundConnector)
    f.RegisterInbound("mysql_inbound", NewMySQLInboundConnector)
    // ... etc

    // Message Queues (6 connectors)
    f.RegisterInbound("rabbitmq_inbound", NewRabbitMQInboundConnector)
    f.RegisterOutbound("rabbitmq_outbound", NewRabbitMQOutboundConnector)
    // ... etc

    // Cloud Storage (6 connectors)
    f.RegisterInbound("aws_s3_inbound", NewAWSS3InboundConnector)
    f.RegisterOutbound("aws_s3_outbound", NewAWSS3OutboundConnector)
    // ... etc
}
```

**Usage in Engine:**

```go
// Create inbound connector (source)
connector, err := pe.connectorFactory.CreateInbound("file_listener")
connector.Initialize(configJSON)
connector.Start(ctx, messageChan)

// Create outbound connector (target)
connector, err := pe.connectorFactory.CreateOutbound("http_outbound")
connector.Initialize(configJSON)
connector.Send(ctx, outboundMessage)
```

---

## 📝 Configuration Storage (Database-Driven)

### Interfaces Table

```sql
CREATE TABLE interfaces (
    id UUID PRIMARY KEY,
    name VARCHAR(255),

    -- SOURCE CONNECTIVITY (OOB)
    source_type VARCHAR(50),              -- 'hl7v2', 'fhir', 'json', etc.
    source_connectivity JSONB,            -- { "type": "file_listener", "config": {...} }

    -- TARGET CONNECTIVITY (OOB)
    target_type VARCHAR(50),              -- 'fhir', 'hl7v2', 'json', etc.
    target_connectivity JSONB,            -- { "type": "http_outbound", "config": {...} }

    -- TRANSFORMATION CONFIG (OOB)
    transformation_flow VARCHAR(50),      -- 'hl7_to_fhir', 'passthrough', etc.
    transformation_mapping JSONB          -- Mapping configuration
);
```

### Example: File → HL7→FHIR → HTTP

```json
{
  "source_connectivity": {
    "type": "file_listener",
    "config": {
      "directory_path": "/data/hl7/incoming",
      "file_pattern": "*.hl7",
      "polling_interval": 10,
      "after_processing": "archive",
      "archive_path": "/data/hl7/archive"
    }
  },
  "target_connectivity": {
    "type": "http_outbound",
    "config": {
      "endpoint": "https://fhir.hospital.com/api/r4",
      "authType": "bearer",
      "bearerToken": "xyz123..."
    }
  }
}
```

**The transformation pipeline is IDENTICAL** whether source is TCP, File, Database, or Queue!

---

## ✅ MVC + OOB Compliance

### Model (Data Layer)
- `models.InboundMessage` - Unified inbound model
- `models.OutboundMessage` - Unified outbound model
- `models.DeliveryPayload` - Transformation output
- Database tables for persistence

### View (Presentation Layer)
- `public/js/components/InterfaceConfigComponents.js`
- Dynamic UI based on connectivity type
- Same components for wizard and edit interface

### Controller (Business Logic)
- `processing/engine.go` - Processing engine controller
- `processing/engine_message_processor.go` - Message processing
- `services/connectors/*` - Connector implementations
- `services/hl7_fhir_transform_service_v3.go` - Transformation logic

### OOB (Out-of-Box) Pattern
- ✅ Configuration-driven (no hardcoded values)
- ✅ Factory pattern for connectors
- ✅ Metadata-based capabilities
- ✅ Database-driven behavior

---

## 🧪 Testing Any Connectivity

### Test Scenario 1: TCP/MLLP → FHIR → HTTP
```bash
# Send HL7 via TCP
echo -e '\x0BMSH|...\x1C\x0D' | nc localhost 6664

# Result:
# 1. TCP connector receives
# 2. Stored in PostgreSQL + MongoDB
# 3. Parsed to JSON
# 4. Transformed to FHIR
# 5. Delivered via HTTP POST
```

### Test Scenario 2: File → FHIR → File
```bash
# Drop HL7 file
cp message.hl7 /data/hl7/incoming/

# Result:
# 1. File connector picks up
# 2. Stored in PostgreSQL + MongoDB
# 3. Parsed to JSON
# 4. Transformed to FHIR
# 5. Written to output file
```

### Test Scenario 3: Database → FHIR → Kafka
```sql
-- Insert HL7 into source table
INSERT INTO hl7_messages (content) VALUES ('MSH|...');

-- Result:
-- 1. PostgreSQL connector polls
-- 2. Stored in interface table + MongoDB
-- 3. Parsed to JSON
-- 4. Transformed to FHIR
-- 5. Published to Kafka topic
```

---

## 🚀 Implemented Connectors (As of Nov 2025)

### ✅ Fully Implemented (Tested)
1. **TCP/MLLP Inbound** - HL7 v2.x receiver
2. **HTTP FHIR Inbound** - FHIR REST endpoint
3. **HTTP Outbound** - REST API delivery
4. **File Listener** - Directory polling
5. **File Writer** - File system output

### 🔄 Stub Implementations (Ready for Implementation)
- 27 additional connectors registered in factory
- All following the same interface pattern
- Can be implemented incrementally

---

## 🎯 Key Advantages

### 1. **Connector Independence**
- Add new connectors without touching transformation logic
- Same transformation works with any input/output combination

### 2. **Scalability**
- Each interface runs in its own goroutine
- Connectors are stateless and thread-safe
- Can scale horizontally

### 3. **Testability**
- Test transformation independently of connectivity
- Mock connectors for unit testing
- End-to-end tests with any connector combination

### 4. **Maintainability**
- Single transformation codebase
- Connector bugs don't affect transformation
- Easy to add new message formats

### 5. **Flexibility**
- Users choose any input/output combination
- 32 inbound × 32 outbound = 1,024 possible configurations!
- All controlled via UI wizard

---

## 📚 Related Documentation

- [CONNECTIVITY_CATALOG.md](CONNECTIVITY_CATALOG.md) - Complete connector catalog
- [CONNECTOR_IMPLEMENTATION_GUIDE.md](CONNECTOR_IMPLEMENTATION_GUIDE.md) - How to implement new connectors
- [TRANSFORMATION_PIPELINE_DESIGN.md](TRANSFORMATION_PIPELINE_DESIGN.md) - Transformation architecture
- [JSON_CONVERSION_ARCHITECTURE.md](JSON_CONVERSION_ARCHITECTURE.md) - Parsing pipeline
- [ARCHITECTURE_REFERENCE.md](ARCHITECTURE_REFERENCE.md) - Design patterns

---

## 🎓 Summary

**The core insight:**

> **Connectivity is orthogonal to transformation.**

Whether a message arrives via TCP, File, Database, or Queue, the transformation pipeline doesn't change. Whether output goes to HTTP, File, Database, or Kafka, the transformation pipeline doesn't change.

This microservice architecture allows unlimited flexibility while maintaining a single, well-tested transformation codebase.
