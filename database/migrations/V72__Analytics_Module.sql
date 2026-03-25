-- ============================================================================
-- V72: Analytics Module — Report Definitions, Monitoring Alerts, Dashboard Layouts
-- ============================================================================

-- ─── Analytics Report Definitions ────────────────────────────────────────────
-- Stores both OOB (is_system=TRUE) and user-saved custom report configurations.

CREATE TABLE IF NOT EXISTS analytics_report_definitions (
    id                  UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    name                VARCHAR(255)    NOT NULL,
    slug                VARCHAR(255)    UNIQUE,
    description         TEXT,
    category            VARCHAR(100)    NOT NULL DEFAULT 'custom',
    -- Values: 'message_flow','pipeline_performance','hipaa_audit',
    --         'connector_activity','dlq_analysis','user_activity','custom'

    -- Report execution config (mirrors ReportExecutionRequest)
    data_source         VARCHAR(100)    NOT NULL,
    metrics             TEXT[]          NOT NULL DEFAULT '{}',
    dimensions          VARCHAR(100),
    filters             JSONB           NOT NULL DEFAULT '{}',
    -- filters: { time_range, interface_ids, statuses, message_types, ... }

    visualization       VARCHAR(50)     NOT NULL DEFAULT 'table',
    -- Values: 'bar','line','pie','table','metric_cards'

    viz_options         JSONB           NOT NULL DEFAULT '{}',
    -- viz_options: { date_bucket:'day'|'hour'|'week', stacked:bool, ... }

    -- Ownership
    is_system           BOOLEAN         NOT NULL DEFAULT FALSE,
    is_public           BOOLEAN         NOT NULL DEFAULT FALSE,
    created_by_user_id  UUID,           -- NULL for OOB system reports

    -- Runtime stats
    last_run_at         TIMESTAMPTZ,
    run_count           INTEGER         NOT NULL DEFAULT 0,
    avg_run_time_ms     INTEGER,

    created_at          TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ard_category    ON analytics_report_definitions(category);
CREATE INDEX IF NOT EXISTS idx_ard_system      ON analytics_report_definitions(is_system);
CREATE INDEX IF NOT EXISTS idx_ard_slug        ON analytics_report_definitions(slug);
CREATE INDEX IF NOT EXISTS idx_ard_created_by  ON analytics_report_definitions(created_by_user_id);

-- ─── Monitoring Alerts ────────────────────────────────────────────────────────
-- Auto-generated threshold alerts for the real-time monitoring dashboard.

CREATE TABLE IF NOT EXISTS monitoring_alerts (
    id                  UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id        UUID,           -- NULL = system-level alert
    alert_type          VARCHAR(100)    NOT NULL,
    -- Types: 'HIGH_ERROR_RATE','SLOW_PROCESSING','DLQ_DEPTH_HIGH',
    --        'INTERFACE_INACTIVE','CONNECTOR_FAILURE','SYSTEM_MEMORY'

    severity            VARCHAR(20)     NOT NULL DEFAULT 'warning',
    -- Values: 'info','warning','critical'

    message             TEXT            NOT NULL,
    context_data        JSONB           NOT NULL DEFAULT '{}',

    acknowledged        BOOLEAN         NOT NULL DEFAULT FALSE,
    acknowledged_by     VARCHAR(255),
    acknowledged_at     TIMESTAMPTZ,

    auto_resolved       BOOLEAN         NOT NULL DEFAULT FALSE,
    resolved_at         TIMESTAMPTZ,

    created_at          TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ma_interface_id  ON monitoring_alerts(interface_id);
CREATE INDEX IF NOT EXISTS idx_ma_severity      ON monitoring_alerts(severity);
CREATE INDEX IF NOT EXISTS idx_ma_unacked       ON monitoring_alerts(acknowledged) WHERE acknowledged = FALSE;
CREATE INDEX IF NOT EXISTS idx_ma_created_at    ON monitoring_alerts(created_at DESC);

-- ─── Dashboard Layouts ────────────────────────────────────────────────────────
-- Persists per-user widget arrangements (used in Phase 4 customisation).

CREATE TABLE IF NOT EXISTS dashboard_layouts (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID,           -- NULL = system default layout
    layout_name     VARCHAR(255)    NOT NULL DEFAULT 'Default',
    page            VARCHAR(50)     NOT NULL DEFAULT 'monitoring',
    -- Values: 'monitoring','reports'

    -- Serialized widget grid: [{widgetType,x,y,w,h,config}]
    layout_config   JSONB           NOT NULL DEFAULT '[]',
    is_default      BOOLEAN         NOT NULL DEFAULT FALSE,

    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_dl_user_page_default
    ON dashboard_layouts(user_id, page) WHERE is_default = TRUE;

-- ─── Performance Indexes on existing tables ───────────────────────────────────
-- Support time-range analytics queries.

CREATE INDEX IF NOT EXISTS idx_te_started_interface
    ON transformation_executions(interface_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_te_started_status
    ON transformation_executions(started_at DESC, status);

CREATE INDEX IF NOT EXISTS idx_cel_interface_started
    ON connectivity_execution_log(interface_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_dlq_error_type_resolved
    ON dead_letter_queue(error_type, resolved, failed_at DESC);

CREATE INDEX IF NOT EXISTS idx_psa_processed_interface
    ON pipeline_step_audit(interface_id, processed_at DESC);

-- ─── OOB Report Seed Data ─────────────────────────────────────────────────────
-- 12 built-in reports seeded as system rows. These cannot be deleted by users.

INSERT INTO analytics_report_definitions
    (name, slug, description, category, data_source, metrics, dimensions, filters, visualization, viz_options, is_system, is_public)
VALUES

-- 1. Interface Throughput Trends
(
    'Interface Throughput Trends',
    'oob-interface-throughput-trends',
    'Daily message volume per interface over a selected time range. Identify traffic patterns and peak periods.',
    'message_flow',
    'message_flow',
    ARRAY['total_messages','processed_count','failed_count'],
    'day',
    '{"time_range":"last_7d"}',
    'line',
    '{"date_bucket":"day","stacked":false}',
    TRUE, TRUE
),

-- 2. Message Status Distribution
(
    'Message Status Distribution',
    'oob-message-status-distribution',
    'Breakdown of message statuses (processed, failed, queued) as a donut chart.',
    'message_flow',
    'message_flow',
    ARRAY['total_messages'],
    'status',
    '{"time_range":"last_24h"}',
    'pie',
    '{"donut":true}',
    TRUE, TRUE
),

-- 3. Message Type Mix
(
    'Message Type Mix',
    'oob-message-type-mix',
    'Distribution of HL7 message types flowing through the system. Shows ADT, ORU, ORM proportions.',
    'message_flow',
    'message_flow',
    ARRAY['total_messages'],
    'message_type',
    '{"time_range":"last_24h"}',
    'pie',
    '{"donut":false}',
    TRUE, TRUE
),

-- 4. Pipeline Execution Summary
(
    'Pipeline Execution Summary',
    'oob-pipeline-execution-summary',
    'Pipeline run counts, success rates, and average duration per interface.',
    'pipeline_performance',
    'pipeline_performance',
    ARRAY['total_executions','success_rate_pct','avg_duration_ms','p95_duration_ms'],
    'interface_id',
    '{"time_range":"last_24h"}',
    'bar',
    '{"stacked":false}',
    TRUE, TRUE
),

-- 5. Step Error Breakdown
(
    'Step Error Breakdown',
    'oob-step-error-breakdown',
    'Which pipeline steps fail most often, ranked by failure count with sample error messages.',
    'pipeline_performance',
    'pipeline_performance',
    ARRAY['failure_count','error_samples'],
    'step_type',
    '{"time_range":"last_24h"}',
    'bar',
    '{"stacked":false,"sort_desc":true}',
    TRUE, TRUE
),

-- 6. Processing Time Distribution
(
    'Processing Time Distribution',
    'oob-processing-time-distribution',
    'Histogram of pipeline processing time buckets with p50, p95, p99 percentiles.',
    'pipeline_performance',
    'pipeline_performance',
    ARRAY['total_executions','p50_duration_ms','p95_duration_ms','p99_duration_ms'],
    'bucket',
    '{"time_range":"last_24h"}',
    'bar',
    '{"histogram":true}',
    TRUE, TRUE
),

-- 7. PHI Access Audit
(
    'PHI Access Audit',
    'oob-phi-access-audit',
    'Which PHI fields were touched in which pipeline steps. For HIPAA compliance audits.',
    'hipaa_audit',
    'hipaa_audit',
    ARRAY['phi_fields_touched','masked_fields','step_name','step_type','interface_id'],
    'step_type',
    '{"time_range":"last_30d"}',
    'table',
    '{}',
    TRUE, TRUE
),

-- 8. Data Masking Coverage
(
    'Data Masking Coverage',
    'oob-data-masking-coverage',
    'Percentage of PHI fields that were masked per interface. Confirms masking policy compliance.',
    'hipaa_audit',
    'hipaa_audit',
    ARRAY['masking_rate_pct','unmasked_count','interface_id'],
    'interface_id',
    '{"time_range":"last_30d"}',
    'metric_cards',
    '{}',
    TRUE, TRUE
),

-- 9. Connector Activity Log
(
    'Connector Activity Log',
    'oob-connector-activity-log',
    'Run history for all connectors showing success, failure, messages retrieved, and duration.',
    'connector_activity',
    'connector_activity',
    ARRAY['execution_count','success_rate_pct','avg_duration_ms','messages_retrieved'],
    'interface_id',
    '{"time_range":"last_24h"}',
    'table',
    '{}',
    TRUE, TRUE
),

-- 10. DLQ Analysis
(
    'DLQ Analysis',
    'oob-dlq-analysis',
    'Dead letter queue breakdown by error type and interface. Identify recurring failure patterns.',
    'dlq_analysis',
    'dlq_analysis',
    ARRAY['dlq_count','avg_retry_count','resolution_rate_pct'],
    'error_type',
    '{"time_range":"last_7d","include_resolved":false}',
    'bar',
    '{"stacked":false}',
    TRUE, TRUE
),

-- 11. User Activity Audit
(
    'User Activity Audit',
    'user_activity',
    'Login events, configuration changes, and high-risk actions by user. For security compliance.',
    'user_activity',
    'user_activity',
    ARRAY['action_count','risk_level','user_id','ip_address'],
    'action',
    '{"time_range":"last_30d"}',
    'table',
    '{}',
    TRUE, TRUE
),

-- 12. Executive Summary
(
    'Executive Summary',
    'oob-executive-summary',
    'All-up KPI dashboard: total messages, error rate, avg processing time, active interfaces. For stakeholder reporting.',
    'message_flow',
    'message_flow',
    ARRAY['total_messages','processed_count','failed_count','error_rate_pct','avg_processing_time_ms','active_interfaces','dlq_depth'],
    'day',
    '{"time_range":"last_7d"}',
    'metric_cards',
    '{"show_trend":true}',
    TRUE, TRUE
)

ON CONFLICT (slug) DO NOTHING;
