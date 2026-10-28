'use strict';
/**
 * QA click sweep — opens every page in public/*.html, then clicks every visible, enabled button / tab / link-button on
 * it (skipping anything that destroys or signs out), and records what a person would notice: script errors, console
 * errors, failed requests, 4xx/5xx answers.
 *
 * It does not assert page-specific outcomes (the functional suites do); it answers "does anything throw or 500 when
 * you press things?" for the pages nobody has a dedicated test for. QA_REPORT_ONLY=1 prints findings without failing.
 */
const { test, expect } = require('@playwright/test');
const fs = require('fs');
const path = require('path');
const { attachGates } = require('./helpers/ui-gates');

const PAGES_DIR = path.resolve(__dirname, '../../public');
const PAGES = fs.readdirSync(PAGES_DIR).filter((f) => f.endsWith('.html')).sort();
const NO_SESSION = new Set(['login.html', 'setup.html', 'invite.html', 'force-reset.html']);
// Never press these: they delete, stop, sign out, send, purge or change real data.
const DESTRUCTIVE = /delete|remove|deactivate|disable|logout|log out|sign out|reset|purge|erase|clear|stop|revoke|drop|cancel|discard|terminate|uninstall|wipe|force|restart|shutdown|suspend|lock|ban\b|reject|approve|redrive|retry all|send|submit|save|create|add\b|apply|import|upload|run\b|execute|pull|install|publish|deploy|activate|start/i;
const REPORT_ONLY = process.env.QA_REPORT_ONLY === '1';
const OUT = process.env.QA_OUT || path.resolve(__dirname, '../../tests/playwright-report/qa');
fs.mkdirSync(OUT, { recursive: true });
// An exploratory QA collector (it reports what a person would notice, including known open items), not a CI gate:
// it runs when asked for (QA_SWEEP=1 or QA_REPORT_ONLY=1), never as part of the default suite.
test.skip(!process.env.QA_SWEEP && !process.env.QA_REPORT_ONLY, 'exploratory QA sweep - set QA_SWEEP=1 (see docs/QA_STRATEGY.md)');

const all = [];

test.setTimeout(150_000);

for (const file of PAGES) {
    test(`click through ${file}`, async ({ browser, baseURL }) => {
        const ctx = await browser.newContext({
            viewport: { width: 1366, height: 768 },
            storageState: NO_SESSION.has(file) ? { cookies: [], origins: [] } : '.auth/user.json',
        });
        const page = await ctx.newPage();
        page.on('dialog', (d) => d.dismiss().catch(() => {}));
        const origin = new URL(baseURL).origin;
        const gates = attachGates(page, { origin, allowStatus: [401, 404] });
        const clicked = [];
        try {
            await page.goto(`/${file}`, { waitUntil: 'networkidle', timeout: 30_000 });
        } catch (e) { clicked.push(`navigation: ${e.message.slice(0, 80)}`); }
        await page.waitForTimeout(500);

        const count = await page.locator('button:visible, [role="tab"]:visible, .tab:visible, .nav-tabs a:visible, summary:visible').count();
        const deadline = Date.now() + 70_000;   // a multi-step wizard must not eat the whole test budget
        for (let i = 0; i < Math.min(count, 60) && Date.now() < deadline; i++) {
            if (page.url().indexOf(file) === -1 && !page.url().includes('#')) {   // a click navigated away: come back
                await page.goto(`/${file}`, { waitUntil: 'domcontentloaded' }).catch(() => {});
            }
            const el = page.locator('button:visible, [role="tab"]:visible, .tab:visible, .nav-tabs a:visible, summary:visible').nth(i);
            const label = ((await el.innerText({ timeout: 1000 }).catch(() => '')) + ' ' + ((await el.getAttribute('aria-label').catch(() => '')) || '') + ' ' + ((await el.getAttribute('title').catch(() => '')) || '')).trim().replace(/\s+/g, ' ');
            if (DESTRUCTIVE.test(label)) continue;
            if (!(await el.isEnabled().catch(() => false))) continue;
            await el.click({ timeout: 1000 }).catch(() => {});
            clicked.push(label.slice(0, 40) || '(icon)');
            await page.waitForTimeout(150);
            // close anything that opened so the next click is on the page itself
            await page.keyboard.press('Escape').catch(() => {});
        }
        await page.waitForTimeout(500);
        const f = await gates.collect().catch((e) => ({ collectError: String(e) }));
        const finding = {
            page: file, buttonsClicked: clicked.length,
            pageErrors: f.pageErrors || [], consoleErrors: f.consoleErrors || [],
            failedRequests: (f.failedRequests || []).filter((r) => !/ERR_ABORTED|net::ERR_BLOCKED/.test(r)),
            badResponses: f.badResponses || [], collectError: f.collectError,
        };
        all.push(finding);
        fs.writeFileSync(path.join(OUT, 'click-' + file + '.json'), JSON.stringify(finding, null, 2));
        await ctx.close();
        const problems = finding.pageErrors.length + finding.consoleErrors.length + finding.failedRequests.length + finding.badResponses.length;
        if (!REPORT_ONLY) expect(problems, JSON.stringify(finding, null, 1)).toBe(0);
    });
}
