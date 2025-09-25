// routes/messageRoutes.js
// Routes for enhanced message management

const express = require('express');
const MessageController = require('../controllers/MessageController');

const router = express.Router();

// Session authentication middleware
const sessionAuth = (req, res, next) => {
    if (!req.session.user) {
        return res.status(401).json({
            success: false,
            message: 'Authentication required'
        });
    }
    next();
};

// Test route to verify messages routes are working
router.get('/test', (req, res) => {
    console.log('🧪 /api/messages/test route hit!');
    res.json({
        success: true,
        message: 'Message routes are working!',
        timestamp: new Date().toISOString()
    });
});

// INTERFACE-SPECIFIC MESSAGE ROUTES (Performance Optimized)
// Get messages for a specific interface only
router.get('/interface/:interfaceId', sessionAuth, (req, res) => MessageController.getInterfaceMessages(req, res));

// Interface-specific message statistics
router.get('/interface/:interfaceId/stats', sessionAuth, (req, res) => MessageController.getInterfaceStats(req, res));

// GLOBAL ENDPOINTS REMOVED - Use interface-specific endpoints only
// Redirect global requests to interface selection
router.get('/', sessionAuth, (req, res) => {
    res.status(400).json({
        success: false,
        error: 'Global message viewing disabled. Please specify an interface.',
        message: 'Use /interface/:interfaceId endpoint for performance',
        redirect: '/interfaces.html'
    });
});

router.get('/stats', sessionAuth, (req, res) => {
    res.status(400).json({
        success: false,
        error: 'Global stats disabled. Please use interface-specific stats.',
        message: 'Use /interface/:interfaceId/stats endpoint for performance'
    });
});

// Get detailed message by ID
router.get('/:messageId', sessionAuth, (req, res) => MessageController.getMessageDetail(req, res));

// Send test message to interface
router.post('/send/:interfaceId', sessionAuth, (req, res) => MessageController.sendMessage(req, res));

// HL7 TO FHIR MESSAGE FLOW ROUTES (NEW)
// Send HL7 message from source interface to FHIR target interface
router.post('/flow/hl7-to-fhir', sessionAuth, (req, res) => MessageController.sendHL7ToFHIR(req, res));

// FHIR RECEIVER ENDPOINT (NEW)
// Receive FHIR messages and store them in interface-specific table
router.post('/fhir/Patient', (req, res) => MessageController.receiveFHIRMessage(req, res));
router.post('/fhir/:resourceType', (req, res) => MessageController.receiveFHIRMessage(req, res));

// Get message flow status between interfaces
router.get('/flow/:sourceInterfaceId/:targetInterfaceId/status', sessionAuth, (req, res) => MessageController.getFlowStatus(req, res));

// Reprocess failed message
router.post('/:messageId/reprocess', sessionAuth, (req, res) => MessageController.reprocessMessage(req, res));

// Delete message
router.delete('/:messageId', sessionAuth, (req, res) => MessageController.deleteMessage(req, res));

module.exports = router;