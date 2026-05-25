-- V133: Add validateBase64:true to all MDM OOB template composites params
--
-- MDM^T01-T11 templates use buildDocumentReferenceFromTXA.  OBX ED segments
-- in MDM messages carry the document body (PDF, images, etc.) encoded as
-- base64.  The procedure now supports a validateBase64 param (wired in V133
-- alongside this migration) with the same ruleOn semantics as ORU:
--
--   true  (default) — validate: omit data and flag with a title when the
--                     base64 string cannot be decoded; satisfies FHIR att-1/att-2.
--   false           — passthrough: copy as-is for known-good sources.
--
-- The JSONB path targets the first (and only) composite entry in the
-- DocumentReference mappings block, matching the structure seeded by V99.

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{mappings,DocumentReference,composites,0,params,validateBase64}',
    'true'::jsonb
)
WHERE message_type LIKE 'MDM^%'
  AND is_default = true;
