// js/wizard/wizard-main.js - FIXED - Proper integration with wizard-navigation.js

// Wrap everything in an IIFE to allow early returns
(function() {
    'use strict';
    
    // Prevent multiple script loads and class redeclaration
    if (typeof window.InterfaceWizardModal !== 'undefined') {
        console.warn('InterfaceWizardModal already exists, skipping redeclaration');
        return;
    }

    // Check if this script has already been loaded
    if (document.querySelector('script[data-wizard-main-loaded="true"]')) {
        console.warn('wizard-main.js already loaded, skipping');
        return;
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

        async initializeServices() {
            try {
                console.log('Initializing services...');
                
                // Initialize core services first
                this.validationService = window.ValidationService ? new ValidationService() : null;
                this.segmentViewer = window.SegmentViewer ? new SegmentViewer(this) : null;
                this.hl7Service = window.HL7Service ? new HL7Service(this.API_BASE_URL) : null;

                // Initialize step handlers with proper error handling
                this.stepHandlers = {};
                
                // Steps 1-3: Standard handlers (with fallback)
                try {
                    this.stepHandlers[1] = window.ConfigurationStepHandler ? new ConfigurationStepHandler(this) : null;
                    this.stepHandlers[2] = window.UploadStepHandler ? new UploadStepHandler(this) : null;
                    this.stepHandlers[3] = window.ReviewStepHandler ? new ReviewStepHandler(this) : null;
                } catch (error) {
                    console.warn('Some standard step handlers not available:', error);
                }

                // Step 4: FHIR Mapping Handler with enhanced error handling and waiting
                await this.initializeStep4Handler();

                // Step 5: Summary Handler
                try {
                    this.stepHandlers[5] = window.SummaryStepHandler ? new SummaryStepHandler(this) : null;
                } catch (error) {
                    console.warn('Summary step handler not available:', error);
                }
                
                // Log successful handlers
                const loadedHandlers = Object.keys(this.stepHandlers).filter(key => this.stepHandlers[key] !== null);
                console.log('Services initialized. Loaded handlers:', loadedHandlers);
                console.log('Step 4 handler type:', this.stepHandlers[4]?.constructor.name || 'None');
                
            } catch (error) {
                console.error('Error initializing services:', error);
                
                // Create fallback notification service
                this.notificationService = {
                    show: (message, type) => {
                        console.log(`${type.toUpperCase()}: ${message}`);
                        this.showNotification(message, type);
                    }
                };
            }
        }

        async initializeStep4Handler() {
            try {
                console.log('Initializing Step 4 handler...');
                
                // FIXED: Load Step 4 modules using the correct module loader method
                if (!window.Step4ModulesLoaded) {
                    console.log('Loading Step 4 modules...');
                    await this.loadStep4Modules();
                    window.Step4ModulesLoaded = true;
                    console.log('Step 4 modules loaded successfully');
                }
                
                // Wait for FHIRMappingStepHandler to be available
                await this.waitForFHIRMappingStepHandler();
                
                // Create Step 4 handler after modules are loaded
                if (typeof window.FHIRMappingStepHandler !== 'undefined') {
                    console.log('FHIRMappingStepHandler found, creating instance...');
                    this.stepHandlers[4] = new FHIRMappingStepHandler(this);
                    console.log('Step 4 handler created:', this.stepHandlers[4].constructor.name);
                } else {
                    throw new Error('FHIRMappingStepHandler not available after loading modules');
                }
                
            } catch (error) {
                console.error('Error initializing Step 4 handler:', error);
                this.createFallbackStep4Handler();
            }
        }

        async loadStep4Modules() {
            try {
                // FIXED: Get the module loader instance properly
                const loader = this.getModuleLoader();
                
                if (loader && typeof loader.loadStep4 === 'function') {
                    console.log('Using existing module loader for Step 4...');
                    await loader.loadStep4();
                } else {
                    console.log('Creating temporary module loader for Step 4...');
                    await this.loadStep4ModulesManually();
                }
                
            } catch (error) {
                console.error('Failed to load Step 4 modules:', error);
                throw error;
            }
        }

        getModuleLoader() {
            // FIXED: Multiple ways to access the module loader
            return window.wizardLoader || 
                   window.wizardAutoLoader?.moduleLoader || 
                   window.moduleLoader ||
                   null;
        }

        async loadStep4ModulesManually() {
            // FIXED: Use the same module list as module-loader.js
            const step4Path = '/js/step4/';
            const modules = [
                'step4-utils.js',
                'step4-styles.js',
                'step4-templates.js',
                'step4-validation.js',
                'step4-mapping.js',
                'step4-resources.js',
                'step4-json-viewer.js',
                'step4-config-manager.js',
                'step4-modals.js',
                'step4-handler.js'  // Main handler loads last
            ];
            
            for (const module of modules) {
                try {
                    await this.loadScriptHelper(step4Path + module);
                    console.log(`Loaded: ${module}`);
                } catch (error) {
                    console.warn(`Failed to load ${module}:`, error);
                    // Continue loading other modules
                }
            }
        }

        loadScriptHelper(src) {
            return new Promise((resolve, reject) => {
                // Check if already loaded
                if (document.querySelector(`script[src="${src}"]`)) {
                    console.log(`Script already loaded: ${src}`);
                    resolve();
                    return;
                }
                
                const script = document.createElement('script');
                script.src = src;
                script.onload = () => {
                    console.log(`Script loaded: ${src}`);
                    resolve();
                };
                script.onerror = () => {
                    console.error(`Failed to load: ${src}`);
                    reject(new Error(`Failed to load ${src}`));
                };
                document.head.appendChild(script);
            });
        }

        async waitForFHIRMappingStepHandler() {
            // Wait for FHIRMappingStepHandler to be available with timeout
            let attempts = 0;
            const maxAttempts = 50; // 10 seconds max wait
            
            while (attempts < maxAttempts) {
                if (typeof window.FHIRMappingStepHandler !== 'undefined') {
                    console.log('FHIRMappingStepHandler is now available');
                    return;
                }
                
                console.log(`Waiting for FHIRMappingStepHandler... (${attempts + 1}/${maxAttempts})`);
                await new Promise(resolve => setTimeout(resolve, 200));
                attempts++;
            }
            
            throw new Error('FHIRMappingStepHandler not available after waiting');
        }

        createFallbackStep4Handler() {
            console.log('Creating fallback Step 4 handler...');
            
            // Create a minimal working handler
            class FallbackStep4Handler {
                constructor(wizard) {
                    this.wizard = wizard;
                    this.initialized = false;
                }
                
                async initialize() {
                    console.log('Fallback Step 4 handler initializing...');
                    const step4Element = document.getElementById('step4');
                    if (step4Element) {
                        step4Element.innerHTML = `
                            <div class="step-header">
                                <div class="step-progress">STEP 4 OF 5</div>
                                <h3 class="step-title">HL7 → FHIR Transformation</h3>
                                <p class="step-description">FHIR mapping functionality is loading...</p>
                            </div>
                            <div style="text-align: center; padding: 40px; color: #6b7280;">
                                <div style="font-size: 24px; margin-bottom: 12px;">⚙️</div>
                                <div style="font-weight: 600; margin-bottom: 8px;">FHIR Mapping Module Loading</div>
                                <div>The FHIR transformation module is being loaded. Please wait...</div>
                                <button onclick="window.location.reload()" style="margin-top: 16px; padding: 8px 16px; background: #3b82f6; color: white; border: none; border-radius: 6px; cursor: pointer;">
                                    Refresh Page
                                </button>
                            </div>
                        `;
                    }
                    this.initialized = true;
                    
                    // Try to load the proper handler after a delay
                    setTimeout(() => this.attemptProperHandlerLoad(), 3000);
                }
                
                async attemptProperHandlerLoad() {
                    if (typeof window.FHIRMappingStepHandler !== 'undefined') {
                        console.log('Proper handler now available, upgrading...');
                        try {
                            this.wizard.stepHandlers[4] = new FHIRMappingStepHandler(this.wizard);
                            if (typeof this.wizard.stepHandlers[4].initialize === 'function') {
                                await this.wizard.stepHandlers[4].initialize();
                            }
                            console.log('Successfully upgraded to proper Step 4 handler');
                            
                            // Refresh the step if currently visible
                            if (this.wizard.currentStep === 4) {
                                this.wizard.showStep(4);
                            }
                        } catch (error) {
                            console.error('Failed to upgrade to proper handler:', error);
                        }
                    }
                }
                
                validate() { return true; }
                reset() {}
                getStepData() { return {}; }
                onStepActivated() { 
                    if (!this.initialized) this.initialize(); 
                }
            }
            
            this.stepHandlers[4] = new FallbackStep4Handler(this);
        }

        init() {
            console.log('Initializing Enhanced Interface Wizard...');
            
            if (document.readyState !== 'complete') {
                window.addEventListener('load', () => this.init());
                return;
            }
            
            this.setupEventListeners();
            this.updateNavigation();
            console.log('Enhanced Interface Wizard initialized with modular architecture');
        }

        getElement(id) {
            const element = document.getElementById(id);
            if (!element) {
                console.warn(`Element not found: ${id}`);
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
            
            console.log('Basic event listeners setup complete');
        }

        setupModalEventListeners() {
            if (this.modalListenersSetup) {
                return;
            }

            console.log('Setting up enhanced modal event listeners...');
            
            // Modal control buttons
            this.setupControlButtons();
            
            // REMOVED: Navigation button event listeners - handled by wizard-navigation.js
            // Let wizard-navigation.js handle all navigation
            console.log('Navigation buttons handled by wizard-navigation.js');
            
            // Setup step-specific handlers
            Object.values(this.stepHandlers).forEach(handler => {
                if (handler && handler.setupEventListeners) {
                    try {
                        handler.setupEventListeners();
                    } catch (error) {
                        console.error('Error setting up handler listeners:', error);
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
            console.log('Enhanced modal event listeners setup complete (navigation delegated)');
        }

        setupControlButtons() {
            let retryCount = 0;
            const maxRetries = 10;
            
            const setupButtons = () => {
                retryCount++;
                
                const closeBtn = document.getElementById('wizardModalClose');
                const maximizeBtn = document.getElementById('wizardModalMaximize');
                
                console.log('Close button:', closeBtn ? 'Found' : 'Missing');
                console.log('Maximize button:', maximizeBtn ? 'Found' : 'Missing');
                
                if (closeBtn && maximizeBtn) {
                    // Both buttons found - set up listeners
                    closeBtn.addEventListener('click', () => {
                        console.log('Close button clicked');
                        this.closeModal();
                    });
                    
                    maximizeBtn.addEventListener('click', () => {
                        console.log('Maximize button clicked');
                        this.toggleMaximize();
                    });
                    
                    console.log('Both modal control buttons configured successfully');
                    return true;
                } else if (retryCount < maxRetries) {
                    // Retry after a short delay
                    console.log(`Retrying in 100ms... (${retryCount}/${maxRetries})`);
                    setTimeout(setupButtons, 100);
                    return false;
                } else {
                    // Max retries reached - create buttons manually
                    console.warn('Max retries reached - creating buttons manually');
                    this.createMissingModalButtons();
                    return true;
                }
            };
            
            setupButtons();
        }

        createMissingModalButtons() {
            console.log('Creating missing modal control buttons...');
            
            const modal = this.getElement('wizardModalContainer');
            if (!modal) {
                console.error('Cannot create buttons - modal container not found');
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
            
            console.log('Missing modal buttons created');
        }
        
        openModal() {
            this.isModalOpen = true;
            const overlay = this.getElement('wizardModalOverlay');
            
            if (!overlay) {
                console.error('Cannot open modal - overlay element not found');
                return;
            }
            
            overlay.classList.add('show');
            document.body.style.overflow = 'hidden';
            
            // Setup event listeners with proper timing
            setTimeout(() => {
                this.setupModalEventListeners();
            }, 200);
            
            // SYNC: Tell wizard navigation that modal is open
            if (window.wizardNavigation) {
                window.wizardNavigation.setModalOpen(true);
            }
            
            this.resetWizard();
            console.log('Modal opened');
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
            
            // SYNC: Tell wizard navigation that modal is closed
            if (window.wizardNavigation) {
                window.wizardNavigation.setModalOpen(false);
            }
            
            console.log('Modal closed');
        }
        
        toggleMaximize() {
            console.log('toggleMaximize() called');
            
            const modal = this.getElement('wizardModalOverlay');
            const maximizeIcon = this.getElement('maximizeIcon');
            const maximizeBtn = this.getElement('wizardModalMaximize');
            
            if (!modal) {
                console.error('Modal overlay not found');
                return;
            }
            
            const isMaximized = modal.classList.contains('maximized');
            
            if (isMaximized) {
                // Restore to normal size
                modal.classList.remove('maximized');
                if (maximizeIcon) maximizeIcon.textContent = '⛶';
                if (maximizeBtn) maximizeBtn.title = 'Maximize';
                console.log('Modal restored to normal size');
            } else {
                // Maximize the modal
                modal.classList.add('maximized');
                if (maximizeIcon) maximizeIcon.textContent = '🗗';
                if (maximizeBtn) maximizeBtn.title = 'Restore';
                console.log('Modal maximized');
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
                        console.error('Error resetting handler:', error);
                    }
                }
            });
            
            // SYNC: Let navigation controller handle display updates
            if (window.wizardNavigation) {
                window.wizardNavigation.setCurrentStep(1);
            } else {
                // Fallback if navigation not available
                this.showStep(1);
                this.updateStepIndicators();
                this.updateNavigation();
            }
        }

        // REMOVED: nextStep() and prevStep() methods
        // These are now handled by wizard-navigation.js to avoid conflicts

        showStep(step) {
    console.log(`showStep called for step ${step} (wizard-main.js)`);
    
    // REMOVED the faulty early return logic that was preventing DOM updates
    // OLD CODE (PROBLEMATIC):
    // if (window.wizardNavigation && window.wizardNavigation.getCurrentStep() === step) {
    //     console.log(`Step ${step} already active in navigation controller`);
    //     return; // ← This was preventing DOM updates!
    // }
    
    // ALWAYS update the DOM regardless of navigation controller state
    console.log(`Updating DOM to show step ${step}`);
    
    // Hide all wizard steps
    document.querySelectorAll('.wizard-step').forEach(el => {
        el.classList.remove('active');
        console.log(`Removed active class from:`, el.id || el.className);
    });
    
    // Show the target step
    const stepElement = this.getElement(`step${step}`);
    if (stepElement) {
        stepElement.classList.add('active');
        console.log(`Added active class to step ${step} element:`, stepElement.id);
    } else {
        console.error(`Step ${step} element not found! Expected: #step${step}`);
        // Try alternative selectors
        const altElement = document.querySelector(`[data-step="${step}"]`) || 
                          document.querySelector(`.step-${step}`) ||
                          document.querySelector(`#wizardStep${step}`);
        if (altElement) {
            altElement.classList.add('active');
            console.log(`Found step ${step} with alternative selector:`, altElement);
        }
    }
    
    // Update current step BEFORE initializing handler
    this.currentStep = step;
    
    // Update step indicators
    this.updateStepIndicators();
    
    // Update navigation buttons
    this.updateNavigation();
    
    // Initialize step-specific functionality
    const handler = this.stepHandlers[step];
    if (handler && handler.initialize) {
        try {
            console.log(`Initializing step ${step} with handler:`, handler.constructor.name);
            handler.initialize();
        } catch (error) {
            console.error(`Error initializing step ${step}:`, error);
        }
    } else {
        console.warn(`No handler available for step ${step}`);
    }
    
    // Special handling for Step 4 - activate FHIR transformation when user navigates to it
    if (step === 4 && handler && handler.onStepActivated) {
        try {
            handler.onStepActivated();
        } catch (error) {
            console.error('Error activating Step 4:', error);
        }
    }
    
    console.log(`DOM update completed for step ${step}`);
}

/**
 * ALSO ADD: Debug method to check DOM state
 */
debugStepVisibility() {
    console.log('=== STEP VISIBILITY DEBUG ===');
    for (let i = 1; i <= 5; i++) {
        const element = document.getElementById(`step${i}`);
        if (element) {
            const isActive = element.classList.contains('active');
            const isVisible = element.style.display !== 'none';
            const computedStyle = window.getComputedStyle(element);
            console.log(`Step ${i}:`, {
                element: element,
                isActive: isActive,
                isVisible: isVisible,
                display: computedStyle.display,
                visibility: computedStyle.visibility,
                classList: Array.from(element.classList)
            });
        } else {
            console.log(`Step ${i}: Element not found`);
        }
    }
    console.log('Current step:', this.currentStep);
    console.log('Navigation current step:', window.wizardNavigation?.getCurrentStep());
}

        updateStepIndicators() {
            // DEFER: Let wizard-navigation handle this to avoid conflicts
            if (window.wizardNavigation) {
                console.log('Deferring step indicators update to wizard-navigation');
                return;
            }
            
            // Fallback if navigation controller not available
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
            // DEFER: Let wizard-navigation handle this to avoid conflicts
            if (window.wizardNavigation) {
                console.log('Deferring navigation update to wizard-navigation');
                return;
            }
            
            // Fallback if navigation controller not available
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
            console.log('Finishing wizard...');
            
            try {
                this.showLoading('Creating interface...', 'Please wait while we set up your HL7 interface');
                
                // Simulate API call
                await this.delay(2000);
                
                this.hideLoading();
                this.showNotification('Interface created successfully!', 'success');
                
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
                console.error('Failed to create interface:', error);
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
                console.error('Failed to initialize wizard:', error);
                AppDialogs.toast('Error: Could not initialize the interface wizard. Please refresh the page and try again.', 'error');
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
            console.log('Wizard already initialized, skipping');
            return;
        }
        
        try {
            window.wizard = new InterfaceWizardModal();
            console.log('Enhanced Interface Wizard initialized with navigation integration');
        } catch (error) {
            console.error('Failed to initialize wizard:', error);
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

    console.log('Protected wizard-main.js loaded successfully with navigation integration');

})(); // Close the IIFE