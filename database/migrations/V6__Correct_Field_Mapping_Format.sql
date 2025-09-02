-- V6__Correct_Field_Mapping_Format.sql
-- Corrective migration to fix the incorrect field format from V5

-- Delete all incorrect field mappings from V5
DELETE FROM field_element_mappings WHERE hl7_field LIKE '%MSH.%' OR hl7_field LIKE '%PID.%' OR hl7_field LIKE '%PV1.%' OR hl7_field LIKE '%OBX.%';

-- Insert correct field mappings with proper format (field number only)
INSERT INTO field_element_mappings (segment_name, hl7_field, hl7_component, fhir_resource_type, fhir_element_path, data_type_transform, is_required, cardinality) VALUES

-- MSH Message Header mappings - CORRECTED FORMAT  
('MSH', '3', null, 'MessageHeader', 'source.name', null, false, '0..1'),
('MSH', '4', null, 'MessageHeader', 'source.endpoint', null, false, '0..1'),
('MSH', '5', null, 'MessageHeader', 'destination.name', null, false, '0..1'),
('MSH', '6', null, 'MessageHeader', 'destination.endpoint', null, false, '0..1'),
('MSH', '7', null, 'MessageHeader', 'timestamp', 'ts_to_datetime', true, '1..1'),
('MSH', '9', '2', 'MessageHeader', 'eventCoding', 'msh9_trigger_event_to_coding', true, '1..1'),
('MSH', '10', null, 'MessageHeader', 'focus', 'control_id_to_reference', false, '0..*'),

-- PID Patient Identification - CORRECTED FORMAT with components
('PID', '3', null, 'Patient', 'identifier', 'cx_to_identifier', true, '1..*'),
('PID', '5', null, 'Patient', 'name', 'xpn_to_humanname', true, '0..*'),
('PID', '5', '1', 'Patient', 'name.family', null, false, '0..1'),
('PID', '5', '2', 'Patient', 'name.given', null, false, '0..*'),
('PID', '5', '3', 'Patient', 'name.given', null, false, '0..*'),
('PID', '7', null, 'Patient', 'birthDate', 'ts_to_date', false, '0..1'),
('PID', '8', null, 'Patient', 'gender', 'gender_mapping', false, '0..1'),
('PID', '11', null, 'Patient', 'address', 'xad_to_address', false, '0..*'),
('PID', '13', null, 'Patient', 'telecom', 'xtn_to_contactpoint', false, '0..*'),
('PID', '14', null, 'Patient', 'telecom', 'xtn_to_contactpoint', false, '0..*'),
('PID', '16', null, 'Patient', 'maritalStatus', 'ce_to_codeableconcept', false, '0..1'),
('PID', '18', null, 'Patient', 'identifier', 'account_to_identifier', false, '0..*'),

-- PV1 Patient Visit - CORRECTED FORMAT
('PV1', '2', null, 'Encounter', 'class', 'ce_to_codeableconcept', false, '1..1'),
('PV1', '3', null, 'Encounter', 'location.location', null, false, '0..1'),
('PV1', '19', null, 'Encounter', 'identifier', 'cx_to_identifier', false, '0..*'),
('PV1', '44', null, 'Encounter', 'period.start', 'ts_to_datetime', false, '0..1'),
('PV1', '45', null, 'Encounter', 'period.end', 'ts_to_datetime', false, '0..1'),

-- OBX Observation - CORRECTED FORMAT
('OBX', '3', null, 'Observation', 'code', 'ce_to_codeableconcept', true, '1..1'),
('OBX', '5', null, 'Observation', 'valueString', null, false, '0..1'),
('OBX', '6', null, 'Observation', 'valueQuantity.unit', null, false, '0..1'),
('OBX', '11', null, 'Observation', 'status', 'observation_status', false, '1..1'),
('OBX', '14', null, 'Observation', 'effectiveDateTime', 'ts_to_datetime', false, '0..1');

-- Fix value set mappings - replace generic with specific names the code expects
DELETE FROM value_set_mappings WHERE mapping_name = 'gender';

INSERT INTO value_set_mappings (mapping_name, hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, mapping_type) VALUES
-- Administrative Sex - correct mapping name for the service code
('administrative_sex', '0001', 'M', 'http://hl7.org/fhir/administrative-gender', 'male', 'Male', 'direct'),
('administrative_sex', '0001', 'F', 'http://hl7.org/fhir/administrative-gender', 'female', 'Female', 'direct'),
('administrative_sex', '0001', 'O', 'http://hl7.org/fhir/administrative-gender', 'other', 'Other', 'direct'),
('administrative_sex', '0001', 'U', 'http://hl7.org/fhir/administrative-gender', 'unknown', 'Unknown', 'direct'),

-- Event Types for MSH.9.2
('v2-0003', '0003', 'A01', 'http://terminology.hl7.org/CodeSystem/v2-0003', 'A01', 'Admit/visit notification', 'direct'),
('v2-0003', '0003', 'A04', 'http://terminology.hl7.org/CodeSystem/v2-0003', 'A04', 'Register a patient', 'direct'),
('v2-0003', '0003', 'A08', 'http://terminology.hl7.org/CodeSystem/v2-0003', 'A08', 'Update patient information', 'direct'),

-- XTN Component mappings for phone processing
('xtn_component_use', 'XTN', '1', 'home', '', 'Component 1 default use', 'positional'),
('xtn_component_use', 'XTN', '2', 'work', '', 'Component 2 default use', 'positional')

ON CONFLICT DO NOTHING;