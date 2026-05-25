'use strict';
/**
 * Shared authentication helpers for Playwright E2E tests.
 *
 * Usage:
 *   const { login, ADMIN_EMAIL, ADMIN_PASSWORD } = require('./helpers/auth');
 *   await login(page);           // navigates to /login.html and signs in
 *   await login(page, email, pw) // explicit credentials
 */

const ADMIN_EMAIL    = process.env.ADMIN_EMAIL    || 'admin@ezhealthkonnect.com';
const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD || 'admin123';

/**
 * Log in via the login form and wait for the dashboard redirect.
 * Idempotent: if the page is already on the dashboard the function returns immediately.
 */
async function login(page, email = ADMIN_EMAIL, password = ADMIN_PASSWORD) {
    // Already authenticated — nothing to do
    if (page.url().includes('dashboard.html')) return;

    await page.goto('/login.html');
    await page.locator('#email').fill(email);
    await page.locator('#password').fill(password);
    await page.locator('#loginButton').click();

    // Wait for redirect to dashboard (status message appears briefly then redirects)
    await page.waitForURL('**/dashboard.html', { timeout: 20_000 }).catch(async () => {
        // If URL didn't change, check for an error message on the login page
        const msg = await page.locator('#statusMessage').textContent().catch(() => '');
        if (!page.url().includes('dashboard.html')) {
            throw new Error(`Login failed: ${msg || 'no redirect to dashboard'}`);
        }
    });

    if (!page.url().includes('dashboard.html')) {
        const msg = await page.locator('#statusMessage').textContent().catch(() => 'unknown error');
        throw new Error(`Login failed: ${msg}`);
    }
}

/**
 * Log out by navigating to the logout endpoint or clearing session.
 * Returns to the login page.
 */
async function logout(page) {
    await page.goto('/api/auth/logout');
    await page.waitForURL('**/login.html', { timeout: 10_000 });
}

module.exports = { login, logout, ADMIN_EMAIL, ADMIN_PASSWORD };
