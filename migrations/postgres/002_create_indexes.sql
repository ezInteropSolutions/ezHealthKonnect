-- migrations/postgres/002_create_indexes.sql
-- Optimized indexes for FHIR schema queries

-- Resource lookup indexes
CREATE INDEX CONCURRENTLY idx_fhir_resources_type_version 
ON fhir_resources(resource_type, version_id);

CREATE INDEX CONCURRENTLY idx_fhir_resources_base 
ON fhir_resources(base_resource) WHERE base_resource IS NOT NULL;

-- Profile lookup indexes  
CREATE INDEX CONCURRENTLY idx_fhir_profiles_name_active 
ON fhir_profiles(profile_name) WHERE is_active = true;

CREATE INDEX CONCURRENTLY idx_fhir_profiles_base_resource 
ON fhir_profiles(base_resource_id);

-- Element query optimization indexes
CREATE INDEX CONCURRENTLY idx_fhir_elements_resource_profile 
ON fhir_elements(resource_id, profile_id);

CREATE INDEX CONCURRENTLY idx_fhir_elements_path_lookup 
ON fhir_elements(path) INCLUDE (name, data_type, is_required);

CREATE INDEX CONCURRENTLY idx_fhir_elements_required_only 
ON fhir_elements(resource_id, profile_id) WHERE is_required = true;

CREATE INDEX CONCURRENTLY idx_fhir_elements_must_support 
ON fhir_elements(resource_id, profile_id) WHERE is_must_support = true;

CREATE INDEX CONCURRENTLY idx_fhir_elements_data_type 
ON fhir_elements(data_type);

-- Value set binding indexes
CREATE INDEX CONCURRENTLY idx_fhir_value_sets_url 
ON fhir_value_sets(url);

CREATE INDEX CONCURRENTLY idx_fhir_value_sets_active 
ON fhir_value_sets(name) WHERE status = 'active';

CREATE INDEX CONCURRENTLY idx_fhir_element_bindings_strength 
ON fhir_element_bindings(element_id, strength);

-- Constraint lookup indexes
CREATE INDEX CONCURRENTLY idx_fhir_constraints_severity 
ON fhir_constraints(element_id, severity);

CREATE INDEX CONCURRENTLY idx_fhir_constraints_key 
ON fhir_constraints(constraint_key);

-- HL7→FHIR mapping indexes for fast transformation
CREATE INDEX CONCURRENTLY idx_hl7_mappings_lookup 
ON hl7_fhir_mappings(hl7_message_type, hl7_segment, hl7_field);

CREATE INDEX CONCURRENTLY idx_hl7_mappings_fhir_target 
ON hl7_fhir_mappings(fhir_resource, fhir_profile);

CREATE INDEX CONCURRENTLY idx_hl7_mappings_priority 
ON hl7_fhir_mappings(hl7_message_type, priority);

-- Composite indexes for common query patterns
CREATE INDEX CONCURRENTLY idx_fhir_elements_composite 
ON fhir_elements(resource_id, profile_id, is_required, is_must_support) 
INCLUDE (path, name, data_type);

-- JSON indexes for transformation rules
CREATE INDEX CONCURRENTLY idx_hl7_mappings_rule_gin 
ON hl7_fhir_mappings USING gin(transformation_rule);

-- Text search indexes
CREATE INDEX CONCURRENTLY idx_fhir_elements_name_text 
ON fhir_elements USING gin(to_tsvector('english', name || ' ' || short_description));

CREATE INDEX CONCURRENTLY idx_fhir_resources_description_text 
ON fhir_resources USING gin(to_tsvector('english', description));

-- Statistics for query planner
ANALYZE fhir_resources;
ANALYZE fhir_profiles;
ANALYZE fhir_elements;
ANALYZE fhir_value_sets;
ANALYZE fhir_element_bindings;
ANALYZE fhir_constraints;
ANALYZE hl7_fhir_mappings;