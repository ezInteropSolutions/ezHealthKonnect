# JSON Conversion Architecture - Final Implementation

## Overview

Automatic JSON conversion pipeline that converts all incoming messages to JSON as the first transformation step. The system follows MVC and OOB design principles, with full integration into the existing HL7 processing engine.

**Status**: ✅ Production Ready
**Last Updated**: 2025-10-02
**Migration**: V19 (Applied and tracked via Flyway)

---

## Table of Contents

1. [Architecture Principles](#architecture-principles)
2. [System Components](#system-components)
3. [Data Flow](#data-flow)
4. [Storage Architecture](#storage-architecture)
5. [Code Structure](#code-structure)
6. [Deployment](#deployment)
7. [Testing](#testing)
8. [Future Enhancements](#future-enhancements)

---

## Architecture Principles

### MVC Pattern Compliance

```
Models:       models/parser_models.go
              - MessageFormat enum
              - ParserResult struct
              - ParserMetadata struct
              - ValidationResult struct
              - FormatDetectionResult struct

Services:     services/format_detector.go
              services/parser_factory.go
              services/message_parser_service.go
              services/parsers/hl7_parser_service.go
              services/mongodb_message_service.go (updated)
              services/interface_message_service.go (updated)

Controllers:  processing/engine.go (orchestration only)
              - Triggers async JSON conversion
              - No business logic in controller
```

### OOB (Out-of-Box) Pattern

- ✅ Auto-detect MongoDB from environment variables
- ✅ Auto-detect message format from content
- ✅ Auto-register parsers on startup
- ✅ Auto-initialize parser service when MongoDB available
- ✅ Zero manual configuration required

### Code Reuse

- ✅ 100% reuse of existing `hl7.ParseWithRealSchema()`
- ✅ ~200 lines of adapter code wraps 2000+ lines of parser
- ✅ No code duplication
- ✅ Preserves full enhanced schema structure

---

## System Components

### 1. Models Layer (`models/parser_models.go`)

**Purpose**: Define data structures for parser system

```go
type MessageFormat string
const (
    FormatHL7v2    MessageFormat = "hl7v2"
    FormatHL7v3    MessageFormat = "hl7v3"
    FormatFHIR     MessageFormat = "fhir"
    FormatXML      MessageFormat = "xml"
    FormatJSON     MessageFormat = "json"
    FormatEDI      MessageFormat = "edi"
    FormatCSV      MessageFormat = "csv"
    FormatUnknown  MessageFormat = "unknown"
)

type ParserResult struct {
    Success          bool
    Format           MessageFormat
    ParsedJSON       map[string]interface{}  // Full enhanced schema
    Metadata         ParserMetadata
    ValidationResult ValidationResult
    Error            string
    ParsingTime      time.Duration
}
```

### 2. Format Detection (`services/format_detector.go`)

**Purpose**: Automatic message format detection

**Features**:
- Signature-based detection (MSH| for HL7, <?xml for XML, etc.)
- Confidence scoring
- Detection indicators for debugging

**Example**:
```go
detector := NewFormatDetector()
result := detector.DetectFormat(rawContent)
// result.DetectedFormat = "hl7v2"
// result.Confidence = 0.95
```

### 3. Parser Factory (`services/parser_factory.go`)

**Purpose**: Registry pattern for parser management

**Features**:
- Auto-registration of all parsers
- Dynamic parser selection
- Extensible for future formats

**Example**:
```go
factory := NewParserFactory()
parser, err := factory.GetParser(models.FormatHL7v2)
```

### 4. HL7 Parser Service (`services/parsers/hl7_parser_service.go`)

**Purpose**: Adapter wrapping existing HL7 parser

**Key Implementation**:
```go
func (hp *HL7ParserService) Parse(rawContent string) (*models.ParserResult, error) {
    // ✅ REUSE EXISTING PARSER
    enhancedResult := hl7.ParseWithRealSchema(rawContent)

    // ✅ PRESERVE FULL ENHANCED SCHEMA
    parsedJSON := convertToStandardJSON(enhancedResult)

    return &models.ParserResult{
        Success:    true,
        Format:     models.FormatHL7v2,
        ParsedJSON: parsedJSON,  // Contains full enhanced schema
        Metadata:   metadata,
        // ...
    }, nil
}
```

**Enhanced Schema Structure Preserved**:
- `enhancedSegments`: Full segment metadata with schema info
- `segmentOrder`: Segment ordering
- `basicSegments`: Basic parsing (if available)
- `validationErrors`: Schema validation results
- `dictionaryUsed`: Whether schema dictionary was loaded
- `schemaLoaded`: Whether schema was successfully loaded

### 5. Message Parser Service (`services/message_parser_service.go`)

**Purpose**: Main orchestration service

**Workflow**:
```
1. Auto-detect format → FormatDetector
2. Get parser        → ParserFactory
3. Parse to JSON     → HL7ParserService
4. Store in MongoDB  → MongoDBMessageService.UpdateParsedContent()
5. Update PostgreSQL → InterfaceMessageService.UpdateMessageParsingStatus()
```

**Example**:
```go
parserService := NewMessageParserService(mongoService, postgresService)
result, err := parserService.ParseToJSON(ctx, messageID, interfaceID, rawContent)
```

### 6. Storage Services

#### MongoDB Service (`services/mongodb_message_service.go`)

**Added Methods**:
```go
type ParsedContentUpdate struct {
    ParsedContent    map[string]interface{}  // Full enhanced schema
    ParsedAt         time.Time
    ParsingTimeMs    int
    Format           string
    MessageType      string
    ValidationResult interface{}
}

func (mms *MongoDBMessageService) UpdateParsedContent(
    ctx context.Context,
    interfaceID string,
    messageID string,
    update *ParsedContentUpdate,
) error
```

#### PostgreSQL Service (`services/interface_message_service.go`)

**Added Methods**:
```go
type MessageStatusUpdate struct {
    Status         string
    MessageType    string
    ParsedAt       time.Time
    ParsingStatus  string
    ParsingTimeMs  int
    ParsingError   string
}

func (ims *InterfaceMessageService) UpdateMessageParsingStatus(
    interfaceID string,
    messageID string,
    update *MessageStatusUpdate,
) error
```

### 7. Processing Engine Integration (`processing/engine.go`)

**Auto-Detection and Initialization**:
```go
func NewProcessingEngine(db *sql.DB) *ProcessingEngine {
    // OOB: Auto-detect MongoDB
    mongoService, err := services.NewMongoDBConnectionService()

    if err == nil {
        // Hybrid storage with parser service
        return createHybridStorageEngine(db, mongoService)
    }

    // Fallback to PostgreSQL-only
    return createPostgreSQLOnlyEngine(db)
}

func createHybridStorageEngine(db *sql.DB, mongoService *services.MongoDBConnectionService) *ProcessingEngine {
    // Create parser service
    mongoMessageService := services.NewMongoDBMessageService(mongoClient, mongoDatabase)
    postgresMessageService := services.NewInterfaceMessageService(db)
    parserService := services.NewMessageParserService(mongoMessageService, postgresMessageService)

    return &ProcessingEngine{
        // ... other fields
        parserService: parserService,  // ✅ Parser service initialized
    }
}
```

**Async JSON Conversion Trigger**:
```go
func (pe *ProcessingEngine) handleMessage(msg Message, interfaceID string) {
    // Store raw message
    err := pe.hybridStorage.StoreMessage(ctx, &messageData)

    // Trigger async JSON conversion
    if pe.parserService != nil {
        go pe.convertToJSON(msg.ID, interfaceID, msg.Content)
    }
}

func (pe *ProcessingEngine) convertToJSON(messageID string, interfaceID string, rawContent string) {
    ctx := context.Background()

    fmt.Printf("🔄 Triggering JSON conversion for message: %s\n", messageID)

    result, err := pe.parserService.ParseToJSON(ctx, messageID, interfaceID, rawContent)

    if result.Success {
        fmt.Printf("✅ JSON conversion completed for %s (format: %s, took %dms)\n",
            messageID, result.Format, result.ParsingTime.Milliseconds())
    }
}
```

---

## Data Flow

### Complete Message Processing Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. MESSAGE INGESTION                                            │
│    - TCP/HTTP Connector receives HL7 message                    │
│    - Connector sends to ProcessingEngine.messageChan            │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. RAW MESSAGE STORAGE                                          │
│    - ProcessingEngine.handleMessage() called                    │
│    - HybridMessageStorage.StoreMessage()                        │
│      ├─ PostgreSQL: Metadata (status, timestamps, etc.)         │
│      └─ MongoDB: Raw content (full HL7 message)                 │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. ASYNC JSON CONVERSION TRIGGER                                │
│    - go pe.convertToJSON(messageID, interfaceID, rawContent)    │
│    - Non-blocking goroutine execution                           │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. FORMAT DETECTION                                             │
│    - FormatDetector.DetectFormat(rawContent)                    │
│    - Returns: format="hl7v2", confidence=0.95                   │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ 5. PARSER SELECTION                                             │
│    - ParserFactory.GetParser(format)                            │
│    - Returns: HL7ParserService                                  │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ 6. HL7 PARSING (REUSE EXISTING PARSER)                          │
│    - HL7ParserService.Parse(rawContent)                         │
│    - Calls: hl7.ParseWithRealSchema(rawContent)                 │
│    - Returns: EnhancedParsedMessage with full schema            │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ 7. ENHANCED SCHEMA CONVERSION                                   │
│    - convertToStandardJSON(enhancedResult)                      │
│    - Preserves FULL enhanced schema structure:                  │
│      ├─ enhancedSegments (with field metadata)                  │
│      ├─ segmentOrder                                            │
│      ├─ basicSegments                                           │
│      ├─ validationErrors                                        │
│      ├─ dictionaryUsed                                          │
│      └─ schemaLoaded                                            │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ 8. STORE PARSED JSON IN MONGODB                                 │
│    - MongoDBMessageService.UpdateParsedContent()                │
│    - Collection: raw_messages_intf_<interface-id>               │
│    - Document fields updated:                                   │
│      ├─ parsed_content: {full enhanced schema}                  │
│      ├─ parsed_at: timestamp                                    │
│      ├─ parsing_time_ms: performance metric                     │
│      ├─ parsed_format: "hl7v2"                                  │
│      ├─ message_type: "ADT^A01"                                 │
│      └─ validation_result: validation errors/warnings           │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ 9. UPDATE PARSING STATUS IN POSTGRESQL                          │
│    - InterfaceMessageService.UpdateMessageParsingStatus()       │
│    - Table: messages_intf_<interface-id>                        │
│    - Columns updated:                                           │
│      ├─ parsing_status: "success"                               │
│      ├─ parsed_at: timestamp                                    │
│      ├─ parsing_time_ms: performance metric                     │
│      └─ parsing_error: null (or error message if failed)        │
└────────────────────┬────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│ 10. READY FOR NEXT TRANSFORMATION LAYER                         │
│     - Global transformations (cross-interface)                  │
│     - Interface-specific transformations (custom)               │
│     - User-defined transformation sequence                      │
└─────────────────────────────────────────────────────────────────┘
```

---

## Storage Architecture

### Multi-Stage Storage Design

```
Stage 1: RAW MESSAGE
├─ PostgreSQL (messages_intf_<id>)
│  ├─ message_id
│  ├─ status: "received"
│  ├─ received_at
│  └─ message_type (basic detection)
│
└─ MongoDB (raw_messages_intf_<id>)
   ├─ message_id
   ├─ raw_content: "MSH|^~\&|..."
   └─ metadata

Stage 2: PARSED JSON (ENHANCED SCHEMA)
├─ PostgreSQL (messages_intf_<id>)
│  ├─ parsing_status: "success"
│  ├─ parsed_at: timestamp
│  ├─ parsing_time_ms: 125
│  └─ parsing_error: null
│
└─ MongoDB (raw_messages_intf_<id>)
   ├─ parsed_content:
   │  ├─ enhancedSegments:
   │  │  ├─ MSH:
   │  │  │  ├─ key: "MSH"
   │  │  │  ├─ name: "Message Header"
   │  │  │  ├─ description: "..."
   │  │  │  ├─ fields: [
   │  │  │  │  {
   │  │  │  │    key: "MSH.3",
   │  │  │  │    name: "Sending Application",
   │  │  │  │    value: "...",
   │  │  │  │    position: 3,
   │  │  │  │    dataType: "HD",
   │  │  │  │    description: "...",
   │  │  │  │    subfields: [...]
   │  │  │  │  }
   │  │  │  │]
   │  │  ├─ PID: {...}
   │  │  └─ PV1: {...}
   │  ├─ segmentOrder: ["MSH", "PID", "PV1"]
   │  ├─ messageType: {...}
   │  ├─ version: "2.5"
   │  ├─ dictionaryUsed: true
   │  ├─ schemaLoaded: true
   │  └─ validationErrors: []
   │
   ├─ parsed_at: ISODate("...")
   ├─ parsing_time_ms: 125
   └─ parsed_format: "hl7v2"

Stage 3: TRANSFORMED MESSAGE (Future)
├─ PostgreSQL (messages_intf_<id>)
│  ├─ transformation_status
│  ├─ transformed_at
│  └─ transformation_time_ms
│
└─ MongoDB (transformed_messages_intf_<id>)
   ├─ transformed_content: {...}
   └─ transformation_config: {...}
```

### MongoDB Document Structure

**Collection**: `raw_messages_intf_<interface-id>`

```javascript
{
  _id: ObjectId("..."),
  message_id: "tcp_1759344557381597526",
  interface_id: "629ac1e8-0c50-447a-b93f-ebfc15830a7d",
  correlation_id: "MSG00001",

  // RAW MESSAGE
  message_type: "ADT^A01",
  source_type: "tcp",
  source_endpoint: "localhost:6661",
  source_ip: "192.168.1.100",
  received_at: ISODate("2025-10-02T19:30:00Z"),
  message_size: 512,
  message_encoding: "UTF-8",
  raw_content: "MSH|^~\\&|SENDING_APP|SENDING_FAC|...",

  // PARSED JSON (FULL ENHANCED SCHEMA)
  parsed_content: {
    raw: "MSH|^~\\&|SENDING_APP|...",
    success: true,
    error: "",
    version: "2.5",

    messageType: {
      code: "ADT",
      event: "A01",
      name: "ADT^A01",
      description: "Admit/visit notification",
      structure: "ADT_A01"
    },

    enhancedSegments: {
      "MSH": {
        key: "MSH",
        raw: "MSH|^~\\&|SENDING_APP|...",
        name: "Message Header",
        description: "The MSH segment defines the intent, source, destination...",
        purpose: "Message identification and routing",
        fields: [
          {
            key: "MSH.1",
            value: "|",
            name: "Field Separator",
            description: "Field separator character",
            dataType: "ST",
            optionality: "R",
            cardinality: "[1..1]",
            position: 1,
            hasValue: true,
            length: 1,
            tableId: "",
            subfields: [],
            sequence: 0
          },
          {
            key: "MSH.3",
            value: "SENDING_APP",
            name: "Sending Application",
            description: "Uniquely identifies the sending application",
            dataType: "HD",
            optionality: "O",
            cardinality: "[0..1]",
            position: 3,
            hasValue: true,
            subfields: [
              {
                key: "MSH.3.1",
                name: "Namespace ID",
                value: "SENDING_APP",
                dataType: "IS",
                position: 1
              }
            ],
            sequence: 2
          }
          // ... more fields
        ],
        fieldCount: 21,
        dictionarySource: "HL7 v2.5 Standard",
        required: true,
        repeating: false,
        sequence: 0
      },
      "PID": {
        key: "PID",
        name: "Patient Identification",
        description: "The PID segment is used to communicate patient identification...",
        fields: [...]
      },
      "PV1": {...}
    },

    segmentOrder: ["MSH", "EVN", "PID", "PV1"],

    basicSegments: {
      "MSH": {
        raw: "MSH|^~\\&|SENDING_APP|...",
        fields: ["|", "^~\\&", "SENDING_APP", ...]
      }
    },

    parsedAt: "2025-10-02T19:30:00.125Z",
    dictionaryUsed: true,
    schemaLoaded: true,

    validationErrors: [
      {
        severity: "warning",
        field: "PID.8",
        message: "Gender code 'U' is deprecated in v2.5",
        code: "DEPRECATED_VALUE"
      }
    ]
  },

  // PARSING METADATA
  parsed_at: ISODate("2025-10-02T19:30:00.125Z"),
  parsing_time_ms: 125,
  parsed_format: "hl7v2",
  validation_result: {
    isValid: true,
    errors: [],
    warnings: ["Gender code 'U' is deprecated in v2.5"]
  },

  // METADATA
  metadata: {
    parser_version: "1.0.0",
    detected_version: "2.5",
    message_control_id: "MSG00001",
    segment_count: 4,
    field_count: 87
  },

  created_at: ISODate("2025-10-02T19:30:00Z"),
  updated_at: ISODate("2025-10-02T19:30:00.125Z")
}
```

### PostgreSQL Schema

**Table**: `messages_intf_<interface-id>` (created per interface)

```sql
CREATE TABLE messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(255) UNIQUE NOT NULL,
    correlation_id VARCHAR(255),
    interface_id UUID NOT NULL,

    -- Status tracking
    status VARCHAR(50) NOT NULL DEFAULT 'received',
    priority INTEGER DEFAULT 5,

    -- Timestamps
    received_at TIMESTAMP WITH TIME ZONE NOT NULL,
    processing_completed_at TIMESTAMP WITH TIME ZONE,

    -- Message metadata
    source_type VARCHAR(50) NOT NULL,
    source_endpoint VARCHAR(255),
    source_ip VARCHAR(45),
    message_type VARCHAR(50),
    message_size INTEGER,
    message_encoding VARCHAR(50),

    -- Performance metrics
    processing_time_ms INTEGER,

    -- Error tracking
    error_count INTEGER DEFAULT 0,
    last_error_message TEXT,

    -- Delivery tracking
    delivery_status VARCHAR(50),
    delivery_attempts INTEGER DEFAULT 0,

    -- V19: Parsing tracking columns
    parsed_at TIMESTAMP WITH TIME ZONE,
    parsing_status VARCHAR(50),
    parsing_time_ms INTEGER,
    parsing_error TEXT,

    -- Audit
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- MongoDB reference
    mongodb_doc_id VARCHAR(255)
);

CREATE INDEX idx_messages_intf_629ac1e8_message_id ON messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d(message_id);
CREATE INDEX idx_messages_intf_629ac1e8_status ON messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d(status);
CREATE INDEX idx_messages_intf_629ac1e8_parsing_status ON messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d(parsing_status);
```

---

## Code Structure

### Directory Layout

```
c:/Projects/ezHealthKonnect/
├── models/
│   └── parser_models.go                    # Data models
│
├── services/
│   ├── format_detector.go                  # Format detection
│   ├── parser_factory.go                   # Parser registry
│   ├── message_parser_service.go           # Main orchestrator
│   ├── mongodb_message_service.go          # MongoDB storage (updated)
│   ├── interface_message_service.go        # PostgreSQL storage (updated)
│   └── parsers/
│       └── hl7_parser_service.go           # HL7 adapter
│
├── processing/
│   └── engine.go                           # Processing engine (updated)
│
├── database/migrations/
│   └── V19__Add_Parsing_Columns.sql        # Flyway migration
│
├── hl7/
│   ├── types.go                            # Enhanced schema types
│   ├── parser.go                           # Existing HL7 parser
│   └── schema_loader.go                    # Schema loading
│
└── docs/
    ├── JSON_CONVERSION_ARCHITECTURE.md     # This file
    ├── JSON_CONVERSION_IMPLEMENTATION_COMPLETE.md
    └── REUSE_EXISTING_HL7_PARSER_DESIGN.md
```

### Key Files Modified

1. **processing/engine.go**
   - Added `parserService *services.MessageParserService` field
   - Auto-detects MongoDB and initializes parser service (OOB)
   - Triggers async JSON conversion after message storage
   - Added `convertToJSON()` method

2. **services/mongodb_message_service.go**
   - Added `ParsedContentUpdate` struct
   - Added `UpdateParsedContent()` method

3. **services/interface_message_service.go**
   - Added `MessageStatusUpdate` struct
   - Added `UpdateMessageParsingStatus()` method

4. **main.go**
   - Updated to use OOB ProcessingEngine initialization
   - MongoDB auto-detection via environment variables

---

## Database Migration

### V19: Add Parsing Columns

**File**: `database/migrations/V19__Add_Parsing_Columns.sql`

**Purpose**: Add parsing tracking columns to all interface message tables

**Columns Added**:
- `parsed_at TIMESTAMP WITH TIME ZONE`
- `parsing_status VARCHAR(50)`
- `parsing_time_ms INTEGER`
- `parsing_error TEXT`

**Application**:
```bash
# Automatic via Flyway on container startup
docker-compose up flyway

# Verify migration
docker-compose exec postgres psql -U ezhealth_user -d ezhealthkonnect \
  -c "SELECT version, description, installed_on FROM flyway_schema_history WHERE version = '19';"
```

**Features**:
- ✅ Idempotent (safe to run multiple times)
- ✅ Applies to ALL interface tables (dynamic)
- ✅ Tracked in `flyway_schema_history`
- ✅ Versioned for deployment consistency

---

## Deployment

### Prerequisites

1. **Environment Variables**:
   ```bash
   # MongoDB (enables hybrid storage and JSON conversion)
   MONGODB_HOST=mongodb
   MONGODB_PORT=27017
   MONGODB_DATABASE=ezhealthkonnect
   MONGODB_USER=ezhealth_user
   MONGODB_PASSWORD=secure_password_change_me

   # PostgreSQL
   DB_HOST=postgres
   DB_PORT=5432
   DB_NAME=ezhealthkonnect
   DB_USER=ezhealth_user
   DB_PASSWORD=secure_password_change_me
   ```

2. **Docker Services**:
   - PostgreSQL container
   - MongoDB container
   - Flyway container (for migrations)

### Deployment Steps

```bash
# 1. Apply database migration
docker-compose up flyway

# 2. Build application
docker-compose build app

# 3. Start services
docker-compose up -d

# 4. Verify parser service initialization
docker-compose logs app | grep -E "Parser Service|createHybridStorageEngine"

# Expected output:
# ✅ createHybridStorageEngine() called - initializing parser service...
# ✅ Parser Service initialized for automatic JSON conversion
```

### Verification

```bash
# Check MongoDB connection
docker-compose logs app | grep "MongoDB connected"

# Check parser service status
docker-compose logs app | grep "Parser Service"

# Check parsing logs when message received
docker-compose logs -f app | grep "JSON conversion"
```

---

## Testing

### Unit Testing

**Test Parser Service**:
```go
// tests/parser_service_test.go
func TestHL7ParserService(t *testing.T) {
    parser := parsers.NewHL7ParserService()

    rawHL7 := `MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20231025120000||ADT^A01|MSG00001|P|2.5|
PID|1||12345||DOE^JOHN^A||19800101|M|||123 MAIN ST^^CITY^STATE^12345|||||||||`

    result, err := parser.Parse(rawHL7)

    assert.NoError(t, err)
    assert.True(t, result.Success)
    assert.Equal(t, models.FormatHL7v2, result.Format)
    assert.NotNil(t, result.ParsedJSON["enhancedSegments"])
}
```

### Integration Testing

**Test End-to-End Flow**:
```bash
# Send test HL7 message
curl -X POST http://localhost:3000/api/messages/send/629ac1e8-0c50-447a-b93f-ebfc15830a7d \
  -H "Content-Type: text/plain" \
  -d "MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20231025120000||ADT^A01|MSG00001|P|2.5|
PID|1||12345||DOE^JOHN^A||19800101|M|||123 MAIN ST^^CITY^STATE^12345|||||||||"

# Check logs for JSON conversion
docker-compose logs -f app | grep "JSON conversion"

# Expected output:
# 🔄 Triggering JSON conversion for message: tcp_...
# ✅ JSON conversion completed for tcp_... (format: hl7v2, took 125ms)
```

### MongoDB Verification

```javascript
// Connect to MongoDB
docker-compose exec mongodb mongosh ezhealthkonnect

// Query parsed content
db.getCollection('raw_messages_intf_629ac1e8-0c50-447a-b93f-ebfc15830a7d').findOne(
  { parsed_content: { $exists: true } },
  {
    message_id: 1,
    'parsed_content.messageType': 1,
    'parsed_content.enhancedSegments.MSH.fields': 1,
    parsed_at: 1,
    parsing_time_ms: 1
  }
)

// Verify enhanced schema structure
db.getCollection('raw_messages_intf_629ac1e8-0c50-447a-b93f-ebfc15830a7d').findOne(
  {},
  {
    'parsed_content.enhancedSegments.PID.fields.name': 1,
    'parsed_content.enhancedSegments.PID.fields.dataType': 1,
    'parsed_content.enhancedSegments.PID.fields.description': 1
  }
)
```

### PostgreSQL Verification

```sql
-- Check parsing status
SELECT
  message_id,
  status,
  parsing_status,
  parsed_at,
  parsing_time_ms,
  parsing_error
FROM messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d
ORDER BY created_at DESC
LIMIT 10;

-- Verify parsing performance
SELECT
  AVG(parsing_time_ms) as avg_parsing_time,
  MIN(parsing_time_ms) as min_parsing_time,
  MAX(parsing_time_ms) as max_parsing_time,
  COUNT(*) as total_parsed
FROM messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d
WHERE parsing_status = 'success';
```

---

## Future Enhancements

### 1. Additional Parsers

**XML Parser**:
```go
// services/parsers/xml_parser_service.go
type XMLParserService struct {
    version string
}

func (xp *XMLParserService) Parse(rawContent string) (*models.ParserResult, error) {
    // Parse XML to JSON
    // Preserve XML structure
}
```

**FHIR Parser**:
```go
// services/parsers/fhir_parser_service.go
type FHIRParserService struct {
    version string
}

func (fp *FHIRParserService) Parse(rawContent string) (*models.ParserResult, error) {
    // Parse FHIR Bundle
    // Extract resources
}
```

### 2. Transformation Pipeline

**Global Transformations**:
- Cross-interface transformations
- Message-type specific rules
- Stored in database
- Applied after JSON conversion

**Interface-Specific Transformations**:
- Custom per-interface logic
- User-defined mapping rules
- Drag-and-drop UI (future)

**Transformation Sequencing**:
```
Raw Message
    ↓
JSON Conversion (automatic)
    ↓
System Transformations (format-specific)
    ↓
Global Transformations (message-type specific)
    ↓
Interface Transformations (custom)
    ↓
Output Message
```

### 3. Performance Optimization

- Parallel parsing for batch messages
- Parser result caching
- Schema caching (already implemented in HL7 parser)
- Connection pooling optimization

### 4. Monitoring & Metrics

- Parsing success/failure rates
- Average parsing time by message type
- Parser performance dashboard
- Alerting on parsing failures

---

## Troubleshooting

### Parser Service Not Initializing

**Symptoms**:
- No "Parser Service initialized" log
- No JSON conversion logs

**Diagnosis**:
```bash
# Check MongoDB connection
docker-compose logs app | grep "MongoDB"

# Check if parser service is nil
docker-compose logs app | grep "Parser service is nil"
```

**Solutions**:
1. Verify MongoDB environment variables
2. Check MongoDB container is running: `docker-compose ps mongodb`
3. Verify MongoDB connection: `docker-compose exec mongodb mongosh --eval "db.version()"`

### Parsing Failures

**Symptoms**:
- "JSON conversion failed" logs
- `parsing_error` populated in PostgreSQL

**Diagnosis**:
```bash
# Check parsing error logs
docker-compose logs app | grep "JSON conversion failed"

# Query failed parsings
docker-compose exec postgres psql -U ezhealth_user -d ezhealthkonnect \
  -c "SELECT message_id, parsing_error FROM messages_intf_* WHERE parsing_status = 'failed';"
```

**Common Causes**:
- Invalid HL7 message format
- Schema file missing
- HL7 version not supported

### Performance Issues

**Symptoms**:
- High `parsing_time_ms` values
- Slow message processing

**Diagnosis**:
```sql
-- Find slow parsings
SELECT message_id, parsing_time_ms, message_type
FROM messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d
WHERE parsing_time_ms > 1000
ORDER BY parsing_time_ms DESC;
```

**Solutions**:
- Check schema caching is enabled
- Verify MongoDB indexes exist
- Consider async processing optimization

---

## References

- **Enhanced Schema Types**: `hl7/types.go`
- **Original HL7 Parser**: `hl7/parser.go`
- **MongoDB Connection**: `services/mongodb_connection_service.go`
- **Hybrid Storage**: `services/hybrid_message_storage.go`
- **Flyway Migrations**: `database/migrations/`

---

## Summary

The JSON conversion pipeline is a production-ready system that:

✅ Automatically converts all incoming messages to JSON
✅ Preserves full enhanced schema from existing HL7 parser
✅ Follows MVC and OOB design principles
✅ Stores at multiple stages (raw, parsed, transformed)
✅ Tracks performance and validation metrics
✅ Integrates seamlessly with existing processing engine
✅ Supports future transformation layers
✅ Maintains complete audit trail

**Next Step**: Build transformation pipeline UI for user-defined mapping rules.
