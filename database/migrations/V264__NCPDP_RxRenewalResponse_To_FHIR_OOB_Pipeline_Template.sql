-- V264: NCPDP SCRIPT RxRenewalResponse to FHIR OOB pipeline template
-- Applied: 2026-09-19

-- ============================================================
-- NCPDP SCRIPT RxRenewalResponse -> FHIR (Task) TEMPLATE
-- ============================================================
-- FHIR target: Task, same reasoning as CancelRxResponse/RxChangeResponse
-- (see V260's own doc comment) -- the schema carries no Patient/Prescriber
-- identification for response messages, so a base-FHIR-required
-- MedicationRequest.subject can't be honestly populated. Cross-validated
-- against cosyte/ncpdp's own LifecycleResponseFields, which
-- RxRenewalResponse extends with ZERO additional fields of its own -- the
-- same 6 outcome elements, same fail-safe denial-first precedence, shared
-- verbatim across all 3 lifecycle response kinds.
--
-- Unlike CancelRxResponse (but like RxChangeResponse), RxRenewalResponse's
-- own optional medicationPrescribed represents real content worth carrying
-- forward on an ApprovedWithChanges outcome -- Task.description sources it
-- when present, falling back to a generic label.
--
-- Config here is transcribed verbatim from
-- services/ncpdp_rxrenewalresponse_fhir_builder_test.go
-- (TestNCPDPRxRenewalResponseFHIRBuilder_ApprovedOutcome_BuildsCleanValidatingTask).

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
    'NCPDP SCRIPT RxRenewalResponse to FHIR (SFTP)',
    'ncpdp-rxrenewalresponse-to-fhir-sftp',
    'Poll an SFTP directory for NCPDP SCRIPT RxRenewalResponse (pharmacy e-prescribing renewal decision) XML files, parse and validate them, then build and validate a FHIR R4 Task resource representing the prescriber''s outcome (Approved/Denied/DenyNewToFollow/...).',
    'ncpdp',
    null,
    ARRAY['ncpdp','script','rxrenewalresponse','pharmacy','e-prescribing','task','sftp','fhir','refill'],
    '📨',
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
                    "step_name": "Receive Rx Renewal Response File (SFTP)",
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
                    "step_name": "Parse Rx Renewal Response",
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
                    "step_name": "Build Rx Renewal Response Task",
                    "step_alias": "build_rx_renewal_response_task",
                    "step_type": "fhir.build",
                    "sequence": 100,
                    "config": {
                        "resourceType": "Task",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirTask",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.header.message_id", "transform": "string_prefix", "valueMap": {"prefix": "task-rxrenewalresponse-"}},
                            {"targetPath": "intent", "literalValue": "order"},
                            {"targetPath": "code.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/task-code"},
                            {"targetPath": "code.coding[0].code", "literalValue": "fulfill"},
                            {"targetPath": "description", "sourcePath": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.medication_prescribed.drug_description", "literalValue": "NCPDP SCRIPT RxRenewalRequest Response"},
                            {"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-request-reference-number"},
                            {"targetPath": "identifier[0].value", "sourcePath": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.request_reference_number"},
                            {"targetPath": "businessStatus.coding[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-rxrenewalresponse-outcome"},
                            {"targetPath": "status", "literalValue": "rejected", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.denied", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "Denied", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.denied", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "rejected", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.deny_new_to_follow", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "DenyNewToFollow", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.deny_new_to_follow", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "completed", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.approved", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "Approved", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.approved", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "completed", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.approved_with_changes", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "ApprovedWithChanges", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.approved_with_changes", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "completed", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.validated", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "Validated", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.validated", "operator": "exists"}},
                            {"targetPath": "status", "literalValue": "completed", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.replace", "operator": "exists"}},
                            {"targetPath": "businessStatus.coding[0].code", "literalValue": "Replace", "condition": {"field": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.replace", "operator": "exists"}},
                            {"targetPath": "note[0].text", "sourcePath": "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.denied.note", "fallbackPaths": [
                                "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.deny_new_to_follow.note",
                                "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.approved.note",
                                "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.approved_with_changes.note",
                                "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.validated.note",
                                "steps.parse_rx_renewal_response.step_output.parsed_ncpdp.body.response.replace.note"
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
                    "step_name": "Assemble Rx Renewal Response FHIR Bundle",
                    "step_alias": "assemble_rx_renewal_response_fhir_bundle",
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
    'RxRenewalResponse',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_dir","label":"Remote Directory","type":"string","hint":"Directory to poll for RxRenewalResponse XML files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive Rx Renewal Response (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse Rx Renewal Response","icon":"📨","step_type":"ncpdp.parse"},
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
