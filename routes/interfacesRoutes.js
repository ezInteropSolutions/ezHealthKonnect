// routes/interfacesRoutes.js - FIXED with proper method binding
const express = require('express');
const interfacesController = require('../controllers/interfacesController');

const router = express.Router();

// Session authentication middleware
const sessionAuth = (req, res, next) => {
    console.log('\n=== INTERFACES ROUTE AUTH ===');
    console.log('🔍 Request URL:', req.url);
    console.log('🔍 Method:', req.method);
    console.log('🔍 Session user:', req.session?.user?.email);
    
    if (!req.session.user) {
        console.log('❌ No session user found - sending 401');
        return res.status(401).json({ message: 'Not authenticated' });
    }
    
    console.log('✅ Session auth successful for user:', req.session.user.email);
    next();
};

// FIXED: Properly bind controller methods to maintain 'this' context
router.get('/', sessionAuth, (req, res) => interfacesController.getAllInterfaces(req, res));
router.post('/', sessionAuth, (req, res) => interfacesController.createInterface(req, res));
router.get('/:interfaceId', sessionAuth, (req, res) => interfacesController.getInterface(req, res));
router.delete('/:interfaceId', sessionAuth, (req, res) => interfacesController.deleteInterface(req, res));
router.post('/:interfaceId/start', sessionAuth, (req, res) => interfacesController.startInterface(req, res));
router.post('/:interfaceId/stop', sessionAuth, (req, res) => interfacesController.stopInterface(req, res));
router.post('/:interfaceId/pause', sessionAuth, (req, res) => interfacesController.pauseInterface(req, res));

module.exports = router;