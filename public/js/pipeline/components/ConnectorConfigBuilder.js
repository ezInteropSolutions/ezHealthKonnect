/**
 * ConnectorConfigBuilder - OOP builder for inbound/outbound connector step configuration
 *
 * Extends BaseStepConfigBuilder (Template Method pattern).
 * Single builder for both inbound and outbound connectors - the `direction` parameter
 * controls which connector types are shown.
 *
 * Features:
 * - Fetches connector types from API (data-driven, not hardcoded)
 * - Renders config fields dynamically from connector's config_schema (JSON Schema)
 * - Groups fields by parameter_groups (basic, security, advanced)
 * - Embeds OAuth2ConfigBuilder for connectors requiring auth
 * - Embeds HeaderBuilder for HTTP connectors
 * - Test Connection button
 *
 * Dependencies:
 * - BaseStepConfigBuilder (abstract base class)
 * - ConfigUtils, DOMUtils (utilities)
 * - window.pipelineAPI (PipelineAPIService singleton)
 * - OAuth2ConfigBuilder (optional, embedded for auth)
 * - HeaderBuilder (optional, embedded for HTTP headers)
 *
 * @class ConnectorConfigBuilder
 * @extends BaseStepConfigBuilder
 */
class ConnectorConfigBuilder extends BaseStepConfigBuilder {
    /**
     * @param {HTMLElement} container - DOM container for rendering
     * @param {Object} initialConfig - Initial step configuration
     * @param {string} direction - 'inbound' or 'outbound'
     */
    constructor(container, initialConfig = {}, direction = 'outbound') {
        super(container, initialConfig);
        this.direction = direction;
        this.connectorTypes = [];
        this.selectedType = null;
        this.configSchema = null;
        this.parameterGroups = null;
        this.embeddedBuilders = {};
    }

    // ========================================
    // ABSTRACT METHOD IMPLEMENTATIONS
    // ========================================

    getDefaultConfig() {
        return {
            connectorType: '',
            config: {},
            contentField: this.direction === 'outbound' ? 'transformed' : '',
            contentType: 'application/json',
            outputField: 'enriched.connector_result',
            timeoutMs: 30000
        };
    }

    render() {
        this.clear();
        this.container.classList.add('connector-config-builder');

        // Header
        const header = this.createElement('div', { class: 'connector-builder-header' }, `
            <div class="connector-builder-title">
                <i class="fas fa-${this.direction === 'inbound' ? 'download' : 'upload'}"></i>
                <h4>${this.direction === 'inbound' ? 'Inbound' : 'Outbound'} Connector Configuration</h4>
            </div>
            <small class="text-muted">Configure the ${this.direction} connector for this step</small>
        `);
        this.container.appendChild(header);

        // Connector type selector (loading state initially)
        const typeSection = this.createElement('div', { class: 'connector-type-section' });
        typeSection.innerHTML = `
            <div class="form-group">
                <label>Connector Type <span class="text-danger">*</span></label>
                <select class="form-control connector-type-select" id="connectorTypeSelect">
                    <option value="">Loading connector types...</option>
                </select>
                <small class="form-text text-muted connector-type-description"></small>
            </div>
        `;
        this.container.appendChild(typeSection);

        // Dynamic config section (populated when connector type is selected)
        const configSection = this.createElement('div', {
            class: 'connector-dynamic-config',
            id: 'connectorDynamicConfig'
        });
        this.container.appendChild(configSection);

        // Direction-specific fields
        const extraFields = this.createElement('div', { class: 'connector-extra-fields' });
        if (this.direction === 'outbound') {
            extraFields.innerHTML = `
                <div class="form-group">
                    <label>Content Field</label>
                    <input type="text" class="form-control connector-content-field"
                           value="${this.escapeHtml(this.config.contentField || 'transformed')}"
                           placeholder="e.g., transformed, fhirBundle">
                    <small class="form-text text-muted">Which field from the pipeline data to send</small>
                </div>
                <div class="form-group">
                    <label>Content Type</label>
                    <select class="form-control connector-content-type">
                        <option value="application/json" ${this.config.contentType === 'application/json' ? 'selected' : ''}>application/json</option>
                        <option value="application/fhir+json" ${this.config.contentType === 'application/fhir+json' ? 'selected' : ''}>application/fhir+json</option>
                        <option value="text/plain" ${this.config.contentType === 'text/plain' ? 'selected' : ''}>text/plain</option>
                        <option value="application/hl7-v2" ${this.config.contentType === 'application/hl7-v2' ? 'selected' : ''}>application/hl7-v2</option>
                    </select>
                </div>
            `;
        } else {
            extraFields.innerHTML = `
                <div class="form-group">
                    <label>Output Field</label>
                    <input type="text" class="form-control connector-output-field"
                           value="${this.escapeHtml(this.config.outputField || 'enriched.connector_result')}"
                           placeholder="e.g., enriched.connector_result">
                    <small class="form-text text-muted">Where to store fetched data in the pipeline</small>
                </div>
                <div class="form-group">
                    <label>Timeout (ms)</label>
                    <input type="number" class="form-control connector-timeout"
                           value="${this.config.timeoutMs || 30000}"
                           min="1000" max="300000" step="1000">
                    <small class="form-text text-muted">Maximum wait time for data fetch</small>
                </div>
            `;
        }
        this.container.appendChild(extraFields);

        // Test connection button
        const testSection = this.createElement('div', { class: 'connector-test-section' });
        testSection.innerHTML = `
            <button type="button" class="btn btn-outline-info btn-sm test-connection-btn" disabled>
                <i class="fas fa-plug"></i> Test Connection
            </button>
            <span class="test-connection-status"></span>
        `;
        this.container.appendChild(testSection);

        // Load connector types from API
        this.loadConnectorTypes();
    }

    attachEvents() {
        // Connector type dropdown change
        const typeSelect = this.container.querySelector('.connector-type-select');
        if (typeSelect) {
            typeSelect.addEventListener('change', (e) => {
                this.onConnectorTypeChange(e.target.value);
                this.onChange();
            });
        }

        // Extra fields
        const contentField = this.container.querySelector('.connector-content-field');
        if (contentField) {
            contentField.addEventListener('input', () => this.onChange());
        }

        const contentType = this.container.querySelector('.connector-content-type');
        if (contentType) {
            contentType.addEventListener('change', () => this.onChange());
        }

        const outputField = this.container.querySelector('.connector-output-field');
        if (outputField) {
            outputField.addEventListener('input', () => this.onChange());
        }

        const timeoutField = this.container.querySelector('.connector-timeout');
        if (timeoutField) {
            timeoutField.addEventListener('input', () => this.onChange());
        }

        // Test connection button
        const testBtn = this.container.querySelector('.test-connection-btn');
        if (testBtn) {
            testBtn.addEventListener('click', () => this.testConnection());
        }
    }

    // ========================================
    // HOOK METHODS
    // ========================================

    addStyles() {
        if (document.getElementById('connector-config-builder-styles')) return;

        const style = document.createElement('style');
        style.id = 'connector-config-builder-styles';
        style.textContent = `
            .connector-config-builder {
                padding: 0;
            }

            .connector-builder-header {
                margin-bottom: 16px;
            }

            .connector-builder-title {
                display: flex;
                align-items: center;
                gap: 8px;
                margin-bottom: 4px;
            }

            .connector-builder-title h4 {
                margin: 0;
                font-size: 15px;
                font-weight: 600;
                color: #1e293b;
            }

            .connector-builder-title i {
                color: #6366f1;
                font-size: 16px;
            }

            .connector-type-section {
                margin-bottom: 16px;
            }

            .connector-type-section .connector-type-select {
                font-weight: 500;
            }

            .connector-type-description {
                margin-top: 4px;
                font-style: italic;
            }

            .connector-dynamic-config {
                margin-bottom: 16px;
            }

            .connector-config-group {
                background: #f8fafc;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
                margin-bottom: 12px;
                overflow: hidden;
            }

            .connector-config-group-header {
                display: flex;
                align-items: center;
                justify-content: space-between;
                padding: 10px 14px;
                background: #f1f5f9;
                cursor: pointer;
                user-select: none;
                border-bottom: 1px solid #e2e8f0;
            }

            .connector-config-group-header:hover {
                background: #e2e8f0;
            }

            .connector-config-group-title {
                font-weight: 600;
                font-size: 13px;
                color: #334155;
                text-transform: capitalize;
            }

            .connector-config-group-toggle {
                color: #94a3b8;
                font-size: 12px;
                transition: transform 0.2s;
            }

            .connector-config-group.collapsed .connector-config-group-toggle {
                transform: rotate(-90deg);
            }

            .connector-config-group-body {
                padding: 12px 14px;
            }

            .connector-config-group.collapsed .connector-config-group-body {
                display: none;
            }

            .connector-config-group .form-group {
                margin-bottom: 12px;
            }

            .connector-config-group .form-group:last-child {
                margin-bottom: 0;
            }

            .connector-config-group .form-group label {
                font-weight: 500;
                font-size: 13px;
                color: #475569;
                margin-bottom: 4px;
                display: block;
            }

            .connector-extra-fields {
                margin-bottom: 16px;
                padding: 12px 14px;
                background: #f8fafc;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
            }

            .connector-extra-fields .form-group {
                margin-bottom: 12px;
            }

            .connector-extra-fields .form-group:last-child {
                margin-bottom: 0;
            }

            .connector-test-section {
                display: flex;
                align-items: center;
                gap: 12px;
                padding-top: 8px;
            }

            .test-connection-btn {
                font-size: 13px;
            }

            .test-connection-status {
                font-size: 13px;
            }

            .connector-no-config {
                color: #94a3b8;
                font-style: italic;
                padding: 12px;
                text-align: center;
            }

            .connector-loading {
                display: flex;
                align-items: center;
                gap: 8px;
                padding: 12px;
                color: #64748b;
                font-size: 13px;
            }

            .connector-embedded-builder {
                margin-top: 12px;
                border: 1px solid #c7d2fe;
                border-radius: 8px;
                padding: 12px;
                background: #eef2ff;
            }
        `;
        document.head.appendChild(style);
    }

    // ========================================
    // CONNECTOR TYPE MANAGEMENT
    // ========================================

    async loadConnectorTypes() {
        const typeSelect = this.container.querySelector('.connector-type-select');
        if (!typeSelect) return;

        try {
            const response = await window.pipelineAPI.getConnectorTypes(this.direction);
            const types = response.data || response || [];
            this.connectorTypes = types;

            // Build dropdown options
            typeSelect.innerHTML = '<option value="">-- Select Connector Type --</option>';
            types.forEach(ct => {
                const option = document.createElement('option');
                option.value = ct.type_name;
                option.textContent = `${ct.icon || ''} ${ct.display_name}`;
                if (ct.is_beta) option.textContent += ' (Beta)';
                if (ct.type_name === this.config.connectorType) {
                    option.selected = true;
                }
                typeSelect.appendChild(option);
            });

            // If we have a pre-selected connector type, load its config
            if (this.config.connectorType) {
                this.onConnectorTypeChange(this.config.connectorType);
            }

            // Enable test button if type is selected
            this.updateTestButton();

            console.log(`[ConnectorConfigBuilder] Loaded ${types.length} ${this.direction} connector types`);
        } catch (error) {
            console.error('[ConnectorConfigBuilder] Failed to load connector types:', error);
            typeSelect.innerHTML = '<option value="">Failed to load connector types</option>';
        }
    }

    onConnectorTypeChange(typeName) {
        this.config.connectorType = typeName;
        const configContainer = this.container.querySelector('#connectorDynamicConfig');
        if (!configContainer) return;

        // Destroy embedded builders
        Object.values(this.embeddedBuilders).forEach(b => {
            if (b && typeof b.destroy === 'function') b.destroy();
        });
        this.embeddedBuilders = {};

        if (!typeName) {
            configContainer.innerHTML = '<div class="connector-no-config">Select a connector type to configure</div>';
            this.selectedType = null;
            this.configSchema = null;
            this.parameterGroups = null;
            this.updateTestButton();
            this.updateTypeDescription('');
            return;
        }

        // Find the selected type data
        this.selectedType = this.connectorTypes.find(ct => ct.type_name === typeName);
        if (!this.selectedType) {
            configContainer.innerHTML = '<div class="connector-no-config">Connector type not found</div>';
            this.updateTestButton();
            return;
        }

        this.configSchema = this.parseJSON(this.selectedType.config_schema);
        this.parameterGroups = this.parseJSON(this.selectedType.parameter_groups);

        this.updateTypeDescription(this.selectedType.description || '');
        this.renderDynamicConfig(configContainer);
        this.updateTestButton();
    }

    // ========================================
    // DYNAMIC CONFIG RENDERING (Schema-Driven)
    // ========================================

    renderDynamicConfig(container) {
        container.innerHTML = '';

        if (!this.configSchema || !this.configSchema.properties) {
            container.innerHTML = '<div class="connector-no-config">No configuration required for this connector</div>';
            return;
        }

        const properties = this.configSchema.properties;
        const required = this.configSchema.required || [];
        const existingConfig = this.config.config || {};

        // Build dependency map: boolean toggle fields control related fields' visibility
        // e.g., enable_tls → [tls_cert_path, tls_key_path], enable_authentication → [authentication_method]
        this.fieldDependencies = this.buildFieldDependencies(properties);

        // If parameter_groups is defined, render grouped
        if (this.parameterGroups && Object.keys(this.parameterGroups).length > 0) {
            const renderedFields = new Set();

            for (const [groupName, fieldNames] of Object.entries(this.parameterGroups)) {
                if (!Array.isArray(fieldNames) || fieldNames.length === 0) continue;

                const group = this.createElement('div', { class: 'connector-config-group' });

                // Group header (collapsible)
                const groupHeader = this.createElement('div', { class: 'connector-config-group-header' }, `
                    <span class="connector-config-group-title">${this.formatGroupName(groupName)}</span>
                    <i class="fas fa-chevron-down connector-config-group-toggle"></i>
                `);
                group.appendChild(groupHeader);

                // Group body
                const groupBody = this.createElement('div', { class: 'connector-config-group-body' });

                fieldNames.forEach(fieldName => {
                    if (properties[fieldName]) {
                        const fieldEl = this.renderSchemaField(fieldName, properties[fieldName], required.includes(fieldName), existingConfig[fieldName]);
                        // Mark dependent fields as hidden initially if their toggle is off
                        if (this.isDependentField(fieldName) && !this.isToggleEnabled(fieldName, existingConfig)) {
                            fieldEl.style.display = 'none';
                            fieldEl.dataset.dependentField = 'true';
                        }
                        groupBody.appendChild(fieldEl);
                        renderedFields.add(fieldName);
                    }
                });

                group.appendChild(groupBody);
                container.appendChild(group);

                // Toggle collapse
                groupHeader.addEventListener('click', () => {
                    group.classList.toggle('collapsed');
                });

                // Collapse non-basic groups by default if no values set
                if (groupName !== 'basic') {
                    const hasValues = fieldNames.some(fn => existingConfig[fn] !== undefined && existingConfig[fn] !== '' && existingConfig[fn] !== false);
                    if (!hasValues) {
                        group.classList.add('collapsed');
                    }
                }
            }

            // Render any ungrouped fields
            for (const [fieldName, schema] of Object.entries(properties)) {
                if (!renderedFields.has(fieldName)) {
                    const fieldEl = this.renderSchemaField(fieldName, schema, required.includes(fieldName), existingConfig[fieldName]);
                    if (this.isDependentField(fieldName) && !this.isToggleEnabled(fieldName, existingConfig)) {
                        fieldEl.style.display = 'none';
                        fieldEl.dataset.dependentField = 'true';
                    }
                    container.appendChild(fieldEl);
                }
            }
        } else {
            // No groups - render all fields flat
            for (const [fieldName, schema] of Object.entries(properties)) {
                const fieldEl = this.renderSchemaField(fieldName, schema, required.includes(fieldName), existingConfig[fieldName]);
                if (this.isDependentField(fieldName) && !this.isToggleEnabled(fieldName, existingConfig)) {
                    fieldEl.style.display = 'none';
                    fieldEl.dataset.dependentField = 'true';
                }
                container.appendChild(fieldEl);
            }
        }

        // Attach toggle listeners for conditional visibility
        this.attachToggleListeners(container);

        // Add embedded builders for special types (conditionally)
        this.addEmbeddedBuilders(container, existingConfig);
    }

    // ========================================
    // CONDITIONAL FIELD VISIBILITY
    // ========================================

    /**
     * Build a map of toggle fields to their dependent fields.
     * Convention: boolean fields named `enable_*` control related fields in the same group.
     * E.g., enable_tls → [tls_cert_path, tls_key_path], enable_authentication → [authentication_method]
     */
    buildFieldDependencies(properties) {
        const deps = {};
        const toggleFields = Object.keys(properties).filter(name =>
            properties[name].type === 'boolean' && name.startsWith('enable_')
        );

        for (const toggle of toggleFields) {
            // Extract the feature name: enable_tls → tls, enable_authentication → authentication
            const feature = toggle.replace('enable_', '');
            // Find fields whose names contain this feature (but aren't the toggle itself)
            const dependents = Object.keys(properties).filter(name =>
                name !== toggle && (name.includes(feature) || name.startsWith(feature + '_'))
            );
            if (dependents.length > 0) {
                deps[toggle] = dependents;
            }
        }

        return deps;
    }

    isDependentField(fieldName) {
        if (!this.fieldDependencies) return false;
        return Object.values(this.fieldDependencies).some(deps => deps.includes(fieldName));
    }

    isToggleEnabled(fieldName, config) {
        if (!this.fieldDependencies) return true;
        for (const [toggle, deps] of Object.entries(this.fieldDependencies)) {
            if (deps.includes(fieldName)) {
                return config[toggle] === true || config[toggle] === 'true';
            }
        }
        return true;
    }

    attachToggleListeners(container) {
        if (!this.fieldDependencies) return;

        for (const [toggleField, dependentFields] of Object.entries(this.fieldDependencies)) {
            const toggleInput = container.querySelector(`.connector-config-field[data-field="${toggleField}"]`);
            if (!toggleInput) continue;

            toggleInput.addEventListener('change', () => {
                const isEnabled = toggleInput.checked;
                dependentFields.forEach(depField => {
                    const depEl = container.querySelector(`.connector-config-field[data-field="${depField}"]`);
                    if (depEl) {
                        const formGroup = depEl.closest('.form-group') || depEl.closest('.form-check')?.parentElement;
                        if (formGroup) {
                            formGroup.style.display = isEnabled ? '' : 'none';
                        }
                    }
                });
            });
        }
    }

    renderSchemaField(fieldName, schema, isRequired, currentValue) {
        const formGroup = this.createElement('div', { class: 'form-group' });
        const title = schema.title || this.formatFieldName(fieldName);
        const value = currentValue !== undefined ? currentValue : (schema.default !== undefined ? schema.default : '');

        // Label
        const label = document.createElement('label');
        label.innerHTML = `${this.escapeHtml(title)}${isRequired ? ' <span class="text-danger">*</span>' : ''}`;
        formGroup.appendChild(label);

        let inputEl;

        if (schema.enum) {
            // Select dropdown
            inputEl = document.createElement('select');
            inputEl.className = 'form-control form-control-sm connector-config-field';
            inputEl.dataset.field = fieldName;

            const emptyOpt = document.createElement('option');
            emptyOpt.value = '';
            emptyOpt.textContent = `-- Select ${title} --`;
            inputEl.appendChild(emptyOpt);

            schema.enum.forEach(enumVal => {
                const opt = document.createElement('option');
                opt.value = enumVal;
                opt.textContent = enumVal;
                if (String(value) === String(enumVal)) opt.selected = true;
                inputEl.appendChild(opt);
            });
        } else if (schema.type === 'boolean') {
            // Checkbox wrapper
            const checkWrapper = this.createElement('div', { class: 'form-check' });
            inputEl = document.createElement('input');
            inputEl.type = 'checkbox';
            inputEl.className = 'form-check-input connector-config-field';
            inputEl.dataset.field = fieldName;
            inputEl.dataset.fieldType = 'boolean';
            inputEl.checked = value === true || value === 'true';
            inputEl.id = `connector-field-${fieldName}`;

            const checkLabel = document.createElement('label');
            checkLabel.className = 'form-check-label';
            checkLabel.htmlFor = inputEl.id;
            checkLabel.textContent = title;

            checkWrapper.appendChild(inputEl);
            checkWrapper.appendChild(checkLabel);

            // For boolean, replace the existing label with the checkbox wrapper
            formGroup.innerHTML = '';
            formGroup.appendChild(checkWrapper);
            inputEl.addEventListener('change', () => this.onChange());
            return formGroup;
        } else if (schema.type === 'integer' || schema.type === 'number') {
            inputEl = document.createElement('input');
            inputEl.type = 'number';
            inputEl.className = 'form-control form-control-sm connector-config-field';
            inputEl.dataset.field = fieldName;
            inputEl.dataset.fieldType = 'number';
            inputEl.value = value;
            if (schema.minimum !== undefined) inputEl.min = schema.minimum;
            if (schema.maximum !== undefined) inputEl.max = schema.maximum;
            inputEl.placeholder = schema.default !== undefined ? `Default: ${schema.default}` : '';
        } else if (schema.type === 'array') {
            // Render as comma-separated text input
            inputEl = document.createElement('input');
            inputEl.type = 'text';
            inputEl.className = 'form-control form-control-sm connector-config-field';
            inputEl.dataset.field = fieldName;
            inputEl.dataset.fieldType = 'array';
            inputEl.value = Array.isArray(value) ? value.join(', ') : (value || '');
            inputEl.placeholder = schema.items?.enum
                ? `Options: ${schema.items.enum.join(', ')}`
                : 'Comma-separated values';
        } else {
            // Default: text input (or password for sensitive fields)
            inputEl = document.createElement('input');
            const isSensitive = fieldName.toLowerCase().includes('password') ||
                                fieldName.toLowerCase().includes('secret') ||
                                fieldName.toLowerCase().includes('key') && !fieldName.toLowerCase().includes('key_path');
            inputEl.type = isSensitive ? 'password' : 'text';
            inputEl.className = 'form-control form-control-sm connector-config-field';
            inputEl.dataset.field = fieldName;
            inputEl.value = value;
            inputEl.placeholder = schema.default !== undefined ? `Default: ${schema.default}` : '';
        }

        if (inputEl) {
            formGroup.appendChild(inputEl);
            inputEl.addEventListener('input', () => this.onChange());
            inputEl.addEventListener('change', () => this.onChange());
        }

        return formGroup;
    }

    addEmbeddedBuilders(container, existingConfig = {}) {
        if (!this.selectedType) return;

        const typeName = this.selectedType.type_name || '';
        const isHTTP = typeName.includes('http');
        const authType = existingConfig.authentication_type || '';

        // OAuth2 only for HTTP connectors when authentication_type is 'oauth2'
        if (isHTTP && typeof OAuth2ConfigBuilder !== 'undefined') {
            const authSection = this.createElement('div', {
                class: 'connector-embedded-builder connector-oauth2-section',
                id: 'connectorOAuth2Section'
            });
            authSection.innerHTML = '<h5 style="margin: 0 0 8px; font-size: 13px; font-weight: 600;">Authentication</h5>';
            const authContainer = this.createElement('div', {});
            authSection.appendChild(authContainer);
            container.appendChild(authSection);

            // Only show if authentication_type is oauth2
            if (authType !== 'oauth2') {
                authSection.style.display = 'none';
            }

            const authBuilder = new OAuth2ConfigBuilder(authContainer, existingConfig.oauth2 || {});
            this.embeddedBuilders.oauth2 = authBuilder;

            // Listen for authentication_type changes to show/hide OAuth2
            const authTypeField = container.querySelector('.connector-config-field[data-field="authentication_type"]');
            if (authTypeField) {
                authTypeField.addEventListener('change', () => {
                    authSection.style.display = authTypeField.value === 'oauth2' ? '' : 'none';
                });
            }
        }

        // Headers only for HTTP connectors
        if (isHTTP && typeof HeaderBuilder !== 'undefined') {
            const headersSection = this.createElement('div', { class: 'connector-embedded-builder' });
            headersSection.innerHTML = '<h5 style="margin: 0 0 8px; font-size: 13px; font-weight: 600;">Custom Headers</h5>';
            const headersContainer = this.createElement('div', {});
            headersSection.appendChild(headersContainer);
            container.appendChild(headersSection);

            const headerBuilder = new HeaderBuilder(headersContainer, existingConfig.headers || {});
            this.embeddedBuilders.headers = headerBuilder;
        }
    }

    // ========================================
    // PUBLIC API OVERRIDES
    // ========================================

    getConfig() {
        // Gather config from dynamic fields
        const config = {};
        const fields = this.container.querySelectorAll('.connector-config-field');
        fields.forEach(field => {
            const fieldName = field.dataset.field;
            if (!fieldName) return;

            let value;
            if (field.dataset.fieldType === 'boolean') {
                value = field.checked;
            } else if (field.dataset.fieldType === 'number') {
                value = field.value ? Number(field.value) : undefined;
            } else if (field.dataset.fieldType === 'array') {
                value = field.value ? field.value.split(',').map(v => v.trim()).filter(v => v) : [];
            } else {
                value = field.value.trim();
            }

            if (value !== undefined && value !== '') {
                config[fieldName] = value;
            }
        });

        // Merge embedded builder configs
        if (this.embeddedBuilders.oauth2) {
            config.oauth2 = this.embeddedBuilders.oauth2.getConfig();
        }
        if (this.embeddedBuilders.headers) {
            const headerConfig = this.embeddedBuilders.headers.getConfig
                ? this.embeddedBuilders.headers.getConfig()
                : this.embeddedBuilders.headers.getHeaders
                    ? this.embeddedBuilders.headers.getHeaders()
                    : {};
            config.headers = headerConfig;
        }

        // Build final step config
        const stepConfig = {
            connectorType: this.config.connectorType || '',
            config: config
        };

        // Direction-specific fields
        if (this.direction === 'outbound') {
            const contentField = this.container.querySelector('.connector-content-field');
            const contentType = this.container.querySelector('.connector-content-type');
            stepConfig.contentField = contentField ? contentField.value.trim() : 'transformed';
            stepConfig.contentType = contentType ? contentType.value : 'application/json';
        } else {
            const outputField = this.container.querySelector('.connector-output-field');
            const timeoutField = this.container.querySelector('.connector-timeout');
            stepConfig.outputField = outputField ? outputField.value.trim() : 'enriched.connector_result';
            stepConfig.timeoutMs = timeoutField ? Number(timeoutField.value) || 30000 : 30000;
        }

        this.config = ConfigUtils.mergeConfig(this.config, stepConfig);
        return this.config;
    }

    validate() {
        const errors = [];

        if (!this.config.connectorType) {
            errors.push('Connector type is required');
        }

        // Check required fields from schema
        if (this.configSchema && this.configSchema.required) {
            const currentConfig = this.config.config || {};
            this.configSchema.required.forEach(field => {
                if (!currentConfig[field] && currentConfig[field] !== 0 && currentConfig[field] !== false) {
                    const title = this.configSchema.properties?.[field]?.title || field;
                    errors.push(`${title} is required`);
                }
            });
        }

        return {
            valid: errors.length === 0,
            errors: errors
        };
    }

    destroy() {
        // Destroy embedded builders first
        Object.values(this.embeddedBuilders).forEach(b => {
            if (b && typeof b.destroy === 'function') b.destroy();
        });
        this.embeddedBuilders = {};
        super.destroy();
    }

    // ========================================
    // TEST CONNECTION
    // ========================================

    async testConnection() {
        const testBtn = this.container.querySelector('.test-connection-btn');
        const statusEl = this.container.querySelector('.test-connection-status');
        if (!testBtn || !statusEl) return;

        const currentConfig = this.getConfig();
        if (!currentConfig.connectorType) {
            statusEl.innerHTML = '<span style="color: #ef4444;">Select a connector type first</span>';
            return;
        }

        const originalHTML = testBtn.innerHTML;
        testBtn.disabled = true;
        testBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Testing...';
        statusEl.innerHTML = '';

        try {
            const response = await window.pipelineAPI.request('POST', '/connectivity/test', {
                connector_type: currentConfig.connectorType,
                config: currentConfig.config
            });

            if (response.success) {
                testBtn.innerHTML = '<i class="fas fa-check-circle"></i> Connected!';
                testBtn.classList.add('btn-success');
                testBtn.classList.remove('btn-outline-info');
                statusEl.innerHTML = '<span style="color: #10b981;">Connection successful</span>';
            } else {
                throw new Error(response.error || 'Connection failed');
            }
        } catch (error) {
            testBtn.innerHTML = '<i class="fas fa-times-circle"></i> Failed';
            testBtn.classList.add('btn-danger');
            testBtn.classList.remove('btn-outline-info');
            statusEl.innerHTML = `<span style="color: #ef4444;">${this.escapeHtml(error.message)}</span>`;
        }

        setTimeout(() => {
            testBtn.innerHTML = originalHTML;
            testBtn.classList.remove('btn-success', 'btn-danger');
            testBtn.classList.add('btn-outline-info');
            testBtn.disabled = false;
        }, 3000);
    }

    // ========================================
    // UTILITY METHODS
    // ========================================

    onChange() {
        const event = new CustomEvent('connectorConfigChanged', {
            detail: { config: this.getConfig() }
        });
        this.container.dispatchEvent(event);
    }

    updateTypeDescription(description) {
        const descEl = this.container.querySelector('.connector-type-description');
        if (descEl) {
            descEl.textContent = description;
        }
    }

    updateTestButton() {
        const testBtn = this.container.querySelector('.test-connection-btn');
        if (testBtn) {
            testBtn.disabled = !this.config.connectorType;
        }
    }

    formatGroupName(name) {
        const icons = {
            basic: '<i class="fas fa-cog"></i> ',
            security: '<i class="fas fa-shield-alt"></i> ',
            advanced: '<i class="fas fa-sliders-h"></i> '
        };
        return (icons[name] || '') + name.charAt(0).toUpperCase() + name.slice(1);
    }

    formatFieldName(name) {
        return name.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
    }

    parseJSON(value) {
        if (!value) return null;
        if (typeof value === 'object') return value;
        try {
            return JSON.parse(value);
        } catch {
            return null;
        }
    }

    escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = String(text);
        return div.innerHTML;
    }
}

// ========================================
// EXPORT & REGISTRATION
// ========================================

if (typeof window !== 'undefined') {
    window.ConnectorConfigBuilder = ConnectorConfigBuilder;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = ConnectorConfigBuilder;
}

// Auto-register with factory if available
if (typeof stepConfigBuilderFactory !== 'undefined') {
    stepConfigBuilderFactory.register('connector.outbound', ConnectorConfigBuilder, {
        displayName: 'Outbound Connector',
        category: 'Connectivity',
        description: 'Send data to external systems via configurable connectors',
        icon: '📤'
    });

    stepConfigBuilderFactory.register('connector.inbound', ConnectorConfigBuilder, {
        displayName: 'Inbound Connector',
        category: 'Connectivity',
        description: 'Fetch data from external systems via configurable connectors',
        icon: '📥'
    });

    console.log('[ConnectorConfigBuilder] Registered with factory for connector.outbound and connector.inbound');
}

console.log('[ConnectorConfigBuilder] Loaded - OOP connector configuration builder ready');
