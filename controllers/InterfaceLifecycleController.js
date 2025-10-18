// controllers/InterfaceLifecycleController.js
// Interface runtime lifecycle management - start, stop, monitor interfaces

const ProcessingEngineService = require('../services/ProcessingEngineService');

class InterfaceLifecycleController {
    constructor() {
        this.processingEngine = new ProcessingEngineService();
        this.isEngineRunning = false;
    }

    /**
     * Start the processing engine
     */
    async startProcessingEngine(req, res) {
        try {
            console.log('🚀 Starting Processing Engine...');

            if (this.isEngineRunning) {
                return res.status(400).json({
                    success: false,
                    message: 'Processing Engine is already running'
                });
            }

            await this.processingEngine.start();
            this.isEngineRunning = true;

            res.json({
                success: true,
                message: 'Processing Engine started successfully',
                stats: this.processingEngine.getProcessingStats()
            });

        } catch (error) {
            console.error('❌ Failed to start Processing Engine:', error);
            res.status(500).json({
                success: false,
                message: 'Failed to start Processing Engine',
                error: error.message
            });
        }
    }

    /**
     * Stop the processing engine
     */
    async stopProcessingEngine(req, res) {
        try {
            console.log('🛑 Stopping Processing Engine...');

            if (!this.isEngineRunning) {
                return res.status(400).json({
                    success: false,
                    message: 'Processing Engine is not running'
                });
            }

            await this.processingEngine.shutdown();
            this.isEngineRunning = false;

            res.json({
                success: true,
                message: 'Processing Engine stopped successfully'
            });

        } catch (error) {
            console.error('❌ Failed to stop Processing Engine:', error);
            res.status(500).json({
                success: false,
                message: 'Failed to stop Processing Engine',
                error: error.message
            });
        }
    }

    /**
     * Activate a specific interface for processing
     */
    async activateInterface(req, res) {
        try {
            const { interfaceId } = req.params;
            const userId = req.user?.id || req.session?.user?.id;

            console.log(`🔄 Activating interface: ${interfaceId}`);

            // Activate interface via Go backend processing engine
            console.log('🚀 Activating interface via Go processing engine...');

            // Get fetch implementation
            let fetch;
            try {
                fetch = require('node-fetch');
            } catch {
                fetch = global.fetch;
            }

            if (!fetch) {
                throw new Error('No fetch implementation available');
            }

            // Call Go backend to activate interface
            const goBackendUrl = process.env.GO_BACKEND_URL || 'http://localhost:8080';
            const response = await fetch(`${goBackendUrl}/api/processing/interfaces/${interfaceId}/activate`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                timeout: 30000
            });

            if (!response.ok) {
                const errorData = await response.json().catch(() => ({ error: 'Unknown error' }));
                throw new Error(`Go backend activation failed: ${response.status} - ${errorData.error || response.statusText}`);
            }

            const result = await response.json();
            console.log(`✅ Interface activated via Go backend: ${interfaceId}`, result);

            // Log activation event
            await this.logAuditEvent(userId, 'INTERFACE_ACTIVATED', {
                interfaceId: interfaceId,
                timestamp: new Date()
            });

            res.json({
                success: true,
                message: `Interface ${interfaceId} activated successfully`,
                interfaceId: interfaceId,
                status: 'active'
            });

        } catch (error) {
            console.error(`❌ Failed to activate interface ${req.params.interfaceId}:`, error);
            res.status(500).json({
                success: false,
                message: 'Failed to activate interface',
                error: error.message,
                interfaceId: req.params.interfaceId
            });
        }
    }

    /**
     * Deactivate a specific interface
     */
    async deactivateInterface(req, res) {
        try {
            const { interfaceId } = req.params;
            const { reason } = req.body;
            const userId = req.user?.id || req.session?.user?.id;

            console.log(`⏸️ Deactivating interface: ${interfaceId}`);

            await this.processingEngine.deactivateInterface(interfaceId, reason || 'manual');

            // Log deactivation event
            await this.logAuditEvent(userId, 'INTERFACE_DEACTIVATED', {
                interfaceId: interfaceId,
                reason: reason || 'manual',
                timestamp: new Date()
            });

            res.json({
                success: true,
                message: `Interface ${interfaceId} deactivated successfully`,
                interfaceId: interfaceId,
                status: 'paused'
            });

        } catch (error) {
            console.error(`❌ Failed to deactivate interface ${req.params.interfaceId}:`, error);
            res.status(500).json({
                success: false,
                message: 'Failed to deactivate interface',
                error: error.message,
                interfaceId: req.params.interfaceId
            });
        }
    }

    /**
     * Get interface runtime status
     */
    async getInterfaceStatus(req, res) {
        try {
            const { interfaceId } = req.params;
            const userId = req.user?.id || req.session?.user?.id;

            // Get interface configuration from database
            const database = require('../config/database');
            const sequelize = database.sequelize;

            const result = await sequelize.query(`
                SELECT
                    i.*,
                    ipm.messages_processed,
                    ipm.messages_failed,
                    ipm.avg_processing_time_ms,
                    CASE
                        WHEN (ipm.messages_processed + ipm.messages_failed) > 0
                        THEN ROUND((ipm.messages_processed::numeric / (ipm.messages_processed + ipm.messages_failed)) * 100, 2)
                        ELSE 100
                    END as success_rate_percent
                FROM interfaces i
                LEFT JOIN interface_processing_metrics ipm ON i.id = ipm.interface_id
                    AND ipm.metric_date = CURRENT_DATE
                WHERE i.id = :interface_id AND i.user_id = :user_id
            `, {
                replacements: { interface_id: interfaceId, user_id: userId },
                type: sequelize.QueryTypes.SELECT
            });

            if (result.length === 0) {
                return res.status(404).json({
                    success: false,
                    message: 'Interface not found'
                });
            }

            const interfaceData = result[0];

            // Get REAL runtime status from Go backend
            let goBackendStatus = null;
            let processingActive = false;
            let processedCount = 0;

            try {
                // Get fetch implementation
                let fetch;
                try {
                    fetch = require('node-fetch');
                } catch {
                    fetch = global.fetch;
                }

                if (fetch) {
                    const goBackendUrl = process.env.GO_BACKEND_URL || 'http://localhost:8080';
                    const response = await fetch(`${goBackendUrl}/api/processing/interfaces/${interfaceId}/status`, {
                        method: 'GET',
                        timeout: 5000
                    });

                    if (response.ok) {
                        const goData = await response.json();
                        goBackendStatus = goData.status;
                        processingActive = goBackendStatus?.status === 'active';
                        processedCount = goBackendStatus?.messages_processed || 0;
                        console.log(`✅ Got runtime status from Go backend:`, { interfaceId, status: goBackendStatus?.status, processedCount });
                    }
                }
            } catch (goError) {
                console.warn(`⚠️ Failed to get Go backend status for ${interfaceId}:`, goError.message);
                // Continue with database status only
            }

            res.json({
                success: true,
                interface: {
                    ...interfaceData,
                    engineRunning: this.isEngineRunning,
                    processingActive: processingActive,
                    processingStats: {
                        processedCount: processedCount,
                        status: goBackendStatus?.status || 'unknown'
                    }
                }
            });

        } catch (error) {
            console.error(`❌ Failed to get interface status ${req.params.interfaceId}:`, error);
            res.status(500).json({
                success: false,
                message: 'Failed to get interface status',
                error: error.message
            });
        }
    }

    /**
     * Get overall processing engine status
     */
    async getEngineStatus(req, res) {
        try {
            const stats = this.processingEngine.getProcessingStats();

            // Get database statistics using existing database config
            const database = require('../config/database');
            const sequelize = database.sequelize;

            const dbStats = await sequelize.query(`
                SELECT
                    COUNT(*) as total_interfaces,
                    COUNT(CASE WHEN interface_status = 'active' THEN 1 END) as active_interfaces,
                    COUNT(CASE WHEN interface_status = 'error' THEN 1 END) as error_interfaces,
                    SUM(COALESCE(total_processed, 0)) as total_messages_processed,
                    SUM(COALESCE(failed_processed, 0)) as total_messages_failed,
                    SUM(COALESCE(successful_processed, 0)) as total_messages_successful
                FROM interfaces
                WHERE is_active = TRUE
            `, {
                type: sequelize.QueryTypes.SELECT
            });

            const recentActivity = await sequelize.query(`
                SELECT
                    COUNT(*) as messages_today,
                    COUNT(CASE WHEN event_type = 'ERROR' THEN 1 END) as errors_today
                FROM message_audit_log
                WHERE created_at >= CURRENT_DATE
            `, {
                type: sequelize.QueryTypes.SELECT
            });

            res.json({
                success: true,
                engine: {
                    ...stats,
                    isRunning: this.isEngineRunning,
                    database: dbStats[0],
                    today: recentActivity[0]
                }
            });

        } catch (error) {
            console.error('❌ Failed to get engine status:', error);
            res.status(500).json({
                success: false,
                message: 'Failed to get engine status',
                error: error.message
            });
        }
    }

    /**
     * Get interface processing history
     */
    async getInterfaceHistory(req, res) {
        try {
            const { interfaceId } = req.params;
            const { page = 1, limit = 25, status, dateFrom, dateTo } = req.query;
            const userId = req.user?.id || req.session?.user?.id;

            const offset = (page - 1) * limit;
            const database = require('../config/database');
            const sequelize = database.sequelize;

            // Verify user owns the interface
            const interfaceCheck = await sequelize.query(
                'SELECT 1 FROM interfaces WHERE id = :interface_id AND user_id = :user_id',
                {
                    replacements: { interface_id: interfaceId, user_id: userId },
                    type: sequelize.QueryTypes.SELECT
                }
            );

            if (interfaceCheck.length === 0) {
                return res.status(404).json({
                    success: false,
                    message: 'Interface not found'
                });
            }

            // Build WHERE clause for replacements
            let whereConditions = ['mal.interface_id = :interface_id'];
            let replacements = { interface_id: interfaceId, limit: limit, offset: offset };

            if (status) {
                whereConditions.push('mal.event_type = :status');
                replacements.status = status;
            }

            if (dateFrom) {
                whereConditions.push('mal.created_at >= :date_from');
                replacements.date_from = dateFrom;
            }

            if (dateTo) {
                whereConditions.push('mal.created_at <= :date_to');
                replacements.date_to = dateTo;
            }

            const query = `
                SELECT
                    mal.*
                FROM message_audit_log mal
                WHERE ${whereConditions.join(' AND ')}
                ORDER BY mal.created_at DESC
                LIMIT :limit OFFSET :offset
            `;

            const result = await sequelize.query(query, {
                replacements: replacements,
                type: sequelize.QueryTypes.SELECT
            });

            // Get total count for pagination
            const countQuery = `
                SELECT COUNT(*) as count
                FROM message_audit_log mal
                WHERE ${whereConditions.join(' AND ')}
            `;
            const countResult = await sequelize.query(countQuery, {
                replacements: { interface_id: interfaceId, status, date_from: dateFrom, date_to: dateTo },
                type: sequelize.QueryTypes.SELECT
            });

            res.json({
                success: true,
                history: result,
                pagination: {
                    currentPage: parseInt(page),
                    totalPages: Math.ceil(countResult[0].count / limit),
                    totalCount: parseInt(countResult[0].count),
                    hasNextPage: page < Math.ceil(countResult[0].count / limit),
                    hasPreviousPage: page > 1
                }
            });

        } catch (error) {
            console.error(`❌ Failed to get interface history ${req.params.interfaceId}:`, error);
            res.status(500).json({
                success: false,
                message: 'Failed to get interface history',
                error: error.message
            });
        }
    }

    /**
     * Log audit event
     */
    async logAuditEvent(userId, eventType, eventDetails) {
        try {
            const database = require('../config/database');
            const sequelize = database.sequelize;

            await sequelize.query(`
                INSERT INTO audit_logs (
                    id, user_id, event_type, event_details,
                    ip_address, user_agent, created_at
                ) VALUES (
                    gen_random_uuid(), :user_id, :event_type, :event_details,
                    '127.0.0.1', 'ProcessingEngine', CURRENT_TIMESTAMP
                )
            `, {
                replacements: {
                    user_id: userId,
                    event_type: eventType,
                    event_details: JSON.stringify(eventDetails)
                },
                type: sequelize.QueryTypes.INSERT
            });

        } catch (error) {
            console.error('Failed to log audit event:', error);
            // Don't throw - audit logging shouldn't break main functionality
        }
    }
}

module.exports = new InterfaceLifecycleController();