// wizard-main.js - Main Orchestrator for ezHealthKonnect Interface Wizard
// Modular architecture working with actual API response structure

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

        // Initialize modular services
        this.initializeServices();
        this.init();
    }

    initializeServices() {
        // Initialize service modules with actual API structure
        this.notificationService = new NotificationService();
        this.validationService = new ValidationService();
        this.segmentViewer = new SegmentViewer(this);
        this.hl7Service = new HL7Service(this.API_BASE_URL);
        
        // Initialize step handlers
        this.stepHandlers = {
            1: new ConfigurationStepHandler(this),
            2: new UploadStepHandler(this),
            3: new ReviewStepHandler(this),
            4: new MappingStepHandler(this),
            5: new SummaryStepHandler(this)
        };
    }

    init() {
        console.log('🔍 Initializing Enhanced Interface Wizard...');
        
        if (document.readyState !== 'complete') {
            window.addEventListener('load', () => this.init());
            return;
        }
        
        this.setupEventListeners();
        this.updateNavigation();
        console.log('✅ Enhanced Interface Wizard initialized with modular architecture');
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
        }
        return false;
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

    setupModalEventListeners() {
        if (this.modalListenersSetup) {
            return;
        }

        console.log('🔍 Setting up enhanced modal event listeners...');
        
        // Modal control buttons
        this.setupControlButtons();
        
        // Navigation buttons
        this.addEventListenerSafe('nextBtn', 'click', () => this.nextStep());
        this.addEventListenerSafe('prevBtn', 'click', () => this.prevStep());
        this.addEventListenerSafe('finishBtn', 'click', () => this.finishWizard());
        
        // Setup step-specific handlers
        Object.values(this.stepHandlers).forEach(handler => {
            if (handler.setupEventListeners) {
                handler.setupEventListeners();
            }
        });
        
        // Backdrop click handling
        const overlay = this.getElement('wizardModalOverlay');
        if (overlay) {
            overlay.addEventListener('click', (e) => {
                if (e.target === overlay) {
                    this.closeModal();
                }
            });
        }
        
        this.modalListenersSetup = true;
        console.log('✅ Enhanced modal event listeners setup complete');
    }

    setupControlButtons() {
        let retryCount = 0;
        const maxRetries = 10;
        
        const setupButtons = () => {
            retryCount++;
            
            const closeBtn = document.getElementById('wizardModalClose');
            const maximizeBtn = document.getElementById('wizardModalMaximize');
            
            if (closeBtn && maximizeBtn) {
                closeBtn.addEventListener('click', () => this.closeModal());
                maximizeBtn.addEventListener('click', () => this.toggleMaximize());
                return true;
            } else if (retryCount < maxRetries) {
                setTimeout(setupButtons, 100);
                return false;
            } else {
                this.createMissingModalButtons();
                return true;
            }
        };
        
        setupButtons();
    }

    createMissingModalButtons() {
        console.log('🔧 Creating missing modal buttons...');
        
        const modal = this.getElement('wizardModalContainer');
        if (!modal) return;
        
        let controlsContainer = modal.querySelector('.wizard-modal-controls');
        
        if (!controlsContainer) {
            controlsContainer = document.createElement('div');
            controlsContainer.className = 'wizard-modal-controls';
            modal.appendChild(controlsContainer);
        }
        
        // Create maximize button if missing
        if (!document.getElementById('wizardModalMaximize')) {
            const maximizeBtn = document.createElement('button');
            maximizeBtn.id = 'wizardModalMaximize';
            maximizeBtn.className = 'wizard-modal-control maximize-btn';
            maximizeBtn.title = 'Maximize';
            maximizeBtn.innerHTML = '<span id="maximizeIcon">⊞</span>';
            
            maximizeBtn.addEventListener('click', () => this.toggleMaximize());
            controlsContainer.appendChild(maximizeBtn);
        }
        
        // Create close button if missing
        if (!document.getElementById('wizardModalClose')) {
            const closeBtn = document.createElement('button');
            closeBtn.id = 'wizardModalClose';
            closeBtn.className = 'wizard-modal-control close-btn';
            closeBtn.title = 'Close';
            closeBtn.textContent = '×';
            
            closeBtn.addEventListener('click', () => this.closeModal());
            controlsContainer.appendChild(closeBtn);
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
        
        setTimeout(() => {
            this.setupModalEventListeners();
        }, 50);
        
        this.resetWizard();
        console.log('🎭 Enhanced Modal opened');
    }

    closeModal() {
        this.isModalOpen = false;
        const overlay = this.getElement('wizardModalOverlay');
        
        if (overlay) {
            overlay.classList.remove('show');
            overlay.classList.remove('maximized');
            document.body.style.overflow = '';
        }
        
        // Reset maximize button
        const maximizeIcon = this.getElement('maximizeIcon');
        const maximizeBtn = this.getElement('wizardModalMaximize');
        if (maximizeIcon) maximizeIcon.textContent = '⊞';
        if (maximizeBtn) maximizeBtn.title = 'Maximize';
        
        console.log('🎭 Enhanced Modal closed');
    }

    toggleMaximize() {
        const modal = this.getElement('wizardModalOverlay');
        const maximizeIcon = document.getElementById('maximizeIcon');
        const maximizeBtn = document.getElementById('wizardModalMaximize');
        
        if (!modal) return;
        
        if (modal.classList.contains('maximized')) {
            modal.classList.remove('maximized');
            if (maximizeIcon) {
                maximizeIcon.textContent = '⊞';
                maximizeIcon.style.transform = 'rotate(0deg)';
            }
            if (maximizeBtn) maximizeBtn.title = 'Maximize';
            console.log('📱 Modal restored to normal size');
        } else {
            modal.classList.add('maximized');
            if (maximizeIcon) {
                maximizeIcon.textContent = '⊟';
                maximizeIcon.style.transform = 'rotate(180deg)';
            }
            if (maximizeBtn) maximizeBtn.title = 'Restore';
            console.log('🖥️ Modal maximized for enhanced segment viewing');
        }
        
        // Enhanced viewing notification for segment drilling
        if (modal.classList.contains('maximized')) {
            this.notificationService.show('Enhanced viewing mode - Perfect for segment drilling!', 'info');
        }
    }

    resetWizard() {
        this.currentStep = 1;
        this.wizardData = {};
        this.parsedHL7Data = null;
        this.uploadedFile = null;
        
        // Reset each step using handlers
        Object.values(this.stepHandlers).forEach(handler => {
            if (handler.reset) {
                handler.reset();
            }
        });
        
        this.showStep(1);
        this.updateStepIndicators();
        this.updateNavigation();
    }

    // Navigation Methods
    nextStep() {
        const currentHandler = this.stepHandlers[this.currentStep];
        
        if (currentHandler && currentHandler.validate && !currentHandler.validate()) {
            return;
        }
        
        if (this.currentStep < this.totalSteps) {
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
        
        // Initialize step-specific functionality
        const handler = this.stepHandlers[step];
        if (handler && handler.initialize) {
            handler.initialize();
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

    async finishWizard() {
        this.showLoading('Creating interface...', 'Saving configuration to ezHealthKonnect platform');
        
        try {
            const interfaceData = {
                name: this.wizardData.name,
                description: this.wizardData.description || '',
                sourceType: this.wizardData.sourceType,
                targetType: this.wizardData.targetType,
                messageType: this.wizardData.messageType || 'auto-detect',
                sourceConfig: this.stepHandlers[1].collectSourceConfig(),
                targetConfig: this.stepHandlers[1].collectTargetConfig(),
                processingRules: this.generateProcessingRules(),
                transformationMapping: this.generateTransformationMapping()
            };

            const response = await fetch(`${this.API_BASE_URL}/interfaces`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(interfaceData)
            });

            const result = await response.json();

            if (!response.ok || !result.success) {
                throw new Error(result.error || `HTTP ${response.status}`);
            }

            this.hideLoading();
            this.notificationService.show('Interface created successfully!', 'success');
            
            setTimeout(() => {
                this.notificationService.show(
                    `🎉 Interface "${result.interface.name}" ready for testing!`, 
                    'success'
                );
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
            this.hideLoading();
            let errorMessage = 'Failed to create interface. ';
            if (error.message.includes('connection') || error.message.includes('fetch')) {
                errorMessage += 'Could not connect to ezHealthKonnect server.';
            } else {
                errorMessage += error.message;
            }
            this.notificationService.show(errorMessage, 'error');
        }
    }

    generateProcessingRules() {
        const rules = {};
        if (this.parsedHL7Data?.data) {
            const data = this.parsedHL7Data.data;
            rules.messageType = data.messageType?.name;
            rules.expectedSegments = Object.keys(data.enhancedSegments || {});
            rules.customSegments = Object.keys(data.enhancedSegments || {}).filter(seg => seg.startsWith('Z'));
            rules.enhancedParsing = data.dictionaryUsed;
            rules.validationRules = this.validationService.generateValidationRules(data);
        }
        return rules;
    }

    generateTransformationMapping() {
        if (this.stepHandlers[4] && this.stepHandlers[4].generateMapping) {
            return this.stepHandlers[4].generateMapping();
        }
        return {};
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

    delay(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }
}

// Global functions for HTML integration
function openInterfaceWizard() {
    if (window.wizard) {
        window.wizard.openModal();
    } else {
        try {
            window.wizard = new InterfaceWizardModal();
            window.wizard.openModal();
        } catch (error) {
            console.error('❌ Failed to initialize wizard:', error);
            alert('Error: Could not initialize the interface wizard.');
        }
    }
}

function closeWizardModal() {
    if (window.wizard) {
        window.wizard.closeModal();
    }
}

// Initialize wizard when DOM is ready
function initializeWizard() {
    if (window.wizard) return;
    
    try {
        window.wizard = new InterfaceWizardModal();
        console.log('🧙‍♂️ Enhanced Interface Wizard initialized with modular architecture');
    } catch (error) {
        console.error('❌ Failed to initialize wizard:', error);
    }
}

// Multiple initialization strategies
document.addEventListener('DOMContentLoaded', initializeWizard);
window.addEventListener('load', initializeWizard);
setTimeout(() => {
    if (!window.wizard) initializeWizard();
}, 1000);

// Export for module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = InterfaceWizardModal;
}