-- V19__Add_Parsing_Columns.sql
-- Add columns for JSON parsing tracking

-- Function to add parsing columns to a message table
CREATE OR REPLACE FUNCTION add_parsing_columns_to_table(p_table_name TEXT) RETURNS VOID AS $$
DECLARE
    column_exists BOOLEAN;
BEGIN
    -- Check and add parsed_at column
    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table_name AND column_name = 'parsed_at'
    ) INTO column_exists;

    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN parsed_at TIMESTAMP WITH TIME ZONE', p_table_name);
        RAISE NOTICE 'Added parsed_at to %', p_table_name;
    END IF;

    -- Check and add parsing_status column
    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table_name AND column_name = 'parsing_status'
    ) INTO column_exists;

    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN parsing_status VARCHAR(50)', p_table_name);
        RAISE NOTICE 'Added parsing_status to %', p_table_name;
    END IF;

    -- Check and add parsing_time_ms column
    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table_name AND column_name = 'parsing_time_ms'
    ) INTO column_exists;

    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN parsing_time_ms INTEGER', p_table_name);
        RAISE NOTICE 'Added parsing_time_ms to %', p_table_name;
    END IF;

    -- Check and add parsing_error column
    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = p_table_name AND column_name = 'parsing_error'
    ) INTO column_exists;

    IF NOT column_exists THEN
        EXECUTE format('ALTER TABLE %I ADD COLUMN parsing_error TEXT', p_table_name);
        RAISE NOTICE 'Added parsing_error to %', p_table_name;
    END IF;

END;
$$ LANGUAGE plpgsql;

-- Apply to all existing interface message tables
DO $$
DECLARE
    table_rec RECORD;
BEGIN
    FOR table_rec IN
        SELECT table_name
        FROM information_schema.tables
        WHERE table_schema = 'public'
        AND table_name LIKE 'messages_intf_%'
    LOOP
        PERFORM add_parsing_columns_to_table(table_rec.table_name);
    END LOOP;
END $$;

-- Create comment
COMMENT ON FUNCTION add_parsing_columns_to_table IS 'Adds parsing tracking columns to interface message tables';
