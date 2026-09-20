'use strict';
/**
 * Playwright E2E — ATNA Audit Export settings section (settings.html)
 *
 * TC-ATNA-001  Nav item and section render, section has all expected fields
 * TC-ATNA-002  Selecting TLS protocol reveals the skip-verify toggle; UDP hides it
 * TC-ATNA-003  Enabling without a host shows a client-side validation error, no save
 * TC-ATNA-004  A full save persists — reloading the section shows the saved values
 * TC-ATNA-005  Settings API GET returns 200 with the expected shape
 */
const { test, expect } = require('@playwright/test');

test.describe('Settings — ATNA Audit Export', () => {

    test.beforeEach(async ({ page }) => {
        await page.goto('/settings.html');
        await page.waitForLoadState('domcontentloaded');
        await page.waitForTimeout(1000);
        await page.locator('.settings-nav-item[data-section="atna"]').click();
        await expect(page.locator('#section-atna')).toBeVisible();
    });

    test('TC-ATNA-001 section renders with all expected fields', async ({ page }) => {
        await expect(page.locator('#section-atna h2')).toContainText('ATNA Audit Export');
        for (const id of ['atnaEnabled', 'atnaHost', 'atnaPort', 'atnaProtocol', 'atnaFacility', 'atnaAppName', 'atnaAuditSourceId', 'atnaEnterpriseSiteId']) {
            await expect(page.locator(`#${id}`)).toBeAttached();
        }
        await expect(page.locator('#atnaSaveBtn')).toBeVisible();
    });

    test('TC-ATNA-002 TLS protocol reveals skip-verify toggle, UDP hides it', async ({ page }) => {
        const skipVerifyGroup = page.locator('#atnaTLSSkipVerifyGroup');
        await expect(skipVerifyGroup).toBeHidden();

        await page.locator('#atnaProtocol').selectOption('tls');
        await expect(skipVerifyGroup).toBeVisible();

        await page.locator('#atnaProtocol').selectOption('udp');
        await expect(skipVerifyGroup).toBeHidden();
    });

    test('TC-ATNA-003 enabling without a host shows a validation error, does not save', async ({ page }) => {
        // #atnaEnabled is `display:none` (a genuinely custom-styled toggle, not
        // just visually disguised) — clicking its visible sibling .toggle-track
        // inside the same <label> triggers it via native label click-forwarding,
        // exactly what a real user does; .check({force:true}) can't click a
        // zero-size element at all.
        const toggle = page.locator('#atnaEnabled + .toggle-track');
        await page.locator('#atnaHost').fill('');
        await expect(page.locator('#atnaEnabled')).not.toBeChecked();
        await toggle.click();
        await expect(page.locator('#atnaEnabled')).toBeChecked();

        await page.locator('#atnaSaveBtn').click();
        await expect(page.locator('#atnaStatus')).toContainText(/host is required/i, { timeout: 5000 });

        // Must not have actually enabled it — leave in a safe, disabled state for real use.
        await toggle.click();
        await expect(page.locator('#atnaEnabled')).not.toBeChecked();
    });

    test('TC-ATNA-004 a full save persists across reload', async ({ page }) => {
        const uniqueSiteId = `pw-test-site-${Date.now()}`;

        await page.locator('#atnaEnabled').uncheck(); // stays disabled — no real target configured
        await page.locator('#atnaHost').fill('pw-test-arr.example.internal');
        await page.locator('#atnaPort').fill('2514');
        await page.locator('#atnaProtocol').selectOption('tcp');
        await page.locator('#atnaFacility').fill('4');
        await page.locator('#atnaAppName').fill('PWTestApp');
        await page.locator('#atnaAuditSourceId').fill('PWTestSource');
        await page.locator('#atnaEnterpriseSiteId').fill(uniqueSiteId);

        await page.locator('#atnaSaveBtn').click();
        await expect(page.locator('#atnaStatus')).toContainText(/saved/i, { timeout: 5000 });

        // Reload the page fresh and re-open the section to confirm real persistence,
        // not just in-memory form state.
        await page.reload();
        await page.waitForLoadState('domcontentloaded');
        await page.waitForTimeout(1000);
        await page.locator('.settings-nav-item[data-section="atna"]').click();
        await expect(page.locator('#section-atna')).toBeVisible();

        await expect(page.locator('#atnaEnabled')).not.toBeChecked();
        await expect(page.locator('#atnaHost')).toHaveValue('pw-test-arr.example.internal');
        await expect(page.locator('#atnaPort')).toHaveValue('2514');
        await expect(page.locator('#atnaProtocol')).toHaveValue('tcp');
        await expect(page.locator('#atnaFacility')).toHaveValue('4');
        await expect(page.locator('#atnaAppName')).toHaveValue('PWTestApp');
        await expect(page.locator('#atnaAuditSourceId')).toHaveValue('PWTestSource');
        await expect(page.locator('#atnaEnterpriseSiteId')).toHaveValue(uniqueSiteId);

        // Leave the row in the clean, disabled, default state for real use afterward.
        await page.locator('#atnaHost').fill('');
        await page.locator('#atnaPort').fill('514');
        await page.locator('#atnaProtocol').selectOption('udp');
        await page.locator('#atnaFacility').fill('10');
        await page.locator('#atnaAppName').fill('ezHealthKonnect');
        await page.locator('#atnaAuditSourceId').fill('ezHealthKonnect');
        await page.locator('#atnaEnterpriseSiteId').fill('');
        await page.locator('#atnaSaveBtn').click();
        await expect(page.locator('#atnaStatus')).toContainText(/saved/i, { timeout: 5000 });
    });

    test('TC-ATNA-005 settings API returns 200 with expected shape', async ({ page }) => {
        const res = await page.request.get('/api/system/settings/atna-syslog');
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.data).toHaveProperty('enabled');
        expect(body.data).toHaveProperty('host');
        expect(body.data).toHaveProperty('protocol');
    });
});
