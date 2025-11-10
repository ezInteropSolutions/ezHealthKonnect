-- V30__Add_Source_Target_Connectivity.sql
-- Converts source_connectivity and target_connectivity from VARCHAR to JSONB
-- Renames to source_connectivity_jsonb and target_connectivity_jsonb for clarity
-- These will replace the old source_config and target_config columns

-- Drop existing VARCHAR columns and replace with JSONB
ALTER TABLE interfaces
DROP COLUMN IF EXISTS source_connectivity CASCADE;

ALTER TABLE interfaces
DROP COLUMN IF EXISTS target_connectivity CASCADE;

-- Add new JSONB columns with better names
ALTER TABLE interfaces
ADD COLUMN source_connectivity JSONB DEFAULT '{}'::jsonb;

ALTER TABLE interfaces
ADD COLUMN target_connectivity JSONB DEFAULT '{}'::jsonb;

-- Migrate data from old source_config/target_config to new structure (if they have data)
UPDATE interfaces
SET source_connectivity = jsonb_build_object(
    'type', COALESCE(source_type, 'unknown'),
    'config', COALESCE(source_config, '{}'::jsonb)
)
WHERE source_config IS NOT NULL AND source_config != '{}'::jsonb;

UPDATE interfaces
SET target_connectivity = jsonb_build_object(
    'type', COALESCE(target_type, 'unknown'),
    'config', COALESCE(target_config, '{}'::jsonb)
)
WHERE target_config IS NOT NULL AND target_config != '{}'::jsonb;

-- Create GIN indexes for efficient JSONB queries
CREATE INDEX idx_interfaces_source_connectivity_gin
  ON interfaces USING GIN (source_connectivity);

CREATE INDEX idx_interfaces_target_connectivity_gin
  ON interfaces USING GIN (target_connectivity);

-- Create indexes for type lookup (common query pattern)
CREATE INDEX idx_interfaces_source_connectivity_type
  ON interfaces ((source_connectivity->>'type'));

CREATE INDEX idx_interfaces_target_connectivity_type
  ON interfaces ((target_connectivity->>'type'));

-- Add comments
COMMENT ON COLUMN interfaces.source_connectivity IS
  'Source/Input connectivity configuration (JSONB): type (fhir_receiver, tcp_mllp, http_listener, etc.), authentication, validation, rate limiting, endpoint configuration';

COMMENT ON COLUMN interfaces.target_connectivity IS
  'Target/Output connectivity configuration (JSONB): type (fhir_http, postgresql, mongodb, etc.), destination, delivery mode, authentication, retry settings, resource selection';
