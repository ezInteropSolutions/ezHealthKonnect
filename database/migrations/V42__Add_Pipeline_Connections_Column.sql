-- V42: Add connections column to transformation_pipelines
-- Stores flowchart connections for visual pipeline builder persistence

-- Add connections column as JSONB array
ALTER TABLE transformation_pipelines
ADD COLUMN IF NOT EXISTS connections JSONB DEFAULT '[]'::jsonb;

-- Add comment
COMMENT ON COLUMN transformation_pipelines.connections IS 'Flowchart connections between steps - array of {from, to, type}';

-- Index for querying pipelines with connections
CREATE INDEX IF NOT EXISTS idx_transformation_pipelines_connections
ON transformation_pipelines USING gin (connections);
