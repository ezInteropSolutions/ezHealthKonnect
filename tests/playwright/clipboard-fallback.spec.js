'use strict';
// The installer sets up plain-HTTP installs by default, where browsers do not provide navigator.clipboard at all.
// Every "Copy" button (22 call sites) used navigator.clipboard.writeText directly and threw. app-auth.js now installs a
// fallback with the same call; this proves it with the API removed, as on an http:// install.
const { test, expect } = require('@playwright/test');

test('copy works when navigator.clipboard is unavailable (plain-HTTP install)', async ({ page }) => {
    await page.addInitScript(() => { Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true }); });
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto('/dashboard.html', { waitUntil: 'networkidle' });
    await page.mouse.click(5, 5);   // user activation, as a real Copy click has
    const result = await page.evaluate(async () => {
        try { await navigator.clipboard.writeText('hello'); return 'ok'; } catch (e) { return 'rejected: ' + e.message; }
    });
    expect(result).toBe('ok');
    expect(errors.filter((m) => /clipboard|writeText/i.test(m))).toEqual([]);
});

test('a refused copy rejects (so callers can show their own message) instead of throwing synchronously', async ({ page }) => {
    await page.addInitScript(() => { Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true }); });
    await page.goto('/dashboard.html', { waitUntil: 'networkidle' });
    const r = await page.evaluate(() => {
        document.execCommand = () => false;
        const p = navigator.clipboard.writeText('x');
        return p.then(() => 'resolved', () => 'rejected');
    });
    expect(r).toBe('rejected');
});
