// js/components/wizard-component.js - Wizard Modal Component Loader
// Loads the wizard modal HTML structure dynamically

(function() {
    'use strict';
    
    // ✅ Wait for DOM to be ready
    function loadWizardComponent() {
        const container = document.getElementById('wizard-modal-container');
        if (!container) {
            console.warn('⚠️ Wizard modal container not found');
            return;
        }
        
        // ✅ PRESERVED: Complete wizard modal HTML with all original IDs
        container.innerHTML = `
            <!-- NEW: Wizard Modal with Left Sidebar Layout -->
            <div class="wizard-modal-overlay" id="wizardModalOverlay">
                <div class="wizard-modal-container" id="wizardModalContainer">
                    
                    <!-- Modal Control Buttons -->
                    <div class="wizard-modal-controls">
                        <button class="wizard-modal-control maximize-btn" id="wizardModalMaximize" title="Maximize">
                            <span id="maximizeIcon">⛶</span>
                        </button>
                        <button class="wizard-modal-control close-btn" id="wizardModalClose" title="Close">×</button>
                    </div>

                    <!-- NEW: Left Sidebar -->
                    <div class="wizard-sidebar">
                        <div class="sidebar-header">
                            <h2 class="sidebar-title">🧙‍♂️ HL7 Interface</h2>
                            <p class="sidebar-subtitle">AI-powered interface builder for seamless healthcare data integration</p>
                        </div>

                        <nav class="step-navigation">
                            <ul class="step-list">
                                <!-- Step 1: Configure -->
                                <li class="step-item active" id="stepIndicator1">
                                    <a href="#" class="step-link">
                                        <div class="step-number" id="stepCircle1">1</div>
                                        <div class="step-content">
                                            <div class="step-title">Configure</div>
                                            <div class="step-description">Set up basic configuration</div>
                                        </div>
                                    </a>
                                </li>

                                <!-- PRESERVED: Step connectors with original IDs -->
                                <div class="step-connector" id="connector1"></div>

                                <!-- Step 2: Upload -->
                                <li class="step-item" id="stepIndicator2">
                                    <a href="#" class="step-link">
                                        <div class="step-number" id="stepCircle2">2</div>
                                        <div class="step-content">
                                            <div class="step-title">Upload</div>
                                            <div class="step-description">Upload HL7 message files</div>
                                        </div>
                                    </a>
                                </li>

                                <div class="step-connector" id="connector2"></div>

                                <!-- Step 3: Review -->
                                <li class="step-item" id="stepIndicator3">
                                    <a href="#" class="step-link">
                                        <div class="step-number" id="stepCircle3">3</div>
                                        <div class="step-content">
                                            <div class="step-title">Review</div>
                                            <div class="step-description">Review parsed content</div>
                                        </div>
                                    </a>
                                </li>

                                <div class="step-connector" id="connector3"></div>

                                <!-- Step 4: Mapping -->
                                <li class="step-item" id="stepIndicator4">
                                    <a href="#" class="step-link">
                                        <div class="step-number" id="stepCircle4">4</div>
                                        <div class="step-content">
                                            <div class="step-title">Mapping</div>
                                            <div class="step-description">Configure data mapping</div>
                                        </div>
                                    </a>
                                </li>

                                <div class="step-connector" id="connector4"></div>

                                <!-- Step 5: Deploy -->
                                <li class="step-item" id="stepIndicator5">
                                    <a href="#" class="step-link">
                                        <div class="step-number" id="stepCircle5">5</div>
                                        <div class="step-content">
                                            <div class="step-title">Deploy</div>
                                            <div class="step-description">Deploy and finalize</div>
                                        </div>
                                    </a>
                                </li>
                            </ul>
                        </nav>
                    </div>

                    <!-- NEW: Main Content Area -->
                    <div class="wizard-main">
                        <!-- Header -->
                        <div class="wizard-header">
                            <h2 class="wizard-title">Interface Configuration</h2>
                            <p class="wizard-subtitle">Set up the basic configuration for your HL7 interface</p>
                        </div>

                        <!-- Content -->
                        <div class="wizard-content">
                            <!-- Step 1: Configure -->
                            <div class="wizard-step active" id="step1">
                                <div class="step-header">
                                    <div class="step-progress">STEP 1 OF 5</div>
                                    <h3 class="step-title">Interface Configuration</h3>
                                    <p class="step-description">Set up the basic configuration for your HL7 interface</p>
                                </div>
                                
                                <div class="form-grid">
                                    <div class="form-row">
                                        <div class="form-group">
                                            <label for="wizardInterfaceName" class="form-label required">Interface Name</label>
                                            <input type="text" id="wizardInterfaceName" class="form-input" 
                                                   placeholder="e.g., ADT Patient Admissions" required>
                                        </div>
                                    </div>
                                    
                                    <div class="form-row">
                                        <div class="form-group">
                                            <label for="wizardInterfaceDescription" class="form-label">Description</label>
                                            <textarea id="wizardInterfaceDescription" class="form-textarea" 
                                                      placeholder="Brief description of this interface and its purpose"></textarea>
                                        </div>
                                    </div>
                                    
                                    <div class="form-row">
                                        <div class="form-group">
                                            <label for="wizardSourceType" class="form-label required">Source Type</label>
                                            <select id="wizardSourceType" class="form-select" required>
                                                <option value="">Select source...</option>
                                                <option value="hl7v2">HL7 v2.x</option>
                                                <option value="hl7v3">HL7 v3</option>
                                                <option value="fhir">FHIR</option>
                                                <option value="ccda">C-CDA</option>
                                                <option value="custom">Custom Format</option>
                                            </select>
                                        </div>
                                        
                                        <div class="form-group">
                                            <label for="wizardTargetType" class="form-label required">Target Type</label>
                                            <select id="wizardTargetType" class="form-select" required>
                                                <option value="">Select target...</option>
                                                <option value="fhir">FHIR R4</option>
                                                <option value="hl7v2">HL7 v2.x</option>
                                                <option value="ccda">C-CDA</option>
                                                <option value="custom">Custom Format</option>
                                            </select>
                                        </div>
                                    </div>
                                    
                                    <div class="form-row">
                                        <div class="form-group">
                                            <label for="wizardMessageType" class="form-label">Message Type</label>
                                            <select id="wizardMessageType" class="form-select">
                                                <option value="">Auto-detect from upload</option>
                                                <option value="ADT">ADT - Admission, Discharge, Transfer</option>
                                                <option value="ORU">ORU - Observation Result</option>
                                                <option value="ORM">ORM - Order Message</option>
                                                <option value="MDM">MDM - Medical Document Management</option>
                                                <option value="SIU">SIU - Scheduling Information</option>
                                            </select>
                                        </div>
                                    </div>
                                    
                                    <!-- PRESERVED: Configuration containers with original IDs -->
                                    <div id="wizardSourceConfig" style="display: none;">
                                        <!-- Dynamic content populated by JavaScript -->
                                    </div>

                                    <div id="wizardTargetConfig" style="display: none;">
                                        <!-- Dynamic content populated by JavaScript -->
                                    </div>
                                </div>
                            </div>

                            <!-- PRESERVED: Step 2 with correct element IDs -->
                            <div class="wizard-step" id="step2">
                                <div class="step-header">
                                    <div class="step-progress">STEP 2 OF 5</div>
                                    <h3 class="step-title">Upload Sample HL7 Message</h3>
                                    <p class="step-description">Upload a sample HL7 message to analyze structure</p>
                                </div>
                                
                                <div id="uploadZone" class="upload-zone">
                                    <div class="upload-content">
                                        <div class="upload-icon">📄</div>
                                        <div class="upload-title">Drop HL7 file here or click to browse</div>
                                        <div class="upload-subtitle">Supports .hl7, .txt, and .msg files</div>
                                        <!-- PRESERVED: Correct file input ID -->
                                        <input type="file" id="hl7FileUpload" accept=".hl7,.txt,.msg" style="display: none;">
                                    </div>
                                </div>
                                
                                <div id="fileInfo" class="file-info"></div>
                                
                                <div class="upload-actions">
                                    <button id="parseBtn" class="wizard-btn primary" style="display: none;">
                                        Parse HL7 Message
                                    </button>
                                    <!-- PRESERVED: Skip button ID -->
                                    <button id="skipUploadBtn" class="wizard-btn secondary">
                                        Skip for Now
                                    </button>
                                </div>
                                
                                <div id="parseResults" class="parse-results"></div>
                            </div>

                            <!-- PRESERVED: Step 3 with correct element IDs -->
                            <div class="wizard-step" id="step3">
                                <div class="step-header">
                                    <div class="step-progress">STEP 3 OF 5</div>
                                    <h3 class="step-title">Review Parsed Data</h3>
                                    <p class="step-description">Verify the HL7 message structure and segments</p>
                                </div>
                                
                                <!-- PRESERVED: parsedDataReview ID -->
                                <div id="parsedDataReview" class="parsed-data-review">
                                    <div class="review-placeholder">
                                        <div style="text-align: center; padding: 40px; color: #6b7280;">
                                            <div style="font-size: 48px; margin-bottom: 16px;">📋</div>
                                            <div style="font-weight: 600; margin-bottom: 8px;">No data to review yet</div>
                                            <div>Upload and parse an HL7 message to see the structure here</div>
                                        </div>
                                    </div>
                                </div>
                            </div>

                            <!-- Step 4: Mapping (placeholder - will be loaded by FHIR component) -->
                            <div class="wizard-step" id="step4">
                                <div class="step-header">
                                    <div class="step-progress">STEP 4 OF 5</div>
                                    <h3 class="step-title">Configure Data Mapping</h3>
                                    <p class="step-description">Configure how HL7 fields map to target format</p>
                                </div>
                                
                                <div class="mapping-placeholder">
                                    <div style="text-align: center; padding: 40px; color: #6b7280;">
                                        <div style="font-size: 48px; margin-bottom: 16px;">🗺️</div>
                                        <div style="font-weight: 600; margin-bottom: 8px;">Mapping Configuration</div>
                                        <div>FHIR mapping interface will load here</div>
                                    </div>
                                </div>
                            </div>

                            <!-- Step 5: Deploy -->
                            <div class="wizard-step" id="step5">
                                <div class="step-header">
                                    <div class="step-progress">STEP 5 OF 5</div>
                                    <h3 class="step-title">Deploy Interface</h3>
                                    <p class="step-description">Review and deploy your new HL7 interface</p>
                                </div>
                                
                                <div class="summary-grid">
                                    <div class="summary-card">
                                        <div class="stat-number" id="summarySegments">0</div>
                                        <div class="stat-label">Segments Processed</div>
                                    </div>
                                    <div class="summary-card">
                                        <div class="stat-number" id="summaryFields">0</div>
                                        <div class="stat-label">Fields Mapped</div>
                                    </div>
                                    <div class="summary-card">
                                        <div class="stat-number" id="summaryValidations">0</div>
                                        <div class="stat-label">Validations Passed</div>
                                    </div>
                                </div>
                                
                                <div class="alert alert-success">
                                    <h5>✅ Ready for Deployment</h5>
                                    <p>Your HL7 interface configuration is complete and ready for deployment. 
                                    Click "Create Interface" to make it live and start processing messages.</p>
                                </div>
                            </div>
                        </div>

                        <!-- Navigation Footer -->
                        <div class="wizard-navigation">
                            <div class="nav-info">
                                <div class="nav-step" id="navStep">Step 1 of 5 • Configure</div>
                                <div class="nav-title" id="navTitle">Interface Configuration</div>
                            </div>
                            <div class="nav-buttons">
                                <button id="prevBtn" class="wizard-btn secondary" style="display: none;">
                                    <span>←</span>
                                    Previous
                                </button>
                                <button id="nextBtn" class="wizard-btn primary">
                                    Next
                                    <span>→</span>
                                </button>
                                <button id="finishBtn" class="wizard-btn primary" style="display: none;">
                                    <span>✓</span>
                                    Create Interface
                                </button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        `;
        
        console.log('✅ Wizard modal component loaded');
        
        // ✅ Setup modal event listeners after HTML is loaded
        setupWizardModalEvents();
    }
    
    // ✅ Setup wizard modal specific events
    function setupWizardModalEvents() {
        // Close button event
        const closeBtn = document.getElementById('wizardModalClose');
        if (closeBtn) {
            closeBtn.addEventListener('click', () => {
                if (typeof window.closeWizard === 'function') {
                    window.closeWizard();
                }
            });
        }
        
        // Maximize button event
        const maximizeBtn = document.getElementById('wizardModalMaximize');
        if (maximizeBtn) {
            maximizeBtn.addEventListener('click', () => {
                const container = document.getElementById('wizardModalContainer');
                const icon = document.getElementById('maximizeIcon');
                
                if (container && icon) {
                    if (container.classList.contains('maximized')) {
                        container.classList.remove('maximized');
                        icon.textContent = '⛶';
                    } else {
                        container.classList.add('maximized');
                        icon.textContent = '⛷';
                    }
                }
            });
        }
        
        // ESC key to close
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') {
                const overlay = document.getElementById('wizardModalOverlay');
                if (overlay && overlay.classList.contains('show')) {
                    if (typeof window.closeWizard === 'function') {
                        window.closeWizard();
                    }
                }
            }
        });
        
        console.log('✅ Wizard modal events setup complete');
    }
    
    // ✅ Load component when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', loadWizardComponent);
    } else {
        loadWizardComponent();
    }
    
})();