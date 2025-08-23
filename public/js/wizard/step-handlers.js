// step-handlers.js - Enhanced modular step handlers with FHIR Transform Integration
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

    showNotification(message, type = 'info') {
        console.log(`[${type.toUpperCase()}] ${message}`);
        
        // Create notification element
        const notification = document.createElement('div');
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: ${
                type === 'success' ? '#10b981' : 
                type === 'error' ? '#ef4444' : 
                type === 'warning' ? '#f59e0b' : 
                '#3b82f6'
            };
            color: white;
            padding: 12px 20px;
            border-radius: 8px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
            z-index: 10000;
            animation: slideIn 0.3s ease;
            max-width: 350px;
            font-size: 14px;
        `;
        notification.textContent = message;
        document.body.appendChild(notification);
        
        setTimeout(() => {
            notification.style.animation = 'slideOut 0.3s ease';
            setTimeout(() => notification.remove(), 300);
        }, 3000);
    }
}

/**
 * Step 2: Upload Handler - BROWSER-SAFE FILE UPLOAD
 */
class UploadStepHandler extends BaseStepHandler {
    constructor(wizardInstance) {
        super(wizardInstance);
        this.fileUploadSetup = false;
    }

    // Called when step 2 is shown
    initialize() {
        console.log('🔧 Step 2 initialize() called - setting up browser-safe file upload');
        this.setupBrowserSafeFileUpload();
    }

    // Called when modal opens
    setupEventListeners() {
        console.log('✅ Upload step event listeners setup called from modal open');
        this.addEventListenerSafe('parseBtn', 'click', () => this.parseHL7Message());
        this.enhanceUploadStep();
    }

    setupBrowserSafeFileUpload() {
        if (this.fileUploadSetup) {
            console.log('🔍 File upload already configured, skipping');
            return;
        }

        console.log('🔧 Setting up browser-safe file upload for step 2');
        
        const uploadZone = this.getElement('uploadZone');
        const fileInput = this.getElement('hl7FileUpload');
        
        if (!uploadZone || !fileInput) {
            console.error('❌ Upload elements not found:', {
                uploadZone: !!uploadZone,
                fileInput: !!fileInput
            });
            return;
        }

        // Method 1: Transparent Overlay Approach (Most Reliable)
        this.setupTransparentOverlay(uploadZone, fileInput);
        
        // Method 2: Fallback Direct Click Handler
        this.setupDirectClickHandler(uploadZone, fileInput);
        
        // Setup drag and drop
        this.setupDragAndDrop(uploadZone);
        
        this.fileUploadSetup = true;
        console.log('✅ Browser-safe file upload configured successfully');
    }

    setupTransparentOverlay(uploadZone, fileInput) {
        console.log('🔧 Setting up transparent overlay method');
        
        // Make upload zone relative positioned
        uploadZone.style.position = 'relative';
        uploadZone.style.overflow = 'hidden';
        
        // Position file input as transparent overlay
        fileInput.style.position = 'absolute';
        fileInput.style.top = '0';
        fileInput.style.left = '0';
        fileInput.style.width = '100%';
        fileInput.style.height = '100%';
        fileInput.style.opacity = '0';
        fileInput.style.cursor = 'pointer';
        fileInput.style.zIndex = '10';
        fileInput.style.fontSize = '100px'; // Large font size helps with click area in some browsers
        
        // File change handler
        fileInput.onchange = (e) => {
            console.log('📁 File selected via transparent overlay');
            this.handleFileUpload(e);
        };
        
        console.log('✅ Transparent overlay configured');
    }

    setupDirectClickHandler(uploadZone, fileInput) {
        console.log('🔧 Setting up direct click handler as fallback');
        
        // Remove any existing click handlers
        uploadZone.onclick = null;
        
        // Direct onclick assignment (more reliable than addEventListener for file inputs)
        uploadZone.onclick = (e) => {
            // Only trigger if the click wasn't on the file input itself
            if (e.target !== fileInput) {
                console.log('🔍 Upload zone clicked - triggering file input');
                
                // Direct click trigger
                try {
                    fileInput.click();
                    console.log('✅ File input triggered via direct click');
                } catch (error) {
                    console.error('❌ Direct click failed:', error);
                }
            }
        };
        
        console.log('✅ Direct click handler configured');
    }

    setupDragAndDrop(uploadZone) {
        console.log('🔧 Setting up drag and drop');
        
        ['dragover', 'dragenter'].forEach(eventName => {
            uploadZone.addEventListener(eventName, (e) => {
                e.preventDefault();
                e.stopPropagation();
                uploadZone.classList.add('dragover');
            });
        });
        
        ['dragleave', 'dragend'].forEach(eventName => {
            uploadZone.addEventListener(eventName, (e) => {
                e.preventDefault();
                e.stopPropagation();
                uploadZone.classList.remove('dragover');
            });
        });
        
        uploadZone.addEventListener('drop', (e) => {
            e.preventDefault();
            e.stopPropagation();
            uploadZone.classList.remove('dragover');
            
            const files = e.dataTransfer.files;
            if (files.length > 0) {
                console.log('📁 File dropped:', files[0].name);
                this.processFile(files[0]);
            }
        });
        
        console.log('✅ Drag and drop configured');
    }

    handleFileUpload(event) {
        const file = event.target.files[0];
        if (file) {
            console.log('📁 Processing uploaded file:', file.name);
            this.processFile(file);
        }
    }
    
    processFile(file) {
        console.log('🔍 Processing file:', file.name);
        
        // File validation
        if (file.size > 5 * 1024 * 1024) {
            this.showNotification('File size must be less than 5MB', 'error');
            return;
        }
        
        const allowedTypes = ['.hl7', '.txt', '.dat', '.msg'];
        const fileExtension = '.' + file.name.split('.').pop().toLowerCase();
        
        if (!allowedTypes.includes(fileExtension) && file.type !== 'text/plain') {
            this.showNotification(`Please upload a valid HL7 file (${allowedTypes.join(', ')})`, 'error');
            return;
        }
        
        this.showFileInfo(file);
        
        const parseBtn = this.getElement('parseBtn');
        if (parseBtn) parseBtn.style.display = 'flex';
        
        this.wizard.uploadedFile = file;
        this.showNotification('File uploaded successfully!', 'success');
        
        console.log('✅ File processed successfully:', {
            name: file.name,
            size: file.size,
            type: file.type
        });
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
            this.showNotification('Please upload a file first', 'error');
            return;
        }
        
        try {
            this.wizard.showLoading('Parsing HL7 message...', 'Analyzing message structure and content');
            
            const fileContent = await this.readFileContent(this.wizard.uploadedFile);
            const parseResult = await this.wizard.hl7Service.parseHL7Message(fileContent);
            
            if (parseResult.success) {
                this.wizard.parsedHL7Data = parseResult;
                this.wizard.wizardData.detectedMessageType = parseResult.data.messageType;
                this.wizard.wizardData.enhancedSegments = parseResult.data.enhancedSegments;
                this.wizard.wizardData.parsedMessage = parseResult.data;
                
                // Store raw file content for Step 4 transform
                this.wizard.uploadedFileContent = fileContent;
                
                this.wizard.hideLoading();
                this.showNotification(`✅ HL7 message parsed successfully! Type: ${parseResult.data.messageType?.name || parseResult.data.messageType}`, 'success');
                
                // Auto advance to next step
                setTimeout(() => {
                    this.wizard.nextStep();
                }, 1000);
            } else {
                throw new Error(parseResult.error || 'Failed to parse HL7 message');
            }
            
        } catch (error) {
            this.wizard.hideLoading();
            console.error('❌ Parse error:', error);
            this.showNotification(`Failed to parse HL7 message: ${error.message}`, 'error');
        }
    }
    
    readFileContent(file) {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = (e) => resolve(e.target.result);
            reader.onerror = (e) => reject(new Error('Failed to read file'));
            reader.readAsText(file);
        });
    }

    enhanceUploadStep() {
        console.log('🎨 Enhancing upload step interface');
    }

    validate() {
        return this.wizard.uploadedFile !== null;
    }

    reset() {
        this.wizard.uploadedFile = null;
        this.wizard.parsedHL7Data = null;
        this.wizard.uploadedFileContent = null;
        this.fileUploadSetup = false; // Reset so it can be set up again
        
        const fileInfo = this.getElement('fileInfo');
        const parseBtn = this.getElement('parseBtn');
        const fileUpload = this.getElement('hl7FileUpload');
        
        if (fileInfo) fileInfo.innerHTML = '';
        if (parseBtn) parseBtn.style.display = 'none';
        if (fileUpload) {
            fileUpload.value = '';
            // Reset file input styling
            fileUpload.style.position = '';
            fileUpload.style.opacity = '';
            fileUpload.style.zIndex = '';
        }
    }

    showNotification(message, type = 'info') {
        console.log(`[${type.toUpperCase()}] ${message}`);
        
        // Create notification element
        const notification = document.createElement('div');
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: ${
                type === 'success' ? '#10b981' : 
                type === 'error' ? '#ef4444' : 
                type === 'warning' ? '#f59e0b' : 
                '#3b82f6'
            };
            color: white;
            padding: 12px 20px;
            border-radius: 8px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
            z-index: 10000;
            animation: slideIn 0.3s ease;
            max-width: 350px;
            font-size: 14px;
        `;
        notification.textContent = message;
        document.body.appendChild(notification);
        
        setTimeout(() => {
            notification.style.animation = 'slideOut 0.3s ease';
            setTimeout(() => notification.remove(), 300);
        }, 3000);
    }
}

/**
 * Step 3: Review Handler - Uses SegmentViewer for drilling
 */
class ReviewStepHandler extends BaseStepHandler {
    initialize() {
        if (this.wizard.parsedHL7Data) {
            this.updateReviewContent();
        }
    }

    updateReviewContent() {
        // Check if the parsedDataReview container exists
        const container = this.getElement('parsedDataReview');
        if (!container) {
            console.error('❌ parsedDataReview container not found');
            return;
        }

        // Show empty state if no parsed data available
        if (!this.wizard.parsedHL7Data) {
            container.innerHTML = `
                <div class="review-placeholder">
                    <div style="text-align: center; padding: 40px; color: #6b7280;">
                        <div style="font-size: 48px; margin-bottom: 16px;">📋</div>
                        <div style="font-weight: 600; margin-bottom: 8px;">No data to review yet</div>
                        <div>Upload and parse an HL7 message to see the structure here</div>
                    </div>
                </div>
            `;
            return;
        }

        // ✅ CORRECT: Use SegmentViewer to render the parsed data
        this.wizard.segmentViewer.renderSegmentList(this.wizard.parsedHL7Data, 'parsedDataReview');
    }

    reset() {
        const container = this.getElement('parsedDataReview');
        if (container) {
            container.innerHTML = `
                <div class="review-placeholder">
                    <div style="text-align: center; padding: 40px; color: #6b7280;">
                        <div style="font-size: 48px; margin-bottom: 16px;">📋</div>
                        <div style="font-weight: 600; margin-bottom: 8px;">No data to review yet</div>
                        <div>Upload and parse an HL7 message to see the structure here</div>
                    </div>
                </div>
            `;
        }
    }

    showNotification(message, type = 'info') {
        console.log(`[${type.toUpperCase()}] ${message}`);
        
        // Create notification element
        const notification = document.createElement('div');
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: ${
                type === 'success' ? '#10b981' : 
                type === 'error' ? '#ef4444' : 
                type === 'warning' ? '#f59e0b' : 
                '#3b82f6'
            };
            color: white;
            padding: 12px 20px;
            border-radius: 8px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
            z-index: 10000;
            animation: slideIn 0.3s ease;
            max-width: 350px;
            font-size: 14px;
        `;
        notification.textContent = message;
        document.body.appendChild(notification);
        
        setTimeout(() => {
            notification.style.animation = 'slideOut 0.3s ease';
            setTimeout(() => notification.remove(), 300);
        }, 3000);
    }

}

/**
 * Step 4: FHIR Transform Handler - UPDATED FOR NEW ENDPOINT
 */
class MappingStepHandler extends BaseStepHandler {
    constructor(wizard) {
        super(wizard);
        this.transformationResult = null;
        this.initialized = false;
        this.debugMode = true;
    }

    async initialize() {
        console.log('🎯 Initializing FHIR Transform Step 4 (NEW ENDPOINT)...');
        
        try {
            const messageType = this.getMessageTypeFromParsedData();
            console.log('✅ Message type:', messageType);
            
            if (!this.wizard.parsedHL7Data) {
                throw new Error('No parsed HL7 data available for transformation');
            }
            
            // Update Step 4 UI for transformation
            this.updateStep4TransformInterface();
            
            this.initialized = true;
            console.log('✅ Step 4 FHIR Transform initialization complete');
        } catch (error) {
            console.error('❌ Step 4 initialization failed:', error);
            this.loadErrorState(error);
        }
    }

    getMessageTypeFromParsedData() {
        if (this.debugMode) {
            console.log('🔍 Analyzing wizard data for message type:');
            console.log('🔍 parsedHL7Data:', this.wizard.parsedHL7Data);
        }

        // Strategy 1: From wizard detected message type
        if (this.wizard.wizardData.detectedMessageType) {
            const msgType = this.wizard.wizardData.detectedMessageType;
            if (typeof msgType === 'string' && msgType !== '[object Object]') {
                console.log('✅ Strategy 1 - Using detectedMessageType:', msgType);
                return msgType;
            }
        }

        // Strategy 2: From parsed HL7 data message type
        if (this.wizard.parsedHL7Data?.data?.messageType) {
            const msgTypeObj = this.wizard.parsedHL7Data.data.messageType;
            
            if (typeof msgTypeObj === 'string') {
                console.log('✅ Strategy 2a - Using messageType string:', msgTypeObj);
                return msgTypeObj;
            } else if (msgTypeObj.name && typeof msgTypeObj.name === 'string') {
                console.log('✅ Strategy 2b - Using messageType.name:', msgTypeObj.name);
                return msgTypeObj.name;
            } else if (msgTypeObj.value && typeof msgTypeObj.value === 'string') {
                console.log('✅ Strategy 2c - Using messageType.value:', msgTypeObj.value);
                return msgTypeObj.value;
            }
        }

        // Strategy 3: Extract from MSH.9 field
        if (this.wizard.parsedHL7Data?.data?.enhancedSegments?.MSH?.fields) {
            const mshFields = this.wizard.parsedHL7Data.data.enhancedSegments.MSH.fields;
            
            if (mshFields['MSH.9']?.value) {
                const msgType = mshFields['MSH.9'].value;
                console.log('✅ Strategy 3 - Using MSH.9 value:', msgType);
                return msgType;
            }
        }

        console.warn('⚠️ Could not determine message type, using default');
        return 'ADT^A01';
    }

    updateStep4TransformInterface() {
        const step4Element = document.getElementById('step4');
        if (!step4Element) {
            console.error('❌ Step 4 element not found');
            return;
        }

        const demoContent = step4Element.querySelector('.demo-content');
        if (demoContent) {
            const messageType = this.getMessageTypeFromParsedData();
            const segmentCount = Object.keys(this.wizard.parsedHL7Data?.data?.enhancedSegments || {}).length;
            
            demoContent.innerHTML = `
                <div class="fhir-transform-interface">
                    <div class="transform-header">
                        <h4>🔄 HL7 to FHIR Transformation</h4>
                        <p>Convert your parsed HL7 message to FHIR R4 format using our advanced transformation engine</p>
                    </div>
                    
                    <div class="transform-preview">
                        <div class="source-preview">
                            <div class="preview-icon">📋</div>
                            <h5>Source HL7 Message</h5>
                            <div class="message-info">
                                <span class="message-type">${messageType}</span>
                                <span class="segment-count">${segmentCount} segments</span>
                            </div>
                            <div class="source-details">
                                <div class="detail-item">
                                    <span class="detail-label">Version:</span>
                                    <span class="detail-value">${this.wizard.parsedHL7Data?.data?.version || '2.5'}</span>
                                </div>
                                <div class="detail-item">
                                    <span class="detail-label">Parsed:</span>
                                    <span class="detail-value">✅ Successfully</span>
                                </div>
                            </div>
                        </div>
                        
                        <div class="transform-arrow">
                            <div class="arrow-icon">→</div>
                            <div class="transform-label">Transform</div>
                        </div>
                        
                        <div class="target-preview">
                            <div class="preview-icon">🔥</div>
                            <h5>FHIR R4 Resources</h5>
                            <div id="fhir-resources-preview" class="resources-preview">
                                <button class="transform-btn" onclick="window.step4Handler.transformToFHIR()">
                                    <span class="btn-icon">🚀</span>
                                    <span class="btn-text">Transform to FHIR</span>
                                </button>
                                <div class="transform-hint">
                                    Click to generate FHIR resources from your HL7 message
                                </div>
                            </div>
                        </div>
                    </div>
                    
                    <div id="transformation-results" class="transformation-results" style="display: none;">
                        <!-- Results will be populated here -->
                    </div>
                </div>
            `;
            
            demoContent.className = 'fhir-transform-content';
        }
        
        // Make this handler available globally
        window.step4Handler = this;
        console.log('✅ Step 4 transform interface updated');
    }

    async transformToFHIR() {
        try {
            this.wizard.showLoading('Transforming HL7 to FHIR...', 'Converting your HL7 message to FHIR R4 format');
            
            // Prepare the data for the new endpoint
            const requestData = {
                ParsedHL7Data: JSON.stringify({
                    success: true,
                    data: this.wizard.parsedHL7Data.data
                }),
                createBundle: true,
                validationMode: "strict"
            };
            
            console.log('📡 Sending transform request:', requestData);
            
            // Call the new transform endpoint
            const response = await fetch('http://localhost:8080/api/fhir/test-transform-v3', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(requestData)
            });
            
            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(`HTTP ${response.status}: ${errorText}`);
            }
            
            const transformResult = await response.json();
            console.log('✅ Transform response:', transformResult);
            
            this.wizard.hideLoading();
            
            // Store transformation result
            this.transformationResult = transformResult;
            
            if (transformResult.success !== false) {
                this.displayTransformationResult(transformResult);
                this.showNotification('✅ HL7 successfully transformed to FHIR!', 'success');
                
                // Store result in wizard data for Step 5
                this.wizard.wizardData.fhirTransformResult = transformResult;
            } else {
                throw new Error(transformResult.error || 'Transformation failed');
            }
            
        } catch (error) {
            this.wizard.hideLoading();
            console.error('❌ FHIR transformation failed:', error);
            this.showNotification(`Transformation failed: ${error.message}`, 'error');
        }
    }

    displayTransformationResult(result) {
        const resultsContainer = document.getElementById('transformation-results');
        if (!resultsContainer) return;
        
        const resourceCount = result.fhirResources ? result.fhirResources.length : 0;
        const bundleResourceCount = result.bundle?.entry ? result.bundle.entry.length : 0;
        const warningCount = result.warnings ? result.warnings.length : 0;
        const errorCount = result.errors ? result.errors.length : 0;
        
        resultsContainer.innerHTML = `
            <div class="transform-results">
                <div class="results-header">
                    <div class="results-title">
                        <h4>✅ FHIR Transformation Complete</h4>
                        <div class="results-subtitle">Your HL7 message has been successfully converted to FHIR R4 format</div>
                    </div>
                    <div class="results-stats">
                        <div class="stat-item">
                            <span class="stat-number">${resourceCount}</span>
                            <span class="stat-label">Resources</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-number">${bundleResourceCount}</span>
                            <span class="stat-label">Bundle Entries</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-number stat-warning">${warningCount}</span>
                            <span class="stat-label">Warnings</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-number stat-error">${errorCount}</span>
                            <span class="stat-label">Errors</span>
                        </div>
                    </div>
                </div>
                
                ${resourceCount > 0 ? `
                    <div class="fhir-resources">
                        <div class="section-header">
                            <h5>📦 Generated FHIR Resources</h5>
                            <button class="toggle-all-btn" onclick="this.classList.toggle('expanded'); window.step4Handler.toggleAllResources(this.classList.contains('expanded'))">
                                <span class="toggle-text">Expand All</span>
                                <span class="toggle-icon">▼</span>
                            </button>
                        </div>
                        <div class="resource-list">
                            ${result.fhirResources.map((resource, index) => `
                                <div class="resource-card" data-index="${index}">
                                    <div class="resource-header" onclick="window.step4Handler.toggleResource(${index})">
                                        <div class="resource-info">
                                            <span class="resource-type">${resource.resourceType}</span>
                                            <span class="resource-id">${resource.id || 'No ID'}</span>
                                        </div>
                                        <div class="resource-meta">
                                            <span class="resource-size">${JSON.stringify(resource).length} chars</span>
                                            <span class="toggle-icon">▼</span>
                                        </div>
                                    </div>
                                    <div class="resource-content">
                                        <div class="resource-preview">
                                            <pre><code class="json">${JSON.stringify(resource, null, 2)}</code></pre>
                                        </div>
                                    </div>
                                </div>
                            `).join('')}
                        </div>
                    </div>
                ` : '<div class="no-resources">No FHIR resources were generated</div>'}
                
                ${result.bundle ? `
                    <div class="bundle-section">
                        <div class="section-header">
                            <h5>📋 FHIR Bundle</h5>
                            <span class="bundle-type">Type: ${result.bundle.type || 'message'}</span>
                        </div>
                        <div class="bundle-info">
                            <div class="bundle-stats">
                                <span class="bundle-stat">ID: ${result.bundle.id || 'Generated'}</span>
                                <span class="bundle-stat">Timestamp: ${result.bundle.timestamp || 'Not set'}</span>
                                <span class="bundle-stat">Entries: ${result.bundle.entry ? result.bundle.entry.length : 0}</span>
                            </div>
                        </div>
                    </div>
                ` : ''}
                
                ${warningCount > 0 ? `
                    <div class="warnings-section">
                        <h5>⚠️ Warnings (${warningCount})</h5>
                        <ul class="warnings-list">
                            ${result.warnings.slice(0, 10).map(warning => `<li>${warning}</li>`).join('')}
                            ${result.warnings.length > 10 ? `<li class="more-items">... and ${result.warnings.length - 10} more warnings</li>` : ''}
                        </ul>
                    </div>
                ` : ''}
                
                ${errorCount > 0 ? `
                    <div class="errors-section">
                        <h5>❌ Errors (${errorCount})</h5>
                        <ul class="errors-list">
                            ${result.errors.slice(0, 10).map(error => `<li>${error}</li>`).join('')}
                            ${result.errors.length > 10 ? `<li class="more-items">... and ${result.errors.length - 10} more errors</li>` : ''}
                        </ul>
                    </div>
                ` : ''}
                
                ${result.mappingStats ? `
                    <div class="mapping-stats">
                        <h5>📊 Mapping Statistics</h5>
                        <div class="stats-grid">
                            <div class="stat-card">
                                <span class="stat-label">Fields Mapped</span>
                                <span class="stat-value">${result.mappingStats.totalFieldsMapped || 0}</span>
                            </div>
                            <div class="stat-card">
                                <span class="stat-label">Data Transforms</span>
                                <span class="stat-value">${result.mappingStats.dataTypeTransforms || 0}</span>
                            </div>
                            <div class="stat-card">
                                <span class="stat-label">Value Sets</span>
                                <span class="stat-value">${result.mappingStats.valueSetTransforms || 0}</span>
                            </div>
                        </div>
                    </div>
                ` : ''}
                
                <div class="results-actions">
                    <button class="action-btn secondary" onclick="window.step4Handler.downloadFHIR('json')">
                        <span class="btn-icon">📥</span>
                        Download FHIR JSON
                    </button>
                    <button class="action-btn secondary" onclick="window.step4Handler.copyToClipboard()">
                        <span class="btn-icon">📋</span>
                        Copy to Clipboard
                    </button>
                    <button class="action-btn primary" onclick="window.step4Handler.validateFHIR()">
                        <span class="btn-icon">✅</span>
                        Validate FHIR
                    </button>
                </div>
            </div>
        `;
        
        resultsContainer.style.display = 'block';
        
        // Scroll to results
        resultsContainer.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    toggleResource(index) {
        const resourceCard = document.querySelector(`[data-index="${index}"]`);
        if (resourceCard) {
            resourceCard.classList.toggle('expanded');
        }
    }

    toggleAllResources(expanded) {
        const resourceCards = document.querySelectorAll('.resource-card');
        const toggleBtn = document.querySelector('.toggle-all-btn');
        
        resourceCards.forEach(card => {
            if (expanded) {
                card.classList.add('expanded');
            } else {
                card.classList.remove('expanded');
            }
        });
        
        if (toggleBtn) {
            const toggleText = toggleBtn.querySelector('.toggle-text');
            if (toggleText) {
                toggleText.textContent = expanded ? 'Collapse All' : 'Expand All';
            }
        }
    }

    downloadFHIR(format = 'json') {
        if (!this.transformationResult) return;
        
        const data = format === 'json' ? 
            JSON.stringify(this.transformationResult.fhirResources || this.transformationResult, null, 2) :
            this.transformationResult;
        
        const blob = new Blob([data], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `fhir-transform-result-${Date.now()}.json`;
        a.click();
        URL.revokeObjectURL(url);
        
        this.showNotification('FHIR data downloaded successfully', 'success');
    }

    async copyToClipboard() {
        if (!this.transformationResult) return;
        
        try {
            const data = JSON.stringify(this.transformationResult.fhirResources || this.transformationResult, null, 2);
            await navigator.clipboard.writeText(data);
            this.showNotification('FHIR data copied to clipboard', 'success');
        } catch (error) {
            console.error('Failed to copy to clipboard:', error);
            this.showNotification('Failed to copy to clipboard', 'error');
        }
    }

    validateFHIR() {
        if (!this.transformationResult) return;
        
        // Simple validation check
        const hasErrors = this.transformationResult.errors && this.transformationResult.errors.length > 0;
        const hasResources = this.transformationResult.fhirResources && this.transformationResult.fhirResources.length > 0;
        
        if (hasResources && !hasErrors) {
            this.showNotification('✅ FHIR resources appear to be valid', 'success');
        } else if (hasErrors) {
            this.showNotification(`⚠️ FHIR validation found ${this.transformationResult.errors.length} issues`, 'warning');
        } else {
            this.showNotification('❌ No FHIR resources to validate', 'error');
        }
    }

    loadErrorState(error) {
        const step4Element = document.getElementById('step4');
        const demoContent = step4Element?.querySelector('.demo-content');
        
        if (demoContent) {
            demoContent.innerHTML = `
                <div class="error-state">
                    <div class="error-icon">⚠️</div>
                    <div class="error-title">FHIR Transform Interface Error</div>
                    <div class="error-message">${error.message}</div>
                    <button class="retry-btn" onclick="window.step4Handler.initialize()">
                        🔄 Retry Initialization
                    </button>
                </div>
            `;
        }
    }

    // BaseStepHandler interface implementation
    validate() {
        if (!this.initialized) {
            this.showNotification('Step 4 not fully initialized', 'error');
            return false;
        }
        
        if (!this.transformationResult) {
            this.showNotification('Please transform the HL7 message to FHIR before proceeding', 'warning');
            return false;
        }
        
        // Check if transformation was successful
        if (this.transformationResult.success === false) {
            this.showNotification('HL7 to FHIR transformation failed. Please fix errors before proceeding.', 'error');
            return false;
        }
        
        return true;
    }

    reset() {
        this.transformationResult = null;
        this.initialized = false;
        
        if (this.wizard.wizardData.fhirTransformResult) {
            delete this.wizard.wizardData.fhirTransformResult;
        }
        
        console.log('✅ Step 4 reset complete');
    }

    setupEventListeners() {
        console.log('✅ FHIR transform step event listeners setup');
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
        const summaryTransform = this.getElement('summaryTransform');
        
        const actualMessageType = this.wizard.wizardData.detectedMessageType || 
                                 this.wizard.wizardData.messageType || 
                                 'Auto-detect';
        
        if (summaryName) summaryName.textContent = this.wizard.wizardData.name || 'New Interface';
        if (summaryType) summaryType.textContent = `${this.wizard.wizardData.sourceType || 'HL7 v2.x'} → ${this.wizard.wizardData.targetType || 'FHIR R4'}`;
        if (summaryMessage) summaryMessage.textContent = actualMessageType;
        if (summarySource) summarySource.textContent = this.wizard.wizardData.sourceType ? this.wizard.wizardData.sourceType.toUpperCase() : 'HL7';
        if (summaryTarget) summaryTarget.textContent = this.wizard.wizardData.targetType ? this.wizard.wizardData.targetType.toUpperCase() : 'FHIR';
        
        if (this.wizard.wizardData.enhancedSegments) {
            const segments = this.wizard.wizardData.enhancedSegments;
            if (summarySegments) summarySegments.textContent = Object.keys(segments).length;
            if (summaryZSegments) summaryZSegments.textContent = Object.keys(segments).filter(seg => seg.startsWith('Z')).length;
        } else if (this.wizard.parsedHL7Data?.data) {
            const data = this.wizard.parsedHL7Data.data;
            if (summarySegments) summarySegments.textContent = Object.keys(data.enhancedSegments || {}).length;
            if (summaryZSegments) summaryZSegments.textContent = Object.keys(data.enhancedSegments || {}).filter(seg => seg.startsWith('Z')).length;
        }
        
        // Show FHIR transformation results
        if (summaryTransform && this.wizard.wizardData.fhirTransformResult) {
            const result = this.wizard.wizardData.fhirTransformResult;
            const resourceCount = result.fhirResources ? result.fhirResources.length : 0;
            summaryTransform.textContent = `${resourceCount} FHIR Resources Generated`;
        } else if (summaryTransform) {
            summaryTransform.textContent = 'Not Transformed';
        }
    }

    setupEventListeners() {
        console.log('✅ Summary step event listeners setup');
    }

    validate() {
        return true;
    }

    reset() {
        // Summary is generated dynamically, no reset needed
    }
}

// Export for module systems and global access
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

// Global availability for direct usage
window.BaseStepHandler = BaseStepHandler;
window.ConfigurationStepHandler = ConfigurationStepHandler;
window.UploadStepHandler = UploadStepHandler;
window.ReviewStepHandler = ReviewStepHandler;
window.MappingStepHandler = MappingStepHandler;
window.SummaryStepHandler = SummaryStepHandler;

console.log('✅ Enhanced Step Handlers loaded with FHIR Transform Integration');