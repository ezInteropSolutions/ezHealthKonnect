/**
 * DatabaseQueryTester Component
 *
 * Test database queries before saving pipeline configuration
 * Shows actual query results and allows click-to-add mapping
 *
 * NO-CODE INTEGRATION ENGINE: See real data before configuration
 *
 * Features:
 * - Test SQL queries with sample parameter values
 * - View actual database results
 * - Click [+ Add to Mapping] to auto-populate ResultMappingBuilder
 * - Error handling and validation
 */

class DatabaseQueryTester {
    constructor(container, config = {}) {
        this.container = container;
        this.config = config;
        this.onAddMappingCallback = null;
        this.render();
    }

    render() {
        this.container.innerHTML = `
            <div class="database-query-tester">
                <div class="tester-header">
                    <div class="tester-title">
                        <i class="fas fa-flask"></i>
                        <span>Test Database Query</span>
                    </div>
                    <button type="button" class="btn btn-sm btn-success run-query-btn">
                        <i class="fas fa-play"></i> Run Query
                    </button>
                </div>

                <div class="tester-body">
                    <div class="test-params-section">
                        <label>Test Parameter Values</label>
                        <div id="testParamsTable"></div>
                        <small class="text-muted">
                            <i class="fas fa-info-circle"></i>
                            Enter sample values to test your query. Example: If query uses $1 for patient ID, enter "12345"
                        </small>
                    </div>

                    <div class="query-results-section" style="display: none;">
                        <div class="results-header">
                            <span class="results-status"></span>
                            <button type="button" class="btn btn-sm btn-outline-secondary clear-results-btn">
                                <i class="fas fa-times"></i> Clear
                            </button>
                        </div>
                        <div class="results-content"></div>
                    </div>

                    <div class="query-error-section" style="display: none;">
                        <div class="alert alert-danger">
                            <strong><i class="fas fa-exclamation-triangle"></i> Query Failed</strong>
                            <pre class="error-message"></pre>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.attachEventListeners();
        this.renderTestParamsTable();
    }

    renderTestParamsTable() {
        const container = this.container.querySelector('#testParamsTable');

        // Get query params from config
        const queryParams = this.config.queryParams || {};
        const paramEntries = Object.entries(queryParams);

        if (paramEntries.length === 0) {
            container.innerHTML = `
                <div class="empty-params-state">
                    <i class="fas fa-info-circle"></i>
                    <p>No query parameters defined yet</p>
                    <small>Add parameters in the "Query Parameters" section above first</small>
                </div>
            `;
            return;
        }

        let html = '<table class="test-params-table">';
        html += '<thead><tr>';
        html += '<th style="width: 80px;">Parameter</th>';
        html += '<th>Field Path</th>';
        html += '<th>Test Value</th>';
        html += '</tr></thead><tbody>';

        paramEntries.forEach(([paramName, fieldPath], index) => {
            html += `
                <tr>
                    <td><code>$${index + 1}</code></td>
                    <td><code>${this.escapeHtml(fieldPath)}</code></td>
                    <td>
                        <input type="text"
                               class="form-control form-control-sm test-param-value"
                               data-param="${this.escapeHtml(paramName)}"
                               placeholder="Enter test value"
                               value="">
                    </td>
                </tr>
            `;
        });

        html += '</tbody></table>';
        container.innerHTML = html;
    }

    attachEventListeners() {
        const runBtn = this.container.querySelector('.run-query-btn');
        runBtn.addEventListener('click', () => this.executeQuery());

        const clearBtn = this.container.querySelector('.clear-results-btn');
        clearBtn.addEventListener('click', () => this.clearResults());
    }

    async executeQuery() {
        const runBtn = this.container.querySelector('.run-query-btn');
        const originalContent = runBtn.innerHTML;
        runBtn.disabled = true;
        runBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Running...';

        this.clearResults();

        try {
            // Validate configuration
            if (!this.config.query || this.config.query.trim() === '') {
                throw new Error('SQL query is required. Please enter a query above.');
            }

            if (!this.config.databaseType) {
                throw new Error('Database type is required. Please select a database type.');
            }

            // Check if we have either connectionString OR individual fields
            const hasConnectionString = this.config.connectionString;
            const hasIndividualFields = this.config.dbHost && this.config.dbPort &&
                                       this.config.dbName && this.config.dbUser && this.config.dbPassword;

            if (!hasConnectionString && !hasIndividualFields) {
                throw new Error('Database connection is required. Please fill in Host, Port, Database, Username, and Password.');
            }

            // Collect test parameter values
            const testParams = {};
            const paramInputs = this.container.querySelectorAll('.test-param-value');
            paramInputs.forEach(input => {
                const paramName = input.dataset.param;
                const value = input.value.trim();
                if (value) {
                    testParams[paramName] = value;
                }
            });

            // Build request payload with both connection methods
            const payload = {
                databaseType: this.config.databaseType,
                connectionString: this.config.connectionString,
                connectionName: this.config.connectionName,
                // Individual connection fields
                dbHost: this.config.dbHost,
                dbPort: this.config.dbPort,
                dbName: this.config.dbName,
                dbUser: this.config.dbUser,
                dbPassword: this.config.dbPassword,
                query: this.config.query,
                queryParams: this.config.queryParams || {},
                testParams: testParams
            };

            console.log('🔍 Testing database query:', payload);
            console.log('🔑 Query params being sent:', payload.queryParams);
            console.log('🧪 Test params being sent:', payload.testParams);

            // Call backend API to test query
            const response = await fetch('/api/database/test-query', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });

            const result = await response.json();
            console.log('📥 Query test response:', result);
            console.log('📊 Columns from backend:', result.columns);
            console.log('📋 Data rows:', result.data);

            if (response.ok && result.success) {
                // Pass both data rows and column names to displayResults
                this.displayResults(result.data || [], result.columns || []);
            } else {
                console.error('❌ Query test failed:', result);
                this.displayError(result.error || 'Query failed with unknown error');
            }
        } catch (error) {
            console.error('Query test error:', error);
            this.displayError(error.message);
        } finally {
            runBtn.disabled = false;
            runBtn.innerHTML = originalContent;
        }
    }

    displayResults(rows, columns) {
        const resultsSection = this.container.querySelector('.query-results-section');
        const resultsContent = this.container.querySelector('.results-content');
        const resultsStatus = this.container.querySelector('.results-status');

        resultsSection.style.display = 'block';
        resultsStatus.innerHTML = `
            <i class="fas fa-check-circle" style="color: #28a745;"></i>
            Query Result (${rows.length} row${rows.length !== 1 ? 's' : ''})
        `;

        // Update ResultMappingBuilder with column names (works even if 0 rows)
        if (columns && columns.length > 0) {
            console.log('[DatabaseQueryTester] Column names from query:', columns);
            this.updateResultMappingColumns(columns);
        }

        if (rows.length === 0) {
            // Show column names with "Add to Mapping" buttons even when no rows returned
            let html = `
                <div class="no-results">
                    <i class="fas fa-info-circle"></i>
                    <p>Query executed successfully but returned 0 rows</p>
                    <small style="color: #6c757d;">No matching data found with test parameters. You can still configure field mappings below.</small>
            `;

            if (columns && columns.length > 0) {
                html += `
                    <div class="column-info">
                        <strong>Available Columns (${columns.length})</strong>
                        <div class="result-fields">
                `;

                columns.forEach(column => {
                    html += `
                        <div class="result-field">
                            <div class="field-info">
                                <span class="field-name"><i class="fas fa-database"></i> ${this.escapeHtml(column)}</span>
                                <span class="field-value"><em class="text-muted">No data available</em></span>
                            </div>
                            <button type="button"
                                    class="btn btn-sm btn-outline-primary add-mapping-btn"
                                    data-column="${this.escapeHtml(column)}"
                                    title="Add to Result Mapping">
                                <i class="fas fa-plus"></i> Add to Mapping
                            </button>
                        </div>
                    `;
                });

                html += `
                        </div>
                    </div>
                `;
            }

            html += '</div>';
            resultsContent.innerHTML = html;

            // Attach add mapping handlers
            this.attachAddMappingHandlers();
            return;
        }

        // Display first row with "Add to Mapping" buttons
        const firstRow = rows[0];
        let html = '<div class="query-result-row">';
        html += '<div class="result-fields">';

        Object.entries(firstRow).forEach(([column, value]) => {
            html += `
                <div class="result-field">
                    <div class="field-info">
                        <span class="field-name"><i class="fas fa-database"></i> ${this.escapeHtml(column)}</span>
                        <span class="field-value">${this.formatValue(value)}</span>
                    </div>
                    <button type="button"
                            class="btn btn-sm btn-outline-primary add-mapping-btn"
                            data-column="${this.escapeHtml(column)}"
                            title="Add to Result Mapping">
                        <i class="fas fa-plus"></i> Add to Mapping
                    </button>
                </div>
            `;
        });

        html += '</div></div>';

        // Show all rows as JSON (collapsed)
        if (rows.length > 1) {
            html += `
                <details class="all-rows-details">
                    <summary>
                        <i class="fas fa-chevron-right"></i>
                        View all ${rows.length} rows (JSON)
                    </summary>
                    <pre class="all-rows-json">${JSON.stringify(rows, null, 2)}</pre>
                </details>
            `;
        } else {
            html += `
                <details class="all-rows-details">
                    <summary>
                        <i class="fas fa-chevron-right"></i>
                        View as JSON
                    </summary>
                    <pre class="all-rows-json">${JSON.stringify(rows, null, 2)}</pre>
                </details>
            `;
        }

        // Add helpful tip
        html += `
            <div class="tester-tip">
                <i class="fas fa-lightbulb"></i>
                <strong>Tip:</strong> Click <span class="badge badge-primary"><i class="fas fa-plus"></i> Add to Mapping</span>
                to automatically add fields to the Result Mapping Builder above
            </div>
        `;

        resultsContent.innerHTML = html;

        // Attach add mapping handlers
        this.attachAddMappingHandlers();

        // Attach details toggle handler
        const details = this.container.querySelector('.all-rows-details');
        if (details) {
            details.addEventListener('toggle', (e) => {
                const icon = details.querySelector('i');
                if (details.open) {
                    icon.className = 'fas fa-chevron-down';
                } else {
                    icon.className = 'fas fa-chevron-right';
                }
            });
        }
    }

    attachAddMappingHandlers() {
        const addBtns = this.container.querySelectorAll('.add-mapping-btn');
        addBtns.forEach(btn => {
            btn.addEventListener('click', () => {
                const column = btn.dataset.column;
                this.addMapping(column, btn);
            });
        });
    }

    addMapping(dbColumn, btn) {
        if (this.onAddMappingCallback) {
            this.onAddMappingCallback(dbColumn);
        }

        // Visual feedback
        const originalContent = btn.innerHTML;
        btn.innerHTML = '<i class="fas fa-check"></i> Added';
        btn.classList.remove('btn-outline-primary');
        btn.classList.add('btn-success');
        btn.disabled = true;

        setTimeout(() => {
            btn.innerHTML = originalContent;
            btn.classList.remove('btn-success');
            btn.classList.add('btn-outline-primary');
            btn.disabled = false;
        }, 2000);
    }

    setOnAddMapping(callback) {
        this.onAddMappingCallback = callback;
    }

    displayError(errorMessage) {
        const errorSection = this.container.querySelector('.query-error-section');
        const errorMsg = this.container.querySelector('.error-message');

        errorSection.style.display = 'block';
        errorMsg.textContent = errorMessage;
    }

    clearResults() {
        this.container.querySelector('.query-results-section').style.display = 'none';
        this.container.querySelector('.query-error-section').style.display = 'none';
    }

    formatValue(value) {
        if (value === null) {
            return '<em class="null-value">null</em>';
        }
        if (typeof value === 'string') {
            return `<code>"${this.escapeHtml(value)}"</code>`;
        }
        if (typeof value === 'boolean') {
            return `<code class="boolean-value">${value}</code>`;
        }
        if (typeof value === 'number') {
            return `<code class="number-value">${value}</code>`;
        }
        if (value instanceof Date) {
            return `<code class="date-value">${value.toISOString()}</code>`;
        }
        return `<code>${this.escapeHtml(String(value))}</code>`;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    /**
     * Update ResultMappingBuilder with available database columns from query results
     * @param {Array<string>} columnNames - Array of column names from query result
     */
    updateResultMappingColumns(columnNames) {
        console.log('[DatabaseQueryTester] updateResultMappingColumns called with:', columnNames);

        // Find ResultMappingBuilder instance
        // It's stored in the properties panel form as a data attribute or global instance
        const form = document.querySelector('.properties-form') ||
                     document.getElementById('formTabContent');

        if (!form) {
            console.warn('[DatabaseQueryTester] Properties form not found');
            return;
        }

        // Find the Result Mapping section container
        const mappingContainer = form.querySelector('.result-mapping-builder-container') ||
                                form.querySelector('.result-mapping-builder');

        if (!mappingContainer) {
            console.warn('[DatabaseQueryTester] Result Mapping Builder container not found');
            return;
        }

        // Check if ResultMappingBuilder instance is stored on the container
        const mappingBuilder = mappingContainer._resultMappingBuilderInstance;

        if (mappingBuilder && typeof mappingBuilder.setAvailableColumns === 'function') {
            console.log('[DatabaseQueryTester] ✅ Found ResultMappingBuilder instance, updating columns');
            mappingBuilder.setAvailableColumns(columnNames);
        } else {
            console.warn('[DatabaseQueryTester] ResultMappingBuilder instance not found on container');
            console.log('[DatabaseQueryTester] Container:', mappingContainer);
            console.log('[DatabaseQueryTester] Instance:', mappingBuilder);
        }
    }

    /**
     * Update config (when user changes query/params in form)
     * @param {Object} config - Updated configuration
     */
    updateConfig(config) {
        this.config = config;
        this.renderTestParamsTable();
        this.clearResults();
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = DatabaseQueryTester;
}
