/**
 * RemoveDuplicatesBuilder — extracted from PropertiesPanel.createRemoveDuplicatesUI(),
 * _initRemoveDuplicatesChips(), and _showRdFieldCandidates().
 *
 * Registered with StepBuilderRegistry as 'remove_duplicates'.
 */
class RemoveDuplicatesBuilder {
    constructor(panel) {
        this._panel = panel;
        this._rdSourceFieldAC = null;
        this._rdOutputFieldAC = null;
    }

    // ── Public builder contract ──────────────────────────────────────────────

    render(step) {
        if (!step.config) step.config = {};
        const html = this._buildHTML(step);
        setTimeout(() => this._attachEvents(), 0);
        return html;
    }

    collectConfig(step) {
        step.config = step.config || {};

        const rdSrc = document.getElementById('rdSourceField')?.value?.trim();
        if (rdSrc !== undefined) step.config.sourceField = rdSrc;

        const rdKeyJson = document.getElementById('rdKeyFieldsJson')?.value;
        if (rdKeyJson) {
            try { step.config.keyFields = JSON.parse(rdKeyJson) || []; } catch (_) { step.config.keyFields = []; }
        }

        const rdStrategy = document.querySelector('input[name="rdStrategy"]:checked')?.value;
        if (rdStrategy) step.config.strategy = rdStrategy;

        const rdNullKey = document.querySelector('input[name="rdNullKeyBehavior"]:checked')?.value;
        if (rdNullKey) step.config.nullKeyBehavior = rdNullKey;

        const rdCaseSens = document.getElementById('rdCaseSensitive');
        if (rdCaseSens) step.config.caseSensitive = rdCaseSens.checked;

        const rdOut = document.getElementById('rdOutputField')?.value?.trim();
        if (rdOut !== undefined) step.config.outputField = rdOut;

        const rdMax = document.getElementById('rdMaxInputRecords')?.value;
        if (rdMax !== undefined && rdMax !== '') step.config.maxInputRecords = parseInt(rdMax, 10) || 0;

        console.log('[RemoveDuplicatesBuilder] ✅ Remove Duplicates config collected:', step.config);
    }

    destroy() {
        if (this._rdSourceFieldAC) { try { this._rdSourceFieldAC.destroy(); } catch (_) {} this._rdSourceFieldAC = null; }
        if (this._rdOutputFieldAC) { try { this._rdOutputFieldAC.destroy(); } catch (_) {} this._rdOutputFieldAC = null; }
    }

    // ── Private: HTML construction ───────────────────────────────────────────

    _buildHTML(step) {
        const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

        let keyFields = [];
        const kf = step.config.keyFields;
        if (Array.isArray(kf)) keyFields = kf.filter(Boolean);
        else if (typeof kf === 'string' && kf) keyFields = kf.split(',').map(f => f.trim()).filter(Boolean);

        const sourceField     = step.config.sourceField     || '';
        const strategy        = step.config.strategy        || 'first';
        const nullKeyBehavior = step.config.nullKeyBehavior  || 'group';
        const caseSensitive   = step.config.caseSensitive   !== false;
        const outputField     = step.config.outputField     || '';
        const maxInputRecords = step.config.maxInputRecords  || 0;

        const chipsHtml = keyFields.map(f =>
            `<span class="rd-field-chip">${esc(f)}<button type="button" class="rd-chip-remove" data-field="${esc(f)}" title="Remove">&#x00D7;</button></span>`
        ).join('');

        const strategyOptions = [
            { value: 'first', title: 'Keep First',   desc: 'Discard all later duplicates — fast, preserves original ordering' },
            { value: 'last',  title: 'Keep Last',    desc: 'Overwrite with the most recent — useful when later records are corrections' },
            { value: 'merge', title: 'Merge Fields', desc: 'Keep first, fill missing fields from later records (non-destructive)' }
        ];
        const nullOptions = [
            { value: 'group',  title: 'Group',    desc: 'Treat all null-key records as one group; apply strategy among them' },
            { value: 'keep',   title: 'Keep All', desc: 'Always keep records where key fields are missing (bypass dedup)' },
            { value: 'remove', title: 'Drop',     desc: 'Remove records where key fields are null or absent' }
        ];

        const radioGroup = (name, options, selected) => options.map(o =>
            `<label class="rd-radio-option${selected === o.value ? ' active' : ''}">
                <input type="radio" name="${name}" value="${o.value}"${selected === o.value ? ' checked' : ''}>
                <div class="rd-radio-content">
                    <span class="rd-radio-title">${esc(o.title)}</span>
                    <span class="rd-radio-desc">${esc(o.desc)}</span>
                </div>
            </label>`
        ).join('');

        const section = document.createElement('div');
        section.className = 'form-section';
        section.innerHTML = `
            <div class="form-group">
                <label class="form-label">Source Array Field <span style="color:#ef4444">*</span></label>
                <input type="text" id="rdSourceField" class="form-control" value="${esc(sourceField)}"
                       placeholder="e.g. enriched.file_parser.records" autocomplete="off">
                <div class="field-help">Dot-path to the array to deduplicate (e.g. <code>enriched.results</code>)</div>
            </div>

            <div class="form-group">
                <label class="form-label">
                    Key Fields
                    <span style="font-weight:400;color:var(--text-muted,#6b7280);font-size:11px;margin-left:4px">(empty = deduplicate on entire record)</span>
                </label>
                <div id="rdKeyChips" class="rd-chips-container">${chipsHtml || '<span style="color:var(--text-muted,#9ca3af);font-size:12px;align-self:center">No key fields — full-record dedup</span>'}</div>
                <input type="hidden" id="rdKeyFieldsJson" value='${JSON.stringify(keyFields)}'>
                <div class="rd-chip-input-row">
                    <input type="text" id="rdKeyFieldInput" class="form-control form-control-sm"
                           placeholder="Type a field name and press Enter or Add" style="flex:1">
                    <button type="button" id="rdAddFieldBtn" class="btn btn-sm btn-outline-primary">Add</button>
                    <button type="button" id="rdDetectBtn" class="btn btn-sm btn-outline-secondary"
                            title="Auto-detect column names from upstream File Parser step">Detect from source</button>
                </div>
                <div class="field-help">Fields forming the unique key. Supports dot-paths (e.g. <code>patient.id</code>).</div>
            </div>

            <div class="form-group">
                <label class="form-label">Duplicate Strategy</label>
                <div class="rd-radio-group">${radioGroup('rdStrategy', strategyOptions, strategy)}</div>
            </div>

            <div class="form-group">
                <label class="form-label">Missing Key Behavior</label>
                <div class="rd-radio-group rd-radio-group-sm">${radioGroup('rdNullKeyBehavior', nullOptions, nullKeyBehavior)}</div>
            </div>

            <div style="display:flex;gap:12px;align-items:flex-start;flex-wrap:wrap">
                <div class="form-group" style="flex:1;min-width:160px">
                    <label class="form-label">Output Field <span style="font-weight:400;color:var(--text-muted,#6b7280)">(optional)</span></label>
                    <input type="text" id="rdOutputField" class="form-control" value="${esc(outputField)}"
                           placeholder="(update source field in-place)" autocomplete="off">
                    <div class="field-help">Write result to a different field, keeping the original array intact</div>
                </div>
                <div class="form-group" style="flex-shrink:0">
                    <label class="form-label" style="display:flex;align-items:center;gap:8px;cursor:pointer">
                        <input type="checkbox" id="rdCaseSensitive"${caseSensitive ? ' checked' : ''}>
                        Case-Sensitive Keys
                    </label>
                    <div class="field-help">Uncheck to treat "Smith" = "SMITH"</div>
                </div>
            </div>

            <details class="form-section-advanced">
                <summary>Advanced options</summary>
                <div class="form-group">
                    <label class="form-label">Max Input Records</label>
                    <input type="number" id="rdMaxInputRecords" class="form-control" value="${maxInputRecords}"
                           min="0" max="10000000" style="width:180px">
                    <div class="field-help">0 = default 1M limit; hard cap 10M.</div>
                </div>
            </details>
        `;

        return section.outerHTML;
    }

    // ── Private: event wiring ────────────────────────────────────────────────

    _attachEvents() {
        this._initChips();

        // IntelliSense on sourceField
        const rdSrcEl = document.getElementById('rdSourceField');
        if (rdSrcEl && typeof FieldPathSearchComponent !== 'undefined') {
            if (typeof this._panel.loadStepVariables === 'function') this._panel.loadStepVariables();
            if (this._rdSourceFieldAC) { try { this._rdSourceFieldAC.destroy(); } catch (_) {} }
            this._rdSourceFieldAC = new FieldPathSearchComponent(rdSrcEl, {
                includeHL7Fields: false, allowCustom: true, showCategories: true,
                placeholder: 'e.g. enriched.file_parser.records',
                getStepVariables: () => this._panel.getStepVariablesForSearch ? this._panel.getStepVariablesForSearch() : [],
                onSelect: path => { rdSrcEl.value = path; rdSrcEl.dispatchEvent(new Event('input', { bubbles: true })); }
            });
        }

        // IntelliSense on outputField
        const rdOutEl = document.getElementById('rdOutputField');
        if (rdOutEl && typeof FieldPathSearchComponent !== 'undefined') {
            if (this._rdOutputFieldAC) { try { this._rdOutputFieldAC.destroy(); } catch (_) {} }
            this._rdOutputFieldAC = new FieldPathSearchComponent(rdOutEl, {
                includeHL7Fields: false, allowCustom: true, showCategories: true,
                placeholder: 'e.g. enriched.deduped_results',
                getStepVariables: () => this._panel.getStepVariablesForSearch ? this._panel.getStepVariablesForSearch() : [],
                onSelect: path => { rdOutEl.value = path; }
            });
        }
    }

    _initChips() {
        const chipsContainer = document.getElementById('rdKeyChips');
        const keyFieldInput  = document.getElementById('rdKeyFieldInput');
        const keyFieldsJson  = document.getElementById('rdKeyFieldsJson');
        const addBtn         = document.getElementById('rdAddFieldBtn');
        const detectBtn      = document.getElementById('rdDetectBtn');
        if (!chipsContainer || !keyFieldInput || !keyFieldsJson) return;

        const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        const getFields = () => { try { return JSON.parse(keyFieldsJson.value) || []; } catch { return []; } };
        const setFields = fields => { keyFieldsJson.value = JSON.stringify(fields); };
        const renderChips = fields => {
            chipsContainer.innerHTML = fields.length
                ? fields.map(f => `<span class="rd-field-chip">${esc(f)}<button type="button" class="rd-chip-remove" data-field="${esc(f)}" title="Remove">&#x00D7;</button></span>`).join('')
                : '<span style="color:var(--text-muted,#9ca3af);font-size:12px;align-self:center">No key fields — full-record dedup</span>';
            chipsContainer.querySelectorAll('.rd-chip-remove').forEach(btn => {
                btn.addEventListener('click', () => { setFields(getFields().filter(f => f !== btn.dataset.field)); renderChips(getFields()); });
            });
        };

        const addField = () => {
            const val = keyFieldInput.value.trim();
            if (!val) return;
            const fields = getFields();
            if (!fields.includes(val)) { fields.push(val); setFields(fields); renderChips(fields); }
            keyFieldInput.value = '';
            keyFieldInput.focus();
        };

        renderChips(getFields());
        if (addBtn) addBtn.addEventListener('click', addField);
        keyFieldInput.addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); addField(); } });

        const syncRadioActive = name => {
            document.querySelectorAll(`input[name="${name}"]`).forEach(radio => {
                radio.addEventListener('change', () => {
                    document.querySelectorAll('.rd-radio-option').forEach(opt => {
                        const r = opt.querySelector(`input[name="${name}"]`);
                        if (r) opt.classList.toggle('active', r.checked);
                    });
                });
            });
        };
        syncRadioActive('rdStrategy');
        syncRadioActive('rdNullKeyBehavior');

        if (detectBtn) {
            detectBtn.addEventListener('click', () => {
                const pipeline = window.pipelineBuilder?.pipeline || window.pipelineBuilder?.getPipeline?.();
                const allSteps = pipeline ? (pipeline.getAllSteps?.() || Object.values(pipeline.steps || {})) : [];
                let candidates = [];
                for (const s of allSteps) {
                    if ((s.stepType || s.step_type) !== 'file_parser') continue;
                    const cols = s.config?.columns;
                    if (Array.isArray(cols) && cols.length > 0) {
                        candidates = cols.map(c => (typeof c === 'string' ? c : (c.name || c.key || ''))).filter(Boolean);
                        break;
                    }
                }
                if (candidates.length === 0) {
                    const manual = prompt('No column definitions found in an upstream File Parser step.\nEnter field names to add (comma-separated):');
                    if (manual) candidates = manual.split(',').map(f => f.trim()).filter(Boolean);
                }
                if (candidates.length > 0) this._showCandidates(candidates, detectBtn, getFields, setFields, renderChips);
            });
        }
    }

    _showCandidates(candidates, anchor, getFields, setFields, renderChips) {
        document.getElementById('rdFieldCandidatesPopover')?.remove();
        const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        const popover = document.createElement('div');
        popover.id = 'rdFieldCandidatesPopover';
        popover.className = 'rd-candidates-popover';
        popover.innerHTML = `
            <div class="rd-candidates-header">
                <span>Click to add field</span>
                <button type="button" id="rdCloseCandidates" style="background:none;border:none;cursor:pointer;font-size:16px;line-height:1;color:var(--text-muted,#6b7280)">&times;</button>
            </div>
            <div class="rd-candidates-list">
                ${candidates.map(c => `<button type="button" class="rd-candidate-item" data-field="${esc(c)}">${esc(c)}</button>`).join('')}
            </div>
        `;
        anchor.parentElement.appendChild(popover);
        popover.querySelector('#rdCloseCandidates').addEventListener('click', () => popover.remove());
        popover.querySelectorAll('.rd-candidate-item').forEach(btn => {
            btn.addEventListener('click', () => {
                const field = btn.dataset.field;
                const fields = getFields();
                if (!fields.includes(field)) { fields.push(field); setFields(fields); renderChips(fields); }
                btn.classList.add('rd-candidate-added');
                btn.disabled = true;
            });
        });
        setTimeout(() => {
            const closeOnOutside = e => {
                if (!popover.contains(e.target) && e.target !== anchor) { popover.remove(); document.removeEventListener('click', closeOnOutside); }
            };
            document.addEventListener('click', closeOnOutside);
        }, 50);
    }
}

StepBuilderRegistry.register('remove_duplicates', RemoveDuplicatesBuilder);
