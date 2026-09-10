-- V236__EDI_837_OOB_Pipeline_Template.sql
-- OOB interface template for EDI X12 837 (Health Care Claim, both
-- Professional and Institutional) inbound ingestion via SFTP -- the 837
-- counterpart to V230__EDI_835_OOB_Pipeline_Template.sql, same pattern.
--
-- ONE template covers BOTH 837P and 837I, not two separate ones: inbound
-- ingestion is transaction-set-agnostic at the connector level (a trading
-- partner's outbound-claims SFTP drop can mix both), and edi.parse itself
-- auto-detects which variant each file actually is from its own GS08 value
-- (see edi/loop_engine.go's composite ST01+GS08 lookup) -- the pipeline
-- doesn't need to know in advance, so the template shouldn't force a choice.
--
-- When a user clicks "Use Template" they get a 4-step pipeline pre-wired for
-- SFTP polling, parsing, and validation:
--   seq 5   connector.inbound   (edi_x12_inbound  — polls an SFTP directory for 837 files)
--   seq 20  edi.parse           (raw X12 -> structured JSON, auto-detects 837P vs 837I)
--   seq 30  edi.validate        (base X12 5010 standard checks; errors block, warnings never do)
--   seq 295 connector.outbound  (sink_outbound — store-only terminal, no external I/O)
--
-- sink_outbound is deliberately the terminal step, same reasoning as V230's
-- own 835 template: no X12->FHIR mapping exists for 837 yet (a named future
-- phase), so this template proves parse+validate end-to-end honestly rather
-- than pretending delivery or claims-adjudication routing is finished.
-- Applied: 2026-09-09

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
    'EDI X12 837 Inbound (SFTP)',
    'edi-837-inbound-sftp',
    'Poll an SFTP directory for X12 837 (Health Care Claim — Professional and Institutional) files, parse them into structured JSON, and validate against the base X12 5010 standard. Auto-detects 837P vs 837I per file from its own GS08 value; no separate template needed for each.',
    'edi',
    null,
    ARRAY['edi','x12','837','claims','professional','institutional','providers','sftp'],
    '🏥',
    'intermediate',
    'edi_x12_inbound',
    '{"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["837P", "837I"]}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    '{
        "execution_groups": [
            {
                "sequence": 5,
                "steps": [{
                    "step_name": "Receive 837 File (SFTP)",
                    "step_type": "connector.inbound",
                    "sequence": 5,
                    "config": {
                        "connectorType": "edi_x12_inbound",
                        "config": {"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["837P", "837I"]}
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 20,
                "steps": [{
                    "step_name": "Parse 837 -> JSON",
                    "step_type": "edi.parse",
                    "sequence": 20,
                    "config": {"sourceField": "raw", "outputField": "parsedEDI", "transactionSet": "837P"},
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
    '837',
    '[
        {"section":"source","field":"host","label":"SFTP Host","type":"string","hint":"Hostname or IP of the trading partner''s SFTP server","required":true},
        {"section":"source","field":"username","label":"SFTP Username","type":"string","required":true},
        {"section":"source","field":"password","label":"SFTP Password","type":"password","hint":"Leave blank if using key-based auth instead","required":false},
        {"section":"source","field":"remote_path","label":"Remote Directory","type":"string","hint":"Directory to poll for 837 files, e.g. /incoming","required":false}
    ]',
    ARRAY['source.password'],
    '[
        {"name":"Receive 837 (SFTP)","icon":"📥","step_type":"connector.inbound"},
        {"name":"Parse 837","icon":"📄","step_type":"edi.parse"},
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
