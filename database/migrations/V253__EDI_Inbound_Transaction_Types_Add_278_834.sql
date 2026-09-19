-- V253: edi_x12_inbound.transaction_types' UI enum widened to include 278/834
-- (EDI Phase 7 — 278 prior authorization request/response, 834 benefit
-- enrollment/maintenance). Additive only, same pattern V235/V241/V250 already
-- established.
--
-- BACKGROUND
-- ----------
-- edi.parse/edi.build/edi.map_to_canonical already resolve "278"/"834"
-- generically once edi/schemas/x12_005010/{278,834}.json exist (schema-driven
-- engine, zero new Go code for a new transaction set) — widening the enum
-- here doesn't change Go behavior, only what values the UI offers.

UPDATE connectivity_types
SET config_schema = jsonb_set(
    config_schema,
    '{properties,transaction_types}',
    '{"type": "array", "title": "Transaction Types", "default": ["835"], "items": {"type": "string", "enum": ["835", "837P", "837I", "999", "270", "271", "276", "277", "278", "834"]}, "description": "Which transaction set(s) this listener expects. Accepted but not enforced at the connector level (batch_splitter.go and edi.parse auto-detect the real transaction set from each file''s own ST01/GS08 regardless of this setting) -- purely documentation for anyone configuring the interface."}'::jsonb
)
WHERE type_name = 'edi_x12_inbound';
