-- ============================================================================
-- Rollback: V139__Interface_Transformation_Metrics
-- ============================================================================

DROP TABLE IF EXISTS interface_alert_thresholds;
DROP TABLE IF EXISTS interface_transformation_metrics;

-- Flyway history cleanup:
-- DELETE FROM flyway_schema_history WHERE version = '139';
