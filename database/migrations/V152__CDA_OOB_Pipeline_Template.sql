-- V152__CDA_OOB_Pipeline_Template.sql
-- Sprint D: OOB interface template for CDA/CCD inbound → FHIR R4 processing.
--
-- Seeds one system template ("CDA/CCD Inbound → FHIR R4") that the Templates
-- gallery surfaces immediately. When a user clicks "Use Template" they get a
-- 5-step pipeline pre-wired for CDA normalisation, parsing, and FHIR mapping.
--
-- Step sequence follows the CDA pipeline architecture (Part 5B of the SDLC plan):
--   seq 5   connector.inbound   (http_rest_inbound — accepts CDA XML via HTTP POST)
--   seq 10  cda.normalize       (C32/HITSP → C-CDA 2.1 normalisation)
--   seq 20  cda.parse           (XML → USCDI-keyed ParsedJSON)
--   seq 100 cda.to_fhir         (parsedCDA → FHIR R4 Bundle, US Core, all sections)
--   seq 295 connector.outbound  (http_fhir_outbound — delivers bundle to FHIR server)
--
-- Uses the same INSERT pattern as V68__OOB_Interface_Templates.sql.

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
    'CDA/CCD Inbound → FHIR R4',
    'cda-ccd-inbound-fhir-r4',
    'Receive a C-CDA 2.1 or C32 clinical document (CCD, Discharge Summary, Referral Note) via HTTP POST, normalise C32/HITSP OIDs, parse to USCDI-keyed JSON, transform to a FHIR R4 Bundle (US Core 6.1.0), and deliver to any FHIR-compliant REST endpoint.',
    'cda',
    null,
    ARRAY['cda','ccda','c32','fhir','r4','us-core','clinical-document','http'],
    '📋',
    'intermediate',
    'http_rest_inbound',
    '{"path": "/cda/inbound", "method": "POST", "content_type": "text/xml", "enable_auth": true, "auth_type": "bearer"}',
    'http_fhir_outbound',
    '{"method": "POST", "content_type": "application/fhir+json", "auth_type": "bearer", "timeout_seconds": 30}',
    '{
        "execution_groups": [
            {
                "sequence": 5,
                "steps": [{
                    "step_name": "Receive CDA Document",
                    "step_type": "connector.inbound",
                    "sequence": 5,
                    "config": {
                        "connectorType": "http_rest_inbound",
                        "config": {"path": "/cda/inbound", "method": "POST", "content_type": "text/xml", "enable_auth": true, "auth_type": "bearer"}
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 10,
                "steps": [{
                    "step_name": "Normalise C32/HITSP → C-CDA 2.1",
                    "step_type": "cda.normalize",
                    "sequence": 10,
                    "config": {"sourceField": "raw"},
                    "enabled": true,
                    "required": false
                }]
            },
            {
                "sequence": 20,
                "steps": [{
                    "step_name": "Parse CDA → USCDI JSON",
                    "step_type": "cda.parse",
                    "sequence": 20,
                    "config": {"sourceField": "raw", "outputField": "parsedCDA"},
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 100,
                "steps": [{
                    "step_name": "CDA → FHIR R4 Bundle (US Core)",
                    "step_type": "cda.to_fhir",
                    "sequence": 100,
                    "config": {
                        "sourceField": "parsedCDA",
                        "bundleType": "collection",
                        "profileMode": "us-core",
                        "onSectionFailure": "continue",
                        "terminologyValidation": false
                    },
                    "enabled": true,
                    "required": true
                }]
            },
            {
                "sequence": 295,
                "steps": [{
                    "step_name": "Deliver FHIR Bundle",
                    "step_type": "connector.outbound",
                    "sequence": 295,
                    "config": {
                        "connectorType": "http_fhir_outbound",
                        "config": {"method": "POST", "content_type": "application/fhir+json", "auth_type": "bearer", "timeout_seconds": 30},
                        "contentField": "fhirBundle"
                    },
                    "enabled": true,
                    "required": true
                }]
            }
        ]
    }',
    null,
    '[
        {"section":"source","field":"path","label":"Inbound HTTP Path","type":"string","hint":"e.g. /cda/inbound — path on the Go listener","required":false},
        {"section":"target","field":"endpoint","label":"FHIR Server Endpoint URL","type":"url","hint":"e.g. https://fhir.hospital.org/R4/","required":true},
        {"section":"target","field":"token","label":"Bearer Token","type":"password","hint":"Authorization token for the FHIR server","required":false}
    ]',
    ARRAY['target.token'],
    '[
        {"name":"Normalise C32","icon":"🔧","step_type":"cda.normalize"},
        {"name":"Parse CDA","icon":"📄","step_type":"cda.parse"},
        {"name":"CDA → FHIR R4","icon":"🔄","step_type":"cda.to_fhir"},
        {"name":"Deliver FHIR","icon":"📤","step_type":"connector.outbound"}
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
