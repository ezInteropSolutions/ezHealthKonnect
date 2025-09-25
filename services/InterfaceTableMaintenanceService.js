// services/InterfaceTableMaintenanceService.js
// Interface-specific table maintenance service

class InterfaceTableMaintenanceService {
    constructor() {
        this.database = require('../config/database');
        this.tableManager = require('./InterfaceTableManager');
        this.maintenanceInterval = null;
        this.isRunning = false;
    }

    /**
     * Start automated maintenance service for interface tables
     */
    async startMaintenanceService() {
        if (this.isRunning) {
            console.log('⚠️ Maintenance service already running');
            return;
        }

        console.log('🔧 Starting Interface Table Maintenance Service...');
        this.isRunning = true;

        // Run maintenance every 6 hours
        this.maintenanceInterval = setInterval(async () => {
            await this.runMaintenanceCycle();
        }, 6 * 60 * 60 * 1000); // 6 hours

        // Run initial maintenance on startup
        setTimeout(async () => {
            await this.runMaintenanceCycle();
        }, 30000); // Wait 30 seconds after startup

        console.log('✅ Interface Table Maintenance Service started');
    }

    /**
     * Stop maintenance service
     */
    stopMaintenanceService() {
        if (this.maintenanceInterval) {
            clearInterval(this.maintenanceInterval);
            this.maintenanceInterval = null;
        }
        this.isRunning = false;
        console.log('🛑 Interface Table Maintenance Service stopped');
    }

    /**
     * Run complete maintenance cycle for all interface tables
     */
    async runMaintenanceCycle() {
        console.log('🔧 Starting maintenance cycle for all interface tables...');

        try {
            // Get all interfaces with dedicated tables
            const interfaces = await this.getInterfacesWithDedicatedTables();

            console.log(`📊 Found ${interfaces.length} interfaces with dedicated tables`);

            for (const interfaceInfo of interfaces) {
                await this.maintainInterfaceTable(interfaceInfo);
            }

            // Update global maintenance log
            await this.logMaintenanceCycle(interfaces.length);

            console.log('✅ Maintenance cycle completed successfully');

        } catch (error) {
            console.error('❌ Maintenance cycle failed:', error);
            await this.logMaintenanceError(error);
        }
    }

    /**
     * Get all interfaces using dedicated table strategy
     */
    async getInterfacesWithDedicatedTables() {
        const query = `
            SELECT
                i.id,
                i.name,
                i.use_dedicated_table,
                i.table_management_strategy,
                itm.table_name,
                itm.estimated_rows,
                itm.last_maintenance_at
            FROM interfaces i
            LEFT JOIN interface_table_metadata itm ON i.id = itm.interface_id
            WHERE i.use_dedicated_table = true
            AND i.table_management_strategy = 'dedicated'
            AND i.status IN ('active', 'testing', 'configured')
            ORDER BY itm.estimated_rows DESC
        `;

        return await this.database.sequelize.query(query, {
            type: this.database.sequelize.QueryTypes.SELECT
        });
    }

    /**
     * Maintain specific interface table
     */
    async maintainInterfaceTable(interfaceInfo) {
        const { id: interfaceId, name: interfaceName, table_name: tableName } = interfaceInfo;

        console.log(`🔧 Maintaining table for interface: ${interfaceName} (${tableName})`);

        try {
            // Step 1: Update table statistics
            await this.updateTableStatistics(interfaceId);

            // Step 2: Analyze table performance
            const performance = await this.analyzeTablePerformance(interfaceId, tableName);

            // Step 3: Cleanup old messages (if configured)
            const cleanupResult = await this.cleanupOldMessages(interfaceId, performance);

            // Step 4: Optimize table (reindex if needed)
            await this.optimizeTable(tableName, performance);

            // Step 5: Check table health
            const healthCheck = await this.checkTableHealth(tableName);

            console.log(`✅ Maintenance completed for ${interfaceName}:`, {
                rows: performance.rowCount,
                sizeMB: Math.round(performance.tableSizeBytes / (1024 * 1024)),
                cleaned: cleanupResult.deletedCount,
                health: healthCheck.status
            });

        } catch (error) {
            console.error(`❌ Failed to maintain table for ${interfaceName}:`, error);
            await this.logInterfaceMaintenanceError(interfaceId, error);
        }
    }

    /**
     * Update table statistics using PostgreSQL function
     */
    async updateTableStatistics(interfaceId) {
        await this.database.sequelize.query(`SELECT update_interface_table_stats(:interfaceId)`, {
            replacements: { interfaceId },
            type: this.database.sequelize.QueryTypes.SELECT
        });
    }

    /**
     * Analyze table performance metrics
     */
    async analyzeTablePerformance(interfaceId, tableName) {
        const query = `
            SELECT
                COUNT(*) as row_count,
                pg_total_relation_size(:tableName) as table_size_bytes,
                pg_size_pretty(pg_total_relation_size(:tableName)) as table_size_human,

                -- Message age analysis
                MIN(received_at) as oldest_message,
                MAX(received_at) as newest_message,

                -- Status distribution
                COUNT(CASE WHEN status IN ('failed', 'error') THEN 1 END) as error_count,
                COUNT(CASE WHEN status = 'sent' THEN 1 END) as success_count,

                -- Performance metrics
                AVG(processing_time_ms) as avg_processing_time,
                MAX(processing_time_ms) as max_processing_time,

                -- Index usage (estimated)
                CASE
                    WHEN COUNT(*) > 100000 THEN 'high_volume'
                    WHEN COUNT(*) > 10000 THEN 'medium_volume'
                    ELSE 'low_volume'
                END as volume_category
            FROM ${tableName}
        `;

        const result = await this.database.sequelize.query(query, {
            replacements: { tableName },
            type: this.database.sequelize.QueryTypes.SELECT
        });

        return result[0];
    }

    /**
     * Cleanup old messages based on retention policy
     */
    async cleanupOldMessages(interfaceId, performance) {
        // Determine retention period based on volume
        let retentionDays;
        switch (performance.volume_category) {
            case 'high_volume':
                retentionDays = 30; // 1 month for high-volume interfaces
                break;
            case 'medium_volume':
                retentionDays = 90; // 3 months for medium-volume
                break;
            default:
                retentionDays = 180; // 6 months for low-volume
        }

        // Check if cleanup is needed (only if > 80% of retention period)
        const daysSinceOldest = performance.oldest_message ?
            (new Date() - new Date(performance.oldest_message)) / (1000 * 60 * 60 * 24) : 0;

        if (daysSinceOldest > retentionDays * 0.8) {
            console.log(`🗑️ Cleaning up messages older than ${retentionDays} days for interface ${interfaceId}`);

            const deletedCount = await this.database.sequelize.query(
                `SELECT cleanup_interface_table_messages(:interfaceId, :retentionDays)`,
                {
                    replacements: { interfaceId, retentionDays },
                    type: this.database.sequelize.QueryTypes.SELECT
                }
            );

            return { deletedCount: deletedCount[0].cleanup_interface_table_messages };
        }

        return { deletedCount: 0 };
    }

    /**
     * Optimize table performance (reindex if needed)
     */
    async optimizeTable(tableName, performance) {
        // Reindex if table is large and hasn't been optimized recently
        if (performance.row_count > 50000) {
            console.log(`🔧 Reindexing table ${tableName} for optimal performance`);

            // Reindex all indexes for this table
            await this.database.sequelize.query(`REINDEX TABLE ${tableName}`);

            // Update table statistics
            await this.database.sequelize.query(`ANALYZE ${tableName}`);
        }
    }

    /**
     * Check table health status
     */
    async checkTableHealth(tableName) {
        try {
            // Check if table exists and is accessible
            const exists = await this.database.sequelize.query(`
                SELECT EXISTS (
                    SELECT FROM information_schema.tables
                    WHERE table_name = :tableName
                )
            `, {
                replacements: { tableName },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (!exists[0].exists) {
                return { status: 'missing', message: 'Table does not exist' };
            }

            // Check for any table corruption or issues
            const corruptionCheck = await this.database.sequelize.query(`
                SELECT COUNT(*) as test_count FROM ${tableName} LIMIT 1
            `, {
                type: this.database.sequelize.QueryTypes.SELECT
            });

            return {
                status: 'healthy',
                message: 'Table is accessible and functional',
                testQuery: true
            };

        } catch (error) {
            return {
                status: 'error',
                message: `Table health check failed: ${error.message}`,
                error: error.message
            };
        }
    }

    /**
     * Log maintenance cycle completion
     */
    async logMaintenanceCycle(tableCount) {
        await this.database.sequelize.query(`
            INSERT INTO audit_logs (
                user_id, action, resource_type, resource_id,
                details, created_at
            ) VALUES (
                NULL, 'maintenance_cycle', 'interface_tables', NULL,
                :details, CURRENT_TIMESTAMP
            )
        `, {
            replacements: {
                details: JSON.stringify({
                    tables_maintained: tableCount,
                    cycle_completed_at: new Date().toISOString(),
                    service_type: 'automated_maintenance'
                })
            },
            type: this.database.sequelize.QueryTypes.INSERT
        });
    }

    /**
     * Log maintenance errors
     */
    async logMaintenanceError(error) {
        await this.database.sequelize.query(`
            INSERT INTO audit_logs (
                user_id, action, resource_type, resource_id,
                details, created_at
            ) VALUES (
                NULL, 'maintenance_error', 'interface_tables', NULL,
                :details, CURRENT_TIMESTAMP
            )
        `, {
            replacements: {
                details: JSON.stringify({
                    error_message: error.message,
                    error_stack: error.stack,
                    cycle_failed_at: new Date().toISOString()
                })
            },
            type: this.database.sequelize.QueryTypes.INSERT
        });
    }

    /**
     * Log interface-specific maintenance errors
     */
    async logInterfaceMaintenanceError(interfaceId, error) {
        await this.database.sequelize.query(`
            INSERT INTO audit_logs (
                user_id, action, resource_type, resource_id,
                details, created_at
            ) VALUES (
                NULL, 'interface_maintenance_error', 'interface', :interfaceId,
                :details, CURRENT_TIMESTAMP
            )
        `, {
            replacements: {
                interfaceId,
                details: JSON.stringify({
                    error_message: error.message,
                    maintenance_failed_at: new Date().toISOString()
                })
            },
            type: this.database.sequelize.QueryTypes.INSERT
        });
    }

    /**
     * Manual maintenance trigger (for admin use)
     */
    async runManualMaintenance(interfaceId) {
        console.log(`🔧 Running manual maintenance for interface: ${interfaceId}`);

        const interfaces = await this.database.sequelize.query(`
            SELECT
                i.id, i.name, i.use_dedicated_table, i.table_management_strategy,
                itm.table_name, itm.estimated_rows, itm.last_maintenance_at
            FROM interfaces i
            LEFT JOIN interface_table_metadata itm ON i.id = itm.interface_id
            WHERE i.id = :interfaceId
        `, {
            replacements: { interfaceId },
            type: this.database.sequelize.QueryTypes.SELECT
        });

        if (interfaces.length === 0) {
            throw new Error(`Interface ${interfaceId} not found`);
        }

        await this.maintainInterfaceTable(interfaces[0]);
        console.log(`✅ Manual maintenance completed for interface: ${interfaceId}`);
    }

    /**
     * Get maintenance status report
     */
    async getMaintenanceReport() {
        const query = `
            SELECT
                i.name as interface_name,
                itm.table_name,
                itm.estimated_rows,
                itm.last_maintenance_at,
                pg_size_pretty(pg_total_relation_size(itm.table_name)) as table_size,
                CASE
                    WHEN itm.last_maintenance_at > NOW() - INTERVAL '1 day' THEN 'recent'
                    WHEN itm.last_maintenance_at > NOW() - INTERVAL '7 days' THEN 'weekly'
                    WHEN itm.last_maintenance_at > NOW() - INTERVAL '30 days' THEN 'monthly'
                    ELSE 'overdue'
                END as maintenance_status
            FROM interfaces i
            INNER JOIN interface_table_metadata itm ON i.id = itm.interface_id
            WHERE i.use_dedicated_table = true
            ORDER BY itm.last_maintenance_at DESC
        `;

        return await this.database.sequelize.query(query, {
            type: this.database.sequelize.QueryTypes.SELECT
        });
    }
}

module.exports = new InterfaceTableMaintenanceService();