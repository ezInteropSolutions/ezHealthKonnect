// public/js/shared/NarrativeFieldsPicker.js
//
// Shared "which fields render in this resource type's narrative" checkbox
// picker — one collapsible section per resource type, fields fetched lazily
// on expand from the schema-driven catalog (GET /api/fhir/narrative-fields/:resourceType,
// resource-type-agnostic and reused as-is everywhere). Storage is injected
// (getConfig/onSave callbacks) so the SAME rendering code works across every
// surface that needs this picker, each pointed at its own backend:
//   - Interface wizard Step 4 (public/js/step4-fhir-transform.js)
//   - hl7_fhir_transform step properties panel (PropertiesPanel.js) — same
//     storage as the wizard (interface_message_mappings.custom_mapping_config)
//   - cda.to_fhir step properties panel (CDAStepBuilder.js) — CDA's own
//     interface_cda_mappings.custom_mapping_config
//   - fhir.build step properties panel (FHIRBuildBuilder.js) — single-type
//     mode (one resourceType, no section list to choose from), same storage
//     as hl7_fhir_transform (a fhir.build-sourced resource shares narrative
//     config with any other resource of the same type in that interface)
//
// Originally built once, inline, in step4-fhir-transform.js; extracted here
// once a second and third call site needed the identical UI, rather than
// re-writing the same collapsible/checkbox logic a third and fourth time.

(function (global) {
    'use strict';

    // Registry so inline onclick handlers in generated HTML can reach the
    // right instance without needing a closures-in-template-strings dance.
    // Keyed by instanceId (caller-supplied, must be unique per concurrently
    // rendered picker — in practice at most one step properties panel is
    // open at a time, but this avoids any cross-panel bleed regardless).
    const registry = {};

    class NarrativeFieldsPicker {
        /**
         * @param {Object} opts
         * @param {string} opts.instanceId - unique id for this picker instance
         * @param {HTMLElement} opts.sectionsEl - element to render collapsible sections into
         * @param {HTMLElement} [opts.statusEl] - optional status text element
         * @param {HTMLElement} [opts.badgeEl] - optional "N customized" badge element
         * @param {() => Promise<Object<string,string[]>>} opts.getConfig - returns the currently-saved
         *        resourceType -> allowed-field-names map (absent key = show everything)
         * @param {(payload: Object<string,string[]>) => Promise<{success: boolean, error?: string}>} opts.onSave -
         *        persists a resourceType -> allowed-field-names payload (only for sections the user touched)
         */
        constructor(opts) {
            this.instanceId = opts.instanceId;
            this.sectionsEl = opts.sectionsEl;
            this.statusEl = opts.statusEl || null;
            this.badgeEl = opts.badgeEl || null;
            this.getConfig = opts.getConfig;
            this.onSave = opts.onSave;
            this._currentConfig = {};
            registry[this.instanceId] = this;
        }

        _esc(s) {
            return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        }

        /**
         * Fetches the current config and renders one collapsible section per
         * resourceType. Safe to call again to re-render with a different/updated
         * resourceTypes list (e.g. once a real transform result is known).
         */
        async render(resourceTypes) {
            if (!this.sectionsEl || !resourceTypes || resourceTypes.length === 0) return;

            try {
                this._currentConfig = (await this.getConfig()) || {};
            } catch (_) {
                this._currentConfig = {};
            }

            this.sectionsEl.innerHTML = resourceTypes.map(rt => `
                <div style="border:1px solid #e0e7ff;border-radius:6px;background:#fff;">
                    <button type="button" onclick="window.NarrativeFieldsPicker.get('${this._esc(this.instanceId)}').toggleSection('${this._esc(rt)}')"
                        style="width:100%;text-align:left;padding:8px 12px;background:none;border:none;cursor:pointer;font-size:12px;font-weight:600;color:#1e293b;display:flex;justify-content:space-between;align-items:center;">
                        <span>${this._esc(rt)}</span>
                        <span id="narr-caret-${this._esc(this.instanceId)}-${this._esc(rt)}" style="font-size:10px;color:#6b7280;">▶</span>
                    </button>
                    <div id="narr-body-${this._esc(this.instanceId)}-${this._esc(rt)}" data-narr-resource="${this._esc(rt)}" style="display:none;padding:0 12px 10px;">
                        <div style="font-size:11px;color:#9ca3af;">Loading fields…</div>
                    </div>
                </div>
            `).join('');

            this._updateBadge();
        }

        _updateBadge() {
            if (!this.badgeEl) return;
            const restrictedCount = Object.values(this._currentConfig).filter(v => Array.isArray(v) && v.length > 0).length;
            if (restrictedCount > 0) {
                this.badgeEl.textContent = `${restrictedCount} customized`;
                this.badgeEl.style.display = '';
            } else {
                this.badgeEl.style.display = 'none';
            }
        }

        async toggleSection(resourceType) {
            const body = document.getElementById(`narr-body-${this.instanceId}-${resourceType}`);
            const caret = document.getElementById(`narr-caret-${this.instanceId}-${resourceType}`);
            if (!body) return;
            const opening = body.style.display === 'none';
            body.style.display = opening ? '' : 'none';
            if (caret) caret.textContent = opening ? '▼' : '▶';
            if (opening && !body.dataset.loaded) {
                await this._loadSectionFields(resourceType, body);
                body.dataset.loaded = '1';
            }
        }

        async _loadSectionFields(resourceType, body) {
            try {
                const res = await fetch(`/api/fhir/narrative-fields/${encodeURIComponent(resourceType)}`, { credentials: 'include' }).then(r => r.json());
                const fields = res.fields || [];
                const restricted = (this._currentConfig || {})[resourceType] || null; // null = show everything (default)
                body.innerHTML = fields.map(f => {
                    const checked = !restricted || restricted.includes(f.name);
                    return `<label style="display:flex;align-items:center;gap:8px;padding:4px 0;font-size:11px;color:#374151;">
                        <input type="checkbox" data-narrfield-name="${this._esc(f.name)}" ${checked ? 'checked' : ''}
                            style="accent-color:#1e3a8a">
                        <span>${this._esc(f.label)}</span>
                    </label>`;
                }).join('') || '<div style="font-size:11px;color:#9ca3af;">No fields found</div>';
            } catch (_) {
                body.innerHTML = '<div style="font-size:11px;color:#dc2626;">Failed to load fields</div>';
            }
        }

        /**
         * Collects checkbox state from every EXPANDED section (unexpanded
         * sections have no checkboxes in the DOM and are correctly never
         * force-saved — they stay "show everything" until a user opens them),
         * and calls the injected onSave callback.
         */
        async save() {
            if (this.statusEl) this.statusEl.textContent = 'Saving…';

            const payload = {};
            this.sectionsEl.querySelectorAll('[data-narr-resource]').forEach(bodyEl => {
                const rt = bodyEl.dataset.narrResource;
                const boxes = bodyEl.querySelectorAll('[data-narrfield-name]');
                if (boxes.length === 0) return; // never loaded/expanded — skip
                const checkedNames = [];
                boxes.forEach(b => { if (b.checked) checkedNames.push(b.dataset.narrfieldName); });
                // Everything checked == no restriction — save [] so the backend
                // clears any prior restriction instead of storing a no-op list.
                payload[rt] = (checkedNames.length === boxes.length) ? [] : checkedNames;
            });

            if (Object.keys(payload).length === 0) {
                if (this.statusEl) {
                    this.statusEl.textContent = 'Expand a resource type to customize its fields first';
                    setTimeout(() => { if (this.statusEl) this.statusEl.textContent = ''; }, 3000);
                }
                return { success: false, error: 'nothing to save' };
            }

            try {
                const result = await this.onSave(payload);
                if (result && result.success) {
                    this._currentConfig = { ...this._currentConfig, ...payload };
                    this._updateBadge();
                    if (this.statusEl) {
                        this.statusEl.textContent = '✓ Saved';
                        setTimeout(() => { if (this.statusEl) this.statusEl.textContent = ''; }, 3000);
                    }
                } else if (this.statusEl) {
                    this.statusEl.textContent = (result && result.error) || 'Save failed';
                }
                return result;
            } catch (e) {
                if (this.statusEl) this.statusEl.textContent = 'Network error';
                return { success: false, error: e.message };
            }
        }
    }

    global.NarrativeFieldsPicker = NarrativeFieldsPicker;
    global.NarrativeFieldsPicker.get = function (instanceId) { return registry[instanceId]; };
})(window);
