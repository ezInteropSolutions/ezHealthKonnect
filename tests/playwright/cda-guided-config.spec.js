'use strict';
/**
 * cda-guided-config.spec.js — spec-guided configuration for cda.map_to_canonical
 * and cda.build (July 2026 guided-config feature).
 *
 * Covers: the document-type requirements catalog API, cda.map_to_canonical's
 * Document Type selector + SHALL pre-population + completeness banner, and
 * cda.build's tabbed Requirements/Custodian UI including its live cross-step
 * read of a sibling cda.map_to_canonical step's config.
 *
 * Run: npx playwright test cda-guided-config --project=chromium
 *
 * Env overrides:
 *   BASE_URL  default: http://localhost:3000
 */

const { test, expect } = require('@playwright/test');
const { ApiHelper }    = require('./helpers/api');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';

const state = { interfaceId: null, ids: [] };

// Steps always live at pipeline.executionGroups[i].steps[j] in this app's
// runtime model (never a flat pipeline.steps array — confirmed against
// LayerContainer.addStep, which writes new steps straight into
// executionGroups). Every page.evaluate() below that needs to find a step
// inlines this same 4-line lookup (can't share a Node closure across the
// isolated browser context) — kept intentionally small/duplicated rather
// than reconstructed via eval() of a shared string.

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 0: Requirements catalog API
// ─────────────────────────────────────────────────────────────────────────────

test.describe('CDA-GC0 Requirements Catalog API', () => {

    test('CDA-GC0-001 GET /api/cda/document-types returns a non-empty list including CCD', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/cda/document-types`);
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(Array.isArray(body.documentTypes)).toBe(true);
        expect(body.documentTypes).toContain('CCD');
    });

    test('CDA-GC0-002 GET /api/cda/document-types/CCD/requirements returns 7 SHALL sections + 3 header groups', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/cda/document-types/CCD/requirements`);
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        const { requirements } = body;
        expect(requirements.documentType).toBe('CCD');

        const shallSections = requirements.sections.filter(s => s.conformance === 'SHALL');
        expect(shallSections.length).toBe(7);

        for (const group of ['patient', 'author', 'custodian']) {
            expect(Array.isArray(requirements.headerGroups[group])).toBe(true);
            expect(requirements.headerGroups[group].length).toBeGreaterThan(0);
        }
        // custodian's own address/telecom aren't asserted requirements — see
        // cda/builder/header_requirements.go's doc comment.
        const custodianKeys = requirements.headerGroups.custodian.map(f => f.key);
        expect(custodianKeys).not.toContain('address.street');
        expect(custodianKeys).not.toContain('phone');
    });

    test('CDA-GC0-003 GET /api/cda/document-types/:unknown/requirements returns 404', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/cda/document-types/Not-A-Real-Type/requirements`);
        expect(res.status()).toBe(404);
    });

    test('CDA-GC0-004 CCD and Discharge Summary have different SHALL section sets', async ({ request }) => {
        const [ccdRes, dsRes] = await Promise.all([
            request.get(`${BASE_URL}/api/cda/document-types/CCD/requirements`),
            request.get(`${BASE_URL}/api/cda/document-types/Discharge Summary/requirements`),
        ]);
        expect(ccdRes.ok()).toBeTruthy();
        expect(dsRes.ok()).toBeTruthy();
        const ccdShall = (await ccdRes.json()).requirements.sections.filter(s => s.conformance === 'SHALL').map(s => s.key).sort();
        const dsShall = (await dsRes.json()).requirements.sections.filter(s => s.conformance === 'SHALL').map(s => s.key).sort();
        expect(ccdShall).not.toEqual(dsShall);
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 1: Pipeline builder UI
// ─────────────────────────────────────────────────────────────────────────────

test.describe('CDA-GC1 Pipeline Builder UI', () => {

    test.beforeAll(async ({ request }) => {
        const api = new ApiHelper(request, BASE_URL);
        try {
            const all = await api.listInterfaces();
            const stale = all.filter(i => (i.name || '').startsWith('CDA_GC_E2E'));
            for (const iface of stale) await api.deleteInterface(iface.id).catch(() => {});
        } catch (_) { /* ignore cleanup errors */ }

        const res = await request.post(`${BASE_URL}/api/interfaces`, {
            data: {
                name: 'CDA_GC_E2E_Test',
                description: 'CDA guided-config E2E test interface',
                direction: 'inbound',
                status: 'inactive',
                message_type: 'CCD',
                sourceType: 'file_listener',
                targetType: 'http_outbound',
            },
        });
        if (res.ok()) {
            const body = await res.json();
            state.interfaceId = body.id || (body.data && body.data.id) || (body.interface && body.interface.id);
            if (state.interfaceId) state.ids.push(state.interfaceId);
        }
    });

    test.afterAll(async ({ request }) => {
        const api = new ApiHelper(request, BASE_URL);
        for (const id of state.ids) await api.deleteInterface(id).catch(() => {});
    });

    test('CDA-GC1-001 cda.map_to_canonical: Document Type defaults to CCD and pre-populates SHALL sections/header rows', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        // window.pipelineBuilder exists before window.pipelineBuilder.pipeline
        // finishes its own async load — wait for the pipeline itself, not
        // just the builder object, or addStep() throws on a null pipeline.
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder.addStep({ stepType: 'cda.map_to_canonical', name: 'Map To Canonical', config: {} });
        });
        await page.waitForTimeout(300);

        // Steps live at pipeline.executionGroups[i].steps[j], and the panel
        // opens via propertiesPanel.showStepProperties(step) — see this
        // file's header comment.
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const groups = pb.pipeline.executionGroups || [];
            const step = groups.flatMap(g => g.steps || []).find(s => (s.stepType || s.step_type) === 'cda.map_to_canonical');
            pb.propertiesPanel.showStepProperties(step);
        });
        await page.waitForTimeout(500);

        await expect(page.locator('#mapToCanonicalBuilder')).toBeVisible({ timeout: 3000 });

        // Document Type select defaults to CCD.
        await expect(page.locator('#m2cDocumentType')).toHaveValue('CCD');

        // Give the requirements fetch + pre-population a moment.
        await page.waitForTimeout(2000);

        // Pre-population is additive config state, not just DOM — assert
        // against the pipeline model directly (robust to markup changes).
        const cfg = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const groups = pb.pipeline.executionGroups || [];
            const step = groups.flatMap(g => g.steps || []).find(s => (s.stepType || s.step_type) === 'cda.map_to_canonical');
            return step.config;
        });
        const shallSectionKeys = ['allergiesAndIntolerances', 'medications', 'problems', 'results', 'planOfTreatment', 'socialHistory', 'vitalSigns'];
        for (const key of shallSectionKeys) {
            expect(cfg.sections.some(s => s.sectionKey === key)).toBe(true);
        }
        expect(cfg.header.some(h => h.group === 'patient' && h.target === 'firstName')).toBe(true);
        expect(cfg.header.some(h => h.group === 'author' && h.target === 'given')).toBe(true);

        // Completeness banner reflects the (still-unmapped) state.
        await expect(page.locator('#mapToCanonicalBuilder')).toContainText('required (SHALL) items configured');
    });

    test('CDA-GC1-002 cda.build: tabbed Requirements/Custodian UI renders and Requirements tab reads sibling live config', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline, { timeout: 8000 });

        // Ensure both steps exist (idempotent — CDA-GC1-001 may have already added map_to_canonical).
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const findStep = t => (pb.pipeline.executionGroups || []).flatMap(g => g.steps || []).find(s => (s.stepType || s.step_type) === t);
            if (!findStep('cda.map_to_canonical')) {
                pb.addStep({
                    stepType: 'cda.map_to_canonical', name: 'Map To Canonical',
                    config: { outputField: 'parsedCDA', header: [], sections: [{ sectionKey: 'problems', rowsPath: 'records', fields: [{ canonicalField: 'conditionCode', sourcePath: 'icd10Code' }] }] },
                });
            }
            if (!findStep('cda.build')) {
                pb.addStep({ stepType: 'cda.build', name: 'Build CDA', config: { documentType: 'CCD' } });
            }
        });
        await page.waitForTimeout(300);

        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const step = (pb.pipeline.executionGroups || []).flatMap(g => g.steps || []).find(s => (s.stepType || s.step_type) === 'cda.build');
            pb.propertiesPanel.showStepProperties(step);
        });
        await page.waitForTimeout(500);

        await expect(page.locator('#cdaBuildBuilder')).toBeVisible({ timeout: 3000 });

        await expect(page.locator('#cdaBuildTabBtn-general')).toBeVisible();
        await expect(page.locator('#cdaBuildTabBtn-requirements')).toBeVisible();
        await expect(page.locator('#cdaBuildTabBtn-custodian')).toBeVisible();

        await page.locator('#cdaBuildTabBtn-requirements').click();
        await page.waitForTimeout(1500);
        // With a sibling cda.map_to_canonical step present, the live-status
        // banner (not the "no cda.map_to_canonical step found" fallback)
        // must render, and Problem List must show as Configured.
        await expect(page.locator('#cdaBuildTab-requirements')).toContainText('Live status of');
        await expect(page.locator('#cdaBuildTab-requirements')).toContainText('Problem List');
        await expect(page.locator('#cdaBuildTab-requirements')).toContainText('Configured');

        await page.locator('#cdaBuildTabBtn-custodian').click();
        await page.waitForTimeout(300);
        await expect(page.locator('#cdaBuildCustOrgName')).toBeVisible();
        // Only Name/ID are spec-required for custodian — Street must not
        // carry a "Required" badge.
        const streetLabel = page.locator('label:has-text("Street Address")');
        await expect(streetLabel).not.toContainText('Required');
    });

    test('CDA-GC1-003 fresh render() shows Group By/Entry-Level Fields for a section that already has groupBy/entryFields set (regression)', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        // showStepProperties() always destroys the previous activeStepBuilder
        // and constructs a brand-new MapToCanonicalBuilder (see
        // PropertiesPanel.js's generic StepBuilderRegistry dispatch) — so a
        // config carrying a RepeatingGroup section's groupBy/entryFields
        // BEFORE the panel is ever opened reproduces the exact bug: the
        // builder's own _repeatingGroupCatalog cache starts empty and used to
        // only ever get populated for sections added/changed via
        // onSectionKeyChange or freshly pre-populated — never for one already
        // sitting in the config, like a step loaded from a saved pipeline.
        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline, { timeout: 8000 });

        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const findStep = t => (pb.pipeline.executionGroups || []).flatMap(g => g.steps || []).find(s => (s.stepType || s.step_type) === t);
            if (!findStep('cda.map_to_canonical')) {
                pb.addStep({
                    stepType: 'cda.map_to_canonical', name: 'Map To Canonical',
                    config: { outputField: 'parsedCDA', header: [], sections: [] },
                });
            }
            const step = findStep('cda.map_to_canonical');
            if (!step.config.sections.some(s => s.sectionKey === 'vitalSigns')) {
                step.config.sections.push({
                    sectionKey: 'vitalSigns', rowsPath: 'message.vitalRecords',
                    fields: [{ canonicalField: 'vitalCode', sourcePath: 'vitalCode' }],
                    groupBy: ['panelId'], groupedItemsKey: 'components',
                    entryFields: [{ canonicalField: 'effectiveTime', sourcePath: 'obsTime' }],
                });
            }
            pb.propertiesPanel.showStepProperties(step);
        });
        await expect(page.locator('#mapToCanonicalBuilder')).toBeVisible({ timeout: 3000 });
        await page.waitForTimeout(2000);

        const idx = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const step = (pb.pipeline.executionGroups || []).flatMap(g => g.steps || []).find(s => (s.stepType || s.step_type) === 'cda.map_to_canonical');
            return step.config.sections.findIndex(s => s.sectionKey === 'vitalSigns');
        });
        const card = page.locator(`.m2c-section-card[data-section-index="${idx}"]`);
        await expect(card.locator('.m2c-section-groupby'), 'Group By input must render for an already-saved RepeatingGroup section').toBeVisible();
        await expect(card.locator('.m2c-section-groupby')).toHaveValue('panelId');
        await expect(card.getByText('Entry-Level Fields')).toBeVisible();
        await expect(card.locator('tr[data-field-kind="entry"]')).toHaveCount(1);
    });

    test('CDA-GC1-004 Test Pipeline copy buttons match the actual output (no Copy FHIR Bundle for a CDA-only pipeline, Copy CCD/CDA XML present)', async ({ page, context }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');
        await context.grantPermissions(['clipboard-read', 'clipboard-write']);

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline, { timeout: 8000 });

        // Ensure a minimal cda.map_to_canonical + cda.build pair exists (idempotent).
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const findStep = t => (pb.pipeline.executionGroups || []).flatMap(g => g.steps || []).find(s => (s.stepType || s.step_type) === t);
            if (!findStep('cda.map_to_canonical')) {
                pb.addStep({
                    stepType: 'cda.map_to_canonical', name: 'Map To Canonical',
                    config: {
                        outputField: 'message.parsedCDA', header: [],
                        sections: [{ sectionKey: 'problems', rowsPath: 'message.records', fields: [{ canonicalField: 'conditionCode', sourcePath: 'icd10Code' }] }],
                    },
                });
            }
            if (!findStep('cda.build')) {
                pb.addStep({ stepType: 'cda.build', name: 'Build CDA', config: { documentType: 'CCD', sourceField: 'message.parsedCDA', outputField: 'message.cdaXML' } });
            }
        });
        await page.waitForTimeout(300);

        // Test Pipeline resolves interface_id/message_type from a saved
        // transformation_pipelines row when no pipeline_id is already known
        // client-side — save once first so the test request can find it,
        // same as how a real user would have a persisted pipeline by the
        // time they run Test Pipeline.
        await page.locator('#savePipelineBtn').click();
        await page.waitForTimeout(1000);

        await page.locator('#testPipelineBtn').click();
        await page.locator('#testMessageInput').fill(JSON.stringify({ records: [{ icd10Code: 'E11.9', description: 'Type 2 diabetes mellitus' }] }));
        await page.locator('#testMessageFormat').selectOption('json').catch(() => {});
        await page.getByRole('button', { name: /Run Test/i }).first().click();
        await expect(page.locator('#testResultsContent')).toContainText('Test Passed', { timeout: 15000 });

        // The bug this guards against: "Copy FHIR Bundle" used to render
        // unconditionally regardless of whether the pipeline ever produces a
        // FHIR bundle — misleading for a CDA-only pipeline like this one.
        await expect(page.locator('#copyBundleBtn')).toHaveCount(0);
        await expect(page.locator('#copyCdaXmlBtn')).toBeVisible();
        await expect(page.locator('#copyResultsBtn')).toBeVisible();

        await page.locator('#copyCdaXmlBtn').click();
        const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
        expect(clipboardText).toContain('<ClinicalDocument');
    });
});
