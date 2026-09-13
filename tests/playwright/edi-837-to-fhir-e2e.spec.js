// tests/playwright/edi-837-to-fhir-e2e.spec.js
// V237/V238's own OOB templates (EDI 837P/837I -> FHIR), driven through the
// REAL "Use Template" click path AND a REAL "Test Pipeline" run against a
// genuine X12.org sample -- closing the one named gap left open by
// services/edi_837_real_parser_roundtrip_test.go's own header comment: that
// suite proves the real parser's output matches what the derive script
// expects, but never proves the whole thing works through the actual
// HTTP/DAG pipeline-execution stack and browser UI a real user drives.
// Same discipline as edi-pipeline-ui.spec.js's own 835-to-FHIR and 837-Inbound
// template tests (Use Template -> canvas renders every saved step with
// correct config -> Test Pipeline against a real sample -> assert on the
// real produced output), extended one step further here: parsing the
// assembled FHIR Bundle out of the payload.builder step's own step_output and
// asserting on real resource counts/fields, not just "the run succeeded".
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
// each resourceType, and returns the resources of `primaryType` for further
// inspection. The one known, pre-existing ClaimTypes ValueSet gap (empty
// codes in schemas/fhir/R4/valuesets/ClaimTypes.gz, documented since the PAS
// work and every 837 Go-level test in this round) is explicitly excluded
// from the error check, matching services/edi_837_real_parser_roundtrip_test.go's
// own assembleAndValidate837Bundle.
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

test('EDI X12 837P to FHIR template: Use Template renders 11 steps, then Test Pipeline against the real Ben Kildare sample produces a valid Claim Bundle', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 837P to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 837P to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-837P-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 11 steps').toBe(11);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 11 steps should render as canvas nodes').toBe(11);

    // Spot-check 2 representative steps' own real config, not just "a panel
    // opened" -- Parse's transactionSet (proves it's the 837P-scoped
    // template, not a generic/blank one) and Build Claim's resourceType.
    await page.locator('.flowchart-step-node', { hasText: 'Parse 837P -> JSON' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ediParseSourceField')).toHaveValue('raw');
    await expect(page.locator('#ediParseTransactionSet')).toHaveValue('837P');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build Claim' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('Claim');
    await closeStepModal(page);

    // Real Test Pipeline run against X12.org's own "Ben Kildare Service"
    // 837P sample (the same one services/edi_837_real_parser_roundtrip_test.go's
    // own TestEDI837RealParserRoundtrip_837P_X12OrgBenKildareSample test
    // proves at the Go level) -- this run proves the SAME sample also works
    // through the real HTTP/DAG pipeline-execution stack.
    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'x12org_837p_ben_kildare_sample.txt'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const unexpectedErrors = (validateStep.step_output?.errors || []).filter(
        (e) => !(String(e).includes('invalid-code') && String(e).includes('Claim.type'))
    );
    expect(unexpectedErrors, `no unexpected FHIR validation errors beyond the known ClaimTypes ValueSet gap, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_837p_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 837P FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 1, Patient: 1, Coverage: 1, Claim: 1,
    });
    const claim = bundle.entry.find((e) => e.resource.resourceType === 'Claim').resource;
    expect(claim.item, 'Ben Kildare has 4 real service lines').toHaveLength(4);
    expect(claim.item[0].productOrService.coding[0].code, 'professional productOrService is CPT-direct, not revenue-based').toBe('99213');
    expect(claim.diagnosis, 'Ben Kildare has 2 diagnoses (BK:0340, BF:V7389)').toHaveLength(2);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

test('EDI X12 837I to FHIR template: Use Template renders 11 steps, then Test Pipeline against the real Jones Hospital sample produces a valid Claim Bundle with dual-coded items', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 837I to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 837I to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-837I-FHIR Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 11 steps').toBe(11);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 11 steps should render as canvas nodes').toBe(11);

    await page.locator('.flowchart-step-node', { hasText: 'Parse 837I -> JSON' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#ediParseSourceField')).toHaveValue('raw');
    await expect(page.locator('#ediParseTransactionSet')).toHaveValue('837I');
    await closeStepModal(page);

    await page.locator('.flowchart-step-node', { hasText: 'Build Claim' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('Claim');
    await closeStepModal(page);

    // Real Test Pipeline run against X12.org's own "Jones Hospital" 837I
    // sample -- the direct proof of the corrected productOrService design
    // (revenue-primary/CPT-secondary dual coding) working through the real
    // HTTP/DAG pipeline-execution stack, matching
    // TestEDI837RealParserRoundtrip_837I_X12OrgJonesHospitalSample at the Go level.
    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'x12org_837i_jones_hospital_sample.txt'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const unexpectedErrors = (validateStep.step_output?.errors || []).filter(
        (e) => !(String(e).includes('invalid-code') && String(e).includes('Claim.type'))
    );
    expect(unexpectedErrors, `no unexpected FHIR validation errors beyond the known ClaimTypes ValueSet gap, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_837i_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 837I FHIR Bundle should produce a payload string').toBeTruthy();
    // Coverage: 2, not 1 -- this real, official X12.org sample turns out to
    // ALSO carry a genuine COB secondary payer
    // (SBR*S*01*351630*STATE TEACHERS*****CI), only surfaced once COB
    // support was added. Confirmed at the Go level too (see
    // TestEDI837RealParserRoundtrip_837I_X12OrgJonesHospitalSample_ProducesValidatingBundle's
    // own matching assertion) -- a second real-sample proof of the COB
    // mapping, independent of Example 3a's own.
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 1, Patient: 1, Coverage: 2, Claim: 1,
    });
    const claim = bundle.entry.find((e) => e.resource.resourceType === 'Claim').resource;
    expect(claim.item, 'Jones Hospital has 2 real service lines').toHaveLength(2);
    // The direct proof of the corrected productOrService design: revenue
    // code as coding[0], CPT as an ADDITIONAL coding[1] -- never a
    // conditional either/or.
    const codings = claim.item[0].productOrService.coding;
    expect(codings, 'item[0] should carry BOTH a revenue code and a CPT code together').toHaveLength(2);
    expect(codings[0].code).toBe('0305');
    expect(codings[1].code).toBe('85025');
    expect(claim.diagnosis, 'Jones Hospital has 3 diagnoses across BK/BF qualifiers (ICD-9 era)').toHaveLength(3);
    expect(claim.insurance, 'Claim.insurance should have 2 entries (primary Medicare + COB secondary STATE TEACHERS)').toHaveLength(2);

    const coverageEntries = bundle.entry.filter((e) => e.resource.resourceType === 'Coverage');
    const secondaryCoverageEntry = coverageEntries.find((e) => e.resource.id === 'coverage-756048Q-2');
    expect(secondaryCoverageEntry, `expected a secondary Coverage with id coverage-756048Q-2, got: ${JSON.stringify(coverageEntries.map(e => e.resource))}`).toBeTruthy();
    expect(secondaryCoverageEntry.resource.payor[0].identifier.value, 'secondary payer id from NM1*PR*2*STATE TEACHERS*...*PI*1135').toBe('1135');
    expect(secondaryCoverageEntry.resource.payor[0].display).toBe('STATE TEACHERS');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

test('EDI X12 837P to FHIR template: COB secondary payer (X12.org Example 3a) produces 2 Coverage resources and a 2-entry Claim.insurance[]', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 837P to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 837P to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-837P-COB Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    // Real Test Pipeline run against X12.org's own official Example 3a
    // (Coordination of Benefits) sample -- the same one
    // TestEDI837RealParserRoundtrip_837P_X12OrgExample3aCOBSample_ProducesValidatingBundle
    // proves at the Go level, now proven through the real HTTP/DAG
    // pipeline-execution stack too. The 2320/2330B secondary-payer loop
    // (SBR*S*01...NM1*PR*2*KEY INSURANCE COMPANY*...*PI*999996666) must
    // produce a SECOND Coverage resource and a 2-entry Claim.insurance[] --
    // the direct proof of the COB mapping working end-to-end, not just at
    // the Go-level executor-chain layer.
    const sample = fs.readFileSync(
        path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples', 'x12org_837p_example3a_cob_sample.txt'),
        'utf8'
    );
    const testOutput = await runTestPipeline(page, sample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const unexpectedErrors = (validateStep.step_output?.errors || []).filter(
        (e) => !(String(e).includes('invalid-code') && String(e).includes('Claim.type'))
    );
    expect(unexpectedErrors, `no unexpected FHIR validation errors beyond the known ClaimTypes ValueSet gap, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_837p_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 837P FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 1, Patient: 1, Coverage: 2, Claim: 1,
    });
    const claim = bundle.entry.find((e) => e.resource.resourceType === 'Claim').resource;
    expect(claim.insurance, 'Claim.insurance should have 2 entries (primary + COB secondary payer)').toHaveLength(2);
    expect(claim.insurance[0].focal).toBe(true);
    expect(claim.insurance[1].focal).toBe(false);

    // AssembleEntries (fhir/r4/bundle_assembler.go) rewrites every internal
    // reference to the referenced resource's own assigned urn:uuid: fullUrl
    // (not the literal ResourceType/id form fhir.build's own config writes,
    // which only survives up to the pre-assembly step) -- correlate by
    // fullUrl, not a hardcoded string, matching that real, documented
    // rewrite behavior.
    const coverageEntries = bundle.entry.filter((e) => e.resource.resourceType === 'Coverage');
    const primaryCoverageEntry = coverageEntries.find((e) => e.resource.id === 'coverage-26407789');
    const secondaryCoverageEntry = coverageEntries.find((e) => e.resource.id === 'coverage-26407789-2');
    expect(primaryCoverageEntry, `expected a primary Coverage with id coverage-26407789, got: ${JSON.stringify(coverageEntries.map(e => e.resource))}`).toBeTruthy();
    expect(secondaryCoverageEntry, `expected a secondary Coverage with id coverage-26407789-2, got: ${JSON.stringify(coverageEntries.map(e => e.resource))}`).toBeTruthy();
    expect(claim.insurance[0].coverage.reference, 'insurance[0] should reference the primary Coverage entry by its own assigned fullUrl').toBe(primaryCoverageEntry.fullUrl);
    expect(claim.insurance[1].coverage.reference, 'insurance[1] should reference the secondary Coverage entry by its own assigned fullUrl').toBe(secondaryCoverageEntry.fullUrl);

    const secondaryCoverage = secondaryCoverageEntry.resource;
    expect(secondaryCoverage.payor[0].identifier.value, 'secondary payer id from NM1*PR*2*KEY INSURANCE COMPANY*...*PI*999996666').toBe('999996666');
    expect(secondaryCoverage.payor[0].display).toBe('KEY INSURANCE COMPANY');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

// Self-authored (same convention as services/edi_837_real_parser_roundtrip_test.go's
// own raw837PMultiProviderSample -- byte-identical to it, since no real
// sample carries 2+ top-level 2000A billing-provider hierarchical levels).
// Proves the HL03 (hierarchicalLevelCode) triggerDiscriminator fix
// (edi/schemas/x12_005010/transactionSets/837{P,I}.json) works through the
// REAL HTTP/DAG pipeline-execution stack, not just the Go-level parser test.
const raw837PMultiProviderSample =
    'ISA*00*          *00*          *ZZ*SENDER123      *ZZ*RECEIVER456    *260115*1200*^*00501*000000003*0*P*:~' +
    'GS*HC*SENDER123*RECEIVER456*20260115*1200*1*X*005010X222A1~' +
    'ST*837*0001*005010X222A1~' +
    'BHT*0019*00*TX0001*20260115*1200*CH~' +
    'NM1*41*2*ACME BILLING*****46*SUB001~' +
    'NM1*40*2*PAYER1*****46*RECV001~' +
    'HL*1**20*1~' +
    'NM1*85*2*FIRST CLINIC*****XX*1111111111~' +
    'HL*2*1*22*0~' +
    'SBR*P*18**GROUPNAME*****CI~' +
    'NM1*IL*1*ALPHA*ANNA****MI*SUB-A~' +
    'DMG*D8*19750322*F~' +
    'NM1*PR*2*PAYER1*****PI*PAYER001~' +
    'CLM*CLM-A-001*100***11:B:1*Y*A*Y*Y~' +
    'HI*ABK:Z0000~' +
    'LX*1~' +
    'SV1*HC:99213*100*UN*1***1~' +
    'DTP*472*D8*20260115~' +
    'HL*3**20*1~' +
    'NM1*85*2*SECOND CLINIC*****XX*2222222222~' +
    'HL*4*3*22*0~' +
    'SBR*P*18**GROUPNAME*****CI~' +
    'NM1*IL*1*BETA*BOB****MI*SUB-B~' +
    'DMG*D8*19800101*M~' +
    'NM1*PR*2*PAYER2*****PI*PAYER002~' +
    'CLM*CLM-B-001*200***11:B:1*Y*A*Y*Y~' +
    'HI*ABK:J0690~' +
    'LX*1~' +
    'SV1*HC:99214*200*UN*1***1~' +
    'DTP*472*D8*20260116~' +
    'SE*29*0001~' +
    'GE*1*1~' +
    'IEA*1*000000003~';

test('EDI X12 837P to FHIR template: multi-billing-provider file (2 distinct 2000A) produces 2 Organizations and each Claim references its OWN provider', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 837P to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 837P to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-837P-MultiProvider Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    const testOutput = await runTestPipeline(page, raw837PMultiProviderSample);
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const validateStep = testOutput.steps?.validate_fhir_bundle;
    expect(validateStep, 'Validate FHIR Bundle step should have its own entry in the test results').toBeTruthy();
    const unexpectedErrors = (validateStep.step_output?.errors || []).filter(
        (e) => !(String(e).includes('invalid-code') && String(e).includes('Claim.type'))
    );
    expect(unexpectedErrors, `no unexpected FHIR validation errors beyond the known ClaimTypes ValueSet gap, got: ${JSON.stringify(validateStep.step_output?.errors)}`).toHaveLength(0);

    const assembleStep = testOutput.steps?.assemble_837p_fhir_bundle;
    expect(assembleStep?.step_output?.payload, 'Assemble 837P FHIR Bundle should produce a payload string').toBeTruthy();
    const bundle = parseBundleAndCount(assembleStep.step_output.payload, {
        Organization: 2, Patient: 2, Coverage: 2, Claim: 2,
    });

    const organizations = bundle.entry.filter((e) => e.resource.resourceType === 'Organization').map((e) => e.resource);
    const orgIds = organizations.map((o) => o.id).sort();
    expect(orgIds, 'both NPI-derived Organization ids should be present').toEqual(['organization-billing-1111111111', 'organization-billing-2222222222']);

    // The direct proof: each Claim references its OWN billing provider's
    // fullUrl, not the same one for both (which the pre-fix HL-trigger
    // ambiguity, or a naive first(loops["2000A"]) mapping bug, would
    // silently produce instead).
    const orgFullUrlById = {};
    for (const e of bundle.entry) {
        if (e.resource.resourceType === 'Organization') orgFullUrlById[e.resource.id] = e.fullUrl;
    }
    const claims = bundle.entry.filter((e) => e.resource.resourceType === 'Claim').map((e) => e.resource);
    const claimA = claims.find((c) => c.id === 'claim-CLM-A-001');
    const claimB = claims.find((c) => c.id === 'claim-CLM-B-001');
    expect(claimA, 'expected claim-CLM-A-001 in built Claims').toBeTruthy();
    expect(claimB, 'expected claim-CLM-B-001 in built Claims').toBeTruthy();
    expect(claimA.provider.reference).toBe(orgFullUrlById['organization-billing-1111111111']);
    expect(claimB.provider.reference).toBe(orgFullUrlById['organization-billing-2222222222']);
    expect(claimA.provider.reference).not.toBe(claimB.provider.reference);

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
