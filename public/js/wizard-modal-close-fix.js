// public/js/wizard-modal-close-fix.js
// Emergency fix for wizard modal not closing after completion

(function() {
    'use strict';
    
    console.log('🔧 Loading Wizard Modal Close Fix...');
    
    // Store original finishWizard if it exists
    let originalFinishWizard = null;
    
    // Wait for wizard to be available
    const initFix = setInterval(() => {
        if (window.wizard && window.wizardNavigation) {
            clearInterval(initFix);
            applyModalCloseFix();
        }
    }, 100);
    
    // Clear after 10 seconds to prevent memory leak
    setTimeout(() => clearInterval(initFix), 10000);
    
    function applyModalCloseFix() {
        console.log('🔧 Applying modal close fix...');
        
        // Override the finishWizard method with a working version
        if (window.wizardNavigation && window.wizardNavigation.finishWizard) {
            originalFinishWizard = window.wizardNavigation.finishWizard.bind(window.wizardNavigation);
            
            window.wizardNavigation.finishWizard = async function() {
                console.log('🎯 Enhanced finish wizard triggered');
                
                // Prevent multiple clicks
                if (this.isFinishing) {
                    console.log('Already finishing, ignoring');
                    return;
                }
                
                this.isFinishing = true;
                const finishBtn = document.getElementById('finishBtn');
                
                if (finishBtn) {
                    finishBtn.disabled = true;
                    finishBtn.innerHTML = '<span>⏳</span> Creating...';
                }
                
                try {
                    // Call the original finish wizard logic
                    if (window.wizard && window.wizard.configManager) {
                        await window.wizard.configManager.completeWizard();
                    }
                    
                    // Show success message
                    showSuccessNotification('Interface created successfully!');
                    
                    // Force close the modal after a short delay
                    setTimeout(() => {
                        forceCloseModal();
                        
                        // Reload interfaces
                        if (typeof window.loadInterfaces === 'function') {
                            window.loadInterfaces();
                        }
                    }, 1500);
                    
                } catch (error) {
                    console.error('Error during wizard completion:', error);
                    showErrorNotification('Failed to create interface');
                    
                    // Re-enable button on error
                    if (finishBtn) {
                        finishBtn.disabled = false;
                        finishBtn.innerHTML = 'Create Interface';
                    }
                    this.isFinishing = false;
                }
            };
        }
        
        // Also add a global force close function
        window.forceCloseWizardModal = forceCloseModal;
        
        console.log('✅ Modal close fix applied');
    }
    
    function forceCloseModal() {
        console.log('🚪 Force closing wizard modal...');
        
        // Method 1: Using the overlay element
        const overlay = document.getElementById('wizardModalOverlay');
        if (overlay) {
            // Remove all classes
            overlay.className = 'wizard-modal-overlay';
            
            // Force hide with direct style manipulation
            overlay.style.display = 'none';
            overlay.style.visibility = 'hidden';
            overlay.style.opacity = '0';
            overlay.style.pointerEvents = 'none';
            overlay.style.zIndex = '-9999';
            
            // Also hide any child modals
            const modalContainer = overlay.querySelector('.wizard-modal-container');
            if (modalContainer) {
                modalContainer.style.display = 'none';
            }
        }
        
        // Method 2: Hide all elements with wizard modal classes
        document.querySelectorAll('.wizard-modal-overlay, .wizard-modal, [id*="wizardModal"]').forEach(el => {
            el.style.display = 'none';
            el.style.visibility = 'hidden';
        });
        
        // Reset body styles
        document.body.style.overflow = '';
        document.body.style.paddingRight = '';
        document.body.classList.remove('modal-open', 'wizard-open');
        
        // Reset wizard states
        if (window.wizard) {
            window.wizard.isModalOpen = false;
            window.wizard.currentStep = 1;
        }
        
        if (window.wizardNavigation) {
            window.wizardNavigation.isModalOpen = false;
            window.wizardNavigation.currentStep = 1;
            window.wizardNavigation.isFinishing = false;
        }
        
        // Clear any stored wizard data
        try {
            sessionStorage.removeItem('wizardData');
            sessionStorage.removeItem('wizardStep');
        } catch (e) {
            // Ignore storage errors
        }
        
        console.log('✅ Modal force closed');
    }
    
    function showSuccessNotification(message) {
        const notification = document.createElement('div');
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: #10b981;
            color: white;
            padding: 12px 20px;
            border-radius: 8px;
            font-size: 14px;
            font-weight: 500;
            z-index: 100000;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
            animation: slideIn 0.3s ease;
        `;
        notification.textContent = message;
        document.body.appendChild(notification);
        
        setTimeout(() => {
            notification.style.animation = 'slideOut 0.3s ease';
            setTimeout(() => notification.remove(), 300);
        }, 3000);
    }
    
    function showErrorNotification(message) {
        const notification = document.createElement('div');
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: #ef4444;
            color: white;
            padding: 12px 20px;
            border-radius: 8px;
            font-size: 14px;
            font-weight: 500;
            z-index: 100000;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
        `;
        notification.textContent = message;
        document.body.appendChild(notification);
        
        setTimeout(() => notification.remove(), 5000);
    }
    
    // Add animation styles if not present
    if (!document.getElementById('modal-close-fix-styles')) {
        const style = document.createElement('style');
        style.id = 'modal-close-fix-styles';
        style.textContent = `
            @keyframes slideIn {
                from { transform: translateX(100%); opacity: 0; }
                to { transform: translateX(0); opacity: 1; }
            }
            
            @keyframes slideOut {
                from { transform: translateX(0); opacity: 1; }
                to { transform: translateX(100%); opacity: 0; }
            }
            
            /* Ensure modal is hidden when these styles are applied */
            .wizard-modal-overlay[style*="display: none"] {
                display: none !important;
                visibility: hidden !important;
                opacity: 0 !important;
                pointer-events: none !important;
            }
        `;
        document.head.appendChild(style);
    }
    
})();