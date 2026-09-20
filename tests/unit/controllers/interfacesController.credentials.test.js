// tests/unit/controllers/interfacesController.credentials.test.js
//
// interfacesController.js's _updateStep used to write connector credentials
// (passwords, API keys) to transformation_steps.config in PLAINTEXT — unlike
// pipelineController.js's savePipeline, which already encrypted the same
// column via encryptSensitiveConfigFields (see
// pipelineController.credentials.test.js). The fix added a mask-on-read /
// resolve-mask-on-write / encrypt-on-write cycle:
//   - maskSensitiveFields() replaces real credential values with a fixed
//     placeholder before a connector config is ever sent to the browser
//     (getInterface/getAllInterfaces).
//   - resolveMaskedFields() is the write-side counterpart: when the browser
//     echoes the placeholder back unchanged (meaning the user never touched
//     that field), it substitutes the REAL value already on file instead of
//     letting the placeholder itself get written back as "the new secret".
//   - containsSensitiveField() decides whether an update actually touches a
//     real credential (for the distinct INTERFACE_CREDENTIALS_UPDATED audit
//     event), and must NOT fire on a masked-but-unchanged round trip.
//
// These three functions were verified live (real HTTP round trips against a
// running app + real Postgres + real AES-256-GCM decryption) during that
// same session, including chasing down a false-alarm ciphertext mismatch
// that turned out to be a shell-escaping bug in the *manual test*, not in
// this code — see the project's own audit-logger-phase4-credential-encryption
// memory for the full story. These tests are the checked-in, repeatable
// version of that verification, so a future refactor can't silently
// reintroduce either the plaintext-storage bug or the false-positive-audit-
// event bug without a test failing.
const {
    containsSensitiveField,
    maskSensitiveFields,
    resolveMaskedFields,
    MASK_PLACEHOLDER,
} = require('../../../controllers/interfacesController');

describe('MASK_PLACEHOLDER', () => {
    it('matches credential_store.go\'s own MaskSensitiveFields placeholder exactly', () => {
        expect(MASK_PLACEHOLDER).toBe('••••••••');
    });
});

describe('containsSensitiveField', () => {
    it('returns true for a top-level real credential value', () => {
        expect(containsSensitiveField({ apiKey: 'real-secret' })).toBe(true);
    });

    it('returns true for a credential nested one level down (the real connector-config shape)', () => {
        expect(containsSensitiveField({
            connectorType: 'http_outbound',
            config: { endpoint: 'http://example.test', apiKey: 'real-secret' },
        })).toBe(true);
    });

    it('returns true for a credential inside an array of objects', () => {
        expect(containsSensitiveField({ connectors: [{ name: 'a' }, { token: 'real-token' }] })).toBe(true);
    });

    it('returns false when no sensitive-keyed field is present', () => {
        expect(containsSensitiveField({ connectorType: 'http_outbound', config: { endpoint: 'http://example.test' } })).toBe(false);
    });

    it('returns false for an empty-string credential value', () => {
        expect(containsSensitiveField({ apiKey: '' })).toBe(false);
    });

    it('returns false when the credential value is exactly MASK_PLACEHOLDER — the critical false-positive guard', () => {
        // Without this exclusion, every save of every interface with a
        // configured credential would wrongly fire INTERFACE_CREDENTIALS_UPDATED,
        // since the browser always echoes the placeholder back for a field
        // the user never touched.
        expect(containsSensitiveField({ apiKey: MASK_PLACEHOLDER })).toBe(false);
    });

    it('returns false when the masked placeholder appears nested one level down', () => {
        expect(containsSensitiveField({
            connectorType: 'http_outbound',
            config: { endpoint: 'http://example.test', apiKey: MASK_PLACEHOLDER },
        })).toBe(false);
    });

    it('handles null/non-object input gracefully', () => {
        expect(containsSensitiveField(null)).toBe(false);
        expect(containsSensitiveField(undefined)).toBe(false);
        expect(containsSensitiveField('a string')).toBe(false);
    });
});

describe('maskSensitiveFields', () => {
    it('masks a top-level sensitive field', () => {
        const result = maskSensitiveFields({ endpoint: 'http://example.test', apiKey: 'real-secret' });
        expect(result.endpoint).toBe('http://example.test');
        expect(result.apiKey).toBe(MASK_PLACEHOLDER);
    });

    it('masks a sensitive field nested one level down', () => {
        const result = maskSensitiveFields({
            connectorType: 'http_outbound',
            config: { endpoint: 'http://example.test', apiKey: 'real-secret' },
        });
        expect(result.connectorType).toBe('http_outbound');
        expect(result.config.endpoint).toBe('http://example.test');
        expect(result.config.apiKey).toBe(MASK_PLACEHOLDER);
    });

    it('leaves an empty-string credential value unmasked', () => {
        const result = maskSensitiveFields({ apiKey: '' });
        expect(result.apiKey).toBe('');
    });

    it('masks credentials inside an array of objects', () => {
        const result = maskSensitiveFields({ connectors: [{ name: 'a', token: 'tok-1' }, { name: 'b', token: 'tok-2' }] });
        expect(result.connectors[0].name).toBe('a');
        expect(result.connectors[0].token).toBe(MASK_PLACEHOLDER);
        expect(result.connectors[1].token).toBe(MASK_PLACEHOLDER);
    });

    it('does not mutate the original input object', () => {
        const original = { config: { apiKey: 'real-secret' } };
        const snapshotBefore = JSON.parse(JSON.stringify(original));
        maskSensitiveFields(original);
        expect(original).toEqual(snapshotBefore);
    });

    it('handles null/non-object input by returning it unchanged', () => {
        expect(maskSensitiveFields(null)).toBeNull();
        expect(maskSensitiveFields(undefined)).toBeUndefined();
    });
});

describe('resolveMaskedFields', () => {
    it('substitutes the stored value when incoming is the mask placeholder for a sensitive key', () => {
        const incoming = { apiKey: MASK_PLACEHOLDER };
        const existing = { apiKey: 'ENC:v1:realCiphertext==' };
        expect(resolveMaskedFields(incoming, existing).apiKey).toBe('ENC:v1:realCiphertext==');
    });

    it('passes through a genuinely new credential value unchanged, even though the key is sensitive', () => {
        const incoming = { apiKey: 'BRAND-NEW-SECRET' };
        const existing = { apiKey: 'ENC:v1:oldCiphertext==' };
        expect(resolveMaskedFields(incoming, existing).apiKey).toBe('BRAND-NEW-SECRET');
    });

    it('passes through a deliberate empty string (clearing the credential) rather than restoring the old value', () => {
        const incoming = { apiKey: '' };
        const existing = { apiKey: 'ENC:v1:oldCiphertext==' };
        expect(resolveMaskedFields(incoming, existing).apiKey).toBe('');
    });

    it('does not substitute a non-sensitive field even if it literally equals the placeholder text', () => {
        // A user is free to name a host "••••••••" — resolveMaskedFields must
        // only special-case sensitive-keyed fields, not any field that
        // happens to match the placeholder string.
        const incoming = { hostLabel: MASK_PLACEHOLDER };
        const existing = { hostLabel: 'some-other-label' };
        expect(resolveMaskedFields(incoming, existing).hostLabel).toBe(MASK_PLACEHOLDER);
    });

    it('resolves nested (one level down) masked fields against the matching nested existing value', () => {
        const incoming = {
            connectorType: 'http_outbound',
            config: { endpoint: 'http://example.test/v2', apiKey: MASK_PLACEHOLDER },
        };
        const existing = {
            connectorType: 'http_outbound',
            config: { endpoint: 'http://example.test/v1', apiKey: 'ENC:v1:realCiphertext==' },
        };
        const resolved = resolveMaskedFields(incoming, existing);
        expect(resolved.config.endpoint).toBe('http://example.test/v2'); // new value wins
        expect(resolved.config.apiKey).toBe('ENC:v1:realCiphertext=='); // masked field restored
    });

    it('resolves masked fields inside an array of objects, matched by index', () => {
        const incoming = { connectors: [{ token: MASK_PLACEHOLDER }, { token: 'NEW-TOKEN' }] };
        const existing = { connectors: [{ token: 'ENC:v1:old1==' }, { token: 'ENC:v1:old2==' }] };
        const resolved = resolveMaskedFields(incoming, existing);
        expect(resolved.connectors[0].token).toBe('ENC:v1:old1==');
        expect(resolved.connectors[1].token).toBe('NEW-TOKEN');
    });

    it('resolves to undefined when a masked field has no corresponding existing value', () => {
        // Documents actual behavior for a step whose stored config never had
        // this field before — not a crash, but also not a fabricated value.
        const incoming = { apiKey: MASK_PLACEHOLDER };
        const existing = {};
        expect(resolveMaskedFields(incoming, existing).apiKey).toBeUndefined();
    });

    it('treats a missing/non-object existing as {} without throwing', () => {
        expect(() => resolveMaskedFields({ apiKey: MASK_PLACEHOLDER }, undefined)).not.toThrow();
        expect(() => resolveMaskedFields({ apiKey: MASK_PLACEHOLDER }, null)).not.toThrow();
    });

    it('handles non-object incoming input by returning it unchanged', () => {
        expect(resolveMaskedFields('a string', {})).toBe('a string');
        expect(resolveMaskedFields(null, {})).toBeNull();
    });
});

describe('mask → resolve round trip (the actual security property)', () => {
    it('a masked-and-unchanged save reproduces the exact original stored credential', () => {
        const storedConfig = {
            connectorType: 'http_outbound',
            config: { endpoint: 'http://example.test/api', apiKey: 'ENC:v1:vNihxCMw4R0n8rfomPP44ntfPvuNF0IrhwSiaionIK8hKeaZ8Cls9n0UBLyFfjITCBZj' },
        };

        // Simulates GET: what the browser actually receives.
        const maskedForBrowser = maskSensitiveFields(storedConfig);
        expect(maskedForBrowser.config.apiKey).toBe(MASK_PLACEHOLDER);

        // Simulates the browser submitting a routine edit (new endpoint) with
        // the masked apiKey echoed back unchanged, exactly as a real <input>
        // pre-filled with the placeholder and never touched would submit.
        const submitted = {
            connectorType: 'http_outbound',
            config: { endpoint: 'http://example.test/api-v2', apiKey: maskedForBrowser.config.apiKey },
        };

        const resolved = resolveMaskedFields(submitted, storedConfig);

        // The real credential is recovered byte-for-byte — this is the exact
        // property a shell-escaping bug in a manual test once appeared to
        // violate (see the phase4 memory) before being proven a test-harness
        // artifact, not a real bug. This test pins the real, correct behavior.
        expect(resolved.config.apiKey).toBe(storedConfig.config.apiKey);
        expect(resolved.config.endpoint).toBe('http://example.test/api-v2');

        // And the credential-change audit event must NOT fire for this submission.
        expect(containsSensitiveField(submitted)).toBe(false);
    });

    it('a genuine credential change is both resolved correctly and detected for auditing', () => {
        const storedConfig = { connectorType: 'http_outbound', config: { apiKey: 'ENC:v1:oldCiphertext==' } };
        const submitted = { connectorType: 'http_outbound', config: { apiKey: 'FRESH-REAL-SECRET' } };

        const resolved = resolveMaskedFields(submitted, storedConfig);
        expect(resolved.config.apiKey).toBe('FRESH-REAL-SECRET');
        expect(containsSensitiveField(submitted)).toBe(true);
    });

    it('an unrelated field-only edit resolves the credential unchanged and does not trip the audit event', () => {
        const storedConfig = { connectorType: 'http_outbound', config: { endpoint: 'http://example.test/api', apiKey: 'ENC:v1:realCiphertext==' } };
        const maskedForBrowser = maskSensitiveFields(storedConfig);
        const submitted = { connectorType: 'http_outbound', config: { endpoint: 'http://example.test/api', apiKey: maskedForBrowser.config.apiKey } };

        const resolved = resolveMaskedFields(submitted, storedConfig);
        expect(resolved.config.apiKey).toBe(storedConfig.config.apiKey);
        expect(containsSensitiveField(submitted)).toBe(false);
    });
});
