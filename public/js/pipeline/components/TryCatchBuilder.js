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
            onError: 'catch'
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
