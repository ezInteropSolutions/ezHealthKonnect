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

        // Extract just the nested JSON from the ParsedHL7Data field
        // Find the start of the actual JSON (after "ParsedHL7Data":")
        const startMarker = '"ParsedHL7Data":"';
        const startIndex = fileContent.indexOf(startMarker) + startMarker.length;

        // Find the end (before the last "},)
        const endMarker = '"},';
        const endIndex = fileContent.lastIndexOf(endMarker);

        if (startIndex === -1 || endIndex === -1) {
            throw new Error('Could not find ParsedHL7Data boundaries');
        }

        // Extract and unescape the JSON string
        let jsonString = fileContent.substring(startIndex, endIndex + 1);

        // Unescape common JSON escape sequences
        jsonString = jsonString.replace(/\\r\\n/g, '\\n');
        jsonString = jsonString.replace(/\\"/g, '"');

        console.log('📝 Extracted JSON string (first 200 chars):', jsonString.substring(0, 200));

        const parsedData = JSON.parse(jsonString);

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
                'Mickey Mouse sample message (ADT^A04 v2.3) - Auto-loaded from parsedhl7.json',
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
                'Mickey Mouse sample message (ADT^A04 v2.3) - Auto-loaded from parsedhl7.json',
                true
            ]);

            console.log('✅ Inserted sample message (ID:', result.rows[0].id, ')');
        }

        console.log('\n✅ Sample message loaded successfully!');
        console.log('   Now refresh your pipeline builder page and try searching for "version" in the autocomplete field');

    } catch (error) {
        console.error('❌ Error loading sample message:', error.message);
        if (error.stack) {
            console.error(error.stack);
        }
    } finally {
        await pool.end();
    }
}

// Run the script
loadSampleMessage();
