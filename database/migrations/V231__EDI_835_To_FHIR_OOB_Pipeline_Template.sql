-- V231: EDI X12 835 to FHIR OOB pipeline template
-- Applied: 2026-09-02

-- ============================================================
-- EDI 835 -> FHIR (PaymentReconciliation) TEMPLATE
-- ============================================================
-- A NEW, separate interface template — V230's existing store-only
-- "edi-835-inbound-sftp" template is left completely unmodified (never edit
-- a shipped migration; this one adds a second, independent template
-- instead). Same SFTP inbound half as V230, extended with EDI Phase 5's
-- declarative FHIR-mapping capability (fhir.build + payload.builder +
-- fhir_validation) all the way to a validated FHIR Bundle:
--   seq 5    connector.inbound   (edi_x12_inbound  — polls an SFTP directory for 835 files)
--   seq 20   edi.parse           (raw X12 -> structured JSON)
--   seq 30   edi.validate        (base X12 5010 standard checks; errors block, warnings never do)
--   seq 100  fhir.build          (PaymentReconciliation, from BPR/TRN + payer identity)
--   seq 200  payload.builder     (fhir_bundle mode — assembles the Bundle)
--   seq 210  fhir_validation     (strict — validates the assembled Bundle)
--   seq 295  connector.outbound  (sink_outbound — store-only terminal, no external I/O)
--
-- Scope, named rather than silently left out: this template builds ONE
-- PaymentReconciliation resource per interchange. It deliberately does NOT
-- build a per-claim ExplanationOfBenefit resource (one per X12 CLP/2100
-- loop) — that needs a control.loop step whose child step's ID can only be
-- wired interactively, in the pipeline builder UI, AFTER the interface
-- exists (services/transformation_pipeline_service.go's stepByID lookup is
-- keyed by the real DB-assigned step.ID, which doesn't exist yet when this
-- migration is written — no template JSON can pre-declare that link). A
-- user who wants per-claim EOB resources adds, after applying this
-- template: (1) a control.loop step (foreach over
-- parsedEDI.loops.2000[0].loops.2100) with a child fhir.build step
-- (resourceType ExplanationOfBenefit) inside it, and (2) updates this
-- template's own "Assemble 835 FHIR Bundle" step's resourcePaths to add the
-- loop's own aggregated array path alongside message.paymentReconciliation.
-- The PaymentReconciliation resource this template DOES build passes strict
-- FHIR validation on its own (verified against all 3 real 835 samples in
-- edi/testdata/real_samples/, chained through the real fhir_validation
-- executor in services/executors/transform/edi_835_to_fhir_test.go) — this
-- template is a complete, working slice on its own, not a stub.
--
-- A real, generic engine gap was found and fixed while building this:
-- payload.builder's fhir_bundle mode outputs its assembled Bundle as an
-- already-marshaled JSON STRING ("payload"), with no separate map-shaped
-- output — fhir_validation's source_field resolution only accepted a map,
-- which would have forced an enrichment.script bridge (the exact workaround
-- V212's PAS template needed) to json.Unmarshal it back. Fixed generically
-- in services/executors/validation/fhir_validation_executor.go
-- (asFHIRDataMap) instead of reaching for a script on a no-code platform —
-- source_field now accepts either shape.

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
    'EDI X12 835 to FHIR (SFTP)',
    'edi-835-to-fhir-sftp',
    'Poll an SFTP directory for X12 835 remittance files, parse and validate them, then build and validate a FHIR R4 Bundle containing a PaymentReconciliation resource. To also get one ExplanationOfBenefit resource per claim, add a control.loop step (over parsedEDI.loops.2000[0].loops.2100) with a child fhir.build(ExplanationOfBenefit) step after applying this template, then add its output path to the Assemble FHIR Bundle step''s resourcePaths.',
    'edi',
    null,
    ARRAY['edi','x12','835','remittance','payers','sftp','fhir'],
    '💰',
    'intermediate',
    'edi_x12_inbound',
    '{"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["835"]}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    '{
        "execution_groups": [
            {
                "sequence": 5,
                "steps": [{
                    "step_name": "Receive 835 File (SFTP)",
                    "step_type": "connector.inbound",
                    "sequence": 5,
                    "config": {
                        "connectorType": "edi_x12_inbound",
                        "config": {"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["835"]}
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 20,
                "steps": [{
                    "step_name": "Parse 835 -> JSON",
                    "step_type": "edi.parse",
                    "sequence": 20,
                    "config": {"sourceField": "raw", "outputField": "parsedEDI", "transactionSet": "835"},
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 30,
                "steps": [{
                    "step_name": "Validate Against X12 5010",
                    "step_type": "edi.validate",
                    "sequence": 30,
                    "config": {"sourceField": "raw", "outputField": "ediValidation", "customRules": []},
                    "enabled": true,
                    "required": false
                }]
            },
            {
                "sequence": 100,
                "steps": [{
                    "step_name": "Build PaymentReconciliation",
                    "step_alias": "build_payment_reconciliation",
                    "step_type": "fhir.build",
                    "sequence": 100,
                    "description": "Builds one PaymentReconciliation resource from the interchange''s BPR (payment amount/date) and TRN (check/EFT trace number) header segments, plus the payer''s own N1 identity from loop 1000A. To also summarize individual claims (detail[]), add a control.loop + child fhir.build(ExplanationOfBenefit) step first, then add a detail[] repeatingGroup here (rowsPath parsedEDI.loops.2000[0].loops.2100) referencing the loop''s own resource IDs.",
                    "config": {
                        "resourceType": "PaymentReconciliation",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.paymentReconciliation",
                        "fields": [
                            {"targetPath": "id", "sourcePath": "parsedEDI.header.TRN.checkOrEFTTraceNumber"},
                            {"targetPath": "status", "literalValue": "active"},
                            {"targetPath": "outcome", "literalValue": "complete"},
                            {"targetPath": "created", "sourcePath": "parsedEDI.header.BPR.paymentEffectiveDate", "transform": "x12_date_to_fhir_date"},
                            {"targetPath": "paymentDate", "sourcePath": "parsedEDI.header.BPR.paymentEffectiveDate", "transform": "x12_date_to_fhir_date"},
                            {"targetPath": "paymentAmount.value", "sourcePath": "parsedEDI.header.BPR.totalActualProviderPaymentAmount", "transform": "cda_decimal_string_to_number"},
                            {"targetPath": "paymentIssuer.display", "sourcePath": "parsedEDI.loops.1000A.N1.name"}
                        ]
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 200,
                "steps": [{
                    "step_name": "Assemble 835 FHIR Bundle",
                    "step_alias": "assemble_835_fhir_bundle",
                    "step_type": "payload.builder",
                    "sequence": 200,
                    "description": "Assembles the built resource(s) into a FHIR R4 Bundle. resourcePaths accepts both single-resource paths (message.paymentReconciliation) and array-valued ones (e.g. a control.loop step''s own aggregated ExplanationOfBenefit array, once added) in the same list — every resource lands in one flat Bundle with correctly cross-referenced fullUrls regardless of which path produced it.",
                    "config": {
                        "mode": "fhir_bundle",
                        "fhirBundle": {
                            "bundleType": "collection",
                            "resourcePaths": ["message.paymentReconciliation"]
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
    }',
    '835',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_path","label":"Remote Directory","type":"string","hint":"Directory to poll for 835 files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive 835 (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse 835","icon":"📄","step_type":"edi.parse"},
        {"name":"Validate X12","icon":"✅","step_type":"edi.validate"},
        {"name":"Build PaymentReconciliation","icon":"💰","step_type":"fhir.build"},
        {"name":"Assemble FHIR Bundle","icon":"📦","step_type":"payload.builder"},
        {"name":"Validate FHIR","icon":"🛡️","step_type":"fhir_validation"},
        {"name":"Store Result","icon":"💾","step_type":"connector.outbound"}
    ]',
    20,
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
