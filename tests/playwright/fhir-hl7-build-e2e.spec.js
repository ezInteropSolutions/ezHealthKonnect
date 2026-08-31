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
const fs               = require('fs');
const path             = require('path');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';

const state = { interfaceId: null, ids: [] };

// Shared comprehensive test payload — the same file the FHIR_Build_Demo
// pipeline's live "Test Pipeline" panel uses, and the Go bundle-assembly
// integration test (fhir_build_bundle_assembly_test.go) loads, so there is
// exactly one source of truth for this data across rounds.
const FHIR_BUILD_DEMO_SAMPLE_MESSAGE = fs.readFileSync(
    path.join(__dirname, '../fixtures/fhir_build_demo_sample_message.json'), 'utf8'
);

// The real FHIR_Build_Demo interface/pipeline — see
// database/migrations/V198__FHIR_Build_Demo_Additional_Resources.sql.
const FHIR_BUILD_DEMO_INTERFACE_ID = '8952d74e-9c81-4bae-a1e9-abd83ca094a6';
const FHIR_BUILD_DEMO_PIPELINE_ID  = '2499db9c-6bb8-499b-85bd-a04d3ac25d82';

// The 10 fhir.build steps + 1 bundle-assembly step, byte-for-byte consistent
// with V198's INSERTs and fhir_build_bundle_assembly_test.go's Go equivalent.
function fhirBuildDemoInlineSteps() {
    const ref = (targetPath, sourcePath, prefix) => ({
        targetPath, sourcePath, transform: 'string_prefix', valueMap: { prefix },
    });
    return [
        {
            id: '11111111-1111-1111-1111-111111111101', stepName: 'Build FHIR Patient', stepType: 'fhir.build', sequence: 10, enabled: true,
            config: {
                resourceType: 'Patient', outputField: 'message.fhirPatient',
                fields: [
                    { targetPath: 'id', sourcePath: 'message.patientIdentifiers[0].idValue' },
                    { targetPath: 'birthDate', sourcePath: 'message.dob' },
                    { targetPath: 'gender', sourcePath: 'message.sexCode', transform: 'string_direct', valueMap: { F: 'female', M: 'male' } },
                ],
                repeatingGroups: [{
                    targetPath: 'identifier', rowsPath: 'message.patientIdentifiers',
                    fields: [{ targetPath: 'system', sourcePath: 'idSystem' }, { targetPath: 'value', sourcePath: 'idValue' }],
                }],
            },
        },
        {
            id: '11111111-1111-1111-1111-111111111102', stepName: 'Build FHIR Encounter', stepType: 'fhir.build', sequence: 20, enabled: true,
            config: {
                resourceType: 'Encounter', outputField: 'message.fhirEncounter',
                fields: [
                    { targetPath: 'id', sourcePath: 'message.encounterId' },
                    { targetPath: 'status', literalValue: 'finished' },
                    { targetPath: 'class.system', literalValue: 'http://terminology.hl7.org/CodeSystem/v3-ActCode' },
                    { targetPath: 'class.code', literalValue: 'AMB' },
                    ref('subject.reference', 'message.patientIdentifiers[0].idValue', 'Patient/'),
                ],
            },
        },
        {
            id: '11111111-1111-1111-1111-111111111103', stepName: 'Build FHIR Condition', stepType: 'fhir.build', sequence: 30, enabled: true,
            config: {
                resourceType: 'Condition', outputField: 'message.fhirCondition',
                fields: [
                    { targetPath: 'clinicalStatus.coding[0].system', literalValue: 'http://terminology.hl7.org/CodeSystem/condition-clinical' },
                    { targetPath: 'clinicalStatus.coding[0].code', literalValue: 'active' },
                    { targetPath: 'code.coding[0].system', sourcePath: 'message.conditionCodeSystem' },
                    { targetPath: 'code.coding[0].code', sourcePath: 'message.conditionCode' },
                    ref('subject.reference', 'message.patientIdentifiers[0].idValue', 'Patient/'),
                ],
            },
        },
        {
            id: '11111111-1111-1111-1111-111111111104', stepName: 'Build FHIR Observation', stepType: 'fhir.build', sequence: 40, enabled: true,
            config: {
                resourceType: 'Observation', outputField: 'message.fhirObservation',
                fields: [
                    { targetPath: 'status', literalValue: 'final' },
                    { targetPath: 'code.coding[0].system', literalValue: 'http://loinc.org' },
                    { targetPath: 'code.coding[0].code', sourcePath: 'message.observationCode' },
                    // Round-2 additions: LOINC 8310-5 auto-selects FHIR's
                    // built-in "bodytemp" vital-signs profile, which requires
                    // category/effective[x]/valueQuantity.system+code.
                    { targetPath: 'category[0].coding[0].system', literalValue: 'http://terminology.hl7.org/CodeSystem/observation-category' },
                    { targetPath: 'category[0].coding[0].code', literalValue: 'vital-signs' },
                    { targetPath: 'category[0].coding[0].display', literalValue: 'Vital Signs' },
                    { targetPath: 'effectiveDateTime', sourcePath: 'message.observationDate' },
                    { targetPath: 'valueQuantity.value', sourcePath: 'message.observationValue', transform: 'cda_decimal_string_to_number' },
                    { targetPath: 'valueQuantity.unit', sourcePath: 'message.observationUnit' },
                    { targetPath: 'valueQuantity.system', literalValue: 'http://unitsofmeasure.org' },
                    { targetPath: 'valueQuantity.code', sourcePath: 'message.observationUnit' },
                    ref('subject.reference', 'message.patientIdentifiers[0].idValue', 'Patient/'),
                    ref('encounter.reference', 'message.encounterId', 'Encounter/'),
                ],
            },
        },
        {
            id: '11111111-1111-1111-1111-111111111105', stepName: 'Build FHIR MedicationRequest', stepType: 'fhir.build', sequence: 50, enabled: true,
            config: {
                resourceType: 'MedicationRequest', outputField: 'message.fhirMedicationRequest',
                fields: [
                    { targetPath: 'status', literalValue: 'active' },
                    { targetPath: 'intent', literalValue: 'order' },
                    { targetPath: 'medicationCodeableConcept.coding[0].system', literalValue: 'http://www.nlm.nih.gov/research/umls/rxnorm' },
                    { targetPath: 'medicationCodeableConcept.coding[0].code', sourcePath: 'message.drugCode' },
                    ref('subject.reference', 'message.patientIdentifiers[0].idValue', 'Patient/'),
                ],
            },
        },
        {
            id: '11111111-1111-1111-1111-111111111106', stepName: 'Build FHIR AllergyIntolerance', stepType: 'fhir.build', sequence: 60, enabled: true,
            config: {
                resourceType: 'AllergyIntolerance', outputField: 'message.fhirAllergyIntolerance',
                fields: [
                    { targetPath: 'clinicalStatus.coding[0].system', literalValue: 'http://terminology.hl7.org/CodeSystem/allergyintolerance-clinical' },
                    { targetPath: 'clinicalStatus.coding[0].code', literalValue: 'active' },
                    { targetPath: 'code.coding[0].system', literalValue: 'http://snomed.info/sct' },
                    { targetPath: 'code.coding[0].code', sourcePath: 'message.allergenCode' },
                    ref('patient.reference', 'message.patientIdentifiers[0].idValue', 'Patient/'),
                ],
            },
        },
        {
            id: '11111111-1111-1111-1111-111111111107', stepName: 'Build FHIR Immunization', stepType: 'fhir.build', sequence: 70, enabled: true,
            config: {
                resourceType: 'Immunization', outputField: 'message.fhirImmunization',
                fields: [
                    { targetPath: 'status', literalValue: 'completed' },
                    { targetPath: 'vaccineCode.coding[0].system', literalValue: 'http://hl7.org/fhir/sid/cvx' },
                    { targetPath: 'vaccineCode.coding[0].code', sourcePath: 'message.vaccineCode' },
                    { targetPath: 'occurrenceDateTime', sourcePath: 'message.immunizationDate' },
                    ref('patient.reference', 'message.patientIdentifiers[0].idValue', 'Patient/'),
                ],
            },
        },
        {
            id: '11111111-1111-1111-1111-111111111108', stepName: 'Build FHIR Practitioner', stepType: 'fhir.build', sequence: 80, enabled: true,
            config: {
                resourceType: 'Practitioner', outputField: 'message.fhirPractitioner',
                fields: [
                    { targetPath: 'identifier[0].system', literalValue: 'http://hl7.org/fhir/sid/us-npi' },
                    { targetPath: 'identifier[0].value', sourcePath: 'message.practitionerNPI' },
                    { targetPath: 'name[0].family', sourcePath: 'message.practitionerFamily' },
                    { targetPath: 'name[0].given[0]', sourcePath: 'message.practitionerGiven' },
                ],
            },
        },
        {
            id: '11111111-1111-1111-1111-111111111109', stepName: 'Build FHIR Organization', stepType: 'fhir.build', sequence: 90, enabled: true,
            config: {
                resourceType: 'Organization', outputField: 'message.fhirOrganization',
                fields: [
                    { targetPath: 'identifier[0].system', literalValue: 'http://hl7.org/fhir/sid/us-npi' },
                    { targetPath: 'identifier[0].value', sourcePath: 'message.orgNPI' },
                    { targetPath: 'name', sourcePath: 'message.orgName' },
                ],
            },
        },
        {
            id: '11111111-1111-1111-1111-111111111110', stepName: 'Build FHIR Location', stepType: 'fhir.build', sequence: 100, enabled: true,
            config: {
                resourceType: 'Location', outputField: 'message.fhirLocation',
                fields: [
                    { targetPath: 'status', literalValue: 'active' },
                    { targetPath: 'name', sourcePath: 'message.locationName' },
                ],
            },
        },
        {
            id: '11111111-1111-1111-1111-111111111111', stepName: 'Assemble Bundle', stepType: 'payload.builder', sequence: 110, enabled: true,
            config: {
                mode: 'fhir_bundle',
                fhirBundle: {
                    bundleType: 'collection',
                    resourcePaths: [
                        'message.fhirPatient', 'message.fhirEncounter', 'message.fhirCondition',
                        'message.fhirObservation', 'message.fhirMedicationRequest', 'message.fhirAllergyIntolerance',
                        'message.fhirImmunization', 'message.fhirPractitioner', 'message.fhirOrganization', 'message.fhirLocation',
                    ],
                },
            },
        },
    ];
}

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

    test('FHB-E0-009 GET /api/hl7/versions returns known versions, no leftover "v" prefix', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/hl7/versions`);
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.versions).toContain('2.5.1');
        expect(body.versions.some(v => v.startsWith('v'))).toBe(false);
    });

    test('FHB-E0-010 GET /api/hl7/schema-tree/ADT/A01 returns EVN/PID/PV1 as the required spine', async ({ request }) => {
        const res = await request.get(`${BASE_URL}/api/hl7/schema-tree/ADT/A01?version=2.5.1`);
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.requiredSpine).toEqual(['EVN', 'PID', 'PV1']);
        // NK1 sits inside the tree (as a repeatable, non-required node) —
        // confirms the group structure survived, not just a flat list.
        const findNode = (nodes, name) => {
            for (const n of nodes || []) {
                if (n.name === name) return n;
                const found = findNode(n.children, name);
                if (found) return found;
            }
            return null;
        };
        const nk1 = findNode(body.tree, 'NK1');
        expect(nk1).toBeTruthy();
        expect(nk1.canRepeat).toBe(true);
        expect(nk1.required).toBe(false);
    });

    test('FHB-E0-011 POST /api/hl7/next-segments/ADT/A01 is forward-only and offers the override list', async ({ request }) => {
        const fresh = await request.post(`${BASE_URL}/api/hl7/next-segments/ADT/A01?version=2.5.1`, { data: { addedSegments: [] } });
        expect(fresh.ok()).toBeTruthy();
        const freshBody = await fresh.json();
        expect(freshBody.success).toBe(true);
        // EVN is the first mandatory checkpoint — PID isn't reachable yet.
        expect(freshBody.allowed).toContain('EVN');
        expect(freshBody.allowed).not.toContain('PID');
        expect(freshBody.all).toContain('PID');
        expect(freshBody.all).not.toContain('MSH');

        // NK1 (sequence 7) actually precedes PV1 (sequence 8) in the real
        // ADT_A01 structure, so it must NOT reappear once PV1 is matched —
        // the guardrail is strictly forward-only, never backtracking.
        const afterSpine = await request.post(`${BASE_URL}/api/hl7/next-segments/ADT/A01?version=2.5.1`, {
            data: { addedSegments: ['EVN', 'PID', 'PV1'] },
        });
        expect(afterSpine.ok()).toBeTruthy();
        const afterBody = await afterSpine.json();
        expect(afterBody.allowed).toContain('PV2');
        expect(afterBody.allowed).not.toContain('EVN');
        expect(afterBody.allowed).not.toContain('NK1');
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

    test('FHB-E1-005 fhir.build required-elements banner reflects fhir/r4 schema and updates as fields are mapped', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        // Encounter has several required elements per fhir/r4's compiled
        // schema, both top-level (class, status) and nested (classHistory.*,
        // location.location) — the banner counts all of them, not just
        // top-level, so this test asserts the relative change (mapping one
        // required field increases the satisfied count by exactly one)
        // rather than hardcoding the exact total, which would be brittle
        // against schema changes.
        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({ stepType: 'fhir.build', stepName: 'Build FHIR Encounter Reqs', config: { resourceType: 'Encounter' } });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build FHIR Encounter Reqs');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1200); // field catalog fetch + rerender

        const builder = page.locator('#fhirBuildBuilder');
        await expect(builder).toBeVisible({ timeout: 3000 });

        const parseBanner = (text) => {
            const m = text.match(/(\d+) of (\d+) required elements mapped/);
            return m ? { satisfied: parseInt(m[1], 10), total: parseInt(m[2], 10) } : null;
        };

        let bannerText = await builder.innerText();
        const before = parseBanner(bannerText);
        expect(before).not.toBeNull();
        expect(before.satisfied).toBe(0);
        expect(before.total).toBeGreaterThanOrEqual(2); // at least class + status
        expect(bannerText).toContain('Still missing');
        expect(bannerText).toContain('class');
        expect(bannerText).toContain('status');

        // Map one required field and confirm the satisfied count increases
        // by exactly one, with the total unchanged.
        await page.evaluate(() => window._fhirBuildBuilder.addField());
        await page.waitForTimeout(200);
        await page.locator('#fbbFieldsContainer .fbb-field-target').first().fill('status');
        await page.evaluate(() => { window._fhirBuildBuilder.addField(); window._fhirBuildBuilder.removeField(-1, 1); });
        await page.waitForTimeout(300);

        bannerText = await builder.innerText();
        const after = parseBanner(bannerText);
        expect(after).not.toBeNull();
        expect(after.satisfied).toBe(before.satisfied + 1);
        expect(after.total).toBe(before.total);
    });

    test('FHB-E1-005b fhir.build completeness banner is cardinality-aware — required children of an unconfigured OPTIONAL group are not flagged', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        // Patient/us-core: communication/link/telecom are all optional (0..*)
        // BackboneElements whose OWN children (communication.language,
        // link.other, link.type, telecom.system, telecom.value) are required
        // WITHIN any entry that exists — but omitting the whole group is
        // valid FHIR. Regression coverage for the false positive reported
        // against this exact banner: it was flagging those nested fields as
        // "still missing" even though neither communication nor link had been
        // touched at all.
        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({ stepType: 'fhir.build', stepName: 'Build FHIR Patient Cardinality', config: { resourceType: 'Patient', profile: 'us-core' } });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build FHIR Patient Cardinality');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1200);

        const builder = page.locator('#fhirBuildBuilder');
        await expect(builder).toBeVisible({ timeout: 3000 });

        // Nothing under communication/link/telecom is configured — none of
        // their required children should appear in "Still missing", and the
        // root groups themselves (never required on their own) shouldn't
        // either.
        let bannerText = await builder.innerText();
        expect(bannerText).not.toContain('communication.language');
        expect(bannerText).not.toContain('link.other');
        expect(bannerText).not.toContain('link.type');
        expect(bannerText).not.toContain('telecom.system');
        expect(bannerText).not.toContain('telecom.value');

        const parseBanner = (text) => {
            const m = text.match(/(\d+) of (\d+) required elements mapped/);
            return m ? { satisfied: parseInt(m[1], 10), total: parseInt(m[2], 10) } : null;
        };
        const before = parseBanner(bannerText);
        expect(before).not.toBeNull();

        // Now configure a "communication" repeating group and map its
        // required "language" field — communication.language becomes
        // reachable (the group is in use) and should now count toward the
        // total, then be satisfied once mapped.
        // onGroupTargetChange is wired to the input's native "change" event
        // (see _renderGroupCard) — Playwright's .fill() only dispatches
        // "input", not "change", so it never fires from a plain .fill().
        // Calling the handler directly is what actually exercises the same
        // sync+rerender path a real onchange would.
        await page.evaluate(() => window._fhirBuildBuilder.addRepeatingGroup());
        await page.waitForTimeout(200);
        await page.evaluate(() => window._fhirBuildBuilder.onGroupTargetChange(0, 'communication'));
        await page.waitForTimeout(300);

        bannerText = await builder.innerText();
        const afterGroupOnly = parseBanner(bannerText);
        expect(afterGroupOnly).not.toBeNull();
        // The group exists but its field isn't filled in yet — now flagged.
        expect(bannerText).toContain('communication.language');
        expect(afterGroupOnly.total).toBe(before.total + 1);

        await page.evaluate(() => window._fhirBuildBuilder.addGroupField(0));
        await page.waitForTimeout(200);
        await page.locator('.fbb-group-card[data-group-index="0"] .fbb-field-target').first().fill('language');
        await page.evaluate(() => {
            window._fhirBuildBuilder.addGroupField(0);
            window._fhirBuildBuilder.removeField(0, 1);
        });
        await page.waitForTimeout(300);

        bannerText = await builder.innerText();
        const afterMapped = parseBanner(bannerText);
        expect(afterMapped).not.toBeNull();
        expect(afterMapped.total).toBe(before.total + 1);
        expect(afterMapped.satisfied).toBe(before.satisfied + 1);
        expect(bannerText).not.toContain('communication.language');
    });

    test('FHB-E1-006 fhir.build extension picker adds a field row with the correct predicate path and no transform', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({ stepType: 'fhir.build', stepName: 'Build FHIR Patient Ext', config: { resourceType: 'Patient' } });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build FHIR Patient Ext');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1000);

        const builder = page.locator('#fhirBuildBuilder');
        await expect(builder).toBeVisible({ timeout: 3000 });

        await page.evaluate(() => window._fhirBuildBuilder.toggleExtensionPanel());
        await page.waitForTimeout(200);
        await page.locator('#fbbExtSearch').fill('race');
        await page.waitForTimeout(200);
        await expect(page.locator('.fbb-ext-row', { hasText: 'Race' })).toBeVisible({ timeout: 2000 });
        await page.locator('.fbb-ext-row', { hasText: 'Race' }).click();
        await page.waitForTimeout(300);

        const lastField = await page.evaluate(() => {
            const fields = window._fhirBuildBuilder._step.config.fields;
            return fields[fields.length - 1];
        });
        expect(lastField.targetPath).toBe('extension[url=http://hl7.org/fhir/us/core/StructureDefinition/us-core-race].valueCodeableConcept');
        expect(lastField.transform).toBeUndefined();
        expect(lastField.sourcePath).toBe('');
    });

    test('FHB-E1-007 fhir.build "Validate Against Sample" (strict mode) catches an invalid bound code and flags the right row', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({ stepType: 'fhir.build', stepName: 'Build FHIR Patient Validate', config: { resourceType: 'Patient' } });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build FHIR Patient Validate');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1000);

        const builder = page.locator('#fhirBuildBuilder');
        await expect(builder).toBeVisible({ timeout: 3000 });

        // gender is bound (required, AdministrativeGender) even on the base
        // profile — a real, already-compiled fixture (see
        // fhir/builder/canonical_field_catalog_test.go's
        // TestFieldCatalog_IncludesBindingInfo). "XYZ" is not a member.
        await page.evaluate(() => window._fhirBuildBuilder.addField());
        await page.waitForTimeout(200);
        await page.locator('#fbbFieldsContainer .fbb-field-target').first().fill('gender');
        await page.locator('#fbbFieldsContainer .fbb-field-literal').first().fill('XYZ');
        await page.evaluate(() => { window._fhirBuildBuilder.addField(); window._fhirBuildBuilder.removeField(-1, 1); });
        await page.waitForTimeout(300);

        await page.evaluate(() => window._fhirBuildBuilder.toggleValidatePanel());
        await page.waitForTimeout(200);
        await page.locator('#fbbValidateSample').fill('{}');

        const [response] = await Promise.all([
            page.waitForResponse(r => r.url().includes('/api/fhir/pipeline/test') && r.request().method() === 'POST'),
            page.getByRole('button', { name: 'Validate', exact: true }).click(),
        ]);
        expect(response.ok()).toBeTruthy();
        await page.waitForTimeout(500);

        const result = await page.evaluate(() => window._fhirBuildBuilder._validateResult);
        expect(result).not.toBeNull();
        expect(result.errorCount).toBeGreaterThan(0);

        const issuesByRowKey = await page.evaluate(() => window._fhirBuildBuilder._validateIssueByRowKey);
        expect(issuesByRowKey['top:0']).toBeTruthy();
        expect(issuesByRowKey['top:0'].some(i => i.severity === 'error')).toBe(true);

        // The row itself renders a visible error badge.
        const badge = page.locator('#fbbFieldsContainer tr[data-field-index="0"] span[title]');
        await expect(badge).toBeVisible({ timeout: 2000 });
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

    test('FHB-E1-008 hl7.build version dropdown is schema-backed, not free text', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({ stepType: 'hl7.build', stepName: 'Build HL7 Version Test', config: {} });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build HL7 Version Test');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1200);

        const versionEl = page.locator('#hbbVersion');
        await expect(versionEl).toBeVisible({ timeout: 3000 });
        expect(await versionEl.evaluate(el => el.tagName)).toBe('SELECT');
        const versionOptions = await versionEl.locator('option').allTextContents();
        expect(versionOptions).toContain('2.5.1');
    });

    test('FHB-E1-009 fresh hl7.build step for ADT^A01 auto-seeds EVN/PID/PV1', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({
                stepType: 'hl7.build', stepName: 'Build HL7 Autoseed Test',
                config: { messageType: 'ADT', triggerEvent: 'A01', version: '2.5.1' },
            });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build HL7 Autoseed Test');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1500); // schema-tree fetch + auto-seed + rerender

        const segments = await page.evaluate(() => window._hl7BuildBuilder?._step?.config?.segments || []);
        expect(segments.map(s => s.segment)).toEqual(['EVN', 'PID', 'PV1']);
        // Reflected in the visible UI, not just in-memory config.
        await expect(page.locator('.hbb-segment-card')).toHaveCount(3);
    });

    test('FHB-E1-010 Add Segment picker excludes already-passed segments (ordering guardrail)', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({
                stepType: 'hl7.build', stepName: 'Build HL7 Guardrail Test',
                config: { messageType: 'ADT', triggerEvent: 'A01', version: '2.5.1' },
            });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build HL7 Guardrail Test');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1500); // auto-seeds EVN/PID/PV1

        await page.locator('button:has-text("Add Segment")').click();
        await page.waitForTimeout(800); // 4th segment's picker fetches /api/hl7/next-segments

        const lastPicker = page.locator('.hbb-segment-picker select').last();
        await expect(lastPicker).toBeVisible({ timeout: 3000 });
        const optionValues = await lastPicker.locator('option').allTextContents();
        expect(optionValues).not.toContain('EVN');
        expect(optionValues).not.toContain('PID');
        expect(optionValues.some(v => v.includes('__override__') || v.includes('non-standard'))).toBe(true);
        expect(optionValues.some(v => v.includes('Custom / Z-segment'))).toBe(true);
    });

    test('FHB-E1-011 Custom / Z-segment entry warns on a non-"Z"-prefixed name', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({
                stepType: 'hl7.build', stepName: 'Build HL7 Zseg Test',
                config: { messageType: 'ADT', triggerEvent: 'A01', version: '2.5.1', segments: [] },
            });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build HL7 Zseg Test');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1500);

        await page.locator('button:has-text("Add Segment")').click();
        await page.waitForTimeout(800);

        const picker = page.locator('.hbb-segment-picker select').last();
        await expect(picker).toBeVisible({ timeout: 3000 });
        await picker.selectOption('__custom__');
        await page.waitForTimeout(200);

        const customInput = page.locator('.hbb-segment-picker .hl7sp-custom-input').last();
        await expect(customInput).toBeVisible({ timeout: 2000 });
        await customInput.fill('XYZ');
        await customInput.dispatchEvent('change');
        await page.waitForTimeout(500);

        const warningText = await page.locator('.hbb-segment-picker .hl7sp-warning').last().innerText();
        expect(warningText).toContain('Non-standard');
    });

    test('FHB-E1-012 Field-key cell is a live-catalog-backed smart search (type "SSN", get PID.19)', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({
                stepType: 'hl7.build', stepName: 'Build HL7 Field Picker Test',
                config: { messageType: 'ADT', triggerEvent: 'A01', version: '2.5.1' },
            });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build HL7 Field Picker Test');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1500); // auto-seeds EVN/PID/PV1

        // PID is the second auto-seeded segment (index 1) — add a field to it.
        await page.evaluate(() => {
            window._hl7BuildBuilder.addSegmentField(1);
        });
        await page.waitForTimeout(600); // field catalog fetch + rerender + picker mount

        const fieldInput = page.locator('.hbb-segment-card[data-path="1"] .hl7fp-input').first();
        await expect(fieldInput).toBeVisible({ timeout: 3000 });

        // Search by real schema LABEL text, not the key itself — proves this
        // is genuinely catalog-driven search, not just a glorified dropdown.
        await fieldInput.fill('SSN');
        await page.waitForTimeout(300);
        const match = page.locator('.field-path-item', { hasText: 'SSN' }).first();
        await expect(match).toBeVisible({ timeout: 2000 });
        await match.click();
        await page.waitForTimeout(300);

        const fieldKey = await page.evaluate(() => window._hl7BuildBuilder._step.config.segments[1].fields[0].fieldKey);
        expect(fieldKey).toBe('PID.19');
    });

    test('FHB-E1-013 Cardinality dropdown offers "Repeating" only for segments the schema allows to repeat', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({
                stepType: 'hl7.build', stepName: 'Build HL7 Cardinality Gate Test',
                // Bypass the ordering-guardrail picker (tested separately) to
                // directly exercise the cardinality-gating render logic: PID
                // (own repeat="1", no repeating ancestor) vs. NK1 (own
                // repeat="*") — confirmed against the real ADT_A01 schema.
                config: { messageType: 'ADT', triggerEvent: 'A01', version: '2.5.1', segments: [
                    { segment: 'PID', cardinality: 'single', fields: [] },
                    { segment: 'NK1', cardinality: 'single', fields: [] },
                ] },
            });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build HL7 Cardinality Gate Test');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1500); // schema-tree fetch + rerender

        // PID can't repeat at all — no interactive dropdown is rendered for
        // it, just a greyed-out fixed "Single (fixed)" badge.
        const pidFixed = page.locator('.hbb-segment-card[data-path="0"] .hbb-segment-cardinality-fixed');
        await expect(pidFixed).toBeVisible({ timeout: 3000 });
        await expect(pidFixed).toHaveText(/Single/);
        expect(await page.locator('.hbb-segment-card[data-path="0"] select.hbb-segment-cardinality').count()).toBe(0);

        // NK1 can repeat — a real dropdown offering "Repeating" is rendered.
        const nk1Cardinality = page.locator('.hbb-segment-card[data-path="1"] select.hbb-segment-cardinality');
        await expect(nk1Cardinality).toBeVisible({ timeout: 3000 });
        const nk1Options = await nk1Cardinality.locator('option').allTextContents();
        expect(nk1Options).toContain('Repeating');
    });

    test('FHB-E1-014 Regression: PID does not offer "Repeating" for ORU^R01 even though the grandparent group repeats', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        // ORU_R01's outer "PATIENT RESULT" group repeats (batched
        // multi-patient results), but PID's own immediate parent, "PATIENT",
        // does not — a real bug (caught via manual testing, not by the
        // original unit tests) let the grandparent's repeat flag leak two
        // levels down and made PID incorrectly offer "Repeating" in the UI.
        // OBX (immediate parent OBSERVATION DOES repeat) must still show it.
        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({
                stepType: 'hl7.build', stepName: 'Build HL7 ORU Grandparent Regression Test',
                config: { messageType: 'ORU', triggerEvent: 'R01', version: '2.5.1', segments: [
                    { segment: 'PID', cardinality: 'single', fields: [] },
                    { segment: 'OBX', cardinality: 'single', fields: [] },
                ] },
            });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build HL7 ORU Grandparent Regression Test');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1500);

        const pidFixed = page.locator('.hbb-segment-card[data-path="0"] .hbb-segment-cardinality-fixed');
        await expect(pidFixed).toBeVisible({ timeout: 3000 });
        expect(await page.locator('.hbb-segment-card[data-path="0"] select.hbb-segment-cardinality').count()).toBe(0);

        const obxCardinality = page.locator('.hbb-segment-card[data-path="1"] select.hbb-segment-cardinality');
        await expect(obxCardinality).toBeVisible({ timeout: 3000 });
        const obxOptions = await obxCardinality.locator('option').allTextContents();
        expect(obxOptions).toContain('Repeating');
    });

    test('FHB-E1-015 Segment-level "+ Add condition" creates a condition editor and updates config', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({
                stepType: 'hl7.build', stepName: 'Build HL7 Segment Condition Test',
                config: { messageType: 'ADT', triggerEvent: 'A01', version: '2.5.1', segments: [
                    { segment: 'IN1', cardinality: 'single', fields: [] },
                ] },
            });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build HL7 Segment Condition Test');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1200);

        await page.locator('.hbb-segment-card[data-path="0"] .hbb-add-condition').click();
        await page.waitForTimeout(300);

        const editor = page.locator('.hbb-segment-card[data-path="0"] .hbb-condition-editor[data-kind="segment"]');
        await expect(editor).toBeVisible({ timeout: 2000 });
        await editor.locator('.hbb-condition-field').fill('message.hasInsurance');
        await editor.locator('.hbb-condition-operator').selectOption('equals');
        await editor.locator('.hbb-condition-value').fill('true');
        await page.waitForTimeout(200);

        // Condition inputs sync into config the same way sourcePath/literalValue/
        // etc. already do — on the next sync-triggering action (Save calls
        // collectConfig -> _syncDOMToConfig internally); call it directly here
        // to read the same state a real Save would persist.
        const condition = await page.evaluate(() => {
            window._hl7BuildBuilder._syncDOMToConfig();
            return window._hl7BuildBuilder._step.config.segments[0].condition;
        });
        expect(condition).toEqual({ field: 'message.hasInsurance', operator: 'equals', value: 'true' });
    });

    test('FHB-E1-016 Field-level condition toggle (⚡) creates a per-field condition without affecting other fields', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({
                stepType: 'hl7.build', stepName: 'Build HL7 Field Condition Test',
                config: { messageType: 'ADT', triggerEvent: 'A01', version: '2.5.1', segments: [
                    { segment: 'PID', cardinality: 'single', fields: [
                        { fieldKey: 'PID.3', sourcePath: 'mrn' },
                        { fieldKey: 'PID.19', sourcePath: 'ssn' },
                    ] },
                ] },
            });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build HL7 Field Condition Test');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1200);

        // Toggle the condition panel on PID.19 (field index 1) only.
        await page.evaluate(() => window._hl7BuildBuilder.toggleFieldConditionPanel(0, 1));
        await page.waitForTimeout(300);

        const editor = page.locator('.hbb-condition-editor[data-kind="field"][data-field-index="1"]');
        await expect(editor).toBeVisible({ timeout: 2000 });
        await editor.locator('.hbb-condition-field').fill('message.country');
        await editor.locator('.hbb-condition-operator').selectOption('equals');
        await editor.locator('.hbb-condition-value').fill('US');
        await page.waitForTimeout(200);

        const fields = await page.evaluate(() => {
            window._hl7BuildBuilder._syncDOMToConfig();
            return window._hl7BuildBuilder._step.config.segments[0].fields;
        });
        expect(fields[1].condition).toEqual({ field: 'message.country', operator: 'equals', value: 'US' });
        expect(fields[0].condition).toBeUndefined();
    });

    test('FHB-E1-017 "+ Add Child Segment" nests a new segment under its parent, addressed by path', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({
                stepType: 'hl7.build', stepName: 'Build HL7 Child Segment Test',
                config: { messageType: 'ORU', triggerEvent: 'R01', version: '2.5.1', segments: [
                    { segment: 'OBR', cardinality: 'repeating', rowsPath: 'message.orders', groupBy: ['orderId'], fields: [] },
                ] },
            });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build HL7 Child Segment Test');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1200);

        // The parent OBR card (path "0") shows the Group By input, since it's
        // repeating with groupBy already configured.
        const groupByInput = page.locator('.hbb-segment-card[data-path="0"] .hbb-segment-groupby');
        await expect(groupByInput).toBeVisible({ timeout: 3000 });
        expect(await groupByInput.inputValue()).toBe('orderId');

        // "+ Add Child Segment" on the OBR card creates a nested card at path "0,0".
        await page.locator('.hbb-segment-card[data-path="0"] > .hbb-child-segments button:has-text("Add Child Segment")').click();
        await page.waitForTimeout(500);

        const childCard = page.locator('.hbb-segment-card[data-path="0,0"]');
        await expect(childCard).toBeVisible({ timeout: 3000 });

        // The child's segment picker runs unrestricted (no root-level ordering
        // guardrail) — select OBX directly and confirm it lands in
        // segments[0].childSegments[0], not the root segments array.
        const childPicker = childCard.locator('.hbb-segment-picker select').first();
        await expect(childPicker).toBeVisible({ timeout: 2000 });
        await childPicker.selectOption('OBX');
        await page.waitForTimeout(400);

        const config = await page.evaluate(() => window._hl7BuildBuilder._step.config);
        expect(config.segments).toHaveLength(1);
        expect(config.segments[0].segment).toBe('OBR');
        expect(config.segments[0].childSegments).toHaveLength(1);
        expect(config.segments[0].childSegments[0].segment).toBe('OBX');
    });

    test('FHB-E1-018 Removing a child segment only removes that child, not the parent or its siblings', async ({ page }) => {
        test.skip(!state.interfaceId, 'Interface creation failed in beforeAll');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.interfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.evaluate(() => {
            window.pipelineBuilder?.addStep?.({
                stepType: 'hl7.build', stepName: 'Build HL7 Remove Child Test',
                config: { messageType: 'ORU', triggerEvent: 'R01', version: '2.5.1', segments: [
                    { segment: 'OBR', cardinality: 'repeating', rowsPath: 'message.orders', fields: [], childSegments: [
                        { segment: 'NTE', cardinality: 'single', fields: [] },
                        { segment: 'OBX', cardinality: 'repeating', rowsPath: 'results', fields: [] },
                    ] },
                ] },
            });
        });
        await page.waitForTimeout(300);
        await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => st.stepName === 'Build HL7 Remove Child Test');
            if (s && pb.propertiesPanel?.showStepProperties) pb.propertiesPanel.showStepProperties(s);
        });
        await page.waitForTimeout(1200);

        await expect(page.locator('.hbb-segment-card[data-path="0,0"]')).toBeVisible({ timeout: 3000 });
        await expect(page.locator('.hbb-segment-card[data-path="0,1"]')).toBeVisible({ timeout: 3000 });

        // Remove the first child (NTE, path "0,0").
        await page.locator('.hbb-segment-card[data-path="0,0"] > .hbb-card-header button[title="Remove segment"]').click();
        await page.waitForTimeout(400);

        const config = await page.evaluate(() => window._hl7BuildBuilder._step.config);
        expect(config.segments).toHaveLength(1); // parent untouched
        expect(config.segments[0].childSegments).toHaveLength(1); // one child removed
        expect(config.segments[0].childSegments[0].segment).toBe('OBX'); // the surviving one
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
        // birthDate stays camelCase — fhir_resource's own internal FHIR field
        // names must never be snake_cased (models/output_normalizer.go
        // preserves them the same way fhirBundle already was), or the output
        // stops being valid FHIR JSON.
        expect(resource.birthDate).toBe('1980-05-20');
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

    test('FHB-E2-002b hl7.build Condition: per-row skip + field-level fallback-to-top-level, end to end', async ({ request }) => {
        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: JSON.stringify({
                    country: 'US', mrn: 'MRN123', ssn: '123-45-6789',
                    labResults: [
                        { testCode: 'GLU', value: '95', status: 'final' },
                        { testCode: 'K', value: '4.1', status: 'cancelled' },
                    ],
                }),
                pipeline: {
                    interfaceId: 'fhb-e2e-hl7-condition', messageType: 'TEST',
                    execution_groups: [{
                        steps: [{
                            id: '11111111-1111-1111-1111-111111111111',
                            stepName: 'Build HL7 Message', stepType: 'hl7.build', sequence: 10, enabled: true,
                            config: {
                                messageType: 'ORU', triggerEvent: 'R01', version: '2.5.1', outputField: 'message.hl7Message',
                                segments: [
                                    { segment: 'PID', cardinality: 'single', fields: [
                                        { fieldKey: 'PID.3', sourcePath: 'message.mrn' },
                                        // PID.19 only when message.country == 'US' — set() here, so the
                                        // condition should PASS and PID.19 should be populated.
                                        { fieldKey: 'PID.19', sourcePath: 'message.ssn',
                                          condition: { field: 'message.country', operator: 'equals', value: 'US' } },
                                    ]},
                                    // Per-row condition: cancelled results excluded, final ones kept.
                                    { segment: 'OBX', cardinality: 'repeating', rowsPath: 'message.labResults',
                                      condition: { field: 'status', operator: 'not_equals', value: 'cancelled' },
                                      fields: [
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
        expect(msg).toContain('PID|||MRN123||||||||||||||||123-45-6789');
        expect(msg).toContain('OBX|||GLU||95');
        expect(msg).not.toContain('OBX|||K||4.1');
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

    test('FHB-E2-004 fhir.build x10 + payload.builder assembles a 10-entry Bundle with resolving references', async ({ request }) => {
        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: FHIR_BUILD_DEMO_SAMPLE_MESSAGE,
                pipeline: {
                    interfaceId: 'fhb-e2e-fhir-bundle', messageType: 'TEST',
                    execution_groups: [{ steps: fhirBuildDemoInlineSteps() }],
                },
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);

        // Every fhir.build step's own resource builds correctly.
        expect(body.steps.build_fhir_patient.step_output.fhir_resource.id).toBe('12345');
        expect(body.steps.build_fhir_encounter.step_output.fhir_resource.subject.reference).toBe('Patient/12345');
        expect(body.steps.build_fhir_observation.step_output.fhir_resource.encounter.reference).toBe('Encounter/enc-001');

        const bundle = JSON.parse(body.steps.assemble_bundle.step_output.payload);
        expect(bundle.resourceType).toBe('Bundle');
        expect(bundle.entry).toHaveLength(10);
        expect(bundle.total).toBeUndefined(); // bun-1: collection bundles must not carry total

        const resourceTypes = bundle.entry.map(e => e.resource.resourceType);
        for (const rt of ['Patient', 'Encounter', 'Condition', 'Observation', 'MedicationRequest',
                           'AllergyIntolerance', 'Immunization', 'Practitioner', 'Organization', 'Location']) {
            expect(resourceTypes).toContain(rt);
        }

        // Round-2 regression guard: a real external validator run flagged
        // every entry as missing fullUrl (and every relative reference as
        // therefore unresolvable) — every entry must have one now. Round 3:
        // fullUrl assignment moved to fhir/r4.AssembleEntries (the same
        // already-validated logic services/cda_fhir's Bundle assembly uses),
        // which assigns a fresh urn:uuid: to every entry uniformly and
        // rewrites every "ResourceType/id" reference to match — so fullUrls
        // are random per run, and the real assertion is that the relative
        // reference built in the Encounter step above now resolves to the
        // Patient entry's own (rewritten) fullUrl.
        for (const entry of bundle.entry) {
            expect(entry.fullUrl).toBeTruthy();
            expect(entry.fullUrl.startsWith('urn:uuid:')).toBe(true);
        }
        const patientEntry = bundle.entry.find(e => e.resource.resourceType === 'Patient');
        const encounterEntry = bundle.entry.find(e => e.resource.resourceType === 'Encounter');
        expect(encounterEntry.resource.subject.reference).toBe(patientEntry.fullUrl);

        // Round-2 regression guard: Observation's vital-signs profile fields.
        const observation = bundle.entry.find(e => e.resource.resourceType === 'Observation').resource;
        expect(observation.category[0].coding[0].code).toBe('vital-signs');
        expect(observation.effectiveDateTime).toBe('2026-01-15T09:30:00Z');
        expect(observation.valueQuantity.system).toBe('http://unitsofmeasure.org');
        expect(observation.valueQuantity.code).toBe('Cel');

        // outbound_payloads fallback: this pipeline has no connector.outbound
        // step, so payload.builder's own output must be surfaced instead.
        expect(body.outbound_payloads).toHaveLength(1);
        expect(body.outbound_payloads[0].step_name).toBe('assemble_bundle');
        expect(body.outbound_payloads[0].connector_type).toBe('');
        expect(body.outbound_payloads[0].is_json).toBe(true);
        expect(JSON.parse(body.outbound_payloads[0].content).resourceType).toBe('Bundle');
    });

    test('FHB-E2-005 real FHIR_Build_Demo pipeline (DB-persisted) builds the same 10-entry Bundle', async ({ request }) => {
        // V198__FHIR_Build_Demo_Additional_Resources.sql's own header is
        // explicit: it "no-ops safely if the FHIR_Build_Demo interface/
        // pipeline doesn't exist in this environment (fresh install without
        // demo data)" — this interface/pipeline was never actually created by
        // any migration (V198/199/200 only ever UPDATE it), so a genuinely
        // fresh database has none. The endpoint reflects that correctly:
        // 200 OK with an empty pipelines array, not a non-2xx. The skip-guard
        // used to check only res.ok(), which is true either way — it never
        // actually skipped on a fresh DB, it just let the test run into a
        // predictable failure a few lines down. Checking the real payload
        // fixes that; an inner retry (tried previously) couldn't have helped
        // since this isn't transient.
        const pipelineCheck = await request.get(`${BASE_URL}/api/pipelines/interface/${FHIR_BUILD_DEMO_INTERFACE_ID}`).catch(() => null);
        const pipelineBody = pipelineCheck && pipelineCheck.ok() ? await pipelineCheck.json().catch(() => null) : null;
        const hasPipeline = !!pipelineBody?.pipelines?.length;
        test.skip(!hasPipeline, 'FHIR_Build_Demo pipeline not present in this environment (never migration-created — see V198\'s own header comment)');

        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                pipeline_id: FHIR_BUILD_DEMO_PIPELINE_ID,
                test_message: FHIR_BUILD_DEMO_SAMPLE_MESSAGE,
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);

        const bundleStepKey = Object.keys(body.steps).find(k => body.steps[k]?.step_output?.payload);
        expect(bundleStepKey).toBeTruthy();
        const bundle = JSON.parse(body.steps[bundleStepKey].step_output.payload);
        expect(bundle.resourceType).toBe('Bundle');
        expect(bundle.entry).toHaveLength(10);
        for (const entry of bundle.entry) {
            expect(entry.fullUrl).toBeTruthy();
        }
    });

    test('FHB-E2-006 file_parser -> hl7.build: flat CSV of two lab panels becomes OBR/OBX with correct nesting and order', async ({ request }) => {
        // Same CBC/CMP flat-rows scenario as the Go unit test
        // TestHL7Build_GroupBy_GroupsFlatCSVIntoOBRWithChildOBX, but driven through
        // a REAL file_parser step (actual CSV text, not a hand-built rows array) so
        // the full file_parser -> hl7.build chain is exercised end to end, including
        // the "steps.<alias>.step_output.<field>" cross-step reference convention.
        const csvContent = [
            'orderId,testName,analyte,value',
            '1,CBC,WBC,5.4',
            '1,CBC,RBC,4.8',
            '2,CMP,Glucose,95',
            '2,CMP,Sodium,140',
        ].join('\n');

        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: JSON.stringify({ csvContent }),
                pipeline: {
                    interfaceId: 'fhb-e2e-csv-to-hl7', messageType: 'TEST',
                    execution_groups: [{
                        steps: [
                            {
                                id: '11111111-1111-1111-1111-111111111111',
                                stepName: 'Parse CSV', stepType: 'file_parser', sequence: 10, enabled: true,
                                // file_parser's sourceField resolves via executors.GetNestedValue,
                                // which auto-unwraps the "message" envelope internally — unlike
                                // sourcePath/rowsPath elsewhere (executors.GetFieldValue), this one
                                // must NOT be given the "message." prefix.
                                config: { sourceField: 'csvContent', fileFormat: 'csv', hasHeader: true },
                            },
                            {
                                id: '22222222-2222-2222-2222-222222222222',
                                stepName: 'Build HL7 Message', stepType: 'hl7.build', sequence: 20, enabled: true,
                                config: {
                                    messageType: 'ORU', triggerEvent: 'R01', version: '2.5.1', outputField: 'message.hl7Message',
                                    segments: [
                                        {
                                            segment: 'OBR', cardinality: 'repeating',
                                            rowsPath: 'steps.parse_csv.step_output.records',
                                            // NOT 'orderId'/'testName': NormalizeStepOutput snake_cases
                                            // every multi-word key inside a prior step's output — including
                                            // keys nested inside an array of records, not just the top-level
                                            // step_output keys — before a later step ever sees it. The raw
                                            // CSV header is camelCase; by the time hl7.build reads
                                            // steps.parse_csv.step_output.records, each row's keys are
                                            // order_id/test_name. 'analyte'/'value' are single-word so they
                                            // pass through unchanged, which is exactly what makes this easy
                                            // to miss when only some columns are multi-word.
                                            groupBy: ['order_id'],
                                            fields: [
                                                { fieldKey: 'OBR.1', sourcePath: 'order_id' },
                                                { fieldKey: 'OBR.2', sourcePath: 'test_name' },
                                            ],
                                            childSegments: [{
                                                segment: 'OBX', cardinality: 'repeating', rowsPath: '_rows',
                                                fields: [
                                                    { fieldKey: 'OBX.3', sourcePath: 'analyte' },
                                                    { fieldKey: 'OBX.5', sourcePath: 'value' },
                                                ],
                                            }],
                                        },
                                    ],
                                },
                            },
                        ],
                    }],
                },
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);

        // file_parser genuinely parsed the CSV text into 4 records, 4 columns.
        expect(body.steps.parse_csv.step_output.record_count).toBe(4);
        expect(body.steps.parse_csv.step_output.column_count).toBe(4);

        // hl7.build consumed those records via steps.parse_csv.step_output.records —
        // GroupBy bucketed them into 2 OBRs, each with its own OBX children
        // interleaved right after it (not all OBX appended at the end).
        const msg = body.steps.build_hl7_message.step_output.hl7_message;
        expect(msg).toContain('ORU^R01');
        const lines = msg.replace(/\r\n?$/, '').split(/\r\n?/);
        const dataLines = lines.slice(1); // drop MSH
        expect(dataLines).toEqual([
            'OBR|1|CBC',
            'OBX|||WBC||5.4',
            'OBX|||RBC||4.8',
            'OBR|2|CMP',
            'OBX|||Glucose||95',
            'OBX|||Sodium||140',
        ]);
    });

    test('FHB-E2-007 file_parser -> hl7.build: ADT^A01 admit with PID/PV1 plus a CSV-driven INSURANCE group (IN1/IN2/IN3)', async ({ request }) => {
        // Patient/visit fields come from the test message directly (there's
        // exactly one patient per admit — no CSV needed for those); the
        // INSURANCE group is the part that's naturally tabular (one row per
        // coverage), so that's what's driven through a real CSV via
        // file_parser — same GroupBy + multi-child-segment shape as
        // TestHL7Build_GroupBy_MultipleSiblingChildren_SingleAndRepeatingMixed
        // (services/executors/transform/hl7_build_executor_test.go), just
        // sourced from actual CSV text instead of a hand-built rows array.
        const csvContent = [
            'planId,groupNumber,certNumber',
            'P1,G100,C1',
            'P1,G100,C2',
            'P2,G200,C3',
        ].join('\n');

        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: JSON.stringify({
                    mrn: 'MRN500', lastName: 'Smith', firstName: 'Robert', admitType: 'I', csvContent,
                }),
                pipeline: {
                    interfaceId: 'fhb-e2e-adt-insurance', messageType: 'TEST',
                    execution_groups: [{
                        steps: [
                            {
                                id: '11111111-1111-1111-1111-111111111111',
                                stepName: 'Parse CSV', stepType: 'file_parser', sequence: 10, enabled: true,
                                config: { sourceField: 'csvContent', fileFormat: 'csv', hasHeader: true },
                            },
                            {
                                id: '22222222-2222-2222-2222-222222222222',
                                stepName: 'Build HL7 Message', stepType: 'hl7.build', sequence: 20, enabled: true,
                                config: {
                                    messageType: 'ADT', triggerEvent: 'A01', version: '2.5.1', outputField: 'message.hl7Message',
                                    segments: [
                                        { segment: 'PID', cardinality: 'single', fields: [
                                            { fieldKey: 'PID.3', sourcePath: 'message.mrn' },
                                            { fieldKey: 'PID.5.1', sourcePath: 'message.lastName' },
                                            { fieldKey: 'PID.5.2', sourcePath: 'message.firstName' },
                                        ]},
                                        { segment: 'PV1', cardinality: 'single', fields: [
                                            { fieldKey: 'PV1.2', sourcePath: 'message.admitType' },
                                        ]},
                                        {
                                            segment: 'IN1', cardinality: 'repeating',
                                            rowsPath: 'steps.parse_csv.step_output.records',
                                            // Same snake_case-in-transit gotcha as FHB-E2-006's
                                            // order_id/test_name: planId/groupNumber/certNumber
                                            // become plan_id/group_number/cert_number by the time
                                            // hl7.build reads them.
                                            groupBy: ['plan_id'],
                                            fields: [{ fieldKey: 'IN1.2', sourcePath: 'plan_id' }],
                                            childSegments: [
                                                { segment: 'IN2', cardinality: 'single',
                                                  fields: [{ fieldKey: 'IN2.1', sourcePath: 'group_number' }] },
                                                { segment: 'IN3', cardinality: 'repeating', rowsPath: '_rows',
                                                  fields: [{ fieldKey: 'IN3.1', sourcePath: 'cert_number' }] },
                                            ],
                                        },
                                    ],
                                },
                            },
                        ],
                    }],
                },
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.steps.parse_csv.step_output.record_count).toBe(3);

        const msg = body.steps.build_hl7_message.step_output.hl7_message;
        expect(msg).toContain('ADT^A01');
        const dataLines = msg.replace(/\r\n?$/, '').split(/\r\n?/).slice(1); // drop MSH
        expect(dataLines).toEqual([
            'PID|||MRN500||Smith^Robert',
            'PV1||I',
            'IN1||P1',
            'IN2|G100',
            'IN3|C1',
            'IN3|C2',
            'IN1||P2',
            'IN2|G200',
            'IN3|C3',
        ]);
    });

    test('FHB-E2-008 file_parser -> hl7.build: RDE^O11 medication order — CSV drives a 3-level ORC -> RXE -> RXR chain', async ({ request }) => {
        // Mirrors TestHL7Build_ThreeLevelChain_ORCOwnsGroupBy_OBRPassesThroughToOBX
        // (hl7_build_executor_test.go) but for a real medication order instead
        // of a lab result: ORC (schema-first, owns the GroupBy bucketing) ->
        // RXE (single — the medication itself, transparently passing its
        // bucket's rows through) -> RXR (repeating routes; order 2 has two
        // routes, order 1 has one, to prove per-order grouping, not just
        // per-row).
        const csvContent = [
            'orderId,drugCode,drugName,dose,route',
            '1,RX100,Lisinopril,10,PO',
            '2,RX200,Albuterol,2,INH',
            '2,RX200,Albuterol,2,NEB',
        ].join('\n');

        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: JSON.stringify({ csvContent }),
                pipeline: {
                    interfaceId: 'fhb-e2e-medication', messageType: 'TEST',
                    execution_groups: [{
                        steps: [
                            {
                                id: '11111111-1111-1111-1111-111111111111',
                                stepName: 'Parse CSV', stepType: 'file_parser', sequence: 10, enabled: true,
                                config: { sourceField: 'csvContent', fileFormat: 'csv', hasHeader: true },
                            },
                            {
                                id: '22222222-2222-2222-2222-222222222222',
                                stepName: 'Build HL7 Message', stepType: 'hl7.build', sequence: 20, enabled: true,
                                config: {
                                    messageType: 'RDE', triggerEvent: 'O11', version: '2.5.1', outputField: 'message.hl7Message',
                                    segments: [{
                                        segment: 'ORC', cardinality: 'repeating',
                                        rowsPath: 'steps.parse_csv.step_output.records',
                                        groupBy: ['order_id'],
                                        fields: [
                                            { fieldKey: 'ORC.1', literalValue: 'NW' },
                                            { fieldKey: 'ORC.2', sourcePath: 'order_id' },
                                        ],
                                        childSegments: [{
                                            segment: 'RXE', cardinality: 'single',
                                            fields: [
                                                { fieldKey: 'RXE.2.1', sourcePath: 'drug_code' },
                                                { fieldKey: 'RXE.2.2', sourcePath: 'drug_name' },
                                                { fieldKey: 'RXE.3', sourcePath: 'dose' },
                                            ],
                                            childSegments: [{
                                                segment: 'RXR', cardinality: 'repeating', rowsPath: '_rows',
                                                fields: [{ fieldKey: 'RXR.1', sourcePath: 'route' }],
                                            }],
                                        }],
                                    }],
                                },
                            },
                        ],
                    }],
                },
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.steps.parse_csv.step_output.record_count).toBe(3);

        const msg = body.steps.build_hl7_message.step_output.hl7_message;
        expect(msg).toContain('RDE^O11');
        const dataLines = msg.replace(/\r\n?$/, '').split(/\r\n?/).slice(1); // drop MSH
        expect(dataLines).toEqual([
            'ORC|NW|1',
            'RXE||RX100^Lisinopril|10',
            'RXR|PO',
            'ORC|NW|2',
            'RXE||RX200^Albuterol|2',
            'RXR|INH',
            'RXR|NEB',
        ]);
    });

});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 3: outbound_payloads — connector.outbound is the source of truth
// ─────────────────────────────────────────────────────────────────────────────
//
// connector.outbound's test-mode dry-run always succeeds and always resolves
// real content (see services/executors/transform/outbound_connector_executor.go
// Execute()'s test-mode branch — it validates the connector config but never
// aborts on an invalid/unreachable one, and returns nil error unconditionally),
// so these tests don't need a real reachable destination — any connectorType
// name is enough to exercise the full content-resolution + outbound_payloads
// path with zero network dependency.

test.describe('FHB-E3 outbound_payloads — connector.outbound source of truth', () => {

    function patientBuildStep() {
        return {
            id: '33333333-3333-3333-3333-333333333301', stepName: 'Build FHIR Patient', stepType: 'fhir.build', sequence: 10, enabled: true,
            config: {
                resourceType: 'Patient', outputField: 'message.fhirResource',
                fields: [{ targetPath: 'birthDate', sourcePath: 'message.dob' }],
            },
        };
    }

    test('FHB-E3-001 single connector.outbound step produces exactly one outbound_payloads entry', async ({ request }) => {
        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: JSON.stringify({ dob: '1980-05-20' }),
                pipeline: {
                    interfaceId: 'fhb-e3-single-outbound', messageType: 'TEST',
                    execution_groups: [{
                        steps: [
                            patientBuildStep(),
                            {
                                id: '33333333-3333-3333-3333-333333333302', stepName: 'Send to EHR', stepType: 'connector.outbound', sequence: 20, enabled: true,
                                config: { connectorType: 'tcp_mllp_outbound', contentField: 'message.fhirResource', contentType: 'application/fhir+json' },
                            },
                        ],
                    }],
                },
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);

        expect(body.outbound_payloads).toHaveLength(1);
        const entry = body.outbound_payloads[0];
        expect(entry.step_name).toBe('send_to_ehr');
        expect(entry.connector_type).toBe('tcp_mllp_outbound');
        expect(entry.is_json).toBe(true);
        expect(JSON.parse(entry.content).birthDate).toBe('1980-05-20');
    });

    test('FHB-E3-002 multiple connector.outbound steps (fan-out) each get their own outbound_payloads entry, in order', async ({ request }) => {
        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: JSON.stringify({ dob: '1980-05-20' }),
                pipeline: {
                    interfaceId: 'fhb-e3-multi-outbound', messageType: 'TEST',
                    execution_groups: [{
                        steps: [
                            patientBuildStep(),
                            {
                                id: '33333333-3333-3333-3333-333333333303', stepName: 'Send to EHR', stepType: 'connector.outbound', sequence: 20, enabled: true,
                                config: { connectorType: 'tcp_mllp_outbound', contentField: 'message.fhirResource', contentType: 'application/fhir+json' },
                            },
                            {
                                id: '33333333-3333-3333-3333-333333333304', stepName: 'Send to Analytics', stepType: 'connector.outbound', sequence: 30, enabled: true,
                                config: { connectorType: 'http_outbound', contentField: 'message.fhirResource', contentType: 'application/fhir+json' },
                            },
                        ],
                    }],
                },
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);

        expect(body.outbound_payloads).toHaveLength(2);
        expect(body.outbound_payloads[0].step_name).toBe('send_to_ehr');
        expect(body.outbound_payloads[0].connector_type).toBe('tcp_mllp_outbound');
        expect(body.outbound_payloads[1].step_name).toBe('send_to_analytics');
        expect(body.outbound_payloads[1].connector_type).toBe('http_outbound');
        // Both destinations receive the same resolved content in this pipeline.
        expect(JSON.parse(body.outbound_payloads[0].content).birthDate).toBe('1980-05-20');
        expect(JSON.parse(body.outbound_payloads[1].content).birthDate).toBe('1980-05-20');
    });

    test('FHB-E3-003 no steps at all → outbound_payloads absent', async ({ request }) => {
        // Nothing ran, so none of the three fallback tiers (connector.outbound,
        // payload.builder, bare build step — see FHB-E3-004) have anything to
        // report. This used to test a bare fhir.build step instead, back when
        // outbound_payloads had no third tier; now that a bare build step DOES
        // populate outbound_payloads (see below), "truly nothing happened" is
        // what this test needs to mean instead.
        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: JSON.stringify({ dob: '1980-05-20' }),
                pipeline: {
                    interfaceId: 'fhb-e3-no-outbound', messageType: 'TEST',
                    execution_groups: [{ steps: [] }],
                },
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.outbound_payloads).toBeUndefined();
    });

    test('FHB-E3-004 bare fhir.build step (no connector.outbound, no payload.builder) still populates outbound_payloads via the third fallback tier', async ({ request }) => {
        const res = await request.post(`${BASE_URL}/api/fhir/pipeline/test`, {
            data: {
                test_message: JSON.stringify({ dob: '1980-05-20' }),
                pipeline: {
                    interfaceId: 'fhb-e3-bare-fhir-build', messageType: 'TEST',
                    execution_groups: [{ steps: [patientBuildStep()] }],
                },
            },
        });
        expect(res.ok()).toBeTruthy();
        const body = await res.json();
        expect(body.success).toBe(true);
        expect(body.outbound_payloads).toHaveLength(1);
        expect(body.outbound_payloads[0].step_name).toBe('build_fhir_patient');
        expect(body.outbound_payloads[0].connector_type).toBe('');
        expect(body.outbound_payloads[0].content_type).toBe('application/fhir+json');
        expect(body.outbound_payloads[0].is_json).toBe(true);
        expect(JSON.parse(body.outbound_payloads[0].content).birthDate).toBe('1980-05-20');
    });

});
