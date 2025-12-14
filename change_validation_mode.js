/**
 * Script to change validation mode for a pipeline
 *
 * Usage:
 *   node change_validation_mode.js <pipeline_id> <mode>
 *
 * Modes:
 *   - strict_reject: Send NACK on validation failure, stop processing
 *   - accept_and_flag: Send ACK with warnings, continue processing (DEFAULT)
 *   - no_validation: Skip all validation
 *
 * Examples:
 *   node change_validation_mode.js 4b3ffa85-2d66-413d-a058-f37ce9c595cb strict_reject
 *   node change_validation_mode.js 4b3ffa85-2d66-413d-a058-f37ce9c595cb accept_and_flag
 */

const { Pool } = require('pg');

const pool = new Pool({
    host: 'localhost',
    port: 5432,
    database: 'ezhealthkonnect',
    user: 'ezhealth_user',
    password: 'secure_password_change_me'
});

const validModes = ['strict_reject', 'accept_and_flag', 'no_validation'];

async function changeValidationMode(pipelineId, newMode) {
    if (!validModes.includes(newMode)) {
        console.error(`❌ Invalid mode: ${newMode}`);
        console.error(`   Valid modes: ${validModes.join(', ')}`);
        process.exit(1);
    }

    try {
        // First, check if the pipeline has a validation step
        const checkQuery = `
            SELECT id, step_name, config
            FROM transformation_steps
            WHERE pipeline_id = $1 AND step_type = 'pre.validation'
        `;
        const checkResult = await pool.query(checkQuery, [pipelineId]);

        if (checkResult.rows.length === 0) {
            console.error(`❌ No validation step found for pipeline: ${pipelineId}`);
            await pool.end();
            process.exit(1);
        }

        const step = checkResult.rows[0];
        console.log(`\n📋 Current Configuration:`);
        console.log(`   Step Name: ${step.step_name}`);
        console.log(`   Current Mode: ${step.config.validation_mode || 'accept_and_flag (default)'}`);
        console.log(`   Rules Count: ${step.config.rules?.length || 0}`);

        // Update the validation mode
        const updateQuery = `
            UPDATE transformation_steps
            SET config = jsonb_set(
                config,
                '{validation_mode}',
                $1::jsonb
            ),
            updated_at = CURRENT_TIMESTAMP
            WHERE id = $2
            RETURNING config
        `;

        const updateResult = await pool.query(updateQuery, [
            JSON.stringify(newMode),
            step.id
        ]);

        const updatedConfig = updateResult.rows[0].config;

        console.log(`\n✅ Validation mode updated successfully!`);
        console.log(`   New Mode: ${updatedConfig.validation_mode}`);

        // Show mode explanation
        console.log(`\n📖 Mode Explanation:`);
        switch (newMode) {
            case 'strict_reject':
                console.log('   ❌ NACK will be sent on validation failure');
                console.log('   🛑 Pipeline processing will STOP');
                console.log('   📊 Message status: "rejected"');
                console.log('   💡 Use for: Critical interfaces requiring perfect data quality');
                break;
            case 'accept_and_flag':
                console.log('   ✅ ACK will be sent even on validation failure');
                console.log('   ▶️  Pipeline processing will CONTINUE');
                console.log('   ⚠️  Validation warnings will be logged');
                console.log('   📊 Message status: "warning"');
                console.log('   💡 Use for: Data quality monitoring, non-critical interfaces');
                break;
            case 'no_validation':
                console.log('   ⏭️  All validation will be SKIPPED');
                console.log('   ✅ ACK will be sent immediately');
                console.log('   💡 Use for: Emergency bypass, debugging');
                break;
        }

        console.log(`\n🧪 Test the new configuration:`);
        console.log(`   node test_validation_pipeline.js`);
        console.log(`   # or`);
        console.log(`   curl -X POST http://localhost:8080/api/fhir/pipeline/test \\`);
        console.log(`     -H "Content-Type: application/json" \\`);
        console.log(`     -d '{"pipeline_id": "${pipelineId}", "test_message": "MSH|..."}'`);

    } catch (error) {
        console.error('❌ Error:', error.message);
        process.exit(1);
    } finally {
        await pool.end();
    }
}

async function listPipelinesWithValidation() {
    try {
        const query = `
            SELECT
                tp.id,
                tp.pipeline_name,
                ts.step_name,
                ts.config->>'validation_mode' as validation_mode,
                jsonb_array_length(ts.config->'rules') as rules_count
            FROM transformation_pipelines tp
            JOIN transformation_steps ts ON ts.pipeline_id = tp.id
            WHERE ts.step_type = 'pre.validation'
            ORDER BY tp.created_at DESC
        `;

        const result = await pool.query(query);

        if (result.rows.length === 0) {
            console.log('❌ No pipelines with validation steps found');
            return;
        }

        console.log('\n📋 Pipelines with Validation Steps:\n');
        console.log('─'.repeat(120));
        console.log(
            'Pipeline ID'.padEnd(40) +
            'Pipeline Name'.padEnd(30) +
            'Mode'.padEnd(20) +
            'Rules'
        );
        console.log('─'.repeat(120));

        result.rows.forEach(row => {
            const mode = row.validation_mode || 'accept_and_flag';
            const modeIcon = mode === 'strict_reject' ? '❌' : mode === 'no_validation' ? '⏭️' : '⚠️';

            console.log(
                row.id.padEnd(40) +
                (row.pipeline_name || 'Unnamed').padEnd(30) +
                `${modeIcon} ${mode}`.padEnd(20) +
                row.rules_count
            );
        });
        console.log('─'.repeat(120));

    } catch (error) {
        console.error('Error:', error.message);
    } finally {
        await pool.end();
    }
}

// Main execution
const args = process.argv.slice(2);

if (args.length === 0 || args[0] === '--list' || args[0] === '-l') {
    listPipelinesWithValidation();
} else if (args.length < 2) {
    console.error('Usage: node change_validation_mode.js <pipeline_id> <mode>');
    console.error('       node change_validation_mode.js --list');
    console.error('');
    console.error('Modes: strict_reject, accept_and_flag, no_validation');
    process.exit(1);
} else {
    const [pipelineId, newMode] = args;
    changeValidationMode(pipelineId, newMode);
}
