// services/connectors/base_connector.go
// Base connector implementation providing common functionality

package connectors

import (
	"context"
	"ezhealthkonnect/models"
	"encoding/json"
	"sync"
	"time"
)

// BaseConnector provides common functionality for all connectors
// Concrete connectors can embed this to inherit common behavior
type BaseConnector struct {
	metadata     ConnectorMetadata
	status       ConnectorStatus
	statusMu     sync.RWMutex
	config       ConnectorConfig
	initialized  bool
}

// NewBaseConnector creates a new base connector with common initialization
func NewBaseConnector(metadata ConnectorMetadata) *BaseConnector {
	return &BaseConnector{
		metadata: metadata,
		status: ConnectorStatus{
			State:            StateInitializing,
			Connected:        false,
			LastActivity:     time.Now(),
			MessagesSent:     0,
			MessagesReceived: 0,
			ErrorCount:       0,
			Metadata:         make(map[string]string),
		},
		initialized: false,
	}
}

// Initialize prepares the connector with its configuration
func (bc *BaseConnector) Initialize(config []byte) error {
	bc.statusMu.Lock()
	defer bc.statusMu.Unlock()

	// Parse configuration
	var configMap map[string]interface{}
	if err := json.Unmarshal(config, &configMap); err != nil {
		bc.updateStateUnsafe(StateError)
		bc.status.LastError = "Failed to parse configuration: " + err.Error()
		return err
	}

	bc.config = ConnectorConfig{
		TypeName: bc.metadata.TypeName,
		Config:   configMap,
	}

	bc.initialized = true
	bc.updateStateUnsafe(StateReady)
	return nil
}

// GetMetadata returns connector metadata
func (bc *BaseConnector) GetMetadata() ConnectorMetadata {
	return bc.metadata
}

// Validate checks if the connector configuration is valid
// This is a basic validation - concrete connectors should override for specific validation
func (bc *BaseConnector) Validate() error {
	bc.statusMu.RLock()
	defer bc.statusMu.RUnlock()

	if !bc.initialized {
		return NewConnectorError(bc.metadata.TypeName, "validate",
			ErrConnectorNotInitialized, false)
	}
	return nil
}

// TestConnection verifies connectivity without sending/receiving data
// This is a placeholder - concrete connectors must implement actual testing
func (bc *BaseConnector) TestConnection(ctx context.Context) error {
	return NewConnectorError(bc.metadata.TypeName, "test_connection",
		ErrNotImplemented, false)
}

// Close releases all resources
// This is a placeholder - concrete connectors must implement actual cleanup
func (bc *BaseConnector) Close() error {
	bc.statusMu.Lock()
	defer bc.statusMu.Unlock()

	bc.updateStateUnsafe(StateStopped)
	bc.status.Connected = false
	return nil
}

// GetStatus returns current connector status
func (bc *BaseConnector) GetStatus() ConnectorStatus {
	bc.statusMu.RLock()
	defer bc.statusMu.RUnlock()
	return bc.status
}

// GetConfig returns the connector configuration
func (bc *BaseConnector) GetConfig() ConnectorConfig {
	bc.statusMu.RLock()
	defer bc.statusMu.RUnlock()
	return bc.config
}

// UpdateStatus updates the connector status (thread-safe)
func (bc *BaseConnector) UpdateStatus(updateFn func(*ConnectorStatus)) {
	bc.statusMu.Lock()
	defer bc.statusMu.Unlock()
	updateFn(&bc.status)
	bc.status.LastActivity = time.Now()
}

// SetState updates the connector state
func (bc *BaseConnector) SetState(state ConnectorState) {
	bc.statusMu.Lock()
	defer bc.statusMu.Unlock()
	bc.updateStateUnsafe(state)
}

// SetConnected updates the connection status
func (bc *BaseConnector) SetConnected(connected bool) {
	bc.statusMu.Lock()
	defer bc.statusMu.Unlock()
	bc.status.Connected = connected
	bc.status.LastActivity = time.Now()
}

// IncrementMessagesSent increments the sent message counter
func (bc *BaseConnector) IncrementMessagesSent() {
	bc.statusMu.Lock()
	defer bc.statusMu.Unlock()
	bc.status.MessagesSent++
	bc.status.LastActivity = time.Now()
}

// IncrementMessagesReceived increments the received message counter
func (bc *BaseConnector) IncrementMessagesReceived() {
	bc.statusMu.Lock()
	defer bc.statusMu.Unlock()
	bc.status.MessagesReceived++
	bc.status.LastActivity = time.Now()
}

// RecordError records an error in the connector status
func (bc *BaseConnector) RecordError(err error) {
	bc.statusMu.Lock()
	defer bc.statusMu.Unlock()
	bc.status.ErrorCount++
	bc.status.LastError = err.Error()
	bc.status.LastActivity = time.Now()
}

// ClearError clears the last error
func (bc *BaseConnector) ClearError() {
	bc.statusMu.Lock()
	defer bc.statusMu.Unlock()
	bc.status.LastError = ""
}

// SetMetadata sets a metadata key-value pair
func (bc *BaseConnector) SetMetadata(key, value string) {
	bc.statusMu.Lock()
	defer bc.statusMu.Unlock()
	if bc.status.Metadata == nil {
		bc.status.Metadata = make(map[string]string)
	}
	bc.status.Metadata[key] = value
}

// GetMetadataValue gets a metadata value by key
func (bc *BaseConnector) GetMetadataValue(key string) string {
	bc.statusMu.RLock()
	defer bc.statusMu.RUnlock()
	return bc.status.Metadata[key]
}

// updateStateUnsafe updates state without locking (internal use only)
func (bc *BaseConnector) updateStateUnsafe(state ConnectorState) {
	bc.status.State = state
	bc.status.LastActivity = time.Now()
}

// IsInitialized returns whether the connector is initialized
func (bc *BaseConnector) IsInitialized() bool {
	bc.statusMu.RLock()
	defer bc.statusMu.RUnlock()
	return bc.initialized
}

// -----------------------------------------------------------------------------
// BaseInboundConnector - Base implementation for inbound connectors
// -----------------------------------------------------------------------------

type BaseInboundConnector struct {
	*BaseConnector
	isRunning bool
	runningMu sync.RWMutex
	stopCh    chan struct{}
}

// NewBaseInboundConnector creates a new base inbound connector
func NewBaseInboundConnector(metadata ConnectorMetadata) *BaseInboundConnector {
	return &BaseInboundConnector{
		BaseConnector: NewBaseConnector(metadata),
		isRunning:     false,
		stopCh:        make(chan struct{}),
	}
}

// Start begins listening/polling for messages
// This is a placeholder - concrete connectors must implement actual logic
func (bic *BaseInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	bic.runningMu.Lock()
	if bic.isRunning {
		bic.runningMu.Unlock()
		return NewConnectorError(bic.metadata.TypeName, "start",
			ErrConnectorAlreadyRunning, false)
	}
	bic.isRunning = true
	bic.runningMu.Unlock()

	bic.SetState(StateRunning)
	return NewConnectorError(bic.metadata.TypeName, "start",
		ErrNotImplemented, false)
}

// Stop gracefully stops the connector
func (bic *BaseInboundConnector) Stop() error {
	bic.runningMu.Lock()
	defer bic.runningMu.Unlock()

	if !bic.isRunning {
		return nil
	}

	close(bic.stopCh)
	bic.isRunning = false
	bic.SetState(StateStopped)
	return nil
}

// SupportsCron indicates if this connector supports scheduled polling
// Default is false - connectors that support cron should override this
func (bic *BaseInboundConnector) SupportsCron() bool {
	return bic.metadata.Capabilities["supports_cron"]
}

// IsRunning returns whether the connector is currently running
func (bic *BaseInboundConnector) IsRunning() bool {
	bic.runningMu.RLock()
	defer bic.runningMu.RUnlock()
	return bic.isRunning
}

// GetStopChannel returns the stop channel for graceful shutdown
func (bic *BaseInboundConnector) GetStopChannel() <-chan struct{} {
	return bic.stopCh
}

// -----------------------------------------------------------------------------
// BaseOutboundConnector - Base implementation for outbound connectors
// -----------------------------------------------------------------------------

type BaseOutboundConnector struct {
	*BaseConnector
	supportsBatch bool
}

// NewBaseOutboundConnector creates a new base outbound connector
func NewBaseOutboundConnector(metadata ConnectorMetadata, supportsBatch bool) *BaseOutboundConnector {
	return &BaseOutboundConnector{
		BaseConnector: NewBaseConnector(metadata),
		supportsBatch: supportsBatch,
	}
}

// Send delivers a message to the destination
// This is a placeholder - concrete connectors must implement actual logic
func (boc *BaseOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	return nil, NewConnectorError(boc.metadata.TypeName, "send",
		ErrNotImplemented, false)
}

// SendBatch delivers multiple messages in a single operation
// Default implementation sends messages individually - connectors can override for true batching
func (boc *BaseOutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	if !boc.supportsBatch {
		return nil, NewConnectorError(boc.metadata.TypeName, "send_batch",
			ErrBatchNotSupported, false)
	}

	results := make([]*DeliveryResult, len(messages))
	for i, msg := range messages {
		result, err := boc.Send(ctx, msg)
		if err != nil {
			results[i] = &DeliveryResult{
				Success:      false,
				MessageID:    msg.MessageID,
				Timestamp:    time.Now(),
				ErrorMessage: err.Error(),
			}
		} else {
			results[i] = result
		}
	}
	return results, nil
}

// SupportsBatch indicates if batch operations are supported
func (boc *BaseOutboundConnector) SupportsBatch() bool {
	return boc.supportsBatch
}

// -----------------------------------------------------------------------------
// Common errors
// -----------------------------------------------------------------------------

var (
	ErrConnectorNotInitialized  = &ConnectorError{Operation: "initialization", Err: ErrGeneric("connector not initialized"), Retryable: false}
	ErrConnectorAlreadyRunning  = &ConnectorError{Operation: "start", Err: ErrGeneric("connector already running"), Retryable: false}
	ErrNotImplemented           = &ConnectorError{Operation: "unimplemented", Err: ErrGeneric("method not implemented"), Retryable: false}
	ErrBatchNotSupported        = &ConnectorError{Operation: "batch", Err: ErrGeneric("batch operations not supported"), Retryable: false}
)

// ErrGeneric creates a generic error
type genericError struct {
	msg string
}

func (e *genericError) Error() string {
	return e.msg
}

func ErrGeneric(msg string) error {
	return &genericError{msg: msg}
}
