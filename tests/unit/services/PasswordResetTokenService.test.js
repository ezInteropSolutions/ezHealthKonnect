'use strict';
const { PasswordResetTokenService } = require('../../../services/PasswordResetTokenService');

function fakeUser() {
    const u = { password_reset_token: null, password_reset_expires: null };
    u.update = async (fields) => { Object.assign(u, fields); };
    return u;
}

describe('PasswordResetTokenService', () => {
    test('an issued token verifies for that user, and only a hash is stored', async () => {
        const svc = new PasswordResetTokenService();
        const user = fakeUser();
        const token = await svc.issue(user);
        expect(token).toMatch(/^[0-9a-f]{64}$/);
        expect(user.password_reset_token).not.toBe(token);
        expect(user.password_reset_token).toMatch(/^[0-9a-f]{64}$/);
        expect(svc.verify(user, token)).toBe(true);
    });

    test('a wrong, empty, missing or non-string token is refused', async () => {
        const svc = new PasswordResetTokenService();
        const user = fakeUser();
        await svc.issue(user);
        for (const bad of ['f'.repeat(64), '', undefined, null, 123, {}]) expect(svc.verify(user, bad)).toBe(false);
        expect(svc.verify(fakeUser(), 'a'.repeat(64))).toBe(false);   // nothing issued
        expect(svc.verify(null, 'a'.repeat(64))).toBe(false);
    });

    test('a token issued to one user does not work for another', async () => {
        const svc = new PasswordResetTokenService();
        const a = fakeUser();
        const b = fakeUser();
        const tokenA = await svc.issue(a);
        await svc.issue(b);
        expect(svc.verify(b, tokenA)).toBe(false);
    });

    test('a token expires', async () => {
        let now = 1_000_000;
        const svc = new PasswordResetTokenService({ ttlMs: 60_000, now: () => now });
        const user = fakeUser();
        const token = await svc.issue(user);
        now += 59_000;
        expect(svc.verify(user, token)).toBe(true);
        now += 2_000;
        expect(svc.verify(user, token)).toBe(false);
    });

    test('clearing makes the token unusable (single use)', async () => {
        const svc = new PasswordResetTokenService();
        const user = fakeUser();
        const token = await svc.issue(user);
        await user.update(PasswordResetTokenService.cleared());
        expect(svc.verify(user, token)).toBe(false);
    });

    test('issuing again replaces the previous token', async () => {
        const svc = new PasswordResetTokenService();
        const user = fakeUser();
        const first = await svc.issue(user);
        const second = await svc.issue(user);
        expect(svc.verify(user, first)).toBe(false);
        expect(svc.verify(user, second)).toBe(true);
    });
});
