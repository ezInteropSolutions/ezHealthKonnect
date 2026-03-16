// processing/connectors.go
// Unified Connector Factory - Enterprise OOB Pattern for ALL 32 Connectors
// Integrates services/connectors framework with processing engine

package processing

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/connectors"
	"fmt"
)

// ============================================================================
// UNIFIED ARCHITECTURE: Use New Framework Interfaces Directly
// ============================================================================

// InputConnector - Alias to new framework InboundConnector
// ALL inbound connectors (TCP, HTTP, File, DB, Queue, Cloud) use this interface
type InputConnector interface {
	Initialize(config []byte) error
	Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error
	Stop() error
	TestConnection(ctx context.Context) error
	Close() error
	GetStatus() connectors.ConnectorStatus
	GetMetadata() connectors.ConnectorMetadata
	Validate() error
	SupportsCron() bool
}

// OutputConnector - Alias to new framework OutboundConnector
// ALL outbound connectors (TCP, HTTP, File, DB, Queue, Cloud) use this interface
type OutputConnector interface {
	Initialize(config []byte) error
	Send(ctx context.Context, message *models.OutboundMessage) (*connectors.DeliveryResult, error)
	SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*connectors.DeliveryResult, error)
	TestConnection(ctx context.Context) error
	Close() error
	GetStatus() connectors.ConnectorStatus
	GetMetadata() connectors.ConnectorMetadata
	Validate() error
	SupportsBatch() bool
}

// ============================================================================
// OOB FACTORY PATTERN: Database-Driven Connector Creation
// Supports ALL 32 Connectors Without Code Changes
// ============================================================================

// CreateInputConnector creates an inbound connector using OOB factory
// Supports: TCP/MLLP, HTTP/REST, File, PostgreSQL, MySQL, SQL Server, MongoDB,
//           Oracle, RabbitMQ, Kafka, Redis, AWS S3, Azure Blob, GCS, SFTP, FTP
func CreateInputConnector(typeName string, config interface{}) (InputConnector, error) {
	// Convert config to JSON bytes
	var configJSON []byte
	var err error

	switch cfg := config.(type) {
	case []byte:
		configJSON = cfg
	case string:
		configJSON = []byte(cfg)
	case map[string]interface{}:
		configJSON, err = json.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config type: %T", config)
	}

	// OOB: Get connector from global factory (database-backed)
	factory := connectors.GetFactory()

	connector, err := factory.CreateInbound(typeName)
	if err != nil {
		return nil, fmt.Errorf("failed to create inbound connector '%s': %w", typeName, err)
	}

	// OOB: Initialize with schema-validated config
	err = connector.Initialize(configJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize connector '%s': %w", typeName, err)
	}

	// OOB: Validate configuration
	err = connector.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid connector config '%s': %w", typeName, err)
	}

	return connector, nil
}

// CreateOutputConnector creates an outbound connector using OOB factory
// Supports: TCP/MLLP, HTTP, File, PostgreSQL, MySQL, SQL Server, MongoDB,
//           Oracle, RabbitMQ, Kafka, Redis, AWS S3, Azure Blob, GCS, SFTP, FTP
func CreateOutputConnector(typeName string, config interface{}) (OutputConnector, error) {
	// Convert config to JSON bytes
	var configJSON []byte
	var err error

	switch cfg := config.(type) {
	case []byte:
		configJSON = cfg
	case string:
		configJSON = []byte(cfg)
	case map[string]interface{}:
		configJSON, err = json.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config type: %T", config)
	}

	// OOB: Get connector from global factory (database-backed)
	factory := connectors.GetFactory()

	connector, err := factory.CreateOutbound(typeName)
	if err != nil {
		return nil, fmt.Errorf("failed to create outbound connector '%s': %w", typeName, err)
	}

	// OOB: Initialize with schema-validated config
	err = connector.Initialize(configJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize connector '%s': %w", typeName, err)
	}

	// OOB: Validate configuration
	err = connector.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid connector config '%s': %w", typeName, err)
	}

	return connector, nil
}

// ============================================================================
// HELPER FUNCTIONS: Type Name Mapping (Backward Compatibility)
// ============================================================================

// MapLegacyConnectorType maps legacy connector type names to OOB type names
// Maintains backward compatibility with existing interface configurations
func MapLegacyConnectorType(legacyType string, direction string) string {
	// Map legacy names to new OOB type names
	if direction == "inbound" {
		switch legacyType {
		case "tcp", "hl7", "mllp", "tcp_mllp":
			return "tcp_mllp_inbound"
		case "http", "rest", "api", "fhir", "http_rest":
			return "http_rest_inbound"
		case "file", "filesystem":
			return "file_listener"
		case "database", "db", "postgresql", "postgres":
			return "postgresql_inbound"
		case "mysql":
			return "mysql_inbound"
		case "sqlserver", "mssql":
			return "sqlserver_inbound"
		case "mongodb", "mongo":
			return "mongodb_inbound"
		case "oracle":
			return "oracle_inbound"
		case "rabbitmq", "amqp":
			return "rabbitmq_inbound"
		case "kafka":
			return "kafka_inbound"
		case "redis":
			return "redis_inbound"
		case "s3", "aws_s3":
			return "aws_s3_inbound"
		case "azure_blob", "azure":
			return "azure_blob_inbound"
		case "gcs", "google_cloud_storage":
			return "gcs_inbound"
		case "sftp":
			return "sftp_inbound"
		case "ftp":
			return "ftp_inbound"
		}
	} else if direction == "outbound" {
		switch legacyType {
		case "tcp", "hl7", "mllp":
			return "tcp_mllp_outbound"
		case "http", "rest", "api":
			return "http_outbound"
		case "fhir", "fhir-server":
			return "http_outbound" // FHIR uses HTTP outbound with FHIR-specific headers
		case "file", "filesystem":
			return "file_writer"
		case "database", "db", "postgresql", "postgres":
			return "postgresql_outbound"
		case "mysql":
			return "mysql_outbound"
		case "sqlserver", "mssql":
			return "sqlserver_outbound"
		case "mongodb", "mongo":
			return "mongodb_outbound"
		case "oracle":
			return "oracle_outbound"
		case "rabbitmq", "amqp":
			return "rabbitmq_outbound"
		case "kafka":
			return "kafka_outbound"
		case "redis":
			return "redis_outbound"
		case "s3", "aws_s3":
			return "aws_s3_outbound"
		case "azure_blob", "azure":
			return "azure_blob_outbound"
		case "gcs", "google_cloud_storage":
			return "gcs_outbound"
		case "sftp":
			return "sftp_outbound"
		case "ftp":
			return "ftp_outbound"
		}
	}

	// If no mapping found, return as-is (assume already OOB name)
	return legacyType
}

// GetAvailableInputConnectors returns list of all available inbound connector types
// OOB: Dynamically retrieved from factory (database-backed)
func GetAvailableInputConnectors() []string {
	factory := connectors.GetFactory()
	allTypes := factory.GetSupportedTypes()

	// Filter for inbound connectors (end with _inbound or file_listener)
	var inboundTypes []string
	for _, typeName := range allTypes {
		if len(typeName) > 8 && typeName[len(typeName)-8:] == "_inbound" {
			inboundTypes = append(inboundTypes, typeName)
		} else if typeName == "file_listener" || typeName == "http_rest_inbound" {
			inboundTypes = append(inboundTypes, typeName)
		}
	}

	return inboundTypes
}

// GetAvailableOutputConnectors returns list of all available outbound connector types
// OOB: Dynamically retrieved from factory (database-backed)
func GetAvailableOutputConnectors() []string {
	factory := connectors.GetFactory()
	allTypes := factory.GetSupportedTypes()

	// Filter for outbound connectors (end with _outbound or file_writer)
	var outboundTypes []string
	for _, typeName := range allTypes {
		if len(typeName) > 9 && typeName[len(typeName)-9:] == "_outbound" {
			outboundTypes = append(outboundTypes, typeName)
		} else if typeName == "file_writer" || typeName == "http_outbound" {
			outboundTypes = append(outboundTypes, typeName)
		}
	}

	return outboundTypes
}

// ============================================================================
// LEGACY TYPE REMOVAL: Old Message and ConnectorStatus types removed
// Use models.InboundMessage, models.OutboundMessage, connectors.ConnectorStatus
// ============================================================================

// DEPRECATED: Old Message struct removed - use models.InboundMessage
// DEPRECATED: Old ProcessedMessage struct removed - use models.OutboundMessage
// DEPRECATED: Old ConnectorStatus struct removed - use connectors.ConnectorStatus

// Note: If you see compilation errors about missing types:
// - Replace "Message" with "*models.InboundMessage"
// - Replace "ProcessedMessage" with "*models.OutboundMessage"
// - Replace "ConnectorStatus" with "connectors.ConnectorStatus"
