-- V55: Add outbound_content_uri column to track the exact payload delivered to each connector.
-- This is distinct from transformed_content_uri (which stores the full pipeline context).
-- Added to standardized schema for all new interface tables.

-- Apply to existing interface tables dynamically via a DO block
DO $$
DECLARE
    tbl TEXT;
BEGIN
    FOR tbl IN
        SELECT table_name FROM interface_table_metadata WHERE table_name IS NOT NULL
    LOOP
        BEGIN
            EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS outbound_content_uri TEXT', tbl);
        EXCEPTION WHEN others THEN
            RAISE WARNING 'Could not add outbound_content_uri to %: %', tbl, SQLERRM;
        END;
    END LOOP;
END $$;
