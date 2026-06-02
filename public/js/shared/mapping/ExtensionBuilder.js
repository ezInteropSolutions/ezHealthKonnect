/**
 * ExtensionBuilder
 *
 * Modal popup for guided FHIR extension mapping.
 * Lets the user:
 *   1. Find an extension by name/keyword or from HL7-field-based suggestions
 *   2. Configure the value[x] type and system URI (for coded types)
 *   3. Override the transform key
 *   4. Test the transform with a live sample HL7 value
 *   5. Confirm — fires 'extension-confirmed' CustomEvent on the overlay
 *
 * Depends on: FhirExtensionCatalog (must load before this file)
 *
 * Usage:
 *   const builder = new ExtensionBuilder({
 *       hl7Field:     'PD1.21',
 *       hl7DataType:  'IS',
 *       resourceType: 'Patient',
 *       onConfirm: function(result) {
 *           // result: { fhirPath, url, valueType, transform, system }
 *       }
 *   });
 *   builder.open();
 */
class ExtensionBuilder {

    constructor(options) {
        this._hl7Field    = (options.hl7Field    || '').trim();
        this._hl7DataType = (options.hl7DataType || '').toUpperCase();
        this._resource    = (options.resourceType || 'Patient').trim();
        this._onConfirm   = options.onConfirm || function() {};
        this._overlay     = null;
        this._destroyed   = false;

        // Current selections — initialised with auto-suggestions
        this._selectedEntry  = null;
        this._selectedUrl    = '';
        this._selectedVT     = FhirExtensionCatalog.suggestValueType(this._hl7DataType);
        this._selectedSystem = '';
        this._selectedTrans  = FhirExtensionCatalog.suggestTransform(this._hl7DataType, this._selectedVT);
        this._searchQuery    = '';
    }

    // ── Public API ──────────────────────────────────────────────────────────

    open() {
        if (this._overlay) return;
        this._buildOverlay();
        document.body.appendChild(this._overlay);
        // Pre-load suggestions based on source field
        this._renderCatalog('');
        this._toggleSystemRow();   // sets hint + URL status on open
        this._updatePreview();
        // Focus search box
        var searchBox = this._overlay.querySelector('#extBuilderSearch');
        if (searchBox) setTimeout(function() { searchBox.focus(); }, 50);
    }

    close() {
        if (this._overlay) {
            this._overlay.remove();
            this._overlay = null;
        }
    }

    destroy() {
        this.close();
        this._destroyed = true;
    }

    // ── Build overlay DOM ───────────────────────────────────────────────────

    _buildOverlay() {
        var self = this;
        var esc = function(s) {
            return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
        };

        var overlay = document.createElement('div');
        overlay.id = 'extBuilderOverlay';
        overlay.style.cssText = [
            'position:fixed;inset:0;z-index:10000;',
            'background:rgba(0,0,0,0.45);',
            'display:flex;align-items:center;justify-content:center;',
        ].join('');

        var vtOptions = FhirExtensionCatalog.getValueTypes().map(function(vt) {
            var sel = (vt.value === self._selectedVT) ? ' selected' : '';
            return '<option value="' + esc(vt.value) + '"' + sel + ' title="' + esc(vt.description) + '">' + esc(vt.label) + '</option>';
        }).join('');

        var transformOptions = [
            { value: '',                       label: 'Auto (engine selects)' },
            { value: 'hl7_active_flag',        label: 'Y/N → Boolean' },
            { value: 'string_direct',          label: 'Pass-through (string)' },
            { value: 'ce_to_codeableconcept',  label: 'CE → CodeableConcept' },
            { value: 'ts_to_datetime',         label: 'TS → dateTime' },
            { value: 'ts_to_date',             label: 'TS → date' },
            { value: 'gender_mapping',         label: 'Gender (M/F → male/female)' },
            { value: 'cx_to_identifier',       label: 'CX → Identifier' },
        ].map(function(t) {
            var sel = (t.value === self._selectedTrans) ? ' selected' : '';
            return '<option value="' + esc(t.value) + '"' + sel + '>' + esc(t.label) + '</option>';
        }).join('');

        var sourceLabel = this._hl7Field
            ? ('<code style="background:#dbeafe;padding:2px 8px;border-radius:4px;color:#1e3a8a;font-size:0.82rem;">' + esc(this._hl7Field) + '</code>')
            : '<em style="color:#94a3b8;">no source field</em>';

        overlay.innerHTML = [
            '<div style="',
                'background:white;border-radius:12px;width:660px;max-width:95vw;',
                'max-height:90vh;display:flex;flex-direction:column;',
                'box-shadow:0 20px 60px rgba(0,0,0,0.3);overflow:hidden;">',

            // ── Header ─────────────────────────────────────────────────
            '<div style="padding:1rem 1.25rem;background:linear-gradient(135deg,#1e3a8a,#2563eb);color:white;flex-shrink:0;">',
                '<div style="display:flex;align-items:center;justify-content:space-between;">',
                    '<div>',
                        '<div style="font-size:1rem;font-weight:700;">🔌 Map to FHIR Extension</div>',
                        '<div style="font-size:0.78rem;opacity:0.85;margin-top:2px;">',
                            'Source: ', sourceLabel,
                            ' &nbsp;→&nbsp; ',
                            '<span style="background:rgba(255,255,255,0.2);padding:2px 8px;border-radius:4px;">',
                                esc(this._resource), '.extension[…]',
                            '</span>',
                        '</div>',
                    '</div>',
                    '<button id="extBuilderClose" style="background:none;border:none;color:white;font-size:1.3rem;cursor:pointer;line-height:1;padding:0 4px;">&times;</button>',
                '</div>',
            '</div>',

            // ── Scrollable body ─────────────────────────────────────────
            '<div style="flex:1;overflow-y:auto;padding:1.1rem 1.25rem;">',

                // Step 1: Find
                '<div style="margin-bottom:1.1rem;">',
                    '<div style="font-size:0.78rem;font-weight:700;color:#475569;text-transform:uppercase;letter-spacing:0.05em;margin-bottom:0.45rem;">',
                        'Step 1 &mdash; Find extension',
                    '</div>',
                    '<div style="position:relative;">',
                        '<i class="fas fa-search" style="position:absolute;left:0.65rem;top:50%;transform:translateY(-50%);color:#94a3b8;font-size:0.8rem;"></i>',
                        '<input id="extBuilderSearch" type="text" placeholder="Search by name or keyword (e.g. organ donor, race, birth sex)"',
                            ' style="width:100%;padding:0.55rem 0.75rem 0.55rem 2.1rem;border:1px solid #cbd5e1;border-radius:6px;font-size:0.875rem;box-sizing:border-box;outline:none;">',
                    '</div>',
                    '<div id="extBuilderCatalog" style="margin-top:0.5rem;max-height:240px;overflow-y:auto;border:1px solid #e2e8f0;border-radius:6px;background:#fafafa;">',
                        '<div style="padding:1rem;text-align:center;color:#94a3b8;font-size:0.85rem;">Loading…</div>',
                    '</div>',
                '</div>',

                // Step 2: Configure
                '<div style="margin-bottom:1.1rem;padding:0.85rem;border:1px solid #e2e8f0;border-radius:8px;background:#f8fafc;">',
                    '<div style="font-size:0.78rem;font-weight:700;color:#475569;text-transform:uppercase;letter-spacing:0.05em;margin-bottom:0.65rem;">',
                        'Step 2 &mdash; Configure',
                    '</div>',

                    // ── Value type ──────────────────────────────────────────
                    '<div style="margin-bottom:0.65rem;">',
                        '<label style="font-size:0.8rem;font-weight:600;color:#374151;display:block;margin-bottom:0.25rem;">',
                            'Value type *',
                        '</label>',
                        '<select id="extBuilderVT" style="width:100%;padding:0.45rem 0.6rem;border:1px solid #cbd5e1;border-radius:5px;font-size:0.82rem;background:white;">',
                            vtOptions,
                        '</select>',
                        // Auto-selection reason — updated dynamically by _updateValueTypeHint()
                        '<div id="extBuilderVTHint" style="margin-top:0.3rem;font-size:0.75rem;color:#0369a1;background:#e0f2fe;border-radius:4px;padding:0.3rem 0.6rem;display:none;"></div>',
                        '<details style="margin-top:0.25rem;display:inline-block;">',
                            '<summary style="font-size:0.75rem;color:#2563eb;cursor:pointer;list-style:none;user-select:none;">',
                                '❓ Which type should I pick?',
                            '</summary>',
                            ExtensionBuilder._VT_HELP_HTML,
                        '</details>',
                    '</div>',

                    // ── Transform ───────────────────────────────────────────
                    '<div style="margin-bottom:0.65rem;">',
                        '<label style="font-size:0.8rem;font-weight:600;color:#374151;display:block;margin-bottom:0.25rem;">Transform</label>',
                        '<select id="extBuilderTransform" style="width:100%;padding:0.45rem 0.6rem;border:1px solid #cbd5e1;border-radius:5px;font-size:0.82rem;background:white;">',
                            transformOptions,
                        '</select>',
                        '<div style="font-size:0.75rem;color:#64748b;margin-top:0.2rem;">',
                            'How the HL7 value is converted. Leave <strong>Auto</strong> unless you need something specific.',
                        '</div>',
                    '</div>',

                    // ── System URI (coded types only) ───────────────────────
                    '<div id="extBuilderSystemRow" style="margin-bottom:0.65rem;display:none;">',
                        '<label style="font-size:0.8rem;font-weight:600;color:#374151;display:block;margin-bottom:0.25rem;">',
                            'Code System URI',
                            '<span style="font-weight:400;color:#64748b;"> — the authority that defines the valid codes</span>',
                        '</label>',
                        '<input id="extBuilderSystem" type="text" list="extBuilderSystemList"',
                            ' placeholder="Type or pick from common systems…"',
                            ' style="width:100%;padding:0.45rem 0.6rem;border:1px solid #cbd5e1;border-radius:5px;font-size:0.82rem;box-sizing:border-box;outline:none;">',
                        '<datalist id="extBuilderSystemList">',
                            '<option value="http://loinc.org">LOINC</option>',
                            '<option value="http://snomed.info/sct">SNOMED CT</option>',
                            '<option value="http://www.nlm.nih.gov/research/umls/rxnorm">RxNorm</option>',
                            '<option value="http://hl7.org/fhir/sid/icd-10-cm">ICD-10-CM</option>',
                            '<option value="http://hl7.org/fhir/sid/icd-9-cm">ICD-9-CM</option>',
                            '<option value="http://www.ama-assn.org/go/cpt">CPT</option>',
                            '<option value="http://hl7.org/fhir/sid/ndc">NDC (drugs)</option>',
                            '<option value="http://hl7.org/fhir/sid/cvx">CVX (vaccines)</option>',
                            '<option value="urn:oid:2.16.840.1.113883.6.238">CDCREC (race/ethnicity)</option>',
                            '<option value="http://terminology.hl7.org/CodeSystem/v3-AdministrativeGender">AdministrativeGender</option>',
                            '<option value="http://unitsofmeasure.org">UCUM (units)</option>',
                            '<option value="urn:ietf:bcp:47">BCP-47 (languages)</option>',
                            '<option value="urn:iso:std:iso:3166">ISO 3166 (countries)</option>',
                        '</datalist>',
                        '<div style="font-size:0.75rem;color:#64748b;margin-top:0.25rem;">',
                            'Start typing to filter, or pick a common system from the list.',
                            ' Leave blank if the codes are self-explanatory (e.g. Y/N, M/F).',
                        '</div>',
                    '</div>',

                    // ── Custom URL ──────────────────────────────────────────
                    '<div>',
                        '<label style="font-size:0.8rem;font-weight:600;color:#374151;display:block;margin-bottom:0.25rem;">',
                            'Extension URL',
                            ' <span id="extBuilderUrlStatus" style="font-size:0.75rem;font-weight:400;"></span>',
                        '</label>',
                        '<input id="extBuilderUrl" type="text"',
                            ' placeholder="Auto-filled when you pick from the catalog above"',
                            ' style="width:100%;padding:0.45rem 0.6rem;border:1px solid #cbd5e1;border-radius:5px;font-size:0.82rem;box-sizing:border-box;outline:none;"',
                            ' value="">',
                        '<div style="margin-top:0.25rem;display:flex;align-items:center;gap:0.4rem;">',
                            '<span style="font-size:0.75rem;color:#64748b;">Picked from catalog above, or type a custom one.</span>',
                            '<details style="display:inline;">',
                                '<summary style="font-size:0.75rem;color:#2563eb;cursor:pointer;list-style:none;user-select:none;">',
                                    '❓ What should I put here?',
                                '</summary>',
                                ExtensionBuilder._URL_HELP_HTML,
                            '</details>',
                        '</div>',
                    '</div>',

                '</div>',

                // Step 3: Try it
                '<div style="margin-bottom:1rem;padding:0.85rem;border:1px solid #e2e8f0;border-radius:8px;background:#f8fafc;">',
                    '<div style="font-size:0.78rem;font-weight:700;color:#475569;text-transform:uppercase;letter-spacing:0.05em;margin-bottom:0.65rem;">',
                        'Step 3 &mdash; Try it',
                    '</div>',
                    '<div style="display:flex;gap:0.5rem;align-items:center;">',
                        '<label style="font-size:0.82rem;color:#374151;flex-shrink:0;">Sample HL7 value:</label>',
                        '<input id="extBuilderSample" type="text" placeholder="e.g. Y" maxlength="80"',
                            ' style="width:90px;padding:0.4rem 0.5rem;border:1px solid #cbd5e1;border-radius:4px;font-size:0.82rem;outline:none;">',
                        '<span style="font-size:0.85rem;color:#94a3b8;">→</span>',
                        '<code id="extBuilderPreviewValue" style="padding:0.3rem 0.6rem;background:#f1f5f9;border-radius:4px;font-size:0.8rem;color:#1e3a8a;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">',
                            '(enter a sample value above)',
                        '</code>',
                    '</div>',
                    '<div style="margin-top:0.75rem;">',
                        '<div style="font-size:0.75rem;font-weight:600;color:#64748b;margin-bottom:0.3rem;">Generated FHIR path:</div>',
                        '<div style="display:flex;gap:0.4rem;align-items:center;">',
                            '<code id="extBuilderPathPreview" style="flex:1;padding:0.4rem 0.6rem;background:#0f172a;color:#e2e8f0;border-radius:5px;font-size:0.75rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">',
                                '—',
                            '</code>',
                            '<button id="extBuilderCopyPath" title="Copy path" style="',
                                'padding:0.35rem 0.5rem;background:#e0f2fe;border:1px solid #bae6fd;border-radius:4px;',
                                'cursor:pointer;font-size:0.78rem;color:#0369a1;flex-shrink:0;">',
                                '<i class="fas fa-copy"></i>',
                            '</button>',
                        '</div>',
                    '</div>',
                    '<div style="margin-top:0.65rem;">',
                        '<div style="font-size:0.75rem;font-weight:600;color:#64748b;margin-bottom:0.3rem;">FHIR JSON preview:</div>',
                        '<pre id="extBuilderJsonPreview" style="',
                            'margin:0;padding:0.5rem 0.75rem;background:#0f172a;color:#86efac;',
                            'border-radius:5px;font-size:0.75rem;overflow-x:auto;white-space:pre;max-height:120px;overflow-y:auto;">',
                            '{ }',
                        '</pre>',
                    '</div>',
                '</div>',

            '</div>', // scrollable body

            // ── Footer ──────────────────────────────────────────────────
            '<div style="padding:0.85rem 1.25rem;border-top:1px solid #e2e8f0;display:flex;justify-content:space-between;align-items:center;flex-shrink:0;background:#f8fafc;">',
                '<button id="extBuilderDefineCustom" style="',
                    'background:none;border:1px solid #cbd5e1;border-radius:5px;',
                    'padding:0.45rem 0.85rem;cursor:pointer;font-size:0.82rem;color:#374151;">',
                    '+ Define custom extension',
                '</button>',
                '<div style="display:flex;gap:0.5rem;">',
                    '<button id="extBuilderCancel" style="',
                        'background:none;border:1px solid #cbd5e1;border-radius:5px;',
                        'padding:0.45rem 0.85rem;cursor:pointer;font-size:0.82rem;color:#374151;">',
                        'Cancel',
                    '</button>',
                    '<button id="extBuilderConfirm" style="',
                        'background:#1e3a8a;color:white;border:none;border-radius:5px;',
                        'padding:0.45rem 1.1rem;cursor:pointer;font-size:0.82rem;font-weight:600;">',
                        'Use this extension →',
                    '</button>',
                '</div>',
            '</div>',

            '</div>', // modal card
        ].join('');

        this._overlay = overlay;
        this._attachOverlayEvents();
    }

    // ── Event wiring ────────────────────────────────────────────────────────

    _attachOverlayEvents() {
        var self = this;

        // Close on overlay background click
        this._overlay.addEventListener('click', function(e) {
            if (e.target === self._overlay) self.close();
        });

        var get = function(id) { return self._overlay.querySelector('#' + id); };

        get('extBuilderClose').addEventListener('click', function() { self.close(); });
        get('extBuilderCancel').addEventListener('click', function() { self.close(); });

        // Search
        get('extBuilderSearch').addEventListener('input', function() {
            self._searchQuery = this.value;
            self._renderCatalog(this.value);
        });

        // Value type change
        get('extBuilderVT').addEventListener('change', function() {
            self._selectedVT = this.value;
            self._toggleSystemRow();
            self._updatePreview();
        });

        // Transform change
        get('extBuilderTransform').addEventListener('change', function() {
            self._selectedTrans = this.value;
            self._updatePreview();
        });

        // System URI change
        get('extBuilderSystem').addEventListener('input', function() {
            self._selectedSystem = this.value.trim();
            self._updatePreview();
        });

        // Custom URL input
        get('extBuilderUrl').addEventListener('input', function() {
            self._selectedUrl = this.value.trim();
            self._selectedEntry = null;
            self._updateUrlStatus();
            self._updatePreview();
        });

        // Sample value live transform preview
        get('extBuilderSample').addEventListener('input', function() {
            self._updateSamplePreview(this.value);
        });

        // Copy path
        get('extBuilderCopyPath').addEventListener('click', function() {
            var code = get('extBuilderPathPreview');
            if (code && code.textContent && code.textContent !== '—') {
                navigator.clipboard && navigator.clipboard.writeText(code.textContent);
                this.innerHTML = '<i class="fas fa-check"></i>';
                var btn = this;
                setTimeout(function() { btn.innerHTML = '<i class="fas fa-copy"></i>'; }, 1500);
            }
        });

        // Define custom
        get('extBuilderDefineCustom').addEventListener('click', function() {
            self._openCustomForm();
        });

        // Confirm
        get('extBuilderConfirm').addEventListener('click', function() {
            self._confirm();
        });
    }

    // ── Catalog rendering ───────────────────────────────────────────────────

    _renderCatalog(query) {
        var self = this;
        var container = this._overlay.querySelector('#extBuilderCatalog');
        if (!container) return;

        var esc = function(s) {
            return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
        };

        var html = [];

        // Suggested (only when no active search)
        if (!query || !query.trim()) {
            var suggestions = FhirExtensionCatalog.getSuggestions(this._hl7Field);
            if (suggestions.length > 0) {
                html.push('<div style="padding:0.3rem 0.6rem;background:#fef9c3;font-size:0.72rem;font-weight:700;color:#92400e;border-bottom:1px solid #fde68a;">✨ Suggested for ' + esc(this._hl7Field) + '</div>');
                suggestions.forEach(function(ext) {
                    html.push(self._renderExtRow(ext, esc, true));
                });
            }
        }

        // Search results or grouped catalog
        if (query && query.trim()) {
            var results = FhirExtensionCatalog.search(query);
            if (results.length === 0) {
                html.push('<div style="padding:1rem;text-align:center;color:#94a3b8;font-size:0.85rem;">No extensions match "' + esc(query) + '"</div>');
            } else {
                html.push('<div style="padding:0.3rem 0.6rem;background:#f1f5f9;font-size:0.72rem;font-weight:700;color:#475569;border-bottom:1px solid #e2e8f0;">Search results</div>');
                results.forEach(function(ext) {
                    html.push(self._renderExtRow(ext, esc, false));
                });
            }
        } else {
            // Grouped catalog
            FhirExtensionCatalog.getGroups().forEach(function(group) {
                html.push(
                    '<div style="padding:0.3rem 0.6rem;background:#f1f5f9;font-size:0.72rem;font-weight:700;color:#475569;border-bottom:1px solid #e2e8f0;border-top:1px solid #e2e8f0;">',
                    group.icon + ' ' + esc(group.category),
                    '</div>'
                );
                group.extensions.forEach(function(ext) {
                    html.push(self._renderExtRow(ext, esc, false));
                });
            });
        }

        container.innerHTML = html.join('');

        // Wire row clicks
        container.querySelectorAll('.ext-row').forEach(function(row) {
            row.addEventListener('click', function() {
                var id = row.dataset.extId;
                self._selectExtension(id);
                // Highlight selected row
                container.querySelectorAll('.ext-row').forEach(function(r) { r.style.background = ''; });
                row.style.background = '#dbeafe';
            });
        });
    }

    _renderExtRow(ext, esc, highlighted) {
        var bg = highlighted ? '#fefce8' : '';
        return [
            '<div class="ext-row" data-ext-id="' + esc(ext.id) + '"',
                ' style="padding:0.5rem 0.75rem;cursor:pointer;border-bottom:1px solid #f1f5f9;',
                'display:flex;align-items:flex-start;justify-content:space-between;gap:0.5rem;',
                'background:' + bg + ';transition:background 0.1s;"',
                ' onmouseover="if(this.style.background!==\'#dbeafe\')this.style.background=\'#f0f9ff\'"',
                ' onmouseout="if(this.style.background!==\'#dbeafe\')this.style.background=\'' + bg + '\'">',
                '<div style="flex:1;min-width:0;">',
                    '<div style="font-size:0.83rem;font-weight:600;color:#1e3a8a;">' + esc(ext.name) + '</div>',
                    '<div style="font-size:0.75rem;color:#64748b;margin-top:1px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="' + esc(ext.url) + '">',
                        esc(ext.url),
                    '</div>',
                    ext.description ? '<div style="font-size:0.75rem;color:#475569;margin-top:1px;">' + esc(ext.description) + '</div>' : '',
                '</div>',
                '<code style="background:#dbeafe;padding:2px 6px;border-radius:3px;font-size:0.72rem;color:#1e3a8a;white-space:nowrap;flex-shrink:0;">',
                    esc(ext.valueType),
                '</code>',
            '</div>',
        ].join('');
    }

    // ── Selection handling ──────────────────────────────────────────────────

    _selectExtension(id) {
        var ext = FhirExtensionCatalog.getById(id);
        if (!ext) return;

        this._selectedEntry = ext;
        this._selectedUrl   = ext.url;
        this._selectedVT    = ext.valueType || this._selectedVT;
        this._selectedTrans = ext.transform  || FhirExtensionCatalog.suggestTransform(this._hl7DataType, this._selectedVT);
        this._selectedSystem = ext.system || '';

        // Sync controls
        var get = function(id) { return this._overlay.querySelector('#' + id); }.bind(this);
        var vtSel = get('extBuilderVT');
        var transSel = get('extBuilderTransform');
        var urlInput = get('extBuilderUrl');
        var sysInput = get('extBuilderSystem');

        if (vtSel)    vtSel.value    = this._selectedVT;
        if (transSel) transSel.value = this._selectedTrans;
        if (urlInput) urlInput.value = this._selectedUrl;
        if (sysInput) sysInput.value = this._selectedSystem;

        this._toggleSystemRow();
        this._updatePreview();
    }

    // ── System row visibility ───────────────────────────────────────────────

    _toggleSystemRow() {
        var codedTypes = ['valueCode', 'valueCoding', 'valueCodeableConcept'];
        var show = codedTypes.indexOf(this._selectedVT) >= 0;
        var row = this._overlay && this._overlay.querySelector('#extBuilderSystemRow');
        if (row) row.style.display = show ? '' : 'none';
        this._updateValueTypeHint();
        this._updateUrlStatus();
    }

    // Shows WHY the value type was auto-selected and what it means in plain English.
    _updateValueTypeHint() {
        var hint = this._overlay && this._overlay.querySelector('#extBuilderVTHint');
        if (!hint) return;

        var HL7_LABELS = {
            'IS': 'IS (coded value, table-defined)',
            'ID': 'ID (HL7-defined code)',
            'ST': 'ST (string)',
            'FT': 'FT (formatted text)',
            'TX': 'TX (text data)',
            'NM': 'NM (numeric)',
            'SI': 'SI (sequence ID)',
            'DT': 'DT (date)',
            'TS': 'TS (timestamp)',
            'DTM': 'DTM (date/time)',
            'CE': 'CE (coded element)',
            'CWE': 'CWE (coded with exceptions)',
            'CNE': 'CNE (coded, no exceptions)',
        };

        var VT_PLAIN = {
            'valueBoolean':         'stores true or false — HL7 Y/N becomes true/false',
            'valueString':          'stores plain text — any string value',
            'valueCode':            'stores a short code from a defined value set',
            'valueCodeableConcept': 'stores a coded concept with system, code, and display text',
            'valueCoding':          'stores a single code with its system URI',
            'valueQuantity':        'stores a number plus unit of measure',
            'valueDateTime':        'stores a date and time in ISO 8601 format',
            'valueDate':            'stores a date only (no time)',
            'valueInteger':         'stores a whole number',
            'valueDecimal':         'stores a decimal number',
            'valueUri':             'stores a URL or URN',
            'valueReference':       'stores a reference to another FHIR resource',
        };

        var suggested = FhirExtensionCatalog.suggestValueType(this._hl7DataType);
        var plain = VT_PLAIN[this._selectedVT] || '';
        var hl7Label = HL7_LABELS[this._hl7DataType] || this._hl7DataType;

        if (!this._hl7DataType && !plain) {
            hint.style.display = 'none';
            return;
        }

        var parts = [];
        if (this._hl7DataType && this._selectedVT === suggested) {
            parts.push('✓ Auto-selected from HL7 type <strong>' + hl7Label + '</strong>.');
        } else if (this._hl7DataType && this._selectedVT !== suggested) {
            parts.push('ℹ️ HL7 type <strong>' + hl7Label + '</strong> — overridden from suggested <code>' + suggested + '</code>.');
        }
        if (plain) {
            parts.push('This type ' + plain + '.');
        }

        hint.innerHTML = parts.join(' ');
        hint.style.display = parts.length > 0 ? '' : 'none';
    }

    // Shows a green ✓ or red warning next to the URL field.
    _updateUrlStatus() {
        var statusEl = this._overlay && this._overlay.querySelector('#extBuilderUrlStatus');
        if (!statusEl) return;
        var url = this._resolveUrl();
        if (!url) {
            statusEl.textContent = '';
            return;
        }
        var valid = /^https?:\/\/.+\..+/.test(url);
        if (valid) {
            statusEl.style.color = '#059669';
            statusEl.textContent = '✓ valid URL';
        } else {
            statusEl.style.color = '#dc2626';
            statusEl.textContent = '⚠ URL should start with http:// or https://';
        }
    }

    // ── Preview update ──────────────────────────────────────────────────────

    _updatePreview() {
        var url = this._resolveUrl();
        var pathEl  = this._overlay && this._overlay.querySelector('#extBuilderPathPreview');
        var jsonEl  = this._overlay && this._overlay.querySelector('#extBuilderJsonPreview');
        if (!pathEl || !jsonEl) return;

        if (!url) {
            pathEl.textContent = '— select an extension above —';
            jsonEl.textContent = '{ }';
            return;
        }

        var path = FhirExtensionCatalog.buildPath(this._resource, url, this._selectedVT);
        pathEl.textContent = path;

        // Build JSON preview with a placeholder value based on valueType
        var sampleInput = (this._overlay.querySelector('#extBuilderSample') || {}).value || '';
        var previewValue = this._applyTransform(sampleInput || this._placeholderValue(), this._selectedTrans, this._selectedVT);
        var extObj = { url: url };
        extObj[this._selectedVT] = previewValue;
        jsonEl.textContent = JSON.stringify(
            { extension: [ extObj ] },
            null, 2
        );
    }

    _updateSamplePreview(rawValue) {
        var previewEl = this._overlay && this._overlay.querySelector('#extBuilderPreviewValue');
        if (!previewEl || !rawValue) {
            if (previewEl) previewEl.textContent = '(enter a sample value above)';
            this._updatePreview();
            return;
        }
        var result = this._applyTransform(rawValue, this._selectedTrans, this._selectedVT);
        previewEl.textContent = JSON.stringify(result);
        this._updatePreview();
    }

    _placeholderValue() {
        var defaults = {
            'valueBoolean':         'Y',
            'valueString':          'sample text',
            'valueCode':            'ABC',
            'valueInteger':         '42',
            'valueDecimal':         '3.14',
            'valueDate':            '20240115',
            'valueDateTime':        '20240115120000',
            'valueUri':             'http://example.org',
            'valueCoding':          'A01^Active^HL70001',
            'valueCodeableConcept': 'A01^Active^HL70001',
            'valueQuantity':        '180^cm',
        };
        return defaults[this._selectedVT] || 'value';
    }

    _applyTransform(raw, transform, valueType) {
        if (!raw) return null;
        switch (transform) {
            case 'hl7_active_flag':
                return /^(Y|1|TRUE|A|ACTIVE)$/i.test(raw.trim());
            case 'ts_to_datetime':
            case 'ts_to_date': {
                var s = raw.trim().replace(/[^0-9]/g, '');
                if (s.length >= 8) return s.slice(0,4) + '-' + s.slice(4,6) + '-' + s.slice(6,8);
                return raw;
            }
            case 'gender_mapping': {
                var g = { M: 'male', F: 'female', O: 'other', U: 'unknown' };
                return g[raw.toUpperCase()] || 'unknown';
            }
            case 'ce_to_codeableconcept': {
                var parts = raw.split('^');
                var coding = { code: parts[0] || raw };
                if (parts[1]) coding.display = parts[1];
                if (parts[2]) coding.system = parts[2];
                return { coding: [ coding ], text: parts[1] || parts[0] };
            }
            default:
                if (valueType === 'valueBoolean') return /^(Y|1|TRUE|A)$/i.test(raw.trim());
                if (valueType === 'valueInteger') return parseInt(raw, 10) || 0;
                if (valueType === 'valueDecimal') return parseFloat(raw) || 0;
                return raw;
        }
    }

    _resolveUrl() {
        // Custom URL input takes precedence over catalog selection
        var urlInput = this._overlay && this._overlay.querySelector('#extBuilderUrl');
        var custom = urlInput ? urlInput.value.trim() : '';
        return custom || this._selectedUrl || '';
    }

    // ── Custom extension form ───────────────────────────────────────────────

    _openCustomForm() {
        var self = this;
        var esc = function(s) { return String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;'); };

        // Slug helper — "Interpreter Required" → "interpreter-required"
        var toSlug = function(s) {
            return s.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
        };

        // Base URL prefix for custom extensions.
        // We try to derive it from the window location (so it's at least a real
        // domain), but the user must edit it to their org's actual FHIR base URL.
        var ORG_BASE = (function() {
            try {
                var host = window.location.hostname;
                // Localhost / IP → use a clear placeholder that signals "change me"
                if (host === 'localhost' || /^\d+\.\d+\.\d+\.\d+$/.test(host)) {
                    return 'http://yourorg.com/fhir/StructureDefinition/';
                }
                return 'http://' + host + '/fhir/StructureDefinition/';
            } catch (e) {
                return 'http://yourorg.com/fhir/StructureDefinition/';
            }
        }());

        // Suggested value type for this HL7 source field
        var suggestedVT = FhirExtensionCatalog.suggestValueType(self._hl7DataType) || 'valueString';

        var vtOptionsHtml = FhirExtensionCatalog.getValueTypes().map(function(v) {
            var sel = (v.value === suggestedVT) ? ' selected' : '';
            return '<option value="' + esc(v.value) + '"' + sel + '>' + esc(v.label) + ' — ' + esc(v.description) + '</option>';
        }).join('');

        var formOverlay = document.createElement('div');
        formOverlay.style.cssText = 'position:fixed;inset:0;z-index:10001;background:rgba(0,0,0,0.35);display:flex;align-items:center;justify-content:center;';
        formOverlay.innerHTML = [
            '<div style="background:white;border-radius:10px;width:520px;max-width:95vw;max-height:90vh;display:flex;flex-direction:column;box-shadow:0 16px 48px rgba(0,0,0,0.25);overflow:hidden;">',

                // Header
                '<div style="padding:0.85rem 1.1rem;background:#1e3a8a;color:white;font-weight:700;font-size:0.95rem;flex-shrink:0;">',
                    '+ Define Custom Extension',
                '</div>',

                // Body
                '<div style="padding:1rem 1.1rem;display:grid;gap:0.75rem;overflow-y:auto;flex:1;">',

                    // ── Name ──────────────────────────────────────────────
                    '<div>',
                        '<label style="font-size:0.8rem;font-weight:600;color:#374151;display:block;margin-bottom:0.2rem;">Name *</label>',
                        '<input id="customExtName" type="text" placeholder="e.g. Interpreter Required"',
                            ' style="width:100%;padding:0.45rem 0.6rem;border:1px solid #cbd5e1;border-radius:5px;font-size:0.82rem;box-sizing:border-box;outline:none;">',
                        '<div style="font-size:0.75rem;color:#64748b;margin-top:0.2rem;">',
                            'A short, descriptive name. Used only for display — not stored in FHIR.',
                        '</div>',
                    '</div>',

                    // ── URL ───────────────────────────────────────────────
                    '<div>',
                        '<label style="font-size:0.8rem;font-weight:600;color:#374151;display:flex;align-items:center;gap:0.4rem;margin-bottom:0.2rem;">',
                            'Extension URL *',
                            '<span id="customExtUrlStatus" style="font-size:0.75rem;font-weight:400;"></span>',
                        '</label>',
                        '<input id="customExtUrl" type="text"',
                            ' placeholder="Auto-generated from name — or type your own"',
                            ' style="width:100%;padding:0.45rem 0.6rem;border:1px solid #cbd5e1;border-radius:5px;font-size:0.82rem;box-sizing:border-box;outline:none;font-family:monospace;">',
                        '<div style="margin-top:0.2rem;font-size:0.75rem;color:#64748b;">',
                            'Auto-filled from the name. ',
                            '<span id="customExtUrlHelpToggle" style="color:#2563eb;cursor:pointer;text-decoration:underline;">',
                                '❓ What should I put here?',
                            '</span>',
                            '<div id="customExtUrlHelp" style="display:none;margin-top:0.4rem;padding:0.6rem 0.75rem;background:#f0f9ff;border:1px solid #bae6fd;border-radius:6px;line-height:1.7;color:#374151;">',
                                'The URL is just a <strong>unique ID</strong> for this type of data in FHIR. ',
                                'It doesn\'t need to be a working website.<br>',
                                'Use your organisation\'s domain name:<br>',
                                '<code style="background:#dbeafe;padding:1px 6px;border-radius:3px;color:#1e3a8a;">http://yourhospital.org/fhir/extensions/interpreter</code><br>',
                                '<br>',
                                '<strong>Rules:</strong>',
                                '<ul style="margin:0.2rem 0 0 1rem;padding:0;">',
                                    '<li>Start with <code>http://</code> or <code>https://</code></li>',
                                    '<li>Use a domain name you own (replace <em>yourhospital.org</em>)</li>',
                                    '<li>Once set, <strong>never change it</strong> — trading partners use it to identify the field</li>',
                                '</ul>',
                            '</div>',
                        '</div>',
                    '</div>',

                    // ── Value type ────────────────────────────────────────
                    '<div>',
                        '<label style="font-size:0.8rem;font-weight:600;color:#374151;display:block;margin-bottom:0.2rem;">',
                            'Value type *',
                            self._hl7DataType
                                ? ' <span style="font-weight:400;font-size:0.75rem;color:#0369a1;">— auto-suggested from HL7 type <strong>' + esc(self._hl7DataType) + '</strong></span>'
                                : '',
                        '</label>',
                        '<select id="customExtVT" style="width:100%;padding:0.45rem 0.6rem;border:1px solid #cbd5e1;border-radius:5px;font-size:0.82rem;background:white;">',
                            vtOptionsHtml,
                        '</select>',
                        '<div id="customExtVTDesc" style="font-size:0.75rem;color:#0369a1;background:#e0f2fe;border-radius:4px;padding:0.3rem 0.6rem;margin-top:0.25rem;"></div>',
                    '</div>',

                    // ── Description ───────────────────────────────────────
                    '<div>',
                        '<label style="font-size:0.8rem;font-weight:600;color:#374151;display:block;margin-bottom:0.2rem;">',
                            'Description <span style="font-weight:400;color:#94a3b8;">(optional)</span>',
                        '</label>',
                        '<input id="customExtDesc" type="text" placeholder="e.g. Whether the patient needs an interpreter"',
                            ' style="width:100%;padding:0.45rem 0.6rem;border:1px solid #cbd5e1;border-radius:5px;font-size:0.82rem;box-sizing:border-box;outline:none;">',
                    '</div>',

                    // ── Live FHIR preview ─────────────────────────────────
                    '<div>',
                        '<div style="font-size:0.75rem;font-weight:600;color:#64748b;margin-bottom:0.3rem;">',
                            'FHIR output preview <span style="font-weight:400;">(updates as you fill in the form)</span>',
                        '</div>',
                        '<pre id="customExtPreview" style="',
                            'margin:0;padding:0.5rem 0.75rem;background:#0f172a;color:#86efac;',
                            'border-radius:5px;font-size:0.75rem;white-space:pre;overflow-x:auto;">',
                            '{ }',
                        '</pre>',
                    '</div>',

                    // ── Inline error message ──────────────────────────────
                    '<div id="customExtError" style="display:none;padding:0.45rem 0.65rem;background:#fef2f2;border:1px solid #fecaca;border-radius:5px;font-size:0.8rem;color:#dc2626;"></div>',

                '</div>',

                // Footer
                '<div style="padding:0.75rem 1.1rem;border-top:1px solid #e2e8f0;display:flex;justify-content:flex-end;gap:0.5rem;flex-shrink:0;">',
                    '<button id="customExtCancel" style="padding:0.45rem 0.85rem;border:1px solid #cbd5e1;border-radius:5px;cursor:pointer;font-size:0.82rem;background:white;">Cancel</button>',
                    '<button id="customExtSave" style="padding:0.45rem 1rem;background:#1e3a8a;color:white;border:none;border-radius:5px;cursor:pointer;font-size:0.82rem;font-weight:600;">Add extension →</button>',
                '</div>',

            '</div>',
        ].join('');

        document.body.appendChild(formOverlay);

        // ── Internal helpers ────────────────────────────────────────────────

        var q = function(id) { return formOverlay.querySelector('#' + id); };

        // VT descriptions for the hint line
        var VT_DESCRIPTIONS = {};
        FhirExtensionCatalog.getValueTypes().forEach(function(v) {
            VT_DESCRIPTIONS[v.value] = v.description;
        });

        var updateVTDesc = function() {
            var vt = q('customExtVT').value;
            var desc = VT_DESCRIPTIONS[vt] || '';
            var el = q('customExtVTDesc');
            el.textContent = desc ? '💡 ' + desc : '';
            el.style.display = desc ? '' : 'none';
        };

        var validateUrl = function(url) {
            return /^https?:\/\/.+\..+/.test(url);
        };

        var updateUrlStatus = function() {
            var url = q('customExtUrl').value.trim();
            var statusEl = q('customExtUrlStatus');
            if (!url) {
                statusEl.textContent = '';
                return;
            }
            if (validateUrl(url)) {
                statusEl.style.color = '#059669';
                statusEl.textContent = '✓';
            } else {
                statusEl.style.color = '#dc2626';
                statusEl.textContent = '⚠ must start with http:// or https://';
            }
        };

        var updatePreview = function() {
            var url = q('customExtUrl').value.trim() || ORG_BASE + 'my-extension';
            var vt  = q('customExtVT').value || 'valueString';
            var placeholders = {
                'valueBoolean': true, 'valueString': '"sample text"',
                'valueCode': '"ABC"', 'valueCodeableConcept': '{"coding":[{"code":"ABC"}]}',
                'valueCoding': '{"code":"ABC","system":"http://..."}',
                'valueDateTime': '"2024-01-15T00:00:00"', 'valueDate': '"2024-01-15"',
                'valueInteger': 42, 'valueDecimal': 3.14,
                'valueUri': '"http://example.org"', 'valueQuantity': '{"value":180,"unit":"cm"}',
            };
            var sampleVal = placeholders[vt] !== undefined ? placeholders[vt] : '"…"';
            var line = '"' + vt + '": ' + (typeof sampleVal === 'string' ? sampleVal : JSON.stringify(sampleVal));
            q('customExtPreview').textContent = '{\n  "extension": [{\n    "url": "' + url + '",\n    ' + line + '\n  }]\n}';
        };

        // ── Help toggle ─────────────────────────────────────────────────────
        var helpToggle = q('customExtUrlHelpToggle');
        var helpPanel  = q('customExtUrlHelp');
        if (helpToggle && helpPanel) {
            helpToggle.addEventListener('click', function() {
                var shown = helpPanel.style.display !== 'none';
                helpPanel.style.display = shown ? 'none' : '';
                helpToggle.textContent  = shown ? '❓ What should I put here?' : '✕ Close help';
            });
        }

        // ── Auto-generate URL from name ─────────────────────────────────────
        // Only auto-fills when the URL field has not been manually edited.
        var urlManuallyEdited = false;

        q('customExtName').addEventListener('input', function() {
            if (!urlManuallyEdited) {
                var slug = toSlug(this.value);
                q('customExtUrl').value = slug ? ORG_BASE + slug : '';
            }
            updateUrlStatus();
            updatePreview();
        });

        q('customExtUrl').addEventListener('input', function() {
            urlManuallyEdited = true;
            updateUrlStatus();
            updatePreview();
        });

        q('customExtVT').addEventListener('change', function() {
            updateVTDesc();
            updatePreview();
        });

        // Initial state
        updateVTDesc();
        updatePreview();

        // ── Close ───────────────────────────────────────────────────────────
        q('customExtCancel').addEventListener('click', function() { formOverlay.remove(); });
        formOverlay.addEventListener('click', function(e) { if (e.target === formOverlay) formOverlay.remove(); });

        // ── Save ────────────────────────────────────────────────────────────
        q('customExtSave').addEventListener('click', function() {
            var name = q('customExtName').value.trim();
            var url  = q('customExtUrl').value.trim();
            var vt   = q('customExtVT').value;
            var desc = q('customExtDesc').value.trim();
            var errEl = q('customExtError');

            // Inline validation — no alert()
            var errors = [];
            if (!name) errors.push('Name is required.');
            if (!url)  errors.push('URL is required.');
            else if (!validateUrl(url)) errors.push('URL must start with http:// or https://');

            if (errors.length > 0) {
                errEl.textContent = errors.join(' ');
                errEl.style.display = '';
                return;
            }
            errEl.style.display = 'none';

            var entry = {
                id:          'custom_' + Date.now(),
                name:        name,
                url:         url,
                valueType:   vt,
                transform:   FhirExtensionCatalog.suggestTransform(self._hl7DataType, vt),
                description: desc,
                resources:   [ self._resource ],
            };

            FhirExtensionCatalog.saveCustomExtension(entry);
            formOverlay.remove();

            // Select the newly defined extension in the parent builder
            self._selectedEntry = entry;
            self._selectedUrl   = entry.url;
            self._selectedVT    = entry.valueType;
            self._selectedTrans = entry.transform;

            var overlay = self._overlay;
            if (overlay) {
                var urlInput = overlay.querySelector('#extBuilderUrl');
                var vtSel    = overlay.querySelector('#extBuilderVT');
                var transSel = overlay.querySelector('#extBuilderTransform');
                if (urlInput) urlInput.value = entry.url;
                if (vtSel)    vtSel.value    = entry.valueType;
                if (transSel) transSel.value = entry.transform;
            }
            self._toggleSystemRow();
            self._renderCatalog('');
            self._updatePreview();
        });
    }

    // ── Confirm ─────────────────────────────────────────────────────────────

    _confirm() {
        var url = this._resolveUrl();
        if (!url) {
            alert('Please select or enter an extension URL.');
            return;
        }

        var vtSel = this._overlay && this._overlay.querySelector('#extBuilderVT');
        var valueType = (vtSel && vtSel.value) ? vtSel.value : this._selectedVT;

        var transSel = this._overlay && this._overlay.querySelector('#extBuilderTransform');
        var transform = (transSel && transSel.value !== undefined) ? transSel.value : this._selectedTrans;

        var sysInput = this._overlay && this._overlay.querySelector('#extBuilderSystem');
        var system = (sysInput && sysInput.value) ? sysInput.value.trim() : this._selectedSystem;

        var fhirPath = FhirExtensionCatalog.buildPath(this._resource, url, valueType);

        this._onConfirm({
            fhirPath:  fhirPath,
            url:       url,
            valueType: valueType,
            transform: transform,
            system:    system,
        });

        this.close();
    }
}

// ── Static help content (plain language, shown on "?" click) ─────────────────

/**
 * Plain-language explanation of what an extension URL is and what to put there.
 * Shown as a collapsible panel next to the URL field.
 */
ExtensionBuilder._URL_HELP_HTML = [
    '<div style="margin-top:0.4rem;padding:0.65rem 0.85rem;background:#f0f9ff;',
        'border:1px solid #bae6fd;border-radius:6px;font-size:0.8rem;',
        'color:#374151;line-height:1.75;max-width:420px;">',
        '<strong>The URL is just a unique ID for this field in FHIR.</strong>',
        ' It doesn\'t need to be a real website.<br><br>',
        '<strong>Use your organisation\'s domain name:</strong><br>',
        '<code style="background:#dbeafe;padding:2px 7px;border-radius:3px;color:#1e3a8a;">',
            'http://<em>yourhospital.org</em>/fhir/extensions/organ-donor',
        '</code><br><br>',
        '<strong>Rules to follow:</strong>',
        '<ol style="margin:0.3rem 0 0 1.1rem;padding:0;">',
            '<li>Replace <em>yourhospital.org</em> with your real domain</li>',
            '<li>Once you start using it, <strong>never change it</strong></li>',
            '<li>Each type of custom data needs its own unique URL</li>',
        '</ol>',
        '<br>',
        '<span style="color:#64748b;">If you\'re using a standard extension from the catalog, ',
        'the URL is already filled in for you.</span>',
    '</div>',
].join('');

/**
 * Plain-language guide to picking the right value[x] type.
 * Shown as a collapsible panel next to the Value type dropdown.
 */
ExtensionBuilder._VT_HELP_HTML = [
    '<div style="margin-top:0.4rem;padding:0.65rem 0.85rem;background:#f0f9ff;',
        'border:1px solid #bae6fd;border-radius:6px;font-size:0.8rem;',
        'color:#374151;line-height:1.75;max-width:420px;">',
        '<strong>Pick the type that matches the kind of data you\'re storing:</strong>',
        '<ul style="margin:0.4rem 0 0 1.1rem;padding:0;">',
            '<li><strong>Boolean</strong> — Yes/No fields (Y or N in HL7). Example: organ donor, interpreter needed.</li>',
            '<li><strong>String</strong> — Free text. Example: a name, a note, a description.</li>',
            '<li><strong>Code</strong> — A short code from a fixed list. Example: M for male, active, completed.</li>',
            '<li><strong>CodeableConcept</strong> — A code with a system and display text (CE or CWE fields). Most coded HL7 fields use this.</li>',
            '<li><strong>DateTime</strong> — A date and time (TS fields). Example: 2024-01-15T09:30:00.</li>',
            '<li><strong>Integer / Decimal</strong> — Numbers. Use Decimal when the value has decimal places.</li>',
        '</ul>',
        '<br>',
        '<span style="color:#64748b;">When in doubt: if your HL7 field contains Y/N, pick <strong>Boolean</strong>. ',
        'If it contains a code or description, pick <strong>CodeableConcept</strong>. ',
        'For everything else, <strong>String</strong> is a safe default.</span>',
    '</div>',
].join('');
