// Code Translations page: add / edit / search / delete, hostile text, CSV import and export,
// per-interface rows, and a viewer who may look but not change. Real data through the real API.
const { test, expect } = require('./helpers/seeded');
const { createUserWithRole, deleteUser, loginWithRetry } = require('./helpers/users');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const API = `${BASE_URL}/api/cda/code-translations`;
const ANON = { cookies: [], origins: [] };

test.describe('Code Translations', () => {
    // Everything this run creates uses its own source system, so cleanup and searches never touch other data.
    const system = `urn:pw:lab-${Date.now()}`;
    let viewer;

    test.beforeAll(async ({ seed }) => {
        viewer = await createUserWithRole(seed.api.request, BASE_URL, 'viewer', '-tr');
    });

    test.afterAll(async ({ seed }) => {
        const admin = seed.api.request;
        const res = await admin.get(`${API}?search=${encodeURIComponent(system)}&limit=500`);
        if (res.ok()) for (const row of (await res.json()).data) await admin.delete(`${API}/${row.id}`);
        if (viewer) await deleteUser(admin, BASE_URL, viewer.id);
    });

    const dialog = (page) => page.locator('.app-dialog-box');
    const field = (page, name) => page.locator(`#_appDlgField_${name}`);
    const fillRow = async (page, v) => {
        for (const [k, val] of Object.entries(v)) await field(page, k).fill(val);
    };
    const rowFor = (page, code) => page.locator('#tableArea tbody tr', { hasText: code });

    test('add, edit, search and delete a shared translation; hostile text stays text', async ({ page }) => {
        const pageErrors = [];
        page.on('pageerror', e => pageErrors.push(e.message));
        await page.goto(`${BASE_URL}/code-translations.html`);
        await expect(page.locator('#addBtn')).toBeEnabled();

        // Add
        await page.locator('#addBtn').click();
        await fillRow(page, {
            sourceSystem: system, sourceCode: 'GLU',
            targetSystem: 'http://loinc.org', targetCode: '2345-7',
            targetDisplay: '<img src=x onerror="window.__xss=1">Glucose'
        });
        await dialog(page).getByRole('button', { name: 'Add translation' }).click();
        await expect(dialog(page)).toBeHidden();
        await expect(page.locator('.app-toast-success')).toContainText('Translation added');

        await page.locator('#searchInput').fill(system);
        const row = rowFor(page, 'GLU');
        await expect(row).toHaveCount(1);
        await expect(row.locator('.badge-shared')).toHaveText('Shared');
        // The hostile display text is shown literally and never became an element.
        await expect(row).toContainText('<img src=x onerror="window.__xss=1">Glucose');
        expect(await page.locator('#tableArea img').count()).toBe(0);
        expect(await page.evaluate(() => window.__xss)).toBeUndefined();

        // The same source code again is refused with a clear message, and the dialog stays open.
        await page.locator('#addBtn').click();
        await fillRow(page, { sourceSystem: system, sourceCode: 'GLU', targetSystem: 'http://loinc.org', targetCode: '9999-9' });
        await dialog(page).getByRole('button', { name: 'Add translation' }).click();
        await expect(dialog(page).locator('[role="alert"]')).toContainText('already exists');
        await dialog(page).getByRole('button', { name: 'Cancel' }).click();

        // Required fields are checked before anything is sent.
        await page.locator('#addBtn').click();
        await dialog(page).getByRole('button', { name: 'Add translation' }).click();
        await expect(dialog(page).locator('[role="alert"]')).toHaveText('Source system is required');
        await dialog(page).getByRole('button', { name: 'Cancel' }).click();

        // Edit
        await row.getByRole('button', { name: 'Edit' }).click();
        await field(page, 'targetCode').fill('2339-0');
        await dialog(page).getByRole('button', { name: 'Save changes' }).click();
        await expect(dialog(page)).toBeHidden();
        await expect(rowFor(page, 'GLU')).toContainText('2339-0');

        // Search that matches nothing
        await page.locator('#searchInput').fill(`${system}-nothing`);
        await expect(page.locator('.tr-empty')).toContainText('No translations match');
        await page.locator('#searchInput').fill(system);

        // Delete
        await rowFor(page, 'GLU').getByRole('button', { name: 'Delete' }).click();
        await dialog(page).getByRole('button', { name: 'Delete' }).click();
        await expect(page.locator('.app-toast-success').last()).toContainText('Translation deleted');
        await expect(rowFor(page, 'GLU')).toHaveCount(0);
        expect(pageErrors).toEqual([]);
    });

    test('a translation can belong to one interface and is listed under it', async ({ page, seed }) => {
        await page.goto(`${BASE_URL}/code-translations.html`);
        await page.locator('#addBtn').click();
        await field(page, 'interfaceId').selectOption(seed.interfaceId);
        await fillRow(page, { sourceSystem: system, sourceCode: 'IFACE1', targetSystem: 'http://loinc.org', targetCode: '1111-1' });
        await dialog(page).getByRole('button', { name: 'Add translation' }).click();
        await expect(dialog(page)).toBeHidden();

        // Not in the shared list, present under its interface.
        await page.locator('#scopeSelect').selectOption('global');
        await page.locator('#searchInput').fill(system);
        await expect(rowFor(page, 'IFACE1')).toHaveCount(0);

        await page.locator('#scopeSelect').selectOption(seed.interfaceId);
        const row = rowFor(page, 'IFACE1');
        await expect(row).toHaveCount(1);
        await expect(row.locator('.badge-iface')).toBeVisible();
    });

    test('CSV import saves good rows, reports bad ones, updates on re-import; export round-trips', async ({ page }) => {
        const csv = [
            'sourceSystem,sourceCode,targetSystem,targetCode,targetDisplay',
            `${system},NA,http://loinc.org,2951-2,Sodium`,
            `${system},K,http://loinc.org,2823-3,"Potassium, serum"`,
            `${system},BAD,http://loinc.org,,missing target code`
        ].join('\n');

        await page.goto(`${BASE_URL}/code-translations.html`);
        await page.locator('#importFile').setInputFiles({ name: 'labs.csv', mimeType: 'text/csv', buffer: Buffer.from(csv) });
        await expect(dialog(page)).toContainText('Import 3 translations');
        await dialog(page).getByRole('button', { name: 'Import' }).click();

        // The bad row is named, with its row number in the file (the header is row 1).
        await expect(dialog(page)).toContainText('2 saved, 1 skipped');
        await expect(dialog(page)).toContainText('Row 4: Target code is required');
        await dialog(page).getByRole('button', { name: 'OK' }).click();

        // Shared translations only (an earlier test leaves an interface-scoped row under the same system).
        await page.locator('#scopeSelect').selectOption('global');
        await page.locator('#searchInput').fill(system);
        await expect(rowFor(page, 'NA')).toHaveCount(1);
        await expect(rowFor(page, 'K')).toContainText('Potassium, serum'); // quoted comma survived

        // Importing the same file again updates; it does not duplicate.
        await page.locator('#importFile').setInputFiles({ name: 'labs.csv', mimeType: 'text/csv', buffer: Buffer.from(csv) });
        await dialog(page).getByRole('button', { name: 'Import' }).click();
        await dialog(page).getByRole('button', { name: 'OK' }).click();
        await expect(page.locator('#summary')).toHaveText(/^2 translations$/);

        // A file with the wrong header is refused clearly.
        await page.locator('#importFile').setInputFiles({ name: 'bad.csv', mimeType: 'text/csv', buffer: Buffer.from('a,b\n1,2\n') });
        await expect(dialog(page)).toContainText('The first row must be a header');
        await dialog(page).getByRole('button', { name: 'OK' }).click();

        // Export what the current filter shows.
        const [download] = await Promise.all([page.waitForEvent('download'), page.locator('#exportBtn').click()]);
        expect(download.suggestedFilename()).toBe('code-translations.csv');
        const text = require('fs').readFileSync(await download.path(), 'utf8');
        expect(text).toContain('sourceSystem,sourceCode,targetSystem,targetCode,targetDisplay');
        expect(text).toContain(`${system},NA,http://loinc.org,2951-2,Sodium`);
        expect(text).toContain('"Potassium, serum"');
    });

    test('a viewer can read translations but cannot change them', async ({ browser, playwright, seed }) => {
        const created = await seed.api.request.post(API, { data: {
            sourceSystem: system, sourceCode: 'VIEW1', targetSystem: 'http://loinc.org', targetCode: '3333-3' } });
        expect(created.status()).toBe(201);

        const api = await playwright.request.newContext({ baseURL: BASE_URL, storageState: ANON });
        const login = await loginWithRetry(api, { data: { email: viewer.email, password: viewer.password } });
        expect(login.ok()).toBeTruthy();
        const state = await api.storageState();

        // The server refuses writes for a viewer and allows the read.
        expect((await api.get(`${API}?search=VIEW1`)).ok()).toBeTruthy();
        const refused = await api.post(API, { data: { sourceSystem: system, sourceCode: 'NOPE', targetSystem: 's', targetCode: 'c' } });
        expect(refused.status()).toBe(403);
        await api.dispose();

        // The page shows the rows and offers no usable write controls.
        const context = await browser.newContext({ storageState: state });
        const page = await context.newPage();
        try {
            await page.goto(`${BASE_URL}/code-translations.html`);
            await page.locator('#searchInput').fill(system);
            await expect(rowFor(page, 'VIEW1')).toHaveCount(1);
            await expect(page.locator('#addBtn')).toHaveAttribute('aria-disabled', 'true');
            await expect(page.locator('#importBtn')).toHaveAttribute('aria-disabled', 'true');
            await expect(rowFor(page, 'VIEW1').getByRole('button', { name: 'Delete' })).toHaveAttribute('aria-disabled', 'true');
        } finally {
            await context.close();
        }
    });
});
