// tests/playwright/connector-array-field-persistence.spec.js
//
// Permanent regression guard for a path flagged during a 360 QA pass
// (October 2026) as "the generic comma-separated array-field mechanism
// exists on paper (ConnectorConfigBuilder.js renders schema.type==='array'
// as a comma-separated text input, splits/trims/filters on save) but was
// never actually driven through a real browser for a real connector field."
//
// Uses the DICOM Storage SCP connector's own allowed_calling_ae_titles
// field (array of strings) as the representative case: drag a generic
// "Inbound Connector" step onto a real canvas, pick dicom_storage_inbound
// from the connector-type dropdown, type a comma-separated AE-title list
// plus ae_title/port, save, reload the page fresh, reopen the same step,
// and confirm every value survived — not just that the dropdown listed the
// connector type.
//
// This exercises a DIFFERENT code path than step-config-persistence-
// regression.spec.js: connector.inbound/outbound steps collect their config
// via VisualStep.isConnectorStep(step) + ConnectorConfigBuilder.getConfig()
// in PropertiesPanel.js, never through the activeStepBuilder/
// StepBuilderRegistry mechanism that file's own bug lived in — so a clean
// pass here is a genuinely independent confirmation, not a duplicate of
// that fix.
const { test, expect } = require('@playwright/test');

async function createThrowawayInterface(request, name) {
    const res = await request.post('/api/interfaces', {
        data: { name, messageType: 'DICOM', description: 'connector array-field persistence regression check', sourceType: '', targetType: '', sourceConfig: {}, targetConfig: {} },
    });
    const data = await res.json();
    expect(data.success, JSON.stringify(data)).toBe(true);
    return data.interface?.id || data.id;
}

async function cleanupInterface(request, interfaceId) {
    await request.post(`/api/runtime/interfaces/${interfaceId}/deactivate`).catch(() => {});
    await request.delete(`/api/interfaces/${interfaceId}`).catch(() => {});
}

test.describe('Connector config array fields survive save -> reload (regression guard)', () => {
    test.describe.configure({ retries: 0 });

    test('DICOM Storage SCP: allowed_calling_ae_titles (array field) survives save and reload', async ({ page, request }) => {
        const interfaceId = await createThrowawayInterface(request, `PW ConnArrayPersist DICOM ${Date.now()}`);

        await page.goto(`/pipeline-builder.html?interfaceId=${interfaceId}`);
        await page.waitForSelector('#all-steps-list .step-card', { timeout: 15000 });
        await page.waitForTimeout(1000);

        const wrapper = page.locator('#canvasWrapper');
        const card = page.locator('.step-card', { hasText: 'Inbound Connector' });
        await expect(card, 'Inbound Connector should exist in the toolbox').toBeVisible({ timeout: 5000 });
        await card.dragTo(wrapper);
        await page.waitForTimeout(800);

        const node = page.locator('.flowchart-step-node', { hasText: /Inbound Connector/i }).first();
        await expect(node, 'Inbound Connector step node should appear on canvas after drag').toBeVisible({ timeout: 5000 });
        await node.dblclick();
        await page.waitForTimeout(500);

        const typeSelect = page.locator('#connectorTypeSelect');
        await expect(typeSelect, 'connector type dropdown should be visible').toBeVisible({ timeout: 5000 });
        await typeSelect.selectOption('dicom_storage_inbound');
        await page.waitForTimeout(500);

        const aeTitleField = page.locator('input.connector-config-field[data-field="ae_title"]');
        const portField = page.locator('input.connector-config-field[data-field="port"]');
        const arrayField = page.locator('input.connector-config-field[data-field="allowed_calling_ae_titles"]');

        await expect(aeTitleField, 'ae_title field should render').toBeVisible({ timeout: 5000 });

        // allowed_calling_ae_titles lives in the "Advanced" parameter group, which
        // starts collapsed by default when no value is set yet (progressive
        // disclosure, not a bug) — expand it before interacting with the field.
        const advancedHeader = page.locator('.connector-config-group-header', { hasText: /advanced/i }).first();
        await expect(advancedHeader, 'Advanced parameter group header should exist').toBeVisible({ timeout: 5000 });
        await advancedHeader.click();
        await page.waitForTimeout(300);

        await expect(arrayField, 'allowed_calling_ae_titles array field should render as a comma-separated text input').toBeVisible({ timeout: 5000 });

        await aeTitleField.fill('REGRESSSCP');
        await portField.fill('11199');
        await arrayField.fill('FUJIFILM_CR, OTHER_MODALITY , THIRD_AE');

        await page.locator('#saveStepBtn').click();
        await page.waitForTimeout(1500);

        await page.goto(`/pipeline-builder.html?interfaceId=${interfaceId}`);
        await page.waitForSelector('#all-steps-list .step-card', { timeout: 15000 });
        await page.waitForTimeout(1000);

        const reloadedNode = page.locator('.flowchart-step-node', { hasText: /Inbound Connector/i }).first();
        await expect(reloadedNode, 'Inbound Connector step should still exist after reload').toBeVisible({ timeout: 5000 });
        await reloadedNode.dblclick();
        await page.waitForTimeout(500);

        await expect(page.locator('#connectorTypeSelect'), 'connector type must still be dicom_storage_inbound after reload').toHaveValue('dicom_storage_inbound');
        await expect(page.locator('input.connector-config-field[data-field="ae_title"]'), 'ae_title must retain its saved value after reload').toHaveValue('REGRESSSCP');
        await expect(page.locator('input.connector-config-field[data-field="port"]'), 'port must retain its saved value after reload').toHaveValue('11199');

        // The Advanced group auto-expands on load when a value is already set
        // (see ConnectorConfigBuilder.js's "Collapse non-basic groups by default
        // if no values set" logic) — but defensively expand it anyway rather
        // than depend on that behavior, so this assertion stays robust even if
        // that heuristic changes.
        const arrayFieldAfterReload = page.locator('input.connector-config-field[data-field="allowed_calling_ae_titles"]');
        if (!(await arrayFieldAfterReload.isVisible())) {
            await page.locator('.connector-config-group-header', { hasText: /advanced/i }).first().click();
            await page.waitForTimeout(300);
        }
        await expect(arrayFieldAfterReload, 'allowed_calling_ae_titles array field should be visible after reload').toBeVisible({ timeout: 5000 });

        const arrayValueAfterReload = await arrayFieldAfterReload.inputValue();
        const parsedBack = arrayValueAfterReload.split(',').map(v => v.trim()).filter(v => v);
        expect(parsedBack, `allowed_calling_ae_titles must retain all 3 saved AE titles, got raw value "${arrayValueAfterReload}"`).toEqual(['FUJIFILM_CR', 'OTHER_MODALITY', 'THIRD_AE']);

        await cleanupInterface(request, interfaceId);
    });
});
