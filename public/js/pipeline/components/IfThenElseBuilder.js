/**
 * If-Then-Else Conditional Logic Builder - COMPACT REDESIGN
 * No-code UI with horizontal layout, smart field search, and minimal scrolling
 *
 * Design Principles:
 * - Horizontal flow (IF → THEN → ELSE in one row)
 * - Smart field search everywhere (FieldPathSearchComponent)
 * - Compact inline configuration (no modals)
 * - 4x less scrolling, 3x fewer clicks
 *
 * Color Theme:
 * - Primary: Navy Blue (#1e3a8a)
 * - Accent: Pastel Pink (#f8bbd9)
 * - Background: White (#ffffff)
 */

class IfThenElseBuilder {
    constructor(container, initialConfig = null) {
        this.container = container;
        this.fieldSearchInstances = []; // Track for cleanup
        this.cachedStepVariables = null; // Cache for step output variables

        // Initialize config with proper validation
        if (initialConfig && initialConfig.conditions && Array.isArray(initialConfig.conditions)) {
            this.config = initialConfig;
        } else {
            this.config = this.getDefaultConfig();
        }

        // Pre-fetch step variables for smart search
        this.loadStepVariables();

        this.render();
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
            const currentSequence = currentStep?.sequence || 0;

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
                // Transform to FieldPathSearchComponent format
                this.cachedStepVariables = this.transformVariablesToSearchFormat(data.variables || []);
                console.log('[IfThenElseBuilder] Loaded', this.cachedStepVariables.length, 'step variables for smart search');
            } else {
                this.cachedStepVariables = [];
            }
        } catch (err) {
            console.warn('[IfThenElseBuilder] Failed to load step variables:', err);
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
     * Called as a callback to provide dynamic variables
     */
    getStepVariablesForSearch() {
        return this.cachedStepVariables || [];
    }

    getDefaultConfig() {
        return {
            conditions: [
                {
                    name: 'Condition 1',
                    condition: {
                        field: '',
                        operator: 'equals',
                        value: '',
                        compareToField: ''  // For cross-field comparison
                    },
                    onTrue: {
                        action: 'continue'
                    },
                    onFalse: {
                        action: 'continue'
                    }
                }
            ]
        };
    }

    render() {
        this.container.innerHTML = '';
        this.container.className = 'if-then-else-builder';

        // Add styles
        this.addStyles();

        // Header with help
        const header = this.createHeader();
        this.container.appendChild(header);

        // Conditions list
        const conditionsList = this.createConditionsList();
        this.container.appendChild(conditionsList);

        // Add condition button
        const addButton = this.createAddConditionButton();
        this.container.appendChild(addButton);
    }

    createHeader() {
        const header = document.createElement('div');
        header.className = 'ifthen-header';
        header.innerHTML = `
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
                <div>
                    <h4 style="margin: 0; color: var(--primary-color); font-size: 1rem;">Conditional Logic</h4>
                    <p style="margin: 0.25rem 0 0 0; font-size: 0.75rem; color: #6b7280;">
                        Smart horizontal layout - less scrolling, fewer clicks
                    </p>
                </div>
                <button class="ifthen-help-btn" title="Show Examples">
                    <i class="fas fa-question-circle"></i>
                </button>
            </div>
        `;

        // Help button event
        const helpBtn = header.querySelector('.ifthen-help-btn');
        helpBtn.addEventListener('click', () => this.showHelp());

        return header;
    }

    createConditionsList() {
        const list = document.createElement('div');
        list.className = 'ifthen-conditions-list';

        this.config.conditions.forEach((condition, index) => {
            const conditionCard = this.createConditionCard(condition, index);
            list.appendChild(conditionCard);
        });

        return list;
    }

    createConditionCard(condition, index) {
        // Validate condition structure (defensive programming for legacy data)
        if (!condition.condition || typeof condition.condition !== 'object') {
            console.warn(`⚠️ Condition ${index} has invalid structure, applying defaults...`);
            condition.condition = {
                field: '',
                operator: 'equals',
                value: '',
                compareToField: ''
            };
        }

        // Ensure onTrue and onFalse exist (defensive programming for legacy data)
        if (!condition.onTrue || typeof condition.onTrue !== 'object') {
            console.warn(`⚠️ Condition ${index} missing onTrue action, applying default...`);
            condition.onTrue = { action: 'continue' };
        }
        if (!condition.onFalse || typeof condition.onFalse !== 'object') {
            console.warn(`⚠️ Condition ${index} missing onFalse action, applying default...`);
            condition.onFalse = { action: 'continue' };
        }

        const card = document.createElement('div');
        card.className = 'ifthen-condition-row';
        card.dataset.index = index;

        // Header with name and delete
        const headerDiv = document.createElement('div');
        headerDiv.className = 'ifthen-row-header';
        headerDiv.innerHTML = `
            <input type="text"
                   class="ifthen-condition-name-compact"
                   value="${condition.name || 'Condition ' + (index + 1)}"
                   placeholder="Condition Name"
                   data-index="${index}">
            <button class="ifthen-help-icon-btn" title="Examples">
                <i class="fas fa-question-circle"></i>
            </button>
            <button class="ifthen-delete-btn-compact" data-index="${index}" title="Delete Condition">
                <i class="fas fa-trash"></i>
            </button>
        `;
        card.appendChild(headerDiv);

        // Horizontal grid container
        const gridDiv = document.createElement('div');
        gridDiv.className = 'ifthen-grid-container';

        // IF Section
        const ifSection = document.createElement('div');
        ifSection.className = 'ifthen-if-section';
        ifSection.innerHTML = `
            <label class="ifthen-section-label-compact if-label">IF</label>
            <div class="field-search-container-compact" id="field-search-${index}"></div>
            <select class="ifthen-select-compact" data-field="operator" data-index="${index}">
                ${this.createOperatorOptions(condition.condition.operator)}
            </select>
            <div class="ifthen-value-section">
                <label class="ifthen-checkbox-compact">
                    <input type="checkbox" data-field="useFieldComparison" data-index="${index}"
                           ${condition.condition.compareToField ? 'checked' : ''}>
                    <span>Compare to field</span>
                </label>
                <div class="value-input-container" id="value-input-${index}"></div>
            </div>
        `;
        gridDiv.appendChild(ifSection);

        // Arrow separator
        const arrow1 = document.createElement('div');
        arrow1.className = 'ifthen-flow-arrow';
        arrow1.innerHTML = '→';
        gridDiv.appendChild(arrow1);

        // THEN Section
        const thenSection = document.createElement('div');
        thenSection.className = 'ifthen-then-section';
        thenSection.innerHTML = `
            <label class="ifthen-section-label-compact then-label">THEN (if true)</label>
            <select class="ifthen-select-compact" data-field="action" data-action-type="onTrue" data-index="${index}">
                ${this.createActionOptions(condition.onTrue.action)}
            </select>
            <div class="action-config-container" id="then-config-${index}"></div>
        `;
        gridDiv.appendChild(thenSection);

        // Arrow separator
        const arrow2 = document.createElement('div');
        arrow2.className = 'ifthen-flow-arrow';
        arrow2.innerHTML = '→';
        gridDiv.appendChild(arrow2);

        // ELSE Section
        const elseSection = document.createElement('div');
        elseSection.className = 'ifthen-else-section';
        elseSection.innerHTML = `
            <label class="ifthen-section-label-compact else-label">ELSE (if false)</label>
            <select class="ifthen-select-compact" data-field="action" data-action-type="onFalse" data-index="${index}">
                ${this.createActionOptions(condition.onFalse.action)}
            </select>
            <div class="action-config-container" id="else-config-${index}"></div>
        `;
        gridDiv.appendChild(elseSection);

        card.appendChild(gridDiv);

        // Initialize field searches after DOM insertion
        setTimeout(() => {
            this.initializeFieldSearch(`field-search-${index}`, condition.condition.field, (value) => {
                condition.condition.field = value;
            });

            // Initialize value input (plain or field search based on checkbox)
            this.updateValueInput(index, condition);

            // Initialize action configs
            this.updateActionConfig(index, 'onTrue', condition.onTrue);
            this.updateActionConfig(index, 'onFalse', condition.onFalse);
        }, 0);

        // Attach event listeners
        this.attachCardEvents(card, index);

        return card;
    }

    createOperatorOptions(selectedOperator) {
        const operators = [
            { value: 'equals', label: 'Equals (=)' },
            { value: 'not_equals', label: 'Not Equals (≠)' },
            { value: 'greater_than', label: 'Greater Than (>)' },
            { value: 'greater_than_or_equal', label: 'Greater Than or Equal (≥)' },
            { value: 'less_than', label: 'Less Than (<)' },
            { value: 'less_than_or_equal', label: 'Less Than or Equal (≤)' },
            { value: 'contains', label: 'Contains' },
            { value: 'not_contains', label: 'Does Not Contain' },
            { value: 'matches_regex', label: 'Matches Regex' },
            { value: 'is_empty', label: 'Is Empty' },
            { value: 'is_not_empty', label: 'Is Not Empty' }
        ];

        return operators.map(op => `
            <option value="${op.value}" ${selectedOperator === op.value ? 'selected' : ''}>
                ${op.label}
            </option>
        `).join('');
    }

    createActionOptions(selectedAction) {
        const actions = [
            { value: 'continue', label: 'Continue', icon: 'arrow-right', color: '#10b981' },
            { value: 'stop', label: 'Stop Processing', icon: 'ban', color: '#ef4444' },
            { value: 'set_value', label: 'Set Value', icon: 'edit', color: '#3b82f6' },
            { value: 'transform', label: 'Transform', icon: 'magic', color: '#8b5cf6' },
            { value: 'delete_field', label: 'Delete Field', icon: 'trash', color: '#ef4444' },
            { value: 'route_to_step', label: 'Route To Step', icon: 'directions', color: '#6366f1' }
        ];

        return actions.map(a => `
            <option value="${a.value}" ${selectedAction === a.value ? 'selected' : ''}>
                ${a.label}
            </option>
        `).join('');
    }

    initializeFieldSearch(containerId, initialValue, onChange) {
        const container = document.getElementById(containerId);
        if (!container) {
            console.warn(`[IfThenElseBuilder] Container not found: ${containerId}`);
            return;
        }

        // Create input element for field search
        const input = document.createElement('input');
        input.type = 'text';
        input.className = 'ifthen-input-compact field-search-input';
        input.value = initialValue || '';
        input.placeholder = 'Search fields or variables...';
        container.appendChild(input);

        // Initialize FieldPathSearchComponent if available
        if (typeof FieldPathSearchComponent !== 'undefined') {
            const fieldSearch = new FieldPathSearchComponent(input, {
                onSelect: (fieldPath) => {
                    input.value = fieldPath;
                    onChange(fieldPath);
                },
                placeholder: 'Search fields or variables... (e.g., PID.8, steps.empi.mrn)',
                allowCustom: true,
                showCategories: true,
                // NEW: Pass step variables for smart search
                getStepVariables: () => this.getStepVariablesForSearch()
            });

            this.fieldSearchInstances.push(fieldSearch);
        } else {
            // Fallback: regular input with change event
            input.addEventListener('input', (e) => {
                onChange(e.target.value);
            });
        }
    }

    updateValueInput(index, condition) {
        const container = document.getElementById(`value-input-${index}`);
        if (!container) return;

        container.innerHTML = ''; // Clear existing

        if (condition.condition.compareToField) {
            // Use field search for cross-field comparison
            this.initializeFieldSearch(`value-input-${index}`, condition.condition.compareToField, (value) => {
                condition.condition.compareToField = value;
                condition.condition.value = ''; // Clear value when using field comparison
            });
        } else {
            // Use plain input for value comparison
            const input = document.createElement('input');
            input.type = 'text';
            input.className = 'ifthen-input-compact';
            input.placeholder = 'Value to compare';
            input.value = condition.condition.value || '';
            input.addEventListener('input', (e) => {
                condition.condition.value = e.target.value;
                condition.condition.compareToField = ''; // Clear field when using value
            });
            container.appendChild(input);
        }
    }

    updateActionConfig(index, actionType, actionData) {
        const container = document.getElementById(`${actionType === 'onTrue' ? 'then' : 'else'}-config-${index}`);
        if (!container) return;

        container.innerHTML = ''; // Clear existing

        switch (actionData.action) {
            case 'continue':
                container.innerHTML = `
                    <select class="ifthen-select-compact" data-field="logLevel" data-action-type="${actionType}" data-index="${index}">
                        <option value="none" ${!actionData.logLevel || actionData.logLevel === 'none' ? 'selected' : ''}>No logging</option>
                        <option value="info" ${actionData.logLevel === 'info' ? 'selected' : ''}>Log Info</option>
                        <option value="warning" ${actionData.logLevel === 'warning' ? 'selected' : ''}>Log Warning</option>
                        <option value="debug" ${actionData.logLevel === 'debug' ? 'selected' : ''}>Log Debug</option>
                    </select>
                    <input type="text" class="ifthen-input-compact ${!actionData.logLevel || actionData.logLevel === 'none' ? 'ifthen-hidden' : ''}"
                           placeholder="Log message"
                           value="${actionData.message || ''}"
                           data-field="message" data-action-type="${actionType}" data-index="${index}"
                           id="continue-message-${actionType}-${index}">
                `;
                break;

            case 'stop':
                container.innerHTML = `
                    <input type="text" class="ifthen-input-compact" placeholder="Error message (required)"
                           value="${actionData.message || ''}"
                           data-field="message" data-action-type="${actionType}" data-index="${index}">
                    <select class="ifthen-select-compact" data-field="severity" data-action-type="${actionType}" data-index="${index}">
                        <option value="warning" ${actionData.severity === 'warning' ? 'selected' : ''}>Warning</option>
                        <option value="error" ${actionData.severity === 'error' ? 'selected' : ''}>Error</option>
                        <option value="fatal" ${actionData.severity === 'fatal' ? 'selected' : ''}>Fatal</option>
                    </select>
                `;
                break;

            case 'set_value':
                container.innerHTML = `
                    <div class="field-search-container-inline" id="set-value-target-${actionType}-${index}"></div>
                    <select class="ifthen-select-compact" data-field="valueType" data-action-type="${actionType}" data-index="${index}">
                        <option value="constant" ${!actionData.valueType || actionData.valueType === 'constant' ? 'selected' : ''}>Constant Value</option>
                        <option value="copy" ${actionData.valueType === 'copy' ? 'selected' : ''}>Copy From Field</option>
                    </select>
                    <div id="set-value-input-${actionType}-${index}"></div>
                `;

                setTimeout(() => {
                    this.initializeFieldSearch(`set-value-target-${actionType}-${index}`, actionData.targetField || '', (value) => {
                        actionData.targetField = value;
                    });

                    this.updateSetValueInput(index, actionType, actionData);
                }, 0);
                break;

            case 'transform':
                container.innerHTML = `
                    <div class="field-search-container-inline" id="transform-source-${actionType}-${index}"></div>
                    <select class="ifthen-select-compact" data-field="transformType" data-action-type="${actionType}" data-index="${index}">
                        <option value="uppercase" ${actionData.transformType === 'uppercase' ? 'selected' : ''}>UPPERCASE</option>
                        <option value="lowercase" ${actionData.transformType === 'lowercase' ? 'selected' : ''}>lowercase</option>
                        <option value="trim" ${actionData.transformType === 'trim' ? 'selected' : ''}>Trim whitespace</option>
                        <option value="concat" ${actionData.transformType === 'concat' ? 'selected' : ''}>Concatenate</option>
                        <option value="replace" ${actionData.transformType === 'replace' ? 'selected' : ''}>Replace text</option>
                        <option value="regex_replace" ${actionData.transformType === 'regex_replace' ? 'selected' : ''}>Regex replace</option>
                        <option value="substring" ${actionData.transformType === 'substring' ? 'selected' : ''}>Substring</option>
                        <option value="split" ${actionData.transformType === 'split' ? 'selected' : ''}>Split & select</option>
                        <option value="pad_left" ${actionData.transformType === 'pad_left' ? 'selected' : ''}>Pad left</option>
                        <option value="pad_right" ${actionData.transformType === 'pad_right' ? 'selected' : ''}>Pad right</option>
                    </select>
                    <div id="transform-options-${actionType}-${index}"></div>
                    <div class="field-search-container-inline" id="transform-target-${actionType}-${index}" style="margin-top: 0.25rem;"></div>
                `;

                setTimeout(() => {
                    // Initialize source field search
                    this.initializeFieldSearch(`transform-source-${actionType}-${index}`, actionData.sourceField || '', (value) => {
                        actionData.sourceField = value;
                    });

                    // Initialize target field search
                    this.initializeFieldSearch(`transform-target-${actionType}-${index}`, actionData.targetField || '', (value) => {
                        actionData.targetField = value;
                    });

                    // Initialize transform options UI
                    this.updateTransformOptions(index, actionType, actionData);
                }, 0);
                break;

            case 'delete_field':
                const deleteFieldDiv = document.createElement('div');
                deleteFieldDiv.className = 'field-search-container-inline';
                deleteFieldDiv.id = `delete-field-target-${actionType}-${index}`;
                container.appendChild(deleteFieldDiv);

                setTimeout(() => {
                    this.initializeFieldSearch(`delete-field-target-${actionType}-${index}`, actionData.targetField || '', (value) => {
                        actionData.targetField = value;
                    });
                }, 0);
                break;

            case 'route_to_step':
                container.innerHTML = `<div id="route-step-select-${actionType}-${index}"></div>`;

                setTimeout(async () => {
                    const steps = await this.loadPipelineSteps();
                    const selectContainer = document.getElementById(`route-step-select-${actionType}-${index}`);

                    if (!selectContainer) return;

                    if (steps.length === 0) {
                        selectContainer.innerHTML = `
                            <div class="ifthen-warning" style="padding: 0.5rem; background: #fef3c7; border: 1px solid #fbbf24; border-radius: 4px; font-size: 0.75rem; color: #92400e;">
                                <i class="fas fa-exclamation-triangle"></i>
                                No steps available to route to. Add more steps to pipeline first.
                            </div>
                        `;
                    } else {
                        // Initialize arrays if not exists
                        if (!actionData.skipSteps) {
                            actionData.skipSteps = [];
                        }
                        // Support both single stepId (legacy) and targetStepIds (new array format)
                        if (!actionData.targetStepIds) {
                            actionData.targetStepIds = actionData.stepId ? [actionData.stepId] : [];
                        }

                        const selectedStepIds = actionData.targetStepIds || [];

                        selectContainer.innerHTML = `
                            <div style="display: flex; flex-direction: column; gap: 0.5rem;">
                                <div>
                                    <label style="font-size: 0.7rem; color: #6b7280; margin-bottom: 0.25rem; display: block;">
                                        Route To Steps (execute in order):
                                        <i class="fas fa-info-circle" title="Select multiple steps to execute in sequence. Steps will run in the order selected."></i>
                                    </label>
                                    <select class="ifthen-select-compact multi-step-add-ite" data-action-type="${actionType}" data-index="${index}">
                                        <option value="">+ Add target step...</option>
                                        ${steps.map(step => `
                                            <option value="${step.value}">
                                                ${step.label}
                                            </option>
                                        `).join('')}
                                    </select>
                                    <div class="selected-steps-list-ite" id="selected-steps-${actionType}-${index}" style="display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; min-height: 24px;">
                                        ${this.renderSelectedStepsITE(selectedStepIds, steps, actionType, index)}
                                    </div>
                                    <div style="font-size: 0.65rem; color: #9ca3af; margin-top: 0.25rem;">
                                        Select multiple steps to execute in sequence
                                    </div>
                                </div>
                                <div>
                                    <label style="font-size: 0.7rem; color: #6b7280; margin-bottom: 0.25rem; display: block;">
                                        Skip Steps (exclusive branches):
                                        <i class="fas fa-info-circle" title="Select steps that should be SKIPPED when this route is taken. Use for exclusive true/false branches."></i>
                                    </label>
                                    <select class="ifthen-select-compact" data-field="skipSteps" data-action-type="${actionType}" data-index="${index}" multiple size="3" style="min-height: 60px;">
                                        ${steps.filter(s => !selectedStepIds.includes(s.value)).map(step => `
                                            <option value="${step.value}" ${actionData.skipSteps?.includes(step.value) ? 'selected' : ''}>
                                                ${step.label}
                                            </option>
                                        `).join('')}
                                    </select>
                                    <div style="font-size: 0.65rem; color: #9ca3af; margin-top: 0.25rem;">
                                        Ctrl+click to select multiple
                                    </div>
                                </div>
                            </div>
                        `;

                        // Attach multi-step add event
                        this.attachMultiStepEvents(actionType, index, actionData, steps);
                    }
                }, 0);
                break;

            default:
                // No additional config needed
                break;
        }
    }

    updateSetValueInput(index, actionType, actionData) {
        const container = document.getElementById(`set-value-input-${actionType}-${index}`);
        if (!container) return;

        container.innerHTML = '';

        if (actionData.valueType === 'copy') {
            const div = document.createElement('div');
            div.className = 'field-search-container-inline';
            div.id = `set-value-source-${actionType}-${index}`;
            container.appendChild(div);

            setTimeout(() => {
                this.initializeFieldSearch(`set-value-source-${actionType}-${index}`, actionData.sourceField || '', (value) => {
                    actionData.sourceField = value;
                });
            }, 0);
        } else {
            const input = document.createElement('input');
            input.type = 'text';
            input.className = 'ifthen-input-compact';
            input.placeholder = 'Value';
            input.value = actionData.value || '';
            input.dataset.field = 'value';
            input.dataset.actionType = actionType;
            input.dataset.index = index;
            container.appendChild(input);
        }
    }

    /**
     * Update transform action options based on transform type
     */
    updateTransformOptions(index, actionType, actionData) {
        const container = document.getElementById(`transform-options-${actionType}-${index}`);
        if (!container) return;

        container.innerHTML = '';
        const transformType = actionData.transformType || 'uppercase';

        // Build options UI based on transform type
        switch (transformType) {
            case 'uppercase':
            case 'lowercase':
            case 'trim':
                // No additional options needed
                container.innerHTML = `<span style="font-size: 0.7rem; color: #6b7280;">→ Target:</span>`;
                break;

            case 'concat':
                container.innerHTML = `
                    <input type="text" class="ifthen-input-compact"
                           placeholder="Append text (or leave empty for field)"
                           value="${actionData.appendText || ''}"
                           data-field="appendText" data-action-type="${actionType}" data-index="${index}">
                    <div class="field-search-container-inline" id="concat-append-field-${actionType}-${index}"></div>
                    <span style="font-size: 0.7rem; color: #6b7280;">→ Target:</span>
                `;
                setTimeout(() => {
                    this.initializeFieldSearch(`concat-append-field-${actionType}-${index}`, actionData.appendField || '', (value) => {
                        actionData.appendField = value;
                    });
                }, 0);
                break;

            case 'replace':
                container.innerHTML = `
                    <input type="text" class="ifthen-input-compact" placeholder="Find"
                           value="${actionData.findText || ''}"
                           data-field="findText" data-action-type="${actionType}" data-index="${index}">
                    <input type="text" class="ifthen-input-compact" placeholder="Replace with"
                           value="${actionData.replaceText || ''}"
                           data-field="replaceText" data-action-type="${actionType}" data-index="${index}">
                    <span style="font-size: 0.7rem; color: #6b7280;">→ Target:</span>
                `;
                break;

            case 'regex_replace':
                container.innerHTML = `
                    <input type="text" class="ifthen-input-compact" placeholder="Regex pattern"
                           value="${actionData.regexPattern || ''}"
                           data-field="regexPattern" data-action-type="${actionType}" data-index="${index}">
                    <input type="text" class="ifthen-input-compact" placeholder="Replace with ($1, $2 for groups)"
                           value="${actionData.replaceText || ''}"
                           data-field="replaceText" data-action-type="${actionType}" data-index="${index}">
                    <span style="font-size: 0.7rem; color: #6b7280;">→ Target:</span>
                `;
                break;

            case 'substring':
                container.innerHTML = `
                    <input type="number" class="ifthen-input-compact" placeholder="Start index"
                           value="${actionData.startIndex || 0}" style="width: 60px;"
                           data-field="startIndex" data-action-type="${actionType}" data-index="${index}">
                    <input type="number" class="ifthen-input-compact" placeholder="Length (optional)"
                           value="${actionData.length || ''}" style="width: 60px;"
                           data-field="length" data-action-type="${actionType}" data-index="${index}">
                    <span style="font-size: 0.7rem; color: #6b7280;">→ Target:</span>
                `;
                break;

            case 'split':
                container.innerHTML = `
                    <input type="text" class="ifthen-input-compact" placeholder="Delimiter"
                           value="${actionData.delimiter || ''}" style="width: 60px;"
                           data-field="delimiter" data-action-type="${actionType}" data-index="${index}">
                    <input type="number" class="ifthen-input-compact" placeholder="Index (0-based)"
                           value="${actionData.selectIndex || 0}" style="width: 60px;"
                           data-field="selectIndex" data-action-type="${actionType}" data-index="${index}">
                    <span style="font-size: 0.7rem; color: #6b7280;">→ Target:</span>
                `;
                break;

            case 'pad_left':
            case 'pad_right':
                container.innerHTML = `
                    <input type="number" class="ifthen-input-compact" placeholder="Total length"
                           value="${actionData.padLength || 10}" style="width: 60px;"
                           data-field="padLength" data-action-type="${actionType}" data-index="${index}">
                    <input type="text" class="ifthen-input-compact" placeholder="Pad char"
                           value="${actionData.padChar || '0'}" style="width: 50px;" maxlength="1"
                           data-field="padChar" data-action-type="${actionType}" data-index="${index}">
                    <span style="font-size: 0.7rem; color: #6b7280;">→ Target:</span>
                `;
                break;

            default:
                container.innerHTML = `<span style="font-size: 0.7rem; color: #6b7280;">→ Target:</span>`;
        }
    }

    async loadPipelineSteps() {
        try {
            const pipeline = window.pipelineBuilder?.getPipeline();
            if (!pipeline || !pipeline.steps) {
                return [];
            }

            const currentStep = this.step;
            const currentStepId = currentStep?.id;
            const currentSequence = currentStep?.sequence || 0;

            return pipeline.steps
                .filter(step => {
                    // Exclude current step itself
                    if (step.id === currentStepId) {
                        return false;
                    }
                    // Include all other steps (can route forward or backward)
                    return true;
                })
                .map(step => ({
                    value: step.id,
                    label: `Step ${step.sequence} - ${step.stepName}`,
                    sequence: step.sequence
                }))
                .sort((a, b) => a.sequence - b.sequence);
        } catch (err) {
            console.error('Failed to load pipeline steps:', err);
            return [];
        }
    }

    /**
     * Renders selected steps as chips for multi-step routing (IfThenElse version)
     */
    renderSelectedStepsITE(selectedStepIds, steps, actionType, index) {
        if (!selectedStepIds || selectedStepIds.length === 0) {
            return '<span style="color: #9ca3af; font-style: italic; font-size: 0.7rem;">No steps selected</span>';
        }

        const stepMap = new Map(steps.map(s => [s.value, s]));

        return selectedStepIds.map((stepId, idx) => {
            const step = stepMap.get(stepId);
            const stepLabel = step ? step.label : stepId;

            return `
                <div class="selected-step-chip-ite" data-step-id="${stepId}" style="
                    display: inline-flex;
                    align-items: center;
                    gap: 4px;
                    padding: 2px 8px;
                    background: #e0e7ff;
                    border: 1px solid #a5b4fc;
                    border-radius: 12px;
                    font-size: 0.7rem;
                ">
                    <span style="font-weight: bold; color: #4338ca;">${idx + 1}.</span>
                    <span>${this.escapeHtml(stepLabel)}</span>
                    <button type="button" class="remove-step-btn-ite"
                            data-action-type="${actionType}"
                            data-index="${index}"
                            data-step-id="${stepId}"
                            style="
                                background: none;
                                border: none;
                                cursor: pointer;
                                color: #6b7280;
                                font-size: 12px;
                                padding: 0 2px;
                                line-height: 1;
                            "
                            title="Remove step">×</button>
                </div>
            `;
        }).join('');
    }

    /**
     * Attaches event handlers for multi-step selection UI
     */
    attachMultiStepEvents(actionType, index, actionData, steps) {
        // Add step dropdown
        const addDropdown = document.querySelector(`.multi-step-add-ite[data-action-type="${actionType}"][data-index="${index}"]`);
        if (addDropdown) {
            addDropdown.addEventListener('change', (e) => {
                const stepId = e.target.value;
                if (stepId && !actionData.targetStepIds.includes(stepId)) {
                    actionData.targetStepIds.push(stepId);
                    // Re-render the selected steps list
                    const listContainer = document.getElementById(`selected-steps-${actionType}-${index}`);
                    if (listContainer) {
                        listContainer.innerHTML = this.renderSelectedStepsITE(actionData.targetStepIds, steps, actionType, index);
                        this.attachRemoveStepEvents(actionType, index, actionData, steps);
                    }
                    e.target.value = ''; // Reset dropdown
                }
            });
        }

        // Attach remove events for initially rendered steps
        this.attachRemoveStepEvents(actionType, index, actionData, steps);
    }

    /**
     * Attaches remove button events for multi-step chips
     */
    attachRemoveStepEvents(actionType, index, actionData, steps) {
        const listContainer = document.getElementById(`selected-steps-${actionType}-${index}`);
        if (!listContainer) return;

        listContainer.querySelectorAll('.remove-step-btn-ite').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const stepId = btn.dataset.stepId;
                actionData.targetStepIds = actionData.targetStepIds.filter(id => id !== stepId);
                listContainer.innerHTML = this.renderSelectedStepsITE(actionData.targetStepIds, steps, actionType, index);
                this.attachRemoveStepEvents(actionType, index, actionData, steps);
            });
        });
    }

    attachCardEvents(card, index) {
        // Condition name input
        const nameInput = card.querySelector('.ifthen-condition-name-compact');
        if (nameInput) {
            nameInput.addEventListener('input', (e) => {
                this.config.conditions[index].name = e.target.value;
            });
        }

        // Delete button
        const deleteBtn = card.querySelector('.ifthen-delete-btn-compact');
        if (deleteBtn) {
            deleteBtn.addEventListener('click', () => {
                this.deleteCondition(index);
            });
        }

        // Help icon
        const helpBtn = card.querySelector('.ifthen-help-icon-btn');
        if (helpBtn) {
            helpBtn.addEventListener('click', () => this.showHelp());
        }

        // Operator select
        const operatorSelect = card.querySelector('select[data-field="operator"]');
        if (operatorSelect) {
            operatorSelect.addEventListener('change', (e) => {
                this.config.conditions[index].condition.operator = e.target.value;
            });
        }

        // Field comparison checkbox
        const fieldCompCheckbox = card.querySelector('input[data-field="useFieldComparison"]');
        if (fieldCompCheckbox) {
            fieldCompCheckbox.addEventListener('change', (e) => {
                if (e.target.checked) {
                    this.config.conditions[index].condition.compareToField = '';
                    this.config.conditions[index].condition.value = '';
                } else {
                    this.config.conditions[index].condition.compareToField = '';
                }
                this.updateValueInput(index, this.config.conditions[index]);
            });
        }

        // Action selects (THEN and ELSE)
        const actionSelects = card.querySelectorAll('select[data-field="action"]');
        actionSelects.forEach(select => {
            select.addEventListener('change', (e) => {
                const actionType = select.dataset.actionType;
                const newAction = e.target.value;
                this.config.conditions[index][actionType].action = newAction;
                this.updateActionConfig(index, actionType, this.config.conditions[index][actionType]);
            });
        });

        // Action config inputs (delegate to container)
        card.addEventListener('input', (e) => {
            const target = e.target;
            if (target.dataset.actionType && target.dataset.field) {
                const actionType = target.dataset.actionType;
                const field = target.dataset.field;
                const actionData = this.config.conditions[index][actionType];

                if (field === 'metadata') {
                    try {
                        actionData[field] = JSON.parse(target.value);
                    } catch (err) {
                        // Invalid JSON, keep as string for now
                        actionData[field] = target.value;
                    }
                } else {
                    actionData[field] = target.value;
                }
            }
        });

        // Action config selects (delegate to container)
        card.addEventListener('change', (e) => {
            const target = e.target;
            if (target.dataset.actionType && target.dataset.field) {
                const actionType = target.dataset.actionType;
                const field = target.dataset.field;
                const actionData = this.config.conditions[index][actionType];

                // Handle multi-select for skipSteps
                if (field === 'skipSteps' && target.multiple) {
                    const selectedValues = Array.from(target.selectedOptions).map(opt => opt.value);
                    actionData[field] = selectedValues;
                    console.log(`[IfThenElse] Updated skipSteps for ${actionType}:`, selectedValues);
                } else {
                    actionData[field] = target.value;
                }

                // Handle special cases that need UI updates
                if (field === 'valueType') {
                    this.updateSetValueInput(index, actionType, actionData);
                } else if (field === 'transformType') {
                    this.updateTransformOptions(index, actionType, actionData);
                } else if (field === 'logLevel') {
                    // Show/hide message input based on log level
                    const messageInput = document.getElementById(`continue-message-${actionType}-${index}`);
                    if (messageInput) {
                        if (target.value === 'none') {
                            messageInput.classList.add('ifthen-hidden');
                        } else {
                            messageInput.classList.remove('ifthen-hidden');
                        }
                    }
                }
            }
        });
    }

    createAddConditionButton() {
        const button = document.createElement('button');
        button.className = 'ifthen-add-btn-compact';
        button.innerHTML = '<i class="fas fa-plus"></i> Add Another Condition';
        button.addEventListener('click', () => this.addCondition());
        return button;
    }

    addCondition() {
        const newCondition = {
            name: `Condition ${this.config.conditions.length + 1}`,
            condition: {
                field: '',
                operator: 'equals',
                value: '',
                compareToField: ''
            },
            onTrue: { action: 'continue' },
            onFalse: { action: 'continue' }
        };

        this.config.conditions.push(newCondition);
        this.render();
    }

    async deleteCondition(index) {
        if (this.config.conditions.length === 1) {
            this.showNotification('At least one condition is required', 'warning');
            return;
        }

        const confirmed = await this.showConfirmDialog(
            'Delete this condition?',
            {
                title: 'Delete Condition',
                confirmText: 'Delete',
                cancelText: 'Cancel',
                type: 'danger'
            }
        );

        if (confirmed) {
            this.config.conditions.splice(index, 1);
            this.render();
        }
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

    showHelp() {
        const helpModal = document.createElement('div');
        helpModal.className = 'ifthen-help-modal';
        helpModal.innerHTML = `
            <div class="ifthen-help-content">
                <div class="ifthen-help-header">
                    <h3>Conditional Logic Examples</h3>
                    <button class="ifthen-help-close-btn">&times;</button>
                </div>
                <div class="ifthen-help-body">
                    <h4>Example 1: Age-Based Routing</h4>
                    <p><strong>IF</strong> patient.age > 65<br>
                    <strong>THEN</strong> Set Metadata: {"priority": "high", "routing": "geriatrics"}<br>
                    <strong>ELSE</strong> Continue</p>

                    <h4>Example 2: Cross-Field Validation</h4>
                    <p><strong>IF</strong> PV1.45 (discharge date) ≤ PV1.44 (admit date)<br>
                    <strong>THEN</strong> Reject: "Discharge date must be after admit date"<br>
                    <strong>ELSE</strong> Continue</p>

                    <h4>Example 3: Gender Code Normalization</h4>
                    <p><strong>IF</strong> PID.8 equals "M"<br>
                    <strong>THEN</strong> Set Field: patient.gender = "male"<br>
                    <strong>ELSE</strong> Set Field: patient.gender = "female"</p>

                    <h4>Example 4: VIP Flagging</h4>
                    <p><strong>IF</strong> patient.notes contains "VIP"<br>
                    <strong>THEN</strong> Set Metadata: {"vip": true, "priority": "high"}<br>
                    <strong>ELSE</strong> Continue</p>

                    <h4>Example 5: Data Quality Check</h4>
                    <p><strong>IF</strong> PID.3 (patient MRN) is_empty<br>
                    <strong>THEN</strong> Reject: "Patient MRN is required"<br>
                    <strong>ELSE</strong> Continue</p>

                    <h4>Example 6: Message Type Routing</h4>
                    <p><strong>IF</strong> message.type equals "ADT^A01"<br>
                    <strong>THEN</strong> Route To: "admission-system"<br>
                    <strong>ELSE</strong> Route To: "default-system"</p>
                </div>
            </div>
        `;

        document.body.appendChild(helpModal);

        const closeBtn = helpModal.querySelector('.ifthen-help-close-btn');
        closeBtn.addEventListener('click', () => {
            document.body.removeChild(helpModal);
        });

        helpModal.addEventListener('click', (e) => {
            if (e.target === helpModal) {
                document.body.removeChild(helpModal);
            }
        });
    }

    getConfig() {
        return this.config;
    }

    destroy() {
        // Clean up field search instances
        this.fieldSearchInstances.forEach(instance => {
            if (instance.destroy) instance.destroy();
        });
        this.fieldSearchInstances = [];
    }

    addStyles() {
        // Check if styles already injected
        if (document.getElementById('ifthen-builder-styles')) {
            return;
        }

        const style = document.createElement('style');
        style.id = 'ifthen-builder-styles';
        style.textContent = `
            /* If-Then-Else Builder - Compact Horizontal Layout */
            .if-then-else-builder {
                font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            }

            /* Header */
            .ifthen-header {
                margin-bottom: 1rem;
            }

            .ifthen-help-btn {
                background: var(--primary-color, #1e3a8a);
                color: white;
                border: none;
                border-radius: 4px;
                padding: 0.5rem 1rem;
                cursor: pointer;
                font-size: 0.875rem;
                transition: background 0.2s;
            }

            .ifthen-help-btn:hover {
                background: var(--primary-hover, #1e40af);
            }

            /* Conditions List */
            .ifthen-conditions-list {
                display: flex;
                flex-direction: column;
                gap: 1rem;
            }

            /* Compact Condition Row (Horizontal Grid) */
            .ifthen-condition-row {
                border: 2px solid var(--accent-pink, #f8bbd9);
                border-radius: 8px;
                padding: 1rem;
                background: #fefcfd;
            }

            .ifthen-row-header {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                margin-bottom: 0.75rem;
                padding-bottom: 0.5rem;
                border-bottom: 1px solid #e5e7eb;
            }

            .ifthen-condition-name-compact {
                flex: 1;
                padding: 0.5rem;
                border: 1px solid #d1d5db;
                border-radius: 4px;
                font-size: 0.875rem;
                font-weight: 600;
                color: var(--primary-color, #1e3a8a);
            }

            .ifthen-help-icon-btn {
                background: transparent;
                border: 1px solid #d1d5db;
                border-radius: 4px;
                padding: 0.5rem;
                cursor: pointer;
                color: #6b7280;
                font-size: 0.875rem;
            }

            .ifthen-help-icon-btn:hover {
                background: #f3f4f6;
                color: var(--primary-color, #1e3a8a);
            }

            .ifthen-delete-btn-compact {
                background: transparent;
                border: 1px solid #fca5a5;
                border-radius: 4px;
                padding: 0.5rem;
                cursor: pointer;
                color: #dc2626;
                font-size: 0.875rem;
            }

            .ifthen-delete-btn-compact:hover {
                background: #fef2f2;
                border-color: #dc2626;
            }

            /* Horizontal Grid Container */
            .ifthen-grid-container {
                display: grid;
                grid-template-columns: 2fr auto 2fr auto 2fr;
                gap: 0.75rem;
                align-items: start;
            }

            /* Sections */
            .ifthen-if-section,
            .ifthen-then-section,
            .ifthen-else-section {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
            }

            .ifthen-section-label-compact {
                font-weight: 600;
                font-size: 0.75rem;
                text-transform: uppercase;
                letter-spacing: 0.05em;
            }

            .if-label {
                color: var(--primary-color, #1e3a8a);
            }

            .then-label {
                color: #10b981;
            }

            .else-label {
                color: #ef4444;
            }

            /* Flow Arrows */
            .ifthen-flow-arrow {
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 1.5rem;
                color: var(--accent-pink, #f8bbd9);
                font-weight: bold;
                padding-top: 1.5rem;
            }

            /* Compact Inputs */
            .ifthen-input-compact,
            .ifthen-select-compact,
            .ifthen-textarea-compact {
                padding: 0.5rem;
                font-size: 0.875rem;
                border: 1px solid #d1d5db;
                border-radius: 4px;
                width: 100%;
                font-family: inherit;
            }

            .ifthen-input-compact:focus,
            .ifthen-select-compact:focus,
            .ifthen-textarea-compact:focus {
                outline: none;
                border-color: var(--primary-color, #1e3a8a);
                box-shadow: 0 0 0 3px rgba(30, 58, 138, 0.1);
            }

            .ifthen-textarea-compact {
                font-family: 'Courier New', monospace;
                resize: vertical;
                min-height: 60px;
            }

            /* Field Search Containers */
            .field-search-container-compact,
            .field-search-container-inline {
                position: relative;
                width: 100%;
            }

            .field-search-input {
                width: 100%;
            }

            /* Value Section */
            .ifthen-value-section {
                display: flex;
                flex-direction: column;
                gap: 0.25rem;
            }

            .ifthen-checkbox-compact {
                display: flex;
                align-items: center;
                gap: 0.25rem;
                font-size: 0.75rem;
                color: #6b7280;
                cursor: pointer;
            }

            .ifthen-checkbox-compact input[type="checkbox"] {
                cursor: pointer;
            }

            .ifthen-checkbox-compact span {
                user-select: none;
            }

            /* Action Config Containers */
            .action-config-container {
                display: flex;
                flex-direction: column;
                gap: 0.25rem;
            }

            .ifthen-inline-config {
                display: flex;
                flex-direction: column;
                gap: 0.25rem;
            }

            /* Hidden class */
            .ifthen-hidden {
                display: none !important;
            }

            /* Add Button */
            .ifthen-add-btn-compact {
                background: var(--primary-color, #1e3a8a);
                color: white;
                border: none;
                border-radius: 6px;
                padding: 0.75rem 1.5rem;
                font-size: 0.875rem;
                font-weight: 600;
                cursor: pointer;
                display: flex;
                align-items: center;
                gap: 0.5rem;
                margin-top: 0.5rem;
                transition: background 0.2s;
            }

            .ifthen-add-btn-compact:hover {
                background: var(--primary-hover, #1e40af);
            }

            /* Help Modal */
            .ifthen-help-modal {
                position: fixed;
                top: 0;
                left: 0;
                width: 100%;
                height: 100%;
                background: rgba(0, 0, 0, 0.5);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 10000;
            }

            .ifthen-help-content {
                background: white;
                border-radius: 8px;
                max-width: 600px;
                max-height: 80vh;
                overflow-y: auto;
                box-shadow: 0 10px 40px rgba(0, 0, 0, 0.2);
            }

            .ifthen-help-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1.5rem;
                border-bottom: 1px solid #e5e7eb;
            }

            .ifthen-help-header h3 {
                margin: 0;
                color: var(--primary-color, #1e3a8a);
            }

            .ifthen-help-close-btn {
                background: transparent;
                border: none;
                font-size: 2rem;
                cursor: pointer;
                color: #6b7280;
                padding: 0;
                width: 2rem;
                height: 2rem;
                display: flex;
                align-items: center;
                justify-content: center;
            }

            .ifthen-help-close-btn:hover {
                color: #111827;
            }

            .ifthen-help-body {
                padding: 1.5rem;
            }

            .ifthen-help-body h4 {
                color: var(--primary-color, #1e3a8a);
                margin-top: 1.5rem;
                margin-bottom: 0.5rem;
            }

            .ifthen-help-body h4:first-child {
                margin-top: 0;
            }

            .ifthen-help-body p {
                margin: 0.5rem 0;
                line-height: 1.6;
                color: #374151;
            }

            /* Responsive Design */
            @media (max-width: 1200px) {
                .ifthen-grid-container {
                    grid-template-columns: 1fr;
                    gap: 1rem;
                }

                .ifthen-flow-arrow {
                    display: none;
                }

                .ifthen-section-label-compact {
                    font-size: 0.875rem;
                }
            }
        `;

        document.head.appendChild(style);
    }
}
