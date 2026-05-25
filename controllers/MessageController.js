// controllers/MessageController.js
// Enhanced message management controller for interface-specific functionality

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
                    // Unsupported source type
                    return res.status(400).json({
                        success: false,
                        error: `Unsupported source type: ${interfaceData.source_type}`,
                        supportedTypes: ['tcp', 'mllp', 'http', 'fhir']
                    });
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

                    // Use $1 placeholder instead of :messageId for better UUID handling
                    const checkQuery = `SELECT * FROM ${metadata.table_name} WHERE id = $1`;
                    const result = await this.database.sequelize.query(checkQuery, {
                        bind: [messageId],
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

            // Update message status to reprocessing and reset delivery status
            const updateQuery = `
                UPDATE ${tableName}
                SET status = 'reprocessing',
                    delivery_status = 'queued',
                    delivery_attempts = COALESCE(delivery_attempts, 0) + 1,
                    last_error_message = NULL,
                    updated_at = CURRENT_TIMESTAMP
                WHERE id = $1
            `;

            await this.database.sequelize.query(updateQuery, {
                bind: [messageId],
                type: this.database.sequelize.QueryTypes.UPDATE
            });

            // Trigger actual reprocessing via Go backend
            // Send the raw message back to the processing engine
            try {
                const axios = require('axios');
                const goBackendUrl = process.env.GO_BACKEND_URL || 'http://localhost:8080';

                await axios.post(`${goBackendUrl}/api/processing/reprocess`, {
                    messageId: messageId,
                    interfaceId: interfaceId,
                    rawMessage: message.raw_message,
                    messageType: message.message_type
                });
            } catch (goError) {
                console.warn('⚠️ Could not trigger Go backend reprocessing, will rely on status change:', goError.message);

                // Fallback: Reset status to 'received' so it gets picked up by processing engine
                setTimeout(async () => {
                    try {
                        await this.database.sequelize.query(`
                            UPDATE ${tableName}
                            SET status = 'received'
                            WHERE id = :messageId
                        `, {
                            replacements: { messageId },
                            type: this.database.sequelize.QueryTypes.UPDATE
                        });
                    } catch (err) {
                        console.error('Error updating reprocess status:', err);
                    }
                }, 1000);
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

            // Verify user owns this interface
            const interfaceCheck = await this.database.sequelize.query(`
                SELECT id, name, status
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
                sortOrder
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
                    interfaceName: interfaceInfo.name
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
            interfaceName
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
     * HL7 TO FHIR MESSAGE FLOW (NEW)
     */

    /**
     * Send HL7 message from source interface to FHIR target interface
     */
    async sendHL7ToFHIR(req, res) {
        try {
            await this.ensureDatabase();

            const {
                sourceInterfaceId,
                targetInterfaceId,
                hl7Message,
                messageType = 'ADT^A01',
                priority = 5
            } = req.body;

            const userId = req.session.user.id;

            // Verify both interfaces exist and belong to user
            const interfacesCheck = await this.database.sequelize.query(`
                SELECT id, name, format, connectivity_type, status
                FROM interfaces
                WHERE id IN (:sourceInterfaceId, :targetInterfaceId) AND user_id = :userId
            `, {
                replacements: { sourceInterfaceId, targetInterfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (interfacesCheck.length !== 2) {
                return res.status(404).json({
                    success: false,
                    error: 'One or both interfaces not found or access denied'
                });
            }

            const sourceInterface = interfacesCheck.find(i => i.id === sourceInterfaceId);
            const targetInterface = interfacesCheck.find(i => i.id === targetInterfaceId);

            // Validate interface compatibility
            if (sourceInterface.format !== 'HL7' && sourceInterface.format !== 'hl7') {
                return res.status(400).json({
                    success: false,
                    error: 'Source interface must be HL7 format'
                });
            }

            if (targetInterface.format !== 'FHIR' && targetInterface.format !== 'fhir') {
                return res.status(400).json({
                    success: false,
                    error: 'Target interface must be FHIR format'
                });
            }

            // Generate correlation ID for tracking
            const correlationId = require('crypto').randomUUID();
            const messageId = `HL7_TO_FHIR_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;

            // Create source message entry
            const sourceMessageQuery = `
                INSERT INTO message_processing_enhanced (
                    interface_id, message_id, correlation_id, status, priority,
                    source_type, message_type, message_size, message_encoding,
                    received_at
                ) VALUES (
                    :sourceInterfaceId, :messageId, :correlationId, 'received', :priority,
                    'hl7_test_flow', :messageType, :messageSize, 'UTF-8',
                    NOW()
                ) RETURNING id
            `;

            const sourceMessageResult = await this.database.sequelize.query(sourceMessageQuery, {
                replacements: {
                    sourceInterfaceId,
                    messageId,
                    correlationId,
                    priority,
                    messageType,
                    messageSize: Buffer.byteLength(hl7Message, 'utf8')
                },
                type: this.database.sequelize.QueryTypes.INSERT
            });

            // Add to message queue for processing
            const queueEntry = {
                id: require('crypto').randomUUID(),
                interface_id: sourceInterfaceId,
                target_interface_id: targetInterfaceId,
                message_id: messageId,
                correlation_id: correlationId,
                action_type: 'hl7_to_fhir_transform',
                priority: priority,
                payload: {
                    hl7_message: hl7Message,
                    message_type: messageType,
                    source_interface: sourceInterface,
                    target_interface: targetInterface
                },
                status: 'pending',
                scheduled_for: new Date(),
                created_at: new Date()
            };

            await this.database.sequelize.query(`
                INSERT INTO message_processing_queue (
                    id, interface_id, message_id, correlation_id, action_type,
                    priority, payload, status, scheduled_for, created_at
                ) VALUES (
                    :id, :interface_id, :message_id, :correlation_id, :action_type,
                    :priority, :payload, :status, :scheduled_for, :created_at
                )
            `, {
                replacements: {
                    ...queueEntry,
                    payload: JSON.stringify(queueEntry.payload)
                },
                type: this.database.sequelize.QueryTypes.INSERT
            });

            res.json({
                success: true,
                data: {
                    messageId,
                    correlationId,
                    sourceInterface: {
                        id: sourceInterface.id,
                        name: sourceInterface.name
                    },
                    targetInterface: {
                        id: targetInterface.id,
                        name: targetInterface.name
                    },
                    status: 'queued_for_processing',
                    queuedAt: new Date().toISOString()
                }
            });

        } catch (error) {
            console.error('Failed to send HL7 to FHIR:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to process HL7 to FHIR message flow'
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
            const goBackendUrl = process.env.GO_BACKEND_URL || `http://localhost:${process.env.API_PORT || 8080}`;
            const storageParams = { interfaceId: inputMessage.interface_id };
            try {
                const axios = require('axios');
                const [rawResp, transformedResp] = await Promise.allSettled([
                    axios.get(`${goBackendUrl}/api/messages/${inputMessage.message_id}/raw`, { params: storageParams, timeout: 10000 }),
                    axios.get(`${goBackendUrl}/api/messages/${inputMessage.message_id}/transformed`, { params: storageParams, timeout: 10000 })
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
            const goBackendUrl = process.env.GO_BACKEND_URL || `http://localhost:${process.env.API_PORT || 8080}`;
            const axios = require('axios');
            const logsResp = await axios.get(
                `${goBackendUrl}/api/messages/${messageId}/logs`,
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