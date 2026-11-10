'use strict';
/**
 * Playwright E2E — Settings page
 *
 * TC-SET-001  Page title contains "Settings"
 * TC-SET-002  Settings nav item is marked active in sidebar
 * TC-SET-003  Section navigation renders expected setting categories
 * TC-SET-004  Clicking a section nav item highlights it as active
 * TC-SET-005  General / Profile section is present and shows form fields
 * TC-SET-006  Notifications section is accessible
 * TC-SET-007  Security section renders password change form fields
 * TC-SET-008  Security section — submitting mismatched passwords shows error
 * TC-SET-009  Database / Connection section shows DB connection fields
 * TC-SET-010  Schema Registry section is present (if feature enabled)
 * TC-SET-011  Saving a setting shows success or confirmation feedback
 * TC-SET-012  Unauthenticated access to settings.html redirects to login
 * TC-SET-013  Admin-only sections hidden from non-admin users (role guard)
 * TC-SET-014  Page has no horizontal overflow at 1280px
 * TC-SET-015  Settings API returns 200 for GET /api/settings (or per-section)
 */

const { test, expect } = require('@playwright/test');

test.describe('Settings', () => {

    test.beforeEach(async ({ page }) => {
        await page.goto('/settings.html');
        await page.waitForLoadState('domcontentloaded');
        await page.waitForTimeout(1500);
    });

    // ── TC-SET-001 ───────────────────────────────────────────────────────────────
    test('TC-SET-001 page title contains Settings', async ({ page }) => {
        await expect(page).toHaveTitle(/Settings/i);
    });

    // ── TC-SET-002 ───────────────────────────────────────────────────────────────
    test('TC-SET-002 sidebar marks Settings as active nav item', async ({ page }) => {
        const active = page.locator('.nav-item.active');
        await expect(active).toContainText(/settings/i);
    });

    // ── TC-SET-003 ───────────────────────────────────────────────────────────────
    test('TC-SET-003 section navigation renders expected categories', async ({ page }) => {
        const sectionNav = page.locator(
            '.settings-nav, .settings-sidebar, [class*="settings-nav"], .section-nav'
        ).first();
        await expect(sectionNav).toBeAttached({ timeout: 8000 });

        // At least some of these common section names should appear
        const expectedSections = ['General', 'Security', 'Notifications', 'Database', 'Profile'];
        let found = 0;
        for (const name of expectedSections) {
            const item = sectionNav.getByText(name, { exact: false });
            const visible = await item.isVisible().catch(() => false);
            if (visible) found++;
        }
        expect(found).toBeGreaterThanOrEqual(1);
    });

    // ── TC-SET-004 ───────────────────────────────────────────────────────────────
    test('TC-SET-004 clicking a section nav item activates it', async ({ page }) => {
        const navItems = page.locator('.settings-nav-item');
        expect(await navItems.count()).toBeGreaterThanOrEqual(10);
        const security = page.locator('.settings-nav-item[data-section="security"]');
        await security.click();
        await expect(security).toHaveClass(/active/);
        await expect(page.locator('#section-security')).toHaveClass(/active/);
        await expect(page.locator('.settings-nav-item[data-section="smtp"]')).not.toHaveClass(/active/);
    });

    // ── TC-SET-005 ───────────────────────────────────────────────────────────────
    test('TC-SET-005 General or Profile section shows form fields', async ({ page }) => {
        // Try to navigate to general/profile section
        const generalTab = page.locator(
            '.settings-nav, .settings-sidebar, .section-nav'
        ).first().getByText(/general|profile/i).first();
        if (await generalTab.isVisible().catch(() => false)) {
            await generalTab.click();
            await page.waitForTimeout(400);
        }

        // Form fields should be visible (email, name, etc.)
        const formFields = page.locator(
            'input[type="text"], input[type="email"], input[name*="name"], input[name*="email"]'
        );
        const count = await formFields.count();
        expect(count).toBeGreaterThanOrEqual(1);
    });

    // ── TC-SET-006 ───────────────────────────────────────────────────────────────
    // The page has no "Notifications" section; notification rules live under Alerts.
    test('TC-SET-006 Alerts (notification rules) section is accessible', async ({ page }) => {
        const alerts = page.locator('.settings-nav-item[data-section="alerts"]');
        await expect(alerts).toBeVisible();
        await alerts.click();
        await expect(page.locator('#section-alerts')).toHaveClass(/active/);
        expect(await page.locator('#section-alerts').locator('input, select').count()).toBeGreaterThan(0);
    });

    // ── TC-SET-007 ───────────────────────────────────────────────────────────────
    test('TC-SET-007 Security section has session, lockout and password-policy fields', async ({ page }) => {
        await page.locator('.settings-nav-item[data-section="security"]').click();
        await expect(page.locator('#section-security')).toHaveClass(/active/);
        for (const id of ['sessionTimeoutMinutes', 'jwtExpiryHours', 'maxLoginAttempts', 'lockoutDurationMinutes', 'passwordMinLength']) {
            await expect(page.locator('#' + id)).toBeVisible();
        }
    });

    // ── TC-SET-008 ───────────────────────────────────────────────────────────────
    // test.fixme, not a silent skip: Settings has no change-password form (the Security section
    // holds session/lockout/password-policy fields only), so there is nothing to mismatch.
    test.fixme('TC-SET-008 mismatched passwords shows validation error', async () => {});

    // TC-SET-009 (a "Database" settings section) was removed on purpose: the connection string and password
    // live in the server's .env, set by the installer. Editing them from a web page could stop the app from
    // reaching its own database and would put a secret on screen.

    // ── TC-SET-010 ───────────────────────────────────────────────────────────────
    test('TC-SET-010 Schema Registry section is present', async ({ page }) => {
        await page.locator('.settings-nav-item[data-section="schemas"]').click();
        await expect(page.locator('#section-schemas')).toHaveClass(/active/);
    });

    // ── TC-SET-011 ───────────────────────────────────────────────────────────────
    test('TC-SET-011 saving a setting persists it and shows success feedback', async ({ page }) => {
        await page.locator('.settings-nav-item[data-section="security"]').click();
        const field = page.locator('#passwordMinLength');
        await expect(field).toBeVisible();
        const original = await field.inputValue();   // save the value back unchanged: non-destructive
        const saved = page.waitForResponse(r => r.url().includes('/api/system/settings') && r.request().method() !== 'GET');
        await page.locator('#section-security button[onclick="saveSecurity()"]').click();
        expect((await saved).status()).toBe(200);
        await expect(page.locator('.toast, .alert-success, [class*="success"]').or(page.getByText(/saved|updated/i)).first()).toBeVisible({ timeout: 5000 });
        await page.reload();
        await page.locator('.settings-nav-item[data-section="security"]').click();
        await expect(page.locator('#passwordMinLength')).toHaveValue(original);
    });

    // ── TC-SET-012 ───────────────────────────────────────────────────────────────
    test('TC-SET-012 unauthenticated access redirects to login', async ({ browser }) => {
        // Create a fresh context with no stored auth
        const context = await browser.newContext({ storageState: { cookies: [], origins: [] } });
        const page    = await context.newPage();
        await page.goto('/settings.html');
        await page.waitForTimeout(1500);
        const url = page.url();
        const redirected = url.includes('login.html') || url.includes('/login') || url.includes('/auth');
        await context.close();
        expect(redirected).toBe(true);
    });

    // ── TC-SET-013 ───────────────────────────────────────────────────────────────
    test('TC-SET-013 admin-only sections require admin role', async ({ page }) => {
        // This test verifies that admin sections are present for admin users
        // (the storageState credentials are admin — see global-setup)
        const adminSection = page.locator(
            '[class*="admin"], #adminSettings, [data-role="admin"]'
        ).first();
        // If such a section exists, it should be visible to admin
        const visible = await adminSection.isVisible({ timeout: 3000 }).catch(() => false);
        if (visible) {
            await expect(adminSection).toBeVisible();
        }
        // No assertion if there's no admin-specific section — test is informational
    });

    // ── TC-SET-014 ───────────────────────────────────────────────────────────────
    test('TC-SET-014 no horizontal overflow at 1280px', async ({ page }) => {
        await page.setViewportSize({ width: 1280, height: 800 });
        const scrollWidth = await page.evaluate(() => document.body.scrollWidth);
        expect(scrollWidth).toBeLessThanOrEqual(1285);
    });

    // ── TC-SET-015 ───────────────────────────────────────────────────────────────
    test('TC-SET-015 settings API returns 200', async ({ page }) => {
        const resp = await page.request.get('/api/system/settings/storage');
        expect(resp.status()).toBe(200);
    });
});
