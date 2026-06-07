'use strict';
/**
 * MLLP helper — sends HL7 over TCP with MLLP framing and returns the ACK.
 * Runs in Node (not the browser) so can be called directly from Playwright tests.
 *
 * MLLP framing:
 *   0x0B  <HL7 message>  0x1C 0x0D
 */

const net = require('net');

const MLLP_START = '\x0B';
const MLLP_END   = '\x1C\x0D';

/**
 * Send one HL7 message to host:port and return the raw ACK string.
 * Resolves when the ACK is received or rejects on timeout / connection error.
 *
 * @param {string} host
 * @param {number} port
 * @param {string} hl7Message  - raw HL7, no MLLP framing
 * @param {number} timeoutMs
 * @returns {Promise<string>}  raw ACK (no framing)
 */
function sendMLLP(host, port, hl7Message, timeoutMs = 10_000) {
    return new Promise((resolve, reject) => {
        const socket = new net.Socket();
        let ackBuffer = '';
        let settled   = false;

        const done = (err, result) => {
            if (settled) return;
            settled = true;
            socket.destroy();
            if (err) reject(err);
            else resolve(result);
        };

        socket.setTimeout(timeoutMs);

        socket.connect(port, host, () => {
            socket.write(MLLP_START + hl7Message + MLLP_END);
        });

        socket.on('data', (chunk) => {
            ackBuffer += chunk.toString();
            // ACK is complete once we see the end block
            if (ackBuffer.includes(MLLP_END)) {
                const raw = ackBuffer
                    .replace(MLLP_START, '')
                    .replace(MLLP_END, '')
                    .trim();
                done(null, raw);
            }
        });

        socket.on('timeout', () => done(new Error(`MLLP timeout after ${timeoutMs}ms`)));
        socket.on('error',   (err) => done(err));
        socket.on('close',   () => {
            if (!settled) done(new Error('Socket closed before ACK received'));
        });
    });
}

/**
 * Parse the ACK code from a raw ACK string.
 * Returns 'AA', 'AE', 'AR', or 'UNKNOWN'.
 */
function parseAckCode(ack) {
    const msaLine = ack.split(/\r\n|\r|\n/).find(l => l.startsWith('MSA'));
    if (!msaLine) return 'UNKNOWN';
    return msaLine.split('|')[1] || 'UNKNOWN';
}

module.exports = { sendMLLP, parseAckCode };
