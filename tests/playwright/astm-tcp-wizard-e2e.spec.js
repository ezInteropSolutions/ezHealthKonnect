// tests/playwright/astm-tcp-wizard-e2e.spec.js
//
// The "Connect a Device" guided wizard's "ASTM over TCP/IP" template
// (V276__Add_ASTM_TCP_Connectivity_Types_And_Template.sql) — Phase B of the
// ASTM E1394-97 feature, reusing Phase A's own astm_framing.go handshake
// unchanged over a real net.Conn. Driven through the real "Use Template"
// click path, same discipline as device-connect-wizard-e2e.spec.js's own
// precedent for HL7-over-TCP/IP — and, unlike astm-serial-wizard-e2e.spec.js
// (which has no real/virtual serial port to test against), this template
// CAN be fully proven end-to-end with a real TCP client, the same way the
// HL7 template already is.
const { test, expect } = require('@playwright/test');
const net = require('net');

const TEMPLATE_NAME = 'ASTM over TCP/IP';
const ENQ = 0x05, ACK = 0x06, STX = 0x02, ETX = 0x03, EOT = 0x04, CR = 0x0D, LF = 0x0A;

function checksum(bytes) {
    let sum = 0;
    for (const b of bytes) sum += b;
    return (sum % 256).toString(16).toUpperCase().padStart(2, '0');
}

// A minimal real ASTM-over-TCP client: send ENQ, wait ACK, send each record
// as its own STX/ETX-framed, checksummed frame (waiting for ACK between
// frames), then EOT — the same wire shape services/connectors/astm_framing.go
// implements on the receiving end.
function sendASTMOverTCP(port, records) {
    return new Promise((resolve, reject) => {
        const sock = net.createConnection({ host: '127.0.0.1', port }, () => {
            sock.write(Buffer.from([ENQ]));
        });
        let stage = 'await_ack_for_enq';
        let frameNum = 1;
        let recordIdx = 0;
        const timeout = setTimeout(() => { sock.destroy(); reject(new Error('timeout waiting for ACK')); }, 8000);

        function sendNextFrame() {
            if (recordIdx >= records.length) {
                sock.write(Buffer.from([EOT]));
                clearTimeout(timeout);
                sock.end();
                resolve();
                return;
            }
            const record = records[recordIdx];
            const body = Buffer.from(`${frameNum}${record}\r`, 'ascii');
            const bodyPlusEtx = Buffer.concat([body, Buffer.from([ETX])]);
            const cs = checksum(bodyPlusEtx);
            const wire = Buffer.concat([Buffer.from([STX]), bodyPlusEtx, Buffer.from(cs, 'ascii'), Buffer.from([CR, LF])]);
            sock.write(wire);
            stage = 'await_ack_for_frame';
        }

        sock.on('data', (chunk) => {
            for (const byte of chunk) {
                if (stage === 'await_ack_for_enq' && byte === ACK) {
                    sendNextFrame();
                } else if (stage === 'await_ack_for_frame' && byte === ACK) {
                    recordIdx++;
                    frameNum = frameNum === 7 ? 0 : frameNum + 1;
                    sendNextFrame();
                }
            }
        });
        sock.on('error', (err) => { clearTimeout(timeout); reject(err); });
    });
}

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

async function openTemplateModal(page) {
    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-cat-tab', { timeout: 15000 });
    await page.locator('.tg-cat-tab', { hasText: 'Devices' }).click();

    const card = page.locator('.tg-card', { hasText: TEMPLATE_NAME });
    await expect(card, `${TEMPLATE_NAME} card should be visible under Devices`).toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();
    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
}

test.describe.serial('Connect a Device wizard ("ASTM over TCP/IP" template)', () => {
    test.describe.configure({ retries: 0 });

    test('Devices tab lists all three device templates', async ({ page }) => {
        await page.goto('/dashboard.html');
        await page.click('a[href="#templates"]');
        await page.waitForSelector('.tg-cat-tab', { timeout: 15000 });
        await page.locator('.tg-cat-tab', { hasText: 'Devices' }).click();

        await expect(page.locator('.tg-card', { hasText: 'HL7 over TCP/IP' })).toBeVisible({ timeout: 5000 });
        await expect(page.locator('.tg-card', { hasText: 'ASTM over Serial Port' })).toBeVisible({ timeout: 5000 });
        await expect(page.locator('.tg-card', { hasText: TEMPLATE_NAME })).toBeVisible({ timeout: 5000 });
    });

    test('Use renders a real Port Number field', async ({ page }) => {
        await openTemplateModal(page);
        const portInput = page.locator('[data-tcf-field="port"][data-tcf-section="source"]');
        await expect(portInput, 'a real Port Number input should render').toBeVisible({ timeout: 3000 });
        await expect(portInput).toHaveAttribute('data-tcf-type', 'number');
        await page.locator('#tgConfigureModal button:has-text("Cancel")').click();
    });

    test('Filling in real values creates a working connector: a real ASTM message over real TCP is received and stored', async ({ page, request }) => {
        const ifaceName = `PW ASTM TCP Wizard ${Date.now()}`;
        const port = 6616; // within the real Docker-mapped 6610-6670 range, distinct from other suites' ports

        await cleanupInterfacesByName(request, ifaceName);
        await openTemplateModal(page);

        await page.fill('#tcf_name', ifaceName);
        await page.fill('[data-tcf-field="port"][data-tcf-section="source"]', String(port));
        await page.fill('[data-tcf-field="endpoint"][data-tcf-section="target"]', 'https://his.example.org/api/results');

        await page.click('#tcf_submit');

        // A real connector gets activated (port is provided, a real required
        // field) -> the verification step should appear.
        await page.waitForSelector('#tgVerifyModal', { state: 'visible', timeout: 15000 });
        await expect(page.locator('#tgVerifyTitle')).toContainText("Waiting for your device's first message");

        // Drive a real ASTM client against the port just configured through
        // the wizard — the actual proof the collected port value reached the
        // real running astm_tcp_inbound connector.
        const records = [
            'H|\\^&|MSGCTRL-PW-1||ANALYZER-PW^1.0^SNPW001|||||LIS||P|E1394-97|20261004120000',
            'P|1||MRN-PW-001||Smith^Alice||19900101|F',
            'O|1|SPEC-PW-001||GLU|R',
            'R|1|GLU|88|mg/dL|70-110|N||F',
            'L|1|N',
        ];
        await sendASTMOverTCP(port, records);

        await expect(page.locator('#tgVerifyTitle')).toContainText('Received a message from your device', { timeout: 20000 });
        await expect(page.locator('#tgVerifyGoBtn')).toBeVisible();

        await cleanupInterfacesByName(request, ifaceName);
    });
});
