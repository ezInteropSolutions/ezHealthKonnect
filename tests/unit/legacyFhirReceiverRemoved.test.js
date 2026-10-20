// tests/unit/legacyFhirReceiverRemoved.test.js
//
// v26.10 removed two Node "FHIR receiver" prototypes: /fhir/receiver/:id
// (optional auth; its Basic auth accepted any password) and
// /api/messages/fhir/:type (no auth). Neither ever ran a pipeline, and both
// wrote message-table columns that no longer exist. FHIR resources are
// received by the http_fhir_inbound connector (authenticated, validated, full
// pipeline). This guards against an unauthenticated write path coming back.
const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..', '..');

function routePaths(router) {
    return router.stack
        .filter(layer => layer.route)
        .map(layer => ({ path: layer.route.path, methods: Object.keys(layer.route.methods) }));
}

describe('legacy Node FHIR receivers stay removed', () => {
    it('message routes expose no /fhir receiver', () => {
        const router = require('../../routes/messageRoutes');
        const fhir = routePaths(router).filter(r => r.path.startsWith('/fhir'));
        expect(fhir).toEqual([]);
    });

    it('app.js mounts nothing at /fhir', () => {
        const app = fs.readFileSync(path.join(ROOT, 'app.js'), 'utf8');
        expect(app).not.toMatch(/app\.use\(\s*['"]\/fhir['"]/);
        expect(app).not.toMatch(/fhirReceiverRoutes/);
    });

    it('the prototype controller and routes are gone', () => {
        expect(fs.existsSync(path.join(ROOT, 'controllers', 'FhirReceiverController.js'))).toBe(false);
        expect(fs.existsSync(path.join(ROOT, 'routes', 'fhirReceiverRoutes.js'))).toBe(false);
    });

    it('the HTTP FHIR Receiver connector that replaces them is still registered', () => {
        const factory = fs.readFileSync(path.join(ROOT, 'services', 'connectors', 'connector_factory.go'), 'utf8');
        expect(factory).toMatch(/"http_fhir_inbound"/);
    });
});
