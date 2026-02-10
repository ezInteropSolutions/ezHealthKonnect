/**
 * API Endpoint Tester Component
 *
 * Allows users to test API endpoints before configuring response mapping.
 * Shows actual request/response and provides visual field picker for mapping.
 */

class APIEndpointTester {
    constructor(containerIdOrElement) {
        // Accept either an ID string or a DOM element
        if (typeof containerIdOrElement === 'string') {
            this.container = document.getElementById(containerIdOrElement);
        } else {
            this.container = containerIdOrElement;
        }
        this.testResults = null;
        this.onAddMappingRule = null; // Callback for when user clicks "Add Field"
    }

    /**
     * Render the API endpoint tester UI
     * @param {Object|Function} stepConfigOrGetter - Current step config or function that returns it
     */
    render(stepConfigOrGetter) {
        if (!this.container) {
            console.error('Container not found for APIEndpointTester');
            return;
        }

        // Store the config getter function for later use
        if (typeof stepConfigOrGetter === 'function') {
            this.getStepConfig = stepConfigOrGetter;
        } else {
            this.getStepConfig = () => stepConfigOrGetter;
        }

        this.container.innerHTML = `
            <div class="api-endpoint-tester">
                <div class="test-section">
                    <h4>🧪 Test API Endpoint</h4>
                    <p class="help-text">
                        Test your API configuration to see the actual response
                        before configuring field mappings.
                    </p>

                    <div class="test-data-section">
                        <label for="test-data-input">Sample Message Data (JSON)</label>
                        <textarea
                            id="test-data-input"
                            rows="6"
                            placeholder='{
  "PID.3": "12345",
  "PID.5": "John^Doe",
  "MSH.9": "ADT^A01"
}'></textarea>
                        <small>Provide sample data to resolve field mappings like {patientId}</small>
                    </div>

                    <button id="test-api-btn" class="btn btn-primary">
                        🧪 Test API Endpoint
                    </button>

                    <div id="test-results-container" style="display:none;">
                        <!-- Results will be shown here -->
                    </div>
                </div>

                <div class="field-picker-section" style="display:none;">
                    <h4>📋 Response Fields (Click to Add to Mapping)</h4>
                    <div id="field-picker-list"></div>
                </div>
            </div>
        `;

        this.attachEventListeners();
    }

    /**
     * Attach event listeners
     */
    attachEventListeners() {
        const testBtn = this.container.querySelector('#test-api-btn');
        if (testBtn) {
            testBtn.addEventListener('click', () => {
                // Get fresh config when button is clicked
                const stepConfig = this.getStepConfig();
                console.log('🧪 Testing API with config:', stepConfig);
                this.testAPIEndpoint(stepConfig);
            });
        } else {
            console.error('Test API button not found in container');
        }
    }

    /**
     * Test the API endpoint with sample data
     */
    async testAPIEndpoint(stepConfig) {
        const testDataInputEl = this.container.querySelector('#test-data-input');
        const testDataInput = testDataInputEl ? testDataInputEl.value : '';
        let testData = {};

        // Parse test data JSON
        try {
            testData = testDataInput ? JSON.parse(testDataInput) : {};
        } catch (e) {
            this.showNotification('Invalid test data JSON: ' + e.message, 'error');
            return;
        }

        const btn = this.container.querySelector('#test-api-btn');
        if (!btn) {
            console.error('Test button not found');
            return;
        }
        btn.disabled = true;
        btn.textContent = '⏳ Testing...';

        try {
            const response = await fetch('/api/fhir/pipeline/test-api-endpoint', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    stepConfig: stepConfig,
                    testData: testData
                })
            });

            const result = await response.json();
            this.testResults = result;
            this.displayTestResults(result);

        } catch (error) {
            console.error('API test failed:', error);
            this.showNotification('API test failed: ' + error.message, 'error');
        } finally {
            btn.disabled = false;
            btn.textContent = '🧪 Test API Endpoint';
        }
    }

    /**
     * Show in-app notification
     */
    showNotification(message, type = 'info') {
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
            max-width: 400px;
        `;
        notification.textContent = message;
        document.body.appendChild(notification);
        setTimeout(() => notification.remove(), 4000);
    }

    /**
     * Display test results in UI
     */
    displayTestResults(result) {
        const container = this.container.querySelector('#test-results-container');
        if (!container) return;

        container.style.display = 'block';

        if (!result.success) {
            container.innerHTML = `
                <div class="alert alert-error">
                    <strong>❌ API Call Failed</strong>
                    <p>${result.error || 'Unknown error'}</p>
                    ${result.request ? `
                        <details>
                            <summary>Request Details</summary>
                            <pre>${JSON.stringify(result.request, null, 2)}</pre>
                        </details>
                    ` : ''}
                </div>
            `;
            return;
        }

        const response = result.response;

        container.innerHTML = `
            <div class="alert alert-success">
                <strong>✅ API Call Successful</strong>
                <p>Status: ${response.status_code} ${response.status_text} (${response.duration_ms}ms)</p>
            </div>

            <div class="tabs">
                <button class="tab-btn active" data-tab="parsed">Parsed Response</button>
                <button class="tab-btn" data-tab="raw">Raw Response</button>
                <button class="tab-btn" data-tab="request">Request Details</button>
            </div>

            <div class="tab-content active" id="tab-parsed">
                <div class="json-viewer">
                    <pre>${JSON.stringify(response.body_parsed, null, 2)}</pre>
                </div>
            </div>

            <div class="tab-content" id="tab-raw">
                <pre>${response.body_raw || 'No raw response body'}</pre>
            </div>

            <div class="tab-content" id="tab-request">
                <h5>Request</h5>
                <pre>${JSON.stringify(result.request, null, 2)}</pre>
            </div>
        `;

        // Show field picker if field structure available
        if (response.field_structure && response.field_structure.length > 0) {
            this.displayFieldPicker(response.field_structure);
        }

        // Attach tab switching
        this.attachTabListeners();
    }

    /**
     * Display field picker with available fields from response
     */
    displayFieldPicker(fields) {
        const pickerSection = this.container.querySelector('.field-picker-section');
        if (!pickerSection) return;

        pickerSection.style.display = 'block';

        const pickerList = this.container.querySelector('#field-picker-list');
        if (!pickerList) return;

        pickerList.innerHTML = `
            <div class="field-list">
                ${fields.map(field => `
                    <div class="field-item" data-path="${this.escapeHtml(field.path)}">
                        <div class="field-info">
                            <code class="field-path">${this.escapeHtml(field.path)}</code>
                            <span class="field-type badge badge-${field.type}">${field.type}</span>
                        </div>
                        <div class="field-sample">${this.formatSample(field.sample)}</div>
                        <button class="btn-add-field" data-path="${this.escapeHtml(field.path)}" data-type="${field.type}">
                            + Add to Mapping
                        </button>
                    </div>
                `).join('')}
            </div>
        `;

        // Attach field picker click handlers
        pickerList.querySelectorAll('.btn-add-field').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const path = e.target.dataset.path;
                const type = e.target.dataset.type;
                this.addFieldToMapping(path, type, e.target);
            });
        });
    }

    /**
     * Add a field to the response mapping configuration
     */
    addFieldToMapping(jsonPath, fieldType, buttonElement) {
        const targetField = this.suggestTargetField(jsonPath);

        // Emit event to parent component to add mapping rule
        if (this.onAddMappingRule && typeof this.onAddMappingRule === 'function') {
            this.onAddMappingRule({
                sourcePath: jsonPath,
                targetField: targetField,
                transformType: 'none',
                fieldType: fieldType
            });
        } else {
            // Fallback: dispatch custom event
            const event = new CustomEvent('add-mapping-rule', {
                detail: {
                    sourcePath: jsonPath,
                    targetField: targetField,
                    transformType: 'none',
                    fieldType: fieldType
                }
            });
            this.container.dispatchEvent(event);
        }

        // Visual feedback
        if (buttonElement) {
            buttonElement.textContent = '✓ Added';
            buttonElement.disabled = true;
            buttonElement.classList.add('btn-added');

            setTimeout(() => {
                buttonElement.textContent = '+ Add to Mapping';
                buttonElement.disabled = false;
                buttonElement.classList.remove('btn-added');
            }, 2000);
        }
    }

    /**
     * Suggest a target field name from JSONPath
     * $.patient.id → patientId
     * $.data.insurance[0].memberId → dataInsuranceMemberId
     */
    suggestTargetField(jsonPath) {
        // Extract segments from JSONPath
        const segments = jsonPath
            .replace(/\$\./g, '')
            .replace(/\[0\]/g, '')
            .replace(/\[\d+\]/g, '')
            .split('.');

        if (segments.length === 1) {
            return segments[0];
        }

        // camelCase: patient.id → patientId
        return segments[0] + segments.slice(1).map(s =>
            s.charAt(0).toUpperCase() + s.slice(1)
        ).join('');
    }

    /**
     * Format sample value for display
     */
    formatSample(sample) {
        if (typeof sample === 'object') {
            return this.escapeHtml(JSON.stringify(sample));
        }
        return this.escapeHtml(String(sample));
    }

    /**
     * Attach tab switching listeners
     */
    attachTabListeners() {
        this.container.querySelectorAll('.tab-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const tabName = e.target.dataset.tab;

                // Update active tab button
                this.container.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
                e.target.classList.add('active');

                // Update active tab content
                this.container.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
                const tabContent = this.container.querySelector(`#tab-${tabName}`);
                if (tabContent) {
                    tabContent.classList.add('active');
                }
            });
        });
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
     * Set callback for when user adds a mapping rule
     */
    setOnAddMappingRule(callback) {
        this.onAddMappingRule = callback;
    }

    /**
     * Get current step config from UI inputs
     */
    getCurrentStepConfig() {
        // This should be overridden by parent component
        // or passed as parameter to render()
        return {};
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = APIEndpointTester;
}
