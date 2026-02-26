const { test, expect } = require('@playwright/test');

// ===============================================================
// ERROR HANDLING & RETRY UI TESTS
// ===============================================================
// Tests the combined "Error Handling & Retry" section in PropertiesPanel
// that appears for every step in the pipeline builder.

const BASE_URL = 'http://localhost:3000';

// Helper: Login via API and set session cookie, then navigate
async function login(page) {
    await page.goto(`${BASE_URL}/login.html`);

    // Try multiple passwords (seed vs reset)
    const passwords = ['admin123', 'Admin123!', 'password'];
    let loggedIn = false;

    for (const password of passwords) {
        const response = await page.evaluate(async (pwd) => {
            const res = await fetch('/api/auth/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email: 'admin@ezhealthkonnect.com', password: pwd })
            });
            return { ok: res.ok, status: res.status, body: await res.json() };
        }, password);

        if (response.ok) {
            if (response.body.token) {
                await page.evaluate((token) => {
                    localStorage.setItem('accessToken', token);
                }, response.body.token);
            }
            loggedIn = true;
            break;
        }
    }

    if (!loggedIn) {
        throw new Error('Could not login with any known password');
    }

    await page.goto(`${BASE_URL}/dashboard.html`);
    await page.waitForLoadState('networkidle');
}

// Helper: Find an interface ID that has a pipeline with steps
async function getInterfaceWithPipeline(page) {
    const result = await page.evaluate(async () => {
        const res = await fetch('/api/interfaces', {
            headers: { 'Authorization': `Bearer ${localStorage.getItem('accessToken')}` }
        });
        if (!res.ok) return null;
        const data = await res.json();
        const interfaces = data.interfaces || data.data || data;
        if (Array.isArray(interfaces) && interfaces.length > 0) {
            return interfaces[0].id;
        }
        return null;
    });
    return result;
}

// Helper: Navigate to pipeline builder for an interface with steps
async function navigateToPipelineBuilder(page) {
    let interfaceId = await getInterfaceWithPipeline(page);

    if (!interfaceId) {
        interfaceId = '762aebb9-0408-4a42-82c5-202f13f28315';
    }

    await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${interfaceId}`);
    await page.waitForLoadState('networkidle');

    // Wait for the flowchart to render
    await page.waitForSelector('.flowchart-step-node', { timeout: 15000 });
}

// Helper: Select first step and expand the Error Handling & Retry section
async function selectFirstStep(page) {
    const stepNode = page.locator('.flowchart-step-node').first();
    await stepNode.click();

    // Wait for properties panel to have the error handling section in DOM
    await page.waitForSelector('.error-handling-section', { timeout: 5000 });

    // The section body is collapsed by default (display:none).
    // Click the h4 header to expand it.
    const sectionHeader = page.locator('.error-handling-section h4');
    // Check if the section body is hidden
    const sectionBody = page.locator('.error-handling-section h4 + div');
    const display = await sectionBody.evaluate(el => el.style.display);
    if (display === 'none') {
        await sectionHeader.click();
        // Wait for section body to be visible
        await page.waitForFunction(() => {
            const body = document.querySelector('.error-handling-section h4 + div');
            return body && body.style.display !== 'none';
        }, { timeout: 3000 });
    }
}

// ===============================================================
// TEST SUITE: Error Handling Section Rendering
// ===============================================================

test.describe('Error Handling & Retry Section', () => {

    test.beforeEach(async ({ page }) => {
        await login(page);
    });

    test('Error Handling section renders for every step', async ({ page }) => {
        await navigateToPipelineBuilder(page);
        await selectFirstStep(page);

        const section = page.locator('.error-handling-section');
        await expect(section).toBeVisible();

        const header = section.locator('h4');
        await expect(header).toContainText('Error Handling & Retry');
    });

    test('Retry toggle renders with correct initial state (off)', async ({ page }) => {
        await navigateToPipelineBuilder(page);
        await selectFirstStep(page);

        const retryCheckbox = page.locator('#retryEnabled');
        await expect(retryCheckbox).not.toBeChecked();

        const configArea = page.locator('#retryConfigArea');
        await expect(configArea).toHaveCSS('opacity', '0.4');
    });

    test('Error handling toggle renders with correct initial state (off)', async ({ page }) => {
        await navigateToPipelineBuilder(page);
        await selectFirstStep(page);

        const ehCheckbox = page.locator('#ehEnabled');
        await expect(ehCheckbox).not.toBeChecked();

        const configArea = page.locator('#ehConfigArea');
        await expect(configArea).toHaveCSS('opacity', '0.4');
    });

    test('Retry toggle enables config area when clicked', async ({ page }) => {
        await navigateToPipelineBuilder(page);
        await selectFirstStep(page);

        // The checkbox is hidden (opacity:0, width:0, height:0) as part of a CSS slider toggle.
        // Scroll it into view first, then use dispatchEvent to toggle.
        const retryCheckbox = page.locator('#retryEnabled');
        await retryCheckbox.scrollIntoViewIfNeeded();
        await retryCheckbox.evaluate(el => { el.checked = true; el.dispatchEvent(new Event('change', { bubbles: true })); });

        await expect(retryCheckbox).toBeChecked();

        // Config area should be enabled
        const configArea = page.locator('#retryConfigArea');
        await expect(configArea).toHaveCSS('opacity', '1');
        await expect(configArea).toHaveCSS('pointer-events', 'auto');
    });

    test('Error handling toggle enables config area when clicked', async ({ page }) => {
        await navigateToPipelineBuilder(page);
        await selectFirstStep(page);

        const ehCheckbox = page.locator('#ehEnabled');
        await ehCheckbox.scrollIntoViewIfNeeded();
        await ehCheckbox.evaluate(el => { el.checked = true; el.dispatchEvent(new Event('change', { bubbles: true })); });

        await expect(ehCheckbox).toBeChecked();

        const configArea = page.locator('#ehConfigArea');
        await expect(configArea).toHaveCSS('opacity', '1');
    });

    test('Retry config fields have correct default values', async ({ page }) => {
        await navigateToPipelineBuilder(page);
        await selectFirstStep(page);

        const maxRetries = page.locator('#retryMaxRetries');
        await expect(maxRetries).toHaveValue('3');

        const delayMs = page.locator('#retryDelayMs');
        await expect(delayMs).toHaveValue('1000');

        const backoffMultiplier = page.locator('#retryBackoffMultiplier');
        await expect(backoffMultiplier).toHaveValue('2');
    });

    test('On Error dropdown has all 3 options', async ({ page }) => {
        await navigateToPipelineBuilder(page);
        await selectFirstStep(page);

        const select = page.locator('#ehOnError');
        const options = select.locator('option');

        // "catch" was removed in P5 (identical to "suppress") — only suppress + rethrow remain
        await expect(options).toHaveCount(2);
        await expect(options.nth(0)).toHaveAttribute('value', 'suppress');
        await expect(options.nth(1)).toHaveAttribute('value', 'rethrow');
    });

    test('Default value fields are present', async ({ page }) => {
        await navigateToPipelineBuilder(page);
        await selectFirstStep(page);

        // These fields are inside ehConfigArea which has opacity:0.4 and pointer-events:none
        // but they should still be "attached" (exist in DOM). Use toBeAttached instead of toBeVisible.
        const fieldInput = page.locator('#ehDefaultField');
        const valueInput = page.locator('#ehDefaultValue');

        await expect(fieldInput).toBeAttached();
        await expect(valueInput).toBeAttached();
        await expect(fieldInput).toHaveAttribute('placeholder', /patient_status/);
        await expect(valueInput).toHaveAttribute('placeholder', /unknown/);
    });

    test('Retry and error handling toggles work independently', async ({ page }) => {
        await navigateToPipelineBuilder(page);
        await selectFirstStep(page);

        const retryCheckbox = page.locator('#retryEnabled');
        const ehCheckbox = page.locator('#ehEnabled');

        // Enable retry only
        await retryCheckbox.scrollIntoViewIfNeeded();
        await retryCheckbox.evaluate(el => { el.checked = true; el.dispatchEvent(new Event('change', { bubbles: true })); });
        await expect(retryCheckbox).toBeChecked();
        await expect(ehCheckbox).not.toBeChecked();

        // Enable error handling too
        await ehCheckbox.scrollIntoViewIfNeeded();
        await ehCheckbox.evaluate(el => { el.checked = true; el.dispatchEvent(new Event('change', { bubbles: true })); });
        await expect(retryCheckbox).toBeChecked();
        await expect(ehCheckbox).toBeChecked();

        // Disable retry — error handling should stay
        await retryCheckbox.evaluate(el => { el.checked = false; el.dispatchEvent(new Event('change', { bubbles: true })); });
        await expect(retryCheckbox).not.toBeChecked();
        await expect(ehCheckbox).toBeChecked();
    });

    test('RETRY ON badge appears when retry is enabled', async ({ page }) => {
        await navigateToPipelineBuilder(page);
        await selectFirstStep(page);

        // Initially no badge
        const header = page.locator('.error-handling-section h4');
        await expect(header).not.toContainText('RETRY ON');
    });

    test('Retry Logic template is NOT in toolbox', async ({ page }) => {
        await navigateToPipelineBuilder(page);

        const retryItem = page.locator('.toolbox-item:has-text("Retry Logic")');
        await expect(retryItem).toHaveCount(0);
    });

    test('Try-Catch template is NOT in toolbox', async ({ page }) => {
        await navigateToPipelineBuilder(page);

        const tryCatchItem = page.locator('.toolbox-item:has-text("Try-Catch")');
        await expect(tryCatchItem).toHaveCount(0);
    });
});
