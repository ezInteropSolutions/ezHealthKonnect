/**
 * FHIRRequirementsHelper — completeness logic for FHIRBuildBuilder (the
 * fhir.build step's config UI). The FHIR-side sibling of
 * CDARequirementsHelper.js, following the same shape (plain functions, no
 * per-instance state) but NOT sharing its computeMissingShall/
 * renderCompletenessBanner directly — CDA's "missing" shape is built around
 * conformance levels (SHALL/SHOULD) across two different config surfaces
 * (sections + header groups); FHIR's compiled schema only carries a flat
 * required:true/false per element, so forcing it through CDA's shape would
 * be a leakier abstraction than a small, clearly-FHIR-shaped twin.
 *
 * The data source is already loaded, not a new endpoint: FHIRBuildBuilder's
 * own _fieldCatalog (GET /api/fhir/canonical-fields/:resourceType) already
 * carries a "required" flag straight off fhir/r4's compiled
 * CompiledProfile.Required — the SAME data fhir_validation checks post-hoc.
 * This surfaces it at config time instead of only after a Test Pipeline run
 * or an external validator round (see project memory: the Observation
 * vital-signs profile gap that took two external-validator rounds to
 * surface — this is the config-time nudge toward catching that class of
 * gap earlier).
 *
 * Cardinality-aware, not flat existence: a nested required field (e.g.
 * Patient.communication.language, required WITHIN a communication entry) is
 * only counted as "missing" if it is actually reachable — either every
 * ancestor on its path is itself required (so the ancestor is guaranteed to
 * exist, e.g. us-core's mandatory Patient.identifier/.name), or an optional
 * ancestor (Patient.communication/.link/.telecom are all 0..*) has actually
 * been configured (a repeating group targeting it, or any field mapped
 * underneath it). Omitting an unconfigured OPTIONAL group entirely is valid
 * FHIR — you simply can't have a communication entry that omits language —
 * so flagging communication.language/link.other/link.type/telecom.system as
 * "still missing" when the user hasn't touched communication/link/telecom at
 * all is a false positive, not a real gap. This mirrors fhir_validation's own
 * structural check, which likewise never faults an absent optional element.
 */

const FHIRRequirementsHelper = {
    /**
     * Diffs a fhir.build step's configured fields + repeating groups against
     * its own resource's required-field list.
     *
     * @param {Array} fieldCatalog - FHIRBuildBuilder._fieldCatalog: [{key, label, dataType, required}]
     * @param {Array} fields - cfg.fields
     * @param {Array} repeatingGroups - cfg.repeatingGroups
     * @returns {{total:number, satisfied:number, missing:Array<{key,label}>}}
     */
    computeMissingRequired(fieldCatalog, fields, repeatingGroups) {
        const catalog = fieldCatalog || [];
        const required = catalog.filter(f => f.required);
        if (required.length === 0) return { total: 0, satisfied: 0, missing: [] };

        const catalogByKey = new Map(catalog.map(f => [f.key, f]));

        const mappedPaths = [
            ...(fields || []).map(f => f.targetPath).filter(Boolean),
            ...(repeatingGroups || []).flatMap(rg =>
                !rg.targetPath ? [] : (rg.fields || [])
                    .filter(f => f.targetPath)
                    .map(f => `${rg.targetPath}.${f.targetPath}`)
            ),
        ];

        // A group is "in use" if the user has targeted it directly (a
        // repeating group card for it, even before any of its own fields are
        // filled in) or mapped anything underneath it (flat field or
        // repeating-group field whose path starts with it).
        const groupTargetPaths = (repeatingGroups || []).map(rg => rg.targetPath).filter(Boolean);
        const isGroupInUse = (ancestorPath) =>
            groupTargetPaths.some(p => p === ancestorPath || p.startsWith(ancestorPath + '.')) ||
            mappedPaths.some(p => p === ancestorPath || p.startsWith(ancestorPath + '.') || p.startsWith(ancestorPath + '['));

        // Walks every ancestor segment of a dotted path (root-most first). A
        // required leaf is only reachable if every OPTIONAL ancestor along
        // the way has been configured — a REQUIRED ancestor is guaranteed to
        // exist so its descendants are unconditional; an ancestor missing
        // from the catalog entirely is unknown, so fail safe and treat it as
        // reachable rather than silently hiding a real gap.
        const isReachable = (key) => {
            const segments = key.split('.');
            let prefix = '';
            for (let i = 0; i < segments.length - 1; i++) {
                prefix = prefix ? `${prefix}.${segments[i]}` : segments[i];
                const ancestorInfo = catalogByKey.get(prefix);
                if (ancestorInfo && !ancestorInfo.required && !isGroupInUse(prefix)) {
                    return false;
                }
            }
            return true;
        };

        const isSatisfied = (key) => {
            // Choice-type elements ("medication[x]") are satisfied by ANY
            // mapped path using FHIR's own type-suffix naming convention for
            // the chosen concrete type (e.g. "medicationCodeableConcept").
            if (key.endsWith('[x]')) {
                const base = key.slice(0, -3);
                return mappedPaths.some(p => p === base || (p.startsWith(base) && /^[A-Z]/.test(p.slice(base.length))));
            }
            // Exact match, or a nested/indexed continuation of this path —
            // NOT a bare prefix match, so "status" doesn't false-positive
            // against a mapped "statusReason".
            return mappedPaths.some(p => p === key || p.startsWith(key + '.') || p.startsWith(key + '['));
        };

        const applicable = required.filter(f => isReachable(f.key));
        const missing = applicable.filter(f => !isSatisfied(f.key));
        return { total: applicable.length, satisfied: applicable.length - missing.length, missing };
    },

    /**
     * Renders the red/amber/green completeness banner — visually mirrors
     * CDARequirementsHelper.renderCompletenessBanner for UI consistency
     * across the two guided builders, kept as FHIR's own small copy since
     * the underlying "missing" shape genuinely differs (see file doc comment).
     */
    renderCompletenessBanner(missing) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        if (missing.total === 0) return '';

        const complete = missing.satisfied === missing.total;
        const bg = complete ? '#f0fdf4' : (missing.satisfied > 0 ? '#fffbeb' : '#fef2f2');
        const border = complete ? '#bbf7d0' : (missing.satisfied > 0 ? '#fde68a' : '#fecaca');
        const color = complete ? '#166534' : (missing.satisfied > 0 ? '#92400e' : '#991b1b');
        const icon = complete ? 'fa-circle-check' : 'fa-triangle-exclamation';

        const missingList = missing.missing.map(f =>
            (f.label && f.label !== f.key) ? `${esc(f.label)} (${esc(f.key)})` : esc(f.key)
        );

        return `
        <div style="background:${bg};border:1px solid ${border};border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:${color};">
            <div style="display:flex;align-items:center;gap:0.4rem;font-weight:600;">
                <i class="fas ${icon}"></i>
                ${missing.satisfied} of ${missing.total} required elements mapped
            </div>
            ${missingList.length > 0 ? `<div style="margin-top:0.4rem;font-weight:400;">Still missing: ${missingList.join(', ')}</div>` : ''}
        </div>`;
    },
};

// Export for use in other modules (Node-style require, unused in-browser — matches CDARequirementsHelper.js's own convention).
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FHIRRequirementsHelper;
}
