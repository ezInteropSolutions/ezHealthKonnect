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

        // First, try loading from PostgreSQL database
        const { sequelize } = require('../config/database');
        const { QueryTypes } = require('sequelize');

        const savedPipeline = await sequelize.query(`
            SELECT pipeline_config
            FROM transformation_pipelines
            WHERE interface_id = $1 AND message_type = $2
            LIMIT 1;
        `, {
            bind: [interfaceId, messageType],
            type: QueryTypes.SELECT
        });

        if (savedPipeline && savedPipeline.length > 0) {
            console.log('✅ Loaded saved pipeline from database');
            return res.json({
                success: true,
                pipeline: savedPipeline[0].pipeline_config,
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
 */
async function loadWizardMappings(interfaceId, messageType) {
    try {
        // Call wizard API endpoint to get runtime mappings
        const response = await axios.get(
            `http://localhost:${process.env.PORT || 3000}/api/wizard/runtime-mapping/${interfaceId}/${encodeURIComponent(messageType)}`,
            { timeout: 5000 }
        );

        if (response.data && response.data.success) {
            // Handle both response formats: data.mapping or data.data.config
            const mappingData = response.data.mapping || response.data.data;

            if (mappingData) {
                // If config is a JSON string, parse it
                if (mappingData.config && typeof mappingData.config === 'string') {
                    mappingData.mappings = JSON.parse(mappingData.config);
                }
                return mappingData;
            }
        }
        return null;
    } catch (error) {
        console.log('⚠️ No wizard mappings found:', error.message);
        return null;
    }
}

/**
 * Generate initial pipeline from wizard mappings
 */
function generatePipelineFromWizard(interfaceId, messageType, wizardMappings) {
    const { v4: uuidv4 } = require('uuid');

    return {
        id: uuidv4(),
        interface_id: interfaceId,  // snake_case for frontend
        message_type: messageType,  // snake_case for frontend
        name: `${messageType} Pipeline`,
        description: `Auto-generated from wizard mappings`,
        layers: {
            pre: {
                name: 'Pre-Processing',
                execution_groups: []  // snake_case for frontend
            },
            core: {
                name: 'Core Transformation',
                execution_groups: [  // snake_case for frontend
                    {
                        id: uuidv4(),
                        name: 'HL7 to FHIR Mapping',
                        mode: 'sequential',
                        steps: [
                            {
                                id: uuidv4(),
                                step_name: 'Apply Wizard Mappings',  // snake_case for frontend
                                step_type: 'core.mapping',  // snake_case for frontend
                                template_id: 'hl7-fhir-mapping',  // snake_case for frontend
                                config: {
                                    mappings: wizardMappings.mappings || wizardMappings.custom_mapping_config || [],
                                    template_id: wizardMappings.standard_template_id,
                                    uses_standard_template: wizardMappings.uses_standard_template,
                                    source: 'wizard',
                                    description: 'Mappings configured via wizard'
                                }
                            }
                        ]
                    }
                ]
            },
            post: {
                name: 'Post-Processing',
                execution_groups: []  // snake_case for frontend
            }
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
        interface_id: interfaceId,  // snake_case for frontend
        message_type: messageType,  // snake_case for frontend
        name: `${messageType} Pipeline`,
        description: 'New pipeline - add steps from the toolbox',
        layers: {
            pre: {
                name: 'Pre-Processing',
                execution_groups: []  // snake_case for frontend
            },
            core: {
                name: 'Core Transformation',
                execution_groups: []  // snake_case for frontend
            },
            post: {
                name: 'Post-Processing',
                execution_groups: []  // snake_case for frontend
            }
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
