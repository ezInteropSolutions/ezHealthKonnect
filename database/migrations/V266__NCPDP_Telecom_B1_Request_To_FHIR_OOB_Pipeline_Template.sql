-- V266: NCPDP Telecommunication D.0 B1 (Claim Billing Request) to FHIR OOB pipeline template
-- Applied: 2026-09-19

-- ============================================================
-- NCPDP Telecom D.0 B1 Request -> FHIR (Organization + Patient + Claim)
-- TEMPLATE
-- ============================================================
-- FHIR target: Claim (type=pharmacy), one per GS-delimited transaction group
-- (one per drug/line item being billed), plus Organization (pharmacy) and
-- Patient built once per transmission. See
-- ncpdptelecom/schemas/telecom_d0's own sourceRefs for wire-format
-- provenance (eduardonunesp/ncpdp-telecom-fmt-book + apiv/dzero, both
-- license:null -- structural facts only, no vendored code/fixtures).
--
-- fhir.build's rowsPath mode resolves fields ROW-ONLY (confirmed directly
-- in fhir_build_executor.go), so a small enrichment.script derive step
-- copies the one header-level value every claim row needs (dateOfService)
-- down onto each row before fhir.build ever sees it -- the same real
-- mechanism gap EDI 835's own "Derive 835 Claim Context" step exists to
-- solve, not a design invented fresh for D.0.
--
-- Organization/Patient use FIXED literal ids ("organization-pharmacy-1"/
-- "patient-1") rather than deriving them from per-row data -- B1 carries
-- exactly ONE pharmacy and ONE patient per transmission (unlike 837's own
-- subscriber/dependent multiplicity), so uniqueness WITHIN one message's
-- own Bundle is all that matters.
--
-- Config here is transcribed verbatim (generated programmatically from the
-- Go source, never hand-retyped) from
-- services/ncpdp_telecom_b1_request_fhir_builder_test.go
-- (TestNCPDPTelecomB1RequestFHIRBuilder_SelfAuthoredSample_BuildsCleanValidatingBundle).

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
    'NCPDP Telecom D.0 B1 Claim Request to FHIR (SFTP)',
    'ncpdp-telecom-b1-request-to-fhir-sftp',
    'Poll an SFTP directory for NCPDP Telecommunication D.0 B1 (Claim Billing) request files, parse and validate them against the D.0 schema, then build and validate a FHIR R4 Bundle containing Organization (pharmacy), Patient, and Claim (type=pharmacy, one per drug/line item) resources.',
    'ncpdp',
    null,
    ARRAY['ncpdp','telecom','d0','b1','pharmacy','claim','sftp','fhir'],
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
                    "step_name": "Receive B1 Claim File (SFTP)",
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
                    "step_name": "Parse B1 Request",
                    "step_type": "ncpdptelecom.parse",
                    "sequence": 20,
                    "config": {
                        "sourceField": "raw",
                        "direction": "request",
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
                    "step_name": "Derive B1 Claim Context",
                    "step_alias": "derive_b1_claim_context",
                    "step_type": "enrichment.script",
                    "sequence": 90,
                    "description": "Copies header.dateOfService down onto each transaction group's own flattened claim row -- fhir.build's rowsPath field resolution is row-scoped only, so a claim row has no reachable path back up to the transmission header without this copy-down step.",
                    "config": {
                        "script": "\n// The prior ncpdptelecom.parse step's own step_output snapshot is ALWAYS\n// reachable via steps.<alias>.step_output (never a plain top-level/\n// message-nested field the way a REAL pipeline's execution context would\n// additionally expose it) -- confirmed directly by how this test's own\n// data fixture is constructed (svcInjectStepOutput only, matching the\n// NewRx/CancelRx/RxChangeRequest precedent). That snapshot is ALWAYS\n// passed through NormalizeStepOutput first, which recursively snake_cases\n// every key at every depth -- so \"transactionGroups[i].Claim\" becomes\n// \"transaction_groups[i].claim\", not just the top level.\nvar stepOut = (input.steps && input.steps.parse_b1_request && input.steps.parse_b1_request.step_output) || {};\nvar parsed = stepOut.parsed_telecom || {};\nvar header = parsed.header || {};\nvar groups = parsed.transaction_groups || [];\nvar claim_rows = [];\nfor (var i = 0; i < groups.length; i++) {\n    var g = groups[i] || {};\n    var claim = g.claim || {};\n    var pricing = g.pricing || {};\n    claim_rows.push({\n        prescription_reference_number: claim.prescription_reference_number,\n        product_service_id: claim.product_service_id,\n        quantity_dispensed: claim.quantity_dispensed,\n        gross_amount_due: pricing.gross_amount_due,\n        date_of_service: header.date_of_service\n    });\n}\n({ claim_rows: claim_rows });\n"
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 100,
            "steps": [
                {
                    "step_name": "Build Pharmacy Organization",
                    "step_alias": "build_pharmacy_organization_fhir",
                    "step_type": "fhir.build",
                    "sequence": 100,
                    "config": {
                        "resourceType": "Organization",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirPharmacyOrganization",
                        "fields": [
                            {
                                "targetPath": "id",
                                "literalValue": "organization-pharmacy-1"
                            },
                            {
                                "targetPath": "active",
                                "literalValue": "true"
                            },
                            {
                                "targetPath": "name",
                                "literalValue": "Pharmacy"
                            },
                            {
                                "targetPath": "identifier[0].system",
                                "literalValue": "http://ezhealthkonnect.local/ncpdp-telecom-provider-id"
                            },
                            {
                                "targetPath": "identifier[0].value",
                                "sourcePath": "steps.parse_b1_request.step_output.parsed_telecom.transmission_group.pharmacy_provider.provider_id"
                            }
                        ]
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 105,
            "steps": [
                {
                    "step_name": "Build Patient",
                    "step_alias": "build_patient_fhir",
                    "step_type": "fhir.build",
                    "sequence": 105,
                    "config": {
                        "resourceType": "Patient",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirPatient",
                        "fields": [
                            {
                                "targetPath": "id",
                                "literalValue": "patient-1"
                            },
                            {
                                "targetPath": "identifier[0].system",
                                "literalValue": "http://ezhealthkonnect.local/ncpdp-telecom-cardholder-id"
                            },
                            {
                                "targetPath": "identifier[0].value",
                                "sourcePath": "steps.parse_b1_request.step_output.parsed_telecom.transmission_group.insurance.cardholder_id"
                            },
                            {
                                "targetPath": "name[0].family",
                                "sourcePath": "steps.parse_b1_request.step_output.parsed_telecom.transmission_group.patient.patient_last_name"
                            },
                            {
                                "targetPath": "birthDate",
                                "sourcePath": "steps.parse_b1_request.step_output.parsed_telecom.transmission_group.patient.date_of_birth",
                                "transform": "x12_date_to_fhir_date"
                            },
                            {
                                "targetPath": "gender",
                                "literalValue": "male",
                                "condition": {
                                    "field": "steps.parse_b1_request.step_output.parsed_telecom.transmission_group.patient.patient_gender_code",
                                    "operator": "equals",
                                    "value": "1"
                                }
                            },
                            {
                                "targetPath": "gender",
                                "literalValue": "female",
                                "condition": {
                                    "field": "steps.parse_b1_request.step_output.parsed_telecom.transmission_group.patient.patient_gender_code",
                                    "operator": "equals",
                                    "value": "2"
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
            "sequence": 110,
            "steps": [
                {
                    "step_name": "Build Claim",
                    "step_alias": "build_claim_fhir",
                    "step_type": "fhir.build",
                    "sequence": 110,
                    "config": {
                        "resourceType": "Claim",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.fhirClaims",
                        "rowsPath": "steps.derive_b1_claim_context.step_output.claim_rows",
                        "fields": [
                            {
                                "targetPath": "id",
                                "sourcePath": "_rowIndex",
                                "transform": "string_prefix",
                                "valueMap": {
                                    "prefix": "claim-"
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
                                "targetPath": "patient.reference",
                                "literalValue": "Patient/patient-1"
                            },
                            {
                                "targetPath": "provider.reference",
                                "literalValue": "Organization/organization-pharmacy-1"
                            },
                            {
                                "targetPath": "priority.coding[0].system",
                                "literalValue": "http://terminology.hl7.org/CodeSystem/processpriority"
                            },
                            {
                                "targetPath": "priority.coding[0].code",
                                "literalValue": "normal"
                            },
                            {
                                "targetPath": "created",
                                "sourcePath": "date_of_service",
                                "transform": "x12_date_to_fhir_date"
                            },
                            {
                                "targetPath": "identifier[0].system",
                                "literalValue": "http://ezhealthkonnect.local/ncpdp-telecom-prescription-reference-number"
                            },
                            {
                                "targetPath": "identifier[0].value",
                                "sourcePath": "prescription_reference_number"
                            },
                            {
                                "targetPath": "insurance[0].sequence",
                                "literalValue": "1"
                            },
                            {
                                "targetPath": "insurance[0].focal",
                                "literalValue": "true"
                            },
                            {
                                "targetPath": "insurance[0].coverage.display",
                                "literalValue": "Insurance Coverage"
                            },
                            {
                                "targetPath": "item[0].sequence",
                                "literalValue": "1"
                            },
                            {
                                "targetPath": "item[0].productOrService.coding[0].system",
                                "literalValue": "http://hl7.org/fhir/sid/ndc"
                            },
                            {
                                "targetPath": "item[0].productOrService.coding[0].code",
                                "sourcePath": "product_service_id"
                            },
                            {
                                "targetPath": "item[0].servicedDate",
                                "sourcePath": "date_of_service",
                                "transform": "x12_date_to_fhir_date"
                            },
                            {
                                "targetPath": "item[0].quantity.value",
                                "sourcePath": "quantity_dispensed"
                            },
                            {
                                "targetPath": "item[0].net.value",
                                "sourcePath": "gross_amount_due"
                            },
                            {
                                "targetPath": "item[0].net.currency",
                                "literalValue": "USD"
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
                    "step_name": "Assemble B1 Request FHIR Bundle",
                    "step_alias": "assemble_b1_request_fhir_bundle",
                    "step_type": "payload.builder",
                    "sequence": 200,
                    "config": {
                        "mode": "fhir_bundle",
                        "fhirBundle": {
                            "bundleType": "collection",
                            "resourcePaths": [
                                "message.fhirPharmacyOrganization",
                                "message.fhirPatient",
                                "message.fhirClaims"
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
    'B1_REQUEST',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_dir","label":"Remote Directory","type":"string","hint":"Directory to poll for B1 claim request files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive B1 Request (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse B1 Request","icon":"🧾","step_type":"ncpdptelecom.parse"},
        {"name":"Validate","icon":"✅","step_type":"ncpdptelecom.validate"},
        {"name":"Derive Claim Context","icon":"🔧","step_type":"enrichment.script"},
        {"name":"Build Organization","icon":"🏥","step_type":"fhir.build"},
        {"name":"Build Patient","icon":"🧑","step_type":"fhir.build"},
        {"name":"Build Claim","icon":"🧾","step_type":"fhir.build"},
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
