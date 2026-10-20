// tests/unit/services/InterfaceConfigProtector.test.js
const InterfaceConfigProtector = require('../../../services/InterfaceConfigProtector');
const InterfaceCredentialMigrator = require('../../../services/InterfaceCredentialMigrator');
const { encryptSensitiveConfigFields, isSensitiveConfigKey } = require('../../../services/credentialCrypto');

const MASK = InterfaceConfigProtector.MASK_PLACEHOLDER;
const KEY = Buffer.alloc(32, 3).toString('base64');

function withKey(fn) {
    const prev = process.env.APP_CREDENTIAL_KEY;
    process.env.APP_CREDENTIAL_KEY = KEY;
    try { return fn(); } finally {
        if (prev === undefined) delete process.env.APP_CREDENTIAL_KEY; else process.env.APP_CREDENTIAL_KEY = prev;
    }
}

const protector = new InterfaceConfigProtector({ isSensitiveKey: isSensitiveConfigKey, encryptFields: encryptSensitiveConfigFields });

describe('maskForDisplay', () => {
    it('masks secrets at any depth and keeps everything else', () => {
        const out = protector.maskForDisplay({
            host: 'h', port: 22, password: 'p',
            auth: { bearer_token: 't', api_key_header: 'X-Key', api_key: 'k' },
            endpoint_auth: { authentication_type: 'basic_auth', username: 'u', password: 'p2' },
        });
        expect(out).toEqual({
            host: 'h', port: 22, password: MASK,
            auth: { bearer_token: MASK, api_key_header: 'X-Key', api_key: MASK },
            endpoint_auth: { authentication_type: 'basic_auth', username: 'u', password: MASK },
        });
    });

    it('passes non-objects through', () => {
        expect(protector.maskForDisplay(null)).toBeNull();
        expect(protector.maskForDisplay(undefined)).toBeUndefined();
    });
});

describe('protectForStorage', () => {
    it('encrypts secrets, keeps non-secret fields (incl. header names) plaintext', () => withKey(() => {
        const out = protector.protectForStorage({ host: 'h', password: 'p', api_key_header: 'X-Key', api_key: 'k' }, {});
        expect(out.host).toBe('h');
        expect(out.api_key_header).toBe('X-Key');
        expect(out.password).toMatch(/^ENC:v1:/);
        expect(out.api_key).toMatch(/^ENC:v1:/);
    }));

    it('resolves a masked echo to the stored value instead of storing the placeholder', () => withKey(() => {
        const stored = { host: 'h', password: 'ENC:v1:stored' };
        const out = protector.protectForStorage({ host: 'h2', password: MASK }, stored);
        expect(out.password).toBe('ENC:v1:stored');
        expect(out.host).toBe('h2');
        expect(JSON.stringify(out)).not.toContain(MASK);
    }));

    it('is a passthrough without APP_CREDENTIAL_KEY (dev mode)', () => {
        const prev = process.env.APP_CREDENTIAL_KEY;
        delete process.env.APP_CREDENTIAL_KEY;
        try {
            expect(protector.protectForStorage({ password: 'p' }, {})).toEqual({ password: 'p' });
        } finally {
            if (prev !== undefined) process.env.APP_CREDENTIAL_KEY = prev;
        }
    });
});

describe('hasPlaintextSecret', () => {
    it.each([
        [{ password: 'p' }, true],
        [{ nested: { token: 't' } }, true],
        [{ password: 'ENC:v1:x' }, false],
        [{ password: MASK }, false],
        [{ password: '' }, false],
        [{ api_key_header: 'X-Key' }, false],
        [{ host: 'h' }, false],
        [null, false],
    ])('%j -> %s', (cfg, expected) => {
        expect(protector.hasPlaintextSecret(cfg)).toBe(expected);
    });
});

describe('InterfaceCredentialMigrator', () => {
    it('is skipped without APP_CREDENTIAL_KEY', async () => {
        const m = new InterfaceCredentialMigrator({ sequelize: {}, protector, env: {} });
        await expect(m.run()).resolves.toEqual({ scanned: 0, updated: 0, skipped: true });
    });

    it('plans encryption only for columns holding plaintext secrets', () => withKey(() => {
        const m = new InterfaceCredentialMigrator({ sequelize: {}, protector, env: { APP_CREDENTIAL_KEY: KEY } });
        const changes = m.planRow({
            id: 'i1',
            source_config: { host: 'h' },
            target_config: JSON.stringify({ url: 'u', password: 'p' }),
            source_connectivity: { type: 'sftp', config: { password: 'ENC:v1:already' } },
            target_connectivity: { type: 'http', config: { bearer_token: 't', endpoint: 'e' } },
        });
        expect(Object.keys(changes).sort()).toEqual(['target_config', 'target_connectivity']);
        expect(changes.target_config.password).toMatch(/^ENC:v1:/);
        expect(changes.target_config.url).toBe('u');
        expect(changes.target_connectivity.type).toBe('http');
        expect(changes.target_connectivity.config.bearer_token).toMatch(/^ENC:v1:/);
        expect(changes.target_connectivity.config.endpoint).toBe('e');
    }));

    it('updates only rows that need it and is idempotent', async () => {
        const prev = process.env.APP_CREDENTIAL_KEY;
        process.env.APP_CREDENTIAL_KEY = KEY;
        const updates = [];
        const sequelize = {
            QueryTypes: { SELECT: 'S', UPDATE: 'U' },
            query: jest.fn(async (sql, opts) => {
                if (opts.type === 'S') {
                    return [
                        { id: 'a', source_config: { password: 'p' }, target_config: {}, source_connectivity: null, target_connectivity: null },
                        { id: 'b', source_config: { host: 'h' }, target_config: {}, source_connectivity: null, target_connectivity: null },
                    ];
                }
                updates.push(opts.replacements);
                return [];
            }),
        };
        const m = new InterfaceCredentialMigrator({ sequelize, protector, logger: { log: jest.fn() }, env: { APP_CREDENTIAL_KEY: KEY } });
        const result = await m.run();
        expect(result).toEqual({ scanned: 2, updated: 1, skipped: false });
        expect(updates).toHaveLength(1);
        expect(updates[0].id).toBe('a');
        expect(JSON.parse(updates[0].source_config).password).toMatch(/^ENC:v1:/);
        if (prev === undefined) delete process.env.APP_CREDENTIAL_KEY; else process.env.APP_CREDENTIAL_KEY = prev;
    });
});
