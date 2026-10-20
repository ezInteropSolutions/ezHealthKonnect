// tests/unit/controllers/interfacesController.endpointAuth.test.js
//
// Partner-endpoint credentials (interfaces.source_config.endpoint_auth):
// masked on every read, never written from a wholesale sourceConfig, written
// only via the dedicated `endpointAuth` field with secrets encrypted at rest.
const controller = require('../../../controllers/interfacesController');
const {
    stripEndpointAuth,
    validateEndpointAuth,
    MASK_PLACEHOLDER,
} = controller;

// Masking of endpoint_auth (and every other interface config secret) is covered by
// tests/unit/services/InterfaceConfigProtector.test.js.

describe('stripEndpointAuth', () => {
    it('removes endpoint_auth so a wholesale sourceConfig write cannot carry it', () => {
        expect(stripEndpointAuth({ port: 1, endpoint_auth: { api_key: MASK_PLACEHOLDER } })).toEqual({ port: 1 });
    });
    it('passes through configs without it', () => {
        const cfg = { port: 1 };
        expect(stripEndpointAuth(cfg)).toBe(cfg);
        expect(stripEndpointAuth(undefined)).toBeUndefined();
    });
});

describe('validateEndpointAuth', () => {
    it.each([undefined, null])('accepts %p (leave unchanged / remove)', (v) => {
        expect(validateEndpointAuth(v)).toBeNull();
    });
    it.each(['api_key', 'basic_auth', 'bearer_token'])('accepts type %s', (t) => {
        expect(validateEndpointAuth({ authentication_type: t })).toBeNull();
    });
    it('rejects "none" — partner endpoints are secure by default', () => {
        expect(validateEndpointAuth({ authentication_type: 'none' })).toMatch(/must be one of/);
    });
    it('rejects unknown types, arrays and scalars', () => {
        expect(validateEndpointAuth({ authentication_type: 'magic' })).toBeTruthy();
        expect(validateEndpointAuth([])).toBeTruthy();
        expect(validateEndpointAuth('api_key')).toBeTruthy();
    });
});

describe('_applyEndpointAuth', () => {
    function fakeThis() {
        const calls = [];
        return {
            calls,
            database: {
                sequelize: {
                    QueryTypes: { UPDATE: 'UPDATE' },
                    query: jest.fn(async (sql, opts) => { calls.push({ sql, opts }); return []; }),
                },
            },
        };
    }
    const apply = (self, ...args) => controller._applyEndpointAuth.call(self, ...args);

    it('undefined restores the previously stored value (survives a wholesale sourceConfig write)', async () => {
        const self = fakeThis();
        const stored = { authentication_type: 'api_key', api_key: 'ENC:v1:stored' };
        await apply(self, 'iface-1', undefined, stored);
        expect(self.calls).toHaveLength(1);
        expect(self.calls[0].sql).toMatch(/jsonb_set/);
        expect(JSON.parse(self.calls[0].opts.replacements.auth)).toEqual(stored);
    });

    it('undefined with nothing stored removes the key (no-op on the data)', async () => {
        const self = fakeThis();
        await apply(self, 'iface-1', undefined, null);
        expect(self.calls[0].sql).toMatch(/- 'endpoint_auth'/);
    });

    it('null removes endpoint_auth', async () => {
        const self = fakeThis();
        await apply(self, 'iface-1', null, { authentication_type: 'api_key', api_key: 'x' });
        expect(self.calls[0].sql).toMatch(/- 'endpoint_auth'/);
    });

    it('a masked echo keeps the stored secret instead of writing the placeholder', async () => {
        const self = fakeThis();
        const stored = { authentication_type: 'api_key', api_key: 'ENC:v1:stored' };
        await apply(self, 'iface-1', { authentication_type: 'api_key', api_key: MASK_PLACEHOLDER }, stored);
        const written = JSON.parse(self.calls[0].opts.replacements.auth);
        expect(written.api_key).toBe('ENC:v1:stored');
        expect(JSON.stringify(written)).not.toContain(MASK_PLACEHOLDER);
    });

    it('a new secret is written (encrypted when APP_CREDENTIAL_KEY is configured)', async () => {
        const prev = process.env.APP_CREDENTIAL_KEY;
        process.env.APP_CREDENTIAL_KEY = Buffer.alloc(32, 7).toString('base64');
        try {
            const self = fakeThis();
            await apply(self, 'iface-1', { authentication_type: 'bearer_token', bearer_token: 'new-token' }, null);
            const written = JSON.parse(self.calls[0].opts.replacements.auth);
            expect(written.authentication_type).toBe('bearer_token');
            expect(written.bearer_token).toMatch(/^ENC:v1:/);
            expect(JSON.stringify(written)).not.toContain('new-token');
        } finally {
            if (prev === undefined) delete process.env.APP_CREDENTIAL_KEY; else process.env.APP_CREDENTIAL_KEY = prev;
        }
    });
});

describe('api_key_header stays readable and unencrypted', () => {
    it('is written in plaintext alongside an encrypted api_key', async () => {
        const prev = process.env.APP_CREDENTIAL_KEY;
        process.env.APP_CREDENTIAL_KEY = Buffer.alloc(32, 9).toString('base64');
        try {
            const calls = [];
            const self = { database: { sequelize: { QueryTypes: { UPDATE: 'U' }, query: async (sql, opts) => { calls.push(opts); } } } };
            await controller._applyEndpointAuth.call(self, 'i', { authentication_type: 'api_key', api_key_header: 'X-Partner-Key', api_key: 'k' }, null);
            const written = JSON.parse(calls[0].replacements.auth);
            expect(written.api_key_header).toBe('X-Partner-Key');
            expect(written.api_key).toMatch(/^ENC:v1:/);
        } finally {
            if (prev === undefined) delete process.env.APP_CREDENTIAL_KEY; else process.env.APP_CREDENTIAL_KEY = prev;
        }
    });
});
