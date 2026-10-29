'use strict';
// The Interfaces page asks the server for one page at a time (filters, sort, paging and the summary-card counts are
// server-side). It used to download every interface on each load and refresh, which got slower with every interface.
const { test, expect } = require('../helpers/seeded');
const { hl7SinkPayload } = require('../helpers/fixtures');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const PREFIX = `PW_PAGING_${Date.now()}`;
const COUNT = 27;   // more than one page of 25
const created = [];

test.describe.serial('Interfaces page: server-side paging', () => {
    test.setTimeout(240_000);

    test.beforeAll(async ({ seed }) => {
        for (let i = 0; i < COUNT; i++) {
            const n = String(i).padStart(2, '0');
            const res = await seed.api.createInterface({ ...hl7SinkPayload(6640 + (i % 20)), name: `${PREFIX}_${n}` });
            created.push(res.interfaceId || res.data?.interfaceId || res.id);
        }
    });

    test.afterAll(async ({ seed }) => {
        for (const id of created) await seed.api.deleteInterface(id).catch(() => {});
    });

    test('the page requests one page, shows the matching total, and Next loads the following one', async ({ page }) => {
        const requests = [];
        page.on('request', (r) => { if (/\/api\/interfaces\?/.test(r.url())) requests.push(r.url()); });
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        await page.locator('#interfaceSearchInput').fill(PREFIX);
        await expect(page.locator('.pagination-info')).toContainText(`of ${COUNT} interfaces`, { timeout: 15_000 });
        await expect(page.locator('#interfacesTableBody tr')).toHaveCount(25);

        const first = requests.find((u) => u.includes('limit=25'));
        expect(first, 'the list is requested with a limit').toBeTruthy();
        expect(requests.every((u) => u.includes('limit=')), `no request fetches the whole list: ${requests.join(' | ')}`).toBeTruthy();

        await page.locator('button[onclick="goToNextPage()"]').click();
        await expect(page.locator('#interfacesTableBody tr')).toHaveCount(COUNT - 25, { timeout: 15_000 });
        expect(requests.some((u) => u.includes('offset=25'))).toBeTruthy();
    });

    test('search narrows on the server and sorting reorders across pages', async ({ page }) => {
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        await page.locator('#interfaceSearchInput').fill(`${PREFIX}_0`);
        await expect(page.locator('.pagination-info')).toContainText('of 10 interfaces', { timeout: 15_000 });   // _00 .. _09

        await page.locator('#interfaceSearchInput').fill(PREFIX);
        await expect(page.locator('.pagination-info')).toContainText(`of ${COUNT}`, { timeout: 15_000 });
        // name ascending (the page default): the first row is _00; descending: _26 comes first, from page 1
        await expect(page.locator('#interfacesTableBody tr').first()).toContainText(`${PREFIX}_00`);
        await page.locator('th.th-sortable', { hasText: 'Interface Name' }).click();
        await expect(page.locator('#interfacesTableBody tr').first()).toContainText(`${PREFIX}_26`, { timeout: 15_000 });
    });

    test('the summary cards come from the server and cover every interface, not only the page', async ({ page, seed }) => {
        const body = await (await seed.api.request.get(`${BASE_URL}/api/interfaces?limit=1&counts=1`)).json();
        expect(body.counts.total).toBeGreaterThanOrEqual(COUNT);
        await page.goto('/interfaces.html', { waitUntil: 'networkidle' });
        await expect(page.locator('#totalInterfaces')).toHaveText(String(body.counts.total), { timeout: 15_000 });
    });

    test('the API refuses to be steered by the sort and filter parameters', async ({ seed }) => {
        for (const q of ["sort=name;DROP TABLE interfaces", "status=a'b", "sourceType=x'--", "format=a b", "dir='"]) {
            const r = await seed.api.request.get(`${BASE_URL}/api/interfaces?limit=5&${q}`);
            expect(r.status(), q).toBe(200);
        }
        const ok = await (await seed.api.request.get(`${BASE_URL}/api/interfaces?limit=5&sort=statistics&dir=desc`)).json();
        expect(ok.success).toBe(true);
    });
});
