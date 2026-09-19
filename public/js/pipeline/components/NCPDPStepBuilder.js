/**
 * NCPDPStepBuilder — step config builders for NCPDP SCRIPT (pharmacy
 * e-prescribing) pipeline steps (Phase 1: NewRx, CancelRx, CancelRxResponse,
 * RxChangeRequest, RxChangeResponse).
 *
 * Registers four step types with StepBuilderRegistry:
 *   - ncpdp.parse             Parse raw NCPDP SCRIPT XML to ParsedJSON
 *   - ncpdp.validate          Check a parsed message against the schema's
 *                              own required field/group flags
 *   - ncpdp.map_to_canonical  No-code field mapping from any source shape
 *                              into ncpdp.build's canonical header/body JSON
 *   - ncpdp.build             Build a complete NCPDP SCRIPT XML message
 *
 * Builder contract (StepBuilderRegistry), matching EDIStepBuilder.js/
 * CDAStepBuilder.js exactly:
 *   render(step)         → string  HTML for the properties panel form tab
 *   collectConfig(step)  → void    reads DOM, writes into step.config
 *   destroy()            → void    tears down event listeners / AC refs
 */

// ── shared helpers (file-scope, used by every builder below) ───────────────

function ncpdpEsc(s) {
    return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

const NCPDP_TRANSFORM_OPTIONS = [
    { value: '', label: '(none)' },
    { value: 'trim', label: 'Trim whitespace' },
    { value: 'uppercase', label: 'Uppercase' },
    { value: 'lowercase', label: 'Lowercase' },
];

function ncpdpTransformOptionsHTML(selected) {
    return NCPDP_TRANSFORM_OPTIONS.map(o =>
        `<option value="${ncpdpEsc(o.value)}" ${o.value === (selected || '') ? 'selected' : ''}>${ncpdpEsc(o.label)}</option>`
    ).join('');
}

// Phase 1 scope — see CLAUDE.md's own "NCPDP SCRIPT Engine — Phase 1"
// section for the full transaction-type roadmap (refills/fill-status/
// history/transfer/REMS are later, named phases, not attempted yet).
const NCPDP_TRANSACTION_TYPES = ['NewRx', 'CancelRx', 'CancelRxResponse', 'RxChangeRequest', 'RxChangeResponse'];

function ncpdpTransactionTypeOptionsHTML(selected) {
    return NCPDP_TRANSACTION_TYPES.map(t =>
        `<option value="${ncpdpEsc(t)}" ${t === (selected || 'NewRx') ? 'selected' : ''}>${ncpdpEsc(t)}</option>`
    ).join('');
}

// ── NcpdpParseStepBuilder ────────────────────────────────────────────────────
// Config: { sourceField, outputField }

class NcpdpParseStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;

        const sourceField = ncpdpEsc(cfg.sourceField || 'raw');
        const outputField = ncpdpEsc(cfg.outputField || 'parsedNCPDP');

        return `
        <div class="ncpdp-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Parse step.</strong> Converts raw NCPDP SCRIPT (pharmacy e-prescribing) XML
                into a structured JSON document (transactionType + messageAttrs + header + body).
                The transaction type (NewRx, CancelRx, ...) is auto-detected from the message's own
                &lt;Body&gt; child element — nothing to configure for it here.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="ncpdpParseSourceField" type="text" class="form-control form-control-sm"
                    value="${sourceField}" placeholder="raw"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field containing the raw NCPDP SCRIPT XML string.</div>
            </div>
            <div class="config-group">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ncpdpParseOutputField" type="text" class="form-control form-control-sm"
                    value="${outputField}" placeholder="parsedNCPDP"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the parsed JSON will be written.</div>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const sourceEl = form.querySelector('#ncpdpParseSourceField');
        const outputEl = form.querySelector('#ncpdpParseOutputField');

        if (sourceEl) step.config.sourceField = sourceEl.value.trim() || 'raw';
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'parsedNCPDP';
    }

    destroy() {
        this._ac.abort();
    }
}

// ── NcpdpValidateStepBuilder ─────────────────────────────────────────────────
// Config: { sourceField, outputField }
//
// Deliberately simpler than EdiValidateStepBuilder — no custom-rule/disabled-
// rule mechanism yet (see ncpdp/validator/validator.go's own doc comment:
// no free source of NCPDP's own conditional-requirement business rules was
// found for Phase 1, so the warning tier is an intentionally empty,
// ready-to-extend mechanism, not a UI surface to build yet).

class NcpdpValidateStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;

        const sourceField = ncpdpEsc(cfg.sourceField || 'parsedNCPDP');
        const outputField = ncpdpEsc(cfg.outputField || 'ncpdpValidation');

        return `
        <div class="ncpdp-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Validate step.</strong> Checks a parsed NCPDP SCRIPT message against the
                schema's own required field/group flags. A missing or empty REQUIRED field/group is a
                blocking <strong>error</strong> (<code>valid=false</code>). Accepts either a prior
                <code>ncpdp.parse</code> step's own output, or raw XML (re-parsed on the fly) if this
                step runs standalone.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="ncpdpValidateSourceField" type="text" class="form-control form-control-sm"
                    value="${sourceField}" placeholder="parsedNCPDP"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field holding a prior ncpdp.parse step's output — or raw XML text.</div>
            </div>
            <div class="config-group">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ncpdpValidateOutputField" type="text" class="form-control form-control-sm"
                    value="${outputField}" placeholder="ncpdpValidation"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where {valid, issues:[{severity,path,message}]} will be written.</div>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const sourceEl = form.querySelector('#ncpdpValidateSourceField');
        const outputEl = form.querySelector('#ncpdpValidateOutputField');

        if (sourceEl) step.config.sourceField = sourceEl.value.trim() || 'parsedNCPDP';
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'ncpdpValidation';
    }

    destroy() {
        this._ac.abort();
    }
}

// ── NcpdpBuildStepBuilder ────────────────────────────────────────────────────
// Config: { sourceField, transactionType, outputField }

class NcpdpBuildStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;

        const sourceField = ncpdpEsc(cfg.sourceField || 'parsedNCPDP');
        const outputField = ncpdpEsc(cfg.outputField || 'ncpdpScript');

        return `
        <div class="ncpdp-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Build step.</strong> Builds a complete NCPDP SCRIPT XML message from canonical
                JSON. Accepts either <code>ncpdp.parse</code>'s own output (a round trip) or
                <code>ncpdp.map_to_canonical</code>'s output (built from any source shape) — both
                produce the same header/body shape this step expects.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="ncpdpBuildSourceField" type="text" class="form-control form-control-sm"
                    value="${sourceField}" placeholder="parsedNCPDP"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field holding canonical header/body JSON.</div>
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ncpdpBuildOutputField" type="text" class="form-control form-control-sm"
                    value="${outputField}" placeholder="ncpdpScript"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the built XML text will be written.</div>
            </div>
            <div class="config-group">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Transaction Type</label>
                <select id="ncpdpBuildTransactionType" class="form-select form-select-sm">
                    ${ncpdpTransactionTypeOptionsHTML(cfg.transactionType)}
                </select>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const get = id => form.querySelector('#' + id);
        const sourceEl = get('ncpdpBuildSourceField');
        const outputEl = get('ncpdpBuildOutputField');
        const txTypeEl = get('ncpdpBuildTransactionType');

        if (sourceEl) step.config.sourceField = sourceEl.value.trim() || 'parsedNCPDP';
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'ncpdpScript';
        if (txTypeEl) step.config.transactionType = txTypeEl.value || 'NewRx';
    }

    destroy() {
        this._ac.abort();
    }
}

// ── NcpdpMapToCanonicalStepBuilder ───────────────────────────────────────────
// Config: { outputField, transactionType,
//           headerFields: [{fieldKey,sourcePath,transform,literalValue}],
//           headerGroups: [{groupKey,rowsPath,fields:[...],groups:[...recursive...]}],
//           bodyFields: [...same shape as headerFields...],
//           bodyGroups: [...same shape as headerGroups...] }
//
// Mirrors EdiMapToCanonicalStepBuilder's own loops recursion (a dot-joined
// groupKey path addresses arbitrarily nested groups, matching
// NCPDPGroupDef's own tree shape) — applied to TWO independent top-level
// trees (Header, Body/transaction) instead of EDI's one (header + loops),
// since NCPDP SCRIPT has two separate root structures, not one.

class NcpdpMapToCanonicalStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
        this._step = null;
        this._groupsByKey = null;   // { [groupKey]: {key,xmlElement,name,fields:[{key,name,dataType,required}],groups:[{key,groupKey,xmlElement,required,repeatable}]} }
        this._headerGroupKey = null;
        this._transactionSchema = null; // { fields:[...], groups:[...] } for the CURRENTLY configured transactionType

        window._ncpdpMapBuilder = this;
    }

    render(step) {
        this._step = step;
        if (!step.config) step.config = {};
        const cfg = step.config;
        if (!cfg.outputField) cfg.outputField = 'parsedNCPDP';
        if (!cfg.transactionType) cfg.transactionType = 'NewRx';
        if (!Array.isArray(cfg.headerFields)) cfg.headerFields = [];
        if (!Array.isArray(cfg.headerGroups)) cfg.headerGroups = [];
        if (!Array.isArray(cfg.bodyFields)) cfg.bodyFields = [];
        if (!Array.isArray(cfg.bodyGroups)) cfg.bodyGroups = [];

        this._loadGroupsCatalog();
        this._loadTransactionSchema();

        return this._renderAll();
    }

    _loadGroupsCatalog() {
        if (this._groupsByKey) return;
        fetch('/api/ncpdp/schema/groups', { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || !data.groups) return;
                this._groupsByKey = {};
                data.groups.forEach(g => { this._groupsByKey[g.key] = g; });
                this._headerGroupKey = data.headerGroupKey || null;
                this._rerender();
            })
            .catch(() => {}); // AbortError on destroy is expected
    }

    // Fetches the top-level fields/groups for the CURRENTLY configured
    // transaction type — not hardcoded to NewRx. onTransactionTypeChange
    // resets this to null before calling again, so switching the picker
    // re-fetches the right tree instead of silently keeping the previous one.
    _loadTransactionSchema() {
        if (this._transactionSchema) return;
        const txType = ncpdpEsc(this._step.config.transactionType || 'NewRx');
        fetch(`/api/ncpdp/schema/transactions/${txType}/groups`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data) return;
                this._transactionSchema = { fields: data.fields || [], groups: data.groups || [] };
                this._rerender();
            })
            .catch(() => {});
    }

    // Switching transaction types invalidates the transaction schema (a
    // different transaction type has a completely different body tree) AND
    // every previously-configured body group mapping, which addresses the
    // OLD tree's own group keys and would silently reference nonexistent
    // groups otherwise. Header groups are unaffected — the Header shape is
    // the same for every transaction type.
    onTransactionTypeChange(value) {
        this._step.config.transactionType = value || 'NewRx';
        this._step.config.bodyGroups = [];
        this._transactionSchema = null;
        this._loadTransactionSchema();
        this._rerender();
    }

    // Resolves the schema group-ref list at a given dot-joined groupKey
    // path, starting from rootGroups (the Header's or transaction's own
    // top-level .groups) — mirrors the Go engine's own findGroupRef +
    // spec.Groups[...] walk (services/executors/transform/
    // ncpdp_map_to_canonical_executor.go) exactly, one level at a time.
    _schemaGroupsAt(rootGroups, idPath) {
        if (!idPath) return rootGroups || [];
        const parts = idPath.split('.');
        let refs = rootGroups || [];
        for (const part of parts) {
            const ref = (refs || []).find(r => r.key === part);
            if (!ref) return [];
            const group = this._groupsByKey ? this._groupsByKey[ref.groupKey] : null;
            refs = group ? (group.groups || []) : [];
        }
        return refs;
    }

    // Resolves the schema field list AT a given dot-joined groupKey path
    // (the fields directly on that group, not its children) — empty path
    // means the root's own top-level fields (Header's, or the transaction's).
    _schemaFieldsAt(rootGroups, rootFields, idPath) {
        if (!idPath) return rootFields || [];
        const parts = idPath.split('.');
        let refs = rootGroups || [];
        let fields = [];
        for (const part of parts) {
            const ref = (refs || []).find(r => r.key === part);
            if (!ref) return [];
            const group = this._groupsByKey ? this._groupsByKey[ref.groupKey] : null;
            fields = group ? (group.fields || []) : [];
            refs = group ? (group.groups || []) : [];
        }
        return fields;
    }

    _rootGroups(scope) {
        if (scope === 'header') {
            const hdr = this._headerGroupKey && this._groupsByKey ? this._groupsByKey[this._headerGroupKey] : null;
            return hdr ? (hdr.groups || []) : [];
        }
        return this._transactionSchema ? this._transactionSchema.groups : [];
    }

    _rootFields(scope) {
        if (scope === 'header') {
            const hdr = this._headerGroupKey && this._groupsByKey ? this._groupsByKey[this._headerGroupKey] : null;
            return hdr ? (hdr.fields || []) : [];
        }
        return this._transactionSchema ? this._transactionSchema.fields : [];
    }

    _configGroupsArray(scope) {
        return scope === 'header' ? this._step.config.headerGroups : this._step.config.bodyGroups;
    }

    // Finds (or creates, when create=true) the config group object at a
    // dot-joined idPath within scope ('header' | 'body'). Returns
    // {array, index} — array is the array CONTAINING the target node, index
    // is its position (-1 when not found and create=false). Mirrors
    // EdiMapToCanonicalStepBuilder's own _configLoopArrayAndIndex exactly.
    _configGroupArrayAndIndex(scope, idPath, create) {
        const parts = idPath.split('.');
        let array = this._configGroupsArray(scope);
        let idx = -1;
        for (let i = 0; i < parts.length; i++) {
            idx = array.findIndex(n => n.groupKey === parts[i]);
            if (idx === -1) {
                if (!create) return { array, index: -1 };
                array.push({ groupKey: parts[i], fields: [], groups: [] });
                idx = array.length - 1;
            }
            if (i === parts.length - 1) break;
            const node = array[idx];
            if (!Array.isArray(node.groups)) node.groups = [];
            array = node.groups;
        }
        return { array, index: idx };
    }

    _configGroupAt(scope, idPath) {
        const { array, index } = this._configGroupArrayAndIndex(scope, idPath, false);
        return index === -1 ? null : array[index];
    }

    _fieldKeyOptionsHTML(fieldsCatalog, selectedKey) {
        return ['<option value="">-- Field --</option>']
            .concat((fieldsCatalog || []).map(f =>
                `<option value="${ncpdpEsc(f.key)}" ${f.key === selectedKey ? 'selected' : ''}>${ncpdpEsc(f.key)}${f.required ? ' *' : ''}${f.name ? ' — ' + ncpdpEsc(f.name) : ''}</option>`))
            .join('');
    }

    _renderAll() {
        const cfg = this._step.config;
        return `
        <div id="ncpdpMapToCanonicalBuilder" class="ncpdp-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Map to Canonical step.</strong> No-code field mapping from any source shape
                (CSV, DB rows, generic JSON) into the canonical header/body JSON <code>ncpdp.build</code>
                consumes — the on-ramp for building an NCPDP SCRIPT message from data that never went
                through <code>ncpdp.parse</code>.
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="ncpdpMapOutputField" type="text" class="form-control form-control-sm"
                    value="${ncpdpEsc(cfg.outputField)}" placeholder="parsedNCPDP"
                    style="font-family:monospace;font-size:0.82rem;">
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Transaction Type</label>
                <select class="form-select form-select-sm"
                    onchange="window._ncpdpMapBuilder && window._ncpdpMapBuilder.onTransactionTypeChange(this.value)">
                    ${ncpdpTransactionTypeOptionsHTML(cfg.transactionType)}
                </select>
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Which transaction type's body tree the Body Groups section below reflects. Switching this clears any body group mappings configured below — they addressed the previous tree's own group keys.</div>
            </div>
            ${this._renderFlatFieldsSection('header', 'Header Fields', cfg.headerFields)}
            ${this._renderGroupsSection('header', 'Header Groups')}
            ${this._renderFlatFieldsSection('body', 'Body Fields', cfg.bodyFields)}
            ${this._renderGroupsSection('body', 'Body Groups')}
        </div>`;
    }

    // ── flat fields (Header's or the transaction's own top-level scalars) ──

    _renderFlatFieldsSection(scope, label, fields) {
        if (!this._groupsByKey || (scope === 'body' && !this._transactionSchema)) {
            return `<div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;font-size:0.78rem;color:#94a3b8;">Loading ${ncpdpEsc(label)}…</div>`;
        }
        const catalog = this._rootFields(scope);
        return `
        <div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.5rem;">
                <div style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;">${ncpdpEsc(label)}</div>
                <button type="button" class="btn btn-sm btn-outline-primary" style="font-size:0.75rem;"
                    onclick="window._ncpdpMapBuilder && window._ncpdpMapBuilder.addFlatField('${scope}')">+ Add Field</button>
            </div>
            ${fields.map((f, i) => this._renderFieldRow(scope, null, f, i, catalog)).join('')}
            ${fields.length === 0 ? '<div style="font-size:0.78rem;color:#94a3b8;">No fields mapped.</div>' : ''}
        </div>`;
    }

    // ── nested groups (recursive) ───────────────────────────────────────────

    _renderGroupsSection(scope, label) {
        if (!this._groupsByKey || (scope === 'body' && !this._transactionSchema)) {
            return `<div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;font-size:0.78rem;color:#94a3b8;">Loading ${ncpdpEsc(label)}…</div>`;
        }
        const rootGroups = this._rootGroups(scope);
        return `
        <div style="border-top:1px solid #e2e8f0;margin:1rem 0 0.75rem;padding-top:0.85rem;">
            <div style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;margin-bottom:0.3rem;">${ncpdpEsc(label)}</div>
            <div style="font-size:0.72rem;color:#94a3b8;margin-bottom:0.5rem;">Check a group to map fields into it. Set Rows Path on a repeatable group to map one entry per source row.</div>
            ${rootGroups.map(ref => this._renderGroupNode(scope, ref, ref.key)).join('')}
            ${rootGroups.length === 0 ? '<div style="font-size:0.78rem;color:#94a3b8;">No nested groups.</div>' : ''}
        </div>`;
    }

    _renderGroupNode(scope, ref, idPath) {
        const depth = idPath.split('.').length;
        const included = !!this._configGroupAt(scope, idPath);
        const repeatHint = ref.repeatable ? ' <span style="color:#94a3b8;">(repeatable)</span>' : '';
        const requiredHint = ref.required ? ' *' : '';
        const childRefs = this._schemaGroupsAt(this._rootGroups(scope), idPath);

        return `
        <div style="margin-left:${depth > 1 ? '1.25rem' : '0'};${depth > 1 ? 'border-left:2px solid #e2e8f0;padding-left:0.75rem;' : ''}margin-bottom:0.4rem;">
            <label style="display:flex;align-items:center;gap:0.5rem;font-size:0.82rem;cursor:pointer;">
                <input type="checkbox" ${included ? 'checked' : ''}
                    onchange="window._ncpdpMapBuilder && window._ncpdpMapBuilder.toggleGroup('${scope}', '${idPath}', this.checked)">
                <strong>${ncpdpEsc(ref.key)}</strong>${requiredHint}${repeatHint}
            </label>
            ${included ? this._renderGroupCard(scope, ref, idPath, childRefs) : ''}
            ${childRefs.map(childRef => this._renderGroupNode(scope, childRef, idPath + '.' + childRef.key)).join('')}
        </div>`;
    }

    _renderGroupCard(scope, ref, idPath, childRefs) {
        const node = this._configGroupAt(scope, idPath);
        if (!node) return '';
        const fields = Array.isArray(node.fields) ? node.fields : (node.fields = []);
        const fieldsCatalog = this._schemaFieldsAt(this._rootGroups(scope), this._rootFields(scope), idPath);
        return `
        <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:6px;padding:0.6rem 0.7rem;margin:0.4rem 0 0.6rem;">
            ${ref.repeatable ? `
            <div class="config-group" style="margin-bottom:0.5rem;">
                <label style="font-size:0.7rem;color:#64748b;display:block;margin-bottom:0.2rem;">Rows Path (one entry per source row)</label>
                <input type="text" value="${ncpdpEsc(node.rowsPath || '')}" placeholder="e.g. diagnoses"
                    style="width:100%;font-family:monospace;font-size:0.78rem;"
                    onblur="window._ncpdpMapBuilder && window._ncpdpMapBuilder.updateGroupRowsPath('${scope}', '${idPath}', this.value)">
            </div>` : ''}
            ${fields.map((f, fi) => this._renderFieldRow(scope, idPath, f, fi, fieldsCatalog)).join('')}
            <button type="button" class="btn btn-sm btn-outline-primary" style="font-size:0.72rem;"
                onclick="window._ncpdpMapBuilder && window._ncpdpMapBuilder.addGroupField('${scope}', '${idPath}')">+ Add Field</button>
        </div>`;
    }

    // ── shared field-row renderer (flat rows and group-card rows) ──────────

    _renderFieldRow(scope, idPath, f, index, fieldsCatalog) {
        const isFlat = idPath === null;
        const prefix = isFlat
            ? `window._ncpdpMapBuilder.updateFlatField('${scope}', ${index}`
            : `window._ncpdpMapBuilder.updateGroupField('${scope}', '${idPath}', ${index}`;
        const removeCall = isFlat
            ? `window._ncpdpMapBuilder.removeFlatField('${scope}', ${index})`
            : `window._ncpdpMapBuilder.removeGroupField('${scope}', '${idPath}', ${index})`;

        return `
        <div style="display:flex;gap:0.4rem;align-items:center;margin-bottom:0.4rem;flex-wrap:wrap;">
            <select style="flex:1;min-width:130px;font-size:0.76rem;" class="form-select form-select-sm"
                onchange="${prefix}, 'fieldKey', this.value)">
                ${this._fieldKeyOptionsHTML(fieldsCatalog, f.fieldKey)}
            </select>
            <input type="text" placeholder="source path" value="${ncpdpEsc(f.sourcePath || '')}"
                style="flex:1;min-width:100px;font-family:monospace;font-size:0.76rem;" class="form-control form-control-sm"
                onblur="${prefix}, 'sourcePath', this.value)">
            <select style="width:130px;font-size:0.76rem;" class="form-select form-select-sm"
                onchange="${prefix}, 'transform', this.value)">
                ${ncpdpTransformOptionsHTML(f.transform)}
            </select>
            <input type="text" placeholder="literal (if no source)" value="${ncpdpEsc(f.literalValue || '')}"
                style="flex:1;min-width:90px;font-family:monospace;font-size:0.76rem;" class="form-control form-control-sm"
                onblur="${prefix}, 'literalValue', this.value)">
            <button type="button" class="btn btn-sm btn-outline-danger" style="font-size:0.7rem;padding:0.1rem 0.4rem;"
                onclick="${removeCall}">&times;</button>
        </div>`;
    }

    // ── interaction handlers ─────────────────────────────────────────────

    toggleGroup(scope, idPath, checked) {
        if (checked) {
            this._configGroupArrayAndIndex(scope, idPath, true);
        } else {
            const { array, index } = this._configGroupArrayAndIndex(scope, idPath, false);
            if (index !== -1) array.splice(index, 1);
        }
        this._rerender();
    }

    updateGroupRowsPath(scope, idPath, value) {
        const node = this._configGroupAt(scope, idPath);
        if (node) node.rowsPath = value;
    }

    addGroupField(scope, idPath) {
        const node = this._configGroupAt(scope, idPath);
        if (!node) return;
        if (!Array.isArray(node.fields)) node.fields = [];
        node.fields.push({ fieldKey: '', sourcePath: '', transform: '', literalValue: '' });
        this._rerender();
    }

    removeGroupField(scope, idPath, index) {
        const node = this._configGroupAt(scope, idPath);
        if (!node) return;
        node.fields.splice(index, 1);
        this._rerender();
    }

    updateGroupField(scope, idPath, index, key, value) {
        const node = this._configGroupAt(scope, idPath);
        if (!node || !node.fields[index]) return;
        node.fields[index][key] = value;
        this._rerender();
    }

    addFlatField(scope) {
        const arr = scope === 'header' ? this._step.config.headerFields : this._step.config.bodyFields;
        arr.push({ fieldKey: '', sourcePath: '', transform: '', literalValue: '' });
        this._rerender();
    }

    removeFlatField(scope, index) {
        const arr = scope === 'header' ? this._step.config.headerFields : this._step.config.bodyFields;
        arr.splice(index, 1);
        this._rerender();
    }

    updateFlatField(scope, index, key, value) {
        const arr = scope === 'header' ? this._step.config.headerFields : this._step.config.bodyFields;
        const f = arr[index];
        if (!f) return;
        f[key] = value;
        this._rerender();
    }

    _rerender() {
        const root = document.getElementById('ncpdpMapToCanonicalBuilder');
        if (!root || !root.parentElement) return;
        root.outerHTML = this._renderAll();
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const outputEl = form.querySelector('#ncpdpMapOutputField');
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'parsedNCPDP';

        // transactionType is already kept in sync on step.config by
        // onTransactionTypeChange (it must reset bodyGroups/the transaction
        // schema immediately, not wait for a later collectConfig call) —
        // nothing further to read from the DOM for it here.

        // headerFields/headerGroups/bodyFields/bodyGroups already live in
        // step.config, kept in sync by the interaction handlers above. Flat
        // rows missing both a source and literal value are dropped (they'd
        // write nothing anyway); group entries are NOT filtered the same
        // way — an empty-fields group is a valid pure nesting container for
        // its own children.
        step.config.headerFields = (step.config.headerFields || []).filter(f => f.fieldKey && (f.sourcePath || f.literalValue));
        step.config.bodyFields = (step.config.bodyFields || []).filter(f => f.fieldKey && (f.sourcePath || f.literalValue));
    }

    destroy() {
        this._ac.abort();
        if (window._ncpdpMapBuilder === this) {
            window._ncpdpMapBuilder = null;
        }
    }
}

// ── registration ─────────────────────────────────────────────────────────

StepBuilderRegistry.register('ncpdp.parse', NcpdpParseStepBuilder);
StepBuilderRegistry.register('ncpdp.validate', NcpdpValidateStepBuilder);
StepBuilderRegistry.register('ncpdp.map_to_canonical', NcpdpMapToCanonicalStepBuilder);
StepBuilderRegistry.register('ncpdp.build', NcpdpBuildStepBuilder);
