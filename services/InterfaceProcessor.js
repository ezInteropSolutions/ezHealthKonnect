// services/InterfaceProcessor.js
// Individual Interface Processing Worker

const EventEmitter = require('events');
const { v4: uuidv4 } = require('uuid');

class InterfaceProcessor extends EventEmitter {
    constructor(interfaceConfig, services) {
        super();
        this.config = interfaceConfig;
        this.services = services;
        this.isActive = false;
        this.processedCount = 0;
        this.lastActivity = null;

        // Initialize connectors based on config
        this.inputConnector = this.createInputConnector();
        this.outputConnector = this.createOutputConnector();
        this.transformEngine = this.createTransformEngine();

        this.setupDataLineage();
    }

    async start() {
        try {
            console.log(`🔄 Starting processor for interface: ${this.config.name}`);

            // Initialize input connector
            await this.inputConnector.connect();

            // Set up message processing pipeline
            this.inputConnector.on('message', this.processMessage.bind(this));
            this.inputConnector.on('error', this.handleConnectorError.bind(this));

            // Start listening for messages
            await this.inputConnector.startListening();

            this.isActive = true;
            this.emit('processor:started', { interfaceId: this.config.id });

            console.log(`✅ Processor started for: ${this.config.name}`);

        } catch (error) {
            console.error(`❌ Failed to start processor for ${this.config.name}:`, error);
            throw error;
        }
    }

    /**
     * Main message processing pipeline
     */
    async processMessage(rawMessage, messageContext = {}) {
        const messageId = uuidv4();
        const startTime = Date.now();

        try {
            console.log(`📨 Processing message ${messageId} for interface ${this.config.name}`);

            // 1. Data Lineage: Record message ingestion
            await this.recordDataLineage(messageId, 'INGESTION', {
                source: this.config.sourceConfig,
                rawMessageSize: Buffer.byteLength(rawMessage),
                messageContext
            });

            // 2. Determine message type
            const messageType = await this.detectMessageType(rawMessage);

            // 3. Get appropriate mapping configuration
            const mappingConfig = this.getMappingForMessageType(messageType);

            if (!mappingConfig) {
                throw new Error(`No mapping configuration found for message type: ${messageType}`);
            }

            // 4. Transform message
            const transformedMessage = await this.transformEngine.transform(
                rawMessage,
                mappingConfig,
                { messageId, messageType }
            );

            // 5. Data Lineage: Record transformation
            await this.recordDataLineage(messageId, 'TRANSFORMATION', {
                messageType,
                mappingUsed: mappingConfig.templateName || 'custom',
                transformationSize: Buffer.byteLength(JSON.stringify(transformedMessage))
            });

            // 6. Output to destination
            const outputResult = await this.outputConnector.send(transformedMessage, {
                messageId,
                messageType,
                sourceInterface: this.config.id
            });

            // 7. Data Lineage: Record delivery
            await this.recordDataLineage(messageId, 'DELIVERY', {
                destination: this.config.targetConfig,
                outputResult,
                processingTime: Date.now() - startTime
            });

            // 8. Update statistics
            this.processedCount++;
            this.lastActivity = new Date();

            // 9. Record success metrics
            this.services.monitoring.recordSuccess(this.config.id, messageId);
            this.services.monitoring.recordProcessingTime(
                this.config.id,
                messageType,
                Date.now() - startTime
            );

            console.log(`✅ Message ${messageId} processed successfully in ${Date.now() - startTime}ms`);

        } catch (error) {
            // Error handling with data lineage
            await this.recordDataLineage(messageId, 'ERROR', {
                error: error.message,
                processingTime: Date.now() - startTime
            });

            const action = await this.services.errorHandler.handleProcessingError(
                this.config.id,
                messageId,
                error,
                { rawMessage: rawMessage.substring(0, 1000), messageContext }
            );

            this.services.monitoring.recordError(this.config.id, error, {
                messageId,
                action
            });

            if (action === 'RETRY') {
                // Add to retry queue with exponential backoff
                await this.scheduleRetry(messageId, rawMessage, messageContext);
            } else {
                // Send to Dead Letter Queue
                await this.sendToDeadLetterQueue(messageId, rawMessage, error);
            }

            console.error(`💥 Message ${messageId} processing failed:`, error.message);
        }
    }

    /**
     * Create appropriate input connector based on configuration
     */
    createInputConnector() {
        const ConnectorFactory = require('./connectors/ConnectorFactory');

        return ConnectorFactory.createInputConnector(
            this.config.sourceType,
            this.config.sourceConfig
        );
    }

    /**
     * Create appropriate output connector based on configuration
     */
    createOutputConnector() {
        const ConnectorFactory = require('./connectors/ConnectorFactory');

        return ConnectorFactory.createOutputConnector(
            this.config.targetType,
            this.config.targetConfig
        );
    }

    /**
     * Create transformation engine
     */
    createTransformEngine() {
        const HL7ToFHIRTransformer = require('./transformers/HL7ToFHIRTransformer');

        return new HL7ToFHIRTransformer();
    }

    /**
     * Detect message type from content
     */
    async detectMessageType(rawMessage) {
        // Simple HL7 message type detection
        if (rawMessage.startsWith('MSH')) {
            const segments = rawMessage.split('\r');
            const mshSegment = segments[0];
            const fields = mshSegment.split('|');
            if (fields.length > 8) {
                return fields[8]; // Message type field (MSH.9)
            }
        }

        // Default fallback
        return 'ADT^A01';
    }

    /**
     * Get mapping configuration for specific message type
     */
    getMappingForMessageType(messageType) {
        const mappings = this.config.message_mappings || [];

        return mappings.find(mapping =>
            mapping.messageType === messageType
        ) || mappings.find(mapping =>
            mapping.messageType === 'ADT^A01' // Default fallback
        );
    }

    /**
     * Data lineage tracking
     */
    setupDataLineage() {
        this.dataLineageTracker = {
            messageTrails: new Map() // messageId -> lineage events
        };
    }

    async recordDataLineage(messageId, event, metadata) {
        const lineageEvent = {
            messageId,
            interfaceId: this.config.id,
            interfaceName: this.config.name,
            event,
            timestamp: new Date(),
            metadata
        };

        // Store in MongoDB for detailed lineage
        // await mongoDb.collection('message_lineage').insertOne(lineageEvent);

        // Store in PostgreSQL for audit
        const { Pool } = require('pg');
        const pool = new Pool();

        await pool.query(
            `INSERT INTO message_audit_log
            (message_id, interface_id, event_type, event_data, created_at)
            VALUES ($1, $2, $3, $4, $5)`,
            [messageId, this.config.id, event, JSON.stringify(lineageEvent), new Date()]
        );

        // Cache recent lineage in Redis
        // await redis.setex(`lineage:${messageId}:${event}`, 3600, JSON.stringify(lineageEvent));

        console.log(`📋 Lineage recorded: ${messageId} -> ${event}`);
    }

    async scheduleRetry(messageId, rawMessage, context) {
        const retryJob = {
            messageId,
            interfaceId: this.config.id,
            rawMessage,
            context,
            attempt: (context.attempt || 0) + 1,
            maxAttempts: 3,
            delay: Math.pow(2, context.attempt || 0) * 1000 // Exponential backoff
        };

        if (retryJob.attempt <= retryJob.maxAttempts) {
            await this.services.messageQueue.add(
                'retry-message',
                retryJob,
                { delay: retryJob.delay }
            );

            console.log(`🔄 Message ${messageId} scheduled for retry (attempt ${retryJob.attempt})`);
        } else {
            await this.sendToDeadLetterQueue(messageId, rawMessage, new Error('Max retries exceeded'));
        }
    }

    async sendToDeadLetterQueue(messageId, rawMessage, error) {
        const dlqEntry = {
            messageId,
            interfaceId: this.config.id,
            rawMessage,
            error: error.message,
            timestamp: new Date(),
            requiresManualReview: true
        };

        // Store in MongoDB DLQ collection
        // await mongoDb.collection('dead_letter_queue').insertOne(dlqEntry);

        console.log(`💀 Message ${messageId} sent to Dead Letter Queue`);
    }

    async stop() {
        console.log(`⏸️ Stopping processor for interface: ${this.config.name}`);

        this.isActive = false;

        if (this.inputConnector) {
            await this.inputConnector.disconnect();
        }

        if (this.outputConnector) {
            await this.outputConnector.disconnect();
        }

        this.emit('processor:stopped', { interfaceId: this.config.id });

        console.log(`✅ Processor stopped for: ${this.config.name}`);
    }

    getStatus() {
        return {
            isActive: this.isActive,
            processedCount: this.processedCount,
            lastActivity: this.lastActivity
        };
    }

    getProcessedCount() {
        return this.processedCount;
    }

    getLastActivity() {
        return this.lastActivity;
    }

    handleConnectorError(error) {
        console.error(`🔌 Connector error for interface ${this.config.name}:`, error);
        this.emit('connector:error', { interfaceId: this.config.id, error });
    }
}

module.exports = InterfaceProcessor;