# Schema Summary (Generated 2025-08-19T16:06:34.067103)

## How to use this document
- Purpose: Quick reference for database entities used by HL7→FHIR transformation, UI, and operations.
- Navigation: Each section is a table. Use bullets to grasp intent fast, then consult PK/FK and columns.
- Change impact: Updating mapping-related tables (e.g., `hl7_fhir_mappings`, `value_set_mappings`) affects transformation results; audit/lineage tables impact compliance and traceability.

## audit_logs
- Purpose: System-wide audit trail of user and system actions with contextual metadata.
- Logical role: Compliance and forensics store; links requests, sessions, and results.
- Business value: Meets regulatory requirements and supports incident investigation.
- **PK**: id
- id (uuid NOT NULL)
- user_id (uuid NULL)
- session_id (character varying NULL)
- action (character varying NOT NULL)
- entity_type (character varying NULL)
- entity_id (character varying NULL)
- old_values (jsonb NULL)
- new_values (jsonb NULL)
- metadata (jsonb NULL)
- ip_address (inet NULL)
- user_agent (text NULL)
- request_id (character varying NULL)
- result (character varying NULL)
- error_message (text NULL)
- risk_level (character varying NULL)
- compliance_flags (jsonb NULL)
- created_at (timestamp with time zone NULL)

## data_type_transforms
- Purpose: Maps HL7 datatypes to FHIR datatypes with transformation/validation guidance.
- Logical role: Reference for datatype conversion during transformation.
- Business value: Ensures consistent, standards-aligned datatype handling.
- **PK**: id
- id (integer NOT NULL)
- transform_name (character varying NOT NULL)
- hl7_data_type (character varying NOT NULL)
- fhir_data_type (character varying NOT NULL)
- transformation_function (text NULL)
- validation_rules (json NULL)
- examples (json NULL)
- source (character varying NULL)
- created_at (timestamp without time zone NULL)

## fhir_analytics_data
- Purpose: Aggregated analytics for transformation throughput, success rate, and resource counts.
- Logical role: Reporting dataset for ops/quality dashboards.
- Business value: Visibility into performance and bottlenecks.
- **PK**: id
- id (uuid NOT NULL)
- report_date (date NOT NULL)
- hour_of_day (integer NULL)
- message_type (character varying NOT NULL)
- total_transformations (integer NULL)
- successful_transformations (integer NULL)
- failed_transformations (integer NULL)
- avg_processing_time_ms (numeric NULL)
- max_processing_time_ms (integer NULL)
- min_processing_time_ms (integer NULL)
- total_patients_created (integer NULL)
- total_encounters_created (integer NULL)
- total_observations_created (integer NULL)
- total_bundles_created (integer NULL)
- avg_warnings_per_message (numeric NULL)
- most_common_warning (character varying NULL)
- data_completeness_score (numeric NULL)
- created_at (timestamp without time zone NULL)

## fhir_custom_value_sets
- Purpose: Custom value set mappings maintained by users/ops to override defaults.
- Logical role: Source of truth for environment-specific code mappings.
- Business value: Rapid adaptation to partner/vendor coding systems.
- **PK**: id
- **FK**: created_by → users.id
- id (uuid NOT NULL)
- value_set_name (character varying NOT NULL)
- value_set_url (character varying NULL)
- value_set_version (character varying NULL)
- hl7_code (character varying NOT NULL)
- hl7_display (character varying NULL)
- hl7_code_system (character varying NULL)
- fhir_code (character varying NOT NULL)
- fhir_display (character varying NULL)
- fhir_code_system (character varying NOT NULL)
- mapping_confidence (numeric NULL)
- mapping_note (text NULL)
- is_default_mapping (boolean NULL)
- created_at (timestamp without time zone NULL)
- updated_at (timestamp without time zone NULL)
- created_by (uuid NULL)
- is_active (boolean NULL)

## fhir_profile_registry
- Purpose: Registry of FHIR profiles (base/us-core/custom) including required/mustSupport elements.
- Logical role: Authoritative store for profile-driven validation and generation.
- Business value: Enforces compliance with implementation guides.
- **PK**: id
- **FK**: created_by → users.id
- id (uuid NOT NULL)
- profile_name (character varying NOT NULL)
- profile_version (character varying NOT NULL)
- profile_url (character varying NOT NULL)
- base_resource (character varying NOT NULL)
- profile_definition (jsonb NOT NULL)
- required_elements (jsonb NULL)
- must_support_elements (jsonb NULL)
- extensions (jsonb NULL)
- validation_rules (jsonb NULL)
- is_active (boolean NULL)
- created_at (timestamp without time zone NULL)
- updated_at (timestamp without time zone NULL)
- created_by (uuid NULL)

## fhir_resource_lineage
- Purpose: Tracks provenance from HL7 message to produced FHIR resources and bundle.
- Logical role: Lineage for traceability and debugging.
- Business value: Supports audits and root-cause analysis.
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
- resource_order (integer NULL)
- parent_resource_id (character varying NULL)
- child_resource_ids (jsonb NULL)
- created_by (uuid NULL)
- created_at (timestamp without time zone NULL)

## fhir_transformation_logs
- Purpose: Per-request logs of transformation execution including warnings/errors and metrics.
- Logical role: Operational log store for troubleshooting.
- Business value: Faster incident resolution and SLA reporting.
- **PK**: id
- **FK**: user_id → users.id
- id (uuid NOT NULL)
- request_id (character varying NOT NULL)
- source_system (character varying NULL)
- user_id (uuid NULL)
- hl7_message_type (character varying NOT NULL)
- fhir_target_profile (character varying NULL)
- rules_applied (integer NULL)
- resources_created (integer NULL)
- warnings_count (integer NULL)
- errors_count (integer NULL)
- processing_time_ms (integer NULL)
- memory_usage_mb (numeric NULL)
- transformation_status (character varying NOT NULL)
- output_bundle_id (character varying NULL)
- input_hl7_message (text NULL)
- output_fhir_bundle (jsonb NULL)
- error_details (jsonb NULL)
- warnings (jsonb NULL)
- started_at (timestamp without time zone NOT NULL)
- completed_at (timestamp without time zone NULL)
- created_at (timestamp without time zone NULL)

## field_element_mappings
- Purpose: Atomic field/component-to-FHIR element mappings with conditions and transforms.
- Logical role: Fine-grained rule source for transformers.
- Business value: Precise control over mapping behavior.
- **PK**: id
- id (integer NOT NULL)
- segment_name (character varying NOT NULL)
- hl7_field (character varying NOT NULL)
- hl7_component (character varying NULL)
- hl7_subcomponent (character varying NULL)
- fhir_resource_type (character varying NOT NULL)
- fhir_element_path (text NOT NULL)
- data_type_transform (character varying NULL)
- value_set_mapping_id (integer NULL)
- mapping_conditions (json NULL)
- transformation_rules (json NULL)
- is_required (boolean NULL)
- cardinality (character varying NULL)
- source (character varying NULL)
- created_at (timestamp without time zone NULL)

## hl7_fhir_mappings
- Purpose: High-level HL7→FHIR mapping rules (segment/field/component to FHIR path) with conditions.
- Logical role: Core mapping table used by transformation engines.
- Business value: Centralizes mapping logic for maintainability.
- **PK**: id
- **FK**: created_by → users.id
- id (integer NOT NULL)
- hl7_version (character varying NOT NULL)
- hl7_message_type (character varying NOT NULL)
- hl7_segment (character varying NOT NULL)
- hl7_field (character varying NOT NULL)
- hl7_component (character varying NULL)
- fhir_resource (character varying NOT NULL)
- fhir_profile (character varying NOT NULL)
- fhir_path (character varying NOT NULL)
- transformation_rule (jsonb NOT NULL)
- condition_expression (text NULL)
- is_required (boolean NULL)
- priority (integer NULL)
- created_at (timestamp without time zone NULL)
- updated_at (timestamp without time zone NULL)
- created_by (uuid NULL)
- is_active (boolean NULL)

## hl7_fhir_value_mappings
- Purpose: Value-level mapping (HL7 table/value → FHIR system/code/display).
- Logical role: Reference for code translation during transformation.
- Business value: Harmonizes codes across systems for interoperability.
- hl7_table (character varying NULL)
- hl7_value (character varying NULL)
- fhir_system (character varying NULL)
- fhir_code (character varying NULL)
- fhir_display (character varying NULL)

## interface_resource_overrides
- Purpose: Interface-specific overrides controlling which FHIR resources to emit per message type.
- Logical role: Policy layer per integration.
- Business value: Tailors outputs to partner needs without code changes.
- **PK**: id
- id (integer NOT NULL)
- interface_id (character varying NOT NULL)
- message_type (character varying NOT NULL)
- fhir_resources (json NOT NULL)
- custom_conditions (json NULL)
- created_by (character varying NULL)
- created_at (timestamp without time zone NULL)

## interfaces
- Purpose: Integration interface definitions and runtime stats.
- Logical role: Primary configuration entity for message pipelines.
- Business value: Operational control and governance.
- **PK**: id
- **FK**: created_by → users.id
- **FK**: updated_by → users.id
- **FK**: user_id → users.id
- id (uuid NOT NULL)
- user_id (uuid NOT NULL)
- name (character varying NOT NULL)
- description (text NULL)
- source_type (character varying NOT NULL)
- target_type (character varying NOT NULL)
- message_type (character varying NULL)
- source_config (jsonb NULL)
- target_config (jsonb NULL)
- processing_rules (jsonb NULL)
- transformation_mapping (jsonb NULL)
- status (character varying NULL)
- total_processed (integer NULL)
- successful_processed (integer NULL)
- failed_processed (integer NULL)
- last_processed_at (timestamp with time zone NULL)
- created_at (timestamp with time zone NULL)
- updated_at (timestamp with time zone NULL)
- created_by (uuid NULL)
- updated_by (uuid NULL)
- version (integer NULL)
- is_active (boolean NULL)

## message_fhir_templates
- Purpose: Default resource templates per message/event type with priorities and conditions.
- Logical role: Starting point for resource selection before overrides.
- Business value: Accelerates onboarding with sensible defaults.
- **PK**: id
- id (integer NOT NULL)
- message_type (character varying NOT NULL)
- event_type (character varying NULL)
- fhir_resources (json NOT NULL)
- resource_conditions (json NOT NULL)
- resource_priorities (json NOT NULL)
- source (character varying NULL)
- is_default (boolean NULL)
- created_at (timestamp without time zone NULL)
- updated_at (timestamp without time zone NULL)

## segment_resource_mappings
- Purpose: Maps HL7 segments to target FHIR resource types with incorporation rules.
- Logical role: Mid-level mapping that guides resource inclusion.
- Business value: Simplifies resource selection logic.
- **PK**: id
- id (integer NOT NULL)
- segment_name (character varying NOT NULL)
- fhir_resource_type (character varying NOT NULL)
- mapping_conditions (json NULL)
- incorporation_rules (json NULL)
- priority (integer NULL)
- multiple_instances (boolean NULL)
- source (character varying NULL)
- created_at (timestamp without time zone NULL)

## users
- Purpose: Application user accounts, roles, and preferences.
- Logical role: Identity and access control base.
- Business value: Security, auditing, and personalization.
- **PK**: id
- id (uuid NOT NULL)
- email (character varying NOT NULL)
- password_hash (character varying NOT NULL)
- first_name (character varying NOT NULL)
- last_name (character varying NOT NULL)
- role (character varying NOT NULL)
- status (character varying NOT NULL)
- email_verified (boolean NOT NULL)
- email_verification_token (character varying NULL)
- password_reset_token (character varying NULL)
- password_reset_expires (timestamp with time zone NULL)
- last_login_at (timestamp with time zone NULL)
- last_login_ip (inet NULL)
- login_attempts (integer NOT NULL)
- locked_until (timestamp with time zone NULL)
- data_consent_given (boolean NOT NULL)
- data_consent_date (timestamp with time zone NULL)
- data_retention_until (timestamp with time zone NULL)
- data_anonymized (boolean NOT NULL)
- gdpr_delete_requested (boolean NOT NULL)
- gdpr_delete_requested_at (timestamp with time zone NULL)
- phone (character varying NULL)
- organization (character varying NULL)
- job_title (character varying NULL)
- department (character varying NULL)
- timezone (character varying NOT NULL)
- locale (character varying NOT NULL)
- preferences (jsonb NULL)
- created_by (uuid NULL)
- updated_by (uuid NULL)
- created_at (timestamp with time zone NOT NULL)
- updated_at (timestamp with time zone NOT NULL)

## value_set_mappings
- Purpose: Configurable table for mapping codes/values between HL7 and FHIR systems.
- Logical role: Extensible vocabulary translation store.
- Business value: Enables quick adaptation to customer vocabularies.
- **PK**: id
- id (integer NOT NULL)
- mapping_name (character varying NOT NULL)
- hl7_table (character varying NULL)
- hl7_value (character varying NOT NULL)
- fhir_system (character varying NOT NULL)
- fhir_code (character varying NOT NULL)
- fhir_display (text NULL)
- mapping_type (character varying NULL)
- context_conditions (json NULL)
- source (character varying NULL)
- created_at (timestamp without time zone NULL)
