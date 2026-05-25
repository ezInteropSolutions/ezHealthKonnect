-- V137: Per-message transformation quality scores
-- Drives the internal mapping improvement feedback loop.
-- Every FHIR transformation is scored automatically; below-threshold
-- records are flagged for integration team review.

CREATE TABLE transformation_quality_scores (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id            VARCHAR     NOT NULL,
    interface_id          UUID        NOT NULL,
    message_type          VARCHAR(50),
    score                 NUMERIC(5,2),           -- 0.00–100.00; lower = worse
    resource_counts       JSONB,                  -- {"Patient":1,"Encounter":1,...}
    expected_resources    TEXT[],                 -- from MessageTypeManifest
    missing_resources     TEXT[],                 -- expected minus produced
    empty_required_fields JSONB,                  -- {"Patient.identifier":true,...}
    unhandled_segments    TEXT[],                 -- Z-segs or segments with no mapping
    warnings              TEXT[],
    errors                TEXT[],
    flagged               BOOLEAN     NOT NULL DEFAULT false,
    flag_reason           TEXT,
    reviewed              BOOLEAN     NOT NULL DEFAULT false,
    reviewed_by           UUID,
    reviewed_at           TIMESTAMPTZ,
    resolution            TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Primary access pattern: review queue (unreviewed flags, newest first)
CREATE INDEX idx_tqs_flagged_unreviewed
    ON transformation_quality_scores (created_at DESC)
    WHERE flagged = true AND reviewed = false;

-- Per-interface + type breakdown for stats aggregation
CREATE INDEX idx_tqs_interface_type
    ON transformation_quality_scores (interface_id, message_type, created_at DESC);

-- Score distribution queries (passing messages only)
CREATE INDEX idx_tqs_score_passing
    ON transformation_quality_scores (score)
    WHERE flagged = false;

-- Fast lookup by message_id for joining with message tables
CREATE INDEX idx_tqs_message_id
    ON transformation_quality_scores (message_id);
