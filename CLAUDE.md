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

## 🚨 MANDATORY: INVESTIGATE BEFORE ANY CODE CHANGE 🚨

Before writing, editing, deleting, or improving ANY code — for any reason, no matter how small:

1. **Read every file that will be touched**
2. **Read every file the changed code calls into or depends on**
3. **State what you found** — what already exists, what is reusable, what the current behavior is
4. **Only then propose or write code**

This is non-negotiable. No exceptions for "small" changes, "obvious" fixes, or "quick" additions.
If you skip this and the user catches it — stop immediately, investigate, then continue.

**Why this rule exists:** Features were built (DLQ retry, SendWithRetry) without reading the
outbound executor first. The result was a redundant retry layer that conflicted with the existing
`ExecuteWithRetry()` in the pipeline service, and a redrive mechanism that bypassed the pipeline
entirely. Reading first prevents building the wrong thing.

## 🚨 MANDATORY: OOP/Enterprise-Grade Code Standards 🚨

All code in this repository must be:

- **Enterprise-grade**: production-ready, no prototypes, no placeholder implementations in shipped code
- **OOP-compliant**: use interfaces, structs with methods, dependency injection — not procedural globals
- **Reusable**: extract shared logic into services/utilities; no copy-paste between files
- **Dependency-injected**: services receive dependencies via constructors, not globals or `init()`
- **Modular, not monolithic**: no single file/object/config blob that keeps growing to cover every case (a ~3000-line hardcoded doc-string object literal is exactly as monolithic as a 3000-line God function — split by responsibility, e.g. one small file per category, a registry that self-assembles, not one ever-growing switch)
- **Not hardcoded**: the concrete anti-pattern to watch for is **one hand-written function per type/section/resource** (e.g. a separate `allergyToCanonical`, `medicationToCanonical`, `conditionToCanonical`, or six near-identical `writeXHeader` functions). This is hardcoded even if each individual function is short and "clean" in isolation — the tell is that *adding a new case requires writing new code* instead of adding schema/config data. Prefer the pattern already proven in this codebase: `cda/builder`'s section/entry engine (`xpath_writer.go`, `entry_archetypes.go`) is ONE generic function driven by schema data (`CDASectionDef`, `StructuralTemplateAnchor`, a small boilerplate lookup table) — new sections get added as pure JSON schema edits, zero new Go functions.
  - **Self-check before calling work done**: could a new instance of "the thing this code handles" (new section, new resource type, new header element, new connector, new document type) be added via data/config alone? If the honest answer is "no, you'd need to write a new function," say that gap out loud rather than presenting the work as finished.

No exceptions. A "quick fix" that violates these standards is not acceptable.

**Go**: Expose behavior through interfaces; accept interfaces, return structs; inject via constructor.
**JavaScript**: Use classes with clear method boundaries; pass services as constructor arguments.

### Follow full SDLC for non-trivial changes
Investigate (see above) → design/plan and get alignment before writing code for anything touching more than a couple of files or introducing a new abstraction → implement → write tests (unit tests for new logic, not just happy-path) → verify (Go: the `/go-build-check` skill, never `go build` on the host directly; UI: actually drive it in a real browser, don't just typecheck — this has repeatedly caught real bugs unit tests missed). Small, localized fixes (bug fixes, nil guards, string coercions) don't need a formal plan; structural changes and new features do.

---

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
- **Playwright E2E suite**: `tests/playwright/` — 9 spec files, ~150 tests across auth, dashboard, interfaces, detail, monitoring, messages, settings, admin, and DLQ pages
  - Run: `npx playwright test --project=chromium`
  - Admin/user management page URL: `/user-management.html` (NOT `/admin.html`)
  - Auth state stored in `tests/playwright/.auth/admin.json` (created by global-setup)

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
- 🗺️ **[architecture/HL7_FHIR_MAPPING_DESIGN.md](architecture/HL7_FHIR_MAPPING_DESIGN.md)** - HL7→FHIR template design: two-phase model, context links, transforms, bindings, OOB template strategy
- 🔄 **[architecture/JSON_CONVERSION_ARCHITECTURE.md](architecture/JSON_CONVERSION_ARCHITECTURE.md)** - JSON conversion pipeline details
- 🔀 **[architecture/TRANSFORMATION_PIPELINE_DESIGN.md](architecture/TRANSFORMATION_PIPELINE_DESIGN.md)** - Transformation pipeline architecture
- ⚡ **[architecture/DAG_PARALLEL_EXECUTION_DESIGN.md](architecture/DAG_PARALLEL_EXECUTION_DESIGN.md)** - DAG parallel execution (replaces sequence-number model, supports floating steps + multi-entry convergence)
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

### Phase 2B: Connector Implementation (✅ Complete - May 2026)

**26 connectors fully implemented** in own `.go` files as of this phase. **Superseded — see the August 2026 snapshot below**, which reflects 11 further stub→real conversions done in later sessions (MySQL/AWS S3/Kafka outbound, HTTP/REST inbound, Azure Blob/Databricks/Snowflake both directions) that this list predates.

1. ✅ **TCP/MLLP Inbound** - [tcp_mllp_inbound.go](services/connectors/tcp_mllp_inbound.go)
   - Full MLLP protocol parser (0x0B start, 0x1C/0x0D end)
   - TLS 1.2/1.3 support with certificate configuration
   - Connection pooling with configurable max connections
   - Configurable ACK/NACK generation (see ACK/NACK section below)
   - Keep-alive with configurable period
   - Read/write timeout handling
   - Graceful shutdown with active connection tracking
   - Message type extraction from MSH segment
   - Message control ID correlation
   - Segment delimiter: **CRLF** (`\r\n`) in generated ACK/NACK messages
2. ✅ **TCP/MLLP Outbound** - [tcp_mllp_outbound.go](services/connectors/tcp_mllp_outbound.go)
3. ✅ **HTTP Outbound** - [http_outbound.go](services/connectors/http_outbound.go)
4. ✅ **HTTP FHIR Inbound** - [http_fhir_inbound.go](services/connectors/http_fhir_inbound.go)
5. ✅ **HTTP FHIR Outbound** - [http_fhir_outbound.go](services/connectors/http_fhir_outbound.go)
6. ✅ **File Listener** - [file_listener.go](services/connectors/file_listener.go)
7. ✅ **File Writer** - [file_writer.go](services/connectors/file_writer.go)
8. ✅ **PostgreSQL Inbound** - [postgresql_inbound.go](services/connectors/postgresql_inbound.go)
9. ✅ **PostgreSQL Outbound** - [postgresql_outbound.go](services/connectors/postgresql_outbound.go)
10. ✅ **MySQL Inbound** - [mysql_inbound.go](services/connectors/mysql_inbound.go)
11. ✅ **SQL Server Inbound** - [sqlserver_inbound.go](services/connectors/sqlserver_inbound.go)
12. ✅ **SQL Server Outbound** - [sqlserver_outbound.go](services/connectors/sqlserver_outbound.go)
13. ✅ **Oracle Inbound** - [oracle_inbound.go](services/connectors/oracle_inbound.go)
14. ✅ **Oracle Outbound** - [oracle_outbound.go](services/connectors/oracle_outbound.go)
15. ✅ **MongoDB Inbound** - [mongodb_inbound.go](services/connectors/mongodb_inbound.go)
16. ✅ **MongoDB Outbound** - [mongodb_outbound.go](services/connectors/mongodb_outbound.go)
17. ✅ **Kafka Inbound** - [kafka_inbound.go](services/connectors/kafka_inbound.go)
18. ✅ **RabbitMQ Inbound** - [rabbitmq_inbound.go](services/connectors/rabbitmq_inbound.go)
19. ✅ **Redis Inbound** - [redis_inbound.go](services/connectors/redis_inbound.go)
20. ✅ **AWS S3 Inbound** - [aws_s3_inbound.go](services/connectors/aws_s3_inbound.go)
21. ✅ **SFTP Inbound** - [sftp_inbound.go](services/connectors/sftp_inbound.go)
22. ✅ **SFTP Outbound** - [sftp_outbound.go](services/connectors/sftp_outbound.go)

### Connector Status Snapshot (re-verified August 2026)

Re-derived directly from ground truth — `connector_stubs.go`'s remaining stub functions (a stub returns a bare `NewBase{In,Out}boundConnector(metadata)` with no real logic) cross-checked against `connector_factory.go`'s `registerBuiltInConnectors()` — rather than trusting the Phase 2B narrative above, which had drifted out of date across several sessions of connector work. **37 of 55 registered connector types are real** (19 of 27 inbound, 18 of 28 outbound; counts exclude type-name aliases like `tcp_mllp`/`http`/`http_rest`, and `sink_outbound`, which is real but trivial by design — a store-only terminal target with no external I/O).

**Real, beyond the Phase 2B list above** (13 further conversions):
- `mysql_outbound`, `aws_s3_outbound`, `kafka_outbound` — [mysql_outbound.go](services/connectors/mysql_outbound.go), [aws_s3_outbound.go](services/connectors/aws_s3_outbound.go), [kafka_outbound.go](services/connectors/kafka_outbound.go)
- `http_rest_inbound` — [http_rest_inbound.go](services/connectors/http_rest_inbound.go) — a genuinely separate, non-FHIR generic HTTP receiver; previously aliased to the FHIR connector, corrected after user feedback (see "HTTP FHIR Receiver vs Generic HTTP/REST Inbound" precedent — any format succeeds via `RawPassthroughParser`, only the FHIR connector validates)
- `azure_blob_inbound`, `azure_blob_outbound` — [azure_blob_inbound.go](services/connectors/azure_blob_inbound.go), [azure_blob_outbound.go](services/connectors/azure_blob_outbound.go)
- `databricks_inbound`, `databricks_outbound` — [databricks_inbound.go](services/connectors/databricks_inbound.go), [databricks_outbound.go](services/connectors/databricks_outbound.go) — official `databricks-sql-go` driver, PAT auth only
- `snowflake_inbound`, `snowflake_outbound` — [snowflake_inbound.go](services/connectors/snowflake_inbound.go), [snowflake_outbound.go](services/connectors/snowflake_outbound.go) — official `gosnowflake/v2` driver, username/password auth only (key-pair/JWT auth explicitly rejected with a clear error, not silently ignored — unverified against any real cloud warehouse account, no test credentials available in this environment)
- `as2_inbound`, `as2_outbound` (Phase 4, September 2026) — [as2_inbound.go](services/connectors/as2_inbound.go), [as2_outbound.go](services/connectors/as2_outbound.go) — signed+encrypted HTTP (S/MIME via `github.com/smallstep/pkcs7`, CMS-wrapped signing not multipart/signed) with synchronous MDN; a NEW connector type, not a `transport` branch on `edi_x12_inbound`/`edi_x12_outbound` (those stay SFTP-only permanently) — see `as2_inbound.go`'s own header comment for the architecture reasoning. Async MDN and multipart/signed are named, deferred items, not attempted
- `direct_messaging_inbound`, `direct_messaging_outbound` (September 2026) — [direct_messaging_inbound.go](services/connectors/direct_messaging_inbound.go), [direct_messaging_outbound.go](services/connectors/direct_messaging_outbound.go) — DirectTrust email (S/MIME over IMAP poll / SMTP send), reusing AS2's own CMS sign+encrypt/decrypt+verify primitives directly (`smime_shared.go`, renamed from `as2_smime.go` once this second connector started depending on it) — see the "Direct Messaging Connector" section below for the full design and full-stack proof. MDN-over-email is a named, deferred item, not attempted
- `websocket_inbound`, `websocket_outbound` (September 2026, brand-new — not a stub→real conversion, the type names didn't exist before this) — [websocket_inbound.go](services/connectors/websocket_inbound.go), [websocket_outbound.go](services/connectors/websocket_outbound.go) — real-time bidirectional pair via `github.com/gorilla/websocket`; see the "WebSocket Connector" section below for the full design and full-stack proof

**Still stubs (18 registered types, unchanged by the above)**:
- Analytics DBs — BigQuery, Redshift, Synapse, ClickHouse, TimescaleDB (both directions — 10 types)
- MQ outbound — RabbitMQ, Redis (publish; Kafka outbound is now real, see above)
- Cloud storage — GCS (both directions)
- File transfer — FTP (both directions)
- `fhir_r4_inbound`/`fhir_r4_outbound` (native FHIR R4, distinct from the already-real `http_fhir_*` connectors) — stub functions exist in `connector_stubs.go` but are **not registered in the factory at all**, i.e. unreachable via any type name today; a genuinely future item, not an oversight in this audit

**Dead code found and removed during this audit**: `connector_stubs.go` had an orphaned `NewHTTPRESTInboundConnector` (all-caps "REST") stub — same `type_name: "http_rest_inbound"` as the real `NewHTTPRestInboundConnector`, but never referenced by the factory (which registers the mixed-case name) — a leftover from the http_rest_inbound stub→real conversion that never got cleaned up. Removed; zero other references existed (confirmed via repo-wide grep before deleting).

### Connector Catalog

**Network Connectors**:
- tcp_mllp_inbound ✅ / tcp_mllp_outbound ✅
- http_outbound ✅ / http_fhir_inbound ✅ / http_fhir_outbound ✅
- http_rest_inbound ✅
- websocket_inbound ✅ / websocket_outbound ✅ (real-time bidirectional pair, `github.com/gorilla/websocket` — see "WebSocket Connector" section below)

**File System Connectors (2 of 2)**:
- file_listener ✅ / file_writer ✅

**Database Connectors (10 full)**:
- postgresql_inbound ✅ / postgresql_outbound ✅
- mysql_inbound ✅ / mysql_outbound ✅
- sqlserver_inbound ✅ / sqlserver_outbound ✅
- mongodb_inbound ✅ / mongodb_outbound ✅
- oracle_inbound ✅ / oracle_outbound ✅
- Analytics DBs (BigQuery, Redshift, Synapse, ClickHouse, TimescaleDB) — stubs

**Cloud Data Warehouse Connectors (both directions full)**:
- databricks_inbound ✅ / databricks_outbound ✅
- snowflake_inbound ✅ / snowflake_outbound ✅

**Message Queue Connectors (3 inbound full, 1 outbound full, 2 outbound stub)**:
- rabbitmq_inbound ✅ / rabbitmq_outbound — stub
- kafka_inbound ✅ / kafka_outbound ✅
- redis_inbound ✅ / redis_outbound — stub

**Cloud Storage Connectors**:
- aws_s3_inbound ✅ / aws_s3_outbound ✅
- azure_blob_inbound ✅ / azure_blob_outbound ✅
- gcs_inbound — stub / gcs_outbound — stub

**File Transfer Connectors**:
- sftp_inbound ✅ / sftp_outbound ✅ (rewritten on the real SFTP protocol via `github.com/pkg/sftp` — the original build used SSH shell-exec, which fails against properly-locked-down SFTP-only servers)
- ftp_inbound — stub / ftp_outbound — stub

**Healthcare Protocol Connectors**:
- edi_x12_inbound ✅ / edi_x12_outbound ✅ (SFTP transport)
- as2_inbound ✅ / as2_outbound ✅ (signed+encrypted HTTP with synchronous MDN, Phase 4 — see EDI X12 sections below)
- direct_messaging_inbound ✅ / direct_messaging_outbound ✅ (S/MIME over IMAP poll / SMTP send — see "Direct Messaging Connector" section below)

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

## TCP/MLLP ACK/NACK Configuration (March 2026)

### Overview
Configurable acknowledgment behaviour on the TCP/MLLP Inbound connector. Each inbound listener can independently control how it ACKs messages, including a custom JavaScript script for fully dynamic ACK logic.

### ACKConfig struct (`tcp_mllp_inbound.go`)
```go
type ACKConfig struct {
    Mode            string // "immediate" (default) | "none"
    OnError         string // "suppress" (default — always AA) | "nack" (send AE on queue full)
    SendingApp      string // MSH-3 in generated ACK (default: "ezHealthKonnect")
    SendingFacility string // MSH-4 in generated ACK (default: "EHK")
    TextSuccess     string // MSA-3 on AA (default: "Message received successfully")
    TextError       string // MSA-3 on AE/AR (default: "Message processing error")
    Script          string // Optional JS override — see Custom Script below
}
```

### ACK Message Format
Generated ACKs use **CRLF** (`\r\n`) as segment terminators:
```
MSH|^~\&|<sendingApp>|<sendingFacility>|SENDER|SENDER|<ts>||ACK|<controlID>|P|2.5\r\n
MSA|<AA|AE|AR>|<controlID>|<textMessage>\r\n
```
- MSH-10 (control ID) is echoed from the original message's MSH-10 field
- Inbound parsers (`extractMessageControlID`, `extractMSHField`) normalise both `\r` and `\r\n` for robustness

### ACK Modes
| Mode | Behaviour |
|---|---|
| `immediate` | AA sent after message is placed on queue |
| `none` | No ACK sent — sender must not expect a response |

### On Error Behaviour
| onError | Behaviour |
|---|---|
| `suppress` | Always send AA regardless of errors (sender retries externally) |
| `nack` | Send AE when queue is full or critical processing error occurs |

### Custom ACK Script (goja JS runtime)
Script runs in a goja VM (Go). Function must be named `buildACK`:
```javascript
function buildACK(msg) {
    // msg properties: controlID, messageType, sendingApp, sendingFacility,
    //                 raw, defaultCode, defaultText
    if (msg.messageType !== 'ADT^A01') {
        return { ackCode: 'AR', textMessage: 'Unsupported message type' };
    }
    return { ackCode: 'AA', textMessage: 'Accepted' };
}
```
- Valid `ackCode` values: `AA`, `AE`, `AR`
- Errors, missing return, or missing function → falls back to default ACK (no crash)
- Script has access to full message context including raw HL7

### Pipeline Step Config (connector.inbound)
```json
{
  "connectorType": "tcp_mllp_inbound",
  "config": {
    "host": "0.0.0.0",
    "port": 2575,
    "ack": {
      "mode": "immediate",
      "on_error": "suppress",
      "sending_app": "MyHIS",
      "sending_facility": "WARD_7B",
      "text_success": "Routed to Epic ADT feed",
      "text_error": "Queue full — please retry",
      "script": "function buildACK(msg) { ... }"
    }
  }
}
```

### UI — Acknowledgment Tab
The `ConnectorConfigBuilder` renders an **Acknowledgment** tab (hidden for non-MLLP/outbound types) with four collapsible groups:
- **Basic** (expanded): ACK Mode + On Error
- **Sender Identity** (collapsed): MSH-3, MSH-4 overrides
- **Message Text** (collapsed): success and error text
- **Custom Script** (collapsed): dark-themed code editor textarea

Tab visibility is controlled by `get isMLLPInbound()` and updated immediately in `onConnectorTypeChange()` before the connector-type lookup guard.

### Test Coverage
| File | Tests | Coverage |
|---|---|---|
| `services/connectors/tcp_mllp_ack_test.go` | 36 Go tests | Unit (getStringFromMap, generateACKMessage, runACKScript, Initialize) + Integration (real TCP round-trips) |
| `tests/playwright/ack-nack-e2e.spec.js` | 43 E2E tests | Tab visibility, field defaults, collapsible groups, getConfig(), pipeline save, API, dry-run, XSS, regression |

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

## CDA Cross-Document Dedupe — crossMessage Architecture (July 2026)

### Why it exists
Most sending EHRs generate CCDs as **cumulative snapshots**, not deltas — every CCD for a patient includes their *entire* current problem/medication/allergy list, not just what changed since the last encounter. A pipeline that parses-and-forwards every CCD independently re-delivers the same facts on every single encounter. `cda.dedupe`'s `crossMessage` mode turns that cumulative feed into an effective incremental one: each clinical fact is delivered downstream exactly once — the first time it's ever seen for that patient on that interface — regardless of how many later documents restate it.

### Mechanism
- **In-document dedup** (always on, no DB): removes literal duplicates within one parsed document via a composite identity key (`cdaDedupeIdentityRules`, e.g. medication = code + start date).
- **Cross-message dedup** (`crossMessage: true` + `patientIdentifierRoot`): after in-document dedup, each surviving entry's identity key is checked against `cda_dedupe_registry` — an atomic `INSERT ... ON CONFLICT ... DO UPDATE` (Postgres `xmax = 0` trick) that inserts-and-keeps a genuinely new identity, or updates `seen_count`/`last_seen_at` and drops the entry if it's already registered. `patient_key` is the raw CDA `<id>` extension for whichever OID root is configured — there is no master-patient-index in this system and no auto-detection; a document without an identifier at that root silently skips crossMessage for that message only (in-document dedup still runs).
- **Scope**: registry rows are keyed `(interface_id, patient_key, section_key, identity_key)` — history never crosses interfaces, even for the same physical patient.
- **No expiry by default** beyond the retention job below — a suppressed identity stays suppressed forever, which is correct for continuous facts (an allergy doesn't "recur") and still safe for date-keyed facts (a genuinely new occurrence gets a new identity key).

### Explicit scope boundary — NOT a patient-summary/CDR builder
`cda_dedupe_registry` stores only an identity **fingerprint** per fact (e.g. `"code.code=I10|effectiveTime.low.value=20240115"`) — never the actual clinical content (no drug name, dose, route). It can answer "have I sent this before," not "what is this patient's complete current record." A use case like "ingest encounter-level CCDs, emit one deduplicated patient-level summary CCD downstream" needs an actual per-patient content store that stays current — closer to a small clinical data repository / MPI-adjacent system, with its own patient-matching and conflict-resolution concerns, than a pipeline step. Deliberately out of scope for this engine; pair with a real CDR/HIE record locator downstream if that's the requirement.

### Lineage (what got removed, and why)
Per-message step output includes both an aggregate count and per-entry lineage:
```
_stepOutput.section_stats.<sectionKey>.cross_message_removed   → count
_stepOutput.section_stats.<sectionKey>.cross_message_suppressed → [{identity_key, first_seen_message_id, first_seen_at}, ...]
```
`cross_message_suppressed` is populated straight from the same `RETURNING` clause that does the upsert (`checkAndRegisterCrossMessage`) — no extra DB round trip. Capture is config-controlled: `cda.dedupe` step config `trackSuppressionLineage` (nil/omitted = on by default; explicit `false` opts out of the per-entry detail while suppression itself is unaffected).

**Per-message UI**: every real production message already gets one `step_executions` row per step (existing pipeline persistence, unrelated to this feature) — `GET /api/messages/:messageId/dedupe-suppressions` (`MessageController.getDedupeSuppressions`, scoped by `interfaces.user_id` like `getMessageDetail`) reads that row's `step_output.section_stats.*.cross_message_suppressed` and flattens it. `public/js/messages.js`'s message detail view renders this as a "Dedup Suppressions" step on the **Journey tab** (only when non-empty), with the first-seen message ID linking back into that message's own detail view. For a broader view across many messages for one patient (not just one message's trace), use the admin registry viewer instead.

### Admin access, audit, and retention (HIPAA/GDPR)
`cda_dedupe_registry` holds PHI (patient identifier + clinical codes/dates), so it has the same operational controls as any other PHI-bearing table in this system:
- **Access**: `GET/DELETE /api/cda/dedupe/registry` (`controllers/cda_dedupe_registry_controller.go`), admin-gated at both the Node proxy (`app.js`, registered before the blanket `/api/cda` forward) and the Go layer (`requireAdminRole`, mirrors `ai_controller.go`). UI: `public/admin-cda-dedupe-registry.html`, linked from Admin nav.
- **Audit**: every view and purge writes to `audit_logs` (`CDA_DEDUPE_REGISTRY_VIEWED` / `CDA_DEDUPE_REGISTRY_PURGED`, real `user_id` from the forwarded `X-User-ID`) — GDPR Art. 17 erasure requires a typed reason. Deliberately does NOT log every automatic suppression decision (that's routine processing, already traced via the lineage fields above) — only human-initiated access/deletion, to avoid doubling the growth problem retention already solves.
- **Retention**: `services/retention_enforcement.go`'s `enforceCDADedupeRegistry`, purging by `last_seen_at` (never `first_seen_at` — an actively-recurring fact must never age out just because it was first seen long ago). Configurable via `AppSettingsCache.GetCDADedupeSettings().RegistryRetentionDays` (default 2555 days / ~7 years).
- **Growth model**: bounded by *distinct facts per patient per section*, not by message volume — a repeat sighting is an `UPDATE` (`seen_count++`), never a new row.

### Key Files
- Executor: `services/executors/transform/cda_dedupe_executor.go`
- OOB identity rules: `services/executors/transform/cda_dedupe_templates.go`
- Registry migration: `database/migrations/V191__Add_CDA_Dedupe_Registry.sql`, retention index: `V196__CDA_Dedupe_Registry_Retention_Index.sql`
- Admin access controller: `controllers/cda_dedupe_registry_controller.go`
- Retention job: `services/retention_enforcement.go` (`enforceCDADedupeRegistry`)
- Settings: `services/app_settings.go` (`CDADedupeSettings`)
- Step config UI (checkbox field picker, no-code): `public/js/pipeline/components/CDAStepBuilder.js` (`CDADedupeStepBuilder`), backed by `GET /api/cda/dedupe/sections`
- Admin registry viewer: `public/admin-cda-dedupe-registry.html` + `public/js/admin-cda-dedupe-registry.js`
- Per-message lineage endpoint: `controllers/MessageController.js` (`getDedupeSuppressions`), `routes/messageRoutes.js` (`GET /:messageId/dedupe-suppressions`)
- Per-message lineage UI (Journey tab "Dedup Suppressions" step): `public/js/messages.js` (`loadDataLineage`, `renderDataLineage`)

## C-CDA R2.1 Document-Type Coverage — 12 of 12 Types, One Documented Gap (August 2026)

### Status
All 12 official HL7 C-CDA R2.1 document-level templates are now registered in `cda/schemas/ccda_2_1.json` and supported end-to-end (inbound parse → FHIR mapping → Coverage Audit, outbound `cda.build`): CCD, Discharge Summary, Referral Note, History and Physical, Consultation Note, Progress Note, Care Plan, Diagnostic Imaging Report, Operative Note, Procedure Note, Transfer Summary, Unstructured Document. Both `cda/builder` (outbound) and `cda/document`/`services/cda_fhir`/`services/cda_coverage` (inbound) are schema-driven and document-type-blind — adding a document type whose sections already exist is a pure JSON edit; only Unstructured Document needed new Go (different body shape, no `structuredBody`).

### Permanent gap: DICOM Object Catalog Section (Diagnostic Imaging Report)
Not built, and not planned under the current architecture. Its own spec constraints (Vol 2 §2.12, templateId `2.16.840.1.113883.10.20.6.1.1`, DCM code `121181`) conflict with 3 assumptions baked into the generic section/entry engine:
1. **No `title`/`text` at all** — the section's SHALL list omits both; `document_builder.go`'s section loop unconditionally writes `<title>` for every section.
2. **Genuine 3-level polymorphic nesting** — Study Act → Series Act (`entryRelationship[@typeCode='COMP']`) → N SOP-Instance observations (`classCode="DGIMG"`, another `COMP` level), three distinct templated entity types. `CDASectionDef` only supports 2 levels (`EntryElementPath` + one `ObservationElementPath`, or `RepeatingGroups` for N repeats of *one* item type) — there's no 3-tier parent/child/grandchild shape.
3. **A positional constraint** — "SHALL be first section in the document if present" — the section-emission loop (SHALL+SHOULD+MAY concatenation) has no ordering concept beyond list order.

Decision (2026-08-09, user-confirmed): document as a permanent, spec-acknowledged limitation rather than build new engine capability or ship a non-conformant stub. Diagnostic Imaging Report's `findingsDIR` section (its one true SHALL section, narrative-only) works today; documents containing DICOM image references will not get a catalog on outbound build, and inbound documents with one will Coverage-Audit as present-but-unmapped rather than error.

### History and Physical SHALL-list correction
`documentTypeSections["History and Physical"]` was wrong since the original 6-type build — it used 3 optional sections (`chiefComplaint`, `historyOfPresentIllness`, `assessment`) as SHALL and was missing 8 of the real 10 required sections. Corrected against Vol 2 §1.1.12 Table 39 + Companion Guide §3.3.6 Table 21 (both agree): SHALL = `reviewOfSystems, generalStatus, medications, results, pastMedicalHistory, vitalSigns, physicalExamination, socialHistory, familyHistory, allergiesAndIntolerances`. The 3 previously-SHALL sections moved to MAY (spec marks them optional, no SHOULD tier exists for this document type in either source). Not yet added: **Instructions Section (V2)** (`2.16.840.1.113883.10.20.22.2.45:2014-06-09`) — a genuinely new, unverified section also listed as optional for H&P; deliberately left out of this batch pending its own entry-level spec verification.

### New sections added this round
- **Care Plan** (MAY): `healthStatusEvaluationsOutcomes` (structured — Outcome Observation `.4.144`, free-form LOINC code, no fixed code unlike most reused-observation sections), `interventions` (narrative-only v1 — Intervention Act `.4.131` wraps 11 possible nested activity types per its own contexts table; entries are 0..* SHOULD so narrative-only is spec-conformant, not a scope reduction).
- **Transfer Summary** (MAY): `courseOfCare`, `generalStatus` — both narrative-only.
- **Procedure Note** (MAY): `reasonForVisit`, `chiefComplaintAndReasonForVisit`, `medicalGeneralHistory` (narrative-only), `medicationsAdministered` (structured — substanceAdministration reusing Medication Activity V2 `.4.16:2014-06-09`, same shape as `anesthesia`), `physicalExamination` (reused — Procedure Note's "Physical Exam Section (V3)" is byte-for-byte the same templateId/ext/LOINC as the already-registered key, not a new section).
- **`courseOfCare`/`hospitalCourse` share LOINC `8648-8`** ("Hospital Course Narrative") — confirmed directly against both sections' own spec tables, not a data-entry error. `resolveKey`'s templateId tier (checked first, always populated for these sections) resolves both correctly; only a malformed document missing templateId but present LOINC could hit the map collision in `sectionByLOINC`, same latent class of risk as any other schema with reused codes.

### FHIR mapping follow-on (not yet done)
None of the 8 new sections have dedicated `MappingRule`s — narrative-only ones ride the existing section-level narrative pass for a `DocumentReference` for free; the 2 structured ones (`healthStatusEvaluationsOutcomes`, `medicationsAdministered`) are Coverage-Audit-visible-but-unmapped until their own rules are written (same trade-off precedent as `header.legalAuthenticator`).

## Document-Type Section-List Audit — CCD, Discharge Summary, Referral Note, Consultation Note, Progress Note (August 2026)

### Why this happened
The original 6-document-type build (pre-dating this session) never rigorously audited any document type's own document-level constraints table (Vol2 §1.1.x) — every `documentTypeSections` SHALL/SHOULD/MAY list was written once and never independently re-verified. History and Physical's audit (above) found a real bug; the same audit was then run against the other 5 "original" types and found real defects in all 5, ranging from minor (CCD) to severe (Progress Note's SHALL list was entirely wrong).

### Corrected section lists
All 5 are now corrected in `cda/schemas/ccda_2_1.json`'s `documentTypeSections`, each independently verified against two primary sources (Vol2's own per-section narrative constraint list — the `i., ii., iii...` items — plus the Companion Guide's required/optional table):
- **CCD**: `planOfTreatment` SHALL→SHOULD, `payersInsurance` SHOULD→MAY.
- **Discharge Summary**: `dischargeMedications` SHALL→SHOULD, `problems` SHALL→MAY, `planOfTreatment` added to SHALL (was entirely unregistered), 5 sections SHOULD→MAY, `results` removed entirely (not a valid section for this type), 9 valid-but-unregistered sections added to MAY, 2 new sections registered (`hospitalConsultations`, `hospitalDischargeStudiesSummary`, both narrative-only).
- **Referral Note**: `encounters` removed (invalid section), 2 sections SHOULD→MAY, 4 valid-but-unregistered sections added to SHOULD, 9 added to MAY, plus a choice constraint (below).
- **Consultation Note**: `historyOfPresentIllness` added to SHALL (was entirely unregistered), `assessment`/`chiefComplaint`/`medications` removed from SHALL (assessment's real requirement moved to the choice constraint, chiefComplaint→MAY, medications→SHOULD), `encounters` removed, `physicalExamination` added to SHOULD, large MAY expansion, plus the same choice constraint.
- **Progress Note**: SHALL list is now genuinely empty — Companion Guide Table 24's Required Sections column is blank; there is no individually-required section for this document type, only the choice constraint. `socialHistory`/`immunizations` removed (invalid sections), 3 sections SHOULD→MAY, 2 new sections registered (`objective`, `subjective`, both narrative-only), plus `instructions` (below) added to MAY.

### Instructions Section (V2) — a genuinely mandatory entry, not narrative-only
Unlike nearly every other section added this session, Instructions Section (V2) (`instructions` key, templateId `2.16.840.1.113883.10.20.22.2.45` ext `2014-06-09`, LOINC `69730-0`) has `entry 1..* SHALL` when the section is present — narrative-only would not be spec-conformant. Its entry references the **Instruction (V2)** act template (`2.16.840.1.113883.10.20.22.4.20:2014-06-09`, `classCode="ACT" moodCode="INT" statusCode="completed"`). Registered as MAY for History and Physical and Progress Note only — the only two document types whose own Contains: table references it (confirmed via full-file grep, not assumption).

### `dischargeInstructions` — pre-existing enhancement beyond its own section's spec (documented, not changed)
`dischargeInstructions` (Discharge Summary, templateId `2.16.840.1.113883.10.20.22.2.41`, LOINC `8653-8`, "Hospital Discharge Instructions Section") already emits a structured `<entry><act>` using the same Instruction (V2) template as `instructions` above — but Hospital Discharge Instructions Section's own Vol2 chapter (§2.28, Table 122) defines **no entry at all** (templateId→code→title→text only). Not a bug: C-CDA sections are open templates, so the extra structured content is valid, just not spec-*mandated* for this particular section. Left as-is — real, working capability, not silently removed.

### Choice constraints — a requirement shape flat SHALL/SHOULD/MAY lists can't express
Consultation Note, Referral Note, and Progress Note each carry a real spec requirement of the form *"SHALL contain Assessment and Plan Section (V2), OR both Assessment Section and Plan of Treatment Section (V2)"* (Vol2 CONF:1198-29102/9501/30657 — same boilerplate text reused across the IG). A flat per-section SHALL/SHOULD/MAY list has no way to express "at least one of these branches."

**New schema construct** (`cda/schema_types.go`): `DocumentTypeSectionInfo.ChoiceConstraints []SectionChoiceConstraint`, where `SectionChoiceConstraint{Description string, Branches [][]string}` — satisfied if every section key in ANY ONE branch is present. Assessment/AssessmentAndPlan/PlanOfTreatment keep their own individually-correct tier (usually MAY) in the flat lists; the compound SHALL-level requirement lives entirely in `choiceConstraints`, not as an inflated tier on any one section.

**Validator integration** (`cda/validator/validator.go`'s `Validate()`): each `ChoiceConstraint` contributes one unit to `shallTotal`/`shallPresent` (satisfied or not), producing a `ChoiceConstraintReport` in the new `ComplianceReport.ChoiceConstraintReports` field. This is how Progress Note — with a genuinely empty SHALL list — can still reach `ShallScore == 1.0` purely via a satisfied choice constraint.

**Scope boundary**: this only affects conformance *scoring*. The matching mutual-exclusion half of the same spec text ("SHALL NOT contain both branches") is **not** enforced — `cda/builder` trusts its canonical input same as everywhere else in the engine; a caller populating more than one branch gets all of it emitted. Real, deliberate, named scope boundary (user-confirmed 2026-08-10) — not an oversight.

### Key Files
- Schema data: `cda/schemas/ccda_2_1.json` (`documentTypeSections`, 5 new section defs) — **superseded**: see "CDA Schema 3-Tier Restructuring" below; the same data now lives in `cda/schemas/ccda_2_1/manifest.json`'s `documentTypeSections`.
- Choice-constraint type: `cda/schema_types.go` (`SectionChoiceConstraint`)
- Choice-constraint scoring: `cda/validator/validator.go` (`validateChoiceConstraint`), `cda/validator/compliance_report.go` (`ChoiceConstraintReport`)
- Tests: `cda/builder/document_builder_test.go` (canonical docs + round-trip tests for all 4 previously-untested document types), `cda/validator/choice_constraints_test.go`

## CDA Schema 3-Tier Restructuring — `ccda_2_1.json` → Directory Tree (August 2026)

### Why
`cda/schemas/ccda_2_1.json` had grown to 4,118 lines (69 sections, 12 document types) and kept growing every time a document type was touched — a real "Modular, not monolithic" violation, unlike `hl7/`/`fhir/` (one file per message/resource type) and unlike `cda/builder` itself (already a generic, schema-driven engine — this file was the same anti-pattern one layer up, in the schema data rather than the Go code). A naive 1-file-per-section split wasn't safe on its own: several entry-level archetypes (Problem Observation, Medication Activity, Author Participation) are structurally reused across sections, and a mechanical split would have copy-pasted that duplication into 69 files instead of removing it.

### New layout
```
cda/schemas/ccda_2_1/
  manifest.json          # Tier 1: profile/documentTemplates/documentTypeMetadata/documentTypeSections/sectionOrder
  sections/<key>.json    # Tier 2: 69 files, one per section (dots kept literally, e.g. header.patient.json)
  entries/
    _constants.json      # flat OID constants for shape-less repeated anchors (Author Participation, Medication Information)
    problem-observation-bare.json        # root-anchor template — pastMedicalHistory, procedureFindings, complications
    problem-observation-wrapped.json     # observation-anchor template — problems, dischargeDiagnosis, admissionDiagnosis, preoperativeDiagnosis, postprocedureDiagnosis
    medication-activity.json             # root-anchor template — medications, anesthesia, medicationsAdministered
    medication-activity-wrapped.json     # observation-anchor template — dischargeMedications, admissionMedications
    instruction-v2.json                  # root-anchor template — dischargeInstructions, instructions
```
`c32_mapping.json` and `uscdi_v3.json` are unrelated, untouched siblings.

### The "wrapped" vs "root" anchor distinction (a correction made mid-implementation)
An initial design had both "wrapped" templates (Problem/Medication Observation nested inside a wrapping Act) own `entryTemplateId`/`entryElementPath` like the "bare"/root templates do. Reading every member section directly disproved that: `problems`/`dischargeDiagnosis`/`admissionDiagnosis`/`preoperativeDiagnosis`/`postprocedureDiagnosis` each cite their OWN distinct wrapping-act template (Problem Concern Act `.4.3` vs. Discharge Diagnosis Act `.4.33` vs. Admission Diagnosis Act `.4.34` vs. `.4.65` vs. `.4.51`) with their own distinct `entryFixedCode`. Only the *nested* Problem/Medication Observation is genuinely identical across each cluster. So the two "wrapped" templates are **observation-anchor-only** (`anchor: "observation"` in the on-disk template — supplies only `observationTemplateId`/`observationElementPathSuffix`/`observationFixedCode*`/`obsCodeTranslation*`); `entryTemplateId`/`entryElementPath`/`entryFixedCode*` stay section-owned. This is the reason `onDiskEntryTemplate` has two anchor modes rather than one.

### Load-time resolution mechanism
`cda/schema_disk_types.go` (new file, package `cda`) defines the on-disk-only types (`onDiskManifest`, `onDiskSectionDef`, `onDiskEntryTemplate`, `onDiskStructuralTemplateAnchor`, `onDiskTemplateConstant`) and `resolveSection`/`resolveXPath`/`resolveAnchor`, which `cda/schema_loader.go`'s rewritten `loadProfile()` calls to assemble the exact same in-memory `*CDAProfileDef`/`*CDASectionDef` shape `cda/schema_types.go` has always defined — **zero changes** to `CDASectionDef`, `CDAFieldDef`, `CDAProfileDef`, or `buildIndexes()`. `NewCDASchemaLoader(schemaDir string)`'s signature is unchanged (24+ call sites across `main.go`, executors, and test files all still pass a single directory path — only the internal join target moved from `<schemaDir>/ccda_2_1.json` to `<schemaDir>/ccda_2_1/manifest.json` + `sections/` + `entries/`).

`resolveXPath(anchor, raw)` treats `raw` as already-absolute (passthrough, unchanged) whenever it starts with `"entry/"` **or** the section has no anchor at all (`anchor == ""` — the 4 header pseudo-sections, whose fields target `ClinicalDocument/...` paths, not `entry/...`). This anchor-less case was a real bug the golden-file regression test caught before it shipped (an earlier version of `resolveXPath` unconditionally prefixed non-`entry/`-prefixed paths, corrupting every header field's xpath with a stray leading `/`). Field xpaths themselves were deliberately left absolute (not converted to anchor-relative form) even for the 15 sections that adopted a template — `problems` has fields relative to BOTH its `EntryElementPath` and `ObservationElementPath` within the same section, and the on-disk schema has no per-field anchor selector, so a blanket "strip the anchor prefix" conversion would have been unsafe for that section specifically. The relative-xpath resolution path exists and is unit-tested but isn't currently exercised by real schema data — see `TestResolveXPath` in `cda/schema_disk_types_test.go`.

### Migration was two separate passes
**Phase 1** (mechanical, zero behavior risk): a one-time, self-deleting test (`cda/tools_migration_test.go`, since removed) unmarshalled the monolith straight into the existing `CDAProfileDef`/`CDASectionDef` Go types and re-marshalled each section to its own file — no template indirection yet, `entries/` empty. **Phase 2** (hand-curated): added the 5 templates + `_constants.json`, migrated 21 of 69 section files (15 to `entryTemplate`/`observationTemplate`, 19 `structuralTemplateIds[]` anchors to `templateIdRef` — 16 Author Participation + 3 Medication Information), the other 48 untouched.

### 3 pre-existing gaps found and fixed during Phase 2 (named, not silent)
Making `problem-observation-wrapped`/`problem-observation-bare` the single source of truth for their fixed-code/translation fields surfaced — and fixed — 3 real inconsistencies among structural siblings that predate this restructuring:
- `dischargeDiagnosis` and `admissionDiagnosis` were missing `observationFixedCode: "55607006"` (SNOMED "Problem") + `obsCodeTranslationCode: "75325-1"` (LOINC "Problem") that `preoperativeDiagnosis`/`postprocedureDiagnosis` already had.
- `pastMedicalHistory` was missing the entry-level equivalent (`entryFixedCode`/`entryCodeTranslationCode`) that `procedureFindings`/`complications` already had.

**Explicitly NOT touched**: `dischargeMedications` has `entryStatusCodeOverride: "completed"`; `admissionMedications` (its structural sibling) doesn't. Left as a section-owned override — the schema alone doesn't say whether this is a real IG distinction or a 4th gap; would need direct IG-text confirmation before normalizing either way.

### Regression proof
`cda/schema_loader_test.go`'s `TestSchemaLoaderResolvesGoldenSnapshot` snapshots `AllSections()`/`AllDocumentTypes()`/`GetDocumentTypeSections`/`GetDocumentTypeMetadata`/`DocumentTemplates` (via testify `assert.Equal` + a checked-in `cda/testdata/schema_snapshot_golden.json`, refreshed only via `-update`). Captured against the *original* monolithic loader before any restructuring code existed; after Phase 1 it was byte-for-byte zero-diff; after Phase 2 the diff was exactly the 6 named fields above on exactly the 3 named sections, nothing else. `cda/schema_disk_types_test.go` adds 15 focused unit tests for the new resolution logic (`resolveXPath` table test, root/observation template merging, fail-fast on unknown `entryTemplate`/`observationTemplate`/`templateIdRef`/duplicate `sectionOrder`/section-key mismatch). Verified via `/go-build-check` (full `docker-compose build app`) plus a live runtime check against the rebuilt app container (`/api/cda/document-types`, `/api/cda/schema/sections`, `/api/cda/canonical-fields/section/problems` all correct).

### Key Files
- Loader rewrite: `cda/schema_loader.go` (`loadProfile()`), new `cda/schema_disk_types.go`
- Schema data: `cda/schemas/ccda_2_1/{manifest.json,sections/*.json,entries/*.json}`
- Tests: `cda/schema_loader_test.go` (golden snapshot), `cda/schema_disk_types_test.go` (resolution unit tests), `cda/testdata/schema_snapshot_golden.json` (fixture)
- Unchanged (verified, not just assumed): `cda/schema_types.go`, `cda/builder/entry_archetypes.go`, `cda/document/section_parser.go`, `services/cda_fhir/`, `services/cda_coverage/`, `controllers/cda_schema_controller.go`

## USCDI v3 Vocabulary Bridge — Fill Status + UDI (Phase A3, August 2026)

### Context
`cda/schemas/uscdi_v3.json` (loaded by `uscdi/vocabulary.go`) is the one ONC-verified USCDI v3
dataset in this codebase — kept deliberately separate from the schema's own `uscdiClass`/
`uscdiElement` display labels (unverified, used only for Requirements-catalog grouping and, as of
this round, confirmed to double as literal narrative `<th>` text — see the bug note below). A prior
round of work expanded it to all 19 USCDI v3 classes / 92 elements and closed 5 of 7 remaining gaps
by checking the real spec text instead of stopping at "not found in the Go code." This round closes
2 more: Fill Status and UDI's core Device Identifier, both requiring new schema capability (not just
a bridge to an existing field), so both directions (parse + build) were scoped and built together.

### Fill Status — `medications.json`'s `medicationDispenses` RepeatingGroup
Medication Dispense (V2) (`.4.18:2014-06-09`) attaches to Medication Activity via a 0..*
`<entryRelationship typeCode="REFR">` wrapping a `<supply>` act (CONF:1098-7549/7553/16090,
distinct from Medication Activity's OTHER REFR relationship, to Medication Supply Order `.4.17`,
which isn't modeled in this schema — no collision risk). `statusCode/@code` is bound to ValueSet
**"Medication Fill Status"** — that's the actual field; the Companion Guide's own crosswalk table
label ("supply/code") is imprecise relative to the real constraint table. Implemented as a new
`RepeatingGroup` — `wrapperTag: "entryRelationship"`/`wrapperAttr: "typeCode"`/`wrapperAttrValue:
"REFR"`, `observationElementPath: "supply"` — the exact same mechanism `indications` already
proved, zero new Go code. Fields: `fillStatus`, `dispenseQuantity`, `dispenseDate`,
`dispenseSequence`.

### UDI — Tier 1 (Product Instance's own `id`) and Tier 2 (full UDI Organizer) both done
**Tier 1**: the base C-CDA R2.1 IG's OWN Product Instance template (`.4.37`, not a Companion Guide
addition) is already the sanctioned UDI/DI capture point — its own prose: *"When sending a UDI,
populate the participantRole/id/@root with the FDA OID and participantRole/id/@extension with the
UDI. When sending a DI, populate participantRole/id/@root with the assigning agency OID and
.../id/@extension with the DI."* `medicalEquipment.json` already anchors Product Instance
(`entry/supply/participant[@typeCode='PRD']/participantRole`) but had never modeled its `id` —
added `deviceIdentifier`/`deviceIdentifierSystem` as two plain fields, no new template, no new Go
code. This alone satisfies the USCDI "Unique Device Identifier (UDI)" element for both directions.

**Tier 2**: the Companion Guide's richer **UDI Organizer** (`.4.311:2019-06-21`, 11 further
sub-observations: Lot/Batch Number, Serial Number, Manufacturing/Expiration Date, Brand Name, Model
Number, Catalog Number, Company Name, MRI Safety, Latex Safety, Implantable Device Status, Distinct
Identification Code). Confirmed via the appendix's own Figure 1 worked example and each
sub-observation's own Contexts table: all 11 are `Contained By: UDI Organizer` only, and the
Organizer itself nests under Procedure Activity Procedure (`medicalEquipment.json`'s existing
`procedureEntries` alternate archetype — a DIFFERENT anchor than Tier 1 uses) via
`<entryRelationship typeCode="COMP">` (confirmed against the main Companion Guide body, not just the
appendix), with its own `<id>` required to VALUE-MATCH Product Instance's `id` for correlation — not
a structural parent/child link.

This needed a genuinely new schema construct — `ComponentGroup`/`ComponentDef`
(`cda/schema_types.go`), a shared `<organizer>` wrapper containing a FIXED, enumerated set of
independently-optional, differently-shaped sub-observations (each its own templateId/fixed NCIt
code/datatype). Neither `RepeatingGroup` (assumes N items of the *same* shape) nor
`StructuralTemplateAnchor` (one fixed path) covers this — confirmed by reading
`cda/builder/xpath_writer.go`'s own disambiguation logic: a bare, repeated `<component>` tag has no
attribute to predicate on, so `findOrCreateChild`'s discriminator-lookahead (which only fires when
the AMBIGUOUS segment itself carries a `[predicate]`) can't safely tell 12 differently-shaped
components apart. New Go function `writeComponentGroups` (`cda/builder/entry_archetypes.go`) always
DIRECT-CREATES a fresh sibling per present component instead — the same "never risk a predicate
match" approach `writeRepeatingGroups` already uses for its own items.

The id-correlation requirement is satisfied for free WITHIN Tier 2 by pointing
`OrganizerIDField`/`OrganizerIDSystemField` at the SAME `deviceIdentifier`/`deviceIdentifierSystem`
keys the Device Identifier component's own value reads — one caller-supplied value, read twice from
the same record. It's NOT automatically tied to Tier 1's own value, though: Tier 1's Product
Instance lives in `medicalEquipment`'s primary `entries[]` array, Tier 2's Organizer lives in the
`procedureEntries[]` alternate — genuinely separate records (possibly from different source rows via
`cda.map_to_canonical`'s cross-table join), so keeping the SAME literal value in both arrays is the
caller's responsibility, same as any other cross-array correlation in this engine.

**Bug found and fixed during Tier 2 implementation**: none of the 12 sub-observation constraint
tables declare a `<statusCode>` element (confirmed individually for several, plus every worked XML
example) — but `tagBoilerplate["observation"]`'s generic `StatusCode: "completed"` default would
have auto-injected one onto every component anyway. `writeComponentGroups` now strips any
auto-injected `<statusCode>` from each component (the shared organizer's OWN
statusCode="completed", which IS spec-required, is untouched) — regression-guarded by an explicit
count assertion in the round-trip test.

### Bug found: `USCDIElement` is also the literal narrative table header
`cda/builder/narrative.go:73-77` uses `CDAFieldDef.USCDIElement` directly as `<th>` text in the
generated document's own narrative table — confirmed by a failing test assertion during this round
(a verbose parenthetical-rationale label for `deviceIdentifier` showed up whole, HTML-escaped, as a
column header in built XML). `USCDIElement` must stay a short display label; longer rationale
belongs in CLAUDE.md/the plan file, not the JSON. Fixed for `deviceIdentifier`/`fillStatus` and 3
pre-existing instances found via a repo-wide grep for the same pattern: `functionalStatus.json`'s
`disabilityStatus` (from the prior A2.5 round), `advanceDirectives.json`'s 5
`referencedDocument*` fields, and `results.json`'s 2 "Reference Range ... Inclusive Flag" fields —
all shortened to clean labels, re-verified via the golden snapshot + full `cda`/`uscdi` test suite
(green, zero regressions).

### Key Files
- `cda/schemas/uscdi_v3.json` — Fill Status and UDI entries now point at real fields (87/92 mapped)
- `cda/schemas/ccda_2_1/sections/medications.json` — `medicationDispenses` RepeatingGroup
- `cda/schemas/ccda_2_1/sections/medicalEquipment.json` — `deviceIdentifier`/`deviceIdentifierSystem` fields (Tier 1) + `procedureEntries`'s `udiOrganizer` ComponentGroup, 12 components (Tier 2)
- New engine mechanism: `cda/schema_types.go` (`ComponentGroup`/`ComponentDef`, `AlternateEntryArchetype.ComponentGroups`), `cda/schema_disk_types.go` (raw passthrough), `cda/builder/entry_archetypes.go` (`writeComponentGroups`, wired into `buildAlternateEntry`)
- Tests: `cda/builder/document_builder_test.go` (`TestBuildDocument_MedicationDispense_FillStatus_BuildsAsREFRSupply`, `TestBuildDocument_MedicalEquipment_DeviceIdentifier_BuildsOnProductInstanceId`, `TestBuildDocument_MedicalEquipment_UDIOrganizer_ComponentGroup`), `cda/builder/entry_archetypes_test.go` (4 synthetic `TestBuildAlternateEntry_ComponentGroup_*` unit tests)
- Plan (full design rationale): `C:\Users\ShanawazKhan\.claude\plans\soft-napping-shell.md`

## USCDI v3 Vocabulary Bridge — Phase C: Build Requirements (August 2026)

### What shipped
Same USCDI class signal Phase B gave Coverage Audit, now also on the guided-configuration
Requirements catalog (`cda.map_to_canonical`/`cda.build`'s SHALL/SHOULD/MAY checklist):
`SectionRequirement.USCDIClasses` (`cda/builder/requirements_catalog.go`) and
`HeaderFieldRequirement.USCDIClasses` (`cda/builder/header_requirements.go`), both resolved from
the real `cda/schemas/uscdi_v3.json` dataset, additive and honestly-absent-when-unmapped — never
fabricated. `CDASchemaController` (`controllers/cda_schema_controller.go`) gained a constructor-
injected `vocabulary *uscdi.USCDIVocabulary` field (DI, matching `db`/`loader`/`mapper`, not a
global singleton); `main.go` constructs its own instance the same per-consumer way Coverage Audit's
`coverageVocab` already does (not shared — different lifecycle/scope). `CDARequirementsHelper.js`
gained `renderUSCDISummary`, a small blue "USCDI v3: represents N classes — A, B, C" banner, wired
into both `CDAStepBuilder.js`'s Requirements tab and `MapToCanonicalBuilder.js`, additive to (never
replacing) the existing SHALL/SHOULD/MAY completeness banner.

### DRY fix found along the way: class-resolution logic promoted to the vocabulary itself
Phase B's `services/cda_coverage/inventory.go` already had this exact dedup/sort logic as an
unexported `uscdiClassesForSection` helper. Rather than copy-pasting a second implementation for
`cda/builder` (which can't import `services/cda_coverage` — wrong dependency direction), the logic
moved up to a new exported `uscdi.USCDIVocabulary.ClassesForSection(sectionKey string) []string`
method (`uscdi/vocabulary.go`), which BOTH `services/cda_coverage` and `cda/builder` now call —
`inventory.go`'s own function is now a 1-line wrapper. Includes a `v == nil` guard so a gracefully-
degraded nil vocabulary (either consumer's own load-failure fallback) never panics.

### Real wrinkle found during implementation: dotted canonical keys need prefix matching
`header_requirements.go`'s own translation table splits ONE schema field — uscdi_v3.json's bare
`"address"` cdaField in `header.patient.json` — into 4 separate UI-facing requirement rows
(`address.street`/`.city`/`.state`/`.postalCode`). A literal `CDAField == canonicalKey` check (as
originally planned) would have silently found zero matches for all 4. Fixed in a new
`classesForHeaderField` helper: when canonicalKey is dotted, also match on the part before the first
`.` against uscdi_v3.json's coarser cdaField.

### Test correction: `results` maps to Clinical Tests + Laboratory, not Diagnostic Imaging
The original plan's illustrative test case assumed `results`'s multi-class set was Diagnostic
Imaging + Laboratory. The real, current data (post-A2.5's Clinical Tests fix) is **Clinical Tests +
Laboratory** — Diagnostic Imaging maps to the separate `findingsDIR` section instead (see the
C-CDA R2.1 Document-Type Coverage section above). Caught by running the test, not assumed.

### Verification status — Go AND UI both done
Go side: `go vet`/`go build .`/`go test` across `cda`, `cda/builder`, `cda/document`,
`cda/validator`, `uscdi`, `controllers` — all green (a full `go build .` now also succeeds; the
pre-existing, unrelated `fhir` package compile break noted earlier this session was resolved by
other in-progress work by the time Phase C landed). UI side: app container rebuilt from current
source (`docker-compose build app && docker-compose up -d app`), then `tests/playwright/
cda-guided-config.spec.js` extended with 4 new permanent tests (CDA-GC0-005/006 for the API's
`uscdiClasses` field, CDA-GC1-005/006 for the rendered banner text in both
`#mapToCanonicalBuilder` and `#cdaBuildTab-requirements`) — all pass, confirming the whole chain
end-to-end. Found and fixed 3 unrelated pre-existing test-staleness bugs along the way (CDA-GC0-002/
CDA-GC1-001 still expected CCD's `planOfTreatment` as SHALL, corrected to SHOULD in the earlier
Document-Type Section-List Audit above but the tests hadn't been updated; CDA-GC1-002's Street
Address locator was unscoped and started matching both Custodian's and Legal Authenticator's own
fields once the latter shipped) — none related to Phase C itself.

### Key Files
- `uscdi/vocabulary.go` — new `ClassesForSection` method (shared by both Phase B and Phase C)
- `services/cda_coverage/inventory.go` — `uscdiClassesForSection` now a thin wrapper
- `cda/builder/requirements_catalog.go`, `cda/builder/header_requirements.go` — `USCDIClasses` fields + resolution
- `controllers/cda_schema_controller.go`, `main.go` — vocabulary DI wiring
- `public/js/pipeline/utils/CDARequirementsHelper.js` — `summarizeUSCDICoverage`/`renderUSCDISummary`
- `public/js/pipeline/components/CDAStepBuilder.js`, `public/js/pipeline/components/MapToCanonicalBuilder.js` — wired the new summary in
- Tests: `cda/builder/requirements_catalog_test.go` (6 new tests), `tests/playwright/cda-guided-config.spec.js` (4 new tests, browser-verified)

## EDI X12 Support — Phase 1 (835 Remittance, SFTP-only, September 2026)

### Why it exists
EDI X12 was never built in this codebase before this phase — the only prior EDI-aware code was `processing/batch_splitter.go`'s `splitEDITransactions` (ISA/GS/ST/SE interchange splitting at ingestion). This phase proves the whole pattern end-to-end on one transaction set — **835 (Health Care Claim Payment/Advice)** — over SFTP only. 837 (claims), 270/271 (eligibility), AS2 transport, and X12→FHIR mapping are named future phases, not attempted here.

### Architecture: a schema-driven engine, mirroring CDA's own precedent but reshaped for X12
New package `edi/`, built bottom-up as explicit, reusable layers — the same anti-hardcoding principle CDA's `cda/builder` demonstrates (schema data drives structure; adding a new transaction set requires zero new Go), but X12 doesn't need CDA's XML-predicate machinery (loops nest unambiguously by position + strict segment order, not by attribute matching):
- **Layer 0 — data types** (`edi/datatypes.go`): the ~12 standard X12 types (AN, ID, DT, TM, R, B, N0-N9). `Validate`/`Parse`/`Format`.
- **Layers 1-2 — elements, composites, intra-segment repeats, segments** (`edi/schema_types.go`): `X12ElementDef`, `X12CompositeDef` (e.g. SVC01's `HC:99213`), `X12RepeatDef` (e.g. CAS's reason/amount/qty trio, repeating up to 6× within ONE segment instance), `X12SegmentDef` — one file per segment ID in `edi/schemas/x12_005010/segments/*.json`, authored once, referenced by every loop that uses it (N1/N3/N4/REF/DTM/AMT/PER/QTY/LQ are already shared-reuse candidates for 837 in a later phase).
- **Layers 3-4 — loops and transaction sets** (`edi/schema_types.go`, `edi/schemas/x12_005010/transactionSets/835.json`): `X12LoopDef` (recursive `Loops []*X12LoopDef` — a parent/child containment tree, no separate "loop relation" type needed), `X12TransactionSetDef` (pure composition — header/loop-tree/trailer, each entry a segment-ID reference into the shared library, never a re-authored element list).
- **Engine**: `edi/loop_engine.go`'s `ParseTransactionSet` (ONE generic recursive walk, no per-segment/per-transaction-set Go) and `edi/builder/document_builder.go`'s `BuildDocument` (the write-direction mirror — canonical JSON → ISA...IEA text). `edi/segment_reader.go`'s `DetectDelimiters` reads element/segment/sub-element/repetition separators live from each file's own ISA header (never hardcoded `*`/`~`) — falls back to conventional defaults only for the bare-`ST` shape `batch_splitter.go` produces when splitting a multi-transaction-set file.
- **Validator** (`edi/validator/validator.go`): two distinct severities, a deliberate product decision (2026-09-01, see below) — a malformed VALUE (wrong type/length/fixed-value mismatch) is an `error` (blocks `Result.Valid`); an X12 element-relational (P/C/L/R/E) `SyntaxRule` violation (e.g. "if BPR06 present, BPR07 required too") is a `warning`, **never blocking**.

### Product decision: flexible, not rigid — validated by real production data
This project's own stated goal (user, 2026-09-01): *"allow users to parse, build EDI segments, convert to FHIR without being too rigid, giving them the flexibility."* Concretely: business-rule/conditional constraints (X12's P/C/L/R/E syntax notes) are checked and surfaced, but **never** block parsing, building, or `Result.Valid` — only a genuinely malformed field value does that. Confirmed against real, unedited 835 samples (see below): the unmodified blueCrossNC sample already carries one legitimate SyntaxRule warning with `valid: true` — proving the mechanism fires on real-world data without being punitive about it.

### Spec sourcing — a real, named trade-off (not TR3)
X12's 835 TR3 (Technical Report Type 3) is a paid document (X12.org/Washington Publishing Company) — no copy exists in this repo. User-decided (2026-08-31): build from **free, cross-referenced companion guides** (Stedi's X12-005010-pinned segment reference + PyX12's own 835.5010.X221.A1.xml IG map) instead of purchasing the TR3 for phase 1. A real, confirmed version-mismatch bug was caught and fixed during a thoroughness re-verification pass: several early fetches used **unversioned** Stedi URLs that silently default to X12 Release 8010 instead of 5010 — most severely corrupting `CLP` (wrong field count/positions entirely). Fixed by re-fetching with version-pinned URLs (`stedi.com/edi/x12-005010/segment/...`) and cross-validating against PyX12. Every `schemas/segments/*.json` file records its own `sourceRefs` provenance. **A real trading-partner pilot needs that specific payer's own companion guide or the licensed TR3** — named as a pre-pilot gate, not a phase-1 blocker.

### Pipeline integration — three steps, and what "automatic" means here
`edi.parse` / `edi.validate` / `edi.build` (`services/executors/transform/edi_{parse,validate,build}_executor.go`), registered in `services/executor_registry.go`. Important nuance, since it mirrors CDA's own relationship to auto-conversion: **EDI messages are parsed automatically after connector ingestion already**, the same generic path HL7/CDA use — `processing/engine_message_processor.go` calls `MessageParserService.ParseToJSON` on every message (the only special case is `sourceType == "http_fhir"`, unrelated to EDI), which calls `FormatDetector.DetectFormat` (its `isEDI()` check, position 7, recognizes both ISA-prefixed and bare-`ST` content) then `ParserFactory.GetParser(models.FormatEDI)` — now resolving to the real `services/parsers/edix12.EDIX12ParserService`, not a raw passthrough. So **`edi.parse` is NOT required to get structured JSON out of an inbound 835** — it exists for the same reason `cda.parse` does: re-parsing EDI content that shows up mid-pipeline from somewhere other than the original connector (a DB lookup step, a mid-pipeline `connector.inbound` pull, etc.), not as a prerequisite.
- `edi.validate` deliberately re-parses from **raw** content (`sourceField` defaults to `"raw"`, not `"parsedEDI"`) rather than trusting a prior step's JSON output — the validator's SyntaxRule checks need the typed `SegmentInstances` data, which doesn't survive the JSON pipeline boundary. A documented, deliberate deviation from this phase's own original design note.
- `edi.build`'s `mergeInterchangeConfig` layers the step's own ISA/GS config (`isaSenderId`/`isaReceiverId`/`gsSenderCode`/`gsReceiverCode`) on top of whatever source data already supplies, **without overwriting** real values already present (e.g. a round-tripped sender ID from a prior `edi.parse`) — same "config fills in what data doesn't supply" precedent as CDA's `CdaCustodianConfig`.
- Outbound delivery reuses the existing generic `connector.outbound` bridge step unmodified — `connectorType: "edi_x12_outbound"`, `contentField` pointed at `edi.build`'s own output field — zero new bridging code, automatic DLQ coverage.

### Connectors — SFTP-only, transport-only "dumb byte shippers"
`edi_x12_inbound`/`edi_x12_outbound` (`services/connectors/edi_x12_{inbound,outbound}.go`) — real SFTP protocol (`github.com/pkg/sftp` + `golang.org/x/crypto/ssh`), structurally mirroring `sftp_inbound.go`/`sftp_outbound.go`. `Validate()` explicitly rejects any `transport` other than `"sftp"` with a clear phase-1 message (the catalog schema narrows the enum too, but this is where the real capability boundary is enforced). Inbound does no pre-splitting/transaction-type filtering — `batch_splitter.go` already runs centrally on every connector's payload. All X12 envelope/business logic lives in `edi.build`, not the connector, matching every other connector in this codebase. Activated via `database/migrations/V229__Activate_EDI_X12_835_Connectivity_Types.sql` (both types were deactivated stubs since V221; V229 does a full config_schema overwrite matching the real Go structs, the same safe pattern V222/V224/V225 established — zero real usage existed to migrate).

### Test coverage
- `edi/`, `edi/builder/`, `edi/validator/`: synthetic-schema unit tests (delimiter detection, nested/repeating loops, round-trip build↔parse, SyntaxRule P/C/L/R/E logic) plus real-sample integration tests against 3 unedited 835 files sourced from `keironstoddart/edi-835-parser`'s own GitHub test fixtures (`edi/testdata/real_samples/`) — a bare-`ST`-no-envelope sample, a full-envelope 3-claim sample, and a non-standard-component-separator (`>` instead of `:`) sample, proving live delimiter detection.
- `services/executors/transform/edi_{parse,validate,build}_executor_test.go` + `edi_pipeline_integration_test.go`: executor-level coverage against the same real samples, including a genuine `edi.parse` → `edi.build` chained round trip and both directions of `mergeInterchangeConfig`'s "don't clobber real data" contract.
- `services/connectors/edi_x12_test.go`: pure config/`Validate()`/filename-resolution logic — no live SFTP server in this environment (same documented limitation as `sftp_outbound_test.go`).
- A real bug this suite caught: `services/parser_factory_test.go`'s `TestParserFactory_GetParser_PreviouslyUnsupportedFormatsNowSucceed` assumed `FormatEDI` always had a (raw-passthrough) parser; once EDI became schema-backed (CWD-dependent, like CDA), the test needed the same "skip strict check" treatment CDA already had in this file — fixed to match that existing precedent rather than working around it.

### Key Files
- Engine: `edi/datatypes.go`, `edi/schema_types.go`, `edi/schema_loader.go`, `edi/segment_reader.go`, `edi/loop_engine.go`
- Build direction: `edi/builder/document_builder.go`, `edi/builder/segment_writer.go`
- Validator: `edi/validator/validator.go`
- Schema data: `edi/schemas/x12_005010/{manifest.json,envelope.json,segments/*.json,transactionSets/835.json}`
- Parser adapter: `services/parsers/edix12/edi_x12_parser_service.go`
- Pipeline steps: `services/executors/transform/edi_{parse,validate,build}_executor.go`
- Connectors: `services/connectors/edi_x12_{inbound,outbound}.go`
- Migration: `database/migrations/V229__Activate_EDI_X12_835_Connectivity_Types.sql`
- Real sample fixtures: `edi/testdata/real_samples/*.txt`

### Named future phases (STATUS UPDATE — all 5 phases below are now COMPLETE; kept for history, not a live TODO list)
- **Phase 2**: 837 (claims) — DONE, see "EDI X12 Support — Phase 2" below.
- **Phase 3**: 270/271 (real-time eligibility) — DONE, transform-only (no new connector/transport needed after all — see `project_edi_phase3_eligibility.md`); also see "Sync Eligibility" work for a synchronous HTTP variant.
- **Phase 4**: AS2 transport (signed/encrypted HTTP + MDN) — DONE, as a genuinely new `as2_inbound`/`as2_outbound` connector pair, NOT a widened `transport` enum on `edi_x12_inbound`/`edi_x12_outbound` (those stay SFTP-only permanently — see the Connector Catalog's "Healthcare Protocol Connectors" entry).
- **Phase 5**: X12→FHIR mapping — DONE for 835 (this section's own "EDI X12 835 → FHIR Mapping" below, per-claim `ExplanationOfBenefit` + `PaymentReconciliation`) and for 837P/837I (see "EDI X12 837P/837I → FHIR Mapping" below, `Claim`/`Patient`/`Coverage`/`Organization`). 999→FHIR remains a deliberate non-goal (a transport-layer acknowledgment has no natural FHIR analogue).

## EDI X12 — Pipeline UI Completion (Step Config UI, Toolbox, Mapping, Custom Rules, September 2026)

### Why it exists
Phase 1 above shipped a fully working backend (engine, parser adapter, connectors, 3 pipeline executors) with **zero UI surface** — confirmed by direct investigation of the pipeline-builder frontend, not assumption. Two real gaps: (1) no step-config UI existed for any `edi.*` step type (the Properties Panel's dispatch chain — hardcoded cases → `StepBuilderRegistry` → a legacy declarative field map → nothing — had zero entries for EDI); (2) no way to get one of these steps onto a pipeline at all (the toolbox's "All Steps" palette is a hardcoded ~16-item list with no CDA/HL7-build/FHIR-build/EDI entries either — the real mechanism for "advanced format" steps is an OOB **Interface Template**, e.g. `V152__CDA_OOB_Pipeline_Template.sql`, not drag-and-drop from a blank toolbox). When scoping this fix, the user reframed it with two explicit new product requirements: `edi.validate` should support user-defined rules layered on the base standard, and `edi.build` should be able to generate an 835 from "any format," not just a prior `edi.parse` result.

### 1. EDI schema-introspection API
`controllers/edi_schema_controller.go` — mirrors `cda_schema_controller.go`'s pattern at a much smaller scale: `GET /api/edi/schema/segments` (every segment's name/elements/repeats), `GET /api/edi/schema/transaction-sets/:id/loops` (the recursive loop tree). Wired in `main.go` next to the CDA/FHIR/HL7 schema-controller blocks; proxied via `app.js`'s `app.use('/api/edi', forwardToGo)`. Feeds the no-code UI pieces below so segment/element/loop names are always live against the real schema, never hardcoded in JS.

### 2. Step-config UI (`public/js/pipeline/components/EDIStepBuilder.js`)
Four classes (`EdiParseStepBuilder`, `EdiValidateStepBuilder`, `EdiMapToCanonicalStepBuilder`, `EdiBuildStepBuilder`) following `CDAStepBuilder.js`'s `CdaParseStepBuilder` contract exactly — plain class, `constructor(panel)`, `render(step) → HTML string`, `collectConfig(step)`, `destroy()`, registered via `StepBuilderRegistry.register(...)`. **Zero changes needed to `PropertiesPanel.js`** — its generic fallback (`if (StepBuilderRegistry.has(registryType))`) already dispatches any registered type. Async schema-data loads (segment/loop catalogs) use the same `fetch().then(() => this._rerender())` + `outerHTML` replace idiom `MapToCanonicalBuilder.js` already established — `render()` returns synchronously with a "Loading…" placeholder, the async fetch swaps in real content once it lands.

### 3. `edi.map_to_canonical` — build an 835 from any source shape
New `services/executors/transform/edi_map_to_canonical_executor.go`, step type `edi.map_to_canonical`. **Not a reuse of `cda.map_to_canonical`**: that step is tightly coupled to CDA's own canonical shape; EDI's shape (segment-ID-keyed header, loop-ID-keyed **recursively nested** loops — 835 alone needs 2000→2100→2110, 3 levels deep) needed its own mapper. Config mirrors `edi.X12LoopDef`'s own recursive tree shape (`header: [...]`, `loops: [{loopId, rowsPath?, fields, loops?}]`) — one recursive Go function (`buildLoopMapping`/`buildLoopInstance`) handles any depth with no hardcoded limit.

**A real bug an initial "schema-blind" design assumption missed, caught by its own round-trip test**: `edi/builder.writeLoops` decides array-vs-single-map OUTPUT shape from the SCHEMA's own `X12LoopDef.RepeatsMultiple()` flag, not from whatever shape the source data happens to be — the same "schema-driven, not observed-count-driven" rule already established for parsing. A loop mapped as single-instance (no `rowsPath`) but landing in a schema-repeating slot (e.g. 2100, always `>1`) would silently produce a bare map `edi.build`'s writer can't consume (`toInterfaceSlice` returns nil for a non-array). Fixed by making the executor self-init an `*edi.X12SchemaLoader` (same convention as the other 3 executors) and consulting the loaded transaction set's loop tree at Execute() time to decide output shape — `rowsPath` alone still controls *how many* rows to map, but the schema's own cardinality decides the wrapping, independent of the step's own config choice.

Reuses `map_to_canonical_executor.go`'s own package-level primitives directly (same package `transform`): `executors.GetFieldValue`, `resolveRows`, `stringifyValue`, `applyCanonicalTransform`. Two new entries added to the shared transform registry (`canonical_value_transforms.go`): `date_to_x12` (reuses `canonicalDateToCDA`'s own function unchanged — its CCYYMMDD output is byte-for-byte X12's own DT format despite the CDA-era name) and `time_to_x12` (new, HHMM). **Deliberately out of scope**: trailer/PLB mapping (a bare repeating segment, not a loop — doesn't fit the loop-mapping recursion and would need its own UI this pass didn't build; `edi.build` still accepts a hand-supplied `trailer` key, just not produced by this step).

### 4. Custom validation rules for `edi.validate`
Reuses the **existing P/C/L/R/E `SyntaxRule` model**, user-supplied, layered on the OOB spec rules — not a second rule language (the user deferred to this judgment call: P/C/L/R/E already covers exactly the class of constraint a trading partner's own companion guide adds on top of base 5010, and costs zero new backend logic since `evaluateSyntaxRule` already implements it fully).

- `edi_validate_executor.go`: new `customRules: []{segmentId, type, positions: []string}` config (positions given as element **keys**, never raw numeric positions — the UI never shows those). `specWithCustomRules` clones only the targeted segments' own `*edi.X12SegmentDef` (never mutates the loader's shared, cached spec — that would leak one pipeline's custom rules into every other validation) and appends translated rules.
- **A real ordering bug caught by its own test**: the merge must happen **before** `edi.ParseTransactionSet`, not after — `parseSegmentInstance` only records a segment's raw values into `ParseResult.SegmentInstances` (the validator's own data source) when that segment's spec entry *already* has at least one `SyntaxRule`. Merging after parsing meant every custom rule on a segment with zero OOB rules (e.g. N3) was silently never checked, since parsing never bothered recording its `SegmentInstance` in the first place.
- `edi.SyntaxRule` gained a `Source string` field (empty = OOB spec rule, `"custom"` = step-supplied) and `edi/validator.Issue.Source` copies it straight through — lets the UI (and a debugging user) distinguish which rule fired.
- UI: `EdiValidateStepBuilder`'s "Add Custom Rule" — segment dropdown (live catalog) → rule type (5 options, each with a plain-English one-line description) → checkboxes of that segment's own named elements. Scoped to a segment's plain `Elements` only in phase 1, not intra-segment `Repeats` groups (e.g. CAS's trios) — those already have OOB rules and would need a repeat-index picker this pass didn't build.

### 5. Toolbox visibility + OOB interface template
- `ToolboxManager.js`'s `getBuiltInTemplates()`: 4 new `StepTemplate` entries (`edi.parse`/`edi.validate`/`edi.map_to_canonical`/`edi.build`), matching each executor's real Go config defaults, plus matching `getIconForType` icon-map entries. Confirmed via direct DB query that `edi_x12_inbound`/`edi_x12_outbound` (the connectors, not the transform steps) already appeared correctly in the existing generic `connector.inbound`/`connector.outbound` dropdown before this work — no separate toolbox entries needed for those.
- New migration `V230__EDI_835_OOB_Pipeline_Template.sql`, mirroring `V152`'s structure: `connector.inbound` (`edi_x12_inbound`, SFTP) → `edi.parse` → `edi.validate` → `connector.outbound` (`sink_outbound` — a store-only terminal, honestly reflecting that no X12→FHIR mapping exists yet rather than pretending delivery is finished). `edi.map_to_canonical`/`edi.build` deliberately not part of this template — the 4 step types are independent, individually-toolbox-addable capabilities; this template is for the inbound-ingestion use case specifically.

### A real deployment bug found only by a container smoke test
`go build`/`go test` never catch this class of bug because they run against the full source tree. The Dockerfile's runtime stage explicitly `COPY`s `cda/` and `uscdi/` for the Go binary to read at startup — but never copied `edi/`, meaning **every** EDI schema-dependent code path (not just the new schema controller — `edi.parse`/`edi.validate`/`edi.build` too) would 503/fail in any real deployed container, undetected since Phase 1 shipped, because verification had only ever run `go build`/`go test` against the checked-out repo or a throwaway `golang:alpine` container with the full source bind-mounted. Caught by actually curling the new endpoint against a real running container (`docker-compose up -d app`) as part of this round's mandatory browser verification — fixed by adding `COPY edi/ ./edi/` to the Dockerfile alongside the `cda/`/`uscdi/` lines.

### Verification
Go: new `edi_map_to_canonical_executor_test.go` (flat header, single-instance loop, 3-level nested repeating loops, chained into `edi.build` + re-parsed round trip) and extended `edi_validate_executor_test.go` (custom rule fires alongside OOB rules with `Source="custom"`, unknown segment/element-key degrades gracefully, never blocks `Valid`). Browser: `tests/playwright/edi-pipeline-ui.spec.js` — toolbox visibility, `StepBuilderRegistry` registration, each step's own config panel renders its real default values (not just generic boilerplate), the custom-rule builder's live segment-catalog fetch, and the map-to-canonical loop tree rendering the real recursive schema (1000A/1000B/2000→2100→2110) — run against a real running container, zero console errors.

### Key Files
- `controllers/edi_schema_controller.go`, `main.go` (wiring), `app.js` (proxy line)
- `public/js/pipeline/components/EDIStepBuilder.js`, `public/pipeline-builder.html` (script tag)
- `services/executors/transform/edi_map_to_canonical_executor.go`, `services/executors/transform/canonical_value_transforms.go` (2 new transforms), `services/executor_registry.go`
- `services/executors/transform/edi_validate_executor.go` (customRules + `specWithCustomRules`), `edi/schema_types.go` (`SyntaxRule.Source`), `edi/validator/validator.go` (`Issue.Source`)
- `public/js/pipeline/managers/ToolboxManager.js` (4 `StepTemplate` entries + icons)
- `database/migrations/V230__EDI_835_OOB_Pipeline_Template.sql`
- `Dockerfile` (`COPY edi/ ./edi/` fix)
- Tests: `services/executors/transform/edi_map_to_canonical_executor_test.go`, extended `edi_validate_executor_test.go`, `tests/playwright/edi-pipeline-ui.spec.js`

### Follow-on: the SAME toolbox gap existed for 13 other step types too
Prompted by the user asking "any step we build should be accessible from toolbox" — a fair, broader read of the gap above than just EDI. A systematic diff (`StepBuilderRegistry`'s registered keys vs. `ToolboxManager.js`'s toolbox types, not a guessed list) found 13 more real, working, `StepBuilderRegistry`-registered step types with zero toolbox entry: `cda.parse`, `cda.normalize`, `cda.to_fhir`, `cda.build`, `fhir.to_cda`, `cda.dedupe`, `cda.map_to_canonical`, `cda.section_to_csv`, `fhir.build`, `hl7.build`, `payload.builder`, `deidentify`, `pas_envelope_mapping` — all fixed the same way (new `StepTemplate` entries in `getBuiltInTemplates()` with each executor's own real Go config defaults, plus icon-map entries). 6 further apparent gaps (`pre.enrichment.api`, `database_enrichment`, `pre.enrichment.database`, `pre.deidentify`, `post.deidentify`, `payload_builder`) are legacy/alias type names that deliberately share their canonical name's existing toolbox card, not real gaps.

**A permanent regression guard, not just a one-time fix**: `tests/playwright/toolbox-step-coverage.spec.js`'s first test diffs `StepBuilderRegistry`'s registered keys against the toolbox's own types at runtime (against an explicit, individually-confirmed alias allowlist) — it fails loudly for ANY future step type that ships a working builder but no toolbox entry, not just the 13 named here. A second test confirms each of the 13 newly-toolboxed types' own real default config actually opens its config panel without a console error (verified live in a real browser — `cda.build`'s screenshot showed its full General/Requirements/Custodian/Legal Authenticator tabs and all 12 real document types rendering correctly, not just the generic wrapper).

## EDI X12 Support — Phase 2 (837 Professional + Institutional Claims, 999 Functional Acknowledgment, September 2026)

### Scope
User-confirmed up front: both 837P and 837I (not a reduced MVP), 999 auto-fire in scope, same free-companion-guide sourcing discipline as 835 (no TR3 access), full completeness for every loop in both variants, and pipeline/toolbox/UI parity built in the *same* pass — Phase 1 shipped 835 with zero UI surface and needed a whole separate follow-up round to fix that; this phase deliberately didn't repeat it. Spec sourced the same way as 835: PyX12's real IG maps (`837.5010.X222.A1.xml`, `837Q3.I.5010.X223.A1.xml`, `999.5010X231.A1.xml`), downloaded and read directly (a Python XML-parser script extracted a compact loop/segment skeleton first — the file is 25,900+ lines, too large to read wholesale — then per-position element detail was extracted the same way), cross-checked against version-pinned Stedi pages.

### 3 architecture fixes (before any 837/999 schema data was written)
835's own shared files (`envelope.json`, `segments/ST.json`) hardcoded GS01/GS08/ST01 as 835-specific `fixedValue`s — harmless while 835 was the only transaction set, actively wrong the moment a second one shares these files.
1. **De-hardcoded GS01/GS08/ST01** off the shared files onto `X12TransactionSetDef` itself (`schema_types.go`): `STTransactionSetID` (the real wire ST01 value, e.g. `"837"`, when it differs from the schema's own more-specific registry key), `FunctionalIdentifierCode`, `VersionReleaseIndustryCode`. `EffectiveST01()` resolves which to use. `edi/builder`/`edi/validator` read these instead of relying on element-level `FixedValue` for these 3 fields specifically — every other element's `FixedValue` mechanism is untouched.
2. **Composite ST01+GS08 lookup** (`loop_engine.go`'s `ParseTransactionSet`): 837P and 837I are NOT distinguishable by ST01 alone (both are literally `"837"`) — GS08 is the real disambiguator (required in every real GS segment, unlike ST03, which many trading partners leave blank). Tries `spec.TransactionSets[ST01+":"+GS08]` first, falls back to bare `spec.TransactionSets[ST01]` — zero behavior change for 835/999 (single-variant, registered under their bare ST01 as before). The manifest registers 837P/837I under that same composite key with a `friendlyId` (e.g. `"837P"`); `schema_loader.go`'s `load()` dual-registers each multi-variant set under BOTH the composite key and its friendly id in the SAME `spec.TransactionSets` map, so every other caller (`edi/builder.BuildDocument`, `GetTransactionSet`, `controllers/edi_schema_controller.go`) needs zero code changes to resolve a friendly id — only parse-time lookup needed to know about the composite form at all.
3. **`loopRef` shared-loop-template mechanism** (mirrors `cda/schemas/ccda_2_1/entries/*`'s proven pattern): the entire `2300` claim subtree (`2310A-F`, `2320→2330A-G`, `2400→2410/2420A-H/2430/2440`) appears in TWO tree positions within 837P (under `2000B` and `2000C`, subscriber-is-patient vs. dependent-is-patient) — confirmed via line-count comparison of both real occurrences in the IG map (~11,371 vs ~11,379 lines, same shape). `X12LoopDef` itself is unchanged; `{"loopRef": "2300-claim"}` is a load-time-only substitution (`schema_loader.go`'s `resolveLoopRefs`/`resolveLoopObjectInPlace`, operating on raw `map[string]interface{}` before the final typed unmarshal) — each reference gets a deep-copied, independently-owned tree (a fresh `json.Unmarshal` per occurrence). `edi/schemas/x12_005010/loops/2300-claim.json` (837P) and `loops/2300-claim-institutional.json` (837I — genuinely different shape: `CL1`/`SV2` instead of `SV1`, 9 Other-Payer sub-loops `2330A-I` instead of 7, both `MIA`+`MOA` together) are each authored once, referenced from both positions.

**A real bug found while wiring `readLoops`**: it initially copied `readSegments`' own "filename must equal the file's declared id" check — wrong for loop templates, whose filename is a *descriptive loopRef lookup key* (`2300-claim`), deliberately different from the loop's own real X12 id (`2300`). Fixed by dropping that check for loops specifically (segments still enforce it).

### 999 (Implementation Acknowledgment) — small, proved the pipeline first
7 new segments (`AK1`/`AK2`/`IK3`/`IK4`/`IK5`/`AK9`/`CTX`) + reuse of `ST`/`SE`/`GE`/`IEA`. Real structural finding: `IK5` sits *after* its parent loop's (`2000`) nested `2100` loop closes, not before it like every segment in 835's own tree — the engine only supported a loop's own segments *preceding* its children. Added `X12LoopDef.TrailerSegmentIDs` (`schema_types.go`) — the loop-level counterpart to `X12TransactionSetDef`'s existing Header/Trailer split, one level down — wired into `loop_engine.go`'s `matchLoopInstance` (matched after child loops) and `edi/builder/segment_writer.go`'s `writeLoopInstance` (written after child loops). `CTX` is genuinely reused at 3 different real positions (`2100`'s own "Segment Context" max_use 9, `2100`'s separate "Business Unit Identifier" max_use 1, `2110`'s "Element Context" max_use 10) with different per-position element constraints — modeled as the *permissive union* of all 3 (every element used in at least one position kept as situational), matching this project's "flexible, not rigid" validator philosophy; only the one sub-position (`CTX06-02`) that's Not-Used in literally all 3 was dropped.

### 837P (Professional) and 837I (Institutional)
837P: 27 new segments + `REF` extended (a genuine C040 composite usage 835 never needed) + a real bug in the *already-shared* `CUR.json`: its own `fixedValue: "PR"` (correct for 835's Assignment-of-Benefits context) was silently wrong for 837P's own `CUR` occurrence (loop `2000A`, needs `"85"` Billing Provider) — same class of bug as the GS01/ST01 fix above, just missed on the first pass; fixed the same way (removed the hardcoded value). 837I: 2 new segments (`CL1`, `SV2`) + `HI`'s composite extended (institutional genuinely populates the Date/Amount/Present-on-Admission sub-elements 837P never uses) + reuse of 835's own `MIA.json` unchanged (already structurally compatible).

**A real composite-alignment gotcha found and fixed** (documented in `CLM.json`'s and `HI.json`'s own `sourceRefs`, since it's a genuine engine property future segment authors need to know): unlike a segment's own top-level `Elements` (which align by their real numeric `Pos`, so dropping an unused *middle* position is safe), a **composite's `SubElements` align by Go slice INDEX**, not by their own `Pos` string (`edi/builder/segment_writer.go`'s `writeComposite` does `parts[i]`; `edi/loop_engine.go`'s `parseComposite` mirrors it on read) — `Pos` is purely informational there. Dropping an unused *middle* sub-position (CLM's `C024` sub03, HI's `C022` subs 6-8) would have silently shifted every later sub-value's wire position on every build/parse. Fixed by keeping reserved, never-populated placeholder fields (`_reserved3`, `_reserved6/7/8`) purely to preserve index alignment — dropping only *trailing* sub-positions (every other composite in this schema) remains safe.

Both proved via a full round-trip test exercising the `loopRef` mechanism's core correctness claim: `TestRealSchema_837P_BuildAndRoundTrip_LoopRefResolvesIndependently` builds ONE document with a claim under the subscriber's own `2300` AND a second, independently-valued claim under a dependent's `2000C→2300` — both resolve from the SAME `loopRef` but carry genuinely different data (`patientControlNumber`), proving the deep-copy-per-occurrence claim, not just that the loop resolves at all.

### Pipeline/UI parity (built this round, not deferred)
- `EDIStepBuilder.js`'s `EDI_TRANSACTION_SETS` grew from `['835']` to `['835','837P','837I','999']` — used by `edi.parse`/`edi.build`'s own pickers.
- `edi.map_to_canonical` gained a Transaction Set picker it had **none of before** — it always fetched 835's own loop tree regardless of step config, a real pre-existing gap this phase's own UI-parity mandate caught. `onTransactionSetChange` resets the cached loop catalog and re-fetches, and clears any prior loop mappings (they addressed the old tree's own loop ids).
- `controllers/edi_schema_controller.go`'s `GET /transaction-sets/:id/loops` needed zero changes — confirmed via direct curl against a real running container that `837P`/`837I`/`999` resolve correctly through `GetTransactionSet`'s existing dual-registration, including the full `loopRef`-resolved subtree serializing correctly over the wire.
- `ToolboxManager.js`: zero changes needed (`transactionSet` is a config field on an existing step, not a separate step type per transaction set) — confirmed via `toolbox-step-coverage.spec.js`.
- `V235__EDI_Inbound_Transaction_Types_Add_837_999.sql`: `edi_x12_inbound`'s own `transaction_types` UI enum (V232) only ever offered `["835"]` — widened to `["835","837P","837I","999"]`, additive-only (the connector itself never enforced this field, matching V232's own precedent).
- `V236__EDI_837_OOB_Pipeline_Template.sql`: ONE template covers both 837P and 837I (not two) — inbound ingestion is transaction-set-agnostic at the connector level, and `edi.parse` itself auto-detects the real variant per file from its own GS08, so the template shouldn't force a choice. Same 4-step shape as V230's own 835 template (`connector.inbound`→`edi.parse`→`edi.validate`→`connector.outbound`/`sink_outbound`).
- A stale existing test found and fixed along the way: `edi-pipeline-ui.spec.js`'s own `edi_x12_inbound` connector-config test hardcoded "exactly one checkbox (only 835 selectable today)" — updated to expect 4, with a genuine markup gotcha discovered fixing it: `ConnectorConfigBuilder.js`'s checklist checkboxes carry no `value` attribute at all (browser-defaults to `"on"`) — the real option text lives only on the associated `<label for="...">`.

### 999 auto-fire — `edi.generate_999` (`services/executors/transform/edi_generate_999_executor.go`)
Not a hidden, automatic side-effect of connector ingestion — matches this project's own explicit composable-pipeline-steps philosophy. Re-parses/re-validates the ORIGINAL raw content (same re-parse-from-raw discipline `edi.validate` already uses, for the same reason: needs `edi.ParseTransactionSet`'s typed `Fields`/`SegmentInstances`, which a prior step's JSON-shaped output doesn't carry). Maps `edi/validator.Result.Issues` onto `IK3`/`IK4` detail, grouped by erroring segment *occurrence* (one `IK3` per occurrence, multiple `IK4`s nested under it for multiple element errors in that same occurrence) — needed a new `SegmentInstance.Position`/`EDIField.SegmentPosition` (both `loop_engine.go`, a running 1-based counter over every segment matched from ST, stamped uniformly) since 999's own `IK3` needs to report a segment's real "position in transaction set," which nothing previously tracked. `AK9`/`IK5`'s accept/reject code is computed from whether any ERROR-severity issue exists — warnings still get reported as `IK3` entries but never flip the code, consistent with the two-severity model `edi/validator` already establishes.

Named, deliberate simplifications: no `CTX` generation (supplementary business-context detail this validator has no source data for); `IK403`'s error-code classification is a best-effort heuristic over `edi.Validate`'s own free-form Go error message text (no structured error-code return exists to classify on instead); a `SyntaxRule`-sourced warning's `Issue.Path` is a bare segment id with no occurrence disambiguation available, resolved to that segment id's first occurrence; GS01/GS08 envelope-identity issues are excluded from `IK3` entirely (a 999 reports on the ST...SE body, not the surrounding interchange — TA1's own domain, itself out of scope).

**Two real bugs found and fixed during testing** (both via direct rebuild/log-inspection debugging against a bare-ST 835 sample — see the disk-space-aware debugging discipline note below):
1. A message with no real GS segment (`envelopePresent: false`, a common, already-supported case) left `AK1`'s own echo fields as Go's literal `interface{}(nil)`, which rendered as the string `"<nil>"` on the wire. Fixed with a fallback chain onto the *resolved original transaction set's own* known `FunctionalIdentifierCode`/`VersionReleaseIndustryCode` (and a placeholder `"1"` for the group control number, since there's no real GS06 to echo).
2. IK4 detail was silently dropped from every build — the composite's own fields were built as a flat map instead of being wrapped under an `"IK4"` key (exactly like `IK3`'s own fields are wrapped under `"IK3"`) — `writeSegmentSequence` looks for `data["IK4"]` specifically, found nothing (IK4 is optional, so no error, just silently missing), and moved on. Root-caused by direct instrumentation of `edi/builder/segment_writer.go`'s own `writeLoopInstance` (temporarily, removed after) rather than continued static reading, after several rounds of code tracing failed to spot it.

### Full-stack verification — closing a real gap found by asking "is this actually tested end-to-end?"
Initial verification (Go unit/integration tests + a live container's HTTP schema API + Playwright driving the pipeline-builder's *step config UI*) was thorough but NOT full-stack — it never proved a real message flowing through a real, saved pipeline. Two gaps closed:
1. **V236's own "Use Template" flow**, driven through the real click path (same discipline as the 835-to-FHIR template's own existing test) — not just confirming the migration exists.
2. **`edi.generate_999` wired into an actual pipeline execution**, which surfaced a real, non-obvious requirement: `POST /api/pipelines/test` (`controllers/transformation_test_controller.go`'s `TestPipeline`) requires a genuine, persisted `interface_id`/`message_type` — an ad-hoc pipeline built via `addStep()` with no backing interface is rejected outright (`"Could not determine interface_id and message_type from request"`). Resolved by adding the step to a template-created (V236) interface's own real pipeline and running "Test Pipeline" against it — Test Pipeline explicitly supports testing unsaved in-progress changes once a real interface backs the pipeline, so the new step didn't need to be saved first. Against a real, unedited 835 sample, produced a fully correct, spec-valid 999 (`AK1*HP*1*005010X221A1` — the envelope-less fallback fix, live; `IK3*PER*9**8` — the sample's own real, known PER warning, correctly surfaced as non-blocking; `IK5*A`/`AK9*A*1*1*1` — Accepted), permanently captured in `edi-pipeline-ui.spec.js`.

**Disk-space-aware debugging discipline** (this machine repeatedly ran low during the session's many rebuild-and-test cycles): every throwaway `docker build --target gobuilder` image was `docker rmi`'d immediately after use, and `docker builder prune -f` was run when free space dropped meaningfully. One real, useful finding along the way: `docker builder prune` frees space *inside* Docker's own accounting but does NOT shrink the host-visible free-space number on Windows — Docker Desktop's WSL2 virtual disk grows on demand but doesn't auto-compact; genuinely reclaiming host disk space needs a `wsl --shutdown` + VHDX compact, deliberately not done automatically since it restarts the whole WSL environment.

### Key Files
- Architecture fixes: `edi/schema_types.go`, `edi/schema_loader.go`, `edi/loop_engine.go`, `edi/validator/validator.go`, `edi/builder/document_builder.go`, `edi/builder/segment_writer.go`
- Schema data: `edi/schemas/x12_005010/{manifest.json,envelope.json,segments/*.json,transactionSets/{999,837P,837I}.json,loops/{2300-claim,2300-claim-institutional}.json}`
- 999 auto-fire: `services/executors/transform/edi_generate_999_executor.go`, registered in `services/executor_registry.go`
- UI: `public/js/pipeline/components/EDIStepBuilder.js`
- Migrations: `database/migrations/V235__EDI_Inbound_Transaction_Types_Add_837_999.sql`, `V236__EDI_837_OOB_Pipeline_Template.sql`
- Tests: `edi/real_schema_integration_test.go` (837P/837I/999 round-trip tests), `services/executors/transform/edi_generate_999_executor_test.go`, `tests/playwright/edi-pipeline-ui.spec.js`

### Named future phases (STATUS UPDATE — both since closed, kept for history)
270/271 (eligibility) and AS2 transport were still out of scope as of this round, but both have SINCE shipped (270/271: transform-only, no new connector needed; AS2: a genuinely new `as2_inbound`/`as2_outbound` connector pair) — see this file's own "Named future phases" note under "EDI X12 Support — Phase 1" above for the current status of all 5 originally-named phases. 837/999 → FHIR mapping was the named gap at the end of this round — closed for 837P/837I in the section below; 999 → FHIR remains explicitly out of scope (see that section).

## EDI X12 837P/837I → FHIR Mapping (Claim + Patient + Coverage + Organization, September 2026)

### Scope
User-confirmed up front: full multi-claim, multi-service-line mapping (837 files realistically carry many claims per file, unlike 835's own single-resource first pass) — not a single-claim MVP. Two **separate** OOB templates (`edi-837p-to-fhir-sftp`, `edi-837i-to-fhir-sftp`), not one with branching — professional and institutional need genuinely different `item[]`/`supportingInfo[]` shapes. **999 → FHIR is explicitly deferred** — a transport-layer acknowledgment has no natural FHIR analogue, named as a non-goal, not an oversight.

### A real architecture constraint found and fixed generically, not worked around
The original plan called for a `control.loop` step (foreach over the file's claims, with `fhir.build` child steps for Patient/Coverage/Claim) — the same shape V231's own 835-to-FHIR template already documented as impossible: `control.loop`'s `childStepIds` config can only hold real, DB-assigned step UUIDs, which don't exist yet at OOB-template-authoring time (confirmed directly — `useTemplate`'s own frontend flow, `public/js/dashboard.js`, explicitly **strips every step's `id`** before saving so the backend generates fresh UUIDs, with no remapping of any template-local loop-child reference). A per-claim control.loop is therefore fundamentally unusable inside a template migration, not just inconvenient.

Fixed generically in `services/executors/transform/fhir_build_executor.go`: `fhir.build` gained an optional `rowsPath` config key — when set, it builds **one resource per row** found at that path (relative to `inputData`) instead of one resource from `inputData` directly, writing an **array** to `outputField`. Field/repeatingGroup resolution is row-scoped (same convention `repeatingGroups.Fields` already use — no automatic top-level fallback for plain `sourcePath`/`fallbackPaths`, only `condition`/`groupCondition` merge in `topLevel` via the existing `mergeWithFallback`). This eliminates the need for `control.loop` entirely for "N independent resources of one type from one array" — Patient/Coverage/Claim each use `rowsPath: "_claim_contexts"` and need no step-ID wiring at all; `payload.builder`'s `fhir_bundle` mode already accepted array-valued `resourcePaths` entries (proven by V231's own precedent), so the 3 arrays plug in directly alongside the once-built Organization resource. Fully regression-tested: 4 new tests in `fhir_build_executor_test.go` (one-resource-per-row, row-scoped-not-topLevel-scoped field resolution, condition-still-merges-topLevel, and a guard proving omitted `rowsPath` reproduces today's exact single-resource behavior) plus the full pre-existing 32-test `TestFHIRBuild_*` suite, all green.

### Architecture — one `enrichment.script` step, then declarative `fhir.build`/`payload.builder`/`fhir_validation`
No `control.loop`, no Practitioner resource (see below) — just:
1. `connector.inbound` (`edi_x12_inbound`, SFTP, `transaction_types` scoped to the one variant) → `edi.parse` → `edi.validate`
2. **`enrichment.script`** ("Derive 837P/837I Claim Contexts") — the only new procedural code, scoped narrowly to pure data reshaping (no FHIR knowledge), matching PAS's own "Derive PAS Computed Fields" scope-boundary precedent. Assumes **one billing provider (2000A) per file** (named, deferred edge case). Walks each `2000B` subscriber: if it has `2000C` dependents, iterates each dependent's own claims (patient = dependent); otherwise the subscriber's own claims (patient = subscriber). Produces one flat `_claim_contexts[]` array, each entry `{patientInfo, subscriberInfo, payerInfo, claim: {...}}`, with diagnosis/procedure/service-line/care-team lists already flattened so the `fhir.build` configs downstream stay purely declarative.
3. `fhir.build(Organization)` — billing provider, built once, single-resource mode, stable literal id `organization-billing`
4. `fhir.build(Patient)` / `fhir.build(Coverage)` / `fhir.build(Claim)` — all `rowsPath: "_claim_contexts"`, one resource per claim
5. `payload.builder` (`fhir_bundle` mode, `resourcePaths` mixing the single Organization path with the 3 arrays) → `fhir_validation` (strict) → `connector.outbound` (`sink_outbound`)

**A real, dangling-reference bug caught and fixed via `fhir.build`'s own `condition` mechanism**: `Coverage.subscriber.reference` would point at a Patient resource that was never built (the actual subscriber, in the dependent-is-patient case, since only the *patient's* own Patient resource is built per claim context). Fixed by gating that one field with `condition: {field: "patientInfo.relationshipCode", operator: "equals", value: "18"}` — only written when the patient IS the subscriber; `Coverage.subscriberId` (a plain string, never a reference) is always populated regardless, so the subscriber's identity is never lost, just never expressed as a dangling `Reference`.

**No separate Practitioner resource** — `Claim.careTeam[].provider` uses a **logical (identifier-only) Reference** (`provider.identifier.system`/`.value`, no `.reference`) instead. A real design correction made mid-implementation: building a Practitioner resource per claim (matching PAS's own "Practitioner × N" language in the original plan) would either need cross-file provider deduplication (real complexity for a single-phase mapping) or produce near-empty `{"resourceType":"Practitioner"}` junk entries for claims with no rendering/attending provider — a FHIR `Reference.identifier` logical reference avoids both problems entirely and is fully spec-conformant.

### Two real required-field gaps found only by running strict `fhir_validation` (not by reading the spec table)
The original cardinality check (this project's own decompressed `Claim.gz`) had correctly flagged `type`/`use`/`patient`/`provider`/`insurance` as required but missed two more real ones surfaced only when the Go-level test's `fhir_validation` step actually ran: `Claim.created` (no X12 837 equivalent — a claim submission carries no "record created" timestamp; defaulted to the pipeline's own processing time, computed once per file in the derive script, same "record processing time, not sourced from the message" convention BHT03/BHT04 already establish for the interchange) and `Claim.priority` (also no X12 837 equivalent — a PAS/preauthorization concept, not a claim-submission one — fixed to a literal `"normal"`).

### 837I-specific mapping (institutional is a genuinely different shape, not a relabeling)
Confirmed directly against `edi/schemas/x12_005010`'s own `loops/2300-claim-institutional.json` and `segments/{CL1,SV2,HI}.json`:
- **SV2** (not SV1) → `Claim.item[]`: adds `item.revenue` from SV2's own revenue code; no item-level `diagnosisSequence` pointers (institutional service lines don't carry them). `productOrService` is **always** anchored on the revenue code (`coding[0]` — the one identifier every real institutional line carries), with the HCPCS/CPT code, when SV202 is actually present, added as an **additional** `coding[1]` — never a conditional either/or (see bug #2 below for the correction history).
- **HI is SHARED** for diagnosis AND procedure codes on 837I (837P's own HI only ever carries diagnoses) — split purely by each repetition's own qualifier: `BK`/`ABK`/`BF`/`ABF`/`BJ`/`ABJ`/`BN`/`ABN`/`PR`/`APR` (ICD-9/ICD-10 qualifier pairs) → `Claim.diagnosis[]`, `BR`/`BBR`/`BQ`/`BBQ` → `Claim.procedure[]`. HI's own Present-on-Admission indicator (C022 sub09) rides on diagnosis repetitions → `Claim.diagnosis[].onAdmission`, populated **only when actually present** on that repetition (a real bug caught by the test: the derive script must omit the `onAdmission` key entirely, not set it to `""`, for the sibling `onAdmission.coding[0].system` literal-value field's own `condition: {field: "onAdmission", operator: "exists"}` guard to correctly suppress it — `exists` checks `!= nil`, which a present-but-empty string would still satisfy).
- **CL1** (Institutional Claim Code — admission type/source, patient status) has no professional equivalent → `Claim.supportingInfo[]`.
- **Care team roles are genuinely different role assignments**, not a relabeling of 837P's own: `2310A`=Attending, `2310B`=Operating Physician, `2310D`=Rendering, `2310F`=Referring (837P's own `2310A`/`2310B` are Referring/Rendering). `Claim.careTeam[]` entries require the role's own NM1 to carry an `identificationCode` (NPI) — a role present with no NPI (e.g. a pre-NPI-era claim identifying its physician by UPIN via a `REF` segment instead) is silently skipped rather than correlated by name; a named, out-of-scope limitation found via X12.org's own "Jones Hospital" example, not a crash risk. `2310E` (Service Facility Location) is deliberately EXCLUDED from `careTeam` — it's a place, not a provider — and instead maps to `Claim.facility.identifier` (see bug #7 below).
- Value/occurrence/condition codes (HI qualifiers `BE`/`BH`/`BG`) and DRG (`DR`) are not modeled in this pass — named simplification, matching 835's own "core fields, not exhaustive" precedent.

### 7 real bugs found only by testing against genuine real-world sample data — two of them a core parsing-engine bug and a schema-modeling bug, not just mapping bugs
Sourcing real 837 samples for round-trip verification (mirroring 835's own precedent of testing against real, unedited files) surfaced a chain of real gaps neither the synthetic Go-test fixtures nor the BuildDocument-round-trip tests could ever have caught — because a canonical-JSON round trip (build synthetic data → write raw text → re-parse) is, by construction, always schema-order-consistent and always D8-dated; only genuinely independent real data exercises the shapes and orderings a real trading partner's file actually uses.

1. **`DTP*472` RD8 date ranges** (`Claim.item.servicedDate` bug). X12's `DTP02` can be `D8` (single `CCYYMMDD` date) or `RD8` (a `CCYYMMDD-CCYYMMDD` **range**) — real claims commonly use RD8 for multi-day services. The original derive scripts wrote the raw DTP value straight into `Claim.item.servicedDate` untransformed; an RD8 value there is neither a valid FHIR `date` (wrong format) nor did the field even have `x12_date_to_fhir_date` applied (a D8 value would have failed too). Fixed (`resolveServicedDate`): D8 → `servicedDate`; RD8 → split on `-` into `Claim.item.servicedPeriod.start`/`.end` — a FHIR choice type, so only one shape is ever populated per line.
2. **Revenue-only institutional service lines** (`Claim.item.productOrService` bug, later corrected a second time — see bug #5's own note). SV202 (procedure code) is genuinely OPTIONAL on real institutional claims — many revenue lines (room & board, etc.) carry only a revenue code, no HCPCS/CPT. Base FHIR R4's `Claim.item.productOrService` is unconditionally 1..1 required, so leaving it unpopulated fails strict validation. First fix: 837I's `extractServiceLines` fell back to the revenue code itself (system `https://codesystem.x12.org/005010/234`, the same system `Claim.item.revenue` already uses) only when no procedure code existed — i.e. CPT-if-present-else-revenue, treating CPT as primary. **User-corrected (2026-09-11)**: that conditional either/or was wrong. Institutional `productOrService` is **always** anchored on the revenue code as `coding[0]` (the one identifier every real institutional line carries), with the CPT code, when present, added as an **additional** `coding[1]` — never a replacement. Confirmed directly against X12.org's own official "Jones Hospital" 837I example, whose real service lines (`SV2*0305*HC:85025*...`, `SV2*0730*HC:93005*...`) carry both codes together on every line.
3. **`HI` segment maxUse — a schema bug, not a mapping bug.** Real institutional claims carry MULTIPLE separate `HI` segment occurrences per claim (one per qualifier-group — diagnosis codes in one, DRG in another, more diagnoses in another, value codes in another), confirmed independently in two real samples. `edi/schemas/x12_005010/segments/HI.json`'s `maxUse` was wrongly `"1"`; corrected to `">1"` (see that file's own maxUse-correction note) — this is a SHARED segment file, so the fix applies to both 837P and 837I. Both derive scripts now flatten `codes[]` across every `HI` occurrence (`allHICodes`) before filtering by qualifier.
4. **Sibling loops sharing one trigger segment — a core `matchLoops` engine bug, not a schema or mapping bug, found in two escalating stages:**
   - **Stage A (silent data loss → hard failure):** `NM1` is globally `maxUse=">1"` (legitimately repeats in some contexts) but is ALSO the trigger for many ADJACENT loops (1000A Submitter immediately followed by 1000B Receiver; 2010BA Subscriber immediately followed by 2010BB Payer — present in literally every real 837 file). `matchSegmentSequence` had no way to know a repeated trigger sighting meant a NEW loop instance had begun rather than a genuine repeat within the current one, so it silently folded the sibling loop's own `NM1` (and, when the current loop's own segment list happened to also declare `N3`/`N4`, the sibling's own address segments too — a hard "required segment SE not found" failure) into the wrong loop's own data. Fixed generically: `matchSegmentSequence` gained a `triggerID` parameter — a loop's own trigger segment is now NEVER treated as repeating within that same loop instance, regardless of the segment's own global `maxUse` (a second sighting always means loop-matching must resume, never "add to this instance").
   - **Stage B (wrong-role assignment, surfaced once Stage A was fixed):** even with Stage A fixed, SEVERAL DISTINCT sibling loops sharing one trigger (837's own provider-role loops, 2310A-F — Referring/Rendering/Service Facility/Supervising/Ambulance for professional, Attending/Operating/Other Operating/Rendering/Service Facility/Referring for institutional) still had no way to be told apart when a real claim's populated roles didn't appear in schema-declaration order (routine — most real claims populate only a subset). This exact gap was already anticipated and deliberately deferred in `matchLoops`' own original doc comment: *"If a future transaction set ever needs [a discriminator], add an explicit element-value discriminator then, not now."* That transaction set is 837. Added `X12LoopDef.TriggerDiscriminator` (`schema_types.go`) — an element key + expected value (e.g. `entityIdentifierCode` = `"71"` for Attending) — and a two-pass `matchLoops` algorithm: discriminated candidates get first refusal on their own known value (schema order among themselves), then undiscriminated candidates act as a pure catch-all for whatever's left (unchanged, fully backward-compatible behavior for every loop that doesn't need one). Applied to 5 of 6 roles in both `2300-claim.json`/`2300-claim-institutional.json` with HIGH-CONFIDENCE, well-established X12 codes (`DN`/`82`/`77`/`DQ`/`PW`/`45` professional; `71`/`72`/`82`/`77`/`DN` institutional — `77`=Service Facility Location confirmed directly against real sample data) — the one code this session couldn't verify with the same confidence (837I's own "Other Operating Physician," 2310C) was deliberately left undiscriminated as the group's own catch-all rather than guess, a named trade-off, not an oversight. 835 is unaffected by any of this (its own envelope loops' trigger, `N1`, is `maxUse="1"`).
5. **HI qualifier filter only recognized ICD-10 forms — a mapping bug found against X12.org's own official "Jones Hospital" 837I example**, which predates the ICD-10 transition and uses the ICD-9 diagnosis qualifiers (`BK`/`BF`) instead of the "AB"-prefixed ICD-10 forms (`ABK`/`ABF`) the original `extractDiagnoses`/`extractProcedures` filter matched exclusively. Both qualifier families are equally valid under 5010 (the qualifier itself indicates which ICD version the code is drawn from) — the original prefix-only check silently dropped every ICD-9-coded diagnosis. Fixed with an explicit whitelist covering both eras (`DIAGNOSIS_HI_QUALIFIERS`/`PROCEDURE_HI_QUALIFIERS` in `edi_837i_fhir_builder_test.go`), rather than a broader/riskier prefix heuristic (`BE`/`BG`/`BH` also start with `B` but are non-diagnostic value/occurrence codes that must stay excluded).
6. **Dependent relationship code read from the wrong segment — affects both variants, a schema-modeling bug found against X12.org's own official "Ben Kildare Service" 837P example.** `Coverage.relationship` (e.g. "child") was read from the subscriber's own `SBR02` (2000B) in the dependent-is-patient case. Real, spec-compliant 837 data leaves `SBR02` **blank** whenever a 2000C dependent loop exists, carrying the real relationship code on the dependent's own `PAT01` (2000C) instead — confirmed directly in the Ben Kildare sample (`SBR*P**2222-SJ*******CI` [SBR02 blank] → `HL*3*2*23*0~PAT*19` [child]). The root cause was one level deeper than the derive script: `edi/schemas/x12_005010/segments/PAT.json` had `PAT01` dropped entirely (marked `usage=N`, based on a misreading of the PyX12 IG map claiming the relationship code lives on SBR02 instead) — so the real parser never even captured the value anywhere. Fixed by re-adding `PAT01` to the schema (a pure config edit, no Go change) and updating both derive scripts to prefer `PAT01`, falling back to `SBR02` for any trading partner that populates it there instead (flexible, not rigid — matching this project's own stated EDI design philosophy).
7. **`Claim.careTeam[]` silently dropped 4 of 6 discriminated provider roles on 837P, and neither variant mapped `Claim.facility` at all — found against X12.org's own COB (Example 3a), Ambulance (Example 5), and Anesthesia (Example 9) samples.** The engine's own `matchLoops` discriminator (bug #4 above) correctly identifies all 6 837P provider-role loops (2310A-F) and all 6 837I ones, but 837P's `extractCareTeam` only ever read 2310A (Referring)/2310B (Rendering) — Example 3a's own 2310C (`NM1*77*2*KILDARE ASSOCIATES*****XX*1581234567`) carried a real NPI that was silently discarded. Root cause was a genuine category error: 2310C/837I's 2310E (**Service Facility Location**) is not a care-team member at all — it's a *place* — confirmed against this project's own decompressed `Claim.gz` schema, which has a dedicated `Claim.facility` (0..1 `Reference`) field for exactly this concept, never wired up. Fixed in two parts: `extractCareTeam` gained 2310D (Supervising, a genuine care-team role, found unmapped by the same audit) for 837P; a new `extractFacility` function (mirrored in both variants) maps 2310C/2310E to `Claim.facility.identifier` via the same logical (identifier-only) reference pattern `careTeam[].provider` already uses. 2310E/F (Ambulance Pick-up/Drop-off Location) remain a deliberate, named gap — the Ambulance sample proved these carry no name or identifier at all (a bare address), so there's nothing to build even a logical reference from, and base FHIR `Claim` has no second location-type field beyond `facility`.

**On the samples themselves**: the first several public samples checked were unusable as *committed* test fixtures — either restrictively licensed (a Databricks repo scoped to Databricks Services use only), unlicensed (`NOASSERTION`, no redistribution right), or mislabeled (institutional GS08 with actual SV1/professional content). The Databricks samples specifically are used here for **test-only verification, not distributed with the application** — an explicit, user-confirmed scope decision (2026-09-11) distinct from redistributing them. `edi/testdata/real_samples/databricks_837{p,i}_sample.txt` hold those two genuine, unedited files; `services/edi_837_real_parser_roundtrip_test.go` also keeps 2 small, entirely self-authored raw X12 samples (deliberately including an RD8 range) as a second, licensing-risk-free layer of the same proof. A fourth layer, added per the user's own explicit "run it against examples from here too" instruction (2026-09-11): X12.org's own official worked examples — the standards body's own published educational material, lower licensing risk than any third-party repo, envelope segments synthesized (the pages show only ST...SE) but every body segment extracted from the page's own raw HTML (not an AI-summarized re-transcription — one such summarization pass silently introduced a spurious extra element into an `HL` segment, caught only by cross-checking against the raw HTML directly; **a documented, real transcription-fidelity risk of `WebFetch`-style tooling for exact-byte-value data, not this codebase's own bug**). Six examples total, chosen for scenario diversity from x12.org's own catalog of 12 (837P) + 2 (837I) rather than exhaustively run: "Ben Kildare Service" (837P Example 1, commercial insurance — bugs #2/#6), "Jones Hospital" (837I Example 1a, institutional — bugs #2/#5), Example 3a (837P, Coordination of Benefits — bug #7's `facility` gap, plus a genuinely untested 2320/2330A/B secondary-payer loop shape that parses cleanly but stays unmapped, named simplification unchanged), Example 5 (837P, Ambulance — proves `CR1`/CR2 Ambulance Certification and address-only 2310E/F don't derail parsing), Example 9 (837P, Anesthesia — a clean confirmation of the new `facility` fix plus multi-modifier procedure-code composites), Example 2a (837I, Automobile Accident/Property & Casualty — a third independent confirmation of the revenue+CPT dual-coding design, plus real-data proof of the `PR`/`BN` HI qualifiers added speculatively for bug #5). `edi/testdata/real_samples/x12org_837{p,i}_*_sample.txt` hold all six.

**A real operational lesson from fixing bugs 1-2 after already pushing V237/V238 once**: editing an already-applied Flyway migration's SQL changes its checksum, which `flyway repair` (already wired into this project's own `command: ["repair", "migrate"]`) silently reconciles — but repair only fixes the bookkeeping; it does **not** re-run the migration's own `INSERT ... ON CONFLICT DO UPDATE`, so the live `interface_templates` rows stayed stale until the corrected `.sql` files were re-applied directly via `psql`. Safe here only because V237/V238 were minutes-old, same-session, unreleased-beyond-this-repo migrations — the established "never edit a shipped migration, add a new one instead" rule (see V231/V236's own comments) still governs anything that's actually reached a real deployment.

### Verification
Go-level: `services/edi_837p_fhir_builder_test.go` and `services/edi_837i_fhir_builder_test.go` (mirroring `pas_fhir_builder_test.go`'s own "config here is the source of truth, transcribed verbatim into the migration" discipline) each chain the real executors — derive script → 4× `fhir.build` → `payload.builder` → `fhir_validation` strict — against a realistic multi-claim, multi-line-item fixture, asserting exact resource/entry counts and zero unexpected validation errors (the one known, pre-existing, out-of-scope `ClaimTypes.gz` ValueSet gap PAS's own test already documents is explicitly excluded, not silently tolerated for anything else). `services/edi_837_real_parser_roundtrip_test.go` proves the same chain against the REAL `edi.ParseTransactionSet` parser's own output across NINE sample sources (2 self-authored, 1 Databricks 837P + 1 Databricks 837I, and 5 X12.org official examples) — 12 tests total, covering every one of the 7 bugs above with a real assertion (RD8→servicedPeriod, revenue-primary/CPT-secondary dual coding across three independent institutional samples, cross-HI-occurrence flattening, the discriminator correctly routing every provider role including Attending vs. Service Facility, ICD-9 AND newly-added `PR`/`BN` qualifiers correctly recognized, `Coverage.relationship`/`patientInfo.relationshipFHIR` correctly reading "child" from PAT01 rather than defaulting to "other" from a blank SBR02 — and, on a genuinely unmapped relationship code (`PAT*21`), correctly falling through to "other" rather than crashing or guessing, `Claim.facility.identifier` correctly populated from 2310C/2310E and correctly ABSENT when no such loop exists, and `CR1`/multi-modifier procedure-code composites proven not to derail subsequent segment parsing). A `numEquals` test helper tolerates the int64-vs-float64 type variance goja produces for whole-number JS values (a test-assertion fragility, not a product bug — `net.value = 40` failed a naive `!=` comparison against a Go `float64(40)` literal purely due to differing underlying types, both printing identically). The full `edi`/`edi/builder`/`edi/validator` package suite (835/999/837P/837I, real 835 samples, synthetic unit tests) and every EDI-related executor test were re-run after each engine/schema change with zero regressions. Every migration's embedded JSON (including the full JS derive script, spliced in programmatically rather than hand-retyped, after hand-retyping an early version silently dropped a comment block) was diff-verified byte-for-byte against its Go test source before being considered done — done four times across this round as fixes accumulated (RD8/created/priority; HI-flatten/engine fixes; the productOrService redesign + qualifier whitelist + PAT01 fixes; the careTeam/facility fixes).

Full-stack: real `docker-compose build app` + `docker-compose up -d app` (clean startup, zero errors in logs), Flyway applied both migrations cleanly against a live Postgres, both templates confirmed present via direct `psql` query (11 execution groups each). **The full browser click-path gap named in an earlier draft of this section is now CLOSED**: `tests/playwright/edi-837-to-fhir-e2e.spec.js` drives the real "Use Template" click path for both templates (Templates gallery → configure modal → Create Interface → redirect to pipeline-builder.html, confirming 11 canvas nodes with correct per-step config), then a real "Test Pipeline" run against X12.org's own Ben Kildare (837P) and Jones Hospital (837I) samples through the complete HTTP/DAG-execution stack, parsing the assembled Bundle out of `testOutput.steps.<alias>.step_output.payload` and asserting real resource counts and field values (4 service lines with CPT-direct coding for Ben Kildare; 2 service lines with revenue+CPT dual coding for Jones Hospital) — not just "the run succeeded." This closing pass is what surfaced the 2 generic pipeline-engine gotchas documented in their own section below (the "message." nesting requirement and the `steps.<alias>.step_output` snake-casing requirement) — both real, both invisible to the Go-level tests above, both now fixed and re-verified through this same real browser path.

### Named simplifications (stated up front, not silently left out)
- Assumes **one billing provider (2000A) per file** — a file with multiple 2000A occurrences only maps the first one to the Organization resource.
- Patient/Coverage are built **per claim context, not deduplicated** across multiple claims for the same real-world patient — a patient with 2 claims gets 2 Bundle entries sharing the same stable id (harmless per FHIR Bundle semantics).
- `NTE`/`K3`/`CRC`/`PWK`/most of `REF`, and item-level `diagnosisSequence`/`careTeamSequence` pointer arrays (`fhir.build`'s `repeatingGroups` builds object rows, not primitive-array rows) are not mapped — `Claim`'s core identifying/clinical/financial shape is the target, matching 835's own "core fields, not exhaustive" precedent.

### Key Files
- FHIR-build engine enhancement: `services/executors/transform/fhir_build_executor.go` (`rowsPath`), tests in `fhir_build_executor_test.go`
- **Core parsing-engine fixes**: `edi/loop_engine.go` (`matchSegmentSequence`'s `triggerID` param, `matchLoops`' two-pass discriminator algorithm, `selectLoopCandidate`, `discriminatorMatches`), `edi/schema_types.go` (`X12LoopDef.TriggerDiscriminator`/`X12LoopTriggerDiscriminator`)
- Schema data fixes: `edi/schemas/x12_005010/segments/HI.json` (`maxUse` correction), `edi/schemas/x12_005010/loops/2300-claim.json` + `2300-claim-institutional.json` (`triggerDiscriminator` entries)
- Go verification tests (source of truth for both migrations): `services/edi_837p_fhir_builder_test.go`, `services/edi_837i_fhir_builder_test.go`
- Real-parser round trip (self-authored + real Databricks samples + 5 X12.org official examples): `services/edi_837_real_parser_roundtrip_test.go`, `edi/testdata/real_samples/databricks_837{p,i}_sample.txt`, `edi/testdata/real_samples/x12org_837p_ben_kildare_sample.txt`, `edi/testdata/real_samples/x12org_837i_jones_hospital_sample.txt`, `edi/testdata/real_samples/x12org_837p_example{3a_cob,5_ambulance,9_anesthesia}_sample.txt`, `edi/testdata/real_samples/x12org_837i_example2a_auto_accident_sample.txt`
- Schema fix from the X12.org samples: `edi/schemas/x12_005010/segments/PAT.json` (`PAT01`/`individualRelationshipCode` re-added)
- Migrations: `database/migrations/V237__EDI_837P_To_FHIR_OOB_Pipeline_Template.sql`, `V238__EDI_837I_To_FHIR_OOB_Pipeline_Template.sql`
- Full browser click-path E2E (Use Template + real Test Pipeline against X12.org samples, asserting on the assembled Bundle's real content): `tests/playwright/edi-837-to-fhir-e2e.spec.js` — also the template for closing the equivalent gap on any other OOB template whose own Playwright coverage never drives a real Test Pipeline run

### Named future phases
999 → FHIR (`OperationOutcome`) remains out of scope. Multiple-billing-provider-per-file (`2000A` repeats) is a named, deferred edge case. 837I's own 2310C (Other Operating Physician) has no confirmed trigger discriminator and acts as a catch-all — a real, if narrow, residual gap if a claim carries BOTH an unrecognized-role NM1 AND a genuine Other Operating Physician at the same position. `Claim.careTeam[]` only correlates providers with a real NPI on their own NM1 — a pre-NPI-era claim identifying a provider by UPIN/REF instead (as X12.org's own Jones Hospital example does for its attending physician) silently produces an empty care team rather than a name-only entry; a named, out-of-scope limitation, not attempted this round. 2310E/F (Ambulance Pick-up/Drop-off Location) remain unmapped — real X12.org sample data confirms these carry no name/ID at all, and base FHIR `Claim` has no second location field beyond the now-mapped `facility`. The 2320/2330A/2330B secondary-payer (COB) loop parses cleanly (confirmed against a real COB sample) but is not mapped to `Claim.insurance[1+]` — `insurance[0]` (the primary payer) remains the only entry built, a named simplification unchanged from the original design.

### The browser click-path test itself found 2 more real bugs — both generic pipeline-engine behavior, not EDI-specific, and both invisible to every Go-level test in this round
Closing the "does this actually work through the real HTTP/DAG pipeline stack" gap named above (not just the Go-level executor chain) surfaced two real, previously-undiscovered gotchas about how `enrichment.script` output reaches a LATER step in a REAL pipeline run — neither is EDI-specific; both apply to any future feature chaining `enrichment.script` → `fhir.build`/`hl7.build`/any other sourcePath-driven step. Both were invisible to every existing Go-level test (this round's own 12 EDI 837 tests included) because the Go-level test helper (`svcInjectStepOutput`) happens to reproduce the SAME two mechanisms correctly — proving the *config* was consistent with itself, but never proving the config was consistent with the *real engine*.

1. **A prior step's plain top-level output field is nested under `input.message.<field>` for a later step, never at `input.<field>` directly.** `edi.parse`'s own `outputField` (`"parsedEDI"`, a plain top-level `outputData["parsedEDI"] = ...` write) survives into later steps' `inputData`, but only reachable via `input.message.parsedEDI` — the orchestrator (`executeStepWithContext`) wraps the whole running `outputData` under a `"message"` key for the next step's `inputData`. Fixed in both derive scripts' own `var parsed = ...` line with a fallback chain: prefer `input.message.parsedEDI`, fall back to bare `input.parsedEDI` for the Go-level test harness (which constructs `data` flat, without a `"message"` wrapper).
2. **An `enrichment.script` step's own RETURNED variables never reach a later step's plain `inputData` at all — only via `input.steps.<step_alias>.step_output.<key>`, and that view is always snake_cased.** `BaseExecutor.SetStepOutputWithDetails` writes a script's own return value ONLY into `inputData["_stepOutput"]`; `executeStepWithContext` extracts that into a per-step `steps.<alias>.step_output` snapshot (for display and cross-step referencing) and then **deletes** `"_stepOutput"` from what actually flows forward — so a script's own `_claim_contexts`/`_billing_provider` never reach a later `fhir.build` step at the top level OR under `"message"`, only via the `steps.<alias>.step_output` reference path already used elsewhere in this codebase (e.g. `fhir_validation`'s own `"source_field": "steps.<alias>.step_output.payload"` pattern). That snapshot is always passed through `models.OutputNormalizer.NormalizeStepOutput` first, which snake_cases every key it doesn't already recognize as snake_case — so `patientInfo`/`totalChargeAmount`/`onAdmission`/etc become `patient_info`/`total_charge_amount`/`on_admission` by the time a later step's `sourcePath` can see them. Fixed two ways together: (a) both derive scripts' `rowsPath`/`sourcePath`-consuming `fhir.build` configs now reference `"steps.derive_837{p,i}_claim_contexts.step_output._claim_contexts"` instead of a bare `"_claim_contexts"`; (b) **every property name in both derive scripts' own RETURNED object is now snake_case** (`patient_info`, `total_charge_amount`, `on_admission`, `facility_npi`, etc.) rather than the camelCase a JS reader might expect — matching what the config will see post-normalization regardless, so the script and the config that consumes it stay honest about the actual data shape instead of silently relying on a normalization step that renames things invisibly.
3. **The `steps.<key>` addressing key is `models.OutputNormalizer.NormalizeKey(step.StepName)` — the configured `step_alias` FIELD ITSELF is never consulted for this** (`transformation_pipeline_helpers.go`'s `executeStepWithContext`, the `displayKey := displayNorm.NormalizeKey(e.so.StepName)` line). Found while building the 835→FHIR per-claim mapping below: a derive step named "Derive 835 Claim Header Context" with `step_alias: "derive_835_claim_context"` (missing the word "Header") silently produced ZERO built resources — every downstream `rowsPath`/`sourcePath` referencing `steps.derive_835_claim_context.step_output...` resolved to nothing, because the REAL runtime key was `derive_835_claim_header_context` (`NormalizeKey` of the full step name, which also splits camelCase word boundaries — e.g. `"Build ExplanationOfBenefit"` → `build_explanation_of_benefit`, not `build_explanationofbenefit`). The 837P/837I derive steps above never hit this because their own step names ("Derive 837P Claim Contexts") happen to already equal their chosen aliases once lowercased — coincidence, not a documented rule, until now. **The actual, precise rule**: whatever step NAME you give an `enrichment.script` step, its `NormalizeKey`-normalized form (lowercase, spaces→underscores, camelCase→underscored) is the ONLY valid `steps.<key>` address for it — pick a step_name whose normalized form is the address you want, don't rely on `step_alias` matching it by assumption.

**Scope check, confirmed not assumed**: V231's own 835-to-FHIR template was unaffected by gotchas #1/#2 above (no `enrichment.script` step in its chain) — but it has SINCE been superseded by V247 (see "EDI X12 835 → FHIR Mapping" below), which DOES add one and is subject to all 3 gotchas, gotcha #3 included (the one that actually bit it during that round's own full-stack verification).

A real, reusable Playwright pattern came out of proving this: `tests/playwright/edi-837-to-fhir-e2e.spec.js` drives Use Template → Test Pipeline against a genuine X12.org sample → parses the assembled Bundle's own JSON out of `testOutput.steps.<payload.builder alias>.step_output.payload` and asserts real resource counts/field values — not just "the run succeeded." This is the concrete template for closing the equivalent gap on any other OOB template whose own Playwright coverage (like V231's) only ever drove the step-config UI, never a real Test Pipeline execution.

## EDI X12 835 → FHIR Mapping — Per-Claim ExplanationOfBenefit (September 2026)

### Scope and why it's not greenfield
V231/V234 (this file's own "EDI X12 Support — Phase 1" section) already shipped a PARTIAL 835→FHIR mapping — one `PaymentReconciliation` resource per interchange, explicitly declining to build a per-claim `ExplanationOfBenefit` because a `control.loop` step's `childStepIds` can only hold real, DB-assigned step UUIDs that don't exist at template-authoring time (V231's own comment named this exactly). That blocker was later solved generically during the 837P/837I work (`fhir.build`'s own `rowsPath` mode — see that section above) — this round closes the 835 gap the same way, plus a small, genuinely reusable primitive (`_rowIndex`) that V231's-era code had no equivalent for.

A fuller mapping already existed, Go-test-only, in `services/executors/transform/edi_835_to_fhir_test.go` since before `rowsPath` existed — it hand-looped `fhir.build` once per claim in Go test code (exactly what `control.loop` would do at runtime, per that file's own header comment), which a real pipeline template can never do. This round rewrote that test to use the real `rowsPath` mechanism (one `fhir.build` call, not a loop) and shipped the now-provably-correct config as a new migration.

### The `_rowIndex` primitive (`fhir_build_executor.go`, generic — not 835-specific)
`EOB.item[].sequence` (required by base FHIR) had no mapping in the pre-existing test — `fhir.build` had no per-row auto-incrementing index a field mapping could reference. Fixed by having `withRowIndex` stamp a synthetic `"_rowIndex"` (1-based position, as a string, matching every other resolved field's un-transformed shape) onto every row BEFORE field/repeatingGroup resolution runs — both at the top-level `rowsPath` branch and inside `applyRepeatingGroup`'s own row loop. A field mapping references it like any other row field: `{"targetPath": "item.sequence", "sourcePath": "_rowIndex", "transform": "cda_decimal_string_to_number"}`. Generic, reusable by any future `rowsPath`/`RepeatingGroup` config that needs a stable per-row ordinal — not invented for 835, just first needed here.

### A real mechanism gap found by reading the code, not assumed: `rowsPath` field resolution is ROW-ONLY
`applyFieldRow`'s call to `resolveRawValue(source, f)` is handed only the current row — `topLevel`/`inputData` is passed into `applyFieldRow` solely for `conditionMet`, never for `sourcePath`/`fallbackPaths` resolution (confirmed directly in `fhir_build_executor.go`, not assumed from the 837P/837I precedent). A claim row (a 2100 loop instance) therefore has NO reachable path back up to the interchange header — the 2 header-level values every claim needs (`BPR16` payment date, `1000A` payer name) must be copied onto EACH row before `fhir.build` ever sees them. This is why a small `enrichment.script` step ("Derive 835 Claim Context") is needed at all, despite 835's much shallower loop nesting than 837 (no subscriber/dependent hierarchy, no diagnosis/procedure/care-team lists) — a genuinely smaller script than 837P/837I's own, but not avoidable.

### Architecture
`connector.inbound`(`edi_x12_inbound`) → `edi.parse` → `edi.validate` → **`enrichment.script`** (derive, pure data reshaping — flattens CLP/NM1/SVC/CAS into one `claim_rows[]` array, each row carrying the 2 copied-down header values, fully-flattened `service_lines[]`, and each service line's own fully-flattened `adjustments[]` — CAS is flattened in JS here rather than relying on `fhir.build`'s own `"CAS[*].adjustments"` wildcard-flatten path, matching 837P/837I's own "the script owns all row reshaping" precedent) → `fhir.build(ExplanationOfBenefit, rowsPath)` (ONE call, one resource per claim) → `fhir.build(PaymentReconciliation)` (**UNCHANGED** from V234 — reads `parsedEDI` directly, never through the new script's own `step_output`, so none of the row-scoping/snake-casing rules below apply to it) → `payload.builder` (`resourcePaths` mixing the EOB array with the single PaymentReconciliation, proven safe by V231's own original precedent) → `fhir_validation`(strict) → `connector.outbound`(`sink_outbound`).

### A real, previously-undocumented pipeline-engine bug found ONLY by a real Test Pipeline run (not by any Go test)
See gotcha #3 in the "EDI X12 837P/837I → FHIR Mapping" section's own bug list above for the full mechanism — summarized here because IT'S WHAT ACTUALLY HAPPENED to this template during its own full-stack verification: the derive step was named "Derive 835 Claim Header Context" with `step_alias: "derive_835_claim_context"` — the alias silently DROPPED the word "Header" relative to the real step name, and since `steps.<key>` addressing is keyed off `NormalizeKey(step.StepName)` (never the configured `step_alias` field), every `rowsPath`/`sourcePath` referencing `steps.derive_835_claim_context...` resolved to nothing — ZERO `ExplanationOfBenefit` resources built, with `success: true` and no error (the rowsPath simply iterated an empty list). Every Go-level test passed throughout, because the Go test harness's own `injectEDIStepOutput` helper uses the SAME string for both the injected key and the config's own reference — self-consistent, but blind to whether that string would ever actually be produced by the real engine from a real step name. **Fixed by renaming the step to "Derive 835 Claim Context"** (dropping "Header," whose normalized form now equals the already-chosen alias) rather than changing the alias/rowsPath references — caught and fixed within the same session, before this migration was considered done, via the Playwright spec below actually running the pipeline, not just asserting the canvas rendered the right step count.

### Named gaps carried forward from the pre-existing test, still real and evidence-based (not silently dropped)
- `EOB.type` stays hardcoded `"professional"` — an 835 alone can't reliably distinguish claim types without deeper facility-type-code knowledge this pass doesn't verify.
- `EOB.provider`/`EOB.insurance` stay unmapped. `provider` was attempted via `NM1[entityIdentifierCode=82]` (X12's own "Rendering Provider" role code) — a directly correct mapping — but neither real 835 sample in this repo (`blue_cross_nc_sample.txt`, `emedny_sample.txt`, `united_healthcare_legacy_sample.txt`) actually carries an "82" NM1 occurrence at the claim level, so it resolves empty. `insurance` (a `Coverage` reference) was never attempted — no `Coverage` resource is built in this pass. Both are real, expected strict-validation errors — the Go test (`assertOnlyNamedEDI835ValidationGaps`) and the Playwright spec both assert these are the ONLY errors, not that validation passes clean.
- `PLB` (provider-level balance) is NOT mapped — none of the 3 real 835 samples in this repo carries a PLB segment; this project's own established discipline is to not fabricate fixtures where real ones don't exist.
- CAS-derived adjudication entries put the literal CARC code directly on `category.coding[0].code` (system = the verified X12 CARC CodeSystem URL, `https://x12.org/codes/claim-adjustment-reason-codes`) — a deliberate, spec-legal choice (that value set's binding strength is "example" per hl7.org/fhir/R4/explanationofbenefit-definitions.html), not a fabricated crosswalk.

### Verification
Go: `services/executors/transform/edi_835_to_fhir_test.go` rewritten to chain the real executors (`edi.parse` → `enrichment.script` via a locally-defined `runScriptEDI`/`ediStepOutput`/`injectEDIStepOutput` harness, mirroring `pas_fhir_builder_test.go`'s own pattern since this file lives in package `transform`, not `services`, and has no access to those helpers directly → `fhir.build(EOB, rowsPath)` ONCE → `fhir.build(PaymentReconciliation)`, unchanged → `payload.builder` → `fhir_validation` strict) against both real samples with a real claim/multi-claim shape — both existing tests (`TestEDI835ToFHIR_BlueCrossNC_SingleClaim_BuildsCorrectBundleShape`, `TestEDI835ToFHIR_EMedNY_MultiClaim_BuildsOneEOBPerClaim`) upgraded from `t.Logf`-only strict-validation logging to real assertions (`assertOnlyNamedEDI835ValidationGaps`), passing cleanly with exactly the 2 named gaps above and nothing else. Full `services/executors/transform` package suite re-run with zero regressions from the new `_rowIndex` primitive.

Migration: `database/migrations/V247__EDI_835_To_FHIR_Per_Claim_EOB.sql` — new migration number, same slug `edi-835-to-fhir-sftp` (never editing V231/V234 in place), `ON CONFLICT (slug) DO UPDATE`. Its embedded JSON (including the full JS derive script) was generated programmatically from the Go test's own source via a small Node script that reads the `.go` file directly and `JSON.stringify`s the extracted script text — avoiding the hand-retyping risk this project has been bitten by before (a dropped comment block during the 837 work) — rather than transcribed by hand.

Full-stack: real `docker-compose build app` (clean) + restart, Flyway applied cleanly, migration's own generated JSON validated as parseable JSON before applying, template row confirmed live via `psql`. `tests/playwright/edi-835-to-fhir-e2e.spec.js` drives the real "Use Template" click path (9 canvas nodes, matching the 9 execution_groups) → spot-checks 2 step configs → real "Test Pipeline" runs against BOTH real samples, asserting real `item[].sequence` values, real adjudication category codes, and `PaymentReconciliation.detail[].response.reference` correctly rewritten to each EOB's own real `fullUrl` — this is the run that caught the gotcha-#3 bug above; both tests pass after the fix.

### Key Files
- `services/executors/transform/fhir_build_executor.go` (`_rowIndex`/`withRowIndex`, generic)
- `services/executors/transform/edi_835_to_fhir_test.go` (rewritten to use real `rowsPath`, source of truth for the migration)
- `database/migrations/V247__EDI_835_To_FHIR_Per_Claim_EOB.sql`
- `tests/playwright/edi-835-to-fhir-e2e.spec.js`

## Direct Messaging Connector — DirectTrust S/MIME over SMTP/IMAP (September 2026)

### What shipped
`direct_messaging_inbound`/`direct_messaging_outbound` went from 100% non-functional stubs (bare `NewBase{In,Out}boundConnector` returns, `is_active=false` since V69/V221) to real connectors: inbound polls an IMAP mailbox for new S/MIME-encrypted DirectTrust emails, decrypts+verifies each against a configured partner certificate, and enqueues the recovered clinical document as a normal `InboundMessage`; outbound signs+encrypts a clinical document and delivers it as an email via SMTP. Activated via `database/migrations/V248__Add_Direct_Messaging_Partner_Cert_And_Activate.sql`.

### Architecture: reuse AS2's S/MIME primitives directly, new SMTP/IMAP transport layer
DirectTrust's own Applicability Statement specifies the identical "sign then encrypt" CMS layering AS2's RFC 4130 does, so `services/connectors/smime_shared.go` (renamed from `as2_smime.go` once this second connector started depending on it — `as2Identity` renamed to `smimeIdentity` throughout) is reused with **zero changes**: `buildAS2Envelope`/`openAS2Envelope` (despite the "AS2" name, pure CMS sign+encrypt/decrypt+verify with no AS2-specific data at all) are called directly by both new connectors. A new shared helper, `parseSMIMEConfig(ownCertPEM, ownKeyPEM, partnerCertPEM)`, factors out the "parse own identity + partner cert from 3 PEM config strings" pattern both AS2 files already inline separately — added so the two NEW files don't duplicate it a third and fourth time (the existing AS2 files were left as-is, not refactored, to avoid touching already-shipped, tested code beyond the planned rename).

**Email vs. HTTP — one real, deliberate difference from AS2**: AS2 sends the raw CMS envelope as-is over HTTP (binary-safe, no encoding needed). SMTP has no such guarantee across arbitrary relays, so the outbound connector base64-encodes the envelope (wrapped at 76 chars/line per RFC 2045 §6.8) into a standard `application/pkcs7-mime; smime-type=enveloped-data` MIME email body — the universal, real-world S/MIME-over-email convention. The inbound connector reverses this: parse the MIME headers via `net/mail`, strip whitespace, base64-decode, then `openAS2Envelope`.

**IMAP polling** (`direct_messaging_inbound.go`) mirrors `postgresql_inbound.go`'s own polling-loop shape (`SupportsCron() → true`, a ticker-driven `pollLoop` with exponential-backoff reconnect via the existing `IsConnectionError` helper) rather than any listener-style connector — an IMAP session is connection-oriented and stateful, much closer to a DB poller than a one-shot directory scan. Watermark is "highest UID seen so far" (IMAP UIDs are permanent, per-mailbox, monotonically increasing — RFC 3501 §2.3.1.1), persisted via `SetMetadata("last_uid", ...)`, with a defensive `msg.Uid <= lastUID` re-check on every fetched message (IMAP's own `"N:*"` range has a documented edge case where a server may still surface its single highest-UID message even when that UID is below N).

**Dependency**: `github.com/emersion/go-imap v1.2.1` — this project's go.mod had zero IMAP/POP3 client support before this (stdlib has none); SMTP sending uses stdlib `net/smtp` directly, matching this project's own "avoid new dependencies without clear need" discipline (no `go-message`/`go-smtp` needed — a single-part, always-base64 S/MIME body doesn't need a general MIME-building library).

### A real bug found only by a genuine full-stack test, not by any Go-level unit test
The initial `use_tls: false` "controlled test server" escape hatch (for a plain-text mailbox with no STARTTLS) called `smtp.PlainAuth` the same way the TLS path does — but stdlib `net/smtp`'s own `plainAuth.Start()` **refuses to send credentials at all** unless the connection is TLS or the server is literally `"localhost"` (a correct, deliberate security guard against leaking a password in the clear). Every Go-level unit test in this round passed because none of them actually dialed a real non-TLS, non-localhost SMTP server — this was only caught running the real full-stack test against a real greenmail container reachable by its container hostname. **Fixed by accepting the honest trade-off the config's own name implies**: `use_tls: false` now skips SMTP AUTH entirely rather than attempting and failing it — a real production Direct Trust deployment has no reason to ever set `use_tls: false` in the first place, so this trade-off only affects the test-only path it was always scoped to.

### Verification
Go: `services/connectors/direct_messaging_test.go` — reuses `as2_test_helpers_test.go`'s own `generateTestIdentity`/`pemCert`/`pemKey` helpers directly (already generic, not AS2-specific despite the file name) since no live DirectTrust partner exists to test against, matching AS2's own precedent exactly. 10 tests: Initialize/Validate field-completeness checks for both connectors, a full build→process round trip proving `direct_messaging_outbound.go`'s own `buildDirectEmailBody` output is exactly what `direct_messaging_inbound.go`'s own `processEmail` expects (the two connectors' own format assumptions genuinely agree, not just individually plausible), a wrong-signer-rejected direct-trust enforcement test (mirroring AS2's own), and unit tests for the base64-wrap/whitespace-strip helpers. Full `services/connectors` package suite re-run — zero regressions from the `as2Identity`→`smimeIdentity` rename.

Full-stack: a genuine real-network round trip against `greenmail/standalone` (a throwaway Docker container on its own docker network, auth disabled, no TLS — matching this session's own AS2 full-stack precedent of "a throwaway Go client, different program," not a permanent test) — real SMTP `Send()` delivered a signed+encrypted email into a real mailbox, a real IMAP `poll()` fetched it back, decrypted, verified, and recovered the exact original clinical document byte-for-byte; a second poll correctly found nothing new (UID watermark advanced). This is the run that caught the SMTP-AUTH-over-plaintext bug above. The throwaway test file and mail server were removed after verification, matching AS2's own "not part of the permanent test suite" precedent — no long-lived mail server exists in CI. App container rebuilt cleanly with the new `go-imap` dependency; migration V248 applied to the live DB; both connectivity_types rows confirmed `is_active=true` with the new `partner_cert_pem` field via `psql`.

**App-level full-stack (closing the gap named in an earlier draft of this section)**: the user directly asked "did you do fullstack testing?" a second time for this feature — the connector-level proof above didn't yet go through the real wizard/activation/pipeline path AS2's own pass did, so that gap was closed for real, not just re-asserted:
- Created a real `direct_messaging_inbound` interface via `POST /api/wizard/complete` (real session-cookie auth against the real running app, admin credentials) pointed at a real `greenmail-fullstack` container attached to the app's OWN docker-compose network (`ezhealthkonnect_ezhealthkonnect`, not a separate throwaway network — reachable by the app by container hostname, confirmed via `nc` from inside the app container before proceeding). Confirmed via `psql` that the FULL config (`imap_host`/`cert_content`/`private_key`/`partner_cert_pem`/everything) landed correctly in `transformation_steps.config` with **zero manual SQL patching needed** — direct proof that this session's own earlier "Wizard Connector-Map Fix" (`TransformationPipelineService.js`'s dynamic `connectivityType` resolution) now correctly handles a connector type it had never actually been exercised against before, unlike AS2's own experience needing a raw `UPDATE` to get its config through.
- Confirmed via the real in-container `logs/application/app.log` that the real IMAP poller actually started (`📧 Direct Messaging Inbound: Starting (interval=5s)`, `✅ Started direct_messaging_inbound connector for interface efcd8437-...`).
- Sent a real signed+encrypted email into the real mailbox (a throwaway Go program playing the "partner" role, reusing the already-proven `buildAS2Envelope`/`buildDirectEmailBody`). Within one real 5-second poll cycle, the REAL running app decrypted, verified, and stored a real message row: `status=processed`, `error_count=0`, `message_size=130` — an EXACT byte-for-byte match to the original clinical document's length — confirmed both via direct `psql` and via the real `GET /api/messages/interface/:id` REST API (`source_endpoint: "greenmail-fullstack:3143"`, `delivery_status: "delivered"`).
- Created a second real interface with `direct_messaging_outbound` as its target, triggered a real send via `POST /api/messages/send/:interfaceId`, confirmed via `app.log` that the real pipeline executed the real `Send()` (`✅ Outbound Connector delivery complete: direct_messaging_outbound (273 bytes)`), then polled the mailbox AS THE PARTNER (a throwaway Go program, separate identity) and recovered the exact original content sent through the API, decrypted and signature-verified — proving the full outbound path end to end through the real app, not just the connector in isolation.
- Cleaned up afterward exactly as a real user's own data would need to be: deactivated both interfaces via the real `POST /api/processing/interfaces/:id/deactivate`, soft-deleted both via the real `DELETE /api/interfaces/:id` (the app's own audit-retention-preserving delete, not a hard delete), removed the throwaway Go test files, and removed the `greenmail-fullstack` container.

### Scope, named and deliberate (not silently absent)
- **MDN-over-email (DirectTrust delivery/read receipts) is explicitly deferred** — building it requires the inbound poller to also distinguish "a new clinical message" from "an MDN receipt for a message I previously sent," genuine added scope beyond the core clinical-document exchange this phase targets.
- **Manually-configured partner cert only** — no DNS CERT-record (RFC 4398) or LDAP auto-discovery exists anywhere in this codebase (confirmed via repo-wide grep — no `miekg/dns`, no `go-ldap/ldap`); building either is a genuinely separate subsystem, matching AS2's own identical scope boundary.
- **Only base64 Content-Transfer-Encoding is decoded on inbound** — the universal, real-world S/MIME-over-email convention (also what this project's own outbound connector always sends); a message using a different encoding fails to base64-decode and is skipped with a logged warning, not silently corrupted.
- **Assumes one billing-provider-style single mailbox per connector instance** — no multi-mailbox/multi-identity support within one connector config, matching every other connector in this codebase's own single-endpoint-per-config convention.

### Key Files
- `services/connectors/smime_shared.go` (renamed from `as2_smime.go`; `parseSMIMEConfig` added)
- `services/connectors/direct_messaging_inbound.go`, `direct_messaging_outbound.go` (new)
- `services/connectors/direct_messaging_test.go` (new, 10 tests)
- `services/connectors/connector_stubs.go` (old stub functions removed)
- `database/migrations/V248__Add_Direct_Messaging_Partner_Cert_And_Activate.sql`
- `go.mod`/`go.sum` (new `github.com/emersion/go-imap` dependency)

### Named future work
Async MDN-over-email and DNS/LDAP-based partner-certificate auto-discovery remain genuinely separate, not-yet-built items if ever needed — the same two items AS2 itself still has open (async MDN, in AS2's case) plus the LDAP/DNS discovery gap both transports share.

## WebSocket Connector — Real-Time Bidirectional Pair (September 2026)

### What shipped
A user asked whether HTTP/TCP/WebSocket outbound senders let a downstream pipeline step read the response and use it further. HTTP already did (`response_status`/`response_body`/`response_headers`); TCP/MLLP surfaced the raw ACK text but not the parsed AA/AE/AR code (fixed first, small additive change); WebSocket didn't exist as a connector at all — confirmed via full grep, only a dead `ProtocolWebSocket` enum value with zero wiring. `websocket_inbound` (a real-time server) and `websocket_outbound` (a client that sends a frame and reads back a response over the same connection) close that gap, using `github.com/gorilla/websocket`. Activated via `database/migrations/V249__Add_WebSocket_Connectivity_Types.sql`. Zero frontend changes needed — confirmed the config UI (`ConnectorConfigBuilder.js`), connector-type dropdowns, and toolbox are all schema/DB-driven with no per-type-name branching that would exclude a new type.

### Architecture
`websocket_inbound.go` mirrors `tcp_mllp_inbound.go`'s `Start()` shape (launches its server in a goroutine and returns immediately) rather than `as2_inbound.go`'s/`http_rest_inbound.go`'s shape (both block on `<-ctx.Done()` before calling `Stop()`) — a real, pre-existing subtlety found while designing this: since `processing/engine.go` always passes `context.Background()` (never cancelled) to `Start()`, that blocking shape leaks a goroutine for the process's lifetime in both of those existing connectors, harmless only because `DeactivateInterface` calls `connector.Stop()` on a separate direct path. Not fixed in those files (out of scope), but not repeated here.

Second subtlety: `gorilla/websocket`'s `Upgrader.Upgrade()` hijacks the connection out of `net/http`'s own tracking, so `http.Server.Shutdown()` (graceful drain) silently does **not** close or wait for any already-upgraded websocket connection. `Stop()` therefore uses `server.Close()` (immediate) plus an explicit close of every tracked `*websocket.Conn` — the same two-step shutdown `tcp_mllp_inbound.go` uses for its own raw `net.Conn` map, regression-guarded by `TestWebSocketInbound_Stop_UnblocksBlockedConnections`.

`websocket_outbound.go` mirrors `tcp_mllp_outbound.go`'s retry-in-`Send()`/persistent-or-per-message shape. A response read timeout or error is **not** treated as a `Send()` failure (the write already succeeded — many real websocket sends are fire-and-forget with no inline reply); it's surfaced as `response_received: false` instead, and marks the persistent connection dead so the next `Send()` transparently redials. A text-frame response lands in `response_body` (reusing the same key HTTP outbound already uses); a binary-frame response is base64-encoded into a separate `response_binary_base64` key (+ `response_frame_type: "binary"`) rather than silently dropped or corrupted by a naive `string()` conversion.

### Generalized response-metadata surfacing (`outbound_connector_executor.go`)
Before this work, every connector's response data was surfaced into `step_output` via a growing hardcoded `if _, ok := result.Metadata["X"]` chain (one per key, per connector type) — exactly the one-branch-per-type anti-pattern this file's own OOP standards section calls out. Replaced with two special-cased keys (kept exactly as-is because `public/js/messages.js`'s step-execution detail panel reads them by these specific names: `status_code`→`response_status`, `response_body`→full value + truncated `response_body_preview`) plus a generic loop forwarding every other `result.Metadata` key as-is. `ack_code`, `response_headers`, `response_received`, `response_binary_base64`, and `response_frame_type` all flow through automatically now — a future connector's response data needs zero changes to this file.

### Verification
Go: `services/connectors/websocket_test.go` (13 tests — Initialize/Validate field checks, a real local echo-server round trip proving `response_body` capture, binary-response base64 encoding, a no-response-within-timeout case proving `Send()` still succeeds, persistent-mode reconnect-after-drop, and a real `Stop()`-unblocks-a-blocked-connection test) and `services/executors/transform/outbound_connector_websocket_response_test.go` (proves the generalized surfacing mechanism end-to-end through the real `OutboundConnectorExecutor`) — all pass, plus the full `services/connectors` and `services/executors/transform` package suites re-run with zero regressions (494s + 21s, both green).

Full-stack: real `docker-compose up -d --force-recreate app`, `V249` confirmed live via `psql`. Created a real `websocket_inbound` interface via the real wizard API, activated it, confirmed via `logs/application/app.log` the real listener started (`✅ WebSocket Inbound: Listening on port 9501/ws`); a throwaway Go client (gorilla/websocket, attached to the app's own docker network) sent a real message, confirmed via `GET /api/messages/interface/:id` a real message row landed with the exact byte count. Created a second real interface with `websocket_outbound` as its target pointed at a throwaway Go echo-server container on the same network; sent a real HTTP request into its (real, non-test-mode) inbound side, confirmed via direct `psql` on `step_executions.step_output` that the real echoed response (`"ECHO: ..."`) landed in `response_body`, with `response_received: true` — the exact mechanism the original question was about, proven working end-to-end through the real running app, not just at the connector or dry-run-Test-Pipeline level (Test Pipeline's dry-run mode validates config/previews the payload but never calls `Send()` over the network — a real message had to flow through un-test-mode to prove this). Cleaned up: deactivated + soft-deleted both interfaces, removed the throwaway containers/images/Go files.

### Named scope, deliberate
- **No pipeline-driven synchronous reply frame on the inbound side** — an inbound-received message is enqueued and acknowledged only at the transport level (no MLLP-style mandatory ACK either); replying with an actual pipeline result would require blocking the connection's goroutine on full async pipeline completion, a genuinely new architecture no existing connector in this codebase has.
- **Binary responses are base64-encoded, never silently dropped** — a deliberate, user-confirmed choice over simply logging-and-discarding a binary reply.

### Key Files
- `services/connectors/websocket_inbound.go`, `websocket_outbound.go` (new)
- `services/connectors/websocket_test.go` (new, 13 tests)
- `services/connectors/connector_factory.go` (2-line registration)
- `services/executors/transform/outbound_connector_executor.go` (generalized metadata surfacing)
- `services/executors/transform/outbound_connector_ack_code_test.go`, `outbound_connector_websocket_response_test.go` (new)
- `database/migrations/V249__Add_WebSocket_Connectivity_Types.sql`
- `go.mod`/`go.sum` (new `github.com/gorilla/websocket` dependency)

## EDI X12 276/277 — Claim Status Request/Response, Async + Sync (September 2026)

### Scope
Following on from 835/837/999/270/271, the user picked 276/277 (Health Care Claim Status Request/Response) as the next transaction set — the cheapest option, since the EDI engine is schema-driven (a new transaction set is a schema file, not new Go code). **278 (prior auth) and 834 (enrollment) are explicitly OUT of scope for this pass** — user-confirmed follow-on phases, not attempted here; 834 in particular has a genuinely different shape (batch member maintenance, not request/response) and deserves its own scoped pass. The user also confirmed this round should include a **synchronous** real-time request/response HTTP endpoint (mirroring Sync Eligibility for 270/271), not just the async SFTP-polling connector path.

### Engine — schema data only, zero new Go engine code
`edi/schemas/x12_005010/segments/STC.json` (new — Status Information, the 277's own claim-status-carrying segment, 12-position composite-heavy shape) plus `transactionSets/276.json`/`277.json`, registered in `manifest.json`. Both are 5-level HL hierarchies (2000A payer → 2000B clearinghouse/information receiver → 2000C provider → 2000D subscriber → 2000E dependent), sourced from PyX12's real 005010X212 IG map, cross-checked against Stedi's HIPAA-specific pages (`stedi.com/edi/hipaa/transaction-set/276-A1`/`277-A1` — NOT the generic `x12-005010` URL pattern, which 404s for HIPAA transaction sets). 277 additionally carries `STC` at every claim-level loop (`2200B`/`2200C` trace-level, `2200D`/`2200E`/`2220D`/`2220E` claim/service-line-level) and reuses `HL`/`NM1`/`TRN`/`REF`/`AMT`/`DTP` from the existing shared segment library unchanged (each verified against its own element list first — no transaction-set-specific fixed-value collision, unlike the 837 CUR-field lesson). Neither `X12LoopDef.Wrapper` (LS/LE wrapping) nor `TriggerDiscriminator` was needed — 276/277's HL loops are unambiguous by structure alone, same as 270/271.

**The recurring HL-hierarchy nesting bug hit again**: "2000E" (Dependent) is a SIBLING of "2100D" within **2000D's own** `loops` map (Dependent's real HL parent is the Subscriber, not the Provider) — got this wrong on the first pass in 4 separate places (2 Go round-trip tests' `BuildInput` literals, 2 JS derive scripts) before correcting, the same class of mistake 270/271 already logged once. `STC`/`REF`/`AMT`/`DTP` all carry `maxUse: ">1"` in their own segment definitions, so the real parser always array-wraps them even when only one instance is present — this bit the FHIR-mapping derive scripts specifically on `REF` (see the bug note below), not just the round-trip tests' `STC` handling.

### FHIR mapping — Task, not ClaimResponse
`Task` (not `ClaimResponse`) is the mapping target for both 276 and 277: a claim status REQUEST is an administrative "please tell me the status of this claim" ask, which FHIR's own Task resource (a generic request-for-work-to-be-done) models directly (`status=requested`/`intent=order`); ClaimResponse is reserved for actual adjudication content this transaction set doesn't carry. 277's own X12 STC category code (`A1`-`A8`/`F0`-`F3`/`P0`-`P5`) is translated into FHIR's `Task.status` vocabulary via `stcCategoryToTaskStatus()` — a code-system translation, the same class of transform 270/271 already does for gender/relationship codes, never an invented fact: the status itself always comes straight from the source STC, and the raw category code is preserved alongside the translation on `Task.businessStatus`.

**A real bug caught only by a genuine browser Test Pipeline run, not the Go-level tests**: both derive scripts' `patient_control_number`/`payer_claim_control_number` field read `claim.REF.referenceIdentification` directly — correct against the Go tests' own hand-built fixture (a plain map), silently `null` against the REAL parser's output, because `REF` has `maxUse: ">1"` and is therefore always array-wrapped. The Go-level `*_fhir_builder_test.go` tests never caught this because their fixtures are hand-constructed Go maps, not real parser output — the exact same masking effect the STC `maxUse` bug already demonstrated for the round-trip tests earlier in this same phase, just resurfacing one layer up (FHIR-mapping derive scripts, not engine round-trip tests) and only visible once a real `edi.parse` → `enrichment.script` chain ran through the actual browser Test Pipeline path. Fixed by adding a `first()` helper to both derive scripts (276's script didn't have one yet; 277's already did for STC) and wrapping the REF read accordingly — backward-compatible with the Go tests' own plain-map fixtures, since `first()` treats a non-array as a 1-element array.

### Synchronous endpoint
`controllers/sync_claim_status_controller.go` — a near-verbatim copy of `sync_eligibility_controller.go`'s structure: resolves the target interface's own configured pipeline by `(interfaceID, messageType="276")`, calls `TransformationPipelineService.ExecutePipeline` directly and synchronously from the HTTP handler (not the usual connector→channel→async-goroutine path), reuses the existing package-level `extractFirstBuildStepPayload` helper unchanged. Registered in `main.go` under `api.Group("/claim-status")` → `POST /:interfaceId/check`, proxied via `app.js`'s `app.use('/api/claim-status', forwardToGo)`.

### Migrations
`V250` widens `edi_x12_inbound`'s `transaction_types` UI enum (mirrors V241's exact `jsonb_set` pattern). `V251`/`V252` (276→FHIR, 277→FHIR OOB templates) mirror V242/V243's structure — 10-step pipelines (`connector.inbound` → `edi.parse` → `edi.validate` → `enrichment.script` derive → `fhir.build`×3 [Organization/Patient/Task] → `payload.builder` → `fhir_validation` → `connector.outbound`/`sink_outbound`). Both migrations' embedded JSON (including the full JS derive script) were generated programmatically from `services/edi_276_fhir_builder_test.go`/`edi_277_fhir_builder_test.go`'s own Go source via a Node script (avoiding the hand-retyping bug class the 837 work hit once) — regenerated a second time after the `REF` fix above, then re-applied directly via `psql` (the migration files were minutes-old, same-session, unreleased-beyond-this-repo edits, matching the established "safe to re-apply directly, only because it's this fresh" precedent from the 837 work) plus `flyway repair` to reconcile the changed checksum.

**Named simplification, not deduplicated**: the Organization (payer) `fhir.build` step uses the SAME `rowsPath` as Patient/Task (one per claim status context), matching V242's own 270 template precedent — so a file with 2 claim status contexts sharing the same real-world payer produces 2 duplicate Organization Bundle entries, not 1. Harmless per FHIR Bundle semantics, same "not deduplicated across claims" precedent 837P/837I's own Patient/Coverage mapping already established.

### Verification
Go: `edi/real_schema_integration_test.go`'s 276/277 round-trip tests (subscriber + dependent claim status, 277's `2200B`/`2200C` trace STC plus finalized/pending claim-level STC) and `services/edi_276_fhir_builder_test.go`/`edi_277_fhir_builder_test.go` (derive → `fhir.build`×3 → `payload.builder` → `fhir_validation` strict, zero unexpected errors) — all pass. `controllers/sync_claim_status_controller_test.go` (4 tests: real round trip, unknown interface → 404, empty body → 400, malformed EDI → real error status) — all pass against real Postgres. Full `services`/`edi`/`controllers` package suites re-run with zero regressions.

Full-stack: real `docker-compose build app` + migration apply, `V250`/`V251`/`V252` confirmed live via `psql` (both templates present, transaction types enum widened). `tests/playwright/edi-276-277-to-fhir-e2e.spec.js` drives the real "Use Template" click path for both templates (10 canvas nodes each) then a real "Test Pipeline" run against the self-authored samples (`edi/testdata/real_samples/self_authored_27{6,7}_sample.txt`), asserting real resource counts and field values in the assembled Bundle — this is the run that caught the `REF` array-wrapping bug above. `tests/playwright/sync-claim-status-e2e.spec.js` (4 tests, mirroring `sync-eligibility-e2e.spec.js`) proves the real synchronous round trip against `/api/claim-status/:interfaceId/check` through a real interface + pipeline created via the same REST endpoints the UI itself calls. All throwaway interfaces deactivated + soft-deleted after verification; throwaway `ehk-gobuilder-tmp` image removed.

### Named future phases (STATUS UPDATE — both since closed, kept for history)
278 (prior auth) and 834 (enrollment) were still out of scope as of this round, but both have SINCE shipped — see the "EDI X12 278 (Prior Authorization) + 834 (Benefit Enrollment) — Full Build" section below for the current status.

### Key Files
- `edi/schemas/x12_005010/segments/STC.json`, `transactionSets/276.json`, `transactionSets/277.json`, `manifest.json` (registration)
- `public/js/pipeline/components/EDIStepBuilder.js` (`EDI_TRANSACTION_SETS` widened)
- `database/migrations/V250__EDI_Inbound_Transaction_Types_Add_276_277.sql`, `V251__EDI_276_To_FHIR_OOB_Pipeline_Template.sql`, `V252__EDI_277_To_FHIR_OOB_Pipeline_Template.sql`
- `controllers/sync_claim_status_controller.go`, `main.go` (route registration), `app.js` (proxy line)
- `services/edi_276_fhir_builder_test.go`, `services/edi_277_fhir_builder_test.go`, `controllers/sync_claim_status_controller_test.go`
- `edi/testdata/real_samples/self_authored_276_sample.txt`, `self_authored_277_sample.txt`
- `tests/playwright/edi-276-277-to-fhir-e2e.spec.js`, `tests/playwright/sync-claim-status-e2e.spec.js`

## EDI X12 278 (Prior Authorization) + 834 (Benefit Enrollment) — Full Build (September 2026)

### Scope and why it's not redundant with PAS
The user asked whether 278 was already covered by the already-shipped Da Vinci PAS work — it is not.
PAS is FHIR-native (`Bundle`/`Claim`/`ClaimResponse` over a REST `$submit` operation, via
`pas_envelope_mapping` → `field_mapping`); X12 278 (Health Care Services Review Request/Response) is
the EDI transaction set for the *same real-world business process*, exchanged over SFTP like every
other transaction set in this engine — genuinely different wire formats, not a duplicate (confirmed
via a full grep of `edi/` turning up zero prior references to `278`/`834`). User-confirmed scope:
full completeness for 278 (X217 Request-for-Review-and-Response only — X215 Inquiry/Response and X216
Notification/Acknowledgment named, deferred), Claim/ClaimResponse as 278's FHIR target (matching
PAS's own resource choice, not `Task` — 278 carries real clinical/service content and an
adjudication-style decision Task can't represent), a synchronous `/api/prior-auth/:interfaceId/check`
endpoint, and full completeness for 834 too (every loop, including the less-common demographic/COB/
reporting-category ones).

### 278 — a genuinely new structural fact: ONE schema serves BOTH directions
Unlike every prior request/response pair this engine has built (270/271, 276/277 — different ST01;
837P/837I — same ST01, disambiguated by a composite `ST01:GS08` key), **278 request and response
share the IDENTICAL ST01='278' AND GS08='005010X217'** — no envelope-level way to tell them apart.
Modeled as ONE unified `edi/schemas/x12_005010/278.json`, registered once under bare `"278"` — every
segment/loop from both the Stedi-sourced A1 (request) and A3 (response) trees unioned into one tree
(the same permissive-union philosophy CTX already established for 999). The FHIR derive script
determines "is this a response" purely from **HCR's own presence** (the certification/decision
segment) — a structural fact already in the message, never invented, the same pattern 271's own
AAA-based rejection detection already uses. New segments: `UM`, `HCR`, `CR5`, `CR6`, `SV3`, `TOO` (6)
— 21 more reused from the existing shared library after verifying each one's own element list first.

**A real, new engine limitation found and fixed via an actual round-trip parse failure**: 2000E
("Patient Event Level") occurs at TWO tree positions with the IDENTICAL discriminator value
(`hierarchicalLevelCode`="EV") — nested inside 2000D for the dependent's own event, AND as a direct
sibling of 2000D inside 2000C for the subscriber's own event. `selectLoopCandidate`'s own
TriggerDiscriminator match has no awareness of HL02 (hierarchicalParentIdNumber) or tree-depth
context — so a still-open recursive `matchLoops` call for the DEEPER position would greedily consume
a LATER "EV"-coded HL that actually belonged to the SHALLOWER sibling slot, if left `repeat='>1'`
(unbounded). Fixed by capping repeat='1' at both occurrences — the second "EV" sighting then correctly
fails the already-matched check at the deeper level and falls through to the shallower slot. A real,
named scope reduction (at most one patient event per subscriber, at most one per dependent) — and a
genuinely new class of gotcha worth remembering for any future transaction set needing the SAME loop
id/discriminator combination at more than one nesting depth.

**Shared-segment global-`usage` conflicts, the same class as CL1's own 837I-vs-999 lesson**: `UM`
(required at 2000E, optional at 2000F), `SV1`/`SV2` (required within 837P/837I's own single-position
use, but 278's 2000F offers a CHOICE of SV1/SV2/SV3 — none individually mandatory), and `TRN`
(required historically, but genuinely optional at both 278 positions) all needed their GLOBAL `usage`
widened from `required` to `situational` — this engine has no per-loop override, so a segment's
`usage` must reflect its LEAST-restrictive real position across every transaction set that shares it.
Purely permissive widenings — zero behavior change for every existing consumer, which already
supplies these segments in every real fixture.

**`fhir.build` has no whole-resource-row "condition" gate** (confirmed by reading the executor
directly — `Condition` exists only on individual fields/repeatingGroups, never at the top-level
config) — an initial design mistakenly relied on one to gate ClaimResponse to response-only rows,
which silently built a ClaimResponse for EVERY row instead. Fixed the correct way: the derive script
itself produces a SECOND, pre-filtered `_response_contexts` array (only rows where `is_response` is
true), and ClaimResponse's own `rowsPath` points at that narrower array — the same "0-or-1-element
array so a resource simply isn't built for excluded rows" convention 271's own insurance/rejection
derive script already established.

### 834 — a genuinely different, flatter shape
Confirmed directly from Stedi's own page data: 834's repeating unit is `LOOP 2000` (triggered by
`INS`, not `HL`) — every member is its own flat, top-level instance; `INS02` (relationship code)
distinguishes self/spouse/child at the SAME nesting level, never HL-nested the way every other
transaction set in this engine nests dependents inside their subscriber. Zero risk of the "recurring
HL-nesting bug" this engine has hit twice before (276/277) — 834 needs no HL-parent bookkeeping at
all. The outer `2700` (Member Reporting Categories) loop is LS/LE-wrapped — the exact same bracketing
shape already proven for 271's own loop 2120 (`X12LoopDef.Wrapper`), reused unchanged.

**8-way sibling discriminator, the widest yet**: `2100A`-`2100H` all trigger on `NM1` under the SAME
parent (`2000`). Real fixed values sourced directly from Stedi's own segment notes where available
(`2100A`="IL" and `2100B`="70", both confirmed word-for-word from 2100B's own note text, not
inferred) and from the standard Entity Identifier Code list (element 98) by exact or closest semantic
name-match for the rest (`2100C`="31" Postal Mailing Address, `2100D`="36" Employer, `2100E`="83"
Subscriber's School, `2100F`="S3" Custodial Parent — an exact match, `2100G`="QD" Responsible Party,
`2100H`="45" Drop-off Location — an exact match). `1000A`/`1000B` (Sponsor/Payer, sharing `N1` as
trigger) use "P5"/"PR" (the SAME "PR" code this schema library already uses for payer roles
throughout 270/271/276/277); `1000C` (TPA/Broker, repeat 2) is deliberately left undiscriminated as a
pure catch-all — real files use either "TV" or "BR"/"BO" at that slot and the discriminator mechanism
only supports one expected value, the same "undiscriminated catch-all" precedent 837I's own 2310C
already established. New segments (11): `BGN`, `EC`, `ICM`, `HLH`, `LUI`, `HD`, `IDC`, `PLA`, `COB`,
`DSB`, `ACT`. `INS.json` extended with positions 6-17 (Medicare status composite, COBRA/employment/
student-status codes) purely additively — positions 1-5 (already used by 270/271/837) unaffected.

### FHIR mapping
**278**: `connector.inbound` → `edi.parse` → `edi.validate` → `enrichment.script` (derive) →
`fhir.build(Organization)` (UMO, single-resource) → `fhir.build(Patient)` (rowsPath) →
`fhir.build(Claim)` (rowsPath, ALWAYS built, `use=preauthorization`, diagnosis[] from HI, item[] from
each service level's own SV1 procedure code) → `fhir.build(ClaimResponse)` (rowsPath against the
pre-filtered `_response_contexts` array — see the gotcha above) → `payload.builder` →
`fhir_validation`(strict) → `connector.outbound`(`sink_outbound`). HCR01's own action code (A1
Certified-in-total, A2 Certified-partial, A3 Not-Certified, A4 Pended, A5 Upheld, A6 Modified,
sourced directly from element 306's own standard code list) is translated into FHIR's
outcome/disposition vocabulary — a code-system translation, never an invented fact.

**834**: `connector.inbound` → `edi.parse` → `edi.validate` → `enrichment.script` (derive, producing
one flattened row per member-coverage pair) → `fhir.build(Organization)` (Sponsor, single-resource)
→ `fhir.build(Patient)` / `fhir.build(Coverage)` (both rowsPath off the SAME flattened array, not
deduplicated across a member's own multiple coverages — the same precedent 837P/837I's own Patient/
Coverage mapping already established) → `payload.builder` → `fhir_validation`(strict) →
`connector.outbound`(`sink_outbound`). HD01's own maintenance type code (same code set INS03 already
uses — "021"/"024"/"025" Addition/Change → `active`, "030" Termination → `cancelled`) is translated
into FHIR's Coverage.status vocabulary. No synchronous endpoint — X12 itself has no 834-response
transaction set (a pure one-way roster feed), so a real-time request/response pattern doesn't apply.

### Verification
Go: `edi/real_schema_integration_test.go`'s 278 (proving BOTH a pure request and a real HCR-bearing
response on one document) and 834 (proving flat, non-HL-nested members with genuinely different
maintenance-type codes) round-trip tests, `services/edi_278_fhir_builder_test.go`/
`edi_834_fhir_builder_test.go` (derive → fhir.build chain → payload.builder → fhir_validation strict,
zero unexpected errors), `controllers/sync_prior_auth_controller_test.go` (4 tests, mirroring
`sync_claim_status_controller_test.go`) — all pass against real Postgres. Full `edi`/`services`/
`controllers` package suites re-run with zero regressions (`services` alone: 118s, all subpackages
green) from the CL1/UM/SV1/SV2/TRN usage widenings and the INS extension.

Full-stack: real `docker-compose build app` + migration apply, `V253`/`V254`/`V255` confirmed live via
`psql` (both templates present, transaction types enum widened to include 278/834).
`tests/playwright/edi-278-to-fhir-e2e.spec.js` and `edi-834-to-fhir-e2e.spec.js` drive the real "Use
Template" click path (11 and 10 canvas nodes respectively) then a real "Test Pipeline" run against the
self-authored samples (`edi/testdata/real_samples/self_authored_27{8}_sample.txt`,
`self_authored_834_sample.txt` — each generated via `edi.build` itself and round-trip-verified before
being committed), asserting real resource counts and field values in the assembled Bundle — this is
the run that caught the `fhir.build` condition-gate bug above. `tests/playwright/
sync-prior-auth-e2e.spec.js` (4 tests, mirroring `sync-claim-status-e2e.spec.js`) proves the real
synchronous round trip. All throwaway interfaces deactivated + soft-deleted after verification;
throwaway `ehk-gobuilder-tmp` image removed.

### 3 real bugs found only by testing against genuine, third-party X12.org sample data (not this session's own synthetic fixtures)
Following the exact same discipline 837P/837I's own real-sample verification round already
established, real 278/834 content was sourced from X12.org's own official worked examples
(x12.org/examples/005010x217, x12.org/examples/005010x220 — the standards body's own published
educational material, envelope segments synthesized since the pages show only ST...SE, every body
segment extracted directly from each page's own raw HTML `<p class="data">` content, not AI-
summarized or retyped from memory). Six 278 scenarios (Referral request+response, institutional
Admission for Surgery request+response — a genuine real CL1 occurrence, Home Health Care — CR6 plus a
genuine two-diagnosis single-HI occurrence and two separate 2000F service levels) and three 834
scenarios (Enroll Employee in Multiple Products — one real member with three real 2300 occurrences,
Add Dependent Full-Time Student, Terminate Eligibility) all parsed and, where chained through the full
FHIR mapping, strict-validated cleanly — but sourcing and testing them surfaced 3 real, consequential
bugs no synthetic fixture had exercised, all in 834:
1. **1000B (Payer)'s own discriminator was wrong.** Modeled as `entityIdentifierCode="PR"` by analogy
   with 270/271/276/277's own payer-role convention — but every one of X12.org's own official 834
   examples uses `N1*IN*` for this role ("IN" = Insurer), never "PR". A concrete demonstration that a
   plausible code borrowed from a DIFFERENT transaction set's own established convention is not a
   substitute for checking this transaction set's own real usage — corrected to "IN".
2. **2100E (Member School)'s own discriminator was wrong.** Modeled as `"83"` (Subscriber's School) —
   a best-semantic-match guess against the generic element 98 code list, explicitly flagged in its own
   sourceRefs as "revisit if a real sample surfaces a different code." X12.org's own official "Add
   Dependent, Full-Time Student" example uses `NM1*M8*` instead — corrected to "M8" after finding it in
   real data, exactly the revisit that note called for.
3. **The most consequential: `maintenanceTypeToCoverageStatus`'s own mapping was BACKWARDS.** The
   original derive script mapped HD01/INS03 `"024"→"active"` and `"030"→"cancelled"` — checked against
   Stedi's own generic element 875 code list this time (not skipped), which shows "024" = "Cancellation
   or Termination" and "030" = "Audit or Compare" (not a termination code at all). Confirmed directly
   against X12.org's own official "Terminate Eligibility for a Subscriber" example, which uses
   `INS*Y*18*024*08*A***TE~` — real title, real code, unambiguous. This session's own self-authored
   834 test fixtures had ALSO used "024" to mean "active, Addition" and "030" to mean "terminated,"
   compounding the same wrong assumption across the round-trip test, the FHIR-builder test, and the
   self-authored Playwright sample — all three corrected together, and the migration regenerated and
   re-applied to the live DB (`flyway repair` reconciling the changed checksum, the same established
   "safe to re-apply directly, only because it's this fresh" precedent from the 837 work).

278's own real samples caught zero further bugs on this pass — a real, worth-stating finding in its
own right, corroborating that the repeat=1 Patient Event fix and the CL1/SV1/SV2/UM/TRN usage
widenings (both found via this session's own synthetic round-trip tests, before real data was sourced)
were the genuinely load-bearing fixes, not papering over a narrower synthetic-only gap.

### Named future phases
X215 (278 Inquiry/Response — a lighter authorization-status-check variant, analogous to 276/277 but
for auth status) and X216 (278 Notification/Acknowledgment) remain explicitly out of scope, per the
user's own confirmed sequencing. 834's own less-common loops (2100B-H demographic variants, 2200
disability, 2310/2320/2330 provider + coordination-of-benefits, 2700/2750 reporting categories) are
schema-complete but not yet mapped into FHIR fields — a named, deferred follow-on, not a schema gap.

### Key Files
- `edi/schemas/x12_005010/segments/{UM,HCR,CR5,CR6,SV3,TOO,BGN,EC,ICM,HLH,LUI,HD,IDC,PLA,COB,DSB,ACT}.json` (17 new), `INS.json` (extended), `CL1.json`/`SV1.json`/`SV2.json`/`TRN.json` (usage widened)
- `edi/schemas/x12_005010/transactionSets/278.json`, `834.json` (1000B/2100E discriminators corrected against real data), `manifest.json` (registration)
- `public/js/pipeline/components/EDIStepBuilder.js` (`EDI_TRANSACTION_SETS` widened to add `'278'`/`'834'`)
- `database/migrations/V253__EDI_Inbound_Transaction_Types_Add_278_834.sql`, `V254__EDI_278_To_FHIR_OOB_Pipeline_Template.sql`, `V255__EDI_834_To_FHIR_OOB_Pipeline_Template.sql`
- Real-sample verification: `edi/real_samples_278_834_test.go` (6 tests against X12.org's own official examples, real-parser-only), `services/edi_278_834_real_parser_roundtrip_test.go` (3 tests chaining real parsed data through the full FHIR mapping + strict validation), `edi/testdata/real_samples/x12org_278_{1a_referral_request,1b_referral_response,2a_admission_request,2b_admission_response,4_home_health_request}_sample.txt`, `x12org_834_{1_multi_product_enrollment,2_add_dependent_student,7_terminate_eligibility}_sample.txt`
- `controllers/sync_prior_auth_controller.go` (new), `main.go` (route registration), `app.js` (proxy line)
- `services/edi_278_fhir_builder_test.go`, `services/edi_834_fhir_builder_test.go`, `controllers/sync_prior_auth_controller_test.go`
- `edi/testdata/real_samples/self_authored_278_sample.txt`, `self_authored_834_sample.txt`
- `tests/playwright/edi-278-to-fhir-e2e.spec.js`, `edi-834-to-fhir-e2e.spec.js`, `sync-prior-auth-e2e.spec.js`

## Universal EDI X12 Receiver — Auto-Route to the Right FHIR Mapping (September 2026)

### Why it exists
The user asked why EDI, which already auto-detects the transaction set from every file's own
ST01/GS08 (identically to how HL7 auto-detects from MSH-9), couldn't offer the same "drop any file
into one interface" experience as HL7's own Universal Receiver — since all 9 existing per-type
X12-to-FHIR templates (835, 837P, 837I, 270, 271, 276, 277, 278, 834) already exist and are already
tested. Confirmed buildable with **zero new Go code** — see `edi-universal-to-fhir-sftp`, a 10th,
purely additive OOB template alongside (not replacing) the 9 per-type ones.

### Two designs considered; one was a dead end found by reading the code, not assumed
`switch_case` + `route_to_step` + `parent_conditional_step_id` branch-skipping (the obvious first
idea) is real but **cannot be authored inside an OOB template**: `public/js/dashboard.js`'s
`useTemplate` flow strips every step's own `id` before saving (`({ id, ...step }) => ({ ...step })`,
avoiding duplicate-key errors when a template was saved from a real interface), and
`controllers/pipelineController.js`'s `savePipeline` then generates a **fresh** `uuidv4()` per step
with no pass that rewrites any `parent_conditional_step_id` reference elsewhere in the same payload to
match. Confirmed by reading both files directly. This is the exact same root-cause class already
documented for `control.loop`'s own `childStepIds` (837P/837I's own "EDI X12 837P/837I → FHIR
Mapping" section above) — a second, previously-undiscovered instance of the same generic gap, not
something to work around per-feature; fixing `savePipeline`/`clonePipeline` generically to support
branch-linked steps in a template stays a separate, unstarted item.

The working design needs no branch-skipping at all: every one of the 9 derive scripts already guards
on `parsed.transactionSet` and returns empty arrays for a non-matching type (this self-gating idiom
already existed in 8 of the 9 — `derive_835_claim_context` was the one exception, predating the
convention; given the same guard here). Every downstream `fhir.build` already reads `rowsPath` off
those arrays, so a non-matching branch's build steps are safe no-ops (0 rows → 0 resources), not
skipped steps. The cost: all 9 derive scripts and 29 `fhir.build` calls execute on every message even
though only one branch's worth ever produces output — a real, named step-count/performance trade-off,
not a correctness one.

### 3 real bugs found by directly querying the 9 live templates and by actually running the merged pipeline — none of them hypothetical
1. **`outputField` collisions.** Confirmed by querying `interface_templates` directly (not assumed):
   `message.fhirPatients` is reused by 8 of the 9 branches, `message.fhirOrganization` by both 278 and
   834, `message.fhirCoverages` by both 834 and 837I/837P, `message.fhirClaims` by both 278 and
   837I/837P, `message.fhirTasks` by both 276 and 277. Merged naively, whichever branch's (possibly
   empty) build ran last would silently overwrite an earlier branch's real output at that shared key —
   a file-corrupting bug, not a hypothetical one. Every one of the 29 `fhir.build` steps' `outputField`
   (and every one's own generic, also-reused `step_alias`, e.g. `build_patient_fhir` × 8) is renamed
   with a per-branch suffix.
2. **3 of the 29 `fhir.build` steps built an unconditional resource regardless of detected type**
   (278's UMO Organization, 834's Sponsor Organization, 835's PaymentReconciliation — confirmed by
   querying which steps lacked `rowsPath`, not assumed). Converted to `rowsPath` mode against a new
   0-or-1-row context array each branch's own derive script now also returns (same convention 278's
   pre-existing `_response_contexts` already established). For the 2 pure-`literalValue` Organization
   builds this needed zero field changes — `literalValue` wins regardless of row content, only
   `rowsPath` + a renamed `outputField` were added.
3. **The `steps.<alias>.step_output` snapshot recursively snake_cases every key it doesn't already
   recognize as snake_case — including arbitrarily deep nested content, not just one level** (the same
   gotcha already named in the 837P/837I section above, rediscovered here in a NEW shape). The first
   attempt at fixing #2 for 835's PaymentReconciliation embedded the **entire** `parsedEDI` object
   under one key on the new context row (`{ parsedEDI: parsed }`), reasoning that every existing
   `sourcePath` would keep working one level down after a `message.parsedEDI.` → `parsedEDI.` prefix
   strip. Running it for real showed every `sourcePath`-based field resolved empty — the snapshot had
   silently rewritten the WHOLE embedded structure to `parsed_edi.header.bpr.payment_effective_date`
   etc., not just top-level keys. Fixed by NOT embedding raw parsed content at all: the new context row
   instead carries a small set of ALREADY-snake_case scalar fields (`check_or_eft_trace_number`,
   `payment_effective_date`, `total_actual_provider_payment_amount`, `payer_name`) reusing values the
   script already computes for `claim_rows`, plus `claim_rows` itself embedded on the same row for the
   repeating adjustments group (reusing its own already-proven `claim_payment_amount`/
   `patient_control_number` keys) — no raw nested structure crosses the snapshot boundary at all.
4. **A sequence-number collision, found only by an actual Test Pipeline run against a real 834
   sample, not by any static check.** The shared bottom group (`payload.builder`/`fhir_validation`/
   `connector.outbound`) was originally sequenced at 900/910/990 — but 834 (the last branch, by
   arbitrary band assignment) was ALSO banded starting at 900, so its own steps landed at 901-904,
   **above** `payload.builder`'s own sequence 900. The DAG executes in ascending sequence order, so
   `payload.builder` ran BEFORE 834's own `fhir.build` calls had executed — their `outputField`s
   genuinely didn't exist yet, resolving to `nil` (not even an empty array, unlike every other
   branch's already-completed, correctly-empty output) and failing the whole bundle assembly with "no
   resources resolved from resourcePaths". Fixed by moving the shared bottom group to 9000/9010/9090,
   safely clear of every branch band's own maximum.

### Verification
Zero new Go code — this is a pure `interface_templates` data/config addition
(`database/migrations/V257__EDI_Universal_To_FHIR_OOB_Pipeline_Template.sql`), generated
programmatically (not hand-retyped) by a scratch Node script that pulls the 9 branches' CURRENTLY LIVE
`pipeline_config` directly from the DB (the authoritative source — several have had multiple
superseding migrations, e.g. 837P: V237→V239→V244) rather than re-deriving from historical `.sql`
files. Full-stack: `tests/playwright/edi-universal-to-fhir-e2e.spec.js` drives the real "Use Template"
click path (44 canvas nodes: 3 shared top + 9 derive + 29 `fhir.build` + 3 shared bottom), then runs
"Test Pipeline" **9 separate times** against the SAME saved pipeline — once per already-committed
real/self-authored sample already used by each transaction set's own standalone e2e spec — asserting
for every run that the assembled Bundle contains **exactly** that branch's own expected resource types
and counts and **no other resource type at all** (the isolation proof that actually validates bugs #1
and #2's fixes, not just "the run succeeded"). All 9 runs pass.

### Named scope, deliberate
No synchronous endpoint — a sync call is inherently for one specific transaction set by definition, so
the existing per-type sync controllers (`sync-eligibility`, `sync-claim-status`, `sync-prior-auth`)
remain the right tool for that use case; this template is async/SFTP-only. A 999 (or any unrecognized)
file matches none of the 9 branches — every guard returns empty, so the assembled Bundle is spec-valid
but empty, matching the established "999→FHIR is a non-goal" precedent, not treated as an error.

### Key Files
- `database/migrations/V257__EDI_Universal_To_FHIR_OOB_Pipeline_Template.sql`
- `tests/playwright/edi-universal-to-fhir-e2e.spec.js`
- Read/reused unmodified (confirmed, not touched): `services/executors/transform/fhir_build_executor.go`

## NCPDP SCRIPT Engine — Phase 1 (Pharmacy E-Prescribing: NewRx, CancelRx, CancelRxResponse, RxChangeRequest, RxChangeResponse, September 2026)

### Why it exists
NCPDP SCRIPT (pharmacy e-prescribing) is a genuinely new clinical domain in this codebase — comparable
in scope to the entire EDI X12 build, and built the same incremental, schema-driven way. "NCPDP" is
actually **two unrelated standards under one brand**: SCRIPT (XML, prescriber↔pharmacy e-prescribing —
what this phase builds) and Telecommunication D.0 (control-character-delimited real-time pharmacy
**claims**, structurally like X12) — D.0 is explicitly out of scope, a separate future effort, not
attempted or even schema-stubbed here. NCPDP's own Implementation Guide is a paid document (the same
constraint EDI X12's TR3 was) — no free official schema exists, so this phase used the same
free-source-plus-cross-validation discipline EDI X12 already proved out: a complete, real, unedited
SCRIPT v2017071 NewRx sample (sourced from `dgoradia/ncpdp`'s own test fixtures) as primary ground
truth, cross-validated against `cosyte/ncpdp` (an actively-maintained, MIT-licensed open-source SCRIPT
v2017071/v2022011 parser) for CancelRx/CancelRxResponse/RxChangeRequest/RxChangeResponse structure.
Where neither source could confirm real structure (RxChangeRequest's own change-reason/type
sub-schema), the schema deliberately does **not** fabricate a field/tag name — it's a named, sourced
gap in the schema's own `sourceRefs`, not a guess.

### Architecture: simpler than both EDI X12 and CDA, not a hybrid of either
NCPDP SCRIPT XML (`<Message TransactionDomain="SCRIPT" ...><Header>...</Header><Body><NewRx>...
</NewRx></Body></Message>`) has **no RIM/classCode/moodCode/templateId model at all** (unlike CDA) and
**no trigger-segment/loop-matching ambiguity** (unlike EDI X12) — every element is uniquely named at
its own tree position, so the engine is a plain schema-driven tree walk, simpler than either precedent:
- **`xmlpath/`** (new top-level package, extracted from `cda/builder/xpath_writer.go` with zero logic
  changes) — `WriteAtXPath`/`TryFindAtXPath`/`SplitPathSegments`/`ReorderChildrenByTag`, the generic,
  already-validator-proven declarative XPath-driven XML tree builder over `etree` CDA's own builder
  had already de-risked. Confirmed to have zero CDA-specific vocabulary before extracting it — `cda/
  builder` now imports it instead of holding its own copy, verified via the existing CDA golden-
  snapshot + round-trip test suite (zero regressions) before any NCPDP-specific code was built on it.
- **`ncpdp/`** — `NCPDPGroupDef`/`NCPDPFieldDef`/`NCPDPGroupRef`/`NCPDPTransactionDef`/`NCPDPSpecDef`
  (`schema_types.go`), a fail-fast schema loader (`schema_loader.go`), and ONE generic recursive parser
  (`parser.go`'s `parseNode`, mirroring `edi.ParseTransactionSet`'s "one generic walk" discipline) — no
  per-transaction-type or per-group Go function anywhere; adding a 6th transaction type (a later phase)
  is a pure schema-data change. Groups (the segment-library equivalent — Name, Address,
  CommunicationNumbers, Quantity, DrugCoded, Patient, Prescriber, Pharmacy, MedicationPrescribed, ...)
  are authored once and referenced by composition, same discipline as EDI's segment library / CDA's
  `entries/*.json` templates.
- **`ncpdp/builder/document_builder.go`** — the write-direction mirror (`buildNode`), using the shared
  `xmlpath` package for every element/attribute write. Several NCPDP groups interleave plain fields and
  nested groups in the real XML (e.g. HumanPatient: Name, Gender, DateOfBirth, Address,
  CommunicationNumbers) in a way the schema's own separate Fields/Groups arrays can't express
  positionally — solved via `NCPDPGroupDef.ElementOrder` + `xmlpath.ReorderChildrenByTag` as a post-hoc
  pass, the exact same "construction order != schema order" problem and fix `cda/builder` already
  proved out for C-CDA.
- **`ncpdp/validator`** — a two-severity model mirroring EDI's own dated (2026-09-01) "flexible, not
  rigid" decision: a missing/empty REQUIRED field or group is a blocking error; the warning tier is an
  intentionally EMPTY, ready-to-extend mechanism for Phase 1 — no free source of NCPDP's own
  conditional-requirement business rules (the equivalent of X12's P/C/L/R/E `SyntaxRule`s) was found,
  so none are fabricated.

### Pipeline integration
`ncpdp.parse`/`ncpdp.validate`/`ncpdp.build`/`ncpdp.map_to_canonical`
(`services/executors/transform/ncpdp_*.go`), registered in `services/executor_registry.go`. `services/
parsers/ncpdpscript/ncpdp_script_parser_service.go` implements the codebase-wide `MessageParser`
interface, registered via `ParserFactory.RegisterNCPDPScriptParser` — `services/format_detector.go`
gained an `isNCPDPScript()` check (root `<Message` + `TransactionDomain="SCRIPT"`), inserted before the
generic `isXML` catch-all, same ordering discipline CCDA's own detection already established.
`controllers/ncpdp_schema_controller.go` (`GET /api/ncpdp/schema/groups`, `/transactions`,
`/transactions/:key/groups`) backs the pipeline UI's own no-code group/field pickers, mirroring `edi_
schema_controller.go` at the same small scale. **No dedicated connector** — NCPDP SCRIPT rides the SAME
generic transport connectors every other format uses (`sftp_inbound`/`sftp_outbound` for batch drop,
`http_outbound`/`http_rest_inbound` for direct HTTPS); real Surescripts network connectivity is
proprietary/partner-gated with no public API to build against, so a dedicated connector is explicitly
out of scope, the same reasoning already applied to why EDI X12 never got a Surescripts-specific
connector either. `ncpdp.map_to_canonical`'s no-code field mapper mirrors `edi.map_to_canonical`'s own
recursive group-tree UI shape (`headerFields`/`headerGroups`/`bodyFields`/`bodyGroups`, a dot-joined
groupKey path addressing arbitrarily nested groups) — adapted since NCPDP has TWO independent top-level
trees (Header, Body) rather than EDI's one (header + loops), and a group's `Repeatable` flag lives
directly on its own `NCPDPGroupRef` (schema-declared), not a separate method call the executor must
consult at runtime the way EDI's `X12LoopDef.RepeatsMultiple()` needs.

### UI built into Phase 1, not bolted on after
Learned directly from EDI X12's own Phase-1 mistake (shipped with zero UI, needed a whole separate
follow-up round): `public/js/pipeline/components/NCPDPStepBuilder.js` (4 step-builder classes,
`StepBuilderRegistry`-registered, `<script>` tag in `pipeline-builder.html`), `ToolboxManager.js` gained
4 new `StepTemplate` entries + icon-map entries, `Dockerfile` gained `COPY ncpdp/ ./ncpdp/` proactively
— the exact same "schema dir missing from the runtime image, only caught by a real container smoke
test" bug EDI X12 hit was pre-empted here rather than repeated.

### FHIR mapping — NewRx only this phase, proving the mechanism before extending it
NewRx → `MedicationRequest` + `Patient` + `Practitioner` (prescriber) + `Organization` (pharmacy) — the
same core resource set HL7's own NCPDP SCRIPT↔FHIR MedicationRequest mapping guidance names.
CancelRx/CancelRxResponse/RxChangeRequest/RxChangeResponse → FHIR mapping is a named, deferred
follow-on (matching 835's own "prove the pattern on one transaction type first" precedent).

**A real, deliberate design simplification, not an oversight**: unlike EVERY EDI X12→FHIR mapping in
this codebase, NewRx needs **no `enrichment.script` derive step and no `fhir.build` `rowsPath`** — a
NewRx message carries exactly one patient/prescriber/pharmacy/medication, no repeating claim/service-
line structure to flatten into row contexts first. Every `fhir.build` field sources straight from
`steps.parse_new_rx.step_output.parsed_ncpdp.{header,body}` paths — adding row-flattening machinery
here would have been exactly the kind of premature complexity this project's own standards call out.

**A real gap in NewRx's own schema, named not silently patched over**: as scoped this phase, NewRx
carries no patient-identifier field (no MRN/insurance-card-ID segment was modeled — that lives in
NCPDP's own COO/insurance section, deliberately deferred). `Patient.id` and every cross-resource
reference are therefore derived from the message's own `Header.messageID` — stable within one message,
but not a real patient identifier and not deduplicated across multiple NewRx messages for the same
real-world patient, the same "not deduplicated across claims" precedent EDI 837P/837I's own Patient/
Coverage mapping already established.

**`cda_gender_to_fhir` doesn't work for NCPDP's own gender field** — that transform (`services/
cda_fhir`'s `DeclarativeTransformRegistry`) expects a CDA-shaped `{"code": "..."}` object, not NCPDP's
own bare `"M"`/`"F"`/`"U"` string; `remarshalInto` would silently zero it out and the transform would
skip the write with no error. Fixed with three conditional literal fields (`condition: {field, operator:
"equals", value}`) mapping NCPDP's own gender vocabulary directly — the same condition-gated-literal
pattern EDI 837I's own `onAdmission` field already established, not a new mechanism.

### A real bug found ONLY by a genuine Test Pipeline run — a NEW instance of an already-documented
### gotcha class, but on a parse step, not an enrichment.script step
Every EDI 837P/278/etc. bug write-up in this file documents "an `enrichment.script` step's own returned
object is recursively snake-cased by `models.OutputNormalizer.NormalizeStepOutput` before a later step
can read it via `steps.<alias>.step_output`." This phase found the SAME mechanism firing on `ncpdp.
parse`'s own step_output — NOT a script step at all — because `NormalizeStepOutput` snake-cases
*every* step's snapshot uniformly, regardless of step type. `ncpdp.parse`'s `ParsedJSON` uses the ncpdp
schema's own deliberately-camelCase field.Key convention (`medicationPrescribed`, `humanPatient`,
`drugCoded`, `addressLine1`, ...) — reachable UNCHANGED via a direct `sourceField` read (proven by the
full Go-level executor-chain test, `services/executors/transform/ncpdp_executors_test.go`, which reads
`parsedNCPDP` directly with zero normalization in between) — but silently rewritten to
`medication_prescribed`/`human_patient`/`drug_coded`/`address_line1`/etc. the ONE TIME a LATER step
addresses it via `steps.parse_new_rx.step_output.parsedNCPDP...`. Every `fhir.build` field in the
OOB template uses exactly that addressing form, so every sourcePath-driven field on every one of the 4
resources resolved silently empty (`literalValue`-only fields still worked, masking the scope of the
bug until the resulting `MedicationRequest.subject` — the one base-FHIR-REQUIRED reference field among
several equally-broken ones — surfaced as a strict-validation error).

The Go-level FHIR-builder test (`services/ncpdp_newrx_fhir_builder_test.go`) originally passed cleanly
despite this, for the exact same reason CLAUDE.md's own EDI 837/835 write-ups warn about: its
`svcInjectStepOutput` helper injects a hand-built step_output map directly, bypassing the real
normalization call entirely — self-consistent (the test's own sourcePath strings matched what it
itself injected), but blind to whether the real engine would ever produce that exact shape. **Fixed
generically, not just patched**: `ncpdpRealNewRxData` now passes its injected step_output through the
REAL `models.NewOutputNormalizer().NormalizeStepOutput(...)` call before injecting it — instead of
hand-transcribing the snake_case shape a second time (a real, documented transcription-risk class in
this project's own history), the test now calls the SAME function the real engine calls, guaranteeing
it can never again silently drift from real behavior. Every `sourcePath`/`condition.field` string in
both the Go test and the migration was updated to the real snake_case form this produces (verified via
a real `curl` against the actual running app's Test Pipeline endpoint, not assumed).

A SECOND, independent instance of the already-documented `step_alias`-vs-`NormalizeKey(step_name)`
gotcha was also caught by the same real Test Pipeline run: the `payload.builder` step's own step_name
"Assemble New Rx FHIR Bundle" normalizes to `assemble_new_rx_fhir_bundle` (an underscore between "new"
and "rx", since NormalizeKey splits on the step name's own space-separated words) — the migration's
own `step_alias` field (and this phase's first Playwright test draft) used `assemble_newrx_fhir_bundle`
(no underscore) instead, a plausible-looking but wrong guess. This one was purely a *test-authoring*
risk (nothing in the pipeline's own config addresses this step via `steps.X` — only the Playwright
spec's own assertions needed the real key to inspect the assembled Bundle) — fixed by renaming the
alias to match the real key and correcting the spec's own reference.

### Real, working, browser-verified — not just Go-level
`tests/playwright/ncpdp-newrx-to-fhir-e2e.spec.js` drives the real "Use Template" click path (10 canvas
nodes, matching the 10 `execution_groups`) then a real "Test Pipeline" run against the real, unedited
NewRx sample through the complete HTTP/DAG-execution stack — asserting real field values (`Organization.
name`, `Patient.name`/`gender`/`birthDate`, `Practitioner.name`, `MedicationRequest.status`/`intent`/
`medicationCodeableConcept.text`/`dispenseRequest.quantity.value`) AND that `subject.reference`/
`requester.reference` are correctly rewritten to each referenced resource's own real bundle `fullUrl`
(payload.builder's documented cross-reference rewriting behavior, the same mechanism the EDI 835/837
templates already rely on and assert against) — not just "the run succeeded." This is the run that
caught both real bugs above; both are fixed and the spec passes clean, zero console errors.

### Named future phases (STATUS UPDATE — FHIR mapping for CancelRx/CancelRxResponse/RxChangeRequest/
### RxChangeResponse AND for RxRenewalRequest/RxRenewalResponse are now DONE, see the sections below;
### kept for history, not a live TODO list)
RxFill/RxFillIndicatorChange (dispense notifications → `MedicationDispense`), Status/Error/Verify/
GetMessage (control/acknowledgment transactions, analogous role to X12's 999), RxHistoryRequest/
RxHistoryResponse, RxTransferRequest/RxTransferResponse/RxTransferConfirm, REMS transactions, Census/
Resupply/DrugAdministration/Recertification, and NCPDP Telecommunication D.0 (pharmacy claims — a
separate, X12-shaped standard under the same NCPDP brand, explicitly not SCRIPT) all remain out of
scope, per the plan's own explicit deferral.

### Key Files
- Shared XML engine: `xmlpath/xpath_writer.go` (extracted from `cda/builder`, zero CDA-specific code)
- Engine: `ncpdp/schema_types.go`, `ncpdp/schema_loader.go`, `ncpdp/parser.go`, `ncpdp/util.go`
- Build direction: `ncpdp/builder/document_builder.go`
- Validator: `ncpdp/validator/validator.go`
- Schema data: `ncpdp/schemas/script_2017071/{manifest.json,groups/*.json,transactions/*.json}`
- Parser adapter: `services/parsers/ncpdpscript/ncpdp_script_parser_service.go`
- Pipeline steps: `services/executors/transform/ncpdp_{parse,validate,build,map_to_canonical}_executor.go`
- Schema API: `controllers/ncpdp_schema_controller.go`
- UI: `public/js/pipeline/components/NCPDPStepBuilder.js`, `ToolboxManager.js` (4 entries + icons)
- FHIR mapping (source of truth for the migration): `services/ncpdp_newrx_fhir_builder_test.go`
- Migration: `database/migrations/V258__NCPDP_NewRx_To_FHIR_OOB_Pipeline_Template.sql`
- Real sample: `ncpdp/testdata/real_samples/dgoradia_sample_newrx.xml` (dgoradia/ncpdp's own test
  fixture); self-authored lifecycle fixtures: `ncpdp/testdata/self_authored/*.xml`
- Tests: `ncpdp/newrx_roundtrip_test.go`, `ncpdp/lifecycle_roundtrip_test.go`, `ncpdp/schema_loader_test.go`,
  `ncpdp/validator/validator_test.go`, `services/executors/transform/ncpdp_executors_test.go`,
  `xmlpath/xpath_writer_test.go`
- Full browser click-path E2E: `tests/playwright/ncpdp-newrx-to-fhir-e2e.spec.js`

## NCPDP SCRIPT FHIR Mapping — CancelRx, CancelRxResponse, RxChangeRequest, RxChangeResponse (September 2026)

### Scope
Closes the FHIR-mapping gap Phase 1 named as deferred: the schema/engine support for these 4
transaction types was already built in Phase 1 (parse/build/validate all worked); only the
`fhir.build`/`payload.builder`/`fhir_validation` OOB templates were missing. Same "prove the pattern,
then extend it" discipline as EDI X12's own phased rollout — no new engine capability needed, purely
composing already-proven pipeline steps against schema paths that already existed.

### CancelRx and RxChangeRequest → the same 4-resource shape as NewRx
Both carry the identical body shape as NewRx (Patient/Pharmacy/Prescriber/MedicationPrescribed), so
both templates mirror V258's own Organization/Patient/Practitioner/MedicationRequest shape almost
field-for-field — confirmed by direct comparison, not assumed. The only real semantic differences are
on `MedicationRequest`:
- **CancelRx**: `status="cancelled"` (the prescriber's own cancellation intent — NCPDP/base FHIR have
  no "pending cancellation" state, a named simplification) + `identifier[0]` carrying the message's own
  `RequestReferenceNumber` so a later CancelRxResponse can be correlated back to it downstream.
- **RxChangeRequest**: `status="draft"` + `intent="proposal"` (both real base-FHIR enum values — a
  pharmacy's PROPOSED change is not yet prescriber-approved, unlike NewRx's `active`/`order` or
  CancelRx's `cancelled`) — RxChangeResponse's own outcome then represents the prescriber's actual
  decision.

### CancelRxResponse and RxChangeResponse → Task, not MedicationRequest
A real, deliberate design correction from the original plan draft, made by directly re-reading both
schema files rather than assuming: neither response transaction carries ANY Patient/Prescriber/
Pharmacy fields — only `RequestReferenceNumber` + one of 6 outcome choices (`Approved`/`Denied`/
`DenyNewToFollow`/`ApprovedWithChanges`/`Validated`/`Replace`, each with its own `ReasonCode`/
`ReferenceNumber`/`DenialReason`/`Note`) + an optional `MedicationPrescribed`. Base FHIR
`MedicationRequest.subject` is 1..1 required; fabricating a Patient reference these messages never
carry would violate this project's own no-invented-structure discipline. `Task.for` is 0..1 optional,
so `Task` can honestly represent "the response to a request, correlated by its own business
identifier" without inventing patient linkage — the same class of correction 837P/837I's own "no
separate Practitioner resource" design note already established for this codebase.

`Task.status` is derived from WHICHEVER of the 6 outcome groups is actually present on the message
(`Approved`/`ApprovedWithChanges`/`Validated`/`Replace` → `"completed"`; `Denied`/`DenyNewToFollow` →
`"rejected"`) via 12 stacked conditional field entries (2 per branch: status + businessStatus code) —
safe to stack unconditionally since exactly one of the 6 branches is ever present on a real message
(this schema doesn't structurally enforce mutual exclusion, matching the CDA choice-constraint scope
boundary precedent already established elsewhere in this codebase). `Task.note[0].text` sources
whichever branch's own `Note` field is actually present via `fallbackPaths` checked in order (`fhir.
build`'s own "first present value wins" convention). The one real difference between the two response
types: RxChangeResponse's own optional `MedicationPrescribed` represents real new prescribing content
(typically present alongside `ApprovedWithChanges`) — `Task.description` sources its `DrugDescription`
when present, falling back to a generic literal label, rather than fabricating a second, patient-less
`MedicationRequest` resource for it.

### Zero new gotchas this round — both previously-documented mechanisms applied proactively
Unlike NewRx's own Phase-1 round (which discovered both the `ncpdp.parse` step_output snake-casing
gotcha and the `step_alias`-vs-`NormalizeKey(step_name)` gotcha via a failing Playwright run), all 4
migrations here were authored with both fixes already applied up front — every `sourcePath`/
`condition.field` string uses the real snake_case form (`steps.<parse_step_key>.step_output.
parsed_ncpdp.body...`), and every `step_alias` was chosen to already equal its own step name's real
`NormalizeKey` output (verified by hand-tracing `toSnakeCase`'s character-by-character behavior before
writing the SQL, not just copy-pasting a plausible guess). All 4 Playwright specs passed on the first
real Test Pipeline run, with zero fix-and-rerun cycles — direct evidence the two Phase-1 gotchas are
now genuinely internalized as a pre-flight check for this pattern, not just documented after the fact.

### Verification
Go: `services/ncpdp_cancelrx_fhir_builder_test.go`, `services/ncpdp_cancelrx_response_fhir_builder_test.go`,
`services/ncpdp_rxchangerequest_fhir_builder_test.go`, `services/ncpdp_rxchangeresponse_fhir_builder_test.go`
— each chains the real executors (`fhir.build` ×N → `payload.builder` → `fhir_validation` strict)
against the self-authored lifecycle fixtures, using the shared `assembleAndValidateNCPDPBundle` helper
(a parameterized generalization of NewRx's own `assembleAndValidateNewRxBundle`). All 4 migrations'
field entries were diffed programmatically against their own Go test source (reusing `validate_and_diff.py`,
a generalized version of the one-off V258 diff script) before being applied — zero drift confirmed for
all 4.

Full-stack: real `docker-compose build app` (clean, `ncpdp/` confirmed present in the runtime image),
`docker-compose run --rm flyway migrate` applied V261/V262 cleanly (V259/V260 were already live from an
earlier point in this session); all 5 NCPDP templates (NewRx + these 4) confirmed live via `psql`.
`tests/playwright/ncpdp-{cancelrx,cancelrxresponse,rxchangerequest,rxchangeresponse}-to-fhir-e2e.spec.js`
each drive the real "Use Template" click path (10 canvas nodes for the 2 Bundle-shaped templates, 7 for
the 2 Task-shaped ones) then a real "Test Pipeline" run against their own self-authored fixture,
asserting real field values and — for the 2 MedicationRequest templates — that `subject.reference`/
`requester.reference` are correctly rewritten to each referenced resource's own real bundle `fullUrl`.
All 4 pass clean on the first run, zero console errors. Throwaway test interfaces deactivated +
soft-deleted via the real API after verification.

### Key Files
- FHIR mapping (source of truth for each migration): `services/ncpdp_cancelrx_fhir_builder_test.go`,
  `services/ncpdp_cancelrx_response_fhir_builder_test.go`, `services/ncpdp_rxchangerequest_fhir_builder_test.go`,
  `services/ncpdp_rxchangeresponse_fhir_builder_test.go`
- Migrations: `database/migrations/V259__NCPDP_CancelRx_To_FHIR_OOB_Pipeline_Template.sql`,
  `V260__NCPDP_CancelRxResponse_To_FHIR_OOB_Pipeline_Template.sql`,
  `V261__NCPDP_RxChangeRequest_To_FHIR_OOB_Pipeline_Template.sql`,
  `V262__NCPDP_RxChangeResponse_To_FHIR_OOB_Pipeline_Template.sql`
- Full browser click-path E2E: `tests/playwright/ncpdp-cancelrx-to-fhir-e2e.spec.js`,
  `ncpdp-cancelrxresponse-to-fhir-e2e.spec.js`, `ncpdp-rxchangerequest-to-fhir-e2e.spec.js`,
  `ncpdp-rxchangeresponse-to-fhir-e2e.spec.js`

## NCPDP SCRIPT FHIR Mapping — RxRenewalRequest, RxRenewalResponse (September 2026)

### Scope and a real structural discovery, cross-validated before writing any schema
Closes another of Phase 1's own named-deferred items (refills). Direct investigation of `cosyte/
ncpdp`'s `src/script/lifecycle.ts` (the same cross-validation source Phase 1 used for CancelRx/
RxChangeRequest/RxChangeResponse) confirmed, before any schema file was written, that
`RxRenewalRequest` extends the library's own `LifecycleRequestFields` (`requestReferenceNumber?`,
`patient?`, `pharmacy?`, `prescriber?`, `medicationPrescribed?`) with **zero additional fields of its
own** — structurally identical to `CancelRx`/`RxChangeRequest` — and `RxRenewalResponse` extends
`LifecycleResponseFields` with the SAME 6 outcome elements (`Approved`/`Denied`/`DenyNewToFollow`/
`ApprovedWithChanges`/`Validated`/`Replace`) in the SAME fail-safe denial-first precedence order,
identical to `CancelRxResponse`/`RxChangeResponse`. This is a genuine, sourced confirmation (not an
assumption carried over from the other lifecycle types) — `lifecycle.ts`'s own type union
(`LifecycleRequestKind`/`LifecycleResponseKind`) explicitly groups `RxRenewalRequest` alongside
`RxChangeRequest`/`CancelRx` as one shared shape.

### Schema addition — pure JSON data, zero new Go code
`ncpdp/schemas/script_2017071/transactions/RxRenewalRequest.json` and `RxRenewalResponse.json`,
registered in `manifest.json` — each a near-verbatim copy of `CancelRx.json`/`CancelRxResponse.json`
with only the transaction key/name changed, confirming the schema-driven "adding a transaction type is
a pure data change" promise from Phase 1's own design. Verified directly: `ncpdp/parser.go`'s
`ParseMessage` dispatches via `spec.Transactions[txType]` (a map lookup keyed off the manifest) and
`ncpdp/builder/document_builder.go` takes `transactionType` as a plain string parameter — grepped both
files plus `ncpdp/validator/validator.go` for any hardcoded transaction-type name before writing a
single line of schema; found none.

### FHIR mapping — same shapes as CancelRx/CancelRxResponse, one semantic difference each
`RxRenewalRequest` → `MedicationRequest` (+ Organization/Patient/Practitioner), mirroring V259's own
shape, with `status="active"`/`intent="order"` — a renewal request represents the pharmacy asking to
CONTINUE an existing, currently-active prescription (unlike `RxChangeRequest`'s own "draft"/"proposal"
pharmacy-PROPOSED alteration, there is no changed content here to mark as unapproved).
`RxRenewalResponse` → `Task` (same reasoning as `CancelRxResponse`/`RxChangeResponse` — no Patient/
Prescriber data on a response message, so `MedicationRequest.subject` can't be honestly populated),
with `Task.description` sourcing `medicationPrescribed.drugDescription` when present (like
`RxChangeResponse`, since an `ApprovedWithChanges` renewal outcome can carry real revised prescribing
content) falling back to a generic literal label.

### Zero new gotchas — both Phase-1 mechanisms applied proactively, again
Same as the CancelRx/RxChangeRequest/RxChangeResponse mapping round above: every `sourcePath`/
`condition.field` used the real snake_case form and every `step_alias` was pre-verified against its
own step name's `NormalizeKey` output before being written into the migration. Both Playwright specs
passed on the first real Test Pipeline run.

### Verification
Go: 2 new round-trip tests in `ncpdp/lifecycle_roundtrip_test.go` (`TestRxRenewalRequest_
SelfAuthoredSample_ParseAndRoundTrip`, `TestRxRenewalResponse_SelfAuthoredSample_ApprovedOutcome_
ParseAndRoundTrip`) plus `services/ncpdp_rxrenewalrequest_fhir_builder_test.go`/
`ncpdp_rxrenewalresponse_fhir_builder_test.go` (chaining the real executors, reusing the shared
`assembleAndValidateNCPDPBundle` helper) — all pass, run via a throwaway `docker build --target
gobuilder` image (schemas/ and tests/ bind-mounted, image removed immediately after each use). Full
`ncpdp`/`ncpdp/validator`/`services/executors/transform` (NCPDP subset) suites re-run with zero
regressions.

Full-stack: real `docker-compose build app` (clean, both new schema files baked into the image) +
`docker-compose up -d app` (which auto-ran Flyway as part of its own dependency chain, applying V263/
V264 without a separate manual step this time), all 7 NCPDP templates confirmed live via `psql` and
via `GET /api/ncpdp/schema/transactions`. `tests/playwright/ncpdp-rxrenewalrequest-to-fhir-e2e.spec.js`/
`ncpdp-rxrenewalresponse-to-fhir-e2e.spec.js` each drive the real "Use Template" click path then a real
"Test Pipeline" run against their own self-authored fixture — both pass clean on the first run, zero
console errors. Throwaway test interfaces deactivated + soft-deleted via the real API after
verification.

### Key Files
- Schema data: `ncpdp/schemas/script_2017071/transactions/RxRenewalRequest.json`, `RxRenewalResponse.json`
- Self-authored fixtures: `ncpdp/testdata/self_authored/rx_renewal_request_sample.xml`,
  `rx_renewal_response_approved_sample.xml`
- FHIR mapping (source of truth for each migration): `services/ncpdp_rxrenewalrequest_fhir_builder_test.go`,
  `services/ncpdp_rxrenewalresponse_fhir_builder_test.go`
- Migrations: `database/migrations/V263__NCPDP_RxRenewalRequest_To_FHIR_OOB_Pipeline_Template.sql`,
  `V264__NCPDP_RxRenewalResponse_To_FHIR_OOB_Pipeline_Template.sql`
- Full browser click-path E2E: `tests/playwright/ncpdp-rxrenewalrequest-to-fhir-e2e.spec.js`,
  `ncpdp-rxrenewalresponse-to-fhir-e2e.spec.js`

## NCPDP SCRIPT FHIR Mapping — RxFill (Fill/Dispense Notification, September 2026)

### A real sourcing gap found and worked around honestly
Closes another Phase-1-named-deferred item. Direct investigation found `cosyte/ncpdp` (the
cross-validation source for every lifecycle transaction so far) does **not** actually model RxFill
structurally at all — `RxFill`/`RxFillIndicatorChange`/`RxHistoryRequest`/`RxTransfer*`/etc. only
appear in `versions.ts`'s own transaction-NAME registry (for version-negotiation), never parsed. Rather
than fabricate structure with no source, a broader search found a genuinely usable one: `usnistgov/
tcamt-2`'s `tcamt-lite-controller/SCRIPT_XML_10_6.xsd` — a real, NIST-hosted, official-provenance
NCPDP SCRIPT XSD, for the OLDER SCRIPT v10.6 wire format (not this engine's own v2017071 target). Used
the same way `cosyte/ncpdp` was used for the lifecycle transactions: a real, sourced, cross-version
structural reference, not an invention — every schema file's own `sourceRefs` names this trade-off
explicitly. `RxFillIndicatorChange` itself has **no confirmed structural source at all** (absent from
both `cosyte/ncpdp` and the 10.6 XSD) — left as a named, still-deferred gap rather than guessed.

### Schema — new group + transaction, FillStatus modeled as a 3-way choice reusing OutcomeReason
`ncpdp/schemas/script_2017071/groups/MedicationDispensed.json` (new — DrugDescription/DrugCoded/
Quantity/WrittenDate/LastFillDate; the source XSD's own `RxFillDispensedMedicationType` also carries
DaysSupply/Directions/Refills/Substitutions/Diagnosis/PriorAuthorization/StructuredSIG/etc., deliberately
deferred — core fields, not exhaustive, matching EDI 835/837's own established precedent) and
`transactions/RxFill.json` (new — `FillStatus`'s 3-way choice, per the XSD's own `FillStatusType`, is
modeled as 3 sibling group refs — `filled`/`notFilled`/`partialFill` — REUSING the already-built
`OutcomeReason` group (ReasonCode/ReferenceNumber/DenialReason/Note) rather than authoring a
structurally-identical new group, since `FillStatusType`'s own `NoteType`/`DeniedFillType` sub-shapes are
a near-exact match). Pharmacy/Prescriber/Patient reuse this engine's own existing NewRx-era groups
rather than the XSD's own RxFill-specific `MandatoryAddressPharmacyType`/`PrescriberRxFillType` — a
named, deliberate simplification trading some RxFill-specific structural detail (ClinicName,
PrescriberAgent, a flatter Identification shape) for consistency with every other already-built
transaction type. `Request`/`Supervisor`/`Facility` (all optional in the source XSD) are not modeled —
named, deferred gaps.

### FHIR mapping — MedicationDispense, confirmed against the real cardinality schema, not assumed
`RxFill` → `MedicationDispense` + `Organization` (pharmacy) + `Patient` — **no separate Practitioner
resource**, since RxFill's own schema (both the 10.6 XSD and this engine's own simplified reuse) carries
no individual dispensing-pharmacist name, only the pharmacy itself. Verified directly against
`schemas/fhir/R4/resources/MedicationDispense.gz`'s own `required` list (not assumed from memory) that
only `status` + `medication[x]` are UNCONDITIONALLY required — `performer.actor` and
`substitution.wasSubstituted` are conditional on those substructures being present at all — so a
Patient/Organization-only Bundle is fully spec-conformant. `FillStatus`'s 3-way choice maps onto FHIR's
own `medicationdispense-status` ValueSet: `Filled` → `"completed"`, `PartialFill` → `"in-progress"` (a
partial fill is, by definition, not yet complete), `NotFilled` → `"declined"` — a code-system
translation, never an invented fact.

### Verification
Go: `TestRxFill_SelfAuthoredSample_FilledOutcome_ParseAndRoundTrip` (`ncpdp/lifecycle_roundtrip_test.go`)
and `services/ncpdp_rxfill_fhir_builder_test.go` (chaining the real executors, reusing the shared
`assembleAndValidateNCPDPBundle` helper) — both pass, run via a throwaway `docker build --target
gobuilder` image (removed immediately after each use, rebuilt once mid-round after a test file was added
after the image's first build — a real, avoidable timing mistake worth naming: always finish writing
every test file BEFORE starting the throwaway image build, not during). Full `ncpdp`/`ncpdp/validator`/
`services/executors/transform` (NCPDP subset) suites re-run with zero regressions.

Full-stack: real `docker-compose build app` (clean, `MedicationDispensed.json`/`RxFill.json` baked into
the image) + `docker-compose up -d app` (auto-ran Flyway, applying V265 without a separate manual step),
confirmed live via `psql` and `GET /api/ncpdp/schema/transactions` (8 transaction types now). `tests/
playwright/ncpdp-rxfill-to-fhir-e2e.spec.js` drives the real "Use Template" click path (9 canvas nodes)
then a real "Test Pipeline" run — passes clean on the first run, zero console errors, including the
`performer[0].actor.reference`/`subject.reference` fullUrl-rewrite assertions (the same `payload.
builder` cross-reference mechanism proven for every other NCPDP template). Throwaway test interface
deactivated + soft-deleted via the real API after verification.

### Key Files
- Schema data: `ncpdp/schemas/script_2017071/groups/MedicationDispensed.json`,
  `transactions/RxFill.json`
- Self-authored fixture: `ncpdp/testdata/self_authored/rx_fill_filled_sample.xml`
- FHIR mapping (source of truth for the migration): `services/ncpdp_rxfill_fhir_builder_test.go`
- Migration: `database/migrations/V265__NCPDP_RxFill_To_FHIR_OOB_Pipeline_Template.sql`
- Full browser click-path E2E: `tests/playwright/ncpdp-rxfill-to-fhir-e2e.spec.js`

### Named future phases (still deferred)
`RxFillIndicatorChange` (no confirmed structural source found), `RxHistoryRequest`/`RxHistoryResponse`,
`RxTransferRequest`/`RxTransferResponse`/`RxTransferConfirm`, REMS transactions, Census/Resupply/
DrugAdministration/Recertification, and NCPDP Telecommunication D.0 all remain out of scope this round.
`Status`/`Error`/`Verify`/`GetMessage` are DONE — see the section immediately below.

## NCPDP SCRIPT Engine — Status, Error, Verify, GetMessage (Control/Acknowledgment Transactions, September 2026)

### Scope — engine support only, no FHIR mapping, matching X12 999's own precedent
Closes the last of Phase 1's originally-named-deferred items with a real, confirmed source. `cosyte/
ncpdp`'s `src/script/response.ts` models `Status`/`Error`/`Verify` identically — all three share ONE
`ResponseFields` shape (`code?`, `descriptionCode?`, `description?`) via `extractResponse()`'s own
generic `childText(el, "Code"|"DescriptionCode"|"Description")` reads — cross-confirmed against
`usnistgov/tcamt-2`'s `SCRIPT_XML_10_6.xsd` (the same NIST-hosted XSD sourced for RxFill), whose own
`<Status>`/`<Error>` elements agree exactly on `Code`/`DescriptionCode`/`Description`. `GetMessage`
(the SCRIPT mailbox transport's own message-pull trigger) is confirmed, not assumed, to carry NO real
structured content of its own — the XSD's own `<GetMessage>` wraps a bare `xs:anyType` placeholder.

**A real source disagreement, resolved by preferring the version-matched source**: the 10.6 XSD's own
`<Verify>` element is materially richer (a nested `VerifyStatus`/`Code` + optional `Pharmacy`/
`Prescriber`, apparently for prescriber identity/credential verification) than `cosyte/ncpdp`'s own flat
`ResponseFields` model. Resolved in favor of `cosyte/ncpdp` — it targets THIS engine's own v2017071/
v2022011 versions directly, while the XSD is an admittedly older v10.6 cross-reference — rather than
guessing which source is "more right" in the abstract; `Verify.json`'s own `sourceRefs` names this
disagreement and the resolution explicitly.

No FHIR mapping for any of the 4 — matching this codebase's own established, user-confirmed EDI X12 999
precedent (`CLAUDE.md`'s own "999 → FHIR remains a deliberate non-goal — a transport-layer
acknowledgment has no natural FHIR analogue"). The identical reasoning applies here: `Status`/`Error`
are positive/negative acknowledgments of a prior transaction, `Verify` a credential check, `GetMessage`
a transport-layer mailbox poll — none carry clinical content a FHIR resource could honestly represent.

### Schema — 4 new, structurally trivial transaction files, zero new groups
`transactions/{Status,Error,Verify,GetMessage}.json` — `Status`/`Error`/`Verify` are each 3 flat
top-level fields (no nested groups needed, unlike every prior transaction type this engine has built);
`GetMessage` has zero fields/groups at all (an honestly-empty transaction body, confirmed by the source
XSD's own degenerate shape, not a placeholder for something unmodeled). `Status.code`/`Error.code` are
modeled `required: true` (both the XSD's own pattern-restricted `Code` and `cosyte/ncpdp`'s own
missing-Code warning agree it's the load-bearing field); `Verify.code` is `required: false` (neither
source treats a missing Verify code as a hard requirement).

**A real generic-engine correctness check performed before writing GetMessage's schema**: confirmed
directly in `ncpdp/parser.go`/`ncpdp/builder/document_builder.go` that a transaction with empty
`Fields`/`Groups`/`ElementOrder` is a fully safe, already-correct no-op path (`parseNode`'s two loops
simply don't execute, `buildNode`'s `len(elementOrder) > 0` guard skips the reorder call) — no new
engine code needed for the empty-transaction case, it already worked by construction.

### Verification
Go: 4 new round-trip tests in `ncpdp/lifecycle_roundtrip_test.go` (`TestStatus_SelfAuthoredSample_
ParseAndRoundTrip`, `TestError_...`, `TestVerify_...`, `TestGetMessage_...`) against 4 new self-authored
fixtures — all pass, run via a throwaway `docker build --target gobuilder` image (removed immediately
after use). Full `ncpdp`/`ncpdp/validator`/`services/executors/transform` suites re-run with zero
regressions (the executor suite's own 467s runtime confirmed clean, not just the NCPDP-scoped subset).

Full-stack: real `docker-compose build app` (clean, all 4 new schema files baked into the image) +
`docker-compose up -d app` + a real `GET /api/ncpdp/schema/transactions` poll confirming **all 12**
NCPDP SCRIPT transaction types now resolve correctly through the live running app (`NewRx`, `CancelRx`,
`CancelRxResponse`, `RxChangeRequest`, `RxChangeResponse`, `RxRenewalRequest`, `RxRenewalResponse`,
`RxFill`, `Status`, `Error`, `Verify`, `GetMessage`) — no Playwright spec, since there is no OOB
pipeline template to click through (no FHIR mapping means no `fhir.build`/`payload.builder` chain to
assemble into a template, matching 999's own precedent of engine-only, template-less support).

### Key Files
- Schema data: `ncpdp/schemas/script_2017071/transactions/{Status,Error,Verify,GetMessage}.json`
- Self-authored fixtures: `ncpdp/testdata/self_authored/{status,error,verify,get_message}_sample.xml`
- Tests: `ncpdp/lifecycle_roundtrip_test.go` (4 new tests)

## NCPDP Telecommunication D.0 Engine — Phase 1 (B1 Claim Billing, September 2026)

### Why it exists
"NCPDP" is actually **two unrelated standards under one brand**: SCRIPT (XML, prescriber↔pharmacy
e-prescribing — built above) and Telecommunication D.0 (control-character-delimited real-time pharmacy
**claims**, structurally like X12) — explicitly named and deferred throughout the SCRIPT work, then
picked up as its own phase per direct user instruction. This phase builds the whole engine pattern
end-to-end on the single highest-value transaction — **B1 (Claim Billing)**, request and response —
the same incremental "prove it on one transaction type first" discipline already used for EDI X12's
835 and NCPDP SCRIPT's NewRx.

### Sourcing discipline (no free official IG, same constraint SCRIPT/EDI X12 TR3 had)
NCPDP's own Telecommunication Standard Implementation Guide is a paid document. Two free, structurally
authoritative sources were read directly (not assumed from training data): **`eduardonunesp/ncpdp-telecom-fmt-book`**
(a structural walkthrough — separators, headers, segments, fields, data types including signed
overpunch, transactions, responses) and **`apiv/dzero`** (a real, "battle-tested at Instacart" Ruby D.0
parser/serializer with a complete field dictionary for all 26 segment types, plus a real fixture pair
demonstrating the GS repeating-group mechanism byte-for-byte). Both show `license: null` on GitHub —
matching this project's own standing discipline (the NIST SCRIPT-10.6-XSD / EDI PyX12+Stedi precedent):
structural facts (field IDs, segment IDs, required-vs-optional shape) are used as sourcing knowledge,
no file/fixture/code from either repo is vendored. Every schema file records its own `sourceRefs`. All
test fixtures are self-authored from the confirmed field shapes — no free, real-world D.0 sample exists
anywhere in this session's own search, unlike NCPDP SCRIPT's NewRx (which had a real `dgoradia/ncpdp`
fixture) — a named, permanent limitation of this phase, not an oversight.

### Confirmed wire format
Control-character-delimited, genuinely different from both SCRIPT (XML) and EDI X12 (printable
delimiters): `0x1C` FS (precedes a 2-char field ID + value), `0x1E` RS (starts a segment), `0x1D` GS
(delimits repeated clusters of segments within one transmission). Fixed 56-byte header (no separators):
`binNumber(6) + version(2) + transactionCode(2) + processorControlNumber(10) + transactionCount(1) +
serviceProviderIdQualifier(2) + serviceProviderId(15) + dateOfService(8) +
softwareVendorCertificationId(10)`. The body is a **transmission group** (segments appearing once —
Patient, Insurance, Pharmacy Provider, Prescriber) plus zero-or-more **transaction groups** (GS-delimited
clusters — Claim, Pricing — one cluster per claim/drug line item), confirmed byte-for-byte from
`dzero`'s own real fixture. **Signed overpunch** is the one genuinely novel data type: the last digit of
a decimal is replaced by a letter encoding both the digit and the sign (`{ABCDEFGHI` = positive 0–9,
`}JKLMNOPQR` = negative 0–9) — e.g. `0000084F` → 8.46 (F=6, positive).

**Request and response share the identical wire transaction code** (`B1` appears in both a claim
request and its own adjudication response) — disambiguated only by disjoint segment-identifier ranges
(01–16 request, 20–29 response), a real structural fact confirmed from source, not guessed. `SniffDirection`
implements this; an explicit `direction` config always wins when the caller already knows it.

### Architecture — a new top-level package `ncpdptelecom/`, mirroring `edi/`'s layering (not `ncpdp/`'s)
Chosen over extending `ncpdp/` because the wire shape is fundamentally X12-like (fixed delimiters +
segment/field codes), not XML:
- **`ncpdptelecom/datatypes.go`** — `TelecomDataType` (string/date/integer/decimal/overpunch),
  `EncodeOverpunch`/`DecodeOverpunch` (the one genuinely new codec in this whole phase, directly unit-
  tested both directions against the doc's own worked examples), `Validate`/`Parse`/`Format`.
- **`ncpdptelecom/schema_types.go`** — `TelecomFieldDef`/`TelecomSegmentDef`/`TelecomTransactionDef`/
  `TelecomSpecDef`, `TransactionKey(code, direction)` helper, `SegmentByIdentifier` (dispatches a raw
  segment to its schema def by its own 2-char wire identifier).
- **`ncpdptelecom/schema_loader.go`** — manifest + `segments/*.json` + `transactions/*.json`, fail-fast
  cross-reference validation, mirroring `edi/schema_loader.go`'s conventions.
- **`ncpdptelecom/parser.go`** — `ParseTransmission(spec, direction, raw) (*ParseResult, error)`: reads
  the fixed header, splits on RS into raw segments, resolves each by its leading `AM` field, buckets
  into the transmission group (once) or the current transaction-group cluster (a GS byte starts a new
  cluster) — ONE generic walk, no per-segment/per-transaction-code Go function.
- **`ncpdptelecom/builder/document_builder.go`** — `BuildTransmission`, the write-direction mirror.
- **`ncpdptelecom/validator/validator.go`** — two-severity model (missing/malformed required
  field/segment = blocking error; everything else = non-blocking warning tier, intentionally empty for
  Phase 1 — no free source of D.0's own conditional-requirement business rules was found, matching
  EDI's dated 2026-09-01 "flexible, not rigid" precedent and NCPDP SCRIPT's own identical decision).

### Pipeline integration, connectors, sync endpoint — reusing every proven mechanism
- **Format detection**: `services/format_detector.go`'s `isNCPDPTelecom()` (header bytes 6:8 == "D0"
  AND the raw content contains the `0x1E` RS byte), inserted before the generic CSV catch-all. New
  `models.FormatNCPDPTelecom` constant.
- **Parser adapter**: `services/parsers/ncpdptelecom/ncpdp_telecom_parser_service.go` implements the
  existing `MessageParser` interface, `Parse()` uses `SniffDirection` for auto-detection; registered via
  `ParserFactory.RegisterNCPDPTelecomParser(schemaDir)`.
- **Pipeline steps**: `ncpdptelecom.parse`/`ncpdptelecom.validate`/`ncpdptelecom.build`/
  `ncpdptelecom.map_to_canonical` (`services/executors/transform/ncpdptelecom_*.go`), the same 4-step
  shape `edi.*`/`ncpdp.*` already established, all shipped in this one phase (learned directly from EDI
  X12 Phase 1's own "shipped with zero UI, needed a whole separate follow-up round" mistake — pre-empted
  here, not repeated a third time).
- **No dedicated connector** — D.0 rides the same generic SFTP/HTTP transport connectors every other
  format uses; real D.0 network connectivity is switch/VAN-proprietary with no public API, the same
  reasoning already applied to why neither SCRIPT nor EDI X12 got a format-specific connector.
- **A genuinely new synchronous endpoint, shipped in Phase 1 (not added later on request, unlike EDI's
  own 270/276/278)**: `controllers/sync_pharmacy_claim_controller.go` (`POST
  /api/pharmacy-claim/:interfaceId/submit`), a near-verbatim structural copy of
  `sync_claim_status_controller.go` — resolves the interface's own `"B1_REQUEST"` pipeline, calls
  `TransformationPipelineService.ExecutePipeline` directly from the HTTP handler, extracts the resolved
  build step's output via the shared `extractFirstBuildStepPayload`/`buildStepContentFields` mechanism (a
  new `"ncpdptelecom.build"` entry added to that map, `field: "ncpdp_telecom"` — camelCase
  `outputField: "ncpdpTelecom"` after `NormalizeStepOutput`'s snake-casing —
  `contentType: "application/x-ncpdp-telecom-d0"`). D.0's entire reason for existing is real-time
  point-of-sale adjudication — a synchronous request/response is the PRIMARY real-world delivery mode
  for this specific standard, not an afterthought, so this shipped in Phase 1 deliberately.
- **Schema browser API**: `controllers/ncpdp_telecom_schema_controller.go` (`GET
  /api/ncpdp-telecom/schema/segments`, `/transactions`), mirroring `ncpdp_schema_controller.go`/
  `edi_schema_controller.go` at the same small scale, wired into `main.go` + proxied via `app.js`.
- **UI**: `public/js/pipeline/components/NCPDPTelecomStepBuilder.js` (4 step-builder classes,
  `StepBuilderRegistry`-registered), `<script>` tag in `pipeline-builder.html`, `ToolboxManager.js` gains
  4 new `StepTemplate` entries + icon-map entries — built into this phase, not deferred. D.0's own
  `map_to_canonical` config is FLATTER than SCRIPT's/EDI's own recursive group/loop trees (no nested
  sub-groups at all — a flat field list per segment, and exactly ONE repeating construct, transaction
  groups, addressed by a single `transactionGroupRowsPath` rather than a per-node `rowsPath` at
  arbitrary depth) — the UI reflects that simpler shape directly rather than reusing the recursive
  tree-walking UI code EDI/SCRIPT's own map-to-canonical builders need.
- **Dockerfile**: `COPY ncpdptelecom/ ./ncpdptelecom/` added proactively — the exact EDI Phase-1
  deployment-bug class (a schema dir missing from the runtime image, only caught by a container smoke
  test) pre-empted here rather than repeated a third time.

### FHIR mapping — B1 request → Organization + Patient + Claim; B1 response → ClaimResponse
Confirmed via HL7's own real precedent that `Claim`/`ClaimResponse` (not a custom resource) are the
correct FHIR targets for a real-time pharmacy claim/adjudication exchange, `type=pharmacy` matching base
FHIR's own `Claim.type` ValueSet. No `PaymentReconciliation` this phase — that resource models 835-style
*aggregate* remittance across many claims, not a single real-time adjudication response; `ClaimResponse`
alone is the correct, sufficient target.

**A real mechanism gap found by reading the code, not assumed** (same class EDI 835's own per-claim EOB
mapping already found): `fhir.build`'s `rowsPath` mode resolves fields ROW-ONLY (confirmed directly in
`fhir_build_executor.go`) — a claim row (one GS-delimited transaction group) has no reachable path back
up to the transmission header. A small `enrichment.script` derive step ("Derive B1 Claim Context")
copies the one header-level value every claim row needs (`dateOfService`) down onto each row before
`fhir.build` ever sees it — not avoidable even though D.0's own loop nesting is far shallower than EDI
837's (no subscriber/dependent hierarchy, no diagnosis/procedure/care-team lists).

Organization (pharmacy) and Patient use **fixed literal ids** (`organization-pharmacy-1`/`patient-1`)
rather than deriving them from per-row data — a deliberate simplification since B1 carries exactly ONE
pharmacy and ONE patient per transmission (unlike 837's own subscriber/dependent multiplicity), so
uniqueness WITHIN one message's own Bundle is all that matters; `Claim.patient.reference`/
`Claim.provider.reference` are therefore plain literal values (`Patient/patient-1`), not sourced from
any row data — `payload.builder` still correctly rewrites them to each resource's own real `fullUrl`.

**ClaimResponse's own `patient` field uses a DISPLAY-ONLY Reference** (no `.reference` pointer) — a B1
response carries NO patient/pharmacy identity data at all (confirmed directly against the schema:
ResponseMessage/ResponseStatus/ResponseClaim/ResponsePricing carry none), the same real gap that led
NCPDP SCRIPT's own CancelRxResponse/RxChangeResponse to use `Task` instead of `MedicationRequest`. Since
the approved plan named `ClaimResponse` as this format's own target (a real, adjudication-bearing
resource `Task` can't represent as honestly), the display-only Reference satisfies FHIR's structural
requirement without fabricating a resource or writing a dangling pointer — the same logical-reference
pattern already used for `Claim.careTeam[].provider` in the EDI 837 work.

**HCR/AN status code translation** (a code-system translation, never an invented fact, sourced directly
from the confirmed response vocabulary): `A`/`C`/`P` (Approved/Captured/Paid) → `outcome: "complete"`;
`D`/`E`/`R` (Duplicate/Error/Rejected) → `outcome: "error"`.

### Real bugs found and fixed this phase
1. **CRITICAL, engine-level: goja can't iterate a concretely-typed Go slice.** `ncpdptelecom.ParseResult.TransactionGroups`
   was initially typed `[]map[string]interface{}` — goja's reflection-based Go-value wrapping does NOT
   expose this as a normal iterable JS array (a `for` loop over it in a JS derive script silently sees
   zero elements, no error anywhere). Every other engine in this codebase (`ncpdp`, `edi`) already used
   `[]interface{}` for exactly this reason; this was the one place that hadn't, until caught by writing
   a debug test that dumped the ACTUAL structure via `json.MarshalIndent` (proving the DATA was fine,
   the CONSUMPTION mechanism was the problem). Fixed by changing the type to `[]interface{}` everywhere
   across the package and its executors, plus ~12 test call sites (a new `groupAt()` helper in
   `roundtrip_test.go`).
2. **`enrichment.script`'s own calling convention: `function transform(input) {...}` is NEVER invoked.**
   The executor runs script text UNWRAPPED first as a top-level program — a bare function DECLARATION's
   own completion value is `undefined` (declarations don't produce a runtime value); since nothing calls
   `transform(input)`, the result silently falls through to an empty pre-injected `output` object with
   ZERO error at any layer. The correct pattern (confirmed directly from `pas_fhir_builder_test.go`'s own
   working `derivePASFieldsScript`): bare top-level statements ending in a trailing PARENTHESIZED
   OBJECT-LITERAL EXPRESSION (`({ claim_rows: claim_rows });`), never a function wrapper. Found via a
   JS-level debug script dumping `Object.keys()` at each nesting level, not static reasoning alone.
3. **`ncpdptelecom.map_to_canonical` never populated the wire header at all** — no `headerFields` config
   option existed, so `ncpdptelecom.build` wrote BLANK transaction-code bytes, unparseable even when
   every segment's own content was mapped correctly. Fixed by adding a `HeaderFields` config option, with
   `transactionCode`/`version` auto-defaulting from the step's own config/loaded schema when not
   explicitly mapped — caught by the executor's own round-trip test, not assumed safe.
4. **3 schema-data corrections, each found by insisting on byte-level/arithmetic verification rather
   than trusting a single prose claim or example in isolation** (documented in each schema file's own
   `sourceRefs`): Pricing's `usualAndCustomaryCharge` (DQ field) was modeled as signed-overpunch (`RO`)
   but the real worked example (`DQ00000000`) has no trailing letter — corrected to plain `R`, width 8.
   Claim's `quantityDispensed`/`quantityPrescribed`/`originallyPrescribedQuantity` were modeled with
   `places: 2` per the source doc's own prose, but the real worked example (`0000030000` → 30.000, not
   30.00) only reconciles at `places: 3` — corrected, with the doc's own internal inconsistency noted.
   ResponsePricing's F5/F6/F9 (Gross Amount Due/Ingredient Cost Paid/Total Amount Paid) were modeled at
   width 9 per a raw code-block example, but `data-types.md`'s own arithmetically-verified example
   showed width 8 — the raw block had a transcription typo (an extra digit); corrected to 8, matching the
   request-side Pricing.json's own convention.
5. **My own Go test files' type assertions were wrong relative to the real engine's own output type** —
   `fhir.build`'s `rowsPath` mode writes `[]map[string]interface{}` to `outputField` (confirmed directly
   in `fhir_build_executor.go`'s own `Execute()`), matching every OTHER existing `*_fhir_builder_test.go`
   in this codebase (e.g. 837P's own `.([]map[string]interface{})`) — the two new D.0 FHIR-builder test
   files had used `.([]interface{})` instead, a plausible-looking but wrong assumption borrowed from the
   goja bug fix above (a genuinely different layer: goja's Go↔JS boundary vs. a Go test's own direct type
   assertion on Go-native data). Both tests failed with a confusing symptom (`%v`-formatted output showed
   exactly one well-formed element, yet `len(claims) != 1` still fired) until the assertion type itself
   was corrected to match the real engine.
6. **A real Node.js proxy bug, found only by this phase's own synchronous-endpoint Playwright test**:
   `app.js`'s `express.raw()` middleware only recognized a fixed content-type allowlist
   (`application/edi-x12`, `text/plain`, etc.) for forwarding a request body verbatim as a Buffer. An
   EMPTY body sent with the new `application/x-ncpdp-telecom-d0` content type fell through to
   `express.json()`'s own default `req.body = {}` (never `undefined`) — `forwardToGo` then
   `JSON.stringify`s that into the non-empty string `"{}"`, so Go's own `len(body) == 0` empty-body guard
   silently never fired, and the request proceeded past it into a genuine pipeline-lookup 404 instead of
   the correct 400. The Go-level controller test (`httptest`, bypassing the Node proxy entirely) never
   could have caught this — only a REAL Playwright test through the REAL running app's own proxy layer
   did. Fixed by adding `application/x-ncpdp-telecom-d0` to the same `express.raw()` type list EDI's own
   content type already uses.

### Named simplifications (stated up front, not silently dropped)
- **Telecom standard only, not the batch standard** — no `STX`/`ETX`/`00T`/`G1`/`99` batch
  header/trailer wrapping; a transmission is always exactly one transaction.
- **B1 (Claim Billing) only** — B2 (Reversal), B3, E1/E2 (Eligibility), D0/D1 (Prior Authorization), and
  every other transaction code are named, deferred future phases; the segment library and generic engine
  make each one a pure schema-data addition later, the same proven claim already made for every other
  format phase in this codebase.
- **B1 segment scope**: Insurance/Claim/Pricing/Patient/Prescriber/Pharmacy-Provider (request) +
  Response Status/Response Pricing/Response Message/Response Claim (response) — DUR/PPS, Coupon,
  Compound, Coordination of Benefits, Workers' Comp, Prior Authorization, Clinical, Additional
  Documentation, Facility, Narrative (request) and Response DUR/PPS, Response Insurance, Response Prior
  Auth, Response Insurance Additional Documentation, Response COB, Response Patient (response) are all
  named, deferred — the schema is additive, so any of these can be added later as a pure data change.
- **Multiple transaction groups per transmission** — engine-supported (schema-driven, not hardcoded to
  1, confirmed via the round-trip test's own GS multi-group test) but only lightly tested; the primary
  fixture/test scenario throughout this phase is the common single-claim case.
- **No free, real-world D.0 sample exists** (unlike NCPDP SCRIPT's own real `dgoradia/ncpdp` NewRx
  fixture) — every test and Playwright fixture in this phase is self-authored from the confirmed field
  dictionary, a real, permanent sourcing-availability gap for this specific standard, not an oversight.

### Verification
Go: `ncpdptelecom/datatypes_test.go` (signed overpunch worked examples both directions), `ncpdptelecom/schema_loader_test.go`,
`ncpdptelecom/roundtrip_test.go` (B1 request/response round trips, GS multi-group test, `SniffDirection`
test), `ncpdptelecom/validator/validator_test.go`, `services/executors/transform/ncpdptelecom_executors_test.go`
(full parse→validate→build→map_to_canonical chain), `services/ncpdp_telecom_b1_{request,response}_fhir_builder_test.go`
(derive → `fhir.build` ×N → `payload.builder` → `fhir_validation` strict, zero unexpected errors),
`controllers/sync_pharmacy_claim_controller_test.go` (4 tests: real round trip against a real Postgres-
backed pipeline, unknown interface → 404, empty body → 400, malformed content → real error status). All
verified via the `/go-build-check` skill plus real `go test` runs through a throwaway `docker build
--target gobuilder` image (schemas/tests bind-mounted, image removed immediately after each use).

Full-stack: real `docker-compose build app` + `up` (confirmed `ncpdptelecom/` present in the runtime
image, clean startup, zero errors), `GET /api/ncpdp-telecom/schema/segments`/`/transactions` confirmed
live via direct `curl` against the real running app, `POST /api/pharmacy-claim/:id/submit` confirmed
live (404 for an unknown interface, 400 for an empty body after the proxy fix above).
`tests/playwright/ncpdp-telecom-b1-{request,response}-to-fhir-e2e.spec.js` each drive the real "Use
Template" click path (10 and 8 canvas nodes, matching each template's own `execution_groups`) then a
real "Test Pipeline" run against a self-authored B1 sample, asserting real field values (`Organization.name`,
`Patient.name`/`gender`, `Claim.status`/`identifier`/`item[].productOrService`/`.quantity`/`.net.value`,
`ClaimResponse.outcome`/`identifier`/`disposition`) AND that `Claim.patient.reference`/
`.provider.reference` are correctly rewritten to each referenced resource's own real bundle `fullUrl` —
not just "the run succeeded." `tests/playwright/sync-pharmacy-claim-e2e.spec.js` (4 tests, mirroring
`sync-claim-status-e2e.spec.js`) proves the real synchronous round trip through a real interface +
pipeline created via the same REST endpoints the UI itself calls — this is the run that caught the
`express.raw()` proxy bug above.

Two OOB migrations (V266 request, V267 response), each transcribed programmatically (never hand-retyped
— a Node script read the Go test source directly and `JSON.stringify`d the extracted config/script text,
diff-verified byte-for-byte against the generated JSON before being considered done) from
`services/ncpdp_telecom_b1_{request,response}_fhir_builder_test.go`, applied via `docker-compose run --rm
flyway` and confirmed live via `psql` (10 and 8 execution groups respectively).

### Diverse sample battery + a real bug it caught (September 2026, post-Phase-1)
Following up on the phase's own "no free, real-world D.0 sample exists" finding, a second, harder pass
searched again before accepting that conclusion: `apiv/dzero`'s own `spec/fixtures/b1.2_groups.request`
IS a real, third-party test fixture, but direct byte-level inspection (comparing it against its own
declared 56-byte `header_schema` and its own companion `.json` file) showed it's genuinely
non-conformant — only a 31-byte header, missing `binNumber` entirely, not a byte-accurate real-world
transmission. Run through the real pipeline anyway (as explicitly requested): the engine correctly
rejected it with a clear, graceful error (`unsupported transaction "11" direction "request"`, from the
fixed-width header slicing misaligning against the short input) — no crash, no silent misparse,
confirming the parser fails safely on malformed input rather than a false negative on real production
data (there is no such data to test against).

With no usable real sample, a diverse, byte-accurate **18-scenario battery** (10 B1 requests, 8 B1
responses) was generated via the real `ncpdptelecom/builder.BuildTransmission` itself (not hand-typed
fixed-width strings) — multi-claim transmissions, minimal-vs-fully-populated fields, fractional
compound quantities, prior authorization, controlled substances, zero-dollar and large-dollar claims,
and every response outcome family (Approved/Captured/Paid/Rejected/Duplicate/Error, with and without
pricing, single and multi-claim) — then run through the REAL running app's full FHIR-mapping pipeline
(`POST /api/pipelines/test` against real, persisted interfaces backed by the live V266/V267 template
config), checking both `ncpdptelecom.validate`'s own structural conformance AND `fhir_validation`'s
strict FHIR conformance for every sample.

**A real bug this found**: every rejected/duplicate/error response sample (which realistically carries
no `ResponsePricing` segment — a denied claim isn't priced) failed `ncpdptelecom.validate` with
"required segment ResponsePricing is missing." `ResponsePricing.json`'s own `grossAmountDue`/
`totalAmountPaid` fields were modeled `required: true`, and the validator has no conditional-
requirement mechanism (by design — see that package's own doc comment) to express "required only on
approval," despite the SAME schema file's own prior sourceRefs literally saying "Response Pricing
required on approval" without the engine actually enforcing that conditionality. Fixed by correcting
both fields to `required: false`, consistent with the validator's own already-stated "flexible, not
rigid, no fabricated conditional-requirement business rules" boundary — matching precedent rather than
inventing a new conditional-segment mechanism. All 18 samples, plus the full existing suite, pass
cleanly after the fix; zero regressions.

The battery itself is now a **permanent regression guard**, not a one-off script: the 18 generated
`.txt` fixtures live under `ncpdptelecom/testdata/generated_samples/`, and
`ncpdptelecom/generated_samples_test.go` parses+validates every one on every test run, asserting
`Valid: true` — a future schema/engine change that reintroduces this class of bug (or any other on
these realistic shapes) fails loudly instead of silently.

### Named future phases (not attempted this round)
B2 (Reversal), B3, E1/E2 (Eligibility), D0/D1 (Prior Authorization), the NCPDP batch standard (STX/ETX/
00T/G1/99 wrapping), and every named-deferred B1 segment above (DUR/PPS, Coupon, Compound, COB, Workers'
Comp, Prior Auth, Clinical, Additional Documentation, Facility, Narrative on the request side; Response
DUR/PPS, Response Insurance, Response Prior Auth, Response Insurance Additional Documentation, Response
COB, Response Patient on the response side).

### Key Files
- Shared engine: `ncpdptelecom/datatypes.go`, `ncpdptelecom/schema_types.go`, `ncpdptelecom/schema_loader.go`, `ncpdptelecom/parser.go`
- Build direction: `ncpdptelecom/builder/document_builder.go`
- Validator: `ncpdptelecom/validator/validator.go`
- Schema data: `ncpdptelecom/schemas/telecom_d0/{manifest.json,segments/*.json,transactions/{B1_request,B1_response}.json}`
- Parser adapter: `services/parsers/ncpdptelecom/ncpdp_telecom_parser_service.go`
- Pipeline steps: `services/executors/transform/ncpdptelecom_{parse,validate,build,map_to_canonical}_executor.go`
- Schema API: `controllers/ncpdp_telecom_schema_controller.go`
- Synchronous endpoint: `controllers/sync_pharmacy_claim_controller.go`
- UI: `public/js/pipeline/components/NCPDPTelecomStepBuilder.js`, `ToolboxManager.js` (4 entries + icons)
- FHIR mapping (source of truth for both migrations): `services/ncpdp_telecom_b1_request_fhir_builder_test.go`, `services/ncpdp_telecom_b1_response_fhir_builder_test.go`
- Migrations: `database/migrations/V266__NCPDP_Telecom_B1_Request_To_FHIR_OOB_Pipeline_Template.sql`, `V267__NCPDP_Telecom_B1_Response_To_FHIR_OOB_Pipeline_Template.sql`
- Proxy fix: `app.js` (`express.raw()` content-type list)
- Full browser click-path E2E: `tests/playwright/ncpdp-telecom-b1-request-to-fhir-e2e.spec.js`, `ncpdp-telecom-b1-response-to-fhir-e2e.spec.js`, `sync-pharmacy-claim-e2e.spec.js`
- Diverse sample battery (permanent regression guard, source of the `ResponsePricing` bug fix): `ncpdptelecom/testdata/generated_samples/*.txt` (18 files), `ncpdptelecom/generated_samples_test.go`

## Real Third-Party Sample Battery — NCPDP SCRIPT (33 fixtures) + a D.0 Bonus Find (September 2026)

### Why it exists
Following the D.0 sample-battery round above, the user asked the natural follow-up: what about SCRIPT (the "non-D.0" NCPDP format)? A harder, more targeted search than the one that originally built SCRIPT Phase 1 found **`cosyte/ncpdp`'s own `test/fixtures/` directory** — MIT-licensed (unlike `dzero`/`eduardonunesp`, both `license:null`), containing 33 real SCRIPT XML fixtures spanning 10 of the 12 transaction types this engine supports (only `RxFill`/`GetMessage` are absent — `cosyte` itself doesn't structurally model RxFill, matching this engine's own earlier finding), plus 3 real `.ncpdp` (Telecommunication D.0) fixtures — a genuine, unexpected second real source for the format this project had already concluded had none.

### Sourcing discipline, applied consistently
Same "real fixture, test-only, sourceRefs documented" precedent already established for EDI X12's own real-sample rounds (Databricks 837 samples, X12.org's own examples) — all 33 SCRIPT fixtures vendored to `ncpdp/testdata/real_samples/cosyte/`, all 3 D.0 fixtures to `ncpdptelecom/testdata/real_samples/`, both with a permanent regression test proving the engine still handles them correctly on every future change.

### Goal 1 (process every message): 100% — zero parse failures across all 33 real SCRIPT fixtures
Every single fixture parsed successfully via `ncpdp.ParseMessage`, including deliberately-adversarial ones (`legacy-version.xml` — SCRIPT v10.6 with an empty `<NewRx/>` body; `newrx-no-version.xml` — no version attribute at all; a root element using an XML namespace `xmlns="http://www.ncpdp.org/schema/SCRIPT"` rather than this engine's own `TransactionDomain="SCRIPT"` detection convention — confirmed harmless since `ParseMessage` itself never inspects that attribute, only `format_detector.go`'s own auto-detection heuristic does, and that heuristic wasn't exercised by this direct-engine-call battery). This is the core "process every message" claim, fully proven against real, independently-authored data.

### 3 real bugs found and fixed — all genuine structural mismatches between the schema's original sourcing and real wire data, none of them fabricated
1. **`CodedElement`'s Code/Qualifier: two real, independent sources genuinely disagree, and both are right.** `dgoradia/ncpdp`'s own real NewRx sample (this schema's original source) uses `<ProductCode><Code>62135012230</Code><Qualifier>ND</Qualifier></ProductCode>` (nested sub-elements). `cosyte/ncpdp`'s own fixtures use `<ProductCode Qualifier="ND">00093505601</ProductCode>` (code as the element's own text content, qualifier as an XML attribute) — confirmed deliberate, not a fixture typo, since `newrx-coded-and-strength.xml` uses BOTH shapes in the SAME message (`ProductCode` attribute-form, sibling `DrugDBCode` nested-form). Fixed generically (not a `ProductCode`-specific hack): `NCPDPFieldDef` gained a `FallbackXPath` field, tried on PARSE ONLY when the primary `XPath` resolves to nothing. The special value `"."` means "the anchor element's own text content" — handled directly in `parser.go`'s `extractValue` (bypassing `xmlpath`'s own segment walk, which has no "self" concept) rather than polluting the shared `xmlpath` package with NCPDP-specific semantics. Build/serialize direction is unchanged — always emits the nested form both sources agree is valid.
2. **`MedicationPrescribed`'s `quantity`/`writtenDate`/`sig` were required=true UNCONDITIONALLY, shared by reference across NewRx AND CancelRx/RxChangeRequest/RxRenewalRequest/RxFill alike** — but a real `cancelrx-request.xml` sample confirmed a cancellation legitimately carries only `DrugDescription`+`DrugCoded` (enough to IDENTIFY the prescription being cancelled), no quantity/dates/sig, since no new prescribing order is being created. This engine has no per-transaction-reference override for a shared group's own internal required flags (matching the exact same class of gap the D.0 `ResponsePricing` fix above already hit) — rather than build that machinery, relaxed to `required: false`, the same "flexible, not rigid" precedent. NewRx's own real fixtures (both `dgoradia`'s sample and `cosyte`'s own `newrx-basic.xml`) still populate all 3 in practice, so real NewRx messages validate cleanly regardless.
3. **CancelRxResponse/RxChangeResponse/RxRenewalResponse's own 6 outcome groups (Approved/Denied/DenyNewToFollow/ApprovedWithChanges/Validated/Replace) are nested one level deeper than originally modeled** — real wire XML is `<CancelRxResponse><Response><Approved>...`, not `<CancelRxResponse><Approved>...` directly. The original sourcing (`cosyte/ncpdp`'s own `lifecycle.ts` TypeScript type definitions) reflected that LIBRARY's own flattened in-memory data model, which drops the `<Response>` wrapper during its own parsing — a real, confirmed lesson that a library's TYPE DEFINITIONS and its ACTUAL WIRE-FORMAT FIXTURES can genuinely disagree, and only the fixtures are authoritative for structure. Fixed with a new shared `ResponseWrapper` group (`xmlElement: "Response"`, containing the 6 outcome group refs), referenced once by each of the 3 response transaction schemas instead of each one re-declaring the 6 refs at its own top level. **This bug was ALSO present, self-consistently, in this project's own self-authored Go-test fixtures** (`cancel_rx_response_denied_sample.xml`, `rx_change_response_approved_with_changes_sample.xml`, `rx_renewal_response_approved_sample.xml` — all 3 lacked the wrapper too, matching this project's own repeatedly-documented "test artifact self-consistent with a wrong assumption, invisible until real data arrives" pattern) — all 3 fixed alongside the schema, plus the 3 corresponding Go round-trip tests' own assertions (which read `result.Body["approved"]` etc. directly) updated to read `result.Body["response"]["approved"]`, plus the 3 live migrations (V260/V262/V264) re-applied directly via `psql` after `flyway repair` (same-session, minutes-old edits — matching the established re-apply-safety precedent).

### Goal 2 (FHIR mapping conformance): 15 of 16 real, FHIR-mapped fixtures pass cleanly; the 1 "failure" is a deliberately-constructed edge case
Of the 33 real fixtures, 15 map to one of the 7 SCRIPT transaction types with an OOB FHIR template (NewRx ×3, CancelRx, CancelRxResponse ×2, RxChangeRequest, RxChangeResponse ×2, RxRenewalRequest, RxRenewalResponse ×6) and validate cleanly at the NCPDP level — every one of these, run through the REAL running app's real Test Pipeline against a real, persisted interface+pipeline backed by each template's own live config, produced a strict-FHIR-conformant Bundle with zero unexpected errors. The 16th (`rxrenewal-response-no-outcome.xml`) fails `Task.status: required field 'status' is missing` — but its own embedded `<Note>` literally reads "Response carried no recognized outcome choice," deliberately testing the no-outcome case. The engine correctly refuses to fabricate a status rather than guess — this is proof the mapping is honest, not a bug.

### The D.0 bonus find: still no complete, spec-conformant sample exists — 2 more real fixtures confirm it, don't contradict it
`cosyte/ncpdp`'s own `test/fixtures/telecom/*.ncpdp` (3 files) were checked too, in the same pass. `pbm-reject-unknown.ncpdp`/`pbm-response-dur.ncpdp` are both shorter than the real 56-byte header (cosyte's own minimal unit-test snippets, not full messages) — the engine correctly rejects both with a clear, graceful error, never a crash. `pbm-person-code.ncpdp` IS byte-length-conformant (a genuine, real 56-byte header) but is missing real-world-required segments (`PharmacyProvider`, `Pricing`) and fields (`patientLastName`, `datePrescriptionWritten`) — `cosyte`'s own test focus was narrowly "person code" parsing, not a complete realistic claim. All 3 findings are now a permanent regression test (`ncpdptelecom/real_cosyte_fixtures_test.go`) proving this exact behavior (2 graceful rejections + 1 correctly-flagged-incomplete parse) rather than a one-time assertion.

### Verification
Go: `ncpdp/real_cosyte_fixtures_test.go` (33 real fixtures, 0 parse failures asserted unconditionally, `Valid: true` asserted for an explicit, individually-confirmed 18-fixture allowlist — the rest are deliberately-incomplete edge cases, logged not asserted), `ncpdptelecom/real_cosyte_fixtures_test.go` (3 real fixtures, the exact 3-way behavior above). Full `ncpdp`/`ncpdp/validator`/`ncpdptelecom`/`ncpdptelecom/validator`/`services` suites re-run after every fix with zero regressions — including 3 pre-existing round-trip tests (`TestCancelRxResponse_SelfAuthoredSample_DeniedOutcome_ParseAndRoundTrip` and its RxChangeResponse/RxRenewalResponse siblings) whose OWN assertions needed updating for the new `response.` nesting level, caught immediately by this same regression run, not silently left broken.

Full-stack: real `docker-compose build app` + restart (twice, once per fix batch) confirmed each fix live; a throwaway Node harness created 7 real interfaces+pipelines from the live V258-V264 template configs and ran all 15 applicable real fixtures through the actual `POST /api/pipelines/test` endpoint — the same mechanism `tests/playwright/ncpdp-*-to-fhir-e2e.spec.js` already uses, just driven directly rather than through a browser, since this was a many-sample sweep rather than a single click-path proof. All 3 fixed migrations (V260/V262/V264) confirmed live via `psql`. Throwaway interfaces deactivated + soft-deleted after verification.

### Key Files
- Engine fix (generic, not `ProductCode`-specific): `ncpdp/schema_types.go` (`NCPDPFieldDef.FallbackXPath`), `ncpdp/parser.go` (`extractValue`'s `"."` self-reference handling)
- Schema fixes: `ncpdp/schemas/script_2017071/groups/CodedElement.json`, `MedicationPrescribed.json`, `ResponseWrapper.json` (new), `manifest.json` (registration), `transactions/{CancelRxResponse,RxChangeResponse,RxRenewalResponse}.json`
- Self-authored fixture fixes (found to share the SAME bug as the schema): `ncpdp/testdata/self_authored/{cancel_rx_response_denied_sample.xml,rx_change_response_approved_with_changes_sample.xml,rx_renewal_response_approved_sample.xml}`
- Go test fixes: `ncpdp/lifecycle_roundtrip_test.go` (3 assertion updates), `services/ncpdp_{cancelrx_response,rxchangeresponse,rxrenewalresponse}_fhir_builder_test.go` (condition field path updates, source of truth for the 3 re-applied migrations)
- Migrations re-applied live: `database/migrations/V260__NCPDP_CancelRxResponse_To_FHIR_OOB_Pipeline_Template.sql`, `V262__NCPDP_RxChangeResponse_To_FHIR_OOB_Pipeline_Template.sql`, `V264__NCPDP_RxRenewalResponse_To_FHIR_OOB_Pipeline_Template.sql`
- Real fixtures + permanent regression tests: `ncpdp/testdata/real_samples/cosyte/*.xml` (33 files) + `ncpdp/real_cosyte_fixtures_test.go`; `ncpdptelecom/testdata/real_samples/*.ncpdp` (3 files) + `ncpdptelecom/real_cosyte_fixtures_test.go`
