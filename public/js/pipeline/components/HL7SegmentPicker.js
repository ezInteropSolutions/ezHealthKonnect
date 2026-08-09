/**
 * HL7SegmentPicker — one segment-name cell inside an hl7.build "Segments"
 * card (see HL7BuildBuilder.js). Replaces a plain, unrestricted `<select>`
 * populated with every segment in the schema with a grammar-guarded picker:
 *
 *   1. Default: only segments hl7/builder.NextAllowedSegments says are valid
 *      to add next, given the segment names already configured before this
 *      row (POST /api/hl7/next-segments/:messageType/:triggerEvent).
 *   2. "⚠ Add anyway (non-standard)" — reveals the full schema segment list
 *      (the "all" side of the same endpoint's response) as an explicit
 *      override, never a dead end.
 *   3. "+ Custom / Z-segment…" — switches to free-text entry for segments
 *      the schema doesn't know about at all. Warns (doesn't block) on a
 *      non-"Z"-prefixed name, mirroring ZSegmentConfigBuilder.js's own
 *      validation. When the typed name matches a known Z-segment template
 *      (GET /api/zsegments/templates?segmentId=), that template is handed
 *      back via onChange so the caller can pre-fill the segment's fields —
 *      the same non-destructive prefill pattern HL7BuildBuilder's own
 *      "✨ Suggest Mappings" panel already uses.
 *
 * options.unrestricted skips the next-segments guardrail entirely (no
 * fetch), presenting options.allSegmentNames as a flat, unfiltered list —
 * for CHILD segments (nested under a parent via HL7BuildBuilder's
 * ChildSegments), where "forward-only from the schema's top-level sequence"
 * isn't a meaningful restriction (a child's valid position is governed by
 * its parent's own group in the schema, not the root sequence — encoding
 * that would need a second, more complex endpoint; see HL7BuildBuilder.js's
 * own header comment for why this is a deliberate v1 scope cut). The
 * "+ Custom / Z-segment…" escape hatch still works identically in this mode.
 *
 * Lifecycle mirrors FieldPathSearchComponent: instantiate onto a placeholder
 * element after the parent's HTML is in the DOM, destroy() before the parent
 * re-renders.
 */
class HL7SegmentPicker {
    constructor(container, options = {}) {
        this.container = container;
        this.options = {
            value: options.value || '',
            messageType: options.messageType || '',
            triggerEvent: options.triggerEvent || '',
            version: options.version || '2.5.1',
            precedingSegments: options.precedingSegments || [],
            unrestricted: !!options.unrestricted,
            allSegmentNames: options.allSegmentNames || [],
            onChange: options.onChange || (() => {}),
        };
        this._ac = new AbortController();
        this._allowed = [];
        this._all = [];
        this._mode = 'select'; // 'select' | 'custom'
        this._loading = true;

        if (this.options.unrestricted) {
            this._loading = false;
            this._allowed = this.options.allSegmentNames.slice();
            this._all = this._allowed;
            if (this.options.value && !this._allowed.includes(this.options.value)) {
                this._mode = 'custom';
            }
            this._render();
        } else {
            this._renderLoading();
            this._loadAllowed();
        }
    }

    // ── Data loading ──────────────────────────────────────────────────────

    _loadAllowed() {
        const { messageType, triggerEvent, version, precedingSegments, value } = this.options;
        if (!messageType || !triggerEvent) {
            this._loading = false;
            this._render();
            return;
        }
        const params = new URLSearchParams({ version });
        fetch(`/api/hl7/next-segments/${encodeURIComponent(messageType)}/${encodeURIComponent(triggerEvent)}?${params}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ addedSegments: precedingSegments }),
            signal: this._ac.signal,
        })
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                this._loading = false;
                if (!data || !data.success) return;
                this._allowed = data.allowed || [];
                this._all = data.all || [];
                // An already-configured value that fell outside "allowed" (e.g.
                // saved before this guardrail existed, or added via override)
                // must stay selectable, never silently disappear from the list.
                if (value && !this._allowed.includes(value) && !this._all.includes(value)) {
                    this._mode = 'custom';
                }
                this._render();
            })
            .catch(() => { this._loading = false; this._render(); });
    }

    _checkZSegmentTemplate(segmentName) {
        return fetch(`/api/zsegments/templates?segmentId=${encodeURIComponent(segmentName)}`, { signal: this._ac.signal })
            .then(r => r.ok ? r.json() : null)
            .then(json => (json && json.success && Array.isArray(json.data) && json.data.length > 0) ? json.data[0] : null)
            .catch(() => null);
    }

    // ── Rendering ─────────────────────────────────────────────────────────

    _renderLoading() {
        if (!this.container) return;
        this.container.innerHTML = `<select class="form-select form-select-sm" disabled><option>Loading…</option></select>`;
    }

    _render() {
        if (!this.container) return;
        const esc = HL7SegmentPicker._esc;

        if (this._mode === 'custom') {
            // Derived from options.value at render time (not just inside the
            // 'change' handler below) so it survives the async re-render the
            // parent triggers from onChange (which resolves after the
            // Z-segment-template lookup and replaces this DOM subtree) —
            // otherwise the warning would flash and immediately disappear.
            const currentValue = this.options.value || '';
            const warningText = currentValue && !currentValue.startsWith('Z')
                ? '⚠ Non-standard: site-specific segments conventionally start with "Z".'
                : '';
            this.container.innerHTML = `
                <div style="display:flex;gap:0.3rem;align-items:center;">
                    <input type="text" class="form-control form-control-sm hl7sp-custom-input"
                        value="${esc(currentValue)}" placeholder="e.g. ZEM" autocomplete="off"
                        style="text-transform:uppercase;font-family:monospace;">
                    <button type="button" class="btn btn-sm btn-outline-secondary hl7sp-back-btn"
                        title="Back to segment list" style="flex:0 0 auto;padding:0.15rem 0.4rem;">↩</button>
                </div>
                <div class="hl7sp-warning" style="font-size:0.68rem;color:#b45309;margin-top:0.2rem;">${esc(warningText)}</div>`;
            const input = this.container.querySelector('.hl7sp-custom-input');
            const warningEl = this.container.querySelector('.hl7sp-warning');
            const backBtn = this.container.querySelector('.hl7sp-back-btn');

            const commit = () => {
                const name = (input.value || '').trim().toUpperCase();
                input.value = name;
                warningEl.textContent = name && !name.startsWith('Z')
                    ? '⚠ Non-standard: site-specific segments conventionally start with "Z".'
                    : '';
                this.options.value = name;
                if (!name) return;
                this._checkZSegmentTemplate(name).then(template => {
                    this.options.onChange(name, { isCustom: true, matchedTemplate: template || null });
                });
            };
            input.addEventListener('change', commit);
            backBtn.addEventListener('click', () => {
                this._mode = 'select';
                this._render();
            });
            return;
        }

        const optionHTML = name => `<option value="${esc(name)}" ${name === this.options.value ? 'selected' : ''}>${esc(name)}</option>`;
        const currentInAllowed = this._allowed.includes(this.options.value);
        const extraCurrentOption = (this.options.value && !currentInAllowed) ? optionHTML(this.options.value) : '';

        this.container.innerHTML = `
            <select class="form-select form-select-sm hl7sp-select" ${this._loading ? 'disabled' : ''}>
                <option value="">— select segment —</option>
                ${extraCurrentOption}
                ${this._allowed.map(optionHTML).join('')}
                ${this.options.unrestricted ? '' : '<option value="__override__">⚠ Add anyway (non-standard)…</option>'}
                <option value="__custom__">+ Custom / Z-segment…</option>
            </select>`;

        this.container.querySelector('.hl7sp-select').addEventListener('change', (e) => {
            const val = e.target.value;
            if (val === '__custom__') {
                this._mode = 'custom';
                this.options.value = '';
                this._render();
                return;
            }
            if (val === '__override__') {
                this._allowed = this._all.slice();
                this._render();
                return;
            }
            this.options.value = val;
            this.options.onChange(val, { isCustom: false, matchedTemplate: null });
        });
    }

    destroy() {
        this._ac.abort();
        this.container = null;
    }

    static _esc(s) {
        return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = HL7SegmentPicker;
}
