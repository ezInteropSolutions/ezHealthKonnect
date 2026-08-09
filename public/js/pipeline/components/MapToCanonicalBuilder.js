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
        this._repeatingGroupCatalog = {};  // sectionKey -> {key, fields:[...]} | null (fetched, section has none) — key absent means not-yet-fetched
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
        // Sections already present in a loaded/saved config (as opposed to
        // ones the user adds/changes THIS session via onSectionKeyChange, or
        // ones _prepopulateShallRequirements just pushed) never had their
        // per-section catalogs fetched otherwise — without this, reopening a
        // saved step hides the Group By box/Entry-Level Fields table for any
        // already-configured RepeatingGroup section (e.g. Results, Vital
        // Signs) even though sec.groupBy/sec.entryFields are still intact.
        this._ensureSectionCatalogsLoaded();

        const html = this._renderAll();
        setTimeout(() => this._attachFieldSearches(), 0);
        return html;
    }

    // Fetches the per-section field catalog + RepeatingGroup catalog for
    // every section currently in the config that doesn't have them cached
    // yet — the single shared loader both initial render() (pre-existing
    // sections) and _prepopulateShallRequirements() (newly-pushed sections)
    // delegate to, so there's one place that knows how to bring a section's
    // catalogs up to date rather than two divergent copies of the same logic.
    _ensureSectionCatalogsLoaded() {
        const cfg = this._step.config;
        (cfg.sections || []).forEach(sec => {
            if (!sec.sectionKey) return;
            const loaders = [];
            if (!this._sectionFieldCatalog[sec.sectionKey]) loaders.push(this._loadSectionFieldCatalog(sec.sectionKey));
            if (!(sec.sectionKey in this._repeatingGroupCatalog)) loaders.push(this._loadSectionRepeatingGroupCatalog(sec.sectionKey));
            if (loaders.length > 0) Promise.all(loaders).then(() => this._rerender());
        });
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
        });

        ['patient', 'author'].forEach(group => {
            (this._requirements.headerGroups && this._requirements.headerGroups[group] || []).forEach(field => {
                if (field.conformance !== 'SHALL') return;
                if (cfg.header.some(h => h.group === group && h.target === field.key)) return;
                cfg.header.push({ group, target: field.key, sourcePath: '' });
            });
        });

        // Covers both sections just pushed above AND — since this also runs
        // on every fresh render() — any section already present in a loaded
        // config whose catalogs render() itself hasn't resolved yet.
        this._ensureSectionCatalogsLoaded();
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
                    <input type="text" class="form-control form-control-sm m2c-header-literal"
                        value="${esc(h.literalValue)}" placeholder="(fixed value)" autocomplete="off"
                        title="Used only when Source Path is empty — a constant written regardless of per-message data (e.g. a coded field's own codeSystem OID)">
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
                        <th style="font-size:0.7rem;color:#64748b;">Literal Value</th>
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
        const entryCatalog = this._sectionFieldCatalog[sec.sectionKey] || [];
        const docTypeSectionReq = (this._requirements && this._requirements.sections || []).find(s => s.key === sec.sectionKey);
        const sectionBadge = docTypeSectionReq && typeof CDARequirementsHelper !== 'undefined'
            ? CDARequirementsHelper.renderConformanceBadge(docTypeSectionReq.conformance) : '';

        // repeatingGroup is undefined until fetched, null once confirmed the
        // section has none (the vast majority) — either way, no Group By UI.
        // Only a section actually declaring one in the schema (e.g. Vital
        // Signs' "components") ever shows it, matching cda/builder's own
        // "vast majority of sections are unaffected" design.
        const repeatingGroup = this._repeatingGroupCatalog[sec.sectionKey];
        const isGrouped = !!repeatingGroup && Array.isArray(sec.groupBy) && sec.groupBy.length > 0;
        // The per-item Fields table's own datalist switches to the
        // RepeatingGroup's per-item vocabulary once grouped (e.g. Vital
        // Signs' vitalCode/value) — the entry-level catalog no longer
        // describes what one row/item maps to once rows are being bucketed.
        const itemCatalog = isGrouped ? repeatingGroup.fields : entryCatalog;

        const groupByHtml = repeatingGroup ? `
            <div style="margin-bottom:0.5rem;padding:0.5rem 0.6rem;background:#f5f3ff;border:1px solid #ddd6fe;border-radius:6px;">
                <label style="font-size:0.72rem;font-weight:600;color:#5b21b6;display:block;margin-bottom:0.25rem;">
                    Group rows by <span style="font-weight:400;color:#7c6aae;">(comma-separated source columns — repeats matching rows into one entry's "${esc(repeatingGroup.key)}")</span>
                </label>
                <input type="text" class="form-control form-control-sm m2c-section-groupby"
                    value="${esc((sec.groupBy || []).join(', '))}" placeholder="e.g. panelId or encounterId, panelId"
                    style="font-family:monospace;font-size:0.8rem;">
            </div>` : '';

        const itemFieldsSection = this._renderFieldMappingTable(sectionIndex, sec.fields, itemCatalog, 'item', {
            title: isGrouped ? `Per-Item Fields (one row per "${esc(repeatingGroup.key)}" entry)` : null,
            emptyText: 'No fields mapped yet.',
            addLabel: '+ Add Field',
        });

        const entryFieldsSection = isGrouped ? this._renderFieldMappingTable(sectionIndex, sec.entryFields, entryCatalog, 'entry', {
            title: 'Entry-Level Fields (resolved once per group, from the group\'s first row)',
            emptyText: 'No entry-level fields mapped (e.g. the organizer\'s own effectiveTime).',
            addLabel: '+ Add Entry Field',
        }) : '';

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
            <div style="display:flex;align-items:center;gap:0.6rem;margin-bottom:0.5rem;">
                <label style="font-size:0.7rem;color:#94a3b8;white-space:nowrap;" title="Advanced — only needed when this section has more than one entry shape (e.g. Medical Equipment's alternate Procedure entries). Add a SECOND section card for the same section, give it its own Rows Path, and set its Entries Key to match the alternate shape's key. Leave blank for the ordinary case.">Entries Key (advanced)</label>
                <input type="text" class="form-control form-control-sm m2c-entries-key" data-section-index="${sectionIndex}"
                    value="${esc(sec.entriesKey || '')}" placeholder="entries (default)"
                    style="flex:0 0 200px;font-family:monospace;font-size:0.78rem;">
            </div>
            ${groupByHtml}
            ${itemFieldsSection}
            ${entryFieldsSection}
        </div>`;
    }

    // Shared renderer for BOTH the per-item Fields table and the (Group By
    // only) once-per-group Entry Fields table — identical row shape/columns,
    // different target array (sec.fields vs sec.entryFields), different
    // canonical-field catalog, and a distinct kind ('item'|'entry') stamped
    // onto each row so _syncDOMToConfig knows which array to write back to.
    //
    // For kind==='item' only, each row may independently be switched into a
    // "related rows join" (Block 3, Option C) instead of a scalar mapping —
    // a FIELD-level choice, not a section-level one like Group By, since a
    // section can mix ordinary scalar fields with a joined repeating field
    // (e.g. Medications: drugCode/status are scalar, indications is a join).
    _renderFieldMappingTable(sectionIndex, fields, catalog, kind, opts) {
        const esc = MapToCanonicalBuilder._esc;
        fields = fields || [];
        const datalistId = `m2cFieldTargets-${kind}-${sectionIndex}`;
        const repeatingGroup = this._repeatingGroupCatalog[this._step.config.sections[sectionIndex].sectionKey] || null;
        const joinCatalog = (repeatingGroup && repeatingGroup.fields) || [];
        const allowJoinMode = kind === 'item';

        const rowsHtml = fields.map((f, fi) => {
            if (allowJoinMode && f.relatedRows) {
                return this._renderRelatedRowsFieldRow(sectionIndex, fi, f, joinCatalog, datalistId);
            }
            return `
            <tr data-field-index="${fi}" data-field-kind="${kind}">
                <td>
                    <input type="text" class="form-control form-control-sm m2c-field-canonical"
                        value="${esc(f.canonicalField)}" list="${datalistId}" placeholder="e.g. conditionCode" autocomplete="off">
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
                <td style="width:70px;text-align:center;white-space:nowrap;">
                    ${allowJoinMode ? `<button type="button" class="btn btn-sm btn-outline-secondary" title="Attach rows from a different pipeline field (cross-table join)"
                        onclick="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.toggleFieldJoinMode(${sectionIndex}, ${fi}, true)"
                        style="font-size:0.7rem;padding:0.1rem 0.35rem;margin-right:0.25rem;"><i class="fas fa-link"></i></button>` : ''}
                    <button type="button" class="btn btn-sm btn-danger"
                        onclick="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.removeSectionField(${sectionIndex}, ${fi}, '${kind}')"
                        title="Remove field"><i class="fas fa-trash"></i></button>
                </td>
            </tr>`;
        }).join('');

        return `
            ${opts.title ? `<div style="font-size:0.72rem;font-weight:600;color:#475569;margin:0.5rem 0 0.3rem;">${esc(opts.title)}</div>` : ''}
            <datalist id="${datalistId}">
                ${catalog.map(f => `<option value="${esc(f.key)}">${esc(f.label || f.key)}</option>`).join('')}
                ${allowJoinMode && repeatingGroup && !catalog.some(f => f.key === repeatingGroup.key)
                    ? `<option value="${esc(repeatingGroup.key)}">${esc(repeatingGroup.key)} (repeating — use "Attach related rows" for this one)</option>`
                    : ''}
            </datalist>
            ${fields.length === 0
                ? `<div style="font-size:0.74rem;color:#94a3b8;margin-bottom:0.4rem;">${esc(opts.emptyText)}</div>`
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
                        <tbody>${rowsHtml}</tbody>
                    </table>
                   </div>`}
            <button type="button" class="btn btn-sm btn-outline-primary"
                onclick="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.addSectionField(${sectionIndex}, '${kind}')"
                style="font-size:0.72rem;padding:0.15rem 0.5rem;margin-top:0.4rem;">${esc(opts.addLabel)}</button>`;
    }

    // Renders one field row in "related rows join" mode (Block 3, Option C)
    // — a single wide card spanning the table's columns instead of the
    // normal per-column scalar cells, since a join needs 3 extra inputs
    // (relatedRowsPath/joinLocalKey/joinRelatedKey) plus its own nested
    // per-matched-row field table, which don't fit the scalar row shape.
    _renderRelatedRowsFieldRow(sectionIndex, fieldIndex, f, joinCatalog, datalistId) {
        const esc = MapToCanonicalBuilder._esc;
        const rr = f.relatedRows || {};
        const nestedTable = this._renderFieldMappingTable(sectionIndex, rr.fields, joinCatalog, `related-${fieldIndex}`, {
            title: null,
            emptyText: 'No fields mapped for the joined rows yet.',
            addLabel: '+ Add Joined Field',
        });

        return `
        <tr data-field-index="${fieldIndex}" data-field-kind="item" data-field-relatedrows="1">
            <td colspan="7">
                <div style="border:1px solid #bfdbfe;background:#eff6ff;border-radius:6px;padding:0.55rem 0.65rem;">
                    <div style="display:flex;align-items:center;gap:0.5rem;margin-bottom:0.45rem;">
                        <i class="fas fa-link" style="color:#1e40af;font-size:0.75rem;"></i>
                        <span style="font-size:0.72rem;font-weight:600;color:#1e40af;">Attach related rows from another pipeline field</span>
                        <div style="flex:1;"></div>
                        <button type="button" class="btn btn-sm btn-outline-secondary"
                            onclick="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.toggleFieldJoinMode(${sectionIndex}, ${fieldIndex}, false)"
                            style="font-size:0.7rem;padding:0.1rem 0.4rem;">Switch to scalar field</button>
                        <button type="button" class="btn btn-sm btn-danger"
                            onclick="window._mapToCanonicalBuilder && window._mapToCanonicalBuilder.removeSectionField(${sectionIndex}, ${fieldIndex}, 'item')"
                            style="font-size:0.7rem;padding:0.1rem 0.4rem;"><i class="fas fa-trash"></i></button>
                    </div>
                    <div style="display:grid;grid-template-columns:repeat(4,1fr);gap:0.5rem;margin-bottom:0.5rem;">
                        <div>
                            <label style="font-size:0.68rem;color:#64748b;display:block;">Canonical Field</label>
                            <input type="text" class="form-control form-control-sm m2c-relatedrows-canonical"
                                value="${esc(f.canonicalField)}" list="${datalistId}" placeholder="e.g. indications" autocomplete="off">
                        </div>
                        <div>
                            <label style="font-size:0.68rem;color:#64748b;display:block;">Related Rows Path</label>
                            <input type="text" class="form-control form-control-sm m2c-relatedrows-path"
                                value="${esc(rr.relatedRowsPath)}" placeholder="e.g. indicationRecords" autocomplete="off"
                                style="font-family:monospace;font-size:0.78rem;">
                        </div>
                        <div>
                            <label style="font-size:0.68rem;color:#64748b;display:block;">Join Local Key <span title="Column on THIS row">ⓘ</span></label>
                            <input type="text" class="form-control form-control-sm m2c-relatedrows-localkey"
                                value="${esc(rr.joinLocalKey)}" placeholder="e.g. medicationId" autocomplete="off">
                        </div>
                        <div>
                            <label style="font-size:0.68rem;color:#64748b;display:block;">Join Related Key <span title="Column on the OTHER array's rows">ⓘ</span></label>
                            <input type="text" class="form-control form-control-sm m2c-relatedrows-relatedkey"
                                value="${esc(rr.joinRelatedKey)}" placeholder="e.g. medicationId" autocomplete="off">
                        </div>
                    </div>
                    <div style="font-size:0.68rem;color:#64748b;margin-bottom:0.3rem;">Every row in Related Rows Path whose Join Related Key matches this row's own Join Local Key becomes one item.</div>
                    ${nestedTable}
                </div>
            </td>
        </tr>`;
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
        if (!newKey) {
            this._rerender();
            return;
        }
        const loaders = [];
        if (!this._sectionFieldCatalog[newKey]) loaders.push(this._loadSectionFieldCatalog(newKey));
        if (!(newKey in this._repeatingGroupCatalog)) loaders.push(this._loadSectionRepeatingGroupCatalog(newKey));
        if (loaders.length === 0) {
            this._rerender();
            return;
        }
        Promise.all(loaders).then(() => this._rerender());
    }

    // Resolves the actual fields array a given kind addresses: 'entry' ->
    // sec.entryFields, 'item' -> sec.fields, 'related-<N>' -> the Nth item
    // field's OWN relatedRows.fields (a nested join's joined-field table —
    // see _renderRelatedRowsFieldRow). Creates intermediate arrays/objects
    // as needed so add/remove never has to special-case a not-yet-existing
    // relatedRows on a freshly-toggled join field.
    _resolveFieldsArray(sec, kind) {
        if (kind === 'entry') {
            if (!Array.isArray(sec.entryFields)) sec.entryFields = [];
            return sec.entryFields;
        }
        if (typeof kind === 'string' && kind.startsWith('related-')) {
            const parentIdx = parseInt(kind.slice('related-'.length), 10);
            const parentField = sec.fields && sec.fields[parentIdx];
            if (!parentField) return null;
            if (!parentField.relatedRows) parentField.relatedRows = { fields: [] };
            if (!Array.isArray(parentField.relatedRows.fields)) parentField.relatedRows.fields = [];
            return parentField.relatedRows.fields;
        }
        if (!Array.isArray(sec.fields)) sec.fields = [];
        return sec.fields;
    }

    addSectionField(sectionIndex, kind) {
        this._syncDOMToConfig();
        const sec = this._step.config.sections[sectionIndex];
        if (!sec) return;
        const arr = this._resolveFieldsArray(sec, kind);
        if (!arr) return;
        arr.push({ canonicalField: '', sourcePath: '' });
        this._rerender();
    }

    removeSectionField(sectionIndex, fieldIndex, kind) {
        this._syncDOMToConfig();
        const sec = this._step.config.sections[sectionIndex];
        if (!sec) return;
        const arr = this._resolveFieldsArray(sec, kind);
        if (!arr) return;
        arr.splice(fieldIndex, 1);
        this._rerender();
    }

    // Switches one 'item'-kind field between scalar and "related rows join"
    // (Block 3, Option C) shape. Sync first so any in-progress edits on
    // OTHER fields aren't lost by the rerender this triggers. Toggling to
    // join mode seeds an empty relatedRows object (not just {}) so the
    // render path's `f.relatedRows` check has real fields to work with;
    // toggling back to scalar simply deletes it — canonicalField is kept
    // either way, everything else starts fresh (a join's
    // path/keys/fields aren't meaningful leftovers for a scalar field, and
    // vice versa).
    toggleFieldJoinMode(sectionIndex, fieldIndex, makeJoin) {
        this._syncDOMToConfig();
        const sec = this._step.config.sections[sectionIndex];
        const f = sec && sec.fields && sec.fields[fieldIndex];
        if (!f) return;
        if (makeJoin) {
            f.relatedRows = { relatedRowsPath: '', joinLocalKey: '', joinRelatedKey: '', fields: [] };
            // Clear the scalar-shape leftovers from before the toggle —
            // Go's applyFieldMapping ignores them once RelatedRows is set,
            // but leaving them in the saved config would misrepresent user
            // intent (they never configured a fallback/literal for this
            // field, it just briefly existed as an empty scalar row).
            delete f.sourcePath;
            delete f.fallbackPaths;
            delete f.literalValue;
            delete f.transform;
            delete f.valueMap;
            delete f.condition;
        } else {
            delete f.relatedRows;
        }
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
            const literalEl = row.querySelector('.m2c-header-literal');
            h.literalValue = literalEl && literalEl.value.trim() ? literalEl.value.trim() : undefined;
            const transformEl = row.querySelector('.m2c-header-transform');
            h.transform = transformEl && transformEl.value ? transformEl.value : undefined;
        });

        root.querySelectorAll('.m2c-section-card').forEach(card => {
            const sIdx = parseInt(card.dataset.sectionIndex, 10);
            const sec = cfg.sections[sIdx];
            if (!sec) return;
            const rowsPathEl = card.querySelector('.m2c-rows-path');
            if (rowsPathEl) sec.rowsPath = rowsPathEl.value.trim();

            const entriesKeyEl = card.querySelector('.m2c-entries-key');
            if (entriesKeyEl) sec.entriesKey = entriesKeyEl.value.trim() || undefined;

            const groupByEl = card.querySelector('.m2c-section-groupby');
            if (groupByEl) {
                const groupByText = groupByEl.value.trim();
                const groupBy = groupByText ? groupByText.split(',').map(s => s.trim()).filter(Boolean) : [];
                sec.groupBy = groupBy.length > 0 ? groupBy : undefined;
                // groupedItemsKey always mirrors the schema's own
                // RepeatingGroup.Key — never user-typed, so it can't drift
                // from what cda/builder actually expects (e.g. "components"
                // for Vital Signs).
                const repeatingGroup = this._repeatingGroupCatalog[sec.sectionKey];
                sec.groupedItemsKey = (sec.groupBy && repeatingGroup) ? repeatingGroup.key : undefined;
            }

            // Only 'item'/'entry' kinds address sec.fields/sec.entryFields
            // directly — a nested join's own kind is 'related-<fieldIndex>'
            // (see _renderRelatedRowsFieldRow), skipped here and instead
            // synced by _syncFieldRow's own scoped recursive call below.
            // Without this filter, a flat sweep over every
            // tr[data-field-index] in the card (which also finds nested
            // rows, since querySelectorAll recurses into the colspan cell's
            // own nested table) could mistake a nested join field's row
            // index for a top-level sec.fields index — both are just small
            // integers, so e.g. a nested row 0 could silently overwrite
            // sec.fields[0].
            card.querySelectorAll('tr[data-field-index]').forEach(row => {
                const kind = row.dataset.fieldKind;
                if (kind !== 'item' && kind !== 'entry') return;
                this._syncFieldRow(row, sec);
            });
        });
    }

    // Syncs ONE top-level field <tr> (kind 'item' or 'entry') back into
    // sec.fields[fIdx]/sec.entryFields[fIdx]. A relatedRows row (data-field-
    // relatedrows="1") has a completely different shape — no source/
    // fallback/literal/transform/valuemap inputs at all — so it's synced via
    // its own dedicated path instead of the normal scalar-cell reads, then
    // recurses into its nested joined-fields table using the SAME function.
    _syncFieldRow(row, sec) {
        const fIdx = parseInt(row.dataset.fieldIndex, 10);
        const arrayName = row.dataset.fieldKind === 'entry' ? 'entryFields' : 'fields';
        const arr = sec[arrayName];
        const f = arr && arr[fIdx];
        if (!f) return;

        if (row.dataset.fieldRelatedrows === '1') {
            // Distinct class from the normal .m2c-field-canonical — this
            // row's subtree ALSO contains a nested table full of
            // .m2c-field-canonical inputs (the joined fields), so reading
            // "the" canonical field here must not risk matching one of
            // those instead.
            f.canonicalField = row.querySelector('.m2c-relatedrows-canonical')?.value.trim() || '';
            if (!f.relatedRows) f.relatedRows = { fields: [] };
            const rr = f.relatedRows;
            rr.relatedRowsPath = row.querySelector('.m2c-relatedrows-path')?.value.trim() || '';
            rr.joinLocalKey = row.querySelector('.m2c-relatedrows-localkey')?.value.trim() || '';
            rr.joinRelatedKey = row.querySelector('.m2c-relatedrows-relatedkey')?.value.trim() || '';
            if (!Array.isArray(rr.fields)) rr.fields = [];
            // Nested joined-field rows live inside THIS row's own colspan
            // cell, with kind `related-${fIdx}` — scope the lookup to this
            // row's subtree only, so sibling relatedRows fields' nested
            // tables (if any) never cross-contaminate each other.
            row.querySelectorAll(`tr[data-field-kind="related-${fIdx}"]`).forEach(nestedRow => {
                this._syncFieldRow(nestedRow, rr);
            });
            return;
        }

        f.canonicalField = row.querySelector('.m2c-field-canonical')?.value.trim() || '';
        f.sourcePath = row.querySelector('.m2c-field-source')?.value.trim() || '';
        const fallbackText = row.querySelector('.m2c-field-fallback')?.value.trim() || '';
        f.fallbackPaths = fallbackText ? fallbackText.split(',').map(s => s.trim()).filter(Boolean) : undefined;
        const literal = row.querySelector('.m2c-field-literal')?.value.trim() || '';
        f.literalValue = literal || undefined;
        const transformEl = row.querySelector('.m2c-field-transform');
        f.transform = transformEl && transformEl.value ? transformEl.value : undefined;
        const valueMapEl = row.querySelector('.m2c-field-valuemap');
        const valueMap = valueMapEl ? MapToCanonicalBuilder._parseValueMapText(valueMapEl.value) : null;
        f.valueMap = valueMap && Object.keys(valueMap).length > 0 ? valueMap : undefined;
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

    // Fetches sectionKey's declared RepeatingGroup (cda/builder.
    // SectionRepeatingGroupCatalog) — {key, fields} if the section is a
    // "loop" section like Vital Signs, or null (cached as such) for the vast
    // majority of sections that aren't. The `sectionKey in
    // this._repeatingGroupCatalog` check callers use distinguishes
    // "not-yet-fetched" (key absent) from "fetched, has none" (key present,
    // value null) so a null result isn't re-fetched forever.
    _loadSectionRepeatingGroupCatalog(sectionKey) {
        return fetch(`/api/cda/canonical-fields/section/${encodeURIComponent(sectionKey)}/repeating-group`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                this._repeatingGroupCatalog[sectionKey] = (data && data.success) ? (data.repeatingGroup || null) : null;
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
