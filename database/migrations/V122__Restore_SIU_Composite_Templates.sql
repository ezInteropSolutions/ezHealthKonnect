-- V122: Restore composite-based SIU templates
--
-- Post-V97 schema-generator migrations inserted flat-mapping rows (one per
-- HL7 version 2.3-2.8) for each SIU^Sxx event using the old "resources" key
-- structure. These rows have no composites, so getProfileComposites loads 0
-- rules and Appointment.participant falls back to the synthetic PART entry.
--
-- This migration updates ALL existing is_default SIU^Sxx rows to carry the
-- correct v2.0 composite template_config. Only template_config and
-- profile_version are updated; hl7_version values are preserved.
--
-- Key fix vs V97 original: display uses AIP.3/AIL.3/AIG.3 (the XCN/PL name
-- field) not AIP.4/AIL.4/AIG.4 (the CE type field), so the XCN rescue in
-- the sanitizer produces "Adams, Douglas" not "D".

BEGIN;

-- Step 1: Restore SIU^S12 with the correct composite template.
-- Use a temp table so variants can reference it without repeated literal.
CREATE TEMP TABLE v122_siu_base AS
SELECT ('{
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
        {"hl7Path": "SCH.25",   "fhirPath": "status",              "transform": "lookup:siuFillerStatus", "fallbackTransform": "lookup:siuEventStatus", "fallbackField": "messageEvent", "required": true},
        {"hl7Path": "SCH.11.4", "fhirPath": "start",               "transform": "hl7datetime"},
        {"hl7Path": "SCH.11.5", "fhirPath": "end",                 "transform": "hl7datetime"},
        {"hl7Path": "SCH.11",   "fhirPath": "start",               "transform": "hl7datetime", "condition": "SCH.11.4 empty"},
        {"hl7Path": "SCH.9",    "fhirPath": "minutesDuration",     "transform": "integer"},
        {"hl7Path": "SCH.7",    "fhirPath": "reasonCode[0].text"},
        {"hl7Path": "SCH.8",    "fhirPath": "appointmentType.text"},
        {"hl7Path": "SCH.6",    "fhirPath": "description"}
      ],
      "composites": [
        {
          "fhir":     "identifier",
          "firstOf":  ["SCH.2", "SCH.1"],
          "template": [{"use": "official", "value": "{{_value}}"}]
        },
        {
          "fhir":      "participant",
          "condition": "PID.3",
          "value": {"actor": {"reference": "Patient/{{PID.3}}"}, "status": "accepted", "required": "required"}
        },
        {
          "fhir":    "participant",
          "forEach": "AIP",
          "value":   {"actor": {"reference": "Practitioner/{{AIP.3}}", "display": "{{AIP.3}}"}, "status": "accepted"}
        },
        {
          "fhir":    "participant",
          "forEach": "AIL",
          "value":   {"actor": {"reference": "Location/{{AIL.3}}", "display": "{{AIL.3}}"}, "status": "accepted"}
        },
        {
          "fhir":    "participant",
          "forEach": "AIG",
          "value":   {"actor": {"reference": "Device/{{AIG.3}}", "display": "{{AIG.3}}"}, "status": "accepted"}
        }
      ]
    }
  }
}')::jsonb AS config;

UPDATE hl7_fhir_templates
SET    template_config = (SELECT config FROM v122_siu_base),
       profile_version = '2.0',
       updated_at      = NOW()
WHERE  message_type = 'SIU^S12'
  AND  is_default   = true;

-- Step 2: Update all SIU variants — copy S12 config, swap messageType.
UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v122_siu_base), '{messageType}', '"SIU^S13"'),
       profile_version = '2.0',
       updated_at      = NOW()
WHERE  message_type = 'SIU^S13' AND is_default = true;

UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v122_siu_base), '{messageType}', '"SIU^S14"'),
       profile_version = '2.0',
       updated_at      = NOW()
WHERE  message_type = 'SIU^S14' AND is_default = true;

UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v122_siu_base), '{messageType}', '"SIU^S15"'),
       profile_version = '2.0',
       updated_at      = NOW()
WHERE  message_type = 'SIU^S15' AND is_default = true;

UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v122_siu_base), '{messageType}', '"SIU^S16"'),
       profile_version = '2.0',
       updated_at      = NOW()
WHERE  message_type = 'SIU^S16' AND is_default = true;

UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v122_siu_base), '{messageType}', '"SIU^S17"'),
       profile_version = '2.0',
       updated_at      = NOW()
WHERE  message_type = 'SIU^S17' AND is_default = true;

UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v122_siu_base), '{messageType}', '"SIU^S26"'),
       profile_version = '2.0',
       updated_at      = NOW()
WHERE  message_type = 'SIU^S26' AND is_default = true;

DROP TABLE v122_siu_base;

DO $$
DECLARE v_count INT;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM   hl7_fhir_templates
    WHERE  message_type LIKE 'SIU%'
      AND  is_default   = true
      AND  template_config->'mappings' IS NOT NULL;
    RAISE NOTICE 'V122 complete: % SIU template rows now carry composite mappings', v_count;
END $$;

COMMIT;
