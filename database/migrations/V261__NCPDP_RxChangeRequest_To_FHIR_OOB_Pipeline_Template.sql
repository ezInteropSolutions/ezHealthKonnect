-- V261: NCPDP SCRIPT RxChangeRequest to FHIR OOB pipeline template
-- Applied: 2026-09-19

-- ============================================================
-- NCPDP SCRIPT RxChangeRequest -> FHIR (MedicationRequest + Patient +
-- Practitioner + Organization) TEMPLATE
-- ============================================================
-- RxChangeRequest carries the SAME body shape as NewRx/CancelRx (Patient/
-- Pharmacy/Prescriber/MedicationPrescribed), so this mirrors V259's own
-- Organization/Patient/Practitioner/MedicationRequest shape almost exactly.
-- The one real semantic difference: MedicationRequest.status="draft" +
-- intent="proposal" (both real base-FHIR MedicationRequest enum values,
-- chosen deliberately -- a pharmacy's PROPOSED change is not yet
-- prescriber-approved, unlike NewRx's own status="active"/intent="order" or
-- CancelRx's status="cancelled") -- RxChangeResponse's own outcome then
-- represents the prescriber's actual decision (see V262).
--
-- Config here is transcribed verbatim from
-- services/ncpdp_rxchangerequest_fhir_builder_test.go
-- (TestNCPDPRxChangeRequestFHIRBuilder_SelfAuthoredSample_BuildsCleanValidatingBundle).

INSERT INTO interface_templates
    (id, name, slug, description, category, subcategory, tags, icon, difficulty,
     source_connector_type, source_config_template,
     target_connector_type, target_config_template,
     pipeline_config, message_type,
     required_connection_fields, sanitized_fields, preview_steps,
     estimated_setup_minutes, is_system, is_public, author, usage_count)
VALUES
(
    gen_random_uuid(),
    'NCPDP SCRIPT RxChangeRequest to FHIR (SFTP)',
    'ncpdp-rxchangerequest-to-fhir-sftp',
    'Poll an SFTP directory for NCPDP SCRIPT RxChangeRequest (pharmacy-proposed prescription change request) XML files, parse and validate them, then build and validate a FHIR R4 Bundle containing Organization (pharmacy), Patient, Practitioner (prescriber), and MedicationRequest (status=draft, intent=proposal) resources.',
    'ncpdp',
    null,
    ARRAY['ncpdp','script','rxchangerequest','pharmacy','e-prescribing','medicationrequest','sftp','fhir'],
    '🔄',
    'advanced',
    'sftp_inbound',
    '{"remote_dir": "/incoming", "file_pattern": "*.xml", "poll_interval_sec": 300, "after_processing": "archive"}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    $pipeline$
    {
        "execution_groups": [
            {
                "sequence": 5,
                "steps": [{
                    "step_name": "Receive Rx Change Request File (SFTP)",
                    "step_type": "connector.inbound",
                    "sequence": 5,
                    "config": {
                        "connectorType": "sftp_inbound",
                        "config": {"remote_dir": "/incoming", "file_pattern": "*.xml", "poll_interval_sec": 300, "after_processing": "archive"}
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 20,
                "steps": [{
                    "step_name": "Parse Rx Change Request",
                    "step_type": "ncpdp.parse",
                    "sequence": 20,
                    "config": {"sourceField": "raw", "outputField": "parsedNCPDP"},
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 30,
                "steps": [{
                    "step_name": "Validate Against NCPDP SCRIPT Schema",
                    "step_type": "ncpdp.validate",
                    "sequence": 30,
                    "config": {"sourceField": "parsedNCPDP", "outputField": "ncpdpValidation"},
                    "enabled": true,
                    "required": false
                }]
            },
            {
                "sequence": 100,
                "steps": [{
                    "step_name": "Build Pharmacy Organization",
                    "step_alias": "build_pharmacy_organization",
                    "step_type": "fhir.build",
                    "sequence": 100,
                    "config": {
                        "resourceType": "Organization",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirPharmacyOrganization",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.pharmacy.identification.ncpdpid", "transform": "string_prefix", "valueMap": {"prefix": "organization-pharmacy-"}},
                            {"targetPath": "active", "literalValue": "true"},
                            {"targetPath": "name", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.pharmacy.business_name"},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-provider-id"},
                            {"targetPath": "identifier[0].value", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.pharmacy.identification.ncpdpid"}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 105,
                "steps": [{
                    "step_name": "Build Patient",
                    "step_alias": "build_patient",
                    "step_type": "fhir.build",
                    "sequence": 105,
                    "config": {
                        "resourceType": "Patient",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirPatient",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.header.message_id", "transform": "string_prefix", "valueMap": {"prefix": "patient-"}},
                            {"targetPath": "name[0].family", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.patient.human_patient.name.last_name"},
                            {"targetPath": "name[0].given[0]", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.patient.human_patient.name.first_name"},
                            {"targetPath": "gender", "literalValue": "male", "condition": {"field": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.patient.human_patient.gender", "operator": "equals", "value": "M"}},
                            {"targetPath": "gender", "literalValue": "female", "condition": {"field": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.patient.human_patient.gender", "operator": "equals", "value": "F"}},
                            {"targetPath": "gender", "literalValue": "unknown", "condition": {"field": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.patient.human_patient.gender", "operator": "equals", "value": "U"}}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 110,
                "steps": [{
                    "step_name": "Build Prescriber Practitioner",
                    "step_alias": "build_prescriber_practitioner",
                    "step_type": "fhir.build",
                    "sequence": 110,
                    "config": {
                        "resourceType": "Practitioner",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirPractitioner",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.identification.npi", "transform": "string_prefix", "valueMap": {"prefix": "practitioner-"}},
                            {"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
                            {"targetPath": "identifier[0].value", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.identification.npi"},
                            {"targetPath": "name[0].family", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.name.last_name"},
                            {"targetPath": "name[0].given[0]", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.name.first_name"}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 115,
                "steps": [{
                    "step_name": "Build Medication Request",
                    "step_alias": "build_medication_request",
                    "step_type": "fhir.build",
                    "sequence": 115,
                    "config": {
                        "resourceType": "MedicationRequest",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirMedicationRequest",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.header.message_id", "transform": "string_prefix", "valueMap": {"prefix": "medicationrequest-"}},
                            {"targetPath": "status", "literalValue": "draft"},
                            {"targetPath": "intent", "literalValue": "proposal"},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-request-reference-number"},
                            {"targetPath": "identifier[0].value", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.request_reference_number"},
                            {"targetPath": "medicationCodeableConcept.text", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.medication_prescribed.drug_description"},
                            {"targetPath": "subject.reference", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.header.message_id", "transform": "string_prefix", "valueMap": {"prefix": "Patient/patient-"}},
                            {"targetPath": "requester.reference", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.prescriber.non_veterinarian.identification.npi", "transform": "string_prefix", "valueMap": {"prefix": "Practitioner/practitioner-"}},
                            {"targetPath": "dispenseRequest.performer.reference", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.pharmacy.identification.ncpdpid", "transform": "string_prefix", "valueMap": {"prefix": "Organization/organization-pharmacy-"}},
                            {"targetPath": "dispenseRequest.quantity.value", "sourcePath": "steps.parse_rx_change_request.step_output.parsed_ncpdp.body.medication_prescribed.quantity.value", "transform": "cda_decimal_string_to_number"}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 200,
                "steps": [{
                    "step_name": "Assemble Rx Change Request FHIR Bundle",
                    "step_alias": "assemble_rx_change_request_fhir_bundle",
                    "step_type": "payload.builder",
                    "sequence": 200,
                    "config": {
                        "mode": "fhir_bundle",
                        "fhirBundle": {
                            "bundleType": "collection",
                            "resourcePaths": ["message.fhirPharmacyOrganization", "message.fhirPatient", "message.fhirPractitioner", "message.fhirMedicationRequest"]
                        }
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 210,
                "steps": [{
                    "step_name": "Validate FHIR Bundle",
                    "step_type": "fhir_validation",
                    "sequence": 210,
                    "config": {
                        "validation_level": "strict",
                        "profile": "base",
                        "fhir_version": "R4",
                        "source_field": "payload"
                    },
                    "enabled": true,
                    "required": false
                }]
            },
            {
                "sequence": 295,
                "steps": [{
                    "step_name": "Store Result",
                    "step_type": "connector.outbound",
                    "sequence": 295,
                    "config": {
                        "connectorType": "sink_outbound",
                        "config": {"enable_logging": true, "enable_validation": true},
                        "contentField": "payload"
                    },
                    "enabled": true,
                    "required": true
                }]
            }
        ]
    }
    $pipeline$::jsonb,
    'RxChangeRequest',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_dir","label":"Remote Directory","type":"string","hint":"Directory to poll for RxChangeRequest XML files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive Rx Change Request (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse Rx Change Request","icon":"🔄","step_type":"ncpdp.parse"},
        {"name":"Validate","icon":"✅","step_type":"ncpdp.validate"},
        {"name":"Build Organization","icon":"🏥","step_type":"fhir.build"},
        {"name":"Build Patient","icon":"🧑","step_type":"fhir.build"},
        {"name":"Build Practitioner","icon":"👨‍⚕️","step_type":"fhir.build"},
        {"name":"Build MedicationRequest","icon":"💊","step_type":"fhir.build"},
        {"name":"Assemble FHIR Bundle","icon":"📦","step_type":"payload.builder"},
        {"name":"Validate FHIR","icon":"🛡️","step_type":"fhir_validation"},
        {"name":"Store Result","icon":"💾","step_type":"connector.outbound"}
    ]',
    15,
    true,
    true,
    'ezHealthKonnect',
    0
)
ON CONFLICT (slug) DO UPDATE SET
    name                   = EXCLUDED.name,
    description            = EXCLUDED.description,
    pipeline_config        = EXCLUDED.pipeline_config,
    preview_steps          = EXCLUDED.preview_steps,
    tags                   = EXCLUDED.tags,
    updated_at             = NOW();
