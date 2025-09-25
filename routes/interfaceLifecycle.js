// routes/interfaceLifecycle.js
// Runtime interface lifecycle management routes

const express = require('express');
const router = express.Router();
const InterfaceLifecycleController = require('../controllers/InterfaceLifecycleController');

// Middleware for authentication
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
router.post('/engine/start', requireAuth, InterfaceLifecycleController.startProcessingEngine.bind(InterfaceLifecycleController));
router.post('/engine/stop', requireAuth, InterfaceLifecycleController.stopProcessingEngine.bind(InterfaceLifecycleController));
router.get('/engine/status', requireAuth, InterfaceLifecycleController.getEngineStatus.bind(InterfaceLifecycleController));

// Individual Interface Management
router.post('/interfaces/:interfaceId/activate', requireAuth, InterfaceLifecycleController.activateInterface.bind(InterfaceLifecycleController));
router.post('/interfaces/:interfaceId/deactivate', requireAuth, InterfaceLifecycleController.deactivateInterface.bind(InterfaceLifecycleController));
router.get('/interfaces/:interfaceId/status', requireAuth, InterfaceLifecycleController.getInterfaceStatus.bind(InterfaceLifecycleController));
router.get('/interfaces/:interfaceId/history', requireAuth, InterfaceLifecycleController.getInterfaceHistory.bind(InterfaceLifecycleController));

module.exports = router;