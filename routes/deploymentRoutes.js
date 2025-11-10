// routes/deploymentRoutes.js
// API endpoints for interface deployment lifecycle management

const express = require('express');
const router = express.Router();

/**
 * POST /api/deployment/deploy/:interfaceId
 * Manually deploy (activate) an interface
 */
router.post('/deploy/:interfaceId', async (req, res) => {
    try {
        if (!global.deploymentService) {
            return res.status(503).json({
                success: false,
                error: 'Deployment service not available'
            });
        }

        const { interfaceId } = req.params;
        console.log(`📥 Manual deployment request for interface: ${interfaceId}`);

        const result = await global.deploymentService.manualDeploy(interfaceId);

        if (result.success) {
            return res.json({
                success: true,
                message: result.message
            });
        } else {
            return res.status(500).json({
                success: false,
                error: result.message
            });
        }

    } catch (error) {
        console.error('Manual deployment error:', error);
        return res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

/**
 * POST /api/deployment/stop/:interfaceId
 * Stop (deactivate) an interface
 */
router.post('/stop/:interfaceId', async (req, res) => {
    try {
        if (!global.deploymentService) {
            return res.status(503).json({
                success: false,
                error: 'Deployment service not available'
            });
        }

        const { interfaceId } = req.params;
        console.log(`📥 Stop request for interface: ${interfaceId}`);

        const result = await global.deploymentService.stopInterface(interfaceId);

        if (result.success) {
            return res.json({
                success: true,
                message: result.message
            });
        } else {
            return res.status(500).json({
                success: false,
                error: result.message
            });
        }

    } catch (error) {
        console.error('Stop interface error:', error);
        return res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

/**
 * GET /api/deployment/status
 * Get deployment status for all interfaces
 */
router.get('/status', async (req, res) => {
    try {
        if (!global.deploymentService) {
            return res.status(503).json({
                success: false,
                error: 'Deployment service not available'
            });
        }

        const status = await global.deploymentService.getDeploymentStatus();

        return res.json({
            success: true,
            interfaces: status
        });

    } catch (error) {
        console.error('Get deployment status error:', error);
        return res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

/**
 * GET /api/deployment/status/:interfaceId
 * Get deployment status for specific interface
 */
router.get('/status/:interfaceId', async (req, res) => {
    try {
        const database = require('../config/database');
        const { interfaceId } = req.params;

        const interfaces = await database.sequelize.query(`
            SELECT
                id,
                name,
                deployment_mode,
                auto_start,
                deployment_delay_seconds,
                deployment_status,
                last_deployed_at,
                status
            FROM interfaces
            WHERE id = :interfaceId
              AND is_active = true
              AND deleted_at IS NULL
        `, {
            replacements: { interfaceId },
            type: database.sequelize.QueryTypes.SELECT
        });

        if (interfaces.length === 0) {
            return res.status(404).json({
                success: false,
                error: 'Interface not found'
            });
        }

        return res.json({
            success: true,
            interface: interfaces[0]
        });

    } catch (error) {
        console.error('Get interface deployment status error:', error);
        return res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

module.exports = router;
