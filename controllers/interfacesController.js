// controllers/interfacesController.js - COMPLETE DEBUG VERSION (All Methods)
console.log('🔧 Loading Complete Debug Interfaces Controller...');

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
            console.log('✅ Complete Debug Interfaces Controller initialized');
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
     */
    async getAllInterfaces(req, res) {
        console.log('\n=== GET ALL INTERFACES (COMPLETE DEBUG) ===');
        console.log('📍 Session user:', req.session?.user?.email);
        console.log('🔍 this.database exists:', !!this.database);
        console.log('🔍 this.database.sequelize exists:', !!this.database?.sequelize);
        
        try {
            await this.ensureDatabase();
            
            const userId = req.session.user.id;
            console.log('🔍 User ID:', userId);
            
            console.log('🔍 About to execute query...');
            const interfaces = await this.database.sequelize.query(`
                SELECT 
                    i.id,
                    i.user_id,
                    i.name,
                    i.description,
                    i.source_type,
                    i.target_type,
                    i.message_type,
                    i.status,
                    i.total_processed,
                    i.successful_processed,
                    i.failed_processed,
                    i.last_processed_at,
                    i.created_at,
                    i.updated_at,
                    i.version,
                    COALESCE(u.first_name || ' ' || u.last_name, 'Unknown User') as created_by_name,
                    u.email as created_by_email
                FROM interfaces i
                LEFT JOIN users u ON i.created_by = u.id
                WHERE i.user_id = :userId AND i.is_active = true
                ORDER BY i.updated_at DESC
            `, {
                replacements: { userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });
            
            console.log(`✅ Found ${interfaces.length} interfaces`);
            
            // Transform for frontend compatibility
            const transformedInterfaces = interfaces.map(item => ({
                id: item.id,
                userId: item.user_id,
                name: item.name,
                description: item.description,
                sourceType: item.source_type,
                sourceConfig: item.source_config || {},
                targetType: item.target_type,
                targetConfig: item.target_config || {},
                messageType: item.message_type,
                processingRules: item.processing_rules || {},
                transformationMapping: item.transformation_mapping || {},
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
     */
    async createInterface(req, res) {
        console.log('\n=== CREATE INTERFACE (COMPLETE DEBUG) ===');
        console.log('📍 Session user:', req.session?.user?.email);
        console.log('📥 Request body:', JSON.stringify(req.body, null, 2));
        
        // Debug: Check all properties
        console.log('🔍 DEBUG: this exists:', !!this);
        console.log('🔍 DEBUG: this.database exists:', !!this.database);
        console.log('🔍 DEBUG: typeof this.database:', typeof this.database);
        
        if (this.database) {
            console.log('🔍 DEBUG: this.database.sequelize exists:', !!this.database.sequelize);
            console.log('🔍 DEBUG: this.database.isConnected:', this.database.isConnected);
        } else {
            console.log('❌ DEBUG: this.database is null/undefined');
        }
        
        try {
            await this.ensureDatabase();
            
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;
            
            const { 
                name, 
                description, 
                sourceType, 
                sourceConfig, 
                targetType, 
                targetConfig,
                messageType,
                processingRules,
                transformationMapping
            } = req.body;

            console.log('🔍 Extracted data:');
            console.log('   userId:', userId);
            console.log('   userEmail:', userEmail);
            console.log('   name:', name);
            console.log('   sourceType:', sourceType);
            console.log('   targetType:', targetType);

            // Validate required fields
            if (!name || !sourceType || !targetType) {
                console.log('❌ Validation failed: Missing required fields');
                return res.status(400).json({
                    success: false,
                    error: 'Name, source type, and target type are required'
                });
            }

            console.log('🔍 About to check for duplicates...');
            // Check for duplicate interface name
            const duplicateInterfaces = await this.database.sequelize.query(`
                SELECT id FROM interfaces 
                WHERE user_id = :userId AND name = :name AND is_active = true
            `, {
                replacements: { userId, name },
                type: this.database.sequelize.QueryTypes.SELECT
            });
            
            if (duplicateInterfaces.length > 0) {
                console.log('❌ Duplicate interface name:', name);
                return res.status(400).json({
                    success: false,
                    error: 'Interface with this name already exists'
                });
            }

            console.log('🔍 About to insert new interface...');
            // Insert new interface
            const newInterfaces = await this.database.sequelize.query(`
                INSERT INTO interfaces (
                    user_id, name, description, source_type, target_type, message_type,
                    source_config, target_config, processing_rules, transformation_mapping,
                    created_by, updated_by
                ) VALUES (
                    :userId, :name, :description, :sourceType, :targetType, :messageType,
                    :sourceConfig, :targetConfig, :processingRules, :transformationMapping,
                    :userId, :userId
                )
                RETURNING *
            `, {
                replacements: {
                    userId,
                    name,
                    description: description || '',
                    sourceType,
                    targetType,
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

            // Transform for frontend
            const responseInterface = {
                id: newInterfaceItem.id,
                userId: newInterfaceItem.user_id,
                name: newInterfaceItem.name,
                description: newInterfaceItem.description,
                sourceType: newInterfaceItem.source_type,
                sourceConfig: newInterfaceItem.source_config,
                targetType: newInterfaceItem.target_type,
                targetConfig: newInterfaceItem.target_config,
                messageType: newInterfaceItem.message_type,
                processingRules: newInterfaceItem.processing_rules,
                transformationMapping: newInterfaceItem.transformation_mapping,
                status: newInterfaceItem.status,
                statistics: {
                    totalProcessed: 0,
                    successful: 0,
                    failed: 0,
                    lastProcessed: null
                },
                createdAt: newInterfaceItem.created_at,
                updatedAt: newInterfaceItem.updated_at,
                createdBy: userEmail,
                version: newInterfaceItem.version
            };

            return res.status(201).json({
                success: true,
                interface: responseInterface,
                message: 'Interface created successfully'
            });

        } catch (error) {
            console.error('❌ Create Interface Error:', error);
            console.error('❌ Error type:', error.constructor.name);
            console.error('❌ Error message:', error.message);
            console.error('❌ Error stack:', error.stack);
            
            return res.status(500).json({
                success: false,
                error: 'Failed to create interface',
                debug: error.message,
                errorType: error.constructor.name
            });
        }
    }

    /**
     * Start an interface
     */
    async startInterface(req, res) {
        console.log('\n=== START INTERFACE (COMPLETE DEBUG) ===');
        
        try {
            await this.ensureDatabase();
            
            const { interfaceId } = req.params;
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;

            console.log('🔍 Starting interface:', interfaceId, 'for user:', userEmail);

            // Get and update interface
            const interfaces = await this.database.sequelize.query(`
                SELECT * FROM interfaces 
                WHERE id = :interfaceId AND user_id = :userId AND is_active = true
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });
            
            if (interfaces.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found or access denied'
                });
            }

            const interfaceItem = interfaces[0];

            // Update status
            await this.database.sequelize.query(`
                UPDATE interfaces 
                SET status = 'running', updated_by = :userId, updated_at = NOW()
                WHERE id = :interfaceId
            `, {
                replacements: { userId, interfaceId },
                type: this.database.sequelize.QueryTypes.UPDATE
            });

            console.log(`✅ Interface Started - User: ${userEmail}, Interface: ${interfaceItem.name}`);

            return res.json({
                success: true,
                interface: { ...interfaceItem, status: 'running' },
                message: `Interface "${interfaceItem.name}" started successfully`
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
        console.log('\n=== STOP INTERFACE (COMPLETE DEBUG) ===');
        
        try {
            await this.ensureDatabase();
            
            const { interfaceId } = req.params;
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;

            console.log('🔍 Stopping interface:', interfaceId, 'for user:', userEmail);

            const interfaces = await this.database.sequelize.query(`
                SELECT * FROM interfaces 
                WHERE id = :interfaceId AND user_id = :userId AND is_active = true
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });
            
            if (interfaces.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found or access denied'
                });
            }

            const interfaceItem = interfaces[0];

            await this.database.sequelize.query(`
                UPDATE interfaces 
                SET status = 'stopped', updated_by = :userId, updated_at = NOW()
                WHERE id = :interfaceId
            `, {
                replacements: { userId, interfaceId },
                type: this.database.sequelize.QueryTypes.UPDATE
            });

            console.log(`✅ Interface Stopped - User: ${userEmail}, Interface: ${interfaceItem.name}`);

            return res.json({
                success: true,
                interface: { ...interfaceItem, status: 'stopped' },
                message: `Interface "${interfaceItem.name}" stopped successfully`
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
        console.log('\n=== PAUSE INTERFACE (COMPLETE DEBUG) ===');
        
        try {
            await this.ensureDatabase();
            
            const { interfaceId } = req.params;
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;

            console.log('🔍 Pausing interface:', interfaceId, 'for user:', userEmail);

            const interfaces = await this.database.sequelize.query(`
                SELECT * FROM interfaces 
                WHERE id = :interfaceId AND user_id = :userId AND is_active = true
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });
            
            if (interfaces.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found or access denied'
                });
            }

            const interfaceItem = interfaces[0];

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
                interface: { ...interfaceItem, status: 'paused' },
                message: `Interface "${interfaceItem.name}" paused successfully`
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
        console.log('\n=== DELETE INTERFACE (COMPLETE DEBUG) ===');
        
        try {
            await this.ensureDatabase();
            
            const { interfaceId } = req.params;
            const userId = req.session.user.id;
            const userEmail = req.session.user.email;

            console.log('🔍 Deleting interface:', interfaceId, 'for user:', userEmail);

            const interfaces = await this.database.sequelize.query(`
                SELECT * FROM interfaces 
                WHERE id = :interfaceId AND user_id = :userId AND is_active = true
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });
            
            if (interfaces.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found or access denied'
                });
            }

            const interfaceItem = interfaces[0];

            // Don't allow deletion of running interfaces
            if (interfaceItem.status === 'running') {
                return res.status(400).json({
                    success: false,
                    error: 'Cannot delete a running interface. Please stop it first.'
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

    /**
     * Get interface by ID
     */
    async getInterface(req, res) {
        console.log('\n=== GET INTERFACE (COMPLETE DEBUG) ===');
        
        try {
            await this.ensureDatabase();
            
            const { interfaceId } = req.params;
            const userId = req.session.user.id;

            console.log('🔍 Getting interface:', interfaceId, 'for user:', userId);

            const interfaces = await this.database.sequelize.query(`
                SELECT 
                    i.*,
                    COALESCE(u.first_name || ' ' || u.last_name, 'Unknown User') as created_by_name,
                    u.email as created_by_email
                FROM interfaces i
                LEFT JOIN users u ON i.created_by = u.id
                WHERE i.id = :interfaceId AND i.user_id = :userId AND i.is_active = true
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });
            
            if (interfaces.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found or access denied'
                });
            }

            const interfaceItem = interfaces[0];

            // Transform for frontend
            const responseInterface = {
                id: interfaceItem.id,
                userId: interfaceItem.user_id,
                name: interfaceItem.name,
                description: interfaceItem.description,
                sourceType: interfaceItem.source_type,
                sourceConfig: interfaceItem.source_config,
                targetType: interfaceItem.target_type,
                targetConfig: interfaceItem.target_config,
                messageType: interfaceItem.message_type,
                processingRules: interfaceItem.processing_rules,
                transformationMapping: interfaceItem.transformation_mapping,
                status: interfaceItem.status,
                statistics: {
                    totalProcessed: interfaceItem.total_processed,
                    successful: interfaceItem.successful_processed,
                    failed: interfaceItem.failed_processed,
                    lastProcessed: interfaceItem.last_processed_at
                },
                createdAt: interfaceItem.created_at,
                updatedAt: interfaceItem.updated_at,
                createdBy: interfaceItem.created_by_email,
                version: interfaceItem.version
            };

            console.log('✅ Interface retrieved successfully');

            return res.json({
                success: true,
                interface: responseInterface
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
}

// Debug: Test module export
console.log('🔍 About to create InterfacesController instance...');
const controllerInstance = new InterfacesController();
console.log('🔍 Controller instance created:', !!controllerInstance);
console.log('🔍 Controller instance.database:', !!controllerInstance.database);

module.exports = controllerInstance;