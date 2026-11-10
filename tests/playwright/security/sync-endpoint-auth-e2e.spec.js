// tests/playwright/security/sync-endpoint-auth-e2e.spec.js
//
// Partner-endpoint authentication, end to end, plus the anonymous-access sweep
// that found the gap (2026-10-19 release regression).
//
// Partner endpoints (/api/eligibility, /api/claim-status, /api/prior-auth,
// /api/pharmacy-claim) are called by external systems with NO app session.
// Each interface carries its own credentials (source_config.endpoint_auth),
// configured by an engineer in the Edit Interface modal ("Partner Endpoint
// Authentication") and verified in Go (controllers/sync_endpoint_auth.go).
//
// Partner calls here use a FRESH request context with no storage state — a
// logged-in app user is deliberately allowed through, so testing with the
// suite's default authenticated `request` fixture would prove nothing.
const { test, expect, request: pwRequest } = require('@playwright/test');
const { Client } = require('pg');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const DB_URL = process.env.DATABASE_URL;

const SYNC_270 = [
    'ISA*00*          *00*          *ZZ*PROVIDER1      *ZZ*PAYER1         *260912*1421*^*00501*889860169*0*P*:~',
    'GS*HS***260912*1421*889893012*X*005010X279A1~',
    'ST*270*4521~',
    'BHT*0022*13*ELIG0001*20260912*1200~',
    'HL*1**20*1~',
    'NM1*PR*2*PAYER1*****PI*PAYER001~',
    'HL*2*1*21*1~',
    'NM1*1P*2*ACME CLINIC*****XX*1234567890~',
    'HL*3*2*22*0~',
    'TRN*1*TRACE001~',
    'NM1*IL*1*SMITH*JANE****MI*SUB123~',
    'DMG*D8*19800101*F~',
    'EQ*30~',
    'SE*12*4521~',
    'GE*1*889893012~',
    'IEA*1*889860169~',
].join('\n') + '\n';

const PARTNER_HEADER = 'X-Payer-Key';
const PARTNER_KEY = `pw-partner-key-${Date.now()}`;

async function anonymousContext() {
    return pwRequest.newContext({ baseURL: BASE_URL, storageState: { cookies: [], origins: [] } });
}

async function partnerCheck(ctx, interfaceId, headers = {}) {
    return ctx.post(`/api/eligibility/${interfaceId}/check`, {
        headers: { 'Content-Type': 'application/edi-x12', ...headers },
        data: SYNC_270,
    });
}

async function openEditModal(page, interfaceId) {
    // The page lists one server-side page at a time, so narrow it to this interface before it can be opened.
    const got = await (await page.request.get(`/api/interfaces/${interfaceId}`)).json();
    const name = (got.interface || got).name;
    await page.goto('/interfaces.html');
    await page.waitForFunction(() => typeof interfaces !== 'undefined' && interfaces.length > 0);
    await page.locator('#interfaceSearchInput').fill(name);
    await page.waitForFunction(
        (id) => typeof interfaces !== 'undefined' && interfaces.some(i => i.id === id),
        interfaceId, { timeout: 15000 }
    );
    await page.evaluate((id) => window.showEditModal(id), interfaceId);
    await page.waitForSelector('#editModal.show', { timeout: 5000 });
    await expect(page.locator('#editEndpointAuthSection')).toBeVisible();
}

async function saveEditModal(page) {
    const done = page.waitForResponse(r => r.url().includes('/api/interfaces/') && r.request().method() === 'PUT');
    await page.locator('#editModal .modal-btn.primary').click();
    const res = await done;
    expect(res.status(), `save failed: ${await res.text()}`).toBe(200);
}

test.describe.serial('Partner endpoint authentication (per interface)', () => {
    test.describe.configure({ retries: 0 });
    let interfaceId;

    test.beforeAll(async ({ request }) => {
        const ifaceRes = await request.post('/api/interfaces', {
            data: { name: `PW Partner Auth ${Date.now()}`, messageType: '270', description: 'partner auth e2e' },
        });
        const body = await ifaceRes.json();
        expect(body.success, JSON.stringify(body)).toBeTruthy();
        interfaceId = body.interface.id;

        const pipeRes = await request.post('/api/pipelines', {
            data: {
                interface_id: interfaceId, message_type: '270', name: 'Partner Auth E2E Pipeline', pipeline_config: {},
                execution_groups: [{ steps: [
                    { step_name: 'Parse 270', step_type: 'edi.parse', sequence: 10, config: {} },
                    { step_name: 'Rebuild 270', step_type: 'edi.build', sequence: 20, config: { sourceField: 'parsedEDI', transactionSet: '270' } },
                ] }],
            },
        });
        expect(pipeRes.ok(), await pipeRes.text()).toBeTruthy();
    });

    test.afterAll(async ({ request }) => {
        if (interfaceId) await request.delete(`/api/interfaces/${interfaceId}`).catch(() => {});
    });

    test('secure by default: an interface without endpoint auth refuses partner calls', async () => {
        const anon = await anonymousContext();
        const res = await partnerCheck(anon, interfaceId, { [PARTNER_HEADER]: 'anything' });
        expect(res.status()).toBe(401);
        expect((await res.json()).error).toMatch(/not configured/);
        await anon.dispose();
    });

    test('engineer configures an API key in the Edit Interface modal; key is masked on read and encrypted at rest', async ({ page, request }) => {
        await openEditModal(page, interfaceId);
        await page.selectOption('#editEndpointAuthSectionType', 'api_key');
        await page.fill('#editEndpointAuthSection_api_key_header', PARTNER_HEADER);
        await page.fill('#editEndpointAuthSection_api_key', PARTNER_KEY);
        await saveEditModal(page);

        const got = await (await request.get(`/api/interfaces/${interfaceId}`)).json();
        const iface = got.interface || got.data || got;
        const auth = iface.sourceConfig.endpoint_auth;
        expect(auth.authentication_type).toBe('api_key');
        expect(auth.api_key_header, 'the header name must stay readable').toBe(PARTNER_HEADER);
        expect(auth.api_key, 'the key must never be returned').toBe('••••••••');

        test.skip(!DB_URL, 'No DATABASE_URL — skipping at-rest encryption check');
        const db = new Client({ connectionString: DB_URL });
        await db.connect();
        try {
            const { rows } = await db.query(`SELECT source_config->'endpoint_auth' AS a FROM interfaces WHERE id = $1`, [interfaceId]);
            expect(rows[0].a.api_key).toMatch(/^ENC:v1:/);
            expect(JSON.stringify(rows[0].a)).not.toContain(PARTNER_KEY);
        } finally { await db.end(); }
    });

    test('partner with the right key gets the real pipeline result; wrong / missing key is refused', async () => {
        const anon = await anonymousContext();
        const ok = await partnerCheck(anon, interfaceId, { [PARTNER_HEADER]: PARTNER_KEY });
        expect(ok.status(), await ok.text()).toBe(200);
        expect(await ok.text()).toContain('ST*270*');

        expect((await partnerCheck(anon, interfaceId, { [PARTNER_HEADER]: 'wrong-key' })).status()).toBe(401);
        expect((await partnerCheck(anon, interfaceId, {})).status()).toBe(401);
        expect((await partnerCheck(anon, interfaceId, { 'X-API-Key': PARTNER_KEY })).status(), 'key on the wrong header').toBe(401);
        await anon.dispose();
    });

    test('saving an unrelated change keeps the stored key (masked round trip)', async ({ page }) => {
        await openEditModal(page, interfaceId);
        await expect(page.locator('#editEndpointAuthSection_api_key')).toHaveValue('••••••••');
        await page.fill('#editInterfaceDescription', `edited ${Date.now()}`);
        await saveEditModal(page);

        const anon = await anonymousContext();
        expect((await partnerCheck(anon, interfaceId, { [PARTNER_HEADER]: PARTNER_KEY })).status()).toBe(200);
        await anon.dispose();
    });

    test('switching to "Not configured" removes access again', async ({ page }) => {
        await openEditModal(page, interfaceId);
        await page.selectOption('#editEndpointAuthSectionType', '');
        await saveEditModal(page);

        const anon = await anonymousContext();
        expect((await partnerCheck(anon, interfaceId, { [PARTNER_HEADER]: PARTNER_KEY })).status()).toBe(401);
        await anon.dispose();
    });

    test('a failed partner attempt is written to the audit log', async () => {
        test.skip(!DB_URL, 'No DATABASE_URL');
        const anon = await anonymousContext();
        await partnerCheck(anon, interfaceId, { [PARTNER_HEADER]: 'audit-probe' });
        await anon.dispose();
        const db = new Client({ connectionString: DB_URL });
        await db.connect();
        try {
            await expect.poll(async () => {
                const { rows } = await db.query(
                    `SELECT count(*)::int AS n FROM audit_logs WHERE action = 'SYNC_ENDPOINT_AUTH_FAILED' AND entity_id = $1`, [interfaceId]);
                return rows[0].n;
            }, { timeout: 10000 }).toBeGreaterThan(0);
        } finally { await db.end(); }
    });
});

// The anonymous-access sweep from the 2026-10-19 regression, kept as a guard.
// All use an all-zero / nonexistent id, so nothing real can be touched even if
// a route regressed.
test.describe('Anonymous callers are refused on formerly-open routes', () => {
    const Z = '00000000-0000-0000-0000-000000000000';
    const cases = [
        ['POST', `/api/processing/interfaces/${Z}/deactivate`],
        ['GET', '/api/cda/documents'],
        ['DELETE', `/api/cda/documents/${Z}`],
        ['GET', '/api/code-templates'],
        ['POST', '/api/code-templates'],
        ['DELETE', `/api/system/schemas/${Z}`],
        ['GET', '/api/processing/interfaces/statuses'],
        ['GET', '/api/file-parser/browse?path=/app'],
        ['POST', '/api/database/test-query'],
        ['GET', `/api/messages/${Z}/raw`],
        ['GET', `/api/messages/${Z}/parsed-content`],
        ['GET', '/api/fhir/validation-queue'],
        ['GET', '/api/hl7/message-types'],
        ['GET', '/api/edi/schema/segments'],
        ['GET', '/api/zsegments/templates'],
        ['GET', '/api/system/metrics'],
    ];
    for (const [method, path] of cases) {
        test(`${method} ${path} -> 401`, async () => {
            const anon = await anonymousContext();
            const res = await anon.fetch(path, { method, data: method === 'GET' ? undefined : {} });
            expect(res.status()).toBe(401);
            await anon.dispose();
        });
    }

    test('the liveness probe stays public', async () => {
        const anon = await anonymousContext();
        expect((await anon.get('/api/system/health')).status()).toBe(200);
        await anon.dispose();
    });

    test('a logged-in user still reaches the same routes', async ({ request }) => {
        expect((await request.get('/api/edi/schema/segments')).status()).toBe(200);
        expect((await request.get('/api/code-templates')).status()).toBe(200);
    });
});
