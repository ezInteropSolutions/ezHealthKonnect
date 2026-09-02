// tests/playwright/toolbox-step-coverage.spec.js
// Regression guard for a real gap found and fixed in September 2026: 13 step
// types (the 4 EDI steps, plus cda.parse/cda.normalize/cda.to_fhir/cda.build/
// fhir.to_cda/cda.dedupe/cda.map_to_canonical/cda.section_to_csv/fhir.build/
// hl7.build/payload.builder/deidentify) had real, working backend executors
// and StepBuilderRegistry-registered config-panel UIs, but were completely
// absent from the toolbox's "All Steps" palette (ToolboxManager.js's
// getBuiltInTemplates()) -- so nobody could ever drag one onto a blank
// canvas. This test doesn't just check those 13 by name; it diffs
// StepBuilderRegistry's own registered keys against the toolbox's own
// template types, so the SAME class of gap fails loudly for any step type
// built after this file was written, not just the ones fixed then.
const { test, expect } = require('@playwright/test');

// Legacy/backward-compat type names that deliberately map to the SAME
// builder as an already-toolboxed canonical name (confirmed individually,
// not guessed) -- these must NOT get their own second toolbox card, since
// that would just duplicate one real step under two visible names. A
// pipeline saved under the legacy name still resolves correctly via
// PropertiesPanel's own alias-detection (VisualStep.is*() helpers); only
// the toolbox's own "which name do we offer to drag in" choice is scoped
// here.
const KNOWN_ALIASES = new Set([
    'pre.enrichment.api',      // -> enrichment.api
    'database_enrichment',     // -> enrichment.database (PropertiesPanel's isDatabaseEnrichment alias)
    'pre.enrichment.database', // -> enrichment.database
    'pre.deidentify',          // -> deidentify
    'post.deidentify',         // -> deidentify
    'payload_builder',         // -> payload.builder
]);

test('every registered step builder has a toolbox entry', async ({ page }) => {
    await page.goto('/pipeline-builder.html');
    await page.waitForSelector('#all-steps-list .step-card', { timeout: 15000 });

    const gap = await page.evaluate((aliases) => {
        const registered = Object.keys(StepBuilderRegistry._registry);
        const toolboxTypes = new Set(
            (window.pipelineBuilder.toolboxManager.templates || []).map(t => t.type)
        );
        return registered.filter(t => !toolboxTypes.has(t) && !aliases.includes(t));
    }, [...KNOWN_ALIASES]);

    expect(gap, `these StepBuilderRegistry-registered step types have no toolbox entry and aren't a known alias: ${gap.join(', ')}`).toEqual([]);
});

// The 13 step types newly added to the toolbox this round (12 CDA/FHIR/HL7/
// general-purpose steps + pas_envelope_mapping) all reuse pre-existing,
// already-proven StepBuilderRegistry builders (CDAStepBuilder.js,
// FHIRBuildBuilder.js, HL7BuildBuilder.js, DeidentifyBuilder.js,
// PayloadBuilderBuilder.js, PASEnvelopeBuilder.js) -- this test isn't
// re-verifying those builders' own correctness (each has its own coverage
// elsewhere), only that each one's real DEFAULT CONFIG this round supplied
// actually renders without throwing, since a wrong default shape would
// throw on first open, before a user ever gets to touch it.
const NEWLY_TOOLBOXED_TYPES = [
    'cda.parse', 'cda.normalize', 'cda.to_fhir', 'cda.build', 'fhir.to_cda',
    'cda.dedupe', 'cda.map_to_canonical', 'cda.section_to_csv',
    'fhir.build', 'hl7.build', 'payload.builder', 'deidentify', 'pas_envelope_mapping',
];

test('newly-toolboxed step types open their config panel without error', async ({ page }) => {
    const consoleErrors = [];
    page.on('console', msg => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('pageerror', err => consoleErrors.push('pageerror: ' + err.message));

    await page.goto('/pipeline-builder.html');
    await page.waitForSelector('#all-steps-list .step-card', { timeout: 15000 });

    for (const stepType of NEWLY_TOOLBOXED_TYPES) {
        const defaultConfig = await page.evaluate((st) => {
            const tpl = window.pipelineBuilder.toolboxManager.templates.find(t => t.type === st);
            return tpl ? tpl.defaultConfig : null;
        }, stepType);
        expect(defaultConfig, `${stepType} should have a toolbox template`).not.toBeNull();

        await page.evaluate(({ st, cfg }) => {
            const step = new VisualStep({ stepName: 'Test ' + st, stepType: st, config: cfg });
            window.pipelineBuilder.addStep(step);
            window.pipelineBuilder.propertiesPanel.showStepProperties(step, false);
        }, { st: stepType, cfg: defaultConfig });

        await page.waitForSelector('#stepPropertiesModal', { state: 'visible', timeout: 5000 });
        const panelText = await page.locator('#formTabContent').innerText();
        expect(panelText.length, `${stepType} config panel should render non-empty content`).toBeGreaterThan(20);
    }

    expect(consoleErrors, `expected no console errors across all newly-toolboxed types, got: ${consoleErrors.join('; ')}`).toEqual([]);
});
