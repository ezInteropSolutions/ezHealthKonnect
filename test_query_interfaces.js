// Script to query interfaces from database
require('dotenv').config();
const database = require('./config/database');

async function queryInterfaces() {
    try {
        console.log('Connecting to database...');
        const connected = await database.connect();

        if (!connected) {
            console.log('Failed to connect to database');
            return;
        }

        console.log('\nQuerying interfaces table...');
        const interfaces = await database.sequelize.query(
            `SELECT id, name, source_type, target_type, source_connectivity, target_connectivity,
                    message_type, status, is_active, description,
                    source_config, target_config, processing_rules, transformation_mapping,
                    total_processed, successful_processed, failed_processed,
                    created_at
             FROM interfaces
             ORDER BY created_at DESC`,
            { type: database.sequelize.QueryTypes.SELECT }
        );

        console.log(`\nFound ${interfaces.length} interfaces:`);
        console.log('=====================================');

        interfaces.forEach((intf, index) => {
            console.log(`\n${index + 1}. Interface: ${intf.name}`);
            console.log(`   ID: ${intf.id}`);
            console.log(`   Type: ${intf.source_type} (${intf.source_connectivity}) → ${intf.target_type} (${intf.target_connectivity})`);
            console.log(`   Message Type: ${intf.message_type || 'Not specified'}`);
            console.log(`   Status: ${intf.status} | Active: ${intf.is_active}`);
            console.log(`   Description: ${intf.description || 'No description'}`);
            console.log(`   Processed: ${intf.total_processed} total, ${intf.successful_processed} success, ${intf.failed_processed} failed`);
            console.log(`   Created: ${intf.created_at}`);

            if (intf.source_config && Object.keys(intf.source_config).length > 0) {
                console.log(`   Source Config: ${JSON.stringify(intf.source_config, null, 4)}`);
            }
            if (intf.target_config && Object.keys(intf.target_config).length > 0) {
                console.log(`   Target Config: ${JSON.stringify(intf.target_config, null, 4)}`);
            }
            if (intf.processing_rules && Object.keys(intf.processing_rules).length > 0) {
                console.log(`   Processing Rules: ${JSON.stringify(intf.processing_rules, null, 4)}`);
            }
        });

        // Check for interface-specific message tables
        console.log('\n\nChecking for interface-specific message tables...');
        const tables = await database.sequelize.query(
            `SELECT table_name FROM information_schema.tables
             WHERE table_schema = 'public' AND table_name LIKE 'messages_intf_%'
             ORDER BY table_name`,
            { type: database.sequelize.QueryTypes.SELECT }
        );

        console.log(`\nFound ${tables.length} interface message tables:`);
        tables.forEach(table => {
            const tableName = table.table_name || table[0];
            console.log(`   - ${tableName}`);
        });

        // Also check for other relevant tables
        console.log('\n\nChecking for other relevant tables...');
        const otherTables = await database.sequelize.query(
            `SELECT table_name FROM information_schema.tables
             WHERE table_schema = 'public' AND (
                table_name LIKE '%message%' OR
                table_name LIKE '%audit%' OR
                table_name LIKE '%interface%' OR
                table_name LIKE '%hl7%' OR
                table_name LIKE '%fhir%'
             )
             ORDER BY table_name`,
            { type: database.sequelize.QueryTypes.SELECT }
        );

        console.log(`\nFound ${otherTables.length} related tables:`);
        otherTables.forEach(table => {
            const tableName = table.table_name || table[0];
            console.log(`   - ${tableName}`);
        });

        // Check audit_logs table structure and content
        console.log('\n\nChecking audit_logs table...');
        const auditLogsQuery = `
            SELECT action, entity_type, result, risk_level, COUNT(*) as count
            FROM audit_logs
            GROUP BY action, entity_type, result, risk_level
            ORDER BY action, entity_type
        `;

        const auditResults = await database.sequelize.query(auditLogsQuery, {
            type: database.sequelize.QueryTypes.SELECT
        });

        console.log(`\nAudit logs summary (${auditResults.length} unique action types):`);
        auditResults.forEach(audit => {
            console.log(`   ${audit.action} (${audit.entity_type}) - ${audit.result} [${audit.risk_level}]: ${audit.count} entries`);
        });

        // Check message_audit_log table
        console.log('\n\nChecking message_audit_log table...');
        const messageAuditQuery = `
            SELECT event_type, COUNT(*) as count
            FROM message_audit_log
            GROUP BY event_type
            ORDER BY count DESC
        `;

        const messageAuditResults = await database.sequelize.query(messageAuditQuery, {
            type: database.sequelize.QueryTypes.SELECT
        });

        console.log(`\nMessage audit logs summary (${messageAuditResults.length} event types):`);
        messageAuditResults.forEach(audit => {
            console.log(`   ${audit.event_type}: ${audit.count} entries`);
        });

        await database.disconnect();

    } catch (error) {
        console.error('Error:', error);
        await database.disconnect();
    }
}

queryInterfaces();