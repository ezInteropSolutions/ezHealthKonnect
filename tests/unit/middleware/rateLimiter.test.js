// tests/unit/middleware/rateLimiter.test.js
//
// RateLimiter is fully constructor-injected (window/max/key-derivation all
// passed in, no globals) and its own comments note the cleanup timer was
// already unref()'d "so tests don't hang" — a rare case of code in this
// codebase written with testing already in mind.
const RateLimiter = require('../../../middleware/rateLimiter');

function mockReqRes(ip = '1.2.3.4') {
    const req = { ip };
    const res = { set: jest.fn(), status: jest.fn().mockReturnThis(), json: jest.fn().mockReturnThis() };
    const next = jest.fn();
    return { req, res, next };
}

describe('RateLimiter', () => {
    it('allows requests under the max within the window', () => {
        const limiter = new RateLimiter({ windowMs: 60000, max: 3 });
        const mw = limiter.middleware();
        for (let i = 0; i < 3; i++) {
            const { req, res, next } = mockReqRes();
            mw(req, res, next);
            expect(next).toHaveBeenCalled();
            expect(res.status).not.toHaveBeenCalled();
        }
    });

    it('blocks the request that exceeds max with 429 and a Retry-After header', () => {
        const limiter = new RateLimiter({ windowMs: 60000, max: 2 });
        const mw = limiter.middleware();
        const ip = '5.5.5.5';

        const calls = [mockReqRes(ip), mockReqRes(ip), mockReqRes(ip)];
        calls.forEach(({ req, res, next }) => mw(req, res, next));
        const third = calls[2];
        expect(third.res.status).toHaveBeenCalledWith(429);
        expect(third.res.set).toHaveBeenCalledWith('Retry-After', expect.any(String));
        expect(third.next).not.toHaveBeenCalled();
    });

    it('tracks separate keys independently (per-IP by default)', () => {
        const limiter = new RateLimiter({ windowMs: 60000, max: 1 });
        const mw = limiter.middleware();

        const a1 = mockReqRes('1.1.1.1');
        mw(a1.req, a1.res, a1.next);
        expect(a1.next).toHaveBeenCalled();

        const b1 = mockReqRes('2.2.2.2');
        mw(b1.req, b1.res, b1.next);
        expect(b1.next).toHaveBeenCalled(); // different key — must not be blocked by IP 1.1.1.1's usage

        const a2 = mockReqRes('1.1.1.1');
        mw(a2.req, a2.res, a2.next);
        expect(a2.res.status).toHaveBeenCalledWith(429); // second request from the SAME key — must be blocked
    });

    it('resets the count once the window has elapsed', () => {
        jest.useFakeTimers();
        const limiter = new RateLimiter({ windowMs: 1000, max: 1 });
        const mw = limiter.middleware();
        const ip = '9.9.9.9';

        const first = mockReqRes(ip);
        mw(first.req, first.res, first.next);
        expect(first.next).toHaveBeenCalled();

        const second = mockReqRes(ip);
        mw(second.req, second.res, second.next);
        expect(second.res.status).toHaveBeenCalledWith(429); // still within window

        jest.advanceTimersByTime(1001); // past the window

        const third = mockReqRes(ip);
        mw(third.req, third.res, third.next);
        expect(third.next).toHaveBeenCalled(); // window reset — allowed again

        jest.useRealTimers();
    });

    it('uses a custom keyFn when provided instead of the IP default', () => {
        const limiter = new RateLimiter({ windowMs: 60000, max: 1, keyFn: (req) => req.username });
        const mw = limiter.middleware();

        const first = { req: { ip: 'same-ip', username: 'alice' }, res: mockReqRes().res, next: jest.fn() };
        mw(first.req, first.res, first.next);
        expect(first.next).toHaveBeenCalled();

        // Same IP, different username via the custom keyFn — must be treated as a separate bucket.
        const second = { req: { ip: 'same-ip', username: 'bob' }, res: mockReqRes().res, next: jest.fn() };
        mw(second.req, second.res, second.next);
        expect(second.next).toHaveBeenCalled();
    });
});
