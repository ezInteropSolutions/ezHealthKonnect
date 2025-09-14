# Schema Summary (Generated 2025-09-08T13:39:50.717593)

**Database Overview:** 15 tables with 214 total columns

## audit_logs
- **PK**: id
- **FK**: user_id → users.id
- id (uuid NOT NULL)
- user_id (uuid NULL)
- session_id (character varying(255) NULL)
- action (character varying(255) NOT NULL)
- entity_type (character varying(100) NULL)
- entity_id (character varying(255) NULL)
- old_values (jsonb NULL)
- new_values (jsonb NULL)
- metadata (jsonb NULL)
- ip_address (inet NULL)
- user_agent (text NULL)
- request_id (character varying(255) NULL)
- result (character varying(100) NULL)
- error_message (text NULL)
- risk_level (character varying(50) NULL)
- compliance_flags (jsonb NULL)
- created_at (timestamp with time zone NULL)

## data_type_transforms
- **PK**: id
- id (integer(32) NOT NULL)
- transform_name (character varying NOT NULL)
- hl7_data_type (character varying NOT NULL)
- fhir_data_type (character varying NOT NULL)
- transformation_function (text NULL)
- validation_rules (json NULL)
- examples (json NULL)
- source (character varying NULL)
- created_at (timestamp without time zone NULL)

## fhir_analytics_data
- **PK**: id
- id (uuid NOT NULL)
- date_hour (timestamp without time zone NOT NULL)
- interface_id (character varying NULL)
- message_type (character varying NOT NULL)
- source_system (character varying NULL)
- total_transformations (integer(32) NULL)
- successful_transformations (integer(32) NULL)
- failed_transformations (integer(32) NULL)
- avg_processing_time_ms (numeric NULL)
- total_resources_created (integer(32) NULL)
- resource_type_counts (jsonb NULL)
- error_summary (jsonb NULL)
- created_at (timestamp without time zone NULL)

## fhir_transformation_logs
- **PK**: id
- **FK**: user_id → users.id
- id (uuid NOT NULL)
- request_id (character varying(255) NOT NULL)
- source_system (character varying(100) NULL)
- user_id (uuid NULL)
- hl7_message_type (character varying(50) NOT NULL)
- fhir_target_profile (character varying(255) NULL)
- rules_applied (integer(32) NULL)
- resources_created (integer(32) NULL)
- warnings_count (integer(32) NULL)
- errors_count (integer(32) NULL)
- processing_time_ms (integer(32) NULL)
- memory_usage_mb (numeric NULL)
- transformation_status (character varying(50) NOT NULL)
- output_bundle_id (character varying(255) NULL)
- input_hl7_message (text NULL)
- output_fhir_bundle (jsonb NULL)
- error_details (jsonb NULL)
- warnings (jsonb NULL)
- started_at (timestamp without time zone NOT NULL)
- completed_at (timestamp without time zone NULL)
- created_at (timestamp without time zone NULL)

## field_element_mappings
- **PK**: id
- id (integer(32) NOT NULL)
- segment_name (character varying NOT NULL)
- hl7_field (character varying NOT NULL)
- hl7_component (character varying NULL)
- hl7_subcomponent (character varying NULL)
- fhir_resource_type (character varying NOT NULL)
- fhir_element_path (text NOT NULL)
- data_type_transform (character varying NULL)
- value_set_mapping_id (integer(32) NULL)
- mapping_conditions (json NULL)
- transformation_rules (json NULL)
- is_required (boolean NULL)
- cardinality (character varying NULL)
- source (character varying NULL)
- created_at (timestamp without time zone NULL)

## flyway_schema_history
- **PK**: installed_rank
- installed_rank (integer(32) NOT NULL)
- version (character varying(50) NULL)
- description (character varying(200) NOT NULL)
- type (character varying(20) NOT NULL)
- script (character varying(1000) NOT NULL)
- checksum (integer(32) NULL)
- installed_by (character varying(100) NOT NULL)
- installed_on (timestamp without time zone NOT NULL)
- execution_time (integer(32) NOT NULL)
- success (boolean NOT NULL)

## hl7_fhir_mappings
- **PK**: id
- **FK**: created_by → users.id
- id (integer(32) NOT NULL)
- hl7_version (character varying(50) NOT NULL)
- hl7_message_type (character varying(50) NOT NULL)
- hl7_segment (character varying(10) NOT NULL)
- hl7_field (character varying(50) NOT NULL)
- hl7_component (character varying(50) NULL)
- fhir_resource (character varying(100) NOT NULL)
- fhir_profile (character varying(255) NOT NULL)
- fhir_path (character varying(255) NOT NULL)
- transformation_rule (jsonb NOT NULL)
- condition_expression (text NULL)
- is_required (boolean NULL)
- priority (integer(32) NULL)
- created_at (timestamp without time zone NULL)
- updated_at (timestamp without time zone NULL)
- created_by (uuid NULL)
- is_active (boolean NULL)

## hl7_fhir_value_mappings
- hl7_table (character varying NULL)
- hl7_value (character varying NULL)
- fhir_system (character varying NULL)
- fhir_code (character varying NULL)
- fhir_display (character varying NULL)

## interface_resource_overrides
- **PK**: id
- id (integer(32) NOT NULL)
- interface_id (character varying NOT NULL)
- message_type (character varying NOT NULL)
- fhir_resources (json NOT NULL)
- custom_conditions (json NULL)
- created_by (character varying NULL)
- created_at (timestamp without time zone NULL)

## interfaces
- **PK**: id
- **FK**: created_by → users.id
- **FK**: updated_by → users.id
- **FK**: user_id → users.id
- id (uuid NOT NULL)
- user_id (uuid NOT NULL)
- name (character varying(255) NOT NULL)
- description (text NULL)
- source_type (character varying(100) NOT NULL)
- target_type (character varying(100) NOT NULL)
- message_type (character varying(100) NULL)
- source_config (jsonb NULL)
- target_config (jsonb NULL)
- processing_rules (jsonb NULL)
- transformation_mapping (jsonb NULL)
- status (character varying(50) NULL)
- total_processed (integer(32) NULL)
- successful_processed (integer(32) NULL)
- failed_processed (integer(32) NULL)
- last_processed_at (timestamp with time zone NULL)
- created_at (timestamp with time zone NULL)
- updated_at (timestamp with time zone NULL)
- created_by (uuid NULL)
- updated_by (uuid NULL)
- version (integer(32) NULL)
- is_active (boolean NULL)
- source_connectivity (character varying(50) NOT NULL)
- target_connectivity (character varying(50) NOT NULL)

## message_fhir_templates
- **PK**: id
- id (integer(32) NOT NULL)
- message_type (character varying NOT NULL)
- fhir_resources (jsonb NOT NULL)
- template_version (character varying NULL)
- description (text NULL)
- created_at (timestamp without time zone NULL)
- updated_at (timestamp without time zone NULL)
- is_active (boolean NULL)

## resource_lineage
- **PK**: id
- **FK**: created_by → users.id
- id (uuid NOT NULL)
- source_message_id (character varying NOT NULL)
- source_system (character varying NULL)
- hl7_message_type (character varying NOT NULL)
- fhir_resource_type (character varying NOT NULL)
- fhir_resource_id (character varying NOT NULL)
- fhir_profile (character varying NULL)
- transformation_request_id (character varying NOT NULL)
- applied_rules (jsonb NULL)
- field_mappings (jsonb NULL)
- bundle_id (character varying NULL)
- resource_order (integer(32) NULL)
- parent_resource_id (character varying NULL)
- child_resource_ids (jsonb NULL)
- created_by (uuid NULL)
- created_at (timestamp without time zone NULL)

## resource_segment_mappings
- **PK**: id
- id (integer(32) NOT NULL)
- segment_name (character varying NOT NULL)
- fhir_resource_type (character varying NOT NULL)
- mapping_conditions (json NULL)
- incorporation_rules (json NULL)
- priority (integer(32) NULL)
- multiple_instances (boolean NULL)
- source (character varying NULL)
- created_at (timestamp without time zone NULL)

## users
- **PK**: id
- id (uuid NOT NULL)
- email (character varying(320) NOT NULL)
- password_hash (character varying(255) NOT NULL)
- first_name (character varying(100) NOT NULL)
- last_name (character varying(100) NOT NULL)
- role (character varying(50) NOT NULL)
- status (character varying(50) NOT NULL)
- email_verified (boolean NOT NULL)
- email_verification_token (character varying(255) NULL)
- password_reset_token (character varying(255) NULL)
- password_reset_expires (timestamp with time zone NULL)
- last_login_at (timestamp with time zone NULL)
- last_login_ip (inet NULL)
- login_attempts (integer(32) NOT NULL)
- locked_until (timestamp with time zone NULL)
- data_consent_given (boolean NOT NULL)
- data_consent_date (timestamp with time zone NULL)
- data_retention_until (timestamp with time zone NULL)
- data_anonymized (boolean NOT NULL)
- gdpr_delete_requested (boolean NOT NULL)
- gdpr_delete_requested_at (timestamp with time zone NULL)
- phone (character varying(255) NULL)
- organization (character varying(255) NULL)
- job_title (character varying(255) NULL)
- department (character varying(255) NULL)
- timezone (character varying(100) NOT NULL)
- locale (character varying(10) NOT NULL)
- preferences (jsonb NULL)
- created_by (uuid NULL)
- updated_by (uuid NULL)
- created_at (timestamp with time zone NOT NULL)
- updated_at (timestamp with time zone NOT NULL)

## value_set_mappings
- **PK**: id
- id (integer(32) NOT NULL)
- mapping_name (character varying(255) NOT NULL)
- hl7_table (character varying(50) NULL)
- hl7_value (character varying(255) NOT NULL)
- fhir_system (character varying(255) NOT NULL)
- fhir_code (character varying(100) NOT NULL)
- fhir_display (text NULL)
- mapping_type (character varying(50) NULL)
- context_conditions (json NULL)
- source (character varying(100) NULL)
- created_at (timestamp without time zone NULL)
