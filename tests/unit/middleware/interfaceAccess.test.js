// tests/unit/middleware/interfaceAccess.test.js
//
// requireInterfaceAccess guards Go-forwarded routes that take an interface id (message trace): the id must be a UUID
// and the caller must be allowed to see that interface (own interface, or the access policy lets the role see all).
// It must fail closed.
const { requireInterfaceAccess } = require('../../../middleware/interfaceAccess');

const IID = '11111111-2222-3333-4444-555555555555';

function harness({ rows = [{ id: IID }], canSeeAll = false, queryError = null } = {}) {
    const query = jest.fn(async () => { if (queryError) throw queryError; return rows; });
    const db = { sequelize: { query, QueryTypes: { SELECT: 'SELECT' } } };
    const policy = { canSeeAllInterfaces: jest.fn().mockResolvedValue(canSeeAll) };
    const mw = requireInterfaceAccess({ database: db, accessPolicy: policy });
    const res = { status: jest.fn().mockReturnThis(), json: jest.fn().mockReturnThis() };
    const next = jest.fn();
    const run = (req) => mw(req, res, next);
    return { run, res, next, query, policy };
}
const req = (query, user = { id: 'u1', role: 'operator' }) => ({ query, session: user ? { user } : {} });

describe('requireInterfaceAccess', () => {
    it.each([
        ['missing', {}],
        ['empty', { interfaceId: '' }],
        ['injection text', { interfaceId: 'x"; DROP TABLE users; --' }],
        ['a table name', { interfaceId: 'messages_intf_abc' }],
    ])('answers 400 and runs no SQL when interfaceId is %s', async (_n, query) => {
        const h = harness();
        await h.run(req(query));
        expect(h.res.status).toHaveBeenCalledWith(400);
        expect(h.query).not.toHaveBeenCalled();
        expect(h.next).not.toHaveBeenCalled();
    });

    it('answers 401 when there is no signed-in user', async () => {
        const h = harness();
        await h.run(req({ interfaceId: IID }, null));
        expect(h.res.status).toHaveBeenCalledWith(401);
        expect(h.next).not.toHaveBeenCalled();
    });

    it('lets the request through when the caller can see the interface', async () => {
        const h = harness({ rows: [{ id: IID }] });
        await h.run(req({ interfaceId: IID }));
        expect(h.next).toHaveBeenCalledTimes(1);
        expect(String(h.query.mock.calls[0][0])).toMatch(/FROM interfaces WHERE id = :interfaceId AND \(user_id = :userId OR false\)/);
    });

    it('interpolates the policy result as a literal boolean, never user input', async () => {
        const h = harness({ canSeeAll: true });
        await h.run(req({ interfaceId: IID }));
        expect(String(h.query.mock.calls[0][0])).toMatch(/OR true\)/);
    });

    it('answers 404 (not 403) when the interface is not visible to the caller, so ids are not confirmable', async () => {
        const h = harness({ rows: [] });
        await h.run(req({ interfaceId: IID }));
        expect(h.res.status).toHaveBeenCalledWith(404);
        expect(h.next).not.toHaveBeenCalled();
    });

    it('fails closed when the database or policy errors', async () => {
        const h = harness({ queryError: new Error('db down') });
        await h.run(req({ interfaceId: IID }));
        expect(h.next).not.toHaveBeenCalled();
        expect(h.res.status).toHaveBeenCalledWith(500);
    });
});
