#!/usr/bin/env node
// scripts/create_missing_interface_tables.js
// Creates PostgreSQL tables and MongoDB collections for interfaces that don't have them

const database = require('../config/database');
const tableManager = require('../services/InterfaceTableManager'); // Already a singleton instance

async function createMissingTables() {
    console.log('🔍 Checking for interfaces without storage tables...\n');

    try {
        // Connect to database
        if (!database.isConnected) {
            await database.connect();
        }

        // Get all active interfaces
        const interfaces = await database.sequelize.query(`
            SELECT id, name, status, created_at
            FROM interfaces
            WHERE is_active = true
            ORDER BY created_at
        `, {
            type: database.sequelize.QueryTypes.SELECT
        });

        console.log(`✅ Found ${interfaces.length} active interface(s)\n`);

        if (interfaces.length === 0) {
            console.log('ℹ️  No active interfaces found. Nothing to do.');
            process.exit(0);
        }
        let created = 0;
        let existing = 0;

        for (const intf of interfaces) {
            const tableName = tableManager.getInterfaceTableName(intf.id);

            console.log(`📋 Interface: ${intf.name} (${intf.id})`);
            console.log(`   Status: ${intf.status}`);
            console.log(`   Expected table: ${tableName}`);

            // Check if table exists
            const tableExists = await tableManager.checkTableExists(tableName);

            if (tableExists) {
                console.log(`   ✅ Table already exists\n`);
                existing++;
            } else {
                console.log(`   🏗️  Creating table...`);
                try {
                    await tableManager.ensureInterfaceTable(intf.id, intf.name);
                    console.log(`   ✅ Table created: ${tableName}\n`);
                    created++;
                } catch (error) {
                    console.error(`   ❌ Failed to create table:`, error.message);
                    console.error(`      ${error.stack}\n`);
                }
            }
        }

        console.log('\n' + '='.repeat(60));
        console.log('📊 Summary:');
        console.log(`   Total interfaces: ${interfaces.length}`);
        console.log(`   Tables existing: ${existing}`);
        console.log(`   Tables created: ${created}`);
        console.log('='.repeat(60));

        if (created > 0) {
            console.log('\n✅ All missing tables have been created!');
            console.log('ℹ️  MongoDB collections will be created automatically when first message arrives.');
        } else {
            console.log('\n✅ All interfaces already have their storage tables.');
        }

        process.exit(0);

    } catch (error) {
        console.error('\n❌ Error:', error);
        console.error(error.stack);
        process.exit(1);
    }
}

// Run the script
createMissingTables();
