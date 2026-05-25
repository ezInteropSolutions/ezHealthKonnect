/**
 * PASEnvelopeBuilder — guided UI for the Da Vinci PAS envelope mapping step.
 *
 * Renders a structured 2-column form grouped by PAS category (Patient, Coverage,
 * Provider, Service Request). Each row shows the PAS field label, a required badge,
 * and an editable source-path input the user fills in from their inbound message.
 *
 * Step type: pas_envelope_mapping  (executor alias → field_mapping)
 * Registered: StepBuilderRegistry.register('pas_envelope_mapping', PASEnvelopeBuilder)
 */
class PASEnvelopeBuilder {
    constructor(panel) {
        this._panel = panel;
    }

    // ── Builder contract ─────────────────────────────────────────────────────

    render(step) {
        if (!step.config) step.config = {};
        // Ensure mappings array exists (pre-populated by template, editable by user)
        if (!step.config.mappings) step.config.mappings = PASEnvelopeBuilder._defaultMappings();
        const html = this._buildHTML(step);
        setTimeout(() => this._attachEvents(step), 0);
        return html;
    }

    collectConfig(step) {
        const esc = s => String(s || '');
        step.config = step.config || {};

        const form = document.querySelector('.properties-form') || document;
        const rows = form.querySelectorAll('.pas-env-row');
        const mappings = [];

        rows.forEach(row => {
            const lhsEl = row.querySelector('.pas-lhs');
            const rhsEl = row.querySelector('.pas-rhs');
            if (!lhsEl || !rhsEl) return;

            const meta = JSON.parse(row.dataset.meta || '{}');
            mappings.push({
                lhs: lhsEl.value,
                rhs: rhsEl.value.trim(),
                _label:    meta.label    || '',
                _required: meta.required || false
            });
        });

        step.config.mappings = mappings;
        step.config._zone = 'user_mapping';
    }

    destroy() {
        // No AC refs to teardown for this builder
    }

    // ── HTML rendering ───────────────────────────────────────────────────────

    _buildHTML(step) {
        const esc = s => {
            const d = document.createElement('div');
            d.textContent = String(s || '');
            return d.innerHTML;
        };

        const mappings = step.config.mappings || PASEnvelopeBuilder._defaultMappings();
        const byGroup = PASEnvelopeBuilder._groupMappings(mappings);

        let rows = '';
        Object.entries(byGroup).forEach(([group, fields]) => {
            rows += `
            <tr class="pas-group-header">
                <td colspan="2" style="background:#1e293b;color:#94a3b8;font-size:11px;
                    font-weight:600;letter-spacing:.08em;text-transform:uppercase;
                    padding:8px 12px;border-top:2px solid #334155;">
                    ${esc(group)}
                </td>
            </tr>`;

            fields.forEach(m => {
                const required  = m._required ? true : false;
                const badge     = required
                    ? '<span style="background:#dc2626;color:#fff;font-size:9px;border-radius:3px;padding:1px 5px;margin-left:5px;vertical-align:middle;">required</span>'
                    : '<span style="background:#334155;color:#94a3b8;font-size:9px;border-radius:3px;padding:1px 5px;margin-left:5px;vertical-align:middle;">optional</span>';
                const rhsVal    = esc(m.rhs || '');
                const lhsVal    = esc(m.lhs || '');
                const meta      = esc(JSON.stringify({ label: m._label, required }));

                rows += `
                <tr class="pas-env-row" data-meta='${meta}'>
                    <td style="padding:7px 12px;border-bottom:1px solid #1e293b;white-space:nowrap;width:44%;">
                        <div style="color:#e2e8f0;font-size:12px;font-weight:500;">
                            ${esc(m._label || m.lhs)}${badge}
                        </div>
                        <div style="color:#64748b;font-size:10px;margin-top:2px;font-family:monospace;">
                            ${lhsVal}
                        </div>
                        <input type="hidden" class="pas-lhs" value="${lhsVal}">
                    </td>
                    <td style="padding:6px 12px;border-bottom:1px solid #1e293b;">
                        <input type="text"
                               class="pas-rhs"
                               value="${rhsVal}"
                               placeholder="e.g. PID.5.1  or  body.patient.firstName"
                               style="width:100%;background:#0f172a;border:1px solid #334155;
                                      border-radius:4px;color:#e2e8f0;padding:5px 8px;font-size:12px;
                                      font-family:monospace;"
                               data-lhs="${lhsVal}">
                    </td>
                </tr>`;
            });
        });

        return `
        <div class="pas-envelope-builder" style="font-family:'Inter',sans-serif;">
            <div style="background:#0f2d4a;border:1px solid #1e4976;border-radius:6px;
                        padding:12px 16px;margin-bottom:14px;">
                <div style="color:#38bdf8;font-weight:600;font-size:13px;margin-bottom:4px;">
                    🗺️ PAS Envelope Mapping
                    <span style="color:#64748b;font-weight:400;font-size:11px;margin-left:8px;">
                        Da Vinci PAS STU2 • CMS-0057-F
                    </span>
                </div>
                <div style="color:#94a3b8;font-size:12px;line-height:1.5;">
                    Map fields from your inbound message to the PAS envelope.
                    Use <code style="background:#1e293b;padding:1px 5px;border-radius:3px;color:#7dd3fc;">
                    PID.5.1</code> for HL7, dot-paths for JSON, or literal values.
                    The PAS Core steps below are pre-built and locked.
                </div>
            </div>

            <table style="width:100%;border-collapse:collapse;background:#0f172a;
                          border:1px solid #1e293b;border-radius:6px;overflow:hidden;">
                <thead>
                    <tr style="background:#1e293b;">
                        <th style="padding:8px 12px;text-align:left;color:#64748b;
                                   font-size:11px;font-weight:600;letter-spacing:.05em;width:44%;">
                            PAS FIELD
                        </th>
                        <th style="padding:8px 12px;text-align:left;color:#64748b;
                                   font-size:11px;font-weight:600;letter-spacing:.05em;">
                            SOURCE PATH  <span style="font-weight:400;">(from your inbound message)</span>
                        </th>
                    </tr>
                </thead>
                <tbody>${rows}</tbody>
            </table>

            <div id="pas-validation-banner" style="display:none;margin-top:10px;padding:8px 12px;
                 background:#450a0a;border:1px solid #dc2626;border-radius:5px;
                 color:#fca5a5;font-size:12px;"></div>

            <div style="margin-top:10px;display:flex;gap:8px;">
                <button id="pas-validate-btn" type="button"
                    style="background:#1e293b;border:1px solid #334155;color:#94a3b8;
                           border-radius:5px;padding:6px 14px;font-size:12px;cursor:pointer;">
                    ✓ Check Required Fields
                </button>
                <div style="color:#475569;font-size:11px;align-self:center;">
                    Required fields must be mapped before saving.
                </div>
            </div>
        </div>`;
    }

    // ── Events ───────────────────────────────────────────────────────────────

    _attachEvents(step) {
        const btn = document.getElementById('pas-validate-btn');
        if (!btn) return;

        btn.addEventListener('click', () => {
            const missing = [];
            document.querySelectorAll('.pas-env-row').forEach(row => {
                const meta    = JSON.parse(row.dataset.meta || '{}');
                const rhsEl   = row.querySelector('.pas-rhs');
                const lhsEl   = row.querySelector('.pas-lhs');
                if (meta.required && rhsEl && !rhsEl.value.trim()) {
                    missing.push(meta.label || lhsEl.value);
                }
            });

            const banner = document.getElementById('pas-validation-banner');
            if (!banner) return;

            if (missing.length === 0) {
                banner.style.display = 'block';
                banner.style.background = '#052e16';
                banner.style.borderColor = '#16a34a';
                banner.style.color = '#86efac';
                banner.textContent = `✓ All required fields are mapped (${document.querySelectorAll('.pas-rhs').length} total mappings).`;
            } else {
                banner.style.display = 'block';
                banner.style.background = '#450a0a';
                banner.style.borderColor = '#dc2626';
                banner.style.color = '#fca5a5';
                banner.textContent = `⚠ Missing required mappings: ${missing.join(', ')}`;
            }
        });

        // Highlight empty required fields on blur
        document.querySelectorAll('.pas-env-row').forEach(row => {
            const meta  = JSON.parse(row.dataset.meta || '{}');
            const rhsEl = row.querySelector('.pas-rhs');
            if (!rhsEl || !meta.required) return;

            rhsEl.addEventListener('blur', () => {
                rhsEl.style.borderColor = !rhsEl.value.trim() ? '#dc2626' : '#334155';
            });
            rhsEl.addEventListener('focus', () => {
                rhsEl.style.borderColor = '#38bdf8';
            });
        });
    }

    // ── Static helpers ───────────────────────────────────────────────────────

    static _groupMappings(mappings) {
        const groups = {
            'Patient': [],
            'Coverage / Payer': [],
            'Provider': [],
            'Service Request': []
        };

        mappings.forEach(m => {
            const lhs = m.lhs || '';
            if (lhs.startsWith('_pas_envelope.patient.'))   groups['Patient'].push(m);
            else if (lhs.startsWith('_pas_envelope.coverage.')) groups['Coverage / Payer'].push(m);
            else if (lhs.startsWith('_pas_envelope.provider.')) groups['Provider'].push(m);
            else                                               groups['Service Request'].push(m);
        });

        // Drop empty groups
        return Object.fromEntries(Object.entries(groups).filter(([, v]) => v.length > 0));
    }

    static _defaultMappings() {
        return [
            { lhs: '_pas_envelope.patient.firstName',       rhs: '', _label: 'Patient First Name',             _required: true  },
            { lhs: '_pas_envelope.patient.lastName',        rhs: '', _label: 'Patient Last Name',              _required: true  },
            { lhs: '_pas_envelope.patient.dob',             rhs: '', _label: 'Date of Birth (YYYY-MM-DD)',     _required: true  },
            { lhs: '_pas_envelope.patient.gender',          rhs: '', _label: 'Gender (male/female/other)',     _required: false },
            { lhs: '_pas_envelope.patient.memberId',        rhs: '', _label: 'Payer Member ID',               _required: true  },
            { lhs: '_pas_envelope.coverage.payerId',        rhs: '', _label: 'Payer ID (NPI or OID)',         _required: true  },
            { lhs: '_pas_envelope.coverage.planId',         rhs: '', _label: 'Plan ID',                       _required: false },
            { lhs: '_pas_envelope.coverage.groupNumber',    rhs: '', _label: 'Group Number',                  _required: false },
            { lhs: '_pas_envelope.provider.npi',            rhs: '', _label: 'Requesting Provider NPI',       _required: true  },
            { lhs: '_pas_envelope.provider.name',           rhs: '', _label: 'Provider Full Name',            _required: false },
            { lhs: '_pas_envelope.provider.facilityNpi',    rhs: '', _label: 'Facility NPI',                  _required: false },
            { lhs: '_pas_envelope.request.serviceCode',     rhs: '', _label: 'CPT / HCPCS Service Code',      _required: true  },
            { lhs: '_pas_envelope.request.diagnosisCodes',  rhs: '', _label: 'ICD-10 Diagnosis Codes (array)',_required: true  },
            { lhs: '_pas_envelope.request.urgency',         rhs: '', _label: 'Urgency (routine / urgent)',    _required: false },
            { lhs: '_pas_envelope.request.serviceStartDate',rhs: '', _label: 'Service Start Date',            _required: false },
            { lhs: '_pas_envelope.request.serviceEndDate',  rhs: '', _label: 'Service End Date',              _required: false },
            { lhs: '_pas_envelope.request.quantity',        rhs: '', _label: 'Service Quantity',              _required: false }
        ];
    }
}

StepBuilderRegistry.register('pas_envelope_mapping', PASEnvelopeBuilder);
