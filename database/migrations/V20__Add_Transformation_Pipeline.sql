-- V20: Transformation Pipeline Architecture for HL7→FHIR Processing
-- MVP Focus: HL7 to FHIR conversion with configurable transformation steps

-- ============================================================================
-- 1. TRANSFORMATION PIPELINES TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS transformation_pipelines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
    message_type VARCHAR(50) NOT NULL, -- ADT^A01, ORU^R01, etc.
    pipeline_name VARCHAR(255) NOT NULL,
    enabled BOOLEAN DEFAULT true,
    version INTEGER DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Ensure one pipeline per interface/message type combination
    CONSTRAINT uq_pipeline_interface_message UNIQUE (interface_id, message_type)
);

CREATE INDEX idx_transformation_pipelines_interface ON transformation_pipelines(interface_id);
CREATE INDEX idx_transformation_pipelines_message_type ON transformation_pipelines(message_type);
CREATE INDEX idx_transformation_pipelines_enabled ON transformation_pipelines(enabled) WHERE enabled = true;

-- ============================================================================
-- 2. TRANSFORMATION STEPS TABLE
-- ============================================================================
CREATE TABLE IF NOT EXISTS transformation_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id UUID NOT NULL REFERENCES transformation_pipelines(id) ON DELETE CASCADE,
    step_name VARCHAR(255) NOT NULL,
    step_type VARCHAR(100) NOT NULL, -- pre.validation, core.mapping, post.validation, custom
    sequence INTEGER NOT NULL, -- Execution order (10, 20, 30...)
    layer VARCHAR(20) DEFAULT 'core', -- pre, core, post
    required BOOLEAN DEFAULT true, -- If true, failure stops pipeline
    timeout_ms INTEGER DEFAULT 5000, -- Step timeout in milliseconds
    enabled BOOLEAN DEFAULT true,
    config JSONB DEFAULT '{}', -- Step-specific configuration
    script_type VARCHAR(20), -- javascript, lua (for custom steps)
    script_content TEXT, -- User-defined script code
    on_error_strategy VARCHAR(20) DEFAULT 'fail', -- fail, skip, default
    execution_mode VARCHAR(20) DEFAULT 'sequential', -- sequential, parallel
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_layer CHECK (layer IN ('pre', 'core', 'post')),
    CONSTRAINT chk_error_strategy CHECK (on_error_strategy IN ('fail', 'skip', 'default')),
    CONSTRAINT chk_execution_mode CHECK (execution_mode IN ('sequential', 'parallel'))
);

CREATE INDEX idx_transformation_steps_pipeline ON transformation_steps(pipeline_id);
CREATE INDEX idx_transformation_steps_sequence ON transformation_steps(pipeline_id, sequence);
CREATE INDEX idx_transformation_steps_layer ON transformation_steps(pipeline_id, layer);
CREATE INDEX idx_transformation_steps_type ON transformation_steps(step_type);

-- ============================================================================
-- 3. TRANSFORMATION EXECUTIONS TABLE (Audit Trail)
-- ============================================================================
CREATE TABLE IF NOT EXISTS transformation_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id VARCHAR(255) NOT NULL, -- Reference to message being transformed
    interface_id UUID NOT NULL REFERENCES interfaces(id),
    pipeline_id UUID NOT NULL REFERENCES transformation_pipelines(id),
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    total_time_ms BIGINT,
    status VARCHAR(20) DEFAULT 'running', -- running, completed, failed
    steps_executed INTEGER DEFAULT 0,
    steps_failed INTEGER DEFAULT 0,
    input_data JSONB, -- Snapshot of input (parsed HL7)
    output_data JSONB, -- Final output (FHIR bundle)
    error_message TEXT,
    execution_log JSONB, -- Array of step execution logs

    CONSTRAINT chk_execution_status CHECK (status IN ('running', 'completed', 'failed'))
);

CREATE INDEX idx_transformation_executions_message ON transformation_executions(message_id);
CREATE INDEX idx_transformation_executions_interface ON transformation_executions(interface_id);
CREATE INDEX idx_transformation_executions_pipeline ON transformation_executions(pipeline_id);
CREATE INDEX idx_transformation_executions_status ON transformation_executions(status);
CREATE INDEX idx_transformation_executions_started ON transformation_executions(started_at DESC);

-- ============================================================================
-- 4. STEP EXECUTIONS TABLE (Detailed Step Audit)
-- ============================================================================
CREATE TABLE IF NOT EXISTS step_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id UUID NOT NULL REFERENCES transformation_executions(id) ON DELETE CASCADE,
    step_id UUID NOT NULL REFERENCES transformation_steps(id),
    step_name VARCHAR(255) NOT NULL,
    step_type VARCHAR(100) NOT NULL,
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_ms BIGINT,
    status VARCHAR(20) DEFAULT 'running', -- running, completed, failed, skipped
    error_message TEXT,
    input_snapshot JSONB, -- Input data for this step
    output_snapshot JSONB, -- Output data from this step

    CONSTRAINT chk_step_execution_status CHECK (status IN ('running', 'completed', 'failed', 'skipped'))
);

CREATE INDEX idx_step_executions_execution ON step_executions(execution_id);
CREATE INDEX idx_step_executions_step ON step_executions(step_id);
CREATE INDEX idx_step_executions_status ON step_executions(status);

-- ============================================================================
-- 5. TRANSFORMATION TEMPLATES TABLE (Pre-built Step Templates)
-- ============================================================================
CREATE TABLE IF NOT EXISTS transformation_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_name VARCHAR(255) NOT NULL UNIQUE,
    template_type VARCHAR(100) NOT NULL, -- pre.validation, core.mapping, etc.
    description TEXT,
    layer VARCHAR(20) NOT NULL,
    default_config JSONB DEFAULT '{}',
    script_template TEXT, -- Template code for custom steps
    is_system BOOLEAN DEFAULT false, -- System templates can't be deleted
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_template_layer CHECK (layer IN ('pre', 'core', 'post'))
);

CREATE INDEX idx_transformation_templates_type ON transformation_templates(template_type);
CREATE INDEX idx_transformation_templates_layer ON transformation_templates(layer);

-- ============================================================================
-- 6. AUTO-UPDATE TRIGGERS
-- ============================================================================

-- Update updated_at timestamp on transformation_pipelines
CREATE OR REPLACE FUNCTION update_transformation_pipeline_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tr_update_transformation_pipeline_timestamp
BEFORE UPDATE ON transformation_pipelines
FOR EACH ROW
EXECUTE FUNCTION update_transformation_pipeline_timestamp();

-- Update updated_at timestamp on transformation_steps
CREATE OR REPLACE FUNCTION update_transformation_step_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tr_update_transformation_step_timestamp
BEFORE UPDATE ON transformation_steps
FOR EACH ROW
EXECUTE FUNCTION update_transformation_step_timestamp();

-- Auto-increment pipeline version on steps change
CREATE OR REPLACE FUNCTION increment_pipeline_version()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE transformation_pipelines
    SET version = version + 1, updated_at = CURRENT_TIMESTAMP
    WHERE id = COALESCE(NEW.pipeline_id, OLD.pipeline_id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tr_increment_pipeline_version_on_step_insert
AFTER INSERT ON transformation_steps
FOR EACH ROW
EXECUTE FUNCTION increment_pipeline_version();

CREATE TRIGGER tr_increment_pipeline_version_on_step_update
AFTER UPDATE ON transformation_steps
FOR EACH ROW
EXECUTE FUNCTION increment_pipeline_version();

CREATE TRIGGER tr_increment_pipeline_version_on_step_delete
AFTER DELETE ON transformation_steps
FOR EACH ROW
EXECUTE FUNCTION increment_pipeline_version();

-- ============================================================================
-- 7. SEED DATA - SYSTEM TEMPLATES
-- ============================================================================

-- Pre-processing template: Validate Required HL7 Fields
INSERT INTO transformation_templates (template_name, template_type, description, layer, default_config, is_system)
VALUES (
    'Validate Required HL7 Fields',
    'pre.validation',
    'Validates that required HL7 segments and fields are present',
    'pre',
    '{
        "rules": [
            {"segment": "MSH", "field": "MSH.9", "required": true, "description": "Message Type"},
            {"segment": "PID", "field": "PID.3", "required": true, "description": "Patient ID"}
        ]
    }'::jsonb,
    true
);

-- Core mapping template: HL7 to FHIR Bundle
INSERT INTO transformation_templates (template_name, template_type, description, layer, default_config, is_system)
VALUES (
    'HL7 to FHIR Bundle',
    'core.mapping',
    'Converts HL7 message to FHIR R4 Bundle using configured mappings',
    'core',
    '{
        "fhir_version": "R4",
        "bundle_type": "transaction",
        "use_template": true,
        "strict_mode": false
    }'::jsonb,
    true
);

-- Post-processing template: Validate FHIR Bundle
INSERT INTO transformation_templates (template_name, template_type, description, layer, default_config, is_system)
VALUES (
    'Validate FHIR Bundle',
    'post.validation',
    'Validates generated FHIR bundle against R4 specification',
    'post',
    '{
        "fhir_version": "R4",
        "strict_mode": false,
        "validate_references": true
    }'::jsonb,
    true
);

-- Custom script template
INSERT INTO transformation_templates (template_name, template_type, description, layer, script_template, is_system)
VALUES (
    'Custom JavaScript Transformation',
    'custom',
    'Execute custom JavaScript transformation logic',
    'pre',
    'function transform(input) {
    // Access parsed HL7 data
    // var segments = input.enhancedSegments;

    // Your custom logic here

    return input;
}',
    true
);

-- ============================================================================
-- 8. MIGRATION COMMENTS
-- ============================================================================

COMMENT ON TABLE transformation_pipelines IS 'Transformation pipelines for interface/message type combinations';
COMMENT ON TABLE transformation_steps IS 'Individual transformation steps within pipelines';
COMMENT ON TABLE transformation_executions IS 'Audit trail of pipeline executions';
COMMENT ON TABLE step_executions IS 'Detailed audit trail of individual step executions';
COMMENT ON TABLE transformation_templates IS 'Pre-built and user-defined transformation step templates';

COMMENT ON COLUMN transformation_steps.layer IS 'Execution layer: pre (preprocessing), core (main mapping), post (postprocessing)';
COMMENT ON COLUMN transformation_steps.sequence IS 'Execution order within layer (10, 20, 30...)';
COMMENT ON COLUMN transformation_steps.on_error_strategy IS 'fail: stop pipeline, skip: continue, default: use default value';
COMMENT ON COLUMN transformation_steps.execution_mode IS 'sequential: one at a time, parallel: concurrent execution';
