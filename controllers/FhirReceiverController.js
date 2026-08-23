// controllers/FhirReceiverController.js
// FHIR Receiver HTTP endpoint controller
// Receives incoming FHIR resources via HTTP POST and stores them using the same
// architecture as HL7 messages (PostgreSQL metadata + MongoDB raw content)

const { v4: uuidv4 } = require('uuid');
const database = require('../config/database');

class FhirReceiverController {
    /**
     * Receive FHIR resource via HTTP POST
     * POST /fhir/receiver/:interfaceId
     */
    async receiveResource(req, res) {
        const { interfaceId } = req.params;
        const fhirResource = req.body;
        const startTime = Date.now();

        try {
            console.log(`📥 FHIR Receiver: Interface ${interfaceId} received ${fhirResource.resourceType || 'Unknown'} resource`);

            // 1. Load interface configuration
            const iface = await this.getInterfaceById(interfaceId);
            if (!iface) {
                return res.status(404).json({
                    resourceType: 'OperationOutcome',
                    issue: [{
                        severity: 'error',
                        code: 'not-found',
                        diagnostics: `FHIR receiver interface ${interfaceId} not found`
                    }]
                });
            }

            // 2. Validate interface is FHIR receiver type
            const sourceConn = iface.source_connectivity || {};
            if (!sourceConn.type || !sourceConn.type.includes('fhir')) {
                return res.status(400).json({
                    resourceType: 'OperationOutcome',
                    issue: [{
                        severity: 'error',
                        code: 'invalid',
                        diagnostics: 'This interface is not configured as a FHIR receiver'
                    }]
                });
            }

            // 3. Check if interface is active
            if (iface.status !== 'active') {
                return res.status(503).json({
                    resourceType: 'OperationOutcome',
                    issue: [{
                        severity: 'error',
                        code: 'business-rule',
                        diagnostics: `Interface is ${iface.status}, not active`
                    }]
                });
            }

            // 4. Authenticate request
            const authResult = await this.authenticateRequest(req, sourceConn);
            if (!authResult.success) {
                return res.status(401).json({
                    resourceType: 'OperationOutcome',
                    issue: [{
                        severity: 'error',
                        code: 'security',
                        diagnostics: authResult.error || 'Authentication failed'
                    }]
                });
            }

            // 5. Validate FHIR resource (basic validation)
            const validationResult = this.validateFhirResource(fhirResource, sourceConn);
            if (!validationResult.valid) {
                // If reject_invalid is true, return error
                if (sourceConn.validation?.reject_invalid) {
                    return res.status(400).json({
                        resourceType: 'OperationOutcome',
                        issue: validationResult.errors.map(err => ({
                            severity: 'error',
                            code: 'invalid',
                            diagnostics: err
                        }))
                    });
                }
                // Otherwise, store with validation warnings
                console.warn('⚠️ FHIR validation warnings:', validationResult.errors);
            }

            // 6. Generate message ID
            const messageId = `fhir_http_${Date.now()}${Math.floor(Math.random() * 1000000)}`;

            // 7. Store raw FHIR in MongoDB (async, non-blocking)
            this.storeInMongoDB(interfaceId, {
                message_id: messageId,
                raw_content: fhirResource,
                received_at: new Date(),
                source_type: 'fhir_http',
                source_endpoint: req.path,
                source_ip: req.ip || req.connection.remoteAddress,
                resource_type: fhirResource.resourceType,
                resource_id: fhirResource.id
            }).catch(err => {
                console.error('❌ MongoDB storage failed (non-fatal):', err.message);
            });

            // 8. Store metadata in PostgreSQL
            const tableName = `messages_intf_${interfaceId.replace(/-/g, '_')}`;

            await database.sequelize.query(`
                INSERT INTO ${tableName} (
                    message_id, interface_id, status, received_at,
                    source_type, source_endpoint, source_ip,
                    message_type, message_size, raw_message,
                    correlation_id
                ) VALUES (:messageId, :interfaceId, :status, :receivedAt,
                    :sourceType, :sourceEndpoint, :sourceIp,
                    :messageType, :messageSize, :rawMessage,
                    :correlationId)
            `, {
                replacements: {
                    messageId,
                    interfaceId,
                    status: 'received',
                    receivedAt: new Date(),
                    sourceType: 'fhir_http',
                    sourceEndpoint: req.path,
                    sourceIp: req.ip || req.connection.remoteAddress,
                    messageType: fhirResource.resourceType || 'Unknown',
                    messageSize: JSON.stringify(fhirResource).length,
                    rawMessage: JSON.stringify(fhirResource), // Fallback if MongoDB fails
                    correlationId: fhirResource.id || null // FHIR resource ID as correlation
                },
                type: database.sequelize.QueryTypes.INSERT
            });

            console.log(`✅ FHIR resource stored: ${messageId} (${Date.now() - startTime}ms)`);

            // 9. Trigger async processing (transformation pipeline) via Go backend
            // TODO: Call Go backend processing engine
            // this.triggerProcessing(interfaceId, messageId);

            // 10. Return FHIR OperationOutcome success
            res.status(201).json({
                resourceType: 'OperationOutcome',
                id: uuidv4(),
                meta: {
                    versionId: '1',
                    lastUpdated: new Date().toISOString()
                },
                issue: [{
                    severity: 'information',
                    code: 'informational',
                    diagnostics: `${fhirResource.resourceType} resource received and queued for processing`
                }],
                extension: [{
                    url: 'http://ezhealthkonnect.com/fhir/message-id',
                    valueString: messageId
                }]
            });

        } catch (error) {
            console.error('❌ FHIR receiver error:', error);
            res.status(500).json({
                resourceType: 'OperationOutcome',
                issue: [{
                    severity: 'error',
                    code: 'exception',
                    diagnostics: error.message || 'Internal server error'
                }]
            });
        }
    }

    /**
     * Update existing FHIR resource
     * PUT /fhir/receiver/:interfaceId/:resourceType/:resourceId
     */
    async updateResource(req, res) {
        const { interfaceId, resourceType, resourceId } = req.params;
        const fhirResource = req.body;

        // Validate resource type and ID match
        if (fhirResource.resourceType !== resourceType || fhirResource.id !== resourceId) {
            return res.status(400).json({
                resourceType: 'OperationOutcome',
                issue: [{
                    severity: 'error',
                    code: 'invalid',
                    diagnostics: 'Resource type or ID mismatch with URL'
                }]
            });
        }

        // For now, treat updates as new resources (store separately)
        // TODO: Implement version tracking
        return this.receiveResource(req, res);
    }

    /**
     * Get interface by ID
     */
    async getInterfaceById(interfaceId) {
        try {
            const result = await database.sequelize.query(
                'SELECT * FROM interfaces WHERE id = :interfaceId',
                { replacements: { interfaceId }, type: database.sequelize.QueryTypes.SELECT }
            );
            return result[0] || null;
        } catch (error) {
            console.error('Error loading interface:', error);
            return null;
        }
    }

    /**
     * Authenticate incoming request based on interface configuration
     */
    async authenticateRequest(req, sourceConnectivity) {
        const authConfig = sourceConnectivity.authentication;

        // If authentication is not enabled or configured
        if (!authConfig || !authConfig.enabled) {
            return { success: true };
        }

        const authType = authConfig.type;

        switch (authType) {
            case 'bearer_token':
                const authHeader = req.headers.authorization;
                if (!authHeader || !authHeader.startsWith('Bearer ')) {
                    return { success: false, error: 'Missing or invalid Authorization header' };
                }
                const token = authHeader.substring(7);
                if (token !== authConfig.token) {
                    return { success: false, error: 'Invalid Bearer token' };
                }
                return { success: true };

            case 'basic_auth':
                // TODO: Implement Basic Auth
                const basicAuth = req.headers.authorization;
                if (!basicAuth || !basicAuth.startsWith('Basic ')) {
                    return { success: false, error: 'Missing Basic authentication' };
                }
                // Decode and validate
                return { success: true }; // Placeholder

            case 'api_key':
                const apiKey = req.headers['x-api-key'];
                if (!apiKey || apiKey !== authConfig.api_key) {
                    return { success: false, error: 'Invalid API key' };
                }
                return { success: true };

            case 'oauth2':
                // TODO: Implement OAuth 2.0 validation
                return { success: false, error: 'OAuth 2.0 not yet implemented' };

            case 'none':
                return { success: true };

            default:
                return { success: false, error: `Unknown auth type: ${authType}` };
        }
    }

    /**
     * Validate FHIR resource structure
     */
    validateFhirResource(resource, sourceConnectivity) {
        const errors = [];

        // Basic validation
        if (!resource || typeof resource !== 'object') {
            errors.push('Invalid FHIR resource: not an object');
            return { valid: false, errors };
        }

        if (!resource.resourceType) {
            errors.push('Missing required field: resourceType');
        }

        // Check accepted resource types filter
        const acceptedTypes = sourceConnectivity.accepted_resource_types;
        if (acceptedTypes && acceptedTypes.length > 0 && !acceptedTypes.includes('all')) {
            if (!acceptedTypes.includes(resource.resourceType)) {
                errors.push(`Resource type ${resource.resourceType} not accepted by this interface`);
            }
        }

        // TODO: Add FHIR schema validation using library like @asymmetrik/fhir-validator
        // if (sourceConnectivity.validation?.validate_fhir_schema) {
        //     // Validate against FHIR schema
        // }

        return {
            valid: errors.length === 0,
            errors
        };
    }

    /**
     * Store raw FHIR in MongoDB
     */
    async storeInMongoDB(interfaceId, data) {
        try {
            // Dynamic import of MongoDB service
            const MongoDBService = require('../services/MongoDBConnectionService');
            const mongoService = await MongoDBService.getInstance();

            if (!mongoService) {
                console.warn('⚠️ MongoDB not available, skipping raw storage');
                return;
            }

            await mongoService.storeRawMessage(interfaceId, data);
            console.log(`✅ Stored in MongoDB: ${data.message_id}`);
        } catch (error) {
            console.error('❌ MongoDB storage error:', error.message);
            // Non-fatal - PostgreSQL still has the data
        }
    }

    /**
     * Trigger async processing via Go backend
     */
    async triggerProcessing(interfaceId, messageId) {
        // TODO: Call Go backend processing engine
        // POST http://localhost:8080/api/processing/trigger
        // Body: { interface_id, message_id }
    }
}

module.exports = new FhirReceiverController();
