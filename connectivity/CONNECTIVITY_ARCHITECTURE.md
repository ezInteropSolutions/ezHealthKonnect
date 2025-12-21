# 🔌 Multi-Connectivity Architecture - Complete Design

## 📋 Overview

**Mission**: Build a universal, pluggable connectivity layer supporting multiple inbound and outbound connectivity types with OOB templates, cron scheduling, and MVC architecture.

**Design Principles**:
- **OOB + Customization**: Pre-built connectors with custom parameter support
- **MVC Pattern**: Clear separation of Models, Views, and Controllers
- **Universal Interface**: All connectors implement common interfaces
- **Hot-Reloadable**: Configuration changes without service restart
- **Cron Support**: Scheduled polling for pull-based connectors
- **Metadata-Driven**: Rich parameter validation and UI generation

---

## 🏗️ Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                    CONNECTIVITY LAYER                            │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │   INBOUND   │  │ PROCESSING  │  │  OUTBOUND   │             │
│  │ CONNECTORS  │→ │   ENGINE    │→ │ CONNECTORS  │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
│                                                                  │
│  • TCP/MLLP     • Parse & Convert • HTTP/HTTPS                  │
│  • HTTP/REST    • Transform        • FHIR API                    │
│  • File/SFTP    • Validate         • Database                    │
│  • Database     • Enrich           • File/SFTP                   │
│  • HL7 API      • Route            • Queue/Kafka                 │
│  • Queue/Kafka  • Monitor          • Email/Webhook               │
│  • Cron Jobs                                                     │
└──────────────────────────────────────────────────────────────────┘
```

---

## 🔌 Supported Connectivity Types

### **Inbound Connectors** (Message Sources)

| Type | Mode | Scheduling | Description | Priority |
|------|------|------------|-------------|----------|
| **TCP/MLLP** | Push | N/A | HL7 v2.x over MLLP (with TLS support) | ✅ Enhanced |
| **HTTP/REST** | Push | N/A | RESTful API endpoint | ⚡ Phase 1 |
| **File Listener** | Pull | Cron | Monitor directory for new files | ⚡ Phase 1 |
| **SFTP** | Pull | Cron | Secure file transfer | ⚡ Phase 1 |
| **Database Poll** | Pull | Cron | Query database table for new rows | ⚡ Phase 1 |
| **AWS S3** | Pull | Cron | Poll S3 bucket for new objects | ⚡ Phase 1 |
| **Azure Blob Storage** | Pull | Cron | Poll Azure Blob Storage | ⚡ Phase 1 |
| **Google Cloud Storage** | Pull | Cron | Poll GCS bucket | ⚡ Phase 1 |
| **HL7 API** | Push | N/A | Receive HL7 messages via HTTP | ⚡ Phase 1 |
| **Kafka Consumer** | Pull | Stream | Consume from Kafka topics | 📋 Phase 2 |
| **RabbitMQ** | Pull | Stream | Consume from RabbitMQ queues | 📋 Phase 2 |
| **Email (IMAP)** | Pull | Cron | Read emails with attachments | 📋 Phase 2 |
| **FTP** | Pull | Cron | Traditional FTP file transfer | 📋 Phase 2 |
| **WebSocket** | Push | N/A | Real-time bidirectional connection | 📋 Phase 3 |
| **SOAP Web Service** | Push | N/A | Legacy SOAP endpoint | 📋 Phase 3 |

### **Outbound Connectors** (Message Destinations)

| Type | Mode | Description | Priority |
|------|------|-------------|----------|
| **HTTP/HTTPS** | Push | POST to REST endpoint | ✅ Phase 1 |
| **FHIR API** | Push | Submit to FHIR server | ✅ Complete |
| **Database Insert** | Push | Insert into database table | ✅ Complete |
| **File Writer** | Push | Write to file system | ⚡ Phase 1 |
| **SFTP Upload** | Push | Upload via SFTP | ⚡ Phase 1 |
| **AWS S3 Upload** | Push | Upload to S3 bucket | ⚡ Phase 1 |
| **Azure Blob Upload** | Push | Upload to Azure Blob Storage | ⚡ Phase 1 |
| **Google Cloud Storage Upload** | Push | Upload to GCS bucket | ⚡ Phase 1 |
| **Email/SMTP** | Push | Send email with attachment | ⚡ Phase 1 |
| **Webhook** | Push | POST to webhook URL | ⚡ Phase 1 |
| **Kafka Producer** | Push | Publish to Kafka topic | 📋 Phase 2 |
| **RabbitMQ** | Push | Publish to RabbitMQ exchange | 📋 Phase 2 |
| **TCP/MLLP** | Push | Send HL7 via MLLP (reverse) | 📋 Phase 2 |
| **FTP** | Push | Upload via FTP | 📋 Phase 2 |
| **SMS/Twilio** | Push | Send SMS notifications | 📋 Phase 3 |

---

## 📊 Database Schema - Connectivity Configuration

### **Migration V26: Multi-Connectivity Support**

```sql
-- ========================================
-- V26__Multi_Connectivity_Support.sql
-- ========================================

-- Connectivity type definitions (OOB connectors)
CREATE TABLE connectivity_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Type identification
    type_name VARCHAR(50) UNIQUE NOT NULL,  -- 'tcp_mllp', 'http_rest', 'file_listener', etc.
    category VARCHAR(20) NOT NULL,          -- 'inbound' or 'outbound'
    display_name VARCHAR(100) NOT NULL,
    description TEXT,
    icon VARCHAR(50),                       -- Emoji or icon identifier

    -- Behavior
    mode VARCHAR(20) NOT NULL,              -- 'push', 'pull', 'stream'
    supports_cron BOOLEAN DEFAULT false,
    requires_auth BOOLEAN DEFAULT false,
    is_bidirectional BOOLEAN DEFAULT false,

    -- Implementation
    implementation_class VARCHAR(255),      -- Go struct or JavaScript class
    config_schema JSONB NOT NULL,          -- JSON Schema for parameters
    default_config JSONB,                  -- Default parameter values

    -- UI hints
    wizard_template VARCHAR(50),           -- Which wizard template to use
    parameter_groups JSONB,                -- Group parameters for UI (Basic, Advanced, Security)
    validation_rules JSONB,                -- Additional validation beyond schema

    -- Status
    is_active BOOLEAN DEFAULT true,
    is_beta BOOLEAN DEFAULT false,
    priority INTEGER DEFAULT 100,

    -- Metadata
    version VARCHAR(20),
    documentation_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Interface connectivity configuration (replaces simple source_type/target_type)
CREATE TABLE interface_connectivity (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id UUID REFERENCES interfaces(id) ON DELETE CASCADE,

    -- Source (Inbound) Configuration
    source_connectivity_type_id UUID REFERENCES connectivity_types(id),
    source_config JSONB NOT NULL,          -- Type-specific configuration
    source_enabled BOOLEAN DEFAULT true,

    -- Cron scheduling (for pull-based sources)
    cron_enabled BOOLEAN DEFAULT false,
    cron_expression VARCHAR(100),          -- e.g., "*/5 * * * *" (every 5 minutes)
    cron_timezone VARCHAR(50) DEFAULT 'UTC',
    next_run_at TIMESTAMP WITH TIME ZONE,
    last_run_at TIMESTAMP WITH TIME ZONE,
    last_run_status VARCHAR(50),

    -- Target (Outbound) Configuration
    target_connectivity_type_id UUID REFERENCES connectivity_types(id),
    target_config JSONB NOT NULL,          -- Type-specific configuration
    target_enabled BOOLEAN DEFAULT true,

    -- Multi-target support (future)
    additional_targets JSONB,              -- Array of {type_id, config, enabled}

    -- Connection state
    connection_status VARCHAR(50) DEFAULT 'disconnected',  -- 'connected', 'disconnected', 'error'
    last_connection_test TIMESTAMP WITH TIME ZONE,
    last_connection_error TEXT,

    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT one_connectivity_per_interface UNIQUE(interface_id)
);

-- Connectivity execution logs (for cron jobs and monitoring)
CREATE TABLE connectivity_execution_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_connectivity_id UUID REFERENCES interface_connectivity(id) ON DELETE CASCADE,
    interface_id UUID REFERENCES interfaces(id) ON DELETE CASCADE,

    -- Execution details
    execution_type VARCHAR(50) NOT NULL,   -- 'cron', 'manual', 'startup'
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50),                    -- 'success', 'failed', 'partial'

    -- Results
    messages_retrieved INTEGER DEFAULT 0,
    messages_processed INTEGER DEFAULT 0,
    messages_failed INTEGER DEFAULT 0,
    error_details JSONB,

    -- Performance
    duration_ms INTEGER,

    -- Metadata
    triggered_by VARCHAR(100),             -- 'cron', 'user:email', 'system'
    execution_metadata JSONB
);

-- Cron job scheduler state
CREATE TABLE cron_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_connectivity_id UUID REFERENCES interface_connectivity(id) ON DELETE CASCADE,

    -- Schedule
    cron_expression VARCHAR(100) NOT NULL,
    timezone VARCHAR(50) DEFAULT 'UTC',

    -- State
    is_enabled BOOLEAN DEFAULT true,
    next_execution TIMESTAMP WITH TIME ZONE,
    last_execution TIMESTAMP WITH TIME ZONE,
    execution_count INTEGER DEFAULT 0,
    failure_count INTEGER DEFAULT 0,
    consecutive_failures INTEGER DEFAULT 0,

    -- Error handling
    max_retries INTEGER DEFAULT 3,
    retry_delay_seconds INTEGER DEFAULT 60,
    circuit_breaker_threshold INTEGER DEFAULT 5,  -- Disable after N consecutive failures
    is_circuit_broken BOOLEAN DEFAULT false,

    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX idx_interface_connectivity_interface ON interface_connectivity(interface_id);
CREATE INDEX idx_interface_connectivity_source_type ON interface_connectivity(source_connectivity_type_id);
CREATE INDEX idx_interface_connectivity_target_type ON interface_connectivity(target_connectivity_type_id);
CREATE INDEX idx_connectivity_execution_log_interface ON connectivity_execution_log(interface_id);
CREATE INDEX idx_connectivity_execution_log_started ON connectivity_execution_log(started_at DESC);
CREATE INDEX idx_cron_jobs_next_execution ON cron_jobs(next_execution) WHERE is_enabled = true AND is_circuit_broken = false;

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_connectivity_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_interface_connectivity_timestamp
    BEFORE UPDATE ON interface_connectivity
    FOR EACH ROW
    EXECUTE FUNCTION update_connectivity_timestamp();

CREATE TRIGGER update_cron_jobs_timestamp
    BEFORE UPDATE ON cron_jobs
    FOR EACH ROW
    EXECUTE FUNCTION update_connectivity_timestamp();

COMMENT ON TABLE connectivity_types IS 'OOB connector definitions with configuration schemas';
COMMENT ON TABLE interface_connectivity IS 'Per-interface source and target connectivity configuration';
COMMENT ON TABLE connectivity_execution_log IS 'Execution history for scheduled and manual runs';
COMMENT ON TABLE cron_jobs IS 'Cron scheduler state and circuit breaker management';
```

---

## 🎯 Connectivity Type Metadata Examples

### **TCP/MLLP Connector** (Existing)

```json
{
  "type_name": "tcp_mllp",
  "category": "inbound",
  "display_name": "TCP/MLLP (HL7 v2.x)",
  "description": "Receive HL7 v2.x messages over MLLP protocol",
  "icon": "🔌",
  "mode": "push",
  "supports_cron": false,
  "requires_auth": false,

  "config_schema": {
    "type": "object",
    "required": ["port"],
    "properties": {
      "port": {
        "type": "integer",
        "minimum": 1024,
        "maximum": 65535,
        "default": 2575,
        "title": "Listen Port",
        "description": "Port to listen for MLLP connections"
      },
      "host": {
        "type": "string",
        "default": "0.0.0.0",
        "title": "Bind Address",
        "description": "IP address to bind (0.0.0.0 for all interfaces)"
      },
      "max_connections": {
        "type": "integer",
        "default": 10,
        "title": "Max Concurrent Connections",
        "description": "Maximum number of simultaneous connections"
      },
      "timeout_seconds": {
        "type": "integer",
        "default": 300,
        "title": "Connection Timeout (seconds)",
        "description": "Idle connection timeout"
      },
      "enable_tls": {
        "type": "boolean",
        "default": false,
        "title": "Enable TLS/SSL"
      },
      "tls_cert_path": {
        "type": "string",
        "title": "TLS Certificate Path"
      },
      "tls_key_path": {
        "type": "string",
        "title": "TLS Private Key Path"
      }
    }
  },

  "parameter_groups": {
    "basic": ["port", "host"],
    "advanced": ["max_connections", "timeout_seconds"],
    "security": ["enable_tls", "tls_cert_path", "tls_key_path"]
  }
}
```

### **File Listener Connector** (New - Cron-based)

```json
{
  "type_name": "file_listener",
  "category": "inbound",
  "display_name": "File System Listener",
  "description": "Monitor directory for new files (scheduled polling)",
  "icon": "📁",
  "mode": "pull",
  "supports_cron": true,
  "requires_auth": false,

  "config_schema": {
    "type": "object",
    "required": ["directory_path", "file_pattern"],
    "properties": {
      "directory_path": {
        "type": "string",
        "title": "Monitor Directory",
        "description": "Full path to directory to monitor",
        "examples": ["/data/inbound/hl7", "C:\\HL7\\Inbound"]
      },
      "file_pattern": {
        "type": "string",
        "default": "*.hl7",
        "title": "File Pattern",
        "description": "Glob pattern for files to process",
        "examples": ["*.hl7", "*.txt", "ADT_*.hl7"]
      },
      "process_subdirectories": {
        "type": "boolean",
        "default": false,
        "title": "Process Subdirectories",
        "description": "Recursively process subdirectories"
      },
      "after_processing": {
        "type": "string",
        "enum": ["delete", "move", "archive", "nothing"],
        "default": "move",
        "title": "After Processing",
        "description": "What to do with file after successful processing"
      },
      "archive_directory": {
        "type": "string",
        "title": "Archive Directory",
        "description": "Directory to move processed files (if after_processing=move)"
      },
      "error_directory": {
        "type": "string",
        "title": "Error Directory",
        "description": "Directory to move failed files"
      },
      "file_encoding": {
        "type": "string",
        "enum": ["UTF-8", "ASCII", "ISO-8859-1", "Windows-1252"],
        "default": "UTF-8",
        "title": "File Encoding"
      },
      "min_file_age_seconds": {
        "type": "integer",
        "default": 5,
        "title": "Minimum File Age (seconds)",
        "description": "Wait before processing (ensures file is completely written)"
      }
    }
  },

  "parameter_groups": {
    "basic": ["directory_path", "file_pattern"],
    "processing": ["after_processing", "archive_directory", "error_directory"],
    "advanced": ["process_subdirectories", "file_encoding", "min_file_age_seconds"]
  }
}
```

### **Database Poll Connector** (New - Cron-based)

```json
{
  "type_name": "database_poll",
  "category": "inbound",
  "display_name": "Database Poller",
  "description": "Query database table for new records (scheduled polling)",
  "icon": "🗄️",
  "mode": "pull",
  "supports_cron": true,
  "requires_auth": true,

  "config_schema": {
    "type": "object",
    "required": ["database_type", "connection_string", "query"],
    "properties": {
      "database_type": {
        "type": "string",
        "enum": ["postgresql", "mysql", "mssql", "oracle"],
        "title": "Database Type"
      },
      "connection_string": {
        "type": "string",
        "format": "password",
        "title": "Connection String",
        "description": "Database connection string",
        "examples": [
          "postgres://user:pass@localhost:5432/dbname",
          "mysql://user:pass@localhost:3306/dbname"
        ]
      },
      "query": {
        "type": "string",
        "title": "SQL Query",
        "description": "Query to fetch new records",
        "examples": [
          "SELECT * FROM messages WHERE processed = false ORDER BY created_at",
          "SELECT * FROM hl7_queue WHERE status = 'pending' LIMIT 100"
        ]
      },
      "id_column": {
        "type": "string",
        "default": "id",
        "title": "ID Column",
        "description": "Column name for record ID (for tracking)"
      },
      "message_column": {
        "type": "string",
        "default": "message",
        "title": "Message Column",
        "description": "Column name containing the message content"
      },
      "mark_processed_query": {
        "type": "string",
        "title": "Mark as Processed Query",
        "description": "Query to mark record as processed (optional)",
        "examples": [
          "UPDATE messages SET processed = true WHERE id = $1",
          "UPDATE hl7_queue SET status = 'processed' WHERE id = $1"
        ]
      },
      "batch_size": {
        "type": "integer",
        "default": 100,
        "minimum": 1,
        "maximum": 10000,
        "title": "Batch Size",
        "description": "Maximum records to fetch per execution"
      },
      "connection_pool_size": {
        "type": "integer",
        "default": 5,
        "title": "Connection Pool Size"
      }
    }
  },

  "parameter_groups": {
    "basic": ["database_type", "connection_string"],
    "query": ["query", "id_column", "message_column", "mark_processed_query"],
    "advanced": ["batch_size", "connection_pool_size"]
  }
}
```

### **HTTP/REST Connector** (Inbound)

```json
{
  "type_name": "http_rest",
  "category": "inbound",
  "display_name": "HTTP/REST API",
  "description": "Receive messages via HTTP POST endpoint",
  "icon": "🌐",
  "mode": "push",
  "supports_cron": false,
  "requires_auth": true,

  "config_schema": {
    "type": "object",
    "required": ["endpoint_path"],
    "properties": {
      "endpoint_path": {
        "type": "string",
        "default": "/api/hl7/receive",
        "title": "Endpoint Path",
        "description": "URL path to receive messages",
        "pattern": "^/.*"
      },
      "http_methods": {
        "type": "array",
        "items": {"type": "string", "enum": ["POST", "PUT"]},
        "default": ["POST"],
        "title": "Allowed HTTP Methods"
      },
      "authentication_type": {
        "type": "string",
        "enum": ["none", "api_key", "basic_auth", "bearer_token", "oauth2"],
        "default": "api_key",
        "title": "Authentication Type"
      },
      "api_key_header": {
        "type": "string",
        "default": "X-API-Key",
        "title": "API Key Header Name"
      },
      "api_key": {
        "type": "string",
        "format": "password",
        "title": "API Key"
      },
      "basic_auth_username": {
        "type": "string",
        "title": "Basic Auth Username"
      },
      "basic_auth_password": {
        "type": "string",
        "format": "password",
        "title": "Basic Auth Password"
      },
      "content_type": {
        "type": "string",
        "enum": ["text/plain", "application/json", "application/xml", "application/hl7-v2"],
        "default": "text/plain",
        "title": "Expected Content-Type"
      },
      "max_body_size_mb": {
        "type": "integer",
        "default": 10,
        "title": "Max Body Size (MB)"
      },
      "enable_cors": {
        "type": "boolean",
        "default": false,
        "title": "Enable CORS"
      },
      "cors_allowed_origins": {
        "type": "array",
        "items": {"type": "string"},
        "title": "CORS Allowed Origins",
        "examples": [["https://example.com", "https://app.example.com"]]
      }
    }
  },

  "parameter_groups": {
    "basic": ["endpoint_path", "http_methods", "content_type"],
    "security": ["authentication_type", "api_key_header", "api_key", "basic_auth_username", "basic_auth_password"],
    "advanced": ["max_body_size_mb", "enable_cors", "cors_allowed_origins"]
  }
}
```

### **SFTP Connector** (Inbound - Cron-based)

```json
{
  "type_name": "sftp_inbound",
  "category": "inbound",
  "display_name": "SFTP File Transfer",
  "description": "Download files from SFTP server (scheduled polling)",
  "icon": "🔐",
  "mode": "pull",
  "supports_cron": true,
  "requires_auth": true,

  "config_schema": {
    "type": "object",
    "required": ["host", "port", "username", "remote_directory"],
    "properties": {
      "host": {
        "type": "string",
        "title": "SFTP Host",
        "description": "Hostname or IP address",
        "examples": ["sftp.example.com", "192.168.1.100"]
      },
      "port": {
        "type": "integer",
        "default": 22,
        "title": "Port"
      },
      "username": {
        "type": "string",
        "title": "Username"
      },
      "authentication_method": {
        "type": "string",
        "enum": ["password", "ssh_key"],
        "default": "password",
        "title": "Authentication Method"
      },
      "password": {
        "type": "string",
        "format": "password",
        "title": "Password"
      },
      "ssh_private_key_path": {
        "type": "string",
        "title": "SSH Private Key Path"
      },
      "ssh_passphrase": {
        "type": "string",
        "format": "password",
        "title": "SSH Key Passphrase"
      },
      "remote_directory": {
        "type": "string",
        "title": "Remote Directory",
        "description": "Directory to download files from",
        "examples": ["/uploads/hl7", "/data/inbound"]
      },
      "file_pattern": {
        "type": "string",
        "default": "*.hl7",
        "title": "File Pattern"
      },
      "delete_after_download": {
        "type": "boolean",
        "default": false,
        "title": "Delete Files After Download"
      },
      "local_staging_directory": {
        "type": "string",
        "title": "Local Staging Directory",
        "description": "Temporary directory for downloaded files"
      }
    }
  },

  "parameter_groups": {
    "basic": ["host", "port", "username", "remote_directory"],
    "security": ["authentication_method", "password", "ssh_private_key_path", "ssh_passphrase"],
    "processing": ["file_pattern", "delete_after_download", "local_staging_directory"]
  }
}
```

---

## 🏛️ MVC Architecture

### **Models** (Go)

```go
// models/connectivity_models.go

package models

import (
	"encoding/json"
	"time"
)

// ConnectivityType represents an OOB connector definition
type ConnectivityType struct {
	ID                   string                 `json:"id" db:"id"`
	TypeName             string                 `json:"type_name" db:"type_name"`
	Category             string                 `json:"category" db:"category"` // "inbound" | "outbound"
	DisplayName          string                 `json:"display_name" db:"display_name"`
	Description          string                 `json:"description" db:"description"`
	Icon                 string                 `json:"icon" db:"icon"`
	Mode                 string                 `json:"mode" db:"mode"` // "push" | "pull" | "stream"
	SupportsCron         bool                   `json:"supports_cron" db:"supports_cron"`
	RequiresAuth         bool                   `json:"requires_auth" db:"requires_auth"`
	IsBidirectional      bool                   `json:"is_bidirectional" db:"is_bidirectional"`
	ImplementationClass  string                 `json:"implementation_class" db:"implementation_class"`
	ConfigSchema         json.RawMessage        `json:"config_schema" db:"config_schema"`
	DefaultConfig        json.RawMessage        `json:"default_config" db:"default_config"`
	WizardTemplate       string                 `json:"wizard_template" db:"wizard_template"`
	ParameterGroups      json.RawMessage        `json:"parameter_groups" db:"parameter_groups"`
	ValidationRules      json.RawMessage        `json:"validation_rules" db:"validation_rules"`
	IsActive             bool                   `json:"is_active" db:"is_active"`
	IsBeta               bool                   `json:"is_beta" db:"is_beta"`
	Priority             int                    `json:"priority" db:"priority"`
	Version              string                 `json:"version" db:"version"`
	DocumentationURL     string                 `json:"documentation_url" db:"documentation_url"`
	CreatedAt            time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at" db:"updated_at"`
}

// InterfaceConnectivity represents connectivity configuration for an interface
type InterfaceConnectivity struct {
	ID                           string          `json:"id" db:"id"`
	InterfaceID                  string          `json:"interface_id" db:"interface_id"`

	// Source (Inbound)
	SourceConnectivityTypeID     string          `json:"source_connectivity_type_id" db:"source_connectivity_type_id"`
	SourceConfig                 json.RawMessage `json:"source_config" db:"source_config"`
	SourceEnabled                bool            `json:"source_enabled" db:"source_enabled"`

	// Cron Scheduling
	CronEnabled                  bool            `json:"cron_enabled" db:"cron_enabled"`
	CronExpression               string          `json:"cron_expression" db:"cron_expression"`
	CronTimezone                 string          `json:"cron_timezone" db:"cron_timezone"`
	NextRunAt                    *time.Time      `json:"next_run_at" db:"next_run_at"`
	LastRunAt                    *time.Time      `json:"last_run_at" db:"last_run_at"`
	LastRunStatus                string          `json:"last_run_status" db:"last_run_status"`

	// Target (Outbound)
	TargetConnectivityTypeID     string          `json:"target_connectivity_type_id" db:"target_connectivity_type_id"`
	TargetConfig                 json.RawMessage `json:"target_config" db:"target_config"`
	TargetEnabled                bool            `json:"target_enabled" db:"target_enabled"`

	// Multi-target support (future)
	AdditionalTargets            json.RawMessage `json:"additional_targets" db:"additional_targets"`

	// Connection State
	ConnectionStatus             string          `json:"connection_status" db:"connection_status"`
	LastConnectionTest           *time.Time      `json:"last_connection_test" db:"last_connection_test"`
	LastConnectionError          string          `json:"last_connection_error" db:"last_connection_error"`

	// Metadata
	CreatedAt                    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt                    time.Time       `json:"updated_at" db:"updated_at"`
}

// CronJob represents a scheduled job for pull-based connectors
type CronJob struct {
	ID                          string     `json:"id" db:"id"`
	InterfaceConnectivityID     string     `json:"interface_connectivity_id" db:"interface_connectivity_id"`
	CronExpression              string     `json:"cron_expression" db:"cron_expression"`
	Timezone                    string     `json:"timezone" db:"timezone"`
	IsEnabled                   bool       `json:"is_enabled" db:"is_enabled"`
	NextExecution               *time.Time `json:"next_execution" db:"next_execution"`
	LastExecution               *time.Time `json:"last_execution" db:"last_execution"`
	ExecutionCount              int        `json:"execution_count" db:"execution_count"`
	FailureCount                int        `json:"failure_count" db:"failure_count"`
	ConsecutiveFailures         int        `json:"consecutive_failures" db:"consecutive_failures"`
	MaxRetries                  int        `json:"max_retries" db:"max_retries"`
	RetryDelaySeconds           int        `json:"retry_delay_seconds" db:"retry_delay_seconds"`
	CircuitBreakerThreshold     int        `json:"circuit_breaker_threshold" db:"circuit_breaker_threshold"`
	IsCircuitBroken             bool       `json:"is_circuit_broken" db:"is_circuit_broken"`
	CreatedAt                   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt                   time.Time  `json:"updated_at" db:"updated_at"`
}

// ConnectivityExecutionLog represents execution history
type ConnectivityExecutionLog struct {
	ID                          string          `json:"id" db:"id"`
	InterfaceConnectivityID     string          `json:"interface_connectivity_id" db:"interface_connectivity_id"`
	InterfaceID                 string          `json:"interface_id" db:"interface_id"`
	ExecutionType               string          `json:"execution_type" db:"execution_type"`
	StartedAt                   time.Time       `json:"started_at" db:"started_at"`
	CompletedAt                 *time.Time      `json:"completed_at" db:"completed_at"`
	Status                      string          `json:"status" db:"status"`
	MessagesRetrieved           int             `json:"messages_retrieved" db:"messages_retrieved"`
	MessagesProcessed           int             `json:"messages_processed" db:"messages_processed"`
	MessagesFailed              int             `json:"messages_failed" db:"messages_failed"`
	ErrorDetails                json.RawMessage `json:"error_details" db:"error_details"`
	DurationMs                  int             `json:"duration_ms" db:"duration_ms"`
	TriggeredBy                 string          `json:"triggered_by" db:"triggered_by"`
	ExecutionMetadata           json.RawMessage `json:"execution_metadata" db:"execution_metadata"`
}
```

### **Controllers** (Go)

```go
// controllers/connectivity_controller.go

package controllers

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"ezhealthkonnect/services"
)

type ConnectivityController struct {
	connectivityService *services.ConnectivityService
	cronService         *services.CronSchedulerService
}

func NewConnectivityController(
	connectivityService *services.ConnectivityService,
	cronService *services.CronSchedulerService,
) *ConnectivityController {
	return &ConnectivityController{
		connectivityService: connectivityService,
		cronService:         cronService,
	}
}

// RegisterRoutes registers all connectivity routes
func (cc *ConnectivityController) RegisterRoutes(router *gin.RouterGroup) {
	connectivity := router.Group("/connectivity")
	{
		// Connectivity Types (OOB Connectors)
		connectivity.GET("/types", cc.ListConnectivityTypes)
		connectivity.GET("/types/:type_name", cc.GetConnectivityType)
		connectivity.GET("/types/category/:category", cc.ListByCategory)

		// Interface Connectivity Configuration
		connectivity.POST("/interfaces/:interface_id", cc.CreateInterfaceConnectivity)
		connectivity.GET("/interfaces/:interface_id", cc.GetInterfaceConnectivity)
		connectivity.PUT("/interfaces/:interface_id", cc.UpdateInterfaceConnectivity)
		connectivity.DELETE("/interfaces/:interface_id", cc.DeleteInterfaceConnectivity)

		// Connection Testing
		connectivity.POST("/interfaces/:interface_id/test-source", cc.TestSourceConnection)
		connectivity.POST("/interfaces/:interface_id/test-target", cc.TestTargetConnection)

		// Cron Management
		connectivity.POST("/interfaces/:interface_id/cron/enable", cc.EnableCron)
		connectivity.POST("/interfaces/:interface_id/cron/disable", cc.DisableCron)
		connectivity.POST("/interfaces/:interface_id/cron/trigger", cc.TriggerCronManually)
		connectivity.GET("/interfaces/:interface_id/cron/status", cc.GetCronStatus)

		// Execution Logs
		connectivity.GET("/interfaces/:interface_id/executions", cc.GetExecutionHistory)
		connectivity.GET("/executions/:execution_id", cc.GetExecutionDetails)

		// Monitoring
		connectivity.GET("/stats", cc.GetConnectivityStats)
		connectivity.GET("/active-jobs", cc.GetActiveJobs)
	}
}

// Implementation of each endpoint...
// (Full implementation in separate file)
```

### **Services** (Go)

```go
// services/connectivity_service.go
// services/connector_factory.go
// services/cron_scheduler_service.go
// services/connectors/file_listener_connector.go
// services/connectors/database_poll_connector.go
// services/connectors/http_rest_connector.go
// services/connectors/sftp_connector.go
// ... etc.

// Full implementation follows OOB pattern with pluggable connectors
```

---

## 📅 Cron Expression Support

### **Cron Syntax**
```
┌───────────── minute (0 - 59)
│ ┌───────────── hour (0 - 23)
│ │ ┌───────────── day of month (1 - 31)
│ │ │ ┌───────────── month (1 - 12)
│ │ │ │ ┌───────────── day of week (0 - 6) (Sunday=0)
│ │ │ │ │
* * * * *
```

### **Common Examples**
```
*/5 * * * *      Every 5 minutes
0 * * * *        Every hour at minute 0
0 0 * * *        Daily at midnight
0 9 * * 1-5      Weekdays at 9 AM
0 */4 * * *      Every 4 hours
*/15 9-17 * * *  Every 15 min during business hours (9 AM - 5 PM)
```

### **Cron Scheduler Implementation**
- Uses `github.com/robfig/cron/v3` library
- Timezone-aware scheduling
- Circuit breaker pattern for repeated failures
- Graceful shutdown support
- Concurrent job execution with worker pools

---

## 🎨 UI/Wizard Integration

### **Wizard Step: Connectivity Configuration**

```javascript
// Wizard Step 2: Source & Target Selection

{
  step: 2,
  title: "Configure Connectivity",
  sections: [
    {
      title: "Source (Inbound)",
      description: "How will messages arrive?",

      // Dynamic connector selection
      connector_selection: {
        category: "inbound",
        render_as: "cards",  // Show as visual cards with icons
        fetch_from: "/api/connectivity/types/category/inbound"
      },

      // Dynamic parameter form (based on selected connector's config_schema)
      parameter_form: {
        source: "selected_connector.config_schema",
        group_by: "parameter_groups",  // Basic, Advanced, Security
        validation: "selected_connector.validation_rules"
      },

      // Cron configuration (if connector.supports_cron)
      cron_configuration: {
        enabled: true,
        presets: [
          { label: "Every 5 minutes", value: "*/5 * * * *" },
          { label: "Every hour", value: "0 * * * *" },
          { label: "Daily at 2 AM", value: "0 2 * * *" },
          { label: "Weekdays at 9 AM", value: "0 9 * * 1-5" }
        ],
        custom_expression: true,
        test_connection: true
      }
    },

    {
      title: "Target (Outbound)",
      description: "Where should transformed messages go?",

      connector_selection: {
        category: "outbound",
        render_as: "cards",
        fetch_from: "/api/connectivity/types/category/outbound"
      },

      parameter_form: {
        source: "selected_connector.config_schema",
        group_by: "parameter_groups",
        validation: "selected_connector.validation_rules"
      },

      test_connection: true
    }
  ]
}
```

---

## 🚀 Implementation Phases

### **Phase 1: Foundation (Week 1-2)**
- ✅ Database migration V26
- ✅ Models and base service structure
- ✅ Connectivity types seed data (5-7 connectors)
- ✅ Basic CRUD API endpoints
- ✅ Wizard UI integration

### **Phase 2: Core Connectors (Week 3-4)**
- ✅ File Listener connector
- ✅ HTTP/REST inbound connector
- ✅ SFTP connector
- ✅ Database Poll connector
- ✅ HTTP/HTTPS outbound connector
- ✅ File Writer outbound connector

### **Phase 3: Cron Scheduler (Week 5)**
- ✅ Cron job scheduler service
- ✅ Circuit breaker implementation
- ✅ Execution logging
- ✅ Manual trigger support
- ✅ Monitoring dashboard

### **Phase 4: Advanced Features (Week 6+)**
- 📋 Kafka/RabbitMQ connectors
- 📋 Cloud storage connectors (S3, Azure, GCS)
- 📋 Email/SMTP connector
- 📋 WebSocket connector
- 📋 Multi-target routing

---

## 📊 Success Metrics

- **Coverage**: 15+ connectivity types by end of Phase 4
- **OOB Quality**: All connectors have complete JSON schemas
- **Reliability**: 99%+ cron job execution success
- **Performance**: <100ms connector factory resolution
- **Usability**: Non-technical users can configure any connector via wizard

---

*Architecture Design: 2025-10-25*
*Conversation Context: Multi-connectivity architecture with OOB, MVC, and cron support*
