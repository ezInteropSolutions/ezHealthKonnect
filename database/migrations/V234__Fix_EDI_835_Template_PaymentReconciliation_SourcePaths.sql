-- V234: Fix bare "parsedEDI.*" sourcePaths in the EDI 835->FHIR OOB template's
-- "Build PaymentReconciliation" step (V231, slug edi-835-to-fhir-sftp).
-- Applied: 2026-09-06

-- ============================================================
-- ROOT CAUSE (found while helping a user test this exact template's
-- fhir.build step through the real Test Pipeline / Re-run code path)
-- ============================================================
-- services/transformation_pipeline_service.go's ExecutePipeline (the engine
-- behind the "Test Pipeline" button and ProcessingEngine.ReprocessMessage's
-- "Re-run" button — a SEPARATE code path from the sequential executePipeline
-- used elsewhere) builds every step's own inputData fresh as
-- {"message": execCtx.Message, ...}. A step like edi.parse that writes its
-- result as a bare top-level outputData["parsedEDI"] (services/executors/
-- transform/edi_parse_executor.go, the standard "outputField" convention
-- shared by cda.parse/edi.validate/etc.) was, before this fix, correctly
-- carried on the OUTBOUND side but then discarded on the way back INTO
-- execCtx.Message: the merge step only ever copied output["message"]'s own
-- keys (plus a hardcoded special case for "enriched") into execCtx.Message,
-- silently dropping any other bare top-level field an executor added. Fixed
-- generically in transformation_pipeline_helpers.go (ExecutePipeline now
-- merges every non-reserved top-level output field, not just "message"/
-- "enriched") — that fix makes the field reach execCtx.Message at all, but a
-- downstream step's own sourcePath still reads its OWN raw inputData
-- ({"message": execCtx.Message, ...}), which has no top-level "parsedEDI"
-- key — only inputData["message"]["parsedEDI"]. The already-proven, already-
-- shipped convention for referencing any message-envelope field from a
-- fhir.build/hl7.build sourcePath (confirmed against tests/playwright/
-- fhir-hl7-build-e2e.spec.js's own working "message.dob" example) is the
-- "message."-prefixed form — GetFieldValue's generic JSON path resolver
-- (services/executors/field_utils.go) has no auto-unwrap fallback the way
-- the legacy GetNestedValue resolver does. This template's own
-- "Build PaymentReconciliation" step was written and verified only via a
-- direct Go-level executor test (services/executors/transform/
-- edi_835_to_fhir_test.go, which hands the executor a hand-built inputData
-- map with "parsedEDI" already at the top level) — never through a real
-- pipeline run — so the bare-path bug shipped unnoticed in V231.
--
-- This migration re-applies the ONE step's config with every "parsedEDI."
-- reference (fields, one field-level condition, and the repeating group's
-- rowsPath + groupCondition) prefixed with "message." — the exact fix
-- verified live against the real interface this bug was found on. The
-- repeating group's own row-level condition ({"field": "CLP...", ...}) is
-- evaluated against each ROW, not the top-level message, so it is
-- deliberately left unprefixed — unaffected by this bug.

UPDATE interface_templates
SET pipeline_config = jsonb_set(
        pipeline_config,
        '{execution_groups,3,steps,0,config}',
        '{
            "resourceType": "PaymentReconciliation",
            "profile": "base",
            "version": "R4",
            "outputField": "message.paymentReconciliation",
            "fields": [
                {"targetPath": "id", "sourcePath": "message.parsedEDI.header.TRN.checkOrEFTTraceNumber"},
                {"targetPath": "status", "literalValue": "active"},
                {"targetPath": "outcome", "literalValue": "complete"},
                {"targetPath": "created", "sourcePath": "message.parsedEDI.header.BPR.paymentEffectiveDate", "transform": "x12_date_to_fhir_date"},
                {"targetPath": "paymentDate", "sourcePath": "message.parsedEDI.header.BPR.paymentEffectiveDate", "transform": "x12_date_to_fhir_date"},
                {"targetPath": "paymentAmount.value", "sourcePath": "message.parsedEDI.header.BPR.totalActualProviderPaymentAmount", "transform": "cda_decimal_string_to_number"},
                {"targetPath": "paymentIssuer.display", "sourcePath": "message.parsedEDI.loops.1000A.N1.name"}
            ]
        }'::jsonb,
        false
    ),
    updated_at = NOW()
WHERE slug = 'edi-835-to-fhir-sftp'
  AND pipeline_config #>> '{execution_groups,3,steps,0,step_alias}' = 'build_payment_reconciliation';
