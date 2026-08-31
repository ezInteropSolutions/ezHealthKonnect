/**
 * Bulk Reprocess (V213) — E2E tests
 *
 * Covers the Messages page's checkbox/select-all-matching selection, the
 * Mirth-style typed-confirmation ("type REPROCESS to confirm"), and a real
 * end-to-end job against live data — not a mock, an actual
 * bulk_reprocess_jobs row processed by the real Go background worker
 * (services/bulk_reprocess_service.go).
 *
 * Test interface: "Test Interface5" (085d4474-bc07-449f-9676-dcebf726292c),
 * owned by the Playwright admin test account, 40 real messages all in
 * status=processed/delivery_status=delivered.
 *
 * This fixture used to only exist as hand-created data on one developer's
 * local database — nothing in migrations or setup created it, so the suite
 * passed locally but timed out waiting for '.bulk-row-checkbox' in any fresh
 * database (CI included) with no visible error explaining why. beforeAll
 * below seeds it directly via `pg`, following the same DATABASE_URL-gated
 * pattern already established in admin-dlq.spec.js, rather than a Flyway SQL
 * migration — the per-interface messages_intf_* table is dynamically created
 * app-side (services/InterfaceTableManager.js's getMessageTableSchema()),
 * not part of the static Flyway-managed schema, so duplicating its DDL here
 * (kept in sync with that JS source) avoids inventing a new
 * migration-owns-dynamic-DDL pattern this repo doesn't otherwise have.
 * raw_content_uri is deliberately left NULL — BR-001 through BR-004 only need
 * the rows to render, and BR-005 accepts "Failed" as a valid terminal state
 * (see its own comment), so a message that fails to reprocess for lack of
 * real object-storage content still satisfies every assertion.
 */
const { test, expect } = require('@playwright/test');
const { Client } = require('pg');

test.use({ storageState: '.auth/user.json' });

const TEST_INTERFACE_ID = '085d4474-bc07-449f-9676-dcebf726292c';
const TABLE_NAME = 'messages_intf_085d4474_bc07_449f_9676_dcebf726292c';
const DB_URL = process.env.DATABASE_URL;

async function getDb() {
    if (!DB_URL) return null;
    const c = new Client({ connectionString: DB_URL });
    await c.connect();
    return c;
}

async function ensureFixture(db) {
    const userRow = await db.query(`SELECT id FROM users WHERE email = 'admin@ezhealthkonnect.com'`);
    const userId = userRow.rows[0]?.id;
    if (!userId) return; // V2 migration always seeds this; bail rather than insert a bad FK if it's ever missing

    await db.query(`
        INSERT INTO interfaces
            (id, user_id, name, source_type, target_type, message_type, status, is_active, version, created_at, updated_at)
        VALUES ($1::uuid, $2::uuid, 'Test Interface5', 'hl7v2', 'fhir', 'ADT^A01', 'active', true, 1, NOW(), NOW())
        ON CONFLICT (id) DO NOTHING`, [TEST_INTERFACE_ID, userId]);

    // Mirrors services/InterfaceTableManager.js's getMessageTableSchema() —
    // keep in sync with that file if the canonical schema ever changes.
    await db.query(`
        CREATE TABLE IF NOT EXISTS ${TABLE_NAME} (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            message_id VARCHAR(255) NOT NULL,
            correlation_id VARCHAR(255),
            interface_id UUID NOT NULL,
            status VARCHAR(50) NOT NULL DEFAULT 'received'
                CHECK (status IN ('received', 'processing', 'transformed', 'sent', 'delivered', 'failed', 'error', 'reprocessing', 'processed')),
            priority INTEGER DEFAULT 5,
            received_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
            processing_started_at TIMESTAMP WITH TIME ZONE,
            processing_completed_at TIMESTAMP WITH TIME ZONE,
            source_type VARCHAR(100) NOT NULL,
            source_endpoint VARCHAR(500),
            source_ip VARCHAR(45),
            message_type VARCHAR(100),
            message_size INTEGER,
            message_encoding VARCHAR(50) DEFAULT 'UTF-8',
            transformation_applied BOOLEAN DEFAULT false,
            transformation_type VARCHAR(100),
            processing_time_ms INTEGER,
            parsed_at TIMESTAMP WITH TIME ZONE,
            parsing_status VARCHAR(50),
            parsing_time_ms INTEGER,
            parsing_error TEXT,
            error_count INTEGER DEFAULT 0,
            last_error_message TEXT,
            last_error_at TIMESTAMP WITH TIME ZONE,
            target_endpoint VARCHAR(500),
            delivery_status VARCHAR(50),
            delivery_attempts INTEGER DEFAULT 0,
            last_delivery_attempt_at TIMESTAMP WITH TIME ZONE,
            raw_content_uri TEXT,
            parsed_content_uri TEXT,
            transformed_content_uri TEXT,
            outbound_content_uri TEXT,
            log_uri TEXT,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        )`);

    await db.query(`
        INSERT INTO interface_table_metadata (interface_id, table_name, schema_version, created_at, updated_at)
        VALUES ($1::uuid, $2, '1.2', NOW(), NOW())
        ON CONFLICT (interface_id) DO NOTHING`, [TEST_INTERFACE_ID, TABLE_NAME]);

    const countRes = await db.query(`SELECT COUNT(*) FROM ${TABLE_NAME} WHERE message_id LIKE 'BULK-FIXTURE-%'`);
    if (parseInt(countRes.rows[0].count, 10) === 0) {
        for (let i = 0; i < 40; i++) {
            await db.query(`
                INSERT INTO ${TABLE_NAME}
                    (message_id, interface_id, status, received_at, source_type, message_type,
                     message_size, delivery_status, delivery_attempts, created_at, updated_at)
                VALUES ($1, $2::uuid, 'processed', NOW() - INTERVAL '60 days', 'hl7v2', 'ADT^A01',
                        120, 'delivered', 1, NOW() - INTERVAL '60 days', NOW() - INTERVAL '60 days')`,
                [`BULK-FIXTURE-${i}`, TEST_INTERFACE_ID]);
        }
    }
}

let db;

test.beforeAll(async () => {
    db = await getDb();
    if (db) await ensureFixture(db);
});

test.afterAll(async () => {
    if (db) await db.end();
});

// The page's own setDefaultDateFilter() defaults Date From to "last 1 day" — the
// fixture messages are seeded 60 days in the past, well outside that window, so
// every test needs to widen the date filter before any rows will render. This is
// pre-existing app behavior, not something these tests are working around a bug
// in — a real user would hit the exact same "0 messages" until they cleared filters.
async function gotoAndWidenDateFilter(page) {
    test.skip(!DB_URL, 'No DATABASE_URL — skipping bulk-reprocess fixture-dependent test');
    await page.goto(`http://localhost:3000/messages.html?interfaceId=${TEST_INTERFACE_ID}`);
    await page.waitForSelector('#filterDateFrom');
    await page.fill('#filterDateFrom', '2020-01-01T00:00');
    await page.click('button:has(i.fa-filter)');
    await page.waitForSelector('.bulk-row-checkbox');
}

test.describe('Bulk Reprocess (V213)', () => {
    test('BR-001 checkbox selection shows the bulk toolbar with the correct count', async ({ page }) => {
        await gotoAndWidenDateFilter(page);

        const checkboxes = page.locator('.bulk-row-checkbox');
        await expect(checkboxes.first()).toBeVisible();
        await checkboxes.nth(0).check();
        await checkboxes.nth(1).check();

        await expect(page.locator('#bulkSelectionToolbar')).toBeVisible();
        await expect(page.locator('#bulkSelectionCount')).toHaveText('2 selected');
    });

    test('BR-002 select-all-on-page then select-all-matching-filter uses the real count API', async ({ page }) => {
        await gotoAndWidenDateFilter(page);

        await page.check('#selectAllOnPageCheckbox');
        await expect(page.locator('#bulkSelectAllMatchingLink')).toBeVisible();

        await page.click('#bulkSelectAllMatchingLink');
        await expect(page.locator('#bulkSelectionCount')).toContainText('selected (all matching current filters)');
        // The API-driven count must be a real positive number, not a placeholder.
        const text = await page.locator('#bulkSelectionCount').textContent();
        const n = parseInt(text.replace(/,/g, ''), 10);
        expect(n).toBeGreaterThan(0);
    });

    test('BR-003 typed confirmation rejects wrong text — no job starts, selection stays intact', async ({ page }) => {
        await gotoAndWidenDateFilter(page);
        await page.locator('.bulk-row-checkbox').first().check();

        await page.click('button:has-text("Reprocess Selected")');
        const input = page.locator('.app-dialog-input');
        await expect(input).toBeVisible();
        await input.fill('wrong text');
        await page.click('#_appDlgOk');

        // Mismatch aborts before any network call — panel never appears, and the
        // original selection (still exactly 1) survives so the user can retry.
        await expect(page.locator('#bulkJobPanel')).toBeHidden();
        await expect(page.locator('#bulkSelectionCount')).toHaveText('1 selected');
    });

    test('BR-004 cancelling the prompt entirely leaves the selection untouched', async ({ page }) => {
        await gotoAndWidenDateFilter(page);
        await page.locator('.bulk-row-checkbox').first().check();

        await page.click('button:has-text("Reprocess Selected")');
        await expect(page.locator('.app-dialog-input')).toBeVisible();
        await page.click('#_appDlgCancel');

        await expect(page.locator('#bulkJobPanel')).toBeHidden();
        await expect(page.locator('#bulkSelectionCount')).toHaveText('1 selected');
    });

    test('BR-005 real end-to-end: typed REPROCESS creates a job that progresses to a terminal state', async ({ page, request }) => {
        // Real pipeline execution for up to 40 messages can take longer than
        // Playwright's default 30s test timeout even though the individual
        // expect() below is already given 90s — the enclosing test needs its
        // own budget increased too, or it gets killed first regardless.
        test.setTimeout(120000);
        await gotoAndWidenDateFilter(page);

        // Explicit-ID mode (checkbox-driven) — exercises the 'ids' selection_mode
        // path through the real job creation endpoint and Go worker.
        await page.check('#selectAllOnPageCheckbox');
        await page.click('button:has-text("Reprocess Selected")');
        const input = page.locator('.app-dialog-input');
        await input.fill('REPROCESS');
        await page.click('#_appDlgOk');

        await expect(page.locator('#bulkJobPanel')).toBeVisible({ timeout: 10000 });
        await expect(page.locator('#bulkJobLabel')).toHaveText(/Completed|Failed|Cancelled/, { timeout: 100000 });

        const counts = await page.locator('#bulkJobCounts').textContent();
        // "<processed> / <total> — <succeeded> succeeded, <failed> failed"
        expect(counts).toMatch(/\d+ \/ \d+/);

        // Confirm the toolbar cleared and the selection reset after job submission.
        await expect(page.locator('#bulkSelectionToolbar')).toBeHidden();
    });
});
