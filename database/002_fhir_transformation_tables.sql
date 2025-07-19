-- =====================================
-- FHIR TRANSFORMATION TABLES - Addition to Existing ezHealthKonnect Database
-- File: database/002_fhir_transformation_tables.sql
-- =====================================
-- This script adds FHIR transformation capabilities to your existing database
-- without affecting any existing tables (users, audit_logs, interfaces, etc.)

-- =====================================
-- CLEANUP SECTION - Drop only FHIR tables if they exist
-- =====================================
DROP TABLE IF EXISTS fhir_transformation_logs CASCADE;
DROP TABLE IF EXISTS fhir_analytics_data CASCADE;
DROP TABLE IF EXISTS fhir_resource_lineage CASCADE;
DROP TABLE IF EXISTS fhir_custom_value_sets CASCADE;
DROP TABLE IF EXISTS hl7_fhir_mappings CASCADE;
DROP TABLE IF EXISTS fhir_profile_registry CASCADE;

-- Drop FHIR-specific indexes
DROP INDEX IF EXISTS idx_fhir_mappings_message_type;
DROP INDEX IF EXISTS idx_fhir_mappings_segment;
DROP INDEX IF EXISTS idx_fhir_mappings_priority;
DROP INDEX IF EXISTS idx_fhir_logs_timestamp;
DROP INDEX IF EXISTS idx_fhir_analytics_date;
DROP INDEX IF EXISTS idx_fhir_lineage_source_id;

-- =====================================
-- ENSURE EXTENSIONS (for UUID support)
-- =====================================
-- uuid-ossp extension should already exist from your existing setup
-- but we'll make sure it's available for FHIR tables
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =====================================
-- 1. CORE HL7→FHIR MAPPING RULES TABLE
-- =====================================
CREATE TABLE hl7_fhir_mappings (
    id SERIAL PRIMARY KEY,
    
    -- HL7 Source Configuration
    hl7_version VARCHAR(10) NOT NULL DEFAULT '2.5.1',
    hl7_message_type VARCHAR(50) NOT NULL,          -- ADT^A01, ORU^R01, etc.
    hl7_segment VARCHAR(10) NOT NULL,               -- PID, PV1, MSH, etc.
    hl7_field VARCHAR(20) NOT NULL,                 -- Field number or path (e.g., "5", "5.1")
    hl7_component VARCHAR(10),                      -- Component within field (optional)
    
    -- FHIR Target Configuration
    fhir_resource VARCHAR(50) NOT NULL,             -- Patient, Encounter, Observation
    fhir_profile VARCHAR(100) NOT NULL DEFAULT 'base', -- base, us-core-patient, etc.
    fhir_path VARCHAR(200) NOT NULL,                -- Patient.name.given, Patient.identifier
    
    -- Transformation Logic
    transformation_rule JSONB NOT NULL,            -- Transformation configuration
    condition_expression TEXT,                     -- Conditional mapping logic
    
    -- Metadata
    is_required BOOLEAN DEFAULT false,
    priority INTEGER DEFAULT 100,                  -- Lower = higher priority
    
    -- Audit fields
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id),          -- Link to your existing users table
    is_active BOOLEAN DEFAULT true,
    
    -- Add constraints
    CONSTRAINT chk_fhir_priority CHECK (priority >= 1 AND priority <= 1000),
    CONSTRAINT chk_fhir_resource CHECK (fhir_resource ~ '^[A-Z][a-zA-Z]+$'),
    CONSTRAINT chk_hl7_field CHECK (hl7_field ~ '^[0-9]+(\.[0-9]+)*$')
);

-- =====================================
-- 2. FHIR PROFILE REGISTRY
-- =====================================
CREATE TABLE fhir_profile_registry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_name VARCHAR(100) NOT NULL UNIQUE,     -- us-core-patient, us-core-encounter
    profile_version VARCHAR(20) NOT NULL,          -- 6.1.0, 7.0.0
    profile_url VARCHAR(500) NOT NULL,             -- Canonical URL
    base_resource VARCHAR(50) NOT NULL,            -- Patient, Encounter
    
    -- Profile Definition
    profile_definition JSONB NOT NULL,             -- Full profile structure
    required_elements JSONB,                       -- Must-have elements
    must_support_elements JSONB,                   -- Must-support elements
    extensions JSONB,                              -- Supported extensions
    validation_rules JSONB,                        -- Profile-specific validation
    
    -- Metadata
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id)           -- Link to your existing users table
);

-- =====================================
-- 3. FHIR TRANSFORMATION AUDIT LOGS
-- =====================================
CREATE TABLE fhir_transformation_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Request Information
    request_id VARCHAR(100) NOT NULL,
    source_system VARCHAR(100),
    user_id UUID REFERENCES users(id),             -- Link to your existing users table
    
    -- Message Details
    hl7_message_type VARCHAR(50) NOT NULL,
    fhir_target_profile VARCHAR(100),
    
    -- Processing Details
    rules_applied INTEGER DEFAULT 0,
    resources_created INTEGER DEFAULT 0,
    warnings_count INTEGER DEFAULT 0,
    errors_count INTEGER DEFAULT 0,
    
    -- Performance Metrics
    processing_time_ms INTEGER,
    memory_usage_mb DECIMAL(10,2),
    
    -- Results
    transformation_status VARCHAR(20) NOT NULL,    -- success, warning, error
    output_bundle_id VARCHAR(100),
    
    -- Raw Data (for debugging)
    input_hl7_message TEXT,
    output_fhir_bundle JSONB,
    error_details JSONB,
    warnings JSONB,
    
    -- Timestamps
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT chk_fhir_status CHECK (transformation_status IN ('success', 'warning', 'error', 'processing'))
);

-- =====================================
-- 4. FHIR ANALYTICS AND REPORTING
-- =====================================
CREATE TABLE fhir_analytics_data (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Time Series Data
    report_date DATE NOT NULL,
    hour_of_day INTEGER CHECK (hour_of_day >= 0 AND hour_of_day <= 23),
    
    -- Aggregated Metrics
    message_type VARCHAR(50) NOT NULL,
    total_transformations INTEGER DEFAULT 0,
    successful_transformations INTEGER DEFAULT 0,
    failed_transformations INTEGER DEFAULT 0,
    
    -- Performance Metrics
    avg_processing_time_ms DECIMAL(10,2),
    max_processing_time_ms INTEGER,
    min_processing_time_ms INTEGER,
    
    -- Resource Creation Stats
    total_patients_created INTEGER DEFAULT 0,
    total_encounters_created INTEGER DEFAULT 0,
    total_observations_created INTEGER DEFAULT 0,
    total_bundles_created INTEGER DEFAULT 0,
    
    -- Data Quality Metrics
    avg_warnings_per_message DECIMAL(5,2),
    most_common_warning VARCHAR(200),
    data_completeness_score DECIMAL(5,2),
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    -- Ensure unique daily records per message type
    UNIQUE(report_date, hour_of_day, message_type)
);

-- =====================================
-- 5. FHIR RESOURCE LINEAGE TRACKING
-- =====================================
CREATE TABLE fhir_resource_lineage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Source Information
    source_message_id VARCHAR(100) NOT NULL,
    source_system VARCHAR(100),
    hl7_message_type VARCHAR(50) NOT NULL,
    
    -- FHIR Resource Details
    fhir_resource_type VARCHAR(50) NOT NULL,
    fhir_resource_id VARCHAR(100) NOT NULL,
    fhir_profile VARCHAR(100),
    
    -- Transformation Details
    transformation_request_id VARCHAR(100) NOT NULL,
    applied_rules JSONB,                            -- Rules that created this resource
    field_mappings JSONB,                           -- Detailed field-by-field mapping
    
    -- Bundle Information
    bundle_id VARCHAR(100),
    resource_order INTEGER,                         -- Order within bundle
    
    -- Relationships
    parent_resource_id VARCHAR(100),                -- For contained/referenced resources
    child_resource_ids JSONB,                       -- Array of child resource IDs
    
    -- Link to your existing users table
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- =====================================
-- 6. FHIR CUSTOM VALUE SETS AND TERMINOLOGY
-- =====================================
CREATE TABLE fhir_custom_value_sets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Value Set Identification
    value_set_name VARCHAR(100) NOT NULL,
    value_set_url VARCHAR(500),
    value_set_version VARCHAR(20) DEFAULT '1.0.0',
    
    -- HL7 to FHIR Code Mappings
    hl7_code VARCHAR(50) NOT NULL,
    hl7_display VARCHAR(200),
    hl7_code_system VARCHAR(100),
    
    fhir_code VARCHAR(50) NOT NULL,
    fhir_display VARCHAR(200),
    fhir_code_system VARCHAR(500) NOT NULL,
    
    -- Mapping Metadata
    mapping_confidence DECIMAL(3,2) DEFAULT 1.00,  -- 0.00 to 1.00
    mapping_note TEXT,
    is_default_mapping BOOLEAN DEFAULT false,
    
    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id),           -- Link to your existing users table
    is_active BOOLEAN DEFAULT true,
    
    UNIQUE(value_set_name, hl7_code)
);

-- =====================================
-- INDEXES FOR PERFORMANCE
-- =====================================

-- Core mapping table indexes
CREATE INDEX idx_fhir_mappings_message_type ON hl7_fhir_mappings(hl7_message_type);
CREATE INDEX idx_fhir_mappings_segment ON hl7_fhir_mappings(hl7_segment);
CREATE INDEX idx_fhir_mappings_priority ON hl7_fhir_mappings(priority);
CREATE INDEX idx_fhir_mappings_fhir_resource ON hl7_fhir_mappings(fhir_resource);
CREATE INDEX idx_fhir_mappings_profile ON hl7_fhir_mappings(fhir_profile);
CREATE INDEX idx_fhir_mappings_active ON hl7_fhir_mappings(is_active) WHERE is_active = true;

-- Composite index for rule lookup
CREATE INDEX idx_fhir_mappings_lookup ON hl7_fhir_mappings(hl7_message_type, fhir_profile, priority) 
    WHERE is_active = true;

-- Transformation logs indexes
CREATE INDEX idx_fhir_logs_timestamp ON fhir_transformation_logs(created_at);
CREATE INDEX idx_fhir_logs_message_type ON fhir_transformation_logs(hl7_message_type);
CREATE INDEX idx_fhir_logs_status ON fhir_transformation_logs(transformation_status);
CREATE INDEX idx_fhir_logs_request_id ON fhir_transformation_logs(request_id);
CREATE INDEX idx_fhir_logs_user_id ON fhir_transformation_logs(user_id);

-- Analytics indexes
CREATE INDEX idx_fhir_analytics_date ON fhir_analytics_data(report_date);
CREATE INDEX idx_fhir_analytics_message_type ON fhir_analytics_data(message_type);

-- Lineage tracking indexes
CREATE INDEX idx_fhir_lineage_source_id ON fhir_resource_lineage(source_message_id);
CREATE INDEX idx_fhir_lineage_fhir_resource ON fhir_resource_lineage(fhir_resource_type, fhir_resource_id);
CREATE INDEX idx_fhir_lineage_bundle ON fhir_resource_lineage(bundle_id);

-- Value sets indexes
CREATE INDEX idx_fhir_valuesets_name ON fhir_custom_value_sets(value_set_name);
CREATE INDEX idx_fhir_valuesets_hl7_code ON fhir_custom_value_sets(hl7_code);
CREATE INDEX idx_fhir_valuesets_active ON fhir_custom_value_sets(is_active) WHERE is_active = true;

-- =====================================
-- SEED FHIR PROFILES DATA
-- =====================================
INSERT INTO fhir_profile_registry (profile_name, profile_version, profile_url, base_resource, profile_definition) VALUES
('base', '4.0.1', 'http://hl7.org/fhir/StructureDefinition/Patient', 'Patient', '{"resourceType": "StructureDefinition", "name": "Patient"}'),
('us-core-patient', '6.1.0', 'http://hl7.org/fhir/us/core/StructureDefinition/us-core-patient', 'Patient', '{"resourceType": "StructureDefinition", "name": "USCorePatient"}'),
('us-core-encounter', '6.1.0', 'http://hl7.org/fhir/us/core/StructureDefinition/us-core-encounter', 'Encounter', '{"resourceType": "StructureDefinition", "name": "USCoreEncounter"}');

-- =====================================
-- SEED COMMON VALUE SET MAPPINGS
-- =====================================
INSERT INTO fhir_custom_value_sets (value_set_name, hl7_code, hl7_display, fhir_code, fhir_display, fhir_code_system) VALUES
('administrative-gender', 'M', 'Male', 'male', 'Male', 'http://hl7.org/fhir/administrative-gender'),
('administrative-gender', 'F', 'Female', 'female', 'Female', 'http://hl7.org/fhir/administrative-gender'),
('administrative-gender', 'O', 'Other', 'other', 'Other', 'http://hl7.org/fhir/administrative-gender'),
('administrative-gender', 'U', 'Unknown', 'unknown', 'Unknown', 'http://hl7.org/fhir/administrative-gender'),

('marital-status', 'M', 'Married', 'M', 'Married', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus'),
('marital-status', 'S', 'Single', 'S', 'Never Married', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus'),
('marital-status', 'D', 'Divorced', 'D', 'Divorced', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus'),
('marital-status', 'W', 'Widowed', 'W', 'Widowed', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus');

-- =====================================
-- SAMPLE ADT^A01 TRANSFORMATION RULES
-- =====================================
INSERT INTO hl7_fhir_mappings (hl7_message_type, hl7_segment, hl7_field, fhir_resource, fhir_profile, fhir_path, transformation_rule, priority) VALUES

-- Patient ID
('ADT^A01', 'PID', '3', 'Patient', 'base', 'identifier', 
'{"type": "identifier", "transform": "hl7_cx_to_fhir_identifier"}', 10),

-- Patient Name
('ADT^A01', 'PID', '5', 'Patient', 'base', 'name', 
'{"type": "humanName", "transform": "hl7_xpn_to_fhir_name"}', 20),

-- Date of Birth
('ADT^A01', 'PID', '7', 'Patient', 'base', 'birthDate', 
'{"type": "date", "transform": "hl7_date_to_fhir_date"}', 30),

-- Administrative Gender
('ADT^A01', 'PID', '8', 'Patient', 'base', 'gender', 
'{"type": "code", "transform": "hl7_code_to_fhir_code", "valueSet": "administrative-gender"}', 40),

-- Patient Address
('ADT^A01', 'PID', '11', 'Patient', 'base', 'address', 
'{"type": "address", "transform": "hl7_xad_to_fhir_address"}', 50),

-- Phone Number
('ADT^A01', 'PID', '13', 'Patient', 'base', 'telecom', 
'{"type": "contactPoint", "transform": "hl7_xtn_to_fhir_telecom", "system": "phone", "use": "home"}', 60),

-- Encounter Class (from PV1)
('ADT^A01', 'PV1', '2', 'Encounter', 'base', 'class', 
'{"type": "coding", "transform": "hl7_code_to_fhir_coding", "system": "http://terminology.hl7.org/CodeSystem/v3-ActCode"}', 70);

-- =====================================
-- UPDATE TIMESTAMP TRIGGER (Optional)
-- =====================================
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to tables that have updated_at
CREATE TRIGGER update_fhir_mappings_updated_at BEFORE UPDATE
    ON hl7_fhir_mappings FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_fhir_profiles_updated_at BEFORE UPDATE
    ON fhir_profile_registry FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_fhir_valuesets_updated_at BEFORE UPDATE
    ON fhir_custom_value_sets FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- =====================================
-- VERIFICATION QUERY
-- =====================================
SELECT 'FHIR Transformation Tables Setup Complete!' as status,
       COUNT(*) FILTER (WHERE table_name = 'hl7_fhir_mappings') as mapping_rules_table,
       COUNT(*) FILTER (WHERE table_name = 'fhir_profile_registry') as profiles_table,
       COUNT(*) FILTER (WHERE table_name = 'fhir_transformation_logs') as logs_table,
       COUNT(*) FILTER (WHERE table_name = 'fhir_analytics_data') as analytics_table,
       COUNT(*) FILTER (WHERE table_name = 'fhir_resource_lineage') as lineage_table,
       COUNT(*) FILTER (WHERE table_name = 'fhir_custom_value_sets') as valuesets_table
FROM information_schema.tables 
WHERE table_schema = 'public' 
  AND table_name IN ('hl7_fhir_mappings', 'fhir_profile_registry', 'fhir_transformation_logs', 
                     'fhir_analytics_data', 'fhir_resource_lineage', 'fhir_custom_value_sets');

-- Show sample data count
SELECT 
    (SELECT COUNT(*) FROM hl7_fhir_mappings) as sample_mapping_rules,
    (SELECT COUNT(*) FROM fhir_profile_registry) as fhir_profiles,
    (SELECT COUNT(*) FROM fhir_custom_value_sets) as value_set_mappings;