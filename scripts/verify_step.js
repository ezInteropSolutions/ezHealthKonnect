const { Client } = require('pg');
const client = new Client({
    host: 'postgres',
    port: 5432,
    database: 'ezhealthkonnect',
    user: 'ezhealth_user',
    password: 'ezhealthkonnect_password'
});

(async () => {
    await client.connect();
    const result = await client.query(`
        SELECT
            s.step_name,
            s.config->>'fhir_version' as fhir_version,
            s.config->>'interface_id' as interface_id,
            CASE WHEN s.config->'embedded_mappings' IS NOT NULL THEN 'YES' ELSE 'NO' END as has_embedded
        FROM transformation_steps s
        WHERE s.pipeline_id = '1688b4ee-ee3d-4706-8683-f51c4b64b14a'
    `);
    console.log('✅ Step configuration:');
    console.table(result.rows);
    await client.end();
})();
