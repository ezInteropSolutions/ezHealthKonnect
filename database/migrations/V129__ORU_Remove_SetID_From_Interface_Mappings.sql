-- ============================================================================
-- V129: ORU^R01 — Remove SI (Set ID) → FHIR resource id mappings from the
--       legacy interfaces.transformation_mapping column.
--
-- V128 cleaned hl7_fhir_templates (OOB templates).  This migration fixes the
-- same problem in wizard-saved interface-level mappings stored in
-- interfaces.transformation_mapping (a JSONB array of
-- {"sourcePath":"OBR.1","targetPath":"DiagnosticReport.id",...} objects).
--
-- OBR.1 and OBX.1 are SI (Sequence ID) fields — positional counters that
-- carry no identity information.  They must not be used as FHIR resource ids.
-- ============================================================================

UPDATE interfaces
SET transformation_mapping = (
    SELECT jsonb_agg(elem)
    FROM jsonb_array_elements(transformation_mapping) AS elem
    WHERE NOT (
        -- Remove any entry whose sourcePath ends with '.1' (Set-ID field)
        -- AND whose targetPath ends with '.id' (FHIR resource identifier).
        elem->>'sourcePath' ~ '^\w+\.1$'
        AND elem->>'targetPath' ~ '\.id$'
    )
)
WHERE transformation_mapping IS NOT NULL
  AND jsonb_typeof(transformation_mapping) = 'array'
  AND EXISTS (
      SELECT 1 FROM jsonb_array_elements(transformation_mapping) e
      WHERE e->>'sourcePath' ~ '^\w+\.1$'
        AND e->>'targetPath' ~ '\.id$'
  );

DO $$
DECLARE v_count INT;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM interfaces,
         jsonb_array_elements(transformation_mapping) AS elem
    WHERE transformation_mapping IS NOT NULL
      AND jsonb_typeof(transformation_mapping) = 'array'
      AND elem->>'sourcePath' ~ '^\w+\.1$'
      AND elem->>'targetPath' ~ '\.id$';

    RAISE NOTICE 'V129: Remaining SI→id wizard mappings after cleanup: %', v_count;
    IF v_count > 0 THEN
        RAISE EXCEPTION 'V129: SI→id wizard mappings still present after cleanup';
    END IF;
    RAISE NOTICE '✅ V129: SI (Set ID) fields removed from resource id in all wizard-saved interface mappings';
END $$;
