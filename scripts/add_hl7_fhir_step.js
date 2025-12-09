// Script to add HL7→FHIR mapping step to Test Interface 7 pipeline
const { Client } = require('pg');

const client = new Client({
    host: process.env.DB_HOST || 'localhost',
    port: process.env.DB_PORT || 5432,
    database: process.env.DB_NAME || 'ezhealthkonnect',
    user: process.env.DB_USER || 'ezhealth_user',
    password: process.env.DB_PASSWORD || 'ezhealthkonnect_password'
});

async function addHL7FHIRStep() {
    try {
        await client.connect();
        console.log('✅ Connected to database');

        const interfaceId = 'ad553ba7-69b4-4e24-a76e-9a749a1087a9';
        const messageType = 'ADT^A01';

        // 1. Check if wizard mappings exist
        const mappingsResult = await client.query(`
            SELECT id, name, message_type,
                   transformation_mapping,
                   transformation_mapping IS NOT NULL as has_mappings
            FROM interfaces
            WHERE id = $1
        `, [interfaceId]);

        if (mappingsResult.rows.length === 0) {
            console.error('❌ Interface not found');
            process.exit(1);
        }

        const interface_ = mappingsResult.rows[0];
        console.log(`📋 Interface: ${interface_.name}`);
        console.log(`📋 Has wizard mappings: ${interface_.has_mappings ? 'YES ✅' : 'NO ❌'}`);

        if (!interface_.has_mappings) {
            console.error('❌ No wizard mappings found. Please complete the wizard first.');
            process.exit(1);
        }

        const wizardMappings = interface_.transformation_mapping;
        const mappingCount = wizardMappings.atomicMappings ? wizardMappings.atomicMappings.length : 0;
        console.log(`📊 Wizard mappings count: ${mappingCount}`);

        // 2. Get or create pipeline
        let pipelineResult = await client.query(`
            SELECT id, pipeline_name
            FROM transformation_pipelines
            WHERE interface_id = $1 AND message_type = $2
        `, [interfaceId, messageType]);

        let pipelineId;
        if (pipelineResult.rows.length === 0) {
            // Create new pipeline
            const { v4: uuidv4 } = require('uuid');
            pipelineId = uuidv4();

            await client.query(`
                INSERT INTO transformation_pipelines
                    (id, interface_id, message_type, pipeline_name, enabled, version, created_at, updated_at)
                VALUES ($1, $2, $3, $4, true, 1, NOW(), NOW())
            `, [pipelineId, interfaceId, messageType, `${messageType} Pipeline`]);

            console.log(`✅ Created new pipeline: ${pipelineId}`);
        } else {
            pipelineId = pipelineResult.rows[0].id;
            console.log(`✅ Using existing pipeline: ${pipelineId}`);
        }

        // 3. Check if HL7→FHIR step already exists
        const existingStepResult = await client.query(`
            SELECT id, step_name, config
            FROM transformation_steps
            WHERE pipeline_id = $1 AND step_type = 'core.mapping'
        `, [pipelineId]);

        if (existingStepResult.rows.length > 0) {
            console.log('⚠️  HL7→FHIR step already exists. Updating...');

            const stepId = existingStepResult.rows[0].id;
            const stepConfig = {
                fhir_version: 'R4',
                use_template: true,
                interface_id: interfaceId,
                embedded_mappings: wizardMappings,
                _embedded_at: new Date().toISOString(),
                _mapping_version: wizardMappings.version || 1
            };

            await client.query(`
                UPDATE transformation_steps
                SET config = $1, updated_at = NOW()
                WHERE id = $2
            `, [JSON.stringify(stepConfig), stepId]);

            console.log(`✅ Updated HL7→FHIR step: ${stepId}`);
            console.log(`💾 Embedded ${mappingCount} wizard mappings`);
        } else {
            // Create new step
            const { v4: uuidv4 } = require('uuid');
            const stepId = uuidv4();

            const stepConfig = {
                fhir_version: 'R4',
                use_template: true,
                interface_id: interfaceId,
                embedded_mappings: wizardMappings,
                _embedded_at: new Date().toISOString(),
                _mapping_version: wizardMappings.version || 1
            };

            await client.query(`
                INSERT INTO transformation_steps
                    (id, pipeline_id, step_name, step_type, sequence, layer, required, timeout_ms, enabled, config, on_error_strategy, created_at, updated_at)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
            `, [
                stepId,
                pipelineId,
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
            console.log(`💾 Embedded ${mappingCount} wizard mappings`);
        }

        // 4. Verify
        const verifyResult = await client.query(`
            SELECT
                s.id,
                s.step_name,
                s.step_type,
                s.sequence,
                s.layer,
                CASE
                    WHEN s.config->'embedded_mappings' IS NOT NULL THEN 'YES ✅'
                    ELSE 'NO ❌'
                END as has_embedded_mappings,
                jsonb_array_length(s.config->'embedded_mappings'->'atomicMappings') as embedded_count
            FROM transformation_steps s
            WHERE s.pipeline_id = $1
            ORDER BY s.sequence
        `, [pipelineId]);

        console.log('\n📊 Pipeline Steps:');
        console.table(verifyResult.rows);

        console.log('\n✅ SUCCESS! HL7→FHIR step linked to Test Interface 7');
        console.log('\n🔗 Open pipeline builder:');
        console.log(`   http://localhost:3000/pipeline-builder.html?interfaceId=${interfaceId}&messageType=${encodeURIComponent(messageType)}`);

    } catch (error) {
        console.error('❌ Error:', error.message);
        console.error(error.stack);
        process.exit(1);
    } finally {
        await client.end();
    }
}

addHL7FHIRStep();
