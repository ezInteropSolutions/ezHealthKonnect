// config/SecretsGuard.js
//
// Startup check for the secrets every deployment must set. Several code paths
// historically fall back to publicly-known placeholder values when a secret is
// unset (app.js session secret 'your-session-secret', legacy /api/login and
// config/environment.js JWT 'your-secret-key-change-this-in-production').
// The installers always generate real values, but a hand-configured production
// deployment would silently run on guessable secrets — so in production the
// server refuses to start instead (2026-10-19 release hardening).

const KNOWN_PLACEHOLDERS = new Set([
    'your-session-secret',
    'your-secret-key-change-this-in-production',
    'change-me',
    'changeme',
    'secret',
]);

const MIN_SECRET_LENGTH = 32;

class SecretsGuard {
    /**
     * @param {object} env  environment map (process.env in production use)
     */
    constructor(env) {
        this.env = env || {};
    }

    get isProduction() {
        return (this.env.NODE_ENV || '').toLowerCase() === 'production';
    }

    /**
     * Returns { errors, warnings }. Errors are fatal in production; in other
     * environments they are reported as warnings so local development keeps
     * working with dev defaults.
     */
    evaluate() {
        const problems = [];
        for (const name of ['SESSION_SECRET', 'JWT_SECRET']) {
            const value = (this.env[name] || '').trim();
            if (!value) {
                problems.push(`${name} is not set`);
            } else if (KNOWN_PLACEHOLDERS.has(value.toLowerCase())) {
                problems.push(`${name} is a publicly-known placeholder value`);
            } else if (value.length < MIN_SECRET_LENGTH) {
                problems.push(`${name} is shorter than ${MIN_SECRET_LENGTH} characters`);
            }
        }

        const warnings = [];
        if (!(this.env.APP_CREDENTIAL_KEY || '').trim()) {
            warnings.push('APP_CREDENTIAL_KEY is not set — stored connector credentials are NOT encrypted (generate one with: openssl rand -base64 32, and never change it afterwards)');
        }
        const cookieSecure = (this.env.SESSION_COOKIE_SECURE || '').toLowerCase();
        if (this.isProduction && cookieSecure !== 'true' && cookieSecure !== 'auto') {
            warnings.push('SESSION_COOKIE_SECURE is not "true"/"auto" — session cookies are sent over plain HTTP. Serve the app over HTTPS (reverse proxy + TRUST_PROXY) and set SESSION_COOKIE_SECURE=auto');
        }

        return this.isProduction
            ? { errors: problems, warnings }
            : { errors: [], warnings: problems.concat(warnings) };
    }

    /**
     * Logs warnings and throws when a production deployment has fatal problems.
     * @param {{warn: Function, error: Function}} logger
     */
    enforce(logger = console) {
        const { errors, warnings } = this.evaluate();
        for (const w of warnings) logger.warn(`⚠️  SECURITY: ${w}`);
        if (errors.length > 0) {
            for (const e of errors) logger.error(`❌ SECURITY: ${e}`);
            throw new Error(`Refusing to start in production with insecure secrets: ${errors.join('; ')}`);
        }
    }
}

module.exports = SecretsGuard;
