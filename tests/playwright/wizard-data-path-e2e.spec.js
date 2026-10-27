'use strict';
/**
 * G6 — the engine and wizard data path, end to end through the real app.
 *
 *   1. Create an interface through the wizard UI (Passthrough flow, MLLP inbound).
 *   2. Activate it and send a real HL7 message over MLLP; expect an AA acknowledgement.
 *   3. The message is stored: visible through the API with the exact content that was sent, and — when
 *      the test has database access (DATABASE_URL, set in CI) — the interface's own message table has
 *      the standard schema and the row points at stored content.
 *   4. Reprocess it from the message view in the browser and watch it finish.
 *
 * Recovery after a service restart is covered by the installer E2E (it can restart the service);
 * see installer/test/e2e-linux.sh and e2e-windows.ps1.
 */

const { test, expect } = require('./helpers/seeded');
const { sendMLLP, parseAckCode } = require('./helpers/mllp');
const { HL7_ADT_A01 } = require('./helpers/fixtures');
const { Client } = require('pg');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const DB_URL = process.env.DATABASE_URL;
const WIZARD_PORT = parseInt(process.env.G6_MLLP_PORT || '6630', 10); // inside the dev stack's published 6610-6670

// Unique control id + patient so the stored message can be recognised unambiguously.
const STAMP = Date.now();
const CONTROL_ID = `G6${STAMP}`;
const HL7 = HL7_ADT_A01.replace('MVPTEST001', CONTROL_ID).replace('SMOKE^PATIENT^MVP', `G6PATIENT${STAMP}^WIZARD^TEST`);

// Columns every interface message table must have. raw_message is NOT among them: message content
// lives in object storage and the row carries raw_content_uri (V60).
const REQUIRED_COLUMNS = [
    'id', 'message_id', 'correlation_id', 'interface_id', 'status', 'priority', 'received_at',
    'source_type', 'source_endpoint', 'source_ip', 'message_type', 'message_size', 'message_encoding',
    'processing_completed_at', 'processing_time_ms', 'error_count', 'last_error_message',
    'delivery_status', 'delivery_attempts', 'raw_content_uri', 'created_at', 'updated_at',
];

const state = { interfaceId: null, name: `PW_G6_WIZARD_${STAMP}`, messageId: null };

test.describe.serial('Wizard-created interface: send, store, reprocess', () => {

    test.afterAll(async ({ seed }) => {
        if (state.interfaceId) await seed.api.deleteInterface(state.interfaceId).catch(() => {});
    });

    test('wizard (Passthrough, MLLP inbound) creates an interface', async ({ page, seed }) => {
        test.setTimeout(120_000);
        page.on('dialog', (d) => d.accept());
        await page.goto('/interfaces.html');
        await page.waitForLoadState('domcontentloaded');
        await page.waitForTimeout(1500);

        const overlay = page.locator('#wizardModalOverlay');
        await expect(async () => {
            if (!(await overlay.isVisible())) await page.locator('button, a').filter({ hasText: /new interface|create interface/i }).first().click();
            await expect(overlay).toBeVisible({ timeout: 2000 });
        }).toPass({ timeout: 20000 });

        await overlay.locator('#interfaceName').fill(state.name);
        await overlay.locator('#sourceType').selectOption('hl7v2');
        await overlay.locator('#transformationFlow').selectOption('passthrough');

        // Inbound connector: TCP/MLLP, on a port the dev stack publishes.
        const inbound = overlay.locator('#wizardInboundConnectorContainer');
        await expect(inbound.locator('#connectorTypeSelect')).toBeVisible({ timeout: 10000 });
        await inbound.locator('#connectorTypeSelect').selectOption('tcp_mllp'); // no default is preselected
        const portField = inbound.locator('[data-field="port"]').first();
        await expect(portField).toBeVisible({ timeout: 5000 });
        await portField.fill(String(WIZARD_PORT));

        await page.locator('#wizardNext').click();
        await expect(overlay.locator('[data-step="4"]')).toBeVisible({ timeout: 10000 });

        const created = page.waitForResponse(r => r.url().includes('/api/wizard/complete') && r.request().method() === 'POST', { timeout: 30000 });
        await page.locator('#wizardFinish').click();
        const res = await created;
        expect(res.status(), await res.text().catch(() => '')).toBe(200);
        const body = await res.json();
        expect(body.success).toBe(true);
        state.interfaceId = body.interfaceId || body.data?.interfaceId || body.id;
        expect(state.interfaceId, 'wizard returned no interface id').toBeTruthy();

        // The interface really exists and is the one that was configured in the wizard.
        const got = await seed.api.getInterface(state.interfaceId);
        expect(JSON.stringify(got)).toContain(state.name);
        expect(JSON.stringify(got)).toContain(String(WIZARD_PORT));
    });

    test('activates cleanly and accepts a real MLLP message (AA)', async ({ seed }) => {
        const activation = await seed.api.activateInterface(state.interfaceId);
        expect(activation.warnings || [], 'activation warnings').toEqual([]);

        const ack = await sendMLLP('localhost', WIZARD_PORT, HL7, 15_000);
        expect(parseAckCode(ack), `ACK was: ${ack}`).toBe('AA');
        expect(ack).toContain(CONTROL_ID);
    });

    test('the message is stored with the exact content that was sent', async ({ seed }) => {
        await expect.poll(async () => (await seed.api.getMessages(state.interfaceId)).length, { timeout: 30000 }).toBeGreaterThan(0);
        const messages = await seed.api.getMessages(state.interfaceId);
        expect(messages).toHaveLength(1);
        state.messageId = messages[0].id || messages[0].message_id;

        // Message detail carries the metadata...
        const detail = await seed.api.request.get(`${BASE_URL}/api/messages/${state.messageId}`);
        expect(detail.status()).toBe(200);
        expect(JSON.stringify(await detail.json())).toContain(CONTROL_ID); // correlation id = MSH-10

        // ...and the stored content itself comes from object storage via /raw (what the viewer's Raw tab reads).
        const raw = await seed.api.request.get(`${BASE_URL}/api/messages/${state.messageId}/raw?interfaceId=${state.interfaceId}`);
        expect(raw.status()).toBe(200);
        const text = JSON.stringify(await raw.json());
        expect(text).toContain(CONTROL_ID);
        expect(text).toContain(`G6PATIENT${STAMP}`);
        expect(text).toContain('ADT^A01');
    });

    test('the interface message table has the standard schema and the row points at stored content', async ({ seed }) => {
        test.skip(!DB_URL, 'No DATABASE_URL — this test inspects the database directly (set in CI)');
        const db = new Client({ connectionString: DB_URL });
        await db.connect();
        try {
            const table = `messages_intf_${state.interfaceId.replace(/-/g, '_')}`;
            const cols = (await db.query(
                `SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=$1`, [table])).rows.map(r => r.column_name);
            expect(cols.length, `table ${table} does not exist`).toBeGreaterThan(0);
            for (const c of REQUIRED_COLUMNS) expect(cols, `missing column ${c}`).toContain(c);
            expect(cols, 'raw_message was dropped in V60').not.toContain('raw_message');

            const meta = await db.query(`SELECT 1 FROM interface_table_metadata WHERE table_name = $1`, [table]);
            expect(meta.rowCount, 'interface_table_metadata row').toBe(1);

            const rows = (await db.query(`SELECT * FROM ${table}`)).rows;
            expect(rows).toHaveLength(1);
            const row = rows[0];
            expect(row.interface_id).toBe(state.interfaceId);
            expect(row.message_type).toMatch(/ADT/);
            expect(row.message_size).toBe(Buffer.byteLength(HL7, 'utf8'));
            expect(row.raw_content_uri, 'raw content must be stored in object storage').toBeTruthy();
            expect(['received', 'processing', 'processed', 'completed', 'delivered']).toContain(row.status);
            // A store-only (Passthrough) interface has nothing to deliver, so nothing may be recorded as an error.
            // This is the assertion that caught the wizard saving its outbound step as connector type 'sink'.
            expect(row.error_count).toBe(0);
            expect(row.last_error_message).toBeNull();
        } finally {
            await db.end();
        }
    });

    test('Reprocess from the message view re-runs the message to completion', async ({ page }) => {
        test.setTimeout(120_000);
        page.on('dialog', (d) => d.accept());
        await page.goto(`/messages.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('domcontentloaded');

        const row = page.locator('tbody tr.message-row').first();
        await expect(row).toBeVisible({ timeout: 15000 });
        await row.click();
        await expect(page.locator('#messageDetailModal')).toBeVisible({ timeout: 10000 });

        const reprocess = page.locator('#reprocessBtn');
        await expect(reprocess).toBeVisible();
        const request = page.waitForResponse(r => /\/api\/messages\/.+\/reprocess/.test(r.url()) && r.request().method() === 'POST');
        await reprocess.click();
        expect((await request).status()).toBe(200);
        await expect(page.getByText(/Re-run started/i)).toBeVisible({ timeout: 5000 });

        // The UI polls until the pipeline finishes; the stored message ends in a terminal state.
        await expect.poll(async () => {
            const r = await page.request.get(`/api/messages/${state.messageId}`);
            const j = await r.json();
            const msg = (j.data && j.data.message) || j.data || j;
            return String(msg.status || '').toLowerCase();
        }, { timeout: 45000, intervals: [1000] }).toMatch(/^(processed|completed|delivered)$/);

        // Still exactly one stored message: reprocessing re-runs it, it does not duplicate it.
        const list = await page.request.get(`/api/messages/interface/${state.interfaceId}?limit=50`);
        const inner = (await list.json());
        const messages = (inner.data && inner.data.messages) || inner.messages || [];
        expect(messages).toHaveLength(1);
    });
});
