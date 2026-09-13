// tests/playwright/edi-270-271-to-fhir-e2e.spec.js
// V242/V243's own OOB templates (EDI 270/271 -> FHIR, Phase 3 real-time
// eligibility, transform-only scope), driven through the REAL "Use Template"
// click path AND a REAL "Test Pipeline" run against a genuine X12 sample --
// same discipline as edi-837-to-fhir-e2e.spec.js's own precedent: parse the
// assembled FHIR Bundle out of the payload.builder step's own step_output and
// assert on real resource counts/fields, not just "the run succeeded".
//
// Samples are self-authored (edi/testdata/real_samples/self_authored_27{0,1}_sample.txt)
// -- generated via edi.build itself and round-trip-verified with
// edi.ParseTransactionSet before being committed, the same "self-authored,
// round-trip-proven" discipline already used for the multi-billing-provider
// 837P sample earlier this session (no external 270/271 sample was available
// to source from X12.org for this phase).
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
    await page.selectOption('#testMessageFormat', 'edi');
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

test('EDI X12 270 to FHIR template: Use Template renders 11 steps, then Test Pipeline against a real 270 sample produces a valid CoverageEligibilityRequest Bundle', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 270 to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 270 to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-270-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 11 steps').toBe(11);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 11 steps should render as canvas nodes').toBe(11);

    // Spot-check the Parse step's own real config -- proves it's the
    // 270-scoped template, not a generic/blank one.
    await page.locator('.flowchart-step-node', { hasText: 'Parse 270 -> JSON' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ediParseTransactionSet')).toHaveValue('270');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build CoverageEligibilityRequest' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('CoverageEligibilityRequest');
    await closeStepModal(page);

    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'self_authored_270_sample.txt'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    expect(validateStep.step_output?.errors || [], `no unexpected FHIR validation errors, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_270_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 270 FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 2, Patient: 1, CoverageEligibilityRequest: 1,
    });
    const request = bundle.entry.find((e) => e.resource.resourceType === 'CoverageEligibilityRequest').resource;
    expect(request.status).toBe('active');
    expect(request.purpose).toEqual(['benefits']);
    expect(request.item, 'the sample asks about exactly 1 service type (30)').toHaveLength(1);
    expect(request.item[0].category.coding[0].code).toBe('30');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

test('EDI X12 271 to FHIR template: Use Template renders 9 steps, then Test Pipeline against a real 271 sample produces a valid CoverageEligibilityResponse Bundle with a real nested benefit', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 271 to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 271 to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-271-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 9 steps').toBe(9);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 9 steps should render as canvas nodes').toBe(9);

    await page.locator('.flowchart-step-node', { hasText: 'Parse 271 -> JSON' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ediParseTransactionSet')).toHaveValue('271');
    await closeStepModal(page);

    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'self_authored_271_sample.txt'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    expect(validateStep.step_output?.errors || [], `no unexpected FHIR validation errors, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_271_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 271 FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Patient: 1, CoverageEligibilityResponse: 1,
    });
    const response = bundle.entry.find((e) => e.resource.resourceType === 'CoverageEligibilityResponse').resource;
    expect(response.outcome, 'the sample carries a real active-coverage answer, not a rejection').toBe('complete');
    expect(response.insurance, 'expected exactly 1 insurance entry').toHaveLength(1);
    expect(response.insurance[0].item, 'expected exactly 1 item (the real EB occurrence)').toHaveLength(1);
    const item0 = response.insurance[0].item[0];
    expect(item0.category.coding[0].code).toBe('30');
    expect(item0.benefit, 'expected exactly 1 benefit').toHaveLength(1);
    expect(item0.benefit[0].type.coding[0].code, 'EB01=1 (active coverage)').toBe('1');
    expect(item0.benefit[0].allowedMoney.value, 'EB07=25.00').toBe('25.00');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
