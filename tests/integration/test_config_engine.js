#!/usr/bin/env node

/**
 * Test script for Interface-Centric Configuration Engine
 * Tests the complete MongoDB-backed configuration system
 */

const axios = require('axios');
const { MongoClient } = require('mongodb');

// Configuration
const BASE_URL = 'http://localhost:8080';
const MONGODB_URI = process.env.MONGODB_URI || 'mongodb://localhost:27017';
const MONGODB_DATABASE = process.env.MONGODB_DATABASE || 'ezhealthkonnect';

class ConfigEngineTest {
    constructor() {
        this.results = {
            total: 0,
            passed: 0,
            failed: 0,
            errors: []
        };
    }

    async runAllTests() {
        console.log('🧪 Starting Interface-Centric Configuration Engine Tests\n');

        try {
            // Test 1: System Health
            await this.testSystemHealth();

            // Test 2: MongoDB Connection
            await this.testMongoDBConnection();

            // Test 3: Configuration CRUD
            await this.testConfigurationCRUD();

            // Test 4: Template Management
            await this.testTemplateManagement();

            // Test 5: Hot Reload
            await this.testHotReload();

            // Test 6: Migration Status
            await this.testMigrationStatus();

            // Test 7: Runtime Monitoring
            await this.testRuntimeMonitoring();

        } catch (error) {
            this.recordError('System Test', error.message);
        }

        this.printResults();
    }

    async testSystemHealth() {
        console.log('📊 Testing System Health...');

        try {
            const response = await axios.get(`${BASE_URL}/api/config/health`);

            if (response.status === 200 && response.data.success) {
                this.recordPass('System Health Check');
                console.log('   ✅ Configuration system is healthy');

                if (response.data.data.services) {
                    const services = response.data.data.services;
                    console.log(`   📋 Config Manager: ${services.config_manager ? '✅' : '❌'}`);
                    console.log(`   ⚙️  Interface Engine: ${services.interface_engine ? '✅' : '❌'}`);
                    console.log(`   🔄 Migration Service: ${services.migration_service ? '✅' : '❌'}`);
                }
            } else {
                this.recordFail('System Health Check', 'Invalid response');
            }
        } catch (error) {
            this.recordFail('System Health Check', error.message);
        }
    }

    async testMongoDBConnection() {
        console.log('\n🗄️  Testing MongoDB Connection...');

        try {
            const client = new MongoClient(MONGODB_URI);
            await client.connect();

            const db = client.db(MONGODB_DATABASE);
            const collections = await db.listCollections().toArray();

            this.recordPass('MongoDB Connection');
            console.log('   ✅ MongoDB connection successful');
            console.log(`   📊 Database: ${MONGODB_DATABASE}`);
            console.log(`   📋 Collections: ${collections.length}`);

            await client.close();
        } catch (error) {
            this.recordFail('MongoDB Connection', error.message);
        }
    }

    async testConfigurationCRUD() {
        console.log('\n🔧 Testing Configuration CRUD Operations...');

        try {
            // Test Create Configuration
            const testConfig = {
                name: "Test Interface",
                description: "Test interface for configuration engine",
                version: "1.0.0",
                status: "draft",
                pipeline: {
                    input: {
                        type: "mllp",
                        connector_config: {
                            host: "0.0.0.0",
                            port: 6661,
                            timeout: 30000
                        },
                        validation: { enabled: true, rules: [] },
                        preprocessing: { enabled: false, steps: [] }
                    },
                    validation: {
                        schema_validation: {
                            enabled: true,
                            schema_type: "hl7",
                            strict_mode: false
                        },
                        business_rules: [],
                        custom_validators: []
                    },
                    transformation: {
                        engine: "hl7_to_fhir",
                        mapping_template: "standard_adt_v4",
                        custom_mappings: [],
                        post_processing: []
                    },
                    business_logic: {
                        rules_engine: { enabled: false, rules: [] },
                        workflow_automation: []
                    },
                    destinations: [
                        {
                            destination_id: "test_destination",
                            type: "file",
                            config: { output_directory: "/tmp/test" },
                            routing_rules: [],
                            error_handling: {
                                retry_count: 3,
                                retry_delay: 5000,
                                dead_letter_queue: true
                            }
                        }
                    ]
                },
                monitoring: {
                    metrics_enabled: true,
                    alert_thresholds: {
                        error_rate: 0.05,
                        processing_time_ms: 5000
                    },
                    retention_policy: {
                        raw_messages: 90,
                        processed_messages: 30
                    }
                }
            };

            const createResponse = await axios.post(`${BASE_URL}/api/config/interfaces`, testConfig);

            if (createResponse.status === 201 && createResponse.data.success) {
                const createdConfig = createResponse.data.data;
                this.recordPass('Create Configuration');
                console.log('   ✅ Configuration created successfully');
                console.log(`   🆔 Interface ID: ${createdConfig.interface_id}`);

                // Test Read Configuration
                const readResponse = await axios.get(`${BASE_URL}/api/config/interfaces/${createdConfig.interface_id}`);

                if (readResponse.status === 200 && readResponse.data.success) {
                    this.recordPass('Read Configuration');
                    console.log('   ✅ Configuration retrieved successfully');
                } else {
                    this.recordFail('Read Configuration', 'Failed to retrieve configuration');
                }

                // Test Update Configuration
                const updatedConfig = { ...createdConfig, description: "Updated test interface" };
                const updateResponse = await axios.put(`${BASE_URL}/api/config/interfaces/${createdConfig.interface_id}`, updatedConfig);

                if (updateResponse.status === 200 && updateResponse.data.success) {
                    this.recordPass('Update Configuration');
                    console.log('   ✅ Configuration updated successfully');
                } else {
                    this.recordFail('Update Configuration', 'Failed to update configuration');
                }

            } else {
                this.recordFail('Create Configuration', 'Failed to create configuration');
            }

        } catch (error) {
            this.recordFail('Configuration CRUD', error.message);
        }
    }

    async testTemplateManagement() {
        console.log('\n📋 Testing Template Management...');

        try {
            const response = await axios.get(`${BASE_URL}/api/config/templates`);

            if (response.status === 200 && response.data.success) {
                this.recordPass('List Templates');
                console.log('   ✅ Templates retrieved successfully');
                console.log(`   📊 Template count: ${response.data.data.length}`);
            } else {
                this.recordFail('List Templates', 'Failed to retrieve templates');
            }
        } catch (error) {
            this.recordFail('Template Management', error.message);
        }
    }

    async testHotReload() {
        console.log('\n🔥 Testing Hot Reload Capability...');

        try {
            // Test reload all configurations
            const response = await axios.post(`${BASE_URL}/api/config/reload/all`);

            if (response.status === 200 && response.data.success) {
                this.recordPass('Hot Reload All');
                console.log('   ✅ Hot reload completed successfully');

                if (response.data.data) {
                    console.log(`   📊 Total configs: ${response.data.data.total_configs}`);
                    console.log(`   🔄 Reloaded: ${response.data.data.reloaded_configs}`);
                }
            } else {
                this.recordFail('Hot Reload All', 'Failed to reload configurations');
            }
        } catch (error) {
            this.recordFail('Hot Reload', error.message);
        }
    }

    async testMigrationStatus() {
        console.log('\n🔄 Testing Migration Status...');

        try {
            const response = await axios.get(`${BASE_URL}/api/config/migrate/status`);

            if (response.status === 200 && response.data.success) {
                this.recordPass('Migration Status');
                console.log('   ✅ Migration status retrieved successfully');

                if (response.data.data) {
                    const stats = response.data.data;
                    console.log(`   📊 Total interfaces: ${stats.total_interfaces}`);
                    console.log(`   🔄 Migrated interfaces: ${stats.migrated_interfaces}`);
                    console.log(`   📋 Total templates: ${stats.total_templates}`);
                    console.log(`   🔄 Migrated templates: ${stats.migrated_templates}`);
                }
            } else {
                this.recordFail('Migration Status', 'Failed to get migration status');
            }
        } catch (error) {
            this.recordFail('Migration Status', error.message);
        }
    }

    async testRuntimeMonitoring() {
        console.log('\n📊 Testing Runtime Monitoring...');

        try {
            // Test runtime stats
            const statsResponse = await axios.get(`${BASE_URL}/api/config/runtime/stats`);

            if (statsResponse.status === 200 && statsResponse.data.success) {
                this.recordPass('Runtime Statistics');
                console.log('   ✅ Runtime stats retrieved successfully');

                if (statsResponse.data.data) {
                    console.log(`   📊 Active messages: ${statsResponse.data.data.active_messages}`);
                }
            } else {
                this.recordFail('Runtime Statistics', 'Failed to get runtime stats');
            }

            // Test active processes
            const activeResponse = await axios.get(`${BASE_URL}/api/config/runtime/active`);

            if (activeResponse.status === 200 && activeResponse.data.success) {
                this.recordPass('Active Processes');
                console.log('   ✅ Active processes retrieved successfully');

                if (activeResponse.data.data) {
                    console.log(`   🔄 Active process count: ${activeResponse.data.data.count}`);
                }
            } else {
                this.recordFail('Active Processes', 'Failed to get active processes');
            }

        } catch (error) {
            this.recordFail('Runtime Monitoring', error.message);
        }
    }

    recordPass(testName) {
        this.results.total++;
        this.results.passed++;
        console.log(`   ✅ ${testName}: PASSED`);
    }

    recordFail(testName, error) {
        this.results.total++;
        this.results.failed++;
        this.results.errors.push({ test: testName, error });
        console.log(`   ❌ ${testName}: FAILED - ${error}`);
    }

    recordError(testName, error) {
        this.results.errors.push({ test: testName, error });
        console.log(`   💥 ${testName}: ERROR - ${error}`);
    }

    printResults() {
        console.log('\n' + '='.repeat(60));
        console.log('📊 TEST RESULTS SUMMARY');
        console.log('='.repeat(60));
        console.log(`Total Tests: ${this.results.total}`);
        console.log(`Passed: ${this.results.passed} ✅`);
        console.log(`Failed: ${this.results.failed} ❌`);

        const successRate = this.results.total > 0 ? (this.results.passed / this.results.total * 100).toFixed(1) : 0;
        console.log(`Success Rate: ${successRate}%`);

        if (this.results.errors.length > 0) {
            console.log('\n🔍 ERROR DETAILS:');
            this.results.errors.forEach((error, index) => {
                console.log(`${index + 1}. ${error.test}: ${error.error}`);
            });
        }

        console.log('\n' + '='.repeat(60));

        if (this.results.failed === 0) {
            console.log('🎉 All tests passed! Configuration Engine is working correctly.');
        } else {
            console.log('⚠️  Some tests failed. Please check the errors above.');
            process.exit(1);
        }
    }
}

// Run tests if this script is executed directly
if (require.main === module) {
    const tester = new ConfigEngineTest();
    tester.runAllTests().catch(error => {
        console.error('💥 Test execution failed:', error);
        process.exit(1);
    });
}

module.exports = ConfigEngineTest;