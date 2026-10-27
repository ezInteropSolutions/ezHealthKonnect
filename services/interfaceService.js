// services/interfaceService.js
// ENHANCED DEBUG VERSION - Add this to identify the root cause
// Interface service compatible with existing PostgreSQL database setup

const database = require('../config/database');
const { v4: uuidv4 } = require('uuid');
// Secret fields in connector configs are encrypted at rest (see services/InterfaceConfigProtector.js).
const configProtector = require('./InterfaceConfigProtector').default;
const interfaceTableManager = require('./InterfaceTableManager');

class InterfaceService {
    constructor() {
        this.database = database;
        // The single owner of the standard messages_intf_* schema.
        this.tableManager = interfaceTableManager;
        console.log('🔧 InterfaceService initialized');
        this.debugDatabaseConnection();
    }

    /**
     * Debug database connection status
     */
    debugDatabaseConnection() {
        console.log('🔍 Debugging database connection...');
        console.log('Database object exists:', !!this.database);
        console.log('Database models exist:', !!this.database?.models);
        console.log('Interface model exists:', !!this.database?.models?.Interface);

        if (this.database?.sequelize) {
            console.log('Sequelize instance exists:', !!this.database.sequelize);
            console.log('Database name:', this.database.sequelize.getDatabaseName());
        } else {
            console.warn('⚠️ No Sequelize instance found');
        }
    }

    /**
     * Ensure database connection and models are initialized
     */
    async ensureDatabaseConnection() {
        try {
            // If not connected, connect now
            if (!this.database.isConnected || !this.database.models.Interface) {
                console.log('🔗 Database not connected, connecting now...');
                await this.database.connect();

                if (!this.database.isConnected) {
                    throw new Error('Failed to establish database connection');
                }

                if (!this.database.models.Interface) {
                    throw new Error('Interface model not available after connection');
                }

                console.log('✅ Database connection established and models loaded');
            } else {
                console.log('✅ Database already connected');
            }
        } catch (error) {
            console.error('❌ Failed to ensure database connection:', error);
            throw error;
        }
    }

    /**
     * Create new interface - ENHANCED DEBUG VERSION
     */
    async createInterface(interfaceData, userId) {
        console.log('\n🚀 === CREATE INTERFACE DEBUG ===');
        console.log('Input data:', {
            name: interfaceData.name,
            sourceType: interfaceData.sourceType,
            targetType: interfaceData.targetType,
            userId: userId
        });

        try {
            // Step 0: Ensure database is connected and models are initialized
            console.log('Step 0: Ensuring database connection...');
            await this.ensureDatabaseConnection();

            // Step 1: Check database availability
            console.log('Step 1: Checking database models...');
            if (!this.database) {
                console.error('❌ Database object is null/undefined');
                throw new Error('Database connection not initialized');
            }

            if (!this.database.models) {
                console.error('❌ Database models object is null/undefined');
                throw new Error('Database models not initialized');
            }

            if (!this.database.models.Interface) {
                console.error('❌ Interface model is null/undefined');
                console.log('Available models:', Object.keys(this.database.models || {}));
                throw new Error('Interface model not found in database models');
            }

            console.log('✅ Database models available');

            // Step 2: Prepare interface data
            console.log('Step 2: Preparing interface data...');
            const interfaceId = uuidv4();
            const now = new Date();

            // ⚠️ CRITICAL FIX: V30 migration changed source_connectivity and target_connectivity to JSONB
            // They now expect structure: { type: "tcp", config: {...} }
            console.log('🔍 RAW interfaceData:', {
                sourceConnectivity: interfaceData.sourceConnectivity,
                targetConnectivity: interfaceData.targetConnectivity,
                sourceConfig: typeof interfaceData.sourceConfig,
                targetConfig: typeof interfaceData.targetConfig
            });

            // Clean target config based on connectivity type
            // Sink targets should not have HTTP-specific fields
            let cleanedTargetConfig = interfaceData.targetConfig || {};
            const targetConnType = interfaceData.targetConnectivity || 'http';

            if (targetConnType === 'sink') {
                // For sink, only keep sink-specific fields
                cleanedTargetConfig = {
                    enableLogging: cleanedTargetConfig.enableLogging !== false,
                    enableValidation: cleanedTargetConfig.enableValidation || false,
                    retentionDays: cleanedTargetConfig.retentionDays || 30,
                    generateAck: cleanedTargetConfig.generateAck !== false
                };
            }

            const createData = {
                id: interfaceId,
                name: interfaceData.name,
                description: interfaceData.description || '',
                message_type: interfaceData.messageType || null,
                source_type: interfaceData.sourceType,
                target_type: interfaceData.targetType,
                // V30+ migration: ONLY use connectivity fields (no legacy duplication)
                source_connectivity: {
                    type: interfaceData.sourceConnectivity || 'tcp',
                    config: configProtector.protectForStorage(interfaceData.sourceConfig || {}, {})
                },
                target_connectivity: {
                    type: targetConnType,
                    config: configProtector.protectForStorage(cleanedTargetConfig, {})
                },
                // REMOVED: Legacy columns source_config/target_config - use *_connectivity only
                // FIXED: Handle both wizard formats - transformationMapping (object with atomicMappings) or mappings (array)
                transformation_mapping: interfaceData.transformationMapping || interfaceData.mappings || {},
                // CDA-specific interface-level defaults (e.g. Plan-of-Care
                // encounter target resource -- Appointment vs Encounter, see
                // services/cda_fhir/generic_mapper.go's CDAToFHIRConfig).
                // Read by the Go backend via processing_rules->'cda'->>'...'
                // (declarative_document_mapper.go), never by Node.js itself.
                processing_rules: interfaceData.processingRules || {},
                status: interfaceData.status || 'draft',
                // V32: Deployment configuration fields
                deployment_mode: interfaceData.deployment_mode || 'manual',
                auto_start: interfaceData.auto_start !== undefined ? interfaceData.auto_start : false,
                deployment_delay_seconds: interfaceData.deployment_delay_seconds || 0,
                user_id: userId,
                created_by: userId,
                updated_by: userId,
                is_active: true,
                created_at: now,
                updated_at: now
            };

            console.log('✅ Prepared create data with JSONB objects:', {
                id: createData.id,
                name: createData.name,
                source_type: createData.source_type,
                target_type: createData.target_type,
                source_connectivity: configProtector.maskForDisplay(createData.source_connectivity),
                target_connectivity: configProtector.maskForDisplay(createData.target_connectivity),
                status: createData.status
            });

            // Step 3: Execute database create
            console.log('Step 3: Executing database create...');
            const interfaceRecord = await this.database.models.Interface.create(createData);
            
            console.log('✅ Interface created in database:', {
                id: interfaceRecord.id,
                name: interfaceRecord.name,
                status: interfaceRecord.status
            });

            // Step 4: Initialize interface tables (OOB requirement)
            console.log('Step 4: Initializing interface tables (OOB)...');
            try {
                await this.initializeInterfaceTables(interfaceRecord.id, interfaceRecord.name);
                console.log('✅ Interface tables initialized');
            } catch (tableError) {
                console.warn('⚠️ Failed to initialize tables (will retry on first message):', tableError.message);
                // Don't fail interface creation if table creation fails
                // Tables will be created on first message if needed
            }

            // Step 5: Return result
            const result = {
                interfaceId: interfaceRecord.id,
                interface: interfaceRecord.toJSON()
            };

            console.log('✅ Returning success result');
            return result;

        } catch (error) {
            console.error('\n❌ === CREATE INTERFACE ERROR ===');
            console.error('Error type:', error.constructor.name);
            console.error('Error message:', error.message);
            console.error('Error code:', error.code);
            console.error('Error details:', error.detail);
            console.error('Full error:', error);
            
            // Additional debugging for Sequelize errors
            if (error.name === 'SequelizeError' || error.name === 'SequelizeValidationError') {
                console.error('Sequelize error details:', {
                    name: error.name,
                    message: error.message,
                    errors: error.errors,
                    sql: error.sql
                });
            }

            // Throw with enhanced error message
            throw new Error(`Interface creation failed: ${error.message}`);
        }
    }

    /**
     * Find interface by name - for wizardController compatibility
     */
    async findInterfaceByName(name, userId) {
        return this.checkDuplicateName(name, userId);
    }

    /**
     * Check if interface name already exists for user
     */
    async checkDuplicateName(name, userId) {
        try {
            console.log(`🔍 Checking duplicate name "${name}" for user ${userId}`);

            // Ensure database connection
            await this.ensureDatabaseConnection();

            if (!this.database || !this.database.models || !this.database.models.Interface) {
                console.warn('⚠️ Database models not available, assuming name is unique');
                return null;
            }
            
            const existingInterface = await this.database.models.Interface.findOne({
                where: {
                    name: this.database.sequelize.where(
                        this.database.sequelize.fn('LOWER', this.database.sequelize.fn('TRIM', this.database.sequelize.col('name'))),
                        this.database.sequelize.fn('LOWER', this.database.sequelize.fn('TRIM', name))
                    ),
                    user_id: userId,
                    is_active: true
                },
                attributes: ['id', 'name', 'description', 'status', 'created_at', 'updated_at']
            });
            
            if (existingInterface) {
                const interfaceData = existingInterface.toJSON();
                console.log(`❌ Duplicate name found: "${name}" (ID: ${interfaceData.id})`);
                return interfaceData;
            }
            
            console.log(`✅ Name is unique: "${name}"`);
            return null;
            
        } catch (error) {
            console.error('❌ Error checking duplicate name:', error);
            // For now, assume unique on error to avoid blocking workflow
            return null;
        }
    }

    /**
     * Updates an existing interface from the wizard's re-run path
     * (wizardController.completeWizard with an existing interfaceId). This
     * method was called there but never existed, so re-running the wizard on an
     * existing interface threw (found 2026-10-19). Only whitelisted columns are
     * written; undefined values are left unchanged; connector configs are
     * protected at rest against the currently stored values (masked
     * placeholders resolve to the stored secrets).
     * @returns {Promise<object|null>} the updated row (masked), or null when not found / not owned
     */
    async updateInterface(interfaceId, fields, userId, allInterfaces = false) {
        await this.ensureDatabaseConnection();
        // allInterfaces: the caller's access policy (AccessPolicyService) lets them change interfaces they did not create
        const record = await this.database.models.Interface.findOne({
            where: allInterfaces ? { id: interfaceId, is_active: true } : { id: interfaceId, user_id: userId, is_active: true }
        });
        if (!record) return null;

        const UPDATABLE = ['name', 'description', 'source_type', 'target_type', 'message_type',
            'status', 'auto_start', 'deployment_mode'];
        const updates = {};
        for (const key of UPDATABLE) {
            if (fields[key] !== undefined) updates[key] = fields[key];
        }
        for (const column of ['source_connectivity', 'target_connectivity']) {
            const incoming = fields[column];
            if (!incoming) continue;
            const stored = record.get(column) || {};
            updates[column] = {
                type: incoming.type || stored.type,
                config: configProtector.protectForStorage(incoming.config || {}, stored.config || {}),
            };
        }
        updates.updated_by = userId;
        updates.updated_at = new Date();

        await record.update(updates);
        return this._toSafeJSON(record);
    }

    /**
     * toJSON() of an Interface row with every stored connector secret masked —
     * these objects are returned to the browser.
     */
    _toSafeJSON(record) {
        const json = record.toJSON();
        for (const column of ['source_config', 'target_config', 'source_connectivity', 'target_connectivity']) {
            if (json[column] && typeof json[column] === 'object') {
                json[column] = configProtector.maskForDisplay(json[column]);
            }
        }
        return json;
    }

    async getInterface(interfaceId, userId) {
        try {
            if (!this.database || !this.database.models || !this.database.models.Interface) {
                return null;
            }

            const interfaceRecord = await this.database.models.Interface.findOne({
                where: {
                    id: interfaceId,
                    user_id: userId,
                    is_active: true
                }
            });
            
            if (!interfaceRecord) {
                return null;
            }
            
            const json = this._toSafeJSON(interfaceRecord);
            
            try {
                json.sourceConfig = JSON.parse(json.source_config || '{}');
                json.targetConfig = JSON.parse(json.target_config || '{}');
                json.mappings = JSON.parse(json.transformation_mapping || '{}');
            } catch (error) {
                console.warn('Failed to parse interface JSON fields:', error.message);
                json.sourceConfig = {};
                json.targetConfig = {};
                json.mappings = {};
            }
            
            return json;
        } catch (error) {
            console.error('Error getting interface:', error);
            return null;
        }
    }

    async getUserInterfaces(userId, page = 1, limit = 25, filters = {}) {
        try {
            if (!this.database || !this.database.models || !this.database.models.Interface) {
                return { interfaces: [], pagination: { totalCount: 0 } };
            }

            const offset = (page - 1) * limit;
            const where = {
                user_id: userId,
                is_active: true
            };
            
            if (filters.status) where.status = filters.status;
            if (filters.sourceType) where.source_type = filters.sourceType;
            if (filters.targetType) where.target_type = filters.targetType;
            if (filters.search) {
                where[this.database.sequelize.Op.or] = [
                    { name: { [this.database.sequelize.Op.iLike]: `%${filters.search}%` } },
                    { description: { [this.database.sequelize.Op.iLike]: `%${filters.search}%` } }
                ];
            }
            
            const { count, rows: interfaces } = await this.database.models.Interface.findAndCountAll({
                where,
                order: [['created_at', 'DESC']],
                limit,
                offset
            });
            
            const processedInterfaces = interfaces.map(i => {
                const json = this._toSafeJSON(i);
                
                try {
                    json.sourceConfig = JSON.parse(json.source_config || '{}');
                    json.targetConfig = JSON.parse(json.target_config || '{}');
                    json.mappings = JSON.parse(json.transformation_mapping || '{}');
                } catch (error) {
                    json.sourceConfig = {};
                    json.targetConfig = {};
                    json.mappings = {};
                }
                
                const mappingCount = Object.keys(json.mappings).length;
                return { ...json, mappingCount };
            });
            
            return {
                interfaces: processedInterfaces,
                pagination: {
                    currentPage: page,
                    totalPages: Math.ceil(count / limit),
                    totalCount: count,
                    hasNextPage: page < Math.ceil(count / limit),
                    hasPreviousPage: page > 1
                }
            };
        } catch (error) {
            console.error('Error getting user interfaces:', error);
            return { interfaces: [], pagination: { totalCount: 0 } };
        }
    }

    /**
     * Initialize interface tables (OOB requirement)
     * Creates input and output tables when interface is created
     */
    async initializeInterfaceTables(interfaceId, interfaceName) {
        console.log(`🏗️ Initializing tables for interface: ${interfaceName} (${interfaceId})`);

        try {
            const outputTableName = `output_intf_${interfaceId.replace(/-/g, '_')}`;

            // The message table always uses the standard schema owned by
            // InterfaceTableManager — the same table every other creation path gets.
            const inputTableName = await this.tableManager.ensureInterfaceTable(interfaceId, interfaceName);
            await this.createOutputTable(outputTableName, interfaceId);
            console.log(`✅ PostgreSQL tables created: ${inputTableName}, ${outputTableName}`);

            // Create MongoDB collections (if MongoDB is available)
            try {
                await this.createMongoDBCollections(interfaceId, interfaceName);
            } catch (mongoError) {
                console.warn(`⚠️ MongoDB collections not created (MongoDB may not be available):`, mongoError.message);
                // Don't fail interface creation if MongoDB is not available
            }

            return { inputTableName, outputTableName };

        } catch (error) {
            console.error(`❌ Failed to initialize tables for interface ${interfaceId}:`, error);
            throw error;
        }
    }

    /**
     * Create output message table
     */
    async createOutputTable(tableName, interfaceId) {
        const createTableSQL = `
            CREATE TABLE IF NOT EXISTS ${tableName} (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                message_id VARCHAR(255) NOT NULL,
                interface_id UUID NOT NULL,
                status VARCHAR(50) DEFAULT 'pending',
                transformed_content TEXT,
                delivery_status VARCHAR(50) DEFAULT 'pending',
                delivery_attempts INTEGER DEFAULT 0,
                last_delivery_error TEXT,
                created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                delivered_at TIMESTAMP WITH TIME ZONE
            );
        `;

        const createIndex = `CREATE INDEX IF NOT EXISTS idx_${tableName}_message_id ON ${tableName}(message_id);`;

        const registerMetadata = `
            INSERT INTO output_table_metadata (interface_id, output_table_name, created_at)
            VALUES ($1, $2, CURRENT_TIMESTAMP)
            ON CONFLICT (interface_id) DO UPDATE SET
                output_table_name = $2, updated_at = CURRENT_TIMESTAMP;
        `;

        await this.database.sequelize.query(createTableSQL);
        await this.database.sequelize.query(createIndex);
        await this.database.sequelize.query(registerMetadata, {
            bind: [interfaceId, tableName]
        });

        console.log(`✅ Output table created: ${tableName}`);
    }

    /**
     * Create MongoDB collections for interface
     * Object storage buckets are created on-demand when messages arrive.
     * This method is retained as a no-op for backward compatibility.
     */
    async createMongoDBCollections(interfaceId, interfaceName) {
        console.log(`ℹ️  createMongoDBCollections: no-op (storage migrated to S3/MinIO)`);
    }
}

module.exports = new InterfaceService();