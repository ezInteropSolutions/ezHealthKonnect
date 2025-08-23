-- Official HL7 to FHIR Message Resource Mapping Database Schema
-- Based on official HL7 working group specifications

-- Core message type to FHIR resource templates
CREATE TABLE message_fhir_templates (
    id SERIAL PRIMARY KEY,
    message_type VARCHAR(20) NOT NULL,
    event_type VARCHAR(10),
    fhir_resources JSON NOT NULL,
    resource_conditions JSON NOT NULL,
    resource_priorities JSON NOT NULL,
    source VARCHAR(50) DEFAULT 'HL7_OFFICIAL',
    is_default BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Segment to resource mapping rules
CREATE TABLE segment_resource_mappings (
    id SERIAL PRIMARY KEY,
    segment_name VARCHAR(10) NOT NULL,
    fhir_resource_type VARCHAR(50) NOT NULL,
    mapping_conditions JSON,
    incorporation_rules JSON,
    priority INTEGER DEFAULT 1,
    multiple_instances BOOLEAN DEFAULT false,
    source VARCHAR(50) DEFAULT 'HL7_OFFICIAL',
    created_at TIMESTAMP DEFAULT NOW()
);

-- Interface-specific overrides
CREATE TABLE interface_resource_overrides (
    id SERIAL PRIMARY KEY,
    interface_id VARCHAR(50) NOT NULL,
    message_type VARCHAR(20) NOT NULL,
    fhir_resources JSON NOT NULL,
    custom_conditions JSON,
    created_by VARCHAR(100),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Insert official HL7 working group message mappings


-- Insert segment to resource mapping rules based on official HL7 WG specifications


-- Create indexes for performance
CREATE INDEX idx_message_fhir_templates_message_type ON message_fhir_templates(message_type);
CREATE INDEX idx_segment_resource_mappings_segment ON segment_resource_mappings(segment_name);
CREATE INDEX idx_interface_overrides_interface_message ON interface_resource_overrides(interface_id, message_type);

-- Comments for documentation
COMMENT ON TABLE message_fhir_templates IS 'Official HL7 working group message type to FHIR resource mappings';
COMMENT ON TABLE segment_resource_mappings IS 'Detailed segment to FHIR resource mapping rules with conditions and incorporation logic';
COMMENT ON TABLE interface_resource_overrides IS 'Interface-specific customizations of default resource mappings';


-- Extended HL7 to FHIR Message Resource Mapping - Additional Message Types
-- Based on official HL7 working group specifications

-- Insert additional official HL7 working group message mappings


-- Insert additional segment to resource mapping rules


-- Create additional indexes for new message types
CREATE INDEX idx_message_fhir_templates_event_type ON message_fhir_templates(event_type) WHERE event_type IS NOT NULL;
CREATE INDEX idx_segment_resource_mappings_message_type ON segment_resource_mappings((mapping_conditions->>'message_type')) WHERE mapping_conditions->>'message_type' IS NOT NULL;

-- Comments for new mappings
COMMENT ON COLUMN message_fhir_templates.event_type IS 'Specific event type for message variants (e.g., A01, A02, A03 for ADT messages)';
COMMENT ON COLUMN segment_resource_mappings.mapping_conditions IS 'JSON conditions for when this mapping applies, including message type context';
COMMENT ON COLUMN segment_resource_mappings.incorporation_rules IS 'JSON rules for how segment data incorporates into FHIR resources';

-- Extended HL7 to FHIR Message Resource Mapping - Additional Message Types
-- Based on official HL7 working group specifications

-- Insert additional official HL7 working group message mappings


-- Insert additional segment to resource mapping rules



-- =====================================================
-- ELEMENT-TO-ELEMENT FIELD MAPPINGS
-- =====================================================

-- Field-level mappings from HL7 segments to FHIR resource elements
CREATE TABLE field_element_mappings (
    id SERIAL PRIMARY KEY,
    segment_name VARCHAR(10) NOT NULL,
    hl7_field VARCHAR(20) NOT NULL,
    hl7_component VARCHAR(10),
    hl7_subcomponent VARCHAR(10),
    fhir_resource_type VARCHAR(50) NOT NULL,
    fhir_element_path TEXT NOT NULL,
    data_type_transform VARCHAR(50),
    value_set_mapping_id INTEGER,
    mapping_conditions JSON,
    transformation_rules JSON,
    is_required BOOLEAN DEFAULT false,
    cardinality VARCHAR(10) DEFAULT '0..1',
    source VARCHAR(50) DEFAULT 'HL7_OFFICIAL',
    created_at TIMESTAMP DEFAULT NOW()
);

-- Value set mappings between HL7 tables and FHIR CodeSystems
CREATE TABLE value_set_mappings (
    id SERIAL PRIMARY KEY,
    mapping_name VARCHAR(100) NOT NULL,
    hl7_table VARCHAR(20),
    hl7_value VARCHAR(100) NOT NULL,
    fhir_system VARCHAR(200) NOT NULL,
    fhir_code VARCHAR(100) NOT NULL,
    fhir_display TEXT,
    mapping_type VARCHAR(50) DEFAULT 'direct',
    context_conditions JSON,
    source VARCHAR(50) DEFAULT 'HL7_OFFICIAL',
    created_at TIMESTAMP DEFAULT NOW()
);

-- Data type transformation rules
CREATE TABLE data_type_transforms (
    id SERIAL PRIMARY KEY,
    transform_name VARCHAR(100) NOT NULL,
    hl7_data_type VARCHAR(20) NOT NULL,
    fhir_data_type VARCHAR(50) NOT NULL,
    transformation_function TEXT,
    validation_rules JSON,
    examples JSON,
    source VARCHAR(50) DEFAULT 'HL7_OFFICIAL',
    created_at TIMESTAMP DEFAULT NOW()
);

-- =====================================================
-- VALUE SET MAPPINGS DATA
-- =====================================================

-- Insert value set mappings for common HL7 tables
INSERT INTO value_set_mappings (mapping_name, hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, mapping_type) VALUES

-- Table 0001 - Administrative Sex
('administrative_sex', '0001', 'M', 'http://hl7.org/fhir/administrative-gender', 'male', 'Male', 'direct'),
('administrative_sex', '0001', 'F', 'http://hl7.org/fhir/administrative-gender', 'female', 'Female', 'direct'),
('administrative_sex', '0001', 'O', 'http://hl7.org/fhir/administrative-gender', 'other', 'Other', 'direct'),
('administrative_sex', '0001', 'U', 'http://hl7.org/fhir/administrative-gender', 'unknown', 'Unknown', 'direct'),

-- Table 0002 - Marital Status
('marital_status', '0002', 'A', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus', 'A', 'Annulled', 'direct'),
('marital_status', '0002', 'D', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus', 'D', 'Divorced', 'direct'),
('marital_status', '0002', 'I', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus', 'I', 'Interlocutory', 'direct'),
('marital_status', '0002', 'L', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus', 'L', 'Legally Separated', 'direct'),
('marital_status', '0002', 'M', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus', 'M', 'Married', 'direct'),
('marital_status', '0002', 'P', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus', 'P', 'Polygamous', 'direct'),
('marital_status', '0002', 'S', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus', 'S', 'Never Married', 'direct'),
('marital_status', '0002', 'T', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus', 'T', 'Domestic partner', 'direct'),
('marital_status', '0002', 'U', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus', 'U', 'unmarried', 'direct'),
('marital_status', '0002', 'W', 'http://terminology.hl7.org/CodeSystem/v3-MaritalStatus', 'W', 'Widowed', 'direct'),

-- Table 0063 - Relationship
('relationship', '0063', 'SEL', 'http://terminology.hl7.org/CodeSystem/v2-0131', 'N', 'Next-of-Kin', 'direct'),
('relationship', '0063', 'SPO', 'http://terminology.hl7.org/CodeSystem/v3-RoleCode', 'SPS', 'Spouse', 'direct'),
('relationship', '0063', 'DOM', 'http://terminology.hl7.org/CodeSystem/v3-RoleCode', 'DOMPART', 'Domestic Partner', 'direct'),
('relationship', '0063', 'CHD', 'http://terminology.hl7.org/CodeSystem/v3-RoleCode', 'CHILD', 'Child', 'direct'),
('relationship', '0063', 'PAR', 'http://terminology.hl7.org/CodeSystem/v3-RoleCode', 'PRN', 'Parent', 'direct'),
('relationship', '0063', 'SIB', 'http://terminology.hl7.org/CodeSystem/v3-RoleCode', 'SIB', 'Sibling', 'direct'),

-- Table 0078 - Interpretation Codes
('interpretation_codes', '0078', 'A', 'http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation', 'A', 'Abnormal', 'direct'),
('interpretation_codes', '0078', 'AA', 'http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation', 'AA', 'Critical abnormal', 'direct'),
('interpretation_codes', '0078', 'HH', 'http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation', 'HH', 'Critical high', 'direct'),
('interpretation_codes', '0078', 'LL', 'http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation', 'LL', 'Critical low', 'direct'),
('interpretation_codes', '0078', 'H', 'http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation', 'H', 'High', 'direct'),
('interpretation_codes', '0078', 'L', 'http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation', 'L', 'Low', 'direct'),
('interpretation_codes', '0078', 'N', 'http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation', 'N', 'Normal', 'direct'),

-- Table 0085 - Observation Result Status
('observation_status', '0085', 'C', 'http://hl7.org/fhir/observation-status', 'corrected', 'Corrected', 'direct'),
('observation_status', '0085', 'D', 'http://hl7.org/fhir/observation-status', 'cancelled', 'Cancelled', 'direct'),
('observation_status', '0085', 'F', 'http://hl7.org/fhir/observation-status', 'final', 'Final', 'direct'),
('observation_status', '0085', 'I', 'http://hl7.org/fhir/observation-status', 'registered', 'Registered', 'direct'),
('observation_status', '0085', 'P', 'http://hl7.org/fhir/observation-status', 'preliminary', 'Preliminary', 'direct'),
('observation_status', '0085', 'R', 'http://hl7.org/fhir/observation-status', 'registered', 'Registered', 'direct'),
('observation_status', '0085', 'S', 'http://hl7.org/fhir/observation-status', 'preliminary', 'Preliminary', 'direct'),
('observation_status', '0085', 'U', 'http://hl7.org/fhir/observation-status', 'unknown', 'Unknown', 'direct'),
('observation_status', '0085', 'W', 'http://hl7.org/fhir/observation-status', 'corrected', 'Corrected', 'direct'),
('observation_status', '0085', 'X', 'http://hl7.org/fhir/observation-status', 'cancelled', 'Cancelled', 'direct');

-- =====================================================
-- DATA TYPE TRANSFORMATIONS
-- =====================================================

INSERT INTO data_type_transforms (transform_name, hl7_data_type, fhir_data_type, transformation_function, validation_rules, examples) VALUES

('cx_to_identifier', 'CX', 'Identifier', 
 'function(cx) { 
   return {
     value: cx.components[0],
     type: {
       coding: [{
         system: "http://terminology.hl7.org/CodeSystem/v2-0203",
         code: cx.components[4] || "MR"
       }]
     },
     system: cx.components[3],
     assigner: cx.components[5] ? { display: cx.components[5] } : undefined
   };
 }',
 '{"required_components": [0], "optional_components": [3,4,5]}',
 '{"hl7": "12345^^^EPIC^MR^Epic", "fhir": {"value": "12345", "system": "EPIC", "type": {"coding": [{"system": "http://terminology.hl7.org/CodeSystem/v2-0203", "code": "MR"}]}}}'),

('xpn_to_humanname', 'XPN', 'HumanName',
 'function(xpn) {
   return {
     family: xpn.components[0],
     given: [xpn.components[1], xpn.components[2]].filter(x => x),
     prefix: xpn.components[5] ? [xpn.components[5]] : undefined,
     suffix: xpn.components[3] ? [xpn.components[3]] : undefined,
     use: mapNameType(xpn.components[6])
   };
 }',
 '{"required_components": [0], "name_type_mapping": {"L": "official", "D": "usual", "M": "maiden"}}',
 '{"hl7": "Doe^John^Middle^Jr^Mr", "fhir": {"family": "Doe", "given": ["John", "Middle"], "suffix": ["Jr"], "prefix": ["Mr"]}}'),

('xad_to_address', 'XAD', 'Address',
 'function(xad) {
   return {
     line: [xad.components[0], xad.components[1]].filter(x => x),
     city: xad.components[2],
     state: xad.components[3],
     postalCode: xad.components[4],
     country: xad.components[5],
     use: mapAddressType(xad.components[6]),
     type: mapAddressType(xad.components[7])
   };
 }',
 '{"address_type_mapping": {"H": "home", "O": "work", "T": "temp"}}',
 '{"hl7": "123 Main St^^Springfield^IL^62701^USA^H", "fhir": {"line": ["123 Main St"], "city": "Springfield", "state": "IL", "postalCode": "62701", "country": "USA", "use": "home"}}'),

('xtn_to_contactpoint', 'XTN', 'ContactPoint',
 'function(xtn) {
   return {
     system: mapTelecomSystem(xtn.components[2]),
     value: xtn.components[0] || xtn.components[3],
     use: mapTelecomUse(xtn.components[1])
   };
 }',
 '{"telecom_system_mapping": {"PH": "phone", "FX": "fax", "Internet": "email"}}',
 '{"hl7": "555-1234^WPN^PH", "fhir": {"system": "phone", "value": "555-1234", "use": "work"}}'),

('ts_to_datetime', 'TS', 'dateTime',
 'function(ts) {
   if (!ts) return null;
   const year = ts.substring(0,4);
   const month = ts.substring(4,6);
   const day = ts.substring(6,8);
   const hour = ts.substring(8,10) || "00";
   const minute = ts.substring(10,12) || "00";
   const second = ts.substring(12,14) || "00";
   return `${year}-${month}-${day}T${hour}:${minute}:${second}`;
 }',
 '{"format": "YYYYMMDDHHMMSS", "min_length": 8}',
 '{"hl7": "20231215143022", "fhir": "2023-12-15T14:30:22"}'),

('ce_to_codeableconcept', 'CE', 'CodeableConcept',
 'function(ce) {
   return {
     coding: [{
       system: mapCodingSystem(ce.components[2]),
       code: ce.components[0],
       display: ce.components[1]
     }],
     text: ce.components[1]
   };
 }',
 '{"coding_system_mapping": {"LN": "http://loinc.org", "SCT": "http://snomed.info/sct"}}',
 '{"hl7": "M^Male^HL70001", "fhir": {"coding": [{"system": "http://terminology.hl7.org/CodeSystem/v3-AdministrativeGender", "code": "M", "display": "Male"}]}}');

-- =====================================================
-- FIELD-TO-ELEMENT MAPPINGS
-- =====================================================

INSERT INTO field_element_mappings (segment_name, hl7_field, hl7_component, fhir_resource_type, fhir_element_path, data_type_transform, value_set_mapping_id, is_required, cardinality, mapping_conditions, transformation_rules) VALUES

-- PID Patient Identification segment mappings
('PID', 'PID.3', null, 'Patient', 'identifier', 'cx_to_identifier', null, true, '1..*', '{}', '{"primary_identifier": "first_occurrence"}'),
('PID', 'PID.5', null, 'Patient', 'name', 'xpn_to_humanname', null, true, '0..*', '{}', '{"official_name": "first_occurrence"}'),
('PID', 'PID.7', null, 'Patient', 'birthDate', 'ts_to_datetime', null, false, '0..1', '{}', '{"date_only": true}'),
('PID', 'PID.8', null, 'Patient', 'gender', null, 1, false, '0..1', '{}', '{}'),
('PID', 'PID.11', null, 'Patient', 'address', 'xad_to_address', null, false, '0..*', '{}', '{}'),
('PID', 'PID.13', null, 'Patient', 'telecom', 'xtn_to_contactpoint', null, false, '0..*', '{"telecom_type": "phone"}', '{}'),
('PID', 'PID.14', null, 'Patient', 'telecom', 'xtn_to_contactpoint', null, false, '0..*', '{"telecom_type": "work"}', '{}'),
('PID', 'PID.16', null, 'Patient', 'maritalStatus', 'ce_to_codeableconcept', 5, false, '0..1', '{}', '{}'),
('PID', 'PID.18', null, 'Patient', 'identifier', 'cx_to_identifier', null, false, '0..*', '{"identifier_type": "account"}', '{}'),
('PID', 'PID.29', null, 'Patient', 'deceasedDateTime', 'ts_to_datetime', null, false, '0..1', '{}', '{}'),

-- PV1 Patient Visit segment mappings
('PV1', 'PV1.2', null, 'Encounter', 'class', 'ce_to_codeableconcept', null, true, '1..1', '{}', '{}'),
('PV1', 'PV1.3', null, 'Encounter', 'location.location', null, null, false, '0..1', '{}', '{"create_location_reference": true}'),
('PV1', 'PV1.7', null, 'Encounter', 'participant.individual', null, null, false, '0..*', '{"participant_type": "attending"}', '{"create_practitioner_reference": true}'),
('PV1', 'PV1.8', null, 'Encounter', 'participant.individual', null, null, false, '0..*', '{"participant_type": "referring"}', '{"create_practitioner_reference": true}'),
('PV1', 'PV1.9', null, 'Encounter', 'participant.individual', null, null, false, '0..*', '{"participant_type": "consulting"}', '{"create_practitioner_reference": true}'),
('PV1', 'PV1.17', null, 'Encounter', 'participant.individual', null, null, false, '0..*', '{"participant_type": "admitting"}', '{"create_practitioner_reference": true}'),
('PV1', 'PV1.19', null, 'Encounter', 'identifier', 'cx_to_identifier', null, false, '0..*', '{}', '{}'),
('PV1', 'PV1.44', null, 'Encounter', 'period.start', 'ts_to_datetime', null, false, '0..1', '{}', '{}'),
('PV1', 'PV1.45', null, 'Encounter', 'period.end', 'ts_to_datetime', null, false, '0..1', '{}', '{}'),

-- OBX Observation segment mappings
('OBX', 'OBX.1', null, 'Observation', 'identifier.value', null, null, false, '0..1', '{}', '{}'),
('OBX', 'OBX.2', null, 'Observation', 'value[x]', null, null, true, '1..1', '{}', '{"determine_value_type": true}'),
('OBX', 'OBX.3', null, 'Observation', 'code', 'ce_to_codeableconcept', null, true, '1..1', '{}', '{}'),
('OBX', 'OBX.5', null, 'Observation', 'value[x]', null, null, false, '0..*', '{}', '{"based_on_obx2": true}'),
('OBX', 'OBX.6', null, 'Observation', 'component.code', 'ce_to_codeableconcept', null, false, '0..1', '{}', '{}'),
('OBX', 'OBX.7', null, 'Observation', 'referenceRange.text', null, null, false, '0..1', '{}', '{}'),
('OBX', 'OBX.8', null, 'Observation', 'interpretation', 'ce_to_codeableconcept', 31, false, '0..*', '{}', '{}'),
('OBX', 'OBX.11', null, 'Observation', 'status', null, 38, true, '1..1', '{}', '{}'),
('OBX', 'OBX.14', null, 'Observation', 'effectiveDateTime', 'ts_to_datetime', null, false, '0..1', '{}', '{}'),

-- MSH Message Header mappings
('MSH', 'MSH.3', null, 'MessageHeader', 'source.name', null, null, false, '0..1', '{}', '{}'),
('MSH', 'MSH.4', null, 'MessageHeader', 'source.endpoint', null, null, false, '0..1', '{}', '{}'),
('MSH', 'MSH.5', null, 'MessageHeader', 'destination.name', null, null, false, '0..1', '{}', '{}'),
('MSH', 'MSH.6', null, 'MessageHeader', 'destination.endpoint', null, null, false, '0..1', '{}', '{}'),
('MSH', 'MSH.7', null, 'MessageHeader', 'timestamp', 'ts_to_datetime', null, true, '1..1', '{}', '{}'),
('MSH', 'MSH.9', null, 'MessageHeader', 'eventCoding', 'ce_to_codeableconcept', null, true, '1..1', '{}', '{}'),
('MSH', 'MSH.10', null, 'MessageHeader', 'identifier', null, null, true, '1..1', '{}', '{}'),

-- AL1 Allergy Information mappings
('AL1', 'AL1.2', null, 'AllergyIntolerance', 'type', null, null, false, '0..1', '{}', '{}'),
('AL1', 'AL1.3', null, 'AllergyIntolerance', 'code', 'ce_to_codeableconcept', null, true, '1..1', '{}', '{}'),
('AL1', 'AL1.4', null, 'AllergyIntolerance', 'criticality', null, null, false, '0..1', '{}', '{}'),
('AL1', 'AL1.5', null, 'AllergyIntolerance', 'reaction.manifestation', 'ce_to_codeableconcept', null, false, '0..*', '{}', '{}'),
('AL1', 'AL1.6', null, 'AllergyIntolerance', 'onsetDateTime', 'ts_to_datetime', null, false, '0..1', '{}', '{}'),

-- DG1 Diagnosis mappings
('DG1', 'DG1.3', null, 'Condition', 'code', 'ce_to_codeableconcept', null, true, '1..1', '{}', '{}'),
('DG1', 'DG1.4', null, 'Condition', 'code.text', null, null, false, '0..1', '{}', '{}'),
('DG1', 'DG1.5', null, 'Condition', 'recordedDate', 'ts_to_datetime', null, false, '0..1', '{}', '{}'),
('DG1', 'DG1.6', null, 'Condition', 'category', null, null, false, '0..*', '{}', '{}'),

-- NK1 Next of Kin mappings
('NK1', 'NK1.2', null, 'RelatedPerson', 'name', 'xpn_to_humanname', null, false, '0..*', '{}', '{}'),
('NK1', 'NK1.3', null, 'RelatedPerson', 'relationship', 'ce_to_codeableconcept', 14, false, '0..*', '{}', '{}'),
('NK1', 'NK1.4', null, 'RelatedPerson', 'address', 'xad_to_address', null, false, '0..*', '{}', '{}'),
('NK1', 'NK1.5', null, 'RelatedPerson', 'telecom', 'xtn_to_contactpoint', null, false, '0..*', '{}', '{}'),
('NK1', 'NK1.7', null, 'RelatedPerson', 'relationship', 'ce_to_codeableconcept', null, false, '0..*', '{}', '{}');

-- Create indexes for performance
CREATE INDEX idx_field_element_mappings_segment_field ON field_element_mappings(segment_name, hl7_field);
CREATE INDEX idx_value_set_mappings_table_value ON value_set_mappings(hl7_table, hl7_value);
CREATE INDEX idx_data_type_transforms_hl7_type ON data_type_transforms(hl7_data_type);

-- Add foreign key constraint

-- Comments for new mappings
COMMENT ON COLUMN message_fhir_templates.event_type IS 'Specific event type for message variants (e.g., A01, A02, A03 for ADT messages)';
COMMENT ON COLUMN segment_resource_mappings.mapping_conditions IS 'JSON conditions for when this mapping applies, including message type context';
COMMENT ON COLUMN segment_resource_mappings.incorporation_rules IS 'JSON rules for how segment data incorporates into FHIR resources';
COMMENT ON TABLE field_element_mappings IS 'Detailed field-level mappings from HL7 segment fields to FHIR resource elements';
COMMENT ON TABLE value_set_mappings IS 'Value set mappings between HL7 table values and FHIR CodeSystem codes';
COMMENT ON TABLE data_type_transforms IS 'Data type transformation rules between HL7 and FHIR data types';

-- Deduplicated Inserts --
INSERT INTO message_fhir_templates (message_type, fhir_resources, resource_conditions, resource_priorities) VALUES 

-- ADT^A01 - Patient Admission (Official HL7 WG Mapping)
('ADT^A01', 
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Account", "Encounter", "Basic", "Coverage", "RelatedPerson", "Observation", "AllergyIntolerance", "Condition", "EpisodeOfCare", "Procedure"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,EVN,PID", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1,PV1", "required": true, "priority": 2},
   "Account": {"from": "PID", "condition": "account_needed", "priority": 3},
   "Encounter": {"from": "PV1,PV2", "required": true, "priority": 3, "references": ["Patient"]},
   "Basic": {"from": "PV1", "condition": "PV1.43_valued", "priority": 4},
   "Coverage": {"from": "PV1,IN1", "condition": "insurance_present", "priority": 4},
   "RelatedPerson": {"from": "NK1", "condition": "next_of_kin_present", "multiple": true, "priority": 5},
   "Observation": {"from": "PD1,OBX", "condition": "observations_present", "multiple": true, "priority": 6},
   "AllergyIntolerance": {"from": "AL1", "condition": "allergies_present", "multiple": true, "priority": 7},
   "Condition": {"from": "DG1", "condition": "diagnosis_present", "multiple": true, "priority": 8},
   "EpisodeOfCare": {"from": "DG1", "condition": "episode_context", "priority": 8},
   "Procedure": {"from": "PR1", "condition": "procedures_present", "multiple": true, "priority": 9}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Account": 3, "Encounter": 3, "Basic": 4, "Coverage": 4, "RelatedPerson": 5, "Observation": 6, "AllergyIntolerance": 7, "Condition": 8, "EpisodeOfCare": 8, "Procedure": 9}'),

-- ORU^R01 - Lab Results (Official HL7 WG Mapping)
('ORU^R01',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Encounter", "Basic", "Coverage", "RelatedPerson", "DiagnosticReport", "ServiceRequest", "Observation", "Specimen", "Device", "PractitionerRole"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,PID", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1,PV1", "required": true, "priority": 2},
   "Encounter": {"from": "PV1,PV2", "condition": "visit_present", "priority": 3, "references": ["Patient"]},
   "Basic": {"from": "PV1", "condition": "PV1.43_valued", "priority": 4},
   "Coverage": {"from": "PV1", "condition": "PV1.20_valued", "priority": 4},
   "RelatedPerson": {"from": "NK1,PRT", "condition": "related_person_present", "multiple": true, "priority": 5},
   "DiagnosticReport": {"from": "ORC,OBR", "required": true, "multiple": true, "priority": 6, "references": ["Patient", "Encounter"]},
   "ServiceRequest": {"from": "ORC,OBR", "condition": "service_request_needed", "multiple": true, "priority": 6},
   "Observation": {"from": "OBX", "multiple": true, "priority": 7, "references": ["DiagnosticReport", "Patient"]},
   "Specimen": {"from": "OBR,SPM", "condition": "specimen_present", "multiple": true, "priority": 8},
   "Device": {"from": "PRT", "condition": "device_present", "multiple": true, "priority": 9},
   "PractitionerRole": {"from": "PRT", "condition": "practitioner_present", "multiple": true, "priority": 9}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Encounter": 3, "Basic": 4, "Coverage": 4, "RelatedPerson": 5, "DiagnosticReport": 6, "ServiceRequest": 6, "Observation": 7, "Specimen": 8, "Device": 9, "PractitionerRole": 9}'),

-- ORM^O01 - Orders (Official HL7 WG Mapping)
('ORM^O01',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Encounter", "Basic", "Coverage", "AllergyIntolerance", "ServiceRequest", "Task", "MedicationRequest", "SupplyRequest", "Condition", "Observation"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,PID,ORC", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1,PV1", "required": true, "priority": 2},
   "Encounter": {"from": "PV1,PV2", "condition": "visit_present", "priority": 3, "references": ["Patient"]},
   "Basic": {"from": "PV1", "condition": "PV1.43_valued", "priority": 4},
   "Coverage": {"from": "PV1,IN1,IN2,IN3", "condition": "insurance_present", "priority": 4},
   "AllergyIntolerance": {"from": "AL1", "condition": "allergies_present", "multiple": true, "priority": 5},
   "ServiceRequest": {"from": "ORC,OBR", "required": true, "multiple": true, "priority": 6, "references": ["Patient"]},
   "Task": {"from": "ORC", "condition": "task_needed", "multiple": true, "priority": 6, "references": ["ServiceRequest"]},
   "MedicationRequest": {"from": "RXO", "condition": "medication_order", "multiple": true, "priority": 7},
   "SupplyRequest": {"from": "OBR,ODS", "condition": "supply_order", "multiple": true, "priority": 7},
   "Condition": {"from": "DG1", "condition": "diagnosis_present", "multiple": true, "priority": 8, "references": ["Patient"]},
   "Observation": {"from": "PD1,OBX", "condition": "observations_present", "multiple": true, "priority": 9}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Encounter": 3, "Basic": 4, "Coverage": 4, "AllergyIntolerance": 5, "ServiceRequest": 6, "Task": 6, "MedicationRequest": 7, "SupplyRequest": 7, "Condition": 8, "Observation": 9}');

INSERT INTO message_fhir_templates (message_type, fhir_resources, resource_conditions, resource_priorities) VALUES 

-- ADT^A02 - Transfer Patient (Official HL7 WG Mapping)
('ADT^A02', 
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Account", "Encounter", "Basic", "Coverage", "RelatedPerson", "Observation", "AllergyIntolerance", "Condition", "EpisodeOfCare", "Location"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,EVN,PID", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1,PV1", "required": true, "priority": 2},
   "Account": {"from": "PID", "condition": "account_needed", "priority": 3},
   "Encounter": {"from": "PV1,PV2", "required": true, "priority": 3, "references": ["Patient"]},
   "Basic": {"from": "PV1", "condition": "PV1.43_valued", "priority": 4},
   "Coverage": {"from": "PV1,IN1", "condition": "insurance_present", "priority": 4},
   "RelatedPerson": {"from": "NK1", "condition": "next_of_kin_present", "multiple": true, "priority": 5},
   "Observation": {"from": "PD1,OBX", "condition": "observations_present", "multiple": true, "priority": 6},
   "AllergyIntolerance": {"from": "AL1", "condition": "allergies_present", "multiple": true, "priority": 7},
   "Condition": {"from": "DG1", "condition": "diagnosis_present", "multiple": true, "priority": 8},
   "EpisodeOfCare": {"from": "DG1", "condition": "episode_context", "priority": 8},
   "Location": {"from": "PV1", "condition": "location_transfer", "multiple": true, "priority": 9}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Account": 3, "Encounter": 3, "Basic": 4, "Coverage": 4, "RelatedPerson": 5, "Observation": 6, "AllergyIntolerance": 7, "Condition": 8, "EpisodeOfCare": 8, "Location": 9}'),

-- ADT^A03 - Discharge Patient (Official HL7 WG Mapping)
('ADT^A03',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Account", "Encounter", "Basic", "Coverage", "RelatedPerson", "Observation", "AllergyIntolerance", "Condition", "EpisodeOfCare", "Procedure"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,EVN,PID", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1,PV1", "required": true, "priority": 2},
   "Account": {"from": "PID", "condition": "account_needed", "priority": 3},
   "Encounter": {"from": "PV1,PV2", "required": true, "priority": 3, "references": ["Patient"]},
   "Basic": {"from": "PV1", "condition": "PV1.43_valued", "priority": 4},
   "Coverage": {"from": "PV1,IN1", "condition": "insurance_present", "priority": 4},
   "RelatedPerson": {"from": "NK1", "condition": "next_of_kin_present", "multiple": true, "priority": 5},
   "Observation": {"from": "PD1,OBX", "condition": "observations_present", "multiple": true, "priority": 6},
   "AllergyIntolerance": {"from": "AL1", "condition": "allergies_present", "multiple": true, "priority": 7},
   "Condition": {"from": "DG1", "condition": "diagnosis_present", "multiple": true, "priority": 8},
   "EpisodeOfCare": {"from": "DG1", "condition": "episode_context", "priority": 8},
   "Procedure": {"from": "PR1", "condition": "procedures_present", "multiple": true, "priority": 9}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Account": 3, "Encounter": 3, "Basic": 4, "Coverage": 4, "RelatedPerson": 5, "Observation": 6, "AllergyIntolerance": 7, "Condition": 8, "EpisodeOfCare": 8, "Procedure": 9}'),

-- ADT^A04 - Register Patient (Official HL7 WG Mapping)
('ADT^A04',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Account", "Encounter", "Basic", "Coverage", "RelatedPerson", "Observation", "AllergyIntolerance", "Condition", "EpisodeOfCare"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,EVN,PID", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1,PV1", "required": true, "priority": 2},
   "Account": {"from": "PID", "condition": "account_needed", "priority": 3},
   "Encounter": {"from": "PV1,PV2", "condition": "visit_present", "priority": 3, "references": ["Patient"]},
   "Basic": {"from": "PV1", "condition": "PV1.43_valued", "priority": 4},
   "Coverage": {"from": "PV1,IN1", "condition": "insurance_present", "priority": 4},
   "RelatedPerson": {"from": "NK1", "condition": "next_of_kin_present", "multiple": true, "priority": 5},
   "Observation": {"from": "PD1,OBX", "condition": "observations_present", "multiple": true, "priority": 6},
   "AllergyIntolerance": {"from": "AL1", "condition": "allergies_present", "multiple": true, "priority": 7},
   "Condition": {"from": "DG1", "condition": "diagnosis_present", "multiple": true, "priority": 8},
   "EpisodeOfCare": {"from": "DG1", "condition": "episode_context", "priority": 8}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Account": 3, "Encounter": 3, "Basic": 4, "Coverage": 4, "RelatedPerson": 5, "Observation": 6, "AllergyIntolerance": 7, "Condition": 8, "EpisodeOfCare": 8}'),

-- ADT^A08 - Update Patient Information (Official HL7 WG Mapping)
('ADT^A08',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Account", "Encounter", "Basic", "Coverage", "RelatedPerson", "Observation", "AllergyIntolerance", "Condition"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,EVN,PID", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1,PV1", "required": true, "priority": 2},
   "Account": {"from": "PID", "condition": "account_needed", "priority": 3},
   "Encounter": {"from": "PV1,PV2", "condition": "visit_present", "priority": 3, "references": ["Patient"]},
   "Basic": {"from": "PV1", "condition": "PV1.43_valued", "priority": 4},
   "Coverage": {"from": "PV1,IN1", "condition": "insurance_present", "priority": 4},
   "RelatedPerson": {"from": "NK1", "condition": "next_of_kin_present", "multiple": true, "priority": 5},
   "Observation": {"from": "PD1,OBX", "condition": "observations_present", "multiple": true, "priority": 6},
   "AllergyIntolerance": {"from": "AL1", "condition": "allergies_present", "multiple": true, "priority": 7},
   "Condition": {"from": "DG1", "condition": "diagnosis_present", "multiple": true, "priority": 8}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Account": 3, "Encounter": 3, "Basic": 4, "Coverage": 4, "RelatedPerson": 5, "Observation": 6, "AllergyIntolerance": 7, "Condition": 8}'),

-- SIU^S12 - Schedule Information Unsolicited (Official HL7 WG Mapping)
('SIU^S12',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Encounter", "Appointment", "AppointmentResponse", "PractitionerRole", "Location", "Schedule", "Slot"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,SCH", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1", "condition": "patient_present", "priority": 2},
   "Encounter": {"from": "PV1,PV2", "condition": "visit_present", "priority": 3, "references": ["Patient"]},
   "Appointment": {"from": "SCH,AIS,AIG,AIL,AIP", "required": true, "priority": 4, "references": ["Patient"]},
   "AppointmentResponse": {"from": "SCH", "condition": "response_needed", "priority": 5, "references": ["Appointment"]},
   "PractitionerRole": {"from": "AIP", "condition": "practitioner_present", "multiple": true, "priority": 6},
   "Location": {"from": "AIL", "condition": "location_present", "multiple": true, "priority": 7},
   "Schedule": {"from": "SCH", "condition": "recurring_schedule", "priority": 8},
   "Slot": {"from": "SCH", "condition": "slot_management", "multiple": true, "priority": 9}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Encounter": 3, "Appointment": 4, "AppointmentResponse": 5, "PractitionerRole": 6, "Location": 7, "Schedule": 8, "Slot": 9}'),

-- MDM^T02 - Document Status Change (Official HL7 WG Mapping)
('MDM^T02',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Encounter", "DocumentReference", "Composition", "PractitionerRole", "Binary"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,TXA", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1", "required": true, "priority": 2},
   "Encounter": {"from": "PV1,PV2", "condition": "visit_present", "priority": 3, "references": ["Patient"]},
   "DocumentReference": {"from": "TXA,OBX", "required": true, "priority": 4, "references": ["Patient"]},
   "Composition": {"from": "TXA", "condition": "composed_document", "priority": 5, "references": ["Patient"]},
   "PractitionerRole": {"from": "TXA", "condition": "practitioner_present", "multiple": true, "priority": 6},
   "Binary": {"from": "OBX", "condition": "binary_content", "multiple": true, "priority": 7}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Encounter": 3, "DocumentReference": 4, "Composition": 5, "PractitionerRole": 6, "Binary": 7}'),

-- VXU^V04 - Vaccination Update (Official HL7 WG Mapping)
('VXU^V04',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Encounter", "Immunization", "ImmunizationEvaluation", "ImmunizationRecommendation", "Observation", "RelatedPerson"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,PID,RXA", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1", "required": true, "priority": 2},
   "Encounter": {"from": "PV1,PV2", "condition": "visit_present", "priority": 3, "references": ["Patient"]},
   "Immunization": {"from": "RXA", "required": true, "multiple": true, "priority": 4, "references": ["Patient"]},
   "ImmunizationEvaluation": {"from": "RXA,OBX", "condition": "evaluation_present", "multiple": true, "priority": 5, "references": ["Immunization"]},
   "ImmunizationRecommendation": {"from": "RXR", "condition": "recommendation_present", "multiple": true, "priority": 6, "references": ["Patient"]},
   "Observation": {"from": "OBX", "condition": "observations_present", "multiple": true, "priority": 7, "references": ["Patient"]},
   "RelatedPerson": {"from": "NK1", "condition": "next_of_kin_present", "multiple": true, "priority": 8}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Encounter": 3, "Immunization": 4, "ImmunizationEvaluation": 5, "ImmunizationRecommendation": 6, "Observation": 7, "RelatedPerson": 8}'),

-- RDE^O11 - Pharmacy/Treatment Encoded Order (Official HL7 WG Mapping)
('RDE^O11',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Encounter", "MedicationRequest", "MedicationDispense", "ServiceRequest", "AllergyIntolerance", "Condition", "Observation", "PractitionerRole"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,ORC,RXE", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1", "required": true, "priority": 2},
   "Encounter": {"from": "PV1,PV2", "condition": "visit_present", "priority": 3, "references": ["Patient"]},
   "MedicationRequest": {"from": "ORC,RXE", "required": true, "multiple": true, "priority": 4, "references": ["Patient"]},
   "MedicationDispense": {"from": "RXD", "condition": "dispense_present", "multiple": true, "priority": 5, "references": ["MedicationRequest"]},
   "ServiceRequest": {"from": "ORC,OBR", "condition": "service_order", "multiple": true, "priority": 6, "references": ["Patient"]},
   "AllergyIntolerance": {"from": "AL1", "condition": "allergies_present", "multiple": true, "priority": 7},
   "Condition": {"from": "DG1", "condition": "diagnosis_present", "multiple": true, "priority": 8},
   "Observation": {"from": "OBX", "condition": "observations_present", "multiple": true, "priority": 9},
   "PractitionerRole": {"from": "RXE", "condition": "prescriber_present", "multiple": true, "priority": 10}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Encounter": 3, "MedicationRequest": 4, "MedicationDispense": 5, "ServiceRequest": 6, "AllergyIntolerance": 7, "Condition": 8, "Observation": 9, "PractitionerRole": 10}'),

-- RDS^O13 - Pharmacy/Treatment Dispense (Official HL7 WG Mapping)
('RDS^O13',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Encounter", "MedicationDispense", "MedicationRequest", "Observation", "PractitionerRole"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,ORC,RXD", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1", "required": true, "priority": 2},
   "Encounter": {"from": "PV1,PV2", "condition": "visit_present", "priority": 3, "references": ["Patient"]},
   "MedicationDispense": {"from": "ORC,RXD", "required": true, "multiple": true, "priority": 4, "references": ["Patient"]},
   "MedicationRequest": {"from": "RXE", "condition": "original_order", "multiple": true, "priority": 5, "references": ["Patient"]},
   "Observation": {"from": "OBX", "condition": "observations_present", "multiple": true, "priority": 6},
   "PractitionerRole": {"from": "RXD", "condition": "dispenser_present", "multiple": true, "priority": 7}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Encounter": 3, "MedicationDispense": 4, "MedicationRequest": 5, "Observation": 6, "PractitionerRole": 7}'),

-- BAR^P01 - Add Patient Accounts (Official HL7 WG Mapping)
('BAR^P01',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Account", "Coverage", "RelatedPerson", "Organization"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,EVN,PID", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1", "required": true, "priority": 2},
   "Account": {"from": "ACC", "required": true, "multiple": true, "priority": 3, "references": ["Patient"]},
   "Coverage": {"from": "IN1,IN2,IN3", "condition": "insurance_present", "multiple": true, "priority": 4},
   "RelatedPerson": {"from": "NK1", "condition": "guarantor_present", "multiple": true, "priority": 5},
   "Organization": {"from": "GT1", "condition": "guarantor_org", "multiple": true, "priority": 6}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Account": 3, "Coverage": 4, "RelatedPerson": 5, "Organization": 6}'),

-- DFT^P03 - Post Detail Financial Transaction (Official HL7 WG Mapping)
('DFT^P03',
 '["Bundle", "MessageHeader", "Provenance", "Patient", "Encounter", "Account", "ChargeItem", "Invoice", "Coverage", "Procedure", "Observation"]',
 '{
   "Bundle": {"from": "MSH", "required": true, "priority": 1},
   "MessageHeader": {"from": "MSH", "required": true, "priority": 1},
   "Provenance": {"from": "MSH,EVN,FT1", "multiple": true, "priority": 1},
   "Patient": {"from": "PID,PD1", "required": true, "priority": 2},
   "Encounter": {"from": "PV1,PV2", "condition": "visit_present", "priority": 3, "references": ["Patient"]},
   "Account": {"from": "FT1", "condition": "account_present", "priority": 4},
   "ChargeItem": {"from": "FT1", "required": true, "multiple": true, "priority": 5, "references": ["Patient"]},
   "Invoice": {"from": "FT1", "condition": "invoice_present", "priority": 6},
   "Coverage": {"from": "IN1,IN2,IN3", "condition": "insurance_present", "multiple": true, "priority": 7},
   "Procedure": {"from": "PR1", "condition": "procedures_present", "multiple": true, "priority": 8},
   "Observation": {"from": "OBX", "condition": "observations_present", "multiple": true, "priority": 9}
 }',
 '{"Bundle": 1, "MessageHeader": 1, "Provenance": 1, "Patient": 2, "Encounter": 3, "Account": 4, "ChargeItem": 5, "Invoice": 6, "Coverage": 7, "Procedure": 8, "Observation": 9}');
INSERT INTO segment_resource_mappings (segment_name, fhir_resource_type, mapping_conditions, incorporation_rules, priority, multiple_instances) VALUES 

-- Core segment mappings
('MSH', 'Bundle', '{}', '{"creates_new": true}', 1, false),
('MSH', 'MessageHeader', '{}', '{"creates_new": true}', 1, false),
('MSH', 'Provenance', '{"condition": "source_tracking"}', '{"creates_new": true, "types": ["source", "transformation"]}', 1, true),

('PID', 'Patient', '{}', '{"creates_new": true}', 1, false),
('PID', 'Account', '{"condition": "account_needed"}', '{"creates_new": true}', 2, false),
('PID', 'Provenance', '{"condition": "PID.33_and_PID.34_valued"}', '{"creates_new": true, "type": "patient_update"}', 2, false),

('PD1', 'Patient', '{}', '{"incorporates_into": "Patient_from_PID"}', 1, false),
('PD1', 'Observation', '{"condition": "PD1.7_valued"}', '{"creates_new": true, "type": "living_will"}', 2, false),

('PV1', 'Encounter', '{}', '{"creates_new": true}', 1, false),
('PV1', 'Basic', '{"condition": "PV1.43_valued"}', '{"creates_new": true, "type": "encounter_history"}', 2, false),
('PV1', 'Patient', '{}', '{"incorporates_into": "Patient_from_PID"}', 3, false),
('PV1', 'Coverage', '{"condition": "PV1.20_valued"}', '{"creates_new": true}', 4, false),

('PV2', 'Encounter', '{}', '{"incorporates_into": "Encounter_from_PV1"}', 1, false),

('NK1', 'RelatedPerson', '{"condition": "NK1.3.1_not_in_emergency_contacts"}', '{"creates_new": true}', 1, true),
('NK1', 'Patient', '{}', '{"incorporates_into": "Patient_from_PID", "adds_contact": true}', 2, false),

('OBX', 'Observation', '{}', '{"creates_new": true}', 1, true),

('AL1', 'AllergyIntolerance', '{}', '{"creates_new": true}', 1, true),

('DG1', 'Condition', '{"context": "patient"}', '{"creates_new": true}', 1, true),
('DG1', 'Encounter', '{"context": "encounter"}', '{"incorporates_into": "Encounter_from_PV1", "adds_diagnosis": true}', 2, false),
('DG1', 'EpisodeOfCare', '{"context": "episode"}', '{"creates_new": true}', 3, true),

('PR1', 'Procedure', '{}', '{"creates_new": true}', 1, true),

('IN1', 'Coverage', '{}', '{"creates_new": true}', 1, true),
('IN2', 'Coverage', '{}', '{"incorporates_into": "Coverage_from_IN1"}', 1, false),
('IN3', 'Coverage', '{}', '{"incorporates_into": "Coverage_from_IN1"}', 1, false),

-- ORU-specific mappings
('ORC', 'DiagnosticReport', '{"message_type": "ORU"}', '{"creates_new": true}', 1, true),
('ORC', 'ServiceRequest', '{"message_type": "ORU", "condition": "service_request_needed"}', '{"creates_new": true}', 2, true),

('OBR', 'DiagnosticReport', '{"message_type": "ORU"}', '{"incorporates_into": "DiagnosticReport_from_ORC"}', 1, false),
('OBR', 'Specimen', '{"message_type": "ORU"}', '{"creates_new": true}', 2, true),
('OBR', 'ServiceRequest', '{"message_type": "ORU", "condition": "service_request_needed"}', '{"incorporates_into": "ServiceRequest_from_ORC"}', 3, false),

('SPM', 'Specimen', '{}', '{"creates_new": true}', 1, true),

-- ORM-specific mappings  
('ORC', 'ServiceRequest', '{"message_type": "ORM"}', '{"creates_new": true}', 1, true),
('ORC', 'Task', '{"message_type": "ORM", "condition": "task_needed"}', '{"creates_new": true}', 2, true),
('ORC', 'Provenance', '{"message_type": "ORM"}', '{"creates_new": true, "type": "order_tracking"}', 3, false),

('OBR', 'ServiceRequest', '{"message_type": "ORM", "condition": "PID_valued"}', '{"incorporates_into": "ServiceRequest_from_ORC"}', 1, false),
('OBR', 'SupplyRequest', '{"message_type": "ORM", "condition": "PID_not_valued"}', '{"creates_new": true}', 2, true),

('RXO', 'MedicationRequest', '{}', '{"creates_new": true}', 1, true),
('ODS', 'SupplyRequest', '{"condition": "PID_not_valued"}', '{"creates_new": true}', 1, true),

-- Participation segments
('PRT', 'PractitionerRole', '{"condition": "practitioner_role"}', '{"creates_new": true}', 1, true),
('PRT', 'RelatedPerson', '{"condition": "related_person_role"}', '{"creates_new": true}', 2, true),
('PRT', 'Device', '{"condition": "device_role"}', '{"creates_new": true}', 3, true);

INSERT INTO segment_resource_mappings (segment_name, fhir_resource_type, mapping_conditions, incorporation_rules, priority, multiple_instances) VALUES 

-- Event Type segment
('EVN', 'Provenance', '{}', '{"creates_new": true, "type": "event_tracking"}', 1, false),

-- Appointment segments
('SCH', 'Appointment', '{}', '{"creates_new": true}', 1, false),
('SCH', 'Schedule', '{"condition": "recurring_appointment"}', '{"creates_new": true}', 2, false),
('SCH', 'Slot', '{"condition": "slot_management"}', '{"creates_new": true}', 3, true),
('SCH', 'AppointmentResponse', '{"condition": "response_required"}', '{"creates_new": true}', 4, false),

('AIS', 'Appointment', '{}', '{"incorporates_into": "Appointment_from_SCH", "adds_service": true}', 1, false),
('AIG', 'Appointment', '{}', '{"incorporates_into": "Appointment_from_SCH", "adds_general_resource": true}', 1, false),
('AIL', 'Appointment', '{}', '{"incorporates_into": "Appointment_from_SCH", "adds_location": true}', 1, false),
('AIL', 'Location', '{}', '{"creates_new": true}', 2, true),
('AIP', 'Appointment', '{}', '{"incorporates_into": "Appointment_from_SCH", "adds_personnel": true}', 1, false),
('AIP', 'PractitionerRole', '{}', '{"creates_new": true}', 2, true),

-- Document segments
('TXA', 'DocumentReference', '{}', '{"creates_new": true}', 1, false),
('TXA', 'Composition', '{"condition": "document_composition"}', '{"creates_new": true}', 2, false),
('TXA', 'Provenance', '{}', '{"creates_new": true, "type": "document_authoring"}', 3, false),

-- Medication segments
('RXE', 'MedicationRequest', '{}', '{"creates_new": true}', 1, true),
('RXD', 'MedicationDispense', '{}', '{"creates_new": true}', 1, true),
('RXR', 'MedicationRequest', '{}', '{"incorporates_into": "MedicationRequest_from_RXE", "adds_route": true}', 1, false),
('RXA', 'Immunization', '{}', '{"creates_new": true}', 1, true),

-- Financial segments
('FT1', 'ChargeItem', '{}', '{"creates_new": true}', 1, true),
('FT1', 'Account', '{"condition": "account_reference"}', '{"references_existing": true}', 2, false),
('FT1', 'Invoice', '{"condition": "invoice_creation"}', '{"creates_new": true}', 3, false),

('ACC', 'Account', '{}', '{"creates_new": true}', 1, true),
('GT1', 'RelatedPerson', '{"condition": "guarantor_person"}', '{"creates_new": true, "type": "guarantor"}', 1, true),
('GT1', 'Organization', '{"condition": "guarantor_organization"}', '{"creates_new": true}', 2, true),

-- Location segments
('LOC', 'Location', '{}', '{"creates_new": true}', 1, true),
('LCH', 'Location', '{}', '{"incorporates_into": "Location_from_LOC", "adds_characteristics": true}', 1, false),
('LRL', 'Location', '{}', '{"incorporates_into": "Location_from_LOC", "adds_relationships": true}', 1, false),
('LDP', 'Location', '{}', '{"incorporates_into": "Location_from_LOC", "adds_department": true}', 1, false),

-- Resource segments
('AIG', 'Device', '{"condition": "general_resource_device"}', '{"creates_new": true}', 1, true),
('AIL', 'HealthcareService', '{"condition": "location_service"}', '{"creates_new": true}', 1, true);
