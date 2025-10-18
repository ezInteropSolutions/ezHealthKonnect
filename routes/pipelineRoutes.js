// routes/pipelineRoutes.js
// Pipeline builder routes (Node.js layer)
// MVC: Routes layer - registers endpoints and delegates to controllers

const express = require('express');
const router = express.Router();
const { requireAuth } = require('../middleware/auth');
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

// Save pipeline (create or update)
router.post('/pipelines', requireAuth, pipelineController.savePipeline);

// Load pipeline by ID
router.get('/pipelines/:id', requireAuth, pipelineController.loadPipeline);

// Load pipeline by interface and message type
router.get('/pipelines/interface/:interfaceId/:messageType',
    requireAuth,
    pipelineController.loadPipelineByInterface
);

// List all pipelines for an interface
router.get('/pipelines/interface/:interfaceId',
    requireAuth,
    pipelineController.listPipelines
);

// Delete pipeline
router.delete('/pipelines/:id', requireAuth, pipelineController.deletePipeline);

// Clone pipeline
router.post('/pipelines/:id/clone', requireAuth, pipelineController.clonePipeline);

// ============================================================================
// PIPELINE EXECUTION ROUTES
// ============================================================================

// Test pipeline with sample data
router.post('/pipelines/test', requireAuth, pipelineController.testPipeline);

// Execute pipeline (production)
router.post('/pipelines/execute', requireAuth, pipelineController.executePipeline);

// Get pipeline execution statistics
router.get('/pipelines/:id/stats', requireAuth, pipelineController.getPipelineStats);

// ============================================================================
// TEMPLATE LIBRARY ROUTES
// ============================================================================

// List all templates
router.get('/templates', requireAuth, pipelineController.listTemplates);

// Get template by ID
router.get('/templates/:id', requireAuth, pipelineController.getTemplate);

// Create custom template
router.post('/templates', requireAuth, pipelineController.createTemplate);

// ============================================================================
// STEP EXECUTION ROUTES (for UI testing individual steps)
// ============================================================================

// Execute single validation step
router.post('/processing/execute/validation',
    requireAuth,
    pipelineController.executeValidationStep
);

// Execute single enrichment step
router.post('/processing/execute/enrichment',
    requireAuth,
    pipelineController.executeEnrichmentStep
);

// Execute single mapping step
router.post('/processing/execute/mapping',
    requireAuth,
    pipelineController.executeMappingStep
);

// Execute custom JavaScript step
router.post('/processing/execute/custom',
    requireAuth,
    pipelineController.executeCustomStep
);

module.exports = router;
