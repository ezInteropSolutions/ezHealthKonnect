// tests/unit/app.forwardToGoAuth.test.js
//
// General regression guard: EVERY route app.js forwards to the Go backend must
// be registered with an authentication middleware, except a short, explicit,
// reasoned allow-list.
//
// Why a general rule (not one test per route): app.connectivityAuth.test.js
// guarded the single route a previous pass fixed, and the same gap survived on
// a dozen others. The 2026-10-19 release regression found anonymous callers
// could deactivate interfaces, delete CDA documents/code templates/schema
// packages, write pipeline code templates, read CDA document metadata
// (patient id/name), browse the server filesystem and execute partner
// pipelines — all through bare `app.use('/api/x', forwardToGo)` lines.
//
// Static source check, same approach as app.connectivityAuth.test.js (requiring
// app.js has real startup side effects).
const fs = require('fs');
const path = require('path');

const APP_JS = fs.readFileSync(path.join(__dirname, '../../app.js'), 'utf8');

// Middleware names in app.js that authenticate the caller.
const AUTH_MIDDLEWARE = ['_verifyToken', '_analyticsAuth', '_isAuth', '_requireAdminUser',
    '_requireSuperAdminUser', '_requireDLQAccess', '_requireAdminAuth'];

// method + path -> reason. Each entry is a deliberate decision.
const ALLOWED_WITHOUT_APP_AUTH = {
    'get /api/system/health': 'liveness probe',
    'use /api/eligibility': 'partner endpoint — per-interface auth in Go (requireSyncEndpointAuth)',
    'use /api/claim-status': 'partner endpoint — per-interface auth in Go (requireSyncEndpointAuth)',
    'use /api/prior-auth': 'partner endpoint — per-interface auth in Go (requireSyncEndpointAuth)',
    'use /api/pharmacy-claim': 'partner endpoint — per-interface auth in Go (requireSyncEndpointAuth)',
};

// Role middleware (authorization, not just authentication). The 2026-10-19
// authorization probe found a no-permission user could deactivate interfaces
// and write pipeline code templates through login-only Go routes.
const ROLE_MIDDLEWARE = ['_goRoleGate', '_aiReadGate', '_requireAdminUser', '_requireSuperAdminUser', '_requireDLQAccess', '_requireAdminAuth'];

// Authenticated routes deliberately open to every role — each a decision.
const ALLOWED_WITHOUT_ROLE_CHECK = {};

function forwardToGoRegistrations() {
    const re = /app\.(use|get|post|put|patch|delete)\(\s*['"]([^'"]+)['"]\s*,([^\n]*?)forwardToGo\s*\)/g;
    const out = [];
    let m;
    while ((m = re.exec(APP_JS)) !== null) {
        out.push({ method: m[1], route: m[2], middleware: m[3] });
    }
    return out;
}

describe('every forwardToGo route is authenticated (app.js)', () => {
    const registrations = forwardToGoRegistrations();

    it('finds the forwardToGo registrations (sanity check on the parser)', () => {
        expect(registrations.length).toBeGreaterThan(30);
    });

    it.each(forwardToGoRegistrations().map(r => [`${r.method} ${r.route}`, r]))(
        '%s has an auth middleware or an allow-listed reason',
        (key, r) => {
            const allowed = ALLOWED_WITHOUT_APP_AUTH[key];
            const hasAuth = AUTH_MIDDLEWARE.some(name => new RegExp(`\\b${name}\\b`).test(r.middleware));
            if (allowed) {
                // Allow-listed partner endpoints must still go through the
                // partner middleware (credential forwarding + optional app user).
                if (r.route !== '/api/system/health') {
                    expect(r.middleware).toMatch(/\b_partnerEndpoint\b/);
                }
                return;
            }
            expect(hasAuth).toBe(true);
            if (!ALLOWED_WITHOUT_ROLE_CHECK[key]) {
                const hasRole = ROLE_MIDDLEWARE.some(name => new RegExp(`\\b${name}\\b`).test(r.middleware));
                expect(hasRole).toBe(true);
            }
        }
    );

    it('every allow-list entry still exists (no stale exemptions)', () => {
        const keys = new Set(registrations.map(r => `${r.method} ${r.route}`));
        for (const key of Object.keys(ALLOWED_WITHOUT_APP_AUTH)) {
            expect(keys.has(key)).toBe(true);
        }
    });
});

describe('forwardToGo trust headers', () => {
    it('marks every proxied request as client-originated for Go\'s defense-in-depth check', () => {
        expect(APP_JS).toMatch(/options\.headers\['X-Proxy-Origin'\]\s*=\s*'client'/);
    });

    it('never lets a client supply the internal trust headers when forwarding partner credentials', () => {
        for (const h of ['x-internal-proxy-secret', 'x-user-id', 'x-user-role', 'x-user-email', 'x-proxy-origin', 'cookie']) {
            expect(APP_JS).toContain(`'${h}'`);
        }
    });
});
