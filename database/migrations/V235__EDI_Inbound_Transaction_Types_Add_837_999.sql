-- V235: edi_x12_inbound.transaction_types' UI enum only ever offered "835"
-- (V232), but 837P/837I/999 are now real, schema-backed transaction sets
-- (edi/schemas/x12_005010/{837P,837I,999}.json) — this file catches the
-- connectivity_types config_schema UI up to that, additive only.
-- Applied: 2026-09-09
--
-- BACKGROUND
-- ----------
-- V232 restricted `transaction_types`' `items.enum` to `["835"]` so the
-- pipeline builder's generic schema renderer (ConnectorConfigBuilder.js)
-- would show a checkbox list instead of free text -- correct at the time
-- (only 835 was supported end-to-end), but now stale: edi.parse/edi.build/
-- edi.map_to_canonical all resolve "837P"/"837I"/"999" via
-- edi/schema_loader.go's dual friendly-id registration (see CLAUDE.md's EDI
-- Phase 2 section), and edi_x12_inbound.go's own transaction_types field was
-- always "accepted but not yet enforced at the connector level" -- widening
-- the enum here doesn't change Go behavior, only what values the UI offers.
--
-- Only edi_x12_inbound has this field (edi_x12_outbound has none), same as
-- V232 found.

UPDATE connectivity_types
SET config_schema = jsonb_set(
    config_schema,
    '{properties,transaction_types}',
    '{"type": "array", "title": "Transaction Types", "default": ["835"], "items": {"type": "string", "enum": ["835", "837P", "837I", "999"]}, "description": "Which transaction set(s) this listener expects. Accepted but not enforced at the connector level (batch_splitter.go and edi.parse auto-detect the real transaction set from each file''s own ST01/GS08 regardless of this setting) -- purely documentation for anyone configuring the interface. 270/271 remain a named future phase, not selectable yet."}'::jsonb
)
WHERE type_name = 'edi_x12_inbound';
