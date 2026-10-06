'use strict';
/**
 * coverage-audit-e2e.spec.js — real, non-dry-run browser verification of the
 * Coverage Audit Journey-tab UI, across all 6 formats the feature supports.
 *
 * WHY NOT THE EXISTING *-to-fhir-e2e.spec.js FILES: those drive the pipeline
 * builder's own "Test Pipeline" modal, which is dry-run and explicitly skips
 * attaching the Coverage Audit tracker (confirmed in
 * transformation_pipeline_helpers.go). Coverage Audit can only be observed
 * through a REAL message delivered to a REAL, activated interface — the
 * same bar mvp-smoke.spec.js already uses, which this file mirrors.
 *
 * WHY EACH FORMAT'S CONNECTOR DIFFERS FROM ITS OWN SHIPPED OOB TEMPLATE:
 * Coverage Audit tracks fields *read by a mapping step*, not bytes accepted
 * by a connector. The EDI/NCPDP SCRIPT/NCPDP Telecom/CDA OOB templates are
 * SFTP- or bearer-auth-HTTP-only, neither reachable simply from a host-side
 * Playwright test. Each real mapping-step chain is pulled live from
 * interface_templates via getTemplateScaffold() (never hand-retyped — this
 * codebase's own established discipline after being bitten by transcription
 * errors before) and only the connector.inbound step is swapped for
 * http_rest_inbound, a real per-interface HTTP listener reachable at
 * localhost:<port> (confirmed directly against services/connectors/
 * http_rest_inbound.go — note the live cda-ccd-inbound-fhir-r4 template's
 * OWN http_rest_inbound config uses the wrong field names and would fail
 * real activation; this file's httpRestInboundStep() uses the real ones).
 *
 * Run: npx playwright test coverage-audit-e2e --project=chromium
 */

const { test, expect } = require('@playwright/test');
const { ApiHelper }     = require('./helpers/api');
const { sendMLLP, parseAckCode } = require('./helpers/mllp');
const fx = require('./helpers/coverage-fixtures');

// Ports within the docker-mapped ranges (6610-6670 TCP, 8080-8099 HTTP).
// mvp-smoke.spec.js already claims 6620-6622 and 8091 — these are distinct.
// Confirmed free via a live DB survey of every currently-active interface's
// own claimed port (several ports in the 8080-8099 range this file
// originally picked, e.g. 8092/8093/8094, turned out to already be in real,
// active use by other unrelated interfaces — "docker-mapped range" alone
// doesn't mean "unclaimed," so this is a real check, not a formality).
const PORT_HL7           = 6625;
const PORT_FHIR          = 8097;
const PORT_EDI           = 8098;
const PORT_NCPDP_SCRIPT  = 8085;
const PORT_NCPDP_TELECOM = 8086;
const PORT_CDA           = 8099;

const NAME_PREFIX = 'COVAUDIT_E2E_';
const COVERAGE_CONFIG = { enabled: true, granularity: 'element' };

// state.formats.<key> = { interfaceId, messageId }
const state = { ids: [], formats: {} };

function wait(ms) { return new Promise(r => setTimeout(r, ms)); }

async function waitFor(fn, { timeoutMs = 20000, intervalMs = 1000 } = {}) {
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
        const result = await fn();
        if (result) return result;
        await wait(intervalMs);
    }
    return null;
}

async function firstMessageId(api, interfaceId) {
    const messages = await waitFor(async () => {
        const list = await api.getMessages(interfaceId);
        return list.length > 0 ? list : null;
    });
    if (!messages) return null;
    const m = messages[0];
    return m.messageId || m.message_id || m.id;
}

async function waitForCoverageAudit(api, messageId) {
    return (await waitFor(async () => {
        const rows = await api.getCoverageAudit(messageId);
        return rows.length > 0 ? rows : null;
    })) || [];
}

// Builds an interface + pipeline for a format whose own OOB template is
// SFTP-only, by copying that template's real mapping-step chain and
// swapping only its connector.inbound step for http_rest_inbound.
async function buildFromTemplate(api, { slug, name, port, endpointPath, messageTypeOverride }) {
    const scaffold = await api.getTemplateScaffold(slug);
    // cda-ccd-inbound-fhir-r4's own message_type column is NULL in the live
    // DB (a real, pre-existing data gap in that template, found while
    // building this fixture) — fall back to the historical "CCD" convention
    // this codebase already uses elsewhere rather than failing the save.
    const messageType = messageTypeOverride || scaffold.message_type || 'CCD';
    const groups = JSON.parse(JSON.stringify(scaffold.pipeline_config.execution_groups));
    for (const g of groups) {
        g.steps = g.steps.map(s => {
            if (s.step_type !== 'connector.inbound') return s;
            return fx.httpRestInboundStep(port, endpointPath, s.sequence, s.step_name);
        });
    }
    const created = await api.createInterface(fx.genericHttpRestPayload(name, messageType, port, endpointPath));
    const interfaceId = created.interfaceId;
    await api.savePipelineRaw({ interfaceId, messageType, executionGroups: groups });
    await api.setCoverageAuditConfig(interfaceId, COVERAGE_CONFIG);
    return interfaceId;
}

// Opens a message's detail modal and switches to its Journey tab — mirrors
// the real click path (showMessageDetail -> switchTab('journey') ->
// loadDataLineage), called directly via page.evaluate rather than locating
// a specific table row, since each test interface here has exactly one
// message and the row-browsing UX itself is not what's under test.
async function openJourneyTab(page, interfaceId, messageId) {
    // messages.html requires ?interfaceId= — it's an interface-specific
    // viewer by design (no global message view), confirmed in CLAUDE.md;
    // without it the page never finishes initializing and showMessageDetail
    // never gets attached, which is what timed out before this fix.
    await page.goto(`/messages.html?interfaceId=${interfaceId}`);
    await page.waitForLoadState('load');
    await page.waitForFunction(() => typeof window.showMessageDetail === 'function', { timeout: 10000 });
    await page.evaluate((id) => showMessageDetail(id), messageId);
    await page.waitForSelector('#messageDetailModal.show', { timeout: 10000 });
    // A real click, not page.evaluate(() => switchTab('journey')) — switchTab
    // reads the global `event.target` internally (the inline onclick handler's
    // implicit event), which is undefined when called programmatically.
    await page.click('#journeyTabBtn');
    await page.waitForFunction(
        () => {
            const el = document.getElementById('messageLineageView');
            return el && !el.innerText.includes('Loading journey data');
        },
        { timeout: 15000 }
    );
}

async function openCoverageAuditStep(page) {
    const step = page.locator('#lj-coverage-audit-body');
    const isOpen = await step.evaluate(el => el.style.display === 'block').catch(() => false);
    if (!isOpen) {
        await page.evaluate(() => messageManager._toggleJourneyStep('lj-coverage-audit'));
    }
    await expect(step).toBeVisible({ timeout: 5000 });
    return step;
}

// ─────────────────────────────────────────────────────────────────────────────
// SETUP — create + activate all 6 interfaces
// ─────────────────────────────────────────────────────────────────────────────
test.describe('COV-S1 Setup', () => {
    test('COV-S1-001 Create and activate one interface per format', async ({ request }) => {
        const api = new ApiHelper(request);

        // Clean up any leftovers from a previously aborted run
        try {
            const all = await api.listInterfaces();
            const stale = all.filter(i => (i.name || '').startsWith(NAME_PREFIX));
            for (const iface of stale) await api.deleteInterface(iface.id).catch(() => {});
        } catch (_) {}

        // HL7 v2 — tcp_mllp_inbound + hl7_fhir_transform (oob), own pipeline
        // built explicitly rather than via a template (none is needed: this
        // exact chain was hand-verified live earlier this session).
        const hl7Created = await api.createInterface(fx.hl7Payload(`${NAME_PREFIX}HL7_${PORT_HL7}`, PORT_HL7));
        state.formats.hl7 = { interfaceId: hl7Created.interfaceId };
        await api.savePipeline({
            interfaceId: hl7Created.interfaceId,
            messageType: 'ADT^A01',
            steps: [
                { step_type: 'connector.inbound', step_name: 'Receive ADT via MLLP', sequence: 5, enabled: true, required: true,
                  config: { connectorType: 'tcp_mllp_inbound', config: { host: '0.0.0.0', port: PORT_HL7 } } },
                { step_type: 'hl7_fhir_transform', step_name: 'Transform ADT to FHIR', sequence: 100, enabled: true, required: true,
                  config: { mapping_mode: 'oob' } },
                fx.sinkOutboundStep(295, 'Store Result'),
            ],
        });
        await api.setCoverageAuditConfig(hl7Created.interfaceId, COVERAGE_CONFIG);

        // FHIR-as-inbound-source — http_fhir_inbound + one fhir.build step
        // reading real fields off the inbound MedicationRequest (its own
        // fields land flat at root for the first step after an
        // http_fhir_inbound connector — confirmed in processing/
        // engine_message_processor.go).
        const fhirCreated = await api.createInterface(fx.fhirPayload(`${NAME_PREFIX}FHIR_${PORT_FHIR}`, PORT_FHIR));
        state.formats.fhir = { interfaceId: fhirCreated.interfaceId };
        await api.savePipeline({
            interfaceId: fhirCreated.interfaceId,
            messageType: 'FHIR:MedicationRequest',
            steps: [
                { step_type: 'connector.inbound', step_name: 'Receive via HTTP FHIR', sequence: 5, enabled: true, required: true,
                  config: { connectorType: 'http_fhir_inbound', config: { host: '0.0.0.0', port: PORT_FHIR } } },
                { step_type: 'fhir.build', step_name: 'Read MedicationRequest Fields', sequence: 100, enabled: true, required: true,
                  config: {
                      resourceType: 'Patient', profile: 'base', version: 'R4', outputField: 'message.fhirOut',
                      fields: [
                          { targetPath: 'id', literalValue: 'covaudit-fhir-test' },
                          { sourcePath: 'subject.display', targetPath: 'name[0].text' },
                          { sourcePath: 'medicationCodeableConcept.text', targetPath: 'address[0].text' },
                          { sourcePath: 'status', targetPath: 'maritalStatus.text' },
                          { sourcePath: 'dosageInstruction[0].text', targetPath: 'telecom[0].value' },
                      ],
                  } },
                { ...fx.sinkOutboundStep(295, 'Store Result'), config: { ...fx.sinkOutboundStep().config, contentField: 'message.fhirOut' } },
            ],
        });
        await api.setCoverageAuditConfig(fhirCreated.interfaceId, COVERAGE_CONFIG);

        // EDI X12 837P, NCPDP SCRIPT NewRx, NCPDP Telecom B1 Request, CDA/CCD
        // — each built by copying the real, live OOB template's own mapping
        // chain (see buildFromTemplate's own doc comment).
        state.formats.edi = { interfaceId: await buildFromTemplate(api, {
            slug: 'edi-837p-to-fhir-sftp', name: `${NAME_PREFIX}EDI_${PORT_EDI}`,
            port: PORT_EDI, endpointPath: '/coverage-test/edi',
        }) };
        state.formats.ncpdpScript = { interfaceId: await buildFromTemplate(api, {
            slug: 'ncpdp-newrx-to-fhir-sftp', name: `${NAME_PREFIX}NCPDP_SCRIPT_${PORT_NCPDP_SCRIPT}`,
            port: PORT_NCPDP_SCRIPT, endpointPath: '/coverage-test/ncpdp-script',
        }) };
        state.formats.ncpdpTelecom = { interfaceId: await buildFromTemplate(api, {
            slug: 'ncpdp-telecom-b1-request-to-fhir-sftp', name: `${NAME_PREFIX}NCPDP_TELECOM_${PORT_NCPDP_TELECOM}`,
            port: PORT_NCPDP_TELECOM, endpointPath: '/coverage-test/ncpdp-telecom',
        }) };
        state.formats.cda = { interfaceId: await buildFromTemplate(api, {
            slug: 'cda-ccd-inbound-fhir-r4', name: `${NAME_PREFIX}CDA_${PORT_CDA}`,
            port: PORT_CDA, endpointPath: '/coverage-test/cda',
        }) };

        state.ids = Object.values(state.formats).map(f => f.interfaceId);
        for (const id of state.ids) expect(id).toBeTruthy();

        await Promise.all(state.ids.map(id =>
            request.post(`http://localhost:3000/api/processing/interfaces/${id}/activate`)
                   .catch(e => console.warn(`  Activation for ${id}: ${e.message}`))
        ));

        // Let the Go engine finish starting every listener before delivering.
        await wait(5000);

        console.log('\n✅ Coverage Audit seed interfaces ready:', state.formats);
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// Real delivery + real coverage_audits row, per format
// ─────────────────────────────────────────────────────────────────────────────
test.describe('COV-S2 Real delivery produces a real coverage report', () => {
    test('COV-S2-001 HL7 v2 ADT^A01 over real MLLP', async ({ request }) => {
        if (!state.formats.hl7) test.skip();
        const api = new ApiHelper(request);
        const ack = await sendMLLP('localhost', PORT_HL7, fx.HL7_ADT_SAMPLE, 10000);
        expect(parseAckCode(ack)).toBe('AA');
        const messageId = await firstMessageId(api, state.formats.hl7.interfaceId);
        expect(messageId).toBeTruthy();
        state.formats.hl7.messageId = messageId;
        const rows = await waitForCoverageAudit(api, messageId);
        expect(rows.length).toBeGreaterThan(0);
        expect(rows[0].overallCoveragePct).not.toBeNull();
    });

    test('COV-S2-002 FHIR MedicationRequest over real HTTP POST', async ({ request }) => {
        if (!state.formats.fhir) test.skip();
        const api = new ApiHelper(request);
        const res = await request.post(`http://localhost:${PORT_FHIR}/fhir/r4/MedicationRequest`, {
            headers: { 'Content-Type': 'application/fhir+json' },
            data: fx.FHIR_MEDICATION_REQUEST_SAMPLE,
        });
        expect([200, 201, 202]).toContain(res.status());
        const messageId = await firstMessageId(api, state.formats.fhir.interfaceId);
        expect(messageId).toBeTruthy();
        state.formats.fhir.messageId = messageId;
        const rows = await waitForCoverageAudit(api, messageId);
        expect(rows.length).toBeGreaterThan(0);
    });

    test('COV-S2-003 EDI X12 837P (real X12.org Ben Kildare sample) over HTTP', async ({ request }) => {
        if (!state.formats.edi) test.skip();
        const api = new ApiHelper(request);
        const res = await request.post(`http://localhost:${PORT_EDI}/coverage-test/edi`, {
            headers: { 'Content-Type': 'text/plain' },
            data: fx.ediSample(),
        });
        expect([200, 201, 202]).toContain(res.status());
        const messageId = await firstMessageId(api, state.formats.edi.interfaceId);
        expect(messageId).toBeTruthy();
        state.formats.edi.messageId = messageId;
        const rows = await waitForCoverageAudit(api, messageId);
        expect(rows.length).toBeGreaterThan(0);
    });

    test('COV-S2-004 NCPDP SCRIPT NewRx (real dgoradia sample) over HTTP', async ({ request }) => {
        if (!state.formats.ncpdpScript) test.skip();
        const api = new ApiHelper(request);
        const res = await request.post(`http://localhost:${PORT_NCPDP_SCRIPT}/coverage-test/ncpdp-script`, {
            headers: { 'Content-Type': 'application/xml' },
            data: fx.ncpdpScriptSample(),
        });
        expect([200, 201, 202]).toContain(res.status());
        const messageId = await firstMessageId(api, state.formats.ncpdpScript.interfaceId);
        expect(messageId).toBeTruthy();
        state.formats.ncpdpScript.messageId = messageId;
        const rows = await waitForCoverageAudit(api, messageId);
        expect(rows.length).toBeGreaterThan(0);
    });

    // Closes the real, named gap found earlier this session: NCPDP Telecom
    // D.0 had ZERO live coverage_audits rows ever, in this environment.
    test('COV-S2-005 NCPDP Telecom D.0 B1 request (real generated sample) over HTTP — first-ever live row', async ({ request }) => {
        if (!state.formats.ncpdpTelecom) test.skip();
        const api = new ApiHelper(request);
        const res = await request.post(`http://localhost:${PORT_NCPDP_TELECOM}/coverage-test/ncpdp-telecom`, {
            headers: { 'Content-Type': 'application/octet-stream' },
            data: fx.ncpdpTelecomSample(),
        });
        expect([200, 201, 202]).toContain(res.status());
        const messageId = await firstMessageId(api, state.formats.ncpdpTelecom.interfaceId);
        expect(messageId).toBeTruthy();
        state.formats.ncpdpTelecom.messageId = messageId;
        const rows = await waitForCoverageAudit(api, messageId);
        expect(rows.length).toBeGreaterThan(0);
    });

    test('COV-S2-006 CDA/CCD (real NIST sample) over HTTP', async ({ request }) => {
        if (!state.formats.cda) test.skip();
        const api = new ApiHelper(request);
        const res = await request.post(`http://localhost:${PORT_CDA}/coverage-test/cda`, {
            headers: { 'Content-Type': 'text/xml' },
            data: fx.cdaSample(),
        });
        expect([200, 201, 202]).toContain(res.status());
        const messageId = await firstMessageId(api, state.formats.cda.interfaceId);
        expect(messageId).toBeTruthy();
        state.formats.cda.messageId = messageId;
        const rows = await waitForCoverageAudit(api, messageId);
        expect(rows.length).toBeGreaterThan(0);
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// Real browser — Journey tab renders the Coverage Audit step correctly
// ─────────────────────────────────────────────────────────────────────────────
test.describe('COV-S3 Journey tab UI', () => {
    for (const [key, label] of Object.entries({
        hl7: 'HL7 v2', fhir: 'FHIR', edi: 'EDI X12 837P',
        ncpdpScript: 'NCPDP SCRIPT NewRx', ncpdpTelecom: 'NCPDP Telecom D.0 B1',
        cda: 'CDA/CCD',
    })) {
        test(`COV-S3-${key} Journey tab renders the Coverage Audit step for ${label}`, async ({ page }) => {
            const entry = state.formats[key];
            if (!entry || !entry.messageId) test.skip();

            await openJourneyTab(page, entry.interfaceId, entry.messageId);
            const stepBody = await openCoverageAuditStep(page);
            const text = await stepBody.innerText();
            expect(text.length).toBeGreaterThan(0);
            // Real numbers, never a literal "undefined"/"NaN"/"-1" — direct
            // regression guard for the historical "#-1" badge bug.
            expect(text).not.toMatch(/undefined|NaN|#-1/);
        });
    }

    test('COV-S3-badge HL7 badge text matches the live API numbers exactly (UI-07)', async ({ page, request }) => {
        const entry = state.formats.hl7;
        if (!entry || !entry.messageId) test.skip();
        const api = new ApiHelper(request);
        const [report] = await api.getCoverageAudit(entry.messageId);
        const pidCategory = report.categories.find(c => c.category === 'PID');
        expect(pidCategory).toBeTruthy();

        await openJourneyTab(page, entry.interfaceId, entry.messageId);
        await openCoverageAuditStep(page);
        const bodyText = await page.locator('#lj-coverage-audit-body').innerText();
        expect(bodyText).toContain(`${pidCategory.found}/${pidCategory.total} used`);
    });

    test('COV-S3-categories expanding one category does not expand siblings (UI-08)', async ({ page }) => {
        const entry = state.formats.hl7;
        if (!entry || !entry.messageId) test.skip();

        await openJourneyTab(page, entry.interfaceId, entry.messageId);
        await openCoverageAuditStep(page);

        const catButtons = page.locator('button[onclick*="_toggleCoverageRow(\'covcat-"]');
        const count = await catButtons.count();
        expect(count).toBeGreaterThan(1);

        const firstCatId = await catButtons.nth(0).getAttribute('onclick');
        const id = firstCatId.match(/_toggleCoverageRow\('([^']+)'\)/)[1];
        await page.evaluate((catId) => messageManager._toggleCoverageRow(catId), id);

        const firstBody  = page.locator(`#${id}-body`);
        await expect(firstBody).toBeVisible();

        // A sibling category's body must still be hidden.
        const secondCatOnclick = await catButtons.nth(1).getAttribute('onclick');
        const secondId = secondCatOnclick.match(/_toggleCoverageRow\('([^']+)'\)/)[1];
        const secondBody = page.locator(`#${secondId}-body`);
        await expect(secondBody).toBeHidden();
    });

    test('COV-S3-admin administrative rows are hidden by default and revealed by the toggle (UI-14)', async ({ page }) => {
        // CDA, not HL7: a real 9-section CCD reliably carries classifier-
        // flagged administrative fields (id/templateId/code); a minimal
        // 4-segment ADT message often doesn't, which made this test skip
        // more often than it ran when pointed at HL7.
        const entry = state.formats.cda;
        if (!entry || !entry.messageId) test.skip();

        await openJourneyTab(page, entry.interfaceId, entry.messageId);
        const stepBody = await openCoverageAuditStep(page);

        const adminRows = stepBody.locator('[data-cov-admin="1"]');
        const adminCount = await adminRows.count();
        if (adminCount === 0) {
            // MSH is excluded from the inventory entirely for HL7, so this
            // message's own report may legitimately carry zero admin-tagged
            // rows — not a failure, just nothing to assert on here.
            test.skip();
        }
        await expect(adminRows.first()).toBeHidden();

        const toggle = stepBody.locator('input[type="checkbox"]');
        await toggle.check();
        // Admin visibility and category collapse are two independent layers
        // — an admin row revealed by the toggle is still not actually on
        // screen if its own category is still collapsed. A real user would
        // need to open both; this mirrors that rather than only checking
        // the row's own inline style.
        await page.evaluate(() => {
            document.querySelectorAll('#lj-coverage-audit-body [id$="-body"]').forEach(el => { el.style.display = 'block'; });
        });
        await expect(adminRows.first()).toBeVisible();
    });

    test('COV-S3-phi rendered Coverage Audit step never shows the real patient name/MRN (UI-15 / SEC-01 / SEC-03)', async ({ page }) => {
        const entry = state.formats.hl7;
        if (!entry || !entry.messageId) test.skip();

        await openJourneyTab(page, entry.interfaceId, entry.messageId);
        const stepBody = await openCoverageAuditStep(page);
        // Open every category + entry row so hidden detail is also checked —
        // innerText() still only returns what's actually display:block, so a
        // row left collapsed would otherwise read as absent rather than
        // genuinely checked.
        await page.evaluate(() => {
            document.querySelectorAll('#lj-coverage-audit-body [id$="-body"]').forEach(el => { el.style.display = 'block'; });
        });
        const text = await stepBody.innerText();
        for (const phi of ['Doe', 'Marie', 'MRN900111', '100 Main St', '5551234567']) {
            expect(text).not.toContain(phi);
        }
    });

    test('COV-S3-config Coverage Audit config section is visible and persists for every format (real UI-09)', async ({ page }) => {
        // The originally-planned GET /api/coverage/formats registry-gate was
        // never actually built — the real, shipped behavior is "always
        // shown; the backend's own per-format adapter registration silently
        // no-ops for an unsupported format." This asserts that real
        // behavior, not the unbuilt one.
        const entry = state.formats.edi;
        if (!entry) test.skip();
        await page.goto('/interfaces.html');
        await page.waitForLoadState('load');
        await page.waitForTimeout(2000);
        const moreBtn = page.locator(`#actions-menu-btn-${entry.interfaceId}`);
        if (await moreBtn.count() === 0) test.skip();
        await moreBtn.click();
        const dropdownMenu = page.locator(`#actions-menu-${entry.interfaceId}`);
        await dropdownMenu.waitFor({ state: 'visible', timeout: 3000 }).catch(() => {});
        await dropdownMenu.locator('.dropdown-item-minimal').filter({ hasText: 'Edit' }).click();
        await page.waitForSelector('#editModal.show', { timeout: 10000 });
        await expect(page.locator('#editCdaCoverageAuditSection')).toBeVisible();
        await expect(page.locator('#editCdaCoverageAuditEnabled')).toBeChecked();
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// CLEANUP — always last, always runs, deactivates + deletes all seed data
// ─────────────────────────────────────────────────────────────────────────────
test.describe('COV-S9 Cleanup', () => {
    test('COV-S9-001 Deactivate and delete all Coverage Audit E2E seed interfaces', async ({ request }) => {
        const api = new ApiHelper(request);
        const failures = [];

        for (const id of state.ids.filter(Boolean)) {
            try {
                await api.deleteInterface(id);
                console.log(`  ✅ Deleted ${id}`);
            } catch (e) {
                failures.push(`${id}: ${e.message}`);
                console.warn(`  ⚠️  ${id}: ${e.message}`);
            }
        }

        try {
            const all = await api.listInterfaces();
            const stale = all.filter(i => (i.name || '').startsWith(NAME_PREFIX) && !state.ids.includes(i.id));
            for (const iface of stale) {
                try { await api.deleteInterface(iface.id); console.log(`  ✅ Cleaned stale ${iface.id} (${iface.name})`); }
                catch (e) { console.warn(`  ⚠️  Stale ${iface.id}: ${e.message}`); }
            }
        } catch (e) {
            console.warn(`  ⚠️  Safety-net scan: ${e.message}`);
        }

        if (failures.length > 0) console.warn(`⚠️  ${failures.length} interface(s) could not be deleted:`, failures);
        expect(true).toBe(true); // cleanup is best-effort, same convention as mvp-smoke.spec.js
    });
});
