/**
 * CDAStepBuilder — step config builders for CDA/CCD pipeline steps.
 *
 * Registers four step types with StepBuilderRegistry:
 *   - cda.parse      Parse raw CDA/CCD XML to USCDI-keyed JSON
 *   - cda.to_fhir    Convert parsedCDA JSON to FHIR R4 Bundle (4-tab full builder)
 *   - fhir.to_cda    Serialize FHIR R4 Bundle to C-CDA 2.1 XML
 *   - cda.normalize  Normalize C32/HITSP template OIDs to C-CDA 2.1 before parsing
 *
 * Builder contract (StepBuilderRegistry):
 *   render(step)         → string  HTML for the properties panel form tab
 *   collectConfig(step)  → void    reads DOM, writes into step.config
 *   destroy()            → void    tears down event listeners / AC refs
 */

// ── CdaParseStepBuilder ────────────────────────────────────────────────────────
// Sprint E1: Minimal 1-tab builder for cda.parse.
// Config: { sourceField, outputField, documentTypeHint }

class CdaParseStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

        const sourceField  = esc(cfg.sourceField  || 'raw');
        const outputField  = esc(cfg.outputField  || 'parsedCDA');
        const docTypeHint  = cfg.documentTypeHint || '';

        const docTypeOptions = [
            { value: '',                  label: 'Auto-detect' },
            { value: 'CCD',               label: 'CCD (Continuity of Care Document)' },
            { value: 'Discharge Summary', label: 'Discharge Summary' },
            { value: 'Referral Note',     label: 'Referral Note' },
            { value: 'H&P',               label: 'History & Physical' },
            { value: 'Consultation',      label: 'Consultation Note' },
            { value: 'Progress Note',     label: 'Progress Note' },
            { value: 'Care Plan',         label: 'Care Plan' },
            { value: 'Transfer Summary',  label: 'Transfer Summary' },
            { value: 'Diagnostic Imaging Report', label: 'Diagnostic Imaging Report' },
            { value: 'Operative Note',    label: 'Operative Note' },
            { value: 'Procedure Note',    label: 'Procedure Note' },
            { value: 'Unstructured Document', label: 'Unstructured Document' },
        ].map(o => `<option value="${esc(o.value)}" ${o.value === docTypeHint ? 'selected' : ''}>${esc(o.label)}</option>`).join('');

        return `
        <div class="cda-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Parse step.</strong> Converts raw CDA/CCD XML into USCDI-keyed JSON.
                Place this step after <code>cda.normalize</code> and before <code>cda.to_fhir</code>.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="cdaParseSourceField" type="text" class="form-control form-control-sm"
                    value="${sourceField}" placeholder="raw"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field containing the raw CDA XML string.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="cdaParseOutputField" type="text" class="form-control form-control-sm"
                    value="${outputField}" placeholder="parsedCDA"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the USCDI-keyed JSON will be written.</div>
            </div>
            <div class="config-group">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Document Type Hint</label>
                <select id="cdaParseDocTypeHint" class="form-select form-select-sm">
                    ${docTypeOptions}
                </select>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">
                    Overrides automatic document type detection from clinical document templateId OIDs.
                </div>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const sourceEl  = form.querySelector('#cdaParseSourceField');
        const outputEl  = form.querySelector('#cdaParseOutputField');
        const hintEl    = form.querySelector('#cdaParseDocTypeHint');

        if (sourceEl)  step.config.sourceField        = sourceEl.value.trim() || 'raw';
        if (outputEl)  step.config.outputField        = outputEl.value.trim() || 'parsedCDA';
        if (hintEl)    step.config.documentTypeHint   = hintEl.value;
    }

    destroy() {
        this._ac.abort();
    }
}


// ── CdaToFhirStepBuilder ───────────────────────────────────────────────────────
// Sprint E2: Full 4-tab builder for cda.to_fhir.
// Tabs: General | Sections | Terminology | Advanced
// Includes: section field editor, OOB/modified badge, type-pair inference, OOB upgrade banner.

class CdaToFhirStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac    = new AbortController();
        // Runtime state set by render()
        this._step             = null;
        this._sections         = [];   // from /api/cda/schema/sections
        this._oobVersion       = null; // from /api/cda/templates/:docType/version
        this._oobTemplateId    = null;
        this._activeTab        = 'general';
        this._editingSection   = null; // sectionKey currently open in field editor
        this._sectionFields    = {};   // cache: sectionKey → []fieldSummary
        this._translations     = [];   // code translation table rows
        this._fieldSearchQuery = '';   // current text in the Section field editor search box
        this._transformDescriptions = null; // name → description, from /api/cda/transforms
        this._sectionFieldPaths = [];  // FHIR path candidates for the currently-open section's autocomplete
        this._activeAutocompleteCallback = null; // (value) => void, set by whichever input last opened the dropdown
        this._ruleVariantsBySection = {}; // cache: sectionKey → [{fhirResource, entryMatch, nestableGroups}]
        this._pendingScope = '';       // Add Field modal's staged Scope (either tab writes here)
        this._pendingSourcePath = '';  // Add Field modal's staged SourcePath
        this._testDataStepNames = undefined; // resolved lazily by _getCDADocumentSourceStepNames(), cached per builder instance

        // Register global instance so inline onclick handlers can reach this builder.
        window._cdaToFhirBuilder = this;
    }

    // ── render ────────────────────────────────────────────────────────────────

    render(step) {
        this._step = step;
        if (!step.config) step.config = {};
        this._applyDefaultConfig(step.config);

        // Kick off async data loading (tabs will render inline-loading until ready)
        this._loadSections(step.config);
        this._checkOOBVersion(step.config);
        this._loadTransformDescriptions();

        return `
<div id="cdaToFhirBuilder" style="font-size:0.84rem;">
    ${this._renderUpgradeBanner(step.config)}
    <div style="display:flex;gap:0;background:#1e3a8a;border-radius:8px 8px 0 0;margin-bottom:1.1rem;padding:0 0.25rem;">
        ${this._renderTabButton('general',     'General')}
        ${this._renderTabButton('sections',    'Sections')}
        ${this._renderTabButton('assembly',    'Assembly')}
        ${this._renderTabButton('terminology', 'Terminology')}
        ${this._renderTabButton('advanced',    'Advanced')}
    </div>
    <div id="cdaToFhirTab-general"     style="${this._activeTab === 'general'     ? '' : 'display:none'}">
        ${this._renderGeneralTab(step.config)}
    </div>
    <div id="cdaToFhirTab-sections"    style="${this._activeTab === 'sections'    ? '' : 'display:none'}">
        ${this._renderSectionsTab(step.config)}
    </div>
    <div id="cdaToFhirTab-assembly"    style="${this._activeTab === 'assembly'    ? '' : 'display:none'}">
        ${this._renderAssemblyTab(step.config)}
    </div>
    <div id="cdaToFhirTab-terminology" style="${this._activeTab === 'terminology' ? '' : 'display:none'}">
        ${this._renderTerminologyTab(step.config)}
    </div>
    <div id="cdaToFhirTab-advanced"    style="${this._activeTab === 'advanced'    ? '' : 'display:none'}">
        ${this._renderAdvancedTab(step.config)}
    </div>
    ${this._renderSectionFieldEditor()}
    ${this._renderTranslationModal()}
</div>`;
    }

    // ── collectConfig ─────────────────────────────────────────────────────────

    collectConfig(step) {
        const root = document.getElementById('cdaToFhirBuilder');
        if (!root) return;
        step.config = step.config || {};

        // General tab
        const pick = id => { const el = root.querySelector('#' + id); return el ? el.value.trim() : null; };
        const check = id => { const el = root.querySelector('#' + id); return el ? el.checked : null; };

        if (pick('cda2fhirSourceField')   !== null) step.config.sourceField   = pick('cda2fhirSourceField')   || 'parsedCDA';
        if (pick('cda2fhirOutputField')   !== null) step.config.outputField   = pick('cda2fhirOutputField')   || 'fhirBundle';
        if (pick('cda2fhirBundleType')    !== null) step.config.bundleType    = pick('cda2fhirBundleType');
        if (pick('cda2fhirProfileMode')   !== null) step.config.profileMode   = pick('cda2fhirProfileMode');
        if (pick('cda2fhirDocType')       !== null) step.config.documentType  = pick('cda2fhirDocType');

        // Sections tab: collect checkbox state
        const sectionOverrides = step.config.sectionOverrides || {};
        root.querySelectorAll('.cda-section-checkbox').forEach(cb => {
            const key = cb.dataset.sectionKey;
            if (!sectionOverrides[key]) sectionOverrides[key] = {};
            sectionOverrides[key].enabled = cb.checked;
        });
        step.config.sectionOverrides = sectionOverrides;

        // The executor (cda_to_fhir_executor.go) reads a flat
        // step.config.enabledSections: string[] — sectionOverrides above is
        // this builder's own UI-state bookkeeping only and is never read by
        // the engine. Derive the flat list the engine actually consumes.
        // Only set it when at least one section is explicitly disabled —
        // an empty/absent enabledSections means "all sections enabled" to
        // the executor, matching a freshly-created step where no checkbox
        // has been touched yet.
        if (this._sections && this._sections.length > 0) {
            const allKeys = this._sections.map(s => s.key);
            const enabledKeys = allKeys.filter(key => (sectionOverrides[key] || {}).enabled !== false);
            if (enabledKeys.length < allKeys.length) {
                step.config.enabledSections = enabledKeys;
            } else {
                delete step.config.enabledSections;
            }
        }

        // Terminology tab
        if (check('cda2fhirTermValidation') !== null) step.config.terminologyValidation = check('cda2fhirTermValidation');
        if (check('cda2fhirCodeTranslation') !== null) step.config.codeTranslation = check('cda2fhirCodeTranslation');

        // Assembly tab
        const assemblyRules = step.config.assemblyRules || {};
        root.querySelectorAll('.cda-assembly-toggle').forEach(cb => {
            assemblyRules[cb.dataset.ruleKey] = cb.checked;
        });
        step.config.assemblyRules = assemblyRules;

        // Plan-of-Care encounter target -- a 3-way string choice, not a
        // boolean, so it's its own top-level key (matching
        // cda_to_fhir_executor.go's cdaToFHIRConfig.PlanOfCareEncounterTarget
        // JSON tag exactly), not folded into assemblyRules.
        const poCarTarget = pick('cda2fhirPlanOfCareEncounterTarget');
        if (poCarTarget) {
            step.config.planOfCareEncounterTarget = poCarTarget;
        } else {
            delete step.config.planOfCareEncounterTarget;
        }

        // Advanced tab
        if (pick('cda2fhirMergeMode')       !== null) step.config.mergeMode          = pick('cda2fhirMergeMode');
        if (pick('cda2fhirOnSectionFail')   !== null) step.config.onSectionFailure   = pick('cda2fhirOnSectionFail');
        if (pick('cda2fhirLogLevel')        !== null) step.config.logLevel            = pick('cda2fhirLogLevel');
        if (check('cda2fhirProcResult')     !== null) step.config.includeProcessingResult = check('cda2fhirProcResult');
    }

    destroy() {
        this._ac.abort();
        if (window._cdaToFhirBuilder === this) {
            window._cdaToFhirBuilder = null;
        }
    }

    // ── Default config ────────────────────────────────────────────────────────

    _applyDefaultConfig(cfg) {
        if (!cfg.sourceField)   cfg.sourceField   = 'parsedCDA';
        if (!cfg.outputField)   cfg.outputField   = 'fhirBundle';
        if (!cfg.bundleType)    cfg.bundleType    = 'collection';
        if (!cfg.profileMode)   cfg.profileMode   = 'us-core';
        if (!cfg.documentType)  cfg.documentType  = 'auto';
        if (cfg.terminologyValidation  === undefined) cfg.terminologyValidation  = false;
        if (cfg.codeTranslation        === undefined) cfg.codeTranslation        = false;
        if (!cfg.mergeMode)     cfg.mergeMode     = 'append';
        if (!cfg.onSectionFailure) cfg.onSectionFailure = 'continue';
        if (!cfg.logLevel)      cfg.logLevel      = 'warning';
        if (cfg.includeProcessingResult === undefined) cfg.includeProcessingResult = true;
        if (!cfg.sectionOverrides)  cfg.sectionOverrides  = {};
        if (!cfg.assemblyRules)     cfg.assemblyRules     = {};
    }

    // ── Tab navigation (callable from inline onclick) ─────────────────────────

    switchTab(tabName) {
        this._activeTab = tabName;
        // Colors here MUST match _renderTabButton's initial render exactly —
        // this tab bar's own background is #1e3a8a (dark blue, see render()'s
        // container div). This function previously used a color scheme sized
        // for a WHITE tab bar (#1e3a8a active text/border, #6b7280 inactive
        // text) — on THIS dark-blue background, the active tab's text and
        // underline became literally the same color as the background the
        // instant a tab was clicked, i.e. invisible, even though the very
        // first render (before any click) looked correct.
        ['general', 'sections', 'assembly', 'terminology', 'advanced'].forEach(t => {
            const panel = document.getElementById('cdaToFhirTab-' + t);
            const btn   = document.getElementById('cdaToFhirTabBtn-' + t);
            if (panel) panel.style.display = (t === tabName) ? '' : 'none';
            if (btn) {
                if (t === tabName) {
                    btn.style.borderBottom = '3px solid #f9a8d4';
                    btn.style.background = 'rgba(255,255,255,0.12)';
                    btn.style.color = '#ffffff';
                    btn.style.fontWeight = '700';
                } else {
                    btn.style.borderBottom = '3px solid transparent';
                    btn.style.background = 'transparent';
                    btn.style.color = 'rgba(255,255,255,0.6)';
                    btn.style.fontWeight = '400';
                }
            }
        });
    }

    // ── Upgrade banner ────────────────────────────────────────────────────────

    _renderUpgradeBanner(cfg) {
        // Banner is hidden initially; _checkOOBVersion() will show it if needed.
        return `<div id="cdaToFhirUpgradeBanner" style="display:none;background:#fef9c3;border:1px solid #fde047;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:0.75rem;font-size:0.8rem;color:#92400e;">
            <strong>OOB template updated.</strong>
            <span id="cdaToFhirBannerMsg"></span>
            <div style="margin-top:0.4rem;display:flex;gap:0.5rem;">
                <button type="button" onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.reviewUpgrade()"
                    style="padding:0.2rem 0.6rem;background:#92400e;color:white;border:none;border-radius:4px;cursor:pointer;font-size:0.75rem;">
                    Review
                </button>
                <button type="button" onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.applyUpgrade()"
                    style="padding:0.2rem 0.6rem;background:#1e3a8a;color:white;border:none;border-radius:4px;cursor:pointer;font-size:0.75rem;">
                    Upgrade
                </button>
                <button type="button" onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.dismissBanner()"
                    style="padding:0.2rem 0.6rem;background:transparent;border:1px solid #92400e;color:#92400e;border-radius:4px;cursor:pointer;font-size:0.75rem;">
                    Dismiss
                </button>
            </div>
        </div>`;
    }

    // ── Tab button HTML ───────────────────────────────────────────────────────

    _renderTabButton(key, label) {
        const active = this._activeTab === key;
        return `<button id="cdaToFhirTabBtn-${key}" type="button"
            onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.switchTab('${key}')"
            style="padding:0.45rem 0.95rem;border:none;
                   border-bottom:3px solid ${active ? '#f9a8d4' : 'transparent'};
                   background:${active ? 'rgba(255,255,255,0.12)' : 'transparent'};
                   cursor:pointer;font-size:0.78rem;
                   color:${active ? '#ffffff' : 'rgba(255,255,255,0.6)'};
                   font-weight:${active ? '700' : '400'};
                   border-radius:6px 6px 0 0;
                   transition:color 0.15s,background 0.15s;">
            ${label}
        </button>`;
    }

    // ── General tab ───────────────────────────────────────────────────────────

    _renderGeneralTab(cfg) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

        const bundleOpts = ['collection','document','transaction'].map(v =>
            `<option value="${v}" ${cfg.bundleType === v ? 'selected' : ''}>${v}</option>`
        ).join('');

        const profileOpts = [
            { v: 'us-core', l: 'US Core R4 6.1.0' },
            { v: 'base',    l: 'Base FHIR R4 (no profile)' },
        ].map(o => `<option value="${o.v}" ${cfg.profileMode === o.v ? 'selected' : ''}>${o.l}</option>`).join('');

        const docTypeOpts = [
            { v: 'auto',               l: 'Auto-detect' },
            { v: 'CCD',                l: 'CCD' },
            { v: 'Discharge Summary',  l: 'Discharge Summary' },
            { v: 'Referral Note',      l: 'Referral Note' },
            { v: 'H&P',                l: 'History & Physical' },
            { v: 'Consultation',       l: 'Consultation Note' },
            { v: 'Progress Note',      l: 'Progress Note' },
            { v: 'Care Plan',          l: 'Care Plan' },
            { v: 'Transfer Summary',   l: 'Transfer Summary' },
            { v: 'Diagnostic Imaging Report', l: 'Diagnostic Imaging Report' },
            { v: 'Operative Note',     l: 'Operative Note' },
            { v: 'Procedure Note',     l: 'Procedure Note' },
            { v: 'Unstructured Document', l: 'Unstructured Document' },
        ].map(o => `<option value="${o.v}" ${cfg.documentType === o.v ? 'selected' : ''}>${o.l}</option>`).join('');

        const lbl = `font-size:0.7rem;font-weight:700;text-transform:uppercase;letter-spacing:0.05em;
                      color:#1e3a8a;display:block;margin-bottom:0.4rem;
                      border-left:3px solid #f472b6;padding-left:0.45rem;`;
        const hint = `font-size:0.71rem;color:#f472b6;font-style:italic;margin-top:0.28rem;`;
        const inputWrap = `position:relative;`;
        const inputStyle = `width:100%;font-family:monospace;font-size:0.82rem;
            background:linear-gradient(135deg,#ffffff 0%,#f8fafc 100%);
            border:1px solid #cbd5e1;border-radius:6px;padding:0.38rem 2rem 0.38rem 0.55rem;
            box-shadow:inset 0 1px 2px rgba(0,0,0,0.05);
            transition:border-color 0.15s,box-shadow 0.15s;outline:none;color:#1f2937;box-sizing:border-box;`;
        const pencilBtn = id => `<button type="button"
            onclick="document.getElementById('${id}').focus()"
            style="position:absolute;right:0.45rem;top:50%;transform:translateY(-50%);
                   background:none;border:none;cursor:pointer;color:#94a3b8;padding:0;font-size:0.8rem;
                   line-height:1;" title="Edit">✏️</button>`;
        const selectStyle = `width:100%;font-size:0.82rem;
            background:linear-gradient(135deg,#ffffff 0%,#f8fafc 100%);
            border:1px solid #cbd5e1;border-radius:6px;padding:0.38rem 0.55rem;
            box-shadow:inset 0 1px 2px rgba(0,0,0,0.05);color:#1f2937;
            appearance:none;-webkit-appearance:none;
            background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6' viewBox='0 0 10 6'%3E%3Cpath d='M0 0l5 6 5-6z' fill='%231e3a8a'/%3E%3C/svg%3E");
            background-repeat:no-repeat;background-position:right 0.6rem center;background-size:8px;
            padding-right:1.8rem;`;
        const grp = `margin-bottom:1.1rem;`;

        return `
        <style>
            #cda2fhirSourceField:focus,#cda2fhirOutputField:focus{
                border-color:#1e3a8a!important;box-shadow:0 0 0 2px rgba(30,58,138,0.12),inset 0 1px 2px rgba(0,0,0,0.05)!important;
            }
        </style>
        <div class="config-group" style="${grp}">
            <label style="${lbl}">Source Field</label>
            <div style="${inputWrap}">
                <input id="cda2fhirSourceField" type="text"
                    value="${esc(cfg.sourceField)}" placeholder="parsedCDA"
                    style="${inputStyle}">
                ${pencilBtn('cda2fhirSourceField')}
            </div>
            <div style="${hint}">Pipeline field containing the parsedCDA JSON (from cda.parse step).</div>
        </div>
        <div class="config-group" style="${grp}">
            <label style="${lbl}">Output Field</label>
            <div style="${inputWrap}">
                <input id="cda2fhirOutputField" type="text"
                    value="${esc(cfg.outputField)}" placeholder="fhirBundle"
                    style="${inputStyle}">
                ${pencilBtn('cda2fhirOutputField')}
            </div>
            <div style="${hint}">Pipeline field where the FHIR R4 Bundle will be written.</div>
        </div>
        <div class="config-group" style="${grp}">
            <label style="${lbl}">Bundle Type</label>
            <div style="${inputWrap}">
                <select id="cda2fhirBundleType" style="${selectStyle}">${bundleOpts}</select>
            </div>
            <div style="${hint}">collection = searchset · document = adds Composition entry · transaction = request entries</div>
        </div>
        <div class="config-group" style="${grp}">
            <label style="${lbl}">Profile Mode</label>
            <div style="${inputWrap}">
                <select id="cda2fhirProfileMode" style="${selectStyle}">${profileOpts}</select>
            </div>
            <div style="${hint}">Injects meta.profile URLs for US Core validation.</div>
        </div>
        <div class="config-group">
            <label style="${lbl}">Document Type</label>
            <div style="${inputWrap}">
                <select id="cda2fhirDocType" style="${selectStyle}">${docTypeOpts}</select>
            </div>
            <div style="${hint}">Overrides document type detection from clinical document OIDs.</div>
        </div>`;
    }

    // ── Sections tab ──────────────────────────────────────────────────────────

    _renderSectionsTab(cfg) {
        if (!this._sections || this._sections.length === 0) {
            return `<div id="cdaSectionsLoading" style="text-align:center;padding:2rem;color:#6b7280;font-size:0.8rem;">
                Loading sections from schema…
            </div>`;
        }

        const overrides = cfg.sectionOverrides || {};
        const rows = this._sections.map(sec => {
            const secOverride = overrides[sec.key] || {};
            const enabled     = secOverride.enabled !== false; // default true
            const modified    = this._isSectionModified(sec.key, overrides);
            const badge       = modified
                ? `<span style="font-size:0.68rem;background:#fef3c7;color:#92400e;border:1px solid #fde68a;border-radius:3px;padding:1px 5px;margin-left:4px;">modified</span>`
                : `<span style="font-size:0.68rem;background:#f0fdf4;color:#166534;border:1px solid #bbf7d0;border-radius:3px;padding:1px 5px;margin-left:4px;">OOB</span>`;
            const editLabel = enabled ? 'Edit' : 'Enable';
            return `
            <tr style="border-bottom:1px solid #f1f5f9;">
                <td style="padding:0.4rem 0.5rem;width:32px;text-align:center;">
                    <input type="checkbox" class="cda-section-checkbox"
                        data-section-key="${sec.key}"
                        ${enabled ? 'checked' : ''}
                        style="accent-color:#1e3a8a;width:14px;height:14px;"
                        onchange="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onSectionToggle('${sec.key}', this.checked)">
                </td>
                <td style="padding:0.4rem 0.5rem;">
                    <span style="font-weight:${enabled ? '500' : '400'};color:${enabled ? '#1e293b' : '#94a3b8'};">${sec.displayName || sec.key}</span>
                    ${badge}
                </td>
                <td style="padding:0.4rem 0.5rem;color:#94a3b8;font-size:0.75rem;text-align:right;">${sec.fieldCount || 0} fields</td>
                <td style="padding:0.4rem 0.5rem;text-align:right;">
                    <button type="button"
                        onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.openSectionEditor('${sec.key}')"
                        style="padding:0.2rem 0.55rem;font-size:0.72rem;background:#f8fafc;border:1px solid #cbd5e1;border-radius:4px;cursor:pointer;color:#334155;">
                        ${editLabel}
                    </button>
                </td>
            </tr>`;
        }).join('');

        return `
        <div style="margin-bottom:0.75rem;display:flex;align-items:center;justify-content:space-between;">
            <span style="font-size:0.78rem;color:#64748b;">Check sections to include in the FHIR Bundle. Edit to customise field mappings.</span>
            <button type="button" onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.saveDelta()"
                style="padding:0.25rem 0.7rem;font-size:0.75rem;background:#1e3a8a;color:white;border:none;border-radius:4px;cursor:pointer;">
                Save Overrides
            </button>
        </div>
        <div style="overflow:auto;max-height:320px;border:1px solid #e2e8f0;border-radius:6px;">
            <table style="width:100%;border-collapse:collapse;">
                <tbody id="cdaSectionsList">${rows}</tbody>
            </table>
        </div>`;
    }

    // ── Terminology tab ───────────────────────────────────────────────────────

    _renderTerminologyTab(cfg) {
        const lbl = `font-size:0.7rem;font-weight:700;text-transform:uppercase;letter-spacing:0.05em;
                      color:#1e3a8a;display:block;margin-bottom:0.5rem;
                      border-left:3px solid #f472b6;padding-left:0.45rem;`;
        const hint = `font-size:0.71rem;color:#f472b6;font-style:italic;margin-top:0.25rem;margin-left:1.35rem;`;
        return `
        <div class="config-group" style="margin-bottom:1.1rem;">
            <label style="${lbl}">Code Validation</label>
            <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-size:0.83rem;color:#1f2937;">
                <input id="cda2fhirTermValidation" type="checkbox"
                    ${cfg.terminologyValidation ? 'checked' : ''}
                    style="accent-color:#1e3a8a;width:14px;height:14px;">
                Validate SNOMED CT, LOINC, RxNorm, and CVX code formats
            </label>
            <div style="${hint}">Checks that each code matches its system's expected shape (e.g. SNOMED CT is all-digit, LOINC is NNNNN-N) — it does NOT look the code up against VSAC (the NLM's Value Set Authority Center), so a well-formatted but non-existent code still passes. Failures show up as warnings in the step's processing result, not as hard errors that stop the pipeline.</div>
        </div>
        <div class="config-group">
            <label style="${lbl}">Code Translation</label>
            <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-size:0.83rem;color:#1f2937;margin-bottom:0.55rem;">
                <input id="cda2fhirCodeTranslation" type="checkbox"
                    ${cfg.codeTranslation ? 'checked' : ''}
                    style="accent-color:#1e3a8a;width:14px;height:14px;">
                Apply code translation table during transformation
            </label>
            <button type="button"
                onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.openTranslationModal()"
                style="padding:0.3rem 0.75rem;font-size:0.78rem;
                       background:linear-gradient(135deg,#1e3a8a 0%,#1e40af 100%);
                       color:white;border:none;border-radius:5px;cursor:pointer;
                       box-shadow:0 1px 3px rgba(30,58,138,0.3);">
                Manage Translation Table
            </button>
            <div style="${hint}">Independent of Code Validation above — this swaps a source code for a target code using a lookup table you manage per interface (e.g. a legacy EHR still sending ICD-9-CM gets it converted to ICD-10-CM). Use this when your source system's codes don't already match what your downstream FHIR consumers expect.</div>
        </div>`;
    }

    // ── Assembly tab ──────────────────────────────────────────────────────────

    _renderAssemblyTab(cfg) {
        const rules = cfg.assemblyRules || {};
        const on = key => rules[key] !== false; // default ON

        // ── Data type dispatch (parallel to HL7's obs_value_dispatch) ──────────
        const DATATYPE_RULES = [
            { key: 'obs_value_type_dispatch',
              src: 'observation/@xsi:type',   fhir: 'Observation.value[x]',
              label: 'Observation value type dispatch',
              desc:  'Reads the xsi:type attribute of each CDA observation/value element to select the correct FHIR value[x] variant (valueQuantity, valueCodeableConcept, valueString, valueBoolean).',
              example: 'xsi:type="PQ" value="4.2" unit="mmol/L"\n→ valueQuantity: { value: 4.2, unit: "mmol/L" }\nxsi:type="CE" code="N" displayName="Normal"\n→ valueCodeableConcept: { coding: [{ code: "N" }] }\nxsi:type="ST" → valueString\nxsi:type="BL" → valueBoolean' },
            { key: 'pq_to_quantity_unit',
              src: 'value[@xsi:type="PQ"]/@unit', fhir: 'Quantity.system (UCUM)',
              label: 'PQ unit → UCUM Quantity',
              desc:  'When unit is a valid UCUM code, attaches the standard UCUM system URI to the Quantity. Non-UCUM units are preserved as display-only.',
              example: 'unit="mmol/L"  → system: "http://unitsofmeasure.org", code: "mmol/L"\nunit="mg/dL"   → system: "http://unitsofmeasure.org", code: "mg/dL"\nunit="ratio"   → unit: "ratio" (no system — not UCUM)' },
            { key: 'cd_to_codeable_concept',
              src: 'CD / CE elements',          fhir: 'CodeableConcept',
              label: 'CD/CE → structured CodeableConcept',
              desc:  'Converts any CDA CD or CE coded element to a fully structured FHIR CodeableConcept with a coding array, including display text and code system URI.',
              example: 'code="73211009" displayName="Diabetes mellitus" codeSystem="2.16…6.96"\n→ { coding: [{ code: "73211009", display: "Diabetes mellitus",\n               system: "http://snomed.info/sct" }] }' },
        ];

        // ── Code system OID → URI (parallel to HL7's LOINC system inference) ──
        const OID_RULES = [
            { key: 'loinc_oid_to_uri',
              src: 'codeSystem="2.16.840.1.113883.6.1"',   fhir: 'coding.system',
              label: 'LOINC OID → URI',
              desc:  'Replaces the LOINC OID with the standard FHIR system URI on every coding element whose codeSystem matches.',
              example: 'codeSystem="2.16.840.1.113883.6.1"\n→ system: "http://loinc.org"' },
            { key: 'snomed_oid_to_uri',
              src: 'codeSystem="2.16.840.1.113883.6.96"',  fhir: 'coding.system',
              label: 'SNOMED CT OID → URI',
              desc:  'Replaces the SNOMED CT OID with the standard FHIR system URI.',
              example: 'codeSystem="2.16.840.1.113883.6.96"\n→ system: "http://snomed.info/sct"' },
            { key: 'rxnorm_oid_to_uri',
              src: 'codeSystem="2.16.840.1.113883.6.88"',  fhir: 'coding.system',
              label: 'RxNorm OID → URI',
              desc:  'Replaces the RxNorm OID with the standard FHIR system URI on medication code elements.',
              example: 'codeSystem="2.16.840.1.113883.6.88"\n→ system: "http://www.nlm.nih.gov/research/umls/rxnorm"' },
            { key: 'icd10_oid_to_uri',
              src: 'codeSystem="2.16.840.1.113883.6.90"',  fhir: 'coding.system',
              label: 'ICD-10-CM OID → URI',
              desc:  'Replaces the ICD-10-CM OID with the standard FHIR system URI on diagnosis code elements.',
              example: 'codeSystem="2.16.840.1.113883.6.90"\n→ system: "http://hl7.org/fhir/sid/icd-10-cm"' },
        ];

        // ── Structural wiring (parallel to HL7's obs_subject, dr_result_links) ─
        const WIRING_RULES = [
            { key: 'obs_subject_ref',
              src: 'Patient (assembled)',        fhir: 'Observation.subject.reference',
              label: 'Observation → Patient reference',
              desc:  'Wires a reference to the assembled Patient resource into every Observation.subject field produced from result sections.',
              example: 'Patient id="patient-abc"\n→ Observation.subject = { reference: "Patient/patient-abc" }' },
            { key: 'condition_subject_ref',
              src: 'Patient (assembled)',        fhir: 'Condition.subject.reference',
              label: 'Condition → Patient reference',
              desc:  'Wires the Patient reference into Condition.subject for all problem/diagnosis resources.',
              example: 'Patient id="patient-abc"\n→ Condition.subject = { reference: "Patient/patient-abc" }' },
            { key: 'allergy_patient_ref',
              src: 'Patient (assembled)',        fhir: 'AllergyIntolerance.patient.reference',
              label: 'AllergyIntolerance → Patient reference',
              desc:  'Wires the Patient reference into AllergyIntolerance.patient for all allergy resources.',
              example: 'Patient id="patient-abc"\n→ AllergyIntolerance.patient = { reference: "Patient/patient-abc" }' },
            { key: 'obs_reference_range',
              src: 'observationRange/value',     fhir: 'Observation.referenceRange[]',
              label: 'Reference range parsing',
              desc:  'Parses CDA reference range text (e.g. "3.5-5.0", "<10", ">2.0") into a structured FHIR referenceRange with typed low/high boundaries.',
              example: 'referenceRange/observationRange/value="3.5-5.0"\n→ referenceRange: [{ low: { value: 3.5 }, high: { value: 5.0 }, text: "3.5-5.0" }]\n"<10.0" → { high: { value: 10.0 } }' },
            { key: 'allergy_reaction_assembly',
              src: 'entryRelationship (reaction)',fhir: 'AllergyIntolerance.reaction[]',
              label: 'Allergy reaction assembly',
              desc:  'Converts CDA entryRelationship observation entries nested under an allergy act into structured AllergyIntolerance.reaction entries with manifestation codes and severity.',
              example: 'entryRelationship/observation/code="39579001" (Anaphylaxis)\n→ reaction[0].manifestation[0].coding[0].code = "39579001"\n   reaction[0].severity = "severe"' },
            { key: 'med_dosage_assembly',
              src: 'substanceAdministration',    fhir: 'MedicationStatement.dosage[]',
              label: 'Medication dosage assembly',
              desc:  'Assembles doseQuantity, routeCode, and effectiveTime from a CDA substanceAdministration entry into a structured FHIR Dosage object.',
              example: 'doseQuantity value="500" unit="mg"\nrouteCode code="C38288" displayName="Oral"\n→ dosage[0].doseAndRate[0].doseQuantity = { value: 500, unit: "mg" }\n   dosage[0].route.coding[0].code = "C38288"' },
            { key: 'composition_section_refs',
              src: 'Per-section resources',      fhir: 'Composition.section[].entry[]',
              label: 'Section resource refs → Composition.section[]',
              desc:  'After all sections process, adds a Composition.section entry for each enabled section linking its produced FHIR resources by reference.',
              example: 'Allergies section → 3 AllergyIntolerance resources\n→ Composition.section[0].entry = [\n    { reference: "AllergyIntolerance/id-1" },\n    { reference: "AllergyIntolerance/id-2" },\n    { reference: "AllergyIntolerance/id-3" }\n  ]' },
        ];

        // ── Status mapping rules ───────────────────────────────────────────────
        const STATUS_RULES = [
            { key: 'obs_status_mapping',
              src: 'observation/statusCode/@code', fhir: 'Observation.status',
              label: 'Observation status mapping',
              desc:  'Translates CDA statusCode values to the FHIR observation-status value set.',
              example: 'statusCode="completed"    → "final"\nstatusCode="active"       → "preliminary"\nstatusCode="aborted"      → "entered-in-error"\nstatusCode="cancelled"    → "cancelled"' },
            { key: 'allergy_status_mapping',
              src: 'act/statusCode/@code',         fhir: 'AllergyIntolerance.clinicalStatus',
              label: 'Allergy clinical status mapping',
              desc:  'Maps CDA allergy act statusCode to FHIR AllergyIntolerance.clinicalStatus and verificationStatus coded values.',
              example: 'statusCode="active"       → clinicalStatus: "active"\nstatusCode="completed"    → clinicalStatus: "resolved"\nstatusCode="aborted"      → verificationStatus: "entered-in-error"' },
            { key: 'condition_status_mapping',
              src: 'observation/statusCode/@code', fhir: 'Condition.clinicalStatus',
              label: 'Condition clinical status mapping',
              desc:  'Maps CDA problem observation statusCode to FHIR Condition.clinicalStatus and verificationStatus.',
              example: 'statusCode="active"       → clinicalStatus: "active"\nstatusCode="completed"    → clinicalStatus: "resolved"' },
        ];

        // r.desc/r.example already exist on every rule below but, until now,
        // were never rendered anywhere — the checkbox + terse "src → fhir"
        // line was ALL a user ever saw, with no way to learn what a rule
        // actually does or see it work on a real value. <details> surfaces
        // that existing content without cluttering the default (collapsed)
        // view — click a row to see the full explanation + worked example.
        const ruleRow = (r, isOn) => `
        <tr style="border-bottom:1px solid #f0f4ff;transition:background 0.1s;"
            onmouseover="this.style.background='#f8faff'" onmouseout="this.style.background=''">
            <td style="padding:0.45rem 0.55rem;width:36px;text-align:center;vertical-align:top;padding-top:0.55rem;">
                <input type="checkbox" class="cda-assembly-toggle"
                    data-rule-key="${r.key}"
                    ${isOn ? 'checked' : ''}
                    style="accent-color:#1e3a8a;width:14px;height:14px;cursor:pointer;">
            </td>
            <td style="padding:0.45rem 0.55rem;vertical-align:middle;">
                <details>
                    <summary style="cursor:pointer;">
                        <span style="font-weight:${isOn ? '600' : '400'};color:${isOn ? '#1f2937' : '#94a3b8'};font-size:0.82rem;">${r.label}</span>
                        <div style="font-size:0.69rem;color:#f472b6;font-style:italic;margin-top:2px;display:inline-block;">${r.src} → <code style="font-size:0.68rem;color:#1e3a8a;font-style:normal;">${r.fhir}</code></div>
                    </summary>
                    <div style="margin-top:0.5rem;padding:0.55rem 0.65rem;background:#f8fafc;border-radius:5px;border:1px solid #e2e8f0;">
                        <div style="font-size:0.76rem;color:#334155;line-height:1.4;margin-bottom:0.5rem;">${r.desc}</div>
                        <pre style="font-size:0.71rem;color:#1e3a8a;background:#eff6ff;border-radius:4px;padding:0.45rem 0.55rem;white-space:pre-wrap;font-family:monospace;margin:0;line-height:1.5;">${r.example}</pre>
                    </div>
                </details>
            </td>
        </tr>`;

        const group = (title, rulesArr) => `
        <div style="margin-bottom:1rem;">
            <div style="font-size:0.69rem;font-weight:700;text-transform:uppercase;letter-spacing:0.06em;
                        color:#1e3a8a;margin-bottom:0.45rem;
                        border-left:3px solid #f472b6;padding-left:0.45rem;">${title}</div>
            <div style="border:1px solid #dbeafe;border-radius:6px;overflow:hidden;
                        box-shadow:0 1px 3px rgba(30,58,138,0.06);">
                <table style="width:100%;border-collapse:collapse;">
                    <tbody>${rulesArr.map(r => ruleRow(r, on(r.key))).join('')}</tbody>
                </table>
            </div>
        </div>`;

        const poCarTarget = cfg.planOfCareEncounterTarget || '';

        return `
        <div style="font-size:0.79rem;color:#475569;margin-bottom:0.9rem;
                    background:linear-gradient(135deg,#eff6ff 0%,#fdf2f8 100%);
                    border:1px solid #dbeafe;border-radius:6px;padding:0.6rem 0.75rem;">
            Assembly rules control structural transformation decisions during CDA→FHIR conversion.
            <span style="color:#f472b6;font-style:italic;">Disable only if a downstream enrichment step overrides that behaviour.</span>
        </div>
        <div style="margin-bottom:1rem;">
            <div style="font-size:0.69rem;font-weight:700;text-transform:uppercase;letter-spacing:0.06em;
                        color:#1e3a8a;margin-bottom:0.45rem;
                        border-left:3px solid #f472b6;padding-left:0.45rem;">Plan-of-Care Encounter Target</div>
            <div style="border:1px solid #dbeafe;border-radius:6px;padding:0.6rem 0.75rem;
                        box-shadow:0 1px 3px rgba(30,58,138,0.06);">
                <select id="cda2fhirPlanOfCareEncounterTarget" style="width:100%;padding:0.4rem;font-size:0.82rem;border:1px solid #cbd5e1;border-radius:4px;">
                    <option value="" ${poCarTarget === '' ? 'selected' : ''}>Use interface default</option>
                    <option value="Encounter" ${poCarTarget === 'Encounter' ? 'selected' : ''}>Encounter</option>
                    <option value="Appointment" ${poCarTarget === 'Appointment' ? 'selected' : ''}>Appointment</option>
                </select>
                <div style="font-size:0.69rem;color:#64748b;margin-top:0.35rem;">
                    How planned/future visit entries in a Plan-of-Care section map. Overrides this interface's own Settings-tab default for this pipeline only.
                </div>
            </div>
        </div>
        ${group('Data Type Dispatch', DATATYPE_RULES)}
        ${group('Code System OID → URI', OID_RULES)}
        ${group('Structural Wiring', WIRING_RULES)}
        ${group('Status Mapping', STATUS_RULES)}`;
    }

    // ── Advanced tab ──────────────────────────────────────────────────────────

    _renderAdvancedTab(cfg) {
        const mergeOpts = ['append','replace'].map(v =>
            `<option value="${v}" ${cfg.mergeMode === v ? 'selected' : ''}>${v}</option>`
        ).join('');
        const failOpts = ['continue','fail-fast'].map(v =>
            `<option value="${v}" ${cfg.onSectionFailure === v ? 'selected' : ''}>${v}</option>`
        ).join('');
        const logOpts = ['error','warning','info','debug'].map(v =>
            `<option value="${v}" ${cfg.logLevel === v ? 'selected' : ''}>${v}</option>`
        ).join('');

        const lbl = `font-size:0.7rem;font-weight:700;text-transform:uppercase;letter-spacing:0.05em;
                      color:#1e3a8a;display:block;margin-bottom:0.4rem;
                      border-left:3px solid #f472b6;padding-left:0.45rem;`;
        const hint = `font-size:0.71rem;color:#f472b6;font-style:italic;margin-top:0.28rem;`;
        const selectStyle = `width:100%;font-size:0.82rem;
            background:linear-gradient(135deg,#ffffff 0%,#f8fafc 100%);
            border:1px solid #cbd5e1;border-radius:6px;padding:0.38rem 0.55rem;
            box-shadow:inset 0 1px 2px rgba(0,0,0,0.05);color:#1f2937;
            appearance:none;-webkit-appearance:none;
            background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6' viewBox='0 0 10 6'%3E%3Cpath d='M0 0l5 6 5-6z' fill='%231e3a8a'/%3E%3C/svg%3E");
            background-repeat:no-repeat;background-position:right 0.6rem center;background-size:8px;
            padding-right:1.8rem;`;

        return `
        <div class="config-group" style="margin-bottom:1.1rem;">
            <label style="${lbl}">Merge Mode</label>
            <select id="cda2fhirMergeMode" style="${selectStyle}">${mergeOpts}</select>
            <div style="${hint}">When fhirBundle already exists: append adds new entries · replace overwrites the bundle.</div>
        </div>
        <div class="config-group" style="margin-bottom:1.1rem;">
            <label style="${lbl}">On Section Failure</label>
            <select id="cda2fhirOnSectionFail" style="${selectStyle}">${failOpts}</select>
            <div style="${hint}">continue: other sections still process · fail-fast: step fails on first section error.</div>
        </div>
        <div class="config-group" style="margin-bottom:1.1rem;">
            <label style="${lbl}">Log Level</label>
            <select id="cda2fhirLogLevel" style="${selectStyle}">${logOpts}</select>
            <div style="${hint}">Controls verbosity of CDA→FHIR transform logs in the processing result.</div>
        </div>
        <div class="config-group">
            <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-size:0.83rem;color:#1f2937;">
                <input id="cda2fhirProcResult" type="checkbox"
                    ${cfg.includeProcessingResult !== false ? 'checked' : ''}
                    style="accent-color:#1e3a8a;width:14px;height:14px;">
                Include processing result in step output
            </label>
            <div style="${hint.replace('margin-top:0.28rem', 'margin-top:0.25rem;margin-left:1.35rem')}">
                Writes resourcesProduced, failedSections, and sectionErrors to <code style="color:#1e3a8a;font-style:normal;">_stepOutput.processingResult</code>.
            </div>
        </div>`;
    }

    // ── Section field editor (inline overlay) ─────────────────────────────────

    _renderSectionFieldEditor() {
        return `
        <div id="cdaSectionFieldEditor"
            style="display:none;position:relative;margin-top:1rem;border:1px solid #cbd5e1;border-radius:8px;background:#fff;padding:1rem;">
            <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.75rem;">
                <strong id="cdaSectionEditorTitle" style="font-size:0.85rem;color:#1e293b;">Section: —</strong>
                <button type="button"
                    onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.closeSectionEditor()"
                    style="background:transparent;border:none;cursor:pointer;font-size:1rem;color:#6b7280;">×</button>
            </div>
            <div id="cdaSectionEditorContent" style="font-size:0.8rem;color:#6b7280;">Loading fields…</div>
        </div>`;
    }

    // ── Add Field modal ────────────────────────────────────────────────────────
    // Lets a user add a genuinely new field mapping — one with no existing
    // MappingRow to inherit Scope/SourcePath from. See declarative_rules_flatten.go's
    // applyAddOverride for the runtime side of this. Two ways to specify the
    // CDA-side source: browse a real parsed test document when one is
    // available (window.pipelineLastTestOutput), or a structured pattern
    // builder covering the common Scope/SourcePath shapes already used
    // throughout declarative_oob_rules.go. Both converge on the same
    // this._pendingScope / this._pendingSourcePath staging fields.

    _renderAddFieldModal(sectionKey) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        const lbl = `font-size:0.72rem;font-weight:600;color:#475569;display:block;margin-bottom:0.3rem;`;
        const inputStyle = `width:100%;padding:0.35rem 0.55rem;font-size:0.78rem;border:1px solid #cbd5e1;border-radius:5px;box-sizing:border-box;font-family:monospace;`;
        const hint = `font-size:0.68rem;color:#94a3b8;margin-top:0.25rem;`;

        return `
        <div id="cdaAddFieldModal"
            style="display:none;position:fixed;inset:0;background:rgba(0,0,0,0.45);z-index:10100;align-items:center;justify-content:center;">
            <div style="background:white;border-radius:8px;width:720px;max-height:88vh;overflow:auto;padding:1.25rem;box-shadow:0 20px 60px rgba(0,0,0,0.25);">
                <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
                    <strong style="font-size:0.9rem;">Add New Field</strong>
                    <button type="button" onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.closeAddFieldModal()"
                        style="background:transparent;border:none;cursor:pointer;font-size:1.1rem;color:#6b7280;">×</button>
                </div>

                <div id="cdaAddFieldResourceBanner" style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:5px;padding:0.45rem 0.65rem;margin-bottom:0.85rem;font-size:0.76rem;color:#1e40af;"></div>

                <div style="margin-bottom:0.85rem;">
                    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.4rem;">
                        <label style="${lbl}margin-bottom:0;">What to capture</label>
                        <label style="display:flex;align-items:center;gap:0.35rem;font-size:0.72rem;color:#475569;cursor:pointer;">
                            <input type="checkbox" id="cdaAddFieldIsExtension"
                                onchange="window._cdaToFhirBuilder && window._cdaToFhirBuilder.toggleExtensionMode()">
                            This is a FHIR Extension
                        </label>
                    </div>

                    <div id="cdaAddFieldPropertyMode">
                        <div style="position:relative;">
                            <span style="position:absolute;left:0.55rem;top:50%;transform:translateY(-50%);color:#94a3b8;font-size:0.8rem;pointer-events:none;">🔍</span>
                            <input id="cdaAddFieldSearchBox" type="text" autocomplete="off"
                                placeholder="Search by name or clinical description… (e.g. criticality, reaction severity)"
                                class="cda-modern-input"
                                style="${inputStyle.replace('font-family:monospace;', '')}padding-left:1.7rem;"
                                oninput="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onResourceFieldSearchInput(this)"
                                onfocus="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onResourceFieldSearchInput(this)"
                                onblur="window._cdaToFhirBuilder && window._cdaToFhirBuilder.hideResourceFieldSearchSoon()">
                        </div>
                        <div id="cdaAddFieldSearchResults"
                            style="display:none;margin-top:0.35rem;max-height:200px;overflow-y:auto;border:1px solid #e2e8f0;border-radius:5px;"></div>
                        <div style="display:flex;align-items:center;gap:0.5rem;margin-top:0.5rem;">
                            <span style="${hint}margin-top:0;white-space:nowrap;">Selected field:</span>
                            <div style="position:relative;flex:1;">
                                <span style="position:absolute;left:0.55rem;top:50%;transform:translateY(-50%);color:#94a3b8;font-size:0.75rem;pointer-events:none;">📌</span>
                                <input id="cdaAddFieldPath" type="text" placeholder="e.g. onset, note[0]" autocomplete="off"
                                    class="cda-modern-input"
                                    style="${inputStyle}padding-left:1.7rem;"
                                    oninput="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onAddFieldPathManualEdit()">
                            </div>
                        </div>
                        <div id="cdaAddFieldTypeHint" style="${hint}"></div>
                    </div>

                    <div id="cdaAddFieldExtensionMode" style="display:none;">
                        <input id="cdaAddFieldExtensionUrl" type="text" autocomplete="off"
                            placeholder="Extension URL, e.g. http://hl7.org/fhir/us/core/StructureDefinition/us-core-race"
                            style="${inputStyle}margin-bottom:0.4rem;"
                            oninput="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onExtensionFieldsChange()">
                        <select id="cdaAddFieldExtensionType" style="${inputStyle.replace('font-family:monospace;', '')}"
                            onchange="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onExtensionFieldsChange()">
                            <option value="String">string</option>
                            <option value="Code">code</option>
                            <option value="Boolean">boolean</option>
                            <option value="DateTime">dateTime</option>
                            <option value="Quantity">Quantity</option>
                            <option value="CodeableConcept">CodeableConcept</option>
                            <option value="Coding">Coding</option>
                            <option value="Identifier">Identifier</option>
                            <option value="Reference">Reference</option>
                        </select>
                        <div style="${hint}">No catalog of known extensions — you'll need the exact canonical URL. Computes to: <code id="cdaAddFieldExtensionPreview">(enter a URL)</code></div>
                    </div>
                </div>

                <div style="margin-bottom:0.85rem;">
                    <label style="${lbl}">Nest under</label>
                    <select id="cdaAddFieldNestedUnder" style="${inputStyle.replace('font-family:monospace;', '')}"
                        onchange="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onAddFieldNestedUnderChange()">
                        <option value="">— top-level —</option>
                    </select>
                    <div style="${hint}">Only existing repeating groups can be nested under — a new repeating group itself can't be created here.</div>
                    <div id="cdaAddFieldNestWarning" style="display:none;font-size:0.7rem;color:#b45309;background:#fffbeb;border:1px solid #fde68a;border-radius:5px;padding:0.4rem 0.6rem;margin-top:0.4rem;"></div>
                </div>

                <div style="margin-bottom:0.85rem;">
                    <label style="display:flex;align-items:center;gap:0.4rem;font-size:0.78rem;color:#334155;cursor:pointer;">
                        <input type="checkbox" id="cdaAddFieldCollectAll"
                            onchange="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onCollectAllChange()">
                        Capture ALL matching entries as a list (not just one)
                    </label>
                    <div style="${hint}">Use this when the CDA source repeats — e.g. multiple reaction manifestations — and every occurrence should map into the FHIR array, not just one. Top-level fields only (not combinable with "Nest under").</div>
                </div>

                <div id="cdaAddFieldResourcesGroup" style="margin-bottom:0.85rem;display:none;">
                    <label style="${lbl}">Applies to</label>
                    <div id="cdaAddFieldResourceCheckboxes" style="display:flex;flex-wrap:wrap;gap:0.75rem;"></div>
                    <div style="${hint}">This section has more than one FHIR resource variant — pick which one(s) this field applies to.</div>
                </div>

                <div style="margin-bottom:0.5rem;">
                    <label style="${lbl}">CDA Source</label>
                    <div style="display:flex;gap:0.25rem;border-bottom:2px solid #e2e8f0;margin-bottom:0.6rem;">
                        <button type="button" id="cdaAddFieldTabBtn-browse"
                            onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.switchAddFieldSourceTab('browse')"
                            style="padding:0.35rem 0.75rem;border:none;background:none;cursor:pointer;font-size:0.76rem;font-weight:600;
                                   border-bottom:2px solid transparent;margin-bottom:-2px;color:#6b7280;">
                            Browse Test Data
                        </button>
                        <button type="button" id="cdaAddFieldTabBtn-pattern"
                            onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.switchAddFieldSourceTab('pattern')"
                            style="padding:0.35rem 0.75rem;border:none;background:none;cursor:pointer;font-size:0.76rem;font-weight:600;
                                   border-bottom:2px solid transparent;margin-bottom:-2px;color:#6b7280;">
                            Build From Pattern
                        </button>
                        <button type="button" id="cdaAddFieldTabBtn-raw"
                            onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.switchAddFieldSourceTab('raw')"
                            style="padding:0.35rem 0.75rem;border:none;background:none;cursor:pointer;font-size:0.76rem;font-weight:600;
                                   border-bottom:2px solid transparent;margin-bottom:-2px;color:#6b7280;">
                            Advanced: raw path
                        </button>
                    </div>
                    <div id="cdaAddFieldTab-browse"></div>
                    <div id="cdaAddFieldTab-pattern"></div>
                    <div id="cdaAddFieldTab-raw"></div>
                </div>

                <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:5px;padding:0.5rem 0.65rem;margin-bottom:0.85rem;font-size:0.72rem;">
                    <div>CDA Source: <code id="cdaAddFieldPreviewCombined" style="color:#1e40af;word-break:break-all;">(entry root)</code></div>
                    <div style="font-size:0.66rem;color:#94a3b8;margin-top:0.25rem;">
                        Scope: <code id="cdaAddFieldPreviewScope" style="word-break:break-all;">(entry root)</code> ·
                        SourcePath: <code id="cdaAddFieldPreviewSourcePath" style="word-break:break-all;">—</code>
                    </div>
                    <div id="cdaAddFieldPreviewWarning" style="display:none;color:#b45309;margin-top:0.3rem;">⚠ review before saving</div>
                </div>

                <div style="margin-bottom:0.5rem;">
                    <label style="${lbl}">Transform</label>
                    <div style="position:relative;">
                        <span style="position:absolute;left:0.55rem;top:50%;transform:translateY(-50%);color:#94a3b8;font-size:0.75rem;pointer-events:none;">🔧</span>
                        <input id="cdaAddFieldTransform" type="text" placeholder="— none —" autocomplete="off"
                            class="cda-modern-input"
                            style="${inputStyle}padding-left:1.7rem;"
                            onfocus="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onTransformFieldFocus(this)"
                            oninput="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onTransformFieldInput(this)"
                            onblur="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onTransformFieldBlur(this)"
                            onkeydown="window._cdaToFhirBuilder && window._cdaToFhirBuilder.handleAutocompleteKeydown(event)">
                    </div>
                    <div id="cdaAddFieldTransformHint" style="font-size:0.7rem;color:#6b7280;margin-top:0.3rem;min-height:1rem;line-height:1.4;"></div>
                </div>

                <div style="display:flex;justify-content:flex-end;gap:0.5rem;padding-top:0.75rem;border-top:1px solid #e2e8f0;margin-top:0.5rem;">
                    <button type="button" onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.closeAddFieldModal()"
                        style="padding:0.35rem 0.8rem;font-size:0.78rem;background:#f8fafc;border:1px solid #cbd5e1;border-radius:5px;cursor:pointer;color:#475569;">
                        Cancel
                    </button>
                    <button type="button" onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.commitAddField()"
                        style="padding:0.35rem 0.8rem;font-size:0.78rem;background:#1e3a8a;color:#fff;border:none;border-radius:5px;cursor:pointer;">
                        Add Field
                    </button>
                </div>
            </div>
        </div>`;
    }

    openAddFieldModal(sectionKey) {
        const modal = document.getElementById('cdaAddFieldModal');
        if (!modal) return;

        this._addFieldSectionKey = sectionKey;
        this._pendingScope = '';
        this._pendingSourcePath = '';
        this._pendingFhirDataType = null;

        const pathInput = document.getElementById('cdaAddFieldPath');
        const transformInput = document.getElementById('cdaAddFieldTransform');
        if (pathInput) pathInput.value = '';
        if (transformInput) transformInput.value = '';

        const searchBox = document.getElementById('cdaAddFieldSearchBox');
        const searchResults = document.getElementById('cdaAddFieldSearchResults');
        const typeHint = document.getElementById('cdaAddFieldTypeHint');
        if (searchBox) searchBox.value = '';
        if (searchResults) { searchResults.style.display = 'none'; searchResults.innerHTML = ''; }
        if (typeHint) typeHint.textContent = '';
        const nestWarning = document.getElementById('cdaAddFieldNestWarning');
        if (nestWarning) nestWarning.style.display = 'none';

        const collectAllCheckbox = document.getElementById('cdaAddFieldCollectAll');
        if (collectAllCheckbox) collectAllCheckbox.checked = false;
        const nestedUnderSelectForCollectAll = document.getElementById('cdaAddFieldNestedUnder');
        if (nestedUnderSelectForCollectAll) nestedUnderSelectForCollectAll.disabled = false;

        const isExtCheckbox = document.getElementById('cdaAddFieldIsExtension');
        if (isExtCheckbox) isExtCheckbox.checked = false;
        const extUrl = document.getElementById('cdaAddFieldExtensionUrl');
        const extPreview = document.getElementById('cdaAddFieldExtensionPreview');
        if (extUrl) extUrl.value = '';
        if (extPreview) extPreview.textContent = '(enter a URL)';
        this.toggleExtensionMode();

        const rawScope = document.getElementById('cdaAddFieldRawScope');
        const rawSourcePath = document.getElementById('cdaAddFieldRawSourcePath');
        if (rawScope) rawScope.value = '';
        if (rawSourcePath) rawSourcePath.value = '';

        const variants = this._ruleVariantsBySection[sectionKey] || [];

        const banner = document.getElementById('cdaAddFieldResourceBanner');
        if (banner) {
            const resources = [...new Set(variants.map(v => v.fhirResource).filter(Boolean))];
            const sec = (this._sections || []).find(s => s.key === sectionKey);
            const sectionLabel = this._escAttr(sec ? (sec.displayName || sec.key) : sectionKey);
            banner.innerHTML = resources.length
                ? `<strong>${sectionLabel}</strong> section maps to <strong>${resources.map(r => this._escAttr(r)).join('</strong>, <strong>')}</strong>.`
                : `<strong>${sectionLabel}</strong> section — FHIR resource could not be determined automatically.`;
        }

        // "Nest under" options: union of every variant's own nestable groups.
        const nestedSelect = document.getElementById('cdaAddFieldNestedUnder');
        if (nestedSelect) {
            const groups = [...new Set(variants.flatMap(v => v.nestableGroups || []))].sort();
            nestedSelect.innerHTML = '<option value="">— top-level —</option>' +
                groups.map(g => `<option value="${this._escAttr(g)}">${this._escAttr(g)}[]</option>`).join('');
        }

        // "Applies to" checkboxes — hidden entirely when there's only one
        // variant (the common case), since there's nothing to choose between.
        const resGroup = document.getElementById('cdaAddFieldResourcesGroup');
        const resBox = document.getElementById('cdaAddFieldResourceCheckboxes');
        if (resGroup && resBox) {
            if (variants.length > 1) {
                resBox.innerHTML = variants.map((v, i) => `
                    <label style="display:flex;align-items:center;gap:0.3rem;font-size:0.76rem;cursor:pointer;">
                        <input type="checkbox" class="cda-add-field-resource" value="${this._escAttr(v.fhirResource)}" id="cdaAddFieldRes-${i}" checked>
                        ${this._escAttr(v.fhirResource)}${v.entryMatch ? ` <span style="color:#94a3b8;">(${this._escAttr(v.entryMatch)})</span>` : ''}
                    </label>`).join('');
                resGroup.style.display = '';
            } else {
                resBox.innerHTML = '';
                resGroup.style.display = 'none';
            }
        }

        this._renderPatternTab();
        this._renderBrowseTestDataTab(sectionKey);
        this._renderRawPathTab();

        // Default to Browse Test Data when real sample data is available for
        // this section — otherwise Build From Pattern (browse-mode has
        // nothing useful to show without it).
        const hasTestData = !!this._getCDASampleEntries(sectionKey);
        this.switchAddFieldSourceTab(hasTestData ? 'browse' : 'pattern');

        modal.style.display = 'flex';
        if (searchBox) searchBox.focus();
        else if (pathInput) pathInput.focus();
    }

    closeAddFieldModal() {
        const modal = document.getElementById('cdaAddFieldModal');
        if (modal) modal.style.display = 'none';
    }

    _escAttr(s) {
        return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    // ── Step 1: "What to capture" — property search / Extension toggle ────────

    toggleExtensionMode() {
        const isExt = !!document.getElementById('cdaAddFieldIsExtension')?.checked;
        const propMode = document.getElementById('cdaAddFieldPropertyMode');
        const extMode = document.getElementById('cdaAddFieldExtensionMode');
        if (propMode) propMode.style.display = isExt ? 'none' : '';
        if (extMode) extMode.style.display = isExt ? '' : 'none';
        if (isExt) {
            this.onExtensionFieldsChange();
        } else {
            const pathInput = document.getElementById('cdaAddFieldPath');
            if (pathInput) pathInput.value = '';
            this._pendingFhirDataType = null;
        }
    }

    onExtensionFieldsChange() {
        const url = document.getElementById('cdaAddFieldExtensionUrl')?.value.trim() || '';
        const type = document.getElementById('cdaAddFieldExtensionType')?.value || 'String';
        const computed = url ? `extension[url=${url}].value${type}` : '';

        const pathInput = document.getElementById('cdaAddFieldPath');
        if (pathInput) pathInput.value = computed;
        const preview = document.getElementById('cdaAddFieldExtensionPreview');
        if (preview) preview.textContent = computed || '(enter a URL)';

        // Map the select's FHIR value-type suffix to the plain FHIR dataType
        // name InferTransform expects (e.g. "String" -> "string").
        const typeMap = {
            String: 'string', Code: 'code', Boolean: 'boolean', DateTime: 'dateTime',
            Quantity: 'Quantity', CodeableConcept: 'CodeableConcept', Coding: 'Coding',
            Identifier: 'Identifier', Reference: 'Reference',
        };
        this._pendingFhirDataType = typeMap[type] || type;
        this._maybeSuggestTransform(this._guessCDATypeFromSourcePath(this._pendingSourcePath));
    }

    onAddFieldPathManualEdit() {
        // Direct manual edits bypass the search results, so the FHIR data
        // type behind the field is no longer reliably known.
        this._pendingFhirDataType = null;
    }

    async onResourceFieldSearchInput(inputEl) {
        const sectionKey = this._addFieldSectionKey;
        const variants = this._ruleVariantsBySection[sectionKey] || [];
        const resourceType = variants[0]?.fhirResource;
        if (!resourceType) return;

        const fields = await this._loadResourceFields(resourceType);
        const query = (inputEl.value || '').trim().toLowerCase();
        const matches = fields.filter(f =>
            !query ||
            (f.name || '').toLowerCase().includes(query) ||
            (f.description || '').toLowerCase().includes(query) ||
            (f.path || '').toLowerCase().includes(query)
        ).slice(0, 15);

        this._renderResourceFieldResults(matches, resourceType);
    }

    hideResourceFieldSearchSoon() {
        setTimeout(() => {
            const results = document.getElementById('cdaAddFieldSearchResults');
            if (results) results.style.display = 'none';
        }, 150);
    }

    // Fetched once per FHIR resource type and cached for the life of this
    // builder instance — same "fetch once, reuse" idiom as _loadTransformDescriptions.
    async _loadResourceFields(resourceType) {
        this._resourceFieldsByType = this._resourceFieldsByType || {};
        if (this._resourceFieldsByType[resourceType]) return this._resourceFieldsByType[resourceType];
        try {
            const resp = await fetch(`/api/cda/schema/resource-fields/${encodeURIComponent(resourceType)}`);
            if (!resp.ok) return [];
            const data = await resp.json();
            this._resourceFieldsByType[resourceType] = data.elements || [];
            return this._resourceFieldsByType[resourceType];
        } catch (e) {
            return [];
        }
    }

    _renderResourceFieldResults(matches, resourceType) {
        const results = document.getElementById('cdaAddFieldSearchResults');
        if (!results) return;
        if (!matches.length) {
            results.style.display = 'none';
            results.innerHTML = '';
            return;
        }
        const esc = this._escAttr.bind(this);
        results.innerHTML = matches.map((f, i) => {
            const bare = f.path && f.path.startsWith(resourceType + '.') ? f.path.slice(resourceType.length + 1) : (f.path || '');
            const { nestGroup } = this._splitNestedFhirPath(bare);
            const nestBadge = nestGroup
                ? `<span style="font-size:0.63rem;background:#eef2ff;color:#4338ca;border-radius:3px;padding:1px 5px;margin-left:4px;">nested in ${esc(nestGroup)}[]</span>`
                : '';
            return `
            <div class="cda-resource-field-result" data-index="${i}"
                style="padding:0.4rem 0.6rem;border-bottom:1px solid #f1f5f9;cursor:pointer;font-size:0.75rem;">
                <div style="font-weight:600;color:#1e293b;">
                    ${esc(f.name || f.path)}
                    ${f.required ? '<span style="font-size:0.63rem;background:#fee2e2;color:#991b1b;border-radius:3px;padding:1px 4px;margin-left:4px;">required</span>' : ''}
                    ${nestBadge}
                    <span style="font-size:0.65rem;color:#94a3b8;font-weight:400;"> · ${esc(f.dataType || '')}</span>
                </div>
                <div style="color:#6b7280;font-size:0.71rem;">${esc(f.description || '')}</div>
                <div style="color:#94a3b8;font-family:monospace;font-size:0.68rem;">${esc(f.path || '')}</div>
            </div>`;
        }).join('');
        results.style.display = 'block';

        this._currentSearchMatches = matches;
        results.querySelectorAll('.cda-resource-field-result').forEach(el => {
            el.addEventListener('mousedown', (e) => {
                e.preventDefault();
                const idx = parseInt(el.dataset.index, 10);
                this._selectResourceField(this._currentSearchMatches[idx], resourceType);
            });
        });
    }

    // Splits a bare FHIR path like "reaction.severity" into its leading
    // BackboneElement group ("reaction") and the remainder ("severity") —
    // only meaningful when the path has more than one segment. Deliberately
    // naive (first-segment-only): this app's "Nest under" groups are
    // single-level CDA relationship groups (reaction[], practitioner[]), not
    // arbitrary FHIR nesting depth.
    _splitNestedFhirPath(bare) {
        const dot = (bare || '').indexOf('.');
        if (dot === -1) return { nestGroup: null, remainder: bare || '' };
        return { nestGroup: bare.slice(0, dot), remainder: bare.slice(dot + 1) };
    }

    _selectResourceField(field, resourceType) {
        if (!field) return;
        const bare = field.path && field.path.startsWith(resourceType + '.')
            ? field.path.slice(resourceType.length + 1)
            : (field.path || '');

        const pathInput = document.getElementById('cdaAddFieldPath');
        const nestedSelect = document.getElementById('cdaAddFieldNestedUnder');
        const nestWarning = document.getElementById('cdaAddFieldNestWarning');
        const { nestGroup, remainder } = this._splitNestedFhirPath(bare);
        const knownGroup = nestGroup && nestedSelect &&
            Array.from(nestedSelect.options).some(o => o.value === nestGroup);

        if (knownGroup) {
            // The FHIR path implies a repeating group this section already
            // has a matching CDA "Nest under" group for — wire both sides
            // automatically instead of leaving the user to notice and set
            // "Nest under" themselves after the fact.
            nestedSelect.value = nestGroup;
            if (pathInput) pathInput.value = remainder;
            if (nestWarning) nestWarning.style.display = 'none';
            this.onAddFieldNestedUnderChange();
        } else {
            if (pathInput) pathInput.value = bare;
            if (nestWarning) {
                if (nestGroup) {
                    nestWarning.style.display = '';
                    nestWarning.textContent = `⚠ This property is nested under "${nestGroup}[]" on the FHIR side, but this section has no matching CDA group for it — added at top level; verify manually or use Advanced: raw path.`;
                } else {
                    nestWarning.style.display = 'none';
                }
            }
        }

        const searchBox = document.getElementById('cdaAddFieldSearchBox');
        if (searchBox) searchBox.value = field.name || bare;

        this._pendingFhirDataType = field.dataType || null;

        const results = document.getElementById('cdaAddFieldSearchResults');
        if (results) results.style.display = 'none';

        this._maybeSuggestTransform(this._guessCDATypeFromSourcePath(this._pendingSourcePath));
    }

    // ── Transform suggestion (heuristic, editable pre-fill — never a blocking
    // required field; see plan's "no blank CDA data type dropdown" decision) ──

    _guessCDATypeFromSourcePath(sourcePath) {
        const p = (sourcePath || '').toLowerCase();
        if (!p) return null;
        if (p.includes('code.code') || p.endsWith('.code') || p.endsWith('.coding')) return 'CE';
        if (p.includes('time') || p.includes('date')) return 'TS';
        if (p.includes('names[') || p.includes('name.')) return 'PN';
        if (p.includes('telecom') || p.includes('tel:') || p.includes('mailto:')) return 'TEL';
        if (p.includes('addr')) return 'AD';
        if (p.includes('quantity') || p.includes('value.value')) return 'PQ';
        return 'ST';
    }

    // Only fires once BOTH sides of the type pair are known — safe to call
    // from either Step 1 (FHIR type just learned) or Step 2 (CDA type just
    // guessed) regardless of which the user filled in first. Never overwrites
    // a Transform the user already typed by hand.
    async _maybeSuggestTransform(guessedCdaType) {
        const fhirType = this._pendingFhirDataType;
        const hasSourceInfo = !!(this._pendingScope || this._pendingSourcePath);
        const typeHint = document.getElementById('cdaAddFieldTypeHint');
        if (!fhirType) return;
        if (!guessedCdaType || !hasSourceInfo) {
            if (typeHint) typeHint.textContent = fhirType ? `Target type: ${fhirType}` : '';
            return;
        }

        try {
            const resp = await fetch('/api/cda/type-pair/infer', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ cdaDataType: guessedCdaType, fhirDataType: fhirType }),
            });
            if (!resp.ok) {
                if (typeHint) typeHint.textContent = `Target type: ${fhirType}`;
                return;
            }
            const data = await resp.json();
            const transformInput = document.getElementById('cdaAddFieldTransform');
            if (data.inferred && data.transform && transformInput && !transformInput.value.trim()) {
                transformInput.value = data.transform;
                if (typeHint) typeHint.textContent = `Guessed transform: ${data.transform} (based on ${guessedCdaType} → ${fhirType} — adjust if wrong)`;
            } else if (typeHint) {
                typeHint.textContent = `Target type: ${fhirType} (no default transform guessed for ${guessedCdaType} → ${fhirType})`;
            }
        } catch (e) {
            if (typeHint) typeHint.textContent = `Target type: ${fhirType}`;
        }
    }

    onAddFieldNestedUnderChange() {
        // Nested adds can't safely use Browse Test Data in v1 (see
        // switchAddFieldSourceTab's own note) — force Build From Pattern and
        // show the sibling nested field's own CDA source as a starting point.
        const nestedUnder = document.getElementById('cdaAddFieldNestedUnder')?.value || '';
        if (nestedUnder) {
            this.switchAddFieldSourceTab('pattern');
            // Mutually exclusive with "capture all as a list" — every real
            // plain-loop CollectAll OOB row is top-level (declarative_rules_
            // flatten.go's applyAddOverride only wires CollectAll onto
            // top-level adds; nesting a CollectAll row under an existing
            // CollectAll+Fields group is unprecedented in this codebase).
            const collectAllCheckbox = document.getElementById('cdaAddFieldCollectAll');
            if (collectAllCheckbox) collectAllCheckbox.checked = false;
        }
        this._renderPatternTab();
    }

    // "Capture all as a list" only makes sense for a top-level field (see
    // onAddFieldNestedUnderChange's own note) and defeats the purpose of
    // Browse Test Data's leaf disambiguation (narrowing to ONE occurrence is
    // the opposite of "capture every occurrence") — so checking it clears
    // and disables Nest Under, matching the same restriction from the other
    // direction.
    onCollectAllChange() {
        const collectAll = document.getElementById('cdaAddFieldCollectAll')?.checked;
        const nestedSelect = document.getElementById('cdaAddFieldNestedUnder');
        if (nestedSelect) {
            if (collectAll) nestedSelect.value = '';
            nestedSelect.disabled = !!collectAll;
        }
    }

    switchAddFieldSourceTab(tab) {
        const nestedUnder = document.getElementById('cdaAddFieldNestedUnder')?.value || '';
        const browseDisabled = !!nestedUnder;
        if (tab === 'browse' && browseDisabled) tab = 'pattern';

        this._activeAddFieldSourceTab = tab;
        ['browse', 'pattern', 'raw'].forEach(t => {
            const panel = document.getElementById('cdaAddFieldTab-' + t);
            const btn = document.getElementById('cdaAddFieldTabBtn-' + t);
            if (panel) panel.style.display = t === tab ? '' : 'none';
            if (btn) {
                btn.style.color = t === tab ? '#1e3a8a' : '#6b7280';
                btn.style.borderBottomColor = t === tab ? '#1e3a8a' : 'transparent';
                btn.disabled = t === 'browse' && browseDisabled;
                btn.style.opacity = btn.disabled ? '0.45' : '1';
                btn.style.cursor = btn.disabled ? 'not-allowed' : 'pointer';
            }
        });
    }

    // ── Build From Pattern tab ─────────────────────────────────────────────────
    // 4 templates grounded in real rows already in declarative_oob_rules.go —
    // not invented generic examples. Each pattern's sub-inputs recompute
    // _pendingScope/_pendingSourcePath live via oninput/onchange.

    _renderPatternTab() {
        const container = document.getElementById('cdaAddFieldTab-pattern');
        if (!container) return;
        const nestedUnder = document.getElementById('cdaAddFieldNestedUnder')?.value || '';
        const esc = this._escAttr.bind(this);

        let nestedHint = '';
        if (nestedUnder && this._addFieldSectionKey) {
            const fields = this._sectionFields[this._addFieldSectionKey] || [];
            const sibling = fields.find(f => f.nestedUnder === nestedUnder);
            if (sibling) {
                nestedHint = `
                    <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:5px;padding:0.5rem 0.65rem;margin-bottom:0.65rem;font-size:0.71rem;color:#1e40af;">
                        A sibling field already nested under "${esc(nestedUnder)}" has CDA source:
                        <code style="word-break:break-all;">${esc(sibling.cdaSource || '')}</code> — use this as a starting point below (SourcePath is relative to that same matched node).
                    </div>`;
            }
        }

        container.innerHTML = `
            ${nestedHint}
            <select id="cdaAddFieldPattern" style="width:100%;padding:0.35rem 0.55rem;font-size:0.78rem;border:1px solid #cbd5e1;border-radius:5px;margin-bottom:0.6rem;"
                onchange="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onAddFieldPatternChange()">
                <option value="direct">Direct field on this entry</option>
                <option value="related">Related act (by relationship type)</option>
                <option value="participant">Participant field</option>
                <option value="author">Author / performer identity</option>
            </select>
            <div id="cdaAddFieldPatternParams"></div>`;
        this.onAddFieldPatternChange();
    }

    onAddFieldPatternChange() {
        const pattern = document.getElementById('cdaAddFieldPattern')?.value || 'direct';
        const params = document.getElementById('cdaAddFieldPatternParams');
        if (!params) return;

        const fieldStyle = `width:100%;padding:0.3rem 0.5rem;font-size:0.75rem;border:1px solid #cbd5e1;border-radius:4px;margin-bottom:0.4rem;box-sizing:border-box;`;
        const onAny = `window._cdaToFhirBuilder && window._cdaToFhirBuilder._recomputePatternScope()`;

        // typeCode values below are the ones actually used across
        // declarative_oob_rules.go, not an invented generic list.
        if (pattern === 'direct') {
            params.innerHTML = `
                <div style="position:relative;margin-bottom:0.4rem;">
                    <span style="position:absolute;left:0.55rem;top:50%;transform:translateY(-50%);color:#94a3b8;font-size:0.75rem;pointer-events:none;">🔍</span>
                    <input id="cdaPatternDirectSearch" type="text" autocomplete="off"
                        placeholder="Search entry fields by name or description… (e.g. status, dose, route)"
                        class="cda-modern-input"
                        style="width:100%;padding:0.35rem 0.55rem 0.35rem 1.7rem;font-size:0.75rem;border:1px solid #cbd5e1;border-radius:5px;background:#f8fafc;box-sizing:border-box;transition:border-color 0.15s,box-shadow 0.15s;"
                        oninput="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onDirectEntryFieldSearchInput(this)"
                        onfocus="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onDirectEntryFieldSearchInput(this)"
                        onblur="window._cdaToFhirBuilder && window._cdaToFhirBuilder.hideDirectEntryFieldSearchSoon()">
                </div>
                <div id="cdaPatternDirectSearchResults" style="display:none;margin-bottom:0.4rem;max-height:180px;overflow-y:auto;border:1px solid #e2e8f0;border-radius:5px;"></div>
                <input id="cdaPatternSourcePath" type="text" placeholder="e.g. statusCode, effectiveTime, value, text"
                    class="cda-modern-input"
                    style="${fieldStyle}font-family:monospace;" oninput="${onAny}">
                <div style="font-size:0.68rem;color:#94a3b8;">A field directly on the matched entry — no relationship traversal needed. Search above or type the field name directly.</div>`;
        } else if (pattern === 'related') {
            params.innerHTML = `
                <div style="display:flex;gap:0.4rem;align-items:center;margin-bottom:0.4rem;">
                    <select id="cdaPatternTypeCode" style="flex:1;padding:0.3rem 0.5rem;font-size:0.75rem;border:1px solid #cbd5e1;border-radius:4px;" onchange="${onAny}">
                        <option value="SUBJ">SUBJ — Subject/assertion</option>
                        <option value="COMP">COMP — Component</option>
                        <option value="REFR">REFR — Refers-to</option>
                        <option value="MFST">MFST — Manifestation</option>
                        <option value="RSON">RSON — Reason</option>
                    </select>
                    <label style="display:flex;align-items:center;gap:0.25rem;font-size:0.73rem;white-space:nowrap;">
                        <input id="cdaPatternInverted" type="checkbox" onchange="${onAny}"> inverted
                    </label>
                </div>
                <label style="display:flex;align-items:center;gap:0.3rem;font-size:0.72rem;margin-bottom:0.3rem;">
                    <input id="cdaPatternHasFilter" type="checkbox" onchange="window._cdaToFhirBuilder && window._cdaToFhirBuilder._togglePatternFilter()"> filter further by code or templateId
                </label>
                <div id="cdaPatternFilterRow" style="display:none;gap:0.4rem;margin-bottom:0.4rem;">
                    <select id="cdaPatternFilterType" style="padding:0.3rem 0.5rem;font-size:0.75rem;border:1px solid #cbd5e1;border-radius:4px;" onchange="${onAny}">
                        <option value="code">code</option>
                        <option value="templateId">templateId</option>
                    </select>
                    <input id="cdaPatternFilterValue" type="text" placeholder="value" style="flex:1;padding:0.3rem 0.5rem;font-size:0.75rem;border:1px solid #cbd5e1;border-radius:4px;font-family:monospace;" oninput="${onAny}">
                </div>
                <input id="cdaPatternSourcePath" type="text" placeholder="SourcePath relative to the matched act, e.g. value.code"
                    style="${fieldStyle}font-family:monospace;" oninput="${onAny}">
                <div style="font-size:0.68rem;color:#94a3b8;">A field on a related act reached via entryRelationships[typeCode=…].entry — mirrors how Allergy's "type"/"criticality" rows work.</div>`;
        } else if (pattern === 'participant') {
            params.innerHTML = `
                <select id="cdaPatternTypeCode" style="${fieldStyle}" onchange="${onAny}">
                    <option value="CSM">CSM — Consumable/substance</option>
                    <option value="LOC">LOC — Location</option>
                    <option value="COV">COV — Coverage</option>
                </select>
                <input id="cdaPatternSourcePath" type="text" placeholder="e.g. participantRole.playingEntity.code.codeSystem"
                    style="${fieldStyle}font-family:monospace;" oninput="${onAny}">
                <div style="font-size:0.68rem;color:#94a3b8;">A field on a participant of the matched entry — mirrors how Allergy's "category" row works.</div>`;
        } else if (pattern === 'author') {
            params.innerHTML = `
                <select id="cdaPatternAuthorRole" style="${fieldStyle}" onchange="${onAny}">
                    <option value="authors[0].assignedAuthor.assignedPerson.names[0]">Author</option>
                    <option value="performers[0].assignedEntity.assignedPerson.names[0]">Performer</option>
                </select>
                <div style="font-size:0.68rem;color:#94a3b8;">Author only — no performer fallback chain (FallbackPaths isn't part of this schema). Mirrors Medication's requesterReference row.</div>`;
        }

        this._recomputePatternScope();
    }

    // ── "Direct field on this entry" smart search ──────────────────────────────
    // Backed by GET /api/cda/schema/entry-fields — a hand-authored catalog of
    // CDAEntry's own direct fields (statusCode, effectiveTime, value, text,
    // routeCode, doseQuantity, …) with real clinical descriptions, since there
    // is no vendored/generated CDA schema equivalent to FHIR's
    // StructureDefinition files to draw from (see cda_schema_controller.go's
    // GetEntryFields doc comment). Mirrors the Step 1 resource-field search's
    // shape (name/description/dataType) so results render the same way.

    async onDirectEntryFieldSearchInput(inputEl) {
        const fields = await this._loadEntryFields();
        const query = (inputEl.value || '').trim().toLowerCase();
        const matches = fields.filter(f =>
            !query ||
            (f.name || '').toLowerCase().includes(query) ||
            (f.description || '').toLowerCase().includes(query) ||
            (f.path || '').toLowerCase().includes(query)
        ).slice(0, 15);
        this._renderEntryFieldResults(matches);
    }

    hideDirectEntryFieldSearchSoon() {
        setTimeout(() => {
            const results = document.getElementById('cdaPatternDirectSearchResults');
            if (results) results.style.display = 'none';
        }, 150);
    }

    async _loadEntryFields() {
        if (this._entryFieldsCache) return this._entryFieldsCache;
        try {
            const resp = await fetch('/api/cda/schema/entry-fields');
            if (!resp.ok) return [];
            const data = await resp.json();
            this._entryFieldsCache = data.elements || [];
            return this._entryFieldsCache;
        } catch (e) {
            return [];
        }
    }

    _renderEntryFieldResults(matches) {
        const results = document.getElementById('cdaPatternDirectSearchResults');
        if (!results) return;
        if (!matches.length) {
            results.style.display = 'none';
            results.innerHTML = '';
            return;
        }
        const esc = this._escAttr.bind(this);
        results.innerHTML = matches.map((f, i) => `
            <div class="cda-entry-field-result" data-index="${i}"
                style="padding:0.4rem 0.6rem;border-bottom:1px solid #f1f5f9;cursor:pointer;font-size:0.75rem;">
                <div style="font-weight:600;color:#1e293b;">
                    ${esc(f.name || f.path)}
                    <span style="font-size:0.65rem;color:#94a3b8;font-weight:400;"> · ${esc(f.dataType || '')}</span>
                </div>
                <div style="color:#6b7280;font-size:0.71rem;">${esc(f.description || '')}</div>
                <div style="color:#94a3b8;font-family:monospace;font-size:0.68rem;">${esc(f.path || '')}</div>
            </div>`).join('');
        results.style.display = 'block';

        this._currentEntryFieldMatches = matches;
        results.querySelectorAll('.cda-entry-field-result').forEach(el => {
            el.addEventListener('mousedown', (e) => {
                e.preventDefault();
                const idx = parseInt(el.dataset.index, 10);
                this._selectEntryField(this._currentEntryFieldMatches[idx]);
            });
        });
    }

    _selectEntryField(field) {
        if (!field) return;
        const srcInput = document.getElementById('cdaPatternSourcePath');
        if (srcInput) srcInput.value = field.path;
        const searchBox = document.getElementById('cdaPatternDirectSearch');
        if (searchBox) searchBox.value = field.name || field.path;
        const results = document.getElementById('cdaPatternDirectSearchResults');
        if (results) results.style.display = 'none';
        this._recomputePatternScope();
    }

    _togglePatternFilter() {
        const has = document.getElementById('cdaPatternHasFilter')?.checked;
        const row = document.getElementById('cdaPatternFilterRow');
        if (row) row.style.display = has ? 'flex' : 'none';
        this._recomputePatternScope();
    }

    // Rebuilds this._pendingScope/_pendingSourcePath from whichever pattern
    // sub-form is currently shown, and refreshes the preview line.
    _recomputePatternScope() {
        const pattern = document.getElementById('cdaAddFieldPattern')?.value || 'direct';
        let scope = '';
        let sourcePath = '';

        if (pattern === 'direct') {
            sourcePath = document.getElementById('cdaPatternSourcePath')?.value.trim() || '';
        } else if (pattern === 'related') {
            const typeCode = document.getElementById('cdaPatternTypeCode')?.value || 'SUBJ';
            const inverted = document.getElementById('cdaPatternInverted')?.checked;
            const hasFilter = document.getElementById('cdaPatternHasFilter')?.checked;
            let entryExpr = `entryRelationships[typeCode=${typeCode}${inverted ? ',inversionInd=true' : ''}].entry`;
            if (hasFilter) {
                const filterType = document.getElementById('cdaPatternFilterType')?.value || 'code';
                const filterValue = document.getElementById('cdaPatternFilterValue')?.value.trim() || '';
                if (filterValue) entryExpr += `[${filterType}=${filterValue}]`;
            }
            scope = entryExpr;
            sourcePath = document.getElementById('cdaPatternSourcePath')?.value.trim() || '';
        } else if (pattern === 'participant') {
            const typeCode = document.getElementById('cdaPatternTypeCode')?.value || 'CSM';
            scope = `participants[typeCode=${typeCode}]`;
            sourcePath = document.getElementById('cdaPatternSourcePath')?.value.trim() || '';
        } else if (pattern === 'author') {
            sourcePath = document.getElementById('cdaPatternAuthorRole')?.value || '';
        }

        this._pendingScope = scope;
        this._pendingSourcePath = sourcePath;
        this._updateAddFieldPreview(false);
        this._maybeSuggestTransform(this._guessCDATypeFromSourcePath(sourcePath));
    }

    // warning: false (none), true (generic "review before saving"), or a
    // custom string (e.g. the ambiguous-sibling message _onBrowseLeafClick
    // passes) shown verbatim instead of the generic text.
    _updateAddFieldPreview(warning) {
        const combinedEl = document.getElementById('cdaAddFieldPreviewCombined');
        const scopeEl = document.getElementById('cdaAddFieldPreviewScope');
        const pathEl = document.getElementById('cdaAddFieldPreviewSourcePath');
        const warnEl = document.getElementById('cdaAddFieldPreviewWarning');
        // Same combined string _describeAddedSource uses for a saved row's
        // "CDA Source" table column — one canonical format for "where this
        // comes from", not a different one here vs. once it's saved.
        if (combinedEl) combinedEl.textContent = this._describeAddedSource(this._pendingScope, this._pendingSourcePath) || '(entry root)';
        if (scopeEl) scopeEl.textContent = this._pendingScope || '(entry root)';
        if (pathEl) pathEl.textContent = this._pendingSourcePath || '—';
        if (warnEl) {
            warnEl.style.display = warning ? '' : 'none';
            if (warning) {
                warnEl.textContent = typeof warning === 'string' ? warning : '⚠ review before saving';
                // "ℹ"-prefixed messages (e.g. the "capturing all as a list"
                // note) are informational, not a problem — distinguish them
                // from the amber warning styling every other case uses.
                const isInfo = typeof warning === 'string' && warning.startsWith('ℹ');
                warnEl.style.color = isInfo ? '#1e40af' : '#b45309';
                warnEl.style.background = isInfo ? '#eff6ff' : '#fffbeb';
                warnEl.style.border = isInfo ? '1px solid #bfdbfe' : '1px solid #fde68a';
                warnEl.style.borderRadius = '5px';
                warnEl.style.padding = '0.4rem 0.6rem';
            }
        }
    }

    // ── Advanced: raw path tab ─────────────────────────────────────────────────
    // For users who already know this document's CDA structure — free-text
    // Scope + SourcePath, no guidance UI. Unlike "direct" in Build From
    // Pattern (which hardcodes Scope to ""), Scope is genuinely free text here.
    _renderRawPathTab() {
        const container = document.getElementById('cdaAddFieldTab-raw');
        if (!container) return;
        container.innerHTML = `
            <div style="font-size:0.71rem;color:#94a3b8;margin-bottom:0.5rem;">
                For users who already know this document's CDA structure — Scope and SourcePath as free text, no guidance.
            </div>
            <div style="margin-bottom:0.5rem;">
                <label style="font-size:0.72rem;font-weight:600;color:#475569;display:block;margin-bottom:0.2rem;">Scope</label>
                <input id="cdaAddFieldRawScope" type="text" autocomplete="off"
                    placeholder="blank = entry root, e.g. entryRelationships[typeCode=SUBJ].entry"
                    style="width:100%;padding:0.4rem 0.6rem;font-size:0.78rem;font-family:monospace;border:1px solid #cbd5e1;border-radius:5px;box-sizing:border-box;"
                    oninput="window._cdaToFhirBuilder && window._cdaToFhirBuilder._recomputeRawPathScope()">
            </div>
            <div>
                <label style="font-size:0.72rem;font-weight:600;color:#475569;display:block;margin-bottom:0.2rem;">SourcePath</label>
                <input id="cdaAddFieldRawSourcePath" type="text" autocomplete="off"
                    placeholder="e.g. value.code.code"
                    style="width:100%;padding:0.4rem 0.6rem;font-size:0.78rem;font-family:monospace;border:1px solid #cbd5e1;border-radius:5px;box-sizing:border-box;"
                    oninput="window._cdaToFhirBuilder && window._cdaToFhirBuilder._recomputeRawPathScope()">
            </div>`;
    }

    _recomputeRawPathScope() {
        this._pendingScope = document.getElementById('cdaAddFieldRawScope')?.value.trim() || '';
        this._pendingSourcePath = document.getElementById('cdaAddFieldRawSourcePath')?.value.trim() || '';
        this._updateAddFieldPreview(false);
        this._maybeSuggestTransform(this._guessCDATypeFromSourcePath(this._pendingSourcePath));
    }

    // ── Browse Test Data tab ───────────────────────────────────────────────────
    // Only meaningful when a "Test Pipeline" run has already parsed a sample
    // CCD/CCDA document (window.pipelineLastTestOutput). Disabled entirely for
    // nested adds in v1 — resolving a nested parent's own Scope against raw
    // JSON client-side would need a JS port of the Go Phase-1 path resolver,
    // out of scope for this feature; nested adds use Build From Pattern
    // instead (see _renderPatternTab's sibling-source hint).

    // Resolves candidate step names whose test-run output might carry the
    // typed CDA document, in priority order: a real "cda.parse" step first,
    // then (fallback) this pipeline's own "cda.to_fhir" step — which exposes
    // the SAME document itself, test-mode only, when it had to auto-parse
    // because no separate cda.parse step precedes it (see
    // cda_to_fhir_executor.go's auto-parse branch). Cached once per builder
    // instance since the pipeline's own step list doesn't change while this
    // modal is in use.
    _getCDADocumentSourceStepNames() {
        if (this._testDataStepNames !== undefined) return this._testDataStepNames;
        const pipeline = window.pipelineBuilder?.getPipeline ? window.pipelineBuilder.getPipeline() : window.pipelineBuilder?.pipeline;
        let allSteps = [];
        if (pipeline?.getAllSteps) {
            allSteps = pipeline.getAllSteps();
        } else if (pipeline?.executionGroups) {
            pipeline.executionGroups.forEach(g => { if (g.steps) allSteps.push(...g.steps); });
        } else if (Array.isArray(pipeline?.steps)) {
            allSteps = pipeline.steps;
        }
        // A real pipeline's steps are VisualStep instances (PipelineModels.js)
        // with a .stepName property — NOT .name or .step_name, which don't
        // exist on that class at all (.step_name is only the snake_case JSON
        // key BEFORE VisualStep.fromJSON parses it). The fallbacks are kept
        // only for any caller that hands in a plain (non-VisualStep) object.
        const nameOf = s => s.stepName || s.step_name || s.name || null;
        const parseStep = allSteps.find(s => (s.step_type || s.stepType) === 'cda.parse');
        const fhirStep = allSteps.find(s => (s.step_type || s.stepType) === 'cda.to_fhir');
        this._testDataStepNames = [parseStep, fhirStep].filter(Boolean).map(nameOf).filter(Boolean);
        return this._testDataStepNames;
    }

    // Canonicalizes a name/key for matching against the server's own
    // normalized keys — used for BOTH step names (matching
    // window.pipelineLastTestOutput.steps' keys) and CDA section keys
    // (matching a snake_cased cda_document.sectionsByKey's keys). The
    // backend (models.OutputNormalizer.NormalizeKey) snake_cases everything
    // it touches, with underscores sometimes inserted at acronym boundaries
    // (e.g. "Parse CDA" -> "parse_cda", "allergiesAndIntolerances" ->
    // "allergies_and_intolerances") — rather than replicate that algorithm's
    // acronym edge cases exactly, strip everything but lowercase
    // alphanumerics on BOTH sides before comparing. The server's transform
    // only ever lowercases and inserts underscores (never drops/reorders
    // characters), so this canonical form is guaranteed to match regardless
    // of exactly where those underscores land.
    _canonicalKey(s) {
        return String(s || '').toLowerCase().replace(/[^a-z0-9]/g, '');
    }

    // Finds this pipeline's cda.parse (or, as fallback, cda.to_fhir) step's
    // real test-run output, matching by canonicalized name (see
    // _canonicalKey) since comparing the RAW step name directly against
    // the server's normalized keys never matched — this is why Browse Test
    // Data appeared broken even after a real Test Pipeline run with a real
    // cda.parse step. Reads `.step_output` (not `.output`) — the field
    // TransformationTestController.TestPipeline actually sends, confirmed via
    // a real E2E run against the live app (`.output` was a carry-over from
    // misreading a different, unused controller of the same near-identical name).
    _getCDATestStepOutput() {
        const names = this._getCDADocumentSourceStepNames();
        const testOutput = window.pipelineLastTestOutput;
        if (!names.length || !testOutput?.steps) return null;

        const stepKeys = Object.keys(testOutput.steps);
        for (const name of names) {
            const canonName = this._canonicalKey(name);
            const matchedKey = stepKeys.find(k => this._canonicalKey(k) === canonName);
            if (matchedKey) return testOutput.steps[matchedKey].step_output || null;
        }
        return null;
    }

    // Reads from "cdaDocument" (no leading underscore) — the key both
    // cda_parse_executor.go and cda_to_fhir_executor.go write the typed
    // *cdadocument.CDADocument struct under in their `variables` map (the one
    // that actually reaches step_output — see SetStepOutputWithDetails's own
    // doc comment in base_executor.go). "_cdaDocument" (with underscore) is a
    // SEPARATE key those same executors also set, but only on the internal
    // message-passing map used for cross-step consumption within the same
    // pipeline run — it never reaches step_output, so it's never visible here.
    //
    // Verified via a real E2E run against the live app (not assumed): by the
    // time this reaches the client, the ENTIRE document tree — including its
    // own struct fields (sectionsByKey -> sections_by_key,
    // allergiesAndIntolerances -> allergies_and_intolerances, ...) — has been
    // snake_cased too, not just the top-level key. So this matches BOTH the
    // section-key lookup and the "sectionsByKey" container name via the same
    // canonicalized comparison _getCDATestStepOutput uses for step names,
    // rather than assuming one specific casing.
    _getCDASampleEntries(sectionKey) {
        const output = this._getCDATestStepOutput();
        const doc = output?.cda_document;
        const sectionsMap = doc?.sectionsByKey || doc?.sections_by_key;
        if (!sectionsMap) return null;

        const canonTarget = this._canonicalKey(sectionKey);
        const matchedKey = Object.keys(sectionsMap).find(k => this._canonicalKey(k) === canonTarget);
        const entries = matchedKey ? sectionsMap[matchedKey].entries : null;
        return Array.isArray(entries) && entries.length > 0 ? entries : null;
    }

    // A CDA document was parsed by the last Test Pipeline run (regardless of
    // section) — used to distinguish "no test has been run yet" from "a test
    // ran, but THIS section had no entries in that sample document", which
    // otherwise look identical to _getCDASampleEntries (both return null).
    _hasCDATestDocument() {
        const output = this._getCDATestStepOutput();
        const doc = output?.cda_document;
        return !!(doc?.sectionsByKey || doc?.sections_by_key);
    }

    _renderBrowseTestDataTab(sectionKey) {
        const container = document.getElementById('cdaAddFieldTab-browse');
        if (!container) return;
        const entries = this._getCDASampleEntries(sectionKey);
        if (!entries) {
            // Same null result either way, but a very different reason — say
            // which one this is instead of always blaming "haven't tested yet".
            const message = this._hasCDATestDocument()
                ? 'The last Test Pipeline run\'s sample document has no entries for this section — try a sample that includes it, or use Build From Pattern / Advanced instead.'
                : 'Run "Test Pipeline" with a sample document first to browse real field values here.';
            container.innerHTML = `<div style="padding:0.75rem;text-align:center;color:#94a3b8;font-size:0.75rem;">${message}</div>`;
            return;
        }

        this._browseLeaves = this._walkCDAEntryLeafPaths(entries);
        if (this._browseLeaves.length === 0) {
            container.innerHTML = `<div style="padding:0.75rem;text-align:center;color:#94a3b8;font-size:0.75rem;">No fields found in the sampled entries.</div>`;
            return;
        }

        const esc = this._escAttr.bind(this);
        container.innerHTML = `
            <div style="max-height:220px;overflow-y:auto;border:1px solid #e2e8f0;border-radius:5px;">
                ${this._browseLeaves.map((l, i) => `
                    <div class="cda-browse-leaf" data-leaf-index="${i}"
                        style="padding:0.35rem 0.55rem;border-bottom:1px solid #f1f5f9;cursor:pointer;font-size:0.73rem;">
                        <code style="color:#1e40af;word-break:break-all;">${esc(l.rawPath)}</code>
                        <div style="color:#94a3b8;font-size:0.68rem;">${esc(String(l.value).slice(0, 80))}</div>
                    </div>`).join('')}
            </div>`;
        container.querySelectorAll('.cda-browse-leaf').forEach(el => {
            el.addEventListener('mouseover', () => { el.style.background = '#f0f7ff'; });
            el.addEventListener('mouseout', () => { el.style.background = ''; });
            el.addEventListener('click', () => this._onBrowseLeafClick(parseInt(el.dataset.leafIndex, 10)));
        });
    }

    // Samples up to 5 entries (different entries can expose different
    // optional substructures) and walks each to its scalar leaves. Depth/path
    // caps are smaller than StepVariablesProvider's HL7-oriented walker since
    // CDA entry trees are shallower. Each leaf keeps a reference to the
    // SPECIFIC sampled entry it came from, not just its value — needed so
    // _deriveScopeFromRawPath can re-inspect sibling fields at each array
    // index when the user clicks it.
    _walkCDAEntryLeafPaths(entries) {
        const results = [];
        const seen = new Set();
        const sampleCount = Math.min(entries.length, 5);
        const MAX_DEPTH = 6;
        const MAX_PATHS = 300;

        const walk = (node, path, depth, rootEntry) => {
            if (results.length >= MAX_PATHS || depth > MAX_DEPTH || node === null || node === undefined) return;
            if (Array.isArray(node)) {
                node.slice(0, 3).forEach((item, i) => walk(item, `${path}[${i}]`, depth + 1, rootEntry));
                return;
            }
            if (typeof node === 'object') {
                Object.keys(node).forEach(key => walk(node[key], path ? `${path}.${key}` : key, depth + 1, rootEntry));
                return;
            }
            if (node === '' || seen.has(path)) return;
            seen.add(path);
            results.push({ rawPath: path, value: node, entry: rootEntry });
        };

        for (let i = 0; i < sampleCount; i++) {
            walk(entries[i], '', 0, entries[i]);
        }
        return results;
    }

    _onBrowseLeafClick(index) {
        const leaf = (this._browseLeaves || [])[index];
        if (!leaf) return;
        const collectAll = !!document.getElementById('cdaAddFieldCollectAll')?.checked;
        const derived = this._deriveScopeFromRawPath(leaf.rawPath, leaf.entry, collectAll);
        this._pendingScope = derived.scope;
        this._pendingSourcePath = derived.sourcePath;
        this._updateAddFieldPreview(collectAll
            ? 'ℹ Capturing EVERY matching entry as a list — the Scope shown intentionally does not narrow to just this one occurrence.'
            : (derived.ambiguous
                ? '⚠ Multiple entries here share the same relationship type — this will only ever capture the FIRST one, not necessarily the occurrence you clicked. Use Advanced: raw path if you need this specific one, or check "Capture ALL matching entries as a list" above to get every one of them.'
                : derived.needsReview));

        // Suggest the leaf's own last path segment as the field name if the
        // user hasn't already typed one.
        const pathInput = document.getElementById('cdaAddFieldPath');
        if (pathInput && !pathInput.value.trim()) {
            const lastKey = leaf.rawPath.split(/[.[\]]/).filter(Boolean).pop();
            if (lastKey && Number.isNaN(Number(lastKey))) pathInput.value = lastKey;
        }

        // Heuristic CDA-type guess from the sampled leaf's own shape: a bare
        // ISO-date-shaped value suggests TS; a path mentioning "code" suggests
        // CE (a coded value); otherwise a plain scalar (ST). Approximate by
        // design — the user can always override the suggested Transform.
        let guessedCdaType = 'ST';
        if (/^\d{4}-?\d{2}-?\d{2}/.test(String(leaf.value))) {
            guessedCdaType = 'TS';
        } else if (leaf.rawPath.toLowerCase().includes('code')) {
            guessedCdaType = 'CE';
        }
        this._maybeSuggestTransform(guessedCdaType);
    }

    // Rewrites a raw walked path like "entryRelationships[0].entry.value.code.code"
    // (a numeric array index — not valid Scope grammar) into a bracket-predicate
    // form like "entryRelationships[typeCode=SUBJ]" by inspecting the sampled
    // array element for a typeCode/classCode/code sibling. Scope is built as
    // everything up to and INCLUDING the last bracket-indexed segment;
    // SourcePath is everything after. This is a best-effort split, not a
    // guaranteed match for this codebase's own per-relationship-type
    // conventions (e.g. some OOB rows fold a literal ".entry" into Scope
    // right after a relationship bracket, which this can't know to do without
    // hardcoding relationship-specific knowledge) — needsReview is TRUE
    // whenever any bracket was converted, not just when no discriminator was
    // found, so the UI always asks for a sanity check rather than presenting
    // a guess as confidently correct.
    //
    // Real gap this closes: a discriminator alone (e.g. typeCode=MFST) is
    // ambiguous when a SIBLING element in the same array shares it — e.g. two
    // Reaction Observations both linked via typeCode=MFST, one per
    // manifestation. cdaPathResolver.go's engine (services/executors/
    // cda_path_resolver.go) only ever takes the FIRST predicate match for a
    // non-CollectAll row (declarative_engine.go:635), so an ambiguous
    // predicate silently always resolves to the SAME (first) sibling
    // regardless of which occurrence the user actually clicked in the
    // browser — the bug reported as "I see entryRelationship 1st entry only".
    // Fixed with three disambiguation tiers, all using ONLY predicate keys
    // the engine actually supports (services/executors/cda_path_resolver.go's
    // cdaPredicateKeys — not invented syntax), tried in order of how real OOB
    // rows actually use them:
    //   1. elem's own "inversionInd" folded into the SAME bracket as a
    //      compound AND (e.g. "[typeCode=MFST,inversionInd=true]") — a real,
    //      already-used shape (declarative_oob_rules.go has
    //      "typeCode=SUBJ,inversionInd=true").
    //   2. elem.entry.templateIds (CDAEntry's own templateIds — confirmed
    //      against real OOB rows like "entryRelationships[typeCode=SUBJ].
    //      entry[templateId=...]") — the IG's own way of telling apart
    //      DIFFERENT KINDS of entry that happen to share a typeCode (e.g. a
    //      Severity Observation vs. a Reaction Observation, both reached via
    //      typeCode=SUBJ). Doesn't help when two siblings are the SAME
    //      template (e.g. two Reaction Observations, one per manifestation —
    //      those share a templateId by definition), which is exactly when
    //      tier 3 below is what actually distinguishes them.
    //   3. elem.entry.code.code (a fixed clinical code on the nested entry,
    //      e.g. two manifestations' own SNOMED codes) applied to the
    //      immediately-following ".entry" key as its own "[code=X]" bracket,
    //      when unique among the colliding siblings.
    // When NEITHER disambiguates (genuinely identical siblings), `ambiguous`
    // is set so the caller can say so plainly instead of presenting a guess
    // that silently only ever reaches the first occurrence.
    //
    // collectAll: when true, skips all of the above entirely and always
    // returns the plain (undisambiguated) discriminator-only bracket — this
    // is the "capture every match as a list" case (CollectAll on the saved
    // MappingRow), where narrowing to one specific sibling would defeat the
    // whole point. Matches every real plain-loop CollectAll OOB row (e.g.
    // "entryRelationships[typeCode=RSON].entry", CollectAll:true,
    // TargetPath:"reasonCode") — none of them add templateId/code
    // disambiguation, since that would just narrow the list back down to one.
    _deriveScopeFromRawPath(rawPath, entry, collectAll) {
        const segments = [];
        const re = /([^.[\]]+)|\[(\d+)\]/g;
        let match;
        while ((match = re.exec(rawPath)) !== null) {
            if (match[1] !== undefined) segments.push({ key: match[1] });
            else segments.push({ index: parseInt(match[2], 10) });
        }

        let cursor = entry;
        const scopeParts = [];
        const sourceParts = [];
        let scopeEndIdx = -1;
        segments.forEach((seg, i) => { if (seg.index !== undefined) scopeEndIdx = i; });

        let convertedAny = false;
        let ambiguous = false;
        let pendingEntryPredicate = null; // {key, value} applied to the next ".entry" key, see tiers 2/3 above

        for (let i = 0; i < segments.length; i++) {
            const seg = segments[i];
            if (seg.key !== undefined) {
                // A pending tier-2/3 compound predicate MUST land in Scope,
                // not SourcePath: resolveScopeCandidate resolves the ENTIRE
                // Scope string as one multi-segment chain (each bracket
                // narrowing the candidate set further) BEFORE the engine ever
                // takes scopedNodes[0] — only then does SourcePath get
                // evaluated, against that single already-chosen node. Putting
                // the predicate in SourcePath would check it against
                // whichever sibling Scope alone (still ambiguous on typeCode)
                // happened to pick first, silently resolving to nothing
                // whenever that guess was the WRONG sibling — not just
                // imprecise, actually broken. Extending scopeEndIdx here is
                // what makes this segment (and everything up to it) part of
                // Scope regardless of where the original raw path's own
                // brackets ended.
                if (pendingEntryPredicate !== null && seg.key === 'entry') {
                    scopeParts.push(`entry[${pendingEntryPredicate.key}=${pendingEntryPredicate.value}]`);
                    pendingEntryPredicate = null;
                    scopeEndIdx = i;
                    cursor = cursor && typeof cursor === 'object' ? cursor[seg.key] : undefined;
                    continue;
                }
                const inScope = i <= scopeEndIdx;
                (inScope ? scopeParts : sourceParts).push(seg.key);
                cursor = cursor && typeof cursor === 'object' ? cursor[seg.key] : undefined;
                continue;
            }
            // Array index segment — rewrite the immediately preceding key
            // ("entryRelationships[0]" → "entryRelationships[typeCode=SUBJ]").
            const arr = cursor;
            const elem = Array.isArray(arr) ? arr[seg.index] : undefined;
            const discriminatorKey = ['typeCode', 'classCode', 'code'].find(
                k => elem && typeof elem === 'object' && typeof elem[k] === 'string' && elem[k]
            );
            const lastScopePart = scopeParts.pop() || '';
            if (discriminatorKey) {
                const discriminatorValue = elem[discriminatorKey];
                let bracket = `${discriminatorKey}=${discriminatorValue}`;

                const siblings = Array.isArray(arr) ? arr.filter((_, idx) => idx !== seg.index) : [];
                const colliding = siblings.filter(s => s && typeof s === 'object' && s[discriminatorKey] === discriminatorValue);
                if (colliding.length > 0 && !collectAll) {
                    // Tier 1: elem's own inversionInd, if it splits elem from every colliding sibling.
                    const ownInversion = typeof elem.inversionInd === 'boolean' ? elem.inversionInd : null;
                    const collidingInversions = colliding.map(s => (typeof s.inversionInd === 'boolean' ? s.inversionInd : null));
                    if (ownInversion !== null && !collidingInversions.some(v => v === ownInversion)) {
                        bracket += `,inversionInd=${ownInversion}`;
                    } else {
                        // Tier 2: the nested entry's own templateId(s) — tells
                        // apart DIFFERENT KINDS of entry sharing a typeCode
                        // (e.g. Severity vs. Reaction Observation), not
                        // multiple instances of the SAME kind (those share a
                        // templateId, so this legitimately falls through).
                        const ownTemplateIds = Array.isArray(elem?.entry?.templateIds) ? elem.entry.templateIds : [];
                        const collidingTemplateIdSets = colliding.map(s => (Array.isArray(s?.entry?.templateIds) ? s.entry.templateIds : []));
                        const distinguishingTemplateId = ownTemplateIds.find(
                            tid => !collidingTemplateIdSets.some(set => set.includes(tid))
                        );
                        if (distinguishingTemplateId) {
                            pendingEntryPredicate = { key: 'templateId', value: distinguishingTemplateId };
                        } else {
                            // Tier 3: the nested entry's own fixed clinical code.
                            const ownCode = elem?.entry?.code?.code;
                            const collidingCodes = colliding.map(s => s?.entry?.code?.code);
                            if (ownCode && !collidingCodes.includes(ownCode)) {
                                pendingEntryPredicate = { key: 'code', value: ownCode };
                            } else {
                                ambiguous = true;
                            }
                        }
                    }
                }

                scopeParts.push(`${lastScopePart}[${bracket}]`);
            } else {
                scopeParts.push(`${lastScopePart}[${seg.index}]`);
            }
            convertedAny = true;
            cursor = elem;
        }

        return {
            scope: scopeParts.join('.'),
            sourcePath: sourceParts.join('.'),
            needsReview: convertedAny,
            ambiguous,
        };
    }

    // ── Commit / save ──────────────────────────────────────────────────────────

    commitAddField() {
        const sectionKey = this._addFieldSectionKey;
        if (!sectionKey || !this._step) return;

        const pathInput = document.getElementById('cdaAddFieldPath');
        const fhirPathValue = pathInput ? pathInput.value.trim() : '';
        if (!fhirPathValue) {
            alert('FHIR Path / Field Name is required.');
            return;
        }

        const nestedUnder = document.getElementById('cdaAddFieldNestedUnder')?.value || '';
        const transform = document.getElementById('cdaAddFieldTransform')?.value.trim() || '';
        const targetFhirResources = Array.from(document.querySelectorAll('.cda-add-field-resource:checked')).map(cb => cb.value);
        // Top-level only (enforced by onCollectAllChange disabling Nest Under
        // while this is checked) — matches every real plain-loop CollectAll
        // OOB row, all of which are top-level (see applyAddOverride's own
        // handling in declarative_rules_flatten.go).
        const collectAll = !!document.getElementById('cdaAddFieldCollectAll')?.checked;

        // The dictionary key (and this row's DISPLAYED "CDA Field" identity)
        // is dot-prefixed for a nested field, matching how every existing
        // nested row is already keyed (flattenRow: nestedUnder + "." +
        // TargetPath) — but the row's actual TargetPath sent to the backend
        // (and used as MappingRow.TargetPath) stays the BARE name the user
        // typed. Conflating these two would break the flattened-key
        // convention for this one field.
        const cdaField = nestedUnder ? `${nestedUnder}.${fhirPathValue}` : fhirPathValue;

        // Checked against the live DOM, not the cached _sectionFields list —
        // a field added earlier THIS session (via _appendNewFieldRow) is a
        // real row already, but was never written back into _sectionFields,
        // so checking that cache alone would miss an in-session duplicate.
        const existingRow = document.querySelector(`#cdaSectionEditorContent tr.cda-field-row[data-field-key="${CSS.escape(cdaField)}"]`);
        if (existingRow) {
            alert(`A field named "${cdaField}" already exists in this section.`);
            return;
        }

        const overrides = this._step.config.sectionOverrides || {};
        if (!overrides[sectionKey]) overrides[sectionKey] = {};
        if (!overrides[sectionKey].fieldOverrides) overrides[sectionKey].fieldOverrides = {};
        overrides[sectionKey].fieldOverrides[cdaField] = {
            action: 'add',
            fhirPath: fhirPathValue,
            transform,
            scope: this._pendingScope,
            sourcePath: this._pendingSourcePath,
            nestedUnder,
            targetFhirResources,
            collectAll,
        };
        this._step.config.sectionOverrides = overrides;
        this._refreshSectionBadge(sectionKey);

        this._appendNewFieldRow(sectionKey, {
            key: cdaField,
            cdaSource: this._describeAddedSource(this._pendingScope, this._pendingSourcePath),
            fhirPath: fhirPathValue,
            transform,
            nestedUnder,
            isNew: true,
        });

        this.closeAddFieldModal();
    }

    // Mirrors DescribeAddedSource in declarative_rules_flatten.go, so a
    // freshly-added row's CDA Source column reads the same way it will once
    // the backend renders it after a save + reload.
    _describeAddedSource(scope, sourcePath) {
        if (sourcePath) {
            return scope ? `${scope}.${sourcePath}` : sourcePath;
        }
        return scope || '';
    }

    // ── Translation table modal ───────────────────────────────────────────────

    _renderTranslationModal() {
        return `
        <div id="cdaTranslationModal"
            style="display:none;position:fixed;inset:0;background:rgba(0,0,0,0.45);z-index:10100;align-items:center;justify-content:center;">
            <div style="background:white;border-radius:8px;width:660px;max-height:80vh;overflow:auto;padding:1.25rem;box-shadow:0 20px 60px rgba(0,0,0,0.25);">
                <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
                    <strong style="font-size:0.9rem;">Code Translation Table</strong>
                    <button type="button" onclick="document.getElementById('cdaTranslationModal').style.display='none';"
                        style="background:transparent;border:none;cursor:pointer;font-size:1.1rem;color:#6b7280;">×</button>
                </div>
                <div id="cdaTranslationRows" style="margin-bottom:0.75rem;"></div>
                <div style="display:flex;gap:0.5rem;align-items:center;padding-top:0.5rem;border-top:1px solid #e2e8f0;">
                    <input id="cdaTrSrcSystem" type="text" placeholder="Source System (e.g. ICD-9-CM)"
                        style="flex:1;padding:0.3rem 0.6rem;border:1px solid #cbd5e1;border-radius:4px;font-size:0.78rem;">
                    <input id="cdaTrSrcCode" type="text" placeholder="Source Code"
                        style="width:90px;padding:0.3rem 0.6rem;border:1px solid #cbd5e1;border-radius:4px;font-size:0.78rem;">
                    <input id="cdaTrTgtSystem" type="text" placeholder="Target System (e.g. ICD-10-CM)"
                        style="flex:1;padding:0.3rem 0.6rem;border:1px solid #cbd5e1;border-radius:4px;font-size:0.78rem;">
                    <input id="cdaTrTgtCode" type="text" placeholder="Target Code"
                        style="width:90px;padding:0.3rem 0.6rem;border:1px solid #cbd5e1;border-radius:4px;font-size:0.78rem;">
                    <button type="button"
                        onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.addTranslationRow()"
                        style="padding:0.3rem 0.7rem;background:#1e3a8a;color:white;border:none;border-radius:4px;cursor:pointer;font-size:0.78rem;">
                        + Add
                    </button>
                </div>
                <div style="margin-top:0.5rem;font-size:0.7rem;color:#94a3b8;">Translations apply globally unless an interface ID is specified on each row.</div>
            </div>
        </div>`;
    }

    // ── API calls ─────────────────────────────────────────────────────────────

    _loadSections(cfg) {
        const sig = this._ac.signal;
        fetch('/api/cda/schema/sections', { signal: sig })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.sections) return;
                this._sections = data.sections;
                const container = document.getElementById('cdaToFhirTab-sections');
                if (container) {
                    container.innerHTML = this._renderSectionsTab(this._step ? this._step.config : cfg);
                }
            })
            .catch(() => {}); // AbortError on destroy is expected
    }

    _checkOOBVersion(cfg) {
        if (!cfg.basedOnVersion) return; // pure OOB — nothing to compare
        const docType = cfg.documentType && cfg.documentType !== 'auto' ? cfg.documentType : 'CCD';
        const sig = this._ac.signal;
        fetch(`/api/cda/templates/${encodeURIComponent(docType)}/version`, { signal: sig })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.version) return;
                this._oobVersion    = data.version;
                this._oobTemplateId = data.templateId;
                if (data.version !== cfg.basedOnVersion) {
                    this._showUpgradeBanner(cfg.basedOnVersion, data.version);
                }
            })
            .catch(() => {});
    }

    // Fetched once per builder instance and cached — the Section field editor
    // shows these on Transform-input focus (see showTransformDescription)
    // instead of round-tripping to the server on every keystroke/focus.
    _loadTransformDescriptions() {
        const sig = this._ac.signal;
        fetch('/api/cda/transforms', { signal: sig })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.transforms) return;
                this._transformDescriptions = {};
                data.transforms.forEach(t => { this._transformDescriptions[t.name] = t.description; });
            })
            .catch(() => {});
    }

    _showUpgradeBanner(oldVer, newVer) {
        const banner = document.getElementById('cdaToFhirUpgradeBanner');
        const msg    = document.getElementById('cdaToFhirBannerMsg');
        if (!banner || !msg) return;
        msg.textContent = ` OOB template updated to v${newVer}. Your interface uses v${oldVer}.`;
        banner.style.display = '';
    }

    dismissBanner() {
        const banner = document.getElementById('cdaToFhirUpgradeBanner');
        if (banner) banner.style.display = 'none';
    }

    reviewUpgrade() {
        if (this._oobVersion) {
            alert(`OOB template is now v${this._oobVersion}. Open the Sections tab and click "Save Overrides" after reviewing your section field customisations.`);
        }
    }

    applyUpgrade() {
        if (!this._step || !this._oobTemplateId || !this._oobVersion) return;
        const cfg = this._step.config;
        cfg.standardTemplateId = this._oobTemplateId;
        cfg.basedOnVersion     = this._oobVersion;
        // ComputeCDADelta is called server-side via saveDelta()
        this.saveDelta();
        this.dismissBanner();
    }

    // ── Section toggle ─────────────────────────────────────────────────────────

    onSectionToggle(sectionKey, enabled) {
        if (!this._step) return;
        const overrides = this._step.config.sectionOverrides || {};
        if (!overrides[sectionKey]) overrides[sectionKey] = {};
        overrides[sectionKey].enabled = enabled;
        this._step.config.sectionOverrides = overrides;
    }

    // ── Section field editor ──────────────────────────────────────────────────

    openSectionEditor(sectionKey) {
        this._editingSection = sectionKey;
        const editor = document.getElementById('cdaSectionFieldEditor');
        const title  = document.getElementById('cdaSectionEditorTitle');
        const content = document.getElementById('cdaSectionEditorContent');
        if (!editor || !title || !content) return;

        const sec = this._sections.find(s => s.key === sectionKey);
        title.textContent = `Section: ${sec ? (sec.displayName || sec.key) : sectionKey}`;
        content.innerHTML = 'Loading fields…';
        editor.style.display = '';

        // Switch to sections tab to show the editor
        this.switchTab('sections');

        if (this._sectionFields[sectionKey]) {
            this._renderFieldEditor(sectionKey, this._sectionFields[sectionKey], content);
            return;
        }

        const interfaceId = window.pipelineBuilder?.pipeline?.interfaceId;
        const docType = (this._step.config.documentType !== 'auto' && this._step.config.documentType) || 'CCD';
        const params = new URLSearchParams({ documentType: docType });
        if (interfaceId) params.set('interfaceId', interfaceId);

        const sig = this._ac.signal;
        fetch(`/api/cda/schema/sections/${encodeURIComponent(sectionKey)}/fields?${params.toString()}`, { signal: sig })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.fields) {
                    content.innerHTML = '<div style="color:#ef4444;">Failed to load fields.</div>';
                    return;
                }
                this._sectionFields[sectionKey] = data.fields;
                this._ruleVariantsBySection[sectionKey] = data.ruleVariants || [];
                this._renderFieldEditor(sectionKey, data.fields, content);
            })
            .catch(() => {});
    }

    closeSectionEditor() {
        this._editingSection = null;
        const editor = document.getElementById('cdaSectionFieldEditor');
        if (editor) editor.style.display = 'none';
    }

    _renderFieldEditor(sectionKey, fields, container) {
        if (!this._step) return;

        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        // Splits camelCase and strips path punctuation so "verificationStatus" /
        // "reaction.manifestation[0]" become space-separated words the search
        // box can substring-match against clinical terms, not just raw keys.
        const humanize = s => String(s || '')
            .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
            .replace(/[_.[\]]+/g, ' ')
            .toLowerCase()
            .trim();
        const searchText = f => [f.key, f.cdaSource, f.fhirPath, f.transform, f.nestedUnder]
            .map(humanize).join(' ');

        // Autocomplete candidates for the FHIR Path input: every path already
        // used by a sibling row in THIS section — i.e. every value here is
        // guaranteed valid for this section's FHIR resource type, since it's
        // copied straight from a working row, not derived from a separate
        // static schema that could disagree with what the declarative engine
        // actually recognises (the USCDI-based /api/fhir/field-search used
        // elsewhere in the app returns resource-qualified paths like
        // "AllergyIntolerance.clinicalStatus" from a different, older static
        // schema — not the bare "clinicalStatus" TargetPath shape this editor
        // and the engine actually use, so it isn't reused here).
        this._sectionFieldPaths = [...new Set(fields.map(f => f.fhirPath).filter(Boolean))].sort();

        let lastNestedUnder = null;
        const rows = fields.map(f => {
            let caption = '';
            if (f.nestedUnder && f.nestedUnder !== lastNestedUnder) {
                caption = `
                <tr data-caption-group="${esc(f.nestedUnder)}" style="background:#eff6ff;">
                    <td colspan="5" style="padding:0.35rem 0.5rem;font-size:0.71rem;color:#1e3a8a;font-weight:600;letter-spacing:0.02em;">
                        ↳ ${esc(f.nestedUnder)}[] (repeating)
                    </td>
                </tr>`;
            }
            lastNestedUnder = f.nestedUnder || null;
            return caption + this._fieldRowHtml(sectionKey, f);
        }).join('');

        container.innerHTML = `
        <style>
            #cdaSectionFieldEditor table.cda-field-table tbody tr.cda-field-row:nth-of-type(even) { background:#fafbfc; }
            #cdaSectionFieldEditor table.cda-field-table tbody tr.cda-field-row:hover { background:#f0f7ff; }
            #cdaSectionFieldEditor table.cda-field-table input:focus,
            #cdaFieldSearchInput:focus,
            .cda-modern-input:focus {
                border-color:#1e3a8a !important; box-shadow:0 0 0 2px rgba(30,58,138,0.12); outline:none;
            }
            .cda-modern-input { background:#f8fafc; transition:border-color 0.15s,box-shadow 0.15s; }
            .cda-resource-field-result:hover, .cda-entry-field-result:hover { background:#eff6ff; }
            .cda-autocomplete-item:hover, .cda-autocomplete-item.active { background:#eff6ff; }
        </style>
        <div id="cdaFieldAutocompleteDropdown"
            style="display:none;position:fixed;z-index:10150;background:#fff;border:1px solid #cbd5e1;
                   border-radius:6px;box-shadow:0 4px 16px rgba(0,0,0,0.15);max-height:220px;overflow-y:auto;"></div>
        <div style="display:flex;gap:0.5rem;align-items:center;margin-bottom:0.65rem;">
            <div style="position:relative;flex:1;">
                <span style="position:absolute;left:0.6rem;top:50%;transform:translateY(-50%);color:#94a3b8;font-size:0.8rem;pointer-events:none;">🔍</span>
                <input id="cdaFieldSearchInput" type="text"
                    value="${esc(this._fieldSearchQuery)}"
                    placeholder="Search fields by name or clinical term… (e.g. criticality, allergy severity)"
                    oninput="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onFieldSearchInput(this.value)"
                    style="width:100%;padding:0.4rem 2rem 0.4rem 1.9rem;font-size:0.78rem;border:1px solid #cbd5e1;border-radius:6px;
                           background:#f8fafc;color:#1f2937;box-sizing:border-box;transition:border-color 0.15s,box-shadow 0.15s;">
                <button type="button" id="cdaFieldSearchClear"
                    onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.clearFieldSearch()"
                    title="Clear search"
                    style="display:${this._fieldSearchQuery ? '' : 'none'};position:absolute;right:0.5rem;top:50%;transform:translateY(-50%);
                           background:none;border:none;cursor:pointer;color:#94a3b8;font-size:0.95rem;line-height:1;">×</button>
            </div>
            <button type="button" id="cdaAddFieldBtn"
                onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.openAddFieldModal('${esc(sectionKey)}')"
                style="padding:0.4rem 0.8rem;font-size:0.78rem;font-weight:600;background:#1e3a8a;color:#fff;
                       border:none;border-radius:6px;cursor:pointer;white-space:nowrap;">
                + Add Field
            </button>
        </div>
        <div id="cdaFieldSearchEmpty" style="display:none;padding:0.75rem;text-align:center;color:#94a3b8;font-size:0.78rem;"></div>
        ${this._renderAddFieldModal(sectionKey)}
        <div style="overflow-x:auto;border:1px solid #e2e8f0;border-radius:6px;">
            <table class="cda-field-table" style="width:100%;min-width:680px;border-collapse:collapse;font-size:0.78rem;">
                <colgroup>
                    <col style="width:20%;min-width:130px;">
                    <col style="width:24%;min-width:150px;">
                    <col style="width:26%;min-width:170px;">
                    <col style="width:24%;min-width:170px;">
                    <col style="width:6%;min-width:60px;">
                </colgroup>
                <thead>
                    <tr style="background:#f8fafc;border-bottom:1px solid #e2e8f0;">
                        <th style="padding:0.35rem 0.4rem;text-align:left;font-weight:600;color:#475569;">CDA Field</th>
                        <th style="padding:0.35rem 0.4rem;text-align:left;font-weight:600;color:#475569;">CDA Source</th>
                        <th style="padding:0.35rem 0.4rem;text-align:left;font-weight:600;color:#475569;">FHIR Path</th>
                        <th style="padding:0.35rem 0.4rem;text-align:left;font-weight:600;color:#475569;">Transform</th>
                        <th style="padding:0.35rem 0.4rem;"></th>
                    </tr>
                </thead>
                <tbody>${rows}</tbody>
            </table>
        </div>
        <div id="cdaTransformInferHint" style="margin-top:0.4rem;font-size:0.72rem;color:#6b7280;min-height:1rem;"></div>`;

        this._applyFieldSearch(this._fieldSearchQuery);

        // Re-attach every render — container.innerHTML above just replaced
        // the dropdown element, so any previous listener is gone with it.
        // mousedown (not click) fires before the input's blur, so the
        // selection is applied before hideAutocompleteSoon's timeout closes
        // the dropdown out from under it.
        const dropdownEl = document.getElementById('cdaFieldAutocompleteDropdown');
        if (dropdownEl) {
            dropdownEl.addEventListener('mousedown', (e) => {
                e.preventDefault();
                const item = e.target.closest('[data-suggest-value]');
                if (!item || !this._activeAutocompleteCallback) return;
                this._activeAutocompleteCallback(item.dataset.suggestValue);
                dropdownEl.style.display = 'none';
            });
        }
    }

    // Renders ONE field row (no caption — _renderFieldEditor's loop prepends
    // that separately when a group boundary is crossed). Shared by the full
    // table render and _appendNewFieldRow's single-row insertion, so a newly
    // added field renders identically to one loaded from the server.
    _fieldRowHtml(sectionKey, f) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        const humanize = s => String(s || '')
            .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
            .replace(/[_.[\]]+/g, ' ')
            .toLowerCase()
            .trim();
        const searchText = [f.key, f.cdaSource, f.fhirPath, f.transform, f.nestedUnder].map(humanize).join(' ');

        // Badge precedence: disabled > new > modified. A brand-new field
        // can't simultaneously be "modified" — it has no OOB baseline to
        // diverge from — and gets a Remove-only action cell (no Reset:
        // there is no OOB default to revert to).
        const isDisabled = !!f.disabled;
        const isNew = !!f.isNew;
        let badge = '';
        if (isDisabled) {
            badge = `<span style="font-size:0.65rem;background:#fee2e2;color:#991b1b;border:1px solid #fecaca;border-radius:3px;padding:1px 4px;margin-left:4px;">removed</span>`;
        } else if (isNew) {
            badge = `<span style="font-size:0.65rem;background:#dbeafe;color:#1e40af;border:1px solid #bfdbfe;border-radius:3px;padding:1px 4px;margin-left:4px;">new</span>`;
        } else if (f.isModified) {
            badge = `<span style="font-size:0.65rem;background:#fef3c7;color:#92400e;border:1px solid #fde68a;border-radius:3px;padding:1px 4px;margin-left:4px;">modified</span>`;
        }
        const resetBtn = isNew ? '' : this._resetButtonHtml(sectionKey, f.key);
        const removeBtn = isNew
            ? this._newFieldRemoveButtonHtml(sectionKey, f.key)
            : this._removeToggleButtonHtml(sectionKey, f.key, f.conformance, isDisabled);

        const fhirPathRaw  = f.fhirPath  || '';
        const transformRaw = f.transform || '';
        const fhirPath  = esc(fhirPathRaw);
        const transform = esc(transformRaw);
        const indent = f.nestedUnder ? 'padding-left:1.1rem;' : '';
        const rowMuted = isDisabled ? 'opacity:0.55;' : '';
        const nameStrike = isDisabled ? 'text-decoration:line-through;' : '';
        const disabledAttr = isDisabled ? 'disabled' : '';
        return `
            <tr class="cda-field-row" style="border-bottom:1px solid #f1f5f9;vertical-align:top;${rowMuted}"
                data-field-key="${esc(f.key)}" data-nested-under="${esc(f.nestedUnder || '')}"
                data-conformance="${esc(f.conformance || '')}"
                data-search="${esc(searchText)}">
                <td class="cda-field-name-cell" style="padding:0.4rem;font-family:monospace;font-size:0.75rem;color:#334155;${indent}${nameStrike}">
                    ${esc(f.key)}${badge}
                </td>
                <td style="padding:0.4rem;font-size:0.72rem;color:#6b7280;font-style:italic;" title="${esc(f.cdaSource || '')}">${esc(f.cdaSource || '')}</td>
                <td style="padding:0.4rem;">
                    <div style="position:relative;">
                        <span style="position:absolute;left:0.4rem;top:50%;transform:translateY(-50%);color:#94a3b8;font-size:0.7rem;pointer-events:none;">📌</span>
                        <input type="text" class="cda-field-fhir-path form-control form-control-sm cda-modern-input"
                            data-field-key="${esc(f.key)}" data-section-key="${esc(sectionKey)}"
                            value="${fhirPath}" title="${fhirPath}" ${disabledAttr} autocomplete="off"
                            style="width:100%;padding:0.3rem 0.4rem 0.3rem 1.4rem;font-size:0.75rem;font-family:monospace;border:1px solid #cbd5e1;border-radius:5px;box-sizing:border-box;"
                            onfocus="window._cdaToFhirBuilder && window._cdaToFhirBuilder.showPathAutocomplete(this)"
                            oninput="window._cdaToFhirBuilder && window._cdaToFhirBuilder.showPathAutocomplete(this)"
                            onblur="window._cdaToFhirBuilder && window._cdaToFhirBuilder.hideAutocompleteSoon()"
                            onkeydown="window._cdaToFhirBuilder && window._cdaToFhirBuilder.handleAutocompleteKeydown(event)"
                            onchange="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onFieldPathChange('${esc(sectionKey)}','${esc(f.key)}',this.value)">
                    </div>
                </td>
                <td style="padding:0.4rem;">
                    <div style="position:relative;">
                        <span style="position:absolute;left:0.4rem;top:50%;transform:translateY(-50%);color:#94a3b8;font-size:0.7rem;pointer-events:none;">🔧</span>
                        <input type="text" class="cda-field-transform form-control form-control-sm cda-modern-input"
                            data-field-key="${esc(f.key)}" data-section-key="${esc(sectionKey)}"
                            value="${transform}" placeholder="— none —" ${disabledAttr} autocomplete="off"
                            title="${transform || 'No transform — value passed through as-is'}"
                            style="width:100%;padding:0.3rem 0.4rem 0.3rem 1.4rem;font-size:0.75rem;font-family:monospace;border:1px solid #cbd5e1;border-radius:5px;box-sizing:border-box;"
                            onfocus="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onTransformFieldFocus(this)"
                            oninput="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onTransformFieldInput(this)"
                            onblur="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onTransformFieldBlur(this)"
                            onkeydown="window._cdaToFhirBuilder && window._cdaToFhirBuilder.handleAutocompleteKeydown(event)"
                            onchange="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onFieldTransformChange('${esc(sectionKey)}','${esc(f.key)}',this.value)">
                    </div>
                </td>
                <td class="cda-field-action-cell" style="padding:0.4rem;white-space:nowrap;text-align:center;">
                    <div style="display:flex;flex-direction:column;gap:2px;align-items:center;">${resetBtn}${removeBtn}</div>
                </td>
            </tr>`;
    }

    _newFieldRemoveButtonHtml(sectionKey, fieldKey) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        return `<button type="button"
                onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onNewFieldRemove('${esc(sectionKey)}','${esc(fieldKey)}')"
                title="Remove this newly-added field"
                style="padding:0.1rem 0.4rem;font-size:0.68rem;background:#fef2f2;border:1px solid #fecaca;border-radius:3px;cursor:pointer;color:#b91c1c;">
                Remove
            </button>`;
    }

    // Deletes an unsaved "add" override outright and removes its row from the
    // DOM directly — unlike onFieldRemove/onFieldRestore's toggle pair for an
    // OOB field, undoing an add should erase it, not leave a disabled husk
    // with nothing to "restore" back to.
    onNewFieldRemove(sectionKey, fieldKey) {
        if (!this._step) return;
        const overrides = this._step.config.sectionOverrides || {};
        if (overrides[sectionKey] && overrides[sectionKey].fieldOverrides) {
            delete overrides[sectionKey].fieldOverrides[fieldKey];
        }
        this._step.config.sectionOverrides = overrides;
        this._refreshSectionBadge(sectionKey);
        const row = document.querySelector(`#cdaSectionEditorContent tr.cda-field-row[data-field-key="${CSS.escape(fieldKey)}"]`);
        if (row) row.remove();
    }

    // Inserts a single new row into the already-rendered table without a
    // full re-render — the same "appears immediately, saved later" pattern
    // editing an existing row's FHIRPath/Transform already uses. fieldDef
    // needs at minimum {key, fhirPath, transform, nestedUnder, isNew:true}.
    _appendNewFieldRow(sectionKey, fieldDef) {
        const tbody = document.querySelector('#cdaSectionEditorContent table.cda-field-table tbody');
        if (!tbody) return;

        // A parser context of <tbody> is required for a bare "<tr>...</tr>"
        // string to be interpreted as a table row rather than dropped.
        const temp = document.createElement('tbody');
        temp.innerHTML = this._fieldRowHtml(sectionKey, fieldDef).trim();
        const newRow = temp.querySelector('tr.cda-field-row');
        if (!newRow) return;

        let insertAfter = null;
        if (fieldDef.nestedUnder) {
            const groupRows = tbody.querySelectorAll(`tr.cda-field-row[data-nested-under="${CSS.escape(fieldDef.nestedUnder)}"]`);
            if (groupRows.length > 0) insertAfter = groupRows[groupRows.length - 1];
        }
        if (insertAfter) {
            insertAfter.parentNode.insertBefore(newRow, insertAfter.nextSibling);
        } else {
            tbody.appendChild(newRow);
        }

        if (fieldDef.fhirPath && !this._sectionFieldPaths.includes(fieldDef.fhirPath)) {
            this._sectionFieldPaths.push(fieldDef.fhirPath);
            this._sectionFieldPaths.sort();
        }

        // Respect an active search filter instead of always showing the new row.
        this._applyFieldSearch(this._fieldSearchQuery);
    }

    // ── Field search (filters Section field editor rows client-side) ─────────

    onFieldSearchInput(query) {
        this._fieldSearchQuery = query;
        this._applyFieldSearch(query);
    }

    clearFieldSearch() {
        this._fieldSearchQuery = '';
        const input = document.getElementById('cdaFieldSearchInput');
        if (input) { input.value = ''; input.focus(); }
        this._applyFieldSearch('');
    }

    _applyFieldSearch(query) {
        const content = document.getElementById('cdaSectionEditorContent');
        if (!content) return;

        const q = String(query || '').trim().toLowerCase();
        const rows = content.querySelectorAll('tr.cda-field-row');
        const captions = content.querySelectorAll('tr[data-caption-group]');
        const visibleGroups = new Set();
        let anyVisible = false;

        rows.forEach(row => {
            const match = q === '' || (row.dataset.search || '').includes(q);
            row.style.display = match ? '' : 'none';
            if (match) {
                anyVisible = true;
                if (row.dataset.nestedUnder) visibleGroups.add(row.dataset.nestedUnder);
            }
        });

        captions.forEach(cap => {
            cap.style.display = visibleGroups.has(cap.dataset.captionGroup) ? '' : 'none';
        });

        const clearBtn = document.getElementById('cdaFieldSearchClear');
        if (clearBtn) clearBtn.style.display = q ? '' : 'none';

        const emptyEl = document.getElementById('cdaFieldSearchEmpty');
        if (emptyEl) {
            if (q && !anyVisible) {
                emptyEl.textContent = `No fields match "${query.trim()}".`;
                emptyEl.style.display = '';
            } else {
                emptyEl.style.display = 'none';
            }
        }
    }

    // ── Live modified/removed-state toggle (badge + Reset/Remove buttons) ────

    _resetButtonHtml(sectionKey, fieldKey) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        return `<button type="button"
                onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.resetFieldToOOB('${esc(sectionKey)}','${esc(fieldKey)}')"
                title="Reset to OOB default"
                style="padding:0.1rem 0.4rem;font-size:0.68rem;background:#f8fafc;border:1px solid #cbd5e1;border-radius:3px;cursor:pointer;color:#64748b;">
                Reset
            </button>`;
    }

    // Toggles between "Remove" (field maps normally today) and "Restore"
    // (field is currently disabled) — a lighter action than Reset: it only
    // flips the disabled flag, leaving any other FHIRPath/Transform
    // customisation on the field untouched.
    _removeToggleButtonHtml(sectionKey, fieldKey, conformance, isDisabled) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        if (isDisabled) {
            return `<button type="button"
                    onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onFieldRestore('${esc(sectionKey)}','${esc(fieldKey)}')"
                    title="Restore this field's mapping"
                    style="padding:0.1rem 0.4rem;font-size:0.68rem;background:#eff6ff;border:1px solid #bfdbfe;border-radius:3px;cursor:pointer;color:#1e40af;">
                    Restore
                </button>`;
        }
        return `<button type="button"
                onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onFieldRemove('${esc(sectionKey)}','${esc(fieldKey)}','${esc(conformance)}')"
                title="Remove this field's mapping — it will not be written to the FHIR output"
                style="padding:0.1rem 0.4rem;font-size:0.68rem;background:#fef2f2;border:1px solid #fecaca;border-radius:3px;cursor:pointer;color:#b91c1c;">
                Remove
            </button>`;
    }

    // Recomputes a single row's modified/disabled state from the live
    // in-memory sectionOverrides (not the stale flags from the last fetch)
    // and updates its badge, row styling (strikethrough/muted), input
    // disabled state, and Remove/Restore button immediately — so editing
    // doesn't require a full reload to see any of this reflected. Reset
    // itself is always rendered (see _resetButtonHtml) — it's not
    // conditional on any of this.
    _refreshFieldRowState(sectionKey, fieldKey) {
        if (!this._step) return;
        const overrides = this._step.config.sectionOverrides || {};
        const fo = ((overrides[sectionKey] || {}).fieldOverrides || {})[fieldKey];
        const isDisabled = !!(fo && fo.disabled);
        const isModified = !isDisabled && !!(fo && (fo.fhirPath !== undefined || fo.transform !== undefined));

        const row = document.querySelector(`#cdaSectionEditorContent tr.cda-field-row[data-field-key="${CSS.escape(fieldKey)}"]`);
        if (!row) return;

        row.style.opacity = isDisabled ? '0.55' : '';

        const nameCell = row.querySelector('.cda-field-name-cell');
        if (nameCell) {
            nameCell.style.textDecoration = isDisabled ? 'line-through' : '';
            let badge = nameCell.querySelector('span');
            if (isDisabled || isModified) {
                if (!badge) {
                    badge = document.createElement('span');
                    nameCell.appendChild(badge);
                }
                if (isDisabled) {
                    badge.style.cssText = 'font-size:0.65rem;background:#fee2e2;color:#991b1b;border:1px solid #fecaca;border-radius:3px;padding:1px 4px;margin-left:4px;';
                    badge.textContent = 'removed';
                } else {
                    badge.style.cssText = 'font-size:0.65rem;background:#fef3c7;color:#92400e;border:1px solid #fde68a;border-radius:3px;padding:1px 4px;margin-left:4px;';
                    badge.textContent = 'modified';
                }
            } else if (badge) {
                badge.remove();
            }
        }

        row.querySelectorAll('.cda-field-fhir-path, .cda-field-transform').forEach(input => {
            input.disabled = isDisabled;
        });

        const conformance = row.dataset.conformance || '';
        const actionCell = row.querySelector('.cda-field-action-cell > div');
        if (actionCell) {
            actionCell.innerHTML = this._resetButtonHtml(sectionKey, fieldKey) +
                this._removeToggleButtonHtml(sectionKey, fieldKey, conformance, isDisabled);
        }
    }

    // Removing a SHALL (required-by-profile) field can produce a FHIR
    // resource that fails US Core validation, so this confirms before
    // proceeding — Restore/Reset need no such guard since they only ever
    // make the output MORE complete, never less.
    onFieldRemove(sectionKey, fieldKey, conformance) {
        if (!this._step) return;
        if (conformance === 'SHALL') {
            const proceed = confirm(
                `"${fieldKey}" is required (SHALL) by the FHIR profile. Removing it may produce ` +
                `a resource that fails validation. Remove anyway?`
            );
            if (!proceed) return;
        }
        const overrides = this._step.config.sectionOverrides || {};
        if (!overrides[sectionKey]) overrides[sectionKey] = {};
        if (!overrides[sectionKey].fieldOverrides) overrides[sectionKey].fieldOverrides = {};
        if (!overrides[sectionKey].fieldOverrides[fieldKey]) overrides[sectionKey].fieldOverrides[fieldKey] = {};
        overrides[sectionKey].fieldOverrides[fieldKey].disabled = true;
        this._step.config.sectionOverrides = overrides;
        this._refreshSectionBadge(sectionKey);
        this._refreshFieldRowState(sectionKey, fieldKey);
        this.showTransformDescription('');
    }

    // Undoes JUST the removal — any other FHIRPath/Transform override on
    // this field (set before or after it was removed) is left in place.
    // Use Reset instead to revert the field fully back to its OOB default.
    onFieldRestore(sectionKey, fieldKey) {
        if (!this._step) return;
        const overrides = this._step.config.sectionOverrides || {};
        const fo = (overrides[sectionKey] || {}).fieldOverrides || {};
        if (fo[fieldKey]) {
            delete fo[fieldKey].disabled;
            if (Object.keys(fo[fieldKey]).length === 0) delete fo[fieldKey];
        }
        this._step.config.sectionOverrides = overrides;
        this._refreshSectionBadge(sectionKey);
        this._refreshFieldRowState(sectionKey, fieldKey);
    }

    onFieldPathChange(sectionKey, fieldKey, value) {
        if (!this._step) return;
        const overrides = this._step.config.sectionOverrides || {};
        if (!overrides[sectionKey]) overrides[sectionKey] = {};
        if (!overrides[sectionKey].fieldOverrides) overrides[sectionKey].fieldOverrides = {};
        const fo = overrides[sectionKey].fieldOverrides;
        if (value) {
            if (!fo[fieldKey]) fo[fieldKey] = {};
            fo[fieldKey].fhirPath = value;
        } else {
            if (fo[fieldKey]) delete fo[fieldKey].fhirPath;
        }
        this._step.config.sectionOverrides = overrides;
        this._refreshSectionBadge(sectionKey);
        this._refreshFieldRowState(sectionKey, fieldKey);
    }

    onFieldTransformChange(sectionKey, fieldKey, value) {
        if (!this._step) return;
        const overrides = this._step.config.sectionOverrides || {};
        if (!overrides[sectionKey]) overrides[sectionKey] = {};
        if (!overrides[sectionKey].fieldOverrides) overrides[sectionKey].fieldOverrides = {};
        const fo = overrides[sectionKey].fieldOverrides;
        if (value) {
            if (!fo[fieldKey]) fo[fieldKey] = {};
            fo[fieldKey].transform = value;
        } else {
            if (fo[fieldKey]) delete fo[fieldKey].transform;
        }
        this._step.config.sectionOverrides = overrides;
        this._refreshSectionBadge(sectionKey);
        this._refreshFieldRowState(sectionKey, fieldKey);
    }

    // Shows what a named transform actually does when its input gains focus —
    // reuses the descriptions fetched once in _loadTransformDescriptions
    // (no per-focus network round trip). transformName may be empty (no
    // transform set on this field); the hint area is simply cleared then.
    //
    // hintId defaults to the section-table's shared hint div, but the Add
    // Field modal's own Transform input passes its own 'cdaAddFieldTransformHint'
    // instead — the shared div lives in the normal page flow BEHIND the
    // modal's full-viewport backdrop (z-index:10100), so writing into it
    // while the modal is open is invisible even though this function still
    // runs correctly. Each caller knows which input (and therefore which
    // hint) is live, so the fix is to route to the right element rather than
    // to guess visibility here.
    showTransformDescription(transformName, hintId = 'cdaTransformInferHint') {
        const hint = document.getElementById(hintId);
        if (!hint) return;

        if (!transformName) {
            hint.textContent = '';
            return;
        }
        if (this._transformDescriptions === null) {
            hint.textContent = 'Loading transform descriptions…';
            hint.style.color = '#94a3b8';
            return;
        }
        const desc = this._transformDescriptions[transformName];
        if (desc) {
            hint.textContent = `${transformName} — ${desc}`;
            hint.style.color = '#475569';
        } else {
            hint.textContent = `${transformName} (no description available — custom or unregistered transform)`;
            hint.style.color = '#94a3b8';
        }
    }

    // ── FHIR Path / Transform autocomplete (one shared dropdown) ──────────────

    // FHIR Path candidates are this section's own sibling paths (see
    // _sectionFieldPaths' doc comment in _renderFieldEditor) — every
    // suggestion is guaranteed valid for this resource type since it's
    // copied from a real, working row.
    showPathAutocomplete(inputEl) {
        this._showAutocomplete(inputEl, this._sectionFieldPaths, (value) => {
            inputEl.value = value;
            inputEl.dispatchEvent(new Event('change', { bubbles: true }));
            inputEl.focus();
        });
    }

    // Transform candidates are every registered transform name (the same
    // dictionary showTransformDescription already draws from) — combines the
    // description hint with the suggestion dropdown since both react to the
    // same focus/input events on the same field.
    _transformHintIdFor(inputEl) {
        return inputEl && inputEl.id === 'cdaAddFieldTransform' ? 'cdaAddFieldTransformHint' : 'cdaTransformInferHint';
    }

    onTransformFieldFocus(inputEl) {
        this.showTransformDescription(inputEl.value, this._transformHintIdFor(inputEl));
        this._showTransformAutocomplete(inputEl);
    }

    onTransformFieldInput(inputEl) {
        this.showTransformDescription(inputEl.value, this._transformHintIdFor(inputEl));
        this._showTransformAutocomplete(inputEl);
    }

    onTransformFieldBlur(inputEl) {
        this.showTransformDescription('', this._transformHintIdFor(inputEl));
        this.hideAutocompleteSoon();
    }

    _showTransformAutocomplete(inputEl) {
        const hintId = this._transformHintIdFor(inputEl);
        const candidates = this._transformDescriptions ? Object.keys(this._transformDescriptions) : [];
        this._showAutocomplete(inputEl, candidates, (value) => {
            inputEl.value = value;
            this.showTransformDescription(value, hintId);
            inputEl.dispatchEvent(new Event('change', { bubbles: true }));
            inputEl.focus();
        });
    }

    // Renders up to 12 substring matches (query-highlighted) into the one
    // shared dropdown, positioned under inputEl. position:fixed (not
    // absolute) so it escapes the field table's own overflow-x:auto
    // scroll container instead of being clipped by it — same technique
    // FieldPathSearchComponent.js uses for its dropdown.
    _showAutocomplete(inputEl, candidates, onSelect) {
        const dropdown = document.getElementById('cdaFieldAutocompleteDropdown');
        if (!dropdown) return;
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

        const query = (inputEl.value || '').trim().toLowerCase();
        const matches = (candidates || [])
            .filter(c => !query || c.toLowerCase().includes(query))
            .slice(0, 12);

        this._activeAutocompleteCallback = onSelect;

        if (matches.length === 0) {
            dropdown.style.display = 'none';
            return;
        }

        dropdown.innerHTML = matches.map(c => {
            const idx = query ? c.toLowerCase().indexOf(query) : -1;
            const label = idx >= 0
                ? `${esc(c.slice(0, idx))}<mark style="background:#fde68a;border-radius:2px;">${esc(c.slice(idx, idx + query.length))}</mark>${esc(c.slice(idx + query.length))}`
                : esc(c);
            return `<div class="cda-autocomplete-item" data-suggest-value="${esc(c)}"
                        style="padding:0.35rem 0.6rem;font-size:0.75rem;font-family:monospace;cursor:pointer;color:#1e293b;">${label}</div>`;
        }).join('');

        const rect = inputEl.getBoundingClientRect();
        dropdown.style.left = `${rect.left}px`;
        dropdown.style.top = `${rect.bottom + 2}px`;
        dropdown.style.width = `${Math.max(rect.width, 220)}px`;
        dropdown.style.display = 'block';
    }

    // Delayed so a click/mousedown on a dropdown item (which preventDefault's
    // the blur-causing mousedown) has a chance to fire and apply its
    // selection before the dropdown disappears.
    hideAutocompleteSoon() {
        setTimeout(() => {
            const dropdown = document.getElementById('cdaFieldAutocompleteDropdown');
            if (dropdown) dropdown.style.display = 'none';
        }, 150);
    }

    handleAutocompleteKeydown(event) {
        if (event.key === 'Escape') {
            const dropdown = document.getElementById('cdaFieldAutocompleteDropdown');
            if (dropdown) dropdown.style.display = 'none';
        }
    }

    resetFieldToOOB(sectionKey, fieldKey) {
        if (!this._step) return;
        const overrides = this._step.config.sectionOverrides || {};
        if (overrides[sectionKey] && overrides[sectionKey].fieldOverrides) {
            delete overrides[sectionKey].fieldOverrides[fieldKey];
        }
        this._step.config.sectionOverrides = overrides;

        // Invalidate the cached field list — it still has the pre-reset
        // fhirPath/transform baked in from the original fetch. Deleting it
        // forces openSectionEditor's next call to re-fetch true OOB values.
        delete this._sectionFields[sectionKey];
        this.openSectionEditor(sectionKey);
        this._refreshSectionBadge(sectionKey);
    }

    _isSectionModified(sectionKey, overrides) {
        const so = overrides[sectionKey] || {};
        return so.fieldOverrides && Object.keys(so.fieldOverrides).length > 0;
    }

    _refreshSectionBadge(sectionKey) {
        // Re-render just the badge for the section row
        const row = document.querySelector(`#cdaSectionsList tr[data-section-key="${sectionKey}"]`);
        if (!row || !this._step) return;
        const modified = this._isSectionModified(sectionKey, this._step.config.sectionOverrides || {});
        const badgeCell = row.cells[1];
        if (badgeCell) {
            const badge = badgeCell.querySelector('span');
            if (badge && modified) {
                badge.style.background = '#fef3c7';
                badge.style.color      = '#92400e';
                badge.style.border     = '1px solid #fde68a';
                badge.textContent      = 'modified';
            } else if (badge && !modified) {
                badge.style.background = '#f0fdf4';
                badge.style.color      = '#166534';
                badge.style.border     = '1px solid #bbf7d0';
                badge.textContent      = 'OOB';
            }
        }
    }

    // ── Delta save ────────────────────────────────────────────────────────────

    saveDelta() {
        if (!this._step) return;
        const interfaceId = window.pipelineBuilder?.pipeline?.interfaceId;
        const docType     = (this._step.config.documentType !== 'auto' && this._step.config.documentType) || 'CCD';
        if (!interfaceId) {
            console.warn('[CdaToFhirStepBuilder] saveDelta: no interfaceId — skipping.');
            return;
        }

        const overrides = this._step.config.sectionOverrides || {};
        const incoming  = this._buildAtomicMappings(overrides);

        const sig = this._ac.signal;
        fetch(`/api/cda/mappings/${encodeURIComponent(interfaceId)}/${encodeURIComponent(docType)}/compute-delta`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ incoming }),
            signal: sig,
        })
        .then(r => r.ok ? r.json() : null)
        .then(data => {
            if (!data || !data.success) return;
            // Update config with new anchor
            if (data.templateId) this._step.config.standardTemplateId = data.templateId;
            if (data.version)    this._step.config.basedOnVersion    = data.version;
            // Invalidate cached field lists so the NEXT openSectionEditor call
            // re-fetches fresh isModified/fhirPath/transform state from the
            // server instead of replaying a stale pre-save snapshot.
            this._sectionFields = {};
            console.log('[CdaToFhirStepBuilder] Delta saved:', data.overridesStored, 'overrides.');
        })
        .catch(() => {});
    }

    _buildAtomicMappings(sectionOverrides) {
        const result = [];
        for (const [sectionKey, so] of Object.entries(sectionOverrides || {})) {
            const fos = so.fieldOverrides || {};
            for (const [fieldKey, fo] of Object.entries(fos)) {
                result.push({
                    sectionKey,
                    cdaField:  fieldKey,
                    fhirPath:  fo.fhirPath  || '',
                    transform: fo.transform || '',
                    // Explicit per-field signal only — never inferred from a
                    // field being absent from this list (see
                    // CDAAtomicMapping.Disabled's doc comment in
                    // generic_mapper.go for why: incoming here is always
                    // sparse, only fields the user touched in the currently
                    // open section, so "absent" can never safely mean
                    // "removed").
                    disabled: !!fo.disabled,
                    // Only meaningful for a new (Action=="add") field —
                    // harmless empty defaults for every other override.
                    scope: fo.scope || '',
                    sourcePath: fo.sourcePath || '',
                    nestedUnder: fo.nestedUnder || '',
                    targetFhirResources: fo.targetFhirResources || [],
                });
            }
        }
        return result;
    }

    // ── Translation table ─────────────────────────────────────────────────────

    openTranslationModal() {
        const modal = document.getElementById('cdaTranslationModal');
        if (!modal) return;
        modal.style.display = 'flex';
        this._loadTranslations();
    }

    _loadTranslations() {
        const interfaceId = window.pipelineBuilder?.pipeline?.interfaceId;
        const url = interfaceId
            ? `/api/cda/mappings/translations?interfaceId=${encodeURIComponent(interfaceId)}`
            : '/api/cda/mappings/translations';

        const sig = this._ac.signal;
        fetch(url, { signal: sig })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                this._translations = data && data.translations ? data.translations : [];
                this._renderTranslationRows();
            })
            .catch(() => {});
    }

    _renderTranslationRows() {
        const container = document.getElementById('cdaTranslationRows');
        if (!container) return;
        if (!this._translations.length) {
            container.innerHTML = '<div style="font-size:0.78rem;color:#94a3b8;padding:0.5rem 0;">No translation rules yet.</div>';
            return;
        }
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        container.innerHTML = `
        <table style="width:100%;border-collapse:collapse;font-size:0.78rem;margin-bottom:0.5rem;">
            <thead><tr style="background:#f8fafc;border-bottom:1px solid #e2e8f0;">
                <th style="padding:0.3rem 0.4rem;text-align:left;">Source System</th>
                <th style="padding:0.3rem 0.4rem;text-align:left;">Source Code</th>
                <th style="padding:0.3rem 0.4rem;text-align:left;">Target System</th>
                <th style="padding:0.3rem 0.4rem;text-align:left;">Target Code</th>
                <th style="padding:0.3rem 0.4rem;width:40px;"></th>
            </tr></thead>
            <tbody>${this._translations.map((t, i) => `
            <tr style="border-bottom:1px solid #f1f5f9;">
                <td style="padding:0.3rem 0.4rem;font-size:0.75rem;">${esc(t.source_system)}</td>
                <td style="padding:0.3rem 0.4rem;font-family:monospace;">${esc(t.source_code)}</td>
                <td style="padding:0.3rem 0.4rem;font-size:0.75rem;">${esc(t.target_system)}</td>
                <td style="padding:0.3rem 0.4rem;font-family:monospace;">${esc(t.target_code)}</td>
                <td style="padding:0.3rem 0.4rem;text-align:center;">
                    <button type="button"
                        onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.deleteTranslation(${i})"
                        style="background:transparent;border:none;cursor:pointer;color:#ef4444;font-size:0.9rem;">×</button>
                </td>
            </tr>`).join('')}</tbody>
        </table>`;
    }

    addTranslationRow() {
        const srcSys = document.getElementById('cdaTrSrcSystem');
        const srcCode = document.getElementById('cdaTrSrcCode');
        const tgtSys = document.getElementById('cdaTrTgtSystem');
        const tgtCode = document.getElementById('cdaTrTgtCode');

        if (!srcSys || !srcCode || !tgtSys || !tgtCode) return;
        const row = {
            source_system: srcSys.value.trim(),
            source_code:   srcCode.value.trim(),
            target_system: tgtSys.value.trim(),
            target_code:   tgtCode.value.trim(),
        };
        if (!row.source_system || !row.source_code || !row.target_system || !row.target_code) {
            alert('All four fields are required.');
            return;
        }

        const interfaceId = window.pipelineBuilder?.pipeline?.interfaceId || null;
        const body = Object.assign({}, row, interfaceId ? { interface_id: interfaceId } : {});

        const sig = this._ac.signal;
        fetch('/api/cda/mappings/translations', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
            signal: sig,
        })
        .then(r => r.ok ? r.json() : null)
        .then(data => {
            if (!data || !data.success) return;
            srcSys.value = srcCode.value = tgtSys.value = tgtCode.value = '';
            this._loadTranslations();
        })
        .catch(() => {});
    }

    deleteTranslation(index) {
        const t = this._translations[index];
        if (!t || !t.id) return;
        const sig = this._ac.signal;
        fetch(`/api/cda/mappings/translations/${encodeURIComponent(t.id)}`, {
            method: 'DELETE',
            signal: sig,
        })
        .then(() => this._loadTranslations())
        .catch(() => {});
    }
}


// ── FHIRToCDABuilder ──────────────────────────────────────────────────────────

class FHIRToCDABuilder {
    constructor(panel) { this._panel = panel; }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;
        const esc = s => String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');

        const sourceField    = esc(cfg.sourceField  || 'fhirBundle');
        const outputField    = esc(cfg.outputField  || 'cdaXML');
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
                <input id="fhirToCdaSourceField" type="text" class="form-control form-control-sm" value="${sourceField}"
                    placeholder="fhirBundle" style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field containing the FHIR R4 Bundle to serialize.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="fhirToCdaOutputField" type="text" class="form-control form-control-sm" value="${outputField}"
                    placeholder="cdaXML" style="font-family:monospace;font-size:0.82rem;">
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

        const sourceEl   = form.querySelector('#fhirToCdaSourceField');
        const outputEl   = form.querySelector('#fhirToCdaOutputField');
        const templateEl = form.querySelector('#fhirToCdaTemplate');
        const narrativeEl = form.querySelector('#fhirToCdaIncludeNarrative');
        const prettyEl   = form.querySelector('#fhirToCdaPrettyPrint');

        if (sourceEl)   step.config.sourceField      = sourceEl.value.trim()  || 'fhirBundle';
        if (outputEl)   step.config.outputField      = outputEl.value.trim() || 'cdaXML';
        if (templateEl) step.config.template         = templateEl.value;
        if (narrativeEl) step.config.includeNarrative = narrativeEl.checked;
        if (prettyEl)   step.config.prettyPrint      = prettyEl.checked;
    }

    destroy() {}
}


// ── CDANormalizerBuilder ──────────────────────────────────────────────────────

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
                <strong>Pre-parse step.</strong> Converts C32/HITSP template OIDs to C-CDA 2.1 equivalents on raw XML fields.
                Run this <em>before</em> a <code>cda.parse</code> step when the source system emits legacy CDA formats.
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


// ── CDADedupeStepBuilder ───────────────────────────────────────────────────────
// Config: { sourceField, sections, strategy, overrides, crossMessage, patientIdentifierRoot }
//
// Sections + their available identity-matching fields are fetched live from
// GET /api/cda/dedupe/sections (cdaTransform.DedupeSectionCatalog(), backend)
// — never hand-duplicated, so this picker can't drift out of sync with what
// cda.dedupe's Execute actually runs, and it automatically covers every
// section that catalog ever gains in the future. Mirrors
// CDASectionToCSVStepBuilder.js's own async-fetch + expand/collapse pattern.
//
// A user picks which fields make up a section's identity via checkboxes
// (pre-checked per the OOB rule) instead of typing raw CDA paths. There is
// deliberately no free-text "custom section" escape hatch: CDA sections are
// identified by template ID / LOINC code, not invented by a user, and the
// dedupe catalog already covers every section the schema knows. A genuinely
// non-standard section can still be added via the step's raw JSON config
// (config.sections / config.overrides) if ever needed — just not through
// this UI.

const CDA_DEDUPE_LBL = 'font-size:0.7rem;font-weight:700;text-transform:uppercase;letter-spacing:0.05em;' +
    'color:#1e3a8a;display:block;margin-bottom:0.35rem;border-left:3px solid #f472b6;padding-left:0.45rem;';
const CDA_DEDUPE_HINT = 'font-size:0.7rem;color:#94a3b8;margin-top:0.25rem;';

class CDADedupeStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
        this._step = null;

        // [{sectionKey, displayName, loincCode, fields:[{name,path,default}]}]
        // populated async by _loadSections(); empty until then (renders a
        // "Loading…" placeholder), same pattern as CDASectionToCSVStepBuilder.
        this._sections = [];

        // sectionKey → bool, whether the section is enabled at all — mirrors
        // CDASectionToCSVStepBuilder's own _sectionEnabled/_explicitAllEnabled.
        this._sectionEnabled = {};
        this._explicitAllEnabled = true;

        // sectionKey → bool, which sections' field list is expanded.
        this._sectionExpanded = {};

        // sectionKey → Set<path>, the checked identity fields for that
        // section. Seeded once the catalog loads (_initCheckedFields): from
        // step.config.overrides[key].keyPaths if a saved override exists,
        // otherwise from the catalog's own field.default flags.
        this._checkedFields = {};

        // sectionKey → [{name, path}], user-added fields with a CDA path not
        // in the known-fields catalog — the escape hatch for a genuinely
        // custom/proprietary XPath the catalog doesn't know about. Seeded
        // from any saved override path that doesn't match a catalog field
        // (_initCheckedFields), so a previously-added custom field is still
        // visible/editable, not silently dropped, when reopening the step.
        // "name" has nowhere to persist (cdaDedupeOverride.KeyPaths is a bare
        // []string, no name field) — it's a display-only convenience label,
        // defaulting back to the path itself on reload.
        this._customFields = {};

        window._cdaDedupeBuilder = this;
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;
        this._step = step;
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

        const sourceField = esc(cfg.sourceField || '');
        const strategy = cfg.strategy === 'last' ? 'last' : 'first';

        // Empty/absent config.sections means "every section enabled" — matches
        // the executor's own SupportedCDADedupeSections() default.
        const explicit = Array.isArray(cfg.sections) && cfg.sections.length > 0 ? new Set(cfg.sections) : null;
        this._sectionEnabled = {};
        if (explicit) explicit.forEach(k => { this._sectionEnabled[k] = true; });
        this._explicitAllEnabled = !explicit;

        this._loadSections();

        const crossMessage = cfg.crossMessage === true;
        const patientRoot = esc(cfg.patientIdentifierRoot || '');
        // nil/omitted (undefined here, since JSON round-trips absent keys as
        // undefined) means "on" — matches the executor's own default; only an
        // explicit false opts out.
        const trackSuppressionLineage = cfg.trackSuppressionLineage !== false;

        return `
        <div class="cda-step-config">
            <div style="background:#fdf2f8;border:1px solid #fbcfe8;border-left:3px solid #f472b6;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e3a8a;">
                <strong>Dedupe step.</strong> Finds and removes duplicate clinical entries — like the same allergy or medication listed twice — by comparing the fields you check below, not the raw text. Place it after <code>cda.parse</code> (so there's a parsed document to clean up) and before <code>cda.to_fhir</code>/<code>cda.section_to_csv</code> (so they see the cleaned-up result instead of the duplicates).
            </div>

            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="${CDA_DEDUPE_LBL}">Source Field</label>
                <input id="cdaDedupeSourceField" type="text" class="form-control form-control-sm" value="${sourceField}" placeholder="(auto-detect — only used with no upstream cda.parse)"
                    style="width:100%;font-family:monospace;font-size:0.82rem;padding:0.38rem 0.55rem;border:1px solid #cbd5e1;border-radius:6px;">
                <div style="${CDA_DEDUPE_HINT}">Only matters if no earlier cda.parse step ran — ignored otherwise. Pipeline field holding the raw CDA XML to parse directly.</div>
            </div>

            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="${CDA_DEDUPE_LBL}">Strategy</label>
                <select id="cdaDedupeStrategy" class="form-select form-select-sm" style="width:100%;font-size:0.82rem;padding:0.38rem 0.55rem;border:1px solid #cbd5e1;border-radius:6px;">
                    <option value="first" ${strategy === 'first' ? 'selected' : ''}>First — keep the earliest occurrence</option>
                    <option value="last" ${strategy === 'last' ? 'selected' : ''}>Last — keep the most recent occurrence</option>
                </select>
                <div style="${CDA_DEDUPE_HINT}">No "merge" option — see the Documentation tab for why.</div>
            </div>

            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="${CDA_DEDUPE_LBL}">Sections + Identity Fields</label>
                <div style="font-size:0.78rem;color:#64748b;margin-bottom:0.55rem;">
                    Uncheck a section to skip deduping it. Click a section's name to see and change which fields determine a duplicate — a match requires every checked field to be equal, not just one.
                </div>
                <div id="cdaDedupeSectionsList">${this._renderSectionsList()}</div>
            </div>

            <div class="config-group" style="border-top:1px solid #e2e8f0;padding-top:0.8rem;">
                <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-size:0.83rem;color:#1f2937;">
                    <input id="cdaDedupeCrossMessage" type="checkbox" ${crossMessage ? 'checked' : ''}
                        style="accent-color:#1e3a8a;width:14px;height:14px;"
                        onchange="window._cdaDedupeBuilder.onCrossMessageToggle(this.checked)">
                    Also dedupe across separate documents for the same patient
                </label>
                <div id="cdaDedupePatientRootWrapper" style="margin-top:0.6rem;margin-left:1.35rem;${crossMessage ? '' : 'display:none;'}">
                    <label style="${CDA_DEDUPE_LBL}">Patient Identifier Root (OID)</label>
                    <input id="cdaDedupePatientRoot" type="text" value="${patientRoot}" placeholder="e.g. 2.16.840.1.113883.19"
                        style="font-family:monospace;font-size:0.82rem;width:100%;padding:0.38rem 0.55rem;border:1px solid #cbd5e1;border-radius:6px;">
                    <div style="${CDA_DEDUPE_HINT}">Which of the patient's CDA &lt;id&gt; roots identifies them for cross-message matching, scoped to this interface. Required — there's no automatic detection.</div>

                    <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;font-size:0.83rem;color:#1f2937;margin-top:0.75rem;">
                        <input id="cdaDedupeTrackLineage" type="checkbox" ${trackSuppressionLineage ? 'checked' : ''}
                            style="accent-color:#1e3a8a;width:14px;height:14px;">
                        Track suppression lineage <span style="font-size:0.72rem;color:#94a3b8;font-weight:400;">(recommended)</span>
                    </label>
                    <div style="${CDA_DEDUPE_HINT}">When a fact is suppressed, records exactly which fact and which earlier message first delivered it (visible on that message's detail page), not just a count. Turn off only to minimize what per-entry detail gets captured — suppression itself is unaffected either way.</div>
                </div>
            </div>
        </div>`;
    }

    // ── data loading ──────────────────────────────────────────────────────

    _loadSections() {
        const sig = this._ac.signal;
        fetch('/api/cda/dedupe/sections', { signal: sig })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.sections) return;
                this._sections = data.sections;
                this._initCheckedFields();
                const list = document.getElementById('cdaDedupeSectionsList');
                if (list) list.innerHTML = this._renderSectionsList();
            })
            .catch(() => {}); // AbortError on destroy is expected
    }

    // Seeds this._checkedFields for every section once the catalog is
    // available: a saved override wins outright (exact CDA paths the user
    // picked before), otherwise the catalog's own OOB defaults apply. Any
    // saved path that doesn't match a catalog field is a previously-added
    // custom field — synthesized back into this._customFields so it's still
    // visible/editable instead of silently vanishing from the UI.
    _initCheckedFields() {
        const overrides = (this._step && this._step.config && this._step.config.overrides) || {};
        this._sections.forEach(sec => {
            const key = sec.sectionKey;
            const override = overrides[key];
            if (override && Array.isArray(override.keyPaths) && override.keyPaths.length > 0) {
                this._checkedFields[key] = new Set(override.keyPaths);
                const catalogPaths = new Set(sec.fields.map(f => f.path));
                const customPaths = override.keyPaths.filter(p => !catalogPaths.has(p));
                if (customPaths.length > 0) {
                    this._customFields[key] = customPaths.map(p => ({ name: p, path: p }));
                }
            } else {
                this._checkedFields[key] = new Set(sec.fields.filter(f => f.default).map(f => f.path));
            }
        });
    }

    // ── state helpers ─────────────────────────────────────────────────────

    _isSectionEnabled(key) {
        if (Object.prototype.hasOwnProperty.call(this._sectionEnabled, key)) {
            return this._sectionEnabled[key];
        }
        return this._explicitAllEnabled !== false; // default: enabled
    }

    setSectionEnabled(key, checked) {
        this._sectionEnabled[key] = checked;
    }

    toggleField(sectionKey, path, checked) {
        const set = this._checkedFields[sectionKey];
        if (!set) return;
        if (checked) set.add(path); else set.delete(path);
        this._refreshSectionRow(sectionKey);
    }

    // Adds a genuinely custom CDA path not in the known-fields catalog — the
    // escape hatch for a proprietary/vendor-specific element the OOB catalog
    // doesn't know about. Checked by default: adding a field only makes sense
    // if you want it to count toward the identity match.
    addCustomField(sectionKey) {
        const form = document.querySelector('.properties-form') || document;
        const nameEl = form.querySelector(`#cdaDedupeCustomFieldName-${sectionKey}`);
        const pathEl = form.querySelector(`#cdaDedupeCustomFieldPath-${sectionKey}`);
        const name = (nameEl?.value || '').trim();
        const path = (pathEl?.value || '').trim();
        if (!path) { alert('A CDA path is required.'); return; }

        if (!this._customFields[sectionKey]) this._customFields[sectionKey] = [];
        if (this._customFields[sectionKey].some(f => f.path === path)) { alert('That path is already added.'); return; }
        this._customFields[sectionKey].push({ name: name || path, path });

        if (!this._checkedFields[sectionKey]) this._checkedFields[sectionKey] = new Set();
        this._checkedFields[sectionKey].add(path);

        this._refreshSectionRow(sectionKey);
    }

    removeCustomField(sectionKey, path) {
        const list = this._customFields[sectionKey];
        if (list) this._customFields[sectionKey] = list.filter(f => f.path !== path);
        const set = this._checkedFields[sectionKey];
        if (set) set.delete(path);
        this._refreshSectionRow(sectionKey);
    }

    // ── Sections list ─────────────────────────────────────────────────────

    _renderSectionsList() {
        if (!this._sections.length) {
            return `<div style="text-align:center;padding:1.5rem;color:#6b7280;font-size:0.8rem;">Loading sections…</div>`;
        }
        return this._sections.map(sec => this._renderSectionRow(sec)).join('');
    }

    _renderSectionRow(sec) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        const enabled = this._isSectionEnabled(sec.sectionKey);
        const expanded = !!this._sectionExpanded[sec.sectionKey];
        const checked = this._checkedFields[sec.sectionKey] || new Set();
        const key = sec.sectionKey;
        const defaultPaths = sec.fields.filter(f => f.default).map(f => f.path);
        const isDefault = defaultPaths.length === checked.size && defaultPaths.every(p => checked.has(p));
        // A section with zero checked fields is never actually deduplicated —
        // the executor skips any section with no OOB rule and no override —
        // so that case needs its own distinct (and slightly warning-toned)
        // badge rather than "OOB default", which checked.size===defaultPaths.length===0
        // would otherwise satisfy vacuously (e.g. a section like
        // admissionMedications that has no OOB identity rule at all).
        const badge = checked.size === 0
            ? `<span style="font-size:0.65rem;background:#fee2e2;color:#b91c1c;border-radius:3px;padding:1px 5px;white-space:nowrap;">no fields — skipped</span>`
            : isDefault
                ? `<span style="font-size:0.65rem;background:#f1f5f9;color:#64748b;border-radius:3px;padding:1px 5px;white-space:nowrap;">OOB default</span>`
                : `<span style="font-size:0.65rem;background:#fef3c7;color:#92400e;border-radius:3px;padding:1px 5px;white-space:nowrap;">${checked.size}/${sec.fields.length} custom</span>`;
        return `
        <div id="cdaDedupeSectionRow-${key}" style="border:1px solid #e2e8f0;border-radius:6px;margin-bottom:0.4rem;overflow:hidden;">
            <div id="cdaDedupeSectionHeader-${key}" style="display:flex;align-items:center;gap:0.5rem;padding:0.45rem 0.6rem;background:${expanded ? '#fdf2f8' : '#fff'};cursor:pointer;"
                onclick="window._cdaDedupeBuilder && window._cdaDedupeBuilder.toggleSection('${key}')">
                <input type="checkbox" class="cda-dedupe-section-checkbox" data-section-key="${key}"
                    ${enabled ? 'checked' : ''}
                    onclick="event.stopPropagation()"
                    onchange="window._cdaDedupeBuilder && window._cdaDedupeBuilder.setSectionEnabled('${key}', this.checked)"
                    style="accent-color:#1e3a8a;width:14px;height:14px;flex-shrink:0;">
                <span style="font-weight:${enabled ? '500' : '400'};color:${enabled ? '#1e293b' : '#94a3b8'};flex:1;">${esc(sec.displayName || key)}</span>
                ${badge}
                <span id="cdaDedupeSectionCaret-${key}" style="color:#94a3b8;">${expanded ? '▾' : '▸'}</span>
            </div>
            <div id="cdaDedupeSectionDetail-${key}" style="display:${expanded ? 'block' : 'none'};border-top:1px solid #e2e8f0;padding:0.65rem 0.75rem;background:#fafbfc;">
                ${expanded ? this._renderSectionDetail(sec) : ''}
            </div>
        </div>`;
    }

    toggleSection(key) {
        const sec = this._sections.find(s => s.sectionKey === key);
        if (!sec) return;
        this._sectionExpanded[key] = !this._sectionExpanded[key];
        const detail = document.getElementById('cdaDedupeSectionDetail-' + key);
        const caret  = document.getElementById('cdaDedupeSectionCaret-' + key);
        if (!detail) return;
        if (this._sectionExpanded[key]) {
            detail.innerHTML = this._renderSectionDetail(sec);
            detail.style.display = 'block';
            if (caret) caret.textContent = '▾';
        } else {
            detail.style.display = 'none';
            detail.innerHTML = '';
            if (caret) caret.textContent = '▸';
        }
        const header = document.getElementById('cdaDedupeSectionHeader-' + key);
        if (header) header.style.background = this._sectionExpanded[key] ? '#fdf2f8' : '#fff';
    }

    _refreshSectionRow(key) {
        const sec = this._sections.find(s => s.sectionKey === key);
        if (!sec) return;
        // Replace the WHOLE row (header + detail) — the "N/M custom" badge
        // lives in the header, outside the detail area that changed.
        const row = document.getElementById('cdaDedupeSectionRow-' + key);
        if (row) row.outerHTML = this._renderSectionRow(sec);
    }

    // ── Section detail: identity-field checkboxes ────────────────────────

    _renderSectionDetail(sec) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        const escAttr = s => esc(s).replace(/'/g, '&#39;');
        const checked = this._checkedFields[sec.sectionKey] || new Set();
        const rows = sec.fields.map(f => `
            <tr style="border-bottom:1px solid #f1f5f9;">
                <td style="padding:0.35rem 0.5rem;width:28px;text-align:center;">
                    <input type="checkbox" class="cda-dedupe-field-checkbox" data-section-key="${sec.sectionKey}" data-path="${esc(f.path)}"
                        ${checked.has(f.path) ? 'checked' : ''}
                        onchange="window._cdaDedupeBuilder && window._cdaDedupeBuilder.toggleField('${sec.sectionKey}', '${escAttr(f.path)}', this.checked)"
                        style="accent-color:#1e3a8a;width:14px;height:14px;">
                </td>
                <td style="padding:0.35rem 0.5rem;font-weight:500;color:#1e293b;white-space:nowrap;">${esc(f.name)}${f.default ? ' <span style="font-size:0.62rem;color:#94a3b8;">(OOB)</span>' : ''}</td>
                <td style="padding:0.35rem 0.5rem;font-family:monospace;font-size:0.72rem;color:#475569;word-break:break-all;">${esc(f.path)}</td>
                <td></td>
            </tr>`).join('');

        const customFields = this._customFields[sec.sectionKey] || [];
        const customRows = customFields.map(f => `
            <tr style="border-bottom:1px solid #f1f5f9;background:#fffbeb;">
                <td style="padding:0.35rem 0.5rem;width:28px;text-align:center;">
                    <input type="checkbox" class="cda-dedupe-field-checkbox" data-section-key="${sec.sectionKey}" data-path="${esc(f.path)}"
                        ${checked.has(f.path) ? 'checked' : ''}
                        onchange="window._cdaDedupeBuilder && window._cdaDedupeBuilder.toggleField('${sec.sectionKey}', '${escAttr(f.path)}', this.checked)"
                        style="accent-color:#1e3a8a;width:14px;height:14px;">
                </td>
                <td style="padding:0.35rem 0.5rem;font-weight:500;color:#92400e;white-space:nowrap;">${esc(f.name)} <span style="font-size:0.62rem;background:#fef3c7;color:#92400e;border-radius:3px;padding:0 4px;">custom</span></td>
                <td style="padding:0.35rem 0.5rem;font-family:monospace;font-size:0.72rem;color:#92400e;word-break:break-all;">${esc(f.path)}</td>
                <td style="padding:0.35rem 0.5rem;text-align:right;white-space:nowrap;">
                    <button type="button" onclick="window._cdaDedupeBuilder && window._cdaDedupeBuilder.removeCustomField('${sec.sectionKey}', '${escAttr(f.path)}')"
                        style="padding:0.1rem 0.45rem;font-size:0.68rem;background:#fff;border:1px solid #fca5a5;color:#b91c1c;border-radius:4px;cursor:pointer;">Remove</button>
                </td>
            </tr>`).join('');

        return `
        <div style="overflow:auto;max-height:280px;border:1px solid #e2e8f0;border-radius:6px;margin-bottom:0.5rem;">
            <table style="width:100%;border-collapse:collapse;">
                <thead>
                    <tr style="background:#f8fafc;">
                        <th style="padding:0.35rem 0.5rem;"></th>
                        <th style="text-align:left;padding:0.35rem 0.5rem;font-size:0.68rem;text-transform:uppercase;color:#64748b;">Field</th>
                        <th style="text-align:left;padding:0.35rem 0.5rem;font-size:0.68rem;text-transform:uppercase;color:#64748b;">CDA Path</th>
                        <th style="padding:0.35rem 0.5rem;"></th>
                    </tr>
                </thead>
                <tbody>${rows}${customRows}</tbody>
            </table>
        </div>
        <div style="font-size:0.72rem;color:#94a3b8;margin-bottom:0.6rem;">Two entries are duplicates only if they match on <strong>every</strong> checked field. Unchecking a field broadens the match (more aggressive); checking more fields narrows it (more precise).</div>

        <div style="display:flex;gap:0.4rem;align-items:center;">
            <input id="cdaDedupeCustomFieldName-${sec.sectionKey}" type="text" placeholder="Field name (optional)"
                style="flex:1;font-size:0.78rem;padding:0.25rem 0.4rem;border:1px solid #e2e8f0;border-radius:4px;">
            <input id="cdaDedupeCustomFieldPath-${sec.sectionKey}" type="text" placeholder="CDA path not in the list above, e.g. a vendor-specific extension"
                style="flex:2;font-family:monospace;font-size:0.76rem;padding:0.25rem 0.4rem;border:1px solid #e2e8f0;border-radius:4px;">
            <button type="button" onclick="window._cdaDedupeBuilder && window._cdaDedupeBuilder.addCustomField('${sec.sectionKey}')"
                style="padding:0.25rem 0.7rem;font-size:0.75rem;background:#1e3a8a;color:white;border:none;border-radius:4px;cursor:pointer;white-space:nowrap;">Add field</button>
        </div>
        <div style="font-size:0.7rem;color:#94a3b8;margin-top:0.25rem;">Don't see the field you need? Add its CDA path directly — for a proprietary or vendor-specific element the standard catalog above doesn't cover.</div>`;
    }

    onCrossMessageToggle(checked) {
        const form = document.querySelector('.properties-form') || document;
        const wrapper = form.querySelector('#cdaDedupePatientRootWrapper');
        if (wrapper) wrapper.style.display = checked ? '' : 'none';
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const sourceEl = form.querySelector('#cdaDedupeSourceField');
        const strategyEl = form.querySelector('#cdaDedupeStrategy');
        const crossMessageEl = form.querySelector('#cdaDedupeCrossMessage');
        const patientRootEl = form.querySelector('#cdaDedupePatientRoot');
        const trackLineageEl = form.querySelector('#cdaDedupeTrackLineage');

        if (sourceEl) step.config.sourceField = sourceEl.value.trim();
        if (strategyEl) step.config.strategy = strategyEl.value;
        step.config.crossMessage = crossMessageEl ? crossMessageEl.checked : false;
        if (patientRootEl) step.config.patientIdentifierRoot = patientRootEl.value.trim();
        // Only emit an explicit false when unchecked — omit the key entirely
        // when checked, matching the executor's nil-means-on default and
        // keeping saved config minimal/diffable (same convention already used
        // for section overrides above).
        if (trackLineageEl && !trackLineageEl.checked) {
            step.config.trackSuppressionLineage = false;
        } else {
            delete step.config.trackSuppressionLineage;
        }

        // Guard against collecting before the async catalog fetch resolves —
        // an empty this._sections would otherwise wipe out a previously saved
        // sections/overrides config (mirrors CDASectionToCSVStepBuilder's
        // identical guard around its own async-loaded state).
        if (this._sections.length === 0) return;

        const sections = [];
        const overrides = {};
        this._sections.forEach(sec => {
            const key = sec.sectionKey;
            if (this._isSectionEnabled(key)) sections.push(key);

            const checked = this._checkedFields[key];
            if (!checked || checked.size === 0) return;
            const defaultPaths = sec.fields.filter(f => f.default).map(f => f.path);
            const isDefault = defaultPaths.length === checked.size && defaultPaths.every(p => checked.has(p));
            if (!isDefault) {
                // Read straight from the checked Set, not filtered through
                // sec.fields — a checked custom-field path (added via
                // addCustomField) has no entry in sec.fields and would
                // otherwise be silently dropped here.
                overrides[key] = { keyPaths: Array.from(checked) };
            }
        });

        step.config.sections = sections;
        step.config.overrides = overrides;
    }

    destroy() {
        this._ac.abort();
        if (window._cdaDedupeBuilder === this) delete window._cdaDedupeBuilder;
    }
}


// ── CDABuildStepBuilder ────────────────────────────────────────────────────────
// Config: { sourceField, inputFormat, outputField, documentType, orgName, custodian }
// Format-agnostic successor to fhir.to_cda — see cda_build_executor.go.
// Document type list mirrors CdaToFhirStepBuilder's docTypeOpts above
// (same fixed set from ccda_2_1.json's documentTypeSections keys).
//
// Tabbed layout (General / Requirements / Custodian) follows
// CdaToFhirStepBuilder's established tab pattern exactly (_renderTabButton,
// switchTab, per-tab display:none toggling) rather than inventing a new
// visual language for a second CDA step builder in this same file.
//
// The Requirements tab reads this._panel.builder.pipeline.steps (already
// available in-memory pipeline state — the panel is constructor-injected
// exactly like every other builder) via CDARequirementsHelper.
// findMapToCanonicalStep to show a REAL "N of M required items configured"
// checklist against a sibling cda.map_to_canonical step's live config, not
// just a static SHALL/SHOULD/MAY reference list.

class CDABuildStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
        this._step = null;
        this._activeTab = 'general';
        this._requirements = null;
        this._requirementsDocType = null;
        window._cdaBuildBuilder = this;
    }

    render(step) {
        this._step = step;
        if (!step.config) step.config = {};
        if (!step.config.documentType) step.config.documentType = 'CCD';
        if (!step.config.custodian) step.config.custodian = {};
        if (!step.config.legalAuthenticator) step.config.legalAuthenticator = {};

        this._loadRequirements(step.config.documentType);

        return `
<div id="cdaBuildBuilder" style="font-size:0.84rem;">
    <div style="display:flex;gap:0;background:#1e3a8a;border-radius:8px 8px 0 0;margin-bottom:1.1rem;padding:0 0.25rem;">
        ${this._renderTabButton('general', 'General')}
        ${this._renderTabButton('requirements', 'Requirements')}
        ${this._renderTabButton('custodian', 'Custodian')}
        ${this._renderTabButton('legalAuthenticator', 'Legal Authenticator')}
    </div>
    <div id="cdaBuildTab-general" style="${this._activeTab === 'general' ? '' : 'display:none'}">
        ${this._renderGeneralTab(step.config)}
    </div>
    <div id="cdaBuildTab-requirements" style="${this._activeTab === 'requirements' ? '' : 'display:none'}">
        ${this._renderRequirementsTab(step.config)}
    </div>
    <div id="cdaBuildTab-custodian" style="${this._activeTab === 'custodian' ? '' : 'display:none'}">
        ${this._renderCustodianTab(step.config)}
    </div>
    <div id="cdaBuildTab-legalAuthenticator" style="${this._activeTab === 'legalAuthenticator' ? '' : 'display:none'}">
        ${this._renderLegalAuthenticatorTab(step.config)}
    </div>
</div>`;
    }

    // ── Tab navigation ────────────────────────────────────────────────────────

    switchTab(tabName) {
        this._activeTab = tabName;
        ['general', 'requirements', 'custodian', 'legalAuthenticator'].forEach(t => {
            const panel = document.getElementById('cdaBuildTab-' + t);
            const btn = document.getElementById('cdaBuildTabBtn-' + t);
            if (panel) panel.style.display = (t === tabName) ? '' : 'none';
            if (btn) {
                if (t === tabName) {
                    btn.style.borderBottom = '3px solid #f9a8d4';
                    btn.style.background = 'rgba(255,255,255,0.12)';
                    btn.style.color = '#ffffff';
                    btn.style.fontWeight = '700';
                } else {
                    btn.style.borderBottom = '3px solid transparent';
                    btn.style.background = 'transparent';
                    btn.style.color = 'rgba(255,255,255,0.6)';
                    btn.style.fontWeight = '400';
                }
            }
        });
    }

    _renderTabButton(key, label) {
        const active = this._activeTab === key;
        return `<button id="cdaBuildTabBtn-${key}" type="button"
            onclick="window._cdaBuildBuilder && window._cdaBuildBuilder.switchTab('${key}')"
            style="padding:0.45rem 0.95rem;border:none;
                   border-bottom:3px solid ${active ? '#f9a8d4' : 'transparent'};
                   background:${active ? 'rgba(255,255,255,0.12)' : 'transparent'};
                   cursor:pointer;font-size:0.78rem;
                   color:${active ? '#ffffff' : 'rgba(255,255,255,0.6)'};
                   font-weight:${active ? '700' : '400'};">${label}</button>`;
    }

    // ── General tab (unchanged fields, moved into its own tab) ───────────────

    _renderGeneralTab(cfg) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

        const sourceField  = esc(cfg.sourceField  || 'parsedCDA');
        const inputFormat  = cfg.inputFormat || 'canonical';
        const outputField  = esc(cfg.outputField  || 'cdaXML');
        const documentType = cfg.documentType || 'CCD';
        const orgName      = esc(cfg.orgName || '');
        const timezoneOffset = esc(cfg.timezoneOffset || '');

        const formatOptions = [
            { value: 'canonical',   label: 'Canonical JSON (from cda.parse or cda.map_to_canonical)' },
            { value: 'fhir_bundle', label: 'FHIR R4 Bundle' },
        ].map(o => `<option value="${o.value}" ${inputFormat === o.value ? 'selected' : ''}>${esc(o.label)}</option>`).join('');

        const docTypeOptions = [
            'CCD', 'Discharge Summary', 'Referral Note',
            'History and Physical', 'Consultation Note', 'Progress Note',
            'Care Plan', 'Transfer Summary', 'Diagnostic Imaging Report',
            'Operative Note', 'Procedure Note', 'Unstructured Document',
        ].map(dt => `<option value="${esc(dt)}" ${documentType === dt ? 'selected' : ''}>${esc(dt)}</option>`).join('');

        return `
        <div class="cda-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Build step.</strong> Converts canonical JSON or a FHIR Bundle into a full C-CDA 2.1 document,
                covering every SHALL and SHOULD section for the selected Document Type — not just Allergies/Medications/Problems/Immunizations.
                See the Requirements tab for what this Document Type needs, and the Custodian tab to configure the sending organization.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Input Format</label>
                <select id="cdaBuildInputFormat" class="form-select form-select-sm">${formatOptions}</select>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Canonical JSON uses the same USCDI-keyed shape cda.parse produces.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="cdaBuildSourceField" type="text" class="form-control form-control-sm" value="${sourceField}"
                    placeholder="parsedCDA" style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field containing the source data to build from.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="cdaBuildOutputField" type="text" class="form-control form-control-sm" value="${outputField}"
                    placeholder="cdaXML" style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the generated C-CDA 2.1 XML will be written.</div>
            </div>
            <div class="config-group">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Document Type</label>
                <select id="cdaBuildDocType" class="form-select form-select-sm"
                    onchange="window._cdaBuildBuilder && window._cdaBuildBuilder.onDocTypeChange(this.value)">${docTypeOptions}</select>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Determines which sections are SHALL/SHOULD and the document-level LOINC code/title — see the Requirements tab.</div>
            </div>
            <div class="config-group" style="margin-top:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Timezone Offset</label>
                <input id="cdaBuildTimezoneOffset" type="text" class="form-control form-control-sm" value="${timezoneOffset}"
                    placeholder="+0000 or -0500 — blank = UTC" style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Applied to the document's own effectiveTime and the author's time — both are generated at build time, so this controls which offset "now" is expressed in. Leave blank for UTC.</div>
            </div>
        </div>`;
    }

    onDocTypeChange(newType) {
        this._collectGeneralAndCustodianTabs();
        this._step.config.documentType = newType;
        this._requirements = null;
        this._requirementsDocType = null;
        this._loadRequirements(newType);
        this._rerender();
    }

    // ── Requirements tab ──────────────────────────────────────────────────────

    _loadRequirements(documentType) {
        if (!documentType || this._requirementsDocType === documentType) return;
        fetch(`/api/cda/document-types/${encodeURIComponent(documentType)}/requirements`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.requirements) return;
                this._requirements = data.requirements;
                this._requirementsDocType = documentType;
                this._rerender();
            })
            .catch(() => {}); // AbortError on destroy is expected
    }

    _renderRequirementsTab(cfg) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        if (!this._requirements) {
            return `<div style="padding:1rem;color:#94a3b8;font-size:0.8rem;">Loading requirements for ${esc(cfg.documentType || 'CCD')}…</div>`;
        }

        const sourceStep = (typeof CDARequirementsHelper !== 'undefined') ? CDARequirementsHelper.findMapToCanonicalStep(this._panel) : null;
        const badge = c => (typeof CDARequirementsHelper !== 'undefined') ? CDARequirementsHelper.renderConformanceBadge(c) : c;
        const uscdiSummary = (typeof CDARequirementsHelper !== 'undefined') ? CDARequirementsHelper.renderUSCDISummary(this._requirements) : '';

        if (!sourceStep) {
            // No live cda.map_to_canonical sibling to score against (source is
            // likely cda.parse, whose completeness depends on the runtime
            // document, not static config) — show the static reference list.
            const sectionRows = this._requirements.sections.map(s =>
                `<tr><td>${esc(s.displayName || s.key)}</td><td>${badge(s.conformance)}</td></tr>`).join('');
            return `
            <div style="background:#fffbeb;border:1px solid #fde68a;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#92400e;">
                No <code>cda.map_to_canonical</code> step found in this pipeline to check live completeness against — showing the static requirements for ${esc(cfg.documentType || 'CCD')} instead. If your source is <code>cda.parse</code>, completeness depends on the parsed document itself; use Test Pipeline's compliance report to check a real message.
            </div>
            ${uscdiSummary}
            <table class="mapping-table" style="width:100%;">
                <thead><tr><th style="font-size:0.7rem;color:#64748b;">Section</th><th style="font-size:0.7rem;color:#64748b;">Requirement</th></tr></thead>
                <tbody>${sectionRows}</tbody>
            </table>`;
        }

        const siblingCfg = sourceStep.config || {};
        const missing = (typeof CDARequirementsHelper !== 'undefined')
            ? CDARequirementsHelper.computeMissingShall(this._requirements, siblingCfg.header, siblingCfg.sections)
            : { shallTotal: 0, shallSatisfied: 0, missingSections: [], missingHeaderFields: [] };
        const banner = (typeof CDARequirementsHelper !== 'undefined') ? CDARequirementsHelper.renderCompletenessBanner(missing) : '';

        const mappedSectionKeys = new Set((siblingCfg.sections || [])
            .filter(s => Array.isArray(s.fields) && s.fields.some(f => f && f.canonicalField))
            .map(s => s.sectionKey));
        const sectionRows = this._requirements.sections.map(s => {
            const configured = mappedSectionKeys.has(s.key);
            return `<tr>
                <td>${esc(s.displayName || s.key)}</td>
                <td>${badge(s.conformance)}</td>
                <td>${configured ? '<i class="fas fa-circle-check" style="color:#16a34a;"></i> Configured' : '<span style="color:#94a3b8;">Not configured</span>'}</td>
            </tr>`;
        }).join('');

        return `
        <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
            Live status of "<strong>${esc(CDARequirementsHelper.stepDisplayName(sourceStep) || 'Map to Canonical')}</strong>" — this pipeline's upstream <code>cda.map_to_canonical</code> step — against ${esc(cfg.documentType || 'CCD')}'s requirements.
        </div>
        ${banner}
        ${uscdiSummary}
        <table class="mapping-table" style="width:100%;">
            <thead><tr>
                <th style="font-size:0.7rem;color:#64748b;">Section</th>
                <th style="font-size:0.7rem;color:#64748b;">Requirement</th>
                <th style="font-size:0.7rem;color:#64748b;">Status</th>
            </tr></thead>
            <tbody>${sectionRows}</tbody>
        </table>`;
    }

    // ── Custodian tab ─────────────────────────────────────────────────────────

    _renderCustodianTab(cfg) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        const cust = cfg.custodian || {};
        const custReq = (this._requirements && this._requirements.headerGroups && this._requirements.headerGroups.custodian) || [];
        const reqByKey = {};
        custReq.forEach(f => { reqByKey[f.key] = f; });
        const badge = key => (reqByKey[key] && typeof CDARequirementsHelper !== 'undefined')
            ? CDARequirementsHelper.renderConformanceBadge(reqByKey[key].conformance) : '';

        const field = (id, label, value, key, placeholder) => `
            <div class="config-group" style="margin-bottom:0.9rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:flex;align-items:center;gap:0.4rem;margin-bottom:0.4rem;">${esc(label)} ${badge(key)}</label>
                <input id="${id}" type="text" class="form-control form-control-sm" value="${esc(value)}" placeholder="${esc(placeholder || '')}" style="font-size:0.82rem;">
            </div>`;

        return `
        <div class="cda-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Custodian.</strong> The organization responsible for this document — deployment-level config (who ezHealthKonnect is sending on behalf of), not per-message source data. Address/Phone are optional — the C-CDA schema this app validates against only requires Name and ID.
            </div>
            ${field('cdaBuildCustOrgName', 'Organization Name', cust.orgName, 'name', cfg.orgName || 'ezHealthKonnect')}
            ${!cust.orgName && cfg.orgName ? `<div style="font-size:0.72rem;color:#94a3b8;margin-top:-0.6rem;margin-bottom:0.9rem;">Not set here yet — currently falls back to this step's previously-saved Organization Name (<code>${esc(cfg.orgName)}</code>). Fill this in to take control of it explicitly.</div>` : ''}
            <div style="display:flex;gap:0.6rem;">
                <div style="flex:1;">${field('cdaBuildCustIdRoot', 'Organization ID Root (OID)', cust.idRoot, 'id', '2.16.840.1.113883.19.5')}</div>
                <div style="flex:1;">${field('cdaBuildCustIdExtension', 'Organization ID Extension', cust.idExtension, 'id', 'e.g. NPI or internal org id')}</div>
            </div>
            ${field('cdaBuildCustStreet', 'Street Address', cust.street, null)}
            <div style="display:flex;gap:0.6rem;">
                <div style="flex:2;">${field('cdaBuildCustCity', 'City', cust.city, null)}</div>
                <div style="flex:1;">${field('cdaBuildCustState', 'State', cust.state, null)}</div>
                <div style="flex:1;">${field('cdaBuildCustPostalCode', 'Postal Code', cust.postalCode, null)}</div>
            </div>
            ${field('cdaBuildCustCountry', 'Country', cust.country, null, 'US')}
            ${field('cdaBuildCustPhone', 'Phone', cust.phone, null)}
        </div>`;
    }

    // ── Legal Authenticator tab ───────────────────────────────────────────────

    _renderLegalAuthenticatorTab(cfg) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        const la = cfg.legalAuthenticator || {};
        const laReq = (this._requirements && this._requirements.headerGroups && this._requirements.headerGroups.legalAuthenticator) || [];
        const reqByKey = {};
        laReq.forEach(f => { reqByKey[f.key] = f; });
        const badge = key => (reqByKey[key] && typeof CDARequirementsHelper !== 'undefined')
            ? CDARequirementsHelper.renderConformanceBadge(reqByKey[key].conformance) : '';

        const field = (id, label, value, key, placeholder) => `
            <div class="config-group" style="margin-bottom:0.9rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:flex;align-items:center;gap:0.4rem;margin-bottom:0.4rem;">${esc(label)} ${badge(key)}</label>
                <input id="${id}" type="text" class="form-control form-control-sm" value="${esc(value)}" placeholder="${esc(placeholder || '')}" style="font-size:0.82rem;">
            </div>`;

        return `
        <div class="cda-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Legal Authenticator.</strong> The person who legally attests to this document's content — genuinely optional (SHOULD, 0..1). Leave Given Name and Family Name blank to omit this element entirely; once a name is provided, Address and Phone (if configured) are written as required children.
            </div>
            <div style="display:flex;gap:0.6rem;">
                <div style="flex:1;">${field('cdaBuildLegalAuthGiven', 'Given Name', la.given, 'given')}</div>
                <div style="flex:1;">${field('cdaBuildLegalAuthFamily', 'Family Name', la.family, 'family')}</div>
            </div>
            ${field('cdaBuildLegalAuthNPI', 'NPI', la.npi, 'npi')}
            <div style="display:flex;gap:0.6rem;">
                <div style="flex:1;">${field('cdaBuildLegalAuthSpecialtyCode', 'Specialty Code', la.specialtyCode, null, 'e.g. 208D00000X')}</div>
                <div style="flex:1;">${field('cdaBuildLegalAuthSpecialtyDisplay', 'Specialty Display Name', la.specialtyCodeDisplay, null)}</div>
                <div style="flex:1;">${field('cdaBuildLegalAuthSpecialtySystem', 'Specialty Code System (OID)', la.specialtyCodeSystem, null, '2.16.840.1.113883.6.101')}</div>
            </div>
            ${field('cdaBuildLegalAuthStreet', 'Street Address', la.street, 'street')}
            <div style="display:flex;gap:0.6rem;">
                <div style="flex:2;">${field('cdaBuildLegalAuthCity', 'City', la.city, 'city')}</div>
                <div style="flex:1;">${field('cdaBuildLegalAuthState', 'State', la.state, 'state')}</div>
                <div style="flex:1;">${field('cdaBuildLegalAuthPostalCode', 'Postal Code', la.postalCode, 'postalCode')}</div>
            </div>
            ${field('cdaBuildLegalAuthCountry', 'Country', la.country, null, 'US')}
            ${field('cdaBuildLegalAuthPhone', 'Phone', la.phone, 'phone')}
        </div>`;
    }

    // ── collectConfig / destroy ──────────────────────────────────────────────

    // Reads General + Custodian + Legal Authenticator tab DOM values into
    // step.config — split out from collectConfig() so onDocTypeChange can
    // persist pending edits before switching document types without needing
    // the full PropertiesPanel save flow.
    _collectGeneralAndCustodianTabs() {
        const root = document.getElementById('cdaBuildBuilder');
        if (!root || !this._step) return;
        const cfg = this._step.config;

        const pick = id => { const el = root.querySelector('#' + id); return el ? el.value.trim() : null; };

        if (pick('cdaBuildInputFormat') !== null) cfg.inputFormat = pick('cdaBuildInputFormat');
        if (pick('cdaBuildSourceField') !== null) cfg.sourceField = pick('cdaBuildSourceField') || 'parsedCDA';
        if (pick('cdaBuildOutputField') !== null) cfg.outputField = pick('cdaBuildOutputField') || 'cdaXML';
        if (pick('cdaBuildDocType') !== null) cfg.documentType = pick('cdaBuildDocType');
        if (pick('cdaBuildTimezoneOffset') !== null) cfg.timezoneOffset = pick('cdaBuildTimezoneOffset');

        cfg.custodian = cfg.custodian || {};
        const custField = (domId, key) => { if (pick(domId) !== null) cfg.custodian[key] = pick(domId); };
        custField('cdaBuildCustOrgName', 'orgName');
        // Keep the legacy top-level orgName in sync with the Custodian tab's
        // Organization Name — cda_build_executor.go's resolveOrgName() is
        // used both as the custodian fallback AND as the document author's
        // representedOrganization/name (which has no dedicated config field
        // of its own), so this one field still needs to serve both roles now
        // that Organization Name has moved into its own tab.
        if (cfg.custodian.orgName) cfg.orgName = cfg.custodian.orgName;
        custField('cdaBuildCustIdRoot', 'idRoot');
        custField('cdaBuildCustIdExtension', 'idExtension');
        custField('cdaBuildCustStreet', 'street');
        custField('cdaBuildCustCity', 'city');
        custField('cdaBuildCustState', 'state');
        custField('cdaBuildCustPostalCode', 'postalCode');
        custField('cdaBuildCustCountry', 'country');
        custField('cdaBuildCustPhone', 'phone');

        cfg.legalAuthenticator = cfg.legalAuthenticator || {};
        const laField = (domId, key) => { if (pick(domId) !== null) cfg.legalAuthenticator[key] = pick(domId); };
        laField('cdaBuildLegalAuthGiven', 'given');
        laField('cdaBuildLegalAuthFamily', 'family');
        laField('cdaBuildLegalAuthNPI', 'npi');
        laField('cdaBuildLegalAuthSpecialtyCode', 'specialtyCode');
        laField('cdaBuildLegalAuthSpecialtyDisplay', 'specialtyCodeDisplay');
        laField('cdaBuildLegalAuthSpecialtySystem', 'specialtyCodeSystem');
        laField('cdaBuildLegalAuthStreet', 'street');
        laField('cdaBuildLegalAuthCity', 'city');
        laField('cdaBuildLegalAuthState', 'state');
        laField('cdaBuildLegalAuthPostalCode', 'postalCode');
        laField('cdaBuildLegalAuthCountry', 'country');
        laField('cdaBuildLegalAuthPhone', 'phone');
    }

    _rerender() {
        const root = document.getElementById('cdaBuildBuilder');
        if (!root || !root.parentElement) return;
        root.outerHTML = this.render(this._step);
    }

    collectConfig(step) {
        this._collectGeneralAndCustodianTabs();
        step.config = this._step.config;
    }

    destroy() {
        this._ac.abort();
        if (window._cdaBuildBuilder === this) {
            window._cdaBuildBuilder = null;
        }
    }
}


// ── Registration ──────────────────────────────────────────────────────────────

if (typeof StepBuilderRegistry !== 'undefined') {
    StepBuilderRegistry.register('cda.parse',     CdaParseStepBuilder);
    StepBuilderRegistry.register('cda.to_fhir',   CdaToFhirStepBuilder);
    StepBuilderRegistry.register('fhir.to_cda',   FHIRToCDABuilder);
    StepBuilderRegistry.register('cda.build',     CDABuildStepBuilder);
    StepBuilderRegistry.register('cda.normalize',  CDANormalizerBuilder);
    StepBuilderRegistry.register('cda.dedupe',    CDADedupeStepBuilder);
}
