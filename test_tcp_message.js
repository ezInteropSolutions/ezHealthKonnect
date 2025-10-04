// test_tcp_message.js
// Test sending HL7 message via TCP/MLLP to port 6661

const net = require('net');

// MLLP protocol bytes
const VT = String.fromCharCode(0x0B);  // Start byte
const FS = String.fromCharCode(0x1C);  // End byte 1
const CR = String.fromCharCode(0x0D);  // End byte 2

// Sample HL7 ADT^A01 message
const hl7Message = `MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20251002130000||ADT^A01|MSG00001|P|2.5|
PID|1||12345||DOE^JOHN^A||19800101|M|||123 MAIN ST^^CITY^STATE^12345|||||||||
PV1|1|I|WARD^ROOM^BED||||ATTENDING^DOC^TOR|||||||||||V123456|||||||||||||||||||||||||20251002120000|`;

// Wrap in MLLP envelope
const mllpMessage = VT + hl7Message + FS + CR;

console.log('🚀 Connecting to localhost:6661...');

const client = net.createConnection({ host: 'localhost', port: 6661 }, () => {
    console.log('✅ Connected to TCP server');
    console.log('📤 Sending HL7 message...');
    console.log(`Message length: ${mllpMessage.length} bytes`);

    client.write(mllpMessage);
});

// Handle ACK response
client.on('data', (data) => {
    console.log('📥 Received response:');
    console.log(data.toString());

    // Check if it's an ACK
    if (data.toString().includes('MSA|AA')) {
        console.log('✅ ACK received successfully!');
    } else {
        console.log('⚠️  Unexpected response');
    }

    client.end();
});

client.on('end', () => {
    console.log('🔌 Disconnected from server');
});

client.on('error', (err) => {
    console.error('❌ Connection error:', err.message);
});

setTimeout(() => {
    if (!client.destroyed) {
        console.log('⏱️  Timeout - closing connection');
        client.destroy();
    }
}, 10000);
