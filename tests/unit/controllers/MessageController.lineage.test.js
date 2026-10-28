// tests/unit/controllers/MessageController.lineage.test.js
//
// Regression: getDataLineage declared `const allInterfaces` (the access-policy boolean) and then, inside its
// try block, a second `const allInterfaces` (the interface rows) whose own initializer interpolated
// ${allInterfaces}. That is a temporal-dead-zone ReferenceError, so every lineage request failed before any
// query ran and the Journey tab answered "Message not found" for every message (found by the v26.10 UAT pass).
const mockQuery = jest.fn();
jest.mock('../../../config/database', () => ({ sequelize: { query: (...a) => mockQuery(...a), QueryTypes: { SELECT: 'SELECT', UPDATE: 'UPDATE' } } }));
jest.mock('../../../services/AccessPolicyService', () => ({ canSeeAllInterfaces: jest.fn().mockResolvedValue(true) }));
jest.mock('../../../services/goBackendClient', () => ({ goClient: {} }));
jest.mock('../../../services/InterfaceTableManager', () => ({}));
jest.mock('../../../services/coverageGapSql', () => ({ hasCoverageGapSql: jest.fn() }));
jest.mock('../../../services/auditService', () => ({ logEvent: jest.fn().mockResolvedValue(undefined) }));

const controller = require('../../../controllers/MessageController');

function mockRes() {
    const res = {};
    res.status = jest.fn().mockReturnValue(res);
    res.json = jest.fn().mockReturnValue(res);
    return res;
}
const req = { params: { messageId: 'tcp_1.2.3.4:5_1' }, session: { user: { id: 'u1', role: 'admin' } }, query: {} };

describe('getDataLineage', () => {
    beforeEach(() => mockQuery.mockReset());

    it('looks up the accessible interfaces with the policy flag interpolated', async () => {
        mockQuery.mockResolvedValue([]);
        await controller.getDataLineage(req, mockRes());
        // The interface lookup must run at all: a TDZ error throws before any query.
        expect(mockQuery).toHaveBeenCalled();
        const sql = String(mockQuery.mock.calls[0][0]);
        expect(sql).toMatch(/FROM interfaces WHERE \(user_id = :userId OR true\)/);
    });

    it('searches each accessible interface message table for the message', async () => {
        mockQuery
            .mockResolvedValueOnce([{ id: 'aaaa-bbbb', name: 'n', source_type: 'x', target_type: 'y' }])
            .mockResolvedValue([]);
        await controller.getDataLineage(req, mockRes());
        const sqls = mockQuery.mock.calls.map((c) => String(c[0]));
        expect(sqls.some((s) => s.includes('messages_intf_aaaa_bbbb'))).toBe(true);
    });
});

describe('getDataLineage with the interface known', () => {
    beforeEach(() => mockQuery.mockReset());
    const id = '11111111-2222-3333-4444-555555555555';

    it('searches only that interface (one table, not every interface)', async () => {
        mockQuery
            .mockResolvedValueOnce([{ id, name: "O'Brien", source_type: 'x', target_type: 'y' }])
            .mockResolvedValue([]);
        await controller.getDataLineage({ ...req, query: { interfaceId: id } }, mockRes());
        const sqls = mockQuery.mock.calls.map((c) => String(c[0]));
        expect(sqls[0]).toMatch(/AND id = :wantedInterface/);
        expect(mockQuery.mock.calls[0][1].replacements.wantedInterface).toBe(id);
        expect(sqls.filter((s) => s.includes('messages_intf_')).length).toBe(1);
    });

    it('ignores an interfaceId that is not a UUID', async () => {
        mockQuery.mockResolvedValue([]);
        await controller.getDataLineage({ ...req, query: { interfaceId: "x'; DROP TABLE users;--" } }, mockRes());
        expect(String(mockQuery.mock.calls[0][0])).not.toMatch(/wantedInterface/);
        expect(mockQuery.mock.calls[0][1].replacements.wantedInterface).toBeNull();
    });

    it('never splices the interface name into SQL text', async () => {
        mockQuery
            .mockResolvedValueOnce([{ id, name: "O'Brien", source_type: 'x', target_type: 'y' }])
            .mockResolvedValue([]);
        await controller.getDataLineage({ ...req, query: { interfaceId: id } }, mockRes());
        for (const c of mockQuery.mock.calls) expect(String(c[0])).not.toContain("O'Brien");
    });
});
