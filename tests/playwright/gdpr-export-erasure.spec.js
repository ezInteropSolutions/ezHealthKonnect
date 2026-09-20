'use strict';
/**
 * Playwright E2E — GDPR Article 15/17 export + erasure UI
 *
 * Drives the real "Export Data" / "Request GDPR Deletion" / "Execute Erasure"
 * buttons on user-management.html's Compliance tab through a real browser
 * against the real running app — not just the underlying API.
 *
 * TC-GDPR-001  Export Data downloads a JSON file containing real PII before erasure
 * TC-GDPR-002  Execute Erasure button is disabled until GDPR deletion is flagged
 * TC-GDPR-003  Request GDPR Deletion flags the user and enables Execute Erasure
 * TC-GDPR-004  Execute Erasure requires a non-empty reason
 * TC-GDPR-005  Execute Erasure anonymizes the user; drawer reflects anonymized data
 * TC-GDPR-006  A second Export Data after erasure downloads anonymized data only
 */
const { test, expect } = require('@playwright/test');
const { login } = require('./helpers/auth');

let testUserId;
let testUserEmail;

test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage();
    await login(page);

    testUserEmail = `pw-gdpr-test-${Date.now()}@example.com`;
    const resp = await page.request.post('/api/users', {
        data: { email: testUserEmail, password: 'TestPassword123!', name: 'PW GDPR TestSubject', role: 'user', phone: '555-9999', organization: 'PW Test Org' },
    });
    expect(resp.ok()).toBeTruthy();
    const body = await resp.json();
    testUserId = body.user.id;

    await page.close();
});

test.afterAll(async ({ browser }) => {
    if (!testUserId) return;
    const page = await browser.newPage();
    await login(page);
    await page.request.delete(`/api/users/${testUserId}`).catch(() => {});
    await page.close();
});

test.beforeEach(async ({ page }) => {
    await login(page);
    await page.goto('/user-management.html');
    await page.waitForLoadState('networkidle');
});

async function openTestUserDrawer(page) {
    // Search narrows the table to just our throwaway user, avoiding pagination issues.
    const search = page.locator('#um-search, input[type="search"], input[placeholder*="Search" i]').first();
    if (await search.count()) {
        await search.fill(testUserEmail);
        await page.waitForTimeout(500);
    }
    const link = page.locator('.um-user-name-link', { hasText: 'PW GDPR TestSubject' }).first();
    await expect(link).toBeVisible({ timeout: 10_000 });
    await link.click();
    await expect(page.locator('#um-drawer')).toBeVisible();
    await page.locator('.um-drawer-tab[data-dtab="compliance"]').click();
    await expect(page.locator('#dtab-compliance')).toBeVisible();
}

test('TC-GDPR-001/002: Export Data downloads real PII; Execute Erasure disabled before flagging', async ({ page }) => {
    await openTestUserDrawer(page);

    const eraseBtn = page.locator('#dc-btn-gdpr-erase');
    await expect(eraseBtn).toBeDisabled();

    const [download] = await Promise.all([
        page.waitForEvent('download'),
        page.locator('#dc-btn-gdpr-export').click(),
    ]);
    const downloadPath = await download.path();
    const fs = require('fs');
    const content = JSON.parse(fs.readFileSync(downloadPath, 'utf8'));
    expect(content.profile.email).toBe(testUserEmail);
    expect(content.profile.first_name).toBe('PW');
    expect(Array.isArray(content.auditTrail)).toBe(true);
});

test('TC-GDPR-003/004/005/006: full flag -> erase (with reason) -> anonymized flow', async ({ page }) => {
    await openTestUserDrawer(page);

    // Step 1: flag for GDPR deletion
    page.once('dialog', d => d.accept()); // native confirm fallback, harmless if AppDialogs handles it in-page
    await page.locator('#dc-btn-gdpr').click();
    // AppDialogs.confirm renders an in-page overlay, not a native dialog — click its own confirm button.
    const confirmBtn = page.locator('.app-dialog-btn-confirm, #_appDlgOk, button:has-text("Flag for Deletion")').first();
    if (await confirmBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
        await confirmBtn.click();
    }
    await expect(page.locator('#dc-gdpr-status')).toHaveText(/Requested/, { timeout: 10_000 });
    await expect(page.locator('#dc-btn-gdpr-erase')).toBeEnabled({ timeout: 10_000 });

    // Step 2: attempt erasure with an EMPTY reason — should be rejected client-side
    await page.locator('#dc-btn-gdpr-erase').click();
    const reasonInput = page.locator('#_appDlgInput');
    await expect(reasonInput).toBeVisible({ timeout: 5000 });
    await reasonInput.fill('');
    await page.locator('#_appDlgOk').click();
    // Empty reason should surface an alert and the erase button must remain enabled (no erasure happened)
    await expect(page.locator('#dc-btn-gdpr-erase')).toBeEnabled();

    // Step 3: real erasure with a real reason
    await page.locator('#dc-btn-gdpr-erase').click();
    await expect(page.locator('#_appDlgInput')).toBeVisible({ timeout: 5000 });
    await page.locator('#_appDlgInput').fill('Playwright E2E — data subject request #PW-TEST');
    await page.locator('#_appDlgOk').click();

    const secondConfirm = page.locator('.app-dialog-btn-confirm, button:has-text("Anonymize Permanently")').first();
    await expect(secondConfirm).toBeVisible({ timeout: 5000 });
    await secondConfirm.click();

    // Step 4: drawer should now show anonymized data
    await expect(page.locator('#dp-email')).toHaveValue(/anonymized\.invalid/, { timeout: 10_000 });
    await expect(page.locator('#dp-first-name')).toHaveValue('Deleted');
    await expect(page.locator('#dc-btn-gdpr-erase')).toBeDisabled();

    // Step 5: a fresh export now must only contain anonymized data
    const [download2] = await Promise.all([
        page.waitForEvent('download'),
        page.locator('#dc-btn-gdpr-export').click(),
    ]);
    const fs = require('fs');
    const content2 = JSON.parse(fs.readFileSync(await download2.path(), 'utf8'));
    expect(content2.profile.email).not.toBe(testUserEmail);
    expect(content2.profile.email).toMatch(/anonymized\.invalid/);
    expect(content2.profile.first_name).toBe('Deleted');
});
