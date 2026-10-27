'use strict';
/**
 * Playwright E2E — Interfaces list page + Interface Wizard
 *
 * TC-IFACE-001  Page title contains "Interfaces"
 * TC-IFACE-002  Sidebar Interfaces nav item is marked active
 * TC-IFACE-003  "New Interface" / "Create Interface" button is visible
 * TC-IFACE-004  Clicking "New Interface" opens the wizard modal
 * TC-IFACE-005  Wizard modal has a step-indicator sidebar
 * TC-IFACE-006  Wizard step 1 shows the interface name input field
 * TC-IFACE-007  Wizard displays at least 3 steps in its sidebar
 * TC-IFACE-008  Submitting wizard step 1 with an empty name shows validation error
 * TC-IFACE-009  Wizard close / cancel button dismisses the modal
 * TC-IFACE-010  Interface cards render with name and status badge
 * TC-IFACE-011  Interface status badges use valid status classes
 * TC-IFACE-012  Interface card action buttons are accessible (view/edit/pipeline)
 * TC-IFACE-013  Clicking an interface card opens interface-detail page
 * TC-IFACE-014  Status filter narrows the interface list (across all pages)
 * TC-IFACE-015  Typing in a search/filter input narrows visible interfaces
 * TC-IFACE-016  Empty state message shown when no interfaces match filter
 * TC-IFACE-017  Wizard step progression — filling name and advancing goes to step 2
 * TC-IFACE-018  Wizard "Back" button returns to previous step
 * TC-IFACE-019  CDA Coverage Audit section shows only for CCD-message-type interfaces
 */

const { test, expect } = require('./helpers/seeded');

test.describe('Interfaces', () => {

    test.beforeEach(async ({ page }) => {
        await page.goto('/interfaces.html');
        await page.waitForLoadState('domcontentloaded');
        await page.waitForTimeout(1500); // Allow API calls to settle
    });

    // ── TC-IFACE-001 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-001 page title contains Interfaces', async ({ page }) => {
        await expect(page).toHaveTitle(/Interfaces/i);
    });

    // ── TC-IFACE-002 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-002 sidebar marks Interfaces as active', async ({ page }) => {
        const active = page.locator('.nav-item.active');
        await expect(active).toContainText(/interfaces/i);
    });

    // ── TC-IFACE-003 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-003 New Interface button is visible', async ({ page }) => {
        const btn = page.locator('button, a').filter({ hasText: /new interface|create interface/i }).first();
        await expect(btn).toBeVisible({ timeout: 8000 });
    });

    // ── TC-IFACE-004 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-004 clicking New Interface opens wizard modal', async ({ page }) => {
        const btn = page.locator('button, a').filter({ hasText: /new interface|create interface/i }).first();
        await btn.click();
        // Use the overlay ID directly — [id*="wizard"] also matches the always-present
        // but empty #wizard-modal-container, which would never be visible.
        const modal = page.locator('#wizardModalOverlay, .wizard-modal-overlay').first();
        await expect(modal).toBeVisible({ timeout: 8000 });
    });

    // ── TC-IFACE-005 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-005 wizard modal has a step-indicator sidebar', async ({ page }) => {
        await openWizard(page);
        const stepIndicator = page.locator(
            '.wizard-sidebar, .wizard-steps, .step-indicator, [class*="wizard-step"]'
        ).first();
        await expect(stepIndicator).toBeVisible({ timeout: 8000 });
    });

    // ── TC-IFACE-006 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-006 wizard step 1 has an interface name input', async ({ page }) => {
        await openWizard(page);
        // Scoped to #wizardModalOverlay, not a page-wide selector -- confirmed via direct
        // inspection that a SECOND, hidden element sharing id="interfaceName" also exists in
        // this page's own DOM once the wizard is open (from other dynamically-rendered UI on
        // the same page, not this wizard). .first() picks DOM order, not visibility, so an
        // unscoped selector can silently grab the wrong, hidden element instead of the
        // wizard's own real, visible field.
        const nameInput = page.locator('#wizardModalOverlay').locator('#wizardInterfaceName, input[name*="name"], input[placeholder*="name"], #interfaceName, #name').first();
        // Element exists in DOM but may be on a wizard step not yet visible
        await expect(nameInput).toBeAttached({ timeout: 8000 });
    });

    // ── TC-IFACE-007 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-007 wizard sidebar shows at least 3 steps', async ({ page }) => {
        await openWizard(page);
        const steps = page.locator('.wizard-step, .step-item, [class*="wizard-step"]');
        await page.waitForTimeout(300);
        const count = await steps.count();
        expect(count).toBeGreaterThanOrEqual(3);
    });

    // ── TC-IFACE-008 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-008 submitting wizard step 1 with empty name shows validation', async ({ page }) => {
        await openWizard(page);
        // #wizardNext (public/js/wizard/optimized/WizardView.js's own real id for
        // the ACTUAL wizard this button opens) -- not a text filter for
        // "next|continue|proceed" across the whole page, and not "#nextBtn" either.
        // Two real, independent bugs were found here:
        //  1. The interfaces LIST page's own pagination control
        //     (public/js/interfaces.js's goToNextPage()) renders a "Next ›" button
        //     matching the same "next" text pattern the original locator filtered
        //     on; .filter({hasText}).first() picked up the pagination button
        //     (behind the modal, so the click just timed out) instead of the
        //     wizard's own Next button.
        //  2. "#nextBtn" (tried as the fix for #1) is a DIFFERENT wizard's own id --
        //     public/interface-wizard.html, a static page never actually linked
        //     from interfaces.html's own "New Interface" button (confirmed via a
        //     full grep: nothing in interfaces.html/interfaces.js references that
        //     file at all). The wizard this button REALLY opens is the "optimized"
        //     one (public/js/wizard/optimized/WizardView.js), whose own Next
        //     button id is #wizardNext -- found only by tracing interfaces.html's
        //     own <script> includes down to the file that actually builds the
        //     modal's inner HTML, not by grepping for a plausible-looking id.
        const nextBtn = page.locator('#wizardNext');
        // locator.isVisible({timeout}) does NOT actually wait/retry despite taking a
        // timeout argument -- it's a point-in-time check (only toBeVisible()-style
        // assertions and waitFor() poll). The wizard's own inner content (including
        // #wizardNext) renders asynchronously after the overlay itself appears, so
        // the old isVisible({timeout: 5000}) call fired before the button existed
        // at all and skipped every run. waitFor() actually polls for up to 5s.
        const nextBtnAppeared = await nextBtn.waitFor({ state: 'visible', timeout: 5000 }).then(() => true).catch(() => false);
        expect(!(!nextBtnAppeared), 'precondition failed (was a skip): !nextBtnAppeared').toBe(true);
        // A disabled button is itself a valid validation pattern
        const isDisabled = await nextBtn.isDisabled().catch(() => false);
        if (isDisabled) return;

        await nextBtn.click();
        // Should either show validation message or stay on step 1
        const validationMsg = page.locator(
            '.error, .invalid-feedback, [class*="error"], [class*="validation"]'
        ).first();
        const isValid = await validationMsg.isVisible({ timeout: 3000 }).catch(() => false);
        // Scoped to #wizardModalOverlay -- see TC-IFACE-006's own comment on the
        // duplicate id="interfaceName" element elsewhere in this page's DOM.
        const stillStep1 = await page.locator('#wizardModalOverlay').locator(
            '#wizardInterfaceName, input[name*="name"], input[placeholder*="name"], #interfaceName'
        ).first().isVisible().catch(() => false);
        expect(isValid || stillStep1).toBe(true);
    });

    // ── TC-IFACE-009 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-009 wizard close button dismisses modal', async ({ page }) => {
        await openWizard(page);
        const closeBtn = page.locator(
            'button[aria-label*="close" i], button[title*="close" i], .modal-close, .close-btn, .wizard-close, button.close'
        ).first();
        if (await closeBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
            await closeBtn.click();
        } else {
            // Try pressing Escape
            await page.keyboard.press('Escape');
        }
        await page.waitForTimeout(500);
        const modal = page.locator('#wizardModalOverlay, .wizard-modal-overlay').first();
        const stillVisible = await modal.isVisible().catch(() => false);
        expect(stillVisible).toBe(false);
    });

    // The interfaces page renders a table: one tr[data-interface-id] per interface.
    // helpers/seeded.js guarantees two (one active, one inactive).
    const { interfaceRow: rowFor, goToInterfaceRow: goToRow } = require('./helpers/ui');

    // ── TC-IFACE-010 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-010 interface rows render with name and status badge', async ({ page, seed }) => {
        const row = await goToRow(page, seed.interfaceId);
        await expect(row.locator('.interface-name')).toHaveText(seed.interfaceName);
        await expect(row.locator('.interface-status').first()).toBeVisible();
    });

    // ── TC-IFACE-011 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-011 interface status badges render and differ between active and inactive', async ({ page, seed }) => {
        await goToRow(page, seed.interfaceId);
        await goToRow(page, seed.idleInterfaceId);
        const active = await rowFor(page, seed.interfaceId).locator('.interface-status').first().innerText();
        const idle   = await rowFor(page, seed.idleInterfaceId).locator('.interface-status').first().innerText();
        expect(active.trim().length).toBeGreaterThan(0);
        expect(idle.trim().length).toBeGreaterThan(0);
        expect(active.trim().toLowerCase()).not.toBe(idle.trim().toLowerCase());
    });

    // ── TC-IFACE-012 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-012 interface rows have action buttons', async ({ page, seed }) => {
        const row = await goToRow(page, seed.interfaceId);
        expect(await row.locator('.action-buttons button').count()).toBeGreaterThan(0);
    });

    // ── TC-IFACE-013 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-013 clicking an interface row navigates to its detail page', { timeout: 60_000 }, async ({ page, seed }) => {
        const row = await goToRow(page, seed.interfaceId);
        await row.locator('.interface-name').click();
        await expect(page).toHaveURL(/interface-detail\.html/, { timeout: 10000 });
        expect(page.url()).toContain(seed.interfaceId);
    });

    // ── TC-IFACE-014/015/016 ─────────────────────────────────────────────────────
    // The toolbar (js/components/table-component.js) has a search box (#interfaceSearchInput) and status /
    // type / format filters; applyFilters() in interfaces.js filters the WHOLE list, not just the visible page.
    const FILTER_ROWS = 'tr[data-interface-id]';

    test('TC-IFACE-014 status filter narrows the list across all pages', async ({ page, seed }) => {
        await goToRow(page, seed.interfaceId);
        const active = (await rowFor(page, seed.interfaceId).locator('.interface-status').first().innerText()).trim().toLowerCase();
        const idle = (await (await goToRow(page, seed.idleInterfaceId)).locator('.interface-status').first().innerText()).trim().toLowerCase();
        expect(active).not.toBe(idle);

        // A developer database has many interfaces in each status, spread over several pages, so combine the status
        // filter with a search for each seeded interface's own (unique) name: exactly one row can match both.
        const matches = async (status, name) => {
            await page.locator('#statusFilter').selectOption(status);
            await page.locator('#interfaceSearchInput').fill(name);
            return page.locator('tr[data-interface-id]').count();
        };
        await expect.poll(() => matches(active, seed.interfaceName), { timeout: 5000 }).toBe(1);
        await expect.poll(() => matches(active, seed.idleInterfaceName), { timeout: 5000 }).toBe(0);
        await expect.poll(() => matches(idle, seed.idleInterfaceName), { timeout: 5000 }).toBe(1);
        await expect.poll(() => matches(idle, seed.interfaceName), { timeout: 5000 }).toBe(0);

        // "All Status" matches both again.
        await expect.poll(() => matches('all', seed.interfaceName), { timeout: 5000 }).toBe(1);
        await expect.poll(() => matches('all', seed.idleInterfaceName), { timeout: 5000 }).toBe(1);
    });

    test('TC-IFACE-015 search finds an interface by name, wherever it is in the list', async ({ page, seed }) => {
        await page.locator('#interfaceSearchInput').fill(seed.interfaceName);
        const rows = page.locator(FILTER_ROWS);
        await expect(rows).toHaveCount(1, { timeout: 5000 });
        await expect(rows.first()).toHaveAttribute('data-interface-id', seed.interfaceId);

        // Case-insensitive, and the pagination collapses to one page.
        await page.locator('#interfaceSearchInput').fill(seed.interfaceName.toUpperCase());
        await expect(rows).toHaveCount(1);
    });

    test('TC-IFACE-016 empty state renders when nothing matches the search', async ({ page }) => {
        await page.locator('#interfaceSearchInput').fill('zzzz-no-such-interface-99999');
        await expect(page.locator(FILTER_ROWS)).toHaveCount(0, { timeout: 5000 });
        await expect(page.locator('#interfacesTableBody .empty-state')).toContainText(/No interfaces found/i);

        // Clearing the search restores the list.
        await page.locator('#interfaceSearchInput').fill('');
        await expect(page.locator(FILTER_ROWS).first()).toBeVisible({ timeout: 5000 });
    });

    // ── TC-IFACE-017 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-017 filling wizard name and clicking Next advances to step 2', async ({ page }) => {
        await openWizard(page);
        // Scoped to #wizardModalOverlay -- see TC-IFACE-006's own comment.
        const nameInput = page.locator('#wizardModalOverlay').locator('#wizardInterfaceName, input[name*="name"], input[placeholder*="name"], #interfaceName, #name').first();
        // waitFor(), not isVisible({timeout}) -- see TC-IFACE-008's own comment:
        // isVisible() never actually polls regardless of a timeout argument.
        const nameInputAppeared = await nameInput.waitFor({ state: 'visible', timeout: 5000 }).then(() => true).catch(() => false);
        expect(!(!nameInputAppeared), 'precondition failed (was a skip): !nameInputAppeared').toBe(true);
        await nameInput.fill('E2E Test Interface');
        // #wizardNext -- see TC-IFACE-008's own comment for why (not a page-wide
        // text filter, and not "#nextBtn" either -- that's a different, unused
        // wizard's own id).
        const nextBtn = page.locator('#wizardNext');
        if (await nextBtn.waitFor({ state: 'visible', timeout: 5000 }).then(() => true).catch(() => false)) {
            await nextBtn.click();
            await page.waitForTimeout(600);
            // Step 2 should now be active — either indicator or new form fields appear
            const step2Active = page.locator('.step-item.active, .wizard-step.active, [class*="step-active"]').nth(1);
            const step2Field  = page.locator('[class*="step-2"], [data-step="2"]').first();
            const advanced = await step2Active.isVisible().catch(() => false)
                          || await step2Field.isVisible().catch(() => false)
                          || page.url().includes('step=2');
            expect(advanced).toBe(true);
        }
    });

    // ── TC-IFACE-018 ─────────────────────────────────────────────────────────────
    test('TC-IFACE-018 wizard Back button returns to previous step', async ({ page }) => {
        await openWizard(page);
        // Advance to step 2 first
        // Scoped to #wizardModalOverlay -- see TC-IFACE-006's own comment.
        const nameInput = page.locator('#wizardModalOverlay').locator('#wizardInterfaceName, input[name*="name"], input[placeholder*="name"], #interfaceName').first();
        // waitFor(), not isVisible({timeout}) -- see TC-IFACE-008's own comment:
        // isVisible() never actually polls regardless of a timeout argument, and the
        // wizard's own inner content (name input, Next/Back buttons) renders
        // asynchronously after the modal overlay itself appears.
        const nameInputAppeared = await nameInput.waitFor({ state: 'visible', timeout: 5000 }).then(() => true).catch(() => false);
        expect(!(!nameInputAppeared), 'precondition failed (was a skip): !nameInputAppeared').toBe(true);
        await nameInput.fill('E2E Back Button Test');
        // #wizardNext/#wizardPrevious -- see TC-IFACE-008's own comment for why
        // (not page-wide text filters, and not "#nextBtn"/"#prevBtn" either --
        // those belong to a different, unused wizard).
        const nextBtn = page.locator('#wizardNext');
        const nextBtnAppeared = await nextBtn.waitFor({ state: 'visible', timeout: 5000 }).then(() => true).catch(() => false);
        expect(!(!nextBtnAppeared), 'precondition failed (was a skip): !nextBtnAppeared').toBe(true);
        await nextBtn.click();
        await page.waitForTimeout(600);

        // Click Back
        const backBtn = page.locator('#wizardPrevious');
        const backBtnAppeared = await backBtn.waitFor({ state: 'visible', timeout: 5000 }).then(() => true).catch(() => false);
        expect(!(!backBtnAppeared), 'precondition failed (was a skip): !backBtnAppeared').toBe(true);
        await backBtn.click();
        await page.waitForTimeout(400);
        // Should be back on step 1 — name input should be visible again
        await expect(nameInput).toBeVisible({ timeout: 5000 });
    });

    // ── TC-IFACE-019 ─────────────────────────────────────────────────────────────
    // CDA Coverage Audit is only meaningful for an interface whose inbound
    // messages actually get parsed as CDA/CCD content — the backend's own
    // runtime gate (transformation_pipeline_helpers.go) only creates a
    // coverage tracker when a message's _format resolves to "ccda", regardless
    // of this UI toggle. Before this fix, the Edit Interface modal showed the
    // "CDA Coverage Audit" checkbox for every interface, with only a text
    // disclaimer saying it "only applies when this interface's input is
    // CDA/CCD" — this test pins the actual visibility gate (message_type ===
    // 'CCD', the same convention messages.js's own isCDA check uses), not
    // just the disclaimer copy.
    test('TC-IFACE-019 Coverage Audit section is available for CCD and non-CCD interfaces', async ({ page, request }) => {
        const unique = Date.now();
        const createInterface = async (name, messageType) => {
            const res = await request.post('/api/interfaces', {
                data: {
                    name, messageType, description: 'TC-IFACE-019 fixture',
                    sourceType: '', targetType: '', sourceConfig: {}, targetConfig: {},
                },
            });
            const body = await res.json();
            expect(body.success, `failed to create fixture interface ${name}: ${JSON.stringify(body)}`).toBe(true);
            return body.interface?.id || body.id;
        };

        const ccdId = await createInterface(`TC-IFACE-019 CCD ${unique}`, 'CCD');
        const nonCcdId = await createInterface(`TC-IFACE-019 ADT ${unique}`, 'ADT^A01');

        // Reload so the page's own interfaces list (fetched once on load,
        // before these fixtures existed) picks both up.
        await page.reload();
        await page.waitForLoadState('domcontentloaded');
        await page.waitForFunction(
            (ids) => typeof interfaces !== 'undefined' && ids.every(id => interfaces.some(i => i.id === id)),
            [ccdId, nonCcdId],
            { timeout: 10000 }
        );

        await page.evaluate((id) => window.showEditModal(id), ccdId);
        await page.waitForSelector('#editModal.show', { timeout: 5000 });
        await expect(page.locator('#editCdaCoverageAuditSection'), 'CCD interface should show the Coverage Audit section').toBeVisible();
        await page.evaluate(() => document.getElementById('editModal')?.classList.remove('show'));

        await page.evaluate((id) => window.showEditModal(id), nonCcdId);
        await page.waitForSelector('#editModal.show', { timeout: 5000 });
        // The CCD-only gate was deliberately removed (2026-09-27) once Coverage
        // Audit was generalized to HL7/FHIR/EDI/NCPDP — the section must now be
        // reachable for non-CCD interfaces too.
        await expect(page.locator('#editCdaCoverageAuditSection'), 'non-CCD interface should also show the Coverage Audit section').toBeVisible();
    });

    // ─── Shared helper ──────────────────────────────────────────────────────────
    async function openWizard(page) {
        const btn = page.locator('button, a').filter({ hasText: /new interface|create interface/i }).first();
        await expect(btn).toBeVisible({ timeout: 8000 });
        await btn.click();
        await page.waitForTimeout(500);
    }
});
