/**
 * SwitchCaseBuilder - No-code UI for Switch/Case conditional logic
 *
 * Extends BaseStepConfigBuilder following OOP Template Method Pattern
 *
 * Design Pattern: OOP with Template Method
 * - Extends BaseStepConfigBuilder (abstract base class)
 * - Implements required abstract methods: getDefaultConfig(), render(), attachEvents()
 * - Uses optional hooks: beforeInit(), afterInit(), addStyles()
 *
 * Features:
 * - Visual switch/case configuration with multiple cases
 * - Smart field search for switch field selection (FieldPathSearchComponent)
 * - Multiple action types per case (continue, stop, set_value, copy_field, transform, route_to_step)
 * - Default case for unmatched values
 * - Advanced options (case-insensitive, trim whitespace)
 *
 * Config Schema:
 * {
 *   field: "PID.8",           // Field to switch on
 *   cases: [
 *     { value: "M", label: "Male", actions: [...] },
 *     { value: "F", label: "Female", actions: [...] }
 *   ],
 *   default: { actions: [...] },
 *   options: { caseInsensitive: false, trimWhitespace: true }
 * }
 *
 * @class SwitchCaseBuilder
 * @extends BaseStepConfigBuilder
 */
class SwitchCaseBuilder extends BaseStepConfigBuilder {
    /**
     * Constructor
     * @param {HTMLElement} container - DOM container for rendering
     * @param {Object} initialConfig - Initial configuration (optional)
     */
    constructor(container, initialConfig = {}) {
        super(container, initialConfig);
        this.fieldSearchComponent = null;
        this.cachedStepVariables = null;

        // Pre-fetch step variables for smart search
        this.loadStepVariables();
    }

    /**
     * Load available step output variables from previous steps
     * Used for smart search in field pickers
     */
    async loadStepVariables() {
        try {
            const pipeline = window.pipelineBuilder?.getPipeline();
            if (!pipeline) {
                this.cachedStepVariables = [];
                return;
            }

            // Get current step context
            const currentStep = window.pipelineBuilder?.currentStep;
            const currentLayer = currentStep?.layer || 'pre';

            // Collect all steps using flat collection pattern
            let allSteps = [];
            if (pipeline.getAllSteps) {
                allSteps = pipeline.getAllSteps();
            } else {
                (pipeline.executionGroups || []).forEach(group => {
                    if (group.steps) {
                        allSteps.push(...group.steps);
                    }
                });
            }

            // Build backend format with all steps under current layer
            const backendLayers = {};
            backendLayers[currentLayer] = { steps: allSteps };

            // Find current step index
            const stepIndex = allSteps.findIndex(s => s.id === currentStep?.id);

            const response = await fetch('/api/pipeline/reference-variables', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    pipeline: { layers: backendLayers },
                    current_layer: currentLayer,
                    current_step: stepIndex >= 0 ? stepIndex : 0
                })
            });

            if (response.ok) {
                const data = await response.json();
                this.cachedStepVariables = this.transformVariablesToSearchFormat(data.variables || []);
                console.log('[SwitchCaseBuilder] Loaded', this.cachedStepVariables.length, 'step variables for smart search');
            } else {
                this.cachedStepVariables = [];
            }
        } catch (err) {
            console.warn('[SwitchCaseBuilder] Failed to load step variables:', err);
            this.cachedStepVariables = [];
        }
    }

    /**
     * Transform backend variables format to FieldPathSearchComponent format
     */
    transformVariablesToSearchFormat(variables) {
        const searchFields = [];

        variables.forEach(category => {
            const stepName = category.category || 'Unknown Step';
            const stepType = category.stepType || 'unknown';

            // Determine category based on step type
            let displayCategory = 'Step Outputs';
            if (stepType.includes('database')) displayCategory = 'Database';
            else if (stepType.includes('api')) displayCategory = 'API';
            else if (stepType.includes('script')) displayCategory = 'Script';

            if (category.variables) {
                category.variables.forEach(variable => {
                    searchFields.push({
                        name: `${stepName}: ${variable.name}`,
                        path: variable.path,
                        description: variable.description || `From ${stepName}`,
                        category: displayCategory,
                        examples: variable.examples || []
                    });
                });
            }
        });

        return searchFields;
    }

    /**
     * Get step variables for FieldPathSearchComponent
     */
    getStepVariablesForSearch() {
        return this.cachedStepVariables || [];
    }

    // ========================================
    // ABSTRACT METHOD IMPLEMENTATIONS (required)
    // ========================================

    /**
     * Returns default configuration structure
     * @override
     * @returns {Object} Default configuration object
     */
    getDefaultConfig() {
        return {
            field: '',
            cases: [
                {
                    value: '',
                    label: 'Case 1',
                    actions: [{ action: 'continue' }]
                }
            ],
            default: {
                actions: [{ action: 'continue' }]
            },
            options: {
                caseInsensitive: false,
                trimWhitespace: true
            }
        };
    }

    /**
     * Renders the UI into the container
     * @override
     */
    render() {
        // Clear container
        this.clear();
        this.container.className = 'switch-case-builder';

        // Normalize config structure
        this.normalizeConfig();

        // Build UI components
        const wrapper = document.createElement('div');
        wrapper.className = 'switch-case-wrapper';

        // Header with title and help
        wrapper.appendChild(this.createHeader());

        // Switch field section
        wrapper.appendChild(this.createSwitchFieldSection());

        // Cases section
        wrapper.appendChild(this.createCasesSection());

        // Default section
        wrapper.appendChild(this.createDefaultSection());

        // Advanced options
        wrapper.appendChild(this.createAdvancedToggle());

        this.container.appendChild(wrapper);

        // Initialize field search components
        this.initializeFieldSearch();
        this.initializeActionFieldSearchComponents();
    }

    /**
     * Attaches event listeners to rendered elements
     * @override
     */
    attachEvents() {
        // Switch field input
        const fieldInput = this.container.querySelector('#switch-field-input');
        if (fieldInput) {
            fieldInput.addEventListener('change', (e) => {
                this.config.field = e.target.value;
                this.notifyChange();
            });
        }

        // Add case button
        const addCaseBtn = this.container.querySelector('#add-case-btn');
        if (addCaseBtn) {
            addCaseBtn.addEventListener('click', () => this.addCase());
        }

        // Case value and label inputs
        this.container.querySelectorAll('.case-value-input').forEach(input => {
            input.addEventListener('change', (e) => {
                const index = parseInt(e.target.dataset.index);
                if (this.config.cases[index]) {
                    this.config.cases[index].value = e.target.value;
                    this.notifyChange();
                }
            });
        });

        this.container.querySelectorAll('.case-label-input').forEach(input => {
            input.addEventListener('change', (e) => {
                const index = parseInt(e.target.dataset.index);
                if (this.config.cases[index]) {
                    this.config.cases[index].label = e.target.value;
                    this.notifyChange();
                }
            });
        });

        // Delete case buttons
        this.container.querySelectorAll('.delete-case-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const index = parseInt(e.target.closest('.delete-case-btn').dataset.index);
                this.deleteCase(index);
            });
        });

        // Action type selects
        this.container.querySelectorAll('.action-type-select').forEach(select => {
            select.addEventListener('change', (e) => {
                const caseIndex = parseInt(e.target.dataset.caseIndex);
                const actionIndex = parseInt(e.target.dataset.actionIndex);
                this.updateActionType(caseIndex, actionIndex, e.target.value);
            });
        });

        // Action config inputs (excluding multi-step-add which has special handling)
        this.container.querySelectorAll('.action-field-input, .action-value-input, .action-transform-select, .action-step-select:not(.multi-step-add)').forEach(input => {
            input.addEventListener('change', (e) => {
                const caseIndex = parseInt(e.target.dataset.caseIndex);
                const actionIndex = parseInt(e.target.dataset.actionIndex);
                const field = e.target.dataset.field;
                this.updateActionConfig(caseIndex, actionIndex, field, e.target.value);
            });
        });

        // Multi-step add dropdown (route_to_step with multiple targets)
        this.container.querySelectorAll('.multi-step-add').forEach(select => {
            select.addEventListener('change', (e) => {
                const caseIndex = parseInt(e.target.dataset.caseIndex);
                const actionIndex = parseInt(e.target.dataset.actionIndex);
                const stepId = e.target.value;
                if (stepId) {
                    this.addTargetStep(caseIndex, actionIndex, stepId);
                    e.target.value = ''; // Reset dropdown
                }
            });
        });

        // Remove step buttons (for multi-step routing)
        this.container.querySelectorAll('.remove-step-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const target = e.target.closest('.remove-step-btn');
                const caseIndex = parseInt(target.dataset.caseIndex);
                const actionIndex = parseInt(target.dataset.actionIndex);
                const stepId = target.dataset.stepId;
                this.removeTargetStep(caseIndex, actionIndex, stepId);
            });
        });

        // Add action buttons
        this.container.querySelectorAll('.add-action-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const caseIndex = parseInt(e.target.dataset.caseIndex);
                this.addAction(caseIndex);
            });
        });

        // Delete action buttons
        this.container.querySelectorAll('.delete-action-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const target = e.target.closest('.delete-action-btn');
                const caseIndex = parseInt(target.dataset.caseIndex);
                const actionIndex = parseInt(target.dataset.actionIndex);
                this.deleteAction(caseIndex, actionIndex);
            });
        });

        // Advanced toggle
        const advancedToggle = this.container.querySelector('#advanced-toggle-btn');
        if (advancedToggle) {
            advancedToggle.addEventListener('click', () => {
                const options = this.container.querySelector('#advanced-options');
                const icon = advancedToggle.querySelector('.toggle-icon');
                if (options.style.display === 'none') {
                    options.style.display = 'block';
                    icon.textContent = '▼';
                } else {
                    options.style.display = 'none';
                    icon.textContent = '▶';
                }
            });
        }

        // Advanced options checkboxes
        const caseInsensitiveCheck = this.container.querySelector('#case-insensitive-check');
        if (caseInsensitiveCheck) {
            caseInsensitiveCheck.addEventListener('change', (e) => {
                if (!this.config.options) this.config.options = {};
                this.config.options.caseInsensitive = e.target.checked;
                this.notifyChange();
            });
        }

        const trimValuesCheck = this.container.querySelector('#trim-values-check');
        if (trimValuesCheck) {
            trimValuesCheck.addEventListener('change', (e) => {
                if (!this.config.options) this.config.options = {};
                this.config.options.trimWhitespace = e.target.checked;
                this.notifyChange();
            });
        }

        // Help button
        const helpBtn = this.container.querySelector('.switch-help-btn');
        if (helpBtn) {
            helpBtn.addEventListener('click', () => this.showHelp());
        }
    }

    // ========================================
    // HOOK METHOD IMPLEMENTATIONS (optional)
    // ========================================

    /**
     * Hook: Called before initialization begins
     * @override
     * @protected
     */
    beforeInit() {
        console.log('[SwitchCaseBuilder] beforeInit - preparing builder');
    }

    /**
     * Hook: Called after initialization completes
     * @override
     * @protected
     */
    afterInit() {
        console.log('[SwitchCaseBuilder] afterInit - builder ready');
    }

    /**
     * Hook: Adds custom CSS styles
     * @override
     * @protected
     */
    addStyles() {
        if (document.getElementById('switch-case-builder-styles')) {
            return;
        }

        const style = document.createElement('style');
        style.id = 'switch-case-builder-styles';
        style.textContent = `
            /* Switch/Case Builder Styles */
            .switch-case-builder {
                font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
                padding: 16px;
                background: #f8f9fa;
                border-radius: 8px;
            }

            .switch-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 8px;
            }

            .switch-title {
                font-size: 16px;
                font-weight: 600;
                color: #333;
                display: flex;
                align-items: center;
                gap: 8px;
            }

            .switch-icon {
                font-size: 20px;
            }

            .switch-help-btn {
                width: 24px;
                height: 24px;
                border-radius: 50%;
                border: 1px solid #ddd;
                background: white;
                cursor: pointer;
                font-weight: bold;
                color: #666;
            }

            .switch-help-btn:hover {
                background: #e9ecef;
            }

            .switch-description {
                color: #666;
                font-size: 13px;
                margin-bottom: 16px;
            }

            .switch-label {
                display: flex;
                align-items: center;
                gap: 6px;
                font-weight: 600;
                font-size: 13px;
                color: #333;
                margin-bottom: 8px;
            }

            .label-icon {
                font-size: 14px;
            }

            /* Switch Field Section */
            .switch-field-section {
                background: white;
                padding: 16px;
                border-radius: 8px;
                margin-bottom: 16px;
                border: 1px solid #e0e0e0;
            }

            .switch-field-input-wrapper {
                position: relative;
            }

            .switch-field-input {
                width: 100%;
                padding: 10px 12px;
                border: 1px solid #ddd;
                border-radius: 6px;
                font-size: 14px;
                box-sizing: border-box;
            }

            .switch-field-input:focus {
                outline: none;
                border-color: #4a90e2;
                box-shadow: 0 0 0 3px rgba(74, 144, 226, 0.1);
            }

            .switch-field-hint {
                font-size: 11px;
                color: #888;
                margin-top: 4px;
                display: block;
            }

            /* Cases Section */
            .switch-cases-section {
                margin-bottom: 16px;
            }

            .cases-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 12px;
            }

            .add-case-btn {
                padding: 6px 12px;
                background: #4a90e2;
                color: white;
                border: none;
                border-radius: 4px;
                cursor: pointer;
                font-size: 13px;
                display: flex;
                align-items: center;
                gap: 4px;
            }

            .add-case-btn:hover {
                background: #3a7bc8;
            }

            .cases-list {
                display: flex;
                flex-direction: column;
                gap: 12px;
            }

            /* Case Card */
            .case-card {
                background: white;
                border: 1px solid #e0e0e0;
                border-radius: 8px;
                overflow: hidden;
            }

            .case-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 12px 16px;
                background: #f5f7fa;
                border-bottom: 1px solid #e0e0e0;
                flex-wrap: wrap;
                gap: 8px;
            }

            .case-match {
                display: flex;
                align-items: center;
                gap: 10px;
            }

            .case-badge {
                background: #4a90e2;
                color: white;
                padding: 4px 10px;
                border-radius: 4px;
                font-size: 11px;
                font-weight: 600;
            }

            .case-equals {
                color: #666;
                font-weight: bold;
            }

            .case-value-input {
                padding: 6px 10px;
                border: 1px solid #ddd;
                border-radius: 4px;
                font-size: 14px;
                width: 150px;
            }

            .case-value-input:focus {
                outline: none;
                border-color: #4a90e2;
            }

            .case-controls {
                display: flex;
                align-items: center;
                gap: 8px;
            }

            .case-label-input {
                padding: 6px 10px;
                border: 1px solid #ddd;
                border-radius: 4px;
                font-size: 12px;
                width: 120px;
                color: #666;
            }

            .delete-case-btn {
                background: none;
                border: none;
                cursor: pointer;
                padding: 4px 8px;
                border-radius: 4px;
                opacity: 0.6;
            }

            .delete-case-btn:hover {
                background: #fee;
                opacity: 1;
            }

            /* Case Actions */
            .case-actions {
                padding: 12px 16px;
            }

            .actions-label {
                font-size: 12px;
                color: #666;
                margin-bottom: 8px;
                display: block;
            }

            .actions-list {
                display: flex;
                flex-direction: column;
                gap: 8px;
                margin-bottom: 8px;
            }

            .action-row {
                display: flex;
                align-items: center;
                gap: 8px;
                padding: 8px;
                background: #f8f9fa;
                border-radius: 4px;
                flex-wrap: wrap;
            }

            .action-type-select {
                padding: 6px 10px;
                border: 1px solid #ddd;
                border-radius: 4px;
                font-size: 13px;
                min-width: 140px;
            }

            .action-config {
                flex: 1;
                display: flex;
                align-items: center;
                min-width: 200px;
            }

            .action-fields {
                display: flex;
                align-items: center;
                gap: 8px;
                flex: 1;
                flex-wrap: wrap;
            }

            .action-field-input,
            .action-value-input {
                padding: 6px 10px;
                border: 1px solid #ddd;
                border-radius: 4px;
                font-size: 13px;
                flex: 1;
                min-width: 100px;
            }

            /* Field search containers for smart search */
            .field-search-container {
                flex: 1;
                min-width: 120px;
                position: relative;
            }

            .field-search-container .field-search-input {
                width: 100%;
                box-sizing: border-box;
            }

            .action-transform-select,
            .action-step-select {
                padding: 6px 10px;
                border: 1px solid #ddd;
                border-radius: 4px;
                font-size: 13px;
            }

            /* Multi-step routing styles */
            .route-to-steps-container {
                flex-direction: column;
                align-items: flex-start;
                gap: 8px;
            }

            .multi-step-select-wrapper {
                display: flex;
                flex-direction: column;
                gap: 8px;
                width: 100%;
            }

            .multi-step-add {
                min-width: 200px;
            }

            .selected-steps-list {
                display: flex;
                flex-wrap: wrap;
                gap: 6px;
                min-height: 28px;
            }

            .selected-step-chip {
                display: flex;
                align-items: center;
                gap: 4px;
                padding: 4px 8px;
                background: #e3f2fd;
                border: 1px solid #90caf9;
                border-radius: 16px;
                font-size: 12px;
            }

            .selected-step-chip .step-order {
                font-weight: bold;
                color: #1565c0;
            }

            .selected-step-chip .step-name {
                color: #333;
            }

            .selected-step-chip .remove-step-btn {
                background: none;
                border: none;
                cursor: pointer;
                color: #666;
                font-size: 14px;
                padding: 0 2px;
                margin-left: 2px;
                line-height: 1;
            }

            .selected-step-chip .remove-step-btn:hover {
                color: #e53935;
            }

            .no-steps-selected {
                color: #999;
                font-style: italic;
                font-size: 12px;
            }

            .multi-step-hint {
                font-size: 11px;
                color: #888;
                margin-top: 2px;
            }

            .action-separator {
                color: #666;
                font-weight: bold;
                padding: 0 4px;
            }

            .no-config {
                color: #888;
                font-size: 12px;
                font-style: italic;
            }

            .delete-action-btn {
                background: none;
                border: none;
                cursor: pointer;
                color: #999;
                font-size: 18px;
                padding: 2px 6px;
                border-radius: 4px;
            }

            .delete-action-btn:hover {
                background: #fee;
                color: #e53935;
            }

            .add-action-btn {
                padding: 4px 10px;
                background: none;
                border: 1px dashed #ccc;
                border-radius: 4px;
                color: #666;
                cursor: pointer;
                font-size: 12px;
            }

            .add-action-btn:hover {
                border-color: #4a90e2;
                color: #4a90e2;
            }

            /* Default Section */
            .default-section {
                background: #fff8e1;
                border: 1px solid #ffe082;
                border-radius: 8px;
                padding: 16px;
                margin-bottom: 16px;
            }

            .default-header {
                margin-bottom: 4px;
            }

            .default-description {
                font-size: 12px;
                color: #666;
                margin-bottom: 12px;
            }

            /* Advanced Toggle */
            .advanced-toggle {
                margin-top: 16px;
            }

            .advanced-toggle-btn {
                background: none;
                border: none;
                color: #666;
                cursor: pointer;
                font-size: 13px;
                padding: 8px 0;
                display: flex;
                align-items: center;
                gap: 6px;
            }

            .advanced-toggle-btn:hover {
                color: #333;
            }

            .toggle-icon {
                font-size: 10px;
            }

            .advanced-options {
                padding: 12px;
                background: white;
                border: 1px solid #e0e0e0;
                border-radius: 6px;
                margin-top: 8px;
            }

            .advanced-option {
                margin-bottom: 12px;
            }

            .advanced-option:last-child {
                margin-bottom: 0;
            }

            .advanced-option label {
                display: flex;
                align-items: center;
                gap: 8px;
                font-size: 13px;
                color: #333;
                cursor: pointer;
            }

            .option-help {
                display: block;
                font-size: 11px;
                color: #888;
                margin-left: 24px;
                margin-top: 4px;
            }

            /* Help Modal */
            .switch-help-overlay {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.5);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 10001;
            }

            .switch-help-content {
                background: white;
                padding: 24px;
                border-radius: 8px;
                max-width: 500px;
                max-height: 80vh;
                overflow-y: auto;
            }

            .switch-help-content h3 {
                margin-top: 0;
                color: #333;
            }

            .switch-help-content h4 {
                color: #555;
                margin-top: 16px;
            }

            .switch-help-content pre {
                background: #f5f5f5;
                padding: 12px;
                border-radius: 4px;
                overflow-x: auto;
                font-size: 12px;
            }

            .switch-help-close {
                margin-top: 16px;
                padding: 8px 16px;
                background: #4a90e2;
                color: white;
                border: none;
                border-radius: 4px;
                cursor: pointer;
            }

            .switch-help-close:hover {
                background: #3a7bc8;
            }
        `;
        document.head.appendChild(style);
    }

    // ========================================
    // VALIDATION (override base)
    // ========================================

    /**
     * Validates current configuration
     * @override
     * @returns {Object} { valid: boolean, errors: string[] }
     */
    validate() {
        const errors = [];

        // Validate switch field
        if (!this.config.field || this.config.field.trim() === '') {
            errors.push('Switch field is required');
        }

        // Validate cases
        if (!this.config.cases || this.config.cases.length === 0) {
            errors.push('At least one case is required');
        } else {
            this.config.cases.forEach((caseItem, index) => {
                if (!caseItem.value || caseItem.value.trim() === '') {
                    errors.push(`Case ${index + 1}: value is required`);
                }
                if (!caseItem.actions || caseItem.actions.length === 0) {
                    errors.push(`Case ${index + 1}: at least one action is required`);
                }
            });
        }

        // Validate default actions
        if (!this.config.default?.actions || this.config.default.actions.length === 0) {
            errors.push('Default case must have at least one action');
        }

        return {
            valid: errors.length === 0,
            errors: errors
        };
    }

    // ========================================
    // PRIVATE HELPER METHODS
    // ========================================

    /**
     * Normalizes config structure with defaults
     * @private
     */
    normalizeConfig() {
        if (!this.config.cases || !Array.isArray(this.config.cases)) {
            this.config.cases = [{ value: '', label: 'Case 1', actions: [{ action: 'continue' }] }];
        }

        if (!this.config.default || !this.config.default.actions) {
            this.config.default = { actions: [{ action: 'continue' }] };
        }

        if (!this.config.options) {
            this.config.options = { caseInsensitive: false, trimWhitespace: true };
        }
    }

    /**
     * Creates header section
     * @private
     */
    createHeader() {
        const wrapper = document.createElement('div');
        wrapper.className = 'switch-header-wrapper';

        wrapper.innerHTML = `
            <div class="switch-header">
                <div class="switch-title">
                    <span class="switch-icon">🔀</span>
                    <span>Switch / Case Configuration</span>
                </div>
                <button type="button" class="switch-help-btn" title="How Switch/Case works">
                    <span>?</span>
                </button>
            </div>
            <p class="switch-description">
                Route processing based on a field's value. Each case matches a specific value and executes its actions.
            </p>
        `;

        return wrapper;
    }

    /**
     * Creates switch field selection section
     * @private
     */
    createSwitchFieldSection() {
        const section = document.createElement('div');
        section.className = 'switch-field-section';
        section.innerHTML = `
            <label class="switch-label">
                <span class="label-icon">📊</span>
                Switch On Field
            </label>
            <div class="switch-field-input-wrapper">
                <input type="text"
                       class="switch-field-input"
                       id="switch-field-input"
                       placeholder="Search for a field (e.g., PID.8, MSH.9)..."
                       value="${this.escapeHtml(this.config.field || '')}">
                <span class="switch-field-hint">The field value will be compared against each case</span>
            </div>
        `;
        return section;
    }

    /**
     * Creates cases section with all case cards
     * @private
     */
    createCasesSection() {
        const section = document.createElement('div');
        section.className = 'switch-cases-section';

        const casesHtml = this.config.cases.map((caseItem, index) =>
            this.createCaseCardHtml(caseItem, index)
        ).join('');

        section.innerHTML = `
            <div class="cases-header">
                <label class="switch-label">
                    <span class="label-icon">📋</span>
                    Cases
                </label>
                <button type="button" class="add-case-btn" id="add-case-btn">
                    <span>+</span> Add Case
                </button>
            </div>
            <div class="cases-list" id="cases-list">
                ${casesHtml}
            </div>
        `;
        return section;
    }

    /**
     * Creates HTML for a single case card
     * @private
     */
    createCaseCardHtml(caseItem, index) {
        const actionsHtml = this.createActionsHtml(caseItem.actions || [], index, 'case');

        return `
            <div class="case-card" data-case-index="${index}">
                <div class="case-header">
                    <div class="case-match">
                        <span class="case-badge">CASE ${index + 1}</span>
                        <span class="case-equals">=</span>
                        <input type="text"
                               class="case-value-input"
                               data-index="${index}"
                               placeholder="Value to match..."
                               value="${this.escapeHtml(caseItem.value || '')}">
                    </div>
                    <div class="case-controls">
                        <input type="text"
                               class="case-label-input"
                               data-index="${index}"
                               placeholder="Label (optional)"
                               value="${this.escapeHtml(caseItem.label || '')}">
                        <button type="button" class="delete-case-btn" data-index="${index}" title="Delete case">
                            🗑️
                        </button>
                    </div>
                </div>
                <div class="case-actions">
                    <label class="actions-label">Then execute:</label>
                    <div class="actions-list" data-case-index="${index}">
                        ${actionsHtml}
                    </div>
                    <button type="button" class="add-action-btn" data-case-index="${index}" data-section="case">
                        + Add Action
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * Creates default section
     * @private
     */
    createDefaultSection() {
        const section = document.createElement('div');
        section.className = 'default-section';
        const actionsHtml = this.createActionsHtml(
            this.config.default?.actions || [{ action: 'continue' }],
            -1,
            'default'
        );

        section.innerHTML = `
            <div class="default-header">
                <label class="switch-label">
                    <span class="label-icon">↩️</span>
                    Default (No Match)
                </label>
            </div>
            <p class="default-description">Actions to execute when no case matches the field value</p>
            <div class="default-actions">
                <div class="actions-list" data-case-index="-1">
                    ${actionsHtml}
                </div>
                <button type="button" class="add-action-btn" data-case-index="-1" data-section="default">
                    + Add Action
                </button>
            </div>
        `;
        return section;
    }

    /**
     * Creates HTML for actions list
     * @private
     */
    createActionsHtml(actions, caseIndex, section) {
        if (!actions || actions.length === 0) {
            actions = [{ action: 'continue' }];
        }

        return actions.map((action, actionIndex) =>
            this.createActionRowHtml(action, caseIndex, actionIndex, section)
        ).join('');
    }

    /**
     * Creates HTML for a single action row
     * @private
     */
    createActionRowHtml(action, caseIndex, actionIndex, section) {
        const actionType = action.action || 'continue';

        return `
            <div class="action-row" data-action-index="${actionIndex}">
                <select class="action-type-select"
                        data-case-index="${caseIndex}"
                        data-action-index="${actionIndex}"
                        data-section="${section}">
                    ${this.createActionOptions(actionType)}
                </select>
                <div class="action-config" data-case-index="${caseIndex}" data-action-index="${actionIndex}">
                    ${this.createActionConfigHtml(action, caseIndex, actionIndex)}
                </div>
                <button type="button" class="delete-action-btn"
                        data-case-index="${caseIndex}"
                        data-action-index="${actionIndex}"
                        data-section="${section}"
                        title="Remove action">×</button>
            </div>
        `;
    }

    /**
     * Creates action type dropdown options
     * @private
     */
    createActionOptions(selectedAction) {
        const actions = [
            { value: 'continue', label: 'Continue', icon: '▶️' },
            { value: 'stop', label: 'Stop Processing', icon: '⏹️' },
            { value: 'set_value', label: 'Set Value', icon: '✏️' },
            { value: 'copy_field', label: 'Copy Field', icon: '📋' },
            { value: 'transform', label: 'Transform', icon: '🔄' },
            { value: 'route_to_step', label: 'Route to Step', icon: '🔀' }
        ];

        return actions.map(a => `
            <option value="${a.value}" ${a.value === selectedAction ? 'selected' : ''}>
                ${a.icon} ${a.label}
            </option>
        `).join('');
    }

    /**
     * Creates action-specific configuration inputs
     * Uses div containers for FieldPathSearchComponent initialization
     * @private
     */
    createActionConfigHtml(action, caseIndex, actionIndex) {
        const actionType = action.action || 'continue';
        const prefix = `action-${caseIndex}-${actionIndex}`;

        switch (actionType) {
            case 'continue':
            case 'stop':
                return '<span class="no-config">No configuration needed</span>';

            case 'set_value':
                return `
                    <div class="action-fields">
                        <div class="field-search-container" id="${prefix}-targetField"
                             data-field="targetField"
                             data-case-index="${caseIndex}"
                             data-action-index="${actionIndex}"
                             data-initial-value="${this.escapeHtml(action.targetField || '')}"></div>
                        <span class="action-separator">=</span>
                        <input type="text" class="action-value-input"
                               data-field="value"
                               data-case-index="${caseIndex}"
                               data-action-index="${actionIndex}"
                               placeholder="Value"
                               value="${this.escapeHtml(action.value || '')}">
                    </div>
                `;

            case 'copy_field':
                return `
                    <div class="action-fields">
                        <div class="field-search-container" id="${prefix}-sourceField"
                             data-field="sourceField"
                             data-case-index="${caseIndex}"
                             data-action-index="${actionIndex}"
                             data-initial-value="${this.escapeHtml(action.sourceField || '')}"></div>
                        <span class="action-separator">→</span>
                        <div class="field-search-container" id="${prefix}-targetField"
                             data-field="targetField"
                             data-case-index="${caseIndex}"
                             data-action-index="${actionIndex}"
                             data-initial-value="${this.escapeHtml(action.targetField || '')}"></div>
                    </div>
                `;

            case 'transform':
                // Simple pattern like Copy Field: source → transform → target
                // Default to 'uppercase' if transformation not set (backward compatibility)
                const transformType = action.transformation || 'uppercase';
                // Also update the config so it gets saved correctly
                if (!action.transformation) {
                    action.transformation = 'uppercase';
                }
                return `
                    <div class="action-fields">
                        <div class="field-search-container" id="${prefix}-sourceField"
                             data-field="sourceField"
                             data-case-index="${caseIndex}"
                             data-action-index="${actionIndex}"
                             data-initial-value="${this.escapeHtml(action.sourceField || action.field || '')}"></div>
                        <select class="action-transform-select"
                                data-field="transformation"
                                data-case-index="${caseIndex}"
                                data-action-index="${actionIndex}">
                            <option value="uppercase" ${transformType === 'uppercase' ? 'selected' : ''}>UPPERCASE</option>
                            <option value="lowercase" ${transformType === 'lowercase' ? 'selected' : ''}>lowercase</option>
                            <option value="trim" ${transformType === 'trim' ? 'selected' : ''}>Trim whitespace</option>
                            <option value="capitalize" ${transformType === 'capitalize' ? 'selected' : ''}>Capitalize</option>
                        </select>
                        <span class="action-separator">→</span>
                        <div class="field-search-container" id="${prefix}-targetField"
                             data-field="targetField"
                             data-case-index="${caseIndex}"
                             data-action-index="${actionIndex}"
                             data-initial-value="${this.escapeHtml(action.targetField || '')}"></div>
                    </div>
                `;

            case 'route_to_step':
                // Support multiple step selection
                const selectedStepIds = action.targetStepIds || (action.targetStepId ? [action.targetStepId] : []);
                return `
                    <div class="action-fields route-to-steps-container">
                        <div class="multi-step-select-wrapper">
                            <select class="action-step-select multi-step-add"
                                    data-case-index="${caseIndex}"
                                    data-action-index="${actionIndex}">
                                <option value="">+ Add target step...</option>
                                ${this.getAvailableStepsOptions('')}
                            </select>
                            <div class="selected-steps-list"
                                 data-case-index="${caseIndex}"
                                 data-action-index="${actionIndex}">
                                ${this.renderSelectedSteps(selectedStepIds, caseIndex, actionIndex)}
                            </div>
                        </div>
                        <span class="multi-step-hint">Select multiple steps to execute in sequence</span>
                    </div>
                `;

            default:
                return '<span class="no-config">Unknown action type</span>';
        }
    }

    /**
     * Gets available steps for routing dropdown
     * @private
     */
    getAvailableStepsOptions(selectedStepId) {
        if (typeof window.pipelineBuilder !== 'undefined') {
            const allSteps = window.pipelineBuilder.getAllSteps ?
                window.pipelineBuilder.getAllSteps() : [];

            return allSteps.map(step => `
                <option value="${step.id}" ${step.id === selectedStepId ? 'selected' : ''}>
                    Step ${step.sequence || ''} - ${step.stepName || step.step_name || 'Unnamed'}
                </option>
            `).join('');
        }
        return '';
    }

    /**
     * Renders selected steps as chips/tags
     * @private
     */
    renderSelectedSteps(stepIds, caseIndex, actionIndex) {
        if (!stepIds || stepIds.length === 0) {
            return '<span class="no-steps-selected">No steps selected</span>';
        }

        const allSteps = window.pipelineBuilder?.getAllSteps?.() || [];
        const stepMap = new Map(allSteps.map(s => [s.id, s]));

        return stepIds.map((stepId, idx) => {
            const step = stepMap.get(stepId);
            const stepName = step ?
                `Step ${step.sequence || ''} - ${step.stepName || step.step_name || 'Unnamed'}` :
                stepId;

            return `
                <div class="selected-step-chip" data-step-id="${stepId}" data-index="${idx}">
                    <span class="step-order">${idx + 1}.</span>
                    <span class="step-name">${this.escapeHtml(stepName)}</span>
                    <button type="button" class="remove-step-btn"
                            data-case-index="${caseIndex}"
                            data-action-index="${actionIndex}"
                            data-step-id="${stepId}"
                            title="Remove step">×</button>
                </div>
            `;
        }).join('');
    }

    /**
     * Adds a step to the targetStepIds array
     * @private
     */
    addTargetStep(caseIndex, actionIndex, stepId) {
        if (!stepId) return;

        const actions = caseIndex === -1 ?
            this.config.default.actions :
            this.config.cases[caseIndex]?.actions;

        if (actions && actions[actionIndex]) {
            const action = actions[actionIndex];

            // Initialize targetStepIds array if needed
            if (!action.targetStepIds) {
                action.targetStepIds = [];
                // Migrate from single targetStepId if exists
                if (action.targetStepId) {
                    action.targetStepIds.push(action.targetStepId);
                    delete action.targetStepId;
                }
            }

            // Add step if not already present
            if (!action.targetStepIds.includes(stepId)) {
                action.targetStepIds.push(stepId);
                this.render();
                this.attachEvents();
                this.notifyChange();
            }
        }
    }

    /**
     * Removes a step from the targetStepIds array
     * @private
     */
    removeTargetStep(caseIndex, actionIndex, stepId) {
        const actions = caseIndex === -1 ?
            this.config.default.actions :
            this.config.cases[caseIndex]?.actions;

        if (actions && actions[actionIndex]) {
            const action = actions[actionIndex];

            if (action.targetStepIds && Array.isArray(action.targetStepIds)) {
                action.targetStepIds = action.targetStepIds.filter(id => id !== stepId);
                this.render();
                this.attachEvents();
                this.notifyChange();
            }
        }
    }

    /**
     * Creates advanced options toggle section
     * @private
     */
    createAdvancedToggle() {
        const section = document.createElement('div');
        section.className = 'advanced-toggle';
        const caseInsensitive = this.config.options?.caseInsensitive || false;
        const trimWhitespace = this.config.options?.trimWhitespace !== false;

        section.innerHTML = `
            <button type="button" class="advanced-toggle-btn" id="advanced-toggle-btn">
                <span class="toggle-icon">▶</span> Advanced Options
            </button>
            <div class="advanced-options" id="advanced-options" style="display: none;">
                <div class="advanced-option">
                    <label>
                        <input type="checkbox" id="case-insensitive-check"
                               ${caseInsensitive ? 'checked' : ''}>
                        Case-insensitive matching
                    </label>
                    <span class="option-help">Match "M", "m", and "Male" as the same value</span>
                </div>
                <div class="advanced-option">
                    <label>
                        <input type="checkbox" id="trim-values-check"
                               ${trimWhitespace ? 'checked' : ''}>
                        Trim whitespace before comparison
                    </label>
                    <span class="option-help">Remove leading/trailing spaces from field value</span>
                </div>
            </div>
        `;
        return section;
    }

    /**
     * Initializes field search component for switch field
     * @private
     */
    initializeFieldSearch() {
        const fieldInput = this.container.querySelector('#switch-field-input');
        if (fieldInput && typeof FieldPathSearchComponent !== 'undefined') {
            this.fieldSearchComponent = new FieldPathSearchComponent(fieldInput, {
                onSelect: (fieldPath) => {
                    this.config.field = fieldPath;
                    this.notifyChange();
                },
                placeholder: 'Search fields or variables... (e.g., PID.8, steps.empi.mrn)',
                allowCustom: true,
                showCategories: true,
                // Pass step variables for smart search
                getStepVariables: () => this.getStepVariablesForSearch()
            });
        }
    }

    /**
     * Initializes FieldPathSearchComponent for all action field containers
     * Finds all .field-search-container elements and creates search inputs
     * @private
     */
    initializeActionFieldSearchComponents() {
        // Track instances for cleanup
        if (!this.actionFieldSearchInstances) {
            this.actionFieldSearchInstances = [];
        }

        // Cleanup previous instances
        this.actionFieldSearchInstances.forEach(instance => {
            if (instance && typeof instance.destroy === 'function') {
                instance.destroy();
            }
        });
        this.actionFieldSearchInstances = [];

        // Find all field search containers in actions
        const containers = this.container.querySelectorAll('.field-search-container');

        containers.forEach(container => {
            const caseIndex = parseInt(container.dataset.caseIndex);
            const actionIndex = parseInt(container.dataset.actionIndex);
            const fieldName = container.dataset.field;
            const initialValue = container.dataset.initialValue || '';

            // Create input element
            const input = document.createElement('input');
            input.type = 'text';
            input.className = 'action-field-input field-search-input';
            input.placeholder = this.getPlaceholderForField(fieldName);
            input.value = initialValue;
            container.appendChild(input);

            // Initialize FieldPathSearchComponent if available
            if (typeof FieldPathSearchComponent !== 'undefined') {
                const fieldSearch = new FieldPathSearchComponent(input, {
                    onSelect: (fieldPath) => {
                        input.value = fieldPath;
                        this.updateActionField(caseIndex, actionIndex, fieldName, fieldPath);
                    },
                    placeholder: this.getPlaceholderForField(fieldName),
                    allowCustom: true,
                    showCategories: true,
                    getStepVariables: () => this.getStepVariablesForSearch()
                });

                this.actionFieldSearchInstances.push(fieldSearch);
            }

            // Add fallback change listener
            input.addEventListener('change', (e) => {
                this.updateActionField(caseIndex, actionIndex, fieldName, e.target.value);
            });
        });
    }

    /**
     * Gets placeholder text for a field name
     * @private
     */
    getPlaceholderForField(fieldName) {
        const placeholders = {
            sourceField: 'Source field (e.g., PID.8)',
            targetField: 'Target variable name',
            field: 'Field path'
        };
        return placeholders[fieldName] || 'Enter field path...';
    }

    /**
     * Updates an action field value
     * @private
     */
    updateActionField(caseIndex, actionIndex, fieldName, value) {
        const actions = caseIndex === -1 ?
            this.config.default.actions :
            this.config.cases[caseIndex]?.actions;

        if (actions && actions[actionIndex]) {
            actions[actionIndex][fieldName] = value;
            this.notifyChange();
        }
    }

    // ========================================
    // ACTION METHODS
    // ========================================

    /**
     * Adds a new case
     */
    addCase() {
        const newCase = {
            value: '',
            label: `Case ${this.config.cases.length + 1}`,
            actions: [{ action: 'continue' }]
        };
        this.config.cases.push(newCase);
        this.render();
        this.attachEvents();
        this.notifyChange();
    }

    /**
     * Deletes a case by index
     */
    async deleteCase(index) {
        if (this.config.cases.length <= 1) {
            this.showNotification('At least one case is required', 'warning');
            return;
        }

        const confirmed = await this.showConfirmDialog(
            'Delete this case and all its actions?',
            {
                title: 'Delete Case',
                confirmText: 'Delete',
                cancelText: 'Cancel',
                type: 'danger'
            }
        );

        if (!confirmed) return;

        this.config.cases.splice(index, 1);
        this.render();
        this.attachEvents();
        this.notifyChange();
    }

    /**
     * Show in-app notification
     */
    showNotification(message, type = 'info') {
        // Try to use builder's notification system first
        if (this.builder && this.builder.dragDropManager && this.builder.dragDropManager.showNotification) {
            this.builder.dragDropManager.showNotification(message, type);
            return;
        }

        // Fallback to simple toast
        const notification = document.createElement('div');
        notification.style.cssText = `
            position: fixed;
            bottom: 20px;
            right: 20px;
            background: ${type === 'success' ? '#10b981' : type === 'error' ? '#ef4444' : type === 'warning' ? '#f59e0b' : '#06b6d4'};
            color: white;
            padding: 12px 20px;
            border-radius: 6px;
            box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
            z-index: 10000;
        `;
        notification.textContent = message;
        document.body.appendChild(notification);
        setTimeout(() => notification.remove(), 3000);
    }

    /**
     * Show in-app confirm dialog
     */
    showConfirmDialog(message, options = {}) {
        // Try to use builder's confirm dialog
        if (this.builder && this.builder.dragDropManager && this.builder.dragDropManager.showConfirmDialog) {
            return this.builder.dragDropManager.showConfirmDialog(message, options);
        }

        // Fallback to Promise-wrapped native confirm
        return Promise.resolve(confirm(message));
    }

    /**
     * Updates action type
     */
    updateActionType(caseIndex, actionIndex, newType) {
        const actions = caseIndex === -1 ?
            this.config.default.actions :
            this.config.cases[caseIndex]?.actions;

        if (actions && actions[actionIndex]) {
            // Create new action object with defaults based on type
            const newAction = { action: newType };

            // Set default values for specific action types
            switch (newType) {
                case 'transform':
                    // Default transformation to uppercase (matches dropdown default)
                    newAction.transformation = 'uppercase';
                    newAction.sourceField = '';
                    newAction.targetField = '';
                    break;
                case 'set_value':
                    newAction.targetField = '';
                    newAction.value = '';
                    break;
                case 'copy_field':
                    newAction.sourceField = '';
                    newAction.targetField = '';
                    break;
                case 'route_to_step':
                    // Use array format for multiple target steps
                    newAction.targetStepIds = [];
                    break;
            }

            actions[actionIndex] = newAction;
            this.render();
            this.attachEvents();
            this.notifyChange();
        }
    }

    /**
     * Updates action config field
     */
    updateActionConfig(caseIndex, actionIndex, field, value) {
        const actions = caseIndex === -1 ?
            this.config.default.actions :
            this.config.cases[caseIndex]?.actions;

        if (actions && actions[actionIndex]) {
            actions[actionIndex][field] = value;
            this.notifyChange();
        }
    }

    /**
     * Adds an action to a case or default
     */
    addAction(caseIndex) {
        const actions = caseIndex === -1 ?
            this.config.default.actions :
            this.config.cases[caseIndex]?.actions;

        if (actions) {
            actions.push({ action: 'continue' });
            this.render();
            this.attachEvents();
            this.notifyChange();
        }
    }

    /**
     * Deletes an action
     */
    deleteAction(caseIndex, actionIndex) {
        const actions = caseIndex === -1 ?
            this.config.default.actions :
            this.config.cases[caseIndex]?.actions;

        if (actions && actions.length > 1) {
            actions.splice(actionIndex, 1);
            this.render();
            this.attachEvents();
            this.notifyChange();
        } else {
            this.showNotification('At least one action is required', 'warning');
        }
    }

    /**
     * Shows help modal
     */
    showHelp() {
        const modal = document.createElement('div');
        modal.className = 'switch-help-overlay';
        modal.innerHTML = `
            <div class="switch-help-content">
                <h3>🔀 Switch/Case Logic</h3>
                <p>The Switch/Case step evaluates a field's value and executes different actions based on matches.</p>

                <h4>How it works:</h4>
                <ol>
                    <li>The <strong>Switch Field</strong> is read from the message</li>
                    <li>Each <strong>Case</strong> compares the value to its match value</li>
                    <li>When a match is found, that case's actions execute</li>
                    <li>If no match, the <strong>Default</strong> actions execute</li>
                </ol>

                <h4>Available Actions:</h4>
                <ul>
                    <li><strong>▶️ Continue</strong> - Proceed to next step</li>
                    <li><strong>⏹️ Stop</strong> - Halt pipeline execution</li>
                    <li><strong>✏️ Set Value</strong> - Set a field to a specific value</li>
                    <li><strong>📋 Copy Field</strong> - Copy value from one field to another</li>
                    <li><strong>🔄 Transform</strong> - Apply text transformation</li>
                    <li><strong>🔀 Route to Step</strong> - Jump to a specific step</li>
                </ul>

                <h4>Example:</h4>
                <pre>
Switch on: PID.8 (Gender)
  Case "M" → Set patient.gender = "Male"
  Case "F" → Set patient.gender = "Female"
  Default → Set patient.gender = "Unknown"
                </pre>

                <h4>Tips:</h4>
                <ul>
                    <li>Use <strong>case-insensitive</strong> matching for status codes</li>
                    <li>Enable <strong>trim whitespace</strong> to handle inconsistent HL7 formatting</li>
                    <li>Always define a <strong>default case</strong> for unexpected values</li>
                </ul>

                <button class="switch-help-close">Close</button>
            </div>
        `;

        modal.querySelector('.switch-help-close').addEventListener('click', () => modal.remove());
        modal.addEventListener('click', (e) => {
            if (e.target === modal) modal.remove();
        });

        document.body.appendChild(modal);
    }

    /**
     * Notifies parent of config change
     * @private
     */
    notifyChange() {
        this.container.dispatchEvent(new CustomEvent('configChange', {
            detail: { config: this.getConfig() },
            bubbles: true
        }));
    }

    /**
     * HTML escaping utility
     * @private
     */
    escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// ========================================
// EXPORT
// ========================================

if (typeof window !== 'undefined') {
    window.SwitchCaseBuilder = SwitchCaseBuilder;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = SwitchCaseBuilder;
}

console.log('[SwitchCaseBuilder] Loaded - OOP builder extending BaseStepConfigBuilder');
