/**
 * @fileoverview DataMaskingBuilder — no-code UI for the Data Masking/Anonymization pipeline step.
 * @version 1.0.0
 *
 * Design principles:
 *   - SOLID: each method has a single responsibility, strategies are Open/Closed via a static Map
 *   - Encapsulation: all internal state is private (_panel, _autocompletes)
 *   - Dependency Inversion: registered via StepBuilderRegistry abstraction
 *   - Follows the same patterns as RemoveDuplicatesBuilder.js and FileParserBuilder.js
 */

/**
 * @class DataMaskingBuilder
 * @classdesc No-code builder for configuring field-level PHI/PII masking rules.
 * Registered with StepBuilderRegistry under the 'data_masking' key.
 *
 * StepBuilderRegistry contract:
 *   render(step)        → {string}  HTML to inject into PropertiesPanel
 *   collectConfig(step) → {void}    Write DOM state back to step.config
 *   destroy()           → {void}    Release all resources (ACs, listeners)
 *
 * @example
 * // PropertiesPanel dispatches automatically via VisualStep.isDataMasking() guard.
 * // Manual usage:
 * const builder = StepBuilderRegistry.create('data_masking', panel);
 * container.innerHTML = builder.render(step);
 */
class DataMaskingBuilder {

    // ─────────────────────────────────────────────────────────────────────────
    // Constructor
    // ─────────────────────────────────────────────────────────────────────────

    /**
     * @param {Object} panel - PropertiesPanel instance (used for context — builder does not call panel methods)
     */
    constructor(panel) {
        /** @private @type {Object} */
        this._panel = panel;
        /** @private @type {Array} — autocomplete instances to destroy on cleanup (legacy) */
        this._autocompletes = [];
        /** @private @type {Object.<number, FieldPathSearchComponent>} — per-row field pickers */
        this._rowACs = {};
        /** @private @type {Function|null} — bound reference for cleanup */
        this._boundAddRule = null;
    }

    // ─────────────────────────────────────────────────────────────────────────
    // Public Builder Contract
    // ─────────────────────────────────────────────────────────────────────────

    /**
     * Renders the no-code masking UI HTML.
     * Schedules async event wiring via setTimeout so the DOM is ready when called.
     * @param {Object} step - VisualStep with step.config
     * @returns {string} HTML string to inject into PropertiesPanel
     */
    render(step) {
        if (!step.config) step.config = {};
        if (!Array.isArray(step.config.rules)) step.config.rules = [];
        const html = this._buildHTML(step);
        setTimeout(() => this._attachEvents(step), 0);
        return html;
    }

    /**
     * Reads current DOM state and writes it back to step.config.
     * Called by PropertiesPanel.collectFormData() before any pipeline save.
     * @param {Object} step - VisualStep reference (mutated in place)
     * @returns {void}
     */
    collectConfig(step) {
        if (!step.config) step.config = {};
        step.config.rules             = this._readRulesFromDOM();
        step.config.maskAllPHI        = document.getElementById('dmMaskAllPHI')?.checked ?? false;
        step.config.preserveFormat    = document.getElementById('dmPreserveFormat')?.checked ?? false;
        // Read active format from top-level tabs (preferred), fall back to PHI section tabs
        const topTab = document.querySelector('#dmTopFormatTabs .dm-top-format-tab.active');
        const phiTab = document.querySelector('#dmFormatTabs .dm-format-tab.active');
        step.config.maskAllPHIFormat  = (topTab?.dataset.format) || (phiTab?.dataset.format) || step.config.maskAllPHIFormat || 'hl7v2';
    }

    /**
     * Releases all resources created by this builder instance.
     * Called by PropertiesPanel before switching to a different step or closing the panel.
     * @returns {void}
     */
    destroy() {
        for (const ac of this._autocompletes) {
            try { ac.destroy(); } catch (_) {}
        }
        this._autocompletes = [];
        for (const ac of Object.values(this._rowACs)) {
            try { ac.destroy(); } catch (_) {}
        }
        this._rowACs = {};
        this._boundAddRule = null;
    }

    // ─────────────────────────────────────────────────────────────────────────
    // Private HTML Construction
    // ─────────────────────────────────────────────────────────────────────────

    /**
     * Builds the complete form HTML for the Data Masking step.
     * @private
     * @param {Object} step - VisualStep with step.config
     * @returns {string} Complete HTML string
     */
    _buildHTML(step) {
        const cfg = step.config;
        const rules = cfg.rules || [];
        const format = cfg.maskAllPHIFormat || 'hl7v2';

        const rulesHtml = rules.length > 0
            ? rules.map((rule, i) => this._buildRuleRow(rule, i, format)).join('')
            : this._buildEmptyState();

        return `
<div class="dm-builder">
  <style>
    .dm-builder { font-size: 13px; }

    /* ── Section Header ── */
    .dm-section-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 10px 12px 8px;
      background: rgba(30,58,138,0.05);
      border: 1px solid rgba(30,58,138,0.15);
      border-radius: 6px 6px 0 0;
      margin-bottom: 0;
    }
    .dm-section-title {
      font-weight: 600;
      color: var(--primary-color, #1e3a8a);
      font-size: 13px;
      display: flex;
      align-items: center;
      gap: 6px;
    }
    .dm-add-btn {
      display: flex;
      align-items: center;
      gap: 5px;
      padding: 5px 10px;
      background: var(--primary-color, #1e3a8a);
      color: #fff;
      border: none;
      border-radius: 5px;
      font-size: 12px;
      font-weight: 500;
      cursor: pointer;
      transition: background 0.15s;
    }
    .dm-add-btn:hover { background: var(--primary-hover, #1e40af); }

    /* ── Rules Table ── */
    .dm-rules-container {
      border: 1px solid rgba(30,58,138,0.15);
      border-top: none;
      border-radius: 0 0 6px 6px;
      overflow: hidden;
      margin-bottom: 12px;
    }
    .dm-rules-table {
      width: 100%;
      border-collapse: collapse;
    }
    .dm-rules-table thead th {
      padding: 7px 10px;
      background: rgba(30,58,138,0.04);
      font-size: 11px;
      font-weight: 600;
      color: #64748b;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      border-bottom: 1px solid rgba(30,58,138,0.10);
      white-space: nowrap;
    }
    .dm-rules-table tbody tr {
      border-bottom: 1px solid rgba(30,58,138,0.08);
      transition: background 0.1s;
    }
    .dm-rules-table tbody tr:last-child { border-bottom: none; }
    .dm-rules-table tbody tr:hover { background: rgba(30,58,138,0.02); }
    .dm-rules-table td { padding: 7px 8px; vertical-align: middle; }

    /* ── Field inputs inside rules ── */
    .dm-field-input, .dm-strategy-select, .dm-small-input {
      width: 100%;
      padding: 5px 8px;
      border: 1px solid rgba(30,58,138,0.20);
      border-radius: 4px;
      font-size: 12px;
      background: #fff;
      color: #1e293b;
      outline: none;
      transition: border-color 0.15s;
    }
    .dm-field-input:focus, .dm-strategy-select:focus, .dm-small-input:focus {
      border-color: var(--primary-color, #1e3a8a);
      box-shadow: 0 0 0 2px rgba(30,58,138,0.08);
    }
    .dm-small-input { width: 68px; text-align: center; }
    .dm-strategy-select { min-width: 100px; }

    /* ── Conditional fields cell ── */
    .dm-options-cell { min-width: 160px; }
    .dm-options-group {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }
    .dm-option-label {
      font-size: 11px;
      color: #64748b;
      white-space: nowrap;
    }
    .dm-option-block { display: flex; align-items: center; gap: 4px; }
    .dm-hidden { display: none !important; }

    /* ── Remove button ── */
    .dm-remove-btn {
      padding: 4px 7px;
      background: transparent;
      border: 1px solid rgba(239,68,68,0.30);
      border-radius: 4px;
      color: #ef4444;
      cursor: pointer;
      font-size: 12px;
      transition: all 0.15s;
      white-space: nowrap;
    }
    .dm-remove-btn:hover { background: rgba(239,68,68,0.08); border-color: #ef4444; }

    /* ── Empty state ── */
    .dm-empty-state {
      padding: 20px;
      text-align: center;
      color: #94a3b8;
      font-size: 12px;
    }

    /* ── PHI Toggle section ── */
    .dm-phi-section {
      border: 1px solid rgba(30,58,138,0.15);
      border-radius: 6px;
      margin-bottom: 10px;
      overflow: hidden;
    }
    .dm-phi-header {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 10px 12px;
      background: rgba(30,58,138,0.04);
      cursor: pointer;
      user-select: none;
    }
    .dm-phi-header:hover { background: rgba(30,58,138,0.07); }
    .dm-phi-toggle-label {
      font-weight: 500;
      color: #1e293b;
      font-size: 13px;
    }
    .dm-phi-badge {
      font-size: 10px;
      background: rgba(30,58,138,0.10);
      color: var(--primary-color, #1e3a8a);
      padding: 2px 7px;
      border-radius: 10px;
      font-weight: 600;
    }
    .dm-phi-chips {
      padding: 8px 12px 10px;
      display: flex;
      flex-direction: column;
      gap: 10px;
      border-top: 1px solid rgba(30,58,138,0.10);
      background: #fff;
    }
    /* ── PHI category grouping ── */
    .dm-phi-category { width: 100%; }
    .dm-phi-cat-label {
      display: block;
      font-size: 10px;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: var(--primary-color, #1e3a8a);
      opacity: 0.70;
      margin-bottom: 5px;
    }
    .dm-phi-cat-chips {
      display: flex;
      flex-wrap: wrap;
      gap: 5px;
    }
    .dm-phi-chip {
      display: flex;
      align-items: center;
      gap: 5px;
      padding: 3px 9px 3px 7px;
      background: rgba(30,58,138,0.06);
      border: 1px solid rgba(30,58,138,0.18);
      border-radius: 12px;
      font-size: 11px;
      color: #1e293b;
      white-space: nowrap;
    }
    .dm-phi-chip-field {
      font-weight: 600;
      color: var(--primary-color, #1e3a8a);
    }
    .dm-phi-chip-strategy {
      color: #64748b;
    }

    /* ── Format selector ── */
    .dm-format-selector {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 8px 12px 6px;
      border-bottom: 1px solid rgba(30,58,138,0.08);
    }
    .dm-format-label {
      font-size: 11px;
      font-weight: 600;
      color: #64748b;
      white-space: nowrap;
      text-transform: uppercase;
      letter-spacing: 0.04em;
    }
    .dm-format-tabs { display: flex; gap: 4px; }
    .dm-format-tab {
      padding: 3px 10px;
      font-size: 11px;
      font-weight: 500;
      border: 1px solid rgba(30,58,138,0.20);
      border-radius: 4px;
      background: #fff;
      color: #64748b;
      cursor: pointer;
      transition: all 0.15s;
    }
    .dm-format-tab:hover { border-color: var(--primary-color,#1e3a8a); color: var(--primary-color,#1e3a8a); }
    .dm-format-tab.active {
      background: var(--primary-color,#1e3a8a);
      border-color: var(--primary-color,#1e3a8a);
      color: #fff;
    }
    .dm-phi-field-list { padding: 10px 12px 12px; }

    /* ── Advanced section ── */
    .dm-advanced {
      border: 1px solid rgba(30,58,138,0.12);
      border-radius: 6px;
      margin-bottom: 4px;
    }
    .dm-advanced summary {
      padding: 9px 12px;
      cursor: pointer;
      font-size: 12px;
      color: #64748b;
      font-weight: 500;
      user-select: none;
      list-style: none;
    }
    .dm-advanced summary::-webkit-details-marker { display: none; }
    .dm-advanced summary::before {
      content: '▸ ';
      font-size: 10px;
      transition: transform 0.15s;
    }
    .dm-advanced[open] summary::before { content: '▾ '; }
    .dm-advanced-body {
      padding: 4px 12px 12px;
      border-top: 1px solid rgba(30,58,138,0.10);
    }
    .dm-checkbox-row {
      display: flex;
      align-items: flex-start;
      gap: 8px;
      margin-top: 10px;
    }
    .dm-checkbox-row input[type=checkbox] { margin-top: 2px; accent-color: var(--primary-color, #1e3a8a); }
    .dm-checkbox-desc {
      font-size: 12px;
      color: #475569;
      line-height: 1.5;
    }
    .dm-checkbox-desc strong { color: #1e293b; }

    /* ── Target Format Banner ── */
    .dm-format-banner {
      display: flex;
      align-items: flex-start;
      gap: 10px;
      padding: 10px 12px;
      background: rgba(30,58,138,0.04);
      border: 1px solid rgba(30,58,138,0.15);
      border-radius: 6px;
      margin-bottom: 10px;
      flex-wrap: wrap;
    }
    .dm-format-banner-label {
      font-size: 12px;
      font-weight: 600;
      color: #1e3a8a;
      white-space: nowrap;
      padding-top: 2px;
    }
    .dm-top-format-tabs {
      display: flex;
      gap: 4px;
      flex-wrap: wrap;
    }
    .dm-top-format-tab {
      padding: 3px 10px;
      font-size: 11px;
      font-weight: 500;
      border: 1px solid rgba(30,58,138,0.3);
      border-radius: 4px;
      background: #fff;
      color: #475569;
      cursor: pointer;
      transition: all 0.15s;
    }
    .dm-top-format-tab.active {
      background: #1e3a8a;
      color: #fff;
      border-color: #1e3a8a;
    }
    .dm-placement-hint {
      width: 100%;
      font-size: 11px;
      color: #64748b;
      margin-top: 4px;
      display: flex;
      gap: 6px;
      align-items: flex-start;
    }
    .dm-placement-hint i { color: #f59e0b; margin-top: 1px; flex-shrink: 0; }
    .dm-placement-hint b { color: #1e293b; }
  </style>

  <!-- ── Target Format Selector ── -->
  <div class="dm-format-banner">
    <span class="dm-format-banner-label">Target Format:</span>
    <div class="dm-top-format-tabs" id="dmTopFormatTabs">
      <button class="dm-top-format-tab ${format === 'hl7v2' ? 'active' : ''}" data-format="hl7v2">HL7 v2</button>
      <button class="dm-top-format-tab ${format === 'fhir'  ? 'active' : ''}" data-format="fhir">FHIR R4</button>
      <button class="dm-top-format-tab ${format === 'json'  ? 'active' : ''}" data-format="json">Generic JSON</button>
    </div>
    <div class="dm-placement-hint" id="dmPlacementHint">
      ${this._buildPlacementHint(format)}
    </div>
  </div>

  <!-- ── Rules Header ── -->
  <div class="dm-section-header">
    <span class="dm-section-title">
      <i class="fas fa-lock" style="font-size:12px;"></i>
      Masking Rules
    </span>
    <button class="dm-add-btn" id="dmAddRuleBtn">
      <i class="fas fa-plus"></i> Add Rule
    </button>
  </div>

  <!-- ── Rules Table ── -->
  <div class="dm-rules-container">
    <table class="dm-rules-table" id="dmRulesTable">
      <thead>
        <tr>
          <th style="width:32%">Field Path</th>
          <th style="width:16%">Strategy</th>
          <th>Options</th>
          <th style="width:42px"></th>
        </tr>
      </thead>
      <tbody id="dmRulesTbody">
        ${rulesHtml}
      </tbody>
    </table>
  </div>

  <!-- ── Mask All PHI Toggle ── -->
  <div class="dm-phi-section">
    <label class="dm-phi-header" for="dmMaskAllPHI">
      <input type="checkbox" id="dmMaskAllPHI" ${cfg.maskAllPHI ? 'checked' : ''}
             style="accent-color: var(--primary-color, #1e3a8a); margin:0;">
      <span class="dm-phi-toggle-label">Mask All PHI</span>
      <span class="dm-phi-badge">HIPAA Safe Harbor · 13 identifiers</span>
    </label>
    <div id="dmPhiChips" ${cfg.maskAllPHI ? '' : 'style="display:none"'}>
      <div class="dm-format-selector">
        <span class="dm-format-label">Format:</span>
        <div class="dm-format-tabs" id="dmFormatTabs">
          <button class="dm-format-tab ${format === 'hl7v2' ? 'active' : ''}" data-format="hl7v2">HL7 v2</button>
          <button class="dm-format-tab ${format === 'fhir'  ? 'active' : ''}" data-format="fhir">FHIR R4</button>
          <button class="dm-format-tab ${format === 'json'  ? 'active' : ''}" data-format="json">Generic JSON</button>
        </div>
      </div>
      <div class="dm-phi-chips dm-phi-field-list" id="dmPhiFieldList">
        ${this._renderPhiPreview(format)}
      </div>
    </div>
  </div>

  <!-- ── Advanced Options ── -->
  <details class="dm-advanced" ${cfg.preserveFormat ? 'open' : ''}>
    <summary>Advanced Options</summary>
    <div class="dm-advanced-body">
      <div class="dm-checkbox-row">
        <input type="checkbox" id="dmPreserveFormat" ${cfg.preserveFormat ? 'checked' : ''}>
        <div class="dm-checkbox-desc">
          <strong>Preserve Format</strong><br>
          For <em>Partial</em> strategy: mask digit characters only, keep separators in place.<br>
          Example: <code>555-867-5309</code> with keepLast=4 → <code>***-***-5309</code>
        </div>
      </div>
    </div>
  </details>
</div>`;
    }

    /**
     * Renders a single masking rule row for the rules table.
     * @private
     * @param {Object} rule  - maskingRule config object
     * @param {number} idx   - Row index (used for unique element IDs)
     * @returns {string} HTML for one <tr>
     */
    _buildRuleRow(rule, idx, format) {
        const stratOpts = this._buildStrategyOptions(rule.strategy || 'mask');
        const condFields = this._buildConditionalFields(rule, idx);
        const placeholder = this._getFieldPlaceholder(format || 'hl7v2');
        return `
<tr data-rule-idx="${idx}" id="dmRuleRow_${idx}">
  <td>
    <input type="text" class="dm-field-input" id="dmField_${idx}"
           placeholder="${placeholder}"
           value="${this._esc(rule.field || '')}">
  </td>
  <td>
    <select class="dm-strategy-select" id="dmStrategy_${idx}">
      ${stratOpts}
    </select>
  </td>
  <td class="dm-options-cell">
    ${condFields}
  </td>
  <td>
    <button class="dm-remove-btn" data-remove-idx="${idx}" title="Remove rule">
      <i class="fas fa-times"></i>
    </button>
  </td>
</tr>`;
    }

    /**
     * Renders <option> elements for the strategy <select> from MASKING_STRATEGIES Map.
     * Satisfies OCP — iterates the Map without hard-coding strategy names.
     * @private
     * @param {string} selected - Currently selected strategy key
     * @returns {string} HTML options string
     */
    _buildStrategyOptions(selected) {
        let html = '';
        for (const [key, meta] of DataMaskingBuilder.MASKING_STRATEGIES) {
            const sel = (key === selected || (!selected && key === 'mask')) ? 'selected' : '';
            html += `<option value="${key}" ${sel} title="${this._esc(meta.description)}">${meta.label}</option>`;
        }
        return html;
    }

    /**
     * Renders the conditional config fields for a rule row.
     * Visibility of each field group is determined by the strategy's declared fields list.
     * @private
     * @param {Object} rule - current rule config
     * @param {number} idx  - row index for unique IDs
     * @returns {string} HTML for the options cell content
     */
    _buildConditionalFields(rule, idx) {
        const strategy = rule.strategy || 'mask';
        const stratMeta = DataMaskingBuilder.MASKING_STRATEGIES.get(strategy) || { fields: [] };
        const hasField = (f) => stratMeta.fields.includes(f);

        const maskCharHidden  = hasField('maskChar')       ? '' : 'dm-hidden';
        const keepFirstHidden = hasField('keepFirst')      ? '' : 'dm-hidden';
        const keepLastHidden  = hasField('keepLast')       ? '' : 'dm-hidden';
        const hashSaltHidden  = hasField('hashSalt')       ? '' : 'dm-hidden';
        const subTypeHidden   = hasField('substituteType') ? '' : 'dm-hidden';
        // SubstituteValue is shown only when strategy=substitute AND type=custom
        const curSubType      = rule.substituteType || 'name';
        const subValueHidden  = (hasField('substituteType') && curSubType === 'custom') ? '' : 'dm-hidden';

        const subTypeOptions = ['name', 'ssn', 'phone', 'email', 'date', 'address', 'custom']
            .map(t => `<option value="${t}" ${curSubType === t ? 'selected' : ''}>${t.charAt(0).toUpperCase() + t.slice(1)}${t === 'custom' ? '…' : ''}</option>`)
            .join('');

        return `
<div class="dm-options-group">
  <div class="dm-option-block ${maskCharHidden}" id="dmMaskCharBlock_${idx}">
    <span class="dm-option-label">Char</span>
    <input type="text" class="dm-small-input" id="dmMaskChar_${idx}"
           maxlength="1" placeholder="*"
           value="${this._esc(rule.maskChar || '*')}"
           style="width:44px;">
  </div>
  <div class="dm-option-block ${keepFirstHidden}" id="dmKeepFirstBlock_${idx}">
    <span class="dm-option-label">First</span>
    <input type="number" class="dm-small-input" id="dmKeepFirst_${idx}"
           min="0" value="${rule.keepFirst ?? 0}">
  </div>
  <div class="dm-option-block ${keepLastHidden}" id="dmKeepLastBlock_${idx}">
    <span class="dm-option-label">Last</span>
    <input type="number" class="dm-small-input" id="dmKeepLast_${idx}"
           min="0" value="${rule.keepLast ?? 4}">
  </div>
  <div class="dm-option-block ${hashSaltHidden}" id="dmHashSaltBlock_${idx}">
    <span class="dm-option-label">Salt</span>
    <input type="text" class="dm-field-input" id="dmHashSalt_${idx}"
           placeholder="optional salt"
           value="${this._esc(rule.hashSalt || '')}"
           style="width:110px;">
  </div>
  <div class="dm-option-block ${subTypeHidden}" id="dmSubTypeBlock_${idx}">
    <span class="dm-option-label">Type</span>
    <select class="dm-strategy-select" id="dmSubType_${idx}" style="min-width:105px;">
      ${subTypeOptions}
    </select>
  </div>
  <div class="dm-option-block ${subValueHidden}" id="dmSubValueBlock_${idx}">
    <span class="dm-option-label">Value</span>
    <input type="text" class="dm-field-input" id="dmSubValue_${idx}"
           placeholder="custom text"
           value="${this._esc(rule.substituteValue || '')}"
           style="width:90px;">
  </div>
  <span class="dm-option-label" id="dmNoOptions_${idx}"
        ${(hashSaltHidden && maskCharHidden && keepFirstHidden && keepLastHidden && subTypeHidden)
            ? '' : 'style="display:none"'}
        style="color:#94a3b8;font-style:italic;">
    —
  </span>
</div>`;
    }

    /**
     * Renders the "empty state" row shown when no rules have been added yet.
     * @private
     * @returns {string} HTML for the empty state
     */
    _buildEmptyState() {
        return `
<tr id="dmEmptyState">
  <td colspan="4">
    <div class="dm-empty-state">
      <i class="fas fa-lock-open" style="font-size:20px;margin-bottom:6px;opacity:0.4;"></i><br>
      No masking rules yet — click <strong>+ Add Rule</strong> to start,<br>
      or enable <strong>Mask All PHI</strong> below for HIPAA auto-rules.
    </div>
  </td>
</tr>`;
    }

    /**
     * Renders PHI field chips for the given data format, grouped by hipaaCategory.
     * Each category gets a labelled section with its chips underneath.
     * @private
     * @param {string} [format='hl7v2'] - 'hl7v2' | 'fhir' | 'json'
     * @returns {string} HTML for grouped category sections
     */
    _renderPhiPreview(format) {
        format = format || 'hl7v2';
        // Group fields by hipaaCategory, preserving insertion order (Map is ordered)
        const groups = new Map();
        for (const f of DataMaskingBuilder.PHI_FIELDS.filter(f => f.format === format)) {
            const cat = f.hipaaCategory || 'Other';
            if (!groups.has(cat)) {
                groups.set(cat, { hipaaId: f.hipaaId, fields: [] });
            }
            groups.get(cat).fields.push(f);
        }

        let html = '';
        for (const [cat, group] of groups) {
            const chips = group.fields.map(f => `<span class="dm-phi-chip" title="${this._esc(f.description)}"><span class="dm-phi-chip-field">${this._esc(f.field)}</span><span class="dm-phi-chip-strategy"> · ${f.strategy}</span></span>`).join('');
            html += `<div class="dm-phi-category"><span class="dm-phi-cat-label">#${group.hipaaId} — ${this._esc(cat)}</span><div class="dm-phi-cat-chips">${chips}</div></div>`;
        }
        return html;
    }

    /**
     * Attaches a FieldPathSearchComponent to the field path input of a rule row.
     * Destroys any previous instance for the same row index before creating a new one.
     * Falls back gracefully when FieldPathSearchComponent is unavailable.
     * @private
     * @param {number} idx - Row index matching dmField_{idx}
     */
    _initFieldSearchForRow(idx) {
        const input = document.getElementById(`dmField_${idx}`);
        if (!input) return;

        // Destroy any previous instance for this row
        if (this._rowACs[idx]) {
            try { this._rowACs[idx].destroy(); } catch (_) {}
            delete this._rowACs[idx];
        }

        if (typeof FieldPathSearchComponent === 'undefined') return;

        const ac = new FieldPathSearchComponent(input, {
            onSelect: (fieldPath) => { input.value = fieldPath; },
            placeholder: 'Search fields… or click to browse step outputs',
            allowCustom: true,
            showCategories: true,
            maxSuggestions: 20,
            getStepVariables: () => {
                if (this._panel && typeof this._panel.getStepVariablesForSearch === 'function') {
                    return this._panel.getStepVariablesForSearch();
                }
                return [];
            },
        });
        this._rowACs[idx] = ac;
    }

    // ─────────────────────────────────────────────────────────────────────────
    // Private Event Wiring
    // ─────────────────────────────────────────────────────────────────────────

    /**
     * Wires all event listeners. Called via setTimeout(0) from render()
     * to ensure the DOM is ready.
     * @private
     * @param {Object} step - VisualStep reference (used by add-rule handler)
     * @returns {void}
     */
    _attachEvents(step) {
        // Add Rule button
        const addBtn = document.getElementById('dmAddRuleBtn');
        if (addBtn) {
            this._boundAddRule = () => this._onAddRule(step);
            addBtn.addEventListener('click', this._boundAddRule);
        }

        // Remove buttons (event delegation on tbody)
        const tbody = document.getElementById('dmRulesTbody');
        if (tbody) {
            tbody.addEventListener('click', (e) => {
                const btn = e.target.closest('[data-remove-idx]');
                if (btn) {
                    const idx = parseInt(btn.dataset.removeIdx, 10);
                    this._onRemoveRule(idx, step);
                }
            });
        }

        // Strategy change + substitute type change (event delegation on tbody)
        if (tbody) {
            tbody.addEventListener('change', (e) => {
                if (e.target.classList.contains('dm-strategy-select')) {
                    const row = e.target.closest('tr[data-rule-idx]');
                    if (row) {
                        const idx = parseInt(row.dataset.ruleIdx, 10);
                        this._onStrategyChange(idx, e.target.value);
                    }
                }
                // Substitute type select: toggle custom value input visibility
                if (e.target.id && e.target.id.startsWith('dmSubType_')) {
                    const idx = parseInt(e.target.id.replace('dmSubType_', ''), 10);
                    if (!isNaN(idx)) this._onSubstituteTypeChange(idx, e.target.value);
                }
            });
        }

        // Mask All PHI toggle
        const phiToggle = document.getElementById('dmMaskAllPHI');
        if (phiToggle) {
            phiToggle.addEventListener('change', () => this._onPhiToggle(phiToggle.checked));
        }

        // Top-level format tabs (target format selector)
        const topFormatTabs = document.getElementById('dmTopFormatTabs');
        if (topFormatTabs) {
            topFormatTabs.addEventListener('click', (e) => {
                const tab = e.target.closest('.dm-top-format-tab');
                if (!tab) return;
                const fmt = tab.dataset.format;
                // Update active state on top tabs
                topFormatTabs.querySelectorAll('.dm-top-format-tab').forEach(t => t.classList.remove('active'));
                tab.classList.add('active');
                // Sync with PHI section tabs if visible
                const phiTab = document.querySelector(`#dmFormatTabs .dm-format-tab[data-format="${fmt}"]`);
                if (phiTab) {
                    document.querySelectorAll('#dmFormatTabs .dm-format-tab').forEach(t => t.classList.remove('active'));
                    phiTab.classList.add('active');
                    this._onPhiFormatChange(fmt);
                }
                // Update placement guidance
                const hint = document.getElementById('dmPlacementHint');
                if (hint) hint.innerHTML = this._buildPlacementHint(fmt);
                // Update field path placeholders
                document.querySelectorAll('.dm-field-input').forEach(inp => {
                    inp.placeholder = this._getFieldPlaceholder(fmt);
                });
            });
        }

        // Format tabs (event delegation — PHI section)
        const formatTabs = document.getElementById('dmFormatTabs');
        if (formatTabs) {
            formatTabs.addEventListener('click', (e) => {
                const tab = e.target.closest('.dm-format-tab');
                if (tab) this._onPhiFormatChange(tab.dataset.format);
            });
        }

        // Initialize variable pickers for all existing rule rows
        const tbody2 = document.getElementById('dmRulesTbody');
        if (tbody2) {
            tbody2.querySelectorAll('tr[data-rule-idx]').forEach(row => {
                this._initFieldSearchForRow(parseInt(row.dataset.ruleIdx, 10));
            });
        }
    }

    /**
     * Appends a new blank rule row to the DOM.
     * Updates step.config.rules in memory so subsequent _readRulesFromDOM() calls are accurate.
     * @private
     * @param {Object} step - VisualStep reference
     * @returns {void}
     */
    _onAddRule(step) {
        const tbody = document.getElementById('dmRulesTbody');
        if (!tbody) return;

        // Remove empty state row if present
        const emptyRow = document.getElementById('dmEmptyState');
        if (emptyRow) emptyRow.remove();

        const newRule = { field: '', strategy: 'mask', maskChar: '*', keepFirst: 0, keepLast: 4 };
        if (!Array.isArray(step.config.rules)) step.config.rules = [];
        step.config.rules.push(newRule);

        const idx = step.config.rules.length - 1;
        const activeTop = document.querySelector('#dmTopFormatTabs .dm-top-format-tab.active');
        const currentFmt = activeTop ? activeTop.dataset.format : 'hl7v2';
        const tempDiv = document.createElement('tbody');
        tempDiv.innerHTML = this._buildRuleRow(newRule, idx, currentFmt);
        const newRow = tempDiv.firstElementChild;
        tbody.appendChild(newRow);

        // Attach variable picker to the new row's field input
        this._initFieldSearchForRow(idx);

        // Focus the field path input for immediate typing
        const fieldInput = document.getElementById(`dmField_${idx}`);
        if (fieldInput) fieldInput.focus();
    }

    /**
     * Removes the rule row at the given index.
     * Re-indexes remaining rows so data-rule-idx attributes stay accurate.
     * @private
     * @param {number} idx  - Index of the row to remove
     * @param {Object} step - VisualStep reference
     * @returns {void}
     */
    _onRemoveRule(idx, step) {
        const row = document.getElementById(`dmRuleRow_${idx}`);
        if (!row) return;
        row.remove();

        // Destroy the variable picker for the removed row
        if (this._rowACs[idx]) {
            try { this._rowACs[idx].destroy(); } catch (_) {}
            delete this._rowACs[idx];
        }

        // Remove from in-memory config and re-index
        if (Array.isArray(step.config.rules)) {
            step.config.rules.splice(idx, 1);
        }

        // Re-index all remaining rows
        const tbody = document.getElementById('dmRulesTbody');
        if (!tbody) return;
        const rows = tbody.querySelectorAll('tr[data-rule-idx]');

        if (rows.length === 0) {
            // Show empty state again
            tbody.innerHTML = this._buildEmptyState();
            return;
        }

        // Re-key _rowACs to match new indices before re-indexing DOM
        const reKeyedACs = {};
        Object.entries(this._rowACs).forEach(([oldIdxStr, ac]) => {
            const oi = parseInt(oldIdxStr, 10);
            if (oi !== idx) {
                reKeyedACs[oi > idx ? oi - 1 : oi] = ac;
            }
        });
        this._rowACs = reKeyedACs;

        rows.forEach((row, newIdx) => {
            row.dataset.ruleIdx = newIdx;
            row.id = `dmRuleRow_${newIdx}`;
            // Re-id all child elements that have index-based IDs
            this._reindexRowElements(row, newIdx);
        });
    }

    /**
     * Re-assigns element IDs in a rule row after re-indexing.
     * @private
     * @param {HTMLElement} row    - The <tr> element
     * @param {number}      newIdx - The new row index
     */
    _reindexRowElements(row, newIdx) {
        const idMap = {
            dmField: 'dmField_',
            dmStrategy: 'dmStrategy_',
            dmMaskCharBlock: 'dmMaskCharBlock_',
            dmMaskChar: 'dmMaskChar_',
            dmKeepFirstBlock: 'dmKeepFirstBlock_',
            dmKeepFirst: 'dmKeepFirst_',
            dmKeepLastBlock: 'dmKeepLastBlock_',
            dmKeepLast: 'dmKeepLast_',
            dmHashSaltBlock: 'dmHashSaltBlock_',
            dmHashSalt: 'dmHashSalt_',
            dmSubTypeBlock: 'dmSubTypeBlock_',
            dmSubType: 'dmSubType_',
            dmSubValueBlock: 'dmSubValueBlock_',
            dmSubValue: 'dmSubValue_',
            dmNoOptions: 'dmNoOptions_',
        };
        // Only update elements that follow the indexed naming convention
        for (const el of row.querySelectorAll('[id]')) {
            for (const [key, prefix] of Object.entries(idMap)) {
                if (el.id.startsWith(prefix)) {
                    el.id = prefix + newIdx;
                    break;
                }
            }
        }
        // Update remove button data attribute
        const removeBtn = row.querySelector('[data-remove-idx]');
        if (removeBtn) removeBtn.dataset.removeIdx = newIdx;
    }

    /**
     * Shows/hides conditional config fields based on the selected strategy.
     * Reads MASKING_STRATEGIES to determine which fields are relevant.
     * @private
     * @param {number} idx      - Row index
     * @param {string} strategy - Newly selected strategy key
     * @returns {void}
     */
    _onStrategyChange(idx, strategy) {
        const meta = DataMaskingBuilder.MASKING_STRATEGIES.get(strategy);
        if (!meta) return;

        const blocks = {
            maskChar:       document.getElementById(`dmMaskCharBlock_${idx}`),
            keepFirst:      document.getElementById(`dmKeepFirstBlock_${idx}`),
            keepLast:       document.getElementById(`dmKeepLastBlock_${idx}`),
            hashSalt:       document.getElementById(`dmHashSaltBlock_${idx}`),
            substituteType: document.getElementById(`dmSubTypeBlock_${idx}`),
        };
        const noOptions = document.getElementById(`dmNoOptions_${idx}`);

        let anyVisible = false;
        for (const [field, el] of Object.entries(blocks)) {
            if (!el) continue;
            const show = meta.fields.includes(field);
            el.classList.toggle('dm-hidden', !show);
            if (show) anyVisible = true;
        }

        // SubstituteValue block: visible only when strategy=substitute AND type=custom
        const subValueBlock = document.getElementById(`dmSubValueBlock_${idx}`);
        if (subValueBlock) {
            if (strategy === 'substitute') {
                const subType = document.getElementById(`dmSubType_${idx}`)?.value || 'name';
                subValueBlock.classList.toggle('dm-hidden', subType !== 'custom');
            } else {
                subValueBlock.classList.add('dm-hidden');
            }
        }

        if (noOptions) noOptions.style.display = anyVisible ? 'none' : '';
    }

    /**
     * Shows/hides the custom substitute value input based on the selected substitute type.
     * @private
     * @param {number} idx     - Row index
     * @param {string} subType - Selected substitute type ('name' | 'ssn' | 'phone' | 'email' | 'date' | 'address' | 'custom')
     * @returns {void}
     */
    _onSubstituteTypeChange(idx, subType) {
        const valueBlock = document.getElementById(`dmSubValueBlock_${idx}`);
        if (valueBlock) valueBlock.classList.toggle('dm-hidden', subType !== 'custom');
    }

    /**
     * Shows/hides the PHI field preview chips below the Mask All PHI toggle.
     * @private
     * @param {boolean} enabled - Whether the toggle is checked
     * @returns {void}
     */
    _onPhiToggle(enabled) {
        const expanded = document.getElementById('dmPhiChips');
        if (expanded) expanded.style.display = enabled ? '' : 'none';
    }

    /**
     * Switches the active format tab and re-renders the PHI field chips.
     * @private
     * @param {string} format - 'hl7v2' | 'fhir' | 'json'
     */
    _onPhiFormatChange(format) {
        document.querySelectorAll('#dmFormatTabs .dm-format-tab').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.format === format);
        });
        const fieldList = document.getElementById('dmPhiFieldList');
        if (fieldList) fieldList.innerHTML = this._renderPhiPreview(format);
    }

    /**
     * Returns the field-path input placeholder text appropriate for the given format.
     * @private
     * @param {string} format - 'hl7v2' | 'fhir' | 'json'
     * @returns {string}
     */
    _getFieldPlaceholder(format) {
        switch (format) {
            case 'fhir': return 'e.g. Patient.name[0].family';
            case 'json': return 'e.g. patient.name or patient.ssn';
            default:     return 'e.g. PID.5 or PID.19';
        }
    }

    /**
     * Builds the placement guidance hint HTML for the top format banner.
     * Explains where this step should be placed in the pipeline for each format.
     * @private
     * @param {string} format - 'hl7v2' | 'fhir' | 'json'
     * @returns {string} HTML string
     */
    _buildPlacementHint(format) {
        const hints = {
            hl7v2: `<i class="fas fa-info-circle"></i>
                    <span>Place this step <b>before the outbound connector</b> for HL7 v2 output.
                    Use paths like <code>PID.5</code>, <code>PID.19</code>, <code>MSH.4</code>.</span>`,
            fhir:  `<i class="fas fa-info-circle"></i>
                    <span>For FHIR output, place this step <b>after the FHIR Transform step</b>.
                    Use paths like <code>Patient.name[0].family</code>, <code>Patient.identifier[0].value</code>.</span>`,
            json:  `<i class="fas fa-info-circle"></i>
                    <span>Place this step <b>after your JSON transform</b> and before the outbound connector.
                    Use dot-notation paths like <code>patient.name</code>, <code>patient.ssn</code>.</span>`,
        };
        return hints[format] || hints.hl7v2;
    }

    // ─────────────────────────────────────────────────────────────────────────
    // Private Helpers
    // ─────────────────────────────────────────────────────────────────────────

    /**
     * Reads all visible rule rows from the DOM and returns them as a maskingRule array.
     * Skips the empty-state row (no data-rule-idx attribute).
     * @private
     * @returns {Array<Object>} Array of maskingRule objects
     */
    _readRulesFromDOM() {
        const tbody = document.getElementById('dmRulesTbody');
        if (!tbody) return [];

        const rules = [];
        const rows = tbody.querySelectorAll('tr[data-rule-idx]');

        rows.forEach((row) => {
            const idx = parseInt(row.dataset.ruleIdx, 10);
            const strategy = document.getElementById(`dmStrategy_${idx}`)?.value || 'mask';
            const meta = DataMaskingBuilder.MASKING_STRATEGIES.get(strategy) || { fields: [] };

            const rule = {
                field:    (document.getElementById(`dmField_${idx}`)?.value || '').trim(),
                strategy,
            };

            // Only include optional fields relevant to the selected strategy
            if (meta.fields.includes('maskChar')) {
                const mc = document.getElementById(`dmMaskChar_${idx}`)?.value || '*';
                if (mc !== '*') rule.maskChar = mc; // omit default
            }
            if (meta.fields.includes('keepFirst')) {
                rule.keepFirst = parseInt(document.getElementById(`dmKeepFirst_${idx}`)?.value || '0', 10) || 0;
            }
            if (meta.fields.includes('keepLast')) {
                rule.keepLast = parseInt(document.getElementById(`dmKeepLast_${idx}`)?.value || '4', 10) || 0;
            }
            if (meta.fields.includes('hashSalt')) {
                const salt = document.getElementById(`dmHashSalt_${idx}`)?.value || '';
                if (salt) rule.hashSalt = salt;
            }
            if (meta.fields.includes('substituteType')) {
                const subType = document.getElementById(`dmSubType_${idx}`)?.value || 'name';
                rule.substituteType = subType;
                if (subType === 'custom') {
                    rule.substituteValue = document.getElementById(`dmSubValue_${idx}`)?.value || '';
                }
            }

            if (rule.field) rules.push(rule); // skip rows with empty field path
        });

        return rules;
    }

    /**
     * Escapes a string for safe insertion into HTML attribute values and text content.
     * Defined as an instance method (not a module-level function) to keep the builder
     * fully self-contained per the project convention.
     * @private
     * @param {*} s - Value to escape
     * @returns {string}
     */
    _esc(s) {
        return String(s ?? '')
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#039;');
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Static class data — Open/Closed Principle
//
// Assigned after the class body so the file remains valid ES6 and loads in
// browsers that do not support the ES2022 static-class-field syntax
// (e.g. Firefox < 90, Safari < 14, Chrome < 74).
// All methods reference these via DataMaskingBuilder.MASKING_STRATEGIES /
// DataMaskingBuilder.PHI_FIELDS — the accessor pattern is unchanged.
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Strategy registry — add a new strategy by adding one entry here.
 * No render logic changes required (Open/Closed Principle).
 *
 * fields: which optional config inputs are shown when this strategy is selected.
 *   'maskChar'  → single-character replacement field
 *   'keepFirst' → number of chars to preserve at start
 *   'keepLast'  → number of chars to preserve at end
 *   'hashSalt'  → salt string for SHA-256 hashing
 *
 * @type {Map<string, {label: string, description: string, fields: string[]}>}
 */
DataMaskingBuilder.MASKING_STRATEGIES = new Map([
    ['mask',       { label: 'Mask',       description: 'Replace entire value with mask char (e.g. ****)',                              fields: ['maskChar'] }],
    ['redact',     { label: 'Redact',     description: 'Replace with [REDACTED]',                                                      fields: [] }],
    ['partial',    { label: 'Partial',    description: 'Reveal first N / last N chars, mask the middle',                               fields: ['keepFirst', 'keepLast', 'maskChar'] }],
    ['hash',       { label: 'Hash',       description: 'SHA-256 deterministic hash — 16 hex chars',                                    fields: ['hashSalt'] }],
    ['tokenize',   { label: 'Tokenize',   description: 'Non-reversible token with TOK- prefix',                                        fields: [] }],
    ['substitute', { label: 'Substitute', description: 'Replace with realistic-looking test data (deterministic, safe for dev/test)',   fields: ['substituteType'] }],
]);

/**
 * Pre-configured HIPAA PHI fields across three data formats: HL7 v2, FHIR R4, and Generic JSON.
 * Covers 13 of the 18 HIPAA Safe Harbor identifiers for each format.
 * (#12 Vehicle, #14 Web URLs, #15 IP, #16 Photos, #17 Biometrics are not applicable to
 * standard healthcare message formats.)
 *
 * Each entry has:
 *   hipaaId       — HIPAA Safe Harbor identifier number (1–18)
 *   hipaaCategory — Human-readable category (used for grouping in the preview)
 *   format        — 'hl7v2' | 'fhir' | 'json'
 *   field         — Field path appropriate for the format
 *   label         — Short display label
 *   strategy      — Recommended masking strategy
 *   description   — Tooltip / hover text
 *
 * @type {Array<{hipaaId:number,hipaaCategory:string,format:string,field:string,label:string,strategy:string,description:string}>}
 */
DataMaskingBuilder.PHI_FIELDS = [

    // ════════════════════════════════════════════════
    //  HIPAA #1 — Names
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:1, hipaaCategory:'Names', format:'hl7v2', field:'PID.5',    label:'Patient Name',          strategy:'mask',    description:'HIPAA #1 — patient name (PID-5)' },
    { hipaaId:1, hipaaCategory:'Names', format:'hl7v2', field:'PID.6',    label:"Mother's Maiden Name",  strategy:'mask',    description:"HIPAA #1 — mother's maiden name (PID-6)" },
    { hipaaId:1, hipaaCategory:'Names', format:'hl7v2', field:'NK1.2',    label:'Next of Kin Name',      strategy:'mask',    description:'HIPAA #1 — next of kin name (NK1-2)' },
    { hipaaId:1, hipaaCategory:'Names', format:'hl7v2', field:'GT1.3',    label:'Guarantor Name',        strategy:'mask',    description:'HIPAA #1 — guarantor name (GT1-3)' },
    // ── FHIR R4 ──
    { hipaaId:1, hipaaCategory:'Names', format:'fhir',  field:'Patient.name[0].family',          label:'Family Name',       strategy:'mask', description:'HIPAA #1 — family name (FHIR Patient.name[0].family)' },
    { hipaaId:1, hipaaCategory:'Names', format:'fhir',  field:'Patient.name[0].given',           label:'Given Name',        strategy:'mask', description:'HIPAA #1 — given name (FHIR Patient.name[0].given)' },
    { hipaaId:1, hipaaCategory:'Names', format:'fhir',  field:'RelatedPerson.name[0].family',    label:'Related Person Name', strategy:'mask', description:'HIPAA #1 — related person name (FHIR RelatedPerson.name[0].family)' },
    // ── Generic JSON ──
    { hipaaId:1, hipaaCategory:'Names', format:'json',  field:'patient.name',       label:'Patient Name',  strategy:'mask', description:'HIPAA #1 — patient.name in generic JSON' },
    { hipaaId:1, hipaaCategory:'Names', format:'json',  field:'patient.firstName',  label:'First Name',    strategy:'mask', description:'HIPAA #1 — patient.firstName in generic JSON' },
    { hipaaId:1, hipaaCategory:'Names', format:'json',  field:'patient.lastName',   label:'Last Name',     strategy:'mask', description:'HIPAA #1 — patient.lastName in generic JSON' },

    // ════════════════════════════════════════════════
    //  HIPAA #2 — Geographic Data
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:2, hipaaCategory:'Geographic', format:'hl7v2', field:'PID.11.1', label:'Street Address', strategy:'mask',    description:'HIPAA #2 — street address (PID-11.1)' },
    { hipaaId:2, hipaaCategory:'Geographic', format:'hl7v2', field:'PID.11.3', label:'City',           strategy:'mask',    description:'HIPAA #2 — city (PID-11.3)' },
    { hipaaId:2, hipaaCategory:'Geographic', format:'hl7v2', field:'PID.11.5', label:'Zip Code',       strategy:'partial', description:'HIPAA #2 — zip: keep first 3 digits (§164.514(b) safe harbor)' },
    // ── FHIR R4 ──
    { hipaaId:2, hipaaCategory:'Geographic', format:'fhir',  field:'Patient.address[0].line[0]',    label:'Street Address', strategy:'mask',    description:'HIPAA #2 — street address (FHIR Patient.address[0].line[0])' },
    { hipaaId:2, hipaaCategory:'Geographic', format:'fhir',  field:'Patient.address[0].city',       label:'City',           strategy:'mask',    description:'HIPAA #2 — city (FHIR Patient.address[0].city)' },
    { hipaaId:2, hipaaCategory:'Geographic', format:'fhir',  field:'Patient.address[0].postalCode', label:'Postal Code',    strategy:'partial', description:'HIPAA #2 — postal code: keep first 3 (FHIR Patient.address[0].postalCode)' },
    // ── Generic JSON ──
    { hipaaId:2, hipaaCategory:'Geographic', format:'json',  field:'patient.streetAddress', label:'Street Address', strategy:'mask',    description:'HIPAA #2 — patient.streetAddress' },
    { hipaaId:2, hipaaCategory:'Geographic', format:'json',  field:'patient.city',          label:'City',           strategy:'mask',    description:'HIPAA #2 — patient.city' },
    { hipaaId:2, hipaaCategory:'Geographic', format:'json',  field:'patient.zipCode',       label:'Zip Code',       strategy:'partial', description:'HIPAA #2 — patient.zipCode: keep first 3 digits' },

    // ════════════════════════════════════════════════
    //  HIPAA #3 — Dates (except year)
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:3, hipaaCategory:'Dates', format:'hl7v2', field:'PID.7',   label:'Date of Birth',   strategy:'partial', description:'HIPAA #3 — DOB: keep year only (keepFirst:4) (PID-7)' },
    { hipaaId:3, hipaaCategory:'Dates', format:'hl7v2', field:'PID.29',  label:'Death Date',      strategy:'redact',  description:'HIPAA #3 — date of death: fully redacted (PID-29)' },
    { hipaaId:3, hipaaCategory:'Dates', format:'hl7v2', field:'PV1.44',  label:'Admit Date',      strategy:'partial', description:'HIPAA #3 — admit date: keep year only (PV1-44)' },
    { hipaaId:3, hipaaCategory:'Dates', format:'hl7v2', field:'PV1.45',  label:'Discharge Date',  strategy:'partial', description:'HIPAA #3 — discharge date: keep year only (PV1-45)' },
    // ── FHIR R4 ──
    { hipaaId:3, hipaaCategory:'Dates', format:'fhir',  field:'Patient.birthDate',         label:'Date of Birth',  strategy:'partial', description:'HIPAA #3 — birthDate: keep year only (FHIR Patient.birthDate)' },
    { hipaaId:3, hipaaCategory:'Dates', format:'fhir',  field:'Patient.deceasedDateTime',  label:'Death Date',     strategy:'redact',  description:'HIPAA #3 — death date: fully redacted (FHIR Patient.deceasedDateTime)' },
    { hipaaId:3, hipaaCategory:'Dates', format:'fhir',  field:'Encounter.period.start',    label:'Admit Date',     strategy:'partial', description:'HIPAA #3 — encounter start: keep year only (FHIR Encounter.period.start)' },
    { hipaaId:3, hipaaCategory:'Dates', format:'fhir',  field:'Encounter.period.end',      label:'Discharge Date', strategy:'partial', description:'HIPAA #3 — encounter end: keep year only (FHIR Encounter.period.end)' },
    // ── Generic JSON ──
    { hipaaId:3, hipaaCategory:'Dates', format:'json',  field:'patient.dateOfBirth', label:'Date of Birth',  strategy:'partial', description:'HIPAA #3 — patient.dateOfBirth: keep year only' },
    { hipaaId:3, hipaaCategory:'Dates', format:'json',  field:'patient.dob',         label:'DOB (short key)', strategy:'partial', description:'HIPAA #3 — patient.dob: keep year only' },

    // ════════════════════════════════════════════════
    //  HIPAA #4 & #5 — Telephone & Fax Numbers
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:4, hipaaCategory:'Phone / Fax', format:'hl7v2', field:'PID.13', label:'Home Phone / Fax', strategy:'partial', description:'HIPAA #4 & #5 — home telecom (PID-13): keep last 4 digits' },
    { hipaaId:4, hipaaCategory:'Phone / Fax', format:'hl7v2', field:'PID.14', label:'Work Phone / Fax', strategy:'partial', description:'HIPAA #4 & #5 — work telecom (PID-14): keep last 4 digits' },
    // ── FHIR R4 ──
    { hipaaId:4, hipaaCategory:'Phone / Fax', format:'fhir', field:'Patient.telecom[0].value', label:'Telecom [0]', strategy:'partial', description:'HIPAA #4/#5 — primary telecom (FHIR Patient.telecom[0].value): keep last 4' },
    { hipaaId:4, hipaaCategory:'Phone / Fax', format:'fhir', field:'Patient.telecom[1].value', label:'Telecom [1]', strategy:'partial', description:'HIPAA #4/#5 — secondary telecom (FHIR Patient.telecom[1].value): keep last 4' },
    // ── Generic JSON ──
    { hipaaId:4, hipaaCategory:'Phone / Fax', format:'json', field:'patient.phone',       label:'Phone',     strategy:'partial', description:'HIPAA #4 — patient.phone: keep last 4' },
    { hipaaId:4, hipaaCategory:'Phone / Fax', format:'json', field:'patient.phoneNumber', label:'Phone No.', strategy:'partial', description:'HIPAA #4 — patient.phoneNumber: keep last 4' },
    { hipaaId:4, hipaaCategory:'Phone / Fax', format:'json', field:'patient.fax',         label:'Fax',       strategy:'partial', description:'HIPAA #5 — patient.fax: keep last 4' },

    // ════════════════════════════════════════════════
    //  HIPAA #6 — Electronic Mail Addresses
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:6, hipaaCategory:'Email', format:'hl7v2', field:'PID.13.4', label:'Email (XTN.4)',  strategy:'mask', description:'HIPAA #6 — email in XTN component 4 of PID-13' },
    // ── FHIR R4 ──
    { hipaaId:6, hipaaCategory:'Email', format:'fhir',  field:'Patient.telecom[2].value', label:'Email Telecom', strategy:'mask', description:'HIPAA #6 — email as telecom entry (FHIR Patient.telecom[2].value)' },
    // ── Generic JSON ──
    { hipaaId:6, hipaaCategory:'Email', format:'json',  field:'patient.email', label:'Email', strategy:'mask', description:'HIPAA #6 — patient.email' },

    // ════════════════════════════════════════════════
    //  HIPAA #7 — Social Security Numbers
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:7, hipaaCategory:'SSN', format:'hl7v2', field:'PID.19', label:'SSN',          strategy:'hash', description:'HIPAA #7 — SSN (PID-19): SHA-256 hash' },
    // ── FHIR R4 ──
    { hipaaId:7, hipaaCategory:'SSN', format:'fhir',  field:'Patient.identifier[0].value', label:'Identifier [0]', strategy:'hash', description:'HIPAA #7 — primary identifier / SSN (FHIR Patient.identifier[0].value): SHA-256 hash' },
    // ── Generic JSON ──
    { hipaaId:7, hipaaCategory:'SSN', format:'json',  field:'patient.ssn',                  label:'SSN',          strategy:'hash', description:'HIPAA #7 — patient.ssn: SHA-256 hash' },
    { hipaaId:7, hipaaCategory:'SSN', format:'json',  field:'patient.socialSecurityNumber', label:'SSN (full key)', strategy:'hash', description:'HIPAA #7 — patient.socialSecurityNumber: SHA-256 hash' },

    // ════════════════════════════════════════════════
    //  HIPAA #8 — Medical Record Numbers
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:8, hipaaCategory:'MRN', format:'hl7v2', field:'PID.3', label:'Medical Record No.', strategy:'partial', description:'HIPAA #8 — MRN (PID-3 CX.1): keep last 4' },
    // ── FHIR R4 ──
    { hipaaId:8, hipaaCategory:'MRN', format:'fhir',  field:'Patient.identifier[1].value', label:'Identifier [1]', strategy:'partial', description:'HIPAA #8 — MRN (FHIR Patient.identifier[1].value): keep last 4' },
    // ── Generic JSON ──
    { hipaaId:8, hipaaCategory:'MRN', format:'json',  field:'patient.mrn',                  label:'MRN',          strategy:'partial', description:'HIPAA #8 — patient.mrn: keep last 4' },
    { hipaaId:8, hipaaCategory:'MRN', format:'json',  field:'patient.medicalRecordNumber',  label:'MRN (full key)', strategy:'partial', description:'HIPAA #8 — patient.medicalRecordNumber: keep last 4' },

    // ════════════════════════════════════════════════
    //  HIPAA #9 — Health Plan Beneficiary Numbers
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:9, hipaaCategory:'Insurance', format:'hl7v2', field:'IN1.49', label:'Member ID',         strategy:'partial', description:'HIPAA #9 — member ID (IN1-49): keep last 4' },
    { hipaaId:9, hipaaCategory:'Insurance', format:'hl7v2', field:'IN1.2',  label:'Insurance Plan ID', strategy:'partial', description:'HIPAA #9 — insurance plan ID (IN1-2): keep last 4' },
    // ── FHIR R4 ──
    { hipaaId:9, hipaaCategory:'Insurance', format:'fhir',  field:'Coverage.subscriberId',           label:'Subscriber ID', strategy:'partial', description:'HIPAA #9 — subscriber ID (FHIR Coverage.subscriberId): keep last 4' },
    { hipaaId:9, hipaaCategory:'Insurance', format:'fhir',  field:'Coverage.identifier[0].value',    label:'Coverage ID',   strategy:'partial', description:'HIPAA #9 — coverage identifier (FHIR Coverage.identifier[0].value): keep last 4' },
    // ── Generic JSON ──
    { hipaaId:9, hipaaCategory:'Insurance', format:'json',  field:'patient.insuranceId',  label:'Insurance ID', strategy:'partial', description:'HIPAA #9 — patient.insuranceId: keep last 4' },
    { hipaaId:9, hipaaCategory:'Insurance', format:'json',  field:'patient.memberId',     label:'Member ID',    strategy:'partial', description:'HIPAA #9 — patient.memberId: keep last 4' },

    // ════════════════════════════════════════════════
    //  HIPAA #10 — Account Numbers
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:10, hipaaCategory:'Account', format:'hl7v2', field:'PID.18', label:'Patient Account No.', strategy:'partial', description:'HIPAA #10 — patient account (PID-18 CX.1): keep last 4' },
    // ── FHIR R4 ──
    { hipaaId:10, hipaaCategory:'Account', format:'fhir',  field:'Account.identifier[0].value', label:'Account No.', strategy:'partial', description:'HIPAA #10 — account number (FHIR Account.identifier[0].value): keep last 4' },
    // ── Generic JSON ──
    { hipaaId:10, hipaaCategory:'Account', format:'json',  field:'patient.accountNumber', label:'Account No.', strategy:'partial', description:'HIPAA #10 — patient.accountNumber: keep last 4' },

    // ════════════════════════════════════════════════
    //  HIPAA #11 — Certificate / License Numbers
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:11, hipaaCategory:'License', format:'hl7v2', field:'PID.20', label:"Driver's License",   strategy:'hash', description:"HIPAA #11 — driver's license / certificate (PID-20): SHA-256 hash" },
    // ── FHIR R4 ──
    { hipaaId:11, hipaaCategory:'License', format:'fhir',  field:'Practitioner.identifier[0].value', label:'License No.', strategy:'hash', description:'HIPAA #11 — practitioner license (FHIR Practitioner.identifier[0].value): SHA-256 hash' },
    // ── Generic JSON ──
    { hipaaId:11, hipaaCategory:'License', format:'json',  field:'patient.driversLicense', label:"Driver's License", strategy:'hash', description:"HIPAA #11 — patient.driversLicense: SHA-256 hash" },

    // ════════════════════════════════════════════════
    //  HIPAA #13 — Device Identifiers and Serial Numbers
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:13, hipaaCategory:'Device ID', format:'hl7v2', field:'OBX.18', label:'Equipment Instance', strategy:'hash', description:'HIPAA #13 — device/equipment ID in OBX-18: SHA-256 hash' },
    // ── FHIR R4 ──
    { hipaaId:13, hipaaCategory:'Device ID', format:'fhir',  field:'Device.identifier[0].value', label:'Device ID', strategy:'hash', description:'HIPAA #13 — device identifier (FHIR Device.identifier[0].value): SHA-256 hash' },
    // ── Generic JSON ──
    { hipaaId:13, hipaaCategory:'Device ID', format:'json',  field:'device.serialNumber', label:'Device Serial No.', strategy:'hash', description:'HIPAA #13 — device.serialNumber: SHA-256 hash' },

    // ════════════════════════════════════════════════
    //  HIPAA #18 — Any Other Unique Identifying Number
    // ════════════════════════════════════════════════
    // ── HL7 v2 ──
    { hipaaId:18, hipaaCategory:'Other IDs', format:'hl7v2', field:'PID.2', label:'Patient Alias ID',     strategy:'hash', description:'HIPAA #18 — patient alias ID (PID-2): SHA-256 hash' },
    { hipaaId:18, hipaaCategory:'Other IDs', format:'hl7v2', field:'PID.4', label:'Alternate Patient ID', strategy:'hash', description:'HIPAA #18 — alternate patient ID (PID-4): SHA-256 hash' },
    // ── FHIR R4 ──
    { hipaaId:18, hipaaCategory:'Other IDs', format:'fhir',  field:'Patient.identifier[2].value', label:'Identifier [2]', strategy:'hash', description:'HIPAA #18 — tertiary identifier (FHIR Patient.identifier[2].value): SHA-256 hash' },
    // ── Generic JSON ──
    { hipaaId:18, hipaaCategory:'Other IDs', format:'json',  field:'patient.alternateId', label:'Alternate ID', strategy:'hash', description:'HIPAA #18 — patient.alternateId: SHA-256 hash' },
];

// ─────────────────────────────────────────────────────────────────────────────
// Registration — Dependency Inversion Principle
// PropertiesPanel never imports DataMaskingBuilder directly; it discovers it
// through the StepBuilderRegistry abstraction.
// ─────────────────────────────────────────────────────────────────────────────
StepBuilderRegistry.register('data_masking', DataMaskingBuilder);
