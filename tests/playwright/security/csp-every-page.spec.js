// tests/playwright/security/csp-every-page.spec.js
//
// Content-Security-Policy regression guard (2026-10-19 release hardening).
// app.js now sends a CSP that still allows inline scripts/handlers (the UI uses
// ~900 of them) but restricts every external origin to this app and the two
// CDNs the UI loads. A wrong directive only shows up at runtime in a browser,
// so this spec opens EVERY page in public/ as a logged-in user and fails on any
// CSP violation the browser reports.
const { test, expect } = require('@playwright/test');
const fs = require('fs');
const path = require('path');

const PUBLIC_DIR = path.join(__dirname, '..', '..', '..', 'public');
const PAGES = fs.readdirSync(PUBLIC_DIR).filter(f => f.endsWith('.html')).sort();

test('the CSP header is present and keeps the expected directives', async ({ request }) => {
    const res = await request.get('/login.html');
    const csp = res.headers()['content-security-policy'] || '';
    expect(csp).toContain("default-src 'self'");
    expect(csp).toContain("object-src 'none'");
    expect(csp).toContain("frame-ancestors 'self'");
    expect(csp).toContain('script-src-attr');      // inline handlers allowed explicitly
    expect(csp).not.toContain('upgrade-insecure-requests'); // would break plain-HTTP installs
});

for (const page of PAGES) {
    test(`no CSP violations on ${page}`, async ({ page: browserPage }) => {
        const violations = [];
        browserPage.on('console', (msg) => {
            const text = msg.text();
            if (/Content Security Policy|violates the following/i.test(text)) violations.push(text);
        });
        await browserPage.addInitScript(() => {
            document.addEventListener('securitypolicyviolation', (e) => {
                // eslint-disable-next-line no-console
                console.error(`CSP-VIOLATION ${e.violatedDirective} blocked=${e.blockedURI} source=${e.sourceFile}:${e.lineNumber}`);
            });
        });
        browserPage.on('console', (msg) => {
            if (msg.text().startsWith('CSP-VIOLATION')) violations.push(msg.text());
        });

        await browserPage.goto('/' + page, { waitUntil: 'load' });
        await browserPage.waitForTimeout(1500); // let deferred scripts/CDN assets load
        expect([...new Set(violations)], `CSP violations on ${page}`).toEqual([]);
    });
}
