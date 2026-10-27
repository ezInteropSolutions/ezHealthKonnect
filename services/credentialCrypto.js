// services/credentialCrypto.js
// Field-level credential encryption shared by every Node writer of
// credential-bearing config (pipeline steps, interface source/target config).
// Moved verbatim from controllers/pipelineController.js (2026-10-19) so services
// can use it without depending on a controller; pipelineController re-exports it.

const crypto = require('crypto');

// ── Credential encryption for step configs ─────────────────────────────────
// Uses the same APP_CREDENTIAL_KEY + AES-256-GCM algorithm as the Go
// CredentialStore so the Go executor can transparently decrypt on execution.
// Wire-format: nonce(12 bytes) || ciphertext(N bytes) || tag(16 bytes), base64-
// encoded, prefixed with 'ENC:v1:'.
//
// Sensitive keys are detected by substring (case-insensitive), matching the same
// logic as Go's isSensitiveKey(). This catches dbPassword, connectionString,
// secretAccessKey, apiKey, etc. without an exhaustive allowlist.

const SENSITIVE_KEY_SUBSTRINGS = [
    'password', 'passwd',
    'secret',
    'token',
    'passphrase',
    'apikey', 'api_key',
    'privatekey', 'private_key',
    'connectionstring', 'connection_string',
    'accesskey', 'access_key',
];

function isSensitiveConfigKey(key) {
    const lower = key.toLowerCase();
    return SENSITIVE_KEY_SUBSTRINGS.some(s => lower.includes(s));
}

/**
 * Encrypts sensitive field values anywhere in a step config object, including
 * inside nested objects and arrays of objects. Returns a deep-enough copy —
 * the original is not mutated (every object on a changed path gets a fresh
 * copy; unchanged scalar leaves are shared, same as Go's mirror-image
 * EncryptConfigFields/encryptMapRecursive in credential_store.go).
 *
 * Recursion matters for connector.outbound/connector.inbound steps: their
 * config shape is {connectorType, config: {host, password, ...}, contentField,
 * ...} — the actual connector credentials live one level down inside the
 * nested "config" object, not alongside "connectorType" at the top level. A
 * flat, top-level-only pass (the original implementation) never reaches them,
 * so passwords/keys saved through the pipeline builder UI were stored in
 * plaintext despite this function running on every save. Recursing generically
 * covers this and any other step type that nests a credential, without an
 * allowlist of "step types with nested config" that would go stale.
 *
 * Passthrough if APP_CREDENTIAL_KEY is not set (dev mode).
 * Idempotent: values already starting with 'ENC:v1:' are skipped.
 */
function encryptSensitiveConfigFields(config) {
    if (!config || typeof config !== 'object' || Array.isArray(config)) return config;

    const credKeyB64 = process.env.APP_CREDENTIAL_KEY;
    if (!credKeyB64) return config; // passthrough in dev mode

    let credKey;
    try {
        credKey = Buffer.from(credKeyB64, 'base64');
        if (credKey.length !== 32) return config; // wrong key length — passthrough
    } catch (e) {
        return config;
    }

    function encryptString(key, value) {
        if (!isSensitiveConfigKey(key) || value === '' || value.startsWith('ENC:v1:')) return value;

        // AES-256-GCM: layout must match Go's gcm.Seal output:
        //   nonce(12) || ciphertext(N) || tag(16)  →  base64  →  'ENC:v1:' prefix
        const nonce = crypto.randomBytes(12);
        const cipher = crypto.createCipheriv('aes-256-gcm', credKey, nonce);
        const enc = Buffer.concat([cipher.update(value, 'utf8'), cipher.final()]);
        const tag = cipher.getAuthTag(); // 16 bytes
        return 'ENC:v1:' + Buffer.concat([nonce, enc, tag]).toString('base64');
    }

    function encryptValue(key, value) {
        if (Array.isArray(value)) return value.map(item => encryptValue(key, item));
        if (value && typeof value === 'object') return encryptObject(value);
        if (typeof value === 'string') return encryptString(key, value);
        return value;
    }

    function encryptObject(obj) {
        const result = Object.assign({}, obj);
        for (const [k, v] of Object.entries(result)) {
            result[k] = encryptValue(k, v);
        }
        return result;
    }

    return encryptObject(config);
}

module.exports = { isSensitiveConfigKey, encryptSensitiveConfigFields, SENSITIVE_KEY_SUBSTRINGS };
