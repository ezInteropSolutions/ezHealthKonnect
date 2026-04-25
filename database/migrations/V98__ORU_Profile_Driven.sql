-- ============================================================================
-- V98: ORU^R01 — Profile-driven assembly (v2.0)
--
-- Upgrades the existing ORU^R01 template from v1.0 flat-mappings format to
-- v2.0 extended format, adding a "procedure" composite that calls the
-- named assembly function "assembleObservationsFromOBX".
--
-- The procedure replaces the hardcoded AssembleORUObservations block in
-- hl7_fhir_transform_service_v3.go (lines 289-306). After this migration is
-- applied and verified, that block can be removed.
--
-- Resources produced:
--   Patient         — from PID field mappings (unchanged from v1.0)
--   DiagnosticReport — from OBR field mappings (unchanged from v1.0)
--   Observation[]   — from OBX segments via assembleObservationsFromOBX
--   DocumentReference (optional) — ED/RP attachment OBX → liftEDAttachments
-- ============================================================================

-- ── 1. Upgrade ORU^R01 to v2.0 profile format ────────────────────────────────
INSERT INTO hl7_fhir_templates (
    message_type,
    template_name,
    template_description,
    profile_version,
    template_config,
    is_default
) VALUES (
    'ORU^R01',
    'Standard ORU R01 Mapping',
    'HL7 ORU^R01 Observation Result — profile-driven assembly with OBX grouping, NTE accumulation, ED/RP attachments, and base64 binary handling',
    '2.0',
    $PROFILE$
{
  "messageType": "ORU^R01",
  "version": "2.0",
  "valueSets": {
    "oruObsStatus": {
      "F": "final",
      "P": "preliminary",
      "C": "corrected",
      "W": "entered-in-error",
      "X": "cancelled",
      "I": "registered",
      "R": "partial",
      "S": "partial",
      "U": "final",
      "D": "entered-in-error"
    },
    "oruDRStatus": {
      "F": "final",
      "P": "preliminary",
      "C": "amended",
      "R": "partial",
      "X": "cancelled",
      "I": "registered",
      "S": "partial",
      "U": "final"
    }
  },
  "mappings": {
    "Patient": {
      "resourceType": "Patient",
      "idFrom": "PID.3",
      "mappings": [
        { "hl7Path": "PID.3",   "fhirPath": "id" },
        { "hl7Path": "PID.5.1", "fhirPath": "name[0].family" },
        { "hl7Path": "PID.5.2", "fhirPath": "name[0].given[0]" },
        { "hl7Path": "PID.5.3", "fhirPath": "name[0].given[1]" },
        { "hl7Path": "PID.7",   "fhirPath": "birthDate",       "transform": "hl7date" },
        { "hl7Path": "PID.8",   "fhirPath": "gender",          "transform": "gender" },
        { "hl7Path": "PID.11.1","fhirPath": "address[0].line[0]" },
        { "hl7Path": "PID.11.3","fhirPath": "address[0].city" },
        { "hl7Path": "PID.11.4","fhirPath": "address[0].state" },
        { "hl7Path": "PID.11.5","fhirPath": "address[0].postalCode" },
        { "hl7Path": "PID.13.4","fhirPath": "telecom[0].value", "transform": "phone" }
      ]
    },
    "DiagnosticReport": {
      "resourceType": "DiagnosticReport",
      "idFrom": "OBR.3 ?? OBR.2",
      "mappings": [
        { "hl7Path": "OBR.25",  "fhirPath": "status",
          "transform": "lookup:oruDRStatus",
          "fallbackTransform": "code", "required": true },
        { "hl7Path": "OBR.4",   "fhirPath": "code.text" },
        { "hl7Path": "OBR.4.1", "fhirPath": "code.coding[0].code" },
        { "hl7Path": "OBR.4.2", "fhirPath": "code.coding[0].display" },
        { "hl7Path": "OBR.7",   "fhirPath": "effectiveDateTime", "transform": "hl7datetime" },
        { "hl7Path": "OBR.8",   "fhirPath": "effectivePeriod.end", "transform": "hl7datetime",
          "condition": "OBR.8 notempty" },
        { "hl7Path": "OBR.22",  "fhirPath": "issued",  "transform": "hl7datetime" },
        { "hl7Path": "OBR.24",  "fhirPath": "category[0].coding[0].code" },
        { "hl7Path": "OBR.3",   "fhirPath": "identifier[0].value" },
        { "hl7Path": "OBR.2",   "fhirPath": "identifier[1].value",
          "condition": "OBR.2 notempty" }
      ],
      "composites": [
        {
          "fhir":      "_procedures",
          "procedure": "assembleObservationsFromOBX",
          "params": {
            "collapseTextOBX":        true,
            "linkToDiagnosticReport": true,
            "liftEDAttachments":      true
          }
        }
      ]
    }
  }
}
$PROFILE$,
    true
)
ON CONFLICT (message_type, hl7_version, is_default) DO UPDATE
    SET template_config      = EXCLUDED.template_config,
        template_name        = EXCLUDED.template_name,
        template_description = EXCLUDED.template_description,
        profile_version      = EXCLUDED.profile_version,
        updated_at           = NOW();

-- ── 2. ORU variant aliases (R02-R30 are rare but encountered in the wild) ────
-- These share the same profile as ORU^R01; event differences are structural
-- (same OBR/OBX pattern) so no per-variant overrides are needed.

DO $$
DECLARE
    v_config TEXT;
    v_variants TEXT[] := ARRAY['ORU^R02','ORU^R03','ORU^R30','ORU^R31','ORU^R32'];
    v_event   TEXT;
BEGIN
    SELECT template_config INTO v_config
    FROM hl7_fhir_templates
    WHERE message_type = 'ORU^R01' AND is_default = true
    LIMIT 1;

    IF v_config IS NULL THEN
        RAISE NOTICE 'ORU^R01 base profile not found; skipping variant aliases';
        RETURN;
    END IF;

    FOREACH v_event IN ARRAY v_variants LOOP
        INSERT INTO hl7_fhir_templates (
            message_type, template_name, template_description,
            profile_version, template_config, is_default
        ) VALUES (
            v_event,
            'ORU ' || split_part(v_event, '^', 2) || ' Variant',
            'ORU variant — shares ORU^R01 base profile',
            '2.0',
            v_config::JSONB || jsonb_build_object('messageType', v_event),
            true
        )
        ON CONFLICT (message_type, hl7_version, is_default) DO UPDATE
            SET template_config  = EXCLUDED.template_config,
                profile_version  = EXCLUDED.profile_version,
                updated_at       = NOW();

        RAISE NOTICE 'Upserted ORU alias: %', v_event;
    END LOOP;
END $$;

-- ── 3. Verification ──────────────────────────────────────────────────────────
DO $$
DECLARE v_count INT;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM hl7_fhir_templates
    WHERE message_type LIKE 'ORU%' AND profile_version = '2.0';
    RAISE NOTICE '✅ V98: % ORU profiles seeded with extended format (v2.0)', v_count;
END $$;
