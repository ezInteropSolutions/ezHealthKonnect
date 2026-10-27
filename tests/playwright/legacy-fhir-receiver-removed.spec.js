// tests/playwright/legacy-fhir-receiver-removed.spec.js
//
// v26.10 removed the Node FHIR receiver prototypes (/fhir/receiver/* and
// /api/messages/fhir/*): optional or no authentication, never ran a pipeline,
// wrote columns that no longer exist. FHIR resources are received by the HTTP
// FHIR Receiver connector. These URLs must answer 404 on the running app —
// never 2xx (an unauthenticated write) and never 5xx (a half-removed handler).
const { test, expect } = require('@playwright/test');

const ID = '00000000-0000-0000-0000-000000000000';
const cases = [
    ['post', `/fhir/receiver/${ID}`],
    ['put', `/fhir/receiver/${ID}/Patient/example`],
    ['get', '/fhir/receiver/health'],
    ['post', '/api/messages/fhir/Patient'],
    ['post', '/api/messages/fhir/Observation'],
];

test.describe('Legacy FHIR receiver URLs are gone', () => {
    for (const [method, url] of cases) {
        test(`${method.toUpperCase()} ${url} → 404`, async ({ playwright, baseURL }) => {
            // A fresh, unauthenticated context: these were partner-facing URLs.
            const anon = await playwright.request.newContext({ baseURL });
            const res = await anon[method](url, {
                headers: { 'Content-Type': 'application/fhir+json' },
                data: { resourceType: 'Patient', id: 'example' },
            });
            expect(res.status()).toBe(404);
            await anon.dispose();
        });
    }
});
