//hl7_admin-- HL7Admin2024!

// Load environment variables FIRST
require('dotenv').config();

// config/database.js - Fixed to match actual PostgreSQL schema
const { Sequelize, DataTypes } = require('sequelize');

// Debug: Log what environment variables are loaded
console.log('🔍 Database.js - Environment variables loaded:');
console.log('   DB_HOST:', process.env.DB_HOST || 'undefined');
console.log('   DB_PORT:', process.env.DB_PORT || 'undefined'); 
console.log('   DB_NAME:', process.env.DB_NAME || 'undefined');
console.log('   DB_USER:', process.env.DB_USER || 'undefined');
console.log('   DB_PASSWORD:', process.env.DB_PASSWORD ? '***SET***' : 'undefined');

class DatabaseManager {
    constructor() {
        this.sequelize = null;
        this.models = {};
        this.isConnected = false;
    }

    // Initialize PostgreSQL connection
    async connect() {
        try {
            // Debug: Show exactly what config will be used
            const config = {
                host: process.env.DB_HOST || 'localhost',
                port: process.env.DB_PORT || 5432,
                database: process.env.DB_NAME || 'ezhealthkonnect',
                username: process.env.DB_USER || 'postgres',
                password: process.env.DB_PASSWORD || '',
                dialect: 'postgres',
                logging: false, // Disable SQL query logging to reduce noise
                pool: {
                    max: 5,
                    min: 0,
                    acquire: 30000,
                    idle: 10000
                }
            };

            console.log('🔗 Database connection config:');
            console.log('   host:', config.host);
            console.log('   port:', config.port);
            console.log('   database:', config.database);
            console.log('   username:', config.username);
            console.log('   password:', config.password ? '***SET***' : 'EMPTY');

            this.sequelize = new Sequelize(config);

            // Test connection
            await this.sequelize.authenticate();
            console.log('✅ PostgreSQL connected successfully');

            // Define models to match ACTUAL database schema
            this.defineModels();
            
            this.isConnected = true;
            return true;

        } catch (error) {
            console.error('❌ PostgreSQL connection failed:', error.message);
            console.log('⚠️  Continuing with JSON storage...');
            return false;
        }
    }

    // Define models to match the ACTUAL database schema
    defineModels() {
        // User Model - matching actual schema
        this.models.User = this.sequelize.define('User', {
            id: {
                type: DataTypes.UUID,
                defaultValue: DataTypes.UUIDV4,
                primaryKey: true
            },
            email: {
                type: DataTypes.STRING(320),
                allowNull: false,
                unique: true,
                validate: { isEmail: true }
            },
            password_hash: {
                type: DataTypes.STRING(255),
                allowNull: false
            },
            first_name: {
                type: DataTypes.STRING(100),
                allowNull: false
            },
            last_name: {
                type: DataTypes.STRING(100),
                allowNull: false
            },
            role: {
                type: DataTypes.ENUM('admin', 'user', 'operator', 'viewer'),
                defaultValue: 'user',
                allowNull: false
            },
            status: {
                type: DataTypes.ENUM('active', 'inactive', 'suspended', 'pending'),
                defaultValue: 'active',
                allowNull: false
            },
            email_verified: {
                type: DataTypes.BOOLEAN,
                defaultValue: false,
                allowNull: false
            },
            email_verification_token: {
                type: DataTypes.STRING(255),
                allowNull: true
            },
            password_reset_token: {
                type: DataTypes.STRING(255),
                allowNull: true
            },
            password_reset_expires: {
                type: DataTypes.DATE,
                allowNull: true
            },
            last_login_at: {
                type: DataTypes.DATE,
                allowNull: true
            },
            last_login_ip: {
                type: DataTypes.INET,
                allowNull: true
            },
            login_attempts: {
                type: DataTypes.INTEGER,
                defaultValue: 0,
                allowNull: false
            },
            locked_until: {
                type: DataTypes.DATE,
                allowNull: true
            },
            
            // HIPAA/GDPR Compliance Fields
            data_consent_given: {
                type: DataTypes.BOOLEAN,
                defaultValue: false,
                allowNull: false
            },
            data_consent_date: {
                type: DataTypes.DATE,
                allowNull: true
            },
            data_retention_until: {
                type: DataTypes.DATE,
                allowNull: true
            },
            data_anonymized: {
                type: DataTypes.BOOLEAN,
                defaultValue: false,
                allowNull: false
            },
            gdpr_delete_requested: {
                type: DataTypes.BOOLEAN,
                defaultValue: false,
                allowNull: false
            },
            gdpr_delete_requested_at: {
                type: DataTypes.DATE,
                allowNull: true
            },
            
            // Additional Profile Fields
            phone: {
                type: DataTypes.STRING(20),
                allowNull: true
            },
            organization: {
                type: DataTypes.STRING(255),
                allowNull: true
            },
            job_title: {
                type: DataTypes.STRING(100),
                allowNull: true
            },
            department: {
                type: DataTypes.STRING(100),
                allowNull: true
            },
            timezone: {
                type: DataTypes.STRING(50),
                defaultValue: 'UTC',
                allowNull: false
            },
            locale: {
                type: DataTypes.STRING(10),
                defaultValue: 'en-US',
                allowNull: false
            },
            preferences: {
                type: DataTypes.JSONB,
                defaultValue: {
                    notifications: { email: true, browser: true },
                    dashboard: { defaultView: 'overview' }
                }
            },
            
            // Audit Fields
            created_by: {
                type: DataTypes.UUID,
                allowNull: true
            },
            updated_by: {
                type: DataTypes.UUID,
                allowNull: true
            }
        }, {
            tableName: 'users',
            underscored: true,
            timestamps: true,
            paranoid: false,
            indexes: [
                { unique: true, fields: ['email'] },
                { fields: ['status'] },
                { fields: ['role'] },
                { fields: ['last_login_at'] }
            ]
        });

        // Interface Model - for wizard-created integrations
        this.models.Interface = this.sequelize.define('Interface', {
            id: {
                type: DataTypes.UUID,
                defaultValue: DataTypes.UUIDV4,
                primaryKey: true
            },
            user_id: {
                type: DataTypes.UUID,
                allowNull: false,
                references: {
                    model: 'users',
                    key: 'id'
                }
            },
            name: {
                type: DataTypes.STRING(255),
                allowNull: false
            },
            description: {
                type: DataTypes.TEXT,
                allowNull: true
            },
            message_type: {
                type: DataTypes.STRING(50),
                allowNull: true
            },
            source_type: {
                type: DataTypes.STRING(50),
                allowNull: false
            },
            source_connectivity: {
                type: DataTypes.STRING(50),
                allowNull: false
            },
            target_type: {
                type: DataTypes.STRING(50),
                allowNull: false
            },
            target_connectivity: {
                type: DataTypes.STRING(50),
                allowNull: false
            },
            source_config: {
                type: DataTypes.JSONB,
                defaultValue: {}
            },
            target_config: {
                type: DataTypes.JSONB,
                defaultValue: {}
            },
            processing_rules: {
                type: DataTypes.JSONB,
                defaultValue: {}
            },
            transformation_mapping: {
                type: DataTypes.JSONB,
                defaultValue: {}
            },
            status: {
                type: DataTypes.ENUM('draft', 'active', 'paused', 'stopped', 'error'),
                defaultValue: 'draft',
                allowNull: false
            },
            is_active: {
                type: DataTypes.BOOLEAN,
                defaultValue: true
            },
            version: {
                type: DataTypes.INTEGER,
                defaultValue: 1
            },
            total_processed: {
                type: DataTypes.INTEGER,
                defaultValue: 0
            },
            successful_processed: {
                type: DataTypes.INTEGER,
                defaultValue: 0
            },
            failed_processed: {
                type: DataTypes.INTEGER,
                defaultValue: 0
            },
            last_processed_at: {
                type: DataTypes.DATE,
                allowNull: true
            },
            created_by: {
                type: DataTypes.UUID,
                allowNull: true
            },
            updated_by: {
                type: DataTypes.UUID,
                allowNull: true
            }
        }, {
            tableName: 'interfaces',
            underscored: true,
            timestamps: true,
            indexes: [
                { fields: ['user_id'] },
                { fields: ['status'] },
                { fields: ['source_type'] },
                { fields: ['target_type'] },
                { fields: ['created_at'] }
            ]
        });

        // Audit Log Model - matching actual schema
        this.models.AuditLog = this.sequelize.define('AuditLog', {
            id: {
                type: DataTypes.UUID,
                defaultValue: DataTypes.UUIDV4,
                primaryKey: true
            },
            user_id: {
                type: DataTypes.UUID,
                allowNull: true,
                references: {
                    model: 'users',
                    key: 'id'
                }
            },
            session_id: {
                type: DataTypes.STRING(255),
                allowNull: true
            },
            action: {
                type: DataTypes.STRING(100),
                allowNull: false
            },
            entity_type: {
                type: DataTypes.STRING(100),
                allowNull: false
            },
            entity_id: {
                type: DataTypes.UUID,
                allowNull: true
            },
            old_values: {
                type: DataTypes.JSONB,
                allowNull: true
            },
            new_values: {
                type: DataTypes.JSONB,
                allowNull: true
            },
            metadata: {
                type: DataTypes.JSONB,
                allowNull: true
            },
            ip_address: {
                type: DataTypes.INET,
                allowNull: true
            },
            user_agent: {
                type: DataTypes.TEXT,
                allowNull: true
            },
            request_id: {
                type: DataTypes.UUID,
                allowNull: true
            },
            result: {
                type: DataTypes.ENUM('success', 'failure', 'error'),
                defaultValue: 'success',
                allowNull: false
            },
            error_message: {
                type: DataTypes.TEXT,
                allowNull: true
            },
            risk_level: {
                type: DataTypes.ENUM('low', 'medium', 'high', 'critical'),
                defaultValue: 'low',
                allowNull: false
            },
            compliance_flags: {
                type: DataTypes.JSONB,
                defaultValue: {}
            }
        }, {
            tableName: 'audit_logs',
            underscored: true,
            timestamps: true,
            updatedAt: false, // Audit logs are immutable
            indexes: [
                { fields: ['user_id'] },
                { fields: ['action'] },
                { fields: ['created_at'] },
                { fields: ['risk_level'] }
            ]
        });

        // Define associations
        this.models.Interface.belongsTo(this.models.User, {
            foreignKey: 'user_id',
            as: 'owner'
        });

        this.models.User.hasMany(this.models.Interface, {
            foreignKey: 'user_id',
            as: 'interfaces'
        });

        this.models.AuditLog.belongsTo(this.models.User, {
            foreignKey: 'user_id',
            as: 'user'
        });

        this.models.User.hasMany(this.models.AuditLog, {
            foreignKey: 'user_id',
            as: 'auditLogs'
        });

        // Helper methods
        this.models.User.findByEmail = function(email) {
            return this.findOne({ where: { email: email.toLowerCase() } });
        };

        // Add instance method to get full name
        this.models.User.prototype.getFullName = function() {
            return `${this.first_name} ${this.last_name}`.trim();
        };

        this.models.AuditLog.logAction = async function(data) {
            try {
                return await this.create(data);
            } catch (error) {
                console.error('Failed to create audit log:', error);
                return null;
            }
        };

        console.log('✅ Database models defined (User, Interface, AuditLog)');
    }

    // In database.js, update the sync method:
async sync() {
    try {
        if (!this.isConnected) return false;
        
        // Skip sync in all environments to avoid conflicts with Flyway
        console.log('Skipping Sequelize sync - using Flyway-managed schema');
        return true;
    } catch (error) {
        console.error('Database sync failed:', error.message);
        return false;
    }
}

    // Create default admin in PostgreSQL
    async createDefaultAdmin() {
        try {
            if (!this.isConnected) return false;

            const adminEmail = 'admin@ezhealthkonnect.com';
            const existing = await this.models.User.findByEmail(adminEmail);
            
            if (!existing) {
                const bcrypt = require('bcryptjs');
                const hashedPassword = await bcrypt.hash('admin123', 12);
                
                const admin = await this.models.User.create({
                    email: adminEmail,
                    password_hash: hashedPassword,
                    first_name: 'System',
                    last_name: 'Administrator',
                    role: 'admin',
                    status: 'active',
                    email_verified: true,
                    created_by: null, // System created
                    data_consent_given: true,
                    data_consent_date: new Date()
                });

                // Log admin creation
                await this.models.AuditLog.logAction({
                    user_id: admin.id,
                    action: 'ADMIN_CREATED',
                    entity_type: 'User',
                    entity_id: admin.id,
                    metadata: {
                        email: admin.email,
                        created_by: 'system'
                    },
                    risk_level: 'high',
                    compliance_flags: {
                        admin_account: true
                    }
                });

                console.log('✅ Default admin created in PostgreSQL');
                return admin;
            } else {
                console.log('✅ Default admin already exists in PostgreSQL');
                return existing;
            }

        } catch (error) {
            console.error('❌ Failed to create default admin:', error.message);
            return null;
        }
    }

    // Health check
    async healthCheck() {
        try {
            if (!this.isConnected) return { status: 'disconnected' };
            
            await this.sequelize.authenticate();
            
            const userCount = await this.models.User.count();
            const interfaceCount = await this.models.Interface.count();
            const auditCount = await this.models.AuditLog.count();
            
            return {
                status: 'healthy',
                userCount,
                interfaceCount,
                auditCount,
                lastCheck: new Date()
            };
        } catch (error) {
            return {
                status: 'unhealthy',
                error: error.message
            };
        }
    }

    // Get PostgreSQL user in server.js compatible format
    formatUserForServerJS(pgUser) {
        return {
            id: pgUser.id,
            email: pgUser.email,
            password: pgUser.password_hash,
            name: pgUser.getFullName(), // Combine first_name + last_name
            role: pgUser.role,
            status: pgUser.status,
            lastLogin: pgUser.last_login_at,
            createdAt: pgUser.created_at,
            source: 'postgresql'
        };
    }

    // Graceful disconnect
    async disconnect() {
        try {
            if (this.sequelize) {
                await this.sequelize.close();
                console.log('✅ Database disconnected');
            }
        } catch (error) {
            console.error('❌ Error disconnecting:', error.message);
        }
    }
}

// Export singleton
const database = new DatabaseManager();
module.exports = database;