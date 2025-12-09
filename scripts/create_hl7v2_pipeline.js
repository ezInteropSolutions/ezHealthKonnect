// Create pipeline for messageType=hl7v2
const { Client } = require('pg');

const client = new Client({
    host: process.env.DB_HOST || 'localhost',
    port: process.env.DB_PORT || 5432,
    database: process.env.DB_NAME || 'ezhealthkonnect',
    user: process.env.DB_USER || 'ezhealth_user',
    password: process.env.DB_PASSWORD || 'ezhealthkonnect_password'
});

async function createPipeline() {
    try {
        await client.connect();
        console.log('✅ Connected to database');

        const interfaceId = 'ad553ba7-69b4-4e24-a76e-9a749a1087a9';
        const messageType = 'hl7v2'; // Using the message type from your URL

        // 1. Load wizard mappings
        const mappingsResult = await client.query(`
            SELECT id, name, transformation_mapping
            FROM interfaces
            WHERE id = $1
        `, [interfaceId]);

        if (mappingsResult.rows.length === 0) {
            console.error('❌ Interface not found');
            process.exit(1);
        }

        const interface_ = mappingsResult.rows[0];
        const wizardMappings = interface_.transformation_mapping;

        console.log(`📋 Interface: ${interface_.name}`);
        console.log(`📋 Has wizard mappings: ${wizardMappings ? 'YES ✅' : 'NO ❌'}`);

        // 2. Create or update pipeline
        const { v4: uuidv4 } = require('uuid');
        const pipelineId = uuidv4();

        await client.query(`
            INSERT INTO transformation_pipelines
                (id, interface_id, message_type, pipeline_name, enabled, version, created_at, updated_at)
            VALUES ($1, $2, $3, $4, true, 1, NOW(), NOW())
            ON CONFLICT (interface_id, message_type)
            DO UPDATE SET
                pipeline_name = EXCLUDED.pipeline_name,
                updated_at = NOW()
            RETURNING id::text
        `, [pipelineId, interfaceId, messageType, `${messageType} Pipeline`]);

        const finalPipelineResult = await client.query(`
            SELECT id FROM transformation_pipelines
            WHERE interface_id = $1 AND message_type = $2
        `, [interfaceId, messageType]);

        const finalPipelineId = finalPipelineResult.rows[0].id;
        console.log(`✅ Pipeline ID: ${finalPipelineId}`);

        // 3. Delete old steps
        await client.query(`
            DELETE FROM transformation_steps WHERE pipeline_id = $1
        `, [finalPipelineId]);

        // 4. Create HL7→FHIR step with embedded mappings
        const stepId = uuidv4();
        const stepConfig = {
            fhir_version: 'R4',
            use_template: true,
            interface_id: interfaceId,
            embedded_mappings: wizardMappings || {},
            _embedded_at: new Date().toISOString(),
            _mapping_version: (wizardMappings && wizardMappings.version) || 1
        };

        await client.query(`
            INSERT INTO transformation_steps
                (id, pipeline_id, step_name, step_type, sequence, layer, required, timeout_ms, enabled, config, on_error_strategy, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
        `, [
            stepId,
            finalPipelineId,
            'HL7 to FHIR Mapping',
            'core.mapping',
            100,
            'core',
            true,
            30000,
            true,
            JSON.stringify(stepConfig),
            'fail'
        ]);

        console.log(`✅ Created HL7→FHIR step: ${stepId}`);

        // 5. Verify
        const verifyResult = await client.query(`
            SELECT
                p.id as pipeline_id,
                p.message_type,
                s.id as step_id,
                s.step_name,
                s.step_type,
                CASE WHEN s.config->'embedded_mappings' IS NOT NULL THEN 'YES' ELSE 'NO' END as has_embedded
            FROM transformation_pipelines p
            LEFT JOIN transformation_steps s ON s.pipeline_id = p.id
            WHERE p.interface_id = $1 AND p.message_type = $2
        `, [interfaceId, messageType]);

        console.log('\n📊 Pipeline created:');
        console.table(verifyResult.rows);

        console.log('\n✅ SUCCESS! Pipeline created for messageType=hl7v2');
        console.log('\n🔗 Open pipeline builder:');
        console.log(`   http://localhost:3000/pipeline-builder.html?interfaceId=${interfaceId}&messageType=${messageType}`);

    } catch (error) {
        console.error('❌ Error:', error.message);
        console.error(error.stack);
        process.exit(1);
    } finally {
        await client.end();
    }
}

createPipeline();
