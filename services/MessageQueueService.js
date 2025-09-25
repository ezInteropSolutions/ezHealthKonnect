// services/MessageQueueService.js
// Message queue service using database-backed queuing (alternative to Redis)

const EventEmitter = require('events');

class MessageQueueService extends EventEmitter {
    constructor() {
        super();
        this.isRunning = false;
        this.processingInterval = null;
        this.batchSize = 10; // Process 10 messages at a time
        this.processingTimeout = 30000; // 30 seconds
    }

    /**
     * Start the message queue processor
     */
    async start() {
        if (this.isRunning) {
            console.log('📦 Message Queue Service already running');
            return;
        }

        console.log('📦 Starting Message Queue Service...');

        this.isRunning = true;

        // Start processing loop (reduced frequency to avoid log spam)
        this.processingInterval = setInterval(async () => {
            await this.processNextBatch();
        }, 30000); // Check for messages every 30 seconds

        this.emit('queue:started');
        console.log('✅ Message Queue Service started');
    }

    /**
     * Stop the message queue processor
     */
    async stop() {
        if (!this.isRunning) {
            console.log('📦 Message Queue Service not running');
            return;
        }

        console.log('📦 Stopping Message Queue Service...');

        this.isRunning = false;

        if (this.processingInterval) {
            clearInterval(this.processingInterval);
            this.processingInterval = null;
        }

        this.emit('queue:stopped');
        console.log('✅ Message Queue Service stopped');
    }

    /**
     * Add message to processing queue
     */
    async enqueueMessage(interfaceId, messageData, options = {}) {
        const database = require('../config/database');
        const sequelize = database.sequelize;

        const messageId = options.messageId || require('crypto').randomUUID();
        const priority = options.priority || 0;
        const scheduledFor = options.delay ? new Date(Date.now() + options.delay) : new Date();
        const maxAttempts = options.maxAttempts || 3;

        try {
            await sequelize.query(`
                INSERT INTO message_processing_queue (
                    id, interface_id, message_id, queue_name, priority,
                    message_data, message_metadata, status, max_attempts,
                    scheduled_for, created_at
                ) VALUES (:id, :interface_id, :message_id, :queue_name, :priority,
                    :message_data, :message_metadata, 'pending', :max_attempts,
                    :scheduled_for, :created_at)
            `, {
                replacements: {
                    id: require('crypto').randomUUID(),
                    interface_id: interfaceId,
                    message_id: messageId,
                    queue_name: options.queueName || 'default',
                    priority: priority,
                    message_data: JSON.stringify(messageData),
                    message_metadata: JSON.stringify(options.metadata || {}),
                    max_attempts: maxAttempts,
                    scheduled_for: scheduledFor,
                    created_at: new Date()
                },
                type: sequelize.QueryTypes.INSERT
            });

            console.log(`📨 Message queued: ${messageId} for interface ${interfaceId}`);

            this.emit('message:queued', {
                messageId,
                interfaceId,
                queueName: options.queueName || 'default',
                priority,
                scheduledFor
            });

            return messageId;

        } catch (error) {
            console.error(`❌ Failed to enqueue message ${messageId}:`, error);
            throw error;
        }
    }

    /**
     * Process next batch of messages
     */
    async processNextBatch() {
        if (!this.isRunning) return;

        // Use existing database configuration
        const database = require('../config/database');
        const sequelize = database.sequelize;

        try {
            // Get next batch of messages to process
            const result = await sequelize.query(`
                SELECT * FROM message_processing_queue
                WHERE status = 'pending'
                AND scheduled_for <= NOW()
                ORDER BY priority DESC, scheduled_for ASC
                LIMIT :batchSize
                FOR UPDATE SKIP LOCKED
            `, {
                replacements: { batchSize: this.batchSize },
                type: sequelize.QueryTypes.SELECT
            });

            if (result.length === 0) {
                return; // No messages to process
            }

            // Process each message
            const processingPromises = result.map(queueItem =>
                this.processQueueMessage(queueItem)
            );

            await Promise.allSettled(processingPromises);

        } catch (error) {
            console.error('❌ Failed to process message batch:', error);
        }
    }

    /**
     * Process individual queue message
     */
    async processQueueMessage(queueItem) {
        const database = require('../config/database');
        const sequelize = database.sequelize;

        try {
            // Mark message as processing
            await sequelize.query(`
                UPDATE message_processing_queue
                SET status = 'processing', started_processing_at = NOW(), attempts = attempts + 1
                WHERE id = :id
            `, {
                replacements: { id: queueItem.id },
                type: sequelize.QueryTypes.UPDATE
            });

            console.log(`🔄 Processing queued message: ${queueItem.message_id}`);

            // Get processing engine instance
            const ProcessingEngineService = require('./ProcessingEngineService');
            const processingEngine = new ProcessingEngineService();

            // Get the interface processor for this message
            const interfaceProcessor = processingEngine.activeInterfaces.get(queueItem.interface_id);

            if (!interfaceProcessor) {
                throw new Error(`Interface ${queueItem.interface_id} is not active`);
            }

            // Parse message data
            const messageData = JSON.parse(queueItem.message_data);

            // Process the message through the interface processor
            await interfaceProcessor.onMessageReceived(messageData, queueItem.message_id);

            // Mark as completed
            await sequelize.query(`
                UPDATE message_processing_queue
                SET status = 'completed', completed_at = NOW()
                WHERE id = :id
            `, {
                replacements: { id: queueItem.id },
                type: sequelize.QueryTypes.UPDATE
            });

            this.emit('message:processed', {
                messageId: queueItem.message_id,
                interfaceId: queueItem.interface_id,
                processingTime: Date.now() - new Date(queueItem.started_processing_at).getTime()
            });

            console.log(`✅ Completed queued message: ${queueItem.message_id}`);

        } catch (error) {
            console.error(`❌ Failed to process queued message ${queueItem.message_id}:`, error);

            // Handle retry or failure
            await this.handleMessageError(queueItem, error);
        }
    }

    /**
     * Handle message processing errors
     */
    async handleMessageError(queueItem, error) {
        const database = require('../config/database');
        const sequelize = database.sequelize;

        const maxAttempts = queueItem.max_attempts;
        const currentAttempts = queueItem.attempts + 1; // Already incremented in processQueueMessage

        if (currentAttempts >= maxAttempts) {
            // Max attempts reached - mark as failed
            await sequelize.query(`
                UPDATE message_processing_queue
                SET status = 'failed', completed_at = NOW(),
                    error_message = :error_message, error_details = :error_details
                WHERE id = :id
            `, {
                replacements: {
                    error_message: error.message,
                    error_details: JSON.stringify({ error: error.message, stack: error.stack }),
                    id: queueItem.id
                },
                type: sequelize.QueryTypes.UPDATE
            });

            this.emit('message:failed', {
                messageId: queueItem.message_id,
                interfaceId: queueItem.interface_id,
                error: error.message,
                attempts: currentAttempts
            });

            console.log(`💀 Message failed permanently: ${queueItem.message_id} (${currentAttempts}/${maxAttempts} attempts)`);

        } else {
            // Schedule for retry with exponential backoff
            const retryDelay = Math.min(Math.pow(2, currentAttempts) * 1000, 300000); // Max 5 minutes
            const nextRetryAt = new Date(Date.now() + retryDelay);

            await sequelize.query(`
                UPDATE message_processing_queue
                SET status = 'retrying', scheduled_for = :next_retry_at,
                    error_message = :error_message, error_details = :error_details
                WHERE id = :id
            `, {
                replacements: {
                    next_retry_at: nextRetryAt,
                    error_message: error.message,
                    error_details: JSON.stringify({ error: error.message, stack: error.stack, attempt: currentAttempts }),
                    id: queueItem.id
                },
                type: sequelize.QueryTypes.UPDATE
            });

            // Schedule message back to pending status
            setTimeout(async () => {
                try {
                    await sequelize.query(`
                        UPDATE message_processing_queue
                        SET status = 'pending', scheduled_for = NOW()
                        WHERE id = :id AND status = 'retrying'
                    `, {
                        replacements: { id: queueItem.id },
                        type: sequelize.QueryTypes.UPDATE
                    });
                } catch (err) {
                    console.error('Failed to reschedule retry message:', err);
                }
            }, retryDelay);

            this.emit('message:retry', {
                messageId: queueItem.message_id,
                interfaceId: queueItem.interface_id,
                error: error.message,
                attempt: currentAttempts,
                nextRetryAt
            });

            console.log(`🔄 Message scheduled for retry: ${queueItem.message_id} (attempt ${currentAttempts}/${maxAttempts}) at ${nextRetryAt.toISOString()}`);
        }
    }

    /**
     * Get queue statistics
     */
    async getQueueStats() {
        const database = require('../config/database');
        const sequelize = database.sequelize;

        try {
            const result = await sequelize.query(`
                SELECT
                    status,
                    COUNT(*) as count,
                    AVG(EXTRACT(EPOCH FROM (NOW() - created_at))) as avg_age_seconds
                FROM message_processing_queue
                WHERE created_at >= NOW() - INTERVAL '24 hours'
                GROUP BY status

                UNION ALL

                SELECT
                    'total' as status,
                    COUNT(*) as count,
                    AVG(EXTRACT(EPOCH FROM (NOW() - created_at))) as avg_age_seconds
                FROM message_processing_queue
                WHERE created_at >= NOW() - INTERVAL '24 hours'
            `);

            const stats = result.rows.reduce((acc, row) => {
                acc[row.status] = {
                    count: parseInt(row.count),
                    avgAgeSeconds: parseFloat(row.avg_age_seconds || 0)
                };
                return acc;
            }, {});

            return {
                isRunning: this.isRunning,
                batchSize: this.batchSize,
                stats: stats,
                lastUpdated: new Date()
            };

        } catch (error) {
            console.error('Failed to get queue stats:', error);
            return {
                isRunning: this.isRunning,
                batchSize: this.batchSize,
                stats: { error: error.message },
                lastUpdated: new Date()
            };
        }
    }

    /**
     * Clean up old processed messages
     */
    async cleanup(olderThanDays = 7) {
        const database = require('../config/database');
        const sequelize = database.sequelize;

        try {
            const result = await sequelize.query(`
                DELETE FROM message_processing_queue
                WHERE status IN ('completed', 'failed')
                AND completed_at < NOW() - INTERVAL '${olderThanDays} days'
            `, {
                type: sequelize.QueryTypes.DELETE
            });

            console.log(`🧹 Cleaned up ${result.rowCount} old queue messages`);

            this.emit('queue:cleanup', {
                deletedCount: result.rowCount,
                olderThanDays
            });

            return result.rowCount;

        } catch (error) {
            console.error('Failed to cleanup queue messages:', error);
            throw error;
        }
    }

    /**
     * Retry all failed messages for an interface
     */
    async retryFailedMessages(interfaceId, maxAge = '24 hours') {
        const database = require('../config/database');
        const sequelize = database.sequelize;

        try {
            const result = await sequelize.query(`
                UPDATE message_processing_queue
                SET status = 'pending', scheduled_for = NOW(),
                    attempts = 0, error_message = NULL, error_details = '{}'
                WHERE interface_id = :interface_id
                AND status = 'failed'
                AND completed_at >= NOW() - INTERVAL '${maxAge}'
            `, {
                replacements: {
                    interface_id: interfaceId
                },
                type: sequelize.QueryTypes.UPDATE
            });

            console.log(`🔄 Retrying ${result.rowCount} failed messages for interface ${interfaceId}`);

            this.emit('messages:bulk_retry', {
                interfaceId,
                retriedCount: result.rowCount
            });

            return result.rowCount;

        } catch (error) {
            console.error(`Failed to retry messages for interface ${interfaceId}:`, error);
            throw error;
        }
    }
}

module.exports = MessageQueueService;