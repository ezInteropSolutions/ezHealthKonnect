# Connector Implementation Guide

## Phase 2 Progress: Foundation Complete ✅

### What's Been Implemented

#### 1. Universal Connector Interface (services/connectors/connector_interface.go)
Defines the contract that all connectors must implement:

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

#### 2. Base Connector Implementation (services/connectors/base_connector.go)
Provides thread-safe base functionality for all connectors:

**BaseConnector**:
- Configuration management
- State tracking (Initializing, Ready, Running, Stopped, Error)
- Connection status monitoring
- Message counters (sent/received)
- Error tracking and recording
- Metadata management
- Thread-safe operations with RWMutex

**BaseInboundConnector**:
- Extends BaseConnector
- Running state management
- Stop channel for graceful shutdown
- Cron support detection

**BaseOutboundConnector**:
- Extends BaseConnector
- Batch operation support
- Default batch implementation (sequential sends)

#### 3. Connector Factory (services/connectors/connector_factory.go)
Factory pattern for creating connector instances:

```go
factory := connectors.GetFactory()

// Create connectors by type name
tcpInbound, err := factory.CreateInbound("tcp_mllp_inbound")
httpOutbound, err := factory.CreateOutbound("http_outbound")
pgInbound, err := factory.CreateInbound("postgresql_inbound")

// Register custom connectors
factory.RegisterInbound("custom_connector", NewCustomConnector)
```

**Built-in Registrations**:
- All 32 OOB connectors automatically registered
- Global factory singleton pattern
- Thread-safe registration

#### 4. Connector Stubs (services/connectors/connector_stubs.go)
Minimal implementations for all 32 connectors:

**Network Connectors (4)**:
- TCP/MLLP Inbound/Outbound
- HTTP REST Inbound
- HTTP Outbound

**File System Connectors (2)**:
- File Listener (Inbound)
- File Writer (Outbound)

**Database Connectors (10)**:
- PostgreSQL Inbound/Outbound
- MySQL Inbound/Outbound
- SQL Server Inbound/Outbound
- MongoDB Inbound/Outbound
- Oracle Inbound/Outbound

**Message Queue Connectors (6)**:
- RabbitMQ Inbound/Outbound
- Kafka Inbound/Outbound
- Redis Inbound/Outbound

**Cloud Storage Connectors (6)**:
- AWS S3 Inbound/Outbound
- Azure Blob Inbound/Outbound
- Google Cloud Storage Inbound/Outbound

**File Transfer Connectors (4)**:
- SFTP Inbound/Outbound
- FTP Inbound/Outbound

---

## How to Implement a Connector

### Step 1: Create Connector Type in Database

Already done via migrations! All 32 connector types are defined in:
- V26__Multi_Connectivity_Support.sql
- V27__Database_Connectivity_Support.sql
- V28__Message_Queue_And_Cloud_Connectors.sql
- V29__Add_TCP_MLLP_Outbound.sql

### Step 2: Create Connector Implementation File

Example: **TCP/MLLP Inbound Connector**

Create: `services/connectors/tcp_mllp_inbound.go`

```go
package connectors

import (
    "context"
    "ezhealthkonnect/models"
    "net"
    "crypto/tls"
    "fmt"
)

// TCPMLLPInboundConnector implements HL7 MLLP protocol listener
type TCPMLLPInboundConnector struct {
    *BaseInboundConnector
    listener   net.Listener
    port       int
    enableTLS  bool
    tlsConfig  *tls.Config
}

// NewTCPMLLPInboundConnector creates a new TCP/MLLP inbound connector
func NewTCPMLLPInboundConnector() InboundConnector {
    metadata := ConnectorMetadata{
        TypeName:           "tcp_mllp_inbound",
        DisplayName:        "TCP/MLLP (HL7 v2.x) Listener",
        Version:            "1.0.0",
        Category:           "inbound",
        Mode:               "push",
        ImplementationLang: "go",
        Capabilities: map[string]bool{
            "supports_cron":  false,
            "supports_tls":   true,
            "supports_auth":  true,
        },
    }

    base := NewBaseInboundConnector(metadata)
    return &TCPMLLPInboundConnector{
        BaseInboundConnector: base,
    }
}

// Initialize prepares the connector with its configuration
func (c *TCPMLLPInboundConnector) Initialize(config []byte) error {
    // Call base initialization
    if err := c.BaseInboundConnector.Initialize(config); err != nil {
        return err
    }

    // Parse connector-specific config
    cfg := c.GetConfig()
    c.port = cfg.GetInt("port")
    c.enableTLS = cfg.GetBool("enable_tls")

    // Setup TLS if enabled
    if c.enableTLS {
        c.tlsConfig = &tls.Config{
            MinVersion: tls.VersionTLS12,
        }
    }

    return nil
}

// Validate checks configuration validity
func (c *TCPMLLPInboundConnector) Validate() error {
    if err := c.BaseInboundConnector.Validate(); err != nil {
        return err
    }

    cfg := c.GetConfig()
    port := cfg.GetInt("port")
    if port <= 0 || port > 65535 {
        return fmt.Errorf("invalid port: %d", port)
    }

    return nil
}

// TestConnection verifies the port can be opened
func (c *TCPMLLPInboundConnector) TestConnection(ctx context.Context) error {
    addr := fmt.Sprintf(":%d", c.port)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        return NewConnectorError(c.metadata.TypeName, "test_connection", err, true)
    }
    defer listener.Close()

    return nil
}

// Start begins listening for MLLP connections
func (c *TCPMLLPInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
    // Check if already running
    if c.IsRunning() {
        return ErrConnectorAlreadyRunning
    }

    // Open listener
    addr := fmt.Sprintf(":%d", c.port)
    var err error

    if c.enableTLS {
        c.listener, err = tls.Listen("tcp", addr, c.tlsConfig)
    } else {
        c.listener, err = net.Listen("tcp", addr)
    }

    if err != nil {
        c.RecordError(err)
        return NewConnectorError(c.metadata.TypeName, "start", err, true)
    }

    c.SetState(StateRunning)
    c.SetConnected(true)

    // Accept connections in goroutine
    go c.acceptConnections(ctx, messageChan)

    return nil
}

// acceptConnections handles incoming connections
func (c *TCPMLLPInboundConnector) acceptConnections(ctx context.Context, messageChan chan<- *models.InboundMessage) {
    for {
        select {
        case <-ctx.Done():
            return
        case <-c.GetStopChannel():
            return
        default:
            conn, err := c.listener.Accept()
            if err != nil {
                c.RecordError(err)
                continue
            }

            // Handle connection in separate goroutine
            go c.handleConnection(conn, messageChan)
        }
    }
}

// handleConnection processes a single MLLP connection
func (c *TCPMLLPInboundConnector) handleConnection(conn net.Conn, messageChan chan<- *models.InboundMessage) {
    defer conn.Close()

    // Parse MLLP frame (0x0B...data...0x1C0x0D)
    // Extract HL7 message
    // Send to message channel
    // Send ACK back

    // TODO: Implement full MLLP protocol parsing

    c.IncrementMessagesReceived()
}

// Stop gracefully stops the listener
func (c *TCPMLLPInboundConnector) Stop() error {
    if !c.IsRunning() {
        return nil
    }

    if c.listener != nil {
        c.listener.Close()
    }

    c.SetState(StateStopped)
    c.SetConnected(false)

    return c.BaseInboundConnector.Stop()
}

// Close releases all resources
func (c *TCPMLLPInboundConnector) Close() error {
    c.Stop()
    return c.BaseInboundConnector.Close()
}
```

### Step 3: Update Stub Constructor

Replace the stub in `connector_stubs.go`:

```go
// OLD STUB:
func NewTCPMLLPInboundConnector() InboundConnector {
    metadata := ConnectorMetadata{...}
    return NewBaseInboundConnector(metadata)
}

// NEW (after creating tcp_mllp_inbound.go):
// Delete from connector_stubs.go - now implemented in tcp_mllp_inbound.go
```

### Step 4: Test the Connector

```go
// Example test
factory := connectors.GetFactory()
connector, err := factory.CreateInbound("tcp_mllp_inbound")

// Initialize with config
config := []byte(`{
    "port": 2575,
    "enable_tls": true,
    "tls_version": "TLS_1_2",
    "max_connections": 10
}`)

err = connector.Initialize(config)
err = connector.Validate()
err = connector.TestConnection(context.Background())

// Start listening
messageChan := make(chan *models.InboundMessage, 100)
err = connector.Start(context.Background(), messageChan)

// Receive messages
go func() {
    for msg := range messageChan {
        fmt.Printf("Received: %s\n", msg.Content)
    }
}()
```

---

## Implementation Priority

### Phase 2A: Critical Connectors (1-2 weeks)
1. **TCP/MLLP Inbound** - Most common HL7 v2.x source
2. **TCP/MLLP Outbound** - Middleware scenario support
3. **HTTP Outbound** - FHIR delivery to REST endpoints
4. **File Writer** - Local archiving and debugging

### Phase 2B: Database Connectors (2-3 weeks)
5. **PostgreSQL Inbound** - Polling EHR databases
6. **PostgreSQL Outbound** - Writing to data warehouse
7. **MongoDB Inbound** - NoSQL source systems
8. **MongoDB Outbound** - FHIR persistence

### Phase 2C: Cloud & Queue Connectors (2-3 weeks)
9. **AWS S3 Inbound** - Cloud file ingestion
10. **AWS S3 Outbound** - Cloud archival
11. **Kafka Producer** - Event streaming
12. **RabbitMQ Publisher** - Message queue delivery

### Phase 2D: Remaining Connectors (3-4 weeks)
13. All remaining database connectors (MySQL, SQL Server, Oracle)
14. All remaining cloud connectors (Azure, GCS)
15. File transfer connectors (SFTP, FTP)
16. Remaining message queue connectors (Kafka Consumer, Redis, RabbitMQ Consumer)

---

## Connector Implementation Checklist

For each connector, implement:

- [ ] **Configuration Parsing** - Extract all config parameters
- [ ] **Connection Management** - Open, test, close connections
- [ ] **Error Handling** - Wrap errors with ConnectorError
- [ ] **State Management** - Use SetState, SetConnected, etc.
- [ ] **Thread Safety** - Use base connector's mutex methods
- [ ] **Retry Logic** - For outbound connectors (configurable)
- [ ] **ACK/NACK Handling** - For message protocols (HL7, MLLP)
- [ ] **TLS/SSL Support** - If capability supported
- [ ] **Authentication** - Based on authentication_type config
- [ ] **Batch Operations** - For outbound connectors that support it
- [ ] **Cron Support** - For inbound pull-based connectors
- [ ] **Incremental Tracking** - For database readers
- [ ] **After-Processing** - For file/database readers (delete, move, archive, update)
- [ ] **Testing** - Unit tests and integration tests
- [ ] **Documentation** - Update connector catalog with examples

---

## Testing Strategy

### Unit Tests
```go
func TestTCPMLLPInboundConnector_Initialize(t *testing.T) {
    connector := NewTCPMLLPInboundConnector()
    config := []byte(`{"port": 2575, "enable_tls": false}`)

    err := connector.Initialize(config)
    assert.NoError(t, err)

    status := connector.GetStatus()
    assert.Equal(t, StateReady, status.State)
}
```

### Integration Tests
```go
func TestTCPMLLPInboundConnector_EndToEnd(t *testing.T) {
    // Start listener
    connector := NewTCPMLLPInboundConnector()
    config := []byte(`{"port": 12575, "enable_tls": false}`)
    connector.Initialize(config)

    messageChan := make(chan *models.InboundMessage, 1)
    ctx := context.Background()
    connector.Start(ctx, messageChan)

    // Send HL7 message via TCP
    conn, _ := net.Dial("tcp", "localhost:12575")
    conn.Write([]byte("\x0BMSH|^~\\&|...\x1C\x0D"))

    // Receive message
    msg := <-messageChan
    assert.NotNil(t, msg)

    connector.Stop()
}
```

---

## Current Status

✅ **Phase 1 Complete**: Database schema, models, services, API controller
✅ **Phase 2A Foundation Complete**: Interfaces, base classes, factory, stubs

🔄 **Phase 2B In Progress**: Implementing actual connector logic (32 connectors)

⏳ **Phase 3 Pending**: UI integration, monitoring dashboard, end-to-end testing

---

## Next Steps

1. **Implement TCP/MLLP Inbound Connector** - Critical for HL7 v2.x integration
2. **Implement TCP/MLLP Outbound Connector** - Enable middleware scenarios
3. **Implement HTTP Outbound Connector** - FHIR delivery to REST endpoints
4. **Create Connector Testing Framework** - Automated integration tests
5. **Begin Database Connector Implementation** - PostgreSQL inbound/outbound
6. **Integrate with Processing Engine** - Wire up connectors to message flow

---

**Last Updated**: October 26, 2025
**Phase**: 2B Foundation Complete
**Status**: Ready for connector implementation
**Total Connectors**: 32 (All stubs created, awaiting implementation)
