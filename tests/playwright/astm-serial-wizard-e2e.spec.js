// tests/playwright/astm-serial-wizard-e2e.spec.js
//
// The "Connect a Device" guided wizard's new "ASTM over Serial Port"
// template (V275__Add_Serial_Connectivity_Types_And_ASTM_Template.sql),
// driven through the real "Use Template" click path — same discipline as
// device-connect-wizard-e2e.spec.js's own precedent for the HL7-over-TCP/IP
// template.
//
// This is also the permanent regression guard for the new "select" field
// type _renderRequiredFieldsSection/_resolveDynamicFieldOptions added this
// round (public/js/dashboard.js) — the first required_connection_fields type
// beyond string/number/password/url/boolean/textarea, and the first one
// backed by a LIVE backend call (GET /api/connectivity/serial-ports).
//
// Honest limitation, stated up front: no real or virtual serial port exists
// in this test environment, so this spec cannot prove a real ASTM message
// round-trips through a live serial_inbound connector the way
// device-connect-wizard-e2e.spec.js proves a real TCP message does for
// tcp_mllp_inbound. That deeper correctness proof already exists at the Go
// level (astm/engine_test.go's parse/build round-trip tests,
// services/connectors/astm_framing_test.go's real ENQ/ACK/STX/ETX/checksum
// handshake tests via net.Pipe) — this spec's own job is narrower: prove the
// wizard UI correctly collects a select-type field (including a live,
// dynamically-fetched one with zero options available, the realistic
// container/CI case) and gets it to a real created interface + pipeline.
const { test, expect } = require('@playwright/test');

const TEMPLATE_NAME = 'ASTM over Serial Port';

async function cleanupInterfacesByName(request, name) {
    const res = await request.get('/api/interfaces');
    const body = await res.json();
    const list = body.data || body.interfaces || body || [];
    for (const iface of list) {
        if (iface.name === name) {
            await request.post(`/api/runtime/interfaces/${iface.id}/deactivate`).catch(() => {});
            await request.delete(`/api/interfaces/${iface.id}`).catch(() => {});
        }
    }
}

async function openTemplateModal(page) {
    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-cat-tab', { timeout: 15000 });
    await page.locator('.tg-cat-tab', { hasText: 'Devices' }).click();

    const card = page.locator('.tg-card', { hasText: TEMPLATE_NAME });
    await expect(card, `${TEMPLATE_NAME} card should be visible under Devices`).toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();
    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
}

test.describe.serial('Connect a Device wizard ("ASTM over Serial Port" template)', () => {
    test.describe.configure({ retries: 0 });

    test('Devices tab lists the ASTM over Serial Port template alongside HL7 over TCP/IP', async ({ page }) => {
        await page.goto('/dashboard.html');
        await page.click('a[href="#templates"]');
        await page.waitForSelector('.tg-cat-tab', { timeout: 15000 });
        await page.locator('.tg-cat-tab', { hasText: 'Devices' }).click();

        await expect(page.locator('.tg-card', { hasText: 'HL7 over TCP/IP' }), 'HL7 template should still be present').toBeVisible({ timeout: 5000 });
        await expect(page.locator('.tg-card', { hasText: TEMPLATE_NAME }), 'ASTM template should be present').toBeVisible({ timeout: 5000 });
    });

    test('Use renders a real COM Port select (live-fetched) and a Baud Rate select with real options', async ({ page }) => {
        // Listen for the live serial-ports fetch to prove the dynamic
        // optionsEndpoint mechanism actually made a real network call,
        // not just that a <select> happened to appear.
        let serialPortsRequested = false;
        page.on('request', (req) => {
            if (req.url().includes('/api/connectivity/serial-ports')) serialPortsRequested = true;
        });

        await openTemplateModal(page);

        expect(serialPortsRequested, 'opening the modal should trigger a live GET /api/connectivity/serial-ports fetch').toBe(true);

        const portSelect = page.locator('select[data-tcf-field="port_name"][data-tcf-section="source"]');
        await expect(portSelect, 'a real COM Port select should render').toBeVisible({ timeout: 3000 });

        // No real/virtual COM port exists in this test environment — the
        // honest, expected state is "no ports detected", not a crash or a
        // missing control.
        await expect(portSelect.locator('option', { hasText: /No ports detected|— Select —/ })).toHaveCount(2);

        const baudSelect = page.locator('select[data-tcf-field="baud_rate"][data-tcf-section="source"]');
        await expect(baudSelect, 'a Baud Rate select with static options should render').toBeVisible({ timeout: 3000 });
        await expect(baudSelect.locator('option[value="9600"]')).toHaveCount(1);
        await expect(baudSelect.locator('option[value="19200"]')).toHaveCount(1);
        await expect(baudSelect.locator('option[value="115200"]')).toHaveCount(1);

        await page.locator('#tgConfigureModal button:has-text("Cancel")').click();
    });

    test('Submitting with a selected baud rate and no detected COM port still creates a real interface + pipeline with the collected value', async ({ page, request }) => {
        const ifaceName = `PW ASTM Serial Wizard ${Date.now()}`;

        await cleanupInterfacesByName(request, ifaceName);
        await openTemplateModal(page);

        await page.fill('#tcf_name', ifaceName);
        await page.selectOption('select[data-tcf-field="baud_rate"][data-tcf-section="source"]', '19200');
        await page.fill('[data-tcf-field="endpoint"][data-tcf-section="target"]', 'https://his.example.org/api/results');

        await page.click('#tcf_submit');

        // With no real COM port selected, activation is expected to fail
        // gracefully (serial_inbound.Validate() requires port_name) — the
        // wizard's own documented "non-fatal" treatment means this redirects
        // straight into Pipeline Builder rather than showing the device
        // verification step (which only appears after a SUCCESSFUL
        // activation). Either outcome is accepted here; what matters is that
        // creation itself succeeds and the collected baud_rate reaches the
        // real saved pipeline.
        await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 20000 });

        const url = new URL(page.url());
        const interfaceId = url.searchParams.get('interfaceId');
        expect(interfaceId, 'should have redirected with a real interfaceId').toBeTruthy();

        // GET /pipelines/interface/:interfaceId (no messageType) hits the
        // LIST endpoint ({success, pipelines:[...]}, each row's own
        // pipeline_config un-flattened) — the real per-interface detail
        // endpoint needs the message type too (pipelineController.js's
        // loadPipelineByInterface), which this template's own migration
        // sets to the literal string "ASTM".
        const pipelineRes = await request.get(`/api/pipelines/interface/${interfaceId}/ASTM`);
        const pipelineData = await pipelineRes.json();
        const pipeline = pipelineData.pipeline;
        expect(pipeline, 'the pipeline should have been saved for message_type=ASTM').toBeTruthy();
        const allSteps = (pipeline.execution_groups || []).flatMap(g => g.steps || []);
        const inboundStep = allSteps.find(s => (s.step_type || s.type) === 'connector.inbound');

        expect(inboundStep, 'the saved pipeline should have a connector.inbound step').toBeTruthy();
        expect(inboundStep.config?.connectorType).toBe('serial_inbound');
        // The real regression guard: baud_rate (a "select" field) must have
        // actually reached the pipeline's own connector config, the same
        // way port (a plain number field) already does for the HL7 template.
        expect(String(inboundStep.config?.config?.baud_rate)).toBe('19200');

        await cleanupInterfacesByName(request, ifaceName);
    });
});
