// middleware/rateLimiter.js
// Lightweight in-memory per-key rate limiter — no external dependency, since this
// currently protects a single endpoint (POST /api/auth/login). Security audit found
// account lockout is per-user only (see routes/auth.js); nothing throttled attempts
// spread across many usernames from a single IP. If more endpoints need this later,
// revisit whether a package like express-rate-limit is worth the dependency.

class RateLimiter {
    /**
     * @param {number} windowMs - length of each rate-limit window in milliseconds
     * @param {number} max - max requests allowed per key within windowMs
     * @param {(req: import('express').Request) => string} [keyFn] - derives the
     *   bucket key from a request; defaults to client IP.
     */
    constructor({ windowMs, max, keyFn }) {
        this.windowMs = windowMs;
        this.max = max;
        this.keyFn = keyFn || ((req) => req.clientIP || req.ip || 'unknown');
        this.hits = new Map(); // key -> { count, resetAt }

        // Periodic cleanup so memory doesn't grow unbounded under many distinct keys.
        // unref() lets the process exit normally in tests without an open handle.
        this.cleanupTimer = setInterval(() => this._cleanup(), windowMs * 2);
        if (typeof this.cleanupTimer.unref === 'function') {
            this.cleanupTimer.unref();
        }
    }

    _cleanup() {
        const now = Date.now();
        for (const [key, entry] of this.hits) {
            if (entry.resetAt <= now) {
                this.hits.delete(key);
            }
        }
    }

    /** Returns Express middleware enforcing this limiter's window/max per key. */
    middleware() {
        return (req, res, next) => {
            const key = this.keyFn(req);
            const now = Date.now();
            let entry = this.hits.get(key);
            if (!entry || entry.resetAt <= now) {
                entry = { count: 0, resetAt: now + this.windowMs };
                this.hits.set(key, entry);
            }
            entry.count += 1;

            if (entry.count > this.max) {
                const retryAfterSec = Math.max(1, Math.ceil((entry.resetAt - now) / 1000));
                res.set('Retry-After', String(retryAfterSec));
                return res.status(429).json({
                    success: false,
                    message: 'Too many requests — please slow down and try again shortly.',
                });
            }

            next();
        };
    }
}

module.exports = RateLimiter;
