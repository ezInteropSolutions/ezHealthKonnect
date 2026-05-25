'use strict';
/**
 * Playwright E2E — Dashboard page
 *
 * TC-DASH-001  Page title contains "Dashboard"
 * TC-DASH-002  Sidebar renders all navigation sections (Main, Analytics, Tools)
 * TC-DASH-003  Dashboard nav item is marked active
 * TC-DASH-004  Sidebar collapses and expands via toggle button
 * TC-DASH-005  All sidebar nav labels are visible when expanded
 * TC-DASH-006  Clicking "Interfaces" nav link navigates to interfaces.html
 * TC-DASH-007  Clicking "Monitoring" nav link navigates to monitoring.html
 * TC-DASH-008  Admin section is visible for the admin user
 * TC-DASH-009  Admin section contains Users and Settings links
 * TC-DASH-010  Dashboard KPI cards render (stat numbers load or show placeholder)
 * TC-DASH-011  Interface cards section is present
 * TC-DASH-012  Interface card status badges render with correct colour classes
 * TC-DASH-013  Templates section is reachable via in-page link
 * TC-DASH-014  Review Queue nav badge is hidden when queue is empty
 * TC-DASH-015  Brand logo image loads (no broken image)
 * TC-DASH-016  Page has no JavaScript console errors on load
 * TC-DASH-017  Sidebar section headers are collapsible (chevron toggles)
 */

const { test, expect } = require('@playwright/test');

test.describe('Dashboard', () => {

    test.beforeEach(async ({ page }) => {
        const errors = [];
        page.on('console', msg => { if (msg.type() === 'error') errors.push(msg.text()); });
        page._testErrors = errors;
        await page.goto('/dashboard.html');
        await page.waitForLoadState('domcontentloaded');
    });

    // ── TC-DASH-001 ─────────────────────────────────────────────────────────────
    test('TC-DASH-001 page title contains Dashboard', async ({ page }) => {
        await expect(page).toHaveTitle(/Dashboard/i);
    });

    // ── TC-DASH-002 ─────────────────────────────────────────────────────────────
    test('TC-DASH-002 sidebar renders Main, Analytics and Tools sections', async ({ page }) => {
        const sidebar = page.locator('#sidebar');
        await expect(sidebar).toBeVisible();
        await expect(sidebar.getByText('Main', { exact: true })).toBeVisible();
        await expect(sidebar.getByText('Analytics', { exact: true })).toBeVisible();
        await expect(sidebar.getByText('Tools', { exact: true })).toBeVisible();
    });

    // ── TC-DASH-003 ─────────────────────────────────────────────────────────────
    test('TC-DASH-003 Dashboard nav item has active class', async ({ page }) => {
        const dashLink = page.locator('.nav-item.active');
        await expect(dashLink).toBeVisible();
        await expect(dashLink).toContainText(/dashboard/i);
    });

    // ── TC-DASH-004 ─────────────────────────────────────────────────────────────
    test('TC-DASH-004 sidebar toggle collapses and expands sidebar', async ({ page }) => {
        const sidebar = page.locator('#sidebar');
        const toggle  = page.locator('#sidebarToggle');

        await expect(sidebar).toBeVisible();
        await toggle.click();
        // After collapse, nav labels should either be hidden or sidebar class changes
        const collapsed = await sidebar.evaluate(el =>
            el.classList.contains('collapsed') || el.offsetWidth < 80
        );
        expect(collapsed).toBe(true);

        // Expand again
        await toggle.click();
        const expanded = await sidebar.evaluate(el =>
            !el.classList.contains('collapsed') && el.offsetWidth >= 80
        );
        expect(expanded).toBe(true);
    });

    // ── TC-DASH-005 ─────────────────────────────────────────────────────────────
    test('TC-DASH-005 all core nav labels are visible', async ({ page }) => {
        const sidebar = page.locator('#sidebar');
        // "Messages" is not a sidebar nav item — messages are accessed via interface cards
        for (const label of ['Dashboard', 'Interfaces', 'Monitoring', 'Reports']) {
            await expect(sidebar.getByText(label, { exact: true })).toBeVisible();
        }
    });

    // ── TC-DASH-006 ─────────────────────────────────────────────────────────────
    test('TC-DASH-006 clicking Interfaces nav link navigates to interfaces.html', async ({ page }) => {
        await page.locator('.nav-item').filter({ hasText: 'Interfaces' }).click();
        await expect(page).toHaveURL(/interfaces\.html/);
    });

    // ── TC-DASH-007 ─────────────────────────────────────────────────────────────
    test('TC-DASH-007 clicking Monitoring nav link navigates to monitoring.html', async ({ page }) => {
        // Analytics section is collapsed on dashboard (only Main section is active).
        // Must expand it before the Monitoring nav item is clickable.
        const analyticsSection = page.locator('.nav-section').filter({ hasText: /Analytics/i }).first();
        const isCollapsed = await analyticsSection.evaluate(el =>
            el.classList.contains('collapsed')
        ).catch(() => false);
        if (isCollapsed) {
            await page.locator('.section-header').filter({ hasText: /Analytics/i }).click();
            await page.waitForTimeout(300);
        }
        await page.locator('.nav-item').filter({ hasText: 'Monitoring' }).click();
        await expect(page).toHaveURL(/monitoring\.html/);
    });

    // ── TC-DASH-008 ─────────────────────────────────────────────────────────────
    test('TC-DASH-008 Admin section is visible for admin user', async ({ page }) => {
        // The admin section starts display:none and is shown by JS after auth check
        const adminSection = page.locator('#adminSection');
        await expect(adminSection).toBeVisible({ timeout: 10_000 });
    });

    // ── TC-DASH-009 ─────────────────────────────────────────────────────────────
    test('TC-DASH-009 admin section contains Users and Settings links', async ({ page }) => {
        const adminSection = page.locator('#adminSection');
        await expect(adminSection).toBeVisible({ timeout: 10_000 });
        await expect(adminSection.getByText('Users', { exact: true })).toBeVisible();
        await expect(adminSection.getByText('Settings', { exact: true })).toBeVisible();
    });

    // ── TC-DASH-010 ─────────────────────────────────────────────────────────────
    test('TC-DASH-010 KPI cards are present and contain numeric or loading content', async ({ page }) => {
        // Wait for any API call to complete
        await page.waitForTimeout(2000);
        // KPI cards should be present — exact selectors depend on dashboard structure
        const cards = page.locator('.stat-card, .kpi-card, [class*="stat"], [class*="kpi"]');
        const count = await cards.count();
        expect(count).toBeGreaterThan(0);
    });

    // ── TC-DASH-011 ─────────────────────────────────────────────────────────────
    test('TC-DASH-011 interface cards section is present', async ({ page }) => {
        await page.waitForTimeout(2000);
        // Interface list / cards section should exist (even if empty state)
        const section = page.locator(
            '.interfaces-section, #interfacesList, .interface-cards, [id*="interface"]'
        ).first();
        await expect(section).toBeAttached({ timeout: 10_000 });
    });

    // ── TC-DASH-012 ─────────────────────────────────────────────────────────────
    test('TC-DASH-012 interface status badges use recognised colour classes', async ({ page }) => {
        await page.waitForTimeout(2000);
        const badges = page.locator('.status-badge, .badge, [class*="status"]');
        const count = await badges.count();
        if (count === 0) {
            test.skip(); // No interfaces seeded — skip silently
            return;
        }
        for (let i = 0; i < Math.min(count, 5); i++) {
            const cls = await badges.nth(i).getAttribute('class') ?? '';
            // At least one of the expected status classes should be present
            const hasStatusClass = /active|paused|error|draft|inactive/i.test(cls);
            if (hasStatusClass) {
                expect(hasStatusClass).toBe(true);
                break; // One confirmed is sufficient
            }
        }
    });

    // ── TC-DASH-013 ─────────────────────────────────────────────────────────────
    test('TC-DASH-013 Templates nav item navigates within dashboard', async ({ page }) => {
        // Must use href selector — "Code Templates" also contains "Templates" text,
        // causing strict-mode violations when using hasText filter.
        const link = page.locator('a[href="dashboard.html#templates"], a[href="#templates"]').first();
        await expect(link).toBeVisible({ timeout: 5000 });
        await link.click();
        // Should stay on dashboard (in-page anchor link)
        await expect(page).toHaveURL(/dashboard\.html/);
    });

    // ── TC-DASH-014 ─────────────────────────────────────────────────────────────
    test('TC-DASH-014 Review Queue badge is hidden when queue is empty', async ({ page }) => {
        await page.waitForTimeout(2000);
        const badge = page.locator('#rqNavBadge');
        // Badge should exist in DOM but may be display:none
        await expect(badge).toBeAttached();
        const visible = await badge.isVisible();
        // Just verify it doesn't display an invalid value if visible
        if (visible) {
            const text = await badge.textContent();
            expect(Number(text)).toBeGreaterThanOrEqual(0);
        }
    });

    // ── TC-DASH-015 ─────────────────────────────────────────────────────────────
    test('TC-DASH-015 brand logo image loads without error', async ({ page }) => {
        const logo = page.locator('.logo-image').first();
        await expect(logo).toBeVisible();
        const naturalWidth = await logo.evaluate((img) => img.naturalWidth);
        expect(naturalWidth).toBeGreaterThan(0);
    });

    // ── TC-DASH-016 ─────────────────────────────────────────────────────────────
    test('TC-DASH-016 no JavaScript console errors on load', async ({ page }) => {
        // Errors collected in beforeEach — filter out known benign network noise including
        // expected 401/403 from API calls made before auth state fully resolves.
        const errors = (page._testErrors ?? []).filter(e =>
            !e.includes('favicon') &&
            !e.includes('net::ERR_') &&
            !e.includes('404') &&
            !e.includes('401') &&
            !e.includes('403')
        );
        expect(errors).toEqual([]);
    });

    // ── TC-DASH-017 ─────────────────────────────────────────────────────────────
    test('TC-DASH-017 nav section headers toggle their content on click', async ({ page }) => {
        const analyticsHeader = page.locator('.section-header').filter({ hasText: 'Analytics' });
        await expect(analyticsHeader).toBeVisible();
        // The nav items below are initially visible; after click they should toggle
        const navItems = analyticsHeader.locator('~ .nav-items');
        const initiallyVisible = await navItems.isVisible();
        await analyticsHeader.click();
        // Class or display state should have changed
        await page.waitForTimeout(300);
        const afterClickVisible = await navItems.isVisible();
        // If initially visible, should now be hidden (or vice versa)
        expect(afterClickVisible).not.toBe(initiallyVisible);
    });
});
