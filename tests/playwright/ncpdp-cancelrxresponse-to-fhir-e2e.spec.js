// tests/playwright/ncpdp-cancelrxresponse-to-fhir-e2e.spec.js
// V260's own OOB template (NCPDP SCRIPT CancelRxResponse -> FHIR Task),
// driven through the REAL "Use Template" click path AND a REAL "Test
// Pipeline" run against the self-authored Denied-outcome sample.
const { test, expect } = require('@playwright/test');
const fs = require('fs');
const path = require('path');

const closeStepModal = async (page) => {
    const closeBtn = page.locator('#stepPropertiesModal .modal-close');
    if (await closeBtn.count() > 0) await closeBtn.first().click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'hidden', timeout: 5000 }).catch(() => {});
};

const runTestPipeline = async (page, sampleText) => {
    await page.evaluate(() => window.pipelineBuilder.openTestModal());
    await page.waitForSelector('#testModal.active', { timeout: 5000 });
    await page.selectOption('#testMessageFormat', 'xml');
    await page.fill('#testMessageInput', sampleText);
    await page.click('#runTestBtn');

    await page.waitForFunction(
        () => {
            const el = document.getElementById('testResultsContent');
            return el && el.innerText.trim().length > 0 && !el.innerText.includes('Running test');
        },
        { timeout: 20000 }
    );

    return page.evaluate(() => window.pipelineLastTestOutput);
};

function parseBundleAndCount(bundleJSON, wantCounts) {
    const bundle = JSON.parse(bundleJSON);
    expect(bundle.resourceType, 'assembled payload should be a FHIR Bundle').toBe('Bundle');
    const counts = {};
    for (const entry of bundle.entry || []) {
        const rt = entry.resource.resourceType;
        counts[rt] = (counts[rt] || 0) + 1;
    }
    for (const [rt, count] of Object.entries(wantCounts)) {
        expect(counts[rt], `expected ${count} ${rt} resource(s) in the bundle, got ${counts[rt] || 0}. Full counts: ${JSON.stringify(counts)}`).toBe(count);
    }
    return bundle;
}

test('NCPDP SCRIPT CancelRxResponse to FHIR template: Use Template renders 7 steps, then Test Pipeline against the self-authored Denied sample produces a clean-validating Task', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'NCPDP SCRIPT CancelRxResponse to FHIR' });
    await expect(card, 'Templates gallery should list the NCPDP SCRIPT CancelRxResponse to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW NCPDP-CancelRxResponse-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 7 steps').toBe(7);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 7 steps should render as canvas nodes').toBe(7);

    await page.locator('.flowchart-step-node', { hasText: 'Parse Cancel Rx Response' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ncpdpParseSourceField')).toHaveValue('raw');
    await expect(page.locator('#ncpdpParseOutputField')).toHaveValue('parsedNCPDP');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build Cancel Rx Response Task' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('Task');
    await closeStepModal(page);

    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'ncpdp', 'testdata', 'self_authored', 'cancel_rx_response_denied_sample.xml'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const errors = validateStep.step_output?.errors || [];
    expect(errors, `expected zero FHIR validation errors, got: ${JSON.stringify(errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_cancel_rx_response_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble Cancel Rx Response FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, { Task: 1 });

    const task = bundle.entry.find((e) => e.resource.resourceType === 'Task').resource;
    expect(task.status, 'Task.status should be rejected for a Denied outcome').toBe('rejected');
    expect(task.businessStatus.coding[0].code).toBe('Denied');
    expect(task.note[0].text, 'Task.note[0].text should be the real note from the Denied branch').toBe('Patient already picked up medication');
    expect(task.identifier[0].value, 'Task.identifier should carry the real RequestReferenceNumber').toBe('REQ-0001');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
