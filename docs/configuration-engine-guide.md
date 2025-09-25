# Interface-Centric Configuration Engine

## Overview

The Interface-Centric Configuration Engine is a MongoDB-backed configuration management system that transforms ezHealthKonnect into a Universal Interface Engine. Each interface becomes a self-contained processing pipeline with hot-reload capability, dynamic execution, and complete isolation.

## 🏗️ Architecture

### Core Components

1. **Configuration Manager** (`services/config_manager.go`)
   - MongoDB backend for configuration storage
   - Hot-reload capability with change streams
   - Version management and rollback
   - Configuration validation and caching

2. **Interface Engine** (`services/interface_engine.go`)
   - Dynamic pipeline execution
   - Pluggable layer processors
   - State management and metrics
   - Error handling and recovery

3. **Processor Library** (`services/processors.go`)
   - Input processors (MLLP, File, API, Queue)
   - Validators (HL7, FHIR, Custom)
   - Transformers (HL7-to-FHIR, Custom, Passthrough)
   - Business logic processors
   - Destination processors (FHIR API, File, Database, Queue)

4. **Migration Service** (`services/migration_service.go`)
   - PostgreSQL to MongoDB migration
   - Data transformation and mapping
   - Validation and rollback capability

## 🚀 Getting Started

### 1. Environment Setup

```bash
# MongoDB Configuration
export MONGODB_URI="mongodb://localhost:27017"
export MONGODB_DATABASE="ezhealthkonnect"

# PostgreSQL (for migration)
export DB_HOST="localhost"
export DB_PORT="5432"
export DB_NAME="ezhealthkonnect"
export DB_USER="postgres"
export DB_PASSWORD="your_password"
```

### 2. MongoDB Installation

```bash
# Install MongoDB (example for Ubuntu)
wget -qO - https://www.mongodb.org/static/pgp/server-7.0.asc | sudo apt-key add -
echo "deb [ arch=amd64,arm64 ] https://repo.mongodb.org/apt/ubuntu jammy/mongodb-org/7.0 multiverse" | sudo tee /etc/apt/sources.list.d/mongodb-org-7.0.list
sudo apt-get update
sudo apt-get install -y mongodb-org

# Start MongoDB
sudo systemctl start mongod
sudo systemctl enable mongod
```

### 3. Start the System

```bash
# Start both Node.js and Go services
npm run dev:all

# Or start Go backend only
go run main.go
```

## 📋 Configuration Schema

### Interface Configuration Document

```javascript
{
  // Identity
  interface_id: "uuid",
  name: "Hospital ADT Interface",
  description: "Main hospital ADT processing",
  version: "1.2.0",
  status: "active",

  // Processing Pipeline
  pipeline: {
    // Input Layer
    input: {
      type: "mllp",
      connector_config: {
        host: "0.0.0.0",
        port: 6661,
        timeout: 30000
      },
      validation: { enabled: true, rules: [...] },
      preprocessing: { enabled: true, steps: [...] }
    },

    // Validation Layer
    validation: {
      schema_validation: {
        enabled: true,
        schema_type: "hl7_v2.5",
        strict_mode: false
      },
      business_rules: [...],
      custom_validators: [...]
    },

    // Transformation Layer
    transformation: {
      engine: "hl7_to_fhir",
      mapping_template: "standard_adt_v4",
      custom_mappings: [...],
      post_processing: [...]
    },

    // Business Logic Layer
    business_logic: {
      rules_engine: { enabled: true, rules: [...] },
      workflow_automation: [...]
    },

    // Destination Layer
    destinations: [
      {
        destination_id: "epic_fhir",
        type: "fhir_api",
        config: { base_url: "...", auth: {...} },
        routing_rules: [...],
        error_handling: { retry_count: 3, ... }
      }
    ]
  },

  // Monitoring
  monitoring: {
    metrics_enabled: true,
    alert_thresholds: { error_rate: 0.05, ... },
    retention_policy: { raw_messages: 90, ... }
  }
}
```

## 🔄 Migration from PostgreSQL

### Automatic Migration

```bash
# Start migration (dry run)
curl -X POST "http://localhost:8080/api/config/migrate?dry_run=true"

# Start actual migration
curl -X POST "http://localhost:8080/api/config/migrate"

# Check migration status
curl "http://localhost:8080/api/config/migrate/status"
```

### Manual Migration Steps

```go
// Initialize migration service
migrationService := services.NewMigrationService(pgDB, configManager, false)

// Run migration
stats, err := migrationService.MigrateAll(context.Background())
if err != nil {
    log.Printf("Migration failed: %v", err)
}

// Validate results
err = migrationService.ValidateMigration(context.Background(), stats)
```

## 🌐 API Endpoints

### Configuration Management

```http
# Get all configurations
GET /api/config/interfaces

# Get specific configuration
GET /api/config/interfaces/:id

# Create new configuration
POST /api/config/interfaces
Content-Type: application/json
{
  "name": "New Interface",
  "pipeline": { ... }
}

# Update configuration
PUT /api/config/interfaces/:id

# Delete configuration
DELETE /api/config/interfaces/:id

# Reload configuration (hot-reload)
POST /api/config/interfaces/:id/reload
```

### Template Management

```http
# Get mapping templates
GET /api/config/templates

# Get specific template
GET /api/config/templates/:id

# Create template
POST /api/config/templates

# Update template
PUT /api/config/templates/:id
```

### Runtime Monitoring

```http
# Get runtime statistics
GET /api/config/runtime/stats

# Get active processes
GET /api/config/runtime/active

# Get interface-specific stats
GET /api/config/interfaces/:id/stats

# Health check
GET /api/config/health
```

## 🔥 Hot-Reload Capability

### How It Works

1. **Change Detection**: MongoDB change streams monitor configuration updates
2. **Cache Invalidation**: Configuration cache is automatically invalidated
3. **Instance Notification**: Active processing instances are notified
4. **Graceful Reload**: Configurations are reloaded without dropping connections

### Manual Hot-Reload

```bash
# Reload specific interface
curl -X POST "http://localhost:8080/api/config/interfaces/uuid/reload"

# Reload all configurations
curl -X POST "http://localhost:8080/api/config/reload/all"
```

## 🔧 Processor Development

### Creating Custom Input Processor

```go
type CustomInputProcessor struct{}

func (p *CustomInputProcessor) ProcessInput(ctx context.Context, config InputConfig, message []byte) (*ProcessedMessage, error) {
    // Parse input message
    parsedContent := map[string]interface{}{
        "raw_message": string(message),
        "custom_field": "value",
    }

    return &ProcessedMessage{
        ParsedContent: parsedContent,
        Metadata: MessageMetadata{
            SourceType: "custom",
            MessageSize: len(message),
        },
    }, nil
}

func (p *CustomInputProcessor) Start(ctx context.Context, config InputConfig) error {
    // Initialize processor
    return nil
}

func (p *CustomInputProcessor) Stop(ctx context.Context) error {
    // Cleanup processor
    return nil
}

// Register processor
engine.RegisterInputProcessor("custom", &CustomInputProcessor{})
```

### Creating Custom Transformer

```go
type CustomTransformer struct{}

func (t *CustomTransformer) Transform(ctx context.Context, config TransformationConfig, message *ProcessedMessage) (*TransformedMessage, error) {
    // Apply transformations
    transformedContent := make(map[string]interface{})

    for _, mapping := range config.CustomMappings {
        // Apply field mappings
        value := extractValue(message.ParsedContent, mapping.SourceField)
        transformedValue := applyTransformation(value, mapping.Transformation)
        setValue(transformedContent, mapping.TargetField, transformedValue)
    }

    return &TransformedMessage{
        ProcessedMessage: message,
        TransformedContent: transformedContent,
        TargetFormat: "custom",
    }, nil
}

// Register transformer
engine.RegisterTransformer("custom", &CustomTransformer{})
```

## 📊 Monitoring and Metrics

### Built-in Metrics

- **Processing Statistics**: Message counts, success/failure rates
- **Performance Metrics**: Processing times, throughput
- **Error Tracking**: Error counts, failure patterns
- **Resource Usage**: Memory, CPU, connection pools

### Custom Metrics

```go
// Record custom metric
engine.metricsCollector.metrics.Store("custom.metric", value)

// Get interface statistics
stats, exists := engine.GetInterfaceStats(interfaceID)
if exists {
    log.Printf("Total processed: %d", stats.TotalProcessed)
    log.Printf("Average time: %.2fms", stats.AvgProcessingTime)
}
```

## 🛠️ Configuration Examples

### Basic HL7-to-FHIR Interface

```json
{
  "name": "Hospital ADT Interface",
  "description": "Process ADT messages from hospital system",
  "version": "1.0.0",
  "status": "active",
  "pipeline": {
    "input": {
      "type": "mllp",
      "connector_config": {
        "host": "0.0.0.0",
        "port": 6661,
        "timeout": 30000,
        "max_connections": 10
      }
    },
    "validation": {
      "schema_validation": {
        "enabled": true,
        "schema_type": "hl7_v2.5"
      }
    },
    "transformation": {
      "engine": "hl7_to_fhir",
      "mapping_template": "standard_adt_v4"
    },
    "destinations": [
      {
        "destination_id": "epic_fhir",
        "type": "fhir_api",
        "config": {
          "base_url": "https://epic.hospital.com/fhir/R4",
          "auth": {
            "type": "oauth2",
            "client_id": "ezhealth_client"
          }
        }
      }
    ]
  }
}
```

### File Processing Interface

```json
{
  "name": "Batch File Processor",
  "pipeline": {
    "input": {
      "type": "file",
      "connector_config": {
        "watch_directory": "/data/input",
        "file_pattern": "*.hl7",
        "polling_interval": 5000
      }
    },
    "transformation": {
      "engine": "custom",
      "custom_mappings": [
        {
          "source_field": "PID.5",
          "target_field": "Patient.name",
          "transformation": {
            "type": "name_parser",
            "config": { "format": "family^given" }
          }
        }
      ]
    },
    "destinations": [
      {
        "destination_id": "output_file",
        "type": "file",
        "config": {
          "output_directory": "/data/output",
          "file_format": "json"
        }
      }
    ]
  }
}
```

## 🔐 Security Considerations

### Authentication

- Configure OAuth2 for FHIR API destinations
- Use TLS for MLLP connections
- Implement API key authentication for REST endpoints

### Data Protection

- Encrypt sensitive configuration data
- Implement field-level masking for PHI
- Configure retention policies for message data

### Access Control

- Role-based access to configurations
- Audit logging for all configuration changes
- IP-based access restrictions

## 🚨 Troubleshooting

### Common Issues

1. **MongoDB Connection Failed**
   ```bash
   # Check MongoDB status
   sudo systemctl status mongod

   # Check connection
   mongo --host localhost --port 27017
   ```

2. **Configuration Not Loading**
   ```bash
   # Check configuration syntax
   curl "http://localhost:8080/api/config/interfaces/uuid"

   # Force reload
   curl -X POST "http://localhost:8080/api/config/interfaces/uuid/reload"
   ```

3. **Processing Errors**
   ```bash
   # Check interface statistics
   curl "http://localhost:8080/api/config/interfaces/uuid/stats"

   # Check active processes
   curl "http://localhost:8080/api/config/runtime/active"
   ```

### Log Analysis

```bash
# Check Go service logs
tail -f logs/ezhealthkonnect.log

# Check Node.js service logs
tail -f logs/node-service.log

# Check MongoDB logs
tail -f /var/log/mongodb/mongod.log
```

## 🔄 Migration Troubleshooting

### Rollback Procedure

```bash
# Stop processing
curl -X POST "http://localhost:8080/api/config/interfaces/uuid/stop"

# Restore from PostgreSQL
# (Manual process - restore from backup)

# Restart with PostgreSQL backend
export MONGODB_URI=""
go run main.go
```

### Data Validation

```bash
# Compare record counts
curl "http://localhost:8080/api/config/migrate/status"

# Validate random configurations
curl -X POST "http://localhost:8080/api/config/migrate/validate"
```

## 📈 Performance Optimization

### Configuration Optimization

- Use specific routing rules to reduce processing
- Implement caching for frequently accessed data
- Configure appropriate retention policies

### MongoDB Optimization

```javascript
// Create performance indexes
db.interface_configs.createIndex({ "status": 1, "updated_at": -1 })
db.interface_configs.createIndex({ "config_hash": 1 })

// Monitor query performance
db.interface_configs.find().explain("executionStats")
```

### Memory Management

- Configure appropriate cache sizes
- Monitor active message counts
- Implement message batching for high volume

## 🎯 Best Practices

### Configuration Design

1. **Use Templates**: Leverage mapping templates for standard transformations
2. **Modular Design**: Break complex pipelines into smaller, testable components
3. **Error Handling**: Configure comprehensive error handling and retry logic
4. **Monitoring**: Enable metrics and configure appropriate alerts

### Operations

1. **Gradual Rollout**: Test configurations in development before production
2. **Version Control**: Use semantic versioning for configuration changes
3. **Backup Strategy**: Regular backups of MongoDB configuration data
4. **Monitoring**: Continuous monitoring of processing performance

### Development

1. **Testing**: Implement unit tests for custom processors
2. **Documentation**: Document custom transformations and business rules
3. **Code Review**: Review configuration changes before deployment
4. **Performance**: Profile custom processors for performance impact

## 🎓 Advanced Topics

### Custom Business Rules Engine

```go
type CustomRulesEngine struct {
    rules []BusinessRule
}

func (re *CustomRulesEngine) ExecuteRules(message *ProcessedMessage) error {
    for _, rule := range re.rules {
        if err := re.evaluateRule(rule, message); err != nil {
            return err
        }
    }
    return nil
}

func (re *CustomRulesEngine) evaluateRule(rule BusinessRule, message *ProcessedMessage) error {
    // Implement complex rule evaluation logic
    // Could integrate with external rules engines like Drools
    return nil
}
```

### Workflow Automation

```go
type WorkflowEngine struct {
    workflows map[string]WorkflowDefinition
}

func (we *WorkflowEngine) TriggerWorkflow(trigger string, message *ProcessedMessage) error {
    workflow, exists := we.workflows[trigger]
    if !exists {
        return nil // No workflow for this trigger
    }

    // Execute workflow steps
    for _, step := range workflow.Steps {
        if err := we.executeStep(step, message); err != nil {
            return err
        }
    }

    return nil
}
```

### Distributed Processing

```go
// Configure for multiple processing nodes
type DistributedEngine struct {
    nodeID    string
    instances map[string]*ProcessingInstance
}

func (de *DistributedEngine) DistributeLoad(message *ProcessedMessage) error {
    // Implement load balancing logic
    selectedNode := de.selectNode(message)
    return selectedNode.ProcessMessage(message)
}
```

This completes the comprehensive Interface-Centric Configuration Engine implementation for ezHealthKonnect. The system provides a production-ready foundation for universal interface processing with MongoDB backend, hot-reload capability, and seamless migration from the existing PostgreSQL system.