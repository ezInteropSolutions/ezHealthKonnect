'use strict';
/**
 * Playwright E2E — Message Trace search page
 *
 * TC-TRACE-001  Page title contains "Message Trace"
 * TC-TRACE-002  Page header and search box render
 * TC-TRACE-003  Sidebar "Message Trace" nav item is present (Operations section)
 * TC-TRACE-004  Searching an unknown ID in an interface shows a "no message found" state that names the interface
 * TC-TRACE-005  Picking an interface + a real correlation ID renders the timeline with expected stages
 * TC-TRACE-006  Deep link (?interfaceId=...&id=...) auto-runs the trace on load
 * TC-TRACE-007  No horizontal overflow at 1280px
 * TC-TRACE-008  Tracing without choosing an interface asks for one (the trace is scoped to one interface)
 *
 * TC-TRACE-005/006 send a real HL7 message over MLLP (self-contained fixture —
 * does not depend on any prior manual test run) with a unique correlation ID,
 * then trace it through the full pipeline via the UI.
 */

const { test, expect } = require('./helpers/seeded');

/** Sends one ADT^A01 HL7 message through the seeded MLLP listener (helpers/seeded.js) with the
 *  given MSH-10 control ID (which becomes the message's correlation_id), and resolves once the
 *  ACK is received. */
function sendTestMessage(seed, controlId) {
    const ts = '20260704120000';
    const hl7 = [
        `MSH|^~\\&|PWTEST|HOSP|EZHK|EZHK|${ts}||ADT^A01^ADT_A01|${controlId}|P|2.5`,
        `EVN|A01|${ts}`,
        `PID|1||PWTRACE001^^^HOSP^MR||TRACE^PLAYWRIGHT|||M|||1 TEST ST^^ANYTOWN^CA^90210`,
        `PV1|1|I|WARD01^101^A^HOSP`,
    ].join('\r');

    return seed.sendMessage(hl7);
}

test.describe('Message Trace', () => {

    test.beforeEach(async ({ page }) => {
        await page.goto('/message-trace.html');
        await page.waitForLoadState('domcontentloaded');
    });

    // ── TC-TRACE-001 ─────────────────────────────────────────────────────────────
    test('TC-TRACE-001 page title contains Message Trace', async ({ page }) => {
        await expect(page).toHaveTitle(/Message Trace/i);
    });

    // ── TC-TRACE-002 ─────────────────────────────────────────────────────────────
    test('TC-TRACE-002 page header and search box render', async ({ page }) => {
        const header = page.locator('.page-header');
        await expect(header).toBeVisible();
        await expect(header.getByText(/message trace/i)).toBeVisible();
        await expect(page.locator('#interfaceInput')).toBeVisible();
        await expect(page.locator('#correlationInput')).toBeVisible();
        await expect(page.locator('#traceBtn')).toBeVisible();
    });

    // ── TC-TRACE-003 ─────────────────────────────────────────────────────────────
    test('TC-TRACE-003 sidebar Message Trace nav item is present', async ({ page }) => {
        const navItem = page.locator('.nav-item', { hasText: 'Message Trace' });
        await expect(navItem).toBeVisible({ timeout: 8000 });
        await expect(navItem).toHaveAttribute('href', 'message-trace.html');
    });

    // ── TC-TRACE-004 ─────────────────────────────────────────────────────────────
    test('TC-TRACE-004 unknown ID shows a not-found state naming the interface', async ({ page, seed }) => {
        await page.locator('#interfaceInput').fill(seed.interfaceName);
        await page.locator('#correlationInput').fill('no-such-correlation-id-xyz');
        await page.locator('#traceBtn').click();
        await expect(page.locator('.empty.error')).toBeVisible({ timeout: 8000 });
        await expect(page.locator('.empty.error')).toContainText(/no message found/i);
        await expect(page.locator('.empty.error')).toContainText(seed.interfaceName);
    });

    // ── TC-TRACE-005 ─────────────────────────────────────────────────────────────
        test('TC-TRACE-005 real correlation ID renders full timeline', async ({ page, seed }) => {
        const controlId = 'PW' + Date.now();
        await sendTestMessage(seed, controlId);
        await page.waitForTimeout(3000); // allow async pipeline (parse -> transform -> deliver) to finish

        await page.locator('#interfaceInput').fill(seed.interfaceName);
        await page.locator('#correlationInput').fill(controlId);
        await page.locator('#traceBtn').click();

        const resultPanel = page.locator('#resultPanel');
        await expect(resultPanel).toBeVisible({ timeout: 10000 });
        await expect(page.locator('#countPill')).toContainText(/event/i, { timeout: 10000 });

        const rows = page.locator('.timeline-row');
        await expect(rows.first()).toBeVisible({ timeout: 10000 });
        const count = await rows.count();
        expect(count).toBeGreaterThan(0);

        // Expect the known lifecycle stages from processing/engine_message_processor.go
        // and executor_registry.go to appear somewhere in the rendered timeline.
        const allStages = (await page.locator('.timeline-stage').allTextContents()).join(' ').toLowerCase();
        expect(allStages).toMatch(/connection|receive/);
        expect(allStages).toMatch(/parsing/);
        expect(allStages).toMatch(/transformation|delivery/);
    });

    // ── TC-TRACE-006 ─────────────────────────────────────────────────────────────
        test('TC-TRACE-006 deep link auto-runs the trace', async ({ page, seed }) => {
        const controlId = 'PW' + Date.now();
        await sendTestMessage(seed, controlId);
        await page.waitForTimeout(3000);

        // The legacy ?correlationId= parameter still works, but the interface must now be given too.
        await page.goto('/message-trace.html?interfaceId=' + seed.interfaceId + '&correlationId=' + encodeURIComponent(controlId));
        await page.waitForLoadState('domcontentloaded');

        await expect(page.locator('#correlationInput')).toHaveValue(controlId);
        await expect(page.locator('#resultPanel')).toBeVisible({ timeout: 10000 });
        await expect(page.locator('.timeline-row').first()).toBeVisible({ timeout: 10000 });
    });

    // ── TC-TRACE-008 ─────────────────────────────────────────────────────────────
    test('TC-TRACE-008 tracing without an interface asks for one', async ({ page }) => {
        await page.locator('#correlationInput').fill('anything');
        await page.locator('#traceBtn').click();
        await expect(page.locator('.empty.error')).toContainText(/pick the interface/i);
    });

    // ── TC-TRACE-007 ─────────────────────────────────────────────────────────────
    test('TC-TRACE-007 no horizontal overflow at 1280px', async ({ page }) => {
        await page.setViewportSize({ width: 1280, height: 800 });
        const scrollWidth = await page.evaluate(() => document.body.scrollWidth);
        expect(scrollWidth).toBeLessThanOrEqual(1285);
    });
});
