-- V67: Interface-Level Templates
-- Stores complete interface configurations (source + pipeline + destination)
-- that users can browse, preview, and use to create new interfaces quickly.
-- Separate from transformation_templates (which stores individual pipeline steps).

CREATE TABLE IF NOT EXISTS interface_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,

    -- Category for gallery grouping
    -- Values: 'emr', 'hl7v2', 'fhir', 'payer', 'specialty', 'custom'
    category VARCHAR(100) NOT NULL DEFAULT 'custom',
    subcategory VARCHAR(100),          -- e.g. 'epic', 'cerner' within 'emr'

    tags TEXT[] DEFAULT '{}',
    icon VARCHAR(10) DEFAULT '📋',
    difficulty VARCHAR(20) DEFAULT 'intermediate', -- 'beginner' | 'intermediate' | 'advanced'

    -- Source connector scaffold (sensitive fields cleared)
    source_connector_type VARCHAR(100),
    source_config_template JSONB NOT NULL DEFAULT '{}',

    -- Destination connector scaffold (sensitive fields cleared)
    target_connector_type VARCHAR(100),
    target_config_template JSONB NOT NULL DEFAULT '{}',

    -- Full pipeline definition — execution_groups with all steps preserved
    pipeline_config JSONB NOT NULL DEFAULT '{}',
    message_type VARCHAR(100),         -- e.g. 'ADT^A01', 'ORU^R01'

    -- Manifest of fields the user must fill in after creating from template
    -- Array of {section, field, label, type, hint}
    required_connection_fields JSONB NOT NULL DEFAULT '[]',

    -- Which fields were cleared during sanitization (for transparency in UI)
    sanitized_fields TEXT[] DEFAULT '{}',

    -- Gallery card preview
    preview_steps JSONB NOT NULL DEFAULT '[]',   -- [{name, icon, step_type}]
    estimated_setup_minutes INTEGER DEFAULT 15,

    -- Ownership & visibility
    is_system BOOLEAN NOT NULL DEFAULT FALSE,    -- OOB factory templates
    is_public BOOLEAN NOT NULL DEFAULT TRUE,
    author VARCHAR(255),                         -- 'ezHealthKonnect' for OOB
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    usage_count INTEGER NOT NULL DEFAULT 0,

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_intf_templates_category ON interface_templates(category);
CREATE INDEX IF NOT EXISTS idx_intf_templates_subcategory ON interface_templates(subcategory);
CREATE INDEX IF NOT EXISTS idx_intf_templates_system ON interface_templates(is_system);
CREATE INDEX IF NOT EXISTS idx_intf_templates_slug ON interface_templates(slug);
CREATE INDEX IF NOT EXISTS idx_intf_templates_public ON interface_templates(is_public) WHERE is_public = TRUE;

-- Auto-update updated_at
CREATE OR REPLACE FUNCTION update_interface_templates_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_interface_templates_updated_at
    BEFORE UPDATE ON interface_templates
    FOR EACH ROW EXECUTE FUNCTION update_interface_templates_updated_at();
