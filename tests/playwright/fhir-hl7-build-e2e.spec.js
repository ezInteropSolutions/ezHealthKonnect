'use strict';
/**
 * fhir-hl7-build-e2e.spec.js — fhir.build / hl7.build no-code builder E2E tests.
 *
 * Covers both the pipeline-builder UI (step builder dispatch + catalog-backed
 * rendering) and the API path (schema catalogs + Test Pipeline execution),
 * mirroring cda-fhir-pipeline.spec.js's structure for the CDA equivalents.
 *
 * Run: npx playwright test fhir-hl7-build-e2e --project=chromium
 *
 * Env overrides:
 *   BASE_URL  default: http://localhost:3000
 *
 * Note: Test Pipeline execution calls go through BASE_URL (the Node.js
 * frontend's proxy), NOT a direct Go backend port — the Go backend enforces
 * its own auth when hit directly, which the Node proxy already satisfies
 * upstream of the /api/fhir/* passthrough.
 */

const { test, expect } = require('@playwright/test');
const { ApiHelper }    = require('./helpers/api');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';

const state = { interfaceId: null, ids: [] };

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 0: Schema API health checks (no pipeline required)
// ─────────────────────────────────────────────────────────────────────────────

test.describe('FHB-E0 FHIR/HL7 Schema API Health', () => {

    test('FHB-E0-001 GET /api/fhir/resource-types returns a non-empty list including Patient', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/fhir/resource-types?version=R4`);
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(Array.isArray(body.resourceTypes)).toBe(true);
        expect(body.resourceTypes).toContain('Patient');
    });

    test('FHB-E0-002 GET /api/fhir/canonical-fields/Patient returns birthDate with stripped prefix', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/fhir/canonical-fields/Patient?profile=base&version=R4`);
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        const keys = body.fields.map(f => f.key);
        expect(keys).toContain('birthDate');
        expect(keys.some(k => k.startsWith('Patient.'))).toBe(false);
    });

    test('FHB-E0-003 GET /api/fhir/transforms returns the DeclarativeTransformRegistry catalog', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/fhir/transforms`);
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.transforms).toHaveProperty('string_direct');
    });

    test('FHB-E0-004 GET /api/hl7/message-types returns ADT^A01', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/hl7/message-types?version=2.5.1`);
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.messageTypes.some(mt => mt.messageType === 'ADT' && mt.triggerEvent === 'A01')).toBe(true);
    });

    test('FHB-E0-005 GET /api/hl7/segments/ADT/A01 includes PID', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/hl7/segments/ADT/A01?version=2.5.1`);
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.segments).toContain('PID');
    });

    test('FHB-E0-006 GET /api/hl7/canonical-fields/ADT/A01/PID includes PID.5.1', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/hl7/canonical-fields/ADT/A01/PID?version=2.5.1`);
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        const keys = body.fields.map(f => f.key);
        expect(keys).toContain('PID.5.1');
    });

    test('FHB-E0-007 unknown FHIR resource type returns 404', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/fhir/canonical-fields/NotARealResourceType?version=R4`);
        expect(res.status()).toBe(404);
    });

    test('FHB-E0-008 unknown HL7 message type returns 404', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/hl7/segments/ZZZ/Z99?version=2.5.1`);
        expect(res.status()).toBe(404);
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 1: Pipeline builder UI — step dispatch + catalog-backed rendering
// ─────────────────────────────────────────────────────────────────────────────

test.describe('FHB-E1 Pipeline Builder UI', () => {

    // POST /api/interfaces currently 500s on a pre-existing, unrelated bug
    // ("Named replacement \":sourceType\" has no entry in the replacement
    // map") — out of scope here. Reuse the known ADT^A01 test interface
    // (see CLAUDE.md / project memory) instead of creating/deleting a new one.
    test.beforeAll(async ({ request }) => {
        const api = new ApiHelper(request, BASE_URL);
        const KNOWN_TEST_INTERFACE_ID = '762aebb9-0408-4a42-82c5-202f13f28315';
        try {
            const all = await api.listInterfaces();
            if (all.some(i => i.id === KNOWN_TEST_INTERFACE_ID)) {
                state.interfaceId = KNOWN_TEST_INTERFACE_ID;
            }
        } catch (_) { /* leave state.interfaceId null — tests skip */ }
    });

    test('FHB-E1-001 Add fhir.build step — builder renders with resource-type dropdown', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({ stepType: 'fhir.build', stepName: 'Build FHIR Patient', config: {} });
        });
        await page.waitForTimeout(300);

        const hasStep = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            return steps.some(s => (s.stepType || s.step_type) === 'fhir.build');
        });
        expect(hasStep).toBe(true);

        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => (st.stepType || st.step_type) === 'fhir.build');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(800);

        const builder = page.locator('#fhirBuildBuilder');
        await expect(builder).toBeVisible({ timeout: 3000 });
        await expect(page.locator('#fbbResourceType')).toBeVisible();
        await expect(page.locator('#fbbOutputField')).toBeVisible();
        // Live-fetched resource-type catalog should have populated real options
        const optionCount = await page.locator('#fbbResourceType option').count();
        expect(optionCount).toBeGreaterThan(1);
    });

    test('FHB-E1-002 fhir.build Fields table datalist reflects live Patient catalog', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({ stepType: 'fhir.build', stepName: 'Build FHIR Patient', config: { resourceType: 'Patient' } });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => (st.stepType || st.step_type) === 'fhir.build');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1000);

        const builder = page.locator('#fhirBuildBuilder');
        await expect(builder).toBeVisible({ timeout: 3000 });

        const datalistOptions = await page.locator('#fbbFieldTargets option').count();
        expect(datalistOptions).toBeGreaterThan(0);
    });

    test('FHB-E1-003 Add hl7.build step — builder renders with message-type dropdown', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({ stepType: 'hl7.build', stepName: 'Build HL7 Message', config: {} });
        });
        await page.waitForTimeout(300);

        const hasStep = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            return steps.some(s => (s.stepType || s.step_type) === 'hl7.build');
        });
        expect(hasStep).toBe(true);

        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => (st.stepType || st.step_type) === 'hl7.build');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(800);

        const builder = page.locator('#hl7BuildBuilder');
        await expect(builder).toBeVisible({ timeout: 3000 });
        await expect(page.locator('#hbbMessageType')).toBeVisible();
        await expect(page.locator('#hbbOutputField')).toBeVisible();
        const optionCount = await page.locator('#hbbMessageType option').count();
        expect(optionCount).toBeGreaterThan(1);
    });

    test('FHB-E1-004 Add segment to hl7.build step and confirm it is tracked in config', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({
                stepType: 'hl7.build', stepName: 'Build HL7 Message',
                config: { messageType: 'ADT', triggerEvent: 'A01', version: '2.5.1' },
            });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => (st.stepType || st.step_type) === 'hl7.build');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1000);

        const builder = page.locator('#hl7BuildBuilder');
        await expect(builder).toBeVisible({ timeout: 3000 });

        const addSegmentBtn = page.locator('button:has-text("Add Segment")');
        await expect(addSegmentBtn).toBeVisible({ timeout: 2000 });
        await addSegmentBtn.click();
        await page.waitForTimeout(300);
        const segCount = await page.evaluate(() => {
            const w = window._hl7BuildBuilder;
            return w?._step?.config?.segments?.length ?? 0;
        });
        expect(segCount).toBeGreaterThan(0);
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 2: Test Pipeline execution API (Go backend) — proven-working payloads
// ─────────────────────────────────────────────────────────────────────────────

test.describe('FHB-E2 Test Pipeline Execution API', () => {

    test('FHB-E2-001 fhir.build builds a Patient with scalar + valueMap + repeating identifier', async ({ request }) => {
        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: JSON.stringify({
                    dob: '1980-05-20', sexCode: 'F',
                    patientIdentifiers: [{ idSystem: 'http://hospital.example.org/mrn', idValue: '12345' }],
                }),
                pipeline: {
                    interfaceId: 'fhb-e2e-fhir', messageType: 'TEST',
                    execution_groups: [{
                        steps: [{
                            id: '11111111-1111-1111-1111-111111111111',
                            stepName: 'Build FHIR Patient', stepType: 'fhir.build', sequence: 10, enabled: true,
                            config: {
                                resourceType: 'Patient', outputField: 'message.fhirResource',
                                fields: [
                                    { targetPath: 'birthDate', sourcePath: 'message.dob' },
                                    { targetPath: 'gender', sourcePath: 'message.sexCode', transform: 'string_direct', valueMap: { F: 'female', M: 'male' } },
                                ],
                                repeatingGroups: [{
                                    targetPath: 'identifier', rowsPath: 'message.patientIdentifiers',
                                    fields: [{ targetPath: 'system', sourcePath: 'idSystem' }, { targetPath: 'value', sourcePath: 'idValue' }],
                                }],
                            },
                        }],
                    }],
                },
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        const resource = body.steps.build_fhir_patient.step_output.fhir_resource;
        expect(resource.birth_date).toBe('1980-05-20');
        expect(resource.gender).toBe('female');
        expect(resource.identifier[0].value).toBe('12345');
    });

    test('FHB-E2-002 hl7.build builds an ORU^R01 with PID + repeating OBX', async ({ request }) => {
        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: JSON.stringify({
                    mrn: 'MRN123', lastName: 'Doe', firstName: 'Jane',
                    labResults: [{ testCode: 'GLU', value: '95' }, { testCode: 'K', value: '4.1' }],
                }),
                pipeline: {
                    interfaceId: 'fhb-e2e-hl7', messageType: 'TEST',
                    execution_groups: [{
                        steps: [{
                            id: '11111111-1111-1111-1111-111111111111',
                            stepName: 'Build HL7 Message', stepType: 'hl7.build', sequence: 10, enabled: true,
                            config: {
                                messageType: 'ORU', triggerEvent: 'R01', version: '2.5.1', outputField: 'message.hl7Message',
                                segments: [
                                    { segment: 'PID', cardinality: 'single', fields: [
                                        { fieldKey: 'PID.3', sourcePath: 'message.mrn' },
                                        { fieldKey: 'PID.5.1', sourcePath: 'message.lastName' },
                                        { fieldKey: 'PID.5.2', sourcePath: 'message.firstName' },
                                    ]},
                                    { segment: 'OBX', cardinality: 'repeating', rowsPath: 'message.labResults', fields: [
                                        { fieldKey: 'OBX.3', sourcePath: 'testCode' },
                                        { fieldKey: 'OBX.5', sourcePath: 'value' },
                                    ]},
                                ],
                            },
                        }],
                    }],
                },
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        const msg = body.steps.build_hl7_message.step_output.hl7_message;
        expect(msg).toContain('MSH|^~\\&|');
        expect(msg).toContain('ORU^R01');
        expect(msg).toContain('PID|||MRN123||Doe^Jane');
        expect(msg).toContain('OBX|||GLU||95');
        expect(msg).toContain('OBX|||K||4.1');
    });

    test('FHB-E2-003 cda.map_to_canonical -> cda.build chain works with message.-prefixed paths', async ({ request }) => {
        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: JSON.stringify({
                    records: [{ icd10Code: 'E11.9', description: 'Type 2 diabetes' }],
                    patientFirstName: 'Jane',
                }),
                pipeline: {
                    interfaceId: 'fhb-e2e-cda-chain', messageType: 'TEST',
                    execution_groups: [{
                        steps: [
                            {
                                id: '11111111-1111-1111-1111-111111111111',
                                stepName: 'Map To Canonical', stepType: 'cda.map_to_canonical', sequence: 10, enabled: true,
                                config: {
                                    outputField: 'message.parsedCDA',
                                    header: [{ group: 'patient', target: 'firstName', sourcePath: 'message.patientFirstName' }],
                                    sections: [{ sectionKey: 'problems', rowsPath: 'message.records', fields: [{ canonicalField: 'conditionCode', sourcePath: 'icd10Code' }] }],
                                },
                            },
                            {
                                id: '22222222-2222-2222-2222-222222222222',
                                stepName: 'Build CDA', stepType: 'cda.build', sequence: 20, enabled: true,
                                config: { sourceField: 'message.parsedCDA', outputField: 'message.cdaXML', documentType: 'CCD' },
                            },
                        ],
                    }],
                },
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.steps.map_to_canonical.step_output.parsed_cda.sections.problems.entries[0].condition_code).toBe('E11.9');
        expect(body.steps.build_cda.step_output.cda_xml).toContain('ClinicalDocument');
        expect(body.steps.build_cda.step_output.cda_xml).toContain('E11.9');
    });

});
