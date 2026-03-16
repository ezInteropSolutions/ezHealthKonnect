/**
 * @fileoverview NormalizerBuilder — no-code UI for the Normalizer / Pivot / Transpose
 * pipeline step. Works with any format (HL7, FHIR, CSV, JSON, EDI).
 * @version 1.0.0
 *
 * Design principles:
 *   - One operation picker drives the entire form (progressive disclosure)
 *   - FieldPathSearchComponent on source/output fields for zero-typing workflow
 *   - Chip arrays for multi-value inputs (groupBy, normalizeFields, preserveFields)
 *   - Post-processing section (case transform + column renames) collapsed by default
 *   - Follows the exact same StepBuilderRegistry contract as DataMaskingBuilder
 */

class NormalizerBuilder {

    constructor(panel) {
        /** @private */
        this._panel = panel;
        /** @private @type {FieldPathSearchComponent[]} */
        this._acs = [];
        /** @private — chip arrays keyed by input id */
        this._chips = {};
    }

    // ─── Public Builder Contract ─────────────────────────────────────────────

    render(step) {
        if (!step.config) step.config = {};
        const html = this._buildHTML(step);
        setTimeout(() => this._attachEvents(step), 0);
        return html;
    }

    collectConfig(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;

        cfg.operation     = document.getElementById('nrmOperation')?.value || 'normalize';
        cfg.sourceField   = document.getElementById('nrmSourceField')?.value?.trim() || '';
        cfg.outputField   = document.getElementById('nrmOutputField')?.value?.trim() || '';
        cfg.caseTransform = document.getElementById('nrmCaseTransform')?.value || '';

        // Per-operation fields
        cfg.keyColumn        = document.getElementById('nrmKeyColumn')?.value?.trim()   || 'field';
        cfg.valueColumn      = document.getElementById('nrmValueColumn')?.value?.trim() || 'value';
        cfg.normalizeFields  = this._readChips('nrmNormalizeFields');
        cfg.preserveFields   = this._readChips('nrmPreserveFields');
        cfg.pivotField       = document.getElementById('nrmPivotField')?.value?.trim()  || '';
        cfg.valueField       = document.getElementById('nrmValueField')?.value?.trim()  || '';
        cfg.groupBy          = this._readChips('nrmGroupBy');
        cfg.delimiter        = document.getElementById('nrmDelimiter')?.value?.trim()   || '.';
        cfg.maxDepth         = parseInt(document.getElementById('nrmMaxDepth')?.value   || '0', 10) || 0;
        cfg.renameMap        = this._readRenameMap();
    }

    destroy() {
        for (const ac of this._acs) {
            try { ac.destroy(); } catch (_) {}
        }
        this._acs = [];
        this._chips = {};
    }

    // ─── Private: HTML ───────────────────────────────────────────────────────

    _buildHTML(step) {
        const cfg = step.config;
        const op  = cfg.operation || 'normalize';

        return `
<div class="nrm-builder">
<style>
.nrm-builder { font-size:13px; }

/* Operation picker */
.nrm-ops { display:grid; grid-template-columns:1fr 1fr; gap:6px; margin-bottom:14px; }
.nrm-op-btn {
    display:flex; flex-direction:column; align-items:center; gap:3px;
    padding:10px 8px; border:1.5px solid rgba(30,58,138,0.20); border-radius:8px;
    background:#fff; cursor:pointer; transition:all 0.15s; text-align:center;
}
.nrm-op-btn:hover { border-color:rgba(30,58,138,0.50); background:rgba(30,58,138,0.04); }
.nrm-op-btn.active {
    border-color:#3b82f6; background:rgba(59,130,246,0.08);
    box-shadow:0 0 0 2px rgba(59,130,246,0.15);
}
.nrm-op-icon { font-size:18px; }
.nrm-op-name { font-size:11px; font-weight:600; color:#1e293b; }
.nrm-op-desc { font-size:10px; color:#64748b; line-height:1.3; }

/* Labels and inputs */
.nrm-field { margin-bottom:10px; }
.nrm-label { display:block; font-size:11px; font-weight:600; color:#64748b;
             text-transform:uppercase; letter-spacing:0.04em; margin-bottom:4px; }
.nrm-input, .nrm-select {
    width:100%; padding:6px 9px; border:1px solid rgba(30,58,138,0.20);
    border-radius:5px; font-size:12px; background:#fff; color:#1e293b;
    outline:none; box-sizing:border-box;
}
.nrm-input:focus, .nrm-select:focus { border-color:#3b82f6; }
.nrm-row2 { display:grid; grid-template-columns:1fr 1fr; gap:8px; }

/* Column suggest dropdown — appended to document.body, positioned via getBoundingClientRect */
.nrm-col-suggest {
    position:fixed; z-index:99999; background:#fff;
    border:1px solid rgba(30,58,138,0.25); border-radius:6px;
    box-shadow:0 4px 16px rgba(0,0,0,0.14); max-height:200px;
    overflow-y:auto; min-width:180px;
}
.nrm-col-suggest-item {
    padding:6px 10px; font-size:12px; cursor:pointer;
    color:#1e293b; border-bottom:1px solid rgba(0,0,0,0.04);
    display:flex; align-items:center; gap:6px;
}
.nrm-col-suggest-item:hover, .nrm-col-suggest-item.focused {
    background:rgba(59,130,246,0.10); color:#1e3a8a;
}
.nrm-col-suggest-item:last-child { border-bottom:none; }
.nrm-col-tag { font-size:9px; padding:1px 5px; border-radius:8px;
               background:rgba(30,58,138,0.10); color:#475569; }

/* Chip input */
.nrm-chip-wrap {
    display:flex; flex-wrap:wrap; gap:4px; align-items:center;
    border:1px solid rgba(30,58,138,0.20); border-radius:5px;
    padding:4px 6px; min-height:32px; cursor:text; background:#fff;
}
.nrm-chip {
    display:inline-flex; align-items:center; gap:4px;
    padding:2px 7px; background:rgba(59,130,246,0.12); color:#1e3a8a;
    border-radius:12px; font-size:11px; font-weight:500;
}
.nrm-chip-x {
    cursor:pointer; color:#64748b; line-height:1; font-size:13px;
}
.nrm-chip-x:hover { color:#ef4444; }
.nrm-chip-input {
    border:none; outline:none; font-size:12px; min-width:80px; flex:1;
    padding:2px 0; background:transparent; color:#1e293b;
}

/* Section card */
.nrm-card {
    border:1px solid rgba(30,58,138,0.12); border-radius:7px;
    padding:12px; margin-bottom:10px; background:rgba(248,250,252,0.7);
}
.nrm-card-title {
    font-size:11px; font-weight:700; text-transform:uppercase;
    letter-spacing:0.05em; color:#64748b; margin-bottom:10px;
    display:flex; align-items:center; gap:6px;
}

/* Rename map */
.nrm-rename-row {
    display:grid; grid-template-columns:1fr 24px 1fr 28px; gap:4px;
    align-items:center; margin-bottom:5px;
}
.nrm-rename-sep { text-align:center; color:#94a3b8; font-size:13px; user-select:none; }
.nrm-rm-btn {
    border:none; background:none; cursor:pointer; color:#94a3b8;
    font-size:14px; padding:2px; line-height:1;
}
.nrm-rm-btn:hover { color:#ef4444; }
.nrm-add-btn {
    display:inline-flex; align-items:center; gap:5px;
    padding:4px 10px; background:rgba(30,58,138,0.07);
    color:#1e3a8a; border:1px solid rgba(30,58,138,0.20);
    border-radius:5px; font-size:11px; font-weight:500;
    cursor:pointer; transition:background 0.15s; margin-top:2px;
}
.nrm-add-btn:hover { background:rgba(30,58,138,0.13); }

/* Collapse toggle */
.nrm-toggle-hdr {
    display:flex; align-items:center; gap:6px; cursor:pointer;
    font-size:11px; font-weight:600; color:#64748b;
    text-transform:uppercase; letter-spacing:0.04em;
    padding:8px 0 4px; border-top:1px solid rgba(30,58,138,0.10); margin-top:6px;
    user-select:none;
}
.nrm-toggle-arrow { font-size:10px; transition:transform 0.2s; }
.nrm-toggle-body { display:none; }
.nrm-toggle-body.open { display:block; }

/* Op section visibility */
.nrm-op-section { display:none; }
.nrm-op-section.active { display:block; }

/* Tip text */
.nrm-tip {
    font-size:10px; color:#64748b; background:rgba(59,130,246,0.07);
    border-radius:5px; padding:5px 8px; margin-bottom:8px; line-height:1.5;
}
</style>

<!-- hidden state field -->
<input type="hidden" id="nrmOperation" value="${op}">

<!-- ── Operation Picker ─────────────────────────────────────── -->
<div class="nrm-card">
  <div class="nrm-card-title"><i class="fas fa-th-large"></i> Operation</div>
  <div class="nrm-ops">
    ${NormalizerBuilder.OPERATIONS.map(o => `
    <button type="button" class="nrm-op-btn${op === o.value ? ' active' : ''}"
            data-op="${o.value}" title="${o.tooltip}">
      <span class="nrm-op-icon">${o.icon}</span>
      <span class="nrm-op-name">${o.name}</span>
      <span class="nrm-op-desc">${o.desc}</span>
    </button>`).join('')}
  </div>
</div>

<!-- ── Source & Output ─────────────────────────────────────── -->
<div class="nrm-card">
  <div class="nrm-card-title"><i class="fas fa-database"></i> Data Source</div>
  <div class="nrm-field">
    <label class="nrm-label">Source field <span style="color:#ef4444">*</span></label>
    <input class="nrm-input" id="nrmSourceField" type="text"
           value="${cfg.sourceField || ''}"
           placeholder="e.g. steps.file_parser.step_output.records">
  </div>
  <div class="nrm-field">
    <label class="nrm-label">Output field <span style="color:#94a3b8">(blank = overwrite source)</span></label>
    <input class="nrm-input" id="nrmOutputField" type="text"
           value="${cfg.outputField || ''}"
           placeholder="e.g. normalized_records">
  </div>
</div>

<!-- ── Normalize section ───────────────────────────────────── -->
<div class="nrm-op-section${op === 'normalize' ? ' active' : ''}" data-section="normalize">
  <div class="nrm-card">
    <div class="nrm-card-title"><i class="fas fa-list"></i> Normalize (Unpivot) Options</div>
    <div class="nrm-tip">Turns each selected column into a row with <em>key</em> and <em>value</em> columns.
         Great for flattening lab result columns, OBX repeats, or observation attribute tables.</div>
    <div class="nrm-field">
      <label class="nrm-label">Fields to normalize <span style="color:#94a3b8">(blank = all fields)</span></label>
      ${this._buildChipInput('nrmNormalizeFields', cfg.normalizeFields || [], 'e.g. lab_result, vitals')}
    </div>
    <div class="nrm-field">
      <label class="nrm-label">Preserve fields <span style="color:#94a3b8">(copied into every row)</span></label>
      ${this._buildChipInput('nrmPreserveFields', cfg.preserveFields || [], 'e.g. patient_id, encounter_id')}
    </div>
    <div class="nrm-row2">
      <div class="nrm-field">
        <label class="nrm-label">Key column name</label>
        <input class="nrm-input" id="nrmKeyColumn" type="text"
               value="${cfg.keyColumn || 'field'}" placeholder="field">
      </div>
      <div class="nrm-field">
        <label class="nrm-label">Value column name</label>
        <input class="nrm-input" id="nrmValueColumn" type="text"
               value="${cfg.valueColumn || 'value'}" placeholder="value">
      </div>
    </div>
  </div>
</div>

<!-- ── Pivot section ───────────────────────────────────────── -->
<div class="nrm-op-section${op === 'pivot' ? ' active' : ''}" data-section="pivot">
  <div class="nrm-card">
    <div class="nrm-card-title"><i class="fas fa-table"></i> Pivot Options</div>
    <div class="nrm-tip">Rotates rows into columns. Each unique value in <em>Pivot field</em> becomes a
         new column, filled from <em>Value field</em>. Rows with the same <em>Group by</em> values are merged.</div>
    <div class="nrm-row2">
      <div class="nrm-field">
        <label class="nrm-label">Pivot field <span style="color:#ef4444">*</span></label>
        <input class="nrm-input" id="nrmPivotField" type="text"
               value="${cfg.pivotField || ''}" placeholder="e.g. metric_name">
      </div>
      <div class="nrm-field">
        <label class="nrm-label">Value field <span style="color:#ef4444">*</span></label>
        <input class="nrm-input" id="nrmValueField" type="text"
               value="${cfg.valueField || ''}" placeholder="e.g. metric_value">
      </div>
    </div>
    <div class="nrm-field">
      <label class="nrm-label">Group by <span style="color:#ef4444">*</span> <span style="color:#94a3b8">(identity columns per output row)</span></label>
      ${this._buildChipInput('nrmGroupBy', cfg.groupBy || [], 'e.g. patient_id  →  press Enter')}
    </div>
  </div>
</div>

<!-- ── Transpose section ───────────────────────────────────── -->
<div class="nrm-op-section${op === 'transpose' ? ' active' : ''}" data-section="transpose">
  <div class="nrm-card">
    <div class="nrm-card-title"><i class="fas fa-sync-alt"></i> Transpose</div>
    <div class="nrm-tip">Swaps rows and columns. Each field name becomes a row labelled <code>field</code>,
         and each original row becomes a column labelled <code>row_0</code>, <code>row_1</code>, …</div>
    <div class="nrm-tip" style="background:rgba(139,92,246,0.07); color:#5b21b6;">
      <i class="fas fa-info-circle"></i> No additional configuration required — just set Source and Output fields above.
    </div>
  </div>
</div>

<!-- ── Flatten section ─────────────────────────────────────── -->
<div class="nrm-op-section${op === 'flatten' ? ' active' : ''}" data-section="flatten">
  <div class="nrm-card">
    <div class="nrm-card-title"><i class="fas fa-layer-group"></i> Flatten Options</div>
    <div class="nrm-tip">Collapses nested JSON into a single-level object using <em>Delimiter</em> as
         the path separator. Useful before CSV export or flat-schema mapping.</div>
    <div class="nrm-row2">
      <div class="nrm-field">
        <label class="nrm-label">Delimiter</label>
        <input class="nrm-input" id="nrmDelimiter" type="text"
               value="${cfg.delimiter || '.'}" placeholder=".">
      </div>
      <div class="nrm-field">
        <label class="nrm-label">Max depth <span style="color:#94a3b8">(0 = unlimited)</span></label>
        <input class="nrm-input" id="nrmMaxDepth" type="number" min="0"
               value="${cfg.maxDepth || 0}" placeholder="0">
      </div>
    </div>
  </div>
</div>

<!-- ── Unflatten section ───────────────────────────────────── -->
<div class="nrm-op-section${op === 'unflatten' ? ' active' : ''}" data-section="unflatten">
  <div class="nrm-card">
    <div class="nrm-card-title"><i class="fas fa-sitemap"></i> Unflatten Options</div>
    <div class="nrm-tip">Rebuilds a nested object from dot-notation keys. Inverse of Flatten.
         Use after reading a flat CSV to reconstruct the target schema.</div>
    <div class="nrm-field" style="max-width:160px;">
      <label class="nrm-label">Delimiter</label>
      <input class="nrm-input" id="nrmDelimiter" type="text"
             value="${cfg.delimiter || '.'}" placeholder=".">
    </div>
  </div>
</div>

<!-- ── Post-processing (collapsed) ────────────────────────── -->
<div class="nrm-toggle-hdr" id="nrmPostHdr">
  <span class="nrm-toggle-arrow" id="nrmPostArrow">&#9654;</span>
  <i class="fas fa-magic"></i> Post-processing
  <span style="font-size:10px; font-weight:400; color:#94a3b8;">(rename columns, change case)</span>
</div>
<div class="nrm-toggle-body${(cfg.caseTransform || Object.keys(cfg.renameMap || {}).length) ? ' open' : ''}"
     id="nrmPostBody">
  <div class="nrm-card" style="margin-top:8px;">
    <div class="nrm-field">
      <label class="nrm-label">Column case transform</label>
      <select class="nrm-select" id="nrmCaseTransform">
        <option value=""       ${!cfg.caseTransform ? 'selected' : ''}>— none —</option>
        <option value="lower"  ${'lower'  === cfg.caseTransform ? 'selected' : ''}>lowercase</option>
        <option value="upper"  ${'upper'  === cfg.caseTransform ? 'selected' : ''}>UPPERCASE</option>
        <option value="snake"  ${'snake'  === cfg.caseTransform ? 'selected' : ''}>snake_case</option>
        <option value="camel"  ${'camel'  === cfg.caseTransform ? 'selected' : ''}>camelCase</option>
      </select>
    </div>
    <div class="nrm-field">
      <label class="nrm-label">Rename columns <span style="color:#94a3b8">(old → new)</span></label>
      <div id="nrmRenameRows">${this._buildRenameRows(cfg.renameMap || {})}</div>
      <button type="button" class="nrm-add-btn" id="nrmAddRename">
        <i class="fas fa-plus"></i> Add rename
      </button>
    </div>
  </div>
</div>

</div>`;
    }

    // ─── Private: chip input HTML ────────────────────────────────────────────

    _buildChipInput(id, values, placeholder) {
        const chips = values.map(v => this._chipTag(v)).join('');
        return `
<div class="nrm-chip-wrap" id="${id}Wrap">
  ${chips}
  <input class="nrm-chip-input" id="${id}Input" type="text" placeholder="${placeholder}">
  <input type="hidden" id="${id}Json" value="${this._esc(JSON.stringify(values))}">
</div>`;
    }

    _chipTag(value) {
        const esc = this._esc(value);
        return `<span class="nrm-chip" data-val="${esc}">${esc}<span class="nrm-chip-x" title="Remove">×</span></span>`;
    }

    // ─── Private: rename map HTML ────────────────────────────────────────────

    _buildRenameRows(map) {
        if (!map || Object.keys(map).length === 0) return '';
        return Object.entries(map).map(([from, to]) => this._renameRow(from, to)).join('');
    }

    _renameRow(from = '', to = '') {
        return `
<div class="nrm-rename-row">
  <input class="nrm-input nrm-rename-from" type="text" value="${this._esc(from)}" placeholder="old name">
  <span class="nrm-rename-sep">→</span>
  <input class="nrm-input nrm-rename-to"  type="text" value="${this._esc(to)}"   placeholder="new name">
  <button type="button" class="nrm-rm-btn" title="Remove">×</button>
</div>`;
    }

    // ─── Private: event wiring ───────────────────────────────────────────────

    _attachEvents(step) {
        // Operation picker
        document.querySelectorAll('.nrm-op-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                document.querySelectorAll('.nrm-op-btn').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                const newOp = btn.dataset.op;
                const hidden = document.getElementById('nrmOperation');
                if (hidden) hidden.value = newOp;
                document.querySelectorAll('.nrm-op-section').forEach(s => {
                    s.classList.toggle('active', s.dataset.section === newOp);
                });
            });
        });

        // Post-processing toggle
        const postHdr = document.getElementById('nrmPostHdr');
        const postBody = document.getElementById('nrmPostBody');
        const postArrow = document.getElementById('nrmPostArrow');
        if (postHdr && postBody) {
            postHdr.addEventListener('click', () => {
                const open = postBody.classList.toggle('open');
                if (postArrow) postArrow.style.transform = open ? 'rotate(90deg)' : '';
            });
            if (postBody.classList.contains('open') && postArrow) {
                postArrow.style.transform = 'rotate(90deg)';
            }
        }

        // Chip inputs
        ['nrmNormalizeFields', 'nrmPreserveFields', 'nrmGroupBy'].forEach(id => {
            this._initChipInput(id);
        });

        // Rename rows: add + remove
        const addBtn = document.getElementById('nrmAddRename');
        if (addBtn) {
            addBtn.addEventListener('click', () => {
                const container = document.getElementById('nrmRenameRows');
                if (container) container.insertAdjacentHTML('beforeend', this._renameRow('', ''));
                this._wireRenameRemove();
            });
        }
        this._wireRenameRemove();

        // FieldPathSearchComponent on source + output fields
        this._initFieldPicker('nrmSourceField', 'e.g. steps.file_parser.step_output.records');
        this._initFieldPicker('nrmOutputField', 'e.g. normalized_data  (blank = overwrite source)');

        // Column-name smart suggestions on per-operation text inputs + chip inputs.
        // _getSourceColumns() is called FRESH on every focus — no caching — so it
        // always reflects the current sourceField value even when the path was chosen
        // via FieldPathSearchComponent (which sets el.value without firing a DOM event).
        ['nrmPivotField', 'nrmValueField', 'nrmKeyColumn', 'nrmValueColumn'].forEach(id => {
            this._initColumnPicker(id);
        });
        ['nrmGroupBy', 'nrmNormalizeFields', 'nrmPreserveFields'].forEach(id => {
            this._initChipSuggest(id);
        });
    }

    _initChipInput(id) {
        const wrap  = document.getElementById(id + 'Wrap');
        const input = document.getElementById(id + 'Input');
        const json  = document.getElementById(id + 'Json');
        if (!wrap || !input || !json) return;

        const getValues = () => {
            try { return JSON.parse(json.value || '[]'); } catch { return []; }
        };
        const setValues = arr => { json.value = JSON.stringify(arr); };
        const addValue = val => {
            val = val.trim();
            if (!val) return;
            const arr = getValues();
            if (!arr.includes(val)) { arr.push(val); setValues(arr); }
            wrap.insertBefore(this._parseChip(val), input);
            input.value = '';
        };
        const removeValue = val => {
            const arr = getValues().filter(v => v !== val);
            setValues(arr);
        };

        wrap.addEventListener('click', e => {
            if (e.target.classList.contains('nrm-chip-x')) {
                const chip = e.target.closest('.nrm-chip');
                if (chip) {
                    removeValue(chip.dataset.val);
                    chip.remove();
                }
            } else {
                input.focus();
            }
        });

        input.addEventListener('keydown', e => {
            if ((e.key === 'Enter' || e.key === ',') && input.value.trim()) {
                e.preventDefault();
                addValue(input.value);
            } else if (e.key === 'Backspace' && !input.value) {
                const chips = wrap.querySelectorAll('.nrm-chip');
                if (chips.length) {
                    const last = chips[chips.length - 1];
                    removeValue(last.dataset.val);
                    last.remove();
                }
            }
        });
        input.addEventListener('blur', () => {
            if (input.value.trim()) addValue(input.value);
        });
    }

    _parseChip(val) {
        const span = document.createElement('span');
        span.className = 'nrm-chip';
        span.dataset.val = val;
        span.innerHTML = `${this._esc(val)}<span class="nrm-chip-x" title="Remove">×</span>`;
        return span;
    }

    _wireRenameRemove() {
        document.querySelectorAll('#nrmRenameRows .nrm-rm-btn').forEach(btn => {
            btn.onclick = () => btn.closest('.nrm-rename-row')?.remove();
        });
    }

    _initFieldPicker(inputId, placeholder) {
        const el = document.getElementById(inputId);
        if (!el || typeof FieldPathSearchComponent === 'undefined') return;
        const ac = new FieldPathSearchComponent(el, {
            includeHL7Fields: false,
            allowCustom: true,
            showCategories: true,
            placeholder,
            getStepVariables: () => this._panel.getStepVariablesForSearch
                ? this._panel.getStepVariablesForSearch()
                : [],
            onSelect: path => { el.value = path; }
        });
        this._acs.push(ac);
    }

    // ─── Private: read helpers ───────────────────────────────────────────────

    _readChips(id) {
        try { return JSON.parse(document.getElementById(id + 'Json')?.value || '[]'); } catch { return []; }
    }

    _readRenameMap() {
        const map = {};
        document.querySelectorAll('#nrmRenameRows .nrm-rename-row').forEach(row => {
            const from = row.querySelector('.nrm-rename-from')?.value?.trim();
            const to   = row.querySelector('.nrm-rename-to')?.value?.trim();
            if (from && to) map[from] = to;
        });
        return map;
    }

    // ─── Column-name suggestions ─────────────────────────────────────────────

    /**
     * Resolve the current sourceField path against the last test run output and
     * return the column names from the first element of the resulting array.
     * Returns [] when the path can't be resolved or the result is not an array.
     *
     * Path format: "steps.stepName.step_output.fieldName"
     * Test output shape: window.pipelineLastTestOutput.steps.{stepNs}.{field}
     */
    _getSourceColumns() {
        const srcPath = document.getElementById('nrmSourceField')?.value?.trim() || '';
        if (!srcPath) return [];

        let root = null;
        try {
            root = window.pipelineLastTestOutput;
            if (!root) {
                const stored = localStorage.getItem('pipeline_last_test_output');
                if (stored) root = JSON.parse(stored);
            }
        } catch (_) {}
        if (!root || typeof root !== 'object') return [];

        // Strip leading "steps." so we can walk the rest against testOutput.steps
        let path = srcPath.replace(/^steps\./, '');
        let node = root.steps;

        for (const part of path.split('.')) {
            if (node == null || typeof node !== 'object') return [];
            // Handle array index notation: fieldName[0]
            const arrMatch = part.match(/^(.+)\[(\d+)\]$/);
            if (arrMatch) {
                node = node[arrMatch[1]];
                if (Array.isArray(node)) node = node[parseInt(arrMatch[2], 10)];
            } else {
                node = node[part];
            }
        }

        if (!Array.isArray(node) || node.length === 0) return [];
        const first = node[0];
        if (!first || typeof first !== 'object') return [];
        return Object.keys(first);
    }

    /**
     * Wire a column-name suggest dropdown to a plain text input.
     * Dropdown is appended to document.body and positioned via getBoundingClientRect()
     * so it is never clipped by the panel's overflow:hidden containers.
     * _getSourceColumns() is called fresh on every open so the list always reflects
     * whatever path is currently typed/selected in the Source field.
     */
    _initColumnPicker(inputId) {
        const el = document.getElementById(inputId);
        if (!el) return;

        let dropEl = null;

        const positionDrop = () => {
            if (!dropEl) return;
            const r = el.getBoundingClientRect();
            dropEl.style.top    = (r.bottom + window.scrollY + 2) + 'px';
            dropEl.style.left   = (r.left   + window.scrollX)     + 'px';
            dropEl.style.width  = r.width + 'px';
        };

        const closeDrop = () => {
            if (dropEl) { try { dropEl.remove(); } catch (_) {} dropEl = null; }
        };

        const openDrop = () => {
            closeDrop();
            const cols = this._getSourceColumns();
            if (!cols.length) return;
            const q = el.value.toLowerCase();
            const matches = cols.filter(c => !q || c.toLowerCase().includes(q));
            if (!matches.length) return;

            dropEl = document.createElement('div');
            dropEl.className = 'nrm-col-suggest';
            dropEl.style.position = 'fixed';

            matches.forEach(col => {
                const item = document.createElement('div');
                item.className = 'nrm-col-suggest-item';
                item.innerHTML = `<span>${this._esc(col)}</span><span class="nrm-col-tag">col</span>`;
                item.addEventListener('mousedown', e => {
                    e.preventDefault();
                    el.value = col;
                    el.dispatchEvent(new Event('input', { bubbles: true }));
                    closeDrop();
                });
                dropEl.appendChild(item);
            });

            document.body.appendChild(dropEl);
            positionDrop();
        };

        el.addEventListener('focus', openDrop);
        el.addEventListener('input', openDrop);
        el.addEventListener('blur',  () => setTimeout(closeDrop, 160));
    }

    /**
     * Add column-name suggestions to a chip input's typing field.
     * Dropdown is appended to document.body (not the wrap) to avoid overflow clipping.
     * Already-added chips are excluded from the suggestion list.
     * Clicking a suggestion adds the chip (same as pressing Enter).
     */
    _initChipSuggest(chipId) {
        const wrap  = document.getElementById(chipId + 'Wrap');
        const input = document.getElementById(chipId + 'Input');
        if (!wrap || !input) return;

        let dropEl = null;

        const getValues = () => {
            try { return JSON.parse(document.getElementById(chipId + 'Json')?.value || '[]'); }
            catch (_) { return []; }
        };
        const setValues = arr => {
            const j = document.getElementById(chipId + 'Json');
            if (j) j.value = JSON.stringify(arr);
        };
        const addChip = val => {
            val = val.trim();
            if (!val) return;
            const arr = getValues();
            if (arr.includes(val)) { input.value = ''; closeDrop(); return; }
            arr.push(val);
            setValues(arr);
            wrap.insertBefore(this._parseChip(val), input);
            input.value = '';
            closeDrop();
        };

        const positionDrop = () => {
            if (!dropEl) return;
            const r = wrap.getBoundingClientRect();
            dropEl.style.top   = (r.bottom + window.scrollY + 2) + 'px';
            dropEl.style.left  = (r.left   + window.scrollX)     + 'px';
            dropEl.style.width = r.width + 'px';
        };

        const closeDrop = () => {
            if (dropEl) { try { dropEl.remove(); } catch (_) {} dropEl = null; }
        };

        const openDrop = () => {
            closeDrop();
            const cols = this._getSourceColumns();
            if (!cols.length) return;
            const q = input.value.toLowerCase();
            const existing = new Set(getValues());
            const matches = cols.filter(c => !existing.has(c) && (!q || c.toLowerCase().includes(q)));
            if (!matches.length) return;

            dropEl = document.createElement('div');
            dropEl.className = 'nrm-col-suggest';
            dropEl.style.position = 'fixed';

            matches.forEach(col => {
                const item = document.createElement('div');
                item.className = 'nrm-col-suggest-item';
                item.innerHTML = `<span>${this._esc(col)}</span><span class="nrm-col-tag">col</span>`;
                item.addEventListener('mousedown', e => {
                    e.preventDefault();
                    addChip(col);
                });
                dropEl.appendChild(item);
            });

            document.body.appendChild(dropEl);
            positionDrop();
        };

        input.addEventListener('focus', openDrop);
        input.addEventListener('input', openDrop);
        input.addEventListener('blur',  () => setTimeout(closeDrop, 160));
    }

    _esc(str) {
        return String(str || '').replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
    }
}

// ─── Static Data (ES6 compatible — no static class fields) ───────────────────

NormalizerBuilder.OPERATIONS = [
    {
        value:   'normalize',
        name:    'Normalize',
        icon:    '↕',
        desc:    'Columns → rows',
        tooltip: 'Unpivot: turn selected columns into key/value row pairs'
    },
    {
        value:   'pivot',
        name:    'Pivot',
        icon:    '↔',
        desc:    'Rows → columns',
        tooltip: 'Pivot: unique values in a column become new column headers'
    },
    {
        value:   'transpose',
        name:    'Transpose',
        icon:    '⤢',
        desc:    'Flip matrix',
        tooltip: 'Transpose: swap rows and columns of a 2D dataset'
    },
    {
        value:   'flatten',
        name:    'Flatten',
        icon:    '⬇',
        desc:    'Nested → flat',
        tooltip: 'Flatten: collapse nested JSON to dot-notation keys'
    },
    {
        value:   'unflatten',
        name:    'Unflatten',
        icon:    '⬆',
        desc:    'Flat → nested',
        tooltip: 'Unflatten: rebuild nested JSON from dot-notation keys'
    },
];

// ─── Registration ─────────────────────────────────────────────────────────────
StepBuilderRegistry.register('normalizer', NormalizerBuilder);
