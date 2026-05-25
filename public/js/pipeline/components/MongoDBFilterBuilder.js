/**
 * MongoDBFilterBuilder Component
 *
 * NO-CODE visual builder for MongoDB filter queries
 * Eliminates the need to write JSON manually
 *
 * Features:
 * - Visual field selection with autocomplete
 * - Operator dropdown (equals, contains, greater than, etc.)
 * - Value input with HL7 field placeholder support
 * - Click to add multiple filter conditions
 * - AND/OR logic support
 */

class MongoDBFilterBuilder {
    constructor(container, initialFilter = {}) {
        this.container = container;
        this.filter = initialFilter;
        this.conditions = this.parseFilterToConditions(initialFilter);
        this.render();
    }

    /**
     * Convert MongoDB filter object to visual conditions
     * Example: { "mrn": "value", "status": "active" }
     * -> [{ field: "mrn", operator: "equals", value: "value" }, ...]
     */
    parseFilterToConditions(filter) {
        const conditions = [];

        for (const [field, value] of Object.entries(filter)) {
            // Simple equality for now
            conditions.push({
                field: field,
                operator: 'equals',
                value: value,
                id: this.generateId()
            });
        }

        if (conditions.length === 0) {
            // Start with one empty condition
            conditions.push({
                field: '',
                operator: 'equals',
                value: '',
                id: this.generateId()
            });
        }

        return conditions;
    }

    generateId() {
        return 'cond_' + Math.random().toString(36).substr(2, 9);
    }

    render() {
        this.container.innerHTML = `
            <div class="mongodb-filter-builder">
                <div class="filter-header">
                    <div class="filter-title">
                        <i class="fas fa-filter"></i>
                        <span>Filter Conditions</span>
                        <span class="help-icon" title="Define which documents to find. Example: Find patients where mrn equals a specific value">
                            <i class="fas fa-question-circle"></i>
                        </span>
                    </div>
                    <button type="button" class="btn btn-sm btn-primary add-condition-btn">
                        <i class="fas fa-plus"></i> Add Condition
                    </button>
                </div>

                <div class="conditions-list"></div>

                <div class="filter-preview">
                    <label>Generated Filter (Preview)</label>
                    <pre class="filter-json-preview"></pre>
                </div>
            </div>
        `;

        this.renderConditions();
        this.attachEventListeners();
        this.updatePreview();
    }

    renderConditions() {
        const conditionsList = this.container.querySelector('.conditions-list');

        if (this.conditions.length === 0) {
            conditionsList.innerHTML = `
                <div class="empty-state">
                    <i class="fas fa-info-circle"></i>
                    <p>No filter conditions defined</p>
                    <small>Click "Add Condition" to start building your MongoDB filter</small>
                </div>
            `;
            return;
        }

        let html = '<div class="conditions-table">';
        html += '<div class="conditions-header">';
        html += '<div class="col-field">Field Name</div>';
        html += '<div class="col-operator">Operator</div>';
        html += '<div class="col-value">Value</div>';
        html += '<div class="col-actions">Actions</div>';
        html += '</div>';

        this.conditions.forEach((condition, index) => {
            html += this.renderCondition(condition, index);
        });

        html += '</div>';
        conditionsList.innerHTML = html;
    }

    renderCondition(condition, index) {
        const operators = [
            { value: 'equals', label: 'Equals (=)', example: 'mrn equals "12345"' },
            { value: 'not_equals', label: 'Not Equals (≠)', example: 'status not equals "inactive"' },
            { value: 'contains', label: 'Contains', example: 'name contains "John"' },
            { value: 'starts_with', label: 'Starts With', example: 'mrn starts with "MRN"' },
            { value: 'gt', label: 'Greater Than (>)', example: 'age > 18' },
            { value: 'gte', label: 'Greater or Equal (≥)', example: 'age >= 18' },
            { value: 'lt', label: 'Less Than (<)', example: 'age < 65' },
            { value: 'lte', label: 'Less or Equal (≤)', example: 'age <= 65' },
            { value: 'in', label: 'In List', example: 'status in ["active", "pending"]' },
            { value: 'exists', label: 'Field Exists', example: 'email exists' }
        ];

        let html = `<div class="condition-row" data-condition-id="${condition.id}">`;

        // Field Name
        html += `<div class="col-field">
            <input type="text"
                   class="form-control form-control-sm condition-field"
                   value="${this.escapeHtml(condition.field)}"
                   placeholder="mrn"
                   list="mongodb-field-suggestions-${index}">
            <datalist id="mongodb-field-suggestions-${index}">
                <option value="mrn">mrn - Medical Record Number</option>
                <option value="firstName">firstName - Patient First Name</option>
                <option value="lastName">lastName - Patient Last Name</option>
                <option value="dateOfBirth">dateOfBirth - Date of Birth</option>
                <option value="gender">gender - Gender</option>
                <option value="ssn">ssn - Social Security Number</option>
                <option value="phone">phone - Phone Number</option>
                <option value="email">email - Email Address</option>
                <option value="status">status - Status</option>
            </datalist>
        </div>`;

        // Operator
        html += `<div class="col-operator">
            <select class="form-control form-control-sm condition-operator">`;
        operators.forEach(op => {
            const selected = condition.operator === op.value ? 'selected' : '';
            html += `<option value="${op.value}" ${selected} title="${op.example}">${op.label}</option>`;
        });
        html += `</select>
        </div>`;

        // Value
        html += `<div class="col-value">
            <input type="text"
                   class="form-control form-control-sm condition-value"
                   value="${this.escapeHtml(condition.value)}"
                   placeholder="MRN123456 or {PID.3}">
            <small class="text-muted">Use {PID.3} for HL7 field values</small>
        </div>`;

        // Actions
        html += `<div class="col-actions">
            <button type="button" class="btn btn-sm btn-danger remove-condition-btn" title="Remove condition">
                <i class="fas fa-trash"></i>
            </button>
        </div>`;

        html += '</div>';
        return html;
    }

    attachEventListeners() {
        // Add condition button
        const addBtn = this.container.querySelector('.add-condition-btn');
        addBtn.addEventListener('click', () => this.addCondition());

        // Delegate events for dynamic condition rows
        const conditionsList = this.container.querySelector('.conditions-list');

        conditionsList.addEventListener('click', (e) => {
            if (e.target.closest('.remove-condition-btn')) {
                const row = e.target.closest('.condition-row');
                const conditionId = row.dataset.conditionId;
                this.removeCondition(conditionId);
            }
        });

        conditionsList.addEventListener('input', (e) => {
            if (e.target.matches('.condition-field, .condition-operator, .condition-value')) {
                this.updateConditionFromUI(e.target.closest('.condition-row'));
            }
        });

        conditionsList.addEventListener('change', (e) => {
            if (e.target.matches('.condition-operator')) {
                this.updateConditionFromUI(e.target.closest('.condition-row'));
            }
        });
    }

    addCondition() {
        this.conditions.push({
            field: '',
            operator: 'equals',
            value: '',
            id: this.generateId()
        });
        this.renderConditions();
        this.updatePreview();
    }

    removeCondition(conditionId) {
        this.conditions = this.conditions.filter(c => c.id !== conditionId);
        this.renderConditions();
        this.updatePreview();
    }

    updateConditionFromUI(row) {
        const conditionId = row.dataset.conditionId;
        const condition = this.conditions.find(c => c.id === conditionId);

        if (condition) {
            condition.field = row.querySelector('.condition-field').value;
            condition.operator = row.querySelector('.condition-operator').value;
            condition.value = row.querySelector('.condition-value').value;
            this.updatePreview();
        }
    }

    updatePreview() {
        const filter = this.buildFilter();
        const preview = this.container.querySelector('.filter-json-preview');
        preview.textContent = JSON.stringify(filter, null, 2);
    }

    /**
     * Build MongoDB filter object from conditions
     * Returns: { "field1": "value1", "field2": { "$gt": 50 } }
     */
    buildFilter() {
        const filter = {};

        this.conditions.forEach(condition => {
            if (!condition.field) return; // Skip empty conditions

            const field = condition.field;
            const value = condition.value;

            switch (condition.operator) {
                case 'equals':
                    filter[field] = value;
                    break;
                case 'not_equals':
                    filter[field] = { $ne: value };
                    break;
                case 'contains':
                    filter[field] = { $regex: value, $options: 'i' };
                    break;
                case 'starts_with':
                    filter[field] = { $regex: '^' + value, $options: 'i' };
                    break;
                case 'gt':
                    filter[field] = { $gt: this.parseValue(value) };
                    break;
                case 'gte':
                    filter[field] = { $gte: this.parseValue(value) };
                    break;
                case 'lt':
                    filter[field] = { $lt: this.parseValue(value) };
                    break;
                case 'lte':
                    filter[field] = { $lte: this.parseValue(value) };
                    break;
                case 'in':
                    // Parse comma-separated values
                    const values = value.split(',').map(v => v.trim());
                    filter[field] = { $in: values };
                    break;
                case 'exists':
                    filter[field] = { $exists: true };
                    break;
            }
        });

        return filter;
    }

    parseValue(value) {
        // Try to parse as number
        const num = parseFloat(value);
        if (!isNaN(num) && value === num.toString()) {
            return num;
        }
        return value;
    }

    /**
     * Get the current filter configuration
     * Called by PropertiesPanel when saving step
     */
    getFilter() {
        return this.buildFilter();
    }

    escapeHtml(text) {
        const map = {
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            '"': '&quot;',
            "'": '&#039;'
        };
        return String(text).replace(/[&<>"']/g, m => map[m]);
    }
}
