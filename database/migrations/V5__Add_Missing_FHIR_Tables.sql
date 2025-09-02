-- V5__Add_Missing_FHIR_Tables.sql
-- Flyway migration to add missing FHIR transformation tables

-- Create field_element_mappings table
CREATE TABLE IF NOT EXISTS field_element_mappings (
    id SERIAL PRIMARY KEY,
    segment_name VARCHAR NOT NULL,
    hl7_field VARCHAR NOT NULL,
    hl7_component VARCHAR,
    hl7_subcomponent VARCHAR,
    fhir_resource_type VARCHAR NOT NULL,
    fhir_element_path TEXT NOT NULL,
    data_type_transform VARCHAR,
    value_set_mapping_id INTEGER,
    mapping_conditions JSON,
    transformation_rules JSON,
    is_required BOOLEAN,
    cardinality VARCHAR,
    source VARCHAR,
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
);

-- Create hl7_fhir_mappings table
CREATE TABLE IF NOT EXISTS hl7_fhir_mappings (
    id SERIAL PRIMARY KEY,
    hl7_version VARCHAR NOT NULL,
    hl7_message_type VARCHAR NOT NULL,
    hl7_segment VARCHAR NOT NULL,
    hl7_field VARCHAR NOT NULL,
    hl7_component VARCHAR,
    fhir_resource VARCHAR NOT NULL,
    fhir_profile VARCHAR NOT NULL,
    fhir_path VARCHAR NOT NULL,
    transformation_rule JSONB NOT NULL,
    condition_expression TEXT,
    is_required BOOLEAN,
    priority INTEGER,
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
    created_by UUID REFERENCES users(id),
    is_active BOOLEAN DEFAULT TRUE
);

-- Create value_set_mappings table
CREATE TABLE IF NOT EXISTS value_set_mappings (
    id SERIAL PRIMARY KEY,
    mapping_name VARCHAR NOT NULL,
    hl7_table VARCHAR,
    hl7_value VARCHAR NOT NULL,
    fhir_system VARCHAR NOT NULL,
    fhir_code VARCHAR NOT NULL,
    fhir_display TEXT,
    mapping_type VARCHAR,
    context_conditions JSON,
    source VARCHAR,
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
);

-- Create message_fhir_templates table
CREATE TABLE IF NOT EXISTS message_fhir_templates (
    id SERIAL PRIMARY KEY,
    message_type VARCHAR NOT NULL,
    fhir_resources JSONB NOT NULL,
    template_version VARCHAR DEFAULT '1.0',
    description TEXT,
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
    is_active BOOLEAN DEFAULT TRUE
);

-- Create data_type_transforms table
CREATE TABLE IF NOT EXISTS data_type_transforms (
    id SERIAL PRIMARY KEY,
    transform_name VARCHAR NOT NULL,
    hl7_data_type VARCHAR NOT NULL,
    fhir_data_type VARCHAR NOT NULL,
    transformation_function TEXT,
    validation_rules JSON,
    examples JSON,
    source VARCHAR,
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
);

-- Create resource_segment_mappings table
CREATE TABLE IF NOT EXISTS resource_segment_mappings (
    id SERIAL PRIMARY KEY,
    segment_name VARCHAR NOT NULL,
    fhir_resource_type VARCHAR NOT NULL,
    mapping_conditions JSON,
    incorporation_rules JSON,
    priority INTEGER,
    multiple_instances BOOLEAN DEFAULT FALSE,
    source VARCHAR,
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
);

-- Create fhir_transformation_logs table
CREATE TABLE IF NOT EXISTS fhir_transformation_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id VARCHAR NOT NULL,
    source_system VARCHAR,
    user_id UUID REFERENCES users(id),
    hl7_message_type VARCHAR NOT NULL,
    fhir_target_profile VARCHAR,
    rules_applied INTEGER,
    resources_created INTEGER,
    warnings_count INTEGER,
    errors_count INTEGER,
    processing_time_ms INTEGER,
    memory_usage_mb NUMERIC,
    transformation_status VARCHAR NOT NULL,
    output_bundle_id VARCHAR,
    input_hl7_message TEXT,
    output_fhir_bundle JSONB,
    error_details JSONB,
    warnings JSONB,
    started_at TIMESTAMP WITHOUT TIME ZONE NOT NULL,
    completed_at TIMESTAMP WITHOUT TIME ZONE,
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
);

-- Create resource_lineage table
CREATE TABLE IF NOT EXISTS resource_lineage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_message_id VARCHAR NOT NULL,
    source_system VARCHAR,
    hl7_message_type VARCHAR NOT NULL,
    fhir_resource_type VARCHAR NOT NULL,
    fhir_resource_id VARCHAR NOT NULL,
    fhir_profile VARCHAR,
    transformation_request_id VARCHAR NOT NULL,
    applied_rules JSONB,
    field_mappings JSONB,
    bundle_id VARCHAR,
    resource_order INTEGER,
    parent_resource_id VARCHAR,
    child_resource_ids JSONB,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
);

-- Create interface_resource_overrides table
CREATE TABLE IF NOT EXISTS interface_resource_overrides (
    id SERIAL PRIMARY KEY,
    interface_id VARCHAR NOT NULL,
    message_type VARCHAR NOT NULL,
    fhir_resources JSON NOT NULL,
    custom_conditions JSON,
    created_by VARCHAR,
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
);

-- Create fhir_analytics_data table
CREATE TABLE IF NOT EXISTS fhir_analytics_data (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    date_hour TIMESTAMP WITHOUT TIME ZONE NOT NULL,
    interface_id VARCHAR,
    message_type VARCHAR NOT NULL,
    source_system VARCHAR,
    total_transformations INTEGER DEFAULT 0,
    successful_transformations INTEGER DEFAULT 0,
    failed_transformations INTEGER DEFAULT 0,
    avg_processing_time_ms NUMERIC,
    total_resources_created INTEGER DEFAULT 0,
    resource_type_counts JSONB,
    error_summary JSONB,
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
);

-- Create hl7_fhir_value_mappings table
CREATE TABLE IF NOT EXISTS hl7_fhir_value_mappings (
    hl7_table VARCHAR,
    hl7_value VARCHAR,
    fhir_system VARCHAR,
    fhir_code VARCHAR,
    fhir_display VARCHAR
);

-- Insert basic message templates
INSERT INTO message_fhir_templates (message_type, fhir_resources, description) VALUES
('ADT^A01', '{"Patient": true, "Encounter": true}', 'Patient admission template'),
('ADT^A04', '{"Patient": true, "Encounter": true}', 'Patient registration template'),
('ADT^A08', '{"Patient": true, "Encounter": true}', 'Patient update template'),
('ORU^R01', '{"Patient": true, "Observation": true, "DiagnosticReport": true}', 'Observation result template'),
('ORM^O01', '{"Patient": true, "ServiceRequest": true}', 'Order message template'),
('SIU^S12', '{"Patient": true, "Appointment": true}', 'Appointment booking template')
ON CONFLICT DO NOTHING;

-- Insert basic field mappings for common HL7 to FHIR mappings
INSERT INTO field_element_mappings (segment_name, hl7_field, fhir_resource_type, fhir_element_path, is_required) VALUES
('MSH', 'MSH.3', 'MessageHeader', 'source.name', false),
('MSH', 'MSH.4', 'MessageHeader', 'source.endpoint', false),
('MSH', 'MSH.9', 'MessageHeader', 'eventCoding', true),
('PID', 'PID.3', 'Patient', 'identifier', true),
('PID', 'PID.5', 'Patient', 'name', true),
('PID', 'PID.7', 'Patient', 'birthDate', false),
('PID', 'PID.8', 'Patient', 'gender', false),
('PID', 'PID.11', 'Patient', 'address', false),
('PID', 'PID.13', 'Patient', 'telecom', false),
('PV1', 'PV1.2', 'Encounter', 'class', false),
('PV1', 'PV1.19', 'Encounter', 'identifier', false),
('PV1', 'PV1.44', 'Encounter', 'period.start', false),
('PV1', 'PV1.45', 'Encounter', 'period.end', false),
('OBX', 'OBX.3', 'Observation', 'code', true),
('OBX', 'OBX.5', 'Observation', 'valueQuantity', false),
('OBX', 'OBX.6', 'Observation', 'valueQuantity.unit', false),
('OBX', 'OBX.11', 'Observation', 'status', false)
ON CONFLICT DO NOTHING;

-- Insert basic value set mappings
INSERT INTO value_set_mappings (mapping_name, hl7_table, hl7_value, fhir_system, fhir_code, fhir_display) VALUES
('gender', '0001', 'M', 'http://hl7.org/fhir/administrative-gender', 'male', 'Male'),
('gender', '0001', 'F', 'http://hl7.org/fhir/administrative-gender', 'female', 'Female'),
('gender', '0001', 'U', 'http://hl7.org/fhir/administrative-gender', 'unknown', 'Unknown'),
('gender', '0001', 'O', 'http://hl7.org/fhir/administrative-gender', 'other', 'Other'),
('patient_class', '0004', 'I', 'http://terminology.hl7.org/CodeSystem/v3-ActCode', 'IMP', 'Inpatient'),
('patient_class', '0004', 'O', 'http://terminology.hl7.org/CodeSystem/v3-ActCode', 'AMB', 'Ambulatory'),
('patient_class', '0004', 'E', 'http://terminology.hl7.org/CodeSystem/v3-ActCode', 'EMER', 'Emergency'),
('patient_class', '0004', 'P', 'http://terminology.hl7.org/CodeSystem/v3-ActCode', 'PRENC', 'Pre-admission'),
('observation_status', '0085', 'F', 'http://hl7.org/fhir/observation-status', 'final', 'Final'),
('observation_status', '0085', 'P', 'http://hl7.org/fhir/observation-status', 'preliminary', 'Preliminary'),
('observation_status', '0085', 'C', 'http://hl7.org/fhir/observation-status', 'corrected', 'Corrected'),
('observation_status', '0085', 'X', 'http://hl7.org/fhir/observation-status', 'cancelled', 'Cancelled')
ON CONFLICT DO NOTHING;

-- Insert basic data type transforms
INSERT INTO data_type_transforms (transform_name, hl7_data_type, fhir_data_type, transformation_function) VALUES
('string_to_string', 'ST', 'string', 'direct'),
('numeric_to_decimal', 'NM', 'decimal', 'parseDecimal'),
('date_to_date', 'DT', 'date', 'parseHL7Date'),
('datetime_to_datetime', 'TS', 'dateTime', 'parseHL7DateTime'),
('coded_element', 'CE', 'CodeableConcept', 'mapCodeableConcept'),
('identifier', 'CX', 'Identifier', 'mapIdentifier'),
('name', 'XPN', 'HumanName', 'mapHumanName'),
('address', 'XAD', 'Address', 'mapAddress'),
('telecom', 'XTN', 'ContactPoint', 'mapContactPoint')
ON CONFLICT DO NOTHING;

-- Insert resource segment mappings
INSERT INTO resource_segment_mappings (segment_name, fhir_resource_type, priority, multiple_instances) VALUES
('MSH', 'MessageHeader', 1, false),
('PID', 'Patient', 2, false),
('PV1', 'Encounter', 3, false),
('OBX', 'Observation', 4, true),
('ORC', 'ServiceRequest', 5, true),
('OBR', 'DiagnosticReport', 6, true),
('SCH', 'Appointment', 7, false)
ON CONFLICT DO NOTHING;

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_field_element_mappings_segment ON field_element_mappings(segment_name);
CREATE INDEX IF NOT EXISTS idx_field_element_mappings_resource ON field_element_mappings(fhir_resource_type);
CREATE INDEX IF NOT EXISTS idx_field_element_mappings_field ON field_element_mappings(hl7_field);
CREATE INDEX IF NOT EXISTS idx_hl7_fhir_mappings_message_type ON hl7_fhir_mappings(hl7_message_type);
CREATE INDEX IF NOT EXISTS idx_hl7_fhir_mappings_segment ON hl7_fhir_mappings(hl7_segment);
CREATE INDEX IF NOT EXISTS idx_value_set_mappings_table ON value_set_mappings(hl7_table);
CREATE INDEX IF NOT EXISTS idx_value_set_mappings_value ON value_set_mappings(hl7_value);
CREATE INDEX IF NOT EXISTS idx_message_templates_type ON message_fhir_templates(message_type);
CREATE INDEX IF NOT EXISTS idx_transformation_logs_request ON fhir_transformation_logs(request_id);
CREATE INDEX IF NOT EXISTS idx_transformation_logs_user ON fhir_transformation_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_resource_lineage_message ON resource_lineage(source_message_id);
CREATE INDEX IF NOT EXISTS idx_analytics_data_hour ON fhir_analytics_data(date_hour);
CREATE INDEX IF NOT EXISTS idx_analytics_data_interface ON fhir_analytics_data(interface_id);