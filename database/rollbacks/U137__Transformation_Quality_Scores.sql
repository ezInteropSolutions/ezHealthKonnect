-- ============================================================================
-- Rollback: V137__Transformation_Quality_Scores
-- ============================================================================
-- Drops the transformation_quality_scores table and its indexes.
-- All quality score history will be permanently lost.
-- ============================================================================

DROP TABLE IF EXISTS transformation_quality_scores;

-- Indexes are dropped automatically by DROP TABLE.
-- Flyway history cleanup:
-- DELETE FROM flyway_schema_history WHERE version = '137';
