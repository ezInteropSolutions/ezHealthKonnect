// controllers/interfacesController.js - UPDATED WITH CONNECTIVITY SUPPORT
console.log('🔧 Loading Enhanced Interfaces Controller with Format + Connectivity Support...');

class InterfacesController {
    constructor() {
        console.log('🔍 Constructor called...');
        
        try {
            console.log('🔍 Requiring database module...');
            this.database = require('../config/database');
            console.log('🔍 Database module type:', typeof this.database);
            console.log('🔍 Database module keys:', Object.keys(this.database));
            console.log('🔍 Database.sequelize exists:', !!this.database.sequelize);
            console.log('🔍 Database.isConnected:', this.database.isConnected);
            console.log('✅ Enhanced Interfaces Controller initialized with Connectivity Support');
        } catch (error) {
            console.error('❌ Error in constructor:', error);
            throw error;
        }
    }

    /**
     * Ensure database is connected
     */
    async ensureDatabase() {
        if (!this.database) {
            console.log('🔗 Database not available, re-requiring...');
            this.database = require('../config/database');
        }
        
        if (!this.database.isConnected) {
            console.log('🔗 Database not connected, connecting now...');
            await this.database.connect();
        }
        
        if (!this.database || !this.database.sequelize) {
            throw new Error('Database not available');
        }
    }

    /**
     * Get all interfaces for the authenticated user
     * ✅ UPDATED: Now returns connectivity fields
     */
    async getAllInterfaces(req, res) {
        console.log('\n=== GET ALL INTERFACES (WITH CONNECTIVITY) ===');
        console.log('🔍 Session user:', req.session?.user?.email);
        console.log('🔍 this.database exists:', !!this.database);
        console.log('🔍 this.database.sequelize exists:', !!this.database?.sequelize);

        try {
            await this.ensureDatabase();
            
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;
            
            console.log(`🔍 Fetching interfaces for user: ${userEmail} (ID: ${userId})`);

            // ✅ UPDATED: Include connectivity fields in SELECT
            const interfaces = await this.database.sequelize.query(`
                SELECT 
                    i.id,
                    i.user_id,
                    i.name,
                    i.description,
                    i.source_type,
                    i.source_connectivity,
                    i.target_type,
                    i.target_connectivity,
                    i.source_config,
                    i.target_config,
                    i.message_type,
                    i.processing_rules,
                    i.transformation_mapping,
                    i.status,
                    i.total_processed,
                    i.successful_processed,
                    i.failed_processed,
                    i.last_processed_at,
                    i.created_at,
                    i.updated_at,
                    i.version,
                    u.email as created_by_email
                FROM interfaces i
                LEFT JOIN users u ON i.created_by = u.id
                WHERE i.user_id = :userId AND i.is_active = true
                ORDER BY i.updated_at DESC
            `, {
                replacements: { userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            console.log(`✅ Found ${interfaces.length} interfaces for user: ${userEmail}`);
            
            // ✅ UPDATED: Transform data with connectivity fields
            const transformedInterfaces = interfaces.map(item => ({
                id: item.id,
                userId: item.user_id,
                name: item.name,
                description: item.description,
                
                // ✅ UPDATED: Include source and target connectivity
                sourceType: item.source_type,
                sourceConnectivity: item.source_connectivity,
                targetType: item.target_type,
                targetConnectivity: item.target_connectivity,
                
                sourceConfig: this.parseJsonField(item.source_config),
                targetConfig: this.parseJsonField(item.target_config),
                messageType: item.message_type,
                processingRules: this.parseJsonField(item.processing_rules),
                transformationMapping: this.parseJsonField(item.transformation_mapping),
                status: item.status,
                statistics: {
                    totalProcessed: item.total_processed || 0,
                    successful: item.successful_processed || 0,
                    failed: item.failed_processed || 0,
                    lastProcessed: item.last_processed_at
                },
                createdAt: item.created_at,
                updatedAt: item.updated_at,
                lastUpdated: item.updated_at, // Frontend compatibility
                lastActivity: item.last_processed_at, // Frontend compatibility
                createdBy: item.created_by_email || 'Unknown',
                version: item.version || 1
            }));
            
            return res.json({
                success: true,
                interfaces: transformedInterfaces,
                total: transformedInterfaces.length,
                timestamp: new Date().toISOString()
            });

        } catch (error) {
            console.error('❌ Get Interfaces Error:', error);
            console.error('❌ Error stack:', error.stack);
            return res.status(500).json({
                success: false,
                error: 'Failed to retrieve interfaces',
                debug: error.message
            });
        }
    }

    /**
     * Create a new interface
     * ✅ FIXED: Better connectivity field handling with defaults
     */
    async createInterface(req, res) {
        console.log('\n=== CREATE INTERFACE WITH CONNECTIVITY ===');
        
        try {
            await this.ensureDatabase();
            
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;
            
            const {
                name,
                description,
                sourceType,
                sourceConnectivity,  // ✅ NEW: From request
                targetType,
                targetConnectivity,  // ✅ NEW: From request
                messageType,
                sourceConfig,
                targetConfig,
                processingRules,
                transformationMapping
            } = req.body;

            console.log(`🔍 Creating interface: ${name}`);
            console.log(`   User: ${userEmail}`);
            console.log(`   Source: ${sourceType} via ${sourceConnectivity || 'default'}`);
            console.log(`   Target: ${targetType} via ${targetConnectivity || 'default'}`);

            // ✅ ENHANCED: Apply defaults if connectivity not specified
            const finalSourceConnectivity = sourceConnectivity || this.getDefaultConnectivity('source', sourceType);
            const finalTargetConnectivity = targetConnectivity || this.getDefaultConnectivity('target', targetType);

            console.log(`   Final Source Connectivity: ${finalSourceConnectivity}`);
            console.log(`   Final Target Connectivity: ${finalTargetConnectivity}`);

            // Validation
            if (!name || !sourceType || !targetType) {
                return res.status(400).json({
                    success: false,
                    error: 'Missing required fields: name, sourceType, targetType'
                });
            }

            // Check for duplicate name
            const existingInterface = await this.database.sequelize.query(`
                SELECT id FROM interfaces 
                WHERE user_id = :userId AND name = :name AND is_active = true
            `, {
                replacements: { userId, name },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (existingInterface.length > 0) {
                return res.status(400).json({
                    success: false,
                    error: `Interface with name "${name}" already exists`
                });
            }

            // ✅ UPDATED: Insert with connectivity fields
            const newInterfaces = await this.database.sequelize.query(`
                INSERT INTO interfaces (
                    user_id, name, description, 
                    source_type, source_connectivity, 
                    target_type, target_connectivity, 
                    message_type, source_config, target_config, 
                    processing_rules, transformation_mapping, 
                    status, created_by, updated_by, created_at, updated_at, is_active
                ) VALUES (
                    :userId, :name, :description, 
                    :sourceType, :sourceConnectivity, 
                    :targetType, :targetConnectivity, 
                    :messageType, :sourceConfig, :targetConfig, 
                    :processingRules, :transformationMapping, 
                    'inactive', :userId, :userId, NOW(), NOW(), true
                ) RETURNING *
            `, {
                replacements: {
                    userId,
                    name,
                    description: description || '',
                    sourceType,
                    sourceConnectivity: finalSourceConnectivity,  // ✅ Use final value
                    targetType,
                    targetConnectivity: finalTargetConnectivity,  // ✅ Use final value
                    messageType: messageType || 'auto-detect',
                    sourceConfig: JSON.stringify(sourceConfig || {}),
                    targetConfig: JSON.stringify(targetConfig || {}),
                    processingRules: JSON.stringify(processingRules || {}),
                    transformationMapping: JSON.stringify(transformationMapping || {})
                },
                type: this.database.sequelize.QueryTypes.SELECT
            });
            
            const newInterfaceItem = newInterfaces[0];
            console.log(`✅ Interface Created - User: ${userEmail}, Interface: ${name} (${newInterfaceItem.id})`);
            console.log(`   Source: ${newInterfaceItem.source_type} via ${newInterfaceItem.source_connectivity}`);
            console.log(`   Target: ${newInterfaceItem.target_type} via ${newInterfaceItem.target_connectivity}`);

            // ✅ UPDATED: Transform for frontend with connectivity fields
            const responseInterface = {
                id: newInterfaceItem.id,
                userId: newInterfaceItem.user_id,
                name: newInterfaceItem.name,
                description: newInterfaceItem.description,
                
                // ✅ UPDATED: Include connectivity fields
                sourceType: newInterfaceItem.source_type,
                sourceConnectivity: newInterfaceItem.source_connectivity,
                targetType: newInterfaceItem.target_type,
                targetConnectivity: newInterfaceItem.target_connectivity,
                
                sourceConfig: this.parseJsonField(newInterfaceItem.source_config),
                targetConfig: this.parseJsonField(newInterfaceItem.target_config),
                messageType: newInterfaceItem.message_type,
                processingRules: this.parseJsonField(newInterfaceItem.processing_rules),
                transformationMapping: this.parseJsonField(newInterfaceItem.transformation_mapping),
                status: newInterfaceItem.status,
                statistics: {
                    totalProcessed: 0,
                    successful: 0,
                    failed: 0,
                    lastProcessed: null
                },
                createdAt: newInterfaceItem.created_at,
                updatedAt: newInterfaceItem.updated_at,
                version: newInterfaceItem.version || 1
            };

            return res.status(201).json({
                success: true,
                interface: responseInterface,
                message: `Interface "${name}" created successfully`
            });

        } catch (error) {
            console.error('❌ Create Interface Error:', error);
            return res.status(500).json({
                success: false,
                error: 'Failed to create interface',
                debug: error.message
            });
        }
    }

    /**
     * Get a specific interface by ID
     * ✅ UPDATED: Include connectivity fields
     */
    async getInterface(req, res) {
        console.log('\n=== GET SPECIFIC INTERFACE ===');
        
        try {
            await this.ensureDatabase();
            
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;
            const interfaceId = req.params.interfaceId;

            console.log(`🔍 Fetching interface ${interfaceId} for user: ${userEmail}`);

            const interfaceData = await this.database.sequelize.query(`
                SELECT 
                    i.id, i.user_id, i.name, i.description,
                    i.source_type, i.source_connectivity,
                    i.target_type, i.target_connectivity,
                    i.source_config, i.target_config,
                    i.message_type, i.processing_rules, i.transformation_mapping,
                    i.status, i.total_processed, i.successful_processed, i.failed_processed,
                    i.last_processed_at, i.created_at, i.updated_at, i.version,
                    u.email as created_by_email
                FROM interfaces i
                LEFT JOIN users u ON i.created_by = u.id
                WHERE i.id = :interfaceId AND i.user_id = :userId AND i.is_active = true
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (interfaceData.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found'
                });
            }

            const item = interfaceData[0];
            const transformedInterface = {
                id: item.id,
                userId: item.user_id,
                name: item.name,
                description: item.description,
                
                // ✅ Include connectivity fields
                sourceType: item.source_type,
                sourceConnectivity: item.source_connectivity,
                targetType: item.target_type,
                targetConnectivity: item.target_connectivity,
                
                sourceConfig: this.parseJsonField(item.source_config),
                targetConfig: this.parseJsonField(item.target_config),
                messageType: item.message_type,
                processingRules: this.parseJsonField(item.processing_rules),
                transformationMapping: this.parseJsonField(item.transformation_mapping),
                status: item.status,
                statistics: {
                    totalProcessed: item.total_processed || 0,
                    successful: item.successful_processed || 0,
                    failed: item.failed_processed || 0,
                    lastProcessed: item.last_processed_at
                },
                createdAt: item.created_at,
                updatedAt: item.updated_at,
                createdBy: item.created_by_email || 'Unknown',
                version: item.version || 1
            };

            console.log(`✅ Found interface: ${item.name}`);
            return res.json({
                success: true,
                interface: transformedInterface
            });

        } catch (error) {
            console.error('❌ Get Interface Error:', error);
            return res.status(500).json({
                success: false,
                error: 'Failed to retrieve interface',
                debug: error.message
            });
        }
    }

    /**
     * Start an interface
     */
    async startInterface(req, res) {
        console.log('\n=== START INTERFACE ===');
        
        try {
            await this.ensureDatabase();
            
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;
            const interfaceId = req.params.interfaceId;

            console.log(`🔍 Starting interface ${interfaceId} for user: ${userEmail}`);

            // Get current interface
            const interfaceData = await this.database.sequelize.query(`
                SELECT id, name, status FROM interfaces 
                WHERE id = :interfaceId AND user_id = :userId AND is_active = true
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (interfaceData.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found'
                });
            }

            const interfaceItem = interfaceData[0];
            
            if (interfaceItem.status === 'active') {
                return res.status(400).json({
                    success: false,
                    error: 'Interface is already active'
                });
            }

            // Update status to active
            await this.database.sequelize.query(`
                UPDATE interfaces 
                SET status = 'active', updated_by = :userId, updated_at = NOW()
                WHERE id = :interfaceId
            `, {
                replacements: { userId, interfaceId },
                type: this.database.sequelize.QueryTypes.UPDATE
            });

            console.log(`✅ Interface Started - User: ${userEmail}, Interface: ${interfaceItem.name}`);

            return res.json({
                success: true,
                message: `Interface "${interfaceItem.name}" started successfully`,
                status: 'active'
            });

        } catch (error) {
            console.error('❌ Start Interface Error:', error);
            return res.status(500).json({
                success: false,
                error: 'Failed to start interface',
                debug: error.message
            });
        }
    }

    /**
     * Stop an interface
     */
    async stopInterface(req, res) {
        console.log('\n=== STOP INTERFACE ===');
        
        try {
            await this.ensureDatabase();
            
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;
            const interfaceId = req.params.interfaceId;

            console.log(`🔍 Stopping interface ${interfaceId} for user: ${userEmail}`);

            // Get current interface
            const interfaceData = await this.database.sequelize.query(`
                SELECT id, name, status FROM interfaces 
                WHERE id = :interfaceId AND user_id = :userId AND is_active = true
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (interfaceData.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found'
                });
            }

            const interfaceItem = interfaceData[0];
            
            if (interfaceItem.status === 'inactive') {
                return res.status(400).json({
                    success: false,
                    error: 'Interface is already inactive'
                });
            }

            // Update status to inactive
            await this.database.sequelize.query(`
                UPDATE interfaces 
                SET status = 'inactive', updated_by = :userId, updated_at = NOW()
                WHERE id = :interfaceId
            `, {
                replacements: { userId, interfaceId },
                type: this.database.sequelize.QueryTypes.UPDATE
            });

            console.log(`✅ Interface Stopped - User: ${userEmail}, Interface: ${interfaceItem.name}`);

            return res.json({
                success: true,
                message: `Interface "${interfaceItem.name}" stopped successfully`,
                status: 'inactive'
            });

        } catch (error) {
            console.error('❌ Stop Interface Error:', error);
            return res.status(500).json({
                success: false,
                error: 'Failed to stop interface',
                debug: error.message
            });
        }
    }

    /**
     * Pause an interface
     */
    async pauseInterface(req, res) {
        console.log('\n=== PAUSE INTERFACE ===');
        
        try {
            await this.ensureDatabase();
            
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;
            const interfaceId = req.params.interfaceId;

            console.log(`🔍 Pausing interface ${interfaceId} for user: ${userEmail}`);

            // Get current interface
            const interfaceData = await this.database.sequelize.query(`
                SELECT id, name, status FROM interfaces 
                WHERE id = :interfaceId AND user_id = :userId AND is_active = true
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (interfaceData.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found'
                });
            }

            const interfaceItem = interfaceData[0];
            
            if (interfaceItem.status === 'paused') {
                return res.status(400).json({
                    success: false,
                    error: 'Interface is already paused'
                });
            }

            if (interfaceItem.status === 'inactive') {
                return res.status(400).json({
                    success: false,
                    error: 'Cannot pause an inactive interface. Please start it first.'
                });
            }

            // Update status to paused
            await this.database.sequelize.query(`
                UPDATE interfaces 
                SET status = 'paused', updated_by = :userId, updated_at = NOW()
                WHERE id = :interfaceId
            `, {
                replacements: { userId, interfaceId },
                type: this.database.sequelize.QueryTypes.UPDATE
            });

            console.log(`✅ Interface Paused - User: ${userEmail}, Interface: ${interfaceItem.name}`);

            return res.json({
                success: true,
                message: `Interface "${interfaceItem.name}" paused successfully`,
                status: 'paused'
            });

        } catch (error) {
            console.error('❌ Pause Interface Error:', error);
            return res.status(500).json({
                success: false,
                error: 'Failed to pause interface',
                debug: error.message
            });
        }
    }

    /**
     * Delete an interface (soft delete)
     */
    async deleteInterface(req, res) {
        console.log('\n=== DELETE INTERFACE ===');
        
        try {
            await this.ensureDatabase();
            
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;
            const interfaceId = req.params.interfaceId;

            console.log(`🔍 Deleting interface ${interfaceId} for user: ${userEmail}`);

            // Get current interface
            const interfaceData = await this.database.sequelize.query(`
                SELECT id, name, status FROM interfaces 
                WHERE id = :interfaceId AND user_id = :userId AND is_active = true
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (interfaceData.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found'
                });
            }

            const interfaceItem = interfaceData[0];
            
            // Check if interface is running
            if (interfaceItem.status === 'active') {
                return res.status(400).json({
                    success: false,
                    error: 'Cannot delete an active interface. Please stop it first.'
                });
            }

            // Soft delete
            await this.database.sequelize.query(`
                UPDATE interfaces 
                SET is_active = false, updated_by = :userId, updated_at = NOW()
                WHERE id = :interfaceId
            `, {
                replacements: { userId, interfaceId },
                type: this.database.sequelize.QueryTypes.UPDATE
            });

            console.log(`✅ Interface Deleted - User: ${userEmail}, Interface: ${interfaceItem.name}`);

            return res.json({
                success: true,
                message: `Interface "${interfaceItem.name}" deleted successfully`
            });

        } catch (error) {
            console.error('❌ Delete Interface Error:', error);
            return res.status(500).json({
                success: false,
                error: 'Failed to delete interface',
                debug: error.message
            });
        }
    }

    // ✅ HELPER METHODS

    /**
     * Get default connectivity for a given type (same logic as wizard controller)
     */
    getDefaultConnectivity(direction, type) {
        if (direction === 'source') {
            const mappings = {
                'hl7v2': 'tcp',
                'hl7': 'tcp',
                'file': 'file',
                'http': 'http',
                'database': 'database',
                'manual': 'tcp'
            };
            return mappings[type] || 'tcp';
        } else {
            const mappings = {
                'fhir': 'http',
                'database': 'database',
                'file': 'file',
                'http': 'http',
                'hl7': 'tcp'
            };
            return mappings[type] || 'http';
        }
    }

    /**
     * Safely parse JSON fields from database
     */
    parseJsonField(jsonString) {
        if (!jsonString) return {};
        
        try {
            return typeof jsonString === 'string' ? JSON.parse(jsonString) : jsonString;
        } catch (error) {
            console.warn('❌ Failed to parse JSON field:', jsonString);
            return {};
        }
    }

    /**
     * Validate interface configuration
     * ✅ UPDATED: Include connectivity validation
     */
    validateInterfaceConfig(interfaceData) {
        const errors = [];
        
        // Basic validation
        if (!interfaceData.name) errors.push('Interface name is required');
        if (!interfaceData.sourceType) errors.push('Source type is required');
        if (!interfaceData.targetType) errors.push('Target type is required');
        
        // Connectivity validation
        if (interfaceData.sourceConfig) {
            const sourceErrors = this.validateConnectivityConfig(
                interfaceData.sourceConnectivity || this.getDefaultConnectivity('source', interfaceData.sourceType),
                interfaceData.sourceConfig,
                'source'
            );
            errors.push(...sourceErrors);
        }
        
        if (interfaceData.targetConfig) {
            const targetErrors = this.validateConnectivityConfig(
                interfaceData.targetConnectivity || this.getDefaultConnectivity('target', interfaceData.targetType),
                interfaceData.targetConfig,
                'target'
            );
            errors.push(...targetErrors);
        }
        
        return {
            isValid: errors.length === 0,
            errors: errors.length > 0 ? errors.join('; ') : null
        };
    }

    /**
     * Validate specific connectivity configuration
     */
    validateConnectivityConfig(connectivityType, config, direction) {
        const errors = [];
        
        switch (connectivityType) {
            case 'tcp':
                if (!config.host) errors.push(`${direction} TCP host not configured`);
                if (!config.port || isNaN(parseInt(config.port))) {
                    errors.push(`${direction} TCP port not configured or invalid`);
                }
                break;
                
            case 'http':
                if (direction === 'target' && !config.targeturl && !config.endpoint) {
                    errors.push(`${direction} HTTP endpoint not configured`);
                }
                break;
                
            case 'file':
                if (direction === 'source' && !config.directory && !config.inputdirectory) {
                    errors.push(`${direction} file directory not configured`);
                }
                if (direction === 'target' && !config.outputdirectory && !config.directory) {
                    errors.push(`${direction} file output directory not configured`);
                }
                break;
                
            case 'database':
                if (!config.connectionstring && !config.host) {
                    errors.push(`${direction} database connection not configured`);
                }
                break;
                
            case 'sftp':
                if (!config.sftphost && !config.host) {
                    errors.push(`${direction} SFTP host not configured`);
                }
                if (!config.username) {
                    errors.push(`${direction} SFTP username not configured`);
                }
                break;
        }
        
        return errors;
    }
}

// Debug: Test module export
console.log('🔍 About to create enhanced InterfacesController instance...');
const controllerInstance = new InterfacesController();
console.log('🔍 Enhanced Controller instance created:', !!controllerInstance);
console.log('🔍 Enhanced Controller instance.database:', !!controllerInstance.database);

module.exports = controllerInstance;