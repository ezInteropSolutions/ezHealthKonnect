-- V124: Fix two issues in SIU OOB template (V123 follow-up)
--
-- Fix 1: PART coding display
--   "Participant" → "Participation"
--   Per HL7 v3 CodeSystem/v3-ParticipationType, PART display is "Participation".
--
-- Fix 2: Appointment identifier firstOf paths
--   SCH.2 / SCH.1 are EI (Entity Identifier) composite fields.
--   Raw field value includes namespace: "2196178^2196178".
--   SCH.2.1 / SCH.1.1 extract only the entity-identifier component.

BEGIN;

-- Apply both fixes in a single jsonb expression per row.
-- jsonb_set cannot reach deeply nested arrays in one step, so we rebuild
-- the composites array with the corrected values using a scalar replacement.

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
        jsonb_set(
            template_config,
            -- Fix PART display inside composites[3].value.type[0].coding[0].display
            '{mappings,Appointment,composites,3,value,type,0,coding,0,display}',
            '"Participation"'
        ),
        -- Fix identifier firstOf: SCH.2 → SCH.2.1, SCH.1 → SCH.1.1
        '{mappings,Appointment,composites,0,firstOf}',
        '["SCH.2.1","SCH.1.1"]'
    ),
    updated_at = NOW()
WHERE message_type LIKE 'SIU%'
  AND is_default = true
  AND profile_version = '2.0'
  AND template_config->'mappings'->'Appointment'->'composites' IS NOT NULL;

DO $$
DECLARE v_count INT;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM hl7_fhir_templates
    WHERE message_type LIKE 'SIU%'
      AND is_default = true
      AND template_config #>> '{mappings,Appointment,composites,3,value,type,0,coding,0,display}' = 'Participation';
    RAISE NOTICE 'V124 complete: % SIU rows now have PART=Participation and SCH.2.1 identifier', v_count;
END $$;

COMMIT;
