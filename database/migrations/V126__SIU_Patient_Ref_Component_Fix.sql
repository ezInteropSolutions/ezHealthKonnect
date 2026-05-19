-- V126: Use PID.3.1 for Patient participant reference in SIU Appointment
--
-- PID.3 is a CX composite field: "42^^^DEMO^MR"
-- Using {{PID.3}} produces "Patient/42^^^DEMO^MR" — invalid FHIR reference URL.
-- Using {{PID.3.1}} extracts only the entity identifier component → "Patient/42".
--
-- composites[1] is the Patient participant composite (condition: PID.3 present).

BEGIN;

UPDATE hl7_fhir_templates
SET template_config = jsonb_set(
        template_config,
        '{mappings,Appointment,composites,1,value,actor,reference}',
        '"Patient/{{PID.3.1}}"'
    ),
    updated_at = NOW()
WHERE message_type LIKE 'SIU%'
  AND is_default = true
  AND profile_version = '2.0'
  AND template_config #>> '{mappings,Appointment,composites,1,value,actor,reference}' IS NOT NULL;

DO $$
DECLARE v_count INT;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM hl7_fhir_templates
    WHERE message_type LIKE 'SIU%'
      AND is_default = true
      AND template_config #>> '{mappings,Appointment,composites,1,value,actor,reference}' = 'Patient/{{PID.3.1}}';
    RAISE NOTICE 'V126 complete: % SIU rows now use PID.3.1 for Patient reference', v_count;
END $$;

COMMIT;
