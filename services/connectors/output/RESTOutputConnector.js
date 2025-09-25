// services/connectors/output/RESTOutputConnector.js
// REST/HTTP output connector for sending processed messages to REST endpoints

class RESTOutputConnector {
    constructor(config) {
        this.config = {
            endpoint: config.endpoint,
            method: config.method || 'POST',
            headers: config.headers || { 'Content-Type': 'application/json' },
            timeout: config.timeout || 30000,
            retryAttempts: config.retryAttempts || 3,
            retryDelay: config.retryDelay || 1000,
            ...config
        };

        console.log(`✅ REST output connector initialized: ${this.config.endpoint}`);
    }

    /**
     * Send processed message to REST endpoint
     * @param {Object} message - Processed message data
     * @param {Object} metadata - Message metadata
     * @returns {Promise<Object>} - Delivery result
     */
    async send(message, metadata = {}) {
        let attempt = 0;
        let lastError;

        while (attempt <= this.config.retryAttempts) {
            try {
                console.log(`📤 Sending message to REST endpoint: ${this.config.endpoint} (attempt ${attempt + 1})`);

                // Get fetch implementation
                let fetch;
                try {
                    fetch = require('node-fetch');
                } catch {
                    fetch = global.fetch;
                }

                if (!fetch) {
                    throw new Error('No fetch implementation available');
                }

                // Prepare request options
                const options = {
                    method: this.config.method,
                    headers: { ...this.config.headers },
                    timeout: this.config.timeout
                };

                // Add body for POST/PUT/PATCH requests
                if (['POST', 'PUT', 'PATCH'].includes(this.config.method)) {
                    options.body = JSON.stringify({
                        message: message,
                        metadata: metadata,
                        timestamp: new Date().toISOString(),
                        source: 'ezHealthKonnect'
                    });
                }

                // Add authentication if configured
                if (this.config.authentication) {
                    this.addAuthentication(options.headers);
                }

                // Make the request
                const response = await fetch(this.config.endpoint, options);

                if (!response.ok) {
                    throw new Error(`HTTP ${response.status}: ${response.statusText}`);
                }

                const result = await response.text();

                console.log(`✅ Message delivered successfully to ${this.config.endpoint}`);

                return {
                    success: true,
                    deliveryId: require('crypto').randomUUID(),
                    timestamp: new Date(),
                    responseStatus: response.status,
                    responseData: result,
                    endpoint: this.config.endpoint
                };

            } catch (error) {
                lastError = error;
                attempt++;

                console.error(`❌ Failed to send message to REST endpoint (attempt ${attempt}):`, error.message);

                if (attempt <= this.config.retryAttempts) {
                    console.log(`🔄 Retrying in ${this.config.retryDelay}ms...`);
                    await new Promise(resolve => setTimeout(resolve, this.config.retryDelay));
                }
            }
        }

        // All retry attempts failed
        throw new Error(`Failed to deliver message after ${this.config.retryAttempts + 1} attempts: ${lastError.message}`);
    }

    /**
     * Add authentication headers based on configuration
     */
    addAuthentication(headers) {
        const auth = this.config.authentication;

        switch (auth.type) {
            case 'bearer':
                headers['Authorization'] = `Bearer ${auth.token}`;
                break;
            case 'basic':
                const credentials = Buffer.from(`${auth.username}:${auth.password}`).toString('base64');
                headers['Authorization'] = `Basic ${credentials}`;
                break;
            case 'apikey':
                headers[auth.headerName || 'X-API-Key'] = auth.key;
                break;
            case 'custom':
                Object.assign(headers, auth.headers);
                break;
        }
    }

    /**
     * Test the connection to the REST endpoint
     */
    async testConnection() {
        try {
            console.log(`🔍 Testing REST endpoint connection: ${this.config.endpoint}`);

            // Use HEAD request for connection test if supported, otherwise GET
            const testMethod = this.config.testMethod || 'HEAD';

            let fetch;
            try {
                fetch = require('node-fetch');
            } catch {
                fetch = global.fetch;
            }

            if (!fetch) {
                throw new Error('No fetch implementation available');
            }

            const options = {
                method: testMethod,
                headers: { ...this.config.headers },
                timeout: this.config.timeout
            };

            if (this.config.authentication) {
                this.addAuthentication(options.headers);
            }

            const response = await fetch(this.config.endpoint, options);

            if (response.ok || response.status === 405) { // 405 = Method Not Allowed is acceptable for HEAD requests
                console.log(`✅ REST endpoint connection test successful`);
                return { success: true, status: response.status };
            } else {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

        } catch (error) {
            console.error(`❌ REST endpoint connection test failed:`, error.message);
            throw error;
        }
    }

    /**
     * Close the connection (cleanup)
     */
    async close() {
        console.log(`🔌 REST output connector disconnected from ${this.config.endpoint}`);
        // No persistent connection to close for REST
    }

    /**
     * Get connector status
     */
    getStatus() {
        return {
            type: 'REST',
            endpoint: this.config.endpoint,
            method: this.config.method,
            status: 'ready',
            lastActivity: new Date()
        };
    }
}

module.exports = RESTOutputConnector;