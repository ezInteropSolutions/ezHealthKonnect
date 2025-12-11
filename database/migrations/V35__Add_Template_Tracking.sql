-- V21: Add Template Tracking to Transformation Steps
-- Enables shared templates with individual step customization

-- ============================================================================
-- 1. ADD TEMPLATE TRACKING TO TRANSFORMATION_STEPS
-- ============================================================================

-- Add template_id to track which template a step was created from
ALTER TABLE transformation_steps
ADD COLUMN IF NOT EXISTS template_id UUID REFERENCES transformation_templates(id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS is_customized BOOLEAN DEFAULT false;

-- Index for finding all steps using a template
CREATE INDEX IF NOT EXISTS idx_transformation_steps_template ON transformation_steps(template_id);

-- ============================================================================
-- 2. ENHANCE TRANSFORMATION_TEMPLATES
-- ============================================================================

-- Add user tracking and visibility controls
ALTER TABLE transformation_templates
ADD COLUMN IF NOT EXISTS created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS is_public BOOLEAN DEFAULT true,
ADD COLUMN IF NOT EXISTS usage_count INTEGER DEFAULT 0;

-- Indexes for template queries
CREATE INDEX IF NOT EXISTS idx_transformation_templates_creator ON transformation_templates(created_by_user_id);
CREATE INDEX IF NOT EXISTS idx_transformation_templates_public ON transformation_templates(is_public) WHERE is_public = true;
CREATE INDEX IF NOT EXISTS idx_transformation_templates_usage ON transformation_templates(usage_count DESC);

-- ============================================================================
-- 3. HELPER FUNCTION: Increment Template Usage
-- ============================================================================

CREATE OR REPLACE FUNCTION increment_template_usage()
RETURNS TRIGGER AS $$
BEGIN
    -- Increment usage count when a step is created from a template
    IF NEW.template_id IS NOT NULL THEN
        UPDATE transformation_templates
        SET usage_count = usage_count + 1
        WHERE id = NEW.template_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-increment usage count
DROP TRIGGER IF EXISTS tr_increment_template_usage ON transformation_steps;
CREATE TRIGGER tr_increment_template_usage
AFTER INSERT ON transformation_steps
FOR EACH ROW
WHEN (NEW.template_id IS NOT NULL)
EXECUTE FUNCTION increment_template_usage();

-- ============================================================================
-- 4. HELPER FUNCTION: Mark Step as Customized
-- ============================================================================

CREATE OR REPLACE FUNCTION mark_step_customized()
RETURNS TRIGGER AS $$
BEGIN
    -- If step has template_id and config changes, mark as customized
    IF NEW.template_id IS NOT NULL AND OLD.config IS DISTINCT FROM NEW.config THEN
        NEW.is_customized = true;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-mark steps as customized when config changes
DROP TRIGGER IF EXISTS tr_mark_step_customized ON transformation_steps;
CREATE TRIGGER tr_mark_step_customized
BEFORE UPDATE OF config ON transformation_steps
FOR EACH ROW
EXECUTE FUNCTION mark_step_customized();

-- ============================================================================
-- 5. COMMENTS
-- ============================================================================

COMMENT ON COLUMN transformation_steps.template_id IS 'Reference to template this step was created from (NULL if custom step)';
COMMENT ON COLUMN transformation_steps.is_customized IS 'True if step config differs from template default_config';
COMMENT ON COLUMN transformation_templates.created_by_user_id IS 'User who created this template (NULL for system templates)';
COMMENT ON COLUMN transformation_templates.is_public IS 'True if template is visible to all users';
COMMENT ON COLUMN transformation_templates.usage_count IS 'Number of times this template has been used';
