/**
 * ZSegmentConfigPanel.js
 *
 * Renders an interactive Z-segment field-mapping configuration panel inside
 * wizard Step 3 (FHIR Mapping).  One card is rendered per detected Z-segment.
 *
 * Design:
 *   - Strategy Pattern: OobStrategy / CustomStrategy handle OOB vs unknown segments
 *   - Each segment card is self-contained and manages its own draft state
 *   - getConfig() returns a ready-to-POST CreateZSegmentConfigRequest[] the
 *     controller uses instead of the old blind apply-template call
 *
 * Segment card states:
 *   OOB  → "Accept OOB default" | "Customize" toggle
 *   Custom → field editor pre-populated from parsedFields (actual message values)
 *
 * No external dependencies beyond the DOM + window.wizardController.
 */

'use strict';

// ─── FHIR resource options shared across cards ───────────────────────────────
const FHIR_RESOURCE_OPTIONS = [
    'Organization', 'Practitioner', 'Patient', 'Location',
    'RelatedPerson', 'Device', 'Medication', 'Substance',
    'PractitionerRole', 'HealthcareService', 'Coverage',
    'ChargeItemDefinition', 'Basic',
];

// ─── Transform options (aligned with zsegment_transforms DB table / V96) ─────
const TRANSFORM_OPTIONS = [
    { code: 'identity',   label: 'Direct (no change)' },
    { code: 'composite',  label: 'Composite (split components)' },
    { code: 'firstOf',    label: 'First of repeat' },
    { code: 'forEach',    label: 'For each repeat' },
    { code: 'hl7datetime',label: 'HL7 date/time → ISO-8601' },
    { code: 'codeValue',  label: 'Code value (CWE.1)' },
    { code: 'codeDisplay',label: 'Code display (CWE.2)' },
    { code: 'join',       label: 'Join components' },
    { code: 'boolean',    label: 'Boolean (Y/N → true/false)' },
];

// ─── Resource role options ────────────────────────────────────────────────────
const RESOURCE_ROLE_OPTIONS = [
    { code: 'primary',   label: 'Primary — replaces default assembly resource' },
    { code: 'related',   label: 'Related — added alongside primary as a linked resource' },
    { code: 'extension', label: 'Extension — merges fields into an existing resource' },
];

// ─────────────────────────────────────────────────────────────────────────────
// ZSegmentCardStrategy — abstract base
// ─────────────────────────────────────────────────────────────────────────────

class ZSegmentCardStrategy {
    /**
     * @param {object} segData  — one entry from detectedZSegmentData
     * @param {string} panelId  — unique DOM id prefix for this card
     */
    constructor(segData, panelId) {
        this.segData = segData;
        this.panelId = panelId;
        // Draft state — mutated by user interactions, read by getConfig()
        this._draft = this._buildInitialDraft();
    }

    /** @returns {string} HTML for the card body */
    renderBody() { throw new Error('ZSegmentCardStrategy.renderBody() must be overridden'); }

    /** @returns {CreateZSegmentConfigRequest|null} null = skip (accept OOB as-is) */
    getConfig() { throw new Error('ZSegmentCardStrategy.getConfig() must be overridden'); }

    /** Called after the card HTML is inserted into the DOM */
    attachEvents() {}

    // ── shared helpers ────────────────────────────────────────────────────────

    _buildInitialDraft() {
        const seg = this.segData;
        const oob = (seg.suggestedTemplates || []).find(t => t.isSystem);
        return {
            segmentId:          seg.segmentId,
            targetFhirResource: oob ? oob.targetFhirResource : '',
            resourceRole:       oob ? (oob.resourceRole || 'primary') : 'primary',
            description:        oob ? oob.description : '',
            fieldMappings:      this._initialFieldMappings(),
        };
    }

    _initialFieldMappings() {
        const seg   = this.segData;
        const oob   = (seg.suggestedTemplates || []).find(t => t.isSystem);
        const pf    = (seg.parsedFields || [])[0] || [];  // first occurrence

        if (oob && oob.fields && oob.fields.length > 0) {
            // Merge OOB template fields with parsed values from the sample message
            return oob.fields.map(f => ({
                position:     f.position,
                fieldLabel:   f.fieldLabel || f.position,
                hl7DataType:  f.hl7DataType || 'ST',
                fhirPath:     f.fhirPath || '',
                transform:    f.transform || 'identity',
                isRequired:   f.isRequired || false,
                defaultValue: f.defaultValue || '',
                rawValue:     this._rawValueForPosition(pf, f.position),
            }));
        }

        // No OOB template — seed rows from parsed fields only
        return pf.map(pf => ({
            position:     pf.position,
            fieldLabel:   pf.position,
            hl7DataType:  'ST',
            fhirPath:     '',
            transform:    'identity',
            isRequired:   false,
            defaultValue: '',
            rawValue:     pf.rawValue,
        }));
    }

    _rawValueForPosition(parsedFields, position) {
        const f = parsedFields.find(p => p.position === position);
        return f ? f.rawValue : '';
    }

    _resourceDropdown(selectedId, id) {
        const opts = FHIR_RESOURCE_OPTIONS.map(r =>
            `<option value="${r}" ${r === selectedId ? 'selected' : ''}>${r}</option>`
        ).join('');
        return `<select id="${id}" style="padding:4px 8px;border:1px solid #d1d5db;border-radius:4px;font-size:12px;width:100%;">${opts}</select>`;
    }

    _transformDropdown(selected, id) {
        const opts = TRANSFORM_OPTIONS.map(t =>
            `<option value="${t.code}" ${t.code === selected ? 'selected' : ''}>${t.label}</option>`
        ).join('');
        return `<select id="${id}" style="padding:3px 6px;border:1px solid #d1d5db;border-radius:4px;font-size:11px;">${opts}</select>`;
    }

    _roleDropdown(selected, id) {
        const opts = RESOURCE_ROLE_OPTIONS.map(r =>
            `<option value="${r.code}" ${r.code === selected ? 'selected' : ''}>${r.label}</option>`
        ).join('');
        return `<select id="${id}" style="padding:4px 8px;border:1px solid #d1d5db;border-radius:4px;font-size:12px;width:100%;">${opts}</select>`;
    }

    /** Render the field mapping table (shared by both strategies) */
    _renderFieldTable(readonly) {
        const rows = this._draft.fieldMappings.map((fm, i) => {
            const posId     = `${this.panelId}-pos-${i}`;
            const labelId   = `${this.panelId}-label-${i}`;
            const fhirId    = `${this.panelId}-fhir-${i}`;
            const transformId = `${this.panelId}-tr-${i}`;
            const reqId     = `${this.panelId}-req-${i}`;
            const esc       = s => String(s || '').replace(/"/g, '&quot;').replace(/</g, '&lt;');
            const rawBadge  = fm.rawValue
                ? `<span style="font-size:10px;color:#0369a1;background:#e0f2fe;padding:1px 5px;border-radius:8px;font-family:monospace;max-width:120px;overflow:hidden;text-overflow:ellipsis;display:inline-block;white-space:nowrap;" title="${esc(fm.rawValue)}">${esc(fm.rawValue.substring(0, 20))}${fm.rawValue.length > 20 ? '…' : ''}</span>`
                : '<span style="font-size:10px;color:#9ca3af;">—</span>';

            return `
            <tr data-row="${i}" style="border-bottom:1px solid #f1f5f9;">
                <td style="padding:6px 8px;font-family:monospace;font-size:12px;font-weight:600;color:#1d4ed8;white-space:nowrap;">
                    ${esc(fm.position)}</td>
                <td style="padding:6px 8px;">${rawBadge}</td>
                <td style="padding:6px 8px;">
                    ${readonly
                        ? `<span style="font-size:12px;color:#374151;">${esc(fm.fieldLabel)}</span>`
                        : `<input id="${labelId}" type="text" value="${esc(fm.fieldLabel)}" style="width:100%;padding:3px 6px;border:1px solid #d1d5db;border-radius:4px;font-size:12px;">`
                    }
                </td>
                <td style="padding:6px 8px;">
                    ${readonly
                        ? `<span style="font-size:12px;font-family:monospace;color:#059669;">${esc(fm.fhirPath)}</span>`
                        : `<input id="${fhirId}" type="text" value="${esc(fm.fhirPath)}" placeholder="e.g. Organization.name" style="width:100%;padding:3px 6px;border:1px solid #d1d5db;border-radius:4px;font-size:12px;font-family:monospace;">`
                    }
                </td>
                <td style="padding:6px 8px;">
                    ${readonly
                        ? `<span style="font-size:11px;background:#f3f4f6;padding:2px 6px;border-radius:8px;">${esc(fm.transform)}</span>`
                        : this._transformDropdown(fm.transform, transformId)
                    }
                </td>
                <td style="padding:6px 8px;text-align:center;">
                    <input type="checkbox" id="${reqId}" ${fm.isRequired ? 'checked' : ''} ${readonly ? 'disabled' : ''}>
                </td>
                ${readonly ? '' : `<td style="padding:6px 8px;text-align:center;">
                    <button data-delete-row="${i}" style="background:#fef2f2;color:#dc2626;border:1px solid #fecaca;border-radius:4px;padding:2px 6px;font-size:11px;cursor:pointer;">✕</button>
                </td>`}
            </tr>`;
        }).join('');

        const addRowBtn = readonly ? '' : `
            <tr><td colspan="7" style="padding:6px 8px;">
                <button id="${this.panelId}-add-row" style="font-size:11px;background:#f0fdf4;color:#15803d;border:1px solid #bbf7d0;border-radius:4px;padding:4px 10px;cursor:pointer;">+ Add field mapping</button>
            </td></tr>`;

        return `
        <table style="width:100%;border-collapse:collapse;font-size:12px;">
            <thead><tr style="background:#f8fafc;border-bottom:2px solid #e2e8f0;">
                <th style="padding:6px 8px;text-align:left;color:#374151;font-weight:600;">Position</th>
                <th style="padding:6px 8px;text-align:left;color:#374151;font-weight:600;">Value in message</th>
                <th style="padding:6px 8px;text-align:left;color:#374151;font-weight:600;">Field label</th>
                <th style="padding:6px 8px;text-align:left;color:#374151;font-weight:600;">FHIR path</th>
                <th style="padding:6px 8px;text-align:left;color:#374151;font-weight:600;">Transform</th>
                <th style="padding:6px 8px;text-align:center;color:#374151;font-weight:600;">Required</th>
                ${readonly ? '' : '<th style="padding:6px 8px;"></th>'}
            </tr></thead>
            <tbody id="${this.panelId}-tbody">${rows}${addRowBtn}</tbody>
        </table>`;
    }

    /** Sync draft.fieldMappings from DOM (called before getConfig) */
    _syncFieldsFromDOM() {
        this._draft.fieldMappings = this._draft.fieldMappings.map((fm, i) => {
            const get = id => (document.getElementById(id) || {}).value;
            const getChecked = id => (document.getElementById(id) || {}).checked;
            return {
                ...fm,
                fieldLabel:  get(`${this.panelId}-label-${i}`)   || fm.fieldLabel,
                fhirPath:    get(`${this.panelId}-fhir-${i}`)    || fm.fhirPath,
                transform:   get(`${this.panelId}-tr-${i}`)      || fm.transform,
                isRequired:  getChecked(`${this.panelId}-req-${i}`),
            };
        }).filter(fm => fm.fhirPath.trim() !== '');  // drop blank FHIR paths
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// OobStrategy — segment has a system template
// ─────────────────────────────────────────────────────────────────────────────

class OobStrategy extends ZSegmentCardStrategy {
    constructor(segData, panelId) {
        super(segData, panelId);
        this._mode = 'accept';  // 'accept' | 'customize'
    }

    renderBody() {
        const oob = (this.segData.suggestedTemplates || []).find(t => t.isSystem);
        const esc = s => String(s || '').replace(/</g, '&lt;');
        return `
        <!-- Mode toggle -->
        <div style="display:flex;gap:8px;margin-bottom:14px;">
            <label style="display:flex;align-items:center;gap:6px;cursor:pointer;padding:6px 12px;border:2px solid #3b82f6;border-radius:6px;background:#eff6ff;font-size:12px;font-weight:600;color:#1d4ed8;">
                <input type="radio" name="${this.panelId}-mode" value="accept" checked>
                ✅ Use OOB default — <em style="font-weight:400;">${esc(oob.templateName)}</em>
            </label>
            <label style="display:flex;align-items:center;gap:6px;cursor:pointer;padding:6px 12px;border:2px solid #d1d5db;border-radius:6px;background:#fafafa;font-size:12px;color:#374151;">
                <input type="radio" name="${this.panelId}-mode" value="customize">
                ✏️ Customize field mappings
            </label>
        </div>

        <!-- Resource header (shown in both modes) -->
        <div style="display:flex;gap:16px;margin-bottom:12px;align-items:center;flex-wrap:wrap;">
            <div style="flex:1;min-width:160px;">
                <label style="font-size:11px;font-weight:600;color:#6b7280;display:block;margin-bottom:3px;">FHIR Resource</label>
                ${this._resourceDropdown(this._draft.targetFhirResource, `${this.panelId}-resource`)}
            </div>
            <div style="flex:2;min-width:200px;">
                <label style="font-size:11px;font-weight:600;color:#6b7280;display:block;margin-bottom:3px;">Resource Role</label>
                ${this._roleDropdown(this._draft.resourceRole, `${this.panelId}-role`)}
            </div>
        </div>

        <!-- Field table -->
        <div id="${this.panelId}-field-table">
            ${this._renderFieldTable(/* readonly= */ true)}
        </div>`;
    }

    attachEvents() {
        const radios = document.querySelectorAll(`input[name="${this.panelId}-mode"]`);
        radios.forEach(r => r.addEventListener('change', () => {
            this._mode = r.value;
            const tableDiv = document.getElementById(`${this.panelId}-field-table`);
            if (tableDiv) {
                tableDiv.innerHTML = this._renderFieldTable(this._mode === 'accept');
                this._attachFieldTableEvents();
            }
        }));

        const resourceSel = document.getElementById(`${this.panelId}-resource`);
        if (resourceSel) resourceSel.addEventListener('change', e => {
            this._draft.targetFhirResource = e.target.value;
        });
        const roleSel = document.getElementById(`${this.panelId}-role`);
        if (roleSel) roleSel.addEventListener('change', e => {
            this._draft.resourceRole = e.target.value;
        });

        this._attachFieldTableEvents();
    }

    _attachFieldTableEvents() {
        // Delete row buttons
        document.querySelectorAll(`[data-delete-row]`).forEach(btn => {
            const row = parseInt(btn.getAttribute('data-delete-row'), 10);
            btn.addEventListener('click', () => {
                this._draft.fieldMappings.splice(row, 1);
                const tableDiv = document.getElementById(`${this.panelId}-field-table`);
                if (tableDiv) {
                    tableDiv.innerHTML = this._renderFieldTable(false);
                    this._attachFieldTableEvents();
                }
            });
        });

        // Add row button
        const addBtn = document.getElementById(`${this.panelId}-add-row`);
        if (addBtn) addBtn.addEventListener('click', () => {
            this._draft.fieldMappings.push({
                position: `${this.segData.segmentId}.${this._draft.fieldMappings.length + 1}`,
                fieldLabel: '',
                hl7DataType: 'ST',
                fhirPath: '',
                transform: 'identity',
                isRequired: false,
                defaultValue: '',
                rawValue: '',
            });
            const tableDiv = document.getElementById(`${this.panelId}-field-table`);
            if (tableDiv) {
                tableDiv.innerHTML = this._renderFieldTable(false);
                this._attachFieldTableEvents();
            }
        });
    }

    getConfig() {
        if (this._mode === 'accept') {
            // Null = caller uses apply-template (existing path, no full config needed)
            return null;
        }
        this._syncFieldsFromDOM();
        const resourceEl = document.getElementById(`${this.panelId}-resource`);
        const roleEl     = document.getElementById(`${this.panelId}-role`);
        if (resourceEl) this._draft.targetFhirResource = resourceEl.value;
        if (roleEl)     this._draft.resourceRole       = roleEl.value;

        return {
            segmentId:          this._draft.segmentId,
            targetFhirResource: this._draft.targetFhirResource,
            resourceRole:       this._draft.resourceRole,
            description:        this._draft.description,
            fieldMappings:      this._draft.fieldMappings.map(fm => ({
                position:     fm.position,
                fieldLabel:   fm.fieldLabel,
                hl7DataType:  fm.hl7DataType,
                fhirPath:     fm.fhirPath,
                transform:    fm.transform,
                isRequired:   fm.isRequired,
                defaultValue: fm.defaultValue || '',
            })),
        };
    }

    /** The template id to use when mode === 'accept' */
    getOobTemplateId() {
        const oob = (this.segData.suggestedTemplates || []).find(t => t.isSystem);
        return oob ? oob.id : null;
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// CustomStrategy — no OOB template, user defines from scratch
// ─────────────────────────────────────────────────────────────────────────────

class CustomStrategy extends ZSegmentCardStrategy {
    renderBody() {
        const esc = s => String(s || '').replace(/</g, '&lt;');
        const hasFields = this._draft.fieldMappings.length > 0;
        return `
        <div style="background:#fffbeb;border:1px solid #fcd34d;border-radius:6px;padding:8px 12px;margin-bottom:12px;font-size:12px;color:#92400e;">
            ⚠️ No OOB template found for <strong>${esc(this.segData.segmentId)}</strong>.
            ${hasFields
                ? `${this._draft.fieldMappings.length} field${this._draft.fieldMappings.length > 1 ? 's' : ''} detected from your sample message.`
                : 'No fields detected — add them manually below.'}
            Define the FHIR mapping for each field.
        </div>

        <div style="display:flex;gap:16px;margin-bottom:12px;align-items:center;flex-wrap:wrap;">
            <div style="flex:1;min-width:160px;">
                <label style="font-size:11px;font-weight:600;color:#6b7280;display:block;margin-bottom:3px;">Map to FHIR Resource</label>
                ${this._resourceDropdown(this._draft.targetFhirResource, `${this.panelId}-resource`)}
            </div>
            <div style="flex:2;min-width:200px;">
                <label style="font-size:11px;font-weight:600;color:#6b7280;display:block;margin-bottom:3px;">Resource Role</label>
                ${this._roleDropdown(this._draft.resourceRole, `${this.panelId}-role`)}
            </div>
        </div>

        <div id="${this.panelId}-field-table">
            ${this._renderFieldTable(/* readonly= */ false)}
        </div>`;
    }

    attachEvents() {
        const resourceSel = document.getElementById(`${this.panelId}-resource`);
        if (resourceSel) resourceSel.addEventListener('change', e => {
            this._draft.targetFhirResource = e.target.value;
        });
        const roleSel = document.getElementById(`${this.panelId}-role`);
        if (roleSel) roleSel.addEventListener('change', e => {
            this._draft.resourceRole = e.target.value;
        });
        this._attachFieldTableEvents();
    }

    _attachFieldTableEvents() {
        document.querySelectorAll(`[data-delete-row]`).forEach(btn => {
            const row = parseInt(btn.getAttribute('data-delete-row'), 10);
            btn.addEventListener('click', () => {
                this._draft.fieldMappings.splice(row, 1);
                const tableDiv = document.getElementById(`${this.panelId}-field-table`);
                if (tableDiv) {
                    tableDiv.innerHTML = this._renderFieldTable(false);
                    this._attachFieldTableEvents();
                }
            });
        });

        const addBtn = document.getElementById(`${this.panelId}-add-row`);
        if (addBtn) addBtn.addEventListener('click', () => {
            this._draft.fieldMappings.push({
                position: `${this.segData.segmentId}.${this._draft.fieldMappings.length + 1}`,
                fieldLabel: '',
                hl7DataType: 'ST',
                fhirPath: '',
                transform: 'identity',
                isRequired: false,
                defaultValue: '',
                rawValue: '',
            });
            const tableDiv = document.getElementById(`${this.panelId}-field-table`);
            if (tableDiv) {
                tableDiv.innerHTML = this._renderFieldTable(false);
                this._attachFieldTableEvents();
            }
        });
    }

    getConfig() {
        this._syncFieldsFromDOM();
        const resourceEl = document.getElementById(`${this.panelId}-resource`);
        const roleEl     = document.getElementById(`${this.panelId}-role`);
        if (resourceEl) this._draft.targetFhirResource = resourceEl.value;
        if (roleEl)     this._draft.resourceRole       = roleEl.value;

        if (!this._draft.targetFhirResource) return null;  // not configured

        return {
            segmentId:          this._draft.segmentId,
            targetFhirResource: this._draft.targetFhirResource,
            resourceRole:       this._draft.resourceRole,
            description:        this._draft.description,
            fieldMappings:      this._draft.fieldMappings.map(fm => ({
                position:     fm.position,
                fieldLabel:   fm.fieldLabel,
                hl7DataType:  fm.hl7DataType,
                fhirPath:     fm.fhirPath,
                transform:    fm.transform,
                isRequired:   fm.isRequired,
                defaultValue: fm.defaultValue || '',
            })),
        };
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// ZSegmentConfigPanel — top-level component
// ─────────────────────────────────────────────────────────────────────────────

class ZSegmentConfigPanel {
    /**
     * @param {Array}  detectedZSegmentData  — from model.data.detectedZSegmentData
     */
    constructor(detectedZSegmentData) {
        this._segmentData = detectedZSegmentData || [];
        this._strategies  = [];  // ZSegmentCardStrategy[], one per segment
    }

    /** Returns true when there are Z-segments to configure */
    hasSegments() {
        return this._segmentData.length > 0;
    }

    /**
     * Render the full panel HTML.
     * Must be inserted into the DOM before calling attachEvents().
     */
    render() {
        if (!this.hasSegments()) return '';

        const cards = this._segmentData.map((seg, i) => {
            const panelId  = `zseg-card-${i}-${seg.segmentId}`;
            const strategy = seg.hasOobTemplate
                ? new OobStrategy(seg, panelId)
                : new CustomStrategy(seg, panelId);
            this._strategies.push(strategy);

            const badgeStyle = seg.hasOobTemplate
                ? 'background:#166534;color:#fff;'
                : 'background:#92400e;color:#fff;';
            const badgeLabel = seg.hasOobTemplate ? 'OOB Template' : 'Custom';

            return `
            <div id="${panelId}-card" style="border:1px solid #e2e8f0;border-radius:10px;overflow:hidden;margin-bottom:12px;">
                <!-- Card header -->
                <div style="display:flex;align-items:center;justify-content:space-between;padding:10px 16px;background:#f8fafc;border-bottom:1px solid #e2e8f0;">
                    <div style="display:flex;align-items:center;gap:10px;">
                        <span style="font-family:monospace;font-size:15px;font-weight:700;color:#1e3a5f;">${seg.segmentId}</span>
                        <span style="font-size:11px;font-weight:600;padding:2px 8px;border-radius:9px;${badgeStyle}">${badgeLabel}</span>
                        ${seg.parsedFields && seg.parsedFields[0]
                            ? `<span style="font-size:11px;color:#6b7280;">${seg.parsedFields[0].length} field${seg.parsedFields[0].length !== 1 ? 's' : ''} in sample message</span>`
                            : ''
                        }
                    </div>
                    <button id="${panelId}-toggle"
                            style="font-size:11px;background:transparent;border:none;cursor:pointer;color:#6b7280;padding:4px 8px;">
                        ▼ Configure
                    </button>
                </div>
                <!-- Card body (collapsible) -->
                <div id="${panelId}-body" style="padding:16px;display:block;">
                    ${strategy.renderBody()}
                </div>
            </div>`;
        }).join('');

        return `
        <div id="zseg-config-panel" style="margin-top:24px;">
            <div style="display:flex;align-items:center;gap:10px;margin-bottom:14px;">
                <span style="font-size:18px;">🔧</span>
                <div>
                    <div style="font-size:15px;font-weight:700;color:#1e3a5f;">
                        Z-Segment Configuration
                    </div>
                    <div style="font-size:12px;color:#6b7280;">
                        ${this._segmentData.length} custom segment${this._segmentData.length !== 1 ? 's' : ''} detected in your message.
                        Configure how each maps to a FHIR resource.
                    </div>
                </div>
            </div>
            ${cards}
        </div>`;
    }

    /** Wire up events — call after render() HTML has been inserted into DOM */
    attachEvents() {
        this._strategies.forEach((strategy, i) => {
            strategy.attachEvents();

            // Collapse / expand toggle
            const panelId = strategy.panelId;
            const toggleBtn = document.getElementById(`${panelId}-toggle`);
            const bodyDiv   = document.getElementById(`${panelId}-body`);
            if (toggleBtn && bodyDiv) {
                toggleBtn.addEventListener('click', () => {
                    const collapsed = bodyDiv.style.display === 'none';
                    bodyDiv.style.display = collapsed ? 'block' : 'none';
                    toggleBtn.textContent = collapsed ? '▼ Configure' : '▶ Configure';
                });
            }
        });
    }

    /**
     * Returns the user's Z-segment configurations.
     * Each entry is either:
     *   { mode: 'apply-template', segmentId, templateId }  — OOB accepted as-is
     *   { mode: 'create-config',  segmentId, config }      — custom or customized OOB
     *   { mode: 'skip',           segmentId }              — unconfigured custom segment
     */
    getConfigs() {
        return this._strategies.map(strategy => {
            const seg = strategy.segData;

            if (strategy instanceof OobStrategy && strategy._mode === 'accept') {
                return {
                    mode:       'apply-template',
                    segmentId:  seg.segmentId,
                    templateId: strategy.getOobTemplateId(),
                };
            }

            const config = strategy.getConfig();
            if (!config) {
                return { mode: 'skip', segmentId: seg.segmentId };
            }
            return {
                mode:      'create-config',
                segmentId: seg.segmentId,
                config,
            };
        });
    }
}

// Export to window for wizard access
window.ZSegmentConfigPanel = ZSegmentConfigPanel;
