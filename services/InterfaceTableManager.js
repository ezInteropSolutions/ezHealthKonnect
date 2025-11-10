// services/InterfaceTableManager.js
// Interface-specific table-per-interface management for ultimate performance isolation

class InterfaceTableManager {
    constructor() {
        this.database = require('../config/database');
        this.tableCache = new Map(); // Cache of existing interface tables
        this.schemaTemplate = this.getMessageTableSchema();
    }

    /**
     * CORE: Get table name for interface (dedicated table naming)
     */
    getInterfaceTableName(interfaceId) {
        // Convert UUID to safe table identifier (use FULL UUID, not truncated)
        // PostgreSQL allows up to 63 chars for identifiers, UUID is 36 chars + prefix = 50 chars total
        const fullId = interfaceId.replace(/-/g, '_');
        return `messages_intf_${fullId}`;
    }

    /**
     * CORE: Ensure interface table exists (create if needed)
     */
    async ensureInterfaceTable(interfaceId, interfaceName) {
        const tableName = this.getInterfaceTableName(interfaceId);

        // Check cache first
        if (this.tableCache.has(tableName)) {
            return tableName;
        }

        try {
            // Check if table exists
            const tableExists = await this.checkTableExists(tableName);

            if (!tableExists) {
                console.log(`🏗️ Creating dedicated table for interface: ${interfaceName} -> ${tableName}`);
                await this.createInterfaceTable(tableName, interfaceId);
            }

            // Cache the table name
            this.tableCache.set(tableName, {
                interfaceId,
                interfaceName,
                createdAt: new Date()
            });

            return tableName;

        } catch (error) {
            console.error(`❌ Failed to ensure interface table ${tableName}:`, error);
            throw error;
        }
    }

    /**
     * Check if table exists in database
     */
    async checkTableExists(tableName) {
        const query = `
            SELECT EXISTS (
                SELECT FROM information_schema.tables
                WHERE table_schema = 'public'
                AND table_name = :tableName
            )
        `;

        const result = await this.database.sequelize.query(query, {
            replacements: { tableName },
            type: this.database.sequelize.QueryTypes.SELECT
        });

        return result[0].exists;
    }

    /**
     * Create new interface-specific message table
     */
    async createInterfaceTable(tableName, interfaceId) {
        const createTableSQL = this.schemaTemplate.replace(/{table_name}/g, tableName);
        const createIndexesSQL = this.getIndexSQL(tableName);

        await this.database.sequelize.transaction(async (t) => {
            // Create table
            await this.database.sequelize.query(createTableSQL, { transaction: t });

            // Create indexes
            for (const indexSQL of createIndexesSQL) {
                await this.database.sequelize.query(indexSQL, { transaction: t });
            }

            // Log table creation in metadata
            await this.logTableCreation(tableName, interfaceId, t);
        });

        console.log(`✅ Created interface table: ${tableName} with performance indexes`);
    }

    /**
     * Get message table schema template
     */
    getMessageTableSchema() {
        return `
            CREATE TABLE {table_name} (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                message_id VARCHAR(255) NOT NULL,
                correlation_id VARCHAR(255),
                interface_id UUID NOT NULL,

                -- Message status and processing
                status VARCHAR(50) NOT NULL DEFAULT 'received'
                    CHECK (status IN ('received', 'processing', 'transformed', 'sent', 'delivered', 'failed', 'error', 'reprocessing')),
                priority INTEGER NOT NULL DEFAULT 5,

                -- Timing information
                received_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
                processing_started_at TIMESTAMP WITH TIME ZONE,
                processing_completed_at TIMESTAMP WITH TIME ZONE,

                -- Source information
                source_type VARCHAR(100) NOT NULL,
                source_endpoint VARCHAR(500),
                source_ip INET,

                -- Message metadata
                message_type VARCHAR(100),
                message_size INTEGER,
                message_encoding VARCHAR(50) DEFAULT 'UTF-8',
                raw_message TEXT,

                -- Processing results
                transformation_applied BOOLEAN DEFAULT false,
                transformation_type VARCHAR(100),
                processing_time_ms INTEGER,

                -- Error handling
                error_count INTEGER DEFAULT 0,
                last_error_message TEXT,
                last_error_at TIMESTAMP WITH TIME ZONE,
                retry_count INTEGER DEFAULT 0,
                max_retries INTEGER DEFAULT 3,

                -- Delivery information
                target_endpoint VARCHAR(500),
                delivery_status VARCHAR(50),
                delivery_attempts INTEGER DEFAULT 0,
                last_delivery_attempt_at TIMESTAMP WITH TIME ZONE,

                -- Audit fields
                created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
            )
        `;
    }

    /**
     * Get performance indexes for interface table
     */
    getIndexSQL(tableName) {
        return [
            `CREATE INDEX idx_${tableName}_received_at ON ${tableName}(received_at DESC)`,
            `CREATE INDEX idx_${tableName}_status ON ${tableName}(status, received_at DESC)`,
            `CREATE INDEX idx_${tableName}_correlation ON ${tableName}(correlation_id) WHERE correlation_id IS NOT NULL`,
            `CREATE INDEX idx_${tableName}_message_id ON ${tableName}(message_id)`,
            `CREATE INDEX idx_${tableName}_message_type ON ${tableName}(message_type) WHERE message_type IS NOT NULL`,
            `CREATE INDEX idx_${tableName}_processing_time ON ${tableName}(processing_started_at, processing_completed_at) WHERE processing_completed_at IS NOT NULL`
        ];
    }

    /**
     * Log table creation in metadata table
     */
    async logTableCreation(tableName, interfaceId, transaction) {
        const metadataSQL = `
            INSERT INTO interface_table_metadata (
                interface_id, table_name, created_at,
                schema_version, estimated_rows, last_maintenance_at
            ) VALUES (
                :interfaceId, :tableName, NOW(),
                '1.0', 0, NOW()
            )
            ON CONFLICT (interface_id) DO UPDATE SET
                table_name = EXCLUDED.table_name,
                updated_at = NOW()
        `;

        await this.database.sequelize.query(metadataSQL, {
            replacements: { interfaceId, tableName },
            transaction
        });
    }

    /**
     * PERFORMANCE: Insert message into interface-specific table
     */
    async insertMessage(interfaceId, messageData) {
        const tableName = await this.ensureInterfaceTable(interfaceId, messageData.interfaceName || 'Unknown');

        // Clean development schema - only new format fields (legacy source is now nullable)
        const insertSQL = `
            INSERT INTO ${tableName} (
                message_id, correlation_id, interface_id, status, priority,
                source_type, source_endpoint, source_ip,
                message_type, message_size, message_encoding,
                raw_message, received_at
            ) VALUES (
                :message_id, :correlation_id, :interface_id, :status, :priority,
                :source_type, :source_endpoint, :source_ip,
                :message_type, :message_size, :message_encoding,
                :raw_message, :received_at
            )
            RETURNING id
        `;

        const replacements = {
            message_id: messageData.messageId,
            correlation_id: messageData.correlationId,
            interface_id: messageData.interfaceId,
            status: messageData.status || 'received',
            priority: messageData.priority || 5,
            source_type: messageData.sourceType,
            source_endpoint: messageData.sourceEndpoint,
            source_ip: messageData.sourceIP,
            message_type: messageData.messageType,
            message_size: messageData.messageSize,
            message_encoding: messageData.messageEncoding || 'UTF-8',
            raw_message: messageData.rawMessage,
            received_at: messageData.receivedAt || new Date()
        };

        const result = await this.database.sequelize.query(insertSQL, {
            replacements,
            type: this.database.sequelize.QueryTypes.INSERT
        });

        console.log(`📧 Message inserted into ${tableName}: ${messageData.messageId}`);
        return result[0][0].id;
    }

    /**
     * PERFORMANCE: Query messages from interface-specific table
     */
    async getInterfaceMessages(interfaceId, options = {}) {
        const tableName = this.getInterfaceTableName(interfaceId);

        // Extract page for early return
        const { page = 1 } = options;

        // Verify table exists
        if (!await this.checkTableExists(tableName)) {
            console.log(`ℹ️ Table ${tableName} does not exist, returning empty results`);
            return {
                messages: [],
                pagination: {
                    currentPage: parseInt(page),
                    totalPages: 0,
                    totalCount: 0,
                    hasNextPage: false,
                    hasPreviousPage: false
                },
                tableName, // Include for consistency with successful queries
                performance: {
                    queryTime: new Date().toISOString(),
                    tableRowsQueried: 0,
                    isolatedTable: false,
                    tableExists: false
                }
            };
        }

        const {
            limit = 50,
            status,
            messageType,
            dateFrom,
            dateTo,
            sortBy = 'received_at',
            sortOrder = 'DESC'
        } = options;

        const offset = (page - 1) * limit;
        let whereConditions = [];
        let replacements = { limit: parseInt(limit), offset: parseInt(offset) };

        // Build dynamic filters
        if (status) {
            whereConditions.push('status = :status');
            replacements.status = status;
        }

        if (messageType) {
            whereConditions.push('message_type ILIKE :messageType');
            replacements.messageType = `%${messageType}%`;
        }

        if (dateFrom) {
            whereConditions.push('received_at >= :dateFrom');
            replacements.dateFrom = dateFrom;
        }

        if (dateTo) {
            whereConditions.push('received_at <= :dateTo');
            replacements.dateTo = dateTo;
        }

        const whereClause = whereConditions.length > 0 ? `WHERE ${whereConditions.join(' AND ')}` : '';
        const validSortColumns = ['received_at', 'message_id', 'status', 'message_type', 'processing_time_ms'];
        const finalSortBy = validSortColumns.includes(sortBy) ? sortBy : 'received_at';
        const finalSortOrder = ['ASC', 'DESC'].includes(sortOrder.toUpperCase()) ? sortOrder.toUpperCase() : 'DESC';

        // Standard schema - all interface tables use the same format
        let selectColumns = 'id, message_id, correlation_id, status, priority, message_type, message_size, received_at, source_type, source_endpoint, processing_completed_at, processing_time_ms, parsed_at, parsing_time_ms, error_count, last_error_message, delivery_status, delivery_attempts';

        // PERFORMANCE QUERY: Only this interface's table
        const messagesQuery = `
            SELECT ${selectColumns}
            FROM ${tableName}
            ${whereClause}
            ORDER BY ${finalSortBy} ${finalSortOrder}
            LIMIT :limit OFFSET :offset
        `;

        const countQuery = `
            SELECT COUNT(*) as total
            FROM ${tableName}
            ${whereClause}
        `;

        const [messages, countResult] = await Promise.all([
            this.database.sequelize.query(messagesQuery, {
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
            tableName, // For debugging/monitoring
            performance: {
                queryTime: new Date().toISOString(),
                tableRowsQueried: totalCount,
                isolatedTable: true
            }
        };
    }

    /**
     * MAINTENANCE: Get interface table statistics
     */
    async getInterfaceTableStats(interfaceId) {
        const tableName = this.getInterfaceTableName(interfaceId);

        if (!await this.checkTableExists(tableName)) {
            return null;
        }

        const statsQuery = `
            SELECT
                COUNT(*) as total_messages,
                COUNT(CASE WHEN status IN ('sent', 'delivered') THEN 1 END) as successful_messages,
                COUNT(CASE WHEN status IN ('failed', 'error') THEN 1 END) as failed_messages,
                COUNT(CASE WHEN status IN ('received', 'processing') THEN 1 END) as processing_messages,
                AVG(processing_time_ms) as avg_processing_time,
                MAX(received_at) as last_message_at,
                MIN(received_at) as first_message_at,
                COUNT(DISTINCT message_type) as unique_message_types,
                pg_total_relation_size('${tableName}') as table_size_bytes
            FROM ${tableName}
        `;

        const result = await this.database.sequelize.query(statsQuery, {
            type: this.database.sequelize.QueryTypes.SELECT
        });

        return {
            ...result[0],
            tableName,
            interfaceId
        };
    }

    /**
     * ADMIN: List all interface tables
     */
    async listInterfaceTables() {
        const query = `
            SELECT
                table_name,
                pg_total_relation_size(table_name) as size_bytes,
                (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE table_name = t.table_name) as exists
            FROM interface_table_metadata t
            ORDER BY created_at DESC
        `;

        return await this.database.sequelize.query(query, {
            type: this.database.sequelize.QueryTypes.SELECT
        });
    }
}

module.exports = new InterfaceTableManager();