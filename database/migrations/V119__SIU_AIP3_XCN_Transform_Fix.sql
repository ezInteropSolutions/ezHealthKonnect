-- V119: Fix AIP.3 transform in SIU templates
--
-- AIP.3 is XCN (Extended Composite ID Number And Name For Persons).
-- The xcn_to_reference transform correctly extracts "Family, Given" from
-- the XCN composite "id^family^given^..." before setting actor.display.
--
-- Templates generated before this transform was wired had an empty or
-- incorrect transform, causing the raw XCN string (e.g. "10345^Adams^Douglas")
-- to land at actor.display. The sanitizer then deleted it because it contained
-- "^", leaving a bare {"status":"needs-action"} participant that violates
-- the app-1 constraint.

BEGIN;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
    template_config,
    '{resources,Appointment,mappings}',
    (
        SELECT jsonb_agg(
            CASE
                WHEN (m->>'hl7Path') = 'AIP.3'
                 AND (m->>'hl7DataType') = 'XCN'
                THEN m || '{"transform": "xcn_to_reference"}'::jsonb
                ELSE m
            END
        )
        FROM jsonb_array_elements(
            template_config->'resources'->'Appointment'->'mappings'
        ) AS m
    )
)
WHERE message_type LIKE 'SIU%'
  AND template_config->'resources'->'Appointment'->'mappings' IS NOT NULL
  AND EXISTS (
      SELECT 1
      FROM jsonb_array_elements(
          template_config->'resources'->'Appointment'->'mappings'
      ) m
      WHERE (m->>'hl7Path') = 'AIP.3'
        AND (m->>'hl7DataType') = 'XCN'
        AND COALESCE(m->>'transform', '') != 'xcn_to_reference'
  );

DO $$
DECLARE v_count INTEGER;
BEGIN
    GET DIAGNOSTICS v_count = ROW_COUNT;
    RAISE NOTICE 'V119 complete: % SIU template(s) updated (AIP.3 XCN → xcn_to_reference)', v_count;
END $$;

COMMIT;
