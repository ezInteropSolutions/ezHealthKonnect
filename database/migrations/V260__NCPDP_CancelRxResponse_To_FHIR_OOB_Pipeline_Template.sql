-- V260: NCPDP SCRIPT CancelRxResponse to FHIR OOB pipeline template
-- Applied: 2026-09-14

-- ============================================================
-- NCPDP SCRIPT CancelRxResponse -> FHIR (Task) TEMPLATE
-- ============================================================
-- FHIR target: Task, NOT MedicationRequest -- a real, deliberate design
-- correction from this feature's original plan draft. CancelRxResponse (per
-- ncpdp/schemas/script_2017071/transactions/CancelRxResponse.json) carries
-- ONLY requestReferenceNumber + one of 6 outcome choices (Approved/Denied/
-- DenyNewToFollow/ApprovedWithChanges/Validated/Replace, each with
-- reasonCode/referenceNumber/denialReason/note) + an optional
-- medicationPrescribed -- NO Patient/Prescriber/Pharmacy fields at all
-- (confirmed directly against the schema file). Base FHIR
-- MedicationRequest.subject is 1..1 required; fabricating a Patient
-- reference this message never carries would violate this project's own
-- no-invented-structure discipline. Task.for is 0..1 optional, so Task can
-- honestly represent "the response to a cancellation request, correlated by
-- its own business identifier" without inventing patient linkage.
--
-- Task.status is derived from WHICHEVER of the 6 outcome groups is present
-- (Approved/ApprovedWithChanges/Validated/Replace -> "completed";
-- Denied/DenyNewToFollow -> "rejected"); Task.businessStatus.coding[0].code
-- carries the raw outcome type name; Task.note[0].text sources whichever
-- branch's own Note field is actually present (fallbackPaths checked in
-- order, "first present value wins").
--
-- Config here is transcribed verbatim from
-- services/ncpdp_cancelrx_response_fhir_builder_test.go
-- (TestNCPDPCancelRxResponseFHIRBuilder_DeniedOutcome_BuildsCleanValidatingTask).

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
    'NCPDP SCRIPT CancelRxResponse to FHIR (SFTP)',
    'ncpdp-cancelrxresponse-to-fhir-sftp',
    'Poll an SFTP directory for NCPDP SCRIPT CancelRxResponse (pharmacy e-prescribing cancellation decision) XML files, parse and validate them, then build and validate a FHIR R4 Task resource representing the outcome (Approved/Denied/...).',
    'ncpdp',
    null,
    ARRAY['ncpdp','script','cancelrxresponse','pharmacy','e-prescribing','task','sftp','fhir'],
    '↩️',
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
                    "step_name": "Receive Cancel Rx Response File (SFTP)",
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
                    "step_name": "Parse Cancel Rx Response",
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
                    "step_name": "Build Cancel Rx Response Task",
                    "step_alias": "build_cancel_rx_response_task",
                    "step_type": "fhir.build",
                    "sequence": 100,
                    "config": {
                        "resourceType": "Task",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirTask",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.header.message_id", "transform": "string_prefix", "valueMap": {"prefix": "task-cancelrx-response-"}},
                            {"targetPath": "intent", "literalValue": "order"},
                            {"targetPath": "code.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/task-code"},
                            {"targetPath": "code.coding[0].code", "literalValue": "fulfill"},
                            {"targetPath": "description", "literalValue": "NCPDP SCRIPT CancelRx Response"},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-request-reference-number"},
                            {"targetPath": "identifier[0].value", "sourcePath": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.request_reference_number"},
                            {"targetPath": "businessStatus.coding[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-cancelrx-outcome"},
                            {"targetPath": "status", "literalValue": "rejected", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.denied", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "Denied", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.denied", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "rejected", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.deny_new_to_follow", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "DenyNewToFollow", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.deny_new_to_follow", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "completed", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.approved", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "Approved", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.approved", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "completed", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.approved_with_changes", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "ApprovedWithChanges", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.approved_with_changes", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "completed", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.validated", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "Validated", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.validated", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "completed", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.replace", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "Replace", "condition": {"field": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.replace", "operator": "exists"}},
                            {"targetPath": "note[0].text", "sourcePath": "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.denied.note", "fallbackPaths": [
                                "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.deny_new_to_follow.note",
                                "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.approved.note",
                                "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.approved_with_changes.note",
                                "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.validated.note",
                                "steps.parse_cancel_rx_response.step_output.parsed_ncpdp.body.response.replace.note"
                            ]}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 200,
                "steps": [{
                    "step_name": "Assemble Cancel Rx Response FHIR Bundle",
                    "step_alias": "assemble_cancel_rx_response_fhir_bundle",
                    "step_type": "payload.builder",
                    "sequence": 200,
                    "config": {
                        "mode": "fhir_bundle",
                        "fhirBundle": {
                            "bundleType": "collection",
                            "resourcePaths": ["message.fhirTask"]
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
    'CancelRxResponse',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_dir","label":"Remote Directory","type":"string","hint":"Directory to poll for CancelRxResponse XML files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive Cancel Rx Response (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse Cancel Rx Response","icon":"↩️","step_type":"ncpdp.parse"},
        {"name":"Validate","icon":"✅","step_type":"ncpdp.validate"},
        {"name":"Build Task","icon":"📋","step_type":"fhir.build"},
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
