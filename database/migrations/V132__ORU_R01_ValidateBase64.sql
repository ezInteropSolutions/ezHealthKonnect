-- V132: Add validateBase64:true to ORU^R01 OOB template composites params
--
-- ORU^R01 uses assembleObservationsFromOBX (same as R03).  The procedure
-- already defaults to validate-ON via ruleOn(nil) semantics, but stating
-- it explicitly in the template makes the intent clear and allows a
-- per-interface override to false when the source is a trusted system.
--
-- Mirror of V131 which applied the same change to ORU^R03.

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{mappings,DiagnosticReport,composites,0,params,validateBase64}',
    'true'::jsonb
)
WHERE message_type = 'ORU^R01'
  AND is_default   = true;
