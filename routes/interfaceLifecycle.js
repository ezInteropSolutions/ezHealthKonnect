// routes/interfaceLifecycle.js
// Runtime interface lifecycle management routes

const express = require('express');
const router = express.Router();
const InterfaceLifecycleController = require('../controllers/InterfaceLifecycleController');
const { requireRole } = require('../middleware/auth');

// RBAC: reads are viewer+; writes (start/stop/activate/deactivate/pause) are operator+
const canRead  = requireRole('admin', 'operator', 'viewer');
const canWrite = requireRole('admin', 'operator');

// Session authentication middleware
const requireAuth = (req, res, next) => {
    if (req.session && req.session.user) {
        return next();
    }
    return res.status(401).json({
        success: false,
        message: 'Authentication required'
    });
};

// Test route
router.get('/test', (req, res) => {
    res.json({ success: true, message: 'Runtime routes working!' });
});

// Processing Engine Management
router.post('/engine/start',  requireAuth, canWrite, InterfaceLifecycleController.startProcessingEngine.bind(InterfaceLifecycleController));
router.post('/engine/stop',   requireAuth, canWrite, InterfaceLifecycleController.stopProcessingEngine.bind(InterfaceLifecycleController));
router.get('/engine/status',  requireAuth, canRead,  InterfaceLifecycleController.getEngineStatus.bind(InterfaceLifecycleController));

// Individual Interface Management
router.post('/interfaces/:interfaceId/activate',   requireAuth, canWrite, InterfaceLifecycleController.activateInterface.bind(InterfaceLifecycleController));
router.post('/interfaces/:interfaceId/deactivate', requireAuth, canWrite, InterfaceLifecycleController.deactivateInterface.bind(InterfaceLifecycleController));
router.post('/interfaces/:interfaceId/pause',      requireAuth, canWrite, InterfaceLifecycleController.pauseInterface.bind(InterfaceLifecycleController));
router.get('/interfaces/statuses',                 requireAuth, canRead,  InterfaceLifecycleController.getAllInterfaceStatuses.bind(InterfaceLifecycleController));
router.get('/interfaces/:interfaceId/status',      requireAuth, canRead,  InterfaceLifecycleController.getInterfaceStatus.bind(InterfaceLifecycleController));
router.get('/interfaces/:interfaceId/history',     requireAuth, canRead,  InterfaceLifecycleController.getInterfaceHistory.bind(InterfaceLifecycleController));

module.exports = router;