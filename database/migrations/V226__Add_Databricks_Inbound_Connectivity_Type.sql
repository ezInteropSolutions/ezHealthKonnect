-- V226: Add the missing connectivity_types row for databricks_inbound.
-- Applied: 2026-08-30

-- ============================================================
-- BACKGROUND
-- ============================================================
-- databricks_inbound had NO row in connectivity_types at all (same gap
-- documented for snowflake_outbound when it was built) -- it was a stub with
-- zero UI-facing schema. It now has a real implementation
-- (services/connectors/databricks_inbound.go), so this adds a properly
-- schema-matched row, modeled directly on databricks_outbound's already-
-- corrected schema (connection fields) and oracle_inbound's schema
-- (polling/incremental/after-processing fields) -- both live conventions
-- confirmed via direct DB query before writing this, not guessed.

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'databricks_inbound',
    'inbound',
    'Databricks SQL Warehouse Reader',
    'Poll a Delta Lake table via a Databricks SQL Warehouse for new records (scheduled)',
    '🧱',
    'pull',
    true,
    true,
    false,
    'DatabricksInboundConnector',
    '{
        "type": "object",
        "required": ["host", "token", "http_path"],
        "properties": {
            "host": {"type": "string", "title": "Workspace Hostname", "description": "e.g. dbc-a1b2345c-d6e7.cloud.databricks.com"},
            "port": {"type": "integer", "title": "Port", "default": 443},
            "token": {"type": "string", "title": "Personal Access Token", "format": "password"},
            "http_path": {"type": "string", "title": "SQL Warehouse HTTP Path", "description": "e.g. /sql/1.0/warehouses/abc123"},
            "database": {"type": "string", "title": "Catalog Name", "description": "Unity Catalog catalog name"},
            "schema": {"type": "string", "title": "Schema Name"},
            "table_name": {"type": "string", "title": "Table Name", "description": "Table to poll (required if Custom Query is not set)"},
            "query": {"type": "string", "title": "Custom SQL Query"},
            "incremental_column": {"type": "string", "title": "Incremental Column"},
            "incremental_type": {"enum": ["integer", "timestamp", "datetime"], "type": "string", "title": "Incremental Type", "default": "integer"},
            "order_by": {"type": "string", "title": "Order By Clause"},
            "max_records": {"type": "integer", "title": "Max Records Per Poll", "default": 100},
            "polling_interval": {"type": "integer", "title": "Polling Interval (seconds)", "default": 60},
            "after_processing": {"enum": ["nothing", "delete", "update_flag"], "type": "string", "title": "After Processing", "default": "nothing"},
            "processed_flag_col": {"type": "string", "title": "Processed Flag Column"},
            "processed_flag_val": {"type": "string", "title": "Processed Flag Value"},
            "max_open_conns": {"type": "integer", "title": "Max Open Connections", "default": 10},
            "max_idle_conns": {"type": "integer", "title": "Max Idle Connections", "default": 5}
        }
    }'::jsonb,
    '{
        "basic": ["host", "port", "http_path", "database", "schema"],
        "query": ["table_name", "query", "incremental_column", "incremental_type", "order_by", "max_records"],
        "advanced": ["polling_interval", "max_open_conns", "max_idle_conns"],
        "processing": ["after_processing", "processed_flag_col", "processed_flag_val"],
        "authentication": ["token"]
    }'::jsonb,
    true,
    55,
    '1.0',
    'Databases',
    30
)
ON CONFLICT (type_name) DO NOTHING;
