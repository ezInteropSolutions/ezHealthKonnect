'use strict';
/**
 * PasswordResetTokenService — single-use, short-lived proof that the caller just authenticated with the CURRENT
 * password of an account that must change it.
 *
 * Why: POST /api/auth/reset-password used to accept only { userId, password } with no credential at all, so anyone
 * who knew (or obtained) the id of an account flagged "must reset" could set its password and take it over. The
 * only legitimate caller is the force-reset page, reached right after a successful login; that login now issues
 * the token and the reset must present it.
 *
 * Only a SHA-256 of the token is stored (users.password_reset_token / password_reset_expires), so a database read
 * does not yield a usable token. A token works once, and only for the account it was issued for.
 */
const crypto = require('crypto');

const TOKEN_TTL_MS = 15 * 60 * 1000;
const hash = (token) => crypto.createHash('sha256').update(String(token)).digest('hex');

class PasswordResetTokenService {
    /** @param {{ ttlMs?: number, now?: () => number }} [options] injectable for tests */
    constructor({ ttlMs = TOKEN_TTL_MS, now = Date.now } = {}) {
        this.ttlMs = ttlMs;
        this.now = now;
    }

    /** Creates and stores a token for `user` (a User model instance) and returns the plain token. */
    async issue(user) {
        const token = crypto.randomBytes(32).toString('hex');
        await user.update({
            password_reset_token: hash(token),
            password_reset_expires: new Date(this.now() + this.ttlMs),
        });
        return token;
    }

    /** True only for the exact, unexpired token issued to this user. Does not consume it. */
    verify(user, token) {
        if (!user || typeof token !== 'string' || !token || !user.password_reset_token || !user.password_reset_expires) return false;
        if (new Date(user.password_reset_expires).getTime() < this.now()) return false;
        const a = Buffer.from(hash(token), 'hex');
        const b = Buffer.from(user.password_reset_token, 'hex');
        return a.length === b.length && crypto.timingSafeEqual(a, b);
    }

    /** The fields that make a token unusable; spread into the user.update() that completes the reset. */
    static cleared() {
        return { password_reset_token: null, password_reset_expires: null };
    }
}

module.exports = new PasswordResetTokenService();
module.exports.PasswordResetTokenService = PasswordResetTokenService;
module.exports.TOKEN_TTL_MS = TOKEN_TTL_MS;
