'use strict';
/**
 * global-setup.spec.js
 *
 * Runs once before all other projects (via playwright.config.js `setup` project).
 * Logs in as admin and saves the browser storage state to .auth/user.json so every
 * other test starts pre-authenticated without repeating the login flow.
 */

const { test: setup } = require('@playwright/test');
const { login }       = require('./helpers/auth');
const path            = require('path');
const fs              = require('fs');

const AUTH_FILE = path.resolve(__dirname, '../../.auth/user.json');

setup('authenticate admin user', async ({ page }) => {
    await login(page);

    // Persist cookies + localStorage so dependent projects start authenticated
    const dir = path.dirname(AUTH_FILE);
    if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
    await page.context().storageState({ path: AUTH_FILE });
});
