-- V4__Add_connectivity_columns_to_interfaces.sql
-- Flywheel migration to add source_connectivity and target_connectivity columns
-- This resolves the "column i.source_connectivity does not exist" error

-- Add source_connectivity column with default value
ALTER TABLE interfaces 
ADD COLUMN source_connectivity VARCHAR(50) DEFAULT 'direct' NOT NULL;

-- Add target_connectivity column with default value  
ALTER TABLE interfaces 
ADD COLUMN target_connectivity VARCHAR(50) DEFAULT 'direct' NOT NULL;

-- Add column comments for documentation
COMMENT ON COLUMN interfaces.source_connectivity IS 'Source connectivity method: direct, api, file, database, ftp, sftp, etc.';
COMMENT ON COLUMN interfaces.target_connectivity IS 'Target connectivity method: direct, api, file, database, ftp, sftp, etc.';

-- Create index for performance on connectivity queries
CREATE INDEX IF NOT EXISTS idx_interfaces_source_connectivity ON interfaces(source_connectivity);
CREATE INDEX IF NOT EXISTS idx_interfaces_target_connectivity ON interfaces(target_connectivity);

-- Update any existing NULL values (shouldn't be any with NOT NULL + DEFAULT, but safety first)
UPDATE interfaces 
SET source_connectivity = 'direct' 
WHERE source_connectivity IS NULL;

UPDATE interfaces 
SET target_connectivity = 'direct' 
WHERE target_connectivity IS NULL;