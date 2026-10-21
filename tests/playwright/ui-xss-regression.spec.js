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

    test('/interface-detail.html renders the interface as text', async ({ page }) => {
        await page.goto(`/interface-detail.html?interfaceId=${interfaceId}`);
        await expect(page.locator('#interfaceName')).toContainText(MARK, { timeout: 15_000 });
        await expect(page.locator('#interfaceMeta code')).toHaveText(interfaceId);
        await page.waitForTimeout(1500);
        expect(await page.evaluate(() => window.__ezhkXss || null)).toBeNull();
        expect(await page.locator(`img[onerror*="${MARK}"], svg[onload*="${MARK}"]`).count()).toBe(0);
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

// A saved template carries user-chosen name/description/tags/icon and is shown
// to every user in the gallery, its preview, the configure modal and the
// delete confirmation.
test.describe('User-saved templates render as text in the gallery', () => {
    let templateId;
    const tName = `PW XSS tpl ${MARK}');window.__ezhkXss='${MARK}-tplclick';(' <img src=x onerror="window.__ezhkXss='${MARK}-tpl'">`;

    test.beforeAll(async ({ request }) => {
        const res = await request.post('/api/interface-templates', { data: {
            name: tName,
            description: `<svg onload="window.__ezhkXss='${MARK}-tpldesc'"></svg>`,
            tags: [`<b onmouseover="window.__ezhkXss=1">${MARK}</b>`],
            icon: '<u>!</u>', // the column holds 10 characters
            category: 'custom',
        } });
        const body = await res.json();
        expect(body.success, JSON.stringify(body)).toBeTruthy();
        templateId = body.template_id;
    });

    test.afterAll(async ({ request }) => {
        if (templateId) await request.delete(`/api/interface-templates/${templateId}`).catch(() => {});
    });

    test('card, details, configure and delete confirm show the name as text', async ({ page }) => {
        const dialogs = [];
        page.on('dialog', d => { dialogs.push(d.message()); d.dismiss().catch(() => {}); });
        await page.goto('/dashboard.html');
        await page.click('a[href="#templates"]');
        await page.waitForSelector('.tg-card', { timeout: 15_000 });
        await page.locator('#tgSearch').fill(MARK);
        const card = page.locator('.tg-card').filter({ hasText: MARK }).first();
        await expect(card).toBeVisible({ timeout: 10_000 });
        expect(await card.innerText()).toContain('<img src=x onerror=');
        await expect(card.locator('.tg-use-btn')).toHaveCount(1);
        await expect(card.locator('u, b')).toHaveCount(0); // icon and tag markup shown as text

        await card.locator('.tg-info-btn').click();
        await expect(page.locator('#tgPreviewModal .tg-modal-title')).toContainText(MARK);
        await page.locator('#tgPreviewModal .tg-modal-close').click();

        await card.locator('.tg-use-btn').click();
        await expect(page.locator('#tcf_name')).toHaveValue(tName);
        await page.locator('#tgConfigureModal').evaluate(el => el.remove());

        await card.locator('.tg-del-btn').click();
        await expect(page.locator('.app-dialog-body')).toContainText(MARK);
        await page.locator('#_appDlgCancel').click();

        await page.waitForTimeout(500);
        expect(await page.evaluate(() => window.__ezhkXss || null)).toBeNull();
        expect(dialogs).toEqual([]);
        expect(await page.locator(`img[onerror*="${MARK}"], svg[onload*="${MARK}"]`).count()).toBe(0);
    });
});

// Alert rules and notification channels are named by operators and listed for
// everyone on the Alerts page.
test.describe('Alert rules and channels render names as text', () => {
    const hostile = (what) => `PW ${MARK} ${what}');window.__ezhkXss='${MARK}-${what}';(' <img src=x onerror="window.__ezhkXss='${MARK}-${what}'">`;
    let ruleId, channelId;

    test.beforeAll(async ({ request }) => {
        const r = await (await request.post('/api/alerts/rules', { data: {
            name: hostile('rule'), rule_type: 'ERROR_RATE', threshold_warning: 5, is_enabled: false,
        } })).json();
        expect(r.success, JSON.stringify(r)).toBeTruthy();
        ruleId = r.data.id;
        const c = await (await request.post('/api/alerts/channels', { data: {
            name: hostile('channel'), channel_type: 'webhook', config: { url: 'https://example.invalid/hook' },
        } })).json();
        expect(c.success, JSON.stringify(c)).toBeTruthy();
        channelId = c.data.id;
    });

    test.afterAll(async ({ request }) => {
        if (ruleId) await request.delete(`/api/alerts/rules/${ruleId}`).catch(() => {});
        if (channelId) await request.delete(`/api/alerts/channels/${channelId}`).catch(() => {});
    });

    test('rules list, channel assignment and channels list', async ({ page }) => {
        const dialogs = [];
        page.on('dialog', d => { dialogs.push(d.message()); d.dismiss().catch(() => {}); });
        await page.goto('/alerts.html#rules');
        const rule = page.locator('.rule-card').filter({ hasText: `${MARK} rule` }).first();
        await expect(rule).toBeVisible({ timeout: 15_000 });
        expect(await rule.innerText()).toContain('<img src=x onerror=');
        // The channel chip used to pass the rule name through an onclick string.
        await rule.locator('.channel-count-chip').click();
        await expect(page.locator('#assignChannelsTitle')).toContainText(`${MARK} rule`);
        await expect(page.locator('#assignChannelsList')).toContainText(`${MARK} channel`, { timeout: 10_000 });

        await page.evaluate(() => { closeModal('assignChannelsModal'); activateTab('channels'); });
        await expect(page.locator('.channel-card').filter({ hasText: `${MARK} channel` })).toBeVisible({ timeout: 15_000 });

        await page.waitForTimeout(500);
        expect(await page.evaluate(() => window.__ezhkXss || null)).toBeNull();
        expect(dialogs).toEqual([]);
        expect(await page.locator(`img[onerror*="${MARK}"]`).count()).toBe(0);
    });
});

// A user's display name is chosen by that user (profile) and shown to admins.
test.describe('User names render as text on user management', () => {
    let userId;

    test.beforeAll(async ({ request }) => {
        const res = await request.post('/api/users', { data: {
            email: `pw-${MARK}@example.invalid`,
            password: `Pw!${MARK}Aa9#long`,
            name: `PW ${MARK} <img src=x onerror="window.__ezhkXss='${MARK}-user'">`,
            role: 'user',
        } });
        const body = await res.json();
        expect(res.ok(), JSON.stringify(body)).toBeTruthy();
        userId = body.user.id;
    });

    test.afterAll(async ({ request }) => {
        if (userId) await request.delete(`/api/users/${userId}`).catch(() => {});
    });

    test('users table shows the name as text', async ({ page }) => {
        await page.goto('/user-management.html');
        await page.locator('#um-search').fill(MARK);
        const row = page.locator('#um-tbody tr.um-row').filter({ hasText: MARK }).first();
        await expect(row).toBeVisible({ timeout: 15_000 });
        expect(await row.innerText()).toContain('<img src=x onerror=');
        await expect(row.locator('select.um-role-select')).toHaveCount(1);
        await page.waitForTimeout(500);
        expect(await page.evaluate(() => window.__ezhkXss || null)).toBeNull();
        expect(await page.locator(`img[onerror*="${MARK}"]`).count()).toBe(0);
    });
});

// AppDialogs is used everywhere with messages that interpolate data (names,
// errors returned by the server). A plain string is shown as text; formatting
// needs HtmlSafe.html (data inside stays escaped) or HtmlSafe.raw (constants).
test.describe('Dialogs show plain-string messages as text', () => {
    test('alert/confirm/prompt escape strings and keep HtmlSafe markup', async ({ page }) => {
        await page.goto('/interfaces.html');
        await page.waitForFunction(() => window.AppDialogs && window.HtmlSafe);
        const result = await page.evaluate(async (mark) => {
            const payload = `<img src=x onerror="window.__ezhkXss='${mark}'"> name`;
            const read = () => {
                const body = document.querySelector('.app-dialog-body');
                const out = { text: body.innerText, bold: body.querySelectorAll('b').length, img: body.querySelectorAll('img').length };
                document.querySelector('.app-dialog-overlay').remove();
                return out;
            };
            const results = {};
            AppDialogs.alert(payload, { title: payload });
            results.alert = read();
            AppDialogs.confirm(HtmlSafe.html`<b>Delete</b> ${payload}?`);
            results.confirm = read();
            AppDialogs.prompt('Reason', { defaultValue: `"><img src=x onerror="window.__ezhkXss='${mark}-v'">` });
            results.promptValue = document.querySelector('#_appDlgInput').value;
            results.promptImg = document.querySelectorAll('.app-dialog-overlay img').length;
            document.querySelector('.app-dialog-overlay').remove();
            AppDialogs.alert(HtmlSafe.raw('Help:<br><b>bold</b>'));
            results.raw = read();
            await new Promise(r => setTimeout(r, 300));
            results.fired = window.__ezhkXss || null;
            return results;
        }, MARK);
        expect(result.fired).toBeNull();
        expect(result.alert.text).toContain('<img src=x onerror=');
        expect(result.alert.img).toBe(0);
        expect(result.confirm.bold).toBe(1);
        expect(result.confirm.text).toContain('<img src=x');
        expect(result.confirm.img).toBe(0);
        expect(result.promptValue).toContain('"><img src=x');
        expect(result.promptImg).toBe(0);
        expect(result.raw.bold).toBe(1);
        expect(result.raw.text).not.toContain('<b>');
    });
});

// Clinical documents are rendered as formatted content on purpose, so they are
// sanitized (FHIR narratives) or isolated (CDA rendering) rather than escaped.
test.describe('Clinical document content cannot run script', () => {
    const HOSTILE_NARRATIVE =
        `<div xmlns="http://www.w3.org/1999/xhtml"><b>Allergy</b> to <i>penicillin</i>` +
        `<img src=x onerror="window.__ezhkXss='narr-img'"><script>window.__ezhkXss='narr-script'</script>` +
        `<a href="javascript:window.__ezhkXss='narr-href'" id="clobber">link</a>` +
        `<svg><a href="javascript:alert(1)"><text>x</text></a></svg>` +
        `<p style="background:url(https://evil.example/x)" onclick="window.__ezhkXss='narr-click'">para</p>` +
        `<table><tr><td colspan="2">cell</td></tr></table></div>`;

    test.beforeEach(async ({ page }) => {
        await page.goto('/messages.html?interfaceId=00000000-0000-0000-0000-000000000000');
        // a top-level class is a global binding, not a window property
        await page.waitForFunction(() => window.HtmlSafe && typeof ClinicalDocumentViewer !== 'undefined');
    });

    test('HtmlSafe.sanitize keeps formatting and removes everything executable', async ({ page }) => {
        const out = await page.evaluate((m) => String(HtmlSafe.sanitize(m)), HOSTILE_NARRATIVE);
        expect(out).toContain('<b>Allergy</b>');
        expect(out).toContain('<i>penicillin</i>');
        expect(out).toContain('<td colspan="2">cell</td>');
        expect(out).not.toMatch(/onerror|onclick|<script|javascript:|<svg|url\(|id="clobber"/i);
        expect(out).toContain('<a>link</a>'); // the javascript: href is dropped, the text kept
    });

    test('FHIR narratives render formatted, with nothing executable', async ({ page }) => {
        const result = await page.evaluate(async (narrative) => {
            const mount = document.createElement('div');
            document.body.appendChild(mount);
            const bundle = { resourceType: 'Bundle', entry: [
                { resource: { resourceType: 'AllergyIntolerance', id: 'a1', text: { status: 'generated', div: narrative } } },
            ] };
            await new ClinicalDocumentViewer(mount, { format: 'fhir', parsedContent: bundle }).mount();
            await new Promise(r => setTimeout(r, 500));
            return { fired: window.__ezhkXss || null, html: mount.innerHTML };
        }, HOSTILE_NARRATIVE);
        expect(result.fired).toBeNull();
        expect(result.html).toContain('<b>Allergy</b>');
        expect(result.html).not.toMatch(/onerror|onclick|<script|javascript:/i);
    });

    test('CDA rendering is isolated in a script-less sandbox', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const mount = document.createElement('div');
            mount.style.height = '300px';
            document.body.appendChild(mount);
            const viewer = new ClinicalDocumentViewer(mount, { format: 'ccda', interfaceId: 'x', messageId: 'y' });
            viewer._fetchRawXml = async () =>
                '<ClinicalDocument xmlns="urn:hl7-org:v3"><title>Doc</title><component><structuredBody><component><section>' +
                '<title>Problems</title><text><paragraph>Hostile <linkHtml href="javascript:parent.__ezhkXss=1">x</linkHtml></paragraph></text>' +
                '</section></component></structuredBody></component></ClinicalDocument>';
            await viewer.mount();
            const iframe = mount.querySelector('iframe');
            await new Promise(r => setTimeout(r, 800));
            let reachable = true;
            try { void iframe.contentDocument.body; reachable = !!iframe.contentDocument; } catch (e) { reachable = false; }
            return { sandbox: iframe && iframe.getAttribute('sandbox'), srcdoc: !!(iframe && iframe.srcdoc), reachable, fired: window.__ezhkXss || null };
        });
        expect(result.sandbox).toBe('');      // no scripts, opaque origin
        expect(result.srcdoc).toBe(true);
        expect(result.reachable).toBe(false); // the app cannot be reached from it (nor it from the app)
        expect(result.fired).toBeNull();
        // ...and the document is still shown.
        await expect(page.frameLocator('iframe[title="Clinical document"]').locator('body')).toContainText('Hostile', { timeout: 10_000 });
    });
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
        if (!interfaceId) return;
        const db = new Client({ connectionString: DB_URL });
        await db.connect();
        try { await db.query('DELETE FROM delivery_dlq WHERE interface_id = $1', [interfaceId]); } finally { await db.end(); }
        await request.delete(`/api/interfaces/${interfaceId}`).catch(() => {});
    });

    test('dead-letter queue: link parameter, row and detail render as text', async ({ page }) => {
        // The DLQ page is opened from a link carrying ?message_id= — a crafted
        // link must not run script, and the failed row carries partner data.
        const db = new Client({ connectionString: DB_URL });
        await db.connect();
        try {
            await db.query(
                `INSERT INTO delivery_dlq (message_id, interface_id, connector_type, payload, error_message, step_name, status)
                 VALUES ($1, $2, 'http_outbound', $3, $4, $5, 'pending')
                 ON CONFLICT DO NOTHING`,
                [MESSAGE_ID, interfaceId, `payload ${xss('payload')}`, `Partner said ${xss('dlqerr')}`, `step ${xss('step')}`]);
        } finally {
            await db.end();
        }
        const dialogs = [];
        page.on('dialog', d => { dialogs.push(d.message()); d.dismiss().catch(() => {}); });
        await page.goto(`/admin-dlq.html?message_id=${encodeURIComponent(MESSAGE_ID)}`);
        const row = page.locator('#dlqTableBody tr').filter({ hasText: MARK }).first();
        await expect(row).toBeVisible({ timeout: 20_000 });
        expect(await row.innerText()).toContain('<img src=x onerror=');
        await expect(row.locator('.btn-redrive')).toHaveCount(1);
        await row.locator('.btn-secondary').click();
        await expect(page.locator('#drawerContent')).toContainText(`dlqerr`, { timeout: 10_000 });
        await page.waitForTimeout(800);
        expect(await page.evaluate(() => window.__ezhkXss || null), 'injected markup executed').toBeNull();
        expect(dialogs).toEqual([]);
        expect(await page.locator(`img[onerror*="${MARK}"]`).count()).toBe(0);
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
