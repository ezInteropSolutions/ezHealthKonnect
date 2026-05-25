// services/connectors/connector_stubs.go
// Stub implementations for all 32 OOB connectors
// These are minimal implementations - actual connector logic will be added in subsequent phases

package connectors

// -----------------------------------------------------------------------------
// Network Connectors (4)
// -----------------------------------------------------------------------------

// NewTCPMLLPInboundConnector - IMPLEMENTED in tcp_mllp_inbound.go
// Removed from stubs - full implementation available

// NewTCPMLLPOutboundConnector — IMPLEMENTED in tcp_mllp_outbound.go
// See services/connectors/tcp_mllp_outbound.go for the full TCP/MLLP outbound connector.

// NewHTTPRESTInboundConnector creates an HTTP REST API inbound connector
func NewHTTPRESTInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "http_rest_inbound",
		DisplayName:        "HTTP/REST API Endpoint",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron": false,
			"supports_tls":  true,
			"supports_auth": true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewHTTPOutboundConnector - MOVED to http_outbound.go (full implementation)
// See services/connectors/http_outbound.go for the complete HTTP outbound connector

// -----------------------------------------------------------------------------
// File System Connectors (2)
// -----------------------------------------------------------------------------

// NewFileListenerConnector - MOVED to file_listener.go (full implementation)
// See services/connectors/file_listener.go for the complete file listener connector

// NewFileWriterConnector - MOVED to file_writer.go (full implementation)
// See services/connectors/file_writer.go for the complete file writer connector

// -----------------------------------------------------------------------------
// Database Connectors - PostgreSQL (2)
// -----------------------------------------------------------------------------

// NewPostgreSQLInboundConnector - IMPLEMENTED in postgresql_inbound.go
// See services/connectors/postgresql_inbound.go for the complete PostgreSQL inbound connector

// NewPostgreSQLOutboundConnector - IMPLEMENTED in postgresql_outbound.go
// See services/connectors/postgresql_outbound.go for the complete PostgreSQL outbound connector

// -----------------------------------------------------------------------------
// Database Connectors - MySQL (2)
// -----------------------------------------------------------------------------

// NewMySQLInboundConnector — IMPLEMENTED in mysql_inbound.go

// NewMySQLOutboundConnector creates a MySQL outbound connector
func NewMySQLOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "mysql_outbound",
		DisplayName:        "MySQL Database Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":   true,
			"supports_tls":     true,
			"supports_replace": true,
			"supports_pool":    true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Database Connectors - SQL Server (2)
// -----------------------------------------------------------------------------

// NewSQLServerInboundConnector — IMPLEMENTED in sqlserver_inbound.go

// NewSQLServerOutboundConnector — IMPLEMENTED in sqlserver_outbound.go
// Supports INSERT and MERGE (upsert) write modes with @pN parameter binding.

// -----------------------------------------------------------------------------
// Database Connectors - MongoDB (2)
// -----------------------------------------------------------------------------

// NewMongoDBInboundConnector — IMPLEMENTED in mongodb_inbound.go

// NewMongoDBOutboundConnector — IMPLEMENTED in mongodb_outbound.go
// See services/connectors/mongodb_outbound.go for the full MongoDB outbound connector.

// -----------------------------------------------------------------------------
// Database Connectors - Oracle (2)
// -----------------------------------------------------------------------------

// NewOracleInboundConnector — IMPLEMENTED in oracle_inbound.go

// NewOracleOutboundConnector — IMPLEMENTED in oracle_outbound.go
// Supports INSERT and MERGE INTO (upsert) write modes with :N positional binding.

// -----------------------------------------------------------------------------
// Cloud Data Warehouse Connectors - Snowflake (2)
// -----------------------------------------------------------------------------

// NewSnowflakeInboundConnector creates a Snowflake inbound connector
func NewSnowflakeInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "snowflake_inbound",
		DisplayName:        "Snowflake Data Warehouse Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_oauth":            true,
			"supports_key_pair_auth":    true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_warehouse_mgmt":   true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewSnowflakeOutboundConnector creates a Snowflake outbound connector
func NewSnowflakeOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "snowflake_outbound",
		DisplayName:        "Snowflake Data Warehouse Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":           true,
			"supports_oauth":           true,
			"supports_key_pair_auth":   true,
			"supports_merge":           true,
			"supports_pool":            true,
			"supports_warehouse_mgmt":  true,
			"supports_stage_copy":      true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Cloud Data Warehouse Connectors - Databricks (2)
// -----------------------------------------------------------------------------

// NewDatabricksInboundConnector creates a Databricks inbound connector
func NewDatabricksInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "databricks_inbound",
		DisplayName:        "Databricks SQL Warehouse Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_pat_auth":         true,
			"supports_oauth":            true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_delta_lake":       true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewDatabricksOutboundConnector creates a Databricks outbound connector
func NewDatabricksOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "databricks_outbound",
		DisplayName:        "Databricks SQL Warehouse Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":       true,
			"supports_pat_auth":    true,
			"supports_oauth":       true,
			"supports_merge":       true,
			"supports_pool":        true,
			"supports_delta_lake":  true,
			"supports_unity_cat":   true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Cloud Data Warehouse Connectors - BigQuery (2)
// -----------------------------------------------------------------------------

// NewBigQueryInboundConnector creates a BigQuery inbound connector
func NewBigQueryInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "bigquery_inbound",
		DisplayName:        "Google BigQuery Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_service_account":  true,
			"supports_oauth":            true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_standard_sql":     true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewBigQueryOutboundConnector creates a BigQuery outbound connector
func NewBigQueryOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "bigquery_outbound",
		DisplayName:        "Google BigQuery Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":          true,
			"supports_service_account": true,
			"supports_oauth":          true,
			"supports_merge":          true,
			"supports_streaming":      true,
			"supports_standard_sql":   true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Cloud Data Warehouse Connectors - Redshift (2)
// -----------------------------------------------------------------------------

// NewRedshiftInboundConnector creates a Redshift inbound connector
func NewRedshiftInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "redshift_inbound",
		DisplayName:        "AWS Redshift Data Warehouse Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_ssl":              true,
			"supports_iam_auth":         true,
			"supports_incremental":      true,
			"supports_after_processing": true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewRedshiftOutboundConnector creates a Redshift outbound connector
func NewRedshiftOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "redshift_outbound",
		DisplayName:        "AWS Redshift Data Warehouse Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":    true,
			"supports_ssl":      true,
			"supports_iam_auth": true,
			"supports_merge":    true,
			"supports_pool":     true,
			"supports_s3_copy":  true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Cloud Data Warehouse Connectors - Azure Synapse (2)
// -----------------------------------------------------------------------------

// NewSynapseInboundConnector creates an Azure Synapse inbound connector
func NewSynapseInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "synapse_inbound",
		DisplayName:        "Azure Synapse Analytics Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_encryption":       true,
			"supports_azure_ad_auth":    true,
			"supports_incremental":      true,
			"supports_after_processing": true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewSynapseOutboundConnector creates an Azure Synapse outbound connector
func NewSynapseOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "synapse_outbound",
		DisplayName:        "Azure Synapse Analytics Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":         true,
			"supports_encryption":    true,
			"supports_azure_ad_auth": true,
			"supports_merge":         true,
			"supports_pool":          true,
			"supports_polybase":      true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Specialized Analytics Connectors - ClickHouse (2)
// -----------------------------------------------------------------------------

// NewClickHouseInboundConnector creates a ClickHouse inbound connector
func NewClickHouseInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "clickhouse_inbound",
		DisplayName:        "ClickHouse OLAP Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_https":            true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_columnar":         true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewClickHouseOutboundConnector creates a ClickHouse outbound connector
func NewClickHouseOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "clickhouse_outbound",
		DisplayName:        "ClickHouse OLAP Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":         true,
			"supports_https":         true,
			"supports_merge":         true,
			"supports_pool":          true,
			"supports_columnar":      true,
			"supports_merge_tree":    true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Specialized Analytics Connectors - TimescaleDB (2)
// -----------------------------------------------------------------------------

// NewTimescaleDBInboundConnector creates a TimescaleDB inbound connector
func NewTimescaleDBInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "timescaledb_inbound",
		DisplayName:        "TimescaleDB Time-Series Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_ssl":              true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_hypertables":      true,
			"supports_time_bucketing":   true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewTimescaleDBOutboundConnector creates a TimescaleDB outbound connector
func NewTimescaleDBOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "timescaledb_outbound",
		DisplayName:        "TimescaleDB Time-Series Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":       true,
			"supports_ssl":         true,
			"supports_upsert":      true,
			"supports_pool":        true,
			"supports_hypertables": true,
			"supports_compression": true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Message Queue Connectors - RabbitMQ (2)
// -----------------------------------------------------------------------------

// NewRabbitMQInboundConnector — IMPLEMENTED in rabbitmq_inbound.go

// NewRabbitMQOutboundConnector creates a RabbitMQ outbound connector
func NewRabbitMQOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "rabbitmq_outbound",
		DisplayName:        "RabbitMQ Publisher",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":      true,
			"supports_tls":        true,
			"supports_persistent": true,
			"supports_confirm":    true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Message Queue Connectors - Kafka (2)
// -----------------------------------------------------------------------------

// NewKafkaInboundConnector — IMPLEMENTED in kafka_inbound.go

// NewKafkaOutboundConnector creates a Kafka outbound connector
func NewKafkaOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "kafka_outbound",
		DisplayName:        "Kafka Producer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":       true,
			"supports_sasl":        true,
			"supports_compression": true,
			"supports_acks":        true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Message Queue Connectors - Redis (2)
// -----------------------------------------------------------------------------

// NewRedisInboundConnector — IMPLEMENTED in redis_inbound.go

// NewRedisOutboundConnector creates a Redis outbound connector
func NewRedisOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "redis_outbound",
		DisplayName:        "Redis Publisher",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":   true,
			"supports_tls":     true,
			"supports_pubsub":  true,
			"supports_streams": true,
			"supports_expiry":  true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Cloud Storage Connectors - AWS S3 (2)
// -----------------------------------------------------------------------------

// NewAWSS3InboundConnector — IMPLEMENTED in aws_s3_inbound.go

// NewAWSS3OutboundConnector creates an AWS S3 outbound connector
func NewAWSS3OutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "aws_s3_outbound",
		DisplayName:        "AWS S3 Bucket Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":      true,
			"supports_iam_role":   true,
			"supports_encryption": true,
			"supports_kms":        true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Cloud Storage Connectors - Azure Blob (2)
// -----------------------------------------------------------------------------

// NewAzureBlobInboundConnector creates an Azure Blob inbound connector
func NewAzureBlobInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "azure_blob_inbound",
		DisplayName:        "Azure Blob Storage Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_after_processing": true,
			"supports_patterns":         true,
			"supports_https":            true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewAzureBlobOutboundConnector creates an Azure Blob outbound connector
func NewAzureBlobOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "azure_blob_outbound",
		DisplayName:        "Azure Blob Storage Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":        true,
			"supports_https":        true,
			"supports_access_tiers": true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Cloud Storage Connectors - Google Cloud Storage (2)
// -----------------------------------------------------------------------------

// NewGCSInboundConnector creates a Google Cloud Storage inbound connector
func NewGCSInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "gcs_inbound",
		DisplayName:        "Google Cloud Storage Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_service_account":  true,
			"supports_after_processing": true,
			"supports_patterns":         true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewGCSOutboundConnector creates a Google Cloud Storage outbound connector
func NewGCSOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "gcs_outbound",
		DisplayName:        "Google Cloud Storage Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":           true,
			"supports_service_account": true,
			"supports_encryption":      true,
			"supports_storage_class":   true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// File Transfer Connectors - SFTP (2)
// -----------------------------------------------------------------------------

// NewSFTPInboundConnector — IMPLEMENTED in sftp_inbound.go
// See services/connectors/sftp_inbound.go for the full SFTP inbound connector.

// NewSFTPOutboundConnector — IMPLEMENTED in sftp_outbound.go
// See services/connectors/sftp_outbound.go for the full SFTP outbound connector.

// -----------------------------------------------------------------------------
// File Transfer Connectors - FTP (2)
// -----------------------------------------------------------------------------

// NewFTPInboundConnector creates an FTP inbound connector
func NewFTPInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "ftp_inbound",
		DisplayName:        "FTP File Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_ftps":             true,
			"supports_after_processing": true,
			"supports_patterns":         true,
			"supports_passive":          true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewFTPOutboundConnector creates an FTP outbound connector
func NewFTPOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "ftp_outbound",
		DisplayName:        "FTP File Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":   true,
			"supports_ftps":    true,
			"supports_passive": true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// EMR / Healthcare Protocol Connectors (stubs — Phase 4 implementation)
// -----------------------------------------------------------------------------

// NewFHIRR4InboundConnector is retained for any code that references the constructor
// directly, but the factory now maps "fhir_r4_inbound" → NewHTTPFHIRInboundConnector.
// This stub is kept so it can be used as a pull-mode FHIR poller in a future sprint.
func NewFHIRR4InboundConnector() InboundConnector {
	// Pull-mode FHIR polling (querying a FHIR server on a schedule) is not yet
	// implemented. The factory maps fhir_r4_inbound to the push-mode HTTP FHIR
	// Receiver instead. When pull-mode polling is built, update the factory entry.
	panic("NewFHIRR4InboundConnector: pull-mode FHIR polling not yet implemented — " +
		"use 'http_fhir_inbound' for push mode or 'fhir_r4_inbound' (routed to http_fhir_inbound via factory)")
}

// NewFHIRR4OutboundConnector is retained for any code that references the constructor
// directly, but the factory now maps "fhir_r4_outbound" → NewHTTPFHIROutboundConnector.
func NewFHIRR4OutboundConnector() OutboundConnector {
	panic("NewFHIRR4OutboundConnector: use 'http_fhir_outbound' or 'fhir_r4_outbound' — " +
		"both are routed to the full HTTPFHIROutboundConnector via factory")
}

// NewEDIX12InboundConnector creates an EDI X12 inbound connector (stub)
func NewEDIX12InboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "edi_x12_inbound",
		DisplayName:        "EDI X12 Inbound",
		Version:            "0.9.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":    true,
			"supports_sftp":    true,
			"supports_http":    true,
			"supports_as2":     true,
			"supports_999_ack": true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewEDIX12OutboundConnector creates an EDI X12 outbound connector (stub)
func NewEDIX12OutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "edi_x12_outbound",
		DisplayName:        "EDI X12 Outbound",
		Version:            "0.9.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": true,
			"supports_sftp":  true,
			"supports_http":  true,
			"supports_as2":   true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// NewDirectMessagingInboundConnector creates a DirectTrust SMTP+SMIME inbound connector (stub)
func NewDirectMessagingInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "direct_messaging_inbound",
		DisplayName:        "Direct Messaging Inbound",
		Version:            "0.9.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":  true,
			"supports_smime": true,
			"supports_cda":   true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewDirectMessagingOutboundConnector creates a DirectTrust SMTP+SMIME outbound connector (stub)
func NewDirectMessagingOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "direct_messaging_outbound",
		DisplayName:        "Direct Messaging Outbound",
		Version:            "0.9.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": false,
			"supports_smime": true,
			"supports_cda":   true,
		},
	}
	return NewBaseOutboundConnector(metadata, false)
}
