-- V267: NCPDP Telecommunication D.0 B1 (Claim Billing Response) to FHIR OOB pipeline template
-- Applied: 2026-09-19

-- ============================================================
-- NCPDP Telecom D.0 B1 Response -> FHIR (ClaimResponse)
-- TEMPLATE
-- ============================================================
-- FHIR target: ClaimResponse, one per GS-delimited transaction group. A B1
-- response carries NO patient/pharmacy identity data at all (confirmed
-- directly against the schema -- ResponseMessage/ResponseStatus/
-- ResponseClaim/ResponsePricing carry none), the same real gap that led
-- NCPDP SCRIPT's CancelRxResponse/RxChangeResponse to use Task instead of
-- MedicationRequest. ClaimResponse.patient is populated with a
-- DISPLAY-ONLY Reference (no `.reference`, since no Patient resource
-- exists in this response-only Bundle to point at) -- the same logical-
-- reference pattern already used for Claim.careTeam[].provider in the EDI
-- 837 work, satisfying FHIR's structural Reference-object requirement
-- without fabricating a resource or writing a dangling pointer.
--
-- HCR/AN status code A/C/P (Approved/Captured/Paid) -> outcome "complete";
-- D/E/R (Duplicate/Error/Rejected) -> outcome "error" -- a code-system
-- translation sourced directly from response.md's own confirmed
-- vocabulary, never an invented fact.
--
-- Config here is transcribed verbatim (generated programmatically from the
-- Go source, never hand-retyped) from
-- services/ncpdp_telecom_b1_response_fhir_builder_test.go
-- (TestNCPDPTelecomB1ResponseFHIRBuilder_ApprovedOutcome_BuildsCleanValidatingBundle).

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
    'NCPDP Telecom D.0 B1 Claim Response to FHIR (SFTP)',
    'ncpdp-telecom-b1-response-to-fhir-sftp',
    'Poll an SFTP directory for NCPDP Telecommunication D.0 B1 (Claim Billing) response/adjudication files, parse and validate them against the D.0 schema, then build and validate a FHIR R4 Bundle containing one ClaimResponse resource per claim adjudication result.',
    'ncpdp',
    null,
    ARRAY['ncpdp','telecom','d0','b1','pharmacy','claimresponse','adjudication','sftp','fhir'],
    '🧾',
    'advanced',
    'sftp_inbound',
    '{"remote_dir": "/incoming", "file_pattern": "*.txt", "poll_interval_sec": 60, "after_processing": "archive"}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    $pipeline$
{
    "execution_groups": [
        {
            "sequence": 5,
            "steps": [
                {
                    "step_name": "Receive B1 Response File (SFTP)",
                    "step_type": "connector.inbound",
                    "sequence": 5,
                    "config": {
                        "connectorType": "sftp_inbound",
                        "config": {
                            "remote_dir": "/incoming",
                            "file_pattern": "*.txt",
                            "poll_interval_sec": 60,
                            "after_processing": "archive"
                        }
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 20,
            "steps": [
                {
                    "step_name": "Parse B1 Response",
                    "step_type": "ncpdptelecom.parse",
                    "sequence": 20,
                    "config": {
                        "sourceField": "raw",
                        "direction": "response",
                        "outputField": "parsedTelecom"
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 30,
            "steps": [
                {
                    "step_name": "Validate Against NCPDP Telecom D.0 Schema",
                    "step_type": "ncpdptelecom.validate",
                    "sequence": 30,
                    "config": {
                        "sourceField": "parsedTelecom",
                        "outputField": "telecomValidation"
                    },
                    "enabled": true,
                    "required": false
                }
            ]
        },
        {
            "sequence": 90,
            "steps": [
                {
                    "step_name": "Derive B1 Response Context",
                    "step_alias": "derive_b1_response_context",
                    "step_type": "enrichment.script",
                    "sequence": 90,
                    "description": "Flattens each transaction group's own Response Status/Response Pricing content into one flat row per claim adjudication result.",
                    "config": {
                        "script": "\n// See deriveB1ClaimContextScript's own doc comment for why this reads\n// steps.<alias>.step_output (already recursively snake_cased by\n// NormalizeStepOutput) rather than a plain \"message.parsedTelecom\" field.\nvar stepOut = (input.steps && input.steps.parse_b1_response && input.steps.parse_b1_response.step_output) || {};\nvar parsed = stepOut.parsed_telecom || {};\nvar groups = parsed.transaction_groups || [];\nvar response_rows = [];\nfor (var i = 0; i < groups.length; i++) {\n    var g = groups[i] || {};\n    var status = g.response_status || {};\n    var pricing = g.response_pricing || {};\n    response_rows.push({\n        response_status: status.response_status,\n        authorization_number: status.authorization_number,\n        reject_code: status.reject_code,\n        transaction_reference_number: status.transaction_reference_number,\n        total_amount_paid: pricing.total_amount_paid\n    });\n}\n({ response_rows: response_rows });\n"
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 110,
            "steps": [
                {
                    "step_name": "Build Claim Response",
                    "step_alias": "build_claim_response_fhir",
                    "step_type": "fhir.build",
                    "sequence": 110,
                    "config": {
                        "resourceType": "ClaimResponse",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirClaimResponses",
                        "rowsPath": "steps.derive_b1_response_context.step_output.response_rows",
                        "fields": [
                            {
                                "targetPath": "id",
                                "sourcePath": "_rowIndex",
                                "transform": "string_prefix",
                                "valueMap": {
                                    "prefix": "claimresponse-"
                                }
                            },
                            {
                                "targetPath": "status",
                                "literalValue": "active"
                            },
                            {
                                "targetPath": "type.coding[0].system",
                                "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"
                            },
                            {
                                "targetPath": "type.coding[0].code",
                                "literalValue": "pharmacy"
                            },
                            {
                                "targetPath": "use",
                                "literalValue": "claim"
                            },
                            {
                                "targetPath": "patient.display",
                                "literalValue": "Patient information not included in claim status response"
                            },
                            {
                                "targetPath": "created",
                                "literalValue": "2026-09-19"
                            },
                            {
                                "targetPath": "insurer.display",
                                "literalValue": "Payer"
                            },
                            {
                                "targetPath": "identifier[0].system",
                                "literalValue": "http://ezhealthkonnect.local/ncpdp-telecom-transaction-reference-number"
                            },
                            {
                                "targetPath": "identifier[0].value",
                                "sourcePath": "transaction_reference_number"
                            },
                            {
                                "targetPath": "disposition",
                                "sourcePath": "authorization_number",
                                "fallbackPaths": [
                                    "reject_code"
                                ]
                            },
                            {
                                "targetPath": "outcome",
                                "literalValue": "complete",
                                "condition": {
                                    "field": "response_status",
                                    "operator": "equals",
                                    "value": "A"
                                }
                            },
                            {
                                "targetPath": "outcome",
                                "literalValue": "complete",
                                "condition": {
                                    "field": "response_status",
                                    "operator": "equals",
                                    "value": "C"
                                }
                            },
                            {
                                "targetPath": "outcome",
                                "literalValue": "complete",
                                "condition": {
                                    "field": "response_status",
                                    "operator": "equals",
                                    "value": "P"
                                }
                            },
                            {
                                "targetPath": "outcome",
                                "literalValue": "error",
                                "condition": {
                                    "field": "response_status",
                                    "operator": "equals",
                                    "value": "D"
                                }
                            },
                            {
                                "targetPath": "outcome",
                                "literalValue": "error",
                                "condition": {
                                    "field": "response_status",
                                    "operator": "equals",
                                    "value": "E"
                                }
                            },
                            {
                                "targetPath": "outcome",
                                "literalValue": "error",
                                "condition": {
                                    "field": "response_status",
                                    "operator": "equals",
                                    "value": "R"
                                }
                            }
                        ]
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 200,
            "steps": [
                {
                    "step_name": "Assemble B1 Response FHIR Bundle",
                    "step_alias": "assemble_b1_response_fhir_bundle",
                    "step_type": "payload.builder",
                    "sequence": 200,
                    "config": {
                        "mode": "fhir_bundle",
                        "fhirBundle": {
                            "bundleType": "collection",
                            "resourcePaths": [
                                "message.fhirClaimResponses"
                            ]
                        }
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 210,
            "steps": [
                {
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
                }
            ]
        },
        {
            "sequence": 295,
            "steps": [
                {
                    "step_name": "Store Result",
                    "step_type": "connector.outbound",
                    "sequence": 295,
                    "config": {
                        "connectorType": "sink_outbound",
                        "config": {
                            "enable_logging": true,
                            "enable_validation": true
                        },
                        "contentField": "payload"
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        }
    ]
}
    $pipeline$::jsonb,
    'B1_RESPONSE',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_dir","label":"Remote Directory","type":"string","hint":"Directory to poll for B1 response files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive B1 Response (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse B1 Response","icon":"🧾","step_type":"ncpdptelecom.parse"},
        {"name":"Validate","icon":"✅","step_type":"ncpdptelecom.validate"},
        {"name":"Derive Response Context","icon":"🔧","step_type":"enrichment.script"},
        {"name":"Build ClaimResponse","icon":"🧾","step_type":"fhir.build"},
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
