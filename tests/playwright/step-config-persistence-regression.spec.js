// tests/playwright/step-config-persistence-regression.spec.js
//
// Permanent regression guard for a real, severe bug found during a full
// QA pass (October 2026): PropertiesPanel.js's collectFormData() only ever
// called a StepBuilderRegistry-registered builder's own collectConfig()
// when the step type matched a hardcoded allowlist of VisualStep.isXxx()
// checks (File Parser / Remove Duplicates / Data Masking / Normalizer /
// API & DB Enrichment / Payload Builder / CDA). Every OTHER registered step
// type — edi.*, ncpdp.*, ncpdptelecom.*, astm.*, dicom.* — silently had its
// own custom config DISCARDED on save: a user could type a real value into
// a step's properties panel, click Save, and have it silently revert to
// the step's default the next time the pipeline was loaded. Found live via
// a real drag -> configure -> save -> reload -> reopen chain, not by
// reading code.
//
// Fixed by replacing the hardcoded allowlist with a plain
// `if (this.activeStepBuilder)` check — activeStepBuilder is only ever set
// when the currently-open panel's step type IS itself registered, so no
// per-type list is needed at all (the exact "no hardcoded one-check-per-
// type" standard this codebase holds everywhere else).
//
// This test cycles through one representative step type from each
// previously-broken family (dicom.parse, astm.parse, edi.build — the
// latter proving the fix also resolves a real, pre-existing EDI bug, not
// just the brand-new DICOM/ASTM work) and asserts the full real-world
// chain: drag onto a real canvas, type a non-default config value, save,
// reload the page fresh, reopen the same step, and confirm the typed value
// survived — not just that collectConfig() is technically callable.
const { test, expect } = require('@playwright/test');

async function createThrowawayInterface(request, name) {
    const res = await request.post('/api/interfaces', {
        data: { name, messageType: 'JSON', description: 'step-config-persistence regression check', sourceType: '', targetType: '', sourceConfig: {}, targetConfig: {} },
    });
    const data = await res.json();
    expect(data.success, JSON.stringify(data)).toBe(true);
    return data.interface?.id || data.id;
}

async function cleanupInterface(request, interfaceId) {
    await request.post(`/api/runtime/interfaces/${interfaceId}/deactivate`).catch(() => {});
    await request.delete(`/api/interfaces/${interfaceId}`).catch(() => {});
}

const CASES = [
    {
        cardText: 'DICOM Parse',
        nodeText: /DICOM Parse/i,
        fieldId: '#dicomParseSourceField',
        secondFieldId: '#dicomParseOutputField',
        value: 'regressionCheckRawField',
        secondValue: 'regressionCheckOutputField',
    },
    {
        cardText: 'ASTM Parse',
        nodeText: /ASTM Parse/i,
        fieldId: '#astmParseSourceField',
        secondFieldId: '#astmParseOutputField',
        value: 'regressionCheckAstmRaw',
        secondValue: 'regressionCheckAstmOutput',
    },
    {
        cardText: 'EDI X12 Build',
        nodeText: /EDI X12 Build/i,
        fieldId: '#ediBuildIsaSenderId',
        secondFieldId: null,
        value: 'REGRESSIONSENDER',
        secondValue: null,
    },
];

test.describe.serial('Step config survives save -> reload (regression guard)', () => {
    test.describe.configure({ retries: 0 });

    for (const c of CASES) {
        test(`${c.cardText}: a typed config value survives save and reload`, async ({ page, request }) => {
            const interfaceId = await createThrowawayInterface(request, `PW StepPersist ${c.cardText} ${Date.now()}`);

            await page.goto(`/pipeline-builder.html?interfaceId=${interfaceId}`);
            await page.waitForSelector('#all-steps-list .step-card', { timeout: 15000 });
            await page.waitForTimeout(1000);

            const wrapper = page.locator('#canvasWrapper');
            const card = page.locator('.step-card', { hasText: c.cardText });
            await expect(card, `${c.cardText} should exist in the toolbox`).toBeVisible({ timeout: 5000 });
            await card.dragTo(wrapper);
            await page.waitForTimeout(800);

            const node = page.locator('.flowchart-step-node', { hasText: c.nodeText }).first();
            await expect(node, `${c.cardText} step node should appear on canvas after drag`).toBeVisible({ timeout: 5000 });
            await node.dblclick();
            await page.waitForTimeout(500);

            const field = page.locator(c.fieldId);
            await expect(field, `${c.fieldId} should be visible in the properties panel`).toBeVisible({ timeout: 5000 });
            await field.fill(c.value);
            if (c.secondFieldId) {
                await page.locator(c.secondFieldId).fill(c.secondValue);
            }

            await page.locator('#saveStepBtn').click();
            await page.waitForTimeout(1500);

            await page.goto(`/pipeline-builder.html?interfaceId=${interfaceId}`);
            await page.waitForSelector('#all-steps-list .step-card', { timeout: 15000 });
            await page.waitForTimeout(1000);

            const reloadedNode = page.locator('.flowchart-step-node', { hasText: c.nodeText }).first();
            await expect(reloadedNode, `${c.cardText} step should still exist after reload`).toBeVisible({ timeout: 5000 });
            await reloadedNode.dblclick();
            await page.waitForTimeout(500);

            await expect(page.locator(c.fieldId), `${c.fieldId} must retain its saved value after reload, not revert to default`).toHaveValue(c.value);
            if (c.secondFieldId) {
                await expect(page.locator(c.secondFieldId)).toHaveValue(c.secondValue);
            }

            await cleanupInterface(request, interfaceId);
        });
    }
});
