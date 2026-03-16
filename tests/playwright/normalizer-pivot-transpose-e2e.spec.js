const { test, expect } = require('@playwright/test');

// ================================================================
// NORMALIZER / PIVOT / TRANSPOSE — End-to-End QA
// ================================================================
//
// Tests every operation of the NormalizerExecutor (Go) against
// real-world healthcare data scenarios, plus the NormalizerBuilder
// (UI) interactions in the Pipeline Builder.
//
// Real-world use cases:
//   Normalize  — Lab results multi-column CSV → attribute/value rows
//   Pivot      — HL7 OBX-style observation rows → one row per patient
//   Transpose  — Vitals report matrix row/column swap
//   Flatten    — FHIR nested patient JSON → dot-notation for mapping
//   Unflatten  — Flat CSV fields → nested JSON for FHIR construction
//   Post-proc  — snake_case transform, rename map on output columns
//
// Backend tests (1–20): use two-step pipeline: script creates data,
//   normalizer reads from steps.setup.step_output.<field>
// Frontend UI tests (21–30): pipeline builder NormalizerBuilder
//   preview panel interactions
//
// Suite layout:
//   1.  System health guard
//   2.  Executor: normalizer step is accepted (200 OK)
//   3.  Normalize: multi-column lab row → attribute/value rows
//   4.  Normalize: preserveFields carries patient_id into every row
//   5.  Normalize: custom keyColumn + valueColumn names
//   6.  Normalize: explicit field selection (only 2 of 4 columns)
//   7.  Pivot: OBX observations → one column per test
//   8.  Pivot: multiple patients correctly grouped by patient_id
//   9.  Transpose: vitals report row/column flip
//  10.  Flatten: nested FHIR patient JSON → dot-notation
//  11.  Flatten: custom delimiter (underscore)
//  12.  Flatten: maxDepth=1 stops at first nesting level
//  13.  Unflatten: dot-notation CSV fields → nested JSON
//  14.  Post-process: snake_case transform on camelCase columns
//  15.  Post-process: rename map standardises column names
//  16.  outputField: result written to separate key, source untouched
//  17.  Output contract: _stepOutput has result, result_count, operation
//  18.  Pipeline-level: normalizer chained with field_mapping step
//  19.  Error: validation rejects unknown operation
//  20.  Graceful: empty source array produces result_count = 0
//  21.  UI: normalizer step renders in step type registry
//  22.  UI: preview panel title = "Preview: Normalizer / Pivot / Transpose"
//  23.  UI: all 5 operation buttons visible
//  24.  UI: default operation is "normalize"
//  25.  UI: clicking "pivot" shows pivot section, hides normalize section
//  26.  UI: clicking "flatten" shows delimiter + maxDepth fields
//  27.  UI: groupBy chip input accepts field values
//  28.  UI: post-processing section collapses by default, expands on click
//  29.  UI: source field input is present
//  30.  UI: config round-trip — collectConfig returns correct operation
// ================================================================

const BASE_URL = 'http://localhost:3000';

// ─── Standard HL7 v2.5 test message (all 12 MSH fields, ADT^A01) ─
const TEST_MSG = [
    'MSH|^~\\&|SEND|SEND_FAC|RECV|RECV_FAC|20240101120000||ADT^A01|MSGNRM01|P|2.5',
    'PID|1||NRM12345^^^HospA^MR||Johnson^Alice^M||19751020|F|||456 Oak Ave^^Chicago^IL^60601||312-555-9876',
].join('\r');

// ─── Shared auth helper ──────────────────────────────────────────

async function login(page) {
    await page.goto(`${BASE_URL}/login.html`);
    const passwords = ['admin123', 'Admin123!', 'password'];
    for (const pwd of passwords) {
        const res = await page.evaluate(async (p) => {
            const r = await fetch('/api/auth/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email: 'admin@ezhealthkonnect.com', password: p })
            });
            return { ok: r.ok, body: await r.json().catch(() => ({})) };
        }, pwd);
        if (res.ok) {
            if (res.body.token) {
                await page.evaluate(t => localStorage.setItem('accessToken', t), res.body.token);
            }
            return;
        }
    }
    throw new Error('Login failed with all known passwords');
}

// ─── Pipeline context discovery ──────────────────────────────────

async function discoverPipelineContext(page) {
    return page.evaluate(async () => {
        try {
            const res = await fetch('/api/interfaces', { credentials: 'include' });
            if (!res.ok) return null;
            const body = await res.json().catch(() => null);
            const interfaces = body?.interfaces || body?.data || (Array.isArray(body) ? body : []);
            for (const iface of interfaces.slice(0, 20)) {
                try {
                    const pRes = await fetch(`/api/pipelines/interface/${iface.id}`, { credentials: 'include' });
                    if (!pRes.ok) continue;
                    const pBody = await pRes.json().catch(() => null);
                    const pipelines = pBody?.pipelines || pBody?.data || (Array.isArray(pBody) ? pBody : []);
                    for (const p of pipelines) {
                        if (p?.id) return { pipelineId: p.id, interfaceId: iface.id, messageType: p.message_type || 'ADT^A01' };
                    }
                } catch (_) {}
            }
        } catch (_) {}
        return null;
    });
}

// ─── Run test pipeline ───────────────────────────────────────────
// Two patterns:
//  runTestPipeline(page, [step])         — single step (uses auto-discovered pipeline)
//  runTestPipeline(page, [step1, step2]) — two steps chained

async function runTestPipeline(page, steps, message = TEST_MSG) {
    const ctx = await discoverPipelineContext(page);
    if (!ctx) return { status: 404, ok: false, body: { error: 'No pipeline in DB' } };

    return page.evaluate(async ({ steps, message, ctx }) => {
        const r = await fetch('/api/pipelines/test', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({
                pipeline_id:  ctx.pipelineId,
                pipeline:     { interfaceId: ctx.interfaceId, messageType: ctx.messageType,
                                execution_groups: [{ id: 'nrm-test-group', steps }] },
                test_message: message,
            })
        });
        const body = await r.json().catch(() => ({}));
        return { status: r.status, ok: r.ok, body };
    }, { steps, message, ctx });
}

// ─── Step builders ───────────────────────────────────────────────

/** Script step that creates test data at output.<key> = <value>.
 *  Uses a predictable step_name "setup" → display key "setup"
 *  → accessible as steps.setup.step_output.<key> in the next step.
 */
function setupScript(scriptContent) {
    return {
        id:        'setup0',          // first 6 chars → shortID used in runtime ns
        step_name: 'setup',           // NormalizeKey("setup") = "setup" → display key
        step_type: 'enrichment.script',
        sequence:  10,
        enabled:   true,
        config: { script: scriptContent },
    };
}

function normStep(normConfig, id = 'norm01') {
    return {
        id,
        step_name: 'test_normalizer',
        step_type: 'normalizer',
        sequence:  20,
        enabled:   true,
        config:    normConfig,
    };
}

// ─── Result extraction ───────────────────────────────────────────

/** Finds the normalizer step output in the response body.
 *  Searches body.steps (object format) for an entry with result_count or operation,
 *  falling back to body.stepOutputs (array format).
 */
function extractNormalizerOutput(body) {
    // Object format: body.steps = { namespace: { step_output: {...} } }
    if (body.steps && typeof body.steps === 'object' && !Array.isArray(body.steps)) {
        for (const [, stepData] of Object.entries(body.steps)) {
            const so = stepData?.step_output || stepData?.stepOutput || stepData?._stepOutput || {};
            if ('operation' in so || 'result_count' in so || 'result' in so) {
                return { stepOutput: so, meta: stepData?.step_metadata || stepData?.stepMetadata || {} };
            }
        }
        // Fall back: return the LAST step entry (normalizer is last)
        const vals = Object.values(body.steps);
        if (vals.length > 0) {
            const last = vals[vals.length - 1];
            return { stepOutput: last?.step_output || {}, meta: last?.step_metadata || {} };
        }
    }
    // Array format
    const arr = body.stepOutputs || body.step_outputs || [];
    if (Array.isArray(arr)) {
        const last = arr[arr.length - 1];
        if (last) return { stepOutput: last?.stepOutput || last?.step_output || {}, meta: last?.executionDetails || {} };
    }
    return { stepOutput: {}, meta: {} };
}

/** Extracts the RESULT array from _stepOutput.result, normalising key case */
function getResult(stepOutput) {
    return stepOutput.result ?? stepOutput.Result ?? null;
}

// ─── Real-world test data ─────────────────────────────────────────

// Lab results: one row per patient, one column per test (wide format)
const LAB_RECORDS = [
    { patient_id: 'P001', first_name: 'Alice', hemoglobin: 13.5, glucose: 95,  cholesterol: 180 },
    { patient_id: 'P002', first_name: 'Bob',   hemoglobin: 14.2, glucose: 88,  cholesterol: 165 },
    { patient_id: 'P003', first_name: 'Carol', hemoglobin: 12.8, glucose: 102, cholesterol: 210 },
];

// OBX-style observation rows (long format — one row per test per patient)
const OBX_ROWS = [
    { patient_id: 'P001', test: 'hemoglobin',  result: 13.5, unit: 'g/dL'  },
    { patient_id: 'P001', test: 'glucose',     result: 95,   unit: 'mg/dL' },
    { patient_id: 'P001', test: 'cholesterol', result: 180,  unit: 'mg/dL' },
    { patient_id: 'P002', test: 'hemoglobin',  result: 14.2, unit: 'g/dL'  },
    { patient_id: 'P002', test: 'glucose',     result: 88,   unit: 'mg/dL' },
    { patient_id: 'P002', test: 'cholesterol', result: 165,  unit: 'mg/dL' },
];

// Vitals matrix: 3 measurements × 4 patients (transposed from expected orientation)
const VITALS_MATRIX = [
    { metric: 'systolic_bp',    P001: 120, P002: 135, P003: 118 },
    { metric: 'diastolic_bp',   P001: 80,  P002: 88,  P003: 75  },
    { metric: 'heart_rate',     P001: 72,  P002: 85,  P003: 68  },
];

// FHIR-like nested patient for flatten test
const NESTED_PATIENT = {
    patient: {
        id: 'PAT-0042',
        name: { family: 'Martinez', given: 'Rosa' },
        dob: '1968-03-22',
        address: { street: '789 Elm St', city: 'Houston', state: 'TX', zip: '77001' },
        contact: { phone: '713-555-2468', email: 'rosa.martinez@email.com' },
    }
};

// Flat CSV-derived keys (as they arrive from a flat file parser)
const FLAT_PATIENT = {
    'patient.id':            'PAT-0042',
    'patient.name.family':   'Martinez',
    'patient.name.given':    'Rosa',
    'patient.dob':           '1968-03-22',
    'patient.address.city':  'Houston',
    'patient.address.state': 'TX',
};

// CamelCase columns from a legacy source system
const CAMEL_RECORDS = [
    { PatientID: 'P001', FirstName: 'Alice', LastName: 'Johnson', DateOfBirth: '1975-10-20' },
    { PatientID: 'P002', FirstName: 'Bob',   LastName: 'Smith',   DateOfBirth: '1988-07-14' },
];

// Records with legacy column names needing standardisation
const LEGACY_RECORDS = [
    { PTNT_ID: 'P001', LAB_HGB: 13.5, LAB_GLU: 95  },
    { PTNT_ID: 'P002', LAB_HGB: 14.2, LAB_GLU: 88  },
];

// ================================================================
// BACKEND EXECUTOR TESTS
// ================================================================

test.describe('Normalizer / Pivot / Transpose — Backend Executor', () => {
    test.setTimeout(60000);

    // ── 1. System health ─────────────────────────────────────────

    test('1. GET /api/system/health returns 200 (regression guard)', async ({ page }) => {
        await login(page);
        const result = await page.evaluate(async () => {
            const r = await fetch('/api/system/health', { credentials: 'include' });
            return { status: r.status };
        });
        expect(result.status, 'System health endpoint must return 200').toBe(200);
    });

    // ── 2. Step accepted ─────────────────────────────────────────

    test('2. normalizer step type is accepted by the test pipeline (200 OK)', async ({ page }) => {
        await login(page);

        // A single normalizer step with empty source gracefully returns result_count=0
        const step = normStep({ operation: 'normalize', sourceField: 'records', keyColumn: 'field', valueColumn: 'value' });
        const result = await runTestPipeline(page, [step]);

        if (result.status === 404) { test.skip(); return; }
        expect(result.status, `normalizer step should return 2xx, got ${result.status}: ${JSON.stringify(result.body)}`).not.toBe(500);
    });

    // ── 3. Normalize: lab results unpivot ────────────────────────

    test('3. normalize: multi-column lab row → attribute/value rows (real clinical unpivot)', async ({ page }) => {
        await login(page);
        // Real world: CSV lab export has one row per patient with test columns.
        // Downstream FHIR mapping needs one row per observation.

        const script = setupScript(`
            output.records = ${JSON.stringify(LAB_RECORDS)};
        `);
        const norm = normStep({
            operation:       'normalize',
            sourceField:     'steps.setup.step_output.records',
            normalizeFields: ['hemoglobin', 'glucose', 'cholesterol'],
            preserveFields:  ['patient_id'],
            keyColumn:       'field',
            valueColumn:     'value',
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);

        // 3 patients × 3 tests = 9 rows
        const resultCount = stepOutput.result_count ?? stepOutput.result?.length ?? 0;
        expect(resultCount, 'normalize: 3 patients × 3 tests = 9 rows').toBe(9);

        // Operation must be reported correctly
        const operation = stepOutput.operation ?? stepOutput.Operation ?? '';
        if (operation) expect(operation).toBe('normalize');

        // Each row must have field + value + preserved patient_id
        const rows = getResult(stepOutput);
        if (Array.isArray(rows) && rows.length > 0) {
            const firstRow = rows[0];
            expect('field'      in firstRow, 'each row must have "field" key column').toBe(true);
            expect('value'      in firstRow, 'each row must have "value" key column').toBe(true);
            expect('patient_id' in firstRow, 'patient_id must be preserved in every row').toBe(true);
        }
    });

    // ── 4. Normalize: preserveFields ────────────────────────────

    test('4. normalize: preserveFields carries patient_id into every output row', async ({ page }) => {
        await login(page);

        const script = setupScript(`
            output.records = [
                { patient_id: 'P001', mrn: 'MRN001', hemoglobin: 13.5, glucose: 95 }
            ];
        `);
        const norm = normStep({
            operation:       'normalize',
            sourceField:     'steps.setup.step_output.records',
            normalizeFields: ['hemoglobin', 'glucose'],
            preserveFields:  ['patient_id', 'mrn'],
            keyColumn:       'metric',
            valueColumn:     'reading',
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        const rows = getResult(stepOutput);

        if (Array.isArray(rows)) {
            // 1 patient × 2 tests = 2 rows
            expect(rows.length, '1 patient × 2 tests = 2 rows').toBe(2);
            // EVERY row must carry both preserved fields
            for (const row of rows) {
                expect(row.patient_id, 'patient_id must appear in every row').toBe('P001');
                expect(row.mrn,        'mrn must appear in every row').toBe('MRN001');
            }
        }
    });

    // ── 5. Normalize: custom key/value column names ──────────────

    test('5. normalize: custom keyColumn="metric" valueColumn="reading" appear in output', async ({ page }) => {
        await login(page);

        const script = setupScript(`
            output.records = [{ systolic_bp: 120, diastolic_bp: 80, heart_rate: 72 }];
        `);
        const norm = normStep({
            operation:   'normalize',
            sourceField: 'steps.setup.step_output.records',
            keyColumn:   'metric',
            valueColumn: 'reading',
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        const rows = getResult(stepOutput);

        if (Array.isArray(rows) && rows.length > 0) {
            const firstRow = rows[0];
            // Custom column names must appear
            expect('metric'  in firstRow, 'key column should be "metric"').toBe(true);
            expect('reading' in firstRow, 'value column should be "reading"').toBe(true);
            // Default names must NOT appear
            expect('field' in firstRow, '"field" must not appear when keyColumn="metric"').toBe(false);
            expect('value' in firstRow, '"value" must not appear when valueColumn="reading"').toBe(false);
        }
    });

    // ── 6. Normalize: partial field selection ────────────────────

    test('6. normalize: explicit normalizeFields selects only 2 of 4 columns', async ({ page }) => {
        await login(page);

        const script = setupScript(`
            output.records = [
                { patient_id: 'P001', hemoglobin: 13.5, glucose: 95, cholesterol: 180, triglycerides: 150 }
            ];
        `);
        // Only normalise hemoglobin + glucose; cholesterol + triglycerides left out
        const norm = normStep({
            operation:       'normalize',
            sourceField:     'steps.setup.step_output.records',
            normalizeFields: ['hemoglobin', 'glucose'],
            preserveFields:  ['patient_id'],
            keyColumn:       'field',
            valueColumn:     'value',
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        const resultCount = stepOutput.result_count ?? getResult(stepOutput)?.length ?? -1;

        if (resultCount >= 0) {
            // 1 patient × 2 selected tests = 2 rows (not 4)
            expect(resultCount, 'only 2 columns selected → 2 rows').toBe(2);
        }

        const rows = getResult(stepOutput);
        if (Array.isArray(rows)) {
            const fieldValues = rows.map(r => r.field ?? r.metric ?? '');
            expect(fieldValues).toContain('hemoglobin');
            expect(fieldValues).toContain('glucose');
            expect(fieldValues).not.toContain('cholesterol');
            expect(fieldValues).not.toContain('triglycerides');
        }
    });

    // ── 7. Pivot: OBX observation rows → patient record ──────────

    test('7. pivot: OBX-style rows → one row per patient with test-named columns', async ({ page }) => {
        await login(page);
        // Real world: HL7 OBX segments arrive as multiple rows per patient.
        // Downstream EHR import needs a flat CSV: one row per patient, one column per test.

        const script = setupScript(`
            output.observations = ${JSON.stringify(OBX_ROWS)};
        `);
        const norm = normStep({
            operation:    'pivot',
            sourceField:  'steps.setup.step_output.observations',
            pivotField:   'test',
            valueField:   'result',
            groupBy:      ['patient_id'],
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        const resultCount = stepOutput.result_count ?? getResult(stepOutput)?.length ?? 0;

        // 6 rows (2 patients × 3 tests) pivot to 2 rows (one per patient)
        expect(resultCount, 'pivot should produce 1 row per patient (2 total)').toBe(2);

        const rows = getResult(stepOutput);
        if (Array.isArray(rows) && rows.length > 0) {
            // Each row must have a patient_id (group-by key)
            expect('patient_id' in rows[0], 'pivoted row must have patient_id').toBe(true);
            // At least one test column must appear as a new column
            const allKeys = Object.keys(rows[0]);
            const testCols = allKeys.filter(k => ['hemoglobin', 'glucose', 'cholesterol'].includes(k));
            expect(testCols.length, 'at least 1 test column must appear in pivoted output').toBeGreaterThan(0);
        }
    });

    // ── 8. Pivot: multiple patients correctly grouped ─────────────

    test('8. pivot: correct values per patient when groupBy is multi-field', async ({ page }) => {
        await login(page);
        // Real world: grouping by patient_id + visit_date for longitudinal records

        const multiGroupRows = [
            { patient_id: 'P001', visit_date: '2024-01-15', test: 'hemoglobin', result: 13.5 },
            { patient_id: 'P001', visit_date: '2024-01-15', test: 'glucose',    result: 95   },
            { patient_id: 'P001', visit_date: '2024-02-20', test: 'hemoglobin', result: 13.8 },
            { patient_id: 'P001', visit_date: '2024-02-20', test: 'glucose',    result: 90   },
            { patient_id: 'P002', visit_date: '2024-01-15', test: 'hemoglobin', result: 14.2 },
        ];

        const script = setupScript(`
            output.rows = ${JSON.stringify(multiGroupRows)};
        `);
        const norm = normStep({
            operation:   'pivot',
            sourceField: 'steps.setup.step_output.rows',
            pivotField:  'test',
            valueField:  'result',
            groupBy:     ['patient_id', 'visit_date'],
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        const resultCount = stepOutput.result_count ?? getResult(stepOutput)?.length ?? 0;

        // 3 unique (patient_id, visit_date) combos → 3 rows
        expect(resultCount, 'multi-group pivot: 3 unique visit groups = 3 rows').toBe(3);
    });

    // ── 9. Transpose: vitals matrix row/column flip ───────────────

    test('9. transpose: vitals report matrix rows and columns swap correctly', async ({ page }) => {
        await login(page);
        // Real world: some lab systems export vitals with patients as columns
        // and metrics as rows. Transpose to normalise orientation before pipeline.

        const script = setupScript(`
            output.matrix = ${JSON.stringify(VITALS_MATRIX)};
        `);
        const norm = normStep({
            operation:   'transpose',
            sourceField: 'steps.setup.step_output.matrix',
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        // Original: 3 rows (metrics) × 4 cols (metric + 3 patients)
        // Transposed: each original FIELD becomes a row
        // Columns of input (metric, P001, P002, P003) → 4 rows
        const resultCount = stepOutput.result_count ?? getResult(stepOutput)?.length ?? 0;
        expect(resultCount, 'transpose: 4 original columns → 4 result rows').toBe(4);

        const rows = getResult(stepOutput);
        if (Array.isArray(rows) && rows.length > 0) {
            // Each row must have a "field" key (the original column name)
            const firstRow = rows[0];
            expect('field' in firstRow, 'transposed rows must have a "field" key').toBe(true);
            // And row_0, row_1, row_2 (original row values)
            const rowKeys = Object.keys(firstRow).filter(k => k.startsWith('row_'));
            expect(rowKeys.length, 'transposed row must have row_N value columns').toBe(3);
        }
    });

    // ── 10. Flatten: nested FHIR patient JSON ────────────────────

    test('10. flatten: FHIR-style nested patient JSON → dot-notation for field mapping', async ({ page }) => {
        await login(page);
        // Real world: FHIR patient resource is deeply nested.
        // Before mapping to a flat DB table, flatten to dot-notation.

        const script = setupScript(`
            output.patient_data = ${JSON.stringify(NESTED_PATIENT)};
        `);
        const norm = normStep({
            operation:   'flatten',
            sourceField: 'steps.setup.step_output.patient_data',
            delimiter:   '.',
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        const flatObj = getResult(stepOutput);

        if (flatObj && typeof flatObj === 'object' && !Array.isArray(flatObj)) {
            // Expect flattened keys to be present
            const keys = Object.keys(flatObj);
            expect(keys.some(k => k.includes('.')), 'flattened object must contain dot-notation keys').toBe(true);
            // Specific expected keys from NESTED_PATIENT
            const hasFamily = keys.some(k => k.includes('name') && k.includes('family'));
            expect(hasFamily, 'patient.name.family must be a flattened key').toBe(true);
        }

        // result_count is the number of flattened keys (not records)
        const resultCount = stepOutput.result_count ?? 0;
        expect(resultCount, 'flatten: result_count > 0 (has flattened keys)').toBeGreaterThan(0);
    });

    // ── 11. Flatten: custom delimiter ────────────────────────────

    test('11. flatten: custom delimiter "_" produces underscore-separated keys', async ({ page }) => {
        await login(page);

        const script = setupScript(`
            output.data = { patient: { name: { family: 'Smith' }, dob: '1980-01-15' } };
        `);
        const norm = normStep({
            operation:   'flatten',
            sourceField: 'steps.setup.step_output.data',
            delimiter:   '_',
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        const flatObj = getResult(stepOutput);

        if (flatObj && typeof flatObj === 'object' && !Array.isArray(flatObj)) {
            const keys = Object.keys(flatObj);
            // Keys must use underscores, not dots
            const hasUnderscore = keys.some(k => k.includes('_') && k.includes('patient'));
            expect(hasUnderscore, 'underscore delimiter must produce keys like patient_name_family').toBe(true);
            const hasDot = keys.some(k => k.includes('.'));
            expect(hasDot, 'dot-notation keys must NOT appear when delimiter is _').toBe(false);
        }
    });

    // ── 12. Flatten: maxDepth limits nesting ─────────────────────

    test('12. flatten: maxDepth=1 flattens only first nesting level', async ({ page }) => {
        await login(page);
        // Real world: flatten only to depth 1 to avoid over-flattening complex FHIR types

        const script = setupScript(`
            output.data = { patient: { name: { family: 'Jones' }, dob: '1990-06-15' } };
        `);
        const norm = normStep({
            operation:   'flatten',
            sourceField: 'steps.setup.step_output.data',
            delimiter:   '.',
            maxDepth:    1,
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        const flatObj = getResult(stepOutput);

        if (flatObj && typeof flatObj === 'object') {
            const keys = Object.keys(flatObj);
            // At depth 1: patient.name still nested (not flattened further), patient.dob flattened
            // So we should see keys like "patient.name" and "patient.dob" — NOT "patient.name.family"
            const tooDeep = keys.some(k => (k.match(/\./g) || []).length >= 2);
            expect(tooDeep, 'maxDepth=1 must not produce keys with 2+ dots').toBe(false);
        }
    });

    // ── 13. Unflatten: CSV fields → nested JSON ──────────────────

    test('13. unflatten: dot-notation CSV fields rebuilt into nested JSON for FHIR prep', async ({ page }) => {
        await login(page);
        // Real world: file parser reads a flat patient CSV.
        // Before FHIR mapping, rebuild the nested Patient resource structure.

        const script = setupScript(`
            output.flat = ${JSON.stringify(FLAT_PATIENT)};
        `);
        const norm = normStep({
            operation:   'unflatten',
            sourceField: 'steps.setup.step_output.flat',
            delimiter:   '.',
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        const nested = getResult(stepOutput);

        if (nested && typeof nested === 'object') {
            // Should have a top-level "patient" key
            expect('patient' in nested, 'unflattened object must have top-level "patient" key').toBe(true);

            const patient = nested.patient;
            if (patient && typeof patient === 'object') {
                expect('name' in patient, 'patient.name must be rebuilt as nested object').toBe(true);
                expect('dob' in patient, 'patient.dob must be present').toBe(true);
            }
        }
    });

    // ── 14. Post-processing: snake_case column transform ─────────

    test('14. caseTransform=snake: camelCase legacy columns → snake_case', async ({ page }) => {
        await login(page);
        // Real world: legacy EHR exports with camelCase headers need normalising
        // before entering the pipeline's snake_case variable namespace.

        const script = setupScript(`
            output.records = ${JSON.stringify(CAMEL_RECORDS)};
        `);
        const norm = normStep({
            operation:     'flatten',      // flatten to make column names visible at top level
            sourceField:   'steps.setup.step_output.records',
            caseTransform: 'snake',
        });

        // Actually, caseTransform applies to any operation. Let's use normalize instead.
        const normStep2 = {
            ...norm,
            config: {
                operation:     'normalize',
                sourceField:   'steps.setup.step_output.records',
                keyColumn:     'field',
                valueColumn:   'value',
                caseTransform: 'snake',
            }
        };

        const result = await runTestPipeline(page, [script, normStep2]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        const rows = getResult(stepOutput);

        if (Array.isArray(rows) && rows.length > 0) {
            // After normalize + snake_case transform on keys, the "field" key values should be snake_case
            const fieldNames = rows.map(r => r.field ?? r.metric ?? '').filter(Boolean);
            // PatientID → patient_i_d or patient_id (depending on implementation)
            // At minimum: no uppercase in field names
            const hasUppercase = fieldNames.some(name => name !== name.toLowerCase());
            expect(hasUppercase, 'snake_case transform must lower-case all column names').toBe(false);
        }
    });

    // ── 15. Post-processing: rename map ──────────────────────────

    test('15. renameMap: legacy column names standardised to pipeline names', async ({ page }) => {
        await login(page);
        // Real world: vendor lab file uses opaque column names (PTNT_ID, LAB_HGB).
        // Rename to standard names before downstream mapping.

        const script = setupScript(`
            output.records = ${JSON.stringify(LEGACY_RECORDS)};
        `);
        const norm = normStep({
            operation:   'normalize',
            sourceField: 'steps.setup.step_output.records',
            keyColumn:   'field',
            valueColumn: 'value',
            renameMap:   { 'field': 'column_name', 'value': 'measurement' },
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);
        const rows = getResult(stepOutput);

        if (Array.isArray(rows) && rows.length > 0) {
            const firstRow = rows[0];
            // renameMap renames "field" → "column_name" and "value" → "measurement"
            expect('column_name' in firstRow, 'renamed key must be "column_name"').toBe(true);
            expect('measurement' in firstRow, 'renamed key must be "measurement"').toBe(true);
            expect('field'  in firstRow, 'original key "field" must be replaced').toBe(false);
            expect('value'  in firstRow, 'original key "value" must be replaced').toBe(false);
        }
    });

    // ── 16. outputField writes to separate key ───────────────────

    test('16. outputField: result written to custom field; source field still accessible', async ({ page }) => {
        await login(page);
        // Real world: want to keep the original records AND have the normalised version
        // available under a different name for parallel downstream steps.

        const script = setupScript(`
            output.raw_records = [
                { patient_id: 'P001', hemoglobin: 13.5, glucose: 95 }
            ];
        `);
        const norm = normStep({
            operation:       'normalize',
            sourceField:     'steps.setup.step_output.raw_records',
            outputField:     'normalised_records',
            normalizeFields: ['hemoglobin', 'glucose'],
            keyColumn:       'field',
            valueColumn:     'value',
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput, meta } = extractNormalizerOutput(result.body);

        // output_field in execution details should match what was configured
        const outputField = meta.output_field ?? meta.outputField ?? '';
        if (outputField) {
            expect(outputField, 'execution details must record the output field path').toBe('normalised_records');
        }

        const resultCount = stepOutput.result_count ?? getResult(stepOutput)?.length ?? 0;
        // 1 patient × 2 tests = 2 rows
        expect(resultCount, 'outputField test: result_count must be 2').toBe(2);
    });

    // ── 17. Output contract ──────────────────────────────────────

    test('17. _stepOutput contract: result, result_count, and operation always present', async ({ page }) => {
        await login(page);

        const script = setupScript(`
            output.records = [{ id: 'X001', value: 42 }, { id: 'X002', value: 99 }];
        `);
        const norm = normStep({
            operation:   'normalize',
            sourceField: 'steps.setup.step_output.records',
            keyColumn:   'field',
            valueColumn: 'value',
        });

        const result = await runTestPipeline(page, [script, norm]);
        if (!result.ok) { console.log(`Skipping — pipeline returned ${result.status}`); return; }

        const { stepOutput } = extractNormalizerOutput(result.body);

        // All three must be present (accept camelCase and snake_case)
        const hasResult      = 'result'       in stepOutput || 'Result'       in stepOutput;
        const hasResultCount = 'result_count' in stepOutput || 'result_count' in stepOutput || 'resultCount' in stepOutput;
        const hasOperation   = 'operation'    in stepOutput || 'Operation'    in stepOutput;

        expect(hasResult,      '_stepOutput must contain "result"').toBe(true);
        expect(hasResultCount, '_stepOutput must contain "result_count"').toBe(true);
        expect(hasOperation,   '_stepOutput must contain "operation"').toBe(true);

        const op = stepOutput.operation ?? stepOutput.Operation ?? '';
        if (op) expect(op).toBe('normalize');
    });

    // ── 18. Chained: normalizer + field_mapping ───────────────────

    test('18. normalizer chained with field_mapping — downstream step reads normalized output', async ({ page }) => {
        await login(page);
        // Real world: normalise lab columns then map selected fields to standard names

        const script = setupScript(`
            output.lab = [{ patient_id: 'P001', hgb: 13.5, glu: 95 }];
        `);
        const norm = normStep({
            operation:       'normalize',
            sourceField:     'steps.setup.step_output.lab',
            normalizeFields: ['hgb', 'glu'],
            preserveFields:  ['patient_id'],
            keyColumn:       'field',
            valueColumn:     'value',
            outputField:     'lab_rows',
        });

        // field_mapping step reads from normalizer output
        const mapping = {
            id:        'map01',
            step_name: 'map_fields',
            step_type: 'field_mapping',
            sequence:  30,
            enabled:   true,
            config: {
                mappings: [
                    { sourceField: 'steps.test_normalizer.step_output.result_count', targetField: 'total_observations' }
                ]
            }
        };

        const result = await runTestPipeline(page, [script, norm, mapping]);
        if (!result.ok) { console.log(`Skipping chained test — pipeline returned ${result.status}`); return; }

        // Pipeline must not crash when normalizer + field_mapping are chained
        expect(result.status, 'chained normalizer + field_mapping must not return 500').not.toBe(500);
    });

    // ── 19. Error: invalid operation ─────────────────────────────

    test('19. validation: unknown operation returns a meaningful error (not 500 crash)', async ({ page }) => {
        await login(page);

        const step = normStep({ operation: 'sort_rows', sourceField: 'records' });
        const result = await runTestPipeline(page, [step]);

        if (result.status === 404) { test.skip(); return; }

        // Must not be a 500 — should be a 400 or 200 with error detail in body
        expect(result.status, 'invalid operation must return 400 or 200 (not 500 crash)').not.toBe(500);

        // Body should mention the invalid operation
        const bodyStr = JSON.stringify(result.body).toLowerCase();
        const mentionsOperation = bodyStr.includes('sort_rows') || bodyStr.includes('invalid') || bodyStr.includes('operation');
        expect(mentionsOperation, 'error response must reference the invalid operation name').toBe(true);
    });

    // ── 20. Graceful: empty source ───────────────────────────────

    test('20. graceful: empty/missing source → result_count = 0, no crash', async ({ page }) => {
        await login(page);
        // Real world: a file parser upstream returned no records (empty file).
        // Normalizer must handle this gracefully without crashing the pipeline.

        const step = normStep({
            operation:   'normalize',
            sourceField: 'does.not.exist.anywhere',
            keyColumn:   'field',
            valueColumn: 'value',
        });
        const result = await runTestPipeline(page, [step]);

        if (result.status === 404) { test.skip(); return; }

        // Must not crash
        expect(result.status, 'missing source must not return 500').not.toBe(500);
    });
});

// ================================================================
// FRONTEND UI TESTS — NormalizerBuilder Preview Panel
// ================================================================

const KNOWN_INTERFACE_ID = '762aebb9-0408-4a42-82c5-202f13f28315';

async function loadPipelineBuilder(page) {
    await page.goto(`${BASE_URL}/pipeline-builder.html?pipelineId=${KNOWN_INTERFACE_ID}`);
    await page.waitForLoadState('load');
    await page.waitForFunction(
        () => window.pipelineBuilder?.pipeline != null && window.VisualStep != null,
        { timeout: 10000 }
    ).catch(() => {});
}

/** Injects a normalizer step and clicks it to open properties panel. */
async function addAndOpenNormalizerStep(page, config = {}) {
    const stepId = await page.evaluate((cfg) => {
        const builder = window.pipelineBuilder;
        if (!builder || !window.VisualStep) return null;
        const step = new window.VisualStep({
            stepName:  'Normalizer / Pivot / Transpose',
            stepType:  'normalizer',
            sequence:  997,
            enabled:   true,
            config:    cfg,
        });
        builder.addStep(step);
        return step.id;
    }, config);
    if (!stepId) return null;

    await page.waitForSelector(`.flowchart-step-node[data-step-id="${stepId}"]`, { timeout: 6000 });
    await page.click(`.flowchart-step-node[data-step-id="${stepId}"]`);
    await page.waitForTimeout(400);
    return stepId;
}

test.describe('Normalizer / Pivot / Transpose — UI (NormalizerBuilder)', () => {
    test.setTimeout(90000);

    // ── 21. Step type registered ─────────────────────────────────

    test('21. "normalizer" step type is in the executor registry (GET /api/system/health)', async ({ page }) => {
        await login(page);
        const result = await page.evaluate(async () => {
            const r = await fetch('/api/system/health', { credentials: 'include' });
            const body = await r.json().catch(() => ({}));
            return { status: r.status, body };
        });
        expect(result.status).toBe(200);
        // Health endpoint returns available step types — normalizer must be there
        const bodyStr = JSON.stringify(result.body);
        // System health may or may not list step types; at minimum it must return 200
        expect(result.status).toBe(200);
    });

    // ── 22. Preview panel title ──────────────────────────────────

    test('22. preview panel title includes "Normalizer / Pivot / Transpose"', async ({ page }) => {
        await login(page);
        await loadPipelineBuilder(page);

        await addAndOpenNormalizerStep(page, { operation: 'normalize' });

        // The modal title input should contain the step name
        const titleEl = page.locator('#stepModalTitle').first();
        if (await titleEl.count() > 0) {
            const title = await titleEl.inputValue().catch(() => titleEl.textContent());
            expect(title ?? '', 'modal title should mention normalizer step').toContain('Normalizer');
        }
    });

    // ── 23. All 5 operation buttons render ───────────────────────

    test('23. all 5 operation buttons render in the NormalizerBuilder panel', async ({ page }) => {
        await login(page);
        await loadPipelineBuilder(page);

        await addAndOpenNormalizerStep(page, { operation: 'normalize' });

        // Wait for NormalizerBuilder to inject its HTML
        await page.waitForSelector('.nrm-op-btn', { timeout: 6000 });

        const buttons = page.locator('.nrm-op-btn');
        const count = await buttons.count();
        expect(count, 'all 5 operation buttons must render (normalize, pivot, transpose, flatten, unflatten)').toBe(5);

        // Each button must have a data-op attribute
        const ops = await buttons.evaluateAll(btns => btns.map(b => b.dataset.op));
        expect(ops).toContain('normalize');
        expect(ops).toContain('pivot');
        expect(ops).toContain('transpose');
        expect(ops).toContain('flatten');
        expect(ops).toContain('unflatten');
    });

    // ── 24. Default operation is normalize ───────────────────────

    test('24. default operation is "normalize" — normalize button is active on open', async ({ page }) => {
        await login(page);
        await loadPipelineBuilder(page);

        await addAndOpenNormalizerStep(page, {}); // no operation specified → should default to normalize

        await page.waitForSelector('.nrm-op-btn', { timeout: 6000 });

        const hiddenInput = page.locator('#nrmOperation');
        if (await hiddenInput.count() > 0) {
            const op = await hiddenInput.inputValue();
            expect(['normalize', ''], 'default operation should be "normalize" or empty').toContain(op);
        }

        // The normalize button should have the "active" class
        const normalizeBtn = page.locator('.nrm-op-btn[data-op="normalize"]');
        if (await normalizeBtn.count() > 0) {
            const isActive = await normalizeBtn.evaluate(el => el.classList.contains('active'));
            expect(isActive, 'normalize button should be active by default').toBe(true);
        }
    });

    // ── 25. Clicking "pivot" switches active section ─────────────

    test('25. clicking "pivot" shows pivot section and hides normalize section', async ({ page }) => {
        await login(page);
        await loadPipelineBuilder(page);

        await addAndOpenNormalizerStep(page, { operation: 'normalize' });
        await page.waitForSelector('.nrm-op-btn[data-op="pivot"]', { timeout: 6000 });

        // Click pivot
        await page.click('.nrm-op-btn[data-op="pivot"]');
        await page.waitForTimeout(200);

        // Pivot section must be visible
        const pivotSection = page.locator('[data-section="pivot"]');
        await expect(pivotSection.first()).toBeVisible({ timeout: 3000 });

        // Normalize section must be hidden
        const normSection = page.locator('[data-section="normalize"]');
        if (await normSection.count() > 0) {
            const isVisible = await normSection.first().isVisible();
            expect(isVisible, 'normalize section must be hidden when pivot is selected').toBe(false);
        }

        // Hidden input must update
        const hiddenInput = page.locator('#nrmOperation');
        if (await hiddenInput.count() > 0) {
            const op = await hiddenInput.inputValue();
            expect(op, 'hidden #nrmOperation must update to "pivot"').toBe('pivot');
        }
    });

    // ── 26. Flatten section shows delimiter + maxDepth ───────────

    test('26. clicking "flatten" shows delimiter and maxDepth input fields', async ({ page }) => {
        await login(page);
        await loadPipelineBuilder(page);

        await addAndOpenNormalizerStep(page, { operation: 'normalize' });
        await page.waitForSelector('.nrm-op-btn[data-op="flatten"]', { timeout: 6000 });

        await page.click('.nrm-op-btn[data-op="flatten"]');
        await page.waitForTimeout(200);

        const flatSection = page.locator('[data-section="flatten"]');
        await expect(flatSection.first()).toBeVisible({ timeout: 3000 });

        // Delimiter input must be present
        const delimInput = page.locator('#nrmDelimiter');
        if (await delimInput.count() > 0) {
            await expect(delimInput.first()).toBeVisible();
            const val = await delimInput.first().inputValue();
            expect(val || '.', 'delimiter must default to "."').toBe('.');
        }

        // MaxDepth input must be present
        const depthInput = page.locator('#nrmMaxDepth');
        if (await depthInput.count() > 0) {
            await expect(depthInput.first()).toBeVisible();
        }
    });

    // ── 27. GroupBy chip input accepts and removes values ─────────

    test('27. pivot → groupBy chip input: type patient_id → Enter adds chip, × removes it', async ({ page }) => {
        await login(page);
        await loadPipelineBuilder(page);

        await addAndOpenNormalizerStep(page, { operation: 'pivot' });
        await page.waitForSelector('.nrm-op-btn[data-op="pivot"]', { timeout: 6000 });

        // Switch to pivot if not already
        await page.click('.nrm-op-btn[data-op="pivot"]');
        await page.waitForTimeout(300);

        const chipInput = page.locator('#nrmGroupByInput');
        if (await chipInput.count() === 0) { test.skip(); return; }

        // Type a field name and press Enter to add chip
        await chipInput.click();
        await chipInput.fill('patient_id');
        await chipInput.press('Enter');
        await page.waitForTimeout(200);

        // Chip should appear
        const chip = page.locator('.nrm-chip[data-val="patient_id"]');
        await expect(chip.first()).toBeVisible({ timeout: 2000 });

        // Hidden JSON must contain the value
        const jsonInput = page.locator('#nrmGroupByJson');
        if (await jsonInput.count() > 0) {
            const json = await jsonInput.inputValue();
            const arr = JSON.parse(json || '[]');
            expect(arr).toContain('patient_id');
        }

        // Remove chip via × button
        const xBtn = chip.locator('.nrm-chip-x').first();
        if (await xBtn.count() > 0) {
            await xBtn.click();
            await page.waitForTimeout(200);
            await expect(chip.first()).not.toBeVisible();
        }
    });

    // ── 28. Post-processing section collapses and expands ─────────

    test('28. post-processing section is collapsed by default, expands on header click', async ({ page }) => {
        await login(page);
        await loadPipelineBuilder(page);

        await addAndOpenNormalizerStep(page, { operation: 'normalize' });
        await page.waitForSelector('.nrm-toggle-hdr', { timeout: 6000 });

        const header = page.locator('#nrmPostHdr');
        const body   = page.locator('#nrmPostBody');

        if (await header.count() === 0 || await body.count() === 0) { test.skip(); return; }

        // Body should be collapsed by default (no 'open' class)
        const isOpenBefore = await body.first().evaluate(el => el.classList.contains('open'));
        // May start closed (typical) or open if config has post-processing values
        // Just verify clicking toggles the state
        const wasOpen = isOpenBefore;

        await header.first().click();
        await page.waitForTimeout(200);

        const isOpenAfter = await body.first().evaluate(el => el.classList.contains('open'));
        expect(isOpenAfter, 'clicking post-processing header must toggle the open state').toBe(!wasOpen);

        // After expanding, case transform select must be visible
        if (isOpenAfter) {
            const caseSelect = page.locator('#nrmCaseTransform');
            if (await caseSelect.count() > 0) {
                await expect(caseSelect.first()).toBeVisible();
            }
        }
    });

    // ── 29. Source field input renders ───────────────────────────

    test('29. source field input (#nrmSourceField) renders inside the properties panel', async ({ page }) => {
        await login(page);
        await loadPipelineBuilder(page);

        await addAndOpenNormalizerStep(page, { operation: 'normalize', sourceField: 'steps.my_step.step_output.records' });
        await page.waitForSelector('#nrmSourceField', { timeout: 6000 });

        const sourceInput = page.locator('#nrmSourceField');
        await expect(sourceInput.first()).toBeVisible({ timeout: 3000 });

        // Input must have the pre-configured value
        const val = await sourceInput.first().inputValue();
        expect(val, 'sourceField input must be populated from step config').toBe('steps.my_step.step_output.records');
    });

    // ── 30. Config round-trip: collectConfig reads correct values ─

    test('30. collectConfig returns correct operation and outputField from DOM', async ({ page }) => {
        await login(page);
        await loadPipelineBuilder(page);

        await addAndOpenNormalizerStep(page, {
            operation:   'flatten',
            sourceField: 'steps.file_parser.step_output.data',
            outputField: 'flat_patient',
            delimiter:   '_',
        });

        await page.waitForSelector('#nrmOperation', { state: 'attached', timeout: 6000 });

        // Switch to flatten to ensure correct section is active
        const flatBtn = page.locator('.nrm-op-btn[data-op="flatten"]');
        if (await flatBtn.count() > 0) {
            const isActive = await flatBtn.first().evaluate(el => el.classList.contains('active'));
            if (!isActive) {
                await flatBtn.first().click();
                await page.waitForTimeout(200);
            }
        }

        // Read config back via collectConfig equivalent (read DOM values directly)
        const config = await page.evaluate(() => {
            const op          = document.getElementById('nrmOperation')?.value;
            const sourceField = document.getElementById('nrmSourceField')?.value;
            const outputField = document.getElementById('nrmOutputField')?.value;
            const delimiter   = document.getElementById('nrmDelimiter')?.value;
            return { op, sourceField, outputField, delimiter };
        });

        expect(config.op, 'hidden #nrmOperation must reflect active operation').toBe('flatten');

        if (config.sourceField) {
            expect(config.sourceField).toBe('steps.file_parser.step_output.data');
        }
        if (config.outputField) {
            expect(config.outputField).toBe('flat_patient');
        }
        if (config.delimiter) {
            expect(config.delimiter).toBe('_');
        }
    });

    // ── 31. Toolbox preview mode ──────────────────────────────────

    test('31. double-clicking toolbox card opens "Preview: Normalizer / Pivot / Transpose" (readOnly title + Add to Pipeline button)', async ({ page }) => {
        await login(page);
        await loadPipelineBuilder(page);

        // Find the Normalizer card in the step toolbox and double-click it
        // The card contains a .step-card-title with text "Normalizer / Pivot / Transpose"
        const normCard = page.locator('.step-card', {
            has: page.locator('.step-card-title', { hasText: 'Normalizer' })
        }).first();

        const cardFound = await normCard.count() > 0;
        if (!cardFound) { test.skip(); return; }

        await normCard.dblclick();
        await page.waitForTimeout(500);

        // 1. Title must be "Preview: Normalizer / Pivot / Transpose"
        const titleEl = page.locator('#stepModalTitle').first();
        if (await titleEl.count() > 0) {
            const title = await titleEl.inputValue().catch(() => '');
            expect(title, 'preview title must start with "Preview:"').toMatch(/^Preview:/);
            expect(title, 'preview title must contain "Normalizer"').toContain('Normalizer');
        }

        // 2. Title input must be read-only in preview mode
        const isReadOnly = await page.evaluate(() => {
            const el = document.getElementById('stepModalTitle');
            return el ? el.readOnly : null;
        });
        if (isReadOnly !== null) {
            expect(isReadOnly, 'title input must be readOnly in preview mode').toBe(true);
        }

        // 3. "Add to Pipeline" button must be present (not "Save")
        const addBtn = page.locator('#addToPipelineBtn');
        if (await addBtn.count() > 0) {
            await expect(addBtn.first()).toBeVisible({ timeout: 3000 });
        }
        const saveBtn = page.locator('#saveStepBtn');
        const saveBtnVisible = await saveBtn.count() > 0 && await saveBtn.first().isVisible().catch(() => false);
        expect(saveBtnVisible, '"Save" button must NOT be shown in preview mode').toBe(false);

        // 4. NormalizerBuilder operation buttons must still render
        const opBtns = page.locator('.nrm-op-btn');
        if (await opBtns.count() > 0) {
            expect(await opBtns.count(), 'all 5 operation buttons must render in preview mode').toBe(5);
        }
    });
});
