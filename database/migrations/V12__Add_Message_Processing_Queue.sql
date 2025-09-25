-- Migration V12: Add message_processing_queue table for MessageQueueService
-- Purpose: Support the message queue functionality expected by MessageQueueService.js

-- =========================================================================
-- Create message_processing_queue table
-- =========================================================================

CREATE TABLE IF NOT EXISTS message_processing_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    message_id UUID NOT NULL,
    queue_name VARCHAR(100) DEFAULT 'default',

    -- Message content and metadata (matching what the code expects)
    message_data JSONB NOT NULL,
    message_metadata JSONB DEFAULT '{}',
    message_type VARCHAR(50),

    -- Queue management
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'retrying')),
    priority INTEGER DEFAULT 5,

    -- Scheduling and processing
    scheduled_for TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    started_processing_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,

    -- Retry handling (matching what the code expects)
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    retry_delay_ms INTEGER DEFAULT 1000,

    -- Error handling
    error_message TEXT,
    error_details JSONB,

    -- Audit fields
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id)
);

-- =========================================================================
-- Create indexes for performance
-- =========================================================================

CREATE INDEX IF NOT EXISTS idx_message_queue_status_priority ON message_processing_queue(status, priority DESC, scheduled_for ASC);
CREATE INDEX IF NOT EXISTS idx_message_queue_interface ON message_processing_queue(interface_id, status);
CREATE INDEX IF NOT EXISTS idx_message_queue_scheduled ON message_processing_queue(scheduled_for) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_message_queue_message_id ON message_processing_queue(message_id);
CREATE INDEX IF NOT EXISTS idx_message_queue_created ON message_processing_queue(created_at DESC);

-- =========================================================================
-- Create trigger for automatic updated_at
-- =========================================================================

CREATE OR REPLACE FUNCTION update_message_queue_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tr_message_queue_update ON message_processing_queue;
CREATE TRIGGER tr_message_queue_update
    BEFORE UPDATE ON message_processing_queue
    FOR EACH ROW
    EXECUTE FUNCTION update_message_queue_timestamp();

-- =========================================================================
-- Grant permissions
-- =========================================================================

GRANT SELECT, INSERT, UPDATE, DELETE ON message_processing_queue TO PUBLIC;

-- =========================================================================
-- Add table comments
-- =========================================================================

COMMENT ON TABLE message_processing_queue IS 'Message processing queue for interface runtime engine';
COMMENT ON COLUMN message_processing_queue.status IS 'Current processing status of the queued message';
COMMENT ON COLUMN message_processing_queue.priority IS 'Processing priority (1=highest, 10=lowest)';
COMMENT ON COLUMN message_processing_queue.message_content IS 'Raw message content and metadata';

-- =========================================================================
-- Migration completion summary
-- =========================================================================

DO $$
BEGIN
    RAISE NOTICE '=== V12 Message Processing Queue Migration Completed ===';
    RAISE NOTICE 'Created message_processing_queue table';
    RAISE NOTICE 'Added indexes for optimal queue performance';
    RAISE NOTICE 'Added automatic timestamp trigger';
    RAISE NOTICE '✅ MessageQueueService infrastructure ready!';
END $$;