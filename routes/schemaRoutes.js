// routes/schemaRoutes.js
// Routes for schema API (HL7 and FHIR schema loading for XPath IntelliSense)

const express = require('express');
const router = express.Router();
const schemaController = require('../controllers/schemaController');

// HL7 schema routes
router.get('/hl7/versions', schemaController.getHL7Versions);
router.get('/hl7/:version/message-types', schemaController.getHL7MessageTypes);
router.get('/hl7/:version/:messageType', schemaController.getHL7Schema);

// Universal field paths (format-agnostic)
router.get('/hl7/fields', schemaController.getUniversalHL7Fields);
router.get('/fhir/fields', schemaController.getUniversalFHIRFields);
router.get('/cda/fields', schemaController.getUniversalCDAFields);

// FHIR schema routes
router.get('/fhir/versions', schemaController.getFHIRVersions);
router.get('/fhir/:version/resources', schemaController.getFHIRResources);
router.get('/fhir/:version/:resourceType', schemaController.getFHIRSchema);

// XPath search/autocomplete
router.post('/search', schemaController.searchXPath);

// Sample message management
router.post('/samples', schemaController.uploadSample);
router.get('/samples', schemaController.listSamples);

module.exports = router;
