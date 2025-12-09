/**
 * Load Sample Parsed Messages
 * Reads parsedhl7.json and inserts it into sample_parsed_messages table
 */

const fs = require('fs');
const path = require('path');
const { Pool } = require('pg');

async function loadSamples() {
    const pool = new Pool({
        host: process.env.DB_HOST || 'localhost',
        port: process.env.DB_PORT || 5432,
        database: process.env.DB_NAME || 'ezhealthkonnect',
        user: process.env.DB_USER || 'postgres',
        password: process.env.DB_PASSWORD || 'postgres'
    });

    try {
        console.log('📂 Reading parsedhl7.json...');

        // Read the parsed HL7 file
        const filePath = path.join(__dirname, '..', 'parsedhl7.json');
        const fileContent = fs.readFileSync(filePath, 'utf8');
        const outerData = JSON.parse(fileContent);

        // The ParsedHL7Data field contains a JSON string, parse it again
        const innerData = typeof outerData.ParsedHL7Data === 'string'
            ? JSON.parse(outerData.ParsedHL7Data)
            : outerData.ParsedHL7Data;

        if (!innerData.data || !innerData.data.enhancedSegments) {
            throw new Error('Invalid parsed data structure');
        }

        const messageType = innerData.data.messageType.name; // "ADT^A04"
        const version = innerData.data.version; // "2.3"
        const enhancedSegments = innerData.data.enhancedSegments;

        console.log(`✅ Found: ${messageType} v${version}`);
        console.log(`   Segments: ${Object.keys(enhancedSegments).join(', ')}`);

        // Insert into database
        const query = `
            INSERT INTO sample_parsed_messages
                (message_type, hl7_version, format, parsed_content, description, is_active)
            VALUES ($1, $2, $3, $4, $5, TRUE)
            ON CONFLICT (message_type, hl7_version, format, is_active)
            DO UPDATE SET
                parsed_content = EXCLUDED.parsed_content,
                description = EXCLUDED.description,
                updated_at = CURRENT_TIMESTAMP
            RETURNING id
        `;

        const parsedContent = {
            enhancedSegments: enhancedSegments
        };

        const result = await pool.query(query, [
            messageType,
            version,
            'hl7v2',
            JSON.stringify(parsedContent),
            `Sample ${messageType} message for XPath autocomplete (Patient: Mickey Mouse)`
        ]);

        console.log(`✅ Sample inserted/updated successfully (ID: ${result.rows[0].id})`);
        console.log(`\nYou can now use XPath autocomplete for: ${messageType} v${version}`);

    } catch (error) {
        console.error('❌ Error loading samples:', error.message);
        process.exit(1);
    } finally {
        await pool.end();
    }
}

loadSamples();
