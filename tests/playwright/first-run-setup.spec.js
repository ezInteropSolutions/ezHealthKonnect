'use strict';
/**
 * first-run-setup.spec.js — first-run setup with the installer's one-time token (G4).
 *
 * Runs as the `first-run` project, BEFORE global-setup.spec.js logs in, because
 * it can only run on a server where setup has never been completed (a fresh
 * install). It drives the real setup page and creates the administrator the
 * rest of the suite logs in as (ADMIN_EMAIL / ADMIN_PASSWORD).
 *
 * Environment:
 *   SETUP_TOKEN  the token the installer wrote to .env (required when the server
 *                requires one). The installed-app CI job reads it from .env.
 *
 * Skips, with the reason stated, when setup is already complete (every dev
 * stack and every run after the first) or when the server requires a token
 * but SETUP_TOKEN was not provided to the test.
 */
const { test, expect } = require('@playwright/test');
const { ADMIN_EMAIL, ADMIN_PASSWORD } = require('./helpers/auth');

const SETUP_TOKEN = (process.env.SETUP_TOKEN || '').trim();
const PLACEHOLDER_ADMIN = 'admin@ezhealthkonnect.com';

test.describe.configure({ mode: 'serial' });

test.describe('First-run setup with the one-time token', () => {
    let tokenRequired = false;

    test.beforeAll(async ({ request }) => {
        const status = await (await request.get('/api/setup/status')).json();
        test.skip(!status.setupRequired, 'Setup is already complete on this server; this spec runs on a fresh install (installer CI).');
        tokenRequired = !!status.tokenRequired;
        test.skip(tokenRequired && !SETUP_TOKEN, 'The server requires a setup token but SETUP_TOKEN was not provided to the test.');
    });

    test('before setup: pages redirect to setup and the placeholder admin cannot log in', async ({ page, request }) => {
        await page.goto('/');
        await expect(page).toHaveURL(/\/setup\.html/);

        // Refused before setup (the API answers 503 until setup is done); after
        // setup the placeholder's password hash is unusable (V146).
        const res = await request.post('/api/auth/login', { data: { email: PLACEHOLDER_ADMIN, password: 'admin123' } });
        expect(res.ok(), 'the well-known placeholder password must not log in').toBe(false);
    });

    async function fillAndSubmit(page) {
        await page.locator('#orgName').fill('ezHealthKonnect E2E');
        await page.locator('#btn-next-1').click();
        await expect(page.locator('#step-2')).toBeVisible();
        await page.locator('#firstName').fill('System');
        await page.locator('#lastName').fill('Administrator');
        await page.locator('#adminEmail').fill(ADMIN_EMAIL);
        await page.locator('#adminPassword').fill(ADMIN_PASSWORD);
        await page.locator('#adminPasswordConfirm').fill(ADMIN_PASSWORD);
        await page.locator('#btn-submit').click();
    }

    test('without a token, setup is refused and the page says how to get the link', async ({ page, request }) => {
        test.skip(!tokenRequired, 'This server does not require a setup token.');
        await page.goto('/setup.html');
        await expect(page.locator('#alert-1')).toContainText('setup link', { timeout: 10_000 });
        await fillAndSubmit(page);
        await expect(page.locator('#alert-2')).toContainText('one-time token', { timeout: 10_000 });
        expect((await (await request.get('/api/setup/status')).json()).setupRequired).toBe(true);
    });

    test('a wrong token is refused', async ({ page, request }) => {
        test.skip(!tokenRequired, 'This server does not require a setup token.');
        await page.goto('/setup.html?token=not-the-real-token');
        await fillAndSubmit(page);
        await expect(page.locator('#alert-2')).toContainText('one-time token', { timeout: 10_000 });
        expect((await (await request.get('/api/setup/status')).json()).setupRequired).toBe(true);
    });

    test('the setup link completes setup and the token leaves the address bar', async ({ page, request }) => {
        await page.goto(SETUP_TOKEN ? `/setup.html?token=${encodeURIComponent(SETUP_TOKEN)}` : '/setup.html');
        // The token is kept for this tab only and removed from the URL (history, screenshots).
        await expect(page).not.toHaveURL(/token=/);
        await fillAndSubmit(page);
        await expect(page.locator('#step-3')).toBeVisible({ timeout: 15_000 });
        await expect(page.locator('#success-msg')).toContainText(ADMIN_EMAIL.toLowerCase());
        expect((await (await request.get('/api/setup/status')).json()).setupRequired).toBe(false);
    });

    test('setup cannot be run a second time', async ({ page, request }) => {
        const res = await request.post('/api/setup/complete', {
            data: { orgName: 'Second', firstName: 'Second', lastName: 'Admin', email: 'second.admin@example.org', password: 'Another-pass-1', setupToken: SETUP_TOKEN },
        });
        expect(res.status()).toBe(409);
        await page.goto('/setup.html');
        await expect(page).not.toHaveURL(/\/setup\.html/, { timeout: 10_000 });
    });

    test('the new administrator can log in', async ({ page }) => {
        await page.goto('/login.html');
        await page.locator('#email').fill(ADMIN_EMAIL);
        await page.locator('#password').fill(ADMIN_PASSWORD);
        await page.locator('#loginButton').click();
        await page.waitForURL('**/dashboard.html', { timeout: 20_000 });
    });
});
