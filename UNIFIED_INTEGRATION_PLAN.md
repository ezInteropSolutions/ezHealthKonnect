# Unified Connector Integration Plan

**Date**: October 26, 2025
**Goal**: Single unified architecture using new connector framework
**Principle**: Enterprise-grade, MVC + OOB pattern, no duplication

---

## 🎯 Architecture Vision

### Current State (Dual Architecture - TEMPORARY)
```
Legacy:  processing/tcp_input_connector.go  ←  REMOVE
Legacy:  processing/http_input_connector.go ←  REMOVE
Legacy:  processing/rest_output_connector.go ← REMOVE
Legacy:  processing/fhir_output_connector.go ← REMOVE

New:     services/connectors/*               ←  KEEP & INTEGRATE
```

### Target State (Unified Architecture)
```
processing/engine.go
    ↓
processing/connectors.go (UPDATED FACTORY)
    ↓
services/connectors/* (NEW FRAMEWORK)
    ↓
database (OOB configuration)
```

**Result**: ONE connector implementation per type, enterprise-grade, OOB-driven

---

## 🏗️ MVC + OOB Architecture (Enterprise Pattern)

### Model (M) - Data Layer ✅ COMPLETE
**Location**: `models/connectivity_models.go`, `models/message_models.go`

```go
type ConnectivityType struct {
    ID           string
    TypeName     string
    ConfigSchema json.RawMessage  // OOB: Schema-driven config
    // ... 20+ OOB fields
}

type InboundMessage struct {
    MessageID     string
    Content       string
    SourceType    string
    // ... metadata fields
}
```

**OOB Principle**: Configuration stored in database, not hardcoded

### View (V) - API Layer ✅ COMPLETE
**Location**: `controllers/connectivity_controller.go`

16 REST API endpoints for OOB connector management:
- List connector types (32 OOB connectors)
- Configure interface connectivity
- Test connections
- Monitor execution logs
- Manage cron schedules

**OOB Principle**: Runtime configuration via API, no code deployment

### Controller (C) - Business Logic ✅ COMPLETE
**Location**: `services/connectivity_service.go`, `services/connectors/*`

- ConnectivityService: CRUD operations for OOB configs
- ConnectorFactory: Dynamic instantiation by type name
- Base connectors: Thread-safe, lifecycle-managed

**OOB Principle**: Pluggable connectors, factory pattern, metadata-driven

---

## 🔧 Integration Strategy

### Phase 1: Update Processing Engine Interfaces

**Current Problem**: Engine expects legacy interfaces
```go
// processing/connectors.go (OLD)
type InputConnector interface {
    Start(ctx context.Context, messageChan chan<- Message) error
    TestConnection() error
    Close() error
    GetStatus() interface{}
}
```

**Solution**: Make engine use new framework interfaces
```go
// processing/connectors.go (NEW)
import "ezhealthkonnect/services/connectors"

// Alias to new framework interfaces
type InputConnector = connectors.InboundConnector
type OutputConnector = connectors.OutboundConnector
```

**Impact**: Engine automatically uses new framework ✅

### Phase 2: Update Message Model Compatibility

**Current Problem**: Engine uses `Message` struct, new framework uses `models.InboundMessage`

**Solution**: Update engine to use `models.InboundMessage` directly
```go
// processing/engine.go (UPDATE)
import "ezhealthkonnect/models"

func (pe *ProcessingEngine) ActivateInterface(interfaceID string) {
    // OLD: messageChan := make(chan Message, 100)
    // NEW:
    messageChan := make(chan *models.InboundMessage, 100)

    // Connector returns models.InboundMessage directly
    connector.Start(ctx, messageChan)
}
```

**Impact**: Zero conversion overhead, direct model usage ✅

### Phase 3: Update Connector Factory (OOB Pattern)

**Current Problem**: Hardcoded switch statement
```go
// processing/connectors.go (OLD)
func CreateInputConnector(connType string, config map[string]interface{}) (InputConnector, error) {
    switch connType {
    case "tcp": return NewTCPInputConnector(config)
    case "http": return NewHTTPInputConnector(config)
    }
}
```

**Solution**: Use OOB factory pattern
```go
// processing/connectors.go (NEW - ENTERPRISE OOB)
func CreateInputConnector(connType string, config []byte) (connectors.InboundConnector, error) {
    // OOB: Get from global factory (database-backed)
    factory := connectors.GetFactory()

    connector, err := factory.CreateInbound(connType)
    if err != nil {
        return nil, err
    }

    // OOB: Initialize with JSON config (schema-validated)
    err = connector.Initialize(config)
    if err != nil {
        return nil, err
    }

    return connector, nil
}
```

**OOB Benefits**:
- ✅ Database-driven configuration
- ✅ Schema validation
- ✅ Metadata tracking
- ✅ No code changes to add new connectors
- ✅ Runtime pluggability

### Phase 4: Update Interface Activation (OOB Config Retrieval)

**Current Problem**: Config hardcoded in interface table
```go
// processing/engine.go - ActivateInterface() (OLD)
sourceConfig := interface.SourceConfig // map[string]interface{}
connector := CreateInputConnector("tcp", sourceConfig)
```

**Solution**: Get OOB config from database
```go
// processing/engine.go - ActivateInterface() (NEW - ENTERPRISE OOB)
// Step 1: Get interface connectivity from OOB database
connectivityService := services.NewConnectivityService(pe.db)
interfaceConn, err := connectivityService.GetInterfaceConnectivity(interfaceID)
if err != nil {
    return fmt.Errorf("no connectivity configured for interface: %w", err)
}

// Step 2: Get connector type metadata from OOB catalog
connectorType, err := connectivityService.GetConnectivityTypeByID(interfaceConn.SourceTypeID)
if err != nil {
    return fmt.Errorf("invalid connector type: %w", err)
}

// Step 3: Create connector using OOB factory
configJSON, _ := json.Marshal(interfaceConn.SourceConfig)
connector, err := CreateInputConnector(connectorType.TypeName, configJSON)
if err != nil {
    return fmt.Errorf("failed to create connector: %w", err)
}

// Step 4: Start connector with OOB monitoring
messageChan := make(chan *models.InboundMessage, 100)
err = connector.Start(ctx, messageChan)

// Step 5: Track execution in OOB logs
executionLog := &models.ConnectivityExecutionLog{
    InterfaceID: interfaceID,
    TypeID:      interfaceConn.SourceTypeID,
    Status:      "running",
    StartedAt:   time.Now(),
}
connectivityService.LogExecution(executionLog)
```

**OOB Benefits**:
- ✅ All config in database (interface_connectivity table)
- ✅ Connector metadata tracked (connectivity_types table)
- ✅ Execution logs (connectivity_execution_log table)
- ✅ Cron schedules (cron_jobs table)
- ✅ Full audit trail

---

## 📊 Enterprise Scalability (Millions/Billions)

### 1. Connection Pooling ✅ Built-in
```go
// services/connectors/tcp_mllp_inbound.go
type TCPMLLPInboundConnector struct {
    maxConnections  int           // OOB: From config
    activeConns     map[string]net.Conn
    connectionMutex sync.RWMutex  // Thread-safe
}
```

**OOB Config**:
```json
{
  "max_connections": 1000,
  "connection_timeout": 30,
  "keep_alive": true
}
```

### 2. Message Channel Buffering ✅ Configurable
```go
// processing/engine.go
bufferSize := interfaceConn.SourceConfig["buffer_size"].(int)
if bufferSize == 0 {
    bufferSize = 10000 // OOB: High-volume default
}
messageChan := make(chan *models.InboundMessage, bufferSize)
```

**OOB Config**: Per-interface buffer size in database

### 3. Goroutine Pool ✅ Resource Management
```go
// processing/tcp_mllp_inbound.go
func (c *TCPMLLPInboundConnector) handleConnection() {
    // Each connection = 1 goroutine
    // Max connections = max goroutines
    // OOB: Configurable limit prevents resource exhaustion
}
```

### 4. Batch Processing ✅ Built-in
```go
// services/connectors/base_connector.go
type BaseOutboundConnector struct {
    supportsBatch bool // OOB: From metadata
}

func (boc *BaseOutboundConnector) SendBatch(messages []*models.OutboundMessage) {
    // Batch size from OOB config
    // Reduces network overhead for billions of messages
}
```

### 5. Async Processing ✅ Already Implemented
```
Message Received (sync - PostgreSQL)
    ↓
MongoDB Storage (async goroutine)
    ↓
JSON Conversion (async goroutine)
    ↓
Transformation (async goroutine)
    ↓
Delivery (async goroutine with retry)
```

**Result**: Input connector never blocks, processes millions/sec

### 6. Database Sharding ✅ Interface-Specific Tables
```sql
-- OOB: Each interface gets dedicated table
messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d
messages_intf_729bc2f9_1d61_558b_a84e_fcgd26941e8e
...
```

**Benefit**: No table lock contention, scales linearly

---

## 🔄 Migration Steps (Zero Downtime)

### Step 1: Backup Old Connectors (Reference Only)
```bash
mv processing/tcp_input_connector.go processing/tcp_input_connector.go.reference
mv processing/http_input_connector.go processing/http_input_connector.go.reference
mv processing/rest_output_connector.go processing/rest_output_connector.go.reference
mv processing/fhir_output_connector.go processing/fhir_output_connector.go.reference
```

**Status**: Old code preserved for reference, NOT compiled

### Step 2: Update Message Model (Unified)
```go
// processing/types.go (DELETE - use models.InboundMessage directly)
// OLD: type Message struct { ... }
// NEW: import "ezhealthkonnect/models"
//      Use models.InboundMessage everywhere
```

### Step 3: Update Connector Interfaces (Alias to New)
```go
// processing/connectors.go (UPDATE)
import "ezhealthkonnect/services/connectors"

// Alias to new framework
type InputConnector = connectors.InboundConnector
type OutputConnector = connectors.OutboundConnector
```

### Step 4: Update Factories (OOB Pattern)
```go
// processing/connectors.go (REWRITE)
func CreateInputConnector(typeName string, configJSON []byte) (InputConnector, error) {
    factory := connectors.GetFactory()
    connector, err := factory.CreateInbound(typeName)
    if err != nil {
        return nil, err
    }
    return connector, connector.Initialize(configJSON)
}

func CreateOutputConnector(typeName string, configJSON []byte) (OutputConnector, error) {
    factory := connectors.GetFactory()
    connector, err := factory.CreateOutbound(typeName)
    if err != nil {
        return nil, err
    }
    return connector, connector.Initialize(configJSON)
}
```

### Step 5: Update Engine Integration (OOB Config)
```go
// processing/engine.go - ActivateInterface() (UPDATE)
// Replace hardcoded config with OOB database lookup
// See "Phase 4" above for full code
```

### Step 6: Update Output Delivery (OOB Config)
```go
// services/output_delivery_service.go (UPDATE)
// Use new OutputConnector interface
// Get config from interface_connectivity.target_config
```

### Step 7: Build & Test
```bash
docker-compose build app
docker-compose restart app
# Test with existing interface
# Old connectors not compiled (zero runtime impact)
```

---

## ✅ Benefits Summary

### Enterprise Features (OOB)
1. ✅ **Configuration Management**: All in database, no code changes
2. ✅ **Metadata Tracking**: 32 connector types with full metadata
3. ✅ **Execution Monitoring**: Real-time status, logs, statistics
4. ✅ **Schema Validation**: JSON schema validation for all configs
5. ✅ **API Management**: 16 REST endpoints for runtime control
6. ✅ **Audit Trail**: Complete execution history in database

### Scalability Features
1. ✅ **Connection Pooling**: Configurable max connections per interface
2. ✅ **Buffered Channels**: High-volume message queuing (10K+ buffer)
3. ✅ **Async Processing**: Non-blocking pipeline (millions/sec throughput)
4. ✅ **Batch Operations**: Reduce overhead for bulk delivery
5. ✅ **Table Sharding**: Interface-specific tables (no lock contention)
6. ✅ **Goroutine Management**: Resource limits prevent exhaustion

### Clean Architecture
1. ✅ **Single Implementation**: One connector per type
2. ✅ **MVC Pattern**: Models, Services (Controller), API (View)
3. ✅ **Factory Pattern**: Dynamic instantiation
4. ✅ **Interface Segregation**: Clean contracts
5. ✅ **Thread Safety**: RWMutex on all shared state
6. ✅ **Graceful Shutdown**: Context cancellation + stop channels

---

## 📝 Files to Modify

### Update (8 files):
1. `processing/connectors.go` - OOB factory pattern
2. `processing/engine.go` - OOB config retrieval
3. `processing/engine_message_processor.go` - Use models.InboundMessage
4. `services/output_delivery_service.go` - Use new OutputConnector
5. `controllers/InterfaceLifecycleController.js` - Check connectivity config exists
6. `processing/types.go` - Remove Message struct (use models.InboundMessage)

### Rename (Reference Only - 4 files):
1. `processing/tcp_input_connector.go` → `.reference`
2. `processing/http_input_connector.go` → `.reference`
3. `processing/rest_output_connector.go` → `.reference`
4. `processing/fhir_output_connector.go` → `.reference`

### No Changes (Preserved - All other files):
- `services/connectors/*` ✅ Already complete
- `models/*` ✅ Already compatible
- `controllers/connectivity_controller.go` ✅ Already operational
- `database/migrations/*` ✅ Schema complete

---

## 🚀 Implementation Timeline

**Total Time**: 3-4 hours

1. **Backup old connectors** (5 min)
2. **Update message models** (30 min)
3. **Update connector interfaces** (15 min)
4. **Rewrite factories with OOB** (45 min)
5. **Update engine integration** (60 min)
6. **Update output delivery** (30 min)
7. **Build & test** (30 min)
8. **End-to-end verification** (30 min)

---

## ✅ Success Criteria

After integration:
1. ✅ Zero old connector code in runtime
2. ✅ All 32 connectors available via OOB database
3. ✅ Interface activation uses new framework
4. ✅ Message flow: TCP → Engine → Transform → REST (all new)
5. ✅ Configuration fully database-driven (OOB)
6. ✅ Execution logs tracked in database
7. ✅ Build succeeds with zero warnings
8. ✅ Existing interfaces work without changes

---

**Ready to implement?**
Start with Step 1: Backup old connectors as reference files.
