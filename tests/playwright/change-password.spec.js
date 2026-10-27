// A signed-in user changes their own password from the sidebar ("Change password").
// Real user, real browser, real login afterwards — no mocks.
const { test, expect } = require('./helpers/seeded');
const { createUserWithRole, deleteUser, loginWithRetry } = require('./helpers/users');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const NEW_PASSWORD = 'Changed-Pw#2026-x';

test.describe('Change my password', () => {
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
            const { requiresPasswordReset, userId } = await first.json();
            expect(requiresPasswordReset).toBe(true);

            expect((await api.post('/api/auth/reset-password', { data: { userId, password: newPw } })).ok()).toBeTruthy();

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
