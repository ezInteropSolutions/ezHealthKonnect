// routes/messageRoutes.js
// Routes for enhanced message management

const express = require('express');
const MessageController = require('../controllers/MessageController');
const { requireRole } = require('../middleware/auth');

const router = express.Router();

// RBAC: messages are visible to operator+viewer; send/delete require operator+
const canRead  = requireRole('admin', 'operator', 'viewer');
const canWrite = requireRole('admin', 'operator');

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
router.get('/interface/:interfaceId',                                     sessionAuth, canRead,  (req, res) => MessageController.getInterfaceMessages(req, res));
router.get('/interface/:interfaceId/stats',                               sessionAuth, canRead,  (req, res) => MessageController.getInterfaceStats(req, res));
router.get('/interface/:interfaceId/message/:messageId/parsed',           sessionAuth, canRead,  (req, res) => MessageController.getParsedContent(req, res));
router.get('/interface/:interfaceId/message/:messageId/fhir-output',      sessionAuth, canRead,  (req, res) => MessageController.getFhirOutput(req, res));

// BULK REPROCESS (V213) — registered before the /:messageId catch-all below,
// same reason /interface/:interfaceId is: a bare literal path like
// GET /bulk-reprocess would otherwise be swallowed by /:messageId (Express
// matches route registration order, not specificity). /count and /:jobId are
// both single-segment GETs under /bulk-reprocess/*, so /count must also come
// before /:jobId for the same reason.
router.get('/bulk-reprocess/count',        sessionAuth, canRead,  (req, res) => MessageController.getBulkReprocessCount(req, res));
router.get('/bulk-reprocess',              sessionAuth, canRead,  (req, res) => MessageController.listBulkReprocessJobs(req, res));
router.post('/bulk-reprocess',             sessionAuth, canWrite, (req, res) => MessageController.createBulkReprocessJob(req, res));
router.get('/bulk-reprocess/:jobId',       sessionAuth, canRead,  (req, res) => MessageController.getBulkReprocessJob(req, res));
router.post('/bulk-reprocess/:jobId/cancel', sessionAuth, canWrite, (req, res) => MessageController.cancelBulkReprocessJob(req, res));

// GLOBAL ENDPOINTS REMOVED - Use interface-specific endpoints only
router.get('/', sessionAuth, canRead, (req, res) => {
    res.status(400).json({ success: false, error: 'Global message viewing disabled. Please specify an interface.', redirect: '/interfaces.html' });
});
router.get('/stats', sessionAuth, canRead, (req, res) => {
    res.status(400).json({ success: false, error: 'Global stats disabled. Use /interface/:interfaceId/stats' });
});

// Message detail reads                                   — viewer+
router.get('/:messageId',                     sessionAuth, canRead,  (req, res) => MessageController.getMessageDetail(req, res));
router.get('/:messageId/lineage',             sessionAuth, canRead,  (req, res) => MessageController.getDataLineage(req, res));
router.get('/:messageId/dedupe-suppressions', sessionAuth, canRead,  (req, res) => MessageController.getDedupeSuppressions(req, res));
router.get('/:messageId/coverage-audit',      sessionAuth, canRead,  (req, res) => MessageController.getCoverageAudit(req, res));
router.get('/:messageId/errors',              sessionAuth, canRead,  (req, res) => MessageController.getMessageErrors(req, res));
router.get('/:messageId/logs',                sessionAuth, canRead,  (req, res) => MessageController.getMessageLogs(req, res));

// Message flow status                                    — viewer+
router.get('/flow/:sourceInterfaceId/:targetInterfaceId/status', sessionAuth, canRead, (req, res) => MessageController.getFlowStatus(req, res));

// Write operations                                       — operator+
router.post('/send/:interfaceId',   sessionAuth, canWrite, (req, res) => MessageController.sendMessage(req, res));
router.post('/:messageId/reprocess', sessionAuth, canWrite, (req, res) => MessageController.reprocessMessage(req, res));
router.delete('/:messageId',        sessionAuth, canWrite, (req, res) => MessageController.deleteMessage(req, res));

// FHIR RECEIVER — unauthenticated (system-to-system endpoint)
router.post('/fhir/Patient',       (req, res) => MessageController.receiveFHIRMessage(req, res));
router.post('/fhir/:resourceType', (req, res) => MessageController.receiveFHIRMessage(req, res));

module.exports = router;