// tests/playwright/dicom-wizard-e2e.spec.js
//
// The "Connect a Device" guided wizard's "DICOM Storage SCP" template
// (V277__Add_DICOM_Storage_SCP_Connectivity_Type.sql) — the imaging-device
// counterpart to the lab-instrument templates (HL7/ASTM) already proven by
// device-connect-wizard-e2e.spec.js / astm-tcp-wizard-e2e.spec.js /
// astm-serial-wizard-e2e.spec.js. Driven through the real "Use Template"
// click path, same discipline as those three.
//
// Unlike ASTM's simple ENQ/ACK framing (hand-rolled in ~30 lines of raw
// net.Socket in astm-tcp-wizard-e2e.spec.js), real DICOM requires full DIMSE
// association negotiation that no lightweight JS library speaks on the
// wire (checked: no DICOM package is in package.json; libraries like dcmjs
// only parse files). The real device-side client here is the permanent Go
// fixture at tests/playwright/fixtures/dicom_scu_client — invoked via
// `docker run --network host golang:1.25-alpine go run main.go ...`, never
// run directly on the host, per this repo's own standing Go-build rule.
const { test, expect } = require('@playwright/test');
const { execFileSync } = require('child_process');
const path = require('path');

const TEMPLATE_NAME = 'DICOM Storage SCP';
const FIXTURE_DIR = path.join(__dirname, 'fixtures', 'dicom_scu_client');

// Runs the real Go SCU client fixture inside a Docker container (never
// directly on the host) against the given host:port, returning stdout.
// Throws if the client's own Associate/Echo/Store sequence fails.
function runDicomSCUClient({ addr, aeTitle, patientId }) {
    const args = [
        'run', '--rm', '--network', 'host',
        '-v', `${FIXTURE_DIR}:/app`, '-w', '/app',
        '-v', 'ehk-gomodcache:/go/pkg/mod',
        '-v', 'ehk-gobuildcache:/root/.cache/go-build',
        'golang:1.25-alpine',
        'go', 'run', 'main.go',
        '-addr', addr, '-ae', aeTitle, '-patient-id', patientId,
    ];
    return execFileSync('docker', args, { encoding: 'utf8', timeout: 30000 });
}

async function cleanupInterfacesByName(request, name) {
    const res = await request.get('/api/interfaces');
    const body = await res.json();
    const list = body.data || body.interfaces || body || [];
    for (const iface of list) {
        if (iface.name === name) {
            await request.post(`/api/runtime/interfaces/${iface.id}/deactivate`).catch(() => {});
            await request.delete(`/api/interfaces/${iface.id}`).catch(() => {});
        }
    }
}

async function openTemplateModal(page) {
    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-cat-tab', { timeout: 15000 });
    await page.locator('.tg-cat-tab', { hasText: 'Devices' }).click();

    const card = page.locator('.tg-card', { hasText: TEMPLATE_NAME });
    await expect(card, `${TEMPLATE_NAME} card should be visible under Devices`).toBeVisible({ timeout: 5000 });
    await card.locator('.tg-use-btn').click();
    await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
}

test.describe.serial('Connect a Device wizard ("DICOM Storage SCP" template)', () => {
    test.describe.configure({ retries: 0 });

    test('Devices tab lists the DICOM Storage SCP template alongside the lab-instrument templates', async ({ page }) => {
        await page.goto('/dashboard.html');
        await page.click('a[href="#templates"]');
        await page.waitForSelector('.tg-cat-tab', { timeout: 15000 });
        await page.locator('.tg-cat-tab', { hasText: 'Devices' }).click();

        await expect(page.locator('.tg-card', { hasText: 'HL7 over TCP/IP' })).toBeVisible({ timeout: 5000 });
        await expect(page.locator('.tg-card', { hasText: TEMPLATE_NAME })).toBeVisible({ timeout: 5000 });
    });

    test('Use renders real AE Title, Port Number, Results Delivery API URL, and Bearer Token fields', async ({ page }) => {
        await openTemplateModal(page);

        const aeTitleInput = page.locator('[data-tcf-field="ae_title"][data-tcf-section="source"]');
        await expect(aeTitleInput, 'a real AE Title input should render').toBeVisible({ timeout: 3000 });

        const portInput = page.locator('[data-tcf-field="port"][data-tcf-section="source"]');
        await expect(portInput, 'a real Port Number input should render').toBeVisible({ timeout: 3000 });
        await expect(portInput).toHaveAttribute('data-tcf-type', 'number');

        const endpointInput = page.locator('[data-tcf-field="endpoint"][data-tcf-section="target"]');
        await expect(endpointInput, 'a real Results Delivery API URL input should render').toBeVisible({ timeout: 3000 });
        await expect(endpointInput).toHaveAttribute('data-tcf-type', 'url');

        const bearerInput = page.locator('[data-tcf-field="bearer_token"][data-tcf-section="target"]');
        await expect(bearerInput, 'a real Bearer Token input should render').toBeVisible({ timeout: 3000 });
        await expect(bearerInput).toHaveAttribute('type', 'password');

        await page.locator('#tgConfigureModal button:has-text("Cancel")').click();
    });

    test('Filling in real values creates a working connector: a real DICOM C-ECHO + C-STORE is received and stored', async ({ page, request }) => {
        const ifaceName = `PW DICOM Wizard ${Date.now()}`;
        const port = 11114; // within the real Docker-mapped 11112-11120 range, distinct from other suites' ports
        const aeTitle = 'EZHEALTHKONNECT';

        await cleanupInterfacesByName(request, ifaceName);
        await openTemplateModal(page);

        await page.fill('#tcf_name', ifaceName);
        await page.fill('[data-tcf-field="ae_title"][data-tcf-section="source"]', aeTitle);
        await page.fill('[data-tcf-field="port"][data-tcf-section="source"]', String(port));
        await page.fill('[data-tcf-field="endpoint"][data-tcf-section="target"]', 'https://his.example.org/api/images');

        await page.click('#tcf_submit');

        // A real connector gets activated (ae_title/port are both provided,
        // real required fields) -> the verification step should appear.
        await page.waitForSelector('#tgVerifyModal', { state: 'visible', timeout: 15000 });
        await expect(page.locator('#tgVerifyTitle')).toContainText("Waiting for your device's first message");

        // Drive a real DICOM association + C-ECHO + C-STORE against the port
        // just configured through the wizard — the actual proof the
        // collected ae_title/port values reached the real running
        // dicom_storage_inbound connector.
        const output = runDicomSCUClient({ addr: `127.0.0.1:${port}`, aeTitle, patientId: 'PWDICOM001' });
        expect(output).toContain('ASSOCIATED');
        expect(output).toContain('ECHO_OK');
        expect(output).toContain('STORE_OK');

        await expect(page.locator('#tgVerifyTitle')).toContainText('Received a message from your device', { timeout: 20000 });
        await expect(page.locator('#tgVerifyGoBtn')).toBeVisible();

        await cleanupInterfacesByName(request, ifaceName);
    });

    // Regression guard for a real gap found during a QA pass (October 2026):
    // _collectTemplateConnectionValues' own client-side "required field
    // empty" check (missingRequiredSourceField) only catches a BLANK field —
    // a literal "0" in the Port Number input is non-empty, non-NaN, so it
    // passes that check and reaches activation, where the Go connector's own
    // Validate() correctly rejects port=0 ("port is required"). Before this
    // round's fix, the wizard would still show "Waiting for your device's
    // first message…" and poll forever for a connection that could
    // structurally never arrive, since the connector never actually started.
    test('A non-empty but invalid port (0) shows the real activation warning, not a false "waiting for device" screen', async ({ page, request }) => {
        const ifaceName = `PW DICOM Wizard BadPort ${Date.now()}`;

        await cleanupInterfacesByName(request, ifaceName);
        await openTemplateModal(page);

        await page.fill('#tcf_name', ifaceName);
        await page.fill('[data-tcf-field="ae_title"][data-tcf-section="source"]', 'EZHEALTHKONNECT');
        await page.fill('[data-tcf-field="port"][data-tcf-section="source"]', '0');
        await page.fill('[data-tcf-field="endpoint"][data-tcf-section="target"]', 'https://his.example.org/api/images');

        await page.click('#tcf_submit');

        // The real activation-warning modal must appear instead of the
        // "waiting for your device" one.
        await page.waitForSelector('#tgVerifyModal', { state: 'visible', timeout: 15000 });
        await expect(page.locator('#tgVerifyModal')).toContainText("didn't fully start");
        await expect(page.locator('#tgVerifyModal')).toContainText('port is required');
        await expect(page.locator('#tgVerifyModal')).not.toContainText("Waiting for your device's first message");

        await cleanupInterfacesByName(request, ifaceName);
    });
});
