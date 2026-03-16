// controllers/InterfaceLifecycleController.js
// Interface runtime lifecycle management - start, stop, monitor interfaces

const ProcessingEngineService = require('../services/ProcessingEngineService');

const GO_BACKEND_URL = process.env.GO_BACKEND_URL || `http://localhost:${process.env.API_PORT || 8080}`;

async function goFetch(path, options = {}) {
    let fetch;
    try { fetch = require('node-fetch'); } catch { fetch = global.fetch; }
    return fetch(`${GO_BACKEND_URL}${path}`, { timeout: 10000, ...options });
}

class InterfaceLifecycleController {
    constructor() {
        this.processingEngine = new ProcessingEngineService();
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

            let result;
            if (!response.ok) {
                const errorData = await response.json().catch(() => ({ error: 'Unknown error' }));

                // Special case: If already active, treat as success
                if (response.status === 500 && errorData.error?.includes('already active')) {
                    console.log(`⚠️ Interface already active in Go backend, updating database: ${interfaceId}`);
                    result = { message: 'Interface already active', alreadyActive: true };
                } else {
                    throw new Error(`Go backend activation failed: ${response.status} - ${errorData.error || response.statusText}`);
                }
            } else {
                result = await response.json();
                console.log(`✅ Interface activated via Go backend: ${interfaceId}`, result);
            }

            // Update database interface_status to 'active' (even if already active)
            const database = require('../config/database');
            const sequelize = database.sequelize;
            await sequelize.query(
                'UPDATE interfaces SET interface_status = :status WHERE id = :interface_id',
                {
                    replacements: { status: 'active', interface_id: interfaceId },
                    type: sequelize.QueryTypes.UPDATE
                }
            );

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

            console.log(`⏹️ Deactivating interface: ${interfaceId}`);

            // Call Go backend to deactivate the interface
            let fetch;
            try {
                fetch = require('node-fetch');
            } catch {
                fetch = global.fetch;
            }

            if (!fetch) {
                throw new Error('No fetch implementation available');
            }

            const goBackendUrl = process.env.GO_BACKEND_URL || 'http://localhost:8080';
            const response = await fetch(`${goBackendUrl}/api/processing/interfaces/${interfaceId}/deactivate`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                timeout: 30000
            });

            if (!response.ok) {
                const errorData = await response.json().catch(() => ({ error: 'Unknown error' }));
                throw new Error(`Go backend deactivation failed: ${response.status} - ${errorData.error || response.statusText}`);
            }

            console.log(`✅ Interface stopped via Go backend: ${interfaceId}`);

            // Update database interface_status to 'configured' (stopped state)
            const database = require('../config/database');
            const sequelize = database.sequelize;
            await sequelize.query(
                'UPDATE interfaces SET interface_status = :status WHERE id = :interface_id',
                {
                    replacements: { status: 'configured', interface_id: interfaceId },
                    type: sequelize.QueryTypes.UPDATE
                }
            );

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
                status: 'configured'
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
     * Pause a specific interface (graceful stop - waits for queue to clear)
     */
    async pauseInterface(req, res) {
        try {
            const { interfaceId } = req.params;
            const { graceful, waitForQueue } = req.body;
            const userId = req.user?.id || req.session?.user?.id;

            console.log(`⏸️ Pausing interface: ${interfaceId} (graceful: ${graceful}, waitForQueue: ${waitForQueue})`);

            // Call Go backend to deactivate the interface
            let fetch;
            try {
                fetch = require('node-fetch');
            } catch {
                fetch = global.fetch;
            }

            if (!fetch) {
                throw new Error('No fetch implementation available');
            }

            const goBackendUrl = process.env.GO_BACKEND_URL || 'http://localhost:8080';
            const response = await fetch(`${goBackendUrl}/api/processing/interfaces/${interfaceId}/deactivate`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                timeout: 30000
            });

            if (!response.ok) {
                const errorData = await response.json().catch(() => ({ error: 'Unknown error' }));
                throw new Error(`Go backend deactivation failed: ${response.status} - ${errorData.error || response.statusText}`);
            }

            console.log(`✅ Interface paused via Go backend: ${interfaceId}`);

            // Update database interface_status to 'paused'
            const database = require('../config/database');
            const sequelize = database.sequelize;
            await sequelize.query(
                'UPDATE interfaces SET interface_status = :status WHERE id = :interface_id',
                {
                    replacements: { status: 'paused', interface_id: interfaceId },
                    type: sequelize.QueryTypes.UPDATE
                }
            );

            // Log pause event
            await this.logAuditEvent(userId, 'INTERFACE_PAUSED', {
                interfaceId: interfaceId,
                graceful: graceful,
                waitForQueue: waitForQueue,
                timestamp: new Date()
            });

            res.json({
                success: true,
                message: `Interface ${interfaceId} paused successfully`,
                interfaceId: interfaceId,
                status: 'paused'
            });

        } catch (error) {
            console.error(`❌ Failed to pause interface ${req.params.interfaceId}:`, error);
            res.status(500).json({
                success: false,
                message: 'Failed to pause interface',
                error: error.message,
                interfaceId: req.params.interfaceId
            });
        }
    }

    /**
     * Get interface runtime status — proxies Go backend, falls back to DB
     */
    async getInterfaceStatus(req, res) {
        const { interfaceId } = req.params;
        const userId = req.session?.user?.id;

        // 1. Try Go backend for real runtime status
        let goStatus = null;
        try {
            const goRes = await goFetch(`/api/processing/interfaces/${interfaceId}/status`);
            if (goRes.ok) {
                const goData = await goRes.json();
                goStatus = goData.status;  // { status, messages_processed, ... }
            }
        } catch (e) {
            console.warn(`⚠️ Go status unavailable for ${interfaceId}:`, e.message);
        }

        // 2. Get DB record for interface metadata
        try {
            const database = require('../config/database');
            const sequelize = database.sequelize;
            const rows = await sequelize.query(
                `SELECT id, name, status, source_type, target_type, total_processed, failed_processed
                 FROM interfaces WHERE id = :id AND is_active = TRUE`,
                { replacements: { id: interfaceId }, type: sequelize.QueryTypes.SELECT }
            );

            if (rows.length === 0) {
                return res.status(404).json({ success: false, message: 'Interface not found' });
            }

            const iface = rows[0];
            // Go runtime status takes precedence over DB status for the "active" determination
            const runtimeStatus = goStatus?.status || iface.status;

            return res.json({
                success: true,
                interface: {
                    ...iface,
                    status: runtimeStatus,
                    processingActive: runtimeStatus === 'active',
                    processingStats: {
                        processedCount: goStatus?.messages_processed || iface.total_processed || 0,
                        status: runtimeStatus
                    }
                }
            });
        } catch (err) {
            console.error(`❌ getInterfaceStatus DB error for ${interfaceId}:`, err.message);
            // Last resort: return whatever Go gave us
            if (goStatus) {
                return res.json({ success: true, interface: { status: goStatus.status, processingActive: goStatus.status === 'active' } });
            }
            return res.status(500).json({ success: false, message: err.message });
        }
    }

    /**
     * Get all interface runtime statuses (batch) — proxies Go backend
     */
    async getAllInterfaceStatuses(req, res) {
        try {
            const goRes = await goFetch('/api/processing/interfaces/statuses');
            if (goRes.ok) {
                return res.json(await goRes.json());
            }
        } catch (e) {
            console.warn('⚠️ Go batch statuses unavailable:', e.message);
        }
        // Fallback: empty statuses so UI falls to individual calls
        return res.json({ success: true, statuses: {} });
    }

    /**
     * Get overall processing engine status — proxies Go backend for real state
     */
    async getEngineStatus(req, res) {
        let running = false;
        let goEngineData = {};

        // Try Go backend first for live engine state
        try {
            const goRes = await goFetch('/api/processing/engine/status');
            if (goRes.ok) {
                const goData = await goRes.json();
                running = goData.engine?.running ?? goData.stats?.running ?? false;
                goEngineData = goData.engine || {};
            }
        } catch (e) {
            console.warn('⚠️ Go engine status unavailable:', e.message);
        }

        // Always query DB for the stats the UI needs
        try {
            const database = require('../config/database');
            const sequelize = database.sequelize;

            // Active interfaces: running or paused (operational states)
            const [[activeRow], [statsRow]] = await Promise.all([
                sequelize.query(
                    `SELECT COUNT(*) AS cnt FROM interfaces
                     WHERE interface_status IN ('running','active','paused') AND deleted_at IS NULL`,
                    { type: sequelize.QueryTypes.SELECT }
                ),
                sequelize.query(
                    `SELECT
                        COALESCE(SUM(total_processed), 0) AS total,
                        COALESCE(SUM(successful_processed), 0) AS successful,
                        COALESCE(SUM(failed_processed), 0) AS failed
                     FROM interfaces WHERE deleted_at IS NULL`,
                    { type: sequelize.QueryTypes.SELECT }
                ).catch(() => [{ total: 0, successful: 0, failed: 0 }])
            ]);

            const activeCount = parseInt(activeRow?.cnt) || 0;
            const totalProcessed = parseInt(statsRow?.total) || 0;
            const totalFailed = parseInt(statsRow?.failed) || 0;
            const successRate = totalProcessed > 0
                ? ((totalProcessed / (totalProcessed + totalFailed)) * 100).toFixed(1)
                : 0;

            return res.json({
                success: true,
                engine: {
                    isRunning: running,
                    ...goEngineData,
                    database: {
                        active_interfaces: activeCount,
                        total_messages_processed: totalProcessed,
                        total_messages_failed: totalFailed,
                        success_rate: successRate
                    },
                    today: {
                        messages_today: totalProcessed  // best approximation without per-day counters
                    }
                }
            });
        } catch (dbErr) {
            console.error('❌ getEngineStatus DB error:', dbErr.message);
            return res.json({
                success: true,
                engine: { isRunning: running, database: {}, today: {} }
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