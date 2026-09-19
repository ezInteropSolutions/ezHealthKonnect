// tests/playwright/ncpdp-newrx-to-fhir-e2e.spec.js
// V258's own OOB template (NCPDP SCRIPT NewRx -> FHIR, pharmacy e-prescribing
// Phase 1), driven through the REAL "Use Template" click path AND a REAL
// "Test Pipeline" run against a genuine, unedited SCRIPT v2017071 NewRx
// sample -- closing the same class of gap edi-835-to-fhir-e2e.spec.js/
// edi-837-to-fhir-e2e.spec.js already closed for EDI X12: Go-level tests
// (services/ncpdp_newrx_fhir_builder_test.go) prove the executor chain in
// isolation, but never prove the whole thing works through the actual
// HTTP/DAG pipeline-execution stack and browser UI a real user drives.
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

test('NCPDP SCRIPT NewRx to FHIR template: Use Template renders 10 steps, then Test Pipeline against the real dgoradia NewRx sample produces a clean-validating Bundle', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'NCPDP SCRIPT NewRx to FHIR' });
    await expect(card, 'Templates gallery should list the NCPDP SCRIPT NewRx to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW NCPDP-NewRx-FHIR Test ${Date.now()}`);
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
    // sourceField/outputField (proves the template's own step config landed
    // correctly) and the MedicationRequest build step's resourceType.
    await page.locator('.flowchart-step-node', { hasText: 'Parse New Rx' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ncpdpParseSourceField')).toHaveValue('raw');
    await expect(page.locator('#ncpdpParseOutputField')).toHaveValue('parsedNCPDP');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build Medication Request' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('MedicationRequest');
    await closeStepModal(page);

    // Real Test Pipeline run against the real, unedited SCRIPT v2017071
    // NewRx sample (sourced from dgoradia/ncpdp's own test fixtures) -- the
    // same one TestNCPDPNewRxFHIRBuilder_RealSample_BuildsCleanValidatingBundle
    // proves at the Go level, now proven through the real HTTP/DAG
    // pipeline-execution stack too.
    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'ncpdp', 'testdata', 'real_samples', 'dgoradia_sample_newrx.xml'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const errors = validateStep.step_output?.errors || [];
    expect(errors, `expected zero FHIR validation errors, got: ${JSON.stringify(errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_new_rx_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble New Rx FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 1, Patient: 1, Practitioner: 1, MedicationRequest: 1,
    });

    const org = bundle.entry.find((e) => e.resource.resourceType === 'Organization').resource;
    expect(org.name, "Organization.name should be the real pharmacy's own name").toBe('A+ Drugs');

    const patientEntry = bundle.entry.find((e) => e.resource.resourceType === 'Patient');
    const patient = patientEntry.resource;
    expect(patient.name[0].family, "Patient.name[0].family should be the real patient's own last name").toBe('Jenny');
    expect(patient.gender, 'Patient.gender should be mapped from the real NCPDP M/F/U code').toBe('female');
    expect(patient.birthDate).toBe('1984-09-09');

    const practitionerEntry = bundle.entry.find((e) => e.resource.resourceType === 'Practitioner');
    const practitioner = practitionerEntry.resource;
    expect(practitioner.name[0].family, "Practitioner.name[0].family should be the real prescriber's own last name").toBe('Bless');

    const medReqEntry = bundle.entry.find((e) => e.resource.resourceType === 'MedicationRequest');
    const medReq = medReqEntry.resource;
    expect(medReq.status).toBe('active');
    expect(medReq.intent).toBe('order');
    expect(medReq.medicationCodeableConcept.text, 'medicationCodeableConcept.text should be the real drug description').toBe('Ondansetron 8 mg Tab Disintegrating');
    // payload.builder rewrites every in-bundle Reference to the REAL fullUrl
    // it assigned that resource (same "correctly cross-referenced fullUrls"
    // mechanism the EDI 835/837 templates already rely on) -- the computed
    // "Patient/patient-<id>" string fhir.build wrote is only the LOOKUP key
    // that rewrite matches against, not the final on-wire value.
    expect(medReq.subject.reference, "MedicationRequest.subject.reference should be rewritten to the real Patient entry's own fullUrl").toBe(patientEntry.fullUrl);
    expect(medReq.requester.reference, "MedicationRequest.requester.reference should be rewritten to the real Practitioner entry's own fullUrl").toBe(practitionerEntry.fullUrl);
    expect(medReq.dispenseRequest.quantity.value).toBe(15);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
