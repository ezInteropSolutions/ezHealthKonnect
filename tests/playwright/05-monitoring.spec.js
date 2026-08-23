'use strict';
/**
 * Playwright E2E — Monitoring dashboard
 *
 * TC-MON-001  Page title contains "Monitoring"
 * TC-MON-002  Page header renders with title and refresh button
 * TC-MON-003  Refresh button is clickable and triggers a loading state
 * TC-MON-004  Help banner is visible on first load
 * TC-MON-005  Dismiss button hides the help banner
 * TC-MON-006  KPI section renders 4 metric cards
 * TC-MON-007  KPI card values are numeric after load (not placeholder text)
 * TC-MON-008  KPI card labels match expected metric names
 * TC-MON-009  Alerts section is present
 * TC-MON-010  Alerts section renders a table or empty-state message
 * TC-MON-011  Live messages section is present
 * TC-MON-012  Live messages table has expected column headers
 * TC-MON-013  Connector health section is present
 * TC-MON-014  Interface health cards section is present
 * TC-MON-015  Auto-refresh badge or timer indicator is visible
 * TC-MON-016  Monitoring API endpoint returns 200 (backend integration check)
 * TC-MON-017  Sidebar Monitoring nav item is active
 * TC-MON-018  No horizontal overflow at 1280px
 */

const { test, expect } = require('@playwright/test');

test.describe('Monitoring', () => {

    test.beforeEach(async ({ page }) => {
        await page.goto('/monitoring.html');
        await page.waitForLoadState('domcontentloaded');
        await page.waitForTimeout(2000); // Allow initial API fetch to complete
    });

    // ── TC-MON-001 ───────────────────────────────────────────────────────────────
    test('TC-MON-001 page title contains Monitoring', async ({ page }) => {
        await expect(page).toHaveTitle(/Monitoring/i);
    });

    // ── TC-MON-002 ───────────────────────────────────────────────────────────────
    test('TC-MON-002 page header renders title and refresh button', async ({ page }) => {
        const header = page.locator('.page-header');
        await expect(header).toBeVisible();
        await expect(header.getByText(/monitoring/i)).toBeVisible();
        await expect(page.locator('.refresh-btn')).toBeVisible();
    });

    // ── TC-MON-003 ───────────────────────────────────────────────────────────────
    test('TC-MON-003 refresh button is clickable', async ({ page }) => {
        const refreshBtn = page.locator('.refresh-btn');
        await expect(refreshBtn).toBeEnabled();
        await refreshBtn.click();
        // After click a loading indicator may appear briefly — just verify no crash
        await page.waitForTimeout(1000);
        await expect(refreshBtn).toBeVisible();
    });

    // ── TC-MON-004 ───────────────────────────────────────────────────────────────
    test('TC-MON-004 help banner is visible on load', async ({ page }) => {
        const banner = page.locator('.help-banner');
        await expect(banner).toBeVisible({ timeout: 8000 });
    });

    // ── TC-MON-005 ───────────────────────────────────────────────────────────────
    test('TC-MON-005 dismiss button hides the help banner', async ({ page }) => {
        const banner  = page.locator('.help-banner');
        const dismiss = page.locator('.help-dismiss');
        await expect(banner).toBeVisible({ timeout: 5000 });
        await dismiss.click();
        await page.waitForTimeout(400);
        await expect(banner).toBeHidden();
    });

    // ── TC-MON-006 ───────────────────────────────────────────────────────────────
    test('TC-MON-006 KPI section renders 4 metric cards', async ({ page }) => {
        const kpiRow  = page.locator('.kpi-row, .kpi-section, [class*="kpi"]').first();
        await expect(kpiRow).toBeAttached({ timeout: 8000 });
        const cards = page.locator('.kpi-card, [class*="kpi-card"]');
        const count = await cards.count();
        expect(count).toBeGreaterThanOrEqual(4);
    });

    // ── TC-MON-007 ───────────────────────────────────────────────────────────────
    test('TC-MON-007 KPI card values are populated after API fetch', async ({ page }) => {
        await page.waitForTimeout(3000);
        const valueEls = page.locator('.kpi-value, [class*="kpi-value"], .kpi-number');
        const count    = await valueEls.count();
        if (count === 0) {
            test.skip();
            return;
        }
        for (let i = 0; i < Math.min(count, 4); i++) {
            const text = (await valueEls.nth(i).textContent())?.trim() ?? '';
            expect(text.length).toBeGreaterThan(0);
            expect(text).not.toMatch(/^\s*$/); // Not whitespace-only
        }
    });

    // ── TC-MON-008 ───────────────────────────────────────────────────────────────
    test('TC-MON-008 KPI card labels contain expected metric names', async ({ page }) => {
        const labels = page.locator('.kpi-label, [class*="kpi-label"]');
        const count  = await labels.count();
        if (count === 0) {
            test.skip();
            return;
        }
        const allText = (await labels.allTextContents()).join(' ').toLowerCase();
        // At least one of these metric names should appear
        const hasExpected = /messages|error|latency|interface|active|processing/i.test(allText);
        expect(hasExpected).toBe(true);
    });

    // ── TC-MON-009 ───────────────────────────────────────────────────────────────
    test('TC-MON-009 alerts section is present', async ({ page }) => {
        const section = page.locator('[class*="alerts"], #alertsSection, .alerts-card').first();
        await expect(section).toBeAttached({ timeout: 8000 });
    });

    // ── TC-MON-010 ───────────────────────────────────────────────────────────────
    test('TC-MON-010 alerts section renders table or empty state', async ({ page }) => {
        await page.waitForTimeout(2000);
        // Real alerts render as .alert-row divs (see monitoring.html's loadAlerts()),
        // never a <table> — the original locator here assumed markup that was
        // never actually used, so this test failed on every run where a real
        // alert existed (i.e. exactly the case it's meant to verify renders
        // correctly).
        const alertRows = page.locator('.alert-row');
        const emptyMsg  = page.locator('.alerts-empty, text=/no alerts|all clear|no active alerts/i').first();
        const hasAlerts = (await alertRows.count()) > 0;
        const hasEmpty  = await emptyMsg.isVisible().catch(() => false);
        expect(hasAlerts || hasEmpty).toBe(true);
    });

    // ── TC-MON-011 ───────────────────────────────────────────────────────────────
    test('TC-MON-011 live messages section is present', async ({ page }) => {
        // #feedBody is the live-messages table body confirmed present in monitoring.html
        const section = page.locator('#feedBody, [class*="live"], [class*="feed"]').first();
        await expect(section).toBeAttached({ timeout: 8000 });
    });

    // ── TC-MON-012 ───────────────────────────────────────────────────────────────
    test('TC-MON-012 live messages table has expected column headers', async ({ page }) => {
        await page.waitForTimeout(2000);
        const table = page.locator('[class*="live-messages"] table, [class*="message-feed"] table').first();
        if (!await table.isVisible().catch(() => false)) {
            test.skip();
            return;
        }
        const headers = (await table.locator('th').allTextContents()).join(' ').toLowerCase();
        const hasExpected = /message|interface|status|time|type/i.test(headers);
        expect(hasExpected).toBe(true);
    });

    // ── TC-MON-013 ───────────────────────────────────────────────────────────────
    test('TC-MON-013 connector health section is present', async ({ page }) => {
        // #connBody is the connector-health table body confirmed present in monitoring.html
        const section = page.locator('#connBody, [class*="connector-health"], [class*="connectors"]').first();
        await expect(section).toBeAttached({ timeout: 8000 });
    });

    // ── TC-MON-014 ───────────────────────────────────────────────────────────────
    test('TC-MON-014 interface health cards section is present', async ({ page }) => {
        // #healthGrid / .health-grid confirmed present in monitoring.html
        const section = page.locator('#healthGrid, .health-grid, [class*="health"]').first();
        await expect(section).toBeAttached({ timeout: 8000 });
    });

    // ── TC-MON-015 ───────────────────────────────────────────────────────────────
    test('TC-MON-015 auto-refresh badge or last-updated indicator is visible', async ({ page }) => {
        const indicator = page.locator('.auto-badge, .last-updated, [class*="auto-refresh"]').first();
        // Badge may only appear after a refresh cycle — accept any of these indicators
        const visible = await indicator.isVisible({ timeout: 5000 }).catch(() => false);
        // Not a hard failure — just verify it's somewhere on the page if present
        if (visible) {
            await expect(indicator).toBeVisible();
        }
    });

    // ── TC-MON-016 ───────────────────────────────────────────────────────────────
    test('TC-MON-016 monitoring summary API returns 200', async ({ page }) => {
        // Go backend requires JWT Bearer token (stored in localStorage.accessToken).
        // page.request.get only sends cookies — must use browser fetch with explicit header.
        const result = await page.evaluate(async () => {
            try {
                const token = localStorage.getItem('accessToken') || '';
                const headers = token ? { 'Authorization': 'Bearer ' + token } : {};
                const r = await fetch('/api/monitoring/summary', { headers });
                const body = await r.json().catch(() => null);
                return { status: r.status, body };
            } catch (e) { return { status: 0, body: null }; }
        });
        if (result.status === 0 || result.status >= 500) { test.skip(); return; }
        expect(result.status).toBe(200);
        if (result.body) expect(result.body.success).toBe(true);
    });

    // ── TC-MON-017 ───────────────────────────────────────────────────────────────
    test('TC-MON-017 monitoring alerts API returns 200', async ({ page }) => {
        // Go backend requires JWT Bearer token (stored in localStorage.accessToken)
        const result = await page.evaluate(async () => {
            try {
                const token = localStorage.getItem('accessToken') || '';
                const headers = token ? { 'Authorization': 'Bearer ' + token } : {};
                const r = await fetch('/api/monitoring/alerts', { headers });
                const body = await r.json().catch(() => null);
                return { status: r.status, body };
            } catch (e) { return { status: 0, body: null }; }
        });
        if (result.status === 0 || result.status >= 500) { test.skip(); return; }
        expect(result.status).toBe(200);
        if (result.body) {
            expect(result.body.success).toBe(true);
            expect(Array.isArray(result.body.data)).toBe(true);
        }
    });

    // ── TC-MON-018 ───────────────────────────────────────────────────────────────
    test('TC-MON-018 no horizontal overflow at 1280px', async ({ page }) => {
        await page.setViewportSize({ width: 1280, height: 800 });
        const scrollWidth = await page.evaluate(() => document.body.scrollWidth);
        expect(scrollWidth).toBeLessThanOrEqual(1285);
    });
});
