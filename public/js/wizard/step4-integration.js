class FHIRMappingStepHandler extends BaseStepHandler {
    constructor(wizard) {
        super(wizard);
        this.transformationData = null;
        this.mappingConfiguration = new Map();
        this.editableMappings = new Map();
        this.apiBaseUrl = this.getApiBaseUrl();
        this.expandedResources = new Set();
        this.currentEditMapping = null;
        
        // CRITICAL FIX: Immediately attach to window for global access
        window.fhirHandler = this;
        console.log('✅ FHIRMappingStepHandler attached to window.fhirHandler');
    }

    getApiBaseUrl() {
        return window.getApiBaseUrl ? window.getApiBaseUrl() : 'http://localhost:8080';
    }

    async initialize() {
        console.log('🎯 Initializing Enhanced FHIR Transformation Results Step 4...');
        window.fhirHandler = this;
        await this.loadTransformationInterface();
        
        if (this.wizard.currentStep === 4 && this.getParsedHL7Data()) {
            await this.onStepActivated();
        } else {
            this.showPlaceholder();
        }
    }

    async loadTransformationInterface() {
        const step4Element = document.getElementById('step4');
        if (!step4Element) return;

        step4Element.innerHTML = `
            <div class="step-header">
                <div class="step-progress">STEP 4 OF 5</div>
                <h3 class="step-title">
                    <span class="title-icon">🔄</span>
                    HL7 to FHIR Transformation Results
                </h3>
                <p class="step-description">
                    <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 16px; height: 16px; border-radius: 2px; margin-right: 6px;">
                    Review mapped fields, edit configurations, and validate FHIR output
                </p>
            </div>
            
            <!-- Transformation Summary -->
            <div class="transformation-summary">
                <div class="summary-header">
                    <div class="summary-title">
                        <span class="summary-icon">📊</span>
                        <h4>Transformation Summary</h4>
                    </div>
                    <div class="summary-actions">
                        <button id="viewJsonBtn" class="action-btn secondary">
                            <span class="btn-icon">{ }</span>
                            View FHIR JSON
                        </button>
                        <button id="saveConfigBtn" class="action-btn primary">
                            <span class="btn-icon">💾</span>
                            Save Mapping Config
                        </button>
                    </div>
                </div>
                
                <div class="summary-grid">
                    <div class="summary-metric">
                        <div class="metric-icon source">📋</div>
                        <div class="metric-content">
                            <div class="metric-number" id="hl7SegmentCount">0</div>
                            <div class="metric-label">HL7 Segments Processed</div>
                        </div>
                    </div>
                    <div class="summary-metric">
                        <div class="metric-icon target">🎯</div>
                        <div class="metric-content">
                            <div class="metric-number" id="fhirResourceCount">0</div>
                            <div class="metric-label">FHIR Resources Created</div>
                        </div>
                    </div>
                    <div class="summary-metric">
                        <div class="metric-icon mapping">🔗</div>
                        <div class="metric-content">
                            <div class="metric-number" id="successfulMappings">0</div>
                            <div class="metric-label">Successful Field Mappings</div>
                        </div>
                    </div>
                    <div class="summary-metric">
                        <div class="metric-icon status">✅</div>
                        <div class="metric-content">
                            <div class="metric-number" id="validationScore">0%</div>
                            <div class="metric-label">FHIR Validation Score</div>
                        </div>
                    </div>
                </div>
                
                <div class="mapping-overview">
                    <div class="overview-item">
                        <span class="overview-label">Message Type:</span>
                        <span class="overview-value" id="messageTypeOverview">Loading...</span>
                    </div>
                    <div class="overview-item">
                        <span class="overview-label">Transformation Status:</span>
                        <span class="overview-value status" id="transformationStatus">Processing...</span>
                    </div>
                    <div class="overview-item">
                        <span class="overview-label">Processing Time:</span>
                        <span class="overview-value" id="processingTime">-</span>
                    </div>
                </div>
            </div>

            <!-- FHIR Resources -->
            <div class="resources-section">
                <div class="section-header">
                    <h4>
                        <span class="section-icon">🏗️</span>
                        FHIR Resources & Field Mappings
                    </h4>
                    <div class="section-actions">
                        <button id="expandAllBtn" class="section-btn">
                            <span class="btn-icon">📖</span>
                            Expand All
                        </button>
                        <button id="validateMappingsBtn" class="section-btn">
                            <span class="btn-icon">✅</span>
                            Validate Mappings
                        </button>
                    </div>
                </div>
                
                <div id="fhirResourcesContainer" class="resources-container">
                    <!-- Will be populated with transformation results -->
                </div>
            </div>

            <!-- Edit Mapping Modal -->
            <div class="edit-mapping-overlay" id="editMappingOverlay">
                <div class="edit-mapping-modal">
                    <div class="modal-header">
                        <h4>
                            <span class="modal-icon">⚙️</span>
                            <span id="editModalTitle">Configure Field Mapping</span>
                        </h4>
                        <button class="modal-close" onclick="window.fhirHandler.closeEditModal()">×</button>
                    </div>
                    
                    <div class="modal-content">
                        <div class="current-mapping-display">
                            <div class="mapping-card">
                                <div class="mapping-side hl7-side">
                                    <div class="side-header">
                                        <span class="side-icon">📨</span>
                                        <span class="side-title">HL7 Source</span>
                                    </div>
                                    <div class="field-display">
                                        <div class="field-path" id="currentHL7Path">PID.5</div>
                                        <div class="field-value" id="currentHL7Value">"MOUSE^MICKEY"</div>
                                        <div class="field-description" id="currentHL7Desc">Patient Name</div>
                                    </div>
                                </div>
                                
                                <div class="mapping-connector">
                                    <div class="connector-line"></div>
                                    <div class="connector-arrow">→</div>
                                </div>
                                
                                <div class="mapping-side fhir-side">
                                    <div class="side-header">
                                        <span class="side-icon">🔥</span>
                                        <span class="side-title">FHIR Target</span>
                                    </div>
                                    <div class="field-display">
                                        <div class="field-path" id="currentFHIRPath">Patient.name[0].family</div>
                                        <div class="field-value" id="currentFHIRValue">"MOUSE"</div>
                                        <div class="field-description" id="currentFHIRDesc">Family Name</div>
                                    </div>
                                </div>
                            </div>
                        </div>
                        
                        <div class="edit-configuration">
                            <div class="config-section">
                                <h5>🔧 Edit Configuration</h5>
                                
                                <div class="config-grid">
                                    <div class="config-group">
                                        <label class="config-label">HL7 Source Field:</label>
                                        <select id="newHL7Field" class="config-select">
                                            <option value="">Select HL7 Field...</option>
                                        </select>
                                        <div class="field-preview" id="hl7FieldPreview">
                                            <span class="preview-label">Current Value:</span>
                                            <span class="preview-value">Select field to see value</span>
                                        </div>
                                    </div>
                                    
                                    <div class="config-group">
                                        <label class="config-label">FHIR Destination:</label>
                                        <select id="newFHIRField" class="config-select">
                                            <option value="">Select FHIR Field...</option>
                                        </select>
                                        <div class="field-preview" id="fhirFieldPreview">
                                            <span class="preview-label">Data Type:</span>
                                            <span class="preview-value">Select field to see type</span>
                                        </div>
                                    </div>
                                </div>
                                
                                <div class="transformation-options">
                                    <label class="config-label">Transformation Type:</label>
                                    <div class="transform-grid">
                                        <label class="transform-option">
                                            <input type="radio" name="transformType" value="direct" checked>
                                            <span class="option-content">
                                                <span class="option-title">Direct Copy</span>
                                                <span class="option-desc">Copy value as-is</span>
                                            </span>
                                        </label>
                                        <label class="transform-option">
                                            <input type="radio" name="transformType" value="format">
                                            <span class="option-content">
                                                <span class="option-title">Format/Convert</span>
                                                <span class="option-desc">Apply formatting rules</span>
                                            </span>
                                        </label>
                                        <label class="transform-option">
                                            <input type="radio" name="transformType" value="split">
                                            <span class="option-content">
                                                <span class="option-title">Split Field</span>
                                                <span class="option-desc">Split by delimiter</span>
                                            </span>
                                        </label>
                                        <label class="transform-option">
                                            <input type="radio" name="transformType" value="custom">
                                            <span class="option-content">
                                                <span class="option-title">Custom Logic</span>
                                                <span class="option-desc">Apply custom transformation</span>
                                            </span>
                                        </label>
                                    </div>
                                </div>
                                
                                <div class="test-transformation">
                                    <label class="config-label">Preview Result:</label>
                                    <div class="test-result" id="transformationPreview">
                                        <div class="test-before">
                                            <span class="test-label">Before:</span>
                                            <span class="test-value" id="beforeValue">-</span>
                                        </div>
                                        <div class="test-arrow">→</div>
                                        <div class="test-after">
                                            <span class="test-label">After:</span>
                                            <span class="test-value" id="afterValue">-</span>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                    
                    <div class="modal-actions">
                        <button class="modal-btn secondary" onclick="window.fhirHandler.closeEditModal()">
                            <span class="btn-icon">❌</span>
                            Cancel
                        </button>
                        <button class="modal-btn primary" onclick="window.fhirHandler.saveMapping()" id="saveMappingBtn">
                            <span class="btn-icon">✅</span>
                            Save Mapping
                        </button>
                    </div>
                </div>
            </div>

            <!-- FHIR JSON Viewer Modal -->
            <div class="json-viewer-overlay" id="jsonViewerOverlay">
                <div class="json-viewer-modal">
                    <div class="modal-header">
                        <h4>
                            <span class="modal-icon">{ }</span>
                            FHIR JSON Output
                        </h4>
                        <div class="header-actions">
                            <button class="header-btn" onclick="window.fhirHandler.copyJson()">
                                <span class="btn-icon">📋</span>
                                Copy
                            </button>
                            <button class="header-btn" onclick="window.fhirHandler.downloadJson()">
                                <span class="btn-icon">💾</span>
                                Download
                            </button>
                            <button class="modal-close" onclick="window.fhirHandler.closeJsonViewer()">×</button>
                        </div>
                    </div>
                    
                    <div class="json-content">
                        <div class="json-tabs">
                            <button class="json-tab active" data-tab="bundle">Bundle</button>
                            <button class="json-tab" data-tab="resources">Individual Resources</button>
                            <button class="json-tab" data-tab="validation">Validation Report</button>
                        </div>
                        
                        <div class="json-display">
                            <pre id="jsonOutput"><code>Loading JSON...</code></pre>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Validation Results Sidebar -->
            <div class="validation-sidebar" id="validationSidebar">
                <div class="sidebar-header">
                    <h4>
                        <span class="sidebar-icon">✅</span>
                        Validation Results
                    </h4>
                    <button class="sidebar-close" onclick="window.fhirHandler.closeValidationSidebar()">×</button>
                </div>
                
                <div class="validation-summary">
                    <div class="validation-metric success">
                        <div class="metric-number" id="validationSuccess">0</div>
                        <div class="metric-label">Valid Fields</div>
                    </div>
                    <div class="validation-metric warning">
                        <div class="metric-number" id="validationWarnings">0</div>
                        <div class="metric-label">Warnings</div>
                    </div>
                    <div class="validation-metric error">
                        <div class="metric-number" id="validationErrors">0</div>
                        <div class="metric-label">Errors</div>
                    </div>
                </div>
                
                <div class="validation-content" id="validationContent">
                    <!-- Populated with filtered validation results -->
                </div>
            </div>

            <!-- Configuration Save Modal -->
            <div class="config-save-overlay" id="configSaveOverlay">
                <div class="config-save-modal">
                    <div class="modal-header">
                        <h4>
                            <span class="modal-icon">💾</span>
                            Save Mapping Configuration
                        </h4>
                        <button class="modal-close" onclick="window.fhirHandler.closeConfigSaveModal()">×</button>
                    </div>
                    
                    <div class="modal-content">
                        <div class="config-form">
                            <div class="form-section">
                                <h5>Configuration Details</h5>
                                <div class="form-grid">
                                    <div class="form-group">
                                        <label class="form-label">Configuration Name:</label>
                                        <input type="text" id="configName" class="form-input" placeholder="e.g., ADT A04 Standard Mapping">
                                    </div>
                                    <div class="form-group">
                                        <label class="form-label">Message Type:</label>
                                        <input type="text" id="configMessageType" class="form-input" readonly>
                                    </div>
                                </div>
                                <div class="form-group">
                                    <label class="form-label">Description:</label>
                                    <textarea id="configDescription" class="form-textarea" placeholder="Describe this mapping configuration..."></textarea>
                                </div>
                            </div>
                            
                            <div class="config-summary-section">
                                <h5>Configuration Summary</h5>
                                <div class="summary-stats">
                                    <div class="stat-item">
                                        <span class="stat-label">Total Mappings:</span>
                                        <span class="stat-value" id="configMappingCount">0</span>
                                    </div>
                                    <div class="stat-item">
                                        <span class="stat-label">FHIR Resources:</span>
                                        <span class="stat-value" id="configResourceCount">0</span>
                                    </div>
                                    <div class="stat-item">
                                        <span class="stat-label">Validation Score:</span>
                                        <span class="stat-value" id="configValidationScore">0%</span>
                                    </div>
                                </div>
                            </div>
                            
                            <div class="config-options">
                                <div class="option-group">
                                    <label class="checkbox-option">
                                        <input type="checkbox" id="setAsDefault" checked>
                                        <span class="checkbox-content">
                                            <span class="option-title">Set as Default</span>
                                            <span class="option-desc">Use this configuration for all future messages of this type</span>
                                        </span>
                                    </label>
                                </div>
                            </div>
                        </div>
                    </div>
                    
                    <div class="modal-actions">
                        <button class="modal-btn secondary" onclick="window.fhirHandler.closeConfigSaveModal()">
                            <span class="btn-icon">❌</span>
                            Cancel
                        </button>
                        <button class="modal-btn primary" onclick="window.fhirHandler.saveConfiguration()" id="saveConfigurationBtn">
                            <span class="btn-icon">💾</span>
                            Save Configuration
                        </button>
                    </div>
                </div>
            </div>

            <!-- Placeholder -->
            <div class="transformation-placeholder" id="transformPlaceholder" style="display: none;">
                <div class="placeholder-content">
                    <div class="placeholder-icon">🔄</div>
                    <h4>Ready for HL7 to FHIR Transformation</h4>
                    <p>Upload and parse an HL7 message in the previous steps to see transformation results and configure field mappings.</p>
                    <div class="placeholder-actions">
                        <button onclick="window.wizard.showStep(2)" class="wizard-btn secondary">
                            <span class="btn-icon">⬅️</span>
                            Go to Step 2 (Upload HL7)
                        </button>
                    </div>
                </div>
            </div>

            <style>
                /* Enhanced FHIR Transformation Results Styles */
                .step-title {
                    display: flex;
                    align-items: center;
                    gap: 12px;
                    margin: 0 0 8px 0;
                }

                .title-icon {
                    font-size: 24px;
                    background: linear-gradient(135deg, #1e3a8a 0%, #3b82f6 100%);
                    -webkit-background-clip: text;
                    -webkit-text-fill-color: transparent;
                    background-clip: text;
                }

                /* Transformation Summary */
                .transformation-summary {
                    background: linear-gradient(135deg, #ffffff 0%, #f8f9ff 100%);
                    border: 2px solid #f8bbd9;
                    border-radius: 16px;
                    padding: 24px;
                    margin-bottom: 24px;
                    box-shadow: 0 4px 6px rgba(248, 187, 217, 0.1);
                }

                .summary-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    margin-bottom: 20px;
                    padding-bottom: 16px;
                    border-bottom: 2px solid #f1f5f9;
                }

                .summary-title {
                    display: flex;
                    align-items: center;
                    gap: 12px;
                }

                .summary-icon {
                    font-size: 20px;
                    background: linear-gradient(135deg, #1e3a8a 0%, #3b82f6 100%);
                    -webkit-background-clip: text;
                    -webkit-text-fill-color: transparent;
                }

                .summary-title h4 {
                    margin: 0;
                    font-size: 18px;
                    font-weight: 600;
                    color: #1e3a8a;
                }

                .summary-actions {
                    display: flex;
                    gap: 12px;
                }

                .action-btn {
                    padding: 10px 16px;
    border: none;
    border-radius: 8px;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
    display: flex;
    align-items: center;
    gap: 8px;
                }

                .action-btn.primary {
                    background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
    color: white !important;
    box-shadow: 0 2px 4px rgba(30, 58, 138, 0.2);
                }

                .action-btn.primary:hover {
                    transform: translateY(-1px);
                    box-shadow: 0 4px 8px rgba(30, 58, 138, 0.3);
                }

                .action-btn.secondary {
                    background: white;
    color: #1e3a8a !important;
    border: 2px solid #e2e8f0;
                }

                .action-btn.secondary:hover {
                    background: #f8f9ff;
                    border-color: #f8bbd9;
                    transform: translateY(-1px);
                }

                .btn-icon {
                    font-size: 16px;
                    line-height: 1;
    display: inline-block;
                }

                /* Summary Metrics Grid */
                .summary-grid {
                    display: grid;
                    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
                    gap: 16px;
                    margin-bottom: 20px;
                }

                .summary-metric {
                    background: white;
                    border: 2px solid #f1f5f9;
                    border-radius: 12px;
                    padding: 20px;
                    display: flex;
                    align-items: center;
                    gap: 16px;
                    transition: all 0.2s ease;
                    position: relative;
                    overflow: hidden;
                }

                .summary-metric:hover {
                    transform: translateY(-2px);
                    box-shadow: 0 8px 16px rgba(248, 187, 217, 0.15);
                    border-color: #f8bbd9;
                }

                .metric-icon {
                    width: 48px;
                    height: 48px;
                    border-radius: 12px;
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    font-size: 20px;
                    font-weight: 600;
                    color: white;
                    flex-shrink: 0;
                }

                .metric-icon.source {
                    background: linear-gradient(135deg, #059669 0%, #10b981 100%);
                }

                .metric-icon.target {
                    background: linear-gradient(135deg, #dc2626 0%, #ef4444 100%);
                }

                .metric-icon.mapping {
                    background: linear-gradient(135deg, #7c3aed 0%, #8b5cf6 100%);
                }

                .metric-icon.status {
                    background: linear-gradient(135deg, #1e3a8a 0%, #3b82f6 100%);
                }

                .metric-content {
                    flex: 1;
                }

                .metric-number {
                    font-size: 28px;
                    font-weight: 700;
                    color: #1e3a8a;
                    line-height: 1;
                    margin-bottom: 6px;
                }

                .metric-label {
                    font-size: 13px;
                    color: #6b7280;
                    font-weight: 500;
                    text-transform: uppercase;
                    letter-spacing: 0.5px;
                }

                /* Mapping Overview */
                .mapping-overview {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    background: linear-gradient(135deg, #f8f9ff 0%, #e0e7ff 100%);
                    border: 1px solid #e0e7ff;
                    border-radius: 8px;
                    padding: 16px 20px;
                }

                .overview-item {
                    display: flex;
                    align-items: center;
                    gap: 8px;
                    font-size: 14px;
                }

                .overview-label {
                    color: #6b7280;
                    font-weight: 500;
                }

                .overview-value {
                    color: #1e3a8a;
                    font-weight: 600;
                    padding: 4px 8px;
                    background: white;
                    border-radius: 4px;
                }

                .overview-value.status {
                    text-transform: capitalize;
                }

                /* Resources Section */
                .resources-section {
                    margin-bottom: 24px;
                }

                .section-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    margin-bottom: 16px;
                }

                .section-header h4 {
                    margin: 0;
                    font-size: 18px;
                    font-weight: 600;
                    color: #1e3a8a;
                    display: flex;
                    align-items: center;
                    gap: 12px;
                }

                .section-icon {
                    font-size: 20px;
                }

                .section-actions {
                    display: flex;
                    gap: 12px;
                }

                .section-btn {
                    padding: 8px 12px;
                    border: none;
                    border-radius: 6px;
                    font-size: 13px;
                    font-weight: 500;
                    cursor: pointer;
                    background: #f8f9ff;
                    color: #1e3a8a;
                    border: 1px solid #e2e8f0;
                    display: flex;
                    align-items: center;
                    gap: 6px;
                    transition: all 0.2s ease;
                }

                .section-btn:hover {
                    background: #e0e7ff;
                    border-color: #f8bbd9;
                }

                /* Resources Container */
                .resources-container {
                    display: flex;
                    flex-direction: column;
                    gap: 20px;
                }

                .fhir-resource {
                    background: white;
                    border: 2px solid #f8bbd9;
                    border-radius: 12px;
                    overflow: hidden;
                    transition: all 0.3s ease;
                }

                .fhir-resource:hover {
                    transform: translateY(-2px);
                    box-shadow: 0 8px 20px rgba(248, 187, 217, 0.15);
                }

                .fhir-resource.collapsed .resource-content {
                    display: none;
                }

                .resource-header {
                    background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
                    color: white;
                    padding: 20px 24px;
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    cursor: pointer;
                    position: relative;
                }

                .resource-header::before {
                    content: '';
                    position: absolute;
                    top: 0;
                    left: 0;
                    right: 0;
                    height: 3px;
                    background: linear-gradient(90deg, #f8bbd9 0%, #fce7f3 100%);
                }

                .resource-title {
                    display: flex;
                    align-items: center;
                    gap: 16px;
                }

                .resource-icon {
                    width: 40px;
                    height: 40px;
                    background: rgba(248, 187, 217, 0.2);
                    border-radius: 8px;
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    font-size: 18px;
                }

                .resource-title-text h4 {
                    margin: 0 0 4px 0;
                    font-size: 16px;
                    font-weight: 600;
                }

                .resource-subtitle {
                    font-size: 12px;
                    opacity: 0.8;
                }

                .resource-controls {
                    display: flex;
                    align-items: center;
                    gap: 12px;
                }

                .resource-stats {
                    display: flex;
                    gap: 16px;
                    font-size: 12px;
                    opacity: 0.9;
                }

                .stat-badge {
                    background: rgba(255, 255, 255, 0.2);
                    padding: 4px 8px;
                    border-radius: 12px;
                    display: flex;
                    align-items: center;
                    gap: 4px;
                }

                .expand-toggle {
                    background: rgba(255, 255, 255, 0.1);
                    border: 1px solid rgba(255, 255, 255, 0.2);
                    color: white;
                    padding: 8px 12px;
                    border-radius: 6px;
                    cursor: pointer;
                    transition: all 0.2s ease;
                    display: flex;
                    align-items: center;
                    gap: 6px;
                    font-size: 12px;
                    font-weight: 500;
                }

                .expand-toggle:hover {
                    background: rgba(255, 255, 255, 0.2);
                    border-color: rgba(255, 255, 255, 0.3);
                }

                .expand-icon {
                    transition: transform 0.3s ease;
                }

                .fhir-resource.collapsed .expand-icon {
                    transform: rotate(-90deg);
                }

                /* Resource Content */
                .resource-content {
                    padding: 0;
                }

                .field-mappings {
                    display: flex;
                    flex-direction: column;
                }

                .field-mapping {
                    display: flex;
                    align-items: center;
                    padding: 20px 24px;
                    border-bottom: 1px solid #f1f5f9;
                    transition: background 0.2s ease;
                    position: relative;
                }

                .field-mapping:hover {
                    background: linear-gradient(135deg, #fafbff 0%, #f8f9ff 100%);
                }

                .field-mapping:last-child {
                    border-bottom: none;
                }

                .field-mapping.required::before {
                    content: '';
                    position: absolute;
                    left: 0;
                    top: 0;
                    bottom: 0;
                    width: 4px;
                    background: linear-gradient(135deg, #dc2626 0%, #ef4444 100%);
                }

                .field-mapping.optional::before {
                    content: '';
                    position: absolute;
                    left: 0;
                    top: 0;
                    bottom: 0;
                    width: 4px;
                    background: linear-gradient(135deg, #059669 0%, #10b981 100%);
                }

                /* Field Information */
                .hl7-field-info, .fhir-field-info {
                    flex: 1;
                    max-width: 300px;
                }

                .field-path {
                    font-family: 'Courier New', monospace;
                    font-weight: 600;
                    font-size: 14px;
                    margin-bottom: 6px;
                }

                .hl7-field-info .field-path {
                    color: #059669;
                }

                .fhir-field-info .field-path {
                    color: #7c3aed;
                }

                .field-value {
                    font-family: 'Courier New', monospace;
                    font-size: 13px;
                    padding: 4px 8px;
                    border-radius: 4px;
                    margin-bottom: 6px;
                    display: inline-block;
                    max-width: 100%;
                    overflow: hidden;
                    text-overflow: ellipsis;
                    white-space: nowrap;
                }

                .hl7-field-info .field-value {
                    background: #dcfce7;
                    color: #166534;
                    border: 1px solid #bbf7d0;
                }

                .fhir-field-info .field-value {
                    background: #e0e7ff;
                    color: #3730a3;
                    border: 1px solid #c7d2fe;
                }

                .field-description {
                    font-size: 11px;
                    color: #6b7280;
                    font-style: italic;
                    line-height: 1.3;
                }

                .mapping-arrow {
                    margin: 0 24px;
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    width: 40px;
                    height: 40px;
                    background: linear-gradient(135deg, #f8bbd9 0%, #fce7f3 100%);
                    border-radius: 50%;
                    color: #1e3a8a;
                    font-size: 18px;
                    font-weight: bold;
                    box-shadow: 0 2px 4px rgba(248, 187, 217, 0.3);
                }

                .mapping-actions {
                    display: flex;
                    flex-direction: column;
                    gap: 8px;
                    margin-left: auto;
                }

                .mapping-btn {
                    padding: 8px 14px;
                    border: none;
                    border-radius: 6px;
                    font-size: 12px;
                    font-weight: 500;
                    cursor: pointer;
                    transition: all 0.2s ease;
                    display: flex;
                    align-items: center;
                    gap: 6px;
                    min-width: 100px;
                    justify-content: center;
                }

                .mapping-btn.edit {
                    background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
                    color: #92400e;
                    border: 1px solid #f59e0b;
                }

                .mapping-btn.edit:hover {
                    background: linear-gradient(135deg, #fde68a 0%, #fcd34d 100%);
                    transform: translateY(-1px);
                    box-shadow: 0 2px 4px rgba(245, 158, 11, 0.2);
                }

                /* Modal Styles */
                .edit-mapping-overlay, .json-viewer-overlay, .config-save-overlay {
                    position: fixed;
                    top: 0;
                    left: 0;
                    right: 0;
                    bottom: 0;
                    background: rgba(0, 0, 0, 0.7);
                    display: none;
                    align-items: center;
                    justify-content: center;
                    z-index: 10000;
                    padding: 20px;
                    backdrop-filter: blur(4px);
                }

                .edit-mapping-overlay.show, .json-viewer-overlay.show, .config-save-overlay.show {
                    display: flex;
                }

                .edit-mapping-modal, .json-viewer-modal, .config-save-modal {
                    background: white;
                    border-radius: 16px;
                    width: 100%;
                    max-width: 800px;
                    max-height: 90vh;
                    overflow: hidden;
                    border: 2px solid #f8bbd9;
                    box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
                    animation: modalSlideIn 0.3s ease;
                }

                @keyframes modalSlideIn {
                    from {
                        opacity: 0;
                        transform: scale(0.95) translateY(-20px);
                    }
                    to {
                        opacity: 1;
                        transform: scale(1) translateY(0);
                    }
                }

                .modal-header {
                    background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
                    color: white;
                    padding: 20px 24px;
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    border-bottom: 2px solid #f8bbd9;
                }

                .modal-header h4 {
                    margin: 0;
                    font-size: 18px;
                    font-weight: 600;
                    display: flex;
                    align-items: center;
                    gap: 12px;
                }

                .modal-icon {
                    font-size: 20px;
                }

                .header-actions {
                    display: flex;
                    align-items: center;
                    gap: 12px;
                }

                .header-btn {
                    background: rgba(255, 255, 255, 0.15);
    border: 1px solid rgba(255, 255, 255, 0.3);
    color: white !important;
    padding: 8px 14px;
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.2s ease;
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    font-weight: 500;
                }

                .header-btn:hover {
                    background: rgba(255, 255, 255, 0.25);
    transform: translateY(-1px);
                }

                .modal-close {
                    background: rgba(255, 255, 255, 0.2);
    border: 1px solid rgba(255, 255, 255, 0.3);
    color: white !important;
    font-size: 24px;
    font-weight: bold;
    cursor: pointer;
    padding: 4px 12px;
    border-radius: 6px;
    transition: all 0.2s ease;
    line-height: 1;
    min-width: 40px;
    text-align: center;
                }

                .modal-close:hover {
                    background: rgba(255, 255, 255, 0.3);
    transform: scale(1.1);
                }

                .modal-content {
                    padding: 24px;
                    max-height: 60vh;
                    overflow-y: auto;
                }

                /* Edit Modal Specific */
                .current-mapping-display {
                    margin-bottom: 24px;
                }

                .mapping-card {
                    background: linear-gradient(135deg, #f8f9ff 0%, #e0e7ff 100%);
                    border: 2px solid #e2e8f0;
                    border-radius: 12px;
                    padding: 20px;
                    display: flex;
                    align-items: center;
                    gap: 20px;
                }

                .mapping-side {
                    flex: 1;
                    text-align: center;
                }

                .side-header {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    gap: 8px;
                    margin-bottom: 12px;
                }

                .side-icon {
                    font-size: 18px;
                }

                .side-title {
                    font-size: 14px;
                    font-weight: 600;
                    color: #1e3a8a;
                }

                .field-display .field-path {
                    font-family: 'Courier New', monospace;
                    font-weight: 600;
                    font-size: 14px;
                    color: #1e3a8a;
                    margin-bottom: 8px;
                }

                .field-display .field-value {
                    font-family: 'Courier New', monospace;
                    font-size: 12px;
                    padding: 4px 8px;
                    border-radius: 4px;
                    margin-bottom: 8px;
                    background: white;
                    border: 1px solid #e2e8f0;
                    color: #059669;
                }

                .field-display .field-description {
                    font-size: 11px;
                    color: #6b7280;
                    font-style: italic;
                }

                .mapping-connector {
                    display: flex;
                    flex-direction: column;
                    align-items: center;
                    position: relative;
                }

                .connector-line {
                    width: 2px;
                    height: 40px;
                    background: linear-gradient(135deg, #f8bbd9 0%, #fce7f3 100%);
                }

                .connector-arrow {
                    background: linear-gradient(135deg, #f8bbd9 0%, #fce7f3 100%);
                    color: #1e3a8a;
                    width: 30px;
                    height: 30px;
                    border-radius: 50%;
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    font-weight: bold;
                    font-size: 16px;
                    position: absolute;
                    top: 50%;
                    transform: translateY(-50%);
                }

                /* Configuration Section */
                .edit-configuration {
                    background: white;
                    border: 2px solid #f1f5f9;
                    border-radius: 12px;
                    padding: 20px;
                }

                .config-section h5 {
                    margin: 0 0 16px 0;
                    font-size: 16px;
                    font-weight: 600;
                    color: #1e3a8a;
                    display: flex;
                    align-items: center;
                    gap: 8px;
                    padding-bottom: 12px;
                    border-bottom: 2px solid #f8bbd9;
                }

                .config-grid {
                    display: grid;
                    grid-template-columns: 1fr 1fr;
                    gap: 20px;
                    margin-bottom: 20px;
                }

                .config-group {
                    display: flex;
                    flex-direction: column;
                }

                .config-label {
                    font-size: 13px;
                    font-weight: 600;
                    color: #1e3a8a;
                    margin-bottom: 8px;
                }

                .config-select {
                    padding: 12px;
                    border: 2px solid #e2e8f0;
                    border-radius: 8px;
                    font-size: 14px;
                    transition: border-color 0.2s ease;
                    margin-bottom: 8px;
                }

                .config-select:focus {
                    outline: none;
                    border-color: #f8bbd9;
                    box-shadow: 0 0 0 3px rgba(248, 187, 217, 0.1);
                }

                .field-preview {
                    background: #f8f9ff;
                    border: 1px solid #e2e8f0;
                    border-radius: 6px;
                    padding: 8px 12px;
                    font-size: 12px;
                }

                .preview-label {
                    color: #6b7280;
                    font-weight: 500;
                    margin-right: 8px;
                }

                .preview-value {
                    color: #1e3a8a;
                    font-weight: 600;
                }

                /* Transformation Options */
                .transformation-options {
                    margin-bottom: 20px;
                }

                .transform-grid {
                    display: grid;
                    grid-template-columns: 1fr 1fr;
                    gap: 12px;
                }

                .transform-option {
                    display: flex;
                    align-items: center;
                    gap: 12px;
                    padding: 12px;
                    border: 2px solid #e2e8f0;
                    border-radius: 8px;
                    cursor: pointer;
                    transition: all 0.2s ease;
                }

                .transform-option:hover {
                    border-color: #f8bbd9;
                    background: #fafbff;
                }

                .transform-option input[type="radio"] {
                    accent-color: #1e3a8a;
                }

                .option-content {
                    display: flex;
                    flex-direction: column;
                }

                .option-title {
                    font-size: 13px;
                    font-weight: 600;
                    color: #1e3a8a;
                    margin-bottom: 2px;
                }

                .option-desc {
                    font-size: 11px;
                    color: #6b7280;
                }

                /* Test Transformation */
                .test-transformation {
                    background: #f8f9ff;
                    border: 2px solid #e2e8f0;
                    border-radius: 8px;
                    padding: 16px;
                }

                .test-result {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    gap: 16px;
                }

                .test-before, .test-after {
                    flex: 1;
                    text-align: center;
                    background: white;
                    border: 1px solid #e2e8f0;
                    border-radius: 6px;
                    padding: 12px;
                }

                .test-label {
                    font-size: 11px;
                    color: #6b7280;
                    font-weight: 600;
                    text-transform: uppercase;
                    margin-bottom: 4px;
                    display: block;
                }

                .test-value {
                    font-family: 'Courier New', monospace;
                    font-size: 12px;
                    color: #1e3a8a;
                    font-weight: 600;
                }

                .test-arrow {
                    color: #f8bbd9;
                    font-size: 18px;
                    font-weight: bold;
                }

                /* Modal Actions */
                .modal-actions {
                    padding: 20px 24px;
                    background: linear-gradient(135deg, #f8f9ff 0%, #ffffff 100%);
                    border-top: 2px solid #f1f5f9;
                    display: flex;
                    justify-content: flex-end;
                    gap: 12px;
                }

                .modal-btn {
                    padding: 12px 20px;
                    border: none;
                    border-radius: 8px;
                    font-size: 14px;
                    font-weight: 600;
                    cursor: pointer;
                    transition: all 0.2s ease;
                    display: flex;
                    align-items: center;
                    gap: 8px;
                    min-width: 120px;
                    justify-content: center;
                }

                .modal-btn.primary {
                     background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
    color: white !important; /* Force white text */
    box-shadow: 0 2px 4px rgba(30, 58, 138, 0.2);
                }

                .modal-btn.primary:hover {
                     transform: translateY(-1px);
    box-shadow: 0 4px 8px rgba(30, 58, 138, 0.3);
    background: linear-gradient(135deg, #1e40af 0%, #1e3a8a 100%);
                }

                .modal-btn.secondary {
                    background: white;
    color: #1e3a8a !important; /* Force blue text */
    border: 2px solid #e2e8f0;
                }

                .modal-btn.secondary:hover {
                    background: #f8f9ff;
    border-color: #f8bbd9;
    transform: translateY(-1px);
                }

                /* JSON Viewer */
                .json-viewer-modal {
                    max-width: 1000px;
                }

                .json-content {
                    padding: 0;
                }

                .json-tabs {
                    display: flex;
                    background: #f8f9ff;
                    border-bottom: 2px solid #f1f5f9;
                }

                .json-tab {
                    padding: 12px 20px;
                    border: none;
                    background: none;
                    color: #6b7280;
                    cursor: pointer;
                    font-size: 14px;
                    font-weight: 500;
                    border-bottom: 3px solid transparent;
                    transition: all 0.2s ease;
                }

                .json-tab.active {
                    color: #1e3a8a;
                    border-bottom-color: #f8bbd9;
                    background: white;
                }

                .json-display {
                    padding: 20px;
                    max-height: 60vh;
                    overflow: auto;
                    background: #fafbff;
                }

                #jsonOutput {
                    background: white;
                    border: 2px solid #e2e8f0;
                    border-radius: 8px;
                    padding: 16px;
                    margin: 0;
                    font-family: 'Courier New', monospace;
                    font-size: 12px;
                    line-height: 1.5;
                    color: #1e3a8a;
                    white-space: pre-wrap;
                    word-wrap: break-word;
                }

                /* Validation Sidebar */
                .validation-sidebar {
                    position: fixed;
                    right: -400px;
                    top: 0;
                    width: 380px;
                    height: 100vh;
                    background: white;
                    border-left: 2px solid #f8bbd9;
                    box-shadow: -4px 0 20px rgba(0, 0, 0, 0.1);
                    transition: right 0.3s ease;
                    z-index: 1000;
                    display: flex;
                    flex-direction: column;
                }

                .validation-sidebar.show {
                    right: 0;
                }

                .sidebar-header {
                    background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
                    color: white;
                    padding: 16px 20px;
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    border-bottom: 2px solid #f8bbd9;
                }

                .sidebar-header h4 {
                    margin: 0;
                    font-size: 16px;
                    font-weight: 600;
                    display: flex;
                    align-items: center;
                    gap: 8px;
                }

                .sidebar-icon {
                    font-size: 18px;
                }

                .sidebar-close {
                    background: transparent;
    border: 2px solid #dc2626;
    color: #dc2626 !important;
    font-size: 20px;
    font-weight: bold;
    width: 32px;
    height: 32px;
    border-radius: 50%;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.2s ease;
                }

                .sidebar-close:hover {
                    background: #dc2626;
    color: white !important;
    transform: rotate(90deg)
                }

                .validation-summary {
                    display: grid;
                    grid-template-columns: 1fr 1fr 1fr;
                    gap: 1px;
                    background: #f1f5f9;
                    border-bottom: 2px solid #f8bbd9;
                }

                .validation-metric {
                    background: white;
                    padding: 16px 12px;
                    text-align: center;
                }

                .validation-metric.success .metric-number {
                    color: #059669;
                }

                .validation-metric.warning .metric-number {
                    color: #f59e0b;
                }

                .validation-metric.error .metric-number {
                    color: #dc2626;
                }

                .validation-metric .metric-number {
                    font-size: 24px;
                    font-weight: 700;
                    line-height: 1;
                    margin-bottom: 4px;
                }

                .validation-metric .metric-label {
                    font-size: 11px;
                    color: #6b7280;
                    text-transform: uppercase;
                    font-weight: 500;
                }

                .validation-content {
                    flex: 1;
                    padding: 20px;
                    overflow-y: auto;
                }

                .validation-item {
                    padding: 12px;
                    margin-bottom: 12px;
                    border-radius: 8px;
                    font-size: 13px;
                    border-left: 4px solid;
                    background: white;
                    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
                }

                .validation-item.success {
                    border-left-color: #059669;
                    background: #f0fdf4;
                    color: #166534;
                }

                .validation-item.warning {
                    border-left-color: #f59e0b;
                    background: #fffbeb;
                    color: #92400e;
                }

                .validation-item.error {
                    border-left-color: #dc2626;
                    background: #fef2f2;
                    color: #991b1b;
                }

                .validation-item-header {
                    font-weight: 600;
                    margin-bottom: 4px;
                    display: flex;
                    align-items: center;
                    gap: 6px;
                }

                /* Config Save Modal */
                .config-form {
                    display: flex;
                    flex-direction: column;
                    gap: 24px;
                }

                .form-section h5 {
                    margin: 0 0 16px 0;
                    font-size: 16px;
                    font-weight: 600;
                    color: #1e3a8a;
                    padding-bottom: 8px;
                    border-bottom: 2px solid #f8bbd9;
                }

                .form-grid {
                    display: grid;
                    grid-template-columns: 1fr 1fr;
                    gap: 16px;
                }

                .form-group {
                    display: flex;
                    flex-direction: column;
                }

                .form-label {
                    font-size: 13px;
                    font-weight: 600;
                    color: #1e3a8a;
                    margin-bottom: 6px;
                }

                .form-input, .form-textarea {
                    padding: 12px;
                    border: 2px solid #e2e8f0;
                    border-radius: 8px;
                    font-size: 14px;
                    transition: border-color 0.2s ease;
                }

                .form-input:focus, .form-textarea:focus {
                    outline: none;
                    border-color: #f8bbd9;
                    box-shadow: 0 0 0 3px rgba(248, 187, 217, 0.1);
                }

                .form-textarea {
                    resize: vertical;
                    min-height: 80px;
                }

                .config-summary-section {
                    background: linear-gradient(135deg, #f8f9ff 0%, #e0e7ff 100%);
                    border: 2px solid #e2e8f0;
                    border-radius: 8px;
                    padding: 16px;
                }

                .summary-stats {
                    display: flex;
                    justify-content: space-between;
                    gap: 16px;
                }

                .stat-item {
                    text-align: center;
                }

                .stat-label {
                    font-size: 12px;
                    color: #6b7280;
                    margin-bottom: 4px;
                }

                .stat-value {
                    font-size: 18px;
                    font-weight: 600;
                    color: #1e3a8a;
                }

                .config-options {
                    border: 2px solid #f1f5f9;
                    border-radius: 8px;
                    padding: 16px;
                }

                .checkbox-option {
                    display: flex;
                    align-items: flex-start;
                    gap: 12px;
                    cursor: pointer;
                }

                .checkbox-option input[type="checkbox"] {
                    accent-color: #1e3a8a;
                    width: 16px;
                    height: 16px;
                    margin-top: 2px;
                }

                .checkbox-content {
                    flex: 1;
                }

                .option-title {
                    font-size: 14px;
                    font-weight: 600;
                    color: #1e3a8a;
                    margin-bottom: 2px;
                }

                .option-desc {
                    font-size: 12px;
                    color: #6b7280;
                }

                /* Placeholder */
                .transformation-placeholder {
                    text-align: center;
                    padding: 60px 40px;
                    background: linear-gradient(135deg, #f8f9ff 0%, #ffffff 100%);
                    border: 2px solid #f8bbd9;
                    border-radius: 16px;
                    margin: 20px 0;
                }

                .placeholder-icon {
                    font-size: 80px;
                    margin-bottom: 24px;
                    background: linear-gradient(135deg, #1e3a8a 0%, #3b82f6 100%);
                    -webkit-background-clip: text;
                    -webkit-text-fill-color: transparent;
                }

                .placeholder-content h4 {
                    margin: 0 0 16px 0;
                    font-size: 24px;
                    font-weight: 600;
                    color: #1e3a8a;
                }

                .placeholder-content p {
                    color: #6b7280;
                    margin: 0 0 24px 0;
                    font-size: 16px;
                    line-height: 1.5;
                }

                .placeholder-actions {
                    display: flex;
                    justify-content: center;
                    gap: 16px;
                }

                /* Responsive Design */
                @media (max-width: 768px) {
                    .transformation-summary {
                        padding: 16px;
                    }
                    
                    .summary-header {
                        flex-direction: column;
                        gap: 16px;
                        align-items: stretch;
                    }
                    
                    .summary-grid {
                        grid-template-columns: 1fr 1fr;
                        gap: 12px;
                    }
                    
                    .mapping-overview {
                        flex-direction: column;
                        align-items: stretch;
                        gap: 12px;
                    }
                    
                    .field-mapping {
                        flex-direction: column;
                        align-items: stretch;
                        gap: 16px;
                    }
                    
                    .mapping-arrow {
                        margin: 0;
                        transform: rotate(90deg);
                    }
                    
                    .config-grid, .transform-grid, .form-grid {
                        grid-template-columns: 1fr;
                    }
                    
                    .validation-sidebar {
                        width: 100%;
                        right: -100%;
                    }
                    
                    .edit-mapping-modal, .json-viewer-modal, .config-save-modal {
                        max-width: 100%;
                        max-height: 95vh;
                        margin: 10px;
                    }
                }

                /* Loading and Animation States */
                .loading-shimmer {
                    background: linear-gradient(90deg, #f1f5f9 25%, #e2e8f0 50%, #f1f5f9 75%);
                    background-size: 200% 100%;
                    animation: shimmer 2s infinite;
                }

                @keyframes shimmer {
                    0% { background-position: -200% 0; }
                    100% { background-position: 200% 0; }
                }

                .fade-in {
                    animation: fadeIn 0.5s ease;
                }

                @keyframes fadeIn {
                    from { opacity: 0; transform: translateY(10px); }
                    to { opacity: 1; transform: translateY(0); }
                }

                /* Utility Classes */
                .text-gradient {
                    background: linear-gradient(135deg, #1e3a8a 0%, #3b82f6 100%);
                    -webkit-background-clip: text;
                    -webkit-text-fill-color: transparent;
                }

                .border-gradient {
                    border: 2px solid transparent;
                    background: linear-gradient(white, white) padding-box,
                                linear-gradient(135deg, #f8bbd9, #fce7f3) border-box;
                }
            </style>
        `;

        console.log('✅ Enhanced FHIR Transformation Results interface loaded');
        this.setupEventListeners();
    }

    setupEventListeners() {
        // Summary actions
        const viewJsonBtn = document.getElementById('viewJsonBtn');
        const saveConfigBtn = document.getElementById('saveConfigBtn');
        const expandAllBtn = document.getElementById('expandAllBtn');
        const validateMappingsBtn = document.getElementById('validateMappingsBtn');

        if (viewJsonBtn) viewJsonBtn.addEventListener('click', () => this.showJsonViewer());
        if (saveConfigBtn) saveConfigBtn.addEventListener('click', () => this.showConfigSaveModal());
        if (expandAllBtn) expandAllBtn.addEventListener('click', () => this.toggleAllResources());
        if (validateMappingsBtn) validateMappingsBtn.addEventListener('click', () => this.showValidationSidebar());

        console.log('✅ Enhanced FHIR Transformation event listeners setup');
    }

    async onStepActivated() {
        console.log('🎯 Enhanced FHIR Transformation Step 4 activated');

        const parsedData = this.getParsedHL7Data();
        if (!parsedData) {
            this.showPlaceholder();
            return;
        }

        this.hidePlaceholder();
        
        // Load transformation results and mapping configuration
        await this.loadTransformationResults();
        await this.loadMappingConfiguration();
        
        console.log('✅ Enhanced FHIR Transformation results loaded');
    }

    async loadTransformationResults() {
        try {
            console.log('🔄 Loading enhanced FHIR transformation results...');

            const parsedData = this.getParsedHL7Data();
            if (!parsedData) {
                throw new Error('No parsed HL7 data available');
            }

            const payload = {
                parsedHL7Data: this.wizard.parsedHL7Data,
                createBundle: true,
                validationMode: 'strict'
            };

            const response = await fetch(`${this.apiBaseUrl}/api/fhir/test-transform-v3`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(payload)
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            this.transformationData = await response.json();
            console.log('✅ Enhanced transformation data loaded:', this.transformationData);

            this.displayEnhancedResults();

        } catch (error) {
            console.error('❌ Failed to load transformation results:', error);
            this.showError(error.message);
        }
    }

    async loadMappingConfiguration() {
        try {
            // Load mapping configuration from database/API
            // This would typically be an API call to get saved mapping configurations
            const messageType = this.transformationData?.messageType || 'ADT^A04';
            
            // For now, we'll simulate this - in real implementation, this would be:
            // const response = await fetch(`${this.apiBaseUrl}/api/mapping-config/${messageType}`);
            // this.mappingConfiguration = await response.json();
            
            console.log('✅ Mapping configuration loaded for:', messageType);
        } catch (error) {
            console.error('⚠️ Could not load mapping configuration:', error);
        }
    }

    displayEnhancedResults() {
        if (!this.transformationData) return;

        this.updateSummaryMetrics();
        this.updateMappingOverview();
        this.displayFHIRResourcesWithMappings();
    }

    updateSummaryMetrics() {
        const hl7SegmentCount = document.getElementById('hl7SegmentCount');
        const fhirResourceCount = document.getElementById('fhirResourceCount');
        const successfulMappings = document.getElementById('successfulMappings');
        const validationScore = document.getElementById('validationScore');

        const parsedData = this.getParsedHL7Data();
        const segmentCount = parsedData ? Object.keys(parsedData.enhancedSegments || {}).length : 0;
        const resourceCount = this.transformationData.fhirResources?.length || 0;
        const mappingCount = this.transformationData.mappingStats?.totalFieldsMapped || 0;
        
        // Calculate validation score based on errors/warnings for mapped resources only
        const filteredErrors = this.getFilteredValidationErrors();
        const filteredWarnings = this.getFilteredValidationWarnings();
        const totalValidation = mappingCount;
        const successfulValidation = Math.max(0, totalValidation - filteredErrors.length);
        const validationPercentage = totalValidation > 0 ? Math.round((successfulValidation / totalValidation) * 100) : 100;

        if (hl7SegmentCount) hl7SegmentCount.textContent = segmentCount;
        if (fhirResourceCount) fhirResourceCount.textContent = resourceCount;
        if (successfulMappings) successfulMappings.textContent = mappingCount;
        if (validationScore) validationScore.textContent = `${validationPercentage}%`;
    }

    updateMappingOverview() {
        const messageTypeOverview = document.getElementById('messageTypeOverview');
        const transformationStatus = document.getElementById('transformationStatus');
        const processingTime = document.getElementById('processingTime');

        if (messageTypeOverview) {
            messageTypeOverview.textContent = this.transformationData.messageType || 'Unknown';
        }

        if (transformationStatus) {
            const success = this.transformationData.success !== false;
            transformationStatus.textContent = success ? 'Successful' : 'Has Issues';
            transformationStatus.className = `overview-value status ${success ? 'success' : 'error'}`;
        }

        if (processingTime) {
            const time = this.transformationData.performance?.totalTime || '-';
            processingTime.textContent = time;
        }
    }

    displayFHIRResourcesWithMappings() {
        const container = document.getElementById('fhirResourcesContainer');
        if (!container || !this.transformationData.fhirResources) return;

        let html = '';
        
        this.transformationData.fhirResources.forEach((resource, index) => {
            const mappings = this.extractDetailedMappings(resource);
            const resourceIcon = this.getResourceIcon(resource.resourceType);
            
            html += `
                <div class="fhir-resource collapsed" data-resource-type="${resource.resourceType}" id="resource-${index}">
                    <div class="resource-header" onclick="window.fhirHandler.toggleResource(${index})">
                        <div class="resource-title">
                            <div class="resource-icon">${resourceIcon}</div>
                            <div class="resource-title-text">
                                <h4>${resource.resourceType}</h4>
                                <div class="resource-subtitle">Resource ID: ${resource.id}</div>
                            </div>
                        </div>
                        <div class="resource-controls">
                            <div class="resource-stats">
                                <div class="stat-badge">
                                    <span>🔗</span>
                                    <span>${mappings.length} mappings</span>
                                </div>
                                <div class="stat-badge">
                                    <span>📊</span>
                                    <span>${mappings.filter(m => m.required).length} required</span>
                                </div>
                            </div>
                            <button class="expand-toggle">
                                <span class="expand-icon">▼</span>
                                <span class="expand-text">Expand</span>
                            </button>
                        </div>
                    </div>
                    <div class="resource-content">
                        <div class="field-mappings">
                            ${mappings.map(mapping => this.createEnhancedMappingHTML(mapping, resource.resourceType, index)).join('')}
                        </div>
                    </div>
                </div>
            `;
        });

        container.innerHTML = html;
    }

    extractDetailedMappings(resource) {
        const mappings = [];
        
        if (resource.resourceType === 'Patient') {
            // Patient identifiers
            if (resource.identifier && resource.identifier.length > 0) {
                resource.identifier.forEach((id, index) => {
                    if (id.value) {
                        mappings.push({
                            hl7Field: index === 0 ? 'PID.3.1' : 'PID.3.4',
                            hl7Value: id.value,
                            hl7Description: 'Patient ID',
                            fhirPath: `Patient.identifier[${index}].value`,
                            fhirValue: id.value,
                            fhirType: 'string',
                            fhirDescription: 'Unique patient identifier',
                            required: true,
                            transformType: 'direct'
                        });
                    }
                });
            }

            // Patient name
            if (resource.name && resource.name.length > 0) {
                const name = resource.name[0];
                if (name.family) {
                    mappings.push({
                        hl7Field: 'PID.5.1',
                        hl7Value: name.family,
                        hl7Description: 'Family name from patient name field',
                        fhirPath: 'Patient.name[0].family',
                        fhirValue: name.family,
                        fhirType: 'string',
                        fhirDescription: 'Family name part of patient name',
                        required: true,
                        transformType: 'direct'
                    });
                }
                if (name.given && name.given.length > 0) {
                    name.given.forEach((given, index) => {
                        mappings.push({
                            hl7Field: `PID.5.${index + 2}`,
                            hl7Value: given,
                            hl7Description: `Given name ${index + 1} from patient name field`,
                            fhirPath: `Patient.name[0].given[${index}]`,
                            fhirValue: given,
                            fhirType: 'string',
                            fhirDescription: `Given name part ${index + 1} of patient name`,
                            required: true,
                            transformType: 'direct'
                        });
                    });
                }
            }

            // Birth date
            if (resource.birthDate) {
                mappings.push({
                    hl7Field: 'PID.7',
                    hl7Value: resource.birthDate,
                    hl7Description: 'Patient date of birth',
                    fhirPath: 'Patient.birthDate',
                    fhirValue: resource.birthDate,
                    fhirType: 'date',
                    fhirDescription: 'Patient birth date in YYYY-MM-DD format',
                    required: false,
                    transformType: 'format'
                });
            }

            // Gender
            if (resource.gender) {
                mappings.push({
                    hl7Field: 'PID.8',
                    hl7Value: resource.gender.toUpperCase(),
                    hl7Description: 'Administrative sex/gender',
                    fhirPath: 'Patient.gender',
                    fhirValue: resource.gender,
                    fhirType: 'code',
                    fhirDescription: 'Administrative gender (male|female|other|unknown)',
                    required: false,
                    transformType: 'lookup'
                });
            }

            // Telecom (phone/email)
            if (resource.telecom && resource.telecom.length > 0) {
                resource.telecom.forEach((telecom, index) => {
                    if (telecom.value) {
                        const fieldNum = telecom.system === 'phone' ? '13' : '14';
                        mappings.push({
                            hl7Field: `PID.${fieldNum}`,
                            hl7Value: telecom.value,
                            hl7Description: `Patient ${telecom.system} number`,
                            fhirPath: `Patient.telecom[${index}].value`,
                            fhirValue: telecom.value,
                            fhirType: 'string',
                            fhirDescription: `Contact ${telecom.system} value`,
                            required: false,
                            transformType: 'direct'
                        });
                    }
                });
            }

            // Address
            if (resource.address && resource.address.length > 0) {
                const address = resource.address[0];
                if (address.line && address.line.length > 0) {
                    mappings.push({
                        hl7Field: 'PID.11.1',
                        hl7Value: address.line[0],
                        hl7Description: 'Street address line 1',
                        fhirPath: 'Patient.address[0].line[0]',
                        fhirValue: address.line[0],
                        fhirType: 'string',
                        fhirDescription: 'Street address line',
                        required: false,
                        transformType: 'direct'
                    });
                }
                if (address.city) {
                    mappings.push({
                        hl7Field: 'PID.11.3',
                        hl7Value: address.city,
                        hl7Description: 'City name',
                        fhirPath: 'Patient.address[0].city',
                        fhirValue: address.city,
                        fhirType: 'string',
                        fhirDescription: 'City or town name',
                        required: false,
                        transformType: 'direct'
                    });
                }
                if (address.state) {
                    mappings.push({
                        hl7Field: 'PID.11.4',
                        hl7Value: address.state,
                        hl7Description: 'State or province',
                        fhirPath: 'Patient.address[0].state',
                        fhirValue: address.state,
                        fhirType: 'string',
                        fhirDescription: 'State, province, or region',
                        required: false,
                        transformType: 'direct'
                    });
                }
                if (address.postalCode) {
                    mappings.push({
                        hl7Field: 'PID.11.5',
                        hl7Value: address.postalCode,
                        hl7Description: 'ZIP or postal code',
                        fhirPath: 'Patient.address[0].postalCode',
                        fhirValue: address.postalCode,
                        fhirType: 'string',
                        fhirDescription: 'Postal or ZIP code',
                        required: false,
                        transformType: 'direct'
                    });
                }
            }
        }

        if (resource.resourceType === 'MessageHeader') {
            // Source information
            if (resource.source) {
                if (resource.source.name) {
                    mappings.push({
                        hl7Field: 'MSH.3',
                        hl7Value: resource.source.name,
                        hl7Description: 'Sending application name',
                        fhirPath: 'MessageHeader.source.name',
                        fhirValue: resource.source.name,
                        fhirType: 'string',
                        fhirDescription: 'Name of the sending application',
                        required: true,
                        transformType: 'direct'
                    });
                }
                if (resource.source.endpoint) {
                    mappings.push({
                        hl7Field: 'MSH.4',
                        hl7Value: resource.source.endpoint,
                        hl7Description: 'Sending facility name',
                        fhirPath: 'MessageHeader.source.endpoint',
                        fhirValue: resource.source.endpoint,
                        fhirType: 'uri',
                        fhirDescription: 'Endpoint URI of the sending system',
                        required: true,
                        transformType: 'direct'
                    });
                }
            }

            // Event coding
            if (resource.eventCoding) {
                mappings.push({
                    hl7Field: 'MSH.9.2',
                    hl7Value: resource.eventCoding.code,
                    hl7Description: 'Message type trigger event',
                    fhirPath: 'MessageHeader.eventCoding.code',
                    fhirValue: resource.eventCoding.code,
                    fhirType: 'code',
                    fhirDescription: 'Event code that triggers the message',
                    required: true,
                    transformType: 'lookup'
                });
            }

            // Destination information
            if (resource.destination && resource.destination.length > 0) {
                const dest = resource.destination[0];
                if (dest.name) {
                    mappings.push({
                        hl7Field: 'MSH.5',
                        hl7Value: dest.name,
                        hl7Description: 'Receiving application name',
                        fhirPath: 'MessageHeader.destination[0].name',
                        fhirValue: dest.name,
                        fhirType: 'string',
                        fhirDescription: 'Name of the receiving application',
                        required: true,
                        transformType: 'direct'
                    });
                }
                if (dest.endpoint) {
                    mappings.push({
                        hl7Field: 'MSH.6',
                        hl7Value: dest.endpoint,
                        hl7Description: 'Receiving facility name',
                        fhirPath: 'MessageHeader.destination[0].endpoint',
                        fhirValue: dest.endpoint,
                        fhirType: 'uri',
                        fhirDescription: 'Endpoint URI of the receiving system',
                        required: true,
                        transformType: 'direct'
                    });
                }
            }
        }

        return mappings;
    }

    createEnhancedMappingHTML(mapping, resourceType, resourceIndex) {
        const statusClass = this.getMappingStatusClass(mapping);
        
        return `
            <div class="field-mapping ${mapping.required ? 'required' : 'optional'} ${statusClass} fade-in">
                <div class="hl7-field-info">
                    <div class="field-path">${mapping.hl7Field}</div>
                    <div class="field-value" title="${mapping.hl7Value}">"${this.truncateValue(mapping.hl7Value)}"</div>
                    <div class="field-description">${mapping.hl7Description}</div>
                </div>
                
                <div class="mapping-arrow">→</div>
                
                <div class="fhir-field-info">
                    <div class="field-path">${mapping.fhirPath}</div>
                    <div class="field-value" title="${mapping.fhirValue}">"${this.truncateValue(mapping.fhirValue)}"</div>
                    <div class="field-description">${mapping.fhirDescription} (${mapping.fhirType})</div>
                </div>
                
                <div class="mapping-actions">
                    <button class="mapping-btn edit" onclick="window.fhirHandler.editMapping('${mapping.fhirPath}', '${resourceType}', ${resourceIndex})">
                        <span class="btn-icon">⚙️</span>
                        Edit Mapping
                    </button>
                </div>
            </div>
        `;
    }

    truncateValue(value, maxLength = 30) {
        if (!value) return '';
        const str = String(value);
        return str.length > maxLength ? str.substring(0, maxLength) + '...' : str;
    }

    getMappingStatusClass(mapping) {
        const filteredErrors = this.getFilteredValidationErrors();
        const filteredWarnings = this.getFilteredValidationWarnings();
        
        const hasError = filteredErrors.some(error => 
            error.includes(mapping.fhirPath) || error.includes(mapping.hl7Field)
        );
        const hasWarning = filteredWarnings.some(warning => 
            warning.includes(mapping.fhirPath) || warning.includes(mapping.hl7Field)
        );
        
        if (hasError) return 'error';
        if (hasWarning) return 'warning';
        return '';
    }

    getFilteredValidationErrors() {
        if (!this.transformationData.errors) return [];
        
        // Only include errors for resources that were actually created
        const createdResources = this.transformationData.fhirResources.map(r => r.resourceType);
        
        return this.transformationData.errors.filter(error => {
            return createdResources.some(resourceType => 
                error.includes(resourceType) || 
                error.toLowerCase().includes(resourceType.toLowerCase())
            );
        });
    }

    getFilteredValidationWarnings() {
        if (!this.transformationData.warnings) return [];
        
        // Only include warnings for resources that were actually created
        const createdResources = this.transformationData.fhirResources.map(r => r.resourceType);
        
        return this.transformationData.warnings.filter(warning => {
            return createdResources.some(resourceType => 
                warning.includes(resourceType) || 
                warning.toLowerCase().includes(resourceType.toLowerCase())
            );
        });
    }

    getResourceIcon(resourceType) {
        const icons = {
            'Patient': '👤',
            'MessageHeader': '📨',
            'Encounter': '🏥',
            'Observation': '📊',
            'Condition': '🩺',
            'AllergyIntolerance': '⚠️',
            'Practitioner': '👨‍⚕️',
            'Organization': '🏢'
        };
        return icons[resourceType] || '📄';
    }

    // UI Interaction Methods
    toggleResource(resourceIndex) {
        const resource = document.getElementById(`resource-${resourceIndex}`);
        const expandIcon = resource.querySelector('.expand-icon');
        const expandText = resource.querySelector('.expand-text');
        
        if (resource.classList.contains('collapsed')) {
            resource.classList.remove('collapsed');
            this.expandedResources.add(resourceIndex);
            expandText.textContent = 'Collapse';
        } else {
            resource.classList.add('collapsed');
            this.expandedResources.delete(resourceIndex);
            expandText.textContent = 'Expand';
        }
    }

    toggleAllResources() {
        const expandAllBtn = document.getElementById('expandAllBtn');
        const resources = document.querySelectorAll('.fhir-resource');
        const allExpanded = this.expandedResources.size === resources.length;
        
        resources.forEach((resource, index) => {
            const expandIcon = resource.querySelector('.expand-icon');
            const expandText = resource.querySelector('.expand-text');
            
            if (allExpanded) {
                resource.classList.add('collapsed');
                this.expandedResources.delete(index);
                expandText.textContent = 'Expand';
                expandAllBtn.innerHTML = '<span class="btn-icon">📖</span> Expand All';
            } else {
                resource.classList.remove('collapsed');
                this.expandedResources.add(index);
                expandText.textContent = 'Collapse';
                expandAllBtn.innerHTML = '<span class="btn-icon">📕</span> Collapse All';
            }
        });
    }

    editMapping(fhirPath, resourceType, resourceIndex) {
        console.log(`🔧 Editing mapping for: ${fhirPath}`);
        
        // Find the mapping from the current transformations
        const resource = this.transformationData.fhirResources[resourceIndex];
        const mappings = this.extractDetailedMappings(resource);
        const mapping = mappings.find(m => m.fhirPath === fhirPath);
        
        if (mapping) {
            this.showEditModal(mapping);
        } else {
            this.showNotification('Could not find mapping details', 'error');
        }
    }

    showEditModal(mapping) {
        const modal = document.getElementById('editMappingOverlay');
        
        // Populate current mapping display
        document.getElementById('currentHL7Path').textContent = mapping.hl7Field;
        document.getElementById('currentHL7Value').textContent = `"${mapping.hl7Value}"`;
        document.getElementById('currentHL7Desc').textContent = mapping.hl7Description;
        
        document.getElementById('currentFHIRPath').textContent = mapping.fhirPath;
        document.getElementById('currentFHIRValue').textContent = `"${mapping.fhirValue}"`;
        document.getElementById('currentFHIRDesc').textContent = mapping.fhirDescription;
        
        // Populate dropdown options
        this.populateHL7FieldOptions();
        this.populateFHIRFieldOptions();
        
        // Set current values
        document.getElementById('newHL7Field').value = mapping.hl7Field;
        document.getElementById('newFHIRField').value = mapping.fhirPath;
        
        // Set transformation type
        const transformRadios = document.querySelectorAll('input[name="transformType"]');
        transformRadios.forEach(radio => {
            radio.checked = radio.value === (mapping.transformType || 'direct');
        });
        
        this.currentEditMapping = mapping;
        modal.classList.add('show');
        
        // Update preview
        this.updateTransformPreview();
    }

    populateHL7FieldOptions() {
        const select = document.getElementById('newHL7Field');
        if (!select) return;

        const parsedData = this.getParsedHL7Data();
        if (!parsedData || !parsedData.enhancedSegments) return;

        let options = '<option value="">Select HL7 Field...</option>';
        
        Object.entries(parsedData.enhancedSegments).forEach(([segmentName, segment]) => {
            if (segment.fields) {
                Object.entries(segment.fields).forEach(([fieldId, field]) => {
                    const fieldPath = `${segmentName}.${fieldId}`;
                    const value = field.value || field.name || '';
                    const description = field.description || 'HL7 Field';
                    
                    options += `<option value="${fieldPath}">${fieldPath} - "${this.truncateValue(value, 20)}" (${description})</option>`;
                });
            }
        });

        select.innerHTML = options;
        
        // Add change listener
        select.addEventListener('change', () => this.updateTransformPreview());
    }

    populateFHIRFieldOptions() {
        const select = document.getElementById('newFHIRField');
        if (!select) return;

        // Common FHIR fields organized by resource type
        const fhirFields = {
            'Patient': [
                { path: 'Patient.identifier[0].value', type: 'string', desc: 'Primary patient identifier' },
                { path: 'Patient.identifier[1].value', type: 'string', desc: 'Secondary patient identifier' },
                { path: 'Patient.name[0].family', type: 'string', desc: 'Family name' },
                { path: 'Patient.name[0].given[0]', type: 'string', desc: 'First given name' },
                { path: 'Patient.name[0].given[1]', type: 'string', desc: 'Middle name' },
                { path: 'Patient.telecom[0].value', type: 'string', desc: 'Phone number' },
                { path: 'Patient.telecom[1].value', type: 'string', desc: 'Email address' },
                { path: 'Patient.gender', type: 'code', desc: 'Administrative gender' },
                { path: 'Patient.birthDate', type: 'date', desc: 'Date of birth' },
                { path: 'Patient.address[0].line[0]', type: 'string', desc: 'Street address' },
                { path: 'Patient.address[0].city', type: 'string', desc: 'City' },
                { path: 'Patient.address[0].state', type: 'string', desc: 'State/Province' },
                { path: 'Patient.address[0].postalCode', type: 'string', desc: 'ZIP/Postal code' }
            ],
            'MessageHeader': [
                { path: 'MessageHeader.source.name', type: 'string', desc: 'Sending application' },
                { path: 'MessageHeader.source.endpoint', type: 'uri', desc: 'Sending facility endpoint' },
                { path: 'MessageHeader.destination[0].name', type: 'string', desc: 'Receiving application' },
                { path: 'MessageHeader.destination[0].endpoint', type: 'uri', desc: 'Receiving facility endpoint' },
                { path: 'MessageHeader.eventCoding.code', type: 'code', desc: 'Message event code' },
                { path: 'MessageHeader.eventCoding.display', type: 'string', desc: 'Event description' }
            ],
            'Encounter': [
                { path: 'Encounter.status', type: 'code', desc: 'Encounter status' },
                { path: 'Encounter.class.code', type: 'code', desc: 'Encounter class' },
                { path: 'Encounter.subject.reference', type: 'Reference', desc: 'Patient reference' },
                { path: 'Encounter.period.start', type: 'dateTime', desc: 'Encounter start time' },
                { path: 'Encounter.period.end', type: 'dateTime', desc: 'Encounter end time' }
            ]
        };

        let options = '<option value="">Select FHIR Field...</option>';
        
        Object.entries(fhirFields).forEach(([resourceType, fields]) => {
            options += `<optgroup label="${resourceType} Resource">`;
            fields.forEach(field => {
                options += `<option value="${field.path}">${field.path} (${field.type}) - ${field.desc}</option>`;
            });
            options += '</optgroup>';
        });

        select.innerHTML = options;
        
        // Add change listener
        select.addEventListener('change', () => this.updateTransformPreview());
    }

    updateTransformPreview() {
        const newHL7Field = document.getElementById('newHL7Field').value;
        const newFHIRField = document.getElementById('newFHIRField').value;
        const selectedTransform = document.querySelector('input[name="transformType"]:checked')?.value || 'direct';
        
        const beforeValue = document.getElementById('beforeValue');
        const afterValue = document.getElementById('afterValue');
        const hl7Preview = document.getElementById('hl7FieldPreview');
        const fhirPreview = document.getElementById('fhirFieldPreview');

        if (newHL7Field) {
            // Get HL7 field value
            const parsedData = this.getParsedHL7Data();
            const [segmentName, fieldId] = newHL7Field.split('.');
            const fieldValue = parsedData?.enhancedSegments?.[segmentName]?.fields?.[fieldId]?.value || '';
            
            hl7Preview.innerHTML = `
                <span class="preview-label">Current Value:</span>
                <span class="preview-value">"${this.truncateValue(fieldValue, 25)}"</span>
            `;
            
            if (beforeValue) beforeValue.textContent = `"${this.truncateValue(fieldValue, 20)}"`;
            
            // Simulate transformation preview
            let transformedValue = fieldValue;
            switch (selectedTransform) {
                case 'format':
                    transformedValue = fieldValue.replace(/\^/g, ' ').trim();
                    break;
                case 'split':
                    transformedValue = fieldValue.split('^')[0];
                    break;
                case 'custom':
                    transformedValue = `custom(${fieldValue})`;
                    break;
                default:
                    transformedValue = fieldValue;
            }
            
            if (afterValue) afterValue.textContent = `"${this.truncateValue(transformedValue, 20)}"`;
        }

        if (newFHIRField) {
            const fieldType = this.getFHIRFieldType(newFHIRField);
            fhirPreview.innerHTML = `
                <span class="preview-label">Data Type:</span>
                <span class="preview-value">${fieldType}</span>
            `;
        }
    }

    getFHIRFieldType(fhirPath) {
        const typeMap = {
            'identifier': 'Identifier',
            'name': 'HumanName',
            'telecom': 'ContactPoint',
            'address': 'Address',
            'gender': 'code',
            'birthDate': 'date',
            'status': 'code',
            'class': 'Coding',
            'period': 'Period',
            'reference': 'Reference',
            'endpoint': 'uri',
            'eventCoding': 'Coding'
        };

        for (const [key, type] of Object.entries(typeMap)) {
            if (fhirPath.includes(key)) return type;
        }
        return 'string';
    }

    closeEditModal() {
        const modal = document.getElementById('editMappingOverlay');
        if (modal) modal.classList.remove('show');
        this.currentEditMapping = null;
    }

    async saveMapping() {
        if (!this.currentEditMapping) return;

        const newHL7Field = document.getElementById('newHL7Field').value;
        const newFHIRField = document.getElementById('newFHIRField').value;
        const selectedTransform = document.querySelector('input[name="transformType"]:checked')?.value || 'direct';

        if (!newHL7Field || !newFHIRField) {
            this.showNotification('Please select both HL7 source and FHIR destination fields', 'warning');
            return;
        }

        // Save the mapping change
        this.editableMappings.set(this.currentEditMapping.fhirPath, {
            originalHL7Field: this.currentEditMapping.hl7Field,
            originalFHIRField: this.currentEditMapping.fhirPath,
            newHL7Field: newHL7Field,
            newFHIRField: newFHIRField,
            transformType: selectedTransform,
            timestamp: new Date().toISOString()
        });

        this.showNotification(`✅ Updated mapping: ${newHL7Field} → ${newFHIRField}`, 'success');
        this.closeEditModal();
        
        // Re-transform with new mappings (in a real implementation)
        console.log('🔄 Would re-transform with updated mappings:', this.editableMappings);
    }

    // JSON Viewer Methods
    showJsonViewer() {
        const modal = document.getElementById('jsonViewerOverlay');
        const jsonOutput = document.getElementById('jsonOutput');
        
        if (this.transformationData.bundle) {
            this.displayJsonTab('bundle');
            modal.classList.add('show');
        } else {
            this.showNotification('No FHIR data available to display', 'warning');
        }
    }

    displayJsonTab(tabType) {
        const jsonOutput = document.getElementById('jsonOutput');
        const tabs = document.querySelectorAll('.json-tab');
        
        // Update tab states
        tabs.forEach(tab => {
            tab.classList.toggle('active', tab.dataset.tab === tabType);
        });

        let content = '';
        switch (tabType) {
            case 'bundle':
                content = JSON.stringify(this.transformationData.bundle, null, 2);
                break;
            case 'resources':
                content = JSON.stringify(this.transformationData.fhirResources, null, 2);
                break;
            case 'validation':
                content = JSON.stringify({
                    success: this.transformationData.success,
                    warnings: this.getFilteredValidationWarnings(),
                    errors: this.getFilteredValidationErrors(),
                    mappingStats: this.transformationData.mappingStats,
                    performance: this.transformationData.performance
                }, null, 2);
                break;
        }
        
        if (jsonOutput) {
            jsonOutput.textContent = content;
        }
    }

    closeJsonViewer() {
        const modal = document.getElementById('jsonViewerOverlay');
        if (modal) modal.classList.remove('show');
    }

    copyJson() {
        const jsonOutput = document.getElementById('jsonOutput');
        if (jsonOutput) {
            navigator.clipboard.writeText(jsonOutput.textContent).then(() => {
                this.showNotification('✅ JSON copied to clipboard', 'success');
            }).catch(() => {
                this.showNotification('❌ Failed to copy JSON', 'error');
            });
        }
    }

    downloadJson() {
        const jsonOutput = document.getElementById('jsonOutput');
        if (!jsonOutput) return;

        const blob = new Blob([jsonOutput.textContent], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        
        const messageType = this.transformationData.messageType?.replace('^', '_') || 'unknown';
        const timestamp = new Date().toISOString().split('T')[0];
        
        a.href = url;
        a.download = `fhir-transformation-${messageType}-${timestamp}.json`;
        a.click();
        
        URL.revokeObjectURL(url);
        this.showNotification('✅ JSON file downloaded', 'success');
    }

    // Validation Sidebar Methods
    showValidationSidebar() {
        const sidebar = document.getElementById('validationSidebar');
        const content = document.getElementById('validationContent');
        
        if (!sidebar || !content) return;

        const filteredWarnings = this.getFilteredValidationWarnings();
        const filteredErrors = this.getFilteredValidationErrors();
        const successfulMappings = this.transformationData.mappingStats?.totalFieldsMapped || 0;

        // Update summary metrics
        document.getElementById('validationSuccess').textContent = Math.max(0, successfulMappings - filteredErrors.length);
        document.getElementById('validationWarnings').textContent = filteredWarnings.length;
        document.getElementById('validationErrors').textContent = filteredErrors.length;

        let html = '';

        // Add successful validations
        if (successfulMappings > 0) {
            html += `
                <div class="validation-item success">
                    <div class="validation-item-header">
                        <span>✅</span>
                        <span>Successful Field Mappings</span>
                    </div>
                    <div>${successfulMappings} fields were successfully mapped to FHIR resources</div>
                </div>
            `;
        }

        // Add filtered warnings
        filteredWarnings.forEach(warning => {
            html += `
                <div class="validation-item warning">
                    <div class="validation-item-header">
                        <span>⚠️</span>
                        <span>Mapping Warning</span>
                    </div>
                    <div>${warning}</div>
                </div>
            `;
        });

        // Add filtered errors
        filteredErrors.forEach(error => {
            html += `
                <div class="validation-item error">
                    <div class="validation-item-header">
                        <span>❌</span>
                        <span>Validation Error</span>
                    </div>
                    <div>${error}</div>
                </div>
            `;
        });

        if (!html) {
            html = `
                <div class="validation-item success">
                    <div class="validation-item-header">
                        <span>🎉</span>
                        <span>Perfect Validation</span>
                    </div>
                    <div>All mapped fields passed FHIR validation successfully!</div>
                </div>
            `;
        }

        content.innerHTML = html;
        sidebar.classList.add('show');
    }

    closeValidationSidebar() {
        const sidebar = document.getElementById('validationSidebar');
        if (sidebar) sidebar.classList.remove('show');
    }

    // Configuration Save Methods
    showConfigSaveModal() {
        const modal = document.getElementById('configSaveOverlay');
        
        // Populate form with current data
        const messageType = this.transformationData.messageType || 'Unknown';
        document.getElementById('configMessageType').value = messageType;
        document.getElementById('configName').value = `${messageType} Standard Mapping`;
        
        // Update summary stats
        const mappingCount = this.transformationData.mappingStats?.totalFieldsMapped || 0;
        const resourceCount = this.transformationData.fhirResources?.length || 0;
        const filteredErrors = this.getFilteredValidationErrors();
        const validationScore = Math.max(0, Math.round(((mappingCount - filteredErrors.length) / mappingCount) * 100)) || 100;
        
        document.getElementById('configMappingCount').textContent = mappingCount;
        document.getElementById('configResourceCount').textContent = resourceCount;
        document.getElementById('configValidationScore').textContent = `${validationScore}%`;
        
        modal.classList.add('show');
    }

    closeConfigSaveModal() {
        const modal = document.getElementById('configSaveOverlay');
        if (modal) modal.classList.remove('show');
    }

    async saveConfiguration() {
        const configName = document.getElementById('configName').value;
        const configDescription = document.getElementById('configDescription').value;
        const setAsDefault = document.getElementById('setAsDefault').checked;

        if (!configName.trim()) {
            this.showNotification('Please enter a configuration name', 'warning');
            return;
        }

        try {
            // Prepare configuration data
            const configData = {
                name: configName,
                description: configDescription,
                messageType: this.transformationData.messageType,
                mappings: Array.from(this.editableMappings.entries()),
                originalMappings: this.extractAllMappings(),
                isDefault: setAsDefault,
                validationScore: this.calculateValidationScore(),
                createdAt: new Date().toISOString(),
                createdBy: 'current_user' // Would come from authentication
            };

            // In a real implementation, this would save to the database
            console.log('💾 Saving mapping configuration:', configData);
            
            // Simulate API call
            await this.delay(1000);
            
            this.showNotification('✅ Mapping configuration saved successfully!', 'success');
            this.closeConfigSaveModal();

        } catch (error) {
            console.error('❌ Failed to save configuration:', error);
            this.showNotification('❌ Failed to save configuration', 'error');
        }
    }

    extractAllMappings() {
        const allMappings = [];
        
        this.transformationData.fhirResources?.forEach(resource => {
            const mappings = this.extractDetailedMappings(resource);
            allMappings.push(...mappings);
        });
        
        return allMappings;
    }

    calculateValidationScore() {
        const filteredErrors = this.getFilteredValidationErrors();
        const totalMappings = this.transformationData.mappingStats?.totalFieldsMapped || 0;
        
        if (totalMappings === 0) return 100;
        return Math.max(0, Math.round(((totalMappings - filteredErrors.length) / totalMappings) * 100));
    }

    // Utility Methods
    showPlaceholder() {
        const placeholder = document.getElementById('transformPlaceholder');
        const summary = document.querySelector('.transformation-summary');
        const resources = document.querySelector('.resources-section');
        
        if (placeholder) placeholder.style.display = 'block';
        if (summary) summary.style.display = 'none';
        if (resources) resources.style.display = 'none';
    }

    hidePlaceholder() {
        const placeholder = document.getElementById('transformPlaceholder');
        const summary = document.querySelector('.transformation-summary');
        const resources = document.querySelector('.resources-section');
        
        if (placeholder) placeholder.style.display = 'none';
        if (summary) summary.style.display = 'block';
        if (resources) resources.style.display = 'block';
    }

    getParsedHL7Data() {
        if (this.wizard.parsedHL7Data?.data) {
            return this.wizard.parsedHL7Data.data;
        }
        
        if (window.parsedHL7Data?.data) {
            console.log('🔄 Using fallback parsedHL7Data from window');
            this.wizard.parsedHL7Data = window.parsedHL7Data;
            return window.parsedHL7Data.data;
        }
        
        return null;
    }

    showError(message) {
        const container = document.getElementById('fhirResourcesContainer');
        if (container) {
            container.innerHTML = `
                <div class="error-display fade-in" style="
                    text-align: center;
                    padding: 60px 40px;
                    background: linear-gradient(135deg, #fef2f2 0%, #ffffff 100%);
                    border: 2px solid #fecaca;
                    border-radius: 16px;
                    color: #dc2626;
                ">
                    <div style="font-size: 64px; margin-bottom: 24px;">❌</div>
                    <h4 style="margin: 0 0 16px 0; font-size: 20px; font-weight: 600;">Transformation Error</h4>
                    <p style="margin: 0 0 24px 0; font-size: 14px;">${message}</p>
                    <button onclick="window.fhirHandler.onStepActivated()" class="wizard-btn primary" style="
                        padding: 12px 24px;
                        background: linear-gradient(135deg, #dc2626 0%, #ef4444 100%);
                        color: white;
                        border: none;
                        border-radius: 8px;
                        font-weight: 600;
                        cursor: pointer;
                        transition: all 0.2s ease;
                    ">
                        🔄 Retry Transformation
                    </button>
                </div>
            `;
        }
    }

    showNotification(message, type = 'info') {
        const toast = document.createElement('div');
        toast.className = 'notification-toast fade-in';
        
        const colors = {
            info: '#3b82f6',
            success: '#10b981',
            warning: '#f59e0b',
            error: '#ef4444'
        };
        
        toast.style.cssText = `
            position: fixed;
            top: 24px;
            right: 24px;
            padding: 16px 20px;
            border-radius: 12px;
            color: white;
            font-size: 14px;
            font-weight: 500;
            z-index: 10001;
            max-width: 400px;
            background: ${colors[type]};
            box-shadow: 0 8px 24px rgba(0,0,0,0.15);
            border: 2px solid rgba(255,255,255,0.2);
            backdrop-filter: blur(8px);
            transform: translateX(100%);
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
        `;
        
        toast.textContent = message;
        document.body.appendChild(toast);
        
        // Animate in
        requestAnimationFrame(() => {
            toast.style.transform = 'translateX(0)';
        });
        
        // Animate out and remove
        setTimeout(() => {
            toast.style.transform = 'translateX(100%)';
            setTimeout(() => toast.remove(), 300);
        }, type === 'error' ? 5000 : 3000);
    }

    delay(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    // Step Handler Interface Methods
    validate() {
        if (!this.transformationData) {
            this.showNotification('FHIR transformation is required', 'error');
            return false;
        }
        
        const filteredErrors = this.getFilteredValidationErrors();
        if (filteredErrors.length > 0) {
            this.showNotification(`Please resolve ${filteredErrors.length} validation errors before proceeding`, 'warning');
            this.showValidationSidebar();
            return false;
        }
        
        console.log('✅ Step 4 Enhanced FHIR transformation validation passed');
        return true;
    }

    reset() {
        this.transformationData = null;
        this.mappingConfiguration.clear();
        this.editableMappings.clear();
        this.expandedResources.clear();
        this.initialize();
    }

    getStepData() {
        return {
            transformationData: this.transformationData,
            originalMappings: this.extractAllMappings(),
            editableMappings: Object.fromEntries(this.editableMappings),
            messageType: this.transformationData?.messageType || 'Unknown',
            resourceCount: this.transformationData?.fhirResources?.length || 0,
            mappingStats: this.transformationData?.mappingStats || {},
            validationResults: {
                warnings: this.getFilteredValidationWarnings(),
                errors: this.getFilteredValidationErrors(),
                score: this.calculateValidationScore()
            },
            performance: this.transformationData?.performance || {},
            timestamp: new Date().toISOString()
        };
    }
}

// Global Event Handlers
document.addEventListener('DOMContentLoaded', () => {
    // JSON tab switching
    document.addEventListener('click', (e) => {
        if (e.target.classList.contains('json-tab')) {
            const tabType = e.target.dataset.tab;
            if (window.fhirHandler) {
                window.fhirHandler.displayJsonTab(tabType);
            }
        }
    });
});

// Global functions and initialization
window.fhirHandler = null;

// Define BaseStepHandler if it doesn't exist
if (typeof BaseStepHandler === 'undefined') {
    class BaseStepHandler {
        constructor(wizard) {
            this.wizard = wizard;
        }
        
        showNotification(message, type = 'info') {
            console.log(`[${type.toUpperCase()}] ${message}`);
            if (this.wizard && this.wizard.showNotification) {
                this.wizard.showNotification(message, type);
            }
        }
    }
    window.BaseStepHandler = BaseStepHandler;
}

// Make the class globally accessible and create the global handler
window.FHIRMappingStepHandler = FHIRMappingStepHandler;

// Set up global handler when the class is instantiated
const originalConstructor = FHIRMappingStepHandler;
window.FHIRMappingStepHandler = function(wizard) {
    const instance = new originalConstructor(wizard);
    window.fhirHandler = instance;
    return instance;
};

// Copy static properties
Object.setPrototypeOf(window.FHIRMappingStepHandler, originalConstructor);
window.FHIRMappingStepHandler.prototype = originalConstructor.prototype;

console.log('✅ FHIRMappingStepHandler (Enhanced Results Version) loaded and available globally');