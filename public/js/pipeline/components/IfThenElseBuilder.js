/**
 * If-Then-Else Conditional Logic Builder
 * No-code UI for building conditional logic with visual condition and action builders
 *
 * Color Theme:
 * - Primary: Navy Blue (#1e3a8a)
 * - Accent: Pastel Pink (#f8bbd9)
 * - Background: White (#ffffff)
 */

class IfThenElseBuilder {
    constructor(container, initialConfig = null) {
        this.container = container;
        this.config = initialConfig || this.getDefaultConfig();
        this.render();
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
                        Define conditions and actions (if this, then that)
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
        const card = document.createElement('div');
        card.className = 'ifthen-condition-card';
        card.innerHTML = `
            <div class="ifthen-condition-header">
                <input type="text"
                       class="ifthen-condition-name"
                       value="${condition.name}"
                       placeholder="Condition Name"
                       data-index="${index}">
                <button class="ifthen-delete-btn" data-index="${index}" title="Delete Condition">
                    <i class="fas fa-trash"></i>
                </button>
            </div>

            <div class="ifthen-section">
                <div class="ifthen-section-label">
                    <i class="fas fa-filter"></i> IF Condition
                </div>
                ${this.createConditionBuilder(condition.condition, index)}
            </div>

            <div class="ifthen-section">
                <div class="ifthen-section-label">
                    <i class="fas fa-check-circle" style="color: #10b981;"></i> THEN Action (if true)
                </div>
                ${this.createActionBuilder(condition.onTrue, index, 'onTrue')}
            </div>

            <div class="ifthen-section">
                <div class="ifthen-section-label">
                    <i class="fas fa-times-circle" style="color: #ef4444;"></i> ELSE Action (if false)
                </div>
                ${this.createActionBuilder(condition.onFalse, index, 'onFalse')}
            </div>
        `;

        // Attach event listeners
        this.attachCardEvents(card, index);

        return card;
    }

    createConditionBuilder(condition, conditionIndex) {
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

        return `
            <div class="ifthen-condition-builder">
                <div class="ifthen-form-group">
                    <label>Field</label>
                    <input type="text"
                           class="ifthen-input"
                           placeholder="e.g., PID.8 or patient.gender"
                           value="${condition.field || ''}"
                           data-field="field"
                           data-index="${conditionIndex}">
                    <small style="color: #6b7280; font-size: 0.75rem;">
                        Use HL7 path (PID.8) or JSON path (patient.gender)
                    </small>
                </div>

                <div class="ifthen-form-group">
                    <label>Operator</label>
                    <select class="ifthen-select" data-field="operator" data-index="${conditionIndex}">
                        ${operators.map(op => `
                            <option value="${op.value}" ${condition.operator === op.value ? 'selected' : ''}>
                                ${op.label}
                            </option>
                        `).join('')}
                    </select>
                </div>

                <div class="ifthen-form-group">
                    <label>
                        <input type="checkbox"
                               class="ifthen-checkbox"
                               ${condition.compareToField ? 'checked' : ''}
                               data-field="useFieldComparison"
                               data-index="${conditionIndex}">
                        Compare to another field (cross-field validation)
                    </label>
                </div>

                <div class="ifthen-form-group ${condition.compareToField ? '' : 'ifthen-hidden'}" data-type="field-comparison">
                    <label>Compare To Field</label>
                    <input type="text"
                           class="ifthen-input"
                           placeholder="e.g., PV1.44 (field to compare with)"
                           value="${condition.compareToField || ''}"
                           data-field="compareToField"
                           data-index="${conditionIndex}">
                    <small style="color: #6b7280; font-size: 0.75rem;">
                        Example: Discharge date (PV1.45) > Admit date (PV1.44)
                    </small>
                </div>

                <div class="ifthen-form-group ${condition.compareToField ? 'ifthen-hidden' : ''}" data-type="value-comparison">
                    <label>Value</label>
                    <input type="text"
                           class="ifthen-input"
                           placeholder="Value to compare"
                           value="${condition.value || ''}"
                           data-field="value"
                           data-index="${conditionIndex}">
                    <small style="color: #6b7280; font-size: 0.75rem;">
                        Example: 'M', '65', 'Emergency'
                    </small>
                </div>
            </div>
        `;
    }

    createActionBuilder(action, conditionIndex, actionType) {
        const actions = [
            { value: 'continue', label: 'Continue Processing', icon: 'arrow-right', color: '#10b981' },
            { value: 'reject', label: 'Reject Message', icon: 'ban', color: '#ef4444' },
            { value: 'log_warning', label: 'Log Warning', icon: 'exclamation-triangle', color: '#f59e0b' },
            { value: 'log_error', label: 'Log Error', icon: 'exclamation-circle', color: '#ef4444' },
            { value: 'set_metadata', label: 'Set Metadata', icon: 'tags', color: '#8b5cf6' },
            { value: 'set_field', label: 'Set Field Value', icon: 'edit', color: '#3b82f6' },
            { value: 'copy_field', label: 'Copy Field', icon: 'copy', color: '#06b6d4' },
            { value: 'delete_field', label: 'Delete Field', icon: 'trash', color: '#ef4444' },
            { value: 'route_to', label: 'Route To Destination', icon: 'directions', color: '#8b5cf6' }
        ];

        const selectedAction = actions.find(a => a.value === action.action) || actions[0];

        return `
            <div class="ifthen-action-builder">
                <div class="ifthen-form-group">
                    <label>Action</label>
                    <select class="ifthen-select" data-field="action" data-action-type="${actionType}" data-index="${conditionIndex}">
                        ${actions.map(a => `
                            <option value="${a.value}" ${action.action === a.value ? 'selected' : ''}>
                                ${a.label}
                            </option>
                        `).join('')}
                    </select>
                </div>

                ${this.createActionConfigFields(action, conditionIndex, actionType)}
            </div>
        `;
    }

    createActionConfigFields(action, conditionIndex, actionType) {
        const actionType_value = action.action;

        switch (actionType_value) {
            case 'reject':
                return `
                    <div class="ifthen-form-group">
                        <label>Error Message</label>
                        <input type="text"
                               class="ifthen-input"
                               placeholder="Message to log when rejecting"
                               value="${action.errorMessage || ''}"
                               data-field="errorMessage"
                               data-action-type="${actionType}"
                               data-index="${conditionIndex}">
                    </div>
                    <div class="ifthen-form-group">
                        <label>Severity</label>
                        <select class="ifthen-select" data-field="severity" data-action-type="${actionType}" data-index="${conditionIndex}">
                            <option value="error" ${action.severity === 'error' ? 'selected' : ''}>Error</option>
                            <option value="warning" ${action.severity === 'warning' ? 'selected' : ''}>Warning</option>
                        </select>
                    </div>
                `;

            case 'log_warning':
            case 'log_error':
                return `
                    <div class="ifthen-form-group">
                        <label>Message</label>
                        <input type="text"
                               class="ifthen-input"
                               placeholder="Log message"
                               value="${action.message || ''}"
                               data-field="message"
                               data-action-type="${actionType}"
                               data-index="${conditionIndex}">
                    </div>
                    <div class="ifthen-form-group">
                        <label>
                            <input type="checkbox"
                                   class="ifthen-checkbox"
                                   ${action.continue !== false ? 'checked' : ''}
                                   data-field="continue"
                                   data-action-type="${actionType}"
                                   data-index="${conditionIndex}">
                            Continue processing after logging
                        </label>
                    </div>
                `;

            case 'set_metadata':
                return `
                    <div class="ifthen-form-group">
                        <label>Metadata (JSON)</label>
                        <textarea class="ifthen-textarea"
                                  placeholder='{"priority": "high", "routing": "geriatrics"}'
                                  rows="3"
                                  data-field="metadata"
                                  data-action-type="${actionType}"
                                  data-index="${conditionIndex}">${action.metadata ? JSON.stringify(action.metadata, null, 2) : ''}</textarea>
                        <small style="color: #6b7280; font-size: 0.75rem;">
                            JSON object with metadata key-value pairs
                        </small>
                    </div>
                `;

            case 'set_field':
                return `
                    <div class="ifthen-form-group">
                        <label>Field</label>
                        <input type="text"
                               class="ifthen-input"
                               placeholder="Field to set (e.g., patient.priority)"
                               value="${action.field || ''}"
                               data-field="field"
                               data-action-type="${actionType}"
                               data-index="${conditionIndex}">
                    </div>
                    <div class="ifthen-form-group">
                        <label>Value</label>
                        <input type="text"
                               class="ifthen-input"
                               placeholder="Value to set"
                               value="${action.value || ''}"
                               data-field="value"
                               data-action-type="${actionType}"
                               data-index="${conditionIndex}">
                    </div>
                `;

            case 'copy_field':
                return `
                    <div class="ifthen-form-group">
                        <label>Source Field</label>
                        <input type="text"
                               class="ifthen-input"
                               placeholder="Field to copy from"
                               value="${action.sourceField || ''}"
                               data-field="sourceField"
                               data-action-type="${actionType}"
                               data-index="${conditionIndex}">
                    </div>
                    <div class="ifthen-form-group">
                        <label>Target Field</label>
                        <input type="text"
                               class="ifthen-input"
                               placeholder="Field to copy to"
                               value="${action.targetField || ''}"
                               data-field="targetField"
                               data-action-type="${actionType}"
                               data-index="${conditionIndex}">
                    </div>
                `;

            case 'delete_field':
                return `
                    <div class="ifthen-form-group">
                        <label>Field</label>
                        <input type="text"
                               class="ifthen-input"
                               placeholder="Field to delete"
                               value="${action.field || ''}"
                               data-field="field"
                               data-action-type="${actionType}"
                               data-index="${conditionIndex}">
                    </div>
                `;

            case 'route_to':
                return `
                    <div class="ifthen-form-group">
                        <label>Destination</label>
                        <input type="text"
                               class="ifthen-input"
                               placeholder="Routing destination"
                               value="${action.destination || ''}"
                               data-field="destination"
                               data-action-type="${actionType}"
                               data-index="${conditionIndex}">
                    </div>
                    <div class="ifthen-form-group">
                        <label>Queue (optional)</label>
                        <input type="text"
                               class="ifthen-input"
                               placeholder="Queue name"
                               value="${action.queue || ''}"
                               data-field="queue"
                               data-action-type="${actionType}"
                               data-index="${conditionIndex}">
                    </div>
                `;

            case 'continue':
            default:
                return `<small style="color: #6b7280; font-size: 0.75rem; display: block; margin-top: 0.5rem;">No additional configuration needed</small>`;
        }
    }

    createAddConditionButton() {
        const button = document.createElement('button');
        button.className = 'ifthen-add-btn';
        button.innerHTML = `
            <i class="fas fa-plus-circle"></i>
            Add Another Condition
        `;
        button.addEventListener('click', () => this.addCondition());
        return button;
    }

    attachCardEvents(card, index) {
        // Delete button
        const deleteBtn = card.querySelector('.ifthen-delete-btn');
        deleteBtn.addEventListener('click', () => this.deleteCondition(index));

        // Condition name
        const nameInput = card.querySelector('.ifthen-condition-name');
        nameInput.addEventListener('input', (e) => {
            this.config.conditions[index].name = e.target.value;
        });

        // Field comparison checkbox
        const fieldComparisonCheckbox = card.querySelector('[data-field="useFieldComparison"]');
        fieldComparisonCheckbox.addEventListener('change', (e) => {
            const fieldComparison = card.querySelector('[data-type="field-comparison"]');
            const valueComparison = card.querySelector('[data-type="value-comparison"]');

            if (e.target.checked) {
                fieldComparison.classList.remove('ifthen-hidden');
                valueComparison.classList.add('ifthen-hidden');
            } else {
                fieldComparison.classList.add('ifthen-hidden');
                valueComparison.classList.remove('ifthen-hidden');
                this.config.conditions[index].condition.compareToField = '';
            }
        });

        // All condition inputs
        const conditionInputs = card.querySelectorAll('[data-field][data-index="' + index + '"]');
        conditionInputs.forEach(input => {
            input.addEventListener('input', (e) => this.updateConditionField(e, index));
            input.addEventListener('change', (e) => this.updateConditionField(e, index));
        });

        // All action selects (need to rebuild config fields on change)
        const actionSelects = card.querySelectorAll('[data-field="action"]');
        actionSelects.forEach(select => {
            select.addEventListener('change', (e) => {
                this.updateActionField(e, index);
                // Rebuild the entire card to show new config fields
                this.render();
            });
        });
    }

    updateConditionField(event, index) {
        const field = event.target.dataset.field;
        const value = event.target.type === 'checkbox' ? event.target.checked : event.target.value;

        this.config.conditions[index].condition[field] = value;
    }

    updateActionField(event, index) {
        const field = event.target.dataset.field;
        const actionType = event.target.dataset.actionType;
        let value = event.target.value;

        // Handle JSON parsing for metadata
        if (field === 'metadata') {
            try {
                value = JSON.parse(value);
            } catch (e) {
                // Keep as string if invalid JSON
            }
        }

        // Handle checkbox
        if (event.target.type === 'checkbox') {
            value = event.target.checked;
        }

        this.config.conditions[index][actionType][field] = value;
    }

    addCondition() {
        this.config.conditions.push({
            name: `Condition ${this.config.conditions.length + 1}`,
            condition: {
                field: '',
                operator: 'equals',
                value: '',
                compareToField: ''
            },
            onTrue: {
                action: 'continue'
            },
            onFalse: {
                action: 'continue'
            }
        });
        this.render();
    }

    deleteCondition(index) {
        if (this.config.conditions.length === 1) {
            alert('Cannot delete the last condition');
            return;
        }
        this.config.conditions.splice(index, 1);
        this.render();
    }

    showHelp() {
        const modal = document.createElement('div');
        modal.className = 'ifthen-help-modal';
        modal.innerHTML = `
            <div class="ifthen-help-content">
                <div class="ifthen-help-header">
                    <h3>If-Then-Else Examples</h3>
                    <button class="ifthen-help-close">&times;</button>
                </div>
                <div class="ifthen-help-body">
                    ${this.getHelpContent()}
                </div>
            </div>
        `;

        document.body.appendChild(modal);

        const closeBtn = modal.querySelector('.ifthen-help-close');
        closeBtn.addEventListener('click', () => modal.remove());
        modal.addEventListener('click', (e) => {
            if (e.target === modal) modal.remove();
        });
    }

    getHelpContent() {
        return `
            <div class="ifthen-example">
                <h4>Example 1: Age-Based Routing</h4>
                <p><strong>IF</strong> patient.age > 65</p>
                <p><strong>THEN</strong> Set metadata: {"priority": "high", "routing": "geriatrics"}</p>
                <p><strong>ELSE</strong> Continue</p>
            </div>

            <div class="ifthen-example">
                <h4>Example 2: Cross-Field Validation</h4>
                <p><strong>IF</strong> PV1.45 (discharge) <= PV1.44 (admit)</p>
                <p><strong>THEN</strong> Reject: "Discharge date must be after admit date"</p>
                <p><strong>ELSE</strong> Continue</p>
            </div>

            <div class="ifthen-example">
                <h4>Example 3: Data Quality Check</h4>
                <p><strong>IF</strong> PID.3.1 != PV1.5.1 (Patient ID mismatch)</p>
                <p><strong>THEN</strong> Log warning: "Patient ID inconsistent between PID and PV1"</p>
                <p><strong>ELSE</strong> Continue</p>
            </div>

            <div class="ifthen-example">
                <h4>Example 4: Message Type Routing</h4>
                <p><strong>IF</strong> messageType equals "ADT^A04"</p>
                <p><strong>THEN</strong> Route to: "registration"</p>
                <p><strong>ELSE</strong> Continue</p>
            </div>

            <div class="ifthen-help-note">
                <strong>Tip:</strong> You can combine multiple conditions to handle complex logic!
            </div>
        `;
    }

    addStyles() {
        if (document.getElementById('ifthen-builder-styles')) return;

        const style = document.createElement('style');
        style.id = 'ifthen-builder-styles';
        style.textContent = `
            .if-then-else-builder {
                width: 100%;
                padding: 1rem;
                background: #ffffff;
            }

            .ifthen-header {
                margin-bottom: 1.5rem;
            }

            .ifthen-help-btn {
                background: var(--accent-pink);
                border: none;
                border-radius: 50%;
                width: 32px;
                height: 32px;
                cursor: pointer;
                color: #1e3a8a;
                font-size: 1rem;
                transition: all 0.2s;
            }

            .ifthen-help-btn:hover {
                background: var(--accent-pink-hover);
                transform: scale(1.1);
            }

            .ifthen-conditions-list {
                display: flex;
                flex-direction: column;
                gap: 1rem;
            }

            .ifthen-condition-card {
                border: 2px solid var(--accent-pink);
                border-radius: 8px;
                padding: 1rem;
                background: #fefcfd;
            }

            .ifthen-condition-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 1rem;
                padding-bottom: 0.75rem;
                border-bottom: 1px solid var(--accent-pink);
            }

            .ifthen-condition-name {
                flex: 1;
                border: 1px solid #e5e7eb;
                border-radius: 4px;
                padding: 0.5rem;
                font-size: 0.875rem;
                font-weight: 600;
                color: var(--primary-color);
            }

            .ifthen-delete-btn {
                background: #ef4444;
                border: none;
                border-radius: 4px;
                padding: 0.5rem 0.75rem;
                color: white;
                cursor: pointer;
                margin-left: 0.5rem;
                transition: background 0.2s;
            }

            .ifthen-delete-btn:hover {
                background: #dc2626;
            }

            .ifthen-section {
                margin-top: 1rem;
                padding: 1rem;
                background: white;
                border-radius: 6px;
                border-left: 3px solid var(--accent-pink);
            }

            .ifthen-section-label {
                font-weight: 600;
                margin-bottom: 0.75rem;
                color: var(--primary-color);
                display: flex;
                align-items: center;
                gap: 0.5rem;
            }

            .ifthen-condition-builder,
            .ifthen-action-builder {
                display: flex;
                flex-direction: column;
                gap: 0.75rem;
            }

            .ifthen-form-group {
                display: flex;
                flex-direction: column;
                gap: 0.25rem;
            }

            .ifthen-form-group label {
                font-size: 0.75rem;
                font-weight: 500;
                color: #374151;
            }

            .ifthen-input,
            .ifthen-select,
            .ifthen-textarea {
                border: 1px solid #d1d5db;
                border-radius: 4px;
                padding: 0.5rem;
                font-size: 0.875rem;
                width: 100%;
                transition: border-color 0.2s;
            }

            .ifthen-input:focus,
            .ifthen-select:focus,
            .ifthen-textarea:focus {
                outline: none;
                border-color: var(--primary-color);
                box-shadow: 0 0 0 3px rgba(30, 58, 138, 0.1);
            }

            .ifthen-checkbox {
                margin-right: 0.5rem;
            }

            .ifthen-hidden {
                display: none !important;
            }

            .ifthen-add-btn {
                width: 100%;
                padding: 0.75rem;
                margin-top: 1rem;
                background: var(--primary-color);
                color: white;
                border: none;
                border-radius: 6px;
                font-size: 0.875rem;
                font-weight: 600;
                cursor: pointer;
                display: flex;
                align-items: center;
                justify-content: center;
                gap: 0.5rem;
                transition: background 0.2s;
            }

            .ifthen-add-btn:hover {
                background: var(--primary-hover);
            }

            .ifthen-help-modal {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
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
                box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1);
            }

            .ifthen-help-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1.5rem;
                border-bottom: 2px solid var(--accent-pink);
            }

            .ifthen-help-header h3 {
                margin: 0;
                color: var(--primary-color);
            }

            .ifthen-help-close {
                background: none;
                border: none;
                font-size: 2rem;
                color: #9ca3af;
                cursor: pointer;
                line-height: 1;
                padding: 0;
                width: 32px;
                height: 32px;
            }

            .ifthen-help-close:hover {
                color: #6b7280;
            }

            .ifthen-help-body {
                padding: 1.5rem;
            }

            .ifthen-example {
                margin-bottom: 1.5rem;
                padding: 1rem;
                background: #fefcfd;
                border-left: 3px solid var(--accent-pink);
                border-radius: 4px;
            }

            .ifthen-example h4 {
                margin: 0 0 0.5rem 0;
                color: var(--primary-color);
                font-size: 0.875rem;
            }

            .ifthen-example p {
                margin: 0.25rem 0;
                font-size: 0.875rem;
                font-family: 'Courier New', monospace;
            }

            .ifthen-help-note {
                padding: 1rem;
                background: #eff6ff;
                border-radius: 4px;
                color: var(--primary-color);
                font-size: 0.875rem;
            }
        `;

        document.head.appendChild(style);
    }

    getConfig() {
        return this.config;
    }

    setConfig(config) {
        this.config = config;
        this.render();
    }
}

// Export
if (typeof window !== 'undefined') {
    window.IfThenElseBuilder = IfThenElseBuilder;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = IfThenElseBuilder;
}
