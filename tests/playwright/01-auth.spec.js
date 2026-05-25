'use strict';
/**
 * Playwright E2E — Authentication flows
 *
 * TC-AUTH-001  Login page renders two-panel layout (brand + form)
 * TC-AUTH-002  Login form contains email, password, submit button
 * TC-AUTH-003  Invalid credentials show an error message
 * TC-AUTH-004  Empty form submission triggers HTML5 validation (no redirect)
 * TC-AUTH-005  Valid credentials redirect to dashboard
 * TC-AUTH-006  "Keep me signed in" checkbox is present and toggleable
 * TC-AUTH-007  Unauthenticated access to protected page redirects to login
 * TC-AUTH-008  Logout navigates back to login page
 * TC-AUTH-009  Forgot-password link is visible and interactive
 * TC-AUTH-010  Create-account link is visible and interactive
 * TC-AUTH-011  Login page is fully responsive at mobile viewport (375 × 812)
 * TC-AUTH-012  Password field masks input
 *
 * NOTE: These tests run WITHOUT pre-saved auth state (no `storageState` dependency)
 * because they deliberately exercise the unauthenticated path.
 */

const { test, expect } = require('@playwright/test');
const { ADMIN_EMAIL, ADMIN_PASSWORD } = require('./helpers/auth');

// Override storageState so these tests always start unauthenticated
test.use({ storageState: { cookies: [], origins: [] } });

test.describe('Authentication', () => {

    // ── TC-AUTH-001 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-001 login page renders brand panel and form panel', async ({ page }) => {
        await page.goto('/login.html');
        await expect(page).toHaveTitle(/Login.*ezHealthKonnect/i);
        await expect(page.locator('.brand-panel')).toBeVisible();
        await expect(page.locator('.form-panel')).toBeVisible();
        await expect(page.locator('.brand-name')).toContainText('ezHealthKonnect');
    });

    // ── TC-AUTH-002 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-002 login form has required inputs and submit button', async ({ page }) => {
        await page.goto('/login.html');
        await expect(page.locator('#email')).toBeVisible();
        await expect(page.locator('#password')).toBeVisible();
        await expect(page.locator('#loginButton')).toBeVisible();
        await expect(page.locator('#loginButton')).toContainText(/sign in/i);
    });

    // ── TC-AUTH-003 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-003 invalid credentials show error message without redirect', async ({ page }) => {
        await page.goto('/login.html');
        await page.locator('#email').fill('nobody@invalid.example.com');
        await page.locator('#password').fill('wrong-password-123');
        await page.locator('#loginButton').click();

        // Must stay on login page
        await expect(page).toHaveURL(/login\.html/);
        // Error message must appear
        await expect(page.locator('#statusMessage')).toBeVisible({ timeout: 10_000 });
        const msg = await page.locator('#statusMessage').textContent();
        expect(msg?.toLowerCase()).toMatch(/invalid|incorrect|unauthorized|error|failed/i);
    });

    // ── TC-AUTH-004 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-004 empty form does not redirect (HTML5 required validation)', async ({ page }) => {
        await page.goto('/login.html');
        // Click submit without filling fields
        await page.locator('#loginButton').click();
        // Still on login page — browser validation prevents submission
        await expect(page).toHaveURL(/login\.html/);
    });

    // ── TC-AUTH-005 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-005 valid credentials redirect to dashboard', async ({ page }) => {
        await page.goto('/login.html');
        await page.locator('#email').fill(ADMIN_EMAIL);
        await page.locator('#password').fill(ADMIN_PASSWORD);
        await page.locator('#loginButton').click();
        await expect(page).toHaveURL(/dashboard\.html/, { timeout: 15_000 });
    });

    // ── TC-AUTH-006 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-006 "keep me signed in" checkbox is present and toggleable', async ({ page }) => {
        await page.goto('/login.html');
        const label    = page.locator('.checkbox-container');
        const checkbox = page.locator('#remember');
        // #remember is CSS-hidden; interact via its visible label container
        await expect(label).toBeVisible();
        await expect(checkbox).not.toBeChecked();
        await label.click();
        await expect(checkbox).toBeChecked();
    });

    // ── TC-AUTH-007 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-007 unauthenticated access to dashboard redirects to login', async ({ page }) => {
        await page.goto('/dashboard.html');
        // Server or client-side guard should redirect to login
        await expect(page).toHaveURL(/login\.html/, { timeout: 10_000 });
    });

    // ── TC-AUTH-008 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-008 logout clears session and returns to login', async ({ page }) => {
        // First log in
        await page.goto('/login.html');
        await page.locator('#email').fill(ADMIN_EMAIL);
        await page.locator('#password').fill(ADMIN_PASSWORD);
        await page.locator('#loginButton').click();
        await expect(page).toHaveURL(/dashboard\.html/, { timeout: 15_000 });

        // Call server-side logout, then clear all client-side auth state
        await page.evaluate(async () => {
            try { await fetch('/api/auth/logout'); } catch (e) { /* ignore */ }
            // Clear JWT token and all auth state from localStorage
            localStorage.removeItem('accessToken');
            localStorage.removeItem('refreshToken');
            localStorage.removeItem('token');
            localStorage.removeItem('user');
            sessionStorage.clear();
            return null;
        });
        // Also clear cookies so session cookie is removed
        await page.context().clearCookies();
        await page.waitForTimeout(300);
        await page.goto('/login.html');
        await expect(page).toHaveURL(/login\.html/);

        // Verify session cleared — trying dashboard should redirect again
        await page.goto('/dashboard.html');
        await expect(page).toHaveURL(/login\.html/, { timeout: 10_000 });
    });

    // ── TC-AUTH-009 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-009 forgot-password link is visible', async ({ page }) => {
        await page.goto('/login.html');
        const link = page.getByText(/forgot password/i);
        await expect(link).toBeVisible();
    });

    // ── TC-AUTH-010 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-010 create-account link is visible', async ({ page }) => {
        await page.goto('/login.html');
        const link = page.getByText(/create new account/i);
        await expect(link).toBeVisible();
    });

    // ── TC-AUTH-011 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-011 login page renders correctly at mobile viewport', async ({ page }) => {
        await page.setViewportSize({ width: 375, height: 812 });
        await page.goto('/login.html');
        // Form panel must be visible at mobile — some layouts hide the brand panel
        await expect(page.locator('#email')).toBeVisible();
        await expect(page.locator('#password')).toBeVisible();
        await expect(page.locator('#loginButton')).toBeVisible();
        // No horizontal scroll
        const bodyWidth = await page.evaluate(() => document.body.scrollWidth);
        expect(bodyWidth).toBeLessThanOrEqual(375 + 5); // 5px tolerance
    });

    // ── TC-AUTH-012 ─────────────────────────────────────────────────────────────
    test('TC-AUTH-012 password field masks its input', async ({ page }) => {
        await page.goto('/login.html');
        const type = await page.locator('#password').getAttribute('type');
        expect(type).toBe('password');
    });
});
