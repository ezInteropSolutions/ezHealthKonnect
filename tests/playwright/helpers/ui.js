'use strict';
/**
 * Small UI helpers shared by specs that drive the interfaces list.
 */

const { expect } = require('@playwright/test');

const interfaceRow = (page, id) => page.locator(`tr[data-interface-id="${id}"]`);

/**
 * The interfaces list is paginated (25 per page) and a developer database can hold
 * many interfaces, so walk the pages until the wanted row is on screen.
 * Call after page.goto('/interfaces.html') has settled.
 */
async function goToInterfaceRow(page, id) {
    const row = interfaceRow(page, id);
    for (let i = 0; i < 40; i++) {
        if (await row.isVisible().catch(() => false)) return row;
        const next = page.getByRole('button', { name: /next/i });
        if (!(await next.isVisible().catch(() => false)) || (await next.isDisabled().catch(() => true))) break;
        await next.click();
        await page.waitForTimeout(300);
    }
    await expect(row).toBeVisible({ timeout: 5000 });
    return row;
}

module.exports = { interfaceRow, goToInterfaceRow };
