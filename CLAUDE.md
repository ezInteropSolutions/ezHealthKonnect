# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ezHealthKonnect is an AI-powered healthcare integration platform that transforms HL7 messages to FHIR format. The system consists of a hybrid Node.js frontend with a Go backend for HL7/FHIR processing.

## Core Architecture

### Dual-Language Architecture
- **Node.js Frontend**: Express.js server handling authentication, UI serving, and API routing
- **Go Backend**: High-performance HL7/FHIR transformation engine
- **Proxy Layer**: Custom proxy in `app.js` forwards HL7/FHIR requests to Go backend

### Key Components
- `app.js`: Main Express application with custom proxy for Go backend
- `server.js`: Server startup with PostgreSQL connection management
- `main.go`: Go backend entry point with Gin router
- `controllers/`: Mixed Go (FHIR/HL7) and JavaScript (UI/auth) controllers
- `services/`: Business logic layer with both Go and JavaScript implementations

### Database Architecture
- **PostgreSQL**: Primary database for user data, audit logs, and configuration
- **Sequelize ORM**: Used for Node.js database operations
- **Go SQL**: Direct PostgreSQL connections for FHIR transformations
- **Interface-Specific Tables**: Dedicated message tables per interface for performance isolation
- **STANDARDIZED SCHEMA ONLY**: All interface tables use identical schema - NO LEGACY COMPATIBILITY

## 🚨 CRITICAL ARCHITECTURAL PRINCIPLES 🚨

### Schema Standards (NEVER COMPROMISE)
**RULE**: We are building NEW - NO legacy compatibility layers, NO schema variations, NO backward compatibility hacks.

**ENFORCEMENT**:
- All interface tables MUST use identical standardized schema from `InterfaceTableManager.getMessageTableSchema()`
- If any interface table has different schema → DROP and RECREATE with standard schema
- Never add conditional column checking or dynamic schema adaptation
- Standard columns: `id, message_id, correlation_id, interface_id, status, priority, received_at, source_type, source_endpoint, source_ip, message_type, message_size, message_encoding, raw_message, processing_completed_at, processing_time_ms, error_count, last_error_message, delivery_status, delivery_attempts, created_at, updated_at`

**RATIONALE**: Clean architecture, predictable behavior, maintainable code. We're in development - no production legacy to worry about.

## Development Commands

### Backend Services
```bash
# Start Node.js service only
npm run dictionary

# Start Node.js service with auto-reload
npm run dictionary:dev

# Start both Node.js and Go services
npm run start:all

# Start both services in development mode
npm run dev:all

# Test dictionary service
npm run test:dictionary
```

### Go Backend
```bash
# Run Go backend directly
go run main.go
```

### Manual Startup
```bash
# Start Node.js frontend (default port 3000)
node server.js

# Start Go backend (default port 8080)
go run main.go
```

## Service Communication

### Proxy Configuration
The Node.js frontend proxies specific routes to Go backend:
- `/api/fhir/*` → Go backend
- `/api/hl7/*` → Go backend
- `/api/system/*` → Go backend

Local Node.js routes:
- `/api/auth/*` → Node.js authentication
- `/api/users/*` → Node.js user management
- `/api/interfaces/*` → Node.js interface management
- `/api/wizard/*` → Node.js wizard functionality
- `/api/messages/*` → Node.js message management (interface-specific only)

### Environment Configuration
Key environment variables in `.env`:
- `PORT`: Node.js frontend port (default: 3000)
- `API_PORT`: Go backend port (default: 8080)
- `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`: PostgreSQL connection
- `SESSION_SECRET`: Session encryption key
- `JWT_SECRET`: JWT token signing key

## Database Schema

### Core Tables
- `users`: User accounts and authentication
- `interfaces`: Healthcare system interface configurations
- `audit_logs`: HIPAA/GDPR compliance audit trail
- `wizard_mappings`: HL7-FHIR field mappings
- `interface_table_metadata`: Tracks interface-specific message tables
- `messages_intf_*`: Interface-specific message tables (one per interface)
- Migration files in `database/migrations/`

## File Structure Patterns

### Controllers
- Go controllers: Handle HL7/FHIR processing, system endpoints
- JavaScript controllers: Handle UI, authentication, user management
- Naming: `*Controller.js` for Node.js, `*_controller.go` for Go

### Services
- Go services: HL7/FHIR transformation logic
- JavaScript services: User management, audit logging, interface configuration
- Mixed implementation based on performance requirements

### Routes
- Node.js routes in `routes/`: Authentication, user management, UI routing
- Go routes defined in `main.go`: HL7/FHIR API endpoints

## Security & Compliance

### Authentication Flow
1. Login via Node.js (`/api/auth/login`)
2. JWT token generation and session management
3. Session-based authentication for UI routes
4. Token-based authentication for API routes

### Audit Logging
- All user actions logged to PostgreSQL `audit_logs` table
- File-based backup logging in `logs/audit.log`
- HIPAA compliance features built-in

## Testing

### Current Test Structure
- Dictionary service testing: `npm run test:dictionary`
- Test files in `tests/` directory
- No comprehensive test suite currently implemented

## Build & Deployment

### Dependencies
- Node.js dependencies managed via `package.json`
- Go dependencies managed via `go.mod`
- Concurrent execution via `concurrently` package

### Database Setup
- PostgreSQL required for production mode
- Sequelize handles schema migration and synchronization
- Default admin credentials: admin@ezhealthkonnect.com / admin123

## Development Notes

### Wizard System
- Interactive HL7-FHIR mapping configuration
- Real-time mapping validation
- Field-level transformation rules
- **FIXED**: Now properly saves to PostgreSQL interfaces table
- Controllers: `wizardController.js`, `WizardMappingController.js`
- Service: `WizardMappingService.js` for detailed HL7-FHIR mappings

### FHIR Transformation
- Go-based high-performance transformation engine
- Support for multiple HL7 message types (ADT^A01, etc.)
- Schema-based validation and mapping
- Resource identification and categorization

## Recent Fixes (2024)

### Message-Type-Centric Architecture (V9)
- **Issue**: Interface-level mapping storage couldn't handle multiple message types per interface
- **Solution**: Completely redesigned with message-type-centric approach
- **New Architecture**:
  - `hl7_fhir_templates` table for standard mapping templates
  - `interface_message_mappings` table for interface-message-type configurations
  - Smart resolution: standard template vs custom mapping per message type
  - 99% storage reduction for interfaces using standard mappings

### Wizard Save Flow Fixed
- **Issue**: Wizard completed successfully but configurations weren't saving to interfaces table
- **Root Cause**: Service mismatch between Node.js interface management and Go backend calls
- **Solution**:
  - Updated `wizardController.js` to use `interfaceService.createInterface()` directly
  - Created `MessageTypeMappingService` for message-type-specific mapping storage
  - Added database migration V9 for message-type-centric relationships

### Multi-Message Type Support
- One interface can now handle multiple message types (ADT^A01, ORU^R01, etc.)
- Each message type gets its own mapping configuration
- Standard templates shared across interfaces for efficiency
- Custom mappings only stored when they differ from standard

### Database Schema Updates
- **V9 Migration**: `V9__Message_Type_Centric_Mapping.sql`
- New tables: `hl7_fhir_templates`, `interface_message_mappings`
- Auto-updating triggers for interface metadata
- Performance indexes for runtime mapping queries
- Seed data with standard ADT^A01 and ORU^R01 templates

## Testing

### Wizard Save Flow Testing
```bash
# Test the wizard save components
node tests/wizard-save-test.js
```

### Current Test Structure
- Dictionary service testing: `npm run test:dictionary`
- Message-type mapping flow: `tests/wizard-save-test.js`
- Test files in `tests/` directory

### Key API Endpoints (New in V9)
- **Runtime Mapping**: `GET /api/wizard/runtime-mapping/:interfaceId/:messageType` (for Go backend)
- **Interface Mappings**: `GET /api/wizard/interface-mappings/:interfaceId` (list all message types)
- **Wizard Complete**: `POST /api/wizard/complete` (saves to new schema)

### Migration Path
1. **V9 Migration** creates new message-type-centric tables
2. **Automatic migration** of existing transformation_mapping data
3. **Backward compatibility** maintained during transition
4. **Go backend** can use new runtime mapping endpoints

## Recent Fixes (2025)

### Interface-Specific Message Architecture (V14-V15)
- **Issue**: Global message viewing caused performance issues with large datasets and mixed interface types
- **Solution**: Implemented dedicated table-per-interface architecture for ultimate performance isolation
- **New Architecture**:
  - Each interface gets its own dedicated message table (`messages_intf_*`)
  - `interface_table_metadata` tracks all interface-specific tables
  - `InterfaceTableManager` service handles dynamic table creation and management
  - Adaptive schema handling for backward compatibility with existing tables

### Message Viewing Modernization
- **Removed**: Global message viewer (`/api/messages` endpoint now returns error)
- **Implemented**: Interface-specific message viewing only
- **Performance Benefits**:
  - No cross-table joins required
  - Isolated query performance per interface
  - Better scalability for high-volume interfaces
- **User Experience**: Users must select an interface to view messages (better workflow)

### Table Schema Compatibility
- **Backward Compatibility**: `InterfaceTableManager` automatically detects and adapts to existing table schemas
- **Legacy Support**: Handles both old format (`source` column) and new format (`source_type` column)
- **Dynamic Queries**: SELECT and INSERT queries adapt based on available columns in each table

## Message Management System

### Current Architecture
- **Interface-Specific Storage**: Each interface has its own dedicated message table
- **Performance Isolation**: No shared table bottlenecks between interfaces
- **Scalable Design**: High-volume interfaces don't impact low-volume ones

### Key Services
- **InterfaceTableManager**: Core service for managing interface-specific tables
  - Dynamic table creation for new interfaces
  - Adaptive schema handling for existing tables
  - Performance-optimized queries
- **InterfaceTableMaintenanceService**: Automated maintenance for interface tables
  - Table statistics updates
  - Data retention cleanup
  - Performance monitoring

### API Endpoints (Message Management)
- **Interface Messages**: `GET /api/messages/interface/:interfaceId` (gets messages for specific interface)
- **Interface Stats**: `GET /api/messages/interface/:interfaceId/stats` (gets statistics for specific interface)
- **Send Message**: `POST /api/messages/send/:interfaceId` (sends message to specific interface)
- **Global Endpoints**: Removed for performance (redirects to interface selection)

### Frontend Integration
- **Navigation**: All message links now redirect to interface selection
- **Interface Cards**: Each interface has a "View Messages" button (💬) linking to its message viewer
- **URL Format**: `messages.html?interfaceId={interfaceId}` for interface-specific viewing
- **No Global View**: Users must select an interface to view messages
## JSON Conversion Pipeline (V19 - October 2025)

### Overview
Automatic JSON conversion system that converts all incoming messages to structured JSON as the first transformation step. Preserves full enhanced schema from HL7 parser.

### Architecture
- **Pattern**: MVC + OOB (Out-of-Box)
- **Storage**: Hybrid (PostgreSQL metadata + MongoDB full content)
- **Processing**: Asynchronous goroutine-based
- **Code Reuse**: 100% reuse of existing `hl7.ParseWithRealSchema()`

### Key Components
```
models/parser_models.go              # Data models
services/format_detector.go          # Auto-detect message format
services/parser_factory.go           # Parser registry (factory pattern)
services/message_parser_service.go   # Main orchestrator
services/parsers/hl7_parser_service.go  # HL7 adapter (wraps existing parser)
processing/engine.go                 # Async conversion trigger
```

### Data Flow
```
Message Received
    ↓
Store Raw (PostgreSQL + MongoDB)
    ↓
Async Trigger: go pe.convertToJSON()
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
// Collection: raw_messages_intf_<interface-id>
{
  message_id: "tcp_...",
  raw_content: "MSH|^~\&|...",  // Original HL7
  
  parsed_content: {  // FULL ENHANCED SCHEMA
    enhancedSegments: {
      "MSH": {
        key: "MSH",
        name: "Message Header",
        fields: [
          {
            key: "MSH.3",
            name: "Sending Application",
            value: "...",
            position: 3,
            dataType: "HD",
            description: "...",
            subfields: [...]
          }
        ]
      },
      "PID": {...},
      "PV1": {...}
    },
    segmentOrder: ["MSH", "PID", "PV1"],
    messageType: { code: "ADT", event: "A01", ... },
    version: "2.5",
    dictionaryUsed: true,
    schemaLoaded: true,
    validationErrors: []
  },
  
  parsed_at: ISODate("..."),
  parsing_time_ms: 125,
  parsed_format: "hl7v2"
}
```

### PostgreSQL Tracking
```sql
-- Table: messages_intf_<interface-id>
-- V19 Migration added columns:
parsed_at TIMESTAMP WITH TIME ZONE
parsing_status VARCHAR(50)
parsing_time_ms INTEGER
parsing_error TEXT
```

### OOB Initialization
```go
// processing/engine.go
func NewProcessingEngine(db *sql.DB) *ProcessingEngine {
    // Auto-detect MongoDB from environment
    mongoService, err := services.NewMongoDBConnectionService()
    
    if err == nil {
        // Hybrid storage with parser service
        return createHybridStorageEngine(db, mongoService)
    }
    
    // Fallback to PostgreSQL-only
    return createPostgreSQLOnlyEngine(db)
}
```

### Verification
```bash
# Check parser initialized
docker-compose logs app | grep "Parser Service initialized"

# Watch JSON conversion
docker-compose logs -f app | grep "JSON conversion"

# Query parsed JSON
docker-compose exec mongodb mongosh ezhealthkonnect
db.getCollection('raw_messages_intf_<id>').findOne(
  { parsed_content: { $exists: true } },
  { 'parsed_content.enhancedSegments': 1 }
)
```

### Migration Status
- **Migration**: V19__Add_Parsing_Columns.sql
- **Applied**: Via Flyway on container startup
- **Tracked**: flyway_schema_history table
- **Status**: ✅ Production Ready

### Documentation
- **Master Reference**: [SYSTEM_DOCUMENTATION.md](SYSTEM_DOCUMENTATION.md) - Complete consolidated system documentation
- **Architecture Details**: [architecture/JSON_CONVERSION_ARCHITECTURE.md](architecture/JSON_CONVERSION_ARCHITECTURE.md)
- **Transformation Pipeline**: [architecture/TRANSFORMATION_PIPELINE_DESIGN.md](architecture/TRANSFORMATION_PIPELINE_DESIGN.md)
- **Hybrid Storage**: [architecture/HYBRID_STORAGE_ARCHITECTURE.md](architecture/HYBRID_STORAGE_ARCHITECTURE.md)
- **Scalability Design**: [architecture/SCALABILITY_AND_GUI_DESIGN.md](architecture/SCALABILITY_AND_GUI_DESIGN.md)


## Transformation Pipeline Architecture (Design Phase - October 2025)

### Overview
Flexible, user-configurable transformation pipeline that applies business logic to parsed JSON messages in a user-defined sequence.

### Three-Layer Model
```
Layer 1: System Transformations (Auto) - JSON conversion ✅ Complete
Layer 2: Pre-Processing (User-defined) - Validation, enrichment, custom logic
Layer 3: Core Mapping (Template-based) - HL7→FHIR using stored mappings
Layer 4: Post-Processing (User-defined) - FHIR validation, anonymization
```

### How Mappings Get Applied
```
1. Message arrives with message_type (e.g., "ADT^A01")
2. Lookup pipeline: WHERE interface_id AND message_type
3. Execute steps in sequence order (10, 20, 100, 200, ...)
4. Each step can be:
   - Built-in executor (validation, enrichment)
   - Template-based (reusable with parameters)
   - Custom JavaScript (user-defined logic)
```

### User-Defined Logic Support

**JavaScript Example**:
```javascript
function transform(input) {
    var pid = input.enhancedSegments.PID;
    if (pid.fields.find(f => f.key === "PID.5").value.includes("VIP")) {
        input._metadata.priority = "high";
    }
    return input;
}
```

**Stored in Database**:
```sql
transformation_steps table:
- pipeline_id (which interface + message type)
- sequence (execution order: 10, 20, 30, ...)
- step_type (validation, enrichment, mapping, custom)
- script_content (JavaScript code)
- config (step-specific parameters)
```

### Sequence Management

**Sequence Rules**:
- Lower number = earlier execution
- Ranges: 1-99 (pre), 100-199 (core), 200-299 (post)
- Dependencies: Step B waits for Step A via `depends_on_steps` array

**Example Pipeline**:
```
Seq 10:  Validate Patient ID (required)
Seq 20:  Enrich from Epic API
Seq 50:  Custom VIP detection (JavaScript)
Seq 100: Apply HL7→FHIR template (core mapping)
Seq 200: Validate FHIR bundle
Seq 210: Anonymize PHI (custom JavaScript)
```

### Database Schema (V20 - Planned)

**New Tables**:
- `transformation_pipelines` - Pipeline configuration per interface + message type
- `transformation_steps` - Individual steps with sequence, type, config
- `transformation_executions` - Execution history and audit trail
- `transformation_step_executions` - Detailed step-by-step tracking
- `transformation_templates` - Reusable step templates

### Integration with Existing System

**Trigger Point** (processing/engine.go):
```go
// After JSON conversion completes
if result.Success {
    go pe.executeTransformationPipeline(
        messageID,
        interfaceID,
        result.ParsedJSON,
        result.Metadata.MessageType,
    )
}
```

**Data Flow**:
```
Parsed JSON (from MongoDB)
    ↓
Get Pipeline (transformation_pipelines)
    ↓
Execute Steps in Sequence (transformation_steps)
    ↓
Store Transformed Output (MongoDB: transformed_content)
    ↓
Deliver to Destination
```

### Implementation Status

**Current**: Design Phase
**Timeline**: 6-8 weeks estimated
**Dependencies**: JSON Conversion Pipeline ✅ Complete

**Next Steps**:
1. Create V20 database migration (5 new tables)
2. Implement TransformationPipelineService (execution engine)
3. Add JavaScript runtime support (goja library)
4. Build management API endpoints
5. Design drag-and-drop UI for pipeline builder

## Master Documentation

### Primary References
- 📚 **[SYSTEM_DOCUMENTATION.md](SYSTEM_DOCUMENTATION.md)** - Complete consolidated reference (all architecture, APIs, schemas)
- 🤖 **[CLAUDE.md](CLAUDE.md)** - AI assistant project guide (this file)

### Architecture Deep Dives
- 🔄 **[architecture/JSON_CONVERSION_ARCHITECTURE.md](architecture/JSON_CONVERSION_ARCHITECTURE.md)** - JSON conversion pipeline details
- 🔀 **[architecture/TRANSFORMATION_PIPELINE_DESIGN.md](architecture/TRANSFORMATION_PIPELINE_DESIGN.md)** - Transformation pipeline architecture
- 💾 **[architecture/HYBRID_STORAGE_ARCHITECTURE.md](architecture/HYBRID_STORAGE_ARCHITECTURE.md)** - PostgreSQL + MongoDB storage design
- 📈 **[architecture/SCALABILITY_AND_GUI_DESIGN.md](architecture/SCALABILITY_AND_GUI_DESIGN.md)** - Scale + UI architecture
- 🏗️ **[architecture/ARCHITECTURE_REFERENCE.md](architecture/ARCHITECTURE_REFERENCE.md)** - Design patterns and principles
- ⚙️ **[architecture/INTERFACE_CONFIGURATION_ENGINE.md](architecture/INTERFACE_CONFIGURATION_ENGINE.md)** - Configuration engine design

### Archived Documentation
- 📦 **[docs/archive/](docs/archive/)** - 120 historical implementation logs, debug guides, and status reports (consolidated into SYSTEM_DOCUMENTATION.md)

## Multi-Connectivity Architecture (October 2025)

### Overview
Universal connector framework supporting 32 OOB connectors for healthcare integration patterns. System acts as a **middleware/integration engine** - receiving messages from any source and delivering to any destination.

### Phase 1: Foundation (✅ Complete - October 26, 2025)
**Database Schema** - 4 migrations created:
- **V26**: Multi-connectivity foundation (4 tables: connectivity_types, interface_connectivity, cron_jobs, connectivity_execution_log)
- **V27**: Database connectors (PostgreSQL, MySQL, SQL Server, MongoDB, Oracle - inbound/outbound)
- **V28**: Message queues + cloud storage (RabbitMQ, Kafka, Redis, AWS S3, Azure Blob, GCS, SFTP, FTP)
- **V29**: TCP/MLLP outbound (middleware scenario support - user-requested feature)

**Models & Services**:
- [models/connectivity_models.go](models/connectivity_models.go) - Complete type definitions
- [services/connectivity_service.go](services/connectivity_service.go) - CRUD operations with NULL JSONB handling
- [controllers/connectivity_controller.go](controllers/connectivity_controller.go) - 16 REST API endpoints

**Final Count**: 32 connectors (16 inbound + 16 outbound) with perfect symmetry

### Phase 2A: Connector Framework (✅ Complete - October 26, 2025)
**Universal Interface** - [services/connectors/connector_interface.go](services/connectors/connector_interface.go):
```go
type Connector interface {
    Initialize(config []byte) error
    GetMetadata() ConnectorMetadata
    Validate() error
    TestConnection(ctx context.Context) error
    Close() error
    GetStatus() ConnectorStatus
}

type InboundConnector interface {
    Connector
    Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error
    Stop() error
    SupportsCron() bool
}

type OutboundConnector interface {
    Connector
    Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error)
    SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error)
    SupportsBatch() bool
}
```

**Base Implementation** - [services/connectors/base_connector.go](services/connectors/base_connector.go):
- BaseConnector with thread-safe state management (RWMutex)
- BaseInboundConnector with graceful shutdown (stop channel)
- BaseOutboundConnector with batch support

**Factory Pattern** - [services/connectors/connector_factory.go](services/connectors/connector_factory.go):
- Global singleton factory with automatic registration
- All 32 connectors registered at initialization
- Support for custom connector plugins

**Connector Stubs** - [services/connectors/connector_stubs.go](services/connectors/connector_stubs.go):
- Minimal implementations for all 32 connectors
- Ready for actual implementation logic

### Phase 2B: Connector Implementation (🔄 In Progress - October 26, 2025)

**Implemented Connectors**:
1. ✅ **TCP/MLLP Inbound** - [tcp_mllp_inbound.go](services/connectors/tcp_mllp_inbound.go)
   - Full MLLP protocol parser (0x0B start, 0x1C/0x0D end)
   - TLS 1.2/1.3 support with certificate configuration
   - Connection pooling with configurable max connections
   - Automatic ACK/NACK generation
   - Keep-alive with configurable period
   - Read/write timeout handling
   - Graceful shutdown with active connection tracking
   - Message type extraction from MSH segment
   - Message control ID correlation

**Priority Queue** (Next 3-4 weeks):
2. ⏳ TCP/MLLP Outbound (middleware scenario - send HL7 to downstream)
3. ⏳ HTTP Outbound (FHIR delivery to REST endpoints)
4. ⏳ File Writer (local archiving and debugging)
5. ⏳ PostgreSQL Inbound/Outbound (EHR database integration)
6. ⏳ MongoDB Inbound/Outbound (FHIR persistence)
7. ⏳ AWS S3 Inbound/Outbound (cloud file handling)
8. ⏳ Kafka Producer (event streaming)
9. ⏳ RabbitMQ Publisher (message queue delivery)

### Connector Catalog

**Network Connectors (4)**:
- tcp_mllp_inbound ✅ / tcp_mllp_outbound ⏳
- http_rest_inbound / http_outbound ⏳

**File System Connectors (2)**:
- file_listener / file_writer ⏳

**Database Connectors (10)**:
- postgresql_inbound / postgresql_outbound ⏳
- mysql_inbound / mysql_outbound
- sqlserver_inbound / sqlserver_outbound
- mongodb_inbound / mongodb_outbound ⏳
- oracle_inbound / oracle_outbound

**Message Queue Connectors (6)**:
- rabbitmq_inbound / rabbitmq_outbound ⏳
- kafka_inbound / kafka_outbound ⏳
- redis_inbound / redis_outbound

**Cloud Storage Connectors (6)**:
- aws_s3_inbound / aws_s3_outbound ⏳
- azure_blob_inbound / azure_blob_outbound
- gcs_inbound / gcs_outbound

**File Transfer Connectors (4)**:
- sftp_inbound / sftp_outbound
- ftp_inbound / ftp_outbound

### Documentation
- 📘 **[connectivity/CONNECTIVITY_CATALOG.md](connectivity/CONNECTIVITY_CATALOG.md)** - Complete catalog with all 32 connectors
- 📗 **[connectivity/CONNECTOR_IMPLEMENTATION_GUIDE.md](connectivity/CONNECTOR_IMPLEMENTATION_GUIDE.md)** - Step-by-step implementation guide
- 🏗️ **[connectivity/CONNECTIVITY_ARCHITECTURE.md](connectivity/CONNECTIVITY_ARCHITECTURE.md)** - Architecture design and patterns
- 🔐 **[connectivity/CONNECTIVITY_CLOUD_AND_SECURITY.md](connectivity/CONNECTIVITY_CLOUD_AND_SECURITY.md)** - Cloud integration and security
- 📋 **[connectivity/CONNECTIVITY_PATTERNS.md](connectivity/CONNECTIVITY_PATTERNS.md)** - Integration pattern explanations

### Key Architectural Decisions
1. **OOB Pattern** - Metadata-driven configuration stored in database
2. **Factory Pattern** - Dynamic connector instantiation by type name
3. **Interface Segregation** - Separate interfaces for inbound/outbound connectors
4. **Thread Safety** - All connectors use mutex-protected state management
5. **Graceful Shutdown** - Context cancellation + stop channels for clean termination
6. **Middleware Support** - TCP/MLLP outbound enables bidirectional scenarios (user feedback)

## Format-Agnostic Field Utilities (January 2025)

### Overview
Shared utilities for reading and updating message fields across different healthcare message formats (HL7v2, FHIR, generic JSON). Uses **Strategy Pattern** for format-specific resolvers with a unified API.

### Key File
- **[services/executors/field_utils.go](services/executors/field_utils.go)** - Format-agnostic field operations

### Supported Path Types
```go
const (
    PathTypeHL7     FieldPathType = "hl7"     // e.g., PID.3, MSH.9.1, OBX.5.2
    PathTypeFHIR    FieldPathType = "fhir"    // e.g., Patient.name[0].given
    PathTypeJSON    FieldPathType = "json"    // e.g., data.patient.name
    PathTypeUnknown FieldPathType = "unknown"
)
```

### Public API (Exported Functions)
```go
// Auto-detect path type
func DetectPathType(path string) FieldPathType

// Format-agnostic getter - retrieves value from any message format
func GetFieldValue(data map[string]interface{}, path string) interface{}

// Format-agnostic setter - updates value in any message format
func UpdateFieldValue(data map[string]interface{}, path string, newValue interface{}) bool

// Path detection helpers
func IsHL7FieldPath(path string) bool   // Detects PID.3, MSH.9.1 patterns
func IsFHIRPath(path string) bool       // Detects Patient.name, Observation.value patterns

// Path conversion for UI display
func GetAbsolutePath(path string) string // Converts short notation to full JSON path
```

### Internal Functions (Private)
- `resolveHL7FieldValue`, `resolveHL7FieldFromMap` - HL7 getters
- `resolveFHIRFieldValue` - FHIR getter
- `resolveJSONPathValue` - Generic JSON getter with array support
- `modifyHL7FieldValue`, `modifyHL7FieldInMap` - HL7 setters
- `modifyFHIRFieldValue` - FHIR setter
- `modifyJSONPathValue` - Generic JSON setter with array support
- `parseJSONPath` - Parses paths like `data.items[0].name` into parts

### Path Format Examples
| Format | Example Path | Description |
|--------|-------------|-------------|
| HL7 | `PID.3` | Patient ID field |
| HL7 | `PID.5.1` | Patient name, family component |
| HL7 | `MSH.9.1` | Message type code |
| FHIR | `Patient.name[0].given` | First name in FHIR Patient |
| FHIR | `Observation.value` | Observation value |
| JSON | `data.items[0].name` | Generic JSON with array |
| JSON | `metadata.source` | Simple dot notation |

### Usage in Executors
```go
// In conditional_executor.go (transform action)
import "ezhealthkonnect/services/executors"

// Get field value (auto-detects format)
value := executors.GetFieldValue(outputData, "PID.13.4")

// Update field value (auto-detects format)
if executors.UpdateFieldValue(outputData, targetField, transformedValue) {
    fmt.Printf("Updated %s = %v\n", targetField, transformedValue)
}

// Get absolute path for UI tooltip display
absolutePath := executors.GetAbsolutePath("PID.13.4")
// Returns: "enhancedSegments.PID.fields[key=PID.13].subfields[key=PID.13.4].value"
```

### HL7 Data Structure Support
The utilities support both:
1. **Typed Go structs**: `map[string]hl7.EnhancedSegment` (runtime)
2. **Generic maps**: `map[string]interface{}` (after JSON marshaling)

### Design Principles
- **DRY**: Single implementation used by all executors
- **Strategy Pattern**: Different resolvers for different formats
- **Extensible**: Easy to add new formats (X12, CDA, etc.)
- **Dual Support**: Works with both typed structs and generic maps

### Related Files
- [services/executors/base_executor.go](services/executors/base_executor.go) - Contains original `getHL7FieldValue` for typed structs only
- [services/executors/control/conditional_executor.go](services/executors/control/conditional_executor.go) - Uses field utilities in transform action
- [models/output_normalizer.go](models/output_normalizer.go) - Preserves HL7 keys (PID.3) instead of sanitizing to snake_case

## Multi-Step Routing in Switch/Case and If-Then-Else (January 2025)

### Overview
The `route_to_step` action now supports routing to **multiple target steps** from a single case or condition. This enables complex workflows where a single condition triggers a sequence of steps to execute.

### Key Files
- **Backend**: [services/executors/control/conditional_executor.go](services/executors/control/conditional_executor.go)
- **SwitchCase UI**: [public/js/pipeline/components/SwitchCaseBuilder.js](public/js/pipeline/components/SwitchCaseBuilder.js)
- **IfThenElse UI**: [public/js/pipeline/components/IfThenElseBuilder.js](public/js/pipeline/components/IfThenElseBuilder.js)

### Config Schema

**Legacy (single step)**:
```json
{
    "action": "route_to_step",
    "targetStepId": "step-123"
}
```

**New (multiple steps)**:
```json
{
    "action": "route_to_step",
    "targetStepIds": ["step-1", "step-2", "step-3"]
}
```

### Backend Behavior
The executor sets `_routing` in the output data:

```go
// Single step routing
routingMap["nextStep"] = stepId

// Multi-step routing (takes priority)
routingMap["nextSteps"] = targetStepIds  // []string
```

### UI Features
- **Dropdown to add steps**: Select from available pipeline steps
- **Chip/tag display**: Shows selected steps in execution order (1., 2., 3.)
- **Remove button**: Click × on any chip to remove a step
- **Skip steps**: Still supported for exclusive branching

### Usage Example
A Switch/Case on `MSH.9.1` (message type) could route:
- Case "ADT" → Execute: [Validate Patient, Enrich Demographics, Route to ADT Handler]
- Case "ORU" → Execute: [Validate Results, Route to Lab Handler]
- Default → Execute: [Log Warning, Route to Error Handler]

### Backward Compatibility
- Existing `targetStepId` (single) configs still work
- System auto-migrates to `targetStepIds` array when editing
- Backend accepts both `stepId` and `targetStepIds`

## File Parser Executor (February 2026)

### Overview
Parses structured files (CSV, TSV, fixed-width, Excel, Avro, Parquet) into `[]map[string]interface{}`
records. Uses the **Strategy Pattern** — format parsers self-register via `init()`; the orchestrator
calls `GetFormatParser(format)` instead of a switch statement.

### Source Types
| `sourceType` | Description |
|---|---|
| `field` (default) | Raw content already in a pipeline field (from an Inbound Connector) |
| `local_path` | Read from the server/container filesystem; batch mode via glob pattern |
| `field_as_path` | A pipeline field holds a URI: `s3://`, `https://`, `file:///` |

### Format Support
CSV, TSV, fixed-width (CCLF, NACHA, X12), Excel xlsx/xls, Apache Avro, Apache Parquet.
Binary formats (xlsx, xls, avro, parquet) detected from magic bytes automatically.

### Key Features
- **File size gate**: `os.Stat()` before `os.ReadFile()`. Default 100 MB, hard cap 500 MB. Configure via `maxFileSizeMB`.
- **Streaming CSV**: `MaxRecords > 0` → row-by-row via `csv.Reader.Read()` — O(MaxRecords) memory.
- **Auto-detect**: Magic bytes → extension → delimiter heuristics. Set `autoDetect: true`.
- **OOB healthcare templates**: `cclf1`–`cclf8`, `nacha_entry`, `era_835_header` — pre-built fixed-width column definitions.
- **S3 credential decrypt**: `interface_connectivity.source_config` → AES-256-GCM decrypt via `CredentialStore.DecryptConfigBytes` → AWS SDK.
- **Content encoding**: Set `contentEncoding: "base64"` when binary content was base64-encoded in a pipeline field.

### Key Files
- Executor: `services/executors/enrichment/file_parser_executor.go`
- Format interface + registry: `services/executors/enrichment/format_parsers.go`
- Parsers: `csv_parser.go`, `fixed_width_parser.go`, `excel_parser.go`, `avro_parser.go`, `parquet_parser.go`
- OOB templates: `services/executors/enrichment/file_parser_templates.go`
- S3/HTTP resolver: `services/executors/enrichment/file_parser_remote.go`
- Full architecture: [architecture/FILE_PARSER_ARCHITECTURE.md](architecture/FILE_PARSER_ARCHITECTURE.md)

