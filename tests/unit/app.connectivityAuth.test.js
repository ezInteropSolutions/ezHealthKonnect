// tests/unit/app.connectivityAuth.test.js
//
// Regression guard for a real bug found while writing the device-connectivity
// test plan: GET /api/connectivity/serial-ports (and every other route under
// /api/connectivity — connector types, interface connectivity CRUD, ad-hoc
// connector testing) returned 200 with ZERO session cookie, confirmed live
// against the real running app. app.js registers most proxy routes with a
// bare `app.use('/api/x', forwardToGo)` and only a named few
// (/api/analytics, /api/alerts, /api/git, /api/migration, /api/ai,
// /api/cda/dedupe/registry) through an auth middleware first — /api/connectivity
// was simply missing from that second group.
//
// A full require('../../app.js') isn't practical here (it has real startup
// side effects — DB connections, session store init, etc.) and isn't how any
// other test in this suite exercises app.js. This is a static source check
// instead: it reads app.js as text and asserts the exact registration line
// includes an auth middleware reference before forwardToGo — a direct,
// low-maintenance regression guard against this exact bug recurring (someone
// removing the middleware, or a future route being added to this same
// unauthenticated pattern).
const fs = require('fs');
const path = require('path');

const APP_JS = fs.readFileSync(path.join(__dirname, '../../app.js'), 'utf8');
const AUTH_MIDDLEWARE_RE = /verifyToken|requireAuth|_analyticsAuth|_isAuth|_requireAdminUser/;

function findRegistration(routePath) {
    const escaped = routePath.replace(/\//g, '\\/');
    const re = new RegExp(`app\\.use\\(\\s*['"]${escaped}['"]\\s*,\\s*([^)]+)\\)`);
    return APP_JS.match(re);
}

describe('/api/connectivity requires authentication (app.js route registration)', () => {
    it('registers /api/connectivity with an auth middleware before forwardToGo', () => {
        const match = findRegistration('/api/connectivity');
        expect(match).toBeTruthy();

        const args = match[1];
        // The registration must name a real auth middleware (verifyToken or
        // requireAuth, whichever alias is in scope) BEFORE forwardToGo —
        // not just forwardToGo alone.
        expect(AUTH_MIDDLEWARE_RE.test(args)).toBe(true);

        // forwardToGo must come AFTER the auth middleware, not before —
        // Express runs handlers in the order given.
        const forwardIdx = args.indexOf('forwardToGo');
        const authIdx = args.search(AUTH_MIDDLEWARE_RE);
        expect(forwardIdx).toBeGreaterThan(-1);
        expect(authIdx).toBeGreaterThan(-1);
        expect(authIdx).toBeLessThan(forwardIdx);
    });

    // Documents the routes already correctly protected, so a future refactor
    // that accidentally un-protects one of these fails loudly here too,
    // rather than only /api/connectivity being checked.
    it.each([
        ['/api/analytics'],
        ['/api/alerts'],
        ['/api/git'],
        ['/api/migration'],
        ['/api/cda/dedupe/registry'],
    ])('registers %s with its own auth middleware before forwardToGo', (routePath) => {
        const match = findRegistration(routePath);
        expect(match).toBeTruthy();
        expect(AUTH_MIDDLEWARE_RE.test(match[1])).toBe(true);
    });
});

// Second real bug found by the same security-validation pass, in a
// different file: routes/interfaceTemplateRoutes.js's POST /:id/use sat
// between a "Read (public)" block and a "Write (requires authentication)"
// block with no auth of its own, despite being a POST with a real side
// effect (usage_count increment) — confirmed live (200 with no session
// cookie) before the fix. Same static-source-check approach as above, since
// this file also has require-time side effects (it pulls in
// controllers/interfaceTemplateController.js, which connects to the DB via
// sequelize) that make a full require() in a unit test impractical.
describe('POST /api/interface-templates/:id/use requires authentication', () => {
    const ROUTES_FILE = fs.readFileSync(
        path.join(__dirname, '../../routes/interfaceTemplateRoutes.js'), 'utf8'
    );

    it('registers the /:id/use route with isAuthenticated before the controller', () => {
        const match = ROUTES_FILE.match(/router\.post\(\s*['"]\/:id\/use['"]\s*,\s*([^)]+)\)/);
        expect(match).toBeTruthy();

        const args = match[1];
        expect(/isAuthenticated/.test(args)).toBe(true);

        const ctrlIdx = args.indexOf('ctrl.useTemplate');
        const authIdx = args.indexOf('isAuthenticated');
        expect(ctrlIdx).toBeGreaterThan(-1);
        expect(authIdx).toBeGreaterThan(-1);
        expect(authIdx).toBeLessThan(ctrlIdx);
    });
});
