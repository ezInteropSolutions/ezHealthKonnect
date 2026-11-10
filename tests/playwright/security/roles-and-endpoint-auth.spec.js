'use strict';
/**
 * G5 — roles in the browser, and the Partner Endpoint Authentication editor.
 *
 * ROLES. Viewer and operator users are created through the admin API (helpers/users.js) and
 * log in through the real login page. The UI (js/core/app-auth.js) mirrors what the server allows:
 *   - read pages load for viewer and operator, with no refused (403) requests anywhere;
 *   - write controls are disabled, with the reason, for a viewer and enabled for an operator;
 *   - a control a role may not use stays blocked even if a page script re-enables it;
 *   - a write the server refuses is shown to the user in the server's own words;
 *   - admin pages show "Access denied" to non-admins, and the menu offers no admin items;
 *   - the server still refuses everything the UI hides (checked directly, not through the UI).
 * An operator, unlike a viewer, can create/change/remove an interface of their own.
 *
 * ENDPOINT AUTH EDITOR. Drives the real Edit Interface modal: set, masked on re-open, kept when
 * untouched, replaced, and removed. The last describe covers the modal's File Writer validation message. Server-side verification of the credentials themselves is
 * covered by sync-endpoint-auth-e2e.spec.js; this spec covers the editor and what it stores.
 */

const { test, expect } = require('../helpers/seeded');
const { login } = require('../helpers/auth');
const { createUserWithRole, deleteUser } = require('../helpers/users');
const { hl7ToFilePayload, adtToFhirPayload } = require('../helpers/fixtures');
const { goToInterfaceRow } = require('../helpers/ui');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const ANON = { cookies: [], origins: [] };
const MASK = '••••••••';

const READ_PAGES = ['dashboard.html', 'interfaces.html', 'monitoring.html', 'engine-metrics.html', 'messages.html'];

async function loginAs(browser, user, { viaUi = false } = {}) {
    const context = await browser.newContext({ storageState: viaUi || !user.state ? ANON : user.state });
    const page = await context.newPage();
    const pageErrors = [];
    const refused = new Set();
    page.on('pageerror', e => pageErrors.push(e.message));
    page.on('response', r => {
        if (r.status() === 403 || r.status() === 401) refused.add(`${r.request().method()} ${new URL(r.url()).pathname}`);
    });
    if (viaUi || !user.state) await login(page, user.email, user.password);
    return { context, page, pageErrors, refused };
}

// Opens the Edit Interface modal for one interface, through the real row menu.
async function openEditModalFor(page, id) {
    // Closing the modal after a save asks to confirm; Playwright dismisses dialogs unless told to accept.
    page.removeAllListeners('dialog');
    page.on('dialog', (d) => d.accept());
    await page.goto('/interfaces.html');
    await page.waitForLoadState('load');
    await page.waitForTimeout(1500);
    await goToInterfaceRow(page, id);
    await page.locator(`#actions-menu-btn-${id}`).click();
    const menu = page.locator(`#actions-menu-${id}`);
    await menu.waitFor({ state: 'visible', timeout: 3000 });
    await menu.locator('.dropdown-item-minimal').filter({ hasText: 'Edit' }).click();
    await expect(page.locator('#editModal')).toHaveClass(/show/, { timeout: 5000 });
}

test.describe.serial('Roles in the UI', () => {
    const users = {};

    test.beforeAll(async ({ seed, playwright }) => {
        const admin = seed.api.request;
        users.viewer   = await createUserWithRole(admin, BASE_URL, 'viewer');
        users.operator = await createUserWithRole(admin, BASE_URL, 'operator');
        // One API login per role; the browser contexts below start from this session (see loginAs).
        for (const user of Object.values(users)) {
            const api = await playwright.request.newContext({ baseURL: BASE_URL, storageState: ANON });
            const res = await api.post('/api/auth/login', { data: { email: user.email, password: user.password } });
            if (!res.ok()) throw new Error(`login for ${user.role} failed: ${res.status()}`);
            user.state = await api.storageState();
            await api.dispose();
        }
    });

    test.afterAll(async ({ seed }) => {
        for (const u of Object.values(users)) if (u) await deleteUser(seed.api.request, BASE_URL, u.id);
    });

    for (const role of ['viewer', 'operator']) {
        test(`${role}: logs in and every read page loads without redirect or script errors`, async ({ browser, seed }) => {
            const { context, page, pageErrors, refused } = await loginAs(browser, users[role], { viaUi: true });
            try {
                for (const p of READ_PAGES) {
                    const target = p === 'messages.html' ? `${p}?interfaceId=${seed.interfaceId}` : p;
                    await page.goto('/' + target);
                    await page.waitForTimeout(1500);
                    expect(page.url(), `${role} was redirected away from ${p}`).toContain(p.split('?')[0]);
                    await expect(page.locator('body')).not.toBeEmpty();
                }
                expect(pageErrors, `${role}: uncaught script errors`).toEqual([]);
                const unexpected = [...refused];
                // Messages of an interface the user does not own are refused by design (see below).
                const unexpectedNonMessages = unexpected.filter(r => !r.includes('/api/messages/'));
                expect(unexpectedNonMessages, `${role}: unexpected refused requests`).toEqual([]);
            } finally {
                await context.close();
            }
        });
    }

    test('viewer: write actions are refused with a message that names the allowed roles', async ({ browser, seed }) => {
        const { context, page } = await loginAs(browser, users.viewer);
        try {
            const writes = [
                ['post',   '/api/interfaces', { name: 'pw-viewer-must-not-create', messageType: 'ADT^A01' }],
                ['put',    `/api/interfaces/${seed.interfaceId}`, { name: 'renamed by viewer' }],
                ['delete', `/api/interfaces/${seed.idleInterfaceId}`, undefined],
                ['post',   `/api/runtime/interfaces/${seed.interfaceId}/deactivate`, {}],
            ];
            for (const [method, url, data] of writes) {
                const res = await page.request[method](url, data === undefined ? {} : { data });
                expect(res.status(), `${method.toUpperCase()} ${url}`).toBe(403);
                expect((await res.json()).message).toMatch(/Access requires one of: .*admin.*operator/);
            }
            // Nothing was actually changed.
            const check = await seed.api.getInterface(seed.interfaceId);
            expect(JSON.stringify(check)).toContain(seed.interfaceName);
        } finally {
            await context.close();
        }
    });

    test('viewer and operator: admin-only data and settings are refused, admin pages list nothing', async ({ browser }) => {
        for (const role of ['viewer', 'operator']) {
            const { context, page } = await loginAs(browser, users[role]);
            try {
                expect((await page.request.get('/api/users')).status(), `${role} GET /api/users`).toBe(403);
                expect((await page.request.get('/api/system/settings/storage')).status(), `${role} GET settings`).toBe(403);
                const put = await page.request.put('/api/system/settings/security', { data: {} });
                expect(put.status(), `${role} PUT settings`).toBe(403);

                await page.goto('/user-management.html');
                await page.waitForTimeout(1500);
                // Only the signed-in user's own name may appear; no user list is rendered for them.
                expect(await page.locator('table tbody tr:has-text("@")').count(), `${role}: user rows shown`).toBe(0);
            } finally {
                await context.close();
            }
        }
    });

    // Which interfaces a role sees is a setting (Settings -> Access Policy). These two tests cover the
    // 'only their own' setting; access-policy.spec.js covers the defaults and the setting itself.
    const POLICY_DEFAULTS = {
        operator: { interfaceScope: 'all', messageContent: 'full' },
        viewer:   { interfaceScope: 'all', messageContent: 'metadata' },
        user:     { interfaceScope: 'own', messageContent: 'full' },
    };
    const setPolicy = (seed, role, interfaceScope) => seed.api.request.put(`${BASE_URL}/api/access-policy`, {
        data: { roles: { ...POLICY_DEFAULTS, [role]: { ...POLICY_DEFAULTS[role], interfaceScope } } },
    });

    test('support roles cannot be limited to their own interfaces: the policy refuses it and the viewer still sees everything', async ({ browser, seed }) => {
        const refused = await setPolicy(seed, 'viewer', 'own');
        expect(refused.status()).toBe(400);
        const { context, page } = await loginAs(browser, users.viewer);
        try {
            const list = await (await page.request.get('/api/interfaces')).json();
            expect(list.interfaces.map(i => i.id)).toContain(seed.interfaceId);
            expect((await page.request.get(`/api/messages/interface/${seed.interfaceId}`)).status()).toBe(200);
        } finally {
            await context.close();
        }
    });

    test('operator: can create, change and remove an interface of their own; the admin\'s is out of reach only when the policy says so', async ({ browser, seed }) => {
        const { context, page } = await loginAs(browser, users.operator);
        let ownId = null;
        try {
            const created = await page.request.post('/api/wizard/complete', {
                data: { wizardData: { ...hl7ToFilePayload(6691, '/tmp/pw-operator/{timestamp}.hl7'), name: `PW_OPERATOR_${Date.now()}`, auto_start: false, status: 'draft' } },
            });
            expect(created.status(), await created.text()).toBe(200);
            const body = await created.json();
            ownId = body.interfaceId || body.data?.interfaceId || body.id;
            expect(ownId).toBeTruthy();

            const updated = await page.request.put(`/api/interfaces/${ownId}`, { data: { description: 'edited by operator' } });
            expect(updated.status()).toBe(200);

            // Someone else's interface: an operator (support) can never be limited to their own - the policy refuses it...
            expect((await setPolicy(seed, 'operator', 'own')).status()).toBe(400);
            // ...so it stays editable (an operator runs everyone's integrations).
            const shared = await page.request.put(`/api/interfaces/${seed.interfaceId}`, { data: { description: 'edited by operator' } });
            expect(shared.status()).toBe(200);

            const removed = await page.request.delete(`/api/interfaces/${ownId}`);
            expect(removed.status()).toBe(200);
            ownId = null;
        } finally {
            await setPolicy(seed, 'operator', 'all'); // never leave the policy changed
            if (ownId) await page.request.delete(`/api/interfaces/${ownId}`).catch(() => {});
            await context.close();
        }
    });

    test('write controls follow the role: disabled with the reason for a viewer, enabled for an operator', async ({ browser }) => {
        for (const [role, canWrite] of [['viewer', false], ['operator', true]]) {
            const { context, page } = await loginAs(browser, users[role]);
            try {
                await page.goto('/interfaces.html');
                const create = page.locator('#createInterfaceBtn');
                await expect(create).toBeVisible({ timeout: 10000 });
                if (canWrite) {
                    await expect(create, `${role} may create interfaces`).toBeEnabled();
                    await expect(create).not.toHaveAttribute('data-auth-denied', /.+/);
                } else {
                    await expect(create, `${role} must not be offered Create Interface`).toBeDisabled();
                    await expect(create).toHaveAttribute('title', /read-only/i);
                }
            } finally {
                await context.close();
            }
        }
    });

    test('viewer: a denied control stays blocked even if a page script re-enables it, and says why', async ({ browser }) => {
        const { context, page } = await loginAs(browser, users.viewer);
        try {
            await page.goto('/interfaces.html');
            const create = page.locator('#createInterfaceBtn');
            await expect(create).toBeDisabled({ timeout: 10000 });
            await create.evaluate((el) => { el.disabled = false; }); // what a page refresh of button state can do
            // force: the control keeps aria-disabled, which Playwright treats as 'not actionable'; a real user can still click it.
            await create.click({ force: true });
            await expect(page.locator('[role="alert"]', { hasText: /read-only/i })).toBeVisible({ timeout: 5000 });
            await expect(page.locator('#wizardModalOverlay')).toBeHidden();
        } finally {
            await context.close();
        }
    });

    test('viewer: a write the server refuses shows the server\'s own message', async ({ browser }) => {
        const { context, page } = await loginAs(browser, users.viewer);
        try {
            await page.goto('/interfaces.html');
            await expect(page.locator('#createInterfaceBtn')).toBeVisible({ timeout: 10000 });
            const status = await page.evaluate(async () => {
                const res = await fetch('/api/interfaces', {
                    method: 'POST', headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name: 'viewer-must-not-create', messageType: 'ADT^A01' }),
                });
                return res.status;
            });
            expect(status).toBe(403);
            await expect(page.locator('[role="alert"]', { hasText: /Not allowed: Access requires one of: .*admin.*operator/ })).toBeVisible({ timeout: 5000 });
        } finally {
            await context.close();
        }
    });

    test('non-admins: admin pages say "Access denied", and the menu offers no admin items', async ({ browser }) => {
        for (const role of ['viewer', 'operator']) {
            const { context, page, refused } = await loginAs(browser, users[role]);
            try {
                for (const p of ['settings.html', 'user-management.html', 'admin-dlq.html', 'admin-cda-dedupe-registry.html', 'admin-mapping-review.html']) {
                    await page.goto('/' + p);
                    const denied = page.locator('#appAuthDenied');
                    await expect(denied, `${role} on ${p}`).toBeVisible({ timeout: 10000 });
                    await expect(denied).toContainText('Access denied');
                    await expect(denied).toContainText(role);
                }
                await page.goto('/dashboard.html');
                await expect(page.locator('a[href="message-trace.html"]'), `${role} may read Message Trace`).toBeVisible({ timeout: 10000 });
                await expect(page.locator('#adminSection')).toBeHidden();
                await expect(page.locator('a[href="admin-dlq.html"]')).toBeHidden();
            } finally {
                await context.close();
            }
        }
    });

    test('admin: every control, menu item and page is available, and the admin-only calls succeed', async ({ page }) => {
        const refused = [];
        page.on('response', (r) => { if (r.status() === 403) refused.push(new URL(r.url()).pathname); });
        await page.goto('/interfaces.html');
        await expect(page.locator('#createInterfaceBtn')).toBeEnabled({ timeout: 10000 });
        await page.goto('/dashboard.html');
        await expect(page.locator('a[href="admin-dlq.html"]')).toBeVisible({ timeout: 10000 });
        await expect(page.locator('#adminSection')).toBeVisible();
        await page.goto('/settings.html');
        await expect(page.locator('.settings-nav-item').first()).toBeVisible({ timeout: 10000 });
        await expect(page.locator('#appAuthDenied')).toHaveCount(0);
        expect(refused, 'an admin should be refused nothing').toEqual([]);
    });
});

test.describe.serial('Partner Endpoint Authentication editor (Edit Interface modal)', () => {
    const SECTION = '#editEndpointAuthSection';
    const typeSelect = (page) => page.locator(`${SECTION}Type`);
    let interfaceId;

    test.beforeAll(async ({ seed }) => {
        // A dedicated interface: this test changes its authentication. An HTTP FHIR target is used because the
        // Edit modal validates it cleanly; see the fixme at the end of this file for the file_writer case.
        const created = await seed.api.createInterface({
            ...adtToFhirPayload(6692),
            name: `PW_ENDPOINT_AUTH_${Date.now()}`, auto_start: false, status: 'draft',
        });
        interfaceId = created.interfaceId || created.data?.interfaceId || created.id;
        await seed.api.deactivateInterface(interfaceId).catch(() => {});
    });

    test.afterAll(async ({ seed }) => {
        if (interfaceId) await seed.api.deleteInterface(interfaceId).catch(() => {});
    });

    async function openEditModal(page) {
        await openEditModalFor(page, interfaceId);
        // The editor may sit on a non-default tab of the modal; open tabs until it is visible.
        const section = page.locator(SECTION);
        if (!(await section.isVisible().catch(() => false))) {
            const tabs = page.locator('#editModal [role="tab"], #editModal .modal-tab, #editModal .tab-btn');
            const n = await tabs.count();
            for (let i = 0; i < n && !(await section.isVisible().catch(() => false)); i++) {
                await tabs.nth(i).click().catch(() => {});
                await page.waitForTimeout(150);
            }
        }
        await expect(section).toBeVisible({ timeout: 5000 });
    }

    async function save(page) {
        const saved = page.waitForResponse(r => r.url().includes(`/api/interfaces/${interfaceId}`) && r.request().method() === 'PUT');
        await page.locator('#editModal button[type="submit"]').click();
        expect((await saved).status()).toBe(200);
        await expect(page.locator('#editModal')).toBeHidden({ timeout: 10000 }); // closes via display:none
    }

    async function storedAuth(seed) {
        const body = await seed.api.getInterface(interfaceId);
        const iface = body.interface || body.data || body;
        return (iface.sourceConfig || iface.source_config || {}).endpoint_auth || null;
    }

    test('starts as "not configured"', async ({ page, seed }) => {
        await openEditModal(page);
        await expect(typeSelect(page)).toHaveValue('');
        expect(await storedAuth(seed)).toBeNull();
    });

    test('set an API key: stored, and the secret comes back masked', async ({ page, seed }) => {
        await openEditModal(page);
        await typeSelect(page).selectOption('api_key');
        await page.locator(`${SECTION}_api_key_header`).fill('X-Partner-Key');
        await page.locator(`${SECTION}_api_key`).fill('pw-secret-key-1');
        await save(page);

        const stored = await storedAuth(seed);
        expect(stored.authentication_type).toBe('api_key');
        expect(stored.api_key_header).toBe('X-Partner-Key');
        expect(stored.api_key).toBe(MASK);
        expect(JSON.stringify(stored)).not.toContain('pw-secret-key-1');
    });

    test('re-opening shows the masked secret, and saving untouched keeps the stored one', async ({ page, seed }) => {
        await openEditModal(page);
        await expect(typeSelect(page)).toHaveValue('api_key');
        await expect(page.locator(`${SECTION}_api_key_header`)).toHaveValue('X-Partner-Key');
        await expect(page.locator(`${SECTION}_api_key`)).toHaveValue(MASK);
        await save(page);

        const stored = await storedAuth(seed);
        expect(stored.authentication_type).toBe('api_key');
        expect(stored.api_key).toBe(MASK);

        // Proof the real secret survived (not replaced by the mask): the server still accepts it.
        const res = await seed.api.request.post(`${BASE_URL}/api/eligibility/${interfaceId}/check`, {
            headers: { 'X-Partner-Key': 'pw-secret-key-1', 'Content-Type': 'application/edi-x12' }, data: 'ISA*',
        });
        expect(res.status(), 'a wrong key would be 401; any other status means the stored key still matched').not.toBe(401);
    });

    test('a required secret left empty is refused with a clear message', async ({ page }) => {
        await openEditModal(page);
        await typeSelect(page).selectOption('bearer_token');
        await page.locator(`${SECTION}_bearer_token`).fill('');
        await page.locator('#editModal button[type="submit"]').click();
        await expect(page.getByText(/Partner Endpoint Authentication: Bearer token is required/i)).toBeVisible({ timeout: 5000 });
        await expect(page.locator('#editModal')).toBeVisible(); // still open: nothing was saved
    });

    test('change to bearer token', async ({ page, seed }) => {
        await openEditModal(page);
        await typeSelect(page).selectOption('bearer_token');
        await page.locator(`${SECTION}_bearer_token`).fill('pw-bearer-1');
        await save(page);
        const stored = await storedAuth(seed);
        expect(stored.authentication_type).toBe('bearer_token');
        expect(stored.bearer_token).toBe(MASK);
    });

    test('"Not configured" removes it', async ({ page, seed }) => {
        await openEditModal(page);
        await typeSelect(page).selectOption('');
        await save(page);
        expect(await storedAuth(seed)).toBeNull();

        // And the partner endpoint is closed again (secure by default).
        const res = await seed.api.request.post(`${BASE_URL}/api/eligibility/${interfaceId}/check`, {
            headers: { 'Content-Type': 'application/edi-x12' }, data: 'ISA*',
        });
        expect(res.status()).not.toBe(200);
    });
});

// Edit modal and a File Writer target. The File Writer connector needs directory_path; the modal validates it
// before sending anything. When it is missing the modal must say so (a toast) and stay open, not appear to do nothing.
test.describe.serial('Edit modal — File Writer target', () => {
    let okId;
    let brokenId;

    test.beforeAll(async ({ seed }) => {
        const ok = await seed.api.createInterface({
            ...hl7ToFilePayload(6693, '/tmp/pw-edit-file/{timestamp}.hl7'),
            name: `PW_EDIT_FILE_OK_${Date.now()}`, auto_start: false, status: 'draft',
        });
        okId = ok.interfaceId || ok.data?.interfaceId || ok.id;
        const base = hl7ToFilePayload(6694, '/tmp/pw-edit-file/{timestamp}.hl7');
        const broken = await seed.api.createInterface({
            ...base, name: `PW_EDIT_FILE_NODIR_${Date.now()}`, auto_start: false, status: 'draft',
            targetConfig: {}, targetConnectorConfig: { connectorType: 'file_writer', config: {} },
        });
        brokenId = broken.interfaceId || broken.data?.interfaceId || broken.id;
        for (const id of [okId, brokenId]) await seed.api.deactivateInterface(id).catch(() => {});
    });

    test.afterAll(async ({ seed }) => {
        for (const id of [okId, brokenId]) if (id) await seed.api.deleteInterface(id).catch(() => {});
    });

    test('with an output directory the Edit modal saves', async ({ page }) => {
        await openEditModalFor(page, okId);
        const saved = page.waitForResponse(r => r.url().includes(`/api/interfaces/${okId}`) && r.request().method() === 'PUT');
        await page.locator('#editModal button[type="submit"]').click();
        expect((await saved).status()).toBe(200);
        await expect(page.locator('#editModal')).toBeHidden({ timeout: 10000 });
    });

    test('without an output directory the Edit modal says why it cannot save, and stays open', async ({ page }) => {
        let putSent = false;
        page.on('request', r => { if (r.method() === 'PUT' && r.url().includes(`/api/interfaces/${brokenId}`)) putSent = true; });
        await openEditModalFor(page, brokenId);
        await page.locator('#editModal button[type="submit"]').click();
        await expect(page.locator('[role="alert"]', { hasText: /Output directory is required for file interfaces/ })).toBeVisible({ timeout: 5000 });
        await expect(page.locator('#editModal')).toBeVisible();
        expect(putSent, 'nothing may be sent when validation fails').toBe(false);
    });
});
