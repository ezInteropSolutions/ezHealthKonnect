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