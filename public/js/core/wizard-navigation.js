// js/core/wizard-navigation.js - FIXED - Complete wizard navigation with proper step handler integration

(function() {
    'use strict';
    
    /**
     * Consolidated Wizard Navigation Controller
     * Handles ALL wizard state management and synchronization with step handlers
     */
    class WizardNavigation {
        constructor() {
            this.currentStep = 1;
            this.totalSteps = 5;
            this.isMaximized = false;
            this.validationInProgress = false;
            this.isModalOpen = false;
            this.modalListenersSetup = false;
            
            // Track step initialization
            this.stepsInitialized = new Set();
            
            this.stepTitles = [
                'Interface Configuration',
                'Upload Sample HL7 Message', 
                'Review Parsed Data',
                'Configure Data Mapping',
                'Deploy Interface'
            ];
            
            this.stepDescriptions = [
                'Set up the basic configuration for your HL7 interface',
                'Upload a sample HL7 message to analyze structure',
                'Verify the HL7 message structure and segments', 
                'Configure how HL7 fields map to target format',
                'Review and deploy your new HL7 interface'
            ];
            
            this.init();
        }
        
        init() {
            this.setupEventListeners();
            this.updateNavigation();
            
            // SYNC: Ensure wizard-main.js follows our state
            this.synchronizeWithWizardMain();
            
            console.log('Wizard navigation initialized with step handler integration');
        }
        
        /**
         * SYNC: Keep wizard-main.js in sync with our currentStep
         */
        synchronizeWithWizardMain() {
            if (window.wizard) {
                window.wizard.currentStep = this.currentStep;
                console.log('Synchronized wizard-main currentStep to:', this.currentStep);
            }
            
            // Set up periodic sync to handle any drift
            if (!this.syncInterval) {
                this.syncInterval = setInterval(() => {
                    if (window.wizard && window.wizard.currentStep !== this.currentStep) {
                        console.log('Drift detected, syncing wizard-main...');
                        window.wizard.currentStep = this.currentStep;
                    }
                }, 1000);
            }
        }
        
        /**
         * FIXED: Event listener setup with enhanced button detection
         */
        setupEventListeners() {
            console.log('Setting up wizard navigation event listeners...');
            
            let retryCount = 0;
            const maxRetries = 10;
            
            const setupListeners = () => {
                retryCount++;
                
                // Enhanced button detection with multiple strategies
                const nextBtn = document.getElementById('nextBtn') ||
                               document.querySelector('.wizard-btn.primary') ||
                               document.querySelector('button[onclick*="next"]') ||
                               document.querySelector('[id*="next"]');
                
                const prevBtn = document.getElementById('prevBtn') ||
                               document.querySelector('.wizard-btn.secondary') ||
                               document.querySelector('button[onclick*="prev"]');
                
                const finishBtn = document.getElementById('finishBtn') ||
                                 document.querySelector('button[onclick*="finish"]');
                
                console.log(`Attempt ${retryCount} - Found buttons:`, {
                    nextBtn: !!nextBtn,
                    prevBtn: !!prevBtn,
                    finishBtn: !!finishBtn
                });
                
                if (nextBtn) {
                    // Remove any existing onclick handlers
                    nextBtn.removeAttribute('onclick');
                    nextBtn.onclick = null;
                    
                    // Add our event listener
                    nextBtn.addEventListener('click', (e) => {
                        e.preventDefault();
                        this.nextStep();
                    });
                    console.log('✅ Next button event listener attached');
                }
                
                if (prevBtn) {
                    prevBtn.removeAttribute('onclick');
                    prevBtn.onclick = null;
                    prevBtn.addEventListener('click', (e) => {
                        e.preventDefault();
                        this.previousStep();
                    });
                    console.log('✅ Previous button event listener attached');
                }
                
                if (finishBtn) {
                    finishBtn.removeAttribute('onclick');
                    finishBtn.onclick = null;
                    finishBtn.addEventListener('click', (e) => {
                        e.preventDefault();
                        this.finishWizard();
                    });
                    console.log('✅ Finish button event listener attached');
                }
                
                // If critical buttons not found, retry
                if (!nextBtn && retryCount < maxRetries) {
                    console.log(`⚠️ Next button not found, retrying in 200ms... (${retryCount}/${maxRetries})`);
                    setTimeout(setupListeners, 200);
                    return;
                }
                
                // Setup other modal event listeners
                this.setupModalEventListeners();
            };
            
            setupListeners();
        }
        
        /**
         * FIXED: Next step with comprehensive validation using step handlers
         */
        async nextStep() {
    if (this.validationInProgress) {
        console.log('Validation already in progress, please wait...');
        return;
    }
    
    console.log(`Next step requested from ${this.currentStep}`);
    
    try {
        this.validationInProgress = true;
        
        // Validate current step using step handlers (preferred) or fallback
        const validationResult = await this.validateCurrentStep();
        console.log('Validation result:', validationResult);
        
        if (!validationResult) {
            console.log('🚫 BLOCKED: Current step validation failed - step transition cancelled');
            this.validationInProgress = false;
            return; // BLOCK progression - this is the key fix
        }
        
        // Only proceed if validation passes
        if (this.currentStep < this.totalSteps) {
            this.setCurrentStep(this.currentStep + 1);
            console.log(`✅ Successfully moved to step ${this.currentStep}`);
        }
        
    } catch (error) {
        console.error('Next step error:', error);
        this.showNotification('An error occurred during validation. Please try again.', 'error');
    } finally {
        this.validationInProgress = false;
    }
}

/**
 * FIXED: Validation that properly delegates to step handlers with correct fallback
 */
async validateCurrentStep() {
    console.log(`Validating step ${this.currentStep}...`);
    
    // First try to use step handlers from wizard-main.js
    if (window.wizard && window.wizard.stepHandlers && window.wizard.stepHandlers[this.currentStep]) {
        const stepHandler = window.wizard.stepHandlers[this.currentStep];
        if (typeof stepHandler.validate === 'function') {
            console.log(`Deferring Step ${this.currentStep} validation to step handler`);
            try {
                const result = await stepHandler.validate();
                console.log(`Step handler validation result for step ${this.currentStep}:`, result);
                return result;
            } catch (error) {
                console.error(`Step handler validation error for step ${this.currentStep}:`, error);
                return false; // Block on handler errors
            }
        }
    }
    
    // Fallback validation if no step handler available
    console.log(`Using fallback validation for step ${this.currentStep}`);
    switch (this.currentStep) {
        case 1:
            return await this.validateStep1(); // Use the fixed validateStep1 method above
        case 2:
            return true; // Upload step is optional
        case 3:
            return true; // Review step is always valid
        case 4:
            return true; // Mapping step basic validation
        case 5:
            return true; // Final step
        default:
            return true;
    }
}


        
        previousStep() {
            console.log(`Previous step requested from ${this.currentStep}`);
            
            if (this.currentStep > 1) {
                this.setCurrentStep(this.currentStep - 1);
                console.log(`Moved to step ${this.currentStep}`);
            }
        }
        
        goToStep(stepNumber) {
            if (stepNumber >= 1 && stepNumber <= this.totalSteps) {
                this.setCurrentStep(stepNumber);
                console.log(`Jumped to step ${this.currentStep}`);
            }
        }
        
        /**
         * FIXED: Set current step with proper handler initialization
         */
        setCurrentStep(stepNumber) {
            this.currentStep = stepNumber;
            
            // Sync with wizard-main
            if (window.wizard) {
                window.wizard.currentStep = stepNumber;
                window.wizard.showStep(stepNumber);
            } else {
                // Fallback if wizard-main not available
                this.showStep(stepNumber);
            }
            
            this.updateNavigation();
            this.updateStepIndicators();
            
            // Initialize step handler if not already initialized
            if (!this.stepsInitialized.has(stepNumber)) {
                this.initializeStepHandler(stepNumber);
                this.stepsInitialized.add(stepNumber);
            }
        }
        
        /**
         * FIXED: Initialize step handler with proper error handling
         */
        initializeStepHandler(step) {
            const handler = window.wizard?.stepHandlers?.[step];
            if (handler && typeof handler.initialize === 'function') {
                try {
                    handler.initialize();
                    console.log(`Step ${step} handler initialized successfully`);
                } catch (error) {
                    console.error(`Failed to initialize step ${step} handler:`, error);
                }
            }
        }
        
        /**
         * FIXED: Validation that properly delegates to step handlers
         */
        async validateCurrentStep() {
            console.log(`Validating step ${this.currentStep}...`);
            
            // First try to use step handlers from wizard-main.js
            if (window.wizard && window.wizard.stepHandlers && window.wizard.stepHandlers[this.currentStep]) {
                const stepHandler = window.wizard.stepHandlers[this.currentStep];
                if (typeof stepHandler.validate === 'function') {
                    console.log(`Deferring Step ${this.currentStep} validation to step handler`);
                    try {
                        const result = await stepHandler.validate();
                        console.log(`Step handler validation result for step ${this.currentStep}:`, result);
                        return result;
                    } catch (error) {
                        console.error(`Step handler validation error for step ${this.currentStep}:`, error);
                        return false;
                    }
                }
            }
            
            // Fallback validation if no step handler available
            console.log(`Using fallback validation for step ${this.currentStep}`);
            switch (this.currentStep) {
                case 1:
                    return await this.validateStep1();
                case 2:
                    return true; // Upload step is optional
                case 3:
                    return true; // Review step is always valid
                case 4:
                    return true; // Mapping step basic validation
                case 5:
                    return true; // Final step
                default:
                    return true;
            }
        }
        
        /**
         * FIXED: Step 1 validation with duplicate name checking
         */
        async validateStep1() {
    console.log('Validating Step 1 with duplicate check (fallback method)...');
    
    const nameInput = document.getElementById('wizardInterfaceName');
    const sourceSelect = document.getElementById('wizardSourceType');
    const targetSelect = document.getElementById('wizardTargetType');
    
    // Check if required elements exist
    if (!nameInput) {
        console.error('Interface name input not found in DOM');
        this.showNotification('Interface name field not found. Please refresh the page.', 'error');
        return false;
    }
    
    if (!sourceSelect) {
        console.error('Source type select not found in DOM'); 
        this.showNotification('Source type field not found. Please refresh the page.', 'error');
        return false;
    }
    
    if (!targetSelect) {
        console.error('Target type select not found in DOM');
        this.showNotification('Target type field not found. Please refresh the page.', 'error');
        return false;
    }
    
    // Validate interface name
    const name = nameInput.value?.trim();
    if (!name) {
        console.error('BLOCKING: Interface name is empty');
        this.showNotification('Please enter an interface name', 'error');
        nameInput.focus();
        nameInput.style.borderColor = '#ef4444';
        nameInput.style.boxShadow = '0 0 0 3px rgba(239, 68, 68, 0.1)';
        return false;
    }
    
    // FIXED: Check for duplicate interface name with BOTH possible API response formats
    try {
        console.log('Fallback validation: Checking for duplicate interface name:', name);
        const duplicateResponse = await fetch('/api/wizard/check-duplicate', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ name: name })
        });
        
        if (duplicateResponse.ok) {
            const duplicateResult = await duplicateResponse.json();
            console.log('Fallback validation: Duplicate check response:', duplicateResult);
            
            // FIXED: Handle BOTH possible API response formats
            let isDuplicate = false;
            
            // Format 1: {success: false, message: "..."}  (controller format)
            if (duplicateResult.success === false) {
                isDuplicate = true;
            }
            // Format 2: {success: true, isDuplicate: true, message: "..."} (actual API format from logs)
            else if (duplicateResult.isDuplicate === true) {
                isDuplicate = true;
            }
            
            if (isDuplicate) {
                console.error('BLOCKING: Interface name already exists (fallback validation)');
                this.showNotification(duplicateResult.message || 'An interface with this name already exists. Please choose a different name.', 'error');
                nameInput.focus();
                nameInput.style.borderColor = '#ef4444';
                nameInput.style.boxShadow = '0 0 0 3px rgba(239, 68, 68, 0.1)';
                return false; // BLOCK step progression
            }
            
            console.log('✅ Fallback validation: Interface name is unique');
            // Clear any error styling
            nameInput.style.borderColor = '';
            nameInput.style.boxShadow = '';
            
        } else {
            // If duplicate check fails, log but continue (don't block user)
            console.warn('Fallback validation: Duplicate check failed, continuing anyway:', duplicateResponse.status);
        }
    } catch (error) {
        // If duplicate check fails, log but continue (don't block user)
        console.warn('Fallback validation: Duplicate check error, continuing anyway:', error.message);
    }
    
    // Validate source type
    const sourceValue = sourceSelect.value;
    if (!sourceValue) {
        console.error('BLOCKING: Source type not selected');
        this.showNotification('Please select a source type', 'error');
        sourceSelect.focus();
        sourceSelect.style.borderColor = '#ef4444';
        return false;
    } else {
        // Clear error styling
        sourceSelect.style.borderColor = '';
    }
    
    // Validate target type  
    const targetValue = targetSelect.value;
    if (!targetValue) {
        console.error('BLOCKING: Target type not selected');
        this.showNotification('Please select a target type', 'error');
        targetSelect.focus();
        targetSelect.style.borderColor = '#ef4444';
        return false;
    } else {
        // Clear error styling
        targetSelect.style.borderColor = '';
    }
    
    console.log('✅ Fallback Step 1 validation completed successfully');
    return true;
}
        
        /**
         * Show step with proper handler integration
         */
        showStep(step) {
            // Hide all steps
            document.querySelectorAll('.wizard-step').forEach(el => el.classList.remove('active'));
            
            // Show target step
            const stepElement = document.getElementById(`step${step}`);
            if (stepElement) {
                stepElement.classList.add('active');
            }
            
            // Initialize step handler if available
            const handler = window.wizard?.stepHandlers?.[step];
            if (handler && typeof handler.initialize === 'function') {
                try {
                    handler.initialize();
                    console.log(`Step ${step} handler initialized`);
                } catch (error) {
                    console.error(`Error initializing step ${step} handler:`, error);
                }
            }
        }
        
        /**
         * Update navigation buttons and indicators
         */
        updateNavigation() {
            const nextBtn = document.getElementById('nextBtn');
            const prevBtn = document.getElementById('prevBtn');
            const finishBtn = document.getElementById('finishBtn');
            const navStep = document.getElementById('navStep');
            const navTitle = document.getElementById('navTitle');
            
            // Update navigation info
            if (navStep) {
                navStep.textContent = `Step ${this.currentStep} of ${this.totalSteps} • ${this.stepTitles[this.currentStep - 1]}`;
            }
            
            if (navTitle) {
                navTitle.textContent = this.stepTitles[this.currentStep - 1];
            }
            
            // Show/hide buttons
            if (prevBtn) {
                prevBtn.style.display = this.currentStep > 1 ? 'flex' : 'none';
            }
            
            if (this.currentStep === this.totalSteps) {
                if (nextBtn) nextBtn.style.display = 'none';
                if (finishBtn) finishBtn.style.display = 'flex';
            } else {
                if (nextBtn) nextBtn.style.display = 'flex';
                if (finishBtn) finishBtn.style.display = 'none';
            }
        }
        
        /**
         * Update step indicators
         */
        updateStepIndicators() {
            for (let i = 1; i <= this.totalSteps; i++) {
                const indicator = document.getElementById(`stepIndicator${i}`);
                const circle = document.getElementById(`stepCircle${i}`);
                const connector = document.getElementById(`connector${i}`);
                
                if (indicator) {
                    indicator.classList.toggle('active', i === this.currentStep);
                    indicator.classList.toggle('completed', i < this.currentStep);
                }
                
                if (circle) {
                    if (i < this.currentStep) {
                        circle.innerHTML = '✓';
                        circle.style.background = '#22c55e';
                        circle.style.color = 'white';
                    } else {
                        circle.innerHTML = i;
                        circle.style.background = i === this.currentStep ? 'white' : 'rgba(255, 255, 255, 0.1)';
                        circle.style.color = i === this.currentStep ? '#1e3a8a' : 'rgba(255, 255, 255, 0.7)';
                    }
                }
                
                if (connector) {
                    connector.classList.toggle('active', i < this.currentStep);
                }
            }
        }
        
        /**
         * FIXED: Enhanced modal event listener setup
         */
        setupModalEventListeners() {
            if (this.modalListenersSetup) {
                return;
            }

            console.log('Setting up enhanced modal event listeners...');
            
            // Enhanced maximize button setup with retry mechanism
            let retryCount = 0;
            const maxRetries = 10;
            
            const setupModalButtons = () => {
                retryCount++;
                
                const maximizeBtn = document.getElementById('wizardModalMaximize') ||
                                   document.querySelector('.maximize-btn') ||
                                   document.querySelector('[title*="Maximize"]');
                
                const closeBtn = document.getElementById('wizardModalClose') ||
                                document.querySelector('.close-btn') ||
                                document.querySelector('[title*="Close"]');
                
                console.log(`Modal buttons attempt ${retryCount}:`, {
                    maximizeBtn: !!maximizeBtn,
                    closeBtn: !!closeBtn
                });
                
                if (maximizeBtn) {
                    maximizeBtn.removeAttribute('onclick');
                    maximizeBtn.onclick = null;
                    maximizeBtn.addEventListener('click', (e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        this.toggleMaximize();
                    });
                    console.log('✅ Maximize button configured');
                }
                
                if (closeBtn) {
                    closeBtn.removeAttribute('onclick');
                    closeBtn.onclick = null;
                    closeBtn.addEventListener('click', (e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        this.closeWizard();
                    });
                    console.log('✅ Close button configured');
                }
                
                // Retry if buttons not found
                if ((!maximizeBtn || !closeBtn) && retryCount < maxRetries) {
                    console.log(`⚠️ Modal buttons not found, retrying in 100ms... (${retryCount}/${maxRetries})`);
                    setTimeout(setupModalButtons, 100);
                    return;
                }
            };
            
            setupModalButtons();
            
            // Backdrop click handling
            const overlay = document.getElementById('wizardModalOverlay') ||
                           document.querySelector('.wizard-modal-overlay');
            
            if (overlay) {
                overlay.addEventListener('click', (e) => {
                    if (e.target === overlay) {
                        this.closeWizard();
                    }
                });
            }
            
            // Keyboard escape handling
            document.addEventListener('keydown', (e) => {
                if (e.key === 'Escape' && this.isModalOpen) {
                    this.closeWizard();
                }
            });
            
            this.modalListenersSetup = true;
            console.log('✅ Enhanced modal event listeners setup complete');
        }
        
        /**
         * FIXED: Enhanced maximize functionality
         */
        toggleMaximize() {
            console.log('Toggle maximize called...');
            
            const modalOverlay = document.getElementById('wizardModalOverlay') ||
                                document.querySelector('.wizard-modal-overlay');
            
            const modalContainer = document.getElementById('wizardModalContainer') ||
                                  modalOverlay?.querySelector('.wizard-modal-container');
            
            const maximizeIcon = document.getElementById('maximizeIcon') ||
                                document.querySelector('#wizardModalMaximize span');
            
            const maximizeBtn = document.getElementById('wizardModalMaximize');
            
            if (!modalOverlay || !modalContainer) {
                console.error('Modal elements not found for maximize operation');
                return;
            }
            
            const isCurrentlyMaximized = modalOverlay.classList.contains('maximized') ||
                                        modalContainer.classList.contains('maximized');
            
            try {
                if (isCurrentlyMaximized) {
                    // Restore to normal size
                    modalOverlay.classList.remove('maximized');
                    modalContainer.classList.remove('maximized');
                    
                    // Reset styles
                    [modalOverlay, modalContainer].forEach(element => {
                        if (element) {
                            element.style.position = '';
                            element.style.top = '';
                            element.style.left = '';
                            element.style.width = '';
                            element.style.height = '';
                            element.style.zIndex = '';
                            element.style.borderRadius = '';
                            element.style.maxWidth = '';
                            element.style.maxHeight = '';
                        }
                    });
                    
                    if (maximizeIcon) maximizeIcon.textContent = '⛶';
                    if (maximizeBtn) maximizeBtn.title = 'Maximize';
                    
                    console.log('✅ Modal restored to normal size');
                    
                } else {
                    // Maximize to full screen
                    modalOverlay.classList.add('maximized');
                    modalContainer.classList.add('maximized');
                    
                    const fullScreenStyles = {
                        position: 'fixed',
                        top: '0',
                        left: '0',
                        width: '100vw',
                        height: '100vh',
                        zIndex: '10000',
                        borderRadius: '0',
                        maxWidth: 'none',
                        maxHeight: 'none'
                    };
                    
                    [modalOverlay, modalContainer].forEach(element => {
                        if (element) {
                            Object.entries(fullScreenStyles).forEach(([property, value]) => {
                                element.style.setProperty(property, value, 'important');
                            });
                        }
                    });
                    
                    if (maximizeIcon) maximizeIcon.textContent = '🗗';
                    if (maximizeBtn) maximizeBtn.title = 'Restore';
                    
                    console.log('✅ Modal maximized to full screen');
                }
                
                // Trigger resize event for responsive components
                window.dispatchEvent(new Event('resize'));
                
            } catch (error) {
                console.error('Error during maximize operation:', error);
            }
        }
        
        /**
         * Close wizard
         */
        closeWizard() {
            if (typeof window.closeWizard === 'function') {
                window.closeWizard();
            } else if (window.wizard && typeof window.wizard.closeModal === 'function') {
                window.wizard.closeModal();
            } else {
                const overlay = document.getElementById('wizardModalOverlay');
                if (overlay) {
                    overlay.classList.remove('show');
                    this.isModalOpen = false;
                }
            }
        }
        
        /**
         * Finish wizard - FIXED: Prevent multiple submissions
         */
        finishWizard() {
            // Prevent multiple clicks at navigation level
            if (this.isFinishing) {
                console.log('Wizard finish already in progress, ignoring click');
                return;
            }
            
            this.isFinishing = true;
            
            // Disable finish button immediately
            const finishBtn = document.getElementById('finishBtn');
            if (finishBtn) {
                finishBtn.disabled = true;
                finishBtn.classList.add('disabled');
                finishBtn.style.pointerEvents = 'none';
                finishBtn.innerHTML = '<span style="display: inline-block; animation: spin 1s linear infinite;">⟳</span> Creating...';
            }
            
            if (window.wizard && typeof window.wizard.finishWizard === 'function') {
                window.wizard.finishWizard().finally(() => {
                    // Reset state after wizard completes (success or failure)
                    this.isFinishing = false;
                    if (finishBtn) {
                        finishBtn.disabled = false;
                        finishBtn.classList.remove('disabled');
                        finishBtn.style.pointerEvents = '';
                        finishBtn.innerHTML = 'Create Interface';
                    }
                });
            } else {
                console.log('Wizard completed!');
                this.showNotification('Interface created successfully!', 'success');
                setTimeout(() => {
                    this.closeWizard();
                    this.isFinishing = false;
                    if (finishBtn) {
                        finishBtn.disabled = false;
                        finishBtn.classList.remove('disabled');
                        finishBtn.style.pointerEvents = '';
                        finishBtn.innerHTML = 'Create Interface';
                    }
                }, 2000);
            }
        }
        
        /**
         * FIXED: Enhanced notification system
         */
        showNotification(message, type = 'info') {
            console.log(`📢 ${type.toUpperCase()}: ${message}`);
            
            // Try multiple notification approaches
            if (window.wizard && typeof window.wizard.showNotification === 'function') {
                window.wizard.showNotification(message, type);
                return;
            }
            
            if (window.showNotification) {
                window.showNotification(message, type);
                return;
            }
            
            // Fallback notification implementation
            const notification = document.createElement('div');
            notification.className = `notification notification-${type}`;
            notification.textContent = message;
            notification.style.cssText = `
                position: fixed;
                top: 20px;
                right: 20px;
                padding: 12px 20px;
                border-radius: 6px;
                color: white;
                font-size: 14px;
                font-weight: 500;
                z-index: 10001;
                background: ${type === 'success' ? '#22c55e' : type === 'error' ? '#ef4444' : '#3b82f6'};
                box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
                animation: slideInRight 0.3s ease;
            `;
            
            document.body.appendChild(notification);
            
            setTimeout(() => {
                notification.style.animation = 'slideOutRight 0.3s ease';
                setTimeout(() => notification.remove(), 300);
            }, 3000);
        }
        
        // Public API methods
        getCurrentStep() {
            return this.currentStep;
        }
        
        getTotalSteps() {
            return this.totalSteps;
        }
        
        isOnLastStep() {
            return this.currentStep === this.totalSteps;
        }
        
        setModalOpen(isOpen) {
            this.isModalOpen = isOpen;
        }
    }
    
    // Initialize and expose globally
    let wizardNavigation = null;
    
    function initWizardNavigation() {
        if (!wizardNavigation) {
            wizardNavigation = new WizardNavigation();
            window.wizardNavigation = wizardNavigation;
            console.log('✅ Wizard navigation controller initialized');
        }
        return wizardNavigation;
    }
    
    // Auto-initialize
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initWizardNavigation);
    } else {
        initWizardNavigation();
    }
    
    // Add CSS for notifications if not exists
    if (!document.querySelector('#wizard-nav-styles')) {
        const style = document.createElement('style');
        style.id = 'wizard-nav-styles';
        style.textContent = `
            @keyframes slideInRight {
                from { transform: translateX(100%); opacity: 0; }
                to { transform: translateX(0); opacity: 1; }
            }
            
            @keyframes slideOutRight {
                from { transform: translateX(0); opacity: 1; }
                to { transform: translateX(100%); opacity: 0; }
            }
            
            @keyframes spin {
                0% { transform: rotate(0deg); }
                100% { transform: rotate(360deg); }
            }
            
            .wizard-btn.disabled {
                opacity: 0.6;
                cursor: not-allowed !important;
            }
        `;
        document.head.appendChild(style);
    }
    
})();