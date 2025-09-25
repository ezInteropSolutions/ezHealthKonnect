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
                whereConditions.push('mpe.status = :status');
                replacements.status = status;
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
            const userId = req.session.user.id;

            // Find which interface table contains this message
            let messageResult = null;
            let interfaceName = null;

            // Get all user's interfaces to search through their tables
            const interfacesQuery = `
                SELECT id, name FROM interfaces
                WHERE user_id = :userId AND status IN ('active', 'configured')
            `;

            const interfaces = await this.database.sequelize.query(interfacesQuery, {
                replacements: { userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            // Search each interface table for the message
            for (const iface of interfaces) {
                const tableName = this.tableManager.getInterfaceTableName(iface.id);

                try {
                    const query = `
                        SELECT * FROM ${tableName}
                        WHERE id = :messageId
                    `;

                    const result = await this.database.sequelize.query(query, {
                        replacements: { messageId },
                        type: this.database.sequelize.QueryTypes.SELECT
                    });

                    if (result.length > 0) {
                        messageResult = result;
                        interfaceName = iface.name;
                        break;
                    }
                } catch (tableError) {
                    // Table might not exist, continue searching
                    continue;
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

            // Generate message ID
            const messageId = `UI-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

            // Store message in interface-specific table using InterfaceTableManager
            let processingId;
            try {
                console.log(`📧 Storing message in interface-specific table for interface ${interfaceId}`);

                const messageData = {
                    messageId,
                    correlationId: messageId, // Use messageId as correlation ID for UI-generated messages
                    interfaceId: interfaceId, // Required for NOT NULL constraint
                    interfaceName: interfaceData.name,
                    status: 'processing',
                    sourceType: 'ui_test',
                    sourceEndpoint: 'ui_manual_send',
                    sourceIP: req.ip || req.connection.remoteAddress || '127.0.0.1',
                    messageType: messageType || 'Unknown',
                    messageSize: messageContent?.length || 0,
                    messageEncoding: 'UTF-8',
                    rawMessage: messageContent, // Store the actual message content
                    receivedAt: new Date()
                };

                processingId = await this.tableManager.insertMessage(interfaceId, messageData);
                console.log(`✅ Message stored in interface table with ID: ${processingId}`);
            } catch (dbError) {
                console.error('Interface table insertion error:', dbError.message);
                return res.status(500).json({
                    success: false,
                    error: 'Failed to store message in interface table',
                    details: dbError.message
                });
            }

            // Note: processingId is the UUID returned from interface table insertion
            const messageRecordId = processingId;

            // Send to interface based on source type and config
            let deliveryResult;
            try {
                if (interfaceData.source_type === 'fhir' || interfaceData.source_type === 'http') {
                    // Send HTTP request
                    const port = sourceConfig.port || 8090;
                    const path = sourceConfig.path || '/fhir';

                    const fetch = require('node-fetch');
                    const response = await fetch(`http://localhost:${port}${path}`, {
                        method: 'POST',
                        headers: {
                            'Content-Type': contentType,
                            'X-Message-ID': messageId
                        },
                        body: messageContent
                    });

                    deliveryResult = {
                        success: response.ok,
                        status: response.status,
                        response: await response.text()
                    };
                } else {
                    // For other types, just mark as sent for now
                    deliveryResult = {
                        success: true,
                        status: 200,
                        response: 'Message queued for processing'
                    };
                }

                // Update processing status in interface table
                const finalStatus = deliveryResult.success ? 'sent' : 'failed';
                const tableName = this.tableManager.getInterfaceTableName(interfaceId);

                await this.database.sequelize.query(`
                    UPDATE ${tableName}
                    SET status = :status,
                        processing_completed_at = CURRENT_TIMESTAMP,
                        delivery_status = :deliveryStatus,
                        delivery_attempts = 1,
                        processing_time_ms = EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - received_at)) * 1000
                    WHERE id = :messageRecordId
                `, {
                    replacements: {
                        status: finalStatus,
                        deliveryStatus: deliveryResult.success ? 'delivered' : 'failed',
                        messageRecordId
                    },
                    type: this.database.sequelize.QueryTypes.UPDATE
                });

            } catch (deliveryError) {
                // Update with error status in interface table
                const tableName = this.tableManager.getInterfaceTableName(interfaceId);

                await this.database.sequelize.query(`
                    UPDATE ${tableName}
                    SET status = 'failed',
                        processing_completed_at = CURRENT_TIMESTAMP,
                        error_count = error_count + 1,
                        last_error_message = :errorMessage,
                        last_error_at = CURRENT_TIMESTAMP
                    WHERE id = :messageRecordId
                `, {
                    replacements: {
                        errorMessage: deliveryError.message,
                        messageRecordId
                    },
                    type: this.database.sequelize.QueryTypes.UPDATE
                });

                deliveryResult = {
                    success: false,
                    error: deliveryError.message
                };
            }

            res.json({
                success: true,
                data: {
                    messageId,
                    messageRecordId,
                    deliveryResult,
                    message: 'Message sent successfully'
                }
            });

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

            // Verify message exists and user has access
            const messageQuery = `
                SELECT mpe.*, i.name as interface_name
                FROM message_processing_enhanced mpe
                JOIN interfaces i ON mpe.interface_id = i.id
                WHERE mpe.id = :messageId AND i.user_id = :userId
                AND mpe.status IN ('failed', 'error')
            `;

            const messageResult = await this.database.sequelize.query(messageQuery, {
                replacements: { messageId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (messageResult.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Message not found or not eligible for reprocessing'
                });
            }

            // Update message status to reprocessing
            await this.database.sequelize.query(`
                UPDATE message_processing_enhanced
                SET status = 'reprocessing',
                    retry_count = retry_count + 1,
                    processing_started_at = CURRENT_TIMESTAMP,
                    last_error_message = NULL
                WHERE id = :messageId
            `, {
                replacements: { messageId },
                type: this.database.sequelize.QueryTypes.UPDATE
            });

            // TODO: Trigger actual reprocessing logic here
            // For now, just mark as received to be picked up by processing engine

            setTimeout(async () => {
                try {
                    await this.database.sequelize.query(`
                        UPDATE message_processing_enhanced
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

            res.json({
                success: true,
                message: 'Message queued for reprocessing'
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
            whereConditions.push('mpe.status = :status');
            replacements.status = status;
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

            // Time range calculation
            let timeFilter = '';
            switch (timeRange) {
                case '1h':
                    timeFilter = "AND mpe.received_at >= NOW() - INTERVAL '1 hour'";
                    break;
                case '24h':
                    timeFilter = "AND mpe.received_at >= NOW() - INTERVAL '24 hours'";
                    break;
                case '7d':
                    timeFilter = "AND mpe.received_at >= NOW() - INTERVAL '7 days'";
                    break;
                case '30d':
                    timeFilter = "AND mpe.received_at >= NOW() - INTERVAL '30 days'";
                    break;
                default:
                    timeFilter = "AND mpe.received_at >= NOW() - INTERVAL '24 hours'";
            }

            const statsQuery = `
                SELECT
                    COUNT(*) as total_messages,
                    COUNT(CASE WHEN mpe.status IN ('sent', 'delivered') THEN 1 END) as successful_messages,
                    COUNT(CASE WHEN mpe.status IN ('failed', 'error') THEN 1 END) as failed_messages,
                    COUNT(CASE WHEN mpe.status IN ('received', 'processing', 'transforming') THEN 1 END) as processing_messages,
                    AVG(mpe.processing_time_ms) as avg_processing_time,
                    MAX(mpe.received_at) as last_message_at,
                    COUNT(DISTINCT mpe.message_type) as unique_message_types
                FROM message_processing_enhanced mpe
                WHERE mpe.interface_id = :interfaceId ${timeFilter}
            `;

            const statsResult = await this.database.sequelize.query(statsQuery, {
                replacements: { interfaceId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            res.json({
                success: true,
                data: {
                    interface: interfaceCheck[0],
                    timeRange,
                    stats: statsResult[0]
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
            const fhirInterface = await database.sequelize.query(`
                SELECT id, name, target_config, processing_rules
                FROM interfaces
                WHERE message_type = 'FHIR'
                AND source_type = 'http'
                AND target_type = 'database'
                AND status = 'active'
                LIMIT 1
            `, { type: database.sequelize.QueryTypes.SELECT });

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

            const insertResult = await database.sequelize.query(`
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
}

module.exports = new MessageController();