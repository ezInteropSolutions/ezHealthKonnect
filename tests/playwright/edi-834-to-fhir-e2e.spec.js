// tests/playwright/edi-834-to-fhir-e2e.spec.js
// V255's own OOB template (EDI 834 -> FHIR, Benefit Enrollment and
// Maintenance, transform-only scope), driven through the REAL "Use Template"
// click path AND a REAL "Test Pipeline" run against a genuine X12 sample --
// same discipline as edi-278-to-fhir-e2e.spec.js's own precedent.
//
// 834 is a genuinely different shape from every other transaction set this
// engine maps -- a flat, one-way member-roster feed (no HL hierarchy, no
// request/response pair). This test's own sample carries a subscriber with
// an ACTIVE health-coverage enrollment and a dependent with a TERMINATED
// one, proving the HD01 maintenance-type-code -> Coverage.status translation
// picks genuinely different real statuses from genuinely different source
// data.
//
// Sample is self-authored (edi/testdata/real_samples/self_authored_834_sample.txt)
// -- generated via edi.build itself and round-trip-verified with
// edi.ParseTransactionSet before being committed.
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

test('EDI X12 834 to FHIR template: Use Template renders 10 steps, then Test Pipeline against a real 834 sample produces Coverage resources with genuinely different translated statuses', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 834 to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 834 to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-834-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 10 steps').toBe(10);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 10 steps should render as canvas nodes').toBe(10);

    await page.locator('.flowchart-step-node', { hasText: 'Parse 834 -> JSON' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ediParseTransactionSet')).toHaveValue('834');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build Coverage' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('Coverage');
    await closeStepModal(page);

    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'self_authored_834_sample.txt'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    expect(validateStep.step_output?.errors || [], `no unexpected FHIR validation errors, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_834_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 834 FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 1, Patient: 2, Coverage: 2,
    });

    const coverages = bundle.entry.filter((e) => e.resource.resourceType === 'Coverage').map((e) => e.resource);
    const statuses = coverages.map((c) => c.status).sort();
    expect(statuses, 'the sample carries the subscriber\'s own active enrollment (HD01=024) and the dependent\'s own terminated one (HD01=030)').toEqual(['active', 'cancelled']);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
