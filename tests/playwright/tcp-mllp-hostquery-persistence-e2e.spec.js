'use strict';
/**
 * tcp-mllp-hostquery-persistence-e2e.spec.js — closes two gaps left open by
 * tcp-mllp-hostquery-e2e.spec.js's own coverage:
 *
 *   1. That spec only ever calls connectorConfigBuilder.getConfig() directly
 *      via page.evaluate() — it never proves clicking the REAL Save button
 *      (#saveStepBtn) persists host_query to the database and survives a
 *      full page reload (a fresh GET of the pipeline from the server, not
 *      just in-memory JS state).
 *   2. It never exercises the "+ Custom / not yet created…" escape hatch on
 *      the Answering Pipeline's Message Type picker (_wirePipelineMessageTypeField
 *      in ConnectorConfigBuilder.js) — only ever selects a real, already-
 *      configured pipeline's message type.
 *
 * Run: npx playwright test tcp-mllp-hostquery-persistence-e2e --project=chromium
 */

const { test, expect } = require('@playwright/test');
const { ApiHelper }     = require('./helpers/api');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const PORT_HOSTQUERY_PERSIST_DEMO = 6641; // distinct port from tcp-mllp-hostquery-e2e.spec.js's 6640

const state = { interfaceId: null };

async function openConnectorInboundStep(page, interfaceId) {
    await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${interfaceId}`);
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
}

test.describe.serial('TCP/MLLP Host Query — real persistence + custom pipeline-type escape hatch', () => {
    test.describe.configure({ retries: 0 });

    test.beforeAll(async ({ request }) => {
        const api = new ApiHelper(request, BASE_URL);

        const existing = await api.listInterfaces();
        for (const iface of existing) {
            if (iface.name === 'Host Query Persistence Test Demo') {
                await api.deleteInterface(iface.id).catch(() => {});
            }
        }

        const created = await api.createInterface({
            name: 'Host Query Persistence Test Demo',
            description: 'Throwaway interface proving the Host Query tab persists through a real Save + page reload, and that the custom pipeline-type escape hatch works.',
            sourceType: 'hl7v2',
            sourceConnectivity: 'tcp_mllp_inbound',
            sourceConfig: { host: '0.0.0.0', port: PORT_HOSTQUERY_PERSIST_DEMO },
            sourceConnectorConfig: {
                connectorType: 'tcp_mllp_inbound',
                config: { host: '0.0.0.0', port: PORT_HOSTQUERY_PERSIST_DEMO },
            },
            targetType: 'file',
            targetConnectivity: 'file_writer',
            targetConfig: { outputPath: '/tmp/hostquery-persist-demo/{timestamp}.hl7', createDirs: true },
            targetConnectorConfig: {
                connectorType: 'file_writer',
                config: { outputPath: '/tmp/hostquery-persist-demo/{timestamp}.hl7', createDirs: true },
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
                        config: { host: '0.0.0.0', port: PORT_HOSTQUERY_PERSIST_DEMO },
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

    test('Clicking the real Save button persists host_query to the DB and survives a page reload', async ({ page }) => {
        test.skip(!state.interfaceId, 'Setup failed');

        await openConnectorInboundStep(page, state.interfaceId);

        await page.locator('#hostQueryTabBtn').click();
        const panel = page.locator('#connPanel-hostquery');
        await expect(panel).toBeVisible();

        await panel.locator('#hostQueryEnabled').check();
        await panel.locator('[data-hostquery-field="message_types"]').fill('QRY, QBP');

        const pipelineTypeSelect = panel.locator('.hostquery-pipeline-type-select');
        await expect(pipelineTypeSelect.locator('option')).not.toHaveCount(2, { timeout: 5000 });
        await pipelineTypeSelect.selectOption('ADT^A01'); // the one real pipeline this interface has

        await panel.locator('.connector-config-group-header', { hasText: 'Timeout & Failure Handling' }).click();
        await panel.locator('[data-hostquery-field="timeout_seconds"]').fill('22');
        await panel.locator('#hostQueryOnFailureNack').uncheck();

        // Click the REAL save button and wait for the REAL network round trip
        // (PipelineAPIService.savePipeline -> POST /api/pipelines) to finish,
        // not just the modal closing — persistStepUpdate's _autoSaveAndClose
        // fires that request without awaiting it before closing the modal.
        const saveResponse = page.waitForResponse(
            resp => resp.url().includes('/api/pipelines') && resp.request().method() === 'POST',
            { timeout: 10000 }
        );
        await page.locator('#saveStepBtn').click();
        const resp = await saveResponse;
        expect(resp.ok(), `pipeline save should succeed, got ${resp.status()}`).toBe(true);

        await expect(page.locator('#stepPropertiesModal')).toBeHidden({ timeout: 3000 });

        // Full page reload — forces a fresh GET of the pipeline from the
        // server, proving the config round-tripped through the database
        // rather than only surviving in this tab's in-memory JS state.
        await openConnectorInboundStep(page, state.interfaceId);
        await page.locator('#hostQueryTabBtn').click();
        const reloadedPanel = page.locator('#connPanel-hostquery');
        await expect(reloadedPanel).toBeVisible();

        await expect(reloadedPanel.locator('#hostQueryEnabled')).toBeChecked();
        await expect(reloadedPanel.locator('[data-hostquery-field="message_types"]')).toHaveValue('QRY, QBP');

        // The pipeline-type field re-renders as the single-option placeholder
        // first (now showing the PERSISTED value), then upgrades async —
        // assert on whichever control currently carries the live
        // data-hostquery-field attribute, matching getConfig()'s own
        // "exactly one such control at a time" contract.
        const persistedPipelineType = reloadedPanel.locator('[data-hostquery-field="pipeline_message_type"]');
        await expect(persistedPipelineType).toHaveValue('ADT^A01', { timeout: 5000 });

        // Expand the Timeout group again (collapsed by default) before
        // reading its persisted values.
        await reloadedPanel.locator('.connector-config-group-header', { hasText: 'Timeout & Failure Handling' }).click();
        await expect(reloadedPanel.locator('[data-hostquery-field="timeout_seconds"]')).toHaveValue('22');
        await expect(reloadedPanel.locator('#hostQueryOnFailureNack')).not.toBeChecked();
    });

    test('The "+ Custom / not yet created…" escape hatch lets a free-typed pipeline message type be collected and saved', async ({ page }) => {
        test.skip(!state.interfaceId, 'Setup failed');

        await openConnectorInboundStep(page, state.interfaceId);
        await page.locator('#hostQueryTabBtn').click();
        const panel = page.locator('#connPanel-hostquery');
        await expect(panel).toBeVisible();

        await panel.locator('#hostQueryEnabled').check();

        const select = panel.locator('.hostquery-pipeline-type-select');
        // Wait for the real async fetch (this interface's configured
        // pipelines) to repopulate the select beyond its two-option
        // placeholder before choosing "+ Custom...".
        await expect(select.locator('option')).not.toHaveCount(2, { timeout: 5000 });
        await select.selectOption('__custom__');

        // Entering custom mode hides the select and reveals a free-text
        // input carrying the live data-hostquery-field attribute instead.
        await expect(select).toBeHidden();
        const customWrap = panel.locator('.hostquery-pipeline-type-custom-wrap');
        await expect(customWrap).toBeVisible();
        const customInput = customWrap.locator('.hostquery-pipeline-type-custom-input');
        await expect(customInput).toHaveAttribute('data-hostquery-field', 'pipeline_message_type');

        await customInput.fill('QBP^Q11');

        let collected = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const builder = pb.propertiesPanel.connectorConfigBuilder;
            return builder.getConfig();
        });
        expect(collected.config.host_query.pipeline_message_type).toBe('QBP^Q11');

        // "Back to list" must restore select mode and move the live
        // data-hostquery-field attribute back onto the select — getConfig()
        // must never double-count both controls at once.
        await customWrap.locator('.hostquery-pipeline-type-back').click();
        await expect(select).toBeVisible();
        await expect(customWrap).toBeHidden();
        await expect(select).toHaveAttribute('data-hostquery-field', 'pipeline_message_type');
        await expect(customInput).not.toHaveAttribute('data-hostquery-field', 'pipeline_message_type');

        collected = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const builder = pb.propertiesPanel.connectorConfigBuilder;
            return builder.getConfig();
        });
        // Back in select mode, the collected value is whatever the select is
        // currently showing (a real configured message type), not the
        // custom text that was abandoned.
        expect(collected.config.host_query.pipeline_message_type).not.toBe('QBP^Q11');

        // Re-enter custom mode and actually save it for real, proving the
        // escape hatch's value survives the same real DB round trip the
        // previous test proved for the guided-select path.
        await select.selectOption('__custom__');
        await customInput.fill('QBP^Q11');

        const saveResponse = page.waitForResponse(
            resp => resp.url().includes('/api/pipelines') && resp.request().method() === 'POST',
            { timeout: 10000 }
        );
        await page.locator('#saveStepBtn').click();
        const resp = await saveResponse;
        expect(resp.ok(), `pipeline save should succeed, got ${resp.status()}`).toBe(true);
        await expect(page.locator('#stepPropertiesModal')).toBeHidden({ timeout: 3000 });

        await openConnectorInboundStep(page, state.interfaceId);
        await page.locator('#hostQueryTabBtn').click();
        const reloadedPanel = page.locator('#connPanel-hostquery');
        await expect(reloadedPanel.locator('[data-hostquery-field="pipeline_message_type"]'))
            .toHaveValue('QBP^Q11', { timeout: 5000 });
    });
});
