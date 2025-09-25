const { Client } = require('pg');

async function setupFHIRFlow() {
    const client = new Client({
        host: 'localhost',
        port: 5432,
        database: 'ezhealthkonnect',
        user: 'ezhealth_user',
        password: 'ezhealth123'
    });

    try {
        await client.connect();
        
        // Get current interfaces
        console.log('🔍 Current interfaces:');
        const result = await client.query('SELECT id, name, source_type, target_type, status FROM interfaces ORDER BY created_at');
        
        result.rows.forEach(row => {
            console.log(`  - ${row.name} (${row.id}): ${row.source_type} → ${row.target_type} [${row.status}]`);
        });
        
        if (result.rows.length >= 2) {
            const testInterface = result.rows[0];  // Assuming first is Test Interface 1
            const fhirReceiver = result.rows[1];   // Assuming second is FHIR Receiver
            
            console.log('\n🔧 Configuring Test Interface 1 to send to FHIR Receiver...');
            
            // Configure Test Interface 1 for HL7→FHIR routing
            await client.query(`
                UPDATE interfaces 
                SET 
                    source_type = 'tcp',
                    source_connectivity = 'inbound',
                    target_type = 'fhir',
                    target_connectivity = 'outbound',
                    message_type = 'HL7',
                    processing_rules = $1,
                    target_config = $2,
                    status = 'configured',
                    updated_at = CURRENT_TIMESTAMP
                WHERE id = $3
            `, [
                JSON.stringify({
                    routingMode: 'hl7-to-fhir',
                    targetFhirInterface: fhirReceiver.id,
                    transformationEngine: 'go-engine',
                    retryPolicy: '3'
                }),
                JSON.stringify({
                    host: 'localhost',
                    port: 8080,
                    path: '/fhir/Patient',
                    targetInterfaceId: fhirReceiver.id
                }),
                testInterface.id
            ]);
            
            console.log('✅ Test Interface 1 configured for HL7→FHIR routing');
            
            // Configure FHIR Receiver for message storage
            await client.query(`
                UPDATE interfaces 
                SET 
                    source_type = 'http',
                    source_connectivity = 'inbound',
                    target_type = 'database',
                    target_connectivity = 'outbound',
                    message_type = 'FHIR',
                    source_config = $1,
                    target_config = $2,
                    processing_rules = $3,
                    status = 'active',
                    updated_at = CURRENT_TIMESTAMP
                WHERE id = $4
            `, [
                JSON.stringify({
                    host: 'localhost',
                    port: 8080,
                    path: '/fhir/Patient'
                }),
                JSON.stringify({
                    storageStrategy: 'interface-specific',
                    tableStrategy: 'dedicated',
                    tableName: `messages_intf_${fhirReceiver.id.replace(/-/g, '_').substring(0, 16)}`
                }),
                JSON.stringify({
                    routingMode: 'store',
                    messageValidation: true,
                    auditLogging: true
                }),
                fhirReceiver.id
            ]);
            
            console.log('✅ FHIR Receiver configured for message storage');
            
            // Verify configurations
            console.log('\n📋 Updated configurations:');
            const updatedResult = await client.query(`
                SELECT id, name, source_type, target_type, status, 
                       processing_rules, source_config, target_config
                FROM interfaces 
                ORDER BY created_at
            `);
            
            updatedResult.rows.forEach(row => {
                console.log(`\n${row.name}:`);
                console.log(`  Source: ${row.source_type} (${JSON.parse(row.source_config || '{}').host || 'N/A'}:${JSON.parse(row.source_config || '{}').port || 'N/A'})`);
                console.log(`  Target: ${row.target_type} (${JSON.parse(row.target_config || '{}').host || 'N/A'}:${JSON.parse(row.target_config || '{}').port || 'N/A'})`);
                console.log(`  Status: ${row.status}`);
                console.log(`  Routing: ${JSON.parse(row.processing_rules || '{}').routingMode || 'N/A'}`);
            });
        }
        
    } catch (error) {
        console.error('❌ Error:', error.message);
    } finally {
        await client.end();
    }
}

setupFHIRFlow();
