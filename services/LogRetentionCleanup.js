// services/LogRetentionCleanup.js
// Automated cleanup service for processing logs based on interface retention policies

const cron = require('node-cron');
const { MongoClient } = require('mongodb');

class LogRetentionCleanupService {
    constructor() {
        this.database = require('../config/database');
        this.mongoUrl = process.env.MONGODB_URL || 'mongodb://localhost:27017';
        this.cronJob = null;
        this.isRunning = false;
    }

    /**
     * Start the cleanup cron job (runs daily at 2 AM)
     */
    start() {
        if (this.cronJob) {
            console.log('⚠️ Log retention cleanup job already running');
            return;
        }

        // Run daily at 2 AM
        this.cronJob = cron.schedule('0 2 * * *', async () => {
            await this.executeCleanup();
        });

        console.log('✅ Log retention cleanup job started (runs daily at 2 AM)');
    }

    /**
     * Stop the cleanup cron job
     */
    stop() {
        if (this.cronJob) {
            this.cronJob.stop();
            this.cronJob = null;
            console.log('🛑 Log retention cleanup job stopped');
        }
    }

    /**
     * Execute cleanup for all interfaces
     */
    async executeCleanup() {
        if (this.isRunning) {
            console.log('⚠️ Cleanup already in progress, skipping this run');
            return;
        }

        this.isRunning = true;
        const startTime = Date.now();

        try {
            console.log('🧹 Starting log retention cleanup...');

            // Get all active interfaces with their retention settings
            const interfaces = await this.database.sequelize.query(`
                SELECT
                    id,
                    name,
                    debug_logging,
                    log_retention_days,
                    retain_error_logs_forever
                FROM interfaces
                WHERE status IN ('active', 'testing', 'configured')
                ORDER BY id
            `, {
                type: this.database.sequelize.QueryTypes.SELECT
            });

            console.log(`📋 Found ${interfaces.length} interfaces to process`);

            let totalDeleted = 0;
            const client = new MongoClient(this.mongoUrl);

            try {
                await client.connect();
                const db = client.db('ezhealthkonnect');

                for (const interface of interfaces) {
                    const deleted = await this.cleanupInterfaceLogs(db, interface);
                    totalDeleted += deleted;
                }

            } finally {
                await client.close();
            }

            const duration = Date.now() - startTime;
            console.log(`✅ Cleanup completed: ${totalDeleted} logs deleted in ${duration}ms`);

        } catch (error) {
            console.error('❌ Log retention cleanup failed:', error);
        } finally {
            this.isRunning = false;
        }
    }

    /**
     * Cleanup logs for a single interface
     */
    async cleanupInterfaceLogs(db, interface) {
        try {
            const collectionName = `processing_logs_intf_${interface.id.replace(/-/g, '_')}`;
            const collection = db.collection(collectionName);

            // Check if collection exists
            const collections = await db.listCollections({ name: collectionName }).toArray();
            if (collections.length === 0) {
                // Collection doesn't exist yet, skip
                return 0;
            }

            const retentionDays = interface.log_retention_days || 30;
            const cutoffDate = new Date();
            cutoffDate.setDate(cutoffDate.getDate() - retentionDays);

            let deleteFilter;

            if (interface.retain_error_logs_forever) {
                // Delete only non-error logs older than retention period
                deleteFilter = {
                    timestamp: { $lt: cutoffDate },
                    log_level: { $nin: ['error', 'warning'] }
                };
            } else {
                // Delete all logs older than retention period
                deleteFilter = {
                    timestamp: { $lt: cutoffDate }
                };
            }

            const result = await collection.deleteMany(deleteFilter);

            if (result.deletedCount > 0) {
                console.log(`   🧹 ${interface.name}: Deleted ${result.deletedCount} logs (retention: ${retentionDays} days)`);
            }

            return result.deletedCount;

        } catch (error) {
            console.error(`   ❌ Failed to cleanup logs for ${interface.name}:`, error.message);
            return 0;
        }
    }

    /**
     * Manual cleanup trigger (for testing or admin use)
     */
    async manualCleanup() {
        console.log('🔧 Manual cleanup triggered');
        await this.executeCleanup();
    }

    /**
     * Get cleanup statistics
     */
    async getStatistics() {
        try {
            const client = new MongoClient(this.mongoUrl);

            try {
                await client.connect();
                const db = client.db('ezhealthkonnect');

                // Get all interface log collections
                const collections = await db.listCollections().toArray();
                const logCollections = collections.filter(c => c.name.startsWith('processing_logs_intf_'));

                const stats = [];

                for (const col of logCollections) {
                    const collection = db.collection(col.name);
                    const count = await collection.countDocuments();
                    const oldestLog = await collection.findOne({}, { sort: { timestamp: 1 } });
                    const newestLog = await collection.findOne({}, { sort: { timestamp: -1 } });

                    stats.push({
                        collection: col.name,
                        total_logs: count,
                        oldest_log: oldestLog?.timestamp,
                        newest_log: newestLog?.timestamp
                    });
                }

                return stats;

            } finally {
                await client.close();
            }

        } catch (error) {
            console.error('❌ Failed to get statistics:', error);
            return [];
        }
    }
}

// Singleton instance
let instance = null;

function getInstance() {
    if (!instance) {
        instance = new LogRetentionCleanupService();
    }
    return instance;
}

module.exports = {
    LogRetentionCleanupService,
    getInstance,
    // Convenience methods
    start: () => getInstance().start(),
    stop: () => getInstance().stop(),
    manualCleanup: () => getInstance().manualCleanup(),
    getStatistics: () => getInstance().getStatistics()
};
