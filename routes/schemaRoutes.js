// routes/schemaRoutes.js
// Routes for schema API (HL7 and FHIR schema loading for XPath IntelliSense)

const express = require('express');
const router = express.Router();
const schemaController = require('../controllers/schemaController');
const { requireAuth, requireRole } = require('../middleware/auth');

// RBAC: all schema endpoints require login; viewer+ can read; operator+ can write
const canRead  = requireRole('admin', 'operator', 'viewer');
const canWrite = requireRole('admin', 'operator');

// HL7 schema routes                                      — viewer+
router.get('/hl7/versions',                  requireAuth, canRead,  schemaController.getHL7Versions);
router.get('/hl7/:version/message-types',    requireAuth, canRead,  schemaController.getHL7MessageTypes);
router.get('/hl7/:version/:messageType',     requireAuth, canRead,  schemaController.getHL7Schema);

// Universal field paths (format-agnostic)               — viewer+
router.get('/hl7/fields',  requireAuth, canRead, schemaController.getUniversalHL7Fields);
router.get('/fhir/fields', requireAuth, canRead, schemaController.getUniversalFHIRFields);
router.get('/cda/fields',  requireAuth, canRead, schemaController.getUniversalCDAFields);

// FHIR schema routes                                     — viewer+
router.get('/fhir/versions',                 requireAuth, canRead,  schemaController.getFHIRVersions);
router.get('/fhir/:version/resources',       requireAuth, canRead,  schemaController.getFHIRResources);
router.get('/fhir/:version/:resourceType',   requireAuth, canRead,  schemaController.getFHIRSchema);

// XPath search/autocomplete                              — viewer+
router.post('/search', requireAuth, canRead, schemaController.searchXPath);

// Sample message management                              — operator+
router.post('/samples', requireAuth, canWrite, schemaController.uploadSample);
router.get('/samples',  requireAuth, canRead,  schemaController.listSamples);

module.exports = router;
