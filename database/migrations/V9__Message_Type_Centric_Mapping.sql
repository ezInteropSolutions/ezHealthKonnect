-- Migration V9: Message-Type-Centric HL7-FHIR Mapping Architecture
-- Purpose: Enable proper multi-message type support with efficient storage
-- Replaces the interface-level mapping with message-type-specific mappings

-- =========================================================================
-- STEP 1: Create Standard Templates Table (Reference Library)
-- =========================================================================

CREATE TABLE IF NOT EXISTS hl7_fhir_templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_type VARCHAR(50) NOT NULL,
    hl7_version VARCHAR(10) DEFAULT '2.5',
    template_name VARCHAR(100) NOT NULL,
    template_description TEXT,
    template_config JSONB NOT NULL,
    is_default BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id),

    CONSTRAINT unique_default_template UNIQUE(message_type, hl7_version, is_default)
);

-- =========================================================================
-- STEP 2: Create Interface-Message-Type Mapping Table (Core Architecture)
-- =========================================================================

CREATE TABLE IF NOT EXISTS interface_message_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    message_type VARCHAR(50) NOT NULL,

    -- Smart mapping resolution
    uses_standard_template BOOLEAN DEFAULT true,
    standard_template_id UUID REFERENCES hl7_fhir_templates(id),
    custom_mapping_config JSONB,

    -- Tracking and metadata
    customized_fields TEXT[] DEFAULT '{}',
    customization_reason TEXT,
    mapping_version INTEGER DEFAULT 1,

    -- Audit fields
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id),
    last_tested_at TIMESTAMP WITH TIME ZONE,

    CONSTRAINT unique_interface_message UNIQUE(interface_id, message_type)
);

-- =========================================================================
-- STEP 3: Update interfaces table for multi-message support
-- =========================================================================

-- Add supported_message_types if not exists (keep as reference)
ALTER TABLE interfaces
ADD COLUMN IF NOT EXISTS supported_message_types JSONB DEFAULT '[]';

-- Add mapping metadata columns
ALTER TABLE interfaces
ADD COLUMN IF NOT EXISTS total_message_types INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_mapping_update TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;

-- =========================================================================
-- STEP 4: Create Performance Indexes
-- =========================================================================

-- Interface-message lookup (most common query)
CREATE INDEX IF NOT EXISTS idx_interface_message_mappings_lookup
ON interface_message_mappings(interface_id, message_type);

-- Template usage tracking
CREATE INDEX IF NOT EXISTS idx_interface_message_mappings_template
ON interface_message_mappings(standard_template_id) WHERE uses_standard_template = true;

-- Custom mapping queries
CREATE INDEX IF NOT EXISTS idx_interface_message_mappings_custom
ON interface_message_mappings(interface_id) WHERE uses_standard_template = false;

-- Template queries
CREATE INDEX IF NOT EXISTS idx_hl7_fhir_templates_message_type
ON hl7_fhir_templates(message_type, is_default);

-- JSONB indexes for fast config access
CREATE INDEX IF NOT EXISTS idx_templates_config
ON hl7_fhir_templates USING gin(template_config);

CREATE INDEX IF NOT EXISTS idx_custom_mappings_config
ON interface_message_mappings USING gin(custom_mapping_config);

-- Interfaces supported message types
CREATE INDEX IF NOT EXISTS idx_interfaces_supported_messages
ON interfaces USING gin(supported_message_types);

-- =========================================================================
-- STEP 5: Create Standard Templates (Seed Data)
-- =========================================================================

-- ADT^A01 - Admission/Discharge/Transfer
INSERT INTO hl7_fhir_templates (message_type, template_name, template_description, template_config) VALUES
('ADT^A01', 'Standard ADT A01 Mapping', 'Standard HL7 ADT A01 to FHIR Patient/Encounter mapping',
'{
  "messageType": "ADT^A01",
  "version": "1.0",
  "mappings": {
    "Patient": {
      "resourceType": "Patient",
      "mappings": [
        {"hl7Path": "PID.3.1", "fhirPath": "identifier[0].value", "required": true},
        {"hl7Path": "PID.5.1", "fhirPath": "name[0].family", "required": true},
        {"hl7Path": "PID.5.2", "fhirPath": "name[0].given[0]", "required": true},
        {"hl7Path": "PID.7", "fhirPath": "birthDate", "transform": "date"},
        {"hl7Path": "PID.8", "fhirPath": "gender", "transform": "gender"}
      ]
    },
    "Encounter": {
      "resourceType": "Encounter",
      "mappings": [
        {"hl7Path": "PV1.2", "fhirPath": "class.code", "required": true},
        {"hl7Path": "PV1.44", "fhirPath": "period.start", "transform": "datetime"},
        {"hl7Path": "PV1.45", "fhirPath": "period.end", "transform": "datetime"}
      ]
    }
  }
}')
ON CONFLICT (message_type, hl7_version, is_default) DO NOTHING;

-- ORU^R01 - Lab Results
INSERT INTO hl7_fhir_templates (message_type, template_name, template_description, template_config) VALUES
('ORU^R01', 'Standard ORU R01 Mapping', 'Standard HL7 ORU R01 to FHIR Observation mapping',
'{
  "messageType": "ORU^R01",
  "version": "1.0",
  "mappings": {
    "Patient": {
      "resourceType": "Patient",
      "mappings": [
        {"hl7Path": "PID.3.1", "fhirPath": "identifier[0].value", "required": true},
        {"hl7Path": "PID.5.1", "fhirPath": "name[0].family", "required": true},
        {"hl7Path": "PID.5.2", "fhirPath": "name[0].given[0]", "required": true}
      ]
    },
    "Observation": {
      "resourceType": "Observation",
      "mappings": [
        {"hl7Path": "OBX.3", "fhirPath": "code.coding[0].code", "required": true},
        {"hl7Path": "OBX.5", "fhirPath": "valueString", "required": true},
        {"hl7Path": "OBX.6", "fhirPath": "valueQuantity.unit"},
        {"hl7Path": "OBX.14", "fhirPath": "effectiveDateTime", "transform": "datetime"}
      ]
    }
  }
}')
ON CONFLICT (message_type, hl7_version, is_default) DO NOTHING;

-- =========================================================================
-- STEP 6: Create Functions for Mapping Resolution
-- =========================================================================

-- Function to get effective mapping configuration
CREATE OR REPLACE FUNCTION get_effective_mapping_config(p_interface_id UUID, p_message_type VARCHAR)
RETURNS JSONB AS $$
DECLARE
    result_config JSONB;
BEGIN
    SELECT
        CASE
            WHEN imm.uses_standard_template THEN t.template_config
            ELSE imm.custom_mapping_config
        END
    INTO result_config
    FROM interface_message_mappings imm
    LEFT JOIN hl7_fhir_templates t ON imm.standard_template_id = t.id
    WHERE imm.interface_id = p_interface_id
      AND imm.message_type = p_message_type;

    RETURN result_config;
END;
$$ LANGUAGE plpgsql STABLE;

-- Function to update interface message type count
CREATE OR REPLACE FUNCTION update_interface_message_count()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE interfaces
    SET
        total_message_types = (
            SELECT COUNT(DISTINCT message_type)
            FROM interface_message_mappings
            WHERE interface_id = COALESCE(NEW.interface_id, OLD.interface_id)
        ),
        supported_message_types = (
            SELECT JSONB_AGG(DISTINCT message_type)
            FROM interface_message_mappings
            WHERE interface_id = COALESCE(NEW.interface_id, OLD.interface_id)
        ),
        last_mapping_update = CURRENT_TIMESTAMP
    WHERE id = COALESCE(NEW.interface_id, OLD.interface_id);

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

-- =========================================================================
-- STEP 7: Create Triggers for Automatic Updates
-- =========================================================================

-- Trigger to update interface counts when mappings change
DROP TRIGGER IF EXISTS tr_update_interface_message_count ON interface_message_mappings;
CREATE TRIGGER tr_update_interface_message_count
    AFTER INSERT OR UPDATE OR DELETE ON interface_message_mappings
    FOR EACH ROW
    EXECUTE FUNCTION update_interface_message_count();

-- Trigger to update timestamps
CREATE OR REPLACE FUNCTION update_mapping_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    IF TG_OP = 'UPDATE' AND OLD.custom_mapping_config IS DISTINCT FROM NEW.custom_mapping_config THEN
        NEW.mapping_version = COALESCE(OLD.mapping_version, 0) + 1;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tr_update_mapping_timestamp ON interface_message_mappings;
CREATE TRIGGER tr_update_mapping_timestamp
    BEFORE UPDATE ON interface_message_mappings
    FOR EACH ROW
    EXECUTE FUNCTION update_mapping_timestamp();

-- =========================================================================
-- STEP 8: Add Column Comments for Documentation
-- =========================================================================

COMMENT ON TABLE hl7_fhir_templates IS 'Standard HL7-FHIR mapping templates for reuse across interfaces';
COMMENT ON TABLE interface_message_mappings IS 'Message-type-specific mapping configurations per interface';

COMMENT ON COLUMN hl7_fhir_templates.template_config IS 'Complete FHIR mapping configuration in JSON format';
COMMENT ON COLUMN interface_message_mappings.uses_standard_template IS 'True if using standard template, false if using custom mapping';
COMMENT ON COLUMN interface_message_mappings.custom_mapping_config IS 'Custom mapping configuration (only populated if not using standard template)';
COMMENT ON COLUMN interface_message_mappings.customized_fields IS 'Array of field paths that have been customized from standard template';

COMMENT ON COLUMN interfaces.supported_message_types IS 'JSON array of message types supported by this interface (auto-updated)';
COMMENT ON COLUMN interfaces.total_message_types IS 'Count of distinct message types supported (auto-updated)';

-- =========================================================================
-- STEP 9: Migration of Existing Data (if any)
-- =========================================================================

-- Migrate existing transformation_mapping data from interfaces table
DO $$
BEGIN
    -- Only migrate if transformation_mapping column exists and has data
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'interfaces' AND column_name = 'transformation_mapping'
    ) THEN
        -- Migrate existing mappings to new structure
        INSERT INTO interface_message_mappings (
            interface_id,
            message_type,
            uses_standard_template,
            custom_mapping_config,
            created_by
        )
        SELECT
            id,
            COALESCE(message_type, 'ADT^A01') as message_type,
            false, -- Mark as custom since they came from wizard
            transformation_mapping,
            user_id
        FROM interfaces
        WHERE transformation_mapping IS NOT NULL
          AND transformation_mapping != '{}'::jsonb
          AND transformation_mapping != 'null'::jsonb;

        RAISE NOTICE 'Migrated existing transformation mappings to new schema';
    END IF;
END $$;

-- =========================================================================
-- STEP 10: Verification and Cleanup
-- =========================================================================

-- Create view for easy querying
CREATE OR REPLACE VIEW v_interface_mappings AS
SELECT
    i.id as interface_id,
    i.name as interface_name,
    imm.message_type,
    imm.uses_standard_template,
    CASE
        WHEN imm.uses_standard_template THEN t.template_name
        ELSE 'Custom Mapping'
    END as mapping_source,
    imm.mapping_version,
    imm.updated_at as last_mapping_update
FROM interfaces i
JOIN interface_message_mappings imm ON i.id = imm.interface_id
LEFT JOIN hl7_fhir_templates t ON imm.standard_template_id = t.id
ORDER BY i.name, imm.message_type;

-- Grant necessary permissions
GRANT SELECT ON hl7_fhir_templates TO PUBLIC;
GRANT SELECT ON interface_message_mappings TO PUBLIC;
GRANT SELECT ON v_interface_mappings TO PUBLIC;

-- =========================================================================
-- STEP 11: Migration Summary
-- =========================================================================

DO $$
DECLARE
    template_count INTEGER;
    mapping_count INTEGER;
    interface_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO template_count FROM hl7_fhir_templates;
    SELECT COUNT(*) INTO mapping_count FROM interface_message_mappings;
    SELECT COUNT(*) INTO interface_count FROM interfaces WHERE supported_message_types IS NOT NULL;

    RAISE NOTICE '=== V9 Migration Completed Successfully ===';
    RAISE NOTICE 'Created standard templates: %', template_count;
    RAISE NOTICE 'Created message mappings: %', mapping_count;
    RAISE NOTICE 'Updated interfaces: %', interface_count;
    RAISE NOTICE '✅ Message-type-centric architecture ready!';
    RAISE NOTICE '📊 Templates: hl7_fhir_templates table';
    RAISE NOTICE '🔗 Mappings: interface_message_mappings table';
    RAISE NOTICE '👀 View: v_interface_mappings for easy querying';
END $$;