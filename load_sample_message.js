const fs = require('fs');
const path = require('path');
const { Pool } = require('pg');

// Database configuration
const pool = new Pool({
    host: process.env.DB_HOST || 'localhost',
    port: process.env.DB_PORT || 5432,
    database: process.env.DB_NAME || 'ezhealthkonnect',
    user: process.env.DB_USER || 'ezhealth_user',
    password: process.env.DB_PASSWORD || 'secure_password_change_me'
});

async function loadSampleMessage() {
    try {
        // Read the parsedhl7.json file
        const filePath = path.join(__dirname, 'parsedhl7.json');
        console.log('📖 Reading file:', filePath);

        let fileContent = fs.readFileSync(filePath, 'utf8');

        // Fix curly quotes to regular quotes
        fileContent = fileContent.replace(/'/g, "'");

        const jsonData = JSON.parse(fileContent);

        // The actual parsed data is in ParsedHL7Data as a string
        const parsedDataString = jsonData.ParsedHL7Data;
        const parsedData = JSON.parse(parsedDataString);

        console.log('✅ Parsed JSON successfully');
        console.log('   Message Type:', parsedData.messageType?.name);
        console.log('   Version:', parsedData.version);

        // Extract the enhancedSegments
        const enhancedSegments = parsedData.enhancedSegments;

        if (!enhancedSegments) {
            throw new Error('No enhancedSegments found in parsed data');
        }

        console.log('   Segments:', Object.keys(enhancedSegments).join(', '));

        // Check if record already exists
        const checkQuery = `
            SELECT id FROM sample_parsed_messages
            WHERE message_type = $1 AND hl7_version = $2
        `;

        const checkResult = await pool.query(checkQuery, [
            parsedData.messageType?.name || 'ADT^A04',
            parsedData.version || '2.3'
        ]);

        if (checkResult.rows.length > 0) {
            console.log('⚠️  Sample message already exists, updating...');

            const updateQuery = `
                UPDATE sample_parsed_messages
                SET parsed_content = $1,
                    description = $2,
                    updated_at = CURRENT_TIMESTAMP
                WHERE id = $3
            `;

            await pool.query(updateQuery, [
                enhancedSegments,
                'Mickey Mouse sample message (ADT^A04) - Auto-loaded from parsedhl7.json',
                checkResult.rows[0].id
            ]);

            console.log('✅ Updated existing sample message (ID:', checkResult.rows[0].id, ')');
        } else {
            console.log('📝 Inserting new sample message...');

            const insertQuery = `
                INSERT INTO sample_parsed_messages
                (message_type, hl7_version, format, parsed_content, description, is_active)
                VALUES ($1, $2, $3, $4, $5, $6)
                RETURNING id
            `;

            const result = await pool.query(insertQuery, [
                parsedData.messageType?.name || 'ADT^A04',
                parsedData.version || '2.3',
                'hl7v2',
                enhancedSegments,
                'Mickey Mouse sample message (ADT^A04) - Auto-loaded from parsedhl7.json',
                true
            ]);

            console.log('✅ Inserted sample message (ID:', result.rows[0].id, ')');
        }

        // Test the field extraction
        console.log('\n🧪 Testing field extraction...');
        const testQuery = `
            SELECT
                jsonb_path_query(parsed_content, '$.*.fields[*].key') as field_key,
                jsonb_path_query(parsed_content, '$.*.fields[*].name') as field_name
            FROM sample_parsed_messages
            WHERE message_type = $1
            LIMIT 10
        `;

        const testResult = await pool.query(testQuery, [parsedData.messageType?.name || 'ADT^A04']);
        console.log('   Found', testResult.rows.length, 'field entries');

        console.log('\n✅ Sample message loaded successfully!');
        console.log('   Now try searching for "version" in the autocomplete field');

    } catch (error) {
        console.error('❌ Error loading sample message:', error.message);
        console.error(error.stack);
    } finally {
        await pool.end();
    }
}

// Run the script
loadSampleMessage();
