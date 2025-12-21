# Dynamic Port Listeners for FHIR Receivers - Design Document

## Problem Statement

**Current Implementation**: FHIR receiver endpoints are mounted on the main application port (3000) at `/fhir/receiver/:interfaceId`.

**User Requirement**: Users should be able to configure FHIR receivers to listen on **dedicated ports** (e.g., 8082, 8083, etc.) that are independent of the main application ports.

**Example Use Case**:
```
Interface 1: FHIR Receiver listening on port 8082 at /fhir/r4
Interface 2: FHIR Receiver listening on port 8083 at /fhir/stu3
Interface 3: FHIR Receiver listening on port 8084 at /api/fhir

Main Application: Continues to run on ports 3000 (Node.js) and 8080 (Go)
```

## Why This Matters

### 1. **Healthcare Integration Standards**
- Different EHR systems may need to connect to different ports
- Each FHIR receiver can have different authentication, base paths, and FHIR versions
- Isolation between different healthcare partners/systems

### 2. **Security & Isolation**
- Each interface can have different firewall rules
- Port-level access control
- Separate SSL/TLS certificates per interface

### 3. **Regulatory Compliance**
- HIPAA: Separate endpoints for different PHI access levels
- Audit trail: Port-level tracking of which system connected

### 4. **Port Conflict Prevention**
- Users cannot accidentally use application ports (3000, 8080, 8090-8097)
- Clear validation messages

## Architecture Design

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    ezHealthKonnect                          │
│                                                             │
│  Main Application (Node.js):                               │
│  ├─ Port 3000: UI + API                                    │
│  └─ Port 8080: Go Backend Proxy                            │
│                                                             │
│  Dynamic FHIR Receivers (Node.js HTTP Servers):            │
│  ├─ Interface A: Port 8082 → /fhir/r4                      │
│  │   └─ Authentication: OAuth 2.0                          │
│  │   └─ Accepted Resources: Patient, Observation           │
│  │                                                          │
│  ├─ Interface B: Port 8083 → /fhir/stu3                    │
│  │   └─ Authentication: Basic Auth                         │
│  │   └─ Accepted Resources: All                            │
│  │                                                          │
│  └─ Interface C: Port 8084 → /api/fhir                     │
│      └─ Authentication: mTLS                                │
│      └─ Accepted Resources: Encounter, DiagnosticReport    │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Component Architecture

```
┌──────────────────────────────────────────────────────────┐
│ 1. Interface Configuration (Wizard + Edit Modal)        │
│    - User configures FHIR receiver with custom port     │
│    - Port validation (must not be reserved)             │
│    - Base path configuration (e.g., /fhir/r4)           │
│    - FHIR version, operations, auth settings            │
└──────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────┐
│ 2. Interface Lifecycle Service                          │
│    - Start interface: Create HTTP server on port        │
│    - Stop interface: Close HTTP server                  │
│    - Restart interface: Stop + Start                    │
│    - Status tracking: running, stopped, error           │
└──────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────┐
│ 3. Dynamic HTTP Server Manager                          │
│    - Registry of active HTTP servers by interface ID    │
│    - Express app creation per interface                 │
│    - Port binding with error handling                   │
│    - Graceful shutdown on interface deactivation        │
└──────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────┐
│ 4. FHIR Receiver Request Handler                        │
│    - Routes incoming FHIR requests per interface        │
│    - Authentication (OAuth, Basic, Bearer, mTLS)        │
│    - Resource validation                                │
│    - Storage (PostgreSQL + MongoDB)                     │
│    - Trigger transformation pipeline                    │
└──────────────────────────────────────────────────────────┘
```

## Configuration Schema

### Source Configuration (FHIR Receiver)

```json
{
  "sourceType": "fhir",
  "sourceConnectivity": {
    "type": "http",
    "config": {
      "port": 8082,              // ✅ User-configured listener port
      "basePath": "/fhir/r4",    // ✅ Base path for FHIR endpoints
      "host": "0.0.0.0"          // Listen on all interfaces
    }
  },
  "sourceConfig": {
    "fhirVersion": "R4",
    "operations": ["CREATE", "READ", "UPDATE", "SEARCH"],
    "contentType": "application/fhir+json",

    // Authentication
    "authType": "oauth2",
    "oauthIssuer": "https://auth.hospital.com",
    "oauthAudience": "https://fhir.hospital.com",

    // Resource Filtering
    "acceptedResources": ["Patient", "Observation", "Encounter"],

    // Validation
    "validateStructure": true,
    "validateProfiles": false,
    "validateTerminology": false,

    // Post-Reception Actions
    "postReceptionActions": ["store", "transform", "audit"]
  }
}
```

## Port Validation Rules

### Reserved Ports (Cannot Use)
```javascript
const RESERVED_PORTS = [
    3000,          // Node.js main app
    8080, 8081,    // Go backend
    8090, 8091, 8092, 8093, 8094, 8095, 8096, 8097,  // Reserved for future use
    5432,          // PostgreSQL
    27017          // MongoDB
];

const RESERVED_RANGES = [
    { start: 1, end: 1023 }      // Well-known ports (requires root)
];
```

### Allowed Port Range
```
Valid Ports: 1024-65535 (excluding reserved ports)
Recommended: 8082-8089, 9000-9999
```

### Validation Logic
```javascript
function validatePort(port, interfaceId) {
    // 1. Check if port is a valid number
    if (!Number.isInteger(port) || port < 1024 || port > 65535) {
        return {
            valid: false,
            error: 'Port must be between 1024 and 65535'
        };
    }

    // 2. Check if port is reserved
    if (RESERVED_PORTS.includes(port)) {
        return {
            valid: false,
            error: `Port ${port} is reserved for application use. Please choose a different port.`
        };
    }

    // 3. Check if port is already in use by another interface
    const existingInterface = getInterfaceByPort(port);
    if (existingInterface && existingInterface.id !== interfaceId) {
        return {
            valid: false,
            error: `Port ${port} is already in use by interface "${existingInterface.name}"`
        };
    }

    // 4. Check if port is available (not bound by another process)
    const isAvailable = await checkPortAvailability(port);
    if (!isAvailable) {
        return {
            valid: false,
            error: `Port ${port} is already in use by another process`
        };
    }

    return { valid: true };
}
```

## Implementation Plan

### Phase 1: Port Validation (Week 1)
**Goal**: Prevent users from using reserved ports

**Tasks**:
1. ✅ Update shared components to show port field with proper labels
2. ✅ Add client-side validation in wizard/edit modal
3. ✅ Add server-side validation in interface creation/update
4. ✅ Add real-time port availability checking
5. ✅ Show helpful error messages

**Files**:
- `public/js/components/InterfaceConfigComponents.js` - Add port validation hints
- `services/interfaceService.js` - Add port validation logic
- `controllers/InterfaceLifecycleController.js` - Add port conflict checks

### Phase 2: Dynamic HTTP Server Manager (Week 2)
**Goal**: Create and manage HTTP servers per interface

**Tasks**:
1. ✅ Create `DynamicServerManager` service
2. ✅ Implement server creation on interface activation
3. ✅ Implement server destruction on interface deactivation
4. ✅ Add server registry (Map of interfaceId → HTTP server)
5. ✅ Handle server restart on configuration changes
6. ✅ Add graceful shutdown logic

**New File**:
```javascript
// services/DynamicServerManager.js
class DynamicServerManager {
    constructor() {
        this.servers = new Map(); // interfaceId → {server, app, port}
    }

    async startServer(interfaceConfig) {
        // 1. Validate port is available
        // 2. Create Express app for this interface
        // 3. Mount FHIR receiver routes
        // 4. Start HTTP server on configured port
        // 5. Store in registry
        // 6. Update interface status to 'running'
    }

    async stopServer(interfaceId) {
        // 1. Get server from registry
        // 2. Close HTTP server
        // 3. Remove from registry
        // 4. Update interface status to 'stopped'
    }

    async restartServer(interfaceId) {
        await this.stopServer(interfaceId);
        const config = await getInterfaceConfig(interfaceId);
        await this.startServer(config);
    }

    getServerStatus(interfaceId) {
        return this.servers.has(interfaceId) ? 'running' : 'stopped';
    }
}
```

### Phase 3: FHIR Request Routing (Week 3)
**Goal**: Route incoming FHIR requests to correct interface handler

**Tasks**:
1. ✅ Update `FhirReceiverController` to work with dynamic servers
2. ✅ Implement per-interface Express middleware chain:
   - Authentication middleware (based on interface config)
   - Resource filtering middleware
   - Validation middleware
   - Logging middleware
3. ✅ Add FHIR operation handlers (CREATE, READ, UPDATE, PATCH, DELETE, SEARCH)
4. ✅ Add FHIR Bundle transaction support
5. ✅ Add OperationOutcome response generation

**Updated File**:
```javascript
// controllers/FhirReceiverController.js
class FhirReceiverController {
    // New: Create Express app for interface
    createInterfaceApp(interfaceConfig) {
        const app = express();
        app.use(express.json());

        // Authentication middleware
        app.use(this.createAuthMiddleware(interfaceConfig.sourceConfig));

        // Resource filtering middleware
        app.use(this.createResourceFilterMiddleware(interfaceConfig.sourceConfig));

        // Validation middleware
        app.use(this.createValidationMiddleware(interfaceConfig.sourceConfig));

        // FHIR REST operations
        const basePath = interfaceConfig.sourceConnectivity.config.basePath || '/fhir';

        if (interfaceConfig.sourceConfig.operations.includes('CREATE')) {
            app.post(`${basePath}/:resourceType`, this.handleCreate.bind(this));
        }
        if (interfaceConfig.sourceConfig.operations.includes('READ')) {
            app.get(`${basePath}/:resourceType/:id`, this.handleRead.bind(this));
        }
        if (interfaceConfig.sourceConfig.operations.includes('UPDATE')) {
            app.put(`${basePath}/:resourceType/:id`, this.handleUpdate.bind(this));
        }
        if (interfaceConfig.sourceConfig.operations.includes('SEARCH')) {
            app.get(`${basePath}/:resourceType`, this.handleSearch.bind(this));
        }
        if (interfaceConfig.sourceConfig.operations.includes('BATCH')) {
            app.post(`${basePath}`, this.handleBundle.bind(this));
        }

        // Metadata endpoint (always enabled)
        app.get(`${basePath}/metadata`, this.handleMetadata.bind(this));

        return app;
    }
}
```

### Phase 4: Lifecycle Management Integration (Week 4)
**Goal**: Integrate with existing interface lifecycle (activate/deactivate/pause)

**Tasks**:
1. ✅ Update activate endpoint to start HTTP server
2. ✅ Update deactivate endpoint to stop HTTP server
3. ✅ Update pause endpoint to stop HTTP server (but keep config)
4. ✅ Update edit/update endpoint to restart server if running
5. ✅ Add server status to interface list UI
6. ✅ Add "Test Connection" button to verify server is listening

**Updated Files**:
- `controllers/InterfaceLifecycleController.js` - Add server lifecycle calls
- `public/interfaces.html` - Add server status indicators
- `public/js/interfaces.js` - Add server status polling

### Phase 5: Docker & Networking (Week 5)
**Goal**: Ensure dynamic ports work in Docker environment

**Tasks**:
1. ✅ Update `docker-compose.yml` to expose dynamic port range
2. ✅ Add environment variable for allowed port range
3. ✅ Add health checks for dynamic servers
4. ✅ Update documentation for Docker networking

**docker-compose.yml Update**:
```yaml
services:
  app:
    ports:
      - "3000:3000"      # Main UI
      - "8080:8080"      # Go backend
      - "8082-8089:8082-8089"  # Dynamic FHIR receivers
      - "9000-9010:9000-9010"  # Additional dynamic ports
    environment:
      - DYNAMIC_PORT_RANGE_START=8082
      - DYNAMIC_PORT_RANGE_END=8089
```

## UI Changes

### 1. FHIR Receiver Configuration Form

**Current** (Confusing):
```
[Host: localhost]  [Port: 2575]
```

**Proposed** (Clear):
```
Listener Configuration
┌────────────────────────────────────────────────────┐
│ 🎧 Listen Port: [8082]                            │
│ ℹ️ Port where this FHIR receiver will listen     │
│ ⚠️ Reserved ports: 3000, 8080-8081, 8090-8097    │
│ ✅ Recommended: 8082-8089, 9000-9999              │
│                                                    │
│ 📍 Base Path: [/fhir/r4]                          │
│ ℹ️ URL path prefix for FHIR endpoints            │
│ Example: http://your-server:8082/fhir/r4/Patient │
└────────────────────────────────────────────────────┘
```

### 2. Port Validation Messages

**Available Port**:
```
✅ Port 8082 is available
   External systems can send FHIR resources to:
   http://your-server:8082/fhir/r4
```

**Reserved Port**:
```
❌ Port 3000 is reserved for the main application
   Please choose a different port (recommended: 8082-8089)
```

**Port In Use**:
```
❌ Port 8083 is already used by interface "Epic FHIR Receiver"
   Please choose a different port
```

### 3. Interface Status Indicators

```
Interface Name         Source         Port    Status      Actions
─────────────────────────────────────────────────────────────────
Epic FHIR Receiver    FHIR (HTTP)    8082    🟢 Running  [Stop] [Restart] [Edit]
Cerner FHIR Receiver  FHIR (HTTP)    8083    🔴 Stopped  [Start] [Edit]
Lab HL7 Interface     HL7 v2 (TCP)   2575    🟢 Running  [Stop] [Restart] [Edit]
```

## Testing Strategy

### Unit Tests
```javascript
describe('Port Validation', () => {
    test('should reject reserved application port 3000', () => {
        const result = validatePort(3000);
        expect(result.valid).toBe(false);
        expect(result.error).toContain('reserved');
    });

    test('should reject port below 1024', () => {
        const result = validatePort(80);
        expect(result.valid).toBe(false);
    });

    test('should accept valid port 8082', () => {
        const result = validatePort(8082);
        expect(result.valid).toBe(true);
    });
});
```

### Integration Tests
```javascript
describe('Dynamic Server Manager', () => {
    test('should start HTTP server on configured port', async () => {
        const config = {
            id: 'test-123',
            sourceConnectivity: { config: { port: 8082 } }
        };

        await manager.startServer(config);

        // Verify server is listening
        const response = await fetch('http://localhost:8082/fhir/metadata');
        expect(response.ok).toBe(true);
    });

    test('should prevent port conflict', async () => {
        // Start first server on 8082
        await manager.startServer(config1);

        // Try to start second server on same port
        await expect(manager.startServer(config2WithSamePort))
            .rejects.toThrow('Port already in use');
    });
});
```

### Manual Testing Checklist
- [ ] Create FHIR receiver with port 8082
- [ ] Activate interface - server starts on 8082
- [ ] Send FHIR Patient resource to http://localhost:8082/fhir/r4/Patient
- [ ] Verify resource is stored in database
- [ ] Try to create second interface with port 8082 - should fail
- [ ] Try to use port 3000 - should fail with helpful message
- [ ] Deactivate interface - server stops
- [ ] Verify port 8082 is released and can be reused

## Security Considerations

### 1. Port Range Restrictions
- Only allow user-configurable ports in safe range (1024-65535)
- Block well-known ports (1-1023) that require root
- Block application ports to prevent accidental conflicts

### 2. Firewall Configuration
- Document firewall rules needed for dynamic ports
- Provide scripts for common firewalls (iptables, ufw, Windows Firewall)

### 3. SSL/TLS Support
- Phase 6: Add per-interface SSL certificate configuration
- Support Let's Encrypt auto-certificates
- Support custom certificates (PEM format)

### 4. DoS Protection
- Add rate limiting per interface
- Connection limits per interface
- Request size limits

## Migration Path

### For Existing Interfaces
1. **Default Migration**: Assign next available port (starting at 8082)
2. **User Notification**: "Your FHIR receiver has been assigned port 8082"
3. **Update Documentation**: External systems need to update their connection URLs

### Backward Compatibility
- Keep `/fhir/receiver/:interfaceId` endpoint on main app (port 3000) for 2 releases
- Add deprecation warnings
- Redirect to new dedicated port endpoints

## Success Criteria

### User Experience
- ✅ Users can configure custom ports for FHIR receivers
- ✅ Clear validation prevents port conflicts
- ✅ Helpful error messages guide users to valid ports
- ✅ Interface list shows server status (running/stopped)
- ✅ Test connection button verifies server is listening

### Technical
- ✅ Each FHIR receiver runs on its configured port
- ✅ No port conflicts with main application
- ✅ Servers start automatically on interface activation
- ✅ Servers stop automatically on interface deactivation
- ✅ Configuration changes trigger server restart
- ✅ Works in Docker environment

### Performance
- ✅ Starting a server takes <1 second
- ✅ Stopping a server is graceful (pending requests complete)
- ✅ Multiple servers run concurrently without interference
- ✅ Supports at least 10 concurrent FHIR receiver interfaces

## Documentation Deliverables

1. **User Guide**: "Configuring FHIR Receiver Ports"
2. **Admin Guide**: "Managing Dynamic Port Listeners"
3. **Developer Guide**: "Dynamic Server Manager Architecture"
4. **API Documentation**: "FHIR Receiver REST API"
5. **Troubleshooting Guide**: "Port Conflicts and Resolution"

## Timeline Estimate

| Phase | Duration | Status |
|-------|----------|--------|
| Phase 1: Port Validation | 1 week | ⏳ Ready to start |
| Phase 2: Dynamic Server Manager | 1 week | ⏳ Pending Phase 1 |
| Phase 3: FHIR Request Routing | 1 week | ⏳ Pending Phase 2 |
| Phase 4: Lifecycle Integration | 1 week | ⏳ Pending Phase 3 |
| Phase 5: Docker & Networking | 1 week | ⏳ Pending Phase 4 |
| **Total** | **5 weeks** | |

## Next Steps

**Immediate Action Items**:
1. Review and approve this design document
2. Update FHIR receiver UI to clarify port is for listening
3. Add port validation to wizard and edit modal
4. Begin Phase 1 implementation

**Questions for Discussion**:
1. What is the preferred dynamic port range? (Recommendation: 8082-8089, 9000-9999)
2. Should we migrate existing interfaces automatically or require manual reconfiguration?
3. Do we need per-interface SSL/TLS in Phase 1, or can that wait for Phase 6?

---

**Status**: 📋 **Design Complete - Awaiting Approval**
**Priority**: 🔴 **High** - Blocks proper FHIR receiver functionality
**Complexity**: 🟡 **Medium** - Well-defined scope, clear implementation path
