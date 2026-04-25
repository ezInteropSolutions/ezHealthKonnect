-- ============================================================================
-- V102: MFN Master File Notifications — Profile-driven assembly (v2.0)
--
-- Creates v2.0 profiles for the most common MFN trigger events.
-- The "procedure" composite calls "assembleMFNResources" which:
--   - Reads MFI.1 to determine master file type:
--       STF + optional PRA → Practitioner
--       LOC             → Location
--       CDM             → ChargeItemDefinition
--       OM1             → ObservationDefinition (lab catalog)
--       MFE.1 action tag added as meta.tag to all produced resources
--   - Tier 2: Per-interface MFIOverrides config from Z-segment config table
--   - Tier 3: Z-segment config rules for custom resource shapes
--   - Produces one or more FHIR resources per MFE/payload segment group
--
-- MFN^Mxx events covered:
--   M02  Staff/practitioner master file
--   M04  Charge description master file (CDM)
--   M05  Location master file (LOC)
--   M06  Clinical study master (generic / ObservationDefinition fallback)
--   M12  Master file for laboratory observations (OM1)
--   M13  Generic master file (fallback Organization)
--
-- Replaces the hardcoded AssembleMFNResources block in
-- hl7_fhir_transform_service_v3.go (lines 239-268).
-- ============================================================================

-- ── Helper: shared profile body ──────────────────────────────────────────────
-- All MFN variants share identical assembly logic; only the "messageType" field
-- in the JSON differs. We seed MFN^M02 first, then alias the rest.

INSERT INTO hl7_fhir_templates (
    message_type,
    template_name,
    template_description,
    profile_version,
    template_config,
    is_default
) VALUES (
    'MFN^M02',
    'Standard MFN M02 Mapping',
    'HL7 MFN^M02 Staff/Practitioner Master File — profile-driven assembly dispatching on MFI.1 to Practitioner, Location, ChargeItemDefinition, or generic Organization',
    '2.0',
    $PROFILE$
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
    }
  },
  "mappings": {
    "_MFN": {
      "resourceType": "_MFN",
      "mappings": [],
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
    true
)
ON CONFLICT (message_type, hl7_version, is_default) DO UPDATE
    SET template_config      = EXCLUDED.template_config,
        template_name        = EXCLUDED.template_name,
        template_description = EXCLUDED.template_description,
        profile_version      = EXCLUDED.profile_version,
        updated_at           = NOW();

-- ── MFN variant aliases ───────────────────────────────────────────────────────
DO $$
DECLARE
    v_config TEXT;
    v_variants TEXT[] := ARRAY[
        'MFN^M04','MFN^M05','MFN^M06',
        'MFN^M07','MFN^M08','MFN^M09','MFN^M10',
        'MFN^M12','MFN^M13','MFN^M15','MFN^M16'
    ];
    v_event TEXT;
    v_names JSONB := '{
        "MFN^M04": "MFN M04 Charge Description Master",
        "MFN^M05": "MFN M05 Location Master",
        "MFN^M06": "MFN M06 Clinical Study Master",
        "MFN^M07": "MFN M07 Clinical Study Observation Master",
        "MFN^M08": "MFN M08 Test/Observation Numeric",
        "MFN^M09": "MFN M09 Test/Observation Categorical",
        "MFN^M10": "MFN M10 Test/Observation Batteries",
        "MFN^M12": "MFN M12 Lab Observation Master",
        "MFN^M13": "MFN M13 Syndromic Surveillance Master",
        "MFN^M15": "MFN M15 Inventory Item Master",
        "MFN^M16": "MFN M16 Inventory Item Master with Packaging Variants"
    }'::JSONB;
BEGIN
    SELECT template_config INTO v_config
    FROM hl7_fhir_templates
    WHERE message_type = 'MFN^M02' AND is_default = true
    LIMIT 1;

    IF v_config IS NULL THEN
        RAISE NOTICE 'MFN^M02 base profile not found; skipping alias creation';
        RETURN;
    END IF;

    FOREACH v_event IN ARRAY v_variants LOOP
        INSERT INTO hl7_fhir_templates (
            message_type, template_name, template_description,
            profile_version, template_config, is_default
        ) VALUES (
            v_event,
            COALESCE(v_names->>v_event, 'MFN ' || split_part(v_event, '^', 2) || ' Variant'),
            'MFN master file variant — shares MFN^M02 base profile',
            '2.0',
            v_config::JSONB || jsonb_build_object('messageType', v_event),
            true
        )
        ON CONFLICT (message_type, hl7_version, is_default) DO UPDATE
            SET template_config  = EXCLUDED.template_config,
                profile_version  = EXCLUDED.profile_version,
                updated_at       = NOW();

        RAISE NOTICE 'Upserted MFN alias: %', v_event;
    END LOOP;
END $$;

-- ── Verification ─────────────────────────────────────────────────────────────
DO $$
DECLARE v_count INT;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM hl7_fhir_templates
    WHERE message_type LIKE 'MFN%' AND profile_version = '2.0';
    RAISE NOTICE '✅ V102: % MFN profiles seeded with extended format (v2.0)', v_count;
END $$;
