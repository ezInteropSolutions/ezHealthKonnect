/**
 * HL7FieldPathPicker — one field-key cell inside an hl7.build segment's
 * fields table (see HL7BuildBuilder.js). Wraps the existing
 * FieldPathSearchComponent (public/js/pipeline/components/FieldPathSearchComponent.js)
 * — the same type-to-search, keyboard-navigable combobox already used for
 * source paths elsewhere in the pipeline builder — instead of building a
 * second search UI from scratch. The only thing this class does is feed it
 * the LIVE hl7/builder.SegmentFieldCatalog entries for the current segment
 * (via `additionalFields`, with `includeHL7Fields: false` so the component's
 * own hardcoded generic HL7 list — a separate, unrelated legacy list — never
 * shows up here) instead of pipeline step-output variables.
 *
 * Searching "SSN" matches PID.19's real label ("SSN Number - Patient");
 * searching "PID.19" or "PID 19" matches the key directly — both field- and
 * component-level catalog entries (e.g. "PID.5" and "PID.5.1") are passed in
 * as one flat, already-schema-sorted list, so there's no separate
 * "drill into a field to see its components" step.
 *
 * `allowCustom: true` (the search component's own built-in escape hatch)
 * covers what the catalog genuinely can't express — subcomponent-depth paths
 * (confirmed no compiled schema encodes a 3rd level) and Z-segment fields
 * (no catalog exists at all, so `fieldCatalog` is simply empty and every
 * search becomes a custom entry) — validated against the same positional
 * grammar hl7/builder.Segment.Set enforces before being accepted.
 */
class HL7FieldPathPicker {
    constructor(container, options = {}) {
        this.container = container;
        this.options = {
            value: options.value || '',
            segment: options.segment || '',
            fieldCatalog: options.fieldCatalog || [],
            onChange: options.onChange || (() => {}),
        };
        this._search = null;
        this._render();
    }

    _render() {
        if (!this.container) return;
        const esc = HL7FieldPathPicker._esc;
        this.container.innerHTML = `
            <input type="text" class="form-control form-control-sm hl7fp-input"
                value="${esc(this.options.value)}" placeholder="Search fields… (e.g. SSN, ${esc(this.options.segment || 'PID')}.19)"
                autocomplete="off" style="font-family:monospace;font-size:0.8rem;">
            <div class="hl7fp-warning" style="font-size:0.68rem;color:#dc2626;margin-top:0.15rem;"></div>`;

        const input = this.container.querySelector('.hl7fp-input');
        const warningEl = this.container.querySelector('.hl7fp-warning');
        const pattern = new RegExp(`^${HL7FieldPathPicker._escapeRegex(this.options.segment)}(\\.[0-9]+){1,4}$`, 'i');

        const commit = (value) => {
            const val = (value || '').trim().toUpperCase();
            if (val && this.options.segment && !pattern.test(val) && !this.options.fieldCatalog.some(f => f.key === val)) {
                warningEl.textContent = `Expected the form ${this.options.segment}.N or ${this.options.segment}.N.C (up to 4 positions) — this won't resolve to a real field when the message is built.`;
            } else {
                warningEl.textContent = '';
            }
            this.options.value = val;
            this.options.onChange(val);
        };

        if (typeof FieldPathSearchComponent !== 'undefined') {
            this._search = new FieldPathSearchComponent(input, {
                onSelect: commit,
                placeholder: `Search fields… (e.g. SSN, ${this.options.segment || 'PID'}.19)`,
                allowCustom: true,
                showCategories: false,
                includeHL7Fields: false,
                additionalFields: HL7FieldPathPicker._catalogToSearchFields(this.options.fieldCatalog),
            });
        }
        input.addEventListener('change', () => commit(input.value));
    }

    static _catalogToSearchFields(catalog) {
        return catalog.map(f => {
            const badges = [f.dataType, f.required ? 'required' : '', f.canRepeat ? 'repeats' : '']
                .filter(Boolean).join(' · ');
            return { name: f.label || f.key, path: f.key, description: badges, category: 'Schema Fields' };
        });
    }

    static _escapeRegex(s) {
        return String(s || '').replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }

    getValue() {
        return this.options.value;
    }

    destroy() {
        if (this._search) { try { this._search.destroy(); } catch (e) { /* no-op */ } }
        this._search = null;
        this.container = null;
    }

    static _esc(s) {
        return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = HL7FieldPathPicker;
}
