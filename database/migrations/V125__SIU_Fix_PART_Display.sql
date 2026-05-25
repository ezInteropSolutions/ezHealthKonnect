-- V125: Fix PART display and restore LOC display after V124 index error
--
-- V124 used composites array index 3 (AIL/LOC) instead of 4 (AIG/PART).
-- Result: LOC display was set to "Participation" (wrong) and PART unchanged.
--
-- This migration:
--   composites[3] (AIL/LOC) → restore display to "Location"
--   composites[4] (AIG/PART) → set display to "Participation" (correct per HL7 v3 spec)

BEGIN;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
        jsonb_set(
            template_config,
            '{mappings,Appointment,composites,3,value,type,0,coding,0,display}',
            '"Location"'
        ),
        '{mappings,Appointment,composites,4,value,type,0,coding,0,display}',
        '"Participation"'
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
      AND template_config #>> '{mappings,Appointment,composites,4,value,type,0,coding,0,display}' = 'Participation'
      AND template_config #>> '{mappings,Appointment,composites,3,value,type,0,coding,0,display}' = 'Location';
    RAISE NOTICE 'V125 complete: % SIU rows have LOC=Location, PART=Participation', v_count;
END $$;

COMMIT;
