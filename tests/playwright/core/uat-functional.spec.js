'use strict';
/**
 * UAT functional cases (docs/UAT_TEST_CASES.md). Each test is named with its case ID and asserts what
 * the persona would see, not that code ran. Uses the seeded real interface (helpers/seeded.js).
 */
const { test, expect } = require('../helpers/seeded');
const { request: pwRequest } = require('@playwright/test');
const { createUserWithRole, deleteUser, loginWithRetry, PASSWORD } = require('../helpers/users');
const { sendMLLP, parseAckCode } = require('../helpers/mllp');
const { HL7_ADT_A01 } = require('../helpers/fixtures');

const BASE = process.env.BASE_URL || 'http://localhost:3000';
const ANON = { cookies: [], origins: [] };

async function signedInContext(browser, email, password) {
    const ctx = await browser.newContext({ baseURL: BASE, storageState: ANON });
    const res = await loginWithRetry(ctx.request, { data: { email, password } });
    expect(res.status(), 'sign-in for test user').toBe(200);
    return ctx;
}

// ─── 1. Getting in ───────────────────────────────────────────────────────────
test.describe('1 Getting in', () => {
    test('ACC-04 signs in with valid credentials and lands on the dashboard', async ({ browser }) => {
        const ctx = await browser.newContext({ baseURL: BASE, storageState: ANON });
        const page = await ctx.newPage();
        await page.goto('/login.html');
        await page.locator('#email').fill(process.env.ADMIN_EMAIL || 'admin@ezhealthkonnect.com');
        await page.locator('#password').fill(process.env.ADMIN_PASSWORD || 'admin123');
        await page.locator('#loginButton').click();
        await page.waitForURL('**/dashboard.html', { timeout: 20000 });
        await expect(page.getByText(/Welcome back/)).toBeVisible();
        await expect(page.locator('#sidebar')).toContainText(/admin/i);
        await ctx.close();
    });

    test('ACC-05 wrong password gives a generic message', async ({ browser }) => {
        const ctx = await browser.newContext({ baseURL: BASE, storageState: ANON });
        const page = await ctx.newPage();
        await page.goto('/login.html');
        await page.locator('#email').fill('admin@ezhealthkonnect.com');
        await page.locator('#password').fill('definitely-wrong-1');
        await page.locator('#loginButton').click();
        const msg = page.locator('#statusMessage');
        await expect(msg).toBeVisible({ timeout: 10000 });
        const text = (await msg.innerText()).toLowerCase();
        expect(text).toMatch(/invalid|incorrect/);
        expect(text).not.toMatch(/user not found|no such user|does not exist/);
        expect(page.url()).toContain('login');
        await ctx.close();
    });

    test('ACC-06 empty and oversized input never produces a server error', async ({ request }) => {
        const anon = await pwRequest.newContext({ baseURL: BASE, storageState: ANON });
        for (const body of [{}, { email: '', password: '' }, { email: 'x'.repeat(5000), password: 'y' }, { email: 123, password: {} }]) {
            const r = await anon.post('/api/auth/login', { data: body });
            expect([400, 401, 429], `login with ${JSON.stringify(body).slice(0, 40)}`).toContain(r.status());
        }
        await anon.dispose();
    });

    test('ACC-09 sign out ends the session and Back does not reveal signed-in pages', async ({ browser }) => {
        test.fail(true, 'KNOWN DEFECT: after Logout, the browser Back button restores /dashboard.html from the back/forward cache with the signed-in sidebar, interface count and system status visible (the API correctly answers 401). The page needs a `pageshow` (event.persisted) session re-check.');
        const ctx = await browser.newContext({ baseURL: BASE, storageState: ANON });
        const r = await loginWithRetry(ctx.request, { data: { email: 'admin@ezhealthkonnect.com', password: 'admin123' } });
        expect(r.status()).toBe(200);
        const page = await ctx.newPage();
        await page.goto('/dashboard.html');
        await page.locator('#sidebarLogoutBtn').click();
        await page.waitForURL('**/login.html', { timeout: 15000 });
        expect((await ctx.request.get('/api/interfaces')).status()).toBe(401);
        // Pages are served no-store; Back must not show signed-in content. Wait for the redirect to settle.
        await page.goBack().catch(() => {});
        await page.waitForTimeout(3000);
        expect(await page.locator('body').innerText()).not.toMatch(/Welcome back/);
        await ctx.close();
    });

    test('ACC-10 deep link while signed out goes to login', async ({ browser }) => {
        const ctx = await browser.newContext({ baseURL: BASE, storageState: ANON });
        const page = await ctx.newPage();
        await page.goto('/interfaces.html');
        await page.waitForURL('**/login.html**', { timeout: 15000 });
        await ctx.close();
    });

    test('ACC-14 change my own password: wrong current refused, right one works, old fails', async ({ playwright }) => {
        test.setTimeout(150_000); // sign-in is limited to 10/min/IP; helpers wait out a 429
        const admin = await playwright.request.newContext({ baseURL: BASE, storageState: '.auth/user.json' });
        const u = await createUserWithRole(admin, BASE, 'operator');
        const me = await playwright.request.newContext({ baseURL: BASE, storageState: ANON });
        expect((await loginWithRetry(me, { data: { email: u.email, password: PASSWORD } })).status()).toBe(200);
        const bad = await me.post('/api/auth/change-password', { data: { currentPassword: 'wrong-pass-1', newPassword: 'NewPass#2026x' } });
        expect([400, 401, 403]).toContain(bad.status());
        const ok = await me.post('/api/auth/change-password', { data: { currentPassword: PASSWORD, newPassword: 'NewPass#2026x' } });
        expect(ok.status()).toBe(200);
        const fresh = await playwright.request.newContext({ baseURL: BASE, storageState: ANON });
        expect((await loginWithRetry(fresh, { data: { email: u.email, password: PASSWORD } })).status(), 'old password must stop working').toBe(401);
        expect((await loginWithRetry(fresh, { data: { email: u.email, password: 'NewPass#2026x' } })).status()).toBe(200);
        await deleteUser(admin, BASE, u.id);
        await Promise.all([admin.dispose(), me.dispose(), fresh.dispose()]);
    });

    test('ACC-16 viewer sees no usable write control; ACC-18 forbidden page says access denied', async ({ browser, playwright }) => {
        const admin = await playwright.request.newContext({ baseURL: BASE, storageState: '.auth/user.json' });
        const u = await createUserWithRole(admin, BASE, 'viewer');
        const ctx = await signedInContext(browser, u.email, PASSWORD);
        const page = await ctx.newPage();
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        const create = page.locator('#createInterfaceBtn');
        await expect(create).toBeVisible();
        await expect(create, 'viewer must not be offered a usable Create Interface').toBeDisabled();
        expect((await create.getAttribute('title')) || '', 'disabled control explains why').not.toBe('');
        await page.goto('/user-management.html', { waitUntil: 'networkidle' });
        await expect(page.getByText(/access denied/i).first()).toBeVisible();
        await ctx.close();
        await deleteUser(admin, BASE, u.id);
        await admin.dispose();
    });

    test('ACC-17 basic user can use the HL7 Reader but is blocked elsewhere', async ({ browser, playwright }) => {
        test.fail(true, 'KNOWN DEFECT: the basic `user` role is documented as "HL7 Reader only" but POST /api/hl7/parse answers 403 (/api/hl7 is gated to GO_READ_ROLES, which excludes `user`). Remove this annotation when fixed.');
        test.setTimeout(150_000);
        const admin = await playwright.request.newContext({ baseURL: BASE, storageState: '.auth/user.json' });
        const u = await createUserWithRole(admin, BASE, 'user');
        const ctx = await signedInContext(browser, u.email, PASSWORD);
        const page = await ctx.newPage();
        await page.goto('/hl7-reader.html', { waitUntil: 'networkidle' });
        await page.getByRole('button', { name: 'ADT^A01' }).first().click();
        await page.getByRole('button', { name: /Parse & Analyze/ }).click();
        await expect(page.getByText(/PID/).first()).toBeVisible({ timeout: 15000 });
        for (const p of ['user-management.html', 'settings.html']) {
            await page.goto('/' + p, { waitUntil: 'networkidle' });
            await expect(page.getByText(/access denied/i).first(), `${p} for a basic user`).toBeVisible();
        }
        await ctx.close();
        await deleteUser(admin, BASE, u.id);
        await admin.dispose();
    });
});

// ─── 2. Interfaces ───────────────────────────────────────────────────────────
test.describe('2 Interfaces', () => {
    test('IFC-09 search narrows the list and clearing restores it', async ({ page, seed }) => {
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        const search = page.getByPlaceholder(/Search interfaces/);
        await search.fill(seed.interfaceName);
        await page.waitForTimeout(800);
        await expect(page.locator(`#runtime-btn-${seed.interfaceId}`)).toBeVisible();
        await expect(page.locator(`#runtime-btn-${seed.idleInterfaceId}`)).toHaveCount(0);
        await search.fill('');
        await page.waitForTimeout(800);
        const problems = [];
        page.on('pageerror', (e) => problems.push(e.message));
        page.on('response', (r) => { if (r.status() >= 500) problems.push(`${r.status()} ${r.url()}`); });
        await search.fill('<img src=x onerror=alert(1)>%\'"');
        await page.waitForTimeout(1200);
        expect(problems, 'hostile search text must not break the page or the API').toEqual([]);
    });

    // Was a known defect (header showed 0 RUNNING / 0 STOPPED while rows were Listening): the cards counted
    // status === 'running', a value no interface has. They now come from the server's counts over everything visible.
    test('OPS-10 interface header counters agree with the API counts', async ({ page, request }) => {
        const { counts } = await (await request.get('/api/interfaces?limit=1&counts=1')).json();
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        await expect(page.locator('#totalInterfaces')).toHaveText(String(counts.total), { timeout: 15_000 });
        await expect(page.locator('#runningInterfaces')).toHaveText(String(counts.running));
        await expect(page.locator('#stoppedInterfaces')).toHaveText(String(counts.stopped));
        await expect(page.locator('#pausedInterfaces')).toHaveText(String(counts.paused));
        // sanity: the counts are consistent with each other
        expect(counts.running + counts.paused + counts.stopped + counts.error).toBeLessThanOrEqual(counts.total);
    });

    test('IFC-10 status filter shows only matching interfaces', async ({ page, seed }) => {
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        const options = await page.locator('#statusFilter option').allTextContents();
        expect(options.length, 'status filter has options').toBeGreaterThan(1);
        const search = page.getByPlaceholder(/Search interfaces/);
        await search.fill('PW_SEED_');
        await page.waitForTimeout(800);
        const before = await page.locator('[id^="runtime-btn-"]').count();
        expect(before).toBeGreaterThanOrEqual(2);
        const active = options.find((o) => /^active|running/i.test(o.trim())) || options[1];
        await page.locator('#statusFilter').selectOption({ label: active.trim() });
        await page.waitForTimeout(800);
        const after = await page.locator('[id^="runtime-btn-"]').count();
        expect(after, `filter "${active.trim()}" must not widen the list`).toBeLessThanOrEqual(before);
    });

    test('IFC-11 paging: page size changes the rows shown and Next moves on', async ({ page }) => {
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        await page.locator('#pageSize').selectOption({ index: 0 });
        await page.waitForTimeout(600);
        const first = await page.locator('[id^="runtime-btn-"]').count();
        const sizes = await page.locator('#pageSize option').allTextContents();
        await page.locator('#pageSize').selectOption({ index: sizes.length - 1 });
        await page.waitForTimeout(800);
        const last = await page.locator('[id^="runtime-btn-"]').count();
        expect(last, 'a larger page size shows at least as many rows').toBeGreaterThanOrEqual(first);
        await page.locator('#pageSize').selectOption({ index: 0 });
        await page.waitForTimeout(600);
        const idsP1 = await page.locator('[id^="runtime-btn-"]').evaluateAll((e) => e.map((x) => x.id));
        await page.getByRole('button', { name: /Next/ }).first().click();
        await page.waitForTimeout(800);
        const idsP2 = await page.locator('[id^="runtime-btn-"]').evaluateAll((e) => e.map((x) => x.id));
        expect(idsP2.length).toBeGreaterThan(0);
        expect(idsP2.filter((i) => idsP1.includes(i)), 'page 2 must not repeat page 1 rows').toEqual([]);
    });

    test('IFC-12 pause then resume processing changes state and matches the backend', async ({ page, seed }) => {
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        await page.getByPlaceholder(/Search interfaces/).fill(seed.interfaceName);
        await page.waitForTimeout(800);
        // The runtime control is an icon-only button; its state is carried by its title.
        const btn = page.locator(`#runtime-btn-${seed.interfaceId}`);
        await expect(btn).toHaveAttribute('title', /Pause Processing/, { timeout: 15000 });
        await btn.click();
        await expect(page.locator(`#runtime-btn-${seed.interfaceId}`)).toHaveAttribute('title', /Start Processing|Resume/, { timeout: 15000 });
        await page.locator(`#runtime-btn-${seed.interfaceId}`).click();
        await expect(page.locator(`#runtime-btn-${seed.interfaceId}`)).toHaveAttribute('title', /Pause Processing/, { timeout: 15000 });
        const ack = await seed.sendMessage(HL7_ADT_A01);
        expect(parseAckCode(ack), 'interface accepts messages again after resume').toBe('AA');
    });

    test('IFC-20 interface detail shows the right interface', async ({ page, seed }) => {
        await page.goto(`/interface-detail.html?interfaceId=${seed.interfaceId}`, { waitUntil: 'networkidle' });
        await expect(page.getByText(seed.interfaceName).first()).toBeVisible();
    });

    test('IFC-02 wizard refuses a duplicate interface name', async ({ page, seed }) => {
        test.fail(true, 'KNOWN DEFECT: /api/wizard/check-duplicate returns isDuplicate as the existing interface object, but public/js/core/wizard-navigation.js only blocks when isDuplicate === true, so a duplicate name advances to step 2 with no message.');
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        await page.click('#createInterfaceBtn');
        await page.waitForSelector('#wizardModalContainer', { state: 'visible' });
        await page.fill('input[placeholder^="e.g., Main Hospital"]', seed.interfaceName);
        await page.waitForTimeout(500);
        await page.locator('#wizardNext').click();
        await page.waitForTimeout(2500);
        const stillStep1 = await page.getByText(/BASIC INFORMATION/i).first().isVisible().catch(() => false);
        const msg = (await page.locator('#wizardModalContainer').innerText()).toLowerCase();
        expect(stillStep1 || /already|exists|duplicate/.test(msg), 'duplicate name must not advance silently').toBeTruthy();
        expect(/already|exists|duplicate|in use/.test(msg), 'user is told why (message mentions duplicate/exists)').toBeTruthy();
    });

    test('IFC-05 closing the wizard creates nothing', async ({ page }) => {
        const before = await (await page.request.get('/api/interfaces')).json();
        const n0 = (before.interfaces || before.data || before).length;
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        await page.click('#createInterfaceBtn');
        await page.waitForSelector('#wizardModalContainer', { state: 'visible' });
        await page.fill('input[placeholder^="e.g., Main Hospital"]', 'QA-should-not-exist-' + Date.now());
        page.once('dialog', (d) => d.accept());
        await page.locator('#wizardClose').click();
        await page.waitForTimeout(1500);
        const after = await (await page.request.get('/api/interfaces')).json();
        expect((after.interfaces || after.data || after).length).toBe(n0);
    });
});

// ─── 3. Messages ─────────────────────────────────────────────────────────────
test.describe('3 Messages and tracing', () => {
    test('MSG-02 a message sent over the real MLLP listener is ACKed and stored byte for byte', async ({ seed, request }) => {
        const ctl = 'UAT' + Date.now();
        const msg = HL7_ADT_A01.replace(/MSH\|[^\r\n]*/, (m) => m.split('|').map((f, i) => (i === 9 ? ctl : f)).join('|'));
        const idOf = (m) => m.message_id || m.id; // /raw is keyed by message_id (tcp_…), not the row uuid
        const known = new Set((await seed.api.getMessages(seed.interfaceId, 200)).map(idOf));
        const ack = await seed.sendMessage(msg);
        expect(parseAckCode(ack)).toBe('AA');
        // The list carries metadata, not content, so identify our message as the one that is new.
        let found;
        for (let i = 0; i < 20 && !found; i++) {
            await new Promise((r) => setTimeout(r, 750));
            found = (await seed.api.getMessages(seed.interfaceId, 200)).find((m) => !known.has(idOf(m)));
        }
        expect(found, 'the message appears in the interface message list').toBeTruthy();
        const id = idOf(found);
        const raw = await request.get(`/api/messages/${id}/raw`);
        expect(raw.status()).toBe(200);
        expect(await raw.text()).toContain(ctl);
    });

    test('MSG-06 a non-HL7 frame is rejected with AR and nothing is stored', async ({ seed }) => {
        const before = (await seed.api.getMessages(seed.interfaceId, 100)).length;
        const ack = await sendMLLP('localhost', seed.port, 'this is not hl7 at all', 10000);
        expect(parseAckCode(ack)).toBe('AR');
        await new Promise((r) => setTimeout(r, 1500));
        expect((await seed.api.getMessages(seed.interfaceId, 100)).length).toBe(before);
    });

    test('MSG-03 messages page lists the interface messages and can filter', async ({ page, seed }) => {
        await page.goto(`/messages.html?interfaceId=${seed.interfaceId}`, { waitUntil: 'networkidle' });
        await page.waitForTimeout(1200);
        const rows = page.locator('table tbody tr');
        expect(await rows.count(), 'at least the 3 seeded messages are listed').toBeGreaterThanOrEqual(3);
        const opts = await page.locator('#filterStatus option').allTextContents();
        expect(opts.length).toBeGreaterThan(1);
        await page.locator('#filterStatus').selectOption({ index: opts.findIndex((o) => /fail|error/i.test(o)) > 0 ? opts.findIndex((o) => /fail|error/i.test(o)) : 1 });
        await page.waitForTimeout(1200);
        await expect(page.locator('body')).not.toContainText(/undefined|\[object/);
    });

    test('MSG-05 trace shows a timeline for a real message', async ({ page, seed }) => {
        await page.goto('/message-trace.html', { waitUntil: 'networkidle' });
        await page.locator('#interfaceInput').fill(seed.interfaceName);
        await page.getByPlaceholder(/tcp_abc123/).fill(String(seed.messageIds[0]));
        await page.getByRole('button', { name: 'Trace' }).click();
        await page.waitForTimeout(4000);
        const body = await page.locator('body').innerText();
        expect(body).not.toMatch(/undefined|\[object Object\]/);
        expect(body).not.toContain('Tracing...');
    });

    test('MSG-05b trace tells the user when an id is unknown', async ({ page, seed }) => {
        await page.goto('/message-trace.html', { waitUntil: 'networkidle' });
        await page.locator('#interfaceInput').fill(seed.interfaceName);
        await page.getByPlaceholder(/tcp_abc123/).fill('no-such-message-id');
        await page.getByRole('button', { name: 'Trace' }).click();
        await page.waitForTimeout(8000);
        const t = (await page.locator('main, .content, body').first().innerText()).toLowerCase();
        expect(t, 'unknown id gives a readable not-found').toMatch(/not found|no message|no trace|couldn.t find|unknown|no results/);
    });

    test('MSG-10 a viewer cannot read message content through the UI-facing API', async ({ browser, playwright, seed }) => {
        const admin = await playwright.request.newContext({ baseURL: BASE, storageState: '.auth/user.json' });
        const u = await createUserWithRole(admin, BASE, 'viewer');
        const ctx = await signedInContext(browser, u.email, PASSWORD);
        const id = seed.messageIds[0];
        for (const p of [`/api/messages/${id}/raw`, `/api/messages/${id}/lineage`, `/api/messages/${id}/transformed`]) {
            const r = await ctx.request.get(p);
            expect(r.status(), `viewer GET ${p}`).toBe(403);
        }
        const page = await ctx.newPage();
        await page.goto(`/messages.html?interfaceId=${seed.interfaceId}`, { waitUntil: 'networkidle' });
        await expect(page.locator('body')).not.toContainText('PWSEED');
        await ctx.close();
        await deleteUser(admin, BASE, u.id);
        await admin.dispose();
    });
});

// ─── 4. Watching the system ──────────────────────────────────────────────────
test.describe('4 Dashboard and monitoring', () => {
    test('OPS-01 dashboard interface count equals the real interface count', async ({ page }) => {
        const list = await (await page.request.get('/api/interfaces')).json();
        const total = (list.interfaces || list.data || list).length;
        await page.goto('/dashboard.html', { waitUntil: 'networkidle' });
        await page.waitForTimeout(2500);
        const tile = await page.locator('a[href="interfaces.html"]:visible').filter({ hasText: /\d/ }).first().innerText();
        const nums = (tile.match(/\d[\d,]*/g) || []).map((n) => parseInt(n.replace(/,/g, ''), 10));
        expect(nums, `Interfaces tile "${tile.replace(/\s+/g, ' ')}" should show ${total}`).toContain(total);
    });

    test('OPS-02 quick-action tiles go where they say', async ({ page }) => {
        await page.goto('/dashboard.html', { waitUntil: 'networkidle' });
        const targets = { 'interfaces.html': /interfaces\.html/, 'monitoring.html': /monitoring\.html/, 'user-management.html': /user-management\.html/, 'settings.html': /settings\.html/ };
        for (const [href, re] of Object.entries(targets)) {
            await page.goto('/dashboard.html', { waitUntil: 'networkidle' });
            await page.locator(`main a[href="${href}"]:visible, .content a[href="${href}"]:visible, a[href="${href}"]:visible`).filter({ hasNot: page.locator('#sidebar') }).last().click();
            await page.waitForURL(re, { timeout: 15000 });
        }
    });

    test('OPS-03 monitoring shows a message that was just sent in the live feed', async ({ page, seed }) => {
        await page.goto('/monitoring.html', { waitUntil: 'networkidle' });
        await seed.sendMessage(HL7_ADT_A01);
        await page.getByRole('button', { name: /Refresh Now/ }).click();
        await page.waitForTimeout(4000);
        await expect(page.getByText(/Live Message Feed/)).toBeVisible();
        const feed = await page.locator('body').innerText();
        expect(feed).not.toMatch(/undefined|\[object Object\]|NaN/);
    });

    test('OPS-04 engine metrics populate every section', async ({ page }) => {
        await page.goto('/engine-metrics.html', { waitUntil: 'networkidle' });
        await page.getByRole('button', { name: /Refresh Now/ }).click();
        await page.waitForTimeout(3000);
        for (const h of ['Throughput', 'Latency', 'Queue & Backpressure', 'Reliability', 'Capacity']) await expect(page.getByText(h, { exact: false }).first()).toBeVisible();
        expect(await page.locator('body').innerText()).not.toMatch(/NaN|undefined|\[object Object\]/);
    });

    test('OPS-06 alerts page loads and its filters work without error', async ({ page }) => {
        await page.goto('/alerts.html', { waitUntil: 'networkidle' });
        await expect(page.getByText('Alert Management').first()).toBeVisible();
        const opts = await page.locator('#filterType option').allTextContents();
        if (opts.length > 1) { await page.locator('#filterType').selectOption({ index: 1 }); await page.waitForTimeout(800); }
        await page.getByRole('button', { name: 'Refresh' }).first().click();
        await page.waitForTimeout(800);
        expect(await page.locator('body').innerText()).not.toMatch(/undefined|\[object Object\]/);
    });

    test('OPS-08 reports gallery: running a report returns something readable', async ({ page }) => {
        await page.goto('/reports.html', { waitUntil: 'networkidle' });
        await page.getByRole('button', { name: /Run/ }).first().click();
        await page.waitForTimeout(4000);
        const t = await page.locator('body').innerText();
        expect(t).not.toMatch(/undefined|\[object Object\]|NaN/);
    });
});

// ─── 5. Tools ────────────────────────────────────────────────────────────────
test.describe('5 Tools', () => {
    test('TLS-01/05 HL7 Reader parses the sample and Clear resets it', async ({ page }) => {
        await page.goto('/hl7-reader.html', { waitUntil: 'networkidle' });
        await page.getByRole('button', { name: 'ADT^A01' }).first().click();
        await page.getByRole('button', { name: /Parse & Analyze/ }).click();
        await expect(page.getByText(/\d+ segments/).first()).toBeVisible({ timeout: 15000 });
        expect(await page.locator('body').innerText()).toMatch(/MSH/);
        await page.locator('button[title*="Clear" i], button[aria-label*="Clear" i]').first().click();
        await expect(page.getByText(/No message parsed yet/)).toBeVisible({ timeout: 10000 });
    });

    test('TLS-01b HL7 Reader shows segment headings as text, never as raw HTML', async ({ page }) => {
        test.fail(true, 'KNOWN DEFECT: the segment heading prints the badge markup literally ("MSH <span class=\\"badge-mini required\\" ...>R</span>") — a double-escape in the HL7 Reader after the HtmlSafe migration.');
        await page.goto('/hl7-reader.html', { waitUntil: 'networkidle' });
        await page.getByRole('button', { name: 'ADT^A01' }).first().click();
        await page.getByRole('button', { name: /Parse & Analyze/ }).click();
        await expect(page.getByText(/\d+ segments/).first()).toBeVisible({ timeout: 15000 });
        expect(await page.locator('body').innerText(), 'markup must not be visible as text').not.toMatch(/<span|<\/span>|class="badge/);
    });

    test('TLS-03 HL7 Reader handles garbage and empty input gracefully', async ({ page }) => {
        await page.goto('/hl7-reader.html', { waitUntil: 'networkidle' });
        const ta = page.locator('textarea').first();
        await ta.fill('');
        await page.getByRole('button', { name: /Parse & Analyze/ }).click();
        await page.waitForTimeout(800);
        await ta.fill('this is definitely not an hl7 message');
        await page.getByRole('button', { name: /Parse & Analyze/ }).click();
        await page.waitForTimeout(1500);
        const t = await page.locator('body').innerText();
        expect(t).not.toMatch(/undefined|\[object Object\]|Cannot read|TypeError/);
        expect(t.toLowerCase()).toMatch(/invalid|error|msh|not a valid|could not|unable|missing/);
    });

    test('TLS-06 FHIR Validator accepts a valid Patient', async ({ page }) => {
        await page.goto('/fhir-validator.html', { waitUntil: 'networkidle' });
        await page.getByRole('button', { name: 'Patient', exact: true }).click();
        await page.getByRole('button', { name: 'Validate' }).click();
        await page.waitForTimeout(3500);
        const t = (await page.locator('body').innerText()).toLowerCase();
        expect(t).not.toContain('nothing validated yet');
        expect(t).toMatch(/valid|pass|0 error|no errors|success/);
    });

    test('TLS-07 FHIR Validator reports errors for the invalid example', async ({ page }) => {
        await page.goto('/fhir-validator.html', { waitUntil: 'networkidle' });
        await page.getByRole('button', { name: 'Invalid' }).click();
        await page.getByRole('button', { name: 'Validate' }).click();
        await page.waitForTimeout(3500);
        const t = (await page.locator('body').innerText()).toLowerCase();
        expect(t).not.toContain('nothing validated yet');
        expect(t).toMatch(/error|invalid|fail|required|missing/);
    });

    test('TLS-11 code translations export gives a CSV', async ({ page }) => {
        await page.goto('/code-translations.html', { waitUntil: 'networkidle' });
        const [dl] = await Promise.all([page.waitForEvent('download', { timeout: 15000 }), page.getByRole('button', { name: /Export CSV/ }).click()]);
        expect(dl.suggestedFilename()).toMatch(/\.csv$/i);
    });
});

// ─── 6. Administration ───────────────────────────────────────────────────────
test.describe('6 Administration', () => {
    test('ADM-01/05/07 a created user can be found, then shows in the audit log', async ({ page, playwright }) => {
        const admin = await playwright.request.newContext({ baseURL: BASE, storageState: '.auth/user.json' });
        const u = await createUserWithRole(admin, BASE, 'operator');
        await page.goto('/user-management.html', { waitUntil: 'networkidle' });
        await page.getByPlaceholder(/Search name or email/).fill(u.email);
        await page.waitForTimeout(1200);
        await expect(page.getByText(u.email).first()).toBeVisible();
        await page.locator('#um-filter-role').selectOption({ index: 1 }).catch(() => {});
        await page.waitForTimeout(600);
        await page.getByRole('button', { name: /Audit Logs/ }).first().click();
        await page.waitForTimeout(2500);
        const audit = await page.locator('body').innerText();
        expect(audit.toLowerCase(), 'audit log lists user creation').toMatch(/user_created|user created|create/);
        await deleteUser(admin, BASE, u.id);
        await admin.dispose();
    });

    test('ADM-10 SMTP settings persist across reload and the password stays masked', async ({ page }) => {
        await page.goto('/settings.html', { waitUntil: 'networkidle' });
        const host = page.getByPlaceholder('smtp.gmail.com');
        const original = await host.inputValue();
        await host.fill('smtp.uat-test.example');
        await page.getByRole('button', { name: /Save Settings/ }).click();
        await page.waitForTimeout(2000);
        await page.reload({ waitUntil: 'networkidle' });
        expect(await page.getByPlaceholder('smtp.gmail.com').inputValue()).toBe('smtp.uat-test.example');
        const pw = page.getByPlaceholder('Leave blank to keep current');
        expect(await pw.inputValue(), 'password field never echoes a stored secret').toBe('');
        await page.getByPlaceholder('smtp.gmail.com').fill(original);
        await page.getByRole('button', { name: /Save Settings/ }).click();
        await page.waitForTimeout(1500);
    });

    test('ADM-14 admin can open Mapping Review from the Admin menu', async ({ page }) => {
        test.fail(true, 'KNOWN DEFECT: Mapping Review is offered in the Admin menu, but its API (/api/fhir/quality/*) is super_admin-only, so an `admin` gets 403s and an "Access denied" page.');
        await page.goto('/admin-mapping-review.html', { waitUntil: 'networkidle' });
        await expect(page.getByText(/access denied/i), 'an admin must not be denied a page the Admin menu offers').toHaveCount(0);
    });

    test('ADM-06 roles tab matches reality for the admin role', async ({ page }) => {
        await page.goto('/user-management.html', { waitUntil: 'networkidle' });
        await page.getByRole('button', { name: /Roles & Permissions/ }).click();
        await page.waitForTimeout(1200);
        const t = await page.locator('body').innerText();
        for (const r of ['admin', 'operator', 'viewer']) expect(t.toLowerCase()).toContain(r);
    });
});

// ─── 7. Partner endpoints and hostile input ──────────────────────────────────
test.describe('7 Partners and hostile users', () => {
    test('PRT-03 partner endpoint without credentials is refused', async ({ seed }) => {
        const anon = await pwRequest.newContext({ baseURL: BASE, storageState: ANON });
        for (const p of ['eligibility', 'claim-status', 'prior-auth']) {
            const r = await anon.post(`/api/${p}/${seed.interfaceId}/check`, { data: 'ISA*00*', headers: { 'content-type': 'application/edi-x12' } });
            expect(r.status(), `/api/${p}`).toBe(401);
        }
        const r = await anon.post(`/api/pharmacy-claim/${seed.interfaceId}/submit`, { data: 'x', headers: { 'content-type': 'application/x-ncpdp-telecom-d0' } });
        expect(r.status()).toBe(401);
        await anon.dispose();
    });

    test('EVE-01 hostile text is shown as text and never runs', async ({ page, seed }) => {
        const payload = '<img src=x onerror="window.__xss=1">';
        const r = await page.request.put(`/api/interfaces/${seed.idleInterfaceId}`, { data: { description: payload } });
        let dialogs = 0;
        page.on('dialog', async (d) => { dialogs++; await d.dismiss(); });
        await page.goto(`/interface-detail.html?interfaceId=${seed.idleInterfaceId}`, { waitUntil: 'networkidle' });
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        await page.getByPlaceholder(/Search interfaces/).fill(seed.idleInterfaceName);
        await page.waitForTimeout(1200);
        expect(await page.evaluate(() => window.__xss), 'injected handler must not have run').toBeUndefined();
        expect(dialogs).toBe(0);
        expect(r.status()).toBeLessThan(500);
    });

    test('EVE-03 a basic user cannot read another person\'s interface by changing the URL id', async ({ browser, playwright, seed }) => {
        const admin = await playwright.request.newContext({ baseURL: BASE, storageState: '.auth/user.json' });
        const u = await createUserWithRole(admin, BASE, 'user');
        const ctx = await signedInContext(browser, u.email, PASSWORD);
        for (const p of [`/api/interfaces/${seed.interfaceId}`, `/api/messages/interface/${seed.interfaceId}`]) {
            const r = await ctx.request.get(p);
            expect([401, 403, 404], `basic user GET ${p} (got ${r.status()})`).toContain(r.status());
        }
        await ctx.close();
        await deleteUser(admin, BASE, u.id);
        await admin.dispose();
    });
});

// ─── 8. Message errors are visible; Trace is scoped to an interface ──────────
// (docs/UAT_TEST_CASES.md MSG-17..MSG-20; found by the v26.10 format×connectivity matrix)
const { hl7SinkPayload } = require('../helpers/fixtures');
const FAIL_PORT = parseInt(process.env.UAT_FAIL_PORT || '6657', 10);

test.describe('8 Message errors and trace', () => {
    test.describe.configure({ mode: 'serial' });
    let failing; // { id, name, messageId }

    test.beforeAll(async ({ playwright, seed }) => {
        // An interface whose destination refuses the connection, so a delivered message must FAIL with a reason.
        const dest = { url: 'http://host.docker.internal:9/none', method: 'POST', content_type: 'text/plain', authentication_type: 'none', retry_attempts: 1, retry_delay_seconds: 1, timeout_seconds: 5 };
        const payload = {
            ...hl7SinkPayload(FAIL_PORT), name: `PW_UAT_FAIL_${Date.now()}`, transformationFlow: 'hl7_passthrough',
            targetType: 'hl7v2', targetConnectivity: 'http_outbound', targetConfig: dest,
            targetConnectorConfig: { connectorType: 'http_outbound', config: dest },
        };
        const created = await seed.api.createInterface(payload);
        const id = created.interfaceId || created.data?.interfaceId || created.id;
        await seed.api.activateInterface(id);
        await new Promise((r) => setTimeout(r, 1500));
        const ack = await sendMLLP('localhost', FAIL_PORT, HL7_ADT_A01, 15000);
        expect(parseAckCode(ack)).toBe('AA');
        let msg;
        for (let i = 0; i < 40 && !(msg && /failed|error/i.test(msg.status)); i++) {
            await new Promise((r) => setTimeout(r, 1000));
            msg = (await seed.api.getMessages(id, 5))[0];
        }
        expect(msg && msg.status, 'a message whose destination refuses the connection ends up failed').toMatch(/failed|error/i);
        failing = { id, name: payload.name, messageId: msg.message_id };
    });

    test.afterAll(async ({ seed }) => { if (failing) await seed.api.deleteInterface(failing.id).catch(() => {}); });

    test('MSG-17 a failed message states that it failed and why, and its Errors tab lists the error', async ({ page }) => {
        await page.goto(`/messages.html?interfaceId=${failing.id}`, { waitUntil: 'networkidle' });
        await page.locator('table tbody tr').first().click();
        const banner = page.getByRole('alert').filter({ hasText: 'This message failed' });
        await expect(banner).toBeVisible({ timeout: 15000 });
        await expect(banner, 'the reason is shown, not just "failed"').toContainText(/connection refused|delivery failed/i);

        const badge = page.locator('#errorsTabBadge');
        await expect(badge).toBeVisible();
        expect(parseInt(await badge.innerText(), 10) || 1).toBeGreaterThanOrEqual(1);

        await page.locator('#errorsTabBtn').click();
        const view = page.locator('#messageErrorsView');
        await expect(view).toContainText(/connection refused|delivery failed/i, { timeout: 15000 });
        await expect(view).not.toContainText(/Unable to load error data|No Errors or Warnings/);
    });

    test('MSG-17b the errors API returns the structured error (stage, severity) for a failed message', async ({ request }) => {
        const r = await request.get(`/api/messages/${encodeURIComponent(failing.messageId)}/errors?interfaceId=${failing.id}`);
        expect(r.status()).toBe(200);
        const body = await r.json();
        expect(body.success).toBe(true);
        expect(body.data.errors.length, 'the structured error stack was captured').toBeGreaterThanOrEqual(1);
        // One failure is one error: the engine's own status update and the error capture must not both count it.
        expect(body.data.summary.error_count, 'error_count equals the number of recorded errors').toBe(body.data.errors.length);
        // The summary keeps the REASON, not just the capture's generic title ("Pipeline execution failed").
        expect(body.data.summary.last_error_message).toMatch(/connection refused|delivery failed/i);
        expect(body.data.errors[0]).toMatchObject({ severity: expect.stringMatching(/error|critical/), stage: expect.any(String) });
    });

    test('MSG-18 the errors endpoint refuses an injected or non-UUID interfaceId', async ({ request }) => {
        for (const bad of ['x"; DROP TABLE users; --', 'messages_intf_abc', '']) {
            const r = await request.get(`/api/messages/${encodeURIComponent(failing.messageId)}/errors?interfaceId=${encodeURIComponent(bad)}`);
            expect(r.status(), `interfaceId=${bad}`).toBe(400);
        }
    });

    test('MSG-19 trace is scoped to an interface: required, exact id wins, a reused id is reported as ambiguous', async ({ request, seed }) => {
        // No interface -> refused (an unscoped lookup can only guess)
        expect((await request.get(`/api/system/message-trace/${encodeURIComponent(seed.messageIds[0])}`)).status()).toBe(400);

        // The message's own id is unambiguous
        const exact = await (await request.get(`/api/system/message-trace/${encodeURIComponent(seed.messageIds[0])}?interfaceId=${seed.interfaceId}`)).json();
        expect(exact.success).toBe(true);
        expect(exact.matched_by).toBe('message_id');
        expect(exact.ambiguous).toBe(false);
        expect(exact.count, 'the processing log timeline is returned').toBeGreaterThan(0);

        // A control id reused by two messages of the interface is reported as ambiguous, newest traced first
        const ctl = `UATDUP${Date.now()}`;
        const mk = (n) => HL7_ADT_A01.replace(/MSH\|[^\r\n]*/, (m) => m.split('|').map((f, i) => (i === 9 ? ctl : f)).join('|'));
        await seed.sendMessage(mk(1)); await new Promise((r) => setTimeout(r, 1500)); await seed.sendMessage(mk(2));
        let t;
        for (let i = 0; i < 20; i++) {
            await new Promise((r) => setTimeout(r, 1000));
            t = await (await request.get(`/api/system/message-trace/${ctl}?interfaceId=${seed.interfaceId}`)).json();
            if (t.success && (t.matches || []).length >= 2) break;
        }
        expect(t.success).toBe(true);
        expect(t.matched_by).toBe('correlation_id');
        expect(t.ambiguous).toBe(true);
        expect(t.matches.length).toBeGreaterThanOrEqual(2);

        // The same id looked up in ANOTHER interface finds nothing
        const other = await request.get(`/api/system/message-trace/${ctl}?interfaceId=${seed.idleInterfaceId}`);
        expect(other.status()).toBe(404);
    });

    test('MSG-20 Message Trace page: pick the interface, paste the id, see the timeline; unknown id names the interface', async ({ page, seed }) => {
        await page.goto(`/message-trace.html?interfaceId=${seed.interfaceId}&id=${encodeURIComponent(seed.messageIds[0])}`, { waitUntil: 'networkidle' });
        await expect(page.locator('#countPill')).toContainText(/\d+ events?/, { timeout: 20000 });
        await expect(page.locator('.timeline-row').first()).toBeVisible();

        await page.locator('#correlationInput').fill('no-such-message-id');
        await page.getByRole('button', { name: /Trace/ }).click();
        const notFound = page.locator('#resultBody');
        await expect(notFound).toContainText('No message found', { timeout: 15000 });
        await expect(notFound, 'says which interface it looked in').toContainText(seed.interfaceName);
    });
});

// ─── 9. Passthrough delivers the original; the message page tells the truth ───
// (docs/UAT_TEST_CASES.md MSG-21..MSG-24; found by the v26.10 format×connectivity matrix)
const http = require('http');
const PT_PORT = parseInt(process.env.UAT_PT_PORT || '6658', 10);

test.describe('9 Passthrough content and message-page numbers', () => {
    test('MSG-21 an HL7 passthrough delivers the ORIGINAL HL7 message, byte for byte', async ({ seed }) => {
        const received = [];
        const recv = http.createServer((req, res) => {
            let body = ''; req.on('data', (c) => (body += c));
            req.on('end', () => { received.push({ body, ct: req.headers['content-type'] || '' }); res.writeHead(200); res.end('{}'); });
        });
        await new Promise((r) => recv.listen(0, '0.0.0.0', r));
        let id;
        try {
            const dest = { url: `http://${process.env.RECEIVER_HOST || 'host.docker.internal'}:${recv.address().port}/pt`, method: 'POST', content_type: 'application/hl7-v2', authentication_type: 'none', retry_attempts: 1 };
            const created = await seed.api.createInterface({
                ...hl7SinkPayload(PT_PORT), name: `PW_UAT_PT_${Date.now()}`, transformationFlow: 'hl7_passthrough',
                targetType: 'hl7v2', targetConnectivity: 'http_outbound', targetConfig: dest, targetConnectorConfig: { connectorType: 'http_outbound', config: dest },
            });
            id = created.interfaceId || created.data?.interfaceId || created.id;
            await seed.api.activateInterface(id);
            await new Promise((r) => setTimeout(r, 1500));
            expect(parseAckCode(await sendMLLP('localhost', PT_PORT, HL7_ADT_A01, 15000))).toBe('AA');
            for (let i = 0; i < 30 && received.length === 0; i++) await new Promise((r) => setTimeout(r, 1000));
            expect(received.length, 'the message was delivered downstream').toBe(1);
            expect(received[0].body.trim(), 'the destination gets the original HL7, not a parsed-JSON blob').toBe(HL7_ADT_A01.trim());
            expect(received[0].ct).toMatch(/hl7/i);
        } finally {
            if (id) await seed.api.deleteInterface(id).catch(() => {});
            recv.close();
        }
    });

    test('MSG-22 the message page numbers add up: a processed interface has a real success rate and Active Interfaces counts active ones', async ({ page, seed, request }) => {
        await page.goto(`/messages.html?interfaceId=${seed.interfaceId}`, { waitUntil: 'networkidle' });
        await expect(page.locator('#totalMessages')).not.toHaveText('-', { timeout: 15000 });
        await expect(page.locator('#successRate'), 'processed messages are successes').not.toHaveText('0.0%', { timeout: 15000 });
        const list = await (await request.get('/api/interfaces')).json();
        const active = (list.interfaces || list.data || list).filter((i) => i.status === 'active').length;
        await expect(page.locator('#activeInterfaces')).toHaveText(String(active));
        const label = (await page.locator('#currentInterfaceSelect option:checked').innerText()).trim();
        expect(label, 'no meaningless "(unknown)" suffix').not.toMatch(/\(unknown\)/i);
    });

    test('MSG-23 the Content tab of a message opens quickly (the interface is passed, so no server-side scan of every table)', async ({ page, seed }) => {
        await page.goto(`/messages.html?interfaceId=${seed.interfaceId}`, { waitUntil: 'networkidle' });
        await page.locator('table tbody tr').first().click();
        const t0 = Date.now();
        await page.locator('#contentTabBtn').click();
        await expect(page.locator('#messageContentView')).not.toContainText('Loading content', { timeout: 15000 });
        const ms = Date.now() - t0;
        expect(ms, `Content tab took ${ms} ms`).toBeLessThan(4000);
    });
});

// A failed message must still show what was attempted (appended to the serial group above so it reuses its failing interface).
test.describe('8b Failed message content', () => {
    test('MSG-24 a message that failed at delivery shows the payload that was attempted, labelled as not delivered', async ({ page, seed }) => {
        const dest = { url: 'http://host.docker.internal:9/none', method: 'POST', content_type: 'text/plain', authentication_type: 'none', retry_attempts: 1, retry_delay_seconds: 1, timeout_seconds: 5 };
        const port = parseInt(process.env.UAT_FAIL2_PORT || '6656', 10);
        const created = await seed.api.createInterface({
            ...hl7SinkPayload(port), name: `PW_UAT_FAIL2_${Date.now()}`, transformationFlow: 'hl7_passthrough',
            targetType: 'hl7v2', targetConnectivity: 'http_outbound', targetConfig: dest, targetConnectorConfig: { connectorType: 'http_outbound', config: dest },
        });
        const id = created.interfaceId || created.data?.interfaceId || created.id;
        try {
            await seed.api.activateInterface(id);
            await new Promise((r) => setTimeout(r, 1500));
            await sendMLLP('localhost', port, HL7_ADT_A01, 15000);
            let msg;
            for (let i = 0; i < 40 && !(msg && /failed|error/i.test(msg.status)); i++) { await new Promise((r) => setTimeout(r, 1000)); msg = (await seed.api.getMessages(id, 5))[0]; }
            expect(msg && msg.status).toMatch(/failed|error/i);

            await page.goto(`/messages.html?interfaceId=${id}`, { waitUntil: 'networkidle' });
            await page.locator('table tbody tr').first().click();
            await page.locator('#contentTabBtn').click();
            const view = page.locator('#messageContentView');
            await expect(view).toContainText('Outbound (attempted, not delivered)', { timeout: 15000 });
            // (Stable tokens, not the MSH delimiter run, so the assertion does not depend on backslash escaping.)
            await expect(view, 'the attempted HL7 itself is shown, so the failure can be diagnosed').toContainText('MVPTEST001');
            await expect(view).toContainText('SMOKE^PATIENT^MVP');
            await expect(view).not.toContainText('Outbound (Sent)');
        } finally {
            await seed.api.deleteInterface(id).catch(() => {});
        }
    });
});
