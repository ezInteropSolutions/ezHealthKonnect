// tests/playwright/device-connect-wizard-e2e.spec.js
//
// The "Connect a Device" guided wizard (V274__Device_Connect_Templates.sql,
// the "HL7 over TCP/IP" template under the new `device` category — named by
// connection TYPE, not by device brand/model, so any future instrument
// sharing the same protocol+transport reuses this same template with no
// new template needed), driven through the REAL "Use Template" click path —
// same discipline as edi-270-271-to-fhir-e2e.spec.js's own precedent — AND,
// since this feature is specifically about making a collected value reach a
// REAL running connector (not just a FHIR-mapping pipeline), a real
// TCP/MLLP client proving the whole chain: gallery -> real field input ->
// real interface + pipeline -> real activation -> real device message ->
// the verification step's "Waiting..." -> "Received" transition.
//
// This is also the permanent regression guard for two real bugs found and
// fixed this session:
//   1. interfaceTemplateSanitizer.js's mergeTemplateWithUserValues was a
//      flat, one-level merge — a dot-path field like "host_query.enabled"
//      would have been written as a literal dotted key, not nested.
//   2. tcp_mllp_inbound.go's handleHostQuery discarded an already-built,
//      valid DSR^Q03 reply whenever a LATER, query-irrelevant pipeline step
//      (e.g. results delivery to the HIS) failed afterward.
const { test, expect } = require('@playwright/test');
const net = require('net');

const TEMPLATE_NAME = 'HL7 over TCP/IP';

const MLLP_START = 0x0B, MLLP_END1 = 0x1C, MLLP_END2 = 0x0D;
function mllpFrame(msg) { return Buffer.concat([Buffer.from([MLLP_START]), Buffer.from(msg, 'ascii'), Buffer.from([MLLP_END1, MLLP_END2])]); }

function sendOru(port) {
    return new Promise((resolve, reject) => {
        const ts = new Date().toISOString().replace(/[-:T.Z]/g, '').slice(0, 14);
        const oru = `MSH|^~\\&|ANALYZER|LAB|EHK|EHK|${ts}||ORU^R01|CTRL-PW-DW-1|P|2.3.1\r` +
            `PID|1||MRN-PW-DW-001||Doe^Jane||19800101|F\r` +
            `OBR|1|ORD001|ORD001|GLU^Glucose^L|||${ts}\r` +
            `OBX|1|NM|GLU^Glucose^L||95|mg/dL|70-110|N|||F\r`;
        const sock = net.createConnection({ host: '127.0.0.1', port }, () => sock.write(mllpFrame(oru)));
        let buf = Buffer.alloc(0);
        const timeout = setTimeout(() => { sock.destroy(); reject(new Error('timeout waiting for ACK')); }, 8000);
        sock.on('data', (chunk) => {
            buf = Buffer.concat([buf, chunk]);
            if (buf.includes(MLLP_END2)) { clearTimeout(timeout); sock.end(); resolve(buf.toString('ascii')); }
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

test.describe.serial('Connect a Device wizard ("HL7 over TCP/IP" connection-type template)', () => {
    test.describe.configure({ retries: 0 });

    test('Devices tab lists the HL7 over TCP/IP template; Use renders real Port Number and bidirectional-mode fields', async ({ page }) => {
        await page.goto('/dashboard.html');
        await page.click('a[href="#templates"]');
        await page.waitForSelector('.tg-cat-tab', { timeout: 15000 });

        const devicesTab = page.locator('.tg-cat-tab', { hasText: 'Devices' });
        await expect(devicesTab, 'a Devices category tab should exist').toBeVisible({ timeout: 5000 });
        await devicesTab.click();

        const card = page.locator('.tg-card', { hasText: TEMPLATE_NAME });
        await expect(card, `${TEMPLATE_NAME} card should be visible under Devices`).toBeVisible({ timeout: 5000 });
        await card.locator('.tg-use-btn').click();
        await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });

        // The real, previously-missing piece this feature adds: actual
        // input controls rendered from required_connection_fields, not just
        // a read-only checklist and an Interface Name box.
        const portInput = page.locator('[data-tcf-field="port"][data-tcf-section="source"]');
        await expect(portInput, 'a real Port Number input should render').toBeVisible({ timeout: 3000 });
        await expect(portInput).toHaveAttribute('data-tcf-type', 'number');

        const hostQueryToggle = page.locator('[data-tcf-field="host_query.enabled"][data-tcf-section="source"]');
        await expect(hostQueryToggle, 'the bidirectional-mode checkbox should render').toBeVisible({ timeout: 3000 });
        await expect(hostQueryToggle).toHaveAttribute('data-tcf-type', 'boolean');
        await expect(hostQueryToggle).not.toBeChecked(); // template default is off

        const endpointInput = page.locator('[data-tcf-field="endpoint"][data-tcf-section="target"]');
        await expect(endpointInput, 'the results-delivery URL input should render').toBeVisible({ timeout: 3000 });

        await page.locator('#tgConfigureModal button:has-text("Cancel")').click();
    });

    test('Filling in real values creates a working connector: real TCP message delivered, verification step shows "Received"', async ({ page, request }) => {
        const ifaceName = `PW Device Wizard HL7TCP ${Date.now()}`;
        const port = 6625; // within the real docker-mapped 6610-6670 range, distinct from other suites' ports

        await cleanupInterfacesByName(request, ifaceName);
        await openTemplateModal(page);

        await page.fill('#tcf_name', ifaceName);
        await page.fill('[data-tcf-field="port"][data-tcf-section="source"]', String(port));
        await page.fill('[data-tcf-field="endpoint"][data-tcf-section="target"]', 'https://his.example.org/api/results');

        await page.click('#tcf_submit');

        // A real connector gets activated (the port+values just entered are
        // real) -> the verification step appears instead of an immediate
        // redirect, since this template has a real connector.inbound step
        // and the user actually supplied source values.
        await page.waitForSelector('#tgVerifyModal', { state: 'visible', timeout: 15000 });
        await expect(page.locator('#tgVerifyTitle')).toContainText("Waiting for your device's first message");

        // Drive a REAL mock analyzer message against the port just
        // configured through the wizard — this is the actual proof the
        // collected value reached the real running connector, not just
        // that the form submitted without error.
        const ack = await sendOru(port);
        expect(ack, 'the real connector should ACK the message').toContain('MSA|AA');

        await expect(page.locator('#tgVerifyTitle')).toContainText('Received a message from your device', { timeout: 20000 });
        await expect(page.locator('#tgVerifyGoBtn')).toBeVisible();

        await cleanupInterfacesByName(request, ifaceName);
    });

    test('Enabling bidirectional mode answers a real host-query live, even when results delivery to the (fake) HIS endpoint fails', async ({ page, request }) => {
        // Permanent regression guard for the real bug found this session:
        // a later, query-irrelevant pipeline step (results delivery)
        // failing must not discard an already-built, valid DSR^Q03 reply.
        const ifaceName = `PW Device Wizard HL7TCP HostQuery ${Date.now()}`;
        const port = 6626;

        await cleanupInterfacesByName(request, ifaceName);
        await openTemplateModal(page);

        await page.fill('#tcf_name', ifaceName);
        await page.fill('[data-tcf-field="port"][data-tcf-section="source"]', String(port));
        await page.check('[data-tcf-field="host_query.enabled"][data-tcf-section="source"]');
        // Deliberately a non-resolvable domain — the point of this test is
        // that the query still gets answered correctly despite this
        // downstream step failing.
        await page.fill('[data-tcf-field="endpoint"][data-tcf-section="target"]', 'https://his.example.org/api/results');

        await page.click('#tcf_submit');
        await page.waitForSelector('#tgVerifyModal', { state: 'visible', timeout: 15000 });

        // Skip waiting for a results message — go straight to sending a
        // real QRY^Q02 host-query against the just-activated connector.
        await page.locator('#tgVerifyModal button:has-text("Skip")').click();
        await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });

        const reply = await new Promise((resolve, reject) => {
            const ts = new Date().toISOString().replace(/[-:T.Z]/g, '').slice(0, 14);
            const qry = `MSH|^~\\&|ANALYZER|LAB|EHK|EHK|${ts}||QRY^Q02|CTRL-PW-HQ-1|P|2.3.1\r` +
                `QRD|${ts}|R|I|CTRL-PW-HQ-1|||1^RD|SAMPLE-PW-1|OTH\r`;
            const sock = net.createConnection({ host: '127.0.0.1', port }, () => sock.write(mllpFrame(qry)));
            let buf = Buffer.alloc(0);
            const timeout = setTimeout(() => { sock.destroy(); reject(new Error('timeout waiting for DSR reply')); }, 8000);
            sock.on('data', (chunk) => {
                buf = Buffer.concat([buf, chunk]);
                if (buf.includes(MLLP_END2)) { clearTimeout(timeout); sock.end(); resolve(buf.toString('ascii')); }
            });
            sock.on('error', (err) => { clearTimeout(timeout); reject(err); });
        });

        expect(reply, 'should receive a real DSR^Q03 reply, not a NACK, despite the HIS delivery step failing').toContain('DSR^Q03');
        expect(reply).toContain('SAMPLE-PW-1');
        expect(reply).not.toContain('NACK');

        await cleanupInterfacesByName(request, ifaceName);
    });
});
