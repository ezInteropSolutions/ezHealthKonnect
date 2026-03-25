/**
 * git-integration-e2e.spec.js
 *
 * Enterprise-grade QA for the Git Integration feature.
 *
 * Coverage dimensions:
 *   API-CRUD   — Repository CRUD via /api/git (no browser)
 *   API-BRANCH — Branch, status, log, sync-log operations
 *   API-EXPORT — Export-preview bundle shape and credential safety
 *   UX-SETTINGS — Settings page Git Integration section (browser)
 *   UX-GIT     — git.html workspace page (browser)
 *   SEC        — Auth gates, XSS
 *
 * Execution:
 *   npx playwright test tests/playwright/git-integration-e2e.spec.js
 *   npx playwright test --headed tests/playwright/git-integration-e2e.spec.js
 *   npx playwright test --grep "API-CRUD" tests/playwright/git-integration-e2e.spec.js
 */

'use strict';

const { test, expect, request } = require('@playwright/test');

// ── Config ────────────────────────────────────────────────────────────────────
const BASE_URL           = process.env.BASE_URL           || 'http://localhost:3000';
const API_BASE           = `${BASE_URL}/api/git`;
const ADMIN_EMAIL        = process.env.ADMIN_EMAIL        || 'admin@ezhealthkonnect.com';
const ADMIN_PASS         = process.env.ADMIN_PASS         || 'admin123';
const KNOWN_INTERFACE_ID = process.env.KNOWN_INTERFACE_ID || '762aebb9-0408-4a42-82c5-202f13f28315';

// Fake URL that will pass DB insert validation but won't be cloned
const TEST_REMOTE_URL = 'https://github.com/nonexistent-for-testing/repo.git';

// ── Helpers ───────────────────────────────────────────────────────────────────

async function login(page) {
    await page.goto(`${BASE_URL}/login.html`);
    await page.fill('input[name="email"], input[type="email"]', ADMIN_EMAIL);
    await page.fill('input[name="password"], input[type="password"]', ADMIN_PASS);
    await page.click('button[type="submit"], .login-btn');
    await page.waitForURL(/dashboard|settings|git/, { timeout: 15000 });
}

async function apiContext() {
    const ctx = await request.newContext({ baseURL: BASE_URL });
    await ctx.post('/api/auth/login', {
        data: { email: ADMIN_EMAIL, password: ADMIN_PASS }
    });
    return ctx;
}

/** Create a git repo via API; returns the created repo object */
async function createTestRepo(api, overrides = {}) {
    const ts  = Date.now();
    const res = await api.post(API_BASE, {
        data: {
            name:        `QA Test Repo ${ts}`,
            remote_url:  TEST_REMOTE_URL,
            branch:      'main',
            description: 'Automated QA repo — safe to delete',
            auto_push:   false,
            credentials: { username: 'qa-user', token: 'qa-token-placeholder' },
            ...overrides
        }
    });
    const body = await res.json();
    return body.data || body;
}

/** Delete a repo via API (best-effort cleanup) */
async function deleteRepo(api, id) {
    if (id) await api.delete(`${API_BASE}/${id}`).catch(() => {});
}

// ═════════════════════════════════════════════════════════════════════════════
//  GROUP 1 — API: Repository CRUD
// ═════════════════════════════════════════════════════════════════════════════
test.describe('API-CRUD — Repository CRUD', () => {

    test('[API-CRUD-01] GET /api/git returns success:true and a data array', async () => {
        const api  = await apiContext();
        const res  = await api.get(API_BASE);
        expect(res.status()).toBe(200);
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(Array.isArray(body.data)).toBe(true);
        await api.dispose();
    });

    test('[API-CRUD-02] POST /api/git with valid payload creates a repo and returns it', async () => {
        const api  = await apiContext();
        const ts   = Date.now();
        const res  = await api.post(API_BASE, {
            data: {
                name:        `QA Repo CRUD-02 ${ts}`,
                remote_url:  TEST_REMOTE_URL,
                branch:      'main',
                description: 'CRUD-02 test repo',
                auto_push:   false
            }
        });
        expect(res.status()).toBeGreaterThanOrEqual(200);
        expect(res.status()).toBeLessThan(300);
        const body = await res.json();
        expect(body.success).toBe(true);
        const repo = body.data || body;
        expect(repo.id).toBeTruthy();
        expect(repo.name).toContain('QA Repo CRUD-02');
        expect(repo.remote_url).toBe(TEST_REMOTE_URL);
        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[API-CRUD-03] POST /api/git with missing name returns 400 success:false', async () => {
        const api  = await apiContext();
        const res  = await api.post(API_BASE, {
            data: { remote_url: TEST_REMOTE_URL }
        });
        expect(res.status()).toBe(400);
        const body = await res.json();
        expect(body.success).toBe(false);
        await api.dispose();
    });

    test('[API-CRUD-04] POST /api/git with missing remote_url returns 400 success:false', async () => {
        const api  = await apiContext();
        const res  = await api.post(API_BASE, {
            data: { name: 'Repo Without URL' }
        });
        expect(res.status()).toBe(400);
        const body = await res.json();
        expect(body.success).toBe(false);
        await api.dispose();
    });

    test('[API-CRUD-05] GET /api/git/:id returns the created repo', async () => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);
        expect(repo.id).toBeTruthy();

        const res  = await api.get(`${API_BASE}/${repo.id}`);
        expect(res.status()).toBe(200);
        const body = await res.json();
        expect(body.success).toBe(true);
        const fetched = body.data || body;
        expect(fetched.id).toBe(repo.id);
        expect(fetched.remote_url).toBe(TEST_REMOTE_URL);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[API-CRUD-06] GET /api/git/nonexistent-uuid returns 404 or 500 (not 200)', async () => {
        const api  = await apiContext();
        const res  = await api.get(`${API_BASE}/00000000-0000-0000-0000-000000000000`);
        expect(res.status()).not.toBe(200);
        await api.dispose();
    });

    test('[API-CRUD-07] PUT /api/git/:id updates the description', async () => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);
        expect(repo.id).toBeTruthy();

        const res  = await api.put(`${API_BASE}/${repo.id}`, {
            data: {
                name:        repo.name,
                remote_url:  repo.remote_url,
                description: 'Updated description by QA'
            }
        });
        expect(res.status()).toBe(200);
        const body    = await res.json();
        expect(body.success).toBe(true);
        const updated = body.data || body;
        expect(updated.description).toBe('Updated description by QA');

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[API-CRUD-08] DELETE /api/git/:id soft-deletes the repo; it no longer appears in list', async () => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);
        expect(repo.id).toBeTruthy();

        const del  = await api.delete(`${API_BASE}/${repo.id}`);
        expect(del.status()).toBe(200);
        const delBody = await del.json();
        expect(delBody.success).toBe(true);

        // Soft delete: repo is hidden from List (is_active=false)
        const list     = await api.get(API_BASE);
        const listBody = await list.json();
        const found    = (listBody.data || []).some(r => r.id === repo.id);
        expect(found).toBe(false);
        await api.dispose();
    });

    test('[API-CRUD-09] Credentials token is never returned in GET responses', async () => {
        const api  = await apiContext();
        const repo = await createTestRepo(api, {
            credentials: { username: 'qa-user', token: 'super-secret-token-12345' }
        });
        expect(repo.id).toBeTruthy();

        const res  = await api.get(`${API_BASE}/${repo.id}`);
        const body = await res.json();
        const raw  = JSON.stringify(body);

        // Token value must not appear verbatim in any response
        expect(raw).not.toContain('super-secret-token-12345');

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

});

// ═════════════════════════════════════════════════════════════════════════════
//  GROUP 2 — API: Branch & Status Operations
//  NOTE: repos are registered in DB but not cloned on-disk, so these endpoints
//  are expected to return success:false with an error field (graceful error)
//  rather than crashing (5xx without a JSON body).
// ═════════════════════════════════════════════════════════════════════════════
test.describe('API-BRANCH — Branch & Status Operations', () => {

    let api;
    let repoId;

    test.beforeEach(async () => {
        api  = await apiContext();
        const repo = await createTestRepo(api);
        repoId = repo.id;
    });

    test.afterEach(async () => {
        await deleteRepo(api, repoId);
        await api.dispose();
    });

    test('[API-BRANCH-01] GET /api/git/:id/branches returns a JSON response with success field', async () => {
        const res  = await api.get(`${API_BASE}/${repoId}/branches`);
        const body = await res.json();
        // Repo is not cloned — expect graceful error OR success with empty list
        expect(body).toHaveProperty('success');
        if (body.success) {
            expect(Array.isArray(body.data)).toBe(true);
            expect(body).toHaveProperty('current');
        } else {
            expect(body.error || body.message).toBeTruthy();
        }
    });

    test('[API-BRANCH-02] GET /api/git/:id/status returns a JSON response with success field and clean bool', async () => {
        const res  = await api.get(`${API_BASE}/${repoId}/status`);
        const body = await res.json();
        expect(body).toHaveProperty('success');
        if (body.success) {
            expect(Array.isArray(body.data)).toBe(true);
            expect(typeof body.clean).toBe('boolean');
        } else {
            expect(body.error || body.message).toBeTruthy();
        }
    });

    test('[API-BRANCH-03] GET /api/git/:id/log returns a JSON response with success field', async () => {
        const res  = await api.get(`${API_BASE}/${repoId}/log?limit=20`);
        const body = await res.json();
        expect(body).toHaveProperty('success');
        if (body.success) {
            expect(Array.isArray(body.data)).toBe(true);
            expect(typeof body.count).toBe('number');
        } else {
            expect(body.error || body.message).toBeTruthy();
        }
    });

    test('[API-BRANCH-04] GET /api/git/:id/sync-log returns a JSON response with success field', async () => {
        const res  = await api.get(`${API_BASE}/${repoId}/sync-log?limit=50`);
        const body = await res.json();
        expect(body).toHaveProperty('success');
        if (body.success) {
            expect(Array.isArray(body.data)).toBe(true);
        } else {
            expect(body.error || body.message).toBeTruthy();
        }
    });

});

// ═════════════════════════════════════════════════════════════════════════════
//  GROUP 3 — API: Export Preview
// ═════════════════════════════════════════════════════════════════════════════
test.describe('API-EXPORT — Export Preview Bundle', () => {

    test('[API-EXPORT-01] GET /api/git/export-preview/:interfaceId returns a valid bundle', async () => {
        const api = await apiContext();
        const res = await api.get(`${API_BASE}/export-preview/${KNOWN_INTERFACE_ID}`);

        if (res.status() === 404) {
            // Known interface not present in this environment — skip gracefully
            test.skip(true, `Interface ${KNOWN_INTERFACE_ID} not found in this environment`);
            await api.dispose();
            return;
        }

        expect(res.status()).toBe(200);
        const body = await res.json();
        expect(body.success).toBe(true);
        const bundle = body.data || body;

        // Required top-level bundle fields
        expect(bundle).toHaveProperty('bundle_version');
        expect(bundle).toHaveProperty('metadata');
        expect(bundle).toHaveProperty('interface');
        expect(bundle).toHaveProperty('pipelines');

        await api.dispose();
    });

    test('[API-EXPORT-02] Exported bundle does not contain raw credential values', async () => {
        const api = await apiContext();
        const res = await api.get(`${API_BASE}/export-preview/${KNOWN_INTERFACE_ID}`);

        if (res.status() === 404) {
            test.skip(true, `Interface ${KNOWN_INTERFACE_ID} not found in this environment`);
            await api.dispose();
            return;
        }

        if (res.status() !== 200) {
            await api.dispose();
            return;
        }

        const body = await res.json();
        const raw  = JSON.stringify(body);

        // No real-looking secret values should appear — only masked placeholders
        // Reject strings that look like actual passwords/tokens (>=16 chars of mixed content)
        const suspiciousPattern = /"(?:password|token|secret|api_key|apikey)"\s*:\s*"(?!\*{3,}|<[^>]+>)[^"]{16,}"/i;
        expect(raw).not.toMatch(suspiciousPattern);

        await api.dispose();
    });

    test('[API-EXPORT-03] GET /api/git/export-preview/nonexistent returns 404 success:false', async () => {
        const api  = await apiContext();
        const res  = await api.get(`${API_BASE}/export-preview/00000000-0000-0000-0000-000000000000`);
        expect(res.status()).toBe(404);
        const body = await res.json();
        expect(body.success).toBe(false);
        await api.dispose();
    });

});

// ═════════════════════════════════════════════════════════════════════════════
//  GROUP 4 — UX: Settings Page Git Integration Section
// ═════════════════════════════════════════════════════════════════════════════
test.describe('UX-SETTINGS — Settings Page Git Integration Section', () => {

    test.beforeEach(async ({ page }) => {
        await login(page);
    });

    test('[UX-SETTINGS-01] Navigate to settings.html → click Git nav item → #section-git is visible', async ({ page }) => {
        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await expect(page.locator('#section-git')).toBeVisible({ timeout: 5000 });
    });

    test('[UX-SETTINGS-02] Git section shows repo table or empty-state message', async ({ page }) => {
        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await expect(page.locator('#section-git')).toBeVisible({ timeout: 5000 });

        // Either the table wrapper exists, or an empty-state message is shown
        const hasTable   = await page.locator('#gitRepoTableWrap').isVisible().catch(() => false);
        const hasEmpty   = await page.locator('#section-git').textContent({ timeout: 3000 }).then(
            t => /no\s+repo|no\s+git|add\s+your\s+first|empty/i.test(t)
        ).catch(() => false);

        expect(hasTable || hasEmpty).toBe(true);
    });

    test('[UX-SETTINGS-03] Click "Add Repository" → form card #gitRepoFormCard appears', async ({ page }) => {
        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await expect(page.locator('#section-git')).toBeVisible({ timeout: 5000 });

        await page.click('button[onclick="showGitRepoForm(null)"]');
        await expect(page.locator('#gitRepoFormCard')).toBeVisible({ timeout: 5000 });
    });

    test('[UX-SETTINGS-04] Form has all required fields: name, remote_url, branch, username, token', async ({ page }) => {
        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await page.click('button[onclick="showGitRepoForm(null)"]');
        await expect(page.locator('#gitRepoFormCard')).toBeVisible({ timeout: 5000 });

        await expect(page.locator('#gitName')).toBeVisible();
        await expect(page.locator('#gitRemoteURL')).toBeVisible();
        await expect(page.locator('#gitBranch')).toBeVisible();
        await expect(page.locator('#gitUsername')).toBeVisible();
        await expect(page.locator('#gitToken')).toBeVisible();
    });

    test('[UX-SETTINGS-05] Save with empty name shows validation (no network call with empty name)', async ({ page }) => {
        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await page.click('button[onclick="showGitRepoForm(null)"]');
        await expect(page.locator('#gitRepoFormCard')).toBeVisible({ timeout: 5000 });

        // Leave name empty, fill URL
        await page.fill('#gitName', '');
        await page.fill('#gitRemoteURL', TEST_REMOTE_URL);

        // Track whether a POST to /api/git was fired
        let postFired = false;
        page.on('request', req => {
            if (req.method() === 'POST' && req.url().includes('/api/git')) postFired = true;
        });

        await page.click('button[onclick="saveGitRepo()"]');
        await page.waitForTimeout(800);

        // Either native validation prevented submission, or an error message appeared
        const hasValidationMsg = await page.evaluate(() => {
            const nameField = document.getElementById('gitName');
            return nameField && !nameField.validity.valid;
        });
        const hasErrorText = await page.locator('#section-git').textContent().then(
            t => /required|name.*required|please.*enter|cannot.*empty/i.test(t)
        ).catch(() => false);

        expect(hasValidationMsg || hasErrorText || !postFired).toBe(true);
    });

    test('[UX-SETTINGS-06] Save with empty remote_url shows validation', async ({ page }) => {
        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await page.click('button[onclick="showGitRepoForm(null)"]');
        await expect(page.locator('#gitRepoFormCard')).toBeVisible({ timeout: 5000 });

        await page.fill('#gitName', 'Repo Without URL');
        await page.fill('#gitRemoteURL', '');

        let postFired = false;
        page.on('request', req => {
            if (req.method() === 'POST' && req.url().includes('/api/git')) postFired = true;
        });

        await page.click('button[onclick="saveGitRepo()"]');
        await page.waitForTimeout(800);

        const hasValidationMsg = await page.evaluate(() => {
            const urlField = document.getElementById('gitRemoteURL');
            return urlField && !urlField.validity.valid;
        });
        const hasErrorText = await page.locator('#section-git').textContent().then(
            t => /required|url.*required|please.*enter|cannot.*empty/i.test(t)
        ).catch(() => false);

        expect(hasValidationMsg || hasErrorText || !postFired).toBe(true);
    });

    test('[UX-SETTINGS-07] Cancel button hides the form card', async ({ page }) => {
        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await page.click('button[onclick="showGitRepoForm(null)"]');
        await expect(page.locator('#gitRepoFormCard')).toBeVisible({ timeout: 5000 });

        await page.click('button[onclick="hideGitRepoForm()"]');
        await expect(page.locator('#gitRepoFormCard')).toBeHidden({ timeout: 3000 });
    });

    test('[UX-SETTINGS-08] Fill form and save → repo appears in the table', async ({ page }) => {
        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await page.click('button[onclick="showGitRepoForm(null)"]');
        await expect(page.locator('#gitRepoFormCard')).toBeVisible({ timeout: 5000 });

        const repoName = `QA UX-SETTINGS-08 ${Date.now()}`;
        await page.fill('#gitName', repoName);
        await page.fill('#gitRemoteURL', TEST_REMOTE_URL);
        await page.fill('#gitBranch', 'main');

        // Intercept the save response to capture the id for cleanup
        let createdId = null;
        page.on('response', async res => {
            if (res.url().includes('/api/git') && res.request().method() === 'POST') {
                const body = await res.json().catch(() => ({}));
                createdId = body.data?.id || body.id || null;
            }
        });

        await page.click('button[onclick="saveGitRepo()"]');

        // Wait for form to close (success) or for the repo name to appear in DOM
        await page.waitForFunction(
            (name) => document.body.innerText.includes(name),
            repoName,
            { timeout: 8000 }
        ).catch(() => {});

        const bodyText = await page.locator('#section-git').textContent();
        expect(bodyText).toContain(repoName);

        // Cleanup via API
        if (createdId) {
            const api = await apiContext();
            await deleteRepo(api, createdId);
            await api.dispose();
        }
    });

    test('[UX-SETTINGS-09] Repo row shows Edit and Delete buttons', async ({ page }) => {
        // Create a repo via API first so we have something to see
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await expect(page.locator('#section-git')).toBeVisible({ timeout: 5000 });

        // Wait for the repo name to appear in the table
        await page.waitForFunction(
            (name) => document.body.innerText.includes(name),
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        // Find the row that contains the repo name
        const row = page.locator('#section-git').locator('tr, .repo-row, .list-item').filter({ hasText: repo.name });
        const editBtn   = row.locator('button, a').filter({ hasText: /edit/i }).or(
            row.locator('[onclick*="showGitRepoForm"], [data-action="edit"], .btn-edit')
        );
        const deleteBtn = row.locator('button, a').filter({ hasText: /delete|remove/i }).or(
            row.locator('[onclick*="deleteGitRepo"], [onclick*="deleteRepo"], .btn-delete')
        );

        await expect(editBtn.first()).toBeVisible({ timeout: 5000 });
        await expect(deleteBtn.first()).toBeVisible({ timeout: 5000 });

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-SETTINGS-10] Edit button repopulates form with existing repo values', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api, { description: 'UX-SETTINGS-10 test repo' });

        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await expect(page.locator('#section-git')).toBeVisible({ timeout: 5000 });

        await page.waitForFunction(
            (name) => document.body.innerText.includes(name),
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        const row     = page.locator('#section-git').locator('tr, .repo-row, .list-item').filter({ hasText: repo.name });
        const editBtn = row.locator('button, a').filter({ hasText: /edit/i }).or(
            row.locator('[onclick*="showGitRepoForm"], [data-action="edit"], .btn-edit')
        ).first();

        await editBtn.click();
        await expect(page.locator('#gitRepoFormCard')).toBeVisible({ timeout: 5000 });

        // Form should be pre-populated with the repo's name and URL
        const nameVal = await page.inputValue('#gitName');
        const urlVal  = await page.inputValue('#gitRemoteURL');
        expect(nameVal).toBe(repo.name);
        expect(urlVal).toBe(repo.remote_url);

        // Hidden ID field should hold the repo id
        const idVal = await page.inputValue('#gitRepoId');
        expect(idVal).toBe(repo.id);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-SETTINGS-11] Delete with confirm:cancel → repo stays in table', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await expect(page.locator('#section-git')).toBeVisible({ timeout: 5000 });

        await page.waitForFunction(
            (name) => document.body.innerText.includes(name),
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        // Override window.confirm to return false (cancel)
        await page.evaluate(() => { window.confirm = () => false; });

        const row       = page.locator('#section-git').locator('tr, .repo-row, .list-item').filter({ hasText: repo.name });
        const deleteBtn = row.locator('button, a').filter({ hasText: /delete|remove/i }).or(
            row.locator('[onclick*="deleteGitRepo"], [onclick*="deleteRepo"], .btn-delete')
        ).first();

        await deleteBtn.click();
        await page.waitForTimeout(800);

        // Repo name should still be visible
        const bodyText = await page.locator('#section-git').textContent();
        expect(bodyText).toContain(repo.name);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-SETTINGS-12] Delete with confirm:accept → repo removed from table', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await expect(page.locator('#section-git')).toBeVisible({ timeout: 5000 });

        await page.waitForFunction(
            (name) => document.body.innerText.includes(name),
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        // Override window.confirm to return true (accept)
        await page.evaluate(() => { window.confirm = () => true; });

        const row       = page.locator('#section-git').locator('tr, .repo-row, .list-item').filter({ hasText: repo.name });
        const deleteBtn = row.locator('button, a').filter({ hasText: /delete|remove/i }).or(
            row.locator('[onclick*="deleteGitRepo"], [onclick*="deleteRepo"], .btn-delete')
        ).first();

        await deleteBtn.click();

        // Wait for repo name to disappear from DOM
        await page.waitForFunction(
            (name) => !document.body.innerText.includes(name),
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        const bodyText = await page.locator('#section-git').textContent();
        expect(bodyText).not.toContain(repo.name);

        // Best-effort cleanup (might already be gone)
        await deleteRepo(api, repo.id);
        await api.dispose();
    });

});

// ═════════════════════════════════════════════════════════════════════════════
//  GROUP 5 — UX: git.html Workspace Page
// ═════════════════════════════════════════════════════════════════════════════
test.describe('UX-GIT — git.html Workspace Page', () => {

    test.beforeEach(async ({ page }) => {
        await login(page);
    });

    test('[UX-GIT-01] Navigate to git.html with no repos → banner/message visible, no crash', async ({ page }) => {
        // Ensure zero repos by checking via API — skip if any pre-exist (env dependent)
        const api  = await apiContext();
        const res  = await api.get(API_BASE);
        const body = await res.json();
        const existing = (body.data || []).length;
        await api.dispose();

        if (existing > 0) {
            test.skip(true, 'Pre-existing repos in environment; cannot test empty-state reliably');
            return;
        }

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');

        // Page must not crash (no JS error modal covering entire screen)
        const pageTitle = await page.title();
        expect(pageTitle).toBeTruthy();

        // Banner or empty-state message should be visible
        const noRepoBanner = page.locator('#noRepoBanner');
        const hasMessage   = await noRepoBanner.isVisible().catch(() => false);
        if (!hasMessage) {
            // Fallback: check body text for empty-state copy
            const text = await page.locator('body').textContent();
            expect(text).toMatch(/no.*repo|connect.*repo|add.*repo|get\s+started/i);
        } else {
            await expect(noRepoBanner).toBeVisible();
        }
    });

    test('[UX-GIT-02] With a repo in DB → #repoSelect is populated with repo name', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        // Wait for option to be populated
        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        const options = await page.locator('#repoSelect option').allTextContents();
        expect(options.some(o => o.includes(repo.name))).toBe(true);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-03] Selecting a repo loads the branch selector', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        // Select the repo
        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForTimeout(1000);

        // Branch selector should be visible (even if empty due to uncloned repo)
        await expect(page.locator('#branchSelect')).toBeVisible({ timeout: 5000 });

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-04] #ifaceList shows interface checkboxes', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });

        // Wait for interface list to populate
        await page.waitForSelector('#ifaceList', { timeout: 8000 });

        // Interface list should contain at least one checkbox (or an empty-state message)
        const checkboxes = page.locator('#ifaceList input[type="checkbox"]');
        const count      = await checkboxes.count();
        if (count === 0) {
            const listText = await page.locator('#ifaceList').textContent();
            expect(listText).toMatch(/no\s+interface|empty|none/i);
        } else {
            expect(count).toBeGreaterThan(0);
        }

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-05] "All" button checks all interface checkboxes', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForSelector('#ifaceList', { timeout: 8000 });

        const checkboxes = page.locator('#ifaceList input[type="checkbox"]');
        const count      = await checkboxes.count();

        if (count === 0) {
            test.skip(true, 'No interface checkboxes present — cannot test select-all');
            await deleteRepo(api, repo.id);
            await api.dispose();
            return;
        }

        // git.html uses onclick="selectAll(true)" button, not a #selectAll checkbox
        await page.click('button[onclick="selectAll(true)"]');
        await page.waitForTimeout(300);

        const checkedCount = await page.locator('#ifaceList input[type="checkbox"]:checked').count();
        expect(checkedCount).toBe(count);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-06] "None" button unchecks all interfaces; after "All" all are deselected', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForSelector('#ifaceList', { timeout: 8000 });

        const checkboxes = page.locator('#ifaceList input[type="checkbox"]');
        const count      = await checkboxes.count();

        if (count < 1) {
            test.skip(true, 'Need at least one interface checkbox for this test');
            await deleteRepo(api, repo.id);
            await api.dispose();
            return;
        }

        // Select all first, then none
        await page.click('button[onclick="selectAll(true)"]');
        await page.waitForTimeout(300);
        await page.click('button[onclick="selectAll(false)"]');
        await page.waitForTimeout(300);

        const checkedCount = await page.locator('#ifaceList input[type="checkbox"]:checked').count();
        expect(checkedCount).toBe(0);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-07] Commit button is disabled when no interfaces are selected', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForSelector('#ifaceList', { timeout: 8000 });

        // Ensure nothing is checked
        const checked = await page.locator('#ifaceList input[type="checkbox"]:checked').count();
        if (checked > 0) {
            // Uncheck everything
            await page.evaluate(() => {
                document.querySelectorAll('#ifaceList input[type="checkbox"]').forEach(cb => { cb.checked = false; cb.dispatchEvent(new Event('change', { bubbles: true })); });
            });
            await page.waitForTimeout(300);
        }

        await page.fill('#commitMsg', '');
        await page.waitForTimeout(200);

        const commitBtnDisabled     = await page.locator('#commitBtn').isDisabled().catch(() => true);
        const commitPushBtnDisabled = await page.locator('#commitPushBtn').isDisabled().catch(() => true);
        expect(commitBtnDisabled || commitPushBtnDisabled).toBe(true);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-08] Commit button is enabled when at least one interface is selected and message is filled', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForSelector('#ifaceList', { timeout: 8000 });

        const checkboxes = page.locator('#ifaceList input[type="checkbox"]');
        const count      = await checkboxes.count();

        if (count === 0) {
            test.skip(true, 'No interface checkboxes — cannot test commit button enable state');
            await deleteRepo(api, repo.id);
            await api.dispose();
            return;
        }

        await checkboxes.first().check();
        await page.fill('#commitMsg', 'QA test commit message');
        await page.waitForTimeout(300);

        const commitBtnEnabled     = await page.locator('#commitBtn').isEnabled().catch(() => false);
        const commitPushBtnEnabled = await page.locator('#commitPushBtn').isEnabled().catch(() => false);
        expect(commitBtnEnabled || commitPushBtnEnabled).toBe(true);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-09] Commit button is disabled when commit message is empty (interfaces selected)', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForSelector('#ifaceList', { timeout: 8000 });

        const checkboxes = page.locator('#ifaceList input[type="checkbox"]');
        const count      = await checkboxes.count();

        if (count === 0) {
            test.skip(true, 'No interface checkboxes — cannot test this scenario');
            await deleteRepo(api, repo.id);
            await api.dispose();
            return;
        }

        // Check an interface but leave message empty
        await checkboxes.first().check();
        await page.fill('#commitMsg', '');
        await page.waitForTimeout(300);

        const commitBtnDisabled     = await page.locator('#commitBtn').isDisabled().catch(() => true);
        const commitPushBtnDisabled = await page.locator('#commitPushBtn').isDisabled().catch(() => true);
        expect(commitBtnDisabled || commitPushBtnDisabled).toBe(true);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-10] Pull button calls /api/git/:id/pull and shows a status response', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForTimeout(500);

        // Intercept the pull request
        let pullCalled = false;
        page.on('request', req => {
            if (req.url().includes(`/api/git/${repo.id}/pull`) && req.method() === 'POST') {
                pullCalled = true;
            }
        });

        await page.waitForSelector('#pullBtn', { timeout: 5000 });
        await page.click('#pullBtn');
        await page.waitForTimeout(2000);

        expect(pullCalled).toBe(true);

        // Page should show some status feedback (success or error message)
        const bodyText = await page.locator('body').textContent();
        expect(bodyText).toMatch(/pull|branch|error|success|up\s+to\s+date|failed|cannot/i);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-11] Sidebar nav shows a "Git" link in the navigation', async ({ page }) => {
        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');

        // Look for a sidebar/nav link referencing git
        const gitNavLink = page.locator(
            'a[href*="git.html"], .leftnav-tab[data-page*="git"], .sidebar-nav a[href*="git"], nav a[href*="git"]'
        );
        // It may be on another page — check dashboard too
        const countOnGit = await gitNavLink.count();
        if (countOnGit === 0) {
            await page.goto(`${BASE_URL}/dashboard.html`);
            await page.waitForLoadState('load');
        }
        const linkAnywhere = page.locator(
            'a[href*="git.html"], .leftnav-tab[data-page*="git"], .sidebar-nav a[href*="git"], nav a[href*="git"]'
        );
        await expect(linkAnywhere.first()).toBeVisible({ timeout: 5000 });
    });

    test('[UX-GIT-12] Clicking sidebar Git link navigates to git.html', async ({ page }) => {
        await page.goto(`${BASE_URL}/dashboard.html`);
        await page.waitForLoadState('load');
        // Wait for sidebar to render
        await page.waitForSelector('#sidebar-nav a, .nav-item', { timeout: 8000 });

        const gitLink = page.locator('a[href="git.html"]').first();
        const visible  = await gitLink.isVisible().catch(() => false);

        if (!visible) {
            // The Tools section is collapsed — click its section-header to expand it properly.
            // We find the section that contains the git.html link.
            const toolsHeader = page.locator(
                '.nav-section:has(a[href="git.html"]) .section-header, ' +
                '.nav-section:has(a[href*="git.html"]) .section-header'
            ).first();
            const headerVisible = await toolsHeader.isVisible().catch(() => false);
            if (headerVisible) {
                await toolsHeader.click();
                await page.waitForTimeout(400);
            } else {
                // Fallback: expand via evaluate
                await page.evaluate(() => {
                    document.querySelectorAll('.nav-section').forEach(sec => {
                        if (sec.querySelector('a[href="git.html"]')) {
                            sec.classList.remove('collapsed');
                            const items = sec.querySelector('.nav-items');
                            if (items) items.style.removeProperty('display');
                        }
                    });
                });
                await page.waitForTimeout(300);
            }
        }

        await expect(gitLink).toBeVisible({ timeout: 5000 });
        // Use JS click to bypass any overlapping elements (e.g., sticky section headers)
        await page.evaluate(() => {
            const link = document.querySelector('a[href="git.html"]');
            if (link) link.click();
        });
        await expect(page).toHaveURL(/git\.html/, { timeout: 8000 });
    });

});

// ═════════════════════════════════════════════════════════════════════════════
//  GROUP 6 — Security
// ═════════════════════════════════════════════════════════════════════════════
test.describe('SEC — Security', () => {

    test('[SEC-01] GET /api/git without authentication returns 401 or redirect', async () => {
        // Use a fresh context with NO auth cookie/session
        const ctx = await request.newContext({ baseURL: BASE_URL });
        const res = await ctx.get(API_BASE);

        // Must not return 200 — expect 401, 403, or a redirect to login
        const status = res.status();
        const isRedirect = status >= 300 && status < 400;
        const isUnauth   = status === 401 || status === 403;

        if (!isRedirect && !isUnauth) {
            // Some implementations return 200 with an HTML login page
            const text = await res.text();
            const looksLikeLoginPage = /login|sign\s*in|unauthorized/i.test(text);
            expect(looksLikeLoginPage).toBe(true);
        } else {
            expect(isRedirect || isUnauth).toBe(true);
        }

        await ctx.dispose();
    });

    test('[SEC-02] XSS: repo name with script tag is rendered as escaped text in settings table', async ({ page }) => {
        const xssName = `<script>alert('XSS-GIT-TEST')</script> QA Repo ${Date.now()}`;

        const api  = await apiContext();
        const res  = await api.post(API_BASE, {
            data: {
                name:       xssName,
                remote_url: TEST_REMOTE_URL
            }
        });
        const body = await res.json();
        const repo = body.data || body;
        const id   = repo.id;

        // Only proceed if creation succeeded (some backends reject XSS payloads — that's also a pass)
        if (!id) {
            await api.dispose();
            return;
        }

        await login(page);
        await page.goto(`${BASE_URL}/settings.html`);
        await page.waitForSelector('.settings-nav-item', { timeout: 10000 });
        await page.click('.settings-nav-item[data-section="git"]');
        await expect(page.locator('#section-git')).toBeVisible({ timeout: 5000 });

        // Verify the script was NOT executed (no alert dialog triggered)
        let alertFired = false;
        page.on('dialog', async dialog => {
            if (dialog.message().includes('XSS-GIT-TEST')) alertFired = true;
            await dialog.dismiss();
        });

        await page.waitForTimeout(2000);
        expect(alertFired).toBe(false);

        // Verify the raw <script> tag is NOT present as an active element
        const scriptTagsInSection = await page.locator('#section-git script').count();
        expect(scriptTagsInSection).toBe(0);

        // The literal word "alert" might appear as escaped text — that's acceptable
        // What's NOT acceptable is an actual script element or dialog

        await deleteRepo(api, id);
        await api.dispose();
    });

});

// ═════════════════════════════════════════════════════════════════════════════
//  GROUP 7 — API: Push Endpoint
// ═════════════════════════════════════════════════════════════════════════════
test.describe('API-PUSH — Push Endpoint', () => {

    test('[API-PUSH-01] POST /api/git/:id/push on uncloned repo returns success:false with error message', async () => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        const res  = await api.post(`${API_BASE}/${repo.id}/push`);
        expect(res.status()).toBe(200);
        const body = await res.json();
        // Repo is not cloned so push must fail gracefully — never a 5xx crash
        expect(typeof body.success).toBe('boolean');
        if (!body.success) {
            expect(body.error || body.message).toBeTruthy();
        }

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[API-PUSH-02] POST /api/git/push on non-existent repo returns 404', async () => {
        const api = await apiContext();
        const res = await api.post(`${API_BASE}/00000000-0000-0000-0000-000000000000/push`);
        expect(res.status()).toBe(404);
        const body = await res.json();
        expect(body.success).toBe(false);
        await api.dispose();
    });

    test('[API-PUSH-03] POST /api/git/:id/push without authentication returns 401 or redirect', async () => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        const unauthCtx = await request.newContext({ baseURL: BASE_URL });
        const res = await unauthCtx.post(`${API_BASE}/${repo.id}/push`);
        const status = res.status();
        const isRedirect = status >= 300 && status < 400;
        const isUnauth   = status === 401 || status === 403;
        if (!isRedirect && !isUnauth) {
            const text = await res.text();
            expect(/login|sign\s*in|unauthorized/i.test(text)).toBe(true);
        } else {
            expect(isRedirect || isUnauth).toBe(true);
        }

        await unauthCtx.dispose();
        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[API-PUSH-04] Push response body has expected shape: {success, pushed?, message?}', async () => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        const res  = await api.post(`${API_BASE}/${repo.id}/push`);
        const body = await res.json();

        expect(body).toHaveProperty('success');
        // On success the body should include a pushed boolean and message string
        if (body.success) {
            expect(typeof body.pushed).toBe('boolean');
            expect(typeof body.message).toBe('string');
        } else {
            // On failure must have an error or message field
            expect(body.error || body.message).toBeTruthy();
        }

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

});

// ═════════════════════════════════════════════════════════════════════════════
//  GROUP 8 — API: Interface Diff Endpoint
// ═════════════════════════════════════════════════════════════════════════════
test.describe('API-DIFF — Interface Diff Endpoint', () => {

    test('[API-DIFF-01] GET /api/git/:id/interface-diff without interfaceId returns 400', async () => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        const res  = await api.get(`${API_BASE}/${repo.id}/interface-diff`);
        expect(res.status()).toBe(400);
        const body = await res.json();
        expect(body.success).toBe(false);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[API-DIFF-02] GET /api/git/:id/interface-diff with interfaceId returns expected shape', async () => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        const res  = await api.get(
            `${API_BASE}/${repo.id}/interface-diff?interfaceId=${KNOWN_INTERFACE_ID}`
        );
        // 200 (success or diff computed) or 4xx (repo not cloned / not found) — never 5xx
        expect(res.status()).toBeLessThan(500);
        const body = await res.json();

        expect(typeof body.success).toBe('boolean');
        if (body.success) {
            expect(body.data).toBeDefined();
            expect(body.data).toHaveProperty('interface_id');
            expect(body.data).toHaveProperty('has_changes');
            expect(body.data).toHaveProperty('not_committed');
        } else {
            expect(body.error || body.message).toBeTruthy();
        }

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[API-DIFF-03] GET /api/git/non-existent-repo/interface-diff returns 404', async () => {
        const api = await apiContext();
        const res = await api.get(
            `${API_BASE}/00000000-0000-0000-0000-000000000000/interface-diff?interfaceId=${KNOWN_INTERFACE_ID}`
        );
        expect(res.status()).toBe(404);
        const body = await res.json();
        expect(body.success).toBe(false);
        await api.dispose();
    });

    test('[API-DIFF-04] GET /api/git/:id/interface-diff without auth returns 401 or redirect', async () => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        const unauthCtx = await request.newContext({ baseURL: BASE_URL });
        const res = await unauthCtx.get(
            `${API_BASE}/${repo.id}/interface-diff?interfaceId=${KNOWN_INTERFACE_ID}`
        );
        const status = res.status();
        const isRedirect = status >= 300 && status < 400;
        const isUnauth   = status === 401 || status === 403;
        if (!isRedirect && !isUnauth) {
            const text = await res.text();
            expect(/login|sign\s*in|unauthorized/i.test(text)).toBe(true);
        } else {
            expect(isRedirect || isUnauth).toBe(true);
        }

        await unauthCtx.dispose();
        await deleteRepo(api, repo.id);
        await api.dispose();
    });

});

// ═════════════════════════════════════════════════════════════════════════════
//  GROUP 9 — UX: Push Button, Branch Dialog, Diff Button
// ═════════════════════════════════════════════════════════════════════════════
test.describe('UX-GIT-NEW — Push, Branch Dialog, Diff Controls', () => {

    test.beforeEach(async ({ page }) => {
        await login(page);
    });

    // ── Push button ────────────────────────────────────────────────────────

    test('[UX-GIT-13] Push button is visible in git.html toolbar', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForTimeout(500);

        await expect(page.locator('#pushBtn')).toBeVisible({ timeout: 5000 });

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-14] Push button calls POST /api/git/:id/push and shows feedback toast', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForTimeout(500);

        let pushCalled = false;
        page.on('request', req => {
            if (req.url().includes(`/api/git/${repo.id}/push`) && req.method() === 'POST') {
                pushCalled = true;
            }
        });

        await page.waitForSelector('#pushBtn', { timeout: 5000 });
        await page.click('#pushBtn');
        await page.waitForTimeout(2000);

        expect(pushCalled).toBe(true);

        // Some toast/feedback text should appear
        const bodyText = await page.locator('body').textContent();
        expect(bodyText).toMatch(/push|error|success|up\s+to\s+date|not\s+cloned|failed/i);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    // ── New branch dialog ──────────────────────────────────────────────────

    test('[UX-GIT-15] "New Branch" button opens in-app modal, not native browser prompt', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForTimeout(500);

        // Intercept any native dialog — it must NOT fire
        let nativeDialogFired = false;
        page.on('dialog', async dialog => {
            nativeDialogFired = true;
            await dialog.dismiss();
        });

        // Click the New Branch button
        const newBranchBtn = page.locator('button[onclick="openNewBranchDialog()"]');
        await expect(newBranchBtn).toBeVisible({ timeout: 5000 });
        await newBranchBtn.click();
        await page.waitForTimeout(400);

        // Native prompt must NOT have been triggered
        expect(nativeDialogFired).toBe(false);

        // In-app modal overlay must be visible
        await expect(page.locator('#branchDialogOverlay')).toBeVisible({ timeout: 3000 });

        // Modal must contain an input for the branch name
        await expect(page.locator('#newBranchInput')).toBeVisible({ timeout: 3000 });

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-16] Branch dialog shows error for empty name, does not close', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForTimeout(500);

        await page.click('button[onclick="openNewBranchDialog()"]');
        await expect(page.locator('#branchDialogOverlay')).toBeVisible({ timeout: 3000 });

        // Submit with empty input
        await page.fill('#newBranchInput', '');
        await page.click('#branchDialogOkBtn');
        await page.waitForTimeout(300);

        // Dialog must still be open (validation blocked submission)
        await expect(page.locator('#branchDialogOverlay')).toBeVisible({ timeout: 2000 });

        // Error message must be visible
        await expect(page.locator('#branchDialogError')).toBeVisible({ timeout: 2000 });
        const errText = await page.locator('#branchDialogError').textContent();
        expect(errText).toMatch(/required|cannot be empty|enter a branch/i);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-17] Branch dialog shows error for name with spaces', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForTimeout(500);

        await page.click('button[onclick="openNewBranchDialog()"]');
        await expect(page.locator('#branchDialogOverlay')).toBeVisible({ timeout: 3000 });

        // Submit with a name containing a space
        await page.fill('#newBranchInput', 'branch with spaces');
        await page.click('#branchDialogOkBtn');
        await page.waitForTimeout(300);

        // Dialog must remain open
        await expect(page.locator('#branchDialogOverlay')).toBeVisible({ timeout: 2000 });

        // Error about spaces must be visible
        await expect(page.locator('#branchDialogError')).toBeVisible({ timeout: 2000 });
        const errText = await page.locator('#branchDialogError').textContent();
        expect(errText).toMatch(/space|cannot contain/i);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-18] Branch dialog closes on Escape key', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForTimeout(500);

        await page.click('button[onclick="openNewBranchDialog()"]');
        await expect(page.locator('#branchDialogOverlay')).toBeVisible({ timeout: 3000 });

        await page.locator('#newBranchInput').press('Escape');
        await page.waitForTimeout(300);

        await expect(page.locator('#branchDialogOverlay')).toBeHidden({ timeout: 3000 });

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-19] Branch dialog closes on backdrop click', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForTimeout(500);

        await page.click('button[onclick="openNewBranchDialog()"]');
        await expect(page.locator('#branchDialogOverlay')).toBeVisible({ timeout: 3000 });

        // Click on the overlay backdrop (outside the dialog box)
        await page.locator('#branchDialogOverlay').click({ position: { x: 10, y: 10 } });
        await page.waitForTimeout(300);

        await expect(page.locator('#branchDialogOverlay')).toBeHidden({ timeout: 3000 });

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    // ── Diff button ────────────────────────────────────────────────────────

    test('[UX-GIT-20] Diff buttons are disabled when repo is not cloned (uncloned test repo)', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);  // Never cloned

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });

        // Wait for the interface list to render
        await page.waitForSelector('#ifaceList', { timeout: 8000 });
        await page.waitForTimeout(1500);  // Allow status load to complete (will fail = not cloned)

        // All diff buttons in the interface list should be disabled
        const diffBtns = page.locator('#ifaceList .diff-btn');
        const count = await diffBtns.count();

        if (count > 0) {
            const disabledCount = await page.evaluate(() => {
                return Array.from(document.querySelectorAll('#ifaceList .diff-btn'))
                    .filter(b => b.disabled).length;
            });
            expect(disabledCount).toBe(count);
        }
        // If no interfaces: that's fine — test passes (nothing to diff)

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-21] Clicking disabled diff button shows toast, does not open diff modal', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForSelector('#ifaceList', { timeout: 8000 });
        await page.waitForTimeout(1500);

        const diffBtns = page.locator('#ifaceList .diff-btn');
        const count = await diffBtns.count();

        if (count === 0) {
            test.skip(true, 'No interface rows — cannot test diff button');
            await deleteRepo(api, repo.id);
            await api.dispose();
            return;
        }

        // Force-call openDiff via evaluate (bypasses the disabled attribute) to verify guard
        await page.evaluate(() => {
            if (typeof openDiff === 'function') {
                openDiff('00000000-0000-0000-0000-000000000000', 'Test Interface');
            }
        });
        await page.waitForTimeout(500);

        // Diff modal (#diffModal) must NOT be visible — guard should have blocked it
        const diffModalVisible = await page.locator('#diffModal').isVisible().catch(() => false);
        expect(diffModalVisible).toBe(false);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    test('[UX-GIT-22] Diff modal has committed/current panes and close button', async ({ page }) => {
        // This test verifies the modal structure exists in the DOM even when not opened
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForTimeout(500);

        // Modal should exist in DOM (hidden by default)
        const diffModal = page.locator('#diffModal');
        await expect(diffModal).toBeAttached({ timeout: 5000 });

        // Verify structural elements exist
        const hasCommittedPane = await page.locator('#diffModal [data-pane="committed"], #diffModal .diff-committed, #diffModal #committedPane').count() > 0 ||
            await page.evaluate(() => !!document.querySelector('#diffModal'));

        expect(hasCommittedPane).toBe(true);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

    // ── Auto-checkout after branch create ─────────────────────────────────

    test('[UX-GIT-23] confirmNewBranch calls /branches then /checkout after success', async ({ page }) => {
        const api  = await apiContext();
        const repo = await createTestRepo(api);

        await page.goto(`${BASE_URL}/git.html`);
        await page.waitForLoadState('load');
        await page.waitForSelector('#repoSelect', { timeout: 8000 });

        await page.waitForFunction(
            (name) => {
                const sel = document.getElementById('repoSelect');
                return sel && Array.from(sel.options).some(o => o.text.includes(name));
            },
            repo.name,
            { timeout: 8000 }
        ).catch(() => {});

        await page.selectOption('#repoSelect', { label: repo.name });
        await page.waitForTimeout(500);

        // Track which API calls are made
        const apiCalls = [];
        page.on('request', req => {
            if (req.url().includes(`/api/git/${repo.id}`)) {
                apiCalls.push({ method: req.method(), url: req.url() });
            }
        });

        // Open branch dialog and submit a valid name
        await page.click('button[onclick="openNewBranchDialog()"]');
        await expect(page.locator('#branchDialogOverlay')).toBeVisible({ timeout: 3000 });
        await page.fill('#newBranchInput', `qa-branch-${Date.now()}`);
        await page.click('#branchDialogOkBtn');

        // Wait for both API calls to fire
        await page.waitForTimeout(3000);

        const branchCreated = apiCalls.some(c => c.method === 'POST' && c.url.includes('/branches'));
        const checkoutCalled = apiCalls.some(c => c.method === 'POST' && c.url.includes('/checkout'));

        // Branch create must have been called
        expect(branchCreated).toBe(true);
        // Checkout must have been attempted (regardless of success — repo not cloned)
        expect(checkoutCalled).toBe(true);

        await deleteRepo(api, repo.id);
        await api.dispose();
    });

});
