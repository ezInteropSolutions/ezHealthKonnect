// tests/unit/scripts/fingerprintSecret.test.js
const fs = require('fs');
const os = require('os');
const path = require('path');
const { CompromisedSecretRegistry } = require('../../../scripts/fingerprint-secret');
const SecretsGuard = require('../../../config/SecretsGuard');

function tempList() {
    const file = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'fp-')), 'list.json');
    fs.writeFileSync(file, JSON.stringify({ guardedVariables: ['JWT_SECRET'], fingerprints: [] }));
    return file;
}

describe('fingerprint-secret', () => {
    it('stores only the hash, and the guard then refuses the value in production', () => {
        const file = tempList();
        const leaked = 'leaked-value-' + 'q'.repeat(40);
        const registry = new CompromisedSecretRegistry(file);
        expect(registry.add(`${leaked}\n`, 'test leak')).toBe(true);
        registry.save();

        const text = fs.readFileSync(file, 'utf8');
        expect(text).not.toContain(leaked);

        const list = SecretsGuard.CompromisedSecretList.load(file);
        const guard = new SecretsGuard({ NODE_ENV: 'production', SESSION_SECRET: 's'.repeat(48), JWT_SECRET: leaked,
            APP_CREDENTIAL_KEY: Buffer.alloc(32, 7).toString('base64'), SESSION_COOKIE_SECURE: 'auto' }, list);
        expect(() => guard.enforce({ warn: () => {}, error: () => {} })).toThrow(/JWT_SECRET/);
    });

    it('does not duplicate an existing fingerprint', () => {
        const registry = new CompromisedSecretRegistry(tempList());
        expect(registry.add('abc-value-123', 'first')).toBe(true);
        expect(registry.add('  abc-value-123  ', 'again')).toBe(false);
        expect(registry.data.fingerprints).toHaveLength(1);
    });

    it('rejects an empty value or a missing note', () => {
        const registry = new CompromisedSecretRegistry(tempList());
        expect(() => registry.add('   ', 'note')).toThrow(/empty/);
        expect(() => registry.add('value-1234', '')).toThrow(/note/);
    });
});
