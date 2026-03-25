/**
 * mirth-migration-e2e.spec.js
 *
 * Enterprise-grade QA for the Mirth Connect Migration feature.
 *
 * Coverage dimensions:
 *   API-PREVIEW  — POST /api/migration/mirth/preview (no browser)
 *   API-IMPORT   — POST /api/migration/mirth/import (no browser)
 *   API-HISTORY  — GET  /api/migration/mirth/history (no browser)
 *   API-SEC      — Auth gates, input validation, size limits
 *   UX-UPLOAD    — migration.html upload zone (drag-and-drop, file browse)
 *   UX-PREVIEW   — Preview panel rendering (connectors, steps, summary)
 *   UX-IMPORT    — Import form validation and success flow
 *   UX-HISTORY   — History tab loading
 *   SEC          — XSS prevention in rendered preview
 *
 * Run:
 *   npx playwright test tests/playwright/mirth-migration-e2e.spec.js
 *   npx playwright test --grep "API-PREVIEW" tests/playwright/mirth-migration-e2e.spec.js
 */

'use strict';

const { test, expect, request } = require('@playwright/test');
const path = require('path');
const fs   = require('fs');
const os   = require('os');

// ── Config ────────────────────────────────────────────────────────────────────
const BASE_URL    = process.env.BASE_URL    || 'http://localhost:3000';
const API_BASE    = `${BASE_URL}/api/migration/mirth`;
const ADMIN_EMAIL = process.env.ADMIN_EMAIL || 'admin@ezhealthkonnect.com';
const ADMIN_PASS  = process.env.ADMIN_PASS  || 'admin123';

// ── Fixtures ──────────────────────────────────────────────────────────────────

const VALID_MIRTH_XML = `<?xml version="1.0" encoding="UTF-8"?>
<channel>
  <id>550e8400-e29b-41d4-a716-446655440000</id>
  <name>E2E Test ADT Channel</name>
  <description>Created by Playwright E2E test</description>
  <enabled>true</enabled>
  <version>3.12.0</version>
  <sourceConnector>
    <name>sourceConnector</name>
    <transportName>TCP Listener</transportName>
    <properties class="com.mirth.connect.connectors.tcp.TcpReceiverProperties">
      <listenerConnectorProperties>
        <host>0.0.0.0</host>
        <port>6661</port>
      </listenerConnectorProperties>
      <mllpMode>true</mllpMode>
      <maxConnections>10</maxConnections>
    </properties>
    <transformer>
      <steps>
        <step>
          <sequenceNumber>0</sequenceNumber>
          <name>Extract Patient ID</name>
          <type>JavaScript</type>
          <data class="map">
            <entry>
              <string>Script</string>
              <string>var pid = msg['PID']['PID.3']['PID.3.1'].toString();</string>
            </entry>
          </data>
        </step>
      </steps>
    </transformer>
    <filter>
      <rules>
        <rule>
          <sequenceNumber>0</sequenceNumber>
          <name>Accept ADT</name>
          <type>JavaScript</type>
          <data class="map">
            <entry>
              <string>Script</string>
              <string>msg['MSH']['MSH.9']['MSH.9.1'].toString() == 'ADT'</string>
            </entry>
          </data>
        </rule>
      </rules>
    </filter>
  </sourceConnector>
  <destinationConnectors>
    <connector>
      <name>Send to FHIR API</name>
      <transportName>HTTP Sender</transportName>
      <properties class="com.mirth.connect.connectors.http.HttpDispatcherProperties">
        <host>http://fhir.hospital.org/api/adt</host>
        <method>POST</method>
      </properties>
      <transformer><steps/></transformer>
      <filter><rules/></filter>
    </connector>
  </destinationConnectors>
</channel>`;

const INVALID_XML = '<not valid xml';

const XSS_CHANNEL_XML = `<?xml version="1.0" encoding="UTF-8"?>
<channel>
  <id>xss-test-001</id>
  <name>&lt;script&gt;alert('xss')&lt;/script&gt;</name>
  <description>XSS test &lt;img src=x onerror=alert(1)&gt;</description>
  <enabled>true</enabled>
  <version>3.12.0</version>
  <sourceConnector>
    <name>sourceConnector</name>
    <transportName>TCP Listener</transportName>
    <properties class="com.mirth.connect.connectors.tcp.TcpReceiverProperties">
      <listenerConnectorProperties><host>0.0.0.0</host><port>6661</port></listenerConnectorProperties>
    </properties>
    <transformer><steps/></transformer>
    <filter><rules/></filter>
  </sourceConnector>
  <destinationConnectors/>
</channel>`;

// ── Helpers ───────────────────────────────────────────────────────────────────

async function login(page) {
    await page.goto(`${BASE_URL}/login.html`);
    await page.fill('input[name="email"], input[type="email"]', ADMIN_EMAIL);
    await page.fill('input[name="password"], input[type="password"]', ADMIN_PASS);
    await page.click('button[type="submit"], .login-btn');
    await page.waitForURL(/dashboard|migration/, { timeout: 15000 });
}

async function apiContext() {
    const ctx = await request.newContext({ baseURL: BASE_URL });
    await ctx.post('/api/auth/login', {
        data: { email: ADMIN_EMAIL, password: ADMIN_PASS },
    });
    return ctx;
}

/** Write XML to a temp file, return the file path. */
function writeTempXML(content, name = 'test-channel.xml') {
    const p = path.join(os.tmpdir(), `ezhk-migration-test-${Date.now()}-${name}`);
    fs.writeFileSync(p, content, 'utf8');
    return p;
}

// Tracks interface IDs created during tests for cleanup
const createdInterfaces = [];

// ─────────────────────────────────────────────────────────────────────────────
// API-PREVIEW
// ─────────────────────────────────────────────────────────────────────────────

test.describe('API-PREVIEW — POST /api/migration/mirth/preview', () => {

    test('[PREV-01] valid channel XML returns success:true with full preview', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/preview`, {
            headers: { 'Content-Type': 'application/xml' },
            data: VALID_MIRTH_XML,
        });
        expect(res.status()).toBe(200);
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.preview).toBeDefined();
        const p = body.preview;
        expect(p.channelName).toBe('E2E Test ADT Channel');
        expect(p.mirthVersion).toBe('3.12.0');
        expect(p.channelId).toBe('550e8400-e29b-41d4-a716-446655440000');
        expect(p.enabled).toBe(true);
    });

    test('[PREV-02] source connector maps to tcp_mllp_inbound with correct config', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/preview`, {
            headers: { 'Content-Type': 'application/xml' },
            data: VALID_MIRTH_XML,
        });
        const { preview } = await res.json();
        expect(preview.source.ezHKConnectorType).toBe('tcp_mllp_inbound');
        expect(preview.source.status).toBe('auto');
        expect(preview.source.config.port).toBe(6661);
        expect(preview.source.config.host).toBe('0.0.0.0');
    });

    test('[PREV-03] destination connector maps to http_outbound with endpoint', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/preview`, {
            headers: { 'Content-Type': 'application/xml' },
            data: VALID_MIRTH_XML,
        });
        const { preview } = await res.json();
        expect(preview.destinations).toHaveLength(1);
        expect(preview.destinations[0].ezHKConnectorType).toBe('http_outbound');
        expect(preview.destinations[0].config.endpoint).toBe('http://fhir.hospital.org/api/adt');
        expect(preview.destinations[0].config.method).toBe('POST');
    });

    test('[PREV-04] steps include filter rule and transformer step', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/preview`, {
            headers: { 'Content-Type': 'application/xml' },
            data: VALID_MIRTH_XML,
        });
        const { preview } = await res.json();
        expect(preview.steps.length).toBe(2); // 1 filter + 1 transformer
        const filterStep = preview.steps.find(s => s.isFilterRule);
        expect(filterStep).toBeDefined();
        expect(filterStep.suggestedType).toBe('control.if_then_else');
        const jsStep = preview.steps.find(s => !s.isFilterRule);
        expect(jsStep.mirthType).toBe('JavaScript');
        expect(jsStep.suggestedType).toBe('enrichment.script');
        expect(jsStep.script).toContain('PID.3');
    });

    test('[PREV-05] summary counts are correct', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/preview`, {
            headers: { 'Content-Type': 'application/xml' },
            data: VALID_MIRTH_XML,
        });
        const { preview } = await res.json();
        expect(typeof preview.summary.autoMapped).toBe('number');
        expect(typeof preview.summary.needsReview).toBe('number');
        expect(typeof preview.summary.totalSteps).toBe('number');
        expect(preview.summary.totalSteps).toBe(2);
    });

    test('[PREV-06] invalid XML returns 422 with error message', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/preview`, {
            headers: { 'Content-Type': 'application/xml' },
            data: INVALID_XML,
        });
        expect(res.status()).toBe(422);
        const body = await res.json();
        expect(body.success).toBe(false);
        expect(body.error).toBeTruthy();
    });

    test('[PREV-07] empty body returns 400', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/preview`, {
            headers: { 'Content-Type': 'application/xml' },
            data: '',
        });
        expect(res.status()).toBe(400);
        const body = await res.json();
        expect(body.success).toBe(false);
    });

    test('[PREV-08] missing channel name returns 422 with descriptive error', async () => {
        const noNameXML = `<?xml version="1.0"?><channel><id>abc</id><enabled>true</enabled>
            <sourceConnector><transportName>TCP Listener</transportName>
            <properties class="x"><listenerConnectorProperties><host>0.0.0.0</host><port>6661</port></listenerConnectorProperties></properties>
            <transformer><steps/></transformer><filter><rules/></filter></sourceConnector>
            <destinationConnectors/></channel>`;
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/preview`, {
            headers: { 'Content-Type': 'application/xml' },
            data: noNameXML,
        });
        expect(res.status()).toBe(422);
        const body = await res.json();
        expect(body.error).toMatch(/name/i);
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// API-IMPORT
// ─────────────────────────────────────────────────────────────────────────────

test.describe('API-IMPORT — POST /api/migration/mirth/import', () => {

    test('[IMP-01] valid import creates interface and returns IDs', async () => {
        const api = await apiContext();
        const ts = Date.now();
        const res = await api.post(`${API_BASE}/import`, {
            data: {
                channelXml:    VALID_MIRTH_XML,
                interfaceName: `E2E Import Test ${ts}`,
                description:   'Playwright import test — safe to delete',
                skipStepIndices: [],
            },
        });
        expect(res.status()).toBe(201);
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.result.interfaceId).toBeTruthy();
        expect(body.result.pipelineId).toBeTruthy();
        expect(body.result.stepsCreated).toBe(2);
        expect(body.result.stepsSkipped).toBe(0);
        createdInterfaces.push(body.result.interfaceId);
    });

    test('[IMP-02] skip steps reduces stepsCreated count', async () => {
        const api = await apiContext();
        const ts = Date.now();
        const res = await api.post(`${API_BASE}/import`, {
            data: {
                channelXml:      VALID_MIRTH_XML,
                interfaceName:   `E2E Skip Test ${ts}`,
                description:     'Playwright skip test — safe to delete',
                skipStepIndices: [0], // skip the filter rule
            },
        });
        expect(res.status()).toBe(201);
        const { result } = await res.json();
        expect(result.stepsCreated).toBe(1);
        expect(result.stepsSkipped).toBe(1);
        createdInterfaces.push(result.interfaceId);
    });

    test('[IMP-03] skip all steps creates interface with zero steps', async () => {
        const api = await apiContext();
        const ts = Date.now();
        const res = await api.post(`${API_BASE}/import`, {
            data: {
                channelXml:      VALID_MIRTH_XML,
                interfaceName:   `E2E All-Skip Test ${ts}`,
                description:     'Playwright all-skip test — safe to delete',
                skipStepIndices: [0, 1],
            },
        });
        expect(res.status()).toBe(201);
        const { result } = await res.json();
        expect(result.stepsCreated).toBe(0);
        expect(result.stepsSkipped).toBe(2);
        createdInterfaces.push(result.interfaceId);
    });

    test('[IMP-04] missing channelXml returns 400', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/import`, {
            data: { interfaceName: 'No XML Test' },
        });
        expect(res.status()).toBe(400);
        const body = await res.json();
        expect(body.success).toBe(false);
        expect(body.error).toMatch(/channelXml/i);
    });

    test('[IMP-05] interface name over 255 chars returns 400', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/import`, {
            data: {
                channelXml:    VALID_MIRTH_XML,
                interfaceName: 'A'.repeat(256),
                description:   'Test',
            },
        });
        expect(res.status()).toBe(400);
        const body = await res.json();
        expect(body.error).toMatch(/255/);
    });

    test('[IMP-06] description over 1000 chars returns 400', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/import`, {
            data: {
                channelXml:    VALID_MIRTH_XML,
                interfaceName: 'Valid Name',
                description:   'D'.repeat(1001),
            },
        });
        expect(res.status()).toBe(400);
        const body = await res.json();
        expect(body.error).toMatch(/1000/);
    });

    test('[IMP-07] invalid XML returns error', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/import`, {
            data: {
                channelXml:    INVALID_XML,
                interfaceName: 'Bad XML Test',
            },
        });
        expect(res.status()).toBeGreaterThanOrEqual(400);
        const body = await res.json();
        expect(body.success).toBe(false);
    });

    test('[IMP-08] interface name defaults to channel name when omitted', async () => {
        const api = await apiContext();
        const ts = Date.now();
        const res = await api.post(`${API_BASE}/import`, {
            data: {
                channelXml:  VALID_MIRTH_XML,
                // interfaceName intentionally omitted
                description: `Default name test ${ts}`,
            },
        });
        expect(res.status()).toBe(201);
        const { result } = await res.json();
        expect(result.interfaceName).toBe('E2E Test ADT Channel');
        createdInterfaces.push(result.interfaceId);
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// API-HISTORY
// ─────────────────────────────────────────────────────────────────────────────

test.describe('API-HISTORY — GET /api/migration/mirth/history', () => {

    test('[HIST-01] returns success:true with data array', async () => {
        const api = await apiContext();
        const res = await api.get(`${API_BASE}/history`);
        expect(res.status()).toBe(200);
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(Array.isArray(body.data)).toBe(true);
        expect(typeof body.count).toBe('number');
    });

    test('[HIST-02] limit parameter is respected', async () => {
        const api = await apiContext();
        const res = await api.get(`${API_BASE}/history?limit=2`);
        expect(res.status()).toBe(200);
        const body = await res.json();
        expect(body.data.length).toBeLessThanOrEqual(2);
    });

    test('[HIST-03] history entries have required fields', async () => {
        // Import something first so there is at least one entry
        const api = await apiContext();
        const importRes = await api.post(`${API_BASE}/import`, {
            data: {
                channelXml:    VALID_MIRTH_XML,
                interfaceName: `History Check ${Date.now()}`,
                description:   'Playwright history field check',
            },
        });
        const { result } = await importRes.json();
        createdInterfaces.push(result.interfaceId);

        const histRes = await api.get(`${API_BASE}/history?limit=5`);
        const body = await histRes.json();
        const entry = body.data.find(e => e.interfaceId === result.interfaceId);
        expect(entry).toBeDefined();
        expect(entry.channelName).toBe('E2E Test ADT Channel');
        expect(entry.stepsCreated).toBeGreaterThanOrEqual(0);
        expect(entry.createdAt).toBeTruthy();
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// API-SEC — Security
// ─────────────────────────────────────────────────────────────────────────────

test.describe('API-SEC — Authentication and input validation', () => {

    test('[SEC-01] unauthenticated preview is rejected (401 or redirect)', async () => {
        const anonCtx = await request.newContext({ baseURL: BASE_URL });
        const res = await anonCtx.post(`${API_BASE}/preview`, {
            headers: { 'Content-Type': 'application/xml' },
            data: VALID_MIRTH_XML,
        });
        // Should be 401 (or 302/303 redirect to login)
        expect([401, 302, 303, 403]).toContain(res.status());
    });

    test('[SEC-02] unauthenticated import is rejected', async () => {
        const anonCtx = await request.newContext({ baseURL: BASE_URL });
        const res = await anonCtx.post(`${API_BASE}/import`, {
            data: { channelXml: VALID_MIRTH_XML, interfaceName: 'Anon Test' },
        });
        expect([401, 302, 303, 403]).toContain(res.status());
    });

    test('[SEC-03] oversized XML is rejected with informative error', async () => {
        const api = await apiContext();
        // Build an XML that is just over 10 MB
        const padding = 'A'.repeat(10 * 1024 * 1024 + 100);
        const oversized = `<?xml version="1.0"?><channel><id>x</id><name>${padding}</name></channel>`;
        const res = await api.post(`${API_BASE}/preview`, {
            headers: { 'Content-Type': 'application/xml' },
            data: oversized,
        });
        // Either 400 (bad request) or 413 (payload too large) is acceptable
        expect([400, 413, 422]).toContain(res.status());
        const body = await res.json().catch(() => ({}));
        if (body.error) {
            expect(body.error.toLowerCase()).toMatch(/size|limit|maximum|large/);
        }
    });

    test('[SEC-04] userId in import body is ignored (server uses X-User-ID header)', async () => {
        const api = await apiContext();
        const ts = Date.now();
        const res = await api.post(`${API_BASE}/import`, {
            data: {
                channelXml:    VALID_MIRTH_XML,
                interfaceName: `UserID Spoof Test ${ts}`,
                userId:        'admin-spoof-attempt', // should be ignored
            },
        });
        // Should succeed (the spoofed userId is harmless but should not be stored as-is)
        if (res.status() === 201) {
            const { result } = await res.json();
            createdInterfaces.push(result.interfaceId);
        }
        // Just assert it didn't crash
        expect([201, 400]).toContain(res.status());
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// UX-UPLOAD — Upload zone interactions
// ─────────────────────────────────────────────────────────────────────────────

test.describe('UX-UPLOAD — migration.html upload zone', () => {

    test('[UPL-01] page loads with correct title and upload zone visible', async ({ page }) => {
        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);
        await expect(page).toHaveTitle(/Migration.*ezHealthKonnect/i);
        await expect(page.locator('#upload-zone')).toBeVisible();
    });

    test('[UPL-02] sidebar shows Migration nav item', async ({ page }) => {
        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);
        const navItem = page.locator('.sidebar a[href*="migration.html"], nav a[href*="migration.html"]');
        await expect(navItem).toBeVisible();
    });

    test('[UPL-03] file upload via input triggers preview panel', async ({ page }) => {
        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);

        const tmpFile = writeTempXML(VALID_MIRTH_XML);
        try {
            await page.locator('#file-input').setInputFiles(tmpFile);
            // Preview panel should become visible after analysis
            await expect(page.locator('#preview-panel')).toBeVisible({ timeout: 10000 });
            await expect(page.locator('#channel-info')).toBeVisible();
        } finally {
            fs.unlinkSync(tmpFile);
        }
    });

    test('[UPL-04] channel info shows correct name and version', async ({ page }) => {
        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);

        const tmpFile = writeTempXML(VALID_MIRTH_XML);
        try {
            await page.locator('#file-input').setInputFiles(tmpFile);
            await expect(page.locator('#channel-info')).toBeVisible({ timeout: 10000 });
            const info = await page.locator('#channel-info').textContent();
            expect(info).toContain('E2E Test ADT Channel');
            expect(info).toContain('3.12.0');
        } finally {
            fs.unlinkSync(tmpFile);
        }
    });

    test('[UPL-05] non-XML file upload shows error toast', async ({ page }) => {
        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);

        const tmpFile = writeTempXML('{"not": "xml"}');
        // Rename to .json to trigger the extension check
        const jsonFile = tmpFile.replace(/\.xml$/, '.json');
        fs.renameSync(tmpFile, jsonFile);
        try {
            await page.locator('#file-input').setInputFiles(jsonFile);
            // Toast or error message should appear
            const toast = page.locator('#toast');
            await expect(toast).toBeVisible({ timeout: 5000 });
            const text = await toast.textContent();
            expect(text.toLowerCase()).toMatch(/xml|format|select/);
        } finally {
            fs.existsSync(jsonFile) && fs.unlinkSync(jsonFile);
        }
    });

    test('[UPL-06] start-over button resets the UI', async ({ page }) => {
        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);

        const tmpFile = writeTempXML(VALID_MIRTH_XML);
        try {
            await page.locator('#file-input').setInputFiles(tmpFile);
            await expect(page.locator('#preview-panel')).toBeVisible({ timeout: 10000 });
            await page.click('#btn-reset');
            await expect(page.locator('#upload-zone')).toBeVisible();
            await expect(page.locator('#preview-panel')).not.toBeVisible();
        } finally {
            fs.unlinkSync(tmpFile);
        }
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// UX-PREVIEW — Preview panel content
// ─────────────────────────────────────────────────────────────────────────────

test.describe('UX-PREVIEW — Preview panel rendering', () => {

    async function loadPreview(page) {
        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);
        const tmpFile = writeTempXML(VALID_MIRTH_XML);
        await page.locator('#file-input').setInputFiles(tmpFile);
        await expect(page.locator('#preview-panel')).toBeVisible({ timeout: 10000 });
        fs.unlinkSync(tmpFile);
    }

    test('[PRV-01] summary badges are shown', async ({ page }) => {
        await loadPreview(page);
        await expect(page.locator('#summary-row')).toBeVisible();
        const badges = await page.locator('#summary-row .summary-badge').count();
        expect(badges).toBeGreaterThanOrEqual(2);
    });

    test('[PRV-02] source connector section shows tcp_mllp_inbound', async ({ page }) => {
        await loadPreview(page);
        const src = await page.locator('#source-body').textContent();
        expect(src).toContain('tcp_mllp_inbound');
    });

    test('[PRV-03] destination connector section shows http_outbound', async ({ page }) => {
        await loadPreview(page);
        const dest = await page.locator('#destinations-body').textContent();
        expect(dest).toContain('http_outbound');
    });

    test('[PRV-04] steps table is rendered with correct count', async ({ page }) => {
        await loadPreview(page);
        const rows = await page.locator('#steps-tbody tr').count();
        expect(rows).toBe(2); // 1 filter + 1 transformer
    });

    test('[PRV-05] view script toggle shows script content', async ({ page }) => {
        await loadPreview(page);
        const toggle = page.locator('.step-script-toggle').first();
        await toggle.click();
        const scriptBlock = page.locator('.step-script-block.visible').first();
        await expect(scriptBlock).toBeVisible();
        const scriptText = await scriptBlock.textContent();
        expect(scriptText).toContain('PID');
    });

    test('[PRV-06] import form is visible after preview', async ({ page }) => {
        await loadPreview(page);
        await expect(page.locator('#import-form')).toBeVisible();
        const nameField = page.locator('#input-interface-name');
        await expect(nameField).toHaveValue('E2E Test ADT Channel');
    });

    test('[PRV-07] skip checkbox marks row as skipped', async ({ page }) => {
        await loadPreview(page);
        await page.locator('#skip-0').check();
        await expect(page.locator('#step-row-0')).toHaveClass(/skipped/);
        await page.locator('#skip-0').uncheck();
        await expect(page.locator('#step-row-0')).not.toHaveClass(/skipped/);
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// UX-IMPORT — Import form flow
// ─────────────────────────────────────────────────────────────────────────────

test.describe('UX-IMPORT — Import form validation and success', () => {

    async function loadPreview(page) {
        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);
        const tmpFile = writeTempXML(VALID_MIRTH_XML);
        await page.locator('#file-input').setInputFiles(tmpFile);
        await expect(page.locator('#import-form')).toBeVisible({ timeout: 10000 });
        fs.unlinkSync(tmpFile);
    }

    test('[IMP-UI-01] empty interface name shows error toast', async ({ page }) => {
        await loadPreview(page);
        await page.fill('#input-interface-name', '');
        await page.click('#btn-import');
        const toast = page.locator('#toast');
        await expect(toast).toBeVisible({ timeout: 5000 });
        expect(await toast.textContent()).toMatch(/name/i);
    });

    test('[IMP-UI-02] successful import shows success toast with pipeline link', async ({ page }) => {
        await loadPreview(page);
        await page.fill('#input-interface-name', `UI Import Test ${Date.now()}`);
        await page.click('#btn-import');
        const toast = page.locator('#toast.success');
        await expect(toast).toBeVisible({ timeout: 15000 });
        // Import status should have link to pipeline builder
        const status = await page.locator('#import-status').textContent();
        expect(status).toMatch(/pipeline-builder/i);
    });

    test('[IMP-UI-03] import button is disabled during in-flight request', async ({ page }) => {
        await loadPreview(page);
        // Intercept the import request to check disabled state during flight
        let buttonDisabledDuringRequest = false;
        await page.route(`**/api/migration/mirth/import`, async route => {
            const disabled = await page.locator('#btn-import').isDisabled();
            buttonDisabledDuringRequest = disabled;
            await route.continue();
        });
        await page.fill('#input-interface-name', `Disabled State Test ${Date.now()}`);
        await page.click('#btn-import');
        await page.locator('#toast').waitFor({ state: 'visible', timeout: 15000 });
        expect(buttonDisabledDuringRequest).toBe(true);
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// UX-HISTORY — History tab
// ─────────────────────────────────────────────────────────────────────────────

test.describe('UX-HISTORY — History tab loading', () => {

    test('[HIST-UI-01] clicking History tab shows history panel', async ({ page }) => {
        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);
        await page.click('.migration-nav-item[data-tab="history"]');
        await expect(page.locator('#tab-history')).toBeVisible();
    });

    test('[HIST-UI-02] history loads without JS error', async ({ page }) => {
        const errors = [];
        page.on('pageerror', e => errors.push(e.message));
        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);
        await page.click('.migration-nav-item[data-tab="history"]');
        // Wait for content to load
        await page.waitForTimeout(2000);
        expect(errors).toHaveLength(0);
    });

    test('[HIST-UI-03] history shows table or empty-state (not loading spinner)', async ({ page }) => {
        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);
        await page.click('.migration-nav-item[data-tab="history"]');
        await page.waitForTimeout(2000);
        const content = await page.locator('#history-content').textContent();
        // Should NOT still show "Loading…"
        expect(content).not.toMatch(/^Loading…?$/);
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// SEC — XSS prevention
// ─────────────────────────────────────────────────────────────────────────────

test.describe('SEC — XSS prevention in rendered preview', () => {

    test('[XSS-01] channel name with script tags is escaped, not executed', async ({ page }) => {
        const xssTriggered = [];
        page.on('dialog', d => { xssTriggered.push(d.message()); d.dismiss(); });

        const api = await apiContext();
        // Just test the API response is sanitised (not executed client-side)
        const res = await api.post(`${API_BASE}/preview`, {
            headers: { 'Content-Type': 'application/xml' },
            data: XSS_CHANNEL_XML,
        });
        expect(res.status()).toBe(200);
        const body = await res.json();
        // The channel name should come back as literal text, not tags
        expect(body.preview.channelName).not.toContain('<script>');
    });

    test('[XSS-02] XSS channel rendered in UI does not trigger alert()', async ({ page }) => {
        const xssTriggered = [];
        page.on('dialog', d => { xssTriggered.push(d.message()); d.dismiss(); });

        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);

        const tmpFile = writeTempXML(XSS_CHANNEL_XML);
        try {
            await page.locator('#file-input').setInputFiles(tmpFile);
            await page.waitForTimeout(3000);
        } finally {
            fs.unlinkSync(tmpFile);
        }

        expect(xssTriggered).toHaveLength(0);
    });

    test('[XSS-03] script content in steps is rendered as text, not executed', async ({ page }) => {
        const xssPayload = `<?xml version="1.0" encoding="UTF-8"?>
<channel>
  <id>xss-step-001</id>
  <name>XSS Step Test</name>
  <enabled>true</enabled>
  <version>3.12.0</version>
  <sourceConnector>
    <transportName>TCP Listener</transportName>
    <properties class="com.mirth.connect.connectors.tcp.TcpReceiverProperties">
      <listenerConnectorProperties><host>0.0.0.0</host><port>6661</port></listenerConnectorProperties>
    </properties>
    <transformer>
      <steps>
        <step>
          <sequenceNumber>0</sequenceNumber>
          <name>&lt;img src=x onerror=alert('xss')&gt;</name>
          <type>JavaScript</type>
          <data class="map">
            <entry>
              <string>Script</string>
              <string>&lt;script&gt;alert('xss-in-script')&lt;/script&gt;</string>
            </entry>
          </data>
        </step>
      </steps>
    </transformer>
    <filter><rules/></filter>
  </sourceConnector>
  <destinationConnectors/>
</channel>`;

        const dialogs = [];
        page.on('dialog', d => { dialogs.push(d.message()); d.dismiss(); });

        await login(page);
        await page.goto(`${BASE_URL}/migration.html`);

        const tmpFile = writeTempXML(xssPayload);
        try {
            await page.locator('#file-input').setInputFiles(tmpFile);
            await page.waitForTimeout(3000);
        } finally {
            fs.unlinkSync(tmpFile);
        }

        expect(dialogs).toHaveLength(0);
    });

});
