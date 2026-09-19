// tests/playwright/sync-pharmacy-claim-e2e.spec.js
// Full-stack proof of the SYNCHRONOUS NCPDP Telecommunication D.0 B1 pharmacy
// claim endpoint (POST /api/pharmacy-claim/:interfaceId/submit). Mirrors
// sync-claim-status-e2e.spec.js exactly -- see that file's own header
// comment for the full rationale (this creates a real interface + pipeline
// via the same REST endpoints the pipeline-builder UI itself calls, then
// posts a real B1 request transmission straight to the sync endpoint and
// asserts a real document comes back in the SAME HTTP response, proving the
// controller's own resolve-by-(interfaceId,messageType)-then-execute-
// synchronously mechanism, not real claim-adjudication business logic --
// that's the resolved pipeline's own job).
const { test, expect } = require('@playwright/test');

// Same self-authored fixture construction as
// services/ncpdp_telecom_b1_request_fhir_builder_test.go's own
// buildSelfAuthoredB1Request() -- built with String.fromCharCode rather than
// literal control characters in the file, so the RS/FS bytes survive
// untouched through editors/diffs/JSON transport.
function buildSyncB1RequestSample() {
    const rs = String.fromCharCode(0x1e);
    const fs = String.fromCharCode(0x1c);
    const header = '999999' + 'D0' + 'B1' +
        '          ' + // processorControlNumber
        '1' + '01' +
        '1111111111     ' + // serviceProviderId (15)
        '20260919' +
        '          '; // software (10)
    const body = rs + fs + 'AM01' + fs + 'CBSMITH' + fs + 'CAJOHN' + fs + 'C419800101' + fs + 'C52' +
        rs + fs + 'AM02' + fs + 'EY01' + fs + 'E91234567890' +
        rs + fs + 'AM04' + fs + 'C2123456789012' +
        rs + fs + 'AM07' + fs + 'D2000000123456' + fs + 'E103' + fs + 'D700003089421' + fs + 'E70000030000' + fs + 'D301' + fs + 'D5030' + fs + 'DE20220924' +
        rs + fs + 'AM11' + fs + 'D90000057A' + fs + 'DC0000027E' + fs + 'DX0000016B' + fs + 'DQ00000000' + fs + 'DU0000084F';
    return header + body;
}

const SYNC_B1_REQUEST_SAMPLE = buildSyncB1RequestSample();

async function createSyncPharmacyClaimInterfaceAndPipeline(request, label) {
    const ifaceRes = await request.post('/api/interfaces', {
        data: {
            name: `Sync Pharmacy Claim E2E ${label} ${Date.now()}`,
            messageType: 'B1_REQUEST',
            description: 'Playwright sync pharmacy claim test',
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
            message_type: 'B1_REQUEST',
            name: 'Sync Pharmacy Claim E2E Pipeline',
            execution_groups: [
                {
                    steps: [
                        { step_name: 'Parse B1 Request', step_type: 'ncpdptelecom.parse', sequence: 10, config: { direction: 'request' } },
                        {
                            step_name: 'Rebuild B1 Request', step_type: 'ncpdptelecom.build', sequence: 20,
                            config: { sourceField: 'parsedTelecom', transactionCode: 'B1', direction: 'request' },
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

test('Sync pharmacy claim: POST /api/pharmacy-claim/:interfaceId/submit returns a real document synchronously through the real pipeline engine', async ({ request }) => {
    const interfaceId = await createSyncPharmacyClaimInterfaceAndPipeline(request, 'happy');

    const submitRes = await request.post(`/api/pharmacy-claim/${interfaceId}/submit`, {
        headers: { 'Content-Type': 'application/x-ncpdp-telecom-d0' },
        data: SYNC_B1_REQUEST_SAMPLE,
    });
    expect(submitRes.status(), `sync submit failed: ${await submitRes.text()}`).toBe(200);
    expect(submitRes.headers()['content-type']).toContain('application/x-ncpdp-telecom-d0');

    const body = await submitRes.text();
    expect(body.startsWith('999999D0B1'), `response should be a rebuilt 56-byte header (bin=999999, version=D0, transactionCode=B1), got: ${body.slice(0, 20)}`).toBe(true);
    expect(body).toContain('CBSMITH');
    expect(body).toContain('D2000000123456');
});

test('Sync pharmacy claim: unknown interface returns 404, not a silent fallback pipeline', async ({ request }) => {
    const res = await request.post('/api/pharmacy-claim/00000000-0000-0000-0000-000000000000/submit', {
        headers: { 'Content-Type': 'application/x-ncpdp-telecom-d0' },
        data: SYNC_B1_REQUEST_SAMPLE,
    });
    expect(res.status()).toBe(404);
});

test('Sync pharmacy claim: empty body returns 400', async ({ request }) => {
    const res = await request.post('/api/pharmacy-claim/00000000-0000-0000-0000-000000000000/submit', {
        headers: { 'Content-Type': 'application/x-ncpdp-telecom-d0' },
        data: '',
    });
    expect(res.status()).toBe(400);
});

test('Sync pharmacy claim: malformed content produces a real error status, not a 200', async ({ request }) => {
    const interfaceId = await createSyncPharmacyClaimInterfaceAndPipeline(request, 'malformed');

    const res = await request.post(`/api/pharmacy-claim/${interfaceId}/submit`, {
        headers: { 'Content-Type': 'application/x-ncpdp-telecom-d0' },
        data: 'NOT REAL NCPDP TELECOM CONTENT',
    });
    expect([422, 502]).toContain(res.status());
});
