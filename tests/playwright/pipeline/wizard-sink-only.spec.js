'use strict';
// A store-only (Passthrough / FHIR receiver) wizard flow has no external destination. The target connector builder
// initialises asynchronously, so the payload can still carry an HTTP default; the interface then got an HTTP outbound
// step with no endpoint and every message failed with "endpoint is required" (seen on the installed-app CI run).
// The server now ignores any target in a sink-only flow.
const { test, expect } = require('../helpers/seeded');

const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';

for (const flow of ['passthrough', 'fhir_receiver']) {
    test(`${flow}: an HTTP target left in the payload is ignored and the interface stores only`, async ({ seed }) => {
        const name = `PW_SINKONLY_${flow}_${Date.now()}`;
        const created = await seed.api.request.post(`${BASE_URL}/api/wizard/complete`, {
            // The body the wizard page itself posts (the `mappings` array selects the wizard's own handler).
            data: {
                wizardData: {
                    name, transformationFlow: flow, messageType: 'ADT^A01', status: 'draft', mappings: [],
                    sourceType: 'hl7v2', sourceConnectivity: 'tcp',
                    sourceConnectorConfig: { connectorType: 'tcp_mllp', config: { host: '0.0.0.0', port: 6639 } },
                    targetType: 'http', targetConnectivity: 'http',
                    targetConnectorConfig: { connectorType: 'http_outbound', config: {} },
                    targetConfig: {},
                },
            },
        });
        expect(created.status(), await created.text()).toBe(200);
        const body = await created.json();
        const id = body.interfaceId || body.data?.interfaceId;
        expect(id, JSON.stringify(body).slice(0, 300)).toBeTruthy();
        try {
            const got = await (await seed.api.request.get(`${BASE_URL}/api/interfaces/${id}`)).json();
            const iface = got.interface || got;
            expect(iface.outboundStepConfig?.connectorType, 'a store-only flow must end in the sink').toBe('sink_outbound');
            expect(iface.targetConnectivity?.type).toBe('sink');
        } finally {
            await seed.api.request.delete(`${BASE_URL}/api/interfaces/${id}`);
        }
    });
}
