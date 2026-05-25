// routes/pipelineRoutes.js
// Pipeline builder routes (Node.js layer)
// MVC: Routes layer - registers endpoints and delegates to controllers

const express = require('express');
const router = express.Router();
const { requireAuth, requireRole } = require('../middleware/auth');

// RBAC: pipelines are read by admin+operator+viewer; written by admin+operator
const canRead  = requireRole('admin', 'operator', 'viewer');
const canWrite = requireRole('admin', 'operator');
const pipelineController = require('../controllers/pipelineController');

// Debug: Verify all controller functions
console.log('🔍 [pipelineRoutes] Checking controller functions:');
const requiredFunctions = [
    'savePipeline', 'loadPipeline', 'loadPipelineByInterface', 'listPipelines',
    'deletePipeline', 'clonePipeline', 'testPipeline', 'executePipeline',
    'getPipelineStats', 'listTemplates', 'getTemplate', 'createTemplate',
    'executeValidationStep', 'executeEnrichmentStep', 'executeMappingStep', 'executeCustomStep'
];

requiredFunctions.forEach(fn => {
    const exists = typeof pipelineController[fn] === 'function';
    console.log(`   ${exists ? '✅' : '❌'} ${fn}: ${typeof pipelineController[fn]}`);
});

// ============================================================================
// PIPELINE MANAGEMENT ROUTES
// ============================================================================

// Save pipeline (create or update)                      — operator+
router.post('/pipelines', requireAuth, canWrite, pipelineController.savePipeline);

// Load pipeline by ID                                   — viewer+
router.get('/pipelines/:id', requireAuth, canRead, pipelineController.loadPipeline);

// Load pipeline by interface and message type           — viewer+
router.get('/pipelines/interface/:interfaceId/:messageType',
    requireAuth, canRead,
    pipelineController.loadPipelineByInterface
);

// List all pipelines for an interface                   — viewer+
router.get('/pipelines/interface/:interfaceId',
    requireAuth, canRead,
    pipelineController.listPipelines
);

// Delete pipeline                                       — operator+
router.delete('/pipelines/:id', requireAuth, canWrite, pipelineController.deletePipeline);

// Clone pipeline                                        — operator+
router.post('/pipelines/:id/clone', requireAuth, canWrite, pipelineController.clonePipeline);

// ============================================================================
// PIPELINE EXECUTION ROUTES
// ============================================================================

// Test pipeline with sample data                        — operator+
router.post('/pipelines/test', requireAuth, canWrite, pipelineController.testPipeline);

// Execute pipeline (production)                         — operator+
router.post('/pipelines/execute', requireAuth, canWrite, pipelineController.executePipeline);

// Get pipeline execution statistics                     — viewer+
router.get('/pipelines/:id/stats', requireAuth, canRead, pipelineController.getPipelineStats);

// ============================================================================
// TEMPLATE LIBRARY ROUTES
// ============================================================================

// List all templates                                    — viewer+
router.get('/templates', requireAuth, canRead, pipelineController.listTemplates);

// Get template by ID                                    — viewer+
router.get('/templates/:id', requireAuth, canRead, pipelineController.getTemplate);

// Create custom template                                — operator+
router.post('/templates', requireAuth, canWrite, pipelineController.createTemplate);

// ============================================================================
// STEP EXECUTION ROUTES (for UI testing individual steps)
// ============================================================================

// Execute single validation step                        — operator+
router.post('/processing/execute/validation',
    requireAuth, canWrite,
    pipelineController.executeValidationStep
);

// Execute single enrichment step                        — operator+
router.post('/processing/execute/enrichment',
    requireAuth, canWrite,
    pipelineController.executeEnrichmentStep
);

// Execute single mapping step                           — operator+
router.post('/processing/execute/mapping',
    requireAuth, canWrite,
    pipelineController.executeMappingStep
);

// Execute custom JavaScript step                        — operator+
router.post('/processing/execute/custom',
    requireAuth, canWrite,
    pipelineController.executeCustomStep
);

// ============================================================================
// HL7-FHIR MAPPING TEMPLATES
// ============================================================================

// Get standard HL7-FHIR template mappings by message type — viewer+
router.get('/hl7-fhir-templates/:messageType',
    requireAuth, canRead,
    pipelineController.getStandardTemplateMappings
);

module.exports = router;
