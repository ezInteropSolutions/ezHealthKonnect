-- V93: Add connector.inbound (seq 5) and connector.outbound (seq 295) execution groups
-- to the Da Vinci PAS template pipeline_config.
--
-- Without these groups the engine has no entry/exit point and _submitConfigureTemplate
-- skips the groups entirely, leaving the pipeline with no way to receive or deliver messages.

UPDATE interface_templates
SET
    pipeline_config = jsonb_set(
        pipeline_config,
        '{execution_groups}',
        (pipeline_config -> 'execution_groups') || $steps$[
            {
                "sequence": 5,
                "steps": [
                    {
                        "step_name":  "Receive PA Request",
                        "step_type":  "connector.inbound",
                        "step_alias": "receive_request",
                        "sequence":   5,
                        "enabled":    true,
                        "required":   true,
                        "description": "Listens for incoming FHIR prior authorization requests from the provider EHR. Configure the host and port after the interface is created.",
                        "config": {
                            "connectorType": "http_rest_inbound",
                            "config": {
                                "host": "0.0.0.0",
                                "port": 8443
                            }
                        }
                    }
                ]
            },
            {
                "sequence": 295,
                "steps": [
                    {
                        "step_name":  "Deliver PA Decision",
                        "step_type":  "connector.outbound",
                        "step_alias": "deliver_decision",
                        "sequence":   295,
                        "enabled":    true,
                        "required":   true,
                        "description": "Delivers the prior authorization decision back to the requesting system. Configure the callback endpoint after the interface is created.",
                        "config": {
                            "connectorType": "http_outbound",
                            "config": {
                                "endpoint": "",
                                "method":   "POST",
                                "authentication_type": "none"
                            },
                            "contentType":  "application/fhir+json",
                            "contentField": "steps.parse_response.step_output"
                        }
                    }
                ]
            }
        ]$steps$::jsonb
    ),

    -- Add the connector steps to the visual preview shown on the template card / modal
    preview_steps = '[
        {"icon": "📥", "name": "Receive PA Request",     "step_type": "connector.inbound"},
        {"icon": "🗺️", "name": "Map to PAS Envelope",   "step_type": "field_mapping"},
        {"icon": "🏗️", "name": "Build FHIR Resources",  "step_type": "enrichment.script"},
        {"icon": "📦", "name": "Assemble PAS Bundle",    "step_type": "enrichment.script"},
        {"icon": "✅", "name": "Validate FHIR R4",       "step_type": "fhir_validation"},
        {"icon": "🔐", "name": "Submit via SMART OAuth", "step_type": "enrichment.api"},
        {"icon": "🔀", "name": "Route AA/AD/CP/PE",      "step_type": "switch_case"},
        {"icon": "📤", "name": "Deliver PA Decision",    "step_type": "connector.outbound"}
    ]'::jsonb

WHERE slug = 'davinci-pas-r4';
