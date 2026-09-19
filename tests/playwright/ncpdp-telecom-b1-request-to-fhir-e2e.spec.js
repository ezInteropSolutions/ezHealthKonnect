// tests/playwright/ncpdp-telecom-b1-request-to-fhir-e2e.spec.js
// V266's own OOB template (NCPDP Telecommunication D.0 B1 Claim Billing
// Request -> FHIR, real-time pharmacy claims Phase 1), driven through the
// REAL "Use Template" click path AND a REAL "Test Pipeline" run against a
// self-authored B1 request sample -- closing the same class of gap
// ncpdp-newrx-to-fhir-e2e.spec.js/edi-835-to-fhir-e2e.spec.js already closed
// for NCPDP SCRIPT/EDI X12: Go-level tests
// (services/ncpdp_telecom_b1_request_fhir_builder_test.go) prove the
// executor chain in isolation, but never prove the whole thing works
// through the actual HTTP/DAG pipeline-execution stack and browser UI a
// real user drives.
const { test, expect } = require('@playwright/test');

const closeStepModal = async (page) => {
    const closeBtn = page.locator('#stepPropertiesModal .modal-close');
    if (await closeBtn.count() > 0) await closeBtn.first().click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'hidden', timeout: 5000 }).catch(() => {});
};

// Runs the real "Test Pipeline" flow against `sampleText` and returns the
// parsed JSON test output (window.pipelineLastTestOutput). sampleText is
// built with String.fromCharCode-based control characters (RS/FS) — Playwright's
// fill() sets the textarea's .value property directly (no keyboard
// emulation), so these survive untouched into the real HTTP request body.
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

function buildSelfAuthoredB1Request() {
    const rs = String.fromCharCode(0x1e);
    const fs = String.fromCharCode(0x1c);
    const header = '999999' + 'D0' + 'B1' +
        '          ' + // processorControlNumber
        '1' + '01' +
        '1111111111     ' + // serviceProviderId (15)
        '20260919' +
        '          '; // software (10)
    const body = rs + fs + 'AM01' + fs + 'CBSMITH' + fs + 'CAJOHN' + fs + 'C419800101' + fs + 'C52' +
        rs + fs + 'AM02' + fs + 'EY01' + fs + 'E91234567890' +
        rs + fs + 'AM04' + fs + 'C2123456789012' +
        rs + fs + 'AM07' + fs + 'D2000000123456' + fs + 'E103' + fs + 'D700003089421' + fs + 'E70000030000' + fs + 'D301' + fs + 'D5030' + fs + 'DE20220924' +
        rs + fs + 'AM11' + fs + 'D90000057A' + fs + 'DC0000027E' + fs + 'DX0000016B' + fs + 'DQ00000000' + fs + 'DU0000084F';
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

test('NCPDP Telecom D.0 B1 Request to FHIR template: Use Template renders 10 steps, then Test Pipeline against a self-authored B1 request produces a clean-validating Bundle', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'NCPDP Telecom D.0 B1 Claim Request to FHIR' });
    await expect(card, 'Templates gallery should list the NCPDP Telecom D.0 B1 Claim Request to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW NCPDP-Telecom-B1-Request-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 10 steps').toBe(10);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 10 steps should render as canvas nodes').toBe(10);

    // Spot-check 2 representative steps' own real config -- Parse's
    // sourceField/direction/outputField (proves the template's own step
    // config landed correctly) and the Claim build step's resourceType.
    await page.locator('.flowchart-step-node', { hasText: 'Parse B1 Request' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ncpdpTelecomParseSourceField')).toHaveValue('raw');
    await expect(page.locator('#ncpdpTelecomParseDirection')).toHaveValue('request');
    await expect(page.locator('#ncpdpTelecomParseOutputField')).toHaveValue('parsedTelecom');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build Claim' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('Claim');
    await closeStepModal(page);

    // Real Test Pipeline run against a self-authored B1 request sample (no
    // free, real-world NCPDP D.0 sample exists -- same "self-author against
    // confirmed field shapes" discipline established for this whole phase).
    const sample = buildSelfAuthoredB1Request();
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const errors = validateStep.step_output?.errors || [];
    expect(errors, `expected zero FHIR validation errors, got: ${JSON.stringify(errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_b1_request_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble B1 Request FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 1, Patient: 1, Claim: 1,
    });

    const orgEntry = bundle.entry.find((e) => e.resource.resourceType === 'Organization');
    expect(orgEntry.resource.name).toBe('Pharmacy');

    const patientEntry = bundle.entry.find((e) => e.resource.resourceType === 'Patient');
    const patient = patientEntry.resource;
    expect(patient.name[0].family, "Patient.name[0].family should be the real patient's own last name").toBe('SMITH');
    // The sample's own C5 (patient gender code) field is "2" -- confirmed
    // directly against buildSelfAuthoredB1Request()'s own "...C52" segment
    // content -- which the template maps to "female", not "male".
    expect(patient.gender, 'Patient.gender should be mapped from the real D.0 gender code').toBe('female');

    const claimEntry = bundle.entry.find((e) => e.resource.resourceType === 'Claim');
    const claim = claimEntry.resource;
    expect(claim.status).toBe('active');
    expect(claim.type.coding[0].code).toBe('pharmacy');
    expect(claim.identifier[0].value, 'Claim.identifier[0].value should be the real prescription reference number').toBe('000000123456');
    expect(claim.item[0].productOrService.coding[0].code).toBe('00003089421');
    expect(claim.item[0].quantity.value).toBe(30);
    expect(claim.item[0].net.value).toBe(8.46);
    // payload.builder rewrites every in-bundle Reference to the REAL fullUrl
    // it assigned that resource -- same mechanism the NCPDP SCRIPT/EDI
    // templates already rely on. Claim.patient/provider use LITERAL
    // "Patient/patient-1"/"Organization/organization-pharmacy-1" reference
    // strings that match the fixed literal ids used for those resources, so
    // they resolve to the real fullUrls the same way.
    expect(claim.patient.reference, "Claim.patient.reference should be rewritten to the real Patient entry's own fullUrl").toBe(patientEntry.fullUrl);
    expect(claim.provider.reference, "Claim.provider.reference should be rewritten to the real Organization entry's own fullUrl").toBe(orgEntry.fullUrl);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
