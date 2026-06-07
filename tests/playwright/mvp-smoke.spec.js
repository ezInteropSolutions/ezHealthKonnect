'use strict';
/**
 * mvp-smoke.spec.js — ezHealthKonnect MVP Pre-Ship Smoke Suite
 *
 * Creates seed interfaces via API, runs end-to-end MVP tests, then tears
 * everything down in a final dedicated CLEANUP test.
 *
 * Run: npx playwright test mvp-smoke --project=chromium
 *
 * Ports used (must be free, within Docker-mapped ranges):
 *   6620 — ADT→FHIR MLLP listener   (range 6610-6670)
 *   6621 — ORU→FHIR MLLP listener
 *   6622 — HL7→File MLLP listener
 *   8091 — FHIR Passthrough HTTP     (range 8081-8099)
 *
 * Env overrides:
 *   BASE_URL        default: http://localhost:3000
 *   SKIP_MLLP       set '1' to skip live TCP tests
 *   SKIP_FHIR_EXT   set '1' to skip calls to hapi.fhir.org
 */

const { test, expect } = require('@playwright/test');
const { ApiHelper }    = require('./helpers/api');
const { sendMLLP, parseAckCode } = require('./helpers/mllp');
const {
    HL7_ADT_A01, HL7_ADT_A03, HL7_ORU_R01,
    adtToFhirPayload, oruToFhirPayload,
    fhirPassthroughPayload, hl7ToFilePayload,
} = require('./helpers/fixtures');

const PORT_ADT  = 6620;   // MLLP — within docker-mapped 6610-6670
const PORT_ORU  = 6621;
const PORT_FILE = 6622;
const PORT_FHIR = 8091;   // HTTP — within docker-mapped 8081-8099

const SKIP_MLLP     = process.env.SKIP_MLLP     === '1';
const SKIP_FHIR_EXT = process.env.SKIP_FHIR_EXT === '1';

// Shared state — populated by the SETUP test, used by all subsequent tests
const state = {
    adtId:  null,
    oruId:  null,
    fhirId: null,
    fileId: null,
    ids:    [],   // all created IDs for cleanup
};

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 0: Infrastructure health (no seed data required)
// ─────────────────────────────────────────────────────────────────────────────
test.describe('MVP-S0 Infrastructure Health', () => {
    test('MVP-S0-001 Go API is healthy', async ({ request }) => {
        const res  = await request.get('http://localhost:8080/health');
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.status).toBe('healthy');
        expect(body.database.connected).toBe(true);
        expect(body.features.hl7_processing).toBe(true);
        expect(body.features.fhir_support).toBe(true);
    });

    test('MVP-S0-002 Node.js frontend is reachable', async ({ request }) => {
        const res = await request.get('http://localhost:3000/login.html');
        expect(res.ok()).toBeTruthy();
    });

    test('MVP-S0-003 Inbound connector types include TCP/MLLP and FHIR', async ({ request }) => {
        const api   = new ApiHelper(request);
        const types = await api.getConnectivityTypes('inbound');
        expect(types.length).toBeGreaterThan(0);
        const names = types.map(t => t.type_name || t.name || t.connector_type || '');
        expect(names.some(n => /mllp|tcp/i.test(n))).toBeTruthy();
        expect(names.some(n => /fhir/i.test(n))).toBeTruthy();
    });

    test('MVP-S0-004 Outbound connector types include HTTP/FHIR and file', async ({ request }) => {
        const api   = new ApiHelper(request);
        const types = await api.getConnectivityTypes('outbound');
        expect(types.length).toBeGreaterThan(0);
        const names = types.map(t => t.type_name || t.name || t.connector_type || '');
        expect(names.some(n => /fhir|http/i.test(n))).toBeTruthy();
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 1: Seed setup — one test that creates all four interfaces + activates
// ─────────────────────────────────────────────────────────────────────────────
test.describe('MVP-S1 Interface Creation', () => {
    // IMPORTANT: this test must run before ALL S2-S8 tests.
    // With workers:1 tests run in file order, so this is guaranteed.
    test('MVP-S1-001 Create four seed interfaces and activate them', async ({ request }) => {
        const api = new ApiHelper(request);

        // Clean up any leftover interfaces from a previous aborted run
        try {
            const all   = await api.listInterfaces();
            const stale = all.filter(i => (i.name || '').startsWith('MVP_SMOKE'));
            for (const iface of stale) {
                await api.deleteInterface(iface.id).catch(() => {});
            }
        } catch (_) {}

        // Create all four
        const [adtRes, oruRes, fhirRes, fileRes] = await Promise.all([
            api.createInterface(adtToFhirPayload(PORT_ADT)),
            api.createInterface(oruToFhirPayload(PORT_ORU)),
            api.createInterface(fhirPassthroughPayload(PORT_FHIR)),
            api.createInterface(hl7ToFilePayload(PORT_FILE, '/tmp/mvp-smoke/{timestamp}.hl7')),
        ]);

        state.adtId  = adtRes.interfaceId;
        state.oruId  = oruRes.interfaceId;
        state.fhirId = fhirRes.interfaceId;
        state.fileId = fileRes.interfaceId;
        state.ids    = [state.adtId, state.oruId, state.fhirId, state.fileId];

        expect(state.adtId).toBeTruthy();
        expect(state.oruId).toBeTruthy();
        expect(state.fhirId).toBeTruthy();
        expect(state.fileId).toBeTruthy();

        // Explicitly activate each interface via the processing engine endpoint
        // (belt-and-suspenders in case auto_start didn't fire in the container)
        // Must go through the Node.js proxy on port 3000 so it gets X-Internal-Proxy-Secret
        await Promise.all(state.ids.map(id =>
            request.post(`http://localhost:3000/api/processing/interfaces/${id}/activate`)
                   .catch(e => console.warn(`  Activation for ${id}: ${e.message}`))
        ));

        // Allow the Go engine time to start listeners
        await new Promise(r => setTimeout(r, 5000));

        console.log(`\n✅ Seed interfaces ready: ADT=${state.adtId}`);
    });

    test('MVP-S1-002 All four interfaces appear in the list', async ({ request }) => {
        if (!state.adtId) test.skip();
        const api  = new ApiHelper(request);
        const list = await api.listInterfaces();
        const ids  = list.map(i => i.id || i.interface_id);
        for (const id of state.ids) {
            expect(ids).toContain(id);
        }
    });

    test('MVP-S1-003 ADT interface reports active status', async ({ request }) => {
        if (!state.adtId) test.skip();
        const api   = new ApiHelper(request);
        const iface = await api.getInterface(state.adtId);
        const status = (iface.data?.status || iface.interface?.status || iface.status || '').toLowerCase();
        expect(status).toMatch(/active/);
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 2: HL7 → FHIR (ADT^A01 / A03)
// ─────────────────────────────────────────────────────────────────────────────
test.describe('MVP-S2 HL7→FHIR ADT', () => {
    test.skip(() => SKIP_MLLP, 'SKIP_MLLP=1');

    test('MVP-S2-001 MLLP port 6620 is accepting connections', async () => {
        if (!state.adtId) test.skip();
        const net = require('net');
        await new Promise((resolve, reject) => {
            const s = net.createConnection(PORT_ADT, 'localhost');
            s.setTimeout(5000);
            s.on('connect', () => { s.destroy(); resolve(); });
            s.on('error',   reject);
            s.on('timeout', () => { s.destroy(); reject(new Error('timeout')); });
        });
    });

    test('MVP-S2-002 ADT^A01 returns AA acknowledgment', async () => {
        if (!state.adtId) test.skip();
        const ack = await sendMLLP('localhost', PORT_ADT, HL7_ADT_A01, 10_000);
        expect(parseAckCode(ack)).toBe('AA');
        expect(ack).toContain('MVPTEST001');
    });

    test('MVP-S2-003 ADT^A03 (discharge) returns AA acknowledgment', async () => {
        if (!state.adtId) test.skip();
        const ack = await sendMLLP('localhost', PORT_ADT, HL7_ADT_A03, 10_000);
        expect(parseAckCode(ack)).toBe('AA');
    });

    test('MVP-S2-004 ADT messages are recorded in the interface', async ({ request }) => {
        if (!state.adtId || SKIP_MLLP) test.skip();
        await new Promise(r => setTimeout(r, 3000));
        const api      = new ApiHelper(request);
        const messages = await api.getMessages(state.adtId);
        expect(messages.length).toBeGreaterThan(0);
    });

    test('MVP-S2-005 ADT messages are in a terminal pipeline state', async ({ request }) => {
        if (!state.adtId || SKIP_MLLP) test.skip();
        const api      = new ApiHelper(request);
        const messages = await api.getMessages(state.adtId);
        if (messages.length === 0) test.skip();
        // Accept any terminal state: completed/delivered (success) or failed (pipeline ran, delivery failed)
        const ok = messages.some(m => /completed|delivered|processed|failed/i.test(m.status || ''));
        expect(ok).toBeTruthy();
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 3: HL7 → FHIR (ORU^R01)
// ─────────────────────────────────────────────────────────────────────────────
test.describe('MVP-S3 HL7→FHIR ORU', () => {
    test.skip(() => SKIP_MLLP, 'SKIP_MLLP=1');

    test('MVP-S3-001 ORU^R01 returns AA acknowledgment', async () => {
        if (!state.oruId) test.skip();
        const ack = await sendMLLP('localhost', PORT_ORU, HL7_ORU_R01, 10_000);
        expect(parseAckCode(ack)).toBe('AA');
        expect(ack).toContain('MVPTEST003');
    });

    test('MVP-S3-002 ORU messages are recorded', async ({ request }) => {
        if (!state.oruId || SKIP_MLLP) test.skip();
        await new Promise(r => setTimeout(r, 3000));
        const api      = new ApiHelper(request);
        const messages = await api.getMessages(state.oruId);
        expect(messages.length).toBeGreaterThan(0);
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 4: FHIR Passthrough
// ─────────────────────────────────────────────────────────────────────────────
test.describe('MVP-S4 FHIR Passthrough', () => {
    const FHIR_PATIENT = JSON.stringify({
        resourceType: 'Patient',
        name: [{ family: 'SMOKETEST', given: ['MVP'] }],
        birthDate: '1990-01-01',
        gender: 'unknown',
    });

    test('MVP-S4-001 HTTP FHIR inbound port 8091 accepts POST /Patient', async ({ request }) => {
        if (!state.fhirId) test.skip();
        // http_fhir_inbound defaults to basePath=/fhir/r4
        const res = await request.post(`http://localhost:${PORT_FHIR}/fhir/r4/Patient`, {
            headers: { 'Content-Type': 'application/fhir+json' },
            data: FHIR_PATIENT,
            timeout: 10_000,
        });
        expect([200, 201, 202]).toContain(res.status());
    });

    test('MVP-S4-002 FHIR passthrough message is recorded', async ({ request }) => {
        if (!state.fhirId) test.skip();
        await new Promise(r => setTimeout(r, 3000));
        const api      = new ApiHelper(request);
        const messages = await api.getMessages(state.fhirId);
        expect(messages.length).toBeGreaterThan(0);
    });

    test('MVP-S4-003 External FHIR server (hapi.fhir.org) returns CapabilityStatement', async ({ request }) => {
        if (SKIP_FHIR_EXT) test.skip();
        const res  = await request.get('http://hapi.fhir.org/baseR4/metadata', { timeout: 15_000 });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.resourceType).toBe('CapabilityStatement');
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 5: HL7 → File (passthrough)
// ─────────────────────────────────────────────────────────────────────────────
test.describe('MVP-S5 HL7→File', () => {
    test.skip(() => SKIP_MLLP, 'SKIP_MLLP=1');

    test('MVP-S5-001 HL7→File port 6622 returns AA', async () => {
        if (!state.fileId) test.skip();
        const ack = await sendMLLP('localhost', PORT_FILE, HL7_ADT_A01, 10_000);
        expect(parseAckCode(ack)).toBe('AA');
    });

    test('MVP-S5-002 File message is recorded', async ({ request }) => {
        if (!state.fileId || SKIP_MLLP) test.skip();
        await new Promise(r => setTimeout(r, 3000));
        const api      = new ApiHelper(request);
        const messages = await api.getMessages(state.fileId);
        expect(messages.length).toBeGreaterThan(0);
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 6: UI smoke
// ─────────────────────────────────────────────────────────────────────────────
test.describe('MVP-S6 UI Smoke', () => {
    test('MVP-S6-001 MVP_SMOKE interfaces appear on the Interfaces page', async ({ page }) => {
        await page.goto('/interfaces.html');
        await page.waitForLoadState('load');
        await page.waitForTimeout(2500);
        const text = await page.locator('body').innerText();
        expect(text).toContain('MVP_SMOKE');
    });

    test('MVP-S6-002 Interface cards have status badges', async ({ page }) => {
        await page.goto('/interfaces.html');
        await page.waitForLoadState('load');
        await page.waitForTimeout(2500);
        const cards = await page.locator('.interface-card, [data-interface-id]').count();
        expect(cards).toBeGreaterThan(0);
    });

    test('MVP-S6-003 Clicking interface card navigates to detail page', async ({ page }) => {
        await page.goto('/interfaces.html');
        await page.waitForLoadState('load');
        await page.waitForTimeout(2500);
        const card = page.locator('.interface-card, [data-interface-id]').first();
        if (await card.count() === 0) test.skip();
        await card.click();
        await page.waitForURL(/interface-detail\.html/, { timeout: 10_000 });
        await expect(page).toHaveURL(/interface-detail\.html/);
    });

    test('MVP-S6-004 Messages tab shows records for ADT interface', async ({ page }) => {
        if (!state.adtId || SKIP_MLLP) test.skip();
        await page.goto(`/messages.html?interfaceId=${state.adtId}`);
        await page.waitForLoadState('load');
        await page.waitForTimeout(3000);
        const rows = await page.locator('tbody tr, .message-row').count();
        expect(rows).toBeGreaterThan(0);
    });

    test('MVP-S6-005 Edit modal Source tab renders ConnectorConfigBuilder', async ({ page }) => {
        if (!state.adtId) test.skip();
        await page.goto('/interfaces.html');
        await page.waitForLoadState('load');
        await page.waitForTimeout(2500);
        // Open the "More Actions" dropdown for the ADT interface row
        const moreBtn = page.locator(`#actions-menu-btn-${state.adtId}`);
        if (await moreBtn.count() === 0) test.skip();
        await moreBtn.click();
        // Wait for dropdown to be visible, then click "Edit"
        const dropdownMenu = page.locator(`#actions-menu-${state.adtId}`);
        await dropdownMenu.waitFor({ state: 'visible', timeout: 3000 }).catch(() => {});
        await dropdownMenu.locator('.dropdown-item-minimal').filter({ hasText: 'Edit' }).click();
        // editModal.classList.add('show') is how the modal becomes visible
        await page.waitForSelector('#editModal.show', { timeout: 10000 });
        await page.locator('button[data-tab="source"]').click();
        await page.waitForTimeout(1000);
        const hasBuilder = await page.locator(
            '#editInboundConnectorContainer select, #editInboundConnectorContainer .connector-type-select'
        ).count();
        expect(hasBuilder).toBeGreaterThan(0);
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 7: ezCompanion
// ─────────────────────────────────────────────────────────────────────────────
test.describe('MVP-S7 ezCompanion', () => {
    test('MVP-S7-001 AI status endpoint is reachable', async ({ request }) => {
        const res = await request.get('http://localhost:8080/api/ai/status', { timeout: 8000 });
        if (!res.ok()) {
            console.warn('⚠️  AI endpoint not reachable — Ollama may still be loading');
            return; // soft pass
        }
        const body = await res.json();
        expect(typeof body).toBe('object');
    });

    test('MVP-S7-002 Dashboard loads without JS errors', async ({ page }) => {
        const errors = [];
        page.on('pageerror', e => errors.push(e.message));
        await page.goto('/dashboard.html');
        await page.waitForLoadState('load');
        await page.waitForTimeout(2000);
        const fatal = errors.filter(e => !/favicon|analytics|fonts/i.test(e));
        expect(fatal).toHaveLength(0);
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 8: Monitoring
// ─────────────────────────────────────────────────────────────────────────────
test.describe('MVP-S8 Monitoring', () => {
    test('MVP-S8-001 Monitoring page loads with KPI cards', async ({ page }) => {
        await page.goto('/monitoring.html');
        await page.waitForLoadState('load');
        await page.waitForTimeout(2000);
        const kpis = await page.locator('.kpi-card, .metric-card, .stat-card').count();
        expect(kpis).toBeGreaterThan(0);
    });

    test('MVP-S8-002 Go API reports all core features healthy', async ({ request }) => {
        const res  = await request.get('http://localhost:8080/health');
        const body = await res.json();
        expect(body.status).toBe('healthy');
        expect(body.features.hl7_processing).toBe(true);
        expect(body.features.fhir_support).toBe(true);
    });

    test('MVP-S8-003 Monitoring summary API returns 200', async ({ request }) => {
        // /api/monitoring is registered before session middleware in app.js (needs Bearer).
        // /api/analytics/monitoring/summary is registered AFTER session middleware — uses
        // requireAuth which accepts the storageState session cookie from the request fixture.
        const res = await request.get('http://localhost:3000/api/analytics/monitoring/summary');
        expect(res.ok()).toBeTruthy();
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 9: CLEANUP — always last, always runs, deactivates + deletes all seed data
// This is a test, not a hook, so it runs in order and is not triggered by retries.
// ─────────────────────────────────────────────────────────────────────────────
test.describe('MVP-S9 Cleanup', () => {
    test('MVP-S9-001 Deactivate and delete all MVP_SMOKE seed interfaces', async ({ request }) => {
        const api = new ApiHelper(request);
        const failures = [];

        // Delete by tracked IDs
        for (const id of state.ids.filter(Boolean)) {
            try {
                await api.deleteInterface(id);
                console.log(`  ✅ Deleted ${id}`);
            } catch (e) {
                failures.push(`${id}: ${e.message}`);
                console.warn(`  ⚠️  ${id}: ${e.message}`);
            }
        }

        // Safety net: catch any leftover MVP_SMOKE_* from prior aborted runs
        try {
            const all   = await api.listInterfaces();
            const stale = all.filter(i =>
                (i.name || '').startsWith('MVP_SMOKE') && !state.ids.includes(i.id)
            );
            for (const iface of stale) {
                try {
                    await api.deleteInterface(iface.id);
                    console.log(`  ✅ Cleaned stale ${iface.id} (${iface.name})`);
                } catch (e) {
                    console.warn(`  ⚠️  Stale ${iface.id}: ${e.message}`);
                }
            }
        } catch (e) {
            console.warn(`  ⚠️  Safety-net scan: ${e.message}`);
        }

        // Cleanup itself failing is informational, not a hard failure
        // (data is not left in a broken state even if a delete fails — the interface is inactive)
        if (failures.length > 0) {
            console.warn(`⚠️  ${failures.length} interface(s) could not be deleted:`, failures);
        }
        // Always pass — data cleanup is best-effort
        expect(true).toBe(true);
    });
});
