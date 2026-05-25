/**
 * Playwright UI tests — Admin Dead-Letter Queue page
 *
 * Test cases:
 *   TC-DLQ-UI001  Page loads and displays stats bar
 *   TC-DLQ-UI002  Stats show numeric values after load
 *   TC-DLQ-UI003  Table renders with correct columns
 *   TC-DLQ-UI004  Table shows empty-state when no rows
 *   TC-DLQ-UI005  Seeded pending rows appear in table
 *   TC-DLQ-UI006  Status filter changes visible rows
 *   TC-DLQ-UI007  Row checkbox selects a row (highlighted)
 *   TC-DLQ-UI008  Select-all checkbox selects all rows
 *   TC-DLQ-UI009  Bulk-action bar appears when rows selected
 *   TC-DLQ-UI010  Bulk-action bar hides when selection cleared
 *   TC-DLQ-UI011  Redrive button opens redrive modal
 *   TC-DLQ-UI012  Redrive modal has both mode options
 *   TC-DLQ-UI013  Selecting "From Start" highlights that option
 *   TC-DLQ-UI014  Cancel button closes redrive modal
 *   TC-DLQ-UI015  Detail drawer opens on search button click
 *   TC-DLQ-UI016  Detail drawer shows row Identity section
 *   TC-DLQ-UI017  Detail drawer closes on X click
 *   TC-DLQ-UI018  Abandon button triggers browser confirm (cancelled)
 *   TC-DLQ-UI019  Refresh button re-loads the table
 *   TC-DLQ-UI020  Interface ID filter applied shows 0 rows for unknown ID
 *
 * Prerequisites:
 *   Stack running: npm run dev:all  OR  docker-compose up
 *   ADMIN_EMAIL / ADMIN_PASSWORD env vars (defaults to seeded admin)
 *   DATABASE_URL for seeding (optional — some tests skip without it)
 */

'use strict';

const { test, expect } = require('@playwright/test');
const { Client } = require('pg');

const BASE_URL      = process.env.BASE_URL      || 'http://localhost:3000';
const ADMIN_EMAIL   = process.env.ADMIN_EMAIL   || 'admin@ezhealthkonnect.com';
const ADMIN_PASS    = process.env.ADMIN_PASSWORD || 'admin123';
const DB_URL        = process.env.DATABASE_URL;

const TEST_IFACE_ID = '22222222-3333-4444-5555-666666666666';
const TEST_TAG      = `dlq-ui-${Date.now()}`;

// ─── DB helpers ───────────────────────────────────────────────────────────────
async function getDb() {
    if (!DB_URL) return null;
    const c = new Client({ connectionString: DB_URL });
    await c.connect();
    return c;
}

async function ensureInterface(db) {
    const userRow = await db.query(`SELECT id FROM users ORDER BY created_at LIMIT 1`);
    const userId  = userRow.rows[0]?.id;
    await db.query(`
        INSERT INTO interfaces
            (id, user_id, name, source_type, target_type, message_type, status, created_at, updated_at)
        VALUES ($1, $2::uuid, 'UI Test Interface (DLQ)', 'tcp_mllp', 'http_rest', 'ADT^A01', 'active', NOW(), NOW())
        ON CONFLICT (id) DO NOTHING`, [TEST_IFACE_ID, userId]);
}

async function seedRow(db, status = 'pending') {
    const msgId = `${TEST_TAG}-${Math.random().toString(36).slice(2)}`;
    const res = await db.query(`
        INSERT INTO delivery_dlq
            (message_id, interface_id, connector_type, payload, content_type,
             error_message, attempt_count, next_retry_at, status, redrive_mode)
        VALUES ($1, $2::uuid, 'http_outbound', '{"test":true}', 'application/json',
                'connection refused', 1, NOW() + INTERVAL '24 hours', $3, 'from_failed_step')
        RETURNING id`, [msgId, TEST_IFACE_ID, status]);
    return res.rows[0].id;
}

async function cleanup(db) {
    await db.query(
        `DELETE FROM delivery_dlq WHERE message_id LIKE $1`, [`${TEST_TAG}%`]);
    await db.query(
        `DELETE FROM interfaces WHERE id = $1`, [TEST_IFACE_ID]);
}

// ─── Auth helper — stores JWT in localStorage for the page ───────────────────
async function loginAndStoreToken(page) {
    const resp = await page.request.post('/api/auth/login', {
        data: { email: ADMIN_EMAIL, password: ADMIN_PASS },
    });
    const body = await resp.json();
    const token = body.token || body.accessToken || '';
    // Inject token before navigating so admin-dlq.js can read it
    await page.addInitScript((t) => {
        localStorage.setItem('accessToken', t);
    }, token);
    return token;
}

// ─── Shared fixture ───────────────────────────────────────────────────────────
let db;
let seededIDs = [];

test.beforeAll(async () => {
    db = await getDb();
    if (db) {
        await ensureInterface(db);
        // Seed 3 pending rows for tests that need visible table rows
        for (let i = 0; i < 3; i++) {
            seededIDs.push(await seedRow(db, 'pending'));
        }
    }
});

test.afterAll(async () => {
    if (db) {
        await cleanup(db);
        await db.end();
    }
});

// ─── Tests ───────────────────────────────────────────────────────────────────

test('TC-DLQ-UI001 Page loads and stats bar is visible', async ({ page }) => {
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.waitForLoadState('load');
    await expect(page.locator('#statsBar')).toBeVisible();
});

test('TC-DLQ-UI002 Stats show numeric values after load', async ({ page }) => {
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    // Wait up to 20s for stats to populate — Go backend may be warming up
    await page.waitForFunction(() => {
        const el = document.getElementById('statPending');
        return el && el.textContent !== '—';
    }, undefined, { timeout: 20000 });
    const pending = await page.locator('#statPending').textContent();
    expect(Number(pending)).toBeGreaterThanOrEqual(0);
});

test('TC-DLQ-UI003 Table renders correct column headers', async ({ page }) => {
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.waitForLoadState('load');
    const headers = await page.locator('.dlq-table thead th').allTextContents();
    const headerStr = headers.join(' ');
    expect(headerStr).toMatch(/Status/);
    expect(headerStr).toMatch(/Message ID/);
    expect(headerStr).toMatch(/Step/);
    expect(headerStr).toMatch(/Actions/);
});

test('TC-DLQ-UI004 Empty-state message shown when no rows', async ({ page }) => {
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    // Filter to abandoned — likely zero rows in test env
    await page.selectOption('#filterStatus', 'abandoned');
    // Apply filter with interface that doesn't exist → guaranteed empty
    await page.fill('#filterInterface', '00000000-0000-0000-0000-000000000000');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        const tbody = document.getElementById('dlqTableBody');
        return tbody && !tbody.innerHTML.includes('fa-spinner');
    }, undefined, { timeout: 6000 });
    await expect(page.locator('.empty-state')).toBeVisible();
});

test('TC-DLQ-UI005 Seeded pending rows visible in table', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping seeded-row visibility test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    // Filter to our test interface
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        const tbody = document.getElementById('dlqTableBody');
        return tbody && !tbody.innerHTML.includes('fa-spinner');
    }, undefined, { timeout: 8000 });
    const rows = await page.locator('#dlqTableBody tr').count();
    expect(rows).toBeGreaterThanOrEqual(3);
});

test('TC-DLQ-UI006 Status filter — "Resolved" shows different (usually empty) result', async ({ page }) => {
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'resolved');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        const tbody = document.getElementById('dlqTableBody');
        return tbody && !tbody.innerHTML.includes('fa-spinner');
    }, undefined, { timeout: 6000 });
    // Expect empty state — we haven't seeded any resolved rows
    await expect(page.locator('.empty-state')).toBeVisible();
});

test('TC-DLQ-UI007 Checkbox selects a row and highlights it', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping row-selection test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    const firstCheckbox = page.locator('#dlqTableBody input[type=checkbox]').first();
    await firstCheckbox.check();
    const firstRow = page.locator('#dlqTableBody tr').first();
    await expect(firstRow).toHaveClass(/selected/);
});

test('TC-DLQ-UI008 Select-all checkbox selects all visible rows', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping select-all test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    await page.locator('#checkAll').check();
    const allRows = page.locator('#dlqTableBody tr');
    const count   = await allRows.count();
    for (let i = 0; i < count; i++) {
        await expect(allRows.nth(i)).toHaveClass(/selected/);
    }
});

test('TC-DLQ-UI009 Bulk bar appears when rows are selected', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping bulk-bar test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    await expect(page.locator('#bulkBar')).not.toHaveClass(/visible/);
    await page.locator('#dlqTableBody input[type=checkbox]').first().check();
    await expect(page.locator('#bulkBar')).toHaveClass(/visible/);
    await expect(page.locator('#bulkCount')).toContainText('1 selected');
});

test('TC-DLQ-UI010 Bulk bar hides when Clear is clicked', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping bulk-bar clear test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    await page.locator('#dlqTableBody input[type=checkbox]').first().check();
    await expect(page.locator('#bulkBar')).toHaveClass(/visible/);
    await page.locator('#bulkBar button:has-text("Clear")').click();
    await expect(page.locator('#bulkBar')).not.toHaveClass(/visible/);
});

test('TC-DLQ-UI011 Row Redrive button opens the redrive modal', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping redrive modal test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    await page.locator('#dlqTableBody .btn-redrive').first().click();
    await expect(page.locator('#redriveModal')).toHaveClass(/open/);
});

test('TC-DLQ-UI012 Redrive modal has both mode options', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping redrive modal options test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    await page.locator('#dlqTableBody .btn-redrive').first().click();
    await expect(page.locator('#modeOptionFailed')).toContainText('From Failed Step');
    await expect(page.locator('#modeOptionStart')).toContainText('From Start');
});

test('TC-DLQ-UI013 Selecting "From Start" highlights that option', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping mode selection test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    await page.locator('#dlqTableBody .btn-redrive').first().click();
    // Default: "From Failed Step" is selected
    await expect(page.locator('#modeOptionFailed')).toHaveClass(/selected/);
    // Click "From Start"
    await page.locator('input[name=redriveMode][value=from_start]').click();
    await expect(page.locator('#modeOptionStart')).toHaveClass(/selected/);
    await expect(page.locator('#modeOptionFailed')).not.toHaveClass(/selected/);
});

test('TC-DLQ-UI014 Cancel button closes the redrive modal', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping modal cancel test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    await page.locator('#dlqTableBody .btn-redrive').first().click();
    await expect(page.locator('#redriveModal')).toHaveClass(/open/);
    await page.locator('#redriveModal button:has-text("Cancel")').click();
    await expect(page.locator('#redriveModal')).not.toHaveClass(/open/);
});

test('TC-DLQ-UI015 Detail drawer opens on search icon click', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping detail drawer test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    // Click the detail (search icon) button
    await page.locator('#dlqTableBody button[title=Detail]').first().click();
    await expect(page.locator('#drawerOverlay')).toHaveClass(/open/);
});

test('TC-DLQ-UI016 Detail drawer shows Identity section', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping drawer content test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    await page.locator('#dlqTableBody button[title=Detail]').first().click();
    // Wait for drawer content to load (spinner disappears)
    await page.waitForFunction(() => {
        const content = document.getElementById('drawerContent');
        return content && !content.innerHTML.includes('fa-spinner');
    }, undefined, { timeout: 6000 });
    await expect(page.locator('#drawerContent')).toContainText('Identity');
    await expect(page.locator('#drawerContent')).toContainText('Message ID');
});

test('TC-DLQ-UI017 Drawer closes when X button clicked', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping drawer close test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    await page.locator('#dlqTableBody button[title=Detail]').first().click();
    await expect(page.locator('#drawerOverlay')).toHaveClass(/open/);
    await page.locator('.drawer-close').click();
    await expect(page.locator('#drawerOverlay')).not.toHaveClass(/open/);
});

test('TC-DLQ-UI018 Abandon button shows browser confirm (dismiss = no change)', async ({ page }) => {
    test.skip(!db, 'No DATABASE_URL — skipping abandon confirm test');
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.fill('#filterInterface', TEST_IFACE_ID);
    await page.selectOption('#filterStatus', 'pending');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        return document.querySelectorAll('#dlqTableBody input[type=checkbox]').length > 0;
    }, undefined, { timeout: 8000 });
    // Dismiss the browser confirm dialog
    page.once('dialog', async (dialog) => {
        expect(dialog.message()).toContain('Abandon');
        await dialog.dismiss();
    });
    await page.locator('#dlqTableBody .btn-abandon').first().click();
    // Row should still be in the table (not removed)
    const rows = await page.locator('#dlqTableBody tr').count();
    expect(rows).toBeGreaterThanOrEqual(1);
});

test('TC-DLQ-UI019 Refresh button reloads the table', async ({ page }) => {
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    // Wait for initial load to complete
    await page.waitForFunction(() => {
        const tbody = document.getElementById('dlqTableBody');
        return tbody && !tbody.innerHTML.includes('fa-spinner');
    }, undefined, { timeout: 8000 });
    // Click refresh — should trigger a new load spinner then complete
    await page.click('button:has-text("Refresh")');
    // Brief spinner should appear
    await page.waitForFunction(() => {
        const tbody = document.getElementById('dlqTableBody');
        return tbody && !tbody.innerHTML.includes('fa-spinner');
    }, undefined, { timeout: 8000 });
    // Page still shows the stats bar
    await expect(page.locator('#statsBar')).toBeVisible();
});

test('TC-DLQ-UI020 Unknown interface ID filter returns empty state', async ({ page }) => {
    await loginAndStoreToken(page);
    await page.goto('/admin-dlq.html');
    await page.selectOption('#filterStatus', 'pending');
    await page.fill('#filterInterface', 'ffffffff-ffff-ffff-ffff-ffffffffffff');
    await page.click('button:has-text("Apply")');
    await page.waitForFunction(() => {
        const tbody = document.getElementById('dlqTableBody');
        return tbody && !tbody.innerHTML.includes('fa-spinner');
    }, undefined, { timeout: 6000 });
    await expect(page.locator('.empty-state')).toBeVisible();
});
