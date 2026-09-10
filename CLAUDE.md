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

Re-derived directly from ground truth — `connector_stubs.go`'s remaining stub functions (a stub returns a bare `NewBase{In,Out}boundConnector(metadata)` with no real logic) cross-checked against `connector_factory.go`'s `registerBuiltInConnectors()` — rather than trusting the Phase 2B narrative above, which had drifted out of date across several sessions of connector work. **33 of 53 registered connector types are real** (17 of 26 inbound, 16 of 27 outbound; counts exclude type-name aliases like `tcp_mllp`/`http`/`http_rest`, and `sink_outbound`, which is real but trivial by design — a store-only terminal target with no external I/O).

**Real, beyond the Phase 2B list above** (11 further conversions):
- `mysql_outbound`, `aws_s3_outbound`, `kafka_outbound` — [mysql_outbound.go](services/connectors/mysql_outbound.go), [aws_s3_outbound.go](services/connectors/aws_s3_outbound.go), [kafka_outbound.go](services/connectors/kafka_outbound.go)
- `http_rest_inbound` — [http_rest_inbound.go](services/connectors/http_rest_inbound.go) — a genuinely separate, non-FHIR generic HTTP receiver; previously aliased to the FHIR connector, corrected after user feedback (see "HTTP FHIR Receiver vs Generic HTTP/REST Inbound" precedent — any format succeeds via `RawPassthroughParser`, only the FHIR connector validates)
- `azure_blob_inbound`, `azure_blob_outbound` — [azure_blob_inbound.go](services/connectors/azure_blob_inbound.go), [azure_blob_outbound.go](services/connectors/azure_blob_outbound.go)
- `databricks_inbound`, `databricks_outbound` — [databricks_inbound.go](services/connectors/databricks_inbound.go), [databricks_outbound.go](services/connectors/databricks_outbound.go) — official `databricks-sql-go` driver, PAT auth only
- `snowflake_inbound`, `snowflake_outbound` — [snowflake_inbound.go](services/connectors/snowflake_inbound.go), [snowflake_outbound.go](services/connectors/snowflake_outbound.go) — official `gosnowflake/v2` driver, username/password auth only (key-pair/JWT auth explicitly rejected with a clear error, not silently ignored — unverified against any real cloud warehouse account, no test credentials available in this environment)

**Still stubs (20 registered types, unchanged by the above)**:
- Analytics DBs — BigQuery, Redshift, Synapse, ClickHouse, TimescaleDB (both directions — 10 types)
- MQ outbound — RabbitMQ, Redis (publish; Kafka outbound is now real, see above)
- Cloud storage — GCS (both directions)
- File transfer — FTP (both directions)
- Phase 4 healthcare protocols — EDI X12, Direct Messaging (both directions)
- `fhir_r4_inbound`/`fhir_r4_outbound` (native FHIR R4, distinct from the already-real `http_fhir_*` connectors) — stub functions exist in `connector_stubs.go` but are **not registered in the factory at all**, i.e. unreachable via any type name today; a genuinely future Phase 4 item, not an oversight in this audit

**Dead code found and removed during this audit**: `connector_stubs.go` had an orphaned `NewHTTPRESTInboundConnector` (all-caps "REST") stub — same `type_name: "http_rest_inbound"` as the real `NewHTTPRestInboundConnector`, but never referenced by the factory (which registers the mixed-case name) — a leftover from the http_rest_inbound stub→real conversion that never got cleaned up. Removed; zero other references existed (confirmed via repo-wide grep before deleting).

### Connector Catalog

**Network Connectors**:
- tcp_mllp_inbound ✅ / tcp_mllp_outbound ✅
- http_outbound ✅ / http_fhir_inbound ✅ / http_fhir_outbound ✅
- http_rest_inbound ✅

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

### Named future phases (not designed here)
- **Phase 2**: 837 (claims) — schema-file-only addition per this design's own promise, plus 999 functional-acknowledgment generation.
- **Phase 3**: 270/271 (real-time eligibility) — needs HTTP transport, a materially different connector shape.
- **Phase 4**: AS2 transport (signed/encrypted HTTP + MDN) — widen the `transport` enum narrowed in V229 back out when this lands.
- **Phase 5**: X12→FHIR mapping (835 → `ExplanationOfBenefit`/`PaymentReconciliation` or a canonical intermediate) — explicitly out of scope; deliberately not scoped until named as its own phase (per user-confirmed sequencing).

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

### Named future phases (unchanged from Phase 1, plus one new one)
270/271 (eligibility) and AS2 transport remain out of scope, as before. **New**: 837/999 → FHIR mapping does not exist yet — 835 already maps to `PaymentReconciliation` (Phase 5, prior round), but 837P/837I currently have no FHIR-side output at all. Not scoped or attempted in this round; a real gap, not an oversight.
