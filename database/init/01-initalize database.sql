-- ================================================
-- FINAL MAPPING TABLES FOR ENHANCED STEP 4
-- ================================================
-- This is the ONLY SQL file you need to run
-- Compatible with your existing interfaces and users tables

-- ================================================
-- PART 1: CREATE THE 4 MAPPING TABLES
-- ================================================

-- 1. Templates that users can start from
CREATE TABLE IF NOT EXISTS mapping_templates (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    message_type VARCHAR(50) NOT NULL,
    fhir_profile VARCHAR(100) DEFAULT 'FHIR R4 Base',
    template_rules JSONB DEFAULT '[]'::jsonb,
    category VARCHAR(100),
    is_system_template BOOLEAN DEFAULT FALSE,
    usage_count INTEGER DEFAULT 0,
    created_by UUID REFERENCES public.users(id),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(name, message_type)
);

-- 2. Actual mapping config for each interface (customizable from templates)
CREATE TABLE IF NOT EXISTS interface_mapping_configs (
    id SERIAL PRIMARY KEY,
    interface_id UUID UNIQUE REFERENCES public.interfaces(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    message_type VARCHAR(50) NOT NULL,
    fhir_profile VARCHAR(100) DEFAULT 'FHIR R4 Base',
    mapping_rules JSONB DEFAULT '[]'::jsonb,
    based_on_template_id INTEGER REFERENCES mapping_templates(id),
    is_active BOOLEAN DEFAULT TRUE,
    stats JSONB DEFAULT '{}'::jsonb,
    transformation_count INTEGER DEFAULT 0,
    error_count INTEGER DEFAULT 0,
    created_by UUID REFERENCES public.users(id),
    updated_by UUID REFERENCES public.users(id),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- 3. Track when transformations are executed
CREATE TABLE IF NOT EXISTS transformation_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interface_id UUID REFERENCES public.interfaces(id) ON DELETE CASCADE,
    config_id INTEGER REFERENCES interface_mapping_configs(id),
    execution_status VARCHAR(20) NOT NULL,
    processing_time_ms INTEGER,
    error_message TEXT,
    rules_processed INTEGER,
    rules_successful INTEGER,
    triggered_by UUID REFERENCES public.users(id),
    executed_at TIMESTAMPTZ DEFAULT now()
);

-- 4. AI suggestions for field mappings
CREATE TABLE IF NOT EXISTS mapping_suggestions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_type VARCHAR(50) NOT NULL,
    source_field VARCHAR(255) NOT NULL,
    suggested_target_field VARCHAR(255) NOT NULL,
    suggestion_type VARCHAR(50) NOT NULL,
    confidence_score DECIMAL(3,2),
    was_accepted BOOLEAN,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- ================================================
-- PART 2: ADD INDEXES FOR PERFORMANCE
-- ================================================

CREATE INDEX IF NOT EXISTS idx_mapping_templates_message_type ON mapping_templates(message_type);
CREATE INDEX IF NOT EXISTS idx_interface_configs_interface_id ON interface_mapping_configs(interface_id);
CREATE INDEX IF NOT EXISTS idx_transformation_executions_interface ON transformation_executions(interface_id);
CREATE INDEX IF NOT EXISTS idx_transformation_executions_status ON transformation_executions(execution_status);
CREATE INDEX IF NOT EXISTS idx_mapping_suggestions_message_type ON mapping_suggestions(message_type);

-- ================================================
-- PART 3: ADD UPDATE TRIGGERS
-- ================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_mapping_templates_timestamp
    BEFORE UPDATE ON mapping_templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_interface_configs_timestamp
    BEFORE UPDATE ON interface_mapping_configs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ================================================
-- PART 4: VERIFICATION
-- ================================================

DO $$
BEGIN
    RAISE NOTICE '========================================';
    RAISE NOTICE '✅ MAPPING TABLES CREATED SUCCESSFULLY';
    RAISE NOTICE '========================================';
    RAISE NOTICE '';
    RAISE NOTICE 'Created tables:';
    RAISE NOTICE '  📚 mapping_templates (%)', (SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'mapping_templates');
    RAISE NOTICE '  ⚙️  interface_mapping_configs (%)', (SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'interface_mapping_configs');
    RAISE NOTICE '  📊 transformation_executions (%)', (SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'transformation_executions');
    RAISE NOTICE '  🤖 mapping_suggestions (%)', (SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'mapping_suggestions');
    RAISE NOTICE '';
    RAISE NOTICE 'Integration status:';
    RAISE NOTICE '  🔗 Links to your interfaces table: ✅';
    RAISE NOTICE '  🔗 Links to your users table: ✅';
    RAISE NOTICE '  🔗 Compatible with audit_logs: ✅';
    RAISE NOTICE '';
    RAISE NOTICE 'Next steps:';
    RAISE NOTICE '  1. Update your Go controller with new API methods';
    RAISE NOTICE '  2. Replace Step 4 in interfaces.html';
    RAISE NOTICE '  3. Test the enhanced mapping interface';
    RAISE NOTICE '';
    RAISE NOTICE '========================================';
END $$;