// tests/playwright/sync-prior-auth-e2e.spec.js
// Full-stack proof of the SYNCHRONOUS 278 prior authorization endpoint
// (POST /api/prior-auth/:interfaceId/check). Mirrors
// sync-claim-status-e2e.spec.js exactly -- see that file's own header
// comment for the full rationale (this creates a real interface + pipeline
// via the same REST endpoints the pipeline-builder UI itself calls, then
// posts a real 278 X12 payload straight to the sync endpoint and asserts a
// real document comes back in the SAME HTTP response, proving the
// controller's own resolve-by-(interfaceId,messageType)-then-execute-
// synchronously mechanism, not real prior-authorization decision logic --
// that's the resolved pipeline's own job).
const { test, expect } = require('@playwright/test');

const SYNC_278_SAMPLE = [
    'ISA*00*          *00*          *ZZ*ACMECLINIC     *ZZ*ACMEUMO        *260914*0900*^*00501*889860180*0*P*:~',
    'GS*HI***260914*0900*889893020*X*005010X217~',
    'ST*278*4530~',
    'BHT*0078*13*PA0001*20260914*0900~',
    'HL*1**20*1~',
    'NM1*X3*2*ACME UMO*****PI*UMO001~',
    'HL*2*1*21*1~',
    'NM1*1P*2*ACME CLINIC*****XX*1234567890~',
    'HL*3*2*22*1~',
    'NM1*IL*1*SMITH*JANE****MI*SUB123~',
    'HL*4*3*EV*0~',
    'TRN*1*EVENTTRACE001~',
    'UM*HS*I*1~',
    'SE*13*4530~',
    'GE*1*889893020~',
    'IEA*1*889860180~',
].join('\n') + '\n';

async function createSyncPriorAuthInterfaceAndPipeline(request, label) {
    const ifaceRes = await request.post('/api/interfaces', {
        data: {
            name: `Sync Prior Auth E2E ${label} ${Date.now()}`,
            messageType: '278',
            description: 'Playwright sync prior auth test',
            sourceType: '',
            targetType: '',
            sourceConfig: {},
            targetConfig: {},
        },
    });
    expect(ifaceRes.ok(), `interface create failed: ${await ifaceRes.text()}`).toBeTruthy();
    const ifaceData = await ifaceRes.json();
    expect(ifaceData.success, JSON.stringify(ifaceData)).toBeTruthy();
    const interfaceId = ifaceData.interface?.id || ifaceData.id;
    expect(interfaceId).toBeTruthy();

    const pipelineRes = await request.post('/api/pipelines', {
        data: {
            interface_id: interfaceId,
            message_type: '278',
            name: 'Sync Prior Auth E2E Pipeline',
            execution_groups: [
                {
                    steps: [
                        { step_name: 'Parse 278', step_type: 'edi.parse', sequence: 10, config: {} },
                        {
                            step_name: 'Rebuild 278', step_type: 'edi.build', sequence: 20,
                            config: { sourceField: 'parsedEDI', transactionSet: '278' },
                        },
                    ],
                },
            ],
            pipeline_config: {},
        },
    });
    expect(pipelineRes.ok(), `pipeline save failed: ${await pipelineRes.text()}`).toBeTruthy();
    const pipelineData = await pipelineRes.json();
    expect(pipelineData.success, JSON.stringify(pipelineData)).toBeTruthy();

    return interfaceId;
}

test('Sync prior auth: POST /api/prior-auth/:interfaceId/check returns a real document synchronously through the real pipeline engine', async ({ request }) => {
    const interfaceId = await createSyncPriorAuthInterfaceAndPipeline(request, 'happy');

    const checkRes = await request.post(`/api/prior-auth/${interfaceId}/check`, {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: SYNC_278_SAMPLE,
    });
    expect(checkRes.status(), `sync check failed: ${await checkRes.text()}`).toBe(200);
    expect(checkRes.headers()['content-type']).toContain('application/edi-x12');

    const body = await checkRes.text();
    expect(body.startsWith('ISA*'), `response should be a rebuilt ISA...IEA document, got: ${body.slice(0, 60)}`).toBe(true);
    expect(body).toContain('ST*278*');
    expect(body).toContain('NM1*IL*1*SMITH*JANE');
    expect(body).toContain('IEA*');
});

test('Sync prior auth: unknown interface returns 404, not a silent fallback pipeline', async ({ request }) => {
    const res = await request.post('/api/prior-auth/00000000-0000-0000-0000-000000000000/check', {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: SYNC_278_SAMPLE,
    });
    expect(res.status()).toBe(404);
});

test('Sync prior auth: empty body returns 400', async ({ request }) => {
    const res = await request.post('/api/prior-auth/00000000-0000-0000-0000-000000000000/check', {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: '',
    });
    expect(res.status()).toBe(400);
});

test('Sync prior auth: malformed EDI content produces a real error status, not a 200', async ({ request }) => {
    const interfaceId = await createSyncPriorAuthInterfaceAndPipeline(request, 'malformed');

    const res = await request.post(`/api/prior-auth/${interfaceId}/check`, {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: 'NOT REAL EDI CONTENT',
    });
    expect([422, 502]).toContain(res.status());
});
