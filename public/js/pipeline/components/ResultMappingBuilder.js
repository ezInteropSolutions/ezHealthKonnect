/**
 * ResultMappingBuilder Component
 *
 * Visual builder for mapping database columns to output field names
 * Used in Database Enrichment step configuration
 *
 * NO-CODE INTEGRATION ENGINE: Eliminates raw JSON editing for result mappings
 *
 * Example:
 * Input:  {"patient_name": "fullName", "dob": "dateOfBirth"}
 * Output: Visual table with add/remove rows
 */

class ResultMappingBuilder {
    constructor(container, initialMappings = {}) {
        this.container = container;
        this.mappings = initialMappings; // {"db_column": "output_field"}
        this.availableColumns = []; // Database columns from query results
        this.render();
    }

    render() {
        this.container.innerHTML = `
            <div class="result-mapping-builder">
                <div class="mapping-builder-header">
                    <span class="builder-label">Map Database Columns to Output Fields</span>
                    <button type="button" class="btn btn-sm btn-primary add-mapping-btn">
                        <i class="fas fa-plus"></i> Add Mapping
                    </button>
                </div>
                <div class="mapping-table-container">
                    <table class="mapping-table">
                        <thead>
                            <tr>
                                <th>Database Column Name</th>
                                <th>Output Field Name</th>
                                <th style="width: 80px;">Actions</th>
                            </tr>
                        </thead>
                        <tbody id="mappingRows"></tbody>
                    </table>
                    <div class="empty-state" style="display: none;">
                        <i class="fas fa-database"></i>
                        <p>No mappings configured</p>
                        <small class="text-muted">Click "Add Mapping" or use the Query Tester to auto-add fields</small>
                    </div>
                </div>
                <div class="mapping-builder-footer">
                    <small class="text-muted">
                        <i class="fas fa-info-circle"></i>
                        <strong>Note:</strong> If no mappings are configured, ALL database columns will be returned with their original names.
                        Add mappings to rename columns or select specific fields.
                    </small>
                </div>
            </div>
        `;

        this.renderRows();
        this.attachEventListeners();
        this.updateEmptyState();
    }

    renderRows() {
        const tbody = this.container.querySelector('#mappingRows');
        tbody.innerHTML = '';

        Object.entries(this.mappings).forEach(([dbColumn, outputField]) => {
            this.addRow(dbColumn, outputField);
        });
    }

    addRow(dbColumn = '', outputField = '') {
        const tbody = this.container.querySelector('#mappingRows');
        const rowId = `row_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
        const datalistId = `columns_${rowId}`;

        const row = document.createElement('tr');
        row.id = rowId;
        row.innerHTML = `
            <td>
                <input type="text"
                       class="form-control db-column-input"
                       value="${this.escapeHtml(dbColumn)}"
                       placeholder="e.g., patient_name, mrn, dob"
                       list="${datalistId}"
                       autocomplete="off"
                       required>
                <datalist id="${datalistId}">
                    ${this.availableColumns.map(col => `<option value="${this.escapeHtml(col)}">${this.escapeHtml(col)}</option>`).join('')}
                </datalist>
                ${this.availableColumns.length > 0 ? `<small class="text-muted">Available columns: ${this.availableColumns.length}</small>` : ''}
            </td>
            <td>
                <div class="output-field-wrapper"></div>
            </td>
            <td>
                <button type="button"
                        class="btn btn-sm btn-danger delete-row-btn"
                        title="Remove mapping">
                    <i class="fas fa-trash"></i>
                </button>
            </td>
        `;

        tbody.appendChild(row);

        // Create output field input with FieldPathSearchComponent
        const outputFieldWrapper = row.querySelector('.output-field-wrapper');
        const outputFieldInput = document.createElement('input');
        outputFieldInput.type = 'text';
        outputFieldInput.className = 'form-control output-field-input';
        outputFieldInput.value = this.escapeHtml(outputField);
        outputFieldInput.placeholder = 'e.g., enriched.patient.fullName or search...';
        outputFieldInput.required = true;
        outputFieldWrapper.appendChild(outputFieldInput);

        // Initialize FieldPathSearchComponent for output field if available
        if (typeof FieldPathSearchComponent !== 'undefined') {
            const fieldPathSearch = new FieldPathSearchComponent(outputFieldInput, {
                onSelect: (fieldPath) => {
                    // Prefix with enriched.database. for consistency
                    const enrichedPath = fieldPath.includes('.') ?
                        `enriched.database.${fieldPath}` :
                        `enriched.database.${fieldPath}`;
                    outputFieldInput.value = enrichedPath;
                },
                placeholder: 'Search HL7 fields or enter custom path...',
                allowCustom: true,
                showCategories: true
            });
            row._fieldPathSearch = fieldPathSearch;
        }

        // Add delete handler
        row.querySelector('.delete-row-btn').addEventListener('click', () => {
            // Cleanup FieldPathSearchComponent if exists
            if (row._fieldPathSearch) {
                row._fieldPathSearch.destroy();
            }
            row.remove();
            this.updateEmptyState();
        });

        // Add change handlers for auto-save
        row.querySelector('.db-column-input').addEventListener('change', () => {
            this.updateEmptyState();
        });

        row.querySelector('.output-field-input').addEventListener('change', () => {
            this.updateEmptyState();
        });
    }

    attachEventListeners() {
        const addBtn = this.container.querySelector('.add-mapping-btn');
        addBtn.addEventListener('click', () => {
            this.addRow();
            this.updateEmptyState();

            // Focus on the first input of the new row
            const tbody = this.container.querySelector('#mappingRows');
            const lastRow = tbody.lastElementChild;
            if (lastRow) {
                const firstInput = lastRow.querySelector('.db-column-input');
                firstInput.focus();
            }
        });
    }

    updateEmptyState() {
        const tbody = this.container.querySelector('#mappingRows');
        const emptyState = this.container.querySelector('.empty-state');
        const table = this.container.querySelector('.mapping-table');

        if (tbody.children.length === 0) {
            table.style.display = 'none';
            emptyState.style.display = 'block';
        } else {
            table.style.display = 'table';
            emptyState.style.display = 'none';
        }
    }

    getMappings() {
        const mappings = {};
        const rows = this.container.querySelectorAll('#mappingRows tr');

        rows.forEach(row => {
            const dbColumn = row.querySelector('.db-column-input').value.trim();
            const outputField = row.querySelector('.output-field-input').value.trim();

            if (dbColumn && outputField) {
                mappings[dbColumn] = outputField;
            }
        });

        return mappings;
    }

    /**
     * Auto-add mapping from query tester
     * Called when user clicks [+ Add to Mapping] in DatabaseQueryTester
     *
     * @param {string} dbColumn - Database column name from query result
     * @param {string} suggestedOutputField - Optional suggested output field name
     */
    addMappingFromQueryResult(dbColumn, suggestedOutputField = null) {
        // Check if mapping already exists
        const existingMappings = this.getMappings();
        if (existingMappings[dbColumn]) {
            console.log(`Mapping for "${dbColumn}" already exists`);
            return;
        }

        // Use suggested field name or convert column name to camelCase
        const outputField = suggestedOutputField || this.toCamelCase(dbColumn);

        this.addRow(dbColumn, outputField);
        this.updateEmptyState();

        // Highlight the newly added row
        const tbody = this.container.querySelector('#mappingRows');
        const newRow = tbody.lastElementChild;
        if (newRow) {
            newRow.classList.add('highlight-row');
            setTimeout(() => {
                newRow.classList.remove('highlight-row');
            }, 2000);
        }

        console.log(`✅ Added mapping: ${dbColumn} → ${outputField}`);
    }

    /**
     * Convert database column name to camelCase
     * Examples:
     *   patient_name → patientName
     *   first_name → firstName
     *   DATE_OF_BIRTH → dateOfBirth
     */
    toCamelCase(str) {
        return str
            .toLowerCase()
            .replace(/[_-](.)/g, (match, char) => char.toUpperCase())
            .replace(/^(.)/, (match, char) => char.toLowerCase());
    }

    /**
     * Clear all mappings
     */
    clear() {
        const tbody = this.container.querySelector('#mappingRows');
        tbody.innerHTML = '';
        this.updateEmptyState();
    }

    /**
     * Set mappings programmatically
     * @param {Object} mappings - Object with {dbColumn: outputField} pairs
     */
    setMappings(mappings) {
        this.mappings = mappings;
        this.renderRows();
        this.updateEmptyState();
    }

    /**
     * Set available database columns from query results
     * Called by DatabaseQueryTester after successful query
     * @param {Array<string>} columns - Array of column names
     */
    setAvailableColumns(columns) {
        console.log('[ResultMappingBuilder] Setting available columns:', columns);
        this.availableColumns = columns || [];
        // Re-render to update datalists
        const currentMappings = this.getMappings();
        this.mappings = currentMappings;
        this.renderRows();
        this.updateEmptyState();
    }

    /**
     * Escape HTML to prevent XSS
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    /**
     * Validate all mappings
     * @returns {Object} {valid: boolean, errors: string[]}
     */
    validate() {
        const errors = [];
        const mappings = this.getMappings();
        const dbColumns = Object.keys(mappings);
        const outputFields = Object.values(mappings);

        // Check for duplicate database columns
        const duplicateColumns = dbColumns.filter((col, index) => dbColumns.indexOf(col) !== index);
        if (duplicateColumns.length > 0) {
            errors.push(`Duplicate database columns: ${duplicateColumns.join(', ')}`);
        }

        // Check for duplicate output fields
        const duplicateFields = outputFields.filter((field, index) => outputFields.indexOf(field) !== index);
        if (duplicateFields.length > 0) {
            errors.push(`Duplicate output fields: ${duplicateFields.join(', ')}`);
        }

        // Check for empty values
        const rows = this.container.querySelectorAll('#mappingRows tr');
        rows.forEach((row, index) => {
            const dbColumn = row.querySelector('.db-column-input').value.trim();
            const outputField = row.querySelector('.output-field-input').value.trim();

            if (!dbColumn) {
                errors.push(`Row ${index + 1}: Database column is empty`);
            }

            if (!outputField) {
                errors.push(`Row ${index + 1}: Output field is empty`);
            }
        });

        return {
            valid: errors.length === 0,
            errors: errors
        };
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = ResultMappingBuilder;
}
