// services/InterfaceConfigProtector.js
//
// At-rest protection for an interface's own source_config / target_config
// (including source_config.endpoint_auth, the partner-endpoint credentials).
//
// Connector credentials inside pipeline STEP configs were already encrypted
// (pipelineController.encryptSensitiveConfigFields) and masked on read; the
// interface-level columns were not — the 2026-10-19 release regression found
// plaintext passwords there, newest from June 2026. This class applies the same
// field-level scheme to those columns:
//   - write: masked placeholders the browser echoed back are resolved to the
//     stored values, then sensitive-named fields are encrypted (ENC:v1:...)
//   - read:  sensitive-named fields are replaced with a fixed placeholder
// Field-level (not whole-document) encryption keeps every non-secret field
// (host, port, path, ...) readable for the consumers that only need those, and
// Go's CredentialStore.DecryptConfigFields transparently decrypts ENC:v1:
// values wherever a secret is actually used — plaintext values written before
// this existed still pass through unchanged.
//
// The key-name heuristic is the one shared with pipelineController.js and Go's
// isSensitiveKey(); a few names it matches are not secrets (an API key's HEADER
// NAME) and are kept readable so an engineer can still see/tell a partner which
// header to send.

const MASK_PLACEHOLDER = '••••••••';

// Names the shared heuristic flags that are NOT secret values.
const NON_SECRET_KEYS = new Set(['api_key_header', 'apikeyheader']);

class InterfaceConfigProtector {
    /**
     * @param {object} deps
     * @param {(key: string) => boolean} deps.isSensitiveKey   shared key-name heuristic
     * @param {(cfg: object) => object} deps.encryptFields     shared field encryptor (no-op without APP_CREDENTIAL_KEY)
     */
    constructor({ isSensitiveKey, encryptFields }) {
        this.isSensitiveKey = isSensitiveKey;
        this.encryptFields = encryptFields;
    }

    _isSecretKey(key) {
        return this.isSensitiveKey(key) && !NON_SECRET_KEYS.has(String(key).toLowerCase());
    }

    /**
     * Returns a copy of cfg with secret values replaced by the placeholder.
     */
    maskForDisplay(cfg) {
        if (!cfg || typeof cfg !== 'object') return cfg;
        if (Array.isArray(cfg)) return cfg.map(v => this.maskForDisplay(v));
        const out = {};
        for (const [key, value] of Object.entries(cfg)) {
            if (value && typeof value === 'object') {
                out[key] = this.maskForDisplay(value);
            } else if (typeof value === 'string' && value !== '' && this._isSecretKey(key)) {
                out[key] = MASK_PLACEHOLDER;
            } else {
                out[key] = value;
            }
        }
        return out;
    }

    /**
     * Prepares an incoming config for storage: placeholders resolved to the
     * values already stored (the user didn't change them), then secret values
     * encrypted. Non-secret fields — including names the shared heuristic
     * over-matches — are stored as plaintext.
     */
    protectForStorage(incoming, existing) {
        if (!incoming || typeof incoming !== 'object') return incoming;
        const resolved = this._resolvePlaceholders(incoming, existing);
        return this._restoreNonSecret(this.encryptFields(resolved), resolved);
    }

    /** True when cfg holds at least one non-empty secret value that is not yet encrypted. */
    hasPlaintextSecret(cfg) {
        if (!cfg || typeof cfg !== 'object') return false;
        if (Array.isArray(cfg)) return cfg.some(v => this.hasPlaintextSecret(v));
        return Object.entries(cfg).some(([key, value]) => {
            if (value && typeof value === 'object') return this.hasPlaintextSecret(value);
            return typeof value === 'string' && value !== '' && value !== MASK_PLACEHOLDER
                && !value.startsWith('ENC:v1:') && this._isSecretKey(key);
        });
    }

    _resolvePlaceholders(incoming, existing) {
        if (!incoming || typeof incoming !== 'object') return incoming;
        if (Array.isArray(incoming)) {
            const prev = Array.isArray(existing) ? existing : [];
            return incoming.map((v, i) => this._resolvePlaceholders(v, prev[i]));
        }
        const prev = existing && typeof existing === 'object' && !Array.isArray(existing) ? existing : {};
        const out = {};
        for (const [key, value] of Object.entries(incoming)) {
            if (value && typeof value === 'object') {
                out[key] = this._resolvePlaceholders(value, prev[key]);
            } else if (value === MASK_PLACEHOLDER && this.isSensitiveKey(key)) {
                out[key] = prev[key];
            } else {
                out[key] = value;
            }
        }
        return out;
    }

    // encryptFields uses the broader heuristic; put over-matched non-secret
    // names (e.g. api_key_header) back to their plaintext value.
    _restoreNonSecret(encrypted, plain) {
        if (!encrypted || typeof encrypted !== 'object' || !plain || typeof plain !== 'object') return encrypted;
        if (Array.isArray(encrypted)) return encrypted.map((v, i) => this._restoreNonSecret(v, plain[i]));
        const out = { ...encrypted };
        for (const [key, value] of Object.entries(plain)) {
            if (value && typeof value === 'object') {
                out[key] = this._restoreNonSecret(encrypted[key], value);
            } else if (NON_SECRET_KEYS.has(String(key).toLowerCase())) {
                out[key] = value;
            }
        }
        return out;
    }
}

InterfaceConfigProtector.MASK_PLACEHOLDER = MASK_PLACEHOLDER;

module.exports = InterfaceConfigProtector;

// Default instance wired to the shared credential crypto (services/credentialCrypto.js).
// Writers of interface source/target config use this one instance.
const { isSensitiveConfigKey, encryptSensitiveConfigFields } = require('./credentialCrypto');
InterfaceConfigProtector.default = new InterfaceConfigProtector({
    isSensitiveKey: isSensitiveConfigKey,
    encryptFields: encryptSensitiveConfigFields,
});
