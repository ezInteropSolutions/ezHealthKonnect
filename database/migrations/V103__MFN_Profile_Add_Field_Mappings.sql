-- ============================================================================
-- V103: MFN Profiles — Add field-level mappings for wizard display
--
-- The V102 MFN v2.0 profile had an empty _MFN.mappings array, which meant:
--   - atomicMappings is always [] in the transform response
--   - If assembleMFNResources also returns empty (no MFI segments in test msg),
--     the wizard has nothing to show and stays blank
--
-- Fix: Replace the synthetic _MFN entry with actual resource definitions that
-- produce field-level mappings from MSH (always present) and MFI (when present).
-- The MessageHeader resource guarantees at least one mapping per message.
-- The assembly procedure (assembleMFNResources) still runs and adds
-- Practitioner/Location/Organization on top.
-- ============================================================================

-- ── Update MFN^M02 base profile with field-level mappings ────────────────────
UPDATE hl7_fhir_templates
SET
    template_config  = $PROFILE$
{
  "messageType": "MFN^M02",
  "version": "2.0",
  "valueSets": {
    "mfeAction": {
      "MAD": "add",
      "MDL": "delete",
      "MUP": "update",
      "MDC": "deactivate",
      "MAC": "reactivate"
    },
    "stfEmployeeType": {
      "PN": "permanent",
      "TM": "temporary",
      "CB": "contract",
      "OF": "other-facility"
    },
    "stfStatus": {
      "A": "active",
      "I": "inactive",
      "P": "pending-review"
    }
  },
  "mappings": {
    "MessageHeader": {
      "resourceType": "MessageHeader",
      "idFrom": "MSH.10",
      "mappings": [
        { "hl7Path": "MSH.9.1",  "fhirPath": "event.code",        "required": true },
        { "hl7Path": "MSH.9.2",  "fhirPath": "event.display" },
        { "hl7Path": "MSH.3",    "fhirPath": "source.name" },
        { "hl7Path": "MSH.4",    "fhirPath": "source.endpoint" },
        { "hl7Path": "MSH.5",    "fhirPath": "destination[0].name" },
        { "hl7Path": "MSH.7",    "fhirPath": "timestamp",          "transform": "hl7datetime" },
        { "hl7Path": "MSH.10",   "fhirPath": "id" }
      ],
      "composites": [
        {
          "fhir":      "_procedures",
          "procedure": "assembleMFNResources",
          "params":    {}
        }
      ]
    }
  }
}
$PROFILE$,
    template_description = 'HL7 MFN^M02 Staff/Practitioner Master File — profile-driven assembly dispatching on MFI.1 to Practitioner, Location, ChargeItemDefinition, or generic Organization. MSH→MessageHeader and STF→Practitioner field mappings guarantee wizard display.',
    updated_at           = NOW()
WHERE message_type = 'MFN^M02'
  AND is_default   = true;

-- ── Re-derive aliases from the updated MFN^M02 base ─────────────────────────
DO $$
DECLARE
    v_config TEXT;
    v_variants TEXT[] := ARRAY[
        'MFN^M04','MFN^M05','MFN^M06',
        'MFN^M07','MFN^M08','MFN^M09','MFN^M10',
        'MFN^M12','MFN^M13','MFN^M15','MFN^M16'
    ];
    v_event TEXT;
BEGIN
    SELECT template_config INTO v_config
    FROM hl7_fhir_templates
    WHERE message_type = 'MFN^M02' AND is_default = true
    LIMIT 1;

    IF v_config IS NULL THEN
        RAISE NOTICE 'MFN^M02 updated profile not found; skipping alias refresh';
        RETURN;
    END IF;

    FOREACH v_event IN ARRAY v_variants LOOP
        UPDATE hl7_fhir_templates
        SET    template_config = v_config::JSONB || jsonb_build_object('messageType', v_event),
               updated_at      = NOW()
        WHERE  message_type = v_event
          AND  is_default   = true;

        RAISE NOTICE 'Refreshed MFN alias: % (rows: %)', v_event, (SELECT COUNT(*) FROM hl7_fhir_templates WHERE message_type = v_event AND is_default = true);
    END LOOP;
END $$;

-- ── Verification ─────────────────────────────────────────────────────────────
DO $$
DECLARE
    v_count     INT;
    v_has_stf   BOOLEAN;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM hl7_fhir_templates
    WHERE message_type LIKE 'MFN%' AND profile_version = '2.0';

    SELECT (template_config::jsonb->'mappings'->'Practitioner'->'mappings') IS NOT NULL
    INTO v_has_stf
    FROM hl7_fhir_templates
    WHERE message_type = 'MFN^M02' AND is_default = true;

    RAISE NOTICE '✅ V103: % MFN profiles updated; MFN^M02 has STF field mappings: %', v_count, v_has_stf;
END $$;
