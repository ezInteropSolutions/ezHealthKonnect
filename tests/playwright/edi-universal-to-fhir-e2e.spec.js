// tests/playwright/edi-universal-to-fhir-e2e.spec.js
// V257's Universal EDI X12 Receiver -- ONE interface that auto-routes to
// whichever of the 9 existing X12-to-FHIR mapping chains (835, 837P, 837I,
// 270, 271, 276, 277, 278, 834) matches each file's own detected transaction
// set (ST01/GS08), reusing every branch's own already-proven derive
// script/fhir.build config unmodified except for a per-branch outputField/
// step_alias rename (to avoid cross-branch collisions) and a rowsPath
// conversion for the 3 build steps that used to build unconditionally
// (278's UMO Organization, 834's Sponsor Organization, 835's
// PaymentReconciliation).
//
// This is the DECISIVE test for the whole feature: it drives the real "Use
// Template" click path once, then runs "Test Pipeline" 9 separate times
// against the SAME saved pipeline -- once per real/self-authored sample
// already used by each transaction set's own standalone e2e spec -- and
// asserts, for every single run, that ONLY the correct branch's resource
// types appear in the assembled Bundle with the exact counts that branch's
// own standalone spec already proves, AND that no OTHER branch's resource
// types leaked in at all. That second half is what actually proves the
// outputField-collision fix and the stub-resource-leak fix both work; a run
// that "succeeds" but silently mixes in another branch's resources would
// still pass a naive "did it run" check.
const { test, expect } = require('@playwright/test');
const fs = require('fs');
const path = require('path');

const SAMPLES_DIR = path.join(__dirname, '..', '..', 'edi', 'testdata', 'real_samples');

const closeStepModal = async (page) => {
    const closeBtn = page.locator('#stepPropertiesModal .modal-close');
    if (await closeBtn.count() > 0) await closeBtn.first().click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'hidden', timeout: 5000 }).catch(() => {});
};

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
        { timeout: 25000 }
    );

    // Close the test modal so the next run starts from a clean state.
    const closeTestBtn = page.locator('#testModal .modal-close');
    return page.evaluate(() => window.pipelineLastTestOutput).finally(async () => {
        if (await closeTestBtn.count() > 0) await closeTestBtn.first().click().catch(() => {});
        await page.waitForSelector('#testModal.active', { state: 'hidden', timeout: 5000 }).catch(() => {});
    });
};

// Asserts the bundle contains EXACTLY wantCounts -- every expected
// resourceType at its expected count, AND no other resourceType present at
// all. This is the isolation check: any cross-branch leakage would show up
// either as an unexpected extra resourceType key, or as a higher-than-
// expected count on a resourceType shared with the branch under test.
function assertExactBundleContents(bundleJSON, wantCounts, label) {
    const bundle = JSON.parse(bundleJSON);
    expect(bundle.resourceType, `${label}: assembled payload should be a FHIR Bundle`).toBe('Bundle');

    const counts = {};
    for (const entry of bundle.entry || []) {
        const rt = entry.resource.resourceType;
        counts[rt] = (counts[rt] || 0) + 1;
    }

    for (const [rt, want] of Object.entries(wantCounts)) {
        expect(counts[rt] || 0, `${label}: expected ${want} ${rt} resource(s), got ${counts[rt] || 0}. Full counts: ${JSON.stringify(counts)}`).toBe(want);
    }

    const unexpectedTypes = Object.keys(counts).filter((rt) => !(rt in wantCounts));
    expect(unexpectedTypes, `${label}: found resource types NOT belonging to this branch -- cross-branch leakage. Full counts: ${JSON.stringify(counts)}`).toEqual([]);

    return bundle;
}

// One entry per branch: sample file, expected exact Bundle contents (taken
// verbatim from that transaction set's own standalone e2e spec).
const BRANCHES = [
    { type: '835', file: 'blue_cross_nc_sample.txt', wantCounts: { ExplanationOfBenefit: 1, PaymentReconciliation: 1 } },
    { type: '837P', file: 'x12org_837p_ben_kildare_sample.txt', wantCounts: { Organization: 1, Patient: 1, Coverage: 1, Claim: 1 } },
    { type: '837I', file: 'x12org_837i_jones_hospital_sample.txt', wantCounts: { Organization: 1, Patient: 1, Coverage: 2, Claim: 1 } },
    { type: '270', file: 'self_authored_270_sample.txt', wantCounts: { Organization: 2, Patient: 1, CoverageEligibilityRequest: 1 } },
    { type: '271', file: 'self_authored_271_sample.txt', wantCounts: { Patient: 1, CoverageEligibilityResponse: 1 } },
    { type: '276', file: 'self_authored_276_sample.txt', wantCounts: { Organization: 2, Patient: 2, Task: 2 } },
    { type: '277', file: 'self_authored_277_sample.txt', wantCounts: { Organization: 2, Patient: 2, Task: 2 } },
    { type: '278', file: 'self_authored_278_sample.txt', wantCounts: { Organization: 1, Patient: 2, Claim: 2, ClaimResponse: 1 } },
    { type: '834', file: 'self_authored_834_sample.txt', wantCounts: { Organization: 1, Patient: 2, Coverage: 2 } },
];

test('Universal EDI Receiver: Use Template renders all 44 steps, then Test Pipeline against all 9 real transaction-set samples auto-routes to the correct branch with zero cross-branch leakage', async ({ page }) => {
    test.setTimeout(180000);

    const consoleErrors = [];
    page.on('console', (msg) => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', (err) => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-card', { timeout: 15000 });

    const card = page.locator('.tg-card', { hasText: 'EDI X12 Universal Receiver' });
    await expect(card, 'Templates gallery should list the Universal EDI Receiver template').toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();

    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
    await page.fill('#tcf_name', `PW EDI-Universal Test ${Date.now()}`);
    await page.click('#tcf_submit');

    await page.waitForURL(/pipeline-builder\.html\?interfaceId=/, { timeout: 15000 });
    await page.waitForLoadState('load');
    await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 10000 });

    const stepCount = await page.evaluate(() => window.pipelineBuilder.getAllStepsFlat().length);
    expect(stepCount, 'template should have saved all 44 steps (3 shared top + 9 derive + 29 fhir.build + 3 shared bottom)').toBe(44);

    await page.waitForSelector('.flowchart-step-node', { timeout: 8000 });
    const nodeCount = await page.locator('.flowchart-step-node').count();
    expect(nodeCount, 'all 44 steps should render as canvas nodes').toBe(44);

    // Spot-check one branch's build step renders its real config (not just
    // the generic wrapper) -- confirms the per-branch renamed step_alias
    // didn't break the step-config UI's own rendering.
    await page.locator('.flowchart-step-node', { hasText: 'Build ExplanationOfBenefit' }).click();
    await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    await expect(page.locator('#fbbResourceType')).toHaveValue('ExplanationOfBenefit');
    await closeStepModal(page);

    for (const branch of BRANCHES) {
        const sample = fs.readFileSync(path.join(SAMPLES_DIR, branch.file), 'utf8');
        const testOutput = await runTestPipeline(page, sample);
        expect(testOutput?.success, `[${branch.type}] Test Pipeline run should succeed, got: ${JSON.stringify(testOutput)}`).toBe(true);

        // NOT testOutput.steps.assemble_universal_fhir_bundle (the step_alias
        // configured in the migration) -- confirmed via a live Test Pipeline
        // run that the real steps.<key> addressing is
        // NormalizeKey(step.StepName) ("Assemble FHIR Bundle" ->
        // assemble_fhir_bundle), never the step_alias field (see CLAUDE.md's
        // own "steps.<key> addressing" gotcha, rediscovered here).
        const assembleStep = testOutput.steps?.assemble_fhir_bundle;
        expect(assembleStep?.step_output?.payload, `[${branch.type}] Assemble FHIR Bundle should produce a payload string`).toBeTruthy();

        assertExactBundleContents(assembleStep.step_output.payload, branch.wantCounts, branch.type);
    }

    expect(consoleErrors, `expected no console errors across all 9 runs, got: ${consoleErrors.join('; ')}`).toHaveLength(0);
});
