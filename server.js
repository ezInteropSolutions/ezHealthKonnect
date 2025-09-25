// server.js - PostgreSQL only version with Class-Based Multi-Interface Support
const app = require('./app');
const InterfaceEngineManager = require('./services/InterfaceEngineManager');
const { PORT = 3000 } = process.env;

// Global interface engine manager instance
let interfaceEngine = null;

/**
 * Initialize Multi-Interface Integration Engine (Class-Based)
 */
async function initializeInterfaceEngine() {
    try {
        // Create new interface engine manager instance
        interfaceEngine = new InterfaceEngineManager();

        // Initialize the engine
        await interfaceEngine.initialize();

        console.log('✅ Class-based Interface Engine Manager initialized successfully');

    } catch (error) {
        console.error('❌ Failed to initialize Interface Engine Manager:', error.message);
        console.error('🚨 Integration engine startup failed - interfaces will not process messages');
        console.error('💡 Server will continue running but interfaces will be inactive\n');

        // Set interfaceEngine to null on failure
        interfaceEngine = null;
    }
}

/**
 * Shutdown interface engine gracefully
 */
async function shutdownInterfaceEngine() {
    if (interfaceEngine) {
        try {
            await interfaceEngine.shutdown();
            console.log('✅ Interface Engine Manager shutdown complete');
        } catch (error) {
            console.error('❌ Error during interface engine shutdown:', error.message);
        } finally {
            interfaceEngine = null;
        }
    }
}

/**
 * Get interface engine instance (for API access)
 */
function getInterfaceEngine() {
    return interfaceEngine;
}

/**
 * Get current interface status (backward compatibility)
 */
function getInterfaceStatus() {
    if (interfaceEngine) {
        return interfaceEngine.getInterfaceStatus();
    }
    return [];
}

/**
 * Get engine health status
 */
function getEngineHealth() {
    if (interfaceEngine) {
        return interfaceEngine.getEngineHealth();
    }
    return {
        isRunning: false,
        activeInterfaces: 0,
        error: 'Engine not initialized'
    };
}

async function startServer() {
    try {
        console.log('🚀 Starting ezHealthKonnect Enterprise Server...');
        console.log('🗄️  Mode: PostgreSQL Only (Production Ready)');
        
        // Verify PostgreSQL connection before starting server
        try {
            const database = require('./config/database');
            const connected = await database.connect();
            
            if (!connected || !database.isConnected) {
                throw new Error('PostgreSQL connection required for production mode');
            }
            
            await database.sync();
            console.log('✅ PostgreSQL database verified and ready');

            // Initialize Multi-Interface Integration Engine
            await initializeInterfaceEngine();
            
        } catch (dbError) {
            console.error('❌ Database initialization failed:', dbError.message);
            console.error('🚨 Cannot start server without PostgreSQL database');
            console.error('📋 Please check:');
            console.error('   - PostgreSQL service is running');
            console.error('   - Database credentials in .env file');
            console.error('   - Database "ezhealthkonnect" exists');
            console.error('   - Database tables are created');
            process.exit(1);
        }
        
        // Start Express server
        app.listen(PORT, () => {
            console.log(`\n🏥 ezHealthKonnect Enterprise Server`);
            console.log(`📍 Server: http://localhost:${PORT}`);
            console.log(`🔐 Login: http://localhost:${PORT}/login.html`);
            console.log(`👥 User Management: http://localhost:${PORT}/user-management`);
            console.log(`💓 Health: http://localhost:${PORT}/api/status`);
            console.log(`🗄️  Storage: PostgreSQL Only (Production Mode)`);
            console.log(`🛡️  Compliance: HIPAA/GDPR audit logging via PostgreSQL`);
            console.log(`📊 Audit Logs: PostgreSQL audit_logs table + logs/audit.log backup`);
            
            console.log(`\n🔑 Default Admin Credentials:`);
            console.log(`   Email: admin@ezhealthkonnect.com`);
            console.log(`   Password: admin123`);
            
            console.log('\n✅ Server ready - PostgreSQL-only production mode active!');
            console.log('🚀 All user data now stored securely in PostgreSQL database');
        });
        
    } catch (error) {
        console.error('❌ Server startup failed:', error);
        process.exit(1);
    }
}

// Graceful shutdown handling for interface engine
process.on('SIGTERM', async () => {
    console.log('\n🔄 SIGTERM received, shutting down gracefully...');
    await shutdownInterfaceEngine();
    process.exit(0);
});

process.on('SIGINT', async () => {
    console.log('\n🔄 SIGINT received, shutting down gracefully...');
    await shutdownInterfaceEngine();
    process.exit(0);
});

// Export interface engine functions for API use
module.exports = {
    getInterfaceEngine,
    getInterfaceStatus,
    getEngineHealth,
    shutdownInterfaceEngine,
    initializeInterfaceEngine
};

startServer();