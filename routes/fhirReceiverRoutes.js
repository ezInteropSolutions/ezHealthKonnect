// routes/fhirReceiverRoutes.js
// FHIR Receiver HTTP endpoint routes
// Handles incoming FHIR resources from external systems

const express = require('express');
const router = express.Router();
const FhirReceiverController = require('../controllers/FhirReceiverController');

/**
 * Receive FHIR resource (POST)
 * Endpoint: POST /fhir/receiver/:interfaceId
 * Content-Type: application/fhir+json or application/json
 *
 * Example:
 * POST /fhir/receiver/fafc66da-995a-46e4-b00d-330a9d62a0e0
 * Authorization: Bearer <token>
 * {
 *   "resourceType": "Patient",
 *   "id": "example",
 *   "name": [{"family": "Doe", "given": ["John"]}]
 * }
 */
router.post('/receiver/:interfaceId', FhirReceiverController.receiveResource.bind(FhirReceiverController));

/**
 * Update FHIR resource (PUT)
 * Endpoint: PUT /fhir/receiver/:interfaceId/:resourceType/:resourceId
 * Content-Type: application/fhir+json or application/json
 *
 * Example:
 * PUT /fhir/receiver/fafc66da-995a-46e4-b00d-330a9d62a0e0/Patient/example
 * Authorization: Bearer <token>
 * {
 *   "resourceType": "Patient",
 *   "id": "example",
 *   "name": [{"family": "Smith", "given": ["John"]}]
 * }
 */
router.put('/receiver/:interfaceId/:resourceType/:resourceId', FhirReceiverController.updateResource.bind(FhirReceiverController));

/**
 * Health check endpoint for FHIR receiver
 * GET /fhir/receiver/health
 */
router.get('/receiver/health', (req, res) => {
    res.json({
        status: 'healthy',
        service: 'fhir-receiver',
        timestamp: new Date().toISOString()
    });
});

module.exports = router;
