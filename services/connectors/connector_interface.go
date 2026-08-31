// services/connectors/connector_interface.go
// Universal Connector Interface - OOB Pattern for all connectivity types

package connectors

import (
	"context"
	"ezhealthkonnect/models"
	"strconv"
	"time"
)

// Connector is the universal interface that all connectors must implement
// Follows OOB pattern: Self-contained, configuration-driven, runtime-pluggable
type Connector interface {
	// Initialize prepares the connector with its configuration
	// Returns error if configuration is invalid or connection fails
	Initialize(config []byte) error

	// GetMetadata returns connector metadata (name, version, capabilities)
	GetMetadata() ConnectorMetadata

	// Validate checks if the connector configuration is valid
	Validate() error

	// TestConnection verifies connectivity without sending/receiving data
	TestConnection(ctx context.Context) error

	// Close releases all resources (connections, goroutines, etc.)
	Close() error

	// GetStatus returns current connector status
	GetStatus() ConnectorStatus

	// SetInterfaceContext attaches the owning interface's ID/name so the
	// connector's Logger() writes to that interface's own dated log file
	// (logs/interfaces/{interfaceID}/{yyyy}/{mm}/{dd}/interface.log) instead
	// of only the global application log. Called by the connector-creation
	// layer (processing.CreateInputConnector/CreateOutputConnector) after
	// Initialize succeeds — deliberately separate from Initialize, since most
	// concrete connectors override Initialize entirely and would otherwise
	// shadow a base-class implementation placed there. Implemented once on
	// BaseConnector and inherited by every connector through embedding, so no
	// per-connector-type code is needed.
	SetInterfaceContext(interfaceID, interfaceName string)
}

// InboundConnector interface for receiving messages
type InboundConnector interface {
	Connector

	// Start begins listening/polling for messages
	// Messages are sent to the provided channel
	// Context cancellation stops the connector
	Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error

	// Stop gracefully stops the connector
	Stop() error

	// SupportsCron indicates if this connector supports scheduled polling
	SupportsCron() bool
}

// OutboundConnector interface for sending messages
type OutboundConnector interface {
	Connector

	// Send delivers a message to the destination
	// Returns delivery result with acknowledgment details
	Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error)

	// SendBatch delivers multiple messages in a single operation
	// Returns results for each message
	SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error)

	// SupportsBatch indicates if batch operations are supported
	SupportsBatch() bool
}

// ValidationAwareConnector interface for connectors that can send validation feedback
// This allows connectors to respond appropriately to validation results:
// - TCP/MLLP: Send ACK (AA) or NACK (AE)
// - HTTP REST: Send HTTP 200/202/400 status codes
// - File: Write .error files
// - Database: Update validation_status column
// - Kafka/RabbitMQ: Publish to DLQ/error topic
type ValidationAwareConnector interface {
	// SendValidationResponse sends appropriate response based on validation result
	SendValidationResponse(ctx context.Context, feedback *models.ValidationFeedback) error

	// SupportsValidationFeedback indicates if connector can send validation responses
	SupportsValidationFeedback() bool
}

// ConnectorMetadata contains static information about a connector
type ConnectorMetadata struct {
	TypeName           string            `json:"type_name"`
	DisplayName        string            `json:"display_name"`
	Version            string            `json:"version"`
	Category           string            `json:"category"` // inbound, outbound
	Mode               string            `json:"mode"`     // push, pull
	ImplementationLang string            `json:"implementation_lang"`
	Capabilities       map[string]bool   `json:"capabilities"`
	ConfigSchema       map[string]string `json:"config_schema"`
}

// ConnectorStatus represents current operational state
type ConnectorStatus struct {
	State            ConnectorState     `json:"state"`
	Connected        bool               `json:"connected"`
	LastActivity     time.Time          `json:"last_activity"`
	MessagesSent     int64              `json:"messages_sent"`
	MessagesReceived int64              `json:"messages_received"`
	ErrorCount       int64              `json:"error_count"`
	LastError        string             `json:"last_error,omitempty"`
	Metadata         map[string]string  `json:"metadata,omitempty"`
}

// ConnectorState enum for connector lifecycle
type ConnectorState string

const (
	StateInitializing ConnectorState = "initializing"
	StateReady        ConnectorState = "ready"
	StateRunning      ConnectorState = "running"
	StateStopped      ConnectorState = "stopped"
	StateError        ConnectorState = "error"
)

// DeliveryResult contains the result of a message delivery
type DeliveryResult struct {
	Success        bool                   `json:"success"`
	MessageID      string                 `json:"message_id"`
	Timestamp      time.Time              `json:"timestamp"`
	Acknowledgment string                 `json:"acknowledgment,omitempty"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	RetryCount     int                    `json:"retry_count"`
	DurationMs     int64                  `json:"duration_ms"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// ConnectorFactory creates connector instances based on type
type ConnectorFactory interface {
	// CreateInbound creates an inbound connector by type name
	CreateInbound(typeName string) (InboundConnector, error)

	// CreateOutbound creates an outbound connector by type name
	CreateOutbound(typeName string) (OutboundConnector, error)

	// GetSupportedTypes returns list of supported connector types
	GetSupportedTypes() []string

	// RegisterInbound registers a new inbound connector implementation
	RegisterInbound(typeName string, constructor InboundConstructor)

	// RegisterOutbound registers a new outbound connector implementation
	RegisterOutbound(typeName string, constructor OutboundConstructor)
}

// Constructor function types
type InboundConstructor func() InboundConnector
type OutboundConstructor func() OutboundConnector

// ConnectorConfig wraps configuration with helper methods
type ConnectorConfig struct {
	TypeName string
	Config   map[string]interface{}
}

// GetString safely retrieves a string value from config
func (c *ConnectorConfig) GetString(key string) string {
	if val, ok := c.Config[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// GetInt safely retrieves an int value from config.
// Handles JSON number (float64), Go int, and quoted-string forms (e.g. "6612").
func (c *ConnectorConfig) GetInt(key string) int {
	if val, ok := c.Config[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
	}
	return 0
}

// GetBool safely retrieves a bool value from config
func (c *ConnectorConfig) GetBool(key string) bool {
	if val, ok := c.Config[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// GetStringSlice safely retrieves a []string from config
func (c *ConnectorConfig) GetStringSlice(key string) []string {
	if val, ok := c.Config[key]; ok {
		if slice, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return []string{}
}

// Has checks if a configuration key exists
func (c *ConnectorConfig) Has(key string) bool {
	_, ok := c.Config[key]
	return ok
}

// GetBoolDefault retrieves a bool value, returning defaultValue when the key
// is absent. Use this instead of GetBool() whenever the zero value (false) is
// not the intended default — e.g. enable_tls defaults to true.
func (c *ConnectorConfig) GetBoolDefault(key string, defaultValue bool) bool {
	if !c.Has(key) {
		return defaultValue
	}
	return c.GetBool(key)
}

// GetMap safely retrieves a map[string]interface{} value from config
func (c *ConnectorConfig) GetMap(key string) map[string]interface{} {
	if val, ok := c.Config[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

// ConnectorError represents connector-specific errors
type ConnectorError struct {
	ConnectorType string
	Operation     string
	Err           error
	Retryable     bool
}

func (e *ConnectorError) Error() string {
	return e.Err.Error()
}

func (e *ConnectorError) Unwrap() error {
	return e.Err
}

// NewConnectorError creates a new connector error
func NewConnectorError(connectorType, operation string, err error, retryable bool) *ConnectorError {
	return &ConnectorError{
		ConnectorType: connectorType,
		Operation:     operation,
		Err:           err,
		Retryable:     retryable,
	}
}
