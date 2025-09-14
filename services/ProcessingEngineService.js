// services/ProcessingEngineService.js
// Enterprise Runtime Processing Engine for Active Interfaces

const EventEmitter = require('events');

class ProcessingEngineService extends EventEmitter {
    constructor() {
        super();
        this.activeInterfaces = new Map(); // interfaceId -> ProcessingWorker
        this.messageQueue = null; // Redis-based queue
        this.monitoring = null;
        this.isRunning = false;

        this.initializeServices();
    }

    async initializeServices() {
        this.messageQueue = await this.setupMessageQueue();
        this.monitoring = await this.setupMonitoring();
        this.setupErrorHandling();
    }

    /**
     * Start processing engine - loads all active interfaces
     */
    async start() {
        console.log('🚀 Starting Processing Engine...');

        // Load all active interfaces from database
        const activeInterfaces = await this.loadActiveInterfaces();

        for (const interfaceConfig of activeInterfaces) {
            await this.activateInterface(interfaceConfig.id);
        }

        this.isRunning = true;
        this.emit('engine:started', { activeCount: activeInterfaces.length });

        console.log(`✅ Processing Engine started with ${activeInterfaces.length} active interfaces`);
    }

    /**
     * Activate a specific interface for processing
     */
    async activateInterface(interfaceId) {
        try {
            console.log(`🔄 Activating interface: ${interfaceId}`);

            // Get interface configuration with mappings
            const config = await this.getInterfaceConfig(interfaceId);

            // Create dedicated processing worker
            const worker = new InterfaceProcessor(config, {
                messageQueue: this.messageQueue,
                monitoring: this.monitoring,
                errorHandler: this.errorHandler
            });

            // Start the worker
            await worker.start();

            // Track active interface
            this.activeInterfaces.set(interfaceId, worker);

            // Update interface status
            await this.updateInterfaceStatus(interfaceId, 'active');

            this.emit('interface:activated', { interfaceId, config: config.name });

            console.log(`✅ Interface activated: ${config.name}`);

        } catch (error) {
            console.error(`❌ Failed to activate interface ${interfaceId}:`, error);
            await this.updateInterfaceStatus(interfaceId, 'error');
            throw error;
        }
    }

    /**
     * Deactivate interface processing
     */
    async deactivateInterface(interfaceId, reason = 'manual') {
        const worker = this.activeInterfaces.get(interfaceId);

        if (worker) {
            console.log(`⏸️ Deactivating interface: ${interfaceId} (${reason})`);

            await worker.stop();
            this.activeInterfaces.delete(interfaceId);

            await this.updateInterfaceStatus(interfaceId, 'paused');

            this.emit('interface:deactivated', { interfaceId, reason });
        }
    }

    /**
     * Get interface configuration with all mappings and connectors
     */
    async getInterfaceConfig(interfaceId) {
        const { Pool } = require('pg');
        const pool = new Pool(); // Use existing connection

        // Get interface details with mappings
        const query = `
            SELECT
                i.*,
                jsonb_agg(
                    jsonb_build_object(
                        'messageType', imm.message_type,
                        'usesStandardTemplate', imm.uses_standard_template,
                        'mappingConfig', CASE
                            WHEN imm.uses_standard_template THEN t.template_config
                            ELSE imm.custom_mapping_config
                        END,
                        'templateName', t.template_name
                    )
                ) as message_mappings
            FROM interfaces i
            LEFT JOIN interface_message_mappings imm ON i.id = imm.interface_id
            LEFT JOIN hl7_fhir_templates t ON imm.standard_template_id = t.id
            WHERE i.id = $1 AND i.interface_status = 'active'
            GROUP BY i.id
        `;

        const result = await pool.query(query, [interfaceId]);

        if (result.rows.length === 0) {
            throw new Error(`Active interface not found: ${interfaceId}`);
        }

        return result.rows[0];
    }

    async loadActiveInterfaces() {
        const { Pool } = require('pg');
        const pool = new Pool();

        const result = await pool.query(
            "SELECT id, name FROM interfaces WHERE interface_status = 'active'"
        );

        return result.rows;
    }

    async updateInterfaceStatus(interfaceId, status) {
        const { Pool } = require('pg');
        const pool = new Pool();

        await pool.query(
            'UPDATE interfaces SET interface_status = $1, last_status_update = CURRENT_TIMESTAMP WHERE id = $2',
            [status, interfaceId]
        );
    }

    async setupMessageQueue() {
        const Queue = require('bull');
        return new Queue('message processing', {
            redis: {
                host: process.env.REDIS_HOST || 'localhost',
                port: process.env.REDIS_PORT || 6379
            }
        });
    }

    async setupMonitoring() {
        return {
            recordProcessingTime: (interfaceId, messageType, duration) => {
                // Prometheus metrics
                console.log(`📊 ${interfaceId}/${messageType}: ${duration}ms`);
            },
            recordError: (interfaceId, error, context) => {
                console.error(`💥 ${interfaceId}: ${error.message}`, context);
            },
            recordSuccess: (interfaceId, messageId) => {
                console.log(`✅ ${interfaceId}: Processed ${messageId}`);
            }
        };
    }

    setupErrorHandling() {
        this.errorHandler = {
            handleProcessingError: async (interfaceId, messageId, error, context) => {
                // Log to MongoDB for detailed error analysis
                const errorDoc = {
                    interfaceId,
                    messageId,
                    error: error.message,
                    stack: error.stack,
                    context,
                    timestamp: new Date(),
                    severity: this.categorizeError(error)
                };

                // Store in MongoDB error collection
                // await mongoDb.collection('processing_errors').insertOne(errorDoc);

                // Update PostgreSQL audit trail
                // await this.logAuditEvent(interfaceId, messageId, 'PROCESSING_ERROR', errorDoc);

                // Decide on retry vs DLQ based on error type
                if (errorDoc.severity === 'RETRY') {
                    return 'RETRY';
                } else {
                    return 'DLQ';
                }
            }
        };
    }

    categorizeError(error) {
        if (error.message.includes('timeout') || error.message.includes('connection')) {
            return 'RETRY';
        }
        if (error.message.includes('validation') || error.message.includes('format')) {
            return 'DLQ';
        }
        return 'RETRY';
    }

    /**
     * Get processing statistics
     */
    getProcessingStats() {
        const stats = {
            engineStatus: this.isRunning ? 'running' : 'stopped',
            activeInterfaces: this.activeInterfaces.size,
            interfaceDetails: []
        };

        this.activeInterfaces.forEach((worker, interfaceId) => {
            stats.interfaceDetails.push({
                interfaceId,
                status: worker.getStatus(),
                messagesProcessed: worker.getProcessedCount(),
                lastActivity: worker.getLastActivity()
            });
        });

        return stats;
    }

    /**
     * Graceful shutdown
     */
    async shutdown() {
        console.log('🛑 Shutting down Processing Engine...');

        // Stop all interface workers
        const shutdownPromises = [];
        this.activeInterfaces.forEach((worker, interfaceId) => {
            shutdownPromises.push(this.deactivateInterface(interfaceId, 'shutdown'));
        });

        await Promise.all(shutdownPromises);

        this.isRunning = false;
        this.emit('engine:shutdown');

        console.log('✅ Processing Engine shutdown complete');
    }
}

module.exports = ProcessingEngineService;