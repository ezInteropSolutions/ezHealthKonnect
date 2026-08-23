/**
 * FHIRBuildBuilder — step config builder for the "fhir.build" pipeline step.
 *
 * The no-code, format-agnostic on-ramp for a single FHIR R4 resource: lets a
 * user map CSV columns, DB query columns, or arbitrary JSON fields directly
 * onto a FHIR resource's own element paths — see
 * services/executors/transform/fhir_build_executor.go. The FHIR-side mirror
 * of MapToCanonicalBuilder.js, adapted for a flat resource shape instead of
 * CDA's header+sections model: there is no separate "build" step here, since
 * a FHIR resource already IS the JSON output this step produces.
 *
 * Catalogs (resource types, profiles, element fields, transforms) are all
 * fetched live from the backend (fhir/builder/canonical_field_catalog.go +
 * services/cda_fhir.DeclarativeTransformRegistry, exposed via /api/fhir/*),
 * never hardcoded here — same discipline MapToCanonicalBuilder.js follows.
 *
 * Builder contract (StepBuilderRegistry):
 *   render(step)         → string  HTML for the properties panel form tab
 *   collectConfig(step)  → void    reads DOM, writes into step.config
 *   destroy()            → void    tears down event listeners / AC refs
 */

class FHIRBuildBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
        this._step = null;
        this._resourceTypes = [];      // ["Patient", "Observation", ...]
        this._profiles = [];           // ["base", "us-core", ...] for current resourceType
        this._fieldCatalog = [];       // [{key, label, dataType, required}] for current resourceType+profile+version
        this._transformCatalog = {};   // name -> description
        this._fieldSearches = [];      // active FieldPathSearchComponent instances
        this._suggestPanelOpen = false;
        this._suggestSampleRowsText = '';
        this._suggestError = '';
        this._suggestBusy = false;

        // Extension picker (Feature 1) — reuses FhirExtensionCatalog's pure
        // catalog data only, never its HL7-shaped ExtensionBuilder modal.
        this._extPanelOpen = false;
        this._extSearchText = '';

        // Validate Against Sample (Feature 3) — "strict mode": reuses the
        // real fhir/r4 validator via the existing Test Pipeline endpoint,
        // scoped to just this resource, instead of a second rules engine.
        this._validatePanelOpen = false;
        this._validateSampleText = '';
        this._validateBusy = false;
        this._validateError = '';
        this._validateResult = null;
        this._validateIssueByRowKey = {};

        window._fhirBuildBuilder = this;
    }

    // ── render ────────────────────────────────────────────────────────────────

    render(step) {
        this._step = step;
        if (!step.config) step.config = {};
        this._applyDefaultConfig(step.config);

        this._loadResourceTypes();
        this._loadProfiles(step.config.resourceType);
        this._loadFieldCatalog();
        this._loadTransformCatalog();

        const html = this._renderAll();
        setTimeout(() => this._attachFieldSearches(), 0);
        return html;
    }

    _applyDefaultConfig(cfg) {
        if (!cfg.resourceType) cfg.resourceType = 'Patient';
        if (!cfg.profile) cfg.profile = 'base';
        if (!cfg.version) cfg.version = 'R4';
        if (!cfg.outputField) cfg.outputField = 'fhirResource';
        if (!Array.isArray(cfg.fields)) cfg.fields = [];
        if (!Array.isArray(cfg.repeatingGroups)) cfg.repeatingGroups = [];
    }

    _renderAll() {
        const cfg = this._step.config;
        const esc = FHIRBuildBuilder._esc;

        const resourceOptions = this._resourceTypes
            .map(rt => `<option value="${esc(rt)}" ${rt === cfg.resourceType ? 'selected' : ''}>${esc(rt)}</option>`)
            .join('');
        const profileOptions = (this._profiles.length ? this._profiles : [cfg.profile])
            .map(p => `<option value="${esc(p)}" ${p === cfg.profile ? 'selected' : ''}>${esc(p)}</option>`)
            .join('');

        const missingRequired = (typeof FHIRRequirementsHelper !== 'undefined')
            ? FHIRRequirementsHelper.computeMissingRequired(this._fieldCatalog, cfg.fields, cfg.repeatingGroups)
            : { total: 0, satisfied: 0, missing: [] };

        return `
        <div id="fhirBuildBuilder" class="cda-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>FHIR Resource Builder step.</strong> Maps CSV/DB/JSON fields directly onto one FHIR R4 resource.
                Add one step per resource type, then use a Payload Builder (FHIR Bundle mode) step to assemble a Bundle.
            </div>
            ${typeof FHIRRequirementsHelper !== 'undefined' ? FHIRRequirementsHelper.renderCompletenessBanner(missingRequired) : ''}

            <div class="config-group" style="margin-bottom:1.1rem;">
                <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.5rem;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;margin:0;">Validation</label>
                    <button type="button" class="btn btn-sm btn-outline-secondary"
                        onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.toggleValidatePanel()"
                        style="font-size:0.72rem;padding:0.15rem 0.5rem;">🛡️ Validate Against Sample</button>
                </div>
                <div style="font-size:0.7rem;color:#94a3b8;">The banner above checks field existence only ("loose"). This runs the real fhir/r4 validator (structure, cardinality, terminology bindings, constraints — "strict") against one sample message you provide.</div>
                ${this._renderValidatePanel()}
            </div>

            <div class="config-group" style="margin-bottom:1.1rem;display:flex;gap:0.6rem;flex-wrap:wrap;">
                <div style="flex:1 1 160px;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Resource Type</label>
                    <select id="fbbResourceType" class="form-select form-select-sm">
                        ${resourceOptions || `<option value="${esc(cfg.resourceType)}">${esc(cfg.resourceType)}</option>`}
                    </select>
                </div>
                <div style="flex:1 1 120px;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Profile</label>
                    <select id="fbbProfile" class="form-select form-select-sm">
                        ${profileOptions}
                    </select>
                </div>
                <div style="flex:0 0 100px;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Version</label>
                    <input id="fbbVersion" type="text" class="form-control form-control-sm" value="${esc(cfg.version)}">
                </div>
                <div style="flex:1 1 200px;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                    <input id="fbbOutputField" type="text" class="form-control form-control-sm"
                        value="${esc(cfg.outputField)}" placeholder="fhirResource"
                        style="font-family:monospace;font-size:0.82rem;">
                </div>
            </div>

            <div class="config-group" style="margin-bottom:1.1rem;">
                <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.5rem;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;margin:0;">Fields</label>
                    <div style="display:flex;gap:0.4rem;">
                        <button type="button" class="btn btn-sm btn-outline-secondary"
                            onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.toggleExtensionPanel()"
                            style="font-size:0.72rem;padding:0.15rem 0.5rem;">🔌 Add Extension</button>
                        <button type="button" class="btn btn-sm btn-outline-secondary"
                            onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.toggleSuggestPanel()"
                            style="font-size:0.72rem;padding:0.15rem 0.5rem;">✨ Suggest Mappings</button>
                        <button type="button" class="btn btn-sm btn-outline-primary"
                            onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.addField()"
                            style="font-size:0.72rem;padding:0.15rem 0.5rem;">+ Add Field</button>
                    </div>
                </div>
                ${this._renderExtensionPanel()}
                ${this._renderSuggestPanel()}
                <datalist id="fbbFieldTargets">
                    ${this._fieldCatalog.map(f => {
                        const bits = [];
                        if (f.required) bits.push('required');
                        if (f.bindingStrength) bits.push(f.bindingStrength + ' binding');
                        return `<option value="${esc(f.key)}">${esc(f.label || f.key)}${bits.length ? ' (' + bits.join(', ') + ')' : ''}</option>`;
                    }).join('')}
                </datalist>
                <div style="font-size:0.7rem;color:#94a3b8;margin-bottom:0.5rem;">Flat/single-value element paths, e.g. <code>birthDate</code>, <code>identifier[0].value</code>.</div>
                <div id="fbbFieldsContainer">
                    ${cfg.fields.length === 0
                        ? `<div style="text-align:center;padding:1rem;color:#94a3b8;font-size:0.8rem;border:1px dashed #cbd5e1;border-radius:6px;">No fields mapped yet.</div>`
                        : this._renderFieldsTable(cfg.fields, 'fbbFieldTargets', -1, this._validateIssueByRowKey)}
                </div>
            </div>

            <div class="config-group">
                <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.5rem;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;margin:0;">Repeating Groups</label>
                    <button type="button" class="btn btn-sm btn-primary"
                        onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.addRepeatingGroup()">
                        <i class="fas fa-plus"></i> Add Group
                    </button>
                </div>
                <div style="font-size:0.7rem;color:#94a3b8;margin-bottom:0.5rem;">For repeating elements (<code>identifier[]</code>, <code>name[]</code>, <code>telecom[]</code>) — one sub-object is built per source row, so multiple fields from the same row stay aligned.</div>
                <div id="fbbGroupsContainer">
                    ${cfg.repeatingGroups.length === 0
                        ? `<div style="text-align:center;padding:1.2rem;color:#94a3b8;font-size:0.8rem;border:1px dashed #cbd5e1;border-radius:6px;">No repeating groups configured yet.</div>`
                        : cfg.repeatingGroups.map((rg, i) => this._renderGroupCard(rg, i)).join('')}
                </div>
            </div>

            ${this._renderNarrativeFieldsSection(cfg)}
        </div>`;
    }

    // ── Narrative Fields ──────────────────────────────────────────────────────
    // Which fields render in THIS resource type's auto-generated narrative
    // (resource.text.div). Single-type mode, pre-scoped to cfg.resourceType —
    // no type list to fetch, unlike the multi-type pickers on
    // hl7_fhir_transform/cda.to_fhir. Shares storage with hl7_fhir_transform
    // (interface_message_mappings.custom_mapping_config, GET/PATCH
    // /api/fhir/optional-segments) per the user-confirmed design: a
    // fhir.build-produced resource renders consistently with any other
    // resource of the same type in that interface, regardless of which step
    // built it. Populated by _initNarrativePicker(), wired from
    // _attachFieldSearches() (called after every render()/_rerender()).

    _renderNarrativeFieldsSection(cfg) {
        const esc = FHIRBuildBuilder._esc;
        return `
        <div class="config-group" style="margin-top:1.1rem;">
            <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.5rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;margin:0;">Narrative Fields — ${esc(cfg.resourceType)}</label>
            </div>
            <div style="font-size:0.7rem;color:#94a3b8;margin-bottom:0.5rem;">Controls which fields appear in this resource's human-readable summary (<code>resource.text.div</code>). Every populated field shows by default — uncheck fields to hide them for this interface's ${esc(cfg.resourceType)} resources.</div>
            <div id="fbbNarrFieldsSections" style="display:flex;flex-direction:column;gap:6px;"></div>
            <div style="margin-top:10px;">
                <button type="button" id="fbbNarrFieldsSaveBtn" class="btn btn-sm btn-outline-secondary" style="font-size:0.72rem;padding:0.15rem 0.5rem;">
                    <i class="fas fa-save"></i> Save Narrative Fields
                </button>
                <span id="fbbNarrFieldsStatus" style="margin-left:8px;font-size:11px;color:#6b7280;"></span>
            </div>
        </div>`;
    }

    _initNarrativePicker() {
        if (typeof NarrativeFieldsPicker === 'undefined') return;
        const root = document.getElementById('fhirBuildBuilder');
        const sectionsEl = root && root.querySelector('#fbbNarrFieldsSections');
        if (!sectionsEl || !this._step) return;

        const cfg = this._step.config;
        const interfaceId = cfg.interface_id || (window.pipelineBuilder && window.pipelineBuilder.pipeline && window.pipelineBuilder.pipeline.interfaceId);
        const messageType = cfg.message_type || (window.pipelineBuilder && window.pipelineBuilder.pipeline && window.pipelineBuilder.pipeline.messageType) || 'ADT^A01';
        const optSegUrl = `/api/fhir/optional-segments?messageType=${encodeURIComponent(messageType)}` +
            (interfaceId ? `&interfaceId=${encodeURIComponent(interfaceId)}` : '');

        const picker = new NarrativeFieldsPicker({
            instanceId: `fhirbuild-${this._step.id}`,
            sectionsEl,
            statusEl: root.querySelector('#fbbNarrFieldsStatus'),
            getConfig: async () => {
                const res = await fetch(optSegUrl, { credentials: 'include' }).then(r => r.json());
                return res.narrativeFields || {};
            },
            onSave: async (payload) => {
                if (!interfaceId) return { success: false, error: 'Save the interface before configuring narrative fields.' };
                return fetch(`/api/fhir/optional-segments/${encodeURIComponent(interfaceId)}/${encodeURIComponent(messageType)}`, {
                    method: 'PATCH',
                    credentials: 'include',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ narrative_fields: payload }),
                }).then(r => r.json());
            },
        });
        picker.render([cfg.resourceType]);

        const saveBtn = root.querySelector('#fbbNarrFieldsSaveBtn');
        if (saveBtn) saveBtn.addEventListener('click', () => picker.save());
    }

    _renderTransformOptions(selectedName) {
        const esc = FHIRBuildBuilder._esc;
        const names = Object.keys(this._transformCatalog).sort();
        const opts = names.map(name =>
            `<option value="${esc(name)}" title="${esc(this._transformCatalog[name])}" ${name === selectedName ? 'selected' : ''}>${esc(name)}</option>`
        ).join('');
        return `<option value="">— none (raw value) —</option>${opts}`;
    }

    // catalogFor scopes the flat resource-level field catalog to one repeating
    // group's targetPath prefix (e.g. "identifier." -> "system"/"value"/...),
    // client-side only — no separate backend endpoint needed since the
    // compiled profile's element paths are already fully qualified.
    _catalogFor(prefix) {
        if (!prefix) return this._fieldCatalog;
        const p = prefix + '.';
        return this._fieldCatalog
            .filter(f => f.key.startsWith(p))
            .map(f => ({ ...f, key: f.key.slice(p.length) }))
            .filter(f => f.key);
    }

    // ── Fields table (shared by top-level Fields and each group's Fields) ────

    _renderFieldsTable(fields, datalistId, groupIndex, issuesByRowKey) {
        const esc = FHIRBuildBuilder._esc;
        const rowsHtml = fields.map((f, fi) => {
            const rowKey = groupIndex === -1 ? `top:${fi}` : `group:${groupIndex}:${fi}`;
            const issues = (issuesByRowKey || {})[rowKey];
            const hasError = issues && issues.some(i => i.severity === 'error');
            const badge = issues && issues.length
                ? `<span title="${esc(issues.map(i => i.message).join('; '))}" style="color:${hasError ? '#dc2626' : '#d97706'};margin-left:4px;cursor:help;">${hasError ? '❌' : '⚠️'}</span>`
                : '';
            return `
            <tr data-field-index="${fi}">
                <td>
                    <input type="text" class="form-control form-control-sm fbb-field-target"
                        value="${esc(f.targetPath)}" list="${datalistId}" placeholder="e.g. birthDate" autocomplete="off">${badge}
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm fbb-field-source"
                        value="${esc(f.sourcePath)}" placeholder="e.g. dob" autocomplete="off">
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm fbb-field-fallback"
                        value="${esc((f.fallbackPaths || []).join(', '))}" placeholder="alt1, alt2" autocomplete="off"
                        title="Comma-separated fallback source paths, tried in order if Source Path is empty">
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm fbb-field-literal"
                        value="${esc(f.literalValue)}" placeholder="(fixed value)" autocomplete="off"
                        title="Used only when no source/fallback path resolves">
                </td>
                <td>
                    <select class="form-select form-select-sm fbb-field-transform" style="font-size:0.78rem;"
                        title="Optional named transform (leave empty to write the raw value)">
                        ${this._renderTransformOptions(f.transform)}
                    </select>
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm fbb-field-valuemap"
                        value="${esc(FHIRBuildBuilder._valueMapToText(f.valueMap))}" placeholder="A=active, R=resolved" autocomplete="off"
                        title="Comma-separated raw=value translations, passed into the Transform">
                </td>
                <td style="width:40px;text-align:center;">
                    <button type="button" class="btn btn-sm btn-danger"
                        onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.removeField(${groupIndex}, ${fi})"
                        title="Remove field"><i class="fas fa-trash"></i></button>
                </td>
            </tr>`;
        }).join('');

        return `
        <div style="overflow-x:auto;">
            <table class="mapping-table" style="width:100%;min-width:760px;">
                <thead><tr>
                    <th style="font-size:0.68rem;color:#64748b;">Target Path</th>
                    <th style="font-size:0.68rem;color:#64748b;">Source Path</th>
                    <th style="font-size:0.68rem;color:#64748b;">Fallback Paths</th>
                    <th style="font-size:0.68rem;color:#64748b;">Literal</th>
                    <th style="font-size:0.68rem;color:#64748b;">Transform</th>
                    <th style="font-size:0.68rem;color:#64748b;">Value Map</th>
                    <th></th>
                </tr></thead>
                <tbody>${rowsHtml}</tbody>
            </table>
        </div>`;
    }

    addField() {
        this._syncDOMToConfig();
        this._step.config.fields.push({ targetPath: '', sourcePath: '' });
        this._rerender();
    }

    // ── Extension picker ──────────────────────────────────────────────────────
    // Reuses FhirExtensionCatalog.js's pure catalog (search/getById/buildPath)
    // ONLY — never ExtensionBuilder.js's modal, which is hard-wired to
    // HL7-specific inputs (hl7Field/hl7DataType) this format-agnostic builder
    // never has, and duplicates transform/sample-testing UI this builder's own
    // Fields-table rows already provide per-row.
    //
    // Deliberately never copies a catalog entry's own "transform" field onto
    // the new row: FhirExtensionCatalog's transform names (e.g.
    // ce_to_codeableconcept, hl7_active_flag) belong to the older HL7-wizard
    // transform vocabulary and have ZERO overlap with
    // cdafhir.DeclarativeTransformRegistry (the registry fhir.build actually
    // uses) — copying one would silently fail at build time
    // (transformReg.Apply -> "unknown transform" -> field skipped, no error
    // surfaced) while the dropdown shows a dead value it doesn't even list.

    _renderExtensionPanel() {
        if (!this._extPanelOpen) return '';
        const esc = FHIRBuildBuilder._esc;
        return `
        <div style="border:1px dashed #94a3b8;border-radius:6px;padding:0.6rem 0.7rem;margin-bottom:0.6rem;background:#fafafa;">
            <div style="font-size:0.72rem;color:#64748b;margin-bottom:0.4rem;">Search FHIR extensions (e.g. "race", "birth sex", "organ donor") — picking one adds a Fields row with the target path pre-filled; set its Source Path like any other field.</div>
            <input id="fbbExtSearch" type="text" class="form-control form-control-sm"
                placeholder="Search extensions…" value="${esc(this._extSearchText)}" autocomplete="off"
                oninput="window._fhirBuildBuilder && window._fhirBuildBuilder.onExtensionSearch(this.value)"
                style="margin-bottom:0.5rem;">
            <div id="fbbExtResults" style="max-height:220px;overflow-y:auto;border:1px solid #e2e8f0;border-radius:6px;background:white;">
                ${this._renderExtensionResults()}
            </div>
        </div>`;
    }

    _renderExtensionResults() {
        if (typeof FhirExtensionCatalog === 'undefined') return '';
        const esc = FHIRBuildBuilder._esc;
        const cfg = this._step.config;
        const results = FhirExtensionCatalog.search(this._extSearchText || '');
        if (results.length === 0) {
            return `<div style="padding:0.75rem;text-align:center;color:#94a3b8;font-size:0.8rem;">No extensions match.</div>`;
        }
        // Matches for the current resource type first — never hard-excludes
        // others, since some catalog entries are resource-agnostic.
        const sorted = [...results].sort((a, b) => {
            const aMatch = (a.resources || []).includes(cfg.resourceType) ? 0 : 1;
            const bMatch = (b.resources || []).includes(cfg.resourceType) ? 0 : 1;
            return aMatch - bMatch;
        });
        return sorted.map(ext => `
            <div class="fbb-ext-row" style="padding:0.5rem 0.7rem;cursor:pointer;border-bottom:1px solid #f1f5f9;display:flex;align-items:flex-start;justify-content:space-between;gap:0.5rem;"
                onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.addExtensionField('${esc(ext.id)}')"
                onmouseover="this.style.background='#f0f9ff'" onmouseout="this.style.background=''">
                <div style="flex:1;min-width:0;">
                    <div style="font-size:0.82rem;font-weight:600;color:#1e3a8a;">${esc(ext.name)}</div>
                    <div style="font-size:0.72rem;color:#64748b;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${esc(ext.url)}">${esc(ext.url)}</div>
                </div>
                <code style="background:#dbeafe;padding:2px 6px;border-radius:3px;font-size:0.7rem;color:#1e3a8a;white-space:nowrap;flex-shrink:0;">${esc(ext.valueType)}</code>
            </div>`).join('');
    }

    onExtensionSearch(text) {
        // Targeted update only — going through _rerender()'s full outerHTML
        // replace would steal the search input's focus on every keystroke.
        this._extSearchText = text;
        const results = document.getElementById('fbbExtResults');
        if (results) results.innerHTML = this._renderExtensionResults();
    }

    addExtensionField(extId) {
        const ext = typeof FhirExtensionCatalog !== 'undefined' ? FhirExtensionCatalog.getById(extId) : null;
        if (!ext) return;
        this._syncDOMToConfig();
        this._step.config.fields.push({
            targetPath: FhirExtensionCatalog.buildPath('', ext.url, ext.valueType),
            sourcePath: '',
        });
        this._extPanelOpen = false;
        this._rerender();
    }

    toggleExtensionPanel() {
        this._syncDOMToConfig();
        this._extPanelOpen = !this._extPanelOpen;
        this._rerender();
    }

    // ── Validate Against Sample ("strict mode") ──────────────────────────────
    // "Loose" is the existence-only completeness banner above. "Strict" is
    // this: run the CURRENT unsaved config through the real fhir.build
    // executor + the real fhir/r4 validator (via the already-existing Test
    // Pipeline endpoint, validation_level "strict" — structure, cardinality,
    // terminology bindings, constraints), scoped to one sample message.
    // Zero new backend code; this is 100% the same engine fhir_validation
    // already runs in production, just surfaced earlier in the workflow.

    _renderValidatePanel() {
        if (!this._validatePanelOpen) return '';
        const esc = FHIRBuildBuilder._esc;

        let resultHtml = '';
        if (this._validateResult) {
            const r = this._validateResult;
            const clean = r.errorCount === 0;
            const bg = clean ? '#f0fdf4' : '#fef2f2';
            const border = clean ? '#bbf7d0' : '#fecaca';
            const color = clean ? '#166534' : '#991b1b';
            const icon = clean ? 'fa-circle-check' : 'fa-triangle-exclamation';
            const unattached = r.unattached || [];
            resultHtml = `
            <div style="background:${bg};border:1px solid ${border};border-radius:6px;padding:0.55rem 0.75rem;margin-top:0.5rem;font-size:0.78rem;color:${color};">
                <div style="font-weight:600;"><i class="fas ${icon}"></i> ${r.errorCount} error(s), ${r.warningCount} warning(s) against the sample</div>
                ${r.errorCount + r.warningCount > 0 ? `<div style="margin-top:0.3rem;font-weight:400;">Errors/warnings tied to a specific field are flagged on that row below.${unattached.length > 0 ? ' Resource-level findings with no single row:' : ''}</div>` : ''}
                ${unattached.length > 0 ? `<div style="margin-top:0.2rem;">${unattached.map(u => `<div>${u.severity === 'error' ? '❌' : '⚠️'} ${esc(u.message)}${u.path ? ` <code>(${esc(u.path)})</code>` : ''}</div>`).join('')}</div>` : ''}
            </div>`;
        }

        return `
        <div style="border:1px dashed #94a3b8;border-radius:6px;padding:0.6rem 0.7rem;margin-top:0.6rem;background:#fafafa;">
            <div style="font-size:0.72rem;color:#64748b;margin-bottom:0.4rem;">Paste ONE sample message — a JSON object, not an array (this runs the resource through fhir.build once, then validates the result).</div>
            <textarea id="fbbValidateSample" class="form-control form-control-sm" rows="4"
                placeholder='{"dob":"1980-05-20","sexCode":"F"}'
                style="font-family:monospace;font-size:0.78rem;margin-bottom:0.4rem;">${esc(this._validateSampleText || '')}</textarea>
            <div style="display:flex;align-items:center;gap:0.6rem;">
                <button type="button" class="btn btn-sm btn-primary" style="font-size:0.72rem;"
                    onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.runValidateSample()">Validate</button>
                ${this._validateError ? `<span style="font-size:0.72rem;color:#dc2626;">${esc(this._validateError)}</span>` : ''}
                ${this._validateBusy ? `<span style="font-size:0.72rem;color:#64748b;">Running fhir.build + fhir_validation…</span>` : ''}
            </div>
            ${resultHtml}
        </div>`;
    }

    toggleValidatePanel() {
        this._syncDOMToConfig();
        this._validatePanelOpen = !this._validatePanelOpen;
        this._rerender();
    }

    runValidateSample() {
        const root = document.getElementById('fhirBuildBuilder');
        const textEl = root && root.querySelector('#fbbValidateSample');
        this._validateSampleText = textEl ? textEl.value : this._validateSampleText;
        this._validateError = '';

        let sample;
        try {
            sample = JSON.parse(this._validateSampleText || '');
            if (!sample || typeof sample !== 'object' || Array.isArray(sample)) {
                throw new Error('Provide a single JSON object, not an array.');
            }
        } catch (e) {
            this._validateError = 'Invalid sample: ' + e.message;
            this._rerender();
            return;
        }

        this._syncDOMToConfig();
        const cfg = this._step.config;
        this._validateBusy = true;
        this._validateResult = null;
        this._validateIssueByRowKey = {};
        this._rerender();

        // The synthetic build step's outputField MUST resolve under "message."
        // regardless of what the real (possibly still-default "fhirResource")
        // step config uses: the pipeline engine's step-chaining only merges a
        // step's own output["message"] into the next step's context
        // (transformation_pipeline_helpers.go) — a sibling top-level key like
        // the default "fhirResource" is silently dropped between steps, so
        // fhir_validation would report "No FHIR data found" even though the
        // build itself succeeded. The real FHIR_BUILD_DEMO pipeline avoids this
        // by explicitly configuring outputField as "message.fhirPatient" etc.;
        // this ad-hoc 2-step check does the same, without touching the field
        // the user actually configured (and will save).
        const buildCfg = Object.assign({}, cfg, { outputField: 'message.fhirResource' });

        fetch('/api/fhir/pipeline/test', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                test_message: JSON.stringify(sample),
                pipeline: {
                    interfaceId: 'fhir-build-validate-sample', messageType: 'TEST',
                    execution_groups: [{
                        steps: [
                            { id: 'fbb-validate-build', stepName: 'Validate Sample Build', stepType: 'fhir.build', sequence: 10, enabled: true, config: buildCfg },
                            {
                                id: 'fbb-validate-check', stepName: 'Validate Sample Check', stepType: 'fhir_validation', sequence: 20, enabled: true,
                                config: { profile: cfg.profile, fhir_version: cfg.version, validation_level: 'strict' },
                            },
                        ],
                    }],
                },
            }),
            signal: this._ac.signal,
        })
            .then(r => r.json())
            .then(resp => {
                this._validateBusy = false;
                if (!resp || !resp.success) {
                    this._validateError = (resp && (resp.error || resp.message)) || 'Validation request failed.';
                    this._rerender();
                    return;
                }
                const checkStep = resp.steps && resp.steps.validate_sample_check;
                const out = (checkStep && checkStep.step_output) || {};
                this._applyValidationResult(out.errors || [], out.warnings || []);
                this._rerender();
            })
            .catch(() => {
                this._validateBusy = false;
                this._validateError = 'Validation request failed.';
                this._rerender();
            });
    }

    // Maps the flat errors/warnings string arrays fhir_validation returns
    // back onto the specific Fields-table row(s) that likely caused each one.
    _applyValidationResult(errors, warnings) {
        const cfg = this._step.config;
        const issuesByRowKey = {};
        const unattached = [];
        const process = (list, severity) => {
            list.forEach(s => {
                const parsed = FHIRBuildBuilder._parseIssueString(s, cfg.resourceType);
                const rowKeys = this._matchIssueToRow(parsed.path);
                if (rowKeys.length === 0) {
                    unattached.push({ severity, message: parsed.message || s, path: parsed.path });
                } else {
                    rowKeys.forEach(k => {
                        if (!issuesByRowKey[k]) issuesByRowKey[k] = [];
                        issuesByRowKey[k].push({ severity, message: parsed.message || s, code: parsed.code });
                    });
                }
            });
        };
        process(errors, 'error');
        process(warnings, 'warning');
        this._validateIssueByRowKey = issuesByRowKey;
        this._validateResult = { errorCount: errors.length, warningCount: warnings.length, unattached };
    }

    // Structurally parses fhir_validation's fixed, code-owned wire format
    // ("[code] ResourceType.path: message", see fmtIssue in
    // services/executors/validation/fhir_validation_executor.go) — not fuzzy
    // text matching. A resource-level/cross-field issue (e.g. a constraint
    // with no single element path) falls through with path:"".
    static _parseIssueString(s, resourceType) {
        const m = /^\[([^\]]+)\]\s*(.*)$/.exec(s);
        if (!m) return { code: '', path: '', message: s };
        const code = m[1];
        const rest = m[2];
        const prefix = resourceType + '.';
        if (rest.startsWith(prefix)) {
            const sep = rest.indexOf(': ');
            if (sep !== -1) {
                return { code, path: rest.slice(prefix.length, sep), message: rest.slice(sep + 2) };
            }
        }
        return { code, path: '', message: rest };
    }

    // Compares a resolved (failing) element path against every configured
    // row's own targetPath — structural comparison (exact, nested-under, or
    // parent-of), not text similarity. Array indices are stripped since a
    // row's own targetPath rarely carries the same index the validator's
    // resolved path does.
    _matchIssueToRow(path) {
        if (!path) return [];
        const cfg = this._step.config;
        const norm = (s) => String(s || '').replace(/\[\d+\]/g, '');
        const relates = (rowPath) => {
            if (!rowPath) return false;
            const a = norm(rowPath), b = norm(path);
            return a === b || b.startsWith(a + '.') || a.startsWith(b + '.');
        };
        const keys = [];
        (cfg.fields || []).forEach((f, fi) => {
            if (relates(f.targetPath)) keys.push('top:' + fi);
        });
        (cfg.repeatingGroups || []).forEach((rg, gi) => {
            (rg.fields || []).forEach((f, fi) => {
                const full = rg.targetPath ? `${rg.targetPath}.${f.targetPath}` : f.targetPath;
                if (relates(full)) keys.push(`group:${gi}:${fi}`);
            });
        });
        return keys;
    }

    // ── AI-assisted "Suggest Mappings" (design-time only) ────────────────────
    // Sends pasted sample rows + this step's own live field catalog to
    // POST /api/ai/suggest-field-mappings (services/ai/mapping_suggester_service.go).
    // Suggestions only ever pre-fill the editable Fields table below — nothing
    // is auto-applied or saved; the pipeline's own Save action remains the
    // human-approval gate.

    _renderSuggestPanel() {
        if (!this._suggestPanelOpen) return '';
        const esc = FHIRBuildBuilder._esc;
        return `
        <div style="border:1px dashed #94a3b8;border-radius:6px;padding:0.6rem 0.7rem;margin-bottom:0.6rem;background:#fafafa;">
            <div style="font-size:0.72rem;color:#64748b;margin-bottom:0.4rem;">Paste sample source rows (a JSON array of objects) — suggestions are added to the Fields table below for you to review, never saved automatically.</div>
            <textarea id="fbbSuggestSampleRows" class="form-control form-control-sm" rows="4"
                placeholder='[{"firstName":"Jane","dob":"1980-05-20"}]'
                style="font-family:monospace;font-size:0.78rem;margin-bottom:0.4rem;">${esc(this._suggestSampleRowsText || '')}</textarea>
            <div style="display:flex;align-items:center;gap:0.6rem;">
                <button type="button" class="btn btn-sm btn-primary" style="font-size:0.72rem;"
                    onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.runSuggestMappings()">Suggest</button>
                ${this._suggestError ? `<span style="font-size:0.72rem;color:#dc2626;">${esc(this._suggestError)}</span>` : ''}
                ${this._suggestBusy ? `<span style="font-size:0.72rem;color:#64748b;">Asking the local model…</span>` : ''}
            </div>
        </div>`;
    }

    toggleSuggestPanel() {
        this._syncDOMToConfig();
        this._suggestPanelOpen = !this._suggestPanelOpen;
        this._rerender();
    }

    runSuggestMappings() {
        const root = document.getElementById('fhirBuildBuilder');
        const textEl = root && root.querySelector('#fbbSuggestSampleRows');
        this._suggestSampleRowsText = textEl ? textEl.value : this._suggestSampleRowsText;
        this._suggestError = '';

        let sampleRows;
        try {
            sampleRows = JSON.parse(this._suggestSampleRowsText || '[]');
            if (!Array.isArray(sampleRows) || sampleRows.length === 0) throw new Error('Provide a non-empty JSON array of sample rows.');
        } catch (e) {
            this._suggestError = 'Invalid sample rows: ' + e.message;
            this._rerender();
            return;
        }

        this._syncDOMToConfig();
        this._suggestBusy = true;
        this._rerender();

        fetch('/api/ai/suggest-field-mappings', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                step_type: 'fhir.build',
                sample_rows: sampleRows,
                target_fields: this._fieldCatalog,
            }),
            signal: this._ac.signal,
        })
            .then(r => r.json())
            .then(resp => {
                this._suggestBusy = false;
                if (!resp || !resp.success) {
                    this._suggestError = (resp && resp.error) || 'Suggestion request failed.';
                    this._rerender();
                    return;
                }
                const suggestions = (resp.data && resp.data.suggestions) || [];
                const existingTargets = new Set(this._step.config.fields.map(f => f.targetPath));
                suggestions.forEach(s => {
                    if (!s.target_field || existingTargets.has(s.target_field)) return;
                    this._step.config.fields.push({ targetPath: s.target_field, sourcePath: s.source_field || '' });
                    existingTargets.add(s.target_field);
                });
                this._suggestPanelOpen = false;
                this._rerender();
            })
            .catch(() => {
                this._suggestBusy = false;
                this._suggestError = 'Suggestion request failed.';
                this._rerender();
            });
    }

    removeField(groupIndex, fieldIndex) {
        this._syncDOMToConfig();
        const cfg = this._step.config;
        const target = groupIndex === -1 ? cfg.fields : (cfg.repeatingGroups[groupIndex] || {}).fields;
        if (!Array.isArray(target)) return;
        target.splice(fieldIndex, 1);
        this._rerender();
    }

    // ── Repeating groups ──────────────────────────────────────────────────────

    _renderGroupCard(rg, groupIndex) {
        const esc = FHIRBuildBuilder._esc;
        const datalistId = `fbbGroupTargets-${groupIndex}`;
        const groupCatalog = this._catalogFor(rg.targetPath);

        return `
        <div class="fbb-group-card" data-group-index="${groupIndex}"
            style="border:1px solid #e2e8f0;border-radius:8px;padding:0.7rem 0.8rem;margin-bottom:0.8rem;background:#fafbfc;">
            <div style="display:flex;align-items:center;gap:0.6rem;margin-bottom:0.55rem;">
                <input type="text" class="form-control form-control-sm fbb-group-target"
                    value="${esc(rg.targetPath)}" placeholder="e.g. identifier"
                    style="flex:0 0 220px;font-family:monospace;font-size:0.82rem;"
                    onchange="window._fhirBuildBuilder && window._fhirBuildBuilder.onGroupTargetChange(${groupIndex}, this.value)">
                <input type="text" class="form-control form-control-sm fbb-group-rowspath" data-search-kind="rowspath" data-group-index="${groupIndex}"
                    value="${esc(rg.rowsPath)}" placeholder="e.g. patientIdentifiers"
                    style="flex:1;font-family:monospace;font-size:0.8rem;">
                <button type="button" class="btn btn-sm btn-outline-danger"
                    onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.removeRepeatingGroup(${groupIndex})"
                    title="Remove group"><i class="fas fa-trash"></i></button>
            </div>
            <div style="font-size:0.7rem;color:#94a3b8;margin-bottom:0.5rem;">
                Target Path: the repeating element on the resource (e.g. <code>identifier</code>).
                Rows Path: pipeline field holding the array of source rows for this group.
            </div>
            <datalist id="${datalistId}">
                ${groupCatalog.map(f => `<option value="${esc(f.key)}">${esc(f.label || f.key)}</option>`).join('')}
            </datalist>
            ${(rg.fields || []).length === 0
                ? `<div style="font-size:0.74rem;color:#94a3b8;margin-bottom:0.4rem;">No fields mapped yet.</div>`
                : this._renderFieldsTableWithDatalist(rg.fields, datalistId, groupIndex, this._validateIssueByRowKey)}
            <button type="button" class="btn btn-sm btn-outline-primary"
                onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.addGroupField(${groupIndex})"
                style="font-size:0.72rem;padding:0.15rem 0.5rem;margin-top:0.4rem;">+ Add Field</button>
        </div>`;
    }

    // _renderFieldsTable hardcodes the datalist id to the shared top-level one;
    // group cards need their own scoped datalist id, so this thin wrapper lets
    // both call sites share the same row-rendering logic without duplicating it.
    _renderFieldsTableWithDatalist(fields, datalistId, groupIndex, issuesByRowKey) {
        return this._renderFieldsTable(fields, datalistId, groupIndex, issuesByRowKey);
    }

    addRepeatingGroup() {
        this._syncDOMToConfig();
        this._step.config.repeatingGroups.push({ targetPath: '', rowsPath: '', fields: [] });
        this._rerender();
    }

    removeRepeatingGroup(index) {
        this._syncDOMToConfig();
        this._step.config.repeatingGroups.splice(index, 1);
        this._rerender();
    }

    onGroupTargetChange(groupIndex, newTarget) {
        this._syncDOMToConfig();
        const rg = this._step.config.repeatingGroups[groupIndex];
        if (!rg) return;
        rg.targetPath = newTarget.trim();
        this._rerender();
    }

    addGroupField(groupIndex) {
        this._syncDOMToConfig();
        const rg = this._step.config.repeatingGroups[groupIndex];
        if (!rg) return;
        if (!Array.isArray(rg.fields)) rg.fields = [];
        rg.fields.push({ targetPath: '', sourcePath: '' });
        this._rerender();
    }

    // ── Resource type / profile change ───────────────────────────────────────

    onResourceTypeChange(newType) {
        this._syncDOMToConfig();
        this._step.config.resourceType = newType;
        this._step.config.profile = 'base';
        this._profiles = [];
        this._loadProfiles(newType);
        this._loadFieldCatalog();
        this._clearValidationResult();
        this._rerender();
    }

    onProfileOrVersionChange() {
        this._syncDOMToConfig();
        this._loadFieldCatalog();
        this._clearValidationResult();
        this._rerender();
    }

    // A validation result/row-badge set from a previous resourceType/profile
    // no longer applies (issue paths were resolved against a different
    // schema) — clear it rather than leave a stale, misleading badge.
    _clearValidationResult() {
        this._validateResult = null;
        this._validateIssueByRowKey = {};
    }

    // ── DOM <-> config sync ───────────────────────────────────────────────────

    _syncDOMToConfig() {
        const root = document.getElementById('fhirBuildBuilder');
        if (!root || !this._step) return;
        const cfg = this._step.config;

        const resourceTypeEl = root.querySelector('#fbbResourceType');
        if (resourceTypeEl) cfg.resourceType = resourceTypeEl.value;
        const profileEl = root.querySelector('#fbbProfile');
        if (profileEl) cfg.profile = profileEl.value;
        const versionEl = root.querySelector('#fbbVersion');
        if (versionEl) cfg.version = versionEl.value.trim() || 'R4';
        const outputEl = root.querySelector('#fbbOutputField');
        if (outputEl) cfg.outputField = outputEl.value.trim() || 'fhirResource';

        const fieldsContainer = root.querySelector('#fbbFieldsContainer');
        if (fieldsContainer) this._syncFieldRows(fieldsContainer, cfg.fields);

        root.querySelectorAll('.fbb-group-card').forEach(card => {
            const gIdx = parseInt(card.dataset.groupIndex, 10);
            const rg = cfg.repeatingGroups[gIdx];
            if (!rg) return;
            const targetEl = card.querySelector('.fbb-group-target');
            if (targetEl) rg.targetPath = targetEl.value.trim();
            const rowsPathEl = card.querySelector('.fbb-group-rowspath');
            if (rowsPathEl) rg.rowsPath = rowsPathEl.value.trim();
            if (!Array.isArray(rg.fields)) rg.fields = [];
            this._syncFieldRows(card, rg.fields);
        });
    }

    _syncFieldRows(container, fields) {
        container.querySelectorAll('tr[data-field-index]').forEach(row => {
            const fIdx = parseInt(row.dataset.fieldIndex, 10);
            const f = fields[fIdx];
            if (!f) return;
            f.targetPath = row.querySelector('.fbb-field-target').value.trim();
            f.sourcePath = row.querySelector('.fbb-field-source').value.trim();
            const fallbackText = row.querySelector('.fbb-field-fallback').value.trim();
            f.fallbackPaths = fallbackText ? fallbackText.split(',').map(s => s.trim()).filter(Boolean) : undefined;
            const literal = row.querySelector('.fbb-field-literal').value.trim();
            f.literalValue = literal || undefined;
            const transformEl = row.querySelector('.fbb-field-transform');
            f.transform = transformEl && transformEl.value ? transformEl.value : undefined;
            const valueMap = FHIRBuildBuilder._parseValueMapText(row.querySelector('.fbb-field-valuemap').value);
            f.valueMap = valueMap && Object.keys(valueMap).length > 0 ? valueMap : undefined;
        });
    }

    _rerender() {
        const root = document.getElementById('fhirBuildBuilder');
        if (!root || !root.parentElement) return;
        this._destroyFieldSearches();
        root.outerHTML = this._renderAll();
        setTimeout(() => this._attachFieldSearches(), 0);
        this._wireTopLevelChangeHandlers();
    }

    _wireTopLevelChangeHandlers() {
        const root = document.getElementById('fhirBuildBuilder');
        if (!root) return;
        const resourceTypeEl = root.querySelector('#fbbResourceType');
        if (resourceTypeEl) resourceTypeEl.onchange = (e) => this.onResourceTypeChange(e.target.value);
        const profileEl = root.querySelector('#fbbProfile');
        if (profileEl) profileEl.onchange = () => this.onProfileOrVersionChange();
        const versionEl = root.querySelector('#fbbVersion');
        if (versionEl) versionEl.onchange = () => this.onProfileOrVersionChange();
    }

    // ── Field path search wiring ──────────────────────────────────────────────

    _attachFieldSearches() {
        this._wireTopLevelChangeHandlers();
        this._initNarrativePicker();
        if (typeof FieldPathSearchComponent === 'undefined') return;
        const root = document.getElementById('fhirBuildBuilder');
        if (!root) return;

        const getStepVars = () => {
            if (this._panel && typeof this._panel.getStepVariablesForSearch === 'function') {
                return this._panel.getStepVariablesForSearch();
            }
            return [];
        };

        root.querySelectorAll('.fbb-field-source, .fbb-group-rowspath').forEach(input => {
            const search = new FieldPathSearchComponent(input, {
                onSelect: (path) => { input.value = path; },
                placeholder: 'Search pipeline fields or enter custom path...',
                allowCustom: true,
                showCategories: true,
                includeHL7Fields: false,
                getStepVariables: getStepVars,
            });
            this._fieldSearches.push(search);
        });
    }

    _destroyFieldSearches() {
        this._fieldSearches.forEach(s => { try { s.destroy(); } catch (e) { /* no-op */ } });
        this._fieldSearches = [];
    }

    // ── Data loading ──────────────────────────────────────────────────────────

    _loadResourceTypes() {
        const version = (this._step.config && this._step.config.version) || 'R4';
        fetch(`/api/fhir/resource-types?version=${encodeURIComponent(version)}`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.resourceTypes) return;
                this._resourceTypes = data.resourceTypes;
                this._rerender();
            })
            .catch(() => {}); // AbortError on destroy is expected
    }

    _loadProfiles(resourceType) {
        if (!resourceType) return;
        const version = (this._step.config && this._step.config.version) || 'R4';
        fetch(`/api/fhir/profiles/${encodeURIComponent(resourceType)}?version=${encodeURIComponent(version)}`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.profiles) return;
                this._profiles = data.profiles;
                this._rerender();
            })
            .catch(() => {});
    }

    _loadFieldCatalog() {
        const cfg = this._step.config;
        if (!cfg.resourceType) return;
        const params = new URLSearchParams({ profile: cfg.profile || 'base', version: cfg.version || 'R4' });
        fetch(`/api/fhir/canonical-fields/${encodeURIComponent(cfg.resourceType)}?${params}`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.fields) return;
                this._fieldCatalog = data.fields;
                this._rerender();
            })
            .catch(() => {});
    }

    _loadTransformCatalog() {
        fetch('/api/fhir/transforms', { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.transforms) return;
                this._transformCatalog = data.transforms;
                this._rerender();
            })
            .catch(() => {});
    }

    // ── collectConfig / destroy ──────────────────────────────────────────────

    collectConfig(step) {
        this._syncDOMToConfig();
        step.config = this._step.config;
    }

    destroy() {
        this._ac.abort();
        this._destroyFieldSearches();
        if (window._fhirBuildBuilder === this) {
            window._fhirBuildBuilder = null;
        }
    }

    // ── Static helpers ────────────────────────────────────────────────────────

    static _esc(s) {
        return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    // "A=active, R=resolved" -> {A: "active", R: "resolved"}
    static _parseValueMapText(text) {
        const map = {};
        String(text || '').split(',').forEach(pair => {
            const eq = pair.indexOf('=');
            if (eq === -1) return;
            const key = pair.slice(0, eq).trim();
            const value = pair.slice(eq + 1).trim();
            if (key) map[key] = value;
        });
        return map;
    }

    static _valueMapToText(valueMap) {
        if (!valueMap || typeof valueMap !== 'object') return '';
        return Object.entries(valueMap).map(([k, v]) => `${k}=${v}`).join(', ');
    }
}

// ── Registration ──────────────────────────────────────────────────────────────

if (typeof StepBuilderRegistry !== 'undefined') {
    StepBuilderRegistry.register('fhir.build', FHIRBuildBuilder);
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FHIRBuildBuilder;
}
