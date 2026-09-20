// services/settingsService.js
// Lightweight Node.js settings reader — queries system_settings directly from
// PostgreSQL with a 5-minute in-process cache.  No Sequelize dependency.
//
// Usage:
//   const settingsService = require('./settingsService');
//   const sec = await settingsService.getSecuritySettings();
//   console.log(sec.jwt_expiry_hours);  // → 24

'use strict';

const { Pool } = require('pg');

// ─── DB pool ──────────────────────────────────────────────────────────────────
// Uses a tiny dedicated pool (max 2) so we don't compete with the main app pool.
let _pool = null;
function getPool() {
    if (!_pool) {
        _pool = new Pool({
            host:     process.env.DB_HOST     || 'localhost',
            port:     parseInt(process.env.DB_PORT || '5432', 10),
            database: process.env.DB_NAME     || 'ezhealthkonnect',
            user:     process.env.DB_USER     || 'postgres',
            password: process.env.DB_PASSWORD || '',
            max: 2,
            idleTimeoutMillis: 30000,
            connectionTimeoutMillis: 3000,
        });
        _pool.on('error', (err) => {
            console.warn('[settingsService] pg pool error:', err.message);
        });
    }
    return _pool;
}

// ─── In-process cache ─────────────────────────────────────────────────────────
const CACHE_TTL_MS = 5 * 60 * 1000; // 5 minutes
const _cache = {};

async function getSetting(key, defaultValue = {}) {
    const cached = _cache[key];
    if (cached && Date.now() - cached.ts < CACHE_TTL_MS) {
        return cached.value;
    }
    try {
        const result = await getPool().query(
            'SELECT value FROM system_settings WHERE key = $1',
            [key]
        );
        const value = result.rows.length > 0 ? result.rows[0].value : defaultValue;
        _cache[key] = { value, ts: Date.now() };
        return value;
    } catch (err) {
        console.warn(`[settingsService] Failed to read setting "${key}":`, err.message);
        // Return cached stale value if available, otherwise default
        return (cached && cached.value) || defaultValue;
    }
}

function invalidate(key) {
    delete _cache[key];
}

// ─── Typed accessors ──────────────────────────────────────────────────────────

const SECURITY_DEFAULTS = {
    session_timeout_minutes:  1440,
    jwt_expiry_hours:         24,
    max_login_attempts:       5,
    lockout_duration_minutes: 30,
    password_min_length:      8,
};

/**
 * Returns security settings merged with defaults.
 * @returns {Promise<typeof SECURITY_DEFAULTS>}
 */
async function getSecuritySettings() {
    const raw = await getSetting('security', SECURITY_DEFAULTS);
    return { ...SECURITY_DEFAULTS, ...raw };
}

const ATNA_SYSLOG_DEFAULTS = {
    enabled: false,
    host: '',
    port: 514,
    protocol: 'udp',
    facility: 10,
    app_name: 'ezHealthKonnect',
    audit_source_id: 'ezHealthKonnect',
    enterprise_site_id: '',
    tls_insecure_skip_verify: false,
};

/**
 * Returns ATNA syslog export settings merged with defaults — the Node-side
 * mirror of services.AppSettingsCache.GetATNASyslogSettings() (Go). Used by
 * services/audit/atnaSyslogExporter.js so an admin enabling/reconfiguring
 * ATNA export via the settings UI takes effect for Node-emitted audit
 * events too, not just Go-emitted ones, without a process restart (within
 * this cache's own 5-minute TTL — the same lag every other settings
 * category here already accepts, per this file's own header).
 * @returns {Promise<typeof ATNA_SYSLOG_DEFAULTS>}
 */
async function getATNASyslogSettings() {
    const raw = await getSetting('atna_syslog', ATNA_SYSLOG_DEFAULTS);
    return { ...ATNA_SYSLOG_DEFAULTS, ...raw };
}

module.exports = { getSetting, getSecuritySettings, getATNASyslogSettings, invalidate };
