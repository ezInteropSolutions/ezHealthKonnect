const fs = require('fs');
const { Pool } = require('pg');

const pool = new Pool({
    host: process.env.DB_HOST || 'localhost',
    port: process.env.DB_PORT || 5432,
    database: process.env.DB_NAME || 'ezhealthkonnect',
    user: process.env.DB_USER || 'ezhealth_user',
    password: process.env.DB_PASSWORD || 'secure_password_change_me'
});

async function extractAndLoad() {
    try {
        console.log('📖 Reading parsedhl7.json...');
        const data = fs.readFileSync('parsedhl7.json', 'utf8');

        // Extract the JSON string between the quotes
        const start = data.indexOf('"ParsedHL7Data":"') + 17;
        const end = data.lastIndexOf('"},');

        if (start === -1 || end === -1) {
            throw new Error('Could not find ParsedHL7Data in file');
        }

        const jsonStr = data.substring(start, end + 1);

        console.log('🔍 Parsing extracted JSON...');
        const parsed = JSON.parse(jsonStr);

        console.log('✅ Successfully parsed!');
        console.log('   Message Type:', parsed.messageType?.name);
        console.log('   Version:', parsed.version);
        console.log('   Segments:', Object.keys(parsed.enhancedSegments || {}).join(', '));

        if (!parsed.enhancedSegments) {
            throw new Error('No enhancedSegments found');
        }

        // Delete existing sample for this message type
        await pool.query('DELETE FROM sample_parsed_messages WHERE message_type = $1 AND hl7_version = $2',
            [parsed.messageType?.name || 'ADT^A04', parsed.version || '2.3']);

        console.log('📝 Inserting into sample_parsed_messages...');

        const result = await pool.query(`
            INSERT INTO sample_parsed_messages
            (message_type, hl7_version, format, parsed_content, description, is_active)
            VALUES ($1, $2, $3, $4, $5, $6)
            RETURNING id
        `, [
            parsed.messageType?.name || 'ADT^A04',
            parsed.version || '2.3',
            'hl7v2',
            parsed.enhancedSegments,
            'Mickey Mouse sample (ADT^A04 v2.3) - Loaded from parsedhl7.json',
            true
        ]);

        console.log('✅ Loaded successfully! ID:', result.rows[0].id);
        console.log('\n🎉 Now refresh your pipeline builder and try searching for "version" in autocomplete!');

    } catch (error) {
        console.error('❌ Error:', error.message);
        console.error(error.stack);
    } finally {
        await pool.end();
    }
}

extractAndLoad();
