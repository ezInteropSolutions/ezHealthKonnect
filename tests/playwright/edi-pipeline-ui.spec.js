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
const fs = require('fs');
const path = require('path');

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
    // Also checks the Documentation tab (EDIStepsDocs.js) for each type -- a
    // distinctive substring from that type's own real doc entry, proving it's
    // not falling back to getStepDocumentation's generic "this is a custom
    // step type" placeholder (which every one of these 4 types rendered
    // before EDIStepsDocs.js existed).
    const expectedField = {
        'edi.parse': { id: '#ediParseSourceField', defaultValue: 'raw', docText: 'envelopePresent' },
        'edi.validate': { id: '#ediValidateSourceField', defaultValue: 'raw', docText: 'customRules' },
        'edi.map_to_canonical': { id: '#ediMapOutputField', defaultValue: 'canonicalEDI', docText: 'canonicalEDI' },
        'edi.build': { id: '#ediBuildSourceField', defaultValue: 'parsedEDI', docText: 'ISA...IEA' },
    };
    for (const stepType of Object.keys(expectedField)) {
        await page.evaluate((st) => {
            const step = new VisualStep({ stepName: 'Test ' + st, stepType: st, config: {} });
            window.pipelineBuilder.addStep(step);
            window.pipelineBuilder.propertiesPanel.showStepProperties(step, false);
        }, stepType);

        await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
        const { id, defaultValue, docText } = expectedField[stepType];
        const fieldLocator = page.locator(id);
        await expect(fieldLocator, `${stepType} should render its own config field ${id}`).toBeVisible({ timeout: 5000 });
        await expect(fieldLocator, `${stepType}'s ${id} should default to "${defaultValue}"`).toHaveValue(defaultValue);

        await page.locator('#stepPropertiesModal .modal-tab[data-tab="docs"]').click();
        const docsText = await page.locator('#docsTabContent').innerText();
        expect(docsText, `${stepType}'s Documentation tab should not fall back to the generic placeholder`).not.toContain('This is a custom step type');
        expect(docsText, `${stepType}'s Documentation tab should contain "${docText}"`).toContain(docText);
        await page.locator('#stepPropertiesModal .modal-tab[data-tab="form"]').click();
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

// EDI Phase 5's OOB template (database/migrations/V231__EDI_835_To_FHIR_OOB_Pipeline_Template.sql),
// driven through the REAL "Use Template" UI flow (Templates gallery card ->
// configure modal -> Create Interface -> redirect to pipeline-builder.html)
// rather than the API calls used to verify the migration itself -- this is
// the actual click path a user takes, and the one thing API-level
// verification can't prove: that the 7 saved steps render on the real
// canvas and each one's own config panel opens with the template's real
// saved values, with no console errors.
test('EDI 835 to FHIR template renders all 7 steps on canvas with correct config after Use Template', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 835 to FHIR' });
    await expect(card, 'Templates gallery should list the EDI 835 to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    const interfaceName = `PW EDI-FHIR Test ${Date.now()}`;
    await page.fill('#tcf_name', interfaceName);
    await page.click('#tcf_submit');

    // _submitConfigureTemplate does 3 real network round trips (load
    // scaffold, create interface, save pipeline) then redirects — give it
    // real time rather than a fixed short wait.
    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 7 steps').toBe(7);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 7 steps should render as canvas nodes').toBe(7);

    // Click each canvas node in turn (the real interaction — a single click
    // both selects the step and opens its properties panel, per
    // FlowchartRenderer.js's node.onclick -> selectStep -> onStepSelected ->
    // openStepProperties) and confirm its own config panel shows the
    // template's real saved values, not just "a panel opened".
    const stepChecks = [
        {
            name: 'Parse 835 -> JSON',
            assert: async () => {
                await expect(page.locator('#ediParseSourceField')).toHaveValue('raw');
            },
        },
        {
            name: 'Validate Against X12 5010',
            assert: async () => {
                await expect(page.locator('#ediValidateSourceField')).toHaveValue('raw');
            },
        },
        {
            name: 'Build PaymentReconciliation',
            assert: async () => {
                await expect(page.locator('#fbbResourceType')).toHaveValue('PaymentReconciliation');
            },
        },
        {
            name: 'Assemble 835 FHIR Bundle',
            assert: async () => {
                await expect(page.locator('#pb-tab-fhir_bundle')).toBeVisible();
                await expect(page.locator('#pb-resource-paths-list .pb-rp-input').first()).toHaveValue('message.paymentReconciliation');
            },
        },
        {
            name: 'Validate FHIR Bundle',
            assert: async () => {
                await expect(page.locator('#fhirValidationLevel')).toHaveValue('strict');
            },
        },
    ];

    // The modal from the PREVIOUS step must be closed before clicking the
    // next canvas node — it's a fixed-position overlay that can sit on top
    // of an as-yet-unclicked node (e.g. the leftmost "Receive 835" node),
    // intercepting the click rather than the new node replacing the old
    // panel's content.
    const closeStepModal = async () => {
        const closeBtn = page.locator('#stepPropertiesModal .modal-close');
        if (await closeBtn.count() > 0) await closeBtn.first().click();
        await page.waitForSelector('#stepPropertiesModal', { state: 'hidden', timeout: 5000 }).catch(() => {});
    };

    for (const { name, assert } of stepChecks) {
        const node = page.locator('.flowchart-step-node', { hasText: name });
        await expect(node, `canvas should have a step node for "${name}"`).toBeVisible({ timeout: 5000 });
        await node.click();
        await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
        await assert();
        await closeStepModal();
    }

    // The two connector steps reuse the existing, already-covered connector
    // config UI unchanged from V230's own template -- just confirm each
    // opens cleanly rather than re-asserting connector-specific fields.
    for (const name of ['Receive 835 File (SFTP)', 'Store Result']) {
        const node = page.locator('.flowchart-step-node', { hasText: name });
        await expect(node, `canvas should have a step node for "${name}"`).toBeVisible({ timeout: 5000 });
        await node.click();
        await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
        await closeStepModal();
    }

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

// edi_x12_inbound's own connector-config UI got 3 fixes this round: (1)
// transaction_types renders as a checkbox list (schema now carries
// items.enum, V232 migration) instead of a free-text comma-separated box,
// (2) auth_type toggles password vs. key_content visibility generically
// (setupPasswordKeyAuthVisibility in ConnectorConfigBuilder.js, driven by
// the schema's own auth_type enum rather than a per-connector-type special
// case), (3) key_content gets a "Browse for key file..." button that reads a
// local file's text via FileReader. Plus: the Documentation tab's
// connector.inbound connectorTypeCards array previously had zero entry for
// edi_x12_inbound at all -- this proves the new card renders correctly too.
test('edi_x12_inbound connector config: transaction_types checkbox, auth_type visibility toggle, key browse button, and Documentation card', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/pipeline-builder.html');
    await page.waitForSelector('#all-steps-list .step-card', { timeout: 15000 });

    await page.evaluate(() => {
        const step = new VisualStep({
            stepName: 'Test EDI Inbound',
            stepType: 'connector.inbound',
            config: { connectorType: 'edi_x12_inbound', config: { auth_type: 'password', transaction_types: ['835'] } }
        });
        window.pipelineBuilder.addStep(step);
        window.pipelineBuilder.propertiesPanel.showStepProperties(step, false);
    });
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    // The real field is a hidden <input> (getConfig() reads it the same as
    // any other array field) -- wait on the visible checklist wrapper instead.
    await page.waitForSelector('.connector-config-array-checklist', { timeout: 5000 });

    // 1. transaction_types is a checkbox list with 4 options (835/837P/837I/999,
    // V235's own enum expansion once 837/999 became real schema-backed
    // transaction sets — was 1 option pre-Phase-2), "835" pre-checked from
    // the seeded config, backed by a hidden field carrying the same
    // comma-joined value getConfig() already expects.
    const transactionCheckboxes = page.locator('.connector-config-array-checklist input[type="checkbox"]');
    await expect(transactionCheckboxes, 'transaction_types should render 4 checkboxes (835/837P/837I/999)').toHaveCount(4);
    // The checkbox itself carries no value attribute (ConnectorConfigBuilder.js
    // never sets cb.value — it browser-defaults to "on") — the real option
    // text lives on the associated <label for="...">, matched by id.
    const checkedValues = await transactionCheckboxes.evaluateAll(
        boxes => boxes.filter(b => b.checked).map(b => document.querySelector(`label[for="${b.id}"]`)?.textContent.trim())
    );
    expect(checkedValues, 'only 835 should be pre-checked from the seeded config').toEqual(['835']);
    await expect(page.locator('.connector-config-field[data-field="transaction_types"]')).toHaveValue('835');

    // 2. auth_type=password (the seeded default): password field visible, key_content hidden.
    const passwordGroup = page.locator('.connector-config-field[data-field="password"]').locator('xpath=ancestor::div[contains(@class,"form-group")][1]');
    const keyContentGroup = page.locator('.connector-config-field[data-field="key_content"]').locator('xpath=ancestor::div[contains(@class,"form-group")][1]');
    await expect(passwordGroup, 'password field should be visible when auth_type=password').toBeVisible();
    await expect(keyContentGroup, 'key_content field should be hidden when auth_type=password').toBeHidden();

    // 3. Switching auth_type to "key" flips visibility, and key_content's
    // form-group now shows the "Browse for key file..." button.
    await page.locator('.connector-config-field[data-field="auth_type"]').selectOption('key');
    await expect(passwordGroup, 'password field should hide when auth_type=key').toBeHidden();
    await expect(keyContentGroup, 'key_content field should show when auth_type=key').toBeVisible();
    await expect(keyContentGroup.locator('button:has-text("Browse for key file")'), 'key_content should have a Browse button').toBeVisible();

    // 4. Documentation tab: edi_x12_inbound now has its own connectorTypeCards
    // entry (previously absent entirely) with the real schema field names.
    await page.locator('#stepPropertiesModal .modal-tab[data-tab="docs"]').click();
    await page.locator('#connectorTypeDocSelect').selectOption('edi_x12_inbound');
    const docDetail = page.locator('#connectorTypeDocDetail');
    await expect(docDetail).toBeVisible();
    const docText = await docDetail.innerText();
    expect(docText, 'doc card should identify the connector as the 835 remittance connector').toContain('EDI X12 Inbound');
    expect(docText, 'doc card should document key_content').toContain('key_content');
    expect(docText, 'doc card should document transaction_types').toContain('transaction_types');
    expect(docText, 'doc card should document transport').toContain('transport');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

// edi.validate got two new capabilities this round: (1) an "OOB Validation
// Rules" section listing every schema-defined business-rule constraint
// (only BPR/CAS/N1/N4/PER carry any, fed by GET /api/edi/schema/segments'
// new syntaxRules field) with a checkbox to selectively suppress one for
// this step only, backed by step.config.disabledRules; (2) the
// Documentation tab now explains the pre-existing Custom Rules mechanism
// step-by-step with a worked example, instead of assuming it's obvious.
test('edi.validate: OOB rules list with enable/disable toggle, and Documentation explains Custom Rules', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/pipeline-builder.html');
    await page.waitForSelector('#all-steps-list .step-card', { timeout: 15000 });

    await page.evaluate(() => {
        const step = new VisualStep({ stepName: 'Test EDI Validate', stepType: 'edi.validate', config: {} });
        window._testEdiValidateStep = step;
        window.pipelineBuilder.addStep(step);
        window.pipelineBuilder.propertiesPanel.showStepProperties(step, false);
    });
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });

    // 1. The OOB rules section lists PER among the segments with real
    // constraints, and defaults every rule to enabled (checked).
    await page.waitForFunction(
        () => (document.getElementById('ediValidateBuilder')?.innerText || '').includes('PER'),
        { timeout: 5000 }
    );
    const oobSection = page.locator('#ediValidateBuilder');
    await expect(oobSection, 'OOB rules section should list PER (a segment with real syntax rules)').toContainText('PER');
    await expect(oobSection, 'OOB rules section should show the Paired rule type label').toContainText('Paired');

    const perCheckboxes = page.locator('#ediValidateBuilder label:has-text("communicationNumberQualifier1") input[type="checkbox"]');
    await expect(perCheckboxes.first(), 'PER\'s 03/04 rule checkbox should exist and default to checked').toBeChecked();

    // 2. Unchecking it records a disabledRules entry on the live step config
    // (the same object showStepProperties was called with).
    await perCheckboxes.first().click();
    const disabledAfterUncheck = await page.evaluate(() => window._testEdiValidateStep.config.disabledRules);
    expect(disabledAfterUncheck, 'unchecking should add exactly one disabledRules entry').toHaveLength(1);
    expect(disabledAfterUncheck[0].segmentId).toBe('PER');
    expect(disabledAfterUncheck[0].type).toBe('P');

    // 3. Re-checking removes it again.
    await perCheckboxes.first().click();
    const disabledAfterRecheck = await page.evaluate(() => window._testEdiValidateStep.config.disabledRules);
    expect(disabledAfterRecheck, 're-checking should remove the disabledRules entry').toHaveLength(0);

    // 4. Documentation tab: explains the custom-rule mechanism step by step
    // and shows a real worked example (the PER "at least one contact" rule),
    // not just a bare parameter list.
    await page.locator('#stepPropertiesModal .modal-tab[data-tab="docs"]').click();
    const docsText = await page.locator('#docsTabContent').innerText();
    expect(docsText, 'Documentation should explain how custom rules work').toContain('How Custom Rules actually work');
    expect(docsText, 'Documentation should mention the OOB rules checkbox list').toContain('OOB Validation Rules');
    expect(docsText, 'Documentation should include the worked PER custom-rule example').toContain('communicationNumberQualifier1');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

// "Assemble 835 FHIR Bundle" (payload.builder, fhir_bundle mode) previously
// gave the user a bare text box for Resource Paths with no way to discover
// what "message.paymentReconciliation" even was or where it came from --
// this proves the fix end-to-end: (1) FHIRBuildExecutor.GetOutputVariables
// now reports THIS step's own configured outputField (not a hardcoded
// "fhirResource" default), and (2) PayloadBuilderBuilder now wires a real
// FieldPathSearchComponent picker onto Resource Paths, Simple mode's custom
// source, and Field Builder's field_ref values -- the same no-code
// "search/click instead of type" mechanism fhir.build/hl7.build already use.
test('payload.builder Resource Paths has a working field picker sourced from the real prior step', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });
    await page.locator('.tg-card', { hasText: 'EDI X12 835 to FHIR' }).locator('.tg-use-btn').click();
    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW Picker Test ${Date.now()}`);
    await page.click('#tcf_submit');
    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });
    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });

    await page.locator('.flowchart-step-node', { hasText: 'Assemble 835 FHIR Bundle' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });

    const rpInput = page.locator('.pb-rp-input').first();
    await rpInput.click();
    const dropdown = page.locator('.field-path-search-dropdown:visible');
    await expect(dropdown, 'clicking Resource Paths should open a picker dropdown').toBeVisible({ timeout: 5000 });
    const suggestion = dropdown.locator('.field-path-item', { hasText: 'Build_PaymentReconciliation' });
    await expect(suggestion, 'the picker should list the real prior step, not a hardcoded default').toBeVisible({ timeout: 5000 });
    const suggestionText = await suggestion.innerText();
    expect(suggestionText, 'the suggested path should reflect the step\'s ACTUAL configured outputField').toContain('message.paymentReconciliation');

    await suggestion.click();
    await expect(rpInput, 'clicking the suggestion should fill in the correct path').toHaveValue('message.paymentReconciliation');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

// EDI Phase 2 (837P/837I/999): EDI_TRANSACTION_SETS grew from ['835'] to
// ['835','837P','837I','999'] (edi.parse/edi.build's own picker), and
// edi.map_to_canonical gained a Transaction Set picker it previously had
// none of at all — it used to always fetch 835's own loop tree regardless of
// step config. Proves both: the dropdown now offers all 4, and switching it
// on edi.map_to_canonical actually re-fetches and re-renders a DIFFERENT
// transaction set's own loop tree (not just that the picker exists).
test('EDI Phase 2: transaction set picker offers 837P/837I/999, and edi.map_to_canonical re-fetches the loop tree on change', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/pipeline-builder.html');
    await page.waitForSelector('#all-steps-list .step-card', { timeout: 15000 });

    // 1. edi.build's own Transaction Set dropdown lists all 4 real options.
    await page.evaluate(() => {
        const step = new VisualStep({ stepName: 'Test edi.build txSets', stepType: 'edi.build', config: {} });
        window.pipelineBuilder.addStep(step);
        window.pipelineBuilder.propertiesPanel.showStepProperties(step, false);
    });
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    const buildOptions = await page.locator('#ediBuildTransactionSet option').allTextContents();
    expect(buildOptions, 'edi.build Transaction Set dropdown should list all 4 real transaction sets').toEqual(['835', '837P', '837I', '999']);
    await expect(page.locator('#ediBuildTransactionSet')).toHaveValue('835');
    await page.locator('#stepPropertiesModal .modal-close').first().click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'hidden', timeout: 5000 }).catch(() => {});

    // 2. edi.map_to_canonical: picker exists (it had none before Phase 2),
    // defaults to 835, and its own loop tree starts as 835's (2100/2110).
    await page.evaluate(() => {
        const step = new VisualStep({ stepName: 'Test map txSet switch', stepType: 'edi.map_to_canonical', config: {} });
        window._testMapStep = step;
        window.pipelineBuilder.addStep(step);
        window.pipelineBuilder.propertiesPanel.showStepProperties(step, false);
    });
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await page.waitForFunction(
        () => (document.getElementById('formTabContent')?.innerText || '').includes('2110'),
        { timeout: 5000 }
    );
    const mapTxSetSelect = page.locator('#ediMapToCanonicalBuilder select').first();
    await expect(mapTxSetSelect, 'edi.map_to_canonical should now have its own Transaction Set picker').toBeVisible();
    const mapOptions = await mapTxSetSelect.locator('option').allTextContents();
    expect(mapOptions, 'edi.map_to_canonical Transaction Set dropdown should list all 4 real transaction sets').toEqual(['835', '837P', '837I', '999']);
    await expect(mapTxSetSelect).toHaveValue('835');
    let panelText = await page.locator('#formTabContent').innerText();
    expect(panelText, 'default (835) loop tree should show 2100/2110').toContain('2110');
    expect(panelText, 'default (835) loop tree should NOT show 837-only loops').not.toContain('2010AA');

    // 3. Switching to 837P re-fetches and re-renders a STRUCTURALLY DIFFERENT
    // loop tree (2010AA/2000A/2000B — none of which exist in 835's own tree)
    // — the real proof this isn't just a cosmetic dropdown that never
    // actually changes what the Loops section below it shows.
    await mapTxSetSelect.selectOption('837P');
    await page.waitForFunction(
        () => (document.getElementById('formTabContent')?.innerText || '').includes('2010AA'),
        { timeout: 5000 }
    );
    panelText = await page.locator('#formTabContent').innerText();
    for (const loopId of ['1000A', '1000B', '2000A', '2010AA', '2000B']) {
        expect(panelText, `837P loop tree should show loop ${loopId} after switching`).toContain(loopId);
    }
    expect(panelText, '837P loop tree should no longer show the stale 835 loop tree markers').not.toContain('2110');

    // 4. The config itself was actually updated (not just the DOM), and
    // switching cleared any prior loop mappings (they addressed 835's own
    // loop IDs, which don't all exist in 837P's tree).
    const mapConfig = await page.evaluate(() => window._testMapStep.config);
    expect(mapConfig.transactionSet, 'step.config.transactionSet should reflect the switch').toBe('837P');
    expect(mapConfig.loops, 'switching transaction sets should clear prior loop mappings').toEqual([]);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

// V236's own OOB template (database/migrations/V236__EDI_837_OOB_Pipeline_Template.sql),
// driven through the REAL "Use Template" UI flow — same discipline as the
// 835-to-FHIR template's own test above, since API-level verification alone
// can't prove the 4 saved steps render on the real canvas with the right
// config. THEN, on that same real (interfaceId/messageType-backed) pipeline,
// adds edi.generate_999 as a 5th step and runs it through the REAL "Test
// Pipeline" button against a real, unedited 835 sample — proving the whole
// chain: /api/pipelines/test requires a genuine interface_id/message_type
// (an ad-hoc, never-saved pipeline gets rejected with "Could not determine
// interface_id and message_type from request" — confirmed by hitting this
// directly during investigation), which only a template-created (or
// otherwise persisted) interface provides. edi.generate_999 doesn't care
// which transaction set it's acknowledging (see its own doc comment) — reusing
// the same real 835 sample the Go-level executor tests use is deliberate,
// not a stand-in for a missing 837 sample.
test('EDI X12 837 Inbound template: Use Template renders 4 steps with correct config, then edi.generate_999 produces a real 999 via Test Pipeline', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 837 Inbound' });
    await expect(card, 'Templates gallery should list the EDI X12 837 Inbound template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-837 Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 4 steps').toBe(4);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 4 steps should render as canvas nodes').toBe(4);

    const closeStepModal = async () => {
        const closeBtn = page.locator('#stepPropertiesModal .modal-close');
        if (await closeBtn.count() > 0) await closeBtn.first().click();
        await page.waitForSelector('#stepPropertiesModal', { state: 'hidden', timeout: 5000 }).catch(() => {});
    };

    const stepChecks = [
        {
            name: 'Parse 837 -> JSON',
            assert: async () => {
                await expect(page.locator('#ediParseSourceField')).toHaveValue('raw');
                await expect(page.locator('#ediParseTransactionSet')).toHaveValue('837P');
            },
        },
        {
            name: 'Validate Against X12 5010',
            assert: async () => {
                await expect(page.locator('#ediValidateSourceField')).toHaveValue('raw');
            },
        },
    ];
    for (const { name, assert } of stepChecks) {
        const node = page.locator('.flowchart-step-node', { hasText: name });
        await expect(node, `canvas should have a step node for "${name}"`).toBeVisible({ timeout: 5000 });
        await node.click();
        await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
        await assert();
        await closeStepModal();
    }
    // The 2 connector steps reuse the existing, already-covered connector
    // config UI — just confirm each opens cleanly.
    for (const name of ['Receive 837 File (SFTP)', 'Store Parsed Result']) {
        const node = page.locator('.flowchart-step-node', { hasText: name });
        await expect(node, `canvas should have a step node for "${name}"`).toBeVisible({ timeout: 5000 });
        await node.click();
        await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
        await closeStepModal();
    }

    // Now add edi.generate_999 as a 5th step on this real, persisted-interface
    // pipeline (per TransformationTestController.TestPipeline's own resolution
    // order, this reads interfaceId/messageType from window.pipelineBuilder.pipeline
    // itself — Test Pipeline explicitly supports testing unsaved in-progress
    // changes, so this step need not be saved first).
    await page.evaluate(() => {
        const step = new VisualStep({ stepName: 'Generate999', stepType: 'edi.generate_999', config: { sourceField: 'raw', outputField: 'generated999' } });
        window.pipelineBuilder.addStep(step);
    });

    const sample = fs.readFileSync(path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'blue_cross_nc_sample.txt'), 'utf8');
    await page.evaluate(() => window.pipelineBuilder.openTestModal());
    await page.waitForSelector('#testModal.active', { timeout: 5000 });
    await page.selectOption('#testMessageFormat', 'edi');
    await page.fill('#testMessageInput', sample);
    await page.click('#runTestBtn');

    await page.waitForFunction(
        () => {
            const el = document.getElementById('testResultsContent');
            return el && el.innerText.trim().length > 0 && !el.innerText.includes('Running test');
        },
        { timeout: 20000 }
    );

    const testOutput = await page.evaluate(() => window.pipelineLastTestOutput);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const genStep = testOutput.steps?.generate999;
    expect(genStep, 'generate999 step should have its own entry in the test results').toBeTruthy();
    expect(genStep.step_metadata?.success, 'edi.generate_999 step itself should succeed').toBe(true);
    expect(genStep.step_metadata?.ackCode, 'a clean real 835 sample should produce an Accepted 999').toBe('A');

    const built999 = genStep.step_output?.generated999;
    expect(built999, 'edi.generate_999 should produce real built 999 text').toBeTruthy();
    expect(built999).toContain('ST*999*');
    // AK1 echoes 835's own known FunctionalIdentifierCode/VersionReleaseIndustryCode
    // (this sample has no real envelope — envelopePresent:false — so this also
    // proves the txSet-level fallback fix for envelope-less messages fires
    // correctly through the real system, not just in the Go unit test).
    expect(built999).toContain('AK1*HP*');
    expect(built999).toContain('AK2*835*1234');
    expect(built999).toContain('IK5*A');
    expect(built999).toContain('AK9*A*1*1*1');
    // The real PER paired-rule SyntaxRule warning this exact sample carries
    // (see edi_validate_executor_test.go's own TestEDIValidateExecutor_SyntaxRuleViolation_IsWarningNeverBlocking)
    // must still surface as an IK3 entry — proving warnings are reported, not
    // silently dropped, even though they never affect AK9/IK5's Accepted code.
    expect(built999).toContain('IK3*PER*');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
