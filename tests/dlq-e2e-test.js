/**
 * DLQ API — Node.js E2E Test
 *
 * Exercises all /api/fhir/dlq endpoints against a live running stack
 * (Node.js proxy on PORT + Go backend on API_PORT).
 *
 * Test cases:
 *   TC-DLQ-E001  Auth — login succeeds and cookie is set
 *   TC-DLQ-E002  Stats — returns success:true with status counts
 *   TC-DLQ-E003  List  — returns rows for the test interface
 *   TC-DLQ-E004  Get   — returns a single row by ID
 *   TC-DLQ-E005  Get   — returns 404 for non-existent ID
 *   TC-DLQ-E006  Redrive — returns 200 (or 500 with error detail) for a real row
 *   TC-DLQ-E007  AbandonOne — marks a row abandoned
 *   TC-DLQ-E008  BulkRedrive — rejects empty ids with 400
 *   TC-DLQ-E009  BulkRedrive — processes multiple ids
 *   TC-DLQ-E010  BulkAbandon — rejects empty ids with 400
 *   TC-DLQ-E011  BulkAbandon — marks multiple rows abandoned
 *   TC-DLQ-E012  Cleanup  — all test rows removed after tests
 *
 * Prerequisites:
 *   - Stack running: npm run dev:all (or docker-compose up)
 *   - DATABASE_URL set for seeding / cleanup
 *   - Admin credentials: ADMIN_EMAIL / ADMIN_PASSWORD (defaults: admin@ezhealthkonnect.com / admin123)
 *
 * Usage:
 *   node tests/dlq-e2e-test.js
 */

'use strict';

const http     = require('http');
const https    = require('https');
const { Client } = require('pg');

// ─── Colours ─────────────────────────────────────────────────────────────────
const GREEN  = '\x1b[32m';
const RED    = '\x1b[31m';
const YELLOW = '\x1b[33m';
const RESET  = '\x1b[0m';

let passed = 0;
let failed = 0;
let skipped = 0;

function pass(id, msg)          { console.log(`  ${GREEN}✓${RESET} [${id}] ${msg}`); passed++; }
function fail(id, msg, detail)  {
    console.log(`  ${RED}✗${RESET} [${id}] ${msg}`);
    if (detail) console.log(`       ${RED}${detail}${RESET}`);
    failed++;
}
function skip(id, msg)          { console.log(`  ${YELLOW}~${RESET} [${id}] ${msg} (skipped)`); skipped++; }
function assert(id, label, ok, detail) { ok ? pass(id, label) : fail(id, label, detail); }

// ─── Config ───────────────────────────────────────────────────────────────────
const BASE_URL    = process.env.BASE_URL  || `http://localhost:${process.env.PORT || 3000}`;
const DB_URL      = process.env.DATABASE_URL;
const ADMIN_EMAIL = process.env.ADMIN_EMAIL    || 'admin@ezhealthkonnect.com';
const ADMIN_PASS  = process.env.ADMIN_PASSWORD || 'admin123';

const TEST_INTERFACE_ID = '11111111-2222-3333-4444-555555555555';
const TEST_TAG          = `dlq-e2e-${Date.now()}`;

// ─── HTTP helper ──────────────────────────────────────────────────────────────
function request(method, path, { body, cookie, token } = {}) {
    return new Promise((resolve, reject) => {
        const url      = new URL(BASE_URL + path);
        const isHttps  = url.protocol === 'https:';
        const lib      = isHttps ? https : http;
        const bodyStr  = body ? JSON.stringify(body) : '';
        const headers  = { 'Content-Type': 'application/json' };
        if (bodyStr)   headers['Content-Length'] = Buffer.byteLength(bodyStr);
        if (cookie)    headers['Cookie'] = cookie;
        if (token)     headers['Authorization'] = 'Bearer ' + token;

        const req = lib.request({
            hostname: url.hostname,
            port:     url.port || (isHttps ? 443 : 80),
            path:     url.pathname + url.search,
            method,
            headers,
        }, (res) => {
            let data = '';
            res.on('data', chunk => { data += chunk; });
            res.on('end', () => {
                let json = null;
                try { json = JSON.parse(data); } catch (_) {}
                resolve({ status: res.statusCode, headers: res.headers, json, raw: data });
            });
        });
        req.on('error', reject);
        if (bodyStr) req.write(bodyStr);
        req.end();
    });
}

// ─── DB seeding helpers ───────────────────────────────────────────────────────
async function getDBClient() {
    if (!DB_URL) return null;
    const client = new Client({ connectionString: DB_URL });
    await client.connect();
    return client;
}

async function ensureTestInterface(client) {
    // Look up any existing user to satisfy the NOT NULL FK on user_id
    const userRow = await client.query(`SELECT id FROM users ORDER BY created_at LIMIT 1`);
    const userId = userRow.rows[0]?.id;
    await client.query(`
        INSERT INTO interfaces (id, user_id, name, source_type, target_type, message_type, status, created_at, updated_at)
        VALUES ($1, $2::uuid, 'E2E Test Interface (DLQ)', 'tcp_mllp', 'http_rest', 'ADT^A01', 'active', NOW(), NOW())
        ON CONFLICT (id) DO NOTHING`, [TEST_INTERFACE_ID, userId]);
}

async function seedDLQRow(client, { messageID, status = 'pending', pipelineID = null, failedStepID = null } = {}) {
    const msg = messageID || `${TEST_TAG}-${Math.random().toString(36).slice(2)}`;
    const result = await client.query(`
        INSERT INTO delivery_dlq
            (message_id, interface_id, connector_type, payload, content_type,
             error_message, attempt_count, next_retry_at, status, redrive_mode,
             pipeline_id, failed_step_id, step_name)
        VALUES ($1, $2::uuid, 'http_outbound', '{"test":true}', 'application/json',
                'connection refused', 1, NOW() - INTERVAL '1 second', $3,
                'from_failed_step', $4, $5, 'Outbound Delivery')
        RETURNING id`,
        [msg, TEST_INTERFACE_ID, status, pipelineID, failedStepID]);
    return result.rows[0].id;
}

async function cleanupTestRows(client) {
    // Remove rows inserted by this test run — identified by connector_type + interface_id pattern
    await client.query(`
        DELETE FROM delivery_dlq
        WHERE interface_id = $1::uuid
          AND connector_type = 'http_outbound'
          AND error_message = 'connection refused'`,
        [TEST_INTERFACE_ID]);
}

// ─── Main test runner ─────────────────────────────────────────────────────────
async function run() {
    console.log('\n─── DLQ API E2E Tests ───────────────────────────────────────────\n');
    console.log(`  Base URL : ${BASE_URL}`);
    console.log(`  Database : ${DB_URL ? 'connected' : 'NOT SET — seeding disabled'}\n`);

    // ── DB setup ──────────────────────────────────────────────────────────────
    const db = await getDBClient();
    if (db) {
        await ensureTestInterface(db);
    }

    let sessionCookie = '';
    let jwtToken = '';
    const seededIDs = [];

    try {
        // ─── TC-DLQ-E001: Auth ───────────────────────────────────────────────
        console.log('Group 1: Authentication');
        const loginResp = await request('POST', '/api/auth/login', {
            body: { email: ADMIN_EMAIL, password: ADMIN_PASS },
        });
        const setCookie = loginResp.headers['set-cookie'] || [];
        sessionCookie   = setCookie.map(c => c.split(';')[0]).join('; ');
        // DLQ routes use JWT Bearer auth (session doesn't reach /api/fhir/* proxy)
        jwtToken = (loginResp.json && (loginResp.json.token || loginResp.json.accessToken)) || '';

        assert('TC-DLQ-E001', 'Login returns 200', loginResp.status === 200,
            `status=${loginResp.status}, body=${loginResp.raw}`);
        assert('TC-DLQ-E001b', 'JWT token returned', jwtToken.length > 0,
            `login json: ${loginResp.raw.slice(0, 200)}`);

        if (!jwtToken) {
            fail('SETUP', 'No JWT token — cannot continue API tests');
            return;
        }

        // Seed test rows before exercising list/get/redrive
        if (db) {
            for (let i = 0; i < 3; i++) {
                const id = await seedDLQRow(db);
                seededIDs.push(id);
            }
        }

        // ─── TC-DLQ-E002: Stats ──────────────────────────────────────────────
        console.log('\nGroup 2: Stats');
        const statsResp = await request('GET', '/api/fhir/dlq/stats', { cookie: sessionCookie, token: jwtToken });
        assert('TC-DLQ-E002', 'Stats returns 200',
            statsResp.status === 200,
            `status=${statsResp.status}, body=${statsResp.raw}`);
        assert('TC-DLQ-E002b', 'Stats success:true',
            statsResp.json && statsResp.json.success === true,
            JSON.stringify(statsResp.json));
        assert('TC-DLQ-E002c', 'Stats data is object',
            statsResp.json && typeof statsResp.json.data === 'object',
            JSON.stringify(statsResp.json));

        // ─── TC-DLQ-E003: List ───────────────────────────────────────────────
        console.log('\nGroup 3: List');
        const listResp = await request(
            'GET', `/api/fhir/dlq?interface_id=${TEST_INTERFACE_ID}&status=pending`,
            { cookie: sessionCookie, token: jwtToken });
        assert('TC-DLQ-E003', 'List returns 200',
            listResp.status === 200,
            `status=${listResp.status}, body=${listResp.raw}`);
        assert('TC-DLQ-E003b', 'List success:true',
            listResp.json && listResp.json.success === true,
            JSON.stringify(listResp.json));
        assert('TC-DLQ-E003c', 'List data is array',
            listResp.json && Array.isArray(listResp.json.data),
            JSON.stringify(listResp.json));

        if (db && seededIDs.length > 0) {
            const data = listResp.json && listResp.json.data || [];
            const found = data.some(r => seededIDs.includes(r.ID || r.id));
            assert('TC-DLQ-E003d', 'Seeded rows visible in list',
                found, `seededIDs=${seededIDs}, returned=${data.map(r => r.ID || r.id)}`);
        } else {
            skip('TC-DLQ-E003d', 'No DB seeding — seeded row visibility not checked');
        }

        // ─── TC-DLQ-E004: Get (found) ────────────────────────────────────────
        console.log('\nGroup 4: Get');
        if (db && seededIDs.length > 0) {
            const getID = seededIDs[0];
            const getResp = await request('GET', `/api/fhir/dlq/${getID}`, { cookie: sessionCookie, token: jwtToken });
            assert('TC-DLQ-E004', `Get ${getID} returns 200`,
                getResp.status === 200,
                `status=${getResp.status}, body=${getResp.raw}`);
            assert('TC-DLQ-E004b', 'Get success:true',
                getResp.json && getResp.json.success === true,
                JSON.stringify(getResp.json));
            const rowID = getResp.json && getResp.json.data && (getResp.json.data.ID || getResp.json.data.id);
            assert('TC-DLQ-E004c', 'Get returns correct ID',
                rowID === getID, `returned ID=${rowID}, want=${getID}`);
        } else {
            skip('TC-DLQ-E004', 'No DB seeding — Get by ID not tested');
        }

        // ─── TC-DLQ-E005: Get (not found) ────────────────────────────────────
        const notFoundResp = await request(
            'GET', '/api/fhir/dlq/00000000-0000-0000-0000-000000000000',
            { cookie: sessionCookie, token: jwtToken });
        assert('TC-DLQ-E005', 'Get non-existent ID returns 404',
            notFoundResp.status === 404,
            `status=${notFoundResp.status}, body=${notFoundResp.raw}`);
        assert('TC-DLQ-E005b', 'Get not-found success:false',
            notFoundResp.json && notFoundResp.json.success === false,
            JSON.stringify(notFoundResp.json));

        // ─── TC-DLQ-E006: Redrive ────────────────────────────────────────────
        console.log('\nGroup 5: Redrive');
        if (db && seededIDs.length > 0) {
            const redriveID = seededIDs[1];
            const redriveResp = await request(
                'POST', `/api/fhir/dlq/${redriveID}/redrive`,
                { body: { mode: 'from_failed_step' }, cookie: sessionCookie, token: jwtToken });
            // Redrive may fail because no real pipeline is configured — 200 or 500 both
            // indicate the endpoint is reachable; the key is the response shape is correct.
            const redriveOk = redriveResp.status === 200 || redriveResp.status === 500;
            assert('TC-DLQ-E006', 'Redrive endpoint reachable (200 or 500)',
                redriveOk, `status=${redriveResp.status}`);
            assert('TC-DLQ-E006b', 'Redrive response has success field',
                redriveResp.json && 'success' in redriveResp.json,
                JSON.stringify(redriveResp.json));
        } else {
            skip('TC-DLQ-E006', 'No DB seeding — Redrive endpoint not tested');
        }

        // ─── TC-DLQ-E007: AbandonOne ─────────────────────────────────────────
        console.log('\nGroup 6: Abandon');
        if (db && seededIDs.length > 1) {
            // Seed a fresh row specifically to abandon
            const abandonID = await seedDLQRow(db);
            seededIDs.push(abandonID);

            const abandonResp = await request(
                'POST', `/api/fhir/dlq/${abandonID}/abandon`,
                { cookie: sessionCookie, token: jwtToken });
            assert('TC-DLQ-E007', 'AbandonOne returns 200',
                abandonResp.status === 200,
                `status=${abandonResp.status}, body=${abandonResp.raw}`);
            assert('TC-DLQ-E007b', 'AbandonOne success:true',
                abandonResp.json && abandonResp.json.success === true,
                JSON.stringify(abandonResp.json));

            // Verify DB status
            const dbCheck = await db.query(
                `SELECT status FROM delivery_dlq WHERE id = $1`, [abandonID]);
            const dbStatus = dbCheck.rows[0] && dbCheck.rows[0].status;
            assert('TC-DLQ-E007c', 'DB row status is abandoned',
                dbStatus === 'abandoned', `DB status=${dbStatus}`);
        } else {
            skip('TC-DLQ-E007', 'No DB seeding — AbandonOne not tested');
        }

        // ─── TC-DLQ-E008: BulkRedrive — empty ids ────────────────────────────
        console.log('\nGroup 7: Bulk operations');
        const bulkRedriveEmptyResp = await request(
            'POST', '/api/fhir/dlq/bulk-redrive',
            { body: { ids: [] }, cookie: sessionCookie, token: jwtToken });
        assert('TC-DLQ-E008', 'BulkRedrive empty ids returns 400',
            bulkRedriveEmptyResp.status === 400,
            `status=${bulkRedriveEmptyResp.status}, body=${bulkRedriveEmptyResp.raw}`);
        assert('TC-DLQ-E008b', 'BulkRedrive empty ids success:false',
            bulkRedriveEmptyResp.json && bulkRedriveEmptyResp.json.success === false,
            JSON.stringify(bulkRedriveEmptyResp.json));

        // ─── TC-DLQ-E009: BulkRedrive — valid ids ────────────────────────────
        if (db && seededIDs.length >= 2) {
            const bulkRedriveIDs = seededIDs.slice(0, 2);
            const bulkRedriveResp = await request(
                'POST', '/api/fhir/dlq/bulk-redrive',
                { body: { ids: bulkRedriveIDs, mode: 'from_failed_step' }, cookie: sessionCookie, token: jwtToken });
            assert('TC-DLQ-E009', 'BulkRedrive valid ids returns 200',
                bulkRedriveResp.status === 200,
                `status=${bulkRedriveResp.status}, body=${bulkRedriveResp.raw}`);
            const data = bulkRedriveResp.json && bulkRedriveResp.json.data;
            assert('TC-DLQ-E009b', 'BulkRedrive returns result per ID',
                Array.isArray(data) && data.length === bulkRedriveIDs.length,
                `data.length=${data ? data.length : null}, want ${bulkRedriveIDs.length}`);
        } else {
            skip('TC-DLQ-E009', 'No DB seeding — BulkRedrive valid ids not tested');
        }

        // ─── TC-DLQ-E010: BulkAbandon — empty ids ────────────────────────────
        const bulkAbandonEmptyResp = await request(
            'POST', '/api/fhir/dlq/bulk-abandon',
            { body: { ids: [] }, cookie: sessionCookie, token: jwtToken });
        assert('TC-DLQ-E010', 'BulkAbandon empty ids returns 400',
            bulkAbandonEmptyResp.status === 400,
            `status=${bulkAbandonEmptyResp.status}`);
        assert('TC-DLQ-E010b', 'BulkAbandon empty ids success:false',
            bulkAbandonEmptyResp.json && bulkAbandonEmptyResp.json.success === false,
            JSON.stringify(bulkAbandonEmptyResp.json));

        // ─── TC-DLQ-E011: BulkAbandon — valid ids ────────────────────────────
        if (db) {
            const bulkID1 = await seedDLQRow(db);
            const bulkID2 = await seedDLQRow(db);
            seededIDs.push(bulkID1, bulkID2);

            const bulkAbandonResp = await request(
                'POST', '/api/fhir/dlq/bulk-abandon',
                { body: { ids: [bulkID1, bulkID2] }, cookie: sessionCookie, token: jwtToken });
            assert('TC-DLQ-E011', 'BulkAbandon returns 200',
                bulkAbandonResp.status === 200,
                `status=${bulkAbandonResp.status}, body=${bulkAbandonResp.raw}`);
            assert('TC-DLQ-E011b', 'BulkAbandon affected = 2',
                bulkAbandonResp.json && bulkAbandonResp.json.affected === 2,
                `affected=${bulkAbandonResp.json && bulkAbandonResp.json.affected}`);
        } else {
            skip('TC-DLQ-E011', 'No DB seeding — BulkAbandon valid ids not tested');
        }

        // ─── TC-DLQ-E012: Cleanup ────────────────────────────────────────────
        console.log('\nGroup 8: Cleanup');
        if (db) {
            await cleanupTestRows(db);
            const remaining = await db.query(
                `SELECT COUNT(*) FROM delivery_dlq WHERE interface_id = $1::uuid AND connector_type = 'http_outbound' AND error_message = 'connection refused'`,
                [TEST_INTERFACE_ID]);
            const count = parseInt(remaining.rows[0].count, 10);
            assert('TC-DLQ-E012', 'All test rows removed after cleanup',
                count === 0, `${count} rows remain`);
        } else {
            skip('TC-DLQ-E012', 'No DB — cleanup not verified');
        }

    } catch (err) {
        console.error(`\n${RED}Unhandled error:${RESET}`, err.message);
        failed++;
    } finally {
        if (db) {
            try {
                await cleanupTestRows(db);
            } catch (_) {}
            await db.end();
        }
    }

    // ─── Summary ──────────────────────────────────────────────────────────────
    const total = passed + failed + skipped;
    const symbol = failed === 0 ? GREEN + '✓' + RESET : RED + '✗' + RESET;
    console.log(`\n${symbol} ${passed}/${total} passed, ${skipped} skipped, ${failed} failed`);
    if (failed > 0) process.exit(1);
}

run().catch(err => {
    console.error('Fatal:', err);
    process.exit(1);
});
