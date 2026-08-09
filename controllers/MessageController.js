// controllers/MessageController.js
// Enhanced message management controller for interface-specific functionality
const { goClient: _goClient } = require('../services/goBackendClient');
const { hasCoverageGapSql } = require('../services/coverageGapSql');

class MessageController {
    constructor() {
        this.database = require('../config/database');
        this.tableManager = require('../services/InterfaceTableManager');
    }

    async ensureDatabase() {
        if (!this.database) {
            this.database = require('../config/database');
        }
    }

    /**
     * Get messages with pagination and filtering (metadata only for performance)
     */
    async getMessages(req, res) {
        try {
            await this.ensureDatabase();

            const {
                interfaceId,
                status,
                messageType,
                dateFrom,
                dateTo,
                page = 1,
                limit = 50,
                sortBy = 'received_at',
                sortOrder = 'DESC'
            } = req.query;

            const userId = req.session.user.id;
            const offset = (page - 1) * limit;

            // Build WHERE clause
            let whereConditions = ['i.user_id = :userId'];
            let replacements = { userId, limit: parseInt(limit), offset: parseInt(offset) };

            if (interfaceId) {
                whereConditions.push('mpe.interface_id = :interfaceId');
                replacements.interfaceId = interfaceId;
            }

            if (status) {
                // Map composite display status values to the two DB columns
                const statusMap = {
                    'received':         "mpe.status = 'received'",
                    'processing':       "mpe.status IN ('processing','reprocessing')",
                    'delivered':        "mpe.status IN ('processed','delivered') AND mpe.delivery_status = 'delivered'",
                    'completed':        "mpe.status IN ('processed','delivered') AND mpe.delivery_status = 'not_required'",
                    'pending_delivery': "mpe.status = 'processed' AND mpe.delivery_status = 'pending'",
                    'failed':           "(mpe.status = 'failed' OR mpe.delivery_status = 'failed')",
                };
                if (statusMap[status]) {
                    whereConditions.push(statusMap[status]);
                } else {
                    // Fallback for any raw status value passed directly
                    whereConditions.push('mpe.status = :status');
                    replacements.status = status;
                }
            }

            if (messageType) {
                whereConditions.push('mpe.message_type ILIKE :messageType');
                replacements.messageType = `%${messageType}%`;
            }

            if (dateFrom) {
                whereConditions.push('mpe.received_at >= :dateFrom');
                replacements.dateFrom = dateFrom;
            }

            if (dateTo) {
                whereConditions.push('mpe.received_at <= :dateTo');
                replacements.dateTo = dateTo;
            }

            // Valid sort columns
            const validSortColumns = ['received_at', 'message_id', 'status', 'message_type', 'processing_time_ms'];
            const finalSortBy = validSortColumns.includes(sortBy) ? sortBy : 'received_at';
            const finalSortOrder = ['ASC', 'DESC'].includes(sortOrder.toUpperCase()) ? sortOrder.toUpperCase() : 'DESC';

            // Main query for message metadata
            const query = `
                SELECT
                    mpe.id,
                    mpe.interface_id,
                    i.name as interface_name,
                    mpe.message_id,
                    mpe.correlation_id,
                    mpe.status,
                    mpe.message_type,
                    mpe.message_size,
                    mpe.source_type,
                    mpe.source_endpoint,
                    mpe.source_ip,
                    mpe.received_at,
                    mpe.processing_started_at,
                    mpe.processing_completed_at,
                    mpe.processing_time_ms,
                    mpe.transformation_applied,
                    mpe.error_count,
                    mpe.retry_count,
                    mpe.last_error_message,
                    mpe.delivery_status,
                    mpe.delivery_attempts,
                    mpe.is_large_message,
                    -- Calculate total processing time
                    CASE
                        WHEN mpe.processing_completed_at IS NOT NULL
                        THEN EXTRACT(EPOCH FROM (mpe.processing_completed_at - mpe.received_at)) * 1000
                        ELSE NULL
                    END as total_processing_time_ms,
                    -- Check if content exists
                    (SELECT COUNT(*) FROM message_content_store mcs
                     WHERE mcs.message_processing_id = mpe.id) as content_count
                FROM message_processing_enhanced mpe
                JOIN interfaces i ON mpe.interface_id = i.id
                WHERE ${whereConditions.join(' AND ')}
                ORDER BY mpe.${finalSortBy} ${finalSortOrder}
                LIMIT :limit OFFSET :offset
            `;

            const messages = await this.database.sequelize.query(query, {
                replacements,
                type: this.database.sequelize.QueryTypes.SELECT
            });

            // Count query for pagination
            const countQuery = `
                SELECT COUNT(*) as total
                FROM message_processing_enhanced mpe
                JOIN interfaces i ON mpe.interface_id = i.id
                WHERE ${whereConditions.join(' AND ')}
            `;

            const countResult = await this.database.sequelize.query(countQuery, {
                replacements: { userId, interfaceId, status, messageType, dateFrom, dateTo },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            const total = parseInt(countResult[0].total);
            const totalPages = Math.ceil(total / limit);

            res.json({
                success: true,
                data: {
                    messages,
                    pagination: {
                        currentPage: parseInt(page),
                        totalPages,
                        totalCount: total,
                        pageSize: parseInt(limit),
                        hasNextPage: page < totalPages,
                        hasPreviousPage: page > 1
                    },
                    filters: {
                        interfaceId,
                        status,
                        messageType,
                        dateFrom,
                        dateTo
                    }
                }
            });

        } catch (error) {
            console.error('❌ Get Messages Error:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to retrieve messages',
                debug: error.message
            });
        }
    }

    /**
     * Get detailed message content by ID
     */
    async getMessageDetail(req, res) {
        try {
            await this.ensureDatabase();

            const { messageId } = req.params;
            const { interfaceId } = req.query;
            const userId = req.session.user.id;

            console.log(`🔍 getMessageDetail: Looking for message ${messageId} for user ${userId}`);

            let messageResult = null;
            let interfaceName = null;

            if (interfaceId) {
                // Fast path: caller told us exactly which interface table to look in
                const ifaceRows = await this.database.sequelize.query(
                    `SELECT id, name FROM interfaces WHERE id = :interfaceId AND user_id = :userId AND is_active = true`,
                    { replacements: { interfaceId, userId }, type: this.database.sequelize.QueryTypes.SELECT }
                );
                if (ifaceRows.length > 0) {
                    const tableName = this.tableManager.getInterfaceTableName(interfaceId);
                    try {
                        const result = await this.database.sequelize.query(
                            `SELECT * FROM ${tableName} WHERE message_id = :messageId OR id::text = :messageId`,
                            { replacements: { messageId }, type: this.database.sequelize.QueryTypes.SELECT }
                        );
                        if (result.length > 0) {
                            messageResult = result;
                            interfaceName = ifaceRows[0].name;
                        }
                    } catch (tableError) {
                        // Table doesn't exist yet; fall through to 404
                    }
                }
            } else {
                // Fallback: scan all user interfaces (no status filter — message could be in any table)
                const interfaces = await this.database.sequelize.query(
                    `SELECT id, name FROM interfaces WHERE user_id = :userId AND is_active = true`,
                    { replacements: { userId }, type: this.database.sequelize.QueryTypes.SELECT }
                );

                console.log(`🔍 Found ${interfaces.length} interfaces for user`);

                for (const iface of interfaces) {
                    const tableName = this.tableManager.getInterfaceTableName(iface.id);
                    try {
                        const result = await this.database.sequelize.query(
                            `SELECT * FROM ${tableName} WHERE message_id = :messageId OR id::text = :messageId`,
                            { replacements: { messageId }, type: this.database.sequelize.QueryTypes.SELECT }
                        );
                        if (result.length > 0) {
                            messageResult = result;
                            interfaceName = iface.name;
                            break;
                        }
                    } catch (tableError) {
                        continue;
                    }
                }
            }

            if (!messageResult || messageResult.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Message not found'
                });
            }

            const message = messageResult[0];

            // Return message details in format expected by frontend
            res.json({
                success: true,
                data: {
                    message: {
                        ...message,
                        interface_name: interfaceName
                    },
                    content: [], // No separate content store in interface-specific tables
                    transformations: [] // No separate transformations in interface-specific tables
                }
            });

        } catch (error) {
            console.error('❌ Get Message Detail Error:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to retrieve message details',
                debug: error.message
            });
        }
    }

    /**
     * Send a test message to an interface
     */
    async sendMessage(req, res) {
        try {
            await this.ensureDatabase();

            const { interfaceId } = req.params;
            const { messageContent, messageType, contentType = 'application/json' } = req.body;
            const userId = req.session.user.id;

            // Verify interface exists and user has access
            const interfaceQuery = `
                SELECT id, name, source_type, source_config
                FROM interfaces
                WHERE id = :interfaceId AND user_id = :userId
            `;

            const interfaceResult = await this.database.sequelize.query(interfaceQuery, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (interfaceResult.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found'
                });
            }

            const interfaceData = interfaceResult[0];
            let sourceConfig;
            try {
                sourceConfig = typeof interfaceData.source_config === 'string'
                    ? JSON.parse(interfaceData.source_config)
                    : interfaceData.source_config;
            } catch (parseError) {
                console.error('Failed to parse source_config:', parseError.message);
                return res.status(400).json({
                    success: false,
                    error: 'Invalid interface configuration'
                });
            }

            // Generate message ID for tracking
            const messageId = `UI-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

            console.log(`🚀 Sending message via ${interfaceData.source_type} connectivity for interface ${interfaceId}`);

            // Send message using actual source connectivity (NO DIRECT DB INSERTION)
            let deliveryResult;
            try {
                if (interfaceData.source_type === 'tcp' || interfaceData.source_type === 'mllp') {
                    // Send via TCP/MLLP connectivity
                    // Force IPv4 for localhost to match Go listener (0.0.0.0)
                    let host = sourceConfig.host || 'localhost';
                    if (host === 'localhost') {
                        host = '127.0.0.1';  // Force IPv4 instead of IPv6
                    }
                    const port = sourceConfig.port || 6661;

                    const net = require('net');
                    const client = new net.Socket();

                    const sendPromise = new Promise((resolve, reject) => {
                        let responseData = '';

                        client.connect(port, host, () => {
                            console.log(`📡 Connected to ${host}:${port}`);

                            // TCP interfaces expect MLLP framing (HL7 protocol standard)
                            // MLLP format: <VT>message<FS><CR> where VT=0x0B, FS=0x1C, CR=0x0D
                            const messageToSend = `\x0B${messageContent}\x1C\x0D`;

                            console.log(`📤 Sending MLLP-wrapped message (${messageContent.length} bytes + framing)`);
                            client.write(messageToSend);
                        });

                        client.on('data', (data) => {
                            responseData += data.toString();
                            console.log(`📥 Received ACK response (${data.length} bytes)`);

                            // Close connection after receiving ACK (don't wait for server to close)
                            client.end();
                        });

                        client.on('close', () => {
                            resolve({
                                success: true,
                                status: 200,
                                response: responseData || 'Message sent via TCP/MLLP connectivity',
                                method: 'tcp_connectivity'
                            });
                        });

                        client.on('error', (err) => {
                            reject({
                                success: false,
                                status: 500,
                                response: err.message,
                                method: 'tcp_connectivity'
                            });
                        });

                        // Timeout after 30 seconds (allow time for HL7→FHIR transformation)
                        setTimeout(() => {
                            client.destroy();
                            reject({
                                success: false,
                                status: 408,
                                response: 'Connection timeout - no ACK received within 30 seconds',
                                method: 'tcp_connectivity'
                            });
                        }, 30000);
                    });

                    deliveryResult = await sendPromise;

                } else if (interfaceData.source_type === 'fhir' || interfaceData.source_type === 'http') {
                    // Send via HTTP connectivity
                    // Force IPv4 for localhost to match Go listener (0.0.0.0)
                    let host = sourceConfig.host || 'localhost';
                    if (host === 'localhost') {
                        host = '127.0.0.1';  // Force IPv4 instead of IPv6
                    }
                    const port = sourceConfig.port || 8090;
                    const path = sourceConfig.path || '/fhir';

                    const fetch = require('node-fetch');
                    const response = await fetch(`http://${host}:${port}${path}`, {
                        method: 'POST',
                        headers: {
                            'Content-Type': contentType,
                            'X-Message-ID': messageId,
                            'X-Interface-ID': interfaceId
                        },
                        body: messageContent
                    });

                    deliveryResult = {
                        success: response.ok,
                        status: response.status,
                        response: await response.text(),
                        method: 'http_connectivity'
                    };

                } else {
                    // Fallback: inject directly into the processing engine pipeline.
                    // Handles file_listener, postgresql_inbound, sftp_inbound, ccda, and any
                    // other connector type that has no live socket for test messages.
                    // Must go through _goClient (services/goBackendClient.js) so the
                    // X-Internal-Proxy-Secret header is attached — the Go backend's
                    // requireProxiedRequest() middleware rejects unauthenticated direct
                    // calls to /api/* with "direct access to internal API not permitted".
                    const _proxySecret = process.env.INTERNAL_PROXY_SECRET || process.env.JWT_SECRET || '';
                    let injectData;
                    try {
                        const injectResp = await _goClient.post(
                            `/api/processing/interfaces/${interfaceId}/inject`,
                            {
                                content: messageContent,
                                messageType: messageType || 'unknown',
                                contentType: contentType || 'application/xml'
                            },
                            {
                                headers: _proxySecret ? { 'X-Internal-Proxy-Secret': _proxySecret } : {},
                                timeout: 15000
                            }
                        );
                        injectData = injectResp.data;
                    } catch (axiosErr) {
                        throw {
                            success: false,
                            status: axiosErr.response?.status || 500,
                            response: axiosErr.response?.data?.error || axiosErr.message || 'Inject failed',
                            method: 'pipeline_inject'
                        };
                    }

                    if (!injectData || !injectData.success) {
                        throw {
                            success: false,
                            status: 500,
                            response: (injectData && injectData.error) || 'Inject failed',
                            method: 'pipeline_inject'
                        };
                    }
                    deliveryResult = {
                        success: true,
                        status: 200,
                        response: `Message injected into pipeline (ID: ${injectData.messageId})`,
                        method: 'pipeline_inject'
                    };
                }

                console.log(`📨 Message delivery result:`, deliveryResult);

                res.json({
                    success: deliveryResult.success,
                    messageId: messageId,
                    interface: {
                        id: interfaceId,
                        name: interfaceData.name,
                        type: interfaceData.source_type,
                        endpoint: `${sourceConfig.host || 'localhost'}:${sourceConfig.port}`
                    },
                    delivery: {
                        status: deliveryResult.status,
                        method: deliveryResult.method,
                        response: deliveryResult.response
                    },
                    message: deliveryResult.success
                        ? `Message sent successfully via ${interfaceData.source_type} connectivity`
                        : `Message delivery failed: ${deliveryResult.response}`
                });

            } catch (connectivityError) {
                console.error(`❌ Connectivity Error (${interfaceData.source_type}):`, connectivityError);

                res.status(500).json({
                    success: false,
                    messageId: messageId,
                    interface: {
                        id: interfaceId,
                        name: interfaceData.name,
                        type: interfaceData.source_type,
                        endpoint: `${sourceConfig.host || 'localhost'}:${sourceConfig.port}`
                    },
                    error: 'Failed to send message via source connectivity',
                    details: connectivityError.response || connectivityError.message || connectivityError,
                    message: `Unable to connect to ${interfaceData.source_type} endpoint. Ensure the interface listener is running.`
                });
            }

        } catch (error) {
            console.error('❌ Send Message Error:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to send message',
                debug: error.message
            });
        }
    }

    /**
     * Reprocess a failed message
     */
    async reprocessMessage(req, res) {
        try {
            await this.ensureDatabase();

            const { messageId } = req.params;
            const userId = req.session.user.id;

            console.log(`🔍 Reprocess request: messageId=${messageId}, userId=${userId}`);

            // First, find which interface table the message is in by querying interface_table_metadata
            const metadataQuery = `
                SELECT itm.table_name, itm.interface_id, i.name as interface_name
                FROM interface_table_metadata itm
                JOIN interfaces i ON itm.interface_id = i.id
                WHERE i.user_id = $1
            `;

            const metadataResult = await this.database.sequelize.query(metadataQuery, {
                bind: [userId],
                type: this.database.sequelize.QueryTypes.SELECT
            });

            console.log(`🔍 Found ${metadataResult.length} interface tables for user ${userId}:`, metadataResult.map(m => m.table_name));

            // Search for the message across all interface tables
            let message = null;
            let tableName = null;
            let interfaceId = null;
            let interfaceName = null;

            for (const metadata of metadataResult) {
                try {
                    console.log(`🔍 Checking table ${metadata.table_name} for message ${messageId}`);

                    const checkQuery = `SELECT * FROM ${metadata.table_name} WHERE message_id = :messageId OR id::text = :messageId`;
                    const result = await this.database.sequelize.query(checkQuery, {
                        replacements: { messageId },
                        type: this.database.sequelize.QueryTypes.SELECT,
                        logging: (sql, timing) => {
                            console.log(`📝 Executing SQL: ${sql}`);
                            console.log(`📝 With params: ${messageId}`);
                        }
                    });

                    console.log(`📊 Query returned ${result.length} rows from ${metadata.table_name}`);

                    if (result.length > 0) {
                        console.log(`✅ Found message in table ${metadata.table_name}`);
                        message = result[0];
                        tableName = metadata.table_name;
                        interfaceId = metadata.interface_id;
                        interfaceName = metadata.interface_name;
                        break;
                    } else {
                        console.log(`❌ Message not found in table ${metadata.table_name}`);
                    }
                } catch (err) {
                    console.error(`⚠️ Error checking table ${metadata.table_name}:`, err.message);
                    console.error(`⚠️ Full error:`, err);
                    // Table might not exist, continue to next
                    continue;
                }
            }

            if (!message) {
                console.error(`❌ Message ${messageId} not found in any interface table`);
                return res.status(404).json({
                    success: false,
                    error: 'Message not found or access denied'
                });
            }

            // Mark as reprocessing and clear the last error.
            // Do NOT touch delivery_status or delivery_attempts here — the Go pipeline
            // owns those: OutboundConnectorExecutor sets 'delivered'/'failed', and
            // the engine resets to 'pending' so the 'not_required' flip works correctly.
            const updateQuery = `
                UPDATE ${tableName}
                SET status = 'reprocessing',
                    delivery_status = 'pending',
                    last_error_message = NULL,
                    updated_at = CURRENT_TIMESTAMP
                WHERE message_id = :messageId OR id::text = :messageId
            `;

            await this.database.sequelize.query(updateQuery, {
                replacements: { messageId },
                type: this.database.sequelize.QueryTypes.UPDATE
            });

            // Trigger reprocessing via Go backend: reads raw content from object storage,
            // re-parses, and re-runs the transformation pipeline on the existing DB row.
            try {
                await _goClient.post(
                    `/api/processing/interfaces/${interfaceId}/reprocess/${messageId}`
                );
            } catch (goError) {
                console.error('❌ Go reprocess call failed:', goError.message);
                // Roll back status so the message isn't stuck as 'reprocessing'
                await this.database.sequelize.query(
                    `UPDATE ${tableName} SET status = 'failed',
                     last_error_message = 'Reprocess trigger failed: engine unavailable'
                     WHERE message_id = :messageId OR id::text = :messageId`,
                    { replacements: { messageId }, type: this.database.sequelize.QueryTypes.UPDATE }
                );
                return res.status(503).json({
                    success: false,
                    error: 'Processing engine unavailable — ensure the interface is activated'
                });
            }

            res.json({
                success: true,
                message: 'Message queued for reprocessing',
                data: {
                    messageId: messageId,
                    interfaceId: interfaceId,
                    interfaceName: interfaceName
                }
            });

        } catch (error) {
            console.error('❌ Reprocess Message Error:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to reprocess message',
                debug: error.message
            });
        }
    }

    /**
     * Delete a message
     */
    async deleteMessage(req, res) {
        try {
            await this.ensureDatabase();

            const { messageId } = req.params;
            const userId = req.session.user.id;

            // Verify message exists and user has access
            const messageQuery = `
                SELECT mpe.id
                FROM message_processing_enhanced mpe
                JOIN interfaces i ON mpe.interface_id = i.id
                WHERE mpe.id = :messageId AND i.user_id = :userId
            `;

            const messageResult = await this.database.sequelize.query(messageQuery, {
                replacements: { messageId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (messageResult.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Message not found'
                });
            }

            // Delete message (cascades will handle related records)
            await this.database.sequelize.query(`
                DELETE FROM message_processing_enhanced
                WHERE id = :messageId
            `, {
                replacements: { messageId },
                type: this.database.sequelize.QueryTypes.DELETE
            });

            res.json({
                success: true,
                message: 'Message deleted successfully'
            });

        } catch (error) {
            console.error('❌ Delete Message Error:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to delete message',
                debug: error.message
            });
        }
    }

    /**
     * Get message statistics for dashboard
     */
    async getMessageStats(req, res) {
        try {
            await this.ensureDatabase();

            const { interfaceId, timeRange = '24h' } = req.query;
            const userId = req.session.user.id;

            let timeCondition = '';
            if (timeRange === '1h') {
                timeCondition = "AND mpe.received_at >= NOW() - INTERVAL '1 hour'";
            } else if (timeRange === '24h') {
                timeCondition = "AND mpe.received_at >= NOW() - INTERVAL '24 hours'";
            } else if (timeRange === '7d') {
                timeCondition = "AND mpe.received_at >= NOW() - INTERVAL '7 days'";
            }

            let interfaceCondition = '';
            let replacements = { userId };
            if (interfaceId) {
                interfaceCondition = 'AND mpe.interface_id = :interfaceId';
                replacements.interfaceId = interfaceId;
            }

            const statsQuery = `
                SELECT
                    COUNT(*) as total_messages,
                    COUNT(CASE WHEN mpe.status = 'sent' THEN 1 END) as successful_messages,
                    COUNT(CASE WHEN mpe.status IN ('failed', 'error') THEN 1 END) as failed_messages,
                    COUNT(CASE WHEN mpe.status = 'processing' THEN 1 END) as processing_messages,
                    AVG(mpe.processing_time_ms) as avg_processing_time,
                    MAX(mpe.processing_time_ms) as max_processing_time,
                    SUM(mpe.message_size) as total_bytes_processed
                FROM message_processing_enhanced mpe
                JOIN interfaces i ON mpe.interface_id = i.id
                WHERE i.user_id = :userId ${timeCondition} ${interfaceCondition}
            `;

            const stats = await this.database.sequelize.query(statsQuery, {
                replacements,
                type: this.database.sequelize.QueryTypes.SELECT
            });

            res.json({
                success: true,
                data: stats[0]
            });

        } catch (error) {
            console.error('❌ Get Message Stats Error:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to retrieve message statistics',
                debug: error.message
            });
        }
    }

    /**
     * INTERFACE-SPECIFIC METHODS (NEW - Performance Optimized)
     */

    /**
     * Get messages for a specific interface only (Performance optimized)
     */
    async getInterfaceMessages(req, res) {
        try {
            await this.ensureDatabase();

            const { interfaceId } = req.params;
            const {
                status,
                messageType,
                dateFrom,
                dateTo,
                page = 1,
                limit = 50,
                sortBy = 'received_at',
                sortOrder = 'DESC'
            } = req.query;

            const userId = req.session.user.id;

            // Verify user owns this interface. coverage_audit_enabled gates whether
            // the has_coverage_gap column below gets computed at all — keeps the
            // "zero cost unless opted in" principle CDA Coverage Audit already
            // commits to (V207 migration) intact for the messages list too.
            // Disabled means NULL *or* the JSON literal null — the latter is what
            // interfacesController.js's updateInterface now writes when a user
            // explicitly unchecks the feature in the Edit modal (see the
            // COALESCE-preserve-if-absent fix there); a plain IS NOT NULL check
            // would treat that as still enabled.
            const interfaceCheck = await this.database.sequelize.query(`
                SELECT id, name, status,
                    (cda_coverage_audit_config IS NOT NULL AND cda_coverage_audit_config <> 'null'::jsonb) AS coverage_audit_enabled
                FROM interfaces
                WHERE id = :interfaceId AND user_id = :userId
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (interfaceCheck.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found or access denied'
                });
            }

            const interfaceInfo = interfaceCheck[0];
            let result;

            // INTERFACE-SPECIFIC: Try dedicated table first, fallback to shared table
            console.log(`🎯 Checking for dedicated table for interface ${interfaceInfo.name}`);

            result = await this.tableManager.getInterfaceMessages(interfaceId, {
                page,
                limit,
                status,
                messageType,
                dateFrom,
                dateTo,
                sortBy,
                sortOrder,
                coverageAuditEnabled: interfaceInfo.coverage_audit_enabled
            });

            // Check if dedicated table exists and has data, or if we need to fallback
            if (result.performance?.tableExists === false ||
                (result.pagination?.totalCount === 0 && !result.performance?.tableExists)) {

                console.log(`⚠️ No dedicated table found, using shared table for interface ${interfaceInfo.name}`);

                // FALLBACK: Use shared table (message_processing_enhanced)
                result = await this.getInterfaceMessagesFromSharedTable(interfaceId, {
                    page,
                    limit,
                    status,
                    messageType,
                    dateFrom,
                    dateTo,
                    sortBy,
                    sortOrder,
                    interfaceName: interfaceInfo.name,
                    coverageAuditEnabled: interfaceInfo.coverage_audit_enabled
                });

                result.interfaceInfo = {
                    id: interfaceId,
                    name: interfaceInfo.name,
                    tableStrategy: 'shared',
                    tableName: 'message_processing_enhanced'
                };
            } else {
                // Add interface info to result for dedicated table
                result.interfaceInfo = {
                    id: interfaceId,
                    name: interfaceInfo.name,
                    tableStrategy: 'dedicated',
                    tableName: result.tableName
                };

                // IMPORTANT: Add interface_name to each message (dedicated tables don't have this column)
                if (result.messages && Array.isArray(result.messages)) {
                    result.messages = result.messages.map(msg => ({
                        ...msg,
                        interface_name: interfaceInfo.name
                    }));
                }
            }

            res.json({
                success: true,
                data: result
            });

        } catch (error) {
            console.error('Failed to get interface messages:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to load interface messages'
            });
        }
    }

    /**
     * LEGACY: Get messages from shared table (for backwards compatibility)
     */
    async getInterfaceMessagesFromSharedTable(interfaceId, options) {
        const {
            page,
            limit,
            status,
            messageType,
            dateFrom,
            dateTo,
            sortBy,
            sortOrder,
            interfaceName,
            coverageAuditEnabled
        } = options;

        const offset = (page - 1) * limit;
        let whereConditions = ['mpe.interface_id = :interfaceId'];
        let replacements = { interfaceId, limit: parseInt(limit), offset: parseInt(offset) };

        if (status) {
            const statusMap = {
                'received':         "mpe.status = 'received'",
                'processing':       "mpe.status IN ('processing','reprocessing')",
                'delivered':        "mpe.status IN ('processed','delivered') AND mpe.delivery_status = 'delivered'",
                'completed':        "mpe.status IN ('processed','delivered') AND mpe.delivery_status = 'not_required'",
                'pending_delivery': "mpe.status = 'processed' AND mpe.delivery_status = 'pending'",
                'failed':           "(mpe.status = 'failed' OR mpe.delivery_status = 'failed')",
            };
            if (statusMap[status]) {
                whereConditions.push(statusMap[status]);
            } else {
                whereConditions.push('mpe.status = :status');
                replacements.status = status;
            }
        }

        if (messageType) {
            whereConditions.push('mpe.message_type ILIKE :messageType');
            replacements.messageType = `%${messageType}%`;
        }

        if (dateFrom) {
            whereConditions.push('mpe.received_at >= :dateFrom');
            replacements.dateFrom = dateFrom;
        }

        if (dateTo) {
            whereConditions.push('mpe.received_at <= :dateTo');
            replacements.dateTo = dateTo;
        }

        const validSortColumns = ['received_at', 'message_id', 'status', 'message_type', 'processing_time_ms'];
        const finalSortBy = validSortColumns.includes(sortBy) ? sortBy : 'received_at';
        const finalSortOrder = ['ASC', 'DESC'].includes(sortOrder.toUpperCase()) ? sortOrder.toUpperCase() : 'DESC';

        // Shared table query
        const query = `
            SELECT
                mpe.id,
                mpe.interface_id,
                i.name as interface_name,
                mpe.message_id,
                mpe.correlation_id,
                mpe.status,
                mpe.message_type,
                mpe.message_size,
                mpe.source_type,
                mpe.source_endpoint,
                mpe.received_at,
                mpe.processing_completed_at,
                mpe.processing_time_ms,
                mpe.error_count,
                mpe.last_error_message,
                mpe.delivery_status,
                mpe.delivery_attempts
                ${coverageAuditEnabled ? ', ' + hasCoverageGapSql('mpe.message_id') + ' AS has_coverage_gap' : ', false AS has_coverage_gap'}
            FROM message_processing_enhanced mpe
            INNER JOIN interfaces i ON mpe.interface_id = i.id
            WHERE ${whereConditions.join(' AND ')}
            ORDER BY mpe.${finalSortBy} ${finalSortOrder}
            LIMIT :limit OFFSET :offset
        `;

        const countQuery = `
            SELECT COUNT(*) as total
            FROM message_processing_enhanced mpe
            WHERE ${whereConditions.join(' AND ')}
        `;

        const [messages, countResult] = await Promise.all([
            this.database.sequelize.query(query, {
                replacements,
                type: this.database.sequelize.QueryTypes.SELECT
            }),
            this.database.sequelize.query(countQuery, {
                replacements,
                type: this.database.sequelize.QueryTypes.SELECT
            })
        ]);

        const totalCount = parseInt(countResult[0].total);
        const totalPages = Math.ceil(totalCount / limit);

        return {
            messages,
            pagination: {
                currentPage: parseInt(page),
                totalPages,
                totalCount,
                hasNextPage: page < totalPages,
                hasPreviousPage: page > 1
            },
            interfaceInfo: {
                id: interfaceId,
                name: interfaceName,
                tableStrategy: 'shared',
                tableName: 'message_processing_enhanced'
            },
            performance: {
                queryTime: new Date().toISOString(),
                tableRowsQueried: totalCount,
                isolatedTable: false
            }
        };
    }

    /**
     * Get statistics for a specific interface
     */
    async getInterfaceStats(req, res) {
        try {
            await this.ensureDatabase();

            const { interfaceId } = req.params;
            const { timeRange = '24h' } = req.query;
            const userId = req.session.user.id;

            // Verify user owns this interface
            const interfaceCheck = await this.database.sequelize.query(`
                SELECT id, name FROM interfaces WHERE id = :interfaceId AND user_id = :userId
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (interfaceCheck.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found or access denied'
                });
            }

            // Get stats from interface-specific table
            const stats = await this.tableManager.getInterfaceTableStats(interfaceId);

            // If no stats (table doesn't exist), return zeros
            if (!stats) {
                return res.json({
                    success: true,
                    data: {
                        interface: interfaceCheck[0],
                        timeRange,
                        stats: {
                            total_messages: 0,
                            successful_messages: 0,
                            failed_messages: 0,
                            processing_messages: 0,
                            avg_processing_time: 0,
                            last_message_at: null,
                            unique_message_types: 0
                        }
                    }
                });
            }

            // Format stats for frontend
            const formattedStats = {
                total_messages: parseInt(stats.total_messages) || 0,
                successful_messages: parseInt(stats.successful_messages) || 0,
                failed_messages: parseInt(stats.failed_messages) || 0,
                processing_messages: parseInt(stats.processing_messages) || 0,
                avg_processing_time: parseFloat(stats.avg_processing_time) || 0,
                last_message_at: stats.last_message_at,
                unique_message_types: parseInt(stats.unique_message_types) || 0
            };

            res.json({
                success: true,
                data: {
                    interface: interfaceCheck[0],
                    timeRange,
                    stats: formattedStats
                }
            });

        } catch (error) {
            console.error('Failed to get interface stats:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to load interface statistics'
            });
        }
    }

    /**
     * Get message flow status between interfaces
     */
    async getFlowStatus(req, res) {
        try {
            await this.ensureDatabase();

            const { sourceInterfaceId, targetInterfaceId } = req.params;
            const { timeRange = '24h' } = req.query;
            const userId = req.session.user.id;

            // Verify interfaces belong to user
            const interfacesCheck = await this.database.sequelize.query(`
                SELECT id, name FROM interfaces
                WHERE id IN (:sourceInterfaceId, :targetInterfaceId) AND user_id = :userId
            `, {
                replacements: { sourceInterfaceId, targetInterfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (interfacesCheck.length !== 2) {
                return res.status(404).json({
                    success: false,
                    error: 'Interfaces not found or access denied'
                });
            }

            // Time filter
            let timeFilter = '';
            switch (timeRange) {
                case '1h':
                    timeFilter = "AND mpq.created_at >= NOW() - INTERVAL '1 hour'";
                    break;
                case '24h':
                    timeFilter = "AND mpq.created_at >= NOW() - INTERVAL '24 hours'";
                    break;
                case '7d':
                    timeFilter = "AND mpq.created_at >= NOW() - INTERVAL '7 days'";
                    break;
                default:
                    timeFilter = "AND mpq.created_at >= NOW() - INTERVAL '24 hours'";
            }

            // Get flow statistics
            const flowStatsQuery = `
                SELECT
                    COUNT(*) as total_flows,
                    COUNT(CASE WHEN mpq.status = 'completed' THEN 1 END) as successful_flows,
                    COUNT(CASE WHEN mpq.status = 'failed' THEN 1 END) as failed_flows,
                    COUNT(CASE WHEN mpq.status = 'pending' THEN 1 END) as pending_flows,
                    AVG(EXTRACT(EPOCH FROM (mpq.completed_at - mpq.created_at)) * 1000) as avg_processing_time_ms,
                    MAX(mpq.created_at) as last_flow_at
                FROM message_processing_queue mpq
                WHERE mpq.interface_id = :sourceInterfaceId
                AND mpq.payload::jsonb->'target_interface'->>'id' = :targetInterfaceId
                AND mpq.action_type = 'hl7_to_fhir_transform'
                ${timeFilter}
            `;

            const flowStats = await this.database.sequelize.query(flowStatsQuery, {
                replacements: { sourceInterfaceId, targetInterfaceId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            // Get recent flows
            const recentFlowsQuery = `
                SELECT
                    mpq.id,
                    mpq.message_id,
                    mpq.correlation_id,
                    mpq.status,
                    mpq.priority,
                    mpq.created_at,
                    mpq.completed_at,
                    mpq.error_message
                FROM message_processing_queue mpq
                WHERE mpq.interface_id = :sourceInterfaceId
                AND mpq.payload::jsonb->'target_interface'->>'id' = :targetInterfaceId
                AND mpq.action_type = 'hl7_to_fhir_transform'
                ${timeFilter}
                ORDER BY mpq.created_at DESC
                LIMIT 20
            `;

            const recentFlows = await this.database.sequelize.query(recentFlowsQuery, {
                replacements: { sourceInterfaceId, targetInterfaceId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            res.json({
                success: true,
                data: {
                    interfaces: {
                        source: interfacesCheck.find(i => i.id === sourceInterfaceId),
                        target: interfacesCheck.find(i => i.id === targetInterfaceId)
                    },
                    timeRange,
                    flowStats: flowStats[0],
                    recentFlows
                }
            });

        } catch (error) {
            console.error('Failed to get flow status:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to load flow status'
            });
        }
    }

    /**
     * Receive and store FHIR messages in the FHIR Receiver interface
     * POST /api/messages/fhir/:resourceType
     */
    async receiveFHIRMessage(req, res) {
        try {
            const resourceType = req.params.resourceType || 'Patient';
            const fhirMessage = req.body;

            console.log(`🏥 Receiving FHIR ${resourceType} message...`);

            // Find FHIR Receiver interface
            await this.ensureDatabase();
            const fhirInterface = await this.database.sequelize.query(`
                SELECT id, name, target_config, processing_rules
                FROM interfaces
                WHERE message_type = 'FHIR'
                AND source_type = 'http'
                AND target_type = 'database'
                AND status = 'active'
                LIMIT 1
            `, { type: this.database.sequelize.QueryTypes.SELECT });

            if (!fhirInterface || fhirInterface.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'No active FHIR receiver interface found'
                });
            }

            const interfaceConfig = fhirInterface[0];
            const targetConfig = JSON.parse(interfaceConfig.target_config || '{}');
            const processingRules = JSON.parse(interfaceConfig.processing_rules || '{}');

            // Generate message ID and correlation ID
            const messageId = `fhir_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
            const correlationId = req.headers['x-correlation-id'] || messageId;

            // Extract FHIR resource information
            let fhirResourceType = resourceType;
            let fhirResourceId = null;

            if (fhirMessage && typeof fhirMessage === 'object') {
                fhirResourceType = fhirMessage.resourceType || resourceType;
                fhirResourceId = fhirMessage.id || null;
            }

            // Validate FHIR message if enabled
            let validationStatus = 'valid';
            let validationError = null;

            if (processingRules.messageValidation) {
                try {
                    // Basic FHIR validation
                    if (!fhirMessage || !fhirMessage.resourceType) {
                        validationStatus = 'invalid';
                        validationError = 'Missing resourceType in FHIR message';
                    } else if (fhirMessage.resourceType !== resourceType) {
                        validationStatus = 'warning';
                        validationError = `Resource type mismatch: expected ${resourceType}, got ${fhirMessage.resourceType}`;
                    }
                } catch (error) {
                    validationStatus = 'invalid';
                    validationError = error.message;
                }
            }

            // Store message in interface-specific table
            const tableName = targetConfig.tableName || `messages_intf_${interfaceConfig.id.replace(/-/g, '_').substring(0, 16)}`;

            const startTime = Date.now();

            const insertResult = await this.database.sequelize.query(`
                INSERT INTO ${tableName} (
                    interface_id, message_id, correlation_id, status, source,
                    message_type, content_type, raw_message, processed_message,
                    fhir_resource_type, fhir_resource_id, error_details,
                    processing_duration_ms, created_at, processed_at
                ) VALUES (
                    :interfaceId, :messageId, :correlationId, :status, :source,
                    :messageType, :contentType, :rawMessage, :processedMessage,
                    :fhirResourceType, :fhirResourceId, :errorDetails,
                    :processingDuration, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
                )
            `, {
                replacements: {
                    interfaceId: interfaceConfig.id,
                    messageId,
                    correlationId,
                    status: validationStatus === 'invalid' ? 'failed' : 'processed',
                    source: req.headers['x-source-interface'] || 'external',
                    messageType: 'FHIR',
                    contentType: req.headers['content-type'] || 'application/fhir+json',
                    rawMessage: JSON.stringify(fhirMessage),
                    processedMessage: JSON.stringify(fhirMessage),
                    fhirResourceType,
                    fhirResourceId,
                    errorDetails: validationError,
                    processingDuration: Date.now() - startTime
                }
            });

            console.log(`✅ FHIR ${resourceType} message stored successfully`);
            console.log(`   Message ID: ${messageId}`);
            console.log(`   Resource: ${fhirResourceType}${fhirResourceId ? `/${fhirResourceId}` : ''}`);
            console.log(`   Status: ${validationStatus}`);
            console.log(`   Table: ${tableName}`);

            // Audit logging
            if (processingRules.auditLogging) {
                try {
                    await database.sequelize.query(`
                        INSERT INTO audit_logs (user_id, action, resource_type, resource_id, details, ip_address, user_agent, created_at)
                        VALUES (:userId, :action, :resourceType, :resourceId, :details, :ipAddress, :userAgent, CURRENT_TIMESTAMP)
                    `, {
                        replacements: {
                            userId: 'system',
                            action: 'fhir_message_received',
                            resourceType: 'message',
                            resourceId: messageId,
                            details: JSON.stringify({
                                interfaceId: interfaceConfig.id,
                                interfaceName: interfaceConfig.name,
                                fhirResourceType,
                                fhirResourceId,
                                validationStatus,
                                correlationId
                            }),
                            ipAddress: req.ip || req.connection.remoteAddress,
                            userAgent: req.headers['user-agent'] || 'Unknown'
                        }
                    });
                } catch (auditError) {
                    console.warn('⚠️ Failed to create audit log:', auditError.message);
                }
            }

            // Return success response
            res.status(201).json({
                success: true,
                data: {
                    messageId,
                    correlationId,
                    resourceType: fhirResourceType,
                    resourceId: fhirResourceId,
                    status: validationStatus === 'invalid' ? 'failed' : 'stored',
                    validation: {
                        status: validationStatus,
                        error: validationError
                    },
                    interface: {
                        id: interfaceConfig.id,
                        name: interfaceConfig.name
                    },
                    storedAt: new Date().toISOString(),
                    processingDuration: Date.now() - startTime
                }
            });

        } catch (error) {
            console.error('Failed to receive FHIR message:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to process FHIR message',
                details: error.message
            });
        }
    }

    /**
     * Get data lineage for a message (input → transformation → output)
     */
    async getDataLineage(req, res) {
        try {
            await this.ensureDatabase();

            const { messageId } = req.params;
            const userId = req.session.user.id;

            console.log(`🔍 Getting data lineage for message: ${messageId}`);

            // Step 1: Find the input message across all interface tables
            let inputMessage;
            try {
                // Find which interface table contains this message
                const allInterfaces = await this.database.sequelize.query(`
                    SELECT id, name, source_type, target_type FROM interfaces WHERE user_id = :userId
                `, {
                    replacements: { userId },
                    type: this.database.sequelize.QueryTypes.SELECT
                });

                for (const intf of allInterfaces) {
                    const tableName = `messages_intf_${intf.id.replace(/-/g, '_')}`;
                    try {
                        const result = await this.database.sequelize.query(`
                            SELECT
                                m.*,
                                '${intf.id}' as interface_id,
                                '${intf.name}' as interface_name,
                                '${intf.source_type}' as interface_source_type,
                                '${intf.target_type}' as interface_target_type
                            FROM ${tableName} m
                            WHERE m.message_id = :messageId
                            LIMIT 1
                        `, {
                            replacements: { messageId },
                            type: this.database.sequelize.QueryTypes.SELECT
                        });

                        if (result.length > 0) {
                            inputMessage = result[0];
                            inputMessage.input_table_name = tableName;
                            break;
                        }
                    } catch (err) {
                        // Table doesn't exist or no access, continue
                        continue;
                    }
                }
            } catch (err) {
                console.error('Error finding input message:', err);
            }

            if (!inputMessage) {
                return res.status(404).json({
                    success: false,
                    error: 'Message not found'
                });
            }

            // Step 2: Get the transformed message from object storage via Go API
            let transformedMessage = null;
            const storageParams = { interfaceId: inputMessage.interface_id };
            try {
                const [rawResp, transformedResp] = await Promise.allSettled([
                    _goClient.get(`/api/messages/${inputMessage.message_id}/raw`, { params: storageParams, timeout: 10000 }),
                    _goClient.get(`/api/messages/${inputMessage.message_id}/transformed`, { params: storageParams, timeout: 10000 })
                ]);
                const rawContent = rawResp.status === 'fulfilled' && rawResp.value.data?.success
                    ? rawResp.value.data.content : null;
                const transformedData = transformedResp.status === 'fulfilled' && transformedResp.value.data?.success
                    ? transformedResp.value.data : null;
                if (rawContent || transformedData) {
                    transformedMessage = {
                        raw_content: rawContent,
                        transformation_status: 'completed',
                        transformation_metadata: transformedData?.pipeline_steps
                            ? { steps: transformedData.pipeline_steps }
                            : null
                    };
                }
            } catch (err) {
                console.log(`⚠️ Object storage not available for transformed content: ${err.message}`);
            }

            // Fallback: Try PostgreSQL output table (legacy architecture)
            let outputMessage = null;
            if (!transformedMessage) {
                try {
                    const outputTableName = `output_intf_${inputMessage.interface_id.replace(/-/g, '_')}`;
                    const outputResult = await this.database.sequelize.query(`
                        SELECT *
                        FROM ${outputTableName}
                        WHERE input_message_id = :inputMessageId
                           OR correlation_id = :correlationId
                        ORDER BY created_at DESC
                        LIMIT 1
                    `, {
                        replacements: {
                            inputMessageId: inputMessage.message_id,
                            correlationId: inputMessage.correlation_id || inputMessage.message_id
                        },
                        type: this.database.sequelize.QueryTypes.SELECT
                    });

                    if (outputResult.length > 0) {
                        outputMessage = outputResult[0];
                        outputMessage.output_table_name = outputTableName;
                        console.log(`✅ Found output message in PostgreSQL table: ${outputTableName}`);
                    }
                } catch (err) {
                    console.log(`⚠️ No PostgreSQL output table found for interface ${inputMessage.interface_id}`);
                }
            }

            // Step 3: Get target interface (where output was delivered)
            let targetInterface = null;
            let deliveredMessage = null;

            if (outputMessage && outputMessage.delivery_status === 'delivered') {
                // Get target interface from source interface's target_config
                const sourceInterfaceData = await this.database.sequelize.query(`
                    SELECT target_config FROM interfaces WHERE id = :interfaceId
                `, {
                    replacements: { interfaceId: inputMessage.interface_id },
                    type: this.database.sequelize.QueryTypes.SELECT
                });

                let targetInterfaceId = null;
                if (sourceInterfaceData.length > 0 && sourceInterfaceData[0].target_config) {
                    const targetConfig = sourceInterfaceData[0].target_config;
                    // Extract target interface ID from target config (if it's an interface-based target)
                    targetInterfaceId = targetConfig.interfaceId || targetConfig.interface_id;
                }

                if (targetInterfaceId) {
                    try {
                        const targetTableName = `messages_intf_${targetInterfaceId.replace(/-/g, '_')}`;
                        const deliveredResult = await this.database.sequelize.query(`
                            SELECT m.*, i.name as interface_name, i.source_type, i.target_type
                            FROM ${targetTableName} m
                            JOIN interfaces i ON i.id = :targetInterfaceId
                            WHERE m.correlation_id = :correlationId
                               OR m.message_id LIKE '%' || :inputMessageId || '%'
                            ORDER BY m.received_at DESC
                            LIMIT 1
                        `, {
                            replacements: {
                                targetInterfaceId,
                                correlationId: outputMessage.correlation_id,
                                inputMessageId: inputMessage.message_id
                            },
                            type: this.database.sequelize.QueryTypes.SELECT
                        });

                        if (deliveredResult.length > 0) {
                            deliveredMessage = deliveredResult[0];
                            targetInterface = {
                                id: targetInterfaceId,
                                name: deliveredResult[0].interface_name,
                                source_type: deliveredResult[0].source_type,
                                target_type: deliveredResult[0].target_type
                            };
                        }
                    } catch (err) {
                        console.log(`⚠️ Could not find delivered message in target interface`);
                    }
                }
            }

            // Step 4: Fetch content from tables/collections (raw message, transformed output)
            let rawContent = inputMessage.raw_message || null;
            let transformedContent = null;
            let transformationSteps = null;

            // Try to get transformed content from MongoDB first (current architecture)
            if (transformedMessage) {
                transformedContent = transformedMessage.delivery_payload || transformedMessage.transformed_content;
                transformationSteps = transformedMessage.transformation_metadata?.steps || null;
                console.log(`📊 Retrieved transformation with ${transformationSteps?.length || 0} steps from MongoDB`);
            }
            // Fallback: Try to get transformed output from PostgreSQL output table (legacy)
            else if (outputMessage) {
                try {
                    const outputTableName = `output_intf_${inputMessage.interface_id.replace(/-/g, '_')}`;
                    const outputContentQuery = `
                        SELECT transformed_message
                        FROM ${outputTableName}
                        WHERE id = :outputId
                    `;
                    const outputContent = await this.database.sequelize.query(outputContentQuery, {
                        replacements: { outputId: outputMessage.id },
                        type: this.database.sequelize.QueryTypes.SELECT
                    });

                    if (outputContent.length > 0 && outputContent[0].transformed_message) {
                        transformedContent = outputContent[0].transformed_message;

                        // No further resolution needed — content is already available
                    }
                } catch (err) {
                    console.log('Could not fetch transformed message:', err.message);
                }
            }

            // Step 5: Build lineage response
            const lineage = {
                correlationId: inputMessage.correlation_id || inputMessage.message_id,

                // Input stage
                input: {
                    messageId: inputMessage.message_id,
                    interfaceId: inputMessage.interface_id,
                    interfaceName: inputMessage.interface_name,
                    interfaceSourceType: inputMessage.interface_source_type,
                    interfaceTargetType: inputMessage.interface_target_type,
                    tableName: inputMessage.input_table_name,
                    status: inputMessage.status,
                    receivedAt: inputMessage.received_at,
                    messageType: inputMessage.message_type,
                    messageSize: inputMessage.message_size,
                    sourceType: inputMessage.source_type,
                    sourceEndpoint: inputMessage.source_endpoint,
                    rawContent: rawContent
                },

                // Transformation stage
                transformation: inputMessage.parsed_at ? {
                    parsedAt: inputMessage.parsed_at,
                    parsingStatus: inputMessage.parsing_status,
                    parsingTimeMs: inputMessage.parsing_time_ms,
                    transformedAt: inputMessage.processing_completed_at,
                    processingTimeMs: inputMessage.processing_time_ms,
                    sourceFormat: (() => {
                        // message_type (e.g. "FHIR:Bundle:message") is the most reliable
                        // indicator of actual message format — use it first.
                        const mt = (inputMessage.message_type || '').split(':')[0].toLowerCase();
                        if (mt === 'fhir') return 'fhir';
                        if (mt === 'hl7' || mt === 'hl7v2') return 'hl7v2';
                        // Fall back to interface source_type column
                        return inputMessage.interface_source_type || null;
                    })(),
                    targetFormat: inputMessage.interface_target_type || null,
                    steps: transformationSteps || []
                } : null,

                // Output stage — prefer object storage, then legacy output table, then message row itself
                output: transformedMessage ? {
                    storageType: 'object_storage',
                    transformationStatus: transformedMessage.transformation_status,
                    rawContent: transformedMessage.raw_content,
                    deliveryStatus: inputMessage.delivery_status,
                    deliveryCompletedAt: inputMessage.processing_completed_at,
                    deliveryEndpoint: inputMessage.source_endpoint,
                    payloadSizeBytes: inputMessage.message_size
                } : (outputMessage ? {
                    // Legacy PostgreSQL output table data
                    outputMessageId: outputMessage.id,
                    storageType: 'postgresql',
                    tableName: outputMessage.output_table_name,
                    transformationStatus: outputMessage.transformation_status,
                    transformedAt: outputMessage.transformed_at,
                    transformationTimeMs: outputMessage.transformation_time_ms,
                    deliveryStatus: outputMessage.delivery_status,
                    deliveryStartedAt: outputMessage.delivery_started_at,
                    deliveryCompletedAt: outputMessage.delivery_completed_at,
                    deliveryTimeMs: outputMessage.delivery_time_ms,
                    deliveryStatusCode: outputMessage.delivery_status_code,
                    deliveryEndpoint: outputMessage.delivery_endpoint_url || outputMessage.endpoint_url,
                    deliveryResponse: outputMessage.delivery_response_body || outputMessage.response_body,
                    retryCount: outputMessage.retry_count,
                    transformedMessage: transformedContent,
                    transformation_steps: outputMessage.transformation_steps || []
                } : {
                    // Current architecture — delivery tracked directly on the message row
                    storageType: 'message_row',
                    deliveryStatus: inputMessage.delivery_status || 'pending',
                    deliveryCompletedAt: inputMessage.processing_completed_at,
                    payloadSizeBytes: inputMessage.message_size,
                    rawContentUri: inputMessage.raw_content_uri,
                    parsedContentUri: inputMessage.parsed_content_uri,
                    transformedContentUri: inputMessage.transformed_content_uri
                }),

                // Target interface (where message was delivered)
                target: deliveredMessage ? {
                    messageId: deliveredMessage.message_id,
                    interfaceId: targetInterface.id,
                    interfaceName: targetInterface.name,
                    sourceType: targetInterface.source_type,
                    targetType: targetInterface.target_type,
                    status: deliveredMessage.status,
                    receivedAt: deliveredMessage.received_at,
                    messageType: deliveredMessage.message_type,
                    messageSize: deliveredMessage.message_size
                } : null,

                // Flow summary
                flowStatus: {
                    inputReceived: !!inputMessage,
                    transformed: !!(outputMessage || transformedMessage || inputMessage?.parsed_at),
                    delivered: inputMessage?.delivery_status === 'delivered' ||
                               outputMessage?.delivery_status === 'delivered' ||
                               transformedMessage?.delivery_status === 'delivered',
                    targetReceived: !!deliveredMessage,
                    complete: !!(inputMessage && inputMessage.delivery_status === 'delivered')
                }
            };

            // Debug: Log what we're actually returning
            console.log('✅ Lineage query completed:', {
                hasInputMessage: !!inputMessage,
                hasTransformedMessage: !!transformedMessage,
                hasOutputMessage: !!outputMessage,
                transformationSteps: transformationSteps?.length || 0,
                hasRawContent: !!rawContent,
                hasTransformedContent: !!transformedContent,
                deliveryStatus: transformedMessage?.delivery_status || outputMessage?.delivery_status || 'unknown',
                flowStatus: lineage.flowStatus
            });

            res.json({
                success: true,
                data: lineage
            });

        } catch (error) {
            console.error('❌ Get Data Lineage Error:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to retrieve data lineage',
                debug: error.message
            });
        }
    }

    /**
     * Get errors and warnings for a specific message
     * Uses PostgreSQL get_message_errors() function from V23 migration
     */
    async getMessageErrors(req, res) {
        try {
            await this.ensureDatabase();

            const { messageId } = req.params;
            const { interfaceId } = req.query;

            if (!interfaceId) {
                return res.status(400).json({
                    success: false,
                    error: 'interfaceId query parameter is required'
                });
            }

            // Get table name
            const tableName = `messages_intf_${interfaceId.replace(/-/g, '_')}`;

            // Call PostgreSQL get_message_errors() function
            const errors = await this.database.sequelize.query(`
                SELECT
                    error_timestamp,
                    stage,
                    severity,
                    error_type,
                    message,
                    details,
                    stack_trace,
                    recovery_action
                FROM get_message_errors(:tableName, :messageId)
                ORDER BY error_timestamp DESC
            `, {
                replacements: { tableName, messageId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            // Also get error summary
            const summary = await this.database.sequelize.query(`
                SELECT
                    error_count,
                    last_error_message,
                    last_error_timestamp,
                    last_error_stage,
                    error_severity
                FROM ${tableName}
                WHERE message_id = :messageId
            `, {
                replacements: { messageId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            res.json({
                success: true,
                data: {
                    errors: errors,
                    summary: summary[0] || {
                        error_count: 0,
                        last_error_message: null,
                        last_error_timestamp: null,
                        last_error_stage: null,
                        error_severity: null
                    }
                }
            });

        } catch (error) {
            console.error('❌ Get Message Errors Error:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to retrieve message errors',
                debug: error.message
            });
        }
    }

    /**
     * Get processing logs for a message from MongoDB (V33 - Interface-Level Logging)
     */
    async getMessageLogs(req, res) {
        try {
            await this.ensureDatabase();

            const { messageId } = req.params;
            const { interfaceId, level } = req.query;

            if (!interfaceId) {
                return res.status(400).json({
                    success: false,
                    error: 'interfaceId query parameter is required'
                });
            }

            // Fetch logs from Go API (backed by object storage)
            const logsResp = await _goClient.get(
                `/api/messages/${messageId}/logs`,
                { params: { interfaceId }, timeout: 10000 }
            );

            const entries = logsResp.data.logs || [];

            // Apply level filter client-side
            const filtered = level && level !== 'all'
                ? entries.filter(e => level === 'error'
                    ? (e.level === 'error' || e.level === 'warning')
                    : e.level === level)
                : entries;

            const summary = {
                total: filtered.length,
                errors:   filtered.filter(l => l.level === 'error').length,
                warnings: filtered.filter(l => l.level === 'warning').length,
                info:     filtered.filter(l => l.level === 'info').length,
                debug:    filtered.filter(l => l.level === 'debug').length
            };

            res.json({
                success: true,
                data: {
                    logs: filtered.map(e => ({
                        timestamp: e.ts,
                        level:     e.level,
                        category:  e.stage,
                        message:   e.message,
                        details:   e.fields
                    })),
                    summary
                }
            });

        } catch (error) {
            console.error('Failed to retrieve message logs:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to retrieve message logs',
                debug: error.message
            });
        }
    }

    /**
     * Get parsed content (narrativeHTML, structured sections) for a CDA/FHIR message.
     * Proxies to Go backend which owns the object storage client.
     * Returns { success, parsed_content, format } — gracefully null if Go endpoint is not yet available.
     */
    async getParsedContent(req, res) {
        try {
            await this.ensureDatabase();

            const { interfaceId, messageId } = req.params;
            const userId = req.session.user.id;

            const check = await this.database.sequelize.query(
                `SELECT id FROM interfaces WHERE id = :interfaceId AND user_id = :userId`,
                { replacements: { interfaceId, userId }, type: this.database.sequelize.QueryTypes.SELECT }
            );
            if (check.length === 0) {
                return res.status(404).json({ success: false, error: 'Interface not found or access denied' });
            }

            // Read raw message from the interface-specific table
            const tableName = `messages_intf_${interfaceId.replace(/-/g, '_')}`;
            let rawMessage = null;
            let messageType = null;
            try {
                const rows = await this.database.sequelize.query(
                    `SELECT raw_message, message_type FROM ${tableName}
                     WHERE message_id = :messageId OR id::text = :messageId
                     LIMIT 1`,
                    { replacements: { messageId }, type: this.database.sequelize.QueryTypes.SELECT }
                );
                if (rows.length > 0) {
                    rawMessage   = rows[0].raw_message;
                    messageType  = rows[0].message_type;
                }
            } catch (dbErr) {
                console.warn(`⚠️ getParsedContent DB read: ${dbErr.message}`);
            }

            // raw_message is NULL for file-based messages (content in object storage).
            // Fall back to Go's raw content endpoint which reads from MinIO using the
            // stored raw_content_uri (correct across day boundaries).
            if (!rawMessage) {
                try {
                    const _proxySecret = process.env.INTERNAL_PROXY_SECRET || process.env.JWT_SECRET || '';
                    const rawResp = await _goClient.get(`/api/messages/${messageId}/raw`, {
                        params: { interfaceId },
                        headers: _proxySecret ? { 'X-Internal-Proxy-Secret': _proxySecret } : {},
                        timeout: 10000,
                    });
                    if (rawResp.data && rawResp.data.success && rawResp.data.content) {
                        rawMessage = rawResp.data.content;
                    }
                } catch (fetchErr) {
                    console.warn(`⚠️ getParsedContent: object storage fetch failed: ${fetchErr.message}`);
                }
            }

            if (!rawMessage) {
                return res.json({ success: false, parsed_content: null });
            }

            // Only parse CDA/XML content — HL7v2 has its own viewer
            const isCDA = (messageType || '').toUpperCase().includes('CCD') ||
                          (messageType || '').toUpperCase().includes('CDA') ||
                          rawMessage.trimStart().startsWith('<');
            if (!isCDA) {
                return res.json({ success: false, parsed_content: null, reason: 'not_cda' });
            }

            // Call Go's CDA parse endpoint with the raw XML.
            // Explicitly include the proxy secret — axios does not merge it from the
            // interceptor when the per-request headers object replaces the defaults.
            const _proxySecret = process.env.INTERNAL_PROXY_SECRET || process.env.JWT_SECRET || '';
            const goResp = await _goClient.post(`/api/fhir/cda/parse`, rawMessage, {
                headers: {
                    'Content-Type': 'application/xml',
                    ...((_proxySecret) ? { 'X-Internal-Proxy-Secret': _proxySecret } : {}),
                },
                timeout: 15000
            });

            if (!goResp.data || !goResp.data.success) {
                return res.json({ success: false, parsed_content: null });
            }

            return res.json({
                success: true,
                format: goResp.data.format || 'ccda',
                parsed_content: {
                    sections: goResp.data.sections || {},
                    header:   goResp.data.header   || null,
                    metadata: goResp.data.metadata  || null,
                    profile:  goResp.data.profile   || null,
                }
            });
        } catch (error) {
            console.warn(`⚠️ getParsedContent: ${error.message}`);
            return res.json({ success: false, parsed_content: null, error: error.message });
        }
    }

    /**
     * Return the stored FHIR bundle for a CDA message.
     * Reads from cda_documents.fhir_bundle (inline) or object-storage URI.
     * Route: GET /api/messages/interface/:interfaceId/message/:messageId/fhir-output
     */
    async getFhirOutput(req, res) {
        try {
            await this.ensureDatabase();
            const { interfaceId, messageId } = req.params;
            const userId = req.session.user.id;

            const check = await this.database.sequelize.query(
                `SELECT id FROM interfaces WHERE id = :interfaceId AND user_id = :userId`,
                { replacements: { interfaceId, userId }, type: this.database.sequelize.QueryTypes.SELECT }
            );
            if (check.length === 0) {
                return res.status(404).json({ success: false, error: 'Interface not found' });
            }

            // Look up the stored FHIR bundle from cda_documents keyed by message_id
            const rows = await this.database.sequelize.query(
                `SELECT fhir_bundle FROM cda_documents
                 WHERE message_id = :messageId AND deleted_at IS NULL
                 ORDER BY created_at DESC LIMIT 1`,
                { replacements: { messageId }, type: this.database.sequelize.QueryTypes.SELECT }
            );

            if (!rows.length || !rows[0].fhir_bundle) {
                return res.json({ success: false, content: null });
            }

            const bundle = typeof rows[0].fhir_bundle === 'string'
                ? rows[0].fhir_bundle
                : JSON.stringify(rows[0].fhir_bundle);

            return res.json({ success: true, content: bundle, format: 'fhir_json' });
        } catch (error) {
            console.warn(`⚠️ getFhirOutput: ${error.message}`);
            return res.json({ success: false, content: null, error: error.message });
        }
    }

    /**
     * Returns cda.dedupe's cross-message suppression lineage for one message —
     * which clinical facts were dropped as duplicates of an earlier message,
     * and which earlier message first delivered each one. Sourced from
     * step_executions.step_output (already persisted for every real message by
     * TransformationPipelineService.persistExecutionRecords — no new
     * persistence, this is a read-only projection over existing data).
     *
     * Scoped to interfaces the requesting user owns, same as getMessageDetail —
     * viewing one message's own processing detail is a normal read, not the
     * cross-patient browse the dedupe registry admin viewer gates separately.
     *
     * Empty array (not 404) when no cda.dedupe step ran for this message, or
     * it ran but suppressed nothing — most messages fall in this bucket.
     */
    async getDedupeSuppressions(req, res) {
        try {
            await this.ensureDatabase();
            const { messageId } = req.params;
            const userId = req.session.user.id;

            const rows = await this.database.sequelize.query(`
                SELECT se.step_output
                FROM step_executions se
                JOIN transformation_executions te ON te.id = se.execution_id
                JOIN interfaces i ON i.id = te.interface_id
                WHERE te.message_id = :messageId
                  AND se.step_type = 'cda.dedupe'
                  AND i.user_id = :userId
                ORDER BY se.started_at DESC
            `, {
                replacements: { messageId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            const suppressions = [];
            for (const row of rows) {
                const output = typeof row.step_output === 'string' ? JSON.parse(row.step_output) : row.step_output;
                const sectionStats = output?.section_stats || {};
                for (const [sectionKey, stats] of Object.entries(sectionStats)) {
                    const entries = stats?.cross_message_suppressed;
                    if (!Array.isArray(entries)) continue;
                    for (const entry of entries) {
                        suppressions.push({
                            sectionKey,
                            identityKey: entry.identity_key,
                            firstSeenMessageId: entry.first_seen_message_id,
                            firstSeenAt: entry.first_seen_at,
                        });
                    }
                }
            }

            return res.json({ success: true, data: suppressions, count: suppressions.length });
        } catch (error) {
            console.warn(`⚠️ getDedupeSuppressions: ${error.message}`);
            return res.status(500).json({ success: false, error: 'Failed to retrieve dedupe suppression lineage' });
        }
    }

    // getCoverageAudit returns the CDA Coverage Audit report(s) for a message —
    // one row per outbound destination (a pipeline can fan out to more than one
    // connector.outbound step). Reads the dedicated cda_coverage_audits table
    // directly (V207 migration) rather than unwrapping step_executions.step_output
    // the way getDedupeSuppressions does above, since a coverage report isn't
    // itself a pipeline step's output — it's built asynchronously, after
    // delivery, by the background worker pool (see services/cda_coverage).
    // Empty array (not 404) is the common case for interfaces that haven't
    // opted into the feature, or a message that was fully covered — same
    // "absence isn't an error" convention getDedupeSuppressions uses.
    async getCoverageAudit(req, res) {
        try {
            await this.ensureDatabase();
            const { messageId } = req.params;
            const userId = req.session.user.id;

            // DISTINCT ON step_id: a redelivered/re-run message can produce more
            // than one historical row for the SAME destination (one per attempt).
            // Only the latest report per destination is meaningful for "how many
            // destinations have gaps right now" — older attempts are superseded,
            // not additional destinations.
            const rows = await this.database.sequelize.query(`
                SELECT DISTINCT ON (ca.step_id)
                       ca.step_id, ca.step_name, ca.connector_type, ca.destination,
                       ca.delivery_outcome, ca.overall_coverage_pct, ca.category_stats, ca.created_at
                FROM cda_coverage_audits ca
                JOIN interfaces i ON i.id = ca.interface_id
                WHERE ca.message_id = :messageId
                  AND i.user_id = :userId
                ORDER BY ca.step_id, ca.created_at DESC
            `, {
                replacements: { messageId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            const reports = rows.map(row => {
                const stats = typeof row.category_stats === 'string' ? JSON.parse(row.category_stats) : row.category_stats;
                return {
                    stepId: row.step_id,
                    stepName: row.step_name,
                    connectorType: row.connector_type,
                    destination: row.destination,
                    deliveryOutcome: row.delivery_outcome,
                    overallCoveragePct: row.overall_coverage_pct,
                    // elementCoveragePct (report.go v4+) is the document-wide
                    // CLINICAL element completeness -- null for reports from
                    // an entry-granularity interface, or ones persisted
                    // before this field existed. The UI should prefer this
                    // over overallCoveragePct whenever it's non-null: entry-
                    // level alone can read a misleading 100% while every
                    // entry still has real element-level gaps (an entry only
                    // needs ONE field read to count as entry-level "found").
                    elementCoveragePct: stats?.elementCoveragePct ?? null,
                    categories: stats?.categories || [],
                    createdAt: row.created_at,
                };
            });

            return res.json({ success: true, data: reports, count: reports.length });
        } catch (error) {
            console.warn(`⚠️ getCoverageAudit: ${error.message}`);
            return res.status(500).json({ success: false, error: 'Failed to retrieve CDA coverage audit' });
        }
    }

    async getTableNameForMessage(messageId, userId) {
        // Helper to find which table a message belongs to
        if (!userId) return null;

        const interfaces = await this.database.sequelize.query(`
            SELECT id FROM interfaces WHERE user_id = :userId
        `, {
            replacements: { userId },
            type: this.database.sequelize.QueryTypes.SELECT
        });

        for (const intf of interfaces) {
            const tableName = `messages_intf_${intf.id.replace(/-/g, '_')}`;
            try {
                const result = await this.database.sequelize.query(`
                    SELECT 1 FROM ${tableName} WHERE id = :messageId LIMIT 1
                `, {
                    replacements: { messageId },
                    type: this.database.sequelize.QueryTypes.SELECT
                });

                if (result.length > 0) {
                    return tableName;
                }
            } catch (err) {
                continue;
            }
        }
        return null;
    }
}

module.exports = new MessageController();