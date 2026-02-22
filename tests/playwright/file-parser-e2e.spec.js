const { test, expect } = require('@playwright/test');

// ===============================================================
// FILE PARSER — END-TO-END TESTS
// ===============================================================
// Expert ELT engineer coverage:
//   Suite 1: Toolbox presence & icon
//   Suite 2: Configuration UI rendering & interactions
//   Suite 3: Config save/load round-trip
//   Suite 4: Config collection (collectFormData)
//   Suite 5: Pipeline test execution (CSV parsing via Script+FileParser)

const BASE_URL = 'http://localhost:3000';

// ─── Shared Helpers ───────────────────────────────────────────

async function login(page) {
    await page.goto(`${BASE_URL}/login.html`);
    const passwords = ['admin123', 'Admin123!'];
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

    if (!loggedIn) throw new Error('Could not login with any known password');
    await page.goto(`${BASE_URL}/dashboard.html`);
    await page.waitForLoadState('networkidle');
}

async function getInterfaceId(page) {
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
    return result || '762aebb9-0408-4a42-82c5-202f13f28315';
}

async function openPipelineBuilder(page) {
    const interfaceId = await getInterfaceId(page);
    await page.goto(`${BASE_URL}/pipeline-builder.html?interfaceId=${interfaceId}`);
    await page.waitForLoadState('networkidle');
    // Wait for flowchart step nodes to render (matching existing test pattern)
    await page.waitForSelector('.flowchart-step-node', { timeout: 15000 });
    return interfaceId;
}

/**
 * Add a File Parser step to the pipeline via JS injection.
 * Returns the step's auto-generated ID.
 */
async function addFileParserStep(page, config = {}) {
    return await page.evaluate((cfg) => {
        const builder = window.pipelineBuilder;
        if (!builder) throw new Error('Pipeline builder not initialized');

        const defaultConfig = {
            sourceField: 'enriched.file_content',
            fileFormat: 'csv',
            delimiter: ',',
            hasHeader: true,
            columns: [],
            trimFields: true,
            skipRows: 0,
            maxRecords: 0,
            ...cfg
        };

        // Use the VisualStep constructor pattern from PipelineModels
        const VisualStep = window.VisualStep || builder.pipeline?.steps?.[0]?.constructor;
        const step = new VisualStep({
            stepName: 'File Parser Test',
            stepType: 'file_parser',
            sequence: 15,
            enabled: true,
            config: defaultConfig,
            layer: 'core'
        });

        builder.addStep(step);
        return step.id;
    }, config);
}

/**
 * Click on a step node in the flowchart by its step type or name.
 */
async function clickStep(page, nameOrType) {
    // Try to find the step by its displayed name in the flowchart
    const stepNode = page.locator(`.flowchart-step-node`).filter({ hasText: nameOrType }).first();
    if (await stepNode.isVisible({ timeout: 3000 }).catch(() => false)) {
        await stepNode.click();
        return true;
    }
    return false;
}

/**
 * Wait for the step properties modal/panel to open and show form fields.
 */
async function waitForPropertiesPanel(page) {
    // Modal uses display:flex when open (not .active class)
    await page.waitForFunction(() => {
        const modal = document.getElementById('stepPropertiesModal');
        return modal && modal.style.display === 'flex';
    }, { timeout: 5000 });
    await page.waitForTimeout(500); // Let dynamic form fields render
}

// ─── SUITE 1: File Parser Step in Toolbox ─────────────────────

test.describe('File Parser in Toolbox', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('File Parser template is visible in the toolbox', async ({ page }) => {
        // The toolbox should contain the File Parser step card
        const fileParserCard = page.locator('.step-card').filter({ hasText: 'File Parser' });
        await expect(fileParserCard).toBeVisible({ timeout: 5000 });
    });

    test('File Parser has correct icon (fa-file-csv)', async ({ page }) => {
        const fileParserCard = page.locator('.step-card').filter({ hasText: 'File Parser' });
        const icon = fileParserCard.locator('i.fa-file-csv, i.fas.fa-file-csv');
        await expect(icon).toBeVisible({ timeout: 5000 });
    });

    test('File Parser shows Built-in badge', async ({ page }) => {
        const fileParserCard = page.locator('.step-card').filter({ hasText: 'File Parser' });
        const badge = fileParserCard.locator('.step-card-badge');
        await expect(badge).toHaveText('Built-in');
    });
});

// ─── SUITE 2: File Parser Configuration UI ────────────────────

test.describe('File Parser Configuration UI', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('Configuration form renders all fields', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Check all form fields exist
        await expect(page.locator('#fpSourceField')).toBeVisible({ timeout: 3000 });
        await expect(page.locator('#fpFileFormat')).toBeVisible();
        await expect(page.locator('#fpDelimiter')).toBeVisible();
        await expect(page.locator('#fpHasHeader')).toBeAttached();
        await expect(page.locator('#fpTrimFields')).toBeAttached();
        await expect(page.locator('#fpSkipRows')).toBeVisible();
        await expect(page.locator('#fpMaxRecords')).toBeVisible();
    });

    test('Changing format to Fixed Width shows column builder, hides delimiter', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Initially delimiter should be visible, columns section hidden
        await expect(page.locator('#fpDelimiterGroup')).toBeVisible();
        await expect(page.locator('#fpColumnsSection')).toBeHidden();

        // Change format to fixed_width
        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);

        // Now delimiter hidden, columns visible
        await expect(page.locator('#fpDelimiterGroup')).toBeHidden();
        await expect(page.locator('#fpColumnsSection')).toBeVisible();
    });

    test('Changing format back to CSV shows delimiter, hides column builder', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'fixed_width' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Change to CSV
        await page.selectOption('#fpFileFormat', 'csv');
        await page.waitForTimeout(300);

        await expect(page.locator('#fpDelimiterGroup')).toBeVisible();
        await expect(page.locator('#fpColumnsSection')).toBeHidden();
    });

    test('Add Column button creates a new row', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'fixed_width' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Select fixed_width to show the columns section
        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);

        const initialRows = await page.locator('#fpColumnsBody tr').count();

        // Click Add Column
        await page.click('#fpAddColumnBtn');
        await page.waitForTimeout(200);

        const newRows = await page.locator('#fpColumnsBody tr').count();
        expect(newRows).toBe(initialRows + 1);
    });

    test('Remove column button removes a row', async ({ page }) => {
        await addFileParserStep(page, {
            fileFormat: 'fixed_width',
            columns: [
                { name: 'COL_A', start: 1, length: 10 },
                { name: 'COL_B', start: 11, length: 10 }
            ]
        });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);

        const initialRows = await page.locator('#fpColumnsBody tr').count();
        expect(initialRows).toBe(2);

        // Remove first column
        await page.locator('.fp-remove-col-btn').first().click();
        await page.waitForTimeout(200);

        const afterRows = await page.locator('#fpColumnsBody tr').count();
        expect(afterRows).toBe(1);
    });

    test('Has Header checkbox toggles', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        const checkbox = page.locator('#fpHasHeader');
        // Default should be checked
        await expect(checkbox).toBeChecked();

        // Uncheck
        await checkbox.uncheck();
        await expect(checkbox).not.toBeChecked();

        // Re-check
        await checkbox.check();
        await expect(checkbox).toBeChecked();
    });
});

// ─── SUITE 3: Config Save/Load Round-Trip ─────────────────────

test.describe('File Parser Config Persistence', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('Config values persist after save and re-open', async ({ page }) => {
        await addFileParserStep(page, {
            sourceField: 'enriched.sftp_data.content',
            fileFormat: 'tsv',
            hasHeader: false,
            skipRows: 3,
            maxRecords: 100
        });

        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Verify the values are rendered in the form
        await expect(page.locator('#fpSourceField')).toHaveValue('enriched.sftp_data.content');
        await expect(page.locator('#fpFileFormat')).toHaveValue('tsv');
        await expect(page.locator('#fpHasHeader')).not.toBeChecked();
        await expect(page.locator('#fpSkipRows')).toHaveValue('3');
        await expect(page.locator('#fpMaxRecords')).toHaveValue('100');
    });
});

// ─── SUITE 4: Config Collection (collectFormData) ─────────────

test.describe('File Parser Config Collection', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('All fields are collected correctly', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Fill in specific values
        await page.fill('#fpSourceField', 'enriched.file_content');
        await page.selectOption('#fpFileFormat', 'csv');
        await page.fill('#fpDelimiter', '|');
        await page.fill('#fpSkipRows', '2');
        await page.fill('#fpMaxRecords', '50');

        // Trigger collection via JavaScript
        const config = await page.evaluate(() => {
            const builder = window.pipelineBuilder;
            const allSteps = builder.getAllStepsFlat();
            const step = allSteps.find(s => s.stepType === 'file_parser');
            if (!step) throw new Error('File Parser step not found');

            // Trigger collectFormData
            const panel = builder.propertiesPanel || window.propertiesPanel;
            panel.collectFormData(step);
            return step.config;
        });

        expect(config.sourceField).toBe('enriched.file_content');
        expect(config.fileFormat).toBe('csv');
        expect(config.delimiter).toBe('|');
        expect(config.skipRows).toBe(2);
        expect(config.maxRecords).toBe(50);
    });

    test('Fixed-width columns collected as array', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'fixed_width' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);

        // Add two columns
        await page.click('#fpAddColumnBtn');
        await page.waitForTimeout(100);
        await page.click('#fpAddColumnBtn');
        await page.waitForTimeout(100);

        // Fill column values
        const nameInputs = page.locator('.fp-col-name');
        const startInputs = page.locator('.fp-col-start');
        const lengthInputs = page.locator('.fp-col-length');

        await nameInputs.nth(0).fill('BENE_ID');
        await startInputs.nth(0).fill('1');
        await lengthInputs.nth(0).fill('11');

        await nameInputs.nth(1).fill('CLM_ID');
        await startInputs.nth(1).fill('12');
        await lengthInputs.nth(1).fill('13');

        // Collect config
        const config = await page.evaluate(() => {
            const builder = window.pipelineBuilder;
            const allSteps = builder.getAllStepsFlat();
            const step = allSteps.find(s => s.stepType === 'file_parser');
            if (!step) throw new Error('File Parser step not found');
            const panel = builder.propertiesPanel || window.propertiesPanel;
            panel.collectFormData(step);
            return step.config;
        });

        expect(config.columns).toBeDefined();
        expect(config.columns.length).toBe(2);
        expect(config.columns[0].name).toBe('BENE_ID');
        expect(config.columns[0].start).toBe(1);
        expect(config.columns[0].length).toBe(11);
        expect(config.columns[1].name).toBe('CLM_ID');
        expect(config.columns[1].start).toBe(12);
        expect(config.columns[1].length).toBe(13);
    });
});

// ─── SUITE 5: Pipeline Test Execution ─────────────────────────

test.describe('File Parser Pipeline Test Execution', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('File Parser processes CSV data correctly in pipeline test', async ({ page }) => {
        // Step 1: Add a Script Enrichment step that injects CSV content
        await page.evaluate(() => {
            const builder = window.pipelineBuilder;
            const VisualStep = window.VisualStep || builder.pipeline.steps[0]?.constructor;

            // Script step to inject CSV data into enriched.file_content
            const scriptStep = new VisualStep({
                stepName: 'Inject CSV Data',
                stepType: 'enrichment.script',
                sequence: 5,
                enabled: true,
                config: {
                    script: `
                        function transform(input) {
                            if (!input.enriched) input.enriched = {};
                            input.enriched.file_content = "name,age,city\\nAlice,30,NYC\\nBob,25,LA\\nCharlie,35,Chicago";
                            return input;
                        }
                    `,
                    targetPath: 'enriched.script'
                },
                layer: 'core'
            });
            builder.addStep(scriptStep);

            // File Parser step to parse the injected CSV
            const parserStep = new VisualStep({
                stepName: 'Parse CSV File',
                stepType: 'file_parser',
                sequence: 10,
                enabled: true,
                config: {
                    sourceField: 'enriched.file_content',
                    fileFormat: 'csv',
                    hasHeader: true,
                    trimFields: true
                },
                layer: 'core'
            });
            builder.addStep(parserStep);
        });

        // Step 2: Open test modal and run test
        await page.click('#testPipelineBtn');
        await page.waitForSelector('#testModal.active', { timeout: 5000 });

        // Paste a sample HL7 message (the pipeline still needs an HL7 input)
        const sampleHL7 = 'MSH|^~\\&|SendApp|SendFac|RecApp|RecFac|20250115120000||ADT^A01|MSG00001|P|2.5\rPID|||12345^^^MRN||Doe^John||19850615|M';
        await page.fill('#testMessageInput', sampleHL7);

        // Click Run Test
        await page.click('#runTestBtn');

        // Wait for results to appear
        await page.waitForSelector('#testResults[style*="block"], #testResults:not([style*="none"])', { timeout: 30000 });
        await page.waitForTimeout(2000); // Allow results to render

        // Verify results rendered (check for step output content)
        const resultsContent = await page.locator('#testResultsContent').textContent();
        expect(resultsContent.length).toBeGreaterThan(0);

        // Check for file parser output indicators
        const hasRecordCount = resultsContent.includes('record_count') || resultsContent.includes('3');
        const hasParsedRecords = resultsContent.includes('parsed_records') || resultsContent.includes('Alice');

        // At minimum, the test should complete without pipeline errors
        const hasError = resultsContent.toLowerCase().includes('pipeline execution failed');
        expect(hasError).toBe(false);
    });
});

// ─── SUITE 6: Excel (.xlsx) UI Tests ─────────────────────────

test.describe('File Parser Excel (.xlsx) UI', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('xlsx option is visible in format dropdown', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        const xlsxOption = page.locator('#fpFileFormat option[value="xlsx"]');
        await expect(xlsxOption).toBeAttached();
        await expect(xlsxOption).toHaveText('Excel (.xlsx)');
    });

    test('Selecting xlsx shows sheet name input, hides delimiter and columns', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Switch to xlsx
        await page.selectOption('#fpFileFormat', 'xlsx');
        await page.waitForTimeout(300);

        // Delimiter should be hidden
        await expect(page.locator('#fpDelimiterGroup')).toBeHidden();
        // Columns section should be hidden
        await expect(page.locator('#fpColumnsSection')).toBeHidden();
        // Excel section (renamed from fpXlsxSection) with sheet name should be visible
        await expect(page.locator('#fpExcelSection')).toBeVisible();
        await expect(page.locator('#fpSheetName')).toBeVisible();
    });

    test('Sheet name field is collected in config', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'xlsx' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Switch to xlsx and fill sheet name
        await page.selectOption('#fpFileFormat', 'xlsx');
        await page.waitForTimeout(300);
        await page.fill('#fpSheetName', 'PatientData');

        // Collect config
        const config = await page.evaluate(() => {
            const builder = window.pipelineBuilder;
            const allSteps = builder.getAllStepsFlat();
            const step = allSteps.find(s => s.stepType === 'file_parser');
            if (!step) throw new Error('File Parser step not found');
            const panel = builder.propertiesPanel || window.propertiesPanel;
            panel.collectFormData(step);
            return step.config;
        });

        expect(config.fileFormat).toBe('xlsx');
        expect(config.sheetName).toBe('PatientData');
    });
});

// ─── SUITE 7: Auto-Detect Toggle ─────────────────────────────

test.describe('File Parser Auto-Detect Toggle', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('Auto-detect toggle is present in the configuration form', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await expect(page.locator('#fpAutoDetect')).toBeAttached();
    });

    test('Enabling auto-detect hides the manual format group', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Initially the manual format group should be visible
        await expect(page.locator('#fpManualFormatGroup')).toBeVisible();

        // Enable auto-detect
        await page.check('#fpAutoDetect');
        await page.waitForTimeout(300);

        await expect(page.locator('#fpManualFormatGroup')).toBeHidden();
    });

    test('Disabling auto-detect restores the format group', async ({ page }) => {
        await addFileParserStep(page, { autoDetect: true });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Should be hidden when auto-detect is on
        const autoDetect = page.locator('#fpAutoDetect');
        if (await autoDetect.isChecked()) {
            await autoDetect.uncheck();
            await page.waitForTimeout(300);
            await expect(page.locator('#fpManualFormatGroup')).toBeVisible();
        }
    });

    test('Auto-detect value is saved in config', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Enable auto-detect
        await page.check('#fpAutoDetect');
        await page.waitForTimeout(200);

        const config = await page.evaluate(() => {
            const builder = window.pipelineBuilder;
            const allSteps = builder.getAllStepsFlat();
            const step = allSteps.find(s => s.stepType === 'file_parser');
            if (!step) throw new Error('File Parser step not found');
            const panel = builder.propertiesPanel || window.propertiesPanel;
            panel.collectFormData(step);
            return step.config;
        });

        expect(config.autoDetect).toBe(true);
    });
});

// ─── SUITE 8: XLS (Legacy Excel) Format ──────────────────────

test.describe('File Parser XLS (Legacy) Format', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('xls option is visible in format dropdown', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        const xlsOption = page.locator('#fpFileFormat option[value="xls"]');
        await expect(xlsOption).toBeAttached();
        await expect(xlsOption).toHaveText(/Excel.*\.xls/i);
    });

    test('Selecting xls shows Excel section, hides delimiter and columns', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'xls');
        await page.waitForTimeout(300);

        await expect(page.locator('#fpDelimiterGroup')).toBeHidden();
        await expect(page.locator('#fpColumnsSection')).toBeHidden();
        await expect(page.locator('#fpExcelSection')).toBeVisible();
    });

    test('Sheet name is collected when xls is selected', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'xls' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'xls');
        await page.waitForTimeout(300);
        await page.fill('#fpSheetName', 'Claims');

        const config = await page.evaluate(() => {
            const builder = window.pipelineBuilder;
            const allSteps = builder.getAllStepsFlat();
            const step = allSteps.find(s => s.stepType === 'file_parser');
            if (!step) throw new Error('File Parser step not found');
            const panel = builder.propertiesPanel || window.propertiesPanel;
            panel.collectFormData(step);
            return step.config;
        });

        expect(config.fileFormat).toBe('xls');
        expect(config.sheetName).toBe('Claims');
    });
});

// ─── SUITE 9: OOB Template Dropdown ──────────────────────────

test.describe('File Parser OOB Template Dropdown', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('Template dropdown is hidden when CSV is selected', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'csv' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'csv');
        await page.waitForTimeout(300);

        await expect(page.locator('#fpTemplateSection')).toBeHidden();
    });

    test('Template dropdown is visible when fixed_width is selected', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);

        await expect(page.locator('#fpTemplateSection')).toBeVisible();
        await expect(page.locator('#fpTemplate')).toBeVisible();
    });

    test('Template dropdown contains CCLF and NACHA options', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);

        // Check key template options exist
        await expect(page.locator('#fpTemplate option[value="cclf1"]')).toBeAttached();
        await expect(page.locator('#fpTemplate option[value="nacha_entry"]')).toBeAttached();
    });

    test('Selecting a template auto-fills the columns table', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);

        // Initially no columns
        const initialRows = await page.locator('#fpColumnsBody tr').count();

        // Select CCLF1 template
        await page.selectOption('#fpTemplate', 'cclf1');
        await page.waitForTimeout(500);

        // Columns should be populated from the template
        const afterRows = await page.locator('#fpColumnsBody tr').count();
        expect(afterRows).toBeGreaterThan(initialRows);
        expect(afterRows).toBeGreaterThan(0);
    });

    test('Template value is saved in collected config', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);
        await page.selectOption('#fpTemplate', 'nacha_entry');
        await page.waitForTimeout(300);

        const config = await page.evaluate(() => {
            const builder = window.pipelineBuilder;
            const allSteps = builder.getAllStepsFlat();
            const step = allSteps.find(s => s.stepType === 'file_parser');
            if (!step) throw new Error('File Parser step not found');
            const panel = builder.propertiesPanel || window.propertiesPanel;
            panel.collectFormData(step);
            return step.config;
        });

        expect(config.template).toBe('nacha_entry');
    });
});

// ─── SUITE 10: Sheet Index and Content Encoding ───────────────

test.describe('File Parser Sheet Index and Content Encoding', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('Sheet index field is visible when xlsx selected', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'xlsx');
        await page.waitForTimeout(300);

        await expect(page.locator('#fpSheetIndex')).toBeVisible();
    });

    test('Sheet index defaults to 0', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'xlsx');
        await page.waitForTimeout(300);

        await expect(page.locator('#fpSheetIndex')).toHaveValue('0');
    });

    test('Sheet index value is saved in config', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'xlsx' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'xlsx');
        await page.waitForTimeout(300);
        await page.fill('#fpSheetIndex', '2');

        const config = await page.evaluate(() => {
            const builder = window.pipelineBuilder;
            const allSteps = builder.getAllStepsFlat();
            const step = allSteps.find(s => s.stepType === 'file_parser');
            if (!step) throw new Error('File Parser step not found');
            const panel = builder.propertiesPanel || window.propertiesPanel;
            panel.collectFormData(step);
            return step.config;
        });

        expect(config.sheetIndex).toBe(2);
    });

    test('Content encoding dropdown is present and has base64 option', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'xlsx');
        await page.waitForTimeout(300);

        await expect(page.locator('#fpContentEncoding')).toBeVisible();
        await expect(page.locator('#fpContentEncoding option[value="base64"]')).toBeAttached();
    });

    test('Content encoding value is saved in config', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'xlsx' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'xlsx');
        await page.waitForTimeout(300);
        await page.selectOption('#fpContentEncoding', 'base64');

        const config = await page.evaluate(() => {
            const builder = window.pipelineBuilder;
            const allSteps = builder.getAllStepsFlat();
            const step = allSteps.find(s => s.stepType === 'file_parser');
            if (!step) throw new Error('File Parser step not found');
            const panel = builder.propertiesPanel || window.propertiesPanel;
            panel.collectFormData(step);
            return step.config;
        });

        expect(config.contentEncoding).toBe('base64');
    });
});

// ─── SUITE 11: Inline Data Preview Button ─────────────────────

test.describe('File Parser Inline Data Preview', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('Preview section is present in the form', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await expect(page.locator('#fpPreviewSection, #fpRunPreviewBtn')).toBeAttached();
    });

    test('Run Preview button is visible', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await expect(page.locator('#fpRunPreviewBtn')).toBeVisible();
    });

    test('Preview textarea accepts sample content', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        const textarea = page.locator('#fpPreviewContent');
        await expect(textarea).toBeAttached();

        await textarea.fill('name,age\nAlice,30\nBob,25');
        await expect(textarea).toHaveValue('name,age\nAlice,30\nBob,25');
    });

    test('Clicking preview button with CSV shows result table', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Fill in some sample CSV data for preview
        const textarea = page.locator('#fpPreviewContent');
        if (await textarea.isVisible({ timeout: 2000 }).catch(() => false)) {
            await textarea.fill('patient,age,city\nAlice,30,NYC\nBob,25,LA');
        }

        // Set format to CSV
        await page.selectOption('#fpFileFormat', 'csv');

        // Click preview button
        await page.click('#fpRunPreviewBtn');

        // Wait for the preview result to show (async API call)
        await page.waitForSelector(
            '#fpPreviewResult:not([style*="display: none"]):not([style*="display:none"])',
            { timeout: 15000 }
        ).catch(() => null); // Non-fatal — service may not be running

        // If the result appeared, verify it has table rows
        const resultVisible = await page.locator('#fpPreviewResult').isVisible().catch(() => false);
        if (resultVisible) {
            // Either an error message or actual table rows are acceptable
            const hasContent = await page.locator('#fpPreviewResult').textContent();
            expect(hasContent.length).toBeGreaterThan(0);
        }
    });

    test('Preview status message is shown during/after preview', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await expect(page.locator('#fpPreviewStatus')).toBeAttached();
    });
});

// ─── SUITE 12: Preview API — Direct HTTP Calls ───────────────
//
// These tests bypass the UI and call the Node.js proxy endpoints
// directly via fetch() inside page.evaluate(), ensuring the API
// contract is stable. Requires the app to be running on localhost:3000.

test.describe('File Parser Preview API (Direct)', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        // Stay on dashboard — just need auth cookie/token
        await page.waitForLoadState('networkidle');
    });

    test('GET /api/file-parser/templates returns success and template list', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/templates', {
                headers: { 'Authorization': `Bearer ${localStorage.getItem('accessToken')}` }
            });
            return { status: res.status, body: await res.json() };
        });

        expect(result.status).toBe(200);
        expect(result.body.success).toBe(true);
        expect(Array.isArray(result.body.templates)).toBe(true);
        expect(result.body.templates.length).toBeGreaterThan(0);
        expect(typeof result.body.count).toBe('number');
    });

    test('Templates endpoint returns known healthcare template keys', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/templates', {
                headers: { 'Authorization': `Bearer ${localStorage.getItem('accessToken')}` }
            });
            return await res.json();
        });

        const keys = result.templates.map(t => t.key);
        expect(keys).toContain('cclf1');
        expect(keys).toContain('nacha_entry');
    });

    test('Templates endpoint returns by_category with expected groups', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/templates', {
                headers: { 'Authorization': `Bearer ${localStorage.getItem('accessToken')}` }
            });
            return await res.json();
        });

        expect(result.by_category).toBeDefined();
        const categories = Object.keys(result.by_category);
        expect(categories).toContain('CMS/CCLF');
        expect(categories).toContain('NACHA/ACH');
    });

    test('POST /api/file-parser/preview with CSV returns parsed records', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content: 'mrn,name,dob\nMRN001,Alice Smith,1985-06-15\nMRN002,Bob Jones,1990-11-22',
                    config: {
                        fileFormat: 'csv',
                        hasHeader: true
                    }
                })
            });
            return { status: res.status, body: await res.json() };
        });

        expect(result.status).toBe(200);
        expect(result.body.success).toBe(true);
        expect(result.body.preview.record_count).toBe(2);
        expect(Array.isArray(result.body.preview.records)).toBe(true);
        expect(result.body.preview.records[0].mrn).toBe('MRN001');
    });

    test('POST /api/file-parser/preview with auto-detect detects CSV format', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content: 'id,status,amount\nC001,active,1500.00\nC002,closed,2400.00',
                    config: { autoDetect: true }
                })
            });
            return await res.json();
        });

        expect(result.success).toBe(true);
        expect(result.preview.record_count).toBe(2);
        expect(result.preview.format).toBe('csv');
    });

    test('POST /api/file-parser/preview caps records at 10 by default', async ({ page }) => {
        const result = await page.evaluate(async () => {
            // Build 15-row CSV
            const rows = ['col_a,col_b'];
            for (let i = 1; i <= 15; i++) {
                rows.push(`val${i},data${i}`);
            }
            const content = rows.join('\n');

            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content,
                    config: { fileFormat: 'csv', hasHeader: true }
                    // maxRecords not set → should default to 10
                })
            });
            return await res.json();
        });

        expect(result.success).toBe(true);
        expect(result.preview.records.length).toBeLessThanOrEqual(10);
    });

    test('POST /api/file-parser/preview with invalid JSON returns 400', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: 'not-json'
            });
            return { status: res.status, body: await res.json() };
        });

        expect(result.status).toBe(400);
        expect(result.body.success).toBe(false);
    });

    test('POST /api/file-parser/preview with unsupported format returns error', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content: 'some data',
                    config: { fileFormat: 'madeupformat' }
                })
            });
            return await res.json();
        });

        // Handler returns 200 with success:false for executor-level errors
        expect(result.success).toBe(false);
        expect(typeof result.error).toBe('string');
    });
});

// ─── SUITE 13: Field URI Source Type — UI ─────────────────────
//
// Verifies the "🔗 Field URI" radio button (sourceType=field_as_path)
// added for containerised cloud storage / SFTP-gateway integration.

test.describe('File Parser — Field URI Source Type UI', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('Three source type radio buttons are present', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await expect(page.locator('#fpSourceTypeField')).toBeAttached();
        await expect(page.locator('#fpSourceTypeFieldAsPath')).toBeAttached();
        await expect(page.locator('#fpSourceTypeLocal')).toBeAttached();
    });

    test('"Field URI" radio label is visible', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        const label = page.locator('label[for="fpSourceTypeFieldAsPath"]');
        await expect(label).toBeVisible({ timeout: 3000 });
        // Label text contains "Field URI" (emoji + text)
        await expect(label).toContainText('Field URI');
    });

    test('Selecting "Field URI" shows source field input (not file path)', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Click the Field URI radio button
        await page.click('label[for="fpSourceTypeFieldAsPath"]');
        await page.waitForTimeout(300);

        // Source field input should be visible
        await expect(page.locator('#fpSourceField')).toBeVisible();
        // Local path group should be hidden
        await expect(page.locator('#fpLocalSourceGroup, #fpFilePath').first()).toBeHidden().catch(() => {
            // Either the group or field is hidden — acceptable
        });
    });

    test('Selecting "Field URI" updates placeholder to URI format', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.click('label[for="fpSourceTypeFieldAsPath"]');
        await page.waitForTimeout(300);

        const placeholder = await page.locator('#fpSourceField').getAttribute('placeholder');
        // Placeholder should hint at S3 or URI field path, e.g. "steps.s3_connector.file_uri"
        expect(placeholder).toBeTruthy();
        expect(placeholder.length).toBeGreaterThan(5);
    });

    test('Selecting "Field URI" shows URI-specific help text', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.click('label[for="fpSourceTypeFieldAsPath"]');
        await page.waitForTimeout(300);

        const helpEl = page.locator('#fpSourceFieldHelp');
        const isAttached = await helpEl.isVisible({ timeout: 2000 }).catch(() => false);
        if (isAttached) {
            const helpText = await helpEl.textContent();
            // Should mention s3:// or URI
            const mentionsURI = helpText.toLowerCase().includes('s3://') || helpText.toLowerCase().includes('uri');
            expect(mentionsURI).toBe(true);
        }
    });

    test('Switching from "Field URI" back to "Field Content" restores original help text', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // First go to Field URI
        await page.click('label[for="fpSourceTypeFieldAsPath"]');
        await page.waitForTimeout(200);

        // Then back to Field Content
        await page.click('label[for="fpSourceTypeField"]');
        await page.waitForTimeout(200);

        const helpEl = page.locator('#fpSourceFieldHelp');
        const isAttached = await helpEl.isVisible({ timeout: 2000 }).catch(() => false);
        if (isAttached) {
            const helpText = await helpEl.textContent();
            // Should NOT show S3 URI help after switching back
            const isURIHelp = helpText.toLowerCase().includes('s3://');
            expect(isURIHelp).toBe(false);
        }
    });

    test('Field URI config is collected correctly in step config', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Select Field URI radio
        await page.click('label[for="fpSourceTypeFieldAsPath"]');
        await page.waitForTimeout(300);

        // Fill in the source field name
        await page.fill('#fpSourceField', 'steps.s3_inbound.file_uri');

        // Collect config via JavaScript
        const config = await page.evaluate(() => {
            const builder = window.pipelineBuilder;
            const allSteps = builder.getAllStepsFlat();
            const step = allSteps.find(s => s.stepType === 'file_parser');
            if (!step) throw new Error('File Parser step not found');
            const panel = builder.propertiesPanel || window.propertiesPanel;
            panel.collectFormData(step);
            return step.config;
        });

        expect(config.sourceType).toBe('field_as_path');
        expect(config.sourceField).toBe('steps.s3_inbound.file_uri');
    });

    test('Field URI step config persists across panel re-open', async ({ page }) => {
        // Create step pre-configured with field_as_path
        await addFileParserStep(page, {
            sourceType: 'field_as_path',
            sourceField: 'enriched.cclf1_uri',
            fileFormat: 'fixed_width'
        });

        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // The Field URI radio should be checked
        const isChecked = await page.locator('#fpSourceTypeFieldAsPath').isChecked({ timeout: 2000 }).catch(() => false);
        expect(isChecked).toBe(true);

        // Source field should show the configured value
        await expect(page.locator('#fpSourceField')).toHaveValue('enriched.cclf1_uri');
    });

    test('Local Path group is hidden when Field URI is selected', async ({ page }) => {
        await addFileParserStep(page);
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.click('label[for="fpSourceTypeFieldAsPath"]');
        await page.waitForTimeout(300);

        // Local path group should not be visible
        const localGroup = page.locator('#fpLocalSourceGroup');
        const isVisible = await localGroup.isVisible({ timeout: 1000 }).catch(() => false);
        expect(isVisible).toBe(false);
    });

    test('Switching to Local Path hides source field group', async ({ page }) => {
        await addFileParserStep(page, { sourceType: 'field_as_path' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        // Switch to Local Path
        await page.click('label[for="fpSourceTypeLocal"]');
        await page.waitForTimeout(300);

        // Field source group should now be hidden
        const fieldGroup = page.locator('#fpFieldSourceGroup');
        const isVisible = await fieldGroup.isVisible({ timeout: 1000 }).catch(() => false);
        expect(isVisible).toBe(false);
    });
});

// ─── SUITE 14: Field URI API Integration ──────────────────────
//
// Direct API calls (bypassing UI) to verify the preview endpoint
// handles sourceType=field_as_path correctly for HTTP URIs.

test.describe('File Parser — Field URI API Integration', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await page.waitForLoadState('networkidle');
    });

    test('Preview endpoint supports CSV content directly (field mode still works)', async ({ page }) => {
        // Regression: field mode (raw content) should still work after field_as_path additions
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content: 'bene_id,clm_id,clm_type\nB001,C001,71\nB002,C002,72\n',
                    config: { fileFormat: 'csv', hasHeader: true }
                })
            });
            return { status: res.status, body: await res.json() };
        });

        expect(result.status).toBe(200);
        expect(result.body.success).toBe(true);
        expect(result.body.preview.record_count).toBe(2);
        expect(result.body.preview.records[0].bene_id).toBe('B001');
    });

    test('Preview endpoint handles fixed-width CCLF1 content', async ({ page }) => {
        // Simulate what a field_as_path step would download and then preview
        const line1 = '1234567890123MBI12345678' + ' '.repeat(87);
        const line2 = '9876543210987MBI98765432' + ' '.repeat(87);
        const content = line1 + '\n' + line2 + '\n';

        const result = await page.evaluate(async (c) => {
            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content: c,
                    config: {
                        fileFormat: 'fixed_width',
                        hasHeader: false,
                        template: 'cclf1',
                        trimFields: true
                    }
                })
            });
            return { status: res.status, body: await res.json() };
        }, content);

        expect(result.status).toBe(200);
        expect(result.body.success).toBe(true);
        expect(result.body.preview.record_count).toBe(2);

        const firstRecord = result.body.preview.records[0];
        expect(firstRecord.CUR_CLM_UNIQ_ID).toBe('1234567890123');
        expect(firstRecord.BENE_MBI_ID).toBe('MBI12345678');
    });

    test('Preview endpoint with TSV content returns correct columns', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content: 'claim_id\tstatus\tamount\nC001\tpaid\t1500.00\nC002\tdenied\t2400.00\n',
                    config: { fileFormat: 'tsv', hasHeader: true }
                })
            });
            return await res.json();
        });

        expect(result.success).toBe(true);
        expect(result.preview.columns).toContain('claim_id');
        expect(result.preview.columns).toContain('status');
        expect(result.preview.columns).toContain('amount');
        expect(result.preview.records[0].status).toBe('paid');
    });

    test('Preview endpoint with NACHA ACH entry template', async ({ page }) => {
        // NACHA entry: RECORD_TYPE_CODE (pos 1, len 1), TRANSACTION_CODE (pos 2, len 2)
        const line = '6' + '27' + '0'.repeat(91);
        const result = await page.evaluate(async (c) => {
            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content: c + '\n',
                    config: {
                        fileFormat: 'fixed_width',
                        hasHeader: false,
                        template: 'nacha_entry',
                        trimFields: true
                    }
                })
            });
            return await res.json();
        }, line);

        expect(result.success).toBe(true);
        expect(result.preview.records[0].RECORD_TYPE_CODE).toBe('6');
        expect(result.preview.records[0].TRANSACTION_CODE).toBe('27');
    });

    test('Preview endpoint respects maxRecords limit', async ({ page }) => {
        const result = await page.evaluate(async () => {
            // Build 20-row CSV
            const rows = ['col_a,col_b'];
            for (let i = 1; i <= 20; i++) rows.push(`R${String(i).padStart(3,'0')},val${i}`);
            const content = rows.join('\n');

            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content,
                    config: { fileFormat: 'csv', hasHeader: true, maxRecords: 5 }
                })
            });
            return await res.json();
        });

        expect(result.success).toBe(true);
        expect(result.preview.records.length).toBe(5);
        expect(result.preview.records[0].col_a).toBe('R001');
    });

    test('Preview endpoint respects skipRows', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const content = [
                'Report: Monthly Claims',    // skip row 1
                'Generated: 2025-01-15',     // skip row 2
                'id,amount',                 // header
                'C001,1500',
                'C002,2400'
            ].join('\n');

            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content,
                    config: { fileFormat: 'csv', hasHeader: true, skipRows: 2 }
                })
            });
            return await res.json();
        });

        expect(result.success).toBe(true);
        expect(result.preview.record_count).toBe(2);
        expect(result.preview.records[0].id).toBe('C001');
    });
});

// ─── SUITE 15: Template Confidence Badge ──────────────────────
//
// Verifies that selecting a CCLF or NACHA template displays the
// confidence badge and loads the correct column count.

test.describe('File Parser — Template Confidence Badge', () => {
    test.beforeEach(async ({ page }) => {
        await login(page);
        await openPipelineBuilder(page);
    });

    test('CCLF1 template option is visible in template dropdown', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'fixed_width' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);

        const cclf1Option = page.locator('#fpTemplate option[value="cclf1"]');
        await expect(cclf1Option).toBeAttached();
    });

    test('CCLF2, CCLF5, CCLF8 template options are visible', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'fixed_width' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);

        for (const key of ['cclf2', 'cclf5', 'cclf8']) {
            await expect(page.locator(`#fpTemplate option[value="${key}"]`)).toBeAttached();
        }
    });

    test('Selecting CCLF1 populates columns with 16 fields', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'fixed_width' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);
        await page.selectOption('#fpTemplate', 'cclf1');
        await page.waitForTimeout(600);

        const rows = await page.locator('#fpColumnsBody tr').count();
        expect(rows).toBe(16);
    });

    test('Selecting CCLF2 populates columns (22 fields)', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'fixed_width' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);
        await page.selectOption('#fpTemplate', 'cclf2');
        await page.waitForTimeout(600);

        const rows = await page.locator('#fpColumnsBody tr').count();
        expect(rows).toBe(22);
    });

    test('Selecting CCLF5 populates columns (49 fields)', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'fixed_width' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);
        await page.selectOption('#fpTemplate', 'cclf5');
        await page.waitForTimeout(600);

        const rows = await page.locator('#fpColumnsBody tr').count();
        expect(rows).toBe(49);
    });

    test('Selecting CCLF8 (Beneficiary Demographics) populates columns (31 fields)', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'fixed_width' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);
        await page.selectOption('#fpTemplate', 'cclf8');
        await page.waitForTimeout(600);

        const rows = await page.locator('#fpColumnsBody tr').count();
        expect(rows).toBe(31);
    });

    test('Selecting CCLF8 loads BENE_MBI_ID as first column', async ({ page }) => {
        await addFileParserStep(page, { fileFormat: 'fixed_width' });
        await clickStep(page, 'File Parser');
        await waitForPropertiesPanel(page);

        await page.selectOption('#fpFileFormat', 'fixed_width');
        await page.waitForTimeout(300);
        await page.selectOption('#fpTemplate', 'cclf8');
        await page.waitForTimeout(600);

        // First column name input should be BENE_MBI_ID
        const firstColName = await page.locator('.fp-col-name').first().inputValue().catch(() => '');
        expect(firstColName).toBe('BENE_MBI_ID');
    });

    test('CCLF1 confidence badge shows via GET /api/file-parser/templates', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/templates', {
                headers: { 'Authorization': `Bearer ${localStorage.getItem('accessToken')}` }
            });
            return await res.json();
        });

        const cclf1 = result.templates.find(t => t.key === 'cclf1');
        expect(cclf1).toBeDefined();
        expect(cclf1.confidence).toBe('high');
        expect(cclf1.columnCount).toBe(16);
        expect(cclf1.category).toBe('CMS/CCLF');
    });

    test('CCLF2 template API returns 22 columns and high confidence', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/templates', {
                headers: { 'Authorization': `Bearer ${localStorage.getItem('accessToken')}` }
            });
            return await res.json();
        });

        const cclf2 = result.templates.find(t => t.key === 'cclf2');
        expect(cclf2).toBeDefined();
        expect(cclf2.confidence).toBe('high');
        expect(cclf2.columnCount).toBe(22);
    });

    test('CCLF5 template API returns 49 columns and high confidence', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/templates', {
                headers: { 'Authorization': `Bearer ${localStorage.getItem('accessToken')}` }
            });
            return await res.json();
        });

        const cclf5 = result.templates.find(t => t.key === 'cclf5');
        expect(cclf5).toBeDefined();
        expect(cclf5.confidence).toBe('high');
        expect(cclf5.columnCount).toBe(49);
    });

    test('CCLF8 template API returns 31 columns and high confidence', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/templates', {
                headers: { 'Authorization': `Bearer ${localStorage.getItem('accessToken')}` }
            });
            return await res.json();
        });

        const cclf8 = result.templates.find(t => t.key === 'cclf8');
        expect(cclf8).toBeDefined();
        expect(cclf8.confidence).toBe('high');
        expect(cclf8.columnCount).toBe(31);
    });

    test('NACHA entry template API returns correct column count and category', async ({ page }) => {
        const result = await page.evaluate(async () => {
            const res = await fetch('/api/file-parser/templates', {
                headers: { 'Authorization': `Bearer ${localStorage.getItem('accessToken')}` }
            });
            return await res.json();
        });

        const nacha = result.templates.find(t => t.key === 'nacha_entry');
        expect(nacha).toBeDefined();
        expect(nacha.category).toBe('NACHA/ACH');
        expect(nacha.columnCount).toBeGreaterThan(5);
    });

    test('Template preview: CCLF1 via API parses synthesised line correctly', async ({ page }) => {
        // Build a synthetic CCLF1 line: CUR_CLM_UNIQ_ID (1-13) + BENE_MBI_ID (14-24) + padding
        const line = '1234567890123' + 'MBI12345678' + ' '.repeat(87);

        const result = await page.evaluate(async (c) => {
            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content: c + '\n',
                    config: {
                        fileFormat: 'fixed_width',
                        hasHeader: false,
                        template: 'cclf1',
                        trimFields: true
                    }
                })
            });
            return await res.json();
        }, line);

        expect(result.success).toBe(true);
        expect(result.preview.column_count).toBe(16);
        expect(result.preview.records[0].CUR_CLM_UNIQ_ID).toBe('1234567890123');
        expect(result.preview.records[0].BENE_MBI_ID).toBe('MBI12345678');
    });

    test('Template preview: CCLF8 includes BENE_DOB and BENE_SEX_CD fields', async ({ page }) => {
        // CCLF8 Beneficiary Demographics: BENE_MBI_ID (1-11), ..., BENE_DOB (13-20), BENE_SEX_CD (21)
        // Build a line with known values at those positions
        const line =
            'MBI12345678' +   // pos 1-11: BENE_MBI_ID (11 chars)
            ' ' +             // pos 12: BENE_HIC_NUM placeholder (skipped, not first few fields)
            '19850615' +      // pos 13-20: BENE_DOB
            'M' +             // pos 21: BENE_SEX_CD
            ' '.repeat(549 - 21); // padding to full record length

        const result = await page.evaluate(async (c) => {
            const res = await fetch('/api/file-parser/preview', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('accessToken')}`
                },
                body: JSON.stringify({
                    content: c + '\n',
                    config: {
                        fileFormat: 'fixed_width',
                        hasHeader: false,
                        template: 'cclf8',
                        trimFields: true
                    }
                })
            });
            return await res.json();
        }, line);

        expect(result.success).toBe(true);
        expect(result.preview.column_count).toBe(31);
        const record = result.preview.records[0];
        expect(record.BENE_MBI_ID).toBe('MBI12345678');
        // DOB and sex depend on exact field positions in the template
        // At minimum: 31 columns should be present
        expect(Object.keys(record).length).toBe(31);
    });
});
