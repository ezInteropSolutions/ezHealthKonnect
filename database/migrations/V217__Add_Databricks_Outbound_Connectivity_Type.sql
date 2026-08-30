-- V217: Add connectivity_types row for databricks_outbound
-- Applied: 2026-08-29
--
-- services/connectors/databricks_outbound.go was built this session but had
-- no UI-facing schema row at all, so it wasn't selectable in the pipeline
-- builder. Field names below match the Go DatabaseOutboundConfig struct
-- exactly (host/port/token/http_path/database/schema/table_name/write_mode/
-- unique_key/batch_size) -- the same convention just corrected for the other
-- 8 DB/cloud connectors in V216, applied here from the start rather than
-- drifting and needing a second fix later.

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority,
    ui_category, ui_sort_order
) VALUES (
    'databricks_outbound', 'outbound', 'Databricks SQL Warehouse Writer',
    'Insert/Upsert records into a Delta Lake table via a Databricks SQL Warehouse',
    '🧱', 'push',
    false, true, false, 'DatabricksOutboundConnector',
    '{"type":"object","required":["host","token","http_path","table_name"],"properties":{"host":{"type":"string","title":"Workspace Hostname","description":"e.g. dbc-a1b2345c-d6e7.cloud.databricks.com"},"port":{"type":"integer","title":"Port","default":443},"token":{"type":"string","title":"Personal Access Token","format":"password"},"http_path":{"type":"string","title":"SQL Warehouse HTTP Path","description":"e.g. /sql/1.0/warehouses/abc123"},"database":{"type":"string","title":"Catalog Name","description":"Unity Catalog catalog name"},"schema":{"type":"string","title":"Schema Name"},"table_name":{"type":"string","title":"Target Table"},"write_mode":{"enum":["insert","upsert","update"],"type":"string","title":"Operation","default":"insert"},"unique_key":{"type":"string","title":"Unique Key Columns","description":"Comma-separated column name(s) for UPSERT/UPDATE operations (Delta Lake MERGE)"},"batch_size":{"type":"integer","title":"Batch Size","default":50}}}'::jsonb,
    '{"basic":["host","port","http_path","database","schema","table_name"],"authentication":["token"],"operation":["write_mode","unique_key","batch_size"]}'::jsonb,
    true, false, 55,
    'Databases', 30
)
ON CONFLICT (type_name) DO UPDATE SET
    config_schema = EXCLUDED.config_schema,
    parameter_groups = EXCLUDED.parameter_groups,
    display_name = EXCLUDED.display_name,
    description = EXCLUDED.description,
    updated_at = NOW();
