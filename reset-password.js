// reset-password.js - Reset admin password
require('dotenv').config();
const database = require('./config/database');
const bcrypt = require('bcryptjs');

async function resetPassword() {
    try {
        console.log('🔑 Resetting admin password...');
        
        const connected = await database.connect();
        if (!connected) {
            console.log('❌ Could not connect to database');
            return;
        }

        // Find admin user
        const admin = await database.models.User.findByEmail('admin@ezhealthkonnect.com');
        if (!admin) {
            console.log('❌ Admin user not found');
            return;
        }

        // Set new password (change this to whatever you want)
        const newPassword = 'Admin123!';  // Change this!
        const hashedPassword = await bcrypt.hash(newPassword, 12);

        // Update password
        await admin.update({ 
            password_hash: hashedPassword,
            updated_by: admin.id // Log who changed it
        });

        // Log the password change
        await database.models.AuditLog.logAction({
            user_id: admin.id,
            action: 'PASSWORD_RESET',
            entity_type: 'User',
            entity_id: admin.id,
            metadata: {
                email: admin.email,
                reset_by: 'admin_script',
                reset_date: new Date()
            },
            risk_level: 'high',
            compliance_flags: {
                password_change: true,
                admin_action: true
            }
        });

        console.log('✅ Password reset successfully!');
        console.log(`📧 Email: ${admin.email}`);
        console.log(`🔑 New Password: ${newPassword}`);
        
        await database.disconnect();

    } catch (error) {
        console.error('❌ Password reset failed:', error.message);
    }
}

resetPassword();