'use strict';
/**
 * hl7-reader-validation.spec.js — HL7 v2 conformance validator E2E tests.
 *
 * Covers public/hl7-reader.html's new Validation Level selector (Basic /
 * Standard / Strict) wired to hl7/validator via POST /api/hl7/parse's
 * `validationLevel` field, and segment-viewer.js's new "Missing Required
 * Segments" banner. Mirrors fhir-hl7-build-e2e.spec.js's structure.
 *
 * Run: npx playwright test hl7-reader-validation --project=chromium
 */

const { test, expect } = require('@playwright/test');

// A structurally valid ADT^A01 missing the required PID segment (only
// EVN/PV1 present) — exercises hl7/validator's missing-segment category.
const ADT_A01_MISSING_PID =
    'MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240101120000||ADT^A01|MSG001|P|2.5.1\r' +
    'EVN|A01|20240101120000\r' +
    'PV1|1|I';

// A valid ADT^A01 with two PID segments — PID cannot repeat in this message
// type, exercising the cardinality category.
const ADT_A01_DUPLICATE_PID =
    'MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240101120000||ADT^A01|MSG001|P|2.5.1\r' +
    'EVN|A01|20240101120000\r' +
    'PID|1||12345^^^MRN||DOE^JOHN^A||19800115|M\r' +
    'PID|2||99999^^^MRN||DOE^JOHN^A||19800115|M\r' +
    'PV1|1|I';

// A valid ADT^A01 with an invalid PID.8 (Administrative Sex, HL7 table 0001)
// code — exercises the table-binding category and its Standard(warning) vs
// Strict(error) severity escalation.
const ADT_A01_BAD_SEX_CODE =
    'MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240101120000||ADT^A01|MSG001|P|2.5.1\r' +
    'EVN|A01|20240101120000\r' +
    'PID|1||12345^^^MRN||DOE^JOHN^A||19800115|Z\r' +
    'PV1|1|I';

async function parseAt(page, rawMessage, level) {
    await page.fill('#hl7Input', rawMessage);
    if (level) {
        await page.selectOption('#validationLevel', level);
    }
    await page.click('#parseBtn');
    await expect(page.locator('#segmentViewerContainer')).toBeVisible();
    // The compact header renders synchronously once the fetch resolves —
    // wait for at least one segment row so we know rendering finished.
    await expect(page.locator('.segment-header-compact')).toBeVisible();
}

test.describe('HL7 Reader — Conformance Validation', () => {
    test.beforeEach(async ({ page }) => {
        await page.goto('/hl7-reader.html');
        await expect(page.locator('#hl7Input')).toBeVisible();
    });

    test('Basic level: missing PID produces no missing-segment banner', async ({ page }) => {
        await parseAt(page, ADT_A01_MISSING_PID, 'basic');
        await expect(page.locator('.missing-segments-banner')).toHaveCount(0);
    });

    test('Standard level: missing PID shows the missing-segment banner and an error metric', async ({ page }) => {
        await parseAt(page, ADT_A01_MISSING_PID, 'standard');
        const banner = page.locator('.missing-segments-banner');
        await expect(banner).toBeVisible();
        await expect(banner).toContainText('PID');
        await expect(page.locator('.metric.error')).toBeVisible();
    });

    test('Strict level: missing PID still shows the missing-segment banner', async ({ page }) => {
        await parseAt(page, ADT_A01_MISSING_PID, 'strict');
        await expect(page.locator('.missing-segments-banner')).toBeVisible();
    });

    test('Standard level: duplicate non-repeating PID raises an error metric', async ({ page }) => {
        await parseAt(page, ADT_A01_DUPLICATE_PID, 'standard');
        await expect(page.locator('.metric.error')).toBeVisible();
        // Cardinality errors attach to a real segment card (unlike missing
        // segments), so no banner is expected here.
        await expect(page.locator('.missing-segments-banner')).toHaveCount(0);
    });

    test('Standard vs Strict: an invalid table code is a warning at Standard and an error at Strict', async ({ page }) => {
        await parseAt(page, ADT_A01_BAD_SEX_CODE, 'standard');
        await expect(page.locator('.metric.warning')).toBeVisible();

        await parseAt(page, ADT_A01_BAD_SEX_CODE, 'strict');
        await expect(page.locator('.metric.error')).toBeVisible();
        await expect(page.locator('.metric.warning')).toHaveCount(0);
    });
});
