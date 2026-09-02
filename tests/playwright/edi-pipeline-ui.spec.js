// tests/playwright/edi-pipeline-ui.spec.js
// EDI X12 pipeline-builder UI coverage: toolbox visibility for the 4 EDI
// step types (edi.parse/edi.validate/edi.map_to_canonical/edi.build),
// StepBuilderRegistry registration, and each step's own config panel
// actually rendering its type-specific fields (not just the generic
// Basic-Properties wrapper every step type shares) -- including the two
// pieces with live schema-API-driven async rendering: edi.validate's
// custom-rule segment picker and edi.map_to_canonical's recursive loop
// tree, both fed by GET /api/edi/schema/segments and
// GET /api/edi/schema/transaction-sets/835/loops.
const { test, expect } = require('@playwright/test');

test('EDI pipeline builder UI renders all 4 step types correctly', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/pipeline-builder.html');
    await page.waitForSelector('#all-steps-list .step-card', { timeout: 15000 });

    // 1. Toolbox visibility: all 4 EDI cards present with expected names.
    const allStepsText = await page.locator('#all-steps-list').innerText();
    for (const name of ['EDI X12 Parse', 'EDI X12 Validate', 'EDI Map to Canonical', 'EDI X12 Build']) {
        expect(allStepsText, `toolbox should list "${name}"`).toContain(name);
    }

    // 2. StepBuilderRegistry has all 4 EDI types registered. (Top-level
    // `class` declarations in a classic script do NOT attach to `window` --
    // reference the bare identifier, same as every *StepBuilder.js file's
    // own registration call does.)
    const registered = await page.evaluate(() => ({
        parse: StepBuilderRegistry.has('edi.parse'),
        validate: StepBuilderRegistry.has('edi.validate'),
        map: StepBuilderRegistry.has('edi.map_to_canonical'),
        build: StepBuilderRegistry.has('edi.build'),
    }));
    expect(registered).toEqual({ parse: true, validate: true, map: true, build: true });

    // 3. Add each EDI step programmatically (bypassing drag-and-drop, which
    // Playwright's HTML5 DnD emulation handles poorly for this canvas) and
    // open its Properties panel -- the real code path drag-and-drop would
    // also hit (VisualStep -> builder.addStep -> propertiesPanel.showStepProperties).
    // Assert on a step-TYPE-SPECIFIC field id and its real default value,
    // not just "panel has content" -- every step type shares a generic
    // Basic-Properties/Execution-Settings wrapper.
    const expectedField = {
        'edi.parse': { id: '#ediParseSourceField', defaultValue: 'raw' },
        'edi.validate': { id: '#ediValidateSourceField', defaultValue: 'raw' },
        'edi.map_to_canonical': { id: '#ediMapOutputField', defaultValue: 'canonicalEDI' },
        'edi.build': { id: '#ediBuildSourceField', defaultValue: 'parsedEDI' },
    };
    for (const stepType of Object.keys(expectedField)) {
        await page.evaluate((st) => {
            const step = new VisualStep({ stepName: 'Test ' + st, stepType: st, config: {} });
            window.pipelineBuilder.addStep(step);
            window.pipelineBuilder.propertiesPanel.showStepProperties(step, false);
        }, stepType);

        await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
        const { id, defaultValue } = expectedField[stepType];
        const fieldLocator = page.locator(id);
        await expect(fieldLocator, `${stepType} should render its own config field ${id}`).toBeVisible({ timeout: 5000 });
        await expect(fieldLocator, `${stepType}'s ${id} should default to "${defaultValue}"`).toHaveValue(defaultValue);
    }

    // 4. edi.validate specifically: the custom-rule builder's segment picker
    // is populated from the live /api/edi/schema/segments catalog (proves
    // the async fetch + rerender path works, not just the static markup).
    await page.evaluate(() => {
        const step = new VisualStep({ stepName: 'Test edi.validate rules', stepType: 'edi.validate', config: {} });
        window.pipelineBuilder.addStep(step);
        window.pipelineBuilder.propertiesPanel.showStepProperties(step, false);
    });
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await page.locator('button:has-text("Add Custom Rule")').click();
    await page.waitForFunction(
        () => document.querySelectorAll('#formTabContent select option').length > 20,
        { timeout: 5000 }
    );

    // 5. edi.map_to_canonical: the loop tree renders from the live
    // /api/edi/schema/transaction-sets/835/loops catalog, including nested
    // children (1000A/1000B/2000 -> 2100 -> 2110).
    await page.evaluate(() => {
        const step = new VisualStep({ stepName: 'Test map loops', stepType: 'edi.map_to_canonical', config: {} });
        window.pipelineBuilder.addStep(step);
        window.pipelineBuilder.propertiesPanel.showStepProperties(step, false);
    });
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await page.waitForFunction(
        () => (document.getElementById('formTabContent')?.innerText || '').includes('2110'),
        { timeout: 5000 }
    );
    const loopsPanelText = await page.locator('#formTabContent').innerText();
    for (const loopId of ['1000A', '1000B', '2000', '2100', '2110']) {
        expect(loopsPanelText, `map_to_canonical loop tree should show loop ${loopId}`).toContain(loopId);
    }

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
