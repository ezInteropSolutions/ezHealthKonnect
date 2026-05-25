-- ============================================================================
-- V97: Extended Mapping Profiles
--
-- Extends the hl7_fhir_templates.template_config JSON schema to support:
--   "composites"  — structural assembly rules (forEach, condition, firstOf)
--   "valueSets"   — inline code lookup tables
--   "idFrom"      — fallback chain for resource ID fields
--
-- This migration seeds the SIU^S12 profile as the first "profile-driven"
-- template, replacing the Go siu_assembly.go hardcoded assembly.
--
-- The existing ADT^A01 and ORU^R01 templates are untouched; they use the
-- plain "mappings" format and continue to work as before.
-- ============================================================================

-- ── 1. Add profile_version column to track which format variant is in use ──
ALTER TABLE hl7_fhir_templates
    ADD COLUMN IF NOT EXISTS profile_version VARCHAR(10) DEFAULT '1.0';

COMMENT ON COLUMN hl7_fhir_templates.profile_version IS
    '1.0 = original flat mappings format; 2.0 = extended format with composites+valueSets';

-- ── 2. SIU^S12 — Appointment scheduling ─────────────────────────────────────
--
-- Replaces services/hl7assembly/siu_assembly.go
--
-- Resources produced:
--   Appointment  — primary (from SCH)
--   Patient      — enriched by field-mapping pass (PID), referenced in participant
--
-- Participant composites:
--   condition PID.3    → patient participant (required)
--   forEach   AIP      → practitioner participants
--   forEach   AIL      → location participants
--   forEach   AIG      → general resource participants
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO hl7_fhir_templates (
    message_type,
    template_name,
    template_description,
    profile_version,
    template_config,
    is_default
) VALUES (
    'SIU^S12',
    'Standard SIU S12 Mapping',
    'HL7 SIU^S12 Appointment scheduling — profile-driven assembly (replaces Go siu_assembly)',
    '2.0',
    $PROFILE$
{
  "messageType": "SIU^S12",
  "version": "2.0",
  "valueSets": {
    "siuFillerStatus": {
      "PENDING":      "pending",
      "WAITLIST":     "pending",
      "BOOKED":       "booked",
      "CONFIRMED":    "booked",
      "ARRIVED":      "arrived",
      "FULFILLED":    "fulfilled",
      "COMPLETE":     "fulfilled",
      "CANCELLED":    "cancelled",
      "DELETED":      "cancelled",
      "DISCONTINUED": "cancelled",
      "NOSHOW":       "noshow"
    },
    "siuEventStatus": {
      "S12": "booked",
      "S13": "booked",
      "S14": "booked",
      "S15": "cancelled",
      "S16": "cancelled",
      "S17": "cancelled",
      "S26": "noshow"
    }
  },
  "mappings": {
    "Appointment": {
      "resourceType": "Appointment",
      "idFrom": "SCH.2 ?? SCH.1",
      "mappings": [
        {
          "hl7Path":  "SCH.25",
          "fhirPath": "status",
          "transform": "lookup:siuFillerStatus",
          "fallbackTransform": "lookup:siuEventStatus",
          "fallbackField": "messageEvent",
          "required": true
        },
        {
          "hl7Path":  "SCH.11.4",
          "fhirPath": "start",
          "transform": "hl7datetime"
        },
        {
          "hl7Path":  "SCH.11.5",
          "fhirPath": "end",
          "transform": "hl7datetime"
        },
        {
          "hl7Path":  "SCH.11",
          "fhirPath": "start",
          "transform": "hl7datetime",
          "condition": "SCH.11.4 empty"
        },
        {
          "hl7Path":  "SCH.9",
          "fhirPath": "minutesDuration",
          "transform": "integer"
        },
        {
          "hl7Path":  "SCH.7",
          "fhirPath": "reasonCode[0].text"
        },
        {
          "hl7Path":  "SCH.8",
          "fhirPath": "appointmentType.text"
        },
        {
          "hl7Path":  "SCH.6",
          "fhirPath": "description"
        }
      ],
      "composites": [
        {
          "fhir":      "identifier",
          "firstOf":   ["SCH.2", "SCH.1"],
          "template":  [{"use": "official", "value": "{{_value}}"}]
        },
        {
          "fhir":      "participant",
          "condition": "PID.3",
          "value": {
            "actor":    {"reference": "Patient/{{PID.3}}"},
            "status":   "accepted",
            "required": "required"
          }
        },
        {
          "fhir":    "participant",
          "forEach": "AIP",
          "value": {
            "actor":  {"reference": "Practitioner/{{AIP.3}}", "display": "{{AIP.4}}"},
            "status": "accepted"
          }
        },
        {
          "fhir":    "participant",
          "forEach": "AIL",
          "value": {
            "actor":  {"reference": "Location/{{AIL.3}}", "display": "{{AIL.4}}"},
            "status": "accepted"
          }
        },
        {
          "fhir":    "participant",
          "forEach": "AIG",
          "value": {
            "actor":  {"reference": "Device/{{AIG.3}}", "display": "{{AIG.4}}"},
            "status": "accepted"
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
    SET template_config  = EXCLUDED.template_config,
        template_name    = EXCLUDED.template_name,
        template_description = EXCLUDED.template_description,
        profile_version  = EXCLUDED.profile_version,
        updated_at       = NOW();

-- ── 3. SIU variant aliases — same profile covers all SIU events ──────────────
-- S13=reschedule, S14=modify, S15=cancel, S17=delete, S26=no-show
-- Each alias points to the same extended profile (event-code differences
-- are resolved via siuEventStatus valueSet when SCH.25 is absent).

DO $$
DECLARE
    v_config  TEXT;
    v_events  TEXT[] := ARRAY['SIU^S13','SIU^S14','SIU^S15','SIU^S16','SIU^S17','SIU^S26'];
    v_event   TEXT;
    v_names   JSONB  := '{
        "SIU^S13": "SIU S13 Appointment Rescheduling",
        "SIU^S14": "SIU S14 Appointment Modification",
        "SIU^S15": "SIU S15 Appointment Cancellation",
        "SIU^S16": "SIU S16 Cancellation with Reason",
        "SIU^S17": "SIU S17 Appointment Deletion",
        "SIU^S26": "SIU S26 Patient No-Show"
    }'::JSONB;
BEGIN
    SELECT template_config INTO v_config
    FROM hl7_fhir_templates
    WHERE message_type = 'SIU^S12' AND is_default = true
    LIMIT 1;

    IF v_config IS NULL THEN
        RAISE NOTICE 'SIU^S12 base profile not found; skipping alias creation';
        RETURN;
    END IF;

    FOREACH v_event IN ARRAY v_events LOOP
        INSERT INTO hl7_fhir_templates (
            message_type, template_name, template_description,
            profile_version, template_config, is_default
        ) VALUES (
            v_event,
            v_names->>v_event,
            'SIU scheduling variant — shares SIU^S12 base profile',
            '2.0',
            v_config::JSONB || jsonb_build_object('messageType', v_event),
            true
        )
        ON CONFLICT (message_type, hl7_version, is_default) DO UPDATE
            SET template_config  = EXCLUDED.template_config,
                profile_version  = EXCLUDED.profile_version,
                updated_at       = NOW();

        RAISE NOTICE 'Upserted SIU alias: %', v_event;
    END LOOP;
END $$;

-- ── 4. Verification ──────────────────────────────────────────────────────────
DO $$
DECLARE
    v_count INT;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM hl7_fhir_templates
    WHERE message_type LIKE 'SIU%' AND profile_version = '2.0';

    RAISE NOTICE '✅ V97: % SIU profiles seeded with extended format (v2.0)', v_count;

    SELECT COUNT(*) INTO v_count
    FROM hl7_fhir_templates;

    RAISE NOTICE '📊 V97: total templates in hl7_fhir_templates: %', v_count;
END $$;
