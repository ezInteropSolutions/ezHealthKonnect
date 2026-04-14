-- V94: Update DFT^P03 OOB template to reflect ChargeItem + Procedure + Condition assembly
--
-- Prior to this migration the OOB_DFT_P03 template declared only
-- ["MessageHeader","Patient"] in fhir_resources because the field-mapping
-- engine had no ChargeItem/Procedure/Condition support for DFT segments.
--
-- The new dft_assembly.go structural assembly layer now produces:
--   FT1  → ChargeItem   (one per FT1 line item)
--   PR1  → Procedure    (one per PR1, linked to same-set-ID ChargeItem)
--   DG1  → Condition    (one per DG1 diagnosis)
--
-- This migration:
--   1. Updates fhir_resources on the existing OOB template.
--   2. Extends template_config with stub resource sections for Procedure
--      and Condition so the wizard UI can display field hints.
--   3. Leaves the existing MessageHeader/Patient field mappings intact.

UPDATE hl7_fhir_templates
SET
    fhir_resources = '["MessageHeader","Patient","ChargeItem","Procedure","Condition"]'::jsonb,
    template_description = 'Post detail financial transaction — produces Patient, ChargeItem (per FT1), Procedure (per PR1), Condition (per DG1)',
    template_config = template_config || '{
      "assembledResources": {
        "ChargeItem": {
          "assembledBy": "dft_assembly.AssembleDFTCharges",
          "sourceSegment": "FT1",
          "note": "One ChargeItem per FT1 segment. Fields: status←FT1.6, code←FT1.7/FT1.8, occurrenceDateTime←FT1.4, quantity←FT1.10, performer←FT1.20, reason←FT1.19"
        },
        "Procedure": {
          "assembledBy": "dft_assembly.AssembleDFTCharges",
          "sourceSegment": "PR1",
          "note": "One Procedure per PR1 segment, paired to ChargeItem by set-ID. Fields: code←PR1.3 (system←PR1.2), performedDateTime←PR1.5, performer←PR1.12, reasonCode←PR1.13"
        },
        "Condition": {
          "assembledBy": "dft_assembly.AssembleDFTCharges",
          "sourceSegment": "DG1",
          "note": "One Condition per DG1 segment. Fields: code←DG1.3, verificationStatus←DG1.6, recordedDate←DG1.5, asserter←DG1.17"
        }
      }
    }'::jsonb,
    updated_at = NOW()
WHERE template_name = 'OOB_DFT_P03';
