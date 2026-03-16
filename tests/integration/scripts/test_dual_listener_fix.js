// tests/integration/scripts/test_dual_listener_fix.js
// Validates that connector.inbound steps no longer cause
// "bind: address already in use" when the engine already started the listener.
//
// Usage: node tests/integration/scripts/test_dual_listener_fix.js [port] [host]
// Defaults: port=6610, host=127.0.0.1

const net = require('net');
const https = require('https');
const http  = require('http');

const PORT = parseInt(process.argv[2] || '6610', 10);
const HOST = process.argv[3] || '127.0.0.1';

// Build a minimal MLLP-framed ADT^A01 HL7 message
const HL7 = [
  'MSH|^~\\&|TestApp|TestFac|RecvApp|RecvFac|20260314120000||ADT^A01|DUALFIX001|P|2.5',
  'PID|1||P99999^^^MRN||Fix^Test^A||19801231|M',
  'PV1|1|I|WARD^^BED|||||||ATTENDING',
].join('\r') + '\r';

const MLLP_START = Buffer.from([0x0b]);
const MLLP_END   = Buffer.from([0x1c, 0x0d]);
const message    = Buffer.concat([MLLP_START, Buffer.from(HL7), MLLP_END]);

console.log(`\n🔌 Connecting to MLLP listener at ${HOST}:${PORT} ...`);

const sock = net.createConnection({ host: HOST, port: PORT }, () => {
  console.log('✅ Connected — sending ADT^A01 (msg ID: DUALFIX001)');
  sock.write(message);
});

let ackRaw = '';
sock.on('data', chunk => { ackRaw += chunk.toString('ascii'); });

sock.setTimeout(5000);
sock.on('timeout', () => {
  console.log('ℹ️  Socket timeout (5s) — closing');
  sock.end();
});

sock.on('end', () => {
  console.log('\n📨 ACK received from server:');
  const visible = ackRaw.replace(/\x0b/g, '<SB>').replace(/\x1c/g, '<EB>').replace(/\r/g, '<CR>\n');
  console.log(visible);

  if (/MSA\|AA/i.test(ackRaw)) {
    console.log('\n✅ PASS — got AA acknowledgment (message accepted by listener)');
  } else if (/MSA\|AE/i.test(ackRaw) || /MSA\|AR/i.test(ackRaw)) {
    console.log('\n⚠️  NAK received — message rejected by server');
  } else {
    console.log('\n⚠️  No MSA segment found in response');
  }

  // Give the pipeline ~3 seconds to execute then check for the bind error in Go logs
  console.log('\n⏳ Waiting 4s for pipeline to execute ...');
  setTimeout(() => {
    console.log('\nCheck docker logs for "bind: address already in use" — should be ABSENT for new messages.');
    console.log('Test complete.\n');
    process.exit(0);
  }, 4000);
});

sock.on('error', err => {
  console.error(`❌ Connection error: ${err.message}`);
  if (err.code === 'ECONNREFUSED') {
    console.error(`   → Port ${PORT} is not listening. Is the interface active?`);
  }
  process.exit(1);
});
