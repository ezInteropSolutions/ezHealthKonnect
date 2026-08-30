-- V218: Add connectivity_types row for snowflake_outbound
-- Applied: 2026-08-29
--
-- services/connectors/snowflake_outbound.go was built this session but had no
-- UI-facing schema row at all, so it wasn't selectable in the pipeline
-- builder -- same gap Databricks had until V217 closed it. Field names below
-- match the Go DatabaseOutboundConfig struct exactly (account/username/
-- password/database/schema/warehouse/role/table_name/write_mode/unique_key/
-- batch_size), same convention as V216/V217.
--
-- IMPORTANT: only username/password authentication is implemented in the Go
-- connector -- key-pair (JWT) auth is explicitly rejected by Initialize() with
-- a clear error rather than attempted. auth_type/private_key/private_key_pass
-- are deliberately NOT exposed in this schema; adding them would let a user
-- configure something that silently fails at Initialize() time instead of
-- being caught by the UI itself.

INSERT INTO connectivity_types (
    type_name, category, display_name, description, icon, mode,
    supports_cron, requires_auth, is_bidirectional, implementation_class,
    config_schema, parameter_groups, is_active, is_beta, priority,
    ui_category, ui_sort_order
) VALUES (
    'snowflake_outbound', 'outbound', 'Snowflake Data Warehouse Writer',
    'Insert/Upsert records into a Snowflake table (username/password authentication only)',
    '❄️', 'push',
    false, true, false, 'SnowflakeOutboundConnector',
    '{"type":"object","required":["account","username","password","table_name"],"properties":{"account":{"type":"string","title":"Account Identifier","description":"e.g. xy12345.us-east-1"},"username":{"type":"string","title":"Username"},"password":{"type":"string","title":"Password","format":"password"},"database":{"type":"string","title":"Database Name"},"schema":{"type":"string","title":"Schema Name"},"warehouse":{"type":"string","title":"Virtual Warehouse"},"role":{"type":"string","title":"Role"},"table_name":{"type":"string","title":"Target Table"},"write_mode":{"enum":["insert","upsert","update"],"type":"string","title":"Operation","default":"insert"},"unique_key":{"type":"string","title":"Unique Key Columns","description":"Comma-separated column name(s) for UPSERT/UPDATE operations (MERGE INTO)"},"batch_size":{"type":"integer","title":"Batch Size","default":50}}}'::jsonb,
    '{"basic":["account","database","schema","warehouse","table_name"],"authentication":["username","password","role"],"operation":["write_mode","unique_key","batch_size"]}'::jsonb,
    true, false, 56,
    'Databases', 30
)
ON CONFLICT (type_name) DO UPDATE SET
    config_schema = EXCLUDED.config_schema,
    parameter_groups = EXCLUDED.parameter_groups,
    display_name = EXCLUDED.display_name,
    description = EXCLUDED.description,
    updated_at = NOW();
