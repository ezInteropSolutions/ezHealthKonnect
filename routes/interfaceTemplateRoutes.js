'use strict';
// routes/interfaceTemplateRoutes.js
// Routes for interface-level template management.
// Mounted at /api/interface-templates in app.js.
// Note: /api/interfaces/:id/save-as-template and /api/interfaces/:id/template-preview
//       are also registered here but consumed via interfacesRoutes mount point below.

const express = require('express');
const router = express.Router();
const ctrl = require('../controllers/interfaceTemplateController');
const { requireAuth: isAuthenticated, requireRole } = require('../middleware/auth');

// RBAC: writes are operator+
const canWrite = requireRole('admin', 'operator');

// ── Read (public — system + public templates visible without login) ───────────
router.get('/', ctrl.listTemplates);
router.get('/categories', ctrl.listCategories);
router.get('/:id', ctrl.getTemplate);

// ── Use a template (increment usage_count, return scaffold) ──────────────────
router.post('/:id/use', ctrl.useTemplate);

// ── Write (requires authentication + operator role) ──────────────────────────
router.post('/', isAuthenticated, canWrite, ctrl.createTemplate);
router.put('/:id', isAuthenticated, canWrite, ctrl.updateTemplate);
router.delete('/:id', isAuthenticated, canWrite, ctrl.deleteTemplate);

module.exports = router;
