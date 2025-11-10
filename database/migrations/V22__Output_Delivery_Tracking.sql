-- V21: Output Delivery Tracking
-- Date: 2025-10-19
-- Description: Add delivery tracking columns to output tables for real-time push-based delivery monitoring

-- ============================================================================
-- PART 1: Add delivery tracking columns to existing output tables
-- ============================================================================

DO $$
DECLARE
    tbl_name TEXT;
    column_count INT;
BEGIN
    RAISE NOTICE 'V21 Migration: Adding delivery tracking columns to output tables';

    -- Loop through all output tables
    FOR tbl_name IN
        SELECT output_table_name
        FROM output_table_metadata
        WHERE output_table_name LIKE 'output_intf_%'
    LOOP
        RAISE NOTICE 'Processing table: %', tbl_name;

        -- Check if columns already exist (idempotent migration)
        EXECUTE format('
            SELECT COUNT(*)
            FROM information_schema.columns
            WHERE table_name = %L
            AND column_name = ''delivery_started_at''
        ', tbl_name) INTO column_count;

        IF column_count = 0 THEN
            -- Add delivery tracking columns individually with IF NOT EXISTS
            EXECUTE format('
                ALTER TABLE %I
                ADD COLUMN IF NOT EXISTS delivery_started_at TIMESTAMP WITH TIME ZONE,
                ADD COLUMN IF NOT EXISTS delivery_completed_at TIMESTAMP WITH TIME ZONE,
                ADD COLUMN IF NOT EXISTS delivery_time_ms INTEGER,
                ADD COLUMN IF NOT EXISTS delivery_status_code INTEGER,
                ADD COLUMN IF NOT EXISTS delivery_response TEXT,
                ADD COLUMN IF NOT EXISTS retry_count INTEGER DEFAULT 0,
                ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMP WITH TIME ZONE,
                ADD COLUMN IF NOT EXISTS max_retries INTEGER DEFAULT 3
            ', tbl_name);

            RAISE NOTICE '  ✅ Added delivery tracking columns to %', tbl_name;
        ELSE
            RAISE NOTICE '  ⏭️  Delivery tracking columns already exist in %', tbl_name;
        END IF;

        -- Create index on delivery_status for efficient querying of pending deliveries
        EXECUTE format('
            CREATE INDEX IF NOT EXISTS idx_%I_delivery_status ON %I(delivery_status)
        ', tbl_name, tbl_name);

        -- Create index on next_retry_at for retry scheduling
        EXECUTE format('
            CREATE INDEX IF NOT EXISTS idx_%I_next_retry ON %I(next_retry_at) WHERE next_retry_at IS NOT NULL
        ', tbl_name, tbl_name);

        RAISE NOTICE '  ✅ Created indexes on %', tbl_name;

    END LOOP;

    RAISE NOTICE 'V21 Migration: Completed adding delivery tracking columns';

END $$;

-- ============================================================================
-- PART 2: Create delivery audit log table
-- ============================================================================

CREATE TABLE IF NOT EXISTS delivery_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    output_message_id VARCHAR(255) NOT NULL,
    interface_id UUID NOT NULL,
    attempt_number INTEGER NOT NULL,
    delivery_status VARCHAR(50) NOT NULL,
    status_code INTEGER,
    request_body TEXT,
    response_body TEXT,
    error_message TEXT,
    delivery_time_ms INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Foreign key constraint
    CONSTRAINT fk_delivery_audit_interface
        FOREIGN KEY (interface_id)
        REFERENCES interfaces(id)
        ON DELETE CASCADE
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_delivery_audit_output_message
    ON delivery_audit_log(output_message_id);

CREATE INDEX IF NOT EXISTS idx_delivery_audit_interface
    ON delivery_audit_log(interface_id);

CREATE INDEX IF NOT EXISTS idx_delivery_audit_status
    ON delivery_audit_log(delivery_status);

CREATE INDEX IF NOT EXISTS idx_delivery_audit_created_at
    ON delivery_audit_log(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_delivery_audit_attempt
    ON delivery_audit_log(output_message_id, attempt_number);

-- Add comment
COMMENT ON TABLE delivery_audit_log IS 'Audit log for output message delivery attempts with full request/response tracking for compliance and debugging';

COMMENT ON COLUMN delivery_audit_log.output_message_id IS 'ID of the output message being delivered';
COMMENT ON COLUMN delivery_audit_log.interface_id IS 'ID of the interface processing this delivery';
COMMENT ON COLUMN delivery_audit_log.attempt_number IS 'Delivery attempt number (1 = first attempt, 2 = first retry, etc.)';
COMMENT ON COLUMN delivery_audit_log.delivery_status IS 'Status of this delivery attempt: delivered, failed, retrying';
COMMENT ON COLUMN delivery_audit_log.status_code IS 'HTTP status code or equivalent protocol response code';
COMMENT ON COLUMN delivery_audit_log.request_body IS 'Full request body sent to destination (FHIR bundle, HL7 message, etc.)';
COMMENT ON COLUMN delivery_audit_log.response_body IS 'Full response body from destination';
COMMENT ON COLUMN delivery_audit_log.error_message IS 'Error message if delivery failed';
COMMENT ON COLUMN delivery_audit_log.delivery_time_ms IS 'Time taken for delivery attempt in milliseconds';

-- ============================================================================
-- PART 3: Update output table metadata schema version
-- ============================================================================

UPDATE output_table_metadata
SET schema_version = '1.2',
    updated_at = NOW()
WHERE schema_version = '1.1';

-- ============================================================================
-- PART 4: Create helper function to get pending deliveries for an interface
-- ============================================================================

CREATE OR REPLACE FUNCTION get_pending_deliveries(p_interface_id UUID)
RETURNS TABLE (
    output_message_id VARCHAR(255),
    correlation_id VARCHAR(255),
    delivery_status VARCHAR(50),
    retry_count INTEGER,
    next_retry_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE
) AS $$
DECLARE
    tbl_name TEXT;
BEGIN
    -- Get table name for this interface
    SELECT output_table_name INTO tbl_name
    FROM output_table_metadata
    WHERE interface_id = p_interface_id;

    IF tbl_name IS NULL THEN
        RAISE EXCEPTION 'No output table found for interface %', p_interface_id;
    END IF;

    -- Return pending deliveries
    RETURN QUERY EXECUTE format('
        SELECT id, correlation_id, delivery_status, retry_count, next_retry_at, created_at
        FROM %I
        WHERE delivery_status IN (''pending'', ''retrying'', ''failed'')
        ORDER BY created_at DESC
    ', tbl_name);
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION get_pending_deliveries IS 'Returns all pending, retrying, or failed deliveries for a specific interface';

-- ============================================================================
-- PART 5: Add migration tracking
-- ============================================================================

-- Insert migration record (Flyway handles this automatically, but we'll add a comment)
COMMENT ON TABLE flyway_schema_history IS 'Flyway migration history - V21 adds output delivery tracking for push-based message delivery';

-- ============================================================================
-- V21 Migration Complete
-- ============================================================================

DO $$
BEGIN
    RAISE NOTICE '========================================';
    RAISE NOTICE 'V21 Migration: Output Delivery Tracking';
    RAISE NOTICE '========================================';
    RAISE NOTICE '✅ Added delivery tracking columns to all output tables';
    RAISE NOTICE '✅ Created delivery_audit_log table';
    RAISE NOTICE '✅ Created indexes for efficient queries';
    RAISE NOTICE '✅ Updated schema version to 1.2';
    RAISE NOTICE '✅ Created get_pending_deliveries() helper function';
    RAISE NOTICE '========================================';
    RAISE NOTICE 'Output Delivery System Ready!';
    RAISE NOTICE '========================================';
END $$;
