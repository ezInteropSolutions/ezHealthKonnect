'use strict';
/**
 * Playwright E2E — Message Trace search page
 *
 * TC-TRACE-001  Page title contains "Message Trace"
 * TC-TRACE-002  Page header and search box render
 * TC-TRACE-003  Sidebar "Message Trace" nav item is present (Operations section)
 * TC-TRACE-004  Searching an unknown correlation ID shows a "no message found" state
 * TC-TRACE-005  Searching a real correlation ID renders the timeline with expected stages
 * TC-TRACE-006  Deep link (?correlationId=...) auto-runs the trace on load
 * TC-TRACE-007  No horizontal overflow at 1280px
 *
 * TC-TRACE-005/006 send a real HL7 message over MLLP (self-contained fixture —
 * does not depend on any prior manual test run) with a unique correlation ID,
 * then trace it through the full pipeline via the UI.
 */

const { test, expect } = require('@playwright/test');
const net = require('net');

// "Universal HL7 Receiver" test interface — MLLP listener on port 6613 (same
// fixture interface used by tests/throughput-test.js and tests/hl7-scale-test.js).
const MLLP_HOST = 'localhost';
const MLLP_PORT = 6613;

function mllpFrame(hl7) {
    return Buffer.concat([Buffer.from([0x0B]), Buffer.from(hl7, 'ascii'), Buffer.from([0x1C, 0x0D])]);
}

/** Sends one ADT^A01 HL7 message over MLLP with the given MSH-10 control ID
 *  (which becomes the message's correlation_id), and resolves once the ACK
 *  is received. */
function sendTestMessage(controlId) {
    const ts = '20260704120000';
    const hl7 = [
        `MSH|^~\\&|PWTEST|HOSP|EZHK|EZHK|${ts}||ADT^A01^ADT_A01|${controlId}|P|2.5`,
        `EVN|A01|${ts}`,
        `PID|1||PWTRACE001^^^HOSP^MR||TRACE^PLAYWRIGHT|||M|||1 TEST ST^^ANYTOWN^CA^90210`,
        `PV1|1|I|WARD01^101^A^HOSP`,
    ].join('\r');

    return new Promise((resolve, reject) => {
        const sock = new net.Socket();
        const timer = setTimeout(() => { sock.destroy(); reject(new Error('MLLP send timed out')); }, 10000);
        sock.on('error', (err) => { clearTimeout(timer); reject(err); });
        let buf = Buffer.alloc(0);
        sock.on('data', (chunk) => {
            buf = Buffer.concat([buf, chunk]);
            if (buf.length >= 2 && buf[buf.length - 1] === 0x0D && buf[buf.length - 2] === 0x1C) {
                clearTimeout(timer);
                sock.destroy();
                resolve();
            }
        });
        sock.connect(MLLP_PORT, MLLP_HOST, () => sock.write(mllpFrame(hl7)));
    });
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
    test('TC-TRACE-004 unknown correlation ID shows not-found state', async ({ page }) => {
        await page.locator('#correlationInput').fill('no-such-correlation-id-xyz');
        await page.locator('#traceBtn').click();
        await expect(page.locator('.empty.error')).toBeVisible({ timeout: 8000 });
        await expect(page.locator('.empty.error')).toContainText(/no message found/i);
    });

    // ── TC-TRACE-005 ─────────────────────────────────────────────────────────────
    test('TC-TRACE-005 real correlation ID renders full timeline', async ({ page }) => {
        const controlId = 'PW' + Date.now();
        await sendTestMessage(controlId);
        await page.waitForTimeout(3000); // allow async pipeline (parse -> transform -> deliver) to finish

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
    test('TC-TRACE-006 deep link auto-runs the trace', async ({ page }) => {
        const controlId = 'PW' + Date.now();
        await sendTestMessage(controlId);
        await page.waitForTimeout(3000);

        await page.goto('/message-trace.html?correlationId=' + encodeURIComponent(controlId));
        await page.waitForLoadState('domcontentloaded');

        await expect(page.locator('#correlationInput')).toHaveValue(controlId);
        await expect(page.locator('#resultPanel')).toBeVisible({ timeout: 10000 });
        await expect(page.locator('.timeline-row').first()).toBeVisible({ timeout: 10000 });
    });

    // ── TC-TRACE-007 ─────────────────────────────────────────────────────────────
    test('TC-TRACE-007 no horizontal overflow at 1280px', async ({ page }) => {
        await page.setViewportSize({ width: 1280, height: 800 });
        const scrollWidth = await page.evaluate(() => document.body.scrollWidth);
        expect(scrollWidth).toBeLessThanOrEqual(1285);
    });
});
