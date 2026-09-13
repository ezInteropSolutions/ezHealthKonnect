// tests/playwright/sync-eligibility-e2e.spec.js
// Full-stack proof of the SYNCHRONOUS 270/271 eligibility endpoint
// (POST /api/eligibility/:interfaceId/check). Unlike every other EDI
// Playwright spec in this suite (which drives the pipeline-builder's own
// "Test Pipeline" button), this endpoint is a plain backend API a caller
// hits directly -- so this test creates a real interface + pipeline via the
// SAME REST endpoints the pipeline-builder UI itself calls (POST
// /api/interfaces, POST /api/pipelines -- see public/js/dashboard.js's own
// "Use Template" flow for the exact same two calls), then posts a real 270
// X12 payload straight to the sync endpoint and asserts a real document
// comes back in the SAME HTTP response -- not a 202/"queued" ack the way
// every async connector in this codebase responds.
//
// The test pipeline itself is deliberately minimal (edi.parse -> edi.build,
// re-emitting the SAME transaction set) -- this proves the CONTROLLER's own
// mechanism (resolve a pipeline by interface_id + message_type, execute it
// synchronously, extract the build step's real output, return real bytes
// with a real status code), which is what this endpoint actually is. Real
// eligibility business logic (turning a 270 into a genuine 271 answer) is
// the resolved pipeline's own job, not this endpoint's -- matching the
// engine's own "transforms, never invents business facts" principle.
const { test, expect } = require('@playwright/test');

const SYNC_270_SAMPLE = [
    'ISA*00*          *00*          *ZZ*PROVIDER1      *ZZ*PAYER1         *260912*1421*^*00501*889860169*0*P*:~',
    'GS*HS***260912*1421*889893012*X*005010X279A1~',
    'ST*270*4521~',
    'BHT*0022*13*ELIG0001*20260912*1200~',
    'HL*1**20*1~',
    'NM1*PR*2*PAYER1*****PI*PAYER001~',
    'HL*2*1*21*1~',
    'NM1*1P*2*ACME CLINIC*****XX*1234567890~',
    'HL*3*2*22*0~',
    'TRN*1*TRACE001~',
    'NM1*IL*1*SMITH*JANE****MI*SUB123~',
    'DMG*D8*19800101*F~',
    'EQ*30~',
    'SE*12*4521~',
    'GE*1*889893012~',
    'IEA*1*889860169~',
].join('\n') + '\n';

async function createSyncEligibilityInterfaceAndPipeline(request, label) {
    const ifaceRes = await request.post('/api/interfaces', {
        data: {
            name: `Sync Eligibility E2E ${label} ${Date.now()}`,
            messageType: '270',
            description: 'Playwright sync eligibility test',
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
            message_type: '270',
            name: 'Sync Eligibility E2E Pipeline',
            execution_groups: [
                {
                    steps: [
                        { step_name: 'Parse 270', step_type: 'edi.parse', sequence: 10, config: {} },
                        {
                            step_name: 'Rebuild 270', step_type: 'edi.build', sequence: 20,
                            config: { sourceField: 'parsedEDI', transactionSet: '270' },
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

test('Sync eligibility: POST /api/eligibility/:interfaceId/check returns a real document synchronously through the real pipeline engine', async ({ request }) => {
    const interfaceId = await createSyncEligibilityInterfaceAndPipeline(request, 'happy');

    const checkRes = await request.post(`/api/eligibility/${interfaceId}/check`, {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: SYNC_270_SAMPLE,
    });
    expect(checkRes.status(), `sync check failed: ${await checkRes.text()}`).toBe(200);
    expect(checkRes.headers()['content-type']).toContain('application/edi-x12');

    const body = await checkRes.text();
    expect(body.startsWith('ISA*'), `response should be a rebuilt ISA...IEA document, got: ${body.slice(0, 60)}`).toBe(true);
    expect(body).toContain('ST*270*');
    expect(body).toContain('NM1*IL*1*SMITH*JANE');
    expect(body).toContain('IEA*');
});

test('Sync eligibility: unknown interface returns 404, not a silent fallback pipeline', async ({ request }) => {
    const res = await request.post('/api/eligibility/00000000-0000-0000-0000-000000000000/check', {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: SYNC_270_SAMPLE,
    });
    expect(res.status()).toBe(404);
});

test('Sync eligibility: empty body returns 400', async ({ request }) => {
    const res = await request.post('/api/eligibility/00000000-0000-0000-0000-000000000000/check', {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: '',
    });
    expect(res.status()).toBe(400);
});

test('Sync eligibility: malformed EDI content produces a real error status, not a 200', async ({ request }) => {
    const interfaceId = await createSyncEligibilityInterfaceAndPipeline(request, 'malformed');

    const res = await request.post(`/api/eligibility/${interfaceId}/check`, {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: 'NOT REAL EDI CONTENT',
    });
    expect([422, 502]).toContain(res.status());
});
