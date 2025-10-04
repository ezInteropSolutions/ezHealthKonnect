// routes/wizardRoutes.js
// Clean routes file - only handles routing, no business logic

const express = require('express');
const router = express.Router();
const wizardController = require('../controllers/wizardController');
const WizardMappingController = require('../controllers/WizardMappingController'); // Fixed path

const wizardMappingController = new WizardMappingController();

/**
 * Authentication middleware
 */
const requireAuth = (req, res, next) => {
    if (!req.session?.user) {
        return res.status(401).json({
            success: false,
            error: 'Authentication required'
        });
    }
    
    // Set user info for controller access
    req.user = {
        id: req.session.user.id,
        email: req.session.user.email,
        name: req.session.user.name,
        role: req.session.user.role
    };
    
    next();
};

// ====================================
// EXISTING WIZARD ROUTES
// ====================================

// Interface configuration routes
router.post('/save-config', requireAuth, wizardController.saveConfiguration);
router.post('/activate-interface', requireAuth, wizardController.activateInterface);
router.post('/complete', requireAuth, wizardController.completeWizard);

// Interface management routes
router.get('/interfaces', requireAuth, wizardController.listInterfaces);
router.get('/interfaces/:id', requireAuth, wizardController.getInterface);
router.get('/interfaces/:id/stats', requireAuth, wizardController.getInterfaceStats);
router.delete('/interfaces/:id', requireAuth, wizardController.deleteInterface);

// ====================================
// PROCESSING ENGINE ROUTES
// ====================================

/**
 * Start processing engine
 * POST /api/wizard/engine/start
 */
router.post('/engine/start', requireAuth, async (req, res) => {
    try {
        const ProcessingEngineService = require('../services/ProcessingEngineService');
        const processingEngine = new ProcessingEngineService();

        await processingEngine.start();

        res.json({
            success: true,
            message: 'Processing engine started successfully',
            stats: processingEngine.getProcessingStats()
        });
    } catch (error) {
        console.error('Failed to start processing engine:', error);
        res.status(500).json({
            success: false,
            error: 'Failed to start processing engine: ' + error.message
        });
    }
});

/**
 * Activate specific interface for processing
 * POST /api/wizard/interfaces/:id/activate
 */
router.post('/interfaces/:id/activate', requireAuth, async (req, res) => {
    try {
        const { id: interfaceId } = req.params;

        const ProcessingEngineService = require('../services/ProcessingEngineService');
        const processingEngine = new ProcessingEngineService();

        await processingEngine.activateInterface(interfaceId);

        res.json({
            success: true,
            message: `Interface ${interfaceId} activated for processing`,
            interfaceId
        });
    } catch (error) {
        console.error(`Failed to activate interface ${req.params.id}:`, error);
        res.status(500).json({
            success: false,
            error: 'Failed to activate interface: ' + error.message
        });
    }
});

/**
 * Deactivate specific interface
 * POST /api/wizard/interfaces/:id/deactivate
 */
router.post('/interfaces/:id/deactivate', requireAuth, async (req, res) => {
    try {
        const { id: interfaceId } = req.params;
        const { reason } = req.body;

        const ProcessingEngineService = require('../services/ProcessingEngineService');
        const processingEngine = new ProcessingEngineService();

        await processingEngine.deactivateInterface(interfaceId, reason || 'manual');

        res.json({
            success: true,
            message: `Interface ${interfaceId} deactivated`,
            interfaceId
        });
    } catch (error) {
        console.error(`Failed to deactivate interface ${req.params.id}:`, error);
        res.status(500).json({
            success: false,
            error: 'Failed to deactivate interface: ' + error.message
        });
    }
});

/**
 * Get processing engine status and statistics
 * GET /api/wizard/engine/status
 */
router.get('/engine/status', requireAuth, async (req, res) => {
    try {
        const ProcessingEngineService = require('../services/ProcessingEngineService');
        const processingEngine = new ProcessingEngineService();

        const stats = processingEngine.getProcessingStats();

        // Get additional database metrics
        const { Pool } = require('pg');
        const pool = new Pool();

        const healthQuery = `
            SELECT
                interface_id,
                get_interface_processing_health(interface_id) as health_data
            FROM interfaces
            WHERE interface_status = 'active'
        `;

        const healthResult = await pool.query(healthQuery);

        res.json({
            success: true,
            engineStats: stats,
            interfaceHealth: healthResult.rows.map(row => ({
                interfaceId: row.interface_id,
                ...row.health_data
            }))
        });
    } catch (error) {
        console.error('Failed to get engine status:', error);
        res.status(500).json({
            success: false,
            error: 'Failed to get engine status: ' + error.message
        });
    }
});

/**
 * Get message lineage for tracking
 * GET /api/wizard/lineage/:messageId
 */
router.get('/lineage/:messageId', requireAuth, async (req, res) => {
    try {
        const { messageId } = req.params;

        const { Pool } = require('pg');
        const pool = new Pool();

        const lineageQuery = `
            SELECT * FROM v_message_lineage
            WHERE message_id = $1
            ORDER BY sequence_number
        `;

        const result = await pool.query(lineageQuery, [messageId]);

        res.json({
            success: true,
            messageId,
            lineage: result.rows
        });
    } catch (error) {
        console.error(`Failed to get lineage for message ${req.params.messageId}:`, error);
        res.status(500).json({
            success: false,
            error: 'Failed to get message lineage: ' + error.message
        });
    }
});

// Interface validation routes
router.post('/check-duplicate', requireAuth, wizardController.checkDuplicateName);

// ====================================
// OPTIMIZED WIZARD ROUTES
// ====================================

// Get mapping templates for optimized wizard
router.get('/mapping-templates', requireAuth, wizardController.getMappingTemplates);

// Validate final configuration before interface creation
router.post('/validate-final-config', requireAuth, wizardController.validateFinalConfig);

// ====================================
// NEW HL7-FHIR MAPPING ROUTES
// ====================================

/**
 * Save wizard-generated HL7-FHIR mapping configuration
 * POST /api/wizard/save-mapping-config
 */
router.post('/save-mapping-config', 
    requireAuth,
    wizardMappingController.saveMappingConfiguration.bind(wizardMappingController)
);

/**
 * Get runtime mapping configuration for Go services
 * GET /api/wizard/runtime-mapping/:interfaceId/:messageType
 */
router.get('/runtime-mapping/:interfaceId/:messageType', async (req, res) => {
    try {
        const { interfaceId, messageType } = req.params;

        // Initialize the mapping service
        const MessageTypeMappingService = require('../services/MessageTypeMappingService');
        const mappingService = new MessageTypeMappingService();

        // Get the effective mapping configuration
        const mappingConfig = await mappingService.getRuntimeMappingConfig(interfaceId, messageType);

        res.json({
            success: true,
            data: {
                interfaceId,
                messageType,
                config: mappingConfig.config,
                usesStandardTemplate: mappingConfig.usesStandard,
                templateName: mappingConfig.templateName
            }
        });

    } catch (error) {
        console.error('Runtime mapping config error:', error);
        res.status(404).json({
            success: false,
            error: error.message
        });
    }
});

/**
 * Get all mappings for an interface
 * GET /api/wizard/interface-mappings/:interfaceId
 */
router.get('/interface-mappings/:interfaceId', requireAuth, async (req, res) => {
    try {
        const { interfaceId } = req.params;

        const MessageTypeMappingService = require('../services/MessageTypeMappingService');
        const mappingService = new MessageTypeMappingService();

        const mappings = await mappingService.getInterfaceMappings(interfaceId);

        res.json({
            success: true,
            data: {
                interfaceId,
                mappings
            }
        });

    } catch (error) {
        console.error('Interface mappings error:', error);
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

/**
 * Get available FHIR templates by message type
 * GET /api/wizard/templates/:messageType
 */
router.get('/templates/:messageType', 
    requireAuth,
    wizardMappingController.getTemplatesByMessageType.bind(wizardMappingController)
);

/**
 * Validate mapping configuration before saving
 * POST /api/wizard/validate-config
 */
router.post('/validate-config', 
    requireAuth,
    wizardMappingController.validateConfiguration.bind(wizardMappingController)
);

// ====================================
// HEALTH CHECK FOR MAPPING SERVICE
// ====================================

/**
 * Health check for mapping service
 * GET /api/wizard/mapping-health
 */
router.get('/mapping-health', (req, res) => {
    try {
        // Basic health check
        res.json({
            success: true,
            service: 'wizard-mapping',
            status: 'healthy',
            timestamp: new Date().toISOString(),
            routes: {
                'save-mapping-config': 'POST /api/wizard/save-mapping-config',
                'runtime-config': 'GET /api/wizard/runtime-config/:interfaceId',
                'templates': 'GET /api/wizard/templates/:messageType',
                'validate-config': 'POST /api/wizard/validate-config'
            }
        });
    } catch (error) {
        res.status(500).json({
            success: false,
            error: 'Mapping service health check failed'
        });
    }
});

module.exports = router;