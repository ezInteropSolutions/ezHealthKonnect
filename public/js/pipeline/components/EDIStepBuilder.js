/**
 * EDIStepBuilder — step config builders for EDI X12 pipeline steps (835 phase 1).
 *
 * Registers four step types with StepBuilderRegistry:
 *   - edi.parse             Parse raw X12 EDI content to ParsedJSON
 *   - edi.validate          Re-check raw EDI against OOB spec rules (errors)
 *                            + optional user-defined SyntaxRules (warnings)
 *   - edi.map_to_canonical  No-code field mapping from any source shape into
 *                            edi.build's canonical interchange/header/loops/trailer JSON
 *   - edi.build             Build a complete ISA...IEA X12 interchange
 *
 * Builder contract (StepBuilderRegistry), matching CDAStepBuilder.js exactly:
 *   render(step)         → string  HTML for the properties panel form tab
 *   collectConfig(step)  → void    reads DOM, writes into step.config
 *   destroy()            → void    tears down event listeners / AC refs
 */

// ── shared helpers (file-scope, used by every builder below) ───────────────

function ediEsc(s) {
    return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// Small, closed, standard-defined set — same category as edi/datatypes.go's
// own X12DataType registry, not the "one entry per business type" anti-
// pattern (it doesn't grow per segment/customer). date_to_cda/datetime_to_cda
// exist in the same shared Go registry (canonical_value_transforms.go) but
// are CDA-output-specific, so deliberately excluded here.
const EDI_TRANSFORM_OPTIONS = [
    { value: '', label: '(none)' },
    { value: 'trim', label: 'Trim whitespace' },
    { value: 'uppercase', label: 'Uppercase' },
    { value: 'lowercase', label: 'Lowercase' },
    { value: 'date_to_x12', label: 'Date → X12 CCYYMMDD' },
    { value: 'time_to_x12', label: 'Time → X12 HHMM' },
];

function ediTransformOptionsHTML(selected) {
    return EDI_TRANSFORM_OPTIONS.map(o =>
        `<option value="${ediEsc(o.value)}" ${o.value === (selected || '') ? 'selected' : ''}>${ediEsc(o.label)}</option>`
    ).join('');
}

// Only "835" is implemented end-to-end in phase 1 (edi_build_executor.go's
// own default is likewise hardcoded "835") — extend this list when 837 lands.
const EDI_TRANSACTION_SETS = ['835'];

function ediTransactionSetOptionsHTML(selected) {
    return EDI_TRANSACTION_SETS.map(ts =>
        `<option value="${ediEsc(ts)}" ${ts === (selected || '835') ? 'selected' : ''}>${ediEsc(ts)}</option>`
    ).join('');
}

// ── EdiParseStepBuilder ──────────────────────────────────────────────────────
// Config: { sourceField, outputField, transactionSet }

class EdiParseStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;

        const sourceField = ediEsc(cfg.sourceField || 'raw');
        const outputField = ediEsc(cfg.outputField || 'parsedEDI');

        return `
        <div class="edi-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Parse step.</strong> Converts raw X12 EDI content into a structured JSON document
                (interchange/header/loops/trailer). Note: EDI content received by an
                <code>edi_x12_inbound</code> connector is already parsed automatically before any
                pipeline step runs — this step is for re-parsing EDI content that shows up mid-pipeline
                from elsewhere (a DB lookup, a mid-pipeline connector pull, etc.).
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="ediParseSourceField" type="text" class="form-control form-control-sm"
                    value="${sourceField}" placeholder="raw"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field containing the raw X12 EDI string.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ediParseOutputField" type="text" class="form-control form-control-sm"
                    value="${outputField}" placeholder="parsedEDI"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the parsed JSON will be written.</div>
            </div>
            <div class="config-group">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Transaction Set</label>
                <select id="ediParseTransactionSet" class="form-select form-select-sm">
                    ${ediTransactionSetOptionsHTML(cfg.transactionSet)}
                </select>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Only 835 (Health Care Claim Payment/Advice) is supported in phase 1.</div>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const sourceEl = form.querySelector('#ediParseSourceField');
        const outputEl = form.querySelector('#ediParseOutputField');
        const txSetEl = form.querySelector('#ediParseTransactionSet');

        if (sourceEl) step.config.sourceField = sourceEl.value.trim() || 'raw';
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'parsedEDI';
        if (txSetEl) step.config.transactionSet = txSetEl.value || '835';
    }

    destroy() {
        this._ac.abort();
    }
}

// ── EdiBuildStepBuilder ──────────────────────────────────────────────────────
// Config: { sourceField, transactionSet, outputField, isaSenderId, isaReceiverId,
//           gsSenderCode, gsReceiverCode }

class EdiBuildStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;

        const sourceField = ediEsc(cfg.sourceField || 'parsedEDI');
        const outputField = ediEsc(cfg.outputField || 'ediX12');
        const isaSenderId = ediEsc(cfg.isaSenderId || '');
        const isaReceiverId = ediEsc(cfg.isaReceiverId || '');
        const gsSenderCode = ediEsc(cfg.gsSenderCode || '');
        const gsReceiverCode = ediEsc(cfg.gsReceiverCode || '');

        return `
        <div class="edi-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Build step.</strong> Builds a complete ISA...IEA X12 interchange from canonical
                JSON. Accepts either <code>edi.parse</code>'s own output (a round trip) or
                <code>edi.map_to_canonical</code>'s output (built from any source shape) — both produce
                the same interchange/header/loops/trailer shape this step expects.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="ediBuildSourceField" type="text" class="form-control form-control-sm"
                    value="${sourceField}" placeholder="parsedEDI"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field holding canonical interchange/header/loops/trailer JSON.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ediBuildOutputField" type="text" class="form-control form-control-sm"
                    value="${outputField}" placeholder="ediX12"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the built EDI text will be written.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Transaction Set</label>
                <select id="ediBuildTransactionSet" class="form-select form-select-sm">
                    ${ediTransactionSetOptionsHTML(cfg.transactionSet)}
                </select>
            </div>
            <div style="border-top:1px solid #e2e8f0;margin:1rem 0;padding-top:0.85rem;">
                <div style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;margin-bottom:0.6rem;">ISA/GS Trading-Partner Identifiers</div>
                <div style="font-size:0.72rem;color:#94a3b8;margin-bottom:0.7rem;">
                    Only applied when the source data doesn't already carry its own values (e.g. a
                    round-tripped <code>edi.parse</code> result keeps its original sender/receiver IDs).
                    GS Sender/Receiver default to the ISA values below when left blank.
                </div>
                <div class="config-group" style="margin-bottom:0.7rem;">
                    <label style="font-size:0.72rem;color:#64748b;display:block;margin-bottom:0.3rem;">ISA Sender ID</label>
                    <input id="ediBuildIsaSenderId" type="text" class="form-control form-control-sm" value="${isaSenderId}" style="font-family:monospace;font-size:0.8rem;">
                </div>
                <div class="config-group" style="margin-bottom:0.7rem;">
                    <label style="font-size:0.72rem;color:#64748b;display:block;margin-bottom:0.3rem;">ISA Receiver ID</label>
                    <input id="ediBuildIsaReceiverId" type="text" class="form-control form-control-sm" value="${isaReceiverId}" style="font-family:monospace;font-size:0.8rem;">
                </div>
                <div class="config-group" style="margin-bottom:0.7rem;">
                    <label style="font-size:0.72rem;color:#64748b;display:block;margin-bottom:0.3rem;">GS Sender Code (optional)</label>
                    <input id="ediBuildGsSenderCode" type="text" class="form-control form-control-sm" value="${gsSenderCode}" style="font-family:monospace;font-size:0.8rem;">
                </div>
                <div class="config-group">
                    <label style="font-size:0.72rem;color:#64748b;display:block;margin-bottom:0.3rem;">GS Receiver Code (optional)</label>
                    <input id="ediBuildGsReceiverCode" type="text" class="form-control form-control-sm" value="${gsReceiverCode}" style="font-family:monospace;font-size:0.8rem;">
                </div>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const get = id => form.querySelector('#' + id);
        const sourceEl = get('ediBuildSourceField');
        const outputEl = get('ediBuildOutputField');
        const txSetEl = get('ediBuildTransactionSet');
        const isaSenderEl = get('ediBuildIsaSenderId');
        const isaReceiverEl = get('ediBuildIsaReceiverId');
        const gsSenderEl = get('ediBuildGsSenderCode');
        const gsReceiverEl = get('ediBuildGsReceiverCode');

        if (sourceEl) step.config.sourceField = sourceEl.value.trim() || 'parsedEDI';
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'ediX12';
        if (txSetEl) step.config.transactionSet = txSetEl.value || '835';
        if (isaSenderEl) step.config.isaSenderId = isaSenderEl.value.trim();
        if (isaReceiverEl) step.config.isaReceiverId = isaReceiverEl.value.trim();
        if (gsSenderEl) step.config.gsSenderCode = gsSenderEl.value.trim();
        if (gsReceiverEl) step.config.gsReceiverCode = gsReceiverEl.value.trim();
    }

    destroy() {
        this._ac.abort();
    }
}

// ── EdiValidateStepBuilder ───────────────────────────────────────────────────
// Config: { sourceField, outputField,
//           customRules: [{segmentId, type, positions:[key,...]}],
//           disabledRules: [{segmentId, type, positions:[pos,...]}] }
//
// customRules reuses the SAME P/C/L/R/E SyntaxRule model edi/validator.go
// already implements for the OOB spec rules (see edi/schema_types.go's
// SyntaxRule doc comment) — a user's own rule is layered on top of the spec
// rules, not a second rule language. Positions are picked here by ELEMENT
// KEY (e.g. "senderDFIIdentificationNumber"), never raw numeric position —
// edi_validate_executor.go translates key -> position server-side against
// the loaded schema. Scoped to a segment's plain Elements only in phase 1
// (not intra-segment Repeats groups like CAS's reason/amount/qty trios,
// which already have their own OOB rules and would need a repeat-index
// picker this first pass doesn't build) — a named, deliberate limitation,
// not an oversight.
//
// disabledRules selectively suppresses individual OOB (schema-defined)
// SyntaxRules for THIS step only — the shared schema itself is never
// edited. Unlike customRules, positions here are the RAW position strings
// (e.g. "06") the segments catalog already returns per OOB rule, not
// element keys — SyntaxRule has no separate ID field, so (segmentId, type,
// positions) exactly as returned by GET /api/edi/schema/segments IS a
// rule's identity, and round-tripping it verbatim avoids inventing a
// second identifier scheme. The UI still displays the resolved element
// keys (segment.syntaxRules[i].elementKeys), never raw positions, to the
// user — only the config payload carries positions.

// Order-independent set comparison, mirroring
// edi_validate_executor.go's own positionsEqual exactly (a disable request
// round-trips the same positions array the segments API returned, but
// comparing as a set rather than requiring identical order is a cheap,
// harmless robustness margin).
function ediPositionsEqual(a, b) {
    const aa = Array.isArray(a) ? a : [];
    const bb = Array.isArray(b) ? b : [];
    if (aa.length !== bb.length) return false;
    const remaining = new Map();
    aa.forEach(v => remaining.set(v, (remaining.get(v) || 0) + 1));
    for (const v of bb) {
        const count = remaining.get(v) || 0;
        if (count === 0) return false;
        remaining.set(v, count - 1);
    }
    return true;
}

const EDI_RULE_TYPES = [
    { value: 'P', label: 'Paired', description: 'If any selected field is present, all must be.' },
    { value: 'C', label: 'Conditional', description: 'If the FIRST selected field is present, the rest become required.' },
    { value: 'L', label: 'List Conditional', description: 'If any field AFTER the first is present, the first becomes required.' },
    { value: 'R', label: 'Required (at least one)', description: 'At least one of the selected fields must be present.' },
    { value: 'E', label: 'Exclusion', description: 'At most one of the selected fields may be present.' },
];

class EdiValidateStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
        this._step = null;
        this._segmentsCatalog = null; // [{id, name, elements:[{pos,key,name,dataType}], repeats:[...], syntaxRules:[{type,positions,elementKeys}]}]

        window._ediValidateBuilder = this;
    }

    render(step) {
        this._step = step;
        if (!step.config) step.config = {};
        if (!Array.isArray(step.config.customRules)) step.config.customRules = [];
        if (!Array.isArray(step.config.disabledRules)) step.config.disabledRules = [];

        this._loadSegmentsCatalog();

        const html = this._renderAll();
        return html;
    }

    _loadSegmentsCatalog() {
        if (this._segmentsCatalog) return;
        fetch('/api/edi/schema/segments', { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.segments) return;
                this._segmentsCatalog = data.segments.slice().sort((a, b) => a.id.localeCompare(b.id));
                this._rerender();
            })
            .catch(() => {}); // AbortError on destroy is expected
    }

    _segmentByID(id) {
        return (this._segmentsCatalog || []).find(s => s.id === id) || null;
    }

    _renderAll() {
        const cfg = this._step.config;
        const sourceField = ediEsc(cfg.sourceField || 'raw');
        const outputField = ediEsc(cfg.outputField || 'ediValidation');

        return `
        <div id="ediValidateBuilder" class="edi-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Validate step.</strong> Re-checks raw EDI content against the base X12 5010
                standard. Two severities: a malformed field value (wrong type/length) is an
                <strong>error</strong> and blocks <code>valid</code>; a business-rule mismatch (a
                relational constraint like "if X is present, Y is required too") is a
                <strong>warning</strong> and never blocks anything — a pipeline is always free to
                ignore warnings. Add your own rules below to layer your trading partner's own
                requirements on top of the base standard.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="ediValidateSourceField" type="text" class="form-control form-control-sm"
                    value="${sourceField}" placeholder="raw"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field containing the raw X12 EDI string (not the already-parsed JSON — this step re-parses independently).</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ediValidateOutputField" type="text" class="form-control form-control-sm"
                    value="${outputField}" placeholder="ediValidation"
                    style="font-family:monospace;font-size:0.82rem;">
            </div>
            <div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;">
                <div style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;margin-bottom:0.4rem;">OOB Validation Rules</div>
                <div style="font-size:0.72rem;color:#94a3b8;margin-bottom:0.5rem;">Every business-rule constraint built into the base X12 5010 standard for this transaction set. Uncheck any rule to suppress it for THIS step only — the shared schema is never changed, and errors (malformed field values) are unaffected either way.</div>
                ${this._renderOOBRulesSection()}
            </div>
            <div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;">
                <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.6rem;">
                    <div style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;">Custom Rules (Optional)</div>
                    <button type="button" class="btn btn-sm btn-outline-primary" style="font-size:0.75rem;"
                        onclick="window._ediValidateBuilder && window._ediValidateBuilder.addRule()">+ Add Custom Rule</button>
                </div>
                ${this._segmentsCatalog ? '' : '<div style="font-size:0.78rem;color:#94a3b8;">Loading segment catalog…</div>'}
                ${cfg.customRules.map((rule, i) => this._renderRule(rule, i)).join('')}
                ${cfg.customRules.length === 0 ? '<div style="font-size:0.78rem;color:#94a3b8;">No custom rules added — only the base X12 5010 standard rules apply.</div>' : ''}
            </div>
        </div>`;
    }

    _renderOOBRulesSection() {
        if (!this._segmentsCatalog) {
            return '<div style="font-size:0.78rem;color:#94a3b8;">Loading OOB rules…</div>';
        }
        const segmentsWithRules = this._segmentsCatalog.filter(s => Array.isArray(s.syntaxRules) && s.syntaxRules.length > 0);
        if (segmentsWithRules.length === 0) {
            return '<div style="font-size:0.78rem;color:#94a3b8;">No OOB business-rule constraints in this schema.</div>';
        }
        const disabled = this._step.config.disabledRules || [];
        return segmentsWithRules.map(seg => `
            <div style="margin-bottom:0.7rem;">
                <div style="font-size:0.78rem;font-weight:600;color:#334155;margin-bottom:0.25rem;"><code>${ediEsc(seg.id)}</code>${seg.name ? ' — ' + ediEsc(seg.name) : ''}</div>
                ${seg.syntaxRules.map((r, i) => {
                    const isDisabled = disabled.some(d => d.segmentId === seg.id && d.type === r.type && ediPositionsEqual(d.positions, r.positions));
                    const typeInfo = EDI_RULE_TYPES.find(t => t.value === r.type) || {};
                    const fieldsLabel = ediEsc((r.elementKeys && r.elementKeys.length ? r.elementKeys : r.positions || []).join(' + '));
                    return `
                    <label style="display:flex;align-items:flex-start;gap:0.4rem;font-size:0.78rem;padding:0.15rem 0 0.15rem 0.75rem;cursor:pointer;">
                        <input type="checkbox" ${isDisabled ? '' : 'checked'} style="margin-top:0.2rem;"
                            onchange="window._ediValidateBuilder && window._ediValidateBuilder.toggleOOBRule('${ediEsc(seg.id)}', ${i}, this.checked)">
                        <span><strong>${ediEsc(typeInfo.label || r.type)}</strong> — ${fieldsLabel}<br>
                            <span style="color:#94a3b8;">${ediEsc(typeInfo.description || '')}</span></span>
                    </label>`;
                }).join('')}
            </div>
        `).join('');
    }

    _renderRule(rule, index) {
        const catalog = this._segmentsCatalog || [];
        const segOptions = ['<option value="">-- Select Segment --</option>']
            .concat(catalog.map(s => `<option value="${ediEsc(s.id)}" ${s.id === rule.segmentId ? 'selected' : ''}>${ediEsc(s.id)}${s.name ? ' — ' + ediEsc(s.name) : ''}</option>`))
            .join('');

        const typeOptions = EDI_RULE_TYPES.map(t =>
            `<option value="${t.value}" ${t.value === rule.type ? 'selected' : ''}>${ediEsc(t.label)}</option>`
        ).join('');
        const typeDesc = (EDI_RULE_TYPES.find(t => t.value === rule.type) || {}).description || '';

        const seg = this._segmentByID(rule.segmentId);
        const positions = Array.isArray(rule.positions) ? rule.positions : [];
        const checkboxesHTML = !seg ? '<div style="font-size:0.75rem;color:#94a3b8;">Select a segment to choose fields.</div>' :
            (seg.elements || []).map(el => `
                <label style="display:flex;align-items:center;gap:0.4rem;font-size:0.78rem;padding:0.15rem 0;cursor:pointer;">
                    <input type="checkbox" ${positions.includes(el.key) ? 'checked' : ''}
                        onchange="window._ediValidateBuilder && window._ediValidateBuilder.toggleRulePosition(${index}, '${ediEsc(el.key)}', this.checked)">
                    <span><code style="font-size:0.75rem;">${ediEsc(el.pos)}</code> ${ediEsc(el.key)}${el.name ? ' — ' + ediEsc(el.name) : ''}</span>
                </label>
            `).join('');

        return `
        <div style="border:1px solid #e2e8f0;border-radius:6px;padding:0.7rem 0.8rem;margin-bottom:0.6rem;background:#f8fafc;">
            <div style="display:flex;justify-content:space-between;align-items:flex-start;gap:0.5rem;">
                <div style="flex:1;">
                    <div style="display:flex;gap:0.5rem;margin-bottom:0.5rem;">
                        <select style="flex:1;font-size:0.8rem;" class="form-select form-select-sm"
                            onchange="window._ediValidateBuilder && window._ediValidateBuilder.onRuleSegmentChange(${index}, this.value)">
                            ${segOptions}
                        </select>
                        <select style="flex:1;font-size:0.8rem;" class="form-select form-select-sm"
                            onchange="window._ediValidateBuilder && window._ediValidateBuilder.onRuleTypeChange(${index}, this.value)">
                            ${typeOptions}
                        </select>
                    </div>
                    <div style="font-size:0.72rem;color:#94a3b8;margin-bottom:0.5rem;">${ediEsc(typeDesc)}</div>
                    <div>${checkboxesHTML}</div>
                </div>
                <button type="button" class="btn btn-sm btn-outline-danger" style="font-size:0.72rem;padding:0.15rem 0.5rem;"
                    onclick="window._ediValidateBuilder && window._ediValidateBuilder.removeRule(${index})">&times;</button>
            </div>
        </div>`;
    }

    // ── interaction handlers (called from inline onclick/onchange) ──────────

    // Toggles one OOB rule's enabled state. ruleIndex addresses
    // segment.syntaxRules[ruleIndex] (looked up fresh from the cached
    // catalog, not passed by value) rather than embedding the rule's own
    // (type, positions) array inside an inline onchange attribute, which
    // would need fragile HTML/JS double-escaping for an array value.
    toggleOOBRule(segmentId, ruleIndex, checked) {
        const seg = this._segmentByID(segmentId);
        const rule = seg && seg.syntaxRules && seg.syntaxRules[ruleIndex];
        if (!rule) return;

        this._step.config.disabledRules = this._step.config.disabledRules || [];
        const idx = this._step.config.disabledRules.findIndex(d =>
            d.segmentId === segmentId && d.type === rule.type && ediPositionsEqual(d.positions, rule.positions));

        if (checked) {
            // Re-enabling: remove the suppression entry, if any.
            if (idx !== -1) this._step.config.disabledRules.splice(idx, 1);
        } else {
            // Disabling: record it (avoid duplicate entries).
            if (idx === -1) this._step.config.disabledRules.push({ segmentId, type: rule.type, positions: rule.positions.slice() });
        }
        // No full rerender needed -- same rationale as toggleRulePosition.
    }

    addRule() {
        this._step.config.customRules.push({ segmentId: '', type: 'P', positions: [] });
        this._rerender();
    }

    removeRule(index) {
        this._step.config.customRules.splice(index, 1);
        this._rerender();
    }

    onRuleSegmentChange(index, segmentId) {
        const rule = this._step.config.customRules[index];
        if (!rule) return;
        rule.segmentId = segmentId;
        rule.positions = []; // a new segment's elements are a different set entirely
        this._rerender();
    }

    onRuleTypeChange(index, type) {
        const rule = this._step.config.customRules[index];
        if (!rule) return;
        rule.type = type;
        this._rerender();
    }

    toggleRulePosition(index, key, checked) {
        const rule = this._step.config.customRules[index];
        if (!rule) return;
        rule.positions = rule.positions || [];
        if (checked) {
            if (!rule.positions.includes(key)) rule.positions.push(key);
        } else {
            rule.positions = rule.positions.filter(p => p !== key);
        }
        // No full rerender needed for a checkbox toggle — avoids losing focus.
    }

    _rerender() {
        const root = document.getElementById('ediValidateBuilder');
        if (!root || !root.parentElement) return;
        root.outerHTML = this._renderAll();
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const sourceEl = form.querySelector('#ediValidateSourceField');
        const outputEl = form.querySelector('#ediValidateOutputField');

        if (sourceEl) step.config.sourceField = sourceEl.value.trim() || 'raw';
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'ediValidation';
        // customRules/disabledRules already live in step.config, kept in sync
        // by the interaction handlers above — nothing further to read from the DOM.
        step.config.customRules = (step.config.customRules || []).filter(r => r.segmentId && r.positions && r.positions.length > 0);
        step.config.disabledRules = step.config.disabledRules || [];
    }

    destroy() {
        this._ac.abort();
        if (window._ediValidateBuilder === this) {
            window._ediValidateBuilder = null;
        }
    }
}

// ── EdiMapToCanonicalStepBuilder ─────────────────────────────────────────────
// Config: { outputField, header: [{segmentId,elementKey,sourcePath,transform,literalValue}],
//           loops: [{loopId, rowsPath, fields:[...], loops:[...recursive...]}] }
//
// The loops array mirrors edi/schema_types.go's own X12LoopDef recursive
// nesting exactly (see this step's own Go executor doc comment) — one
// recursive UI section for one recursive config shape, addressed here by a
// dot-joined loop-ID path (e.g. "2000.2100.2110", loop IDs are unique within
// one transaction set's own tree) rather than array indices, so the UI stays
// stable across add/remove/reorder elsewhere in the tree.

class EdiMapToCanonicalStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
        this._step = null;
        this._segmentsCatalog = null; // [{id, name, elements:[...]}]
        this._loopsCatalog = null;    // [{id, name, repeat, segmentIds, loops:[...]}] — the 835 loop tree

        window._ediMapBuilder = this;
    }

    render(step) {
        this._step = step;
        if (!step.config) step.config = {};
        const cfg = step.config;
        if (!cfg.outputField) cfg.outputField = 'canonicalEDI';
        if (!Array.isArray(cfg.header)) cfg.header = [];
        if (!Array.isArray(cfg.loops)) cfg.loops = [];

        this._loadSegmentsCatalog();
        this._loadLoopsCatalog();

        return this._renderAll();
    }

    _loadSegmentsCatalog() {
        if (this._segmentsCatalog) return;
        fetch('/api/edi/schema/segments', { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.segments) return;
                this._segmentsCatalog = data.segments.slice().sort((a, b) => a.id.localeCompare(b.id));
                this._rerender();
            })
            .catch(() => {});
    }

    _loadLoopsCatalog() {
        if (this._loopsCatalog) return;
        fetch('/api/edi/schema/transaction-sets/835/loops', { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.loops) return;
                this._loopsCatalog = data.loops;
                this._rerender();
            })
            .catch(() => {});
    }

    _segmentByID(id) {
        return (this._segmentsCatalog || []).find(s => s.id === id) || null;
    }

    // Finds (or creates, when create=true) the config loop object at a
    // dot-joined idPath, walking/creating nested `loops` arrays to match the
    // schema tree's own shape. Returns {array, index} — array is the array
    // CONTAINING the target node (so a caller can splice it out), index is
    // its position (-1 when not found and create=false).
    _configLoopArrayAndIndex(idPath, create) {
        const parts = idPath.split('.');
        let array = this._step.config.loops;
        let idx = -1;
        for (let i = 0; i < parts.length; i++) {
            idx = array.findIndex(n => n.loopId === parts[i]);
            if (idx === -1) {
                if (!create) return { array, index: -1 };
                array.push({ loopId: parts[i], fields: [], loops: [] });
                idx = array.length - 1;
            }
            if (i === parts.length - 1) break;
            const node = array[idx];
            if (!Array.isArray(node.loops)) node.loops = [];
            array = node.loops;
        }
        return { array, index: idx };
    }

    _configLoopAt(idPath) {
        const { array, index } = this._configLoopArrayAndIndex(idPath, false);
        return index === -1 ? null : array[index];
    }

    _segOptionsHTML(segmentIds, selectedId) {
        return ['<option value="">-- Segment --</option>']
            .concat((segmentIds || []).map(sid =>
                `<option value="${ediEsc(sid)}" ${sid === selectedId ? 'selected' : ''}>${ediEsc(sid)}</option>`))
            .join('');
    }

    _elementOptionsHTML(segmentId, selectedKey) {
        const seg = this._segmentByID(segmentId);
        if (!seg) return '<option value="">-- Select Segment First --</option>';
        return ['<option value="">-- Element --</option>']
            .concat((seg.elements || []).map(el =>
                `<option value="${ediEsc(el.key)}" ${el.key === selectedKey ? 'selected' : ''}>${ediEsc(el.pos)} ${ediEsc(el.key)}</option>`))
            .join('');
    }

    _renderAll() {
        const cfg = this._step.config;
        return `
        <div id="ediMapToCanonicalBuilder" class="edi-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Map to Canonical step.</strong> No-code field mapping from any source shape
                (CSV, DB rows, generic JSON) into the canonical interchange/header/loops/trailer JSON
                <code>edi.build</code> consumes — the on-ramp for building an 835 from data that never
                went through <code>edi.parse</code>.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ediMapOutputField" type="text" class="form-control form-control-sm"
                    value="${ediEsc(cfg.outputField)}" placeholder="canonicalEDI"
                    style="font-family:monospace;font-size:0.82rem;">
            </div>
            ${this._renderHeaderSection()}
            ${this._renderLoopsSection()}
        </div>`;
    }

    // ── Header fields (flat) ──────────────────────────────────────────────

    _renderHeaderSection() {
        const cfg = this._step.config;
        if (!this._segmentsCatalog) {
            return `<div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;font-size:0.78rem;color:#94a3b8;">Loading segment catalog…</div>`;
        }
        return `
        <div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.5rem;">
                <div style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;">Header Fields</div>
                <button type="button" class="btn btn-sm btn-outline-primary" style="font-size:0.75rem;"
                    onclick="window._ediMapBuilder && window._ediMapBuilder.addHeaderField()">+ Add Field</button>
            </div>
            <div style="font-size:0.72rem;color:#94a3b8;margin-bottom:0.5rem;">Flat, single-instance header segments (ST, BPR, TRN, CUR, REF, DTM).</div>
            ${cfg.header.map((f, i) => this._renderFieldRow('header', null, f, i)).join('')}
            ${cfg.header.length === 0 ? '<div style="font-size:0.78rem;color:#94a3b8;">No header fields mapped.</div>' : ''}
        </div>`;
    }

    // ── Loops (recursive) ─────────────────────────────────────────────────

    _renderLoopsSection() {
        if (!this._loopsCatalog) {
            return `<div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;font-size:0.78rem;color:#94a3b8;">Loading loop catalog…</div>`;
        }
        return `
        <div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;">
            <div style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;margin-bottom:0.3rem;">Loops</div>
            <div style="font-size:0.72rem;color:#94a3b8;margin-bottom:0.5rem;">Check a loop to map fields into it. Set Rows Path on a repeating loop to map one entry per source row.</div>
            ${this._loopsCatalog.map(node => this._renderLoopNode(node, node.id)).join('')}
        </div>`;
    }

    _renderLoopNode(schemaNode, idPath) {
        const depth = idPath.split('.').length;
        const included = !!this._configLoopAt(idPath);
        const repeatHint = schemaNode.repeat && schemaNode.repeat !== '1' ? ' <span style="color:#94a3b8;">(repeats)</span>' : '';
        const children = (schemaNode.loops || [])
            .map(child => this._renderLoopNode(child, idPath + '.' + child.id))
            .join('');

        return `
        <div style="margin-left:${depth > 1 ? '1.25rem' : '0'};${depth > 1 ? 'border-left:2px solid #e2e8f0;padding-left:0.75rem;' : ''}margin-bottom:0.4rem;">
            <label style="display:flex;align-items:center;gap:0.5rem;font-size:0.82rem;cursor:pointer;">
                <input type="checkbox" ${included ? 'checked' : ''}
                    onchange="window._ediMapBuilder && window._ediMapBuilder.toggleLoop('${idPath}', this.checked)">
                <strong>${ediEsc(schemaNode.id)}</strong> ${ediEsc(schemaNode.name || '')}${repeatHint}
            </label>
            ${included ? this._renderLoopCard(schemaNode, idPath) : ''}
            ${children}
        </div>`;
    }

    _renderLoopCard(schemaNode, idPath) {
        const node = this._configLoopAt(idPath);
        if (!node) return '';
        const fields = Array.isArray(node.fields) ? node.fields : (node.fields = []);
        return `
        <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:6px;padding:0.6rem 0.7rem;margin:0.4rem 0 0.6rem;">
            <div class="config-group" style="margin-bottom:0.5rem;">
                <label style="font-size:0.7rem;color:#64748b;display:block;margin-bottom:0.2rem;">Rows Path (blank = single-instance loop)</label>
                <input type="text" value="${ediEsc(node.rowsPath || '')}" placeholder="e.g. claims"
                    style="width:100%;font-family:monospace;font-size:0.78rem;"
                    onblur="window._ediMapBuilder && window._ediMapBuilder.updateLoopRowsPath('${idPath}', this.value)">
            </div>
            ${fields.map((f, fi) => this._renderFieldRow('loop', idPath, f, fi, schemaNode.segmentIds)).join('')}
            <button type="button" class="btn btn-sm btn-outline-primary" style="font-size:0.72rem;"
                onclick="window._ediMapBuilder && window._ediMapBuilder.addLoopField('${idPath}')">+ Add Field</button>
        </div>`;
    }

    // ── shared field-row renderer (header rows and loop-card rows) ──────────

    _renderFieldRow(kind, idPath, f, index, restrictSegmentIds) {
        const segmentIds = restrictSegmentIds || (this._segmentsCatalog || []).map(s => s.id);
        const prefix = kind === 'header'
            ? `window._ediMapBuilder.updateHeaderField(${index}`
            : `window._ediMapBuilder.updateLoopField('${idPath}', ${index}`;
        const segChangePrefix = kind === 'header'
            ? `window._ediMapBuilder.onHeaderSegmentChange(${index}`
            : `window._ediMapBuilder.onLoopFieldSegmentChange('${idPath}', ${index}`;
        const removePrefix = kind === 'header'
            ? `window._ediMapBuilder.removeHeaderField(${index})`
            : `window._ediMapBuilder.removeLoopField('${idPath}', ${index})`;

        return `
        <div style="display:flex;gap:0.4rem;align-items:center;margin-bottom:0.4rem;flex-wrap:wrap;">
            <select style="flex:1;min-width:110px;font-size:0.76rem;" class="form-select form-select-sm"
                onchange="${segChangePrefix}, this.value)">
                ${this._segOptionsHTML(segmentIds, f.segmentId)}
            </select>
            <select style="flex:1;min-width:130px;font-size:0.76rem;" class="form-select form-select-sm"
                onchange="${prefix}, 'elementKey', this.value)">
                ${this._elementOptionsHTML(f.segmentId, f.elementKey)}
            </select>
            <input type="text" placeholder="source path" value="${ediEsc(f.sourcePath || '')}"
                style="flex:1;min-width:100px;font-family:monospace;font-size:0.76rem;" class="form-control form-control-sm"
                onblur="${prefix}, 'sourcePath', this.value)">
            <select style="width:130px;font-size:0.76rem;" class="form-select form-select-sm"
                onchange="${prefix}, 'transform', this.value)">
                ${ediTransformOptionsHTML(f.transform)}
            </select>
            <input type="text" placeholder="literal (if no source)" value="${ediEsc(f.literalValue || '')}"
                style="flex:1;min-width:90px;font-family:monospace;font-size:0.76rem;" class="form-control form-control-sm"
                onblur="${prefix}, 'literalValue', this.value)">
            <button type="button" class="btn btn-sm btn-outline-danger" style="font-size:0.7rem;padding:0.1rem 0.4rem;"
                onclick="${removePrefix}">&times;</button>
        </div>`;
    }

    // ── interaction handlers ─────────────────────────────────────────────

    toggleLoop(idPath, checked) {
        if (checked) {
            this._configLoopArrayAndIndex(idPath, true);
        } else {
            const { array, index } = this._configLoopArrayAndIndex(idPath, false);
            if (index !== -1) array.splice(index, 1);
        }
        this._rerender();
    }

    updateLoopRowsPath(idPath, value) {
        const node = this._configLoopAt(idPath);
        if (node) node.rowsPath = value;
    }

    addLoopField(idPath) {
        const node = this._configLoopAt(idPath);
        if (!node) return;
        if (!Array.isArray(node.fields)) node.fields = [];
        node.fields.push({ segmentId: '', elementKey: '', sourcePath: '', transform: '', literalValue: '' });
        this._rerender();
    }

    removeLoopField(idPath, index) {
        const node = this._configLoopAt(idPath);
        if (!node) return;
        node.fields.splice(index, 1);
        this._rerender();
    }

    onLoopFieldSegmentChange(idPath, index, segmentId) {
        const node = this._configLoopAt(idPath);
        if (!node || !node.fields[index]) return;
        node.fields[index].segmentId = segmentId;
        node.fields[index].elementKey = '';
        this._rerender();
    }

    updateLoopField(idPath, index, key, value) {
        const node = this._configLoopAt(idPath);
        if (!node || !node.fields[index]) return;
        node.fields[index][key] = value;
        this._rerender();
    }

    addHeaderField() {
        this._step.config.header.push({ segmentId: '', elementKey: '', sourcePath: '', transform: '', literalValue: '' });
        this._rerender();
    }

    removeHeaderField(index) {
        this._step.config.header.splice(index, 1);
        this._rerender();
    }

    onHeaderSegmentChange(index, segmentId) {
        const h = this._step.config.header[index];
        if (!h) return;
        h.segmentId = segmentId;
        h.elementKey = '';
        this._rerender();
    }

    updateHeaderField(index, key, value) {
        const h = this._step.config.header[index];
        if (!h) return;
        h[key] = value;
        this._rerender();
    }

    _rerender() {
        const root = document.getElementById('ediMapToCanonicalBuilder');
        if (!root || !root.parentElement) return;
        root.outerHTML = this._renderAll();
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const outputEl = form.querySelector('#ediMapOutputField');
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'canonicalEDI';

        // header/loops already live in step.config, kept in sync by the
        // interaction handlers above. Header rows missing both a source and
        // literal value are dropped (they'd write nothing anyway); loop
        // entries are NOT filtered the same way — an empty-fields loop is a
        // valid pure nesting container for its own children.
        step.config.header = (step.config.header || []).filter(h => h.segmentId && h.elementKey && (h.sourcePath || h.literalValue));
    }

    destroy() {
        this._ac.abort();
        if (window._ediMapBuilder === this) {
            window._ediMapBuilder = null;
        }
    }
}

// ── registration ─────────────────────────────────────────────────────────

StepBuilderRegistry.register('edi.parse', EdiParseStepBuilder);
StepBuilderRegistry.register('edi.validate', EdiValidateStepBuilder);
StepBuilderRegistry.register('edi.map_to_canonical', EdiMapToCanonicalStepBuilder);
StepBuilderRegistry.register('edi.build', EdiBuildStepBuilder);
