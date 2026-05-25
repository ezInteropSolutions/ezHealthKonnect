/**
 * @fileoverview DeidentifyBuilder — no-code UI for the HIPAA Safe Harbor De-identify step.
 * Implements 45 CFR §164.514(b) — the 18 Safe Harbor identifiers are automatically
 * de-identified. Users can optionally exclude fields, add extra fields, or enable
 * pseudonymization for research linkage.
 * @version 1.0.0
 */

class DeidentifyBuilder {

    constructor(panel) {
        /** @private */
        this._panel = panel;
        /** @private @type {FieldPathSearchComponent[]} */
        this._acs = [];
        /** @private — chip arrays */
        this._chips = {};
    }

    // ─── Public Builder Contract ─────────────────────────────────────────────

    render(step) {
        if (!step.config) step.config = {};
        const html = this._buildHTML(step);
        setTimeout(() => this._attachEvents(step), 0);
        return html;
    }

    collectConfig(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;
        cfg.format        = document.getElementById('deiFormat')?.value          || 'auto';
        cfg.auditHash     = document.getElementById('deiAuditHash')?.checked     ?? true;
        cfg.pseudonymize  = document.getElementById('deiPseudonymize')?.checked  || false;
        cfg.hashSalt      = document.getElementById('deiHashSalt')?.value?.trim() || '';
        cfg.excludeFields      = this._readChips('deiExclude');
        cfg.includeExtraFields = this._readChips('deiExtra');
    }

    destroy() {
        for (const ac of this._acs) {
            try { ac.destroy(); } catch (_) {}
        }
        this._acs = [];
        this._chips = {};
        // Remove body-appended dropdowns
        document.querySelectorAll('.dei-chip-drop').forEach(el => el.remove());
    }

    // ─── HTML ─────────────────────────────────────────────────────────────────

    _buildHTML(step) {
        const cfg = step.config || {};
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;');
        const fmt = cfg.format || 'auto';

        return `
<div class="dei-builder">
<style>
.dei-builder { font-size:13px; color:#1e3a5f; }
.dei-section { border:1px solid rgba(30,58,138,0.15); border-radius:8px; margin-bottom:8px; overflow:hidden; }
.dei-shdr {
    display:flex; align-items:center; gap:6px; padding:9px 12px;
    background:rgba(30,58,138,0.05); cursor:pointer; user-select:none;
    font-weight:600; font-size:12px;
}
.dei-shdr:hover { background:rgba(30,58,138,0.10); }
.dei-caret { margin-left:auto; font-size:10px; transition:transform .2s; }
.dei-shdr.collapsed .dei-caret { transform:rotate(-90deg); }
.dei-shdr.collapsed + .dei-sbody { display:none; }
.dei-sbody { padding:12px; }
.dei-label { font-size:11px; font-weight:600; color:#374151; display:block; margin-bottom:3px; }
.dei-hint  { font-size:10px; color:#94a3b8; margin-top:2px; }
.dei-inp, .dei-sel {
    padding:6px 8px; border:1px solid rgba(30,58,138,0.25); border-radius:6px;
    font-size:12px; background:#fff; color:#1e3a5f; width:100%; box-sizing:border-box;
}
.dei-inp:focus, .dei-sel:focus { outline:none; border-color:#3b82f6; }
.dei-row { display:flex; gap:8px; align-items:flex-start; margin-bottom:10px; }
.dei-col { display:flex; flex-direction:column; gap:3px; }
.dei-col.grow { flex:1; min-width:0; }
.dei-irow { display:flex; gap:12px; flex-wrap:wrap; margin-bottom:10px; }
.dei-irow .dei-col { flex:1; min-width:120px; }

/* Toggle switch */
.dei-toggle-row { display:flex; align-items:center; gap:10px; margin-bottom:10px; }
.dei-toggle { position:relative; display:inline-block; width:36px; height:20px; flex-shrink:0; }
.dei-toggle input { opacity:0; width:0; height:0; }
.dei-slider {
    position:absolute; cursor:pointer; top:0; left:0; right:0; bottom:0;
    background:#d1d5db; border-radius:20px; transition:.2s;
}
.dei-toggle input:checked + .dei-slider { background:#3b82f6; }
.dei-slider:before {
    content:''; position:absolute; height:14px; width:14px; left:3px; bottom:3px;
    background:white; border-radius:50%; transition:.2s;
}
.dei-toggle input:checked + .dei-slider:before { transform:translateX(16px); }

/* Chip inputs */
.dei-chip-wrap { border:1px solid rgba(30,58,138,0.25); border-radius:6px; padding:4px 6px; background:#fff; min-height:34px; cursor:text; }
.dei-chips { display:flex; flex-wrap:wrap; gap:4px; align-items:center; }
.dei-chip {
    display:inline-flex; align-items:center; gap:4px; padding:2px 8px;
    background:#dbeafe; color:#1d4ed8; border-radius:12px; font-size:11px;
}
.dei-chip-rm { background:none; border:none; color:#1d4ed8; cursor:pointer; font-size:12px; padding:0; line-height:1; }
.dei-chip-input {
    border:none; outline:none; font-size:12px; color:#1e3a5f; min-width:140px;
    padding:2px 4px; background:transparent;
}

/* PHI coverage grid */
.dei-phi-grid {
    display:grid; grid-template-columns:repeat(auto-fill,minmax(240px,1fr)); gap:4px; margin-top:6px;
}
.dei-phi-item {
    display:flex; align-items:center; gap:6px; padding:4px 8px;
    background:rgba(30,58,138,0.04); border-radius:5px; font-size:11px;
}
.dei-phi-dot { width:6px; height:6px; border-radius:50%; background:#ef4444; flex-shrink:0; }
</style>

<!-- ① Overview -->
<div style="background:linear-gradient(135deg,#fff7ed,#fef3c7); border:1px solid #fcd34d; border-radius:8px; padding:12px; margin-bottom:10px; font-size:12px; color:#92400e;">
  <strong>🛡️ HIPAA Safe Harbor De-identification</strong> — 45 CFR §164.514(b)<br>
  All 18 Safe Harbor identifiers are automatically replaced with placeholders.
  The original values are <em>never stored</em> — only an audit hash is emitted.
</div>

<!-- ② Format -->
<div class="dei-section">
  <div class="dei-shdr" data-dei-sec="deiSFormat">
    ⚙️ Settings <span class="dei-caret">▾</span>
  </div>
  <div class="dei-sbody" id="deiSFormat">
    <div class="dei-irow">
      <div class="dei-col">
        <span class="dei-label">Message Format</span>
        <select id="deiFormat" class="dei-sel" style="max-width:180px">
          ${[['auto','Auto-detect'],['hl7v2','HL7 v2'],['fhir','FHIR R4'],['json','Generic JSON']].map(
            ([v,l]) => `<option value="${v}"${fmt===v?' selected':''}>${l}</option>`).join('')}
        </select>
        <span class="dei-hint">Auto-detect inspects enhancedSegments / resourceType</span>
      </div>
    </div>

    <div class="dei-toggle-row">
      <label class="dei-toggle">
        <input type="checkbox" id="deiAuditHash"${(cfg.auditHash !== false) ?' checked':''}>
        <span class="dei-slider"></span>
      </label>
      <span class="dei-label" style="margin:0">Emit Audit Hash</span>
    </div>
    <p class="dei-hint" style="margin:-6px 0 10px 46px">
      SHA-256 of original PHI field values — safe to store in <code>pipeline_step_audit</code> for HIPAA 164.312 compliance
    </p>

    <div class="dei-toggle-row">
      <label class="dei-toggle">
        <input type="checkbox" id="deiPseudonymize"${cfg.pseudonymize?' checked':''}>
        <span class="dei-slider"></span>
      </label>
      <span class="dei-label" style="margin:0">Pseudonymize (deterministic)</span>
    </div>
    <p class="dei-hint" style="margin:-6px 0 10px 46px">
      Replace PHI with hash-derived pseudonyms (e.g. [MRN-a3f9b2]) for research linkage.
      Without this, fields are replaced with fixed placeholders ([MRN], [EMAIL], etc.)
    </p>

    <div id="deiHashSaltRow" style="${cfg.auditHash !== false || cfg.pseudonymize ? '' : 'display:none'}">
      <div class="dei-col" style="max-width:320px">
        <span class="dei-label">Hash Salt (optional)</span>
        <input id="deiHashSalt" class="dei-inp" type="password" value="${esc(cfg.hashSalt)}"
               placeholder="secret pepper applied to hash">
        <span class="dei-hint">Stored in step config (encrypted at rest). Adds resistance to rainbow-table attacks.</span>
      </div>
    </div>
  </div>
</div>

<!-- ③ What's covered -->
<div class="dei-section">
  <div class="dei-shdr collapsed" data-dei-sec="deiSCoverage">
    📋 18 Safe Harbor Identifiers Covered <span class="dei-caret">▾</span>
  </div>
  <div class="dei-sbody" id="deiSCoverage">
    <div class="dei-phi-grid">
      ${DeidentifyBuilder.SAFE_HARBOR_IDS.map(({ id, name }) =>
        `<div class="dei-phi-item"><span class="dei-phi-dot"></span><span>#${id} ${name}</span></div>`
      ).join('')}
    </div>
    <p class="dei-hint" style="margin-top:8px">
      Identifiers #12 (vehicle), #14 (URLs), #15 (IP), #16 (photos), #17 (biometric)
      are logged but have no standard HL7/FHIR field to clear automatically.
      Use "Include Extra Fields" below if your pipeline uses custom paths for these.
    </p>
  </div>
</div>

<!-- ④ Exclude Fields -->
<div class="dei-section">
  <div class="dei-shdr${!(cfg.excludeFields||[]).length?' collapsed':''}" data-dei-sec="deiSExclude">
    🚫 Exclude Fields <span class="dei-caret">▾</span>
  </div>
  <div class="dei-sbody" id="deiSExclude">
    <p class="dei-hint" style="margin:0 0 8px">
      Field paths to skip (e.g. keep 3-digit zip prefix per Safe Harbor geographic exception).
      Use FieldPath search to pick from pipeline variables.
    </p>
    <div class="dei-chip-wrap" id="deiExcludeWrap">
      <div class="dei-chips" id="deiExcludeChips">
        ${(cfg.excludeFields || []).map(f => this._chipHTML(f, 'deiExclude')).join('')}
        <input class="dei-chip-input" id="deiExclude" placeholder="Field path + Enter to add…">
      </div>
    </div>
  </div>
</div>

<!-- ⑤ Include Extra Fields -->
<div class="dei-section">
  <div class="dei-shdr${!(cfg.includeExtraFields||[]).length?' collapsed':''}" data-dei-sec="deiSExtra">
    ➕ Include Extra Fields <span class="dei-caret">▾</span>
  </div>
  <div class="dei-sbody" id="deiSExtra">
    <p class="dei-hint" style="margin:0 0 8px">
      Additional field paths to de-identify beyond the built-in 18 identifiers.
      Useful for custom fields or formats not covered by the built-in Safe Harbor list.
    </p>
    <div class="dei-chip-wrap" id="deiExtraWrap">
      <div class="dei-chips" id="deiExtraChips">
        ${(cfg.includeExtraFields || []).map(f => this._chipHTML(f, 'deiExtra')).join('')}
        <input class="dei-chip-input" id="deiExtra" placeholder="Field path + Enter to add…">
      </div>
    </div>
  </div>
</div>
</div>`;
    }

    // ─── Chip Helpers ─────────────────────────────────────────────────────────

    _chipHTML(value, chipId) {
        const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;');
        return `<span class="dei-chip" data-chip-id="${esc(chipId)}" data-value="${esc(value)}">
            ${esc(value)}
            <button class="dei-chip-rm" type="button" title="Remove">×</button>
        </span>`;
    }

    _readChips(inputId) {
        const container = document.getElementById(inputId + 'Chips') ||
                          document.getElementById(inputId)?.closest('.dei-chips');
        if (!container) return [];
        return Array.from(container.querySelectorAll('.dei-chip[data-value]'))
            .map(el => el.dataset.value)
            .filter(Boolean);
    }

    _addChip(inputId, value) {
        if (!value || !value.trim()) return;
        const val = value.trim();
        const container = document.getElementById(inputId + 'Chips');
        const input = document.getElementById(inputId);
        if (!container || !input) return;
        // Check duplicate
        const existing = Array.from(container.querySelectorAll('.dei-chip[data-value]'))
            .map(el => el.dataset.value);
        if (existing.includes(val)) return;
        // Insert chip before input
        const div = document.createElement('span');
        div.innerHTML = this._chipHTML(val, inputId);
        const chip = div.firstElementChild;
        chip.querySelector('.dei-chip-rm').addEventListener('click', () => chip.remove());
        container.insertBefore(chip, input);
        input.value = '';
    }

    // ─── Events ──────────────────────────────────────────────────────────────

    _attachEvents(step) {
        // Section collapse toggles
        document.querySelectorAll('.dei-shdr[data-dei-sec]').forEach(hdr => {
            hdr.addEventListener('click', () => hdr.classList.toggle('collapsed'));
        });

        // Wire existing chip remove buttons
        document.querySelectorAll('.dei-chip .dei-chip-rm').forEach(btn => {
            btn.addEventListener('click', () => btn.closest('.dei-chip').remove());
        });

        // Show/hide hash salt row based on auditHash or pseudonymize
        const auditCb = document.getElementById('deiAuditHash');
        const pseudoCb = document.getElementById('deiPseudonymize');
        const saltRow  = document.getElementById('deiHashSaltRow');
        const updateSaltRow = () => {
            if (saltRow) saltRow.style.display =
                (auditCb?.checked || pseudoCb?.checked) ? '' : 'none';
        };
        auditCb?.addEventListener('change', updateSaltRow);
        pseudoCb?.addEventListener('change', updateSaltRow);

        // Chip input — Enter or comma adds chip
        ['deiExclude', 'deiExtra'].forEach(chipId => {
            const input = document.getElementById(chipId);
            if (!input) return;

            input.addEventListener('keydown', e => {
                if (e.key === 'Enter' || e.key === ',') {
                    e.preventDefault();
                    this._addChip(chipId, input.value);
                } else if (e.key === 'Backspace' && !input.value) {
                    // Remove last chip
                    const chips = input.closest('.dei-chips')?.querySelectorAll('.dei-chip');
                    if (chips && chips.length) chips[chips.length - 1].remove();
                }
            });

            input.addEventListener('blur', () => {
                if (input.value.trim()) this._addChip(chipId, input.value);
            });

            // Attach FieldPathSearch for autocomplete
            this._attachFieldAC(input, chipId);

            // Click on wrap focuses input
            input.closest('.dei-chip-wrap')?.addEventListener('click', e => {
                if (e.target === e.currentTarget || e.target.classList.contains('dei-chips')) {
                    input.focus();
                }
            });
        });
    }

    _attachFieldAC(inputEl, chipId) {
        if (!inputEl || !window.FieldPathSearchComponent) return;
        try {
            const ac = new FieldPathSearchComponent(inputEl, {
                builder: this._panel?.builder,
                onSelect: path => {
                    this._addChip(chipId, path);
                    inputEl.value = '';
                },
                placeholder: 'Search pipeline vars...',
                maxSuggestions: 20,
            });
            this._acs.push(ac);
        } catch (_) {}
    }
}

DeidentifyBuilder.SAFE_HARBOR_IDS = [
    { id: 1,  name: 'Names' },
    { id: 2,  name: 'Geographic subdivisions (< state)' },
    { id: 3,  name: 'Dates (except year)' },
    { id: 4,  name: 'Phone numbers' },
    { id: 5,  name: 'Fax numbers' },
    { id: 6,  name: 'Email addresses' },
    { id: 7,  name: 'Social Security Numbers' },
    { id: 8,  name: 'Medical Record Numbers' },
    { id: 9,  name: 'Health plan beneficiary numbers' },
    { id: 10, name: 'Account numbers' },
    { id: 11, name: 'Certificate / license numbers' },
    { id: 12, name: 'Vehicle identifiers & serial numbers' },
    { id: 13, name: 'Device identifiers & serial numbers' },
    { id: 14, name: 'Web URLs' },
    { id: 15, name: 'IP addresses' },
    { id: 16, name: 'Biometric identifiers' },
    { id: 17, name: 'Full-face photos' },
    { id: 18, name: 'Any other unique identifying number or characteristic' },
];

StepBuilderRegistry.register('deidentify', DeidentifyBuilder);
StepBuilderRegistry.register('pre.deidentify', DeidentifyBuilder);
StepBuilderRegistry.register('post.deidentify', DeidentifyBuilder);
