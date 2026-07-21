// controllers/interfacesController.js - UPDATED WITH CONNECTIVITY SUPPORT
console.log('🔧 Loading Enhanced Interfaces Controller with Format + Connectivity Support...');
const { GO_BACKEND_URL: _GO_URL, internalHeaders: _goHeaders, goClient: _goClient } = require('../services/goBackendClient');

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

            // ✅ UPDATED: Include connectivity fields, deployment settings, and logging settings in SELECT
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
                    i.deployment_mode,
                    i.auto_start,
                    i.deployment_delay_seconds,
                    i.log_level,
                    i.debug_logging,
                    i.log_retention_days,
                    i.retain_error_logs_forever,
                    i.fhir_validation_policy,
                    i.accepted_message_families,
                    i.dlq_config,
                    i.total_processed,
                    i.successful_processed,
                    i.failed_processed,
                    i.last_processed_at,
                    i.created_at,
                    i.updated_at,
                    i.version,
                    i.processing_stats,
                    u.email as created_by_email,
                    (SELECT ts.config::text
                     FROM transformation_steps ts
                     JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
                     WHERE tp.interface_id = i.id AND ts.step_type = 'connector.inbound'
                     ORDER BY ts.sequence LIMIT 1) AS inbound_step_config,
                    (SELECT ts.id::text
                     FROM transformation_steps ts
                     JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
                     WHERE tp.interface_id = i.id AND ts.step_type = 'connector.inbound'
                     ORDER BY ts.sequence LIMIT 1) AS inbound_step_id,
                    (SELECT ts.config::text
                     FROM transformation_steps ts
                     JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
                     WHERE tp.interface_id = i.id AND ts.step_type = 'connector.outbound'
                     ORDER BY ts.sequence LIMIT 1) AS outbound_step_config,
                    (SELECT ts.id::text
                     FROM transformation_steps ts
                     JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
                     WHERE tp.interface_id = i.id AND ts.step_type = 'connector.outbound'
                     ORDER BY ts.sequence LIMIT 1) AS outbound_step_id
                FROM interfaces i
                LEFT JOIN users u ON i.created_by = u.id
                WHERE i.user_id = :userId AND i.is_active = true
                ORDER BY i.updated_at DESC
            `, {
                replacements: { userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            console.log(`✅ Found ${interfaces.length} interfaces for user: ${userEmail}`);
            
            // ✅ UPDATED: Transform data with connectivity fields, deployment settings, and logging settings
            const transformedInterfaces = interfaces.map(item => ({
                id: item.id,
                userId: item.user_id,
                name: item.name,
                description: item.description,

                // ✅ UPDATED: Include source and target connectivity
                sourceType: item.source_type,
                sourceConnectivity: item.source_connectivity,
                sourceFormat: this.deriveSourceFormat(item.source_connectivity),
                targetType: item.target_type,
                targetConnectivity: item.target_connectivity,

                sourceConfig: this.parseJsonField(item.source_config),
                targetConfig: this.parseJsonField(item.target_config),
                messageType: item.message_type,
                acceptedMessageFamilies: this.parseJsonField(item.accepted_message_families) || null,
                processingRules: this.parseJsonField(item.processing_rules),
                transformationMapping: this.parseJsonField(item.transformation_mapping),
                status: item.status,

                // Deployment settings (V32)
                deployment_mode: item.deployment_mode || 'manual',
                auto_start: item.auto_start || false,
                deployment_delay_seconds: item.deployment_delay_seconds || 0,

                // Logging settings (V33 + V59)
                log_level: item.log_level || 'debug',
                debug_logging: item.debug_logging || false,
                log_retention_days: item.log_retention_days || 30,
                retain_error_logs_forever: item.retain_error_logs_forever !== false,

                // FHIR Validation Policy (V66)
                fhir_validation_policy: item.fhir_validation_policy || 'proceed',
                fhirValidationPolicy: item.fhir_validation_policy || 'proceed',

                // Recovery Queue (DLQ) config (V141)
                dlq_config: this.parseJsonField(item.dlq_config) || {
                    max_attempts: 10, initial_delay_s: 30,
                    retry_delay_s: 60, backoff_multiplier: 2.0, expires_after_hours: 0
                },

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
                version: item.version || 1,
                processingStats: this.parseJsonField(item.processing_stats),

                // Connector step configs — mirror of getInterface so edit modal shows pipeline-accurate data
                inboundStepId: item.inbound_step_id || null,
                inboundStepConfig: this.parseJsonField(item.inbound_step_config),
                outboundStepId: item.outbound_step_id || null,
                outboundStepConfig: this.parseJsonField(item.outbound_step_config)
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
                sourceConnectivity,
                targetType,
                targetConnectivity,
                messageType,
                acceptedMessageFamilies, // array e.g. ["ADT","ORU"] or null = accept all
                sourceConfig,
                targetConfig,
                processingRules,
                transformationMapping,
                deploymentMode,
                autoStart,
                deploymentDelay
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

            // ✅ NEW: Apply deployment configuration defaults (changed to auto-deploy by default)
            const finalDeploymentMode = deploymentMode || 'auto';
            const finalAutoStart = autoStart !== false; // true by default
            const finalDeploymentDelay = deploymentDelay || 0;

            console.log(`   Deployment Mode: ${finalDeploymentMode}`);
            console.log(`   Auto-Start: ${finalAutoStart}`);
            if (finalDeploymentMode === 'delayed') {
                console.log(`   Deployment Delay: ${finalDeploymentDelay}s`);
            }

            // Validation — sourceType/targetType optional when creating from template (configured later)
            if (!name) {
                return res.status(400).json({
                    success: false,
                    error: 'Missing required field: name'
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

            // ✅ UPDATED: Insert with connectivity and deployment fields
            // OOB: Use centralized JSONB preparation
            const replacements = this.prepareJsonbReplacements({
                userId,
                name,
                description: description || '',
                sourceType,
                sourceConnectivity: finalSourceConnectivity,
                targetType,
                targetConnectivity: finalTargetConnectivity,
                messageType: messageType || 'auto-detect',
                sourceConfig,
                targetConfig,
                processingRules,
                transformationMapping,
                deploymentMode: finalDeploymentMode,
                autoStart: finalAutoStart,
                deploymentDelay: finalDeploymentDelay
            });
            replacements.acceptedMessageFamilies = Array.isArray(acceptedMessageFamilies) && acceptedMessageFamilies.length > 0
                ? JSON.stringify(acceptedMessageFamilies)
                : null;

            const newInterfaces = await this.database.sequelize.query(`
                INSERT INTO interfaces (
                    user_id, name, description,
                    source_type, source_connectivity,
                    target_type, target_connectivity,
                    message_type, source_config, target_config,
                    processing_rules, transformation_mapping,
                    deployment_mode, auto_start, deployment_delay_seconds,
                    deployment_status,
                    accepted_message_families,
                    status, created_by, updated_by, created_at, updated_at, is_active
                ) VALUES (
                    :userId, :name, :description,
                    :sourceType, :sourceConnectivity,
                    :targetType, :targetConnectivity,
                    :messageType, :sourceConfig::jsonb, :targetConfig::jsonb,
                    :processingRules::jsonb, :transformationMapping::jsonb,
                    :deploymentMode, :autoStart, :deploymentDelay,
                    'not_deployed',
                    :acceptedMessageFamilies::jsonb,
                    'inactive', :userId, :userId, NOW(), NOW(), true
                ) RETURNING *
            `, {
                replacements,
                type: this.database.sequelize.QueryTypes.SELECT
            });
            
            const newInterfaceItem = newInterfaces[0];
            console.log(`✅ Interface Created - User: ${userEmail}, Interface: ${name} (${newInterfaceItem.id})`);
            console.log(`   Source: ${newInterfaceItem.source_type} via ${newInterfaceItem.source_connectivity}`);
            console.log(`   Target: ${newInterfaceItem.target_type} via ${newInterfaceItem.target_connectivity}`);

            // 🏗️ AUTO-CREATE: PostgreSQL table + MongoDB collection for new interface
            try {
                const InterfaceTableManager = require('../services/InterfaceTableManager');
                const tableManager = new InterfaceTableManager();

                // Create PostgreSQL table for this interface
                const tableName = await tableManager.ensureInterfaceTable(
                    newInterfaceItem.id,
                    newInterfaceItem.name
                );
                console.log(`✅ Created PostgreSQL table: ${tableName}`);

                // Object storage buckets are created on-demand when messages are received.
                // No pre-provisioning needed.
            } catch (tableError) {
                console.error(`❌ Failed to create storage for interface ${newInterfaceItem.id}:`, tableError);
                // Don't fail interface creation if table creation fails - table can be created on first message
                console.warn(`⚠️  Storage will be created automatically when first message is received`);
            }

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
                    i.message_type, i.accepted_message_families,
                    i.processing_rules, i.transformation_mapping,
                    i.status, i.interface_status, i.processing_stats,
                    i.total_processed, i.successful_processed, i.failed_processed,
                    i.last_processed_at, i.created_at, i.updated_at, i.version,
                    i.fhir_validation_policy,
                    i.dlq_config,
                    i.log_level, i.debug_logging, i.log_retention_days, i.retain_error_logs_forever,
                    i.deployment_mode, i.auto_start, i.deployment_delay_seconds,
                    u.email as created_by_email,
                    -- Fetch connector step configs directly from transformation_steps (single source of truth)
                    (SELECT ts.config::text
                     FROM transformation_steps ts
                     JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
                     WHERE tp.interface_id = i.id AND ts.step_type = 'connector.inbound'
                     ORDER BY ts.sequence LIMIT 1) AS inbound_step_config,
                    (SELECT ts.id::text
                     FROM transformation_steps ts
                     JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
                     WHERE tp.interface_id = i.id AND ts.step_type = 'connector.inbound'
                     ORDER BY ts.sequence LIMIT 1) AS inbound_step_id,
                    (SELECT ts.config::text
                     FROM transformation_steps ts
                     JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
                     WHERE tp.interface_id = i.id AND ts.step_type = 'connector.outbound'
                     ORDER BY ts.sequence LIMIT 1) AS outbound_step_config,
                    (SELECT ts.id::text
                     FROM transformation_steps ts
                     JOIN transformation_pipelines tp ON tp.id = ts.pipeline_id
                     WHERE tp.interface_id = i.id AND ts.step_type = 'connector.outbound'
                     ORDER BY ts.sequence LIMIT 1) AS outbound_step_id
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
                
                // Connectivity metadata (display/fallback only — Go reads from connectorSteps below)
                sourceType: item.source_type,
                sourceConnectivity: item.source_connectivity,
                sourceFormat: this.deriveSourceFormat(item.source_connectivity),
                targetType: item.target_type,
                targetConnectivity: item.target_connectivity,
                sourceConfig: this.parseJsonField(item.source_config),
                targetConfig: this.parseJsonField(item.target_config),

                // Single source of truth: connector step configs from transformation_steps
                // Edit modal Source/Target tabs read from and write to these, not source_connectivity
                inboundStepId: item.inbound_step_id || null,
                inboundStepConfig: this.parseJsonField(item.inbound_step_config),
                outboundStepId: item.outbound_step_id || null,
                outboundStepConfig: this.parseJsonField(item.outbound_step_config),
                messageType: item.message_type,
                acceptedMessageFamilies: this.parseJsonField(item.accepted_message_families) || null,
                processingRules: this.parseJsonField(item.processing_rules),
                transformationMapping: this.parseJsonField(item.transformation_mapping),
                status: item.status,
                interface_status: item.interface_status,
                processingStats: this.parseJsonField(item.processing_stats),

                // FHIR Validation Policy (V66)
                fhir_validation_policy: item.fhir_validation_policy || 'proceed',
                fhirValidationPolicy: item.fhir_validation_policy || 'proceed',

                // Recovery Queue (DLQ) config (V141)
                dlq_config: this.parseJsonField(item.dlq_config) || {
                    max_attempts: 10, initial_delay_s: 30,
                    retry_delay_s: 60, backoff_multiplier: 2.0, expires_after_hours: 0
                },

                // Deployment settings
                deployment_mode: item.deployment_mode || 'manual',
                auto_start: item.auto_start || false,
                deployment_delay_seconds: item.deployment_delay_seconds || 0,

                // Logging settings
                log_level: item.log_level || 'debug',
                debug_logging: item.debug_logging || false,
                log_retention_days: item.log_retention_days || 30,
                retain_error_logs_forever: item.retain_error_logs_forever !== false,

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
        console.log('\n=== DELETE INTERFACE (ENHANCED WITH TABLE CLEANUP) ===');

        try {
            await this.ensureDatabase();

            const userId = req.session.user.id;
            const userEmail = req.session.user.email;
            const interfaceId = req.params.interfaceId;

            // New: Support request body parameters from delete modal
            let deleteType = req.body?.deleteType || req.query.deleteType || req.query.mode || 'soft'; // soft or hard
            let dataRetention = req.body?.dataRetention || req.query.dataRetention || 'keep_all'; // keep_all, keep_errors, delete_all

            // Legacy support: ?retainData=false maps to hard delete + delete_all
            const legacyRetainData = req.query.retainData;
            if (legacyRetainData !== undefined) {
                console.log('⚠️  Using legacy retainData parameter');
                if (legacyRetainData === 'false') {
                    deleteType = 'hard';
                    dataRetention = 'delete_all';
                }
            }

            console.log(`🔍 Deleting interface ${interfaceId} for user: ${userEmail}`);
            console.log(`   Delete type: ${deleteType}`);
            console.log(`   Data retention: ${dataRetention}`);

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

            // CRITICAL: Deactivate interface in Go backend FIRST to release ports/resources
            // Do this regardless of DB status — Go runtime may be active even if DB shows inactive
            try {
                console.log(`🛑 Deactivating interface ${interfaceId} in Go backend before delete...`);
                const goBackendUrl = `${_GO_URL}/api/processing/interfaces/${interfaceId}/deactivate`;
                const deactivateResponse = await fetch(goBackendUrl, {
                    method: 'POST',
                    headers: _goHeaders(),
                });

                if (deactivateResponse.ok) {
                    console.log(`✅ Interface ${interfaceId} deactivated in Go backend`);
                } else {
                    console.log(`⚠️  Deactivation returned status ${deactivateResponse.status}, proceeding with delete anyway`);
                }
            } catch (deactivateError) {
                console.log(`⚠️  Could not deactivate interface in Go backend: ${deactivateError.message}`);
                console.log(`   Proceeding with delete anyway (interface may already be stopped)`);
            }

            // If DB still shows 'active', update to 'inactive' now that Go runtime is stopped
            if (interfaceItem.status === 'active') {
                console.log(`📝 Updating DB status from 'active' to 'inactive' for interface ${interfaceId}`);
                await this.database.sequelize.query(`
                    UPDATE interfaces SET status = 'inactive', updated_at = NOW()
                    WHERE id = :interfaceId
                `, {
                    replacements: { interfaceId },
                    type: this.database.sequelize.QueryTypes.UPDATE
                });
            }

            // Step 1: Mark interface as deleted (soft or hard delete)
            if (deleteType === 'soft') {
                // Soft delete: Mark as deleted but keep record
                await this.database.sequelize.query(`
                    UPDATE interfaces
                    SET is_active = false,
                        status = 'deleted',
                        updated_by = :userId,
                        updated_at = NOW(),
                        deleted_at = NOW()
                    WHERE id = :interfaceId
                `, {
                    replacements: { userId, interfaceId },
                    type: this.database.sequelize.QueryTypes.UPDATE
                });
                console.log(`✅ Interface soft-deleted (can be restored) - User: ${userEmail}, Interface: ${interfaceItem.name}`);
            } else {
                // Hard delete: cascade through all FK-dependent tables first, then delete interface.
                // Order matters — delete children before parents.
                const cascadeTables = [
                    // Deepest dependents first
                    'transformation_step_executions', // depends on transformation_executions
                    'transformation_executions',
                    'transformation_steps',           // depends on transformation_pipelines
                    'transformation_pipelines',
                    // Direct interface_id references (order among these doesn't matter)
                    'alert_silences',
                    'alert_rules',
                    'connectivity_execution_log',
                    'cross_db_integrity_log',
                    'cross_db_integrity_stats',
                    'delivery_audit_log',
                    'hl7_fhir_mappings',
                    'interface_connectivity',
                    'interface_message_mappings',
                    'interface_processing_metrics',
                    'interface_table_metadata',
                    'interface_table_performance_log',
                    'message_audit_log',
                    'message_processing_enhanced',
                    'message_processing_queue',
                    'mirth_migration_history',
                    'output_table_metadata',
                    'processing_jobs',
                    'processing_queue_status',
                    'sample_parsed_messages',
                ];
                for (const tbl of cascadeTables) {
                    try {
                        await this.database.sequelize.query(
                            `DELETE FROM ${tbl} WHERE interface_id = :interfaceId`,
                            { replacements: { interfaceId }, type: this.database.sequelize.QueryTypes.DELETE }
                        );
                    } catch (e) {
                        // Table may not exist in all environments — skip silently
                        console.log(`⚠️  Skipping cascade delete for ${tbl}: ${e.message}`);
                    }
                }
                await this.database.sequelize.query(`
                    DELETE FROM interfaces WHERE id = :interfaceId
                `, {
                    replacements: { interfaceId },
                    type: this.database.sequelize.QueryTypes.DELETE
                });
                console.log(`🗑️  Interface permanently deleted - User: ${userEmail}, Interface: ${interfaceItem.name}`);
            }

            // Step 2: Handle message data based on retention policy
            try {
                const InterfaceTableManager = require('../services/InterfaceTableManager');
                const tableManager = new InterfaceTableManager();
                const tableName = tableManager.getInterfaceTableName(interfaceId);

                if (dataRetention === 'delete_all') {
                    // Delete all messages - drop table completely
                    await this.database.sequelize.query(`DROP TABLE IF EXISTS ${tableName} CASCADE`);
                    console.log(`🗑️  Dropped PostgreSQL table: ${tableName}`);

                    // Remove from metadata
                    await this.database.sequelize.query(`
                        DELETE FROM interface_table_metadata WHERE interface_id = :interfaceId
                    `, {
                        replacements: { interfaceId },
                        type: this.database.sequelize.QueryTypes.DELETE
                    });
                    console.log(`🗑️  All message data permanently deleted`);

                } else if (dataRetention === 'keep_errors') {
                    // Delete successful messages, keep errors for analysis
                    await this.database.sequelize.query(`
                        DELETE FROM ${tableName}
                        WHERE status NOT IN ('error', 'failed', 'validation_error')
                    `);
                    console.log(`🗑️  Deleted successful messages, retained error records`);

                } else {
                    // keep_all - do nothing, preserve all data
                    console.log(`ℹ️  All message data preserved for audit/recovery`);
                }

            } catch (tableError) {
                console.error(`⚠️  Failed to handle message data (non-critical):`, tableError.message);
                // Continue - message data cleanup is optional
            }

            // Build response message
            let dataInfo = '';
            if (dataRetention === 'delete_all') {
                dataInfo = 'All message data permanently deleted.';
            } else if (dataRetention === 'keep_errors') {
                dataInfo = 'Successful messages deleted, error records retained for analysis.';
            } else {
                dataInfo = 'All message data retained for audit/recovery.';
            }

            return res.json({
                success: true,
                message: `Interface "${interfaceItem.name}" deleted successfully`,
                deleteType: deleteType,
                dataRetention: dataRetention,
                info: deleteType === 'soft'
                    ? `Interface soft-deleted (can be restored). ${dataInfo}`
                    : `Interface permanently deleted. ${dataInfo}`
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
     * Update an existing interface
     */
    async updateInterface(req, res) {
        console.log('\n=== UPDATE INTERFACE ===');

        try {
            await this.ensureDatabase();

            const userId = req.session.user.id;
            const userEmail = req.session.user.email;
            const interfaceId = req.params.interfaceId;

            const {
                name,
                description,
                sourceType,
                sourceConnectivity,
                targetType,
                targetConnectivity,
                messageType,
                acceptedMessageFamilies,
                sourceConfig,
                targetConfig,
                processingRules,
                transformationMapping,
                // Recovery Queue (DLQ) per-interface config
                dlq_config,
                // Connector step IDs — write directly to transformation_steps (single source of truth)
                inboundStepId,
                outboundStepId,
                // Full connector configs from ConnectorConfigBuilder — replaces entire step config
                sourceConnectorConfig,
                targetConnectorConfig,
                // Deployment settings (V32)
                deployment_mode,
                auto_start,
                deployment_delay_seconds,
                status,
                // Logging settings (V33 + V59)
                debug_logging,
                log_level,
                log_retention_days,
                retain_error_logs_forever,
                // FHIR Validation Policy (V66)
                fhirValidationPolicy,
                fhir_validation_policy
            } = req.body;

            console.log(`🔍 Updating interface ${interfaceId} for user: ${userEmail}`);
            console.log(`   New name: ${name}`);
            console.log(`   Source Type: ${sourceType} | Target Type: ${targetType}`);
            console.log(`   Source Config (type: ${typeof sourceConfig}):`, sourceConfig);
            console.log(`   Target Config (type: ${typeof targetConfig}):`, targetConfig);
            console.log(`   Processing Rules (type: ${typeof processingRules}):`, processingRules);
            console.log(`   Transformation Mapping (type: ${typeof transformationMapping}):`, transformationMapping);
            console.log(`   Deployment: mode=${deployment_mode}, auto_start=${auto_start}, delay=${deployment_delay_seconds}s`);
            console.log(`   Status: ${status}`);
            console.log(`   Logging: debug=${debug_logging}, retention=${log_retention_days} days, retain_errors=${retain_error_logs_forever}`);

            // Verify interface exists and belongs to user
            const existingInterface = await this.database.sequelize.query(`
                SELECT id, name, status FROM interfaces
                WHERE id = :interfaceId AND user_id = :userId AND is_active = true
            `, {
                replacements: { interfaceId, userId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (existingInterface.length === 0) {
                return res.status(404).json({
                    success: false,
                    error: 'Interface not found or access denied'
                });
            }

            // Apply connectivity defaults
            const finalSourceConnectivity = sourceConnectivity || this.getDefaultConnectivity('source', sourceType);
            const finalTargetConnectivity = targetConnectivity || this.getDefaultConnectivity('target', targetType);

            // OOB: Build V30 JSONB structures for connectivity columns
            // V30 migration changed source_connectivity and target_connectivity to JSONB: {type, config}
            const sourceConnectivityJsonb = {
                type: finalSourceConnectivity,
                config: sourceConfig || {}
            };

            const targetConnectivityJsonb = {
                type: finalTargetConnectivity,
                config: targetConfig || {}
            };

            console.log('🔧 V30 JSONB structures:', {
                sourceConnectivityJsonb,
                targetConnectivityJsonb
            });

            // OOB: Use centralized JSONB preparation (now includes connectivity JSONB objects)
            const replacements = this.prepareJsonbReplacements({
                interfaceId,
                name,
                description,
                sourceType,
                sourceConnectivity: sourceConnectivityJsonb,  // V30 JSONB: {type, config}
                targetType,
                targetConnectivity: targetConnectivityJsonb,  // V30 JSONB: {type, config}
                messageType,
                sourceConfig,
                targetConfig,
                processingRules,
                transformationMapping,
                userId
            });

            // Add deployment settings to replacements (V32)
            replacements.deployment_mode = deployment_mode || 'manual';
            replacements.auto_start = auto_start === true || auto_start === 'true';
            replacements.deployment_delay_seconds = parseInt(deployment_delay_seconds) || 0;
            replacements.status = status || 'inactive';

            // Add logging settings to replacements (V33 + V59)
            replacements.log_level = ['debug','info','warning','error','off'].includes(log_level) ? log_level : 'debug';
            replacements.debug_logging = replacements.log_level !== 'off';
            replacements.log_retention_days = parseInt(log_retention_days) || 30;
            replacements.retain_error_logs_forever = retain_error_logs_forever !== false && retain_error_logs_forever !== 'false';

            // Recovery Queue (DLQ) config — merge with defaults so partial updates are safe
            const defaultDLQConfig = {
                max_attempts: 10, initial_delay_s: 30,
                retry_delay_s: 60, backoff_multiplier: 2.0, expires_after_hours: 0
            };
            const incomingDLQ = (typeof dlq_config === 'object' && dlq_config !== null)
                ? dlq_config
                : (typeof dlq_config === 'string' ? JSON.parse(dlq_config) : {});
            replacements.dlq_config = JSON.stringify({ ...defaultDLQConfig, ...incomingDLQ });

            // FHIR Validation Policy (V66) — accept both camelCase and snake_case
            const validPolicies = ['proceed', 'warn', 'reject', 'queue_review'];
            // snake_case takes precedence over camelCase alias (explicit API field wins)
            const rawPolicy = fhir_validation_policy || fhirValidationPolicy || 'proceed';
            replacements.fhir_validation_policy = validPolicies.includes(rawPolicy) ? rawPolicy : 'proceed';

            console.log('📊 OOB Update - JSONB fields prepared:', {
                sourceConnectivity: replacements.sourceConnectivity?.substring(0, 150),
                targetConnectivity: replacements.targetConnectivity?.substring(0, 150)
            });

            // V111: accepted_message_families — null = accept all
            replacements.acceptedMessageFamilies = Array.isArray(acceptedMessageFamilies) && acceptedMessageFamilies.length > 0
                ? JSON.stringify(acceptedMessageFamilies)
                : null;

            await this.database.sequelize.query(`
                UPDATE interfaces SET
                    name = :name,
                    description = :description,
                    status = :status,
                    source_type = :sourceType,
                    source_connectivity = :sourceConnectivity::jsonb,
                    target_type = :targetType,
                    target_connectivity = :targetConnectivity::jsonb,
                    message_type = :messageType,
                    source_config      = COALESCE(NULLIF(:sourceConfig::jsonb,      '{}'::jsonb), source_config),
                    target_config      = COALESCE(NULLIF(:targetConfig::jsonb,      '{}'::jsonb), target_config),
                    processing_rules   = COALESCE(NULLIF(:processingRules::jsonb,   '{}'::jsonb), processing_rules),
                    transformation_mapping = COALESCE(NULLIF(:transformationMapping::jsonb, '{}'::jsonb), transformation_mapping),
                    deployment_mode = :deployment_mode,
                    auto_start = :auto_start,
                    deployment_delay_seconds = :deployment_delay_seconds,
                    log_level = :log_level,
                    debug_logging = :debug_logging,
                    log_retention_days = :log_retention_days,
                    retain_error_logs_forever = :retain_error_logs_forever,
                    fhir_validation_policy = :fhir_validation_policy,
                    accepted_message_families = :acceptedMessageFamilies::jsonb,
                    dlq_config = :dlq_config::jsonb,
                    updated_by = :userId,
                    updated_at = CURRENT_TIMESTAMP
                WHERE id = :interfaceId
            `, {
                replacements,
                type: this.database.sequelize.QueryTypes.UPDATE
            });

            // ── PRIMARY WRITE: update transformation_steps directly ──────────────────
            // transformation_steps is the single source of truth for connector config.
            // When the client sends a full sourceConnectorConfig (from ConnectorConfigBuilder),
            // replace the entire step config so connectorType + config are always in sync.
            // Fall back to a field-merge when only sourceConfig is present (legacy path).
            const _updateStep = async (fullConnectorCfg, innerConfig, stepId, stepType) => {
                const hasFullConfig = fullConnectorCfg && fullConnectorCfg.connectorType;
                const hasInnerConfig = innerConfig && Object.keys(innerConfig).length > 0;
                if (!hasFullConfig && !hasInnerConfig) return;
                try {
                    if (hasFullConfig && stepId) {
                        // Full replacement — connectorType + config in one atomic write
                        await this.database.sequelize.query(`
                            UPDATE transformation_steps
                            SET    config = :fullConfig::jsonb,
                                   updated_at = CURRENT_TIMESTAMP
                            WHERE  id = :stepId
                        `, {
                            replacements: { fullConfig: JSON.stringify(fullConnectorCfg), stepId },
                            type: this.database.sequelize.QueryTypes.UPDATE
                        });
                        console.log(`✅ Full config replacement for ${stepType} step ${stepId}: connectorType=${fullConnectorCfg.connectorType}`);
                    } else if (hasFullConfig) {
                        // Full config but no stepId — find by interface + step_type
                        await this.database.sequelize.query(`
                            UPDATE transformation_steps ts
                            SET    config = :fullConfig::jsonb,
                                   updated_at = CURRENT_TIMESTAMP
                            FROM   transformation_pipelines tp
                            WHERE  tp.id = ts.pipeline_id
                              AND  tp.interface_id = :interfaceId
                              AND  ts.step_type = :stepType
                        `, {
                            replacements: { fullConfig: JSON.stringify(fullConnectorCfg), interfaceId, stepType },
                            type: this.database.sequelize.QueryTypes.UPDATE
                        });
                        console.log(`✅ Full config replacement for ${stepType} (by interface): connectorType=${fullConnectorCfg.connectorType}`);
                    } else if (hasInnerConfig && stepId) {
                        // Legacy merge-patch — only inner config fields changed
                        await this.database.sequelize.query(`
                            UPDATE transformation_steps
                            SET    config = jsonb_set(config, '{config}', config->'config' || :patch::jsonb, true),
                                   updated_at = CURRENT_TIMESTAMP
                            WHERE  id = :stepId
                        `, {
                            replacements: { patch: JSON.stringify(innerConfig), stepId },
                            type: this.database.sequelize.QueryTypes.UPDATE
                        });
                        console.log(`✅ Merge-patch for ${stepType} step ${stepId}`);
                    } else if (hasInnerConfig) {
                        // Legacy merge-patch — no step ID
                        await this.database.sequelize.query(`
                            UPDATE transformation_steps ts
                            SET    config = jsonb_set(ts.config, '{config}', ts.config->'config' || :patch::jsonb, true),
                                   updated_at = CURRENT_TIMESTAMP
                            FROM   transformation_pipelines tp
                            WHERE  tp.id = ts.pipeline_id
                              AND  tp.interface_id = :interfaceId
                              AND  ts.step_type = :stepType
                        `, {
                            replacements: { patch: JSON.stringify(innerConfig), interfaceId, stepType },
                            type: this.database.sequelize.QueryTypes.UPDATE
                        });
                        console.log(`✅ Merge-patch for ${stepType} (by interface)`);
                    }
                } catch (err) {
                    console.warn(`⚠️ Failed to update ${stepType} step: ${err.message}`);
                }
            };

            await _updateStep(sourceConnectorConfig, sourceConfig, inboundStepId,  'connector.inbound');
            await _updateStep(targetConnectorConfig, targetConfig, outboundStepId, 'connector.outbound');

            // Tell the Go engine to reload the filter cache for this interface
            // so the new accept list takes effect immediately without restart.
            try {
                await _goClient.post(`/api/interfaces/${interfaceId}/reload-filter`, {}, { timeout: 3000 });
            } catch (_) { /* non-fatal — filter reloads on next engine restart */ }

            console.log(`✅ Interface ${interfaceId} updated successfully`);

            return res.json({
                success: true,
                message: 'Interface updated successfully',
                interfaceId: interfaceId
            });

        } catch (error) {
            console.error('❌ Update Interface Error:', error);
            return res.status(500).json({
                success: false,
                error: 'Failed to update interface',
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
     * Derive the actual message format (hl7v2, fhir, ccda, json, ...) from an
     * interface's connectivity config, instead of trusting the static
     * `message_type` column (which only encodes an HL7v2 trigger event like
     * "ADT^A01" and goes stale if the connector is reconfigured later).
     *
     * Vocabulary mirrors models.MessageFormat (Go): hl7v2, hl7v3, fhir, ccda,
     * xml, json, edi, csv, unknown.
     */
    deriveSourceFormat(sourceConnectivity) {
        const KNOWN_FORMATS = new Set(['hl7v2', 'hl7v3', 'fhir', 'ccda', 'xml', 'json', 'edi', 'csv']);
        const CONNECTOR_TYPE_FORMAT = {
            tcp_mllp_inbound: 'hl7v2',
            tcp_mllp: 'hl7v2',
            tcp: 'hl7v2',
            http_fhir_inbound: 'fhir',
            http_fhir: 'fhir',
            http_rest_inbound: 'json',
            file_listener: 'hl7v2',
            file: 'hl7v2'
        };

        const conn = this.parseJsonField(sourceConnectivity);
        const config = conn?.config || {};

        // Explicit format set on the connector config wins (e.g. file_listener
        // reading .ccda files, http_fhir_inbound's "fhir" type).
        if (typeof config.type === 'string' && KNOWN_FORMATS.has(config.type.toLowerCase())) {
            return config.type.toLowerCase();
        }

        const connectorType = config?.connectorconfig?.connectorType || conn?.type;
        if (connectorType && CONNECTOR_TYPE_FORMAT[connectorType]) {
            return CONNECTOR_TYPE_FORMAT[connectorType];
        }

        return 'unknown';
    }

    /**
     * Safely stringify JSON fields for database
     * Handles both objects and already-stringified JSON to prevent double-encoding
     */
    /**
     * OOB: Centralized JSONB value preparation for PostgreSQL
     * Handles JSON stringification AND type casting for PostgreSQL JSONB columns
     * @param {any} value - Value to prepare (object, string, null, undefined)
     * @returns {string} JSON string suitable for PostgreSQL JSONB column
     */
    safeJsonStringify(value) {
        if (!value) return '{}';

        if (typeof value === 'string') {
            // Already a string - validate it's valid JSON and return as-is
            try {
                JSON.parse(value); // Validate it's valid JSON
                return value; // Return the string as-is
            } catch (error) {
                console.warn('⚠️ Invalid JSON string provided, returning empty object');
                return '{}';
            }
        }

        // It's an object, stringify it
        return JSON.stringify(value);
    }

    /**
     * OOB: Build SQL column assignments with proper JSONB casting
     * Centralized approach to handle PostgreSQL JSONB requirements consistently
     * @param {object} replacements - Object with column names and values
     * @returns {object} Updated replacements with JSONB-ready values
     */
    prepareJsonbReplacements(replacements) {
        // V30 Update: These columns are JSONB in PostgreSQL and need proper handling
        // source_connectivity and target_connectivity are V30 JSONB: {type, config}
        const jsonbColumns = [
            'sourceConfig', 'targetConfig', 'processingRules', 'transformationMapping',
            'sourceConnectivity', 'targetConnectivity'  // V30 JSONB columns
        ];

        const prepared = { ...replacements };

        console.log('🔧 OOB prepareJsonbReplacements - Input types:');
        jsonbColumns.forEach(column => {
            // Always ensure JSONB columns have a value (even if undefined/null)
            if (prepared.hasOwnProperty(column)) {
                console.log(`   ${column}: ${typeof prepared[column]} = ${JSON.stringify(prepared[column] || null).substring(0, 100)}`);
                const originalType = typeof prepared[column];
                prepared[column] = this.safeJsonStringify(prepared[column]);
                console.log(`   ✅ ${column}: ${originalType} → string (length: ${prepared[column].length})`);
            }
        });

        return prepared;
    }

    /**
     * OOB: Get SQL column assignment with JSONB cast
     * Use in SQL queries to ensure proper type casting
     * @param {string} column - Column name
     * @param {string} param - Parameter name
     * @returns {string} SQL fragment with proper casting
     */
    getJsonbColumnAssignment(column, param) {
        const jsonbColumns = ['source_config', 'target_config', 'processing_rules', 'transformation_mapping'];

        if (jsonbColumns.includes(column)) {
            return `${column} = :${param}::jsonb`;
        }

        return `${column} = :${param}`;
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