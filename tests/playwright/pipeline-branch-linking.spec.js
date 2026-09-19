// tests/playwright/pipeline-branch-linking.spec.js
// Proves the fix for switch_case/if_then_else/control.loop branch-linked
// steps losing their parent/child wiring -- two real, related bugs found
// while building the Universal EDI Receiver template:
//
//   1. savePipeline: a TEMPLATE-authored step has no real id at all ("Use
//      Template" strips every id before saving), so a template could never
//      express "this step belongs to that switch_case's branch" -- there was
//      no stable value to reference a sibling step by. Fixed: a step can now
//      reference a parent by that parent's own step_alias string, resolved
//      to the real generated id at save time.
//   2. clonePipeline: gave every cloned step a fresh, UNRELATED uuid while
//      copying parent_conditional_step_id verbatim from the source pipeline
//      -- always a dangling reference. Fixed: old-id -> new-id remapping via
//      the same resolver, plus a topological sort so a branch child is never
//      inserted before its own conditional parent. A SEPARATE, unconditional
//      bug was found alongside it (transformation_pipelines' own
//      UNIQUE(interface_id, message_type) constraint made every clone call
//      fail regardless of branching, since the endpoint always reused the
//      source's own interface_id/message_type) -- fixed by accepting an
//      optional interface_id override, the only way a clone can avoid
//      colliding with its own source.
//
// API-only (no page navigation needed) -- same request.post/get pattern
// already used in tests/playwright/03-interfaces.spec.js's own TC-IFACE-019.
const { test, expect } = require('@playwright/test');

// Two real, pre-existing pipelines in the DB (not fixtures -- created by
// earlier, unrelated QA work) that already use real switch_case branch
// children (Female Branch / Male Branch). Used here specifically because
// they are REAL data demonstrating the clone bug, not a synthetic case that
// might accidentally avoid it.
const REAL_BRANCHED_PIPELINE_IDS = [
    '16e9db5f-4063-479c-b868-6c1e933f69e2',
    'cec56cd2-8615-4ace-8c20-d398459d1bfb',
];

test.describe('Pipeline branch-linked step reference resolution', () => {
    test('savePipeline: a template-shaped payload (no step ids) can express branch membership via step_alias, resolved to the real generated id', async ({ request }) => {
        const unique = Date.now();

        const ifaceRes = await request.post('/api/interfaces', {
            data: {
                name: `PW Branch-Alias Test ${unique}`,
                messageType: 'JSON:BranchAliasTest',
                description: 'pipeline-branch-linking.spec.js fixture',
                sourceType: '', targetType: '', sourceConfig: {}, targetConfig: {},
            },
        });
        const ifaceBody = await ifaceRes.json();
        expect(ifaceBody.success, `failed to create fixture interface: ${JSON.stringify(ifaceBody)}`).toBe(true);
        const interfaceId = ifaceBody.interface?.id || ifaceBody.id;

        // Shaped exactly like what dashboard.js sends after stripping every
        // step's own id for a template-sourced save: the switch_case step
        // carries a step_alias but no id; both children reference that SAME
        // alias string (not a UUID, which can't exist yet) as their
        // parent_conditional_step_id.
        const saveRes = await request.post('/api/pipelines', {
            data: {
                interface_id: interfaceId,
                message_type: 'JSON:BranchAliasTest',
                name: `Branch Alias Test Pipeline ${unique}`,
                execution_groups: [
                    {
                        sequence: 10,
                        steps: [
                            {
                                step_name: 'Gender Switch',
                                step_alias: 'gender_switch',
                                step_type: 'switch_case',
                                sequence: 10,
                                config: {
                                    field: 'message.gender',
                                    cases: [
                                        { value: 'F', actions: [{ action: 'continue' }] },
                                        { value: 'M', actions: [{ action: 'continue' }] },
                                    ],
                                    default: [{ action: 'continue' }],
                                },
                                enabled: true, required: true,
                            },
                            {
                                step_name: 'Female Branch Step',
                                step_type: 'enrichment.script',
                                sequence: 20,
                                parent_conditional_step_id: 'gender_switch',
                                case_value: 'F',
                                config: { script: 'return input;' },
                                enabled: true, required: true,
                            },
                            {
                                step_name: 'Male Branch Step',
                                step_type: 'enrichment.script',
                                sequence: 21,
                                parent_conditional_step_id: 'gender_switch',
                                case_value: 'M',
                                config: { script: 'return input;' },
                                enabled: true, required: true,
                            },
                        ],
                    },
                ],
            },
        });
        const saveBody = await saveRes.json();
        expect(saveBody.success, `pipeline save should succeed: ${JSON.stringify(saveBody)}`).toBe(true);
        expect(saveBody.steps_saved, 'all 3 steps should be saved').toBe(3);

        const pipelineId = saveBody.pipeline.id;
        const loadRes = await request.get(`/api/pipelines/${pipelineId}`);
        const loadBody = await loadRes.json();
        expect(loadBody.success).toBe(true);

        const steps = loadBody.pipeline.execution_groups[0].steps;
        const switchStep = steps.find(s => s.step_name === 'Gender Switch');
        const femaleStep = steps.find(s => s.step_name === 'Female Branch Step');
        const maleStep = steps.find(s => s.step_name === 'Male Branch Step');

        expect(switchStep.id, 'switch_case step should have a real UUID, not the string "gender_switch"').toMatch(/^[0-9a-f-]{36}$/);

        expect(femaleStep.parent_conditional_step_id, 'Female Branch\'s parent_conditional_step_id should resolve to the switch_case step\'s REAL id, not stay as the raw alias string').toBe(switchStep.id);
        expect(maleStep.parent_conditional_step_id, 'Male Branch\'s parent_conditional_step_id should resolve to the switch_case step\'s REAL id').toBe(switchStep.id);
        expect(femaleStep.case_value).toBe('F');
        expect(maleStep.case_value).toBe('M');

        // Cleanup
        await request.delete(`/api/interfaces/${interfaceId}`).catch(() => {});
    });

    for (const sourceId of REAL_BRANCHED_PIPELINE_IDS) {
        test(`clonePipeline: real branch-linked pipeline ${sourceId} clones into a new interface with correctly re-linked branch children (not dangling references to the source)`, async ({ request }) => {
            const unique = Date.now();

            // Load the source pipeline to know its own message_type (the
            // clone target must share it -- the whole point of the fix under
            // test is proving branch linkage survives a real clone, not
            // exercising an arbitrary message_type).
            const srcRes = await request.get(`/api/pipelines/${sourceId}`);
            const srcBody = await srcRes.json();
            expect(srcBody.success, `source pipeline ${sourceId} should still exist: ${JSON.stringify(srcBody)}`).toBe(true);
            const srcSteps = srcBody.pipeline.execution_groups[0].steps;
            const srcBranchChildren = srcSteps.filter(s => s.parent_conditional_step_id);
            expect(srcBranchChildren.length, 'source pipeline should have real branch-linked children (Female/Male Branch)').toBeGreaterThanOrEqual(2);

            // A fresh interface to clone INTO -- cloning onto the source's
            // own interface_id+message_type is guaranteed to collide with
            // the constraint (that's the separate bug this same fix closes:
            // the endpoint now returns a clear 409 for that case instead of
            // a raw Postgres error, but a genuinely NEW target is what
            // proves the endpoint can succeed at all).
            const ifaceRes = await request.post('/api/interfaces', {
                data: {
                    name: `PW Clone Target ${unique}`,
                    messageType: srcBody.pipeline.message_type,
                    description: 'pipeline-branch-linking.spec.js clone-target fixture',
                    sourceType: '', targetType: '', sourceConfig: {}, targetConfig: {},
                },
            });
            const ifaceBody = await ifaceRes.json();
            expect(ifaceBody.success, `failed to create clone-target interface: ${JSON.stringify(ifaceBody)}`).toBe(true);
            const targetInterfaceId = ifaceBody.interface?.id || ifaceBody.id;

            const cloneRes = await request.post(`/api/pipelines/${sourceId}/clone`, {
                data: { new_name: `Clone of ${sourceId} ${unique}`, interface_id: targetInterfaceId },
            });
            const cloneBody = await cloneRes.json();
            expect(cloneBody.success, `clone should succeed: ${JSON.stringify(cloneBody)}`).toBe(true);
            const clonedPipelineId = cloneBody.pipeline_id;

            const cloneLoadRes = await request.get(`/api/pipelines/${clonedPipelineId}`);
            const cloneLoadBody = await cloneLoadRes.json();
            expect(cloneLoadBody.success).toBe(true);
            const clonedSteps = cloneLoadBody.pipeline.execution_groups[0].steps;
            const clonedStepIds = new Set(clonedSteps.map(s => s.id));

            const clonedBranchChildren = clonedSteps.filter(s => s.parent_conditional_step_id);
            expect(clonedBranchChildren.length, 'the clone should carry the same number of branch-linked children as the source').toBe(srcBranchChildren.length);

            for (const child of clonedBranchChildren) {
                // The load-bearing assertion: the child's parent reference
                // must point at ANOTHER STEP THAT ACTUALLY EXISTS IN THIS
                // SAME CLONED PIPELINE -- not a dangling reference back to
                // the source pipeline's own (different) step ids.
                expect(clonedStepIds.has(child.parent_conditional_step_id), `cloned step "${child.step_name}"'s parent_conditional_step_id (${child.parent_conditional_step_id}) should reference a step within the SAME cloned pipeline, not a dangling reference to the source pipeline`).toBe(true);

                // And it must genuinely be a NEW id, not accidentally still
                // equal to the id it had in the source pipeline.
                const srcCounterpart = srcSteps.find(s => s.step_name === child.step_name);
                expect(child.id, `cloned step "${child.step_name}" should have gotten a fresh id distinct from its source counterpart`).not.toBe(srcCounterpart.id);
                expect(child.parent_conditional_step_id, `cloned step "${child.step_name}"'s parent reference should be a NEW id, not the source pipeline's own parent id`).not.toBe(srcCounterpart.parent_conditional_step_id);
            }

            // Cleanup: soft-delete the throwaway target interface (cascades
            // to the cloned pipeline's own steps via FK ON DELETE CASCADE --
            // the interface row itself is the only thing this test created
            // that needs explicit cleanup).
            await request.delete(`/api/interfaces/${targetInterfaceId}`).catch(() => {});
        });
    }

    test('clonePipeline: cloning onto the SAME interface+message_type as the source returns a clear 409, not a raw 500', async ({ request }) => {
        const sourceId = REAL_BRANCHED_PIPELINE_IDS[0];
        const cloneRes = await request.post(`/api/pipelines/${sourceId}/clone`, {
            data: { new_name: 'Should collide' },
        });
        expect(cloneRes.status(), 'cloning onto the source\'s own interface+message_type should be rejected, not attempted').toBe(409);
        const body = await cloneRes.json();
        expect(body.success).toBe(false);
        expect(body.error, 'error message should explain the collision and name the fix (pass a different interface_id)').toMatch(/interface_id/i);
    });

    test('clonePipeline: a message_type override alone (same interface, no interface_id override) is also sufficient to avoid the collision', async ({ request }) => {
        const unique = Date.now();
        const sourceId = REAL_BRANCHED_PIPELINE_IDS[0];

        const srcRes = await request.get(`/api/pipelines/${sourceId}`);
        const srcBody = await srcRes.json();
        expect(srcBody.success).toBe(true);
        const sourceInterfaceId = srcBody.pipeline.interface_id;

        const newMessageType = `JSON:BranchCloneTest-${unique}`;
        const cloneRes = await request.post(`/api/pipelines/${sourceId}/clone`, {
            data: { new_name: `Clone onto same interface ${unique}`, message_type: newMessageType },
        });
        const cloneBody = await cloneRes.json();
        expect(cloneBody.success, `clone with only a message_type override should succeed: ${JSON.stringify(cloneBody)}`).toBe(true);

        const cloneLoadRes = await request.get(`/api/pipelines/${cloneBody.pipeline_id}`);
        const cloneLoadBody = await cloneLoadRes.json();
        expect(cloneLoadBody.pipeline.interface_id, 'clone should stay on the SAME interface').toBe(sourceInterfaceId);
        expect(cloneLoadBody.pipeline.message_type, 'clone should carry the NEW message_type').toBe(newMessageType);

        // Cleanup: no new interface was created here, just delete the extra
        // pipeline row + its steps directly (FK ON DELETE CASCADE covers the
        // steps once the pipeline row itself is removed).
        await request.fetch(`/api/pipelines/${cloneBody.pipeline_id}`, { method: 'DELETE' }).catch(() => {});
    });
});
