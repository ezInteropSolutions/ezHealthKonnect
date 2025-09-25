-- V13__Enhanced_Message_Management.sql
-- Enhanced message management system for interface-specific functionality

-- Enhanced message processing table with metadata focus
CREATE TABLE IF NOT EXISTS message_processing_enhanced (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,

    -- Message identification
    message_id VARCHAR(255) NOT NULL, -- External message ID (from source system)
    correlation_id VARCHAR(255), -- For tracking related messages

    -- Processing metadata
    status VARCHAR(50) NOT NULL DEFAULT 'received', -- received, processing, transformed, sent, failed, error
    priority INTEGER NOT NULL DEFAULT 5,

    -- Timing information
    received_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processing_started_at TIMESTAMP WITH TIME ZONE,
    processing_completed_at TIMESTAMP WITH TIME ZONE,

    -- Source information
    source_type VARCHAR(100) NOT NULL, -- hl7, fhir, http, file, etc.
    source_endpoint VARCHAR(500), -- Where message came from
    source_ip INET,

    -- Message metadata (for performance - no full content here)
    message_type VARCHAR(100), -- ADT^A01, Patient, etc.
    message_size INTEGER, -- Size in bytes
    message_encoding VARCHAR(50) DEFAULT 'UTF-8',

    -- Processing results
    transformation_applied BOOLEAN DEFAULT false,
    transformation_type VARCHAR(100), -- hl7-to-fhir, fhir-validation, etc.
    processing_time_ms INTEGER,

    -- Error handling
    error_count INTEGER DEFAULT 0,
    last_error_message TEXT,
    last_error_at TIMESTAMP WITH TIME ZONE,
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,

    -- Delivery information
    target_endpoint VARCHAR(500),
    delivery_status VARCHAR(50), -- pending, delivered, failed
    delivery_attempts INTEGER DEFAULT 0,
    last_delivery_attempt_at TIMESTAMP WITH TIME ZONE,

    -- Performance metadata
    is_large_message BOOLEAN DEFAULT false, -- Flag for messages >1MB
    compression_used BOOLEAN DEFAULT false,

    -- Audit
    created_by UUID REFERENCES users(id),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Indexing for performance
    CONSTRAINT valid_status CHECK (status IN ('received', 'processing', 'transformed', 'sent', 'failed', 'error', 'reprocessing'))
);

-- Separate table for actual message content (for performance)
CREATE TABLE IF NOT EXISTS message_content_store (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_processing_id UUID NOT NULL REFERENCES message_processing_enhanced(id) ON DELETE CASCADE,

    -- Content types
    content_type VARCHAR(50) NOT NULL, -- original, transformed, error_details

    -- Message content (can be large)
    content_data TEXT, -- For smaller messages
    content_blob BYTEA, -- For binary or compressed content
    content_compressed BOOLEAN DEFAULT false,

    -- Metadata
    content_size INTEGER,
    content_encoding VARCHAR(50) DEFAULT 'UTF-8',
    checksum VARCHAR(64), -- SHA-256 for integrity

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT valid_content_type CHECK (content_type IN ('original', 'transformed', 'error_details', 'delivery_response'))
);

-- Message transformation tracking
CREATE TABLE IF NOT EXISTS message_transformations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_processing_id UUID NOT NULL REFERENCES message_processing_enhanced(id) ON DELETE CASCADE,

    -- Transformation details
    transformation_step INTEGER NOT NULL, -- For multi-step transformations
    transformation_name VARCHAR(200) NOT NULL,
    transformation_type VARCHAR(100) NOT NULL, -- mapping, validation, formatting, etc.

    -- Transformation metadata
    input_format VARCHAR(100),
    output_format VARCHAR(100),
    mapping_rules_used JSONB,

    -- Results
    success BOOLEAN NOT NULL,
    processing_time_ms INTEGER,
    warnings_count INTEGER DEFAULT 0,
    errors_count INTEGER DEFAULT 0,

    -- Details
    warnings JSONB,
    errors JSONB,
    metadata JSONB,

    -- Timestamps
    started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,

    -- Unique constraint for step ordering
    UNIQUE(message_processing_id, transformation_step)
);

-- Performance indexes
CREATE INDEX IF NOT EXISTS idx_message_processing_enhanced_interface_status
ON message_processing_enhanced(interface_id, status);

CREATE INDEX IF NOT EXISTS idx_message_processing_enhanced_received_at
ON message_processing_enhanced(received_at DESC);

CREATE INDEX IF NOT EXISTS idx_message_processing_enhanced_message_id
ON message_processing_enhanced(message_id);

CREATE INDEX IF NOT EXISTS idx_message_processing_enhanced_correlation_id
ON message_processing_enhanced(correlation_id) WHERE correlation_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_message_processing_enhanced_status_priority
ON message_processing_enhanced(status, priority DESC, received_at);

CREATE INDEX IF NOT EXISTS idx_message_content_store_processing_id_type
ON message_content_store(message_processing_id, content_type);

CREATE INDEX IF NOT EXISTS idx_message_transformations_processing_id
ON message_transformations(message_processing_id, transformation_step);

-- Views for common queries
CREATE OR REPLACE VIEW message_processing_summary AS
SELECT
    mpe.id,
    mpe.interface_id,
    i.name as interface_name,
    mpe.message_id,
    mpe.correlation_id,
    mpe.status,
    mpe.message_type,
    mpe.message_size,
    mpe.source_type,
    mpe.source_endpoint,
    mpe.received_at,
    mpe.processing_completed_at,
    mpe.processing_time_ms,
    mpe.error_count,
    mpe.retry_count,
    mpe.transformation_applied,
    mpe.delivery_status,
    mpe.delivery_attempts,
    CASE
        WHEN mpe.processing_completed_at IS NOT NULL
        THEN EXTRACT(EPOCH FROM (mpe.processing_completed_at - mpe.received_at)) * 1000
        ELSE NULL
    END as total_processing_time_ms
FROM message_processing_enhanced mpe
JOIN interfaces i ON mpe.interface_id = i.id;

-- Function to update processing timestamps
CREATE OR REPLACE FUNCTION update_message_processing_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;

    -- Auto-update processing timestamps based on status changes
    IF NEW.status = 'processing' AND OLD.status != 'processing' THEN
        NEW.processing_started_at = CURRENT_TIMESTAMP;
    END IF;

    IF NEW.status IN ('transformed', 'sent', 'failed', 'error') AND OLD.status = 'processing' THEN
        NEW.processing_completed_at = CURRENT_TIMESTAMP;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for automatic timestamp updates
DROP TRIGGER IF EXISTS tr_update_message_processing_timestamp ON message_processing_enhanced;
CREATE TRIGGER tr_update_message_processing_timestamp
    BEFORE UPDATE ON message_processing_enhanced
    FOR EACH ROW
    EXECUTE FUNCTION update_message_processing_timestamp();

-- Sample data for testing (optional)
-- INSERT INTO message_processing_enhanced (
--     interface_id, message_id, status, source_type, message_type, message_size
-- ) VALUES (
--     (SELECT id FROM interfaces LIMIT 1),
--     'TEST-MSG-001',
--     'received',
--     'http',
--     'Patient',
--     1024
-- );

COMMENT ON TABLE message_processing_enhanced IS 'Enhanced message processing tracking with metadata focus for performance';
COMMENT ON TABLE message_content_store IS 'Separate storage for message content to improve query performance on metadata';
COMMENT ON TABLE message_transformations IS 'Detailed transformation tracking for debugging and audit';
COMMENT ON VIEW message_processing_summary IS 'Optimized view for message listing UI with essential metadata';