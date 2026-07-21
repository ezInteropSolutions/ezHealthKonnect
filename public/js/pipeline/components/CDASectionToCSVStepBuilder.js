/**
 * CDASectionToCSVStepBuilder — step config builder for cda.section_to_csv.
 *
 * The OOB CSV export (services/executors/transform/cda_csv_templates.go)
 * captures ~35 CDA sections into flat, one-row-per-entry CSV files, but
 * until now the pipeline builder gave zero visibility into WHAT it was
 * capturing (column names, CDA paths) and no way to add a column of your
 * own or override an OOB one — the properties panel just showed the generic
 * name/sequence/timeout form every step type gets by default.
 *
 * This builder lets a user:
 *   - See every OOB column (name, CDA path, fallback paths, whether it
 *     expands into CodeSystem/CodeSystemName/OriginalText, whether it
 *     collects ALL matching entries or just the first) per section, sourced
 *     live from GET /api/cda/csv/sections — never hand-duplicated, so this
 *     view can't drift from what the Go executor actually runs.
 *   - Enable/disable which of the sections get exported at all
 *     (step.config.sections — empty/absent means "every section").
 *   - Add a custom column with their own CDA path (optionally with its own
 *     fallback paths / code-metadata / multi-value flags) — appended
 *     alongside the OOB columns for that section.
 *   - Override an OOB column outright by reusing its exact Name — the
 *     override's Path/etc replace the OOB column's, same position in the
 *     row (services/executors/transform/cda_section_to_csv_executor.go's
 *     mergeCustomColumns is the runtime side of this). A "Reset to OOB"
 *     button removes the override.
 *
 * Config keys this builder reads/writes (see the executor's own doc
 * comment for the authoritative list):
 *   sourceField, outputPrefix, sections, customColumns
 *
 * Builder contract (StepBuilderRegistry):
 *   render(step)         → string  HTML for the properties panel form tab
 *   collectConfig(step)  → void    reads DOM/internal state, writes step.config
 *   destroy()            → void    tears down AC refs
 */

class CDASectionToCSVStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
        this._step = null;

        // [{sectionKey, narrativeOnly, columns:[{name,path,fallbackPaths,exposeCodeMetadata,multiple}]}]
        // populated async by _loadSections(); empty until then (renders a
        // "Loading…" placeholder).
        this._sections = [];

        // sectionKey → bool, whether the section is enabled — initialised
        // from step.config.sections in render(), mutated by checkbox
        // onchange, read directly (not scraped from the DOM) in
        // collectConfig() so it stays correct regardless of which rows are
        // currently expanded/collapsed.
        this._sectionEnabled = {};

        // sectionKey → bool, which sections' column detail is expanded.
        this._sectionExpanded = {};

        // sectionKey → []CSVColumn-shaped {name,path,fallbackPaths,exposeCodeMetadata,multiple}
        // working copy of step.config.customColumns, mutated by every
        // add/override/remove action; written back verbatim in
        // collectConfig() (stripped of empty section entries).
        this._customColumns = {};

        // sectionKey currently showing the "add/edit custom column" inline
        // form, or null. Set to a column's Name (string) when editing an
        // EXISTING custom column/override, or true when adding a brand new
        // one — see openAddForm/openEditForm.
        this._formOpenFor = null;
        this._formEditingName = null;

        window._cdaCsvBuilder = this;
    }

    // ── render ────────────────────────────────────────────────────────────

    render(step) {
        this._step = step;
        if (!step.config) step.config = {};
        const cfg = step.config;
        if (cfg.outputPrefix === undefined || cfg.outputPrefix === null) cfg.outputPrefix = 'csv_';
        this._customColumns = JSON.parse(JSON.stringify(cfg.customColumns || {}));

        // Empty/absent config.sections means "every section enabled" —
        // matches SupportedCDACSVSections() being the executor's own default.
        const explicit = Array.isArray(cfg.sections) && cfg.sections.length > 0 ? new Set(cfg.sections) : null;
        this._sectionEnabled = {};
        if (explicit) {
            explicit.forEach(k => { this._sectionEnabled[k] = true; });
        }
        this._explicitAllEnabled = !explicit;

        this._loadSections();

        return `
        <div id="cdaCsvBuilder" style="font-size:0.84rem;">
            ${this._renderGeneral(cfg)}
            <div style="margin:1.15rem 0 0.5rem;">
                <label style="${CDA_CSV_LBL}">Sections</label>
                <div style="font-size:0.78rem;color:#64748b;margin-bottom:0.55rem;">
                    Uncheck a section to skip exporting it. Click a section's name to see exactly what it captures, add a custom column, or override an OOB one.
                </div>
                <div id="cdaCsvSectionsList">${this._renderSectionsList()}</div>
            </div>
        </div>`;
    }

    // ── collectConfig ─────────────────────────────────────────────────────

    collectConfig(step) {
        const root = document.getElementById('cdaCsvBuilder');
        if (!root) return;
        step.config = step.config || {};

        const sourceEl = root.querySelector('#cdaCsvSourceField');
        const prefixEl = root.querySelector('#cdaCsvOutputPrefix');
        if (sourceEl) step.config.sourceField = sourceEl.value.trim();
        if (prefixEl) step.config.outputPrefix = prefixEl.value.trim() || 'csv_';

        if (this._sections.length > 0) {
            const allKeys = this._sections.map(s => s.sectionKey);
            const enabledKeys = allKeys.filter(k => this._isSectionEnabled(k));
            if (enabledKeys.length < allKeys.length) {
                step.config.sections = enabledKeys;
            } else {
                delete step.config.sections;
            }
        }

        const cleaned = {};
        Object.keys(this._customColumns).forEach(key => {
            const cols = this._customColumns[key];
            if (cols && cols.length > 0) cleaned[key] = cols;
        });
        step.config.customColumns = cleaned;
    }

    destroy() {
        this._ac.abort();
        if (window._cdaCsvBuilder === this) window._cdaCsvBuilder = null;
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

    // ── data loading ──────────────────────────────────────────────────────

    _loadSections() {
        const sig = this._ac.signal;
        fetch('/api/cda/csv/sections', { signal: sig })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.sections) return;
                this._sections = data.sections;
                const list = document.getElementById('cdaCsvSectionsList');
                if (list) list.innerHTML = this._renderSectionsList();
            })
            .catch(() => {}); // AbortError on destroy is expected
    }

    // ── General fields ───────────────────────────────────────────────────

    _renderGeneral(cfg) {
        const esc = cdaCsvEsc;
        return `
        <div class="config-group" style="margin-bottom:1.1rem;">
            <label style="${CDA_CSV_LBL}">Source Field</label>
            <input id="cdaCsvSourceField" type="text" class="form-control form-control-sm"
                value="${esc(cfg.sourceField || '')}" placeholder="(auto-detect parsed CDA document)"
                style="width:100%;font-family:monospace;font-size:0.82rem;padding:0.38rem 0.55rem;border:1px solid #cbd5e1;border-radius:6px;">
            <div style="${CDA_CSV_HINT}">Dot-path to the parsed CDA map. Leave blank to auto-detect (looks for a _format=ccda document from an upstream cda.parse/cda.dedupe step).</div>
        </div>
        <div class="config-group">
            <label style="${CDA_CSV_LBL}">Output Field Prefix</label>
            <input id="cdaCsvOutputPrefix" type="text" class="form-control form-control-sm"
                value="${esc(cfg.outputPrefix || 'csv_')}" placeholder="csv_"
                style="width:100%;font-family:monospace;font-size:0.82rem;padding:0.38rem 0.55rem;border:1px solid #cbd5e1;border-radius:6px;">
            <div style="${CDA_CSV_HINT}">Each enabled section's CSV is written to "&lt;prefix&gt;&lt;sectionKey&gt;", e.g. csv_medications, csv_allergiesAndIntolerances.</div>
        </div>`;
    }

    // ── Sections list ─────────────────────────────────────────────────────

    _renderSectionsList() {
        if (!this._sections.length) {
            return `<div style="text-align:center;padding:1.5rem;color:#6b7280;font-size:0.8rem;">Loading sections…</div>`;
        }
        return this._sections.map(sec => this._renderSectionRow(sec)).join('');
    }

    _renderSectionRow(sec) {
        const enabled = this._isSectionEnabled(sec.sectionKey);
        const expanded = !!this._sectionExpanded[sec.sectionKey];
        const overrideCount = (this._customColumns[sec.sectionKey] || []).length;
        const key = sec.sectionKey;
        const kindBadge = sec.narrativeOnly
            ? `<span style="font-size:0.65rem;background:#f1f5f9;color:#64748b;border-radius:3px;padding:1px 5px;white-space:nowrap;">narrative</span>`
            : `<span style="font-size:0.7rem;color:#94a3b8;white-space:nowrap;">${sec.columns.length} columns</span>`;
        const customBadge = overrideCount
            ? `<span style="font-size:0.65rem;background:#fef3c7;color:#92400e;border-radius:3px;padding:1px 5px;white-space:nowrap;">${overrideCount} custom</span>`
            : '';
        return `
        <div id="cdaCsvSectionRow-${key}" style="border:1px solid #e2e8f0;border-radius:6px;margin-bottom:0.4rem;overflow:hidden;">
            <div id="cdaCsvSectionHeader-${key}" style="display:flex;align-items:center;gap:0.5rem;padding:0.45rem 0.6rem;background:${expanded ? '#eff6ff' : '#fff'};cursor:pointer;"
                onclick="window._cdaCsvBuilder && window._cdaCsvBuilder.toggleSection('${key}')">
                <input type="checkbox" class="cda-csv-section-checkbox" data-section-key="${key}"
                    ${enabled ? 'checked' : ''}
                    onclick="event.stopPropagation()"
                    onchange="window._cdaCsvBuilder && window._cdaCsvBuilder.setSectionEnabled('${key}', this.checked)"
                    style="accent-color:#1e3a8a;width:14px;height:14px;flex-shrink:0;">
                <span style="font-weight:${enabled ? '500' : '400'};color:${enabled ? '#1e293b' : '#94a3b8'};flex:1;">${cdaCsvEsc(key)}</span>
                ${kindBadge}
                ${customBadge}
                <span id="cdaCsvSectionCaret-${key}" style="color:#94a3b8;">${expanded ? '▾' : '▸'}</span>
            </div>
            <div id="cdaCsvSectionDetail-${key}" style="display:${expanded ? 'block' : 'none'};border-top:1px solid #e2e8f0;padding:0.65rem 0.75rem;background:#fafbfc;">
                ${expanded ? this._renderSectionDetail(sec) : ''}
            </div>
        </div>`;
    }

    toggleSection(key) {
        const sec = this._sections.find(s => s.sectionKey === key);
        if (!sec) return;
        this._sectionExpanded[key] = !this._sectionExpanded[key];
        const detail = document.getElementById('cdaCsvSectionDetail-' + key);
        const caret  = document.getElementById('cdaCsvSectionCaret-' + key);
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
        const header = document.getElementById('cdaCsvSectionHeader-' + key);
        if (header) header.style.background = this._sectionExpanded[key] ? '#eff6ff' : '#fff';
    }

    _refreshSectionDetail(key) {
        const sec = this._sections.find(s => s.sectionKey === key);
        if (!sec) return;
        // Replace the WHOLE row (header + detail), not just the detail div —
        // the custom-column count badge lives in the header, outside the
        // detail area, and _renderSectionRow already renders the detail
        // expanded/collapsed correctly based on this._sectionExpanded[key].
        const row = document.getElementById('cdaCsvSectionRow-' + key);
        if (row) row.outerHTML = this._renderSectionRow(sec);
    }

    // ── Section detail: OOB columns table + custom columns + add form ─────

    _renderSectionDetail(sec) {
        if (sec.narrativeOnly) {
            return `<div style="font-size:0.78rem;color:#64748b;">
                This section is narrative-only — no structured CDA entries exist for it in real documents, so it always exports a single row of free-text (SourceFile, NarrativeText). Custom columns aren't applicable here.
            </div>`;
        }

        const custom = this._customColumns[sec.sectionKey] || [];
        const customByName = {};
        custom.forEach(c => { customByName[c.name] = c; });

        const oobRows = sec.columns.map(col => {
            const override = customByName[col.name];
            const flags = [
                col.exposeCodeMetadata ? `<span title="Also adds ${cdaCsvEsc(col.name)}CodeSystem / CodeSystemName / OriginalText" style="font-size:0.62rem;background:#eef2ff;color:#4338ca;border-radius:3px;padding:0 4px;">coded</span>` : '',
                col.multiple ? `<span title="Collects every matching entry, semicolon-joined" style="font-size:0.62rem;background:#ecfdf5;color:#065f46;border-radius:3px;padding:0 4px;">multi</span>` : '',
            ].filter(Boolean).join(' ');
            const pathDisplay = override
                ? `<span style="text-decoration:line-through;color:#cbd5e1;">${cdaCsvEsc(col.path)}</span><br><span style="color:#b45309;">${cdaCsvEsc(override.path)}</span>`
                : cdaCsvEsc(col.path);
            const fallbacks = (override ? override.fallbackPaths : col.fallbackPaths) || [];
            const action = override
                ? `<button type="button" onclick="window._cdaCsvBuilder && window._cdaCsvBuilder.removeCustomColumn('${sec.sectionKey}', '${cdaCsvEscAttr(col.name)}')"
                     style="padding:0.12rem 0.5rem;font-size:0.68rem;background:#fff;border:1px solid #fca5a5;color:#b91c1c;border-radius:4px;cursor:pointer;">Reset to OOB</button>`
                : `<button type="button" onclick="window._cdaCsvBuilder && window._cdaCsvBuilder.openEditForm('${sec.sectionKey}', '${cdaCsvEscAttr(col.name)}')"
                     style="padding:0.12rem 0.5rem;font-size:0.68rem;background:#fff;border:1px solid #cbd5e1;color:#334155;border-radius:4px;cursor:pointer;">Override</button>`;
            return `
            <tr style="border-bottom:1px solid #f1f5f9;">
                <td style="padding:0.35rem 0.5rem;font-weight:500;color:#1e293b;white-space:nowrap;">${cdaCsvEsc(col.name)} ${flags}</td>
                <td style="padding:0.35rem 0.5rem;font-family:monospace;font-size:0.72rem;color:#475569;word-break:break-all;">${pathDisplay}${fallbacks.length ? `<div style="font-size:0.68rem;color:#94a3b8;margin-top:2px;">fallback: ${fallbacks.map(cdaCsvEsc).join(' · ')}</div>` : ''}</td>
                <td style="padding:0.35rem 0.5rem;text-align:right;white-space:nowrap;">${action}</td>
            </tr>`;
        }).join('');

        const pureAdds = custom.filter(c => !sec.columns.some(col => col.name === c.name));
        const addRows = pureAdds.map(c => {
            const flags = [
                c.exposeCodeMetadata ? `<span style="font-size:0.62rem;background:#eef2ff;color:#4338ca;border-radius:3px;padding:0 4px;">coded</span>` : '',
                c.multiple ? `<span style="font-size:0.62rem;background:#ecfdf5;color:#065f46;border-radius:3px;padding:0 4px;">multi</span>` : '',
            ].filter(Boolean).join(' ');
            return `
            <tr style="border-bottom:1px solid #f1f5f9;background:#fffbeb;">
                <td style="padding:0.35rem 0.5rem;font-weight:500;color:#92400e;white-space:nowrap;">${cdaCsvEsc(c.name)} ${flags} <span style="font-size:0.62rem;background:#fef3c7;color:#92400e;border-radius:3px;padding:0 4px;">custom</span></td>
                <td style="padding:0.35rem 0.5rem;font-family:monospace;font-size:0.72rem;color:#92400e;word-break:break-all;">${cdaCsvEsc(c.path)}${(c.fallbackPaths||[]).length ? `<div style="font-size:0.68rem;color:#b45309;margin-top:2px;">fallback: ${(c.fallbackPaths||[]).map(cdaCsvEsc).join(' · ')}</div>` : ''}</td>
                <td style="padding:0.35rem 0.5rem;text-align:right;white-space:nowrap;">
                    <button type="button" onclick="window._cdaCsvBuilder && window._cdaCsvBuilder.openEditForm('${sec.sectionKey}', '${cdaCsvEscAttr(c.name)}')"
                        style="padding:0.12rem 0.5rem;font-size:0.68rem;background:#fff;border:1px solid #cbd5e1;color:#334155;border-radius:4px;cursor:pointer;margin-right:0.3rem;">Edit</button>
                    <button type="button" onclick="window._cdaCsvBuilder && window._cdaCsvBuilder.removeCustomColumn('${sec.sectionKey}', '${cdaCsvEscAttr(c.name)}')"
                        style="padding:0.12rem 0.5rem;font-size:0.68rem;background:#fff;border:1px solid #fca5a5;color:#b91c1c;border-radius:4px;cursor:pointer;">Remove</button>
                </td>
            </tr>`;
        }).join('');

        const formOpen = this._formOpenFor === sec.sectionKey;

        return `
        <div style="overflow:auto;max-height:280px;border:1px solid #e2e8f0;border-radius:6px;margin-bottom:0.6rem;">
            <table style="width:100%;border-collapse:collapse;">
                <thead>
                    <tr style="background:#f8fafc;">
                        <th style="text-align:left;padding:0.35rem 0.5rem;font-size:0.68rem;text-transform:uppercase;color:#64748b;">Name</th>
                        <th style="text-align:left;padding:0.35rem 0.5rem;font-size:0.68rem;text-transform:uppercase;color:#64748b;">CDA Path</th>
                        <th style="text-align:right;padding:0.35rem 0.5rem;font-size:0.68rem;text-transform:uppercase;color:#64748b;"></th>
                    </tr>
                </thead>
                <tbody>${oobRows}${addRows}</tbody>
            </table>
        </div>
        ${formOpen ? this._renderColumnForm(sec.sectionKey) : `
        <button type="button" onclick="window._cdaCsvBuilder && window._cdaCsvBuilder.openAddForm('${sec.sectionKey}')"
            style="padding:0.3rem 0.75rem;font-size:0.76rem;background:#1e3a8a;color:white;border:none;border-radius:5px;cursor:pointer;">
            + Add Custom Column
        </button>`}`;
    }

    // ── Add/Edit custom column form ─────────────────────────────────────────

    openAddForm(sectionKey) {
        this._formOpenFor = sectionKey;
        this._formEditingName = null;
        this._formDraft = { name: '', path: '', fallbackPaths: [], exposeCodeMetadata: false, multiple: false };
        this._refreshSectionDetail(sectionKey);
    }

    // Looks up the column's current data itself (from this._customColumns
    // for an existing custom column/override, else this._sections for an
    // OOB column being overridden for the first time) rather than taking it
    // as an argument — passing a JSON blob through an inline onclick's
    // double-quoted HTML attribute is a quoting minefield (JSON.stringify's
    // own double quotes would terminate the attribute early).
    openEditForm(sectionKey, columnName) {
        this._formOpenFor = sectionKey;
        this._formEditingName = columnName;

        const existingCustom = (this._customColumns[sectionKey] || []).find(c => c.name === columnName);
        if (existingCustom) {
            this._formDraft = {
                name: existingCustom.name,
                path: existingCustom.path,
                fallbackPaths: existingCustom.fallbackPaths || [],
                exposeCodeMetadata: !!existingCustom.exposeCodeMetadata,
                multiple: !!existingCustom.multiple,
            };
        } else {
            const sec = this._sections.find(s => s.sectionKey === sectionKey);
            const oob = sec && sec.columns.find(c => c.name === columnName);
            this._formDraft = oob
                ? { name: oob.name, path: oob.path, fallbackPaths: oob.fallbackPaths || [], exposeCodeMetadata: !!oob.exposeCodeMetadata, multiple: !!oob.multiple }
                : { name: columnName, path: '', fallbackPaths: [], exposeCodeMetadata: false, multiple: false };
        }
        this._refreshSectionDetail(sectionKey);
    }

    closeColumnForm(sectionKey) {
        this._formOpenFor = null;
        this._formEditingName = null;
        this._refreshSectionDetail(sectionKey);
    }

    _renderColumnForm(sectionKey) {
        const d = this._formDraft || { name: '', path: '', fallbackPaths: [], exposeCodeMetadata: false, multiple: false };
        const esc = cdaCsvEsc;
        const isEdit = !!this._formEditingName;
        return `
        <div style="border:1px solid #cbd5e1;border-radius:6px;padding:0.75rem;margin-top:0.5rem;background:#fff;">
            <div style="font-size:0.8rem;font-weight:600;color:#1e293b;margin-bottom:0.6rem;">${isEdit ? 'Edit' : 'Add'} Custom Column</div>
            <div style="margin-bottom:0.6rem;">
                <label style="${CDA_CSV_LBL}">Name</label>
                <input id="cdaCsvFormName" type="text" value="${esc(d.name)}" placeholder="e.g. Reaction"
                    oninput="window._cdaCsvBuilder && window._cdaCsvBuilder.onFormNameInput('${sectionKey}')"
                    style="width:100%;font-size:0.8rem;padding:0.32rem 0.5rem;border:1px solid #cbd5e1;border-radius:5px;box-sizing:border-box;">
                <div id="cdaCsvFormOverrideWarning" style="${CDA_CSV_HINT}"></div>
            </div>
            <div style="margin-bottom:0.6rem;">
                <label style="${CDA_CSV_LBL}">CDA Path</label>
                <input id="cdaCsvFormPath" type="text" value="${esc(d.path)}" placeholder="e.g. entryRelationships[typeCode=SUBJ].entry.value.code"
                    style="width:100%;font-family:monospace;font-size:0.78rem;padding:0.32rem 0.5rem;border:1px solid #cbd5e1;border-radius:5px;box-sizing:border-box;">
                <div style="${CDA_CSV_HINT}">Dot-separated path, relative to one section entry. Supports [key=value] bracket predicates (typeCode, templateId, classCode, moodCode, code, inversionInd, ...), [*] wildcards, [N] indices, and "A|B" OR values.</div>
            </div>
            <div style="margin-bottom:0.6rem;">
                <label style="${CDA_CSV_LBL}">Fallback Paths <span style="font-weight:400;color:#94a3b8;text-transform:none;">(optional, one per line, tried in order if Path resolves to nothing)</span></label>
                <textarea id="cdaCsvFormFallbacks" rows="2" placeholder="entryRelationships[typeCode=SUBJ,inversionInd=true].entry.value.code"
                    style="width:100%;font-family:monospace;font-size:0.76rem;padding:0.32rem 0.5rem;border:1px solid #cbd5e1;border-radius:5px;box-sizing:border-box;resize:vertical;">${esc((d.fallbackPaths || []).join('\n'))}</textarea>
            </div>
            <div style="display:flex;gap:1.25rem;margin-bottom:0.75rem;">
                <label style="display:flex;align-items:center;gap:0.4rem;font-size:0.78rem;color:#1f2937;cursor:pointer;">
                    <input id="cdaCsvFormExposeCode" type="checkbox" ${d.exposeCodeMetadata ? 'checked' : ''}
                        style="accent-color:#1e3a8a;width:14px;height:14px;">
                    Coded concept <span style="color:#94a3b8;font-weight:400;">(adds CodeSystem/CodeSystemName/OriginalText columns)</span>
                </label>
                <label style="display:flex;align-items:center;gap:0.4rem;font-size:0.78rem;color:#1f2937;cursor:pointer;">
                    <input id="cdaCsvFormMultiple" type="checkbox" ${d.multiple ? 'checked' : ''}
                        style="accent-color:#1e3a8a;width:14px;height:14px;">
                    Capture all matches <span style="color:#94a3b8;font-weight:400;">(semicolon-joined, not just the first)</span>
                </label>
            </div>
            <div style="display:flex;gap:0.5rem;">
                <button type="button" onclick="window._cdaCsvBuilder && window._cdaCsvBuilder.saveColumnForm('${sectionKey}')"
                    style="padding:0.3rem 0.85rem;font-size:0.78rem;background:#1e3a8a;color:white;border:none;border-radius:5px;cursor:pointer;">Save</button>
                <button type="button" onclick="window._cdaCsvBuilder && window._cdaCsvBuilder.closeColumnForm('${sectionKey}')"
                    style="padding:0.3rem 0.85rem;font-size:0.78rem;background:#fff;border:1px solid #cbd5e1;color:#334155;border-radius:5px;cursor:pointer;">Cancel</button>
            </div>
        </div>`;
    }

    onFormNameInput(sectionKey) {
        const nameEl = document.getElementById('cdaCsvFormName');
        const warnEl = document.getElementById('cdaCsvFormOverrideWarning');
        if (!nameEl || !warnEl) return;
        const name = nameEl.value.trim();
        const sec = this._sections.find(s => s.sectionKey === sectionKey);
        const oobMatch = sec && sec.columns.find(c => c.name === name);
        if (oobMatch && name !== this._formEditingName) {
            warnEl.style.color = '#b45309';
            warnEl.textContent = `This will override the existing OOB "${name}" column (currently: ${oobMatch.path}).`;
        } else {
            warnEl.textContent = '';
        }
    }

    saveColumnForm(sectionKey) {
        const nameEl = document.getElementById('cdaCsvFormName');
        const pathEl = document.getElementById('cdaCsvFormPath');
        const fbEl   = document.getElementById('cdaCsvFormFallbacks');
        const codeEl = document.getElementById('cdaCsvFormExposeCode');
        const multEl = document.getElementById('cdaCsvFormMultiple');
        if (!nameEl || !pathEl) return;

        const name = nameEl.value.trim();
        const path = pathEl.value.trim();
        if (!name || !path) {
            alert('Name and CDA Path are both required.');
            return;
        }
        const fallbackPaths = (fbEl ? fbEl.value : '').split('\n').map(s => s.trim()).filter(Boolean);

        const newCol = {
            name,
            path,
            fallbackPaths,
            exposeCodeMetadata: !!(codeEl && codeEl.checked),
            multiple: !!(multEl && multEl.checked),
        };

        if (!this._customColumns[sectionKey]) this._customColumns[sectionKey] = [];
        const list = this._customColumns[sectionKey];

        // Renaming an existing custom column (name changed while editing) —
        // drop the old entry first so we don't leave a stale duplicate.
        if (this._formEditingName && this._formEditingName !== name) {
            const idx = list.findIndex(c => c.name === this._formEditingName);
            if (idx !== -1) list.splice(idx, 1);
        }

        const existingIdx = list.findIndex(c => c.name === name);
        if (existingIdx !== -1) {
            list[existingIdx] = newCol;
        } else {
            list.push(newCol);
        }

        this._formOpenFor = null;
        this._formEditingName = null;
        this._refreshSectionDetail(sectionKey);
    }

    removeCustomColumn(sectionKey, name) {
        const list = this._customColumns[sectionKey];
        if (!list) return;
        this._customColumns[sectionKey] = list.filter(c => c.name !== name);
        this._refreshSectionDetail(sectionKey);
    }
}

// ── module-scope helpers (shared style strings + tiny escaping utils) ──────

const CDA_CSV_LBL = 'font-size:0.7rem;font-weight:700;text-transform:uppercase;letter-spacing:0.05em;' +
    'color:#1e3a8a;display:block;margin-bottom:0.35rem;border-left:3px solid #f472b6;padding-left:0.45rem;';
const CDA_CSV_HINT = 'font-size:0.7rem;color:#94a3b8;margin-top:0.25rem;';

function cdaCsvEsc(s) {
    return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// For values interpolated inside a single-quoted HTML attribute (onclick
// argument strings) — escapes single quotes in addition to the usual HTML
// entities, since cdaCsvEsc alone leaves ' unescaped and every call site
// here wraps its string args in '...'.
function cdaCsvEscAttr(s) {
    return cdaCsvEsc(s).replace(/'/g, '&#39;');
}

StepBuilderRegistry.register('cda.section_to_csv', CDASectionToCSVStepBuilder);
