// services/interfaceService.js
// Interface service compatible with existing PostgreSQL database setup

const database = require('../config/database');
const { v4: uuidv4 } = require('uuid');

class InterfaceService {
    constructor() {
        this.database = database;
    }

    /**
     * Find interface by name - for wizardController compatibility
     * @param {string} name - Interface name to find
     * @param {string} userId - User ID
     * @returns {Promise<Object|null>} Existing interface or null
     */
    async findInterfaceByName(name, userId) {
        return this.checkDuplicateName(name, userId);
    }

    /**
     * Check if interface name already exists for user
     * @param {string} name - Interface name to check
     * @param {string} userId - User ID
     * @returns {Promise<Object|null>} Existing interface or null
     */
    async checkDuplicateName(name, userId) {
        try {
            if (!this.database || !this.database.models || !this.database.models.Interface) {
                console.warn('Database models not available, assuming name is unique');
                return null;
            }

            console.log(`Checking duplicate name "${name}" for user ${userId}`);
            
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
                console.log(`Duplicate name found: "${name}" (ID: ${interfaceData.id})`);
                return interfaceData;
            }
            
            console.log(`Name is unique: "${name}"`);
            return null;
            
        } catch (error) {
            console.error('Error checking duplicate name:', error);
            // For now, assume unique on error to avoid blocking workflow
            // In production, you might want to throw the error instead
            return null;
        }
    }

    /**
     * Create new interface
     */
    async createInterface(interfaceData, userId) {
        try {
            if (!this.database || !this.database.models || !this.database.models.Interface) {
                throw new Error('Database models not available');
            }

            const interfaceRecord = await this.database.models.Interface.create({
                id: uuidv4(),
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
                created_at: new Date(),
                updated_at: new Date()
            });
            
            return {
                interfaceId: interfaceRecord.id,
                interface: interfaceRecord.toJSON()
            };
        } catch (error) {
            console.error('Interface creation failed:', error);
            throw error;
        }
    }

    /**
     * Get interface by ID
     */
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
            
            // Parse JSON fields
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
    
    /**
     * Update interface status
     */
    async updateInterfaceStatus(interfaceId, status, userId) {
        try {
            if (!this.database || !this.database.models || !this.database.models.Interface) {
                throw new Error('Database models not available');
            }

            const interfaceRecord = await this.database.models.Interface.findOne({
                where: {
                    id: interfaceId,
                    user_id: userId,
                    is_active: true
                }
            });
            
            if (!interfaceRecord) {
                throw new Error('Interface not found or unauthorized');
            }
            
            const oldStatus = interfaceRecord.status;
            
            interfaceRecord.status = status;
            interfaceRecord.updated_by = userId;
            interfaceRecord.updated_at = new Date();
            await interfaceRecord.save();
            
            return {
                interface: interfaceRecord.toJSON(),
                oldStatus,
                newStatus: status
            };
        } catch (error) {
            console.error('Error updating interface status:', error);
            throw error;
        }
    }
    
    /**
     * Get user interfaces
     */
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
            
            // Apply filters
            if (filters.status) {
                where.status = filters.status;
            }
            if (filters.sourceType) {
                where.source_type = filters.sourceType;
            }
            if (filters.targetType) {
                where.target_type = filters.targetType;
            }
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
            
            // Process interfaces
            const processedInterfaces = interfaces.map(i => {
                const json = i.toJSON();
                
                // Parse JSON fields safely
                try {
                    json.sourceConfig = JSON.parse(json.source_config || '{}');
                    json.targetConfig = JSON.parse(json.target_config || '{}');
                    json.mappings = JSON.parse(json.transformation_mapping || '{}');
                } catch (error) {
                    json.sourceConfig = {};
                    json.targetConfig = {};
                    json.mappings = {};
                }
                
                // Add computed fields
                const mappingCount = Object.keys(json.mappings).length;
                
                return {
                    ...json,
                    mappingCount
                };
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
     * Delete interface (soft delete)
     */
    async deleteInterface(interfaceId, userId) {
        try {
            if (!this.database || !this.database.models || !this.database.models.Interface) {
                throw new Error('Database models not available');
            }

            const interfaceRecord = await this.database.models.Interface.findOne({
                where: {
                    id: interfaceId,
                    user_id: userId,
                    is_active: true
                }
            });
            
            if (!interfaceRecord) {
                throw new Error('Interface not found or unauthorized');
            }
            
            interfaceRecord.is_active = false;
            interfaceRecord.status = 'deleted';
            interfaceRecord.updated_by = userId;
            interfaceRecord.updated_at = new Date();
            await interfaceRecord.save();
            
            return true;
        } catch (error) {
            console.error('Error deleting interface:', error);
            throw error;
        }
    }

    /**
     * Get interface statistics
     */
    async getInterfaceStats(interfaceId, userId) {
        try {
            if (!this.database || !this.database.models || !this.database.models.Interface) {
                return null;
            }

            const interfaceRecord = await this.database.models.Interface.findOne({
                where: {
                    id: interfaceId,
                    user_id: userId,
                    is_active: true
                },
                attributes: [
                    'id', 'name', 'status', 'source_type', 'target_type', 
                    'source_connectivity', 'target_connectivity',
                    'total_processed', 'successful_processed', 'failed_processed', 
                    'last_processed_at', 'created_at', 'updated_at'
                ]
            });
            
            if (!interfaceRecord) {
                return null;
            }
            
            const json = interfaceRecord.toJSON();
            
            // Calculate success rate
            const totalProcessed = json.total_processed || 0;
            const successfulProcessed = json.successful_processed || 0;
            const successRate = totalProcessed > 0 ? 
                ((successfulProcessed / totalProcessed) * 100).toFixed(2) + '%' : '0%';
            
            return {
                ...json,
                stats: {
                    totalProcessed,
                    successful: successfulProcessed,
                    failed: json.failed_processed || 0,
                    successRate,
                    lastProcessed: json.last_processed_at
                }
            };
        } catch (error) {
            console.error('Error getting interface stats:', error);
            return null;
        }
    }
}

module.exports = new InterfaceService();