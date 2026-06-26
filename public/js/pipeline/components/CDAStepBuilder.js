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
        ['general', 'sections', 'assembly', 'terminology', 'advanced'].forEach(t => {
            const panel = document.getElementById('cdaToFhirTab-' + t);
            const btn   = document.getElementById('cdaToFhirTabBtn-' + t);
            if (panel) panel.style.display = (t === tabName) ? '' : 'none';
            if (btn) {
                if (t === tabName) {
                    btn.style.borderBottom = '2px solid #1e3a8a';
                    btn.style.color = '#1e3a8a';
                    btn.style.fontWeight = '600';
                } else {
                    btn.style.borderBottom = '2px solid transparent';
                    btn.style.color = '#6b7280';
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
            <div style="${hint}">Checks code format (not against VSAC). Invalid codes produce processing result warnings.</div>
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
            <div style="${hint}">Map source codes (e.g. ICD-9-CM) to target codes (e.g. ICD-10-CM) per interface.</div>
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

        const ruleRow = (r, isOn) => `
        <tr style="border-bottom:1px solid #f0f4ff;transition:background 0.1s;"
            onmouseover="this.style.background='#f8faff'" onmouseout="this.style.background=''">
            <td style="padding:0.45rem 0.55rem;width:36px;text-align:center;vertical-align:middle;">
                <input type="checkbox" class="cda-assembly-toggle"
                    data-rule-key="${r.key}"
                    ${isOn ? 'checked' : ''}
                    style="accent-color:#1e3a8a;width:14px;height:14px;cursor:pointer;">
            </td>
            <td style="padding:0.45rem 0.55rem;vertical-align:middle;">
                <span style="font-weight:${isOn ? '600' : '400'};color:${isOn ? '#1f2937' : '#94a3b8'};font-size:0.82rem;">${r.label}</span>
                <div style="font-size:0.69rem;color:#f472b6;font-style:italic;margin-top:2px;">${r.src} → <code style="font-size:0.68rem;color:#1e3a8a;font-style:normal;">${r.fhir}</code></div>
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

        return `
        <div style="font-size:0.79rem;color:#475569;margin-bottom:0.9rem;
                    background:linear-gradient(135deg,#eff6ff 0%,#fdf2f8 100%);
                    border:1px solid #dbeafe;border-radius:6px;padding:0.6rem 0.75rem;">
            Assembly rules control structural transformation decisions during CDA→FHIR conversion.
            <span style="color:#f472b6;font-style:italic;">Disable only if a downstream enrichment step overrides that behaviour.</span>
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

        let lastNestedUnder = null;
        const rows = fields.map(f => {
            let caption = '';
            if (f.nestedUnder && f.nestedUnder !== lastNestedUnder) {
                caption = `
                <tr><td colspan="5" style="padding:0.3rem 0.4rem 0.1rem;font-size:0.7rem;color:#94a3b8;font-style:italic;">
                    ↳ ${esc(f.nestedUnder)}[] (repeating)
                </td></tr>`;
            }
            lastNestedUnder = f.nestedUnder || null;

            const badge = f.isModified
                ? `<span style="font-size:0.65rem;background:#fef3c7;color:#92400e;border:1px solid #fde68a;border-radius:3px;padding:1px 4px;">modified</span>`
                : `<span style="font-size:0.65rem;background:#f0fdf4;color:#166534;border:1px solid #bbf7d0;border-radius:3px;padding:1px 4px;">OOB</span>`;

            const fhirPath = esc(f.fhirPath || '');
            const transform = esc(f.transform || '');
            const indent = f.nestedUnder ? 'padding-left:1.1rem;' : '';
            return `${caption}
            <tr style="border-bottom:1px solid #f1f5f9;vertical-align:top;" data-field-key="${esc(f.key)}">
                <td style="padding:0.35rem 0.4rem;font-family:monospace;font-size:0.75rem;color:#334155;${indent}">
                    ${esc(f.key)} ${badge}
                </td>
                <td style="padding:0.35rem 0.4rem;font-size:0.72rem;color:#6b7280;font-style:italic;">${esc(f.cdaSource || '')}</td>
                <td style="padding:0.35rem 0.4rem;">
                    <input type="text" class="cda-field-fhir-path form-control form-control-sm"
                        data-field-key="${esc(f.key)}" data-section-key="${esc(sectionKey)}"
                        value="${fhirPath}"
                        style="font-size:0.75rem;font-family:monospace;"
                        onchange="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onFieldPathChange('${esc(sectionKey)}','${esc(f.key)}',this.value)">
                </td>
                <td style="padding:0.35rem 0.4rem;">
                    <input type="text" class="cda-field-transform form-control form-control-sm"
                        data-field-key="${esc(f.key)}" data-section-key="${esc(sectionKey)}"
                        value="${transform}"
                        style="font-size:0.75rem;font-family:monospace;"
                        onchange="window._cdaToFhirBuilder && window._cdaToFhirBuilder.onFieldTransformChange('${esc(sectionKey)}','${esc(f.key)}',this.value)">
                </td>
                <td style="padding:0.35rem 0.4rem;white-space:nowrap;">
                    <button type="button"
                        onclick="window._cdaToFhirBuilder && window._cdaToFhirBuilder.resetFieldToOOB('${esc(sectionKey)}','${esc(f.key)}')"
                        title="Reset to OOB"
                        style="padding:0.1rem 0.4rem;font-size:0.68rem;background:#f8fafc;border:1px solid #cbd5e1;border-radius:3px;cursor:pointer;color:#64748b;">
                        Reset
                    </button>
                </td>
            </tr>`;
        }).join('');

        container.innerHTML = `
        <table style="width:100%;border-collapse:collapse;font-size:0.78rem;">
            <thead>
                <tr style="background:#f8fafc;border-bottom:1px solid #e2e8f0;">
                    <th style="padding:0.3rem 0.4rem;text-align:left;font-weight:600;color:#475569;width:22%;">CDA Field</th>
                    <th style="padding:0.3rem 0.4rem;text-align:left;font-weight:600;color:#475569;width:28%;">CDA Source</th>
                    <th style="padding:0.3rem 0.4rem;text-align:left;font-weight:600;color:#475569;width:25%;">FHIR Path</th>
                    <th style="padding:0.3rem 0.4rem;text-align:left;font-weight:600;color:#475569;width:20%;">Transform</th>
                    <th style="padding:0.3rem 0.4rem;width:5%;"></th>
                </tr>
            </thead>
            <tbody>${rows}</tbody>
        </table>
        <div id="cdaTransformInferHint" style="margin-top:0.4rem;font-size:0.72rem;color:#6b7280;min-height:1rem;"></div>`;
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
    }

    onTransformFocus(sectionKey, fieldKey, cdaDataType) {
        // When the transform field is focused, show type-pair inference hint if we know the FHIR type
        const hint = document.getElementById('cdaTransformInferHint');
        if (!hint || !cdaDataType) return;

        // Look up the FHIR path input for this field to get the FHIR type hint
        const pathInput = document.querySelector(`.cda-field-fhir-path[data-field-key="${CSS.escape(fieldKey)}"]`);
        const fhirPath  = pathInput ? pathInput.value.trim() : '';
        if (!fhirPath) {
            hint.textContent = '';
            return;
        }

        // Infer from type pair if FHIR type is derivable from the path
        // (e.g. "AllergyIntolerance.onsetDateTime" → "dateTime")
        const fhirType = this._inferFhirTypeFromPath(fhirPath);
        if (!fhirType) {
            hint.textContent = '';
            return;
        }

        const sig = this._ac.signal;
        fetch('/api/cda/type-pair/infer', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ cdaDataType, fhirDataType: fhirType }),
            signal: sig,
        })
        .then(r => r.ok ? r.json() : null)
        .then(data => {
            if (!data) return;
            if (data.inferred && data.transform) {
                hint.textContent = `Inferred from: ${cdaDataType} → ${fhirType} → ${data.transform}`;
                hint.style.color = '#166534';
            } else {
                hint.textContent = data.message || 'No default transform for this type pair.';
                hint.style.color = '#6b7280';
            }
        })
        .catch(() => {});
    }

    _inferFhirTypeFromPath(fhirPath) {
        // Simple suffix-based heuristics for common FHIR element names
        const lower = fhirPath.toLowerCase();
        if (lower.endsWith('datetime') || lower.endsWith('effectivedatetime') || lower.endsWith('recordeddate')) return 'dateTime';
        if (lower.endsWith('date')) return 'date';
        if (lower.endsWith('quantity') || lower.endsWith('valuequantity')) return 'Quantity';
        if (lower.endsWith('period')) return 'Period';
        if (lower.endsWith('range')) return 'Range';
        if (lower.endsWith('codeableconcept') || lower.endsWith('code')) return 'code';
        if (lower.endsWith('humanname') || lower.endsWith('name')) return 'HumanName';
        if (lower.endsWith('address')) return 'Address';
        if (lower.endsWith('contactpoint') || lower.endsWith('telecom')) return 'ContactPoint';
        return '';
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


// ── Registration ──────────────────────────────────────────────────────────────

if (typeof StepBuilderRegistry !== 'undefined') {
    StepBuilderRegistry.register('cda.parse',     CdaParseStepBuilder);
    StepBuilderRegistry.register('cda.to_fhir',   CdaToFhirStepBuilder);
    StepBuilderRegistry.register('fhir.to_cda',   FHIRToCDABuilder);
    StepBuilderRegistry.register('cda.normalize',  CDANormalizerBuilder);
}
