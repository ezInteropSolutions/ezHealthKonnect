-- V286: repair passthrough pipelines that deliver the parsed-message envelope instead of the original message.
--
-- The wizard used to save EVERY outbound connector step with contentField "fhirBundle" + contentType
-- "application/fhir+json", whatever the flow (TransformationPipelineService.addConnectorStep). A passthrough builds no
-- FHIR bundle, so the outbound executor fell back to the whole parsed-message envelope and labelled it FHIR: an HL7
-- passthrough delivered ~387 KB of JSON (the full schema embedded) for a 4-segment message, and a FHIR passthrough
-- handed its destination {"_format":"fhir","_semantic_index":...}. New interfaces are fixed in the service; this
-- repairs the ones already created.
--
-- Scope — deliberately narrow, only what is certainly wrong:
--   * the pipeline's ONLY enabled steps are one connector.inbound and one connector.outbound (nothing between them can
--     ever produce a fhirBundle, so the setting can never resolve);
--   * the outbound step still carries exactly the wizard's wrong default (fhirBundle + application/fhir+json);
--   * the inbound is HL7 over MLLP (-> original HL7, application/hl7-v2) or a FHIR receiver (-> original resource).
-- Anything else (file/REST inbounds of unknown format, pipelines with transform steps, hand-edited steps) is untouched.
-- Idempotent: once repaired, a step no longer matches.

WITH passthrough AS (
    SELECT p.id AS pipeline_id
    FROM transformation_pipelines p
    JOIN transformation_steps s ON s.pipeline_id = p.id AND s.enabled
    GROUP BY p.id
    HAVING COUNT(*) = 2
       AND COUNT(*) FILTER (WHERE s.step_type = 'connector.inbound')  = 1
       AND COUNT(*) FILTER (WHERE s.step_type = 'connector.outbound') = 1
)
UPDATE transformation_steps o
SET config = jsonb_set(
                 jsonb_set(o.config, '{contentField}', '"raw"'),
                 '{contentType}',
                 to_jsonb(CASE WHEN i.config->>'connectorType' IN ('tcp_mllp', 'tcp_mllp_inbound')
                               THEN 'application/hl7-v2'
                               ELSE 'application/fhir+json' END)
             ),
    updated_at = NOW()
FROM passthrough pt
JOIN transformation_steps i
  ON i.pipeline_id = pt.pipeline_id AND i.step_type = 'connector.inbound' AND i.enabled
WHERE o.pipeline_id = pt.pipeline_id
  AND o.step_type = 'connector.outbound'
  AND o.enabled
  AND o.config->>'contentField' = 'fhirBundle'
  AND o.config->>'contentType'  = 'application/fhir+json'
  AND i.config->>'connectorType' IN ('tcp_mllp', 'tcp_mllp_inbound', 'http_fhir_inbound');
