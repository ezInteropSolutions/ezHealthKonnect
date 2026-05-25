-- V123: SIU complete IG-aligned template
--
-- Replaces V122 template (Appointment-only, wrong paths) with a complete
-- HL7 v2-to-FHIR R4 IG-aligned template covering:
--   MessageHeader  (MSH)
--   Patient        (PID)
--   Appointment    (SCH + AIS + AIP/AIL/AIG composites)
--
-- Fixes applied vs V122:
--   1. SCH.6 → serviceType      (ce_to_codeableconcept, NOT description string)
--   2. SCH.7 → reasonCode       (ce_to_codeableconcept, NOT reasonCode[0].text)
--   3. SCH.8 → appointmentType  (ce_to_codeableconcept, NOT appointmentType.text)
--   4. SCH.11.4/5 → start/end   (hl7_timestamp_to_fhir_datetime, NOT hl7datetime alias)
--   5. Status lookup covers all Table 0278 codes incl. "SCHEDULED" / "Scheduled"
--   6. AIS.3 → serviceType and AIS.10 → participant.status added as flat mappings
--   7. Composite participant actor uses display-only (no invalid ^-in-URL reference)
--   8. MessageHeader (MSH) and Patient (PID) resource sections added back

BEGIN;

CREATE TEMP TABLE v123_siu_base AS
SELECT ('{
  "messageType": "SIU^S12",
  "version": "2.0",
  "valueSets": {
    "siuFillerStatus": {
      "PENDING":        "pending",
      "WAITLIST":       "waitlist",
      "BOOKED":         "booked",
      "SCHEDULED":      "booked",
      "CONFIRMED":      "booked",
      "ARRIVED":        "arrived",
      "COMPLETE":       "fulfilled",
      "FULFILLED":      "fulfilled",
      "CANCELLED":      "cancelled",
      "DELETED":        "cancelled",
      "DISCONTINUED":   "cancelled",
      "NOSHOW":         "noshow",
      "ENTERED-IN-ERROR":"entered-in-error"
    },
    "siuEventStatus": {
      "S12": "booked",
      "S13": "booked",
      "S14": "booked",
      "S15": "cancelled",
      "S16": "cancelled",
      "S17": "cancelled",
      "S26": "noshow"
    },
    "participantStatus": {
      "AC": "accepted",
      "NA": "declined",
      "NI": "tentative",
      "UN": "needs-action"
    }
  },
  "mappings": {
    "MessageHeader": {
      "resourceType": "MessageHeader",
      "mappings": [
        {"hl7Path": "MSH.3",  "fhirPath": "source.name",                "transform": "string_direct"},
        {"hl7Path": "MSH.4",  "fhirPath": "source.endpoint",            "transform": "string_direct"},
        {"hl7Path": "MSH.5",  "fhirPath": "destination[0].name",        "transform": "string_direct"},
        {"hl7Path": "MSH.6",  "fhirPath": "destination[0].endpoint",    "transform": "string_direct"},
        {"hl7Path": "MSH.9",  "fhirPath": "eventCoding",                "transform": "msh9_to_coding"},
        {"hl7Path": "MSH.10", "fhirPath": "id",                         "transform": "string_direct"},
        {"hl7Path": "MSH.11", "fhirPath": "meta.security[0].code",      "transform": "string_direct"}
      ]
    },
    "Patient": {
      "resourceType": "Patient",
      "mappings": [
        {"hl7Path": "PID.3",  "fhirPath": "identifier",    "transform": "cx_to_identifier",            "required": true},
        {"hl7Path": "PID.5",  "fhirPath": "name",          "transform": "xpn_to_humanname",            "required": true},
        {"hl7Path": "PID.7",  "fhirPath": "birthDate",     "transform": "hl7_timestamp_to_fhir_date"},
        {"hl7Path": "PID.8",  "fhirPath": "gender",        "transform": "hl7_table_0001_gender"},
        {"hl7Path": "PID.11", "fhirPath": "address",       "transform": "xad_to_address"},
        {"hl7Path": "PID.13", "fhirPath": "telecom",       "transform": "xtn_to_contactpoint"},
        {"hl7Path": "PID.16", "fhirPath": "maritalStatus", "transform": "ce_to_codeableconcept"},
        {"hl7Path": "PID.18", "fhirPath": "identifier",    "transform": "account_to_identifier"}
      ]
    },
    "Appointment": {
      "resourceType": "Appointment",
      "idFrom": "SCH.2 ?? SCH.1",
      "mappings": [
        {
          "hl7Path": "SCH.25",
          "fhirPath": "status",
          "transform": "lookup:siuFillerStatus",
          "fallbackTransform": "lookup:siuEventStatus",
          "fallbackField": "messageEvent",
          "required": true
        },
        {"hl7Path": "SCH.6",    "fhirPath": "serviceType",           "transform": "ce_to_codeableconcept"},
        {"hl7Path": "SCH.7",    "fhirPath": "reasonCode",            "transform": "ce_to_codeableconcept"},
        {"hl7Path": "SCH.8",    "fhirPath": "appointmentType",       "transform": "ce_to_codeableconcept"},
        {"hl7Path": "SCH.9",    "fhirPath": "minutesDuration",       "transform": "integer"},
        {"hl7Path": "SCH.11.4", "fhirPath": "start",                 "transform": "hl7_timestamp_to_fhir_datetime"},
        {"hl7Path": "SCH.11.5", "fhirPath": "end",                   "transform": "hl7_timestamp_to_fhir_datetime"},
        {"hl7Path": "SCH.11",   "fhirPath": "start",                 "transform": "hl7_timestamp_to_fhir_datetime", "condition": "SCH.11.4 empty"},
        {"hl7Path": "SCH.12",   "fhirPath": "end",                   "transform": "hl7_timestamp_to_fhir_datetime", "condition": "SCH.11.5 empty"},
        {"hl7Path": "AIS.3",    "fhirPath": "serviceType",           "transform": "ce_to_codeableconcept"},
        {"hl7Path": "AIS.10",   "fhirPath": "participant[0].status", "transform": "lookup:participantStatus"}
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
            "actor":  {"display": "{{AIP.3}}"},
            "type": [{"coding": [{"system": "http://terminology.hl7.org/CodeSystem/v3-ParticipationType", "code": "PPRF", "display": "Primary performer"}]}],
            "status": "accepted"
          }
        },
        {
          "fhir":    "participant",
          "forEach": "AIL",
          "value": {
            "actor":  {"display": "{{AIL.3}}"},
            "type": [{"coding": [{"system": "http://terminology.hl7.org/CodeSystem/v3-ParticipationType", "code": "LOC", "display": "Location"}]}],
            "status": "accepted"
          }
        },
        {
          "fhir":    "participant",
          "forEach": "AIG",
          "value": {
            "actor":  {"display": "{{AIG.3}}"},
            "type": [{"coding": [{"system": "http://terminology.hl7.org/CodeSystem/v3-ParticipationType", "code": "PART", "display": "Participant"}]}],
            "status": "accepted"
          }
        }
      ]
    }
  }
}')::jsonb AS config;

-- Update all SIU^S12 is_default rows
UPDATE hl7_fhir_templates
SET    template_config = (SELECT config FROM v123_siu_base),
       profile_version = '2.0',
       updated_at      = NOW()
WHERE  message_type = 'SIU^S12' AND is_default = true;

-- Update all SIU variant rows (swap messageType key only)
UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v123_siu_base), '{messageType}', '"SIU^S13"'),
       profile_version = '2.0', updated_at = NOW()
WHERE  message_type = 'SIU^S13' AND is_default = true;

UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v123_siu_base), '{messageType}', '"SIU^S14"'),
       profile_version = '2.0', updated_at = NOW()
WHERE  message_type = 'SIU^S14' AND is_default = true;

UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v123_siu_base), '{messageType}', '"SIU^S15"'),
       profile_version = '2.0', updated_at = NOW()
WHERE  message_type = 'SIU^S15' AND is_default = true;

UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v123_siu_base), '{messageType}', '"SIU^S16"'),
       profile_version = '2.0', updated_at = NOW()
WHERE  message_type = 'SIU^S16' AND is_default = true;

UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v123_siu_base), '{messageType}', '"SIU^S17"'),
       profile_version = '2.0', updated_at = NOW()
WHERE  message_type = 'SIU^S17' AND is_default = true;

UPDATE hl7_fhir_templates
SET    template_config = jsonb_set((SELECT config FROM v123_siu_base), '{messageType}', '"SIU^S26"'),
       profile_version = '2.0', updated_at = NOW()
WHERE  message_type = 'SIU^S26' AND is_default = true;

DROP TABLE v123_siu_base;

DO $$
DECLARE v_count INT;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM   hl7_fhir_templates
    WHERE  message_type LIKE 'SIU%'
      AND  is_default = true
      AND  template_config->'mappings'->'MessageHeader' IS NOT NULL;
    RAISE NOTICE 'V123 complete: % SIU rows now have MessageHeader + Patient + Appointment (IG-aligned)', v_count;
END $$;

COMMIT;
