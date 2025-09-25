// services/EnterpriseProcessingEngine.js
// Enterprise Processing Engine - Orchestrates the complete message processing pipeline
// Coordinates MessageQueue → Transformation → Routing → Delivery flow

const EventEmitter = require('events');
const MessageQueueService = require('./MessageQueueService');
const TransformationService = require('./TransformationService');
const RoutingService = require('./RoutingService');
const DeliveryService = require('./DeliveryService');
const InterfaceTableManager = require('./InterfaceTableManager');

class EnterpriseProcessingEngine extends EventEmitter {
    constructor() {
        super();

        // Initialize microservices
        this.messageQueue = new MessageQueueService();
        this.transformationService = new TransformationService();
        this.routingService = new RoutingService();
        this.deliveryService = new DeliveryService();

        // Engine state
        this.isRunning = false;
        this.processingWorkers = new Map(); // interfaceId -> worker
        this.processingInterval = null;

        // Configuration
        this.maxConcurrentProcessing = parseInt(process.env.MAX_CONCURRENT_PROCESSING) || 5;
        this.processingCheckInterval = parseInt(process.env.PROCESSING_CHECK_INTERVAL) || 2000; // 2 seconds

        // Performance metrics
        this.metrics = {
            messagesProcessed: 0,
            transformationSuccesses: 0,
            transformationFailures: 0,
            deliverySuccesses: 0,
            deliveryFailures: 0,
            avgPipelineTime: 0,
            startTime: null
        };

        this.setupEventHandlers();
    }

    /**
     * Setup event handlers between services
     */
    setupEventHandlers() {
        // Listen for new messages in queue
        process.on('messageQueued', (queuedMessage) => {
            this.processQueuedMessage(queuedMessage);
        });
    }

    /**
     * Start the enterprise processing engine
     */
    async start() {
        if (this.isRunning) {
            console.log('⚠️ EnterpriseProcessingEngine already running');
            return;
        }

        console.log('🚀 Starting Enterprise Processing Engine...');

        try {
            // Start the existing message queue service if it has a start method
            if (this.messageQueue && typeof this.messageQueue.start === 'function') {
                await this.messageQueue.start();
            }

            this.isRunning = true;
            this.metrics.startTime = new Date();

            // Start the main processing loop
            this.startProcessingLoop();

            console.log('✅ Enterprise Processing Engine started successfully');
            console.log(`🔧 Configuration: ${this.maxConcurrentProcessing} concurrent workers, ${this.processingCheckInterval}ms check interval`);

            this.emit('engine:started');

        } catch (error) {
            console.error('❌ Failed to start Enterprise Processing Engine:', error.message);
            throw error;
        }
    }

    /**
     * Stop the enterprise processing engine
     */
    async stop() {
        console.log('🛑 Stopping Enterprise Processing Engine...');

        this.isRunning = false;

        // Stop processing loop
        if (this.processingInterval) {
            clearInterval(this.processingInterval);
        }

        // Stop the message queue service if it has a stop method
        if (this.messageQueue && typeof this.messageQueue.stop === 'function') {
            await this.messageQueue.stop();
        }

        // Wait for active workers to complete
        await this.waitForWorkersToComplete();

        console.log('✅ Enterprise Processing Engine stopped');
        this.emit('engine:stopped');
    }

    /**
     * Start the main processing loop
     */
    startProcessingLoop() {
        console.log(`🔄 Starting processing loop (interval: ${this.processingCheckInterval}ms)...`);

        this.processingInterval = setInterval(async () => {
            if (!this.isRunning) return;

            try {
                await this.processNextBatch();
            } catch (error) {
                console.error('❌ Error in processing loop:', error.message);
            }
        }, this.processingCheckInterval);
    }

    /**
     * Process next batch of messages from queue
     */
    async processNextBatch() {
        // Check if we have capacity for more processing
        const activeWorkers = this.processingWorkers.size;
        if (activeWorkers >= this.maxConcurrentProcessing) {
            return; // At capacity
        }

        // The existing MessageQueueService handles message processing differently
        // It automatically processes messages from the database queue
        // So this method doesn't need to actively poll for messages
        // Instead, we'll rely on immediate processing when messages arrive
    }

    /**
     * Start processing worker for a single message
     */
    async startMessageProcessingWorker(queuedMessage) {
        const workerId = `worker_${queuedMessage.id}_${Date.now()}`;

        try {
            console.log(`🏭 Starting processing worker ${workerId} for message ${queuedMessage.message_id}`);

            // Track active worker
            this.processingWorkers.set(workerId, {
                messageId: queuedMessage.message_id,
                interfaceId: queuedMessage.interface_id,
                startTime: Date.now()
            });

            // Process the message through the pipeline
            const result = await this.processMessagePipeline(queuedMessage);

            if (result.success) {
                this.metrics.messagesProcessed++;
                console.log(`✅ Worker ${workerId} completed successfully`);
            } else {
                console.log(`❌ Worker ${workerId} failed: ${result.error}`);
            }

        } catch (error) {
            console.error(`❌ Worker ${workerId} error:`, error.message);

        } finally {
            // Remove worker from tracking
            this.processingWorkers.delete(workerId);
        }
    }

    /**
     * Process message through the complete pipeline
     */
    async processMessagePipeline(queuedMessage) {
        const pipelineStartTime = Date.now();

        try {
            console.log(`🔄 Starting pipeline for message ${queuedMessage.message_id}`);

            // Step 1: Determine routing destination
            console.log(`📍 Step 1: Routing analysis...`);
            const routingResult = await this.routingService.routeMessage(
                queuedMessage,
                queuedMessage.interfaceConfig
            );

            if (!routingResult.success) {
                throw new Error(`Routing failed: ${routingResult.error}`);
            }

            const routingDecision = routingResult.routingDecision;
            console.log(`✅ Step 1: Route determined - ${routingDecision.selectedDestination.type}://${routingDecision.selectedDestination.host}:${routingDecision.selectedDestination.port}`);

            // Step 2: Transform message (if required)
            let transformedContent = queuedMessage.raw_message;

            if (routingDecision.transformationRequired) {
                console.log(`🔄 Step 2: Transformation required...`);

                const transformationResult = await this.transformationService.transformHl7ToFhir(
                    queuedMessage.raw_message,
                    routingDecision.transformationConfig
                );

                if (!transformationResult.success) {
                    this.metrics.transformationFailures++;
                    throw new Error(`Transformation failed: ${transformationResult.error}`);
                }

                transformedContent = JSON.stringify(transformationResult.fhirResource, null, 2);
                this.metrics.transformationSuccesses++;

                console.log(`✅ Step 2: Transformation completed - ${transformationResult.targetResourceType} resource created`);
            } else {
                console.log(`⏭️ Step 2: No transformation required, using original content`);
            }

            // Step 3: Deliver to destination
            console.log(`📤 Step 3: Delivering to destination...`);

            const deliveryResult = await this.deliveryService.deliverMessage(
                queuedMessage,
                routingDecision.selectedDestination,
                transformedContent
            );

            if (!deliveryResult.success) {
                this.metrics.deliveryFailures++;
                throw new Error(`Delivery failed: ${deliveryResult.error}`);
            }

            this.metrics.deliverySuccesses++;
            console.log(`✅ Step 3: Delivery completed successfully`);

            // Calculate and record pipeline performance
            const pipelineTime = Date.now() - pipelineStartTime;
            this.updatePipelineMetrics(pipelineTime);

            console.log(`🎉 Pipeline completed for message ${queuedMessage.message_id} in ${pipelineTime}ms`);

            return {
                success: true,
                pipelineTime,
                transformationResult: routingDecision.transformationRequired,
                deliveryResult: deliveryResult.response,
                routingDecision
            };

        } catch (error) {
            const pipelineTime = Date.now() - pipelineStartTime;
            console.error(`💥 Pipeline failed for message ${queuedMessage.message_id} after ${pipelineTime}ms:`, error.message);

            return {
                success: false,
                error: error.message,
                pipelineTime
            };
        }
    }

    /**
     * Process specific queued message (event handler)
     */
    async processQueuedMessage(queuedMessage) {
        // This method can be used for immediate processing of high-priority messages
        // For now, we rely on the main processing loop
        console.log(`📥 Message queued for processing: ${queuedMessage.message_id} (priority ${queuedMessage.priority})`);
    }

    /**
     * Update pipeline performance metrics
     */
    updatePipelineMetrics(pipelineTime) {
        const totalProcessed = this.metrics.messagesProcessed;
        this.metrics.avgPipelineTime =
            ((this.metrics.avgPipelineTime * (totalProcessed - 1)) + pipelineTime) / totalProcessed;
    }

    /**
     * Wait for all active workers to complete
     */
    async waitForWorkersToComplete(timeoutMs = 30000) {
        const startTime = Date.now();

        while (this.processingWorkers.size > 0) {
            if (Date.now() - startTime > timeoutMs) {
                console.warn(`⚠️ Timeout waiting for ${this.processingWorkers.size} workers to complete`);
                break;
            }

            await new Promise(resolve => setTimeout(resolve, 100));
        }
    }

    /**
     * Get comprehensive engine metrics
     */
    getMetrics() {
        const uptime = this.metrics.startTime ?
            Math.floor((new Date() - this.metrics.startTime) / 1000) : 0;

        const totalTransformations = this.metrics.transformationSuccesses + this.metrics.transformationFailures;
        const totalDeliveries = this.metrics.deliverySuccesses + this.metrics.deliveryFailures;

        return {
            engine: {
                isRunning: this.isRunning,
                uptime: `${Math.floor(uptime / 60)}m ${uptime % 60}s`,
                activeWorkers: this.processingWorkers.size,
                maxConcurrentProcessing: this.maxConcurrentProcessing
            },
            pipeline: {
                messagesProcessed: this.metrics.messagesProcessed,
                avgPipelineTime: Math.round(this.metrics.avgPipelineTime)
            },
            transformation: {
                successful: this.metrics.transformationSuccesses,
                failed: this.metrics.transformationFailures,
                total: totalTransformations,
                successRate: totalTransformations > 0 ?
                    ((this.metrics.transformationSuccesses / totalTransformations) * 100).toFixed(2) + '%' : '0%'
            },
            delivery: {
                successful: this.metrics.deliverySuccesses,
                failed: this.metrics.deliveryFailures,
                total: totalDeliveries,
                successRate: totalDeliveries > 0 ?
                    ((this.metrics.deliverySuccesses / totalDeliveries) * 100).toFixed(2) + '%' : '0%'
            },
            microservices: {
                messageQueue: this.messageQueue.getMetrics(),
                transformation: this.transformationService.getMetrics(),
                routing: this.routingService.getMetrics(),
                delivery: this.deliveryService.getMetrics()
            }
        };
    }

    /**
     * Get active workers status
     */
    getActiveWorkers() {
        return Array.from(this.processingWorkers.entries()).map(([workerId, worker]) => ({
            workerId,
            messageId: worker.messageId,
            interfaceId: worker.interfaceId,
            duration: Date.now() - worker.startTime
        }));
    }

    /**
     * Process specific message immediately (bypass queue)
     */
    async processMessageImmediate(message, interfaceConfig) {
        console.log(`⚡ Processing message ${message.message_id} immediately (bypass queue)`);

        const queuedMessage = {
            ...message,
            interfaceConfig,
            queuedAt: new Date(),
            processingAttempts: 0
        };

        return await this.processMessagePipeline(queuedMessage);
    }

    /**
     * Get engine health status
     */
    getHealthStatus() {
        const metrics = this.getMetrics();
        const queueMetrics = this.messageQueue.getMetrics();

        const isHealthy = this.isRunning &&
                         metrics.engine.activeWorkers < this.maxConcurrentProcessing &&
                         queueMetrics.performance.successRate !== '0%';

        return {
            status: isHealthy ? 'healthy' : 'unhealthy',
            engine: {
                running: this.isRunning,
                activeWorkers: metrics.engine.activeWorkers,
                capacity: `${metrics.engine.activeWorkers}/${this.maxConcurrentProcessing}`
            },
            messageQueue: {
                status: queueMetrics.isRunning ? 'running' : 'stopped',
                messagesInQueue: queueMetrics.messagesInQueue || 0,
                deadLetterQueue: queueMetrics.deadLetterQueueSize || 0
            },
            performance: {
                avgPipelineTime: metrics.pipeline.avgPipelineTime,
                transformationSuccessRate: metrics.transformation.successRate,
                deliverySuccessRate: metrics.delivery.successRate
            }
        };
    }

    /**
     * Reprocess failed messages
     */
    async reprocessFailedMessages(interfaceId, maxAge = '24 hours') {
        console.log(`🔄 Reprocessing failed messages for interface ${interfaceId}...`);

        // Check if the message queue service supports retry functionality
        if (this.messageQueue && typeof this.messageQueue.retryFailedMessages === 'function') {
            const reprocessedCount = await this.messageQueue.retryFailedMessages(interfaceId, maxAge);
            console.log(`✅ Queued ${reprocessedCount} messages for reprocessing`);
            this.emit('messages:reprocessed', { interfaceId, count: reprocessedCount });
            return reprocessedCount;
        } else {
            console.log(`⚠️ Message queue service does not support retry functionality`);
            return 0;
        }
    }
}

module.exports = EnterpriseProcessingEngine;