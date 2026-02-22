-- V15: Fix Dedicated Interface Table Schema
-- Purpose: Add missing columns to match InterfaceTableManager query expectations

-- =========================================================================
-- Conditionally fix existing dedicated tables (only if they exist)
-- These tables are created dynamically per-interface, so they may not exist on fresh installs
-- =========================================================================

DO $$
BEGIN
    -- Fix Test Interface1 dedicated table schema (if it exists)
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'messages_intf_146941d7_dc19_4e') THEN
        ALTER TABLE messages_intf_146941d7_dc19_4e
        ADD COLUMN IF NOT EXISTS priority INTEGER DEFAULT 5,
        ADD COLUMN IF NOT EXISTS message_size INTEGER,
        ADD COLUMN IF NOT EXISTS source_type VARCHAR(100),
        ADD COLUMN IF NOT EXISTS source_endpoint VARCHAR(500),
        ADD COLUMN IF NOT EXISTS processing_time_ms INTEGER,
        ADD COLUMN IF NOT EXISTS error_count INTEGER DEFAULT 0,
        ADD COLUMN IF NOT EXISTS last_error_message TEXT,
        ADD COLUMN IF NOT EXISTS delivery_status VARCHAR(50),
        ADD COLUMN IF NOT EXISTS delivery_attempts INTEGER DEFAULT 0;

        UPDATE messages_intf_146941d7_dc19_4e
        SET priority = 5, source_type = source,
            message_size = COALESCE(LENGTH(raw_message), 0),
            error_count = 0, delivery_attempts = 0
        WHERE priority IS NULL OR source_type IS NULL;
    END IF;

    -- Fix FHIR Receiver Interface dedicated table schema (if it exists)
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'messages_intf_90d34743_5fc9_4e') THEN
        ALTER TABLE messages_intf_90d34743_5fc9_4e
        ADD COLUMN IF NOT EXISTS priority INTEGER DEFAULT 5,
        ADD COLUMN IF NOT EXISTS message_size INTEGER,
        ADD COLUMN IF NOT EXISTS source_type VARCHAR(100),
        ADD COLUMN IF NOT EXISTS source_endpoint VARCHAR(500),
        ADD COLUMN IF NOT EXISTS processing_time_ms INTEGER,
        ADD COLUMN IF NOT EXISTS error_count INTEGER DEFAULT 0,
        ADD COLUMN IF NOT EXISTS last_error_message TEXT,
        ADD COLUMN IF NOT EXISTS delivery_status VARCHAR(50),
        ADD COLUMN IF NOT EXISTS delivery_attempts INTEGER DEFAULT 0;

        UPDATE messages_intf_90d34743_5fc9_4e
        SET priority = 5, source_type = COALESCE(source, 'external'),
            message_size = COALESCE(LENGTH(raw_message), 0),
            error_count = 0, delivery_attempts = 0
        WHERE priority IS NULL OR source_type IS NULL;
    END IF;
END$$;

-- =========================================================================
-- Create function to ensure new dedicated tables have correct schema
-- =========================================================================

CREATE OR REPLACE FUNCTION create_interface_message_table(table_name TEXT, interface_id UUID)
RETURNS VOID AS $$
BEGIN
    EXECUTE format('
        CREATE TABLE IF NOT EXISTS %I (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            interface_id UUID NOT NULL,
            message_id VARCHAR(255) NOT NULL UNIQUE,
            correlation_id VARCHAR(255),
            status VARCHAR(50) NOT NULL DEFAULT ''received'',
            priority INTEGER NOT NULL DEFAULT 5,
            received_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
            processing_completed_at TIMESTAMP WITH TIME ZONE,
            source VARCHAR(100) NOT NULL,
            source_type VARCHAR(100),
            source_endpoint VARCHAR(500),
            message_type VARCHAR(100),
            message_size INTEGER,
            content_type VARCHAR(100) DEFAULT ''application/json'',
            raw_message TEXT,
            processed_message TEXT,
            fhir_resource_type VARCHAR(100),
            fhir_resource_id VARCHAR(255),
            processing_time_ms INTEGER,
            error_count INTEGER DEFAULT 0,
            last_error_message TEXT,
            delivery_status VARCHAR(50),
            delivery_attempts INTEGER DEFAULT 0,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            processed_at TIMESTAMP WITH TIME ZONE,
            CONSTRAINT %I_interface_fkey FOREIGN KEY (interface_id) REFERENCES interfaces(id)
        )', table_name, table_name || '_interface');

    -- Add indexes for performance
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_message_id_idx ON %I (message_id)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_status_idx ON %I (status)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_received_at_idx ON %I (received_at DESC)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_interface_status_idx ON %I (interface_id, status)', table_name, table_name);
END;
$$ LANGUAGE plpgsql;

-- =========================================================================
-- Add table metadata for existing tables (only if interfaces exist)
-- =========================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'interface_table_metadata') THEN
        INSERT INTO interface_table_metadata (interface_id, table_name, schema_version)
        SELECT id, 'messages_intf_146941d7_dc19_4e', '1.1'
        FROM interfaces WHERE id = '146941d7-dc19-4ee2-964a-7fe6c1cb429f'
        ON CONFLICT (interface_id) DO UPDATE SET
            schema_version = EXCLUDED.schema_version,
            updated_at = CURRENT_TIMESTAMP;

        INSERT INTO interface_table_metadata (interface_id, table_name, schema_version)
        SELECT id, 'messages_intf_90d34743_5fc9_4e', '1.1'
        FROM interfaces WHERE id = '90d34743-5fc9-4e2e-8751-70aec1d43536'
        ON CONFLICT (interface_id) DO UPDATE SET
            schema_version = EXCLUDED.schema_version,
            updated_at = CURRENT_TIMESTAMP;
    END IF;
END$$;
