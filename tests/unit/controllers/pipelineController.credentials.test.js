// tests/unit/controllers/pipelineController.credentials.test.js
//
// encryptSensitiveConfigFields() is the JS half of the credential-encryption
// mechanism whose Go half lives in services/credential_store.go
// (EncryptConfigFields/DecryptConfigFields). It used to only walk the TOP
// LEVEL of a step's config object -- for connector.outbound/connector.inbound
// steps, whose real shape is {connectorType, config: {host, password, ...},
// contentField, ...}, the actual credentials live one level down inside the
// nested "config" object, so they were never encrypted despite this function
// running on every pipeline save. These tests cover the fixed, recursive
// version -- both the previously-working flat case and the newly-fixed
// nested case, plus arrays of objects for full generality.
const { encryptSensitiveConfigFields, isSensitiveConfigKey } = require('../../../controllers/pipelineController');

const TEST_KEY = Buffer.alloc(32, 7).toString('base64'); // deterministic 32-byte key for tests

describe('isSensitiveConfigKey', () => {
    it.each(['password', 'dbPassword', 'db_password', 'secretAccessKey', 'apiKey', 'api_key', 'token', 'connectionString', 'privateKey'])(
        'flags %s as sensitive', (key) => {
            expect(isSensitiveConfigKey(key)).toBe(true);
        }
    );

    it.each(['host', 'port', 'database', 'schema', 'tableName', 'writeMode'])(
        'does not flag %s as sensitive', (key) => {
            expect(isSensitiveConfigKey(key)).toBe(false);
        }
    );
});

describe('encryptSensitiveConfigFields', () => {
    const ORIGINAL_ENV = process.env.APP_CREDENTIAL_KEY;

    afterEach(() => {
        process.env.APP_CREDENTIAL_KEY = ORIGINAL_ENV;
    });

    it('passes through unchanged when APP_CREDENTIAL_KEY is unset (dev mode)', () => {
        delete process.env.APP_CREDENTIAL_KEY;
        const input = { password: 'plaintext' };
        expect(encryptSensitiveConfigFields(input)).toEqual(input);
    });

    it('encrypts a top-level sensitive field (flat, legacy-supported shape)', () => {
        process.env.APP_CREDENTIAL_KEY = TEST_KEY;
        const result = encryptSensitiveConfigFields({ host: 'db.local', password: 'super-secret' });

        expect(result.host).toBe('db.local');
        expect(result.password).toMatch(/^ENC:v1:/);
        expect(result.password).not.toBe('super-secret');
    });

    it('encrypts a credential nested one level down — the connector.outbound/inbound real shape', () => {
        process.env.APP_CREDENTIAL_KEY = TEST_KEY;
        const stepConfig = {
            connectorType: 'mysql_outbound',
            config: {
                host: 'mysql.internal',
                port: 3306,
                username: 'app_user',
                password: 'sup3r-secret-db-pass',
            },
            contentField: 'message',
            contentType: 'application/json',
        };

        const result = encryptSensitiveConfigFields(stepConfig);

        // Non-sensitive fields, at every level, are untouched.
        expect(result.connectorType).toBe('mysql_outbound');
        expect(result.config.host).toBe('mysql.internal');
        expect(result.config.port).toBe(3306);
        expect(result.config.username).toBe('app_user');
        expect(result.contentField).toBe('message');

        // The nested password is now encrypted -- this is the actual bug fix.
        expect(result.config.password).toMatch(/^ENC:v1:/);
        expect(result.config.password).not.toBe('sup3r-secret-db-pass');
    });

    it('encrypts credentials inside an array of objects', () => {
        process.env.APP_CREDENTIAL_KEY = TEST_KEY;
        const input = {
            connectors: [
                { name: 'primary', token: 'token-one' },
                { name: 'secondary', token: 'token-two' },
            ],
        };

        const result = encryptSensitiveConfigFields(input);

        expect(result.connectors[0].name).toBe('primary');
        expect(result.connectors[0].token).toMatch(/^ENC:v1:/);
        expect(result.connectors[1].token).toMatch(/^ENC:v1:/);
        expect(result.connectors[0].token).not.toBe(result.connectors[1].token); // distinct nonces
    });

    it('does not mutate the original input object (nested included)', () => {
        process.env.APP_CREDENTIAL_KEY = TEST_KEY;
        const original = { config: { password: 'still-plaintext' } };
        const snapshotBefore = JSON.parse(JSON.stringify(original));

        encryptSensitiveConfigFields(original);

        expect(original).toEqual(snapshotBefore);
    });

    it('is idempotent — encrypting an already-encrypted nested value does not double-encrypt', () => {
        process.env.APP_CREDENTIAL_KEY = TEST_KEY;
        const once = encryptSensitiveConfigFields({ config: { password: 'plaintext' } });
        const twice = encryptSensitiveConfigFields(once);

        expect(twice.config.password).toBe(once.config.password);
    });

    it('skips empty-string and non-string sensitive values, nested or not', () => {
        process.env.APP_CREDENTIAL_KEY = TEST_KEY;
        const result = encryptSensitiveConfigFields({
            password: '',
            config: { apiKey: 12345, token: null },
        });

        expect(result.password).toBe('');
        expect(result.config.apiKey).toBe(12345);
        expect(result.config.token).toBeNull();
    });
});
