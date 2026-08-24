'use strict';
/**
 * global-setup.spec.js
 *
 * Runs once before all other projects (via playwright.config.js `setup` project).
 * Logs in as admin and saves the browser storage state to .auth/user.json so every
 * other test starts pre-authenticated without repeating the login flow.
 */

const { test: setup } = require('@playwright/test');
const { login, ADMIN_EMAIL, ADMIN_PASSWORD } = require('./helpers/auth');
const path            = require('path');
const fs              = require('fs');

const AUTH_FILE = path.resolve(__dirname, '../../.auth/user.json');

// A fresh database (e.g. a new CI Postgres volume) seeds admin@ezhealthkonnect.com
// but immediately locks it (V146__Lock_Placeholder_Admin.sql) with an unusable
// password hash until the first-run setup wizard runs — see setupController.js /
// middleware/setupCheck.js. Complete it here, once, before any login is attempted,
// using the same admin credentials the rest of the suite expects.
async function completeFirstRunSetupIfNeeded(page) {
    const status = await page.request.get('/api/setup/status');
    const { setupRequired } = await status.json();
    if (!setupRequired) return;

    const res = await page.request.post('/api/setup/complete', {
        data: {
            orgName:   'ezHealthKonnect E2E',
            firstName: 'System',
            lastName:  'Administrator',
            email:     ADMIN_EMAIL,
            password:  ADMIN_PASSWORD,
        },
    });
    if (!res.ok()) {
        const body = await res.text();
        throw new Error(`First-run setup failed (${res.status()}): ${body.slice(0, 300)}`);
    }
}

setup('authenticate admin user', async ({ page }) => {
    await completeFirstRunSetupIfNeeded(page);
    await login(page);

    // Persist cookies + localStorage so dependent projects start authenticated
    const dir = path.dirname(AUTH_FILE);
    if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
    await page.context().storageState({ path: AUTH_FILE });
});
