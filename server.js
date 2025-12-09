// server.js - Node.js Frontend Server (UI, Auth, API Routing)
// All TCP/MLLP listeners and message processing handled by Go backend
const app = require('./app');
const { PORT = 3000 } = process.env;

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
            console.log('ℹ️  All TCP/MLLP listeners managed by Go backend (port 8080)');

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
        const server = app.listen(PORT, async () => {
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

            // Initialize interface deployment service (auto-start interfaces)
            try {
                const database = require('./config/database');
                const InterfaceDeploymentService = require('./services/InterfaceDeploymentService');
                const deploymentService = new InterfaceDeploymentService(database);

                // Store globally for API access
                global.deploymentService = deploymentService;

                // Initialize and auto-start interfaces (async, non-blocking)
                deploymentService.initializeOnStartup().catch(error => {
                    console.error('⚠️  Interface auto-start failed:', error.message);
                });
            } catch (error) {
                console.error('⚠️  Deployment service initialization failed:', error.message);
            }

            // Initialize log retention cleanup service (V33 - Interface-Level Logging)
            try {
                const LogRetentionCleanup = require('./services/LogRetentionCleanup');
                LogRetentionCleanup.start();
                console.log('🧹 Log retention cleanup service started');

                // Store globally for manual cleanup API access
                global.logRetentionCleanup = LogRetentionCleanup;
            } catch (error) {
                console.error('⚠️  Log retention cleanup service failed:', error.message);
            }
        });
        
    } catch (error) {
        console.error('❌ Server startup failed:', error);
        process.exit(1);
    }
}

// Graceful shutdown handling
process.on('SIGTERM', async () => {
    console.log('\n🔄 SIGTERM received, shutting down gracefully...');
    if (global.deploymentService) {
        global.deploymentService.cleanup();
    if (global.logRetentionCleanup) {
        global.logRetentionCleanup.stop();
    }
    }
    process.exit(0);
});

process.on('SIGINT', async () => {
    console.log('\n🔄 SIGINT received, shutting down gracefully...');
    if (global.deploymentService) {
        global.deploymentService.cleanup();
    if (global.logRetentionCleanup) {
        global.logRetentionCleanup.stop();
    }
    }
    process.exit(0);
});

startServer();