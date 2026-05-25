// routes/templateRoutes.js
// Routes for transformation template management

const express = require('express');
const router = express.Router();
const templateController = require('../controllers/templateController');
const { requireAuth, requireRole } = require('../middleware/auth');

// RBAC: writes are operator+
const canRead  = requireRole('admin', 'operator', 'viewer');
const canWrite = requireRole('admin', 'operator');

// List all templates (public access for viewing system templates)
router.get('/', templateController.listTemplates);

// Get single template
router.get('/:id', templateController.getTemplate);

// Create template from existing step                     — operator+
router.post('/from-step', requireAuth, canWrite, templateController.createTemplateFromStep);

// Create template directly                               — operator+
router.post('/', requireAuth, canWrite, templateController.createTemplate);

// Update template                                        — operator+
router.put('/:id', requireAuth, canWrite, templateController.updateTemplate);

// Delete template                                        — operator+
router.delete('/:id', requireAuth, canWrite, templateController.deleteTemplate);

// Apply template to pipeline                             — operator+
router.post('/:id/apply', requireAuth, canWrite, templateController.applyTemplate);

module.exports = router;
