// services/DeliveryService.js
// Enterprise Delivery Service - Reliable message delivery with retry, circuit breaker, and monitoring
// Handles HTTP, TCP, File, and Database destinations with comprehensive delivery tracking

const http = require('http');
const https = require('https');
const net = require('net');
const fs = require('fs');
const path = require('path');

class DeliveryService {
    constructor() {
        this.activeDeliveries = new Map();
        this.circuitBreakers = new Map();
        this.connectionPools = new Map();

        // Configuration
        this.maxRetries = 3;
        this.retryBackoffMs = 1000;
        this.connectionTimeout = 10000;
        this.deliveryTimeout = 30000;
        this.circuitBreakerThreshold = 5;
        this.circuitBreakerTimeout = 60000;

        // Performance metrics
        this.metrics = {
            deliveriesAttempted: 0,
            deliveriesSuccessful: 0,
            deliveriesFailed: 0,
            avgDeliveryTime: 0,
            circuitBreakerTrips: 0,
            retriesMade: 0,
            startTime: new Date()
        };

        console.log(`📦 DeliveryService initialized with ${this.maxRetries} max retries`);
    }

    /**
     * Deliver message to destination
     */
    async deliverMessage(message, destination, transformedContent) {
        const deliveryId = `delivery_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
        const startTime = Date.now();

        try {
            console.log(`📤 Starting delivery ${deliveryId} to ${destination.type}://${destination.host}:${destination.port}`);

            // Check circuit breaker
            if (this.isCircuitBreakerOpen(destination)) {
                throw new Error(`Circuit breaker is open for destination ${destination.id}`);
            }

            // Track active delivery
            this.activeDeliveries.set(deliveryId, {
                messageId: message.message_id,
                destination,
                startTime,
                attempts: 0
            });

            // Perform delivery based on destination type
            let result;
            switch (destination.type.toLowerCase()) {
                case 'http':
                case 'https':
                    result = await this.deliverHttp(message, destination, transformedContent, deliveryId);
                    break;
                case 'tcp':
                    result = await this.deliverTcp(message, destination, transformedContent, deliveryId);
                    break;
                case 'file':
                    result = await this.deliverFile(message, destination, transformedContent, deliveryId);
                    break;
                case 'database':
                    result = await this.deliverDatabase(message, destination, transformedContent, deliveryId);
                    break;
                default:
                    throw new Error(`Unsupported destination type: ${destination.type}`);
            }

            // Update metrics on success
            const deliveryTime = Date.now() - startTime;
            this.updateMetrics(true, deliveryTime);
            this.recordCircuitBreakerSuccess(destination);

            console.log(`✅ Delivery ${deliveryId} completed successfully in ${deliveryTime}ms`);

            return {
                success: true,
                deliveryId,
                deliveryTime,
                destination: {
                    id: destination.id,
                    type: destination.type,
                    endpoint: `${destination.host}:${destination.port}`
                },
                response: result
            };

        } catch (error) {
            const deliveryTime = Date.now() - startTime;
            this.updateMetrics(false, deliveryTime);
            this.recordCircuitBreakerFailure(destination);

            console.error(`❌ Delivery ${deliveryId} failed after ${deliveryTime}ms:`, error.message);

            return {
                success: false,
                deliveryId,
                deliveryTime,
                error: error.message,
                destination: {
                    id: destination.id,
                    type: destination.type,
                    endpoint: `${destination.host}:${destination.port}`
                }
            };

        } finally {
            // Clean up active delivery tracking
            this.activeDeliveries.delete(deliveryId);
        }
    }

    /**
     * Deliver message via HTTP/HTTPS
     */
    async deliverHttp(message, destination, content, deliveryId) {
        return new Promise((resolve, reject) => {
            const isHttps = destination.type.toLowerCase() === 'https';
            const httpModule = isHttps ? https : http;

            // Prepare request options
            const options = {
                hostname: destination.host,
                port: destination.port,
                path: destination.path || '/',
                method: destination.method || 'POST',
                headers: {
                    'Content-Type': destination.contentType || 'application/json',
                    'Content-Length': Buffer.byteLength(content),
                    'User-Agent': 'ezHealthKonnect-DeliveryService/1.0',
                    'X-Message-ID': message.message_id,
                    'X-Correlation-ID': message.correlation_id || message.message_id,
                    'X-Delivery-ID': deliveryId,
                    ...destination.headers
                },
                timeout: this.deliveryTimeout
            };

            console.log(`📡 HTTP ${options.method} ${options.hostname}:${options.port}${options.path}`);

            const req = httpModule.request(options, (res) => {
                let responseData = '';

                res.on('data', (chunk) => {
                    responseData += chunk;
                });

                res.on('end', () => {
                    if (res.statusCode >= 200 && res.statusCode < 300) {
                        resolve({
                            statusCode: res.statusCode,
                            statusMessage: res.statusMessage,
                            headers: res.headers,
                            body: responseData,
                            contentLength: Buffer.byteLength(responseData)
                        });
                    } else {
                        reject(new Error(`HTTP ${res.statusCode}: ${res.statusMessage} - ${responseData}`));
                    }
                });
            });

            req.on('error', (error) => {
                reject(new Error(`HTTP request failed: ${error.message}`));
            });

            req.on('timeout', () => {
                req.destroy();
                reject(new Error(`HTTP request timeout after ${this.deliveryTimeout}ms`));
            });

            // Send the content
            req.write(content);
            req.end();
        });
    }

    /**
     * Deliver message via TCP
     */
    async deliverTcp(message, destination, content, deliveryId) {
        return new Promise((resolve, reject) => {
            const socket = new net.Socket();

            console.log(`🔌 TCP connection to ${destination.host}:${destination.port}`);

            socket.setTimeout(this.deliveryTimeout);

            socket.connect(destination.port, destination.host, () => {
                console.log(`🔗 TCP connected to ${destination.host}:${destination.port}`);

                // Add MLLP framing if content doesn't have it
                let framedContent = content;
                if (!content.startsWith('\x0B')) {
                    framedContent = '\x0B' + content + '\x1C\x0D';
                }

                socket.write(framedContent);
            });

            socket.on('data', (data) => {
                const response = data.toString();
                console.log(`📥 TCP response received: ${response.substring(0, 100)}...`);

                socket.destroy();
                resolve({
                    response,
                    contentLength: data.length,
                    protocol: 'TCP'
                });
            });

            socket.on('error', (error) => {
                reject(new Error(`TCP delivery failed: ${error.message}`));
            });

            socket.on('timeout', () => {
                socket.destroy();
                reject(new Error(`TCP delivery timeout after ${this.deliveryTimeout}ms`));
            });

            socket.on('close', () => {
                // If no response received, consider it successful for send-only TCP
                if (!socket.response) {
                    resolve({
                        response: 'TCP_SENT',
                        contentLength: content.length,
                        protocol: 'TCP'
                    });
                }
            });
        });
    }

    /**
     * Deliver message to file system
     */
    async deliverFile(message, destination, content, deliveryId) {
        try {
            const filePath = this.generateFilePath(destination, message, deliveryId);

            // Ensure directory exists
            const dir = path.dirname(filePath);
            if (!fs.existsSync(dir)) {
                fs.mkdirSync(dir, { recursive: true });
            }

            // Write content to file
            fs.writeFileSync(filePath, content, 'utf8');

            console.log(`💾 File delivered to: ${filePath}`);

            return {
                filePath,
                contentLength: Buffer.byteLength(content),
                fileSize: fs.statSync(filePath).size,
                protocol: 'FILE'
            };

        } catch (error) {
            throw new Error(`File delivery failed: ${error.message}`);
        }
    }

    /**
     * Deliver message to database
     */
    async deliverDatabase(message, destination, content, deliveryId) {
        try {
            const database = require('../config/database');
            const InterfaceTableManager = require('./InterfaceTableManager');

            // Parse destination configuration
            const dbConfig = destination.databaseConfig || {};
            const targetInterfaceId = destination.targetInterfaceId;

            if (!targetInterfaceId) {
                throw new Error('Target interface ID required for database delivery');
            }

            // Create message data for target interface
            const messageData = {
                interfaceId: targetInterfaceId,
                interfaceName: destination.targetInterfaceName || 'Database-Target',
                messageId: `delivered_${message.message_id}_${deliveryId}`,
                correlationId: message.correlation_id,
                status: 'received',
                priority: message.priority || 5,
                sourceType: 'delivery_service',
                sourceEndpoint: `${message.source_type}://${message.source_endpoint}`,
                sourceIP: null,
                messageType: dbConfig.messageType || message.message_type,
                messageSize: Buffer.byteLength(content),
                messageEncoding: 'UTF-8',
                rawMessage: content,
                receivedAt: new Date()
            };

            // Insert into target interface table
            const insertedId = await InterfaceTableManager.insertMessage(targetInterfaceId, messageData);

            console.log(`💽 Database delivery completed: ${insertedId}`);

            return {
                insertedId,
                targetInterfaceId,
                contentLength: messageData.messageSize,
                protocol: 'DATABASE'
            };

        } catch (error) {
            throw new Error(`Database delivery failed: ${error.message}`);
        }
    }

    /**
     * Deliver with retry logic
     */
    async deliverWithRetry(message, destination, transformedContent) {
        let lastError = null;
        let attempt = 0;

        while (attempt < this.maxRetries) {
            try {
                const result = await this.deliverMessage(message, destination, transformedContent);

                if (result.success) {
                    if (attempt > 0) {
                        console.log(`✅ Delivery succeeded on attempt ${attempt + 1}/${this.maxRetries}`);
                        this.metrics.retriesMade += attempt;
                    }
                    return result;
                }

                lastError = new Error(result.error);

            } catch (error) {
                lastError = error;
            }

            attempt++;

            if (attempt < this.maxRetries) {
                const backoffTime = this.retryBackoffMs * Math.pow(2, attempt - 1);
                console.log(`🔄 Delivery attempt ${attempt} failed, retrying in ${backoffTime}ms...`);

                await new Promise(resolve => setTimeout(resolve, backoffTime));
            }
        }

        console.error(`💀 Delivery failed after ${this.maxRetries} attempts`);
        this.metrics.retriesMade += this.maxRetries - 1;

        return {
            success: false,
            error: `Delivery failed after ${this.maxRetries} attempts: ${lastError.message}`,
            attempts: this.maxRetries
        };
    }

    /**
     * Circuit breaker management
     */
    isCircuitBreakerOpen(destination) {
        const breaker = this.circuitBreakers.get(destination.id);
        if (!breaker) return false;

        if (breaker.state === 'open') {
            if (Date.now() - breaker.lastFailure > this.circuitBreakerTimeout) {
                // Move to half-open state
                breaker.state = 'half-open';
                console.log(`🔄 Circuit breaker half-open for ${destination.id}`);
                return false;
            }
            return true;
        }

        return false;
    }

    recordCircuitBreakerSuccess(destination) {
        const breaker = this.circuitBreakers.get(destination.id);
        if (breaker) {
            if (breaker.state === 'half-open') {
                breaker.state = 'closed';
                breaker.failureCount = 0;
                console.log(`✅ Circuit breaker closed for ${destination.id}`);
            }
        }
    }

    recordCircuitBreakerFailure(destination) {
        let breaker = this.circuitBreakers.get(destination.id);
        if (!breaker) {
            breaker = { state: 'closed', failureCount: 0, lastFailure: null };
            this.circuitBreakers.set(destination.id, breaker);
        }

        breaker.failureCount++;
        breaker.lastFailure = Date.now();

        if (breaker.failureCount >= this.circuitBreakerThreshold) {
            breaker.state = 'open';
            this.metrics.circuitBreakerTrips++;
            console.log(`⚠️ Circuit breaker opened for ${destination.id} after ${breaker.failureCount} failures`);
        }
    }

    /**
     * Generate file path for file delivery
     */
    generateFilePath(destination, message, deliveryId) {
        const basePath = destination.basePath || './data/outbound';
        const timestamp = new Date().toISOString().split('T')[0]; // YYYY-MM-DD
        const filename = destination.filenamePattern || `${message.message_id}_${deliveryId}.txt`;

        return path.join(basePath, timestamp, filename);
    }

    /**
     * Update performance metrics
     */
    updateMetrics(success, deliveryTime) {
        this.metrics.deliveriesAttempted++;

        if (success) {
            this.metrics.deliveriesSuccessful++;

            // Update average delivery time
            const totalSuccessful = this.metrics.deliveriesSuccessful;
            this.metrics.avgDeliveryTime =
                ((this.metrics.avgDeliveryTime * (totalSuccessful - 1)) + deliveryTime) / totalSuccessful;
        } else {
            this.metrics.deliveriesFailed++;
        }
    }

    /**
     * Get delivery service metrics
     */
    getMetrics() {
        const uptime = Math.floor((new Date() - this.metrics.startTime) / 1000);
        const totalDeliveries = this.metrics.deliveriesSuccessful + this.metrics.deliveriesFailed;
        const successRate = totalDeliveries > 0 ?
            ((this.metrics.deliveriesSuccessful / totalDeliveries) * 100).toFixed(2) + '%' : '0%';

        return {
            uptime: `${Math.floor(uptime / 60)}m ${uptime % 60}s`,
            deliveries: {
                attempted: this.metrics.deliveriesAttempted,
                successful: this.metrics.deliveriesSuccessful,
                failed: this.metrics.deliveriesFailed,
                successRate
            },
            performance: {
                avgDeliveryTime: Math.round(this.metrics.avgDeliveryTime),
                retriesMade: this.metrics.retriesMade,
                circuitBreakerTrips: this.metrics.circuitBreakerTrips
            },
            activeDeliveries: this.activeDeliveries.size,
            circuitBreakers: Array.from(this.circuitBreakers.entries()).map(([id, breaker]) => ({
                destinationId: id,
                state: breaker.state,
                failureCount: breaker.failureCount,
                lastFailure: breaker.lastFailure ? new Date(breaker.lastFailure).toISOString() : null
            }))
        };
    }

    /**
     * Get active deliveries status
     */
    getActiveDeliveries() {
        return Array.from(this.activeDeliveries.entries()).map(([deliveryId, delivery]) => ({
            deliveryId,
            messageId: delivery.messageId,
            destination: {
                id: delivery.destination.id,
                type: delivery.destination.type,
                endpoint: `${delivery.destination.host}:${delivery.destination.port}`
            },
            duration: Date.now() - delivery.startTime,
            attempts: delivery.attempts
        }));
    }

    /**
     * Reset circuit breaker for destination
     */
    resetCircuitBreaker(destinationId) {
        const breaker = this.circuitBreakers.get(destinationId);
        if (breaker) {
            breaker.state = 'closed';
            breaker.failureCount = 0;
            breaker.lastFailure = null;
            console.log(`🔄 Circuit breaker reset for ${destinationId}`);
        }
    }

    /**
     * Test destination connectivity
     */
    async testDestination(destination) {
        const testMessage = {
            message_id: `test_${Date.now()}`,
            correlation_id: `test_correlation_${Date.now()}`
        };

        const testContent = JSON.stringify({
            test: true,
            timestamp: new Date().toISOString(),
            source: 'DeliveryService-ConnectivityTest'
        });

        try {
            const result = await this.deliverMessage(testMessage, destination, testContent);
            return {
                success: true,
                connectivity: 'healthy',
                responseTime: result.deliveryTime,
                details: result.response
            };
        } catch (error) {
            return {
                success: false,
                connectivity: 'unhealthy',
                error: error.message
            };
        }
    }
}

module.exports = DeliveryService;