// tests/playwright/ncpdp-telecom-b1-response-to-fhir-e2e.spec.js
// V267's own OOB template (NCPDP Telecommunication D.0 B1 Claim Billing
// Response -> FHIR, real-time pharmacy claims Phase 1), driven through the
// REAL "Use Template" click path AND a REAL "Test Pipeline" run against a
// self-authored B1 response sample -- mirrors
// ncpdp-telecom-b1-request-to-fhir-e2e.spec.js's own structure exactly (see
// that file's header comment for the full rationale).
const { test, expect } = require('@playwright/test');

const closeStepModal = async (page) => {
    const closeBtn = page.locator('#stepPropertiesModal .modal-close');
    if (await closeBtn.count() > 0) await closeBtn.first().click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'hidden', timeout: 5000 }).catch(() => {});
};

const runTestPipeline = async (page, sampleText) => {
    await page.evaluate(() => window.pipelineBuilder.openTestModal());
    await page.waitForSelector('#testModal.active', { timeout: 5000 });
    await page.selectOption('#testMessageFormat', 'auto');
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

// Same self-authored fixture construction as
// services/ncpdp_telecom_b1_response_fhir_builder_test.go's own
// buildSelfAuthoredB1Response(). AM21's own "P" response status code
// (Paid) drives outcome="complete" per the template's own condition-based
// outcome mapping.
function buildSelfAuthoredB1Response() {
    const rs = String.fromCharCode(0x1e);
    const fs = String.fromCharCode(0x1c);
    const header = '999999' + 'D0' + 'B1' +
        '          ' +
        '1' + '01' +
        '1111111111     ' +
        '20260919' +
        '          ';
    const body = rs + fs + 'AM20' + fs + 'F4Claim processed successfully' +
        rs + fs + 'AM21' + fs + 'ANP' + fs + 'F31234567890' + fs + 'K5REQ-0005' +
        rs + fs + 'AM23' + fs + 'F50000084F' + fs + 'F60000057A' + fs + 'F70000027E' + fs + 'F90000084F' + fs + 'FI0000016B';
    return header + body;
}

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

test('NCPDP Telecom D.0 B1 Response to FHIR template: Use Template renders 8 steps, then Test Pipeline against a self-authored B1 response produces a clean-validating Bundle', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'NCPDP Telecom D.0 B1 Claim Response to FHIR' });
    await expect(card, 'Templates gallery should list the NCPDP Telecom D.0 B1 Claim Response to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW NCPDP-Telecom-B1-Response-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 8 steps').toBe(8);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 8 steps should render as canvas nodes').toBe(8);

    await page.locator('.flowchart-step-node', { hasText: 'Parse B1 Response' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ncpdpTelecomParseSourceField')).toHaveValue('raw');
    await expect(page.locator('#ncpdpTelecomParseDirection')).toHaveValue('response');
    await expect(page.locator('#ncpdpTelecomParseOutputField')).toHaveValue('parsedTelecom');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build Claim Response' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('ClaimResponse');
    await closeStepModal(page);

    const sample = buildSelfAuthoredB1Response();
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const errors = validateStep.step_output?.errors || [];
    expect(errors, `expected zero FHIR validation errors, got: ${JSON.stringify(errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_b1_response_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble B1 Response FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        ClaimResponse: 1,
    });

    const cr = bundle.entry.find((e) => e.resource.resourceType === 'ClaimResponse').resource;
    expect(cr.status).toBe('active');
    expect(cr.outcome, 'outcome should be "complete" for the real Paid (P) response status code').toBe('complete');
    expect(cr.identifier[0].value, 'identifier[0].value should be the real transaction reference number').toBe('REQ-0005');
    expect(cr.disposition, 'disposition should be the real authorization number').toBe('1234567890');
    // No Patient resource exists in this response-only Bundle -- patient is
    // a DISPLAY-ONLY Reference (no .reference), the same logical-reference
    // pattern already used for Claim.careTeam[].provider in the EDI 837
    // work, confirmed here at the real pipeline-execution level too.
    expect(cr.patient.reference).toBeUndefined();
    expect(cr.patient.display).toBeTruthy();

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
