'use strict';
/**
 * Seeded-data test fixture.
 *
 * A fresh database has no interfaces and no messages, so every UI spec that
 * needs "an interface" or "a message" used to skip itself. Specs that import
 * `test` from this module instead get a real, activated interface with real
 * messages, created through the same APIs the UI uses and removed afterwards.
 *
 *   const { test, expect } = require('./helpers/seeded');
 *   test('...', async ({ page, seed }) => { seed.interfaceId; seed.messageIds; ... });
 *
 * The fixture is worker-scoped and automatic: the interface is created once per
 * worker (workers = 1 in playwright.config.js, so once per run), shared by every
 * spec file that imports this module, and deleted when the worker ends.
 *
 * Environment:
 *   SEED_MLLP_PORT   first MLLP port to try (default 6613 — inside the Docker
 *                    dev stack's published 6610-6670 range). The next
 *                    SEED_PORT_ATTEMPTS-1 ports are tried when one is in use.
 */

const base = require('@playwright/test');
const path = require('path');
const { ApiHelper } = require('./api');
const { sendMLLP, parseAckCode } = require('./mllp');
const { HL7_ADT_A01, HL7_ADT_A03, HL7_ORU_R01, hl7SinkPayload } = require('./fixtures');

const AUTH_FILE      = path.resolve(__dirname, '../../../.auth/user.json');
const BASE_URL       = process.env.BASE_URL || 'http://localhost:3000';
const FIRST_PORT     = parseInt(process.env.SEED_MLLP_PORT || '6613', 10);
const PORT_ATTEMPTS  = parseInt(process.env.SEED_PORT_ATTEMPTS || '7', 10);
const NAME_PREFIX    = 'PW_SEED_';
const SEED_MESSAGES  = [
    { control: 'PWSEED001', hl7: HL7_ADT_A01 },
    { control: 'PWSEED002', hl7: HL7_ADT_A03 },
    { control: 'PWSEED003', hl7: HL7_ORU_R01 },
];

async function removeStaleSeedInterfaces(api) {
    // Left behind by a run that crashed before its teardown.
    const list = await api.listInterfaces().catch(() => []);
    for (const i of list) {
        if ((i.name || '').startsWith(NAME_PREFIX)) await api.deleteInterface(i.id).catch(() => {});
    }
}

async function waitForMessages(api, interfaceId, count, timeoutMs = 30_000) {
    const deadline = Date.now() + timeoutMs;
    let messages = [];
    while (Date.now() < deadline) {
        messages = await api.getMessages(interfaceId, 50).catch(() => []);
        if (messages.length >= count) return messages;
        await new Promise(r => setTimeout(r, 750));
    }
    throw new Error(`seed: expected ${count} stored messages, saw ${messages.length} after ${timeoutMs}ms`);
}

/** Creates + activates the interface on the first free port. Returns { id, port, name }. */
async function createActiveInterface(api) {
    const failures = [];
    for (let port = FIRST_PORT; port < FIRST_PORT + PORT_ATTEMPTS; port++) {
        const name = `${NAME_PREFIX}${port}_${Date.now()}`;
        const payload = { ...hl7SinkPayload(port), name, description: 'Playwright seeded interface' };
        const created = await api.createInterface(payload);
        const id = created.interfaceId || created.data?.interfaceId || created.id;
        if (!id) throw new Error(`seed: createInterface returned no id: ${JSON.stringify(created).slice(0, 200)}`);

        const activation = await api.activateInterface(id).catch(e => ({ error: e.message }));
        const warnings = activation.warnings || [];
        if (!activation.error && warnings.length === 0) return { id, port, name };

        failures.push(`port ${port}: ${activation.error || warnings.join('; ')}`);
        await api.deleteInterface(id).catch(() => {});
    }
    throw new Error(`seed: could not activate an MLLP interface on any port.\n${failures.join('\n')}`);
}

const test = base.test.extend({
    seed: [async ({ playwright }, use) => {
        const request = await playwright.request.newContext({ baseURL: BASE_URL, storageState: AUTH_FILE });
        const api = new ApiHelper(request, BASE_URL);

        await removeStaleSeedInterfaces(api);
        const { id, port, name } = await createActiveInterface(api);

        // A second, deliberately inactive interface so list/filter specs have two
        // interfaces in two different states.
        const idleName = `${NAME_PREFIX}idle_${Date.now()}`;
        const idleCreated = await api.createInterface({
            ...hl7SinkPayload(port + PORT_ATTEMPTS),
            name: idleName, description: 'Playwright seeded interface (inactive)', auto_start: false, status: 'draft',
        });
        const idleId = idleCreated.interfaceId || idleCreated.data?.interfaceId || idleCreated.id;
        await api.deactivateInterface(idleId).catch(() => {});

        try {
            for (const m of SEED_MESSAGES) {
                const ack = await sendMLLP('localhost', port, m.hl7, 15_000);
                if (parseAckCode(ack) !== 'AA') throw new Error(`seed: ${m.control} was not accepted (ACK ${parseAckCode(ack)})`);
            }
            const messages = await waitForMessages(api, id, SEED_MESSAGES.length);

            await use({
                interfaceId: id,
                interfaceName: name,
                idleInterfaceId: idleId,
                idleInterfaceName: idleName,
                port,
                controlIds: SEED_MESSAGES.map(m => m.control),
                messageIds: messages.map(m => m.id || m.message_id),
                /** Send one more real message through the seeded listener. */
                sendMessage: (hl7) => sendMLLP('localhost', port, hl7, 15_000),
                api,
            });
        } finally {
            await api.deleteInterface(id).catch(() => {});
            if (idleId) await api.deleteInterface(idleId).catch(() => {});
            await request.dispose();
        }
    }, { scope: 'worker', auto: true, timeout: 120_000 }],
});

module.exports = { test, expect: base.expect, NAME_PREFIX };
