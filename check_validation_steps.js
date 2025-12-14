const { Pool } = require('pg');

const pool = new Pool({
    host: 'localhost',
    port: 5432,
    database: 'ezhealthkonnect',
    user: 'ezhealth_user',
    password: 'secure_password_change_me'
});

async function checkValidationSteps() {
    try {
        const result = await pool.query(`
            SELECT pipeline_id, step_name, step_type, sequence, config
            FROM transformation_steps
            WHERE step_type = 'pre.validation'
            ORDER BY sequence
            LIMIT 5
        `);

        console.log('Validation Steps:', JSON.stringify(result.rows, null, 2));

        // Also check all steps
        const allSteps = await pool.query(`
            SELECT pipeline_id, step_name, step_type, sequence
            FROM transformation_steps
            ORDER BY pipeline_id, sequence
            LIMIT 10
        `);

        console.log('\nAll Steps:', JSON.stringify(allSteps.rows, null, 2));
    } catch (error) {
        console.error('Error:', error.message);
    } finally {
        await pool.end();
    }
}

checkValidationSteps();
