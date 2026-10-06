// tests/playwright/device-wizard-ui-polish-e2e.spec.js
//
// Closes UI-004, UI-005, UI-013, UI-014, UI-016, UI-022 from the device-
// connectivity test plan — gaps left manual in the first automation pass.
const { test, expect } = require('@playwright/test');

async function openDevicesTab(page) {
    await page.goto('/dashboard.html');
    await page.click('a[href="#templates"]');
    await page.waitForSelector('.tg-cat-tab', { timeout: 15000 });
    await page.locator('.tg-cat-tab', { hasText: 'Devices' }).click();
}

test.describe.serial('Connect a Device wizard — UI polish regression guards', () => {
    test.describe.configure({ retries: 0 });

    // UI-004 — guide button renders the correct, template-specific content.
    test('Guide button on "ASTM over Serial Port" renders that template\'s own guide text', async ({ page }) => {
        await openDevicesTab(page);
        const card = page.locator('.tg-card', { hasText: 'ASTM over Serial Port' });
        await card.locator('.tg-guide-btn').click();

        await page.waitForSelector('#tgPreviewModal', { state: 'visible', timeout: 5000 });
        const guidePane = page.locator('#tgPreviewModal .tg-guide-body');
        await expect(guidePane).toContainText('serial', { ignoreCase: true });
        await expect(guidePane).toContainText('COM Port', { ignoreCase: true });
        // Negative check — must not show a DIFFERENT template's own content.
        await expect(guidePane).not.toContainText('TCP/IP / Network / Ethernet');

        await page.locator('#tgPreviewModal .tg-modal-close').click();
    });

    // UI-022 — regression guard for the two-stage edit this exact guide text
    // went through this session (V275 then V276).
    test('HL7 template\'s guide correctly references both ASTM templates (not "not supported yet")', async ({ page }) => {
        await openDevicesTab(page);
        const card = page.locator('.tg-card', { hasText: 'HL7 over TCP/IP' });
        await card.locator('.tg-guide-btn').click();

        await page.waitForSelector('#tgPreviewModal', { state: 'visible', timeout: 5000 });
        const guidePane = page.locator('#tgPreviewModal .tg-guide-body');
        await expect(guidePane).toContainText('ASTM over Serial Port');
        await expect(guidePane).toContainText('ASTM over TCP/IP');
        await expect(guidePane).not.toContainText('not supported yet');

        await page.locator('#tgPreviewModal .tg-modal-close').click();
    });

    // UI-013 — every required field shows a visible asterisk.
    test('Required fields show a visible asterisk; optional fields do not', async ({ page }) => {
        await openDevicesTab(page);
        await page.locator('.tg-card', { hasText: 'ASTM over Serial Port' }).locator('.tg-use-btn').click();
        await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });

        // port_name and baud_rate are both required:true on this template.
        const portLabel = page.locator('label', { hasText: 'COM Port' });
        await expect(portLabel.locator('span', { hasText: '*' })).toBeVisible();
        const baudLabel = page.locator('label', { hasText: 'Baud Rate' });
        await expect(baudLabel.locator('span', { hasText: '*' })).toBeVisible();

        // bearer_token is required:false on this template.
        const tokenLabel = page.locator('label', { hasText: 'Bearer Token' });
        await expect(tokenLabel.locator('span', { hasText: '*' })).toHaveCount(0);

        await page.locator('#tgConfigureModal button:has-text("Cancel")').click();
    });

    // UI-014 — blank Interface Name blocks submission client-side, no API call made.
    test('Submitting with Interface Name blank shows an inline error and makes no API call', async ({ page }) => {
        await openDevicesTab(page);
        await page.locator('.tg-card', { hasText: 'ASTM over TCP/IP' }).locator('.tg-use-btn').click();
        await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });

        let useEndpointCalled = false;
        page.on('request', (req) => {
            if (req.url().includes('/use') && req.method() === 'POST') useEndpointCalled = true;
        });

        await page.fill('#tcf_name', '');
        await page.click('#tcf_submit');

        await expect(page.locator('#tcf_error')).toBeVisible({ timeout: 3000 });
        await expect(page.locator('#tcf_error')).toContainText('name', { ignoreCase: true });
        expect(useEndpointCalled, 'no /use API call should fire when client-side validation blocks submission').toBe(false);

        await page.locator('#tgConfigureModal button:has-text("Cancel")').click();
    });

    // UI-016 — Cancel and overlay-click both close the modal without creating anything.
    test('Cancel button closes the configure modal without creating an interface', async ({ page, request }) => {
        await openDevicesTab(page);
        await page.locator('.tg-card', { hasText: 'HL7 over TCP/IP' }).locator('.tg-use-btn').click();
        await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });

        const uniqueName = `UI-016 Cancel Test ${Date.now()}`;
        await page.fill('#tcf_name', uniqueName);
        await page.locator('#tgConfigureModal button:has-text("Cancel")').click();

        await expect(page.locator('#tgConfigureModal')).toHaveCount(0);

        const res = await request.get('/api/interfaces');
        const body = await res.json();
        const list = body.data || body.interfaces || body || [];
        const found = list.some((i) => i.name === uniqueName);
        expect(found, 'cancelling must never create an interface').toBe(false);
    });

    test('Clicking outside the configure modal (overlay click) closes it without creating anything', async ({ page, request }) => {
        await openDevicesTab(page);
        await page.locator('.tg-card', { hasText: 'ASTM over Serial Port' }).locator('.tg-use-btn').click();
        await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });

        const uniqueName = `UI-016 Overlay Test ${Date.now()}`;
        await page.fill('#tcf_name', uniqueName);
        // Click the overlay itself, not the inner modal card.
        await page.locator('#tgConfigureModal').click({ position: { x: 5, y: 5 } });

        await expect(page.locator('#tgConfigureModal')).toHaveCount(0);

        const res = await request.get('/api/interfaces');
        const body = await res.json();
        const list = body.data || body.interfaces || body || [];
        const found = list.some((i) => i.name === uniqueName);
        expect(found, 'an overlay click must never create an interface').toBe(false);
    });

    // UI-005 — no leftover state leaks between two different templates.
    test('Opening one template, cancelling, then opening a different template shows no leftover values', async ({ page }) => {
        await openDevicesTab(page);

        await page.locator('.tg-card', { hasText: 'ASTM over TCP/IP' }).locator('.tg-use-btn').click();
        await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });
        await page.fill('[data-tcf-field="port"][data-tcf-section="source"]', '6699');
        await page.fill('#tcf_name', 'Should Not Leak');
        await page.locator('#tgConfigureModal button:has-text("Cancel")').click();
        await expect(page.locator('#tgConfigureModal')).toHaveCount(0);

        await page.locator('.tg-card', { hasText: 'ASTM over Serial Port' }).locator('.tg-use-btn').click();
        await page.waitForSelector('#tgConfigureModal', { state: 'visible', timeout: 5000 });

        // The Serial template has no plain "port" number field at all (it's
        // "port_name", a select) — confirming the TCP template's own field
        // didn't somehow persist into this modal.
        await expect(page.locator('[data-tcf-field="port"][data-tcf-section="source"]')).toHaveCount(0);
        const nameValue = await page.locator('#tcf_name').inputValue();
        expect(nameValue).not.toBe('Should Not Leak');

        await page.locator('#tgConfigureModal button:has-text("Cancel")').click();
    });
});
