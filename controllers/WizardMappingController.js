// controller/WizardMappingController.js
// Controller for wizard-generated HL7-FHIR mapping configuration endpoints
// Integrates with existing ezHealthKonnect database schema and authentication

const WizardMappingService = require('../services/WizardMappingService');
const Joi = require('joi');

class WizardMappingController {
    constructor() {
        this.wizardMappingService = new WizardMappingService();
        //this.logger = require('../utils/logger') || console; // Fallback to console if logger not available
    }

    /**
     * Save wizard-generated mapping configuration to database
     * POST /api/wizard/save-mapping-config
     * 
     * Request Body:
     * {
     *   interfaceId: "uuid",
     *   wizardData: {
     *     detectedMessageType: "ADT^A01",
     *     enhancedSegments: {...},
     *     atomicMappings: [...],
     *     fhirTransformResult: {...}
     *   }
     * }
     */
    async saveMappingConfiguration(req, res) {
        const startTime = Date.now();
        
        try {
            this.logger.info('📥 Wizard mapping configuration save requested', {
                userId: req.user?.id,
                userEmail: req.user?.email,
                requestId: req.requestId || req.headers['x-request-id'],
                timestamp: new Date().toISOString()
            });

            // 1. Validate request structure
            const validationResult = this.validateSaveMappingRequest(req.body);
            if (validationResult.error) {
                this.logger.warn('❌ Request validation failed', {
                    errors: validationResult.error.details.map(d => d.message),
                    userId: req.user?.id
                });

                return res.status(400).json({
                    success: false,
                    error: 'Request validation failed',
                    details: validationResult.error.details.map(d => ({
                        field: d.path.join('.'),
                        message: d.message
                    }))
                });
            }

            const { interfaceId, wizardData } = validationResult.value;

            // 2. Verify interface exists and belongs to user
            const interfaceOwnership = await this.verifyInterfaceOwnership(interfaceId, req.user.id);
            if (!interfaceOwnership.valid) {
                this.logger.warn('⚠️ Interface access denied or not found', {
                    interfaceId,
                    userId: req.user.id,
                    reason: interfaceOwnership.reason
                });

                return res.status(interfaceOwnership.status).json({
                    success: false,
                    error: interfaceOwnership.message
                });
            }

            // 3. Pre-save validation of wizard data
            const dataValidation = await this.validateWizardData(wizardData);
            if (!dataValidation.isValid) {
                this.logger.warn('⚠️ Wizard data validation failed', {
                    interfaceId,
                    userId: req.user.id,
                    validationErrors: dataValidation.errors
                });

                return res.status(400).json({
                    success: false,
                    error: 'Wizard data validation failed',
                    details: dataValidation.errors,
                    warnings: dataValidation.warnings
                });
            }

            // 4. Save mapping configuration using service
            this.logger.info('💾 Starting mapping configuration save', {
                interfaceId,
                messageType: wizardData.detectedMessageType || wizardData.messageType,
                mappingCount: wizardData.atomicMappings?.length || 0,
                userId: req.user.id
            });

            const saveResult = await this.wizardMappingService.saveWizardConfiguration(
                interfaceId,
                {
                    ...wizardData,
                    savedBy: req.user.id,
                    savedAt: new Date().toISOString(),
                    userEmail: req.user.email
                }
            );

            // 5. Handle save result
            if (saveResult.success) {
                const processingTime = Date.now() - startTime;
                
                this.logger.info('✅ Mapping configuration saved successfully', {
                    interfaceId,
                    savedMappings: saveResult.savedMappings,
                    processingTimeMs: processingTime,
                    userId: req.user.id
                });

                return res.status(200).json({
                    success: true,
                    message: 'HL7-FHIR mapping configuration saved successfully',
                    data: {
                        interfaceId: saveResult.interfaceId,
                        mappingsSaved: saveResult.savedMappings,
                        processingTimeMs: processingTime,
                        savedAt: new Date().toISOString(),
                        details: {
                            fieldMappingIds: saveResult.data.fieldMappingIds,
                            templateId: saveResult.data.templateId,
                            valueSetMappingIds: saveResult.data.valueSetMappingIds
                        }
                    }
                });
            } else {
                this.logger.error('❌ Failed to save mapping configuration', {
                    interfaceId,
                    error: saveResult.error,
                    userId: req.user.id,
                    processingTimeMs: Date.now() - startTime
                });

                return res.status(500).json({
                    success: false,
                    error: 'Failed to save mapping configuration',
                    details: saveResult.error,
                    interfaceId: interfaceId
                });
            }

        } catch (error) {
            const processingTime = Date.now() - startTime;
            
            this.logger.error('💥 Unexpected error in mapping configuration save:', {
                error: error.message,
                stack: error.stack,
                userId: req.user?.id,
                processingTimeMs: processingTime,
                requestBody: req.body ? 'present' : 'missing'
            });

            return res.status(500).json({
                success: false,
                error: 'Internal server error while saving mapping configuration',
                requestId: req.requestId || req.headers['x-request-id']
            });
        }
    }

    /**
     * Get runtime configuration for Go services
     * GET /api/wizard/runtime-config/:interfaceId
     */
    async getRuntimeConfiguration(req, res) {
        try {
            const { interfaceId } = req.params;

            // Validate interface ID format
            if (!this.isValidUUID(interfaceId)) {
                return res.status(400).json({
                    success: false,
                    error: 'Invalid interface ID format. Must be a valid UUID.'
                });
            }

            // Verify access permissions (optional - depends on if Go services need auth)
            if (req.user) {
                const ownership = await this.verifyInterfaceOwnership(interfaceId, req.user.id);
                if (!ownership.valid && ownership.status === 404) {
                    return res.status(404).json({
                        success: false,
                        error: 'Interface not found'
                    });
                }
            }

            // Get runtime configuration from database
            const runtimeConfig = await this.getRuntimeConfigFromDatabase(interfaceId);

            if (!runtimeConfig) {
                return res.status(404).json({
                    success: false,
                    error: 'Runtime configuration not found for interface',
                    interfaceId: interfaceId
                });
            }

            this.logger.info('📋 Runtime configuration retrieved', {
                interfaceId,
                configVersion: runtimeConfig.version,
                messageType: runtimeConfig.messageType
            });

            return res.status(200).json({
                success: true,
                data: {
                    interfaceId: interfaceId,
                    configuration: runtimeConfig,
                    lastUpdated: runtimeConfig.lastUpdated,
                    version: runtimeConfig.version,
                    retrievedAt: new Date().toISOString()
                }
            });

        } catch (error) {
            this.logger.error('💥 Runtime configuration retrieval error:', {
                error: error.message,
                interfaceId: req.params.interfaceId,
                stack: error.stack
            });

            return res.status(500).json({
                success: false,
                error: 'Failed to retrieve runtime configuration'
            });
        }
    }

    /**
     * Get available FHIR templates by message type
     * GET /api/wizard/templates/:messageType
     */
    async getTemplatesByMessageType(req, res) {
        try {
            const { messageType } = req.params;

            // Validate message type
            if (!messageType || messageType.trim().length === 0) {
                return res.status(400).json({
                    success: false,
                    error: 'Message type parameter is required'
                });
            }

            // Sanitize message type (prevent injection)
            const sanitizedMessageType = messageType.replace(/[^A-Z0-9^_-]/gi, '');
            
            if (sanitizedMessageType !== messageType) {
                return res.status(400).json({
                    success: false,
                    error: 'Invalid message type format. Use alphanumeric characters, ^, _, and - only.'
                });
            }

            const templates = await this.getMessageTypeTemplates(sanitizedMessageType);

            this.logger.info('📚 Templates retrieved for message type', {
                messageType: sanitizedMessageType,
                templateCount: templates.length,
                userId: req.user?.id
            });

            return res.status(200).json({
                success: true,
                data: {
                    messageType: sanitizedMessageType,
                    templates: templates,
                    totalCount: templates.length,
                    retrievedAt: new Date().toISOString()
                }
            });

        } catch (error) {
            this.logger.error('💥 Template retrieval error:', {
                error: error.message,
                messageType: req.params.messageType,
                stack: error.stack
            });

            return res.status(500).json({
                success: false,
                error: 'Failed to retrieve templates for message type'
            });
        }
    }

    /**
     * Validate mapping configuration before saving
     * POST /api/wizard/validate-config
     */
    async validateConfiguration(req, res) {
        try {
            // Validate request structure first
            const structureValidation = this.validateSaveMappingRequest(req.body);
            if (structureValidation.error) {
                return res.status(400).json({
                    success: false,
                    error: 'Invalid request structure',
                    details: structureValidation.error.details.map(d => d.message)
                });
            }

            const { wizardData } = structureValidation.value;

            // Perform comprehensive configuration validation
            const configValidation = await this.performConfigurationValidation(wizardData);

            this.logger.info('🔍 Configuration validation completed', {
                isValid: configValidation.isValid,
                warningCount: configValidation.warnings.length,
                errorCount: configValidation.errors.length,
                userId: req.user?.id
            });

            return res.status(200).json({
                success: true,
                data: {
                    isValid: configValidation.isValid,
                    validationScore: configValidation.validationScore,
                    warnings: configValidation.warnings,
                    errors: configValidation.errors,
                    summary: configValidation.summary,
                    validatedAt: new Date().toISOString()
                }
            });

        } catch (error) {
            this.logger.error('💥 Configuration validation error:', {
                error: error.message,
                stack: error.stack
            });

            return res.status(500).json({
                success: false,
                error: 'Failed to validate configuration'
            });
        }
    }

    // ====================================
    // VALIDATION HELPER METHODS
    // ====================================

    /**
     * Validate save mapping request structure using Joi
     */
    validateSaveMappingRequest(requestBody) {
        const schema = Joi.object({
            interfaceId: Joi.string().uuid().required().messages({
                'string.guid': 'Interface ID must be a valid UUID',
                'any.required': 'Interface ID is required'
            }),
            wizardData: Joi.object({
                // Message type information
                detectedMessageType: Joi.string().optional(),
                messageType: Joi.string().optional(),
                
                // HL7 parsing results
                enhancedSegments: Joi.object().optional(),
                parsedMessage: Joi.object().optional(),
                
                // Field mappings from Step 4
                atomicMappings: Joi.array().items(Joi.object({
                    hl7Field: Joi.string().required(),
                    fhirPath: Joi.string().required(),
                    value: Joi.any().required(),
                    resourceType: Joi.string().required(),
                    transformationType: Joi.string().optional(),
                    validated: Joi.boolean().optional()
                })).optional(),
                
                // FHIR transformation results
                fhirTransformResult: Joi.object({
                    fhirResources: Joi.array().optional(),
                    bundle: Joi.object().optional(),
                    errors: Joi.array().optional(),
                    warnings: Joi.array().optional(),
                    success: Joi.boolean().optional()
                }).optional(),
                
                // Additional transformation data
                transformationData: Joi.object().optional(),
                mappingData: Joi.object().optional(),
                
                // User metadata
                savedBy: Joi.string().optional(),
                savedAt: Joi.string().optional(),
                userEmail: Joi.string().email().optional()
            }).required().messages({
                'any.required': 'Wizard data is required'
            })
        });

        return schema.validate(requestBody, { allowUnknown: true, abortEarly: false });
    }

    /**
     * Verify interface exists and user has access
     */
    async verifyInterfaceOwnership(interfaceId, userId) {
        try {
            const result = await this.wizardMappingService.pool.query(`
                SELECT 
                    id, 
                    name, 
                    user_id, 
                    status,
                    is_active 
                FROM interfaces 
                WHERE id = $1
            `, [interfaceId]);

            if (result.rows.length === 0) {
                return {
                    valid: false,
                    status: 404,
                    reason: 'interface_not_found',
                    message: 'Interface not found'
                };
            }

            const interface_record = result.rows[0];

            // Check if user owns the interface
            if (interface_record.user_id !== userId) {
                return {
                    valid: false,
                    status: 403,
                    reason: 'access_denied',
                    message: 'Access denied: Interface belongs to different user'
                };
            }

            // Check if interface is active
            if (!interface_record.is_active) {
                return {
                    valid: false,
                    status: 410,
                    reason: 'interface_inactive',
                    message: 'Interface is no longer active'
                };
            }

            return {
                valid: true,
                interface: interface_record
            };

        } catch (error) {
            this.logger.error('Database error in verifyInterfaceOwnership:', error);
            return {
                valid: false,
                status: 500,
                reason: 'database_error',
                message: 'Unable to verify interface ownership'
            };
        }
    }

    /**
     * Validate wizard data content and structure
     */
    async validateWizardData(wizardData) {
        const validation = {
            isValid: true,
            warnings: [],
            errors: [],
            validationScore: 100
        };

        // Check for message type
        const messageType = wizardData.detectedMessageType || wizardData.messageType;
        if (!messageType) {
            validation.errors.push('Message type is missing or could not be detected');
            validation.validationScore -= 20;
        }

        // Check for atomic mappings
        const atomicMappings = wizardData.atomicMappings || [];
        if (atomicMappings.length === 0) {
            validation.warnings.push('No field mappings found - interface may not process messages correctly');
            validation.validationScore -= 15;
        }

        // Validate individual mappings
        atomicMappings.forEach((mapping, index) => {
            if (!mapping.hl7Field || !mapping.fhirPath) {
                validation.errors.push(`Mapping ${index + 1} is missing HL7 field or FHIR path`);
                validation.validationScore -= 5;
            }
            if (!mapping.resourceType) {
                validation.errors.push(`Mapping ${index + 1} is missing FHIR resource type`);
                validation.validationScore -= 5;
            }
        });

        // Check for FHIR transformation result
        if (!wizardData.fhirTransformResult) {
            validation.warnings.push('FHIR transformation result not available - some mappings may be incomplete');
            validation.validationScore -= 10;
        }

        // Check enhanced segments
        if (!wizardData.enhancedSegments || Object.keys(wizardData.enhancedSegments).length === 0) {
            validation.warnings.push('Enhanced segment data not available - mapping quality may be reduced');
            validation.validationScore -= 10;
        }

        validation.isValid = validation.errors.length === 0 && validation.validationScore >= 50;

        return validation;
    }

    /**
     * Get runtime configuration from database for Go services
     */
    async getRuntimeConfigFromDatabase(interfaceId) {
        try {
            const result = await this.wizardMappingService.pool.query(`
                SELECT 
                    i.id,
                    i.name,
                    i.transformation_mapping,
                    i.message_type,
                    i.source_type,
                    i.target_type,
                    i.updated_at,
                    i.version,
                    i.is_active
                FROM interfaces i
                WHERE i.id = $1 AND i.is_active = true
            `, [interfaceId]);

            if (result.rows.length === 0) {
                return null;
            }

            const interface_data = result.rows[0];
            
            return {
                interfaceId: interface_data.id,
                name: interface_data.name,
                messageType: interface_data.message_type,
                sourceType: interface_data.source_type,
                targetType: interface_data.target_type,
                transformationMapping: interface_data.transformation_mapping,
                lastUpdated: interface_data.updated_at,
                version: interface_data.version || 1,
                isActive: interface_data.is_active
            };

        } catch (error) {
            this.logger.error('Database error in getRuntimeConfigFromDatabase:', error);
            throw error;
        }
    }

    /**
     * Get templates for specific message type from database
     */
    async getMessageTypeTemplates(messageType) {
        try {
            const result = await this.wizardMappingService.pool.query(`
                SELECT 
                    id,
                    message_type,
                    fhir_resources,
                    template_version,
                    description,
                    created_at,
                    updated_at
                FROM message_fhir_templates 
                WHERE message_type = $1 AND is_active = true
                ORDER BY updated_at DESC, created_at DESC
                LIMIT 10
            `, [messageType]);

            return result.rows.map(row => ({
                id: row.id,
                messageType: row.message_type,
                templateVersion: row.template_version,
                description: row.description,
                fhirResources: row.fhir_resources,
                createdAt: row.created_at,
                updatedAt: row.updated_at
            }));

        } catch (error) {
            this.logger.error('Database error in getMessageTypeTemplates:', error);
            throw error;
        }
    }

    /**
     * Perform comprehensive configuration validation
     */
    async performConfigurationValidation(wizardData) {
        const validation = await this.validateWizardData(wizardData);
        
        // Add summary
        validation.summary = {
            totalMappings: wizardData.atomicMappings?.length || 0,
            messageType: wizardData.detectedMessageType || wizardData.messageType || 'Unknown',
            hasSegmentData: !!(wizardData.enhancedSegments && Object.keys(wizardData.enhancedSegments).length > 0),
            hasFhirResult: !!wizardData.fhirTransformResult,
            validationScore: validation.validationScore
        };

        return validation;
    }

    /**
     * Utility: Check if string is valid UUID
     */
    isValidUUID(str) {
        const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
        return uuidRegex.test(str);
    }
}

module.exports = WizardMappingController;