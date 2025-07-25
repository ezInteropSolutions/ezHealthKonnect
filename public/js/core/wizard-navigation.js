// js/core/wizard-navigation.js - Missing Wizard Navigation Logic
// This handles the Next/Previous buttons and step navigation

(function() {
    'use strict';
    
    /**
     * Wizard Navigation Controller
     * Handles step transitions, validation, and navigation buttons
     */
    class WizardNavigation {
        constructor() {
            this.currentStep = 1;
            this.totalSteps = 5;
            this.isMaximized = false;
            
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
            console.log('✅ Wizard navigation initialized');
        }
        
        setupEventListeners() {
            // Wait for wizard to be loaded
            document.addEventListener('DOMContentLoaded', () => {
                setTimeout(() => this.bindEvents(), 500);
            });
            
            // If DOM is already ready
            if (document.readyState !== 'loading') {
                setTimeout(() => this.bindEvents(), 500);
            }
        }
        
        bindEvents() {
            // Navigation buttons
            const nextBtn = document.getElementById('nextBtn');
            const prevBtn = document.getElementById('prevBtn');
            const finishBtn = document.getElementById('finishBtn');
            
            if (nextBtn) {
                nextBtn.addEventListener('click', () => this.nextStep());
                console.log('✅ Next button event bound');
            }
            
            if (prevBtn) {
                prevBtn.addEventListener('click', () => this.previousStep());
                console.log('✅ Previous button event bound');
            }
            
            if (finishBtn) {
                finishBtn.addEventListener('click', () => this.finishWizard());
                console.log('✅ Finish button event bound');
            }
            
            // Maximize button
            const maximizeBtn = document.getElementById('wizardModalMaximize');
            if (maximizeBtn) {
                maximizeBtn.addEventListener('click', () => this.toggleMaximize());
                console.log('✅ Maximize button event bound');
            }
            
            // Close button
            const closeBtn = document.getElementById('wizardModalClose');
            if (closeBtn) {
                closeBtn.addEventListener('click', () => this.closeWizard());
                console.log('✅ Close button event bound');
            }
            
            // Step indicator clicks
            for (let i = 1; i <= this.totalSteps; i++) {
                const stepIndicator = document.getElementById(`stepIndicator${i}`);
                if (stepIndicator) {
                    stepIndicator.addEventListener('click', (e) => {
                        e.preventDefault();
                        this.goToStep(i);
                    });
                }
            }
        }
        
        nextStep() {
            console.log(`🔄 Next step requested from ${this.currentStep}`);
            
            // Validate current step
            if (!this.validateCurrentStep()) {
                console.log('❌ Current step validation failed');
                return;
            }
            
            if (this.currentStep < this.totalSteps) {
                this.currentStep++;
                this.updateWizardDisplay();
                console.log(`✅ Moved to step ${this.currentStep}`);
            }
        }
        
        previousStep() {
            console.log(`🔄 Previous step requested from ${this.currentStep}`);
            
            if (this.currentStep > 1) {
                this.currentStep--;
                this.updateWizardDisplay();
                console.log(`✅ Moved to step ${this.currentStep}`);
            }
        }
        
        goToStep(stepNumber) {
            if (stepNumber >= 1 && stepNumber <= this.totalSteps) {
                this.currentStep = stepNumber;
                this.updateWizardDisplay();
                console.log(`✅ Jumped to step ${this.currentStep}`);
            }
        }
        
        validateCurrentStep() {
            // Basic validation for each step
            switch (this.currentStep) {
                case 1: // Configuration
                    return this.validateStep1();
                case 2: // Upload
                    return this.validateStep2();
                case 3: // Review  
                    return this.validateStep3();
                case 4: // Mapping
                    return this.validateStep4();
                case 5: // Deploy
                    return true; // Final step
                default:
                    return true;
            }
        }
        
        // PATCH for wizard-navigation.js - Fix next button from step 1 not working
// Replace the existing validateStep1() function with this enhanced version:

validateStep1() {
    console.log('🔍 Validating Step 1 with enhanced detection...');
    
    // FIX: More robust element detection with multiple strategies
    const nameInput = document.getElementById('wizardInterfaceName') || 
                     document.querySelector('[id="wizardInterfaceName"]') ||
                     document.querySelector('input[placeholder*="ADT Patient"]') ||
                     document.querySelector('input[placeholder*="Interface"]');
    
    const sourceSelect = document.getElementById('wizardSourceType') ||
                        document.querySelector('[id="wizardSourceType"]') ||
                        document.querySelector('select[required]');
    
    const targetSelect = document.getElementById('wizardTargetType') ||
                        document.querySelector('[id="wizardTargetType"]') ||
                        document.querySelectorAll('select[required]')[1];
    
    console.log('🔍 Step 1 form elements found:', {
        nameInput: !!nameInput,
        sourceSelect: !!sourceSelect, 
        targetSelect: !!targetSelect,
        nameValue: nameInput?.value,
        sourceValue: sourceSelect?.value,
        targetValue: targetSelect?.value
    });
    
    // FIX: Validate name field with better error handling
    if (!nameInput) {
        console.error('❌ Interface name input not found in DOM');
        this.showNotification('Interface name field not found. Please refresh the page.', 'error');
        return false;
    }
    
    const name = nameInput.value?.trim();
    if (!name) {
        console.error('❌ Interface name is empty');
        this.showNotification('Please enter an interface name', 'error');
        nameInput.focus();
        nameInput.style.borderColor = '#ef4444'; // Visual feedback
        return false;
    } else {
        nameInput.style.borderColor = ''; // Reset border
    }
    
    // FIX: Validate source type with better error handling
    if (!sourceSelect) {
        console.error('❌ Source type select not found in DOM'); 
        this.showNotification('Source type field not found. Please refresh the page.', 'error');
        return false;
    }
    
    const sourceType = sourceSelect.value;
    if (!sourceType || sourceType === '') {
        console.error('❌ Source type not selected');
        this.showNotification('Please select a source type', 'error');
        sourceSelect.focus();
        sourceSelect.style.borderColor = '#ef4444'; // Visual feedback
        return false;
    } else {
        sourceSelect.style.borderColor = ''; // Reset border
    }
    
    // FIX: Validate target type with better error handling
    if (!targetSelect) {
        console.error('❌ Target type select not found in DOM');
        this.showNotification('Target type field not found. Please refresh the page.', 'error'); 
        return false;
    }
    
    const targetType = targetSelect.value;
    if (!targetType || targetType === '') {
        console.error('❌ Target type not selected');
        this.showNotification('Please select a target type', 'error');
        targetSelect.focus();
        targetSelect.style.borderColor = '#ef4444'; // Visual feedback
        return false;
    } else {
        targetSelect.style.borderColor = ''; // Reset border
    }
    
    // FIX: Success case - store data and provide feedback
    console.log('✅ Step 1 validation passed:', { name, sourceType, targetType });
    this.showNotification('Configuration validated successfully!', 'success');
    
    // Store validated data in wizard instance
    if (this.wizardData) {
        this.wizardData.name = name;
        this.wizardData.sourceType = sourceType;
        this.wizardData.targetType = targetType;
        
        // Also capture description if present
        const descElement = document.getElementById('wizardInterfaceDescription');
        this.wizardData.description = descElement ? descElement.value.trim() : '';
        
        console.log('✅ Wizard data updated:', this.wizardData);
    }
    
    return true;
}

// FIX: Enhanced next step function with better error handling
nextStep() {
    console.log(`🔄 Next step requested from ${this.currentStep}`);
    
    // FIX: Validate current step with enhanced logging
    let validationResult = false;
    try {
        validationResult = this.validateCurrentStep();
        console.log('🔍 Validation result:', validationResult);
    } catch (error) {
        console.error('❌ Validation error:', error);
        this.showNotification('Validation failed due to an error. Please try again.', 'error');
        return;
    }
    
    if (!validationResult) {
        console.log('❌ Current step validation failed, staying on step', this.currentStep);
        return;
    }
    
    // FIX: Proceed to next step with proper bounds checking
    if (this.currentStep < this.totalSteps) {
        const previousStep = this.currentStep;
        this.currentStep++;
        
        try {
            this.updateWizardDisplay();
            console.log(`✅ Successfully moved from step ${previousStep} to step ${this.currentStep}`);
            
            // FIX: Show step transition feedback
            this.showNotification(`Proceeding to Step ${this.currentStep}`, 'info');
            
        } catch (error) {
            console.error('❌ Error updating wizard display:', error);
            // Rollback on error
            this.currentStep = previousStep;
            this.showNotification('Failed to proceed to next step. Please try again.', 'error');
        }
    } else {
        console.log('🏁 Already on final step');
    }
}

// FIX: Enhanced event listener setup with retry mechanism
setupEventListeners() {
    console.log('🔍 Setting up wizard navigation event listeners...');
    
    let retryCount = 0;
    const maxRetries = 5;
    
    const setupListeners = () => {
        retryCount++;
        
        // FIX: Next button with multiple detection strategies
        const nextBtn = document.getElementById('nextBtn') ||
                       document.querySelector('.wizard-btn.primary') ||
                       document.querySelector('button[onclick*="next"]') ||
                       document.querySelector('[id*="next"]');
        
        const prevBtn = document.getElementById('prevBtn') ||
                       document.querySelector('.wizard-btn.secondary') ||
                       document.querySelector('button[onclick*="prev"]');
        
        const finishBtn = document.getElementById('finishBtn') ||
                         document.querySelector('button[onclick*="finish"]');
        
        console.log(`🔍 Attempt ${retryCount} - Found buttons:`, {
            nextBtn: !!nextBtn,
            prevBtn: !!prevBtn,
            finishBtn: !!finishBtn
        });
        
        if (nextBtn) {
            // FIX: Remove any existing onclick handlers and use event listener
            nextBtn.removeAttribute('onclick');
            nextBtn.removeEventListener('click', () => this.nextStep());
            nextBtn.addEventListener('click', () => this.nextStep());
            console.log('✅ Next button event listener attached');
        }
        
        if (prevBtn) {
            prevBtn.removeAttribute('onclick');
            prevBtn.removeEventListener('click', () => this.previousStep());
            prevBtn.addEventListener('click', () => this.previousStep());
            console.log('✅ Previous button event listener attached');
        }
        
        if (finishBtn) {
            finishBtn.removeAttribute('onclick');
            finishBtn.removeEventListener('click', () => this.finishWizard());
            finishBtn.addEventListener('click', () => this.finishWizard());
            console.log('✅ Finish button event listener attached');
        }
        
        // FIX: If critical buttons not found, retry
        if (!nextBtn && retryCount < maxRetries) {
            console.log(`⚠️ Next button not found, retrying in 200ms... (${retryCount}/${maxRetries})`);
            setTimeout(setupListeners, 200);
            return;
        }
        
        // Continue with other event listeners...
        this.setupModalEventListeners();
    };
    
    setupListeners();
}

// FIX: Add this helper method for better notifications
showNotification(message, type = 'info') {
    console.log(`📢 ${type.toUpperCase()}: ${message}`);
    
    // Try multiple notification approaches
    if (this.notificationService && this.notificationService.show) {
        this.notificationService.show(message, type);
    } else if (window.showNotification) {
        window.showNotification(message, type);
    } else {
        // Fallback to console for non-critical messages
        console.log(`${type}: ${message}`);
        
        // Show alert only for errors
        if (type === 'error') {
            alert(`Error: ${message}`);
        }
    }
}
        
        validateStep2() {
            // Step 2 is optional (upload), so always valid
            return true;
        }
        
        validateStep3() {
            // Step 3 is review, so always valid
            return true;
        }
        
        validateStep4() {
            // Step 4 is mapping, basic validation
            return true;
        }
        
        updateWizardDisplay() {
            // Update step visibility
            for (let i = 1; i <= this.totalSteps; i++) {
                const step = document.getElementById(`step${i}`);
                const indicator = document.getElementById(`stepIndicator${i}`);
                const circle = document.getElementById(`stepCircle${i}`);
                const connector = document.getElementById(`connector${i}`);
                
                if (step) {
                    step.classList.toggle('active', i === this.currentStep);
                }
                
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
            
            this.updateNavigation();
            this.updateHeader();
        }
        
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
        
        updateHeader() {
            const wizardTitle = document.querySelector('.wizard-title');
            const wizardSubtitle = document.querySelector('.wizard-subtitle');
            
            if (wizardTitle) {
                wizardTitle.textContent = this.stepTitles[this.currentStep - 1];
            }
            
            if (wizardSubtitle) {
                wizardSubtitle.textContent = this.stepDescriptions[this.currentStep - 1];
            }
        }
        
        // PATCH for wizard maximize functionality - Fix maximize button not working
// This can be added to wizard-navigation.js or interface-wizard.js

// FIX: Enhanced toggleMaximize function with better DOM detection and CSS handling
toggleMaximize() {
    console.log('🔍 Enhanced maximize toggle called...');
    
    // FIX: Multiple strategies to find modal elements
    const modalOverlay = document.getElementById('wizardModalOverlay') ||
                        document.querySelector('.wizard-modal-overlay') ||
                        document.querySelector('[id*="wizard"][id*="modal"]') ||
                        document.querySelector('[class*="wizard"][class*="overlay"]');
    
    const modalContainer = document.getElementById('wizardModalContainer') ||
                          document.querySelector('.wizard-modal-container') ||
                          modalOverlay?.querySelector('.wizard-modal-container') ||
                          modalOverlay?.querySelector('[class*="container"]');
    
    const maximizeIcon = document.getElementById('maximizeIcon') ||
                        document.querySelector('#wizardModalMaximize span') ||
                        document.querySelector('.maximize-btn span') ||
                        document.querySelector('[title*="Maximize"] span');
    
    const maximizeBtn = document.getElementById('wizardModalMaximize') ||
                       document.querySelector('.maximize-btn') ||
                       document.querySelector('[title*="Maximize"]') ||
                       document.querySelector('button[onclick*="maximize"]');
    
    console.log('🔍 Modal elements detection:', {
        modalOverlay: !!modalOverlay,
        modalContainer: !!modalContainer,
        maximizeIcon: !!maximizeIcon,
        maximizeBtn: !!maximizeBtn,
        overlayClasses: modalOverlay?.className,
        containerClasses: modalContainer?.className
    });
    
    if (!modalOverlay || !modalContainer) {
        console.error('❌ Modal elements not found for maximize operation');
        this.showNotification?.('Unable to maximize - modal elements not found', 'error');
        return;
    }
    
    // FIX: Check current maximized state from multiple indicators
    const isCurrentlyMaximized = modalOverlay.classList.contains('maximized') ||
                                modalContainer.classList.contains('maximized') ||
                                modalContainer.style.position === 'fixed' ||
                                modalOverlay.style.position === 'fixed' ||
                                maximizeIcon?.textContent === '🗗';
    
    console.log('🔍 Current maximized state:', isCurrentlyMaximized);
    
    try {
        if (isCurrentlyMaximized) {
            // FIX: Restore to normal size
            console.log('📐 Restoring modal to normal size...');
            
            modalOverlay.classList.remove('maximized');
            modalContainer.classList.remove('maximized');
            
            // FIX: Reset all possible CSS styles
            const elementsToReset = [modalOverlay, modalContainer];
            elementsToReset.forEach(element => {
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
            
            // FIX: Update icon and button state
            if (maximizeIcon) maximizeIcon.textContent = '⛶';
            if (maximizeBtn) {
                maximizeBtn.title = 'Maximize';
                maximizeBtn.setAttribute('aria-label', 'Maximize modal');
            }
            
            console.log('✅ Modal restored to normal size');
            this.showNotification?.('Modal restored to normal size', 'success');
            
        } else {
            // FIX: Maximize to full screen
            console.log('📐 Maximizing modal to full screen...');
            
            modalOverlay.classList.add('maximized');
            modalContainer.classList.add('maximized');
            
            // FIX: Apply full screen CSS with !important overrides
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
            
            // Apply to both overlay and container for maximum compatibility
            [modalOverlay, modalContainer].forEach(element => {
                if (element) {
                    Object.entries(fullScreenStyles).forEach(([property, value]) => {
                        element.style.setProperty(property, value, 'important');
                    });
                }
            });
            
            // FIX: Update icon and button state
            if (maximizeIcon) maximizeIcon.textContent = '🗗';
            if (maximizeBtn) {
                maximizeBtn.title = 'Restore';
                maximizeBtn.setAttribute('aria-label', 'Restore modal size');
            }
            
            console.log('✅ Modal maximized to full screen');
            this.showNotification?.('Modal maximized to full screen', 'success');
        }
        
        // FIX: Trigger resize event for any responsive components
        window.dispatchEvent(new Event('resize'));
        
    } catch (error) {
        console.error('❌ Error during maximize operation:', error);
        this.showNotification?.('Failed to toggle maximize state', 'error');
    }
}

// FIX: Enhanced modal event listener setup for maximize button
setupModalEventListeners() {
    if (this.modalListenersSetup) {
        return;
    }

    console.log('🔍 Setting up enhanced modal event listeners...');
    
    // FIX: Enhanced maximize button setup with retry mechanism
    let retryCount = 0;
    const maxRetries = 10;
    
    const setupMaximizeButton = () => {
        retryCount++;
        
        const maximizeBtn = document.getElementById('wizardModalMaximize') ||
                           document.querySelector('.maximize-btn') ||
                           document.querySelector('[title*="Maximize"]') ||
                           document.querySelector('button[onclick*="maximize"]');
        
        const closeBtn = document.getElementById('wizardModalClose') ||
                        document.querySelector('.close-btn') ||
                        document.querySelector('[title*="Close"]');
        
        console.log(`🔍 Attempt ${retryCount} - Modal buttons found:`, {
            maximizeBtn: !!maximizeBtn,
            closeBtn: !!closeBtn
        });
        
        if (maximizeBtn) {
            // FIX: Remove any existing handlers and add new ones
            maximizeBtn.removeAttribute('onclick');
            const oldHandler = maximizeBtn.onclick;
            maximizeBtn.onclick = null;
            
            // Remove any existing event listeners by cloning
            const newMaximizeBtn = maximizeBtn.cloneNode(true);
            maximizeBtn.parentNode.replaceChild(newMaximizeBtn, maximizeBtn);
            
            // Add new event listener
            newMaximizeBtn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.toggleMaximize();
            });
            
            console.log('✅ Maximize button event listener attached');
        }
        
        if (closeBtn) {
            closeBtn.removeAttribute('onclick');
            closeBtn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.closeWizard();
            });
            console.log('✅ Close button event listener attached');
        }
        
        // FIX: Retry if maximize button not found
        if (!maximizeBtn && retryCount < maxRetries) {
            console.log(`⚠️ Maximize button not found, retrying in 100ms... (${retryCount}/${maxRetries})`);
            setTimeout(setupMaximizeButton, 100);
            return;
        }
        
        if (!maximizeBtn) {
            console.warn('⚠️ Maximize button not found after all retries');
        }
        
        // Continue with other modal setup...
      //  this.setupNavigationButtons();
    };
    
    setupMaximizeButton();
    
    // FIX: Backdrop click handling
    const overlay = document.getElementById('wizardModalOverlay') ||
                   document.querySelector('.wizard-modal-overlay');
    
    if (overlay) {
        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) {
                this.closeModal();
            }
        });
    }
    
    // FIX: Keyboard escape handling
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && this.isModalOpen) {
            this.closeModal();
        }
        
        // FIX: F11 for maximize toggle
        if (e.key === 'F11' && this.isModalOpen) {
            e.preventDefault();
            this.toggleMaximize();
        }
    });
    
    this.modalListenersSetup = true;
    console.log('✅ Enhanced modal event listeners setup complete');
}

        
        closeWizard() {
            if (typeof window.closeWizard === 'function') {
                window.closeWizard();
            } else {
                const overlay = document.getElementById('wizardModalOverlay');
                if (overlay) {
                    overlay.classList.remove('show');
                }
            }
        }
        
        finishWizard() {
            // Collect all form data
            const wizardData = this.collectWizardData();
            
            console.log('🎉 Wizard completed with data:', wizardData);
            
            // Show success message
            this.showNotification('Interface created successfully!', 'success');
            
            // Close wizard after delay
            setTimeout(() => {
                this.closeWizard();
                
                // Refresh interface table if function exists
                if (typeof window.loadInterfaces === 'function') {
                    window.loadInterfaces();
                }
            }, 2000);
        }
        
        collectWizardData() {
            return {
                // Step 1 data
                name: document.getElementById('wizardInterfaceName')?.value || '',
                description: document.getElementById('wizardInterfaceDescription')?.value || '',
                sourceType: document.getElementById('wizardSourceType')?.value || '',
                targetType: document.getElementById('wizardTargetType')?.value || '',
                messageType: document.getElementById('wizardMessageType')?.value || '',
                
                // Additional data would be collected from other steps
                timestamp: new Date().toISOString(),
                status: 'created'
            };
        }
        
        showNotification(message, type = 'info') {
            // Simple notification implementation
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
                animation: slideIn 0.3s ease;
            `;
            
            document.body.appendChild(notification);
            
            setTimeout(() => {
                notification.style.animation = 'slideOut 0.3s ease';
                setTimeout(() => notification.remove(), 300);
            }, 3000);
        }
        
        // Public methods
        getCurrentStep() {
            return this.currentStep;
        }
        
        getTotalSteps() {
            return this.totalSteps;
        }
        
        isOnLastStep() {
            return this.currentStep === this.totalSteps;
        }
    }
    
    // Initialize wizard navigation when DOM is ready
    let wizardNav = null;
    
    function initWizardNavigation() {
        if (!wizardNav) {
            wizardNav = new WizardNavigation();
            window.wizardNavigation = wizardNav;
        }
    }
    
    // Auto-initialize
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initWizardNavigation);
    } else {
        initWizardNavigation();
    }
    
    // Add CSS for notifications
    const style = document.createElement('style');
    style.textContent = `
        @keyframes slideIn {
            from { transform: translateX(100%); opacity: 0; }
            to { transform: translateX(0); opacity: 1; }
        }
        
        @keyframes slideOut {
            from { transform: translateX(0); opacity: 1; }
            to { transform: translateX(100%); opacity: 0; }
        }
    `;
    document.head.appendChild(style);
    
    console.log('✅ Wizard navigation controller loaded');
    
})();