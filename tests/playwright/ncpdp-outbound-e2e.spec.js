// tests/playwright/ncpdp-outbound-e2e.spec.js
// Closes the browser/UI verification gap named directly by the user after
// the initial API-level full-stack proof of the FIRST genuinely OUTBOUND
// NCPDP templates (V268 SCRIPT NewRx, V269 D.0 B1): that proof replicated
// the "Use Template" flow's own API calls by hand and drove a real FHIR
// payload through a real running app over the real network — solid, but
// never through the actual browser UI, and never confirmed the step-config
// panels (built for INBOUND-shaped configs) correctly render these
// differently-shaped OUTBOUND configs (deeply nested bodyGroups, a
// validate-before-build step, literalValue-only header fields).
const { test, expect } = require('@playwright/test');

const closeStepModal = async (page) => {
    const closeBtn = page.locator('#stepPropertiesModal .modal-close');
    if (await closeBtn.count() > 0) await closeBtn.first().click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'hidden', timeout: 5000 }).catch(() => {});
};

const runTestPipeline = async (page, sampleText, format) => {
    await page.evaluate(() => window.pipelineBuilder.openTestModal());
    await page.waitForSelector('#testModal.active', { timeout: 5000 });
    await page.selectOption('#testMessageFormat', format);
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

// findStepOutput locates a step's own output by a case-insensitive substring
// match on its key (robust to the exact NormalizeKey-derived key, which this
// spec deliberately does not hardcode a guess for) and returns whichever of
// step_output/output actually holds data (the test endpoint's own response
// shape).
function findStepOutput(testOutput, substr) {
    const steps = testOutput.steps || {};
    const key = Object.keys(steps).find(k => k.toLowerCase().includes(substr.toLowerCase()));
    if (!key) return { key: null, output: null, allKeys: Object.keys(steps) };
    const stepData = steps[key];
    const output = stepData.step_output || stepData.output || {};
    return { key, output, allKeys: Object.keys(steps) };
}

const SAMPLE_MEDICATION_REQUEST = JSON.stringify({
    resourceType: 'MedicationRequest',
    id: 'e2e-ui-medreq-001',
    status: 'active',
    intent: 'order',
    subject: { reference: 'Patient/pat-ui', display: 'Alice Walker' },
    requester: {
        reference: 'Practitioner/prac-ui',
        display: 'Meredith Grey',
        identifier: { system: 'http://hl7.org/fhir/sid/us-npi', value: '5556667778' },
    },
    medicationCodeableConcept: {
        text: 'Amoxicillin 500mg Capsule',
        coding: [{ system: 'http://hl7.org/fhir/sid/ndc', code: '00093414601' }],
    },
    dosageInstruction: [{ text: 'Take one capsule three times daily' }],
    dispenseRequest: {
        quantity: { value: 21, unit: 'CAP' },
        performer: {
            reference: 'Organization/pharm-ui',
            display: 'Seattle Grace Pharmacy',
            identifier: { system: 'http://hl7.org/fhir/sid/us-npi', value: '4443332221' },
        },
    },
    authoredOn: '2026-09-20T08:00:00Z',
});

const SAMPLE_MEDICATION_DISPENSE = JSON.stringify({
    resourceType: 'MedicationDispense',
    id: 'e2e-ui-meddisp-001',
    status: 'completed',
    identifier: [{ value: 'RXUI0009988' }],
    subject: {
        reference: 'Patient/pat-ui',
        display: 'Alice Walker',
        identifier: { system: 'http://example.org/mrn', value: 'MRNUI4455' },
    },
    performer: [{
        actor: {
            reference: 'Organization/pharm-ui',
            display: 'Seattle Grace Pharmacy',
            identifier: { system: 'http://hl7.org/fhir/sid/us-npi', value: '4443332221' },
        },
    }],
    medicationCodeableConcept: {
        text: 'Amoxicillin 500mg Capsule',
        coding: [{ system: 'http://hl7.org/fhir/sid/ndc', code: '00093414601' }],
    },
    quantity: { value: 21, unit: 'CAP' },
    daysSupply: { value: 7, unit: 'd' },
    whenHandedOver: '2026-09-20T08:15:00Z',
});

test('FHIR MedicationRequest to NCPDP SCRIPT NewRx (Outbound): Use Template renders 6 steps, Test Pipeline builds a real NewRx XML', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'FHIR MedicationRequest to NCPDP SCRIPT NewRx' });
    await expect(card, 'Templates gallery should list the outbound NewRx template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW NewRx Outbound Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 6 steps (inbound, derive, map, validate, build, outbound)').toBe(6);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 6 steps should render as canvas nodes').toBe(6);

    // Open the enrichment.script step's own config panel — this is a config
    // shape (a large script + no field-mapping UI) the pipeline builder's
    // generic script-step panel already supports for every other
    // enrichment.script step in this codebase, but never yet confirmed for
    // THIS specific outbound-direction script.
    await page.locator('.flowchart-step-node', { hasText: 'Derive New Rx Outbound Fields' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await closeStepModal(page);

    // Open the ncpdp.map_to_canonical step's own config panel — the real
    // point of this check: this config has deeply nested bodyGroups
    // (patient->humanPatient->name, prescriber->nonVeterinarian->
    // identification/name, medicationPrescribed->drugCoded->productCode)
    // that NCPDPStepBuilder.js's config panel was built against INBOUND-
    // shaped configs, never confirmed against this shape until now.
    await page.locator('.flowchart-step-node', { hasText: 'Map to Canonical NewRx' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await closeStepModal(page);

    // Open the new ncpdp.validate step's own config panel.
    await page.locator('.flowchart-step-node', { hasText: 'Validate Canonical New Rx' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await closeStepModal(page);

    const testOutput = await runTestPipeline(page, SAMPLE_MEDICATION_REQUEST, 'fhir');
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const buildResult = findStepOutput(testOutput, 'build');
    expect(buildResult.key, `expected a "build" step in test output, found keys: ${JSON.stringify(buildResult.allKeys)}`).toBeTruthy();
    // The test endpoint's own step-output snapshot is passed through
    // models.OutputNormalizer.NormalizeStepOutput, which snake_cases every
    // key it doesn't already recognize as snake_case — the same
    // well-established gotcha documented throughout this codebase (e.g.
    // CLAUDE.md's own EDI 837/NCPDP write-ups) — so ncpdp.build's own
    // "ncpdpScript" outputField key lands here as "ncpdp_script".
    const xmlDoc = buildResult.output.ncpdpScript || buildResult.output.ncpdp_script;
    expect(xmlDoc, `expected ncpdpScript/ncpdp_script in build step output: ${JSON.stringify(buildResult.output)}`).toBeTruthy();
    expect(xmlDoc).toContain('NewRx');
    expect(xmlDoc).toContain('Amoxicillin 500mg Capsule');
    expect(xmlDoc).toContain('Walker');
    expect(xmlDoc).toContain('Alice');
    expect(xmlDoc).toContain('Grey');
    expect(xmlDoc).toContain('5556667778');

    const validateResult = findStepOutput(testOutput, 'validate');
    expect(validateResult.key, `expected a "validate" step in test output, found keys: ${JSON.stringify(validateResult.allKeys)}`).toBeTruthy();
    expect(validateResult.output.valid, `expected the canonical NewRx to validate cleanly: ${JSON.stringify(validateResult.output)}`).toBe(true);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

test('FHIR MedicationDispense to NCPDP D.0 B1 Claim (Outbound): Use Template renders 6 steps, Test Pipeline builds a real B1 transmission', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'FHIR MedicationDispense to NCPDP D.0 B1' });
    await expect(card, 'Templates gallery should list the outbound B1 template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW B1 Outbound Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 6 steps').toBe(6);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 6 steps should render as canvas nodes').toBe(6);

    await page.locator('.flowchart-step-node', { hasText: 'Derive B1 Outbound Fields' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await closeStepModal(page);

    // The real point of this check: ncpdptelecom.map_to_canonical's config
    // panel, confirmed (via NCPDPTelecomStepBuilder.js's own header comment)
    // to use a FLATTER shape than SCRIPT's own recursive tree (a flat field
    // list per segment, one transactionGroupRowsPath) — never confirmed
    // against a REAL 3-segment outbound config (Insurance/Patient/
    // PharmacyProvider transmission-group + Claim transaction-group) until now.
    await page.locator('.flowchart-step-node', { hasText: 'Map to Canonical B1' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Validate Canonical B1' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await closeStepModal(page);

    const testOutput = await runTestPipeline(page, SAMPLE_MEDICATION_DISPENSE, 'fhir');
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const buildResult = findStepOutput(testOutput, 'build');
    expect(buildResult.key, `expected a "build" step in test output, found keys: ${JSON.stringify(buildResult.allKeys)}`).toBeTruthy();
    const transmission = buildResult.output.ncpdpTelecom || buildResult.output.ncpdp_telecom;
    expect(transmission, `expected ncpdpTelecom/ncpdp_telecom in build step output: ${JSON.stringify(buildResult.output)}`).toBeTruthy();
    expect(transmission).toContain('999999'); // BIN
    expect(transmission).toContain('00093414601'); // NDC
    expect(transmission).toContain('4443332221'); // pharmacy NPI (header.serviceProviderId)
    expect(transmission).toContain('Walker'); // patient last name

    const validateResult = findStepOutput(testOutput, 'validate');
    expect(validateResult.key, `expected a "validate" step in test output, found keys: ${JSON.stringify(validateResult.allKeys)}`).toBeTruthy();
    // NOT expected to be fully clean — Pricing is a real, named, permanent
    // gap for this trigger resource (see V269's own header comment): a
    // MedicationDispense carries no cost/pricing data at all. Assert the
    // gap is EXACTLY the named one, not silently ignored or a NEW regression.
    const validationDetail = validateResult.output.ncpdpValidation || validateResult.output.ncpdp_validation
        || validateResult.output.telecomValidation || validateResult.output.telecom_validation || {};
    const issues = validateResult.output.issues || validationDetail.issues || [];
    const issuePaths = issues.map(i => i.path);
    expect(issuePaths, `expected exactly the named Pricing gap, got: ${JSON.stringify(issues)}`).toEqual(['TransactionGroups[0].Pricing']);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
