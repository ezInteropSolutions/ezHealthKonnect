-- V134: Add non-standard ORC.5 status codes to ORM/OML/OMG order status valueMaps
--
-- Some senders use plain-English values like "Pending" in ORC.5 instead of
-- HL7 Table 0038 codes (NW, IP, CM, etc.).  Map these to their closest
-- FHIR RequestStatus equivalent so validation passes.

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{mappings,ServiceRequest,fieldMappings}',
    (
        SELECT jsonb_agg(
            CASE
                WHEN (elem->>'hl7Path') = 'ORC.5'
                THEN jsonb_set(
                    elem,
                    '{valueMap}',
                    (elem->'valueMap') ||
                    '{"Pending":"active","Scheduled":"active","Hold":"on-hold","Cancelled":"revoked","Completed":"completed"}'::jsonb
                )
                ELSE elem
            END
        )
        FROM jsonb_array_elements(template_config->'mappings'->'ServiceRequest'->'fieldMappings') AS elem
    )
)
WHERE message_type IN ('ORM^O01', 'OML^O21', 'OMG^O19')
  AND is_default = true
  AND template_config->'mappings'->'ServiceRequest'->'fieldMappings' IS NOT NULL;
