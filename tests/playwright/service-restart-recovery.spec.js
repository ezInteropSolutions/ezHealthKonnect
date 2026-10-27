'use strict';
/**
 * G6 (last part) — a message left stuck in "processing" is picked up again after the service restarts.
 *
 * processing/engine.go's recoverUnprocessedMessages scans every interface message table at startup for
 * messages still in 'received'/'processing' for more than 5 minutes, reads their raw content back from
 * object storage and re-enqueues them. This test makes that happen for real:
 *
 *   1. create an interface and send one message; wait until it is stored and processed;
 *   2. put the stored row back into "processing", received 10 minutes ago (what a crash mid-processing
 *      leaves behind — the raw content is already in storage);
 *   3. restart the service;
 *   4. wait: the engine must re-run the message and finish it, without creating a second row.
 *
 * It needs two things from the environment, so it skips (documented, not "no data") without them:
 *   DATABASE_URL     direct database access to plant the stuck state
 *   RESTART_COMMAND  a shell command that restarts the whole application and returns when issued, e.g.
 *                    Docker dev stack   docker restart ezhealthkonnect-app
 *                    Linux installer    sudo systemctl restart ezhealthkonnect-api ezhealthkonnect
 *                    Windows installer  powershell -Command "Restart-Service ezHealthKonnect*"
 * The same spec therefore runs unchanged in the installed-app job.
 *
 * Optional: RECOVERY_WAIT_SECONDS (default 150) — recovery waits ~10 s after start, then processes.
 */

const { test, expect } = require('./helpers/seeded');
const { sendMLLP, parseAckCode } = require('./helpers/mllp');
const { HL7_ADT_A01, hl7SinkPayload } = require('./helpers/fixtures');
const { Client } = require('pg');
const { exec } = require('child_process');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const DB_URL = process.env.DATABASE_URL;
const RESTART_COMMAND = process.env.RESTART_COMMAND;
const WAIT_MS = parseInt(process.env.RECOVERY_WAIT_SECONDS || '150', 10) * 1000;
const PORT = parseInt(process.env.RECOVERY_MLLP_PORT || '6640', 10); // inside the dev stack's published 6610-6670

const run = (command) => new Promise((resolve, reject) =>
    exec(command, { timeout: 120_000 }, (err, stdout, stderr) => (err ? reject(new Error(`${command}\n${stderr || err.message}`)) : resolve(stdout))));

async function waitForApp(request, timeoutMs = 180_000) {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
        const ok = await request.get(`${BASE_URL}/api/auth/session`).then(r => r.status() === 200).catch(() => false);
        if (ok) return;
        await new Promise(r => setTimeout(r, 3000));
    }
    throw new Error(`the application did not come back within ${timeoutMs / 1000}s of the restart`);
}

test.describe.serial('Startup recovery of a stuck message', () => {
    test.skip(!DB_URL || !RESTART_COMMAND, 'Needs DATABASE_URL and RESTART_COMMAND (environment, see the file header)');

    const state = { interfaceId: null, messageId: null, table: null };

    test.afterAll(async ({ seed }) => {
        if (state.interfaceId) await seed.api.deleteInterface(state.interfaceId).catch(() => {});
    });

    test('a message stuck in "processing" is re-run after a service restart', async ({ seed }) => {
        test.setTimeout(WAIT_MS + 240_000);
        const api = seed.api;

        // 1. A store-only interface, one message, processed.
        const created = await api.createInterface({
            ...hl7SinkPayload(PORT),
            name: `PW_RECOVERY_${Date.now()}`,
        });
        state.interfaceId = created.interfaceId || created.data?.interfaceId || created.id;
        expect(state.interfaceId).toBeTruthy();
        state.table = `messages_intf_${state.interfaceId.replace(/-/g, '_')}`;
        const activation = await api.activateInterface(state.interfaceId);
        expect(activation.warnings || [], 'activation warnings').toEqual([]);

        const ack = await sendMLLP('localhost', PORT, HL7_ADT_A01.replace('MVPTEST001', `REC${Date.now()}`), 15_000);
        expect(parseAckCode(ack)).toBe('AA');
        await expect.poll(async () => (await api.getMessages(state.interfaceId)).length, { timeout: 30_000 }).toBe(1);
        state.messageId = (await api.getMessages(state.interfaceId))[0].message_id;
        await expect.poll(async () => String((await api.getMessages(state.interfaceId))[0].status).toLowerCase(), { timeout: 30_000 })
            .toMatch(/^(processed|completed|delivered)$/);

        // 2. Put it back the way a crash mid-processing leaves it.
        const db = new Client({ connectionString: DB_URL });
        await db.connect();
        try {
            const upd = await db.query(
                `UPDATE ${state.table}
                    SET status = 'processing', delivery_status = NULL, processing_completed_at = NULL,
                        received_at = NOW() - INTERVAL '10 minutes'
                  WHERE message_id = $1 RETURNING status, raw_content_uri`, [state.messageId]);
            expect(upd.rowCount).toBe(1);
            expect(upd.rows[0].raw_content_uri, 'raw content must already be in storage for recovery').toBeTruthy();

            // 3. Restart the whole service.
            await run(RESTART_COMMAND);
            await new Promise(r => setTimeout(r, 5000)); // let it actually go down before polling for it to be up
            await waitForApp(seed.api.request);

            // 4. Recovery runs ~10 s after start; the message must be finished again, and still be one row.
            await expect.poll(async () => {
                const r = await db.query(`SELECT status FROM ${state.table} WHERE message_id = $1`, [state.messageId]);
                return r.rows[0]?.status;
            }, { timeout: WAIT_MS, intervals: [3000], message: 'the stuck message was not recovered after the restart' })
                .toMatch(/^(processed|completed|delivered)$/);

            const count = await db.query(`SELECT COUNT(*)::int AS n FROM ${state.table}`);
            expect(count.rows[0].n, 'recovery must re-run the message, not duplicate it').toBe(1);
        } finally {
            await db.end();
        }
    });
});
