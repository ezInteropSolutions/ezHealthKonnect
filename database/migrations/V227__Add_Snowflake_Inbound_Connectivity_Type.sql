-- V227: Add the missing connectivity_types row for snowflake_inbound.
-- Applied: 2026-08-30

-- ============================================================
-- BACKGROUND
-- ============================================================
-- snowflake_inbound had NO row in connectivity_types at all -- same gap
-- V226 closed for databricks_inbound. It now has a real implementation
-- (services/connectors/snowflake_inbound.go), so this adds a properly
-- schema-matched row, modeled directly on snowflake_outbound's already-
-- corrected schema (connection fields) and oracle_inbound's schema
-- (polling/incremental/after-processing fields) -- both live conventions
-- confirmed via direct DB query before writing this, not guessed.

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, priority, version,
    ui_category, ui_sort_order
) VALUES (
    'snowflake_inbound',
    'inbound',
    'Snowflake Data Warehouse Reader',
    'Poll a Snowflake table for new records (scheduled)',
    '❄️',
    'pull',
    true,
    true,
    false,
    'SnowflakeInboundConnector',
    '{
        "type": "object",
        "required": ["account", "username", "password"],
        "properties": {
            "account": {"type": "string", "title": "Account Identifier", "description": "e.g. xy12345.us-east-1"},
            "username": {"type": "string", "title": "Username"},
            "password": {"type": "string", "title": "Password", "format": "password"},
            "database": {"type": "string", "title": "Database Name"},
            "schema": {"type": "string", "title": "Schema Name"},
            "warehouse": {"type": "string", "title": "Virtual Warehouse"},
            "role": {"type": "string", "title": "Role"},
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
        "basic": ["account", "database", "schema", "warehouse", "table_name"],
        "query": ["query", "incremental_column", "incremental_type", "order_by", "max_records"],
        "advanced": ["polling_interval", "max_open_conns", "max_idle_conns"],
        "processing": ["after_processing", "processed_flag_col", "processed_flag_val"],
        "authentication": ["username", "password", "role"]
    }'::jsonb,
    true,
    56,
    '1.0',
    'Databases',
    30
)
ON CONFLICT (type_name) DO NOTHING;
