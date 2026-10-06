'use strict';
/**
 * tcp-mllp-hostquery-e2e.spec.js — real-browser validation of the new
 * "Host Query" tab on ConnectorConfigBuilder (tcp_mllp_inbound only).
 *
 * Backs a real, new capability: TCPMLLPInboundConnector can answer a live
 * HL7 host-query (e.g. Mindray lab analyzer QRY^Q02) synchronously on the
 * same connection with a real pipeline-built reply (e.g. DSR^Q03), instead
 * of only the fixed MSA ack. See services/connectors/tcp_mllp_inbound.go's
 * own HostQueryConfig/handleHostQuery, and public/js/pipeline/components/
 * ConnectorConfigBuilder.js's _buildHostQueryPanel.
 *
 * This spec creates one small, throwaway interface via the real wizard API
 * (creation itself isn't what's being tested), drives the real
 * pipeline-builder UI in an actual browser to open the connector.inbound
 * step's real Properties Panel, and verifies:
 *   - the Host Query tab is visible for tcp_mllp_inbound (and hidden for a
 *     non-MLLP connector type)
 *   - the panel's fields render with the right defaults
 *   - editing + collecting the config (getConfig()) produces a real
 *     host_query block, and toggling "enabled" off omits it entirely
 *
 * Run: npx playwright test tcp-mllp-hostquery-e2e --project=chromium
 */

const { test, expect } = require('@playwright/test');
const { ApiHelper }     = require('./helpers/api');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const PORT_HOSTQUERY_DEMO = 6640; // tcp_mllp, within the 6610-6670 docker-mapped range

const state = { interfaceId: null };

test.describe.serial('TCP/MLLP Host Query tab (ConnectorConfigBuilder)', () => {
    test.describe.configure({ retries: 0 });

    test.beforeAll(async ({ request }) => {
        const api = new ApiHelper(request, BASE_URL);

        const existing = await api.listInterfaces();
        for (const iface of existing) {
            if (iface.name === 'Host Query UI Test Demo') {
                await api.deleteInterface(iface.id).catch(() => {});
            }
        }

        const created = await api.createInterface({
            name: 'Host Query UI Test Demo',
            description: 'Throwaway interface proving the TCP/MLLP Host Query tab renders and collects config correctly.',
            sourceType: 'hl7v2',
            sourceConnectivity: 'tcp_mllp_inbound',
            sourceConfig: { host: '0.0.0.0', port: PORT_HOSTQUERY_DEMO },
            sourceConnectorConfig: {
                connectorType: 'tcp_mllp_inbound',
                config: { host: '0.0.0.0', port: PORT_HOSTQUERY_DEMO },
            },
            targetType: 'file',
            targetConnectivity: 'file_writer',
            targetConfig: { outputPath: '/tmp/hostquery-ui-demo/{timestamp}.hl7', createDirs: true },
            targetConnectorConfig: {
                connectorType: 'file_writer',
                config: { outputPath: '/tmp/hostquery-ui-demo/{timestamp}.hl7', createDirs: true },
            },
            messageType: 'ADT^A01',
            mappings: [],
            transformationFlow: 'hl7_passthrough',
            auto_start: false,
            deployment_mode: 'manual',
            status: 'active',
            debug_logging: true,
            log_retention_days: 7,
        });
        state.interfaceId = created.interface?.id || created.interfaceId || created.id;
        expect(state.interfaceId, `createInterface response: ${JSON.stringify(created)}`).toBeTruthy();

        await api.savePipeline({
            interfaceId: state.interfaceId,
            messageType: 'ADT^A01',
            steps: [
                {
                    step_name: 'TCP/MLLP Inbound',
                    step_type: 'connector.inbound',
                    sequence: 10,
                    enabled: true,
                    config: {
                        connectorType: 'tcp_mllp_inbound',
                        config: { host: '0.0.0.0', port: PORT_HOSTQUERY_DEMO },
                        timeoutMs: 30000,
                    },
                },
            ],
        });
    });

    test.afterAll(async ({ request }) => {
        if (!state.interfaceId) return;
        const api = new ApiHelper(request, BASE_URL);
        await api.deleteInterface(state.interfaceId).catch(() => {});
    });

    test('SETUP sanity: demo interface was created', async () => {
        expect(state.interfaceId).toBeTruthy();
    });

    test('Host Query tab is visible for tcp_mllp_inbound and renders real default fields', async ({ page }) => {
        test.skip(!state.interfaceId, 'Setup failed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        const opened = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => (st.stepType || st.step_type) === 'connector.inbound');
            if (s && pb.propertiesPanel?.showStepProperties) {
                pb.propertiesPanel.showStepProperties(s);
                return true;
            }
            return false;
        });
        expect(opened, 'connector.inbound step should exist and its panel should open').toBe(true);
        await page.waitForTimeout(500);

        await expect(page.locator('#stepPropertiesModal')).toBeVisible({ timeout: 3000 });

        // Tab button must exist and be visible (not display:none) for tcp_mllp_inbound.
        const hostQueryTab = page.locator('#hostQueryTabBtn');
        await expect(hostQueryTab).toBeVisible({ timeout: 3000 });
        await expect(hostQueryTab).toContainText('Host Query');

        // The Acknowledgment tab should also be visible (unrelated, pre-existing
        // capability) — confirms this connector type still renders both tabs,
        // not one replacing the other.
        await expect(page.locator('#ackTabBtn')).toBeVisible();

        await hostQueryTab.click();
        const panel = page.locator('#connPanel-hostquery');
        await expect(panel).toBeVisible();

        // Basic group defaults: disabled, "QRY" placeholder/value, pipeline
        // message type "QRY".
        const enabledCheckbox = panel.locator('#hostQueryEnabled');
        await expect(enabledCheckbox).not.toBeChecked();
        const messageTypesInput = panel.locator('[data-hostquery-field="message_types"]');
        await expect(messageTypesInput).toHaveValue('QRY');
        const pipelineTypeInput = panel.locator('[data-hostquery-field="pipeline_message_type"]');
        await expect(pipelineTypeInput).toHaveValue('QRY');

        // Timeout & Failure group starts collapsed.
        const timeoutGroup = panel.locator('.connector-config-group', { hasText: 'Timeout & Failure Handling' });
        await expect(timeoutGroup).toHaveClass(/collapsed/);
    });

    test('Enabling Host Query and editing fields collects into a real host_query config block', async ({ page }) => {
        test.skip(!state.interfaceId, 'Setup failed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb.getAllStepsFlat();
            const s = steps.find(st => (st.stepType || st.step_type) === 'connector.inbound');
            pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(500);

        await page.locator('#hostQueryTabBtn').click();
        const panel = page.locator('#connPanel-hostquery');
        await expect(panel).toBeVisible();

        await panel.locator('#hostQueryEnabled').check();
        await panel.locator('[data-hostquery-field="message_types"]').fill('QRY, QBP');
        // pipeline_message_type is now a guided <select> (upgraded async from
        // its own real fetch of this interface's configured pipelines) with a
        // "+ Custom..." escape hatch — not a plain text input. Wait for the
        // real fetch to resolve (repopulating beyond the single-option
        // placeholder) before interacting with it.
        const pipelineTypeSelect = panel.locator('.hostquery-pipeline-type-select');
        await expect(pipelineTypeSelect.locator('option')).not.toHaveCount(2, { timeout: 5000 }); // placeholder = [current, __custom__]
        await pipelineTypeSelect.selectOption('QRY');

        // Expand the collapsed Timeout group before touching its fields.
        await panel.locator('.connector-config-group-header', { hasText: 'Timeout & Failure Handling' }).click();
        await panel.locator('[data-hostquery-field="timeout_seconds"]').fill('15');

        // Read back the real getConfig() output — the same method the panel's
        // own Save button calls — via the live ConnectorConfigBuilder instance.
        const collected = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const builder = pb.propertiesPanel.connectorConfigBuilder;
            return builder.getConfig();
        });

        expect(collected.config.host_query).toBeTruthy();
        expect(collected.config.host_query.enabled).toBe(true);
        expect(collected.config.host_query.message_types).toEqual(['QRY', 'QBP']);
        expect(collected.config.host_query.pipeline_message_type).toBe('QRY');
        expect(collected.config.host_query.timeout_seconds).toBe(15);
        // on_failure_nack defaults to checked/true and wasn't touched.
        expect(collected.config.host_query.on_failure_nack).toBe(true);
    });

    test('Leaving Host Query disabled omits the host_query block entirely from getConfig()', async ({ page }) => {
        test.skip(!state.interfaceId, 'Setup failed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb.getAllStepsFlat();
            const s = steps.find(st => (st.stepType || st.step_type) === 'connector.inbound');
            pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(500);

        // Don't touch the enabled checkbox at all — an untouched, disabled
        // panel must not add a host_query block to a step that never opted in.
        const collected = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const builder = pb.propertiesPanel.connectorConfigBuilder;
            return builder.getConfig();
        });

        expect(collected.config.host_query).toBeUndefined();
    });
});
