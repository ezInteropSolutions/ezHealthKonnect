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

        // Start message queue service
        if (this.messageQueue && typeof this.messageQueue.start === 'function') {
            await this.messageQueue.start();
        }
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
        const database = require('../config/database');
        const sequelize = database.sequelize;

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
            WHERE i.id = :interface_id AND i.interface_status IN ('draft', 'configured', 'active', 'paused', 'error')
            GROUP BY i.id
        `;

        const result = await sequelize.query(query, {
            replacements: { interface_id: interfaceId },
            type: sequelize.QueryTypes.SELECT
        });

        if (result.length === 0) {
            throw new Error(`Interface not found or not ready for activation: ${interfaceId}`);
        }

        return result[0];
    }

    async loadActiveInterfaces() {
        const database = require('../config/database');
        const sequelize = database.sequelize;

        const result = await sequelize.query(
            "SELECT id, name FROM interfaces WHERE interface_status = 'active'",
            { type: sequelize.QueryTypes.SELECT }
        );

        return result;
    }

    async updateInterfaceStatus(interfaceId, status) {
        const database = require('../config/database');
        const sequelize = database.sequelize;

        await sequelize.query(
            'UPDATE interfaces SET interface_status = :status, last_status_update = CURRENT_TIMESTAMP WHERE id = :interface_id',
            {
                replacements: { status, interface_id: interfaceId },
                type: sequelize.QueryTypes.UPDATE
            }
        );
    }

    async setupMessageQueue() {
        const MessageQueueService = require('./MessageQueueService');
        return new MessageQueueService();
    }

    async setupMonitoring() {
        const MonitoringService = require('./MonitoringService');
        return new MonitoringService();
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

/**
 * Individual Interface Processing Worker
 * Handles message ingestion, transformation, and delivery for one interface
 */
class InterfaceProcessor extends EventEmitter {
    constructor(interfaceConfig, services) {
        super();
        this.config = interfaceConfig;
        this.messageQueue = services.messageQueue;
        this.monitoring = services.monitoring;
        this.errorHandler = services.errorHandler;

        this.isRunning = false;
        this.processedCount = 0;
        this.lastActivity = new Date();

        this.inputConnector = null;
        this.outputConnector = null;
        this.transformer = null;
    }

    async start() {
        console.log(`🔄 Starting processor for interface: ${this.config.name}`);

        // Initialize connectors based on interface configuration
        this.inputConnector = await this.createInputConnector();
        this.outputConnector = await this.createOutputConnector();
        this.transformer = await this.createTransformer();

        // Start message ingestion
        await this.inputConnector.start();

        this.isRunning = true;
        this.emit('processor:started', { interfaceId: this.config.id });
    }

    async stop() {
        console.log(`⏹️ Stopping processor for interface: ${this.config.name}`);

        if (this.inputConnector) {
            await this.inputConnector.stop();
        }

        this.isRunning = false;
        this.emit('processor:stopped', { interfaceId: this.config.id });
    }

    async createInputConnector() {
        const ConnectorFactory = require('./connectors/ConnectorFactory');
        const sourceConfig = JSON.parse(this.config.source_config || '{}');

        // Add message handler to configuration
        sourceConfig.messageHandler = this.onMessageReceived.bind(this);

        return ConnectorFactory.createInputConnector(
            this.config.source_connectivity,
            sourceConfig
        );
    }

    async createOutputConnector() {
        const ConnectorFactory = require('./connectors/ConnectorFactory');
        const targetConfig = JSON.parse(this.config.target_config || '{}');

        return ConnectorFactory.createOutputConnector(
            this.config.target_connectivity,
            targetConfig
        );
    }

    async createTransformer() {
        const TransformerFactory = require('./transformers/TransformerFactory');
        return TransformerFactory.createTransformer(
            this.config.source_type,
            this.config.target_type,
            this.config.message_mappings
        );
    }

    async onMessageReceived(messageData, messageId) {
        const startTime = Date.now();

        try {
            console.log(`📨 Processing message ${messageId} for interface ${this.config.name}`);

            // Create processing lineage record
            const lineageId = await this.createDataLineage(messageId);

            // Transform message using Go backend
            const transformedData = await this.transformer.transform(
                messageData,
                messageId,
                lineageId
            );

            // Deliver to target system
            const deliveryResult = await this.outputConnector.deliver(
                transformedData,
                messageId
            );

            // Update lineage with success
            await this.updateDataLineage(lineageId, 'completed', deliveryResult);

            // Record success metrics
            this.processedCount++;
            this.lastActivity = new Date();
            this.monitoring.recordSuccess(this.config.id, messageId);
            this.monitoring.recordProcessingTime(
                this.config.id,
                this.getMessageType(messageData),
                Date.now() - startTime
            );

            console.log(`✅ Successfully processed message ${messageId}`);

        } catch (error) {
            console.error(`❌ Failed to process message ${messageId}:`, error);

            // Handle error via error handler
            const decision = await this.errorHandler.handleProcessingError(
                this.config.id,
                messageId,
                error,
                { messageData, processingTime: Date.now() - startTime }
            );

            // Record error metrics
            this.monitoring.recordError(this.config.id, error, { messageId });

            // Handle retry or DLQ based on error handler decision
            if (decision === 'RETRY') {
                // Re-queue message for retry using MessageQueueService
                if (this.messageQueue && typeof this.messageQueue.enqueueMessage === 'function') {
                    await this.messageQueue.enqueueMessage(
                        this.config.id,
                        messageData,
                        {
                            messageId,
                            delay: 5000, // 5 second delay
                            maxAttempts: 3,
                            queueName: 'retry',
                            metadata: { originalError: error.message }
                        }
                    );
                }
            }
        }
    }

    async createDataLineage(messageId) {
        const lineageId = require('crypto').randomUUID();

        // Store in PostgreSQL for query performance
        const database = require('../config/database');
        const sequelize = database.sequelize;

        await sequelize.query(`
            INSERT INTO message_audit_log (
                id, interface_id, message_id, event_type,
                event_data, created_at
            ) VALUES (:id, :interface_id, :message_id, 'INGESTION', :event_data, :created_at)
        `, {
            replacements: {
                id: lineageId,
                interface_id: this.config.id,
                message_id: messageId,
                event_data: JSON.stringify({
                    source_system: this.config.source_type,
                    target_system: this.config.target_type,
                    processing_started: new Date()
                }),
                created_at: new Date()
            },
            type: sequelize.QueryTypes.INSERT
        });

        return lineageId;
    }

    async updateDataLineage(lineageId, status, details = {}) {
        const database = require('../config/database');
        const sequelize = database.sequelize;

        // Add a new audit log entry for the status update
        await sequelize.query(`
            INSERT INTO message_audit_log (
                id, interface_id, message_id, event_type, event_data, created_at
            )
            SELECT
                gen_random_uuid(),
                interface_id,
                message_id,
                :event_type,
                :event_data,
                :created_at
            FROM message_audit_log
            WHERE id = :lineage_id
        `, {
            replacements: {
                event_type: status,
                event_data: JSON.stringify(details),
                created_at: new Date(),
                lineage_id: lineageId
            },
            type: sequelize.QueryTypes.INSERT
        });
    }

    getMessageType(messageData) {
        // Extract message type from HL7 message
        if (typeof messageData === 'string' && messageData.startsWith('MSH')) {
            const segments = messageData.split('\r');
            const mshSegment = segments[0];
            const fields = mshSegment.split('|');
            return fields[8] || 'UNKNOWN'; // Message type is in MSH.9
        }
        return 'UNKNOWN';
    }

    getStatus() {
        return this.isRunning ? 'running' : 'stopped';
    }

    getProcessedCount() {
        return this.processedCount;
    }

    getLastActivity() {
        return this.lastActivity;
    }
}

module.exports = ProcessingEngineService;