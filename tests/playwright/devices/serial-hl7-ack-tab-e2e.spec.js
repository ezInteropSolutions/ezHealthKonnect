'use strict';
/**
 * serial-hl7-ack-tab-e2e.spec.js — the RS232 serial connector's Protocol
 * setting (V281) in the real pipeline builder.
 *
 * serial_inbound speaks ASTM (default) or HL7 v2 with MLLP framing. Only the
 * HL7 mode answers with an HL7 ACK, so the connector's Acknowledgment tab must
 * follow the Protocol field live, and the Host Query tab (TCP-only) must never
 * appear for a serial connector. Also checks the "HL7 over Serial Port"
 * template is listed in the device gallery data.
 *
 * Run: npx playwright test serial-hl7-ack-tab-e2e --project=chromium
 */

const { test, expect } = require('@playwright/test');
const { ApiHelper }     = require('../helpers/api');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const NAME = 'Serial HL7 ACK Tab UI Test';
const state = { interfaceId: null };

test.describe.serial('Serial connector protocol drives the Acknowledgment tab', () => {
    test.describe.configure({ retries: 0 });

    test.beforeAll(async ({ request }) => {
        const api = new ApiHelper(request, BASE_URL);
        for (const iface of await api.listInterfaces()) {
            if (iface.name === NAME) await api.deleteInterface(iface.id).catch(() => {});
        }
        const serialConfig = { port_name: '/dev/ttyUSB9', baud_rate: 9600, protocol: 'hl7_mllp' };
        const created = await api.createInterface({
            name: NAME,
            description: 'Throwaway interface for the serial protocol / ACK tab UI test.',
            sourceType: 'hl7v2',
            sourceConnectivity: 'serial_inbound',
            sourceConfig: serialConfig,
            sourceConnectorConfig: { connectorType: 'serial_inbound', config: serialConfig },
            targetType: 'file',
            targetConnectivity: 'file_writer',
            targetConfig: { outputPath: '/tmp/serial-hl7-ui/{timestamp}.hl7', createDirs: true },
            targetConnectorConfig: { connectorType: 'file_writer', config: { outputPath: '/tmp/serial-hl7-ui/{timestamp}.hl7', createDirs: true } },
            messageType: 'ORU^R01',
            mappings: [],
            transformationFlow: 'hl7_passthrough',
            auto_start: false,
            deployment_mode: 'manual',
            status: 'active',
        });
        state.interfaceId = created.interface?.id || created.interfaceId || created.id;
        expect(state.interfaceId, `createInterface: ${JSON.stringify(created)}`).toBeTruthy();

        await api.savePipeline({
            interfaceId: state.interfaceId,
            messageType: 'ORU^R01',
            steps: [{
                step_name: 'Serial Inbound (HL7 v2)',
                step_type: 'connector.inbound',
                sequence: 10,
                enabled: true,
                config: { connectorType: 'serial_inbound', config: serialConfig, timeoutMs: 30000 },
            }],
        });
    });

    test.afterAll(async ({ request }) => {
        if (!state.interfaceId) return;
        await new ApiHelper(request, BASE_URL).deleteInterface(state.interfaceId).catch(() => {});
    });

    test('Acknowledgment tab follows the Protocol field; Host Query never shows', async ({ page }) => {
        test.skip(!state.interfaceId, 'Setup failed');
        const errors = [];
        page.on('pageerror', e => errors.push(e.message));

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });
        const opened = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const s = (pb.getAllStepsFlat?.() || []).find(st => (st.stepType || st.step_type) === 'connector.inbound');
            if (!s) return false;
            pb.propertiesPanel.showStepProperties(s);
            return true;
        });
        expect(opened).toBe(true);
        await expect(page.locator('#stepPropertiesModal')).toBeVisible({ timeout: 3000 });

        const protocol = page.locator('.connector-config-field[data-field="protocol"]');
        await expect(protocol).toHaveValue('hl7_mllp', { timeout: 5000 });
        await expect(page.locator('#ackTabBtn')).toBeVisible();
        await expect(page.locator('#hostQueryTabBtn')).toBeHidden();

        await protocol.selectOption('astm');
        await expect(page.locator('#ackTabBtn')).toBeHidden();

        await protocol.selectOption('hl7_mllp');
        await expect(page.locator('#ackTabBtn')).toBeVisible();
        await expect(page.locator('#hostQueryTabBtn')).toBeHidden();

        expect(errors, 'no page errors').toEqual([]);
    });

    test('"HL7 over Serial Port" is a device template', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/interface-templates?category=device`);
        expect(res.ok()).toBe(true);
        const body = await res.json();
        const list = body.templates || body.data || body;
        const tpl = list.find(t => t.slug === 'device-hl7-serial');
        expect(tpl, 'device-hl7-serial listed').toBeTruthy();
        expect(tpl.source_connector_type).toBe('serial_inbound');
    });
});
