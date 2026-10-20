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
