// controllers/pipelineController.js
// Pipeline builder controller (Node.js layer)
// MVC: This is a CONTROLLER - thin orchestration, delegates to Go backend

console.log('🔍 [pipelineController] Loading module...');

let axios;
try {
    axios = require('axios');
    console.log('✅ [pipelineController] axios loaded successfully');
} catch (error) {
    console.error('❌ [pipelineController] Failed to load axios:', error.message);
    throw error;
}

// const pipelineService = require('../services/PipelineService'); // Disabled - using inline fallback

// OOB: Auto-detect Go backend URL from environment
// Use 127.0.0.1 instead of localhost to force IPv4 (avoid IPv6 connection issues)
const GO_BACKEND_URL = process.env.GO_BACKEND_URL || `http://127.0.0.1:${process.env.API_PORT || 8080}`;
console.log('🔍 [pipelineController] GO_BACKEND_URL:', GO_BACKEND_URL);

// Fallback template library (used if database query fails)
const FALLBACK_TEMPLATES = [
    {
        id: 'validate-required-fields',
        name: 'Validate Required Fields',
        type: 'pre.validation',
        layer: 'pre',
        icon: 'fas fa-check-circle',
        isSystem: true,
        defaultConfig: { rules: [{ field: 'MSH.9', required: true }] }
    },
    {
        id: 'enrich-patient-data',
        name: 'Enrich Patient Data',
        type: 'pre.enrichment',
        layer: 'pre',
        icon: 'fas fa-plus-circle',
        isSystem: true,
        defaultConfig: { sources: ['EMPI'], timeout_ms: 3000 }
    },
    {
        id: 'hl7-fhir-mapping',
        name: 'HL7 to FHIR Mapping',
        type: 'core.mapping',
        layer: 'core',
        icon: 'fas fa-exchange-alt',
        isSystem: true,
        defaultConfig: { fhir_version: 'R4', use_template: true }
    },
    {
        id: 'validate-fhir',
        name: 'Validate FHIR Resource',
        type: 'post.validation',
        layer: 'post',
        icon: 'fas fa-shield-alt',
        isSystem: true,
        defaultConfig: {
            fhir_version: 'R4',
            validation_level: 'STANDARD',
            resource_rules: {
                validate_structure: true,
                validate_cardinality: true,
                validate_data_types: true,
                validate_references: true
            },
            required_resources: {
                Patient: { min: 1, max: 1 }
            },
            patient_validation: {
                require_identifier: true,
                require_name: true,
                validate_gender: true,
                validate_birthdate_format: true
            }
        }
    },
    {
        id: 'deliver-fhir',
        name: 'Deliver to FHIR Server',
        type: 'post.delivery',
        layer: 'post',
        icon: 'fas fa-paper-plane',
        isSystem: true,
        defaultConfig: { endpoint: 'http://fhir-server:8080/fhir', resource: 'Patient' }
    }
];

/**
 * Save pipeline (create or update)
 * POST /api/pipelines
 */
exports.savePipeline = async (req, res) => {
    try {
        const pipelineData = req.body;

        // Validate required fields
        const interfaceId = pipelineData.interface_id || pipelineData.interfaceId;
        const messageType = pipelineData.message_type || pipelineData.messageType;

        if (!interfaceId || !messageType) {
            return res.status(400).json({
                success: false,
                error: 'interface_id and message_type are required'
            });
        }

        // Save to PostgreSQL database
        const { sequelize } = require('../config/database');
        const { v4: uuidv4 } = require('uuid');
        const { QueryTypes } = require('sequelize');

        const pipelineId = pipelineData.id || uuidv4();
        const userId = req.session?.user?.id || req.user?.id;

        // Check if transformation_pipelines table exists
        const tableExists = await sequelize.query(`
            SELECT EXISTS (
                SELECT FROM information_schema.tables
                WHERE table_schema = 'public' AND table_name = 'transformation_pipelines'
            );
        `, { type: QueryTypes.SELECT });

        if (!tableExists[0].exists) {
            return res.status(503).json({
                success: false,
                error: 'Pipeline storage not implemented. transformation_pipelines table does not exist.'
            });
        }

        // Upsert pipeline
        const result = await sequelize.query(`
            INSERT INTO transformation_pipelines
                (id, interface_id, message_type, pipeline_name, pipeline_config, status, created_by, created_at, updated_at)
            VALUES
                ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
            ON CONFLICT (interface_id, message_type)
            DO UPDATE SET
                pipeline_name = EXCLUDED.pipeline_name,
                pipeline_config = EXCLUDED.pipeline_config,
                status = EXCLUDED.status,
                updated_at = NOW()
            RETURNING id::text, interface_id::text, message_type, pipeline_name, status;
        `, {
            bind: [pipelineId, interfaceId, messageType, pipelineData.name || `${messageType} Pipeline`,
                   JSON.stringify(pipelineData), pipelineData.status || 'draft', userId],
            type: QueryTypes.SELECT
        });

        console.log(`✅ Pipeline saved: ${pipelineId} for ${interfaceId}/${messageType}`);

        res.json({
            success: true,
            pipeline: result[0],
            message: 'Pipeline saved successfully'
        });

    } catch (error) {
        console.error('❌ Save pipeline error:', error.message);
        console.error('Stack:', error.stack);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

/**
 * Load pipeline by ID
 * GET /api/pipelines/:id
 */
exports.loadPipeline = async (req, res) => {
    try {
        const { id } = req.params;

        // Forward to Go backend
        const response = await axios.get(
            `${GO_BACKEND_URL}/api/pipelines/${id}`,
            { timeout: 10000 }
        );

        res.json(response.data);

    } catch (error) {
        console.error('❌ Load pipeline error:', error.message);
        res.status(error.response?.status || 500).json({
            success: false,
            error: error.response?.data?.error || error.message
        });
    }
};

/**
 * Load pipeline by interface and message type
 * GET /api/pipelines/interface/:interfaceId/:messageType
 *
 * SPECIAL LOGIC: If no pipeline exists, auto-generates one from wizard mappings
 */
exports.loadPipelineByInterface = async (req, res) => {
    try {
        const { interfaceId, messageType } = req.params;

        // Load pipeline and steps from PostgreSQL database
        const { sequelize } = require('../config/database');
        const { QueryTypes } = require('sequelize');

        const pipelineData = await sequelize.query(`
            SELECT
                tp.id::text as pipeline_id,
                tp.pipeline_name,
                tp.interface_id::text,
                tp.message_type,
                tp.enabled,
                tp.connections,
                i.name as interface_name,
                ts.id::text as step_id,
                ts.step_name,
                ts.step_type,
                ts.sequence,
                ts.layer,
                ts.required,
                ts.enabled as step_enabled,
                ts.config,
                ts.timeout_ms,
                ts.on_error_strategy,
                ts.position_x,
                ts.position_y,
                ts.parent_conditional_step_id::text,
                ts.branch_type,
                ts.case_value
            FROM transformation_pipelines tp
            LEFT JOIN interfaces i ON i.id = tp.interface_id
            LEFT JOIN transformation_steps ts ON ts.pipeline_id = tp.id
            WHERE tp.interface_id = $1 AND tp.message_type = $2
            ORDER BY ts.sequence;
        `, {
            bind: [interfaceId, messageType],
            type: QueryTypes.SELECT
        });

        if (pipelineData && pipelineData.length > 0) {
            console.log(`✅ Loaded pipeline with ${pipelineData.filter(r => r.step_id).length} steps from database`);

            // Collect all steps into a single flat list
            const allSteps = pipelineData
                .filter(row => row.step_id)
                .map(row => ({
                    id: row.step_id,
                    step_name: row.step_name,
                    step_type: row.step_type,
                    sequence: row.sequence,
                    required: row.required,
                    enabled: row.step_enabled,
                    config: row.config,
                    timeout_ms: row.timeout_ms,
                    on_error_strategy: row.on_error_strategy,
                    layer: row.layer || 'core',
                    position_x: row.position_x,
                    position_y: row.position_y,
                    parent_conditional_step_id: row.parent_conditional_step_id,
                    branch_type: row.branch_type,
                    case_value: row.case_value
                }));

            // New format: flat execution_groups array (single group with all steps)
            const pipeline = {
                id: pipelineData[0].pipeline_id,
                interface_id: pipelineData[0].interface_id,
                message_type: pipelineData[0].message_type,
                name: pipelineData[0].interface_name || pipelineData[0].pipeline_name,
                enabled: pipelineData[0].enabled,
                connections: pipelineData[0].connections || [],
                // New flat format (snake_case for consistency with VisualExecutionGroup.fromJSON)
                execution_groups: allSteps.length > 0 ? [{
                    id: `group_${Date.now()}`,
                    group_type: 'inline',
                    layer: 'core',
                    sequence: 100,
                    steps: allSteps
                }] : [],
                // Legacy format for backward compat (also use snake_case)
                layers: {
                    pre: { execution_groups: [] },
                    core: {
                        execution_groups: allSteps.length > 0 ? [{
                            id: `core_group_${Date.now()}`,
                            group_type: 'inline',
                            layer: 'core',
                            sequence: 100,
                            steps: allSteps
                        }] : []
                    },
                    post: { execution_groups: [] }
                }
            };

            console.log(`🔗 Loaded ${(pipeline.connections || []).length} connections from database`);
            console.log(`📤 Returning pipeline with ${allSteps.length} steps`);

            return res.json({
                success: true,
                pipeline: pipeline,
                source: 'database'
            });
        }

        console.log('📋 No saved pipeline found, checking for wizard mappings...');

        // Try loading wizard mappings to auto-generate pipeline
        const wizardMappings = await loadWizardMappings(interfaceId, messageType);

        if (wizardMappings) {
            console.log('✅ Wizard mappings found, generating initial pipeline');
            const initialPipeline = generatePipelineFromWizard(interfaceId, messageType, wizardMappings);
            console.log('📤 Returning pipeline:', JSON.stringify(initialPipeline, null, 2));
            return res.json({
                success: true,
                pipeline: initialPipeline,
                generated: true,
                message: 'Pipeline auto-generated from wizard mappings'
            });
        }

        // No pipeline and no wizard mappings - return empty pipeline
        console.log('📝 No mappings found, returning empty pipeline template');
        const emptyPipeline = generateEmptyPipeline(interfaceId, messageType);
        return res.json({
            success: true,
            pipeline: emptyPipeline,
            generated: true,
            message: 'Empty pipeline template created'
        });

    } catch (error) {
        console.error('❌ Load pipeline by interface error:', error.message);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

/**
 * List all pipelines for an interface
 * GET /api/pipelines/interface/:interfaceId
 */
exports.listPipelines = async (req, res) => {
    try {
        const { interfaceId } = req.params;

        // Forward to Go backend
        const response = await axios.get(
            `${GO_BACKEND_URL}/api/pipelines/interface/${interfaceId}`,
            { timeout: 10000 }
        );

        res.json(response.data);

    } catch (error) {
        console.error('❌ List pipelines error:', error.message);
        res.status(error.response?.status || 500).json({
            success: false,
            error: error.response?.data?.error || error.message
        });
    }
};

/**
 * Delete pipeline
 * DELETE /api/pipelines/:id
 */
exports.deletePipeline = async (req, res) => {
    try {
        const { id } = req.params;

        // Forward to Go backend
        const response = await axios.delete(
            `${GO_BACKEND_URL}/api/pipelines/${id}`,
            { timeout: 10000 }
        );

        res.json(response.data);

    } catch (error) {
        console.error('❌ Delete pipeline error:', error.message);
        res.status(error.response?.status || 500).json({
            success: false,
            error: error.response?.data?.error || error.message
        });
    }
};

/**
 * Clone pipeline
 * POST /api/pipelines/:id/clone
 */
exports.clonePipeline = async (req, res) => {
    try {
        const { id } = req.params;
        const { new_name } = req.body;

        if (!new_name) {
            return res.status(400).json({
                success: false,
                error: 'new_name is required'
            });
        }

        // Forward to Go backend
        const response = await axios.post(
            `${GO_BACKEND_URL}/api/pipelines/${id}/clone`,
            { new_name },
            { timeout: 30000 }
        );

        res.json(response.data);

    } catch (error) {
        console.error('❌ Clone pipeline error:', error.message);
        res.status(error.response?.status || 500).json({
            success: false,
            error: error.response?.data?.error || error.message
        });
    }
};

/**
 * Test pipeline with sample data
 * POST /api/pipelines/test
 */
exports.testPipeline = async (req, res) => {
    try {
        const { pipeline_id, pipeline, test_message, sample_message, input_data } = req.body;

        // Accept either pipeline_id (saved pipeline) or pipeline object (unsaved)
        if (!pipeline_id && !pipeline) {
            return res.status(400).json({
                success: false,
                error: 'Either pipeline_id or pipeline object is required'
            });
        }

        // Normalize message field names
        const testMessage = test_message || sample_message;

        if (!testMessage) {
            return res.status(400).json({
                success: false,
                error: 'test_message or sample_message is required'
            });
        }

        console.log('🧪 Forwarding test request to Go backend...');
        console.log('   Pipeline ID:', pipeline_id || 'N/A (using object)');
        console.log('   Message length:', testMessage?.length || 0);

        // Forward to Go backend (correct path)
        const response = await axios.post(
            `${GO_BACKEND_URL}/api/fhir/pipeline/test`,
            {
                pipeline_id,
                pipeline,
                test_message: testMessage,
                input_data
            },
            { timeout: 60000 } // 60 second timeout for tests
        );

        console.log('✅ Go backend response status:', response.status);
        console.log('✅ Go backend response success:', response.data?.success);
        console.log('✅ Go backend response keys:', Object.keys(response.data || {}));

        res.json(response.data);

    } catch (error) {
        console.error('❌ Test pipeline error:', error.message);
        console.error('❌ Error response status:', error.response?.status);
        console.error('❌ Error response data:', JSON.stringify(error.response?.data, null, 2));
        res.status(error.response?.status || 500).json({
            success: false,
            error: error.response?.data?.error || error.message,
            result: error.response?.data?.result // Include partial result
        });
    }
};

/**
 * Execute pipeline (production)
 * POST /api/pipelines/execute
 */
exports.executePipeline = async (req, res) => {
    try {
        const { pipeline_id, message_id, input_data } = req.body;

        if (!pipeline_id || !message_id) {
            return res.status(400).json({
                success: false,
                error: 'pipeline_id and message_id are required'
            });
        }

        // Forward to Go backend
        const response = await axios.post(
            `${GO_BACKEND_URL}/api/pipelines/execute`,
            {
                pipeline_id,
                message_id,
                input_data
            },
            { timeout: 120000 } // 2 minute timeout
        );

        res.json(response.data);

    } catch (error) {
        console.error('❌ Execute pipeline error:', error.message);
        res.status(error.response?.status || 500).json({
            success: false,
            error: error.response?.data?.error || error.message,
            result: error.response?.data?.result
        });
    }
};

/**
 * Get pipeline execution statistics
 * GET /api/pipelines/:id/stats
 */
exports.getPipelineStats = async (req, res) => {
    try {
        const { id } = req.params;

        // Forward to Go backend
        const response = await axios.get(
            `${GO_BACKEND_URL}/api/pipelines/${id}/stats`,
            { timeout: 10000 }
        );

        res.json(response.data);

    } catch (error) {
        console.error('❌ Get pipeline stats error:', error.message);
        res.status(error.response?.status || 500).json({
            success: false,
            error: error.response?.data?.error || error.message
        });
    }
};

/**
 * List all templates
 * GET /api/templates
 */
exports.listTemplates = async (req, res) => {
    try {
        // Load templates from database using Sequelize
        const { sequelize } = require('../config/database');
        const { QueryTypes } = require('sequelize');

        const query = `
            SELECT
                id::text,
                template_name as name,
                template_type as type,
                description,
                layer,
                default_config as "defaultConfig",
                is_system as "isSystem"
            FROM transformation_templates
            WHERE is_system = true
            ORDER BY
                CASE layer
                    WHEN 'pre' THEN 1
                    WHEN 'core' THEN 2
                    WHEN 'post' THEN 3
                END,
                template_name;
        `;

        const templates = await sequelize.query(query, {
            type: QueryTypes.SELECT
        });

        const enrichedTemplates = templates.map(row => ({
            ...row,
            icon: getIconForTemplateType(row.type)
        }));

        console.log(`✅ Loaded ${enrichedTemplates.length} templates from database`);

        res.json({
            success: true,
            templates: enrichedTemplates.length > 0 ? enrichedTemplates : FALLBACK_TEMPLATES
        });

    } catch (error) {
        console.error('❌ List templates error:', error.message);
        console.error('Stack:', error.stack);
        // Fallback to hardcoded templates on error
        res.json({
            success: true,
            templates: FALLBACK_TEMPLATES
        });
    }
};

// Helper function to assign icons based on template type
function getIconForTemplateType(type) {
    const iconMap = {
        'pre.validation': 'fas fa-check-circle',
        'pre.enrichment': 'fas fa-plus-circle',
        'core.mapping': 'fas fa-exchange-alt',
        'post.validation': 'fas fa-shield-alt',
        'post.delivery': 'fas fa-paper-plane'
    };
    return iconMap[type] || 'fas fa-cog';
}

/**
 * Get template by ID
 * GET /api/templates/:id
 */
exports.getTemplate = async (req, res) => {
    try {
        const { id } = req.params;

        const template = FALLBACK_TEMPLATES.find(t => t.id === id);

        if (!template) {
            return res.status(404).json({
                success: false,
                error: 'Template not found'
            });
        }

        res.json({
            success: true,
            template
        });

    } catch (error) {
        console.error('❌ Get template error:', error.message);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

/**
 * Create custom template
 * POST /api/templates
 */
exports.createTemplate = async (req, res) => {
    try {
        const templateData = req.body;

        // For now, just return the template data
        // TODO: Implement template persistence
        res.json({
            success: true,
            template: templateData
        });

    } catch (error) {
        console.error('❌ Create template error:', error.message);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
};

/**
 * Execute single validation step
 * POST /api/processing/execute/validation
 */
exports.executeValidationStep = async (req, res) => {
    try {
        const { step, inputData } = req.body;

        // Forward to Go backend
        const response = await axios.post(
            `${GO_BACKEND_URL}/api/processing/execute/validation`,
            { step, inputData },
            { timeout: 10000 }
        );

        res.json(response.data);

    } catch (error) {
        console.error('❌ Execute validation step error:', error.message);
        res.status(200).json({ // Return 200 for easier frontend handling
            success: false,
            error: error.response?.data?.error || error.message,
            data: error.response?.data?.data || req.body.inputData
        });
    }
};

/**
 * Execute single enrichment step
 * POST /api/processing/execute/enrichment
 */
exports.executeEnrichmentStep = async (req, res) => {
    try {
        const { step, inputData } = req.body;

        const response = await axios.post(
            `${GO_BACKEND_URL}/api/processing/execute/enrichment`,
            { step, inputData },
            { timeout: 30000 }
        );

        res.json(response.data);

    } catch (error) {
        console.error('❌ Execute enrichment step error:', error.message);
        res.status(200).json({
            success: false,
            error: error.response?.data?.error || error.message,
            data: error.response?.data?.data || req.body.inputData
        });
    }
};

/**
 * Execute single mapping step
 * POST /api/processing/execute/mapping
 */
exports.executeMappingStep = async (req, res) => {
    try {
        const { step, inputData } = req.body;

        const response = await axios.post(
            `${GO_BACKEND_URL}/api/processing/execute/mapping`,
            { step, inputData },
            { timeout: 30000 }
        );

        res.json(response.data);

    } catch (error) {
        console.error('❌ Execute mapping step error:', error.message);
        res.status(200).json({
            success: false,
            error: error.response?.data?.error || error.message,
            data: error.response?.data?.data || req.body.inputData
        });
    }
};

/**
 * Execute custom JavaScript step
 * POST /api/processing/execute/custom
 */
exports.executeCustomStep = async (req, res) => {
    try {
        const { step, inputData } = req.body;

        const response = await axios.post(
            `${GO_BACKEND_URL}/api/processing/execute/custom`,
            { step, inputData },
            { timeout: 10000 }
        );

        res.json(response.data);

    } catch (error) {
        console.error('❌ Execute custom step error:', error.message);
        res.status(200).json({
            success: false,
            error: error.response?.data?.error || error.message,
            data: error.response?.data?.data || req.body.inputData
        });
    }
};

// ============================================================================
// HELPER FUNCTIONS - Wizard Integration
// ============================================================================

/**
 * Load wizard mappings from database for an interface + message type
 * UPDATED: Read directly from interfaces table transformation_mapping column
 */
async function loadWizardMappings(interfaceId, messageType) {
    try {
        const { sequelize } = require('../config/database');
        const { QueryTypes } = require('sequelize');

        console.log(`🔍 Loading wizard mappings for interface ${interfaceId}, message type ${messageType}`);

        // Query transformation_mapping JSONB from interfaces table
        const result = await sequelize.query(`
            SELECT
                transformation_mapping,
                message_type,
                name
            FROM interfaces
            WHERE id = $1 AND message_type = $2
        `, {
            bind: [interfaceId, messageType],
            type: QueryTypes.SELECT
        });

        if (result && result.length > 0 && result[0].transformation_mapping) {
            const mappingData = result[0].transformation_mapping;

            // Handle both formats:
            // 1. Direct array: transformation_mapping = [{...}, {...}]
            // 2. Nested object: transformation_mapping = { atomicMappings: [...], enhancedSegments: {...} }
            const mappings = Array.isArray(mappingData)
                ? mappingData
                : (mappingData.atomicMappings || []);

            console.log(`✅ Found wizard mappings: ${mappings.length} mappings`);

            return {
                mappings: mappings,
                enhancedSegments: mappingData.enhancedSegments || {},
                messageType: mappingData.messageType || messageType,
                fhirTransformResult: mappingData.fhirTransformResult,
                sourceType: mappingData.sourceType,
                targetType: mappingData.targetType
            };
        }

        console.log('⚠️ No wizard mappings found in interfaces table');
        return null;
    } catch (error) {
        console.log('⚠️ Error loading wizard mappings:', error.message);
        return null;
    }
}

/**
 * Generate initial pipeline from wizard mappings
 */
function generatePipelineFromWizard(interfaceId, messageType, wizardMappings) {
    const { v4: uuidv4 } = require('uuid');

    const groupId = uuidv4();
    const stepId = uuidv4();
    const step = {
        id: stepId,
        step_name: 'HL7 to FHIR Mapping',
        step_type: 'hl7_fhir_transform',
        template_id: 'hl7-fhir-mapping',
        layer: 'core',
        sequence: 100,
        config: {
            mappings: wizardMappings.mappings || wizardMappings.custom_mapping_config || [],
            template_id: wizardMappings.standard_template_id,
            uses_standard_template: wizardMappings.uses_standard_template,
            source: 'wizard',
            description: 'Auto-configured from wizard'
        }
    };

    const group = {
        id: groupId,
        group_type: 'inline',
        layer: 'core',
        sequence: 100,
        steps: [step]
    };

    return {
        id: uuidv4(),
        interface_id: interfaceId,
        message_type: messageType,
        name: `${messageType} Pipeline`,
        description: `Auto-generated from wizard mappings`,
        // New flat format (takes priority in fromJSON)
        execution_groups: [group],
        // Legacy format for backward compat
        layers: {
            pre: { name: 'Pre-Processing', execution_groups: [] },
            core: { name: 'Core Transformation', execution_groups: [group] },
            post: { name: 'Post-Processing', execution_groups: [] }
        },
        metadata: {
            created_from: 'wizard',
            created_at: new Date().toISOString(),
            version: '1.0'
        }
    };
}

/**
 * Generate empty pipeline template
 */
function generateEmptyPipeline(interfaceId, messageType) {
    const { v4: uuidv4 } = require('uuid');

    return {
        id: uuidv4(),
        interface_id: interfaceId,
        message_type: messageType,
        name: `${messageType} Pipeline`,
        description: 'New pipeline - add steps from the toolbox',
        // New flat format (empty)
        execution_groups: [],
        // Legacy format for backward compat (also empty)
        layers: {
            pre: { name: 'Pre-Processing', execution_groups: [] },
            core: { name: 'Core Transformation', execution_groups: [] },
            post: { name: 'Post-Processing', execution_groups: [] }
        },
        metadata: {
            created_from: 'scratch',
            created_at: new Date().toISOString(),
            version: '1.0'
        }
    };
}

// Debug: Log all exports
console.log('🔍 [pipelineController] Exporting functions...');
const exportedFunctions = Object.keys(exports);
console.log(`✅ [pipelineController] Exported ${exportedFunctions.length} functions:`, exportedFunctions.join(', '));
console.log('✅ [pipelineController] Module loaded successfully');
