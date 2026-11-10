'use strict';
/**
 * QA page sweep — every page in public/*.html at every viewport, with the automatic UI gates.
 * Collector: writes QA_OUT/ui-sweep-<mode>.json and screenshots; fails on findings unless
 * QA_REPORT_ONLY=1. QA_OFFLINE=1 aborts every request that leaves the app origin
 * (proves nothing depends on a public CDN).
 */
const { test, expect } = require('@playwright/test');
const fs = require('fs');
const path = require('path');
const { attachGates } = require('../helpers/ui-gates');

const PAGES_DIR = path.resolve(__dirname, '../../../public');
const OUT = process.env.QA_OUT || path.resolve(__dirname, '../../../tests/playwright-report/qa');
const OFFLINE = process.env.QA_OFFLINE === '1';
const MODE = OFFLINE ? 'offline' : 'online';

const VIEWPORTS = [
    { name: '360', width: 360, height: 740 },
    { name: '768', width: 768, height: 1024 },
    { name: '1366', width: 1366, height: 768 },
    { name: '1920', width: 1920, height: 1080 },
];
const PAGES = fs.readdirSync(PAGES_DIR).filter((f) => f.endsWith('.html')).sort();
// Public pages that need no / a different session
const NEEDS_NO_SESSION = new Set(['login.html', 'setup.html', 'invite.html', 'force-reset.html']);

fs.mkdirSync(OUT, { recursive: true });
// An exploratory QA collector (it reports what a person would notice, including known open items), not a CI gate:
// it runs when asked for (QA_SWEEP=1 or QA_REPORT_ONLY=1), never as part of the default suite.
test.skip(!process.env.QA_SWEEP && !process.env.QA_REPORT_ONLY, 'exploratory QA sweep - set QA_SWEEP=1 (see docs/QA_STRATEGY.md)');

const results = [];

test.describe.configure({ mode: 'serial' });
test.setTimeout(60_000);

for (const vp of VIEWPORTS) {
    test.describe(`${MODE} @${vp.name}`, () => {
        for (const file of PAGES) {
            test(`${file}`, async ({ browser, baseURL }) => {
                const ctx = await browser.newContext({
                    viewport: { width: vp.width, height: vp.height },
                    storageState: NEEDS_NO_SESSION.has(file) ? { cookies: [], origins: [] } : '.auth/user.json',
                });
                const origin = new URL(baseURL).origin;
                if (OFFLINE) {
                    await ctx.route('**/*', (route) => {
                        const u = route.request().url();
                        if (u.startsWith('data:') || u.startsWith('blob:') || new URL(u).origin === origin) return route.continue();
                        return route.abort('internetdisconnected');
                    });
                }
                const page = await ctx.newPage();
                // 401 on a logged-out public page and 404 for optional resources are recorded, not hidden.
                const gates = attachGates(page, { origin });
                let nav = null;
                try {
                    const resp = await page.goto(`/${file}`, { waitUntil: 'networkidle', timeout: 30_000 });
                    nav = resp && resp.status();
                } catch (e) { nav = `navigation error: ${e.message.slice(0, 120)}`; }
                await page.waitForTimeout(800);
                const finalUrl = page.url().replace(origin, '');
                const f = await gates.collect().catch((e) => ({ collectError: String(e) }));
                const shot = path.join(OUT, `${MODE}-${vp.name}-${file.replace('.html', '')}.png`);
                await page.screenshot({ path: shot, fullPage: false }).catch(() => {});
                results.push({ page: file, viewport: vp.name, mode: MODE, nav, finalUrl, ...f });
                await ctx.close();
            });
        }
    });
}

test.afterAll(() => {
    fs.writeFileSync(path.join(OUT, `ui-sweep-${MODE}.json`), JSON.stringify(results, null, 2));
    if (process.env.QA_REPORT_ONLY === '1') return;
    const bad = results.filter((r) => r.collectError || (r.consoleErrors || []).length || (r.pageErrors || []).length
        || (r.failedRequests || []).length || (r.icons || []).length || (r.offOrigin || []).length || r.overflowPage);
    expect(bad.map((b) => `${b.page}@${b.viewport}`), 'pages with UI-gate findings').toEqual([]);
});
