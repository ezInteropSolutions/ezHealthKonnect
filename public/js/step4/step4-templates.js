// step4-templates.js - Complete HTML templates with compact layout

class Step4Templates {
    getMainTemplate(transformationData) {
        return `
            ${this.getCompactSummaryTemplate(transformationData)}
            ${this.getResourcesSectionTemplate()}
            ${this.getModalsTemplate()}
        `;
    }

    getCompactSummaryTemplate(data) {
        // Extract data with defaults
        const segments = data?.parsedData?.enhancedSegments ? 
            Object.keys(data.parsedData.enhancedSegments).length : 3;
        const resources = data?.fhirResources?.length || 2;
        const mappings = data?.mappingStats?.totalFieldsMapped || 22;
        const validationScore = this.calculateValidationScore(data);
        const messageType = data?.messageType || 'ADT^A04';
        const status = data?.success !== false ? 'Success' : 'Has Issues';
        const processingTime = data?.performance?.totalTime || '0ms';

        return `
            <div class="transformation-summary">
                <div class="summary-header-compact">
                    <h3 class="summary-title">
                        <span>📊</span>
                        <span>HL7 to FHIR Transformation Results</span>
                    </h3>
                    <div class="summary-actions">
                        <button class="action-btn-compact secondary" onclick="window.fhirHandler.expandAllResources()">
                            <span class="btn-icon">📖</span>
                            <span>Expand All</span>
                        </button>
                        <button class="action-btn-compact secondary" onclick="window.fhirHandler.validateMappings()">
                            <span class="btn-icon">✅</span>
                            <span>Validate</span>
                        </button>
                        <button class="action-btn-compact secondary" onclick="window.fhirHandler.openJsonViewer()">
                            <span class="btn-icon">{ }</span>
                            <span>View JSON</span>
                        </button>
                        <button class="action-btn-compact primary" onclick="window.fhirHandler.openConfigSaveModal()">
                            <span class="btn-icon">💾</span>
                            <span>Save</span>
                        </button>
                    </div>
                </div>

                <div class="summary-metrics-row">
                    <div class="metric-item">
                        <div class="metric-icon" style="background: #e0f2fe; color: #0369a1;">📥</div>
                        <div class="metric-content">
                            <div class="metric-value">${segments}</div>
                            <div class="metric-label">HL7 Segments</div>
                        </div>
                    </div>
                    
                    <div class="metric-item">
                        <div class="metric-icon" style="background: #fce7f3; color: #be185d;">📦</div>
                        <div class="metric-content">
                            <div class="metric-value">${resources}</div>
                            <div class="metric-label">FHIR Resources</div>
                        </div>
                    </div>
                    
                    <div class="metric-item">
                        <div class="metric-icon" style="background: #ede9fe; color: #7c3aed;">🔗</div>
                        <div class="metric-content">
                            <div class="metric-value">${mappings}</div>
                            <div class="metric-label">Field Mappings</div>
                        </div>
                    </div>
                    
                    <div class="metric-item">
                        <div class="metric-icon" style="background: #dcfce7; color: #16a34a;">✓</div>
                        <div class="metric-content">
                            <div class="metric-value">${validationScore}%</div>
                            <div class="metric-label">Validation</div>
                        </div>
                    </div>
                </div>

                <div class="transformation-metadata">
                    <div class="metadata-item">
                        <span class="metadata-label">Message Type:</span>
                        <span class="metadata-value">${messageType}</span>
                    </div>
                    <div class="metadata-item">
                        <span class="metadata-label">Status:</span>
                        <span class="metadata-value">${status}</span>
                    </div>
                    <div class="metadata-item">
                        <span class="metadata-label">Processing Time:</span>
                        <span class="metadata-value">${processingTime}</span>
                    </div>
                </div>
            </div>
        `;
    }

    getResourcesSectionTemplate() {
        return `
            <div class="resources-section" id="resourcesSection">
                <!-- Resources will be populated dynamically -->
            </div>
        `;
    }

    getResourceCardTemplate(resource, index) {
        const icon = this.getResourceIcon(resource.resourceType);
        const description = this.getResourceDescription(resource);
        const mappingCount = this.countResourceMappings(resource);

        return `
            <div class="fhir-resource-card" data-index="${index}" id="resource-${index}">
                <div class="resource-header">
                    <div class="resource-title">
                        <div class="resource-icon">${icon}</div>
                        <div class="resource-info">
                            <h4>${resource.resourceType}</h4>
                            <p>${description}</p>
                        </div>
                    </div>
                    <div class="resource-actions">
                        <span class="resource-btn mappings">
                            ${mappingCount} mappings
                        </span>
                        <button class="resource-btn expand" onclick="window.fhirHandler.toggleResource(${index})">
                            <span class="expand-icon">▼</span>
                            <span>Expand</span>
                        </button>
                    </div>
                </div>
                <div class="resource-content">
                    ${this.getResourceMappingsTemplate(resource)}
                </div>
            </div>
        `;
    }

    getResourceMappingsTemplate(resource) {
        const mappings = this.extractMappings(resource);
        
        if (!mappings || mappings.length === 0) {
            return '<p style="color: #6b7280; font-style: italic; padding: 12px;">No mappings configured</p>';
        }

        return mappings.map(mapping => `
            <div class="mapping-row">
                <div class="mapping-hl7">
                    <div class="field-info">
                        <div class="field-path">${mapping.hl7Path || 'Static'}</div>
                        <div class="field-value">"${this.truncate(mapping.hl7Value || '', 30)}"</div>
                        <div class="field-description">${mapping.hl7Description || ''}</div>
                    </div>
                </div>
                <div class="mapping-arrow">→</div>
                <div class="mapping-fhir">
                    <div class="field-info">
                        <div class="field-path">${mapping.fhirPath}</div>
                        <div class="field-value">"${this.truncate(mapping.fhirValue || mapping.value || '', 30)}"</div>
                        <div class="field-description">${mapping.fhirDescription || ''}</div>
                    </div>
                </div>
                <div class="mapping-actions">
                    <button class="edit-mapping-btn" onclick="window.fhirHandler.editMapping('${mapping.fhirPath}', '${resource.resourceType}', ${mappings.indexOf(mapping)})">
                        <span class="btn-icon">✏️</span>
                        <span class="btn-text">Edit</span>
                    </button>
                </div>
            </div>
        `).join('');
    }

    getModalsTemplate() {
        return `
            ${this.getJsonViewerModalTemplate()}
            ${this.getValidationSidebarTemplate()}
            ${this.getConfigSaveModalTemplate()}
            ${this.getEditMappingModalTemplate()}
        `;
    }

    getJsonViewerModalTemplate() {
        return `
            <div class="json-viewer-overlay" id="jsonViewerOverlay">
                <div class="json-viewer-modal">
                    <div class="modal-header">
                        <h4>
                            <span>{ }</span>
                            <span>FHIR JSON Output</span>
                        </h4>
                        <div class="header-actions">
                            <button class="header-btn" onclick="window.fhirHandler.copyJson()">
                                <span>📋</span>
                                <span>Copy</span>
                            </button>
                            <button class="header-btn" onclick="window.fhirHandler.downloadJson()">
                                <span>💾</span>
                                <span>Download</span>
                            </button>
                            <button class="modal-close" onclick="window.fhirHandler.closeJsonViewer()">×</button>
                        </div>
                    </div>
                    <div class="json-content">
                        <div class="json-tabs">
                            <button class="json-tab active" data-tab="bundle" onclick="window.fhirHandler.displayJsonTab('bundle')">Bundle</button>
                            <button class="json-tab" data-tab="resources" onclick="window.fhirHandler.displayJsonTab('resources')">Individual Resources</button>
                            <button class="json-tab" data-tab="validation" onclick="window.fhirHandler.displayJsonTab('validation')">Validation Report</button>
                        </div>
                        <div class="json-display">
                            <div id="jsonOutput">Loading...</div>
                        </div>
                    </div>
                </div>
            </div>
        `;
    }

    getValidationSidebarTemplate() {
        return `
            <div class="validation-sidebar" id="validationSidebar">
                <div style="position: fixed; right: 0; top: 0; bottom: 0; width: 400px; background: white; box-shadow: -4px 0 20px rgba(0,0,0,0.1); padding: 20px; overflow-y: auto;">
                    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
                        <h4 style="margin: 0; color: #1e3a8a;">✅ Validation Results</h4>
                        <button class="modal-close" onclick="window.fhirHandler.closeValidationSidebar()" style="color: #6b7280;">×</button>
                    </div>
                    <div id="validationContent">
                        <!-- Validation results will be populated here -->
                    </div>
                </div>
            </div>
        `;
    }

    getConfigSaveModalTemplate() {
        return `
            <div class="config-save-overlay" id="configSaveOverlay">
                <div style="background: white; border-radius: 16px; width: 90%; max-width: 600px; padding: 0; overflow: hidden;">
                    <div class="modal-header">
                        <h4>
                            <span>💾</span>
                            <span>Save Mapping Configuration</span>
                        </h4>
                        <button class="modal-close" onclick="window.fhirHandler.closeConfigSaveModal()">×</button>
                    </div>
                    <div style="padding: 24px;">
                        <div style="margin-bottom: 20px;">
                            <label style="display: block; margin-bottom: 8px; font-weight: 600; color: #1e3a8a;">Configuration Name:</label>
                            <input type="text" id="configName" style="width: 100%; padding: 10px; border: 2px solid #e2e8f0; border-radius: 8px;" placeholder="e.g., ADT A04 Standard Mapping">
                        </div>
                        <div style="margin-bottom: 20px;">
                            <label style="display: block; margin-bottom: 8px; font-weight: 600; color: #1e3a8a;">Description:</label>
                            <textarea id="configDescription" style="width: 100%; padding: 10px; border: 2px solid #e2e8f0; border-radius: 8px; min-height: 100px;" placeholder="Describe this configuration..."></textarea>
                        </div>
                        <div style="display: flex; justify-content: flex-end; gap: 12px; padding-top: 20px; border-top: 2px solid #f1f5f9;">
                            <button class="action-btn-compact secondary" onclick="window.fhirHandler.closeConfigSaveModal()">Cancel</button>
                            <button class="action-btn-compact primary" onclick="window.fhirHandler.saveConfiguration()">Save</button>
                        </div>
                    </div>
                </div>
            </div>
        `;
    }

    getEditMappingModalTemplate() {
        return `
            <div class="edit-mapping-overlay" id="editMappingOverlay">
                <!-- Edit mapping modal will be populated dynamically -->
            </div>
        `;
    }

    // Helper methods
    calculateValidationScore(data) {
        const total = data?.mappingStats?.totalFieldsMapped || 0;
        const errors = data?.errors?.length || 0;
        if (total === 0) return 100;
        return Math.max(0, Math.round(((total - errors) / total) * 100));
    }

    getResourceIcon(resourceType) {
        const icons = {
            'Patient': '👤',
            'MessageHeader': '📨',
            'Encounter': '🏥',
            'Observation': '🔬',
            'Condition': '🩺',
            'Procedure': '💉',
            'DiagnosticReport': '📋',
            'AllergyIntolerance': '⚠️',
            'Medication': '💊',
            'Organization': '🏢',
            'Practitioner': '👨‍⚕️',
            'Bundle': '📦'
        };
        return icons[resourceType] || '📄';
    }

    getResourceDescription(resource) {
        if (resource.resourceType === 'Patient' && resource.name?.[0]) {
            const name = resource.name[0];
            return `${name.given?.join(' ') || ''} ${name.family || ''}`.trim() || 'Patient Record';
        }
        if (resource.resourceType === 'Encounter' && resource.class) {
            return `${resource.class.display || resource.class.code || 'Visit'}`;
        }
        if (resource.resourceType === 'MessageHeader') {
            return `Message ${resource.id || ''}`;
        }
        return `${resource.id || 'Generated Resource'}`;
    }

    countResourceMappings(resource) {
        // Simple count of non-null fields
        let count = 0;
        const countFields = (obj) => {
            for (const key in obj) {
                if (key === 'resourceType' || key === 'meta') continue;
                if (obj[key] !== null && obj[key] !== undefined) {
                    if (typeof obj[key] === 'object' && !Array.isArray(obj[key])) {
                        countFields(obj[key]);
                    } else {
                        count++;
                    }
                }
            }
        };
        countFields(resource);
        return count;
    }

    extractMappings(resource) {
        // This would be replaced with actual mapping extraction logic
        const mappings = [];
        
        // Example mappings for Patient resource
        if (resource.resourceType === 'Patient') {
            if (resource.id) {
                mappings.push({
                    hl7Path: 'PID.3.1',
                    hl7Value: resource.id,
                    hl7Description: 'Patient ID',
                    fhirPath: 'Patient.id',
                    fhirValue: resource.id,
                    fhirDescription: 'Patient Identifier'
                });
            }
            if (resource.name?.[0]) {
                mappings.push({
                    hl7Path: 'PID.5.1',
                    hl7Value: resource.name[0].family,
                    hl7Description: 'Family Name',
                    fhirPath: 'Patient.name[0].family',
                    fhirValue: resource.name[0].family,
                    fhirDescription: 'Family Name'
                });
            }
        }
        
        return mappings;
    }

    truncate(str, maxLength = 50) {
        if (!str) return '';
        return str.length > maxLength ? str.substring(0, maxLength) + '...' : str;
    }
}

// Export for use
window.Step4Templates = Step4Templates;