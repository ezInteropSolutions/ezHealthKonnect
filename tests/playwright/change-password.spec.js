// A signed-in user changes their own password from the sidebar ("Change password").
// Real user, real browser, real login afterwards — no mocks.
const { test, expect } = require('./helpers/seeded');
const { createUserWithRole, deleteUser, loginWithRetry } = require('./helpers/users');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const NEW_PASSWORD = 'Changed-Pw#2026-x';

test.describe('Change my password', () => {
    // Logins wait out the server's rate limit (10/min/IP) when a full run exceeds it, which can take a minute.
    test.describe.configure({ timeout: 120_000 });

    let user;

    test.beforeAll(async ({ seed }) => {
        user = await createUserWithRole(seed.api.request, BASE_URL, 'operator', '-pwchange');
    });

    test.afterAll(async ({ seed }) => {
        if (user) await deleteUser(seed.api.request, BASE_URL, user.id);
    });

    test('validation messages, then a successful change, then the old password stops working', async ({ browser, playwright }) => {
        // One API login for the session the browser starts from (login is limited to 10/min/IP).
        const api = await playwright.request.newContext({ baseURL: BASE_URL, storageState: { cookies: [], origins: [] } });
        const login = await loginWithRetry(api, { data: { email: user.email, password: user.password } });
        expect(login.ok()).toBeTruthy();
        const state = await api.storageState();
        await api.dispose();

        const context = await browser.newContext({ storageState: state });
        const page = await context.newPage();
        try {
            await page.goto(`${BASE_URL}/dashboard.html`);
            await page.locator('#sidebarChangePwdBtn').click();
            const dialog = page.locator('.app-dialog-box');
            await expect(dialog).toBeVisible();
            const error = dialog.locator('[role="alert"]');
            const fill = async (cur, next, confirm) => {
                await page.locator('#_appDlgField_currentPassword').fill(cur);
                await page.locator('#_appDlgField_newPassword').fill(next);
                await page.locator('#_appDlgField_confirmPassword').fill(confirm);
                await dialog.getByRole('button', { name: 'Change password' }).click();
            };

            // Typed passwords that do not match are caught before anything is sent.
            await fill(user.password, NEW_PASSWORD, NEW_PASSWORD + 'x');
            await expect(error).toHaveText('The new passwords do not match');

            // Wrong current password.
            await fill('not-my-password', NEW_PASSWORD, NEW_PASSWORD);
            await expect(error).toHaveText('Current password is incorrect');

            // Too short for the security policy.
            await fill(user.password, 'short', 'short');
            await expect(error).toContainText('at least');

            // Same as the current one.
            await fill(user.password, user.password, user.password);
            await expect(error).toHaveText('New password must be different from the current password');

            // Success: dialog closes, toast confirms.
            await fill(user.password, NEW_PASSWORD, NEW_PASSWORD);
            await expect(dialog).toBeHidden();
            await expect(page.locator('.app-toast-success')).toHaveText('Password changed');
        } finally {
            await context.close();
        }

        // The real proof: the old password no longer logs in, the new one does.
        const check = await playwright.request.newContext({ baseURL: BASE_URL, storageState: { cookies: [], origins: [] } });
        try {
            expect((await loginWithRetry(check, { data: { email: user.email, password: user.password } })).status()).toBe(401);
            expect((await loginWithRetry(check, { data: { email: user.email, password: NEW_PASSWORD } })).ok()).toBeTruthy();
        } finally {
            await check.dispose();
        }
    });

    // The real page flow: sign in -> redirected to the reset page -> the token is not left in the address bar,
    // and opening the page without one (a bare userId link) is sent back to sign-in.
    test('the forced-reset page works from the sign-in redirect and refuses a link without a token', async ({ seed, browser }) => {
        const email = `pw-forced-ui-${Date.now()}@example.org`;
        const oldPw = 'PwTest-Forced#2026a';
        const newPw = 'PwTest-Forced#2026c';
        const created = await seed.api.request.post(`${BASE_URL}/api/users`, { data: { email, password: oldPw, name: 'PW Forced UI', role: 'user' } });
        expect(created.ok()).toBeTruthy();
        const id = (await created.json()).user?.id;
        expect((await seed.api.request.post(`${BASE_URL}/api/users/${id}/force-reset`)).ok()).toBeTruthy();
        const ctx = await browser.newContext({ storageState: { cookies: [], origins: [] } });
        try {
            const page = await ctx.newPage();
            await page.goto('/force-reset.html?userId=' + id);   // no token
            await expect(page).not.toHaveURL(/force-reset/, { timeout: 10_000 });

            await page.goto('/login.html');
            await page.locator('#email').fill(email);
            await page.locator('#password').fill(oldPw);
            await page.locator('#loginButton').click();
            await page.waitForURL(/force-reset\.html/, { timeout: 30_000 });
            expect(page.url()).not.toContain('token=');          // dropped from the address bar once read
            await page.locator('#password').fill(newPw);
            await page.locator('#confirmPassword').fill(newPw);
            await page.locator('#submit-btn').click();
            await expect(page.locator('#success-view')).toBeVisible({ timeout: 15_000 });
        } finally {
            await ctx.close();
            await deleteUser(seed.api.request, BASE_URL, id);
        }
    });

    // The first-login / admin-forced reset must really set the password the user typed, not just clear the flag.
    test('a forced password reset sets the new password', async ({ seed, playwright }) => {
        const email = `pw-forced-${Date.now()}@example.org`;
        const oldPw = 'PwTest-Forced#2026a';
        const newPw = 'PwTest-Forced#2026b';
        const created = await seed.api.request.post(`${BASE_URL}/api/users`, { data: { email, password: oldPw, name: 'PW Forced', role: 'user' } });
        expect(created.ok()).toBeTruthy();
        const id = (await created.json()).user?.id;
        // An administrator forces a reset (the same call the Users page makes).
        expect((await seed.api.request.post(`${BASE_URL}/api/users/${id}/force-reset`)).ok()).toBeTruthy();

        const api = await playwright.request.newContext({ baseURL: BASE_URL, storageState: { cookies: [], origins: [] } });
        try {
            const first = await loginWithRetry(api, { data: { email, password: oldPw } });
            expect(first.status()).toBe(403);
            const { requiresPasswordReset, userId, resetToken } = await first.json();
            expect(requiresPasswordReset).toBe(true);

            // A bare userId, or the wrong token, is not enough to set the password.
            expect((await api.post('/api/auth/reset-password', { data: { userId, password: newPw } })).status()).toBe(400);
            expect((await api.post('/api/auth/reset-password', { data: { userId, resetToken: 'f'.repeat(64), password: newPw } })).status()).toBe(400);
            expect((await loginWithRetry(api, { data: { email, password: newPw } })).status()).toBe(401);   // still the old password

            expect((await api.post('/api/auth/reset-password', { data: { userId, resetToken, password: newPw } })).ok()).toBeTruthy();
            // Single use: the same token cannot reset it again.
            expect((await api.post('/api/auth/reset-password', { data: { userId, resetToken, password: 'Another#Pass9' } })).status()).toBe(400);

            expect((await loginWithRetry(api, { data: { email, password: oldPw } })).status()).toBe(401);
            expect((await loginWithRetry(api, { data: { email, password: newPw } })).ok()).toBeTruthy();
        } finally {
            await api.dispose();
            await deleteUser(seed.api.request, BASE_URL, id);
        }
    });

    test('anonymous callers cannot use the endpoint', async ({ playwright }) => {
        const anon = await playwright.request.newContext({ baseURL: BASE_URL, storageState: { cookies: [], origins: [] } });
        try {
            const res = await anon.post('/api/auth/change-password', { data: { currentPassword: 'a', newPassword: 'bbbbbbbbb' } });
            expect(res.status()).toBe(401);
        } finally {
            await anon.dispose();
        }
    });
});
