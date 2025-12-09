// js/components/wizard-component.js - Wizard Modal Component Loader
// Loads the wizard modal HTML structure dynamically

(function() {
    'use strict';
    
    // Wait for DOM to be ready
    function loadWizardComponent() {
        const container = document.getElementById('wizard-modal-container');
        if (!container) {
            console.warn('Wizard modal container not found');
            return;
        }
        
        // Complete wizard modal HTML with connectivity fields
        container.innerHTML = `
            <!-- Wizard Modal with Left Sidebar Layout -->
            <div class="wizard-modal-overlay" id="wizardModalOverlay">
                <div class="wizard-modal-container" id="wizardModalContainer">
                    
                    <!-- Modal Control Buttons -->
                    <div class="wizard-modal-controls">
                        <button class="wizard-modal-control maximize-btn" id="wizardModalMaximize" title="Maximize">
                            <span id="maximizeIcon">⛶</span>
                        </button>
                        <button class="wizard-modal-control close-btn" id="wizardModalClose" title="Close">×</button>
                    </div>

                    <!-- Left Sidebar -->
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

                    <!-- Main Content Area -->
                    <div class="wizard-main">
                        <!-- Header -->
                        <div class="wizard-header">
                            <h2 class="wizard-title">Interface Configuration</h2>
                            <p class="wizard-subtitle">Configure message formats and connectivity for your interface</p>
                        </div>

                        <!-- Content -->
                        <div class="wizard-content">
                            <!-- Step 1: Configure - UPDATED WITH CONNECTIVITY FIELDS -->
                            <div class="wizard-step active" id="step1">
                                <div class="step-header">
                                    <div class="step-progress">STEP 1 OF 5</div>
                                    <h3 class="step-title">Interface Configuration</h3>
                                    <p class="step-description">Configure message formats and connectivity for your interface</p>
                                </div>

                                <div class="form-grid">
                                    <!-- Basic Information -->
                                    <div class="form-section">
                                        <div class="form-section-title">
                                            📋 Basic Information
                                        </div>
                                        
                                        <div class="form-group">
                                            <label class="form-label" for="wizardInterfaceName">Interface Name *</label>
                                            <input type="text" id="wizardInterfaceName" class="form-input" placeholder="e.g., ADT Patient Admissions" required>
                                            <div class="field-hint">A descriptive name for this interface</div>
                                        </div>

                                        <div class="form-group">
                                            <label class="form-label" for="wizardInterfaceDescription">Description</label>
                                            <textarea id="wizardInterfaceDescription" class="form-textarea" 
                                                      placeholder="Brief description of this interface and its purpose"></textarea>
                                            <div class="field-hint">Optional description for documentation purposes</div>
                                        </div>

                                        <div class="form-group">
                                            <label for="wizardMessageType" class="form-label">Message Type</label>
                                            <select id="wizardMessageType" class="form-select">
                                                <option value="auto-detect">Auto-detect from file</option>
                                                <option value="ADT^A01">ADT^A01 - Admit/Visit Notification</option>
                                                <option value="ADT^A04">ADT^A04 - Register Patient</option>
                                                <option value="ADT^A08">ADT^A08 - Update Patient Info</option>
                                                <option value="ORU^R01">ORU^R01 - Observation Result</option>
                                                <option value="ORM^O01">ORM^O01 - Order Message</option>
                                                <option value="SIU^S12">SIU^S12 - Appointment Booking</option>
                                            </select>
                                            <div class="field-hint">Select the HL7 message type or auto-detect from uploaded file</div>
                                        </div>
                                    </div>

                                    <!-- Source Configuration -->
                                    <div class="form-section">
                                        <div class="form-section-title">
                                            📥 Source Configuration (Where messages come from)
                                        </div>
                                        
                                        <div class="form-row">
                                            <div class="form-group">
                                                <label class="form-label" for="wizardSourceType">Source Format *</label>
                                                <select id="wizardSourceType" class="form-select" required>
                                                    <option value="">Select format...</option>
                                                    <option value="hl7v2">HL7 v2.x</option>
                                                    <option value="hl7v3">HL7 v3.x</option>
                                                    <option value="fhir">FHIR</option>
                                                    <option value="ccda">C-CDA</option>
                                                    <option value="file">File Format</option>
                                                    <option value="database">Database</option>
                                                </select>
                                                <div class="field-hint">The message format you're receiving</div>
                                            </div>

                                            <div class="form-group">
                                                <label class="form-label" for="wizardSourceConnectivity">Source Connectivity *</label>
                                                <select id="wizardSourceConnectivity" class="form-select" required>
                                                    <option value="">Select method...</option>
                                                    <option value="tcp">TCP/IP Listener</option>
                                                    <option value="http">HTTP Endpoint</option>
                                                    <option value="file">File Polling</option>
                                                    <option value="database">Database Polling</option>
                                                    <option value="sftp">SFTP</option>
                                                    <option value="mq">Message Queue</option>
                                                </select>
                                                <div class="field-hint">How you'll receive the messages</div>
                                            </div>
                                        </div>

                                        <!-- Source Connectivity Configuration -->
                                        <div id="wizardSourceConnectivityConfig" class="connectivity-config" style="display: none;">
                                            <!-- TCP Configuration -->
                                            <div class="tcp-config config-section" data-connectivity="tcp">
                                                <div class="form-row">
                                                    <div class="form-group">
                                                        <label class="form-label">Host</label>
                                                        <input type="text" class="form-input" id="sourceHost" placeholder="localhost" value="localhost">
                                                        <div class="field-hint">Server hostname or IP address</div>
                                                    </div>
                                                    <div class="form-group">
                                                        <label class="form-label">Port</label>
                                                        <input type="number" class="form-input" id="sourcePort" placeholder="2575" value="2575">
                                                        <div class="field-hint">TCP port number</div>
                                                    </div>
                                                </div>
                                            </div>
                                            
                                            <!-- HTTP Configuration -->
                                            <div class="http-config config-section" data-connectivity="http" style="display: none;">
                                                <div class="form-group">
                                                    <label class="form-label">Endpoint URL</label>
                                                    <input type="url" class="form-input" id="sourceEndpoint" placeholder="https://api.example.com/hl7">
                                                    <div class="field-hint">HTTP endpoint for receiving HL7 messages</div>
                                                </div>
                                            </div>
                                            
                                            <!-- File Configuration -->
                                            <div class="file-config config-section" data-connectivity="file" style="display: none;">
                                                <div class="form-group">
                                                    <label class="form-label">File Path</label>
                                                    <input type="text" class="form-input" id="sourceFilePath" placeholder="/path/to/input/directory">
                                                    <div class="field-hint">Directory to monitor for incoming files</div>
                                                </div>
                                            </div>
                                            
                                            <!-- Database Configuration -->
                                            <div class="database-config config-section" data-connectivity="database" style="display: none;">
                                                <div class="form-group">
                                                    <label class="form-label">Connection String</label>
                                                    <input type="text" class="form-input" id="sourceConnectionString" placeholder="postgresql://user:pass@host:port/db">
                                                    <div class="field-hint">Database connection string</div>
                                                </div>
                                            </div>
                                        </div>
                                    </div>

                                    <!-- Target Configuration -->
                                    <div class="form-section">
                                        <div class="form-section-title">
                                            📤 Target Configuration (Where messages go)
                                        </div>
                                        
                                        <div class="form-row">
                                            <div class="form-group">
                                                <label class="form-label" for="wizardTargetType">Target Format *</label>
                                                <select id="wizardTargetType" class="form-select" required>
                                                    <option value="">Select format...</option>
                                                    <option value="fhir">FHIR R4</option>
                                                    <option value="hl7v2">HL7 v2.x</option>
                                                    <option value="ccda">C-CDA</option>
                                                    <option value="database">Database</option>
                                                    <option value="flatfile">Flat File</option>
                                                    <option value="json">JSON</option>
                                                    <option value="xml">XML</option>
                                                </select>
                                                <div class="field-hint">The format you want to convert to</div>
                                            </div>

                                            <div class="form-group">
                                                <label class="form-label" for="wizardTargetConnectivity">Target Connectivity *</label>
                                                <select id="wizardTargetConnectivity" class="form-select" required>
                                                    <option value="">Select method...</option>
                                                    <option value="http">HTTP/REST API</option>
                                                    <option value="database">Database Insert</option>
                                                    <option value="file">File Output</option>
                                                    <option value="tcp">TCP/IP Send</option>
                                                    <option value="sftp">SFTP Upload</option>
                                                    <option value="mq">Message Queue</option>
                                                </select>
                                                <div class="field-hint">How you'll send the messages</div>
                                            </div>
                                        </div>

                                        <!-- Target Connectivity Configuration -->
                                        <div id="wizardTargetConnectivityConfig" class="connectivity-config" style="display: none;">
                                            <!-- HTTP Configuration -->
                                            <div class="http-config config-section" data-connectivity="http">
                                                <div class="form-group">
                                                    <label class="form-label">FHIR Server URL</label>
                                                    <input type="url" class="form-input" id="fhirserverurl" placeholder="https://hapi.fhir.org/baseR4" value="https://hapi.fhir.org/baseR4">
                                                    <div class="field-hint">FHIR server endpoint URL</div>
                                                </div>
                                            </div>
                                            
                                            <!-- TCP Configuration -->
                                            <div class="tcp-config config-section" data-connectivity="tcp" style="display: none;">
                                                <div class="form-row">
                                                    <div class="form-group">
                                                        <label class="form-label">Target Host</label>
                                                        <input type="text" class="form-input" id="targetHost" placeholder="destination.server.com">
                                                        <div class="field-hint">Destination server hostname</div>
                                                    </div>
                                                    <div class="form-group">
                                                        <label class="form-label">Target Port</label>
                                                        <input type="number" class="form-input" id="targetPort" placeholder="2575">
                                                        <div class="field-hint">Destination port number</div>
                                                    </div>
                                                </div>
                                            </div>
                                            
                                            <!-- File Configuration -->
                                            <div class="file-config config-section" data-connectivity="file" style="display: none;">
                                                <div class="form-group">
                                                    <label class="form-label">Output Path</label>
                                                    <input type="text" class="form-input" id="targetFilePath" placeholder="/path/to/output/directory">
                                                    <div class="field-hint">Directory for output files</div>
                                                </div>
                                            </div>
                                            
                                            <!-- Database Configuration -->
                                            <div class="database-config config-section" data-connectivity="database" style="display: none;">
                                                <div class="form-group">
                                                    <label class="form-label">Connection String</label>
                                                    <input type="text" class="form-input" id="targetConnectionString" placeholder="postgresql://user:pass@host:port/db">
                                                    <div class="field-hint">Target database connection string</div>
                                                </div>
                                            </div>
                                        </div>
                                    </div>
                                    
                                    <!-- Configuration containers for dynamic content -->
                                    <div id="wizardSourceConfig" style="display: none;">
                                        <!-- Dynamic source configuration populated by JavaScript -->
                                    </div>

                                    <div id="wizardTargetConfig" style="display: none;">
                                        <!-- Dynamic target configuration populated by JavaScript -->
                                    </div>
                                </div>
                            </div>

                            <!-- Step 2: Upload -->
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
                                        <input type="file" id="hl7FileUpload" accept=".hl7,.txt,.msg" style="display: none;">
                                    </div>
                                </div>
                                
                                <div id="fileInfo" class="file-info"></div>
                                
                                <div class="upload-actions">
                                    <button id="parseBtn" class="wizard-btn primary" style="display: none;">
                                        Parse HL7 Message
                                    </button>
                                    <button id="skipUploadBtn" class="wizard-btn secondary">
                                        Skip for Now
                                    </button>
                                </div>
                                
                                <div id="parseResults" class="parse-results"></div>
                            </div>

                            <!-- Step 3: Review -->
                            <div class="wizard-step" id="step3">
                                <div class="step-header">
                                    <div class="step-progress">STEP 3 OF 5</div>
                                    <h3 class="step-title">Review Parsed Data</h3>
                                    <p class="step-description">Verify the HL7 message structure and segments</p>
                                </div>
                                
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

                            <!-- Step 4: Mapping -->
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

                                <!-- Deployment Settings -->
                                <div id="wizardDeploymentSettingsContainer"></div>

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
        
        console.log('Wizard modal component loaded with connectivity fields');
        
        // Setup modal event listeners after HTML is loaded
        setupWizardModalEvents();
    }
    
    // Setup wizard modal specific events
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

        // Auto-suggest and show connectivity configuration based on selection
        setupConnectivityHandlers();
        
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
        
        console.log('Wizard modal events setup complete');
    }

    // Setup connectivity configuration handlers
    function setupConnectivityHandlers() {
        const sourceConnectivitySelect = document.getElementById('wizardSourceConnectivity');
        const targetConnectivitySelect = document.getElementById('wizardTargetConnectivity');
        const sourceConnectivityConfig = document.getElementById('wizardSourceConnectivityConfig');
        const targetConnectivityConfig = document.getElementById('wizardTargetConnectivityConfig');

        // Source connectivity change handler
        if (sourceConnectivitySelect && sourceConnectivityConfig) {
            sourceConnectivitySelect.addEventListener('change', function() {
                const selectedConnectivity = this.value;
                const configSections = sourceConnectivityConfig.querySelectorAll('.config-section');
                
                // Hide all config sections
                configSections.forEach(section => {
                    section.style.display = 'none';
                });
                
                if (selectedConnectivity) {
                    // Show the relevant config section
                    const relevantSection = sourceConnectivityConfig.querySelector(`[data-connectivity="${selectedConnectivity}"]`);
                    if (relevantSection) {
                        relevantSection.style.display = 'block';
                    }
                    sourceConnectivityConfig.style.display = 'block';
                } else {
                    sourceConnectivityConfig.style.display = 'none';
                }
            });
        }

        // Target connectivity change handler
        if (targetConnectivitySelect && targetConnectivityConfig) {
            targetConnectivitySelect.addEventListener('change', function() {
                const selectedConnectivity = this.value;
                const configSections = targetConnectivityConfig.querySelectorAll('.config-section');
                
                // Hide all config sections
                configSections.forEach(section => {
                    section.style.display = 'none';
                });
                
                if (selectedConnectivity) {
                    // Show the relevant config section
                    const relevantSection = targetConnectivityConfig.querySelector(`[data-connectivity="${selectedConnectivity}"]`);
                    if (relevantSection) {
                        relevantSection.style.display = 'block';
                    }
                    targetConnectivityConfig.style.display = 'block';
                } else {
                    targetConnectivityConfig.style.display = 'none';
                }
            });
        }

        // Auto-suggest connectivity based on format selection
        const sourceTypeSelect = document.getElementById('wizardSourceType');
        const targetTypeSelect = document.getElementById('wizardTargetType');

        if (sourceTypeSelect && sourceConnectivitySelect) {
            sourceTypeSelect.addEventListener('change', function() {
                const format = this.value;
                const suggestions = {
                    'hl7v2': 'tcp',
                    'hl7v3': 'http',
                    'fhir': 'http',
                    'ccda': 'http',
                    'file': 'file',
                    'database': 'database'
                };
                
                if (suggestions[format]) {
                    sourceConnectivitySelect.value = suggestions[format];
                    // Trigger change event to show config
                    sourceConnectivitySelect.dispatchEvent(new Event('change'));
                }
            });
        }

        if (targetTypeSelect && targetConnectivitySelect) {
            targetTypeSelect.addEventListener('change', function() {
                const format = this.value;
                const suggestions = {
                    'fhir': 'http',
                    'hl7v2': 'tcp',
                    'ccda': 'http',
                    'database': 'database',
                    'flatfile': 'file',
                    'json': 'http',
                    'xml': 'http'
                };
                
                if (suggestions[format]) {
                    targetConnectivitySelect.value = suggestions[format];
                    // Trigger change event to show config
                    targetConnectivitySelect.dispatchEvent(new Event('change'));
                }
            });
        }
    }
    
    // Load component when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', loadWizardComponent);
    } else {
        loadWizardComponent();
    }
    
})();