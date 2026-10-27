// Settings -> Access policy: what each role may SEE, edited in the app instead of in code.
// Real operator and viewer users against an interface and messages owned by the admin.
const { test, expect } = require('./helpers/seeded');
const { createUserWithRole, deleteUser, loginWithRetry } = require('./helpers/users');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const ANON = { cookies: [], origins: [] };
const DEFAULTS = {
    operator: { interfaceScope: 'all', messageContent: 'full' },
    viewer:   { interfaceScope: 'all', messageContent: 'metadata' },
    user:     { interfaceScope: 'own', messageContent: 'full' },
};

test.describe.configure({ mode: 'serial' }); // the tests change one shared setting

test.describe('Access policy', () => {
    let operator, viewer, operatorApi, viewerApi, messageId;

    const putPolicy = (admin, roles) => admin.put(`${BASE_URL}/api/access-policy`, { data: { roles } });

    test.beforeAll(async ({ seed, playwright }) => {
        const admin = seed.api.request;
        expect((await putPolicy(admin, DEFAULTS)).ok()).toBeTruthy(); // start from the shipped defaults
        operator = await createUserWithRole(admin, BASE_URL, 'operator', '-ap');
        viewer = await createUserWithRole(admin, BASE_URL, 'viewer', '-ap');
        // One login per user (login is limited to 10/min/IP); the API contexts keep their session cookie.
        for (const [u, set] of [[operator, (c) => { operatorApi = c; }], [viewer, (c) => { viewerApi = c; }]]) {
            const ctx = await playwright.request.newContext({ baseURL: BASE_URL, storageState: ANON });
            expect((await loginWithRetry(ctx, { data: { email: u.email, password: u.password } })).ok()).toBeTruthy();
            set(ctx);
        }
        messageId = seed.messageIds[0];
    });

    test.afterAll(async ({ seed }) => {
        const admin = seed.api.request;
        await putPolicy(admin, DEFAULTS); // never leave a changed policy behind
        await operatorApi?.dispose();
        await viewerApi?.dispose();
        for (const u of [operator, viewer]) if (u) await deleteUser(admin, BASE_URL, u.id);
    });

    const interfaceIds = async (api) => ((await (await api.get('/api/interfaces')).json()).interfaces || []).map(i => i.id);
    const listMessages = (api, seed) => api.get(`/api/messages/interface/${seed.interfaceId}?limit=5`);
    const raw = (api, seed) => api.get(`/api/messages/${messageId}/raw?interfaceId=${seed.interfaceId}`);

    test('default policy: an operator sees the admin\'s interface and its content; a viewer sees details only', async ({ seed }) => {
        // Operator: everything.
        expect(await interfaceIds(operatorApi)).toContain(seed.interfaceId);
        expect((await listMessages(operatorApi, seed)).ok()).toBeTruthy();
        expect((await raw(operatorApi, seed)).ok()).toBeTruthy();

        // Viewer: the interface and the message list and detail...
        expect(await interfaceIds(viewerApi)).toContain(seed.interfaceId);
        expect((await listMessages(viewerApi, seed)).ok()).toBeTruthy();
        expect((await viewerApi.get(`/api/messages/${messageId}?interfaceId=${seed.interfaceId}`)).ok()).toBeTruthy();

        // ...but not what is inside a message, on any route that returns content.
        const contentRoutes = [
            `/api/messages/${messageId}/raw?interfaceId=${seed.interfaceId}`,
            `/api/messages/${messageId}/transformed`,
            `/api/messages/${messageId}/parsed-content?interfaceId=${seed.interfaceId}`,
            `/api/messages/${messageId}/mapping-log?interfaceId=${seed.interfaceId}`,
            `/api/messages/${messageId}/lineage`,
            `/api/messages/interface/${seed.interfaceId}/message/${messageId}/parsed`,
        ];
        for (const path of contentRoutes) {
            const res = await viewerApi.get(path);
            expect(res.status(), path).toBe(403);
            const body = await res.json();
            expect(body.code, path).toBe('MESSAGE_CONTENT_RESTRICTED');
            expect(body.error, path).toContain('details');
        }
    });

    test('the session tells the page what the role may see', async () => {
        const viewerSession = await (await viewerApi.get('/api/auth/session')).json();
        expect(viewerSession.access).toEqual(DEFAULTS.viewer);
        const operatorSession = await (await operatorApi.get('/api/auth/session')).json();
        expect(operatorSession.access).toEqual(DEFAULTS.operator);
    });

    test('an admin can change a role\'s policy and it applies at once, then restore it', async ({ seed }) => {
        const admin = seed.api.request;

        // Viewer limited to their own interfaces: the admin's interface disappears for them.
        expect((await putPolicy(admin, { ...DEFAULTS, viewer: { interfaceScope: 'own', messageContent: 'metadata' } })).ok()).toBeTruthy();
        expect(await interfaceIds(viewerApi)).not.toContain(seed.interfaceId);
        expect((await listMessages(viewerApi, seed)).status()).toBeGreaterThanOrEqual(400);

        // Viewer allowed full content on all interfaces.
        expect((await putPolicy(admin, { ...DEFAULTS, viewer: { interfaceScope: 'all', messageContent: 'full' } })).ok()).toBeTruthy();
        expect(await interfaceIds(viewerApi)).toContain(seed.interfaceId);
        expect((await raw(viewerApi, seed)).ok()).toBeTruthy();

        // Back to defaults.
        expect((await putPolicy(admin, DEFAULTS)).ok()).toBeTruthy();
        expect((await raw(viewerApi, seed)).status()).toBe(403);
    });

    test('the policy cannot lock administrators out, and invalid values are refused', async ({ seed }) => {
        const admin = seed.api.request;
        // Admin is not in the editable set, so a policy that restricts "everyone else" never touches them.
        const current = await (await admin.get(`${BASE_URL}/api/access-policy`)).json();
        expect(Object.keys(current.data.roles).sort()).toEqual(['operator', 'user', 'viewer']);
        expect((await raw(admin, seed)).ok()).toBeTruthy();

        const bad = await putPolicy(admin, { ...DEFAULTS, viewer: { interfaceScope: 'everything', messageContent: 'full' } });
        expect(bad.status()).toBe(400);
        const missing = await putPolicy(admin, { operator: DEFAULTS.operator });
        expect(missing.status()).toBe(400);
        // Nothing was saved by the refused requests.
        expect((await (await admin.get(`${BASE_URL}/api/access-policy`)).json()).data.roles.viewer).toEqual(DEFAULTS.viewer);
    });

    test('only administrators can read or change the policy', async () => {
        for (const api of [operatorApi, viewerApi]) {
            expect((await api.get('/api/access-policy')).status()).toBe(403);
            expect((await api.put('/api/access-policy', { data: { roles: DEFAULTS } })).status()).toBe(403);
        }
        const anon = await (await require('@playwright/test').request.newContext({ baseURL: BASE_URL, storageState: ANON }));
        try {
            expect((await anon.get('/api/access-policy')).status()).toBe(401);
        } finally { await anon.dispose(); }
    });

    test('the Settings page edits the policy', async ({ page, seed }) => {
        const admin = seed.api.request;
        await page.goto(`${BASE_URL}/settings.html`);
        await page.locator('.settings-nav-item[data-section="access"]').click();
        await expect(page.locator('#section-access')).toBeVisible();
        // Loaded values match what is stored.
        await expect(page.locator('#ap-viewer-content')).toHaveValue('metadata');
        await expect(page.locator('#ap-operator-scope')).toHaveValue('all');

        await page.locator('#ap-viewer-content').selectOption('full');
        await page.locator('#accessSaveBtn').click();
        await expect(page.locator('#accessStatus')).toContainText('Saved');

        const saved = await (await admin.get(`${BASE_URL}/api/access-policy`)).json();
        expect(saved.data.roles.viewer.messageContent).toBe('full');
        expect((await raw(viewerApi, seed)).ok()).toBeTruthy(); // takes effect without a restart

        expect((await putPolicy(admin, DEFAULTS)).ok()).toBeTruthy();
    });

    test('a viewer opening a message sees an explanation instead of empty content', async ({ browser, seed }) => {
        const context = await browser.newContext({ storageState: await viewerApi.storageState() });
        const page = await context.newPage();
        try {
            await page.goto(`${BASE_URL}/messages.html?interfaceId=${seed.interfaceId}`);
            const rows = page.locator('tbody tr.message-row');
            await expect(rows.first()).toBeVisible({ timeout: 20000 });
            await rows.first().click();

            const notice = 'Message content is not available for your role';
            await page.locator('#contentTabBtn').click();
            await expect(page.locator('#messageContentView')).toContainText(notice, { timeout: 15000 });
            await page.locator('#journeyTabBtn').click();
            await expect(page.locator('#messageLineageView')).toContainText(notice, { timeout: 15000 });
            // The overview (details) is still there.
            await page.locator('#overviewTabBtn').click();
            await expect(page.locator('#overviewTab')).toBeVisible();
        } finally {
            await context.close();
        }
    });
});
