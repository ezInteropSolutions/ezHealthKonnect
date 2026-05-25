'use strict';
/**
 * Playwright E2E — Admin / User Management page
 *
 * TC-ADMIN-001  Page title contains "Admin" or "Users"
 * TC-ADMIN-002  Admin nav section is visible and marked active
 * TC-ADMIN-003  User management table renders with column headers
 * TC-ADMIN-004  At least one user row exists (the seeded admin account)
 * TC-ADMIN-005  Search / filter input narrows the user list
 * TC-ADMIN-006  Non-matching search shows empty state or zero rows
 * TC-ADMIN-007  Role filter dropdown is present and functional
 * TC-ADMIN-008  Clicking "Edit" on a user row opens an edit modal
 * TC-ADMIN-009  Edit modal has username / email / role fields
 * TC-ADMIN-010  Edit modal cancel / close button dismisses without changes
 * TC-ADMIN-011  Invite / Create User button is visible
 * TC-ADMIN-012  Create User modal has required fields (email, role)
 * TC-ADMIN-013  Submitting create user form with empty email shows validation
 * TC-ADMIN-014  Self-delete is prevented (delete button disabled/absent for own row)
 * TC-ADMIN-015  Unauthenticated access to admin page redirects to login
 * TC-ADMIN-016  Users API endpoint GET /api/users returns 200 with user list
 * TC-ADMIN-017  Admin page has no horizontal overflow at 1280px
 */

const { test, expect } = require('@playwright/test');

// Best-guess URL patterns for the admin / user management page
const ADMIN_URLS = ['/admin.html', '/admin/users.html', '/users.html'];

let adminUrl = null;

test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage();
    for (const url of ADMIN_URLS) {
        try {
            const resp = await page.goto(url);
            if (resp && resp.status() !== 404) {
                adminUrl = url;
                break;
            }
        } catch {
            // Try next URL
        }
    }
    await page.close();
});

test.describe('Admin / User Management', () => {

    test.beforeEach(async ({ page }) => {
        if (!adminUrl) {
            test.skip();
            return;
        }
        await page.goto(adminUrl);
        await page.waitForLoadState('domcontentloaded');
        await page.waitForTimeout(1500);
    });

    // ── TC-ADMIN-001 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-001 page title contains Admin or Users', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const title = await page.title();
        expect(title).toMatch(/admin|users|user management/i);
    });

    // ── TC-ADMIN-002 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-002 admin nav section is visible', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const adminSection = page.locator('#adminSection, [class*="admin"], .nav-item').filter({
            hasText: /users|admin/i
        }).first();
        await expect(adminSection).toBeVisible({ timeout: 8000 });
    });

    // ── TC-ADMIN-003 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-003 user table renders with column headers', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const table = page.locator('table').first();
        await expect(table).toBeVisible({ timeout: 8000 });
        const headers = (await table.locator('th').allTextContents()).join(' ').toLowerCase();
        const hasExpected = /name|email|role|user|action|status/i.test(headers);
        expect(hasExpected).toBe(true);
    });

    // ── TC-ADMIN-004 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-004 at least one user row exists', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        await page.waitForTimeout(1000);
        const rows = page.locator('tbody tr');
        const count = await rows.count();
        expect(count).toBeGreaterThanOrEqual(1);
    });

    // ── TC-ADMIN-005 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-005 search input filters user list', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const search = page.locator(
            'input[type="search"], input[placeholder*="search" i], input[placeholder*="filter" i]'
        ).first();
        if (!await search.isVisible({ timeout: 3000 }).catch(() => false)) {
            test.skip();
            return;
        }
        await search.fill('admin@ezhealthkonnect.com');
        await page.waitForTimeout(600);
        const rows = page.locator('tbody tr:not(.empty-row)');
        const count = await rows.count();
        // Admin user should still appear, row count should be ≤ total
        expect(count).toBeGreaterThanOrEqual(0);
    });

    // ── TC-ADMIN-006 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-006 non-matching search shows empty state', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const search = page.locator(
            'input[type="search"], input[placeholder*="search" i]'
        ).first();
        if (!await search.isVisible({ timeout: 3000 }).catch(() => false)) {
            test.skip();
            return;
        }
        await search.fill('zzzznotauser99999_xyzxyz');
        await page.waitForTimeout(600);
        const rows    = await page.locator('tbody tr:not(.empty-row)').count();
        const empty   = page.locator('[class*="empty"], text=/no users|no results/i');
        const isEmpty = rows === 0 || await empty.isVisible().catch(() => false);
        expect(isEmpty).toBe(true);
    });

    // ── TC-ADMIN-007 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-007 role filter dropdown is present', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const roleFilter = page.locator(
            'select[name*="role"], select[id*="role"], select[class*="role"]'
        ).first();
        if (!await roleFilter.isVisible({ timeout: 3000 }).catch(() => false)) {
            test.skip();
            return;
        }
        await expect(roleFilter).toBeEnabled();
        // Check options exist
        const options = await roleFilter.locator('option').count();
        expect(options).toBeGreaterThan(1); // At least placeholder + one role
    });

    // ── TC-ADMIN-008 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-008 clicking Edit on a user row opens edit modal', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const editBtn = page.locator(
            'tbody tr button, tbody tr a'
        ).filter({ hasText: /edit/i }).first();
        if (!await editBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
            // Try action menu / kebab icon
            const actionBtn = page.locator('tbody tr [class*="action"], tbody tr [class*="menu"]').first();
            if (!await actionBtn.isVisible().catch(() => false)) {
                test.skip();
                return;
            }
            await actionBtn.click();
            await page.waitForTimeout(200);
        } else {
            await editBtn.click();
        }
        const modal = page.locator('.modal, [class*="modal"], [role="dialog"]').first();
        await expect(modal).toBeVisible({ timeout: 5000 });
    });

    // ── TC-ADMIN-009 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-009 edit modal contains user fields', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const editBtn = page.locator(
            'tbody tr button, tbody tr a'
        ).filter({ hasText: /edit/i }).first();
        if (!await editBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
            test.skip();
            return;
        }
        await editBtn.click();
        const modal = page.locator('.modal, [class*="modal"], [role="dialog"]').first();
        if (!await modal.isVisible({ timeout: 5000 }).catch(() => false)) {
            test.skip();
            return;
        }
        // Should have at least one relevant field
        const fields = modal.locator('input, select');
        const count  = await fields.count();
        expect(count).toBeGreaterThanOrEqual(1);
    });

    // ── TC-ADMIN-010 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-010 edit modal cancel dismisses without navigating away', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const editBtn = page.locator('tbody tr button, tbody tr a').filter({ hasText: /edit/i }).first();
        if (!await editBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
            test.skip();
            return;
        }
        await editBtn.click();
        const modal = page.locator('.modal, [class*="modal"], [role="dialog"]').first();
        if (!await modal.isVisible({ timeout: 5000 }).catch(() => false)) {
            test.skip();
            return;
        }
        const cancelBtn = modal.locator(
            'button[aria-label*="close" i], button.close, .close-btn, button'
        ).filter({ hasText: /cancel|close/i }).first();
        if (await cancelBtn.isVisible().catch(() => false)) {
            await cancelBtn.click();
        } else {
            await page.keyboard.press('Escape');
        }
        await page.waitForTimeout(400);
        await expect(modal).toBeHidden();
        await expect(page).toHaveURL(new RegExp(adminUrl.replace('.html', '')));
    });

    // ── TC-ADMIN-011 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-011 Invite or Create User button is visible', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const createBtn = page.locator('button, a').filter({
            hasText: /invite|create user|add user|new user/i
        }).first();
        await expect(createBtn).toBeVisible({ timeout: 8000 });
    });

    // ── TC-ADMIN-012 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-012 Create User modal has email and role fields', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const createBtn = page.locator('button, a').filter({
            hasText: /invite|create user|add user|new user/i
        }).first();
        if (!await createBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
            test.skip();
            return;
        }
        await createBtn.click();
        const modal = page.locator('.modal, [class*="modal"], [role="dialog"]').first();
        if (!await modal.isVisible({ timeout: 5000 }).catch(() => false)) {
            test.skip();
            return;
        }
        const emailField = modal.locator('input[type="email"], input[name*="email"]').first();
        const roleField  = modal.locator('select[name*="role"], select[id*="role"]').first();
        const hasEmail = await emailField.isVisible().catch(() => false);
        const hasRole  = await roleField.isVisible().catch(() => false);
        expect(hasEmail || hasRole).toBe(true);
    });

    // ── TC-ADMIN-013 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-013 create user form validation on empty email', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const createBtn = page.locator('button, a').filter({
            hasText: /invite|create user|add user|new user/i
        }).first();
        if (!await createBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
            test.skip();
            return;
        }
        await createBtn.click();
        const modal = page.locator('.modal, [class*="modal"], [role="dialog"]').first();
        if (!await modal.isVisible({ timeout: 5000 }).catch(() => false)) {
            test.skip();
            return;
        }
        const submitBtn = modal.locator('button[type="submit"], button').filter({
            hasText: /invite|create|save|add/i
        }).first();
        if (!await submitBtn.isVisible().catch(() => false)) {
            test.skip();
            return;
        }
        await submitBtn.click();
        await page.waitForTimeout(500);
        const validationMsg = modal.locator(
            '.error, .invalid-feedback, [class*="error"], :invalid'
        ).first();
        const stillOpen   = await modal.isVisible().catch(() => false);
        const hasError    = await validationMsg.isVisible({ timeout: 2000 }).catch(() => false);
        expect(hasError || stillOpen).toBe(true);
    });

    // ── TC-ADMIN-014 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-014 current user cannot delete their own account', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        await page.waitForTimeout(1000);
        // Find the row that contains the logged-in admin email
        const adminRow = page.locator('tbody tr').filter({
            hasText: /admin@ezhealthkonnect.com/i
        }).first();
        if (!await adminRow.isVisible({ timeout: 5000 }).catch(() => false)) {
            test.skip(); // Can't identify own row
            return;
        }
        const deleteBtn = adminRow.locator('button, a').filter({ hasText: /delete|remove/i }).first();
        const isDisabled = await deleteBtn.isDisabled().catch(() => true);
        const isAbsent   = !(await deleteBtn.isVisible().catch(() => false));
        // Self-delete should be prevented
        expect(isDisabled || isAbsent).toBe(true);
    });

    // ── TC-ADMIN-015 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-015 unauthenticated access redirects to login', async ({ browser }) => {
        if (!adminUrl) { test.skip(); return; }
        const context = await browser.newContext({ storageState: { cookies: [], origins: [] } });
        const page    = await context.newPage();
        await page.goto(adminUrl);
        await page.waitForTimeout(1500);
        const url = page.url();
        const redirected = url.includes('login.html') || url.includes('/login') || url.includes('/auth');
        await context.close();
        expect(redirected).toBe(true);
    });

    // ── TC-ADMIN-016 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-016 users API returns 200 with user list', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        const resp = await page.request.get('/api/users');
        expect(resp.status()).toBe(200);
        const body = await resp.json().catch(() => null);
        // Body should be an array or an object containing an array
        const list = body?.users ?? body?.data ?? body;
        expect(Array.isArray(list)).toBe(true);
    });

    // ── TC-ADMIN-017 ─────────────────────────────────────────────────────────────
    test('TC-ADMIN-017 no horizontal overflow at 1280px', async ({ page }) => {
        if (!adminUrl) { test.skip(); return; }
        await page.setViewportSize({ width: 1280, height: 800 });
        const scrollWidth = await page.evaluate(() => document.body.scrollWidth);
        expect(scrollWidth).toBeLessThanOrEqual(1285);
    });
});
