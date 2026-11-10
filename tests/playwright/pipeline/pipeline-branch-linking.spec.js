// tests/playwright/pipeline/pipeline-branch-linking.spec.js
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
// Every test creates its own branch-linked source pipeline (an earlier
// version cloned two pipeline ids that only existed in one developer's
// database, so it failed on any fresh database, including CI's).
//
// API-only (no page navigation needed) -- same request.post/get pattern
// already used in tests/playwright/core/03-interfaces.spec.js's own TC-IFACE-019.
const { test, expect } = require('@playwright/test');

async function createInterface(request, name, messageType) {
    const res = await request.post('/api/interfaces', {
        data: {
            name,
            messageType,
            description: 'pipeline-branch-linking.spec.js fixture',
            sourceType: '', targetType: '', sourceConfig: {}, targetConfig: {},
        },
    });
    const body = await res.json();
    expect(body.success, `failed to create fixture interface: ${JSON.stringify(body)}`).toBe(true);
    return body.interface?.id || body.id;
}

/**
 * Creates an interface plus a pipeline whose two enrichment steps are
 * switch_case branch children. The payload is shaped exactly like what
 * dashboard.js sends for a template-sourced save: no step ids, and the
 * children reference the switch step by its step_alias.
 */
async function createBranchedPipeline(request, label) {
    const unique = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
    const messageType = `JSON:BranchLinkTest-${unique}`;
    const interfaceId = await createInterface(request, `PW ${label} ${unique}`, messageType);

    const saveRes = await request.post('/api/pipelines', {
        data: {
            interface_id: interfaceId,
            message_type: messageType,
            name: `${label} Pipeline ${unique}`,
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
    return { interfaceId, pipelineId: saveBody.pipeline.id, messageType, saveBody };
}

async function loadSteps(request, pipelineId) {
    const res = await request.get(`/api/pipelines/${pipelineId}`);
    const body = await res.json();
    expect(body.success, `pipeline ${pipelineId} should load: ${JSON.stringify(body)}`).toBe(true);
    return { pipeline: body.pipeline, steps: body.pipeline.execution_groups[0].steps };
}

test.describe('Pipeline branch-linked step reference resolution', () => {
    test('savePipeline: a template-shaped payload (no step ids) can express branch membership via step_alias, resolved to the real generated id', async ({ request }) => {
        const { interfaceId, pipelineId } = await createBranchedPipeline(request, 'Branch-Alias Test');
        try {
            const { steps } = await loadSteps(request, pipelineId);
            const switchStep = steps.find(s => s.step_name === 'Gender Switch');
            const femaleStep = steps.find(s => s.step_name === 'Female Branch Step');
            const maleStep = steps.find(s => s.step_name === 'Male Branch Step');

            expect(switchStep.id, 'switch_case step should have a real UUID, not the string "gender_switch"').toMatch(/^[0-9a-f-]{36}$/);
            expect(femaleStep.parent_conditional_step_id, 'Female Branch\'s parent_conditional_step_id should resolve to the switch_case step\'s REAL id, not stay as the raw alias string').toBe(switchStep.id);
            expect(maleStep.parent_conditional_step_id, 'Male Branch\'s parent_conditional_step_id should resolve to the switch_case step\'s REAL id').toBe(switchStep.id);
            expect(femaleStep.case_value).toBe('F');
            expect(maleStep.case_value).toBe('M');
        } finally {
            await request.delete(`/api/interfaces/${interfaceId}`).catch(() => {});
        }
    });

    test('clonePipeline: a branch-linked pipeline clones into a new interface with correctly re-linked branch children (not dangling references to the source)', async ({ request }) => {
        const source = await createBranchedPipeline(request, 'Clone Source');
        let targetInterfaceId;
        try {
            const { steps: srcSteps } = await loadSteps(request, source.pipelineId);
            const srcBranchChildren = srcSteps.filter(s => s.parent_conditional_step_id);
            expect(srcBranchChildren.length, 'source pipeline should have real branch-linked children (Female/Male Branch)').toBeGreaterThanOrEqual(2);

            // A fresh interface to clone INTO -- cloning onto the source's own
            // interface_id+message_type collides with the unique constraint
            // (covered by the 409 test below).
            targetInterfaceId = await createInterface(request, `PW Clone Target ${Date.now()}`, source.messageType);

            const cloneRes = await request.post(`/api/pipelines/${source.pipelineId}/clone`, {
                data: { new_name: `Clone of ${source.pipelineId}`, interface_id: targetInterfaceId },
            });
            const cloneBody = await cloneRes.json();
            expect(cloneBody.success, `clone should succeed: ${JSON.stringify(cloneBody)}`).toBe(true);

            const { steps: clonedSteps } = await loadSteps(request, cloneBody.pipeline_id);
            const clonedStepIds = new Set(clonedSteps.map(s => s.id));
            const clonedBranchChildren = clonedSteps.filter(s => s.parent_conditional_step_id);
            expect(clonedBranchChildren.length, 'the clone should carry the same number of branch-linked children as the source').toBe(srcBranchChildren.length);

            for (const child of clonedBranchChildren) {
                // The load-bearing assertion: the child's parent reference must
                // point at a step in THIS cloned pipeline, not a dangling
                // reference back to the source pipeline's own step ids.
                expect(clonedStepIds.has(child.parent_conditional_step_id), `cloned step "${child.step_name}"'s parent_conditional_step_id (${child.parent_conditional_step_id}) should reference a step within the SAME cloned pipeline, not a dangling reference to the source pipeline`).toBe(true);

                const srcCounterpart = srcSteps.find(s => s.step_name === child.step_name);
                expect(child.id, `cloned step "${child.step_name}" should have gotten a fresh id distinct from its source counterpart`).not.toBe(srcCounterpart.id);
                expect(child.parent_conditional_step_id, `cloned step "${child.step_name}"'s parent reference should be a NEW id, not the source pipeline's own parent id`).not.toBe(srcCounterpart.parent_conditional_step_id);
            }
        } finally {
            // Deleting an interface cascades to its pipeline and steps.
            if (targetInterfaceId) await request.delete(`/api/interfaces/${targetInterfaceId}`).catch(() => {});
            await request.delete(`/api/interfaces/${source.interfaceId}`).catch(() => {});
        }
    });

    test('clonePipeline: cloning onto the SAME interface+message_type as the source returns a clear 409, not a raw 500', async ({ request }) => {
        const source = await createBranchedPipeline(request, 'Clone Collision');
        try {
            const cloneRes = await request.post(`/api/pipelines/${source.pipelineId}/clone`, {
                data: { new_name: 'Should collide' },
            });
            expect(cloneRes.status(), 'cloning onto the source\'s own interface+message_type should be rejected, not attempted').toBe(409);
            const body = await cloneRes.json();
            expect(body.success).toBe(false);
            expect(body.error, 'error message should explain the collision and name the fix (pass a different interface_id)').toMatch(/interface_id/i);
        } finally {
            await request.delete(`/api/interfaces/${source.interfaceId}`).catch(() => {});
        }
    });

    test('clonePipeline: a message_type override alone (same interface, no interface_id override) is also sufficient to avoid the collision', async ({ request }) => {
        const source = await createBranchedPipeline(request, 'Clone MessageType');
        try {
            const newMessageType = `${source.messageType}-clone`;
            const cloneRes = await request.post(`/api/pipelines/${source.pipelineId}/clone`, {
                data: { new_name: 'Clone onto same interface', message_type: newMessageType },
            });
            const cloneBody = await cloneRes.json();
            expect(cloneBody.success, `clone with only a message_type override should succeed: ${JSON.stringify(cloneBody)}`).toBe(true);

            const { pipeline } = await loadSteps(request, cloneBody.pipeline_id);
            expect(pipeline.interface_id, 'clone should stay on the SAME interface').toBe(source.interfaceId);
            expect(pipeline.message_type, 'clone should carry the NEW message_type').toBe(newMessageType);
        } finally {
            // Both pipelines live on the source interface; deleting it cascades.
            await request.delete(`/api/interfaces/${source.interfaceId}`).catch(() => {});
        }
    });
});
