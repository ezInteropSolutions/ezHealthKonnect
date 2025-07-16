// scripts/migrate-interfaces.js - COMPLETE FIXED VERSION
// Run this to add interface management tables to your existing PostgreSQL database

const database = require('../config/database');

async function runMigration() {
    console.log('🚀 Starting Interface Management Migration...');
    
    try {
        const connected = await database.connect();
        if (!connected) {
            throw new Error('Failed to connect to database');
        }
        console.log('✅ Connected to PostgreSQL database');
        
        // 1. Create interfaces table
        console.log('📋 Creating interfaces table...');
        await database.sequelize.query(`
            CREATE TABLE IF NOT EXISTS interfaces (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                name VARCHAR(255) NOT NULL,
                description TEXT,
                
                -- Interface Configuration
                source_type VARCHAR(50) NOT NULL CHECK (source_type IN ('file', 'tcp', 'http', 'manual', 'database')),
                target_type VARCHAR(50) NOT NULL CHECK (target_type IN ('fhir', 'database', 'file', 'http', 'tcp')),
                message_type VARCHAR(50) DEFAULT 'auto-detect',
                
                -- JSON Configuration Fields
                source_config JSONB DEFAULT '{}',
                target_config JSONB DEFAULT '{}',
                processing_rules JSONB DEFAULT '{}',
                transformation_mapping JSONB DEFAULT '{}',
                
                -- Status and Control
                status VARCHAR(20) DEFAULT 'stopped' CHECK (status IN ('running', 'stopped', 'paused', 'error', 'draft')),
                
                -- Statistics
                total_processed INTEGER DEFAULT 0,
                successful_processed INTEGER DEFAULT 0,
                failed_processed INTEGER DEFAULT 0,
                last_processed_at TIMESTAMPTZ,
                
                -- Metadata
                created_at TIMESTAMPTZ DEFAULT NOW(),
                updated_at TIMESTAMPTZ DEFAULT NOW(),
                created_by UUID REFERENCES users(id),
                updated_by UUID REFERENCES users(id),
                
                -- Versioning
                version INTEGER DEFAULT 1,
                is_active BOOLEAN DEFAULT true,
                
                -- Indexes for performance
                CONSTRAINT unique_interface_name_per_user UNIQUE (user_id, name, is_active)
            );
        `);
        
        // 2. Create interface_versions table
        console.log('📋 Creating interface_versions table...');
        await database.sequelize.query(`
            CREATE TABLE IF NOT EXISTS interface_versions (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
                version INTEGER NOT NULL,
                
                -- Snapshot of configuration at this version
                name VARCHAR(255) NOT NULL,
                description TEXT,
                source_type VARCHAR(50) NOT NULL,
                target_type VARCHAR(50) NOT NULL,
                message_type VARCHAR(50),
                source_config JSONB,
                target_config JSONB,
                processing_rules JSONB,
                transformation_mapping JSONB,
                status VARCHAR(20),
                
                -- Change metadata
                change_type VARCHAR(20) NOT NULL CHECK (change_type IN ('created', 'updated', 'deleted', 'status_changed')),
                change_summary TEXT,
                changed_fields JSONB,
                
                -- Audit info
                created_at TIMESTAMPTZ DEFAULT NOW(),
                created_by UUID NOT NULL REFERENCES users(id),
                user_agent TEXT,
                ip_address INET,
                
                CONSTRAINT unique_interface_version UNIQUE (interface_id, version)
            );
        `);
        
        // 3. Create interface_audit_log table
        console.log('📋 Creating interface_audit_log table...');
        await database.sequelize.query(`
            CREATE TABLE IF NOT EXISTS interface_audit_log (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                interface_id UUID REFERENCES interfaces(id) ON DELETE CASCADE,
                user_id UUID NOT NULL REFERENCES users(id),
                
                -- Action details
                action VARCHAR(50) NOT NULL,
                entity_type VARCHAR(50) DEFAULT 'interface',
                
                -- Change details
                old_values JSONB,
                new_values JSONB,
                changed_fields TEXT[],
                
                -- Context
                reason TEXT,
                metadata JSONB DEFAULT '{}',
                
                -- Request info
                ip_address INET,
                user_agent TEXT,
                session_id VARCHAR(255),
                request_id UUID,
                
                -- Compliance
                retention_date TIMESTAMPTZ DEFAULT (NOW() + INTERVAL '7 years'),
                
                created_at TIMESTAMPTZ DEFAULT NOW()
            );
        `);
        
        // 4. Create interface_sessions table
        console.log('📋 Creating interface_sessions table...');
        await database.sequelize.query(`
            CREATE TABLE IF NOT EXISTS interface_sessions (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                interface_id UUID NOT NULL REFERENCES interfaces(id) ON DELETE CASCADE,
                
                -- Session details
                status VARCHAR(20) NOT NULL CHECK (status IN ('starting', 'running', 'stopping', 'stopped', 'error')),
                started_at TIMESTAMPTZ DEFAULT NOW(),
                stopped_at TIMESTAMPTZ,
                started_by UUID NOT NULL REFERENCES users(id),
                
                -- Runtime info
                process_id INTEGER,
                last_heartbeat TIMESTAMPTZ,
                error_message TEXT,
                
                -- Performance metrics
                messages_processed INTEGER DEFAULT 0,
                bytes_processed BIGINT DEFAULT 0,
                avg_processing_time_ms DECIMAL(10,2),
                
                created_at TIMESTAMPTZ DEFAULT NOW()
            );
        `);
        
        // 5. Create indexes
        console.log('📋 Creating indexes...');
        const indexes = [
            'CREATE INDEX IF NOT EXISTS idx_interfaces_user_id ON interfaces(user_id)',
            'CREATE INDEX IF NOT EXISTS idx_interfaces_status ON interfaces(status)',
            'CREATE INDEX IF NOT EXISTS idx_interfaces_updated_at ON interfaces(updated_at)',
            'CREATE INDEX IF NOT EXISTS idx_interfaces_active ON interfaces(is_active) WHERE is_active = true',
            'CREATE INDEX IF NOT EXISTS idx_interface_versions_interface_id ON interface_versions(interface_id)',
            'CREATE INDEX IF NOT EXISTS idx_interface_versions_created_at ON interface_versions(created_at)',
            'CREATE INDEX IF NOT EXISTS idx_interface_audit_log_interface_id ON interface_audit_log(interface_id)',
            'CREATE INDEX IF NOT EXISTS idx_interface_audit_log_user_id ON interface_audit_log(user_id)',
            'CREATE INDEX IF NOT EXISTS idx_interface_audit_log_created_at ON interface_audit_log(created_at)',
            'CREATE INDEX IF NOT EXISTS idx_interface_audit_log_action ON interface_audit_log(action)',
            'CREATE INDEX IF NOT EXISTS idx_interface_sessions_interface_id ON interface_sessions(interface_id)',
            'CREATE INDEX IF NOT EXISTS idx_interface_sessions_status ON interface_sessions(status)'
        ];
        
        for (const indexSql of indexes) {
            await database.sequelize.query(indexSql);
        }
        
        // 6. Create versioning trigger function
        console.log('📋 Creating versioning trigger...');
        await database.sequelize.query(`
            CREATE OR REPLACE FUNCTION update_interface_version()
            RETURNS TRIGGER AS $$
            BEGIN
                -- Update the updated_at timestamp
                NEW.updated_at = NOW();
                
                -- Increment version number
                NEW.version = OLD.version + 1;
                
                -- Create version record
                INSERT INTO interface_versions (
                    interface_id, version, name, description, source_type, target_type,
                    message_type, source_config, target_config, processing_rules,
                    transformation_mapping, status, change_type, change_summary,
                    created_by
                ) VALUES (
                    NEW.id, NEW.version, NEW.name, NEW.description, NEW.source_type, NEW.target_type,
                    NEW.message_type, NEW.source_config, NEW.target_config, NEW.processing_rules,
                    NEW.transformation_mapping, NEW.status, 
                    CASE WHEN TG_OP = 'INSERT' THEN 'created' ELSE 'updated' END,
                    CASE WHEN TG_OP = 'INSERT' THEN 'Interface created' ELSE 'Interface configuration updated' END,
                    NEW.updated_by
                );
                
                RETURN NEW;
            END;
            $$ LANGUAGE plpgsql;
        `);
        
        // 7. Create trigger
        await database.sequelize.query(`
            DROP TRIGGER IF EXISTS trigger_interface_versioning ON interfaces;
            CREATE TRIGGER trigger_interface_versioning
                BEFORE UPDATE ON interfaces
                FOR EACH ROW
                EXECUTE FUNCTION update_interface_version();
        `);
        
        // 8. Create utility views (FIXED - no more u.name or CONCAT issues)
        console.log('📋 Creating utility views...');
        await database.sequelize.query(`
            CREATE OR REPLACE VIEW interface_summary AS
            SELECT 
                i.id,
                i.name,
                i.description,
                i.source_type,
                i.target_type,
                i.message_type,
                i.status,
                i.total_processed,
                i.successful_processed,
                i.failed_processed,
                CASE 
                    WHEN i.total_processed > 0 
                    THEN ROUND((i.successful_processed::DECIMAL / i.total_processed * 100), 2)
                    ELSE 0 
                END AS success_rate_percentage,
                i.last_processed_at,
                i.created_at,
                i.updated_at,
                i.version,
                COALESCE(u.first_name || ' ' || u.last_name, 'Unknown User') AS created_by_name,
                u.email AS created_by_email
            FROM interfaces i
            LEFT JOIN users u ON i.created_by = u.id
            WHERE i.is_active = true;
        `);
        
        await database.sequelize.query(`
            CREATE OR REPLACE VIEW recent_interface_changes AS
            SELECT 
                iv.id,
                iv.interface_id,
                i.name AS interface_name,
                iv.version,
                iv.change_type,
                iv.change_summary,
                iv.changed_fields,
                iv.created_at,
                COALESCE(u.first_name || ' ' || u.last_name, 'Unknown User') AS changed_by_name,
                u.email AS changed_by_email
            FROM interface_versions iv
            LEFT JOIN interfaces i ON iv.interface_id = i.id
            LEFT JOIN users u ON iv.created_by = u.id
            ORDER BY iv.created_at DESC
            LIMIT 50;
        `);
        
        // 9. Insert sample data for testing (FIXED - no more $1 parameters)
        console.log('📋 Inserting sample data...');
        const adminUsers = await database.sequelize.query(
            "SELECT id FROM users WHERE email = 'admin@ezhealthkonnect.com' LIMIT 1",
            { type: database.sequelize.QueryTypes.SELECT }
        );
        
        if (adminUsers.length > 0) {
            const adminUserId = adminUsers[0].id;
            
            await database.sequelize.query(`
                INSERT INTO interfaces (
                    user_id, name, description, source_type, target_type, message_type,
                    source_config, target_config, processing_rules, created_by, updated_by
                ) VALUES 
                (:adminId, 'ADT Patient Admissions', 'Real-time ADT admission messages to FHIR Patient resources', 'tcp', 'fhir', 'ADT^A01', 
                 '{"host": "0.0.0.0", "port": 2575, "timeout": 30000}', 
                 '{"endpoint": "https://fhir.example.com/api", "timeout": 5000, "auth": "bearer"}', 
                 '{"validateStructure": true, "requirePID": true, "autoAck": true}', :adminId, :adminId),
                (:adminId, 'Lab Results TCP Listener', 'Real-time ORU lab results from TCP port 2576', 'tcp', 'database', 'ORU^R01',
                 '{"host": "0.0.0.0", "port": 2576, "timeout": 30000}',
                 '{"table": "lab_results", "database": "clinical_data"}',
                 '{"validateResults": true, "filterAbnormal": false}', :adminId, :adminId)
                ON CONFLICT (user_id, name, is_active) DO NOTHING
            `, {
                replacements: { adminId: adminUserId },
                type: database.sequelize.QueryTypes.INSERT
            });
            
            console.log('✅ Sample interfaces created for admin user');
        } else {
            console.log('⚠️  No admin user found, skipping sample data');
        }
        
        console.log('\n🎉 Migration completed successfully!');
        console.log('📊 Interface Management System is now ready');
        console.log('🔧 Tables created:');
        console.log('   - interfaces (main configuration)');
        console.log('   - interface_versions (change history)');
        console.log('   - interface_audit_log (detailed audit trail)');
        console.log('   - interface_sessions (runtime tracking)');
        console.log('🔍 Views created:');
        console.log('   - interface_summary (dashboard queries)');
        console.log('   - recent_interface_changes (change tracking)');
        console.log('⚡ Triggers created for automatic versioning');
        
    } catch (error) {
        console.error('❌ Migration failed:', error);
        console.error('Error details:', error.message);
        process.exit(1);
    } finally {
        console.log('✅ Migration script completed');
    }
}

// Run migration if this file is executed directly
if (require.main === module) {
    runMigration();
}

module.exports = { runMigration };