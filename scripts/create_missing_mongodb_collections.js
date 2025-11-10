#!/usr/bin/env node
// scripts/create_missing_mongodb_collections.js
// Creates MongoDB collections for interfaces that don't have them

const database = require('../config/database');
const { MongoClient } = require('mongodb');

async function createMissingCollections() {
    console.log('🔍 Checking for interfaces without MongoDB collections...\n');

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
            const collectionName = `raw_messages_${intf.id}`;

            console.log(`📋 Interface: ${intf.name} (${intf.id})`);
            console.log(`   Status: ${intf.status}`);
            console.log(`   Expected collection: ${collectionName}`);

            if (existingNames.has(collectionName)) {
                console.log(`   ✅ Collection already exists\n`);
                existing++;
            } else {
                console.log(`   🏗️  Creating collection...`);
                try {
                    // Create collection with schema validation and indexes
                    await db.createCollection(collectionName, {
                        validator: {
                            $jsonSchema: {
                                bsonType: "object",
                                required: ["message_id", "raw_content", "received_at"],
                                properties: {
                                    message_id: {
                                        bsonType: "string",
                                        description: "Unique message identifier"
                                    },
                                    raw_content: {
                                        bsonType: "string",
                                        description: "Raw message content (HL7, FHIR, etc.)"
                                    },
                                    parsed_content: {
                                        bsonType: "object",
                                        description: "Parsed JSON representation"
                                    },
                                    received_at: {
                                        bsonType: "date",
                                        description: "Timestamp when message was received"
                                    },
                                    parsed_at: {
                                        bsonType: "date",
                                        description: "Timestamp when message was parsed"
                                    },
                                    parsing_time_ms: {
                                        bsonType: "int",
                                        description: "Time taken to parse (milliseconds)"
                                    },
                                    parsed_format: {
                                        bsonType: "string",
                                        description: "Detected format (hl7v2, fhir, etc.)"
                                    }
                                }
                            }
                        }
                    });

                    // Create indexes for performance
                    const collection = db.collection(collectionName);
                    await collection.createIndex({ message_id: 1 }, { unique: true });
                    await collection.createIndex({ received_at: -1 });
                    await collection.createIndex({ parsed_at: -1 });
                    await collection.createIndex({ parsed_format: 1 });

                    console.log(`   ✅ Collection created: ${collectionName}`);
                    console.log(`   ✅ Indexes created: message_id (unique), received_at, parsed_at, parsed_format\n`);
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
            console.log('\n✅ All missing MongoDB collections have been created!');
        } else {
            console.log('\n✅ All interfaces already have their MongoDB collections.');
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
createMissingCollections();
