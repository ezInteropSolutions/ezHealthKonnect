-- V28__Message_Queue_And_Cloud_Connectors.sql
-- Add Message Queue Connectors (RabbitMQ, Kafka, Redis)
-- Add Cloud Storage Connectors (Azure Blob, Google Cloud Storage)
-- Add SFTP/FTP Connectors

-- ========================================
-- MESSAGE QUEUE CONNECTORS
-- ========================================

-- RabbitMQ Connector (Inbound - Consumer)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'rabbitmq_inbound',
    'inbound',
    'RabbitMQ Consumer',
    'Consume messages from RabbitMQ queue',
    '🐰',
    'push',
    false,
    true,
    false,
    'RabbitMQInboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "queue_name"],
        "properties": {
            "host": {
                "type": "string",
                "title": "RabbitMQ Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Port",
                "default": 5672
            },
            "username": {
                "type": "string",
                "title": "Username",
                "default": "guest"
            },
            "password": {
                "type": "string",
                "title": "Password",
                "format": "password",
                "default": "guest"
            },
            "vhost": {
                "type": "string",
                "title": "Virtual Host",
                "default": "/"
            },
            "queue_name": {
                "type": "string",
                "title": "Queue Name"
            },
            "exchange_name": {
                "type": "string",
                "title": "Exchange Name"
            },
            "routing_key": {
                "type": "string",
                "title": "Routing Key"
            },
            "prefetch_count": {
                "type": "integer",
                "title": "Prefetch Count",
                "description": "Number of messages to prefetch",
                "default": 1
            },
            "auto_ack": {
                "type": "boolean",
                "title": "Auto Acknowledge",
                "default": false
            },
            "durable_queue": {
                "type": "boolean",
                "title": "Durable Queue",
                "default": true
            },
            "enable_tls": {
                "type": "boolean",
                "title": "Enable TLS/SSL",
                "default": false
            },
            "connection_timeout": {
                "type": "integer",
                "title": "Connection Timeout (seconds)",
                "default": 30
            }
        }
    }',
    '{
        "basic": ["host", "port", "username", "password", "vhost"],
        "queue": ["queue_name", "exchange_name", "routing_key"],
        "consumer": ["prefetch_count", "auto_ack", "durable_queue"],
        "advanced": ["enable_tls", "connection_timeout"]
    }',
    true,
    false,
    60,
    '1.0'
);

-- RabbitMQ Connector (Outbound - Publisher)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'rabbitmq_outbound',
    'outbound',
    'RabbitMQ Publisher',
    'Publish messages to RabbitMQ exchange/queue',
    '🐰',
    'push',
    false,
    true,
    false,
    'RabbitMQOutboundConnector',
    '{
        "type": "object",
        "required": ["host", "port"],
        "properties": {
            "host": {
                "type": "string",
                "title": "RabbitMQ Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Port",
                "default": 5672
            },
            "username": {
                "type": "string",
                "title": "Username",
                "default": "guest"
            },
            "password": {
                "type": "string",
                "title": "Password",
                "format": "password",
                "default": "guest"
            },
            "vhost": {
                "type": "string",
                "title": "Virtual Host",
                "default": "/"
            },
            "exchange_name": {
                "type": "string",
                "title": "Exchange Name"
            },
            "exchange_type": {
                "type": "string",
                "title": "Exchange Type",
                "enum": ["direct", "topic", "fanout", "headers"],
                "default": "direct"
            },
            "routing_key": {
                "type": "string",
                "title": "Routing Key"
            },
            "queue_name": {
                "type": "string",
                "title": "Queue Name (optional)"
            },
            "persistent": {
                "type": "boolean",
                "title": "Persistent Messages",
                "default": true
            },
            "mandatory": {
                "type": "boolean",
                "title": "Mandatory Routing",
                "default": false
            },
            "enable_tls": {
                "type": "boolean",
                "title": "Enable TLS/SSL",
                "default": false
            },
            "connection_pool_size": {
                "type": "integer",
                "title": "Connection Pool Size",
                "default": 5
            }
        }
    }',
    '{
        "basic": ["host", "port", "username", "password", "vhost"],
        "routing": ["exchange_name", "exchange_type", "routing_key", "queue_name"],
        "delivery": ["persistent", "mandatory"],
        "advanced": ["enable_tls", "connection_pool_size"]
    }',
    true,
    false,
    60,
    '1.0'
);

-- Apache Kafka Connector (Inbound - Consumer)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'kafka_inbound',
    'inbound',
    'Kafka Consumer',
    'Consume messages from Kafka topic',
    '⚡',
    'push',
    false,
    true,
    false,
    'KafkaInboundConnector',
    '{
        "type": "object",
        "required": ["bootstrap_servers", "topic", "group_id"],
        "properties": {
            "bootstrap_servers": {
                "type": "string",
                "title": "Bootstrap Servers",
                "description": "Comma-separated list (e.g., localhost:9092,localhost:9093)",
                "default": "localhost:9092"
            },
            "topic": {
                "type": "string",
                "title": "Topic Name"
            },
            "group_id": {
                "type": "string",
                "title": "Consumer Group ID"
            },
            "client_id": {
                "type": "string",
                "title": "Client ID"
            },
            "auto_offset_reset": {
                "type": "string",
                "title": "Auto Offset Reset",
                "enum": ["earliest", "latest", "none"],
                "default": "latest"
            },
            "enable_auto_commit": {
                "type": "boolean",
                "title": "Enable Auto Commit",
                "default": true
            },
            "auto_commit_interval_ms": {
                "type": "integer",
                "title": "Auto Commit Interval (ms)",
                "default": 5000
            },
            "max_poll_records": {
                "type": "integer",
                "title": "Max Poll Records",
                "default": 500
            },
            "security_protocol": {
                "type": "string",
                "title": "Security Protocol",
                "enum": ["PLAINTEXT", "SSL", "SASL_PLAINTEXT", "SASL_SSL"],
                "default": "PLAINTEXT"
            },
            "sasl_mechanism": {
                "type": "string",
                "title": "SASL Mechanism",
                "enum": ["PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"],
                "default": "PLAIN"
            },
            "sasl_username": {
                "type": "string",
                "title": "SASL Username"
            },
            "sasl_password": {
                "type": "string",
                "title": "SASL Password",
                "format": "password"
            }
        }
    }',
    '{
        "basic": ["bootstrap_servers", "topic", "group_id", "client_id"],
        "consumer": ["auto_offset_reset", "enable_auto_commit", "auto_commit_interval_ms", "max_poll_records"],
        "security": ["security_protocol", "sasl_mechanism", "sasl_username", "sasl_password"]
    }',
    true,
    false,
    61,
    '1.0'
);

-- Apache Kafka Connector (Outbound - Producer)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'kafka_outbound',
    'outbound',
    'Kafka Producer',
    'Publish messages to Kafka topic',
    '⚡',
    'push',
    false,
    true,
    false,
    'KafkaOutboundConnector',
    '{
        "type": "object",
        "required": ["bootstrap_servers", "topic"],
        "properties": {
            "bootstrap_servers": {
                "type": "string",
                "title": "Bootstrap Servers",
                "description": "Comma-separated list",
                "default": "localhost:9092"
            },
            "topic": {
                "type": "string",
                "title": "Topic Name"
            },
            "client_id": {
                "type": "string",
                "title": "Client ID"
            },
            "acks": {
                "type": "string",
                "title": "Acknowledgments",
                "enum": ["0", "1", "all"],
                "default": "all"
            },
            "compression_type": {
                "type": "string",
                "title": "Compression Type",
                "enum": ["none", "gzip", "snappy", "lz4", "zstd"],
                "default": "none"
            },
            "batch_size": {
                "type": "integer",
                "title": "Batch Size (bytes)",
                "default": 16384
            },
            "linger_ms": {
                "type": "integer",
                "title": "Linger Time (ms)",
                "default": 0
            },
            "max_in_flight_requests": {
                "type": "integer",
                "title": "Max In-Flight Requests",
                "default": 5
            },
            "security_protocol": {
                "type": "string",
                "title": "Security Protocol",
                "enum": ["PLAINTEXT", "SSL", "SASL_PLAINTEXT", "SASL_SSL"],
                "default": "PLAINTEXT"
            },
            "sasl_mechanism": {
                "type": "string",
                "title": "SASL Mechanism",
                "enum": ["PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"]
            },
            "sasl_username": {
                "type": "string",
                "title": "SASL Username"
            },
            "sasl_password": {
                "type": "string",
                "title": "SASL Password",
                "format": "password"
            }
        }
    }',
    '{
        "basic": ["bootstrap_servers", "topic", "client_id"],
        "producer": ["acks", "compression_type", "batch_size", "linger_ms", "max_in_flight_requests"],
        "security": ["security_protocol", "sasl_mechanism", "sasl_username", "sasl_password"]
    }',
    true,
    false,
    61,
    '1.0'
);

-- Redis Connector (Inbound - Subscribe/List Pop)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'redis_inbound',
    'inbound',
    'Redis Consumer',
    'Consume from Redis list/stream/pub-sub',
    '🔴',
    'push',
    false,
    true,
    false,
    'RedisInboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "mode"],
        "properties": {
            "host": {
                "type": "string",
                "title": "Redis Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Redis Port",
                "default": 6379
            },
            "password": {
                "type": "string",
                "title": "Password",
                "format": "password"
            },
            "database": {
                "type": "integer",
                "title": "Database Number",
                "default": 0,
                "minimum": 0,
                "maximum": 15
            },
            "mode": {
                "type": "string",
                "title": "Consumer Mode",
                "enum": ["list_pop", "pub_sub", "stream"],
                "default": "list_pop"
            },
            "key_name": {
                "type": "string",
                "title": "Key/Channel/Stream Name"
            },
            "consumer_group": {
                "type": "string",
                "title": "Consumer Group (for streams)"
            },
            "consumer_name": {
                "type": "string",
                "title": "Consumer Name (for streams)"
            },
            "blocking_timeout": {
                "type": "integer",
                "title": "Blocking Timeout (seconds)",
                "default": 0,
                "description": "0 = indefinite"
            },
            "enable_tls": {
                "type": "boolean",
                "title": "Enable TLS/SSL",
                "default": false
            },
            "connection_pool_size": {
                "type": "integer",
                "title": "Connection Pool Size",
                "default": 10
            }
        }
    }',
    '{
        "basic": ["host", "port", "password", "database"],
        "consumer": ["mode", "key_name", "blocking_timeout"],
        "stream": ["consumer_group", "consumer_name"],
        "advanced": ["enable_tls", "connection_pool_size"]
    }',
    true,
    false,
    62,
    '1.0'
);

-- Redis Connector (Outbound - Push/Publish)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'redis_outbound',
    'outbound',
    'Redis Publisher',
    'Publish to Redis list/stream/pub-sub',
    '🔴',
    'push',
    false,
    true,
    false,
    'RedisOutboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "mode"],
        "properties": {
            "host": {
                "type": "string",
                "title": "Redis Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Redis Port",
                "default": 6379
            },
            "password": {
                "type": "string",
                "title": "Password",
                "format": "password"
            },
            "database": {
                "type": "integer",
                "title": "Database Number",
                "default": 0
            },
            "mode": {
                "type": "string",
                "title": "Publisher Mode",
                "enum": ["list_push", "pub_sub", "stream", "set_key"],
                "default": "list_push"
            },
            "key_name": {
                "type": "string",
                "title": "Key/Channel/Stream Name"
            },
            "list_direction": {
                "type": "string",
                "title": "List Push Direction",
                "enum": ["left", "right"],
                "default": "right"
            },
            "max_list_length": {
                "type": "integer",
                "title": "Max List Length (trim)",
                "description": "0 = no limit"
            },
            "key_expiry_seconds": {
                "type": "integer",
                "title": "Key Expiry (seconds)",
                "description": "For set_key mode"
            },
            "enable_tls": {
                "type": "boolean",
                "title": "Enable TLS/SSL",
                "default": false
            },
            "connection_pool_size": {
                "type": "integer",
                "title": "Connection Pool Size",
                "default": 10
            }
        }
    }',
    '{
        "basic": ["host", "port", "password", "database"],
        "publisher": ["mode", "key_name", "list_direction", "max_list_length"],
        "expiry": ["key_expiry_seconds"],
        "advanced": ["enable_tls", "connection_pool_size"]
    }',
    true,
    false,
    62,
    '1.0'
);

-- ========================================
-- CLOUD STORAGE CONNECTORS
-- ========================================

-- Azure Blob Storage Connector (Inbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'azure_blob_inbound',
    'inbound',
    'Azure Blob Storage Reader',
    'Poll Azure Blob Storage container for new blobs (scheduled)',
    '☁️',
    'pull',
    true,
    true,
    false,
    'AzureBlobInboundConnector',
    '{
        "type": "object",
        "required": ["account_name", "container_name"],
        "properties": {
            "account_name": {
                "type": "string",
                "title": "Storage Account Name"
            },
            "account_key": {
                "type": "string",
                "title": "Account Key",
                "format": "password"
            },
            "connection_string": {
                "type": "string",
                "title": "Connection String (alternative)",
                "format": "password"
            },
            "container_name": {
                "type": "string",
                "title": "Container Name"
            },
            "prefix": {
                "type": "string",
                "title": "Blob Prefix/Folder"
            },
            "blob_pattern": {
                "type": "string",
                "title": "Blob Pattern",
                "default": "*.hl7"
            },
            "after_processing": {
                "type": "string",
                "title": "After Processing",
                "enum": ["delete", "move", "tag", "nothing"],
                "default": "move"
            },
            "archive_container": {
                "type": "string",
                "title": "Archive Container"
            },
            "max_blobs_per_poll": {
                "type": "integer",
                "title": "Max Blobs Per Poll",
                "default": 100
            },
            "enable_https": {
                "type": "boolean",
                "title": "Use HTTPS",
                "default": true
            }
        }
    }',
    '{
        "basic": ["account_name", "account_key", "connection_string", "container_name"],
        "filter": ["prefix", "blob_pattern", "max_blobs_per_poll"],
        "processing": ["after_processing", "archive_container"],
        "advanced": ["enable_https"]
    }',
    true,
    false,
    40,
    '1.0'
);

-- Azure Blob Storage Connector (Outbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'azure_blob_outbound',
    'outbound',
    'Azure Blob Storage Writer',
    'Upload to Azure Blob Storage container',
    '☁️',
    'push',
    false,
    true,
    false,
    'AzureBlobOutboundConnector',
    '{
        "type": "object",
        "required": ["account_name", "container_name"],
        "properties": {
            "account_name": {
                "type": "string",
                "title": "Storage Account Name"
            },
            "account_key": {
                "type": "string",
                "title": "Account Key",
                "format": "password"
            },
            "connection_string": {
                "type": "string",
                "title": "Connection String (alternative)",
                "format": "password"
            },
            "container_name": {
                "type": "string",
                "title": "Container Name"
            },
            "prefix": {
                "type": "string",
                "title": "Blob Prefix/Folder"
            },
            "blob_name_pattern": {
                "type": "string",
                "title": "Blob Name Pattern",
                "default": "{message_id}_{timestamp}.json"
            },
            "content_type": {
                "type": "string",
                "title": "Content Type",
                "default": "application/json"
            },
            "access_tier": {
                "type": "string",
                "title": "Access Tier",
                "enum": ["Hot", "Cool", "Archive"],
                "default": "Hot"
            },
            "enable_https": {
                "type": "boolean",
                "title": "Use HTTPS",
                "default": true
            }
        }
    }',
    '{
        "basic": ["account_name", "account_key", "connection_string", "container_name"],
        "upload": ["prefix", "blob_name_pattern", "content_type"],
        "storage": ["access_tier"],
        "advanced": ["enable_https"]
    }',
    true,
    false,
    40,
    '1.0'
);

-- Google Cloud Storage Connector (Inbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'gcs_inbound',
    'inbound',
    'Google Cloud Storage Reader',
    'Poll GCS bucket for new objects (scheduled)',
    '☁️',
    'pull',
    true,
    true,
    false,
    'GCSInboundConnector',
    '{
        "type": "object",
        "required": ["bucket_name", "credentials_json"],
        "properties": {
            "bucket_name": {
                "type": "string",
                "title": "Bucket Name"
            },
            "credentials_json": {
                "type": "string",
                "title": "Service Account JSON",
                "format": "password",
                "description": "Google Cloud service account credentials (JSON)"
            },
            "prefix": {
                "type": "string",
                "title": "Object Prefix/Folder"
            },
            "file_pattern": {
                "type": "string",
                "title": "File Pattern",
                "default": "*.hl7"
            },
            "after_processing": {
                "type": "string",
                "title": "After Processing",
                "enum": ["delete", "move", "nothing"],
                "default": "move"
            },
            "archive_bucket": {
                "type": "string",
                "title": "Archive Bucket"
            },
            "archive_prefix": {
                "type": "string",
                "title": "Archive Prefix"
            },
            "max_objects_per_poll": {
                "type": "integer",
                "title": "Max Objects Per Poll",
                "default": 100
            }
        }
    }',
    '{
        "basic": ["bucket_name", "credentials_json"],
        "filter": ["prefix", "file_pattern", "max_objects_per_poll"],
        "processing": ["after_processing", "archive_bucket", "archive_prefix"]
    }',
    true,
    false,
    41,
    '1.0'
);

-- Google Cloud Storage Connector (Outbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'gcs_outbound',
    'outbound',
    'Google Cloud Storage Writer',
    'Upload to GCS bucket',
    '☁️',
    'push',
    false,
    true,
    false,
    'GCSOutboundConnector',
    '{
        "type": "object",
        "required": ["bucket_name", "credentials_json"],
        "properties": {
            "bucket_name": {
                "type": "string",
                "title": "Bucket Name"
            },
            "credentials_json": {
                "type": "string",
                "title": "Service Account JSON",
                "format": "password"
            },
            "prefix": {
                "type": "string",
                "title": "Object Prefix/Folder"
            },
            "object_name_pattern": {
                "type": "string",
                "title": "Object Name Pattern",
                "default": "{message_id}_{timestamp}.json"
            },
            "content_type": {
                "type": "string",
                "title": "Content Type",
                "default": "application/json"
            },
            "storage_class": {
                "type": "string",
                "title": "Storage Class",
                "enum": ["STANDARD", "NEARLINE", "COLDLINE", "ARCHIVE"],
                "default": "STANDARD"
            },
            "enable_encryption": {
                "type": "boolean",
                "title": "Enable Encryption",
                "default": true
            }
        }
    }',
    '{
        "basic": ["bucket_name", "credentials_json"],
        "upload": ["prefix", "object_name_pattern", "content_type"],
        "storage": ["storage_class", "enable_encryption"]
    }',
    true,
    false,
    41,
    '1.0'
);

-- ========================================
-- SFTP/FTP CONNECTORS
-- ========================================

-- SFTP Connector (Inbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'sftp_inbound',
    'inbound',
    'SFTP File Reader',
    'Poll SFTP server for new files (scheduled)',
    '🔐',
    'pull',
    true,
    true,
    false,
    'SFTPInboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "username", "remote_directory"],
        "properties": {
            "host": {
                "type": "string",
                "title": "SFTP Host"
            },
            "port": {
                "type": "integer",
                "title": "SFTP Port",
                "default": 22
            },
            "username": {
                "type": "string",
                "title": "Username"
            },
            "password": {
                "type": "string",
                "title": "Password",
                "format": "password"
            },
            "private_key_path": {
                "type": "string",
                "title": "Private Key Path (alternative)"
            },
            "private_key_passphrase": {
                "type": "string",
                "title": "Private Key Passphrase",
                "format": "password"
            },
            "remote_directory": {
                "type": "string",
                "title": "Remote Directory"
            },
            "file_pattern": {
                "type": "string",
                "title": "File Pattern",
                "default": "*.hl7"
            },
            "after_processing": {
                "type": "string",
                "title": "After Processing",
                "enum": ["delete", "move", "rename", "nothing"],
                "default": "move"
            },
            "archive_directory": {
                "type": "string",
                "title": "Archive Directory"
            },
            "max_files_per_poll": {
                "type": "integer",
                "title": "Max Files Per Poll",
                "default": 100
            },
            "connection_timeout": {
                "type": "integer",
                "title": "Connection Timeout (seconds)",
                "default": 30
            }
        }
    }',
    '{
        "basic": ["host", "port", "username", "password"],
        "authentication": ["private_key_path", "private_key_passphrase"],
        "file": ["remote_directory", "file_pattern", "max_files_per_poll"],
        "processing": ["after_processing", "archive_directory"],
        "advanced": ["connection_timeout"]
    }',
    true,
    false,
    70,
    '1.0'
);

-- SFTP Connector (Outbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'sftp_outbound',
    'outbound',
    'SFTP File Writer',
    'Upload files to SFTP server',
    '🔐',
    'push',
    false,
    true,
    false,
    'SFTPOutboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "username", "remote_directory"],
        "properties": {
            "host": {
                "type": "string",
                "title": "SFTP Host"
            },
            "port": {
                "type": "integer",
                "title": "SFTP Port",
                "default": 22
            },
            "username": {
                "type": "string",
                "title": "Username"
            },
            "password": {
                "type": "string",
                "title": "Password",
                "format": "password"
            },
            "private_key_path": {
                "type": "string",
                "title": "Private Key Path (alternative)"
            },
            "private_key_passphrase": {
                "type": "string",
                "title": "Private Key Passphrase",
                "format": "password"
            },
            "remote_directory": {
                "type": "string",
                "title": "Remote Directory"
            },
            "filename_pattern": {
                "type": "string",
                "title": "Filename Pattern",
                "default": "{message_id}_{timestamp}.hl7"
            },
            "create_directories": {
                "type": "boolean",
                "title": "Create Directories",
                "default": true
            },
            "overwrite_existing": {
                "type": "boolean",
                "title": "Overwrite Existing Files",
                "default": false
            },
            "file_permissions": {
                "type": "string",
                "title": "File Permissions (octal)",
                "default": "0644"
            },
            "connection_timeout": {
                "type": "integer",
                "title": "Connection Timeout (seconds)",
                "default": 30
            }
        }
    }',
    '{
        "basic": ["host", "port", "username", "password"],
        "authentication": ["private_key_path", "private_key_passphrase"],
        "file": ["remote_directory", "filename_pattern", "file_permissions"],
        "options": ["create_directories", "overwrite_existing"],
        "advanced": ["connection_timeout"]
    }',
    true,
    false,
    70,
    '1.0'
);

-- FTP Connector (Inbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'ftp_inbound',
    'inbound',
    'FTP File Reader',
    'Poll FTP server for new files (scheduled)',
    '📂',
    'pull',
    true,
    true,
    false,
    'FTPInboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "username", "remote_directory"],
        "properties": {
            "host": {
                "type": "string",
                "title": "FTP Host"
            },
            "port": {
                "type": "integer",
                "title": "FTP Port",
                "default": 21
            },
            "username": {
                "type": "string",
                "title": "Username"
            },
            "password": {
                "type": "string",
                "title": "Password",
                "format": "password"
            },
            "enable_ftps": {
                "type": "boolean",
                "title": "Enable FTPS (FTP over SSL)",
                "default": false
            },
            "passive_mode": {
                "type": "boolean",
                "title": "Use Passive Mode",
                "default": true
            },
            "remote_directory": {
                "type": "string",
                "title": "Remote Directory"
            },
            "file_pattern": {
                "type": "string",
                "title": "File Pattern",
                "default": "*.hl7"
            },
            "after_processing": {
                "type": "string",
                "title": "After Processing",
                "enum": ["delete", "move", "rename", "nothing"],
                "default": "move"
            },
            "archive_directory": {
                "type": "string",
                "title": "Archive Directory"
            },
            "max_files_per_poll": {
                "type": "integer",
                "title": "Max Files Per Poll",
                "default": 100
            },
            "connection_timeout": {
                "type": "integer",
                "title": "Connection Timeout (seconds)",
                "default": 30
            }
        }
    }',
    '{
        "basic": ["host", "port", "username", "password"],
        "security": ["enable_ftps", "passive_mode"],
        "file": ["remote_directory", "file_pattern", "max_files_per_poll"],
        "processing": ["after_processing", "archive_directory"],
        "advanced": ["connection_timeout"]
    }',
    true,
    false,
    71,
    '1.0'
);

-- FTP Connector (Outbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'ftp_outbound',
    'outbound',
    'FTP File Writer',
    'Upload files to FTP server',
    '📂',
    'push',
    false,
    true,
    false,
    'FTPOutboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "username", "remote_directory"],
        "properties": {
            "host": {
                "type": "string",
                "title": "FTP Host"
            },
            "port": {
                "type": "integer",
                "title": "FTP Port",
                "default": 21
            },
            "username": {
                "type": "string",
                "title": "Username"
            },
            "password": {
                "type": "string",
                "title": "Password",
                "format": "password"
            },
            "enable_ftps": {
                "type": "boolean",
                "title": "Enable FTPS (FTP over SSL)",
                "default": false
            },
            "passive_mode": {
                "type": "boolean",
                "title": "Use Passive Mode",
                "default": true
            },
            "remote_directory": {
                "type": "string",
                "title": "Remote Directory"
            },
            "filename_pattern": {
                "type": "string",
                "title": "Filename Pattern",
                "default": "{message_id}_{timestamp}.hl7"
            },
            "create_directories": {
                "type": "boolean",
                "title": "Create Directories",
                "default": true
            },
            "overwrite_existing": {
                "type": "boolean",
                "title": "Overwrite Existing Files",
                "default": false
            },
            "transfer_mode": {
                "type": "string",
                "title": "Transfer Mode",
                "enum": ["binary", "ascii"],
                "default": "binary"
            },
            "connection_timeout": {
                "type": "integer",
                "title": "Connection Timeout (seconds)",
                "default": 30
            }
        }
    }',
    '{
        "basic": ["host", "port", "username", "password"],
        "security": ["enable_ftps", "passive_mode"],
        "file": ["remote_directory", "filename_pattern", "transfer_mode"],
        "options": ["create_directories", "overwrite_existing"],
        "advanced": ["connection_timeout"]
    }',
    true,
    false,
    71,
    '1.0'
);

-- Summary: 14 new connectors added
-- Message Queues: 6 (RabbitMQ, Kafka, Redis - inbound + outbound)
-- Cloud Storage: 4 (Azure Blob, GCS - inbound + outbound)
-- File Transfer: 4 (SFTP, FTP - inbound + outbound)
-- Total connectors: 17 (existing) + 14 (new) = 31 OOB connectors
