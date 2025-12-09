// Fix duplicate HL7→FHIR steps
const { Client } = require('pg');

const client = new Client({
    host: process.env.DB_HOST || 'localhost',
    port: process.env.DB_PORT || 5432,
    database: process.env.DB_NAME || 'ezhealthkonnect',
    user: process.env.DB_USER || 'ezhealth_user',
    password: process.env.DB_PASSWORD || 'ezhealthkonnect_password'
});

async function fixDuplicates() {
    try {
        await client.connect();
        console.log('✅ Connected');

        const pipelineId = '1688b4ee-ee3d-4706-8683-f51c4b64b14a';

        // Delete old step (type = 'hl7_to_fhir_mapping' without embedded mappings)
        const deleteResult = await client.query(`
            DELETE FROM transformation_steps
            WHERE pipeline_id = $1
              AND step_type = 'hl7_to_fhir_mapping'
              AND config->'embedded_mappings' IS NULL
            RETURNING id, step_name
        `, [pipelineId]);

        console.log(`✅ Deleted ${deleteResult.rowCount} old step(s)`);

        // Verify remaining steps
        const verifyResult = await client.query(`
            SELECT
                step_name,
                step_type,
                sequence,
                layer,
                CASE
                    WHEN config->'embedded_mappings' IS NOT NULL THEN 'YES'
                    ELSE 'NO'
                END as has_embedded
            FROM transformation_steps
            WHERE pipeline_id = $1
            ORDER BY sequence
        `, [pipelineId]);

        console.log('\n📊 Remaining steps:');
        console.table(verifyResult.rows);

    } catch (error) {
        console.error('❌ Error:', error.message);
    } finally {
        await client.end();
    }
}

fixDuplicates();
