// tests/unit/middleware/auth.test.js
//
// middleware/auth.js has zero DB coupling — pure functions of (req, res, next)
// plus jwt.verify() against process.env.JWT_SECRET. First real test file in
// the new suite covering genuinely security-relevant, previously-untested code.
const jwt = require('jsonwebtoken');
const { requireAuth, requireAdmin, requireSuperAdmin, requireRole, verifyToken } = require('../../../middleware/auth');

function mockReqRes(overrides = {}) {
    const req = { session: {}, headers: {}, query: {}, ...overrides };
    req.header = (name) => req.headers[name.toLowerCase()] || req.headers[name];
    const res = { status: jest.fn().mockReturnThis(), json: jest.fn().mockReturnThis() };
    const next = jest.fn();
    return { req, res, next };
}

describe('requireAuth', () => {
    it('calls next() when a session user is present', () => {
        const { req, res, next } = mockReqRes({ session: { user: { id: 'u1' } } });
        requireAuth(req, res, next);
        expect(next).toHaveBeenCalled();
        expect(res.status).not.toHaveBeenCalled();
    });

    it('returns 401 when there is no session user', () => {
        const { req, res, next } = mockReqRes({ session: {} });
        requireAuth(req, res, next);
        expect(next).not.toHaveBeenCalled();
        expect(res.status).toHaveBeenCalledWith(401);
    });
});

describe('requireAdmin', () => {
    it.each(['admin', 'super_admin'])('allows role %s through', (role) => {
        const { req, res, next } = mockReqRes({ session: { user: { role } } });
        requireAdmin(req, res, next);
        expect(next).toHaveBeenCalled();
    });

    it('rejects a non-admin role with 403', () => {
        const { req, res, next } = mockReqRes({ session: { user: { role: 'operator' } } });
        requireAdmin(req, res, next);
        expect(next).not.toHaveBeenCalled();
        expect(res.status).toHaveBeenCalledWith(403);
    });

    it('rejects when there is no session user at all', () => {
        const { req, res, next } = mockReqRes({ session: {} });
        requireAdmin(req, res, next);
        expect(res.status).toHaveBeenCalledWith(403);
    });
});

describe('requireSuperAdmin', () => {
    it('allows only super_admin through, not plain admin', () => {
        const superAdmin = mockReqRes({ session: { user: { role: 'super_admin' } } });
        requireSuperAdmin(superAdmin.req, superAdmin.res, superAdmin.next);
        expect(superAdmin.next).toHaveBeenCalled();

        const plainAdmin = mockReqRes({ session: { user: { role: 'admin' } } });
        requireSuperAdmin(plainAdmin.req, plainAdmin.res, plainAdmin.next);
        expect(plainAdmin.next).not.toHaveBeenCalled();
        expect(plainAdmin.res.status).toHaveBeenCalledWith(403);
    });
});

describe('requireRole', () => {
    it('allows a user whose role is in the allowed list', () => {
        const { req, res, next } = mockReqRes({ session: { user: { role: 'operator' } } });
        requireRole('admin', 'operator')(req, res, next);
        expect(next).toHaveBeenCalled();
    });

    it('rejects a user whose role is not in the allowed list', () => {
        const { req, res, next } = mockReqRes({ session: { user: { role: 'viewer' } } });
        requireRole('admin', 'operator')(req, res, next);
        expect(res.status).toHaveBeenCalledWith(403);
    });

    it('returns 401 (not 403) when there is no session user', () => {
        const { req, res, next } = mockReqRes({ session: {} });
        requireRole('admin')(req, res, next);
        expect(res.status).toHaveBeenCalledWith(401);
    });
});

describe('verifyToken', () => {
    const OLD_ENV = process.env.JWT_SECRET;
    beforeAll(() => { process.env.JWT_SECRET = 'test-secret'; });
    afterAll(() => { process.env.JWT_SECRET = OLD_ENV; });

    it('accepts a valid Bearer token and sets req.user from the JWT payload', () => {
        const token = jwt.sign({ userId: 'u1', role: 'admin' }, 'test-secret');
        const { req, res, next } = mockReqRes({ headers: { authorization: `Bearer ${token}` } });
        verifyToken(req, res, next);
        expect(next).toHaveBeenCalled();
        expect(req.user).toMatchObject({ userId: 'u1', role: 'admin' });
    });

    it('rejects an invalid/expired token with 401, never falling back to session', () => {
        const { req, res, next } = mockReqRes({
            headers: { authorization: 'Bearer not-a-real-token' },
            session: { user: { id: 'u1' } }, // present, but must not be used when a Bearer token was supplied and failed
        });
        verifyToken(req, res, next);
        expect(next).not.toHaveBeenCalled();
        expect(res.status).toHaveBeenCalledWith(401);
    });

    it('falls back to session auth when no Bearer token is present', () => {
        const { req, res, next } = mockReqRes({ session: { user: { id: 'u1', role: 'admin' } } });
        verifyToken(req, res, next);
        expect(next).toHaveBeenCalled();
        expect(req.user).toEqual({ id: 'u1', role: 'admin' });
    });

    it('returns 401 when neither a token nor a session is present', () => {
        const { req, res, next } = mockReqRes();
        verifyToken(req, res, next);
        expect(next).not.toHaveBeenCalled();
        expect(res.status).toHaveBeenCalledWith(401);
    });

    it('accepts a token passed via ?token= query param, not just the header', () => {
        const token = jwt.sign({ userId: 'u1' }, 'test-secret');
        const { req, res, next } = mockReqRes({ query: { token } });
        verifyToken(req, res, next);
        expect(next).toHaveBeenCalled();
    });
});
