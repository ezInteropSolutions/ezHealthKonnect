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
 * Segments form a TREE, not a flat list: a segment can have ChildSegments,
 * built immediately after each instance of their parent — the only way to
 * correctly express e.g. "OBX results grouped under their own OBR", since
 * HL7's pipe-delimited wire format has no explicit nesting, only sequential
 * position. Every segment/field is addressed by a PATH (an array of indices
 * from the root, e.g. [0] or [0,1]) rather than a single index — see
 * _getSegmentByPath/_getParentListForPath. GroupBy buckets a single flat
 * rows array (e.g. one CSV) before building one parent instance per bucket
 * instead of per row, so children can iterate that bucket's own rows via
 * RowsPath = GroupedItemsKey (default "_rows"); ChildSegments under a
 * non-GroupBy repeating parent instead resolve RowsPath directly against
 * each parent row, for already-nested source data. See
 * hl7_build_executor.go's own doc comment for the executor-side mechanics.
 *
 * Catalogs (message types, segment names, field/component keys) are all
 * fetched live from the backend (hl7/builder/field_catalog.go, exposed via
 * /api/hl7/*), never hardcoded here — same discipline
 * MapToCanonicalBuilder.js/FHIRBuildBuilder.js follow. The schema-driven
 * ordering guardrail (HL7SegmentPicker's default "allowed next" list, backed
 * by /api/hl7/next-segments) only applies at the ROOT level — it encodes the
 * schema's own top-to-bottom segment sequence, which has no direct
 * equivalent for "what may nest under segment X" without a second dedicated
 * endpoint. Nested (child) segment pickers run in "unrestricted" mode
 * instead: any segment valid for the message type, no forward-only
 * restriction — a deliberate v1 scope cut, not an oversight.
 *
 * Builder contract (StepBuilderRegistry):
 *   render(step)         → string  HTML for the properties panel form tab
 *   collectConfig(step)  → void    reads DOM, writes into step.config
 *   destroy()            → void    tears down event listeners / AC refs
 */

// Mirrors services/executors/condition.go's EvaluateCondition switch exactly.
// Operators are inherent to that Go logic, not external/schema data (unlike
// message types, segments, or fields), so a small hand-synced list here —
// rather than a live catalog endpoint — is the right-sized choice: adding an
// operator always requires a Go change anyway, at which point updating this
// list in the same change is a normal, low-risk sync.
const HL7_CONDITION_OPERATORS = [
    { value: 'equals', label: 'Equals' },
    { value: 'not_equals', label: 'Not Equals' },
    { value: 'contains', label: 'Contains' },
    { value: 'starts_with', label: 'Starts With' },
    { value: 'ends_with', label: 'Ends With' },
    { value: 'greater_than', label: 'Greater Than' },
    { value: 'greater_than_or_equal', label: 'Greater Than or Equal' },
    { value: 'less_than', label: 'Less Than' },
    { value: 'less_than_or_equal', label: 'Less Than or Equal' },
    { value: 'exists', label: 'Exists' },
    { value: 'not_exists', label: 'Does Not Exist' },
    { value: 'regex_match', label: 'Matches Regex' },
    { value: 'in_list', label: 'In List' },
];

class HL7BuildBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
        this._step = null;
        this._messageTypes = [];       // [{messageType, triggerEvent}]
        this._versions = [];           // ["2.3","2.5.1",...] from /api/hl7/versions
        this._segmentNames = [];       // ["MSH","PID","PV1",...] for current messageType/triggerEvent/version
        this._fieldCatalogs = {};      // segmentName -> [{key, label, dataType, required, canRepeat}]
        this._schemaTree = [];         // group/segment tree from /api/hl7/schema-tree, for CanRepeat gating + auto-seed
        this._requiredSpine = [];      // required-segment names, for auto-seeding a fresh step's segment list
        this._fieldSearches = [];      // active FieldPathSearchComponent instances (source-path/condition-field cells)
        this._segmentPickers = [];     // active HL7SegmentPicker instances (segment-name cells)
        this._fieldPickers = [];       // active HL7FieldPathPicker instances (field-key cells)
        this._suggestPanelOpenPath = null; // path-string ("0" / "0,1") of the segment whose Suggest Mappings panel is open
        this._suggestSampleRowsText = '';
        this._suggestError = '';
        this._suggestBusy = false;
        this._conditionPanelOpen = new Set(); // `${pathStr}|${fieldIndex}` keys with an expanded field-level condition editor

        window._hl7BuildBuilder = this;
    }

    // ── render ────────────────────────────────────────────────────────────────

    render(step) {
        this._step = step;
        if (!step.config) step.config = {};
        this._applyDefaultConfig(step.config);

        this._loadVersions();
        this._loadMessageTypes();
        this._loadSegmentNames();
        this._loadSchemaTree();

        const html = this._renderAll();
        setTimeout(() => this._attachChildComponents(), 0);
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
                <div style="flex:0 0 110px;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Version</label>
                    <select id="hbbVersion" class="form-select form-select-sm">
                        ${(this._versions.length ? this._versions : [cfg.version]).map(v =>
                            `<option value="${esc(v)}" ${v === cfg.version ? 'selected' : ''}>${esc(v)}</option>`
                        ).join('')}
                    </select>
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
                        onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.addSegment('')">
                        <i class="fas fa-plus"></i> Add Segment
                    </button>
                </div>
                <div style="font-size:0.7rem;color:#94a3b8;margin-bottom:0.5rem;">Segments render in the order listed here, after the auto-generated MSH.</div>
                <div id="hbbSegmentsContainer">
                    ${this._renderSegmentList(cfg.segments, [])}
                </div>
            </div>
        </div>`;
    }

    // ── Path addressing ──────────────────────────────────────────────────────
    // Every segment/field is addressed by a PATH: an array of indices from
    // the root segments list down through nested ChildSegments (e.g. [0] is
    // root segments[0]; [0,2] is that segment's 3rd child). Paths travel
    // through the DOM as comma-joined strings (data-path="0,2") since HTML
    // attributes are strings.

    static _pathToStr(path) {
        return path.join(',');
    }

    static _pathFromStr(str) {
        return str === '' || str == null ? [] : String(str).split(',').map(Number);
    }

    _getSegmentByPath(path) {
        let list = this._step.config.segments;
        let seg = null;
        for (const idx of path) {
            seg = list ? list[idx] : null;
            if (!seg) return null;
            list = seg.childSegments;
        }
        return seg;
    }

    // Returns the array a NEW segment at parentPath should be pushed into —
    // this._step.config.segments for parentPath [], or that segment's own
    // (lazily-created) childSegments array otherwise.
    _getParentListForPath(parentPath) {
        let list = this._step.config.segments;
        for (const idx of parentPath) {
            const seg = list[idx];
            if (!seg) return null;
            if (!Array.isArray(seg.childSegments)) seg.childSegments = [];
            list = seg.childSegments;
        }
        return list;
    }

    // ── Segments ──────────────────────────────────────────────────────────────

    _renderSegmentList(list, parentPath) {
        if (!list || list.length === 0) {
            return parentPath.length === 0
                ? `<div style="text-align:center;padding:1.5rem;color:#94a3b8;font-size:0.8rem;border:1px dashed #cbd5e1;border-radius:6px;">No segments configured yet. Click "Add Segment" to map fields into a segment.</div>`
                : `<div style="font-size:0.72rem;color:#94a3b8;padding:0.2rem 0;">No child segments yet.</div>`;
        }
        return list.map((seg, i) => this._renderSegmentCard(seg, [...parentPath, i], list)).join('');
    }

    _renderSegmentCard(seg, path, siblingList) {
        const esc = HL7BuildBuilder._esc;
        const pathStr = HL7BuildBuilder._pathToStr(path);
        const segIndex = path[path.length - 1];
        const isRoot = path.length === 1;
        const precedingSegments = siblingList.slice(0, segIndex).map(s => s.segment).filter(Boolean);

        const canRepeat = this._canSegmentRepeat(seg.segment);
        // The schema disallows repeating here — force back to "single" so a
        // stale/legacy config value can never silently claim otherwise.
        if (!canRepeat && seg.cardinality === 'repeating') seg.cardinality = 'single';
        const isRepeating = seg.cardinality === 'repeating';
        const hasGroupBy = isRepeating && Array.isArray(seg.groupBy) && seg.groupBy.length > 0;

        const fieldsHtml = (seg.fields || []).map((f, fi) => {
            const condKey = `${pathStr}|${fi}`;
            const condOpen = this._conditionPanelOpen.has(condKey) || !!f.condition;
            return `
            <tr data-field-index="${fi}">
                <td>
                    <div class="hbb-field-picker" data-path="${pathStr}" data-field-index="${fi}"></div>
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
                <td style="width:70px;text-align:center;white-space:nowrap;">
                    <button type="button" class="btn btn-sm ${f.condition ? 'btn-warning' : 'btn-outline-secondary'}"
                        onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.toggleFieldConditionPanel('${pathStr}', ${fi})"
                        title="Populate only if a condition holds">⚡</button>
                    <button type="button" class="btn btn-sm btn-danger"
                        onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.removeSegmentField('${pathStr}', ${fi})"
                        title="Remove field"><i class="fas fa-trash"></i></button>
                </td>
            </tr>
            ${condOpen ? `
            <tr class="hbb-field-condition-row">
                <td colspan="6" style="padding-top:0;">
                    ${this._renderConditionEditor(f.condition, { kind: 'field', path: pathStr, 'field-index': fi })}
                </td>
            </tr>` : ''}`;
        }).join('');

        return `
        <div class="hbb-segment-card" data-path="${pathStr}"
            style="border:1px solid #e2e8f0;border-radius:8px;padding:0.7rem 0.8rem;margin-bottom:0.8rem;background:#fafbfc;${isRoot ? '' : 'margin-left:1.1rem;border-left:3px solid #93c5fd;'}">
            <div class="hbb-card-header" style="display:flex;align-items:center;gap:0.6rem;margin-bottom:0.55rem;flex-wrap:wrap;">
                <div class="hbb-segment-picker" data-path="${pathStr}" style="flex:0 0 200px;"></div>
                ${canRepeat ? `
                <select class="form-select form-select-sm hbb-segment-cardinality" style="flex:0 0 140px;"
                    onchange="window._hl7BuildBuilder && window._hl7BuildBuilder.onCardinalityChange('${pathStr}', this.value)">
                    <option value="single" ${!isRepeating ? 'selected' : ''}>Single</option>
                    <option value="repeating" ${isRepeating ? 'selected' : ''}>Repeating</option>
                </select>` : `
                <span class="form-control form-control-sm hbb-segment-cardinality-fixed" style="flex:0 0 140px;background:#f1f5f9;color:#94a3b8;cursor:not-allowed;"
                    title="${esc(seg.segment || 'This segment')} cannot repeat per the HL7 schema">Single (fixed)</span>`}
                ${isRepeating ? `
                <input type="text" class="form-control form-control-sm hbb-segment-rowspath" data-search-kind="rowspath"
                    value="${esc(seg.rowsPath)}" placeholder="e.g. labResults"
                    style="flex:1 1 150px;font-family:monospace;font-size:0.8rem;">
                <input type="text" class="form-control form-control-sm hbb-segment-groupby"
                    value="${esc((seg.groupBy || []).join(', '))}" placeholder="Group By (e.g. orderId)"
                    title="Optional: bucket Rows Path's rows by these column(s) before building one instance per bucket instead of per row — lets a single flat CSV (e.g. one row per analyte result) produce a grouped parent+children (one OBR per order, each with its own OBX children)"
                    style="flex:1 1 150px;font-family:monospace;font-size:0.8rem;">` : ''}
                <button type="button" class="btn btn-sm btn-outline-danger"
                    onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.removeSegment('${pathStr}')"
                    title="Remove segment"><i class="fas fa-trash"></i></button>
            </div>
            ${isRepeating ? `<div style="font-size:0.7rem;color:#94a3b8;margin-bottom:0.5rem;">
                Rows Path: pipeline field holding the array of source rows — one ${esc(seg.segment || 'segment')} instance is built per ${hasGroupBy ? 'group' : 'row'}.
                ${hasGroupBy ? `Grouped by <code>${esc(seg.groupBy.join(', '))}</code> — a child segment below can iterate this group's own rows via Rows Path <code>${esc(seg.groupedItemsKey || '_rows')}</code>.` : ''}
            </div>` : ''}
            <div class="hbb-card-condition">
                ${seg.condition
                    ? this._renderConditionEditor(seg.condition, { kind: 'segment', path: pathStr })
                    : `<button type="button" class="btn btn-sm btn-link hbb-add-condition" style="font-size:0.7rem;padding:0.1rem 0;margin-bottom:0.3rem;"
                        onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.addCondition('segment', '${pathStr}')">+ Add condition (only build ${isRepeating ? 'a row/group of ' : ''}this segment if…)</button>`}
            </div>
            ${this._renderSuggestPanel(pathStr)}
            <div class="hbb-card-fields">
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
            </div>
            <div class="hbb-card-actions" style="display:flex;gap:0.4rem;margin-top:0.4rem;">
                <button type="button" class="btn btn-sm btn-outline-primary"
                    onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.addSegmentField('${pathStr}')"
                    style="font-size:0.72rem;padding:0.15rem 0.5rem;">+ Add Field</button>
                <button type="button" class="btn btn-sm btn-outline-secondary"
                    onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.toggleSuggestPanel('${pathStr}')"
                    style="font-size:0.72rem;padding:0.15rem 0.5rem;">✨ Suggest Mappings</button>
            </div>
            <div class="hbb-child-segments" style="margin-top:0.6rem;padding-top:0.5rem;border-top:1px dashed #cbd5e1;">
                <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:0.4rem;">
                    <label style="font-size:0.68rem;font-weight:600;text-transform:uppercase;color:#64748b;">
                        Child Segments <span style="font-weight:400;text-transform:none;color:#94a3b8;">(built immediately after each ${esc(seg.segment || 'this')} instance)</span>
                    </label>
                    <button type="button" class="btn btn-sm btn-outline-primary" style="font-size:0.68rem;padding:0.1rem 0.4rem;"
                        onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.addSegment('${pathStr}')">+ Add Child Segment</button>
                </div>
                ${this._renderSegmentList(seg.childSegments || [], path)}
            </div>
        </div>`;
    }

    // ── Conditional segment/field population ────────────────────────────────
    // Renders one {field, operator, value} condition editor — shared markup
    // for both segment-level Condition (gates whether this segment/row/group
    // is built at all) and field-level Condition (gates whether just this
    // one field is populated). Evaluated server-side by
    // executors.EvaluateCondition (services/executors/condition.go) — the
    // same shared evaluator control.if_then_else/control.switch_case use —
    // against the current row with fallback to the top-level pipeline data
    // when the field isn't found on the row (hl7_build_executor.go's
    // conditionMet/mergeWithFallback).
    _renderConditionEditor(condition, ctxAttrs) {
        const esc = HL7BuildBuilder._esc;
        const cond = condition || {};
        const operator = cond.operator || 'equals';
        const needsValue = operator !== 'exists' && operator !== 'not_exists';
        const isListOp = operator === 'in_list';
        const valueText = isListOp
            ? (Array.isArray(cond.value) ? cond.value.join(', ') : '')
            : (cond.value !== undefined && cond.value !== null ? cond.value : '');
        const attrs = Object.entries(ctxAttrs).map(([k, v]) => `data-${k}="${esc(v)}"`).join(' ');
        const fieldIndex = ctxAttrs['field-index'];

        return `
        <div class="hbb-condition-editor" ${attrs}
            style="display:flex;gap:0.3rem;align-items:center;flex-wrap:wrap;background:#fffbeb;border:1px dashed #f0c36d;border-radius:5px;padding:0.35rem 0.5rem;margin-bottom:0.4rem;">
            <span style="font-size:0.68rem;color:#92400e;font-weight:600;">IF</span>
            <input type="text" class="form-control form-control-sm hbb-condition-field" style="flex:1 1 140px;font-family:monospace;font-size:0.75rem;"
                value="${esc(cond.field)}" placeholder="field path (e.g. message.country)" autocomplete="off">
            <select class="form-select form-select-sm hbb-condition-operator" style="flex:0 0 160px;font-size:0.75rem;"
                onchange="window._hl7BuildBuilder && window._hl7BuildBuilder.onConditionOperatorChange('${ctxAttrs.kind}', '${ctxAttrs.path}', ${fieldIndex ?? 'null'}, this.value)">
                ${HL7_CONDITION_OPERATORS.map(op => `<option value="${op.value}" ${op.value === operator ? 'selected' : ''}>${op.label}</option>`).join('')}
            </select>
            ${needsValue ? `<input type="text" class="form-control form-control-sm hbb-condition-value" style="flex:1 1 120px;font-size:0.75rem;"
                value="${esc(valueText)}" placeholder="${isListOp ? 'val1, val2' : 'value'}" autocomplete="off">` : ''}
            <button type="button" class="btn btn-sm btn-outline-danger"
                onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.removeCondition('${ctxAttrs.kind}', '${ctxAttrs.path}', ${fieldIndex ?? 'null'})"
                title="Remove condition">×</button>
        </div>`;
    }

    addCondition(kind, pathStr, fieldIndex) {
        this._syncDOMToConfig();
        const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(pathStr));
        if (!seg) return;
        const blank = { field: '', operator: 'equals', value: '' };
        if (kind === 'segment') {
            seg.condition = blank;
        } else {
            const f = seg.fields && seg.fields[fieldIndex];
            if (!f) return;
            f.condition = blank;
            this._conditionPanelOpen.add(`${pathStr}|${fieldIndex}`);
        }
        this._rerender();
    }

    removeCondition(kind, pathStr, fieldIndex) {
        this._syncDOMToConfig();
        const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(pathStr));
        if (!seg) return;
        if (kind === 'segment') {
            delete seg.condition;
        } else {
            const f = seg.fields && seg.fields[fieldIndex];
            if (f) delete f.condition;
            this._conditionPanelOpen.delete(`${pathStr}|${fieldIndex}`);
        }
        this._rerender();
    }

    toggleFieldConditionPanel(pathStr, fieldIndex) {
        this._syncDOMToConfig();
        const key = `${pathStr}|${fieldIndex}`;
        if (this._conditionPanelOpen.has(key)) {
            this._conditionPanelOpen.delete(key);
        } else {
            this._conditionPanelOpen.add(key);
            const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(pathStr));
            const f = seg && seg.fields && seg.fields[fieldIndex];
            if (f && !f.condition) f.condition = { field: '', operator: 'equals', value: '' };
        }
        this._rerender();
    }

    onConditionOperatorChange(kind, pathStr, fieldIndex, newOperator) {
        this._syncDOMToConfig();
        const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(pathStr));
        if (!seg) return;
        const target = kind === 'segment' ? seg : (seg.fields && seg.fields[fieldIndex]);
        if (target && target.condition) target.condition.operator = newOperator;
        this._rerender();
    }

    // ── AI-assisted "Suggest Mappings" (design-time only) ────────────────────
    // Sends pasted sample rows + this SEGMENT's own live field catalog to
    // POST /api/ai/suggest-field-mappings (services/ai/mapping_suggester_service.go).
    // Suggestions only ever pre-fill the editable field table above — nothing
    // is auto-applied or saved; the pipeline's own Save action remains the
    // human-approval gate.

    _renderSuggestPanel(pathStr) {
        if (this._suggestPanelOpenPath !== pathStr) return '';
        const esc = HL7BuildBuilder._esc;
        const idSafe = pathStr.replace(/,/g, '-');
        return `
        <div style="border:1px dashed #94a3b8;border-radius:6px;padding:0.6rem 0.7rem;margin-bottom:0.6rem;background:#fafafa;">
            <div style="font-size:0.72rem;color:#64748b;margin-bottom:0.4rem;">Paste sample source rows (a JSON array of objects) — suggestions are added to this segment's field table for you to review, never saved automatically.</div>
            <textarea id="hbbSuggestSampleRows-${idSafe}" class="form-control form-control-sm" rows="4"
                placeholder='[{"lastName":"Doe","mrn":"12345"}]'
                style="font-family:monospace;font-size:0.78rem;margin-bottom:0.4rem;">${esc(this._suggestSampleRowsText || '')}</textarea>
            <div style="display:flex;align-items:center;gap:0.6rem;">
                <button type="button" class="btn btn-sm btn-primary" style="font-size:0.72rem;"
                    onclick="window._hl7BuildBuilder && window._hl7BuildBuilder.runSuggestMappings('${pathStr}')">Suggest</button>
                ${this._suggestError ? `<span style="font-size:0.72rem;color:#dc2626;">${esc(this._suggestError)}</span>` : ''}
                ${this._suggestBusy ? `<span style="font-size:0.72rem;color:#64748b;">Asking the local model…</span>` : ''}
            </div>
        </div>`;
    }

    toggleSuggestPanel(pathStr) {
        this._syncDOMToConfig();
        this._suggestPanelOpenPath = this._suggestPanelOpenPath === pathStr ? null : pathStr;
        this._suggestError = '';
        this._rerender();
    }

    runSuggestMappings(pathStr) {
        const root = document.getElementById('hl7BuildBuilder');
        const idSafe = pathStr.replace(/,/g, '-');
        const textEl = root && root.querySelector(`#hbbSuggestSampleRows-${idSafe}`);
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
        const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(pathStr));
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
                this._suggestPanelOpenPath = null;
                this._rerender();
            })
            .catch(() => {
                this._suggestBusy = false;
                this._suggestError = 'Suggestion request failed.';
                this._rerender();
            });
    }

    addSegment(parentPathStr) {
        this._syncDOMToConfig();
        const list = this._getParentListForPath(HL7BuildBuilder._pathFromStr(parentPathStr));
        if (!list) return;
        list.push({ segment: '', cardinality: 'single', fields: [] });
        this._rerender();
    }

    removeSegment(pathStr) {
        this._syncDOMToConfig();
        const path = HL7BuildBuilder._pathFromStr(pathStr);
        const index = path[path.length - 1];
        const list = this._getParentListForPath(path.slice(0, -1));
        if (!list) return;
        list.splice(index, 1);
        this._rerender();
    }

    // Called by HL7SegmentPicker's onChange — replaces the old plain-<select>
    // onchange attribute now that segment-name choice is grammar-guarded (at
    // the root level; unrestricted for nested children — see this file's own
    // header comment). meta.matchedTemplate (a ZSegmentConfigBuilder
    // template, when the typed name matches one) pre-fills this segment's
    // fields with the template's position list, exactly like "✨ Suggest
    // Mappings" pre-fills without auto-saving — never overwrites fields the
    // user already mapped.
    _handleSegmentPickerChange(pathStr, newName, meta) {
        this._syncDOMToConfig();
        const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(pathStr));
        if (!seg) return;
        seg.segment = newName;

        const finish = () => {
            if (meta && meta.matchedTemplate && (!seg.fields || seg.fields.length === 0)) {
                seg.fields = (meta.matchedTemplate.fields || [])
                    .filter(f => f && f.position)
                    .map(f => ({ fieldKey: f.position, sourcePath: '' }));
            }
            this._rerender();
        };

        if (!newName || this._fieldCatalogs[newName]) {
            finish();
            return;
        }
        this._loadFieldCatalog(newName).then(finish);
    }

    // Looks up whether segmentName can legally repeat, per the loaded schema
    // tree (own repeat="*" or inherited from an enclosing repeating group —
    // see hl7/builder.SchemaTree). A segment absent from the tree (a Z/custom
    // segment the schema knows nothing about) has no constraint to enforce,
    // so it's always allowed to repeat. Pure name lookup — unaffected by
    // where in the segment TREE this UI card lives.
    _canSegmentRepeat(segmentName) {
        if (!segmentName) return false;
        const node = HL7BuildBuilder._findTreeNode(this._schemaTree, segmentName);
        return node ? !!node.canRepeat : true;
    }

    static _findTreeNode(nodes, name) {
        for (const n of nodes || []) {
            if (n.name === name) return n;
            const found = HL7BuildBuilder._findTreeNode(n.children, name);
            if (found) return found;
        }
        return null;
    }

    onCardinalityChange(pathStr, newCardinality) {
        this._syncDOMToConfig();
        const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(pathStr));
        if (!seg) return;
        seg.cardinality = newCardinality;
        this._rerender();
    }

    addSegmentField(pathStr) {
        this._syncDOMToConfig();
        const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(pathStr));
        if (!seg) return;
        if (!Array.isArray(seg.fields)) seg.fields = [];
        seg.fields.push({ fieldKey: '', sourcePath: '' });
        this._rerender();
    }

    removeSegmentField(pathStr, fieldIndex) {
        this._syncDOMToConfig();
        const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(pathStr));
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
        this._schemaTree = [];
        this._requiredSpine = [];
        this._loadSegmentNames();
        this._loadSchemaTree();
        this._rerender();
    }

    onVersionChange() {
        this._syncDOMToConfig();
        this._segmentNames = [];
        this._fieldCatalogs = {};
        this._schemaTree = [];
        this._requiredSpine = [];
        this._loadMessageTypes();
        this._loadSegmentNames();
        this._loadSchemaTree();
        this._rerender();
    }

    // ── DOM <-> config sync ───────────────────────────────────────────────────
    // Per-card single-value inputs (rowsPath, groupBy) are read via
    // ":scope > .hbb-card-header ..." so a query on a PARENT card can never
    // pick up a NESTED child segment card's own inputs — child cards live
    // several levels down inside ".hbb-child-segments", never as a direct
    // child of ".hbb-card-header". Field rows/condition editors instead
    // carry their own explicit path (+ field-index) as data attributes, so
    // they're resolved directly rather than needing card-scoping at all.

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
            const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(card.dataset.path));
            if (!seg) return;

            const rowsPathEl = card.querySelector(':scope > .hbb-card-header .hbb-segment-rowspath');
            if (rowsPathEl) seg.rowsPath = rowsPathEl.value.trim();
            const groupByEl = card.querySelector(':scope > .hbb-card-header .hbb-segment-groupby');
            if (groupByEl) {
                const gb = groupByEl.value.trim();
                seg.groupBy = gb ? gb.split(',').map(s => s.trim()).filter(Boolean) : undefined;
            }

            card.querySelectorAll(':scope > .hbb-card-fields tr[data-field-index]').forEach(row => {
                const fIdx = parseInt(row.dataset.fieldIndex, 10);
                const f = seg.fields[fIdx];
                if (!f) return;
                // f.fieldKey is kept in sync live by HL7FieldPathPicker's onChange
                // callback (see _attachChildComponents) — no DOM value to read here.
                f.sourcePath = row.querySelector('.hbb-field-source').value.trim();
                const fallbackText = row.querySelector('.hbb-field-fallback').value.trim();
                f.fallbackPaths = fallbackText ? fallbackText.split(',').map(s => s.trim()).filter(Boolean) : undefined;
                const literal = row.querySelector('.hbb-field-literal').value.trim();
                f.literalValue = literal || undefined;
                const valueMap = HL7BuildBuilder._parseValueMapText(row.querySelector('.hbb-field-valuemap').value);
                f.valueMap = valueMap && Object.keys(valueMap).length > 0 ? valueMap : undefined;
            });

            const segCondEditor = card.querySelector(':scope > .hbb-card-condition .hbb-condition-editor');
            if (segCondEditor) this._syncOneCondition(segCondEditor, seg);
        });

        root.querySelectorAll('.hbb-condition-editor[data-kind="field"]').forEach(editor => {
            const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(editor.dataset.path));
            const fIdx = parseInt(editor.dataset.fieldIndex, 10);
            const f = seg && seg.fields && seg.fields[fIdx];
            if (f) this._syncOneCondition(editor, f);
        });
    }

    _syncOneCondition(editor, target) {
        const field = editor.querySelector('.hbb-condition-field').value.trim();
        if (!field) { delete target.condition; return; }
        const operator = editor.querySelector('.hbb-condition-operator').value;
        const valueEl = editor.querySelector('.hbb-condition-value');
        const condition = { field, operator };
        if (valueEl) {
            condition.value = operator === 'in_list'
                ? valueEl.value.split(',').map(s => s.trim()).filter(Boolean)
                : valueEl.value;
        }
        target.condition = condition;
    }

    _rerender() {
        const root = document.getElementById('hl7BuildBuilder');
        if (!root || !root.parentElement) return;
        this._destroyChildComponents();
        root.outerHTML = this._renderAll();
        setTimeout(() => this._attachChildComponents(), 0);
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

    // ── Child component wiring (field search, segment picker, field picker) ───

    _attachChildComponents() {
        this._wireTopLevelChangeHandlers();
        const root = document.getElementById('hl7BuildBuilder');
        if (!root) return;
        const cfg = this._step.config;

        if (typeof FieldPathSearchComponent !== 'undefined') {
            const getStepVars = () => {
                if (this._panel && typeof this._panel.getStepVariablesForSearch === 'function') {
                    return this._panel.getStepVariablesForSearch();
                }
                return [];
            };
            root.querySelectorAll('.hbb-field-source, .hbb-segment-rowspath, .hbb-condition-field').forEach(input => {
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

        if (typeof HL7SegmentPicker !== 'undefined') {
            root.querySelectorAll('.hbb-segment-picker').forEach(el => {
                const path = HL7BuildBuilder._pathFromStr(el.dataset.path);
                const seg = this._getSegmentByPath(path);
                if (!seg) return;
                const isRoot = path.length === 1;
                const parentList = isRoot ? cfg.segments : (this._getSegmentByPath(path.slice(0, -1)).childSegments || []);
                const segIndex = path[path.length - 1];
                const precedingSegments = parentList.slice(0, segIndex).map(s => s.segment).filter(Boolean);
                const picker = new HL7SegmentPicker(el, {
                    value: seg.segment,
                    messageType: cfg.messageType,
                    triggerEvent: cfg.triggerEvent,
                    version: cfg.version,
                    precedingSegments,
                    unrestricted: !isRoot,
                    allSegmentNames: this._segmentNames,
                    onChange: (name, meta) => this._handleSegmentPickerChange(el.dataset.path, name, meta),
                });
                this._segmentPickers.push(picker);
            });
        }

        if (typeof HL7FieldPathPicker !== 'undefined') {
            root.querySelectorAll('.hbb-field-picker').forEach(el => {
                const seg = this._getSegmentByPath(HL7BuildBuilder._pathFromStr(el.dataset.path));
                const fieldIndex = parseInt(el.dataset.fieldIndex, 10);
                const f = seg && seg.fields && seg.fields[fieldIndex];
                if (!f) return;
                const picker = new HL7FieldPathPicker(el, {
                    value: f.fieldKey,
                    segment: seg.segment,
                    fieldCatalog: this._fieldCatalogs[seg.segment] || [],
                    onChange: (val) => { f.fieldKey = val; },
                });
                this._fieldPickers.push(picker);
            });
        }
    }

    _destroyChildComponents() {
        this._fieldSearches.forEach(s => { try { s.destroy(); } catch (e) { /* no-op */ } });
        this._fieldSearches = [];
        this._segmentPickers.forEach(p => { try { p.destroy(); } catch (e) { /* no-op */ } });
        this._segmentPickers = [];
        this._fieldPickers.forEach(p => { try { p.destroy(); } catch (e) { /* no-op */ } });
        this._fieldPickers = [];
    }

    // ── Data loading ──────────────────────────────────────────────────────────

    _loadVersions() {
        fetch('/api/hl7/versions', { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.versions) return;
                this._versions = data.versions;
                this._rerender();
            })
            .catch(() => {});
    }

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

    // Loads the group/segment tree + required-segment spine for the current
    // messageType/triggerEvent/version (hl7/builder.SchemaTree/RequiredSpine),
    // then auto-seeds a still-empty ROOT segments list with the required
    // spine — never touches a segments list the user has already started
    // editing, and never applies to nested childSegments (auto-seed is a
    // root-level-only concept: the required spine describes the message's
    // own top-level structure, not what any particular segment might nest).
    _loadSchemaTree() {
        const cfg = this._step.config;
        if (!cfg.messageType || !cfg.triggerEvent) return;
        const params = new URLSearchParams({ version: cfg.version || '2.5.1' });
        fetch(`/api/hl7/schema-tree/${encodeURIComponent(cfg.messageType)}/${encodeURIComponent(cfg.triggerEvent)}?${params}`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.success) return;
                this._schemaTree = data.tree || [];
                this._requiredSpine = data.requiredSpine || [];
                this._autoSeedRequiredSegments();
                this._rerender();
            })
            .catch(() => {});
    }

    _autoSeedRequiredSegments() {
        const cfg = this._step.config;
        if (!Array.isArray(cfg.segments) || cfg.segments.length > 0) return;
        if (!this._requiredSpine || this._requiredSpine.length === 0) return;
        cfg.segments = this._requiredSpine.map(name => ({ segment: name, cardinality: 'single', fields: [] }));
        cfg.segments.forEach(seg => this._loadFieldCatalog(seg.segment));
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
        this._destroyChildComponents();
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
