-- ============================================================================
-- V99: MDM^T01-T11 — Profile-driven assembly (v2.0)
--
-- Creates v2.0 profiles for all MDM trigger events. The "procedure" composite
-- calls "buildDocumentReferenceFromTXA" which:
--   - Maps TXA fields to DocumentReference
--   - Accumulates OBX.5 TX/FT segments (NTE continuation) into content[].attachment.data
--   - Promotes OBX.5 ED binary to DocumentReference.content[].attachment with base64 encoding
--   - Promotes OBX.5 RP reference pointer to DocumentReference.content[].attachment.url
--   - Removes raw Observation resources produced by field mappings that have
--     been absorbed into the DocumentReference
--
-- Replaces the hardcoded AssembleMDMDocument block in
-- hl7_fhir_transform_service_v3.go (lines 187-205).
-- ============================================================================

-- ── MDM^T01 — Document notification ─────────────────────────────────────────
INSERT INTO hl7_fhir_templates (
    message_type,
    template_name,
    template_description,
    profile_version,
    template_config,
    is_default
) VALUES (
    'MDM^T01',
    'Standard MDM T01 Mapping',
    'HL7 MDM^T01 Document Notification — profile-driven assembly with TXA→DocumentReference, OBX attachment handling, NTE text accumulation, ED binary base64, and RP reference pointers',
    '2.0',
    $PROFILE$
{
  "messageType": "MDM^T01",
  "version": "2.0",
  "valueSets": {
    "txaDocStatus": {
      "AU": "current",
      "CA": "superseded",
      "DI": "entered-in-error",
      "DO": "current",
      "IN": "preliminary",
      "IP": "preliminary",
      "LA": "current",
      "OB": "superseded",
      "UN": "preliminary",
      "PA": "preliminary"
    },
    "txaAvailability": {
      "AV": "available",
      "CA": "cancelled",
      "OB": "obsolete",
      "UN": "unavailable"
    },
    "txaDocType": {
      "11506-3": "Consultation note",
      "11524-6": "EKG study",
      "11526-1": "Pathology study",
      "18748-4": "Diagnostic imaging study",
      "34117-2": "History and physical note",
      "34133-9": "Summarization of episode note",
      "34748-4": "Telephone encounter note",
      "51845-6": "Outpatient consultation note",
      "57133-1": "Referral note",
      "57134-9": "Nursing facility note",
      "59258-4": "Emergency department note"
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
        { "hl7Path": "PID.8",   "fhirPath": "gender",    "transform": "gender" }
      ]
    },
    "DocumentReference": {
      "resourceType": "DocumentReference",
      "idFrom": "TXA.12 ?? TXA.16",
      "mappings": [
        { "hl7Path": "TXA.17", "fhirPath": "status",
          "transform": "lookup:txaAvailability", "required": true },
        { "hl7Path": "TXA.19", "fhirPath": "docStatus",
          "transform": "lookup:txaDocStatus" },
        { "hl7Path": "TXA.2",  "fhirPath": "type.text" },
        { "hl7Path": "TXA.2.1","fhirPath": "type.coding[0].code" },
        { "hl7Path": "TXA.2.2","fhirPath": "type.coding[0].display" },
        { "hl7Path": "TXA.4",  "fhirPath": "date", "transform": "hl7datetime" },
        { "hl7Path": "TXA.6",  "fhirPath": "authenticator.reference",
          "transform": "reference:Practitioner" },
        { "hl7Path": "TXA.12", "fhirPath": "identifier[0].value" },
        { "hl7Path": "TXA.16", "fhirPath": "identifier[1].value",
          "condition": "TXA.16 notempty" }
      ],
      "composites": [
        {
          "fhir":      "_procedures",
          "procedure": "buildDocumentReferenceFromTXA",
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

-- ── MDM variant aliases T02-T11 ───────────────────────────────────────────────
-- All MDM events share the same assembly logic (TXA + OBX pattern).
-- Event semantics (new doc, update, cancel, etc.) are captured in TXA fields;
-- no per-event override is needed in the profile.

DO $$
DECLARE
    v_config TEXT;
    v_variants TEXT[] := ARRAY[
        'MDM^T02','MDM^T03','MDM^T04','MDM^T05',
        'MDM^T06','MDM^T07','MDM^T08','MDM^T09',
        'MDM^T10','MDM^T11'
    ];
    v_event TEXT;
    v_names JSONB := '{
        "MDM^T02": "MDM T02 Document Notification and Content",
        "MDM^T03": "MDM T03 Document Status Change",
        "MDM^T04": "MDM T04 Document Status Change and Content",
        "MDM^T05": "MDM T05 Document Addendum Notification",
        "MDM^T06": "MDM T06 Document Addendum Notification and Content",
        "MDM^T07": "MDM T07 Document Edit Notification",
        "MDM^T08": "MDM T08 Document Edit Notification and Content",
        "MDM^T09": "MDM T09 Document Replacement Notification",
        "MDM^T10": "MDM T10 Document Replacement Notification and Content",
        "MDM^T11": "MDM T11 Document Cancel Notification"
    }'::JSONB;
BEGIN
    SELECT template_config INTO v_config
    FROM hl7_fhir_templates
    WHERE message_type = 'MDM^T01' AND is_default = true
    LIMIT 1;

    IF v_config IS NULL THEN
        RAISE NOTICE 'MDM^T01 base profile not found; skipping alias creation';
        RETURN;
    END IF;

    FOREACH v_event IN ARRAY v_variants LOOP
        INSERT INTO hl7_fhir_templates (
            message_type, template_name, template_description,
            profile_version, template_config, is_default
        ) VALUES (
            v_event,
            v_names->>v_event,
            'MDM document variant — shares MDM^T01 base profile',
            '2.0',
            v_config::JSONB || jsonb_build_object('messageType', v_event),
            true
        )
        ON CONFLICT (message_type, hl7_version, is_default) DO UPDATE
            SET template_config  = EXCLUDED.template_config,
                profile_version  = EXCLUDED.profile_version,
                updated_at       = NOW();

        RAISE NOTICE 'Upserted MDM alias: %', v_event;
    END LOOP;
END $$;

-- ── Verification ─────────────────────────────────────────────────────────────
DO $$
DECLARE v_count INT;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM hl7_fhir_templates
    WHERE message_type LIKE 'MDM%' AND profile_version = '2.0';
    RAISE NOTICE '✅ V99: % MDM profiles seeded with extended format (v2.0)', v_count;
END $$;
