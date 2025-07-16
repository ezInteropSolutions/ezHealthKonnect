// server.js - PostgreSQL only version
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

startServer();