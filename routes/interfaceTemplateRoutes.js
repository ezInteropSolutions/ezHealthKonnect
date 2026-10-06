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
// Session auth required — found unauthenticated during a security-validation
// pass on the device-connectivity feature: this sat between the public "Read"
// block above and the authenticated "Write" block below with no auth of its
// own, despite being a POST with a real side effect (usage_count increment).
// The real UI only ever calls this from an already-authenticated dashboard
// session, so this closes a gap without changing real behavior.
router.post('/:id/use', isAuthenticated, ctrl.useTemplate);

// ── Write (requires authentication + operator role) ──────────────────────────
router.post('/', isAuthenticated, canWrite, ctrl.createTemplate);
router.put('/:id', isAuthenticated, canWrite, ctrl.updateTemplate);
router.delete('/:id', isAuthenticated, canWrite, ctrl.deleteTemplate);

module.exports = router;
