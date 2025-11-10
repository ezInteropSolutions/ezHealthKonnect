// Check transformed message in MongoDB
const collectionName = 'transformed_messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d';

print(`\n=== Checking ${collectionName} ===\n`);

const doc = db.getCollection(collectionName).findOne({}, {sort: {created_at: -1}});

if (!doc) {
    print('❌ No documents found!');
} else {
    print(`✅ Found document: ${doc.message_id}`);
    print(`   Format: ${doc.delivery_payload ? doc.delivery_payload.format : 'NO DELIVERY PAYLOAD'}`);
    print(`   Transmission content type: ${doc.delivery_payload?.transmission?.content_type || 'N/A'}`);
    print(`   Transmission content size: ${doc.delivery_payload?.transmission?.content?.length || 0} bytes`);
    print(`   Transformed content type: ${doc.transformed_content ? doc.transformed_content.resourceType : 'N/A'}`);
    print(`   Delivery status: ${doc.delivery_status || 'N/A'}`);

    // Check if transmission.content has actual FHIR data
    if (doc.delivery_payload?.transmission?.content) {
        const content = doc.delivery_payload.transmission.content;
        if (typeof content === 'object' && content.type === 'Buffer') {
            const bytes = Buffer.from(content.data);
            const preview = bytes.toString('utf8', 0, Math.min(100, bytes.length));
            print(`\n   📦 Transmission content preview (first 100 chars):`);
            print(`   ${preview}...`);
        }
    }
}
