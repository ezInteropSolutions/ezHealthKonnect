-- V38: Add Response Mapping Templates for API Enrichment Steps
-- ================================================================
-- This migration adds support for reusable response mapping templates
-- that can be referenced from API enrichment step configurations.
--
-- Design: Templates are stored in a library and referenced via step config JSONB
-- Alignment: Follows existing pipeline → steps → config pattern
-- Reusability: Templates can be shared across steps, interfaces, and organizations

-- ================================================================
-- Response Mapping Templates (Reusable Library)
-- ================================================================

CREATE TABLE IF NOT EXISTS response_mapping_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Template identification
    template_name VARCHAR(200) NOT NULL,
    description TEXT,

    -- Template categorization
    api_type VARCHAR(100),  -- 'empi', 'ehr', 'lims', 'insurance', 'pharmacy', etc.
    vendor VARCHAR(100),    -- 'epic', 'cerner', 'allscripts', 'meditech', 'custom', etc.

    -- Mapping rules (array of extractor configurations)
    -- Each rule defines: sourcePath (JSONPath), targetField, transformType, transformConfig
    mapping_rules JSONB NOT NULL,

    -- Sharing & ownership
    is_system_template BOOLEAN DEFAULT false,  -- System-provided vs user-created
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    organization_id UUID,  -- NULL = shared across all organizations

    -- Versioning & status
    version INTEGER DEFAULT 1,
    is_active BOOLEAN DEFAULT true,

    -- Audit timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Constraints
    CONSTRAINT unique_template_name_version UNIQUE (template_name, version),
    CONSTRAINT valid_mapping_rules CHECK (jsonb_typeof(mapping_rules) = 'array')
);

-- ================================================================
-- Indexes for Performance
-- ================================================================

-- Fast template lookup by type and vendor
CREATE INDEX idx_response_templates_type ON response_mapping_templates(api_type, vendor) WHERE is_active = true;

-- Fast lookup of system templates
CREATE INDEX idx_response_templates_system ON response_mapping_templates(is_system_template) WHERE is_active = true;

-- Fast lookup by organization
CREATE INDEX idx_response_templates_org ON response_mapping_templates(organization_id) WHERE is_active = true AND organization_id IS NOT NULL;

-- Fast lookup by creator
CREATE INDEX idx_response_templates_creator ON response_mapping_templates(created_by) WHERE is_active = true;

-- Full-text search on template name and description
CREATE INDEX idx_response_templates_search ON response_mapping_templates USING gin(to_tsvector('english', template_name || ' ' || COALESCE(description, '')));

-- ================================================================
-- Seed Data: System Templates
-- ================================================================

-- Epic EMPI Patient Lookup Template
INSERT INTO response_mapping_templates (
    template_name,
    description,
    api_type,
    vendor,
    is_system_template,
    mapping_rules
) VALUES (
    'Epic EMPI Patient Lookup',
    'Standard Epic EMPI patient demographic lookup response mapping. Extracts patient ID, name, date of birth, and primary insurance.',
    'empi',
    'epic',
    true,
    '[
        {
            "sourcePath": "$.patient.id",
            "targetField": "patientId",
            "transformType": "none",
            "required": true,
            "description": "Extract patient ID"
        },
        {
            "sourcePath": "$.patient.firstName",
            "targetField": "patientName",
            "transformType": "combine",
            "transformConfig": {
                "additionalPaths": ["$.patient.lastName"],
                "separator": " ",
                "format": "{0} {1}"
            },
            "required": true,
            "description": "Combine first and last name"
        },
        {
            "sourcePath": "$.patient.dateOfBirth",
            "targetField": "dateOfBirth",
            "transformType": "format",
            "transformConfig": {
                "inputFormat": "MM/DD/YYYY",
                "outputFormat": "YYYY-MM-DD"
            },
            "required": false,
            "description": "Convert date format to ISO 8601"
        },
        {
            "sourcePath": "$.patient.insurance",
            "targetField": "primaryInsurance",
            "transformType": "filter",
            "transformConfig": {
                "filterField": "type",
                "filterOperator": "equals",
                "filterValue": "primary",
                "extractField": "memberId"
            },
            "required": false,
            "defaultValue": null,
            "description": "Extract primary insurance member ID"
        }
    ]'::jsonb
) ON CONFLICT (template_name, version) DO NOTHING;

-- Cerner Patient Demographics Template
INSERT INTO response_mapping_templates (
    template_name,
    description,
    api_type,
    vendor,
    is_system_template,
    mapping_rules
) VALUES (
    'Cerner Patient Demographics',
    'Standard Cerner patient demographics API response mapping. Extracts patient identifiers, name, and contact information.',
    'ehr',
    'cerner',
    true,
    '[
        {
            "sourcePath": "$.entry[0].resource.id",
            "targetField": "patientId",
            "transformType": "none",
            "required": true,
            "description": "Extract FHIR patient resource ID"
        },
        {
            "sourcePath": "$.entry[0].resource.identifier",
            "targetField": "mrn",
            "transformType": "filter",
            "transformConfig": {
                "filterField": "system",
                "filterOperator": "contains",
                "filterValue": "MRN",
                "extractField": "value"
            },
            "required": true,
            "description": "Extract MRN from identifiers"
        },
        {
            "sourcePath": "$.entry[0].resource.name[0].given[0]",
            "targetField": "patientName",
            "transformType": "combine",
            "transformConfig": {
                "additionalPaths": ["$.entry[0].resource.name[0].family"],
                "separator": " "
            },
            "required": true,
            "description": "Combine given and family name"
        },
        {
            "sourcePath": "$.entry[0].resource.telecom",
            "targetField": "phoneNumber",
            "transformType": "filter",
            "transformConfig": {
                "filterField": "system",
                "filterOperator": "equals",
                "filterValue": "phone",
                "extractField": "value"
            },
            "required": false,
            "description": "Extract phone number from telecom"
        }
    ]'::jsonb
) ON CONFLICT (template_name, version) DO NOTHING;

-- Generic REST API Simple Extract Template
INSERT INTO response_mapping_templates (
    template_name,
    description,
    api_type,
    vendor,
    is_system_template,
    mapping_rules
) VALUES (
    'Generic JSON API - Simple Extract',
    'Simple template for extracting top-level fields from a generic JSON API response.',
    'custom',
    'custom',
    true,
    '[
        {
            "sourcePath": "$.id",
            "targetField": "id",
            "transformType": "none",
            "required": false,
            "description": "Extract ID field"
        },
        {
            "sourcePath": "$.data",
            "targetField": "data",
            "transformType": "none",
            "required": false,
            "description": "Extract data object"
        },
        {
            "sourcePath": "$.status",
            "targetField": "status",
            "transformType": "none",
            "required": false,
            "description": "Extract status field"
        }
    ]'::jsonb
) ON CONFLICT (template_name, version) DO NOTHING;

-- ================================================================
-- Comments
-- ================================================================

COMMENT ON TABLE response_mapping_templates IS 'Reusable response mapping templates for API enrichment steps. Templates define how to extract and transform specific fields from API responses.';
COMMENT ON COLUMN response_mapping_templates.template_name IS 'Unique name for the template (e.g., "Epic EMPI Patient Lookup")';
COMMENT ON COLUMN response_mapping_templates.api_type IS 'Category of API (empi, ehr, lims, insurance, etc.)';
COMMENT ON COLUMN response_mapping_templates.vendor IS 'API vendor/provider (epic, cerner, custom, etc.)';
COMMENT ON COLUMN response_mapping_templates.mapping_rules IS 'Array of extractor rules. Each rule: {sourcePath, targetField, transformType, transformConfig, required, defaultValue}';
COMMENT ON COLUMN response_mapping_templates.is_system_template IS 'True if provided by system, false if user-created';
COMMENT ON COLUMN response_mapping_templates.organization_id IS 'If set, template is only visible to this organization. NULL = shared globally.';

-- ================================================================
-- Update Trigger
-- ================================================================

CREATE OR REPLACE FUNCTION update_response_template_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_response_template_timestamp
    BEFORE UPDATE ON response_mapping_templates
    FOR EACH ROW
    EXECUTE FUNCTION update_response_template_timestamp();

-- ================================================================
-- Usage Notes
-- ================================================================

-- How to use templates in step configuration:
--
-- 1. Use template as-is:
-- {
--   "step_type": "pre.enrichment.api",
--   "config": {
--     "endpoint": "...",
--     "responseMapping": {
--       "templateId": "template-uuid-here",
--       "mode": "template"
--     }
--   }
-- }
--
-- 2. Extend template with custom fields:
-- {
--   "step_type": "pre.enrichment.api",
--   "config": {
--     "endpoint": "...",
--     "responseMapping": {
--       "templateId": "template-uuid-here",
--       "mode": "extend",
--       "customExtractors": [
--         {"sourcePath": "$.custom.field", "targetField": "customData"}
--       ]
--     }
--   }
-- }
--
-- 3. Fully custom (no template):
-- {
--   "step_type": "pre.enrichment.api",
--   "config": {
--     "endpoint": "...",
--     "responseMapping": {
--       "mode": "custom",
--       "extractors": [
--         {"sourcePath": "$.data.id", "targetField": "id"}
--       ]
--     }
--   }
-- }
