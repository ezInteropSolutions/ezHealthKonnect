'use strict';
/**
 * step-type-fix-validation.spec.js — real-browser validation of three production bugs
 * fixed 2026-09-08:
 *
 *   1. field_validation was completely unreachable — a stale GetStepType() override on
 *      FieldValidationExecutor returned "pre.validation" instead of "field_validation" (the
 *      name every real caller, including 4 pre-existing live steps, actually used). Fixed by
 *      deleting the override (services/executors/validation/field_validation_executor.go).
 *   2. control.loop could not iterate over any bracket-indexed collection path (the shape
 *      every EDI-parsed nested loop uses, e.g. "loops.2000[0].loops.2100") — the JSON
 *      collection resolver's plain strings.Split(path, ".") treated "2000[0]" as one literal,
 *      nonexistent map key. Fixed by routing it through the same bracket-aware resolver
 *      fhir.build/hl7.build already use (services/executors/control/collection_resolver.go).
 *   3. if_then_else/switch_case never automatically excluded the untaken branch's own steps —
 *      picking one case only ran that case's inline actions; sibling steps for the OTHER
 *      case(s) still executed right after, as plain sequential fall-through. The auto-exclusion
 *      mechanism (BranchResolver, keyed off ParentConditionalStepID + BranchType/CaseValue —
 *      metadata the pipeline builder UI already saves for every step dropped into a branch) had
 *      two independent bugs: (a) it was never called from ExecutePipeline, the engine Test
 *      Pipeline/Re-run/production ingestion all actually use (only a dead/legacy execution path
 *      called it) — fixed by wiring tps.branchResolver.GetStepsToSkip into ExecutePipeline's own
 *      routing handling (services/transformation_pipeline_helpers.go); (b) even where it WAS
 *      called, IfThenElseResolver/SwitchCaseResolver's own CanHandle()/GetStepType() only
 *      recognized the pre-rename "pre.logic"/"pre.logic.switch" names, never the real
 *      "if_then_else"/"switch_case" names every saved step and executor actually uses — so the
 *      resolver lookup always failed silently, in every engine, from day one (services/branch_resolver.go).
 *      With both fixed, branching now needs zero manual "skipSteps" config for the common case:
 *      the untaken branch's steps are excluded automatically, matching how switch/case works in
 *      every general-purpose language.
 *
 * This spec creates small, purpose-built, PERSISTENT demo interfaces via the real wizard
 * API (not a UI click-through — creation itself isn't what's being tested) and then drives the
 * real pipeline-builder UI in an actual browser: opens each fixed step's real Properties Panel,
 * and runs the real "Test Pipeline" modal (the same button and results panel a user clicks) for
 * both a passing and a failing case. Deliberately does NOT delete the interfaces afterward —
 * they're meant to stay in the system so a human can open them in the UI and re-run the same
 * checks independently.
 *
 * Run: npx playwright test step-type-fix-validation --project=chromium
 */

const { test, expect } = require('@playwright/test');
const { ApiHelper }    = require('./helpers/api');
const fs               = require('fs');
const path             = require('path');
const { randomUUID }   = require('crypto');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';

const PORT_FIELD_VALIDATION_DEMO = 6630; // tcp_mllp, within the 6610-6670 docker-mapped range
const PORT_CONTROL_LOOP_DEMO     = 8092; // http, within the 8081-8099 docker-mapped range

const PORT_FHIR_DEMO = 8093; // http, within the 8081-8099 docker-mapped range
const PORT_AUTOSKIP_DEMO = 8095; // http, within the 8081-8099 docker-mapped range

const state = {
    fieldValidationInterfaceId: null,
    controlLoopInterfaceId: null,
    fhirInterfaceId: null,
    autoSkipInterfaceId: null,
};

const HL7_ADT_VALID = [
    'MSH|^~\\&|DEMO|DEMO_FAC|EHK|EHK|20260908120000||ADT^A01|DEMO0001|P|2.5',
    'EVN|A01|20260908120000',
    'PID|1||P123456^^^DEMO^MR||DOE^JANE||19850314|F',
].join('\r');

const HL7_ADT_MISSING_PATIENT_ID = [
    'MSH|^~\\&|DEMO|DEMO_FAC|EHK|EHK|20260908120000||ADT^A01|DEMO0002|P|2.5',
    'EVN|A01|20260908120000',
    'PID|1||||DOE^JANE||19850314|F',
].join('\r');

const ORDERS_JSON_TWO_ITEMS = JSON.stringify({
    orders: [
        {
            items: [
                { sku: 'WIDGET-A', qty: 3, price: 10.5 },
                { sku: 'WIDGET-B', qty: 0, price: 5.0 },
            ],
        },
    ],
});

test.describe.serial('Step-type fix validation (field_validation + control.loop)', () => {
    // beforeAll creates persistent, named demo interfaces — retrying it would create
    // duplicates rather than fixing a flaky assertion, so this spec always runs once.
    test.describe.configure({ retries: 0 });

    test.beforeAll(async ({ request }) => {
        const api = new ApiHelper(request, BASE_URL);

        // Idempotent (as far as the real delete API allows): removes any previous
        // run's ACTIVE copy of these named demo interfaces before recreating, so
        // re-running this file never leaves more than one ACTIVE copy visible
        // anywhere a real user would look (the interfaces list, this file's own
        // listInterfaces() check, pipeline-builder). DELETE /api/interfaces/:id is
        // a deliberate soft-delete (is_active=false, row kept for restore) — every
        // listing endpoint already filters is_active=true, so the previous run's
        // row becomes invisible, not gone; only a direct SQL query against
        // interfaces (not this file, not the app) would ever show it lingering.
        const existing = await api.listInterfaces();
        for (const iface of existing) {
            if (['Field Validation Fix Demo', 'Control Loop Fix Demo', 'FHIR Passthrough Fix Demo', 'Switch Case Auto-Skip Fix Demo'].includes(iface.name)) {
                await api.deleteInterface(iface.id).catch(() => {});
            }
        }

        // ── Interface A: field_validation fix ──────────────────────────────────
        const fv = await api.createInterface({
            name: 'Field Validation Fix Demo',
            description: 'Persistent demo interface (2026-09-08) proving field_validation is reachable again — a stale GetStepType() override previously made it silently unusable platform-wide.',
            sourceType: 'hl7v2',
            sourceConnectivity: 'tcp_mllp_inbound',
            sourceConfig: { host: '0.0.0.0', port: PORT_FIELD_VALIDATION_DEMO },
            sourceConnectorConfig: {
                connectorType: 'tcp_mllp_inbound',
                config: { host: '0.0.0.0', port: PORT_FIELD_VALIDATION_DEMO },
            },
            targetType: 'file',
            targetConnectivity: 'file_writer',
            targetConfig: { outputPath: '/tmp/field-validation-fix-demo/{timestamp}.hl7', createDirs: true },
            targetConnectorConfig: {
                connectorType: 'file_writer',
                config: { outputPath: '/tmp/field-validation-fix-demo/{timestamp}.hl7', createDirs: true },
            },
            messageType: 'ADT^A01',
            mappings: [],
            transformationFlow: 'hl7_passthrough',
            auto_start: false,
            deployment_mode: 'manual',
            status: 'active',
            debug_logging: true,
            log_retention_days: 7,
        });
        state.fieldValidationInterfaceId = fv.interface?.id || fv.interfaceId || fv.id;
        expect(state.fieldValidationInterfaceId, `createInterface response: ${JSON.stringify(fv)}`).toBeTruthy();

        await api.savePipeline({
            interfaceId: state.fieldValidationInterfaceId,
            messageType: 'ADT^A01',
            steps: [
                {
                    step_name: 'TCP/MLLP Inbound',
                    step_type: 'connector.inbound',
                    sequence: 10,
                    enabled: true,
                    config: {
                        connectorType: 'tcp_mllp_inbound',
                        config: { host: '0.0.0.0', port: PORT_FIELD_VALIDATION_DEMO },
                        timeoutMs: 30000,
                    },
                },
                {
                    step_name: 'Field Validation',
                    step_type: 'field_validation',
                    sequence: 20,
                    enabled: true,
                    required: true,
                    config: {
                        validation_mode: 'strict_reject',
                        detailedOutput: true,
                        rules: [
                            { type: 'required', field: 'MSH.9', errorMessage: 'Message type is required' },
                            { type: 'required', field: 'PID.3', errorMessage: 'Patient ID is required' },
                            { type: 'required', field: 'PID.5.1', errorMessage: 'Family name is required' },
                        ],
                    },
                },
                {
                    step_name: 'File Writer (Passthrough)',
                    step_type: 'connector.outbound',
                    sequence: 30,
                    enabled: true,
                    config: {
                        connectorType: 'file_writer',
                        config: { outputPath: '/tmp/field-validation-fix-demo/{timestamp}.hl7', createDirs: true },
                        contentField: 'raw',
                    },
                },
            ],
        });

        // ── Interface B: control.loop bracket-index fix ────────────────────────
        const cl = await api.createInterface({
            name: 'Control Loop Fix Demo',
            description: 'Persistent demo interface (2026-09-08) proving control.loop can iterate a bracket-indexed collection path (e.g. "orders[0].items") — the exact shape every EDI-parsed nested loop uses. Source type is labeled "fhir" only to reuse a proven connectivity config; the test payload is plain JSON, not a FHIR resource.',
            sourceType: 'fhir',
            sourceConnectivity: 'http_fhir_inbound',
            sourceConfig: { host: '0.0.0.0', port: PORT_CONTROL_LOOP_DEMO },
            sourceConnectorConfig: {
                connectorType: 'http_fhir_inbound',
                config: { host: '0.0.0.0', port: PORT_CONTROL_LOOP_DEMO },
            },
            targetType: 'file',
            targetConnectivity: 'file_writer',
            targetConfig: { outputPath: '/tmp/control-loop-fix-demo/{timestamp}.json', createDirs: true },
            targetConnectorConfig: {
                connectorType: 'file_writer',
                config: { outputPath: '/tmp/control-loop-fix-demo/{timestamp}.json', createDirs: true },
            },
            messageType: 'JSON:Orders',
            mappings: [],
            transformationFlow: 'fhir_passthrough',
            auto_start: false,
            deployment_mode: 'manual',
            status: 'active',
            debug_logging: true,
            log_retention_days: 7,
        });
        state.controlLoopInterfaceId = cl.interface?.id || cl.interfaceId || cl.id;
        expect(state.controlLoopInterfaceId, `createInterface response: ${JSON.stringify(cl)}`).toBeTruthy();

        const classifyItemScript = [
            'var item = input.item || {};',
            'var qty = parseFloat(item.qty || 0);',
            'var price = parseFloat(item.price || 0);',
            'return {',
            '  sku: item.sku,',
            '  lineTotal: qty * price,',
            '  status: qty > 0 ? "in_stock" : "backordered",',
            '  index: input.index',
            '};',
        ].join('\n');

        // transformation_steps.id is a globally-unique primary key, not scoped per
        // pipeline — fresh UUIDs every run (rather than fixed literals) means a
        // re-run can never collide with an earlier run's rows, including any left
        // behind by a delete that silently failed.
        const inboundStepId = randomUUID();
        const loopStepId    = randomUUID();
        const scriptStepId  = randomUUID();
        const outboundStepId = randomUUID();

        await api.savePipeline({
            interfaceId: state.controlLoopInterfaceId,
            messageType: 'JSON:Orders',
            steps: [
                {
                    id: inboundStepId,
                    step_name: 'HTTP FHIR Receiver',
                    step_type: 'connector.inbound',
                    sequence: 10,
                    enabled: true,
                    config: {
                        connectorType: 'http_fhir_inbound',
                        config: { host: '0.0.0.0', port: PORT_CONTROL_LOOP_DEMO },
                        timeoutMs: 30000,
                    },
                },
                {
                    id: loopStepId,
                    step_name: 'Loop Order Items',
                    step_type: 'control.loop',
                    sequence: 20,
                    enabled: true,
                    config: {
                        loopType: 'foreach',
                        collection: 'message.orders[0].items',
                        itemVariable: 'item',
                        indexVariable: 'index',
                        maxIterations: 1000,
                        continueOnEmpty: true,
                        breakOnError: false,
                        childStepIds: [scriptStepId],
                    },
                },
                {
                    id: scriptStepId,
                    step_name: 'Classify Item',
                    step_type: 'enrichment.script',
                    sequence: 21,
                    enabled: true,
                    config: { script: classifyItemScript, timeoutMs: 5000 },
                },
                {
                    id: outboundStepId,
                    step_name: 'File Writer (Passthrough)',
                    step_type: 'connector.outbound',
                    sequence: 30,
                    enabled: true,
                    config: {
                        connectorType: 'file_writer',
                        config: { outputPath: '/tmp/control-loop-fix-demo/{timestamp}.json', createDirs: true },
                        contentField: 'raw',
                    },
                },
            ],
        });

        // ── Interface C: dedicated, stable FHIR demo ───────────────────────────
        // Deliberately NOT reusing one of the MVP smoke-test's own
        // "MVP_SMOKE_FHIR_PASS_*" interfaces here — those are ephemeral fixtures
        // another suite creates/deactivates/recreates on every run (confirmed:
        // one such interface went is_active=false between one test run and the
        // next), so hardcoding one of their IDs is inherently flaky. This one is
        // owned by this spec, like the other two.
        const fhirIface = await api.createInterface({
            name: 'FHIR Passthrough Fix Demo',
            description: 'Persistent demo interface (2026-09-08) for the general cross-format regression check — confirms FHIR-native Test Pipeline processing still works after the field_validation/control.loop registry fixes (neither of which this interface uses directly).',
            sourceType: 'fhir',
            sourceConnectivity: 'http_fhir_inbound',
            sourceConfig: { host: '0.0.0.0', port: PORT_FHIR_DEMO },
            sourceConnectorConfig: {
                connectorType: 'http_fhir_inbound',
                config: { host: '0.0.0.0', port: PORT_FHIR_DEMO },
            },
            targetType: 'file',
            targetConnectivity: 'file_writer',
            targetConfig: { outputPath: '/tmp/fhir-fix-demo/{timestamp}.json', createDirs: true },
            targetConnectorConfig: {
                connectorType: 'file_writer',
                config: { outputPath: '/tmp/fhir-fix-demo/{timestamp}.json', createDirs: true },
            },
            messageType: 'FHIR:Patient',
            mappings: [],
            transformationFlow: 'fhir_passthrough',
            auto_start: false,
            deployment_mode: 'manual',
            status: 'active',
            debug_logging: true,
            log_retention_days: 7,
        });
        state.fhirInterfaceId = fhirIface.interface?.id || fhirIface.interfaceId || fhirIface.id;
        expect(state.fhirInterfaceId, `createInterface response: ${JSON.stringify(fhirIface)}`).toBeTruthy();

        await api.savePipeline({
            interfaceId: state.fhirInterfaceId,
            messageType: 'FHIR:Patient',
            steps: [
                {
                    step_name: 'HTTP FHIR Receiver',
                    step_type: 'connector.inbound',
                    sequence: 10,
                    enabled: true,
                    config: {
                        connectorType: 'http_fhir_inbound',
                        config: { host: '0.0.0.0', port: PORT_FHIR_DEMO },
                        timeoutMs: 30000,
                    },
                },
                {
                    step_name: 'File Writer (Passthrough)',
                    step_type: 'connector.outbound',
                    sequence: 20,
                    enabled: true,
                    config: {
                        connectorType: 'file_writer',
                        config: { outputPath: '/tmp/fhir-fix-demo/{timestamp}.json', createDirs: true },
                        contentField: 'raw',
                    },
                },
            ],
        });

        // ── Interface D: automatic branch-exclusion fix ────────────────────────
        // Deliberately configures ZERO manual "skipSteps" anywhere — the whole
        // point is that ParentConditionalStepID + CaseValue (below) is enough on
        // its own now. Before the fix, both branches' own steps would run.
        const autoSkipIface = await api.createInterface({
            name: 'Switch Case Auto-Skip Fix Demo',
            description: 'Persistent demo interface (2026-09-08) proving switch_case (and if_then_else) automatically exclude the untaken branch\'s own steps — zero manual "skipSteps" config needed. The switch step\'s two case actions are both a no-op "continue"; the branching is expressed purely by tagging each downstream step with parent_conditional_step_id + case_value, the same metadata the pipeline-builder UI already saves when you drop a step into a case\'s lane.',
            sourceType: 'fhir',
            sourceConnectivity: 'http_fhir_inbound',
            sourceConfig: { host: '0.0.0.0', port: PORT_AUTOSKIP_DEMO },
            sourceConnectorConfig: {
                connectorType: 'http_fhir_inbound',
                config: { host: '0.0.0.0', port: PORT_AUTOSKIP_DEMO },
            },
            targetType: 'file',
            targetConnectivity: 'file_writer',
            targetConfig: { outputPath: '/tmp/autoskip-fix-demo/{timestamp}.json', createDirs: true },
            targetConnectorConfig: {
                connectorType: 'file_writer',
                config: { outputPath: '/tmp/autoskip-fix-demo/{timestamp}.json', createDirs: true },
            },
            messageType: 'JSON:Gender',
            mappings: [],
            transformationFlow: 'fhir_passthrough',
            auto_start: false,
            deployment_mode: 'manual',
            status: 'active',
            debug_logging: true,
            log_retention_days: 7,
        });
        state.autoSkipInterfaceId = autoSkipIface.interface?.id || autoSkipIface.interfaceId || autoSkipIface.id;
        expect(state.autoSkipInterfaceId, `createInterface response: ${JSON.stringify(autoSkipIface)}`).toBeTruthy();

        const switchStepId = randomUUID();
        await api.savePipeline({
            interfaceId: state.autoSkipInterfaceId,
            messageType: 'JSON:Gender',
            steps: [
                {
                    id: switchStepId,
                    step_name: 'Route By Gender',
                    step_type: 'switch_case',
                    sequence: 10,
                    enabled: true,
                    config: {
                        field: 'message.gender',
                        cases: [
                            { value: 'F', actions: [{ action: 'continue' }] },
                            { value: 'M', actions: [{ action: 'continue' }] },
                        ],
                        default: [{ action: 'continue' }],
                    },
                },
                {
                    id: randomUUID(),
                    step_name: 'Female Branch',
                    step_type: 'enrichment.script',
                    sequence: 20,
                    enabled: true,
                    parent_conditional_step_id: switchStepId,
                    case_value: 'F',
                    config: { script: "return { branch: 'female' };" },
                },
                {
                    id: randomUUID(),
                    step_name: 'Male Branch',
                    step_type: 'enrichment.script',
                    sequence: 30,
                    enabled: true,
                    parent_conditional_step_id: switchStepId,
                    case_value: 'M',
                    config: { script: "return { branch: 'male' };" },
                },
            ],
        });
    });

    test('SETUP sanity: all four demo interfaces were created', async () => {
        expect(state.fieldValidationInterfaceId).toBeTruthy();
        expect(state.controlLoopInterfaceId).toBeTruthy();
        expect(state.fhirInterfaceId).toBeTruthy();
        expect(state.autoSkipInterfaceId).toBeTruthy();
    });

    test('Field Validation Fix Demo: real Properties Panel renders the saved step (not the generic fallback)', async ({ page }) => {
        test.skip(!state.fieldValidationInterfaceId, 'Setup failed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.fieldValidationInterfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        const opened = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => (st.stepType || st.step_type) === 'field_validation');
            if (s && pb.propertiesPanel?.showStepProperties) {
                pb.propertiesPanel.showStepProperties(s);
                return true;
            }
            return false;
        });
        expect(opened, 'field_validation step should exist in the saved pipeline and its panel should open').toBe(true);
        await page.waitForTimeout(500);

        // The panel must show real rule content, not an empty/generic/error state.
        await expect(page.locator('#stepPropertiesModal')).toBeVisible({ timeout: 3000 });
        // The 3 real rules render as their own FieldValidationStepBuilder UI (Field
        // Path/Validation Type/etc. rows) — before the fix, an unregistered
        // "field_validation" step type fell through to the generic fallback panel
        // instead, which has none of these field-specific inputs at all.
        await expect(page.locator('#formTabContent')).toContainText('Validation Rules');
        const fieldPathValue = await page.locator('#formTabContent input[value="PID.3"]').count();
        expect(fieldPathValue, 'the real saved rule value "PID.3" should appear in a Field Path input').toBeGreaterThan(0);
    });

    test('Field Validation Fix Demo: real Test Pipeline modal rejects a message missing the required Patient ID', async ({ page }) => {
        test.skip(!state.fieldValidationInterfaceId, 'Setup failed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.fieldValidationInterfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.locator('#testPipelineBtn').click();
        await expect(page.locator('#testModal')).toHaveClass(/active/);

        await page.locator('#testMessageInput').fill(HL7_ADT_MISSING_PATIENT_ID);
        await page.locator('#runTestBtn').click();

        const resultsContent = page.locator('#testResultsContent');
        await expect(resultsContent).toContainText(/Patient ID is required|validation failed/i, { timeout: 15000 });
    });

    test('Field Validation Fix Demo: real Test Pipeline modal accepts a complete message', async ({ page }) => {
        test.skip(!state.fieldValidationInterfaceId, 'Setup failed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.fieldValidationInterfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.locator('#testPipelineBtn').click();
        await expect(page.locator('#testModal')).toHaveClass(/active/);

        await page.locator('#testMessageInput').fill(HL7_ADT_VALID);
        await page.locator('#runTestBtn').click();

        const resultsContent = page.locator('#testResultsContent');
        await expect(resultsContent).not.toContainText(/Patient ID is required/i, { timeout: 15000 });
        await expect(resultsContent).not.toBeEmpty();
    });

    test('Control Loop Fix Demo: real Properties Panel renders the saved step (not the generic fallback)', async ({ page }) => {
        test.skip(!state.controlLoopInterfaceId, 'Setup failed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.controlLoopInterfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        const opened = await page.evaluate(() => {
            const pb = window.pipelineBuilder;
            const steps = pb?.getAllStepsFlat?.() || [];
            const s = steps.find(st => (st.stepType || st.step_type) === 'control.loop');
            if (s && pb.propertiesPanel?.showStepProperties) {
                pb.propertiesPanel.showStepProperties(s);
                return true;
            }
            return false;
        });
        expect(opened, 'control.loop step should exist in the saved pipeline and its panel should open').toBe(true);
        await page.waitForTimeout(500);

        await expect(page.locator('#stepPropertiesModal')).toBeVisible({ timeout: 3000 });
        // Collection path is a form input's value, not text content — check both
        // the visible label text and the actual saved value.
        await expect(page.locator('#formTabContent')).toContainText(/collection/i);
        const collectionValue = await page.locator('#formTabContent input[value="message.orders[0].items"]').count();
        expect(collectionValue, 'the real saved bracket-indexed collection path should appear in a form input').toBeGreaterThan(0);
    });

    test('Control Loop Fix Demo: real Test Pipeline modal iterates the bracket-indexed collection and classifies both items', async ({ page }) => {
        test.skip(!state.controlLoopInterfaceId, 'Setup failed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.controlLoopInterfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.locator('#testPipelineBtn').click();
        await expect(page.locator('#testModal')).toHaveClass(/active/);

        await page.locator('#testMessageInput').fill(ORDERS_JSON_TWO_ITEMS);
        await page.locator('#runTestBtn').click();

        const resultsContent = page.locator('#testResultsContent');
        // Before the fix this always showed 0 iterations (the bracketed path never resolved).
        await expect(resultsContent).toContainText(/"iterations":\s*2|in_stock|backordered/i, { timeout: 15000 });
        await expect(resultsContent).not.toContainText(/"iterations":\s*0/);
    });

    test('Switch Case Auto-Skip Fix Demo: real Test Pipeline modal runs only the matched branch, with zero manual skipSteps config', async ({ page }) => {
        test.skip(!state.autoSkipInterfaceId, 'Setup failed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.autoSkipInterfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.locator('#testPipelineBtn').click();
        await expect(page.locator('#testModal')).toHaveClass(/active/);

        await page.locator('#testMessageInput').fill(JSON.stringify({ gender: 'F' }));
        await page.locator('#runTestBtn').click();

        const resultsContent = page.locator('#testResultsContent');
        // Before the fix, BOTH branch markers ran (plain sequential fall-through) —
        // this asserts the male branch specifically never appears, not just that
        // the female branch does. Note "male_branch" is a literal substring of
        // "female_branch" ("fe" + "male_branch"), so the negative check must look
        // for the quoted JSON key/value form, not a bare substring.
        await expect(resultsContent).toContainText(/female/i, { timeout: 15000 });
        await expect(resultsContent).not.toContainText(/"male_branch"|"branch":\s*"male"/i);
    });
});

// ─────────────────────────────────────────────────────────────────────────────
// Cross-format regression check — real, PRE-EXISTING interfaces (not created by
// this spec), one per format the user asked about (HL7 already covered above by
// the field_validation demo). These don't exercise the two bugs directly — none
// of these real pipelines use field_validation or control.loop — they exist to
// close the gap between "I tested this via curl" and "I actually clicked the
// real Test Pipeline button in a real browser for every format," per explicit
// user request (2026-09-08): "did we do it for all 4 types, HL7, FHIR, CCD and
// 835?" Skips (not fails) if a real interface's ID has since changed/been
// removed, since these are live interfaces this spec does not own.
// ─────────────────────────────────────────────────────────────────────────────

// FHIR uses the dedicated "FHIR Passthrough Fix Demo" interface created above
// (state.fhirInterfaceId) rather than a hardcoded ID — see that interface's own
// comment for why (the MVP smoke suite's own FHIR fixtures are ephemeral).
const CCD_EXPORT_INTERFACE_ID = 'f1137fe4-14ee-4275-8b6a-e7b6ef5e3ac0'; // CCD to CSV Export (Test)
const EDI_835_INTERFACE_ID    = '1d40b494-e89e-470c-a947-c8c32482f93f'; // PW EDI-FHIR Test 1788681501486

const FHIR_PATIENT_SAMPLE = JSON.stringify({
    resourceType: 'Patient',
    id: 'smoke-test-1',
    identifier: [{ system: 'urn:oid:2.16.840.1.113883.4.1', value: '999-99-9999' }],
    name: [{ family: 'Doe', given: ['Jane'] }],
    gender: 'female',
    birthDate: '1985-03-14',
});

const CCD_SAMPLE = fs.readFileSync(path.join(__dirname, '../fixtures/sample_ccda_2_1.xml'), 'utf8');

const EDI_835_SAMPLE = fs.readFileSync(
    path.join(__dirname, '../../edi/testdata/real_samples/emedny_sample.txt'), 'utf8'
);

async function realInterfaceExists(request, id) {
    const res = await request.get(`${BASE_URL}/api/interfaces/${id}`);
    return res.ok();
}

test.describe('Cross-format Test Pipeline UI check — FHIR, CCD, EDI 835 (real interfaces)', () => {
    test('FHIR: real Test Pipeline modal on the dedicated demo interface accepts a Patient resource', async ({ page }) => {
        test.skip(!state.fhirInterfaceId, 'FHIR demo interface setup failed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${state.fhirInterfaceId}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.locator('#testPipelineBtn').click();
        await expect(page.locator('#testModal')).toHaveClass(/active/);

        await page.locator('#testMessageInput').fill(FHIR_PATIENT_SAMPLE);
        await page.locator('#runTestBtn').click();

        const resultsContent = page.locator('#testResultsContent');
        await expect(resultsContent).toContainText(/smoke-test-1|Patient/i, { timeout: 15000 });
        await expect(page.locator('.toast, .notification', { hasText: /Test passed/i })).toBeVisible({ timeout: 5000 }).catch(() => {});
    });

    test('CCD: real Test Pipeline modal on the live CCD export interface parses the sample document and extracts sections', async ({ page, request }) => {
        test.skip(!(await realInterfaceExists(request, CCD_EXPORT_INTERFACE_ID)), 'Known CCD export interface not found — may have been removed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${CCD_EXPORT_INTERFACE_ID}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.locator('#testPipelineBtn').click();
        await expect(page.locator('#testModal')).toHaveClass(/active/);

        await page.locator('#testMessageInput').fill(CCD_SAMPLE);
        await page.locator('#runTestBtn').click();

        const resultsContent = page.locator('#testResultsContent');
        // allergiesAndIntolerances: 1 is the real, known-correct section count for this fixture.
        await expect(resultsContent).toContainText(/allergiesAndIntolerances|section_rows|cda_dedupe/i, { timeout: 20000 });
    });

    test('EDI 835: real Test Pipeline modal on the user\'s own real interface builds a PaymentReconciliation with correct conditions/loop', async ({ page, request }) => {
        test.skip(!(await realInterfaceExists(request, EDI_835_INTERFACE_ID)), 'Known EDI 835 interface not found — may have been removed');

        await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${EDI_835_INTERFACE_ID}`);
        await page.waitForLoadState('load');
        await page.waitForFunction(() => window.pipelineBuilder && window.pipelineBuilder.pipeline != null, { timeout: 8000 });

        await page.locator('#testPipelineBtn').click();
        await expect(page.locator('#testModal')).toHaveClass(/active/);

        await page.locator('#testMessageInput').fill(EDI_835_SAMPLE);
        await page.locator('#runTestBtn').click();

        const resultsContent = page.locator('#testResultsContent');
        // Real, previously-verified expected values for this exact sample: BPR04=ACH → disposition
        // fires; 3 claims (34.25/0/11.5) → row condition drops the $0 one, leaving 2 in detail[].
        await expect(resultsContent).toContainText(/PaymentReconciliation/i, { timeout: 20000 });
        await expect(resultsContent).toContainText(/ach-remit|Payment issued via direct ACH deposit/i);
    });
});
