// Guard against HtmlSafe regressions where markup stored in a plain string is
// escaped and shown to the user as literal text (e.g. `<i class="fas fa-cog"></i> Basic`).
const { test, expect } = require('@playwright/test');

const PAGES = [
    'dashboard', 'interfaces', 'interface-wizard', 'messages', 'monitoring', 'alerts',
    'reports', 'settings', 'user-management', 'pipeline-builder', 'code-templates',
    'admin-dlq', 'review-queue', 'engine-metrics', 'hl7-reader', 'fhir-validator',
    'admin-mapping-review', 'admin-cda-dedupe-registry', 'git',
];
const TAG = /<\/?(i|b|u|em|strong|span|div|br|code|small|p|a|li|ul|ol|button|input|select|option|table|tr|td|th|img|svg|label|h[1-6]|pre|mark|kbd)(\s[^<>]*)?>/i;

async function leaked(page) {
    return page.evaluate((src) => {
        const re = new RegExp(src, 'i');
        const out = [];
        const w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
        for (let n; (n = w.nextNode());) {
            const p = n.parentElement;
            if (!p || ['SCRIPT', 'STYLE', 'TEXTAREA', 'PRE', 'CODE'].includes(p.tagName)) continue;
            if (p.closest('pre, code, textarea, .code-editor, .hljs')) continue;
            if (re.test(n.nodeValue)) out.push(n.nodeValue.trim().slice(0, 120));
        }
        return out;
    }, TAG.source);
}

for (const name of PAGES) {
    test(`no escaped markup text on ${name}.html`, async ({ page }) => {
        await page.goto(`/${name}.html`, { waitUntil: 'networkidle' }).catch(() => {});
        await page.waitForTimeout(800);
        expect(await leaked(page)).toEqual([]);
    });
}

for (const direction of ['inbound', 'outbound']) {
    test(`every ${direction} connector config panel shows icons, not markup text`, async ({ page }) => {
        await page.goto('/pipeline-builder.html', { waitUntil: 'networkidle' });
        const types = await page.evaluate(async (d) => {
            const r = await window.pipelineAPI.getConnectorTypes(d);
            return (r.data || r || []).map(t => t.type_name || t.typeName || t.name).filter(Boolean);
        }, direction);
        expect(types.length).toBeGreaterThan(5);
        const bad = {};
        for (const type of types) {
            await page.evaluate(async ({ d, type }) => {
                document.getElementById('__ccb')?.remove();
                const host = document.createElement('div');
                host.id = '__ccb';
                document.body.appendChild(host);
                const b = new window.ConnectorConfigBuilder(host, { connectorType: type }, d);
                b.render();
                await b.loadConnectorTypes();
                b.onConnectorTypeChange(type);
                await new Promise(r => setTimeout(r, 250));
            }, { d: direction, type });
            const host = page.locator('#__ccb');
            const hits = await page.evaluate((src) => {
                const re = new RegExp(src, 'i');
                const out = [];
                const w = document.createTreeWalker(document.getElementById('__ccb'), NodeFilter.SHOW_TEXT);
                for (let n; (n = w.nextNode());) {
                    if (['SCRIPT', 'STYLE', 'TEXTAREA'].includes(n.parentElement.tagName)) continue;
                    if (re.test(n.nodeValue)) out.push(n.nodeValue.trim().slice(0, 100));
                }
                return out;
            }, TAG.source);
            if (hits.length) bad[type] = hits;
            const icons = await host.locator('.connector-config-group-title i').count();
            const titles = await host.locator('.connector-config-group-title').count();
            if (titles && !icons) bad[type + ' (icons)'] = ['group titles have no <i> icon'];
        }
        expect(bad).toEqual({});
    });
}
