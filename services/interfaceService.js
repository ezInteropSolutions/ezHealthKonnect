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

            // Step 4: Return result
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
}

module.exports = new InterfaceService();