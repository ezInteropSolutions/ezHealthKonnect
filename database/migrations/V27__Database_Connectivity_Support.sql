-- V27__Database_Connectivity_Support.sql
-- Add database connectivity types (PostgreSQL, MySQL, SQL Server, Oracle, MongoDB)

-- PostgreSQL Database Connector (Inbound - Poll for new records)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'postgresql_inbound',
    'inbound',
    'PostgreSQL Database Reader',
    'Poll PostgreSQL database for new records (scheduled)',
    '🐘',
    'pull',
    true,
    true,
    false,
    'PostgreSQLInboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "database", "query"],
        "properties": {
            "host": {
                "type": "string",
                "title": "Database Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Database Port",
                "default": 5432,
                "minimum": 1,
                "maximum": 65535
            },
            "database": {
                "type": "string",
                "title": "Database Name"
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
            "query": {
                "type": "string",
                "title": "SQL Query",
                "description": "SELECT query to fetch records (use WHERE clause for incremental polling)"
            },
            "incremental_column": {
                "type": "string",
                "title": "Incremental Column",
                "description": "Column to track last polled value (e.g., id, updated_at)"
            },
            "after_processing": {
                "type": "string",
                "title": "After Processing",
                "enum": ["update_flag", "delete", "archive", "nothing"],
                "default": "update_flag"
            },
            "update_column": {
                "type": "string",
                "title": "Update Column",
                "description": "Column to set after processing (if using update_flag)"
            },
            "update_value": {
                "type": "string",
                "title": "Update Value",
                "description": "Value to set after processing",
                "default": "processed"
            },
            "ssl_mode": {
                "type": "string",
                "title": "SSL Mode",
                "enum": ["disable", "require", "verify-ca", "verify-full"],
                "default": "disable"
            },
            "max_records_per_poll": {
                "type": "integer",
                "title": "Max Records Per Poll",
                "default": 100,
                "minimum": 1,
                "maximum": 10000
            },
            "connection_timeout": {
                "type": "integer",
                "title": "Connection Timeout (seconds)",
                "default": 30
            }
        }
    }',
    '{
        "basic": ["host", "port", "database", "username", "password"],
        "query": ["query", "incremental_column", "max_records_per_poll"],
        "processing": ["after_processing", "update_column", "update_value"],
        "advanced": ["ssl_mode", "connection_timeout"]
    }',
    true,
    false,
    50,
    '1.0'
);

-- PostgreSQL Database Connector (Outbound - Write/Insert)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'postgresql_outbound',
    'outbound',
    'PostgreSQL Database Writer',
    'Insert/Update records to PostgreSQL database',
    '🐘',
    'push',
    false,
    true,
    false,
    'PostgreSQLOutboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "database", "table"],
        "properties": {
            "host": {
                "type": "string",
                "title": "Database Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Database Port",
                "default": 5432,
                "minimum": 1,
                "maximum": 65535
            },
            "database": {
                "type": "string",
                "title": "Database Name"
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
            "table": {
                "type": "string",
                "title": "Target Table"
            },
            "operation": {
                "type": "string",
                "title": "Operation",
                "enum": ["insert", "upsert", "update"],
                "default": "insert"
            },
            "unique_key_columns": {
                "type": "array",
                "title": "Unique Key Columns",
                "description": "Columns for UPSERT/UPDATE operations",
                "items": {
                    "type": "string"
                }
            },
            "ssl_mode": {
                "type": "string",
                "title": "SSL Mode",
                "enum": ["disable", "require", "verify-ca", "verify-full"],
                "default": "disable"
            },
            "batch_size": {
                "type": "integer",
                "title": "Batch Size",
                "default": 1,
                "minimum": 1,
                "maximum": 1000
            },
            "connection_pool_size": {
                "type": "integer",
                "title": "Connection Pool Size",
                "default": 5,
                "minimum": 1,
                "maximum": 50
            }
        }
    }',
    '{
        "basic": ["host", "port", "database", "username", "password", "table"],
        "operation": ["operation", "unique_key_columns", "batch_size"],
        "advanced": ["ssl_mode", "connection_pool_size"]
    }',
    true,
    false,
    50,
    '1.0'
);

-- MySQL Database Connector (Inbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'mysql_inbound',
    'inbound',
    'MySQL Database Reader',
    'Poll MySQL database for new records (scheduled)',
    '🐬',
    'pull',
    true,
    true,
    false,
    'MySQLInboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "database", "query"],
        "properties": {
            "host": {
                "type": "string",
                "title": "Database Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Database Port",
                "default": 3306,
                "minimum": 1,
                "maximum": 65535
            },
            "database": {
                "type": "string",
                "title": "Database Name"
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
            "query": {
                "type": "string",
                "title": "SQL Query"
            },
            "incremental_column": {
                "type": "string",
                "title": "Incremental Column"
            },
            "after_processing": {
                "type": "string",
                "title": "After Processing",
                "enum": ["update_flag", "delete", "archive", "nothing"],
                "default": "update_flag"
            },
            "update_column": {
                "type": "string",
                "title": "Update Column"
            },
            "enable_tls": {
                "type": "boolean",
                "title": "Enable TLS/SSL",
                "default": false
            },
            "max_records_per_poll": {
                "type": "integer",
                "title": "Max Records Per Poll",
                "default": 100
            }
        }
    }',
    '{
        "basic": ["host", "port", "database", "username", "password"],
        "query": ["query", "incremental_column", "max_records_per_poll"],
        "processing": ["after_processing", "update_column"],
        "advanced": ["enable_tls"]
    }',
    true,
    false,
    51,
    '1.0'
);

-- MySQL Database Connector (Outbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'mysql_outbound',
    'outbound',
    'MySQL Database Writer',
    'Insert/Update records to MySQL database',
    '🐬',
    'push',
    false,
    true,
    false,
    'MySQLOutboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "database", "table"],
        "properties": {
            "host": {
                "type": "string",
                "title": "Database Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Database Port",
                "default": 3306
            },
            "database": {
                "type": "string",
                "title": "Database Name"
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
            "table": {
                "type": "string",
                "title": "Target Table"
            },
            "operation": {
                "type": "string",
                "title": "Operation",
                "enum": ["insert", "replace", "update"],
                "default": "insert"
            },
            "unique_key_columns": {
                "type": "array",
                "title": "Unique Key Columns",
                "items": {
                    "type": "string"
                }
            },
            "enable_tls": {
                "type": "boolean",
                "title": "Enable TLS/SSL",
                "default": false
            },
            "batch_size": {
                "type": "integer",
                "title": "Batch Size",
                "default": 1
            }
        }
    }',
    '{
        "basic": ["host", "port", "database", "username", "password", "table"],
        "operation": ["operation", "unique_key_columns", "batch_size"],
        "advanced": ["enable_tls"]
    }',
    true,
    false,
    51,
    '1.0'
);

-- SQL Server Database Connector (Inbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'sqlserver_inbound',
    'inbound',
    'SQL Server Database Reader',
    'Poll SQL Server database for new records (scheduled)',
    '🗄️',
    'pull',
    true,
    true,
    false,
    'SQLServerInboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "database", "query"],
        "properties": {
            "host": {
                "type": "string",
                "title": "Database Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Database Port",
                "default": 1433
            },
            "database": {
                "type": "string",
                "title": "Database Name"
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
            "query": {
                "type": "string",
                "title": "SQL Query"
            },
            "incremental_column": {
                "type": "string",
                "title": "Incremental Column"
            },
            "after_processing": {
                "type": "string",
                "title": "After Processing",
                "enum": ["update_flag", "delete", "archive", "nothing"],
                "default": "update_flag"
            },
            "authentication_type": {
                "type": "string",
                "title": "Authentication Type",
                "enum": ["sql_authentication", "windows_authentication"],
                "default": "sql_authentication"
            },
            "encrypt_connection": {
                "type": "boolean",
                "title": "Encrypt Connection",
                "default": true
            },
            "trust_server_certificate": {
                "type": "boolean",
                "title": "Trust Server Certificate",
                "default": false
            },
            "max_records_per_poll": {
                "type": "integer",
                "title": "Max Records Per Poll",
                "default": 100
            }
        }
    }',
    '{
        "basic": ["host", "port", "database", "username", "password"],
        "query": ["query", "incremental_column", "max_records_per_poll"],
        "processing": ["after_processing"],
        "security": ["authentication_type", "encrypt_connection", "trust_server_certificate"]
    }',
    true,
    false,
    52,
    '1.0'
);

-- SQL Server Database Connector (Outbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'sqlserver_outbound',
    'outbound',
    'SQL Server Database Writer',
    'Insert/Update records to SQL Server database',
    '🗄️',
    'push',
    false,
    true,
    false,
    'SQLServerOutboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "database", "table"],
        "properties": {
            "host": {
                "type": "string",
                "title": "Database Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Database Port",
                "default": 1433
            },
            "database": {
                "type": "string",
                "title": "Database Name"
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
            "table": {
                "type": "string",
                "title": "Target Table"
            },
            "operation": {
                "type": "string",
                "title": "Operation",
                "enum": ["insert", "merge", "update"],
                "default": "insert"
            },
            "unique_key_columns": {
                "type": "array",
                "title": "Unique Key Columns",
                "items": {
                    "type": "string"
                }
            },
            "authentication_type": {
                "type": "string",
                "title": "Authentication Type",
                "enum": ["sql_authentication", "windows_authentication"],
                "default": "sql_authentication"
            },
            "encrypt_connection": {
                "type": "boolean",
                "title": "Encrypt Connection",
                "default": true
            },
            "batch_size": {
                "type": "integer",
                "title": "Batch Size",
                "default": 1
            }
        }
    }',
    '{
        "basic": ["host", "port", "database", "username", "password", "table"],
        "operation": ["operation", "unique_key_columns", "batch_size"],
        "security": ["authentication_type", "encrypt_connection"]
    }',
    true,
    false,
    52,
    '1.0'
);

-- MongoDB Connector (Inbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'mongodb_inbound',
    'inbound',
    'MongoDB Database Reader',
    'Poll MongoDB collection for new documents (scheduled)',
    '🍃',
    'pull',
    true,
    true,
    false,
    'MongoDBInboundConnector',
    '{
        "type": "object",
        "required": ["connection_uri", "database", "collection"],
        "properties": {
            "connection_uri": {
                "type": "string",
                "title": "MongoDB Connection URI",
                "description": "mongodb://user:password@host:port or mongodb+srv://...",
                "default": "mongodb://localhost:27017"
            },
            "database": {
                "type": "string",
                "title": "Database Name"
            },
            "collection": {
                "type": "string",
                "title": "Collection Name"
            },
            "query_filter": {
                "type": "string",
                "title": "Query Filter (JSON)",
                "description": "MongoDB query filter in JSON format",
                "default": "{}"
            },
            "incremental_field": {
                "type": "string",
                "title": "Incremental Field",
                "description": "Field to track last polled value (e.g., _id, updated_at)"
            },
            "after_processing": {
                "type": "string",
                "title": "After Processing",
                "enum": ["update_flag", "delete", "move_collection", "nothing"],
                "default": "update_flag"
            },
            "update_field": {
                "type": "string",
                "title": "Update Field",
                "default": "processed"
            },
            "update_value": {
                "type": "string",
                "title": "Update Value",
                "default": "true"
            },
            "archive_collection": {
                "type": "string",
                "title": "Archive Collection",
                "description": "Collection to move documents to (if using move_collection)"
            },
            "max_documents_per_poll": {
                "type": "integer",
                "title": "Max Documents Per Poll",
                "default": 100
            },
            "connection_timeout": {
                "type": "integer",
                "title": "Connection Timeout (ms)",
                "default": 30000
            }
        }
    }',
    '{
        "basic": ["connection_uri", "database", "collection"],
        "query": ["query_filter", "incremental_field", "max_documents_per_poll"],
        "processing": ["after_processing", "update_field", "update_value", "archive_collection"],
        "advanced": ["connection_timeout"]
    }',
    true,
    false,
    53,
    '1.0'
);

-- MongoDB Connector (Outbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'mongodb_outbound',
    'outbound',
    'MongoDB Database Writer',
    'Insert/Update documents to MongoDB collection',
    '🍃',
    'push',
    false,
    true,
    false,
    'MongoDBOutboundConnector',
    '{
        "type": "object",
        "required": ["connection_uri", "database", "collection"],
        "properties": {
            "connection_uri": {
                "type": "string",
                "title": "MongoDB Connection URI",
                "description": "mongodb://user:password@host:port or mongodb+srv://...",
                "default": "mongodb://localhost:27017"
            },
            "database": {
                "type": "string",
                "title": "Database Name"
            },
            "collection": {
                "type": "string",
                "title": "Collection Name"
            },
            "operation": {
                "type": "string",
                "title": "Operation",
                "enum": ["insert", "upsert", "update", "replace"],
                "default": "insert"
            },
            "unique_key_fields": {
                "type": "array",
                "title": "Unique Key Fields",
                "description": "Fields for upsert/update/replace operations",
                "items": {
                    "type": "string"
                }
            },
            "write_concern": {
                "type": "string",
                "title": "Write Concern",
                "enum": ["majority", "1", "2", "3"],
                "default": "majority"
            },
            "batch_size": {
                "type": "integer",
                "title": "Batch Size",
                "default": 1,
                "minimum": 1,
                "maximum": 1000
            },
            "connection_pool_size": {
                "type": "integer",
                "title": "Connection Pool Size",
                "default": 5
            }
        }
    }',
    '{
        "basic": ["connection_uri", "database", "collection"],
        "operation": ["operation", "unique_key_fields", "batch_size"],
        "advanced": ["write_concern", "connection_pool_size"]
    }',
    true,
    false,
    53,
    '1.0'
);

-- Oracle Database Connector (Inbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'oracle_inbound',
    'inbound',
    'Oracle Database Reader',
    'Poll Oracle database for new records (scheduled)',
    '🔶',
    'pull',
    true,
    true,
    false,
    'OracleInboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "service_name", "query"],
        "properties": {
            "host": {
                "type": "string",
                "title": "Database Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Database Port",
                "default": 1521
            },
            "service_name": {
                "type": "string",
                "title": "Service Name"
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
            "query": {
                "type": "string",
                "title": "SQL Query"
            },
            "incremental_column": {
                "type": "string",
                "title": "Incremental Column"
            },
            "after_processing": {
                "type": "string",
                "title": "After Processing",
                "enum": ["update_flag", "delete", "nothing"],
                "default": "update_flag"
            },
            "enable_ssl": {
                "type": "boolean",
                "title": "Enable SSL",
                "default": false
            },
            "max_records_per_poll": {
                "type": "integer",
                "title": "Max Records Per Poll",
                "default": 100
            }
        }
    }',
    '{
        "basic": ["host", "port", "service_name", "username", "password"],
        "query": ["query", "incremental_column", "max_records_per_poll"],
        "processing": ["after_processing"],
        "advanced": ["enable_ssl"]
    }',
    true,
    false,
    54,
    '1.0'
);

-- Oracle Database Connector (Outbound)
INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority, version
) VALUES (
    'oracle_outbound',
    'outbound',
    'Oracle Database Writer',
    'Insert/Update records to Oracle database',
    '🔶',
    'push',
    false,
    true,
    false,
    'OracleOutboundConnector',
    '{
        "type": "object",
        "required": ["host", "port", "service_name", "table"],
        "properties": {
            "host": {
                "type": "string",
                "title": "Database Host",
                "default": "localhost"
            },
            "port": {
                "type": "integer",
                "title": "Database Port",
                "default": 1521
            },
            "service_name": {
                "type": "string",
                "title": "Service Name"
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
            "table": {
                "type": "string",
                "title": "Target Table"
            },
            "operation": {
                "type": "string",
                "title": "Operation",
                "enum": ["insert", "merge", "update"],
                "default": "insert"
            },
            "unique_key_columns": {
                "type": "array",
                "title": "Unique Key Columns",
                "items": {
                    "type": "string"
                }
            },
            "enable_ssl": {
                "type": "boolean",
                "title": "Enable SSL",
                "default": false
            },
            "batch_size": {
                "type": "integer",
                "title": "Batch Size",
                "default": 1
            }
        }
    }',
    '{
        "basic": ["host", "port", "service_name", "username", "password", "table"],
        "operation": ["operation", "unique_key_columns", "batch_size"],
        "advanced": ["enable_ssl"]
    }',
    true,
    false,
    54,
    '1.0'
);

-- Summary: 10 new database connectivity types added
-- Total connectors: 7 (existing) + 10 (database) = 17 OOB connectors
