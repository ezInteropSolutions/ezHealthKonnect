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

        return `
        <div id="fhirBuildBuilder" class="cda-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>FHIR Resource Builder step.</strong> Maps CSV/DB/JSON fields directly onto one FHIR R4 resource.
                Add one step per resource type, then use a Payload Builder (FHIR Bundle mode) step to assemble a Bundle.
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
                            onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.toggleSuggestPanel()"
                            style="font-size:0.72rem;padding:0.15rem 0.5rem;">✨ Suggest Mappings</button>
                        <button type="button" class="btn btn-sm btn-outline-primary"
                            onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.addField()"
                            style="font-size:0.72rem;padding:0.15rem 0.5rem;">+ Add Field</button>
                    </div>
                </div>
                ${this._renderSuggestPanel()}
                <datalist id="fbbFieldTargets">
                    ${this._fieldCatalog.map(f => `<option value="${esc(f.key)}">${esc(f.label || f.key)}${f.required ? ' (required)' : ''}</option>`).join('')}
                </datalist>
                <div style="font-size:0.7rem;color:#94a3b8;margin-bottom:0.5rem;">Flat/single-value element paths, e.g. <code>birthDate</code>, <code>identifier[0].value</code>.</div>
                <div id="fbbFieldsContainer">
                    ${cfg.fields.length === 0
                        ? `<div style="text-align:center;padding:1rem;color:#94a3b8;font-size:0.8rem;border:1px dashed #cbd5e1;border-radius:6px;">No fields mapped yet.</div>`
                        : this._renderFieldsTable(cfg.fields, 'fbbFieldTargets', -1)}
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
        </div>`;
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

    _renderFieldsTable(fields, datalistId, groupIndex) {
        const esc = FHIRBuildBuilder._esc;
        const rowsHtml = fields.map((f, fi) => `
            <tr data-field-index="${fi}">
                <td>
                    <input type="text" class="form-control form-control-sm fbb-field-target"
                        value="${esc(f.targetPath)}" list="${datalistId}" placeholder="e.g. birthDate" autocomplete="off">
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
            </tr>`).join('');

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
                : this._renderFieldsTableWithDatalist(rg.fields, datalistId, groupIndex)}
            <button type="button" class="btn btn-sm btn-outline-primary"
                onclick="window._fhirBuildBuilder && window._fhirBuildBuilder.addGroupField(${groupIndex})"
                style="font-size:0.72rem;padding:0.15rem 0.5rem;margin-top:0.4rem;">+ Add Field</button>
        </div>`;
    }

    // _renderFieldsTable hardcodes the datalist id to the shared top-level one;
    // group cards need their own scoped datalist id, so this thin wrapper lets
    // both call sites share the same row-rendering logic without duplicating it.
    _renderFieldsTableWithDatalist(fields, datalistId, groupIndex) {
        return this._renderFieldsTable(fields, datalistId, groupIndex);
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
        this._rerender();
    }

    onProfileOrVersionChange() {
        this._syncDOMToConfig();
        this._loadFieldCatalog();
        this._rerender();
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
