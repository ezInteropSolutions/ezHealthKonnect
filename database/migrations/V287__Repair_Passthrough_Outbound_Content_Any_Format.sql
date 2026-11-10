-- V287: V286 repaired only HL7-over-MLLP and FHIR passthrough pipelines. A passthrough means "output is the input",
-- whatever the format, so the same wizard default (contentField fhirBundle + application/fhir+json) was equally wrong
-- for every other inbound: EDI, NCPDP, CSV, ASTM, DICOM, REST, file ... all delivered the parsed-message envelope.
--
-- Scope — same narrow rule as V286, for the inbounds V286 did not cover: the pipeline's ONLY enabled steps are one
-- connector.inbound and one connector.outbound (nothing between them can ever produce a fhirBundle), and the outbound
-- still carries exactly the wizard's wrong default. The step then delivers the original content (raw); no contentType
-- is forced because the format is unknown here. Idempotent.

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
SET config = jsonb_set(o.config, '{contentField}', '"raw"') - 'contentType',
    updated_at = NOW()
FROM passthrough pt
JOIN transformation_steps i
  ON i.pipeline_id = pt.pipeline_id AND i.step_type = 'connector.inbound' AND i.enabled
WHERE o.pipeline_id = pt.pipeline_id
  AND o.step_type = 'connector.outbound'
  AND o.enabled
  AND o.config->>'contentField' = 'fhirBundle'
  AND o.config->>'contentType'  = 'application/fhir+json'
  AND COALESCE(i.config->>'connectorType', '') NOT IN ('tcp_mllp', 'tcp_mllp_inbound', 'http_fhir_inbound');
