#!/usr/bin/env node
// check_templates.js
// Check if OOB templates are properly loaded

const { Pool } = require('pg');

const dbConfig = {
    host: process.env.DB_HOST || 'localhost',
    port: process.env.DB_PORT || 5432,
    database: process.env.DB_NAME || 'ezhealthkonnect',
    user: process.env.DB_USER || 'ezhealth_user',
    password: process.env.DB_PASSWORD || 'secure_password_change_me'
};

async function checkTemplates() {
    const pool = new Pool(dbConfig);

    try {
        console.log('🔍 Checking OOB templates in database...');

        // Check table structure
        const columnsResult = await pool.query(`
            SELECT column_name, data_type
            FROM information_schema.columns
            WHERE table_name = 'hl7_fhir_templates'
            ORDER BY ordinal_position
        `);

        console.log('\n📋 Table structure:');
        columnsResult.rows.forEach(row => {
            console.log(`  ${row.column_name}: ${row.data_type}`);
        });

        // Check all templates
        const templatesResult = await pool.query(`
            SELECT message_type, template_name, is_default,
                   LENGTH(template_config::text) as config_size,
                   created_at
            FROM hl7_fhir_templates
            ORDER BY message_type
        `);

        console.log('\n🎯 All templates:');
        if (templatesResult.rows.length === 0) {
            console.log('  ❌ No templates found!');
        } else {
            templatesResult.rows.forEach(row => {
                console.log(`  ${row.message_type}: ${row.template_name} (default: ${row.is_default}, size: ${row.config_size})`);
            });
        }

        // Check ADT^A01 specifically
        const adtResult = await pool.query(`
            SELECT message_type, template_name, template_description, is_default,
                   LENGTH(template_config::text) as config_size,
                   template_config::text LIKE '%resources%' as has_resources
            FROM hl7_fhir_templates
            WHERE message_type = $1
        `, ['ADT^A01']);

        console.log('\n🎯 ADT^A01 template check:');
        if (adtResult.rows.length === 0) {
            console.log('  ❌ ADT^A01 template not found!');
        } else {
            const template = adtResult.rows[0];
            console.log(`  ✅ Found: ${template.template_name}`);
            console.log(`     - Description: ${template.template_description}`);
            console.log(`     - Is default: ${template.is_default}`);
            console.log(`     - Config size: ${template.config_size} bytes`);
            console.log(`     - Has resources: ${template.has_resources}`);

            // Sample the template config
            if (template.config_size > 0) {
                const configResult = await pool.query(`
                    SELECT LEFT(template_config::text, 500) as config_sample
                    FROM hl7_fhir_templates
                    WHERE message_type = $1
                `, ['ADT^A01']);

                console.log('\n📋 Template config sample (first 500 chars):');
                console.log(configResult.rows[0].config_sample);
            }
        }

        // Test the exact query our V3 service uses
        console.log('\n🔧 Testing V3 service query...');
        const v3QueryResult = await pool.query(`
            SELECT template_config, template_description, template_name
            FROM hl7_fhir_templates
            WHERE message_type = $1 AND is_default = true
            ORDER BY created_at DESC
            LIMIT 1
        `, ['ADT^A01']);

        if (v3QueryResult.rows.length === 0) {
            console.log('  ❌ V3 query returned no results!');
            console.log('  🔍 This explains why 0 field mappings were found');
        } else {
            console.log('  ✅ V3 query returned 1 template');
            const template = v3QueryResult.rows[0];
            console.log(`     - Template: ${template.template_name}`);
            console.log(`     - Description: ${template.template_description}`);
            console.log(`     - Config size: ${template.template_config.length} bytes`);
        }

    } catch (error) {
        console.error('❌ Error:', error.message);
    } finally {
        await pool.end();
    }
}

// Run the check
if (require.main === module) {
    checkTemplates().catch(error => {
        console.error('Fatal error:', error);
        process.exit(1);
    });
}

module.exports = { checkTemplates };