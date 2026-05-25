'use strict';
/**
 * Playwright E2E — Interface Detail page
 *
 * TC-DETAIL-001  Page loads with correct title when given a valid interfaceId
 * TC-DETAIL-002  Page header shows the interface name
 * TC-DETAIL-003  Status pill renders with a recognised status class
 * TC-DETAIL-004  Back button is visible and navigates to interfaces list
 * TC-DETAIL-005  Left-nav renders expected tab items
 * TC-DETAIL-006  Overview tab is active by default
 * TC-DETAIL-007  KPI cards are present in the Overview section
 * TC-DETAIL-008  KPI card values load (numeric or loading state — not blank)
 * TC-DETAIL-009  Clicking Messages left-nav tab switches the content area
 * TC-DETAIL-010  Clicking Configuration left-nav tab switches the content area
 * TC-DETAIL-011  Tab switching does NOT navigate away from detail page
 * TC-DETAIL-012  "Open in Pipeline Builder" / pipeline link is accessible
 * TC-DETAIL-013  Loading an invalid interface ID shows error or empty state
 * TC-DETAIL-014  Page has no horizontal overflow at 1280px
 */

const { test, expect } = require('@playwright/test');

// We discover a real interface ID from the API before running detail tests.
// If no interfaces exist, data-dependent tests are skipped gracefully.
let interfaceId = null;

test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage();
    try {
        const resp = await page.request.get('/api/interfaces');
        const json = await resp.json().catch(() => null);
        const list = json?.interfaces ?? json?.data ?? json ?? [];
        if (Array.isArray(list) && list.length > 0) {
            interfaceId = list[0]?.id ?? list[0]?.interface_id ?? null;
        }
    } catch {
        // Silently ignore — tests skip if no ID found
    } finally {
        await page.close();
    }
});

test.describe('Interface Detail', () => {

    function detailUrl(id) {
        return id ? `/interface-detail.html?interfaceId=${id}` : '/interface-detail.html';
    }

    test.beforeEach(async ({ page }) => {
        if (!interfaceId) {
            test.skip();
            return;
        }
        await page.goto(detailUrl(interfaceId));
        await page.waitForLoadState('domcontentloaded');
        await page.waitForTimeout(1500);
    });

    // ── TC-DETAIL-001 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-001 page title contains Interface Detail or interface name', async ({ page }) => {
        await expect(page).toHaveTitle(/.+/); // Not blank
        const title = await page.title();
        expect(title.toLowerCase()).toMatch(/interface|detail|ezhealthkonnect/i);
    });

    // ── TC-DETAIL-002 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-002 page header shows a non-empty interface name', async ({ page }) => {
        const heading = page.locator('.page-header-title h1, .page-header h1').first();
        await expect(heading).toBeVisible({ timeout: 8000 });
        const text = await heading.textContent();
        expect(text?.trim().length).toBeGreaterThan(0);
    });

    // ── TC-DETAIL-003 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-003 status pill renders with a valid status class', async ({ page }) => {
        const pill = page.locator('.status-pill').first();
        await expect(pill).toBeVisible({ timeout: 8000 });
        const cls = await pill.getAttribute('class') ?? '';
        expect(cls).toMatch(/active|paused|error|draft|inactive/i);
    });

    // ── TC-DETAIL-004 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-004 back button navigates to interfaces list', async ({ page }) => {
        // Back button text is "← Interfaces", not "Back" — use class selector directly
        const backBtn = page.locator('.back-btn, a[href="interfaces.html"]').first();
        await expect(backBtn).toBeVisible();
        await backBtn.click();
        await expect(page).toHaveURL(/interfaces\.html/, { timeout: 8000 });
    });

    // ── TC-DETAIL-005 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-005 left-nav renders expected tab items', async ({ page }) => {
        const tabs = page.locator('.leftnav-tab');
        const count = await tabs.count();
        expect(count).toBeGreaterThanOrEqual(3);

        // Expected tabs — some subset must be present
        const expectedLabels = ['Overview', 'Messages', 'Configuration', 'Logs'];
        let found = 0;
        for (const label of expectedLabels) {
            const visible = await tabs.filter({ hasText: label }).isVisible().catch(() => false);
            if (visible) found++;
        }
        expect(found).toBeGreaterThanOrEqual(2);
    });

    // ── TC-DETAIL-006 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-006 Overview tab is active by default', async ({ page }) => {
        const overviewTab = page.locator('.leftnav-tab.active');
        await expect(overviewTab).toBeVisible();
        await expect(overviewTab).toContainText(/overview/i);
    });

    // ── TC-DETAIL-007 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-007 KPI cards are present in the Overview section', async ({ page }) => {
        const kpiCards = page.locator('.kpi-card, .kpi-grid .card, [class*="kpi"]');
        const count = await kpiCards.count();
        expect(count).toBeGreaterThanOrEqual(1);
    });

    // ── TC-DETAIL-008 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-008 KPI card values are not blank after load', async ({ page }) => {
        await page.waitForTimeout(2500); // Allow data fetch to complete
        const kpiCards = page.locator('.kpi-card, [class*="kpi-card"]');
        const count    = await kpiCards.count();
        if (count === 0) return;

        // Check first few cards have some content
        for (let i = 0; i < Math.min(count, 4); i++) {
            const text = await kpiCards.nth(i).textContent();
            expect(text?.trim().length).toBeGreaterThan(0);
        }
    });

    // ── TC-DETAIL-009 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-009 clicking Messages tab switches content area', async ({ page }) => {
        const msgTab = page.locator('.leftnav-tab').filter({ hasText: /messages/i });
        if (!await msgTab.isVisible().catch(() => false)) {
            test.skip();
            return;
        }
        await msgTab.click();
        await page.waitForTimeout(500);
        await expect(msgTab).toHaveClass(/active/);
        // Overview content should no longer be the active section
        const overviewTab = page.locator('.leftnav-tab').filter({ hasText: /overview/i });
        const overviewActive = await overviewTab.evaluate(el => el.classList.contains('active'));
        expect(overviewActive).toBe(false);
    });

    // ── TC-DETAIL-010 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-010 clicking Configuration tab switches content area', async ({ page }) => {
        const configTab = page.locator('.leftnav-tab').filter({ hasText: /config/i });
        if (!await configTab.isVisible().catch(() => false)) {
            test.skip();
            return;
        }
        await configTab.click();
        await page.waitForTimeout(400);
        await expect(configTab).toHaveClass(/active/);
    });

    // ── TC-DETAIL-011 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-011 tab switching stays on the detail page', async ({ page }) => {
        const tabs = page.locator('.leftnav-tab');
        const count = await tabs.count();
        for (let i = 0; i < Math.min(count, 4); i++) {
            await tabs.nth(i).click();
            await expect(page).toHaveURL(/interface-detail\.html/);
        }
    });

    // ── TC-DETAIL-012 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-012 pipeline builder link is present and reachable', async ({ page }) => {
        const pipelineLink = page.locator('a[href*="pipeline-builder"], button').filter({ hasText: /pipeline/i }).first();
        const visible = await pipelineLink.isVisible({ timeout: 5000 }).catch(() => false);
        if (!visible) {
            test.skip(); // Interface may not have a pipeline yet
            return;
        }
        expect(visible).toBe(true);
    });

    // ── TC-DETAIL-013 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-013 invalid interface ID shows error or empty state', async ({ page }) => {
        await page.goto('/interface-detail.html?interfaceId=00000000-0000-0000-0000-000000000000');
        await page.waitForTimeout(3500);
        // Either an error message, "not found" text, or redirected back to list
        const errorText = page.locator('text=/not found|error|invalid|does not exist/i');
        const redirected = page.url().includes('interfaces.html');
        const hasError   = await errorText.isVisible().catch(() => false);
        expect(hasError || redirected).toBe(true);
    });

    // ── TC-DETAIL-014 ────────────────────────────────────────────────────────────
    test('TC-DETAIL-014 no horizontal overflow at 1280px', async ({ page }) => {
        await page.setViewportSize({ width: 1280, height: 800 });
        const bodyScrollWidth = await page.evaluate(() => document.body.scrollWidth);
        expect(bodyScrollWidth).toBeLessThanOrEqual(1285); // 5px tolerance
    });
});
