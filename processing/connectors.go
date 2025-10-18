// processing/connectors.go
// Connector interfaces and factory for Go processing engine

package processing

import (
	"context"
	"fmt"
	"time"
)

// InputConnector interface for receiving messages
type InputConnector interface {
	// Start begins listening for messages
	Start(ctx context.Context, messageChan chan<- Message) error

	// TestConnection tests the connection to the source
	TestConnection() error

	// Close cleans up the connector
	Close() error

	// GetStatus returns connector status
	GetStatus() ConnectorStatus
}

// OutputConnector interface for sending processed messages
type OutputConnector interface {
	// Send delivers a processed message
	Send(ctx context.Context, message ProcessedMessage) error

	// TestConnection tests the connection to the target
	TestConnection() error

	// Close cleans up the connector
	Close() error

	// GetStatus returns connector status
	GetStatus() ConnectorStatus
}

// Message represents an incoming message
type Message struct {
	ID          string                 `json:"id"`
	Content     string                 `json:"content"`
	Type        string                 `json:"type"`
	Source      string                 `json:"source"`
	Timestamp   string                 `json:"timestamp"`
	ReceivedAt  time.Time              `json:"received_at"`
	Size        int                    `json:"size"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ProcessedMessage represents a processed message ready for output
type ProcessedMessage struct {
	OriginalID     string                 `json:"original_id"`
	ProcessedID    string                 `json:"processed_id"`
	Content        string                 `json:"content"`
	Type           string                 `json:"type"`
	Transformation string                 `json:"transformation"`
	Metadata       map[string]interface{} `json:"metadata"`
	ProcessingTime int64                  `json:"processing_time_ms"`
}

// ConnectorStatus represents connector status information
type ConnectorStatus struct {
	Type         string `json:"type"`
	Status       string `json:"status"`
	LastActivity string `json:"last_activity"`
	MessageCount int64  `json:"message_count"`
	ErrorCount   int64  `json:"error_count"`
}

// CreateInputConnector creates an input connector based on type and configuration
func CreateInputConnector(connectorType string, config map[string]interface{}) (InputConnector, error) {
	switch connectorType {
	case "tcp", "hl7":
		return NewTCPInputConnector(config)
	case "file", "filesystem":
		return NewFileInputConnector(config)
	case "database", "db":
		return NewDatabaseInputConnector(config)
	case "http", "rest", "api", "fhir":
		return NewHTTPInputConnector(config)
	default:
		return nil, fmt.Errorf("unsupported input connector type: %s", connectorType)
	}
}

// CreateOutputConnector creates an output connector based on type and configuration
func CreateOutputConnector(connectorType string, config map[string]interface{}) (OutputConnector, error) {
	switch connectorType {
	case "tcp", "hl7":
		return NewTCPOutputConnector(config)
	case "file", "filesystem":
		return NewFileOutputConnector(config)
	case "database", "db":
		return NewDatabaseOutputConnector(config)
	case "http", "rest", "api":
		return NewRESTOutputConnector(config)
	case "fhir", "fhir-server":
		return NewFHIROutputConnector(config)
	default:
		return nil, fmt.Errorf("unsupported output connector type: %s", connectorType)
	}
}