-- V232: edi_x12_inbound.transaction_types needs a fixed value set to render
-- as a checkbox list in the pipeline builder's connector config UI.
-- Applied: 2026-09-06
--
-- BACKGROUND
-- ----------
-- connectivity_types.config_schema's `transaction_types` field was declared
-- as a bare `{"type": "array"}` with no `items` at all -- the generic schema
-- renderer (public/js/pipeline/components/ConnectorConfigBuilder.js) only
-- turns an array field into a checkbox list when `items.enum` is present;
-- with no `items` it fell through to a free-text comma-separated input,
-- letting a user type any string even though only "835" is supported
-- end-to-end today (edi_x12_inbound.go's own doc comment: "transaction_types
-- is accepted in config ... but not yet enforced at the connector level in
-- phase 1 -- with only '835' supported end-to-end, filtering has no real
-- effect yet"). Adding `items.enum` here is additive -- it doesn't change
-- what the Go connector reads or enforces, only what values the UI offers.
--
-- Only edi_x12_inbound has this field (edi_x12_outbound has no
-- transaction_types key at all -- confirmed via direct schema inspection).

UPDATE connectivity_types
SET config_schema = jsonb_set(
    config_schema,
    '{properties,transaction_types}',
    '{"type": "array", "title": "Transaction Types", "default": ["835"], "items": {"type": "string", "enum": ["835"]}, "description": "Accepted but not yet enforced at the connector level in phase 1 (only 835 is supported end-to-end) -- 837/270/271 are named future phases, not selectable yet"}'::jsonb
)
WHERE type_name = 'edi_x12_inbound';
