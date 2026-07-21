/**
 * MapToCanonicalBuilder — step config builder for the "cda.map_to_canonical"
 * pipeline step.
 *
 * The no-code, format-agnostic on-ramp for CCD construction: lets a user map
 * CSV columns, DB query columns, or arbitrary JSON fields onto the same
 * canonical USCDI-keyed JSON shape cda.parse and cda.build already share —
 * see services/executors/transform/map_to_canonical_executor.go.
 *
 * Modeled on ResultMappingBuilder.js's proportions (a plain repeatable-row
 * table + add/remove buttons), not CdaToFhirStepBuilder's 4-tab wizard —
 * this step's config is a flat list of {source path -> canonical field}
 * mappings, not a nested resource-construction model.
 *
 * Canonical field vocabularies (both header and section) are fetched live
 * from the backend (cda/builder/canonical_field_catalog.go, exposed via
 * /api/cda/canonical-*), never hardcoded here — the same "single source of
 * truth" discipline CdaToFhirStepBuilder's Section field editor already
 * follows for its own (different) field vocabulary.
 *
 * Builder contract (StepBuilderRegistry):
 *   render(step)         → string  HTML for the properties panel form tab
 *   collectConfig(step)  → void    reads DOM, writes into step.config
 *   destroy()            → void    tears down event listeners / AC refs
 */

class MapToCanonicalBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
        this._step = null;
        this._sectionsCatalog = null;      // [{key, displayName, loincCode, conformance}]
        this._headerCatalog = {};          // group -> [{key, label, dataType}]
        this._sectionFieldCatalog = {};    // sectionKey -> [{key, label, dataType}]
        this._transformCatalog = [];       // [{name, description}]
        this._fieldSearches = [];          // active FieldPathSearchComponent instances
        this._requirements = null;         // GET /api/cda/document-types/:type/requirements response's "requirements" object
        this._requirementsDocType = null;  // documentType this._requirements was fetched for — avoids redundant re-fetches

        window._mapToCanonicalBuilder = this;
    }

    // ── render ────────────────────────────────────────────────────────────────

    render(step) {
        this._step = step;
        if (!step.config) step.config = {};
        this._applyDefaultConfig(step.config);

        this._loadSectionsCatalog();
        this._loadHeaderCatalog('patient');
        this._loadHeaderCatalog('author');
        this._loadTransformCatalog();
        this._loadRequirements(step.config.documentType);

        const html = this._renderAll();
        setTimeout(() => this._attachFieldSearches(), 0);
        return html;
    }

    _applyDefaultConfig(cfg) {
        if (!cfg.outputField) cfg.outputField = 'parsedCDA';
        if (!cfg.documentType) cfg.documentType = 'CCD';
        if (!Array.isArray(cfg.header)) cfg.header = [];
        if (!Array.isArray(cfg.sections)) cfg.sections = [];
    }

    // ── Document Type + requirements-driven guidance ─────────────────────────

    // Fetches the combined requirements catalog for documentType and, once
    // loaded, pre-populates any still-missing SHALL sections/header rows —
    // additive only (never removes or overwrites a row the user already
    // configured, even one that isn't SHALL for the newly-selected document
    // type) so switching Document Type mid-edit can't silently destroy work.
    _loadRequirements(documentType) {
        if (!documentType || this._requirementsDocType === documentType) return;
        fetch(`/api/cda/document-types/${encodeURIComponent(documentType)}/requirements`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.requirements) return;
                this._requirements = data.requirements;
                this._requirementsDocType = documentType;
                this._prepopulateShallRequirements();
                this._rerender();
            })
            .catch(() => {}); // AbortError on destroy is expected
    }

    onDocumentTypeChange(newType) {
        this._syncDOMToConfig();
        this._step.config.documentType = newType;
        this._requirements = null;
        this._requirementsDocType = null;
        this._loadRequirements(newType);
        this._rerender();
    }

    // Auto-adds an empty section card for every SHALL section, and an empty
    // header row for every SHALL patient/author field, not already present
    // in the saved config — same "pre-seeded row, user still supplies the
    // value" pattern CDADedupeStepBuilder already establishes for identity
    // fields. A section/row already present (even unmapped) is left alone —
    // this only ever adds, never removes.
    _prepopulateShallRequirements() {
        if (!this._requirements) return;
        const cfg = this._step.config;

        (this._requirements.sections || []).forEach(sec => {
            if (sec.conformance !== 'SHALL') return;
            if (cfg.sections.some(s => s.sectionKey === sec.key)) return;
            cfg.sections.push({ sectionKey: sec.key, rowsPath: '', fields: [] });
            if (!this._sectionFieldCatalog[sec.key]) {
                // Same "load then rerender once resolved" pattern as
                // onSectionKeyChange — without it the pre-populated card's
                // field datalist would stay empty until an unrelated rerender.
                this._loadSectionFieldCatalog(sec.key).then(() => this._rerender());
            }
        });

        ['patient', 'author'].forEach(group => {
            (this._requirements.headerGroups && this._requirements.headerGroups[group] || []).forEach(field => {
                if (field.conformance !== 'SHALL') return;
                if (cfg.header.some(h => h.group === group && h.target === field.key)) return;
                cfg.header.push({ group, target: field.key, sourcePath: '' });
            });
        });
    }

    _renderAll() {
        const cfg = this._step.config;
        const esc = MapToCanonicalBuilder._esc;
        const missing = (typeof CDARequirementsHelper !== 'undefined')
            ? CDARequirementsHelper.computeMissingShall(this._requirements, cfg.header, cfg.sections)
            : { shallTotal: 0 };

        const docTypeOptions = ['CCD', 'Discharge Summary', 'Referral Note', 'History and Physical', 'Consultation Note', 'Progress Note']
            .map(dt => `<option value="${esc(dt)}" ${cfg.documentType === dt ? 'selected' : ''}>${esc(dt)}</option>`).join('');

        return `
        <div id="mapToCanonicalBuilder" class="cda-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Map to Canonical step.</strong> Maps CSV/DB/JSON fields onto the same canonical JSON shape
                <code>cda.build</code> consumes. Pair the two steps to build a CCD from any source with no new code.
            </div>

            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Document Type</label>
                <select id="m2cDocumentType" class="form-select form-select-sm"
                    onchange="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.onDocumentTypeChange(this.value)">
                    ${docTypeOptions}
                </select>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Determines which sections and header fields are required (SHALL) — match this to the paired cda.build step's own Document Type.</div>
            </div>

            ${typeof CDARequirementsHelper !== 'undefined' ? CDARequirementsHelper.renderCompletenessBanner(missing) : ''}

            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="m2cOutputField" type="text" class="form-control form-control-sm"
                    value="${esc(cfg.outputField)}" placeholder="parsedCDA"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the canonical JSON is written — point cda.build's Source Field at this.</div>
            </div>

            <div class="config-group" style="margin-bottom:1.3rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.5rem;">Header Fields</label>
                ${this._renderHeaderGroup('patient', 'Patient')}
                ${this._renderHeaderGroup('author', 'Author')}
            </div>

            <div class="config-group">
                <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.5rem;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;margin:0;">Sections</label>
                    <button type="button" class="btn btn-sm btn-primary"
                        onclick="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.addSection()">
                        <i class="fas fa-plus"></i> Add Section
                    </button>
                </div>
                <div id="m2cSectionsContainer">
                    ${cfg.sections.length === 0
                        ? `<div style="text-align:center;padding:1.5rem;color:#94a3b8;font-size:0.8rem;border:1px dashed #cbd5e1;border-radius:6px;">No sections configured yet. Click "Add Section" to map rows into a canonical section.</div>`
                        : cfg.sections.map((sec, i) => this._renderSectionCard(sec, i)).join('')}
                </div>
            </div>
        </div>`;
    }

    // Shared <option> list for every Transform <select> — same catalog
    // regardless of header vs. section field, since canonical_value_transforms.go
    // is a single flat registry (not scoped per section like the canonical
    // field catalogs are).
    _renderTransformOptions(selectedName) {
        const esc = MapToCanonicalBuilder._esc;
        const opts = this._transformCatalog.map(t =>
            `<option value="${esc(t.name)}" title="${esc(t.description)}" ${t.name === selectedName ? 'selected' : ''}>${esc(t.name)}</option>`
        ).join('');
        return `<option value="">— none —</option>${opts}`;
    }

    // ── Header groups ────────────────────────────────────────────────────────

    _renderHeaderGroup(group, label) {
        const esc = MapToCanonicalBuilder._esc;
        const cfg = this._step.config;
        const rows = cfg.header.filter(h => h.group === group);
        const catalog = this._headerCatalog[group] || [];
        const datalistId = `m2cHeaderTargets-${group}`;
        const requirementByKey = {};
        ((this._requirements && this._requirements.headerGroups && this._requirements.headerGroups[group]) || [])
            .forEach(f => { requirementByKey[f.key] = f; });

        const rowsHtml = rows.map(h => {
            const globalIndex = cfg.header.indexOf(h);
            const req = requirementByKey[h.target];
            return `
            <tr data-header-index="${globalIndex}">
                <td>
                    <input type="text" class="form-control form-control-sm m2c-header-target"
                        value="${esc(h.target)}" list="${datalistId}" placeholder="e.g. firstName" autocomplete="off">
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm m2c-header-source"
                        value="${esc(h.sourcePath)}" placeholder="e.g. patientFirstName" autocomplete="off">
                </td>
                <td>
                    <select class="form-select form-select-sm m2c-header-transform" style="font-size:0.78rem;">
                        ${this._renderTransformOptions(h.transform)}
                    </select>
                </td>
                <td style="width:90px;">${req && typeof CDARequirementsHelper !== 'undefined' ? CDARequirementsHelper.renderConformanceBadge(req.conformance) : ''}</td>
                <td style="width:44px;text-align:center;">
                    <button type="button" class="btn btn-sm btn-danger"
                        onclick="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.removeHeaderRow(${globalIndex})"
                        title="Remove"><i class="fas fa-trash"></i></button>
                </td>
            </tr>`;
        }).join('');

        return `
        <div style="margin-bottom:0.9rem;border:1px solid #e2e8f0;border-radius:6px;padding:0.6rem 0.7rem;">
            <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.4rem;">
                <span style="font-size:0.78rem;font-weight:600;color:#334155;">${esc(label)}</span>
                <button type="button" class="btn btn-sm btn-outline-primary"
                    onclick="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.addHeaderRow('${group}')"
                    style="font-size:0.72rem;padding:0.15rem 0.5rem;">+ Add Field</button>
            </div>
            <datalist id="${datalistId}">
                ${catalog.map(f => {
                    const req = requirementByKey[f.key];
                    const suffix = req ? ` (${req.conformance === 'SHALL' ? 'required' : 'recommended'})` : '';
                    return `<option value="${esc(f.key)}">${esc(f.label || f.key)}${esc(suffix)}</option>`;
                }).join('')}
            </datalist>
            ${rows.length === 0
                ? `<div style="font-size:0.74rem;color:#94a3b8;">No ${esc(label.toLowerCase())} fields mapped.</div>`
                : `<table class="mapping-table" style="width:100%;">
                    <thead><tr>
                        <th style="font-size:0.7rem;color:#64748b;">Target Field</th>
                        <th style="font-size:0.7rem;color:#64748b;">Source Path</th>
                        <th style="font-size:0.7rem;color:#64748b;">Transform</th>
                        <th style="font-size:0.7rem;color:#64748b;">Requirement</th>
                        <th></th>
                    </tr></thead>
                    <tbody>${rowsHtml}</tbody>
                   </table>`}
        </div>`;
    }

    addHeaderRow(group) {
        this._syncDOMToConfig();
        this._step.config.header.push({ group, target: '', sourcePath: '' });
        this._rerender();
    }

    removeHeaderRow(globalIndex) {
        this._syncDOMToConfig();
        this._step.config.header.splice(globalIndex, 1);
        this._rerender();
    }

    // ── Sections ──────────────────────────────────────────────────────────────

    _renderSectionCard(sec, sectionIndex) {
        const esc = MapToCanonicalBuilder._esc;
        const sectionOptions = (this._sectionsCatalog || [])
            .map(s => `<option value="${esc(s.key)}" ${s.key === sec.sectionKey ? 'selected' : ''}>${esc(s.displayName || s.key)}</option>`)
            .join('');
        const fieldCatalog = this._sectionFieldCatalog[sec.sectionKey] || [];
        const fieldDatalistId = `m2cFieldTargets-${sectionIndex}`;
        const docTypeSectionReq = (this._requirements && this._requirements.sections || []).find(s => s.key === sec.sectionKey);
        const sectionBadge = docTypeSectionReq && typeof CDARequirementsHelper !== 'undefined'
            ? CDARequirementsHelper.renderConformanceBadge(docTypeSectionReq.conformance) : '';

        const fieldsHtml = (sec.fields || []).map((f, fi) => `
            <tr data-field-index="${fi}">
                <td>
                    <input type="text" class="form-control form-control-sm m2c-field-canonical"
                        value="${esc(f.canonicalField)}" list="${fieldDatalistId}" placeholder="e.g. conditionCode" autocomplete="off">
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm m2c-field-source"
                        value="${esc(f.sourcePath)}" placeholder="e.g. icd10Code" autocomplete="off">
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm m2c-field-fallback"
                        value="${esc((f.fallbackPaths || []).join(', '))}" placeholder="alt1, alt2" autocomplete="off"
                        title="Comma-separated fallback source paths, tried in order if Source Path is empty">
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm m2c-field-literal"
                        value="${esc(f.literalValue)}" placeholder="(fixed value)" autocomplete="off"
                        title="Used only when no source/fallback path resolves">
                </td>
                <td>
                    <select class="form-select form-select-sm m2c-field-transform" style="font-size:0.78rem;"
                        title="Applied to the resolved value BEFORE Value Map">
                        ${this._renderTransformOptions(f.transform)}
                    </select>
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm m2c-field-valuemap"
                        value="${esc(MapToCanonicalBuilder._valueMapToText(f.valueMap))}" placeholder="A=active, R=resolved" autocomplete="off"
                        title="Comma-separated raw=canonical value translations, applied AFTER Transform">
                </td>
                <td style="width:40px;text-align:center;">
                    <button type="button" class="btn btn-sm btn-danger"
                        onclick="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.removeSectionField(${sectionIndex}, ${fi})"
                        title="Remove field"><i class="fas fa-trash"></i></button>
                </td>
            </tr>`).join('');

        return `
        <div class="m2c-section-card" data-section-index="${sectionIndex}"
            style="border:1px solid #e2e8f0;border-radius:8px;padding:0.7rem 0.8rem;margin-bottom:0.8rem;background:#fafbfc;">
            <div style="display:flex;align-items:center;gap:0.6rem;margin-bottom:0.55rem;">
                <select class="form-select form-select-sm m2c-section-key" style="flex:0 0 240px;"
                    onchange="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.onSectionKeyChange(${sectionIndex}, this.value)">
                    <option value="">— select section —</option>
                    ${sectionOptions}
                </select>
                <input type="text" class="form-control form-control-sm m2c-rows-path" data-search-kind="rowspath" data-section-index="${sectionIndex}"
                    value="${esc(sec.rowsPath)}" placeholder="e.g. records or rows"
                    style="flex:1;font-family:monospace;font-size:0.8rem;">
                ${sectionBadge}
                <button type="button" class="btn btn-sm btn-outline-danger"
                    onclick="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.removeSection(${sectionIndex})"
                    title="Remove section"><i class="fas fa-trash"></i></button>
            </div>
            <div style="font-size:0.7rem;color:#94a3b8;margin-bottom:0.5rem;">Rows Path: pipeline field holding the array of source rows for this section (e.g. a File Parser step's "records" or a Database Enrichment step's "rows").</div>
            <datalist id="${fieldDatalistId}">
                ${fieldCatalog.map(f => `<option value="${esc(f.key)}">${esc(f.label || f.key)}</option>`).join('')}
            </datalist>
            ${(sec.fields || []).length === 0
                ? `<div style="font-size:0.74rem;color:#94a3b8;margin-bottom:0.4rem;">No fields mapped yet.</div>`
                : `<div style="overflow-x:auto;">
                    <table class="mapping-table" style="width:100%;min-width:760px;">
                        <thead><tr>
                            <th style="font-size:0.68rem;color:#64748b;">Canonical Field</th>
                            <th style="font-size:0.68rem;color:#64748b;">Source Path</th>
                            <th style="font-size:0.68rem;color:#64748b;">Fallback Paths</th>
                            <th style="font-size:0.68rem;color:#64748b;">Literal</th>
                            <th style="font-size:0.68rem;color:#64748b;">Transform</th>
                            <th style="font-size:0.68rem;color:#64748b;">Value Map</th>
                            <th></th>
                        </tr></thead>
                        <tbody>${fieldsHtml}</tbody>
                    </table>
                   </div>`}
            <button type="button" class="btn btn-sm btn-outline-primary"
                onclick="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.addSectionField(${sectionIndex})"
                style="font-size:0.72rem;padding:0.15rem 0.5rem;margin-top:0.4rem;">+ Add Field</button>
        </div>`;
    }

    addSection() {
        this._syncDOMToConfig();
        this._step.config.sections.push({ sectionKey: '', rowsPath: '', fields: [] });
        this._rerender();
    }

    removeSection(index) {
        this._syncDOMToConfig();
        this._step.config.sections.splice(index, 1);
        this._rerender();
    }

    onSectionKeyChange(sectionIndex, newKey) {
        this._syncDOMToConfig();
        const sec = this._step.config.sections[sectionIndex];
        if (!sec) return;
        sec.sectionKey = newKey;
        if (!newKey || this._sectionFieldCatalog[newKey]) {
            this._rerender();
            return;
        }
        this._loadSectionFieldCatalog(newKey).then(() => this._rerender());
    }

    addSectionField(sectionIndex) {
        this._syncDOMToConfig();
        const sec = this._step.config.sections[sectionIndex];
        if (!sec) return;
        if (!Array.isArray(sec.fields)) sec.fields = [];
        sec.fields.push({ canonicalField: '', sourcePath: '' });
        this._rerender();
    }

    removeSectionField(sectionIndex, fieldIndex) {
        this._syncDOMToConfig();
        const sec = this._step.config.sections[sectionIndex];
        if (!sec || !Array.isArray(sec.fields)) return;
        sec.fields.splice(fieldIndex, 1);
        this._rerender();
    }

    // ── DOM <-> config sync ───────────────────────────────────────────────────

    _syncDOMToConfig() {
        const root = document.getElementById('mapToCanonicalBuilder');
        if (!root || !this._step) return;
        const cfg = this._step.config;

        const outputEl = root.querySelector('#m2cOutputField');
        if (outputEl) cfg.outputField = outputEl.value.trim() || 'parsedCDA';

        root.querySelectorAll('tr[data-header-index]').forEach(row => {
            const idx = parseInt(row.dataset.headerIndex, 10);
            const h = cfg.header[idx];
            if (!h) return;
            h.target = row.querySelector('.m2c-header-target').value.trim();
            h.sourcePath = row.querySelector('.m2c-header-source').value.trim();
            const transformEl = row.querySelector('.m2c-header-transform');
            h.transform = transformEl && transformEl.value ? transformEl.value : undefined;
        });

        root.querySelectorAll('.m2c-section-card').forEach(card => {
            const sIdx = parseInt(card.dataset.sectionIndex, 10);
            const sec = cfg.sections[sIdx];
            if (!sec) return;
            const rowsPathEl = card.querySelector('.m2c-rows-path');
            if (rowsPathEl) sec.rowsPath = rowsPathEl.value.trim();

            card.querySelectorAll('tr[data-field-index]').forEach(row => {
                const fIdx = parseInt(row.dataset.fieldIndex, 10);
                const f = sec.fields[fIdx];
                if (!f) return;
                f.canonicalField = row.querySelector('.m2c-field-canonical').value.trim();
                f.sourcePath = row.querySelector('.m2c-field-source').value.trim();
                const fallbackText = row.querySelector('.m2c-field-fallback').value.trim();
                f.fallbackPaths = fallbackText ? fallbackText.split(',').map(s => s.trim()).filter(Boolean) : undefined;
                const literal = row.querySelector('.m2c-field-literal').value.trim();
                f.literalValue = literal || undefined;
                const transformEl = row.querySelector('.m2c-field-transform');
                f.transform = transformEl && transformEl.value ? transformEl.value : undefined;
                const valueMap = MapToCanonicalBuilder._parseValueMapText(row.querySelector('.m2c-field-valuemap').value);
                f.valueMap = valueMap && Object.keys(valueMap).length > 0 ? valueMap : undefined;
            });
        });
    }

    _rerender() {
        const root = document.getElementById('mapToCanonicalBuilder');
        if (!root || !root.parentElement) return;
        this._destroyFieldSearches();
        root.outerHTML = this._renderAll();
        setTimeout(() => this._attachFieldSearches(), 0);
    }

    // ── Field path search wiring ──────────────────────────────────────────────

    _attachFieldSearches() {
        if (typeof FieldPathSearchComponent === 'undefined') return;
        const root = document.getElementById('mapToCanonicalBuilder');
        if (!root) return;

        const getStepVars = () => {
            if (this._panel && typeof this._panel.getStepVariablesForSearch === 'function') {
                return this._panel.getStepVariablesForSearch();
            }
            return [];
        };

        root.querySelectorAll('.m2c-header-source').forEach(input => {
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

        root.querySelectorAll('.m2c-rows-path').forEach(input => {
            const search = new FieldPathSearchComponent(input, {
                onSelect: (path) => { input.value = path; },
                placeholder: 'Search pipeline fields (e.g. records, rows)...',
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

    _loadSectionsCatalog() {
        fetch('/api/cda/canonical-sections', { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.sections) return;
                this._sectionsCatalog = data.sections;
                this._rerender();
            })
            .catch(() => {}); // AbortError on destroy is expected
    }

    _loadHeaderCatalog(group) {
        fetch(`/api/cda/canonical-fields/header/${encodeURIComponent(group)}`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.fields) return;
                this._headerCatalog[group] = data.fields;
                this._rerender();
            })
            .catch(() => {});
    }

    _loadSectionFieldCatalog(sectionKey) {
        return fetch(`/api/cda/canonical-fields/section/${encodeURIComponent(sectionKey)}`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (data && data.fields) this._sectionFieldCatalog[sectionKey] = data.fields;
            })
            .catch(() => {});
    }

    _loadTransformCatalog() {
        fetch('/api/cda/canonical-transforms', { signal: this._ac.signal })
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
        if (window._mapToCanonicalBuilder === this) {
            window._mapToCanonicalBuilder = null;
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
    StepBuilderRegistry.register('cda.map_to_canonical', MapToCanonicalBuilder);
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = MapToCanonicalBuilder;
}
