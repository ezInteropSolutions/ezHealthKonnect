// middleware/setupCheck.js
// Gates access to the app until first-run setup is complete.
//
// Bypass list (always allowed):
//   GET  /setup.html
//   Any  /api/setup/*
//   Any  static asset (.js, .css, .png, .ico, .woff*, .svg, .map, .json in /public)
//
// When setup is required:
//   HTML page requests → 302 redirect to /setup.html
//   API requests       → 503 { setupRequired: true }

'use strict';

const { isSetupRequired } = require('../controllers/setupController');

// Cache to avoid a DB hit on every single request.
// Invalidated immediately after setup completes (the controller clears settingsService cache,
// and this module re-reads on the next request after CACHE_TTL_MS).
const CACHE_TTL_MS = 5_000; // 5 s
let _cachedRequired = null;
let _cachedAt       = 0;

async function getSetupRequired() {
    const now = Date.now();
    if (_cachedRequired !== null && now - _cachedAt < CACHE_TTL_MS) {
        return _cachedRequired;
    }
    _cachedRequired = await isSetupRequired();
    _cachedAt       = now;
    return _cachedRequired;
}

// Call this after /api/setup/complete succeeds so the cache clears immediately
function invalidateSetupCache() {
    _cachedRequired = false;
    _cachedAt       = Date.now();
}

// Paths that must always be reachable before setup
const BYPASS_PREFIXES = [
    '/api/setup',
    '/setup.html'
];

// Static asset extensions — let express.static handle these normally
const STATIC_EXTS = /\.(js|css|map|png|jpg|jpeg|ico|svg|woff|woff2|ttf|eot|json)$/i;

function isBypassed(url) {
    // Strip query string for comparison
    const path = url.split('?')[0];

    for (const prefix of BYPASS_PREFIXES) {
        if (path === prefix || path.startsWith(prefix + '/')) return true;
    }

    // Static assets
    if (STATIC_EXTS.test(path)) return true;

    return false;
}

async function setupCheckMiddleware(req, res, next) {
    if (isBypassed(req.originalUrl)) return next();

    try {
        const required = await getSetupRequired();
        if (!required) return next();

        // Setup is required — gate the request
        const acceptsHtml = (req.headers.accept || '').includes('text/html');
        if (acceptsHtml && req.method === 'GET') {
            return res.redirect('/setup.html');
        }

        // API / non-HTML request
        return res.status(503).json({
            error:        'setup_required',
            setupRequired: true,
            message:      'First-run setup is not yet complete. Please visit /setup.html'
        });

    } catch (err) {
        console.error('[setupCheck] middleware error:', err.message);
        // On error, let the request through (fail open — don't block the app)
        next();
    }
}

module.exports = setupCheckMiddleware;
module.exports.invalidateSetupCache = invalidateSetupCache;
