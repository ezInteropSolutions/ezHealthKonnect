/**
 * PayloadBuilderBuilder — step UI for the Payload Builder executor.
 * Constructs the final outbound wire payload from pipeline data.
 *
 * Four modes exposed as tabs:
 *   Simple       (pass_through)  — source picker + format selector
 *   Template     (template)      — {{ variable }} substitution in a template string
 *   Field Builder (field_builder) — add/remove field mapping rows
 *   FHIR Bundle  (fhir_bundle)   — assemble a FHIR Bundle from pipeline resources
 *
 * Registered with StepBuilderRegistry as 'payload.builder' and 'payload_builder'.
 */
class PayloadBuilderBuilder {
    constructor(propertiesPanel) {
        this.panel = propertiesPanel;
    }

    // ── Public builder contract ──────────────────────────────────────────────

    render(step) {
        if (!step.config) step.config = {};
        const html = this._buildHTML(step);
        setTimeout(() => this._attachEvents(step), 0);
        return html;
    }

    collectConfig(step) {
        const container = document.querySelector('#formTabContent') ||
                          document.querySelector('.properties-panel-content') ||
                          document.body;

        const mode = container.querySelector('#pb-mode')?.value || 'pass_through';
        step.config = step.config || {};
        step.config.mode = mode;

        if (mode === 'pass_through') {
            const sourceSelect = container.querySelector('#pb-source-select');
            const customInput  = container.querySelector('#pb-source-custom');
            let sourcePath = '';
            if (sourceSelect) {
                sourcePath = sourceSelect.value === '_custom_' ?
                    (customInput?.value || '') : sourceSelect.value;
            }
            step.config.source_path   = sourcePath;
            step.config.output_format = container.querySelector('#pb-format-select')?.value || 'fhir_r4';
        }

        if (mode === 'template') {
            step.config.template      = container.querySelector('#pb-template')?.value || '';
            step.config.output_format = container.querySelector('#pb-tpl-format')?.value || 'fhir_r4';
        }

        if (mode === 'field_builder') {
            step.config.output_format = container.querySelector('#pb-fb-format')?.value || 'json';
            const rows = container.querySelectorAll('.pb-mapping-row');
            const mappings = [];
            rows.forEach(row => {
                const target = row.querySelector('.pb-map-target')?.value?.trim();
                const type   = row.querySelector('.pb-map-type')?.value || 'field_ref';
                const val    = row.querySelector('.pb-map-value')?.value?.trim();
                if (target && val) {
                    mappings.push({
                        target,
                        source_type: type,
                        value:  type === 'literal'   ? val : '',
                        source: type === 'field_ref' ? val : '',
                    });
                }
            });
            step.config.field_mappings = mappings;
        }

        if (mode === 'fhir_bundle') {
            const fb = step.config.fhirBundle || {};
            fb.bundleType     = container.querySelector('#pb-fb-bundle-type')?.value || 'collection';
            fb.bundleId       = container.querySelector('#pb-fb-bundle-id')?.value?.trim() || '';
            fb.includeRequest = container.querySelector('#pb-fb-include-request')?.checked ?? false;

            const pathRows = container.querySelectorAll('.pb-rp-row');
            const paths = [];
            pathRows.forEach(row => {
                const v = row.querySelector('.pb-rp-input')?.value?.trim();
                if (v) paths.push(v);
            });
            fb.resourcePaths = paths;
            step.config.fhirBundle = fb;
        }

        console.log('[PayloadBuilderBuilder] ✅ config collected:', step.config);
    }

    destroy() {
        // No AC components to clean up in v1
    }

    // ── Private: HTML construction ───────────────────────────────────────────

    _buildHTML(step) {
        const cfg = step.config || {};
        const mode = cfg.mode || 'pass_through';
        const esc = s => (s || '').toString()
            .replace(/&/g, '&amp;').replace(/</g, '&lt;')
            .replace(/>/g, '&gt;').replace(/"/g, '&quot;');

        const tabBtn = (id, label, active) =>
            `<button class="pb-tab-btn${active ? ' pb-tab-active' : ''}" data-mode="${id}"
                style="flex:1;padding:8px 4px;border:none;border-bottom:${active ? '2px solid #1a73e8' : '2px solid transparent'};
                background:transparent;color:${active ? '#1a73e8' : '#555'};font-size:12px;cursor:pointer;
                font-weight:${active ? '600' : '400'};transition:all 0.15s;">${label}</button>`;

        const section = document.createElement('div');
        section.className = 'form-section';
        section.innerHTML = `
<!-- Payload Builder -->
<div class="payload-builder-wrap" style="padding:0">

  <div class="pb-tabs" style="display:flex;border-bottom:1px solid #e0e0e0;margin-bottom:14px;">
    ${tabBtn('pass_through', 'Simple',        mode === 'pass_through')}
    ${tabBtn('template',     'Template',      mode === 'template')}
    ${tabBtn('field_builder','Field Builder', mode === 'field_builder')}
    ${tabBtn('fhir_bundle',  'FHIR Bundle',  mode === 'fhir_bundle')}
  </div>

  <input type="hidden" id="pb-mode" value="${esc(mode)}">

  <!-- ── SIMPLE TAB ── -->
  <div id="pb-tab-pass_through" class="pb-tab-pane" style="display:${mode === 'pass_through' ? 'block' : 'none'}">
    <div style="background:#f0f7ff;border:1px solid #cce0ff;border-radius:6px;padding:10px;margin-bottom:12px;font-size:12px;color:#1a5276;">
      Sends an existing pipeline variable directly to the outbound connector. Best for standard HL7&rarr;FHIR flows.
    </div>
    <div class="form-group">
      <label class="form-label">Source</label>
      <select id="pb-source-select" class="form-control" style="margin-bottom:4px;">
        <option value="">-- auto-detect --</option>
        <option value="fhirBundle"  ${(cfg.source_path || '') === 'fhirBundle'  ? 'selected' : ''}>fhirBundle &mdash; HL7&rarr;FHIR transform output</option>
        <option value="fhir_bundle" ${(cfg.source_path || '') === 'fhir_bundle' ? 'selected' : ''}>fhir_bundle &mdash; normalized key</option>
        <option value="message"     ${(cfg.source_path || '') === 'message'     ? 'selected' : ''}>message &mdash; full message envelope</option>
        <option value="_custom_"    ${cfg.source_path && !['','fhirBundle','fhir_bundle','message'].includes(cfg.source_path) ? 'selected' : ''}>Custom path&hellip;</option>
      </select>
      <input type="text" id="pb-source-custom" class="form-control form-control-sm"
        placeholder="e.g. steps.hl7_fhir_transform.step_output.fhir_bundle"
        value="${esc(cfg.source_path || '')}"
        style="display:${cfg.source_path && !['','fhirBundle','fhir_bundle','message'].includes(cfg.source_path) ? 'block' : 'none'};margin-top:4px;">
      <div class="field-help">Leave blank to auto-detect (fhirBundle &rarr; fhir_bundle &rarr; message).</div>
    </div>
    <div class="form-group">
      <label class="form-label">Output Format</label>
      <select id="pb-format-select" class="form-control">
        <option value="fhir_r4" ${(cfg.output_format || 'fhir_r4') === 'fhir_r4' ? 'selected' : ''}>FHIR R4 &mdash; application/fhir+json</option>
        <option value="hl7v2"   ${cfg.output_format === 'hl7v2'   ? 'selected' : ''}>HL7 v2.x &mdash; application/hl7-v2</option>
        <option value="json"    ${cfg.output_format === 'json'    ? 'selected' : ''}>JSON &mdash; application/json</option>
        <option value="csv"     ${cfg.output_format === 'csv'     ? 'selected' : ''}>CSV &mdash; text/csv</option>
      </select>
    </div>
  </div>

  <!-- ── TEMPLATE TAB ── -->
  <div id="pb-tab-template" class="pb-tab-pane" style="display:${mode === 'template' ? 'block' : 'none'}">
    <div style="background:#fff8e1;border:1px solid #ffe082;border-radius:6px;padding:10px;margin-bottom:12px;font-size:12px;color:#6d4c00;">
      Use <code style="background:#fff3e0;padding:1px 4px;border-radius:3px;">{{ variable.path }}</code> to insert pipeline values.
      Example: <code style="background:#fff3e0;padding:1px 4px;border-radius:3px;">{{ PID.5.1 }}</code>,
      <code style="background:#fff3e0;padding:1px 4px;border-radius:3px;">{{ steps.enrich.step_output.mrn }}</code>
    </div>
    <div class="form-group">
      <label class="form-label">Output Format</label>
      <select id="pb-tpl-format" class="form-control" style="margin-bottom:8px;">
        <option value="fhir_r4" ${(cfg.output_format || 'fhir_r4') === 'fhir_r4' ? 'selected' : ''}>FHIR R4</option>
        <option value="hl7v2"   ${cfg.output_format === 'hl7v2'   ? 'selected' : ''}>HL7 v2.x</option>
        <option value="json"    ${cfg.output_format === 'json'    ? 'selected' : ''}>JSON</option>
      </select>
    </div>
    <div class="form-group">
      <label class="form-label">Template</label>
      <textarea id="pb-template" class="form-control" rows="12"
        style="font-family:monospace;font-size:11px;resize:vertical;"
        placeholder='{"resourceType":"Bundle","entry":[{"resource":{"resourceType":"Patient","id":"{{ steps.enrich.step_output.mrn }}"}}]}'>${esc(cfg.template || '')}</textarea>
      <div class="field-help">Write JSON, HL7, or any text. Placeholders are replaced at runtime.</div>
    </div>
    <div class="form-group">
      <label class="form-label" style="color:#555;">Available Variables</label>
      <div id="pb-tpl-vars" style="max-height:140px;overflow-y:auto;background:#f5f5f5;border:1px solid #ddd;
           border-radius:4px;padding:8px;font-size:11px;font-family:monospace;color:#444;white-space:pre-wrap;">
        Loading variables&hellip;
      </div>
    </div>
  </div>

  <!-- ── FIELD BUILDER TAB ── -->
  <div id="pb-tab-field_builder" class="pb-tab-pane" style="display:${mode === 'field_builder' ? 'block' : 'none'}">
    <div style="background:#f3e5f5;border:1px solid #e1bee7;border-radius:6px;padding:10px;margin-bottom:12px;font-size:12px;color:#4a148c;">
      Map specific fields to build a custom output structure. Each row defines one output field.
    </div>
    <div class="form-group">
      <label class="form-label">Output Format</label>
      <select id="pb-fb-format" class="form-control" style="margin-bottom:8px;">
        <option value="fhir_r4" ${(cfg.output_format || 'fhir_r4') === 'fhir_r4' ? 'selected' : ''}>FHIR R4</option>
        <option value="hl7v2"   ${cfg.output_format === 'hl7v2'   ? 'selected' : ''}>HL7 v2.x</option>
        <option value="json"    ${cfg.output_format === 'json'    ? 'selected' : ''}>JSON</option>
        <option value="csv"     ${cfg.output_format === 'csv'     ? 'selected' : ''}>CSV</option>
      </select>
    </div>
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:6px;">
      <span class="form-label" style="margin:0;">Field Mappings</span>
      <button id="pb-add-mapping" class="btn btn-sm"
        style="background:#1a73e8;color:#fff;padding:3px 10px;font-size:11px;border:none;border-radius:4px;cursor:pointer;">
        + Add Field
      </button>
    </div>
    <div style="font-size:11px;color:#888;display:grid;grid-template-columns:1fr 80px 1fr 24px;gap:4px;margin-bottom:4px;padding:0 2px;">
      <span>Target Path</span><span>Type</span><span>Value / Source</span><span></span>
    </div>
    <div id="pb-mappings-list">
      ${this._renderMappingRows(cfg.field_mappings || [])}
    </div>
  </div>

  <!-- ── FHIR BUNDLE TAB ── -->
  <div id="pb-tab-fhir_bundle" class="pb-tab-pane" style="display:${mode === 'fhir_bundle' ? 'block' : 'none'}">
    <div style="background:#e8f5e9;border:1px solid #a5d6a7;border-radius:6px;padding:10px;margin-bottom:12px;font-size:12px;color:#1b5e20;">
      Assemble a FHIR R4 Bundle from pipeline variables. Resources are resolved at runtime
      from the paths you specify below (e.g. <code style="background:#f1f8e9;padding:1px 4px;border-radius:3px;">steps.transform.step_output.fhirBundle</code>).
      The Bundle is written to the <code style="background:#f1f8e9;padding:1px 4px;border-radius:3px;">payload</code> variable.
    </div>

    <!-- Bundle Type -->
    <div class="form-group">
      <label class="form-label">Bundle Type</label>
      <select id="pb-fb-bundle-type" class="form-control">
        <option value="collection"  ${(cfg.fhirBundle?.bundleType || 'collection') === 'collection'  ? 'selected' : ''}>collection — read-only set of resources</option>
        <option value="transaction" ${cfg.fhirBundle?.bundleType === 'transaction' ? 'selected' : ''}>transaction — atomic FHIR transaction (entry.request auto-added)</option>
        <option value="batch"       ${cfg.fhirBundle?.bundleType === 'batch'       ? 'selected' : ''}>batch — independent operations (entry.request auto-added)</option>
        <option value="document"    ${cfg.fhirBundle?.bundleType === 'document'    ? 'selected' : ''}>document — clinical document with Composition as first entry</option>
      </select>
      <div class="field-help">Determines the FHIR Bundle.type and required entry structure.</div>
    </div>

    <!-- Resource Paths -->
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:6px;">
      <label class="form-label" style="margin:0;">Resource Paths</label>
      <button id="pb-add-resource-path" class="btn btn-sm"
        style="background:#2e7d32;color:#fff;padding:3px 10px;font-size:11px;border:none;border-radius:4px;cursor:pointer;">
        + Add Path
      </button>
    </div>
    <div class="field-help" style="margin-bottom:6px;">
      Each path is resolved from pipeline variables. May contain a single FHIR resource or an array of resources.
      Examples: <code>steps.transform.step_output.fhirBundle</code>, <code>fhirBundle</code>, <code>steps.enrich.step_output.patient</code>
    </div>
    <div id="pb-resource-paths-list">
      ${this._renderResourcePathRows(cfg.fhirBundle?.resourcePaths || [])}
    </div>

    <!-- Bundle ID (optional) -->
    <div class="form-group" style="margin-top:12px;">
      <label class="form-label">Bundle ID <span style="font-weight:400;color:#888;">(optional)</span></label>
      <input type="text" id="pb-fb-bundle-id" class="form-control"
        placeholder="Auto-generated UUID if blank"
        value="${esc(cfg.fhirBundle?.bundleId || '')}">
      <div class="field-help">If set, used as the Bundle.id. Leave blank for a random UUID.</div>
    </div>

    <!-- Include Request entries -->
    <div class="form-group">
      <label style="display:flex;align-items:center;gap:8px;cursor:pointer;font-size:13px;">
        <input type="checkbox" id="pb-fb-include-request"
          ${cfg.fhirBundle?.includeRequest ? 'checked' : ''}>
        <span>Include <code>entry.request</code> <span style="font-weight:400;color:#555;">(auto-enabled for transaction/batch)</span></span>
      </label>
      <div class="field-help" style="margin-left:22px;">
        Adds <code>{"method":"POST","url":"ResourceType"}</code> or <code>{"method":"PUT","url":"ResourceType/id"}</code>
        to each Bundle entry. Required for transaction/batch Bundles.
      </div>
    </div>

    <!-- Live preview hint -->
    <div style="background:#f5f5f5;border:1px solid #ddd;border-radius:4px;padding:10px;font-size:11px;color:#444;margin-top:4px;">
      <strong>Output variable:</strong> <code>payload</code>
      &nbsp;|&nbsp; <strong>Content-Type:</strong> <code>application/fhir+json</code>
      &nbsp;|&nbsp; Wire directly to an <em>HTTP FHIR Sender</em> outbound connector.
    </div>
  </div>

</div>`;

        return section.outerHTML;
    }

    _renderMappingRows(mappings) {
        if (!mappings || mappings.length === 0) {
            return this._renderMappingRow({ target: '', source_type: 'field_ref', source: '', value: '' }, 0);
        }
        return mappings.map((m, i) => this._renderMappingRow(m, i)).join('');
    }

    _renderMappingRow(m, i) {
        const esc = s => (s || '').toString()
            .replace(/&/g, '&amp;').replace(/</g, '&lt;')
            .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        const type = m.source_type || 'field_ref';
        const displayVal = type === 'literal' ? (m.value || '') : (m.source || '');
        return `
<div class="pb-mapping-row" data-index="${i}"
  style="display:grid;grid-template-columns:1fr 80px 1fr 24px;gap:4px;margin-bottom:4px;align-items:center;">
  <input type="text" class="form-control form-control-sm pb-map-target"
    placeholder="e.g. entry.resource.id" value="${esc(m.target || '')}">
  <select class="form-control form-control-sm pb-map-type">
    <option value="field_ref" ${type === 'field_ref' ? 'selected' : ''}>Field</option>
    <option value="literal"   ${type === 'literal'   ? 'selected' : ''}>Literal</option>
  </select>
  <input type="text" class="form-control form-control-sm pb-map-value"
    placeholder="${type === 'literal' ? 'Enter value' : 'e.g. PID.5.1 or steps.enrich.mrn'}"
    value="${esc(displayVal)}">
  <button class="pb-remove-row"
    style="background:none;border:none;color:#e53935;cursor:pointer;padding:0;font-size:16px;line-height:1;"
    title="Remove row">&times;</button>
</div>`;
    }

    _renderResourcePathRows(paths) {
        const esc = s => (s || '').toString()
            .replace(/&/g, '&amp;').replace(/</g, '&lt;')
            .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        if (!paths || paths.length === 0) {
            paths = [''];
        }
        return paths.map((p, i) => `
<div class="pb-rp-row" data-index="${i}"
  style="display:flex;gap:4px;margin-bottom:4px;align-items:center;">
  <input type="text" class="form-control form-control-sm pb-rp-input"
    placeholder="e.g. steps.transform.step_output.fhirBundle"
    value="${esc(p)}" style="flex:1;">
  <button class="pb-rp-remove"
    style="background:none;border:none;color:#e53935;cursor:pointer;padding:0 4px;font-size:16px;line-height:1;"
    title="Remove path">&times;</button>
</div>`).join('');
    }

    // ── Private: event wiring ────────────────────────────────────────────────

    _attachEvents(step) {
        const root = document.querySelector('#formTabContent') ||
                     document.querySelector('.properties-panel-content') ||
                     document.body;

        // Tab switching
        root.querySelectorAll('.pb-tab-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const mode = btn.dataset.mode;
                root.querySelector('#pb-mode').value = mode;
                root.querySelectorAll('.pb-tab-btn').forEach(b => {
                    const isActive = b.dataset.mode === mode;
                    b.style.color       = isActive ? '#1a73e8' : '#555';
                    b.style.fontWeight  = isActive ? '600' : '400';
                    b.style.borderBottom = isActive ? '2px solid #1a73e8' : '2px solid transparent';
                });
                root.querySelectorAll('.pb-tab-pane').forEach(p => {
                    p.style.display = p.id === `pb-tab-${mode}` ? 'block' : 'none';
                });
            });
        });

        // Source select → toggle custom input visibility
        const sourceSelect = root.querySelector('#pb-source-select');
        const customInput  = root.querySelector('#pb-source-custom');
        if (sourceSelect && customInput) {
            sourceSelect.addEventListener('change', () => {
                if (sourceSelect.value === '_custom_') {
                    customInput.style.display = 'block';
                    customInput.focus();
                } else {
                    customInput.style.display = 'none';
                    customInput.value = sourceSelect.value;
                }
            });
        }

        // Add mapping row button
        const addBtn = root.querySelector('#pb-add-mapping');
        const list   = root.querySelector('#pb-mappings-list');
        if (addBtn && list) {
            addBtn.addEventListener('click', () => {
                const count = list.querySelectorAll('.pb-mapping-row').length;
                const tmp = document.createElement('div');
                tmp.innerHTML = this._renderMappingRow(
                    { target: '', source_type: 'field_ref', source: '', value: '' }, count
                );
                const newRow = tmp.firstElementChild;
                list.appendChild(newRow);
                this._wireRowEvents(newRow);
            });
        }

        // Wire existing rows
        list && list.querySelectorAll('.pb-mapping-row').forEach(row => this._wireRowEvents(row));

        // ── FHIR Bundle tab events ────────────────────────────────────────────

        // Bundle type → auto-check includeRequest for transaction/batch
        const bundleTypeSelect   = root.querySelector('#pb-fb-bundle-type');
        const includeRequestChk  = root.querySelector('#pb-fb-include-request');
        if (bundleTypeSelect && includeRequestChk) {
            const syncIncludeRequest = () => {
                const bt = bundleTypeSelect.value;
                if (bt === 'transaction' || bt === 'batch') {
                    includeRequestChk.checked  = true;
                    includeRequestChk.disabled = true;
                    includeRequestChk.title    = 'Required for transaction/batch bundles';
                } else {
                    includeRequestChk.disabled = false;
                    includeRequestChk.title    = '';
                }
            };
            bundleTypeSelect.addEventListener('change', syncIncludeRequest);
            syncIncludeRequest(); // apply on initial render
        }

        // Add resource path row
        const addPathBtn  = root.querySelector('#pb-add-resource-path');
        const pathsList   = root.querySelector('#pb-resource-paths-list');
        if (addPathBtn && pathsList) {
            addPathBtn.addEventListener('click', () => {
                const count = pathsList.querySelectorAll('.pb-rp-row').length;
                const tmp = document.createElement('div');
                tmp.innerHTML = this._renderResourcePathRows(['']);
                const newRow = tmp.firstElementChild;
                newRow.dataset.index = count;
                pathsList.appendChild(newRow);
                this._wireResourcePathRowEvents(newRow);
                newRow.querySelector('.pb-rp-input')?.focus();
            });
        }

        // Wire existing resource path rows
        pathsList && pathsList.querySelectorAll('.pb-rp-row')
            .forEach(row => this._wireResourcePathRowEvents(row));

        // Load variables into template tab hint
        this._loadTemplateVars(root);
    }

    _wireRowEvents(row) {
        if (!row) return;
        row.querySelector('.pb-remove-row')?.addEventListener('click', () => row.remove());
        const typeSelect = row.querySelector('.pb-map-type');
        const valInput   = row.querySelector('.pb-map-value');
        if (typeSelect && valInput) {
            typeSelect.addEventListener('change', () => {
                valInput.placeholder = typeSelect.value === 'literal'
                    ? 'Enter value'
                    : 'e.g. PID.5.1 or steps.enrich.mrn';
            });
        }
    }

    _wireResourcePathRowEvents(row) {
        if (!row) return;
        const removeBtn = row.querySelector('.pb-rp-remove');
        if (removeBtn) {
            removeBtn.addEventListener('click', () => {
                const list = row.closest('#pb-resource-paths-list');
                row.remove();
                // Always keep at least one blank row
                if (list && list.querySelectorAll('.pb-rp-row').length === 0) {
                    const tmp = document.createElement('div');
                    tmp.innerHTML = this._renderResourcePathRows(['']);
                    const newRow = tmp.firstElementChild;
                    list.appendChild(newRow);
                    this._wireResourcePathRowEvents(newRow);
                }
            });
        }
    }

    _loadTemplateVars(root) {
        const varsDiv = root.querySelector('#pb-tpl-vars');
        if (!varsDiv) return;
        try {
            // Try to load from last test run via StepVariablesProvider
            if (this.panel && typeof this.panel.getStepVariablesForSearch === 'function') {
                const vars = this.panel.getStepVariablesForSearch();
                if (vars && vars.length > 0) {
                    varsDiv.textContent = vars.slice(0, 30).map(v => v.path || v).join('\n');
                    return;
                }
            }
        } catch (_) { /* ignore */ }
        // Fallback — common paths
        varsDiv.innerHTML = [
            'fhirBundle',
            'fhir_bundle',
            'PID.3 &mdash; Patient MRN',
            'PID.5.1 &mdash; Family Name',
            'PID.5.2 &mdash; Given Name',
            'PID.7 &mdash; Date of Birth',
            'MSH.9 &mdash; Message Type',
            'steps.&lt;step_name&gt;.step_output.&lt;field&gt;',
        ].join('\n');
    }
}

// Register under both canonical name and underscore alias.
if (typeof StepBuilderRegistry !== 'undefined') {
    StepBuilderRegistry.register('payload.builder', PayloadBuilderBuilder);
    StepBuilderRegistry.register('payload_builder', PayloadBuilderBuilder);
}
