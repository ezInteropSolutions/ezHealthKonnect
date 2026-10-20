// tests/playwright/ui-xss-regression.spec.js
//
// Stored-XSS regression: values users type (interface name, description) must
// render as text on every page that lists them, never as HTML. An operator can
// name an interface; an admin opens the page — a payload that executes there
// runs with the admin's session. The CSP still allows inline handlers
// ('unsafe-inline', ~900 of them in the UI), so escaping is the defense.
const { test, expect } = require('@playwright/test');

const MARK = `xss${Date.now()}`;
const PAYLOAD_NAME = `PW XSS <img src=x onerror="window.__ezhkXss='${MARK}-name'"> ${MARK}`;
const PAYLOAD_DESC = `<svg onload="window.__ezhkXss='${MARK}-desc'"></svg> desc ${MARK}`;

// Pages that list interfaces by name. `search` filters a long list down to the
// test interface so it is certainly rendered; pages without one are checked
// for whatever they render (they may not list this interface at all).
const PAGES = [
    { url: '/interfaces.html', search: '#interfaceSearchInput' },
    { url: '/dashboard.html' },
    { url: '/monitoring.html' },
];

test.describe('User-entered text is never rendered as HTML', () => {
    let interfaceId;

    test.beforeAll(async ({ request }) => {
        const res = await request.post('/api/interfaces', {
            data: { name: PAYLOAD_NAME, description: PAYLOAD_DESC, messageType: 'ADT^A01' },
        });
        const body = await res.json();
        expect(body.success, JSON.stringify(body)).toBeTruthy();
        interfaceId = body.interface.id;
    });

    test.afterAll(async ({ request }) => {
        if (interfaceId) await request.delete(`/api/interfaces/${interfaceId}`).catch(() => {});
    });

    for (const p of PAGES) {
        test(`${p.url} renders an interface name and description as text`, async ({ page }) => {
            const dialogs = [];
            page.on('dialog', d => { dialogs.push(d.message()); d.dismiss().catch(() => {}); });
            await page.goto(p.url);
            await page.waitForLoadState('networkidle').catch(() => {});
            if (p.search) {
                await page.locator(p.search).fill(MARK);
                const row = page.locator('tr[data-interface-id]').filter({ hasText: MARK }).first();
                await expect(row).toBeVisible({ timeout: 20_000 });
                // Escaped exactly once: the payload shows as text, and the row's
                // own markup (badge, buttons) still renders as elements — not as
                // visible "&lt;span…" text from double escaping.
                const text = await row.innerText();
                expect(text).toContain('<img src=x onerror=');
                expect(text).not.toMatch(/&lt;|&amp;|<span|<button|<div/);
                await expect(row.locator('.interface-status')).toHaveCount(1);
                expect(await row.locator('button.act-btn').count()).toBeGreaterThan(2);
            } else {
                await page.waitForTimeout(3000);
            }
            const fired = await page.evaluate(() => window.__ezhkXss || null);
            expect(fired, `injected markup executed on ${p.url}`).toBeNull();
            expect(dialogs).toEqual([]);
            // The payload must not have produced live elements either.
            const injected = await page.locator(`img[onerror*="${MARK}"], svg[onload*="${MARK}"]`).count();
            expect(injected, `injected elements present on ${p.url}`).toBe(0);
        });
    }
});

// Message data comes from outside: the message id can be a partner's AS2
// Message-ID header or an S3 object key, and type, correlation id, source
// endpoint and error text are derived from what the sender sent. The row is
// written straight into the interface's message table — exactly what a
// connector stores — so every externally controlled field carries a payload,
// including a quote that would break out of an onclick string.
test.describe('Externally supplied message data is never rendered as HTML', () => {
    const { Client } = require('pg');
    const DB_URL = process.env.DATABASE_URL;
    const xss = (what) => `<img src=x onerror="window.__ezhkXss='${MARK}-${what}'">`;
    const MESSAGE_ID = `${MARK}');window.__ezhkXss='${MARK}-onclick';(' ${xss('id')}`;
    let interfaceId;

    test.beforeAll(async ({ request }) => {
        test.skip(!DB_URL, 'No DATABASE_URL — cannot plant an external message');
        const res = await request.post('/api/interfaces', {
            data: { name: `PW XSS messages ${MARK}`, messageType: 'ADT^A01' },
        });
        const body = await res.json();
        expect(body.success, JSON.stringify(body)).toBeTruthy();
        interfaceId = body.interface.id;
        const db = new Client({ connectionString: DB_URL });
        await db.connect();
        try {
            await db.query(
                `INSERT INTO messages_intf_${interfaceId.replace(/-/g, '_')}
                   (message_id, interface_id, status, source_type, source_endpoint, message_type,
                    correlation_id, message_size, error_count, last_error_message, delivery_status, received_at)
                 VALUES ($1, $2, 'failed', 'as2', $3, $4, $5, 42, 1, $6, 'failed', NOW())`,
                [MESSAGE_ID, interfaceId, `https://partner.example${xss('endpoint')}`, `ADT${xss('type')}`.slice(0, 100),
                    `corr${xss('corr')}`, `Partner said ${xss('error')}`]);
        } finally {
            await db.end();
        }
    });

    test.afterAll(async ({ request }) => {
        if (interfaceId) await request.delete(`/api/interfaces/${interfaceId}`).catch(() => {});
    });

    test('messages list and message detail render partner data as text', async ({ page }) => {
        const dialogs = [];
        page.on('dialog', d => { dialogs.push(d.message()); d.dismiss().catch(() => {}); });
        await page.goto(`/messages.html?interfaceId=${interfaceId}`);
        const row = page.locator('tr.message-row').filter({ hasText: MARK }).first();
        await expect(row).toBeVisible({ timeout: 20_000 });
        const text = await row.innerText();
        expect(text).toContain(`window.__ezhkXss='${MARK}-onclick'`);
        expect(text).not.toMatch(/&lt;|&amp;|<span|<button/);
        await expect(row.locator('.status-badge')).toHaveCount(1);

        // Opening the message runs the row's onclick with the hostile id.
        await row.click();
        await expect(page.locator('#modalMessageSubtitle')).toContainText(MARK, { timeout: 15_000 });
        await page.waitForTimeout(1500);

        expect(await page.evaluate(() => window.__ezhkXss || null), 'injected markup executed').toBeNull();
        expect(dialogs).toEqual([]);
        expect(await page.locator(`img[onerror*="${MARK}"]`).count(), 'injected elements present').toBe(0);
    });
});
