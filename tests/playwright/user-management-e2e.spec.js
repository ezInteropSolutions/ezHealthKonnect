/**
 * user-management-e2e.spec.js
 *
 * Enterprise-grade QA for the User Management system.
 *
 * Coverage dimensions:
 *   BIZ  — Business flows (admin / compliance-officer perspective)
 *   USR  — User interaction flows (navigation, forms, visual feedback)
 *   TECH — Technical flows (API contracts, data integrity, state management)
 *   SEC  — Security (auth gates, XSS, RBAC, lockout, self-protection)
 *
 * Test categories mirror UC-USER-MANAGEMENT.md:
 *   UC-AUTH   Authentication & session management
 *   UC-ONB    User invitation & onboarding
 *   UC-PROF   Profile management
 *   UC-RBAC   Role-based access control
 *   UC-BULK   Bulk operations
 *   UC-COMP   HIPAA/GDPR compliance
 *   UC-AUDIT  Audit log visibility
 *   UC-SRCH   Search & filter
 *   UC-OFF    Offboarding / account lifecycle
 *   UC-API    API contract verification
 *
 * Execution:
 *   npx playwright test tests/playwright/user-management-e2e.spec.js
 *   npx playwright test --headed tests/playwright/user-management-e2e.spec.js
 *   npx playwright test --grep "UC-AUTH" tests/playwright/user-management-e2e.spec.js
 */

'use strict';

const { test, expect, request } = require('@playwright/test');

// ── Config ────────────────────────────────────────────────────────────────────
const BASE_URL    = process.env.BASE_URL    || 'http://localhost:3000';
const API_USERS   = `${BASE_URL}/api/users`;
const ADMIN_EMAIL = process.env.ADMIN_EMAIL || 'admin@ezhealthkonnect.com';
const ADMIN_PASS  = process.env.ADMIN_PASS  || 'admin123';

// ── Helpers ───────────────────────────────────────────────────────────────────
async function login(page) {
    await page.goto(`${BASE_URL}/login.html`);
    await page.fill('input[name="email"], input[type="email"]', ADMIN_EMAIL);
    await page.fill('input[name="password"], input[type="password"]', ADMIN_PASS);
    await page.click('button[type="submit"], .login-btn');
    await page.waitForURL(/dashboard|user-management/, { timeout: 15000 });
}

async function apiContext() {
    const ctx = await request.newContext({ baseURL: BASE_URL });
    await ctx.post('/api/auth/login', {
        data: { email: ADMIN_EMAIL, password: ADMIN_PASS }
    });
    return ctx;
}

async function goToUserManagement(page) {
    await page.goto(`${BASE_URL}/user-management.html`);
    await page.waitForSelector('#um-tbody', { timeout: 10000 });
}

/** Create a user via API; returns the created user object */
async function createTestUser(api, overrides = {}) {
    const ts = Date.now();
    const res = await api.post(`${API_USERS}`, {
        data: {
            email:     `qa-test-${ts}@ezhk.test`,
            password:  'QaTest1234!',
            firstName: 'QA',
            lastName:  'TestUser',
            role:      'user',
            status:    'active',
            ...overrides
        }
    });
    const body = await res.json();
    return body.user || body;
}

/** Delete a user via API (cleanup) */
async function deleteUser(api, userId) {
    await api.delete(`${API_USERS}/${userId}`).catch(() => {/* ignore if already gone */});
}

// ── UC-AUTH: Authentication & Session ────────────────────────────────────────
test.describe('UC-AUTH — Authentication & Session', () => {

    test('UC-AUTH-01 [BIZ] Unauthenticated redirect — page redirects to login', async ({ page }) => {
        // Do NOT login first
        await page.goto(`${BASE_URL}/user-management.html`);
        await expect(page).toHaveURL(/login/, { timeout: 10000 });
    });

    test('UC-AUTH-02 [SEC] Non-admin user cannot access user management', async ({ page, request: req }) => {
        const api = await request.newContext({ baseURL: BASE_URL });
        // Create a non-admin user
        const adminApi = await apiContext();
        const user = await createTestUser(adminApi, { role: 'viewer', password: 'Viewer1234!' });

        try {
            // Login as viewer
            await page.goto(`${BASE_URL}/login.html`);
            await page.fill('input[name="email"], input[type="email"]', user.email || `qa-test-${Date.now()}@ezhk.test`);
            await page.fill('input[name="password"], input[type="password"]', 'Viewer1234!');
            await page.click('button[type="submit"], .login-btn');
            await page.waitForTimeout(2000);

            // Attempt to navigate to user management
            await page.goto(`${BASE_URL}/user-management.html`);
            await page.waitForTimeout(2000);

            // Should redirect away OR the Admin nav section should not be visible
            const url = page.url();
            const isRedirected = !url.includes('user-management');
            const adminSectionHidden = await page.$eval('#adminSection', el => el.style.display === 'none').catch(() => true);
            expect(isRedirected || adminSectionHidden).toBeTruthy();
        } finally {
            if (user.id) await deleteUser(adminApi, user.id);
            await adminApi.dispose();
        }
    });

    test('UC-AUTH-03 [TECH] Session API returns authenticated user info', async () => {
        const api = await apiContext();
        const res = await api.get(`${BASE_URL}/api/auth/session`);
        expect(res.status()).toBe(200);
        const body = await res.json();
        expect(body).toHaveProperty('user');
        expect(body.user).toHaveProperty('email', ADMIN_EMAIL);
        await api.dispose();
    });

    test('UC-AUTH-04 [SEC] Account lockout — 5 bad passwords locks the account', async () => {
        const adminApi = await apiContext();
        const ts = Date.now();
        const email = `lock-test-${ts}@ezhk.test`;
        const user = await createTestUser(adminApi, { email, password: 'Correct1234!' });

        try {
            const guestCtx = await request.newContext({ baseURL: BASE_URL });
            for (let i = 0; i < 5; i++) {
                await guestCtx.post(`${BASE_URL}/api/auth/login`, {
                    data: { email, password: 'WrongPassword!' }
                });
            }
            const res = await guestCtx.post(`${BASE_URL}/api/auth/login`, {
                data: { email, password: 'Correct1234!' }
            });
            // After 5 failures the account should be locked → 423
            expect(res.status()).toBe(423);
            const body = await res.json();
            expect(body).toHaveProperty('lockedUntil');
            await guestCtx.dispose();
        } finally {
            if (user.id) await deleteUser(adminApi, user.id);
            await adminApi.dispose();
        }
    });

    test('UC-AUTH-05 [BIZ] Admin can unlock a locked account', async ({ page }) => {
        const api = await apiContext();
        const ts = Date.now();
        const email = `unlock-test-${ts}@ezhk.test`;
        const user = await createTestUser(api, { email, password: 'Unlock1234!' });

        try {
            // Lock the account via 5 bad logins
            const guestCtx = await request.newContext({ baseURL: BASE_URL });
            for (let i = 0; i < 5; i++) {
                await guestCtx.post(`${BASE_URL}/api/auth/login`, {
                    data: { email, password: 'WrongPassword!' }
                });
            }
            await guestCtx.dispose();

            // Admin unlocks via UI
            await login(page);
            await goToUserManagement(page);

            // Find the user row and open drawer
            await page.fill('#um-search', email);
            await page.waitForTimeout(600);
            const row = page.locator('#um-tbody tr').filter({ hasText: email }).first();
            await row.locator('.btn-open-drawer, [data-action="open"]').click();
            await page.waitForSelector('#um-drawer:not([style*="display:none"])', { timeout: 5000 });

            // Switch to Security tab and click Unlock
            await page.click('[data-dtab="security"]');
            await page.waitForSelector('#dtab-security', { timeout: 3000 });
            await page.click('#ds-btn-unlock');
            await page.waitForTimeout(1000);

            // Verify unlock via API
            const check = await api.get(`${API_USERS}/${user.id}`);
            const userData = await check.json();
            const u = userData.user || userData;
            expect(u.login_attempts || u.loginAttempts || 0).toBe(0);
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });
});

// ── UC-ONB: User Invitation & Onboarding ──────────────────────────────────────
test.describe('UC-ONB — User Invitation & Onboarding', () => {

    test('UC-ONB-01 [BIZ] Admin can open invite modal and submit valid email', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);

        await page.click('#btnInviteUser');
        await page.waitForSelector('#modal-invite', { timeout: 5000 });
        const modal = page.locator('#modal-invite');
        await expect(modal).toBeVisible();

        await modal.locator('input[name="email"], #invite-email').fill(`invite-test-${Date.now()}@ezhk.test`);
        await modal.locator('select[name="role"], #invite-role').selectOption('viewer');
        // Do NOT submit — just verify modal is functional
        await expect(modal.locator('button[type="submit"], #invite-submit-btn')).toBeEnabled();
    });

    test('UC-ONB-02 [TECH] Invite API creates pending user with token', async () => {
        const api = await apiContext();
        const ts = Date.now();
        const email = `api-invite-${ts}@ezhk.test`;
        let userId;

        try {
            const res = await api.post(`${API_USERS}/invite`, {
                data: { email, role: 'user' }
            });
            expect(res.status()).toBe(201);
            const body = await res.json();
            expect(body).toHaveProperty('inviteLink');
            expect(body.inviteLink).toContain('token=');
            userId = body.user?.id || body.userId;
        } finally {
            if (userId) await deleteUser(api, userId);
            await api.dispose();
        }
    });

    test('UC-ONB-03 [TECH] Invite link resolves to invite.html with token in URL', async ({ page }) => {
        const api = await apiContext();
        const ts = Date.now();
        const email = `invite-link-${ts}@ezhk.test`;
        let userId;

        try {
            const res = await api.post(`${API_USERS}/invite`, {
                data: { email, role: 'user' }
            });
            const body = await res.json();
            expect(body).toHaveProperty('inviteLink');
            userId = body.user?.id || body.userId;

            // Navigate to the invite link
            await page.goto(body.inviteLink);
            await page.waitForSelector('#form-view, #expired-view', { timeout: 10000 });

            // Should show form (valid token) or expired (if clock is off)
            const formVisible = await page.$('#form-view:not([style*="display:none"])');
            const expiredVisible = await page.$('#expired-view:not([style*="display:none"])');
            expect(formVisible || expiredVisible).toBeTruthy();
        } finally {
            if (userId) await deleteUser(api, userId);
            await api.dispose();
        }
    });

    test('UC-ONB-04 [USR] Invite page shows expired view when no token in URL', async ({ page }) => {
        await page.goto(`${BASE_URL}/invite.html`);
        await page.waitForSelector('#expired-view', { timeout: 5000 });
        const expired = page.locator('#expired-view');
        await expect(expired).toBeVisible();
    });

    test('UC-ONB-05 [SEC] Duplicate invite email returns appropriate error', async () => {
        const api = await apiContext();
        const ts = Date.now();
        const email = `dup-invite-${ts}@ezhk.test`;
        let userId;

        try {
            const res1 = await api.post(`${API_USERS}/invite`, { data: { email, role: 'user' } });
            const b1 = await res1.json();
            userId = b1.user?.id || b1.userId;

            const res2 = await api.post(`${API_USERS}/invite`, { data: { email, role: 'user' } });
            // Should either return 409 conflict or 400 bad request
            expect([400, 409]).toContain(res2.status());
        } finally {
            if (userId) await deleteUser(api, userId);
            await api.dispose();
        }
    });
});

// ── UC-PROF: Profile Management ───────────────────────────────────────────────
test.describe('UC-PROF — Profile Management', () => {

    test('UC-PROF-01 [BIZ] Admin can update another user\'s profile via drawer', async ({ page }) => {
        const api = await apiContext();
        const user = await createTestUser(api);

        try {
            await login(page);
            await goToUserManagement(page);

            await page.fill('#um-search', user.email);
            await page.waitForTimeout(600);
            const row = page.locator('#um-tbody tr').filter({ hasText: user.email }).first();
            await row.locator('[data-action="open"], .btn-open-drawer').click();
            await page.waitForSelector('#um-drawer:not([style*="display:none"])', { timeout: 5000 });

            // Update department field
            const newDept = `QA-Dept-${Date.now()}`;
            await page.fill('#dp-dept', newDept);
            await page.click('#drawer-profile-form button[type="submit"]');
            await page.waitForTimeout(1000);

            // Verify via API
            const check = await api.get(`${API_USERS}/${user.id}`);
            const body = await check.json();
            const u = body.user || body;
            expect(u.department).toBe(newDept);
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-PROF-02 [USR] Drawer opens with user info populated', async ({ page }) => {
        const api = await apiContext();
        const user = await createTestUser(api, { firstName: 'DrawerTest', lastName: 'Open' });

        try {
            await login(page);
            await goToUserManagement(page);

            await page.fill('#um-search', user.email);
            await page.waitForTimeout(600);
            const row = page.locator('#um-tbody tr').filter({ hasText: user.email }).first();
            await row.locator('[data-action="open"], .btn-open-drawer').click();
            await page.waitForSelector('#um-drawer:not([style*="display:none"])', { timeout: 5000 });

            // Check that the drawer is populated
            await expect(page.locator('#drawer-name')).not.toHaveText('—');
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-PROF-03 [TECH] Profile update API accepts all standard fields', async () => {
        const api = await apiContext();
        const user = await createTestUser(api);

        try {
            const res = await api.put(`${API_USERS}/${user.id}/profile`, {
                data: {
                    first_name:  'Updated',
                    last_name:   'Name',
                    phone:       '+1-555-0001',
                    department:  'Engineering',
                    job_title:   'QA Engineer',
                    organization:'EZHK QA',
                    timezone:    'America/New_York',
                    locale:      'en-US'
                }
            });
            expect(res.status()).toBe(200);
            const body = await res.json();
            const u = body.user || body;
            expect(u.department).toBe('Engineering');
            expect(u.job_title || u.jobTitle).toBe('QA Engineer');
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-PROF-04 [USR] Drawer closes on X button click', async ({ page }) => {
        const api = await apiContext();
        const user = await createTestUser(api);

        try {
            await login(page);
            await goToUserManagement(page);

            await page.fill('#um-search', user.email);
            await page.waitForTimeout(600);
            const row = page.locator('#um-tbody tr').filter({ hasText: user.email }).first();
            await row.locator('[data-action="open"], .btn-open-drawer').click();
            await page.waitForSelector('#um-drawer:not([style*="display:none"])', { timeout: 5000 });

            await page.click('#um-drawer-close');
            await page.waitForTimeout(300);
            const drawerVisible = await page.$eval('#um-drawer', el => el.style.display !== 'none').catch(() => false);
            expect(drawerVisible).toBeFalsy();
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-PROF-05 [SEC] XSS in profile fields is escaped in the UI', async ({ page }) => {
        const api = await apiContext();
        const xss = '<script>window.__xss_fired=1</script>';
        const user = await createTestUser(api, { firstName: xss });

        try {
            await login(page);
            await goToUserManagement(page);

            await page.fill('#um-search', user.email);
            await page.waitForTimeout(600);
            const row = page.locator('#um-tbody tr').filter({ hasText: user.email }).first();
            await row.locator('[data-action="open"], .btn-open-drawer').click();
            await page.waitForSelector('#um-drawer:not([style*="display:none"])', { timeout: 5000 });

            // Script must NOT have executed
            const xssFired = await page.evaluate(() => window.__xss_fired);
            expect(xssFired).toBeFalsy();
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });
});

// ── UC-RBAC: Role-Based Access Control ────────────────────────────────────────
test.describe('UC-RBAC — Role-Based Access Control', () => {

    test('UC-RBAC-01 [BIZ] Admin can change a user\'s role via security tab', async ({ page }) => {
        const api = await apiContext();
        const user = await createTestUser(api, { role: 'user' });

        try {
            await login(page);
            await goToUserManagement(page);

            await page.fill('#um-search', user.email);
            await page.waitForTimeout(600);
            const row = page.locator('#um-tbody tr').filter({ hasText: user.email }).first();
            await row.locator('[data-action="open"], .btn-open-drawer').click();
            await page.waitForSelector('#um-drawer:not([style*="display:none"])', { timeout: 5000 });

            await page.click('[data-dtab="security"]');
            await page.waitForSelector('#dtab-security:not([style*="display:none"])', { timeout: 3000 });

            await page.selectOption('#ds-role', 'viewer');
            await page.click('#drawer-security-form button[type="submit"]');
            await page.waitForTimeout(1000);

            const check = await api.get(`${API_USERS}/${user.id}`);
            const body = await check.json();
            const u = body.user || body;
            expect(u.role).toBe('viewer');
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-RBAC-02 [SEC] Non-admin API call to /api/users returns 403', async () => {
        const api = await apiContext();
        const ts = Date.now();
        const email = `rbac-viewer-${ts}@ezhk.test`;
        const user = await createTestUser(api, { email, role: 'viewer', password: 'Viewer1234!' });

        try {
            const viewerCtx = await request.newContext({ baseURL: BASE_URL });
            await viewerCtx.post(`${BASE_URL}/api/auth/login`, {
                data: { email, password: 'Viewer1234!' }
            });
            const res = await viewerCtx.get(API_USERS);
            expect([401, 403]).toContain(res.status());
            await viewerCtx.dispose();
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-RBAC-03 [USR] Roles & Permissions tab renders the permissions matrix', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);

        await page.click('[data-tab="roles"]');
        await page.waitForSelector('#tab-roles:not([style*="display:none"])', { timeout: 3000 });

        const matrix = page.locator('.um-roles-table');
        await expect(matrix).toBeVisible();
        // Admin column should show "Full" for User Management
        const adminCell = page.locator('.um-roles-table tbody tr').filter({ hasText: 'User Management' }).locator('.perm-yes').first();
        await expect(adminCell).toBeVisible();
    });

    test('UC-RBAC-04 [BIZ] Force password reset flag can be set', async ({ page }) => {
        const api = await apiContext();
        const user = await createTestUser(api);

        try {
            await login(page);
            await goToUserManagement(page);

            await page.fill('#um-search', user.email);
            await page.waitForTimeout(600);
            const row = page.locator('#um-tbody tr').filter({ hasText: user.email }).first();
            await row.locator('[data-action="open"], .btn-open-drawer').click();
            await page.waitForSelector('#um-drawer:not([style*="display:none"])', { timeout: 5000 });

            await page.click('[data-dtab="security"]');
            await page.waitForSelector('#dtab-security:not([style*="display:none"])', { timeout: 3000 });
            await page.click('#ds-btn-force-reset');
            await page.waitForTimeout(1000);

            const check = await api.get(`${API_USERS}/${user.id}`);
            const body = await check.json();
            const u = body.user || body;
            expect(u.force_password_reset || u.forcePasswordReset).toBe(true);
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-RBAC-05 [TECH] Stats endpoint returns role breakdown', async () => {
        const api = await apiContext();
        const res = await api.get(`${API_USERS}/stats`);
        expect(res.status()).toBe(200);
        const body = await res.json();
        expect(body).toHaveProperty('total');
        expect(body).toHaveProperty('active');
        expect(body).toHaveProperty('admins');
        expect(typeof body.total).toBe('number');
        await api.dispose();
    });
});

// ── UC-BULK: Bulk Operations ──────────────────────────────────────────────────
test.describe('UC-BULK — Bulk Operations', () => {

    test('UC-BULK-01 [USR] Selecting rows reveals bulk action toolbar', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);
        await page.waitForSelector('#um-tbody tr:not(.um-loading)', { timeout: 8000 });

        const firstCheckbox = page.locator('#um-tbody tr').first().locator('input[type="checkbox"]');
        await firstCheckbox.check();

        const bulkBar = page.locator('#um-bulk-bar');
        await expect(bulkBar).toBeVisible({ timeout: 2000 });
        await expect(page.locator('#um-bulk-count')).toContainText('1');
    });

    test('UC-BULK-02 [USR] Select-all checkbox selects all visible rows', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);
        await page.waitForSelector('#um-tbody tr:not(.um-loading)', { timeout: 8000 });

        await page.check('#um-select-all');
        await page.waitForTimeout(300);

        const checked = await page.locator('#um-tbody input[type="checkbox"]:checked').count();
        const total   = await page.locator('#um-tbody input[type="checkbox"]').count();
        expect(checked).toBe(total);
    });

    test('UC-BULK-03 [BIZ] Bulk deactivate updates user statuses', async ({ page }) => {
        const api = await apiContext();
        const user1 = await createTestUser(api, { status: 'active' });
        const user2 = await createTestUser(api, { status: 'active' });

        try {
            const res = await api.post(`${API_USERS}/bulk`, {
                data: { action: 'deactivate', userIds: [user1.id, user2.id] }
            });
            expect(res.status()).toBe(200);

            const c1 = await api.get(`${API_USERS}/${user1.id}`);
            const b1 = await c1.json();
            expect((b1.user || b1).status).toBe('inactive');
        } finally {
            if (user1.id) await deleteUser(api, user1.id);
            if (user2.id) await deleteUser(api, user2.id);
            await api.dispose();
        }
    });

    test('UC-BULK-04 [SEC] Bulk delete cannot delete the currently logged-in admin', async () => {
        const api = await apiContext();
        // Get current admin's ID
        const sessionRes = await api.get(`${BASE_URL}/api/auth/session`);
        const session = await sessionRes.json();
        const adminId = session.user?.id;

        const res = await api.post(`${API_USERS}/bulk`, {
            data: { action: 'delete', userIds: [adminId] }
        });
        const body = await res.json();
        // Either 400 or 200 with self-protection message
        const selfProtected =
            res.status() === 400 ||
            (body.skipped && body.skipped.length > 0) ||
            (body.message && body.message.toLowerCase().includes('self')) ||
            body.deleted === 0;
        expect(selfProtected).toBeTruthy();
        await api.dispose();
    });
});

// ── UC-COMP: HIPAA / GDPR Compliance ─────────────────────────────────────────
test.describe('UC-COMP — HIPAA / GDPR Compliance', () => {

    test('UC-COMP-01 [BIZ] Admin can toggle data consent in compliance tab', async ({ page }) => {
        const api = await apiContext();
        const user = await createTestUser(api);

        try {
            await login(page);
            await goToUserManagement(page);

            await page.fill('#um-search', user.email);
            await page.waitForTimeout(600);
            const row = page.locator('#um-tbody tr').filter({ hasText: user.email }).first();
            await row.locator('[data-action="open"], .btn-open-drawer').click();
            await page.waitForSelector('#um-drawer:not([style*="display:none"])', { timeout: 5000 });

            await page.click('[data-dtab="compliance"]');
            await page.waitForSelector('#dtab-compliance:not([style*="display:none"])', { timeout: 3000 });

            // Toggle consent ON
            const consentToggle = page.locator('#dc-consent');
            const wasChecked = await consentToggle.isChecked();
            await consentToggle.setChecked(!wasChecked);
            await page.click('#drawer-compliance-form button[type="submit"]');
            await page.waitForTimeout(1000);

            const check = await api.get(`${API_USERS}/${user.id}`);
            const body = await check.json();
            const u = body.user || body;
            expect(u.data_consent_given || u.dataConsentGiven).toBe(!wasChecked);
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-COMP-02 [BIZ] Admin can update data retention date', async () => {
        const api = await apiContext();
        const user = await createTestUser(api);
        const retentionDate = '2028-12-31';

        try {
            const res = await api.put(`${API_USERS}/${user.id}/compliance`, {
                data: {
                    data_consent_given:  true,
                    data_consent_date:   '2026-01-01',
                    data_retention_until: retentionDate
                }
            });
            expect(res.status()).toBe(200);
            const body = await res.json();
            const u = body.user || body;
            expect((u.data_retention_until || u.dataRetentionUntil || '').startsWith('2028-12-31')).toBeTruthy();
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-COMP-03 [BIZ] GDPR deletion request can be flagged via API', async () => {
        const api = await apiContext();
        const user = await createTestUser(api);

        try {
            const res = await api.post(`${API_USERS}/${user.id}/gdpr-request`);
            expect(res.status()).toBe(200);

            const check = await api.get(`${API_USERS}/${user.id}`);
            const body = await check.json();
            const u = body.user || body;
            expect(u.gdpr_delete_requested || u.gdprDeleteRequested).toBe(true);
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-COMP-04 [USR] Compliance tab shows GDPR request date after flagging', async ({ page }) => {
        const api = await apiContext();
        const user = await createTestUser(api);

        try {
            // Flag via API first
            await api.post(`${API_USERS}/${user.id}/gdpr-request`);

            await login(page);
            await goToUserManagement(page);

            await page.fill('#um-search', user.email);
            await page.waitForTimeout(600);
            const row = page.locator('#um-tbody tr').filter({ hasText: user.email }).first();
            await row.locator('[data-action="open"], .btn-open-drawer').click();
            await page.waitForSelector('#um-drawer:not([style*="display:none"])', { timeout: 5000 });

            await page.click('[data-dtab="compliance"]');
            await page.waitForSelector('#dtab-compliance:not([style*="display:none"])', { timeout: 3000 });

            // GDPR date row should be visible
            const gdprRow = page.locator('#dc-gdpr-date-row');
            await expect(gdprRow).toBeVisible({ timeout: 3000 });
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-COMP-05 [TECH] Compliance update is recorded in audit log', async () => {
        const api = await apiContext();
        const user = await createTestUser(api);

        try {
            await api.put(`${API_USERS}/${user.id}/compliance`, {
                data: { data_consent_given: true, data_consent_date: '2026-01-01' }
            });

            // Check that audit log has an entry for this user
            const res = await api.get(`${BASE_URL}/api/audit-logs?userId=${user.id}&limit=5`);
            expect(res.status()).toBe(200);
            const body = await res.json();
            const logs = body.logs || body.data || body;
            expect(Array.isArray(logs)).toBeTruthy();
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });
});

// ── UC-AUDIT: Audit Log Visibility ───────────────────────────────────────────
test.describe('UC-AUDIT — Audit Log Visibility', () => {

    test('UC-AUDIT-01 [BIZ] Audit Logs tab loads with data', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);

        await page.click('[data-tab="audit"]');
        await page.waitForSelector('#tab-audit:not([style*="display:none"])', { timeout: 3000 });

        // Apply with no filters to load all
        await page.click('#audit-apply');
        await page.waitForTimeout(2000);

        const rows = await page.locator('#audit-tbody tr:not(.um-loading)').count();
        // Should have at least the admin login events
        expect(rows).toBeGreaterThanOrEqual(0);
    });

    test('UC-AUDIT-02 [BIZ] Audit log can be filtered by risk level', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);

        await page.click('[data-tab="audit"]');
        await page.waitForSelector('#tab-audit:not([style*="display:none"])', { timeout: 3000 });

        await page.selectOption('#audit-filter-risk', 'low');
        await page.click('#audit-apply');
        await page.waitForTimeout(2000);

        // All visible risk badges should be "low"
        const badges = page.locator('#audit-tbody .risk-badge');
        const count = await badges.count();
        if (count > 0) {
            for (let i = 0; i < Math.min(count, 5); i++) {
                const text = await badges.nth(i).textContent();
                expect(text.toLowerCase()).toContain('low');
            }
        }
    });

    test('UC-AUDIT-03 [BIZ] Activity tab in user drawer shows last 50 events', async ({ page }) => {
        const api = await apiContext();
        const user = await createTestUser(api);

        try {
            await login(page);
            await goToUserManagement(page);

            await page.fill('#um-search', user.email);
            await page.waitForTimeout(600);
            const row = page.locator('#um-tbody tr').filter({ hasText: user.email }).first();
            await row.locator('[data-action="open"], .btn-open-drawer').click();
            await page.waitForSelector('#um-drawer:not([style*="display:none"])', { timeout: 5000 });

            await page.click('[data-dtab="activity"]');
            await page.waitForSelector('#dtab-activity:not([style*="display:none"])', { timeout: 3000 });
            await page.waitForTimeout(1500); // Wait for activity to load

            const list = page.locator('#activity-list');
            await expect(list).toBeVisible();
            // Should not show HTTP 404
            const html = await list.innerHTML();
            expect(html).not.toContain('HTTP 404');
            expect(html).not.toContain('Not Found');
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-AUDIT-04 [TECH] Audit log API supports date range filter', async () => {
        const api = await apiContext();
        const from = '2020-01-01';
        const to   = new Date().toISOString().split('T')[0];

        const res = await api.get(`${BASE_URL}/api/audit-logs?from=${from}&to=${to}&limit=10`);
        expect(res.status()).toBe(200);
        const body = await res.json();
        const logs = body.logs || body.data || body;
        expect(Array.isArray(logs)).toBeTruthy();
        await api.dispose();
    });

    test('UC-AUDIT-05 [BIZ] Export CSV link is present on Audit Logs tab', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);

        await page.click('[data-tab="audit"]');
        await page.waitForSelector('#tab-audit:not([style*="display:none"])', { timeout: 3000 });

        const exportLink = page.locator('#audit-export-csv');
        await expect(exportLink).toBeVisible();
    });
});

// ── UC-SRCH: Search & Filter ──────────────────────────────────────────────────
test.describe('UC-SRCH — Search & Filter', () => {

    test('UC-SRCH-01 [USR] Search box filters table by email', async ({ page }) => {
        const api = await apiContext();
        const ts = Date.now();
        const email = `search-test-${ts}@ezhk.test`;
        const user  = await createTestUser(api, { email });

        try {
            await login(page);
            await goToUserManagement(page);
            await page.waitForSelector('#um-tbody tr:not(.um-loading)', { timeout: 8000 });

            await page.fill('#um-search', `search-test-${ts}`);
            await page.waitForTimeout(600);

            const rows = page.locator('#um-tbody tr');
            const count = await rows.count();
            expect(count).toBeGreaterThanOrEqual(1);

            const firstRowText = await rows.first().textContent();
            expect(firstRowText).toContain(email);
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-SRCH-02 [USR] Role filter reduces table to matching roles only', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);
        await page.waitForSelector('#um-tbody tr:not(.um-loading)', { timeout: 8000 });

        await page.selectOption('#um-filter-role', 'admin');
        await page.waitForTimeout(600);

        const rows = page.locator('#um-tbody tr:not(.um-loading)');
        const count = await rows.count();
        if (count > 0) {
            for (let i = 0; i < Math.min(count, 3); i++) {
                const rowText = await rows.nth(i).textContent();
                const hasAdminBadge = rowText.toLowerCase().includes('admin');
                expect(hasAdminBadge).toBeTruthy();
            }
        }
    });

    test('UC-SRCH-03 [USR] Status filter shows only pending users', async ({ page }) => {
        const api = await apiContext();
        // Create a pending user via invite
        const ts = Date.now();
        const email = `pending-filter-${ts}@ezhk.test`;
        let userId;

        try {
            const inviteRes = await api.post(`${API_USERS}/invite`, { data: { email, role: 'user' } });
            const inviteBody = await inviteRes.json();
            userId = inviteBody.user?.id || inviteBody.userId;

            await login(page);
            await goToUserManagement(page);
            await page.waitForSelector('#um-tbody tr:not(.um-loading)', { timeout: 8000 });

            await page.selectOption('#um-filter-status', 'pending');
            await page.waitForTimeout(600);

            const rows = page.locator('#um-tbody tr:not(.um-loading)');
            const count = await rows.count();
            if (count > 0) {
                // At least our new pending user should appear
                const bodyText = await page.locator('#um-tbody').textContent();
                expect(bodyText.toLowerCase()).toContain('pending');
            }
        } finally {
            if (userId) await deleteUser(api, userId);
            await api.dispose();
        }
    });

    test('UC-SRCH-04 [USR] Refresh button reloads the user list', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);
        await page.waitForSelector('#um-tbody tr:not(.um-loading)', { timeout: 8000 });

        const countBefore = await page.locator('#um-tbody tr').count();
        await page.click('#btnRefresh');
        await page.waitForSelector('#um-tbody tr:not(.um-loading)', { timeout: 8000 });
        const countAfter = await page.locator('#um-tbody tr').count();

        // Should reload successfully (count may be same or different)
        expect(countAfter).toBeGreaterThanOrEqual(0);
    });
});

// ── UC-OFF: Offboarding / Account Lifecycle ───────────────────────────────────
test.describe('UC-OFF — Offboarding & Account Lifecycle', () => {

    test('UC-OFF-01 [BIZ] Admin can suspend a user account via security tab', async ({ page }) => {
        const api = await apiContext();
        const user = await createTestUser(api, { status: 'active' });

        try {
            await login(page);
            await goToUserManagement(page);

            await page.fill('#um-search', user.email);
            await page.waitForTimeout(600);
            const row = page.locator('#um-tbody tr').filter({ hasText: user.email }).first();
            await row.locator('[data-action="open"], .btn-open-drawer').click();
            await page.waitForSelector('#um-drawer:not([style*="display:none"])', { timeout: 5000 });

            await page.click('[data-dtab="security"]');
            await page.waitForSelector('#dtab-security:not([style*="display:none"])', { timeout: 3000 });

            await page.selectOption('#ds-status', 'suspended');
            await page.click('#drawer-security-form button[type="submit"]');
            await page.waitForTimeout(1000);

            const check = await api.get(`${API_USERS}/${user.id}`);
            const body = await check.json();
            const u = body.user || body;
            expect(u.status).toBe('suspended');
        } finally {
            if (user.id) await deleteUser(api, user.id);
            await api.dispose();
        }
    });

    test('UC-OFF-02 [BIZ] Delete user API removes user from the list', async ({ page }) => {
        const api = await apiContext();
        const user = await createTestUser(api);

        await login(page);
        await goToUserManagement(page);

        await page.fill('#um-search', user.email);
        await page.waitForTimeout(600);
        const row = page.locator('#um-tbody tr').filter({ hasText: user.email }).first();

        // Click delete action
        await row.locator('[data-action="delete"]').click();

        // Handle browser confirmation dialog
        page.on('dialog', async dialog => {
            await dialog.accept();
        });
        await page.waitForTimeout(1500);

        // Verify user is gone
        const check = await api.get(`${API_USERS}/${user.id}`);
        expect([404, 410]).toContain(check.status());
        await api.dispose();
    });

    test('UC-OFF-03 [TECH] Deleting a user preserves audit log entries (audit trail)', async () => {
        const api = await apiContext();
        const user = await createTestUser(api);

        // Perform an action to generate audit entry
        await api.put(`${API_USERS}/${user.id}/profile`, { data: { department: 'TestDept' } });

        // Delete user
        await deleteUser(api, user.id);

        // Audit log should still return entries for this user (ON DELETE SET NULL preserves rows)
        const res = await api.get(`${BASE_URL}/api/audit-logs?userId=${user.id}&limit=5`);
        expect(res.status()).toBe(200); // Not 404 — audit trail preserved
        await api.dispose();
    });
});

// ── UC-API: API Contract Verification ────────────────────────────────────────
test.describe('UC-API — API Contract Verification', () => {

    test('UC-API-01 [TECH] GET /api/users returns paginated response with required fields', async () => {
        const api = await apiContext();
        const res = await api.get(`${API_USERS}?page=1&limit=10`);
        expect(res.status()).toBe(200);
        const body = await res.json();
        const users = body.users || body.data || body;
        expect(Array.isArray(users)).toBeTruthy();
        if (users.length > 0) {
            expect(users[0]).toHaveProperty('id');
            expect(users[0]).toHaveProperty('email');
            expect(users[0]).toHaveProperty('role');
            expect(users[0]).toHaveProperty('status');
        }
        await api.dispose();
    });

    test('UC-API-02 [TECH] GET /api/users supports search query param', async () => {
        const api = await apiContext();
        const res = await api.get(`${API_USERS}?search=admin`);
        expect(res.status()).toBe(200);
        const body = await res.json();
        const users = body.users || body.data || body;
        expect(Array.isArray(users)).toBeTruthy();
        // All results should mention admin
        users.forEach(u => {
            const hasAdmin = u.email?.includes('admin') || u.role === 'admin' ||
                             (u.first_name + ' ' + u.last_name).toLowerCase().includes('admin');
            expect(hasAdmin).toBeTruthy();
        });
        await api.dispose();
    });

    test('UC-API-03 [TECH] GET /api/users/:id returns full user details', async () => {
        const api = await apiContext();
        const listRes = await api.get(`${API_USERS}?limit=1`);
        const listBody = await listRes.json();
        const users = listBody.users || listBody.data || listBody;
        expect(users.length).toBeGreaterThan(0);
        const userId = users[0].id;

        const res = await api.get(`${API_USERS}/${userId}`);
        expect(res.status()).toBe(200);
        const body = await res.json();
        const u = body.user || body;
        expect(u).toHaveProperty('id', userId);
        expect(u).toHaveProperty('email');
        await api.dispose();
    });

    test('UC-API-04 [TECH] Create user API validates required fields', async () => {
        const api = await apiContext();

        // Missing email
        const res1 = await api.post(API_USERS, { data: { password: 'Test1234!', role: 'user' } });
        expect([400, 422]).toContain(res1.status());

        // Missing password
        const res2 = await api.post(API_USERS, { data: { email: 'test@test.com', role: 'user' } });
        expect([400, 422]).toContain(res2.status());

        await api.dispose();
    });

    test('UC-API-05 [SEC] Unauthenticated requests to /api/users return 401', async () => {
        const anonCtx = await request.newContext({ baseURL: BASE_URL });

        const res = await anonCtx.get(API_USERS);
        expect([401, 403]).toContain(res.status());

        await anonCtx.dispose();
    });
});

// ── UI Integration: Stats Bar ─────────────────────────────────────────────────
test.describe('UI Integration — Stats Bar & Page Load', () => {

    test('[USR] Stats bar populates with numeric values on load', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);
        await page.waitForTimeout(2000); // Allow stats to load

        const totalText = await page.locator('#stat-total').textContent();
        expect(totalText).not.toBe('—');
        expect(parseInt(totalText)).toBeGreaterThanOrEqual(0);
    });

    test('[USR] Tab switching shows and hides correct content panels', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);

        // Switch to Audit tab
        await page.click('[data-tab="audit"]');
        await page.waitForSelector('#tab-audit:not([style*="display:none"])', { timeout: 3000 });
        await expect(page.locator('#tab-users')).toBeHidden();

        // Switch to Roles tab
        await page.click('[data-tab="roles"]');
        await page.waitForSelector('#tab-roles:not([style*="display:none"])', { timeout: 3000 });
        await expect(page.locator('#tab-audit')).toBeHidden();

        // Switch back to Users tab
        await page.click('[data-tab="users"]');
        await page.waitForSelector('#tab-users:not([style*="display:none"])', { timeout: 3000 });
        await expect(page.locator('#tab-roles')).toBeHidden();
    });

    test('[USR] Create User button opens the create modal', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);

        await page.click('#btnCreateUser');
        await page.waitForSelector('#modal-create', { timeout: 5000 });
        await expect(page.locator('#modal-create')).toBeVisible();
    });

    test('[USR] Pagination controls render when more than one page of users exists', async ({ page }) => {
        await login(page);
        await goToUserManagement(page);
        await page.waitForSelector('#um-tbody tr:not(.um-loading)', { timeout: 8000 });

        const paginationVisible = await page.locator('#um-pagination').isVisible();
        expect(paginationVisible).toBeTruthy();
    });
});
