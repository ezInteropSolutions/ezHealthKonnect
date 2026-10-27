// config/SecretsGuard.js
//
// Startup check for the secrets every deployment must set.
//
// Rules (2026-10-20 hardening — "no publicly-known secret may protect a real
// system"):
//   1. SESSION_SECRET and JWT_SECRET must be set, non-placeholder and at least
//      32 characters in EVERY environment except unit tests (NODE_ENV=test).
//      The code no longer has fallback values, so a missing secret must stop
//      startup rather than run on a guessable default.
//   2. No guarded variable may hold a value whose SHA-256 fingerprint is in
//      config/compromised_secrets.json — values that have been public (e.g.
//      committed to this public repository). Fatal in production; a loud
//      warning elsewhere, so an existing dev environment keeps working until
//      its owner rotates.
// The Go service applies rule 2 itself (services/secretguard) because
// standalone installs run it as its own process.
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');

const REQUIRED_SECRETS = ['SESSION_SECRET', 'JWT_SECRET'];
const MIN_SECRET_LENGTH = 32;
const DEFAULT_COMPROMISED_FILE = path.join(__dirname, 'compromised_secrets.json');

function sha256(value) {
    return crypto.createHash('sha256').update(value, 'utf8').digest('hex');
}

/**
 * Publicly-known secret fingerprints (config/compromised_secrets.json).
 */
class CompromisedSecretList {
    /**
     * @param {{guardedVariables: string[], fingerprints: {sha256: string, note: string}[]}} data
     */
    constructor(data) {
        this.guardedVariables = (data && data.guardedVariables) || [];
        this.byHash = new Map(((data && data.fingerprints) || []).map(f => [f.sha256, f.note]));
    }

    static load(file = DEFAULT_COMPROMISED_FILE) {
        return new CompromisedSecretList(JSON.parse(fs.readFileSync(file, 'utf8')));
    }

    /** Returns the note for a publicly-known value, or null. Case-insensitive for placeholders. */
    match(value) {
        const v = (value || '').trim();
        if (!v) return null;
        return this.byHash.get(sha256(v)) || this.byHash.get(sha256(v.toLowerCase())) || null;
    }
}

class SecretsGuard {
    /**
     * @param {object} env  environment map (process.env in production use)
     * @param {CompromisedSecretList} [compromised]  injected for tests
     */
    constructor(env, compromised) {
        this.env = env || {};
        this.compromised = compromised || CompromisedSecretList.load();
    }

    get nodeEnv() {
        return (this.env.NODE_ENV || '').toLowerCase();
    }

    get isProduction() {
        return this.nodeEnv === 'production';
    }

    get isUnitTest() {
        return this.nodeEnv === 'test';
    }

    /**
     * Returns { errors, warnings }.
     */
    evaluate() {
        const errors = [];
        const warnings = [];

        const requiredProblems = [];
        for (const name of REQUIRED_SECRETS) {
            const value = (this.env[name] || '').trim();
            if (!value) {
                requiredProblems.push(`${name} is not set`);
            } else if (value.length < MIN_SECRET_LENGTH) {
                requiredProblems.push(`${name} is shorter than ${MIN_SECRET_LENGTH} characters`);
            }
        }
        (this.isUnitTest ? warnings : errors).push(...requiredProblems);

        for (const name of this.compromised.guardedVariables) {
            const note = this.compromised.match(this.env[name]);
            if (note) {
                const msg = `${name} is a publicly-known value (${note}) — generate a new one`;
                (this.isProduction ? errors : warnings).push(msg);
            }
        }

        if (!(this.env.APP_CREDENTIAL_KEY || '').trim()) {
            warnings.push('APP_CREDENTIAL_KEY is not set — stored connector credentials are NOT encrypted (generate one with: openssl rand -base64 32, and never change it afterwards)');
        }
        const cookieSecure = (this.env.SESSION_COOKIE_SECURE || '').toLowerCase();
        if (this.isProduction && cookieSecure !== 'true' && cookieSecure !== 'auto') {
            warnings.push('SESSION_COOKIE_SECURE is not "true"/"auto" — session cookies are sent over plain HTTP. Serve the app over HTTPS (reverse proxy + TRUST_PROXY) and set SESSION_COOKIE_SECURE=auto');
        }
        return { errors, warnings };
    }

    /**
     * Logs warnings and throws when startup must not continue.
     * @param {{warn: Function, error: Function}} logger
     */
    enforce(logger = console) {
        const { errors, warnings } = this.evaluate();
        for (const w of warnings) logger.warn(`⚠️  SECURITY: ${w}`);
        if (errors.length > 0) {
            for (const e of errors) logger.error(`❌ SECURITY: ${e}`);
            throw new Error(`Refusing to start with insecure secrets: ${errors.join('; ')}`);
        }
    }
}

module.exports = SecretsGuard;
module.exports.CompromisedSecretList = CompromisedSecretList;
module.exports.sha256 = sha256;
