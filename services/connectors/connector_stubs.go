// services/connectors/connector_stubs.go
// Stub implementations for all 32 OOB connectors
// These are minimal implementations - actual connector logic will be added in subsequent phases

package connectors

// -----------------------------------------------------------------------------
// Network Connectors (4)
// -----------------------------------------------------------------------------

// NewTCPMLLPInboundConnector - IMPLEMENTED in tcp_mllp_inbound.go
// Removed from stubs - full implementation available

// NewTCPMLLPOutboundConnector creates a TCP/MLLP outbound connector
func NewTCPMLLPOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "tcp_mllp_outbound",
		DisplayName:        "TCP/MLLP (HL7 v2.x) Client",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":  false,
			"supports_tls":    true,
			"supports_auth":   true,
			"supports_retry":  true,
			"validates_ack":   true,
		},
	}
	return NewBaseOutboundConnector(metadata, false)
}

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

// NewHTTPOutboundConnector creates an HTTP outbound connector
func NewHTTPOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "http_outbound",
		DisplayName:        "HTTP/HTTPS Endpoint",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": false,
			"supports_tls":   true,
			"supports_auth":  true,
			"supports_retry": true,
		},
	}
	return NewBaseOutboundConnector(metadata, false)
}

// -----------------------------------------------------------------------------
// File System Connectors (2)
// -----------------------------------------------------------------------------

// NewFileListenerConnector creates a file system listener connector
func NewFileListenerConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "file_listener",
		DisplayName:        "File System Listener",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_after_processing": true,
			"supports_patterns":         true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewFileWriterConnector creates a file system writer connector
func NewFileWriterConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "file_writer",
		DisplayName:        "File System Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":   true,
			"supports_append":  true,
			"supports_pattern": true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Database Connectors - PostgreSQL (2)
// -----------------------------------------------------------------------------

// NewPostgreSQLInboundConnector creates a PostgreSQL inbound connector
func NewPostgreSQLInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "postgresql_inbound",
		DisplayName:        "PostgreSQL Database Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_ssl":              true,
			"supports_incremental":      true,
			"supports_after_processing": true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewPostgreSQLOutboundConnector creates a PostgreSQL outbound connector
func NewPostgreSQLOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "postgresql_outbound",
		DisplayName:        "PostgreSQL Database Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":  true,
			"supports_ssl":    true,
			"supports_upsert": true,
			"supports_pool":   true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Database Connectors - MySQL (2)
// -----------------------------------------------------------------------------

// NewMySQLInboundConnector creates a MySQL inbound connector
func NewMySQLInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "mysql_inbound",
		DisplayName:        "MySQL Database Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_tls":              true,
			"supports_incremental":      true,
			"supports_after_processing": true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

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

// NewSQLServerInboundConnector creates a SQL Server inbound connector
func NewSQLServerInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "sqlserver_inbound",
		DisplayName:        "SQL Server Database Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_encryption":       true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_windows_auth":     true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewSQLServerOutboundConnector creates a SQL Server outbound connector
func NewSQLServerOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "sqlserver_outbound",
		DisplayName:        "SQL Server Database Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":        true,
			"supports_encryption":   true,
			"supports_merge":        true,
			"supports_pool":         true,
			"supports_windows_auth": true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Database Connectors - MongoDB (2)
// -----------------------------------------------------------------------------

// NewMongoDBInboundConnector creates a MongoDB inbound connector
func NewMongoDBInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "mongodb_inbound",
		DisplayName:        "MongoDB Database Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_tls":              true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_filters":          true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewMongoDBOutboundConnector creates a MongoDB outbound connector
func NewMongoDBOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "mongodb_outbound",
		DisplayName:        "MongoDB Database Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":  true,
			"supports_tls":    true,
			"supports_upsert": true,
			"supports_pool":   true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Database Connectors - Oracle (2)
// -----------------------------------------------------------------------------

// NewOracleInboundConnector creates an Oracle inbound connector
func NewOracleInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "oracle_inbound",
		DisplayName:        "Oracle Database Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_ssl":              true,
			"supports_incremental":      true,
			"supports_after_processing": true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewOracleOutboundConnector creates an Oracle outbound connector
func NewOracleOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "oracle_outbound",
		DisplayName:        "Oracle Database Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":  true,
			"supports_ssl":    true,
			"supports_merge":  true,
			"supports_pool":   true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

// -----------------------------------------------------------------------------
// Message Queue Connectors - RabbitMQ (2)
// -----------------------------------------------------------------------------

// NewRabbitMQInboundConnector creates a RabbitMQ inbound connector
func NewRabbitMQInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "rabbitmq_inbound",
		DisplayName:        "RabbitMQ Consumer",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":     false,
			"supports_tls":      true,
			"supports_prefetch": true,
			"supports_auto_ack": true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

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

// NewKafkaInboundConnector creates a Kafka inbound connector
func NewKafkaInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "kafka_inbound",
		DisplayName:        "Kafka Consumer",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":           false,
			"supports_sasl":           true,
			"supports_consumer_group": true,
			"supports_offset_mgmt":    true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

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

// NewRedisInboundConnector creates a Redis inbound connector
func NewRedisInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "redis_inbound",
		DisplayName:        "Redis Consumer",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":    false,
			"supports_tls":     true,
			"supports_pubsub":  true,
			"supports_streams": true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

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

// NewAWSS3InboundConnector creates an AWS S3 inbound connector
func NewAWSS3InboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "aws_s3_inbound",
		DisplayName:        "AWS S3 Bucket Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_iam_role":         true,
			"supports_after_processing": true,
			"supports_patterns":         true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

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

// NewSFTPInboundConnector creates an SFTP inbound connector
func NewSFTPInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "sftp_inbound",
		DisplayName:        "SFTP File Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_ssh_key":          true,
			"supports_after_processing": true,
			"supports_patterns":         true,
		},
	}
	return NewBaseInboundConnector(metadata)
}

// NewSFTPOutboundConnector creates an SFTP outbound connector
func NewSFTPOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "sftp_outbound",
		DisplayName:        "SFTP File Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":       true,
			"supports_ssh_key":     true,
			"supports_permissions": true,
		},
	}
	return NewBaseOutboundConnector(metadata, true)
}

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
