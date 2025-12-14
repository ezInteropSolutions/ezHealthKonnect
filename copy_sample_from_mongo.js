const { MongoClient } = require('mongodb');
const { Pool } = require('pg');

// MongoDB configuration
const mongoUrl = process.env.MONGODB_URI || 'mongodb://ezhealth_user:secure_password_change_me@localhost:27017/ezhealthkonnect?authSource=admin';
const mongoDbName = 'ezhealthkonnect';

// PostgreSQL configuration
const pgPool = new Pool({
    host: process.env.DB_HOST || 'localhost',
    port: process.env.DB_PORT || 5432,
    database: process.env.DB_NAME || 'ezhealthkonnect',
    user: process.env.DB_USER || 'ezhealth_user',
    password: process.env.DB_PASSWORD || 'secure_password_change_me'
});

async function copySampleMessage() {
    let mongoClient;

    try {
        // Connect to MongoDB
        console.log('📡 Connecting to MongoDB...');
        mongoClient = new MongoClient(mongoUrl);
        await mongoClient.connect();

        const db = mongoClient.db(mongoDbName);

        // List collections
        const collections = await db.listCollections().toArray();
        const messageCollections = collections.filter(c => c.name.startsWith('raw_messages_intf_'));

        console.log(`✅ Found ${messageCollections.length} message collections`);

        if (messageCollections.length === 0) {
            console.log('⚠️  No message collections found. Please send a test message through an interface first.');
            return;
        }

        // Try to find a message with parsed content
        let foundMessage = null;
        let foundCollection = null;

        for (const collInfo of messageCollections.slice(0, 5)) {
            const coll = db.collection(collInfo.name);
            const message = await coll.findOne({
                parsed_content: { $exists: true, $ne: null }
            });

            if (message && message.parsed_content) {
                foundMessage = message;
                foundCollection = collInfo.name;
                console.log(`✅ Found parsed message in collection: ${collInfo.name}`);
                break;
            }
        }

        if (!foundMessage) {
            console.log('⚠️  No parsed messages found in MongoDB. Please send a test message first.');
            return;
        }

        const parsedContent = foundMessage.parsed_content;

        // Extract metadata
        const messageType = parsedContent.messageType?.name || parsedContent.messageType?.code || 'UNKNOWN';
        const version = parsedContent.version || '2.5';

        console.log('📝 Message details:');
        console.log('   Type:', messageType);
        console.log('   Version:', version);
        console.log('   Segments:', Object.keys(parsedContent.enhancedSegments || {}).join(', '));

        // Insert into PostgreSQL
        const checkQuery = `
            SELECT id FROM sample_parsed_messages
            WHERE message_type = $1 AND hl7_version = $2
        `;

        const checkResult = await pgPool.query(checkQuery, [messageType, version]);

        if (checkResult.rows.length > 0) {
            console.log('⚠️  Sample already exists, updating...');

            const updateQuery = `
                UPDATE sample_parsed_messages
                SET parsed_content = $1,
                    description = $2,
                    updated_at = CURRENT_TIMESTAMP
                WHERE id = $3
                RETURNING id
            `;

            const result = await pgPool.query(updateQuery, [
                parsedContent.enhancedSegments || parsedContent,
                `Sample message (${messageType} v${version}) - Auto-copied from ${foundCollection}`,
                checkResult.rows[0].id
            ]);

            console.log(`✅ Updated sample message (ID: ${result.rows[0].id})`);
        } else {
            console.log('📝 Inserting new sample...');

            const insertQuery = `
                INSERT INTO sample_parsed_messages
                (message_type, hl7_version, format, parsed_content, description, is_active)
                VALUES ($1, $2, $3, $4, $5, $6)
                RETURNING id
            `;

            const result = await pgPool.query(insertQuery, [
                messageType,
                version,
                'hl7v2',
                parsedContent.enhancedSegments || parsedContent,
                `Sample message (${messageType} v${version}) - Auto-copied from ${foundCollection}`,
                true
            ]);

            console.log(`✅ Inserted sample message (ID: ${result.rows[0].id})`);
        }

        console.log('\n✅ Sample message loaded successfully!');
        console.log('   Refresh your pipeline builder page and the autocomplete should now show all fields');

    } catch (error) {
        console.error('❌ Error:', error.message);
        console.error(error.stack);
    } finally {
        if (mongoClient) {
            await mongoClient.close();
        }
        await pgPool.end();
    }
}

copySampleMessage();
