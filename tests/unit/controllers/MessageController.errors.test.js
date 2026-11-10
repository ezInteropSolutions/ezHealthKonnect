// tests/unit/controllers/MessageController.errors.test.js
//
// getMessageErrors built `FROM ${tableName}` from the raw `interfaceId` query parameter (authenticated SQL
// injection into an identifier) and never checked that the caller may see that interface. It must accept only a
// UUID, must verify access the same way its sibling methods do, and must not execute anything otherwise.
const mockQuery = jest.fn();
jest.mock('../../../config/database', () => ({ sequelize: { query: (...a) => mockQuery(...a), QueryTypes: { SELECT: 'SELECT', UPDATE: 'UPDATE' } } }));
jest.mock('../../../services/AccessPolicyService', () => ({ canSeeAllInterfaces: jest.fn().mockResolvedValue(false) }));
jest.mock('../../../services/goBackendClient', () => ({ goClient: {} }));
jest.mock('../../../services/InterfaceTableManager', () => ({}));
jest.mock('../../../services/coverageGapSql', () => ({ hasCoverageGapSql: jest.fn() }));
jest.mock('../../../services/auditService', () => ({ logEvent: jest.fn().mockResolvedValue(undefined) }));

const controller = require('../../../controllers/MessageController');

const IID = '11111111-2222-3333-4444-555555555555';
const mkReq = (query) => ({ params: { messageId: 'tcp_1:2_3' }, query, session: { user: { id: 'u1', role: 'operator' } } });
function mockRes() { const r = {}; r.status = jest.fn().mockReturnValue(r); r.json = jest.fn().mockReturnValue(r); return r; }
const sqlOf = (c) => String(c[0]);

describe('getMessageErrors', () => {
    beforeEach(() => mockQuery.mockReset());

    it.each([
        ['an injection attempt', 'x"; DROP TABLE users; --'],
        ['a table name instead of a uuid', 'messages_intf_abc'],
        ['an empty id', ''],
    ])('rejects %s with 400 and runs no SQL', async (_n, interfaceId) => {
        const res = mockRes();
        await controller.getMessageErrors(mkReq({ interfaceId }), res);
        expect(res.status).toHaveBeenCalledWith(400);
        expect(mockQuery).not.toHaveBeenCalled();
    });

    it('answers 404 and never touches the message table when the caller cannot see the interface', async () => {
        mockQuery.mockResolvedValueOnce([]); // interface-access lookup returns nothing
        const res = mockRes();
        await controller.getMessageErrors(mkReq({ interfaceId: IID }), res);
        expect(res.status).toHaveBeenCalledWith(404);
        expect(mockQuery).toHaveBeenCalledTimes(1);
        expect(sqlOf(mockQuery.mock.calls[0])).toMatch(/FROM interfaces WHERE id = :interfaceId AND \(user_id = :userId OR false\)/);
    });

    it('returns the error stack and summary for an accessible interface, with a quoted table identifier', async () => {
        const errors = [{ error_timestamp: '2026-10-27T10:00:00Z', stage: 'delivery', severity: 'error', error_type: 'network', message: 'connection refused', details: 'dial tcp', stack_trace: '', recovery_action: 'retry' }];
        const summary = [{ error_count: 1, last_error_message: 'connection refused', last_error_timestamp: '2026-10-27T10:00:00Z', last_error_stage: 'delivery', error_severity: 'error', status: 'failed' }];
        mockQuery.mockResolvedValueOnce([{ id: IID }]).mockResolvedValueOnce(errors).mockResolvedValueOnce(summary);
        const res = mockRes();
        await controller.getMessageErrors(mkReq({ interfaceId: IID }), res);
        const body = res.json.mock.calls[0][0];
        expect(body.success).toBe(true);
        expect(body.data.errors).toEqual(errors);
        expect(body.data.summary.error_count).toBe(1);
        const summarySql = sqlOf(mockQuery.mock.calls[2]);
        expect(summarySql).toContain(`"messages_intf_${IID.replace(/-/g, '_')}"`);
    });
});
