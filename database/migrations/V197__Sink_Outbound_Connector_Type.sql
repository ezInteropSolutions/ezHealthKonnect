-- V197: Sink (Store Only) Outbound Connector Type
-- Registers the sink_outbound connector so it appears in the Target/Outbound
-- connector dropdown (ConnectorConfigBuilder pulls its option list live from
-- this table). Restores "store only, no forwarding" as an explicit, selectable
-- target — previously only a dead option in a legacy pre-connector-framework
-- UI (FormFieldSchema.js) that was never wired into the live interface editor.

INSERT INTO connectivity_types
    (id, type_name, display_name, category, mode, description, icon,
     config_schema, parameter_groups, supports_cron, requires_auth,
     is_bidirectional, is_beta, ui_category, ui_sort_order)
SELECT
    gen_random_uuid(), 'sink_outbound', 'Sink (Store Only)', 'outbound', 'push',
    'Terminal target that accepts messages without forwarding them anywhere. Every message is already persisted by the standard pipeline regardless of outbound connector — use this to explicitly declare an interface as store-only instead of leaving the target unconfigured.',
    '💾',
    '{
        "properties": {
            "enable_logging": {"type": "boolean", "title": "Log Accepted Messages", "default": true},
            "enable_validation": {"type": "boolean", "title": "Reject Empty Messages", "default": true},
            "retention_days": {"type": "number", "title": "Retention (days)", "description": "Informational only — not enforced by this connector", "default": 30},
            "generate_ack": {"type": "boolean", "title": "Generate Acknowledgment", "default": true}
        }
    }',
    '{"basic": ["enable_logging", "generate_ack"], "advanced": ["enable_validation", "retention_days"]}',
    false, false, false, false,
    'Other', 210
WHERE NOT EXISTS (SELECT 1 FROM connectivity_types WHERE type_name = 'sink_outbound');
