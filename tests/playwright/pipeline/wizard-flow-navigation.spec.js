'use strict';
/**
 * Interface Wizard — step navigation for every processing flow x source format,
 * and rendering of every inbound / outbound connector type inside the wizard.
 *
 * Why it exists: "Other / Custom" and "File Processor" used to jump from step 1
 * to the FHIR mapping screen (model step 5) and Next from there landed on
 * "Invalid step". Those flows have no HL7 parsing / FHIR transform / mapping
 * screens at all: they go 1 -> Target Config (the last step) and offer the real
 * outbound connector builder. Passthrough / FHIR Receiver also skip straight to
 * Target Config but are storage-only (no connector builder).
 *
 * Expectations below are written out independently of FlowRegistry.js on
 * purpose, so a wrong registry edit fails here instead of agreeing with itself.
 */

const { test, expect } = require('@playwright/test');

// target: 'builder' = outbound connector builder, 'sink' = storage-only panel
const SKIP_FLOWS = [
    { flow: 'passthrough',    target: 'sink' },
    { flow: 'fhir_receiver',  target: 'sink' },
    { flow: 'file_processor', target: 'builder' },
    { flow: 'others',         target: 'builder' },
];
const HL7_FLOWS = ['hl7_to_fhir', 'hl7_to_fhir_r5'];

// Source formats offered in step 1 that make sense with each flow group.
const SOURCES_FOR_SKIP_FLOWS = ['hl7v2', 'fhir', 'file'];

async function openWizard(page, errors) {
    page.on('pageerror', e => errors.push(`pageerror: ${e.message}`));
    page.on('console', m => { if (m.type() === 'error') errors.push(`console: ${m.text()}`); });
    await page.goto('/interfaces.html');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500); // let page scripts attach their click handlers
    const newBtn = page.locator('button, a').filter({ hasText: /new interface|create interface/i }).first();
    const overlay = page.locator('#wizardModalOverlay');
    // The click can land before the wizard bootstraps; retry until the modal is visible.
    await expect(async () => {
        if (!(await overlay.isVisible())) await newBtn.click();
        await expect(overlay).toBeVisible({ timeout: 2000 });
    }).toPass({ timeout: 20000 });
    await expect(page.locator('#wizardModalOverlay #interfaceName')).toBeVisible({ timeout: 10000 });
}

async function fillStep1(page, { source, flow }) {
    await page.locator('#wizardModalOverlay #interfaceName').fill(`PW Wizard Nav ${flow} ${source} ${Date.now()}`);
    await page.locator('#wizardModalOverlay #sourceType').selectOption(source);
    await page.locator('#wizardModalOverlay #transformationFlow').selectOption(flow);
}

async function stepShown(page) {
    const el = page.locator('#wizardStepContainer [data-step]').first();
    await expect(el).toBeVisible({ timeout: 10000 });
    return el.getAttribute('data-step');
}

async function clickNext(page) {
    const next = page.locator('#wizardNext');
    await expect(next).toBeEnabled({ timeout: 10000 });
    await next.click();
    // The step transition animates; wait for it to settle.
    await page.waitForTimeout(700);
}

function ignorable(e) {
    // Network noise from unrelated dashboard widgets is not what this spec is about.
    return /favicon|Failed to load resource|net::ERR|ResizeObserver/i.test(e);
}

test.describe('Wizard step navigation — flows that skip to Target Config', () => {
    for (const { flow, target } of SKIP_FLOWS) {
        for (const source of SOURCES_FOR_SKIP_FLOWS) {
            test(`${flow} + ${source}: Step 1 -> Target Config (no FHIR/HL7 screens), Back returns to Step 1`, async ({ page }) => {
                const errors = [];
                await openWizard(page, errors);
                await fillStep1(page, { source, flow });
                await clickNext(page);

                // Landed on Target Config, not HL7 parsing / FHIR transform / mapping / "Invalid step".
                expect(await stepShown(page)).toBe('4');
                const body = page.locator('#wizardStepContainer');
                await expect(body).not.toContainText('Invalid step');
                await expect(body).not.toContainText(/Out-of-Box Mapping Templates/i);
                await expect(body).not.toContainText(/FHIR Target/);

                // Last step: Finish replaces Next.
                await expect(page.locator('#wizardFinish')).toBeVisible();
                await expect(page.locator('#wizardNext')).toBeHidden();

                if (target === 'builder') {
                    await expect(page.locator('#wizardOutboundConnectorContainer #connectorTypeSelect')).toBeVisible({ timeout: 10000 });
                    await expect(body).not.toContainText(/Storage Only/i);
                    await expect(body).not.toContainText(/FHIR resources will be sent/i);
                } else {
                    await expect(page.locator('input#targetConnectivity[value="sink"]')).toHaveCount(1);
                    await expect(page.locator('#wizardOutboundConnectorContainer')).toHaveCount(0);
                }

                // Back goes straight to Step 1 (nothing skipped silently in between).
                await page.locator('#wizardPrevious').click();
                await page.waitForTimeout(700);
                expect(await stepShown(page)).toBe('1');

                expect(errors.filter(e => !ignorable(e))).toEqual([]);
            });
        }
    }
});

test.describe('Wizard step navigation — HL7 -> FHIR flows keep their steps', () => {
    for (const flow of HL7_FLOWS) {
        test(`${flow}: Step 1 -> Step 2 (HL7 parsing), not Target Config`, async ({ page }) => {
            const errors = [];
            await openWizard(page, errors);
            await fillStep1(page, { source: 'hl7v2', flow });
            await clickNext(page);

            const step = await stepShown(page);
            expect(step).not.toBe('4');
            expect(step).not.toBe('5');
            await expect(page.locator('#wizardStepContainer')).not.toContainText('Invalid step');
            await expect(page.locator('#wizardNext')).toBeVisible();

            expect(errors.filter(e => !ignorable(e))).toEqual([]);
        });
    }

    test('ccd_to_fhir: Step 1 -> CDA step, never "Invalid step"', async ({ page }) => {
        const errors = [];
        await openWizard(page, errors);
        await fillStep1(page, { source: 'ccda', flow: 'ccd_to_fhir' });
        await clickNext(page);
        await expect(page.locator('#wizardStepContainer')).not.toContainText('Invalid step');
        expect(await stepShown(page)).not.toBe('4');
        expect(errors.filter(e => !ignorable(e))).toEqual([]);
    });
});

test.describe('Wizard connectors — every type renders without errors', () => {
    test('inbound: every connector type in Step 1 renders its config form', async ({ page }) => {
        const errors = [];
        await openWizard(page, errors);
        await fillStep1(page, { source: 'hl7v2', flow: 'others' });

        const select = page.locator('#wizardInboundConnectorContainer #connectorTypeSelect');
        await expect(select).toBeVisible({ timeout: 10000 });
        const types = await select.locator('option').evaluateAll(
            os => os.map(o => o.value).filter(v => v && v !== '_custom_'));
        expect(types.length).toBeGreaterThan(5);

        const broken = [];
        for (const type of types) {
            const before = errors.length;
            await select.selectOption(type);
            await page.waitForTimeout(150);
            const html = await page.locator('#wizardInboundConnectorContainer').innerHTML();
            if (/&lt;i |&lt;\/?(div|span|i)\b/.test(html)) broken.push(`${type}: escaped markup`);
            const newErrors = errors.slice(before).filter(e => !ignorable(e));
            if (newErrors.length) broken.push(`${type}: ${newErrors[0]}`);
        }
        expect(broken).toEqual([]);
    });

    for (const flow of ['others', 'file_processor']) {
        test(`outbound (${flow}): every connector type in Target Config renders its config form`, async ({ page }) => {
            const errors = [];
            await openWizard(page, errors);
            await fillStep1(page, { source: 'hl7v2', flow });
            await clickNext(page);

            const select = page.locator('#wizardOutboundConnectorContainer #connectorTypeSelect');
            await expect(select).toBeVisible({ timeout: 10000 });
            const types = await select.locator('option').evaluateAll(
                os => os.map(o => o.value).filter(v => v && v !== '_custom_'));
            expect(types.length).toBeGreaterThan(5);

            const broken = [];
            for (const type of types) {
                const before = errors.length;
                await select.selectOption(type);
                await page.waitForTimeout(150);
                const html = await page.locator('#wizardOutboundConnectorContainer').innerHTML();
                if (/&lt;i |&lt;\/?(div|span|i)\b/.test(html)) broken.push(`${type}: escaped markup`);
                const newErrors = errors.slice(before).filter(e => !ignorable(e));
                if (newErrors.length) broken.push(`${type}: ${newErrors[0]}`);
            }
            expect(broken).toEqual([]);
        });
    }
});
