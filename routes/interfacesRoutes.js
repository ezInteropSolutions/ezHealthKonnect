// routes/interfacesRoutes.js - FIXED with proper method binding
const express = require('express');
const interfacesController = require('../controllers/interfacesController');
const { requireRole } = require('../middleware/auth');

const router = express.Router();

// Session authentication middleware
const sessionAuth = (req, res, next) => {
    if (!req.session.user) {
        return res.status(401).json({ message: 'Not authenticated' });
    }
    next();
};

// RBAC helpers
// Reads:   admin, operator, viewer
// Writes:  admin, operator
// Control: admin, operator  (start/stop/pause)
const canRead    = requireRole('admin', 'operator', 'viewer');
const canWrite   = requireRole('admin', 'operator');

// FIXED: Properly bind controller methods to maintain 'this' context
router.get('/', sessionAuth, canRead,  (req, res) => interfacesController.getAllInterfaces(req, res));
router.post('/', sessionAuth, canWrite, (req, res) => interfacesController.createInterface(req, res));
router.get('/:interfaceId', sessionAuth, canRead,  (req, res) => interfacesController.getInterface(req, res));
router.put('/:interfaceId', sessionAuth, canWrite, (req, res) => interfacesController.updateInterface(req, res));
router.delete('/:interfaceId', sessionAuth, canWrite, (req, res) => interfacesController.deleteInterface(req, res));
router.post('/:interfaceId/start', sessionAuth, canWrite, (req, res) => interfacesController.startInterface(req, res));
router.post('/:interfaceId/stop',  sessionAuth, canWrite, (req, res) => interfacesController.stopInterface(req, res));
router.post('/:interfaceId/pause', sessionAuth, canWrite, (req, res) => interfacesController.pauseInterface(req, res));

module.exports = router;