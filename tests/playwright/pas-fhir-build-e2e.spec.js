// tests/playwright/pas-fhir-build-e2e.spec.js
// V91/V212's own "Da Vinci Prior Authorization (PAS)" OOB template, driven
// through the REAL "Use Template" click path AND a REAL "Test Pipeline" run
// against a genuine PA-request JSON body -- proving the fix for a real, live
// bug found this session: FieldMappingExecutor's own "pas_envelope_mapping"
// step wrote its mapped fields as a FLAT map with a literal dotted-string
// key (e.g. "_pas_envelope.patient.firstName" as ONE key), which (a) no
// downstream fhir.build/enrichment.script consumer was ever written to
// read, and (b) got silently mangled by models.OutputNormalizer's own
// key-normalization pass (which treats "." as a character to strip, not a
// path separator to preserve) once captured into that step's own
// "steps.map_to_pas_envelope.step_output" snapshot. Combined with the same
// "steps.<alias>.step_output" addressing requirement found for EDI 837 (see
// edi-837-to-fhir-e2e.spec.js's own header comment), every FHIR resource
// this template built in a real pipeline run was previously missing ALL of
// its real patient/provider/claim data -- only literalValue fields survived.
// The existing services/executors/enrichment/pas_integration_test.go suite
// never caught this because it deliberately bypasses FieldMappingExecutor
// for its own "Zone 1" step, hand-injecting an already-correct _pas_envelope
// shape instead of calling the real executor.
const { test, expect } = require('@playwright/test');

test('Da Vinci PAS template: Use Template renders correctly, then Test Pipeline against a real PA request produces a valid Claim/Patient/Coverage/Practitioner/Organization Bundle', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'Da Vinci Prior Authorization' });
    await expect(card, 'Templates gallery should list the Da Vinci PAS template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW PAS Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 15 steps').toBe(15);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 15 steps should render as canvas nodes').toBe(15);

    // Configure the "Map to PAS Envelope" step's own rhs mappings to point
    // at a flat test JSON body -- this is the guided, user-facing config
    // step a real user would fill in via the pipeline builder's own field
    // mapping UI; setting it programmatically here exercises the exact same
    // step.config.mappings shape the real UI writes.
    const rhsByLHS = {
        '_pas_envelope.patient.firstName': 'body.patientFirstName',
        '_pas_envelope.patient.lastName': 'body.patientLastName',
        '_pas_envelope.patient.dob': 'body.patientDob',
        '_pas_envelope.patient.gender': 'body.patientGender',
        '_pas_envelope.patient.memberId': 'body.memberId',
        '_pas_envelope.coverage.payerId': 'body.payerId',
        '_pas_envelope.coverage.planId': 'body.planId',
        '_pas_envelope.coverage.groupNumber': 'body.groupNumber',
        '_pas_envelope.provider.npi': 'body.providerNpi',
        '_pas_envelope.provider.firstName': 'body.providerFirstName',
        '_pas_envelope.provider.lastName': 'body.providerLastName',
        '_pas_envelope.provider.facilityNpi': 'body.facilityNpi',
        '_pas_envelope.request.serviceCode': 'body.serviceCode',
        '_pas_envelope.request.diagnosisCodes': 'body.diagnosisCodes',
        '_pas_envelope.request.urgency': 'body.urgency',
        '_pas_envelope.request.serviceStartDate': 'body.serviceStartDate',
        '_pas_envelope.request.serviceEndDate': 'body.serviceEndDate',
        '_pas_envelope.request.quantity': 'body.quantity',
    };
    await page.evaluate((rhsByLHS) => {
        const steps = window.pipelineBuilder.getAllStepsFlat();
        const envStep = steps.find(s => s.stepAlias === 'pas_envelope' || s.stepType === 'pas_envelope_mapping');
        for (const m of envStep.config.mappings) {
            if (rhsByLHS[m.lhs]) m.rhs = rhsByLHS[m.lhs];
        }
    }, rhsByLHS);

    const testBody = {
        patientFirstName: 'Jane',
        patientLastName: 'Smith',
        patientDob: '1975-03-22',
        patientGender: 'female',
        memberId: 'MEM-00123',
        payerId: '1234567890',
        planId: 'GOLD-PPO',
        groupNumber: 'GRP-999',
        providerNpi: '9876543210',
        providerFirstName: 'Alice',
        providerLastName: 'Johnson',
        facilityNpi: '1111111111',
        serviceCode: '99213',
        diagnosisCodes: ['Z00.00', 'J06.9'],
        urgency: 'routine',
        serviceStartDate: '2026-05-01',
        serviceEndDate: '2026-05-01',
        quantity: '1',
    };

    await page.evaluate(() => window.pipelineBuilder.openTestModal());
    await page.waitForSelector('#testModal.active', { timeout: 5000 });
    await page.selectOption('#testMessageFormat', 'json');
    await page.fill('#testMessageInput', JSON.stringify({ body: testBody }));
    await page.click('#runTestBtn');
    await page.waitForFunction(
        () => {
            const el = document.getElementById('testResultsContent');
            return el && el.innerText.trim().length > 0 && !el.innerText.includes('Running test');
        },
        { timeout: 20000 }
    );
    const testOutput = await page.evaluate(() => window.pipelineLastTestOutput);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    // The direct proof of the fix: Zone 1 (field_mapping) now produces a
    // REAL, nested _pas_envelope, reachable and correctly keyed.
    const envelopeStep = testOutput.steps?.map_to_pas_envelope;
    expect(envelopeStep, 'map_to_pas_envelope step should have its own entry in the test results').toBeTruthy();
    const envelope = envelopeStep.step_output?._pas_envelope;
    expect(envelope, 'field_mapping should now produce a real nested _pas_envelope object, not a mangled flat key').toBeTruthy();
    expect(envelope.patient?.first_name).toBe('Jane');
    expect(envelope.patient?.last_name).toBe('Smith');
    expect(envelope.provider?.first_name).toBe('Alice');
    expect(envelope.provider?.last_name).toBe('Johnson');

    const deriveStep = testOutput.steps?.derive_pas_computed_fields;
    expect(deriveStep, 'derive_pas_computed_fields step should have its own entry').toBeTruthy();
    const derived = deriveStep.step_output?._pas_derived;
    expect(derived?.gender).toBe('female');
    expect(derived?.diagnosis_codes).toHaveLength(2);

    // The real acid test: every built FHIR resource now actually contains
    // real data, not just literalValue fields.
    const patientOut = testOutput.steps?.build_fhir_patient?.step_output?.fhir_resource;
    expect(patientOut?.id, 'Patient.id should come from the real memberId, not be missing').toBe('MEM-00123');
    expect(patientOut?.name?.[0]?.family).toBe('Smith');
    expect(patientOut?.name?.[0]?.given?.[0]).toBe('Jane');
    expect(patientOut?.birthDate).toBe('1975-03-22');
    expect(patientOut?.gender).toBe('female');

    // Coverage/Practitioner/Organization.identifier are genuine FHIR arrays
    // (0..*) -- the real build_*_fhir configs write "identifier[0].value",
    // so assert the array form here too (Claim.insurer.identifier below is
    // correctly singular -- Reference.identifier is 0..1, not an array).
    const coverageOut = testOutput.steps?.build_fhir_coverage?.step_output?.fhir_resource;
    expect(coverageOut?.identifier?.[0]?.value).toBe('1234567890');
    expect(coverageOut?.subscriberId).toBe('MEM-00123');
    expect(coverageOut?.class, 'Coverage.class should have plan+group entries from coverage_classes').toHaveLength(2);

    const practitionerOut = testOutput.steps?.build_fhir_practitioner?.step_output?.fhir_resource;
    expect(practitionerOut?.identifier?.[0]?.value).toBe('9876543210');
    expect(practitionerOut?.name?.[0]?.family).toBe('Johnson');
    expect(practitionerOut?.name?.[0]?.given?.[0]).toBe('Alice');

    const organizationOut = testOutput.steps?.build_fhir_organization?.step_output?.fhir_resource;
    expect(organizationOut?.identifier?.[0]?.value).toBe('1111111111');
    // organizationName isn't collected by the real guided mapping -- falls
    // through to the derived "Alice Johnson" full-name tier.
    expect(organizationOut?.name).toBe('Alice Johnson');

    const claimOut = testOutput.steps?.build_fhir_claim?.step_output?.fhir_resource;
    expect(claimOut?.patient?.reference).toBe('Patient/MEM-00123');
    expect(claimOut?.insurer?.identifier?.value).toBe('1234567890');
    expect(claimOut?.priority?.coding?.[0]?.code).toBe('normal');
    expect(claimOut?.item?.[0]?.productOrService?.coding?.[0]?.code).toBe('99213');
    expect(claimOut?.diagnosis, 'Claim.diagnosis should have 2 real ICD-10 entries').toHaveLength(2);

    // fhirBundle (NOT pas_bundle) -- models.OutputNormalizer.NormalizeStepOutput
    // preserves this exact key name, unnormalized, precisely so a real FHIR
    // resource tree's own internal field names (resourceType, etc.) survive
    // intact. Real, deterministic bug found here (2026-09): the pipeline
    // used to return this under "pas_bundle", which fell outside the
    // normalizer's small preserve-list and had its entire nested content
    // silently snake-cased.
    const assembleStep = testOutput.steps?.stamp_bundle_profile;
    const pasBundle = assembleStep?.step_output?.fhirBundle;
    expect(pasBundle?.resourceType).toBe('Bundle');
    expect(pasBundle?.entry, 'assembled Bundle should have all 5 resources').toHaveLength(5);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
