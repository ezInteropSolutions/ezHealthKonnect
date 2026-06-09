/**
 * CDAStepBuilder — step config builders for CDA/CCD pipeline steps.
 *
 * Registers three step types with StepBuilderRegistry:
 *   - cda.to_fhir     Convert CDA/CCD XML to FHIR R4 Bundle
 *   - fhir.to_cda     Serialize FHIR R4 Bundle to C-CDA 2.1 XML
 *   - cda.normalize   Normalize C32/HITSP template OIDs to C-CDA 2.1 before parsing
 *
 * Builder contract (StepBuilderRegistry):
 *   render(step)         → string  HTML for the properties panel
 *   collectConfig(step)  → void    reads DOM, writes into step.config
 *   destroy()            → void    tears down event listeners / AC refs
 */

// ── cda.to_fhir ───────────────────────────────────────────────────────────────

class CDAToFHIRBuilder {
    constructor(panel) { this._panel = panel; }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;
        const esc = s => String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');

        const sectionsDefault = [
            'allergiesAndIntolerances','medications','problems',
            'vitalSigns','results','immunizations','procedures','socialHistory'
        ];
        const selectedSections = Array.isArray(cfg.sections) ? cfg.sections : sectionsDefault;
        const sectionLabels = {
            allergiesAndIntolerances: 'Allergies & Intolerances',
            medications:   'Medications',
            problems:      'Problems / Conditions',
            vitalSigns:    'Vital Signs',
            results:       'Lab Results',
            immunizations: 'Immunizations',
            procedures:    'Procedures',
            socialHistory: 'Social History',
        };

        const sectionCheckboxes = Object.entries(sectionLabels).map(([key, label]) => {
            const checked = selectedSections.includes(key) ? 'checked' : '';
            return `<label style="display:flex;align-items:center;gap:0.4rem;font-size:0.83rem;cursor:pointer;margin-bottom:0.3rem;">
                <input type="checkbox" class="cdaSection" value="${esc(key)}" ${checked}
                    style="accent-color:#2563eb;width:14px;height:14px;">
                ${esc(label)}
            </label>`;
        }).join('');

        const outputField   = esc(cfg.outputField  || '_fhirBundle');
        const inputField    = esc(cfg.inputField   || '');
        const addNarratives = cfg.addNarratives !== false;
        const preserveRaw   = cfg.preserveRaw   === true;

        const html = `
        <div class="cda-step-config">
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Input Field</label>
                <input id="cdaInputField" type="text" class="form-control form-control-sm" value="${inputField}"
                    placeholder="Leave blank to use message envelope raw content"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field containing raw CDA/CCD XML. Blank = use inbound message body.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="cdaOutputField" type="text" class="form-control form-control-sm" value="${outputField}"
                    placeholder="_fhirBundle" style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the FHIR R4 Bundle will be written.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.55rem;">Sections to Convert</label>
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.1rem 1rem;">
                    ${sectionCheckboxes}
                </div>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.35rem;">USCDI v3 sections extracted and mapped to FHIR resources.</div>
            </div>
            <div class="config-group" style="margin-bottom:0.6rem;">
                <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-size:0.83rem;">
                    <input id="cdaAddNarratives" type="checkbox" ${addNarratives ? 'checked' : ''} style="accent-color:#2563eb;width:14px;height:14px;">
                    Generate FHIR narrative (text.div) for each resource
                </label>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.2rem;margin-left:1.3rem;">Calls the FHIR narrative generator for resources without existing narrative.</div>
            </div>
            <div class="config-group">
                <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-size:0.83rem;">
                    <input id="cdaPreserveRaw" type="checkbox" ${preserveRaw ? 'checked' : ''} style="accent-color:#2563eb;width:14px;height:14px;">
                    Preserve raw CDA XML in <code>_rawCDA</code> field
                </label>
            </div>
        </div>`;

        return html;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const inputEl  = form.querySelector('#cdaInputField');
        const outputEl = form.querySelector('#cdaOutputField');
        if (inputEl)  step.config.inputField  = inputEl.value.trim();
        if (outputEl) step.config.outputField = outputEl.value.trim() || '_fhirBundle';

        const sectionBoxes = form.querySelectorAll('.cdaSection:checked');
        step.config.sections = Array.from(sectionBoxes).map(b => b.value);

        const narrativeEl  = form.querySelector('#cdaAddNarratives');
        const preserveEl   = form.querySelector('#cdaPreserveRaw');
        if (narrativeEl) step.config.addNarratives = narrativeEl.checked;
        if (preserveEl)  step.config.preserveRaw   = preserveEl.checked;
    }

    destroy() {}
}

// ── fhir.to_cda ──────────────────────────────────────────────────────────────

class FHIRToCDABuilder {
    constructor(panel) { this._panel = panel; }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;
        const esc = s => String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');

        const inputField     = esc(cfg.inputField   || '_fhirBundle');
        const outputField    = esc(cfg.outputField  || '_cdaXML');
        const template       = esc(cfg.template     || 'discharge_summary');
        const includeNarrative = cfg.includeNarrative !== false;
        const prettyPrint      = cfg.prettyPrint    !== false;

        const templates = [
            { value: 'discharge_summary', label: 'Discharge Summary (LOINC 18842-5)' },
            { value: 'progress_note',     label: 'Progress Note (LOINC 11506-3)' },
            { value: 'continuity_of_care',label: 'Continuity of Care Document (LOINC 34133-9)' },
            { value: 'referral_note',     label: 'Referral Note (LOINC 57133-1)' },
        ];

        const templateOptions = templates.map(t =>
            `<option value="${esc(t.value)}" ${template === t.value ? 'selected' : ''}>${esc(t.label)}</option>`
        ).join('');

        return `
        <div class="cda-step-config">
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Input FHIR Bundle Field</label>
                <input id="fhirToCdaInputField" type="text" class="form-control form-control-sm" value="${inputField}"
                    placeholder="_fhirBundle" style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field containing the FHIR R4 Bundle to serialize.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="fhirToCdaOutputField" type="text" class="form-control form-control-sm" value="${outputField}"
                    placeholder="_cdaXML" style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the generated C-CDA 2.1 XML will be written.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Document Template</label>
                <select id="fhirToCdaTemplate" class="form-select form-select-sm">
                    ${templateOptions}
                </select>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">LOINC-coded document type for the generated CDA header.</div>
            </div>
            <div class="config-group" style="margin-bottom:0.6rem;">
                <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-size:0.83rem;">
                    <input id="fhirToCdaIncludeNarrative" type="checkbox" ${includeNarrative ? 'checked' : ''} style="accent-color:#2563eb;width:14px;height:14px;">
                    Include human-readable narrative in each CDA section
                </label>
            </div>
            <div class="config-group">
                <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-size:0.83rem;">
                    <input id="fhirToCdaPrettyPrint" type="checkbox" ${prettyPrint ? 'checked' : ''} style="accent-color:#2563eb;width:14px;height:14px;">
                    Pretty-print output XML (indented)
                </label>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const inputEl    = form.querySelector('#fhirToCdaInputField');
        const outputEl   = form.querySelector('#fhirToCdaOutputField');
        const templateEl = form.querySelector('#fhirToCdaTemplate');
        const narrativeEl = form.querySelector('#fhirToCdaIncludeNarrative');
        const prettyEl   = form.querySelector('#fhirToCdaPrettyPrint');

        if (inputEl)    step.config.inputField       = inputEl.value.trim()  || '_fhirBundle';
        if (outputEl)   step.config.outputField      = outputEl.value.trim() || '_cdaXML';
        if (templateEl) step.config.template         = templateEl.value;
        if (narrativeEl) step.config.includeNarrative = narrativeEl.checked;
        if (prettyEl)   step.config.prettyPrint      = prettyEl.checked;
    }

    destroy() {}
}

// ── cda.normalize ────────────────────────────────────────────────────────────

class CDANormalizerBuilder {
    constructor(panel) { this._panel = panel; }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;
        const esc = s => String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');

        const inputField    = esc(cfg.inputField  || '');
        const outputField   = esc(cfg.outputField || '');
        const strictMode    = cfg.strictMode === true;
        const logSubstitutions = cfg.logSubstitutions !== false;

        return `
        <div class="cda-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <i class="fas fa-info-circle" style="margin-right:0.4rem;"></i>
                <strong>Pre-parse step.</strong> Converts C32/HITSP template OIDs to C-CDA 2.1 equivalents on raw XML fields.
                Run this <em>before</em> a <code>cda.to_fhir</code> step when the source system emits legacy CDA formats.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Input Field (Raw XML)</label>
                <input id="cdaNormInputField" type="text" class="form-control form-control-sm" value="${inputField}"
                    placeholder="Leave blank to use inbound message body"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field containing the raw C32 or HITSP XML string to normalize.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="cdaNormOutputField" type="text" class="form-control form-control-sm" value="${outputField}"
                    placeholder="Defaults to overwriting input field"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Leave blank to overwrite the input field in place.</div>
            </div>
            <div class="config-group" style="margin-bottom:0.6rem;">
                <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-size:0.83rem;">
                    <input id="cdaNormStrictMode" type="checkbox" ${strictMode ? 'checked' : ''} style="accent-color:#2563eb;width:14px;height:14px;">
                    Strict mode — fail step on unrecognized template OIDs
                </label>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.2rem;margin-left:1.3rem;">Default: pass through unknown OIDs unchanged.</div>
            </div>
            <div class="config-group">
                <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-size:0.83rem;">
                    <input id="cdaNormLogSubs" type="checkbox" ${logSubstitutions ? 'checked' : ''} style="accent-color:#2563eb;width:14px;height:14px;">
                    Log template OID substitutions to pipeline trace
                </label>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const inputEl  = form.querySelector('#cdaNormInputField');
        const outputEl = form.querySelector('#cdaNormOutputField');
        const strictEl = form.querySelector('#cdaNormStrictMode');
        const logEl    = form.querySelector('#cdaNormLogSubs');

        if (inputEl)  step.config.inputField     = inputEl.value.trim();
        if (outputEl) step.config.outputField    = outputEl.value.trim();
        if (strictEl) step.config.strictMode     = strictEl.checked;
        if (logEl)    step.config.logSubstitutions = logEl.checked;
    }

    destroy() {}
}

// ── Registration ──────────────────────────────────────────────────────────────

if (typeof StepBuilderRegistry !== 'undefined') {
    StepBuilderRegistry.register('cda.to_fhir',  CDAToFHIRBuilder);
    StepBuilderRegistry.register('fhir.to_cda',  FHIRToCDABuilder);
    StepBuilderRegistry.register('cda.normalize', CDANormalizerBuilder);
}
