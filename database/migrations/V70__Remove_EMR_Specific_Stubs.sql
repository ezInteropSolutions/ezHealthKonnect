-- V70: Remove EMR-vendor-specific templates and connector types
-- Keep generic: fhir_r4_inbound/outbound, edi_x12_*, direct_messaging_*
-- Remove: epic_fhir_*, cerner_fhir_*, meditech_fhir_*
-- Remove: Epic ADT and Cerner ORU OOB templates (vendor-branded, not meaningfully different from generic)

-- ── Remove vendor-branded OOB templates ──────────────────────────────────────
DELETE FROM interface_templates
WHERE slug IN ('epic-adt-feed-fhir', 'cerner-oru-results-kafka');

-- ── Remove EMR-vendor connector types ────────────────────────────────────────
DELETE FROM connectivity_types
WHERE type_name IN (
    'epic_fhir_inbound',
    'epic_fhir_outbound',
    'cerner_fhir_inbound',
    'cerner_fhir_outbound',
    'meditech_fhir_inbound'
);
