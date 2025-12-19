-- V34: Add scoping and priority to sample_parsed_messages
-- Purpose: Support interface-specific samples with fallback to system library

-- Add new columns
ALTER TABLE sample_parsed_messages
ADD COLUMN IF NOT EXISTS sample_scope VARCHAR(20) DEFAULT 'global',
ADD COLUMN IF NOT EXISTS interface_id UUID,
ADD COLUMN IF NOT EXISTS source VARCHAR(50) DEFAULT 'system-library',
ADD COLUMN IF NOT EXISTS priority INTEGER DEFAULT 50;

-- Add foreign key constraint (if not exists)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_sample_interface'
    ) THEN
        ALTER TABLE sample_parsed_messages
        ADD CONSTRAINT fk_sample_interface
        FOREIGN KEY (interface_id) REFERENCES interfaces(id)
        ON DELETE CASCADE;
    END IF;
END$$;

-- Add indexes for efficient lookups
CREATE INDEX IF NOT EXISTS idx_samples_scope
ON sample_parsed_messages(sample_scope, format, message_type);

CREATE INDEX IF NOT EXISTS idx_samples_interface
ON sample_parsed_messages(interface_id, message_type, format)
WHERE interface_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_samples_priority
ON sample_parsed_messages(format, message_type, priority, updated_at);

-- Add comments
COMMENT ON COLUMN sample_parsed_messages.sample_scope IS 'Scope of sample: global (system library) or interface (user-specific)';
COMMENT ON COLUMN sample_parsed_messages.interface_id IS 'If scope=interface, which interface this sample belongs to';
COMMENT ON COLUMN sample_parsed_messages.source IS 'Source of sample: system-library, user-upload, auto-captured';
COMMENT ON COLUMN sample_parsed_messages.priority IS 'Lower number = higher priority. Interface samples default to 10, system library to 50';

-- Update existing samples to be system library
UPDATE sample_parsed_messages
SET
    sample_scope = 'global',
    source = 'system-library',
    priority = 50
WHERE sample_scope IS NULL OR sample_scope = 'global';
