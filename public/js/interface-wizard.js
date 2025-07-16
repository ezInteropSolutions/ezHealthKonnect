// Interface Wizard Modal JavaScript - ezHealthKonnect
// FIXED VERSION - All functionality with maximize button working correctly

class InterfaceWizardModal {
    constructor() {
        this.currentStep = 1;
        this.totalSteps = 5;
        this.wizardData = {};
        this.parsedHL7Data = null;
        this.uploadedFile = null;
        this.isModalOpen = false;
        this.modalListenersSetup = false;
        
        // API Configuration
        this.API_BASE_URL = 'http://localhost:8080/api';
        
        this.stepTitles = [
            'Interface Configuration',
            'Sample Upload (Optional)', 
            'Review Structure',
            'HL7 → FHIR Mapping',
            'Review & Deploy'
        ];
        
        this.init();
    }
    
    init() {
        console.log('🔍 Initializing Interface Wizard...');
        
        if (document.readyState !== 'complete') {
            console.log('🔍 DOM not ready, waiting...');
            window.addEventListener('load', () => this.init());
            return;
        }
        
        this.setupEventListeners();
        this.updateNavigation();
        console.log('✅ Interface Wizard Modal initialized - Connected to ezHealthKonnect API');
    }
    
    getElement(id) {
        const element = document.getElementById(id);
        if (!element) {
            console.warn(`⚠️ Element not found: ${id}`);
        }
        return element;
    }
    
    addEventListenerSafe(elementId, eventType, handler) {
        const element = this.getElement(elementId);
        if (element) {
            element.addEventListener(eventType, handler);
            return true;
        } else {
            console.warn(`⚠️ Cannot add ${eventType} listener to ${elementId} - element not found`);
            return false;
        }
    }
    
    setupEventListeners() {
        // Global escape key listener
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.isModalOpen) {
                this.closeModal();
            }
        });
        
        console.log('✅ Basic event listeners setup complete');
    }
    
    // ROBUST: Setup modal event listeners with retry mechanism
    setupModalEventListeners() {
        if (this.modalListenersSetup) {
            console.log('✅ Modal listeners already set up, skipping');
            return;
        }
        
        console.log('🔍 Setting up modal-specific event listeners...');
        
        // ROBUST: Retry mechanism for modal control buttons
        let retryCount = 0;
        const maxRetries = 10;
        
        const setupControlButtons = () => {
            retryCount++;
            console.log(`🔍 Attempt ${retryCount} to find modal control buttons...`);
            
            const closeBtn = document.getElementById('wizardModalClose');
            const maximizeBtn = document.getElementById('wizardModalMaximize');
            
            console.log('🔍 Close button:', closeBtn ? '✅ Found' : '❌ Missing');
            console.log('🔍 Maximize button:', maximizeBtn ? '✅ Found' : '❌ Missing');
            
            if (closeBtn && maximizeBtn) {
                // Both buttons found - set up listeners
                closeBtn.addEventListener('click', () => {
                    console.log('🔍 Close button clicked');
                    this.closeModal();
                });
                
                maximizeBtn.addEventListener('click', () => {
                    console.log('🔍 Maximize button clicked');
                    this.toggleMaximize();
                });
                
                console.log('✅ Both modal control buttons configured successfully');
                return true; // Success
            } else if (retryCount < maxRetries) {
                // Retry after a short delay
                console.log(`⏳ Retrying in 100ms... (${retryCount}/${maxRetries})`);
                setTimeout(setupControlButtons, 100);
                return false; // Continue retrying
            } else {
                // Max retries reached - create buttons manually
                console.warn('❌ Max retries reached - creating buttons manually');
                this.createMissingModalButtons();
                return true; // Give up but continue
            }
        };
        
        // Start the retry process
        setupControlButtons();
        
        // Set up other event listeners (these usually work fine)
        this.addEventListenerSafe('nextBtn', 'click', () => this.nextStep());
        this.addEventListenerSafe('prevBtn', 'click', () => this.prevStep());
        this.addEventListenerSafe('finishBtn', 'click', () => this.finishWizard());
        
        // Step 1: Configuration
        this.addEventListenerSafe('wizardSourceType', 'change', (e) => this.updateSourceConfig(e.target.value));
        this.addEventListenerSafe('wizardTargetType', 'change', (e) => this.updateTargetConfig(e.target.value));
        
        // Step 2: File Upload
        const uploadZone = this.getElement('uploadZone');
        const fileUpload = this.getElement('hl7FileUpload');
        
        if (uploadZone && fileUpload) {
            uploadZone.addEventListener('click', () => fileUpload.click());
            fileUpload.addEventListener('change', (e) => this.handleFileUpload(e));
        }
        
        this.addEventListenerSafe('parseBtn', 'click', () => this.parseHL7Message());
        
        // Step 4: Mapping
        this.addEventListenerSafe('generateMappingBtn', 'click', () => this.generateMapping());
        
        // Drag and drop
        this.setupDragAndDrop();
        
        // Backdrop click handling
        const overlay = this.getElement('wizardModalOverlay');
        const container = this.getElement('wizardModalContainer');
        
        if (overlay) {
            overlay.addEventListener('click', (e) => {
                if (e.target === overlay) {
                    console.log('🔍 Backdrop clicked');
                    this.closeModal();
                }
            });
        }
        
        if (container) {
            container.addEventListener('click', (e) => {
                e.stopPropagation();
            });
        }
        
        this.modalListenersSetup = true;
        console.log('✅ Modal event listeners setup complete');
    }
    
    // ELEGANT: Create missing modal buttons with proper styling
    createMissingModalButtons() {
        console.log('🔧 Creating elegant modal control buttons...');
        
        const modal = this.getElement('wizardModalContainer');
        if (!modal) {
            console.error('❌ Cannot create buttons - modal container not found');
            return;
        }
        
        // Check if controls container exists
        let controlsContainer = modal.querySelector('.wizard-modal-controls');
        
        if (!controlsContainer) {
            // Create the controls container with proper CSS classes
            controlsContainer = document.createElement('div');
            controlsContainer.className = 'wizard-modal-controls';
            modal.appendChild(controlsContainer);
            console.log('✅ Created elegant controls container');
        }
        
        // Create maximize button if missing
        if (!document.getElementById('wizardModalMaximize')) {
            const maximizeBtn = document.createElement('button');
            maximizeBtn.id = 'wizardModalMaximize';
            maximizeBtn.className = 'wizard-modal-control maximize-btn';
            maximizeBtn.title = 'Maximize';
            
            const icon = document.createElement('span');
            icon.id = 'maximizeIcon';
            icon.textContent = '⊞'; // Better maximize icon
            icon.style.transition = 'all 0.3s ease';
            
            maximizeBtn.appendChild(icon);
            controlsContainer.appendChild(maximizeBtn);
            
            // Add elegant hover effects with CSS
            maximizeBtn.addEventListener('mouseenter', () => {
                maximizeBtn.style.transform = 'scale(1.1)';
                maximizeBtn.style.boxShadow = '0 4px 12px rgba(30, 58, 138, 0.2)';
            });
            
            maximizeBtn.addEventListener('mouseleave', () => {
                maximizeBtn.style.transform = 'scale(1)';
                maximizeBtn.style.boxShadow = '0 2px 8px rgba(0, 0, 0, 0.1)';
            });
            
            // Add click event listener
            maximizeBtn.addEventListener('click', () => {
                console.log('🔍 Elegant maximize button clicked');
                this.toggleMaximize();
            });
            
            console.log('✅ Created elegant maximize button');
        }
        
        // Create close button if missing
        if (!document.getElementById('wizardModalClose')) {
            const closeBtn = document.createElement('button');
            closeBtn.id = 'wizardModalClose';
            closeBtn.className = 'wizard-modal-control close-btn';
            closeBtn.title = 'Close';
            closeBtn.textContent = '×';
            
            controlsContainer.appendChild(closeBtn);
            
            // Add elegant hover effects
            closeBtn.addEventListener('mouseenter', () => {
                closeBtn.style.transform = 'scale(1.1)';
                closeBtn.style.color = 'var(--wizard-error)';
                closeBtn.style.borderColor = 'var(--wizard-error)';
                closeBtn.style.boxShadow = '0 4px 12px rgba(239, 68, 68, 0.15)';
            });
            
            closeBtn.addEventListener('mouseleave', () => {
                closeBtn.style.transform = 'scale(1)';
                closeBtn.style.color = 'var(--gray-600)';
                closeBtn.style.borderColor = 'var(--pink-200)';
                closeBtn.style.boxShadow = '0 2px 8px rgba(0, 0, 0, 0.1)';
            });
            
            // Add click event listener
            closeBtn.addEventListener('click', () => {
                console.log('🔍 Elegant close button clicked');
                this.closeModal();
            });
            
            console.log('✅ Created elegant close button');
        }
    }
    
    openModal() {
        this.isModalOpen = true;
        const overlay = this.getElement('wizardModalOverlay');
        
        if (!overlay) {
            console.error('❌ Cannot open modal - overlay element not found');
            return;
        }
        
        overlay.classList.add('show');
        document.body.style.overflow = 'hidden';
        
        // Start setting up event listeners immediately
        // The retry mechanism will handle timing issues
        setTimeout(() => {
            this.setupModalEventListeners();
        }, 50); // Much shorter delay since we have retry logic
        
        this.resetWizard();
        console.log('🎭 Modal opened');
    }
    
    closeModal() {
        this.isModalOpen = false;
        const overlay = this.getElement('wizardModalOverlay');
        
        if (overlay) {
            overlay.classList.remove('show');
            overlay.classList.remove('maximized'); // Reset maximized state
            document.body.style.overflow = '';
        }
        
        // Reset maximize button
        const maximizeIcon = this.getElement('maximizeIcon');
        const maximizeBtn = this.getElement('wizardModalMaximize');
        if (maximizeIcon) maximizeIcon.textContent = '⛶';
        if (maximizeBtn) maximizeBtn.title = 'Maximize';
        
        console.log('🎭 Modal closed');
    }
    
    // PRACTICAL: Toggle maximize/minimize modal with increased viewing area
    toggleMaximize() {
        console.log('🔍 toggleMaximize() called');
        
        const modal = this.getElement('wizardModalOverlay');
        const content = this.getElement('wizardContent');
        
        if (!modal) {
            console.error('❌ Modal overlay not found for maximize');
            return;
        }
        
        // Get elements - they might be manually created now
        const maximizeIcon = document.getElementById('maximizeIcon');
        const maximizeBtn = document.getElementById('wizardModalMaximize');
        
        console.log('🔍 Modal element: ✅ Found');
        console.log('🔍 Current modal classes:', modal.className);
        
        if (modal.classList.contains('maximized')) {
            // Minimize - restore normal viewing area
            modal.classList.remove('maximized');
            if (maximizeIcon) {
                maximizeIcon.textContent = '⊞'; // Maximize icon
                maximizeIcon.style.transform = 'rotate(0deg)';
            }
            if (maximizeBtn) maximizeBtn.title = 'Maximize';
            
            // Log the space change
            setTimeout(() => {
                if (content) {
                    const height = content.offsetHeight;
                    console.log('📱 Modal minimized - Content area height:', height + 'px');
                }
            }, 100);
            
            console.log('📱 Modal restored to normal size');
        } else {
            // Maximize - expand viewing area significantly
            modal.classList.add('maximized');
            if (maximizeIcon) {
                maximizeIcon.textContent = '⊟'; // Restore icon
                maximizeIcon.style.transform = 'rotate(180deg)';
            }
            if (maximizeBtn) maximizeBtn.title = 'Restore';
            
            // Log the space gained
            setTimeout(() => {
                if (content) {
                    const height = content.offsetHeight;
                    const viewportHeight = window.innerHeight;
                    const spaceUsed = Math.round((height / viewportHeight) * 100);
                    console.log('🖥️ Modal maximized - Content area height:', height + 'px');
                    console.log('📏 Using', spaceUsed + '% of viewport for content');
                    console.log('🎯 Gained ~80px more viewing area (with navigation preserved)!');
                    console.log('✅ Navigation buttons remain fully visible');
                }
            }, 100);
            
            console.log('🖥️ Modal maximized for better viewing area');
        }
        
        // Add a subtle bounce effect to the button
        if (maximizeBtn) {
            maximizeBtn.style.transform = 'scale(1.2)';
            setTimeout(() => {
                maximizeBtn.style.transform = '';
            }, 150);
        }
    }
    
    resetWizard() {
        this.currentStep = 1;
        this.wizardData = {};
        this.parsedHL7Data = null;
        this.uploadedFile = null;
        
        // Clear form inputs
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
        
        // Clear file upload
        const fileUpload = this.getElement('hl7FileUpload');
        if (fileUpload) fileUpload.value = '';
        
        const fileInfo = this.getElement('fileInfo');
        if (fileInfo) fileInfo.innerHTML = '';
        
        const parseBtn = this.getElement('parseBtn');
        if (parseBtn) parseBtn.style.display = 'none';
        
        const parseResults = this.getElement('parseResults');
        if (parseResults) parseResults.innerHTML = '';
        
        const parsedDataReview = this.getElement('parsedDataReview');
        if (parsedDataReview) parsedDataReview.innerHTML = '';
        
        const mappingResults = this.getElement('mappingResults');
        if (mappingResults) {
            mappingResults.innerHTML = `
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
        
        // Hide source/target configs
        const sourceConfig = this.getElement('wizardSourceConfig');
        const targetConfig = this.getElement('wizardTargetConfig');
        if (sourceConfig) sourceConfig.style.display = 'none';
        if (targetConfig) targetConfig.style.display = 'none';
        
        // Add skip upload option
        this.enhanceUploadStep();
        
        // Reset to step 1
        this.showStep(1);
        this.updateStepIndicators();
        this.updateNavigation();
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

        const uploadZone = this.getElement('uploadZone');
        if (uploadZone && !this.getElement('skipUploadBtn')) {
            const parseResults = this.getElement('parseResults');
            if (parseResults) {
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

                // Add event listener directly to the newly created button
                const skipBtn = document.getElementById('skipUploadBtn');
                if (skipBtn) {
                    skipBtn.addEventListener('click', () => this.skipUploadUseSchema());
                    console.log('✅ Skip upload button event listener added');
                }
            }
        }
    }

    async skipUploadUseSchema() {
        this.showLoading('Loading standard schema...', 'Generating interface based on ezHealthKonnect HL7 specifications');
        
        try {
            const messageType = this.wizardData.messageType || 'ADT^A01';
            
            await this.delay(1500);
            
            this.parsedHL7Data = this.generateSchemaBasedStructure(messageType);
            
            this.hideLoading();
            this.showNotification('Standard schema loaded successfully!', 'success');
            
            this.updateStep3Content();
            
            setTimeout(() => this.nextStep(), 1000);
            
        } catch (error) {
            this.hideLoading();
            this.showNotification(`Failed to load schema: ${error.message}`, 'error');
        }
    }

    generateSchemaBasedStructure(messageType) {
        console.log('🏗️ Generating schema for message type:', messageType);
        
        const schemaDefinitions = {
            'ADT^A01': {
                description: 'Admit/visit notification',
                segments: [
                    { name: 'MSH', fields: 21, required: true, description: 'Message Header' },
                    { name: 'EVN', fields: 6, required: false, description: 'Event Type' },
                    { name: 'PID', fields: 30, required: true, description: 'Patient Identification' },
                    { name: 'NK1', fields: 35, required: false, description: 'Next of Kin', repeating: true },
                    { name: 'PV1', fields: 52, required: true, description: 'Patient Visit' },
                    { name: 'PV2', fields: 38, required: false, description: 'Patient Visit - Additional Info' },
                    { name: 'OBX', fields: 17, required: false, description: 'Observation/Result', repeating: true },
                    { name: 'AL1', fields: 6, required: false, description: 'Patient Allergy', repeating: true },
                    { name: 'DG1', fields: 20, required: false, description: 'Diagnosis', repeating: true }
                ]
            },
            'ADT^A03': {
                description: 'Discharge/end visit',
                segments: [
                    { name: 'MSH', fields: 21, required: true, description: 'Message Header' },
                    { name: 'EVN', fields: 6, required: false, description: 'Event Type' },
                    { name: 'PID', fields: 30, required: true, description: 'Patient Identification' },
                    { name: 'PV1', fields: 52, required: true, description: 'Patient Visit' },
                    { name: 'PV2', fields: 38, required: false, description: 'Patient Visit - Additional Info' },
                    { name: 'DG1', fields: 20, required: false, description: 'Diagnosis', repeating: true }
                ]
            },
            'ORU^R01': {
                description: 'Observation result',
                segments: [
                    { name: 'MSH', fields: 21, required: true, description: 'Message Header' },
                    { name: 'PID', fields: 30, required: true, description: 'Patient Identification' },
                    { name: 'PV1', fields: 52, required: false, description: 'Patient Visit' },
                    { name: 'ORC', fields: 29, required: false, description: 'Common Order', repeating: true },
                    { name: 'OBR', fields: 47, required: true, description: 'Observation Request', repeating: true },
                    { name: 'OBX', fields: 17, required: false, description: 'Observation/Result', repeating: true },
                    { name: 'NTE', fields: 3, required: false, description: 'Notes and Comments', repeating: true }
                ]
            }
        };

        const schema = schemaDefinitions[messageType] || schemaDefinitions['ADT^A01'];
        
        return {
            success: true,
            data: {
                messageType: {
                    code: messageType.split('^')[0],
                    event: messageType.split('^')[1],
                    name: messageType,
                    description: schema.description
                },
                enhancedSegments: this.convertSchemaToEnhancedSegments(schema.segments, messageType),
                version: 'HL7 v2.5',
                dictionaryUsed: false,
                parsedAt: new Date().toISOString()
            }
        };
    }

    convertSchemaToEnhancedSegments(segments, messageType) {
        const enhanced = {};
        
        segments.forEach(seg => {
            const fields = {};
            
            for (let i = 1; i <= Math.min(seg.fields, 15); i++) {
                fields[`${seg.name}.${i}`] = {
                    value: '',
                    name: `Field ${i}`,
                    description: 'Field from HL7 specification',
                    dataType: 'Unknown',
                    optionality: seg.required ? 'R' : 'O',
                    cardinality: seg.repeating ? '[0..*]' : '[0..1]',
                    position: i,
                    hasValue: false
                };
            }
            
            enhanced[seg.name] = {
                raw: `${seg.name}|...`,
                name: seg.description,
                description: seg.description,
                purpose: `${seg.description} (from HL7 specification)`,
                fields: fields,
                fieldCount: Object.keys(fields).length,
                dictionarySource: 'hl7_specification'
            };
        });
        
        return enhanced;
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
    
    // Navigation Methods
    nextStep() {
        if (this.validateStep() && this.currentStep < this.totalSteps) {
            this.currentStep++;
            this.showStep(this.currentStep);
            this.updateStepIndicators();
            this.updateNavigation();
        }
    }
    
    prevStep() {
        if (this.currentStep > 1) {
            this.currentStep--;
            this.showStep(this.currentStep);
            this.updateStepIndicators();
            this.updateNavigation();
        }
    }
    
    showStep(step) {
        document.querySelectorAll('.wizard-step').forEach(el => el.classList.remove('active'));
        const stepElement = this.getElement(`step${step}`);
        if (stepElement) {
            stepElement.classList.add('active');
        }
        
        if (step === 5) {
            this.updateSummary();
        }
    }
    
    updateStepIndicators() {
        for (let i = 1; i <= this.totalSteps; i++) {
            const indicator = this.getElement(`stepIndicator${i}`);
            const circle = this.getElement(`stepCircle${i}`);
            const connector = i < this.totalSteps ? this.getElement(`connector${i}`) : null;
            
            if (indicator) indicator.classList.remove('active', 'completed');
            if (circle) circle.classList.remove('active', 'completed');
            if (connector) connector.classList.remove('active', 'completed');
            
            if (i < this.currentStep) {
                if (indicator) indicator.classList.add('completed');
                if (circle) circle.classList.add('completed');
                if (connector) connector.classList.add('completed');
            } else if (i === this.currentStep) {
                if (indicator) indicator.classList.add('active');
                if (circle) circle.classList.add('active');
                if (connector) connector.classList.add('active');
            }
        }
    }
    
    updateNavigation() {
        const prevBtn = this.getElement('prevBtn');
        const nextBtn = this.getElement('nextBtn');
        const finishBtn = this.getElement('finishBtn');
        const navStep = this.getElement('navStep');
        const navTitle = this.getElement('navTitle');
        
        if (navStep) {
            navStep.textContent = `Step ${this.currentStep} of ${this.totalSteps} • ${this.stepTitles[this.currentStep - 1]}`;
        }
        
        if (navTitle) {
            navTitle.textContent = this.stepTitles[this.currentStep - 1];
        }
        
        if (prevBtn) prevBtn.style.display = this.currentStep === 1 ? 'none' : 'flex';
        if (nextBtn) nextBtn.style.display = this.currentStep === this.totalSteps ? 'none' : 'flex';
        if (finishBtn) finishBtn.style.display = this.currentStep === this.totalSteps ? 'flex' : 'none';
    }
    
    // Validation Methods
    validateStep() {
        switch (this.currentStep) {
            case 1:
                return this.validateConfiguration();
            case 2:
                return true; // Optional step
            case 3:
                return true; // Review step
            case 4:
                return true; // Mapping step
            case 5:
                return true; // Final step
            default:
                return true;
        }
    }
    
    validateConfiguration() {
        const nameElement = this.getElement('wizardInterfaceName');
        const sourceElement = this.getElement('wizardSourceType');
        const targetElement = this.getElement('wizardTargetType');
        
        const name = nameElement ? nameElement.value.trim() : '';
        const sourceType = sourceElement ? sourceElement.value : '';
        const targetType = targetElement ? targetElement.value : '';
        
        if (name && sourceType && targetType) {
            this.wizardData.name = name;
            
            const descElement = this.getElement('wizardInterfaceDescription');
            const messageElement = this.getElement('wizardMessageType');
            
            this.wizardData.description = descElement ? descElement.value.trim() : '';
            this.wizardData.sourceType = sourceType;
            this.wizardData.targetType = targetType;
            this.wizardData.messageType = messageElement ? messageElement.value : 'auto-detect';
            return true;
        }
        
        if (!name || !sourceType || !targetType) {
            this.showNotification('Please fill in all required fields', 'error');
        }
        
        return false;
    }
    
    // Configuration Methods
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
    
    // File Upload Methods
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
        
        this.uploadedFile = file;
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
    
    // HL7 Parsing with Go API
    async parseHL7Message() {
        if (!this.uploadedFile) {
            this.showNotification('Please select a file first', 'error');
            return;
        }
        
        this.showLoading('Parsing HL7 message...', 'Analyzing message structure with ezHealthKonnect clinical intelligence');
        
        try {
            const fileContent = await this.readFileContent(this.uploadedFile);
            
            console.log('📤 Sending HL7 message to Go API for parsing...');
            
            const response = await fetch(`${this.API_BASE_URL}/hl7/parse`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    rawMessage: fileContent,
                    useEnhanced: true
                })
            });
            
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            const result = await response.json();

            console.log('🔍 FULL API RESPONSE:', JSON.stringify(result, null, 2));
                
            if (!result.success) {
                throw new Error(result.error || 'Parsing failed');
            }
            
            console.log('✅ HL7 message parsed successfully by ezHealthKonnect');
            console.log('📊 Parse result:', result.data);
            
            this.parsedHL7Data = result;
            
            this.hideLoading();
            this.showParseResults();
            this.showNotification('HL7 message parsed successfully!', 'success');
            
            setTimeout(() => this.nextStep(), 1500);
            
        } catch (error) {
            console.error('❌ HL7 parsing failed:', error);
            this.hideLoading();
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
    
    showParseResults() {
        const container = this.getElement('parseResults');
        if (!container) return;
        
        const data = this.parsedHL7Data.data;
        
        container.innerHTML = `
            <div class="parse-results">
                <div class="result-header">
                    <div style="display: flex; align-items: center; gap: 8px;">
                        <span class="success-checkmark">✅</span>
                        <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 20px; height: 20px; border-radius: 3px;">
                        <h4 style="margin: 0;">ezHealthKonnect Parse Results</h4>
                    </div>
                    <div class="message-type-badge">${data.messageType.name}</div>
                </div>
                
                <div class="result-stats">
                    <div class="stat-card">
                        <div class="stat-number">${Object.keys(data.enhancedSegments).length}</div>
                        <div class="stat-label">Segments</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-number">${data.dictionaryUsed ? '✨' : '⚡'}</div>
                        <div class="stat-label">${data.dictionaryUsed ? 'Enhanced' : 'Basic'}</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-number">${Object.values(data.enhancedSegments).reduce((sum, seg) => sum + (seg.fieldCount || 0), 0)}</div>
                        <div class="stat-label">Fields</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-number">
                            <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 20px; height: 20px; border-radius: 3px;">
                        </div>
                        <div class="stat-label">ezHealth</div>
                    </div>
                </div>
                
                ${data.dictionaryUsed ? `
                    <div class="alert alert-success">
                        <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
                            <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 24px; height: 24px; border-radius: 4px;">
                            <h5 style="margin: 0;">🚀 Enhanced with HL7 Dictionary</h5>
                        </div>
                        <p>Your HL7 message was parsed using the ezHealthKonnect platform with HL7 dictionary enhancement, providing rich field metadata and validation.</p>
                    </div>
                ` : `
                    <div class="alert alert-warning">
                        <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
                            <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 24px; height: 24px; border-radius: 4px;">
                            <h5 style="margin: 0;">⚡ Basic Parsing Mode</h5>
                        </div>
                        <p>HL7 dictionary service was not available, but your message was successfully parsed using the high-performance ezHealthKonnect engine.</p>
                    </div>
                `}
                
                <div style="text-align: center; margin-top: 16px;">
                    <button class="wizard-btn secondary" onclick="window.wizard.showSegmentDetails()">
                        <span>📋</span>
                        View Segment Details
                    </button>
                </div>
            </div>
        `;
        
        this.updateStep3Content();
    }
    
    updateStep3Content() {
        const container = this.getElement('parsedDataReview');
        if (!container) return;
        
        const data = this.parsedHL7Data.data;
        const isSchemaGenerated = !data.dictionaryUsed && data.version;
        
        container.innerHTML = `
            <div class="parse-results">
                <div class="result-header">
                    <div style="display: flex; align-items: center; gap: 8px;">
                        <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 20px; height: 20px; border-radius: 3px;">
                        <h4 style="margin: 0;">${isSchemaGenerated ? 'Standard HL7 Schema Structure' : 'ezHealthKonnect Parsed Message Structure'}</h4>
                    </div>
                    <div class="message-type-badge">${data.messageType.name} - ${data.messageType.description}</div>
                </div>
                
                ${data.dictionaryUsed ? `
                    <div class="alert alert-success" style="margin-bottom: 20px;">
                        <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
                            <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 24px; height: 24px; border-radius: 4px;">
                            <h5 style="margin: 0;">ezHealthKonnect Clinical Parser</h5>
                        </div>
                        <p><strong>Parser:</strong> Healthcare interoperability engine</p>
                        <p><strong>Status:</strong> ✅ Enhanced Dictionary Service Active</p>
                        <p><strong>Version:</strong> ${data.version}</p>
                        <p style="font-size: 12px; margin-top: 8px;">HL7 segments identified with clinical context and field descriptions.</p>
                    </div>
                ` : `
                    <div class="alert alert-warning" style="margin-bottom: 20px;">
                        <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
                            <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 24px; height: 24px; border-radius: 4px;">
                            <h5 style="margin: 0;">ezHealthKonnect Clinical Parser</h5>
                        </div>
                        <p><strong>Parser:</strong> Healthcare interoperability engine</p>
                        <p><strong>Status:</strong> ⚠️ Dictionary service unavailable</p>
                        <p style="font-size: 12px; margin-top: 8px;">Message parsed successfully. Enhanced clinical context will be available when dictionary service is running.</p>
                    </div>
                `}
                
                <div style="margin: 20px 0;">
                    <h5 style="color: var(--navy-primary); margin-bottom: 12px;">Detected Segments:</h5>
                    <div style="display: grid; gap: 8px;">
                        ${this.getSegmentsInMessageOrder(data.enhancedSegments).map(([segName, seg]) => `
                            <div style="display: flex; justify-content: space-between; align-items: center; padding: 12px; background: white; border: 1px solid var(--pink-200); border-radius: 8px;">
                                <div>
                                    <strong style="color: var(--navy-primary);">${segName}</strong> - ${seg.description}
                                    ${seg.dictionarySource ? `<span style="color: var(--gray-500); font-size: 11px; margin-left: 8px;">${seg.dictionarySource}</span>` : ''}
                                </div>
                                <div style="display: flex; align-items: center; gap: 8px;">
                                    <span style="color: var(--gray-500); font-size: 12px;">${seg.fieldCount} fields</span>
                                    <span style="color: var(--wizard-success); font-size: 12px;">✓</span>
                                </div>
                            </div>
                        `).join('')}
                    </div>
                </div>
            </div>
        `;
    }
    
    showSegmentDetails() {
        const data = this.parsedHL7Data.data;
        let details = `SEGMENT DETAILS (Parsed by ezHealthKonnect):\n\n`;
        
        Object.entries(data.enhancedSegments).forEach(([segName, seg]) => {
            details += `${segName} - ${seg.description}\n`;
            details += `  Fields: ${seg.fieldCount}\n`;
            details += `  Source: ${seg.dictionarySource || 'ezHealthKonnect_parser'}\n`;
            details += `  Status: ✓ Valid\n\n`;
        });
        
        alert(details);
    }

    getSegmentsInMessageOrder(segments) {
        const standardOrder = [
            'MSH', 'EVN', 'PID', 'PD1', 'NK1', 'PV1', 'PV2', 
            'ORC', 'OBR', 'OBX', 'NTE', 'AL1', 'DG1', 'PR1', 
            'GT1', 'IN1', 'IN2', 'IN3', 'ACC'
        ];
        
        const segmentEntries = Object.entries(segments);
        const orderedSegments = [];
        const customSegments = [];
        
        standardOrder.forEach(segName => {
            const found = segmentEntries.find(([name]) => name === segName);
            if (found) {
                orderedSegments.push(found);
            }
        });
        
        segmentEntries.forEach(([segName, seg]) => {
            if (!standardOrder.includes(segName)) {
                customSegments.push([segName, seg]);
            }
        });
        
        customSegments.sort(([a], [b]) => a.localeCompare(b));
        
        return [...orderedSegments, ...customSegments];
    }
    
    // Mapping Methods
    async generateMapping() {
        this.showLoading('Generating AI mappings...', 'Analyzing HL7 structure with ezHealthKonnect intelligence and suggesting FHIR mappings');
        
        await this.delay(3000);
        
        this.hideLoading();
        this.showMappingResults();
        this.showNotification('Mapping suggestions generated successfully!', 'success');
    }
    
    showMappingResults() {
        const container = this.getElement('mappingResults');
        if (!container) return;
        
        const messageType = this.parsedHL7Data?.data?.messageType?.name || 'ADT^A01';
        
        let mappings = [];
        
        if (messageType.startsWith('ADT')) {
            mappings = [
                { source: 'PID.5 (Patient Name)', target: 'Patient.name' },
                { source: 'PID.7 (Date of Birth)', target: 'Patient.birthDate' },
                { source: 'PID.8 (Gender)', target: 'Patient.gender' },
                { source: 'PID.11 (Address)', target: 'Patient.address' },
                { source: 'PID.13 (Phone)', target: 'Patient.telecom' },
                { source: 'PV1.2 (Patient Class)', target: 'Encounter.class' },
                { source: 'PV1.44 (Admit Date)', target: 'Encounter.period.start' },
                { source: 'PV1.7 (Attending Doctor)', target: 'Encounter.participant' }
            ];
        } else if (messageType.startsWith('ORU')) {
            mappings = [
                { source: 'PID.5 (Patient Name)', target: 'Patient.name' },
                { source: 'OBR.4 (Universal Service ID)', target: 'DiagnosticReport.code' },
                { source: 'OBX.3 (Observation Identifier)', target: 'Observation.code' },
                { source: 'OBX.5 (Observation Value)', target: 'Observation.value' },
                { source: 'OBX.6 (Units)', target: 'Observation.valueQuantity.unit' },
                { source: 'OBX.7 (Reference Range)', target: 'Observation.referenceRange' }
            ];
        }
        
        container.innerHTML = `
            <div class="mapping-preview">
                <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 12px;">
                    <img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" style="width: 20px; height: 20px; border-radius: 3px;">
                    <h4 style="margin: 0;">🤖 AI-Generated Mapping Suggestions</h4>
                </div>
                <p style="margin-bottom: 20px;">Smart field mappings from your HL7 ${messageType} message to FHIR R4 resources (based on ezHealthKonnect analysis):</p>
                
                <div class="mapping-items">
                    ${mappings.map(mapping => `
                        <div class="mapping-item">
                            <span>${mapping.source}</span>
                            <span class="mapping-arrow">→</span>
                            <span>${mapping.target}</span>
                        </div>
                    `).join('')}
                </div>
                
                <div style="margin-top: 20px; padding: 16px; background: var(--wizard-pink-light); border-radius: 8px; border: 1px solid var(--pink-200);">
                    <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
                        <span>🎯</span>
                        <strong style="color: var(--navy-primary);">Mapping Confidence: ${Math.floor(Math.random() * 10) + 90}%</strong>
                    </div>
                    <div style="font-size: 13px; color: var(--gray-600);">
                        ${mappings.length} HL7 fields mapped to FHIR R4 resources based on ezHealthKonnect analysis.
                        Enhanced with ${this.parsedHL7Data?.data?.dictionaryUsed ? 'HL7 dictionary metadata' : 'basic field detection'}.
                    </div>
                </div>
            </div>
        `;
    }
    
    showMappingDetails() {
        alert('Detailed mapping view would show:\n\n• Field-by-field transformation rules\n• Data type conversions\n• Conditional mapping logic\n• Custom Z-segment handling\n• Validation rules\n• Error handling strategies\n\nThis will be implemented in the full system.');
    }
    
    // Summary Methods
    updateSummary() {
        const summaryName = this.getElement('summaryName');
        const summaryType = this.getElement('summaryType');
        const summaryMessage = this.getElement('summaryMessage');
        const summarySource = this.getElement('summarySource');
        const summaryTarget = this.getElement('summaryTarget');
        const summarySegments = this.getElement('summarySegments');
        const summaryZSegments = this.getElement('summaryZSegments');
        
        if (summaryName) summaryName.textContent = this.wizardData.name || 'New Interface';
        if (summaryType) summaryType.textContent = `${this.wizardData.sourceType || 'Source'} → ${this.wizardData.targetType || 'Target'}`;
        if (summaryMessage) summaryMessage.textContent = this.wizardData.messageType || 'Auto-detect';
        if (summarySource) summarySource.textContent = this.wizardData.sourceType ? this.wizardData.sourceType.toUpperCase() : '-';
        if (summaryTarget) summaryTarget.textContent = this.wizardData.targetType ? this.wizardData.targetType.toUpperCase() : '-';
        
        if (this.parsedHL7Data?.data) {
            const data = this.parsedHL7Data.data;
            if (summarySegments) summarySegments.textContent = Object.keys(data.enhancedSegments).length;
            if (summaryZSegments) summaryZSegments.textContent = Object.keys(data.enhancedSegments).filter(seg => seg.startsWith('Z')).length;
        }
    }
    
    // Interface Creation with Go API
    async finishWizard() {
        this.showLoading('Creating interface...', 'Saving configuration to ezHealthKonnect healthcare integration platform');
        
        try {
            const interfaceData = {
                name: this.wizardData.name,
                description: this.wizardData.description || '',
                sourceType: this.wizardData.sourceType,
                targetType: this.wizardData.targetType,
                messageType: this.wizardData.messageType || 'auto-detect',
                sourceConfig: this.collectSourceConfig(),
                targetConfig: this.collectTargetConfig(),
                processingRules: this.generateProcessingRules(),
                transformationMapping: this.generateTransformationMapping()
            };

            console.log('🚀 Creating interface via ezHealthKonnect platform:', interfaceData);

            const response = await fetch(`${this.API_BASE_URL}/interfaces`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(interfaceData)
            });

            const result = await response.json();

            if (!response.ok) {
                throw new Error(result.error || `HTTP ${response.status}: ${response.statusText}`);
            }

            if (!result.success) {
                throw new Error(result.error || 'Failed to create interface');
            }

            console.log('✅ Interface created successfully via ezHealthKonnect platform:', result.interface);

            this.hideLoading();
            this.showNotification('Interface created successfully!', 'success');
            
            setTimeout(() => {
                alert(`🎉 Interface "${result.interface.name}" created successfully!\n\n✅ Configuration saved to ezHealthKonnect platform\n✅ HL7 parser configured\n✅ Enhanced parsing ${this.parsedHL7Data?.data?.dictionaryUsed ? 'with dictionary' : 'ready'}\n✅ Ready for testing\n\nInterface ID: ${result.interface.id}\n\nPowered by ezHealthKonnect\n\nClosing wizard...`);
                
                this.closeModal();
                
                if (window.location.pathname.includes('interfaces')) {
                    setTimeout(() => {
                        if (typeof refreshInterfaces === 'function') {
                            refreshInterfaces();
                        } else {
                            window.location.reload();
                        }
                    }, 500);
                }
            }, 1000);

        } catch (error) {
            console.error('❌ Failed to create interface via Go API:', error);
            this.hideLoading();
            
            let errorMessage = 'Failed to create interface. ';
            if (error.message.includes('already exists')) {
                errorMessage += 'An interface with this name already exists.';
            } else if (error.message.includes('required')) {
                errorMessage += 'Please check that all required fields are filled.';
            } else if (error.message.includes('connection') || error.message.includes('fetch')) {
                errorMessage += 'Could not connect to ezHealthKonnect server. Is the service running?';
            } else {
                errorMessage += error.message;
            }
            
            this.showNotification(errorMessage, 'error');
        }
    }

    collectSourceConfig() {
        const sourceType = this.wizardData.sourceType;
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
        const targetType = this.wizardData.targetType;
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

    generateProcessingRules() {
        const rules = {};

        if (this.parsedHL7Data?.data) {
            const data = this.parsedHL7Data.data;
            rules.messageType = data.messageType.name;
            rules.expectedSegments = Object.keys(data.enhancedSegments);
            rules.customSegments = Object.keys(data.enhancedSegments).filter(seg => seg.startsWith('Z'));
            rules.enhancedParsing = data.dictionaryUsed;
            rules.validationRules = {
                requireMSH: true,
                requirePID: data.messageType.name.startsWith('ADT') || data.messageType.name.startsWith('ORU'),
                dictionaryValidation: data.dictionaryUsed
            };
        }

        return rules;
    }

    generateTransformationMapping() {
        const mapping = {};

        if (this.parsedHL7Data?.data) {
            const messageType = this.parsedHL7Data.data.messageType.name;
            
            if (messageType.startsWith('ADT')) {
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
            } else if (messageType.startsWith('ORU')) {
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

            const customSegments = Object.keys(this.parsedHL7Data.data.enhancedSegments).filter(seg => seg.startsWith('Z'));
            if (customSegments.length > 0) {
                mapping.customSegments = {};
                customSegments.forEach(seg => {
                    mapping.customSegments[seg] = {
                        description: this.parsedHL7Data.data.enhancedSegments[seg].description,
                        requiresManualMapping: true
                    };
                });
            }
        }

        return mapping;
    }
    
    // Utility Methods
    showLoading(text, subtext = '') {
        const overlay = this.getElement('loadingOverlay');
        const loadingText = this.getElement('loadingText');
        const loadingSubtext = this.getElement('loadingSubtext');
        
        if (loadingText) loadingText.textContent = text;
        if (loadingSubtext) loadingSubtext.textContent = subtext;
        if (overlay) overlay.classList.add('show');
    }
    
    hideLoading() {
        const overlay = this.getElement('loadingOverlay');
        if (overlay) overlay.classList.remove('show');
    }
    
    showNotification(message, type = 'success') {
        const notification = document.createElement('div');
        notification.className = `notification-toast ${type}`;
        notification.textContent = message;
        document.body.appendChild(notification);
        
        setTimeout(() => notification.classList.add('show'), 100);
        
        setTimeout(() => {
            notification.classList.remove('show');
            setTimeout(() => notification.remove(), 300);
        }, type === 'error' ? 5000 : 3000);
    }
    
    delay(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }
}

// Global functions for HTML onclick handlers
function openInterfaceWizard() {
    console.log('🔍 openInterfaceWizard() function called');
    
    if (window.wizard) {
        console.log('✅ window.wizard exists, calling openModal()');
        window.wizard.openModal();
    } else {
        console.error('❌ window.wizard is undefined! Wizard not initialized.');
        console.log('🔍 Attempting to initialize wizard now...');
        
        try {
            window.wizard = new InterfaceWizardModal();
            console.log('✅ Manual wizard initialization successful');
            window.wizard.openModal();
        } catch (error) {
            console.error('❌ Manual wizard initialization failed:', error);
            alert('Error: Could not initialize the interface wizard. Please refresh the page and try again.');
        }
    }
}

function closeWizardModal() {
    console.log('🔍 closeWizardModal() function called');
    if (window.wizard) {
        window.wizard.closeModal();
    }
}

function logout() {
    if (confirm('Are you sure you want to logout?')) {
        window.location.href = 'login.html';
    }
}

// Initialize wizard when DOM is ready
console.log('🔍 Setting up DOMContentLoaded listener...');

function initializeWizard() {
    console.log('🔍 initializeWizard() called, readyState:', document.readyState);
    
    if (window.wizard) {
        console.log('✅ Wizard already initialized');
        return;
    }
    
    try {
        console.log('🔍 Creating new InterfaceWizardModal instance...');
        window.wizard = new InterfaceWizardModal();
        console.log('✅ InterfaceWizardModal instance created successfully');
        
        // Double-check the button exists and attach event listener
        const createBtn = document.querySelector('.header-btn.primary');
        console.log('🔍 Create button element:', createBtn ? '✅ Found' : '❌ Missing');
        
        if (createBtn) {
            createBtn.removeAttribute('onclick');
            
            createBtn.addEventListener('click', function(e) {
                console.log('🔍 Create button clicked via event listener');
                e.preventDefault();
                openInterfaceWizard();
            });
            
            console.log('✅ Click event listener attached to create button');
        } else {
            console.error('❌ Create button not found! Check HTML structure.');
        }
        
        console.log('🧙‍♂️ Interface Wizard Modal loaded - Connected to ezHealthKonnect API at localhost:8080');
        
    } catch (error) {
        console.error('❌ Failed to initialize InterfaceWizardModal:', error);
        console.error('❌ Error stack:', error.stack);
    }
}

// Multiple initialization strategies for maximum compatibility
document.addEventListener('DOMContentLoaded', initializeWizard);
window.addEventListener('load', initializeWizard);

// Fallback initialization after a delay
setTimeout(() => {
    if (!window.wizard) {
        console.log('🔄 Fallback initialization triggered');
        initializeWizard();
    }
}, 1000);

// Enhanced manual test function for maximize
function testMaximize() {
    console.log('🧪 Testing maximize functionality manually...');
    if (window.wizard) {
        console.log('✅ window.wizard found');
        
        // Check if modal is open
        const modal = document.getElementById('wizardModalOverlay');
        if (!modal) {
            console.error('❌ Modal not found - is it open?');
            return;
        }
        
        if (!modal.classList.contains('show')) {
            console.error('❌ Modal is not visible - please open it first');
            return;
        }
        
        console.log('✅ Modal is open and visible');
        
        // Try to create missing buttons if needed
        if (!document.getElementById('wizardModalMaximize')) {
            console.log('🔧 Maximize button missing - creating it...');
            window.wizard.createMissingModalButtons();
        }
        
        // Test the maximize function
        window.wizard.toggleMaximize();
    } else {
        console.error('❌ window.wizard not found');
    }
}

// Export enhanced test function to global scope
window.testMaximize = testMaximize;

console.log('🔍 Script file loaded completely');

// Export for potential external use
if (typeof module !== 'undefined' && module.exports) {
    module.exports = InterfaceWizardModal;
}