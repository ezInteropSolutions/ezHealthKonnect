/**
 * ZSegmentConfigBuilder.js
 *
 * Enterprise Z-segment mapping configuration UI.
 *
 * Usage contexts:
 *   1. Wizard inline — auto-rendered after sample message step when Z-segments
 *      are detected, OR when user clicks "Add Z-segment".
 *   2. Interface settings — standalone panel for post-wizard editing.
 *
 * Architecture:
 *   - Tier 2: MFI.1 → FHIR resource override (per-interface)
 *   - Tier 3: Z-segment field mappings (per-interface, per Z-segment)
 *   - Templates: apply OOB or user templates; save existing config as template
 *
 * API endpoints used (all /api/zsegments/*):
 *   GET  /catalog/transforms                 → transform dropdown
 *   GET  /templates?segmentId=ZEM            → template picker
 *   POST /detect                             → auto-detect from raw message
 *   GET  /interfaces/:id                     → load existing configs
 *   POST /interfaces/:id                     → create config
 *   PUT  /interfaces/:id/:cid               → update config metadata
 *   DELETE /interfaces/:id/:cid             → delete config
 *   POST /interfaces/:id/apply-template     → apply template
 *   POST /interfaces/:id/:cid/save-as-template → save as template
 *   PUT  /interfaces/:id/:cid/fields        → upsert field mapping
 *   DELETE /interfaces/:id/:cid/fields/:fid → delete field mapping
 *   GET  /interfaces/:id/mfn-config         → MFI.1 overrides
 *   PUT  /interfaces/:id/mfn-config         → upsert MFI.1 override
 */

const FHIR_RESOURCES = [
    'Organization', 'Practitioner', 'Patient', 'Person', 'Location',
    'Coverage', 'AllergyIntolerance', 'Condition', 'Observation',
    'MedicationRequest', 'Encounter', 'RelatedPerson', 'Device',
    'ChargeItemDefinition', 'ServiceRequest', 'DiagnosticReport'
];

const RESOURCE_ROLES = [
    { value: 'primary',   label: 'Primary',   desc: 'Becomes the main assembled resource' },
    { value: 'related',   label: 'Related',   desc: 'Added as a linked resource in the bundle' },
    { value: 'extension', label: 'Extension', desc: 'Adds fields onto an existing resource' }
];

const HL7_DATA_TYPES = [
    'ST', 'IS', 'ID', 'NM', 'SI', 'DT', 'TS', 'DTM',
    'CWE', 'CE', 'CNE', 'CX', 'XPN', 'XAD', 'XTN', 'XON', 'HD', 'EI'
];

class ZSegmentConfigBuilder extends EventTarget {
    constructor(options = {}) {
        super();

        this.interfaceId = options.interfaceId || null;
        this.containerId = options.containerId || 'zsegment-config-container';
        this.readOnly    = options.readOnly    || false;

        // Loaded data
        this.configs      = [];   // ZSegmentConfig[]
        this.mfnOverrides = [];   // MFNInterfaceConfig[]
        this.transforms   = [];   // ZSegmentTransform[]
        this.templates    = [];   // ZSegmentTemplate[]

        // UI state
        this._editingConfig   = null;  // configId being edited, or null
        this._editingField    = null;  // fieldId being edited, or null
        this._expandedConfigs = new Set();

        this._initialized = false;
    }

    // ───────────────────────────────────────────────────────────────────────
    // Public API
    // ───────────────────────────────────────────────────────────────────────

    /**
     * Initialize the builder.  Must be called before render().
     * Loads the transform catalog and existing configs from the API.
     */
    async init() {
        if (!this.interfaceId) {
            console.warn('ZSegmentConfigBuilder: no interfaceId set — operating in standalone/create mode');
        }
        await Promise.all([
            this._loadTransforms(),
            this._loadTemplates(),
            this.interfaceId ? this._loadConfigs() : Promise.resolve(),
            this.interfaceId ? this._loadMFNOverrides() : Promise.resolve()
        ]);
        this._initialized = true;
        return this;
    }

    /**
     * Detect Z-segments from a raw HL7 message string.
     * Returns { detectedSegments, data } from the API.
     */
    async detectFromMessage(rawMessage) {
        const res = await fetch('/api/zsegments/detect', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ rawMessage })
        });
        const json = await res.json();
        if (!json.success) throw new Error(json.error || 'Detection failed');
        return json;
    }

    /**
     * Render the full builder UI into this.containerId.
     */
    render() {
        const container = document.getElementById(this.containerId);
        if (!container) {
            console.error(`ZSegmentConfigBuilder: container #${this.containerId} not found`);
            return;
        }
        container.innerHTML = this._buildHTML();
        this._attachEvents(container);
        return this;
    }

    /**
     * Render a compact "detected Z-segments" banner with one-click apply.
     * Used by the wizard when Z-segments are found in the sample message.
     */
    renderDetectionBanner(detectionResult, container) {
        if (!detectionResult.detectedSegments || detectionResult.detectedSegments.length === 0) {
            container.innerHTML = '';
            return;
        }

        const esc = s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');

        const segCards = detectionResult.data.map(seg => {
            const hasOOB = seg.hasOobTemplate;
            const oobTmpl = seg.suggestedTemplates.find(t => t.isSystem);
            return `
            <div class="zseg-detect-card ${hasOOB ? 'has-oob' : ''}" data-segment-id="${esc(seg.segmentId)}">
                <div class="zseg-detect-card-header">
                    <span class="zseg-segment-badge">${esc(seg.segmentId)}</span>
                    ${hasOOB ? `<span class="zseg-oob-badge">OOB Template</span>` : '<span class="zseg-custom-badge">Custom</span>'}
                </div>
                ${oobTmpl ? `<p class="zseg-detect-desc">${esc(oobTmpl.description)}</p>` : `<p class="zseg-detect-desc">No OOB template — configure manually</p>`}
                <div class="zseg-detect-actions">
                    ${oobTmpl ? `<button class="btn btn-sm btn-primary zseg-apply-oob" data-template-id="${esc(oobTmpl.id)}" data-segment-id="${esc(seg.segmentId)}">Apply OOB Template</button>` : ''}
                    <button class="btn btn-sm btn-outline-secondary zseg-configure-manual" data-segment-id="${esc(seg.segmentId)}">Configure Manually</button>
                </div>
            </div>`;
        }).join('');

        container.innerHTML = `
        <div class="zseg-detection-banner">
            <div class="zseg-detection-header">
                <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                    <circle cx="12" cy="12" r="10"/><path d="M12 8v4m0 4h.01"/>
                </svg>
                <strong>Z-segments detected in your sample message</strong>
                <span class="zseg-detection-count">${detectionResult.detectedSegments.length} found</span>
            </div>
            <div class="zseg-detect-cards">${segCards}</div>
            <div id="zseg-inline-config"></div>
        </div>`;

        this._attachBannerEvents(container);
    }

    // ───────────────────────────────────────────────────────────────────────
    // HTML builders
    // ───────────────────────────────────────────────────────────────────────

    _buildHTML() {
        const esc = s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
        return `
        <div class="zseg-builder">
            ${this._buildStyles()}
            <div class="zseg-builder-header">
                <h3 class="zseg-builder-title">Z-Segment Mapping</h3>
                <p class="zseg-builder-subtitle">
                    Configure how site-specific Z-segments map to FHIR resources for this interface.
                    Z-segments have no standard schema — each trading partner defines their own field positions.
                </p>
            </div>

            ${this._buildMFNOverridesSection()}
            ${this._buildZSegmentsSection()}
        </div>`;
    }

    _buildMFNOverridesSection() {
        const esc = s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');

        const rows = this.mfnOverrides.map(o => `
            <tr>
                <td><code>${esc(o.mfiIdentifier)}</code></td>
                <td>${esc(o.fhirResource)}</td>
                <td>${esc(o.description || '—')}</td>
                <td>
                    <span class="badge ${o.isActive ? 'badge-success' : 'badge-secondary'}">${o.isActive ? 'Active' : 'Inactive'}</span>
                </td>
                <td>
                    <button class="btn btn-xs btn-outline-danger zseg-delete-mfn" data-id="${esc(o.id)}" title="Remove">×</button>
                </td>
            </tr>`).join('');

        return `
        <div class="zseg-section" id="zseg-mfn-section">
            <div class="zseg-section-header">
                <h4>MFI.1 → FHIR Resource Override <span class="zseg-tier-badge">Tier 2</span></h4>
                <p class="text-muted small">
                    Override the default MFI.1 mapping for non-standard codes (e.g. <code>EMP</code> → <code>Organization</code>).
                    Standard codes STF, LOC, CDM are always hardcoded and cannot be overridden.
                </p>
            </div>

            <table class="table table-sm zseg-mfn-table">
                <thead><tr><th>MFI.1</th><th>FHIR Resource</th><th>Description</th><th>Status</th><th></th></tr></thead>
                <tbody id="zseg-mfn-rows">${rows}</tbody>
            </table>

            <div class="zseg-add-mfn-form" id="zseg-add-mfn-form" style="display:none;">
                <div class="form-row">
                    <div class="form-group col-md-3">
                        <label>MFI.1 Code</label>
                        <input type="text" class="form-control" id="zseg-mfn-code" placeholder="EMP" maxlength="20">
                    </div>
                    <div class="form-group col-md-3">
                        <label>FHIR Resource</label>
                        <select class="form-control" id="zseg-mfn-resource">
                            ${FHIR_RESOURCES.map(r => `<option value="${r}">${r}</option>`).join('')}
                        </select>
                    </div>
                    <div class="form-group col-md-4">
                        <label>Description</label>
                        <input type="text" class="form-control" id="zseg-mfn-desc" placeholder="Optional description">
                    </div>
                    <div class="form-group col-md-2 d-flex align-items-end">
                        <button class="btn btn-primary btn-sm mr-1" id="zseg-save-mfn">Save</button>
                        <button class="btn btn-secondary btn-sm" id="zseg-cancel-mfn">Cancel</button>
                    </div>
                </div>
            </div>

            <button class="btn btn-sm btn-outline-primary mt-2" id="zseg-add-mfn-btn">+ Add MFI.1 Override</button>
        </div>`;
    }

    _buildZSegmentsSection() {
        const configCards = this.configs.map(c => this._buildConfigCard(c)).join('');

        return `
        <div class="zseg-section" id="zseg-configs-section">
            <div class="zseg-section-header">
                <h4>Z-Segment Configurations <span class="zseg-tier-badge">Tier 3</span></h4>
                <p class="text-muted small">
                    Define which Z-segments appear in messages from this trading partner and how each
                    field maps to a FHIR resource.
                </p>
                <div class="zseg-section-actions">
                    <button class="btn btn-sm btn-outline-secondary mr-2" id="zseg-from-template-btn">
                        📋 Apply Template
                    </button>
                    <button class="btn btn-sm btn-primary" id="zseg-add-config-btn">
                        + Add Z-Segment
                    </button>
                </div>
            </div>

            <div id="zseg-config-cards">
                ${configCards || '<p class="text-muted">No Z-segments configured. Click "Add Z-Segment" or apply a template.</p>'}
            </div>

            <!-- Add/Edit config panel (hidden by default) -->
            <div id="zseg-config-editor" style="display:none;">
                ${this._buildConfigEditor()}
            </div>

            <!-- Template picker panel (hidden by default) -->
            <div id="zseg-template-picker" style="display:none;">
                ${this._buildTemplatePicker()}
            </div>
        </div>`;
    }

    _buildConfigCard(cfg) {
        const esc = s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
        const isExpanded = this._expandedConfigs.has(cfg.id);
        const roleColors = { primary: '#0d6efd', related: '#6f42c1', extension: '#20c997' };
        const roleColor  = roleColors[cfg.resourceRole] || '#6c757d';

        const fieldRows = (cfg.fieldMappings || []).map((f, i) => `
            <tr class="zseg-field-row">
                <td><code>${esc(f.position)}</code></td>
                <td>${esc(f.fieldLabel || '—')}</td>
                <td><span class="badge badge-light">${esc(f.hl7DataType)}</span></td>
                <td><code class="small">${esc(f.fhirPath)}</code></td>
                <td><span class="badge badge-info">${esc(f.transform)}</span></td>
                <td>${f.isRequired ? '<span class="text-danger">✓</span>' : ''}</td>
                <td>
                    <button class="btn btn-xs btn-outline-primary zseg-edit-field" data-config-id="${esc(cfg.id)}" data-field-id="${esc(f.id)}" title="Edit">✎</button>
                    <button class="btn btn-xs btn-outline-danger zseg-delete-field" data-config-id="${esc(cfg.id)}" data-field-id="${esc(f.id)}" title="Delete">×</button>
                </td>
            </tr>`).join('');

        return `
        <div class="zseg-config-card ${cfg.isActive ? '' : 'inactive'}" data-config-id="${esc(cfg.id)}">
            <div class="zseg-config-card-header" data-toggle-config="${esc(cfg.id)}">
                <div class="zseg-config-card-title">
                    <span class="zseg-segment-badge">${esc(cfg.segmentId)}</span>
                    <span class="zseg-arrow">→</span>
                    <span class="zseg-resource-name">${esc(cfg.targetFhirResource)}</span>
                    <span class="zseg-role-badge" style="background:${roleColor}20;color:${roleColor};">${esc(cfg.resourceRole)}</span>
                    <span class="zseg-field-count">${(cfg.fieldMappings || []).length} fields</span>
                    ${!cfg.isActive ? '<span class="badge badge-warning ml-2">Inactive</span>' : ''}
                    ${cfg.sourceTemplateId ? '<span class="badge badge-secondary ml-1" title="Applied from template">template</span>' : ''}
                </div>
                <div class="zseg-config-card-actions" onclick="event.stopPropagation()">
                    <button class="btn btn-xs btn-outline-primary zseg-edit-config" data-config-id="${esc(cfg.id)}" title="Edit">✎</button>
                    <button class="btn btn-xs btn-outline-secondary zseg-save-as-template" data-config-id="${esc(cfg.id)}" title="Save as Template">📋</button>
                    <button class="btn btn-xs btn-outline-danger zseg-delete-config" data-config-id="${esc(cfg.id)}" title="Delete">🗑</button>
                    <span class="zseg-expand-icon">${isExpanded ? '▲' : '▼'}</span>
                </div>
            </div>

            ${isExpanded ? `
            <div class="zseg-config-card-body">
                ${cfg.description ? `<p class="text-muted small mb-2">${esc(cfg.description)}</p>` : ''}
                <table class="table table-sm zseg-fields-table">
                    <thead>
                        <tr>
                            <th>Position</th><th>Label</th><th>HL7 Type</th>
                            <th>FHIR Path</th><th>Transform</th><th>Req.</th><th></th>
                        </tr>
                    </thead>
                    <tbody id="zseg-fields-${esc(cfg.id)}">${fieldRows}</tbody>
                </table>
                <button class="btn btn-xs btn-outline-primary mt-1 zseg-add-field" data-config-id="${esc(cfg.id)}">
                    + Add Field Mapping
                </button>
            </div>

            <!-- Inline field editor for this config -->
            <div class="zseg-field-editor" id="zseg-field-editor-${esc(cfg.id)}" style="display:none;">
                ${this._buildFieldEditor(cfg.id)}
            </div>
            ` : ''}
        </div>`;
    }

    _buildConfigEditor(cfg = null) {
        const esc = s => String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
        const isEdit = !!cfg;
        return `
        <div class="zseg-editor-panel">
            <h5>${isEdit ? 'Edit Z-Segment' : 'Add Z-Segment'}</h5>
            <div class="form-row">
                <div class="form-group col-md-3">
                    <label>Segment ID <span class="text-danger">*</span></label>
                    <input type="text" class="form-control text-uppercase" id="zseg-editor-segment-id"
                           placeholder="ZEM" maxlength="10"
                           value="${esc(cfg ? cfg.segmentId : '')}"
                           ${isEdit ? 'readonly' : ''}>
                    <small class="form-text text-muted">e.g. ZEM, ZPI, ZIN, ZPD</small>
                </div>
                <div class="form-group col-md-3">
                    <label>Target FHIR Resource <span class="text-danger">*</span></label>
                    <select class="form-control" id="zseg-editor-resource">
                        ${FHIR_RESOURCES.map(r => `<option value="${r}" ${cfg && cfg.targetFhirResource === r ? 'selected' : ''}>${r}</option>`).join('')}
                    </select>
                </div>
                <div class="form-group col-md-3">
                    <label>Role</label>
                    <select class="form-control" id="zseg-editor-role">
                        ${RESOURCE_ROLES.map(r => `<option value="${r.value}" ${cfg && cfg.resourceRole === r.value ? 'selected' : ''} title="${esc(r.desc)}">${r.label}</option>`).join('')}
                    </select>
                </div>
                <div class="form-group col-md-3">
                    <label>Occurrence (if duplicate seg IDs)</label>
                    <input type="number" class="form-control" id="zseg-editor-occurrence"
                           min="0" max="9" value="${esc(cfg ? cfg.occurrenceIndex : 0)}">
                    <small class="form-text text-muted">0 = first, 1 = second, etc.</small>
                </div>
            </div>
            <div class="form-group">
                <label>Description</label>
                <input type="text" class="form-control" id="zseg-editor-desc"
                       placeholder="Optional description for this Z-segment"
                       value="${esc(cfg ? cfg.description : '')}">
            </div>
            <div class="d-flex">
                <button class="btn btn-primary mr-2" id="zseg-editor-save" data-config-id="${esc(cfg ? cfg.id : '')}">
                    ${isEdit ? 'Update' : 'Add Z-Segment'}
                </button>
                <button class="btn btn-secondary" id="zseg-editor-cancel">Cancel</button>
            </div>
        </div>`;
    }

    _buildFieldEditor(configId, field = null) {
        const esc = s => String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
        const isEdit = !!field;
        const transformOptions = this.transforms.map(t =>
            `<option value="${esc(t.code)}" ${field && field.transform === t.code ? 'selected' : ''}>${esc(t.label)}</option>`
        ).join('');

        return `
        <div class="zseg-field-editor-inner">
            <h6>${isEdit ? 'Edit Field' : 'Add Field Mapping'}</h6>
            <div class="form-row">
                <div class="form-group col-md-2">
                    <label>Position <span class="text-danger">*</span></label>
                    <input type="text" class="form-control form-control-sm" id="zseg-field-position-${esc(configId)}"
                           placeholder="ZEM.1" value="${esc(field ? field.position : '')}">
                    <small class="form-text text-muted">e.g. ZEM.1, ZEM.3.2</small>
                </div>
                <div class="form-group col-md-3">
                    <label>Label</label>
                    <input type="text" class="form-control form-control-sm" id="zseg-field-label-${esc(configId)}"
                           placeholder="Employer ID" value="${esc(field ? field.fieldLabel : '')}">
                </div>
                <div class="form-group col-md-2">
                    <label>HL7 Type</label>
                    <select class="form-control form-control-sm" id="zseg-field-datatype-${esc(configId)}">
                        ${HL7_DATA_TYPES.map(t => `<option value="${t}" ${field && field.hl7DataType === t ? 'selected' : ''}>${t}</option>`).join('')}
                    </select>
                </div>
                <div class="form-group col-md-2">
                    <label>FHIR Path <span class="text-danger">*</span></label>
                    <input type="text" class="form-control form-control-sm" id="zseg-field-fhirpath-${esc(configId)}"
                           placeholder="identifier[0].value" value="${esc(field ? field.fhirPath : '')}">
                </div>
                <div class="form-group col-md-2">
                    <label>Transform</label>
                    <select class="form-control form-control-sm" id="zseg-field-transform-${esc(configId)}">
                        ${transformOptions}
                    </select>
                </div>
                <div class="form-group col-md-1 d-flex flex-column justify-content-end">
                    <div class="form-check mb-1">
                        <input class="form-check-input" type="checkbox" id="zseg-field-required-${esc(configId)}"
                               ${field && field.isRequired ? 'checked' : ''}>
                        <label class="form-check-label" for="zseg-field-required-${esc(configId)}">Req.</label>
                    </div>
                </div>
            </div>
            <div class="form-group">
                <label>Default Value (when field empty)</label>
                <input type="text" class="form-control form-control-sm" id="zseg-field-default-${esc(configId)}"
                       placeholder="Optional default" value="${esc(field ? field.defaultValue : '')}">
            </div>
            <div class="d-flex">
                <button class="btn btn-sm btn-primary mr-2 zseg-field-save"
                        data-config-id="${esc(configId)}"
                        data-field-id="${esc(field ? field.id : '')}">
                    ${isEdit ? 'Update Field' : 'Add Field'}
                </button>
                <button class="btn btn-sm btn-secondary zseg-field-cancel" data-config-id="${esc(configId)}">Cancel</button>
            </div>
        </div>`;
    }

    _buildTemplatePicker() {
        const esc = s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
        const cards = this.templates.map(t => `
            <div class="zseg-template-card" data-template-id="${esc(t.id)}">
                <div class="zseg-template-card-header">
                    <span class="zseg-segment-badge">${esc(t.segmentId)}</span>
                    <strong>${esc(t.templateName)}</strong>
                    ${t.isSystem ? '<span class="badge badge-primary ml-1">OOB</span>' : ''}
                    <span class="zseg-resource-name ml-2">→ ${esc(t.targetFhirResource)}</span>
                    <span class="zseg-field-count">${(t.fields || []).length} fields</span>
                </div>
                <p class="text-muted small mb-2">${esc(t.description || '')}</p>
                <div class="zseg-template-fields-preview">
                    ${(t.fields || []).slice(0, 4).map(f =>
                        `<span class="zseg-field-chip"><code>${esc(f.position)}</code> → ${esc(f.fhirPath)}</span>`
                    ).join('')}
                    ${(t.fields || []).length > 4 ? `<span class="text-muted small">+${(t.fields || []).length - 4} more</span>` : ''}
                </div>
                <div class="mt-2">
                    <span class="text-muted small">Used ${t.usageCount} time${t.usageCount !== 1 ? 's' : ''}</span>
                </div>
                <button class="btn btn-sm btn-primary mt-2 zseg-apply-template" data-template-id="${esc(t.id)}">
                    Apply This Template
                </button>
            </div>`).join('') || '<p class="text-muted">No templates available.</p>';

        return `
        <div class="zseg-editor-panel">
            <div class="d-flex justify-content-between align-items-center mb-3">
                <h5>Apply Template</h5>
                <button class="btn btn-sm btn-secondary" id="zseg-template-picker-close">Close</button>
            </div>
            <div class="form-group">
                <input type="text" class="form-control" id="zseg-template-search"
                       placeholder="Search templates by name or segment…">
            </div>
            <div class="zseg-template-grid" id="zseg-template-grid">${cards}</div>
        </div>`;
    }

    // ───────────────────────────────────────────────────────────────────────
    // Event wiring
    // ───────────────────────────────────────────────────────────────────────

    _attachEvents(container) {
        // MFI.1 section
        container.querySelector('#zseg-add-mfn-btn')?.addEventListener('click', () => {
            const form = container.querySelector('#zseg-add-mfn-form');
            form.style.display = form.style.display === 'none' ? 'block' : 'none';
        });
        container.querySelector('#zseg-save-mfn')?.addEventListener('click', () => this._saveMFNOverride(container));
        container.querySelector('#zseg-cancel-mfn')?.addEventListener('click', () => {
            container.querySelector('#zseg-add-mfn-form').style.display = 'none';
        });

        // Delete MFI override
        container.addEventListener('click', async e => {
            if (e.target.classList.contains('zseg-delete-mfn')) {
                if (!confirm('Remove this MFI.1 override?')) return;
                const id = e.target.dataset.id;
                await this._deleteMFNOverride(id);
                this._refresh(container);
            }
        });

        // Add Z-segment button
        container.querySelector('#zseg-add-config-btn')?.addEventListener('click', () => {
            const editor = container.querySelector('#zseg-config-editor');
            editor.innerHTML = this._buildConfigEditor(null);
            editor.style.display = 'block';
            this._attachConfigEditorEvents(container);
        });

        // Apply template button
        container.querySelector('#zseg-from-template-btn')?.addEventListener('click', () => {
            const picker = container.querySelector('#zseg-template-picker');
            picker.style.display = picker.style.display === 'none' ? 'block' : 'none';
            this._attachTemplatePickerEvents(container);
        });

        // Expand/collapse config cards
        container.addEventListener('click', e => {
            const header = e.target.closest('[data-toggle-config]');
            if (header) {
                const configId = header.dataset.toggleConfig;
                if (this._expandedConfigs.has(configId)) {
                    this._expandedConfigs.delete(configId);
                } else {
                    this._expandedConfigs.add(configId);
                }
                this._refresh(container);
            }
        });

        // Edit config
        container.addEventListener('click', e => {
            if (e.target.classList.contains('zseg-edit-config')) {
                e.stopPropagation();
                const configId = e.target.dataset.configId;
                const cfg = this.configs.find(c => c.id === configId);
                if (!cfg) return;
                const editor = container.querySelector('#zseg-config-editor');
                editor.innerHTML = this._buildConfigEditor(cfg);
                editor.style.display = 'block';
                this._attachConfigEditorEvents(container);
            }
        });

        // Delete config
        container.addEventListener('click', async e => {
            if (e.target.classList.contains('zseg-delete-config')) {
                e.stopPropagation();
                if (!confirm('Delete this Z-segment config and all its field mappings?')) return;
                await this._deleteConfig(e.target.dataset.configId);
                this._refresh(container);
            }
        });

        // Save as template
        container.addEventListener('click', e => {
            if (e.target.classList.contains('zseg-save-as-template')) {
                e.stopPropagation();
                this._promptSaveAsTemplate(e.target.dataset.configId, container);
            }
        });

        // Add field mapping
        container.addEventListener('click', e => {
            if (e.target.classList.contains('zseg-add-field')) {
                e.stopPropagation();
                const configId = e.target.dataset.configId;
                const editorId = `zseg-field-editor-${configId}`;
                const editorEl = container.querySelector(`#${editorId}`);
                if (editorEl) {
                    editorEl.innerHTML = this._buildFieldEditor(configId, null);
                    editorEl.style.display = 'block';
                    this._attachFieldEditorEvents(container, configId);
                }
            }
        });

        // Edit field
        container.addEventListener('click', e => {
            if (e.target.classList.contains('zseg-edit-field')) {
                e.stopPropagation();
                const { configId, fieldId } = e.target.dataset;
                const cfg = this.configs.find(c => c.id === configId);
                const field = cfg?.fieldMappings?.find(f => f.id === fieldId);
                if (!field) return;
                const editorId = `zseg-field-editor-${configId}`;
                const editorEl = container.querySelector(`#${editorId}`);
                if (editorEl) {
                    editorEl.innerHTML = this._buildFieldEditor(configId, field);
                    editorEl.style.display = 'block';
                    this._attachFieldEditorEvents(container, configId);
                }
            }
        });

        // Delete field
        container.addEventListener('click', async e => {
            if (e.target.classList.contains('zseg-delete-field')) {
                e.stopPropagation();
                if (!confirm('Remove this field mapping?')) return;
                const { configId, fieldId } = e.target.dataset;
                await this._deleteField(configId, fieldId);
                this._refresh(container);
            }
        });

        // Field editor: save / cancel (delegated via class)
        container.addEventListener('click', async e => {
            if (e.target.classList.contains('zseg-field-save')) {
                const { configId, fieldId } = e.target.dataset;
                await this._saveField(configId, fieldId || null, container);
            }
            if (e.target.classList.contains('zseg-field-cancel')) {
                const configId = e.target.dataset.configId;
                const editorEl = container.querySelector(`#zseg-field-editor-${configId}`);
                if (editorEl) editorEl.style.display = 'none';
            }
        });
    }

    _attachBannerEvents(container) {
        container.addEventListener('click', async e => {
            // Apply OOB template
            if (e.target.classList.contains('zseg-apply-oob')) {
                const { templateId } = e.target.dataset;
                try {
                    await this._applyTemplate(templateId);
                    this.dispatchEvent(new CustomEvent('configChanged', { detail: { configs: this.configs } }));
                    e.target.textContent = '✓ Applied';
                    e.target.disabled = true;
                    e.target.classList.replace('btn-primary', 'btn-success');
                } catch (err) {
                    alert('Failed to apply template: ' + err.message);
                }
            }
            // Configure manually
            if (e.target.classList.contains('zseg-configure-manual')) {
                const segmentId = e.target.dataset.segmentId;
                const inlineArea = container.querySelector('#zseg-inline-config');
                if (inlineArea) {
                    inlineArea.innerHTML = `
                        <div style="margin-top:12px;padding:12px;background:#f8f9fa;border-radius:6px;">
                            ${this._buildConfigEditor(null)}
                        </div>`;
                    // Pre-fill segment ID
                    const segInput = inlineArea.querySelector('#zseg-editor-segment-id');
                    if (segInput) segInput.value = segmentId;
                    this._attachConfigEditorEvents(inlineArea);
                }
            }
        });
    }

    _attachConfigEditorEvents(container) {
        container.querySelector('#zseg-editor-save')?.addEventListener('click', async e => {
            const configId = e.target.dataset.configId || null;
            await this._saveConfig(configId, container);
        });
        container.querySelector('#zseg-editor-cancel')?.addEventListener('click', () => {
            const editor = container.querySelector('#zseg-config-editor');
            if (editor) editor.style.display = 'none';
        });
    }

    _attachFieldEditorEvents(container, configId) {
        // Events are delegated via class in _attachEvents — no additional wiring needed here
    }

    _attachTemplatePickerEvents(container) {
        container.querySelector('#zseg-template-picker-close')?.addEventListener('click', () => {
            container.querySelector('#zseg-template-picker').style.display = 'none';
        });

        container.querySelector('#zseg-template-search')?.addEventListener('input', e => {
            const q = e.target.value.toLowerCase();
            container.querySelectorAll('.zseg-template-card').forEach(card => {
                const text = card.textContent.toLowerCase();
                card.style.display = text.includes(q) ? '' : 'none';
            });
        });

        container.addEventListener('click', async e => {
            if (e.target.classList.contains('zseg-apply-template')) {
                const templateId = e.target.dataset.templateId;
                try {
                    await this._applyTemplate(templateId);
                    container.querySelector('#zseg-template-picker').style.display = 'none';
                    this._refresh(container);
                } catch (err) {
                    alert('Failed to apply template: ' + err.message);
                }
            }
        });
    }

    // ───────────────────────────────────────────────────────────────────────
    // API calls
    // ───────────────────────────────────────────────────────────────────────

    async _loadTransforms() {
        try {
            const res = await fetch('/api/zsegments/catalog/transforms');
            const json = await res.json();
            if (json.success) this.transforms = json.data || [];
        } catch (e) {
            console.warn('ZSegmentConfigBuilder: failed to load transform catalog', e);
            // Fallback to static list so the UI still works
            this.transforms = [
                { code: 'identity',        label: 'As-Is' },
                { code: 'first_component', label: 'First Component (^)' },
                { code: 'xad_to_address',  label: 'XAD → Address' },
                { code: 'xpn_to_humanname',label: 'XPN → HumanName' },
                { code: 'xtn_to_telecom',  label: 'XTN → ContactPoint' },
                { code: 'cwe_to_codeable', label: 'CWE → CodeableConcept' },
                { code: 'ts_to_date',      label: 'TS → Date' },
                { code: 'ts_to_datetime',  label: 'TS → DateTime' },
            ];
        }
    }

    async _loadTemplates(segmentId = '') {
        const url = '/api/zsegments/templates' + (segmentId ? `?segmentId=${segmentId}` : '');
        const res = await fetch(url);
        const json = await res.json();
        if (json.success) this.templates = json.data || [];
    }

    async _loadConfigs() {
        const res = await fetch(`/api/zsegments/interfaces/${this.interfaceId}`);
        const json = await res.json();
        if (json.success) this.configs = json.data || [];
    }

    async _loadMFNOverrides() {
        const res = await fetch(`/api/zsegments/interfaces/${this.interfaceId}/mfn-config`);
        const json = await res.json();
        if (json.success) this.mfnOverrides = json.data || [];
    }

    async _saveMFNOverride(container) {
        const code = container.querySelector('#zseg-mfn-code')?.value?.trim().toUpperCase();
        const resource = container.querySelector('#zseg-mfn-resource')?.value;
        const desc = container.querySelector('#zseg-mfn-desc')?.value?.trim();
        if (!code) { alert('MFI.1 code is required'); return; }

        await fetch(`/api/zsegments/interfaces/${this.interfaceId}/mfn-config`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mfiIdentifier: code, fhirResource: resource, description: desc, isActive: true })
        });
        await this._loadMFNOverrides();
        this._refresh(container);
    }

    async _deleteMFNOverride(id) {
        await fetch(`/api/zsegments/interfaces/${this.interfaceId}/mfn-config/${id}`, { method: 'DELETE' });
        await this._loadMFNOverrides();
    }

    async _saveConfig(configId, container) {
        const segmentId = container.querySelector('#zseg-editor-segment-id')?.value?.trim().toUpperCase();
        const resource  = container.querySelector('#zseg-editor-resource')?.value;
        const role      = container.querySelector('#zseg-editor-role')?.value;
        const occurrence= parseInt(container.querySelector('#zseg-editor-occurrence')?.value || '0', 10);
        const desc      = container.querySelector('#zseg-editor-desc')?.value?.trim();

        if (!segmentId) { alert('Segment ID is required'); return; }
        if (!segmentId.startsWith('Z')) { alert('Z-segment IDs must start with Z (e.g. ZEM, ZPI)'); return; }

        const body = { segmentId, targetFhirResource: resource, resourceRole: role, occurrenceIndex: occurrence, description: desc };

        if (configId) {
            await fetch(`/api/zsegments/interfaces/${this.interfaceId}/${configId}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });
        } else {
            const res = await fetch(`/api/zsegments/interfaces/${this.interfaceId}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });
            const json = await res.json();
            if (json.success && json.data) {
                this._expandedConfigs.add(json.data.id);
            }
        }

        await this._loadConfigs();
        this._refresh(container);
        this.dispatchEvent(new CustomEvent('configChanged', { detail: { configs: this.configs } }));
    }

    async _deleteConfig(configId) {
        await fetch(`/api/zsegments/interfaces/${this.interfaceId}/${configId}`, { method: 'DELETE' });
        await this._loadConfigs();
    }

    async _applyTemplate(templateId) {
        const res = await fetch(`/api/zsegments/interfaces/${this.interfaceId}/apply-template`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ templateId, occurrenceIndex: 0 })
        });
        const json = await res.json();
        if (!json.success) throw new Error(json.error || 'Apply template failed');
        if (json.data) this._expandedConfigs.add(json.data.id);
        await this._loadConfigs();
        return json.data;
    }

    async _saveField(configId, fieldId, container) {
        const position  = container.querySelector(`#zseg-field-position-${configId}`)?.value?.trim();
        const label     = container.querySelector(`#zseg-field-label-${configId}`)?.value?.trim();
        const dataType  = container.querySelector(`#zseg-field-datatype-${configId}`)?.value;
        const fhirPath  = container.querySelector(`#zseg-field-fhirpath-${configId}`)?.value?.trim();
        const transform = container.querySelector(`#zseg-field-transform-${configId}`)?.value;
        const required  = container.querySelector(`#zseg-field-required-${configId}`)?.checked;
        const defVal    = container.querySelector(`#zseg-field-default-${configId}`)?.value?.trim();

        if (!position) { alert('Position is required (e.g. ZEM.2)'); return; }
        if (!fhirPath) { alert('FHIR path is required'); return; }

        await fetch(`/api/zsegments/interfaces/${this.interfaceId}/${configId}/fields`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                position, fieldLabel: label, hl7DataType: dataType,
                fhirPath, transform, isRequired: required, defaultValue: defVal
            })
        });

        const editorEl = container.querySelector(`#zseg-field-editor-${configId}`);
        if (editorEl) editorEl.style.display = 'none';

        await this._loadConfigs();
        this._refresh(container);
    }

    async _deleteField(configId, fieldId) {
        await fetch(`/api/zsegments/interfaces/${this.interfaceId}/${configId}/fields/${fieldId}`, { method: 'DELETE' });
        await this._loadConfigs();
    }

    async _promptSaveAsTemplate(configId, container) {
        const name = prompt('Template name:', '');
        if (!name) return;
        const desc = prompt('Description (optional):', '');
        const res = await fetch(`/api/zsegments/interfaces/${this.interfaceId}/${configId}/save-as-template`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ templateName: name, description: desc || '', isPublic: true, tags: [] })
        });
        const json = await res.json();
        if (json.success) {
            alert(`Template "${name}" saved successfully.`);
            await this._loadTemplates();
        } else {
            alert('Failed to save template: ' + (json.error || 'Unknown error'));
        }
    }

    _refresh(container) {
        const cardsEl = container.querySelector('#zseg-config-cards');
        if (cardsEl) {
            const configCards = this.configs.map(c => this._buildConfigCard(c)).join('');
            cardsEl.innerHTML = configCards || '<p class="text-muted">No Z-segments configured. Click "Add Z-Segment" or apply a template.</p>';
        }
        const mfnRowsEl = container.querySelector('#zseg-mfn-rows');
        if (mfnRowsEl) {
            const esc = s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
            mfnRowsEl.innerHTML = this.mfnOverrides.map(o => `
                <tr>
                    <td><code>${esc(o.mfiIdentifier)}</code></td>
                    <td>${esc(o.fhirResource)}</td>
                    <td>${esc(o.description || '—')}</td>
                    <td><span class="badge ${o.isActive ? 'badge-success' : 'badge-secondary'}">${o.isActive ? 'Active' : 'Inactive'}</span></td>
                    <td><button class="btn btn-xs btn-outline-danger zseg-delete-mfn" data-id="${esc(o.id)}">×</button></td>
                </tr>`).join('');
        }
    }

    // ───────────────────────────────────────────────────────────────────────
    // Styles
    // ───────────────────────────────────────────────────────────────────────

    _buildStyles() {
        return `<style>
.zseg-builder { font-size: 14px; }
.zseg-builder-title { font-size: 1.1rem; font-weight: 600; margin-bottom: 4px; }
.zseg-builder-subtitle { color: #6c757d; font-size: 13px; margin-bottom: 20px; }
.zseg-section { border: 1px solid #dee2e6; border-radius: 6px; padding: 16px; margin-bottom: 20px; }
.zseg-section-header { display: flex; flex-wrap: wrap; align-items: flex-start; gap: 8px; margin-bottom: 12px; }
.zseg-section-header h4 { margin: 0; flex: 1 1 auto; font-size: 0.95rem; }
.zseg-section-actions { display: flex; gap: 8px; }
.zseg-tier-badge { font-size: 10px; background: #e9ecef; color: #495057; padding: 2px 6px; border-radius: 10px; font-weight: 500; }
.zseg-segment-badge { display: inline-block; background: #0d6efd; color: white; padding: 2px 8px; border-radius: 4px; font-weight: 700; font-size: 12px; font-family: monospace; }
.zseg-oob-badge { background: #198754; color: white; font-size: 11px; padding: 2px 6px; border-radius: 10px; }
.zseg-custom-badge { background: #6c757d; color: white; font-size: 11px; padding: 2px 6px; border-radius: 10px; }
.zseg-resource-name { font-weight: 500; color: #0d6efd; }
.zseg-role-badge { font-size: 11px; padding: 2px 8px; border-radius: 10px; font-weight: 500; }
.zseg-field-count { font-size: 11px; color: #6c757d; background: #f8f9fa; padding: 2px 6px; border-radius: 10px; }
.zseg-arrow { color: #adb5bd; margin: 0 4px; }
.zseg-config-card { border: 1px solid #dee2e6; border-radius: 6px; margin-bottom: 10px; overflow: hidden; }
.zseg-config-card.inactive { opacity: 0.6; }
.zseg-config-card-header { display: flex; align-items: center; justify-content: space-between; padding: 10px 14px; background: #f8f9fa; cursor: pointer; user-select: none; }
.zseg-config-card-header:hover { background: #e9ecef; }
.zseg-config-card-title { display: flex; align-items: center; gap: 8px; flex: 1; }
.zseg-config-card-actions { display: flex; align-items: center; gap: 6px; }
.zseg-config-card-body { padding: 14px; }
.zseg-expand-icon { color: #6c757d; font-size: 11px; }
.zseg-fields-table { margin-bottom: 4px; }
.zseg-field-row { font-size: 12px; }
.zseg-field-chip { display: inline-block; background: #f0f4ff; border: 1px solid #c9d8ff; border-radius: 4px; padding: 2px 6px; font-size: 12px; margin: 2px; }
.zseg-editor-panel { background: #f8f9fa; border: 1px solid #dee2e6; border-radius: 6px; padding: 16px; margin-top: 12px; }
.zseg-editor-panel h5 { font-size: 0.9rem; margin-bottom: 12px; }
.zseg-field-editor-inner { background: #fff; border: 1px solid #dee2e6; border-radius: 4px; padding: 12px; margin-top: 8px; }
.zseg-field-editor-inner h6 { font-size: 0.85rem; margin-bottom: 10px; }
.zseg-add-mfn-form { background: #f8f9fa; border: 1px solid #dee2e6; border-radius: 4px; padding: 12px; margin-top: 8px; }
.zseg-mfn-table { margin-bottom: 0; }
.zseg-detection-banner { border: 1px solid #b6d4fe; background: #cfe2ff; border-radius: 8px; padding: 16px; margin-bottom: 20px; }
.zseg-detection-header { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; color: #0a58ca; }
.zseg-detection-count { background: #0d6efd; color: white; border-radius: 10px; padding: 2px 8px; font-size: 12px; }
.zseg-detect-cards { display: flex; flex-wrap: wrap; gap: 12px; }
.zseg-detect-card { background: white; border: 1px solid #dee2e6; border-radius: 6px; padding: 12px; min-width: 220px; max-width: 280px; }
.zseg-detect-card.has-oob { border-color: #0d6efd; }
.zseg-detect-card-header { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.zseg-detect-desc { color: #6c757d; font-size: 12px; margin-bottom: 10px; }
.zseg-detect-actions { display: flex; gap: 6px; flex-wrap: wrap; }
.zseg-template-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; max-height: 400px; overflow-y: auto; }
.zseg-template-card { border: 1px solid #dee2e6; border-radius: 6px; padding: 12px; }
.zseg-template-card-header { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-bottom: 8px; }
</style>`;
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Wizard integration hook
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Attaches Z-segment detection to the wizard's sample message step.
 *
 * Call this after the wizard's sample message textarea is in the DOM.
 * The wizard passes its interfaceId and sampleMessage; this function:
 *   1. Calls /api/zsegments/detect with the sample
 *   2. If Z-segments found, renders the detection banner
 *   3. User can apply templates or configure manually inline
 *
 * @param {Object} opts
 * @param {string} opts.interfaceId
 * @param {string} opts.sampleMessage    - raw HL7 from wizard
 * @param {string} opts.bannerContainerId - element to render banner into
 */
async function attachZSegmentWizardHook(opts = {}) {
    const { interfaceId, sampleMessage, bannerContainerId } = opts;
    const bannerContainer = document.getElementById(bannerContainerId);
    if (!bannerContainer) return;

    if (!sampleMessage || !sampleMessage.includes('|')) {
        bannerContainer.innerHTML = '';
        return;
    }

    const builder = new ZSegmentConfigBuilder({ interfaceId });
    await builder.init();

    try {
        const detection = await builder.detectFromMessage(sampleMessage);
        if (detection.detectedSegments && detection.detectedSegments.length > 0) {
            builder.renderDetectionBanner(detection, bannerContainer);
        } else {
            bannerContainer.innerHTML = `
                <div class="alert alert-light border small py-2">
                    No Z-segments detected in sample message.
                    <button class="btn btn-xs btn-outline-secondary ml-2" id="zseg-manual-add-btn">+ Add Z-Segment Manually</button>
                </div>`;
            document.getElementById('zseg-manual-add-btn')?.addEventListener('click', () => {
                bannerContainer.innerHTML = `<div id="${bannerContainerId}-full"></div>`;
                const fullBuilder = new ZSegmentConfigBuilder({
                    interfaceId,
                    containerId: `${bannerContainerId}-full`
                });
                fullBuilder.init().then(() => fullBuilder.render());
            });
        }
    } catch (err) {
        console.warn('Z-segment detection failed:', err);
    }

    return builder;
}

// Export for use in wizard and interface settings
if (typeof module !== 'undefined') {
    module.exports = { ZSegmentConfigBuilder, attachZSegmentWizardHook };
}
