// tests/playwright/ncpdp-cancelrx-to-fhir-e2e.spec.js
// V259's own OOB template (NCPDP SCRIPT CancelRx -> FHIR), driven through the
// REAL "Use Template" click path AND a REAL "Test Pipeline" run against the
// self-authored CancelRx sample -- mirrors ncpdp-newrx-to-fhir-e2e.spec.js's
// own discipline exactly.
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

test('NCPDP SCRIPT CancelRx to FHIR template: Use Template renders 10 steps, then Test Pipeline against the self-authored CancelRx sample produces a clean-validating Bundle', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'NCPDP SCRIPT CancelRx to FHIR' });
    await expect(card, 'Templates gallery should list the NCPDP SCRIPT CancelRx to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW NCPDP-CancelRx-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 10 steps').toBe(10);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 10 steps should render as canvas nodes').toBe(10);

    await page.locator('.flowchart-step-node', { hasText: 'Parse Cancel Rx' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ncpdpParseSourceField')).toHaveValue('raw');
    await expect(page.locator('#ncpdpParseOutputField')).toHaveValue('parsedNCPDP');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build Medication Request' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('MedicationRequest');
    await closeStepModal(page);

    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'ncpdp', 'testdata', 'self_authored', 'cancel_rx_sample.xml'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const errors = validateStep.step_output?.errors || [];
    expect(errors, `expected zero FHIR validation errors, got: ${JSON.stringify(errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_cancel_rx_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble Cancel Rx FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 1, Patient: 1, Practitioner: 1, MedicationRequest: 1,
    });

    const org = bundle.entry.find((e) => e.resource.resourceType === 'Organization').resource;
    expect(org.name, "Organization.name should be the real pharmacy's own name").toBe('A+ Drugs');

    const patientEntry = bundle.entry.find((e) => e.resource.resourceType === 'Patient');
    const patient = patientEntry.resource;
    expect(patient.name[0].family).toBe('Jenny');
    expect(patient.gender).toBe('female');

    const practitionerEntry = bundle.entry.find((e) => e.resource.resourceType === 'Practitioner');
    expect(practitionerEntry.resource.name[0].family).toBe('Bless');

    const medReqEntry = bundle.entry.find((e) => e.resource.resourceType === 'MedicationRequest');
    const medReq = medReqEntry.resource;
    expect(medReq.status, 'MedicationRequest.status should be cancelled for a CancelRx').toBe('cancelled');
    expect(medReq.intent).toBe('order');
    expect(medReq.identifier[0].value, 'MedicationRequest.identifier should carry the real RequestReferenceNumber').toBe('REQ-0001');
    expect(medReq.medicationCodeableConcept.text).toBe('Ondansetron 8 mg Tab Disintegrating');
    expect(medReq.subject.reference, "MedicationRequest.subject.reference should be rewritten to the real Patient entry's own fullUrl").toBe(patientEntry.fullUrl);
    expect(medReq.requester.reference, "MedicationRequest.requester.reference should be rewritten to the real Practitioner entry's own fullUrl").toBe(practitionerEntry.fullUrl);
    expect(medReq.dispenseRequest.quantity.value).toBe(15);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
