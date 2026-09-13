// tests/playwright/edi-build-e2e.spec.js
// Full-stack proof of the OUTBOUND direction of the EDI engine: edi.build
// (canonical JSON -> raw ISA...IEA X12 text), for all 3 transaction sets this
// codebase supports (835, 837P, 837I). Closes a real, named gap: edi.build
// already had solid Go-level round-trip proof (edi/real_schema_integration_test.go's
// own TestRealSchema_BuildAndRoundTrip_TwoClaims / TestRealSchema_837P_.../
// TestRealSchema_837I_...) and UI config-panel coverage (edi-pipeline-ui.spec.js's
// own dropdown/default-value checks), but NEVER a real browser-driven Test
// Pipeline execution proving the actual HTTP/DAG pipeline stack builds valid
// X12 text end-to-end -- exactly the same class of gap the 837-to-FHIR (parse
// direction) work this session found and closed for edi.parse.
//
// Each test: "Use Template" for an existing EDI template (just to get a real,
// persisted interface_id/message_type -- Test Pipeline requires one), clears
// every pre-populated step (they expect raw X12 input, not the canonical JSON
// this test supplies), adds a fresh edi.build -> edi.parse chain, and feeds a
// canonical JSON object (the SAME shape edi.parse's own ParsedJSON output
// uses, and edi_build_executor_test.go's own fixtures use) as the JSON test
// input. edi.parse re-parsing edi.build's own text output, through the real
// pipeline-execution stack, is the genuine round-trip proof -- not just "the
// built string looks like X12".
const { test, expect } = require('@playwright/test');

const closeStepModal = async (page) => {
    const closeBtn = page.locator('#stepPropertiesModal .modal-close');
    if (await closeBtn.count() > 0) await closeBtn.first().click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'hidden', timeout: 5000 }).catch(() => {});
};

// Replaces whatever steps a template pre-populated with a single
// edi.build -> edi.parse chain, so this test's own canonical-JSON input
// (not raw X12 text) is the only thing any step in this pipeline sees.
const buildOnlyPipeline = async (page, transactionSet) => {
    await page.evaluate((txSet) => {
        const existing = window.pipelineBuilder.getAllStepsFlat();
        for (const s of existing) window.pipelineBuilder.deleteStep(s.id);

        // outputField names are deliberately snake_case (not "builtEDI"/"reparsedEDI"):
        // models.OutputNormalizer.NormalizeStepOutput snake_cases every step_output key
        // that isn't already snake_case before it reaches the test-results JSON this
        // spec reads, so a camelCase field name here would silently rename itself
        // (e.g. "builtEDI" -> "built_edi") between what the config says and what the
        // browser actually sees -- the same class of bug this session's own PAS work
        // found and fixed for "pas_bundle"/"fhirBundle". Using snake_case up front
        // keeps the config and the assertions honestly in agreement.
        const buildStep = new VisualStep({
            stepName: 'Build EDI Document',
            stepType: 'edi.build',
            sequence: 10,
            config: { sourceField: 'parsedEDI', transactionSet: txSet, outputField: 'built_edi' },
        });
        window.pipelineBuilder.addStep(buildStep);

        const parseStep = new VisualStep({
            stepName: 'Reparse Built EDI',
            stepType: 'edi.parse',
            sequence: 20,
            config: { sourceField: 'built_edi', transactionSet: txSet, outputField: 'reparsed_edi' },
        });
        window.pipelineBuilder.addStep(parseStep);
    }, transactionSet);
};

const runBuildTestPipeline = async (page, canonicalInput) => {
    await page.evaluate(() => window.pipelineBuilder.openTestModal());
    await page.waitForSelector('#testModal.active', { timeout: 5000 });
    await page.selectOption('#testMessageFormat', 'json');
    await page.fill('#testMessageInput', JSON.stringify({ parsedEDI: canonicalInput }));
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

// Asserts the common shape every one of these tests needs: edi.build produced
// a real ISA...IEA string, and edi.parse re-parsed it back into a structured
// map matching the expected round-tripped fields.
//
// NOTE on reparsed's own key casing: NormalizeStepOutput recursively snake_cases
// EVERY key of a step_output value that isn't one of the few protected wrapper
// keys ("result"/"fhirBundle"/"fhirResource") -- and edi.parse's own ParsedJSON
// output is a plain map, not one of those, so by the time `reparsed` reaches this
// test every key in it (at every depth) has gone through that transform: a loop
// id like "2000A" becomes "2000a" (letters lowercased, no underscore inserted --
// there's no lower->upper/upper->lower transition for toSnakeCase to key off of),
// and a field like "transactionSet" becomes "transaction_set". VALUES are never
// touched by this (only object/array structure is walked), so plain strings like
// "PC001" or "SUBSCRIBER-CLAIM-001" survive completely unchanged -- which is why
// each test below leans on JSON.stringify(reparsed).includes(...) content checks
// for its own value-level round-trip proof instead of guessing every renamed key.
function assertBuildAndReparse(testOutput, wantTransactionSet) {
    expect(testOutput?.success, `Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

    const buildStep = testOutput.steps?.build_edi_document;
    expect(buildStep, 'build_edi_document step should have its own entry in the test results').toBeTruthy();
    const builtEDI = buildStep.step_output?.built_edi;
    expect(builtEDI, 'edi.build should produce a non-empty built EDI string').toBeTruthy();
    expect(builtEDI.startsWith('ISA*'), `built EDI should start with ISA*, got: ${builtEDI.slice(0, 50)}`).toBe(true);
    expect(builtEDI, 'built EDI should terminate with a real IEA trailer').toContain('IEA*');

    const parseStep = testOutput.steps?.reparse_built_edi;
    expect(parseStep, 'reparse_built_edi step should have its own entry in the test results').toBeTruthy();
    const reparsed = parseStep.step_output?.reparsed_edi;
    expect(reparsed, 'edi.parse should produce a structured reparsed_edi map from the built text').toBeTruthy();
    expect(reparsed.transaction_set, 'the re-parsed transaction set should match what was built').toBe(wantTransactionSet);

    return { builtEDI, reparsed };
}

// A loop id as it appears after NormalizeStepOutput's key normalization --
// see assertBuildAndReparse's own note above. Purely a lowercase transform for
// these particular ids (no underscores are ever inserted for them).
const normLoopKey = (id) => id.toLowerCase();

test('edi.build: builds a real 835 remittance from canonical JSON, and it re-parses correctly (real HTTP/DAG pipeline)', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });
    const card = page.locator('.tg-card', { hasText: 'EDI X12 835 Inbound' });
    await expect(card, 'Templates gallery should list the EDI X12 835 Inbound template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-Build-835 Test ${Date.now()}`);
    await page.click('#tcf_submit');
    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    await buildOnlyPipeline(page, '835');

    // Same canonical shape edi/real_schema_integration_test.go's own
    // TestRealSchema_BuildAndRoundTrip_TwoClaims proves at the Go level --
    // one claim, one service line, one PLB trailer adjustment.
    const canonicalInput = {
        interchange: { senderId: 'PAYERSENDER', receiverId: 'PROVIDERRECV' },
        header: {
            BPR: { transactionHandlingCode: 'I', totalActualProviderPaymentAmount: '400.00', creditOrDebitFlagCode: 'C', paymentMethodCode: 'ACH' },
            TRN: { traceTypeCode: '1', checkOrEFTTraceNumber: 'TRACE12345' },
        },
        loops: {
            '1000A': { N1: { entityIdentifierCode: 'PR', name: 'ACME PAYER' } },
            '1000B': { N1: { entityIdentifierCode: 'PE', name: 'PROVIDER GROUP' } },
            '2000': [
                {
                    LX: { assignedNumber: '1' },
                    loops: {
                        '2100': [
                            {
                                CLP: { patientControlNumber: 'PC001', claimStatusCode: '1', totalClaimChargeAmount: '500.00', claimPaymentAmount: '400.00' },
                                loops: {
                                    '2110': [
                                        { SVC: { procedureCode: { qualifier: 'HC', code: '99213' }, chargeAmount: '500.00', paidAmount: '400.00' } },
                                    ],
                                },
                            },
                        ],
                    },
                },
            ],
        },
        trailer: {
            PLB: [
                { referenceIdentification: '1234567890', fiscalPeriodDate: '20261231', adjustments: [{ adjustmentIdentifier: { reasonCode: 'WO', referenceIdentification: 'REF1' }, amount: '-25.00' }] },
            ],
        },
    };

    const testOutput = await runBuildTestPipeline(page, canonicalInput);
    const { builtEDI, reparsed } = assertBuildAndReparse(testOutput, '835');

    // edi.build itself produced the right X12 content for this transaction set.
    expect(builtEDI).toContain('ST*835*');
    expect(builtEDI).toContain('PAYERSENDER');
    expect(builtEDI).toContain('PC001');
    expect(builtEDI).toContain('99213');

    // Structural round trip: exactly 1 instance of loop 2000, containing exactly
    // 1 instance of loop 2100 (the claim), carrying its own CLP segment -- all
    // reparsed back out of the built text by a REAL edi.parse step.
    const loop2000 = reparsed.loops?.['2000'];
    expect(Array.isArray(loop2000) ? loop2000.length : 0, `expected 1 instance of loop 2000, got: ${JSON.stringify(loop2000)}`).toBe(1);
    const loop2100 = loop2000[0].loops?.[normLoopKey('2100')];
    expect(Array.isArray(loop2100) ? loop2100.length : 0, `expected 1 instance of loop 2100, got: ${JSON.stringify(loop2100)}`).toBe(1);
    expect(loop2100[0][normLoopKey('CLP')], 'the reparsed claim loop should carry its own CLP segment').toBeTruthy();

    // Value-level round trip via content search (robust to the key-casing
    // transform documented on assertBuildAndReparse).
    const reparsedJSON = JSON.stringify(reparsed);
    expect(reparsedJSON).toContain('PAYERSENDER');
    expect(reparsedJSON).toContain('TRACE12345');
    expect(reparsedJSON).toContain('PC001');
    expect(reparsedJSON).toContain('99213');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

test('edi.build: builds a real 837P professional claim from canonical JSON, and it re-parses correctly (real HTTP/DAG pipeline)', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });
    const card = page.locator('.tg-card', { hasText: 'EDI X12 837P to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 837P to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-Build-837P Test ${Date.now()}`);
    await page.click('#tcf_submit');
    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    await buildOnlyPipeline(page, '837P');

    // Same canonical shape edi/real_schema_integration_test.go's own
    // TestRealSchema_837P_BuildAndRoundTrip_LoopRefResolvesIndependently
    // proves at the Go level -- one billing provider, one subscriber (as her
    // own patient), one claim, one service line.
    const canonicalInput = {
        interchange: { senderId: 'PROVIDER1', receiverId: 'PAYER1' },
        header: {
            BHT: {
                hierarchicalStructureCode: '0019', transactionSetPurposeCode: '00',
                originatorApplicationTransactionIdentifier: 'TX0001',
                transactionSetCreationDate: '20260115', transactionSetCreationTime: '1200',
                claimOrEncounterIdentifier: 'CH',
            },
        },
        loops: {
            '1000A': { NM1: { entityIdentifierCode: '41', entityTypeQualifier: '2', nameLastOrOrganizationName: 'ACME BILLING', identificationCodeQualifier: '46', identificationCode: 'SUB001' } },
            '1000B': { NM1: { entityIdentifierCode: '40', entityTypeQualifier: '2', nameLastOrOrganizationName: 'PAYER1', identificationCodeQualifier: '46', identificationCode: 'RECV001' } },
            '2000A': [
                {
                    HL: { hierarchicalIdNumber: '1', hierarchicalLevelCode: '20', hierarchicalChildCode: '1' },
                    loops: {
                        '2010AA': { NM1: { entityIdentifierCode: '85', entityTypeQualifier: '2', nameLastOrOrganizationName: 'ACME CLINIC', identificationCodeQualifier: 'XX', identificationCode: '1234567890' } },
                        '2000B': [
                            {
                                HL: { hierarchicalIdNumber: '2', hierarchicalParentIdNumber: '1', hierarchicalLevelCode: '22', hierarchicalChildCode: '0' },
                                SBR: { payerResponsibilitySequenceNumberCode: 'P', individualRelationshipCode: '18' },
                                loops: {
                                    '2010BA': { NM1: { entityIdentifierCode: 'IL', entityTypeQualifier: '1', nameLastOrOrganizationName: 'SMITH', nameFirst: 'JANE', identificationCodeQualifier: 'MI', identificationCode: 'SUB123' } },
                                    '2010BB': { NM1: { entityIdentifierCode: 'PR', entityTypeQualifier: '2', nameLastOrOrganizationName: 'PAYER1', identificationCodeQualifier: 'PI', identificationCode: 'PAYER001' } },
                                    '2300': [
                                        {
                                            CLM: {
                                                patientControlNumber: 'SUBSCRIBER-CLAIM-001', totalClaimChargeAmount: '250.00',
                                                healthCareServiceLocation: { placeOfServiceCode: '11', facilityCodeQualifier: 'B', claimFrequencyCode: '1' },
                                                providerSignatureIndicator: 'Y', assignmentOrPlanParticipationCode: 'A',
                                                benefitsAssignmentCertificationIndicator: 'Y', releaseOfInformationCode: 'Y',
                                            },
                                            HI: [{ codes: [{ code: { qualifier: 'ABK', code: 'R51' } }] }],
                                            loops: {
                                                '2400': [
                                                    {
                                                        LX: { assignedNumber: '1' },
                                                        SV1: { procedureCode: { qualifier: 'HC', code: '99213' }, lineItemChargeAmount: '250.00', unitOfMeasurementCode: 'UN', serviceUnitCount: '1', diagnosisCodePointer: { pointer1: '1' } },
                                                        DTP: [{ dateTimeQualifier: '472', dateTimePeriodFormatQualifier: 'D8', datePeriod: '20260115' }],
                                                    },
                                                ],
                                            },
                                        },
                                    ],
                                },
                            },
                        ],
                    },
                },
            ],
        },
    };

    const testOutput = await runBuildTestPipeline(page, canonicalInput);
    const { builtEDI, reparsed } = assertBuildAndReparse(testOutput, '837P');

    // edi.build itself produced the right X12 content for this transaction set.
    expect(builtEDI).toContain('ST*837*');
    expect(builtEDI).toContain('PROVIDER1');
    expect(builtEDI).toContain('SUBSCRIBER-CLAIM-001');
    expect(builtEDI).toContain('99213');

    // Structural round trip: 2000A -> 2000B -> 2300 -> 2400, each exactly 1
    // instance, each carrying its own segment -- reparsed by a REAL edi.parse step.
    const loop2000A = reparsed.loops?.[normLoopKey('2000A')];
    expect(Array.isArray(loop2000A) ? loop2000A.length : 0, `expected 1 instance of loop 2000A, got: ${JSON.stringify(loop2000A)}`).toBe(1);
    const loop2000B = loop2000A[0].loops?.[normLoopKey('2000B')];
    expect(Array.isArray(loop2000B) ? loop2000B.length : 0, `expected 1 instance of loop 2000B, got: ${JSON.stringify(loop2000B)}`).toBe(1);
    const loop2300 = loop2000B[0].loops?.[normLoopKey('2300')];
    expect(Array.isArray(loop2300) ? loop2300.length : 0, `expected 1 instance of loop 2300, got: ${JSON.stringify(loop2300)}`).toBe(1);
    expect(loop2300[0][normLoopKey('CLM')], 'the reparsed claim loop should carry its own CLM segment').toBeTruthy();
    const loop2400 = loop2300[0].loops?.[normLoopKey('2400')];
    expect(Array.isArray(loop2400) ? loop2400.length : 0, `expected 1 instance of loop 2400, got: ${JSON.stringify(loop2400)}`).toBe(1);
    expect(loop2400[0][normLoopKey('SV1')], 'the reparsed service line should carry its own SV1 segment').toBeTruthy();

    // Value-level round trip via content search.
    const reparsedJSON = JSON.stringify(reparsed);
    expect(reparsedJSON).toContain('PROVIDER1');
    expect(reparsedJSON).toContain('SUBSCRIBER-CLAIM-001');
    expect(reparsedJSON).toContain('99213');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

test('edi.build: builds a real 837I institutional claim from canonical JSON, and it re-parses correctly (real HTTP/DAG pipeline)', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });
    const card = page.locator('.tg-card', { hasText: 'EDI X12 837I to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 837I to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-Build-837I Test ${Date.now()}`);
    await page.click('#tcf_submit');
    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    await buildOnlyPipeline(page, '837I');

    // Same canonical shape edi/real_schema_integration_test.go's own
    // TestRealSchema_837I_BuildAndRoundTrip_DistinctFrom837P proves at the Go
    // level -- institutional-only CL1/SV2, plus a 2320 COB secondary payer
    // carrying both MIA and MOA together.
    const canonicalInput = {
        interchange: { senderId: 'HOSPITAL1', receiverId: 'PAYER1' },
        header: {
            BHT: {
                hierarchicalStructureCode: '0019', transactionSetPurposeCode: '00',
                originatorApplicationTransactionIdentifier: 'TX0002',
                transactionSetCreationDate: '20260115', transactionSetCreationTime: '1200',
                claimOrEncounterIdentifier: 'CH',
            },
        },
        loops: {
            '1000A': { NM1: { entityIdentifierCode: '41', entityTypeQualifier: '2', nameLastOrOrganizationName: 'GENERAL HOSPITAL', identificationCodeQualifier: '46', identificationCode: 'SUB002' } },
            '1000B': { NM1: { entityIdentifierCode: '40', entityTypeQualifier: '2', nameLastOrOrganizationName: 'PAYER1', identificationCodeQualifier: '46', identificationCode: 'RECV001' } },
            '2000A': [
                {
                    HL: { hierarchicalIdNumber: '1', hierarchicalLevelCode: '20', hierarchicalChildCode: '1' },
                    loops: {
                        '2010AA': { NM1: { entityIdentifierCode: '85', entityTypeQualifier: '2', nameLastOrOrganizationName: 'GENERAL HOSPITAL', identificationCodeQualifier: 'XX', identificationCode: '9876543210' } },
                        '2000B': [
                            {
                                HL: { hierarchicalIdNumber: '2', hierarchicalParentIdNumber: '1', hierarchicalLevelCode: '22', hierarchicalChildCode: '0' },
                                SBR: { payerResponsibilitySequenceNumberCode: 'P', individualRelationshipCode: '18' },
                                loops: {
                                    '2010BA': { NM1: { entityIdentifierCode: 'IL', entityTypeQualifier: '1', nameLastOrOrganizationName: 'DOE', nameFirst: 'JOHN', identificationCodeQualifier: 'MI', identificationCode: 'SUB456' } },
                                    '2010BB': { NM1: { entityIdentifierCode: 'PR', entityTypeQualifier: '2', nameLastOrOrganizationName: 'PAYER1', identificationCodeQualifier: 'PI', identificationCode: 'PAYER001' } },
                                    '2300': [
                                        {
                                            CLM: {
                                                patientControlNumber: 'INST-CLAIM-001', totalClaimChargeAmount: '5000.00',
                                                healthCareServiceLocation: { placeOfServiceCode: '11', facilityCodeQualifier: 'A', claimFrequencyCode: '1' },
                                                providerSignatureIndicator: 'Y', assignmentOrPlanParticipationCode: 'A',
                                                benefitsAssignmentCertificationIndicator: 'Y', releaseOfInformationCode: 'Y',
                                            },
                                            CL1: { admissionTypeCode: '1', admissionSourceCode: '1', patientStatusCode: '01' },
                                            HI: [{ codes: [{ code: { qualifier: 'ABK', code: 'I219' } }] }],
                                            loops: {
                                                '2320': [
                                                    {
                                                        SBR: { payerResponsibilitySequenceNumberCode: 'S', individualRelationshipCode: '01' },
                                                        OI: { benefitsAssignmentCertificationIndicator: 'Y', releaseOfInformationCode: 'Y' },
                                                        MIA: { coveredDaysCount: '5', claimDRGAmount: '4500.00' },
                                                        MOA: { reimbursementRate: '0.8' },
                                                        loops: {
                                                            '2330A': { NM1: { entityIdentifierCode: 'IL', entityTypeQualifier: '1', nameLastOrOrganizationName: 'DOE', nameFirst: 'JOHN' } },
                                                            '2330B': { NM1: { entityIdentifierCode: 'PR', entityTypeQualifier: '2', nameLastOrOrganizationName: 'SECONDARY PAYER' } },
                                                        },
                                                    },
                                                ],
                                                '2400': [
                                                    {
                                                        LX: { assignedNumber: '1' },
                                                        SV2: { serviceLineRevenueCode: '0450', procedureCode: { qualifier: 'HC', code: '99284' }, lineItemChargeAmount: '5000.00', unitOfMeasurementCode: 'UN', serviceUnitCount: '1' },
                                                    },
                                                ],
                                            },
                                        },
                                    ],
                                },
                            },
                        ],
                    },
                },
            ],
        },
    };

    const testOutput = await runBuildTestPipeline(page, canonicalInput);
    const { builtEDI, reparsed } = assertBuildAndReparse(testOutput, '837I');

    // edi.build itself produced the right X12 content for this transaction set,
    // including the institutional-only CL1/SV2 segments and the COB secondary
    // payer's own MIA/MOA + 2330B payer name.
    expect(builtEDI).toContain('ST*837*');
    expect(builtEDI).toContain('HOSPITAL1');
    expect(builtEDI).toContain('INST-CLAIM-001');
    expect(builtEDI).toContain('99284');
    expect(builtEDI).toContain('0450');
    expect(builtEDI).toContain('4500.00');
    expect(builtEDI).toContain('SECONDARY PAYER');

    // Structural round trip: 2000A -> 2000B -> 2300 -> {2320 (COB), 2400} --
    // reparsed by a REAL edi.parse step.
    const loop2000A = reparsed.loops?.[normLoopKey('2000A')];
    expect(Array.isArray(loop2000A) ? loop2000A.length : 0, `expected 1 instance of loop 2000A, got: ${JSON.stringify(loop2000A)}`).toBe(1);
    const loop2000B = loop2000A[0].loops?.[normLoopKey('2000B')];
    expect(Array.isArray(loop2000B) ? loop2000B.length : 0, `expected 1 instance of loop 2000B, got: ${JSON.stringify(loop2000B)}`).toBe(1);
    const loop2300 = loop2000B[0].loops?.[normLoopKey('2300')];
    expect(Array.isArray(loop2300) ? loop2300.length : 0, `expected 1 instance of loop 2300, got: ${JSON.stringify(loop2300)}`).toBe(1);
    const claim = loop2300[0];
    expect(claim[normLoopKey('CLM')], 'the reparsed claim loop should carry its own CLM segment').toBeTruthy();
    expect(claim[normLoopKey('CL1')], 'the reparsed claim loop should carry its own institutional-only CL1 segment').toBeTruthy();

    // The direct structural proof of the COB secondary payer + MIA/MOA-together round trip.
    const cob = claim.loops?.[normLoopKey('2320')];
    expect(Array.isArray(cob) ? cob.length : 0, `expected 1 instance of the COB loop 2320, got: ${JSON.stringify(cob)}`).toBe(1);
    expect(cob[0][normLoopKey('SBR')], 'the COB loop should carry its own SBR segment').toBeTruthy();
    expect(cob[0][normLoopKey('MIA')], 'the COB loop should carry its own MIA segment').toBeTruthy();
    expect(cob[0][normLoopKey('MOA')], 'the COB loop should carry its own MOA segment (proving MIA+MOA-together survives the round trip)').toBeTruthy();

    const loop2400 = claim.loops?.[normLoopKey('2400')];
    expect(Array.isArray(loop2400) ? loop2400.length : 0, `expected 1 instance of loop 2400, got: ${JSON.stringify(loop2400)}`).toBe(1);
    expect(loop2400[0][normLoopKey('SV2')], 'the reparsed service line should carry its own institutional SV2 segment (not SV1)').toBeTruthy();

    // Value-level round trip via content search.
    const reparsedJSON = JSON.stringify(reparsed);
    expect(reparsedJSON).toContain('HOSPITAL1');
    expect(reparsedJSON).toContain('INST-CLAIM-001');
    expect(reparsedJSON).toContain('99284');
    expect(reparsedJSON).toContain('4500.00');
    expect(reparsedJSON).toContain('SECONDARY PAYER');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});

// EDI X12 Phase 3 (270/271 eligibility) — proves the user's own second
// direction ("data in any other form -> 271") is already covered by
// edi.build's existing, fully generic canonical-JSON-in mechanism, with zero
// new mapping code: a 271 (eligibility RESPONSE) built from a hand-built
// canonical JSON object, the same way 835/837P/837I already proved earlier
// in this file.
test('edi.build: builds a real 271 eligibility response from canonical JSON, and it re-parses correctly (real HTTP/DAG pipeline)', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });
    const card = page.locator('.tg-card', { hasText: 'EDI X12 271 to FHIR' });
    await expect(card, 'Templates gallery should list the EDI X12 271 to FHIR template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-Build-271 Test ${Date.now()}`);
    await page.click('#tcf_submit');
    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

    await buildOnlyPipeline(page, '271');

    // Same canonical shape services/edi_271_fhir_builder_test.go's own
    // edi271FixtureLoops() proves at the Go level -- one subscriber getting a
    // real active-coverage EB answer with a nested 2115C benefit-additional
    // info III.
    const canonicalInput = {
        interchange: { senderId: 'PAYER1', receiverId: 'PROVIDER1' },
        header: {
            BHT: {
                hierarchicalStructureCode: '0022', transactionSetPurposeCode: '11',
                originatorApplicationTransactionIdentifier: 'ELIG0001',
                transactionSetCreationDate: '20260912', transactionSetCreationTime: '1201',
            },
        },
        loops: {
            '2000A': [
                {
                    HL: { hierarchicalIdNumber: '1', hierarchicalLevelCode: '20', hierarchicalChildCode: '1' },
                    loops: {
                        '2100A': { NM1: { entityIdentifierCode: 'PR', entityTypeQualifier: '2', nameLastOrOrganizationName: 'PAYER1', identificationCodeQualifier: 'PI', identificationCode: 'PAYER001' } },
                        '2000B': [
                            {
                                HL: { hierarchicalIdNumber: '2', hierarchicalParentIdNumber: '1', hierarchicalLevelCode: '21', hierarchicalChildCode: '1' },
                                loops: {
                                    '2100B': { NM1: { entityIdentifierCode: '1P', entityTypeQualifier: '2', nameLastOrOrganizationName: 'ACME CLINIC', identificationCodeQualifier: 'XX', identificationCode: '1234567890' } },
                                    '2000C': [
                                        {
                                            HL: { hierarchicalIdNumber: '3', hierarchicalParentIdNumber: '2', hierarchicalLevelCode: '22', hierarchicalChildCode: '0' },
                                            TRN: { traceTypeCode: '2', checkOrEFTTraceNumber: 'TRACE001' },
                                            loops: {
                                                '2100C': {
                                                    NM1: { entityIdentifierCode: 'IL', entityTypeQualifier: '1', nameLastOrOrganizationName: 'SMITH', nameFirst: 'JANE', identificationCodeQualifier: 'MI', identificationCode: 'SUB123' },
                                                    DMG: { dateTimePeriodFormatQualifier: 'D8', birthDate: '19800101', genderCode: 'F' },
                                                    loops: {
                                                        '2110C': [
                                                            {
                                                                EB: { eligibilityBenefitInformationCode: '1', coverageLevelCode: 'IND', serviceTypeCode: '30', planCoverageDescription: 'PPO GOLD', monetaryAmount: '25.00' },
                                                                loops: {
                                                                    '2115C': [
                                                                        { III: { codeListQualifierCode: 'ZZ', industryCode: 'REMAINING VISITS: 5' } },
                                                                    ],
                                                                },
                                                            },
                                                        ],
                                                    },
                                                },
                                            },
                                        },
                                    ],
                                },
                            },
                        ],
                    },
                },
            ],
        },
    };

    const testOutput = await runBuildTestPipeline(page, canonicalInput);
    const { builtEDI, reparsed } = assertBuildAndReparse(testOutput, '271');

    // edi.build itself produced the right X12 content for this transaction set.
    expect(builtEDI).toContain('ST*271*');
    expect(builtEDI).toContain('PAYER1');
    expect(builtEDI).toContain('SUB123');
    expect(builtEDI).toContain('PPO GOLD');
    expect(builtEDI).toContain('25.00');
    expect(builtEDI).toContain('REMAINING VISITS: 5');

    // Structural round trip: 2000A -> 2000B -> 2000C -> 2100C -> 2110C -> 2115C,
    // each exactly 1 instance -- reparsed by a REAL edi.parse step.
    const loop2000A = reparsed.loops?.[normLoopKey('2000A')];
    expect(Array.isArray(loop2000A) ? loop2000A.length : 0, `expected 1 instance of loop 2000A, got: ${JSON.stringify(loop2000A)}`).toBe(1);
    const loop2000B = loop2000A[0].loops?.[normLoopKey('2000B')];
    expect(Array.isArray(loop2000B) ? loop2000B.length : 0, `expected 1 instance of loop 2000B, got: ${JSON.stringify(loop2000B)}`).toBe(1);
    const loop2000C = loop2000B[0].loops?.[normLoopKey('2000C')];
    expect(Array.isArray(loop2000C) ? loop2000C.length : 0, `expected 1 instance of loop 2000C, got: ${JSON.stringify(loop2000C)}`).toBe(1);
    const loop2100C = loop2000C[0].loops?.[normLoopKey('2100C')];
    expect(loop2100C, 'the reparsed subscriber loop should carry its own 2100C sub-loop').toBeTruthy();
    const loop2110C = loop2100C.loops?.[normLoopKey('2110C')];
    expect(Array.isArray(loop2110C) ? loop2110C.length : 0, `expected 1 instance of loop 2110C, got: ${JSON.stringify(loop2110C)}`).toBe(1);
    expect(loop2110C[0][normLoopKey('EB')], 'the reparsed benefit loop should carry its own EB segment').toBeTruthy();
    const loop2115C = loop2110C[0].loops?.[normLoopKey('2115C')];
    expect(Array.isArray(loop2115C) ? loop2115C.length : 0, `expected 1 instance of loop 2115C, got: ${JSON.stringify(loop2115C)}`).toBe(1);
    expect(loop2115C[0][normLoopKey('III')], 'the reparsed benefit-additional-info loop should carry its own III segment').toBeTruthy();

    // Value-level round trip via content search.
    const reparsedJSON = JSON.stringify(reparsed);
    expect(reparsedJSON).toContain('SUB123');
    expect(reparsedJSON).toContain('PPO GOLD');
    expect(reparsedJSON).toContain('REMAINING VISITS: 5');

    expect(consoleErrors, `expected no console errors, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
