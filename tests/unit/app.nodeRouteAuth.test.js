// tests/unit/app.nodeRouteAuth.test.js
//
// Node-handled routes found answering anonymously in the 2026-10-19 release
// hardening pass (the Go-forwarded routes are guarded separately by
// app.forwardToGoAuth.test.js). Static source checks, same approach as the
// other app.*Auth tests — requiring app.js has real startup side effects.
const fs = require('fs');
const path = require('path');

const read = (rel) => fs.readFileSync(path.join(__dirname, '../..', rel), 'utf8');
const APP_JS = read('app.js');
const TEMPLATE_ROUTES = read('routes/interfaceTemplateRoutes.js');
const WIZARD_ROUTES = read('routes/wizardRoutes.js');

describe('debug routes are admin-only', () => {
    it.each(['/api/test-route', '/api/proxy/test'])('%s requires login + admin role', (route) => {
        const line = APP_JS.split('\n').find(l => l.includes(`app.get('${route}'`));
        expect(line).toBeDefined();
        expect(line).toMatch(/_verifyToken\s*,\s*_requireAdminUser/);
    });
});

describe('/api/status stays a public probe without internal details', () => {
    const block = APP_JS.slice(APP_JS.indexOf("app.get('/api/status'"), APP_JS.indexOf("app.get('/api/status'") + 900);
    it('does not disclose the backend URL or ports', () => {
        expect(block).not.toMatch(/backend_url\s*:/);
        expect(block).not.toMatch(/ports\s*:/);
    });
});

describe('template-preview requires login', () => {
    it('is registered with an auth middleware', () => {
        expect(APP_JS).toMatch(/app\.post\('\/api\/interfaces\/:interfaceId\/template-preview',\s*_isAuth\s*,/);
    });
});

describe('interface template reads require login', () => {
    it.each([
        ["router.get('/',"],
        ["router.get('/categories',"],
        ["router.get('/:id',"],
    ])('%s is authenticated', (prefix) => {
        const line = TEMPLATE_ROUTES.split('\n').find(l => l.trim().startsWith(prefix));
        expect(line).toBeDefined();
        expect(line).toMatch(/isAuthenticated/);
    });
});

describe('wizard runtime-mapping requires login', () => {
    it('is registered with requireAuth', () => {
        expect(WIZARD_ROUTES).toMatch(/router\.get\('\/runtime-mapping\/:interfaceId\/:messageType',\s*requireAuth\s*,\s*canRead/);
    });
});
