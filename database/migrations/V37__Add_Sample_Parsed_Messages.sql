-- V30: Add Sample Parsed Messages Table
-- Purpose: Store pre-parsed sample messages for XPath autocomplete
-- This ensures autocomplete works even when no real messages exist

CREATE TABLE IF NOT EXISTS sample_parsed_messages (
    id SERIAL PRIMARY KEY,
    message_type VARCHAR(50) NOT NULL,  -- e.g., 'ADT^A01', 'ORU^R01'
    hl7_version VARCHAR(10) NOT NULL,   -- e.g., '2.3', '2.5'
    format VARCHAR(20) NOT NULL DEFAULT 'hl7v2',  -- 'hl7v2', 'fhir', 'ccd'
    parsed_content JSONB NOT NULL,      -- Full enhancedSegments structure
    description TEXT,                    -- Optional description of this sample
    is_active BOOLEAN DEFAULT TRUE,      -- Allow disabling samples
    -- V34 scoping columns (included here since V34 runs before V37)
    sample_scope VARCHAR(20) DEFAULT 'global',
    interface_id UUID REFERENCES interfaces(id) ON DELETE CASCADE,
    source VARCHAR(50) DEFAULT 'system-library',
    priority INTEGER DEFAULT 50,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Ensure one active sample per message type + version combination
    CONSTRAINT unique_sample_per_type_version UNIQUE (message_type, hl7_version, format, is_active)
);

-- Index for fast lookups
CREATE INDEX IF NOT EXISTS idx_sample_parsed_messages_lookup
ON sample_parsed_messages(message_type, hl7_version, format)
WHERE is_active = TRUE;

-- Comments
COMMENT ON TABLE sample_parsed_messages IS 'Pre-parsed sample messages for XPath autocomplete and field selection';
COMMENT ON COLUMN sample_parsed_messages.parsed_content IS 'Full enhancedSegments structure as produced by the HL7 parser';
COMMENT ON COLUMN sample_parsed_messages.message_type IS 'HL7 message type in caret format (e.g., ADT^A01)';

-- V34 scoping indexes (included here since V34 runs before V37)
CREATE INDEX IF NOT EXISTS idx_samples_scope
ON sample_parsed_messages(sample_scope, format, message_type);

CREATE INDEX IF NOT EXISTS idx_samples_interface
ON sample_parsed_messages(interface_id, message_type, format)
WHERE interface_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_samples_priority
ON sample_parsed_messages(format, message_type, priority, updated_at);

-- Insert sample from parsedhl7.json (ADT^A04 v2.3)
-- This will be inserted via Node.js service with the actual parsed_content
