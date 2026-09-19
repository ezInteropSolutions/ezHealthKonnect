-- V256: edi_x12_inbound.transaction_types gains friendly display labels
-- (enumNames, the common JSON-Schema companion-array convention for
-- enum -- same length/order as items.enum) and a shorter, UI-appropriate
-- description (the previous wording referenced "batch_splitter.go" by Go
-- filename, appropriate for a code comment, not end-user-facing UI copy).
-- Also flips the default from a misleading ["835"] (implying "only 835 is
-- accepted") to the FULL list -- the real, enforced-nowhere behavior is
-- "every transaction set is auto-detected and accepted regardless of this
-- field," so a fresh interface's own default should reflect that honestly.
--
-- Purely additive/cosmetic -- no change to what is accepted or enforced at
-- the connector level (see V232/V250/V253's own comments: this field has
-- never been enforced; edi.parse/batch_splitter.go always auto-detect the
-- real transaction set from each file's own ST01/GS08).

UPDATE connectivity_types
SET config_schema = jsonb_set(
    config_schema,
    '{properties,transaction_types}',
    '{
        "type": "array",
        "title": "Transaction Types",
        "default": ["835", "837P", "837I", "999", "270", "271", "276", "277", "278", "834"],
        "items": {
            "type": "string",
            "enum": ["835", "837P", "837I", "999", "270", "271", "276", "277", "278", "834"],
            "enumNames": [
                "835 — Remittance Advice",
                "837P — Professional Claim",
                "837I — Institutional Claim",
                "999 — Functional Acknowledgment",
                "270 — Eligibility Request",
                "271 — Eligibility Response",
                "276 — Claim Status Request",
                "277 — Claim Status Response",
                "278 — Prior Authorization",
                "834 — Benefit Enrollment"
            ]
        },
        "description": "Documents which transaction sets this interface is expected to handle. Every transaction set is automatically detected and parsed from each file''s own envelope (ST01/GS08) regardless of this selection — this list is informational only, not a filter."
    }'::jsonb
)
WHERE type_name = 'edi_x12_inbound';
