-- V143: Assembly Rules — logical_ref rule type + ADT Coverage focus wiring
--
-- Changes:
--   1. Add config JSONB column to assembly_rules for rule-specific parameters.
--      Used by logical_ref to store identifier_system, identifier_field,
--      display_field.  NULL for all existing rule types — no behaviour change.
--   2. Extend rule_type constraint to include 'logical_ref'.
--      A logical_ref rule builds an identifier-based FHIR reference
--      ({identifier:{system,value}, display}) instead of a URL reference.
--      Used when the referenced resource may not exist in the same bundle.
--   3. Add MessageHeader.focus → Coverage for all ADT variants.
--      Fixes FHIR validator warning: "Coverage not reachable from MessageHeader".
--   4. Add Coverage.payor → Organization (logical_ref) for all ADT variants.
--      When an Organization exists in the bundle its name is used as the
--      identifier value.  Fallback: use the display-only value already set
--      by field mapping (e.g. ZIN.2 = "BCBS" → payor[0].display).

BEGIN;

-- 1. Add config column (nullable — existing rows carry NULL, behaviour unchanged)
ALTER TABLE assembly_rules ADD COLUMN IF NOT EXISTS config JSONB;

-- 2. Extend rule_type constraint
ALTER TABLE assembly_rules DROP CONSTRAINT IF EXISTS assembly_rules_rule_type_check;
ALTER TABLE assembly_rules ADD CONSTRAINT assembly_rules_rule_type_check
    CHECK (rule_type IN (
        'reference', 'focus', 'result', 'subject',
        'performer', 'encounter', 'author', 'logical_ref'
    ));

-- 3 + 4. New ADT rules
INSERT INTO assembly_rules
    (message_type, rule_type, source_resource, target_resource,
     reference_path, sequence, config)
SELECT msg, rule_type, source_resource, target_resource,
       reference_path, sequence,
       CASE WHEN config IS NOT NULL THEN config::jsonb ELSE NULL END
FROM (VALUES
    -- MessageHeader.focus gains Coverage so the bundle is self-navigable
    ('focus',       'MessageHeader', 'Coverage',     'focus',  206, NULL),
    -- Coverage.payor → logical identifier reference (org name as identifier value)
    ('logical_ref', 'Coverage',      'Organization', 'payor',  300,
     '{"identifier_system":"urn:local:payer","identifier_field":"name","display_field":"name"}')
) AS t(rule_type, source_resource, target_resource, reference_path, sequence, config)
CROSS JOIN (VALUES
    ('ADT^A01'),('ADT^A02'),('ADT^A03'),('ADT^A04'),('ADT^A05'),
    ('ADT^A06'),('ADT^A07'),('ADT^A08'),('ADT^A09'),('ADT^A10'),
    ('ADT^A11'),('ADT^A12'),('ADT^A13'),('ADT^A28'),('ADT^A31'),
    ('ADT^A40'),('ADT^A47')
) AS m(msg)
ON CONFLICT DO NOTHING;

-- Verify
DO $$
DECLARE v_focus   INTEGER;
        v_logical INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_focus
    FROM assembly_rules
    WHERE rule_type = 'focus' AND target_resource = 'Coverage' AND is_active = true;

    SELECT COUNT(*) INTO v_logical
    FROM assembly_rules
    WHERE rule_type = 'logical_ref' AND is_active = true;

    RAISE NOTICE 'V143 complete: % Coverage-focus rules, % logical_ref rules', v_focus, v_logical;
END $$;

COMMIT;
