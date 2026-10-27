// tests/unit/scripts/initDevEnv.test.js
const { buildEnv, parseEnv, SECRET_GENERATORS } = require('../../../scripts/init-dev-env');
const SecretsGuard = require('../../../config/SecretsGuard');

const EXAMPLE = [
    '# comment',
    'SESSION_SECRET=',
    'JWT_SECRET=',
    'APP_CREDENTIAL_KEY=',
    'DB_PASSWORD=',
    'MINIO_PASSWORD=',
    'NODE_ENV=development',
].join('\n');

describe('init-dev-env', () => {
    it('creates .env with every secret generated and non-secret settings kept', () => {
        const { text, generated } = buildEnv(null, EXAMPLE);
        const env = parseEnv(text);
        expect(generated.sort()).toEqual(Object.keys(SECRET_GENERATORS).sort());
        expect(env.NODE_ENV).toBe('development');
        for (const name of Object.keys(SECRET_GENERATORS)) expect(env[name]).toBeTruthy();
        expect(Buffer.from(env.APP_CREDENTIAL_KEY, 'base64')).toHaveLength(32);
        expect(env.DB_PASSWORD).toMatch(/^[0-9a-f]+$/); // URL-safe for connection strings
    });

    it('generated secrets satisfy the startup guard', () => {
        const env = parseEnv(buildEnv(null, EXAMPLE).text);
        expect(() => new SecretsGuard({ ...env, NODE_ENV: 'production', SESSION_COOKIE_SECURE: 'auto' })
            .enforce({ warn: () => {}, error: () => {} })).not.toThrow();
    });

    it('generates different values each time', () => {
        expect(parseEnv(buildEnv(null, EXAMPLE).text).JWT_SECRET)
            .not.toBe(parseEnv(buildEnv(null, EXAMPLE).text).JWT_SECRET);
    });

    it('never overwrites an existing value and only appends what is missing', () => {
        const existing = 'SESSION_SECRET=keep-me-' + 'x'.repeat(40) + '\nDB_PASSWORD=keep-db\n';
        const { text, generated } = buildEnv(existing, EXAMPLE);
        const env = parseEnv(text);
        expect(env.SESSION_SECRET).toBe('keep-me-' + 'x'.repeat(40));
        expect(env.DB_PASSWORD).toBe('keep-db');
        expect(generated.sort()).toEqual(['APP_CREDENTIAL_KEY', 'JWT_SECRET', 'MINIO_PASSWORD']);
        expect(text.startsWith(existing)).toBe(true);
    });

    it('leaves a complete .env unchanged', () => {
        const full = Object.keys(SECRET_GENERATORS).map(n => `${n}=v-${n}`).join('\n') + '\n';
        const { text, generated } = buildEnv(full, EXAMPLE);
        expect(text).toBe(full);
        expect(generated).toEqual([]);
    });
});
