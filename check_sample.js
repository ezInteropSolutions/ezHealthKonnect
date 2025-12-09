const { Pool } = require('pg');
const pool = new Pool({
    host: 'postgres',
    port: 5432,
    database: 'ezhealthkonnect',
    user: 'ezhealth_user',
    password: 'secure_password_change_me'
});

async function checkSample() {
    try {
        const result = await pool.query(`
            SELECT message_type, hl7_version, format, parsed_content
            FROM sample_parsed_messages
            WHERE is_active = TRUE
            LIMIT 1
        `);

        if (result.rows.length > 0) {
            const row = result.rows[0];
            console.log('✅ Sample exists:', row.message_type, row.hl7_version);

            const enhancedSegments = row.parsed_content.enhancedSegments;
            if (enhancedSegments && enhancedSegments.PID) {
                const pidFields = enhancedSegments.PID.fields || [];
                console.log(`   PID field count: ${pidFields.length}`);
                console.log('\nFirst 5 PID fields:');
                pidFields.slice(0, 5).forEach((field, i) => {
                    console.log(`${i+1}. ${field.key} - "${field.name || field.description}" (${field.dataType || 'N/A'})`);
                });
            }
        } else {
            console.log('❌ No sample messages found');
        }
    } catch (error) {
        console.error('Error:', error.message);
    } finally {
        await pool.end();
    }
}

checkSample();
