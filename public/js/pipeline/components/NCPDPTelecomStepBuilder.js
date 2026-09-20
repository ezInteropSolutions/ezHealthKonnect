/**
 * NCPDPTelecomStepBuilder — step config builders for NCPDP Telecommunication
 * D.0 (real-time pharmacy claims) pipeline steps (Phase 1: B1 Claim Billing
 * request/response).
 *
 * Registers four step types with StepBuilderRegistry:
 *   - ncpdptelecom.parse             Parse raw D.0 transmission to ParsedJSON
 *   - ncpdptelecom.validate          Check a parsed transmission against the
 *                                     schema's own required field/segment flags
 *   - ncpdptelecom.map_to_canonical  No-code field mapping from any source shape
 *                                     into ncpdptelecom.build's canonical JSON
 *   - ncpdptelecom.build             Build a complete D.0 transmission
 *
 * Builder contract (StepBuilderRegistry), matching NCPDPStepBuilder.js/
 * EDIStepBuilder.js exactly:
 *   render(step)         → string  HTML for the properties panel form tab
 *   collectConfig(step)  → void    reads DOM, writes into step.config
 *   destroy()            → void    tears down event listeners / AC refs
 *
 * D.0's own map_to_canonical config is FLATTER than NCPDP SCRIPT's own
 * (which has arbitrarily-nested groups): a segment's own fields are a flat
 * list, and there is exactly ONE repeating construct (transaction groups —
 * one GS-delimited cluster per claim/drug), not a nestable tree. See
 * ncpdptelecom_map_to_canonical_executor.go's own doc comment for the exact
 * config shape this UI mirrors.
 */

// ── shared helpers (file-scope, used by every builder below) ───────────────

function ncpdpTelecomEsc(s) {
    return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

const NCPDP_TELECOM_TRANSFORM_OPTIONS = [
    { value: '', label: '(none)' },
    { value: 'trim', label: 'Trim whitespace' },
    { value: 'uppercase', label: 'Uppercase' },
    { value: 'lowercase', label: 'Lowercase' },
];

function ncpdpTelecomTransformOptionsHTML(selected) {
    return NCPDP_TELECOM_TRANSFORM_OPTIONS.map(o =>
        `<option value="${ncpdpTelecomEsc(o.value)}" ${o.value === (selected || '') ? 'selected' : ''}>${ncpdpTelecomEsc(o.label)}</option>`
    ).join('');
}

function ncpdpTelecomDirectionOptionsHTML(selected) {
    const opts = [
        { value: '', label: '(auto-detect)' },
        { value: 'request', label: 'Request' },
        { value: 'response', label: 'Response' },
    ];
    return opts.map(o =>
        `<option value="${ncpdpTelecomEsc(o.value)}" ${o.value === (selected || '') ? 'selected' : ''}>${ncpdpTelecomEsc(o.label)}</option>`
    ).join('');
}

// Phase 1 (B1 Claim Billing) + the documented sourcing pass that added B2
// (Reversal), B3 (Rebill), and E1 (Eligibility Verification) — see
// CLAUDE.md's own "NCPDP Telecommunication D.0 — B2/B3/E1" section. B2/E1
// have no transaction-group/claim-line-item concept in their own response
// (E1) or request (B2's own Claim segment is narrower) — the config UI
// doesn't need to know this, since transactionCode alone selects the right
// schema-driven segment set server-side. E2/D0/D1/the batch standard remain
// out of scope (no real, byte-level-confirmed source found for those yet).
const NCPDP_TELECOM_TRANSACTION_CODES = ['B1', 'B2', 'B3', 'E1'];

function ncpdpTelecomTransactionCodeOptionsHTML(selected) {
    return NCPDP_TELECOM_TRANSACTION_CODES.map(t =>
        `<option value="${ncpdpTelecomEsc(t)}" ${t === (selected || 'B1') ? 'selected' : ''}>${ncpdpTelecomEsc(t)}</option>`
    ).join('');
}

// ── NcpdpTelecomParseStepBuilder ─────────────────────────────────────────────
// Config: { sourceField, direction, outputField }

class NcpdpTelecomParseStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;

        const sourceField = ncpdpTelecomEsc(cfg.sourceField || 'raw');
        const outputField = ncpdpTelecomEsc(cfg.outputField || 'parsedTelecom');

        return `
        <div class="ncpdp-telecom-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Parse step.</strong> Converts a raw NCPDP Telecommunication D.0 real-time
                pharmacy claim transmission (control-character-delimited, fixed-width header) into a
                structured JSON document (transactionCode + direction + header + transmissionGroup +
                transactionGroups). Request and response share the same wire transaction code — set
                Direction explicitly when known, or leave on auto-detect (sniffed from the
                response-only segment-identifier range).
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="ncpdpTelecomParseSourceField" type="text" class="form-control form-control-sm"
                    value="${sourceField}" placeholder="raw"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field containing the raw D.0 transmission string.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Direction</label>
                <select id="ncpdpTelecomParseDirection" class="form-select form-select-sm">
                    ${ncpdpTelecomDirectionOptionsHTML(cfg.direction)}
                </select>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Request and response share the same transaction code on the wire — set this explicitly when this step only ever sees one direction.</div>
            </div>
            <div class="config-group">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ncpdpTelecomParseOutputField" type="text" class="form-control form-control-sm"
                    value="${outputField}" placeholder="parsedTelecom"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the parsed JSON will be written.</div>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const sourceEl = form.querySelector('#ncpdpTelecomParseSourceField');
        const directionEl = form.querySelector('#ncpdpTelecomParseDirection');
        const outputEl = form.querySelector('#ncpdpTelecomParseOutputField');

        if (sourceEl) step.config.sourceField = sourceEl.value.trim() || 'raw';
        if (directionEl) step.config.direction = directionEl.value || '';
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'parsedTelecom';
    }

    destroy() {
        this._ac.abort();
    }
}

// ── NcpdpTelecomValidateStepBuilder ──────────────────────────────────────────
// Config: { sourceField, direction, outputField }
//
// Deliberately simpler than EdiValidateStepBuilder — no custom-rule/disabled-
// rule mechanism yet (see ncpdptelecom/validator/validator.go's own doc
// comment: no free source of D.0's own conditional-requirement business
// rules was found for Phase 1, so the warning tier is an intentionally
// empty, ready-to-extend mechanism, not a UI surface to build yet).

class NcpdpTelecomValidateStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;

        const sourceField = ncpdpTelecomEsc(cfg.sourceField || 'parsedTelecom');
        const outputField = ncpdpTelecomEsc(cfg.outputField || 'telecomValidation');

        return `
        <div class="ncpdp-telecom-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Validate step.</strong> Checks a parsed D.0 transmission against the schema's
                own required field/segment flags. A missing or malformed REQUIRED field/segment is a
                blocking <strong>error</strong> (<code>valid=false</code>). Accepts either a prior
                <code>ncpdptelecom.parse</code> step's own output, or raw transmission text (re-parsed
                on the fly) if this step runs standalone.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="ncpdpTelecomValidateSourceField" type="text" class="form-control form-control-sm"
                    value="${sourceField}" placeholder="parsedTelecom"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field holding a prior ncpdptelecom.parse step's output — or raw transmission text.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Direction</label>
                <select id="ncpdpTelecomValidateDirection" class="form-select form-select-sm">
                    ${ncpdpTelecomDirectionOptionsHTML(cfg.direction)}
                </select>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Only used when re-parsing raw text directly (a prior parse step's own output already carries its direction).</div>
            </div>
            <div class="config-group">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ncpdpTelecomValidateOutputField" type="text" class="form-control form-control-sm"
                    value="${outputField}" placeholder="telecomValidation"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where {valid, issues:[{severity,path,message}]} will be written.</div>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const sourceEl = form.querySelector('#ncpdpTelecomValidateSourceField');
        const directionEl = form.querySelector('#ncpdpTelecomValidateDirection');
        const outputEl = form.querySelector('#ncpdpTelecomValidateOutputField');

        if (sourceEl) step.config.sourceField = sourceEl.value.trim() || 'parsedTelecom';
        if (directionEl) step.config.direction = directionEl.value || '';
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'telecomValidation';
    }

    destroy() {
        this._ac.abort();
    }
}

// ── NcpdpTelecomBuildStepBuilder ─────────────────────────────────────────────
// Config: { sourceField, transactionCode, direction, outputField }

class NcpdpTelecomBuildStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;

        const sourceField = ncpdpTelecomEsc(cfg.sourceField || 'parsedTelecom');
        const outputField = ncpdpTelecomEsc(cfg.outputField || 'ncpdpTelecom');

        return `
        <div class="ncpdp-telecom-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Build step.</strong> Builds a complete NCPDP Telecommunication D.0 transmission
                from canonical JSON. Accepts either <code>ncpdptelecom.parse</code>'s own output (a
                round trip) or <code>ncpdptelecom.map_to_canonical</code>'s output (built from any
                source shape) — both produce the same header/transmissionGroup/transactionGroups shape
                this step expects.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="ncpdpTelecomBuildSourceField" type="text" class="form-control form-control-sm"
                    value="${sourceField}" placeholder="parsedTelecom"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field holding canonical header/transmissionGroup/transactionGroups JSON.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ncpdpTelecomBuildOutputField" type="text" class="form-control form-control-sm"
                    value="${outputField}" placeholder="ncpdpTelecom"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the built transmission text will be written.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Transaction Code</label>
                <select id="ncpdpTelecomBuildTransactionCode" class="form-select form-select-sm">
                    ${ncpdpTelecomTransactionCodeOptionsHTML(cfg.transactionCode)}
                </select>
            </div>
            <div class="config-group">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Direction</label>
                <select id="ncpdpTelecomBuildDirection" class="form-select form-select-sm">
                    <option value="request" ${(cfg.direction || 'request') === 'request' ? 'selected' : ''}>Request</option>
                    <option value="response" ${cfg.direction === 'response' ? 'selected' : ''}>Response</option>
                </select>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Which segment-identifier range (01-16 request, 20-29 response) to emit.</div>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const get = id => form.querySelector('#' + id);
        const sourceEl = get('ncpdpTelecomBuildSourceField');
        const outputEl = get('ncpdpTelecomBuildOutputField');
        const codeEl = get('ncpdpTelecomBuildTransactionCode');
        const directionEl = get('ncpdpTelecomBuildDirection');

        if (sourceEl) step.config.sourceField = sourceEl.value.trim() || 'parsedTelecom';
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'ncpdpTelecom';
        if (codeEl) step.config.transactionCode = codeEl.value || 'B1';
        if (directionEl) step.config.direction = directionEl.value || 'request';
    }

    destroy() {
        this._ac.abort();
    }
}

// ── NcpdpTelecomMapToCanonicalStepBuilder ────────────────────────────────────
// Config: { outputField, transactionCode, direction,
//           headerFields: [{fieldKey,sourcePath,transform,literalValue}],
//           transmissionGroupSegments: [{segmentKey, fields:[...]}],
//           transactionGroupRowsPath,
//           transactionGroupSegments: [{segmentKey, fields:[...]}] }
//
// Flatter than NcpdpMapToCanonicalStepBuilder/EdiMapToCanonicalStepBuilder's
// own recursive group trees — D.0 segments have no nested sub-groups at all
// (a flat field list per segment), and there is exactly ONE repeating
// construct (transaction groups), addressed by a single rowsPath rather
// than a per-node rowsPath at arbitrary nesting depth.

class NcpdpTelecomMapToCanonicalStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
        this._step = null;
        this._segmentsByKey = null; // { [segmentKey]: {key,identifier,name,fields:[{key,fieldId,name,dataType,required}]} }
        this._headerFieldsCatalog = null; // [{key,name,dataType,required}] — fixed-width header fields

        window._ncpdpTelecomMapBuilder = this;
    }

    render(step) {
        this._step = step;
        if (!step.config) step.config = {};
        const cfg = step.config;
        if (!cfg.outputField) cfg.outputField = 'parsedTelecom';
        if (!cfg.transactionCode) cfg.transactionCode = 'B1';
        if (!cfg.direction) cfg.direction = 'request';
        if (!Array.isArray(cfg.headerFields)) cfg.headerFields = [];
        if (!Array.isArray(cfg.transmissionGroupSegments)) cfg.transmissionGroupSegments = [];
        if (!Array.isArray(cfg.transactionGroupSegments)) cfg.transactionGroupSegments = [];

        this._loadSegmentsCatalog();

        return this._renderAll();
    }

    _loadSegmentsCatalog() {
        if (this._segmentsByKey) return;
        fetch('/api/ncpdp-telecom/schema/segments', { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.segments) return;
                this._segmentsByKey = {};
                data.segments.forEach(s => { this._segmentsByKey[s.key] = s; });
                this._headerFieldsCatalog = data.headerFields || [];
                this._rerender();
            })
            .catch(() => {}); // AbortError on destroy is expected
    }

    _segmentKeysHTML(selectedKey) {
        const keys = this._segmentsByKey ? Object.keys(this._segmentsByKey).sort() : [];
        return ['<option value="">-- Segment --</option>']
            .concat(keys.map(k => {
                const seg = this._segmentsByKey[k];
                return `<option value="${ncpdpTelecomEsc(k)}" ${k === selectedKey ? 'selected' : ''}>${ncpdpTelecomEsc(k)} (${ncpdpTelecomEsc(seg.identifier)})${seg.name ? ' — ' + ncpdpTelecomEsc(seg.name) : ''}</option>`;
            }))
            .join('');
    }

    _fieldKeyOptionsHTML(fieldsCatalog, selectedKey) {
        return ['<option value="">-- Field --</option>']
            .concat((fieldsCatalog || []).map(f =>
                `<option value="${ncpdpTelecomEsc(f.key)}" ${f.key === selectedKey ? 'selected' : ''}>${ncpdpTelecomEsc(f.key)}${f.required ? ' *' : ''}${f.name ? ' — ' + ncpdpTelecomEsc(f.name) : ''}</option>`))
            .join('');
    }

    _renderAll() {
        const cfg = this._step.config;
        return `
        <div id="ncpdpTelecomMapToCanonicalBuilder" class="ncpdp-telecom-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Map to Canonical step.</strong> No-code field mapping from any source shape
                (CSV, DB rows, generic JSON) into the canonical header/transmissionGroup/
                transactionGroups JSON <code>ncpdptelecom.build</code> consumes — the on-ramp for
                building a D.0 transmission from data that never went through
                <code>ncpdptelecom.parse</code>.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ncpdpTelecomMapOutputField" type="text" class="form-control form-control-sm"
                    value="${ncpdpTelecomEsc(cfg.outputField)}" placeholder="parsedTelecom"
                    style="font-family:monospace;font-size:0.82rem;">
            </div>
            <div style="display:flex;gap:0.75rem;margin-bottom:1.1rem;">
                <div class="config-group" style="flex:1;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Transaction Code</label>
                    <select id="ncpdpTelecomMapTransactionCode" class="form-select form-select-sm">
                        ${ncpdpTelecomTransactionCodeOptionsHTML(cfg.transactionCode)}
                    </select>
                </div>
                <div class="config-group" style="flex:1;">
                    <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Direction</label>
                    <select id="ncpdpTelecomMapDirection" class="form-select form-select-sm">
                        <option value="request" ${cfg.direction === 'request' ? 'selected' : ''}>Request</option>
                        <option value="response" ${cfg.direction === 'response' ? 'selected' : ''}>Response</option>
                    </select>
                </div>
            </div>
            ${this._renderHeaderFieldsSection()}
            ${this._renderSegmentsSection('transmissionGroupSegments', 'Transmission Group Segments', 'Segments appearing exactly once per transmission (e.g. Patient, Insurance, Pharmacy Provider, Prescriber).', false)}
            <div class="config-group" style="margin:1rem 0 0.5rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Transaction Group Rows Path</label>
                <input id="ncpdpTelecomMapRowsPath" type="text" class="form-control form-control-sm"
                    value="${ncpdpTelecomEsc(cfg.transactionGroupRowsPath || '')}" placeholder="e.g. claims (leave blank for a single transaction group from top-level data)"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Dot-path to an array of rows, one per claim/drug being submitted — each row builds one GS-delimited transaction group.</div>
            </div>
            ${this._renderSegmentsSection('transactionGroupSegments', 'Transaction Group Segments', 'Segments that repeat as a GS-delimited cluster, one per row above (e.g. Claim, Pricing).', true)}
        </div>`;
    }

    // ── header fields (fixed-width, flat) ───────────────────────────────

    _renderHeaderFieldsSection() {
        if (!this._headerFieldsCatalog) {
            return `<div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;font-size:0.78rem;color:#94a3b8;">Loading Header Fields…</div>`;
        }
        const fields = this._step.config.headerFields;
        return `
        <div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.3rem;">
                <div style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;">Header Fields</div>
                <button type="button" class="btn btn-sm btn-outline-primary" style="font-size:0.75rem;"
                    onclick="window._ncpdpTelecomMapBuilder && window._ncpdpTelecomMapBuilder.addHeaderField()">+ Add Field</button>
            </div>
            <div style="font-size:0.72rem;color:#94a3b8;margin-bottom:0.5rem;">The fixed-width wire header (BIN number, version, transaction code, service provider ID, date of service, ...). transactionCode/version default from the Transaction Code/Version above if not mapped here.</div>
            ${fields.map((f, i) => this._renderFieldRow('header', null, f, i, this._headerFieldsCatalog)).join('')}
            ${fields.length === 0 ? '<div style="font-size:0.78rem;color:#94a3b8;">No header fields mapped (transactionCode/version will still default correctly).</div>' : ''}
        </div>`;
    }

    // ── segment lists (flat, no nesting) ────────────────────────────────

    _renderSegmentsSection(configKey, label, hint, repeating) {
        if (!this._segmentsByKey) {
            return `<div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;font-size:0.78rem;color:#94a3b8;">Loading ${ncpdpTelecomEsc(label)}…</div>`;
        }
        const segments = this._step.config[configKey];
        return `
        <div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.3rem;">
                <div style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;">${ncpdpTelecomEsc(label)}</div>
                <button type="button" class="btn btn-sm btn-outline-primary" style="font-size:0.75rem;"
                    onclick="window._ncpdpTelecomMapBuilder && window._ncpdpTelecomMapBuilder.addSegment('${configKey}')">+ Add Segment</button>
            </div>
            <div style="font-size:0.72rem;color:#94a3b8;margin-bottom:0.5rem;">${ncpdpTelecomEsc(hint)}</div>
            ${segments.map((seg, si) => this._renderSegmentCard(configKey, seg, si, repeating)).join('')}
            ${segments.length === 0 ? '<div style="font-size:0.78rem;color:#94a3b8;">No segments mapped.</div>' : ''}
        </div>`;
    }

    _renderSegmentCard(configKey, seg, segIndex, repeating) {
        const fieldsCatalog = seg.segmentKey && this._segmentsByKey[seg.segmentKey] ? (this._segmentsByKey[seg.segmentKey].fields || []) : [];
        const fields = Array.isArray(seg.fields) ? seg.fields : (seg.fields = []);
        return `
        <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:6px;padding:0.6rem 0.7rem;margin:0.4rem 0 0.6rem;">
            <div style="display:flex;gap:0.4rem;align-items:center;margin-bottom:0.5rem;">
                <select style="flex:1;font-size:0.8rem;" class="form-select form-select-sm"
                    onchange="window._ncpdpTelecomMapBuilder && window._ncpdpTelecomMapBuilder.updateSegmentKey('${configKey}', ${segIndex}, this.value)">
                    ${this._segmentKeysHTML(seg.segmentKey)}
                </select>
                <button type="button" class="btn btn-sm btn-outline-danger" style="font-size:0.7rem;padding:0.1rem 0.5rem;"
                    onclick="window._ncpdpTelecomMapBuilder && window._ncpdpTelecomMapBuilder.removeSegment('${configKey}', ${segIndex})">Remove Segment</button>
            </div>
            ${fields.map((f, fi) => this._renderFieldRow(configKey, segIndex, f, fi, fieldsCatalog)).join('')}
            <button type="button" class="btn btn-sm btn-outline-primary" style="font-size:0.72rem;"
                onclick="window._ncpdpTelecomMapBuilder && window._ncpdpTelecomMapBuilder.addSegmentField('${configKey}', ${segIndex})">+ Add Field</button>
        </div>`;
    }

    // ── shared field-row renderer (header rows and segment-card rows) ────

    _renderFieldRow(configKey, segIndex, f, index, fieldsCatalog) {
        const isHeader = configKey === 'header';
        const prefix = isHeader
            ? `window._ncpdpTelecomMapBuilder.updateHeaderField(${index}`
            : `window._ncpdpTelecomMapBuilder.updateSegmentField('${configKey}', ${segIndex}, ${index}`;
        const removeCall = isHeader
            ? `window._ncpdpTelecomMapBuilder.removeHeaderField(${index})`
            : `window._ncpdpTelecomMapBuilder.removeSegmentField('${configKey}', ${segIndex}, ${index})`;

        return `
        <div style="display:flex;gap:0.4rem;align-items:center;margin-bottom:0.4rem;flex-wrap:wrap;">
            <select style="flex:1;min-width:130px;font-size:0.76rem;" class="form-select form-select-sm"
                onchange="${prefix}, 'fieldKey', this.value)">
                ${this._fieldKeyOptionsHTML(fieldsCatalog, f.fieldKey)}
            </select>
            <input type="text" placeholder="source path" value="${ncpdpTelecomEsc(f.sourcePath || '')}"
                style="flex:1;min-width:100px;font-family:monospace;font-size:0.76rem;" class="form-control form-control-sm"
                onblur="${prefix}, 'sourcePath', this.value)">
            <select style="width:130px;font-size:0.76rem;" class="form-select form-select-sm"
                onchange="${prefix}, 'transform', this.value)">
                ${ncpdpTelecomTransformOptionsHTML(f.transform)}
            </select>
            <input type="text" placeholder="literal (if no source)" value="${ncpdpTelecomEsc(f.literalValue || '')}"
                style="flex:1;min-width:90px;font-family:monospace;font-size:0.76rem;" class="form-control form-control-sm"
                onblur="${prefix}, 'literalValue', this.value)">
            <button type="button" class="btn btn-sm btn-outline-danger" style="font-size:0.7rem;padding:0.1rem 0.4rem;"
                onclick="${removeCall}">&times;</button>
        </div>`;
    }

    // ── interaction handlers ─────────────────────────────────────────────

    addHeaderField() {
        this._step.config.headerFields.push({ fieldKey: '', sourcePath: '', transform: '', literalValue: '' });
        this._rerender();
    }

    removeHeaderField(index) {
        this._step.config.headerFields.splice(index, 1);
        this._rerender();
    }

    updateHeaderField(index, key, value) {
        const f = this._step.config.headerFields[index];
        if (!f) return;
        f[key] = value;
        this._rerender();
    }

    addSegment(configKey) {
        this._step.config[configKey].push({ segmentKey: '', fields: [] });
        this._rerender();
    }

    removeSegment(configKey, segIndex) {
        this._step.config[configKey].splice(segIndex, 1);
        this._rerender();
    }

    updateSegmentKey(configKey, segIndex, value) {
        const seg = this._step.config[configKey][segIndex];
        if (!seg) return;
        seg.segmentKey = value;
        this._rerender();
    }

    addSegmentField(configKey, segIndex) {
        const seg = this._step.config[configKey][segIndex];
        if (!seg) return;
        if (!Array.isArray(seg.fields)) seg.fields = [];
        seg.fields.push({ fieldKey: '', sourcePath: '', transform: '', literalValue: '' });
        this._rerender();
    }

    removeSegmentField(configKey, segIndex, fieldIndex) {
        const seg = this._step.config[configKey][segIndex];
        if (!seg) return;
        seg.fields.splice(fieldIndex, 1);
        this._rerender();
    }

    updateSegmentField(configKey, segIndex, fieldIndex, key, value) {
        const seg = this._step.config[configKey][segIndex];
        if (!seg || !seg.fields[fieldIndex]) return;
        seg.fields[fieldIndex][key] = value;
        this._rerender();
    }

    _rerender() {
        const root = document.getElementById('ncpdpTelecomMapToCanonicalBuilder');
        if (!root || !root.parentElement) return;
        root.outerHTML = this._renderAll();
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const outputEl = form.querySelector('#ncpdpTelecomMapOutputField');
        const codeEl = form.querySelector('#ncpdpTelecomMapTransactionCode');
        const directionEl = form.querySelector('#ncpdpTelecomMapDirection');
        const rowsPathEl = form.querySelector('#ncpdpTelecomMapRowsPath');

        if (outputEl) step.config.outputField = outputEl.value.trim() || 'parsedTelecom';
        if (codeEl) step.config.transactionCode = codeEl.value || 'B1';
        if (directionEl) step.config.direction = directionEl.value || 'request';
        if (rowsPathEl) step.config.transactionGroupRowsPath = rowsPathEl.value.trim();

        // headerFields/transmissionGroupSegments/transactionGroupSegments
        // already live in step.config, kept in sync by the interaction
        // handlers above. Header/segment field rows missing both a source
        // and literal value are dropped (they'd write nothing anyway).
        step.config.headerFields = (step.config.headerFields || []).filter(f => f.fieldKey && (f.sourcePath || f.literalValue));
        ['transmissionGroupSegments', 'transactionGroupSegments'].forEach(k => {
            (step.config[k] || []).forEach(seg => {
                seg.fields = (seg.fields || []).filter(f => f.fieldKey && (f.sourcePath || f.literalValue));
            });
            step.config[k] = (step.config[k] || []).filter(seg => seg.segmentKey);
        });
    }

    destroy() {
        this._ac.abort();
        if (window._ncpdpTelecomMapBuilder === this) {
            window._ncpdpTelecomMapBuilder = null;
        }
    }
}

// ── registration ─────────────────────────────────────────────────────────

StepBuilderRegistry.register('ncpdptelecom.parse', NcpdpTelecomParseStepBuilder);
StepBuilderRegistry.register('ncpdptelecom.validate', NcpdpTelecomValidateStepBuilder);
StepBuilderRegistry.register('ncpdptelecom.map_to_canonical', NcpdpTelecomMapToCanonicalStepBuilder);
StepBuilderRegistry.register('ncpdptelecom.build', NcpdpTelecomBuildStepBuilder);
