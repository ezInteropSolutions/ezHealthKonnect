#!/usr/bin/env node
// load_oob_templates.js
// Loads real schema-based OOB templates into the V9 hl7_fhir_templates table

const fs = require('fs');
const path = require('path');
const { Pool } = require('pg');

// Database configuration
const dbConfig = {
    host: process.env.DB_HOST || 'localhost',
    port: process.env.DB_PORT || 5432,
    database: process.env.DB_NAME || 'ezhealthkonnect',
    user: process.env.DB_USER || 'postgres',
    password: process.env.DB_PASSWORD || 'password'
};

console.log('🚀 Loading OOB Templates into V9 Architecture...');
console.log('Database:', `${dbConfig.host}:${dbConfig.port}/${dbConfig.database}`);

async function loadOOBTemplates() {
    const pool = new Pool(dbConfig);

    try {
        // Test database connection
        console.log('🔌 Testing database connection...');
        await pool.query('SELECT NOW()');
        console.log('✅ Database connection successful');

        // Check if V9 tables exist
        console.log('🔍 Checking V9 table structure...');
        const tableCheck = await pool.query(`
            SELECT table_name FROM information_schema.tables
            WHERE table_name = 'hl7_fhir_templates'
        `);

        if (tableCheck.rows.length === 0) {
            throw new Error('❌ V9 migration not applied! Please run V9__Message_Type_Centric_Mapping.sql first');
        }
        console.log('✅ V9 hl7_fhir_templates table found');

        // Check current table structure
        console.log('🔍 Checking table structure...');
        const columnCheck = await pool.query(`
            SELECT column_name, data_type
            FROM information_schema.columns
            WHERE table_name = 'hl7_fhir_templates'
            ORDER BY ordinal_position
        `);

        console.log('📋 Available columns:', columnCheck.rows.map(r => r.column_name).join(', '));

        // Clear existing templates (use template_description to identify OOB templates)
        console.log('🧹 Clearing existing OOB templates...');
        await pool.query("DELETE FROM hl7_fhir_templates WHERE template_description LIKE '%Schema-based%'");
        console.log('✅ Existing OOB templates cleared');

        // Load template files
        const templates = [
            {
                file: 'templates/REAL_ADT^A01_v2.5.1.json',
                messageType: 'ADT^A01',
                hl7Version: '2.5.1',
                templateName: 'OOB_ADT_A01_REAL',
                templateDescription: 'Schema-based Patient Admission Mapping - 34 fields, 99% confidence core demographics'
            },
            {
                file: 'templates/REAL_ORU^R01_v2.5.1.json',
                messageType: 'ORU^R01',
                hl7Version: '2.5.1',
                templateName: 'OOB_ORU_R01_REAL',
                templateDescription: 'Schema-based Lab Results Mapping - 42 fields, comprehensive observation coverage'
            }
        ];

        // Insert each template
        for (const template of templates) {
            console.log(`📋 Loading template: ${template.messageType}...`);

            // Read template file
            const templatePath = path.join(__dirname, template.file);
            if (!fs.existsSync(templatePath)) {
                console.error(`⚠️ Template file not found: ${templatePath}`);
                continue;
            }

            const templateData = fs.readFileSync(templatePath, 'utf8');
            let parsedTemplate;

            try {
                parsedTemplate = JSON.parse(templateData);
                console.log(`   ✅ Template parsed successfully (${templateData.length} bytes)`);
            } catch (parseError) {
                console.error(`   ❌ Failed to parse template JSON: ${parseError.message}`);
                continue;
            }

            // Insert into database using actual column names
            const insertQuery = `
                INSERT INTO hl7_fhir_templates (
                    message_type,
                    hl7_version,
                    template_name,
                    template_description,
                    template_config,
                    is_default,
                    created_at,
                    updated_at
                ) VALUES ($1, $2, $3, $4, $5, true, NOW(), NOW())
                RETURNING id
            `;

            const result = await pool.query(insertQuery, [
                template.messageType,
                template.hl7Version,
                template.templateName,
                template.templateDescription,
                templateData // Store as raw JSON string
            ]);

            console.log(`   ✅ Inserted with ID: ${result.rows[0].id}`);
            console.log(`   📊 Template: ${template.templateName}`);
        }

        // Verify results
        console.log('\n📈 Verifying loaded templates...');
        const verifyQuery = `
            SELECT
                message_type,
                hl7_version,
                template_name,
                template_description,
                is_default,
                LENGTH(template_config::text) as config_size_bytes,
                created_at
            FROM hl7_fhir_templates
            WHERE template_description LIKE '%Schema-based%'
            ORDER BY message_type
        `;

        const verification = await pool.query(verifyQuery);

        console.log('\n🎯 Loaded OOB Templates:');
        console.log('+-----------------+--------+-------------------------+------------+----------+');
        console.log('| Message Type    | Ver    | Template Name           | Size (KB)  | Default  |');
        console.log('+-----------------+--------+-------------------------+------------+----------+');

        verification.rows.forEach(row => {
            console.log(
                `| ${row.message_type.padEnd(15)} | ${row.hl7_version.padEnd(6)} | ${row.template_name.padEnd(23)} | ${(row.config_size_bytes / 1024).toFixed(1).padStart(10)} | ${row.is_default ? 'Yes'.padStart(8) : 'No'.padStart(8)} |`
            );
        });
        console.log('+-----------------+--------+-------------------------+------------+----------+');

        // Show summary
        const summary = await pool.query(`
            SELECT 'OOB Templates Loaded' as status, COUNT(*) as count
            FROM hl7_fhir_templates
            WHERE template_description LIKE '%Schema-based%'
        `);

        console.log(`\n🎉 SUCCESS: ${summary.rows[0].count} OOB templates loaded successfully!`);
        console.log('\n🔥 The wizard step 4 should now work with our new OOB architecture!');
        console.log('\n💡 Next steps:');
        console.log('   1. Test wizard step 4 HL7-FHIR transformation');
        console.log('   2. Verify ADT^A01 and ORU^R01 mappings are working');
        console.log('   3. Add more message types (ORM, MDM, SIU, DFT, RDE)');

    } catch (error) {
        console.error('❌ Error loading OOB templates:', error.message);
        process.exit(1);
    } finally {
        await pool.end();
        console.log('\n🔌 Database connection closed');
    }
}

// Run the script
if (require.main === module) {
    loadOOBTemplates().catch(error => {
        console.error('Fatal error:', error);
        process.exit(1);
    });
}

module.exports = { loadOOBTemplates };