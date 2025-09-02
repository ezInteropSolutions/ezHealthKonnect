-- V1__production_schema.sql
-- Complete schema matching your existing Sequelize models

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table (matching your Sequelize model exactly)
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL,
    email_verified BOOLEAN NOT NULL DEFAULT false,
    email_verification_token VARCHAR(255),
    password_reset_token VARCHAR(255),
    password_reset_expires TIMESTAMP WITH TIME ZONE,
    last_login_at TIMESTAMP WITH TIME ZONE,
    last_login_ip INET,
    login_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until TIMESTAMP WITH TIME ZONE,
    data_consent_given BOOLEAN NOT NULL DEFAULT false,
    data_consent_date TIMESTAMP WITH TIME ZONE,
    data_retention_until TIMESTAMP WITH TIME ZONE,
    data_anonymized BOOLEAN NOT NULL DEFAULT false,
    gdpr_delete_requested BOOLEAN NOT NULL DEFAULT false,
    gdpr_delete_requested_at TIMESTAMP WITH TIME ZONE,
    phone VARCHAR(255),
    organization VARCHAR(255),
    job_title VARCHAR(255),
    department VARCHAR(255),
    timezone VARCHAR(100) NOT NULL DEFAULT 'UTC',
    locale VARCHAR(10) NOT NULL DEFAULT 'en',
    preferences JSONB,
    created_by UUID,
    updated_by UUID,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Interfaces table (matching your schema)
CREATE TABLE interfaces (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    source_type VARCHAR(100) NOT NULL,
    target_type VARCHAR(100) NOT NULL,
    message_type VARCHAR(100),
    source_config JSONB,
    target_config JSONB,
    processing_rules JSONB,
    transformation_mapping JSONB,
    status VARCHAR(50),
    total_processed INTEGER DEFAULT 0,
    successful_processed INTEGER DEFAULT 0,
    failed_processed INTEGER DEFAULT 0,
    last_processed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id),
    updated_by UUID REFERENCES users(id),
    version INTEGER DEFAULT 1,
    is_active BOOLEAN DEFAULT true
);

-- Audit logs table
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id),
    session_id VARCHAR(255),
    action VARCHAR(255) NOT NULL,
    entity_type VARCHAR(100),
    entity_id VARCHAR(255),
    old_values JSONB,
    new_values JSONB,
    metadata JSONB,
    ip_address INET,
    user_agent TEXT,
    request_id VARCHAR(255),
    result VARCHAR(100),
    error_message TEXT,
    risk_level VARCHAR(50),
    compliance_flags JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- HL7 to FHIR mappings table
CREATE TABLE hl7_fhir_mappings (
    id SERIAL PRIMARY KEY,
    hl7_version VARCHAR(50) NOT NULL,
    hl7_message_type VARCHAR(50) NOT NULL,
    hl7_segment VARCHAR(10) NOT NULL,
    hl7_field VARCHAR(50) NOT NULL,
    hl7_component VARCHAR(50),
    fhir_resource VARCHAR(100) NOT NULL,
    fhir_profile VARCHAR(255) NOT NULL,
    fhir_path VARCHAR(255) NOT NULL,
    transformation_rule JSONB NOT NULL,
    condition_expression TEXT,
    is_required BOOLEAN DEFAULT false,
    priority INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id),
    is_active BOOLEAN DEFAULT true
);

-- Value set mappings table
CREATE TABLE value_set_mappings (
    id SERIAL PRIMARY KEY,
    mapping_name VARCHAR(255) NOT NULL,
    hl7_table VARCHAR(50),
    hl7_value VARCHAR(255) NOT NULL,
    fhir_system VARCHAR(255) NOT NULL,
    fhir_code VARCHAR(100) NOT NULL,
    fhir_display TEXT,
    mapping_type VARCHAR(50),
    context_conditions JSON,
    source VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- FHIR transformation logs
CREATE TABLE fhir_transformation_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id VARCHAR(255) NOT NULL,
    source_system VARCHAR(100),
    user_id UUID REFERENCES users(id),
    hl7_message_type VARCHAR(50) NOT NULL,
    fhir_target_profile VARCHAR(255),
    rules_applied INTEGER,
    resources_created INTEGER,
    warnings_count INTEGER,
    errors_count INTEGER,
    processing_time_ms INTEGER,
    memory_usage_mb NUMERIC,
    transformation_status VARCHAR(50) NOT NULL,
    output_bundle_id VARCHAR(255),
    input_hl7_message TEXT,
    output_fhir_bundle JSONB,
    error_details JSONB,
    warnings JSONB,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_status ON users(status);
CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_interfaces_user_id ON interfaces(user_id);
CREATE INDEX idx_interfaces_status ON interfaces(status);
CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);
CREATE INDEX idx_hl7_mappings_message_type ON hl7_fhir_mappings(hl7_message_type);
CREATE INDEX idx_hl7_mappings_segment ON hl7_fhir_mappings(hl7_segment);

-- Create updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply triggers
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_interfaces_updated_at BEFORE UPDATE ON interfaces 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_hl7_mappings_updated_at BEFORE UPDATE ON hl7_fhir_mappings 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();