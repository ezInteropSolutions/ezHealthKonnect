// services/connectors/input/TCPInputConnector.js
// TCP Input Connector for HL7 message processing

const net = require('net');
const BaseInputConnector = require('../BaseInputConnector');

class TCPInputConnector extends BaseInputConnector {
    constructor(config) {
        super({
            ...config,
            type: 'TCP'
        });

        this.server = null;
        this.connections = new Set();
        this.messageBuffer = new Map(); // Per-connection message buffers
        this.messageDelimiter = config.messageDelimiter || '\x1c'; // HL7 file separator
        this.acknowledgmentMode = config.acknowledgmentMode || 'auto'; // auto, manual, none
    }

    /**
     * Validate TCP connector configuration
     */
    validateConfig() {
        if (!this.config.host) {
            throw new Error('TCP connector requires host configuration');
        }

        if (!this.config.port || this.config.port <= 0 || this.config.port > 65535) {
            throw new Error('TCP connector requires valid port configuration');
        }

        console.log(`✅ TCP connector config validated: ${this.config.host}:${this.config.port}`);
    }

    /**
     * Create and start TCP server
     */
    async connect() {
        return new Promise((resolve, reject) => {
            try {
                console.log(`🔌 Starting TCP server on ${this.config.host}:${this.config.port}...`);

                this.server = net.createServer((socket) => {
                    this.handleNewConnection(socket);
                });

                // Server event handlers
                this.server.on('listening', () => {
                    console.log(`✅ TCP server listening on ${this.config.host}:${this.config.port}`);
                    this.setConnected(true);
                    resolve();
                });

                this.server.on('error', (error) => {
                    console.error('❌ TCP server error:', error);
                    this.emit('error', error);
                    if (!this.isConnected) {
                        reject(error);
                    }
                });

                this.server.on('close', () => {
                    console.log('🔌 TCP server closed');
                    this.setConnected(false);
                });

                // Start listening
                this.server.listen(this.config.port, this.config.host);

            } catch (error) {
                console.error('❌ Failed to create TCP server:', error);
                reject(error);
            }
        });
    }

    /**
     * Start listening (server is already listening after connect)
     */
    async startListening() {
        this.setListening(true);
        console.log(`👂 TCP connector actively listening for connections`);

        // Start health monitoring
        this.startHealthMonitoring(30000);
    }

    /**
     * Handle new client connection
     */
    handleNewConnection(socket) {
        const clientId = `${socket.remoteAddress}:${socket.remotePort}`;
        console.log(`🔗 New client connected: ${clientId}`);

        // Add to active connections
        this.connections.add(socket);
        this.messageBuffer.set(socket, '');

        // Connection event handlers
        socket.on('data', (data) => {
            this.handleIncomingData(socket, data);
        });

        socket.on('error', (error) => {
            console.error(`❌ Client connection error (${clientId}):`, error);
            this.cleanupConnection(socket);
        });

        socket.on('close', () => {
            console.log(`🔌 Client disconnected: ${clientId}`);
            this.cleanupConnection(socket);
        });

        socket.on('timeout', () => {
            console.log(`⏰ Client connection timeout: ${clientId}`);
            socket.destroy();
            this.cleanupConnection(socket);
        });

        // Set socket timeout if configured
        if (this.config.socketTimeout) {
            socket.setTimeout(this.config.socketTimeout);
        }

        this.emit('client:connected', { clientId, socket });
    }

    /**
     * Handle incoming data from client
     */
    handleIncomingData(socket, data) {
        const clientId = `${socket.remoteAddress}:${socket.remotePort}`;

        try {
            // Get existing buffer for this connection
            let buffer = this.messageBuffer.get(socket) || '';
            buffer += data.toString();

            // Process complete messages
            const messages = buffer.split(this.messageDelimiter);

            // Keep the last incomplete message in the buffer
            this.messageBuffer.set(socket, messages.pop() || '');

            // Process complete messages
            for (const message of messages) {
                if (message.trim()) {
                    this.processCompleteMessage(socket, message.trim(), clientId);
                }
            }

        } catch (error) {
            console.error(`❌ Error processing data from ${clientId}:`, error);
            this.emit('error', error);
        }
    }

    /**
     * Process a complete message
     */
    processCompleteMessage(socket, message, clientId) {
        console.log(`📨 Received message from ${clientId} (${message.length} chars)`);

        // Extract message metadata for HL7
        const messageContext = {
            client: clientId,
            socket: socket,
            receivedAt: new Date(),
            size: message.length
        };

        // Parse HL7 metadata if it's an HL7 message
        if (message.startsWith('MSH')) {
            const hl7Metadata = this.parseHL7Metadata(message);
            messageContext.hl7 = hl7Metadata;
        }

        // Send acknowledgment if required
        if (this.acknowledgmentMode === 'auto') {
            this.sendAcknowledgment(socket, message, messageContext);
        }

        // Process the message through the base class
        this.processIncomingMessage(message, messageContext);
    }

    /**
     * Parse HL7 message metadata
     */
    parseHL7Metadata(message) {
        try {
            const segments = message.split('\r');
            const mshSegment = segments[0];
            const fields = mshSegment.split('|');

            if (fields.length > 8) {
                return {
                    messageType: fields[8] || 'Unknown',
                    sendingApplication: fields[2] || 'Unknown',
                    receivingApplication: fields[4] || 'Unknown',
                    messageControlId: fields[9] || 'Unknown',
                    timestamp: fields[6] || new Date().toISOString()
                };
            }
        } catch (error) {
            console.warn('⚠️ Could not parse HL7 metadata:', error.message);
        }

        return { messageType: 'Unknown' };
    }

    /**
     * Send acknowledgment back to client
     */
    sendAcknowledgment(socket, originalMessage, context) {
        try {
            const ackMessage = this.buildAcknowledgment(originalMessage, context);

            socket.write(ackMessage + this.messageDelimiter, (error) => {
                if (error) {
                    console.error(`❌ Failed to send ACK to ${context.client}:`, error);
                } else {
                    console.log(`✅ ACK sent to ${context.client}`);
                }
            });

        } catch (error) {
            console.error('❌ Error building acknowledgment:', error);
        }
    }

    /**
     * Build HL7 acknowledgment message
     */
    buildAcknowledgment(originalMessage, context) {
        const timestamp = new Date().toISOString().replace(/[-:T]/g, '').substr(0, 14);
        const controlId = context.hl7?.messageControlId || 'ACK001';

        return `MSH|^~\\&|ezHealthKonnect|System|${context.hl7?.sendingApplication || 'Unknown'}|Client|${timestamp}||ACK|${controlId}|P|2.5\r` +
               `MSA|AA|${controlId}|Message accepted\r`;
    }

    /**
     * Cleanup connection resources
     */
    cleanupConnection(socket) {
        this.connections.delete(socket);
        this.messageBuffer.delete(socket);
    }

    /**
     * Stop listening for new connections
     */
    async stopListening() {
        this.setListening(false);
        this.stopHealthMonitoring();

        // Close all client connections
        for (const socket of this.connections) {
            socket.end();
        }

        this.connections.clear();
        this.messageBuffer.clear();

        console.log('⏸️ TCP connector stopped listening');
    }

    /**
     * Disconnect server
     */
    async disconnect() {
        return new Promise((resolve) => {
            if (!this.server) {
                resolve();
                return;
            }

            console.log('🔌 Shutting down TCP server...');

            this.server.close(() => {
                console.log('✅ TCP server shutdown complete');
                this.setConnected(false);
                resolve();
            });

            // Force close if server doesn't close gracefully
            setTimeout(() => {
                if (this.server && this.server.listening) {
                    this.server.unref();
                }
                resolve();
            }, 5000);
        });
    }

    /**
     * Health check - verify server is listening
     */
    async healthCheck() {
        if (!this.server || !this.server.listening) {
            return false;
        }

        // Additional health checks could include:
        // - Memory usage for message buffers
        // - Number of active connections
        // - Recent message activity

        return true;
    }

    /**
     * Get TCP-specific status information
     */
    getStatus() {
        const baseStatus = super.getStatus();

        return {
            ...baseStatus,
            serverListening: this.server?.listening || false,
            activeConnections: this.connections.size,
            bufferSizes: Array.from(this.messageBuffer.entries()).map(([socket, buffer]) => ({
                client: `${socket.remoteAddress}:${socket.remotePort}`,
                bufferSize: buffer.length
            })),
            config: {
                host: this.config.host,
                port: this.config.port,
                messageDelimiter: this.config.messageDelimiter,
                acknowledgmentMode: this.acknowledgmentMode
            }
        };
    }
}

module.exports = TCPInputConnector;