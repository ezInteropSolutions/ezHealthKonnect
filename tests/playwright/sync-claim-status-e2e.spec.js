// tests/playwright/sync-claim-status-e2e.spec.js
// Full-stack proof of the SYNCHRONOUS 276/277 claim status endpoint
// (POST /api/claim-status/:interfaceId/check). Mirrors
// sync-eligibility-e2e.spec.js exactly -- see that file's own header comment
// for the full rationale (this creates a real interface + pipeline via the
// same REST endpoints the pipeline-builder UI itself calls, then posts a
// real 276 X12 payload straight to the sync endpoint and asserts a real
// document comes back in the SAME HTTP response, proving the controller's
// own resolve-by-(interfaceId,messageType)-then-execute-synchronously
// mechanism, not real claim-adjudication business logic -- that's the
// resolved pipeline's own job).
const { test, expect } = require('@playwright/test');

const SYNC_276_SAMPLE = [
    'ISA*00*          *00*          *ZZ*PROVIDER1      *ZZ*PAYER1         *260914*0900*^*00501*889860170*0*P*:~',
    'GS*HR***260914*0900*889893013*X*005010X212~',
    'ST*276*4522~',
    'BHT*0010*13*CLMSTAT01*20260914*0900~',
    'HL*1**20*1~',
    'NM1*PR*2*PAYER1*****PI*PAYER001~',
    'HL*2*1*21*1~',
    'NM1*41*2*ACME CLEARINGHOUSE*****46*CLR001~',
    'HL*3*2*19*1~',
    'NM1*1P*2*ACME CLINIC*****XX*1234567890~',
    'HL*4*3*22*1~',
    'NM1*IL*1*SMITH*JANE****MI*SUB123~',
    'DMG*D8*19800101*F~',
    'TRN*1*REQTRACE001~',
    'REF*EJ*PCN0001~',
    'AMT*T3*250.00~',
    'DTP*472*D8*20260110~',
    'SVC*HC:99213**250.00~',
    'HL*5*4*23*0~',
    'NM1*QC*1*SMITH*TOMMY****MI*SUB123~',
    'DMG*D8*20100601*M~',
    'TRN*1*REQTRACE002~',
    'REF*EJ*PCN0002~',
    'SVC*HC:90460**75.00~',
    'SE*23*4522~',
    'GE*1*889893013~',
    'IEA*1*889860170~',
].join('\n') + '\n';

async function createSyncClaimStatusInterfaceAndPipeline(request, label) {
    const ifaceRes = await request.post('/api/interfaces', {
        data: {
            name: `Sync Claim Status E2E ${label} ${Date.now()}`,
            messageType: '276',
            description: 'Playwright sync claim status test',
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
            message_type: '276',
            name: 'Sync Claim Status E2E Pipeline',
            execution_groups: [
                {
                    steps: [
                        { step_name: 'Parse 276', step_type: 'edi.parse', sequence: 10, config: {} },
                        {
                            step_name: 'Rebuild 276', step_type: 'edi.build', sequence: 20,
                            config: { sourceField: 'parsedEDI', transactionSet: '276' },
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

test('Sync claim status: POST /api/claim-status/:interfaceId/check returns a real document synchronously through the real pipeline engine', async ({ request }) => {
    const interfaceId = await createSyncClaimStatusInterfaceAndPipeline(request, 'happy');

    const checkRes = await request.post(`/api/claim-status/${interfaceId}/check`, {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: SYNC_276_SAMPLE,
    });
    expect(checkRes.status(), `sync check failed: ${await checkRes.text()}`).toBe(200);
    expect(checkRes.headers()['content-type']).toContain('application/edi-x12');

    const body = await checkRes.text();
    expect(body.startsWith('ISA*'), `response should be a rebuilt ISA...IEA document, got: ${body.slice(0, 60)}`).toBe(true);
    expect(body).toContain('ST*276*');
    expect(body).toContain('NM1*IL*1*SMITH*JANE');
    expect(body).toContain('NM1*QC*1*SMITH*TOMMY');
    expect(body).toContain('IEA*');
});

test('Sync claim status: unknown interface returns 404, not a silent fallback pipeline', async ({ request }) => {
    const res = await request.post('/api/claim-status/00000000-0000-0000-0000-000000000000/check', {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: SYNC_276_SAMPLE,
    });
    expect(res.status()).toBe(404);
});

test('Sync claim status: empty body returns 400', async ({ request }) => {
    const res = await request.post('/api/claim-status/00000000-0000-0000-0000-000000000000/check', {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: '',
    });
    expect(res.status()).toBe(400);
});

test('Sync claim status: malformed EDI content produces a real error status, not a 200', async ({ request }) => {
    const interfaceId = await createSyncClaimStatusInterfaceAndPipeline(request, 'malformed');

    const res = await request.post(`/api/claim-status/${interfaceId}/check`, {
        headers: { 'Content-Type': 'application/edi-x12' },
        data: 'NOT REAL EDI CONTENT',
    });
    expect([422, 502]).toContain(res.status());
});
