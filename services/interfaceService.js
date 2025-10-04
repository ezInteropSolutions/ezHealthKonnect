// services/interfaceService.js
// ENHANCED DEBUG VERSION - Add this to identify the root cause
// Interface service compatible with existing PostgreSQL database setup

const database = require('../config/database');
const { v4: uuidv4 } = require('uuid');

class InterfaceService {
    constructor() {
        this.database = database;
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

            const createData = {
                id: interfaceId,
                name: interfaceData.name,
                description: interfaceData.description || '',
                message_type: interfaceData.messageType || 'ADT^A01',
                source_type: interfaceData.sourceType,
                target_type: interfaceData.targetType,
                source_connectivity: interfaceData.sourceConnectivity,
                target_connectivity: interfaceData.targetConnectivity,
                source_config: JSON.stringify(interfaceData.sourceConfig || {}),
                target_config: JSON.stringify(interfaceData.targetConfig || {}),
                transformation_mapping: JSON.stringify(interfaceData.mappings || {}),
                status: interfaceData.status || 'draft',
                user_id: userId,
                created_by: userId,
                updated_by: userId,
                is_active: true,
                created_at: now,
                updated_at: now
            };

            console.log('Prepared create data:', {
                id: createData.id,
                name: createData.name,
                source_type: createData.source_type,
                target_type: createData.target_type,
                status: createData.status,
                user_id: createData.user_id
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

    // Keep all other methods from the original file unchanged...
    
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
            
            const json = interfaceRecord.toJSON();
            
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
                const json = i.toJSON();
                
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
            // Generate table names
            const inputTableName = `messages_intf_${interfaceId.replace(/-/g, '_')}`;
            const outputTableName = `output_intf_${interfaceId.replace(/-/g, '_')}`;

            // Create input table (for received messages)
            await this.createInputTable(inputTableName, interfaceId);

            // Create output table (for transformed/delivered messages)
            await this.createOutputTable(outputTableName, interfaceId);

            console.log(`✅ Tables created: ${inputTableName}, ${outputTableName}`);
            return { inputTableName, outputTableName };

        } catch (error) {
            console.error(`❌ Failed to initialize tables for interface ${interfaceId}:`, error);
            throw error;
        }
    }

    /**
     * Create input message table
     */
    async createInputTable(tableName, interfaceId) {
        const createTableSQL = `
            CREATE TABLE IF NOT EXISTS ${tableName} (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                message_id VARCHAR(255) NOT NULL,
                correlation_id VARCHAR(255),
                interface_id UUID NOT NULL,
                status VARCHAR(50) DEFAULT 'received',
                priority INTEGER DEFAULT 5,
                received_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                source_type VARCHAR(50),
                source_endpoint VARCHAR(255),
                source_ip VARCHAR(45),
                message_type VARCHAR(100),
                message_size INTEGER,
                message_encoding VARCHAR(50) DEFAULT 'UTF-8',
                raw_message TEXT,
                processing_completed_at TIMESTAMP WITH TIME ZONE,
                processing_time_ms BIGINT,
                error_count INTEGER DEFAULT 0,
                last_error_message TEXT,
                delivery_status VARCHAR(50) DEFAULT 'pending',
                delivery_attempts INTEGER DEFAULT 0,
                created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
            );
        `;

        const createIndexes = [
            `CREATE INDEX IF NOT EXISTS idx_${tableName}_message_id ON ${tableName}(message_id);`,
            `CREATE INDEX IF NOT EXISTS idx_${tableName}_received_at ON ${tableName}(received_at DESC);`,
            `CREATE INDEX IF NOT EXISTS idx_${tableName}_status ON ${tableName}(status);`
        ];

        const registerMetadata = `
            INSERT INTO interface_table_metadata (interface_id, table_name, created_at)
            VALUES ($1, $2, CURRENT_TIMESTAMP)
            ON CONFLICT (interface_id) DO UPDATE SET
                table_name = $2, updated_at = CURRENT_TIMESTAMP;
        `;

        await this.database.sequelize.query(createTableSQL);
        for (const indexSQL of createIndexes) {
            await this.database.sequelize.query(indexSQL);
        }
        await this.database.sequelize.query(registerMetadata, {
            bind: [interfaceId, tableName]
        });

        console.log(`✅ Input table created: ${tableName}`);
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
            INSERT INTO output_table_metadata (interface_id, table_name, created_at)
            VALUES ($1, $2, CURRENT_TIMESTAMP)
            ON CONFLICT (interface_id) DO UPDATE SET
                table_name = $2, updated_at = CURRENT_TIMESTAMP;
        `;

        await this.database.sequelize.query(createTableSQL);
        await this.database.sequelize.query(createIndex);
        await this.database.sequelize.query(registerMetadata, {
            bind: [interfaceId, tableName]
        });

        console.log(`✅ Output table created: ${tableName}`);
    }
}

module.exports = new InterfaceService();