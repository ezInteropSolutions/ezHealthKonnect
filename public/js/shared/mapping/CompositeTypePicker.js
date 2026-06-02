/**
 * CompositeTypePicker
 *
 * Guided UI for mapping HL7 composite type fields (XPN, XAD, XTN, CE, CX, …).
 * Renders a mode selector (whole-object | individual component) and a
 * component table. When the user makes a selection, fires a CustomEvent on
 * the container element so the host (PropertiesPanel) can populate the
 * form fields without knowing the picker's internals.
 *
 * Depends on: HL7TypeCatalog (must be loaded before this file)
 *
 * Usage:
 *   const picker = new CompositeTypePicker(containerEl, {
 *       hl7Field:     'PID.5',
 *       hl7DataType:  'XPN',
 *       resourceType: 'Patient',
 *   });
 *   picker.render();
 *   containerEl.addEventListener('mapping-selected', e => {
 *       // e.detail: { hl7Field, fhirPath, transformType, hl7DataType }
 *   });
 *   containerEl.addEventListener('composite-dismissed', () => {
 *       // user clicked "Type manually"
 *   });
 *
 * Lifecycle: always call destroy() before removing the container from the DOM.
 */
class CompositeTypePicker {

    /**
     * @param {HTMLElement} containerEl  Element to render into
     * @param {Object}      options
     * @param {string}      options.hl7Field      Base HL7 path (e.g. "PID.5")
     * @param {string}      options.hl7DataType   HL7 data type code (e.g. "XPN")
     * @param {string}      [options.resourceType] FHIR resource type for path prefix (e.g. "Patient")
     */
    constructor(containerEl, options) {
        this._container   = containerEl;
        this._hl7Field    = (options.hl7Field    || '').trim();
        this._hl7DataType = (options.hl7DataType || '').toUpperCase();
        this._resource    = (options.resourceType || '').trim();
        this._mode        = 'whole';   // 'whole' | 'component'
        this._destroyed   = false;
    }

    // ── Public API ──────────────────────────────────────────────────────────

    render() {
        if (this._destroyed) return;

        var self         = this;
        var components   = HL7TypeCatalog.getComponents(this._hl7DataType);
        var wholeHint    = HL7TypeCatalog.getWholeObjectHint(this._hl7DataType);
        var esc          = function(s) {
            return String(s)
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;');
        };

        var resourcePrefix  = this._resource ? (this._resource + '.') : '';
        var hasFixedFhir    = wholeHint && wholeHint.fhirHint !== '(target path)';
        var wholePathDisplay = hasFixedFhir ? resourcePrefix + wholeHint.fhirHint : '…';

        this._container.innerHTML = [
            '<div class="composite-picker" style="',
                'border:1px solid #bae6fd;border-radius:8px;background:#f0f9ff;overflow:hidden;font-size:0.85rem;">',

            // ── Header ────────────────────────────────────────────────────
            '<div style="padding:0.6rem 0.85rem;background:#0369a1;color:white;',
                'display:flex;align-items:center;gap:0.5rem;">',
                '<i class="fas fa-layer-group" style="font-size:0.78rem;"></i>',
                '<strong>' + esc(this._hl7DataType) + '</strong>',
                '<span style="font-weight:400;opacity:0.9;">composite field — choose mapping mode</span>',
            '</div>',

            // ── Mode selector ─────────────────────────────────────────────
            '<div style="padding:0.75rem 0.85rem;border-bottom:1px solid #bae6fd;">',
                // Whole-object option
                '<label id="compPickerWholeLabel" style="display:flex;align-items:flex-start;gap:0.6rem;',
                    'cursor:pointer;margin-bottom:0.6rem;">',
                    '<input type="radio" name="compositeMode_' + this._uid() + '" value="whole" checked',
                        ' style="margin-top:3px;flex-shrink:0;">',
                    '<div>',
                        '<div style="font-weight:600;color:#0c4a6e;">',
                            'Map as ' + esc((wholeHint && wholeHint.label) || 'whole object'),
                        '</div>',
                        wholeHint ? (
                            '<div style="font-size:0.78rem;color:#0369a1;margin-top:2px;">'
                            + '→ <code style="background:#dbeafe;padding:1px 5px;border-radius:3px;font-size:0.75rem;color:#1e3a8a;">'
                            + esc(wholePathDisplay) + '</code>'
                            + '&nbsp;<code style="background:#ede9fe;padding:1px 5px;border-radius:3px;font-size:0.75rem;color:#6d28d9;">'
                            + esc(wholeHint.transform) + '</code>'
                            + '</div>'
                        ) : '',
                    '</div>',
                '</label>',
                // Individual component option
                '<label style="display:flex;align-items:flex-start;gap:0.6rem;cursor:pointer;">',
                    '<input type="radio" name="compositeMode_' + this._uid() + '" value="component"',
                        ' style="margin-top:3px;flex-shrink:0;">',
                    '<div>',
                        '<div style="font-weight:600;color:#0c4a6e;">Map individual component</div>',
                        '<div style="font-size:0.78rem;color:#475569;margin-top:2px;">',
                            'Select one sub-field from the table below',
                        '</div>',
                    '</div>',
                '</label>',
            '</div>',

            // ── Whole-object action panel ─────────────────────────────────
            '<div id="compPickerWholeSection"',
                ' style="padding:0.65rem 0.85rem;border-bottom:1px solid #bae6fd;">',
                wholeHint ? (
                    '<button type="button" id="compPickerWholeBtn"',
                        ' style="padding:0.45rem 1rem;background:#0369a1;color:white;border:none;',
                            'border-radius:5px;cursor:pointer;font-size:0.82rem;font-weight:600;">',
                        '<i class="fas fa-check"></i> Use ' + esc((wholeHint && wholeHint.label) || 'whole object') + ' mapping',
                    '</button>',
                    '<span style="margin-left:0.6rem;font-size:0.78rem;color:#475569;">',
                        'source: <code>' + esc(this._hl7Field) + '</code>',
                        '&nbsp;&rarr;&nbsp;',
                        'target: <code>' + esc(wholePathDisplay) + '</code>',
                    '</span>'
                ) : (
                    '<p style="color:#64748b;font-size:0.8rem;margin:0;">',
                        'Whole-object mapping not defined for ' + esc(this._hl7DataType) + '.',
                        ' Use individual component mapping below.',
                    '</p>'
                ),
            '</div>',

            // ── Component table ───────────────────────────────────────────
            '<div id="compPickerCompSection" style="display:none;">',
                '<div style="padding:0.35rem 0.85rem;background:#e0f2fe;font-size:0.75rem;',
                    'color:#0369a1;font-weight:600;border-bottom:1px solid #bae6fd;">',
                    'Click a row to select it',
                '</div>',
                '<div style="max-height:200px;overflow-y:auto;">',
                    '<table style="width:100%;border-collapse:collapse;">',
                        '<thead>',
                            '<tr style="background:#f0f9ff;">',
                                '<th style="padding:0.35rem 0.75rem;text-align:left;font-size:0.73rem;',
                                    'color:#64748b;font-weight:600;border-bottom:1px solid #e2e8f0;white-space:nowrap;">HL7 Path</th>',
                                '<th style="padding:0.35rem 0.75rem;text-align:left;font-size:0.73rem;',
                                    'color:#64748b;font-weight:600;border-bottom:1px solid #e2e8f0;">Component</th>',
                                '<th style="padding:0.35rem 0.75rem;text-align:left;font-size:0.73rem;',
                                    'color:#64748b;font-weight:600;border-bottom:1px solid #e2e8f0;">→ FHIR Target</th>',
                            '</tr>',
                        '</thead>',
                        '<tbody>',
                            components.map(function(comp) {
                                var compPath = self._hl7Field + '.' + comp.position;
                                var fhirPath = resourcePrefix + comp.fhirHint;
                                return [
                                    '<tr class="comp-row"',
                                        ' data-hl7="' + esc(compPath) + '"',
                                        ' data-fhir="' + esc(fhirPath) + '"',
                                        ' data-transform="' + esc(comp.transform) + '"',
                                        ' style="cursor:pointer;border-bottom:1px solid #f1f5f9;transition:background 0.1s;"',
                                        ' onmouseover="this.style.background=\'#e0f2fe\'"',
                                        ' onmouseout="this.style.background=\'\'">',
                                        '<td style="padding:0.42rem 0.75rem;white-space:nowrap;">',
                                            '<code style="background:#ede9fe;padding:1px 6px;border-radius:3px;',
                                                'font-size:0.78rem;color:#6d28d9;">' + esc(compPath) + '</code>',
                                        '</td>',
                                        '<td style="padding:0.42rem 0.75rem;font-size:0.8rem;color:#374151;">',
                                            esc(comp.name),
                                            comp.note
                                                ? ' <i class="fas fa-info-circle" style="color:#94a3b8;font-size:0.7rem;margin-left:3px;" title="' + esc(comp.note) + '"></i>'
                                                : '',
                                        '</td>',
                                        '<td style="padding:0.42rem 0.75rem;">',
                                            '<code style="background:#fce7f3;padding:1px 6px;border-radius:3px;',
                                                'font-size:0.78rem;color:#831843;">' + esc(fhirPath) + '</code>',
                                        '</td>',
                                    '</tr>',
                                ].join('');
                            }).join(''),
                        '</tbody>',
                    '</table>',
                '</div>',
            '</div>',

            // ── Dismiss link ──────────────────────────────────────────────
            '<div style="padding:0.4rem 0.85rem;border-top:1px solid #bae6fd;text-align:right;">',
                '<button type="button" id="compPickerDismissBtn"',
                    ' style="background:none;border:none;color:#64748b;font-size:0.78rem;',
                        'cursor:pointer;text-decoration:underline;padding:0;">',
                    '&#8617; Type FHIR path manually',
                '</button>',
            '</div>',

            '</div>',  // .composite-picker
        ].join('');

        this._attachEvents();
    }

    destroy() {
        if (this._destroyed) return;
        this._destroyed = true;
        this._container.innerHTML = '';
    }

    // ── Private ────────────────────────────────────────────────────────────

    _uid() {
        // Stable per-instance key for radio group names so multiple pickers
        // on the same page don't share the same group.
        if (!this.__uid) {
            this.__uid = Math.random().toString(36).slice(2, 8);
        }
        return this.__uid;
    }

    _attachEvents() {
        var self = this;

        // Mode toggle via radio buttons
        var radios = this._container.querySelectorAll('input[type="radio"]');
        radios.forEach(function(radio) {
            radio.addEventListener('change', function() {
                self._setMode(radio.value);
            });
        });

        // Whole-object confirm button
        var wholeBtn = this._container.querySelector('#compPickerWholeBtn');
        if (wholeBtn) {
            wholeBtn.addEventListener('click', function() {
                self._selectWholeObject();
            });
        }

        // Component row clicks
        var rows = this._container.querySelectorAll('tr.comp-row');
        rows.forEach(function(row) {
            row.addEventListener('click', function() {
                self._selectComponent(row);
            });
        });

        // Dismiss (manual entry) link
        var dismissBtn = this._container.querySelector('#compPickerDismissBtn');
        if (dismissBtn) {
            dismissBtn.addEventListener('click', function() {
                self._dismiss();
            });
        }
    }

    _setMode(mode) {
        this._mode = mode;
        var wholeSection = this._container.querySelector('#compPickerWholeSection');
        var compSection  = this._container.querySelector('#compPickerCompSection');
        if (wholeSection) wholeSection.style.display = (mode === 'whole')     ? '' : 'none';
        if (compSection)  compSection.style.display  = (mode === 'component') ? '' : 'none';
    }

    _selectWholeObject() {
        var wholeHint = HL7TypeCatalog.getWholeObjectHint(this._hl7DataType);
        if (!wholeHint) return;

        var resourcePrefix = this._resource ? (this._resource + '.') : '';
        var fhirPath = (wholeHint.fhirHint !== '(target path)')
            ? resourcePrefix + wholeHint.fhirHint
            : '';

        this._fire('mapping-selected', {
            hl7Field:      this._hl7Field,
            fhirPath:      fhirPath,
            transformType: wholeHint.transform,
            hl7DataType:   this._hl7DataType,
        });
    }

    _selectComponent(rowEl) {
        this._fire('mapping-selected', {
            hl7Field:      rowEl.dataset.hl7,
            fhirPath:      rowEl.dataset.fhir,
            transformType: rowEl.dataset.transform,
            hl7DataType:   this._hl7DataType,
        });
    }

    _dismiss() {
        this._fire('composite-dismissed', {});
    }

    _fire(eventName, detail) {
        this._container.dispatchEvent(new CustomEvent(eventName, {
            bubbles: true,
            detail: detail,
        }));
    }
}
