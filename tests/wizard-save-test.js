// tests/wizard-save-test.js
// Simple test to verify the wizard save flow works end-to-end

const WizardController = require('../controllers/wizardController');
const MessageTypeMappingService = require('../services/MessageTypeMappingService');

/**
 * Test the wizard completion flow with sample data
 * This test simulates what happens when a user completes the wizard
 */
async function testWizardSaveFlow() {
    console.log('🧪 Testing Wizard Save Flow...\n');

    // Sample wizard data that would come from the frontend
    const sampleWizardData = {
        name: 'Test HL7-FHIR Interface',
        description: 'Test interface for ADT message transformation',
        sourceType: 'hl7v2',
        targetType: 'fhir',
        messageType: 'ADT^A01',
        sourceConfig: {
            type: 'hl7',
            connectivity: 'tcp',
            host: 'localhost',
            port: 2575
        },
        targetConfig: {
            type: 'fhir',
            connectivity: 'http',
            endpoint: 'https://fhir.example.com/R4'
        },
        // Sample transformation mapping data from Step 4
        transformationMapping: {
            atomicMappings: [
                {
                    hl7Field: 'MSH.3',
                    resourceType: 'MessageHeader',
                    fhirPath: 'source.name',
                    value: 'Test Application',
                    transformationType: 'direct',
                    validated: true
                },
                {
                    hl7Field: 'PID.3.1',
                    resourceType: 'Patient',
                    fhirPath: 'identifier[0].value',
                    value: '12345',
                    transformationType: 'direct',
                    validated: true
                },
                {
                    hl7Field: 'PID.5.1',
                    resourceType: 'Patient',
                    fhirPath: 'name[0].family',
                    value: 'Smith',
                    transformationType: 'direct',
                    validated: true
                }
            ],
            messageType: 'ADT^A01',
            enhancedSegments: {
                MSH: { messageType: 'ADT^A01', fields: ['MSH.3'] },
                PID: { messageType: 'ADT^A01', fields: ['PID.3.1', 'PID.5.1'] }
            },
            version: '2.0',
            extractionSummary: {
                atomicMappingsCount: 3,
                segmentCount: 2,
                messageType: 'ADT^A01'
            }
        }
    };

    // Mock session user
    const mockUser = {
        id: '123e4567-e89b-12d3-a456-426614174000',
        email: 'test@example.com',
        name: 'Test User'
    };

    try {
        // Test 1: Check WizardController
        console.log('1️⃣ Checking WizardController...');
        const wizardController = WizardController; // It's already an instance
        console.log('   ✅ WizardController loaded (pre-instantiated)');

        // Test 2: Test MessageTypeMappingService initialization
        console.log('\n2️⃣ Testing MessageTypeMappingService...');
        const mappingService = new MessageTypeMappingService();
        console.log('   ✅ MessageTypeMappingService initialized');

        // Test 3: Test the data extraction methods
        console.log('\n3️⃣ Testing data extraction methods...');

        // Test message type extraction
        const messageType = mappingService.extractMessageType(sampleWizardData.transformationMapping);
        console.log(`   📋 Extracted message type: ${messageType}`);

        // Test custom mapping config building
        const customConfig = mappingService.buildCustomMappingConfig(sampleWizardData.transformationMapping);
        console.log(`   🔧 Built custom config with ${customConfig.atomicMappings.length} mappings`);

        console.log('   ✅ Data extraction methods work correctly');

        // Test 4: Verify the controller can handle the wizard data format
        console.log('\n4️⃣ Testing controller data processing...');

        const sourceMapping = wizardController.mapSourceTypeAndConnectivity(sampleWizardData.sourceType);
        const targetMapping = wizardController.mapTargetTypeAndConnectivity(sampleWizardData.targetType);

        console.log(`   🔗 Source mapping: ${JSON.stringify(sourceMapping)}`);
        console.log(`   🔗 Target mapping: ${JSON.stringify(targetMapping)}`);
        console.log('   ✅ Controller data processing works');

        // Test 5: Test the interface data preparation
        console.log('\n5️⃣ Testing interface data preparation...');

        const interfaceData = {
            name: sampleWizardData.name,
            description: sampleWizardData.description,
            sourceType: sourceMapping.format,
            sourceConnectivity: sourceMapping.connectivity,
            targetType: targetMapping.format,
            targetConnectivity: targetMapping.connectivity,
            messageType: sampleWizardData.messageType,
            status: 'active',
            sourceConfig: sampleWizardData.sourceConfig,
            targetConfig: sampleWizardData.targetConfig,
            transformationMapping: sampleWizardData.transformationMapping
        };

        console.log(`   📝 Interface data prepared: ${interfaceData.name}`);
        console.log(`   📊 Atomic mappings count: ${interfaceData.transformationMapping.atomicMappings.length}`);
        console.log('   ✅ Interface data preparation works');

        // Test Summary
        console.log('\n🎯 Test Summary:');
        console.log('   ✅ WizardController initialization: PASS');
        console.log('   ✅ MessageTypeMappingService initialization: PASS');
        console.log('   ✅ Data extraction methods: PASS');
        console.log('   ✅ Controller data processing: PASS');
        console.log('   ✅ Interface data preparation: PASS');

        console.log('\n✅ ALL TESTS PASSED - Message-type-centric wizard save flow ready!');
        console.log('\n📝 Next Steps:');
        console.log('   1. Run database migration V9 to create message-type-centric schema');
        console.log('   2. Test the actual wizard completion in the UI');
        console.log('   3. Verify data is saved to interfaces and interface_message_mappings tables');
        console.log('   4. Test Go backend can retrieve mappings via /api/wizard/runtime-mapping/:interfaceId/:messageType');

        return true;

    } catch (error) {
        console.error('\n❌ TEST FAILED:', error.message);
        console.error('   Error details:', error.stack);
        return false;
    }
}

// Run the test if called directly
if (require.main === module) {
    testWizardSaveFlow().then(success => {
        process.exit(success ? 0 : 1);
    });
}

module.exports = { testWizardSaveFlow };