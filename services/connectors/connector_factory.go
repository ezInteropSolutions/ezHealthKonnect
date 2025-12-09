// services/connectors/connector_factory.go
// Factory pattern for creating connector instances

package connectors

import (
	"fmt"
	"sync"
)

// DefaultConnectorFactory is the default implementation of ConnectorFactory
type DefaultConnectorFactory struct {
	inboundRegistry  map[string]InboundConstructor
	outboundRegistry map[string]OutboundConstructor
	mu               sync.RWMutex
}

// NewConnectorFactory creates a new connector factory
func NewConnectorFactory() ConnectorFactory {
	factory := &DefaultConnectorFactory{
		inboundRegistry:  make(map[string]InboundConstructor),
		outboundRegistry: make(map[string]OutboundConstructor),
	}

	// Register all built-in connectors
	factory.registerBuiltInConnectors()

	return factory
}

// CreateInbound creates an inbound connector by type name
func (f *DefaultConnectorFactory) CreateInbound(typeName string) (InboundConnector, error) {
	f.mu.RLock()
	constructor, exists := f.inboundRegistry[typeName]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("inbound connector type '%s' not found", typeName)
	}

	return constructor(), nil
}

// CreateOutbound creates an outbound connector by type name
func (f *DefaultConnectorFactory) CreateOutbound(typeName string) (OutboundConnector, error) {
	f.mu.RLock()
	constructor, exists := f.outboundRegistry[typeName]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("outbound connector type '%s' not found", typeName)
	}

	return constructor(), nil
}

// GetSupportedTypes returns list of supported connector types
func (f *DefaultConnectorFactory) GetSupportedTypes() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	types := make([]string, 0, len(f.inboundRegistry)+len(f.outboundRegistry))

	for typeName := range f.inboundRegistry {
		types = append(types, typeName)
	}
	for typeName := range f.outboundRegistry {
		types = append(types, typeName)
	}

	return types
}

// RegisterInbound registers a new inbound connector implementation
func (f *DefaultConnectorFactory) RegisterInbound(typeName string, constructor InboundConstructor) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inboundRegistry[typeName] = constructor
}

// RegisterOutbound registers a new outbound connector implementation
func (f *DefaultConnectorFactory) RegisterOutbound(typeName string, constructor OutboundConstructor) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outboundRegistry[typeName] = constructor
}

// registerBuiltInConnectors registers all built-in connector implementations
func (f *DefaultConnectorFactory) registerBuiltInConnectors() {
	// Network Connectors
	f.RegisterInbound("tcp_mllp_inbound", NewTCPMLLPInboundConnector)
	f.RegisterOutbound("tcp_mllp_outbound", NewTCPMLLPOutboundConnector)
	f.RegisterInbound("http_rest_inbound", NewHTTPFHIRInboundConnector) // HTTP FHIR receiver
	f.RegisterInbound("http", NewHTTPFHIRInboundConnector)               // Alias for http connectivity
	f.RegisterOutbound("http_outbound", NewHTTPOutboundConnector)        // HTTP delivery (full implementation)
	f.RegisterOutbound("http", NewHTTPOutboundConnector)                 // Alias for http connectivity
	f.RegisterOutbound("http_rest", NewHTTPOutboundConnector)            // Alias for destination type compatibility

	// File System Connectors
	f.RegisterInbound("file_listener", NewFileListenerConnector)
	f.RegisterOutbound("file_writer", NewFileWriterConnector)

	// Database Connectors - PostgreSQL
	f.RegisterInbound("postgresql_inbound", NewPostgreSQLInboundConnector)
	f.RegisterOutbound("postgresql_outbound", NewPostgreSQLOutboundConnector)

	// Database Connectors - MySQL
	f.RegisterInbound("mysql_inbound", NewMySQLInboundConnector)
	f.RegisterOutbound("mysql_outbound", NewMySQLOutboundConnector)

	// Database Connectors - SQL Server
	f.RegisterInbound("sqlserver_inbound", NewSQLServerInboundConnector)
	f.RegisterOutbound("sqlserver_outbound", NewSQLServerOutboundConnector)

	// Database Connectors - MongoDB
	f.RegisterInbound("mongodb_inbound", NewMongoDBInboundConnector)
	f.RegisterOutbound("mongodb_outbound", NewMongoDBOutboundConnector)

	// Database Connectors - Oracle
	f.RegisterInbound("oracle_inbound", NewOracleInboundConnector)
	f.RegisterOutbound("oracle_outbound", NewOracleOutboundConnector)

	// Cloud Data Warehouse Connectors - Snowflake
	f.RegisterInbound("snowflake_inbound", NewSnowflakeInboundConnector)
	f.RegisterOutbound("snowflake_outbound", NewSnowflakeOutboundConnector)

	// Cloud Data Warehouse Connectors - Databricks
	f.RegisterInbound("databricks_inbound", NewDatabricksInboundConnector)
	f.RegisterOutbound("databricks_outbound", NewDatabricksOutboundConnector)

	// Cloud Data Warehouse Connectors - BigQuery
	f.RegisterInbound("bigquery_inbound", NewBigQueryInboundConnector)
	f.RegisterOutbound("bigquery_outbound", NewBigQueryOutboundConnector)

	// Cloud Data Warehouse Connectors - Redshift
	f.RegisterInbound("redshift_inbound", NewRedshiftInboundConnector)
	f.RegisterOutbound("redshift_outbound", NewRedshiftOutboundConnector)

	// Cloud Data Warehouse Connectors - Azure Synapse
	f.RegisterInbound("synapse_inbound", NewSynapseInboundConnector)
	f.RegisterOutbound("synapse_outbound", NewSynapseOutboundConnector)

	// Specialized Analytics Connectors - ClickHouse
	f.RegisterInbound("clickhouse_inbound", NewClickHouseInboundConnector)
	f.RegisterOutbound("clickhouse_outbound", NewClickHouseOutboundConnector)

	// Specialized Analytics Connectors - TimescaleDB (PostgreSQL-based)
	f.RegisterInbound("timescaledb_inbound", NewTimescaleDBInboundConnector)
	f.RegisterOutbound("timescaledb_outbound", NewTimescaleDBOutboundConnector)

	// Message Queue Connectors - RabbitMQ
	f.RegisterInbound("rabbitmq_inbound", NewRabbitMQInboundConnector)
	f.RegisterOutbound("rabbitmq_outbound", NewRabbitMQOutboundConnector)

	// Message Queue Connectors - Kafka
	f.RegisterInbound("kafka_inbound", NewKafkaInboundConnector)
	f.RegisterOutbound("kafka_outbound", NewKafkaOutboundConnector)

	// Message Queue Connectors - Redis
	f.RegisterInbound("redis_inbound", NewRedisInboundConnector)
	f.RegisterOutbound("redis_outbound", NewRedisOutboundConnector)

	// Cloud Storage Connectors - AWS S3
	f.RegisterInbound("aws_s3_inbound", NewAWSS3InboundConnector)
	f.RegisterOutbound("aws_s3_outbound", NewAWSS3OutboundConnector)

	// Cloud Storage Connectors - Azure Blob
	f.RegisterInbound("azure_blob_inbound", NewAzureBlobInboundConnector)
	f.RegisterOutbound("azure_blob_outbound", NewAzureBlobOutboundConnector)

	// Cloud Storage Connectors - Google Cloud Storage
	f.RegisterInbound("gcs_inbound", NewGCSInboundConnector)
	f.RegisterOutbound("gcs_outbound", NewGCSOutboundConnector)

	// File Transfer Connectors - SFTP
	f.RegisterInbound("sftp_inbound", NewSFTPInboundConnector)
	f.RegisterOutbound("sftp_outbound", NewSFTPOutboundConnector)

	// File Transfer Connectors - FTP
	f.RegisterInbound("ftp_inbound", NewFTPInboundConnector)
	f.RegisterOutbound("ftp_outbound", NewFTPOutboundConnector)
}

// -----------------------------------------------------------------------------
// Global factory instance
// -----------------------------------------------------------------------------

var (
	globalFactory     ConnectorFactory
	globalFactoryOnce sync.Once
)

// GetFactory returns the global connector factory instance
func GetFactory() ConnectorFactory {
	globalFactoryOnce.Do(func() {
		globalFactory = NewConnectorFactory()
	})
	return globalFactory
}

// RegisterCustomInbound registers a custom inbound connector with the global factory
func RegisterCustomInbound(typeName string, constructor InboundConstructor) {
	GetFactory().RegisterInbound(typeName, constructor)
}

// RegisterCustomOutbound registers a custom outbound connector with the global factory
func RegisterCustomOutbound(typeName string, constructor OutboundConstructor) {
	GetFactory().RegisterOutbound(typeName, constructor)
}
