-- V2__seed_data.sql
-- Initial seed data for production system

-- Insert default admin user with proper password hash
INSERT INTO users (
    email, password_hash, first_name, last_name, role, status,
    email_verified, timezone, locale, data_consent_given
) VALUES (
    'admin@ezhealthkonnect.com',
    '$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi', -- admin123
    'System',
    'Administrator',
    'admin',
    'active',
    true,
    'UTC',
    'en',
    true
) ON CONFLICT (email) DO NOTHING;

-- Insert basic HL7 to FHIR mappings for common message types
INSERT INTO hl7_fhir_mappings (
    hl7_version, hl7_message_type, hl7_segment, hl7_field, 
    fhir_resource, fhir_profile, fhir_path, transformation_rule,
    is_required, priority, is_active, created_by
) VALUES 
-- Patient mappings from PID segment
('2.5', 'ADT^A01', 'PID', 'PID.3', 'Patient', 'Patient', 'identifier', '{"type": "direct", "datatype": "Identifier"}', true, 1, true, (SELECT id FROM users WHERE email = 'admin@ezhealthkonnect.com')),
('2.5', 'ADT^A01', 'PID', 'PID.5', 'Patient', 'Patient', 'name', '{"type": "direct", "datatype": "HumanName"}', true, 1, true, (SELECT id FROM users WHERE email = 'admin@ezhealthkonnect.com')),
('2.5', 'ADT^A01', 'PID', 'PID.7', 'Patient', 'Patient', 'birthDate', '{"type": "date_transform", "format": "YYYYMMDD"}', false, 1, true, (SELECT id FROM users WHERE email = 'admin@ezhealthkonnect.com')),
('2.5', 'ADT^A01', 'PID', 'PID.8', 'Patient', 'Patient', 'gender', '{"type": "code_mapping", "table": "HL70001"}', false, 1, true, (SELECT id FROM users WHERE email = 'admin@ezhealthkonnect.com'))
ON CONFLICT DO NOTHING;

-- Insert basic value set mappings
INSERT INTO value_set_mappings (
    mapping_name, hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, mapping_type
) VALUES 
('Gender Mapping', 'HL70001', 'M', 'http://hl7.org/fhir/administrative-gender', 'male', 'Male', 'administrative'),
('Gender Mapping', 'HL70001', 'F', 'http://hl7.org/fhir/administrative-gender', 'female', 'Female', 'administrative'),
('Gender Mapping', 'HL70001', 'O', 'http://hl7.org/fhir/administrative-gender', 'other', 'Other', 'administrative'),
('Gender Mapping', 'HL70001', 'U', 'http://hl7.org/fhir/administrative-gender', 'unknown', 'Unknown', 'administrative')
ON CONFLICT DO NOTHING;