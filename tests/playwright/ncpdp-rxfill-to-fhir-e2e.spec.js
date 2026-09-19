// tests/playwright/ncpdp-rxfill-to-fhir-e2e.spec.js
// V265's own OOB template (NCPDP SCRIPT RxFill -> FHIR MedicationDispense),
// driven through the REAL "Use Template" click path AND a REAL "Test
// Pipeline" run against the self-authored Filled-outcome sample.
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

test('NCPDP SCRIPT RxFill to FHIR template: Use Template renders 9 steps, then Test Pipeline against the self-authored Filled sample produces a clean-validating Bundle', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'NCPDP SCRIPT RxFill to FHIR' });
    await expect(card, 'Templates gallery should list the NCPDP SCRIPT RxFill to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW NCPDP-RxFill-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 9 steps').toBe(9);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 9 steps should render as canvas nodes').toBe(9);

    await page.locator('.flowchart-step-node', { hasText: 'Parse Rx Fill' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ncpdpParseSourceField')).toHaveValue('raw');
    await expect(page.locator('#ncpdpParseOutputField')).toHaveValue('parsedNCPDP');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build Medication Dispense' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('MedicationDispense');
    await closeStepModal(page);

    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'ncpdp', 'testdata', 'self_authored', 'rx_fill_filled_sample.xml'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const errors = validateStep.step_output?.errors || [];
    expect(errors, `expected zero FHIR validation errors, got: ${JSON.stringify(errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_rx_fill_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble Rx Fill FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 1, Patient: 1, MedicationDispense: 1,
    });

    const org = bundle.entry.find((e) => e.resource.resourceType === 'Organization').resource;
    expect(org.name).toBe('A+ Drugs');

    const patientEntry = bundle.entry.find((e) => e.resource.resourceType === 'Patient');
    expect(patientEntry.resource.name[0].family).toBe('Jenny');
    expect(patientEntry.resource.gender).toBe('female');

    const orgEntry = bundle.entry.find((e) => e.resource.resourceType === 'Organization');

    const medDispEntry = bundle.entry.find((e) => e.resource.resourceType === 'MedicationDispense');
    const medDisp = medDispEntry.resource;
    expect(medDisp.status, 'MedicationDispense.status should be completed for a Filled outcome').toBe('completed');
    expect(medDisp.medicationCodeableConcept.text).toBe('Ondansetron 8 mg Tab Disintegrating');
    expect(medDisp.identifier[0].value, 'MedicationDispense.identifier should carry the real fill reference number').toBe('REQ-0004');
    expect(medDisp.note[0].text).toBe('Dispensed as written');
    expect(medDisp.subject.reference, "MedicationDispense.subject.reference should be rewritten to the real Patient entry's own fullUrl").toBe(patientEntry.fullUrl);
    expect(medDisp.performer[0].actor.reference, "MedicationDispense.performer[0].actor.reference should be rewritten to the real Organization entry's own fullUrl").toBe(orgEntry.fullUrl);
    expect(medDisp.quantity.value).toBe(15);
    expect(medDisp.whenHandedOver).toBe('2026-09-19');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
