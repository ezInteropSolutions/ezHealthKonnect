const net = require('net');

// MLLP protocol wrapper function
function wrapMLLP(message) {
    const START_BLOCK = '\x0B'; // VT (Vertical Tab)
    const END_BLOCK = '\x1C';   // FS (File Separator)
    const CARRIAGE_RETURN = '\x0D'; // CR (Carriage Return)

    return START_BLOCK + message + END_BLOCK + CARRIAGE_RETURN;
}

// HL7 Test Message
const hl7Message = `MSH|^~\\&|SendingApp|SendingFacility|ReceivingApp|ReceivingFacility|20250921124500||ADT^A01^ADT_A01|MSG001|P|2.5\r` +
`EVN|A01|20250921124500|||^ADMIN^USER\r` +
`PID|1||123456^^^MRN||Doe^John^A||19850315|M|||123 Main St^^Anytown^NY^12345^USA||555-123-4567|||S||123456789|987-65-4321\r` +
`NK1|1|Doe^Jane^M|SPO|||||555-987-6543\r` +
`PV1|1|I|ICU^101^01||||DOC001^Doctor^Primary|||SUR||||1|A|||||||||||||||||||||||20250921124500`;

console.log('🚀 Sending HL7 message to TCP interface on port 6661...');
console.log('📋 Message Type: ADT^A01 (Admission)');
console.log('🏥 Patient: John Doe (MRN: 123456)');

const client = net.createConnection({ port: 6661, host: 'localhost' }, () => {
    console.log('✅ Connected to TCP interface');

    // Send MLLP-wrapped message
    const mllpMessage = wrapMLLP(hl7Message);
    client.write(mllpMessage);

    console.log('📤 HL7 message sent via MLLP protocol');
});

client.on('data', (data) => {
    console.log('📥 Received acknowledgment:', data.toString());
});

client.on('end', () => {
    console.log('🔚 Connection closed by server');
});

client.on('error', (err) => {
    console.error('❌ Connection error:', err.message);
});

// Close connection after 2 seconds
setTimeout(() => {
    client.end();
    console.log('✅ Test completed');
    process.exit(0);
}, 2000);