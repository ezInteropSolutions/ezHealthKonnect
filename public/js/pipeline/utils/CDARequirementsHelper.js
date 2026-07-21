/**
 * CDARequirementsHelper — shared completeness logic for the CDA guided-
 * configuration UIs (cda.map_to_canonical's MapToCanonicalBuilder and
 * cda.build's CDABuildStepBuilder Requirements tab).
 *
 * Both builders need to answer the exact same question — "given this
 * document type's requirements catalog (GET /api/cda/document-types/:type/
 * requirements) and a cda.map_to_canonical step's saved header/sections
 * config, which SHALL items are still unmapped?" — so the logic lives here
 * once rather than drifting between two copies. "Satisfied" means the row
 * exists AND has a mapped value, matching CDADedupeStepBuilder's own
 * "0 checked fields = skipped" convention (CDAStepBuilder.js) — an empty
 * section card or a header row with no source path doesn't count.
 *
 * Plain functions, not a class — this is pure data transformation with no
 * per-instance state, matching StepVariablesProvider.js's own module shape.
 */

const CDARequirementsHelper = {
    /**
     * Diffs a cda.map_to_canonical step's config against a document type's
     * requirements catalog.
     *
     * @param {object|null} requirements - GET /document-types/:type/requirements response's "requirements" object, or null if not yet loaded.
     * @param {Array} header - cfg.header from cda.map_to_canonical (may be undefined).
     * @param {Array} sections - cfg.sections from cda.map_to_canonical (may be undefined).
     * @returns {{shallTotal:number, shallSatisfied:number, missingSections:Array, missingHeaderFields:Array}}
     */
    computeMissingShall(requirements, header, sections) {
        const result = { shallTotal: 0, shallSatisfied: 0, missingSections: [], missingHeaderFields: [] };
        if (!requirements) return result;

        const headerRows = Array.isArray(header) ? header : [];
        const sectionRows = Array.isArray(sections) ? sections : [];

        (requirements.sections || []).forEach(sec => {
            if (sec.conformance !== 'SHALL') return;
            result.shallTotal++;
            const row = sectionRows.find(s => s.sectionKey === sec.key);
            const satisfied = !!row && Array.isArray(row.fields) && row.fields.some(f => f && f.canonicalField);
            if (satisfied) {
                result.shallSatisfied++;
            } else {
                result.missingSections.push(sec);
            }
        });

        // Only "patient"/"author" are ever mappable in cda.map_to_canonical's
        // header config — "custodian" (also present in requirements.headerGroups)
        // is deliberately excluded here: per decision #2, custodian lives as
        // structured config directly on the cda.build step's own Custodian tab,
        // never in the canonical-JSON header this function is scoring. Counting
        // it here would flag it "missing" unconditionally, in a UI with no way
        // to satisfy it — see CDABuildStepBuilder's Custodian tab for where
        // custodian's own required badges actually live instead.
        const headerGroups = requirements.headerGroups || {};
        ['patient', 'author'].forEach(group => {
            (headerGroups[group] || []).forEach(field => {
                if (field.conformance !== 'SHALL') return;
                result.shallTotal++;
                const row = headerRows.find(h => h.group === group && h.target === field.key);
                const satisfied = !!row && !!(row.sourcePath && row.sourcePath.trim());
                if (satisfied) {
                    result.shallSatisfied++;
                } else {
                    result.missingHeaderFields.push({ group, ...field });
                }
            });
        });

        return result;
    },

    /**
     * Finds the first cda.map_to_canonical step in the live in-memory
     * pipeline, or null.
     *
     * Pipeline shape is NOT stable: a pipeline loaded from the backend with
     * saved connections is DAG/execution-group based (see architecture/
     * DAG_PARALLEL_EXECUTION_DESIGN.md) — step objects live at
     * pipeline.executionGroups[i].steps[j] — but a freshly-built pipeline
     * (e.g. window.pipelineBuilder.addStep() before any connections exist)
     * may only have a flat pipeline.steps array. Checks both, flattening
     * across every execution group since a DAG can, in principle, split
     * cda.map_to_canonical and cda.build across groups — Phase 1 scope; see
     * requirements_catalog plan's note on deferring true DAG-aware
     * resolution.
     *
     * Step object key casing is ALSO not stable: steps arrive from the API
     * in raw snake_case (step_type, matching the transformation_steps
     * column), but opening any step's properties panel normalizes every
     * step in the pipeline to camelCase (stepType) in place — so depending
     * on whether a panel has been opened yet, either key may be live. Reads
     * both defensively rather than assuming one shape, matching this
     * codebase's own established test convention (see
     * tests/playwright/cda-fhir-pipeline.spec.js's `s.stepType || s.step_type`).
     */
    findMapToCanonicalStep(panel) {
        const pipeline = panel && panel.builder && panel.builder.pipeline;
        if (!pipeline) return null;
        const isMapToCanonical = s => (s.stepType || s.step_type) === 'cda.map_to_canonical';

        if (Array.isArray(pipeline.steps)) {
            const found = pipeline.steps.find(isMapToCanonical);
            if (found) return found;
        }
        if (Array.isArray(pipeline.executionGroups)) {
            for (const group of pipeline.executionGroups) {
                const found = (group.steps || []).find(isMapToCanonical);
                if (found) return found;
            }
        }
        return null;
    },

    /** Reads a step's display name regardless of snake_case/camelCase normalization state — see findMapToCanonicalStep's doc comment. */
    stepDisplayName(step) {
        return (step && (step.stepName || step.step_name)) || '';
    },

    /** Small conformance badge — SHALL=red/required, SHOULD=amber/recommended, MAY=gray/optional. */
    renderConformanceBadge(conformance) {
        const styles = {
            SHALL: 'background:#fee2e2;color:#991b1b;',
            SHOULD: 'background:#fef3c7;color:#92400e;',
            MAY: 'background:#e2e8f0;color:#475569;',
        };
        const labels = { SHALL: 'Required', SHOULD: 'Recommended', MAY: 'Optional' };
        const style = styles[conformance] || styles.MAY;
        const label = labels[conformance] || conformance || '';
        return `<span style="${style}font-size:0.65rem;font-weight:600;padding:0.1rem 0.4rem;border-radius:10px;white-space:nowrap;">${label}</span>`;
    },

    /**
     * Renders the red/amber/green completeness banner shown above both
     * guided UIs. `missing` is computeMissingShall's return value.
     */
    renderCompletenessBanner(missing) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        if (missing.shallTotal === 0) return '';

        const complete = missing.shallSatisfied === missing.shallTotal;
        const bg = complete ? '#f0fdf4' : (missing.shallSatisfied > 0 ? '#fffbeb' : '#fef2f2');
        const border = complete ? '#bbf7d0' : (missing.shallSatisfied > 0 ? '#fde68a' : '#fecaca');
        const color = complete ? '#166534' : (missing.shallSatisfied > 0 ? '#92400e' : '#991b1b');
        const icon = complete ? 'fa-circle-check' : 'fa-triangle-exclamation';

        const missingList = [
            ...missing.missingSections.map(s => `Section: ${esc(s.displayName || s.key)}`),
            ...missing.missingHeaderFields.map(f => `${esc(f.group)}.${esc(f.key)} (${esc(f.label || f.key)})`),
        ];

        return `
        <div style="background:${bg};border:1px solid ${border};border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:${color};">
            <div style="display:flex;align-items:center;gap:0.4rem;font-weight:600;">
                <i class="fas ${icon}"></i>
                ${missing.shallSatisfied} of ${missing.shallTotal} required (SHALL) items configured
            </div>
            ${missingList.length > 0 ? `<div style="margin-top:0.4rem;font-weight:400;">Still missing: ${missingList.join(', ')}</div>` : ''}
        </div>`;
    },
};

// Export for use in other modules (Node-style require, unused in-browser — matches this codebase's existing module.exports convention).
if (typeof module !== 'undefined' && module.exports) {
    module.exports = CDARequirementsHelper;
}
