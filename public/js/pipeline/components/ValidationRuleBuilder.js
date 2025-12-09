/**
 * ValidationRuleBuilder - MVC Component for Building Validation Rules
 * Modular, reusable, object-oriented validation rule builder
 *
 * @class ValidationRuleBuilder
 * @follows MVC Pattern
 */

class ValidationRuleBuilder {
    /**
     * Format presets configuration
     * @static
     */
    static FORMAT_PRESETS = [
        { value: 'phone', label: 'Phone Number', regex: '^\\d{10}$|^\\d{3}-\\d{3}-\\d{4}$' },
        { value: 'ssn', label: 'SSN (XXX-XX-XXXX)', regex: '^\\d{3}-\\d{2}-\\d{4}$' },
        { value: 'email', label: 'Email Address', regex: '^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$' },
        { value: 'date', label: 'Date (YYYYMMDD)', regex: '^\\d{8}$' },
        { value: 'datetime', label: 'DateTime (YYYYMMDDHHMMSS)', regex: '^\\d{14}$' },
        { value: 'mrn', label: 'MRN (Medical Record Number)', regex: '^[A-Z0-9]{6,12}$' },
        { value: 'zip', label: 'ZIP Code', regex: '^\\d{5}(-\\d{4})?$' }
    ];

    /**
     * Validation type configuration
     * @static
     */
    static VALIDATION_TYPES = [
        { value: 'required', label: 'Required Field', hasOptions: false },
        { value: 'format', label: 'Format Check', hasOptions: true },
        { value: 'length', label: 'Length Validation', hasOptions: true },
        { value: 'pattern', label: 'Regex Pattern', hasOptions: true }
    ];

    /**
     * Constructor
     * @param {HTMLElement} container - Container element for the builder
     * @param {Array} initialRules - Initial rules to populate
     * @param {Object} options - Configuration options
     */
    constructor(container, initialRules = [], options = {}) {
        this.container = container;
        this.rules = initialRules.length > 0 ? initialRules : [this.createEmptyRule()];
        this.hiddenField = null;
        this.fieldSelectors = [];
        // Removed inputMode - using StaticFieldDropdown only // Global search mode for all field inputs

        // Options (keeping for backward compatibility)
        this.options = {
            format: options.format || 'hl7v2',
            version: options.version || 'v2.5',
            messageType: options.messageType || '',
            resourceType: options.resourceType || ''
        };

        this.render();
        this.attachEventListeners();
        this.initializeFieldSelectors();
    }

    /**
     * Create empty rule object
     * @returns {Object} Empty rule
     */
    createEmptyRule() {
        return {
            field: '',
            type: 'required',
            errorMessage: ''
        };
    }

    /**
     * Render the validation builder
     */
    render() {
        const html = `
            <div class="validation-builder" data-component="validation-rule-builder">
                <div class="validation-rules-list">
                    ${this.rules.map((rule, index) => this.renderRuleRow(rule, index)).join('')}
                </div>
                <button type="button" class="btn-add-rule">
                    <span style="font-size: 1.2em; margin-right: 0.25rem;">+</span> Add Rule
                </button>
                <input type="hidden" id="validationRules" name="validationRules" value='${JSON.stringify(this.rules)}'>
            </div>
        `;

        this.container.innerHTML = html;
        this.hiddenField = this.container.querySelector('#validationRules');
        console.log('[ValidationRuleBuilder] After render, hidden field:', this.hiddenField ? 'FOUND' : 'NOT FOUND', 'ID:', this.hiddenField?.id);
    }

    /**
     * Escape HTML to prevent attribute breaking
     */
    escapeHtml(text) {
        if (!text) return '';
        return String(text)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#039;');
    }

    /**
     * Render a single rule row
     * @param {Object} rule - Rule object
     * @param {number} index - Row index
     * @returns {string} HTML string
     */
    renderRuleRow(rule, index) {
        const validationType = rule.type || 'required';

        return `
            <div class="validation-rule-row" data-index="${index}">
                <div class="rule-fields">
                    <!-- Field Path -->
                    ${this.renderFieldPath(rule)}

                    <!-- Validation Type -->
                    ${this.renderValidationType(rule)}

                    <!-- Conditional Fields -->
                    ${this.renderConditionalFields(rule, validationType)}

                    <!-- Error Message -->
                    ${this.renderErrorMessage(rule)}

                    <!-- Actions -->
                    ${this.renderActions()}
                </div>
            </div>
        `;
    }

    /**
     * Render field path input with simple field selector
     * @param {Object} rule
     * @returns {string}
     */
    renderFieldPath(rule) {
        return `
            <div class="rule-field">
                <label>Field Path</label>
                <div class="field-path-input-container" data-field-index="${this.rules.indexOf(rule)}"></div>
                <input type="hidden"
                       class="rule-field-path"
                       value="${this.escapeHtml(rule.field)}"
                       data-rule-prop="field">
            </div>
        `;
    }

    /**
     * Render validation type dropdown
     * @param {Object} rule
     * @returns {string}
     */
    renderValidationType(rule) {
        const validationType = rule.type || 'required';
        const options = ValidationRuleBuilder.VALIDATION_TYPES.map(type =>
            `<option value="${type.value}" ${validationType === type.value ? 'selected' : ''}>${type.label}</option>`
        ).join('');

        return `
            <div class="rule-field">
                <label>Validation Type</label>
                <select class="rule-type" data-rule-prop="type">
                    ${options}
                </select>
            </div>
        `;
    }

    /**
     * Render conditional fields based on validation type
     * @param {Object} rule
     * @param {string} validationType
     * @returns {string}
     */
    renderConditionalFields(rule, validationType) {
        let html = '';

        // Format preset dropdown
        if (validationType === 'format') {
            html += this.renderFormatPreset(rule);
        }

        // Length inputs
        if (validationType === 'length') {
            html += this.renderLengthInputs(rule);
        }

        // Pattern input
        if (validationType === 'pattern') {
            html += this.renderPatternInput(rule);
        }

        return html;
    }

    /**
     * Render format preset dropdown
     * @param {Object} rule
     * @returns {string}
     */
    renderFormatPreset(rule) {
        const options = ValidationRuleBuilder.FORMAT_PRESETS.map(preset =>
            `<option value="${preset.value}" ${rule.format === preset.value ? 'selected' : ''}>${preset.label}</option>`
        ).join('');

        return `
            <div class="rule-field rule-format-field">
                <label>Format Preset</label>
                <select class="rule-format-preset" data-rule-prop="format">
                    <option value="">Select format...</option>
                    ${options}
                </select>
                <small style="color: #6b7280;">Pre-defined format validation</small>
            </div>
        `;
    }

    /**
     * Render length min/max inputs
     * @param {Object} rule
     * @returns {string}
     */
    renderLengthInputs(rule) {
        return `
            <div class="rule-field rule-length-field">
                <label>Min/Max Length</label>
                <div style="display: flex; gap: 0.5rem;">
                    <input type="number"
                           class="rule-min-length"
                           placeholder="Min"
                           value="${rule.minLength || ''}"
                           data-rule-prop="minLength"
                           style="width: 50%;">
                    <input type="number"
                           class="rule-max-length"
                           placeholder="Max"
                           value="${rule.maxLength || ''}"
                           data-rule-prop="maxLength"
                           style="width: 50%;">
                </div>
                <small style="color: #6b7280;">Leave blank for no limit</small>
            </div>
        `;
    }

    /**
     * Render pattern input
     * @param {Object} rule
     * @returns {string}
     */
    renderPatternInput(rule) {
        return `
            <div class="rule-field rule-pattern-field">
                <label>Regex Pattern</label>
                <input type="text"
                       class="rule-pattern"
                       placeholder="e.g., ^[A-Z]{2}\\d{6}$"
                       value="${rule.pattern || ''}"
                       data-rule-prop="pattern">
                <small style="color: #6b7280;">Enter custom regex pattern</small>
            </div>
        `;
    }

    /**
     * Render error message input
     * @param {Object} rule
     * @returns {string}
     */
    renderErrorMessage(rule) {
        return `
            <div class="rule-field">
                <label>Error Message</label>
                <input type="text"
                       class="rule-error-msg"
                       placeholder="Custom error message (optional)"
                       value="${this.escapeHtml(rule.errorMessage)}"
                       data-rule-prop="errorMessage">
            </div>
        `;
    }

    /**
     * Render action buttons
     * @returns {string}
     */
    renderActions() {
        return `
            <div class="rule-actions">
                <button type="button" class="btn-remove-rule" title="Remove Rule">
                    <span style="color: #ef4444; font-size: 1.2em;">×</span>
                </button>
            </div>
        `;
    }

    /**
     * Destroy all field selector instances (MUST be called BEFORE render())
     */
    destroyFieldSelectors() {
        if (this.fieldSelectors && this.fieldSelectors.length > 0) {
            console.log(`[ValidationRuleBuilder] Destroying ${this.fieldSelectors.length} field selectors...`);
            this.fieldSelectors.forEach(selector => {
                if (selector && typeof selector.destroy === 'function') {
                    selector.destroy();
                }
            });
        }
        this.fieldSelectors = [];
    }

    /**
     * Initialize field selectors for all field path inputs
     */
    initializeFieldSelectors() {
        // Note: destroyFieldSelectors() should be called BEFORE render()
        // Find all input containers
        const containers = this.container.querySelectorAll('.field-path-input-container');

        containers.forEach((container, index) => {
            // Use autocomplete with MongoDB field data
            if (!window.FieldPathInputWithAutocomplete) {
                console.error('[ValidationRuleBuilder] FieldPathInputWithAutocomplete component not loaded!');
                return;
            }

            const rule = this.rules[index];
            const hiddenInput = container.parentElement.querySelector('.rule-field-path');

            const input = new FieldPathInputWithAutocomplete(container, {
                initialValue: rule.field || '',
                searchMode: 'path',
                onChange: (path, fieldData) => {
                    hiddenInput.value = path;
                    this.rules[index].field = path;
                    // Store field description for better error messages
                    if (fieldData && fieldData.description) {
                        this.rules[index]._fieldDescription = fieldData.description;
                    }
                    this.updateHiddenField();
                    
                    // Auto-populate error message when field is selected
                    const ruleRow = container.closest('.validation-rule-row');
                    const validationType = ruleRow.querySelector('.rule-type')?.value || 'required';
                    this.autoPopulateErrorMessage(ruleRow, validationType, index);
                }
            });

            // CRITICAL FIX: Set hidden input value immediately for existing rules
            // onChange only fires on user selection, not on initialization
            if (rule.field) {
                hiddenInput.value = rule.field;
                console.log('[ValidationRuleBuilder] Initialized rule', index + 1, 'field:', rule.field);
            }
            if (rule.errorMessage) {
                console.log('[ValidationRuleBuilder] Initialized rule', index + 1, 'error:', rule.errorMessage);
            }

            this.fieldSelectors.push(input);
        });
    }

    /**
     * Update schema configuration for all autocompletes
     * @param {Object} options - New schema options
     */
    updateSchemaOptions(options) {
        Object.assign(this.options, options);

        this.xpathAutocompletes.forEach(autocomplete => {
            autocomplete.updateOptions({
                format: this.options.format,
                version: this.options.version,
                messageType: this.options.messageType,
                resourceType: this.options.resourceType
            });
        });
    }

    /**
     * Attach event listeners
     */
    attachEventListeners() {
        const builderContainer = this.container.querySelector('.validation-builder');

        // Input mode toggle buttons
        const toggleButtons = builderContainer.querySelectorAll('.toggle-btn');
        toggleButtons.forEach(btn => {
            btn.addEventListener('click', () => {
                const mode = btn.dataset.mode;
                this.setInputMode(mode);
            });
        });

        // Add rule button
        const addBtn = builderContainer.querySelector('.btn-add-rule');
        addBtn.addEventListener('click', () => this.addRule());

        // Remove rule buttons
        this.attachRemoveListeners();

        // Input change listeners
        this.attachInputListeners();

        // Type change listeners for conditional fields
        this.attachTypeChangeListeners();
    }

    /**
     * Set input mode for all field inputs (autocomplete vs freetext)
     */
    setInputMode(mode) {
        this.inputMode = mode;

        // Update toggle button active state
        const toggleButtons = this.container.querySelectorAll('.toggle-btn');
        toggleButtons.forEach(btn => {
            if (btn.dataset.mode === mode) {
                btn.classList.add('active');
            } else {
                btn.classList.remove('active');
            }
        });

        // Destroy and recreate all field selectors with new mode
        this.destroyFieldSelectors();
        this.initializeFieldSelectors();
    }

    /**
     * Attach remove button listeners
     */
    attachRemoveListeners() {
        const removeButtons = this.container.querySelectorAll('.btn-remove-rule');
        removeButtons.forEach((btn, index) => {
            btn.addEventListener('click', () => this.removeRule(index));
        });
    }

    /**
     * Attach input change listeners
     */
    attachInputListeners() {
        const inputs = this.container.querySelectorAll('[data-rule-prop]');
        inputs.forEach(input => {
            input.addEventListener('change', () => this.updateRulesFromDOM());
            // REMOVED: input.addEventListener('input', ...) - causes Out of Memory crash
        });
    }

    /**
     * Attach type change listeners for showing/hiding conditional fields
     */
    attachTypeChangeListeners() {
        const typeSelects = this.container.querySelectorAll('.rule-type');
        typeSelects.forEach((select, index) => {
            select.addEventListener('change', (e) => this.handleTypeChange(e.target, index));
        });
    }

    /**
     * Handle validation type change
     * @param {HTMLElement} selectElement
     * @param {number} ruleIndex
     */
    handleTypeChange(selectElement, ruleIndex) {
        const ruleRow = selectElement.closest('.validation-rule-row');
        const validationType = selectElement.value;

        // Hide all conditional fields
        ruleRow.querySelectorAll('.rule-format-field, .rule-length-field, .rule-pattern-field').forEach(el => {
            el.remove();
        });

        // Show appropriate conditional field
        let conditionalHTML = '';
        const rule = this.rules[ruleIndex];

        if (validationType === 'format') {
            conditionalHTML = this.renderFormatPreset(rule);
        } else if (validationType === 'length') {
            conditionalHTML = this.renderLengthInputs(rule);
        } else if (validationType === 'pattern') {
            conditionalHTML = this.renderPatternInput(rule);
        }

        if (conditionalHTML) {
            const typeField = ruleRow.querySelector('.rule-field:nth-child(2)');
            typeField.insertAdjacentHTML('afterend', conditionalHTML);

            // Re-attach listeners for new fields
            this.attachInputListeners();
        }

        // Auto-populate error message
        this.autoPopulateErrorMessage(ruleRow, validationType, ruleIndex);

        // Update rules model
        this.updateRulesFromDOM();
    }

    /**
     * Add new rule
     */
    addRule() {
        this.rules.push(this.createEmptyRule());
        this.destroyFieldSelectors(); // Destroy BEFORE render destroys DOM
        this.render();
        this.attachEventListeners();
        this.initializeFieldSelectors();
    }

    /**
     * Remove rule
     * @param {number} index
     */
    removeRule(index) {
        if (this.rules.length === 1) {
            // Clear the only rule instead of removing
            this.rules[0] = this.createEmptyRule();
        } else {
            this.rules.splice(index, 1);
        }
        this.destroyFieldSelectors(); // Destroy BEFORE render destroys DOM
        this.render();
        this.attachEventListeners();
        this.initializeFieldSelectors();
    }

    /**
     * Update rules model from DOM
     */
    updateRulesFromDOM() {
        const ruleRows = this.container.querySelectorAll('.validation-rule-row');
        const updatedRules = [];

        console.log('[ValidationRuleBuilder] updateRulesFromDOM: Found', ruleRows.length, 'rule rows');

        ruleRows.forEach((row, index) => {
            const rule = {};
            const inputs = row.querySelectorAll('[data-rule-prop]');

            console.log('[ValidationRuleBuilder] Processing row', index + 1, '- found', inputs.length, 'inputs');

            inputs.forEach(input => {
                const prop = input.dataset.ruleProp;
                let value = input.value.trim();
                console.log('[ValidationRuleBuilder]   Input:', prop, '=', value || '(empty)');
                console.log('[ValidationRuleBuilder]   Input:', prop, '=', value || '(empty)');

                // Type conversion
                if (prop === 'minLength' || prop === 'maxLength') {
                    value = value ? parseInt(value) : null;
                }

                // Only add non-empty values
                if (value !== '' && value !== null) {
                    rule[prop] = value;
                }
            });

            // Only add rules with at least a field path
            if (rule.field) {
                updatedRules.push(rule);
                console.log(`[ValidationRuleBuilder] Rule ${index + 1}: field=${rule.field}, type=${rule.type}`);
            } else {
                console.warn(`[ValidationRuleBuilder] Rule ${index + 1} skipped (no field path)`);
            }
        });

        this.rules = updatedRules.length > 0 ? updatedRules : [this.createEmptyRule()];
        console.log('[ValidationRuleBuilder] Final rules count:', this.rules.length);
        this.updateHiddenField();
    }

    /**
     * Update hidden field with current rules
     */
    updateHiddenField() {
        console.log('[ValidationRuleBuilder] updateHiddenField called, rules:', this.rules.length);
        console.log('[ValidationRuleBuilder] 📋 this.rules content:', JSON.stringify(this.rules, null, 2));
        if (this.hiddenField) {
            this.hiddenField.value = JSON.stringify(this.rules);
            console.log('[ValidationRuleBuilder] ✅ Updated hidden field with', this.rules.length, 'rules');
            console.log('[ValidationRuleBuilder] Hidden field ID:', this.hiddenField.id, 'value:', this.hiddenField.value);
        } else {
            console.error('[ValidationRuleBuilder] ❌ hiddenField is null!');
        }
    }

    /**
     * Get current rules
     * @returns {Array}
     */
    getRules() {
        return this.rules;
    }

    /**
     * Set rules programmatically
     * @param {Array} rules
     */
    setRules(rules) {
        this.rules = rules.length > 0 ? rules : [this.createEmptyRule()];
        this.destroyFieldSelectors(); // Destroy BEFORE render destroys DOM
        this.render();
        this.attachEventListeners();
        this.initializeFieldSelectors();
    }

    /**
     * Validate all rules
     * @returns {Object} Validation result
     */
    validate() {
        const errors = [];

        this.rules.forEach((rule, index) => {
            if (!rule.field) {
                errors.push(`Rule ${index + 1}: Field path is required`);
            }

            if (rule.type === 'format' && !rule.format) {
                errors.push(`Rule ${index + 1}: Format preset is required`);
            }

            if (rule.type === 'pattern' && !rule.pattern) {
                errors.push(`Rule ${index + 1}: Regex pattern is required`);
            }

            if (rule.type === 'length' && !rule.minLength && !rule.maxLength) {
                errors.push(`Rule ${index + 1}: At least one length constraint is required`);
            }
        });

        return {
            valid: errors.length === 0,
            errors: errors
        };
    }


    /**
     * Auto-populate error message based on validation type and field
     * @param {HTMLElement} ruleRow - The rule row element
     * @param {string} validationType - Type of validation
     * @param {number} ruleIndex - Index of the rule
     */
    autoPopulateErrorMessage(ruleRow, validationType, ruleIndex) {
        const errorMsgInput = ruleRow.querySelector('.rule-error-msg');
        if (!errorMsgInput) return;

        const rule = this.rules[ruleIndex];

        // Use stored field description from autocomplete, fallback to parsed field name
        const fieldName = rule._fieldDescription || this.getFieldDisplayName(rule.field);

        let defaultMessage = '';
        switch (validationType) {
            case 'required':
                defaultMessage = `${fieldName} is required`;
                break;
            case 'format':
                defaultMessage = `${fieldName} has invalid format`;
                break;
            case 'length':
                defaultMessage = `${fieldName} length is invalid`;
                break;
            case 'pattern':
                defaultMessage = `${fieldName} does not match required pattern`;
                break;
        }

        if (defaultMessage) {
            errorMsgInput.value = defaultMessage;
            this.rules[ruleIndex].errorMessage = defaultMessage;
            this.updateHiddenField();
            console.log(`[ValidationRuleBuilder] Auto-populated error message: "${defaultMessage}"`);
        }
    }

    /**
     * Get display name from field path
     * @param {string} fieldPath - Field path
     * @returns {string} Display name
     */
    getFieldDisplayName(fieldPath) {
        if (!fieldPath) return 'Field';

        // Try to extract from path like "enhancedSegments.PID.fields[2].value"
        const match = fieldPath.match(/enhancedSegments\.(\w+)\./);
        if (match) {
            const segment = match[1];
            const segmentNames = {
                'PID': 'Patient ID',
                'MSH': 'Message Header',
                'PV1': 'Patient Visit',
                'OBR': 'Observation Request',
                'OBX': 'Observation Result',
                'DG1': 'Diagnosis',
                'AL1': 'Allergy'
            };
            return segmentNames[segment] || segment;
        }

        return 'Field';
    }

    /**
     * Destroy the builder and cleanup
     */
    destroy() {
        this.destroyFieldSelectors();
        this.container.innerHTML = '';
        this.rules = [];
        this.hiddenField = null;
    }
}

// Export for use in other modules
if (typeof window !== 'undefined') {
    window.ValidationRuleBuilder = ValidationRuleBuilder;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = ValidationRuleBuilder;
}
