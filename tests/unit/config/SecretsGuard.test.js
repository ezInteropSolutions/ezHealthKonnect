// tests/unit/config/SecretsGuard.test.js
const SecretsGuard = require('../../../config/SecretsGuard');

const STRONG = 'x'.repeat(48);
const KEY = Buffer.alloc(32, 7).toString('base64'); // test fixture, built at run time

function silentLogger() {
    return { warn: jest.fn(), error: jest.fn() };
}

describe('SecretsGuard in production', () => {
    const prod = (extra) => new SecretsGuard({ NODE_ENV: 'production', SESSION_COOKIE_SECURE: 'auto', APP_CREDENTIAL_KEY: KEY, ...extra });

    it('passes with strong secrets', () => {
        expect(() => prod({ SESSION_SECRET: STRONG, JWT_SECRET: STRONG }).enforce(silentLogger())).not.toThrow();
    });

    it.each([
        ['missing session secret', { JWT_SECRET: STRONG }],
        ['missing JWT secret', { SESSION_SECRET: STRONG }],
        ['placeholder session secret', { SESSION_SECRET: 'your-session-secret', JWT_SECRET: STRONG }],
        ['placeholder JWT secret', { SESSION_SECRET: STRONG, JWT_SECRET: 'your-secret-key-change-this-in-production' }],
        ['short secret', { SESSION_SECRET: 'short', JWT_SECRET: STRONG }],
    ])('refuses to start: %s', (_, env) => {
        expect(() => prod(env).enforce(silentLogger())).toThrow(/Refusing to start in production/);
    });

    it('warns (does not fail) when APP_CREDENTIAL_KEY is missing', () => {
        const logger = silentLogger();
        new SecretsGuard({ NODE_ENV: 'production', SESSION_COOKIE_SECURE: 'auto', SESSION_SECRET: STRONG, JWT_SECRET: STRONG }).enforce(logger);
        expect(logger.warn).toHaveBeenCalledWith(expect.stringMatching(/APP_CREDENTIAL_KEY is not set/));
    });

    it('warns when session cookies are not Secure in production', () => {
        const logger = silentLogger();
        new SecretsGuard({ NODE_ENV: 'production', SESSION_SECRET: STRONG, JWT_SECRET: STRONG, APP_CREDENTIAL_KEY: KEY }).enforce(logger);
        expect(logger.warn).toHaveBeenCalledWith(expect.stringMatching(/SESSION_COOKIE_SECURE/));
    });
});

describe('SecretsGuard outside production', () => {
    it('never throws, but reports weak secrets as warnings', () => {
        const logger = silentLogger();
        expect(() => new SecretsGuard({ NODE_ENV: 'development' }).enforce(logger)).not.toThrow();
        expect(logger.warn).toHaveBeenCalledWith(expect.stringMatching(/SESSION_SECRET is not set/));
    });
});
