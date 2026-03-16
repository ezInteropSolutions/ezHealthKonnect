/**
 * RemoveDuplicatesBuilder — extracted from PropertiesPanel.createRemoveDuplicatesUI(),
 * _initRemoveDuplicatesChips(), and _showRdFieldCandidates().
 *
 * Registered with StepBuilderRegistry as 'remove_duplicates'.
 */
class RemoveDuplicatesBuilder {
    constructor(panel) {
        this._panel = panel;
        this._rdOutputFieldAC = null;
    }

    // ── Public builder contract ──────────────────────────────────────────────

    render(step) {
        if (!step.config) step.config = {};
        const html = this._buildHTML(step);
        setTimeout(() => this._attachEvents(step), 0);
        return html;
    }

    collectConfig(step) {
        step.config = step.config || {};

        // sourceField is kept as a hidden input — updated whenever the step picker changes
        const rdSrc = document.getElementById('rdSourceField')?.value?.trim();
        if (rdSrc !== undefined) step.config.sourceField = rdSrc;

        const rdKeyJson = document.getElementById('rdKeyFieldsJson')?.value;
        if (rdKeyJson) {
            try { step.config.keyFields = JSON.parse(rdKeyJson) || []; } catch (_) { step.config.keyFields = []; }
        }

        const rdStrategy = document.querySelector('input[name="rdStrategy"]:checked')?.value;
        if (rdStrategy) step.config.strategy = rdStrategy;

        const rdNullKey = document.getElementById('rdNullKeyBehavior')?.value;
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
            { value: 'first', title: 'Keep First',   desc: 'Discard later duplicates — preserves original order' },
            { value: 'last',  title: 'Keep Last',    desc: 'Replace with most recent — useful when later records are corrections' },
            { value: 'merge', title: 'Merge Fields', desc: 'Keep first, backfill missing fields from later records' }
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
                <label class="form-label">Source Step <span style="color:#ef4444">*</span></label>
                <select id="rdSourceStep" class="form-control">
                    <option value="">— select a step —</option>
                </select>
                <div style="margin-top:6px;display:flex;gap:8px;align-items:center">
                    <span style="font-size:12px;color:var(--text-muted,#6b7280);white-space:nowrap;flex-shrink:0">Field within output:</span>
                    <input type="text" id="rdSourceSubPath" class="form-control form-control-sm"
                           placeholder="e.g. rows, records (blank = full output)" style="flex:1;max-width:260px">
                </div>
                <div id="rdComputedPath" style="margin-top:4px;font-size:11px;font-family:monospace;color:var(--text-muted,#9ca3af)"></div>
                <input type="hidden" id="rdSourceField" value="${esc(sourceField)}">
                <div class="field-help">Pick the upstream step that produces the array to deduplicate.</div>
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
                            title="Auto-detect column names from the selected source step">Detect fields</button>
                </div>
                <div class="field-help">Fields forming the unique key. Supports dot-paths (e.g. <code>patient.id</code>).</div>
            </div>

            <div class="form-group">
                <label class="form-label">Duplicate Strategy</label>
                <div class="rd-radio-group">${radioGroup('rdStrategy', strategyOptions, strategy)}</div>
            </div>

            <div class="form-group">
                <label class="form-label">Missing Key Behavior
                    <span style="font-weight:400;color:var(--text-secondary,#64748b);font-size:11px;margin-left:4px">when a key field is null or absent</span>
                </label>
                <select id="rdNullKeyBehavior" class="form-control" style="max-width:320px">
                    <option value="group"${nullKeyBehavior === 'group'  ? ' selected' : ''}>Group — deduplicate null-key records together</option>
                    <option value="keep" ${nullKeyBehavior === 'keep'   ? ' selected' : ''}>Keep All — always pass through null-key records</option>
                    <option value="remove"${nullKeyBehavior === 'remove' ? ' selected' : ''}>Drop — discard records with missing key fields</option>
                </select>
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

    _attachEvents(step) {
        const chipsAPI = this._initChips();
        this._populateSourceSelect(step, chipsAPI);

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

    /**
     * Set up the source step picker dropdown.
     * Populates options from the pipeline, pre-selects from existing config,
     * and wires change event to update the hidden sourceField value.
     */
    _populateSourceSelect(currentStep, chipsAPI) {
        const select      = document.getElementById('rdSourceStep');
        const subPathInput = document.getElementById('rdSourceSubPath');
        const hiddenField = document.getElementById('rdSourceField');
        const pathHint    = document.getElementById('rdComputedPath');
        if (!select) return;

        // Get pipeline steps (exclude this step itself)
        const pipeline = this._panel?.builder?.pipeline || window.pipelineBuilder?.pipeline;
        const currentId = currentStep?.id || this._panel?.currentStep?.id;
        const allSteps = pipeline
            ? (pipeline.getAllSteps?.() || Object.values(pipeline.steps || {}))
            : [];
        const upstream = allSteps.filter(s => s.id !== currentId);

        // Populate dropdown options
        select.innerHTML = '<option value="">— select a step —</option>' +
            upstream.map(s => {
                const ns    = this._computeNamespace(s);
                const type  = (s.stepType || s.step_type || '').split('.').pop();
                const label = `${s.stepName || s.step_name || 'Unnamed'} (${type})`;
                const esc   = v => String(v).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
                return `<option value="${esc(ns)}" data-step-type="${esc(s.stepType || s.step_type || '')}">${esc(label)}</option>`;
            }).join('');

        // Re-compute the full path and update hint + hidden field
        const updatePath = () => {
            const ns  = select.value;
            const sub = subPathInput?.value?.trim() || '';
            const path = ns ? `steps.${ns}.step_output${sub ? '.' + sub : ''}` : '';
            if (hiddenField) hiddenField.value = path;
            if (pathHint) pathHint.textContent = path ? `→ ${path}` : '';
        };

        // Pre-select from existing config value (e.g. "steps.script_b4c9f1.step_output.rows")
        const existing = hiddenField?.value || '';
        if (existing) {
            const parts = existing.split('.');
            if (parts[0] === 'steps' && parts.length >= 3) {
                const ns  = parts[1];
                const sub = parts.slice(3).join('.');   // everything after "step_output"
                select.value = ns;                       // selects the matching option (if present)
                if (subPathInput) subPathInput.value = sub;
            }
        }
        updatePath(); // initialise hint

        // On step change: auto-set default sub-path, update hidden field, try auto-detect
        select.addEventListener('change', () => {
            const opt     = select.options[select.selectedIndex];
            const stepType = opt?.dataset?.stepType || '';
            // Only auto-fill sub-path if the field is currently empty
            if (subPathInput && !subPathInput.value) {
                subPathInput.value = this._defaultSubPath(stepType);
            }
            updatePath();

            // Try to auto-detect key field candidates from test output
            if (chipsAPI) {
                this._detectFieldCandidates().then(candidates => {
                    if (candidates.length > 0) {
                        const detectBtn = document.getElementById('rdDetectBtn');
                        if (detectBtn) {
                            this._showCandidates(candidates, detectBtn,
                                chipsAPI.getFields, chipsAPI.setFields, chipsAPI.renderChips);
                        }
                    }
                }).catch(() => {});
            }
        });

        subPathInput?.addEventListener('input', updatePath);
    }

    _initChips() {
        const chipsContainer = document.getElementById('rdKeyChips');
        const keyFieldInput  = document.getElementById('rdKeyFieldInput');
        const keyFieldsJson  = document.getElementById('rdKeyFieldsJson');
        const addBtn         = document.getElementById('rdAddFieldBtn');
        const detectBtn      = document.getElementById('rdDetectBtn');
        if (!chipsContainer || !keyFieldInput || !keyFieldsJson) return null;

        const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        const getFields  = () => { try { return JSON.parse(keyFieldsJson.value) || []; } catch { return []; } };
        const setFields  = fields => { keyFieldsJson.value = JSON.stringify(fields); };
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

        if (detectBtn) {
            detectBtn.addEventListener('click', async () => {
                const orig = detectBtn.textContent;
                detectBtn.disabled = true;
                detectBtn.textContent = 'Detecting…';
                try {
                    const candidates = await this._detectFieldCandidates();
                    if (candidates.length > 0) {
                        this._showCandidates(candidates, detectBtn, getFields, setFields, renderChips);
                    } else {
                        this._promptFieldNames(getFields, setFields, renderChips);
                    }
                } finally {
                    detectBtn.disabled = false;
                    detectBtn.textContent = orig;
                }
            });
        }

        return { getFields, setFields, renderChips };
    }

    // ── Private: field detection ─────────────────────────────────────────────

    /**
     * Detect field candidates from the selected source step.
     *
     * Test output keys use normalized step name WITHOUT shortID
     * (e.g. "script_enrichment_dedupe"), but the namespace/sourceField uses
     * the full format WITH shortID (e.g. "script_enrichment_dedupe_b9fb33").
     * We resolve this by stripping the 6-char hex shortID suffix.
     */
    async _detectFieldCandidates() {
        const ns      = document.getElementById('rdSourceStep')?.value || '';
        const subPath = document.getElementById('rdSourceSubPath')?.value?.trim() || '';

        // If no cached test output, run a partial pipeline test (without remove_duplicates)
        // so we can inspect the source step's output even before a successful full test run.
        let testOutput = window.pipelineLastTestOutput;
        if (!testOutput?.steps && ns) {
            const partialResult = await this._runPartialPipelineTest();
            if (partialResult?.steps) {
                testOutput = partialResult;
                window.pipelineLastTestOutput = partialResult; // cache for future use
                console.log('[RD detect] Populated test output via partial run:', Object.keys(partialResult.steps));
            }
        }

        console.log('[RD detect] pipelineLastTestOutput present:', !!testOutput,
            '| step ns:', ns, '| sub-path:', subPath || '(none)',
            '| steps keys:', testOutput?.steps ? Object.keys(testOutput.steps) : 'N/A');

        if (testOutput?.steps && ns) {
            // Test output keys use normalizeKey(stepName) WITHOUT shortID suffix.
            // Resolve with three fallback strategies.
            let stepsEntry =
                testOutput.steps[ns] ||                                    // 1. exact match
                testOutput.steps[ns.replace(/_[a-f0-9]{1,8}$/, '')] ||   // 2. strip shortID
                (() => {                                                    // 3. prefix match
                    const k = Object.keys(testOutput.steps).find(k => ns.startsWith(k + '_'));
                    return k ? testOutput.steps[k] : null;
                })();

            console.log('[RD detect] stepsEntry found:', !!stepsEntry,
                '| step_output keys:', stepsEntry?.step_output ? Object.keys(stepsEntry.step_output) : 'N/A');

            if (stepsEntry) {
                let cursor = stepsEntry.step_output;

                // Navigate user-supplied sub-path (e.g. "rows", "records.data")
                if (subPath && cursor) {
                    for (const part of subPath.split('.')) {
                        if (cursor == null || typeof cursor !== 'object') { cursor = null; break; }
                        cursor = cursor[part];
                    }
                }

                // If cursor is still an object (not array), auto-find the first array child.
                // Handles scripts returning {records:[...]} without requiring a sub-path.
                if (cursor && typeof cursor === 'object' && !Array.isArray(cursor)) {
                    for (const [key, val] of Object.entries(cursor)) {
                        if (Array.isArray(val) && val.length > 0) {
                            console.log('[RD detect] Auto-found array child key:', key);
                            // Auto-fill the sub-path input so user sees the correct path
                            const subPathEl = document.getElementById('rdSourceSubPath');
                            if (subPathEl && !subPathEl.value) {
                                subPathEl.value = key;
                                subPathEl.dispatchEvent(new Event('input', { bubbles: true }));
                            }
                            cursor = val;
                            break;
                        }
                    }
                }

                const arr = Array.isArray(cursor) ? cursor : null;
                console.log('[RD detect] arr length:', arr?.length ?? 'N/A (not array)');

                if (arr && arr.length > 0) {
                    const first = arr[0];
                    if (first && typeof first === 'object' && !Array.isArray(first)) {
                        const fields = Object.keys(first).filter(k => !k.startsWith('_'));
                        console.log('[RD detect] fields found:', fields);
                        if (fields.length > 0) return fields;
                    }
                }
            }
        }

        // Fallback: file_parser column definitions from pipeline config
        const pipeline = this._panel?.builder?.pipeline || window.pipelineBuilder?.pipeline;
        const allSteps = pipeline ? (pipeline.getAllSteps?.() || Object.values(pipeline.steps || {})) : [];
        for (const s of allSteps) {
            if ((s.stepType || s.step_type) !== 'file_parser') continue;
            const cols = s.config?.columns;
            if (Array.isArray(cols) && cols.length > 0) {
                return cols.map(c => (typeof c === 'string' ? c : (c.name || c.key || ''))).filter(Boolean);
            }
        }

        return [];
    }

    /**
     * Run the pipeline test with remove_duplicates steps excluded so the test
     * succeeds even when sourceField is not yet correctly configured.
     * This lets us inspect upstream step outputs to detect field candidates.
     * Returns the test result object, or null on failure.
     */
    async _runPartialPipelineTest() {
        const messageInput = document.getElementById('testMessageInput');
        // Use whatever is in the test modal, fall back to a minimal JSON object.
        // Scripts that generate their own data work fine with '{}' as input.
        const sampleMessage = messageInput?.value?.trim() || '{}';
        const pipeline = this._panel?.builder?.pipeline || window.pipelineBuilder?.pipeline;
        if (!pipeline || !window.pipelineAPI) {
            console.log('[RD detect] Partial test skipped: no pipeline or pipelineAPI');
            return null;
        }
        console.log('[RD detect] Sample message for partial test:', sampleMessage.substring(0, 60));
        try {
            const pipelineData = JSON.parse(JSON.stringify(
                pipeline.toJSON ? pipeline.toJSON() : pipeline
            ));
            // Strip all remove_duplicates steps so the pipeline won't fail on
            // sourceField validation while we're still configuring this step.
            let removedCount = 0;
            for (const group of (pipelineData.execution_groups || [])) {
                const before = (group.steps || []).length;
                group.steps = (group.steps || []).filter(s => {
                    const t = s.step_type || s.stepType || s.type || '';
                    return t !== 'remove_duplicates';
                });
                removedCount += before - group.steps.length;
            }
            const totalSteps = (pipelineData.execution_groups || []).reduce((n, g) => n + (g.steps || []).length, 0);
            console.log(`[RD detect] Partial pipeline: removed ${removedCount} remove_duplicates step(s), ${totalSteps} steps remaining in ${(pipelineData.execution_groups || []).length} group(s)`);
            console.log('[RD detect] Running partial pipeline test (remove_duplicates excluded)…');
            return await window.pipelineAPI.testPipeline({ toJSON: () => pipelineData }, sampleMessage);
        } catch (err) {
            console.log('[RD detect] Partial pipeline test failed:', err.message);
            return null;
        }
    }

    // ── Private: namespace helpers ───────────────────────────────────────────

    /**
     * Replicates Go's GenerateStepNamespace() on the client side.
     * Format: {sanitized_alias_or_name}_{first6ofID}
     */
    _computeNamespace(step) {
        const alias = (step.stepAlias || step.step_alias || '').trim();
        let base = alias || this._normalizeStepKey(step.stepName || step.step_name || '');
        // sanitize: lowercase, spaces → _, strip non-alphanumeric/underscore
        base = base.toLowerCase().replace(/\s+/g, '_').replace(/[^a-z0-9_]/g, '');
        const shortId = (step.id || '').substring(0, 6);
        return `${base}_${shortId}`;
    }

    /** Mirrors Go's NormalizeStepKey() */
    _normalizeStepKey(name) {
        let key = name.toLowerCase().replace(/\s+/g, '_').replace(/_enrichment$/, '');
        const map = {
            'field_mapping': 'field_mapping', 'core_transformation': 'field_mapping',
            'metadata': 'metadata', 'api': 'api', 'database': 'database',
            'script': 'script', 'calculated': 'calculated'
        };
        return map[key] || key;
    }

    /** Returns the default sub-path suffix for a given step type */
    _defaultSubPath(stepType) {
        const t = (stepType || '').toLowerCase().replace(/^(?:pre|post)\./, '');
        if (t.includes('database')) return 'rows';
        if (t === 'file_parser') return 'records';
        return '';
    }

    // ── Private: field name prompt ───────────────────────────────────────────

    /**
     * In-app replacement for browser prompt() — shows a small modal asking the
     * user to enter comma-separated field names when auto-detection finds nothing.
     */
    _promptFieldNames(getFields, setFields, renderChips) {
        const overlay = document.createElement('div');
        overlay.style.cssText = `position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.5);
            display:flex;align-items:center;justify-content:center;z-index:200000;`;

        const dialog = document.createElement('div');
        dialog.style.cssText = `background:white;border-radius:12px;box-shadow:0 20px 40px rgba(0,0,0,0.2);
            max-width:440px;width:90%;overflow:hidden;`;
        dialog.innerHTML = `
            <div style="padding:14px 20px;background:rgba(30,58,138,0.07);border-bottom:1px solid rgba(30,58,138,0.12);display:flex;align-items:center;gap:10px;">
                <span style="font-size:16px;">🔑</span>
                <span style="font-weight:600;color:#1e3a8a;font-size:14px;">Enter Field Names</span>
            </div>
            <div style="padding:18px 20px;">
                <p style="margin:0 0 12px;color:var(--text-secondary,#64748b);font-size:13px;line-height:1.5;">
                    No fields were auto-detected from the source step.<br>
                    Run <strong>Test</strong> first to enable auto-detection, or enter field names manually:
                </p>
                <input id="rdManualFieldsInput" type="text" placeholder="e.g. patient_id, name, dob"
                    style="width:100%;box-sizing:border-box;padding:8px 12px;border:1.5px solid #e2e8f0;
                    border-radius:6px;font-size:13px;outline:none;font-family:inherit;"
                    onfocus="this.style.borderColor='#1e3a8a'" onblur="this.style.borderColor='#e2e8f0'"/>
                <small style="color:var(--text-tertiary,#94a3b8);display:block;margin-top:6px;font-size:11px;">Comma-separated field names</small>
            </div>
            <div style="padding:10px 20px;background:var(--bg-secondary,#f8fafc);border-top:1px solid var(--border-color,#e2e8f0);display:flex;justify-content:flex-end;gap:8px;">
                <button id="rdPromptCancel" style="padding:7px 16px;border:1.5px solid #e2e8f0;background:white;color:#64748b;border-radius:6px;cursor:pointer;font-size:13px;font-family:inherit;">Cancel</button>
                <button id="rdPromptOk" style="padding:7px 18px;border:none;background:#1e3a8a;color:white;border-radius:6px;cursor:pointer;font-size:13px;font-weight:500;font-family:inherit;">Add Fields</button>
            </div>
        `;
        overlay.appendChild(dialog);
        document.body.appendChild(overlay);

        const input    = dialog.querySelector('#rdManualFieldsInput');
        const okBtn    = dialog.querySelector('#rdPromptOk');
        const cancelBtn = dialog.querySelector('#rdPromptCancel');
        input.focus();

        const close  = () => overlay.remove();
        const submit = () => {
            const candidates = input.value.split(',').map(f => f.trim()).filter(Boolean);
            if (candidates.length > 0) {
                const fields = getFields();
                candidates.forEach(f => { if (!fields.includes(f)) fields.push(f); });
                setFields(fields);
                renderChips(fields);
            }
            close();
        };

        okBtn.addEventListener('click', submit);
        cancelBtn.addEventListener('click', close);
        input.addEventListener('keydown', e => { if (e.key === 'Enter') submit(); if (e.key === 'Escape') close(); });
        overlay.addEventListener('click', e => { if (e.target === overlay) close(); });
    }

    _showCandidates(candidates, anchor, getFields, setFields, renderChips) {
        document.getElementById('rdFieldCandidatesPopover')?.remove();
        const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

        const popover = document.createElement('div');
        popover.id = 'rdFieldCandidatesPopover';
        popover.className = 'rd-candidates-popover';

        popover.innerHTML = `
            <div class="rd-pop-header">
                <span>Detected fields — click to add</span>
                <button type="button" class="rd-pop-header-close" id="rdCloseCandidates" title="Close">&times;</button>
            </div>
            <div class="rd-pop-items">
                ${candidates.map(c =>
                    `<button type="button" class="rd-candidate-pill" data-field="${esc(c)}">${esc(c)}</button>`
                ).join('')}
            </div>
        `;

        // Position below the anchor button (fixed to viewport)
        document.body.appendChild(popover);
        const rect = anchor.getBoundingClientRect();
        const popW = Math.max(popover.offsetWidth, 240);
        let left = rect.left;
        if (left + popW > window.innerWidth - 12) left = window.innerWidth - popW - 12;
        popover.style.left = Math.max(8, left) + 'px';
        popover.style.top  = (rect.bottom + 6) + 'px';

        popover.querySelector('#rdCloseCandidates').addEventListener('click', () => popover.remove());
        popover.querySelectorAll('.rd-candidate-pill').forEach(btn => {
            btn.addEventListener('click', () => {
                const field = btn.dataset.field;
                const fields = getFields();
                if (!fields.includes(field)) { fields.push(field); setFields(fields); renderChips(fields); }
                btn.disabled = true;
            });
        });

        setTimeout(() => {
            const closeOnOutside = e => {
                if (!popover.contains(e.target) && e.target !== anchor) {
                    popover.remove();
                    document.removeEventListener('click', closeOnOutside);
                }
            };
            document.addEventListener('click', closeOnOutside);
        }, 50);
    }
}

StepBuilderRegistry.register('remove_duplicates', RemoveDuplicatesBuilder);
