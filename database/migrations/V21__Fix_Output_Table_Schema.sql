-- V21: Fix Output Table Schema for Existing Tables
-- Purpose: Add missing columns to output tables that were created before schema standardization
-- This migration ensures all output tables have the complete standardized schema

-- =========================================================================
-- Function to upgrade existing output tables to standardized schema
-- =========================================================================

CREATE OR REPLACE FUNCTION upgrade_output_table_schema(table_name TEXT)
RETURNS VOID AS $$
BEGIN
    -- Add missing columns if they don't exist
    -- These columns are required by the OutputMessageService but may be missing from older tables

    -- Core identification columns
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS input_message_id VARCHAR(255)', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS correlation_id VARCHAR(255)', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS message_type VARCHAR(100)', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS transformation_pipeline_id UUID', table_name);

    -- Source message metadata
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS source_message_size INTEGER', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS source_message_type VARCHAR(100)', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS source_encoding VARCHAR(50) DEFAULT ''UTF-8''', table_name);

    -- Transformation results (ensure correct column names)
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS transformed_message JSONB', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS transformation_metadata JSONB', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS target_format VARCHAR(50) DEFAULT ''fhir''', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS target_encoding VARCHAR(50) DEFAULT ''UTF-8''', table_name);

    -- Output message details
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS output_message_size INTEGER', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS fhir_resource_type VARCHAR(100)', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS fhir_resource_id VARCHAR(255)', table_name);

    -- Processing metadata
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS transformation_started_at TIMESTAMP WITH TIME ZONE', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS transformation_completed_at TIMESTAMP WITH TIME ZONE', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS transformation_time_ms BIGINT', table_name);

    -- Error handling
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS error_count INTEGER DEFAULT 0', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS last_error_message TEXT', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS validation_status VARCHAR(50) DEFAULT ''valid''', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS validation_errors JSONB', table_name);

    -- Delivery tracking
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS delivery_endpoint VARCHAR(500)', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS last_delivery_attempt TIMESTAMP WITH TIME ZONE', table_name);
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS delivery_response TEXT', table_name);

    -- Audit fields (if missing)
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS created_by VARCHAR(255) DEFAULT ''system''', table_name);

    -- Add missing indexes if they don't exist
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_input_msg_idx ON %I (input_message_id)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_correlation_idx ON %I (correlation_id)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_msg_type_idx ON %I (message_type)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_fhir_resource_idx ON %I (fhir_resource_type, fhir_resource_id)', table_name, table_name);

    -- Add JSONB GIN indexes for efficient querying (if missing)
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_transformed_msg_gin ON %I USING GIN (transformed_message)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_transformation_meta_gin ON %I USING GIN (transformation_metadata)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_validation_errors_gin ON %I USING GIN (validation_errors)', table_name, table_name);

    -- Add unique constraint if missing
    BEGIN
        EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I_input_msg_unique UNIQUE (interface_id, input_message_id)',
                      table_name, table_name);
    EXCEPTION
        WHEN duplicate_table THEN
            -- Constraint already exists, ignore
            NULL;
        WHEN duplicate_object THEN
            -- Constraint already exists with different name, ignore
            NULL;
    END;

    -- Update schema version in metadata
    UPDATE output_table_metadata
    SET schema_version = '1.1',
        updated_at = CURRENT_TIMESTAMP
    WHERE output_table_name = table_name;

    RAISE NOTICE 'Upgraded output table schema: %', table_name;
END;
$$ LANGUAGE plpgsql;

-- =========================================================================
-- Apply schema upgrade to all existing output tables
-- =========================================================================

DO $$
DECLARE
    table_record RECORD;
BEGIN
    -- Upgrade all existing output tables
    FOR table_record IN
        SELECT output_table_name
        FROM output_table_metadata
        WHERE schema_version = '1.0' OR schema_version IS NULL
    LOOP
        PERFORM upgrade_output_table_schema(table_record.output_table_name);
        RAISE NOTICE 'Processed table: %', table_record.output_table_name;
    END LOOP;

    -- Also check for any output_intf_* tables that might not be in metadata
    FOR table_record IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename LIKE 'output_intf_%'
        AND tablename NOT IN (SELECT output_table_name FROM output_table_metadata)
    LOOP
        RAISE WARNING 'Found orphaned output table not in metadata: %', table_record.tablename;
        -- Optionally upgrade these as well
        PERFORM upgrade_output_table_schema(table_record.tablename);
    END LOOP;
END
$$;

-- =========================================================================
-- Update the create_interface_output_table function to use schema version 1.1
-- =========================================================================

CREATE OR REPLACE FUNCTION create_interface_output_table(table_name TEXT, interface_id UUID)
RETURNS VOID AS $$
BEGIN
    -- Create interface-specific output table with standardized schema v1.1
    EXECUTE format('
        CREATE TABLE IF NOT EXISTS %I (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            interface_id UUID NOT NULL,
            input_message_id VARCHAR(255), -- Link to input message
            correlation_id VARCHAR(255), -- Same correlation as input
            transformation_pipeline_id UUID, -- Link to transformation pipeline used
            message_type VARCHAR(100), -- HL7 message type (ADT^A01, etc.)
            status VARCHAR(50) NOT NULL DEFAULT ''transformed'', -- transformed, failed, delivered
            priority INTEGER NOT NULL DEFAULT 5,

            -- Source message metadata
            source_message_size INTEGER,
            source_message_type VARCHAR(100),
            source_encoding VARCHAR(50) DEFAULT ''UTF-8'',

            -- Transformation results
            transformed_message JSONB, -- Structured FHIR/target message
            transformation_metadata JSONB, -- Transformation details, statistics
            target_format VARCHAR(50) DEFAULT ''fhir'', -- fhir, json, xml, etc.
            target_encoding VARCHAR(50) DEFAULT ''UTF-8'',

            -- Output message details
            output_message_size INTEGER,
            fhir_resource_type VARCHAR(100), -- Patient, Observation, etc.
            fhir_resource_id VARCHAR(255), -- FHIR resource identifier

            -- Processing metadata
            transformation_started_at TIMESTAMP WITH TIME ZONE,
            transformation_completed_at TIMESTAMP WITH TIME ZONE,
            transformation_time_ms BIGINT,

            -- Error handling
            error_count INTEGER DEFAULT 0,
            last_error_message TEXT,
            validation_status VARCHAR(50) DEFAULT ''valid'', -- valid, invalid, warning
            validation_errors JSONB, -- FHIR validation errors if any

            -- Delivery tracking
            delivery_status VARCHAR(50) DEFAULT ''pending'', -- pending, delivered, failed
            delivery_attempts INTEGER DEFAULT 0,
            delivery_endpoint VARCHAR(500), -- Where message should be delivered
            last_delivery_attempt TIMESTAMP WITH TIME ZONE,
            delivery_response TEXT, -- Response from delivery endpoint

            -- Audit fields
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            created_by VARCHAR(255) DEFAULT ''system'',

            -- Foreign key constraints
            CONSTRAINT %I_interface_fkey FOREIGN KEY (interface_id) REFERENCES interfaces(id) ON DELETE CASCADE,
            CONSTRAINT %I_input_msg_unique UNIQUE (interface_id, input_message_id)
        )', table_name, table_name || '_interface', table_name || '_input_unique');

    -- Add performance indexes
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_input_msg_idx ON %I (input_message_id)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_correlation_idx ON %I (correlation_id)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_status_idx ON %I (status)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_msg_type_idx ON %I (message_type)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_created_at_idx ON %I (created_at DESC)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_interface_status_idx ON %I (interface_id, status)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_delivery_status_idx ON %I (delivery_status)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_fhir_resource_idx ON %I (fhir_resource_type, fhir_resource_id)', table_name, table_name);

    -- Add JSONB indexes for efficient querying
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_transformed_msg_gin ON %I USING GIN (transformed_message)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_transformation_meta_gin ON %I USING GIN (transformation_metadata)', table_name, table_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I_validation_errors_gin ON %I USING GIN (validation_errors)', table_name, table_name);

END;
$$ LANGUAGE plpgsql;

-- Update get_interface_output_table to use schema version 1.1
CREATE OR REPLACE FUNCTION get_interface_output_table(p_interface_id UUID)
RETURNS TEXT AS $$
DECLARE
    v_table_name TEXT;
    v_interface_uuid_str TEXT;
BEGIN
    -- Check if output table already exists in metadata
    SELECT output_table_name INTO v_table_name
    FROM output_table_metadata
    WHERE interface_id = p_interface_id;

    IF v_table_name IS NOT NULL THEN
        RETURN v_table_name;
    END IF;

    -- Generate new table name based on interface UUID
    v_interface_uuid_str := REPLACE(p_interface_id::TEXT, '-', '_');
    v_table_name := 'output_intf_' || v_interface_uuid_str;

    -- Create the table with schema v1.1
    PERFORM create_interface_output_table(v_table_name, p_interface_id);

    -- Register in metadata with schema version 1.1
    INSERT INTO output_table_metadata (interface_id, output_table_name, schema_version)
    VALUES (p_interface_id, v_table_name, '1.1')
    ON CONFLICT (interface_id) DO UPDATE SET
        output_table_name = EXCLUDED.output_table_name,
        schema_version = '1.1',
        updated_at = CURRENT_TIMESTAMP;

    RETURN v_table_name;
END;
$$ LANGUAGE plpgsql;

-- =========================================================================
-- Summary
-- =========================================================================

-- Migration V21 Summary:
-- 1. Created upgrade_output_table_schema() function to add missing columns
-- 2. Upgraded all existing output tables to schema version 1.1
-- 3. Updated create_interface_output_table() to create v1.1 tables
-- 4. Updated get_interface_output_table() to register v1.1 schema version
-- 5. Detected and upgraded any orphaned output tables not in metadata
--
-- Schema v1.1 ensures all output tables have complete standardized columns
-- for storing transformation results with full metadata and delivery tracking.
