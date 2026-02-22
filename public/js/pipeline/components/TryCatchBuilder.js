/**
 * TryCatchBuilder - No-code UI for Try-Catch-Finally Container Steps
 *
 * Extends BaseStepConfigBuilder following OOP Template Method Pattern
 *
 * Config Schema (matches try_catch_executor.go):
 * {
 *   trySteps: [],        // Step IDs for try block
 *   catchSteps: [],      // Step IDs for catch block
 *   finallySteps: [],    // Step IDs for finally block
 *   onError: "catch"     // "catch" (default), "suppress", "rethrow"
 * }
 *
 * @class TryCatchBuilder
 * @extends BaseStepConfigBuilder
 */
class TryCatchBuilder extends BaseStepConfigBuilder {
    constructor(container, initialConfig = {}) {
        super(container, initialConfig);
        this.availableSteps = [];
        this.loadAvailableSteps();

        // Pre-planned catch actions catalog
        this.catchActionCatalog = [
            {
                type: 'log_error',
                label: 'Log Exception',
                icon: 'fas fa-file-alt',
                description: 'Log the error message with configurable severity level',
                color: '#ef4444',
                defaultConfig: { level: 'error', includeStepName: true },
                configFields: [
                    { key: 'level', label: 'Log Level', type: 'select', options: ['error', 'warn', 'info'] }
                ]
            },
            {
                type: 'set_error_flag',
                label: 'Set Error Flag',
                icon: 'fas fa-flag',
                description: 'Set _errorHandled = true for downstream steps to check',
                color: '#f59e0b',
                defaultConfig: { flagName: '_errorHandled' },
                configFields: [
                    { key: 'flagName', label: 'Flag Variable', type: 'text', placeholder: '_errorHandled' }
                ]
            },
            {
                type: 'store_error_details',
                label: 'Store Error Details',
                icon: 'fas fa-database',
                description: 'Save full error context (message, timestamp, step) to a variable',
                color: '#8b5cf6',
                defaultConfig: { variableName: '_lastError' },
                configFields: [
                    { key: 'variableName', label: 'Variable Name', type: 'text', placeholder: '_lastError' }
                ]
            },
            {
                type: 'increment_error_counter',
                label: 'Increment Error Counter',
                icon: 'fas fa-plus-circle',
                description: 'Track error count in _errorCount for alerting thresholds',
                color: '#06b6d4',
                defaultConfig: { counterName: '_errorCount' },
                configFields: [
                    { key: 'counterName', label: 'Counter Variable', type: 'text', placeholder: '_errorCount' }
                ]
            },
            {
                type: 'set_default_value',
                label: 'Set Default Value',
                icon: 'fas fa-edit',
                description: 'Set a fallback value for a field when the try block fails',
                color: '#10b981',
                defaultConfig: { fieldName: '', defaultValue: '' },
                configFields: [
                    { key: 'fieldName', label: 'Field Name', type: 'text', placeholder: 'e.g. patient_status' },
                    { key: 'defaultValue', label: 'Default Value', type: 'text', placeholder: 'e.g. unknown' }
                ]
            }
        ];
    }

    /**
     * Load available pipeline steps (excluding this container step)
     */
    loadAvailableSteps() {
        try {
            const pipeline = window.pipelineBuilder?.getPipeline();
            if (!pipeline) return;

            const currentStep = window.pipelineBuilder?.currentStep;
            let allSteps = [];
            if (pipeline.getAllSteps) {
                allSteps = pipeline.getAllSteps();
            } else {
                (pipeline.executionGroups || []).forEach(group => {
                    if (group.steps) allSteps.push(...group.steps);
                });
            }

            // Exclude current step and steps already assigned to other containers
            this.availableSteps = allSteps.filter(s => {
                if (currentStep && s.id === currentStep.id) return false;
                if (s.parentStepId && s.parentStepId !== currentStep?.id) return false;
                return true;
            });

            console.log(`[TryCatchBuilder] ${this.availableSteps.length} available steps`);
        } catch (err) {
            console.warn('[TryCatchBuilder] Failed to load steps:', err);
        }
    }

    // ========================================
    // ABSTRACT METHOD IMPLEMENTATIONS
    // ========================================

    getDefaultConfig() {
        return {
            trySteps: [],
            catchSteps: [],
            finallySteps: [],
            onError: 'catch',
            catchActions: []
        };
    }

    render() {
        const config = this.config;

        this.container.innerHTML = `
            <div class="try-catch-builder">
                <div class="try-catch-header">
                    <h4 style="margin: 0; font-size: 14px; color: var(--text-primary);">
                        Try-Catch-Finally Configuration
                    </h4>
                    <p style="margin: 4px 0 0; font-size: 12px; color: var(--text-secondary);">
                        Wrap steps in error handling. Steps in the <strong>Try</strong> block execute normally.
                        If an error occurs, <strong>Catch</strong> steps handle it.
                        <strong>Finally</strong> steps always execute.
                    </p>
                </div>

                <!-- Error Handling Mode -->
                <div class="form-group" style="margin-top: 12px;">
                    <label style="font-weight: 600; font-size: 13px;">Error Handling Mode</label>
                    <select id="tcOnError" class="form-control" style="margin-top: 4px;">
                        <option value="catch" ${config.onError === 'catch' ? 'selected' : ''}>
                            Catch &amp; Handle - Execute catch block, then continue pipeline
                        </option>
                        <option value="suppress" ${config.onError === 'suppress' ? 'selected' : ''}>
                            Suppress - Ignore errors, continue pipeline
                        </option>
                        <option value="rethrow" ${config.onError === 'rethrow' ? 'selected' : ''}>
                            Rethrow - Propagate error to pipeline (stops execution)
                        </option>
                    </select>
                </div>

                <!-- Zone Sections -->
                ${this.renderZone('try', 'Try Block', config.trySteps, '#22c55e', 'Steps to execute. If any fail, execution moves to Catch block.')}
                ${this.renderZone('catch', 'Catch Block', config.catchSteps, '#ef4444', 'Steps to execute when an error occurs. Receives _error context variable.')}

                <!-- Built-in Catch Actions -->
                ${this.renderCatchActions(config.catchActions || [])}

                ${this.renderZone('finally', 'Finally Block', config.finallySteps, '#3b82f6', 'Steps that ALWAYS execute (success or failure). Receives _trySuccess context variable.')}

                <!-- Info Box -->
                <div style="margin-top: 16px; padding: 10px 14px; background: rgba(59, 130, 246, 0.06); border: 1px solid rgba(59, 130, 246, 0.15); border-radius: 8px; font-size: 12px; color: var(--text-secondary);">
                    <strong style="color: var(--text-primary);">Context Variables:</strong><br>
                    Catch block receives <code>_error.message</code>, <code>_error.caught</code>, <code>_error.timestamp</code><br>
                    Finally block receives <code>_trySuccess</code> (boolean) and <code>_tryError</code> (string if failed)
                </div>
            </div>
        `;
    }

    /**
     * Render a zone section (try/catch/finally) with step chips and add dropdown
     */
    renderZone(zone, label, stepIds, color, description) {
        const steps = (stepIds || []).map(id => {
            const step = this.availableSteps.find(s => s.id === id);
            return step ? { id, name: step.stepName || step.step_name || 'Unknown' } : { id, name: id.substring(0, 8) + '...' };
        });

        const chipsHtml = steps.length > 0
            ? steps.map((s, idx) => `
                <span class="tc-step-chip" data-zone="${zone}" data-step-id="${s.id}" style="
                    display: inline-flex; align-items: center; gap: 4px;
                    padding: 4px 10px; border-radius: 6px; font-size: 12px;
                    background: ${color}15; border: 1px solid ${color}30; color: var(--text-primary);
                    margin: 2px;">
                    <span style="font-weight: 600; color: ${color}; font-size: 10px;">${idx + 1}.</span>
                    ${s.name}
                    <button class="tc-remove-step" data-zone="${zone}" data-step-id="${s.id}" style="
                        background: none; border: none; cursor: pointer; color: var(--text-tertiary);
                        padding: 0 2px; font-size: 14px; line-height: 1;">&times;</button>
                </span>
            `).join('')
            : `<span style="font-size: 12px; color: var(--text-tertiary); font-style: italic;">No steps assigned</span>`;

        // Available steps for dropdown (exclude steps already in ANY zone)
        const usedIds = new Set([
            ...(this.config.trySteps || []),
            ...(this.config.catchSteps || []),
            ...(this.config.finallySteps || [])
        ]);
        const available = this.availableSteps.filter(s => !usedIds.has(s.id));

        return `
            <div class="tc-zone-section" data-zone="${zone}" style="
                margin-top: 12px; padding: 10px 14px;
                border: 1px dashed ${color}40; border-radius: 10px;
                background: ${color}06;">
                <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 6px;">
                    <span style="font-weight: 700; font-size: 11px; text-transform: uppercase;
                        letter-spacing: 0.5px; color: ${color};">${label}</span>
                    <span style="font-size: 11px; color: var(--text-tertiary);">${steps.length} step${steps.length !== 1 ? 's' : ''}</span>
                </div>
                <p style="margin: 0 0 8px; font-size: 11px; color: var(--text-secondary);">${description}</p>
                <div class="tc-zone-chips" data-zone="${zone}" style="min-height: 28px; margin-bottom: 8px;">
                    ${chipsHtml}
                </div>
                ${available.length > 0 ? `
                    <div style="display: flex; gap: 6px; align-items: center;">
                        <select class="tc-add-step-select form-control" data-zone="${zone}" style="flex: 1; font-size: 12px; padding: 4px 8px;">
                            <option value="">-- Add step --</option>
                            ${available.map(s => `<option value="${s.id}">${s.stepName || s.step_name || s.id.substring(0, 12)}</option>`).join('')}
                        </select>
                        <button class="tc-add-step-btn btn btn-sm" data-zone="${zone}" style="
                            font-size: 12px; padding: 4px 10px; background: ${color}; color: white;
                            border: none; border-radius: 6px; cursor: pointer; white-space: nowrap;">+ Add</button>
                    </div>
                ` : `
                    <span style="font-size: 11px; color: var(--text-tertiary);">All steps assigned</span>
                `}
            </div>
        `;
    }

    /**
     * Render built-in catch actions as toggle cards
     */
    renderCatchActions(activeActions) {
        const activeMap = new Map();
        (activeActions || []).forEach(a => activeMap.set(a.type, a));

        const cards = this.catchActionCatalog.map(action => {
            const active = activeMap.get(action.type);
            const isEnabled = active?.enabled || false;
            const currentConfig = active?.config || action.defaultConfig;

            const configFieldsHtml = (action.configFields || []).map(field => {
                const value = currentConfig[field.key] ?? '';
                if (field.type === 'select') {
                    const options = field.options.map(opt =>
                        `<option value="${opt}" ${value === opt ? 'selected' : ''}>${opt}</option>`
                    ).join('');
                    return `
                        <div style="margin-top: 6px; ${!isEnabled ? 'opacity: 0.4; pointer-events: none;' : ''}">
                            <label style="font-size: 10px; color: var(--text-secondary);">${field.label}</label>
                            <select class="catch-action-config form-control" data-action-type="${action.type}" data-config-key="${field.key}"
                                style="font-size: 11px; padding: 2px 6px; margin-top: 2px;">
                                ${options}
                            </select>
                        </div>
                    `;
                }
                return `
                    <div style="margin-top: 6px; ${!isEnabled ? 'opacity: 0.4; pointer-events: none;' : ''}">
                        <label style="font-size: 10px; color: var(--text-secondary);">${field.label}</label>
                        <input type="text" class="catch-action-config form-control" data-action-type="${action.type}" data-config-key="${field.key}"
                            value="${value}" placeholder="${field.placeholder || ''}"
                            style="font-size: 11px; padding: 2px 6px; margin-top: 2px;">
                    </div>
                `;
            }).join('');

            return `
                <div class="catch-action-card" data-action-type="${action.type}" style="
                    display: flex; gap: 10px; padding: 8px 10px;
                    border: 1px solid ${isEnabled ? action.color + '40' : '#e2e8f020'};
                    border-radius: 8px; margin-bottom: 6px;
                    background: ${isEnabled ? action.color + '08' : 'transparent'};
                    transition: all 0.15s;">
                    <div style="flex-shrink: 0; padding-top: 2px;">
                        <label class="catch-action-toggle" style="position: relative; display: inline-block; width: 32px; height: 18px; cursor: pointer;">
                            <input type="checkbox" class="catch-action-checkbox" data-action-type="${action.type}"
                                ${isEnabled ? 'checked' : ''}
                                style="opacity: 0; width: 0; height: 0;">
                            <span style="
                                position: absolute; top: 0; left: 0; right: 0; bottom: 0;
                                background: ${isEnabled ? action.color : '#cbd5e1'};
                                border-radius: 9px; transition: 0.2s;
                            "></span>
                            <span style="
                                position: absolute; top: 2px; left: ${isEnabled ? '16px' : '2px'};
                                width: 14px; height: 14px; background: white;
                                border-radius: 50%; transition: 0.2s;
                            "></span>
                        </label>
                    </div>
                    <div style="flex: 1; min-width: 0;">
                        <div style="display: flex; align-items: center; gap: 6px;">
                            <i class="${action.icon}" style="font-size: 12px; color: ${action.color};"></i>
                            <span style="font-weight: 600; font-size: 12px; color: var(--text-primary);">${action.label}</span>
                        </div>
                        <p style="margin: 2px 0 0; font-size: 11px; color: var(--text-secondary); line-height: 1.3;">
                            ${action.description}
                        </p>
                        ${configFieldsHtml}
                    </div>
                </div>
            `;
        }).join('');

        return `
            <div class="tc-catch-actions-section" style="
                margin-top: 12px; padding: 10px 14px;
                border: 1px solid rgba(239, 68, 68, 0.15); border-radius: 10px;
                background: rgba(239, 68, 68, 0.02);">
                <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
                    <span style="font-weight: 700; font-size: 11px; text-transform: uppercase;
                        letter-spacing: 0.5px; color: #ef4444;">Quick Actions</span>
                    <span style="font-size: 11px; color: var(--text-tertiary);">Built-in error handlers (run before catch steps)</span>
                </div>
                ${cards}
            </div>
        `;
    }

    attachEvents() {
        // Error handling mode change
        const onErrorSelect = this.container.querySelector('#tcOnError');
        if (onErrorSelect) {
            onErrorSelect.addEventListener('change', () => {
                this.config.onError = onErrorSelect.value;
                this.emitChange();
            });
        }

        // Add step buttons
        this.container.querySelectorAll('.tc-add-step-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const zone = btn.dataset.zone;
                const select = this.container.querySelector(`.tc-add-step-select[data-zone="${zone}"]`);
                if (!select || !select.value) return;

                const stepId = select.value;
                const key = zone + 'Steps';
                if (!this.config[key]) this.config[key] = [];
                if (!this.config[key].includes(stepId)) {
                    this.config[key].push(stepId);
                    this.emitChange();
                    this.reRender();
                }
            });
        });

        // Catch action toggles
        this.container.querySelectorAll('.catch-action-checkbox').forEach(checkbox => {
            checkbox.addEventListener('change', () => {
                this.updateCatchActions();
            });
        });

        // Catch action config inputs
        this.container.querySelectorAll('.catch-action-config').forEach(input => {
            const eventType = input.tagName === 'SELECT' ? 'change' : 'blur';
            input.addEventListener(eventType, () => {
                this.updateCatchActions();
            });
        });

        // Remove step buttons
        this.container.querySelectorAll('.tc-remove-step').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const zone = btn.dataset.zone;
                const stepId = btn.dataset.stepId;
                const key = zone + 'Steps';
                if (this.config[key]) {
                    this.config[key] = this.config[key].filter(id => id !== stepId);
                    this.emitChange();
                    this.reRender();
                }
            });
        });
    }

    /**
     * Collect current catch action state from the UI toggle cards
     * and update config.catchActions
     */
    updateCatchActions() {
        const actions = [];

        this.container.querySelectorAll('.catch-action-checkbox').forEach(checkbox => {
            const actionType = checkbox.dataset.actionType;
            const catalogEntry = this.catchActionCatalog.find(a => a.type === actionType);
            if (!catalogEntry) return;

            const actionConfig = { ...catalogEntry.defaultConfig };

            // Read config field values from DOM
            this.container.querySelectorAll(`.catch-action-config[data-action-type="${actionType}"]`).forEach(input => {
                const key = input.dataset.configKey;
                actionConfig[key] = input.value;
            });

            actions.push({
                type: actionType,
                enabled: checkbox.checked,
                config: actionConfig
            });
        });

        // Only store enabled actions (or actions with non-default config)
        this.config.catchActions = actions.filter(a => a.enabled);
        this.emitChange();

        // Re-render to update toggle card visuals (border, background, field opacity)
        this.reRender();
    }

    /**
     * Re-render and re-attach events after step list change
     */
    reRender() {
        this.render();
        this.attachEvents();
    }

    /**
     * Emit config change event
     */
    emitChange() {
        console.log('[TryCatchBuilder] Config changed:', this.config);
        const event = new CustomEvent('configChange', {
            bubbles: true,
            detail: { config: { ...this.config } }
        });
        this.container.dispatchEvent(event);
    }

    /**
     * Get current config (called by PropertiesPanel.collectFormData)
     */
    getConfig() {
        return { ...this.config };
    }
}

// Export
if (typeof module !== 'undefined' && module.exports) {
    module.exports = TryCatchBuilder;
}
