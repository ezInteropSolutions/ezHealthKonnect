// tests/unit/controllers/FhirReceiverController.test.js
//
// Regression coverage for the real bug found while investigating Node.js
// testability: FhirReceiverController.js destructured a `getDatabase`
// function from config/database.js that was never exported (module.exports
// is the DatabaseManager instance itself), and both call sites used a raw
// `pg`-pool calling convention (`db.query(sql, [array])`, `.rows[0]`) that
// Sequelize's actual `.query()` never produces. POST /fhir/receiver/:id
// threw TypeError on every single call before the fix in this file.

jest.mock('../../../config/database', () => ({
    sequelize: {
        query: jest.fn(),
        QueryTypes: { SELECT: 'SELECT', INSERT: 'INSERT' },
    },
}));

// storeInMongoDB is called fire-and-forget from receiveResource and is
// already non-fatal by design (wrapped in its own try/catch, and the caller
// also .catch()s it) — mocked here just to keep test output clean, not
// because it's part of what this file is regression-testing.
jest.mock('../../../services/MongoDBConnectionService', () => ({
    getInstance: jest.fn().mockResolvedValue(null),
}));

const database = require('../../../config/database');
const controller = require('../../../controllers/FhirReceiverController');

function mockRes() {
    const res = {};
    res.status = jest.fn().mockReturnValue(res);
    res.json = jest.fn().mockReturnValue(res);
    return res;
}

describe('FhirReceiverController.getInterfaceById', () => {
    beforeEach(() => {
        database.sequelize.query.mockReset();
    });

    it('queries with a named replacement and SELECT type, and returns the row directly (no .rows wrapper)', async () => {
        const fakeInterface = { id: 'iface-1', status: 'active', source_connectivity: { type: 'fhir' } };
        database.sequelize.query.mockResolvedValue([fakeInterface]);

        const result = await controller.getInterfaceById('iface-1');

        expect(database.sequelize.query).toHaveBeenCalledWith(
            expect.stringContaining('WHERE id = :interfaceId'),
            expect.objectContaining({
                replacements: { interfaceId: 'iface-1' },
                type: 'SELECT',
            })
        );
        expect(result).toEqual(fakeInterface);
    });

    it('returns null (not a thrown TypeError) when the query fails', async () => {
        database.sequelize.query.mockRejectedValue(new Error('connection reset'));
        const result = await controller.getInterfaceById('iface-1');
        expect(result).toBeNull();
    });

    it('returns null when no interface matches', async () => {
        database.sequelize.query.mockResolvedValue([]);
        const result = await controller.getInterfaceById('missing-id');
        expect(result).toBeNull();
    });
});

describe('FhirReceiverController.receiveResource', () => {
    beforeEach(() => {
        database.sequelize.query.mockReset();
    });

    it('stores the message via a named-replacement INSERT and responds 201 — regression test for the getDatabase-is-not-a-function bug', async () => {
        const fakeInterface = {
            id: 'iface-1',
            status: 'active',
            source_connectivity: { type: 'fhir_http', authentication: { enabled: false } },
        };
        // First call: getInterfaceById's SELECT. Second call: the metadata INSERT.
        database.sequelize.query
            .mockResolvedValueOnce([fakeInterface])
            .mockResolvedValueOnce([]);

        const req = {
            params: { interfaceId: 'iface-1' },
            body: { resourceType: 'Patient', id: 'pat-1' },
            path: '/fhir/receiver/iface-1',
            ip: '127.0.0.1',
            connection: {},
        };
        const res = mockRes();

        await controller.receiveResource(req, res);

        expect(res.status).toHaveBeenCalledWith(201);
        expect(res.json).toHaveBeenCalledWith(
            expect.objectContaining({ resourceType: 'OperationOutcome' })
        );

        // The INSERT call (second query call) must use named replacements
        // into a dynamically-named per-interface table, matching
        // InterfaceTableManager.js's established convention — not the raw
        // positional-$1 style the code used before this fix.
        const insertCall = database.sequelize.query.mock.calls[1];
        expect(insertCall[0]).toContain('messages_intf_iface_1');
        expect(insertCall[0]).toContain(':messageId');
        expect(insertCall[1]).toEqual(
            expect.objectContaining({
                type: 'INSERT',
                replacements: expect.objectContaining({
                    interfaceId: 'iface-1',
                    status: 'received',
                    sourceType: 'fhir_http',
                    messageType: 'Patient',
                    correlationId: 'pat-1',
                }),
            })
        );
    });

    it('returns 404 when the interface does not exist, without ever attempting the INSERT', async () => {
        database.sequelize.query.mockResolvedValueOnce([]); // getInterfaceById finds nothing

        const req = { params: { interfaceId: 'missing' }, body: { resourceType: 'Patient' }, path: '/x', ip: '127.0.0.1', connection: {} };
        const res = mockRes();

        await controller.receiveResource(req, res);

        expect(res.status).toHaveBeenCalledWith(404);
        expect(database.sequelize.query).toHaveBeenCalledTimes(1); // only the SELECT, no INSERT attempted
    });
});
