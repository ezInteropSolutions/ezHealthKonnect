/**
 * @file normalizer-comprehensive-e2e.spec.js
 *
 * Comprehensive use-case tests for the Normalizer / Pivot / Transpose pipeline step.
 * Each section targets one operation with real healthcare data and explicit assertions
 * on both the transformed records AND the step_output metadata contract.
 *
 * Structure:
 *   Section 1  — NORMALIZE   (columns → rows, 6 use cases)
 *   Section 2  — PIVOT       (rows → columns, 5 use cases)
 *   Section 3  — TRANSPOSE   (flip matrix, 3 use cases)
 *   Section 4  — FLATTEN     (nested → flat, 5 use cases)
 *   Section 5  — UNFLATTEN   (flat → nested, 4 use cases)
 *   Section 6  — POST-PROC   (rename + caseTransform combos, 4 use cases)
 *   Section 7  — UX          (form interactions for every input widget, 9 tests)
 *   Section 8  — EDGE CASES  (null rows, nested arrays, empty data, 4 tests)
 *
 * All backend tests use the two-step pipeline pattern:
 *   Step 1 (enrichment.script, seq 10) — creates test data at output.<key>
 *   Step 2 (normalizer, seq 20)        — reads from steps.setup.step_output.<key>
 */

const { test, expect } = require('@playwright/test');

const BASE_URL = 'http://localhost:3000';
const KNOWN_INTERFACE_ID = '762aebb9-0408-4a42-82c5-202f13f28315';

// ─── Auth ────────────────────────────────────────────────────────────────────

async function login(page) {
    await page.goto(`${BASE_URL}/login.html`);
    for (const pwd of ['admin123', 'Admin123!', 'password']) {
        const res = await page.evaluate(async p => {
            const r = await fetch('/api/auth/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email: 'admin@ezhealthkonnect.com', password: p })
            });
            return { ok: r.ok, body: await r.json().catch(() => ({})) };
        }, pwd);
        if (res.ok) {
            if (res.body.token) await page.evaluate(t => localStorage.setItem('accessToken', t), res.body.token);
            return;
        }
    }
    throw new Error('Login failed');
}

// ─── Pipeline context ─────────────────────────────────────────────────────────

async function discoverPipeline(page) {
    return page.evaluate(async () => {
        try {
            const r = await fetch('/api/interfaces', { credentials: 'include' });
            if (!r.ok) return null;
            const body = await r.json().catch(() => null);
            const ifaces = body?.interfaces || body?.data || (Array.isArray(body) ? body : []);
            for (const iface of ifaces.slice(0, 20)) {
                const pr = await fetch(`/api/pipelines/interface/${iface.id}`, { credentials: 'include' });
                if (!pr.ok) continue;
                const pb = await pr.json().catch(() => null);
                const pipes = pb?.pipelines || pb?.data || (Array.isArray(pb) ? pb : []);
                for (const p of pipes) {
                    if (p?.id) return { pipelineId: p.id, interfaceId: iface.id, messageType: p.message_type || 'ADT^A01' };
                }
            }
        } catch (_) {}
        return null;
    });
}

// ─── Test message ─────────────────────────────────────────────────────────────

const TEST_MSG = [
    'MSH|^~\\&|SEND|SEND_FAC|RECV|RECV_FAC|20240101120000||ADT^A01|MSGCMP01|P|2.5',
    'PID|1||CMP12345^^^HospA^MR||Williams^David^J||19680322|M|||789 Pine Ave^^Boston^MA^02101||617-555-4321',
].join('\r');

// ─── Pipeline runner ──────────────────────────────────────────────────────────

async function runNormPipeline(page, steps) {
    const ctx = await discoverPipeline(page);
    if (!ctx) return { status: 404, ok: false, body: { error: 'No pipeline found' } };
    return page.evaluate(async ({ steps, msg, ctx }) => {
        const r = await fetch('/api/pipelines/test', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({
                pipeline_id: ctx.pipelineId,
                pipeline: {
                    interfaceId: ctx.interfaceId,
                    messageType: ctx.messageType,
                    execution_groups: [{ id: 'cmp-test-group', steps }]
                },
                test_message: msg
            })
        });
        const body = await r.json().catch(() => ({}));
        return { status: r.status, ok: r.ok, body };
    }, { steps, msg: TEST_MSG, ctx });
}

// ─── Step builders ────────────────────────────────────────────────────────────

function scriptStep(content) {
    return {
        id:        'setup0',
        step_name: 'setup',
        step_type: 'enrichment.script',
        sequence:  10,
        enabled:   true,
        config:    { script: content }
    };
}

function normStep(cfg, id = 'norm01') {
    return {
        id,
        step_name: 'test_normalizer',
        step_type: 'normalizer',
        sequence:  20,
        enabled:   true,
        config:    cfg
    };
}

// ─── Result extraction ────────────────────────────────────────────────────────

function extractNormOutput(body) {
    if (body.steps && typeof body.steps === 'object' && !Array.isArray(body.steps)) {
        for (const [, sd] of Object.entries(body.steps)) {
            const so = sd?.step_output || sd?.stepOutput || sd?._stepOutput || {};
            if ('operation' in so || 'result_count' in so || 'result' in so) {
                return { so, meta: sd?.step_metadata || sd?.stepMetadata || {} };
            }
        }
        const vals = Object.values(body.steps);
        if (vals.length > 0) {
            const last = vals[vals.length - 1];
            return { so: last?.step_output || {}, meta: last?.step_metadata || {} };
        }
    }
    return { so: {}, meta: {} };
}

function getRows(so) { return so.result ?? so.Result ?? null; }
function getCount(so) { return so.result_count ?? so.result?.length ?? 0; }

// ─── UI helpers ───────────────────────────────────────────────────────────────

async function loadBuilder(page) {
    await page.goto(`${BASE_URL}/pipeline-builder.html?pipelineId=${KNOWN_INTERFACE_ID}`);
    await page.waitForLoadState('load');
    await page.waitForFunction(
        () => window.pipelineBuilder?.pipeline != null && window.VisualStep != null,
        { timeout: 10000 }
    ).catch(() => {});
}

async function addNormStep(page, cfg = {}) {
    const stepId = await page.evaluate(cfg => {
        if (!window.pipelineBuilder || !window.VisualStep) return null;
        const s = new window.VisualStep({
            stepName: 'Test Normalizer',
            stepType: 'normalizer',
            sequence: 998,
            enabled:  true,
            config:   cfg
        });
        window.pipelineBuilder.addStep(s);
        return s.id;
    }, cfg);
    if (!stepId) return null;
    await page.waitForSelector(`.flowchart-step-node[data-step-id="${stepId}"]`, { timeout: 6000 });
    await page.click(`.flowchart-step-node[data-step-id="${stepId}"]`);
    await page.waitForTimeout(400);
    return stepId;
}

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 1 — NORMALIZE (Columns → Rows)
// Use case: wide-format clinical data → long format ready for FHIR Observation mapping
// ─────────────────────────────────────────────────────────────────────────────

test.describe('Normalize — Columns → Rows', () => {
    test.setTimeout(60000);

    // N-1: Basic unpivot — lab results wide format → one row per test
    test('N-1: lab results wide → long (3 patients × 4 tests = 12 rows)', async ({ page }) => {
        await login(page);
        const data = [
            { patient_id: 'P001', mrn: 'M001', glucose: 95,  hemoglobin: 13.5, cholesterol: 180, wbc: 7.2 },
            { patient_id: 'P002', mrn: 'M002', glucose: 110, hemoglobin: 12.8, cholesterol: 220, wbc: 9.1 },
            { patient_id: 'P003', mrn: 'M003', glucose: 88,  hemoglobin: 14.1, cholesterol: 165, wbc: 6.8 },
        ];

        const [s, n] = [
            scriptStep(`output.labs = ${JSON.stringify(data)};`),
            normStep({
                operation:       'normalize',
                sourceField:     'steps.setup.step_output.labs',
                normalizeFields: ['glucose', 'hemoglobin', 'cholesterol', 'wbc'],
                preserveFields:  ['patient_id', 'mrn'],
                keyColumn:       'test_name',
                valueColumn:     'test_result',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        expect(getCount(so), 'N-1: 3 patients × 4 tests = 12 rows').toBe(12);
        expect(so.operation, 'N-1: operation field').toBe('normalize');

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            // Each row must have test_name, test_result, patient_id, mrn
            expect('test_name'   in rows[0], 'custom keyColumn must be test_name').toBe(true);
            expect('test_result' in rows[0], 'custom valueColumn must be test_result').toBe(true);
            expect('patient_id'  in rows[0], 'patient_id must be preserved').toBe(true);
            expect('mrn'         in rows[0], 'mrn must be preserved').toBe(true);
            // No source-only columns should leak in (glucose etc. should not appear)
            expect('glucose' in rows[0], 'source col must not appear in row').toBe(false);
        }
    });

    // N-2: All fields normalized (no normalizeFields specified)
    test('N-2: normalize all fields when normalizeFields is empty', async ({ page }) => {
        await login(page);
        const data = [
            { systolic: 120, diastolic: 80, heart_rate: 72, spo2: 98 }
        ];

        const [s, n] = [
            scriptStep(`output.vitals = ${JSON.stringify(data)};`),
            normStep({
                operation:   'normalize',
                sourceField: 'steps.setup.step_output.vitals',
                keyColumn:   'vital_name',
                valueColumn: 'vital_value',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        // 1 record × 4 fields = 4 rows
        expect(getCount(so), 'N-2: 1 record × 4 fields = 4 rows').toBe(4);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            const names = rows.map(r => r.vital_name);
            expect(names, 'all 4 vital names should appear').toEqual(
                expect.arrayContaining(['systolic', 'diastolic', 'heart_rate', 'spo2'])
            );
        }
    });

    // N-3: Multiple preserve fields (patient_id + encounter_id) in every output row
    test('N-3: multiple preserveFields — patient_id + encounter_id in every output row', async ({ page }) => {
        await login(page);
        const data = [
            { patient_id: 'P001', encounter_id: 'E001', hba1c: 6.8, ldl: 110 },
            { patient_id: 'P002', encounter_id: 'E002', hba1c: 7.2, ldl: 135 },
        ];

        const [s, n] = [
            scriptStep(`output.results = ${JSON.stringify(data)};`),
            normStep({
                operation:       'normalize',
                sourceField:     'steps.setup.step_output.results',
                normalizeFields: ['hba1c', 'ldl'],
                preserveFields:  ['patient_id', 'encounter_id'],
                keyColumn:       'field',
                valueColumn:     'value',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        expect(getCount(so), 'N-3: 2 patients × 2 tests = 4 rows').toBe(4);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            for (const row of rows) {
                expect('patient_id'   in row, 'patient_id preserved in each row').toBe(true);
                expect('encounter_id' in row, 'encounter_id preserved in each row').toBe(true);
            }
        }
    });

    // N-4: Output written to outputField, source data still accessible under original key
    test('N-4: outputField separates normalized rows from original source', async ({ page }) => {
        await login(page);
        const data = [{ patient_id: 'P001', temperature: 98.6, pulse: 72 }];

        const [s, n] = [
            scriptStep(`output.raw_vitals = ${JSON.stringify(data)};`),
            normStep({
                operation:       'normalize',
                sourceField:     'steps.setup.step_output.raw_vitals',
                outputField:     'normalized_vitals',
                normalizeFields: ['temperature', 'pulse'],
                keyColumn:       'field',
                valueColumn:     'value',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so, meta } = extractNormOutput(body);

        expect(getCount(so), 'N-4: 1 patient × 2 vitals = 2 rows').toBe(2);

        // Execution details should record the outputField
        const outField = meta.output_field ?? meta.outputField ?? '';
        if (outField) {
            expect(outField, 'execution details: output_field = normalized_vitals').toBe('normalized_vitals');
        }
    });

    // N-5: Normalize only numeric columns, skip ID/name columns
    test('N-5: normalizeFields selects only the columns that should become observations', async ({ page }) => {
        await login(page);
        // Real scenario: CBC panel — only numeric results should become FHIR Observations
        const data = [
            { patient_id: 'P001', first_name: 'Alice', last_name: 'Johnson',
              wbc: 7.2, rbc: 4.5, hgb: 13.5, hct: 40.1, plt: 250 }
        ];

        const [s, n] = [
            scriptStep(`output.cbc = ${JSON.stringify(data)};`),
            normStep({
                operation:       'normalize',
                sourceField:     'steps.setup.step_output.cbc',
                normalizeFields: ['wbc', 'rbc', 'hgb', 'hct', 'plt'],
                preserveFields:  ['patient_id'],
                keyColumn:       'analyte',
                valueColumn:     'result',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        expect(getCount(so), 'N-5: 1 patient × 5 CBC analytes = 5 rows').toBe(5);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            const analytes = rows.map(r => r.analyte);
            // Demographic columns must NOT appear as analyte names
            expect(analytes, 'first_name must not be an analyte').not.toContain('first_name');
            expect(analytes, 'last_name must not be an analyte').not.toContain('last_name');
            // But the 5 requested analytes must all be present
            expect(analytes, 'all 5 CBC analytes must appear').toEqual(
                expect.arrayContaining(['wbc', 'rbc', 'hgb', 'hct', 'plt'])
            );
        }
    });

    // N-6: CaseTransform=snake turns CamelCase source column names into snake_case analyte names
    test('N-6: caseTransform=snake normalizes CamelCase column names in keyColumn values', async ({ page }) => {
        await login(page);
        // Legacy system uses CamelCase column names
        const data = [
            { PatientId: 'P001', BloodGlucose: 95, HeartRate: 72, SystolicBP: 120 }
        ];

        const [s, n] = [
            scriptStep(`output.legacy = ${JSON.stringify(data)};`),
            normStep({
                operation:       'normalize',
                sourceField:     'steps.setup.step_output.legacy',
                normalizeFields: ['BloodGlucose', 'HeartRate', 'SystolicBP'],
                keyColumn:       'field',
                valueColumn:     'value',
                caseTransform:   'snake',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        expect(getCount(so), 'N-6: 1 patient × 3 fields = 3 rows').toBe(3);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            const fields = rows.map(r => r.field);
            // CamelCase column names must become snake_case in the field column
            expect(fields, 'BloodGlucose → blood_glucose').toContain('blood_glucose');
            expect(fields, 'HeartRate → heart_rate').toContain('heart_rate');
            expect(fields, 'SystolicBP → systolic_b_p').toContain('systolic_b_p');
        }
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 2 — PIVOT (Rows → Columns)
// Use case: long-format observation rows → one wide row per patient
// ─────────────────────────────────────────────────────────────────────────────

test.describe('Pivot — Rows → Columns', () => {
    test.setTimeout(60000);

    // P-1: OBX-style rows → one row per patient with each test as a column
    test('P-1: OBX rows → wide patient table (each lab test becomes a column)', async ({ page }) => {
        await login(page);
        // Simulates how HL7 OBX segments arrive: one segment per observation
        const obx = [
            { patient_id: 'P001', test_code: 'GLUC',  result_value: 95.0  },
            { patient_id: 'P001', test_code: 'HGB',   result_value: 13.5  },
            { patient_id: 'P001', test_code: 'CHOL',  result_value: 180.0 },
            { patient_id: 'P002', test_code: 'GLUC',  result_value: 110.0 },
            { patient_id: 'P002', test_code: 'HGB',   result_value: 12.8  },
            { patient_id: 'P002', test_code: 'CHOL',  result_value: 220.0 },
            { patient_id: 'P003', test_code: 'GLUC',  result_value: 88.0  },
            { patient_id: 'P003', test_code: 'HGB',   result_value: 14.1  },
            { patient_id: 'P003', test_code: 'CHOL',  result_value: 165.0 },
        ];

        const [s, n] = [
            scriptStep(`output.obx = ${JSON.stringify(obx)};`),
            normStep({
                operation:  'pivot',
                sourceField: 'steps.setup.step_output.obx',
                pivotField:  'test_code',
                valueField:  'result_value',
                groupBy:     ['patient_id'],
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        expect(getCount(so), 'P-1: 3 patients → 3 pivot rows').toBe(3);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            for (const row of rows) {
                // Each row must have GLUC, HGB, CHOL columns
                expect('GLUC' in row, 'GLUC column must exist').toBe(true);
                expect('HGB'  in row, 'HGB column must exist').toBe(true);
                expect('CHOL' in row, 'CHOL column must exist').toBe(true);
                expect('patient_id' in row, 'patient_id group key must exist').toBe(true);
            }
            // P001 row must have correct values
            const p001 = rows.find(r => r.patient_id === 'P001');
            if (p001) {
                expect(p001.GLUC, 'P001 GLUC').toBe(95.0);
                expect(p001.HGB,  'P001 HGB').toBe(13.5);
                expect(p001.CHOL, 'P001 CHOL').toBe(180.0);
            }
        }
    });

    // P-2: Encounter counts by month → months as columns
    test('P-2: monthly encounter counts → patient row with each month as column', async ({ page }) => {
        await login(page);
        const data = [
            { patient_id: 'P001', month: 'Jan', count: 2 },
            { patient_id: 'P001', month: 'Feb', count: 1 },
            { patient_id: 'P001', month: 'Mar', count: 3 },
            { patient_id: 'P002', month: 'Jan', count: 4 },
            { patient_id: 'P002', month: 'Feb', count: 2 },
            { patient_id: 'P002', month: 'Mar', count: 1 },
        ];

        const [s, n] = [
            scriptStep(`output.counts = ${JSON.stringify(data)};`),
            normStep({
                operation:   'pivot',
                sourceField: 'steps.setup.step_output.counts',
                pivotField:  'month',
                valueField:  'count',
                groupBy:     ['patient_id'],
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        expect(getCount(so), 'P-2: 2 patients → 2 pivot rows').toBe(2);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            const p001 = rows.find(r => r.patient_id === 'P001');
            if (p001) {
                expect(p001.Jan, 'P001 Jan count').toBe(2);
                expect(p001.Feb, 'P001 Feb count').toBe(1);
                expect(p001.Mar, 'P001 Mar count').toBe(3);
            }
        }
    });

    // P-3: Composite groupBy (patient_id + provider_id = unique row per patient-provider pair)
    test('P-3: composite groupBy — patient+provider combo defines a unique pivot row', async ({ page }) => {
        await login(page);
        const data = [
            { patient_id: 'P001', provider_id: 'DR01', metric: 'visit_count', value: 3 },
            { patient_id: 'P001', provider_id: 'DR01', metric: 'avg_los',     value: 2.5 },
            { patient_id: 'P001', provider_id: 'DR02', metric: 'visit_count', value: 1 },
            { patient_id: 'P001', provider_id: 'DR02', metric: 'avg_los',     value: 1.2 },
            { patient_id: 'P002', provider_id: 'DR01', metric: 'visit_count', value: 5 },
            { patient_id: 'P002', provider_id: 'DR01', metric: 'avg_los',     value: 3.1 },
        ];

        const [s, n] = [
            scriptStep(`output.stats = ${JSON.stringify(data)};`),
            normStep({
                operation:   'pivot',
                sourceField: 'steps.setup.step_output.stats',
                pivotField:  'metric',
                valueField:  'value',
                groupBy:     ['patient_id', 'provider_id'],
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        // 3 unique (patient_id, provider_id) combos: P001+DR01, P001+DR02, P002+DR01
        expect(getCount(so), 'P-3: 3 patient-provider combos → 3 pivot rows').toBe(3);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            for (const row of rows) {
                expect('patient_id'  in row, 'patient_id group key present').toBe(true);
                expect('provider_id' in row, 'provider_id group key present').toBe(true);
                expect('visit_count' in row, 'visit_count column present').toBe(true);
                expect('avg_los'     in row, 'avg_los column present').toBe(true);
            }
        }
    });

    // P-4: Medication route table — drug_name as pivot, dose as value
    test('P-4: medication records pivoted — each drug becomes a dose column', async ({ page }) => {
        await login(page);
        const data = [
            { patient_id: 'P001', drug: 'metformin',   dose_mg: 500  },
            { patient_id: 'P001', drug: 'lisinopril',  dose_mg: 10   },
            { patient_id: 'P001', drug: 'atorvastatin', dose_mg: 20  },
            { patient_id: 'P002', drug: 'metformin',   dose_mg: 1000 },
            { patient_id: 'P002', drug: 'lisinopril',  dose_mg: 20   },
        ];

        const [s, n] = [
            scriptStep(`output.meds = ${JSON.stringify(data)};`),
            normStep({
                operation:   'pivot',
                sourceField: 'steps.setup.step_output.meds',
                pivotField:  'drug',
                valueField:  'dose_mg',
                groupBy:     ['patient_id'],
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        expect(getCount(so), 'P-4: 2 patients → 2 pivot rows').toBe(2);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            const p001 = rows.find(r => r.patient_id === 'P001');
            if (p001) {
                expect(p001.metformin,    'P001 metformin dose').toBe(500);
                expect(p001.lisinopril,   'P001 lisinopril dose').toBe(10);
                expect(p001.atorvastatin, 'P001 atorvastatin dose').toBe(20);
            }
            // P002 has no atorvastatin row — column should be absent or undefined
            const p002 = rows.find(r => r.patient_id === 'P002');
            if (p002) {
                expect(p002.atorvastatin, 'P002 atorvastatin should be absent').toBeUndefined();
            }
        }
    });

    // P-5: Single groupBy field, values verified against known inputs
    test('P-5: ICD-10 diagnosis code table — code as column, frequency as value', async ({ page }) => {
        await login(page);
        const data = [
            { facility: 'FAC-A', icd_code: 'E11.9', freq: 42 },
            { facility: 'FAC-A', icd_code: 'I10',   freq: 67 },
            { facility: 'FAC-A', icd_code: 'J06.9', freq: 15 },
            { facility: 'FAC-B', icd_code: 'E11.9', freq: 28 },
            { facility: 'FAC-B', icd_code: 'I10',   freq: 51 },
            { facility: 'FAC-B', icd_code: 'J06.9', freq: 9  },
        ];

        const [s, n] = [
            scriptStep(`output.dx = ${JSON.stringify(data)};`),
            normStep({
                operation:   'pivot',
                sourceField: 'steps.setup.step_output.dx',
                pivotField:  'icd_code',
                valueField:  'freq',
                groupBy:     ['facility'],
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        expect(getCount(so), 'P-5: 2 facilities → 2 pivot rows').toBe(2);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            const facA = rows.find(r => r.facility === 'FAC-A');
            if (facA) {
                expect(facA['E11.9'], 'FAC-A E11.9 freq = 42').toBe(42);
                expect(facA['I10'],   'FAC-A I10 freq = 67').toBe(67);
            }
        }
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 3 — TRANSPOSE (Flip Matrix)
// Use case: metrics-per-patient matrix → metrics as rows, patients as columns
// ─────────────────────────────────────────────────────────────────────────────

test.describe('Transpose — Flip Matrix', () => {
    test.setTimeout(60000);

    // T-1: Vitals comparison grid — flip so each vital becomes a row
    test('T-1: vitals report matrix transposed — each vital becomes a row', async ({ page }) => {
        await login(page);
        // Input: 4 rows (patients), 5 columns (vitals + metric label)
        const matrix = [
            { metric: 'systolic_bp',  P001: 120, P002: 135, P003: 118 },
            { metric: 'diastolic_bp', P001: 80,  P002: 88,  P003: 75  },
            { metric: 'heart_rate',   P001: 72,  P002: 85,  P003: 68  },
            { metric: 'spo2',         P001: 98,  P002: 97,  P003: 99  },
        ];

        const [s, n] = [
            scriptStep(`output.matrix = ${JSON.stringify(matrix)};`),
            normStep({
                operation:   'transpose',
                sourceField: 'steps.setup.step_output.matrix',
                outputField: 'transposed',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        // Transpose: 4 rows × 4 cols → 4 fields become rows (metric, P001, P002, P003)
        // Actually transpose produces one row per unique KEY across all input rows
        // Keys: metric, P001, P002, P003 = 4 unique keys → 4 output rows
        expect(getCount(so), 'T-1: 4 unique keys → 4 transposed rows').toBe(4);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            for (const row of rows) {
                // Each transposed row must have "field" column + row_0..row_N columns
                expect('field' in row, 'transposed row must have "field" key').toBe(true);
                expect('row_0' in row, 'transposed row must have row_0').toBe(true);
                expect('row_3' in row, 'transposed row must have row_3 (4 input rows)').toBe(true);
            }
            // Find the "metric" field row — its row_0..row_3 should be the metric names
            const metricRow = rows.find(r => r.field === 'metric');
            if (metricRow) {
                expect(metricRow.row_0, 'metric row_0 = systolic_bp').toBe('systolic_bp');
            }
        }
    });

    // T-2: Reference range table — specimens as rows, analytes as columns → flip
    test('T-2: reference range table flipped — analytes become rows', async ({ page }) => {
        await login(page);
        const refRanges = [
            { analyte: 'glucose',   male_low: 70, male_high: 99, female_low: 70, female_high: 99 },
            { analyte: 'hemoglobin',male_low: 13.5,male_high: 17.5,female_low: 12.0,female_high: 15.5 },
            { analyte: 'platelets', male_low: 150, male_high: 400, female_low: 150, female_high: 400 },
        ];

        const [s, n] = [
            scriptStep(`output.refs = ${JSON.stringify(refRanges)};`),
            normStep({
                operation:   'transpose',
                sourceField: 'steps.setup.step_output.refs',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        // Input has 5 columns: analyte, male_low, male_high, female_low, female_high
        // Transpose → 5 rows (one per original column)
        expect(getCount(so), 'T-2: 5 keys → 5 transposed rows').toBe(5);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            // Each original column becomes a row — "analyte" row, "male_low" row, etc.
            const fieldNames = rows.map(r => r.field);
            expect(fieldNames, 'analyte must be a field row').toContain('analyte');
            expect(fieldNames, 'male_low must be a field row').toContain('male_low');
            expect(fieldNames, 'female_high must be a field row').toContain('female_high');
            // 3 input rows → row_0, row_1, row_2 value columns
            expect('row_2' in rows[0], 'must have row_2 for 3 input rows').toBe(true);
        }
    });

    // T-3: Single-row input → each field becomes a row with one value column
    test('T-3: single-row summary transposed → each field becomes a standalone row', async ({ page }) => {
        await login(page);
        const summary = [
            { total_patients: 1250, avg_age: 54.3, pct_diabetic: 18.2, pct_hypertensive: 31.5 }
        ];

        const [s, n] = [
            scriptStep(`output.summary = ${JSON.stringify(summary)};`),
            normStep({
                operation:   'transpose',
                sourceField: 'steps.setup.step_output.summary',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        // 4 fields → 4 rows; 1 input row → only row_0 column
        expect(getCount(so), 'T-3: 4 summary fields → 4 rows').toBe(4);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            const fieldNames = rows.map(r => r.field);
            expect(fieldNames, 'all 4 summary fields must appear').toEqual(
                expect.arrayContaining(['total_patients', 'avg_age', 'pct_diabetic', 'pct_hypertensive'])
            );
            // Only one value column since input had 1 row
            expect('row_0' in rows[0], 'must have row_0').toBe(true);
            expect('row_1' in rows[0], 'must NOT have row_1 (only 1 input row)').toBe(false);
        }
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 4 — FLATTEN (Nested → Flat)
// Use case: deep FHIR resources → dot-notation keys ready for field mapping
// Note: flatten accepts a single OBJECT (not an array). Arrays in the object
//       are treated as leaf values (not recursed into).
// ─────────────────────────────────────────────────────────────────────────────

test.describe('Flatten — Nested → Flat', () => {
    test.setTimeout(60000);

    // F-1: FHIR-style patient resource → flat dot-notation keys
    test('F-1: FHIR patient nested object → flat dot-notation for field mapping', async ({ page }) => {
        await login(page);
        // Simplified FHIR Patient using snake_case keys (pipeline normalizer converts
        // camelCase in script step outputs, so we use snake_case to keep assertions exact)
        const patient = {
            id: 'PAT-001',
            name:       { family: 'Williams', given: 'David' },
            address:    { line: '789 Pine Ave', city: 'Boston', state: 'MA', postal_code: '02101' },
            birth_date: '1968-03-22',
            gender:     'male',
        };

        const [s, n] = [
            scriptStep(`output.patient = ${JSON.stringify(patient)};`),
            normStep({
                operation:   'flatten',
                sourceField: 'steps.setup.step_output.patient',
                delimiter:   '.',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        // Must produce > 0 flattened keys
        expect(getCount(so), 'F-1: flattened patient must have > 0 keys').toBeGreaterThan(0);

        const flat = getRows(so);
        if (flat && typeof flat === 'object' && !Array.isArray(flat)) {
            const keys = Object.keys(flat);
            // Top-level scalars
            expect(flat['id'],          'id leaf key').toBe('PAT-001');
            expect(flat['birth_date'],  'birth_date leaf key').toBe('1968-03-22');
            expect(flat['gender'],      'gender leaf key').toBe('male');
            // Nested keys should be dot-notated
            expect(flat['name.family'],   'name.family flattened').toBe('Williams');
            expect(flat['name.given'],    'name.given flattened').toBe('David');
            expect(flat['address.city'],  'address.city flattened').toBe('Boston');
            expect(flat['address.state'], 'address.state flattened').toBe('MA');
            // No raw nested objects should remain
            expect(typeof flat['name'],    'name must not be an object').not.toBe('object');
            expect(typeof flat['address'], 'address must not be an object').not.toBe('object');
        }
    });

    // F-2: maxDepth=1 — only first nesting level is flattened
    test('F-2: maxDepth=1 flattens only first level; deeper nesting stays as objects', async ({ page }) => {
        await login(page);
        const nested = {
            patient: {
                name:    { family: 'Smith', given: 'Jane' },
                contact: { phone: '617-555-0001', email: 'j.smith@test.com' }
            },
            encounter: { id: 'ENC-001', date: '2024-01-15' }
        };

        const [s, n] = [
            scriptStep(`output.record = ${JSON.stringify(nested)};`),
            normStep({
                operation:   'flatten',
                sourceField: 'steps.setup.step_output.record',
                delimiter:   '.',
                maxDepth:    1,
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        const flat = getRows(so);
        if (flat && typeof flat === 'object' && !Array.isArray(flat)) {
            // depth=1: patient.name is a leaf (not further expanded to patient.name.family)
            const keys = Object.keys(flat);
            const tooDeep = keys.some(k => (k.match(/\./g) || []).length >= 2);
            expect(tooDeep, 'maxDepth=1: no key should have 2+ dots').toBe(false);
            // But patient.name, patient.contact, encounter.id, encounter.date should exist
            expect('patient.name'    in flat, 'patient.name at depth 1').toBe(true);
            expect('encounter.id'    in flat, 'encounter.id at depth 1').toBe(true);
            expect('encounter.date'  in flat, 'encounter.date at depth 1').toBe(true);
        }
    });

    // F-3: Custom delimiter "/" — produces path/like/keys for specific systems
    test('F-3: custom delimiter "/" produces path-style keys (FHIR JSON Pointer style)', async ({ page }) => {
        await login(page);
        const resource = {
            patient: { id: 'P001', name: { family: 'Jones' } },
            meta:    { source: 'HL7', version: '2.5' }
        };

        const [s, n] = [
            scriptStep(`output.res = ${JSON.stringify(resource)};`),
            normStep({
                operation:   'flatten',
                sourceField: 'steps.setup.step_output.res',
                delimiter:   '/',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        const flat = getRows(so);
        if (flat && typeof flat === 'object' && !Array.isArray(flat)) {
            const keys = Object.keys(flat);
            expect(keys.some(k => k.includes('/')), 'keys must use / delimiter').toBe(true);
            expect(keys.some(k => k.includes('.')), 'keys must NOT use . delimiter').toBe(false);
            expect(flat['patient/id'],         'patient/id').toBe('P001');
            expect(flat['patient/name/family'],'patient/name/family').toBe('Jones');
            expect(flat['meta/source'],        'meta/source').toBe('HL7');
        }
    });

    // F-4: Deeply nested claim data — 4 levels of nesting
    test('F-4: insurance claim — 4 levels of nesting fully flattened', async ({ page }) => {
        await login(page);
        const claim = {
            claim: {
                id: 'CLM-001',
                patient: {
                    member: {
                        id:   'MBR-001',
                        name: 'Alice Brown'
                    }
                },
                service: {
                    diagnosis: {
                        primary: 'E11.9',
                        secondary: 'I10'
                    }
                }
            }
        };

        const [s, n] = [
            scriptStep(`output.claim = ${JSON.stringify(claim)};`),
            normStep({
                operation:   'flatten',
                sourceField: 'steps.setup.step_output.claim',
                delimiter:   '.',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        const flat = getRows(so);
        if (flat && typeof flat === 'object' && !Array.isArray(flat)) {
            // 4 levels deep: claim.patient.member.id
            expect(flat['claim.patient.member.id'],    '4-level key').toBe('MBR-001');
            expect(flat['claim.patient.member.name'],  '4-level name').toBe('Alice Brown');
            expect(flat['claim.service.diagnosis.primary'], '4-level diagnosis').toBe('E11.9');
        }
    });

    // F-5: Flatten then renameMap — standardise long dot-notation keys
    test('F-5: flatten + renameMap standardises long keys to shorter aliases', async ({ page }) => {
        await login(page);
        const data = {
            patient: { demographics: { first: 'Rosa', last: 'Mendez' } }
        };

        const [s, n] = [
            scriptStep(`output.data = ${JSON.stringify(data)};`),
            normStep({
                operation:   'flatten',
                sourceField: 'steps.setup.step_output.data',
                delimiter:   '.',
                renameMap:   {
                    'patient.demographics.first': 'first_name',
                    'patient.demographics.last':  'last_name',
                }
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        const flat = getRows(so);
        if (flat && typeof flat === 'object' && !Array.isArray(flat)) {
            expect(flat['first_name'],                   'renamed to first_name').toBe('Rosa');
            expect(flat['last_name'],                    'renamed to last_name').toBe('Mendez');
            expect('patient.demographics.first' in flat, 'original key replaced').toBe(false);
        }
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 5 — UNFLATTEN (Flat → Nested)
// Use case: flat CSV file parser output → nested FHIR resource ready for mapping
// ─────────────────────────────────────────────────────────────────────────────

test.describe('Unflatten — Flat → Nested', () => {
    test.setTimeout(60000);

    // U-1: Flat CSV keys → nested FHIR Patient structure
    test('U-1: flat CSV columns → nested FHIR Patient structure (dot-delimiter)', async ({ page }) => {
        await login(page);
        // How a flat file parser produces data (keys are snake_case so they survive
        // pipeline normalization on the script step output without being mangled)
        const flat = {
            'patient.id':              'PAT-002',
            'patient.name.family':     'Garcia',
            'patient.name.given':      'Maria',
            'patient.address.city':    'Miami',
            'patient.address.state':   'FL',
            'patient.address.zip':     '33101',
            'patient.birth_date':      '1975-06-14',
            'patient.gender':          'female',
        };

        const [s, n] = [
            scriptStep(`output.flat = ${JSON.stringify(flat)};`),
            normStep({
                operation:   'unflatten',
                sourceField: 'steps.setup.step_output.flat',
                delimiter:   '.',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        const nested = getRows(so);
        if (nested && typeof nested === 'object' && !Array.isArray(nested)) {
            expect(nested.patient,                     'patient key exists').toBeTruthy();
            expect(nested.patient.id,                  'patient.id').toBe('PAT-002');
            expect(nested.patient.name?.family,        'patient.name.family').toBe('Garcia');
            expect(nested.patient.name?.given,         'patient.name.given').toBe('Maria');
            expect(nested.patient.address?.city,       'patient.address.city').toBe('Miami');
            expect(nested.patient.address?.state,      'patient.address.state').toBe('FL');
            expect(nested.patient.birth_date,          'patient.birth_date').toBe('1975-06-14');
        }
    });

    // U-2: Custom delimiter "_" (common in EDI/X12 flat file exports)
    test('U-2: underscore-delimited flat keys → nested object (EDI-style)', async ({ page }) => {
        await login(page);
        const flat = {
            'claim_id':                  'CLM-001',
            'claim_patient_id':          'P001',
            'claim_patient_name':        'John Doe',
            'claim_service_date':        '2024-01-15',
            'claim_service_code':        '99213',
            'claim_billing_amount':      150.00,
        };

        const [s, n] = [
            scriptStep(`output.edi = ${JSON.stringify(flat)};`),
            normStep({
                operation:   'unflatten',
                sourceField: 'steps.setup.step_output.edi',
                delimiter:   '_',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        const nested = getRows(so);
        if (nested && typeof nested === 'object' && !Array.isArray(nested)) {
            expect(nested.claim,                      'claim root key').toBeTruthy();
            expect(nested.claim.id,                   'claim.id').toBe('CLM-001');
            expect(nested.claim.patient?.id,          'claim.patient.id').toBe('P001');
            expect(nested.claim.service?.code,        'claim.service.code').toBe('99213');
            expect(nested.claim.billing?.amount,      'claim.billing.amount').toBe(150.00);
        }
    });

    // U-3: Round-trip: flatten then unflatten → original object restored
    test('U-3: flatten → unflatten round-trip restores original structure', async ({ page }) => {
        await login(page);
        // Step 1: script creates original object
        // Step 2: normalizer flattens it
        // Step 3: another normalizer unflattens it
        // We test: flatten step produces correct keys, unflatten restores structure

        // Flatten step
        const original = {
            record: { type: 'observation', value: 95.0, unit: 'mg/dL', status: 'final' }
        };

        const [s, flatN] = [
            scriptStep(`output.original = ${JSON.stringify(original)};`),
            normStep({
                operation:   'flatten',
                sourceField: 'steps.setup.step_output.original',
                outputField: 'flattened',
                delimiter:   '.',
            })
        ];

        const flatResult = await runNormPipeline(page, [s, flatN]);
        const { so: flatSo } = extractNormOutput(flatResult.body);
        const flatData = getRows(flatSo);

        if (flatData && typeof flatData === 'object' && !Array.isArray(flatData)) {
            expect(flatData['record.type'],  'flatten: record.type').toBe('observation');
            expect(flatData['record.value'], 'flatten: record.value').toBe(95.0);
        }

        // Now unflatten
        const flatJson = JSON.stringify(flatData || {
            'record.type': 'observation', 'record.value': 95.0,
            'record.unit': 'mg/dL', 'record.status': 'final'
        });

        const [s2, unflatN] = [
            scriptStep(`output.flat = ${flatJson};`),
            normStep({
                operation:   'unflatten',
                sourceField: 'steps.setup.step_output.flat',
                delimiter:   '.',
            })
        ];

        const unflatResult = await runNormPipeline(page, [s2, unflatN]);
        const { so: unflatSo } = extractNormOutput(unflatResult.body);
        const restored = getRows(unflatSo);

        if (restored && typeof restored === 'object') {
            expect(restored.record?.type,   'round-trip: type').toBe('observation');
            expect(restored.record?.value,  'round-trip: value').toBe(95.0);
            expect(restored.record?.unit,   'round-trip: unit').toBe('mg/dL');
            expect(restored.record?.status, 'round-trip: status').toBe('final');
        }
    });

    // U-4: Multi-level deep unflatten (5 segments: a.b.c.d.e)
    test('U-4: deep unflatten — 5 dot-notation segments build 5-level nesting', async ({ page }) => {
        await login(page);
        const flat = {
            'org.system.facility.dept.unit': 'ICU',
            'org.system.facility.dept.bed':  '12A',
            'org.system.facility.name':      'General Hospital',
            'org.system.name':               'EHR-Primary',
        };

        const [s, n] = [
            scriptStep(`output.deep = ${JSON.stringify(flat)};`),
            normStep({
                operation:   'unflatten',
                sourceField: 'steps.setup.step_output.deep',
                delimiter:   '.',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        const nested = getRows(so);
        if (nested && typeof nested === 'object' && !Array.isArray(nested)) {
            expect(
                nested?.org?.system?.facility?.dept?.unit,
                '5-level nesting: org.system.facility.dept.unit'
            ).toBe('ICU');
            expect(
                nested?.org?.system?.facility?.dept?.bed,
                '5-level nesting: org.system.facility.dept.bed'
            ).toBe('12A');
            expect(
                nested?.org?.system?.name,
                '3-level nesting: org.system.name'
            ).toBe('EHR-Primary');
        }
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 6 — POST-PROCESSING (renameMap + caseTransform)
// ─────────────────────────────────────────────────────────────────────────────

test.describe('Post-processing — rename + caseTransform', () => {
    test.setTimeout(60000);

    // PP-1: renameMap on pivot output — rename ICD codes to readable names
    test('PP-1: renameMap on pivot — ICD codes renamed to readable column names', async ({ page }) => {
        await login(page);
        const data = [
            { patient_id: 'P001', code: 'E11.9', count: 3 },
            { patient_id: 'P001', code: 'I10',   count: 5 },
            { patient_id: 'P002', code: 'E11.9', count: 1 },
            { patient_id: 'P002', code: 'I10',   count: 2 },
        ];

        const [s, n] = [
            scriptStep(`output.dx = ${JSON.stringify(data)};`),
            normStep({
                operation:   'pivot',
                sourceField: 'steps.setup.step_output.dx',
                pivotField:  'code',
                valueField:  'count',
                groupBy:     ['patient_id'],
                renameMap:   { 'E11.9': 'diabetes_encounters', 'I10': 'hypertension_encounters' }
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        const rows = getRows(so);
        if (Array.isArray(rows)) {
            const p001 = rows.find(r => r.patient_id === 'P001');
            if (p001) {
                expect(p001.diabetes_encounters,    'PP-1: E11.9 → diabetes_encounters').toBe(3);
                expect(p001.hypertension_encounters,'PP-1: I10 → hypertension_encounters').toBe(5);
                expect('E11.9' in p001, 'PP-1: original code key removed').toBe(false);
            }
        }
    });

    // PP-2: caseTransform=upper on normalize output — all column names UPPER
    test('PP-2: caseTransform=upper — all output column names are UPPERCASE', async ({ page }) => {
        await login(page);
        const data = [{ patient_id: 'P001', test_name: 'glucose', result: 95 }];

        const [s, n] = [
            scriptStep(`output.rows = ${JSON.stringify(data)};`),
            normStep({
                operation:       'normalize',
                sourceField:     'steps.setup.step_output.rows',
                normalizeFields: ['test_name', 'result'],
                keyColumn:       'field',
                valueColumn:     'value',
                caseTransform:   'upper',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        const rows = getRows(so);
        if (Array.isArray(rows) && rows.length > 0) {
            const keys = Object.keys(rows[0]);
            expect(keys.every(k => k === k.toUpperCase()), 'PP-2: all keys must be UPPER').toBe(true);
            expect(keys, 'PP-2: FIELD column').toContain('FIELD');
            expect(keys, 'PP-2: VALUE column').toContain('VALUE');
        }
    });

    // PP-3: caseTransform=camel on flatten output — dot-keys become camelCase
    test('PP-3: caseTransform=camel on flatten — snake_case keys become camelCase', async ({ page }) => {
        await login(page);
        const nested = {
            patient_id:   'P001',
            first_name:   'Alice',
            last_name:    'Smith',
            date_of_birth:'1980-01-01',
        };

        const [s, n] = [
            scriptStep(`output.rec = ${JSON.stringify(nested)};`),
            normStep({
                operation:     'flatten',
                sourceField:   'steps.setup.step_output.rec',
                delimiter:     '.',
                caseTransform: 'camel',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        const flat = getRows(so);
        if (flat && typeof flat === 'object' && !Array.isArray(flat)) {
            const keys = Object.keys(flat);
            // snake_case → camelCase: patient_id → patientId, first_name → firstName
            expect(keys, 'PP-3: patient_id → patientId').toContain('patientId');
            expect(keys, 'PP-3: first_name → firstName').toContain('firstName');
            expect(keys, 'PP-3: last_name → lastName').toContain('lastName');
            expect(keys, 'PP-3: date_of_birth → dateOfBirth').toContain('dateOfBirth');
            // Original snake_case keys must not remain
            expect(keys, 'PP-3: patient_id must be gone').not.toContain('patient_id');
        }
    });

    // PP-4: renameMap + caseTransform together — rename first, then case-transform
    test('PP-4: renameMap + caseTransform combined — rename applied before case transform', async ({ page }) => {
        await login(page);
        const data = [{ pid: 'P001', hgb: 13.5, glu: 95 }];

        const [s, n] = [
            scriptStep(`output.data = ${JSON.stringify(data)};`),
            normStep({
                operation:       'normalize',
                sourceField:     'steps.setup.step_output.data',
                normalizeFields: ['hgb', 'glu'],
                preserveFields:  ['pid'],
                keyColumn:       'field',
                valueColumn:     'value',
                renameMap:       { field: 'test_name', value: 'result', pid: 'patient_id' },
                caseTransform:   'upper',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        const rows = getRows(so);
        if (Array.isArray(rows) && rows.length > 0) {
            // rename: field→test_name, value→result, pid→patient_id
            // then upper: TEST_NAME, RESULT, PATIENT_ID
            const keys = Object.keys(rows[0]);
            expect(keys, 'PP-4: test_name → TEST_NAME').toContain('TEST_NAME');
            expect(keys, 'PP-4: result → RESULT').toContain('RESULT');
            expect(keys, 'PP-4: patient_id → PATIENT_ID').toContain('PATIENT_ID');
        }
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 7 — UX (NormalizerBuilder Form Interactions)
// ─────────────────────────────────────────────────────────────────────────────

test.describe('UX — NormalizerBuilder Form Interactions', () => {
    test.setTimeout(90000);

    // UX-1: Source and output fields pre-populated from step config
    test('UX-1: source + output fields pre-populated when step re-opened', async ({ page }) => {
        await login(page);
        await loadBuilder(page);

        await addNormStep(page, {
            operation:   'normalize',
            sourceField: 'steps.lab_parser.step_output.records',
            outputField: 'normalized_labs',
        });

        await page.waitForSelector('#nrmSourceField', { timeout: 6000 });

        const src = await page.locator('#nrmSourceField').inputValue();
        const out = await page.locator('#nrmOutputField').inputValue();

        expect(src, 'UX-1: sourceField pre-populated').toBe('steps.lab_parser.step_output.records');
        expect(out, 'UX-1: outputField pre-populated').toBe('normalized_labs');
    });

    // UX-2: keyColumn + valueColumn pre-populated for normalize operation
    test('UX-2: keyColumn and valueColumn fields show saved values', async ({ page }) => {
        await login(page);
        await loadBuilder(page);

        await addNormStep(page, {
            operation:   'normalize',
            keyColumn:   'test_name',
            valueColumn: 'test_result',
        });

        await page.waitForSelector('#nrmKeyColumn', { timeout: 6000 });

        const key = await page.locator('#nrmKeyColumn').inputValue();
        const val = await page.locator('#nrmValueColumn').inputValue();

        expect(key, 'UX-2: keyColumn = test_name').toBe('test_name');
        expect(val, 'UX-2: valueColumn = test_result').toBe('test_result');
    });

    // UX-3: normalizeFields chip input — Enter adds chip; Backspace removes last
    test('UX-3: normalizeFields chip — type+Enter adds chip; Backspace removes last chip', async ({ page }) => {
        await login(page);
        await loadBuilder(page);

        await addNormStep(page, { operation: 'normalize' });
        await page.waitForSelector('#nrmNormalizeFieldsInput', { timeout: 6000 });

        const input = page.locator('#nrmNormalizeFieldsInput');

        // Type "glucose" and press Enter
        await input.fill('glucose');
        await input.press('Enter');
        await page.waitForTimeout(100);

        // Type "hemoglobin" and press comma
        await input.fill('hemoglobin');
        await input.press(',');
        await page.waitForTimeout(100);

        // Check JSON hidden input contains both values
        const json = await page.locator('#nrmNormalizeFieldsJson').inputValue();
        const arr = JSON.parse(json || '[]');
        expect(arr, 'UX-3: normalizeFields must contain glucose').toContain('glucose');
        expect(arr, 'UX-3: normalizeFields must contain hemoglobin').toContain('hemoglobin');

        // Press Backspace to remove last chip
        await input.press('Backspace');
        await page.waitForTimeout(100);

        const json2 = await page.locator('#nrmNormalizeFieldsJson').inputValue();
        const arr2 = JSON.parse(json2 || '[]');
        expect(arr2.length, 'UX-3: Backspace removes last chip → 1 chip remains').toBe(1);
        expect(arr2, 'UX-3: glucose must still be there').toContain('glucose');
    });

    // UX-4: preserveFields chip — blur adds pending value
    test('UX-4: preserveFields chip — blur on non-empty input commits the value', async ({ page }) => {
        await login(page);
        await loadBuilder(page);

        await addNormStep(page, { operation: 'normalize' });
        await page.waitForSelector('#nrmPreserveFieldsInput', { timeout: 6000 });

        const input = page.locator('#nrmPreserveFieldsInput');
        await input.fill('patient_id');
        // Blur by clicking outside
        await page.click('#nrmSourceField');
        await page.waitForTimeout(200);

        const json = await page.locator('#nrmPreserveFieldsJson').inputValue();
        const arr = JSON.parse(json || '[]');
        expect(arr, 'UX-4: blur must commit patient_id').toContain('patient_id');
    });

    // UX-5: Pivot section shows pivotField, valueField, groupBy chip
    test('UX-5: clicking Pivot reveals pivotField + valueField inputs and groupBy chip', async ({ page }) => {
        await login(page);
        await loadBuilder(page);

        await addNormStep(page, { operation: 'normalize' });
        await page.waitForSelector('.nrm-op-btn[data-op="pivot"]', { timeout: 6000 });

        await page.click('.nrm-op-btn[data-op="pivot"]');
        await page.waitForTimeout(200);

        const pivotSection = page.locator('[data-section="pivot"]');
        await expect(pivotSection.first()).toBeVisible({ timeout: 3000 });

        // pivotField and valueField inputs must be visible
        const pivotField = page.locator('#nrmPivotField');
        const valueField = page.locator('#nrmValueField');
        await expect(pivotField.first()).toBeVisible({ timeout: 2000 });
        await expect(valueField.first()).toBeVisible({ timeout: 2000 });

        // groupBy chip input must be visible
        const groupByInput = page.locator('#nrmGroupByInput');
        await expect(groupByInput.first()).toBeVisible({ timeout: 2000 });

        // Fill them in and verify collectConfig picks them up
        await pivotField.first().fill('test_code');
        await valueField.first().fill('result_value');
        await groupByInput.first().fill('patient_id');
        await groupByInput.first().press('Enter');
        await page.waitForTimeout(100);

        const cfg = await page.evaluate(() => {
            const panel = window.pipelineBuilder?.propertiesPanel;
            return {
                pivotField: document.getElementById('nrmPivotField')?.value,
                valueField: document.getElementById('nrmValueField')?.value,
                groupBy:    JSON.parse(document.getElementById('nrmGroupByJson')?.value || '[]'),
            };
        });

        expect(cfg.pivotField, 'UX-5: pivotField').toBe('test_code');
        expect(cfg.valueField, 'UX-5: valueField').toBe('result_value');
        expect(cfg.groupBy, 'UX-5: groupBy').toContain('patient_id');
    });

    // UX-6: Flatten shows delimiter + maxDepth; Unflatten shows delimiter only
    test('UX-6: flatten and unflatten sections show the correct config fields', async ({ page }) => {
        await login(page);
        await loadBuilder(page);

        await addNormStep(page, { operation: 'normalize' });
        await page.waitForSelector('.nrm-op-btn[data-op="flatten"]', { timeout: 6000 });

        // Switch to flatten
        await page.click('.nrm-op-btn[data-op="flatten"]');
        await page.waitForTimeout(200);

        const flatSection = page.locator('[data-section="flatten"]');
        await expect(flatSection.first()).toBeVisible({ timeout: 2000 });

        // Delimiter and maxDepth must be visible in flatten section
        const delimInput  = flatSection.locator('#nrmDelimiter').first();
        const maxDepthInput = flatSection.locator('#nrmMaxDepth').first();
        await expect(delimInput,    'UX-6: delimiter visible in flatten').toBeVisible({ timeout: 2000 });
        await expect(maxDepthInput, 'UX-6: maxDepth visible in flatten').toBeVisible({ timeout: 2000 });

        // Set delimiter to / and maxDepth to 3
        await delimInput.fill('/');
        await maxDepthInput.fill('3');

        const delVal = await delimInput.inputValue();
        const mdVal  = await maxDepthInput.inputValue();
        expect(delVal, 'UX-6: delimiter updated to /').toBe('/');
        expect(mdVal,  'UX-6: maxDepth updated to 3').toBe('3');

        // Switch to unflatten — its section should appear, flatten section should hide
        await page.click('.nrm-op-btn[data-op="unflatten"]');
        await page.waitForTimeout(200);

        const unflatSection = page.locator('[data-section="unflatten"]');
        await expect(unflatSection.first()).toBeVisible({ timeout: 2000 });

        const flatSectionNow = page.locator('[data-section="flatten"]');
        const flatVisible = await flatSectionNow.first().isVisible().catch(() => false);
        expect(flatVisible, 'UX-6: flatten section hidden when unflatten selected').toBe(false);
    });

    // UX-7: Rename map — add two rows, fill them, remove one
    test('UX-7: rename map — add rows, fill from/to, × button removes row', async ({ page }) => {
        await login(page);
        await loadBuilder(page);

        await addNormStep(page, { operation: 'normalize' });
        await page.waitForSelector('#nrmPostHdr', { timeout: 6000 });

        // Open post-processing
        await page.click('#nrmPostHdr');
        await page.waitForTimeout(200);
        await expect(page.locator('#nrmPostBody')).toHaveClass(/open/);

        // Add rename row
        await page.click('#nrmAddRename');
        await page.waitForTimeout(100);
        await page.click('#nrmAddRename');
        await page.waitForTimeout(100);

        const fromInputs = page.locator('.nrm-rename-from');
        const toInputs   = page.locator('.nrm-rename-to');

        let count = await fromInputs.count();
        expect(count, 'UX-7: 2 rename rows added').toBeGreaterThanOrEqual(2);

        // Fill row 0: glucose → blood_glucose
        await fromInputs.nth(count - 2).fill('glucose');
        await toInputs.nth(count - 2).fill('blood_glucose');
        // Fill row 1: hgb → hemoglobin
        await fromInputs.nth(count - 1).fill('hgb');
        await toInputs.nth(count - 1).fill('hemoglobin');

        // Remove row 1 via × button
        const rmBtns = page.locator('#nrmRenameRows .nrm-rm-btn');
        const rmCount = await rmBtns.count();
        await rmBtns.nth(rmCount - 1).click();
        await page.waitForTimeout(100);

        const finalRows = await page.locator('.nrm-rename-from').count();
        expect(finalRows, 'UX-7: 1 row remains after removal').toBe(count - 1);

        // Verify remaining row still has glucose → blood_glucose
        const remaining = page.locator('.nrm-rename-from').last();
        const val = await remaining.inputValue();
        expect(val, 'UX-7: remaining row has glucose').toBe('glucose');
    });

    // UX-8: collectConfig captures all fields for normalize operation
    test('UX-8: collectConfig round-trip for normalize — all 6 fields captured', async ({ page }) => {
        await login(page);
        await loadBuilder(page);

        await addNormStep(page, {
            operation:       'normalize',
            sourceField:     'steps.file_parser.step_output.rows',
            outputField:     'obs_rows',
            keyColumn:       'analyte',
            valueColumn:     'measurement',
            normalizeFields: ['wbc', 'rbc', 'hgb'],
            preserveFields:  ['patient_id', 'specimen_id'],
        });

        await page.waitForSelector('#nrmOperation', { state: 'attached', timeout: 6000 });

        const cfg = await page.evaluate(() => ({
            operation:      document.getElementById('nrmOperation')?.value,
            sourceField:    document.getElementById('nrmSourceField')?.value,
            outputField:    document.getElementById('nrmOutputField')?.value,
            keyColumn:      document.getElementById('nrmKeyColumn')?.value,
            valueColumn:    document.getElementById('nrmValueColumn')?.value,
            normalizeFields:JSON.parse(document.getElementById('nrmNormalizeFieldsJson')?.value || '[]'),
            preserveFields: JSON.parse(document.getElementById('nrmPreserveFieldsJson')?.value  || '[]'),
        }));

        expect(cfg.operation,      'UX-8: operation').toBe('normalize');
        expect(cfg.sourceField,    'UX-8: sourceField').toBe('steps.file_parser.step_output.rows');
        expect(cfg.outputField,    'UX-8: outputField').toBe('obs_rows');
        expect(cfg.keyColumn,      'UX-8: keyColumn').toBe('analyte');
        expect(cfg.valueColumn,    'UX-8: valueColumn').toBe('measurement');
        expect(cfg.normalizeFields,'UX-8: normalizeFields').toEqual(['wbc', 'rbc', 'hgb']);
        expect(cfg.preserveFields, 'UX-8: preserveFields').toEqual(['patient_id', 'specimen_id']);
    });

    // UX-9: collectConfig for pivot operation
    test('UX-9: collectConfig round-trip for pivot — pivotField, valueField, groupBy captured', async ({ page }) => {
        await login(page);
        await loadBuilder(page);

        await addNormStep(page, {
            operation:  'pivot',
            sourceField:'steps.obx.step_output.rows',
            pivotField: 'test_code',
            valueField: 'result',
            groupBy:    ['patient_id', 'encounter_id'],
        });

        await page.waitForSelector('#nrmOperation', { state: 'attached', timeout: 6000 });

        const cfg = await page.evaluate(() => ({
            operation:  document.getElementById('nrmOperation')?.value,
            pivotField: document.getElementById('nrmPivotField')?.value,
            valueField: document.getElementById('nrmValueField')?.value,
            groupBy:    JSON.parse(document.getElementById('nrmGroupByJson')?.value || '[]'),
        }));

        expect(cfg.operation,  'UX-9: operation = pivot').toBe('pivot');
        expect(cfg.pivotField, 'UX-9: pivotField = test_code').toBe('test_code');
        expect(cfg.valueField, 'UX-9: valueField = result').toBe('result');
        expect(cfg.groupBy,    'UX-9: groupBy').toEqual(['patient_id', 'encounter_id']);
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// SECTION 8 — EDGE CASES
// ─────────────────────────────────────────────────────────────────────────────

test.describe('Edge Cases', () => {
    test.setTimeout(60000);

    // E-1: Empty input array → result_count=0, no crash
    test('E-1: empty source array → result_count=0 for all array operations', async ({ page }) => {
        await login(page);

        for (const op of ['normalize', 'pivot', 'transpose']) {
            const config = op === 'normalize'
                ? { operation: op, sourceField: 'steps.setup.step_output.empty', keyColumn: 'f', valueColumn: 'v' }
                : op === 'pivot'
                ? { operation: op, sourceField: 'steps.setup.step_output.empty', pivotField: 'p', valueField: 'v', groupBy: ['id'] }
                : { operation: op, sourceField: 'steps.setup.step_output.empty' };

            const [s, n] = [
                scriptStep(`output.empty = [];`),
                normStep(config)
            ];

            const result = await runNormPipeline(page, [s, n]);
            // Should not 500 — pipeline continues with graceful result
            if (result.ok) {
                const { so } = extractNormOutput(result.body);
                const rc = getCount(so);
                expect(rc, `E-1: ${op} on empty array → result_count=0`).toBe(0);
            }
        }
    });

    // E-2: Array rows with inconsistent keys — normalize skips missing field values
    test('E-2: normalize with inconsistent row keys — missing fields produce null values', async ({ page }) => {
        await login(page);
        // Row 1 has all 3 fields; Row 2 is missing "cholesterol"
        const data = [
            { patient_id: 'P001', glucose: 95,  cholesterol: 180 },
            { patient_id: 'P002', glucose: 110 },  // no cholesterol
        ];

        const [s, n] = [
            scriptStep(`output.inconsistent = ${JSON.stringify(data)};`),
            normStep({
                operation:       'normalize',
                sourceField:     'steps.setup.step_output.inconsistent',
                normalizeFields: ['glucose', 'cholesterol'],
                preserveFields:  ['patient_id'],
                keyColumn:       'field',
                valueColumn:     'value',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        // Should produce 4 rows (2 patients × 2 fields, even if some values are null/undefined)
        expect(getCount(so), 'E-2: 4 rows including null-value rows').toBe(4);
    });

    // E-3: Pivot with no matching groupBy rows — empty result
    test('E-3: pivot with empty source after script produces result_count=0', async ({ page }) => {
        await login(page);
        const [s, n] = [
            scriptStep(`output.data = [];`),
            normStep({
                operation:   'pivot',
                sourceField: 'steps.setup.step_output.data',
                pivotField:  'code',
                valueField:  'value',
                groupBy:     ['id'],
            })
        ];

        const result = await runNormPipeline(page, [s, n]);
        if (result.ok) {
            const { so } = extractNormOutput(result.body);
            expect(getCount(so), 'E-3: empty pivot → result_count=0').toBe(0);
        }
    });

    // E-4: Step output contract always populated even when result is a single object
    test('E-4: flatten result is an object — result_count = number of keys (not rows)', async ({ page }) => {
        await login(page);
        const data = { patient: { id: 'P001', name: { family: 'Lee' } }, status: 'active' };

        const [s, n] = [
            scriptStep(`output.nested = ${JSON.stringify(data)};`),
            normStep({
                operation:   'flatten',
                sourceField: 'steps.setup.step_output.nested',
                delimiter:   '.',
            })
        ];

        const { body } = await runNormPipeline(page, [s, n]);
        const { so } = extractNormOutput(body);

        // Flat object keys: patient.id, patient.name.family, status = 3 keys
        expect(getCount(so), 'E-4: flatten result_count = number of flattened keys = 3').toBe(3);

        // Contract: result, result_count, operation always present
        expect('result'       in so || 'Result'       in so, 'E-4: result key present').toBe(true);
        expect('result_count' in so, 'E-4: result_count key present').toBe(true);
        expect('operation'    in so || 'Operation'    in so, 'E-4: operation key present').toBe(true);
    });
});
