'use strict';
/**
 * The UI must not depend on the public internet. Customer installs often sit on hospital networks with no outbound
 * access; the v26.10 release showed icon names as text when the Font Awesome CDN was unreachable. Font Awesome and
 * Bootstrap are now served from /vendor/, and the Content-Security-Policy allows no third-party origin.
 *
 * Every request that leaves the app origin is aborted here, so a page that still needs a CDN fails: a request to
 * another origin, or an icon that renders as text / a blank box.
 */
const { test, expect } = require('@playwright/test');
const { attachGates } = require('./helpers/ui-gates');

const PAGES = ['dashboard.html', 'interfaces.html', 'messages.html', 'settings.html', 'reports.html', 'review-queue.html',
    'pipeline-builder.html', 'user-management.html', 'message-trace.html'];

test.describe('no third-party resources', () => {
    for (const file of PAGES) {
        test(`${file} loads and shows its icons with the internet blocked`, async ({ browser, baseURL }) => {
            const origin = new URL(baseURL).origin;
            const ctx = await browser.newContext({ storageState: '.auth/user.json', viewport: { width: 1366, height: 768 } });
            await ctx.route('**/*', (route) => {
                const u = route.request().url();
                if (u.startsWith('data:') || u.startsWith('blob:') || new URL(u).origin === origin) return route.continue();
                return route.abort('internetdisconnected');
            });
            const page = await ctx.newPage();
            const gates = attachGates(page, { origin });
            await page.goto(`/${file}`, { waitUntil: 'networkidle' });
            await page.waitForTimeout(800);
            const f = await gates.collect();
            expect(f.offOrigin, `${file} asked another origin for something`).toEqual([]);
            expect(f.icons, `${file} has icons that render as text or blanks`).toEqual([]);
            await ctx.close();
        });
    }

    test('the Content-Security-Policy allows no third-party origin', async ({ request }) => {
        const csp = (await request.get('/login.html')).headers()['content-security-policy'] || '';
        expect(csp).not.toMatch(/cdnjs|jsdelivr|unpkg|googleapis|gstatic/);
        expect(csp).toContain("script-src 'self'");
    });
});
