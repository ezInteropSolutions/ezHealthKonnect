-- V282: repair wizard-created store-only interfaces whose outbound step names a connector type that
-- does not exist.
--
-- The wizard's Passthrough / FHIR Receiver flows saved their outbound step with
-- connectorType "sink". The registered connector is "sink_outbound" (services/connectors/
-- sink_outbound.go), so every message to such an interface failed at delivery with
-- "outbound connector type 'sink' not found" (status = failed) even though nothing was meant to be
-- delivered. Found by tests/playwright/wizard-data-path-e2e.spec.js.
--
-- New interfaces are fixed in TransformationPipelineService (alias sink -> sink_outbound); this
-- migration repairs the steps already stored. Only the type name is changed: the step's other
-- settings (the connector reads snake_case keys and falls back to its defaults for anything absent)
-- are left exactly as saved.

UPDATE transformation_steps
   SET config = jsonb_set(config, '{connectorType}', '"sink_outbound"'::jsonb)
 WHERE step_type = 'connector.outbound'
   AND config ->> 'connectorType' = 'sink';
