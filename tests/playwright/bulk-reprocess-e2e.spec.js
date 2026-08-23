/**
 * Bulk Reprocess (V213) — E2E tests
 *
 * Covers the Messages page's checkbox/select-all-matching selection, the
 * Mirth-style typed-confirmation ("type REPROCESS to confirm"), and a real
 * end-to-end job against live data — not a mock, an actual
 * bulk_reprocess_jobs row processed by the real Go background worker
 * (services/bulk_reprocess_service.go).
 *
 * Test interface: "Test Interface5" (085d4474-bc07-449f-9676-dcebf726292c),
 * owned by the Playwright admin test account, 40 real messages all in
 * status=processed/delivery_status=delivered.
 */
const { test, expect } = require('@playwright/test');

test.use({ storageState: '.auth/user.json' });

const TEST_INTERFACE_ID = '085d4474-bc07-449f-9676-dcebf726292c';

// The page's own setDefaultDateFilter() defaults Date From to "last 1 day" — this
// interface's 40 real messages are all from 2026-07-04, well outside that window,
// so every test needs to widen the date filter before any rows will render. This
// is pre-existing app behavior, not something these tests are working around a bug
// in — a real user would hit the exact same "0 messages" until they cleared filters.
async function gotoAndWidenDateFilter(page) {
    await page.goto(`http://localhost:3000/messages.html?interfaceId=${TEST_INTERFACE_ID}`);
    await page.waitForSelector('#filterDateFrom');
    await page.fill('#filterDateFrom', '2020-01-01T00:00');
    await page.click('button:has(i.fa-filter)');
    await page.waitForSelector('.bulk-row-checkbox');
}

test.describe('Bulk Reprocess (V213)', () => {
    test('BR-001 checkbox selection shows the bulk toolbar with the correct count', async ({ page }) => {
        await gotoAndWidenDateFilter(page);

        const checkboxes = page.locator('.bulk-row-checkbox');
        await expect(checkboxes.first()).toBeVisible();
        await checkboxes.nth(0).check();
        await checkboxes.nth(1).check();

        await expect(page.locator('#bulkSelectionToolbar')).toBeVisible();
        await expect(page.locator('#bulkSelectionCount')).toHaveText('2 selected');
    });

    test('BR-002 select-all-on-page then select-all-matching-filter uses the real count API', async ({ page }) => {
        await gotoAndWidenDateFilter(page);

        await page.check('#selectAllOnPageCheckbox');
        await expect(page.locator('#bulkSelectAllMatchingLink')).toBeVisible();

        await page.click('#bulkSelectAllMatchingLink');
        await expect(page.locator('#bulkSelectionCount')).toContainText('selected (all matching current filters)');
        // The API-driven count must be a real positive number, not a placeholder.
        const text = await page.locator('#bulkSelectionCount').textContent();
        const n = parseInt(text.replace(/,/g, ''), 10);
        expect(n).toBeGreaterThan(0);
    });

    test('BR-003 typed confirmation rejects wrong text — no job starts, selection stays intact', async ({ page }) => {
        await gotoAndWidenDateFilter(page);
        await page.locator('.bulk-row-checkbox').first().check();

        await page.click('button:has-text("Reprocess Selected")');
        const input = page.locator('.app-dialog-input');
        await expect(input).toBeVisible();
        await input.fill('wrong text');
        await page.click('#_appDlgOk');

        // Mismatch aborts before any network call — panel never appears, and the
        // original selection (still exactly 1) survives so the user can retry.
        await expect(page.locator('#bulkJobPanel')).toBeHidden();
        await expect(page.locator('#bulkSelectionCount')).toHaveText('1 selected');
    });

    test('BR-004 cancelling the prompt entirely leaves the selection untouched', async ({ page }) => {
        await gotoAndWidenDateFilter(page);
        await page.locator('.bulk-row-checkbox').first().check();

        await page.click('button:has-text("Reprocess Selected")');
        await expect(page.locator('.app-dialog-input')).toBeVisible();
        await page.click('#_appDlgCancel');

        await expect(page.locator('#bulkJobPanel')).toBeHidden();
        await expect(page.locator('#bulkSelectionCount')).toHaveText('1 selected');
    });

    test('BR-005 real end-to-end: typed REPROCESS creates a job that progresses to a terminal state', async ({ page, request }) => {
        // Real pipeline execution for up to 40 messages can take longer than
        // Playwright's default 30s test timeout even though the individual
        // expect() below is already given 90s — the enclosing test needs its
        // own budget increased too, or it gets killed first regardless.
        test.setTimeout(120000);
        await gotoAndWidenDateFilter(page);

        // Explicit-ID mode (checkbox-driven) — exercises the 'ids' selection_mode
        // path through the real job creation endpoint and Go worker.
        await page.check('#selectAllOnPageCheckbox');
        await page.click('button:has-text("Reprocess Selected")');
        const input = page.locator('.app-dialog-input');
        await input.fill('REPROCESS');
        await page.click('#_appDlgOk');

        await expect(page.locator('#bulkJobPanel')).toBeVisible({ timeout: 10000 });
        await expect(page.locator('#bulkJobLabel')).toHaveText(/Completed|Failed|Cancelled/, { timeout: 100000 });

        const counts = await page.locator('#bulkJobCounts').textContent();
        // "<processed> / <total> — <succeeded> succeeded, <failed> failed"
        expect(counts).toMatch(/\d+ \/ \d+/);

        // Confirm the toolbar cleared and the selection reset after job submission.
        await expect(page.locator('#bulkSelectionToolbar')).toBeHidden();
    });
});
