-- V241: edi_x12_inbound.transaction_types' UI enum widened to include 270/271
-- (EDI X12 Phase 3 — eligibility, transform-only: 270/271 <-> FHIR, same as
-- 835/837 <-> FHIR). Additive only, same pattern V235 already established for
-- 837P/837I/999.
--
-- BACKGROUND
-- ----------
-- V235's own description text explicitly said "270/271 remain a named future
-- phase, not selectable yet" -- that phase is this one. edi.parse/edi.build/
-- edi.map_to_canonical already resolve "270"/"271" generically once
-- edi/schemas/x12_005010/{270,271}.json exist (schema-driven engine, zero new
-- Go code for a new transaction set, same promise this project made from
-- Phase 1 onward) -- widening the enum here doesn't change Go behavior, only
-- what values the UI offers.

UPDATE connectivity_types
SET config_schema = jsonb_set(
    config_schema,
    '{properties,transaction_types}',
    '{"type": "array", "title": "Transaction Types", "default": ["835"], "items": {"type": "string", "enum": ["835", "837P", "837I", "999", "270", "271"]}, "description": "Which transaction set(s) this listener expects. Accepted but not enforced at the connector level (batch_splitter.go and edi.parse auto-detect the real transaction set from each file''s own ST01/GS08 regardless of this setting) -- purely documentation for anyone configuring the interface."}'::jsonb
)
WHERE type_name = 'edi_x12_inbound';
