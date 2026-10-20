// tests/unit/config/SecretsGuard.test.js
const SecretsGuard = require('../../../config/SecretsGuard');
const { CompromisedSecretList, sha256 } = SecretsGuard;

// Fixtures are built at run time so secret scanners never see a literal.
const STRONG = 'x'.repeat(48);
const KEY = Buffer.alloc(32, 7).toString('base64');
const LEAKED = 'L'.repeat(40);

const compromised = new CompromisedSecretList({
    guardedVariables: ['SESSION_SECRET', 'JWT_SECRET', 'APP_CREDENTIAL_KEY', 'DB_PASSWORD'],
    fingerprints: [
        { sha256: sha256(LEAKED), note: 'test leak' },
        { sha256: sha256('your-session-secret'), note: 'placeholder' },
    ],
});

function silentLogger() {
    return { warn: jest.fn(), error: jest.fn() };
}
const guard = (env) => new SecretsGuard(env, compromised);

describe('SecretsGuard in production', () => {
    const prod = (extra) => guard({ NODE_ENV: 'production', SESSION_COOKIE_SECURE: 'auto', APP_CREDENTIAL_KEY: KEY, ...extra });

    it('passes with strong secrets', () => {
        expect(() => prod({ SESSION_SECRET: STRONG, JWT_SECRET: STRONG }).enforce(silentLogger())).not.toThrow();
    });

    it.each([
        ['missing session secret', { JWT_SECRET: STRONG }],
        ['missing JWT secret', { SESSION_SECRET: STRONG }],
        ['short secret', { SESSION_SECRET: 'short', JWT_SECRET: STRONG }],
        ['leaked session secret', { SESSION_SECRET: LEAKED, JWT_SECRET: STRONG }],
        ['leaked credential key', { SESSION_SECRET: STRONG, JWT_SECRET: STRONG, APP_CREDENTIAL_KEY: LEAKED }],
        ['leaked database password', { SESSION_SECRET: STRONG, JWT_SECRET: STRONG, DB_PASSWORD: LEAKED }],
    ])('refuses to start: %s', (_, env) => {
        expect(() => prod(env).enforce(silentLogger())).toThrow(/Refusing to start/);
    });

    it('matches placeholders case-insensitively', () => {
        const padded = 'YOUR-SESSION-SECRET';
        expect(compromised.match(padded)).toBe('placeholder');
    });

    it('never echoes a secret value in its messages', () => {
        const logger = silentLogger();
        expect(() => prod({ SESSION_SECRET: LEAKED, JWT_SECRET: STRONG }).enforce(logger)).toThrow();
        const all = [...logger.error.mock.calls, ...logger.warn.mock.calls].flat().join(' ');
        expect(all).not.toContain(LEAKED);
    });

    it('warns (does not fail) when APP_CREDENTIAL_KEY is missing', () => {
        const logger = silentLogger();
        guard({ NODE_ENV: 'production', SESSION_COOKIE_SECURE: 'auto', SESSION_SECRET: STRONG, JWT_SECRET: STRONG }).enforce(logger);
        expect(logger.warn).toHaveBeenCalledWith(expect.stringMatching(/APP_CREDENTIAL_KEY is not set/));
    });

    it('warns when session cookies are not Secure in production', () => {
        const logger = silentLogger();
        guard({ NODE_ENV: 'production', SESSION_SECRET: STRONG, JWT_SECRET: STRONG, APP_CREDENTIAL_KEY: KEY }).enforce(logger);
        expect(logger.warn).toHaveBeenCalledWith(expect.stringMatching(/SESSION_COOKIE_SECURE/));
    });
});

describe('SecretsGuard outside production', () => {
    it('refuses to start when secrets are missing — there is no fallback value any more', () => {
        expect(() => guard({ NODE_ENV: 'development' }).enforce(silentLogger())).toThrow(/SESSION_SECRET is not set/);
        expect(() => guard({}).enforce(silentLogger())).toThrow(/Refusing to start/);
    });

    it('only warns about a leaked value, so an existing dev environment keeps working', () => {
        const logger = silentLogger();
        expect(() => guard({ NODE_ENV: 'development', SESSION_SECRET: STRONG, JWT_SECRET: STRONG, APP_CREDENTIAL_KEY: LEAKED }).enforce(logger)).not.toThrow();
        expect(logger.warn).toHaveBeenCalledWith(expect.stringMatching(/APP_CREDENTIAL_KEY is a publicly-known value/));
    });

    it('unit tests (NODE_ENV=test) may run without secrets', () => {
        expect(() => guard({ NODE_ENV: 'test' }).enforce(silentLogger())).not.toThrow();
    });
});

describe('the shipped compromised-secrets list', () => {
    const shipped = CompromisedSecretList.load();

    it('guards every app secret and stores only SHA-256 fingerprints', () => {
        for (const name of ['SESSION_SECRET', 'JWT_SECRET', 'INTERNAL_PROXY_SECRET', 'APP_CREDENTIAL_KEY', 'DB_PASSWORD']) {
            expect(shipped.guardedVariables).toContain(name);
        }
        for (const hash of shipped.byHash.keys()) {
            expect(hash).toMatch(/^[0-9a-f]{64}$/);
        }
    });

    it('includes the well-known code placeholders', () => {
        expect(shipped.match('your-secret-key-change-this-in-production')).not.toBeNull();
        expect(shipped.match('dev-encryption-key')).not.toBeNull();
    });
});
