// services/InterfaceListenerService.js
// Creates and manages port listeners for each interface based on their configuration

const http = require('http');
const net = require('net');
const { Client } = require('pg');
const InterfaceTableManager = require('./InterfaceTableManager');
const EnterpriseProcessingEngine = require('./EnterpriseProcessingEngine');

class InterfaceListenerService {
    constructor() {
        this.listeners = new Map(); // Store active listeners by interface ID
        this.dbConfig = {
            host: process.env.DB_HOST || 'localhost',
            port: parseInt(process.env.DB_PORT) || 5432,
            database: process.env.DB_NAME || 'ezhealthkonnect',
            user: process.env.DB_USER || 'ezhealth_user',
            password: process.env.DB_PASSWORD || 'secure_password_change_me'
        };

        // Initialize enterprise processing engine
        this.processingEngine = new EnterpriseProcessingEngine();
    }

    /**
     * Start all interface listeners based on database configuration
     */
    async startAllListeners() {
        console.log('🚀 Starting Interface Listener Service...');

        try {
            // Start the enterprise processing engine first
            console.log('🔧 Starting Enterprise Processing Engine...');
            await this.processingEngine.start();

            const client = new Client(this.dbConfig);
            await client.connect();

            // Get all active interfaces
            const result = await client.query(`
                SELECT id, name, source_type, source_config, target_config, processing_rules, status
                FROM interfaces
                WHERE status IN ('active', 'configured')
                ORDER BY created_at
            `);

            await client.end();

            for (const interfaceConfig of result.rows) {
                await this.startInterfaceListener(interfaceConfig);
            }

            console.log(`✅ Interface Listener Service started with ${this.listeners.size} active listeners`);
            console.log(`✅ Enterprise Processing Engine ready for message processing`);

        } catch (error) {
            console.error('❌ Failed to start Interface Listener Service:', error.message);
        }
    }

    /**
     * Start listener for a specific interface
     */
    async startInterfaceListener(interfaceConfig) {
        try {
            const { id, name, source_type, source_config } = interfaceConfig;
            const sourceConfig = typeof source_config === 'string'
                ? JSON.parse(source_config)
                : source_config;

            if (!sourceConfig.port) {
                console.log(`⚠️ No port configured for interface: ${name}`);
                return;
            }

            console.log(`🔧 Starting listener for ${name} (${source_type}:${sourceConfig.port})`);

            switch (source_type) {
                case 'http':
                    await this.startHttpListener(interfaceConfig, sourceConfig);
                    break;
                case 'tcp':
                    await this.startTcpListener(interfaceConfig, sourceConfig);
                    break;
                default:
                    console.log(`⚠️ Unsupported source type: ${source_type} for interface: ${name}`);
            }

        } catch (error) {
            console.error(`❌ Failed to start listener for ${interfaceConfig.name}:`, error.message);
        }
    }

    /**
     * Start HTTP listener for FHIR interfaces
     */
    async startHttpListener(interfaceConfig, sourceConfig) {
        const { id, name } = interfaceConfig;
        const port = sourceConfig.port;
        const path = sourceConfig.path || '/';

        const server = http.createServer(async (req, res) => {
            try {
                // Only handle configured path
                if (!req.url.startsWith(path)) {
                    res.writeHead(404, {'Content-Type': 'application/json'});
                    res.end(JSON.stringify({error: 'Path not found', expectedPath: path}));
                    return;
                }

                // Only handle POST requests for now
                if (req.method !== 'POST') {
                    res.writeHead(405, {'Content-Type': 'application/json'});
                    res.end(JSON.stringify({error: 'Method not allowed', allowedMethods: ['POST']}));
                    return;
                }

                // Read request body
                let body = '';
                req.on('data', chunk => {
                    body += chunk.toString();
                });

                req.on('end', async () => {
                    await this.handleFhirMessage(interfaceConfig, req, body, res);
                });

            } catch (error) {
                console.error(`❌ HTTP handler error for ${name}:`, error.message);
                res.writeHead(500, {'Content-Type': 'application/json'});
                res.end(JSON.stringify({error: 'Internal server error'}));
            }
        });

        server.listen(port, sourceConfig.host || 'localhost', () => {
            console.log(`✅ HTTP listener started: ${name} on ${sourceConfig.host || 'localhost'}:${port}${path}`);
            this.listeners.set(id, { type: 'http', server, config: interfaceConfig });
        });

        server.on('error', (error) => {
            if (error.code === 'EADDRINUSE') {
                console.log(`⚠️ Port ${port} already in use for ${name} - listener may already be running`);
            } else {
                console.error(`❌ HTTP server error for ${name}:`, error.message);
            }
        });
    }

    /**
     * Start TCP listener for HL7 interfaces
     */
    async startTcpListener(interfaceConfig, sourceConfig) {
        const { id, name } = interfaceConfig;
        const port = sourceConfig.port;

        const server = net.createServer((socket) => {
            console.log(`🔌 TCP connection established on ${name}:${port}`);

            socket.on('data', async (data) => {
                await this.handleHl7Message(interfaceConfig, socket, data);
            });

            socket.on('close', () => {
                console.log(`🔌 TCP connection closed on ${name}:${port}`);
            });

            socket.on('error', (error) => {
                console.error(`❌ TCP socket error for ${name}:`, error.message);
            });
        });

        server.listen(port, sourceConfig.host || 'localhost', () => {
            console.log(`✅ TCP listener started: ${name} on ${sourceConfig.host || 'localhost'}:${port}`);
            this.listeners.set(id, { type: 'tcp', server, config: interfaceConfig });
        });

        server.on('error', (error) => {
            if (error.code === 'EADDRINUSE') {
                console.log(`⚠️ Port ${port} already in use for ${name} - listener may already be running`);
            } else {
                console.error(`❌ TCP server error for ${name}:`, error.message);
            }
        });
    }

    /**
     * Handle FHIR message received on HTTP interface
     */
    async handleFhirMessage(interfaceConfig, req, body, res) {
        try {
            const { id, name, target_config, processing_rules } = interfaceConfig;
            const targetConfig = typeof target_config === 'string'
                ? JSON.parse(target_config)
                : target_config;
            const processingRules = typeof processing_rules === 'string'
                ? JSON.parse(processing_rules)
                : processing_rules;

            console.log(`🏥 FHIR message received on ${name}:`, body.substring(0, 100) + '...');

            // Parse FHIR message
            let fhirMessage;
            try {
                fhirMessage = JSON.parse(body);
            } catch (error) {
                res.writeHead(400, {'Content-Type': 'application/json'});
                res.end(JSON.stringify({error: 'Invalid JSON', details: error.message}));
                return;
            }

            // Generate message ID
            const messageId = `fhir_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
            const correlationId = req.headers['x-correlation-id'] || messageId;

            // Store in database using standardized schema
            const messageData = {
                interfaceId: id,
                interfaceName: name,
                messageId,
                correlationId,
                status: 'received',
                priority: 5,
                sourceType: req.headers['x-source-interface'] || 'http_endpoint',
                sourceEndpoint: `${req.headers.host || 'localhost'}${req.url}`,
                sourceIP: req.socket.remoteAddress,
                messageType: fhirMessage.resourceType || 'FHIR',
                messageSize: Buffer.byteLength(body, 'utf8'),
                messageEncoding: 'UTF-8',
                rawMessage: body,
                receivedAt: new Date()
            };

            await InterfaceTableManager.insertMessage(id, messageData);

            console.log(`✅ FHIR message stored: ${messageId} via InterfaceTableManager`);

            // Send success response
            res.writeHead(201, {'Content-Type': 'application/json'});
            res.end(JSON.stringify({
                success: true,
                messageId,
                correlationId,
                resourceType: fhirMessage.resourceType,
                resourceId: fhirMessage.id,
                status: 'stored',
                interface: { id, name },
                storedAt: new Date().toISOString()
            }));

        } catch (error) {
            console.error(`❌ Error handling FHIR message:`, error.message);
            res.writeHead(500, {'Content-Type': 'application/json'});
            res.end(JSON.stringify({error: 'Failed to process FHIR message', details: error.message}));
        }
    }

    /**
     * Handle HL7 message received on TCP interface
     */
    async handleHl7Message(interfaceConfig, socket, data) {
        try {
            const { id, name, target_config, processing_rules } = interfaceConfig;
            const targetConfig = typeof target_config === 'string'
                ? JSON.parse(target_config)
                : target_config;

            console.log(`📨 HL7 message received on ${name}:`, data.toString().substring(0, 100) + '...');

            const messageId = `hl7_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;

            // Store incoming HL7 message using standardized schema
            const messageData = {
                interfaceId: id,
                interfaceName: name,
                messageId,
                correlationId: messageId,
                status: 'received',
                priority: 5,
                sourceType: 'tcp_mllp',
                sourceEndpoint: `${socket.remoteAddress}:${socket.remotePort}`,
                sourceIP: socket.remoteAddress,
                messageType: 'ADT^A01', // TODO: Parse from HL7 MSH segment
                messageSize: Buffer.byteLength(data, 'utf8'),
                messageEncoding: 'UTF-8',
                rawMessage: data.toString(),
                receivedAt: new Date()
            };

            await InterfaceTableManager.insertMessage(id, messageData);

            // Send ACK response (basic MLLP ACK)
            const ack = `\x0BMSA|AA|${messageId}|\x1C\x0D`;
            socket.write(ack);

            console.log(`✅ HL7 message stored: ${messageId}`);

            // Process through Enterprise Processing Engine
            console.log(`🚀 Submitting message to Enterprise Processing Engine...`);
            try {
                const processingResult = await this.processingEngine.processMessageImmediate(
                    messageData,
                    interfaceConfig
                );

                if (processingResult.success) {
                    console.log(`✅ Enterprise processing completed for ${messageId} in ${processingResult.pipelineTime}ms`);
                } else {
                    console.error(`❌ Enterprise processing failed for ${messageId}: ${processingResult.error}`);
                }
            } catch (processingError) {
                console.error(`❌ Enterprise processing error for ${messageId}:`, processingError.message);
            }

        } catch (error) {
            console.error(`❌ Error handling HL7 message:`, error.message);
            // Send NACK response
            const nack = `\x0BMSA|AE|error|${error.message}|\x1C\x0D`;
            socket.write(nack);
        }
    }

    /**
     * Send transformed message to FHIR receiver
     */
    async sendToFhirReceiver(targetConfig, fhirMessage, correlationId) {
        try {
            const options = {
                hostname: targetConfig.host || 'localhost',
                port: targetConfig.port || 8080,
                path: targetConfig.path || '/fhir/Patient',
                method: 'POST',
                headers: {
                    'Content-Type': 'application/fhir+json',
                    'x-correlation-id': correlationId,
                    'x-source-interface': 'hl7-transform'
                }
            };

            const postData = JSON.stringify(fhirMessage);

            const req = http.request(options, (res) => {
                let data = '';
                res.on('data', (chunk) => data += chunk);
                res.on('end', () => {
                    console.log(`✅ FHIR message sent successfully: ${res.statusCode}`);
                    console.log(`📦 Response: ${data}`);
                });
            });

            req.on('error', (error) => {
                console.error('❌ Failed to send FHIR message:', error.message);
            });

            req.write(postData);
            req.end();

        } catch (error) {
            console.error('❌ Error sending to FHIR receiver:', error.message);
        }
    }

    /**
     * Stop all listeners
     */
    async stopAllListeners() {
        console.log('🛑 Stopping all interface listeners...');

        for (const [interfaceId, listener] of this.listeners) {
            try {
                listener.server.close();
                console.log(`✅ Stopped listener for interface: ${interfaceId}`);
            } catch (error) {
                console.error(`❌ Error stopping listener ${interfaceId}:`, error.message);
            }
        }

        this.listeners.clear();

        // Stop the enterprise processing engine
        console.log('🛑 Stopping Enterprise Processing Engine...');
        await this.processingEngine.stop();

        console.log('✅ All interface listeners and processing engine stopped');
    }

    /**
     * Get status of all listeners
     */
    getListenerStatus() {
        const status = [];
        for (const [interfaceId, listener] of this.listeners) {
            status.push({
                interfaceId,
                type: listener.type,
                name: listener.config.name,
                port: listener.config.source_config.port,
                status: 'active'
            });
        }
        return status;
    }
}

module.exports = InterfaceListenerService;