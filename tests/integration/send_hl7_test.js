const net = require('net');
const fs = require('fs');

// Read the HL7 message
const hl7Message = fs.readFileSync('./test_hl7_message.txt', 'utf8');

// Create MLLP framed message
const mllpMessage = '\x0B' + hl7Message + '\x1C\x0D';

// Connect to Test Interface1
const client = new net.Socket();

client.connect(6661, 'localhost', () => {
    console.log('🔗 Connected to Test Interface1 (port 6661)');
    console.log('📤 Sending HL7 ADT^A01 message...');

    // Send the message
    client.write(mllpMessage);
});

client.on('data', (data) => {
    console.log('📥 Response received:', data.toString());
    client.destroy();
});

client.on('close', () => {
    console.log('✅ Connection closed');
});

client.on('error', (err) => {
    console.error('❌ Connection error:', err.message);
});

// Auto-close after 5 seconds
setTimeout(() => {
    if (!client.destroyed) {
        console.log('⏰ Timeout - closing connection');
        client.destroy();
    }
}, 5000);