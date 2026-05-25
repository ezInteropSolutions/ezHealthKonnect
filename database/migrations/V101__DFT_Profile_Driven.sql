-- ============================================================================
-- V101: DFT^P03 — Profile-driven assembly (v2.0)
--
-- Creates a v2.0 profile for DFT^P03 (Detail Financial Transaction).
-- The "procedure" composite calls "assembleDFTCharges" which:
--   - Maps FT1 segments → ChargeItem (transaction date, quantity, unit price,
--     service code, transaction type)
--   - Correlates FT1↔PR1 by set-ID → ChargeItem.service reference
--   - Maps PR1 segments → Procedure (code, type, performer, date, duration)
--   - Maps DG1 segments → Condition (diagnosis, rank, date, coding system)
--   - Cross-wires DG1 Conditions to Procedures via Procedure.reasonReference[]
--   - Applies coding system URI resolution for ICD-9/ICD-10/CPT/HCPCS/SNOMED
--   - Generates XHTML narratives for ChargeItem, Procedure, Condition
--
-- Replaces the hardcoded AssembleDFTCharges block in
-- hl7_fhir_transform_service_v3.go (lines 207-226).
-- ============================================================================

INSERT INTO hl7_fhir_templates (
    message_type,
    template_name,
    template_description,
    profile_version,
    template_config,
    is_default
) VALUES (
    'DFT^P03',
    'Standard DFT P03 Mapping',
    'HL7 DFT^P03 Detail Financial Transaction — profile-driven assembly with FT1→ChargeItem, PR1→Procedure, DG1→Condition, FT1↔PR1 set-ID correlation, and DG1↔PR1 cross-linking',
    '2.0',
    $PROFILE$
{
  "messageType": "DFT^P03",
  "version": "2.0",
  "valueSets": {
    "ft1TransactionType": {
      "AJ": "adjustment",
      "CG": "charge",
      "CD": "credit",
      "CO": "co-payment",
      "DE": "deposit",
      "DI": "discharge",
      "NA": "not-applicable",
      "PY": "payment",
      "WO": "write-off"
    },
    "dg1Type": {
      "A": "admitting",
      "F": "final",
      "W": "working",
      "P": "preliminary"
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
    "ChargeItem": {
      "resourceType": "ChargeItem",
      "idFrom": "FT1.1",
      "mappings": [],
      "composites": [
        {
          "fhir":      "_procedures",
          "procedure": "assembleDFTCharges",
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

-- ── DFT variant aliases ───────────────────────────────────────────────────────
DO $$
DECLARE
    v_config TEXT;
    v_variants TEXT[] := ARRAY['DFT^P11'];
    v_event TEXT;
BEGIN
    SELECT template_config INTO v_config
    FROM hl7_fhir_templates
    WHERE message_type = 'DFT^P03' AND is_default = true
    LIMIT 1;

    IF v_config IS NULL THEN
        RAISE NOTICE 'DFT^P03 base profile not found; skipping alias creation';
        RETURN;
    END IF;

    FOREACH v_event IN ARRAY v_variants LOOP
        INSERT INTO hl7_fhir_templates (
            message_type, template_name, template_description,
            profile_version, template_config, is_default
        ) VALUES (
            v_event,
            'DFT ' || split_part(v_event, '^', 2) || ' Variant',
            'DFT financial transaction variant — shares DFT^P03 base profile',
            '2.0',
            v_config::JSONB || jsonb_build_object('messageType', v_event),
            true
        )
        ON CONFLICT (message_type, hl7_version, is_default) DO UPDATE
            SET template_config  = EXCLUDED.template_config,
                profile_version  = EXCLUDED.profile_version,
                updated_at       = NOW();

        RAISE NOTICE 'Upserted DFT alias: %', v_event;
    END LOOP;
END $$;

-- ── Verification ─────────────────────────────────────────────────────────────
DO $$
DECLARE v_count INT;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM hl7_fhir_templates
    WHERE message_type LIKE 'DFT%' AND profile_version = '2.0';
    RAISE NOTICE '✅ V101: % DFT profiles seeded with extended format (v2.0)', v_count;
END $$;
