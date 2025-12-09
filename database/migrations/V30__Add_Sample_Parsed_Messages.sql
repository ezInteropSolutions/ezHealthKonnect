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
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Ensure one active sample per message type + version combination
    CONSTRAINT unique_sample_per_type_version UNIQUE (message_type, hl7_version, format, is_active)
);

-- Index for fast lookups
CREATE INDEX idx_sample_parsed_messages_lookup
ON sample_parsed_messages(message_type, hl7_version, format)
WHERE is_active = TRUE;

-- Comments
COMMENT ON TABLE sample_parsed_messages IS 'Pre-parsed sample messages for XPath autocomplete and field selection';
COMMENT ON COLUMN sample_parsed_messages.parsed_content IS 'Full enhancedSegments structure as produced by the HL7 parser';
COMMENT ON COLUMN sample_parsed_messages.message_type IS 'HL7 message type in caret format (e.g., ADT^A01)';

-- Insert sample from parsedhl7.json (ADT^A04 v2.3)
-- This will be inserted via Node.js service with the actual parsed_content
