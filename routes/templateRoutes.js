// routes/templateRoutes.js
// Routes for transformation template management

const express = require('express');
const router = express.Router();
const templateController = require('../controllers/templateController');
const { isAuthenticated } = require('../middleware/auth');

// List all templates (public access for viewing system templates)
router.get('/', templateController.listTemplates);

// Get single template
router.get('/:id', templateController.getTemplate);

// Create template from existing step (requires auth)
router.post('/from-step', isAuthenticated, templateController.createTemplateFromStep);

// Create template directly (requires auth)
router.post('/', isAuthenticated, templateController.createTemplate);

// Update template (requires auth + ownership)
router.put('/:id', isAuthenticated, templateController.updateTemplate);

// Delete template (requires auth + ownership)
router.delete('/:id', isAuthenticated, templateController.deleteTemplate);

// Apply template to pipeline (requires auth)
router.post('/:id/apply', isAuthenticated, templateController.applyTemplate);

module.exports = router;
