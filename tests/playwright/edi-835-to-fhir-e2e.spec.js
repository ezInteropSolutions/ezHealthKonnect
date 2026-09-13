// tests/playwright/edi-835-to-fhir-e2e.spec.js
// V247's own OOB template (EDI 835 -> FHIR, per-claim ExplanationOfBenefit),
// driven through the REAL "Use Template" click path AND a REAL "Test
// Pipeline" run against genuine real 835 samples -- closing the same class
// of gap edi-837-to-fhir-e2e.spec.js already closed for 837P/837I: Go-level
// tests (edi_835_to_fhir_test.go) prove the executor chain in isolation, but
// never prove the whole thing works through the actual HTTP/DAG
// pipeline-execution stack and browser UI a real user drives -- including
// the "message." nesting and steps.<alias>.step_output snake-casing
// gotchas that are only observable through a real pipeline run.
const { test, expect } = require('@playwright/test');
const fs = require('fs');
const path = require('path');

const closeStepModal = async (page) => {
    const closeBtn = page.locator('#stepPropertiesModal .modal-close');
    if (await closeBtn.count() > 0) await closeBtn.first().click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'hidden', timeout: 5000 }).catch(() => {});
};

// Runs the real "Test Pipeline" flow against `sampleText` and returns the
// parsed JSON test output (window.pipelineLastTestOutput).
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

// Asserts a FHIR Bundle JSON string contains exactly the expected count of
// each resourceType, and returns the parsed Bundle for further inspection.
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

// The only 2 real, named, evidence-based gaps this template's own migration
// comment documents (no role-82 NM1/no Coverage resource in this pass) --
// matching services/executors/transform/edi_835_to_fhir_test.go's own
// assertOnlyNamedEDI835ValidationGaps. Any OTHER error is a real regression.
function unexpectedEDI835ValidationErrors(errors) {
    return (errors || []).filter((e) => {
        const s = String(e).toLowerCase();
        return !(s.includes('provider') || s.includes('insurance'));
    });
}

test('EDI X12 835 to FHIR template: Use Template renders 9 steps, then Test Pipeline against the real Blue Cross NC sample produces one EOB with a mapped item.sequence', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 835 to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 835 to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-835-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 9 steps').toBe(9);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 9 steps should render as canvas nodes').toBe(9);

    // Spot-check 2 representative steps' own real config -- Parse's
    // transactionSet (proves it's the 835-scoped template) and the new
    // Build ExplanationOfBenefit step's resourceType/rowsPath.
    await page.locator('.flowchart-step-node', { hasText: 'Parse 835 -> JSON' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ediParseSourceField')).toHaveValue('raw');
    await expect(page.locator('#ediParseTransactionSet')).toHaveValue('835');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build ExplanationOfBenefit' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('ExplanationOfBenefit');
    await closeStepModal(page);

    // Real Test Pipeline run against the real, unedited Blue Cross NC
    // sample (1 claim) -- the same one
    // TestEDI835ToFHIR_BlueCrossNC_SingleClaim_BuildsCorrectBundleShape
    // proves at the Go level, now proven through the real HTTP/DAG
    // pipeline-execution stack too.
    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'blue_cross_nc_sample.txt'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const unexpectedErrors = unexpectedEDI835ValidationErrors(validateStep.step_output?.errors);
    expect(unexpectedErrors, `no unexpected FHIR validation errors beyond the named provider/insurance gaps, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_835_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 835 FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        ExplanationOfBenefit: 1, PaymentReconciliation: 1,
    });

    const eobEntry = bundle.entry.find((e) => e.resource.resourceType === 'ExplanationOfBenefit');
    const eob = eobEntry.resource;
    expect(eob.item, 'EOB should have at least one item').toBeTruthy();
    expect(eob.item[0].sequence, 'item[0].sequence should be mapped via the new _rowIndex primitive').toBe(1);
    expect(eob.item[0].adjudication[0].category.coding[0].code).toBe('submitted');
    expect(eob.item[0].adjudication[1].category.coding[0].code).toBe('benefit');

    const prEntry = bundle.entry.find((e) => e.resource.resourceType === 'PaymentReconciliation');
    const pr = prEntry.resource;
    expect(pr.detail, 'PaymentReconciliation should have 1 detail entry (1 claim)').toHaveLength(1);
    expect(pr.detail[0].response.reference, "detail[0].response.reference should be rewritten to the EOB's own fullUrl").toBe(eobEntry.fullUrl);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

test('EDI X12 835 to FHIR template: Test Pipeline against the real eMedNY multi-claim sample produces 3 independently-sequenced EOBs', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 835 to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 835 to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-835-FHIR MultiClaim Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    // Real Test Pipeline run against the real, unedited eMedNY sample (3
    // claims) -- the same one
    // TestEDI835ToFHIR_EMedNY_MultiClaim_BuildsOneEOBPerClaim proves at the
    // Go level, now proven through the real HTTP/DAG pipeline-execution
    // stack too, including payload.builder's own array-valued resourcePaths
    // spreading each rowsPath-built EOB into its own Bundle entry.
    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'emedny_sample.txt'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const unexpectedErrors = unexpectedEDI835ValidationErrors(validateStep.step_output?.errors);
    expect(unexpectedErrors, `no unexpected FHIR validation errors beyond the named provider/insurance gaps, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_835_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 835 FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        ExplanationOfBenefit: 3, PaymentReconciliation: 1,
    });

    // Each EOB's own item[].sequence must independently start at 1 -- proves
    // _rowIndex is computed per-resource (from the OWN service_lines rowsPath
    // resolved against THAT claim row), not a single running counter leaking
    // across claims.
    const eobs = bundle.entry.filter((e) => e.resource.resourceType === 'ExplanationOfBenefit').map((e) => e.resource);
    for (const eob of eobs) {
        expect(eob.item?.[0]?.sequence, `EOB ${eob.id} item[0].sequence should be 1`).toBe(1);
    }

    const prEntry = bundle.entry.find((e) => e.resource.resourceType === 'PaymentReconciliation');
    expect(prEntry.resource.detail, 'PaymentReconciliation should have 3 detail entries (3 claims)').toHaveLength(3);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
