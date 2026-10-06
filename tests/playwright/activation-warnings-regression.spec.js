// tests/playwright/activation-warnings-regression.spec.js
//
// Permanent regression guard for a real, platform-wide bug found during a
// 360 QA pass (October 2026): processing/engine.go's ActivateInterface
// deliberately treats one misconfigured connector.inbound step as
// skip-and-continue rather than failing the whole interface activation
// (useful when an interface has several inbound connectors) — but before
// this fix, that trade-off meant the problem was ONLY ever visible in the
// server's own container log file. The HTTP response — and therefore the
// UI's "Activated!" message — reported plain success with zero indication
// any step failed to start.
//
// Reproduced live with a DICOM Storage SCP connector.inbound step missing
// its required ae_title/port fields, but the underlying bug and fix are
// NOT DICOM-specific: CreateInputConnector's own Validate() call is generic
// across every connector type, and so is the fix (engine.go's new
// activationWarnings mechanism, surfaced through processing_controller.go
// and InterfaceLifecycleController.js).
const { test, expect } = require('@playwright/test');
const { ApiHelper } = require('./helpers/api');

test.describe('Interface activation surfaces connector-start warnings (regression guard)', () => {
    test.describe.configure({ retries: 0 });

    test('a connector.inbound step missing required config produces a visible warning, not silent success', async ({ request }) => {
        const api = new ApiHelper(request);

        const createRes = await request.post('/api/interfaces', {
            data: {
                name: `PW ActivationWarning Bad ${Date.now()}`,
                messageType: 'DICOM',
                description: 'activation-warnings regression check',
                sourceType: '', targetType: '', sourceConfig: {}, targetConfig: {},
            },
        });
        const createBody = await createRes.json();
        expect(createBody.success, JSON.stringify(createBody)).toBe(true);
        const interfaceId = createBody.interface.id;

        try {
            const pipelineRes = await request.post('/api/pipelines', {
                data: {
                    interface_id: interfaceId,
                    message_type: 'DICOM',
                    execution_groups: [{
                        sequence: 5,
                        steps: [{
                            step_type: 'connector.inbound',
                            step_name: 'DICOM Storage SCP',
                            step_alias: 'dicom_inbound',
                            config: { connectorType: 'dicom_storage_inbound', config: {} }, // ae_title/port deliberately omitted
                        }],
                    }],
                },
            });
            expect((await pipelineRes.json()).success).toBe(true);

            const activation = await api.activateInterface(interfaceId);
            expect(activation.success, JSON.stringify(activation)).toBe(true);
            expect(Array.isArray(activation.warnings), `expected a warnings array in the activation response, got: ${JSON.stringify(activation)}`).toBe(true);
            expect(activation.warnings.length).toBeGreaterThan(0);
            expect(activation.warnings.some(w => w.includes('ae_title')), `expected a warning mentioning the missing ae_title field, got: ${JSON.stringify(activation.warnings)}`).toBe(true);
        } finally {
            await request.post(`/api/runtime/interfaces/${interfaceId}/deactivate`).catch(() => {});
            await request.delete(`/api/interfaces/${interfaceId}`).catch(() => {});
        }
    });

    test('a fully valid connector.inbound config produces no warnings (no false positives)', async ({ request }) => {
        const api = new ApiHelper(request);
        const port = 19876 + Math.floor(Math.random() * 500); // avoid colliding with any other test's fixed port

        const createRes = await request.post('/api/interfaces', {
            data: {
                name: `PW ActivationWarning Good ${Date.now()}`,
                messageType: 'DICOM',
                description: 'activation-warnings regression check (valid config)',
                sourceType: '', targetType: '', sourceConfig: {}, targetConfig: {},
            },
        });
        const createBody = await createRes.json();
        expect(createBody.success, JSON.stringify(createBody)).toBe(true);
        const interfaceId = createBody.interface.id;

        try {
            const pipelineRes = await request.post('/api/pipelines', {
                data: {
                    interface_id: interfaceId,
                    message_type: 'DICOM',
                    execution_groups: [{
                        sequence: 5,
                        steps: [{
                            step_type: 'connector.inbound',
                            step_name: 'DICOM Storage SCP',
                            step_alias: 'dicom_inbound',
                            config: { connectorType: 'dicom_storage_inbound', config: { ae_title: 'REGRESSGOOD', port } },
                        }],
                    }],
                },
            });
            expect((await pipelineRes.json()).success).toBe(true);

            const activation = await api.activateInterface(interfaceId);
            expect(activation.success, JSON.stringify(activation)).toBe(true);
            expect(activation.warnings, `a fully valid config must not produce any warnings, got: ${JSON.stringify(activation.warnings)}`).toBeUndefined();
        } finally {
            await request.post(`/api/runtime/interfaces/${interfaceId}/deactivate`).catch(() => {});
            await request.delete(`/api/interfaces/${interfaceId}`).catch(() => {});
        }
    });
});
