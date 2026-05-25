-- V121: Inject AIP.3 actor.display mapping into SIU wizard-saved custom configs
--
-- Wizard-saved custom_mapping_config in interface_message_mappings stores
-- atomicMappings as a JSON array of {sourcePath, targetPath, transformType}.
-- The wizard generated AIS.10 → participant[0].status (creating the participant)
-- but did not include AIP.3 (personnel practitioner name), so participant[0]
-- ended up with no actor — violating FHIR constraint app-1.
--
-- This migration appends an AIP.3 entry to every SIU atomicMappings array that
-- does not already contain AIP.3.

BEGIN;

UPDATE interface_message_mappings
SET custom_mapping_config = jsonb_set(
    custom_mapping_config,
    '{atomicMappings}',
    (
        custom_mapping_config->'atomicMappings'
        || '[{"sourcePath":"AIP.3","targetPath":"Appointment.participant[0].actor.display","transformType":"xcn_to_reference","isRequired":false}]'::jsonb
    )
)
WHERE message_type LIKE 'SIU%'
  AND uses_standard_template = false
  AND custom_mapping_config->'atomicMappings' IS NOT NULL
  AND NOT EXISTS (
      SELECT 1
      FROM jsonb_array_elements(custom_mapping_config->'atomicMappings') m
      WHERE m->>'sourcePath' = 'AIP.3'
  );

DO $$
DECLARE v_count INTEGER;
BEGIN
    GET DIAGNOSTICS v_count = ROW_COUNT;
    RAISE NOTICE 'V121 complete: % SIU interface_message_mappings row(s) updated (AIP.3 actor.display injected)', v_count;
END $$;

COMMIT;
