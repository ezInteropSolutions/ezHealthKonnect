// tests/playwright/edi-278-to-fhir-e2e.spec.js
// V254's own OOB template (EDI 278 -> FHIR, Health Care Services Review --
// prior authorization request/response, transform-only scope -- this engine
// converts X12 <-> FHIR, it never decides a prior authorization outcome),
// driven through the REAL "Use Template" click path AND a REAL "Test
// Pipeline" run against a genuine X12 sample -- same discipline as
// edi-276-277-to-fhir-e2e.spec.js's own precedent: parse the assembled FHIR
// Bundle out of the payload.builder step's own step_output and assert on
// real resource counts/fields, not just "the run succeeded".
//
// 278 is a genuinely new shape for this engine: ONE unified schema serves
// BOTH the request and response directions (no separate ST01/GS08 the way
// every other pair in this engine uses) -- this test's own sample carries
// BOTH a pure request (the dependent's own patient event, no HCR) and a real
// response (the subscriber's own patient event, HCR present), proving a
// Claim is ALWAYS built but a ClaimResponse only for the response.
//
// Sample is self-authored (edi/testdata/real_samples/self_authored_278_sample.txt)
// -- generated via edi.build itself and round-trip-verified with
// edi.ParseTransactionSet before being committed, the same "self-authored,
// round-trip-proven" discipline already used for 270/271/276/277.
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

test('EDI X12 278 to FHIR template: Use Template renders 11 steps, then Test Pipeline against a real 278 sample produces a Claim for every patient event but a ClaimResponse only for the one with a real HCR decision', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 278 to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 278 to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-278-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 11 steps').toBe(11);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 11 steps should render as canvas nodes').toBe(11);

    await page.locator('.flowchart-step-node', { hasText: 'Parse 278 -> JSON' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ediParseTransactionSet')).toHaveValue('278');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: /Build Claim$/ }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('Claim');
    await closeStepModal(page);

    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'self_authored_278_sample.txt'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    expect(validateStep.step_output?.errors || [], `no unexpected FHIR validation errors, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_278_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 278 FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 1, Patient: 2, Claim: 2, ClaimResponse: 1,
    });

    const claims = bundle.entry.filter((e) => e.resource.resourceType === 'Claim').map((e) => e.resource);
    for (const c of claims) {
        expect(c.use).toBe('preauthorization');
    }

    const claimResponse = bundle.entry.find((e) => e.resource.resourceType === 'ClaimResponse').resource;
    expect(claimResponse.outcome, 'the sample\'s subscriber event carries HCR*A1 (Certified in total)').toBe('complete');
    expect(claimResponse.preAuthRef).toBe('AUTH99001');
    expect(claimResponse.disposition).toBe('Certified in total');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
