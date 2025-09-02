// routes/wizardRoutes.js
// Clean routes file - only handles routing, no business logic

const express = require('express');
const router = express.Router();
const wizardController = require('../controllers/wizardController');

/**
 * Authentication middleware
 */
const requireAuth = (req, res, next) => {
    if (!req.session?.user) {
        return res.status(401).json({
            success: false,
            error: 'Authentication required'
        });
    }
    next();
};

// Interface configuration routes
router.post('/save-config', requireAuth, wizardController.saveConfiguration);
router.post('/activate-interface', requireAuth, wizardController.activateInterface);
router.post('/complete', requireAuth, wizardController.completeWizard);

// Interface management routes
router.get('/interfaces', requireAuth, wizardController.listInterfaces);
router.get('/interfaces/:id', requireAuth, wizardController.getInterface);
router.get('/interfaces/:id/stats', requireAuth, wizardController.getInterfaceStats);
router.delete('/interfaces/:id', requireAuth, wizardController.deleteInterface);

// Interface validation routes
router.post('/check-duplicate', requireAuth, wizardController.checkDuplicateName);

module.exports = router;