// step-handlers.js - Modular step handlers working with actual API response structure
// Each step has its own handler class for better organization and maintainability

/**
 * Base Step Handler - Common functionality for all steps
 */
class BaseStepHandler {
    constructor(wizardInstance) {
        this.wizard = wizardInstance;
    }

    getElement(id) {
        return this.wizard.getElement(id);
    }

    addEventListenerSafe(elementId, eventType, handler) {
        return this.wizard.addEventListenerSafe(elementId, eventType, handler);
    }

    showNotification(message, type) {
        return this.wizard.notificationService.show(message, type);
    }

    // Override in subclasses
    setupEventListeners() {}
    initialize() {}
    validate() { return true; }
    reset() {}
}

/**
 * Step 1: Configuration Handler
 */
class ConfigurationStepHandler extends BaseStepHandler {
    setupEventListeners() {
        this.addEventListenerSafe('wizardSourceType', 'change', (e) => this.updateSourceConfig(e.target.value));
        this.addEventListenerSafe('wizardTargetType', 'change', (e) => this.updateTargetConfig(e.target.value));
        console.log('✅ Configuration step event listeners setup');
    }

    validate() {
        const nameElement = this.getElement('wizardInterfaceName');
        const sourceElement = this.getElement('wizardSourceType');
        const targetElement = this.getElement('wizardTargetType');
        
        const name = nameElement ? nameElement.value.trim() : '';
        const sourceType = sourceElement ? sourceElement.value : '';
        const targetType = targetElement ? targetElement.value : '';
        
        if (name && sourceType && targetType) {
            this.wizard.wizardData.name = name;
            
            const descElement = this.getElement('wizardInterfaceDescription');
            const messageElement = this.getElement('wizardMessageType');
            
            this.wizard.wizardData.description = descElement ? descElement.value.trim() : '';
            this.wizard.wizardData.sourceType = sourceType;
            this.wizard.wizardData.targetType = targetType;
            this.wizard.wizardData.messageType = messageElement ? messageElement.value : 'auto-detect';
            return true;
        }
        
        if (!name || !sourceType || !targetType) {
            this.showNotification('Please fill in all required fields', 'error');
        }
        
        return false;
    }

    updateSourceConfig(sourceType) {
        const container = this.getElement('wizardSourceConfig');
        
        if (!sourceType || !container) {
            if (container) container.style.display = 'none';
            return;
        }
        
        container.style.display = 'block';
        let html = '<label>Source Configuration</label><div style="background: var(--gray-50); padding: 16px; border-radius: 8px; border: 1px solid var(--pink-200);">';
        
        switch (sourceType) {
            case 'file':
                html += `
                    <div class="form-row">
                        <div class="form-group">
                            <label>Input Directory</label>
                            <input type="text" class="form-control" placeholder="/path/to/input/files" value="/data/hl7/input">
                        </div>
                        <div class="form-group">
                            <label>File Pattern</label>
                            <input type="text" class="form-control" placeholder="*.hl7" value="*.hl7">
                        </div>
                    </div>`;
                break;
            case 'tcp':
                html += `
                    <div class="form-row">
                        <div class="form-group">
                            <label>Host</label>
                            <input type="text" class="form-control" placeholder="localhost" value="0.0.0.0">
                        </div>
                        <div class="form-group">
                            <label>Port</label>
                            <input type="number" class="form-control" placeholder="2575" value="2575">
                        </div>
                    </div>`;
                break;
            case 'http':
                html += `
                    <div class="form-group">
                        <label>Endpoint Path</label>
                        <input type="text" class="form-control" placeholder="/api/hl7/receive" value="/api/hl7/receive">
                    </div>`;
                break;
            case 'manual':
                html += `<div style="color: var(--gray-500); font-size: 14px; text-align: center; padding: 20px;">
                    <div style="font-size: 24px; margin-bottom: 8px;">✋</div>
                    <strong>Manual Input Mode</strong><br>
                    Messages will be processed on-demand through the interface dashboard
                </div>`;
                break;
        }
        
        html += '</div>';
        container.innerHTML = html;
    }
    
    updateTargetConfig(targetType) {
        const container = this.getElement('wizardTargetConfig');
        
        if (!targetType || !container) {
            if (container) container.style.display = 'none';
            return;
        }
        
        container.style.display = 'block';
        let html = '<label>Target Configuration</label><div style="background: var(--gray-50); padding: 16px; border-radius: 8px; border: 1px solid var(--pink-200);">';
        
        switch (targetType) {
            case 'fhir':
                html += `
                    <div class="form-group">
                        <label>FHIR Server URL</label>
                        <input type="url" class="form-control" placeholder="https://fhir.server.com/fhir" value="https://hapi.fhir.org/baseR4">
                    </div>`;
                break;
            case 'database':
                html += `
                    <div class="form-group">
                        <label>Database Connection</label>
                        <input type="text" class="form-control" placeholder="postgresql://user:pass@localhost/db" value="postgresql://ezhealth:****@localhost/fhir_data">
                    </div>`;
                break;
            case 'file':
                html += `
                    <div class="form-group">
                        <label>Output Directory</label>
                        <input type="text" class="form-control" placeholder="/path/to/output/files" value="/data/fhir/output">
                    </div>`;
                break;
            case 'http':
                html += `
                    <div class="form-group">
                        <label>Target URL</label>
                        <input type="url" class="form-control" placeholder="https://target-system.com/api" value="https://ehr-system.hospital.com/api/fhir">
                    </div>`;
                break;
        }
        
        html += '</div>';
        container.innerHTML = html;
    }

    collectSourceConfig() {
        const sourceType = this.wizard.wizardData.sourceType;
        const config = {};
        const container = this.getElement('wizardSourceConfig');
        
        if (!container || container.style.display === 'none') {
            return config;
        }

        const inputs = container.querySelectorAll('input, select');
        inputs.forEach(input => {
            if (input.value && input.value.trim()) {
                const key = input.previousElementSibling?.textContent?.toLowerCase()
                    ?.replace(/[^a-z0-9]/g, '') || input.name || 'value';
                config[key] = input.value.trim();
            }
        });

        return config;
    }

    collectTargetConfig() {
        const targetType = this.wizard.wizardData.targetType;
        const config = {};
        const container = this.getElement('wizardTargetConfig');
        
        if (!container || container.style.display === 'none') {
            return config;
        }

        const inputs = container.querySelectorAll('input, select');
        inputs.forEach(input => {
            if (input.value && input.value.trim()) {
                const key = input.previousElementSibling?.textContent?.toLowerCase()
                    ?.replace(/[^a-z0-9]/g, '') || input.name || 'value';
                config[key] = input.value.trim();
            }
        });

        return config;
    }

    reset() {
        const nameInput = this.getElement('wizardInterfaceName');
        const descInput = this.getElement('wizardInterfaceDescription');
        const sourceSelect = this.getElement('wizardSourceType');
        const targetSelect = this.getElement('wizardTargetType');
        const messageSelect = this.getElement('wizardMessageType');
        
        if (nameInput) nameInput.value = '';
        if (descInput) descInput.value = '';
        if (sourceSelect) sourceSelect.value = '';
        if (targetSelect) targetSelect.value = '';
        if (messageSelect) messageSelect.value = 'auto-detect';
        
        const sourceConfig = this.getElement('wizardSourceConfig');
        const targetConfig = this.getElement('wizardTargetConfig');
        if (sourceConfig) sourceConfig.style.display = 'none';
        if (targetConfig) targetConfig.style.display = 'none';
    }
}

/**
 * Step 2: Upload Handler - Works with actual HL7 parsing API
 */
class UploadStepHandler extends BaseStepHandler {
    setupEventListeners() {
        const uploadZone = this.getElement('uploadZone');
        const fileUpload = this.getElement('hl7FileUpload');
        
        if (uploadZone && fileUpload) {
            uploadZone.addEventListener('click', () => fileUpload.click());
            fileUpload.addEventListener('change', (e) => this.handleFileUpload(e));
        }
        
        this.addEventListenerSafe('parseBtn', 'click', () => this.parseHL7Message());
        this.setupDragAndDrop();
        this.enhanceUploadStep();
        console.log('✅ Upload step event listeners setup');
    }

    setupDragAndDrop() {
        const zone = this.getElement('uploadZone');
        if (!zone) return;
        
        zone.addEventListener('dragover', (e) => {
            e.preventDefault();
            zone.classList.add('dragover');
        });
        
        zone.addEventListener('dragleave', () => {
            zone.classList.remove('dragover');
        });
        
        zone.addEventListener('drop', (e) => {
            e.preventDefault();
            zone.classList.remove('dragover');
            const files = e.dataTransfer.files;
            if (files.length > 0) {
                this.processFile(files[0]);
            }
        });
    }

    enhanceUploadStep() {
        const step2 = this.getElement('step2');
        if (!step2) return;
        
        const stepDescription = step2.querySelector('.step-description');
        if (stepDescription) {
            stepDescription.innerHTML = `
                <div style="display: flex; align-items: center; justify-content: center; gap: 6px; margin-bottom: 8px;">
                    Upload a sample HL7 message to analyze with
                    <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 16px; height: 16px; border-radius: 2px;">
                    <strong>ezHealthKonnect</strong>, or skip to use standard schemas
                </div>
            `;
        }

        const parseResults = this.getElement('parseResults');
        if (parseResults && !this.getElement('skipUploadBtn')) {
            const skipSection = document.createElement('div');
            skipSection.innerHTML = `
                <div style="text-align: center; margin: 32px 0 24px; position: relative;">
                    <div style="border-top: 1px solid var(--pink-200); position: absolute; top: 50%; left: 0; right: 0;"></div>
                    <span style="background: white; padding: 0 16px; color: var(--gray-500); font-size: 14px; font-weight: 600;">OR</span>
                </div>
                
                <div style="text-align: center;">
                    <button id="skipUploadBtn" class="wizard-btn secondary" style="display: flex; align-items: center; justify-content: center; margin: 0 auto; gap: 8px;">
                        <span>📋</span>
                        Skip Upload - Use Standard Schema
                    </button>
                    <p style="color: var(--gray-500); font-size: 12px; margin-top: 8px;">
                        Generate interface based on HL7 v2.5 specifications for your selected message type
                    </p>
                </div>
            `;
            
            parseResults.parentNode.insertBefore(skipSection, parseResults.nextSibling);

            const skipBtn = document.getElementById('skipUploadBtn');
            if (skipBtn) {
                skipBtn.addEventListener('click', () => this.skipUploadUseSchema());
            }
        }
    }

    handleFileUpload(event) {
        const file = event.target.files[0];
        if (file) {
            this.processFile(file);
        }
    }
    
    processFile(file) {
        if (file.size > 5 * 1024 * 1024) {
            this.showNotification('File size must be less than 5MB', 'error');
            return;
        }
        
        const allowedTypes = ['.hl7', '.txt', '.dat'];
        const fileExtension = '.' + file.name.split('.').pop().toLowerCase();
        
        if (!allowedTypes.includes(fileExtension) && file.type !== 'text/plain') {
            this.showNotification('Please upload a valid HL7 file (.hl7, .txt, .dat)', 'error');
            return;
        }
        
        this.showFileInfo(file);
        
        const parseBtn = this.getElement('parseBtn');
        if (parseBtn) parseBtn.style.display = 'flex';
        
        this.wizard.uploadedFile = file;
        this.showNotification('File uploaded successfully!', 'success');
    }
    
    showFileInfo(file) {
        const fileInfo = this.getElement('fileInfo');
        if (!fileInfo) return;
        
        fileInfo.innerHTML = `
            <div class="file-info-card">
                <div class="file-icon">📄</div>
                <div class="file-details">
                    <div class="file-name">${file.name}</div>
                    <div class="file-meta">${this.formatFileSize(file.size)} • ${file.type || 'HL7 Message'} • Modified ${file.lastModified ? new Date(file.lastModified).toLocaleDateString() : 'Unknown'}</div>
                </div>
                <div style="color: var(--wizard-success); font-size: 20px;">✓</div>
            </div>
        `;
    }
    
    formatFileSize(bytes) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    }

    async parseHL7Message() {
        if (!this.wizard.uploadedFile) {
            this.showNotification('Please select a file first', 'error');
            return;
        }
        
        this.wizard.showLoading('Parsing HL7 message...', 'Analyzing message structure with ezHealthKonnect clinical intelligence');
        
        try {
            const fileContent = await this.readFileContent(this.wizard.uploadedFile);
            
            console.log('📤 Sending HL7 message to API for parsing...');
            
            // Use the actual API endpoint structure
            const result = await this.wizard.hl7Service.parseHL7Message(fileContent, true);
            
            if (!result.success) {
                throw new Error(result.error || 'Parsing failed');
            }
            
            console.log('✅ HL7 message parsed successfully by ezHealthKonnect');
            console.log('📊 Parse result:', result.data);
            
            this.wizard.parsedHL7Data = result;
            
            this.wizard.hideLoading();
            this.showParseResults(result);
            this.showNotification('HL7 message parsed successfully!', 'success');
            
            setTimeout(() => this.wizard.nextStep(), 1500);
            
        } catch (error) {
            console.error('❌ HL7 parsing failed:', error);
            this.wizard.hideLoading();
            this.showNotification(`Failed to parse HL7: ${error.message}`, 'error');
        }
    }

    readFileContent(file) {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = e => resolve(e.target.result);
            reader.onerror = reject;
            reader.readAsText(file);
        });
    }

    showParseResults(apiResponse) {
        const container = this.getElement('parseResults');
        if (!container) return;
        
        const data = apiResponse.data;
        const validationErrors = data.validationErrors || [];
        
        container.innerHTML = `
            <div class="parse-results">
                <div class="result-header">
                    <div style="display: flex; align-items: center; gap: 8px;">
                        <span class="success-checkmark">✅</span>
                        <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 20px; height: 20px; border-radius: 3px;">
                        <h4 style="margin: 0;">ezHealthKonnect Parse Results</h4>
                    </div>
                    <div class="message-type-badge">${data.messageType?.name || 'Unknown'}</div>
                </div>
                
                <div class="result-stats">
                    <div class="stat-card">
                        <div class="stat-number">${Object.keys(data.enhancedSegments || {}).length}</div>
                        <div class="stat-label">Segments</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-number">${data.dictionaryUsed ? '✨' : '⚡'}</div>
                        <div class="stat-label">${data.dictionaryUsed ? 'Enhanced' : 'Basic'}</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-number">${Object.values(data.enhancedSegments || {}).reduce((sum, seg) => sum + (seg.fieldCount || 0), 0)}</div>
                        <div class="stat-label">Fields</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-number">${validationErrors.length}</div>
                        <div class="stat-label">Issues</div>
                    </div>
                </div>
                
                ${validationErrors.length > 0 ? `
                    <div class="alert alert-warning">
                        <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
                            <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 24px; height: 24px; border-radius: 4px;">
                            <h5 style="margin: 0;">⚠️ Validation Issues Found</h5>
                        </div>
                        <p>${validationErrors.length} validation issue${validationErrors.length > 1 ? 's' : ''} detected. Review details in the next step.</p>
                    </div>
                ` : `
                    <div class="alert alert-success">
                        <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
                            <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 24px; height: 24px; border-radius: 4px;">
                            <h5 style="margin: 0;">🚀 Message Validated Successfully</h5>
                        </div>
                        <p>Your HL7 message passed all validation checks and is ready for mapping.</p>
                    </div>
                `}
                
                <div style="text-align: center; margin-top: 16px;">
                    <button class="wizard-btn secondary" onclick="window.wizard.segmentViewer.showSegmentSummary()">
                        <span>📋</span>
                        View Segment Summary
                    </button>
                </div>
            </div>
        `;
    }

    async skipUploadUseSchema() {
        this.wizard.showLoading('Loading standard schema...', 'Generating interface based on ezHealthKonnect HL7 specifications');
        
        try {
            const messageType = this.wizard.wizardData.messageType || 'ADT^A01';
            
            await this.wizard.delay(1500);
            
            // Generate schema-based structure using the HL7 service
            this.wizard.parsedHL7Data = this.wizard.hl7Service.generateMockSchemaStructure(messageType);
            
            this.wizard.hideLoading();
            this.showNotification('Standard schema loaded successfully!', 'success');
            
            setTimeout(() => this.wizard.nextStep(), 1000);
            
        } catch (error) {
            this.wizard.hideLoading();
            this.showNotification(`Failed to load schema: ${error.message}`, 'error');
        }
    }

    reset() {
        const fileUpload = this.getElement('hl7FileUpload');
        if (fileUpload) fileUpload.value = '';
        
        const fileInfo = this.getElement('fileInfo');
        if (fileInfo) fileInfo.innerHTML = '';
        
        const parseBtn = this.getElement('parseBtn');
        if (parseBtn) parseBtn.style.display = 'none';
        
        const parseResults = this.getElement('parseResults');
        if (parseResults) parseResults.innerHTML = '';
    }
}

/**
 * Step 3: Review Handler - Uses SegmentViewer for drilling
 */
class ReviewStepHandler extends BaseStepHandler {
    initialize() {
        if (this.wizard.parsedHL7Data) {
            this.updateStep3Content();
        }
    }

    updateStep3Content() {
        const container = this.getElement('parsedDataReview');
        if (!container) return;
        
        // Use the SegmentViewer to render the parsed data with drilling capability
        this.wizard.segmentViewer.renderSegmentList(this.wizard.parsedHL7Data, 'parsedDataReview');
    }

    reset() {
        const container = this.getElement('parsedDataReview');
        if (container) container.innerHTML = '';
    }
}

/**
 * Step 4: Mapping Handler
 */
class MappingStepHandler extends BaseStepHandler {
    setupEventListeners() {
        this.addEventListenerSafe('generateMappingBtn', 'click', () => this.generateMapping());
        console.log('✅ Mapping step event listeners setup');
    }

    async generateMapping() {
        this.wizard.showLoading('Generating AI mappings...', 'Analyzing HL7 structure with ezHealthKonnect intelligence and suggesting FHIR mappings');
        
        await this.wizard.delay(3000);
        
        this.wizard.hideLoading();
        this.showMappingResults();
        this.showNotification('Mapping suggestions generated successfully!', 'success');
    }
    
    showMappingResults() {
        const container = this.getElement('mappingResults');
        if (!container) return;
        
        const messageType = this.wizard.parsedHL7Data?.data?.messageType?.name || 'ADT^A01';
        
        let mappings = [];
        
        if (messageType.startsWith('ADT')) {
            mappings = [
                { source: 'PID.5 (Patient Name)', target: 'Patient.name', confidence: 95 },
                { source: 'PID.7 (Date of Birth)', target: 'Patient.birthDate', confidence: 98 },
                { source: 'PID.8 (Gender)', target: 'Patient.gender', confidence: 92 },
                { source: 'PID.11 (Address)', target: 'Patient.address', confidence: 88 },
                { source: 'PID.13 (Phone)', target: 'Patient.telecom', confidence: 85 },
                { source: 'PV1.2 (Patient Class)', target: 'Encounter.class', confidence: 90 },
                { source: 'PV1.44 (Admit Date)', target: 'Encounter.period.start', confidence: 93 },
                { source: 'PV1.7 (Attending Doctor)', target: 'Encounter.participant', confidence: 87 }
            ];
        } else if (messageType.startsWith('ORU')) {
            mappings = [
                { source: 'PID.5 (Patient Name)', target: 'Patient.name', confidence: 95 },
                { source: 'OBR.4 (Universal Service ID)', target: 'DiagnosticReport.code', confidence: 92 },
                { source: 'OBX.3 (Observation Identifier)', target: 'Observation.code', confidence: 94 },
                { source: 'OBX.5 (Observation Value)', target: 'Observation.value', confidence: 89 },
                { source: 'OBX.6 (Units)', target: 'Observation.valueQuantity.unit', confidence: 91 },
                { source: 'OBX.7 (Reference Range)', target: 'Observation.referenceRange', confidence: 86 }
            ];
        }
        
        const avgConfidence = Math.round(mappings.reduce((sum, m) => sum + m.confidence, 0) / mappings.length);
        
        container.innerHTML = `
            <div class="mapping-preview">
                <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 12px;">
                    <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 20px; height: 20px; border-radius: 3px;">
                    <h4 style="margin: 0;">🤖 AI-Generated Mapping Suggestions</h4>
                </div>
                <p style="margin-bottom: 20px;">Smart field mappings from your HL7 ${messageType} message to FHIR R4 resources:</p>
                
                <div class="mapping-items">
                    ${mappings.map(mapping => `
                        <div class="mapping-item">
                            <div class="mapping-source">${mapping.source}</div>
                            <div class="mapping-arrow">→</div>
                            <div class="mapping-target">${mapping.target}</div>
                            <div class="mapping-confidence ${mapping.confidence >= 90 ? 'high' : mapping.confidence >= 80 ? 'medium' : 'low'}">
                                ${mapping.confidence}%
                            </div>
                        </div>
                    `).join('')}
                </div>
                
                <div style="margin-top: 20px; padding: 16px; background: var(--wizard-pink-light); border-radius: 8px; border: 1px solid var(--pink-200);">
                    <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
                        <span>🎯</span>
                        <strong style="color: var(--navy-primary);">Average Mapping Confidence: ${avgConfidence}%</strong>
                    </div>
                    <div style="font-size: 13px; color: var(--gray-600);">
                        ${mappings.length} HL7 fields mapped to FHIR R4 resources based on ezHealthKonnect analysis.
                        Enhanced with ${this.wizard.parsedHL7Data?.data?.dictionaryUsed ? 'HL7 dictionary metadata' : 'basic field detection'}.
                    </div>
                </div>
            </div>
        `;
    }

    generateMapping() {
        const mapping = {};

        if (this.wizard.parsedHL7Data?.data) {
            const messageType = this.wizard.parsedHL7Data.data.messageType?.name;
            
            if (messageType?.startsWith('ADT')) {
                mapping.patient = {
                    'PID.5': 'Patient.name',
                    'PID.7': 'Patient.birthDate', 
                    'PID.8': 'Patient.gender',
                    'PID.11': 'Patient.address',
                    'PID.13': 'Patient.telecom'
                };
                mapping.encounter = {
                    'PV1.2': 'Encounter.class',
                    'PV1.44': 'Encounter.period.start',
                    'PV1.7': 'Encounter.participant'
                };
            } else if (messageType?.startsWith('ORU')) {
                mapping.patient = {
                    'PID.5': 'Patient.name'
                };
                mapping.observation = {
                    'OBX.3': 'Observation.code',
                    'OBX.5': 'Observation.value',
                    'OBX.6': 'Observation.valueQuantity.unit'
                };
                mapping.diagnosticReport = {
                    'OBR.4': 'DiagnosticReport.code'
                };
            }

            const customSegments = Object.keys(this.wizard.parsedHL7Data.data.enhancedSegments || {}).filter(seg => seg.startsWith('Z'));
            if (customSegments.length > 0) {
                mapping.customSegments = {};
                customSegments.forEach(seg => {
                    mapping.customSegments[seg] = {
                        description: this.wizard.parsedHL7Data.data.enhancedSegments[seg].description,
                        requiresManualMapping: true
                    };
                });
            }
        }

        return mapping;
    }

    reset() {
        const container = this.getElement('mappingResults');
        if (container) {
            container.innerHTML = `
                <div class="mapping-preview">
                    <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 12px;">
                        <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 20px; height: 20px; border-radius: 3px;">
                        <h4 style="margin: 0;">🎯 Ready for AI-Powered Mapping</h4>
                    </div>
                    <p>Click the button above to generate intelligent mapping suggestions based on your HL7 message structure.</p>
                    <div style="margin-top: 20px; padding: 16px; background: white; border-radius: 8px; border: 1px solid var(--pink-200);">
                        <div style="font-size: 12px; color: var(--gray-500); margin-bottom: 8px;">COMING SOON IN PHASE 2</div>
                        <div style="font-weight: 600; color: var(--navy-primary);">🚀 Claude AI Integration</div>
                    </div>
                </div>
            `;
        }
    }
}

/**
 * Step 5: Summary Handler
 */
class SummaryStepHandler extends BaseStepHandler {
    initialize() {
        this.updateSummary();
    }

    updateSummary() {
        const summaryName = this.getElement('summaryName');
        const summaryType = this.getElement('summaryType');
        const summaryMessage = this.getElement('summaryMessage');
        const summarySource = this.getElement('summarySource');
        const summaryTarget = this.getElement('summaryTarget');
        const summarySegments = this.getElement('summarySegments');
        const summaryZSegments = this.getElement('summaryZSegments');
        
        if (summaryName) summaryName.textContent = this.wizard.wizardData.name || 'New Interface';
        if (summaryType) summaryType.textContent = `${this.wizard.wizardData.sourceType || 'Source'} → ${this.wizard.wizardData.targetType || 'Target'}`;
        if (summaryMessage) summaryMessage.textContent = this.wizard.wizardData.messageType || 'Auto-detect';
        if (summarySource) summarySource.textContent = this.wizard.wizardData.sourceType ? this.wizard.wizardData.sourceType.toUpperCase() : '-';
        if (summaryTarget) summaryTarget.textContent = this.wizard.wizardData.targetType ? this.wizard.wizardData.targetType.toUpperCase() : '-';
        
        if (this.wizard.parsedHL7Data?.data) {
            const data = this.wizard.parsedHL7Data.data;
            if (summarySegments) summarySegments.textContent = Object.keys(data.enhancedSegments || {}).length;
            if (summaryZSegments) summaryZSegments.textContent = Object.keys(data.enhancedSegments || {}).filter(seg => seg.startsWith('Z')).length;
        }
    }

    reset() {
        // Summary is generated dynamically, no reset needed
    }
}

// Export for module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { 
        BaseStepHandler, 
        ConfigurationStepHandler, 
        UploadStepHandler, 
        ReviewStepHandler, 
        MappingStepHandler, 
        SummaryStepHandler 
    };
}