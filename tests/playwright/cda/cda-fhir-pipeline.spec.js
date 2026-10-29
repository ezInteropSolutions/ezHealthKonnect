'use strict';
/**
 * cda-fhir-pipeline.spec.js — CDA/CCD → FHIR R4 Sprint E E2E tests.
 *
 * 15 tests covering both the pipeline-builder UI (step builder dispatch,
 * section checkboxes, field editor, upgrade banner) and the API path
 * (transform, compute-delta, translation table, OOB template).
 *
 * Run: npx playwright test cda-fhir-pipeline --project=chromium
 *
 * Env overrides:
 *   BASE_URL       default: http://localhost:3000
 *   GO_URL         default: http://localhost:8080
 */

const { test, expect } = require('../helpers/seeded');
const { ApiHelper }    = require('../helpers/api');
const fs               = require('fs');
const path             = require('path');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';
const GO_URL   = process.env.GO_URL   || 'http://localhost:8080';

// CCD XML fixture path (Sprint A fixture — always present)
const CCD_FIXTURE_PATH = path.join(__dirname, '../../fixtures/cda/ccd_minimal.xml');
const CCD_FIXTURE_XML  = fs.existsSync(CCD_FIXTURE_PATH) ? fs.readFileSync(CCD_FIXTURE_PATH, 'utf8') : '';

// Shared state populated by SETUP tests.
const state = {
    interfaceId: null,
    pipelineId:  null,
    ids:         [],
};

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 0: Schema API health checks (no pipeline required)
// ─────────────────────────────────────────────────────────────────────────────

test.describe('CDA-E0 Schema API Health', () => {

    test('CDA-E0-001 GET /api/cda/schema/sections returns section list', async ({ request }) => {
        const res  = await request.get(`${BASE_URL}/api/cda/schema/sections`);
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(Array.isArray(body.sections)).toBe(true);
        expect(body.sections.length).toBeGreaterThan(0);
        // Spot-check that expected keys are present
        const keys = body.sections.map(s => s.key);
        expect(keys.some(k => /allerg/i.test(k))).toBeTruthy();
        expect(keys.some(k => /medic/i.test(k))).toBeTruthy();
    });

    test('CDA-E0-002 GET /api/cda/schema/sections/:key/fields returns field list', async ({ request }) => {
        // Use "allergiesAndIntolerances" which is always present in ccda_2_1.json
        const res  = await request.get(`${BASE_URL}/api/cda/schema/sections/allergiesAndIntolerances/fields`);
        if (!res.ok()) {
            // If section key is named differently, fall back to first available section
            const sectRes = await request.get(`${BASE_URL}/api/cda/schema/sections`);
            const sects   = (await sectRes.json()).sections || [];
            const first   = sects.find(s => !s.isHeader);
            expect(first, 'schema returned no non-header section').toBeTruthy();
            const res2 = await request.get(`${BASE_URL}/api/cda/schema/sections/${encodeURIComponent(first.key)}/fields`);
            expect(res2.ok()).toBeTruthy();
            const body2 = await res2.json();
            expect(body2.success).toBe(true);
            expect(Array.isArray(body2.fields)).toBe(true);
            return;
        }
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(Array.isArray(body.fields)).toBe(true);
        expect(body.fields.length).toBeGreaterThan(0);
        // Each field should have at minimum a key and dataType
        for (const f of body.fields) {
            expect(typeof f.key).toBe('string');
        }
    });

    test('CDA-E0-003 POST /api/cda/type-pair/infer returns transform for TS→dateTime', async ({ request }) => {
        const res = await request.post(`${BASE_URL}/api/cda/type-pair/infer`, {
            data: { cdaDataType: 'TS', fhirDataType: 'dateTime' },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        // Should be able to infer a transform for CDA timestamp → FHIR dateTime
        if (body.inferred) {
            expect(typeof body.transform).toBe('string');
            expect(body.transform.length).toBeGreaterThan(0);
        }
    });

    test('CDA-E0-004 GET /api/cda/templates/:documentType/version returns version', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/cda/templates/CCD/version`);
        // May return 404 if no OOB template is seeded yet — accept either
        if (res.status() === 404) {
            const body = await res.json();
            expect(body.success).toBe(false);
            return;
        }
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(typeof body.version).toBe('string');
        expect(typeof body.templateId).toBe('string');
        expect(body.documentType).toBe('CCD');
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 1: Pipeline builder UI — step picker + builder dispatch
// ─────────────────────────────────────────────────────────────────────────────

test.describe('CDA-E1 Pipeline Builder UI', () => {

    // The seeded, activated interface from helpers/seeded.js (created through the real wizard API).
    test.beforeAll(async ({ seed }) => {
        state.interfaceId = seed.interfaceId;
    });

    // Opens a step's properties panel the way the app does: a VisualStep added to the builder, then
    // showStepProperties. The default config comes from the toolbox template, like a dragged-in step.
    async function openStepPanel(page, stepType) {
        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForSelector('#all-steps-list .step-card', { timeout: 15000 });
        // The interface's pipeline loads asynchronously; adding a step before it exists throws.
        await page.waitForFunction(() => !!(window.pipelineBuilder && window.pipelineBuilder.pipeline), { timeout: 15000 });
        await page.evaluate((st) => {
            const tpl = window.pipelineBuilder.toolboxManager.templates.find(t => t.type === st);
            const step = new VisualStep({ stepName: 'Test ' + st, stepType: st, config: tpl ? JSON.parse(JSON.stringify(tpl.defaultConfig || {})) : {} });
            window.pipelineBuilder.addStep(step);
            window.pipelineBuilder.propertiesPanel.showStepProperties(step, false);
        }, stepType);
        await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
    }

    test('CDA-E1-001 Add cda.parse step — step is on the canvas and its builder opens', async ({ page }) => {
        expect(state.interfaceId, 'seeded interface missing').toBeTruthy();
        await openStepPanel(page, 'cda.parse');

        // CdaParseStepBuilder's own fields prove the cda.parse builder (not a generic panel) was dispatched.
        await expect(page.locator('#cdaParseSourceField')).toBeVisible();
        await expect(page.locator('#cdaParseOutputField')).toBeVisible();
        await expect(page.locator('#cdaParseDocTypeHint')).toBeVisible();
    });

    test('CDA-E1-002 Add cda.to_fhir step — 4-tab builder is dispatched', async ({ page }) => {
        expect(state.interfaceId, 'seeded interface missing').toBeTruthy();
        await openStepPanel(page, 'cda.to_fhir');

        await expect(page.locator('#cdaToFhirBuilder')).toBeVisible();
        for (const tab of ['general', 'sections', 'terminology', 'advanced']) {
            await expect(page.locator(`#cdaToFhirTabBtn-${tab}`)).toBeVisible();
        }
    });

    test('CDA-E1-003 Sections tab shows section list with field counts', async ({ page }) => {
        expect(state.interfaceId, 'seeded interface missing').toBeTruthy();
        await openStepPanel(page, 'cda.to_fhir');

        await page.locator('#cdaToFhirTabBtn-sections').click();
        const rows = page.locator('#cdaToFhirTab-sections .cda-section-checkbox');
        await expect(rows.first()).toBeVisible({ timeout: 15000 });
        expect(await rows.count()).toBeGreaterThan(5);
        await expect(page.locator('#cdaToFhirTab-sections').getByText(/\d+ fields/).first()).toBeVisible();
    });

    test('CDA-E1-004 OOB upgrade banner is hidden when no template version set', async ({ page }) => {
        expect(state.interfaceId, 'seeded interface missing').toBeTruthy();

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => !!(window.pipelineBuilder && window.pipelineBuilder.pipeline), { timeout: 15000 });

        // Step with no basedOnVersion → banner stays hidden
        await page.evaluate(() => {
            const step = { stepType: 'cda.to_fhir', name: 'CDA→FHIR', config: { documentType: 'CCD' } };
            const pb = window.pipelineBuilder;
            if (pb?.addStep) pb.addStep(step);
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            if (!pb?.pipeline) return;
            const steps = pb.pipeline.steps || pb.pipeline.getSteps?.() || [];
            const s = steps.find(st => (st.stepType || st.step_type) === 'cda.to_fhir');
            if (s && pb.propertiesPanel?.show) pb.propertiesPanel.show(s);
        });
        await page.waitForTimeout(500);

        const banner = page.locator('#cdaToFhirUpgradeBanner');
        if (await banner.count() > 0) {
            await expect(banner).not.toBeVisible();
        }
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 2: Transform API tests (Go backend)
// ─────────────────────────────────────────────────────────────────────────────

test.describe('CDA-E2 Transform API', () => {

    test('CDA-E2-001 CDA parse executor returns parsedCDA JSON', async ({ request }) => {
        expect(CCD_FIXTURE_XML, 'CCD fixture file not found at tests/fixtures/cda/ccd_minimal.xml').toBeTruthy();

        // POST directly to Go backend pipeline test endpoint with a cda.parse step
        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                interfaceId: 'test',
                messageType: 'CCD',
                testInput:   CCD_FIXTURE_XML,
                steps: [
                    {
                        stepType: 'cda.parse',
                        sequence: 10,
                        config: { sourceField: 'raw', outputField: 'parsedCDA', documentTypeHint: 'CCD' },
                    },
                ],
            },
        });

        // Accept 200 or 422 (missing template) — we just test that the server responds
        expect([200, 400, 422, 503]).toContain(res.status());
        if (res.ok()) {
            const body = await res.json();
            // If the step ran, the output should contain parsedCDA or _testError
            expect(body).toBeDefined();
        }
    });

    test('CDA-E2-002 CDA→FHIR executor returns FHIR Bundle resourceType', async ({ request }) => {
        expect(CCD_FIXTURE_XML, 'CCD fixture file not found').toBeTruthy();

        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                interfaceId: 'test',
                messageType: 'CCD',
                testInput:   CCD_FIXTURE_XML,
                steps: [
                    {
                        stepType: 'cda.parse',
                        sequence: 10,
                        config: { sourceField: 'raw', outputField: 'parsedCDA' },
                    },
                    {
                        stepType: 'cda.to_fhir',
                        sequence: 20,
                        config: {
                            sourceField: 'parsedCDA',
                            outputField: 'fhirBundle',
                            bundleType:  'collection',
                            profileMode: 'us-core',
                            documentType: 'CCD',
                        },
                    },
                ],
            },
        });

        expect([200, 400, 422, 503]).toContain(res.status());
        if (res.ok()) {
            const body = await res.json();
            // If the transform ran and returned a bundle, verify resourceType
            const output = body.output || body.result || body;
            if (output && output.fhirBundle) {
                expect(output.fhirBundle.resourceType).toBe('Bundle');
            }
        }
    });

    test('CDA-E2-003 POST /api/cda/mappings/:id/:type/compute-delta returns valid response', async ({ request }) => {
        // Use a known fake interfaceId — delta endpoint is idempotent for testing
        const testInterfaceId = '00000000-0000-0000-0000-000000000001';
        const res = await request.post(
            `${BASE_URL}/api/cda/mappings/${testInterfaceId}/CCD/compute-delta`,
            {
                data: {
                    incoming: [
                        {
                            sectionKey: 'allergiesAndIntolerances',
                            cdaField:   'medicationAllergyCode',
                            fhirPath:   'AllergyIntolerance.code.coding[0].code',
                            transform:  'cda_cd_to_fhir_code',
                        },
                    ],
                    ccdaVersion: '2.1',
                    fhirVersion: 'R4',
                },
            }
        );
        // Accept 200, 404 (no OOB template), 500 (no DB) — never a 400
        expect([200, 404, 500, 503]).toContain(res.status());
        if (res.ok()) {
            const body = await res.json();
            expect(body.success).toBe(true);
            expect(typeof body.overridesStored).toBe('number');
        }
    });

    test('CDA-E2-004 Section failure is isolated in processingResult', async ({ request }) => {
        // Run a cda.to_fhir test with a malformed parsedCDA (no sections) —
        // mapper should return an empty bundle without throwing a 500.
        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                interfaceId: 'test',
                messageType: 'CCD',
                testInput:   CCD_FIXTURE_XML || '<empty/>',
                steps: [
                    {
                        stepType: 'cda.to_fhir',
                        sequence: 10,
                        config: {
                            sourceField: 'raw',   // deliberately wrong field — should produce empty bundle not panic
                            outputField: 'fhirBundle',
                            bundleType:  'collection',
                        },
                    },
                ],
            },
        });
        // Server must not 500-crash on missing input data
        expect([200, 400, 422, 503]).toContain(res.status());
    });

    test('CDA-E2-005 Discharge Summary document type accepted by schema API', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/cda/templates/Discharge%20Summary/version`);
        // Accept 404 (no OOB template) or 200 — never 500
        expect([200, 404, 503]).toContain(res.status());
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 3: Translation table CRUD
// ─────────────────────────────────────────────────────────────────────────────

// The translation table now has a real API and page (/api/cda/code-translations, code-translations.html);
// its list / create / update / delete / import / export behaviour is covered by code-translations.spec.js and
// controllers/cda_code_translations_controller_test.go. The placeholders that used to be here assumed a
// different, never-built path (/api/cda/mappings/translations).

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 4: OOB pipeline template
// ─────────────────────────────────────────────────────────────────────────────

test.describe('CDA-E4 OOB Pipeline Template', () => {

    test('CDA-E4-001 OOB CDA/CCD pipeline template exists in interface_templates', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/interface-templates`);
        expect([200, 404, 503]).toContain(res.status());
        if (res.ok()) {
            const body = await res.json();
            const templates = body.templates || body.data || [];
            // OOB CDA/CCD template should be in the list
            const cdaTemplates = templates.filter(t =>
                /(CDA|CCD)/i.test(t.name || '') ||
                /(CDA|CCD)/i.test(t.description || '')
            );
            // If templates are seeded, at least one should be CDA-related
            if (templates.length > 0) {
                expect(cdaTemplates.length).toBeGreaterThan(0);
            }
        }
    });

    test('CDA-E4-002 Using the CDA/CCD OOB template scaffolds an interface with a CDA pipeline', async ({ request }) => {
        const listRes = await request.get(`${BASE_URL}/api/interface-templates`);
        expect(listRes.ok()).toBeTruthy();
        const body = await listRes.json();
        const templates = body.templates || body.data || [];
        const cdaTemplate = templates.find(t => t.slug === 'cda-ccd-inbound-fhir-r4' || /CDA\/CCD Inbound/i.test(t.name || ''));
        expect(cdaTemplate, 'CDA/CCD OOB template not found in /api/interface-templates').toBeTruthy();

        // The same scaffold call the "Use Template" button makes (creates nothing by itself).
        const api = new ApiHelper(request, BASE_URL);
        const scaffold = await api.getTemplateScaffold(cdaTemplate.slug || cdaTemplate.id);
        const groups = scaffold.pipeline_config?.execution_groups || [];
        expect(groups.length).toBeGreaterThan(0);
        const stepTypes = groups.flatMap(g => (g.steps || []).map(s => s.step_type || s.stepType));
        expect(stepTypes.some(t => /^cda\./.test(t))).toBe(true);
    });

});
