-- migrations/postgres/001_create_fhir_tables.sql
-- PostgreSQL schema for FHIR metadata storage

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- FHIR Versions table
CREATE TABLE fhir_versions (
    id SERIAL PRIMARY KEY,
    version VARCHAR(20) NOT NULL UNIQUE,  -- 'R4', 'R5'
    description TEXT,
    release_date DATE,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- FHIR Resource types
CREATE TABLE fhir_resources (
    id SERIAL PRIMARY KEY,
    resource_type VARCHAR(50) NOT NULL,
    version_id INTEGER REFERENCES fhir_versions(id),
    base_resource VARCHAR(50),
    kind VARCHAR(20),                    -- 'resource', 'complex-type', 'primitive-type'
    is_abstract BOOLEAN DEFAULT false,
    description TEXT,
    url TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(resource_type, version_id)
);

-- FHIR Profiles (US Core, etc.)
CREATE TABLE fhir_profiles (
    id SERIAL PRIMARY KEY,
    profile_name VARCHAR(100) NOT NULL,  -- 'us-core', 'base'
    display_name VARCHAR(200),           -- 'US Core Patient Profile'
    version VARCHAR(20),                 -- '6.1.0'
    base_resource_id INTEGER REFERENCES fhir_resources(id),
    description TEXT,
    url TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(profile_name, version)
);

-- FHIR Elements (fields within resources)
CREATE TABLE fhir_elements (
    id SERIAL PRIMARY KEY,
    resource_id INTEGER REFERENCES fhir_resources(id),
    profile_id INTEGER REFERENCES fhir_profiles(id),
    path VARCHAR(200) NOT NULL,          -- 'Patient.name', 'Patient.name.family'
    name VARCHAR(100),                   -- 'Patient Name', 'Family Name'
    short_description VARCHAR(500),      -- Brief description
    definition TEXT,                     -- Full definition
    data_type VARCHAR(50),               -- 'HumanName', 'string', 'code'
    min_cardinality INTEGER DEFAULT 0,   -- Minimum occurrences
    max_cardinality VARCHAR(10),         -- Maximum occurrences ('1', '*')
    is_required BOOLEAN GENERATED ALWAYS AS (min_cardinality > 0) STORED,
    is_must_support BOOLEAN DEFAULT false,
    is_modifier BOOLEAN DEFAULT false,
    is_summary BOOLEAN DEFAULT false,
    position INTEGER,                    -- Order within parent
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_fhir_elements_path (resource_id, path),
    INDEX idx_fhir_elements_required (is_required) WHERE is_required = true
);

-- FHIR Value Sets and Bindings
CREATE TABLE fhir_value_sets (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    url TEXT NOT NULL UNIQUE,
    version VARCHAR(20),
    title VARCHAR(200),
    description TEXT,
    status VARCHAR(20),                  -- 'active', 'draft', 'retired'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- FHIR Element Bindings (element to value set relationships)
CREATE TABLE fhir_element_bindings (
    id SERIAL PRIMARY KEY,
    element_id INTEGER REFERENCES fhir_elements(id),
    value_set_id INTEGER REFERENCES fhir_value_sets(id),
    strength VARCHAR(20),               -- 'required', 'extensible', 'preferred', 'example'
    description TEXT,
    
    UNIQUE(element_id, value_set_id)
);

-- FHIR Constraints (validation rules)
CREATE TABLE fhir_constraints (
    id SERIAL PRIMARY KEY,
    element_id INTEGER REFERENCES fhir_elements(id),
    constraint_key VARCHAR(50),         -- 'pat-1', 'ele-1'
    severity VARCHAR(20),               -- 'error', 'warning'
    human_readable TEXT,                -- Human-readable description
    fhir_path_expression TEXT,          -- FHIRPath expression
    xpath_expression TEXT,              -- XPath expression (if available)
    
    INDEX idx_fhir_constraints_element (element_id)
);

-- FHIR Data Types (reusable complex types)
CREATE TABLE fhir_data_types (
    id SERIAL PRIMARY KEY,
    type_name VARCHAR(50) NOT NULL,
    version_id INTEGER REFERENCES fhir_versions(id),
    kind VARCHAR(20),                   -- 'complex-type', 'primitive-type'
    base_type VARCHAR(50),
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(type_name, version_id)
);

-- FHIR Extensions
CREATE TABLE fhir_extensions (
    id SERIAL PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    name VARCHAR(100),
    title VARCHAR(200),
    version VARCHAR(20),
    context_type VARCHAR(20),           -- 'element', 'extension'
    context_path TEXT[],                -- Array of context paths
    data_type VARCHAR(50),
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- HL7 to FHIR Mapping Rules (for transformation)
CREATE TABLE hl7_fhir_mappings (
    id SERIAL PRIMARY KEY,
    hl7_version VARCHAR(10),            -- '2.5.1'
    hl7_message_type VARCHAR(20),       -- 'ADT^A01'
    hl7_segment VARCHAR(10),            -- 'PID'
    hl7_field VARCHAR(20),              -- 'PID.5'
    hl7_component VARCHAR(10),          -- '1' (for PID.5.1)
    fhir_resource VARCHAR(50),          -- 'Patient'
    fhir_profile VARCHAR(100),          -- 'us-core-patient'
    fhir_path VARCHAR(200),             -- 'Patient.name.family'
    transformation_rule JSONB,          -- Complex transformation logic
    condition_expression TEXT,          -- When this mapping applies
    is_required BOOLEAN DEFAULT false,
    priority INTEGER DEFAULT 100,       -- Lower = higher priority
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_hl7_mappings_message (hl7_message_type),
    INDEX idx_hl7_mappings_segment (hl7_segment),
    INDEX idx_hl7_mappings_fhir (fhir_resource, fhir_profile)
);

-- Insert initial data
INSERT INTO fhir_versions (version, description, release_date, is_active) VALUES
('R4', 'FHIR R4 (4.0.1)', '2019-10-30', true),
('R5', 'FHIR R5 (5.0.0)', '2023-03-26', false);

-- Insert base profile
INSERT INTO fhir_profiles (profile_name, display_name, version, description, url) VALUES
('base', 'Base FHIR Profile', '4.0.1', 'Base FHIR resource definitions', 'http://hl7.org/fhir/StructureDefinition/'),
('us-core', 'US Core Profile', '6.1.0', 'US Core Implementation Guide', 'http://hl7.org/fhir/us/core/StructureDefinition/');