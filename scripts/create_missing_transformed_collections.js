#!/usr/bin/env node
// scripts/create_missing_transformed_collections.js
// Creates MongoDB transformed message collections for interfaces that don't have them

const database = require('../config/database');
const { MongoClient } = require('mongodb');

async function createMissingTransformedCollections() {
    console.log('🔍 Checking for interfaces without transformed MongoDB collections...\n');

    let mongoClient = null;

    try {
        // Connect to PostgreSQL
        if (!database.isConnected) {
            await database.connect();
        }

        // Get all active interfaces
        const interfaces = await database.sequelize.query(`
            SELECT id, name, status, created_at
            FROM interfaces
            WHERE is_active = true
            ORDER BY created_at
        `, {
            type: database.sequelize.QueryTypes.SELECT
        });

        console.log(`✅ Found ${interfaces.length} active interface(s)\n`);

        if (interfaces.length === 0) {
            console.log('ℹ️  No active interfaces found. Nothing to do.');
            process.exit(0);
        }

        // Connect to MongoDB
        const mongoUri = process.env.MONGODB_URI || 'mongodb://ezhealth_user:secure_password_change_me@mongodb:27017/ezhealthkonnect?authSource=admin';
        const mongoDbName = process.env.MONGODB_DATABASE || 'ezhealthkonnect';

        console.log('🔗 Connecting to MongoDB...');
        mongoClient = new MongoClient(mongoUri);
        await mongoClient.connect();
        const db = mongoClient.db(mongoDbName);
        console.log(`✅ Connected to MongoDB database: ${mongoDbName}\n`);

        // Get existing collections
        const existingCollections = await db.listCollections().toArray();
        const existingNames = new Set(existingCollections.map(c => c.name));

        let created = 0;
        let existing = 0;

        for (const intf of interfaces) {
            const collectionName = `transformed_messages_intf_${intf.id}`;

            console.log(`📋 Interface: ${intf.name} (${intf.id})`);
            console.log(`   Status: ${intf.status}`);
            console.log(`   Expected collection: ${collectionName}`);

            if (existingNames.has(collectionName)) {
                console.log(`   ✅ Collection already exists\n`);
                existing++;
            } else {
                console.log(`   🏗️  Creating collection...`);
                try {
                    // Create collection with schema validation for FHIR transformed messages
                    await db.createCollection(collectionName, {
                        validator: {
                            $jsonSchema: {
                                bsonType: "object",
                                required: ["message_id", "interface_id", "created_at"],
                                properties: {
                                    message_id: {
                                        bsonType: "string",
                                        description: "Original message ID from raw message"
                                    },
                                    correlation_id: {
                                        bsonType: "string",
                                        description: "Correlation ID for tracking message flow"
                                    },
                                    interface_id: {
                                        bsonType: "string",
                                        description: "Interface UUID"
                                    },
                                    message_type: {
                                        bsonType: "string",
                                        description: "HL7 message type (e.g., ADT^A01)"
                                    },
                                    fhir_bundle: {
                                        bsonType: "object",
                                        description: "FHIR Bundle resource (complete transformation output)"
                                    },
                                    fhir_resources: {
                                        bsonType: "array",
                                        description: "Individual FHIR resources extracted from bundle",
                                        items: {
                                            bsonType: "object"
                                        }
                                    },
                                    transformation_pipeline_id: {
                                        bsonType: "string",
                                        description: "Pipeline used for transformation"
                                    },
                                    transformation_steps: {
                                        bsonType: "array",
                                        description: "Steps executed during transformation",
                                        items: {
                                            bsonType: "object",
                                            properties: {
                                                step_id: { bsonType: "string" },
                                                step_name: { bsonType: "string" },
                                                status: { bsonType: "string" },
                                                execution_time_ms: { bsonType: "int" },
                                                error_message: { bsonType: "string" }
                                            }
                                        }
                                    },
                                    transformation_status: {
                                        bsonType: "string",
                                        enum: ["pending", "in_progress", "completed", "failed", "partial"],
                                        description: "Overall transformation status"
                                    },
                                    created_at: {
                                        bsonType: "date",
                                        description: "Timestamp when transformation was created"
                                    },
                                    completed_at: {
                                        bsonType: "date",
                                        description: "Timestamp when transformation completed"
                                    },
                                    transformation_time_ms: {
                                        bsonType: "int",
                                        description: "Total transformation time (milliseconds)"
                                    },
                                    error_count: {
                                        bsonType: "int",
                                        description: "Number of errors during transformation"
                                    },
                                    errors: {
                                        bsonType: "array",
                                        description: "Detailed error information",
                                        items: {
                                            bsonType: "object",
                                            properties: {
                                                step: { bsonType: "string" },
                                                message: { bsonType: "string" },
                                                timestamp: { bsonType: "date" }
                                            }
                                        }
                                    },
                                    validation_results: {
                                        bsonType: "object",
                                        description: "FHIR validation results"
                                    }
                                }
                            }
                        }
                    });

                    // Create indexes for performance
                    const collection = db.collection(collectionName);
                    await collection.createIndex({ message_id: 1 }, { unique: true });
                    await collection.createIndex({ interface_id: 1 });
                    await collection.createIndex({ correlation_id: 1 });
                    await collection.createIndex({ created_at: -1 });
                    await collection.createIndex({ completed_at: -1 });
                    await collection.createIndex({ transformation_status: 1 });
                    await collection.createIndex({ message_type: 1 });
                    await collection.createIndex({ 'fhir_resources.resourceType': 1 });

                    console.log(`   ✅ Collection created: ${collectionName}`);
                    console.log(`   ✅ Indexes created: message_id (unique), interface_id, correlation_id, created_at, completed_at, transformation_status, message_type, fhir_resources.resourceType\n`);
                    created++;
                } catch (error) {
                    console.error(`   ❌ Failed to create collection:`, error.message);
                }
            }
        }

        console.log('\n' + '='.repeat(60));
        console.log('📊 Summary:');
        console.log(`   Total interfaces: ${interfaces.length}`);
        console.log(`   Collections existing: ${existing}`);
        console.log(`   Collections created: ${created}`);
        console.log('='.repeat(60));

        if (created > 0) {
            console.log('\n✅ All missing transformed MongoDB collections have been created!');
        } else {
            console.log('\n✅ All interfaces already have their transformed MongoDB collections.');
        }

    } catch (error) {
        console.error('\n❌ Error:', error);
        console.error(error.stack);
        process.exit(1);
    } finally {
        if (mongoClient) {
            await mongoClient.close();
        }
        process.exit(0);
    }
}

// Run the script
createMissingTransformedCollections();
