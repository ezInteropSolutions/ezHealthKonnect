// services/connectors/BaseInputConnector.js
// Base class for all input connectors

const EventEmitter = require('events');

class BaseInputConnector extends EventEmitter {
    constructor(config) {
        super();
        this.config = config;
        this.isConnected = false;
        this.isListening = false;
        this.connectionRetries = 0;
        this.maxRetries = config.maxRetries || 5;
        this.retryDelay = config.retryDelay || 5000;
        this.healthCheckInterval = null;
        this.lastHealthCheck = null;

        // Validation
        this.validateConfig();
    }

    /**
     * Validate connector configuration - to be implemented by subclasses
     */
    validateConfig() {
        throw new Error('validateConfig() must be implemented by subclass');
    }

    /**
     * Establish connection - to be implemented by subclasses
     */
    async connect() {
        throw new Error('connect() must be implemented by subclass');
    }

    /**
     * Start listening for messages - to be implemented by subclasses
     */
    async startListening() {
        throw new Error('startListening() must be implemented by subclass');
    }

    /**
     * Stop listening for messages - to be implemented by subclasses
     */
    async stopListening() {
        throw new Error('stopListening() must be implemented by subclass');
    }

    /**
     * Disconnect from source - to be implemented by subclasses
     */
    async disconnect() {
        throw new Error('disconnect() must be implemented by subclass');
    }

    /**
     * Health check - to be implemented by subclasses
     */
    async healthCheck() {
        throw new Error('healthCheck() must be implemented by subclass');
    }

    /**
     * Connection retry logic with exponential backoff
     */
    async retryConnection() {
        if (this.connectionRetries >= this.maxRetries) {
            const error = new Error(`Max connection retries (${this.maxRetries}) exceeded`);
            this.emit('error', error);
            return false;
        }

        const delay = this.retryDelay * Math.pow(2, this.connectionRetries);
        this.connectionRetries++;

        console.log(`🔄 Retrying connection (attempt ${this.connectionRetries}/${this.maxRetries}) in ${delay}ms...`);

        await new Promise(resolve => setTimeout(resolve, delay));

        try {
            await this.connect();
            this.connectionRetries = 0; // Reset on successful connection
            return true;
        } catch (error) {
            console.error(`❌ Connection retry ${this.connectionRetries} failed:`, error.message);
            return this.retryConnection();
        }
    }

    /**
     * Start health check monitoring
     */
    startHealthMonitoring(intervalMs = 30000) {
        this.healthCheckInterval = setInterval(async () => {
            try {
                const isHealthy = await this.healthCheck();
                this.lastHealthCheck = {
                    timestamp: new Date(),
                    status: isHealthy ? 'healthy' : 'unhealthy'
                };

                if (!isHealthy && this.isConnected) {
                    console.warn(`⚠️ Health check failed for ${this.config.type} connector`);
                    this.emit('health:unhealthy', this.lastHealthCheck);

                    // Attempt to reconnect
                    await this.reconnect();
                }
            } catch (error) {
                console.error('Health check error:', error);
                this.lastHealthCheck = {
                    timestamp: new Date(),
                    status: 'error',
                    error: error.message
                };
            }
        }, intervalMs);
    }

    /**
     * Stop health check monitoring
     */
    stopHealthMonitoring() {
        if (this.healthCheckInterval) {
            clearInterval(this.healthCheckInterval);
            this.healthCheckInterval = null;
        }
    }

    /**
     * Reconnection logic
     */
    async reconnect() {
        console.log(`🔄 Attempting to reconnect ${this.config.type} connector...`);

        try {
            await this.disconnect();
            await this.connect();
            await this.startListening();

            console.log(`✅ ${this.config.type} connector reconnected successfully`);
            this.emit('reconnected');
        } catch (error) {
            console.error(`❌ Reconnection failed:`, error);
            this.emit('reconnection:failed', error);

            // Start retry process
            this.retryConnection();
        }
    }

    /**
     * Process incoming message with error handling
     */
    processIncomingMessage(rawMessage, context = {}) {
        try {
            // Add connector metadata
            const messageContext = {
                ...context,
                connector: {
                    type: this.config.type,
                    receivedAt: new Date(),
                    config: {
                        // Only include safe config fields
                        type: this.config.type,
                        name: this.config.name
                    }
                }
            };

            // Emit message event
            this.emit('message', rawMessage, messageContext);

        } catch (error) {
            console.error('Error processing incoming message:', error);
            this.emit('error', error);
        }
    }

    /**
     * Get connector status
     */
    getStatus() {
        return {
            type: this.config.type,
            isConnected: this.isConnected,
            isListening: this.isListening,
            connectionRetries: this.connectionRetries,
            lastHealthCheck: this.lastHealthCheck,
            uptime: this.getUptime()
        };
    }

    /**
     * Get connector uptime
     */
    getUptime() {
        if (!this.connectedAt) return 0;
        return Date.now() - this.connectedAt;
    }

    /**
     * Set connected state
     */
    setConnected(connected) {
        this.isConnected = connected;
        if (connected && !this.connectedAt) {
            this.connectedAt = Date.now();
        }
    }

    /**
     * Set listening state
     */
    setListening(listening) {
        this.isListening = listening;
    }

    /**
     * Graceful shutdown
     */
    async shutdown() {
        console.log(`🛑 Shutting down ${this.config.type} connector...`);

        this.stopHealthMonitoring();

        if (this.isListening) {
            await this.stopListening();
        }

        if (this.isConnected) {
            await this.disconnect();
        }

        console.log(`✅ ${this.config.type} connector shutdown complete`);
    }
}

module.exports = BaseInputConnector;