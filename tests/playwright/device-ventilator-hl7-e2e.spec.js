// tests/playwright/device-ventilator-hl7-e2e.spec.js
//
// ICU ventilator over LAN (HL7 v2 over TCP/MLLP), driven end to end through the
// real "Connect a Device" wizard ("HL7 over TCP/IP" template, V274) — the
// release-regression guard for the device-integration use case a prospect
// actually described: a ventilator that speaks HL7 on its LAN port and needs a
// listener.
//
// What device-connect-wizard-e2e.spec.js does NOT already cover, and this does:
//   - A realistic IHE PCD-01 (DEC) ORU^R01: HL7 v2.6, IEEE 11073 MDC codes,
//     MDS/VMD/channel hierarchy in OBX-4, multiple numeric OBX per message.
//   - A sustained stream on ONE persistent connection (ventilators keep the
//     socket open and push every few seconds), every ACK correlated to its own
//     MSH-10 control ID.
//   - Real downstream delivery: the template's http_outbound target points at
//     a local HTTP receiver, so the test asserts the ventilator's values
//     actually left the engine, not just that an ACK came back.
//   - Robustness: a frame without MSH, and a connection dropped mid-frame, must
//     not take the listener down for the next well-formed message.
const { test, expect } = require('@playwright/test');
const net = require('net');
const http = require('http');

const TEMPLATE_NAME = 'HL7 over TCP/IP';
const MLLP_PORT = 6657;          // inside the docker-mapped 6610-6670 range, unused by other suites
const STREAM_COUNT = 30;

const VB = 0x0B, FS = 0x1C, CR = 0x0D;
const frame = (msg) => Buffer.concat([Buffer.from([VB]), Buffer.from(msg, 'ascii'), Buffer.from([FS, CR])]);

function hl7ts(d = new Date()) { return d.toISOString().replace(/[-:T]/g, '').slice(0, 14); }

// IHE PCD-01 style ventilator observation. OBX-3 = MDC code, OBX-4 = containment
// path (MDS.VMD.channel.metric), OBX-18 = equipment identifier (EUI-64).
function ventilatorOru(controlId, seq) {
    const ts = hl7ts();
    const peep = (5 + (seq % 3)).toString();
    return [
        `MSH|^~\\&|VENT^0012340000ABCDEF^EUI-64|ICU|EHK|HOSP|${ts}||ORU^R01^ORU_R01|${controlId}|P|2.6|||NE|AL||UNICODE UTF-8|||IHE_PCD_001^IHE PCD^1.3.6.1.4.1.19376.1.6.1.1.1^ISO`,
        `PID|||MRN-VENT-001^^^HOSP^MR||Patient^Ventilated^^^^^L||19650412|M`,
        `PV1||I|ICU^BED07^1`,
        `OBR|1|${controlId}^VENT^0012340000ABCDEF^EUI-64|${controlId}^VENT^0012340000ABCDEF^EUI-64|182777000^monitoring of patient^SCT|||${ts}`,
        `OBX|1||70686^MDC_DEV_SYS_VENT_MDS^MDC|1.0.0.0|||||||X|||||||0012340000ABCDEF^^0012340000ABCDEF^EUI-64`,
        `OBX|2|NM|151586^MDC_VENT_RESP_RATE_SETTING^MDC|1.1.1.1|14|2147483648^/min^MDC|||||R|||${ts}`,
        `OBX|3|NM|151980^MDC_VENT_VOL_TIDAL^MDC|1.1.1.2|450|2490^mL^MDC|||||R|||${ts}`,
        `OBX|4|NM|151976^MDC_VENT_PRESS_AWAY_END_EXP_POS^MDC|1.1.1.3|${peep}|3840^cm[H2O]^MDC|||||R|||${ts}`,
        `OBX|5|NM|151957^MDC_VENT_PRESS_AWAY_INSP_PEAK^MDC|1.1.1.4|22|3840^cm[H2O]^MDC|||||R|||${ts}`,
        `OBX|6|NM|150456^MDC_PULS_OXIM_SAT_O2^MDC|1.1.2.1|96|262688^%^MDC|90-100||||R|||${ts}`,
        `OBX|7|NM|152604^MDC_VENT_CONC_AWAY_O2_INSP^MDC|1.1.1.5|40|262688^%^MDC|||||R|||${ts}`,
    ].join('\r') + '\r';
}

// One persistent socket; resolves each send with the ACK that carries its own control ID.
class PersistentMllpClient {
    constructor(port) { this.port = port; this.buf = Buffer.alloc(0); this.waiters = []; }
    connect() {
        return new Promise((resolve, reject) => {
            this.sock = net.createConnection({ host: '127.0.0.1', port: this.port }, resolve);
            this.sock.on('error', reject);
            this.sock.on('data', (chunk) => {
                this.buf = Buffer.concat([this.buf, chunk]);
                let end;
                while ((end = this.buf.indexOf(Buffer.from([FS, CR]))) !== -1) {
                    const start = this.buf.indexOf(VB);
                    const ack = this.buf.slice(start + 1, end).toString('ascii');
                    this.buf = this.buf.slice(end + 2);
                    const w = this.waiters.shift();
                    if (w) { clearTimeout(w.timer); w.resolve(ack); }
                }
            });
        });
    }
    send(msg, timeoutMs = 8000) {
        return new Promise((resolve, reject) => {
            const timer = setTimeout(() => reject(new Error('timeout waiting for ACK')), timeoutMs);
            this.waiters.push({ resolve, timer });
            this.sock.write(frame(msg));
        });
    }
    close() { if (this.sock) this.sock.end(); }
}

function sendOnce(port, payload, { waitAck = true, timeoutMs = 5000 } = {}) {
    return new Promise((resolve) => {
        const sock = net.createConnection({ host: '127.0.0.1', port }, () => sock.write(payload));
        let buf = Buffer.alloc(0);
        const done = (v) => { clearTimeout(t); sock.destroy(); resolve(v); };
        const t = setTimeout(() => done(buf.length ? buf.toString('ascii') : null), timeoutMs);
        sock.on('data', (c) => { buf = Buffer.concat([buf, c]); if (waitAck && buf.includes(CR)) done(buf.toString('ascii')); });
        sock.on('error', () => done(null));
        if (!waitAck) setTimeout(() => done(null), 300);
    });
}

const ackCode = (ack) => (ack.match(/MSA\|(A[AER])\|([^|\r]*)/) || []).slice(1);

async function cleanupInterfacesByName(request, name) {
    const res = await request.get('/api/interfaces');
    const body = await res.json();
    const list = body.data || body.interfaces || body || [];
    for (const iface of list) {
        if (iface.name === name) {
            await request.post(`/api/runtime/interfaces/${iface.id}/deactivate`).catch(() => {});
            await request.delete(`/api/interfaces/${iface.id}`).catch(() => {});
        }
    }
}

async function findInterfaceId(request, name) {
    const res = await request.get('/api/interfaces');
    const body = await res.json();
    const list = body.data || body.interfaces || body || [];
    const hit = list.find((i) => i.name === name);
    return hit && hit.id;
}

test.describe.serial('ICU ventilator over LAN (IHE PCD-01 ORU^R01 via HL7 over TCP/IP wizard)', () => {
    test.describe.configure({ retries: 0 });
    test.setTimeout(180_000);

    let receiver, received = [], ifaceName, ifaceId, conflictName;

    test.beforeAll(async () => {
        // Local downstream "EHR/HIS" receiver for the template's http_outbound target.
        receiver = http.createServer((req, res) => {
            let body = '';
            req.on('data', (c) => { body += c; });
            req.on('end', () => { received.push({ path: req.url, body }); res.writeHead(200, { 'Content-Type': 'application/json' }); res.end('{"ok":true}'); });
        });
        await new Promise((r) => receiver.listen(0, '0.0.0.0', r));
    });

    test.afterAll(async ({ request }) => {
        if (ifaceName) await cleanupInterfacesByName(request, ifaceName);
        if (conflictName) await cleanupInterfacesByName(request, conflictName);
        if (receiver) receiver.close();
    });

    test('wizard creates a live listener; a sustained PCD-01 stream on one connection is ACKed AA per control ID and delivered downstream', async ({ page, request }) => {
        ifaceName = `PW Ventilator PCD01 ${Date.now()}`;
        await cleanupInterfacesByName(request, ifaceName);
        const recvUrl = `http://host.docker.internal:${receiver.address().port}/results`;

        await openDeviceTemplate(page);
        await page.fill('#tcf_name', ifaceName);
        await page.fill('[data-tcf-field="port"][data-tcf-section="source"]', String(MLLP_PORT));
        await page.fill('[data-tcf-field="endpoint"][data-tcf-section="target"]', recvUrl);
        await page.click('#tcf_submit');
        await page.waitForSelector('#tgVerifyModal', { state: 'visible', timeout: 15000 });

        ifaceId = await findInterfaceId(request, ifaceName);
        expect(ifaceId, 'wizard should have created the interface').toBeTruthy();

        const client = new PersistentMllpClient(MLLP_PORT);
        await client.connect();
        const failures = [];
        for (let i = 1; i <= STREAM_COUNT; i++) {
            const ctrl = `VENT${Date.now()}${String(i).padStart(3, '0')}`;
            const ack = await client.send(ventilatorOru(ctrl, i));
            const [code, echoed] = ackCode(ack);
            if (code !== 'AA' || echoed !== ctrl) failures.push({ i, ctrl, code, echoed });
            await new Promise((r) => setTimeout(r, 100)); // ~10 msg/s, faster than a real vent's 1 msg / few s
        }
        client.close();
        expect(failures, 'every message on the persistent connection must get its own AA ACK').toEqual([]);

        await expect(page.locator('#tgVerifyTitle')).toContainText('Received a message from your device', { timeout: 20000 });

        // Every message persisted and processed on the interface's own table.
        await expect.poll(async () => {
            const r = await request.get(`/api/messages/interface/${ifaceId}?limit=100`);
            const b = await r.json();
            // Only this test's ORU stream has reached the interface at this point.
            return (b.data && b.data.pagination && b.data.pagination.totalCount) || 0;
        }, { timeout: 60000, message: 'all streamed ORU messages should be stored' }).toBeGreaterThanOrEqual(STREAM_COUNT);

        // Real downstream delivery carrying the ventilator's actual values.
        await expect.poll(() => received.length, { timeout: 60000, message: 'messages should be delivered to the downstream receiver' })
            .toBeGreaterThanOrEqual(STREAM_COUNT);
        const sample = received[0].body;
        expect(sample, 'delivered payload should carry the MDC tidal volume code').toMatch(/151980|MDC_VENT_VOL_TIDAL/);
        expect(sample, 'delivered payload should carry the tidal volume value').toMatch(/450/);
    });

    test('malformed frame and a mid-frame disconnect do not take the listener down', async () => {
        // No MSH segment at all: must be rejected with AR (never AA — a device told
        // "accepted" would not resend) and must not take the listener down.
        const bad = await sendOnce(MLLP_PORT, frame('GARBAGE|not|an|hl7|message\r'));
        expect(bad, 'a non-HL7 frame must get a reply').toBeTruthy();
        expect(bad, 'a non-HL7 frame must be rejected with AR').toMatch(/MSA\|AR/);

        // Start-block + partial message, then drop the socket.
        await sendOnce(MLLP_PORT, Buffer.concat([Buffer.from([VB]), Buffer.from('MSH|^~\\&|VENT|ICU|EHK|HOSP|', 'ascii')]), { waitAck: false });

        // The very next well-formed message must still be ACKed.
        const ctrl = `VENTOK${Date.now()}`;
        const ack = await sendOnce(MLLP_PORT, frame(ventilatorOru(ctrl, 1)), { timeoutMs: 8000 });
        expect(ack, 'listener should still ACK after malformed input').toBeTruthy();
        expect(ackCode(ack)).toEqual(['AA', ctrl]);
    });

    test('a burst of concurrent connections (several devices on one listener) is fully ACKed', async () => {
        const N = 10;
        const results = await Promise.all(Array.from({ length: N }, (_, i) => {
            const ctrl = `VENTB${Date.now()}${i}`;
            return sendOnce(MLLP_PORT, frame(ventilatorOru(ctrl, i)), { timeoutMs: 10000 }).then((a) => ({ ctrl, a }));
        }));
        const bad = results.filter(({ ctrl, a }) => !a || ackCode(a)[0] !== 'AA' || ackCode(a)[1] !== ctrl);
        expect(bad.map((b) => b.ctrl), 'every concurrent device connection should get its own AA').toEqual([]);
    });

    // Must run LAST: a port conflict deliberately halts BOTH interfaces on the port.
    test('wizard on an already-used port shows the activation failure instead of silently redirecting', async ({ page, request }) => {
        conflictName = `PW Ventilator Conflict ${Date.now()}`;
        await openDeviceTemplate(page);
        await page.fill('#tcf_name', conflictName);
        await page.fill('[data-tcf-field="port"][data-tcf-section="source"]', String(MLLP_PORT));
        await page.fill('[data-tcf-field="endpoint"][data-tcf-section="target"]', 'https://his.example.org/api/results');
        await page.click('#tcf_submit');

        await page.waitForSelector('#tgVerifyModal', { state: 'visible', timeout: 15000 });
        await expect(page.locator('#tgVerifyModal')).toContainText("didn't fully start");
        await expect(page.locator('#tgVerifyModal')).toContainText(String(MLLP_PORT));
        expect(page.url(), 'must not silently land on the pipeline builder').not.toContain('pipeline-builder.html');
    });
});

async function openDeviceTemplate(page) {
    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-cat-tab', { timeout: 15000 });
    await page.locator('.tg-cat-tab', { hasText: 'Devices' }).click();
    const card = page.locator('.tg-card', { hasText: TEMPLATE_NAME });
    await expect(card).toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();
    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
}
