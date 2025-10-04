-- V16: Add Output Message Tables for Interface-Specific Storage
-- Purpose: Create output message table structure for storing transformation results
-- Following interface-specific table pattern for performance isolation

-- =========================================================================
-- Create output message table metadata registry
-- =========================================================================

CREATE TABLE IF NOT EXISTS output_table_metadata (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    output_table_name VARCHAR(255) NOT NULL UNIQUE,
    schema_version VARCHAR(10) DEFAULT '1.0',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(interface_id)
);

-- Add updated_at trigger
CREATE OR REPLACE FUNCTION update_output_table_metadata_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER output_table_metadata_updated_at_trigger
    BEFORE UPDATE ON output_table_metadata
    FOR EACH ROW
    EXECUTE FUNCTION update_output_table_metadata_timestamp();

-- =========================================================================
-- Function to create interface-specific output message tables
-- =========================================================================

CREATE OR REPLACE FUNCTION create_interface_output_table(table_name TEXT, interface_id UUID)
RETURNS VOID AS $$
BEGIN
    -- Create interface-specific output table with standardized schema
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

-- =========================================================================
-- Function to get or create output table name for interface
-- =========================================================================

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

    -- Create the table
    PERFORM create_interface_output_table(v_table_name, p_interface_id);

    -- Register in metadata
    INSERT INTO output_table_metadata (interface_id, output_table_name, schema_version)
    VALUES (p_interface_id, v_table_name, '1.0')
    ON CONFLICT (interface_id) DO UPDATE SET
        output_table_name = EXCLUDED.output_table_name,
        schema_version = EXCLUDED.schema_version,
        updated_at = CURRENT_TIMESTAMP;

    RETURN v_table_name;
END;
$$ LANGUAGE plpgsql;

-- =========================================================================
-- Add updated_at trigger function for output tables
-- =========================================================================

CREATE OR REPLACE FUNCTION update_output_message_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- =========================================================================
-- Create initial output tables for existing interfaces
-- =========================================================================

-- Create output tables for Test Interface1 and FHIR Receiver Interface
DO $$
DECLARE
    interface_record RECORD;
    output_table_name TEXT;
BEGIN
    -- Create output tables for all existing interfaces
    FOR interface_record IN
        SELECT id, name FROM interfaces WHERE id IN (
            '146941d7-dc19-4ee2-964a-7fe6c1cb429f', -- Test Interface1
            '90d34743-5fc9-4e2e-8751-70aec1d43536'  -- FHIR Receiver Interface
        )
    LOOP
        -- Get or create output table
        output_table_name := get_interface_output_table(interface_record.id);

        -- Add updated_at trigger to the table
        EXECUTE format('
            CREATE TRIGGER %I_updated_at_trigger
                BEFORE UPDATE ON %I
                FOR EACH ROW
                EXECUTE FUNCTION update_output_message_timestamp()
        ', output_table_name, output_table_name);

        RAISE NOTICE 'Created output table: % for interface: %', output_table_name, interface_record.name;
    END LOOP;
END
$$;

-- =========================================================================
-- Create view for unified output message querying across interfaces
-- =========================================================================

CREATE OR REPLACE VIEW v_output_messages_summary AS
SELECT
    otm.interface_id,
    i.name as interface_name,
    otm.output_table_name,
    otm.schema_version,
    otm.created_at as table_created_at,
    otm.updated_at as table_updated_at
FROM output_table_metadata otm
JOIN interfaces i ON otm.interface_id = i.id
ORDER BY otm.created_at DESC;

-- =========================================================================
-- Grant permissions (commented out - role may not exist in all environments)
-- =========================================================================

-- GRANT SELECT, INSERT, UPDATE, DELETE ON output_table_metadata TO ezhealthkonnect;
-- GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO ezhealthkonnect;
-- GRANT SELECT ON v_output_messages_summary TO ezhealthkonnect;

-- Grant permissions on dynamically created tables (for existing tables)
-- DO $$
-- DECLARE
--     table_record RECORD;
-- BEGIN
--     FOR table_record IN SELECT output_table_name FROM output_table_metadata
--     LOOP
--         EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO ezhealthkonnect', table_record.output_table_name);
--     END LOOP;
-- END
-- $$;