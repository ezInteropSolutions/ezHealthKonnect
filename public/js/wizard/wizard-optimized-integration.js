/**
 * Wizard Optimized Integration
 * Connects the new MVC wizard system to the existing ezHealthKonnect application
 *
 * Features:
 * - Seamless integration with existing interface management
 * - Replaces old wizard system components
 * - Maintains backward compatibility
 * - Follows ezHealthKonnect architecture patterns
 */

(function() {
    'use strict';

    console.log('🚀 Loading Optimized Wizard Integration...');

    let optimizedWizard = null;
    let isInitialized = false;

    /**
     * Initialize the optimized wizard system
     */
    function initializeOptimizedWizard() {
        if (isInitialized) {
            console.log('✅ Optimized wizard already initialized');
            return;
        }

        // Check if all required components are loaded
        if (!window.WizardController || !window.WizardModel || !window.WizardView) {
            console.log('⏳ Waiting for wizard components to load...');
            setTimeout(initializeOptimizedWizard, 100);
            return;
        }

        // Wait for DOM to be ready and other scripts to settle
        if (document.readyState !== 'complete') {
            console.log('⏳ Waiting for DOM to be complete...');
            setTimeout(initializeOptimizedWizard, 200);
            return;
        }

        console.log('🎯 Initializing optimized wizard system');

        try {
            // Use existing wizard instance if available, or create new one
            if (window.wizardController) {
                console.log('✅ Using existing wizard controller instance');
                optimizedWizard = window.wizardController;
            } else {
                console.log('🆕 Creating new wizard controller instance');
                optimizedWizard = new window.WizardController('wizard-modal-container');
                window.wizardController = optimizedWizard;
            }

            // Prevent the automatic initialization that causes loops
            optimizedWizard.isInitialized = true;

            // Set up completion handler for interface creation (only once)
            if (!optimizedWizard._completionHandlerAttached) {
                optimizedWizard.addEventListener('wizardCompleted', handleWizardCompletion);
                optimizedWizard._completionHandlerAttached = true;
                console.log('✅ Completion handler attached');
            }

            // Override global wizard functions to use optimized version
            setupGlobalIntegration();

            isInitialized = true;
            console.log('✅ Optimized wizard system initialized successfully');

        } catch (error) {
            console.error('❌ Failed to initialize optimized wizard:', error);
            // Fall back to legacy wizard if available
            console.log('⚠️ Falling back to legacy wizard system');
        }
    }

    /**
     * Handle wizard completion event (post-creation actions only)
     * NOTE: API call is already made by WizardController.finishWizard()
     * This is just for additional actions like analytics, custom notifications, etc.
     */
    async function handleWizardCompletion(event) {
        console.log('🎯 Wizard completion event received (post-creation)');

        const wizardData = event.detail;

        try {
            console.log('✅ Interface created successfully:', {
                interfaceId: wizardData.interfaceId,
                name: wizardData.name
            });

            // NOTE: WizardController already:
            // 1. Made the API call to /api/wizard/complete
            // 2. Showed success notification
            // 3. Closed the wizard
            // 4. Refreshed the interface list
            //
            // This handler is just for logging/analytics

        } catch (error) {
            console.error('❌ Error in post-completion handler:', error);
        }
    }

    /**
     * Set up global integration to replace legacy wizard functions
     */
    function setupGlobalIntegration() {
        // Override the global openWizard function
        window.openOptimizedWizard = function() {
            if (!optimizedWizard) {
                console.error('Optimized wizard not initialized');
                return;
            }

            console.log('🔓 Opening optimized wizard');
            optimizedWizard.openModal();
        };

        // Also provide backward compatibility
        const originalOpenWizard = window.openWizard;
        const originalOpenInterfaceWizard = window.openInterfaceWizard;

        window.openWizard = function() {
            if (optimizedWizard && isInitialized) {
                console.log('🎯 Using optimized wizard');
                window.openOptimizedWizard();
            } else if (originalOpenWizard) {
                console.log('⚠️ Falling back to legacy openWizard');
                originalOpenWizard();
            } else {
                console.error('No wizard system available');
            }
        };

        // Override the legacy interface wizard function too
        window.openInterfaceWizard = function() {
            if (optimizedWizard && isInitialized) {
                console.log('🎯 Using optimized wizard (via openInterfaceWizard)');
                window.openOptimizedWizard();
            } else if (originalOpenInterfaceWizard) {
                console.log('⚠️ Falling back to legacy openInterfaceWizard');
                originalOpenInterfaceWizard();
            } else {
                console.error('No interface wizard system available');
            }
        };

        // Provide wizard instance access for debugging
        window.getOptimizedWizard = function() {
            return optimizedWizard;
        };

        // Set up keyboard shortcuts
        document.addEventListener('keydown', function(event) {
            // Ctrl/Cmd + Shift + W to open wizard
            if ((event.ctrlKey || event.metaKey) && event.shiftKey && event.key === 'W') {
                event.preventDefault();
                window.openOptimizedWizard();
            }
        });

        console.log('🔗 Global wizard integration configured');
    }

    /**
     * Notification functions for user feedback
     */
    function showLoadingNotification(message) {
        removeExistingNotifications();

        const notification = createNotification(message, 'loading');
        notification.innerHTML = `
            <div style="display: flex; align-items: center; gap: 10px;">
                <div class="spinner"></div>
                <span>${message}</span>
            </div>
        `;

        document.body.appendChild(notification);
    }

    function showSuccessNotification(message) {
        removeExistingNotifications();

        const notification = createNotification(message, 'success');
        document.body.appendChild(notification);

        setTimeout(() => {
            notification.style.animation = 'slideOut 0.3s ease';
            setTimeout(() => notification.remove(), 300);
        }, 3000);
    }

    function showErrorNotification(message) {
        removeExistingNotifications();

        const notification = createNotification(message, 'error');
        document.body.appendChild(notification);

        setTimeout(() => notification.remove(), 5000);
    }

    function createNotification(message, type) {
        const colors = {
            loading: { bg: '#2563eb', text: 'white' },
            success: { bg: '#10b981', text: 'white' },
            error: { bg: '#ef4444', text: 'white' }
        };

        const color = colors[type] || colors.loading;

        const notification = document.createElement('div');
        notification.className = 'wizard-notification';
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: ${color.bg};
            color: ${color.text};
            padding: 12px 20px;
            border-radius: 8px;
            font-size: 14px;
            font-weight: 500;
            z-index: 100000;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
            animation: slideIn 0.3s ease;
            max-width: 300px;
        `;
        notification.textContent = message;

        return notification;
    }

    function removeExistingNotifications() {
        document.querySelectorAll('.wizard-notification').forEach(el => el.remove());
    }

    /**
     * CSS for notifications and loading spinner
     */
    function addNotificationStyles() {
        if (document.getElementById('wizard-notification-styles')) return;

        const style = document.createElement('style');
        style.id = 'wizard-notification-styles';
        style.textContent = `
            @keyframes slideIn {
                from { transform: translateX(100%); opacity: 0; }
                to { transform: translateX(0); opacity: 1; }
            }

            @keyframes slideOut {
                from { transform: translateX(0); opacity: 1; }
                to { transform: translateX(100%); opacity: 0; }
            }

            .spinner {
                width: 16px;
                height: 16px;
                border: 2px solid rgba(255, 255, 255, 0.3);
                border-top: 2px solid white;
                border-radius: 50%;
                animation: spin 1s linear infinite;
            }

            @keyframes spin {
                0% { transform: rotate(0deg); }
                100% { transform: rotate(360deg); }
            }
        `;
        document.head.appendChild(style);
    }

    /**
     * Health check for wizard system
     */
    function performHealthCheck() {
        const checks = {
            wizardModel: !!window.WizardModel,
            wizardView: !!window.WizardView,
            wizardController: !!window.WizardController,
            optimizedWizardInstance: !!optimizedWizard,
            initialization: isInitialized
        };

        console.log('🏥 Wizard Health Check:', checks);

        const allHealthy = Object.values(checks).every(Boolean);
        if (allHealthy) {
            console.log('✅ All wizard systems healthy');
        } else {
            console.warn('⚠️ Some wizard systems not ready:',
                Object.entries(checks).filter(([key, value]) => !value)
            );
        }

        return checks;
    }

    /**
     * Initialize when DOM is ready
     */
    function init() {
        addNotificationStyles();

        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', initializeOptimizedWizard);
        } else {
            initializeOptimizedWizard();
        }

        // Periodic health check during development
        if (window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1') {
            setInterval(performHealthCheck, 30000); // Every 30 seconds
        }

        // Expose health check globally for debugging
        window.wizardHealthCheck = performHealthCheck;
    }

    // Start initialization
    init();

    console.log('🎮 Wizard Optimized Integration loaded');

})();