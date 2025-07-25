// wizard-main.js - Protected version against duplicate declarations

// Wrap everything in an IIFE to allow early returns
(function() {
    // Prevent multiple script loads and class redeclaration
    if (typeof window.InterfaceWizardModal !== 'undefined') {
        console.warn('⚠️ InterfaceWizardModal already exists, skipping redeclaration');
        return; // Now this return is valid inside the function
    }

    // Check if this script has already been loaded
    if (document.querySelector('script[data-wizard-main-loaded="true"]')) {
        console.warn('⚠️ wizard-main.js already loaded, skipping');
        return; // Now this return is valid inside the function
    }

    // Mark this script as loaded
    const currentScript = document.currentScript || document.querySelector('script[src*="wizard-main.js"]');
    if (currentScript) {
        currentScript.setAttribute('data-wizard-main-loaded', 'true');
    }

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
        // Initialize service modules - always create them directly
        try {
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
            
            console.log('✅ Services initialized:', Object.keys(this.stepHandlers));
        } catch (error) {
            console.error('❌ Error initializing services:', error);
            // Fallback: create simple notification service
            this.notificationService = {
                show: (message, type) => {
                    console.log(`${type.toUpperCase()}: ${message}`);
                    alert(`${type.toUpperCase()}: ${message}`);
                }
            };
        }
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
            if (handler && handler.setupEventListeners) {
                try {
                    handler.setupEventListeners();
                } catch (error) {
                    console.error('❌ Error setting up handler listeners:', error);
                }
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
                return true;
            } else if (retryCount < maxRetries) {
                // Retry after a short delay
                console.log(`⏳ Retrying in 100ms... (${retryCount}/${maxRetries})`);
                setTimeout(setupButtons, 100);
                return false;
            } else {
                // Max retries reached - create buttons manually
                console.warn('❌ Max retries reached - creating buttons manually');
                this.createMissingModalButtons();
                return true;
            }
        };
        
        setupButtons();
    }

    createMissingModalButtons() {
        console.log('🔧 Creating missing modal control buttons...');
        
        const modal = this.getElement('wizardModalContainer');
        if (!modal) {
            console.error('❌ Cannot create buttons - modal container not found');
            return;
        }
        
        // Check if controls container exists
        let controlsContainer = modal.querySelector('.wizard-modal-controls');
        
        if (!controlsContainer) {
            controlsContainer = document.createElement('div');
            controlsContainer.className = 'wizard-modal-controls';
            controlsContainer.style.cssText = `
                position: absolute;
                top: 16px;
                right: 16px;
                display: flex;
                gap: 8px;
                z-index: 10;
            `;
            modal.appendChild(controlsContainer);
        }
        
        // Create maximize button if missing
        if (!document.getElementById('wizardModalMaximize')) {
            const maximizeBtn = document.createElement('button');
            maximizeBtn.id = 'wizardModalMaximize';
            maximizeBtn.innerHTML = '<span id="maximizeIcon">⛶</span>';
            maximizeBtn.style.cssText = `
                background: white;
                border: 1px solid #ddd;
                border-radius: 4px;
                width: 32px;
                height: 32px;
                cursor: pointer;
                display: flex;
                align-items: center;
                justify-content: center;
            `;
            controlsContainer.appendChild(maximizeBtn);
            
            maximizeBtn.addEventListener('click', () => this.toggleMaximize());
        }
        
        // Create close button if missing
        if (!document.getElementById('wizardModalClose')) {
            const closeBtn = document.createElement('button');
            closeBtn.id = 'wizardModalClose';
            closeBtn.innerHTML = '×';
            closeBtn.style.cssText = `
                background: white;
                border: 1px solid #ddd;
                border-radius: 4px;
                width: 32px;
                height: 32px;
                cursor: pointer;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 18px;
            `;
            controlsContainer.appendChild(closeBtn);
            
            closeBtn.addEventListener('click', () => this.closeModal());
        }
        
        console.log('✅ Missing modal buttons created');
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
        
        // Setup event listeners with proper timing
        setTimeout(() => {
            this.setupModalEventListeners();
        }, 200);
        
        this.resetWizard();
        console.log('🎭 Modal opened');
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
        if (maximizeIcon) maximizeIcon.textContent = '⛶';
        if (maximizeBtn) maximizeBtn.title = 'Maximize';
        
        console.log('🎭 Modal closed');
    }
    
    toggleMaximize() {
        console.log('🔍 toggleMaximize() called');
        
        const modal = this.getElement('wizardModalOverlay');
        const maximizeIcon = this.getElement('maximizeIcon');
        const maximizeBtn = this.getElement('wizardModalMaximize');
        
        if (!modal) {
            console.error('❌ Modal overlay not found');
            return;
        }
        
        const isMaximized = modal.classList.contains('maximized');
        
        if (isMaximized) {
            // Restore to normal size
            modal.classList.remove('maximized');
            if (maximizeIcon) maximizeIcon.textContent = '⛶';
            if (maximizeBtn) maximizeBtn.title = 'Maximize';
            console.log('📱 Modal restored to normal size');
        } else {
            // Maximize the modal
            modal.classList.add('maximized');
            if (maximizeIcon) maximizeIcon.textContent = '🗗';
            if (maximizeBtn) maximizeBtn.title = 'Restore';
            console.log('🖥️ Modal maximized');
        }
    }

    resetWizard() {
        this.currentStep = 1;
        this.wizardData = {};
        this.parsedHL7Data = null;
        this.uploadedFile = null;
        
        // Reset each step using handlers
        Object.values(this.stepHandlers).forEach(handler => {
            if (handler && handler.reset) {
                try {
                    handler.reset();
                } catch (error) {
                    console.error('❌ Error resetting handler:', error);
                }
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
            try {
                handler.initialize();
            } catch (error) {
                console.error(`❌ Error initializing step ${step}:`, error);
            }
        }
    }

    updateStepIndicators() {
        for (let i = 1; i <= this.totalSteps; i++) {
            const indicator = this.getElement(`stepIndicator${i}`);
            const circle = this.getElement(`stepCircle${i}`);
            
            if (indicator) {
                indicator.classList.toggle('active', i === this.currentStep);
                indicator.classList.toggle('completed', i < this.currentStep);
            }
            
            if (circle) {
                if (i < this.currentStep) {
                    circle.textContent = '✓';
                } else {
                    circle.textContent = i;
                }
            }
        }
    }

    updateNavigation() {
        const prevBtn = this.getElement('prevBtn');
        const nextBtn = this.getElement('nextBtn');
        const finishBtn = this.getElement('finishBtn');
        const navStep = this.getElement('navStep');
        const navTitle = this.getElement('navTitle');
        
        if (prevBtn) prevBtn.style.display = this.currentStep === 1 ? 'none' : 'flex';
        if (nextBtn) nextBtn.style.display = this.currentStep === this.totalSteps ? 'none' : 'flex';
        if (finishBtn) finishBtn.style.display = this.currentStep === this.totalSteps ? 'flex' : 'none';
        
        if (navStep) navStep.textContent = `Step ${this.currentStep} of ${this.totalSteps}`;
        if (navTitle) navTitle.textContent = this.stepTitles[this.currentStep - 1] || 'Wizard Step';
    }

    async finishWizard() {
        console.log('🏁 Finishing wizard...');
        
        try {
            this.showLoading('Creating interface...', 'Please wait while we set up your HL7 interface');
            
            // Simulate API call
            await this.delay(2000);
            
            this.hideLoading();
            this.showNotification('🎉 Interface created successfully!', 'success');
            
            // Close modal after success
            setTimeout(() => {
                this.closeModal();
                
                // Refresh interface table if function exists
                if (typeof loadInterfaces === 'function') {
                    loadInterfaces();
                }
            }, 1500);
            
        } catch (error) {
            this.hideLoading();
            console.error('❌ Failed to create interface:', error);
            this.showNotification('Failed to create interface. Please try again.', 'error');
        }
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
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: ${type === 'error' ? '#dc3545' : type === 'warning' ? '#ffc107' : '#28a745'};
            color: white;
            padding: 12px 16px;
            border-radius: 4px;
            font-weight: 500;
            z-index: 10000;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            max-width: 300px;
        `;
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

    // Make class available globally but protect against redeclaration
    window.InterfaceWizardModal = InterfaceWizardModal;

    // Global functions for HTML integration
    window.openInterfaceWizard = function() {
        if (window.wizard) {
            window.wizard.openModal();
        } else {
            try {
                window.wizard = new InterfaceWizardModal();
                window.wizard.openModal();
            } catch (error) {
                console.error('❌ Failed to initialize wizard:', error);
                alert('Error: Could not initialize the interface wizard. Please refresh the page and try again.');
            }
        }
    };

    window.closeWizardModal = function() {
        if (window.wizard) {
            window.wizard.closeModal();
        }
    };

    // Safe initialization function
    function initializeWizard() {
        if (window.wizard) {
            console.log('⚠️ Wizard already initialized, skipping');
            return;
        }
        
        try {
            window.wizard = new InterfaceWizardModal();
            console.log('🧙‍♂️ Enhanced Interface Wizard initialized with modular architecture');
        } catch (error) {
            console.error('❌ Failed to initialize wizard:', error);
        }
    }

    // Safer initialization strategy - only one method
    if (document.readyState === 'complete') {
        initializeWizard();
    } else {
        document.addEventListener('DOMContentLoaded', initializeWizard);
    }

    // Export for module systems
    if (typeof module !== 'undefined' && module.exports) {
        module.exports = InterfaceWizardModal;
    }

    console.log('✅ Protected wizard-main.js loaded successfully');

})(); // Close the IIFE