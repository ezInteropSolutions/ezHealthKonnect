-- ============================================================================
-- V100: VXU^V04 — Profile-driven assembly (v2.0)
--
-- Creates a v2.0 profile for VXU^V04 (Unsolicited Vaccination Record Update).
-- The "procedure" composite calls "assembleVXUImmunizations" which:
--   - Groups RXA segments with co-located RXR (route/site) by position
--   - Correlates OBX sub-groups by OBX.4 (sub-ID) for LOINC-dispatched fields:
--       30973-2  → protocolApplied.doseNumber
--       59782-3  → protocolApplied.seriesDoses
--       59783-1  → protocolApplied.targetDisease
--       69764-9  → vaccinationProtocol.authority / VIS publication date
--       29768-9  → VIS presentation date
--       29769-7  → VIS presentation date (alternate code)
--       64994-7  → fundingEligibility
--   - Maps RXA.20 refusal reason → statusReason
--   - Maps RXA.18 → reaction[]
--   - Produces one Immunization resource per RXA group
--
-- Replaces the hardcoded AssembleVXUImmunizations block in
-- hl7_fhir_transform_service_v3.go (lines 270-287).
-- ============================================================================

INSERT INTO hl7_fhir_templates (
    message_type,
    template_name,
    template_description,
    profile_version,
    template_config,
    is_default
) VALUES (
    'VXU^V04',
    'Standard VXU V04 Mapping',
    'HL7 VXU^V04 Unsolicited Vaccination Record — profile-driven assembly with RXA/RXR grouping, OBX LOINC dispatch, reaction tracking, and VIS publication date handling',
    '2.0',
    $PROFILE$
{
  "messageType": "VXU^V04",
  "version": "2.0",
  "valueSets": {
    "vxuCompletionStatus": {
      "CP": "completed",
      "RE": "not-done",
      "NA": "not-done",
      "PA": "not-done"
    },
    "vxuInformationSource": {
      "00": "provider",
      "01": "other-provider",
      "02": "parent",
      "03": "other-registry",
      "04": "school",
      "05": "public-health-agency",
      "06": "birth-certificate",
      "07": "pharmacy",
      "08": "other",
      "09": "patient",
      "10": "payer",
      "11": "other-non-clinical"
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
        { "hl7Path": "PID.7",   "fhirPath": "birthDate", "transform": "hl7date" },
        { "hl7Path": "PID.8",   "fhirPath": "gender",    "transform": "gender" },
        { "hl7Path": "PID.11.1","fhirPath": "address[0].line[0]" },
        { "hl7Path": "PID.11.3","fhirPath": "address[0].city" },
        { "hl7Path": "PID.11.4","fhirPath": "address[0].state" },
        { "hl7Path": "PID.11.5","fhirPath": "address[0].postalCode" }
      ]
    },
    "Immunization": {
      "resourceType": "Immunization",
      "idFrom": "RXA.3",
      "mappings": [],
      "composites": [
        {
          "fhir":      "_procedures",
          "procedure": "assembleVXUImmunizations",
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

-- ── Verification ─────────────────────────────────────────────────────────────
DO $$
DECLARE v_count INT;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM hl7_fhir_templates
    WHERE message_type LIKE 'VXU%' AND profile_version = '2.0';
    RAISE NOTICE '✅ V100: % VXU profiles seeded with extended format (v2.0)', v_count;
END $$;
