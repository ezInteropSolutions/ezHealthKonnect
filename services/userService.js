// services/userService.js - PostgreSQL only version
const bcrypt = require('bcryptjs');

class UserService {
    constructor() {
        this.database = null;
        this.usePostgreSQL = false;
        this.initialized = false;
        
        // Initialize immediately but don't wait
        this.initializeStorage();
    }

    async initializeStorage() {
        console.log('🔄 Initializing UserService storage (PostgreSQL only)...');
        
        try {
            // Import the database manager
            this.database = require('../config/database');
            console.log('📦 Database module loaded successfully');
            
            // Try to connect to PostgreSQL
            const connected = await this.database.connect();
            
            if (connected && this.database.isConnected) {
                console.log('🐘 PostgreSQL connection established');
                console.log('🔍 Checking PostgreSQL models...');
                console.log('Available models:', Object.keys(this.database.models || {}));
                
                if (this.database.models && this.database.models.User) {
                    console.log('✅ User model found');
                    this.usePostgreSQL = true;
                    
                    // Initialize the database (sync tables)
                    await this.database.sync();
                    
                    // Create default admin if needed
                    await this.createDefaultAdmin();
                } else {
                    console.log('❌ User model not found in PostgreSQL');
                    throw new Error('PostgreSQL User model not available');
                }
            } else {
                console.log('❌ PostgreSQL connection failed');
                throw new Error('PostgreSQL connection failed');
            }
            
            this.initialized = true;
            console.log('✅ UserService initialization complete - PostgreSQL only mode active');
            
        } catch (error) {
            console.error('❌ UserService initialization failed:', error.message);
            console.error('🚨 PostgreSQL is required - application cannot start without database');
            throw error; // Don't continue without database
        }
    }

    async findByEmail(email) {
        // Wait for initialization if not complete
        if (!this.initialized) {
            console.log('⏳ Waiting for UserService initialization...');
            let attempts = 0;
            while (!this.initialized && attempts < 50) {
                await new Promise(resolve => setTimeout(resolve, 100));
                attempts++;
            }
            if (!this.initialized) {
                throw new Error('UserService initialization timeout');
            }
        }
        
        console.log(`🔍 Looking for user: ${email}`);
        
        try {
            const pgUser = await this.database.models.User.findByEmail(email);
            if (pgUser) {
                console.log(`✅ Found user in PostgreSQL: ${email}`);
                return {
                    id: pgUser.id,
                    email: pgUser.email,
                    password: pgUser.password_hash,
                    name: pgUser.getFullName(),
                    role: pgUser.role,
                    status: pgUser.status,
                    lastLogin: pgUser.last_login_at,
                    createdAt: pgUser.created_at,
                    source: 'postgresql'
                };
            }
        } catch (error) {
            console.error('PostgreSQL user lookup failed:', error.message);
            throw error;
        }
        
        console.log(`❌ User not found: ${email}`);
        return null;
    }

    async updateLastLogin(userId, email, ipAddress) {
        const loginTime = new Date();
        
        try {
            const user = await this.database.models.User.findByPk(userId);
            if (user) {
                await user.update({ 
                    last_login_at: loginTime,
                    last_login_ip: ipAddress,
                    login_attempts: 0
                });
                console.log(`✅ Updated last login in PostgreSQL for: ${email}`);
            } else {
                console.log(`⚠️  User not found for last login update: ${userId}`);
            }
        } catch (error) {
            console.error('PostgreSQL last login update failed:', error.message);
            throw error;
        }
    }

    async createUser(userData, createdBy) {
        const { email, password, name, role } = userData;
        
        console.log(`🆕 Creating user: ${email} with role: ${role}`);
        console.log(`👤 Created by: ${createdBy}`);
        
        // Check if user exists
        const existing = await this.findByEmail(email);
        if (existing) {
            throw new Error('User with this email already exists');
        }
        
        // Hash password
        const hashedPassword = await bcrypt.hash(password, 12);
        console.log('🔐 Password hashed successfully');
        
        try {
            console.log('🐘 Creating user in PostgreSQL...');
            
            const [firstName, ...lastNameParts] = name.split(' ');
            const lastName = lastNameParts.join(' ') || '';
            
            console.log(`📝 PostgreSQL user data: firstName="${firstName}", lastName="${lastName}"`);
            
            // Find the creator's UUID if createdBy is an email
            let createdByUuid = null;
            if (createdBy && typeof createdBy === 'string' && createdBy.includes('@')) {
                try {
                    const creator = await this.database.models.User.findByEmail(createdBy);
                    if (creator) {
                        createdByUuid = creator.id;
                        console.log(`🔍 Found creator UUID: ${createdByUuid} for email: ${createdBy}`);
                    } else {
                        console.log(`⚠️  Creator not found in PostgreSQL: ${createdBy}`);
                    }
                } catch (error) {
                    console.log(`⚠️  Could not lookup creator: ${error.message}`);
                }
            } else if (createdBy && typeof createdBy === 'string' && !createdBy.includes('@')) {
                // Assume it's already a UUID
                createdByUuid = createdBy;
            }
            
            const pgUser = await this.database.models.User.create({
                email: email.toLowerCase(),
                password_hash: hashedPassword,
                first_name: firstName,
                last_name: lastName,
                role: role,
                status: 'active',
                email_verified: true,
                created_by: createdByUuid,
                data_consent_given: true,
                data_consent_date: new Date()
            });
            
            console.log(`✅ User created successfully in PostgreSQL with ID: ${pgUser.id}`);
            
            return {
                id: pgUser.id,
                email: pgUser.email,
                name: pgUser.getFullName(),
                role: pgUser.role,
                status: pgUser.status,
                createdAt: pgUser.created_at,
                source: 'postgresql'
            };
            
        } catch (error) {
            console.error('❌ PostgreSQL user creation failed:', error);
            console.error('Error details:', {
                message: error.message,
                code: error.code,
                detail: error.detail
            });
            
            throw new Error(`Database error: ${error.message}`);
        }
    }

    async getAllUsers() {
        try {
            const pgUsers = await this.database.models.User.findAll({
                attributes: ['id', 'email', 'first_name', 'last_name', 'role', 'status', 'created_at', 'last_login_at'],
                order: [['created_at', 'DESC']]
            });
            
            return pgUsers.map(user => ({
                id: user.id,
                email: user.email,
                name: user.getFullName(),
                role: user.role,
                status: user.status,
                createdAt: user.created_at,
                lastLogin: user.last_login_at,
                source: 'postgresql'
            }));
        } catch (error) {
            console.error('Error fetching PostgreSQL users:', error.message);
            throw error;
        }
    }

    async createDefaultAdmin() {
        console.log('🔧 Creating default admin user...');
        
        try {
            const adminEmail = 'admin@ezhealthkonnect.com';
            const adminPassword = 'admin123';
            
            // Check if admin already exists
            const existingAdmin = await this.database.models.User.findByEmail(adminEmail);
            if (existingAdmin) {
                console.log('✅ Default admin already exists');
                return;
            }
            
            const hashedPassword = await bcrypt.hash(adminPassword, 12);
            console.log('🔐 Password hashed for default admin');
            
            const adminUser = await this.database.models.User.create({
                email: adminEmail,
                password_hash: hashedPassword,
                first_name: 'System',
                last_name: 'Administrator',
                role: 'admin',
                status: 'active',
                email_verified: true,
                created_by: null,
                data_consent_given: true,
                data_consent_date: new Date()
            });
            
            console.log(`✅ Default admin created in PostgreSQL with ID: ${adminUser.id}`);
            
        } catch (error) {
            console.error('❌ Error creating default admin:', error);
            throw error;
        }
    }
}

module.exports = new UserService(); // Singleton