// tests/unit/routes/auth.login.validation.test.js
//
// Regression: POST /api/auth/login with an empty body or non-string credentials reached
// userService.findByEmail(undefined|123), which threw, and the route answered 500 "Internal server error".
// Malformed input is the caller's fault: it must be a 400 and must never reach the user lookup.
jest.mock('../../../services/userService', () => ({ findByEmail: jest.fn() }));
jest.mock('../../../services/auditService', () => ({ logEvent: jest.fn().mockResolvedValue(undefined) }));
jest.mock('../../../services/settingsService', () => ({ getSetting: jest.fn().mockResolvedValue(undefined) }));
jest.mock('../../../services/AccessPolicyService', () => ({}));
jest.mock('../../../middleware/auth', () => ({ requireAuth: (req, res, next) => next() }));

const userService = require('../../../services/userService');
const router = require('../../../routes/auth');

function loginHandler() {
    const layer = router.stack.find((l) => l.route && l.route.path === '/login' && l.route.methods.post);
    return layer.route.stack[layer.route.stack.length - 1].handle; // last = the async handler, after the rate limiter
}
function mockRes() {
    const res = {};
    res.status = jest.fn().mockReturnValue(res);
    res.json = jest.fn().mockReturnValue(res);
    return res;
}

describe('POST /api/auth/login input validation', () => {
    beforeEach(() => userService.findByEmail.mockReset());

    it.each([
        ['empty body', {}],
        ['missing password', { email: 'a@b.com' }],
        ['missing email', { password: 'x' }],
        ['non-string email', { email: 123, password: 'x' }],
        ['non-string password', { email: 'a@b.com', password: { $ne: null } }],
        ['empty strings', { email: '', password: '' }],
    ])('%s -> 400 and no user lookup', async (_name, body) => {
        const res = mockRes();
        await loginHandler()({ body, headers: {}, get: () => '', sessionID: 's', clientIP: '127.0.0.1' }, res);
        expect(res.status).toHaveBeenCalledWith(400);
        expect(userService.findByEmail).not.toHaveBeenCalled();
    });
});
