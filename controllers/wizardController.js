// controllers/wizardController.js
// FIXED: Database connection issue by using service layer consistently
// REMOVED: Direct database access that was causing "db.query is not a function" error
// UPDATED: Uses interfaceService for all database operations

const interfaceService = require('../services/interfaceService');
const auditService = require('../services/auditService');
const MessageTypeMappingService = require('../services/MessageTypeMappingService');
const { v4: uuidv4 } = require('uuid');

class WizardController {
    /**
     * Constructor - Bind methods to preserve 'this' context for Express route handlers
     */
    constructor() {
        this.saveConfiguration = this.saveConfiguration.bind(this);
        this.completeWizard = this.completeWizard.bind(this);
        this.listInterfaces = this.listInterfaces.bind(this);
        this.getInterface = this.getInterface.bind(this);
        this.deleteInterface = this.deleteInterface.bind(this);
        this.getInterfaceStats = this.getInterfaceStats.bind(this);
        this.checkDuplicateName = this.checkDuplicateName.bind(this);
        this.debugWizardData = this.debugWizardData.bind(this);

        // Initialize the message-type mapping service for HL7-FHIR mapping management
        this.mappingService = new MessageTypeMappingService();
    }

    /**
     * Activate interface (legacy route - now handled by completeWizard)
     */
    async activateInterface(req, res) {
        try {
            const { interfaceId } = req.body;
            const userId = req.session.user.id;

            if (!interfaceId) {
                return res.status(400).json({
                    success: false,
                    error: 'Interface ID is required'
                });
            }

            // Use interfaceService instead of direct database access
            const result = await interfaceService.activateInterface(interfaceId, userId);

            res.json({
                success: true,
                data: {
                    interfaceId: interfaceId,
                    name: result.name,
                    status: 'active',
                    message: 'Interface activated successfully'
                }
            });

        } catch (error) {
            console.error('Activate interface error:', error);
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to activate interface'
            });
        }
    }

    /**
     * Map UI source types to message formats and connectivity
     */
    mapSourceTypeAndConnectivity(uiSourceType, sourceConfig = {}) {
        const mappings = {
            'hl7v2': { format: 'hl7', connectivity: 'tcp' },
            'hl7': { format: 'hl7', connectivity: 'tcp' },
            'file': { format: 'flatfile', connectivity: 'file' },
            'http': { format: 'hl7', connectivity: 'http' },
            'database': { format: 'database', connectivity: 'database' },
            'manual': { format: 'hl7', connectivity: 'tcp' }
        };
        
        const mapping = mappings[uiSourceType] || { format: 'hl7', connectivity: 'tcp' };
        console.log(`Source mapping: ${uiSourceType} → format: ${mapping.format}, connectivity: ${mapping.connectivity}`);
        return mapping;
    }

    /**
     * Map UI target types to message formats and connectivity
     */
    mapTargetTypeAndConnectivity(uiTargetType, targetConfig = {}) {
        const mappings = {
            'fhir': { format: 'fhir', connectivity: 'http' },
            'database': { format: 'database', connectivity: 'database' },
            'file': { format: 'flatfile', connectivity: 'file' },
            'http': { format: 'fhir', connectivity: 'http' },
            'hl7': { format: 'hl7', connectivity: 'tcp' }
        };
        
        const mapping = mappings[uiTargetType] || { format: 'fhir', connectivity: 'http' };
        console.log(`Target mapping: ${uiTargetType} → format: ${mapping.format}, connectivity: ${mapping.connectivity}`);
        return mapping;
    }

    /**
     * Save wizard configuration
     */
    async saveConfiguration(req, res) {
        try {
            const { wizardData } = req.body;
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;

            console.log('\n=== SAVE WIZARD CONFIGURATION ===');
            console.log('User:', userEmail);

            if (!wizardData || !wizardData.name) {
                return res.status(400).json({
                    success: false,
                    error: 'Invalid wizard data: name is required'
                });
            }

            const sourceMapping = this.mapSourceTypeAndConnectivity(wizardData.sourceType, wizardData.sourceConfig);
            const targetMapping = this.mapTargetTypeAndConnectivity(wizardData.targetType, wizardData.targetConfig);

            const mappedData = {
                ...wizardData,
                sourceType: sourceMapping.format,
                sourceConnectivity: sourceMapping.connectivity,
                targetType: targetMapping.format,
                targetConnectivity: targetMapping.connectivity,
                status: 'draft'
            };

            const result = await interfaceService.saveWizardConfiguration(mappedData, userId, userEmail);

            await auditService.logActivity({
                userId: userId,
                action: 'wizard_config_saved',
                resource: 'interface',
                resourceId: result.interfaceId,
                details: `Configuration saved for interface: ${wizardData.name}`
            });

            res.json({
                success: true,
                data: {
                    interfaceId: result.interfaceId,
                    message: 'Configuration saved successfully',
                    status: 'draft',
                    nextStep: 'activation'
                }
            });

        } catch (error) {
            console.error('Save configuration error:', error);
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to save configuration'
            });
        }
    }

    /**
     * OPTIMIZED: Complete wizard method - clean JSON config storage
     * STORES: Consolidated config in transformation_mapping JSONB with minimal redundancy
     */
    async completeWizard(req, res) {
        try {
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;
            const wizardData = req.body.wizardData || req.body;

            console.log('\n=== OPTIMIZED WIZARD COMPLETION ===');
            console.log('User:', userEmail);
            console.log('Interface name:', wizardData?.name);
            console.log('Wizard data structure:', Object.keys(wizardData));

            // Validate required data
            if (!wizardData || !wizardData.name) {
                return res.status(400).json({
                    success: false,
                    error: 'Invalid wizard data: name is required'
                });
            }

            // Handle both legacy and optimized wizard data formats
            const isOptimizedFormat = wizardData.mappings && Array.isArray(wizardData.mappings);

            if (isOptimizedFormat) {
                console.log('📱 Processing optimized wizard format');
                return await this.handleOptimizedWizard(req, res, wizardData, userId, userEmail);
            } else {
                console.log('🔧 Processing legacy wizard format');
                // Continue with existing logic for legacy format
            }

            // Extract and clean mapping data
            const atomicMappings = this.extractAtomicMappings(wizardData);
            const messageType = this.extractMessageType(wizardData);
            const enhancedSegments = this.extractEnhancedSegments(wizardData);
            
            console.log(`Extracted ${atomicMappings.length} atomic mappings for storage`);

            // Map UI values to backend format
            const sourceMapping = this.mapSourceTypeAndConnectivity(wizardData.sourceType);
            const targetMapping = this.mapTargetTypeAndConnectivity(wizardData.targetType);

            // Build minimal source config (only connection-specific settings)
            const sourceConfig = {
                type: sourceMapping.format,
                connectivity: sourceMapping.connectivity,
                ...(wizardData.sourceConfig || {})
            };

            // Build minimal target config (only connection-specific settings)
            const targetConfig = {
                type: targetMapping.format,
                connectivity: targetMapping.connectivity,
                ...(wizardData.targetConfig || {})
            };

            // Build comprehensive transformation mapping (this is the main config)
            const transformationMapping = {
                // Core mapping data
                atomicMappings: atomicMappings,
                messageType: messageType,
                enhancedSegments: enhancedSegments,
                
                // Processing metadata
                version: '2.0',
                createdAt: new Date().toISOString(),
                createdBy: userEmail,
                
                // Source/Target summary for quick reference
                sourceType: sourceMapping.format,
                targetType: targetMapping.format,
                
                // Optional: FHIR transform result for validation/debugging
                fhirTransformResult: this.extractFhirTransformResult(wizardData),
                
                // Wizard metadata
                wizardVersion: '2.0',
                extractionSummary: {
                    atomicMappingsCount: atomicMappings.length,
                    segmentCount: Object.keys(enhancedSegments).length,
                    messageType: messageType
                }
            };

            console.log('Clean transformation mapping prepared:', {
                atomicMappingsCount: transformationMapping.atomicMappings.length,
                messageType: transformationMapping.messageType,
                segmentsCount: Object.keys(transformationMapping.enhancedSegments).length,
                configSize: JSON.stringify(transformationMapping).length
            });

            // Prepare clean interface data
            const interfaceData = {
                name: wizardData.name,
                description: wizardData.description || '',
                sourceType: sourceMapping.format,
                sourceConnectivity: sourceMapping.connectivity,
                targetType: targetMapping.format,
                targetConnectivity: targetMapping.connectivity,
                messageType: messageType,
                status: 'active',
                
                // Clean, separated configs
                sourceConfig: sourceConfig,
                targetConfig: targetConfig,
                transformationMapping: transformationMapping
            };

            // Create interface with clean separation of concerns
            const result = await this.createInterfaceWithMappings(interfaceData, userId, userEmail);

            console.log('Interface created with clean config storage:', {
                interfaceId: result.interfaceId,
                name: result.name,
                atomicMappingsCount: transformationMapping.atomicMappings.length,
                configSizeKB: Math.round(JSON.stringify(transformationMapping).length / 1024)
            });

            // Audit logging
            await auditService.logActivity({
                userId: userId,
                action: 'wizard_completed',
                resource: 'interface',
                resourceId: result.interfaceId,
                details: `Wizard completed for interface: ${wizardData.name}`,
                metadata: {
                    interfaceName: wizardData.name,
                    sourceType: sourceMapping.format,
                    targetType: targetMapping.format,
                    atomicMappingsCount: transformationMapping.atomicMappings.length,
                    messageType: messageType,
                    configVersion: '2.0'
                }
            });

            res.json({
                success: true,
                data: {
                    interfaceId: result.interfaceId,
                    name: wizardData.name,
                    status: 'active',
                    atomicMappingsCount: transformationMapping.atomicMappings.length,
                    messageType: messageType,
                    configVersion: '2.0',
                    message: 'Interface created and HL7-FHIR mappings saved successfully'
                }
            });
            
        } catch (error) {
            console.error('Wizard completion error:', error);
            
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to complete wizard'
            });
        }
    }

    /**
     * FIXED: Create interface with transformation mappings using service layer
     * FIXED: Handle different return formats from interfaceService
     */
    async createInterfaceWithMappings(interfaceData, userId, userEmail) {
        try {
            console.log('🔧 Creating interface with HL7-FHIR mappings...');
            console.log('Interface data:', {
                name: interfaceData.name,
                messageType: interfaceData.messageType,
                atomicMappingsCount: interfaceData.transformationMapping?.atomicMappings?.length || 0
            });

            // Step 1: Create the interface record using the existing service
            const serviceData = {
                name: interfaceData.name,
                description: interfaceData.description,
                sourceType: interfaceData.sourceType,
                sourceConnectivity: interfaceData.sourceConnectivity,
                targetType: interfaceData.targetType,
                targetConnectivity: interfaceData.targetConnectivity,
                messageType: interfaceData.messageType,
                status: interfaceData.status,
                sourceConfig: interfaceData.sourceConfig,
                targetConfig: interfaceData.targetConfig,
                transformationMapping: interfaceData.transformationMapping
            };

            console.log('📝 Creating interface record via interfaceService...');
            const result = await interfaceService.createInterface(serviceData, userId, userEmail);

            // Handle different possible return formats from the service
            let interfaceId, interfaceName;

            if (result && result.success && result.interfaceId) {
                interfaceId = result.interfaceId;
                interfaceName = result.name || interfaceData.name;
            } else if (result && result.id) {
                interfaceId = result.id;
                interfaceName = result.name || interfaceData.name;
            } else if (result && typeof result === 'string') {
                interfaceId = result;
                interfaceName = interfaceData.name;
            } else if (result && result.success === false) {
                throw new Error(result.error || 'Interface creation failed');
            } else {
                console.warn('Unexpected interfaceService result format:', result);
                interfaceId = result?.interfaceId || result?.id || result?.data?.id || result?.data?.interfaceId;
                interfaceName = result?.name || result?.data?.name || interfaceData.name;

                if (!interfaceId) {
                    throw new Error('Could not extract interface ID from service result');
                }
            }

            console.log('✅ Interface created successfully:', {
                interfaceId: interfaceId,
                name: interfaceName
            });

            // Step 2: Save message-type-specific HL7-FHIR mappings using MessageTypeMappingService
            if (interfaceData.transformationMapping) {
                console.log('💾 Saving message-type-specific HL7-FHIR mappings...');

                try {
                    const mappingResult = await this.mappingService.saveWizardConfiguration(
                        interfaceId,
                        {
                            messageType: interfaceData.messageType,
                            transformationMapping: interfaceData.transformationMapping
                        }
                    );

                    console.log('✅ Message-type mappings saved successfully:', {
                        messageType: mappingResult.messageType,
                        usesStandardTemplate: mappingResult.usesStandardTemplate,
                        mappingId: mappingResult.mappingId
                    });

                } catch (mappingError) {
                    console.error('⚠️ Failed to save message-type mappings:', mappingError.message);
                    console.log('📝 Interface was created successfully, but message-type mappings failed');
                    console.log('🔄 This can be retried later or mappings can be configured manually');

                    // Don't fail the entire operation - interface was created successfully
                    // The transformation mapping is still stored in the interfaces table as fallback
                }
            } else {
                console.log('ℹ️ No transformation mapping found, skipping message-type mapping save');
            }

            return {
                interfaceId: interfaceId,
                name: interfaceName
            };

        } catch (error) {
            console.error('Service layer error creating interface:', error);
            throw error;
        }
    }

    /**
     * Helper method to update transformation mapping using safe database access
     */
    async updateTransformationMapping(interfaceId, transformationMapping) {
        try {
            // Use a safe database connection approach
            const { Pool } = require('pg');
            
            // Get database configuration - this should work regardless of how db config is structured
            let dbConfig;
            try {
                dbConfig = require('../config/database');
                // If it exports a config object directly, use it
                if (typeof dbConfig === 'object' && !dbConfig.query) {
                    // It's a config object, not a pool
                } else if (dbConfig.pool) {
                    // It exports { pool: poolInstance }
                    dbConfig = dbConfig.pool.options || dbConfig;
                } else {
                    // Fall back to environment variables
                    dbConfig = {
                        host: process.env.DB_HOST || 'localhost',
                        port: process.env.DB_PORT || 5432,
                        database: process.env.DB_NAME || 'ezhealthkonnect',
                        user: process.env.DB_USER || 'postgres',
                        password: process.env.DB_PASSWORD || 'password'
                    };
                }
            } catch (configError) {
                console.warn('Could not load database config, using environment variables');
                dbConfig = {
                    host: process.env.DB_HOST || 'localhost',
                    port: process.env.DB_PORT || 5432,
                    database: process.env.DB_NAME || 'ezhealthkonnect',
                    user: process.env.DB_USER || 'postgres',
                    password: process.env.DB_PASSWORD || 'password'
                };
            }
            
            // Create a temporary pool for this operation
            const pool = new Pool(dbConfig);
            
            const updateQuery = `
                UPDATE interfaces 
                SET transformation_mapping = $1, updated_at = NOW()
                WHERE id = $2
            `;
            
            const result = await pool.query(updateQuery, [
                JSON.stringify(transformationMapping),
                interfaceId
            ]);
            
            await pool.end();
            
            console.log('Transformation mapping updated successfully');
            
        } catch (error) {
            console.error('Failed to update transformation mapping:', error);
            // Don't throw here - interface was created successfully
            console.warn('Interface created but transformation mapping update failed');
        }
    }

    /**
     * FIXED: Get interface with transformation mappings using safe database access
     */
    async getInterfaceWithMappings(interfaceId) {
        try {
            // First verify access through service layer
            const userInterface = await interfaceService.getInterfaceById(interfaceId, null);
            if (!userInterface) {
                throw new Error(`Interface ${interfaceId} not found`);
            }
            
            // If the service already returns transformation_mapping, use it
            if (userInterface.transformation_mapping || userInterface.transformationMapping) {
                const transformationMapping = userInterface.transformation_mapping || userInterface.transformationMapping || {};
                
                return {
                    interfaceId: userInterface.id,
                    name: userInterface.name,
                    description: userInterface.description,
                    sourceType: userInterface.source_type || userInterface.sourceType,
                    targetType: userInterface.target_type || userInterface.targetType,
                    messageType: userInterface.message_type || userInterface.messageType,
                    status: userInterface.status,
                    atomicMappings: transformationMapping.atomicMappings || [],
                    enhancedSegments: transformationMapping.enhancedSegments || {},
                    fhirTransformResult: transformationMapping.fhirTransformResult || null,
                    mappingVersion: transformationMapping.version,
                    createdAt: userInterface.created_at || userInterface.createdAt,
                    updatedAt: userInterface.updated_at || userInterface.updatedAt
                };
            }
            
            // If service doesn't return transformation_mapping, we'd need to query it separately
            // For now, return basic interface data
            return {
                interfaceId: userInterface.id,
                name: userInterface.name,
                description: userInterface.description,
                sourceType: userInterface.source_type || userInterface.sourceType,
                targetType: userInterface.target_type || userInterface.targetType,
                messageType: userInterface.message_type || userInterface.messageType,
                status: userInterface.status,
                atomicMappings: [],
                enhancedSegments: {},
                fhirTransformResult: null,
                mappingVersion: null,
                createdAt: userInterface.created_at || userInterface.createdAt,
                updatedAt: userInterface.updated_at || userInterface.updatedAt
            };
            
        } catch (error) {
            console.error('Failed to retrieve interface mappings:', error);
            throw error;
        }
    }

    /**
     * List interfaces - use service layer primarily
     */
    async listInterfaces(req, res) {
        try {
            const userId = req.session.user.id;
            
            // Use existing service first
            const interfaces = await interfaceService.getUserInterfaces(userId);
            
            // Transform the data to include mapping information if available
            const transformedInterfaces = interfaces.map(iface => ({
                id: iface.id,
                name: iface.name,
                description: iface.description,
                sourceType: iface.source_type || iface.sourceType,
                targetType: iface.target_type || iface.targetType,
                messageType: iface.message_type || iface.messageType,
                status: iface.status,
                hasMappings: !!(iface.transformation_mapping || iface.transformationMapping),
                mappingsCount: this.getMappingsCount(iface.transformation_mapping || iface.transformationMapping),
                createdAt: iface.created_at || iface.createdAt,
                updatedAt: iface.updated_at || iface.updatedAt
            }));
            
            res.json({
                success: true,
                data: transformedInterfaces,
                count: transformedInterfaces.length
            });
            
        } catch (error) {
            console.error('List interfaces error:', error);
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to list interfaces'
            });
        }
    }
    
    /**
     * Helper to get mappings count from transformation_mapping JSON
     */
    getMappingsCount(transformationMapping) {
        try {
            if (!transformationMapping) return 0;
            
            const mapping = typeof transformationMapping === 'string' 
                ? JSON.parse(transformationMapping) 
                : transformationMapping;
                
            return mapping.atomicMappings?.length || 0;
        } catch (error) {
            return 0;
        }
    }
    
    /**
     * Get interface details with mappings
     */
    async getInterface(req, res) {
        try {
            const userId = req.session.user.id;
            const interfaceId = req.params.id;
            
            if (!interfaceId) {
                return res.status(400).json({
                    success: false,
                    error: 'Interface ID is required'
                });
            }
            
            const interfaceData = await this.getInterfaceWithMappings(interfaceId);
            
            // Verify user owns this interface via interfaceService
            const userInterface = await interfaceService.getInterfaceById(interfaceId, userId);
            
            if (!userInterface) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found'
                });
            }
            
            res.json({
                success: true,
                data: interfaceData
            });
            
        } catch (error) {
            console.error('Get interface error:', error);
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to get interface'
            });
        }
    }
    
    /**
     * Delete interface
     */
    async deleteInterface(req, res) {
        try {
            const userId = req.session.user.id;
            const interfaceId = req.params.id;
            const userEmail = req.session.user.email;
            
            if (!interfaceId) {
                return res.status(400).json({
                    success: false,
                    error: 'Interface ID is required'
                });
            }
            
            const result = await interfaceService.deleteInterface(interfaceId, userId);
            
            await auditService.logActivity({
                userId: userId,
                action: 'interface_deleted',
                resource: 'interface',
                resourceId: interfaceId,
                details: `Interface deleted: ${result.name}`
            });
            
            res.json({
                success: true,
                message: 'Interface deleted successfully'
            });
            
        } catch (error) {
            console.error('Delete interface error:', error);
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to delete interface'
            });
        }
    }
    
    /**
     * Get interface statistics
     */
    async getInterfaceStats(req, res) {
        try {
            const userId = req.session.user.id;
            const interfaceId = req.params.id;
            
            const interfaceRecord = await this.getInterfaceWithMappings(interfaceId);
            
            res.json({
                success: true,
                data: {
                    interfaceId: interfaceRecord.interfaceId,
                    name: interfaceRecord.name,
                    status: interfaceRecord.status,
                    sourceType: interfaceRecord.sourceType,
                    targetType: interfaceRecord.targetType,
                    messageType: interfaceRecord.messageType,
                    mappingsCount: interfaceRecord.atomicMappings.length,
                    stats: {
                        totalProcessed: 0, // These would come from processing logs
                        successful: 0,
                        failed: 0,
                        successRate: '0%',
                        lastProcessed: null
                    }
                }
            });
            
        } catch (error) {
            console.error('Get stats error:', error);
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to get statistics'
            });
        }
    }

    /**
     * Check for duplicate interface names
     */
    async checkDuplicateName(req, res) {
        try {
            const { name } = req.body;
            const userId = req.session.user.id;

            if (!name || typeof name !== 'string' || !name.trim()) {
                return res.status(400).json({
                    success: false,
                    error: 'Interface name is required'
                });
            }

            const isDuplicate = await interfaceService.checkDuplicateName(name.trim(), userId);
            
            res.json({
                success: true,
                isDuplicate: isDuplicate,
                message: isDuplicate 
                    ? `Interface name "${name.trim()}" already exists. Please choose a different name.`
                    : `Interface name "${name.trim()}" is available.`
            });

        } catch (error) {
            console.error('Check duplicate name error:', error);
            res.json({
                success: true,
                isDuplicate: false,
                message: 'Unable to verify name uniqueness. Please ensure the name is unique.',
                warning: 'Duplicate check service temporarily unavailable'
            });
        }
    }

    /**
     * DEBUG ENDPOINT: Analyze wizard data structure
     */
    async debugWizardData(req, res) {
        try {
            const wizardData = req.body;
            
            const extractedData = {
                messageType: this.extractMessageType(wizardData),
                enhancedSegments: this.extractEnhancedSegments(wizardData),
                atomicMappings: this.extractAtomicMappings(wizardData),
                fhirTransformResult: this.extractFhirTransformResult(wizardData),
                parsedMessage: this.extractParsedMessage(wizardData)
            };
            
            const extractionSummary = {
                messageType: extractedData.messageType || 'MISSING',
                segmentCount: Object.keys(extractedData.enhancedSegments).length,
                mappingCount: extractedData.atomicMappings.length,
                hasFhirResult: !!extractedData.fhirTransformResult,
                hasParsedMessage: !!extractedData.parsedMessage
            };
            
            res.json({
                success: true,
                debug: {
                    requestAnalysis: {
                        topLevelKeys: Object.keys(wizardData || {}),
                        extracted: extractionSummary
                    },
                    sampleData: {
                        firstMapping: extractedData.atomicMappings[0] || null,
                        segmentKeys: Object.keys(extractedData.enhancedSegments).slice(0, 5)
                    }
                }
            });
            
        } catch (error) {
            console.error('Debug endpoint error:', error);
            res.status(500).json({
                success: false,
                error: error.message
            });
        }
    }

    // ====================================
    // HELPER METHODS FOR DATA EXTRACTION
    // ====================================

    extractMessageType(wizardData) {
        return wizardData.messageType || 
               wizardData.detectedMessageType ||
               wizardData.parsedMessage?.messageType?.name ||
               'ADT^A01';
    }

    extractEnhancedSegments(wizardData) {
        return wizardData.enhancedSegments ||
               wizardData.parsedMessage?.enhancedSegments ||
               {};
    }

    extractParsedMessage(wizardData) {
        return wizardData.parsedMessage ||
               wizardData.parsedHL7Data ||
               null;
    }

    extractAtomicMappings(wizardData) {
        console.log('Debugging: extractAtomicMappings called');
        console.log('WizardData available keys:', Object.keys(wizardData || {}));
        
        // Try direct access first
        if (Array.isArray(wizardData.atomicMappings) && wizardData.atomicMappings.length > 0) {
            console.log('Found atomicMappings directly:', wizardData.atomicMappings.length);
            return wizardData.atomicMappings;
        }
        
        // Try other possible locations
        const candidates = [
            wizardData.step4Data?.atomicMappings,
            wizardData.mappingConfiguration?.atomicMappings,
            wizardData.mappingData?.atomicMappings,
            wizardData.transformationData?.atomicMappings,
            wizardData.fhirTransformResult?.mappings,
            wizardData.mappings
        ];

        for (const candidate of candidates) {
            if (Array.isArray(candidate) && candidate.length > 0) {
                console.log(`Found ${candidate.length} atomic mappings in alternative location`);
                return candidate;
            }
        }

        console.warn('No atomic mappings found in wizard data');
        return [];
    }

    extractFhirTransformResult(wizardData) {
        return wizardData.fhirTransformResult ||
               wizardData.transformationData?.fhirTransformResult ||
               null;
    }

    /**
     * Get mapping templates for optimized wizard
     */
    async getMappingTemplates(req, res) {
        try {
            console.log('📋 Fetching mapping templates for optimized wizard');

            // Get templates from the model's OOB templates
            const oobTemplates = {
                adt_a01_basic: {
                    name: 'ADT^A01 - Patient Admission',
                    messageType: 'ADT^A01',
                    description: 'Standard patient admission message mapping',
                    mappings: [
                        { hl7Path: 'MSH.3', fhirPath: 'MessageHeader.source.name', required: true },
                        { hl7Path: 'MSH.4', fhirPath: 'MessageHeader.source.endpoint', required: true },
                        { hl7Path: 'PID.3', fhirPath: 'Patient.identifier[0].value', required: true },
                        { hl7Path: 'PID.5.1', fhirPath: 'Patient.name[0].family', required: true },
                        { hl7Path: 'PID.5.2', fhirPath: 'Patient.name[0].given[0]', required: true },
                        { hl7Path: 'PID.7', fhirPath: 'Patient.birthDate', required: false },
                        { hl7Path: 'PID.8', fhirPath: 'Patient.gender', required: false },
                        { hl7Path: 'PV1.2', fhirPath: 'Encounter.class.code', required: true },
                        { hl7Path: 'PV1.3', fhirPath: 'Encounter.location[0].location.reference', required: false }
                    ]
                },
                oru_r01_basic: {
                    name: 'ORU^R01 - Lab Results',
                    messageType: 'ORU^R01',
                    description: 'Standard lab results message mapping',
                    mappings: [
                        { hl7Path: 'MSH.3', fhirPath: 'MessageHeader.source.name', required: true },
                        { hl7Path: 'PID.3', fhirPath: 'Patient.identifier[0].value', required: true },
                        { hl7Path: 'PID.5.1', fhirPath: 'Patient.name[0].family', required: true },
                        { hl7Path: 'PID.5.2', fhirPath: 'Patient.name[0].given[0]', required: true },
                        { hl7Path: 'OBR.4.1', fhirPath: 'DiagnosticReport.code.coding[0].code', required: true },
                        { hl7Path: 'OBX.3.1', fhirPath: 'Observation.code.coding[0].code', required: true },
                        { hl7Path: 'OBX.5', fhirPath: 'Observation.valueString', required: true },
                        { hl7Path: 'OBX.6', fhirPath: 'Observation.valueQuantity.unit', required: false }
                    ]
                },
                adt_a03_basic: {
                    name: 'ADT^A03 - Patient Discharge',
                    messageType: 'ADT^A03',
                    description: 'Standard patient discharge message mapping',
                    mappings: [
                        { hl7Path: 'MSH.3', fhirPath: 'MessageHeader.source.name', required: true },
                        { hl7Path: 'PID.3', fhirPath: 'Patient.identifier[0].value', required: true },
                        { hl7Path: 'PID.5.1', fhirPath: 'Patient.name[0].family', required: true },
                        { hl7Path: 'PV1.36', fhirPath: 'Encounter.hospitalization.dischargeDisposition.coding[0].code', required: false },
                        { hl7Path: 'PV1.45', fhirPath: 'Encounter.period.end', required: true }
                    ]
                }
            };

            res.json({
                success: true,
                templates: oobTemplates
            });

        } catch (error) {
            console.error('❌ Failed to get mapping templates:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to load mapping templates'
            });
        }
    }

    /**
     * Validate final configuration before interface creation
     */
    async validateFinalConfig(req, res) {
        try {
            const config = req.body;
            console.log('🔍 Validating final configuration for interface creation');

            const validation = {
                isValid: true,
                errors: [],
                warnings: []
            };

            // Required field validation
            if (!config.name || config.name.trim().length < 3) {
                validation.errors.push('Interface name must be at least 3 characters');
                validation.isValid = false;
            }

            if (!config.sourceType) {
                validation.errors.push('Source type is required');
                validation.isValid = false;
            }

            if (!config.targetType) {
                validation.errors.push('Target type is required');
                validation.isValid = false;
            }

            if (!config.messageType) {
                validation.errors.push('Message type is required');
                validation.isValid = false;
            }

            // Configuration validation
            if (config.sourceConnectivity === 'tcp') {
                if (!config.sourceConfig?.host) {
                    validation.errors.push('Host is required for TCP connectivity');
                    validation.isValid = false;
                }
                if (!config.sourceConfig?.port || config.sourceConfig.port < 1 || config.sourceConfig.port > 65535) {
                    validation.errors.push('Valid port number (1-65535) is required');
                    validation.isValid = false;
                }
            }

            if (config.targetConnectivity === 'http') {
                if (!config.targetConfig?.endpoint) {
                    validation.errors.push('FHIR endpoint URL is required');
                    validation.isValid = false;
                } else {
                    try {
                        new URL(config.targetConfig.endpoint);
                    } catch {
                        validation.errors.push('Valid URL format required for FHIR endpoint');
                        validation.isValid = false;
                    }
                }
            }

            // Mapping validation
            if (!config.mappingTemplate && (!config.customMappings || config.customMappings.length === 0)) {
                validation.errors.push('Either select a template or create custom mappings');
                validation.isValid = false;
            }

            // Check for duplicate name
            try {
                const userId = req.session.user?.id;
                if (userId) {
                    const isDuplicate = await interfaceService.checkDuplicateName(config.name.trim(), userId);
                    if (isDuplicate) {
                        validation.errors.push(`Interface name "${config.name.trim()}" already exists. Please choose a different name.`);
                        validation.isValid = false;
                    }
                }
            } catch (error) {
                console.warn('Could not check for duplicate name:', error.message);
                validation.warnings.push('Could not verify name uniqueness');
            }

            res.json({
                success: true,
                validation: validation
            });

        } catch (error) {
            console.error('❌ Configuration validation failed:', error);
            res.status(500).json({
                success: false,
                error: 'Failed to validate configuration'
            });
        }
    }

    /**
     * Handle optimized wizard format from MVC wizard system
     */
    async handleOptimizedWizard(req, res, wizardData, userId, userEmail) {
        try {
            console.log('🚀 Processing optimized wizard data');

            // Build interface data from optimized format
            const interfaceData = {
                name: wizardData.name,
                description: wizardData.description || `HL7 to FHIR interface for ${wizardData.name}`,
                sourceType: wizardData.sourceType || 'hl7v2',
                sourceConnectivity: wizardData.sourceConnectivity || 'tcp',
                sourceConfig: wizardData.sourceConfig || {},
                targetType: wizardData.targetType || 'fhir',
                targetConnectivity: wizardData.targetConnectivity || 'http',
                targetConfig: wizardData.targetConfig || {},
                messageType: wizardData.messageType || 'ADT^A01',
                mappings: wizardData.mappings || wizardData.customMappings || [],
                status: wizardData.status || 'active'
            };

            console.log('Creating interface with optimized data:', {
                name: interfaceData.name,
                messageType: interfaceData.messageType,
                mappingsCount: wizardData.mappings?.length || 0
            });

            // Create the interface using the service
            const result = await interfaceService.createInterface(interfaceData, userId);
            const createdInterface = result.interface || result;
            const interfaceId = result.interfaceId || createdInterface.id;
            console.log('✅ Interface created successfully:', interfaceId);

            // Store message-type-specific mappings if provided
            if (wizardData.mappings && Array.isArray(wizardData.mappings) && wizardData.mappings.length > 0) {
                try {
                    console.log(`📋 Storing ${wizardData.mappings.length} mappings for message type ${wizardData.messageType}`);

                    await this.mappingService.storeInterfaceMessageMapping(
                        interfaceId,
                        wizardData.messageType || 'ADT^A01',
                        wizardData.mappings,
                        {
                            templateUsed: wizardData.templateUsed,
                            createdBy: userEmail,
                            customTemplate: !wizardData.templateUsed // true if custom mappings
                        }
                    );

                    console.log('✅ Message-type mappings stored successfully');
                } catch (mappingError) {
                    console.error('⚠️ Failed to store mappings (interface still created):', mappingError);
                }
            }

            // Log audit trail
            await auditService.logActivity({
                userId: userId,
                action: 'interface_created',
                resource: 'Interface',
                resourceId: interfaceId,
                details: `Interface '${wizardData.name}' created via optimized wizard`,
                metadata: {
                    wizardType: 'optimized',
                    messageType: wizardData.messageType,
                    mappingsCount: wizardData.mappings?.length || 0
                }
            });

            console.log('✅ Optimized wizard completion successful');

            res.json({
                success: true,
                interfaceId: interfaceId,
                interface: {
                    id: interfaceId,
                    name: createdInterface.name || wizardData.name,
                    description: createdInterface.description || wizardData.description,
                    messageType: createdInterface.message_type || wizardData.messageType,
                    status: createdInterface.status || wizardData.status,
                    mappingsCount: wizardData.mappings?.length || 0
                }
            });

        } catch (error) {
            console.error('❌ Optimized wizard completion failed:', error);
            res.status(500).json({
                success: false,
                error: error.message || 'Failed to create interface'
            });
        }
    }
}

module.exports = new WizardController();