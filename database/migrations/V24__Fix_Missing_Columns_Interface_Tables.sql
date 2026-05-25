-- V24: Fix Missing Columns in Interface Tables
-- This migration ensures all existing interface tables have required columns from V19 and V23

-- ============================================
-- Add V19 Parsing Columns to ALL Interface Tables
-- ============================================

DO $$
DECLARE
    table_record RECORD;
BEGIN
    -- Loop through all messages_intf_* tables
    FOR table_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'messages_intf_%'
    LOOP
        -- Add V19 parsing columns if they don't exist
        EXECUTE format('
            ALTER TABLE %I
            ADD COLUMN IF NOT EXISTS parsed_at TIMESTAMP WITH TIME ZONE,
            ADD COLUMN IF NOT EXISTS parsing_status VARCHAR(50),
            ADD COLUMN IF NOT EXISTS parsing_time_ms INTEGER,
            ADD COLUMN IF NOT EXISTS parsing_error TEXT
        ', table_record.tablename);

        RAISE NOTICE 'Added V19 parsing columns to table: %', table_record.tablename;
    END LOOP;
END $$;

-- ============================================
-- Add V23 Error Handling Columns to ALL Interface Tables (if missing)
-- ============================================

DO $$
DECLARE
    table_record RECORD;
BEGIN
    -- Loop through all messages_intf_* tables
    FOR table_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'messages_intf_%'
    LOOP
        -- Add V23 error handling columns if they don't exist
        EXECUTE format('
            ALTER TABLE %I
            ADD COLUMN IF NOT EXISTS error_stack JSONB DEFAULT ''[]''::jsonb,
            ADD COLUMN IF NOT EXISTS last_error_timestamp TIMESTAMP WITH TIME ZONE,
            ADD COLUMN IF NOT EXISTS last_error_stage VARCHAR(50),
            ADD COLUMN IF NOT EXISTS error_severity VARCHAR(20)
        ', table_record.tablename);

        RAISE NOTICE 'Added V23 error columns to table: %', table_record.tablename;
    END LOOP;
END $$;

-- ============================================
-- Create Indexes for Error Columns (if missing)
-- ============================================

DO $$
DECLARE
    table_record RECORD;
    index_name TEXT;
BEGIN
    FOR table_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'messages_intf_%'
    LOOP
        -- Index for error severity
        index_name := format('idx_%s_error_severity', table_record.tablename);
        EXECUTE format('
            CREATE INDEX IF NOT EXISTS %I ON %I(error_severity)
            WHERE error_severity IS NOT NULL
        ', index_name, table_record.tablename);
        RAISE NOTICE 'Ensured index exists: %', index_name;

        -- Index for error timestamp
        index_name := format('idx_%s_error_timestamp', table_record.tablename);
        EXECUTE format('
            CREATE INDEX IF NOT EXISTS %I ON %I(last_error_timestamp)
            WHERE last_error_timestamp IS NOT NULL
        ', index_name, table_record.tablename);
        RAISE NOTICE 'Ensured index exists: %', index_name;

        -- Index for error stage
        index_name := format('idx_%s_error_stage', table_record.tablename);
        EXECUTE format('
            CREATE INDEX IF NOT EXISTS %I ON %I(last_error_stage)
            WHERE last_error_stage IS NOT NULL
        ', index_name, table_record.tablename);
        RAISE NOTICE 'Ensured index exists: %', index_name;
    END LOOP;
END $$;

-- ============================================
-- Do the same for output_intf_* tables
-- ============================================

DO $$
DECLARE
    table_record RECORD;
BEGIN
    FOR table_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'output_intf_%'
    LOOP
        -- Add V23 error handling columns
        EXECUTE format('
            ALTER TABLE %I
            ADD COLUMN IF NOT EXISTS error_stack JSONB DEFAULT ''[]''::jsonb,
            ADD COLUMN IF NOT EXISTS last_error_timestamp TIMESTAMP WITH TIME ZONE,
            ADD COLUMN IF NOT EXISTS last_error_stage VARCHAR(50),
            ADD COLUMN IF NOT EXISTS error_severity VARCHAR(20)
        ', table_record.tablename);

        RAISE NOTICE 'Added V23 error columns to output table: %', table_record.tablename;
    END LOOP;
END $$;

-- ============================================
-- Verification Query
-- ============================================

-- This query helps verify all tables have required columns
DO $$
DECLARE
    missing_columns TEXT := '';
    table_record RECORD;
    has_parsed_at BOOLEAN;
    has_error_stack BOOLEAN;
BEGIN
    FOR table_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'messages_intf_%'
    LOOP
        -- Check for parsed_at
        SELECT EXISTS(
            SELECT 1 FROM information_schema.columns
            WHERE table_name = table_record.tablename
            AND column_name = 'parsed_at'
        ) INTO has_parsed_at;

        -- Check for error_stack
        SELECT EXISTS(
            SELECT 1 FROM information_schema.columns
            WHERE table_name = table_record.tablename
            AND column_name = 'error_stack'
        ) INTO has_error_stack;

        IF NOT has_parsed_at OR NOT has_error_stack THEN
            missing_columns := missing_columns || table_record.tablename || ', ';
        END IF;
    END LOOP;

    IF missing_columns != '' THEN
        RAISE WARNING 'Tables still missing columns: %', missing_columns;
    ELSE
        RAISE NOTICE '✅ All interface tables have required columns';
    END IF;
END $$;

-- Add comment
COMMENT ON SCHEMA public IS 'V24: Fixed missing columns in interface tables created before V19 and V23 migrations';
