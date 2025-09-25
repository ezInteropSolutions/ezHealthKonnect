# MongoDB Configuration Schema Design

## Overview
Interface-Centric Configuration Engine schema for MongoDB backend with version management, hot-reload capability, and migration strategy from PostgreSQL.

## 1. Interface Configuration Document Schema

### Core Structure
```javascript
{
  _id: ObjectId("..."),

  // Identity and Metadata
  interface_id: "uuid-from-postgresql",
  name: "Hospital ADT Interface",
  description: "Main hospital ADT message processing",
  version: "1.2.0",
  status: "active", // draft, active, paused, stopped, error

  // Version Management
  version_history: [
    {
      version: "1.1.0",
      created_at: ISODate("..."),
      created_by: "user-uuid",
      changes: ["Updated validation rules", "Added new transformation"],
      config_snapshot: ObjectId("config-history-id")
    }
  ],

  // Processing Pipeline Configuration
  pipeline: {

    // 1. INPUT LAYER
    input: {
      type: "mllp", // mllp, file, api, queue
      connector_config: {
        host: "0.0.0.0",
        port: 6661,
        timeout: 30000,
        max_connections: 10,
        encoding: "utf-8"
      },
      validation: {
        enabled: true,
        rules: [
          {
            rule_type: "required_field",
            field_path: "MSH.3",
            error_action: "reject" // reject, warn, continue
          }
        ]
      },
      preprocessing: {
        enabled: true,
        steps: [
          {
            type: "normalize_encoding",
            config: { target_encoding: "utf-8" }
          },
          {
            type: "remove_whitespace",
            config: { trim_only: false }
          }
        ]
      }
    },

    // 2. VALIDATION LAYER
    validation: {
      schema_validation: {
        enabled: true,
        schema_type: "hl7_v2.5",
        strict_mode: false
      },
      business_rules: [
        {
          rule_id: "patient_id_required",
          condition: "message_type == 'ADT^A01'",
          validation: "PID.3.1 != null",
          error_message: "Patient ID is required for ADT messages",
          severity: "error"
        }
      ],
      custom_validators: [
        {
          name: "hospital_specific_validation",
          type: "javascript",
          code: "function validate(message) { /* custom logic */ }"
        }
      ]
    },

    // 3. TRANSFORMATION LAYER
    transformation: {
      engine: "hl7_to_fhir", // hl7_to_fhir, custom, passthrough
      mapping_template: "standard_adt_v4", // Reference to template
      custom_mappings: [
        {
          source_field: "PID.5",
          target_field: "Patient.name",
          transformation: {
            type: "name_parser",
            config: {
              format: "family^given^middle",
              case_transform: "title"
            }
          }
        }
      ],
      post_processing: [
        {
          type: "add_metadata",
          config: {
            source_system: "hospital_adt",
            processing_timestamp: "current_time"
          }
        }
      ]
    },

    // 4. BUSINESS LOGIC LAYER
    business_logic: {
      rules_engine: {
        enabled: true,
        rules: [
          {
            rule_id: "duplicate_patient_check",
            condition: "message_type == 'ADT^A01'",
            action: {
              type: "api_call",
              endpoint: "/api/patient/check-duplicate",
              on_duplicate: "merge_records"
            }
          }
        ]
      },
      workflow_automation: [
        {
          trigger: "message_processed",
          actions: [
            {
              type: "notify",
              recipients: ["admin@hospital.com"],
              template: "patient_admission_notification"
            }
          ]
        }
      ]
    },

    // 5. DESTINATION LAYER
    destinations: [
      {
        destination_id: "epic_fhir_server",
        type: "fhir_api",
        config: {
          base_url: "https://epic.hospital.com/fhir/R4",
          auth: {
            type: "oauth2",
            client_id: "ezhealth_client",
            scope: "system/Patient.write"
          }
        },
        routing_rules: [
          {
            condition: "message_type == 'ADT^A01'",
            resource_type: "Patient",
            operation: "create_or_update"
          }
        ],
        error_handling: {
          retry_count: 3,
          retry_delay: 5000,
          dead_letter_queue: true
        }
      }
    ]
  },

  // Performance and Monitoring
  monitoring: {
    metrics_enabled: true,
    alert_thresholds: {
      error_rate: 0.05,
      processing_time_ms: 5000,
      queue_depth: 100
    },
    retention_policy: {
      raw_messages: 90, // days
      processed_messages: 30,
      error_logs: 365
    }
  },

  // Metadata
  created_at: ISODate("..."),
  updated_at: ISODate("..."),
  created_by: "user-uuid",
  updated_by: "user-uuid",

  // Hot-reload tracking
  last_loaded_at: ISODate("..."),
  config_hash: "sha256-hash-of-config",
  active_instances: ["node-1", "node-2"] // Which processing nodes have this config loaded
}
```

## 2. Mapping Templates Collection

### Standard HL7-FHIR Templates
```javascript
{
  _id: ObjectId("..."),
  template_id: "standard_adt_v4",
  name: "Standard ADT to FHIR R4",
  version: "4.0.0",
  message_types: ["ADT^A01", "ADT^A02", "ADT^A03"],

  mappings: [
    {
      source_path: "MSH.3",
      target_path: "MessageHeader.source.name",
      transformation: "direct_copy",
      required: true
    },
    {
      source_path: "PID.3.1",
      target_path: "Patient.identifier[0].value",
      transformation: "patient_id_formatter",
      validation: {
        pattern: "^[0-9]{8}$",
        required: true
      }
    }
  ],

  created_at: ISODate("..."),
  is_system_template: true,
  usage_count: 45 // How many interfaces use this
}
```

## 3. Configuration History Collection

### Version Control and Rollback
```javascript
{
  _id: ObjectId("..."),
  interface_id: "uuid",
  version: "1.1.0",
  config_snapshot: { /* Full configuration at this version */ },
  changes: [
    {
      type: "field_added",
      path: "pipeline.validation.business_rules",
      description: "Added patient ID validation rule"
    }
  ],
  created_at: ISODate("..."),
  created_by: "user-uuid"
}
```

## 4. Runtime State Collection

### Active Instance Tracking
```javascript
{
  _id: ObjectId("..."),
  interface_id: "uuid",
  instance_id: "processing-node-1",
  status: "running", // loading, running, stopping, error
  config_version: "1.2.0",
  config_hash: "sha256-hash",
  last_heartbeat: ISODate("..."),
  performance_stats: {
    messages_processed: 1543,
    avg_processing_time_ms: 245,
    error_count: 2,
    uptime_seconds: 86400
  }
}
```

## 5. Index Strategy

### Performance Indexes
```javascript
// Interface configurations
db.interface_configs.createIndex({ "interface_id": 1 }, { unique: true })
db.interface_configs.createIndex({ "status": 1, "updated_at": -1 })
db.interface_configs.createIndex({ "config_hash": 1 })

// Templates
db.mapping_templates.createIndex({ "template_id": 1 }, { unique: true })
db.mapping_templates.createIndex({ "message_types": 1 })

// Version history
db.config_history.createIndex({ "interface_id": 1, "version": -1 })

// Runtime state
db.runtime_state.createIndex({ "interface_id": 1, "instance_id": 1 })
db.runtime_state.createIndex({ "last_heartbeat": 1 })
```

## 6. Migration Strategy from PostgreSQL

### Data Mapping
- PostgreSQL `interfaces` table → MongoDB `interface_configs` collection
- PostgreSQL `hl7_fhir_templates` → MongoDB `mapping_templates` collection
- PostgreSQL `interface_message_mappings` → Embedded in `interface_configs.pipeline.transformation`

### Migration Steps
1. Export PostgreSQL interface configurations
2. Transform to MongoDB schema format
3. Preserve existing HL7-FHIR mappings
4. Maintain backward compatibility during transition
5. Gradual rollout with fallback to PostgreSQL

## 7. Hot-Reload Implementation

### Configuration Change Detection
1. MongoDB Change Streams on `interface_configs` collection
2. Config hash comparison for detecting changes
3. Graceful reload without dropping active connections
4. Version rollback capability

### Performance Considerations
- Configuration caching in memory
- Lazy loading of large mapping templates
- Bulk configuration updates
- Connection pooling and reuse