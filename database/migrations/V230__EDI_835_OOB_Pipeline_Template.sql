-- V230__EDI_835_OOB_Pipeline_Template.sql
-- OOB interface template for EDI X12 835 (Health Care Claim Payment/Advice)
-- inbound ingestion via SFTP -- the Templates gallery counterpart to
-- V152__CDA_OOB_Pipeline_Template.sql, same INSERT pattern.
--
-- When a user clicks "Use Template" they get a 4-step pipeline pre-wired for
-- SFTP polling, parsing, and validation:
--   seq 5   connector.inbound   (edi_x12_inbound  — polls an SFTP directory for 835 files)
--   seq 20  edi.parse           (raw X12 -> structured JSON)
--   seq 30  edi.validate        (base X12 5010 standard checks; errors block, warnings never do)
--   seq 295 connector.outbound  (sink_outbound — store-only terminal, no external I/O)
--
-- sink_outbound is deliberately the terminal step, not edi.build or a real
-- downstream system: there is no X12->FHIR mapping yet (a separate, later
-- phase) and no real trading-partner delivery target to assume — this
-- template proves parse+validate end-to-end honestly rather than pretending
-- delivery is finished. edi.map_to_canonical/edi.build are deliberately NOT
-- part of this template: per this feature's own design, the 4 EDI step
-- types are independent, individually-toolbox-addable capabilities, and
-- this specific template is for the inbound-ingestion use case only, not a
-- round-trip demo.

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
    'EDI X12 835 Inbound (SFTP)',
    'edi-835-inbound-sftp',
    'Poll an SFTP directory for X12 835 (Health Care Claim Payment/Advice) remittance files, parse them into structured JSON, and validate against the base X12 5010 standard. Phase 1: SFTP transport and 835 only.',
    'edi',
    null,
    ARRAY['edi','x12','835','remittance','payers','sftp'],
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
                "sequence": 295,
                "steps": [{
                    "step_name": "Store Parsed Result",
                    "step_type": "connector.outbound",
                    "sequence": 295,
                    "config": {
                        "connectorType": "sink_outbound",
                        "config": {"enable_logging": true, "enable_validation": true},
                        "contentField": "parsedEDI"
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
        {"name":"Validate","icon":"✅","step_type":"edi.validate"},
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
