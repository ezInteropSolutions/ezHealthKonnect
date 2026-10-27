// tests/unit/middleware/partnerEndpoint.test.js
//
// attachUserIfPresent / forwardPartnerCredentials — the Node half of the
// partner-endpoint auth path (/api/eligibility, /api/claim-status,
// /api/prior-auth, /api/pharmacy-claim). The real credential check happens in
// Go per interface; Node must (a) never reject, since a partner's own Bearer
// token is not an app JWT, and (b) still identify a genuinely logged-in user.
const jwt = require('jsonwebtoken');
const { attachUserIfPresent, forwardPartnerCredentials } = require('../../../middleware/auth');

function mockReqRes(overrides = {}) {
    const req = { session: {}, headers: {}, query: {}, ...overrides };
    req.header = (name) => req.headers[name.toLowerCase()] || req.headers[name];
    const res = { status: jest.fn().mockReturnThis(), json: jest.fn().mockReturnThis() };
    const next = jest.fn();
    return { req, res, next };
}

describe('attachUserIfPresent', () => {
    const SECRET = 'test-jwt-secret-for-partner-endpoint';
    let prev;
    beforeAll(() => { prev = process.env.JWT_SECRET; process.env.JWT_SECRET = SECRET; });
    afterAll(() => { process.env.JWT_SECRET = prev; });

    it('attaches the user for a valid app JWT', () => {
        const token = jwt.sign({ userId: 'u1', role: 'admin' }, SECRET);
        const { req, res, next } = mockReqRes({ headers: { authorization: `Bearer ${token}` } });
        attachUserIfPresent(req, res, next);
        expect(next).toHaveBeenCalled();
        expect(req.user.userId).toBe('u1');
    });

    it('does NOT reject a non-JWT bearer token (a partner credential) and attaches no user', () => {
        const { req, res, next } = mockReqRes({ headers: { authorization: 'Bearer partner-opaque-token' } });
        attachUserIfPresent(req, res, next);
        expect(next).toHaveBeenCalled();
        expect(res.status).not.toHaveBeenCalled();
        expect(req.user).toBeUndefined();
    });

    it('does NOT reject Basic credentials and attaches no user', () => {
        const { req, res, next } = mockReqRes({ headers: { authorization: 'Basic dTpw' } });
        attachUserIfPresent(req, res, next);
        expect(next).toHaveBeenCalled();
        expect(req.user).toBeUndefined();
    });

    it('falls back to the session user', () => {
        const { req, res, next } = mockReqRes({ session: { user: { id: 'u2', role: 'engineer' } } });
        attachUserIfPresent(req, res, next);
        expect(next).toHaveBeenCalled();
        expect(req.user.id).toBe('u2');
    });

    it('continues anonymously with no credentials at all', () => {
        const { req, res, next } = mockReqRes();
        attachUserIfPresent(req, res, next);
        expect(next).toHaveBeenCalled();
        expect(res.status).not.toHaveBeenCalled();
        expect(req.user).toBeUndefined();
    });
});

describe('forwardPartnerCredentials', () => {
    it('flags the request for partner credential forwarding', () => {
        const { req, res, next } = mockReqRes();
        forwardPartnerCredentials(req, res, next);
        expect(req.forwardPartnerCredentials).toBe(true);
        expect(next).toHaveBeenCalled();
    });
});

describe('requireRoleByMethod', () => {
    const { requireRoleByMethod } = require('../../../middleware/auth');
    const gate = requireRoleByMethod(['admin', 'operator', 'viewer'], ['admin', 'operator']);
    const run = (method, user) => {
        const { req, res, next } = mockReqRes({ method, user });
        gate(req, res, next);
        return { res, next };
    };

    it.each([
        ['GET', 'viewer', true],
        ['GET', 'admin', true],
        ['POST', 'viewer', false],
        ['DELETE', 'viewer', false],
        ['POST', 'operator', true],
        ['GET', 'user', false],
        ['POST', 'user', false],
    ])('%s by %s -> allowed=%s', (method, role, allowed) => {
        const { res, next } = run(method, { id: 'u', role });
        expect(next.mock.calls.length > 0).toBe(allowed);
        if (!allowed) expect(res.status).toHaveBeenCalledWith(403);
    });

    it('401 without a user', () => {
        const { res, next } = run('GET', undefined);
        expect(next).not.toHaveBeenCalled();
        expect(res.status).toHaveBeenCalledWith(401);
    });

    it('accepts a session user when no JWT user is set', () => {
        const { req, res, next } = mockReqRes({ method: 'POST', session: { user: { role: 'operator' } } });
        gate(req, res, next);
        expect(next).toHaveBeenCalled();
    });
});
