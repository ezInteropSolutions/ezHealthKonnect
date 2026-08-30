-- V221: Hide unimplemented (stub) connector types from the pipeline builder picker
-- Applied: 2026-08-30
--
-- These 11 connector types have a real, selectable connectivity_types row
-- (is_active=true) but their actual Go implementation
-- (services/connectors/connector_stubs.go) is still a placeholder that does
-- nothing real. Selecting one of these in the UI today would let a user
-- configure something that silently doesn't work, with no indication that
-- it's incomplete.
--
-- Confirmed zero real usage before hiding (same due-diligence as V216):
--   SELECT ct.type_name, count(ic.id)
--   FROM connectivity_types ct
--   LEFT JOIN interface_connectivity ic
--     ON ic.source_connectivity_type_id = ct.id OR ic.target_connectivity_type_id = ct.id
--   WHERE ct.type_name IN (...) GROUP BY ct.type_name;
--   -> all 11 returned usage_count = 0
--   Also confirmed 0 rows in transformation_steps for the outbound-side names.
-- So hiding these cannot break any existing running interface or saved pipeline.
--
-- This is a pure data change -- zero Go or JS code changes needed.
-- GetConnectivityTypesByCategory (controllers/connectivity_controller.go), the
-- endpoint the pipeline builder's connector-type dropdown actually calls,
-- already hardcodes filter.IsActive = true unconditionally. No other frontend
-- code queries the unfiltered /api/connectivity/types endpoint. So setting
-- is_active=false here is sufficient on its own to remove these from the UI.
--
-- azure_blob_outbound is deliberately NOT in this list -- it was built for
-- real earlier this session and stays active. Only azure_blob_inbound (still
-- a stub) is hidden.
--
-- To re-show one of these later (once it's actually implemented for real),
-- simply flip its is_active back to true -- no migration needed for that,
-- since the row and its config_schema/parameter_groups are untouched here.

UPDATE connectivity_types
SET is_active = false, updated_at = NOW()
WHERE type_name IN (
    'azure_blob_inbound',
    'direct_messaging_inbound',
    'edi_x12_inbound',
    'ftp_inbound',
    'gcs_inbound',
    'direct_messaging_outbound',
    'edi_x12_outbound',
    'ftp_outbound',
    'gcs_outbound',
    'rabbitmq_outbound',
    'redis_outbound'
);
