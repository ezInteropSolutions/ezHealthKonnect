/**
 * FHIRFocusedMappingClass - Complete HL7 to FHIR Mapping Interface
 * File: public/js/components/fhir-focused-mapping.js
 * 
 * This is the missing piece that integrates with your existing backend APIs
 */

class FHIRFocusedMappingClass {
    constructor(options = {}) {
        this.options = {
            messageType: 'auto-detect',
            profile: 'base',
            apiBaseUrl: '/api/fhir/transform',
            enableAutoSave: true,
            enableRealTimeValidation: true,
            showAdvancedOptions: false,
            ...options
        };

        // State management
        this.currentMessageType = null;
        this.currentProfile = 'base';
        this.mappingRules = [];
        this.hl7Structure = null;
        this.fhirStructure = null;
        this.unsavedChanges = false;
        this.validationResults = null;

        // API client
        this.api = window.step4API || new Step4API();

        // Initialize the interface
        this.init();
    }

    /**
     * Initialize the FHIR mapping interface
     */
    async init() {
        console.log('🚀 Initializing FHIR Focused Mapping Class...');
        
        try {
            // Find or create container
            this.container = this.findOrCreateContainer();
            
            // Load initial structure and render UI
            await this.renderInterface();
            
            // Setup event listeners
            this.setupEventListeners();
            
            console.log('✅ FHIR Focused Mapping Class initialized successfully');
        } catch (error) {
            console.error('❌ Failed to initialize FHIR mapping:', error);
            this.showError('Failed to initialize FHIR mapping interface');
        }
    }

    /**
     * Find existing container or create new one
     */
    findOrCreateContainer() {
        // Try to find existing step4 container
        let container = document.getElementById('step4');
        
        if (!container) {
            // Try to find mapping placeholder
            container = document.querySelector('.mapping-placeholder');
        }
        
        if (!container) {
            // Create new container
            container = document.createElement('div');
            container.id = 'fhir-mapping-container';
            document.body.appendChild(container);
        }
        
        return container;
    }

    /**
     * Load configuration for specific message type
     */
    async loadConfiguration(messageType, profile = 'base') {
        console.log(`🔄 Loading FHIR configuration for ${messageType} with profile ${profile}`);
        
        try {
            this.currentMessageType = messageType;
            this.currentProfile = profile;
            
            // Show loading state
            this.showLoadingState();
            
            // Load structures and rules in parallel
            const [hl7Structure, fhirStructure, mappingRules] = await Promise.all([
                this.loadHL7Structure(messageType),
                this.loadFHIRStructure(profile),
                this.loadMappingRules(messageType, profile)
            ]);
            
            this.hl7Structure = hl7Structure;
            this.fhirStructure = fhirStructure;
            this.mappingRules = mappingRules || [];
            
            // Render the interface
            await this.renderInterface();
            
            console.log(`✅ Configuration loaded for ${messageType}`);
        } catch (error) {
            console.error('❌ Failed to load configuration:', error);
            this.showError(`Failed to load configuration for ${messageType}`);
        }
    }

    /**
     * Load HL7 structure from API
     */
    async loadHL7Structure(messageType) {
        try {
            const response = await this.api.getHL7Structure(messageType);
            return response.structure || {};
        } catch (error) {
            console.warn('⚠️ Could not load HL7 structure:', error);
            return this.getDefaultHL7Structure(messageType);
        }
    }

    /**
     * Load FHIR structure from API
     */
    async loadFHIRStructure(profile) {
        try {
            const response = await this.api.getFHIRStructure(profile);
            return response.structure || {};
        } catch (error) {
            console.warn('⚠️ Could not load FHIR structure:', error);
            return this.getDefaultFHIRStructure(profile);
        }
    }

    /**
     * Load mapping rules from API
     */
    async loadMappingRules(messageType, profile) {
        try {
            const endpoint = `/rules?messageType=${encodeURIComponent(messageType)}&profile=${profile}&includeMetadata=true`;
            const response = await this.api.request('GET', endpoint);
            return response.rules || [];
        } catch (error) {
            console.warn('⚠️ Could not load mapping rules:', error);
            return [];
        }
    }

    /**
     * Render the complete mapping interface
     */
    async renderInterface() {
        if (!this.container) return;

        const html = `
            <div class="fhir-mapping-interface">
                ${this.renderConfigurationHeader()}
                ${this.renderMappingStrategy()}
                ${this.renderFHIRResources()}
                ${this.renderMappingSummary()}
                ${this.renderMappingDialog()}
                ${this.renderConfigurationModals()}
            </div>
        `;

        this.container.innerHTML = html;
        this.attachEventListeners();
    }

    /**
     * Render configuration header
     */
    renderConfigurationHeader() {
        const status = this.unsavedChanges ? 'modified' : 'saved';
        const statusText = this.unsavedChanges ? 'Unsaved Changes' : 'Saved';
        
        return `
            <div class="mapping-config-header">
                <div class="config-info">
                    <div class="config-details">
                        <h4>HL7 to FHIR Mapping Configuration</h4>
                        <div class="config-meta">
                            <span class="message-type">${this.currentMessageType || 'Not Selected'}</span>
                            <span class="separator">→</span>
                            <span class="profile-type">FHIR ${this.currentProfile.toUpperCase()}</span>
                            <span class="separator">•</span>
                            <span class="config-status ${status}">${statusText}</span>
                        </div>
                    </div>
                    <div class="config-actions">
                        <button class="config-btn secondary" onclick="window.fhirMapping.openLoadConfigModal()">
                            📂 Load Config
                        </button>
                        <button class="config-btn secondary" onclick="window.fhirMapping.openSaveConfigModal()">
                            💾 Save Config
                        </button>
                        <button class="config-btn primary" onclick="window.fhirMapping.previewTransformation()">
                            🔍 Preview
                        </button>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Render mapping strategy tabs
     */
    renderMappingStrategy() {
        return `
            <div class="mapping-strategy">
                <div class="strategy-tabs">
                    <button class="strategy-tab active" data-strategy="manual">
                        🎯 Manual Mapping
                    </button>
                    <button class="strategy-tab" data-strategy="ai">
                        🤖 AI Suggested
                    </button>
                    <button class="strategy-tab" data-strategy="template">
                        📋 From Template
                    </button>
                </div>
                <div class="strategy-content">
                    <div class="strategy-description">
                        Configure how HL7 fields map to FHIR resources. Click on individual fields to set up custom mappings.
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Render FHIR resources with mapping fields
     */
    renderFHIRResources() {
        const resources = this.getFHIRResourcesForMessageType();
        
        return `
            <div class="fhir-resources-container">
                ${resources.map(resource => this.renderFHIRResource(resource)).join('')}
            </div>
        `;
    }

    /**
     * Render individual FHIR resource
     */
    renderFHIRResource(resource) {
        const mappedFields = this.getMappedFieldsCount(resource.resourceType);
        const totalFields = resource.fields ? resource.fields.length : 0;
        const completionRate = totalFields > 0 ? Math.round((mappedFields / totalFields) * 100) : 0;
        
        return `
            <div class="fhir-resource-card" data-resource="${resource.resourceType}">
                <div class="resource-header" onclick="toggleResourceCard(this.parentElement.querySelector('.resource-toggle-btn'))">
                    <div class="resource-title">
                        <span class="resource-icon">${this.getResourceIcon(resource.resourceType)}</span>
                        <h4>${resource.resourceType}</h4>
                        <span class="completion-badge">${mappedFields}/${totalFields} mapped (${completionRate}%)</span>
                    </div>
                    <button class="resource-toggle-btn" onclick="toggleResourceCard(this)">
                        <span class="toggle-icon">▼</span>
                    </button>
                </div>
                <div class="resource-content">
                    ${this.renderFHIRFieldList(resource)}
                </div>
            </div>
        `;
    }

    /**
     * Render FHIR field list for a resource
     */
    renderFHIRFieldList(resource) {
        if (!resource.fields || resource.fields.length === 0) {
            return '<div class="no-fields">No fields defined for this resource</div>';
        }

        const requiredFields = resource.fields.filter(field => field.required || field.mustSupport);
        const optionalFields = resource.fields.filter(field => !field.required && !field.mustSupport);

        return `
            <div class="fhir-field-list">
                ${requiredFields.length > 0 ? `
                    <div class="field-category">
                        <h5 class="category-title required">Required Fields</h5>
                        ${requiredFields.map(field => this.renderFHIRField(field, resource.resourceType)).join('')}
                    </div>
                ` : ''}
                ${optionalFields.length > 0 ? `
                    <div class="field-category">
                        <h5 class="category-title optional">Optional Fields</h5>
                        ${optionalFields.map(field => this.renderFHIRField(field, resource.resourceType)).join('')}
                    </div>
                ` : ''}
            </div>
        `;
    }

    /**
     * Render individual FHIR field
     */
    renderFHIRField(field, resourceType) {
        const mapping = this.getMappingForField(resourceType, field.path);
        const isMapped = !!mapping;
        const isRequired = field.required || field.mustSupport;
        
        return `
            <div class="fhir-field-item ${isRequired ? 'required' : 'optional'} ${isMapped ? 'mapped' : ''}">
                <div class="field-info">
                    <div class="field-name">${field.path}</div>
                    <div class="field-description">${field.description || field.name}</div>
                    <div class="field-type">${field.type} ${field.cardinality || ''}</div>
                </div>
                <div class="mapping-section">
                    ${isMapped ? `
                        <div class="mapped-indicator">
                            ✓ Mapped
                            <div class="mapped-field">${mapping.hl7Field}</div>
                            <div class="mapping-actions">
                                <button class="mapping-action" onclick="window.fhirMapping.editMapping('${field.path}')">Edit</button>
                                <button class="mapping-action" onclick="window.fhirMapping.removeMapping('${field.path}')">Remove</button>
                            </div>
                        </div>
                    ` : `
                        <button class="map-button" onclick="openMappingDialog('${field.path}')">
                            Map Field
                        </button>
                    `}
                </div>
            </div>
        `;
    }

    /**
     * Render mapping summary
     */
    renderMappingSummary() {
        const totalFields = this.getTotalFHIRFields();
        const mappedFields = this.getTotalMappedFields();
        const unmappedRequired = this.getUnmappedRequiredFields();
        
        return `
            <div class="mapping-summary">
                <div class="summary-stats">
                    <div class="summary-stat">
                        <div class="stat-number">${mappedFields}</div>
                        <div class="stat-label">Mapped Fields</div>
                    </div>
                    <div class="summary-stat">
                        <div class="stat-number">${totalFields - mappedFields}</div>
                        <div class="stat-label">Unmapped Fields</div>
                    </div>
                    <div class="summary-stat">
                        <div class="stat-number">${unmappedRequired}</div>
                        <div class="stat-label">Missing Required</div>
                    </div>
                </div>
                <div class="summary-actions">
                    <button class="summary-btn secondary" onclick="window.fhirMapping.autoMapFields()">
                        🤖 Auto Map
                    </button>
                    <button class="summary-btn primary" onclick="window.fhirMapping.testMapping()">
                        🧪 Test Mapping
                    </button>
                </div>
            </div>
        `;
    }

    /**
     * Render mapping dialog
     */
    renderMappingDialog() {
        return `
            <div id="mappingDialog" class="mapping-dialog-overlay" style="display: none;">
                <div class="mapping-dialog">
                    <div class="dialog-header">
                        <h3>Configure Field Mapping</h3>
                        <button class="dialog-close" onclick="closeMappingDialog()">×</button>
                    </div>
                    <div class="dialog-content">
                        <div class="mapping-form">
                            <div class="form-section">
                                <label>FHIR Field:</label>
                                <input type="text" id="fhirFieldPath" readonly class="form-input">
                            </div>
                            <div class="form-section">
                                <label>HL7 Source Field:</label>
                                <select id="hl7SourceField" class="form-input">
                                    <option value="">Select HL7 field...</option>
                                </select>
                            </div>
                            <div class="form-section">
                                <label>Transformation Type:</label>
                                <select id="transformationType" class="form-input">
                                    <option value="direct">Direct Copy</option>
                                    <option value="identifier">Identifier Format</option>
                                    <option value="human_name">Human Name</option>
                                    <option value="date">Date Format</option>
                                    <option value="code">Code Lookup</option>
                                    <option value="custom">Custom Logic</option>
                                </select>
                            </div>
                            <div class="form-section" id="customLogicSection" style="display: none;">
                                <label>Custom Transformation Logic:</label>
                                <textarea id="customLogic" class="form-input" rows="4" placeholder="Enter JavaScript transformation logic..."></textarea>
                            </div>
                        </div>
                    </div>
                    <div class="dialog-footer">
                        <button class="btn-secondary" onclick="closeMappingDialog()">Cancel</button>
                        <button class="btn-primary" onclick="saveMapping()">Save Mapping</button>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Render configuration modals
     */
    renderConfigurationModals() {
        return `
            ${this.renderLoadConfigModal()}
            ${this.renderSaveConfigModal()}
        `;
    }

    /**
     * Render load configuration modal
     */
    renderLoadConfigModal() {
        return `
            <div id="loadConfigModal" class="config-modal-overlay" style="display: none;">
                <div class="config-modal">
                    <div class="modal-header">
                        <h3>Load Configuration</h3>
                        <button class="modal-close" onclick="closeLoadConfigModal()">×</button>
                    </div>
                    <div class="modal-content">
                        <div id="configurationList" class="configuration-list">
                            <!-- Configurations loaded dynamically -->
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn-secondary" onclick="closeLoadConfigModal()">Cancel</button>
                        <button class="btn-primary" onclick="loadSelectedConfig()" id="loadConfigBtn" disabled>Load Configuration</button>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Render save configuration modal
     */
    renderSaveConfigModal() {
        return `
            <div id="saveConfigModal" class="config-modal-overlay" style="display: none;">
                <div class="config-modal">
                    <div class="modal-header">
                        <h3>Save Configuration</h3>
                        <button class="modal-close" onclick="closeSaveConfigModal()">×</button>
                    </div>
                    <div class="modal-content">
                        <div class="form-section">
                            <label>Configuration Name:</label>
                            <input type="text" id="configName" class="form-input" placeholder="Enter configuration name...">
                        </div>
                        <div class="form-section">
                            <label>Description:</label>
                            <textarea id="configDescription" class="form-input" rows="3" placeholder="Optional description..."></textarea>
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn-secondary" onclick="closeSaveConfigModal()">Cancel</button>
                        <button class="btn-primary" onclick="saveConfigurationToDb()">Save Configuration</button>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Setup event listeners
     */
    setupEventListeners() {
        // Strategy tab switching
        const strategyTabs = this.container.querySelectorAll('.strategy-tab');
        strategyTabs.forEach(tab => {
            tab.addEventListener('click', (e) => {
                this.switchStrategy(e.target.dataset.strategy);
            });
        });

        // Transformation type change
        const transformationType = this.container.querySelector('#transformationType');
        if (transformationType) {
            transformationType.addEventListener('change', (e) => {
                this.handleTransformationTypeChange(e.target.value);
            });
        }
    }

    /**
     * Attach event listeners after rendering
     */
    attachEventListeners() {
        // This is called after each render to reattach listeners
        this.setupEventListeners();
    }

    // ===========================================
    // DIALOG AND MODAL METHODS
    // ===========================================

    /**
     * Open mapping dialog for a specific field
     */
    openMappingDialog(fhirPath) {
        const dialog = document.getElementById('mappingDialog');
        const fhirFieldInput = document.getElementById('fhirFieldPath');
        const hl7SourceSelect = document.getElementById('hl7SourceField');
        
        if (!dialog || !fhirFieldInput || !hl7SourceSelect) return;
        
        // Set FHIR field path
        fhirFieldInput.value = fhirPath;
        
        // Populate HL7 fields
        this.populateHL7Fields(hl7SourceSelect);
        
        // Load existing mapping if available
        const existingMapping = this.getMappingForField(null, fhirPath);
        if (existingMapping) {
            hl7SourceSelect.value = existingMapping.hl7Field;
            document.getElementById('transformationType').value = existingMapping.transformationType || 'direct';
        }
        
        dialog.style.display = 'flex';
    }

    /**
     * Close mapping dialog
     */
    closeMappingDialog() {
        const dialog = document.getElementById('mappingDialog');
        if (dialog) {
            dialog.style.display = 'none';
        }
    }

    /**
     * Save mapping from dialog
     */
    saveMapping() {
        const fhirPath = document.getElementById('fhirFieldPath').value;
        const hl7Field = document.getElementById('hl7SourceField').value;
        const transformationType = document.getElementById('transformationType').value;
        const customLogic = document.getElementById('customLogic').value;
        
        if (!fhirPath || !hl7Field) {
            alert('Please select both FHIR and HL7 fields');
            return;
        }
        
        const mapping = {
            fhirPath: fhirPath,
            hl7Field: hl7Field,
            transformationType: transformationType,
            customLogic: customLogic
        };
        
        this.addOrUpdateMapping(mapping);
        this.closeMappingDialog();
        this.renderInterface(); // Re-render to show changes
    }

    /**
     * Open load configuration modal
     */
    async openLoadConfigModal() {
        const modal = document.getElementById('loadConfigModal');
        const configList = document.getElementById('configurationList');
        
        if (!modal || !configList) return;
        
        try {
            // Load configurations from API
            const response = await this.api.listConfigurations({
                messageType: this.currentMessageType
            });
            
            const configurations = response.configurations || [];
            
            configList.innerHTML = configurations.length > 0 ? 
                configurations.map(config => `
                    <div class="config-item" data-config-id="${config.id}" onclick="selectConfiguration('${config.id}')">
                        <div class="config-name">${config.name}</div>
                        <div class="config-meta">${config.messageType} • ${config.profile}</div>
                        <div class="config-date">${new Date(config.created_at).toLocaleDateString()}</div>
                    </div>
                `).join('') :
                '<div class="no-configs">No saved configurations found</div>';
            
            modal.style.display = 'flex';
        } catch (error) {
            console.error('Failed to load configurations:', error);
            this.showError('Failed to load configurations');
        }
    }

    /**
     * Close load configuration modal
     */
    closeLoadConfigModal() {
        const modal = document.getElementById('loadConfigModal');
        if (modal) {
            modal.style.display = 'none';
        }
    }

    /**
     * Open save configuration modal
     */
    openSaveConfigModal() {
        const modal = document.getElementById('saveConfigModal');
        if (modal) {
            modal.style.display = 'flex';
        }
    }

    /**
     * Close save configuration modal
     */
    closeSaveConfigModal() {
        const modal = document.getElementById('saveConfigModal');
        if (modal) {
            modal.style.display = 'none';
        }
    }

    // ===========================================
    // MAPPING MANAGEMENT METHODS
    // ===========================================

    /**
     * Add or update a mapping rule
     */
    addOrUpdateMapping(mapping) {
        const existingIndex = this.mappingRules.findIndex(rule => rule.fhirPath === mapping.fhirPath);
        
        const mappingRule = {
            ...mapping,
            id: existingIndex >= 0 ? this.mappingRules[existingIndex].id : Date.now(),
            enabled: true,
            confidence: 'high',
            priority: existingIndex >= 0 ? this.mappingRules[existingIndex].priority : this.mappingRules.length + 1
        };
        
        if (existingIndex >= 0) {
            this.mappingRules[existingIndex] = mappingRule;
        } else {
            this.mappingRules.push(mappingRule);
        }
        
        this.unsavedChanges = true;
    }

    /**
     * Remove a mapping
     */
    removeMapping(fhirPath) {
        this.mappingRules = this.mappingRules.filter(rule => rule.fhirPath !== fhirPath);
        this.unsavedChanges = true;
        this.renderInterface();
    }

    /**
     * Edit existing mapping
     */
    editMapping(fhirPath) {
        this.openMappingDialog(fhirPath);
    }

    /**
     * Get mapping for a specific field
     */
    getMappingForField(resourceType, fhirPath) {
        return this.mappingRules.find(rule => rule.fhirPath === fhirPath);
    }

    /**
     * Auto-map fields using AI suggestions
     */
    async autoMapFields() {
        try {
            const response = await this.api.request('POST', '/suggestions', {
                messageType: this.currentMessageType,
                profile: this.currentProfile,
                options: {
                    confidenceThreshold: 0.7,
                    includeExplanations: true
                }
            });
            
            const suggestions = response.suggestions || [];
            
            // Apply high-confidence suggestions
            suggestions.forEach(suggestion => {
                if (suggestion.confidence >= 0.8) {
                    this.addOrUpdateMapping({
                        fhirPath: suggestion.fhirPath,
                        hl7Field: suggestion.hl7Field,
                        transformationType: suggestion.transformationType || 'direct'
                    });
                }
            });
            
            this.renderInterface();
            
            this.showSuccess(`Auto-mapped ${suggestions.length} fields with high confidence`);
        } catch (error) {
            console.error('Auto-mapping failed:', error);
            this.showError('Auto-mapping failed');
        }
    }

    /**
     * Test current mapping configuration
     */
    async testMapping() {
        if (!this.currentMessageType) {
            this.showError('No message type selected');
            return;
        }
        
        try {
            const response = await this.api.request('POST', '/preview', {
                messageType: this.currentMessageType,
                mappingRules: this.mappingRules,
                options: {
                    validateFHIR: true,
                    includeUnmapped: true
                }
            });
            
            this.validationResults = response.validationResults;
            
            // Show results in a modal or expand summary
            this.showTestResults(response);
            
        } catch (error) {
            console.error('Mapping test failed:', error);
            this.showError('Mapping test failed');
        }
    }

    /**
     * Preview transformation
     */
    async previewTransformation() {
        // This will open a preview modal showing the transformed FHIR output
        this.testMapping();
    }

    // ===========================================
    // UTILITY METHODS
    // ===========================================

    /**
     * Get FHIR resources for current message type
     */
    getFHIRResourcesForMessageType() {
        if (this.fhirStructure && Object.keys(this.fhirStructure).length > 0) {
            return Object.values(this.fhirStructure);
        }
        
        // Default resources for common message types
        const commonResources = {
            'ADT^A01': ['Patient', 'Encounter', 'MessageHeader'],
            'ORU^R01': ['Patient', 'Observation', 'DiagnosticReport', 'MessageHeader'],
            'ORM^O01': ['Patient', 'ServiceRequest', 'MessageHeader']
        };
        
        const resourceTypes = commonResources[this.currentMessageType] || ['Patient', 'MessageHeader'];
        
        return resourceTypes.map(type => ({
            resourceType: type,
            fields: this.getDefaultFieldsForResource(type)
        }));
    }

    /**
     * Get default fields for a resource type
     */
    getDefaultFieldsForResource(resourceType) {
        const defaults = {
            Patient: [
                { path: 'Patient.identifier', name: 'Identifier', required: true, type: 'Identifier[]' },
                { path: 'Patient.name', name: 'Name', required: true, type: 'HumanName[]' },
                { path: 'Patient.birthDate', name: 'Birth Date', required: false, type: 'date' },
                { path: 'Patient.gender', name: 'Gender', required: false, type: 'code' },
                { path: 'Patient.address', name: 'Address', required: false, type: 'Address[]' },
                { path: 'Patient.telecom', name: 'Contact', required: false, type: 'ContactPoint[]' }
            ],
            Encounter: [
                { path: 'Encounter.status', name: 'Status', required: true, type: 'code' },
                { path: 'Encounter.class', name: 'Class', required: true, type: 'Coding' },
                { path: 'Encounter.subject', name: 'Patient', required: true, type: 'Reference(Patient)' },
                { path: 'Encounter.period', name: 'Period', required: false, type: 'Period' }
            ],
            MessageHeader: [
                { path: 'MessageHeader.source', name: 'Source', required: true, type: 'MessageHeaderSource' },
                { path: 'MessageHeader.event', name: 'Event', required: true, type: 'Coding' },
                { path: 'MessageHeader.timestamp', name: 'Timestamp', required: true, type: 'instant' }
            ]
        };
        
        return defaults[resourceType] || [];
    }

    /**
     * Populate HL7 fields dropdown
     */
    populateHL7Fields(selectElement) {
        if (!this.hl7Structure || !selectElement) return;
        
        selectElement.innerHTML = '<option value="">Select HL7 field...</option>';
        
        Object.keys(this.hl7Structure).forEach(segmentName => {
            const segment = this.hl7Structure[segmentName];
            if (segment.fields) {
                segment.fields.forEach(field => {
                    const option = document.createElement('option');
                    option.value = `${segmentName}.${field.field}`;
                    option.textContent = `${segmentName}.${field.field} - ${field.name || field.description || 'Unknown field'}`;
                    selectElement.appendChild(option);
                });
            }
        });
    }

    /**
     * Get resource icon
     */
    getResourceIcon(resourceType) {
        const icons = {
            Patient: '👤',
            Encounter: '🏥',
            Observation: '🔬',
            DiagnosticReport: '📋',
            ServiceRequest: '📝',
            MessageHeader: '📨',
            Practitioner: '👩‍⚕️',
            Organization: '🏢'
        };
        
        return icons[resourceType] || '📄';
    }

    /**
     * Get mapped fields count for resource
     */
    getMappedFieldsCount(resourceType) {
        return this.mappingRules.filter(rule => 
            rule.fhirPath.startsWith(`${resourceType}.`)
        ).length;
    }

    /**
     * Get total FHIR fields
     */
    getTotalFHIRFields() {
        const resources = this.getFHIRResourcesForMessageType();
        return resources.reduce((total, resource) => total + (resource.fields?.length || 0), 0);
    }

    /**
     * Get total mapped fields
     */
    getTotalMappedFields() {
        return this.mappingRules.length;
    }

    /**
     * Get unmapped required fields count
     */
    getUnmappedRequiredFields() {
        const resources = this.getFHIRResourcesForMessageType();
        let unmappedRequired = 0;
        
        resources.forEach(resource => {
            if (resource.fields) {
                resource.fields.forEach(field => {
                    if (field.required && !this.getMappingForField(resource.resourceType, field.path)) {
                        unmappedRequired++;
                    }
                });
            }
        });
        
        return unmappedRequired;
    }

    /**
     * Handle transformation type change
     */
    handleTransformationTypeChange(type) {
        const customSection = document.getElementById('customLogicSection');
        if (customSection) {
            customSection.style.display = type === 'custom' ? 'block' : 'none';
        }
    }

    /**
     * Switch mapping strategy
     */
    switchStrategy(strategy) {
        // Update active tab
        this.container.querySelectorAll('.strategy-tab').forEach(tab => {
            tab.classList.remove('active');
        });
        this.container.querySelector(`[data-strategy="${strategy}"]`).classList.add('active');
        
        // Handle strategy-specific logic
        switch (strategy) {
            case 'ai':
                this.autoMapFields();
                break;
            case 'template':
                this.loadTemplateMapping();
                break;
            default:
                // Manual mapping - no action needed
                break;
        }
    }

    /**
     * Show loading state
     */
    showLoadingState() {
        if (this.container) {
            this.container.innerHTML = `
                <div class="loading-state">
                    <div class="loading-spinner"></div>
                    <div class="loading-text">Loading FHIR mapping configuration...</div>
                </div>
            `;
        }
    }

    /**
     * Show error message
     */
    showError(message) {
        console.error('FHIR Mapping Error:', message);
        // You can implement a toast notification system here
        alert(`Error: ${message}`);
    }

    /**
     * Show success message
     */
    showSuccess(message) {
        console.log('FHIR Mapping Success:', message);
        // You can implement a toast notification system here
    }

    /**
     * Show test results
     */
    showTestResults(results) {
        // This would open a modal or update the interface to show test results
        console.log('Test Results:', results);
        this.showSuccess(`Test completed: ${results.fhir_resources?.length || 0} resources generated`);
    }

    /**
     * Get default HL7 structure (fallback)
     */
    getDefaultHL7Structure(messageType) {
        // Return basic structure if API fails
        return {
            MSH: {
                segment: 'MSH',
                fields: [
                    { field: '3', name: 'Sending Application' },
                    { field: '7', name: 'Date/Time of Message' },
                    { field: '9', name: 'Message Type' }
                ]
            },
            PID: {
                segment: 'PID',
                fields: [
                    { field: '3', name: 'Patient Identifier List' },
                    { field: '5', name: 'Patient Name' },
                    { field: '7', name: 'Date/Time of Birth' },
                    { field: '8', name: 'Administrative Sex' }
                ]
            }
        };
    }

    /**
     * Get default FHIR structure (fallback)
     */
    getDefaultFHIRStructure(profile) {
        return {
            Patient: {
                resourceType: 'Patient',
                profile: profile,
                fields: this.getDefaultFieldsForResource('Patient')
            }
        };
    }
}

// Global export
window.FHIRFocusedMappingClass = FHIRFocusedMappingClass;

console.log('✅ FHIR Focused Mapping Class loaded successfully');