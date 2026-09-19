// tests/playwright/edi-276-277-to-fhir-e2e.spec.js
// V251/V252's own OOB templates (EDI 276/277 -> FHIR, claim status
// request/response, transform-only scope -- this engine converts X12 <->
// FHIR, it never decides claim status facts), driven through the REAL
// "Use Template" click path AND a REAL "Test Pipeline" run against a
// genuine X12 sample -- same discipline as edi-270-271-to-fhir-e2e.spec.js's
// own precedent: parse the assembled FHIR Bundle out of the payload.builder
// step's own step_output and assert on real resource counts/fields, not
// just "the run succeeded".
//
// Samples are self-authored (edi/testdata/real_samples/self_authored_27{6,7}_sample.txt)
// -- the same "self-authored, sufficient at this complexity level" bar
// 270/271 already established (no external 276/277 sample was needed).
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

test('EDI X12 276 to FHIR template: Use Template renders 10 steps, then Test Pipeline against a real 276 sample produces a valid Task Bundle for both a subscriber and dependent claim inquiry', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 276 to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 276 to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-276-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 10 steps').toBe(10);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 10 steps should render as canvas nodes').toBe(10);

    // Spot-check the Parse and Build Task steps' own real config -- proves
    // it's the 276-scoped template, not a generic/blank one.
    await page.locator('.flowchart-step-node', { hasText: 'Parse 276 -> JSON' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ediParseTransactionSet')).toHaveValue('276');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build Task' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('Task');
    await closeStepModal(page);

    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'self_authored_276_sample.txt'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    expect(validateStep.step_output?.errors || [], `no unexpected FHIR validation errors, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_276_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 276 FHIR Bundle should produce a payload string').toBeTruthy();
    // Organization is built via the SAME rowsPath as Patient/Task (one per
    // claim status context, matching V242's own 270 template precedent), so
    // the sample's 2 contexts (subscriber + dependent, same real-world
    // payer) produce 2 Organization entries with identical content -- not
    // deduplicated, same "harmless per FHIR Bundle semantics" precedent
    // 837P/837I's own Patient/Coverage mapping already established.
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 2, Patient: 2, Task: 2,
    });
    const tasks = bundle.entry.filter((e) => e.resource.resourceType === 'Task').map((e) => e.resource);
    for (const t of tasks) {
        expect(t.status).toBe('requested');
        expect(t.intent).toBe('order');
    }
    const identifiers = tasks.map((t) => t.identifier?.[0]?.value).sort();
    expect(identifiers, 'the sample carries the subscriber\'s own PCN0001 and the dependent\'s own PCN0002').toEqual(['PCN0001', 'PCN0002']);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

test('EDI X12 277 to FHIR template: Use Template renders 10 steps, then Test Pipeline against a real 277 sample produces a valid Task Bundle with genuinely different translated statuses', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 277 to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 277 to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-277-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 10 steps').toBe(10);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 10 steps should render as canvas nodes').toBe(10);

    await page.locator('.flowchart-step-node', { hasText: 'Parse 277 -> JSON' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ediParseTransactionSet')).toHaveValue('277');
    await closeStepModal(page);

    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'self_authored_277_sample.txt'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    expect(validateStep.step_output?.errors || [], `no unexpected FHIR validation errors, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_277_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 277 FHIR Bundle should produce a payload string').toBeTruthy();
    // Same not-deduplicated-per-context Organization behavior as the 276
    // test above -- see that test's own comment.
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 2, Patient: 2, Task: 2,
    });
    const tasks = bundle.entry.filter((e) => e.resource.resourceType === 'Task').map((e) => e.resource);

    // The sample's subscriber claim carries STC F1:1 (finalized/paid) and the
    // dependent's own carries STC A2:35 (pending) -- genuinely different real
    // statuses, proving the STC -> Task.status translation isn't a hardcoded
    // constant.
    const finalized = tasks.find((t) => t.identifier?.[0]?.value === 'PAYERCLM001');
    const pending = tasks.find((t) => t.identifier?.[0]?.value === 'PAYERCLM002');
    expect(finalized, 'expected a Task carrying the subscriber\'s own payer claim control number PAYERCLM001').toBeTruthy();
    expect(pending, 'expected a Task carrying the dependent\'s own payer claim control number PAYERCLM002').toBeTruthy();
    expect(finalized.status).toBe('completed');
    expect(finalized.businessStatus.coding[0].code).toBe('F1');
    expect(pending.status).toBe('accepted');
    expect(pending.businessStatus.coding[0].code).toBe('A2');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
