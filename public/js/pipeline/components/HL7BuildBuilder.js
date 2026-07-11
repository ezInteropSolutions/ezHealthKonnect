/**
 * HL7BuildBuilder — step config builder for the "hl7.build" pipeline step.
 *
 * The no-code, format-agnostic on-ramp for a complete HL7 v2 message: lets a
 * user map CSV columns, DB query columns, or arbitrary JSON fields directly
 * onto HL7 segment/field/component paths — see
 * services/executors/transform/hl7_build_executor.go. The HL7-side mirror of
 * FHIRBuildBuilder.js/MapToCanonicalBuilder.js, adapted for HL7's
 * segment-ordered, MSH-is-special shape: MSH is always auto-populated, every
 * other segment is configured explicitly with "single" or "repeating"
 * cardinality.
 *
 * Catalogs (message types, segment names, field/component keys) are all
 * fetched live from the backend (hl7/builder/field_catalog.go, exposed via
 * /api/hl7/*), never hardcoded here — same discipline
 * MapToCanonicalBuilder.js/FHIRBuildBuilder.js follow.
 *
 * Builder contract (StepBuilderRegistry):
 *   render(step)         → string  HTML for the properties panel form tab
 *   collectConfig(step)  → void    reads DOM, writes into step.config
 *   destroy()            → void    tears down event listeners / AC refs
 */

class HL7BuildBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
        this._step = null;
        this._messageTypes = [];       // [{messageType, triggerEvent}]
        this._segmentNames = [];       // ["MSH","PID","PV1",...] for current messageType/triggerEvent/version
        this._fieldCatalogs = {};      // segmentName -> [{key, label, dataType}]
        this._fieldSearches = [];      // active FieldPathSearchComponent instances
        this._suggestPanelOpenIndex = -1;
        this._suggestSampleRowsText = '';
        this._suggestError = '';
        this._suggestBusy = false;

        window._hl7BuildBuilder = this;
    }

    // ── render ────────────────────────────────────────────────────────────────

    render(step) {
        this._step = step;
        if (!step.config) step.config = {};
        this._applyDefaultConfig(step.config);

        this._loadMessageTypes();
        this._loadSegmentNames();

        const html = this._renderAll();
        setTimeout(() => this._attachFieldSearches(), 0);
        return html;
    }

    _applyDefaultConfig(cfg) {
        if (!cfg.messageType) cfg.messageType = 'ADT';
        if (!cfg.triggerEvent) cfg.triggerEvent = 'A01';
        if (!cfg.version) cfg.version = '2.5.1';
        if (!cfg.outputField) cfg.outputField = 'hl7Message';
        if (!Array.isArray(cfg.segments)) cfg.segments = [];
    }

    _renderAll() {
        const cfg = this._step.config;
        const esc = HL7BuildBuilder._esc;

        const messageTypeOptions = this._messageTypes
            .map(mt => `${esc(mt.messageType)}^${esc(mt.triggerEvent)}`)
            .filter((v, i, arr) => arr.indexOf(v) === i)
            .map(v => `<option value="${v}" ${v === `${esc(cfg.messageType)}^${esc(cfg.triggerEvent)}` ? 'selected' : ''}>${v}</option>`)
            .join('');

        return `
        <div id="hl7BuildBuilder" class="cda-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>HL7 v2 Message Builder step.</strong> Maps CSV/DB/JSON fields directly onto HL7 segment/field paths.
                MSH is auto-populated; add segments below in message order.
            </div>

            <div class="config-group" style="margin-bottom:1.1rem;display:flex;gap:0.6rem;flex-wrap:wrap;">
                <div style="flex:1 1 200px;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Message Type</label>
                    <select id="hbbMessageType" class="form-select form-select-sm">
                        ${messageTypeOptions || `<option value="${esc(cfg.messageType)}^${esc(cfg.triggerEvent)}">${esc(cfg.messageType)}^${esc(cfg.triggerEvent)}</option>`}
                    </select>
                </div>
                <div style="flex:0 0 100px;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Version</label>
                    <input id="hbbVersion" type="text" class="form-control form-control-sm" value="${esc(cfg.version)}">
                </div>
                <div style="flex:1 1 200px;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                    <input id="hbbOutputField" type="text" class="form-control form-control-sm"
                        value="${esc(cfg.outputField)}" placeholder="hl7Message"
                        style="font-family:monospace;font-size:0.82rem;">
                </div>
            </div>

            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.5rem;">MSH Overrides (optional)</label>
                <div style="display:flex;gap:0.5rem;flex-wrap:wrap;">
                    <input id="hbbSendingApp" type="text" class="form-control form-control-sm" placeholder="Sending App (default: ezHealthKonnect)"
                        value="${esc(cfg.sendingApplication)}" style="flex:1 1 180px;">
                    <input id="hbbSendingFacility" type="text" class="form-control form-control-sm" placeholder="Sending Facility (default: EHK)"
                        value="${esc(cfg.sendingFacility)}" style="flex:1 1 180px;">
                    <input id="hbbReceivingApp" type="text" class="form-control form-control-sm" placeholder="Receiving App"
                        value="${esc(cfg.receivingApplication)}" style="flex:1 1 180px;">
                    <input id="hbbReceivingFacility" type="text" class="form-control form-control-sm" placeholder="Receiving Facility"
                        value="${esc(cfg.receivingFacility)}" style="flex:1 1 180px;">
                </div>
            </div>

            <div class="config-group">
                <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.5rem;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;margin:0;">Segments</label>
                    <button type="button" class="btn btn-sm btn-primary"
                        onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.addSegment()">
                        <i class="fas fa-plus"></i> Add Segment
                    </button>
                </div>
                <div style="font-size:0.7rem;color:#94a3b8;margin-bottom:0.5rem;">Segments render in the order listed here, after the auto-generated MSH.</div>
                <div id="hbbSegmentsContainer">
                    ${cfg.segments.length === 0
                        ? `<div style="text-align:center;padding:1.5rem;color:#94a3b8;font-size:0.8rem;border:1px dashed #cbd5e1;border-radius:6px;">No segments configured yet. Click "Add Segment" to map fields into a segment.</div>`
                        : cfg.segments.map((seg, i) => this._renderSegmentCard(seg, i)).join('')}
                </div>
            </div>
        </div>`;
    }

    // ── Segments ──────────────────────────────────────────────────────────────

    _renderSegmentCard(seg, segIndex) {
        const esc = HL7BuildBuilder._esc;
        const segmentOptions = (this._segmentNames.length ? this._segmentNames : [seg.segment])
            .filter(Boolean)
            .map(name => `<option value="${esc(name)}" ${name === seg.segment ? 'selected' : ''}>${esc(name)}</option>`)
            .join('');
        const fieldCatalog = this._fieldCatalogs[seg.segment] || [];
        const fieldDatalistId = `hbbFieldTargets-${segIndex}`;
        const isRepeating = seg.cardinality === 'repeating';

        const fieldsHtml = (seg.fields || []).map((f, fi) => `
            <tr data-field-index="${fi}">
                <td>
                    <input type="text" class="form-control form-control-sm hbb-field-key"
                        value="${esc(f.fieldKey)}" list="${fieldDatalistId}" placeholder="e.g. PID.5.1" autocomplete="off">
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm hbb-field-source"
                        value="${esc(f.sourcePath)}" placeholder="e.g. lastName" autocomplete="off">
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm hbb-field-fallback"
                        value="${esc((f.fallbackPaths || []).join(', '))}" placeholder="alt1, alt2" autocomplete="off"
                        title="Comma-separated fallback source paths, tried in order if Source Path is empty">
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm hbb-field-literal"
                        value="${esc(f.literalValue)}" placeholder="(fixed value)" autocomplete="off"
                        title="Used only when no source/fallback path resolves">
                </td>
                <td>
                    <input type="text" class="form-control form-control-sm hbb-field-valuemap"
                        value="${esc(HL7BuildBuilder._valueMapToText(f.valueMap))}" placeholder="A=active, R=resolved" autocomplete="off"
                        title="Comma-separated raw=translated value mappings">
                </td>
                <td style="width:40px;text-align:center;">
                    <button type="button" class="btn btn-sm btn-danger"
                        onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.removeSegmentField(${segIndex}, ${fi})"
                        title="Remove field"><i class="fas fa-trash"></i></button>
                </td>
            </tr>`).join('');

        return `
        <div class="hbb-segment-card" data-segment-index="${segIndex}"
            style="border:1px solid #e2e8f0;border-radius:8px;padding:0.7rem 0.8rem;margin-bottom:0.8rem;background:#fafbfc;">
            <div style="display:flex;align-items:center;gap:0.6rem;margin-bottom:0.55rem;flex-wrap:wrap;">
                <select class="form-select form-select-sm hbb-segment-name" style="flex:0 0 140px;"
                    onchange="window._hl7BuildBuilder && window._hl7BuildBuilder.onSegmentNameChange(${segIndex}, this.value)">
                    <option value="">— select segment —</option>
                    ${segmentOptions}
                </select>
                <select class="form-select form-select-sm hbb-segment-cardinality" style="flex:0 0 140px;"
                    onchange="window._hl7BuildBuilder && window._hl7BuildBuilder.onCardinalityChange(${segIndex}, this.value)">
                    <option value="single" ${!isRepeating ? 'selected' : ''}>Single</option>
                    <option value="repeating" ${isRepeating ? 'selected' : ''}>Repeating</option>
                </select>
                ${isRepeating ? `
                <input type="text" class="form-control form-control-sm hbb-segment-rowspath" data-search-kind="rowspath" data-segment-index="${segIndex}"
                    value="${esc(seg.rowsPath)}" placeholder="e.g. labResults"
                    style="flex:1;font-family:monospace;font-size:0.8rem;">` : ''}
                <button type="button" class="btn btn-sm btn-outline-danger"
                    onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.removeSegment(${segIndex})"
                    title="Remove segment"><i class="fas fa-trash"></i></button>
            </div>
            ${isRepeating ? `<div style="font-size:0.7rem;color:#94a3b8;margin-bottom:0.5rem;">Rows Path: pipeline field holding the array of source rows — one ${esc(seg.segment || 'segment')} instance is built per row.</div>` : ''}
            <datalist id="${fieldDatalistId}">
                ${fieldCatalog.map(f => `<option value="${esc(f.key)}">${esc(f.label || f.key)}</option>`).join('')}
            </datalist>
            ${this._renderSuggestPanel(segIndex)}
            ${(seg.fields || []).length === 0
                ? `<div style="font-size:0.74rem;color:#94a3b8;margin-bottom:0.4rem;">No fields mapped yet.</div>`
                : `<div style="overflow-x:auto;">
                    <table class="mapping-table" style="width:100%;min-width:700px;">
                        <thead><tr>
                            <th style="font-size:0.68rem;color:#64748b;">Field Key</th>
                            <th style="font-size:0.68rem;color:#64748b;">Source Path</th>
                            <th style="font-size:0.68rem;color:#64748b;">Fallback Paths</th>
                            <th style="font-size:0.68rem;color:#64748b;">Literal</th>
                            <th style="font-size:0.68rem;color:#64748b;">Value Map</th>
                            <th></th>
                        </tr></thead>
                        <tbody>${fieldsHtml}</tbody>
                    </table>
                   </div>`}
            <div style="display:flex;gap:0.4rem;margin-top:0.4rem;">
                <button type="button" class="btn btn-sm btn-outline-primary"
                    onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.addSegmentField(${segIndex})"
                    style="font-size:0.72rem;padding:0.15rem 0.5rem;">+ Add Field</button>
                <button type="button" class="btn btn-sm btn-outline-secondary"
                    onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.toggleSuggestPanel(${segIndex})"
                    style="font-size:0.72rem;padding:0.15rem 0.5rem;">✨ Suggest Mappings</button>
            </div>
        </div>`;
    }

    // ── AI-assisted "Suggest Mappings" (design-time only) ────────────────────
    // Sends pasted sample rows + this SEGMENT's own live field catalog to
    // POST /api/ai/suggest-field-mappings (services/ai/mapping_suggester_service.go).
    // Suggestions only ever pre-fill the editable field table above — nothing
    // is auto-applied or saved; the pipeline's own Save action remains the
    // human-approval gate.

    _renderSuggestPanel(segIndex) {
        if (this._suggestPanelOpenIndex !== segIndex) return '';
        const esc = HL7BuildBuilder._esc;
        return `
        <div style="border:1px dashed #94a3b8;border-radius:6px;padding:0.6rem 0.7rem;margin-bottom:0.6rem;background:#fafafa;">
            <div style="font-size:0.72rem;color:#64748b;margin-bottom:0.4rem;">Paste sample source rows (a JSON array of objects) — suggestions are added to this segment's field table for you to review, never saved automatically.</div>
            <textarea id="hbbSuggestSampleRows-${segIndex}" class="form-control form-control-sm" rows="4"
                placeholder='[{"lastName":"Doe","mrn":"12345"}]'
                style="font-family:monospace;font-size:0.78rem;margin-bottom:0.4rem;">${esc(this._suggestSampleRowsText || '')}</textarea>
            <div style="display:flex;align-items:center;gap:0.6rem;">
                <button type="button" class="btn btn-sm btn-primary" style="font-size:0.72rem;"
                    onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.runSuggestMappings(${segIndex})">Suggest</button>
                ${this._suggestError ? `<span style="font-size:0.72rem;color:#dc2626;">${esc(this._suggestError)}</span>` : ''}
                ${this._suggestBusy ? `<span style="font-size:0.72rem;color:#64748b;">Asking the local model…</span>` : ''}
            </div>
        </div>`;
    }

    toggleSuggestPanel(segIndex) {
        this._syncDOMToConfig();
        this._suggestPanelOpenIndex = this._suggestPanelOpenIndex === segIndex ? -1 : segIndex;
        this._suggestError = '';
        this._rerender();
    }

    runSuggestMappings(segIndex) {
        const root = document.getElementById('hl7BuildBuilder');
        const textEl = root && root.querySelector(`#hbbSuggestSampleRows-${segIndex}`);
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
        const seg = this._step.config.segments[segIndex];
        if (!seg) return;
        const targetFields = this._fieldCatalogs[seg.segment] || [];

        this._suggestBusy = true;
        this._rerender();

        fetch('/api/ai/suggest-field-mappings', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                step_type: 'hl7.build',
                sample_rows: sampleRows,
                target_fields: targetFields,
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
                if (!Array.isArray(seg.fields)) seg.fields = [];
                const existingKeys = new Set(seg.fields.map(f => f.fieldKey));
                suggestions.forEach(s => {
                    if (!s.target_field || existingKeys.has(s.target_field)) return;
                    seg.fields.push({ fieldKey: s.target_field, sourcePath: s.source_field || '' });
                    existingKeys.add(s.target_field);
                });
                this._suggestPanelOpenIndex = -1;
                this._rerender();
            })
            .catch(() => {
                this._suggestBusy = false;
                this._suggestError = 'Suggestion request failed.';
                this._rerender();
            });
    }

    addSegment() {
        this._syncDOMToConfig();
        this._step.config.segments.push({ segment: '', cardinality: 'single', fields: [] });
        this._rerender();
    }

    removeSegment(index) {
        this._syncDOMToConfig();
        this._step.config.segments.splice(index, 1);
        this._rerender();
    }

    onSegmentNameChange(segIndex, newName) {
        this._syncDOMToConfig();
        const seg = this._step.config.segments[segIndex];
        if (!seg) return;
        seg.segment = newName;
        if (!newName || this._fieldCatalogs[newName]) {
            this._rerender();
            return;
        }
        this._loadFieldCatalog(newName).then(() => this._rerender());
    }

    onCardinalityChange(segIndex, newCardinality) {
        this._syncDOMToConfig();
        const seg = this._step.config.segments[segIndex];
        if (!seg) return;
        seg.cardinality = newCardinality;
        this._rerender();
    }

    addSegmentField(segIndex) {
        this._syncDOMToConfig();
        const seg = this._step.config.segments[segIndex];
        if (!seg) return;
        if (!Array.isArray(seg.fields)) seg.fields = [];
        seg.fields.push({ fieldKey: '', sourcePath: '' });
        this._rerender();
    }

    removeSegmentField(segIndex, fieldIndex) {
        this._syncDOMToConfig();
        const seg = this._step.config.segments[segIndex];
        if (!seg || !Array.isArray(seg.fields)) return;
        seg.fields.splice(fieldIndex, 1);
        this._rerender();
    }

    // ── Message type / version change ────────────────────────────────────────

    onMessageTypeChange(value) {
        this._syncDOMToConfig();
        const [messageType, triggerEvent] = value.split('^');
        this._step.config.messageType = messageType || '';
        this._step.config.triggerEvent = triggerEvent || '';
        this._segmentNames = [];
        this._fieldCatalogs = {};
        this._loadSegmentNames();
        this._rerender();
    }

    onVersionChange() {
        this._syncDOMToConfig();
        this._segmentNames = [];
        this._fieldCatalogs = {};
        this._loadMessageTypes();
        this._loadSegmentNames();
        this._rerender();
    }

    // ── DOM <-> config sync ───────────────────────────────────────────────────

    _syncDOMToConfig() {
        const root = document.getElementById('hl7BuildBuilder');
        if (!root || !this._step) return;
        const cfg = this._step.config;

        const versionEl = root.querySelector('#hbbVersion');
        if (versionEl) cfg.version = versionEl.value.trim() || '2.5.1';
        const outputEl = root.querySelector('#hbbOutputField');
        if (outputEl) cfg.outputField = outputEl.value.trim() || 'hl7Message';
        const sendingAppEl = root.querySelector('#hbbSendingApp');
        if (sendingAppEl) cfg.sendingApplication = sendingAppEl.value.trim() || undefined;
        const sendingFacilityEl = root.querySelector('#hbbSendingFacility');
        if (sendingFacilityEl) cfg.sendingFacility = sendingFacilityEl.value.trim() || undefined;
        const receivingAppEl = root.querySelector('#hbbReceivingApp');
        if (receivingAppEl) cfg.receivingApplication = receivingAppEl.value.trim() || undefined;
        const receivingFacilityEl = root.querySelector('#hbbReceivingFacility');
        if (receivingFacilityEl) cfg.receivingFacility = receivingFacilityEl.value.trim() || undefined;

        root.querySelectorAll('.hbb-segment-card').forEach(card => {
            const sIdx = parseInt(card.dataset.segmentIndex, 10);
            const seg = cfg.segments[sIdx];
            if (!seg) return;
            const rowsPathEl = card.querySelector('.hbb-segment-rowspath');
            if (rowsPathEl) seg.rowsPath = rowsPathEl.value.trim();

            card.querySelectorAll('tr[data-field-index]').forEach(row => {
                const fIdx = parseInt(row.dataset.fieldIndex, 10);
                const f = seg.fields[fIdx];
                if (!f) return;
                f.fieldKey = row.querySelector('.hbb-field-key').value.trim();
                f.sourcePath = row.querySelector('.hbb-field-source').value.trim();
                const fallbackText = row.querySelector('.hbb-field-fallback').value.trim();
                f.fallbackPaths = fallbackText ? fallbackText.split(',').map(s => s.trim()).filter(Boolean) : undefined;
                const literal = row.querySelector('.hbb-field-literal').value.trim();
                f.literalValue = literal || undefined;
                const valueMap = HL7BuildBuilder._parseValueMapText(row.querySelector('.hbb-field-valuemap').value);
                f.valueMap = valueMap && Object.keys(valueMap).length > 0 ? valueMap : undefined;
            });
        });
    }

    _rerender() {
        const root = document.getElementById('hl7BuildBuilder');
        if (!root || !root.parentElement) return;
        this._destroyFieldSearches();
        root.outerHTML = this._renderAll();
        setTimeout(() => this._attachFieldSearches(), 0);
        this._wireTopLevelChangeHandlers();
    }

    _wireTopLevelChangeHandlers() {
        const root = document.getElementById('hl7BuildBuilder');
        if (!root) return;
        const messageTypeEl = root.querySelector('#hbbMessageType');
        if (messageTypeEl) messageTypeEl.onchange = (e) => this.onMessageTypeChange(e.target.value);
        const versionEl = root.querySelector('#hbbVersion');
        if (versionEl) versionEl.onchange = () => this.onVersionChange();
    }

    // ── Field path search wiring ──────────────────────────────────────────────

    _attachFieldSearches() {
        this._wireTopLevelChangeHandlers();
        if (typeof FieldPathSearchComponent === 'undefined') return;
        const root = document.getElementById('hl7BuildBuilder');
        if (!root) return;

        const getStepVars = () => {
            if (this._panel && typeof this._panel.getStepVariablesForSearch === 'function') {
                return this._panel.getStepVariablesForSearch();
            }
            return [];
        };

        root.querySelectorAll('.hbb-field-source, .hbb-segment-rowspath').forEach(input => {
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

    _loadMessageTypes() {
        const version = (this._step.config && this._step.config.version) || '2.5.1';
        fetch(`/api/hl7/message-types?version=${encodeURIComponent(version)}`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.messageTypes) return;
                this._messageTypes = data.messageTypes;
                this._rerender();
            })
            .catch(() => {}); // AbortError on destroy is expected
    }

    _loadSegmentNames() {
        const cfg = this._step.config;
        if (!cfg.messageType || !cfg.triggerEvent) return;
        const params = new URLSearchParams({ version: cfg.version || '2.5.1' });
        fetch(`/api/hl7/segments/${encodeURIComponent(cfg.messageType)}/${encodeURIComponent(cfg.triggerEvent)}?${params}`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.segments) return;
                this._segmentNames = data.segments;
                this._rerender();
            })
            .catch(() => {});
    }

    _loadFieldCatalog(segmentName) {
        const cfg = this._step.config;
        if (!cfg.messageType || !cfg.triggerEvent || !segmentName) return Promise.resolve();
        const params = new URLSearchParams({ version: cfg.version || '2.5.1' });
        return fetch(`/api/hl7/canonical-fields/${encodeURIComponent(cfg.messageType)}/${encodeURIComponent(cfg.triggerEvent)}/${encodeURIComponent(segmentName)}?${params}`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (data && data.fields) this._fieldCatalogs[segmentName] = data.fields;
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
        if (window._hl7BuildBuilder === this) {
            window._hl7BuildBuilder = null;
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
    StepBuilderRegistry.register('hl7.build', HL7BuildBuilder);
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = HL7BuildBuilder;
}
